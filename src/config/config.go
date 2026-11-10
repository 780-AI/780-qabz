// Package config maps a YAML file to the typed Config.
package config

import (
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
	"uuid"

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
	// Mode is the run environment: ModeDev, ModeStage or ModeProd. The key is required: a missing
	// key or any other value stops the start. A mode other than ModeDev fully removes the debug
	// section (task 020c). The development commands (devdb) run only in ModeDev.
	Mode        string `path:"mode"`
	ServiceName string `path:"service_name"`
	// ServiceID is the ID of this service in the payment flow. GET /v1/services sends it as
	// serviceId. The old bill service has the same key (service.id); qabz uses bill's ID.
	// 0 to 2147483647 (int32 in the proto).
	ServiceID int    `path:"service.id"`
	LogLevel  string `path:"logs.level"`

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

	// JWTHMACSecret is the secret that the auth service signs the user JWT with (HS256, HS384 or
	// HS512). The raw string is the key. It sits in the main config with the other auth secrets
	// (auth.secrets). Qabz only verifies tokens with it. Required while the auth check is on.
	JWTHMACSecret Secret `path:"auth.secrets.hmac"`
	// DebugAuthDisable turns the auth check off. Development only: a mode other than dev removes
	// the debug section, and a missing key is false (the check is on).
	DebugAuthDisable bool `path:"debug.auth.disable"`
	// DebugAuthMockUserID is the user of every call while the auth check is off (debug.auth.disable):
	// the handlers that need a user (for example My Bills) get it as the token sub. A UUID in the
	// canonical lower-case form, or "" for no user (those handlers then answer PermissionDenied).
	// It has no effect while the check is on. Development only.
	DebugAuthMockUserID string `path:"debug.auth.mock_user_id"`

	MetricsAddr string `path:"metrics.prometheus.addr"`
	// MetricsEnabled is the Prometheus switch (Implementation_Plan Step 5). It is on by default and
	// off only with an explicit false: then no metric is registered or recorded and the metrics
	// endpoint does not listen.
	MetricsEnabled bool   `path:"metrics.prometheus.enabled"`
	OTLPEndpoint   string `path:"logs.otlp_endpoint"` // empty: tracing is off

	// HotReloadInterval is how often the hot_reload:"true" fields are read again from Postgres.
	HotReloadInterval time.Duration `path:"hot_reload_interval"`
	// MigrateOnStart runs the SQL migrations at app start.
	MigrateOnStart bool `path:"database.migration.on_start"`
	// MigrateTimeout limits the wait for the migration lock plus the migration run.
	MigrateTimeout time.Duration `path:"database.migration.timeout"`

	// ProvidersLoadInterval is how often ProvidersService loads the providers table again
	// (task 020). At least 1s. Hot-reloadable (key providers_load_interval, type duration, in
	// app_hot_configs): the reload routine reads the current value before each wait.
	ProvidersLoadInterval time.Duration `path:"providers.general.load_interval" hot_reload:"true"`

	// QabzinoEnabled is providers.qabzino.enabled (task 030). False (the default): Qabzino cannot be
	// used: no call is sent to Ayantech, every Qabzino inquiry fails as "provider down", and the
	// Qabzino mock does not bypass the switch. Not hot-reloadable: the adapter is built at start.
	QabzinoEnabled bool `path:"providers.qabzino.enabled"`
	// QabzinoGRPCAddress is the host:port of the old qabzino service (gRPC, no TLS, as in bill:
	// qabzino.grpcAddress). Since task 030 only QabzinoProxy (package provider/qabzinoproxy, not
	// used) reads it; the App does not.
	QabzinoGRPCAddress string `path:"providers.qabzino.grpc_address"`
	// QabzinoTimeout limits each call to the Qabzino backend. Positive. Not hot-reloadable.
	QabzinoTimeout time.Duration `path:"providers.qabzino.timeout"`
	// QabzinoToken is the Ayantech token of the Qabzino provider (bill: qabzino.token). A Secret:
	// it never prints. Empty by default; the real value is only in config.yml (task 029). Read by
	// the direct Qabzino adapter (task 030).
	QabzinoToken Secret `path:"providers.qabzino.token"`
	// QabzinoElectricityBillEndpoint, QabzinoGasBillEndpoint and QabzinoWaterBillEndpoint are the
	// Ayantech inquiry URLs (bill: qabzino.electricityBillEndpoint and so on). Not checked. Only the
	// water one is read (task 030); the electricity and gas ones are unused so far.
	QabzinoElectricityBillEndpoint string `path:"providers.qabzino.electricity_bill_endpoint"`
	QabzinoGasBillEndpoint         string `path:"providers.qabzino.gas_bill_endpoint"`
	QabzinoWaterBillEndpoint       string `path:"providers.qabzino.water_bill_endpoint"`

	// TavanirGRPCAddress is the host:port of the Tavanir backend (gRPC, no TLS, as in bill:
	// tavanir.grpcAddress). Empty: every call to Tavanir fails as "provider down" (task 022). Not
	// hot-reloadable: the client is built at start.
	TavanirGRPCAddress string `path:"providers.tavanir.grpc_address"`
	// TavanirTimeout limits each call to the Tavanir backend. Positive. Not hot-reloadable.
	TavanirTimeout time.Duration `path:"providers.tavanir.timeout"`

	// CachingWater is how long a saved water inquiry answers the same bill again with no provider
	// call (bill: caching.water, 1h). 0 turns the cache off; a negative value is rejected. Not
	// hot-reloadable (task 021, decision 1): read at start.
	CachingWater time.Duration `path:"caching.water"`
	// CachingElectricity is how long a saved electricity inquiry answers the same bill again with
	// no provider call (bill: caching.electricity, 1h). 0 turns the cache off; a negative value is
	// rejected. Not hot-reloadable (task 022, as caching.water): read at start.
	CachingElectricity time.Duration `path:"caching.electricity"`

	// DebugMockQabzinoEnable makes the Qabzino adapter answer with a fixed answer, with no call to
	// the backend (task 021). Development only: a mode other than dev removes the debug section, so
	// the key is false there (and the app ignores it outside dev again). Not hot-reloadable.
	DebugMockQabzinoEnable bool `path:"debug.mocks.qabzino.enable"`
	// DebugMockQabzinoReturnType is the answer of the Qabzino mock: MockReturnSuccess or
	// MockReturnError. It has no effect while the mock is off. Development only.
	DebugMockQabzinoReturnType string `path:"debug.mocks.qabzino.return_type"`

	// DebugMockTavanirEnable makes the Tavanir adapter answer with a fixed answer, with no call to
	// the backend (task 022). Development only, as DebugMockQabzinoEnable. Not hot-reloadable.
	DebugMockTavanirEnable bool `path:"debug.mocks.tavanir.enable"`
	// DebugMockTavanirReturnType is the answer of the Tavanir mock: MockReturnSuccess or
	// MockReturnError. It has no effect while the mock is off. Development only.
	DebugMockTavanirReturnType string `path:"debug.mocks.tavanir.return_type"`
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
		MetricsEnabled:    true,
		HotReloadInterval: 30 * time.Second,
		MigrateOnStart:    true,
		MigrateTimeout:    60 * time.Second,

		ProvidersLoadInterval: time.Minute,

		QabzinoTimeout:                 10 * time.Second,
		QabzinoElectricityBillEndpoint: "https://core.inquiry.ayantech.ir/webservices/core.svc/ElectricityBillInquiry",
		QabzinoGasBillEndpoint:         "https://core.inquiry.ayantech.ir/webservices/core.svc/GasBillInquiry",
		QabzinoWaterBillEndpoint:       "https://core.inquiry.ayantech.ir/webservices/core.svc/WaterBillInquiry",
		TavanirTimeout:                 10 * time.Second,
		CachingWater:                   time.Hour,
		CachingElectricity:             time.Hour,
		DebugMockQabzinoReturnType:     MockReturnSuccess,
		DebugMockTavanirReturnType:     MockReturnSuccess,
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

// Parse reads a YAML document into a Config. When mode is not dev, it first removes the debug
// section, so every debug.* field keeps its default. Paths that are missing keep their default; the
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
	// Task 020c: a mode other than dev fully removes the debug section. Every debug.* field then
	// keeps its default, and no value of the section is read or checked.
	if mode, _ := m["mode"].(string); mode != ModeDev {
		delete(m, debugSection)
	}
	c := defaults()
	if err := decodeStruct(m, reflect.ValueOf(&c).Elem(), ""); err != nil {
		return Config{}, err
	}
	return c, c.validate()
}

