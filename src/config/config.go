// Package config maps environment variables (ENV) to the typed Config.
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Secret is a string that never prints its value.
type Secret string

// String returns a mask, so a Secret is safe in any log line or format verb.
func (Secret) String() string { return "***" }

// GoString masks the value for %#v too.
func (Secret) GoString() string { return `"***"` }

// Reveal returns the real value. Use it only to build a connection.
func (s Secret) Reveal() string { return string(s) }

// Lookup reads one environment variable. os.LookupEnv is the production value.
type Lookup func(key string) (string, bool)

// Config holds every setting of the service.
type Config struct {
	ServiceName string
	LogLevel    string

	PostgresHost     string
	PostgresPort     int
	PostgresUser     string
	PostgresPassword Secret
	PostgresDB       string

	RedisHost     string
	RedisPort     int
	RedisPassword Secret

	MetricsAddr  string
	OTLPEndpoint string // empty: tracing is off

	// HotReloadInterval is how often the hot_reload:"true" fields are read again from Postgres.
	HotReloadInterval time.Duration
	// MigrateOnStart runs the SQL migrations at app start.
	MigrateOnStart bool
	// MigrateTimeout limits the wait for the migration lock plus the migration run.
	MigrateTimeout time.Duration
}

// Load reads the Config from the process environment.
func Load() (Config, error) { return LoadFrom(os.LookupEnv) }

// LoadFrom reads the Config with lookup. It returns all errors together.
func LoadFrom(lookup Lookup) (Config, error) {
	r := reader{lookup: lookup}
	c := Config{
		ServiceName:       r.str("SERVICE_NAME", "qabz"),
		LogLevel:          r.str("LOG_LEVEL", "info"),
		PostgresHost:      r.str("POSTGRES_HOST", "localhost"),
		PostgresPort:      r.port("POSTGRES_PORT", 5432),
		PostgresUser:      r.str("POSTGRES_USER", "qabz"),
		PostgresPassword:  Secret(r.required("POSTGRES_PASSWORD")),
		PostgresDB:        r.str("POSTGRES_DB", "qabz"),
		RedisHost:         r.str("REDIS_HOST", "localhost"),
		RedisPort:         r.port("REDIS_PORT", 6379),
		RedisPassword:     Secret(r.str("REDIS_PASSWORD", "")),
		MetricsAddr:       r.str("METRICS_ADDR", ":9090"),
		OTLPEndpoint:      r.str("OTLP_ENDPOINT", ""),
		HotReloadInterval: r.duration("HOT_RELOAD_INTERVAL", 30*time.Second),
		MigrateOnStart:    r.boolean("MIGRATE_ON_START", true),
		MigrateTimeout:    r.duration("MIGRATE_TIMEOUT", 60*time.Second),
	}
	return c, r.err()
}

// PostgresDSN builds the connection string. It contains the password: never log it.
func (c Config) PostgresDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s/%s",
		urlEscape(c.PostgresUser), urlEscape(c.PostgresPassword.Reveal()),
		net.JoinHostPort(c.PostgresHost, strconv.Itoa(c.PostgresPort)), urlEscape(c.PostgresDB))
}

// RedisAddr returns host:port.
func (c Config) RedisAddr() string {
	return net.JoinHostPort(c.RedisHost, strconv.Itoa(c.RedisPort))
}

// PostgresTarget returns host:port/db for logs. It has no user and no password.
func (c Config) PostgresTarget() string {
	return net.JoinHostPort(c.PostgresHost, strconv.Itoa(c.PostgresPort)) + "/" + c.PostgresDB
}

func urlEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch >= 'a' && ch <= 'z', ch >= 'A' && ch <= 'Z', ch >= '0' && ch <= '9',
			ch == '-', ch == '_', ch == '.', ch == '~':
			b.WriteByte(ch)
		default:
			fmt.Fprintf(&b, "%%%02X", ch)
		}
	}
	return b.String()
}

type reader struct {
	lookup Lookup
	errs   []error
}

func (r *reader) str(key, def string) string {
	if v, ok := r.lookup(key); ok && v != "" {
		return v
	}
	return def
}

func (r *reader) required(key string) string {
	v, ok := r.lookup(key)
	if !ok || v == "" {
		r.errs = append(r.errs, fmt.Errorf("%s is required", key))
	}
	return v
}

func (r *reader) port(key string, def int) int {
	v, ok := r.lookup(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		r.errs = append(r.errs, fmt.Errorf("%s must be a port number (1-65535)", key))
		return def
	}
	return n
}

func (r *reader) err() error { return errors.Join(r.errs...) }

func (r *reader) duration(key string, def time.Duration) time.Duration {
	v, ok := r.lookup(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		r.errs = append(r.errs, fmt.Errorf("%s must be a positive duration (for example 30s)", key))
		return def
	}
	return d
}

func (r *reader) boolean(key string, def bool) bool {
	v, ok := r.lookup(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		r.errs = append(r.errs, fmt.Errorf("%s must be true or false", key))
		return def
	}
	return b
}
