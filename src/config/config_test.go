package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
)

func TestParseDefaults(t *testing.T) {
	c, err := Parse([]byte(`{mode: dev, database: {postgres: {password: "pw"}}}`))
	assert.NoError(t, err)
	assert.Equal(t, "qabz", c.ServiceName)
	assert.Equal(t, 0, c.ServiceID)
	assert.Equal(t, "info", c.LogLevel)
	assert.Equal(t, 5432, c.PostgresPort)
	assert.Equal(t, "localhost:6379", c.RedisAddr())
	assert.Equal(t, ":9090", c.MetricsAddr)
	assert.True(t, c.MetricsEnabled)
	assert.Empty(t, c.OTLPEndpoint)
	assert.Equal(t, 30*time.Second, c.HotReloadInterval)
	assert.True(t, c.MigrateOnStart)
	assert.Equal(t, 60*time.Second, c.MigrateTimeout)
}

const fullYAML = `
mode: stage
service_name: "svc"
service:
  id: 105
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
	assert.Equal(t, ModeStage, c.Mode)
	assert.Equal(t, "svc", c.ServiceName)
	assert.Equal(t, 105, c.ServiceID)
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
		"string given a number":   "mode: dev\nservice_name: 5\ndatabase: {postgres: {password: pw}}",
		"int given a string":      "mode: dev\ndatabase: {postgres: {password: pw, port: \"5432\"}}",
		"bool given a string":     "mode: dev\ndatabase: {postgres: {password: pw}, migration: {on_start: \"yes\"}}",
		"duration given a number": "mode: dev\nhot_reload_interval: 30\ndatabase: {postgres: {password: pw}}",
		"bad duration":            "mode: dev\nhot_reload_interval: soon\ndatabase: {postgres: {password: pw}}",
		"null":                    "mode: dev\nservice_name:\ndatabase: {postgres: {password: pw}}",
		"object given an array":   "mode: dev\ndatabase: [1, 2]",
		"scalar in the path":      "mode: dev\ndatabase: {postgres: 5}",
		"root is an array":        "- a\n- b",
		"invalid yaml":            "mode: dev\ndatabase: {postgres: [",
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
	c, err := Parse([]byte(`{mode: dev, database: {postgres: {password: s3cret}, redis: {password: r3dis}}, auth: {secrets: {hmac: hm4c}}}`))
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	for _, out := range []string{fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), c.PostgresTarget()} {
		assert.NotContains(t, out, "s3cret")
		assert.NotContains(t, out, "r3dis")
		assert.NotContains(t, out, "hm4c")
	}
}

// TestParseMetricsSwitch: the Prometheus switch is on by default and with an explicit true; it is
// off only with an explicit false (Implementation_Plan Step 5).
func TestParseMetricsSwitch(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want bool
	}{
		{name: "default", yaml: `{mode: dev, database: {postgres: {password: x}}}`, want: true},
		{name: "default with other metrics keys", yaml: `{mode: dev, database: {postgres: {password: x}}, metrics: {prometheus: {addr: ":1"}}}`, want: true},
		{name: "explicit on", yaml: `{mode: dev, database: {postgres: {password: x}}, metrics: {prometheus: {enabled: true}}}`, want: true},
		{name: "explicit off", yaml: `{mode: dev, database: {postgres: {password: x}}, metrics: {prometheus: {enabled: false}}}`, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Parse([]byte(tc.yaml))
			if assert.NoError(t, err) {
				assert.Equal(t, tc.want, c.MetricsEnabled)
			}
		})
	}
	_, err := Parse([]byte(`{mode: dev, database: {postgres: {password: x}}, metrics: {prometheus: {enabled: "no"}}}`))
	assert.Error(t, err, "a non-bool value is an error, not off")
}

// TestParseAuthKeys: auth.secrets.hmac is read from the YAML; a missing section leaves it empty.
func TestParseAuthKeys(t *testing.T) {
	c, err := Parse([]byte(`{mode: dev, database: {postgres: {password: x}}, auth: {secrets: {hmac: hm4c}}}`))
	if assert.NoError(t, err) {
		assert.Equal(t, "hm4c", c.JWTHMACSecret.Reveal())
	}
	c, err = Parse([]byte(`{mode: dev, database: {postgres: {password: x}}}`))
	if assert.NoError(t, err) {
		assert.Equal(t, "", c.JWTHMACSecret.Reveal())
	}
}

func TestPostgresDSNEscapes(t *testing.T) {
	cases := []struct{ name, yml, want string }{
		{"password", `{mode: dev, database: {postgres: {password: "p@ss/w:rd"}}}`,
			"postgres://qabz:p%40ss%2Fw%3Ard@localhost:5432/qabz"},
		{"user", `{mode: dev, database: {postgres: {user: "a:b@c", password: x}}}`,
			"postgres://a%3Ab%40c:x@localhost:5432/qabz"},
		{"db", `{mode: dev, database: {postgres: {db: "d/b?x#y", password: x}}}`,
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
	c, err := Parse([]byte(`{mode: dev, database: {postgres: {user: "u:s@r", password: "p@ss/w:rd %?#", db: "d/b?x"}}}`))
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
	c, err := Parse([]byte(`{mode: dev, database: {postgres: {password: x}}}`))
	if assert.NoError(t, err) {
		assert.Equal(t, ":8082", c.GRPCAddr)
		assert.Equal(t, ":8080", c.HTTPAddr)
	}

	c, err = Parse([]byte(`{mode: dev, database: {postgres: {password: x}}, server: {grpc: {addr: "127.0.0.1:0"}, http: {addr: "127.0.0.1:0"}}}`))
	if assert.NoError(t, err, "port 0 on both servers is allowed (a free port each)") {
		assert.Equal(t, "127.0.0.1:0", c.GRPCAddr)
		assert.Equal(t, "127.0.0.1:0", c.HTTPAddr)
	}

	for name, tc := range map[string]struct{ yml, want string }{
		"no port":   {`{mode: dev, database: {postgres: {password: x}}, server: {grpc: {addr: "localhost"}}}`, "server.grpc.addr must be host:port"},
		"bad port":  {`{mode: dev, database: {postgres: {password: x}}, server: {http: {addr: ":http"}}}`, "server.http.addr must have a port from 0 to 65535"},
		"high port": {`{mode: dev, database: {postgres: {password: x}}, server: {http: {addr: ":70000"}}}`, "server.http.addr must have a port from 0 to 65535"},
		"empty":     {`{mode: dev, database: {postgres: {password: x}}, server: {grpc: {addr: ""}}}`, "server.grpc.addr must be host:port"},
		"same addr": {`{mode: dev, database: {postgres: {password: x}}, server: {grpc: {addr: ":9000"}, http: {addr: ":9000"}}}`, "must not be the same address"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Parse([]byte(tc.yml))
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestParseServiceIDRange(t *testing.T) {
	for name, id := range map[string]string{"negative": "-1", "above int32": "2147483648"} {
		_, err := Parse([]byte("mode: dev\nservice: {id: " + id + "}\ndatabase: {postgres: {password: pw}}"))
		assert.ErrorContains(t, err, "service.id must be from 0 to 2147483647", name)
	}
	c, err := Parse([]byte("mode: dev\nservice: {id: 2147483647}\ndatabase: {postgres: {password: pw}}"))
	assert.NoError(t, err)
	assert.Equal(t, 2147483647, c.ServiceID)
}

func TestParseDebugAuthMockUserID(t *testing.T) {
	const id = "5a2c8173-cae5-4a6b-a109-af45f760824b"
	c, err := Parse([]byte("mode: dev\ndatabase: {postgres: {password: pw}}\ndebug: {auth: {disable: true, mock_user_id: \"" + id + "\"}}"))
	if assert.NoError(t, err) {
		assert.Equal(t, id, c.DebugAuthMockUserID)
	}
	c, err = Parse([]byte("mode: dev\ndatabase: {postgres: {password: pw}}"))
	if assert.NoError(t, err) {
		assert.Equal(t, "", c.DebugAuthMockUserID, "the default is no user")
	}
	for _, bad := range []string{"x", "5A2C8173-CAE5-4A6B-A109-AF45F760824B", "{" + id + "}", "urn:uuid:" + id,
		"5a2c8173cae54a6ba109af45f760824b", " " + id} {
		_, err := Parse([]byte("mode: dev\ndatabase: {postgres: {password: pw}}\ndebug: {auth: {mock_user_id: \"" + bad + "\"}}"))
		assert.ErrorContains(t, err, "debug.auth.mock_user_id must be a UUID", bad)
	}
}

// TestParseMode: mode is required and accepts only dev, stage and prod (exact, lower case).
func TestParseMode(t *testing.T) {
	for _, m := range []string{ModeDev, ModeStage, ModeProd} {
		c, err := Parse([]byte("mode: " + m + "\ndatabase: {postgres: {password: pw}}"))
		if assert.NoError(t, err, m) {
			assert.Equal(t, m, c.Mode)
		}
	}
	_, err := Parse([]byte("database: {postgres: {password: pw}}"))
	assert.ErrorContains(t, err, "mode is required (dev, stage or prod)", "missing key")
	_, err = Parse([]byte("mode: \"\"\ndatabase: {postgres: {password: pw}}"))
	assert.ErrorContains(t, err, "mode is required (dev, stage or prod)", "empty value")
	for _, bad := range []string{"Dev", "DEV", "development", "production", "test", " dev"} {
		_, err := Parse([]byte("mode: \"" + bad + "\"\ndatabase: {postgres: {password: pw}}"))
		assert.ErrorContains(t, err, "mode must be dev, stage or prod", bad)
	}
}

// TestProvidersLoadInterval: providers.general.load_interval (task 020) defaults to 1m, takes any
// duration of 1s or more, and rejects a lower value. It is the one hot-reloadable key.
func TestProvidersLoadInterval(t *testing.T) {
	c, err := Parse([]byte(`{mode: dev, database: {postgres: {password: pw}}}`))
	assert.NoError(t, err)
	assert.Equal(t, time.Minute, c.ProvidersLoadInterval, "default")

	for raw, want := range map[string]time.Duration{"1s": time.Second, "5s": 5 * time.Second, "1500ms": 1500 * time.Millisecond, "1h": time.Hour} {
		c, err := Parse([]byte(`{mode: dev, database: {postgres: {password: pw}}, providers: {general: {load_interval: "` + raw + `"}}}`))
		assert.NoError(t, err, raw)
		assert.Equal(t, want, c.ProvidersLoadInterval, raw)
	}
	for _, raw := range []string{`"999ms"`, `"0s"`, `"-1s"`, `"1ns"`} {
		_, err := Parse([]byte(`{mode: dev, database: {postgres: {password: pw}}, providers: {general: {load_interval: ` + raw + `}}}`))
		assert.ErrorContains(t, err, "providers.general.load_interval must be at least 1s", raw)
	}
	for _, raw := range []string{`soon`, `60`} {
		_, err := Parse([]byte(`{mode: dev, database: {postgres: {password: pw}}, providers: {general: {load_interval: ` + raw + `}}}`))
		assert.Error(t, err, raw)
	}

	fields, err := hotFields[Config]()
	assert.NoError(t, err)
	assert.Equal(t, []hotField{{index: fieldIndex(t, "ProvidersLoadInterval"), key: "providers_load_interval", typ: "duration"}}, fields)
}

// fieldIndex returns the index of the Config field name. It stops the test if there is none.
func fieldIndex(t *testing.T, name string) int {
	t.Helper()
	f, ok := reflect.TypeFor[Config]().FieldByName(name)
	if !assert.True(t, ok, name) || !assert.Len(t, f.Index, 1) {
		t.FailNow()
	}
	return f.Index[0]
}

// TestParseDebugSectionOnlyInDev (task 020c): a mode other than dev fully removes the debug section,
// so a config with the section gives the same Config as the same config without it. In dev the
// section is read.
func TestParseDebugSectionOnlyInDev(t *testing.T) {
	const id = "5a2c8173-cae5-4a6b-a109-af45f760824b"
	const base = "database: {postgres: {password: pw}}\n"
	sections := map[string]string{
		"full":    "debug: {auth: {disable: true, mock_user_id: \"" + id + "\"}}\n",
		"partial": "debug: {auth: {disable: true}}\n",
		"empty":   "debug: {}\n",
		"null":    "debug:\n",
		"bad":     "debug: {auth: {mock_user_id: \"not-a-uuid\"}}\n", // not checked: the section is removed
	}
	for _, mode := range []string{ModeStage, ModeProd} {
		want, err := Parse([]byte("mode: " + mode + "\n" + base))
		if !assert.NoError(t, err, mode) {
			continue
		}
		assert.False(t, want.DebugAuthDisable, mode)
		for name, debug := range sections {
			got, err := Parse([]byte("mode: " + mode + "\n" + base + debug))
			if assert.NoError(t, err, mode+" "+name) {
				assert.Equal(t, want, got, mode+" "+name)
			}
		}
	}

	c, err := Parse([]byte("mode: dev\n" + base + sections["full"]))
	if assert.NoError(t, err) {
		assert.True(t, c.DebugAuthDisable, "dev reads debug.auth.disable")
		assert.Equal(t, id, c.DebugAuthMockUserID, "dev reads debug.auth.mock_user_id")
	}
	_, err = Parse([]byte("mode: dev\n" + base + sections["bad"]))
	assert.ErrorContains(t, err, "debug.auth.mock_user_id must be a UUID", "dev checks the debug values")
	c, err = Parse([]byte("mode: dev\n" + base + sections["empty"]))
	if assert.NoError(t, err, "empty") {
		assert.False(t, c.DebugAuthDisable, "empty")
	}
	// A null section in dev stays an error, as before the task (the decoder wants an object).
	_, err = Parse([]byte("mode: dev\n" + base + sections["null"]))
	assert.ErrorContains(t, err, "debug is null, want an object", "null")
}

// TestParseWaterInquiryKeys (task 021): the Qabzino keys and caching.water have their defaults, take
// their values and reject a value out of range.
func TestParseWaterInquiryKeys(t *testing.T) {
	const base = "mode: dev\ndatabase: {postgres: {password: pw}}\n"
	c, err := Parse([]byte(base))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "", c.QabzinoGRPCAddress, "default address")
	assert.Equal(t, 10*time.Second, c.QabzinoTimeout, "default timeout")
	assert.Equal(t, time.Hour, c.CachingWater, "default cache TTL (bill: caching.water 1h)")
	assert.False(t, c.DebugMockQabzinoEnable, "the mock is off by default")
	assert.Equal(t, MockReturnSuccess, c.DebugMockQabzinoReturnType)

	c, err = Parse([]byte(base + `providers: {qabzino: {grpc_address: "q:9", timeout: "2s"}}
