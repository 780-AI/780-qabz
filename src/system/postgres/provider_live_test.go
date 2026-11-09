package postgres_test

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
)

// seededClient connects to a new throw-away schema with the devdb bill tables and the 45 seed
// providers (QABZ_TEST_RUN_DSN, make test), so the real tables are never touched. It skips the test
// without the DSN and stops it if a step fails.
func seededClient(t *testing.T, ctx context.Context) *postgres.Client {
	t.Helper()
	dsn := postgrestest.RunDSN(t)
	c, err := postgres.New(ctx, postgrestest.SeededSchema(t, ctx, dsn))
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	t.Cleanup(c.Close)
	return c
}

// directProviders reads the providers with a direct SQL query, independent of the repository: NULL
// columns scanned into pointers and turned into zero values here. With sorted, Postgres sorts the
// rows in the reference order (bill type, action, priority, id; byte order as Go compares strings)
// and TmpPriority is set to Priority; otherwise the rows come by id.
func directProviders(t *testing.T, ctx context.Context, c *postgres.Client, sorted bool) []provider.Provider {
	t.Helper()
	order := ` ORDER BY id`
	if sorted {
		order = ` ORDER BY COALESCE(bill_type, '') COLLATE "C", COALESCE(action, '') COLLATE "C", COALESCE(priority, 0), id`
	}
	rows, err := c.Pool().Query(ctx, `SELECT id, name, subject, bill_type, action, type, priority, timeout, status,
		updated_at, use_nats, inquiry_step, provider_fee FROM providers`+order)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	defer rows.Close()
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	out := []provider.Provider{}
	for rows.Next() {
		var id int64
		var name, subject, billType, action, typ, status *string
		var priority, inquiryStep *int16
		var timeout *int32
		var updated *time.Time
		var useNats *bool
		var fee *int64
		if !assert.NoError(t, rows.Scan(&id, &name, &subject, &billType, &action, &typ, &priority, &timeout, &status,
			&updated, &useNats, &inquiryStep, &fee)) {
			t.FailNow()
		}
		p := provider.Provider{ID: id, Name: str(name), Subject: str(subject), BillType: str(billType), Action: str(action),
			Type: str(typ), Status: str(status)}
		if priority != nil {
			p.Priority = int(*priority)
		}
		if timeout != nil {
			p.Timeout = int(*timeout)
		}
		if updated != nil {
			p.UpdatedAt = *updated
		}
		if useNats != nil {
			p.UseNats = *useNats
		}
		if inquiryStep != nil {
			p.InquiryStep = int(*inquiryStep)
		}
		if fee != nil {
			p.ProviderFee = *fee
		}
		if sorted {
			p.TmpPriority = p.Priority
		}
		out = append(out, p)
	}
	if !assert.NoError(t, rows.Err()) {
		t.FailNow()
	}
	return out
}

// sameTimes returns ps with each UpdatedAt in UTC, so a compare does not depend on the location.
func sameTimes(ps []provider.Provider) []provider.Provider {
	out := slices.Clone(ps)
	for i := range out {
		out[i].UpdatedAt = out[i].UpdatedAt.UTC()
	}
	return out
}

// TestLiveListProviders: the repository reads every row (the 45 seed rows, a row with NULL
// columns and a row with a negative priority) as a direct SQL query does.
func TestLiveListProviders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	_, err := c.Pool().Exec(ctx, `INSERT INTO providers (id, name, bill_type, action, priority, status, updated_at, use_nats, inquiry_step, provider_fee)
		VALUES (9001, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL),
		       (9002, 'neg', 'gas', 'order', -7, 'enable', now(), false, 2, 1500)`)
	if !assert.NoError(t, err) {
		return
	}
	got, err := postgres.NewProviderRepository(c).ListProviders(ctx)
	if !assert.NoError(t, err) {
		return
	}
	slices.SortFunc(got, func(a, b provider.Provider) int { return cmp.Compare(a.ID, b.ID) })
	want := directProviders(t, ctx, c, false)
	assert.Len(t, got, 47)
	assert.Equal(t, sameTimes(want), sameTimes(got))
	i := slices.IndexFunc(got, func(p provider.Provider) bool { return p.ID == 9001 })
	if assert.GreaterOrEqual(t, i, 0) {
		assert.Equal(t, provider.Provider{ID: 9001}, got[i], "NULL columns read as zero values")
	}

	_, err = c.Pool().Exec(ctx, `DELETE FROM providers`)
	assert.NoError(t, err)
	got, err = postgres.NewProviderRepository(c).ListProviders(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []provider.Provider{}, got)
}

