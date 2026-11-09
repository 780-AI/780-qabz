package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"

	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/provider"
)

type fakeProviders struct {
	c        *calls
	startErr error
}

func (f *fakeProviders) Start(context.Context) error { f.c.add("providers.start"); return f.startErr }
func (f *fakeProviders) Stop(context.Context) error  { f.c.add("providers.stop"); return nil }

// TestProvidersLoadBeforeServers: the provider list starts (its first load) after the config
// reloader and before the servers, and stops after the servers.
func TestProvidersLoadBeforeServers(t *testing.T) {
	r := newRig()
	p := &fakeProviders{c: r.c}
	cfg := config.Config{HotReloadInterval: time.Second, ProvidersLoadInterval: time.Minute}
	a := build(t, r.builder().WithConfig(cfg).WithConfigReloader(&fakeReloader{c: r.c}).WithProviders(p).
		WithGRPCServer(&fakeServer{name: "grpc", c: r.c}).WithHTTPServer(&fakeServer{name: "http", c: r.c}))
	assert.NoError(t, a.Start(context.Background()))
	assert.Equal(t, []string{"db.ping", "cache.ping", "reloader.start", "providers.start", "grpc.start", "http.start", "metrics.start"}, r.c.order)
	assert.Contains(t, r.log.logs, "providers started")

	r.c.order = nil
	assert.NoError(t, a.Stop(context.Background()))
	assert.Equal(t, []string{"metrics.stop", "http.stop", "grpc.stop", "providers.stop", "reloader.stop", "cache.close", "db.close", "logger.sync"}, r.c.order)
}

// TestProvidersStartError: an error of Start itself (not a failed load: that never fails, see
// provider.ProvidersService.Start) stops the App start and releases what started.
func TestProvidersStartError(t *testing.T) {
	r := newRig()
	p := &fakeProviders{c: r.c, startErr: errors.New("already started")}
	a := build(t, r.builder().WithConfigReloader(&fakeReloader{c: r.c}).WithProviders(p).
		WithGRPCServer(&fakeServer{name: "grpc", c: r.c}))
	assert.ErrorContains(t, a.Start(context.Background()), "providers")
	assert.Equal(t, []string{"db.ping", "cache.ping", "reloader.start", "providers.start", "reloader.stop", "cache.close", "db.close", "logger.sync"}, r.c.order)
	assert.Contains(t, r.log.logs, "ERR providers start failed")
}

// TestProvidersMinIntervalIsOne: the config check and the reload routine use the same minimum.
func TestProvidersMinIntervalIsOne(t *testing.T) {
	assert.Equal(t, provider.MinLoadInterval, config.ProvidersMinLoadInterval)
}

// providerRepo answers ListProviders with rows or err.
type providerRepo struct {
	rows []provider.Provider
	err  error
}

func (f *providerRepo) ListProviders(context.Context) ([]provider.Provider, error) {
	return f.rows, f.err
}

// metricValue returns the value of the series name{labels} (a counter or gauge value, or a
// histogram sample count) and whether it exists.
func metricValue(t *testing.T, reg *promclient.Registry, name string, labels map[string]string) (float64, bool) {
	t.Helper()
	families, err := reg.Gather()
	if !assert.NoError(t, err) {
		return 0, false
	}
	for _, f := range families {
		if f == nil || f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			got := map[string]string{}
			for _, l := range m.GetLabel() {
				got[l.GetName()] = l.GetValue()
			}
			if len(got) != len(labels) {
				continue
			}
			match := true
			for k, v := range labels {
				if got[k] != v {
					match = false
				}
			}
			if !match {
				continue
			}
			switch {
			case m.GetCounter() != nil:
				return m.GetCounter().GetValue(), true
			case m.GetGauge() != nil:
				return m.GetGauge().GetValue(), true
			case m.GetHistogram() != nil:
				return float64(m.GetHistogram().GetSampleCount()), true
			}
		}
	}
	return 0, false
}

