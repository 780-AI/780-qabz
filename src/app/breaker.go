package app

// The circuit breaker of the provider rows (task 060): the wiring of provider.Breaker with the hot
// config keys providers.general.circuit_breaker.* and providers.<group>.<provider>.circuit_breaker.*
// and the metrics qabz_circuit_breaker_state and qabz_circuit_breaker_transitions_total.

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"

	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/provider"
	"qabz/provider/irancellproxy"
	"qabz/provider/jiring"
	"qabz/provider/karashop"
	"qabz/provider/parsianproxy"
	"qabz/provider/qabzino"
	"qabz/provider/tavanir"
)

// breakerProviders maps the config path of each provider block with a circuit_breaker override
// (config.Config.CircuitBreakerOverrides) to the provider name of its rows (providers.name). A row
// of another provider takes the general values.
var breakerProviders = map[string]string{
	"internal.irancell":      irancellproxy.Name,
	"internal.jiring_switch": jiring.Name,
	"internal.parsian":       parsianproxy.Name,
	"external.karashop":      karashop.Name,
	"external.qabzino":       qabzino.Name,
	"external.tavanir":       tavanir.Name,
}

// newBreaker builds the circuit breaker (task 060). demote is ProvidersService.Demote. With metrics
// on, each open sets qabz_circuit_breaker_state{provider,bill_type,action} to 1 and each close to 0,
// and each adds 1 to qabz_circuit_breaker_transitions_total{provider,bill_type,action,to}.
func newBreaker(holder *config.Holder[config.Config], demote func(id int64, period time.Duration) error, metrics *prometheus.Server, logger Logger) (*provider.Breaker, error) {
	if holder == nil {
		return nil, errors.New("circuit breaker: config holder is required")
	}
	if logger == nil {
		return nil, errors.New("circuit breaker: logger is required")
	}
	var onTransition func(p provider.Provider, to string) // nil: metrics are off
	if reg := metrics.Registry(); reg != nil {
		state := promclient.NewGaugeVec(promclient.GaugeOpts{
			Name: "qabz_circuit_breaker_state",
			Help: "State of the circuit breaker (task 060) of each provider row (provider, bill type, action: inquiry, order): 0 closed (it counts), 1 open (the row is demoted by the Provider Switcher).",
		}, []string{"provider", "bill_type", "action"})
		transitions := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_circuit_breaker_transitions_total",
			Help: "Changes of the state of the circuit breaker (task 060) of each provider row (provider, bill type, action: inquiry, order), by to (open: the row was demoted; closed: the row returned).",
		}, []string{"provider", "bill_type", "action", "to"})
		for _, c := range []promclient.Collector{state, transitions} {
			if err := reg.Register(c); err != nil {
				return nil, fmt.Errorf("circuit breaker: register metric: %w", err)
			}
		}
		onTransition = func(p provider.Provider, to string) {
			v := 0.0
			if to == provider.BreakerOpen {
				v = 1
			}
			state.WithLabelValues(p.Name, p.BillType, p.Action).Set(v)
			transitions.WithLabelValues(p.Name, p.BillType, p.Action, to).Inc()
		}
	}
	settings := newBreakerSettings(holder, logger)
	return provider.NewBreaker(provider.BreakerOptions{
		Settings:     settings.get,
		Demote:       demote,
		Logger:       logger,
		OnTransition: onTransition,
	})
}

// breakerSettings gives the settings of the breaker of each provider name from the hot config. The
// table is built again only after a change of the config (config.Holder.Changed), so a report reads
// one atomic pointer and one map: no copy of the config on the request path.
type breakerSettings struct {
	holder *config.Holder[config.Config]
	log    Logger
	mu     sync.Mutex // one build at a time
	cur    atomic.Pointer[breakerTable]
}

