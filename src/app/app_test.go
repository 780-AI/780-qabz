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

type fakeServer struct {
	name     string
	c        *calls
	startErr error
}

func (f *fakeServer) Start(context.Context) error { f.c.add(f.name + ".start"); return f.startErr }
func (f *fakeServer) Stop(context.Context) error  { f.c.add(f.name + ".stop"); return nil }

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

// build runs Build and stops the test if it fails, so no test calls a method on a nil *App.
func build(t *testing.T, b *Builder) *App {
	t.Helper()
	a, err := b.Build()
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	return a
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

	a := build(t, newRig().builder())
	assert.NotNil(t, a.Database())
	assert.NotNil(t, a.Cache())
	assert.NotNil(t, a.Logger())
}

func TestStartAndStopOrder(t *testing.T) {
	r := newRig()
	a := build(t, r.builder().WithTracer(&fakeTracer{c: r.c}))

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
	a := build(t, newRig().builder())
	assert.NoError(t, a.Start(context.Background()))
	assert.ErrorContains(t, a.Start(context.Background()), "already started")
}

func TestStartFailureReleasesWithoutStoppingMetrics(t *testing.T) {
	r := newRig()
	r.cache.pingErr = errors.New("down")
	a := build(t, r.builder())

	err := a.Start(context.Background())
	assert.ErrorContains(t, err, "redis")
	assert.Equal(t, []string{"db.ping", "cache.ping", "cache.close", "db.close", "logger.sync"}, r.c.order)
	assert.Contains(t, r.log.logs, "ERR redis ping failed")
	assert.NoError(t, a.Stop(context.Background()), "not started: Stop is a no-op")
}

func TestStartMetricsFailure(t *testing.T) {
	r := newRig()
	r.metrics.startErr = errors.New("busy")
	a := build(t, r.builder())
	assert.ErrorContains(t, a.Start(context.Background()), "metrics")
	assert.NotContains(t, r.c.order, "metrics.stop")
}

func TestStopReturnsErrors(t *testing.T) {
	r := newRig()
	r.metrics.stopErr = errors.New("stuck")
	a := build(t, r.builder())
	assert.NoError(t, a.Start(context.Background()))
	assert.ErrorContains(t, a.Stop(context.Background()), "stuck")
}