// TestNewProvidersMetrics: with metrics on, each load result, the load duration, the time of the
// last good load and every change × result pair are recorded; a second call conflicts; with metrics off
// nothing is registered; the hot-reloadable interval is read from the holder.
func TestNewProvidersMetrics(t *testing.T) {
	holder := config.NewHolder(testConfig(t))
	repo := &providerRepo{rows: []provider.Provider{
		{ID: 1, BillType: "gas", Action: "order", Priority: 1, Status: provider.StatusEnable},
		{ID: 2, BillType: "gas", Action: "order", Priority: 2, Status: provider.StatusEnable},
	}}
	on := prometheus.New("127.0.0.1:0", true)
	reg := on.Registry()
	if !assert.NotNil(t, reg) {
		return
	}
	s, err := newProviders(holder, repo, on, &fakeLogger{c: &calls{}})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	v, ok := metricValue(t, reg, "qabz_providers_last_load_success_timestamp_ms", map[string]string{})
	assert.True(t, ok)
	assert.Zero(t, v, "no good load yet")

	before := float64(time.Now().UnixMilli())
	assert.NoError(t, s.Load(context.Background()))
	repo.err = errors.New("down")
	assert.Error(t, s.Load(context.Background()))
	assert.Error(t, s.Load(context.Background()))
	for labels, want := range map[string]float64{provider.LoadOK: 1, provider.LoadError: 2} {
		v, ok := metricValue(t, reg, "qabz_providers_loads_total", map[string]string{"result": labels})
		assert.True(t, ok, labels)
		assert.Equal(t, want, v, labels)
	}
	v, ok = metricValue(t, reg, "qabz_providers_load_duration_ms", map[string]string{})
	assert.True(t, ok)
	assert.Equal(t, float64(3), v, "one sample per load")
	v, _ = metricValue(t, reg, "qabz_providers_last_load_success_timestamp_ms", map[string]string{})
	assert.GreaterOrEqual(t, v, before)
	assert.LessOrEqual(t, v, float64(time.Now().UnixMilli()))

	// Each change × result pair that can occur, once: rows 1 and 2 of gas/order, 9 is unknown.
	assert.NoError(t, s.MoveUp(2))                                // move_up changed: 2 first
	assert.NoError(t, s.MoveUp(2))                                // move_up unchanged: 2 is first
	assert.ErrorIs(t, s.MoveUp(9), provider.ErrUnknownProvider)   // move_up unknown_id
	assert.NoError(t, s.MoveDown(2))                              // move_down changed: 2 last
	assert.NoError(t, s.MoveDown(2))                              // move_down unchanged: 2 is last
	assert.ErrorIs(t, s.MoveDown(9), provider.ErrUnknownProvider) // move_down unknown_id
	assert.NoError(t, s.Disable(1))                               // disable changed
	assert.NoError(t, s.Disable(1))                               // disable unchanged
	assert.ErrorIs(t, s.Disable(9), provider.ErrUnknownProvider)  // disable unknown_id
	assert.NoError(t, s.Enable(1))                                // enable changed
	assert.NoError(t, s.Enable(1))                                // enable unchanged
	assert.ErrorIs(t, s.Enable(9), provider.ErrUnknownProvider)   // enable unknown_id
	for _, change := range []string{provider.ChangeMoveUp, provider.ChangeMoveDown, provider.ChangeEnable, provider.ChangeDisable} {
		for _, result := range []string{provider.ResultChanged, provider.ResultUnchanged, provider.ResultUnknownID} {
			v, ok := metricValue(t, reg, "qabz_providers_changes_total", map[string]string{"change": change, "result": result})
			assert.True(t, ok, "%s/%s", change, result)
			assert.Equal(t, float64(1), v, "%s/%s", change, result)
		}
	}

	_, err = newProviders(holder, repo, on, &fakeLogger{c: &calls{}})
	assert.ErrorContains(t, err, "providers: register metric")

	off := prometheus.New("127.0.0.1:0", false)
	for range 2 {
		s, err := newProviders(holder, repo, off, &fakeLogger{c: &calls{}})
		assert.NoError(t, err)
		assert.NotNil(t, s)
	}
	_, err = newProviders(nil, repo, off, &fakeLogger{c: &calls{}})
	assert.ErrorContains(t, err, "config holder is required")
	_, err = newProviders(holder, nil, off, &fakeLogger{c: &calls{}})
	assert.ErrorContains(t, err, "repository is required")
}