caching: {water: "0s"}
debug: {mocks: {qabzino: {enable: true, return_type: error}}}
`))
	if assert.NoError(t, err) {
		assert.Equal(t, "q:9", c.QabzinoGRPCAddress)
		assert.Equal(t, 2*time.Second, c.QabzinoTimeout)
		assert.Equal(t, time.Duration(0), c.CachingWater, "0 turns the cache off")
		assert.True(t, c.DebugMockQabzinoEnable)
		assert.Equal(t, MockReturnError, c.DebugMockQabzinoReturnType)
	}

	for yml, want := range map[string]string{
		"providers: {qabzino: {timeout: \"0s\"}}":                      "providers.qabzino.timeout must be a positive duration",
		"providers: {qabzino: {timeout: \"-1s\"}}":                     "providers.qabzino.timeout must be a positive duration",
		"caching: {water: \"-1s\"}":                                    "caching.water must not be negative",
		"debug: {mocks: {qabzino: {return_type: \"maybe\"}}}":          "debug.mocks.qabzino.return_type must be",
		"debug: {mocks: {qabzino: {enable: true, return_type: \"\"}}}": "debug.mocks.qabzino.return_type must be",
	} {
		_, err := Parse([]byte(base + yml))
		assert.ErrorContains(t, err, want, yml)
	}
}

// TestMockSectionOnlyInDev (task 021, decision 4): a mode other than dev removes the debug section
// (task 020c), so debug.mocks.qabzino.enable: true is ignored there: the mock is off and the real
// provider is used. It is not an error at start.
func TestMockSectionOnlyInDev(t *testing.T) {
	const base = "database: {postgres: {password: pw}}\ndebug: {mocks: {qabzino: {enable: true, return_type: error}}}\n"
	for _, mode := range []string{ModeStage, ModeProd} {
		c, err := Parse([]byte("mode: " + mode + "\n" + base))
		if assert.NoError(t, err, mode) {
			assert.False(t, c.DebugMockQabzinoEnable, mode)
			assert.Equal(t, MockReturnSuccess, c.DebugMockQabzinoReturnType, mode)
		}
	}
	c, err := Parse([]byte("mode: dev\n" + base))
	if assert.NoError(t, err) {
		assert.True(t, c.DebugMockQabzinoEnable)
	}
}

func TestValidMockReturnType(t *testing.T) {
	assert.True(t, ValidMockReturnType("success"))
	assert.True(t, ValidMockReturnType("error"))
	assert.False(t, ValidMockReturnType(""))
	assert.False(t, ValidMockReturnType("Success"))
}

// TestParseElectricityInquiryKeys (task 022): the Tavanir keys and caching.electricity have their
// defaults, take their values and are checked; they do not depend on the Qabzino and water keys.
func TestParseElectricityInquiryKeys(t *testing.T) {
	const base = "mode: dev\ndatabase: {postgres: {password: pw}}\n"
	c, err := Parse([]byte(base))
	if !assert.NoError(t, err) {
		return
	}
	assert.False(t, c.TavanirEnabled, "Tavanir is off by default (task 034)")
	assert.Equal(t, 10*time.Second, c.TavanirTimeout, "default timeout")
	assert.Equal(t, time.Hour, c.CachingElectricity, "default cache TTL (bill: caching.electricity 1h)")
	assert.False(t, c.DebugMockTavanirEnable, "the mock is off by default")
	assert.Equal(t, MockReturnSuccess, c.DebugMockTavanirReturnType)

	c, err = Parse([]byte(base + `providers: {tavanir: {enabled: true, timeout: "3s"}}