// breakerTable is the settings of one config value. It is never changed after it is stored.
type breakerTable struct {
	changed  <-chan struct{} // closed by the next config change
	general  provider.BreakerSettings
	byName   map[string]provider.BreakerSettings
	rawGen   config.CircuitBreaker            // the configured general values (to warn once at a change)
	rawByKey map[string]config.CircuitBreaker // the configured overrides by config path
}

func newBreakerSettings(holder *config.Holder[config.Config], logger Logger) *breakerSettings {
	s := &breakerSettings{holder: holder, log: logger}
	s.cur.Store(s.build(nil))
	return s
}

// get is BreakerOptions.Settings: the settings of the row p.
func (s *breakerSettings) get(p provider.Provider) provider.BreakerSettings {
	t := s.cur.Load()
	select {
	case <-t.changed:
		t = s.rebuild()
	default:
	}
	if b, ok := t.byName[p.Name]; ok {
		return b
	}
	return t.general
}

// rebuild builds the table again after a config change (once for each change).
func (s *breakerSettings) rebuild() *breakerTable {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.cur.Load()
	select {
	case <-t.changed:
	default:
		return t // another report built it
	}
	next := s.build(t)
	s.cur.Store(next)
	return next
}

// build makes the table of the current config. A value out of range is not used (operator decision
// 6): a general value takes its default, an override value the general value. prev is the table of
// the last config (nil at start): a warn line is written only for a value that changed, so one bad
// value gives one line. At start Validate has refused a bad value, so prev nil writes no line.
func (s *breakerSettings) build(prev *breakerTable) *breakerTable {
	changed := s.holder.Changed() // before Get (config.Holder)
	cfg := s.holder.Get()
	t := &breakerTable{changed: changed, rawGen: cfg.CircuitBreakerGeneral(), rawByKey: cfg.CircuitBreakerOverrides(),
		byName: make(map[string]provider.BreakerSettings, len(breakerProviders))}
	g := t.rawGen
	def := config.CircuitBreaker{FailureRatio: config.DefaultCircuitBreakerFailureRatio, Window: config.DefaultCircuitBreakerWindow,
		MinCalls: config.DefaultCircuitBreakerMinCalls, OpenTime: config.DefaultCircuitBreakerOpenTime}
	if !g.GeneralValid() {
		if prev != nil && prev.rawGen != g {
			s.log.Warn("circuit breaker: a hot value of providers.general.circuit_breaker is out of range, the default is used for it")
		}
		g = merge(g, def)
	}
	t.general = settingsOf(cfg.CircuitBreakerEnabled, g)
	for _, key := range slices.Sorted(maps.Keys(breakerProviders)) {
		o, ok := t.rawByKey[key]
		if !ok {
			continue
		}
		if !o.OverrideValid() && prev != nil && prev.rawByKey[key] != o {
			s.log.Warn("circuit breaker: a hot value of the override is out of range, the general value is used for it",
				"path", "providers."+key+".circuit_breaker")
		}
		// 0 and a value out of range take the general value.
		b := merge(o, g)
		t.byName[breakerProviders[key]] = settingsOf(cfg.CircuitBreakerEnabled, b)
	}
	return t
}

// merge keeps each value of v that is in the range of the general block and takes the value of
// fallback for the others (0 too).
func merge(v, fallback config.CircuitBreaker) config.CircuitBreaker {
	if v.FailureRatio < 1 || v.FailureRatio > 100 {
		v.FailureRatio = fallback.FailureRatio
	}
	if v.Window <= 0 {
		v.Window = fallback.Window
	}
	if v.MinCalls < 1 {
		v.MinCalls = fallback.MinCalls
	}
	if v.OpenTime <= 0 {
		v.OpenTime = fallback.OpenTime
	}
	return v
}

func settingsOf(enabled bool, b config.CircuitBreaker) provider.BreakerSettings {
	return provider.BreakerSettings{Enabled: enabled, FailureRatio: b.FailureRatio, Window: b.Window, MinCalls: b.MinCalls, OpenTime: b.OpenTime}
}
