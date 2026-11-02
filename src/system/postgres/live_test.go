package postgres

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/config"

	"qabz/migrations"
)

// These tests need a real Postgres. Set QABZ_TEST_POSTGRES_DSN, for example
// postgres://qabz:pw@127.0.0.1:55432/qabz. The tests use a throw-away schema state:
// they drop the tables of the migrations at the start and at the end.
func liveDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("QABZ_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("QABZ_TEST_POSTGRES_DSN is not set")
	}
	return dsn
}

// reset drops the tables the migrations create, so each test starts from an empty database.
// (There are no down migrations.)
func reset(t *testing.T, dsn string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := New(ctx, dsn)
	assert.NoError(t, err)
	defer c.Close()
	_, err = c.Pool().Exec(ctx, `DROP TABLE IF EXISTS app_hot_configs, qabz_schema_migrations`)
	assert.NoError(t, err)
}

func TestLiveConcurrentUp(t *testing.T) {
	dsn := liveDSN(t)
	m := NewMigrator(dsn, migrations.FS)
	reset(t, dsn)
	t.Cleanup(func() { reset(t, dsn) })

	const n = 8
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			errs[i] = NewMigrator(dsn, migrations.FS).Up(ctx)
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
	m := NewMigrator(dsn, migrations.FS)
	reset(t, dsn)
	ctx := context.Background()
	assert.NoError(t, m.Up(ctx))
	t.Cleanup(func() { reset(t, dsn) })

	c, err := New(ctx, dsn)
	assert.NoError(t, err)
	defer c.Close()
	repo := NewConfigRepository(c)

	rows, err := repo.Load(ctx)
	assert.NoError(t, err)
	assert.Empty(t, rows)

	_, err = c.Pool().Exec(ctx, `INSERT INTO app_hot_configs (key, cnf_type, value) VALUES ('log_level','string','debug')`)
	assert.NoError(t, err)
	rows, err = repo.Load(ctx)
	assert.NoError(t, err)
	assert.Len(t, rows, 1)
	assert.Equal(t, "log_level", rows[0].Key)
	assert.Equal(t, "string", rows[0].Type)
	assert.Equal(t, "debug", rows[0].Value)

	_, err = c.Pool().Exec(ctx, `INSERT INTO app_hot_configs (key, cnf_type, value) VALUES ('log_level','string','x')`)
	assert.Error(t, err, "key is unique")
}

func TestMigrateUpTimeout(t *testing.T) {
	m := NewMigrator("postgres://u:p@127.0.0.1:1/db", migrations.FS)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := m.Up(ctx)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), ":p@")
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
	m := NewMigrator(dsn, migrations.FS)
	reset(t, dsn)
	ctx := context.Background()
	assert.NoError(t, m.Up(ctx))
	t.Cleanup(func() { reset(t, dsn) })

	c, err := New(ctx, dsn)
	assert.NoError(t, err)
	defer c.Close()

	holder := config.NewHolder(liveCfg{LogLevel: "info", Workers: 1, Frozen: "keep"})
	rel, err := config.NewReloader[liveCfg](NewConfigRepository(c), holder, 100*time.Millisecond, testLog{})
	assert.NoError(t, err)
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
