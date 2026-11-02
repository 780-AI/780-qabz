package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/config"
)

type calls struct{ order []string }

func (c *calls) add(s string) { c.order = append(c.order, s) }

type fakeLogger struct {
	c    *calls
	logs []string
}

func (l *fakeLogger) Debug(string, ...any) {}
func (l *fakeLogger) Warn(string, ...any)  {}
func (l *fakeLogger) Info(msg string, kv ...any) {
	l.logs = append(l.logs, msg)
}
func (l *fakeLogger) Error(msg string, kv ...any) { l.logs = append(l.logs, "ERR "+msg) }
func (l *fakeLogger) Sync() error                 { l.c.add("logger.sync"); return nil }

type fakeConn struct {
	name    string
	c       *calls
	pingErr error
}

func (f *fakeConn) Ping(context.Context) error { f.c.add(f.name + ".ping"); return f.pingErr }
func (f *fakeConn) Close()                     { f.c.add(f.name + ".close") }

type fakeMetrics struct {
	c        *calls
	startErr error
	stopErr  error
}

func (f *fakeMetrics) Start(context.Context) error { f.c.add("metrics.start"); return f.startErr }
func (f *fakeMetrics) Stop(context.Context) error  { f.c.add("metrics.stop"); return f.stopErr }

type fakeTracer struct{ c *calls }

func (f *fakeTracer) Shutdown(context.Context) error { f.c.add("tracer.shutdown"); return nil }

type rig struct {
	c       *calls
	log     *fakeLogger
	db      *fakeConn
	cache   *fakeConn
	metrics *fakeMetrics
}

func newRig() *rig {
	c := &calls{}
	return &rig{c: c, log: &fakeLogger{c: c}, db: &fakeConn{name: "db", c: c},
		cache: &fakeConn{name: "cache", c: c}, metrics: &fakeMetrics{c: c}}
}

func (r *rig) builder() *Builder {
	cfg := config.Config{PostgresHost: "h", PostgresPort: 1, PostgresDB: "d", RedisHost: "r", RedisPort: 2, MetricsAddr: ":0"}
	return NewBuilder().WithConfig(cfg).WithLogger(r.log).WithDatabase(r.db).WithCache(r.cache).WithMetrics(r.metrics)
}

func TestBuildRequiresDependencies(t *testing.T) {
	_, err := NewBuilder().Build()
	assert.ErrorContains(t, err, "logger is required")
	assert.ErrorContains(t, err, "database is required")
	assert.ErrorContains(t, err, "cache is required")
	assert.ErrorContains(t, err, "metrics is required")

	a, err := newRig().builder().Build()
	assert.NoError(t, err)
	assert.NotNil(t, a.Database())
	assert.NotNil(t, a.Cache())
	assert.NotNil(t, a.Logger())
}

func TestStartAndStopOrder(t *testing.T) {
	r := newRig()
	a, err := r.builder().WithTracer(&fakeTracer{c: r.c}).Build()
	assert.NoError(t, err)

	assert.NoError(t, a.Start(context.Background()))
	assert.Equal(t, []string{"db.ping", "cache.ping", "metrics.start"}, r.c.order)
	assert.Contains(t, r.log.logs, "postgres ok")
	assert.Contains(t, r.log.logs, "redis ok")

	r.c.order = nil
	assert.NoError(t, a.Stop(context.Background()))
	assert.Equal(t, []string{"metrics.stop", "tracer.shutdown", "cache.close", "db.close", "logger.sync"}, r.c.order)

	r.c.order = nil
	assert.NoError(t, a.Stop(context.Background()), "second Stop is a no-op")
	assert.Empty(t, r.c.order)
}

func TestStartTwiceFails(t *testing.T) {
	a, _ := newRig().builder().Build()
	assert.NoError(t, a.Start(context.Background()))
	assert.ErrorContains(t, a.Start(context.Background()), "already started")
}

func TestStartFailureReleasesWithoutStoppingMetrics(t *testing.T) {
	r := newRig()
	r.cache.pingErr = errors.New("down")
	a, _ := r.builder().Build()

	err := a.Start(context.Background())
	assert.ErrorContains(t, err, "redis")
	assert.Equal(t, []string{"db.ping", "cache.ping", "cache.close", "db.close", "logger.sync"}, r.c.order)
	assert.Contains(t, r.log.logs, "ERR redis ping failed")
	assert.NoError(t, a.Stop(context.Background()), "not started: Stop is a no-op")
}

func TestStartMetricsFailure(t *testing.T) {
	r := newRig()
	r.metrics.startErr = errors.New("busy")
	a, _ := r.builder().Build()
	assert.ErrorContains(t, a.Start(context.Background()), "metrics")
	assert.NotContains(t, r.c.order, "metrics.stop")
}

