package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"qabz/common/monitoring/jaeger/jaegertest"
)

// TestLiveRedisSpans (task 047 D8): each command is a client span, a child of the span in ctx, with
// the command name and no argument (the key has a phone, the value a bill ID); redis.Nil is not an
// error.
func TestLiveRedisSpans(t *testing.T) {
	addr, password, prefix := liveRedis(t)
	sr := jaegertest.Install(t)
	c := New(addr, password, prefix)
	defer c.Close()

	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	key := c.Key("inquiry:989121234567")
	assert.NoError(t, c.rdb.Set(ctx, key, "1234567890125", 10*time.Second).Err())
	v, err := c.rdb.Get(ctx, key).Result()
	assert.NoError(t, err)
	assert.Equal(t, "1234567890125", v)
	assert.Error(t, c.rdb.Get(ctx, c.Key("none")).Err())
	parent.End()

	set, get := jaegertest.ByName(sr, "set"), jaegertest.ByName(sr, "get")
	if !assert.Len(t, set, 1) || !assert.Len(t, get, 2) {
		return
	}
	for _, s := range append(set, get...) {
		assert.Equal(t, trace.SpanKindClient, s.SpanKind())
		assert.Equal(t, parent.SpanContext().SpanID(), s.Parent().SpanID())
		assert.Equal(t, codes.Unset, s.Status().Code, "redis.Nil is not an error")
	}
	jaegertest.AssertClean(t, sr.Ended(), "989121234567", "1234567890125", prefix)
}