caching: {electricity: "0s"}
debug: {mocks: {tavanir: {enable: true, return_type: error}}}
`))
	if assert.NoError(t, err) {
		assert.True(t, c.TavanirEnabled)
		assert.Equal(t, 3*time.Second, c.TavanirTimeout)
		assert.Equal(t, time.Duration(0), c.CachingElectricity, "0 turns the cache off")
		assert.True(t, c.DebugMockTavanirEnable)
		assert.Equal(t, MockReturnError, c.DebugMockTavanirReturnType)
		assert.Equal(t, "", c.QabzinoGRPCAddress, "the Qabzino keys keep their defaults")
		assert.Equal(t, 10*time.Second, c.QabzinoTimeout)
		assert.Equal(t, time.Hour, c.CachingWater)
		assert.False(t, c.DebugMockQabzinoEnable)
	}
	c, err = Parse([]byte(base + "caching: {electricity: \"90m\", water: \"5m\"}\n"))
	if assert.NoError(t, err) {
		assert.Equal(t, 90*time.Minute, c.CachingElectricity)
		assert.Equal(t, 5*time.Minute, c.CachingWater)
	}

	for yml, want := range map[string]string{
		"providers: {tavanir: {timeout: \"0s\"}}":                      "providers.tavanir.timeout must be a positive duration",
		"providers: {tavanir: {timeout: \"-1s\"}}":                     "providers.tavanir.timeout must be a positive duration",
		"caching: {electricity: \"-1s\"}":                              "caching.electricity must not be negative",
		"debug: {mocks: {tavanir: {return_type: \"maybe\"}}}":          "debug.mocks.tavanir.return_type must be",
		"debug: {mocks: {tavanir: {enable: true, return_type: \"\"}}}": "debug.mocks.tavanir.return_type must be",
	} {
		_, err := Parse([]byte(base + yml))
		assert.ErrorContains(t, err, want, yml)
	}
}

// TestTavanirMockSectionOnlyInDev (task 022): a mode other than dev removes the debug section (task
// 020c), so debug.mocks.tavanir.enable: true is ignored there and a bad return type is not checked.
func TestTavanirMockSectionOnlyInDev(t *testing.T) {
	const base = "database: {postgres: {password: pw}}\ndebug: {mocks: {tavanir: {enable: true, return_type: error}}}\n"
	for _, mode := range []string{"stage", "prod"} {
		c, err := Parse([]byte("mode: " + mode + "\n" + base))
		if assert.NoError(t, err, mode) {
			assert.False(t, c.DebugMockTavanirEnable, mode)
			assert.Equal(t, MockReturnSuccess, c.DebugMockTavanirReturnType, mode)
		}
		_, err = Parse([]byte("mode: " + mode + "\ndatabase: {postgres: {password: pw}}\ndebug: {mocks: {tavanir: {return_type: maybe}}}\n"))
		assert.NoError(t, err, mode+": the section is removed, not checked")
	}
	c, err := Parse([]byte("mode: dev\n" + base))
	if assert.NoError(t, err) {
		assert.True(t, c.DebugMockTavanirEnable)
		assert.Equal(t, MockReturnError, c.DebugMockTavanirReturnType)
	}
}

// TestParseQabzinoProviderKeys (task 029): the Qabzino token and bill endpoints have no default
// (task 036: the real URLs are only in config.yml), take their values, and the token never prints.
func TestParseQabzinoProviderKeys(t *testing.T) {
	const base = "mode: dev\ndatabase: {postgres: {password: pw}}\n"
	c, err := Parse([]byte(base))
	if !assert.NoError(t, err) {
		return
	}
	assert.False(t, c.QabzinoEnabled, "Qabzino is disabled by default (task 030)")
	assert.Equal(t, "", c.QabzinoToken.Reveal(), "default token is empty")
	assert.Empty(t, c.QabzinoElectricityBillEndpoint)
	assert.Empty(t, c.QabzinoGasBillEndpoint)
	assert.Empty(t, c.QabzinoRightelBillEndpoint, "no default (task 037, as the other endpoints)")
	assert.Empty(t, c.QabzinoWaterBillEndpoint)

	c, err = Parse([]byte(base + `providers: {qabzino: {enabled: true, token: "tok", electricity_bill_endpoint: "http://e", gas_bill_endpoint: "http://g", rightel_bill_endpoint: "http://r", water_bill_endpoint: "http://w"}}