func (c Config) validate() error {
	var errs []error
	switch c.Mode {
	case ModeDev, ModeStage, ModeProd:
	case "":
		errs = append(errs, errors.New("mode is required (dev, stage or prod)"))
	default:
		errs = append(errs, fmt.Errorf("mode must be dev, stage or prod (got %q)", c.Mode))
	}
	if c.PostgresPassword == "" {
		errs = append(errs, errors.New("database.postgres.password is required"))
	}
	if c.ServiceID < 0 || c.ServiceID > math.MaxInt32 {
		errs = append(errs, fmt.Errorf("service.id must be from 0 to %d", math.MaxInt32))
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
	if c.ProvidersLoadInterval < ProvidersMinLoadInterval {
		errs = append(errs, fmt.Errorf("providers.general.load_interval must be at least %s", ProvidersMinLoadInterval))
	}
	if c.QabzinoTimeout <= 0 {
		errs = append(errs, errors.New("providers.qabzino.timeout must be a positive duration (for example 10s)"))
	}
	if c.TavanirTimeout <= 0 {
		errs = append(errs, errors.New("providers.tavanir.timeout must be a positive duration (for example 10s)"))
	}
	if c.CachingWater < 0 {
		errs = append(errs, errors.New("caching.water must not be negative (0 turns the cache off)"))
	}
	if c.CachingElectricity < 0 {
		errs = append(errs, errors.New("caching.electricity must not be negative (0 turns the cache off)"))
	}
	if !ValidMockReturnType(c.DebugMockQabzinoReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.qabzino.return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if !ValidMockReturnType(c.DebugMockTavanirReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.tavanir.return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if c.DebugAuthMockUserID != "" {
		if u, err := uuid.Parse(c.DebugAuthMockUserID); err != nil || u.String() != c.DebugAuthMockUserID {
			errs = append(errs, errors.New("debug.auth.mock_user_id must be a UUID in the lower-case form xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"))
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
