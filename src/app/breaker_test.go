package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/provider"
)

// Task 060: the wiring of the circuit breaker.

func cbRow(name string) provider.Provider {
	return provider.Provider{ID: 5, Name: name, BillType: "water", Action: "inquiry"}
}

// TestBreakerSettingsDefaults: the general values for every provider; enabled false by default; a
// provider with no config block takes the general values.
func TestBreakerSettingsDefaults(t *testing.T) {
	cfg := testConfig(t)
	s := newBreakerSettings(config.NewHolder(cfg), &liveLogger{})
	want := provider.BreakerSettings{Enabled: false, FailureRatio: 30, Window: 2 * time.Minute, MinCalls: 20, OpenTime: time.Minute}
	for _, name := range []string{"irancell", "jiring-switch", "parsian", "karashop", "qabzino", "tavanir", "eways2"} {
		assert.Equal(t, want, s.get(cbRow(name)), name)
	}
	assert.Len(t, breakerProviders, len(cfg.CircuitBreakerOverrides()), "every override block has a provider name")
}

// TestBreakerSettingsOverride: each key of an override replaces the general value for its provider
// only; 0 takes the general value.
func TestBreakerSettingsOverride(t *testing.T) {
	cfg := testConfig(t)
	cfg.CircuitBreakerEnabled = true
	cfg.TavanirCircuitBreakerWindow = 5 * time.Minute
	cfg.QabzinoCircuitBreakerFailureRatio = 60
	cfg.QabzinoCircuitBreakerMinCalls = 3
	cfg.QabzinoCircuitBreakerOpenTime = 10 * time.Second
	s := newBreakerSettings(config.NewHolder(cfg), &liveLogger{})
	assert.Equal(t, provider.BreakerSettings{Enabled: true, FailureRatio: 30, Window: 5 * time.Minute, MinCalls: 20, OpenTime: time.Minute}, s.get(cbRow("tavanir")))
	assert.Equal(t, provider.BreakerSettings{Enabled: true, FailureRatio: 60, Window: 2 * time.Minute, MinCalls: 3, OpenTime: 10 * time.Second}, s.get(cbRow("qabzino")))
	assert.Equal(t, provider.BreakerSettings{Enabled: true, FailureRatio: 30, Window: 2 * time.Minute, MinCalls: 20, OpenTime: time.Minute}, s.get(cbRow("karashop")))
}

// TestBreakerSettingsHotReload: a change of each key applies at the next report; a hot value out
// of range is not used (a general value takes its default, an override the general value) with
// one warn line at the change, not at each report.
func TestBreakerSettingsHotReload(t *testing.T) {
	cfg := testConfig(t)
	holder := config.NewHolder(cfg)
	log := &liveLogger{}
	s := newBreakerSettings(holder, log)
	// A fixed order: the override of parsian (last) wins over the general window.
	for _, tc := range []struct {
		name   string
		change func(c *config.Config)
		check  func(b provider.BreakerSettings) bool
	}{
		{"enabled", func(c *config.Config) { c.CircuitBreakerEnabled = true }, func(b provider.BreakerSettings) bool { return b.Enabled }},
		{"failure_ratio", func(c *config.Config) { c.CircuitBreakerFailureRatio = 45 }, func(b provider.BreakerSettings) bool { return b.FailureRatio == 45 }},
		{"window", func(c *config.Config) { c.CircuitBreakerWindow = time.Hour }, func(b provider.BreakerSettings) bool { return b.Window == time.Hour }},
		{"min_calls", func(c *config.Config) { c.CircuitBreakerMinCalls = 7 }, func(b provider.BreakerSettings) bool { return b.MinCalls == 7 }},
		{"open_time", func(c *config.Config) { c.CircuitBreakerOpenTime = time.Second }, func(b provider.BreakerSettings) bool { return b.OpenTime == time.Second }},
		{"override", func(c *config.Config) { c.ParsianCircuitBreakerWindow = 9 * time.Second }, func(b provider.BreakerSettings) bool { return b.Window == 9*time.Second }},
	} {
		c := holder.Get()
		tc.change(&c)
		holder.Set(c)
		assert.True(t, tc.check(s.get(cbRow("parsian"))), tc.name)
	}
	assert.Equal(t, 0, log.count("warn", "circuit breaker: a hot value of providers.general.circuit_breaker is out of range, the default is used for it"))

	c := holder.Get()
	c.CircuitBreakerMinCalls = 0       // out of range: the default 20
	c.ParsianCircuitBreakerWindow = -1 // out of range: the general window
	c.KarashopCircuitBreakerFailureRatio = 101
	holder.Set(c)
	b := s.get(cbRow("parsian"))
	assert.Equal(t, 20, b.MinCalls)
	assert.Equal(t, time.Hour, b.Window)
	assert.Equal(t, 45, b.FailureRatio, "the other general values stay")
	assert.Equal(t, 45, s.get(cbRow("karashop")).FailureRatio)
	for range 3 {
		s.get(cbRow("parsian"))
	}
	assert.Equal(t, 1, log.count("warn", "circuit breaker: a hot value of providers.general.circuit_breaker is out of range, the default is used for it"))
	assert.Equal(t, 2, log.count("warn", "circuit breaker: a hot value of the override is out of range, the general value is used for it"))
	c.LogLevel = "debug" // another change: the bad values did not change, no new line
	holder.Set(c)
	s.get(cbRow("parsian"))
	assert.Equal(t, 1, log.count("warn", "circuit breaker: a hot value of providers.general.circuit_breaker is out of range, the default is used for it"))
	assert.Equal(t, 2, log.count("warn", "circuit breaker: a hot value of the override is out of range, the general value is used for it"))
}