`))
	if assert.NoError(t, err) {
		assert.True(t, c.QabzinoEnabled)
		assert.Equal(t, "tok", c.QabzinoToken.Reveal())
		assert.Equal(t, "http://e", c.QabzinoElectricityBillEndpoint)
		assert.Equal(t, "http://g", c.QabzinoGasBillEndpoint)
		assert.Equal(t, "http://r", c.QabzinoRightelBillEndpoint)
		assert.Equal(t, "http://w", c.QabzinoWaterBillEndpoint)
		assert.NotContains(t, fmt.Sprintf("%v %+v %#v", c.QabzinoToken, c.QabzinoToken, c.QabzinoToken), "tok")
		assert.Equal(t, 10*time.Second, c.QabzinoTimeout, "the older Qabzino keys keep their defaults")
	}
}

// TestParseTavanirProviderKeys (task 033): the Tavanir endpoints (no default since task 036),
// credentials and circuit breaker have their defaults, take their values, and the password never prints.
func TestParseTavanirProviderKeys(t *testing.T) {
	const base = "mode: dev\ndatabase: {postgres: {password: pw}}\n"
	c, err := Parse([]byte(base))
	if !assert.NoError(t, err) {
		return
	}
	assert.Empty(t, c.TavanirAuthEndpoint)
	assert.Empty(t, c.TavanirBasicBranchEndpoint)
	assert.Empty(t, c.TavanirBillDocumentEndpoint)
	assert.Empty(t, c.TavanirBranchDebitEndpoint)
	assert.Equal(t, "", c.TavanirUsername)
	assert.Equal(t, "", c.TavanirPassword.Reveal(), "default password is empty")
	assert.Equal(t, "tavanir", c.TavanirCircuitBreakerName)
	assert.EqualValues(t, 100000, c.TavanirCircuitBreakerMaxRequests)
	assert.Equal(t, time.Second, c.TavanirCircuitBreakerInterval)
	assert.Equal(t, 10*time.Second, c.TavanirCircuitBreakerTimeout)
	assert.EqualValues(t, 99, c.TavanirCircuitBreakerFailureRatio)

	c, err = Parse([]byte(base + `providers: {tavanir: {timeout: "120s", auth_endpoint: "http://a", basic_branch_endpoint: "http://b", bill_document_endpoint: "http://d", branch_debit_endpoint: "http://r", username: "u", password: "secret-pw", circuit_breaker: {name: "n", max_requests: 5, interval: "2s", timeout: "3s", failure_ratio: 50}}}
