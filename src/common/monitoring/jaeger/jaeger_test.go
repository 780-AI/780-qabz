package jaeger

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewAndShutdown(t *testing.T) {
	tr, err := New(context.Background(), "127.0.0.1:1", "qabz")
	if !assert.NoError(t, err) || !assert.NotNil(t, tr) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = tr.Shutdown(ctx) // the collector is absent: a flush error is allowed, a hang is not
}

func TestNewSetsTraceContextPropagator(t *testing.T) {
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Cleanup(func() { otel.SetTracerProvider(prevTP); otel.SetTextMapPropagator(prevProp) })
	tr, err := New(context.Background(), "127.0.0.1:1", "qabz")
	if !assert.NoError(t, err) || !assert.NotNil(t, tr) {
		return
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = tr.Shutdown(ctx)
	}()
	assert.Equal(t, []string{"traceparent", "tracestate"}, otel.GetTextMapPropagator().Fields())
}

func TestFail(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	_, s := tp.Tracer("test").Start(context.Background(), "step")
	Fail(s, "provider_down", fmt.Errorf("call for 989121234567: %w", errors.New("down")))
	s.End()
	_, s2 := tp.Tracer("test").Start(context.Background(), "step2")
	Fail(s2, "no_route", nil)
	s2.End()
	Fail(nil, "x", nil) // no panic

	got := sr.Ended()
	if !assert.Len(t, got, 2) {
		return
	}
	assert.Equal(t, codes.Error, got[0].Status().Code)
	assert.Equal(t, "provider_down", got[0].Status().Description)
	if assert.Len(t, got[0].Events(), 1) {
		e := got[0].Events()[0]
		assert.Equal(t, "exception", e.Name)
		assert.Equal(t, []attribute.KeyValue{attribute.String("exception.type", "*fmt.wrapError")}, e.Attributes)
	}
	assert.Equal(t, "no_route", got[1].Status().Description)
	assert.Empty(t, got[1].Events())
}
