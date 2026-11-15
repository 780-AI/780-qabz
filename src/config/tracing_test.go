package config

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

func (r spanRepo) Load(ctx context.Context) ([]Row, error) {
	ctx, span := otel.Tracer("test").Start(ctx, "query")
	defer span.End()
	return r.fakeRepo.Load(ctx)
}

// TestReloadRootSpan (task 047 D12): each reload is a root span "config reload" with the result ok
// or error; the query is its child.
func TestReloadRootSpan(t *testing.T) {
	sr := jaegertest.Install(t)
	repo := &fakeRepo{rows: []Row{{"log_level", "string", "debug"}}}
	r, _, _ := newRel(t, spanRepo{repo}, time.Hour)
	r.Reload(context.Background())
	repo.set(nil, errors.New("down"))
	r.Reload(context.Background())

	reloads, queries := jaegertest.ByName(sr, reloadSpan), jaegertest.ByName(sr, "query")
	if !assert.Len(t, reloads, 2) || !assert.Len(t, queries, 2) {
		return
	}
	for i, want := range []struct {
		result string
		code   codes.Code
	}{{"ok", codes.Unset}, {"error", codes.Error}} {
		s := reloads[i]
		assert.False(t, s.Parent().IsValid(), "a root")
		assert.Equal(t, s.SpanContext().SpanID(), queries[i].Parent().SpanID())
		assert.Equal(t, []attribute.KeyValue{attribute.String("result", want.result)}, s.Attributes())
		assert.Equal(t, want.code, s.Status().Code)
	}
	jaegertest.AssertClean(t, sr.Ended(), "down", "debug")
}