`))
	if assert.NoError(t, err) {
		assert.Equal(t, 120*time.Second, c.TavanirTimeout)
		assert.Equal(t, []string{"http://a", "http://b", "http://d", "http://r", "u", "secret-pw"},
			[]string{c.TavanirAuthEndpoint, c.TavanirBasicBranchEndpoint, c.TavanirBillDocumentEndpoint, c.TavanirBranchDebitEndpoint, c.TavanirUsername, c.TavanirPassword.Reveal()})
		assert.Equal(t, "n", c.TavanirCircuitBreakerName)
		assert.EqualValues(t, 5, c.TavanirCircuitBreakerMaxRequests)
		assert.Equal(t, 2*time.Second, c.TavanirCircuitBreakerInterval)
		assert.Equal(t, 3*time.Second, c.TavanirCircuitBreakerTimeout)
		assert.EqualValues(t, 50, c.TavanirCircuitBreakerFailureRatio)
		assert.NotContains(t, fmt.Sprintf("%v %+v %#v", c.TavanirPassword, c.TavanirPassword, c.TavanirPassword), "secret-pw")
	}

	_, err = Parse([]byte(base + "providers: {tavanir: {circuit_breaker: {failure_ratio: 101}}}\n"))
	assert.ErrorContains(t, err, "providers.tavanir.circuit_breaker.failure_ratio")
	_, err = Parse([]byte(base + "providers: {tavanir: {circuit_breaker: {failure_ratio: 100}}}\n"))
	assert.NoError(t, err, "100 is valid")
}

// TestParseIrancellProviderKeys (task 035): the keys of the Irancell provider have no default, take
// their values, and a bad value names its key.
func TestParseIrancellProviderKeys(t *testing.T) {
	const base = "mode: dev\ndatabase: {postgres: {password: pw}}\n"
	c, err := Parse([]byte(base))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{"", "", "", ""},
		[]string{c.IrancellName, c.IrancellGRPCAddress, c.IrancellNATSSubject, c.IrancellGetStatusName})
	assert.Equal(t, make([]time.Duration, 9), []time.Duration{
		c.IrancellGetStatusCronTime, c.IrancellGetStatusStart, c.IrancellGetStatusEnd, c.IrancellGetStatusEvery,
		c.IrancellGetStatusLockTime, c.IrancellGetStatusReverseDeadline,
		c.IrancellGetStatusSyncAPICall, c.IrancellGetStatusSyncGetOrder, c.IrancellGetStatusSyncInsertOrder,
	})
	assert.Equal(t, 0, c.IrancellGetStatusChunk)
	assert.False(t, c.IrancellGetStatusIsLockable)

	c, err = Parse([]byte(base + `providers: {irancell: {name: "irancell", grpc_address: "irancell:8084", nats_subject: "irancell-bill", get_status: {cron_time: "15m", name: "irancell-get-status", start: "30m", end: "48h", chunk: 100, every: "16m", lock_time: "31m", is_lockable: true, reverse_deadline: "10h", sync_operations: {api_call_get_status: "30s", get_order_query: "3ms", insert_order_query: "4ms"}}}}
