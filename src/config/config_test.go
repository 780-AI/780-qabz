package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestParseDefaults(t *testing.T) {
	c, err := Parse([]byte(`database: {postgres: {password: "pw"}}`))
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

const fullYAML = `
service_name: "svc"
hot_reload_interval: "5s"
logs:
  level: "debug"
  otlp_endpoint: "jaeger:4317"
metrics:
  prometheus:
    addr: ":9191"
database:
  postgres:
    host: "db"
    port: 6000
    user: "u"
    password: "pw"
    db: "d"
  redis:
    host: "cache"
    port: 7000
    password: "rp"
  migration:
    on_start: false
    timeout: "2m"
`

func TestParseAllKeys(t *testing.T) {
	c, err := Parse([]byte(fullYAML))
	assert.NoError(t, err)
	assert.Equal(t, "svc", c.ServiceName)
	assert.Equal(t, "debug", c.LogLevel)
	assert.Equal(t, "jaeger:4317", c.OTLPEndpoint)
	assert.Equal(t, ":9191", c.MetricsAddr)
	assert.Equal(t, "db:6000/d", c.PostgresTarget())
	assert.Equal(t, "u", c.PostgresUser)
	assert.Equal(t, "pw", c.PostgresPassword.Reveal())
	assert.Equal(t, "cache:7000", c.RedisAddr())
	assert.Equal(t, "rp", c.RedisPassword.Reveal())
	assert.Equal(t, 5*time.Second, c.HotReloadInterval)
	assert.False(t, c.MigrateOnStart)
	assert.Equal(t, 2*time.Minute, c.MigrateTimeout)
}

func TestParseRuleErrors(t *testing.T) {
	_, err := Parse([]byte(`
hot_reload_interval: "-1s"
database:
  postgres: {port: 0}
  redis: {port: 70000}
  migration: {timeout: "0s"}
`))
	assert.ErrorContains(t, err, "database.postgres.password is required")
	assert.ErrorContains(t, err, "database.postgres.port must be a port number")
	assert.ErrorContains(t, err, "database.redis.port must be a port number")
	assert.ErrorContains(t, err, "hot_reload_interval must be a positive duration")
	assert.ErrorContains(t, err, "database.migration.timeout must be a positive duration")
}

func TestParseEmptyFile(t *testing.T) {
	_, err := Parse(nil)
	assert.ErrorContains(t, err, "database.postgres.password is required")
}

func TestParseWrongTypesAndBadYAML(t *testing.T) {
	for name, doc := range map[string]string{
		"string given a number":   "service_name: 5\ndatabase: {postgres: {password: pw}}",
		"int given a string":      "database: {postgres: {password: pw, port: \"5432\"}}",
		"bool given a string":     "database: {postgres: {password: pw}, migration: {on_start: \"yes\"}}",
		"duration given a number": "hot_reload_interval: 30\ndatabase: {postgres: {password: pw}}",
		"bad duration":            "hot_reload_interval: soon\ndatabase: {postgres: {password: pw}}",
		"null":                    "service_name:\ndatabase: {postgres: {password: pw}}",
		"object given an array":   "database: [1, 2]",
		"scalar in the path":      "database: {postgres: 5}",
		"root is an array":        "- a\n- b",
		"invalid yaml":            "database: {postgres: [",
	} {
		_, err := Parse([]byte(doc))
		if assert.Error(t, err, name) {
			assert.NotContains(t, err.Error(), "s3cret", name)
		}
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "c.yml")
	assert.NoError(t, os.WriteFile(good, []byte(fullYAML), 0o600))
	c, err := LoadFile(good)
	assert.NoError(t, err)
	assert.Equal(t, "svc", c.ServiceName)
	_, err = LoadFile(filepath.Join(dir, "missing.yml"))
	assert.Error(t, err)
}

func TestSecretNeverPrints(t *testing.T) {
	c, err := Parse([]byte(`{database: {postgres: {password: s3cret}, redis: {password: r3dis}}, auth: {secrets: {hmac: hm4c}}}`))
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	for _, out := range []string{fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), c.PostgresTarget()} {
		assert.NotContains(t, out, "s3cret")
		assert.NotContains(t, out, "r3dis")
		assert.NotContains(t, out, "hm4c")
	}
}

