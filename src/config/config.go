// Package config maps a YAML file to the typed Config.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Secret is a string that never prints its value.
type Secret string

// String returns a mask, so a Secret is safe in any log line or format verb.
func (Secret) String() string { return "***" }

// GoString masks the value for %#v too.
func (Secret) GoString() string { return `"***"` }

// Reveal returns the real value. Use it only to build a connection.
func (s Secret) Reveal() string { return string(s) }

// Config holds every setting of the service. A field with the tag `path:"a.b.c"` gets the value at
// that dotted path of the YAML tree (see decode.go). A path that is missing keeps the default.
type Config struct {
	ServiceName string `path:"service_name"`
	LogLevel    string `path:"logs.level"`

	PostgresHost     string `path:"database.postgres.host"`
	PostgresPort     int    `path:"database.postgres.port"`
	PostgresUser     string `path:"database.postgres.user"`
	PostgresPassword Secret `path:"database.postgres.password"`
	PostgresDB       string `path:"database.postgres.db"`

	RedisHost     string `path:"database.redis.host"`
	RedisPort     int    `path:"database.redis.port"`
	RedisPassword Secret `path:"database.redis.password"`

	// GRPCAddr is the listen address of the gRPC server (host:port; port 0 picks a free port).
	GRPCAddr string `path:"server.grpc.addr"`
	// HTTPAddr is the listen address of the grpc-gateway HTTP server (host:port; port 0 picks a free port).
	HTTPAddr string `path:"server.http.addr"`

	// JWTPublicKeyPath is the PEM file of the auth service's ES512 public key. Qabz only verifies
	// tokens with it. Required while the auth check is on.
	JWTPublicKeyPath string `path:"auth.jwt.public_key_path"`
	// JWTAudience is the value that the token's aud claim must contain. Required while the auth check is on.
	JWTAudience string `path:"auth.jwt.audience"`
	// DebugAuthDisable turns the auth check off. Development only: the production config has no
	// debug section, and a missing key is false (the check is on).
	DebugAuthDisable bool `path:"debug.auth.disable"`

	MetricsAddr  string `path:"metrics.prometheus.addr"`
	OTLPEndpoint string `path:"logs.otlp_endpoint"` // empty: tracing is off

	// HotReloadInterval is how often the hot_reload:"true" fields are read again from Postgres.
	HotReloadInterval time.Duration `path:"hot_reload_interval"`
	// MigrateOnStart runs the SQL migrations at app start.
	MigrateOnStart bool `path:"database.migration.on_start"`
	// MigrateTimeout limits the wait for the migration lock plus the migration run.
	MigrateTimeout time.Duration `path:"database.migration.timeout"`
}

func defaults() Config {
	return Config{
		ServiceName:       "qabz",
		LogLevel:          "info",
		PostgresHost:      "localhost",
		PostgresPort:      5432,
		PostgresUser:      "qabz",
		PostgresDB:        "qabz",
		RedisHost:         "localhost",
		RedisPort:         6379,
		GRPCAddr:          ":8082",
		HTTPAddr:          ":8080",
		MetricsAddr:       ":9090",
		HotReloadInterval: 30 * time.Second,
		MigrateOnStart:    true,
		MigrateTimeout:    60 * time.Second,
	}
}

// LoadFile reads the YAML file at path into a Config. A missing or unreadable file is an error.
func LoadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config file: %w", err)
	}
	return Parse(data)
}

// Parse reads a YAML document into a Config. Paths that are missing keep their default; the
// rules (required password, port range, positive durations) are checked after the mapping.
// It returns all rule errors together.
func Parse(data []byte) (Config, error) {
	var root any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return Config{}, fmt.Errorf("config yaml: %w", err)
	}
	if root == nil { // an empty file
		root = map[string]any{}
	}
	m, ok := root.(map[string]any)
	if !ok {
		return Config{}, errors.New("config yaml: the document must be an object")
	}
	c := defaults()
	if err := decodeStruct(m, reflect.ValueOf(&c).Elem(), ""); err != nil {
		return Config{}, err
	}
	return c, c.validate()
}

func (c Config) validate() error {
	var errs []error
	if c.PostgresPassword == "" {
		errs = append(errs, errors.New("database.postgres.password is required"))
	}
	for path, n := range map[string]int{"database.postgres.port": c.PostgresPort, "database.redis.port": c.RedisPort} {
		if n < 1 || n > 65535 {
			errs = append(errs, fmt.Errorf("%s must be a port number (1-65535)", path))
		}
	}
	for _, a := range []struct{ path, addr string }{{"server.grpc.addr", c.GRPCAddr}, {"server.http.addr", c.HTTPAddr}} {
		if err := checkListenAddr(a.addr); err != nil {
			errs = append(errs, fmt.Errorf("%s %w", a.path, err))
		}
	}
	if c.GRPCAddr == c.HTTPAddr && checkListenAddr(c.GRPCAddr) == nil && !strings.HasSuffix(c.GRPCAddr, ":0") {
		errs = append(errs, errors.New("server.grpc.addr and server.http.addr must not be the same address"))
	}
	for path, d := range map[string]time.Duration{"hot_reload_interval": c.HotReloadInterval, "database.migration.timeout": c.MigrateTimeout} {
		if d <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive duration (for example 30s)", path))
		}
	}
	return errors.Join(errs...)
}

// checkListenAddr checks that addr is host:port with a port from 0 to 65535 (0 picks a free port).
func checkListenAddr(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("must be host:port (for example :8080)")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return errors.New("must have a port from 0 to 65535")
	}
	return nil
}

// PostgresDSN builds the connection string. It contains the password: never log it.
func (c Config) PostgresDSN() string {
	u := url.URL{
		Scheme:  "postgres",
		User:    url.UserPassword(c.PostgresUser, c.PostgresPassword.Reveal()),
		Host:    net.JoinHostPort(c.PostgresHost, strconv.Itoa(c.PostgresPort)),
		Path:    "/" + c.PostgresDB,
		RawPath: "/" + url.PathEscape(c.PostgresDB), // keeps a "/" in the name escaped as %2F
	}
	return u.String()
}

// RedisAddr returns host:port.
func (c Config) RedisAddr() string {
	return net.JoinHostPort(c.RedisHost, strconv.Itoa(c.RedisPort))
}

// PostgresTarget returns host:port/db for logs. It has no user and no password.
func (c Config) PostgresTarget() string {
	return net.JoinHostPort(c.PostgresHost, strconv.Itoa(c.PostgresPort)) + "/" + c.PostgresDB
}
