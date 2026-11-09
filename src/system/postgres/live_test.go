package postgres_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/config"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"

	"qabz/migrations"
)

// liveDSN returns the DSN of a new empty throw-away schema in the run database of `make test`
// (QABZ_TEST_RUN_DSN), so each test starts with no migration tables (there are no down migrations)
// and never touches the development database. The schema is dropped at the end of the test. It
// skips the test without the run database.
func liveDSN(t *testing.T) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return postgrestest.EmptySchema(t, ctx, postgrestest.RunDSN(t))
}

// newClient opens a pool and stops the test if it fails, so no test calls a method on a nil *Client.
func newClient(t *testing.T, ctx context.Context, dsn string) *postgres.Client {
	t.Helper()
	c, err := postgres.New(ctx, dsn)
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	return c
}

func TestLiveConcurrentUp(t *testing.T) {
	dsn := liveDSN(t)
	m := postgres.NewMigrator(dsn, migrations.FS)

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			errs[i] = postgres.NewMigrator(dsn, migrations.FS).Up(ctx)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		assert.NoError(t, err, "caller %d", i)
	}
	v, dirty, err := m.Version()
	assert.NoError(t, err)
	assert.Equal(t, uint(1), v)
	assert.False(t, dirty)
}

func TestLiveRepositoryLoad(t *testing.T) {
	dsn := liveDSN(t)
	m := postgres.NewMigrator(dsn, migrations.FS)
	ctx := context.Background()
	assert.NoError(t, m.Up(ctx))

	c := newClient(t, ctx, dsn)
	defer c.Close()
	repo := postgres.NewConfigRepository(c)

	rows, err := repo.Load(ctx)
	assert.NoError(t, err)
	assert.Empty(t, rows)

	_, err = c.Pool().Exec(ctx, `INSERT INTO app_hot_configs (key, cnf_type, value) VALUES ('log_level','string','debug')`)
	assert.NoError(t, err)
	rows, err = repo.Load(ctx)
	assert.NoError(t, err)
	if assert.Len(t, rows, 1) {
		assert.Equal(t, "log_level", rows[0].Key)
		assert.Equal(t, "string", rows[0].Type)
		assert.Equal(t, "debug", rows[0].Value)
	}

	_, err = c.Pool().Exec(ctx, `INSERT INTO app_hot_configs (key, cnf_type, value) VALUES ('log_level','string','x')`)
	assert.Error(t, err, "key is unique")
}

func TestMigrateUpTimeout(t *testing.T) {
	m := postgres.NewMigrator("postgres://u:p@127.0.0.1:1/db", migrations.FS)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := m.Up(ctx)
	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), ":p@")
	}
}

type liveCfg struct {
	LogLevel string `hot_reload:"true"`
	Workers  int    `hot_reload:"true"`
	Frozen   string
}

type testLog struct{}

func (testLog) Info(string, ...any)  {}
func (testLog) Warn(string, ...any)  {}
func (testLog) Error(string, ...any) {}

// End to end: a row changed in Postgres reaches the config field within the interval.
func TestLiveHotReloadEndToEnd(t *testing.T) {
	dsn := liveDSN(t)
	m := postgres.NewMigrator(dsn, migrations.FS)
	ctx := context.Background()
	assert.NoError(t, m.Up(ctx))

	c := newClient(t, ctx, dsn)
	defer c.Close()

	holder := config.NewHolder(liveCfg{LogLevel: "info", Workers: 1, Frozen: "keep"})
	rel, err := config.NewReloader[liveCfg](postgres.NewConfigRepository(c), holder, 100*time.Millisecond, testLog{})
	if !assert.NoError(t, err) || !assert.NotNil(t, rel) {
		return
	}
	assert.NoError(t, rel.Start(ctx))
	defer rel.Stop(ctx)
	assert.Equal(t, "info", holder.Get().LogLevel, "empty table: nothing changes")

	_, err = c.Pool().Exec(ctx, `INSERT INTO app_hot_configs (key, cnf_type, value) VALUES
		('log_level','string','debug'), ('workers','int','5'), ('frozen','string','hacked')`)
	assert.NoError(t, err)
	assert.Eventually(t, func() bool { g := holder.Get(); return g.LogLevel == "debug" && g.Workers == 5 },
		3*time.Second, 20*time.Millisecond)
	assert.Equal(t, "keep", holder.Get().Frozen, "untagged field stays")

	_, err = c.Pool().Exec(ctx, `UPDATE app_hot_configs SET value='9', updated_at=now() WHERE key='workers'`)
	assert.NoError(t, err)
	assert.Eventually(t, func() bool { return holder.Get().Workers == 9 }, 3*time.Second, 20*time.Millisecond)
}
