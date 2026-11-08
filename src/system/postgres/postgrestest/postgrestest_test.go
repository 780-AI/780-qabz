package postgrestest

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/system/postgres"
)

// TestServicesSchema needs a real Postgres (QABZ_TEST_POSTGRES_DSN). It checks that the helper makes
// an empty services table in its own schema, and that the schema is gone after the test.
func TestServicesSchema(t *testing.T) {
	dsn := os.Getenv("QABZ_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("QABZ_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var schema string
	t.Run("create", func(t *testing.T) {
		schemaDSN := ServicesSchema(t, ctx, dsn)
		u, err := url.Parse(schemaDSN)
		if !assert.NoError(t, err) || !assert.NotNil(t, u) {
			return
		}
		schema = u.Query().Get("search_path")
		assert.True(t, strings.HasPrefix(schema, "qabz_test_"), schema)

		c, err := postgres.New(ctx, schemaDSN)
		if !assert.NoError(t, err) || !assert.NotNil(t, c) {
			return
		}
		defer c.Close()
		var current string
		var rows int
		assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT current_schema()`).Scan(&current))
		assert.Equal(t, schema, current)
		assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM services`).Scan(&rows))
		assert.Equal(t, 0, rows)
		_, err = c.Pool().Exec(ctx, `INSERT INTO services (id, type) VALUES (1, 'water')`) // the defaults fill the rest
		assert.NoError(t, err)
	})
	if schema == "" {
		return
	}

	// The cleanup of the sub-test dropped the schema.
	c, err := postgres.New(ctx, dsn)
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	defer c.Close()
	var n int
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM information_schema.schemata WHERE schema_name = $1`, schema).Scan(&n))
	assert.Equal(t, 0, n)
}

// TestBillsSchema needs a real Postgres (QABZ_TEST_POSTGRES_DSN). It checks that the helper makes an
// empty bills table with bill's defaults and its UNIQUE (user_id, type, bill_id) key.
func TestBillsSchema(t *testing.T) {
	dsn := os.Getenv("QABZ_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("QABZ_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := postgres.New(ctx, BillsSchema(t, ctx, dsn))
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	defer c.Close()

	var rows int
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM bills`).Scan(&rows))
	assert.Equal(t, 0, rows)
	const insert = `INSERT INTO bills (id, user_id, type, bill_id) VALUES (gen_random_uuid(), '6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11', 'water', '1234567')`
	_, err = c.Pool().Exec(ctx, insert)
	if !assert.NoError(t, err) {
		return
	}
	var status string
	var hasCreated bool
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT status, created_at IS NOT NULL FROM bills`).Scan(&status, &hasCreated))
	assert.Equal(t, "enable", status)
	assert.True(t, hasCreated)
	_, err = c.Pool().Exec(ctx, insert) // the same user, type and bill_id
	assert.ErrorContains(t, err, "23505")
}