`))
	if assert.NoError(t, err) {
		assert.Equal(t, []string{"irancell", "irancell:8084", "irancell-bill", "irancell-get-status"},
			[]string{c.IrancellName, c.IrancellGRPCAddress, c.IrancellNATSSubject, c.IrancellGetStatusName})
		assert.Equal(t, []time.Duration{
			15 * time.Minute, 30 * time.Minute, 48 * time.Hour, 16 * time.Minute, 31 * time.Minute, 10 * time.Hour,
			30 * time.Second, 3 * time.Millisecond, 4 * time.Millisecond,
		}, []time.Duration{
			c.IrancellGetStatusCronTime, c.IrancellGetStatusStart, c.IrancellGetStatusEnd, c.IrancellGetStatusEvery,
			c.IrancellGetStatusLockTime, c.IrancellGetStatusReverseDeadline,
			c.IrancellGetStatusSyncAPICall, c.IrancellGetStatusSyncGetOrder, c.IrancellGetStatusSyncInsertOrder,
		})
		assert.Equal(t, 100, c.IrancellGetStatusChunk)
		assert.True(t, c.IrancellGetStatusIsLockable)
		assert.Equal(t, 10*time.Second, c.QabzinoTimeout, "the keys of the other providers keep their defaults")
	}

	_, err = Parse([]byte(base + "providers: {irancell: {get_status: {every: \"soon\"}}}\n"))
	assert.ErrorContains(t, err, "providers.irancell.get_status.every")
	_, err = Parse([]byte(base + "providers: {irancell: {get_status: {chunk: \"many\"}}}\n"))
	assert.ErrorContains(t, err, "providers.irancell.get_status.chunk")
}

// TestParseGasInquiryKeys (task 023): caching.gas has its default, takes its value and is checked;
// it does not depend on the water and electricity keys. The gas endpoint of Qabzino is read.
func TestParseGasInquiryKeys(t *testing.T) {
	const base = "mode: dev\ndatabase: {postgres: {password: pw}}\n"
	c, err := Parse([]byte(base))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, time.Hour, c.CachingGas, "default cache TTL (bill: caching.gas 1h)")
	assert.Empty(t, c.QabzinoGasBillEndpoint)

	c, err = Parse([]byte(base + "caching: {gas: \"0s\"}\nproviders: {qabzino: {gas_bill_endpoint: \"http://127.0.0.1:1/gas\"}}\n"))
	if assert.NoError(t, err) {
		assert.Equal(t, time.Duration(0), c.CachingGas, "0 turns the cache off")
		assert.Equal(t, "http://127.0.0.1:1/gas", c.QabzinoGasBillEndpoint)
		assert.Equal(t, time.Hour, c.CachingWater, "the other TTLs keep their defaults")
		assert.Equal(t, time.Hour, c.CachingElectricity)
	}
	c, err = Parse([]byte(base + "caching: {gas: \"90m\", water: \"5m\", electricity: \"7m\"}\n"))
	if assert.NoError(t, err) {
		assert.Equal(t, []time.Duration{90 * time.Minute, 5 * time.Minute, 7 * time.Minute}, []time.Duration{c.CachingGas, c.CachingWater, c.CachingElectricity})
	}
	_, err = Parse([]byte(base + "caching: {gas: \"-1s\"}\n"))
	assert.ErrorContains(t, err, "caching.gas must not be negative")
	_, err = Parse([]byte(base + "caching: {gas: \"soon\"}\n"))
	assert.Error(t, err)
}

// TestParseMCIProviderKeys (task 036): the Jiring Switch and the Parsian keys, caching.mobile and
// the two mocks have their defaults, take their values, and a bad value names its key.
func TestParseMCIProviderKeys(t *testing.T) {
	const base = "mode: dev\ndatabase: {postgres: {password: pw}}\n"
	c, err := Parse([]byte(base))
	if !assert.NoError(t, err) {
		return
	}
	assert.False(t, c.JiringSwitchEnabled, "default: off")
	assert.False(t, c.ParsianEnabled, "default: off")
	assert.Equal(t, []time.Duration{10 * time.Second, 10 * time.Second, 5 * time.Hour},
		[]time.Duration{c.JiringSwitchTimeout, c.ParsianTimeout, c.CachingMobile}, "defaults (bill: caching.mobile 5h)")
	assert.Equal(t, []string{"", ""}, []string{c.JiringSwitchGRPCAddress, c.ParsianGRPCAddress})
	assert.False(t, c.DebugMockJiringSwitchEnable)
	assert.False(t, c.DebugMockParsianEnable)
	assert.Equal(t, []string{MockReturnSuccess, MockReturnSuccess}, []string{c.DebugMockJiringSwitchReturnType, c.DebugMockParsianReturnType})

	c, err = Parse([]byte(base + `providers: {jiring_switch: {enabled: true, timeout: "3s", grpc_address: "jiring-switch:8082"}, parsian: {enabled: true, timeout: "4s", grpc_address: "parsian-v2:8083"}}
