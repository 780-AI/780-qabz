package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/common/auth/authtest"
	"qabz/config"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Parse([]byte(`
database:
  postgres: {host: "127.0.0.1", port: 1, password: "s3cret"}
  redis: {host: "127.0.0.1", port: 1}
metrics:
  prometheus: {addr: "127.0.0.1:0"}
server:
  grpc: {addr: "127.0.0.1:0"}
  http: {addr: "127.0.0.1:0"}
debug:
  auth: {disable: true}
`))
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	return c
}

// defaultApp builds the App from the real dependencies. It stops the test if a step fails.
func defaultApp(t *testing.T) *App {
	t.Helper()
	b, err := NewDefaultBuilder(context.Background(), testConfig(t))
	if !assert.NoError(t, err) || !assert.NotNil(t, b) {
		t.FailNow()
	}
	return build(t, b)
}

func TestDefaultBuilderBuilds(t *testing.T) {
	a := defaultApp(t)
	defer a.Database().Close()
	defer a.Cache().Close()
}

func TestDefaultStartFailsWithoutServers(t *testing.T) {
	a := defaultApp(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := a.Start(ctx)
	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), "s3cret")
	}
}

func TestDefaultBuilderRejectsBadLogLevel(t *testing.T) {
	cfg := testConfig(t)
	cfg.LogLevel = "loud"
	_, err := NewDefaultBuilder(context.Background(), cfg)
	assert.Error(t, err)
}

// TestDefaultBuilderAuthRules: with the check on, the start needs the HMAC secret and an audience;
// with debug.auth.disable it needs neither. The secret is never in the start error.
func TestDefaultBuilderAuthRules(t *testing.T) {
	cases := []struct {
		name     string
		disable  bool
		secret   config.Secret
		audience string
		ok       bool
	}{
		{name: "disabled, no secret", disable: true, ok: true},
		{name: "on, secret and audience", secret: authtest.Secret, audience: authtest.Audience, ok: true},
		{name: "on, no secret", audience: authtest.Audience},
		{name: "on, no audience", secret: authtest.Secret},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig(t)
			cfg.DebugAuthDisable = tc.disable
			cfg.JWTHMACSecret = tc.secret
			cfg.JWTAudience = tc.audience
			b, err := NewDefaultBuilder(context.Background(), cfg)
			if !tc.ok {
				if assert.Error(t, err) {
					assert.NotContains(t, err.Error(), authtest.Secret)
				}
				return
			}
			if assert.NoError(t, err) && assert.NotNil(t, b) {
				a := build(t, b)
				a.Database().Close()
				a.Cache().Close()
			}
		})
	}
}