// TestParseAuthKeys: the auth.jwt keys are read from the YAML; a missing section leaves them empty.
func TestParseAuthKeys(t *testing.T) {
	c, err := Parse([]byte(`{database: {postgres: {password: x}}, auth: {secrets: {hmac: hm4c}, jwt: {audience: qabz}}}`))
	if assert.NoError(t, err) {
		assert.Equal(t, "hm4c", c.JWTHMACSecret.Reveal())
		assert.Equal(t, "qabz", c.JWTAudience)
	}
	c, err = Parse([]byte(`database: {postgres: {password: x}}`))
	if assert.NoError(t, err) {
		assert.Equal(t, "", c.JWTHMACSecret.Reveal())
		assert.Equal(t, "", c.JWTAudience)
	}
}

func TestPostgresDSNEscapes(t *testing.T) {
	cases := []struct{ name, yml, want string }{
		{"password", `database: {postgres: {password: "p@ss/w:rd"}}`,
			"postgres://qabz:p%40ss%2Fw%3Ard@localhost:5432/qabz"},
		{"user", `database: {postgres: {user: "a:b@c", password: x}}`,
			"postgres://a%3Ab%40c:x@localhost:5432/qabz"},
		{"db", `database: {postgres: {db: "d/b?x#y", password: x}}`,
			"postgres://qabz:x@localhost:5432/d%2Fb%3Fx%23y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Parse([]byte(tc.yml))
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, tc.want, c.PostgresDSN())
		})
	}
}

// TestPostgresDSNPgxParses checks that pgx, the real consumer, decodes every escaped part back.
func TestPostgresDSNPgxParses(t *testing.T) {
	c, err := Parse([]byte(`database: {postgres: {user: "u:s@r", password: "p@ss/w:rd %?#", db: "d/b?x"}}`))
	if !assert.NoError(t, err) {
		return
	}
	pc, err := pgconn.ParseConfig(c.PostgresDSN())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "u:s@r", pc.User)
	assert.Equal(t, "p@ss/w:rd %?#", pc.Password)
	assert.Equal(t, "d/b?x", pc.Database)
	assert.Equal(t, "localhost", pc.Host)
	assert.Equal(t, uint16(5432), pc.Port)
}

func TestServerAddrs(t *testing.T) {
	c, err := Parse([]byte(`database: {postgres: {password: x}}`))
	if assert.NoError(t, err) {
		assert.Equal(t, ":8082", c.GRPCAddr)
		assert.Equal(t, ":8080", c.HTTPAddr)
	}

	c, err = Parse([]byte(`{database: {postgres: {password: x}}, server: {grpc: {addr: "127.0.0.1:0"}, http: {addr: "127.0.0.1:0"}}}`))
	if assert.NoError(t, err, "port 0 on both servers is allowed (a free port each)") {
		assert.Equal(t, "127.0.0.1:0", c.GRPCAddr)
		assert.Equal(t, "127.0.0.1:0", c.HTTPAddr)
	}

	for name, tc := range map[string]struct{ yml, want string }{
		"no port":   {`{database: {postgres: {password: x}}, server: {grpc: {addr: "localhost"}}}`, "server.grpc.addr must be host:port"},
		"bad port":  {`{database: {postgres: {password: x}}, server: {http: {addr: ":http"}}}`, "server.http.addr must have a port from 0 to 65535"},
		"high port": {`{database: {postgres: {password: x}}, server: {http: {addr: ":70000"}}}`, "server.http.addr must have a port from 0 to 65535"},
		"empty":     {`{database: {postgres: {password: x}}, server: {grpc: {addr: ""}}}`, "server.grpc.addr must be host:port"},
		"same addr": {`{database: {postgres: {password: x}}, server: {grpc: {addr: ":9000"}, http: {addr: ":9000"}}}`, "must not be the same address"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yml))
			assert.ErrorContains(t, err, tc.want)
		})
	}
}