caching: {mobile: "0s"}
debug: {mocks: {jiring_switch: {enable: true, return_type: error}, parsian: {enable: true, return_type: success}}}
`))
	if assert.NoError(t, err) {
		assert.True(t, c.JiringSwitchEnabled)
		assert.True(t, c.ParsianEnabled)
		assert.Equal(t, []time.Duration{3 * time.Second, 4 * time.Second, 0},
			[]time.Duration{c.JiringSwitchTimeout, c.ParsianTimeout, c.CachingMobile}, "0 turns the cache off")
		assert.Equal(t, []string{"jiring-switch:8082", "parsian-v2:8083"}, []string{c.JiringSwitchGRPCAddress, c.ParsianGRPCAddress})
		assert.True(t, c.DebugMockJiringSwitchEnable)
		assert.True(t, c.DebugMockParsianEnable)
		assert.Equal(t, []string{MockReturnError, MockReturnSuccess}, []string{c.DebugMockJiringSwitchReturnType, c.DebugMockParsianReturnType})
		assert.Equal(t, time.Hour, c.CachingWater, "the other cache keys keep their defaults")
	}

	for yml, want := range map[string]string{
		"providers: {jiring_switch: {timeout: \"0s\"}}\n":         "providers.jiring_switch.timeout",
		"providers: {parsian: {timeout: \"-1s\"}}\n":              "providers.parsian.timeout",
		"caching: {mobile: \"-1s\"}\n":                            "caching.mobile must not be negative",
		"debug: {mocks: {jiring_switch: {return_type: maybe}}}\n": "debug.mocks.jiring_switch.return_type",
		"debug: {mocks: {parsian: {return_type: maybe}}}\n":       "debug.mocks.parsian.return_type",
	} {
		_, err := Parse([]byte(base + yml))
		assert.ErrorContains(t, err, want, yml)
	}
}

// TestParseKarashopKeys (task 025): the Karashop keys, caching.tel and the Karashop mock have their
// defaults, take their values, and a bad value names its key. The password never prints.
func TestParseKarashopKeys(t *testing.T) {
	const base = "mode: dev\ndatabase: {postgres: {password: pw}}\n"
	c, err := Parse([]byte(base))
	if !assert.NoError(t, err) {
		return
	}
	assert.False(t, c.KarashopEnabled, "default: off")
	assert.Equal(t, []time.Duration{30 * time.Second, time.Hour}, []time.Duration{c.KarashopTimeout, c.CachingTel},
		"defaults (bill: karashop.timeout 30s, caching.tel 1h)")
	assert.Equal(t, []string{"", "", "", ""},
		[]string{c.KarashopCode, c.KarashopPassword.Reveal(), c.KarashopBillInquiryURL, c.KarashopBillPaymentInfoURL})
	assert.False(t, c.DebugMockKarashopEnable)
	assert.Equal(t, MockReturnSuccess, c.DebugMockKarashopReturnType)

	c, err = Parse([]byte(base + `providers: {karashop: {enabled: true, timeout: "5s", code: "c1", password: "p@ss", bill_inquiry_url: "https://127.0.0.1:1/in", bill_payment_info_url: "https://127.0.0.1:1/pay"}}
