// Package postgrestest has Postgres helpers for tests only. Production code never imports it.
package postgrestest

import (
	"context"
	"crypto/rand"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"qabz/system/postgres"
)

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

// newSchema creates a throw-away schema, runs ddl in it and returns dsn with search_path set to it.
func newSchema(t testing.TB, ctx context.Context, dsn, ddl string) string {
	t.Helper()
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
