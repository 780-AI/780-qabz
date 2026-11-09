package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/migrations"
	"qabz/provider"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
)

// liveLogger records the level and message of each line. Safe for concurrent use (the App, the
// config reloader and the provider routine write from their own goroutines).
type liveLogger struct {
	mu    sync.Mutex
	lines [][2]string // level, message
}

func (l *liveLogger) add(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, [2]string{level, msg})
}

func (l *liveLogger) Debug(msg string, _ ...any) { l.add("debug", msg) }
func (l *liveLogger) Info(msg string, _ ...any)  { l.add("info", msg) }
func (l *liveLogger) Warn(msg string, _ ...any)  { l.add("warn", msg) }
func (l *liveLogger) Error(msg string, _ ...any) { l.add("error", msg) }
func (l *liveLogger) Sync() error                { return nil }

// count returns the number of lines with this level and message.
func (l *liveLogger) count(level, msg string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, x := range l.lines {
		if x == [2]string{level, msg} {
			n++
		}
	}
	return n
}

// timedRepo passes each load to the real repository and records the time of each call. Safe for
// concurrent use.
type timedRepo struct {
	inner provider.Repository
	mu    sync.Mutex
	times []time.Time
}

func (r *timedRepo) ListProviders(ctx context.Context) ([]provider.Provider, error) {
	r.mu.Lock()
	r.times = append(r.times, time.Now())
	r.mu.Unlock()
	return r.inner.ListProviders(ctx)
}

// since returns the load times after t, in order.
func (r *timedRepo) since(t time.Time) []time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []time.Time
	for _, x := range r.times {
		if x.After(t) {
			out = append(out, x)
		}
	}
	return out
}

func (r *timedRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.times)
}

// waitUntil polls cond every 20 ms until it is true or timeout ends; then the test fails.
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, msg string) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return assert.Fail(t, "timeout: "+msg)
}

// gapsIn checks that each gap between two loads in times is in [lo, hi].
func gapsIn(t *testing.T, times []time.Time, lo, hi time.Duration, msg string) bool {
	t.Helper()
	ok := true
	for i := 1; i < len(times); i++ {
		gap := times[i].Sub(times[i-1])
		ok = assert.GreaterOrEqual(t, gap, lo, "%s: gap %d", msg, i) && ok
		ok = assert.LessOrEqual(t, gap, hi, "%s: gap %d", msg, i) && ok
	}
	return ok
}