caching: {tel: "0s"}
debug: {mocks: {karashop: {enable: true, return_type: error}}}
`))
	if assert.NoError(t, err) {
		assert.True(t, c.KarashopEnabled)
		assert.Equal(t, []time.Duration{5 * time.Second, 0}, []time.Duration{c.KarashopTimeout, c.CachingTel}, "0 turns the cache off")
		assert.Equal(t, []string{"c1", "p@ss", "https://127.0.0.1:1/in", "https://127.0.0.1:1/pay"},
			[]string{c.KarashopCode, c.KarashopPassword.Reveal(), c.KarashopBillInquiryURL, c.KarashopBillPaymentInfoURL})
		assert.NotContains(t, fmt.Sprint(c.KarashopPassword), "p@ss", "the password never prints")
		assert.True(t, c.DebugMockKarashopEnable)
		assert.Equal(t, MockReturnError, c.DebugMockKarashopReturnType)
		assert.Equal(t, []time.Duration{time.Hour, 5 * time.Hour}, []time.Duration{c.CachingWater, c.CachingMobile}, "the other cache keys keep their defaults")
	}

	for yml, want := range map[string]string{
		"providers: {karashop: {timeout: \"0s\"}}\n":         "providers.karashop.timeout",
		"caching: {tel: \"-1s\"}\n":                          "caching.tel must not be negative",
		"debug: {mocks: {karashop: {return_type: maybe}}}\n": "debug.mocks.karashop.return_type",
	} {
		_, err := Parse([]byte(base + yml))
		assert.ErrorContains(t, err, want, yml)
	}
}

// TestParseJiringSwitchAndParsianBillKeys (task 036): bill's keys of the jiring-switch and the
// parsian-v2 service clients have no default, take their values, and a wrong type names the key.
func TestParseJiringSwitchAndParsianBillKeys(t *testing.T) {
	const base = "mode: dev\ndatabase: {postgres: {password: pw}}\n"
	c, err := Parse([]byte(base))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{"", "", "", ""}, []string{c.JiringSwitchName, c.JiringSwitchGetStatusName, c.ParsianName, c.ParsianGetStatusName})
	for _, d := range []time.Duration{
		c.JiringSwitchGetStatusStart, c.JiringSwitchGetStatusEnd, c.JiringSwitchGetStatusCronTime, c.JiringSwitchGetStatusLockTime,
		c.JiringSwitchGetStatusReverseDeadline, c.JiringSwitchGetStatusSyncAPICall, c.JiringSwitchGetStatusSyncGetOrder, c.JiringSwitchGetStatusSyncInsertOrder,
		c.ParsianGetStatusStart, c.ParsianGetStatusEnd, c.ParsianGetStatusCronTime, c.ParsianGetStatusLockTime,
		c.ParsianGetStatusReverseDeadline, c.ParsianGetStatusSyncAPICall, c.ParsianGetStatusSyncGetOrder, c.ParsianGetStatusSyncInsertOrder,
		c.ParsianSettlementInquiryWater, c.ParsianSettlementInquiryElectricity, c.ParsianSettlementInquiryGas,
		c.ParsianSettlementInquiryTelephone, c.ParsianSettlementInquiryMCIMobile, c.ParsianSettlementInquiryRightelMobile,
	} {
		assert.Zero(t, d)
	}
	assert.Zero(t, c.JiringSwitchGetStatusChunk)
	assert.Zero(t, c.ParsianGetStatusChunk)
	assert.False(t, c.JiringSwitchGetStatusIsLockable)
	assert.False(t, c.ParsianGetStatusIsLockable)

	status := `get_status: {name: "%s-getstatus", chunk: 100, start: "30m", end: "10h", cron_time: "15m", lock_time: "30m", is_lockable: true, reverse_deadline: "10h", sync_operations: {api_call_get_status: "30s", get_order_query: "3ms", insert_order_query: "4ms"}}`
	yml := base + "providers:\n" +
		"  jiring_switch: {name: \"jiring-switch\", grpc_address: \"jiring-switch:8084\", " + fmt.Sprintf(status, "jiring-switch") + "}\n" +
		"  parsian: {name: \"parsian\", grpc_address: \"parsian-v2:8084\", " + fmt.Sprintf(status, "parsian") +
		", settlement_inquiry_water: \"360h\", settlement_inquiry_electricity: \"361h\", settlement_inquiry_gas: \"362h\", settlement_inquiry_telephone: \"363h\", settlement_inquiry_mci_mobile: \"0s\", settlement_inquiry_rightel_mobile: \"364h\"}\n"
	c, err = Parse([]byte(yml))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{"jiring-switch", "jiring-switch:8084", "jiring-switch-getstatus", "parsian", "parsian-v2:8084", "parsian-getstatus"},
		[]string{c.JiringSwitchName, c.JiringSwitchGRPCAddress, c.JiringSwitchGetStatusName, c.ParsianName, c.ParsianGRPCAddress, c.ParsianGetStatusName})
	want := []time.Duration{30 * time.Minute, 10 * time.Hour, 15 * time.Minute, 30 * time.Minute, 10 * time.Hour, 30 * time.Second, 3 * time.Millisecond, 4 * time.Millisecond}
	assert.Equal(t, want, []time.Duration{c.JiringSwitchGetStatusStart, c.JiringSwitchGetStatusEnd, c.JiringSwitchGetStatusCronTime, c.JiringSwitchGetStatusLockTime,
		c.JiringSwitchGetStatusReverseDeadline, c.JiringSwitchGetStatusSyncAPICall, c.JiringSwitchGetStatusSyncGetOrder, c.JiringSwitchGetStatusSyncInsertOrder})
	assert.Equal(t, want, []time.Duration{c.ParsianGetStatusStart, c.ParsianGetStatusEnd, c.ParsianGetStatusCronTime, c.ParsianGetStatusLockTime,
		c.ParsianGetStatusReverseDeadline, c.ParsianGetStatusSyncAPICall, c.ParsianGetStatusSyncGetOrder, c.ParsianGetStatusSyncInsertOrder})
	assert.Equal(t, []time.Duration{360 * time.Hour, 361 * time.Hour, 362 * time.Hour, 363 * time.Hour, 0, 364 * time.Hour},
		[]time.Duration{c.ParsianSettlementInquiryWater, c.ParsianSettlementInquiryElectricity, c.ParsianSettlementInquiryGas,
			c.ParsianSettlementInquiryTelephone, c.ParsianSettlementInquiryMCIMobile, c.ParsianSettlementInquiryRightelMobile})
	assert.Equal(t, []int{100, 100}, []int{c.JiringSwitchGetStatusChunk, c.ParsianGetStatusChunk})
	assert.True(t, c.JiringSwitchGetStatusIsLockable)
	assert.True(t, c.ParsianGetStatusIsLockable)

	_, err = Parse([]byte(base + "providers: {parsian: {settlement_inquiry_gas: \"soon\"}}\n"))
	assert.ErrorContains(t, err, "providers.parsian.settlement_inquiry_gas")
	_, err = Parse([]byte(base + "providers: {jiring_switch: {get_status: {chunk: \"many\"}}}\n"))
	assert.ErrorContains(t, err, "providers.jiring_switch.get_status.chunk")
}

// TestParseCacheInquiries (task 031): debug.cache_inquiries is true by default, takes false in dev,
// and is ignored (true) in a mode other than dev (task 020c removes the debug section).
func TestParseCacheInquiries(t *testing.T) {
	const base = "database: {postgres: {password: pw}}\n"
	for _, mode := range []string{ModeDev, ModeStage, ModeProd} {
		c, err := Parse([]byte("mode: " + mode + "\n" + base))
		if assert.NoError(t, err, mode) {
			assert.True(t, c.DebugCacheInquiries, "default, %s", mode)
		}
	}
	const off = "debug: {cache_inquiries: false}\n"
	c, err := Parse([]byte("mode: dev\n" + base + off))
	if assert.NoError(t, err) {
		assert.False(t, c.DebugCacheInquiries, "false in dev")
	}
	c, err = Parse([]byte("mode: dev\n" + base + "debug: {cache_inquiries: true}\n"))
	if assert.NoError(t, err) {
		assert.True(t, c.DebugCacheInquiries)
	}
	for _, mode := range []string{ModeStage, ModeProd} {
		c, err := Parse([]byte("mode: " + mode + "\n" + base + off))
		if assert.NoError(t, err, mode) {
			assert.True(t, c.DebugCacheInquiries, "ignored outside dev, %s", mode)
		}
	}
	_, err = Parse([]byte("mode: dev\n" + base + "debug: {cache_inquiries: \"no\"}\n"))
	assert.Error(t, err, "not a bool")
}
