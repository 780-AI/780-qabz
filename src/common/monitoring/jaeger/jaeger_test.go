package jaeger

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
