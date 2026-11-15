package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"qabz/common/monitoring/jaeger/jaegertest"
)

// spanRepo is fakeRepo with a child span "query" for each call, as the Postgres tracer makes.
type spanRepo struct{ *fakeRepo }

func (r spanRepo) ListProviders(ctx context.Context) ([]Provider, error) {
	ctx, span := otel.Tracer("test").Start(ctx, "query")
	defer span.End()
	return r.fakeRepo.ListProviders(ctx)
}

// TestLoadRootSpan (task 047 D12): each load is a root span "providers load" (also when ctx has a
// span), the query is its child, the result is the one of qabz_providers_loads_total; an aborted
// load has the result aborted and no error status.
func TestLoadRootSpan(t *testing.T) {
	sr := jaegertest.Install(t)
	repo := &fakeRepo{}
	repo.set(moveRows(), nil)
	s, err := NewProvidersService(spanRepo{repo}, Options{Interval: func() time.Duration { return time.Hour }, Logger: &fakeLog{}})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	ctx, outer := otel.Tracer("test").Start(context.Background(), "outer")
	assert.NoError(t, s.Load(ctx))
	repo.set(nil, errors.New("down"))
	assert.Error(t, s.Load(ctx))
	abort, cancel := context.WithCancel(context.Background())
	cancel()
	repo.set(nil, context.Canceled)
	assert.Error(t, s.load(ctx, abort))
	outer.End()

	loads, queries := jaegertest.ByName(sr, loadSpan), jaegertest.ByName(sr, "query")
	if !assert.Len(t, loads, 3) || !assert.Len(t, queries, 3) {
		return
	}
	for i, want := range []struct {
		result string
		code   codes.Code
	}{{LoadOK, codes.Unset}, {LoadError, codes.Error}, {"aborted", codes.Unset}} {
		l := loads[i]
		assert.False(t, l.Parent().IsValid(), "a root")
		assert.Equal(t, l.SpanContext().SpanID(), queries[i].Parent().SpanID(), "the query is a child")
		assert.Equal(t, []attribute.KeyValue{attribute.String("result", want.result)}, l.Attributes())
		assert.Equal(t, want.code, l.Status().Code)
	}
	jaegertest.AssertClean(t, sr.Ended(), "down")
}