// liveLog records the lines of the service. Safe for concurrent use.
type liveLog struct {
	mu     sync.Mutex
	errors int
	debug  int
}

func (l *liveLog) Debug(string, ...any) { l.mu.Lock(); l.debug++; l.mu.Unlock() }
func (l *liveLog) Warn(string, ...any)  {}
func (l *liveLog) Error(string, ...any) { l.mu.Lock(); l.errors++; l.mu.Unlock() }

// TestLiveProvidersServiceReload: the running service (1 s load interval) picks up, without
// restart, an UPDATE of priority, an UPDATE of status, an INSERT and a DELETE; after each one the
// list equals the reference sort of Postgres and PickProvider gives the expected provider. A load
// with no change keeps the order. The 45 seed rows load and sort as the reference says.
func TestLiveProvidersServiceReload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	var loads atomic.Int64
	log := &liveLog{}
	s, err := provider.NewProvidersService(postgres.NewProviderRepository(c), provider.Options{
		Interval: func() time.Duration { return time.Second },
		Logger:   log,
		OnLoad: func(result string, _ time.Duration) {
			if result == provider.LoadOK {
				loads.Add(1)
			}
		},
	})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	if !assert.NoError(t, s.Start(ctx)) {
		return
	}
	defer func() { assert.NoError(t, s.Stop(context.Background())) }()

	// check checks the list against Postgres and the pick of gas/inquiry.
	check := func(step string, pick int64) bool {
		if !assert.Equal(t, sameTimes(directProviders(t, ctx, c, true)), sameTimes(s.List()), step) {
			return false
		}
		p, err := s.PickProvider("gas", "inquiry")
		if pick == 0 {
			return assert.ErrorIs(t, err, provider.ErrNoProvider, step)
		}
		return assert.NoError(t, err, step) && assert.Equal(t, pick, p.ID, step)
	}
	// waitLoad waits until a load that started after the call has ended.
	waitLoad := func() bool {
		n := loads.Load()
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if loads.Load() >= n+2 {
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
		return assert.Fail(t, "no load")
	}
	exec := func(sql string) bool {
		_, err := c.Pool().Exec(ctx, sql)
		return assert.NoError(t, err, sql)
	}

	assert.Len(t, s.List(), 45, "the seed rows, loaded by Start")
	if !check("seed", 43) {
		return
	}
	assert.Equal(t, []int64{43, 99, 35}, providerIDs(s.Group("gas", "inquiry")))
	other := s.Group("telmidterm", "order")

	steps := []struct {
		name, sql string
		order     []int64
		pick      int64
	}{
		{"priority", `UPDATE providers SET priority = 1 WHERE id = 35`, []int64{35, 43, 99}, 43},
		{"status", `UPDATE providers SET status = 'enable' WHERE id = 35`, []int64{35, 43, 99}, 35},
		{"insert", `INSERT INTO providers (id, name, bill_type, action, priority, status) VALUES (9100, 'new', 'gas', 'inquiry', 0, 'enable')`, []int64{9100, 35, 43, 99}, 9100},
		{"delete", `DELETE FROM providers WHERE id = 9100`, []int64{35, 43, 99}, 35},
		{"disable all", `UPDATE providers SET status = 'disable' WHERE bill_type = 'gas' AND action = 'inquiry'`, []int64{35, 43, 99}, 0},
	}
	for _, st := range steps {
		if !exec(st.sql) || !waitLoad() || !check(st.name, st.pick) {
			return
		}
		assert.Equal(t, st.order, providerIDs(s.Group("gas", "inquiry")), st.name)
		assert.Equal(t, other, s.Group("telmidterm", "order"), "another group does not change")
	}

	before := s.List()
	if !waitLoad() {
		return
	}
	assert.Equal(t, before, s.List(), "a load with no change keeps the order")

	// A runtime move lasts until the next load (the database wins).
	assert.NoError(t, s.MoveDown(35))
	assert.Equal(t, []int64{43, 35, 99}, providerIDs(s.Group("gas", "inquiry")))
	if !waitLoad() {
		return
	}
	assert.Equal(t, []int64{35, 43, 99}, providerIDs(s.Group("gas", "inquiry")))

	// A failed load (the table is gone) keeps the last list and writes an error line.
	if !exec(`ALTER TABLE providers RENAME TO providers_gone`) || !waitErr(t, log) {
		return
	}
	assert.Equal(t, before, s.List())
	_ = exec(`ALTER TABLE providers_gone RENAME TO providers`)
}