// TestNewBreakerMetrics: qabz_circuit_breaker_state is 1 while open and 0 after the close; each
// change adds 1 to qabz_circuit_breaker_transitions_total; with metrics off nothing is registered;
// the required arguments; a second registration fails.
func TestNewBreakerMetrics(t *testing.T) {
	holder := config.NewHolder(testConfig(t))
	dem := func(int64, time.Duration) error { return nil }
	on := prometheus.New("127.0.0.1:0", true)
	b, err := newBreaker(holder, dem, on, &liveLogger{})
	if !assert.NoError(t, err) {
		return
	}
	labels := map[string]string{"provider": "qabzino", "bill_type": "water", "action": "inquiry"}
	to := func(v string) map[string]string {
		return map[string]string{"provider": "qabzino", "bill_type": "water", "action": "inquiry", "to": v}
	}
	b.Demoted(cbRow("qabzino"), true)
	v, _ := metricValue(t, on.Registry(), "qabz_circuit_breaker_state", labels)
	assert.Equal(t, 1.0, v)
	b.Demoted(cbRow("qabzino"), false)
	v, _ = metricValue(t, on.Registry(), "qabz_circuit_breaker_state", labels)
	assert.Equal(t, 0.0, v)
	v, _ = metricValue(t, on.Registry(), "qabz_circuit_breaker_transitions_total", to("open"))
	assert.Equal(t, 1.0, v)
	v, _ = metricValue(t, on.Registry(), "qabz_circuit_breaker_transitions_total", to("closed"))
	assert.Equal(t, 1.0, v)

	_, err = newBreaker(holder, dem, on, &liveLogger{})
	assert.ErrorContains(t, err, "circuit breaker: register metric")
	off := prometheus.New("127.0.0.1:0", false)
	b, err = newBreaker(holder, dem, off, &liveLogger{})
	if assert.NoError(t, err) {
		assert.NotPanics(t, func() { b.Demoted(cbRow("qabzino"), true) })
	}
	_, err = newBreaker(nil, dem, off, &liveLogger{})
	assert.ErrorContains(t, err, "config holder is required")
	_, err = newBreaker(holder, dem, off, nil)
	assert.ErrorContains(t, err, "logger is required")
	_, err = newBreaker(holder, nil, off, &liveLogger{})
	assert.ErrorContains(t, err, "demote is required")
}

// TestNewProvidersAndBreaker: the breaker demotes through the provider list; before the list is set
// the demote is refused.
func TestNewProvidersAndBreaker(t *testing.T) {
	repo := &providerRepo{rows: []provider.Provider{
		{ID: 1, Name: "qabzino", BillType: "water", Action: "inquiry", Priority: 1, Status: provider.StatusEnable},
		{ID: 2, Name: "tavanir", BillType: "water", Action: "inquiry", Priority: 2, Status: provider.StatusEnable},
	}}
	cfg := testConfig(t)
	cfg.CircuitBreakerEnabled = true
	cfg.CircuitBreakerMinCalls = 2
	log := &liveLogger{}
	list, b, err := newProvidersAndBreaker(config.NewHolder(cfg), repo, prometheus.New("127.0.0.1:0", true), log, nil)
	if !assert.NoError(t, err) || !assert.NoError(t, list.Load(t.Context())) {
		return
	}
	b.Report(repo.rows[0], true)
	b.Report(repo.rows[0], true)
	assert.Eventually(t, func() bool {
		p, _ := list.PickProvider("water", provider.ActionInquiry)
		return p.ID == 2
	}, 3*time.Second, time.Millisecond)
	assert.Eventually(t, func() bool { return log.count("info", "circuit breaker: open, row demoted") == 1 }, 3*time.Second, time.Millisecond)
	_, _, err = newProvidersAndBreaker(nil, repo, prometheus.New("127.0.0.1:0", false), log, nil)
	assert.Error(t, err)
}
