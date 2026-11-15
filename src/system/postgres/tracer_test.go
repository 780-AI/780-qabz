package postgres

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"qabz/common/monitoring/jaeger/jaegertest"
)

func TestOperation(t *testing.T) {
	for sql, want := range map[string]string{
		"SELECT 1":                             "SELECT",
		"  select id FROM orders":              "SELECT",
		"\n\tINSERT INTO orders (id) VALUES":   "INSERT",
		"update orders set x = 1":              "UPDATE",
		"DELETE FROM bills":                    "DELETE",
		"WITH x AS (SELECT 1) SELECT * FROM x": "WITH",
		"begin":                                "BEGIN",
		"commit;":                              "COMMIT",
		"rollback":                             "ROLLBACK",
		"SELECT(1)":                            "SELECT",
		"":                                     "QUERY",
		"   ":                                  "QUERY",
		"989121234567 is not SQL":              "QUERY",
		"truncate bills":                       "QUERY",
	} {
		assert.Equal(t, want, operation(sql), sql)
	}
}

// TestTracerSpans: one client span for each query, a child of the span in ctx, with the operation
// only; a failed query has the error status and only the error type.
func TestTracerSpans(t *testing.T) {
	sr := jaegertest.Install(t)
	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	var tr tracer
	qctx := tr.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT * FROM orders WHERE phone = $1", Args: []any{"989121234567"}})
	tr.TraceQueryEnd(qctx, nil, pgx.TraceQueryEndData{})
	qctx = tr.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "INSERT INTO orders VALUES ($1)", Args: []any{"989121234567"}})
	tr.TraceQueryEnd(qctx, nil, pgx.TraceQueryEndData{Err: assert.AnError})
	tr.TraceQueryEnd(context.Background(), nil, pgx.TraceQueryEndData{}) // no span in ctx: no panic
	parent.End()

	spans := jaegertest.ByName(sr, "SELECT")
	failed := jaegertest.ByName(sr, "INSERT")
	if !assert.Len(t, spans, 1) || !assert.Len(t, failed, 1) {
		return
	}
	s := spans[0]
	assert.Equal(t, trace.SpanKindClient, s.SpanKind())
	assert.Equal(t, parent.SpanContext().SpanID(), s.Parent().SpanID())
	assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("db.system.name", "postgresql"), attribute.String("db.operation.name", "SELECT")}, s.Attributes())
	assert.Equal(t, codes.Unset, s.Status().Code)
	assert.Equal(t, codes.Error, failed[0].Status().Code)
	assert.Equal(t, "error", failed[0].Status().Description)
	assert.Len(t, failed[0].Events(), 1)
	jaegertest.AssertClean(t, sr.Ended(), "989121234567", "orders", assert.AnError.Error())
}
