package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"qabz/common/monitoring/jaeger"
)

// operations are the span names of the queries: the first word of the SQL, in upper case. Any other
// first word gives "QUERY", so a span name never has free text (conventions § 17).
var operations = map[string]bool{
	"SELECT": true, "INSERT": true, "UPDATE": true, "DELETE": true, "WITH": true,
	"BEGIN": true, "COMMIT": true, "ROLLBACK": true, "CREATE": true, "DROP": true, "ALTER": true,
}

// operation returns the span name of sql. (Ping and the health ping of the pool go to pgconn and
// do not reach the tracer, so they make no span.)
func operation(sql string) string {
	sql = strings.TrimSpace(sql)
	end := strings.IndexFunc(sql, func(r rune) bool { return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') })
	if end < 0 {
		end = len(sql)
	}
	word := strings.ToUpper(sql[:end])
	if operations[word] {
		return word
	}
	return "QUERY"
}

// tracer is the pgx query tracer of the pool (task 047 decision D7): one client span for each query,
// a child of the span in ctx. The span has the operation only: no SQL text and no argument (the
// arguments are phones, bill IDs, amounts).
type tracer struct{}

type spanKey struct{}

// TraceQueryStart starts the span of the query.
func (tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	op := operation(data.SQL)
	ctx, span := otel.Tracer("qabz/system/postgres").Start(ctx, op, trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system.name", "postgresql"), attribute.String("db.operation.name", op)))
	return context.WithValue(ctx, spanKey{}, span)
}

// TraceQueryEnd ends the span; a failed query gets the error status (jaeger.Fail, no error text).
func (tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, ok := ctx.Value(spanKey{}).(trace.Span)
	if !ok || span == nil {
		return
	}
	if data.Err != nil {
		jaeger.Fail(span, "error", data.Err)
	}
	span.End()
}
