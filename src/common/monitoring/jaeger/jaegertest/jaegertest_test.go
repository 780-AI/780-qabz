package jaegertest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// fakeT records whether a check failed.
type fakeT struct{ failed bool }

func (f *fakeT) Helper()               {}
func (f *fakeT) Errorf(string, ...any) { f.failed = true }

func spans(t *testing.T, f func(trace.Span)) []sdktrace.ReadOnlySpan {
	sr := Install(t)
	_, s := otel.Tracer("test").Start(context.Background(), "step")
	f(s)
	s.End()
	return sr.Ended()
}

func TestAssertCleanPasses(t *testing.T) {
	got := spans(t, func(s trace.Span) {
		s.SetAttributes(attribute.String("bill_type", "water"), attribute.String("rpc.method", "Inquiry"))
		s.AddEvent("exception", trace.WithAttributes(attribute.String("exception.type", "*errors.errorString")))
		s.SetStatus(codes.Error, "provider_down")
	})
	assert.True(t, AssertClean(t, got, "989121234567"))
	// a library span (scope not qabz/...) may have exception.message, its value still checked
	sr := Install(t)
	_, lib := otel.Tracer("github.com/redis/go-redis/extra/redisotel").Start(context.Background(), "get")
	lib.AddEvent("exception", trace.WithAttributes(attribute.String("exception.message", "WRONGTYPE")))
	lib.End()
	assert.True(t, AssertClean(t, sr.Ended(), "989121234567"))
	assert.False(t, AssertClean(&fakeT{}, sr.Ended(), "WRONGTYPE"))
	assert.Len(t, ByName(nil, "step"), 0)
}

// Positive controls: each forbidden case fails the check.
func TestAssertCleanFails(t *testing.T) {
	cases := map[string]func(trace.Span){
		"key":         func(s trace.Span) { s.SetAttributes(attribute.String("phone", "x")) },
		"value":       func(s trace.Span) { s.SetAttributes(attribute.String("provider", "p-989121234567")) },
		"event key":   func(s trace.Span) { s.AddEvent("e", trace.WithAttributes(attribute.String("body", "x"))) },
		"event value": func(s trace.Span) { s.AddEvent("e", trace.WithAttributes(attribute.String("result", "989121234567"))) },
		"status":      func(s trace.Span) { s.SetStatus(codes.Error, "bad 989121234567") },
		"event name":  func(s trace.Span) { s.AddEvent("989121234567") },
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			got := spans(t, f)
			inner := &fakeT{}
			assert.False(t, AssertClean(inner, got, "989121234567"))
			assert.True(t, inner.failed)
		})
	}
	t.Run("library key on a qabz span", func(t *testing.T) {
		sr := Install(t)
		_, s := otel.Tracer("qabz/core/x").Start(context.Background(), "step")
		s.AddEvent("exception", trace.WithAttributes(attribute.String("exception.message", "x")))
		s.End()
		assert.False(t, AssertClean(&fakeT{}, sr.Ended()))
	})
	t.Run("name", func(t *testing.T) {
		sr := Install(t)
		_, s := otel.Tracer("test").Start(context.Background(), "x 989121234567")
		s.End()
		assert.False(t, AssertClean(&fakeT{}, sr.Ended(), "989121234567"))
		assert.Len(t, ByName(sr, "x 989121234567"), 1)
	})
}

func TestInstallRestores(t *testing.T) {
	prevTP, prevProp := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	t.Run("inner", func(t *testing.T) {
		Install(t)
		_, ok := otel.GetTextMapPropagator().(propagation.TraceContext)
		assert.True(t, ok)
	})
	assert.Equal(t, prevTP, otel.GetTracerProvider())
	assert.Equal(t, prevProp, otel.GetTextMapPropagator())
}
