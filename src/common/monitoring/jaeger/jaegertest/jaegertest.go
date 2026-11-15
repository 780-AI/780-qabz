// Package jaegertest is the test helper of the span-tree tests (conventions § 17): an in-memory
// recorder as the global tracer provider, and a check that no span has a forbidden attribute.
package jaegertest

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Install sets a global tracer provider that records every span in memory and the W3C trace-context
// propagator (as jaeger.New does). t.Cleanup puts back the old provider and propagator. A test that
// calls Install must not call t.Parallel: the globals are shared.
func Install(t testing.TB) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background()) // t.Context() is already canceled in Cleanup
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
	return sr
}

// allowedKeys are the attribute keys that Qabz code sets (values from a small fixed set, or the
// internal order ID).
var allowedKeys = map[string]bool{
	"bill_type": true, "provider": true, "operator": true, "result": true, "source": true, "cron": true,
	"order.id": true, "exception.type": true,
	// the keys of the instrumentation (otelgrpc, otelhttp, redisotel, the pgx tracer, kafka.Publish)
	"db.system": true, "db.system.name": true, "db.operation.name": true, "db.redis.database_index": true,
	"db.connection_string": true, "db.redis.num_cmd": true, // redisotel: "tcp://<host:port of the config>", a count
	"http.request.method": true, "http.response.status_code": true, "url.full": true, "error.type": true,
}

// allowedPrefixes are key namespaces of the instrumentation (no body, no argument in them).
var allowedPrefixes = []string{"rpc.", "server.", "network.", "messaging.", "pool."}

// libraryKeys are allowed only on a span of an instrumentation library (redisotel, otelhttp record
// their own errors with the text of a Redis or transport error), never on a span of Qabz code
// (instrumentation scope qabz/...), which uses jaeger.Fail (task 047 decision D10).
var libraryKeys = map[string]bool{"exception.message": true}

func keyAllowed(k, scope string) bool {
	if libraryKeys[k] {
		return !strings.HasPrefix(scope, "qabz/")
	}
	if allowedKeys[k] {
		return true
	}
	for _, p := range allowedPrefixes {
		if strings.HasPrefix(k, p) {
			return true
		}
	}
	return false
}

// TB is the part of testing.TB that AssertClean uses.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
}

// AssertClean checks every span: each attribute key (also of the events) is in the allow-list, and
// no span name, attribute value, event attribute value or status description has any of inputs (the
// phone, the bill ID, the pay ID, the user ID, the amount, the token of the test). An empty input is
// ignored. It returns false when a check fails.
func AssertClean(t TB, spans []sdktrace.ReadOnlySpan, inputs ...string) bool {
	t.Helper()
	ok := true
	hasInput := func(where, text string) {
		for _, in := range inputs {
			if in != "" && strings.Contains(text, in) {
				ok = assert.Failf(t, "span has a forbidden value", "%s has %q", where, in) && ok
			}
		}
	}
	checkAttrs := func(span, scope string, attrs []attribute.KeyValue) {
		for _, a := range attrs {
			k := string(a.Key)
			if !keyAllowed(k, scope) {
				ok = assert.Failf(t, "span has a forbidden attribute", "span %q: attribute %s is not allowed", span, k) && ok
			}
			hasInput("span "+span+" attribute "+k, a.Value.Emit())
		}
	}
	for _, s := range spans {
		if s == nil {
			continue
		}
		name, scope := s.Name(), s.InstrumentationScope().Name
		hasInput("span name", name)
		hasInput("span "+name+" status", s.Status().Description)
		checkAttrs(name, scope, s.Attributes())
		for _, e := range s.Events() {
			hasInput("span "+name+" event name", e.Name)
			checkAttrs(name, scope, e.Attributes)
		}
	}
	return ok
}

// ByName returns the ended spans of sr with the name, in the order they ended.
func ByName(sr *tracetest.SpanRecorder, name string) []sdktrace.ReadOnlySpan {
	if sr == nil {
		return nil
	}
	var out []sdktrace.ReadOnlySpan
	for _, s := range sr.Ended() {
		if s != nil && s.Name() == name {
			out = append(out, s)
		}
	}
	return out
}
