package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"

	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
)

// TestLiveTracer: a real query through New gives its span, also in a transaction; a failed query
// gives the error status.
func TestLiveTracer(t *testing.T) {
	dsn := postgrestest.RunDSN(t)
	sr := jaegertest.Install(t)
	ctx := context.Background()
	c, err := postgres.New(ctx, dsn)
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	defer c.Close()
	ctx, parent := otel.Tracer("test").Start(ctx, "parent")
	var n int
	assert.NoError(t, c.Pool().QueryRow(ctx, "SELECT $1::int", 7).Scan(&n))
	assert.Equal(t, 7, n)
	_, err = c.Pool().Exec(ctx, "SELECT * FROM no_table_989121234567")
	assert.Error(t, err)
	tx, err := c.Pool().Begin(ctx)
	if assert.NoError(t, err) {
		assert.NoError(t, tx.Commit(ctx))
	}
	parent.End()

	sel := jaegertest.ByName(sr, "SELECT")
	if !assert.Len(t, sel, 2) {
		return
	}
	assert.Equal(t, codes.Unset, sel[0].Status().Code)
	assert.Equal(t, parent.SpanContext().SpanID(), sel[0].Parent().SpanID())
	assert.Equal(t, codes.Error, sel[1].Status().Code)
	assert.Len(t, jaegertest.ByName(sr, "BEGIN"), 1)
	assert.Len(t, jaegertest.ByName(sr, "COMMIT"), 1)
	jaegertest.AssertClean(t, sr.Ended(), "989121234567", "no_table")
}