// waitErr waits until the service writes an error line.
func waitErr(t *testing.T, l *liveLog) bool {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		l.mu.Lock()
		n := l.errors
		l.mu.Unlock()
		if n > 0 {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return assert.Fail(t, "no error line")
}

func providerIDs(ps []provider.Provider) []int64 {
	out := make([]int64, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

// TestLiveProvidersServiceFirstLoadFails: with no providers table, Start does not fail, the list is
// empty and PickProvider gives ErrNoProvider; when the table comes, the routine loads it.
func TestLiveProvidersServiceFirstLoadFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	if _, err := c.Pool().Exec(ctx, `ALTER TABLE providers RENAME TO providers_later`); !assert.NoError(t, err) {
		return
	}
	log := &liveLog{}
	s, err := provider.NewProvidersService(postgres.NewProviderRepository(c), provider.Options{
		Interval: func() time.Duration { return time.Second }, Logger: log})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	assert.NoError(t, s.Start(ctx))
	defer func() { assert.NoError(t, s.Stop(context.Background())) }()
	assert.Empty(t, s.List())
	_, err = s.PickProvider("gas", "inquiry")
	assert.ErrorIs(t, err, provider.ErrNoProvider)
	log.mu.Lock()
	assert.Equal(t, 1, log.errors)
	log.mu.Unlock()

	if _, err := c.Pool().Exec(ctx, `ALTER TABLE providers_later RENAME TO providers`); !assert.NoError(t, err) {
		return
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && len(s.List()) != 45 {
		time.Sleep(20 * time.Millisecond)
	}
	assert.Len(t, s.List(), 45)
}

// TestLiveStopDuringQueryIsNotAFailure: a Stop while the reload routine waits in a real pgx query
// is a shutdown, not a failed load. In the throw-away schema, providers becomes a view whose rows
// call pg_sleep (the delay is read from the table slow_load), so the query blocks; the service
// loads once at Start (no delay), then a load runs with a 30 s delay and Stop is called while
// Postgres sleeps. Pass: no "providers: load failed" error line, no result="error" count, and Stop
// returns within 5 s (far before the 30 s sleep ends).
func TestLiveStopDuringQueryIsNotAFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	for _, sql := range []string{
		`ALTER TABLE providers RENAME TO providers_base`,
		`CREATE TABLE slow_load (seconds double precision NOT NULL)`,
		`INSERT INTO slow_load VALUES (0)`,
		`CREATE FUNCTION slow_load_wait() RETURNS boolean LANGUAGE plpgsql VOLATILE AS $$
		BEGIN
			PERFORM pg_sleep((SELECT seconds FROM slow_load LIMIT 1));
			RETURN true;
		END $$`,
		`CREATE VIEW providers AS SELECT * FROM providers_base WHERE slow_load_wait()`,
	} {
		if _, err := c.Pool().Exec(ctx, sql); !assert.NoError(t, err, sql) {
			return
		}
	}
	var oks, errs atomic.Int64
	log := &liveLog{}
	s, err := provider.NewProvidersService(postgres.NewProviderRepository(c), provider.Options{
		Interval: func() time.Duration { return time.Second },
		Logger:   log,
		OnLoad: func(result string, _ time.Duration) {
			if result == provider.LoadError {
				errs.Add(1)
				return
			}
			oks.Add(1)
		},
	})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	if !assert.NoError(t, s.Start(ctx)) {
		return
	}
	stopped := false
	defer func() {
		if !stopped {
			assert.NoError(t, s.Stop(context.Background()))
		}
	}()
	if !assert.Len(t, s.List(), 45, "the first load (no delay)") || !assert.Equal(t, int64(1), oks.Load()) {
		return
	}

	if _, err := c.Pool().Exec(ctx, `UPDATE slow_load SET seconds = 30`); !assert.NoError(t, err) {
		return
	}
	// Wait until the reload routine's query sleeps in Postgres.
	sleeping := false
	deadline := time.Now().Add(10 * time.Second)
	for !sleeping && time.Now().Before(deadline) {
		var n int
		if err := c.Pool().QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE wait_event = 'PgSleep' AND query LIKE '%FROM providers%' AND pid <> pg_backend_pid()`).Scan(&n); !assert.NoError(t, err) {
			return
		}
		sleeping = n > 0
		if !sleeping {
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !assert.True(t, sleeping, "a load query sleeps in Postgres") {
		return
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	began := time.Now()
	stopped = true
	assert.NoError(t, s.Stop(stopCtx), "Stop returns before its 5 s bound")
	assert.Less(t, time.Since(began), 5*time.Second)
	log.mu.Lock()
	assert.Zero(t, log.errors, "no providers: load failed line")
	log.mu.Unlock()
	assert.Zero(t, errs.Load(), "no result=error count")
	assert.Len(t, s.List(), 45, "the list stays")
}
