package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/config"
)

func testConfig() config.Config {
	c, _ := config.LoadFrom(func(k string) (string, bool) {
		m := map[string]string{"POSTGRES_PASSWORD": "s3cret", "POSTGRES_HOST": "127.0.0.1", "POSTGRES_PORT": "1",
			"REDIS_HOST": "127.0.0.1", "REDIS_PORT": "1", "METRICS_ADDR": "127.0.0.1:0"}
		v, ok := m[k]
		return v, ok
	})
	return c
}

func TestDefaultBuilderBuilds(t *testing.T) {
	b, err := NewDefaultBuilder(context.Background(), testConfig())
	assert.NoError(t, err)
	a, err := b.Build()
	assert.NoError(t, err)
	defer a.Database().Close()
	defer a.Cache().Close()
}

func TestDefaultStartFailsWithoutServers(t *testing.T) {
	b, _ := NewDefaultBuilder(context.Background(), testConfig())
	a, _ := b.Build()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := a.Start(ctx)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "s3cret")
}

func TestDefaultBuilderRejectsBadLogLevel(t *testing.T) {
	cfg := testConfig()
	cfg.LogLevel = "loud"
	_, err := NewDefaultBuilder(context.Background(), cfg)
	assert.Error(t, err)
}
