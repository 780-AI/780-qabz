// Package jaeger sets up tracing: OpenTelemetry with the OTLP gRPC exporter (Jaeger accepts OTLP).
// It also has Fail, the one way to mark a failed span (conventions § 17).
package jaeger

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Tracer owns the tracer provider.
type Tracer struct{ tp *sdktrace.TracerProvider }

// New creates the provider and sets it as the global one. The exporter connects lazily. It also sets
// the global propagator to the W3C trace context only (no baggage: baggage can carry free input), so
// the servers read traceparent and the outbound clients send it. Without New (logs.otlp_endpoint
// empty) both globals stay the no-op ones: no span, no header, no connection.
func New(ctx context.Context, endpoint, service string) (*Tracer, error) {
	exp, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return &Tracer{tp: tp}, nil
}

// Shutdown flushes spans and stops the provider.
func (t *Tracer) Shutdown(ctx context.Context) error { return t.tp.Shutdown(ctx) }

// Fail marks span as failed: the status Error with result (the result that the feature metric
// counts, conventions § 12) and, when err is not nil, an event "exception" with only the Go type of
// err. The error text is not put in the span: it can have a phone or a bill ID in it (task 047
// decision D10); the log line has it. Use Fail, never span.RecordError.
func Fail(span trace.Span, result string, err error) {
	if span == nil {
		return
	}
	span.SetStatus(codes.Error, result)
	if err != nil {
		span.AddEvent("exception", trace.WithAttributes(attribute.String("exception.type", fmt.Sprintf("%T", err))))
	}
}
