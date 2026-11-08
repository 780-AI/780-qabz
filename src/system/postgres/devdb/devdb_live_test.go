package devdb_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/billtype"
	"qabz/system/postgres"
	"qabz/system/postgres/devdb"
	"qabz/system/postgres/postgrestest"
)

// liveClient connects to a new empty throw-away schema (QABZ_TEST_POSTGRES_DSN), so the real tables
// of the database are never touched. The schema is dropped at the end of the test.
func liveClient(t *testing.T, ctx context.Context) *postgres.Client {
	t.Helper()
	dsn := os.Getenv("QABZ_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("QABZ_TEST_POSTGRES_DSN is not set")
	}
	c, err := postgres.New(ctx, postgrestest.EmptySchema(t, ctx, dsn))
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	t.Cleanup(c.Close)
	return c
}

func count(t *testing.T, ctx context.Context, c *postgres.Client, sql string) int {
	t.Helper()
	var n int
	if !assert.NoError(t, c.Pool().QueryRow(ctx, sql).Scan(&n), sql) {
		t.FailNow()
	}
	return n
}

// TestLiveSeedTwiceThenUnseed: seed makes every bill table and the seed rows; a second seed changes
// nothing (also a row that was edited in between); unseed deletes only the seed rows and keeps the
// tables; a second unseed deletes nothing.
func TestLiveSeedTwiceThenUnseed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := liveClient(t, ctx)
	tables := `SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema()`
	seeded := `SELECT (SELECT count(*) FROM services) + (SELECT count(*) FROM providers)`

	n, err := devdb.Seed(ctx, c.Pool())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, devdb.Counts{Services: 5, Providers: 45}, n)
	assert.Equal(t, 10, count(t, ctx, c, tables), "every bill table")
	assert.Equal(t, 50, count(t, ctx, c, seeded))

	// The rows as given: GET /v1/services reads them through this repository (all have order 0, so
	// id breaks the tie). updated_at keeps the instant of the given time with its offset.
	got, err := postgres.NewServiceRepository(c).ListServices(ctx, "fa-IR")
	assert.NoError(t, err)
	assert.Equal(t, []billtype.Service{
		{Name: "برق", Type: "electricity", IsActive: true, Inquiry: true},
		{Name: "گاز", Type: "gas", IsActive: true, Inquiry: true},
		{Name: "تلفن همراه", Type: "mobile", IsActive: true, Inquiry: true},
		{Name: "تلفن", Type: "tel", IsActive: true, Inquiry: true},
		{Name: "آب", Type: "water", IsActive: true, Inquiry: true},
	}, got)
	var updated time.Time
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT updated_at FROM services WHERE id = 1`).Scan(&updated))
	assert.True(t, time.Date(2022, 2, 5, 7, 23, 35, 586761000, time.UTC).Equal(updated), updated)
	// A provider row in full; the columns that the seed does not give have bill's defaults.
	var name, subject, billType, action, typ, status string
	var priority, timeout, step int
	var useNats bool
	var fee *int64
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT name, subject, bill_type, action, type, priority, timeout, status,
		use_nats, inquiry_step, provider_fee, updated_at FROM providers WHERE id = 35`).
		Scan(&name, &subject, &billType, &action, &typ, &priority, &timeout, &status, &useNats, &step, &fee, &updated))
	assert.Equal(t, []any{"qabzino", "qabzino-gas-inquiry", "gas", "inquiry", "retryable", 10, 10000, "disable", false, 1},
		[]any{name, subject, billType, action, typ, priority, timeout, status, useNats, step})
	assert.Nil(t, fee)
	assert.True(t, time.Date(2026, 10, 26, 15, 0, 45, 59563000, time.UTC).Equal(updated), updated)
	assert.Equal(t, 45, count(t, ctx, c, `SELECT count(*) FROM providers WHERE inquiry_step = 1 AND provider_fee IS NULL`))

	// The sequences are past the largest seeded id.
	assert.Equal(t, 6, count(t, ctx, c, `INSERT INTO services (type) VALUES ('extra') RETURNING id`))
	assert.Equal(t, 124326, count(t, ctx, c, `INSERT INTO providers (name) VALUES ('extra') RETURNING id`))

	// A second seed inserts nothing and does not overwrite an edited seed row.
	_, err = c.Pool().Exec(ctx, `UPDATE services SET name = 'edited' WHERE id = 1`)
	assert.NoError(t, err)
	n, err = devdb.Seed(ctx, c.Pool())
	assert.NoError(t, err)
	assert.Equal(t, devdb.Counts{}, n)
	assert.Equal(t, 52, count(t, ctx, c, seeded), "50 seed rows + 2 extra rows")
	assert.Equal(t, 1, count(t, ctx, c, `SELECT count(*) FROM services WHERE id = 1 AND name = 'edited'`))
	assert.Equal(t, 7, count(t, ctx, c, `INSERT INTO services (type) VALUES ('extra2') RETURNING id`),
		"the second seed does not move the sequence back")

	// Unseed deletes the 50 seed rows only; the tables and the other rows stay.
	n, err = devdb.Unseed(ctx, c.Pool())
	assert.NoError(t, err)
	assert.Equal(t, devdb.Counts{Services: 5, Providers: 45}, n)
	assert.Equal(t, 3, count(t, ctx, c, seeded), "only the extra rows stay")
	assert.Equal(t, 10, count(t, ctx, c, tables), "the tables stay")
	n, err = devdb.Unseed(ctx, c.Pool())
	assert.NoError(t, err)
	assert.Equal(t, devdb.Counts{}, n)
}

// TestLiveSeedIsAtomic: when a step fails, nothing is changed. Here a services table made before
// the command has no lang column, so the services insert fails after the CREATE statements ran.
func TestLiveSeedIsAtomic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := liveClient(t, ctx)
	_, err := c.Pool().Exec(ctx, `CREATE TABLE services (id integer PRIMARY KEY, type varchar(32))`)
	if !assert.NoError(t, err) {
		return
	}
	_, err = devdb.Seed(ctx, c.Pool())
	assert.ErrorContains(t, err, "seed services")
	assert.Equal(t, 1, count(t, ctx, c, `SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema()`),
		"the rollback removed the new tables")
	assert.Equal(t, 0, count(t, ctx, c, `SELECT count(*) FROM services`))
}