func TestRunStopsOnContextDone(t *testing.T) {
	r := newRig()
	a := build(t, r.builder())
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
	a := build(t, b)
	other := &fakeConn{name: "other", c: r.c}
	b.WithDatabase(other)
	assert.NoError(t, a.Start(context.Background()))
	if assert.NotEmpty(t, r.c.order) {
		assert.Equal(t, "db.ping", r.c.order[0])
	}
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

func (r *rig) full(t *testing.T, migrateOnStart bool) (*App, *fakeMigrator, *fakeReloader) {
	t.Helper()
	m, rl := &fakeMigrator{c: r.c}, &fakeReloader{c: r.c}
	cfg := config.Config{MigrateOnStart: migrateOnStart, MigrateTimeout: time.Second, HotReloadInterval: time.Second}
	a := build(t, r.builder().WithConfig(cfg).WithMigrator(m).WithConfigReloader(rl))
	return a, m, rl
}

func TestMigrateThenReloaderThenMetrics(t *testing.T) {
	r := newRig()
	a, _, _ := r.full(t, true)
	assert.NoError(t, a.Start(context.Background()))
	assert.Equal(t, []string{"db.ping", "cache.ping", "migrate.up", "reloader.start", "metrics.start"}, r.c.order)

	r.c.order = nil
	assert.NoError(t, a.Stop(context.Background()))
	assert.Equal(t, []string{"metrics.stop", "reloader.stop", "cache.close", "db.close", "logger.sync"}, r.c.order)
}

func TestMigrateOffSkipsMigration(t *testing.T) {
	r := newRig()
	a, _, _ := r.full(t, false)
	assert.NoError(t, a.Start(context.Background()))
	assert.NotContains(t, r.c.order, "migrate.up")
	assert.Contains(t, r.c.order, "reloader.start")
}

func TestMigrationFailureStopsStart(t *testing.T) {
	r := newRig()
	a, m, _ := r.full(t, true)
	m.err = errors.New("dirty")
	assert.ErrorContains(t, a.Start(context.Background()), "dirty")
	assert.Equal(t, []string{"db.ping", "cache.ping", "migrate.up", "cache.close", "db.close", "logger.sync"}, r.c.order)
	assert.Contains(t, r.log.logs, "ERR migration failed")
}

func TestReloaderStartFailure(t *testing.T) {
	r := newRig()
	a, _, rl := r.full(t, true)
	rl.startErr = errors.New("boom")
	assert.ErrorContains(t, a.Start(context.Background()), "config reloader")
	assert.NotContains(t, r.c.order, "metrics.start")
	assert.NotContains(t, r.c.order, "reloader.stop")
}

func TestMetricsFailureStopsReloader(t *testing.T) {
	r := newRig()
	a, _, _ := r.full(t, true)
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
	a := build(t, r.builder().WithConfigHolder(h))
	assert.Equal(t, "a", a.Config().ServiceName)
	h.Set(config.Config{ServiceName: "b"})
	assert.Equal(t, "b", a.Config().ServiceName)
}

func (r *rig) servers(t *testing.T) (*App, *fakeServer, *fakeServer) {
	t.Helper()
	g, h := &fakeServer{name: "grpc", c: r.c}, &fakeServer{name: "http", c: r.c}
	cfg := config.Config{MigrateOnStart: false, HotReloadInterval: time.Second}
	a := build(t, r.builder().WithConfig(cfg).WithConfigReloader(&fakeReloader{c: r.c}).WithGRPCServer(g).WithHTTPServer(h))
	return a, g, h
}

func TestServersStartAfterReloaderAndStopBeforeIt(t *testing.T) {
	r := newRig()
	a, _, _ := r.servers(t)
	assert.NoError(t, a.Start(context.Background()))
	assert.Equal(t, []string{"db.ping", "cache.ping", "reloader.start", "grpc.start", "http.start", "metrics.start"}, r.c.order)
	assert.Contains(t, r.log.logs, "grpc started")
	assert.Contains(t, r.log.logs, "http started")

	r.c.order = nil
	assert.NoError(t, a.Stop(context.Background()))
	assert.Equal(t, []string{"metrics.stop", "http.stop", "grpc.stop", "reloader.stop", "cache.close", "db.close", "logger.sync"}, r.c.order)
}

func TestGRPCStartFailureStopsOnlyStartedParts(t *testing.T) {
	r := newRig()
	a, g, _ := r.servers(t)
	g.startErr = errors.New("port busy")
	assert.ErrorContains(t, a.Start(context.Background()), "grpc")
	assert.Equal(t, []string{"db.ping", "cache.ping", "reloader.start", "grpc.start", "reloader.stop", "cache.close", "db.close", "logger.sync"}, r.c.order)
	assert.Contains(t, r.log.logs, "ERR grpc start failed")
}

func TestHTTPStartFailureStopsGRPC(t *testing.T) {
	r := newRig()
	a, _, h := r.servers(t)
	h.startErr = errors.New("port busy")
	assert.ErrorContains(t, a.Start(context.Background()), "http")
	assert.Equal(t, []string{"db.ping", "cache.ping", "reloader.start", "grpc.start", "http.start", "grpc.stop", "reloader.stop", "cache.close", "db.close", "logger.sync"}, r.c.order)
	assert.Contains(t, r.log.logs, "ERR http start failed")
}

func TestMetricsFailureStopsServers(t *testing.T) {
	r := newRig()
	a, _, _ := r.servers(t)
	r.metrics.startErr = errors.New("busy")
	assert.Error(t, a.Start(context.Background()))
	assert.Contains(t, r.c.order, "http.stop")
	assert.Contains(t, r.c.order, "grpc.stop")
	assert.NotContains(t, r.c.order, "metrics.stop")
}