func TestStopReturnsErrors(t *testing.T) {
	r := newRig()
	r.metrics.stopErr = errors.New("stuck")
	a, _ := r.builder().Build()
	assert.NoError(t, a.Start(context.Background()))
	assert.ErrorContains(t, a.Stop(context.Background()), "stuck")
}

func TestRunStopsOnContextDone(t *testing.T) {
	r := newRig()
	a, _ := r.builder().Build()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	assert.NoError(t, <-done)
	assert.Contains(t, r.c.order, "metrics.stop")
}

func TestBuiltAppIsIsolatedFromBuilder(t *testing.T) {
	r := newRig()
	b := r.builder()
	a, _ := b.Build()
	other := &fakeConn{name: "other", c: r.c}
	b.WithDatabase(other)
	assert.NoError(t, a.Start(context.Background()))
	assert.Equal(t, "db.ping", r.c.order[0])
}

type fakeMigrator struct {
	c   *calls
	err error
}

func (f *fakeMigrator) Up(context.Context) error { f.c.add("migrate.up"); return f.err }

type fakeReloader struct {
	c        *calls
	startErr error
}

func (f *fakeReloader) Start(context.Context) error { f.c.add("reloader.start"); return f.startErr }
func (f *fakeReloader) Stop(context.Context) error  { f.c.add("reloader.stop"); return nil }

func (r *rig) full(migrateOnStart bool) (*App, *fakeMigrator, *fakeReloader) {
	m, rl := &fakeMigrator{c: r.c}, &fakeReloader{c: r.c}
	cfg := config.Config{MigrateOnStart: migrateOnStart, MigrateTimeout: time.Second, HotReloadInterval: time.Second}
	a, _ := r.builder().WithConfig(cfg).WithMigrator(m).WithConfigReloader(rl).Build()
	return a, m, rl
}

func TestMigrateThenReloaderThenMetrics(t *testing.T) {
	r := newRig()
	a, _, _ := r.full(true)
	assert.NoError(t, a.Start(context.Background()))
	assert.Equal(t, []string{"db.ping", "cache.ping", "migrate.up", "reloader.start", "metrics.start"}, r.c.order)

	r.c.order = nil
	assert.NoError(t, a.Stop(context.Background()))
	assert.Equal(t, []string{"metrics.stop", "reloader.stop", "cache.close", "db.close", "logger.sync"}, r.c.order)
}

func TestMigrateOffSkipsMigration(t *testing.T) {
	r := newRig()
	a, _, _ := r.full(false)
	assert.NoError(t, a.Start(context.Background()))
	assert.NotContains(t, r.c.order, "migrate.up")
	assert.Contains(t, r.c.order, "reloader.start")
}

func TestMigrationFailureStopsStart(t *testing.T) {
	r := newRig()
	a, m, _ := r.full(true)
	m.err = errors.New("dirty")
	assert.ErrorContains(t, a.Start(context.Background()), "dirty")
	assert.Equal(t, []string{"db.ping", "cache.ping", "migrate.up", "cache.close", "db.close", "logger.sync"}, r.c.order)
	assert.Contains(t, r.log.logs, "ERR migration failed")
}

func TestReloaderStartFailure(t *testing.T) {
	r := newRig()
	a, _, rl := r.full(true)
	rl.startErr = errors.New("boom")
	assert.ErrorContains(t, a.Start(context.Background()), "config reloader")
	assert.NotContains(t, r.c.order, "metrics.start")
	assert.NotContains(t, r.c.order, "reloader.stop")
}

func TestMetricsFailureStopsReloader(t *testing.T) {
	r := newRig()
	a, _, _ := r.full(true)
	r.metrics.startErr = errors.New("busy")
	assert.Error(t, a.Start(context.Background()))
	assert.Contains(t, r.c.order, "reloader.stop")
	assert.NotContains(t, r.c.order, "metrics.stop")
}

func TestBuildRequiresConfig(t *testing.T) {
	r := newRig()
	_, err := NewBuilder().WithLogger(r.log).WithDatabase(r.db).WithCache(r.cache).WithMetrics(r.metrics).Build()
	assert.ErrorContains(t, err, "config is required")
}

func TestConfigReturnsCurrentHolderValue(t *testing.T) {
	r := newRig()
	h := config.NewHolder(config.Config{ServiceName: "a"})
	a, _ := r.builder().WithConfigHolder(h).Build()
	assert.Equal(t, "a", a.Config().ServiceName)
	h.Set(config.Config{ServiceName: "b"})
	assert.Equal(t, "b", a.Config().ServiceName)
}
