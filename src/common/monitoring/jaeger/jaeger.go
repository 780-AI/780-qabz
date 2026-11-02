// Package jaeger sets up tracing: OpenTelemetry with the OTLP gRPC exporter (Jaeger accepts OTLP).
package jaeger

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Tracer owns the tracer provider.
type Tracer struct{ tp *sdktrace.TracerProvider }

// New creates the provider and sets it as the global one. The exporter connects lazily.
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
	return &Tracer{tp: tp}, nil
}

// Shutdown flushes spans and stops the provider.
func (t *Tracer) Shutdown(ctx context.Context) error { return t.tp.Shutdown(ctx) }
