// Package postgrestest has Postgres helpers for tests only. Production code never imports it.
//
// Every live test works in the run database of `make test` (task 020b): one database
// qabz_test_<random hex> for the whole `go test ./...`, created and dropped by the target. A test gets
// its DSN only from RunDSN; the helpers refuse every DSN whose database name does not start with
// RunDBPrefix, so no test can work in the development database.
package postgrestest

import (
	"context"
	"crypto/rand"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"qabz/system/postgres"
	"qabz/system/postgres/devdb"
)

// RunDBPrefix starts the name of every test run database. The development database never has it.
const RunDBPrefix = "qabz_test_"

// RunDSNEnv names the variable with the DSN of the run database. Only `make test` sets it.
const RunDSNEnv = "QABZ_TEST_RUN_DSN"

// RunDBName returns the name of the database that the driver connects to for dsn, and an error when
// that name does not start with RunDBPrefix or when dsn is not a postgres:// URL (the helpers add
// search_path to the URL). The name is the one pgconn.ParseConfig gives: the URL path, a dbname= or
// database= parameter that overrides it, or PGDATABASE when the DSN names no database. The error has
// the name but never the DSN (the DSN has the password).
func RunDBName(dsn string) (string, error) {
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil || cfg == nil {
		return "", errors.New("the run DSN is not a valid Postgres DSN")
	}
	name := cfg.Database
	if !strings.HasPrefix(name, RunDBPrefix) || len(name) == len(RunDBPrefix) {
		return "", errors.New("database " + strconv.Quote(name) + " is not a test run database (name must start with " + RunDBPrefix + ")")
	}
	u, err := url.Parse(dsn)
	if err != nil || u == nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return "", errors.New("the run DSN is not a postgres:// URL")
	}
	return name, nil
}

// RunDSN returns the DSN of the run database (QABZ_TEST_RUN_DSN). Without it the test is skipped
// (run the live tests with `make test`). It stops the test (FailNow) when the database name does not
// start with RunDBPrefix. For tests only.
func RunDSN(t testing.TB) string {
	t.Helper()
	dsn := os.Getenv(RunDSNEnv)
	if dsn == "" {
		t.Skip(RunDSNEnv + " is not set (run the live tests with make test)")
	}
	if _, err := RunDBName(dsn); !assert.NoError(t, err) {
		t.FailNow()
	}
	return dsn
}

// servicesTableSQL is the services table of the bill database (docs/initial-discovery/database.html).
// Tests make it in a throw-away schema; qabz has no migration for it.
const servicesTableSQL = `CREATE TABLE services (
	id integer PRIMARY KEY,
	name varchar(64),
	type varchar(32) NOT NULL,
	tag varchar(32),
	"order" smallint DEFAULT 0,
	is_active boolean DEFAULT true,
	lang varchar(32) DEFAULT 'fa-IR',
	inquiry boolean DEFAULT true,
	icon varchar,
	updated_at timestamp DEFAULT now()
)`

// billsTableSQL is the bills table (My Bills) of the bill database: the columns of
// docs/initial-discovery/database.html, the defaults and the UNIQUE (user_id, type, bill_id) key of
// the bill entity (domain/entity/bill.go, go-pg tags). Tests make it in a throw-away schema.
const billsTableSQL = `CREATE TABLE bills (
	id uuid PRIMARY KEY,
	user_id uuid,
	name varchar(128),
	type varchar(32),
	bill_id varchar(32),
	payment_deadline varchar(32),
	status varchar(32) DEFAULT 'enable',
	last_order_date varchar,
	data varchar(100),
	json_data jsonb,
	created_at timestamp DEFAULT now(),
	UNIQUE (user_id, type, bill_id)
)`

// ServicesSchema creates a new schema with an empty services table and returns dsn with
// search_path set to that schema. The schema is dropped at the end of the test. It stops the test
// if a step fails. For tests only.
func ServicesSchema(t testing.TB, ctx context.Context, dsn string) string {
	t.Helper()
	return newSchema(t, ctx, dsn, servicesTableSQL)
}

// BillsSchema is ServicesSchema with an empty bills table. For tests only.
func BillsSchema(t testing.TB, ctx context.Context, dsn string) string {
	t.Helper()
	return newSchema(t, ctx, dsn, billsTableSQL)
}

// EmptySchema is ServicesSchema with no table: for code that creates its own tables (the devdb
// command). For tests only.
func EmptySchema(t testing.TB, ctx context.Context, dsn string) string {
	t.Helper()
	return newSchema(t, ctx, dsn, "")
}

// SeededSchema is EmptySchema with every bill table as the devdb command makes it (one definition
// of the bill tables, task 015) and the devdb seed rows (45 providers, 5 services). For the
// providers tests (task 020). For tests only.
func SeededSchema(t testing.TB, ctx context.Context, dsn string) string {
	t.Helper()
	schemaDSN := newSchema(t, ctx, dsn, "")
	c, err := postgres.New(ctx, schemaDSN)
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	defer c.Close()
	if _, err := devdb.Seed(ctx, c.Pool()); !assert.NoError(t, err) {
		t.FailNow()
	}
	return schemaDSN
}

// newSchema creates a throw-away schema, runs ddl in it (no ddl: an empty schema) and returns dsn
// with search_path set to it.
// It stops the test before it connects when dsn is not a run database DSN (RunDBName).
func newSchema(t testing.TB, ctx context.Context, dsn, ddl string) string {
	t.Helper()
	if _, err := RunDBName(dsn); !assert.NoError(t, err) {
		t.FailNow()
	}
	// rand.Text has only A-Z and 2-7; in lower case the name needs no quoting.
	suffix := rand.Text()
	if !assert.GreaterOrEqual(t, len(suffix), 10, "rand.Text is too short") {
		t.FailNow()
	}
	schema := "qabz_test_" + strings.ToLower(suffix[:10])
	admin, err := postgres.New(ctx, dsn)
	if !assert.NoError(t, err) || !assert.NotNil(t, admin) {
		t.FailNow()
	}
	defer admin.Close()
	if _, err := admin.Pool().Exec(ctx, `CREATE SCHEMA `+schema); !assert.NoError(t, err) {
		t.FailNow()
	}
	t.Cleanup(func() {
		c, err := postgres.New(context.Background(), dsn)
		if !assert.NoError(t, err) || !assert.NotNil(t, c) {
			return
		}
		defer c.Close()
		_, err = c.Pool().Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		assert.NoError(t, err)
	})
	u, err := url.Parse(dsn)
	if !assert.NoError(t, err) || !assert.NotNil(t, u) {
		t.FailNow()
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	schemaDSN := u.String()

	if ddl == "" {
		return schemaDSN
	}
	c, err := postgres.New(ctx, schemaDSN)
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	defer c.Close()
	if _, err := c.Pool().Exec(ctx, ddl); !assert.NoError(t, err) {
		t.FailNow()
	}
	return schemaDSN
}
