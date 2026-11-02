package config

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func env(m map[string]string) Lookup {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestLoadFromDefaults(t *testing.T) {
	c, err := LoadFrom(env(map[string]string{"POSTGRES_PASSWORD": "pw"}))
	assert.NoError(t, err)
	assert.Equal(t, "qabz", c.ServiceName)
	assert.Equal(t, "info", c.LogLevel)
	assert.Equal(t, 5432, c.PostgresPort)
	assert.Equal(t, "localhost:6379", c.RedisAddr())
	assert.Equal(t, ":9090", c.MetricsAddr)
	assert.Empty(t, c.OTLPEndpoint)
	assert.Equal(t, 30*time.Second, c.HotReloadInterval)
	assert.True(t, c.MigrateOnStart)
	assert.Equal(t, 60*time.Second, c.MigrateTimeout)
}

func TestLoadFromOverrides(t *testing.T) {
	c, err := LoadFrom(env(map[string]string{
		"POSTGRES_PASSWORD": "pw", "POSTGRES_HOST": "db", "POSTGRES_PORT": "6000",
		"REDIS_HOST": "cache", "REDIS_PORT": "7000", "OTLP_ENDPOINT": "jaeger:4317",
		"HOT_RELOAD_INTERVAL": "5s", "MIGRATE_ON_START": "false", "MIGRATE_TIMEOUT": "2m",
	}))
	assert.NoError(t, err)
	assert.Equal(t, "db:6000/qabz", c.PostgresTarget())
	assert.Equal(t, "cache:7000", c.RedisAddr())
	assert.Equal(t, "jaeger:4317", c.OTLPEndpoint)
	assert.Equal(t, 5*time.Second, c.HotReloadInterval)
	assert.False(t, c.MigrateOnStart)
	assert.Equal(t, 2*time.Minute, c.MigrateTimeout)
}

func TestLoadFromErrors(t *testing.T) {
	_, err := LoadFrom(env(map[string]string{"POSTGRES_PORT": "abc", "REDIS_PORT": "70000",
		"HOT_RELOAD_INTERVAL": "soon", "MIGRATE_TIMEOUT": "-1s", "MIGRATE_ON_START": "maybe"}))
	assert.ErrorContains(t, err, "POSTGRES_PASSWORD is required")
	assert.ErrorContains(t, err, "POSTGRES_PORT must be a port number")
	assert.ErrorContains(t, err, "REDIS_PORT must be a port number")
	assert.ErrorContains(t, err, "HOT_RELOAD_INTERVAL must be a positive duration")
	assert.ErrorContains(t, err, "MIGRATE_TIMEOUT must be a positive duration")
	assert.ErrorContains(t, err, "MIGRATE_ON_START must be true or false")
}

func TestSecretNeverPrints(t *testing.T) {
	c, err := LoadFrom(env(map[string]string{"POSTGRES_PASSWORD": "s3cret", "REDIS_PASSWORD": "r3dis"}))
	assert.NoError(t, err)
	for _, out := range []string{fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), c.PostgresTarget()} {
		assert.NotContains(t, out, "s3cret")
		assert.NotContains(t, out, "r3dis")
	}
}

func TestPostgresDSNEscapes(t *testing.T) {
	c, _ := LoadFrom(env(map[string]string{"POSTGRES_PASSWORD": "p@ss/w:rd"}))
	assert.Equal(t, "postgres://qabz:p%40ss%2Fw%3Ard@localhost:5432/qabz", c.PostgresDSN())
}