// countingRepo counts the loads. Safe for concurrent use.
type countingRepo struct{ n atomic.Int64 }

func (f *countingRepo) ListProviders(context.Context) ([]provider.Provider, error) {
	f.n.Add(1)
	return nil, nil
}

// TestNewProvidersReadsIntervalFromHolder: the load interval comes from the config holder before
// each wait, so a hot-reloaded value applies without restart: at 1 s the routine loads every
// second; after the value becomes 1 h it stops loading once the wait in progress ends.
func TestNewProvidersReadsIntervalFromHolder(t *testing.T) {
	cfg := testConfig(t)
	cfg.ProvidersLoadInterval = time.Second
	holder := config.NewHolder(cfg)
	repo := &countingRepo{}
	s, err := newProviders(holder, repo, prometheus.New("127.0.0.1:0", false), &fakeLogger{c: &calls{}})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	assert.NoError(t, s.Start(context.Background()))
	defer func() { assert.NoError(t, s.Stop(context.Background())) }()
	assert.Equal(t, int64(1), repo.n.Load(), "the first load runs in Start")

	time.Sleep(2300 * time.Millisecond)
	assert.GreaterOrEqual(t, repo.n.Load(), int64(3), "loads at 1 s")

	cfg.ProvidersLoadInterval = time.Hour
	holder.Set(cfg)
	time.Sleep(1100 * time.Millisecond) // the 1 s wait in progress ends; the next wait is 1 h
	n := repo.n.Load()
	time.Sleep(1200 * time.Millisecond)
	assert.Equal(t, n, repo.n.Load(), "no load after the value became 1 h")
}

// TestNewProvidersActiveCount: qabz_providers_active_count (no labels) is the number of enabled
// providers in all groups: set after a good load and after an Enable or Disable that changed the
// list; an unchanged Enable or Disable and a failed load keep the value; an empty list gives 0.
func TestNewProvidersActiveCount(t *testing.T) {
	repo := &providerRepo{rows: []provider.Provider{
		{ID: 1, BillType: "gas", Action: "order", Priority: 1, Status: provider.StatusEnable},
		{ID: 2, BillType: "gas", Action: "order", Priority: 2, Status: provider.StatusDisable},
		{ID: 3, BillType: "water", Action: "inquiry", Priority: 1, Status: provider.StatusEnable},
		{ID: 4, BillType: "water", Action: "inquiry", Priority: 2, Status: ""},
	}}
	on := prometheus.New("127.0.0.1:0", true)
	reg := on.Registry()
	if !assert.NotNil(t, reg) {
		return
	}
	s, err := newProviders(config.NewHolder(testConfig(t)), repo, on, &fakeLogger{c: &calls{}})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	active := func() float64 {
		t.Helper()
		v, ok := metricValue(t, reg, "qabz_providers_active_count", map[string]string{})
		assert.True(t, ok, "the gauge has no labels")
		return v
	}
	assert.Zero(t, active(), "no load yet")

	assert.NoError(t, s.Load(context.Background()))
	assert.Equal(t, float64(2), active(), "after a load: 1 and 3")
	assert.NoError(t, s.Enable(2))
	assert.Equal(t, float64(3), active(), "after Enable")
	assert.NoError(t, s.Disable(3))
	assert.Equal(t, float64(2), active(), "after Disable")
	assert.NoError(t, s.Disable(3))
	assert.NoError(t, s.Enable(2))
	assert.NoError(t, s.MoveDown(1))
	assert.Equal(t, float64(2), active(), "unchanged Enable / Disable and a move")

	repo.err = errors.New("down")
	assert.Error(t, s.Load(context.Background()))
	assert.Equal(t, float64(2), active(), "a failed load keeps the value")

	repo.err = nil
	assert.NoError(t, s.Load(context.Background()))
	assert.Equal(t, float64(2), active(), "the load resets the runtime status")
	repo.rows = nil
	assert.NoError(t, s.Load(context.Background()))
	assert.Zero(t, active(), "an empty list")
}