// TestLiveProvidersLoadIntervalHotReload needs a real Postgres (QABZ_TEST_RUN_DSN, make test). It proves
// end to end that providers.general.load_interval is hot-reloadable (operator, 2026-11-09): an App
// with the real migrator, the real config reloader (config.Config, so the hot_reload tag of
// ProvidersLoadInterval decides) and the real ProvidersService of newProviders (interval read from
// the holder, woken by holder.Changed), all on a throw-away schema with the devdb tables and seed
// rows. The migration makes app_hot_configs in that schema. The App starts with a 1 h interval; a
// row providers_load_interval = 1s makes the loads come every second at once, not after the 1 h
// wait in progress (operator decision 9; the test fails within seconds when the wake-up does not
// work); a row value of 200ms makes the routine use the 1 s minimum (the same effective value: no
// restart, no extra load) and write one warn line.
func TestLiveProvidersLoadIntervalHotReload(t *testing.T) {
	dsn := postgrestest.RunDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	schemaDSN := postgrestest.SeededSchema(t, ctx, dsn)
	db, err := postgres.New(ctx, schemaDSN)
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		return
	}
	t.Cleanup(db.Close) // the App closes it too; a second Close is a no-op

	const startInterval = time.Hour
	cfg := testConfig(t)
	cfg.ProvidersLoadInterval = startInterval
	cfg.HotReloadInterval = 100 * time.Millisecond
	cfg.MigrateOnStart = true
	cfg.MigrateTimeout = 30 * time.Second
	holder := config.NewHolder(cfg)
	log := &liveLogger{}
	reloader, err := config.NewReloader[config.Config](postgres.NewConfigRepository(db), holder, cfg.HotReloadInterval, log)
	if !assert.NoError(t, err) || !assert.NotNil(t, reloader) {
		return
	}
	repo := &timedRepo{inner: postgres.NewProviderRepository(db)}
	providers, err := newProviders(holder, repo, prometheus.New("127.0.0.1:0", false), log)
	if !assert.NoError(t, err) || !assert.NotNil(t, providers) {
		return
	}
	c := &calls{}
	b := NewBuilder().WithConfigHolder(holder).WithLogger(log).WithDatabase(db).
		WithCache(&fakeConn{name: "cache", c: c}).WithMetrics(&fakeMetrics{c: c}).
		WithMigrator(postgres.NewMigrator(schemaDSN, migrations.FS)).WithProviders(providers)
	if reloader.HasHotFields() { // as NewDefaultBuilder does
		b.WithConfigReloader(reloader)
	}
	a := build(t, b)
	if !assert.NoError(t, a.Start(ctx)) {
		return
	}
	defer func() { assert.NoError(t, a.Stop(context.Background())) }()
	started := time.Now()
	assert.Equal(t, 1, repo.count(), "the first load runs in Start")
	assert.Equal(t, 1, log.count("info", "config reloader started"), "the App starts the config reloader")

	var hotTable bool
	if err := db.Pool().QueryRow(ctx, `SELECT to_regclass('app_hot_configs') IS NOT NULL`).Scan(&hotTable); !assert.NoError(t, err) ||
		!assert.True(t, hotTable, "the migration made app_hot_configs in the throw-away schema") {
		return
	}
	if _, err := db.Pool().Exec(ctx, `INSERT INTO app_hot_configs (key, cnf_type, value) VALUES ('providers_load_interval', 'duration', '1s')`); !assert.NoError(t, err) {
		return
	}
	if !waitUntil(t, 5*time.Second, func() bool { return holder.Get().ProvidersLoadInterval == time.Second },
		"the config reloader sets the hot value 1s") {
		return
	}
	hot := time.Now()
	// The 1 h wait stops at once: the next load is due at (first load + 1 s), then one every second.
	if !waitUntil(t, 3*time.Second, func() bool { return repo.count() >= 2 }, "a load within about 1 s, not after 1 h") {
		return
	}
	if !waitUntil(t, 8*time.Second, func() bool { return repo.count() >= 5 }, "loads every second") {
		return
	}
	loads := repo.since(started.Add(-time.Hour))
	if !assert.GreaterOrEqual(t, len(loads), 5) {
		return
	}
	assert.Less(t, loads[1].Sub(hot), 2500*time.Millisecond, "the second load follows the hot change")
	gapsIn(t, loads[len(loads)-4:], 800*time.Millisecond, 2500*time.Millisecond, "1s hot value")

	// A hot value under the 1 s minimum: the routine uses 1 s and writes one warn line.
	if _, err := db.Pool().Exec(ctx, `UPDATE app_hot_configs SET value = '200ms', updated_at = now() WHERE key = 'providers_load_interval'`); !assert.NoError(t, err) {
		return
	}
	if !waitUntil(t, 5*time.Second, func() bool { return holder.Get().ProvidersLoadInterval == 200*time.Millisecond },
		"the config reloader sets the hot value 200ms") {
		return
	}
	const warn = "providers: load interval below the minimum; using the minimum"
	if !waitUntil(t, 5*time.Second, func() bool { return log.count("warn", warn) == 1 }, "one warn line for the low value") {
		return
	}
	mark := time.Now()
	time.Sleep(3500 * time.Millisecond)
	low := repo.since(mark)
	assert.GreaterOrEqual(t, len(low), 2, "the routine still loads")
	assert.LessOrEqual(t, len(low), 5, "not every 200 ms")
	gapsIn(t, low, 800*time.Millisecond, 2500*time.Millisecond, "200ms hot value uses 1s")
	assert.Equal(t, 1, log.count("warn", warn), "one warn line for the same low value")
	gapsIn(t, repo.since(hot), 800*time.Millisecond, 2500*time.Millisecond, "no extra load at the 200ms change (the same effective 1 s)")
	assert.Zero(t, log.count("error", "providers: load failed; keeping the last list"))
}
