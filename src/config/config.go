// Package config maps a YAML file to the typed Config.
package config

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"net"
	"net/url"
	"os"
	"reflect"
	"slices"
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
	Mode string `path:"mode"`
	// ServiceName is the name of the service: the service field of every log line, the tracing
	// service name and the kid header of the order token. In the service block with service.id
	// (operator, 2026-11-14; it was the top-level key service_name).
	ServiceName string `path:"service.name"`
	// ServiceID is the ID of this service in the payment flow. GET /v1/services sends it as
	// serviceId. The old bill service has the same key (service.id); qabz uses bill's ID.
	// 0 to 2147483647 (int32 in the proto).
	ServiceID int `path:"service.id"`
	// ServiceEcosystemName is the name of the service that the other services of the ecosystem see
	// (task 045 D17): the key and the client ID of every Kafka message and currentStep of the
	// OrderComplete answer. Default "bill": Qabz is not known in the ecosystem yet, and the deployed
	// bill sends its name, bill. service.name stays the name of the logs and the tracing. Not empty.
	ServiceEcosystemName string `path:"service.ecosystem_name"`
	LogLevel             string `path:"logs.level"`

	PostgresHost     string `path:"database.postgres.host"`
	PostgresPort     int    `path:"database.postgres.port"`
	PostgresUser     string `path:"database.postgres.user"`
	PostgresPassword Secret `path:"database.postgres.password"`
	PostgresDB       string `path:"database.postgres.db"`

	RedisHost     string `path:"database.redis.host"`
	RedisPort     int    `path:"database.redis.port"`
	RedisPassword Secret `path:"database.redis.password"`
	// RedisPrefix is put in front of every Redis key of Qabz by the Redis client (task 045,
	// supertask 043), so a Redis that other services share cannot mix the keys. Default "qabz:".
	RedisPrefix string `path:"database.redis.prefix"`

	// GRPCAddr is the listen address of the gRPC server (host:port; port 0 picks a free port).
	GRPCAddr string `path:"server.grpc.addr"`
	// HTTPAddr is the listen address of the grpc-gateway HTTP server (host:port; port 0 picks a free port).
	HTTPAddr string `path:"server.http.addr"`
	// InternalGRPCAddr is the listen address of the internal gRPC server (task 044; bill:
	// service.internalGrpc): the payment service calls PaymentComplete/OrderComplete on it. It has
	// no auth check and no HTTP gateway, as in bill. Empty (the default): no internal server, so
	// OrderComplete is not served. host:port (port 0 picks a free port). Not hot-reloadable.
	InternalGRPCAddr string `path:"server.internal_grpc.addr"`

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
	MetricsEnabled bool `path:"metrics.prometheus.enabled"`
	// TracingEnabled is the tracing switch (task 051; conventions § 17). False (the default): no
	// span, no traceparent header, no connection, also when TracingOTLPEndpoint is set. True needs
	// TracingOTLPEndpoint (validate). Read at start.
	TracingEnabled bool `path:"logs.tracing.enabled"`
	// TracingOTLPEndpoint is the host:port (no scheme) of the OTLP gRPC receiver of Jaeger, for
	// example "localhost:4317" (task 051; was logs.otlp_endpoint). Read at start.
	TracingOTLPEndpoint string `path:"logs.tracing.otlp_endpoint"`

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

	// QabzinoEnabled is providers.external.qabzino.enabled (task 030). False (the default): Qabzino cannot be
	// used: no call is sent to Ayantech, every Qabzino inquiry fails as "provider down", and the
	// Qabzino mock does not bypass the switch. Not hot-reloadable: the adapter is built at start.
	QabzinoEnabled bool `path:"providers.external.qabzino.enabled"`
	// QabzinoGRPCAddress is the host:port of the old qabzino service (gRPC, no TLS, as in bill:
	// qabzino.grpcAddress). Since task 030 only QabzinoProxy (package provider/qabzinoproxy, not
	// used) reads it; the App does not.
	QabzinoGRPCAddress string `path:"providers.external.qabzino.grpc_address"`
	// QabzinoTimeout limits each call to the Qabzino backend. Positive. Not hot-reloadable.
	QabzinoTimeout time.Duration `path:"providers.external.qabzino.timeout"`
	// QabzinoToken is the Ayantech token of the Qabzino provider (bill: qabzino.token). A Secret:
	// it never prints. Empty by default; the real value is only in config.yml (task 029). Read by
	// the direct Qabzino adapter (task 030).
	QabzinoToken Secret `path:"providers.external.qabzino.token"`
	// QabzinoElectricityBillEndpoint, QabzinoGasBillEndpoint, QabzinoRightelBillEndpoint and
	// QabzinoWaterBillEndpoint are the Ayantech inquiry URLs (bill: qabzino.electricityBillEndpoint
	// and so on; the qabzino service: rightelBillEndpoint). Not checked. The water one (task 030),
	// the gas one (task 023) and the Rightel one (task 037) are read; the electricity one is unused so far.
	// No default (task 036, operator): the real URLs are only in config.yml; an empty URL makes
	// every call fail as "provider down".
	QabzinoElectricityBillEndpoint string `path:"providers.external.qabzino.electricity_bill_endpoint"`
	QabzinoGasBillEndpoint         string `path:"providers.external.qabzino.gas_bill_endpoint"`
	QabzinoRightelBillEndpoint     string `path:"providers.external.qabzino.rightel_bill_endpoint"`
	QabzinoWaterBillEndpoint       string `path:"providers.external.qabzino.water_bill_endpoint"`

	// TavanirEnabled turns the Tavanir provider on (task 034). False (the default): every call to
	// Tavanir fails as "provider down", nothing is sent to Tavanir and the mock
	// (debug.mocks.tavanir) does not apply. Not hot-reloadable.
	TavanirEnabled bool `path:"providers.external.tavanir.enabled"`
	// TavanirTimeout limits each HTTP call to Tavanir (the login and the inquiry each). Positive. Not hot-reloadable.
	TavanirTimeout time.Duration `path:"providers.external.tavanir.timeout"`
	// The config of the Tavanir adapter (task 033; the tavanir service: tavanir.authEndpoint and so
	// on). The adapter (task 034) uses auth_endpoint, basic_branch_endpoint, username and password;
	// bill_document_endpoint and branch_debit_endpoint are not used yet. The password is a Secret
	// (never printed). The endpoints have no default (task 036, operator): the real URLs are only in
	// config.yml; an empty auth or basic branch URL makes every call fail as "provider down".
	TavanirAuthEndpoint         string `path:"providers.external.tavanir.auth_endpoint"`
	TavanirBasicBranchEndpoint  string `path:"providers.external.tavanir.basic_branch_endpoint"`
	TavanirBillDocumentEndpoint string `path:"providers.external.tavanir.bill_document_endpoint"`
	TavanirBranchDebitEndpoint  string `path:"providers.external.tavanir.branch_debit_endpoint"`
	TavanirUsername             string `path:"providers.external.tavanir.username"`
	TavanirPassword             Secret `path:"providers.external.tavanir.password"`
	// The circuit breaker (task 060; system.html, Provider Switcher): it counts the provider calls of
	// each provider row (provider, bill type, action) in a sliding window and asks the Provider
	// Switcher to demote the row when the row has CircuitBreakerMinCalls calls or more and
	// CircuitBreakerFailureRatio percent or more of them were provider down. It never blocks a call.
	// All keys are hot-reloadable (the key in app_hot_configs is the field name in snake case, for
	// example circuit_breaker_window). Validate checks the start values; a hot value out of range is
	// ignored by the App (an override falls back to the general value, a general value to its
	// default) with one warn line at the change.
	//   - CircuitBreakerEnabled false (the default): no count and no demotion (the behavior before
	//     task 060). A row that is demoted when it goes to false returns by its trial.
	//   - CircuitBreakerFailureRatio is the percent of failed calls (1 to 100) that opens the row.
	//   - CircuitBreakerWindow is the length of the sliding window. Positive.
	//   - CircuitBreakerMinCalls is the smallest number of calls in the window for which the ratio is
	//     trusted (not a number of failed calls). 1 or more.
	//   - CircuitBreakerOpenTime is the time a row stays demoted before its trial inquiry. Positive.
	CircuitBreakerEnabled      bool          `path:"providers.general.circuit_breaker.enabled" hot_reload:"true"`
	CircuitBreakerFailureRatio int           `path:"providers.general.circuit_breaker.failure_ratio" hot_reload:"true"`
	CircuitBreakerWindow       time.Duration `path:"providers.general.circuit_breaker.window" hot_reload:"true"`
	CircuitBreakerMinCalls     int           `path:"providers.general.circuit_breaker.min_calls" hot_reload:"true"`
	CircuitBreakerOpenTime     time.Duration `path:"providers.general.circuit_breaker.open_time" hot_reload:"true"`
	// The override of the circuit breaker of each provider (task 060; operator decision 5: no switch
	// for each provider, only the four number keys). 0 (the default) takes the general value. A
	// failure_ratio is 0 to 100, the others are not negative. Hot-reloadable as the general keys.
	IrancellCircuitBreakerFailureRatio     int           `path:"providers.internal.irancell.circuit_breaker.failure_ratio" hot_reload:"true"`
	IrancellCircuitBreakerWindow           time.Duration `path:"providers.internal.irancell.circuit_breaker.window" hot_reload:"true"`
	IrancellCircuitBreakerMinCalls         int           `path:"providers.internal.irancell.circuit_breaker.min_calls" hot_reload:"true"`
	IrancellCircuitBreakerOpenTime         time.Duration `path:"providers.internal.irancell.circuit_breaker.open_time" hot_reload:"true"`
	JiringSwitchCircuitBreakerFailureRatio int           `path:"providers.internal.jiring_switch.circuit_breaker.failure_ratio" hot_reload:"true"`
	JiringSwitchCircuitBreakerWindow       time.Duration `path:"providers.internal.jiring_switch.circuit_breaker.window" hot_reload:"true"`
	JiringSwitchCircuitBreakerMinCalls     int           `path:"providers.internal.jiring_switch.circuit_breaker.min_calls" hot_reload:"true"`
	JiringSwitchCircuitBreakerOpenTime     time.Duration `path:"providers.internal.jiring_switch.circuit_breaker.open_time" hot_reload:"true"`
	ParsianCircuitBreakerFailureRatio      int           `path:"providers.internal.parsian.circuit_breaker.failure_ratio" hot_reload:"true"`
	ParsianCircuitBreakerWindow            time.Duration `path:"providers.internal.parsian.circuit_breaker.window" hot_reload:"true"`
	ParsianCircuitBreakerMinCalls          int           `path:"providers.internal.parsian.circuit_breaker.min_calls" hot_reload:"true"`
	ParsianCircuitBreakerOpenTime          time.Duration `path:"providers.internal.parsian.circuit_breaker.open_time" hot_reload:"true"`
	KarashopCircuitBreakerFailureRatio     int           `path:"providers.external.karashop.circuit_breaker.failure_ratio" hot_reload:"true"`
	KarashopCircuitBreakerWindow           time.Duration `path:"providers.external.karashop.circuit_breaker.window" hot_reload:"true"`
	KarashopCircuitBreakerMinCalls         int           `path:"providers.external.karashop.circuit_breaker.min_calls" hot_reload:"true"`
	KarashopCircuitBreakerOpenTime         time.Duration `path:"providers.external.karashop.circuit_breaker.open_time" hot_reload:"true"`
	QabzinoCircuitBreakerFailureRatio      int           `path:"providers.external.qabzino.circuit_breaker.failure_ratio" hot_reload:"true"`
	QabzinoCircuitBreakerWindow            time.Duration `path:"providers.external.qabzino.circuit_breaker.window" hot_reload:"true"`
	QabzinoCircuitBreakerMinCalls          int           `path:"providers.external.qabzino.circuit_breaker.min_calls" hot_reload:"true"`
	QabzinoCircuitBreakerOpenTime          time.Duration `path:"providers.external.qabzino.circuit_breaker.open_time" hot_reload:"true"`
	TavanirCircuitBreakerFailureRatio      int           `path:"providers.external.tavanir.circuit_breaker.failure_ratio" hot_reload:"true"`
	TavanirCircuitBreakerWindow            time.Duration `path:"providers.external.tavanir.circuit_breaker.window" hot_reload:"true"`
	TavanirCircuitBreakerMinCalls          int           `path:"providers.external.tavanir.circuit_breaker.min_calls" hot_reload:"true"`
	TavanirCircuitBreakerOpenTime          time.Duration `path:"providers.external.tavanir.circuit_breaker.open_time" hot_reload:"true"`

	// The config of the Irancell provider (task 035): the keys that bill has for its irancell
	// service client (bill: irancell.name, irancell.grpcAddress, irancell.natsSubject and the
	// irancell.getStatus block), in the layout of this project, and the switch and the timeout of the
	// adapter. The bill keys name, grpc_address and nats_subject have no default: the real values are
	// only in config.yml. Not hot-reloadable.
	//   - IrancellGRPCAddress is the host:port of the internal gRPC server of the irancell service
	//     (pb.InternalService, no TLS, as in bill). The Irancell adapter (provider/irancellproxy)
	//     reads it. Empty: every Irancell call fails as "provider down".
	//   - IrancellName is not read.
	//   - IrancellNATSSubject is read since task 056: the NATS subject of the Irancell bill payment
	//     (bill: irancell.natsSubject, "irancell-bill"). Empty with the adapter enabled and the mock
	//     off: one warn line at start, and every Irancell payment is pending.
	//   - IrancellPayTimeout (task 056, not a bill key; operator decision Q4 (a)) limits each payment
	//     request over NATS. Default 30s (bill: the fixed 30 s of pkg/orderbus). Positive.
	//   - The get_status keys are the settings of the Irancell status worker (task 050; bill:
	//     IrancellGetStatusTask), read since task 050 as the Parsian worker (see
	//     ParsianGetStatusEnabled). IrancellGetStatusEnabled (not a bill key) turns the worker on;
	//     false (the default): no worker, as before task 050. With the worker on, End must also not
	//     be greater than StatusMaxEnd. The defaults are the values of the operator's config (task 050
	//     D2; bill's example config has no Irancell worker values). Every and IsLockable are not read
	//     (Qabz uses CronTime and always takes the lock). The three sync_operations keys are bill's
	//     list getStatus.syncOperations (one single-key map for each item) as three keys, because a
	//     Config field maps one path.
	//   - IrancellEnabled turns the Irancell provider on. False (the default): every call fails as
	//     "provider down", nothing is sent to the irancell service and the mock
	//     (debug.mocks.irancell) does not apply, as for Qabzino and Tavanir.
	//   - IrancellTimeout limits each call to the irancell service. Positive.
	IrancellEnabled                  bool          `path:"providers.internal.irancell.enabled"`
	IrancellTimeout                  time.Duration `path:"providers.internal.irancell.timeout"`
	IrancellName                     string        `path:"providers.internal.irancell.name"`
	IrancellGRPCAddress              string        `path:"providers.internal.irancell.grpc_address"`
	IrancellNATSSubject              string        `path:"providers.internal.irancell.nats_subject"`
	IrancellPayTimeout               time.Duration `path:"providers.internal.irancell.pay_timeout"`
	IrancellGetStatusEnabled         bool          `path:"providers.internal.irancell.get_status.enabled"`
	IrancellGetStatusCronTime        time.Duration `path:"providers.internal.irancell.get_status.cron_time"`
	IrancellGetStatusName            string        `path:"providers.internal.irancell.get_status.name"`
	IrancellGetStatusStart           time.Duration `path:"providers.internal.irancell.get_status.start"`
	IrancellGetStatusEnd             time.Duration `path:"providers.internal.irancell.get_status.end"`
	IrancellGetStatusChunk           int           `path:"providers.internal.irancell.get_status.chunk"`
	IrancellGetStatusEvery           time.Duration `path:"providers.internal.irancell.get_status.every"`
	IrancellGetStatusLockTime        time.Duration `path:"providers.internal.irancell.get_status.lock_time"`
	IrancellGetStatusIsLockable      bool          `path:"providers.internal.irancell.get_status.is_lockable"`
	IrancellGetStatusReverseDeadline time.Duration `path:"providers.internal.irancell.get_status.reverse_deadline"`
	IrancellGetStatusSyncAPICall     time.Duration `path:"providers.internal.irancell.get_status.sync_operations.api_call_get_status"`
	IrancellGetStatusSyncGetOrder    time.Duration `path:"providers.internal.irancell.get_status.sync_operations.get_order_query"`
	IrancellGetStatusSyncInsertOrder time.Duration `path:"providers.internal.irancell.get_status.sync_operations.insert_order_query"`

	// The config of the Jiring Switch and the Parsian provider (task 036, the MCI mobile bill): each
	// adapter (provider/jiring, provider/parsianproxy) is a gRPC client of our own service
	// (jiring-switch, parsian-v2), as bill.
	//   - <Provider>Enabled turns the provider on. False (the default): every call fails as "provider
	//     down", nothing is sent to the service and the mock (debug.mocks.<provider>) does not apply.
	//   - <Provider>Timeout limits each call to the service. Positive.
	//   - <Provider>GRPCAddress is the host:port of the internal gRPC server of the service (no TLS,
	//     as in bill). Empty: every call fails as "provider down".
	// The config key of Jiring Switch is jiring_switch (the providers.name is "jiring-switch": the
	// env generator scripts/yml_to_env rejects a key with a hyphen).
	// The other keys are bill's keys of the two service clients (bill: jiringSwitch.* and
	// parsian.*: name, the getStatus block, and for Parsian the settlementInquiry* durations), in the
	// layout of this project, as for Irancell. The Parsian get_status keys are read since task 045
	// (see ParsianGetStatusEnabled). The three sync_operations keys are bill's list
	// getStatus.syncOperations as three keys, as for Irancell. Not hot-reloadable.
	//
	// The Jiring Switch status worker (task 058; bill: JiringSwitchGetStatusTask, InquiryCron) reads
	// the jiring_switch get_status keys as the Parsian worker reads its own: JiringSwitchGetStatusEnabled
	// (not a bill key) turns it on; false (the default): no worker. The defaults are the values of the
	// operator's config (the Parsian values). End is not greater than StatusMaxEnd, and the name must
	// differ from the names of the other workers that are on. LockTime is only logged, IsLockable and
	// JiringSwitchName are not read. bill's jiringSwitch.retry block is not a key of Qabz (bill's retry
	// cron is commented out).
	JiringSwitchEnabled     bool          `path:"providers.internal.jiring_switch.enabled"`
	JiringSwitchTimeout     time.Duration `path:"providers.internal.jiring_switch.timeout"`
	JiringSwitchGRPCAddress string        `path:"providers.internal.jiring_switch.grpc_address"`
	ParsianEnabled          bool          `path:"providers.internal.parsian.enabled"`
	ParsianTimeout          time.Duration `path:"providers.internal.parsian.timeout"`
	ParsianGRPCAddress      string        `path:"providers.internal.parsian.grpc_address"`

	JiringSwitchGetStatusEnabled         bool          `path:"providers.internal.jiring_switch.get_status.enabled"`
	JiringSwitchName                     string        `path:"providers.internal.jiring_switch.name"`
	JiringSwitchGetStatusName            string        `path:"providers.internal.jiring_switch.get_status.name"`
	JiringSwitchGetStatusChunk           int           `path:"providers.internal.jiring_switch.get_status.chunk"`
	JiringSwitchGetStatusStart           time.Duration `path:"providers.internal.jiring_switch.get_status.start"`
	JiringSwitchGetStatusEnd             time.Duration `path:"providers.internal.jiring_switch.get_status.end"`
	JiringSwitchGetStatusCronTime        time.Duration `path:"providers.internal.jiring_switch.get_status.cron_time"`
	JiringSwitchGetStatusLockTime        time.Duration `path:"providers.internal.jiring_switch.get_status.lock_time"`
	JiringSwitchGetStatusIsLockable      bool          `path:"providers.internal.jiring_switch.get_status.is_lockable"`
	JiringSwitchGetStatusReverseDeadline time.Duration `path:"providers.internal.jiring_switch.get_status.reverse_deadline"`
	JiringSwitchGetStatusSyncAPICall     time.Duration `path:"providers.internal.jiring_switch.get_status.sync_operations.api_call_get_status"`
	JiringSwitchGetStatusSyncGetOrder    time.Duration `path:"providers.internal.jiring_switch.get_status.sync_operations.get_order_query"`
	JiringSwitchGetStatusSyncInsertOrder time.Duration `path:"providers.internal.jiring_switch.get_status.sync_operations.insert_order_query"`

	// The Parsian status worker (task 045; bill: ParsianGetStatusTask, InquiryCron). It reads the
	// get_status keys since task 045; the defaults are the values of the operator's config (the
	// bill example config does not fill bill's getStatus block). ParsianGetStatusEnabled (not a bill
	// key) turns the worker on; false (the default): no worker, as before task 045. With the worker
	// on: CronTime is the interval (positive), Start and End give the window of the orders (now-End
	// to now-Start; End greater than Start), Chunk is the page size (positive), ReverseDeadline is
	// the age after which a final status is saved as reverse_deadline_exceeded and not reported (0:
	// every final status, as bill), and the three sync_operations durations give the lock TTL
	// (bill's formula). LockTime is only logged and IsLockable is not read (bill: lockTime is only
	// logged; Qabz always takes the lock). ParsianName is not read. Not hot-reloadable.
	ParsianGetStatusEnabled         bool          `path:"providers.internal.parsian.get_status.enabled"`
	ParsianName                     string        `path:"providers.internal.parsian.name"`
	ParsianGetStatusName            string        `path:"providers.internal.parsian.get_status.name"`
	ParsianGetStatusChunk           int           `path:"providers.internal.parsian.get_status.chunk"`
	ParsianGetStatusStart           time.Duration `path:"providers.internal.parsian.get_status.start"`
	ParsianGetStatusEnd             time.Duration `path:"providers.internal.parsian.get_status.end"`
	ParsianGetStatusCronTime        time.Duration `path:"providers.internal.parsian.get_status.cron_time"`
	ParsianGetStatusLockTime        time.Duration `path:"providers.internal.parsian.get_status.lock_time"`
	ParsianGetStatusIsLockable      bool          `path:"providers.internal.parsian.get_status.is_lockable"`
	ParsianGetStatusReverseDeadline time.Duration `path:"providers.internal.parsian.get_status.reverse_deadline"`
	ParsianGetStatusSyncAPICall     time.Duration `path:"providers.internal.parsian.get_status.sync_operations.api_call_get_status"`
	ParsianGetStatusSyncGetOrder    time.Duration `path:"providers.internal.parsian.get_status.sync_operations.get_order_query"`
	ParsianGetStatusSyncInsertOrder time.Duration `path:"providers.internal.parsian.get_status.sync_operations.insert_order_query"`
	// ParsianSettlementInquiryWater is read since task 048: the "bill already paid" check of a water
	// order paid through Parsian (bill: shouldSkipNewOrder, parsian.settlementInquiryWater). Default
	// 360h (bill's example). 0: no window; a negative value is rejected.
	ParsianSettlementInquiryWater time.Duration `path:"providers.internal.parsian.settlement_inquiry_water"`
	// ParsianSettlementInquiryElectricity is read since task 052: the "bill already paid" check of an
	// electricity order paid through Parsian (bill: shouldSkipNewOrder,
	// parsian.settlementInquiryElectricity). Default 360h (bill's example). 0: no window; a negative
	// value is rejected.
	ParsianSettlementInquiryElectricity time.Duration `path:"providers.internal.parsian.settlement_inquiry_electricity"`
	// ParsianSettlementInquiryGas is read since task 053: the "bill already paid" check of a gas order
	// paid through Parsian (bill: shouldSkipNewOrder, parsian.settlementInquiryGas). Default 360h
	// (bill's example). 0: no window; a negative value is rejected.
	ParsianSettlementInquiryGas time.Duration `path:"providers.internal.parsian.settlement_inquiry_gas"`
	// ParsianSettlementInquiryTelephone is read since task 044: the "bill already paid" check of a
	// landline order paid through Parsian (bill: shouldSkipNewOrder) looks at the orders created in
	// this window. Default 360h (bill: parsian.settlementInquiryTelephone). 0: no window (every
	// order of the bill ID and pay ID); a negative value is rejected.
	ParsianSettlementInquiryTelephone time.Duration `path:"providers.internal.parsian.settlement_inquiry_telephone"`
	// ParsianSettlementInquiryMCIMobile is read since task 054: the "bill already paid" check of an
	// MCI order (mobile, mobilemidterm) paid through Parsian (bill: shouldSkipNewOrder,
	// parsian.settlementInquiryMCIMobile). Default 0 (bill's example). As bill, 0 is not "no check":
	// the check looks at the orders of any time. A negative value is rejected.
	ParsianSettlementInquiryMCIMobile time.Duration `path:"providers.internal.parsian.settlement_inquiry_mci_mobile"`
	// ParsianSettlementInquiryRightelMobile is read since task 055: the "bill already paid" check of
	// a Rightel order (mobilemidtermRightel) paid through Parsian (bill: shouldSkipNewOrder,
	// parsian.settlementInquiryRightelMobile). Default 360h (bill's example). 0: no window (the
	// orders of any time); a negative value is rejected.
	ParsianSettlementInquiryRightelMobile time.Duration `path:"providers.internal.parsian.settlement_inquiry_rightel_mobile"`

	// The config of the payment service (task 041; bill: paymentServer.host, paymentServer.port,
	// paymentServer.privateKeyPath). Operator decisions of task 041: snake_case names, as the other
	// keys (payment_server, private_key_path); one key address in place of bill's host and port. No default:
	// the real values are only in config.yml. PaymentServerPrivateKeyPath is the path of the general
	// private key (a PKCS #1 PEM file) that signs the order token (task 039): the App reads it at
	// start, and a missing or bad key stops the start. PaymentServerKID (task 042) is the kid of the
	// token header, by which the payment service finds the public key; empty (the default) gives
	// service.name (see OrderTokenKID). PaymentServerAddress is not read yet. Not hot-reloadable.
	PaymentServerAddress        string `path:"providers.internal.payment_server.address"`
	PaymentServerPrivateKeyPath string `path:"providers.internal.payment_server.private_key_path"`
	PaymentServerKID            string `path:"providers.internal.payment_server.kid"`

	// The config of the Karashop provider (task 025, the landline bill; bill: thirdparty.karashop):
	// the adapter (provider/karashop) calls the Karashop HTTPS API itself.
	//   - KarashopEnabled turns the provider on. False (the default): every call fails as "provider
	//     down", nothing is sent to Karashop and the mock (debug.mocks.karashop) does not apply.
	//   - KarashopTimeout limits each Karashop call (an inquiry is two calls, sent at the same time).
	//     Positive. Default 30s (bill: karashop.timeout).
	//   - KarashopCode and KarashopPassword are the credential of every call; the password is a
	//     Secret (never printed). KarashopBillInquiryURL is the inquiry URL. They have no default:
	//     the real values are only in config.yml. An empty one makes every call fail as "provider down".
	//   - KarashopBillPaymentInfoURL (bill: billPaymentInfoURL) is the URL of the settlement call
	//     (SendPaymentInfo) after a landline payment (task 044). No default; empty: every settlement
	//     fails, nothing is sent.
	// Not hot-reloadable.
	KarashopEnabled            bool          `path:"providers.external.karashop.enabled"`
	KarashopTimeout            time.Duration `path:"providers.external.karashop.timeout"`
	KarashopCode               string        `path:"providers.external.karashop.code"`
	KarashopPassword           Secret        `path:"providers.external.karashop.password"`
	KarashopBillInquiryURL     string        `path:"providers.external.karashop.bill_inquiry_url"`
	KarashopBillPaymentInfoURL string        `path:"providers.external.karashop.bill_payment_info_url"`

	// CachingWater is how long a saved water inquiry answers the same bill again with no provider
	// call (bill: caching.water, 1h). 0 turns the cache off; a negative value is rejected. Not
	// hot-reloadable (task 021, decision 1): read at start.
	CachingWater time.Duration `path:"caching.water"`
	// CachingElectricity is how long a saved electricity inquiry answers the same bill again with
	// no provider call (bill: caching.electricity, 1h). 0 turns the cache off; a negative value is
	// rejected. Not hot-reloadable (task 022, as caching.water): read at start.
	CachingElectricity time.Duration `path:"caching.electricity"`
	// CachingGas is how long a saved gas inquiry answers the same participate code again with no
	// provider call (bill: caching.gas, 1h). 0 turns the cache off; a negative value is rejected.
	// Not hot-reloadable (task 023, as caching.water): read at start.
	CachingGas time.Duration `path:"caching.gas"`
	// CachingIrancell is how long a saved Irancell mobile inquiry answers the same phone again with
	// no provider call. The default is 0 (cache off): bill has no cache for the bill type irancell,
	// so every request asks Irancell (task 035, decision A). A negative value is rejected. Not
	// hot-reloadable (as caching.water): read at start.
	CachingIrancell time.Duration `path:"caching.irancell"`
	// CachingMobile is how long a saved MCI mobile inquiry (the bill type mobile) or Rightel mobile
	// inquiry (the bill type mobilemidtermRightel, task 037; operator decision: the same key, as bill)
	// answers the same phone again with no provider call (bill: caching.mobile, 5h). 0 turns the
	// cache off; a negative value is rejected. Not hot-reloadable (task 036, as caching.water): read at start.
	CachingMobile time.Duration `path:"caching.mobile"`
	// CachingTel is how long a saved landline inquiry (the bill type tel, task 025) answers the same
	// number again with no provider call (bill: caching.tel, 1h). 0 turns the cache off; a negative
	// value is rejected. Not hot-reloadable (as caching.water): read at start.
	CachingTel time.Duration `path:"caching.tel"`
	// CachingPaidTel is how long a saved landline inquiry gives the price of a landline order (the
	// bill types tel and telmidterm, task 044) with no provider call (bill: caching.paidTel, 72h). 0:
	// every order asks the provider; a negative value is rejected. Read at start.
	CachingPaidTel time.Duration `path:"caching.paid_tel"`
	// CachingPaidWater is how long a saved water inquiry gives the price of a water order (task 048)
	// with no provider call (bill: caching.paidWater, 72h). It applies only when the raw inquiry of the
	// order fails. 0: every such order asks the provider; a negative value is rejected. Read at start.
	CachingPaidWater time.Duration `path:"caching.paid_water"`
	// CachingPaidElectricity is how long a saved electricity inquiry gives the price of an electricity
	// order (task 052) with no provider call (bill: caching.paidElectricity, 72h). It applies only when
	// the raw inquiry of the order fails. 0: every such order asks the provider; a negative value is
	// rejected. Read at start.
	CachingPaidElectricity time.Duration `path:"caching.paid_electricity"`
	// CachingPaidGas is how long a saved gas inquiry gives the price of a gas order (task 053) with no
	// provider call (bill: caching.paidGas, 72h). It applies only when the raw inquiry of the order
	// fails. 0: every such order asks the provider; a negative value is rejected. Read at start.
	CachingPaidGas time.Duration `path:"caching.paid_gas"`
	// CachingPaidMobile is how long a saved MCI mobile inquiry gives the price of a full-term MCI
	// order (the bill type mobile, task 054) with no provider call (bill: caching.paidMobile, 1h). It
	// applies only when the raw inquiry of the order fails. A mid-term order (mobilemidterm) never
	// uses saved inquiries (bill has no TTL for it). Since task 055 it is also the TTL of a Rightel
	// order (mobilemidtermRightel; bill: caching.paidMobile for MOBILEMIDTERMRIGHTEL). 0: every such
	// order asks the provider; a negative value is rejected. Read at start.
	CachingPaidMobile time.Duration `path:"caching.paid_mobile"`

	// DebugMockQabzinoEnabled makes the Qabzino adapter answer with a fixed answer, with no call to
	// the backend (task 021). Development only: a mode other than dev removes the debug section, so
	// the key is false there (and the app ignores it outside dev again). Not hot-reloadable.
	DebugMockQabzinoEnabled bool `path:"debug.mocks.qabzino.enabled"`
	// DebugMockQabzinoReturnType is the answer of the Qabzino mock: MockReturnSuccess or
	// MockReturnError. It has no effect while the mock is off. Development only.
	DebugMockQabzinoReturnType string `path:"debug.mocks.qabzino.return_type"`

	// DebugMockTavanirEnabled makes the Tavanir adapter answer with a fixed answer, with no call to
	// the backend (task 022). Development only, as DebugMockQabzinoEnabled. Not hot-reloadable.
	DebugMockTavanirEnabled bool `path:"debug.mocks.tavanir.enabled"`
	// DebugMockTavanirReturnType is the answer of the Tavanir mock: MockReturnSuccess or
	// MockReturnError. It has no effect while the mock is off. Development only.
	DebugMockTavanirReturnType string `path:"debug.mocks.tavanir.return_type"`

	// DebugMockIrancellEnabled makes the Irancell adapter answer with a fixed answer, with no call to
	// the irancell service (task 035). Development only, as DebugMockQabzinoEnabled. Not hot-reloadable.
	DebugMockIrancellEnabled bool `path:"debug.mocks.irancell.enabled"`
	// DebugMockIrancellReturnType is the answer of the Irancell mock: MockReturnSuccess or
	// MockReturnError. It has no effect while the mock is off. Development only.
	DebugMockIrancellReturnType string `path:"debug.mocks.irancell.return_type"`
	// DebugMockIrancellStatusReturnType is the answer of the Irancell mock to a status call (task
	// 050): MockReturnSuccess, MockReturnFailed or MockReturnPending (the answer of a timeout). No
	// effect while the mock is off. Development only.
	DebugMockIrancellStatusReturnType string `path:"debug.mocks.irancell.status_return_type"`
	// DebugMockIrancellPayReturnType is the answer of the Irancell mock to a payment (task 056):
	// MockReturnSuccess, MockReturnFailed or MockReturnPending (the answer of a timeout). No effect
	// while the mock is off. Development only.
	DebugMockIrancellPayReturnType string `path:"debug.mocks.irancell.pay_return_type"`

	// DebugMockJiringSwitchEnabled and DebugMockParsianEnabled make the Jiring Switch and the Parsian
	// adapter answer with a fixed answer, with no call to the jiring-switch or the parsian-v2
	// service (task 036). Development only, as DebugMockQabzinoEnabled. Not hot-reloadable. The
	// return types are MockReturnSuccess or MockReturnError; no effect while the mock is off.
	DebugMockJiringSwitchEnabled    bool   `path:"debug.mocks.jiring_switch.enabled"`
	DebugMockJiringSwitchReturnType string `path:"debug.mocks.jiring_switch.return_type"`
	// DebugMockJiringSwitchPayReturnType and DebugMockJiringSwitchStatusReturnType are the answers of
	// the Jiring Switch mock to a payment and a status call (task 058): MockReturnSuccess,
	// MockReturnFailed or MockReturnPending (the answer of a timeout: never a fallback). No effect
	// while the mock is off. Development only.
	DebugMockJiringSwitchPayReturnType    string `path:"debug.mocks.jiring_switch.pay_return_type"`
	DebugMockJiringSwitchStatusReturnType string `path:"debug.mocks.jiring_switch.status_return_type"`
	DebugMockParsianEnabled               bool   `path:"debug.mocks.parsian.enabled"`
	DebugMockParsianReturnType            string `path:"debug.mocks.parsian.return_type"`
	// DebugMockParsianPayReturnType is the answer of the Parsian mock to a payment (task 044,
	// conventions § 15: one return-type key for each method): MockReturnSuccess, MockReturnFailed or
	// MockReturnPending (the answer of a timeout). No effect while the mock is off. Development only.
	DebugMockParsianPayReturnType string `path:"debug.mocks.parsian.pay_return_type"`

	// DebugMockParsianStatusReturnType is the answer of the Parsian mock to a status call (task
	// 045): MockReturnSuccess, MockReturnFailed or MockReturnPending (the answer of a timeout). No
	// effect while the mock is off. Development only.
	DebugMockParsianStatusReturnType string `path:"debug.mocks.parsian.status_return_type"`

	// DebugMockKarashopEnabled makes the Karashop adapter answer with a fixed answer, with no call to
	// Karashop (task 025). Development only, as DebugMockQabzinoEnabled. Not hot-reloadable. The
	// return type is MockReturnSuccess or MockReturnError; no effect while the mock is off.
	DebugMockKarashopEnabled    bool   `path:"debug.mocks.karashop.enabled"`
	DebugMockKarashopReturnType string `path:"debug.mocks.karashop.return_type"`
	// DebugMockKarashopSettleReturnType is the answer of the Karashop mock to a settlement (task
	// 044): MockReturnSuccess or MockReturnError. No effect while the mock is off. Development only.
	DebugMockKarashopSettleReturnType string `path:"debug.mocks.karashop.settle_return_type"`

	// The Kafka client (task 045, supertask 043 decision C; bill: kafka.*). Every key of bill's
	// kafka block, with the defaults of bill's example config. Read: KafkaBrokers (the addresses
	// of the producer) and KafkaPaymentWriteSubject (the topic of the status messages to the payment
	// service) and, since task 050, KafkaTimeout (the limit of each publish; bill has it and does not
	// read it). Not read (bill has them and does not use them): the write group, the read subject and
	// group, the user name, the password (a Secret) and auth. The producer exists only while a status
	// worker (Parsian or Irancell) is on; then the brokers and the write subject must not be empty and
	// the timeout must be positive. Not hot-reloadable.
	KafkaBrokers             []string      `path:"kafka.brokers"`
	KafkaPaymentWriteSubject string        `path:"kafka.topic.payment.write.subject"`
	KafkaPaymentWriteGroup   string        `path:"kafka.topic.payment.write.group"`
	KafkaPaymentReadSubject  string        `path:"kafka.topic.payment.read.subject"`
	KafkaPaymentReadGroup    string        `path:"kafka.topic.payment.read.group"`
	KafkaUsername            string        `path:"kafka.username"`
	KafkaPassword            Secret        `path:"kafka.password"`
	KafkaAuth                bool          `path:"kafka.auth"`
	KafkaTimeout             time.Duration `path:"kafka.timeout"`

	// The NATS client (task 056; bill: Nats.*, client/broker nats_dto.go). bill's keys in snake_case,
	// with the defaults of bill's example config; the endpoints, the user name and the password have
	// no default (the real values only in config.yml). The client exists only while the Irancell
	// adapter is enabled and its mock is off; then the endpoints must not be empty. NATSAuth turns TLS
	// on, as bill (it passes the key to nats.Options.Secure), not the user name and password.
	// NATSReconnectWait and NATSTimeout are durations (bill: whole seconds, 5 and 3). bill's
	// nats.encoder is not a key: Qabz sends JSON only. Not hot-reloadable.
	NATSEndpoints      []string      `path:"nats.endpoints"`
	NATSUsername       string        `path:"nats.username"`
	NATSPassword       Secret        `path:"nats.password"`
	NATSAuth           bool          `path:"nats.auth"`
	NATSAllowReconnect bool          `path:"nats.allow_reconnect"`
	NATSMaxReconnect   int           `path:"nats.max_reconnect"`
	NATSReconnectWait  time.Duration `path:"nats.reconnect_wait"`
	NATSTimeout        time.Duration `path:"nats.timeout"`

	// The settlement worker (task 046; bill: confirmPayment.telephone, ConfirmPaymentTelephone).
	// bill's example config has no values; the defaults are the operator's (2026-11-15).
	// ConfirmPaymentTelephoneEnabled (not a bill key) turns the worker on; false (the default): no
	// worker, as before task 046. With the worker on: Name is the lock key and the metric label (not
	// empty), Every is the interval (positive), Start and End give the window of the paid orders
	// (created after now-End and before now-Start; 0 <= Start < End <= 48h, because the confirmation
	// sets only the success rows of the last 48 h), Chunk is the page size (positive). LockTime is
	// only logged (bill). Not hot-reloadable.
	ConfirmPaymentTelephoneEnabled  bool          `path:"confirm_payment.telephone.enabled"`
	ConfirmPaymentTelephoneName     string        `path:"confirm_payment.telephone.name"`
	ConfirmPaymentTelephoneEvery    time.Duration `path:"confirm_payment.telephone.every"`
	ConfirmPaymentTelephoneLockTime time.Duration `path:"confirm_payment.telephone.lock_time"`
	ConfirmPaymentTelephoneStart    time.Duration `path:"confirm_payment.telephone.start"`
	ConfirmPaymentTelephoneEnd      time.Duration `path:"confirm_payment.telephone.end"`
	ConfirmPaymentTelephoneChunk    int           `path:"confirm_payment.telephone.chunk"`

	// DebugCacheInquiries is the switch of every use of the inquiry tables (bill_inquiries,
	// bill_inquiry_requests) by the inquiry flow (task 031). True (the default): the flow reads the
	// saved inquiries first and saves the request row and the answer, as before. False: the flow
	// does not read or write these tables; every inquiry calls the provider. Development only: a
	// mode other than dev removes the debug section, so the key is true there. Not hot-reloadable:
	// read at start (as the caching.* keys).
	DebugCacheInquiries bool `path:"debug.cache_inquiries"`
}

func defaults() Config {
	return Config{
		ServiceName:          "qabz",
		ServiceEcosystemName: "bill",
		RedisPrefix:          "qabz:",
		LogLevel:             "info",
		PostgresHost:         "localhost",
		PostgresPort:         5432,
		PostgresUser:         "qabz",
		PostgresDB:           "qabz",
		RedisHost:            "localhost",
		RedisPort:            6379,
		GRPCAddr:             ":8082",
		HTTPAddr:             ":8080",
		MetricsAddr:          ":9090",
		MetricsEnabled:       true,
		HotReloadInterval:    30 * time.Second,
		MigrateOnStart:       true,
		MigrateTimeout:       60 * time.Second,

		ProvidersLoadInterval: time.Minute,

		CircuitBreakerFailureRatio: DefaultCircuitBreakerFailureRatio,
		CircuitBreakerWindow:       DefaultCircuitBreakerWindow,
		CircuitBreakerMinCalls:     DefaultCircuitBreakerMinCalls,
		CircuitBreakerOpenTime:     DefaultCircuitBreakerOpenTime,

		QabzinoTimeout:      10 * time.Second,
		TavanirTimeout:      10 * time.Second,
		IrancellTimeout:     10 * time.Second,
		JiringSwitchTimeout: 10 * time.Second,
		ParsianTimeout:      10 * time.Second,
		KarashopTimeout:     30 * time.Second,

		CachingWater:                        time.Hour,
		CachingElectricity:                  time.Hour,
		CachingGas:                          time.Hour,
		CachingMobile:                       5 * time.Hour,
		CachingTel:                          time.Hour,
		CachingPaidTel:                      72 * time.Hour,
		CachingPaidWater:                    72 * time.Hour,
		CachingPaidElectricity:              72 * time.Hour,
		CachingPaidGas:                      72 * time.Hour,
		CachingPaidMobile:                   time.Hour,
		ParsianSettlementInquiryTelephone:   360 * time.Hour,
		ParsianSettlementInquiryWater:       360 * time.Hour,
		ParsianSettlementInquiryElectricity: 360 * time.Hour,
		ParsianSettlementInquiryGas:         360 * time.Hour,
		ParsianGetStatusName:                "parsian-getstatus",
		ParsianGetStatusChunk:               100,
		ParsianGetStatusStart:               30 * time.Minute,
		ParsianGetStatusEnd:                 10 * time.Hour,
		ParsianGetStatusCronTime:            15 * time.Minute,
		ParsianGetStatusLockTime:            30 * time.Minute,
		ParsianGetStatusIsLockable:          true,
		ParsianGetStatusReverseDeadline:     10 * time.Hour,
		ParsianGetStatusSyncAPICall:         30 * time.Second,
		ParsianGetStatusSyncGetOrder:        3 * time.Millisecond,
		ParsianGetStatusSyncInsertOrder:     3 * time.Millisecond,

		// The Irancell status worker (task 050 D2): the values of the operator's config.
		IrancellGetStatusName:             "irancell-get-status",
		IrancellGetStatusCronTime:         15 * time.Minute,
		IrancellGetStatusStart:            30 * time.Minute,
		IrancellGetStatusEnd:              48 * time.Hour,
		IrancellGetStatusChunk:            100,
		IrancellGetStatusEvery:            15 * time.Minute,
		IrancellGetStatusLockTime:         30 * time.Minute,
		IrancellGetStatusIsLockable:       true,
		IrancellGetStatusReverseDeadline:  10 * time.Hour,
		IrancellGetStatusSyncAPICall:      30 * time.Second,
		IrancellGetStatusSyncGetOrder:     3 * time.Millisecond,
		IrancellGetStatusSyncInsertOrder:  3 * time.Millisecond,
		KafkaBrokers:                      []string{"localhost:9092"},
		KafkaPaymentWriteSubject:          "paymentwriter",
		KafkaPaymentWriteGroup:            "paymentwriter",
		KafkaPaymentReadSubject:           "payment",
		KafkaPaymentReadGroup:             "paymentBill",
		KafkaTimeout:                      5 * time.Second,
		ConfirmPaymentTelephoneName:       "telephone-confirmpayment",
		ConfirmPaymentTelephoneEvery:      15 * time.Minute,
		ConfirmPaymentTelephoneLockTime:   30 * time.Minute,
		ConfirmPaymentTelephoneStart:      5 * time.Minute,
		ConfirmPaymentTelephoneEnd:        24 * time.Hour,
		ConfirmPaymentTelephoneChunk:      100,
		DebugMockParsianStatusReturnType:  MockReturnSuccess,
		DebugMockQabzinoReturnType:        MockReturnSuccess,
		DebugMockTavanirReturnType:        MockReturnSuccess,
		DebugMockIrancellReturnType:       MockReturnSuccess,
		DebugMockIrancellStatusReturnType: MockReturnSuccess,
		DebugMockJiringSwitchReturnType:   MockReturnSuccess,

		// Task 058: the Jiring Switch status worker (the values of the operator's config, as Parsian)
		// and the payment and status answers of its mock.
		JiringSwitchGetStatusName:             "jiring-switch-getstatus",
		JiringSwitchGetStatusChunk:            100,
		JiringSwitchGetStatusStart:            30 * time.Minute,
		JiringSwitchGetStatusEnd:              10 * time.Hour,
		JiringSwitchGetStatusCronTime:         15 * time.Minute,
		JiringSwitchGetStatusLockTime:         30 * time.Minute,
		JiringSwitchGetStatusIsLockable:       true,
		JiringSwitchGetStatusReverseDeadline:  10 * time.Hour,
		JiringSwitchGetStatusSyncAPICall:      30 * time.Second,
		JiringSwitchGetStatusSyncGetOrder:     3 * time.Millisecond,
		JiringSwitchGetStatusSyncInsertOrder:  3 * time.Millisecond,
		DebugMockJiringSwitchPayReturnType:    MockReturnSuccess,
		DebugMockJiringSwitchStatusReturnType: MockReturnSuccess,

		DebugMockParsianReturnType:        MockReturnSuccess,
		DebugMockParsianPayReturnType:     MockReturnSuccess,
		DebugMockKarashopReturnType:       MockReturnSuccess,
		DebugMockKarashopSettleReturnType: MockReturnSuccess,
		DebugCacheInquiries:               true,

		// Task 055: bill's example value.
		ParsianSettlementInquiryRightelMobile: 360 * time.Hour,

		// Task 056: the Irancell payment over NATS (bill's example values).
		IrancellPayTimeout:             30 * time.Second,
		DebugMockIrancellPayReturnType: MockReturnSuccess,
		NATSEndpoints:                  []string{}, // no default; the example has an empty list
		NATSAllowReconnect:             true,
		NATSMaxReconnect:               50,
		NATSReconnectWait:              5 * time.Second,
		NATSTimeout:                    3 * time.Second,
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
	if err := checkRenamed(m); err != nil {
		return Config{}, err
	}
	c := defaults()
	if err := decodeStruct(m, reflect.ValueOf(&c).Elem(), ""); err != nil {
		return Config{}, err
	}
	return c, c.validate()
}

// renamedKeys maps each config path that was renamed to its new path (task 051 decision D4). Parse
// rejects a file that still has an old path, so a renamed switch is never dropped with no message.
var renamedKeys = map[string]string{
	"logs.otlp_endpoint":               "logs.tracing.otlp_endpoint",
	"debug.mocks.qabzino.enable":       "debug.mocks.qabzino.enabled",
	"debug.mocks.tavanir.enable":       "debug.mocks.tavanir.enabled",
	"debug.mocks.irancell.enable":      "debug.mocks.irancell.enabled",
	"debug.mocks.jiring_switch.enable": "debug.mocks.jiring_switch.enabled",
	"debug.mocks.parsian.enable":       "debug.mocks.parsian.enabled",
	"debug.mocks.karashop.enable":      "debug.mocks.karashop.enabled",
}

// checkRenamed returns one error for each old path of renamedKeys that m has, sorted by path. Parse
// calls it after the debug section is removed outside dev, so an old debug key there is dropped as
// every debug key is. A part of a path that is not an object stops that lookup with no error: the
// decode reports the wrong kind of the new path.
func checkRenamed(m map[string]any) error {
	var errs []error
	for _, old := range slices.Sorted(maps.Keys(renamedKeys)) {
		if _, found, err := lookup(m, old, ""); err == nil && found {
			errs = append(errs, fmt.Errorf("%s was renamed to %s", old, renamedKeys[old]))
		}
	}
	return errors.Join(errs...)
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
	if c.InternalGRPCAddr != "" {
		if err := checkListenAddr(c.InternalGRPCAddr); err != nil {
			errs = append(errs, fmt.Errorf("server.internal_grpc.addr %w", err))
		} else if !strings.HasSuffix(c.InternalGRPCAddr, ":0") && (c.InternalGRPCAddr == c.GRPCAddr || c.InternalGRPCAddr == c.HTTPAddr) {
			errs = append(errs, errors.New("server.internal_grpc.addr must not be the address of server.grpc.addr or server.http.addr"))
		}
	}
	for path, d := range map[string]time.Duration{"hot_reload_interval": c.HotReloadInterval, "database.migration.timeout": c.MigrateTimeout} {
		if d <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive duration (for example 30s)", path))
		}
	}
	if c.ProvidersLoadInterval < ProvidersMinLoadInterval {
		errs = append(errs, fmt.Errorf("providers.general.load_interval must be at least %s", ProvidersMinLoadInterval))
	}
	errs = append(errs, c.circuitBreakerErrors()...)
	if c.QabzinoTimeout <= 0 {
		errs = append(errs, errors.New("providers.external.qabzino.timeout must be a positive duration (for example 10s)"))
	}
	if c.TavanirTimeout <= 0 {
		errs = append(errs, errors.New("providers.external.tavanir.timeout must be a positive duration (for example 10s)"))
	}
	if c.IrancellTimeout <= 0 {
		errs = append(errs, errors.New("providers.internal.irancell.timeout must be a positive duration (for example 10s)"))
	}
	if c.IrancellPayTimeout <= 0 {
		errs = append(errs, errors.New("providers.internal.irancell.pay_timeout must be a positive duration (for example 30s)"))
	}
	if c.NATSMaxReconnect < -1 {
		errs = append(errs, errors.New("nats.max_reconnect must be -1 (no limit) or more"))
	}
	if c.NATSReconnectWait < 0 {
		errs = append(errs, errors.New("nats.reconnect_wait must not be negative"))
	}
	if c.NATSTimeout <= 0 {
		errs = append(errs, errors.New("nats.timeout must be a positive duration (for example 3s)"))
	}
	if !ValidMockPayReturnType(c.DebugMockIrancellPayReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.irancell.pay_return_type must be %q, %q or %q", MockReturnSuccess, MockReturnFailed, MockReturnPending))
	}
	if c.CachingIrancell < 0 {
		errs = append(errs, errors.New("caching.irancell must not be negative (0 turns the cache off)"))
	}
	if !ValidMockReturnType(c.DebugMockIrancellReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.irancell.return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if c.JiringSwitchTimeout <= 0 {
		errs = append(errs, errors.New("providers.internal.jiring_switch.timeout must be a positive duration (for example 10s)"))
	}
	if c.ParsianTimeout <= 0 {
		errs = append(errs, errors.New("providers.internal.parsian.timeout must be a positive duration (for example 10s)"))
	}
	if c.CachingMobile < 0 {
		errs = append(errs, errors.New("caching.mobile must not be negative (0 turns the cache off)"))
	}
	if !ValidMockReturnType(c.DebugMockJiringSwitchReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.jiring_switch.return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if !ValidMockReturnType(c.DebugMockParsianReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.parsian.return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if c.KarashopTimeout <= 0 {
		errs = append(errs, errors.New("providers.external.karashop.timeout must be a positive duration (for example 30s)"))
	}
	if c.CachingTel < 0 {
		errs = append(errs, errors.New("caching.tel must not be negative (0 turns the cache off)"))
	}
	if c.CachingPaidTel < 0 {
		errs = append(errs, errors.New("caching.paid_tel must not be negative (0: every order asks the provider)"))
	}
	if c.ParsianSettlementInquiryTelephone < 0 {
		errs = append(errs, errors.New("providers.internal.parsian.settlement_inquiry_telephone must not be negative (0: no window)"))
	}
	if c.CachingPaidWater < 0 {
		errs = append(errs, errors.New("caching.paid_water must not be negative (0: every order asks the provider)"))
	}
	if c.ParsianSettlementInquiryWater < 0 {
		errs = append(errs, errors.New("providers.internal.parsian.settlement_inquiry_water must not be negative (0: no window)"))
	}
	if c.CachingPaidElectricity < 0 {
		errs = append(errs, errors.New("caching.paid_electricity must not be negative (0: every order asks the provider)"))
	}
	if c.ParsianSettlementInquiryElectricity < 0 {
		errs = append(errs, errors.New("providers.internal.parsian.settlement_inquiry_electricity must not be negative (0: no window)"))
	}
	if c.CachingPaidGas < 0 {
		errs = append(errs, errors.New("caching.paid_gas must not be negative (0: every order asks the provider)"))
	}
	if c.ParsianSettlementInquiryGas < 0 {
		errs = append(errs, errors.New("providers.internal.parsian.settlement_inquiry_gas must not be negative (0: no window)"))
	}
	if c.CachingPaidMobile < 0 {
		errs = append(errs, errors.New("caching.paid_mobile must not be negative (0: every order asks the provider)"))
	}
	if c.ParsianSettlementInquiryRightelMobile < 0 {
		errs = append(errs, errors.New("providers.internal.parsian.settlement_inquiry_rightel_mobile must not be negative (0: no window)"))
	}
	if c.ParsianSettlementInquiryMCIMobile < 0 {
		errs = append(errs, errors.New("providers.internal.parsian.settlement_inquiry_mci_mobile must not be negative (0: the orders of any time)"))
	}
	if !ValidMockPayReturnType(c.DebugMockParsianPayReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.parsian.pay_return_type must be %q, %q or %q", MockReturnSuccess, MockReturnFailed, MockReturnPending))
	}
	if !ValidMockPayReturnType(c.DebugMockParsianStatusReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.parsian.status_return_type must be %q, %q or %q", MockReturnSuccess, MockReturnFailed, MockReturnPending))
	}
	if c.ServiceEcosystemName == "" {
		errs = append(errs, errors.New("service.ecosystem_name must not be empty (the name that the other services see; for example bill)"))
	}
	if !ValidMockPayReturnType(c.DebugMockIrancellStatusReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.irancell.status_return_type must be %q, %q or %q", MockReturnSuccess, MockReturnFailed, MockReturnPending))
	}
	if c.ParsianGetStatusEnabled {
		errs = append(errs, c.validateParsianGetStatus()...)
	}
	if c.IrancellGetStatusEnabled {
		errs = append(errs, c.validateIrancellGetStatus()...)
	}
	if c.JiringSwitchGetStatusEnabled {
		errs = append(errs, c.validateJiringSwitchGetStatus()...)
	}
	if !ValidMockPayReturnType(c.DebugMockJiringSwitchPayReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.jiring_switch.pay_return_type must be %q, %q or %q", MockReturnSuccess, MockReturnFailed, MockReturnPending))
	}
	if !ValidMockPayReturnType(c.DebugMockJiringSwitchStatusReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.jiring_switch.status_return_type must be %q, %q or %q", MockReturnSuccess, MockReturnFailed, MockReturnPending))
	}
	if c.StatusWorkerEnabled() {
		errs = append(errs, c.validateKafkaProducer()...)
	}
	if c.ConfirmPaymentTelephoneEnabled {
		errs = append(errs, c.validateConfirmPaymentTelephone()...)
	}
	if !ValidMockReturnType(c.DebugMockKarashopSettleReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.karashop.settle_return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if !ValidMockReturnType(c.DebugMockKarashopReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.karashop.return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if c.CachingWater < 0 {
		errs = append(errs, errors.New("caching.water must not be negative (0 turns the cache off)"))
	}
	if c.CachingElectricity < 0 {
		errs = append(errs, errors.New("caching.electricity must not be negative (0 turns the cache off)"))
	}
	if c.CachingGas < 0 {
		errs = append(errs, errors.New("caching.gas must not be negative (0 turns the cache off)"))
	}
	if !ValidMockReturnType(c.DebugMockQabzinoReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.qabzino.return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if !ValidMockReturnType(c.DebugMockTavanirReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.tavanir.return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if c.TracingEnabled && strings.TrimSpace(c.TracingOTLPEndpoint) == "" {
		errs = append(errs, errors.New("logs.tracing.otlp_endpoint is required while logs.tracing.enabled is true"))
	}
	if c.DebugAuthMockUserID != "" {
		if u, err := uuid.Parse(c.DebugAuthMockUserID); err != nil || u.String() != c.DebugAuthMockUserID {
			errs = append(errs, errors.New("debug.auth.mock_user_id must be a UUID in the lower-case form xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"))
		}
	}
	return errors.Join(errs...)
}

// statusWorkerKeys are the keys of one status worker, for validateStatusWorker.
type statusWorkerKeys struct {
	prefix                          string // the config path of the get_status block, with the dot
	name                            string
	cronTime, start, end, deadline  time.Duration
	chunk                           int
	syncAPI, syncGetOrder, syncSave time.Duration
	maxEnd                          time.Duration // 0: no limit
}

// validateStatusWorker checks the keys of one status worker (task 045; shared since task 050).
func validateStatusWorker(k statusWorkerKeys) []error {
	p := k.prefix
	var errs []error
	if k.name == "" {
		errs = append(errs, errors.New(p+"name must not be empty (the lock key and the metric label)"))
	}
	if k.cronTime <= 0 {
		errs = append(errs, errors.New(p+"cron_time must be a positive duration (for example 15m)"))
	}
	if k.chunk <= 0 {
		errs = append(errs, errors.New(p+"chunk must be positive (for example 100)"))
	}
	if k.start < 0 || k.end <= k.start {
		errs = append(errs, errors.New(p+"start must not be negative and end must be greater than start (for example 30m and 10h)"))
	}
	if k.maxEnd > 0 && k.end > k.maxEnd {
		errs = append(errs, fmt.Errorf("%send must not be greater than %s (the worker reads the newest row of an order of the last %s)", p, k.maxEnd, k.maxEnd))
	}
	if k.deadline < 0 {
		errs = append(errs, errors.New(p+"reverse_deadline must not be negative"))
	}
	if k.syncAPI < 0 || k.syncGetOrder < 0 || k.syncSave < 0 {
		errs = append(errs, errors.New(p+"sync_operations must not be negative"))
	}
	return errs
}

// validateParsianGetStatus checks the keys of the Parsian status worker (task 045). validate calls
// it only while the worker is on.
func (c Config) validateParsianGetStatus() []error {
	return validateStatusWorker(statusWorkerKeys{
		prefix: "providers.internal.parsian.get_status.", name: c.ParsianGetStatusName,
		cronTime: c.ParsianGetStatusCronTime, start: c.ParsianGetStatusStart, end: c.ParsianGetStatusEnd,
		deadline: c.ParsianGetStatusReverseDeadline, chunk: c.ParsianGetStatusChunk,
		syncAPI: c.ParsianGetStatusSyncAPICall, syncGetOrder: c.ParsianGetStatusSyncGetOrder, syncSave: c.ParsianGetStatusSyncInsertOrder,
	})
}

// StatusMaxEnd is the greatest providers.internal.irancell.get_status.end: the worker reads the
// newest row of an order of the last 48 h (order.LatestWindow), so an older order is never checked
// (task 050 D2).
const StatusMaxEnd = 48 * time.Hour

// validateIrancellGetStatus checks the keys of the Irancell status worker (task 050). validate calls
// it only while the worker is on. Its name must differ from the Parsian worker name while both are
// on (the lock key and the metric label).
func (c Config) validateIrancellGetStatus() []error {
	errs := validateStatusWorker(statusWorkerKeys{
		prefix: "providers.internal.irancell.get_status.", name: c.IrancellGetStatusName,
		cronTime: c.IrancellGetStatusCronTime, start: c.IrancellGetStatusStart, end: c.IrancellGetStatusEnd,
		deadline: c.IrancellGetStatusReverseDeadline, chunk: c.IrancellGetStatusChunk,
		syncAPI: c.IrancellGetStatusSyncAPICall, syncGetOrder: c.IrancellGetStatusSyncGetOrder, syncSave: c.IrancellGetStatusSyncInsertOrder,
		maxEnd: StatusMaxEnd,
	})
	if c.ParsianGetStatusEnabled && c.IrancellGetStatusName != "" && c.IrancellGetStatusName == c.ParsianGetStatusName {
		errs = append(errs, errors.New("providers.internal.irancell.get_status.name must differ from providers.internal.parsian.get_status.name (the lock key and the metric label)"))
	}
	return errs
}

// validateJiringSwitchGetStatus checks the keys of the Jiring Switch status worker (task 058).
// validate calls it only while the worker is on. Its name must differ from the name of each other
// status worker that is on (the lock key and the metric label).
func (c Config) validateJiringSwitchGetStatus() []error {
	errs := validateStatusWorker(statusWorkerKeys{
		prefix: "providers.internal.jiring_switch.get_status.", name: c.JiringSwitchGetStatusName,
		cronTime: c.JiringSwitchGetStatusCronTime, start: c.JiringSwitchGetStatusStart, end: c.JiringSwitchGetStatusEnd,
		deadline: c.JiringSwitchGetStatusReverseDeadline, chunk: c.JiringSwitchGetStatusChunk,
		syncAPI: c.JiringSwitchGetStatusSyncAPICall, syncGetOrder: c.JiringSwitchGetStatusSyncGetOrder, syncSave: c.JiringSwitchGetStatusSyncInsertOrder,
		maxEnd: StatusMaxEnd,
	})
	if c.JiringSwitchGetStatusName != "" {
		if c.ParsianGetStatusEnabled && c.JiringSwitchGetStatusName == c.ParsianGetStatusName {
			errs = append(errs, errors.New("providers.internal.jiring_switch.get_status.name must differ from providers.internal.parsian.get_status.name (the lock key and the metric label)"))
		}
		if c.IrancellGetStatusEnabled && c.JiringSwitchGetStatusName == c.IrancellGetStatusName {
			errs = append(errs, errors.New("providers.internal.jiring_switch.get_status.name must differ from providers.internal.irancell.get_status.name (the lock key and the metric label)"))
		}
	}
	return errs
}

// StatusWorkerEnabled reports whether a status worker is on (Parsian, Irancell or Jiring Switch):
// the workers report on Kafka (task 058: one place for the three switches).
func (c Config) StatusWorkerEnabled() bool {
	return c.ParsianGetStatusEnabled || c.IrancellGetStatusEnabled || c.JiringSwitchGetStatusEnabled
}

// validateKafkaProducer checks the keys of the Kafka producer of the status workers (task 045; the
// timeout since task 050). validate calls it only while a status worker is on.
func (c Config) validateKafkaProducer() []error {
	var errs []error
	if len(c.KafkaBrokers) == 0 || slices.Contains(c.KafkaBrokers, "") {
		errs = append(errs, errors.New("kafka.brokers must have at least one address and no empty one (the status worker reports on Kafka)"))
	}
	if c.KafkaPaymentWriteSubject == "" {
		errs = append(errs, errors.New("kafka.topic.payment.write.subject must not be empty (the status worker reports on Kafka)"))
	}
	if c.KafkaTimeout <= 0 {
		errs = append(errs, errors.New("kafka.timeout must be a positive duration (the limit of each publish of the status worker; for example 5s)"))
	}
	return errs
}

// ConfirmPaymentMaxEnd is the greatest confirm_payment.telephone.end: the confirmation of the
// provider sets only the success rows of the last 48 h (order.LatestWindow; bill:
// UpdateProviderConfirm), so an older order would be sent at each run and never marked.
const ConfirmPaymentMaxEnd = 48 * time.Hour

// validateConfirmPaymentTelephone checks the keys of the settlement worker (task 046). validate
// calls it only while the worker is on.
func (c Config) validateConfirmPaymentTelephone() []error {
	const p = "confirm_payment.telephone."
	var errs []error
	if c.ConfirmPaymentTelephoneName == "" {
		errs = append(errs, errors.New(p+"name must not be empty (the lock key and the metric label)"))
	}
	if c.ConfirmPaymentTelephoneEvery <= 0 {
		errs = append(errs, errors.New(p+"every must be a positive duration (for example 15m)"))
	}
	if c.ConfirmPaymentTelephoneChunk <= 0 {
		errs = append(errs, errors.New(p+"chunk must be positive (for example 100)"))
	}
	if c.ConfirmPaymentTelephoneStart < 0 || c.ConfirmPaymentTelephoneEnd <= c.ConfirmPaymentTelephoneStart {
		errs = append(errs, errors.New(p+"start must not be negative and end must be greater than start (for example 5m and 24h)"))
	}
	if c.ConfirmPaymentTelephoneEnd > ConfirmPaymentMaxEnd {
		errs = append(errs, errors.New(p+"end must not be greater than 48h (the confirmation sets only the rows of the last 48 h)"))
	}
	return errs
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

// OrderTokenKID returns the kid of the order token header (task 042):
// providers.internal.payment_server.kid, or service.name when that key is empty.
func (c Config) OrderTokenKID() string {
	if c.PaymentServerKID != "" {
		return c.PaymentServerKID
	}
	return c.ServiceName
}

// RedisAddr returns host:port.
func (c Config) RedisAddr() string {
	return net.JoinHostPort(c.RedisHost, strconv.Itoa(c.RedisPort))
}

// PostgresTarget returns host:port/db for logs. It has no user and no password.
func (c Config) PostgresTarget() string {
	return net.JoinHostPort(c.PostgresHost, strconv.Itoa(c.PostgresPort)) + "/" + c.PostgresDB
}

// The defaults of the general circuit breaker keys (task 060). The App also falls back to them for
// a hot general value out of range.
const (
	DefaultCircuitBreakerFailureRatio = 30
	DefaultCircuitBreakerWindow       = 2 * time.Minute
	DefaultCircuitBreakerMinCalls     = 20
	DefaultCircuitBreakerOpenTime     = time.Minute
)

// CircuitBreaker is one block of the circuit breaker keys (task 060): the general block, or the
// override of one provider (0: the general value).
type CircuitBreaker struct {
	FailureRatio int
	Window       time.Duration
	MinCalls     int
	OpenTime     time.Duration
}

// GeneralValid reports whether b is valid as the general block: a failure_ratio of 1 to 100, a
// positive window and open_time, and a min_calls of 1 or more.
func (b CircuitBreaker) GeneralValid() bool {
	return b.FailureRatio >= 1 && b.FailureRatio <= 100 && b.Window > 0 && b.MinCalls >= 1 && b.OpenTime > 0
}

// OverrideValid reports whether b is valid as the override of a provider: a failure_ratio of 0 to
// 100 and no negative value (0 takes the general value).
func (b CircuitBreaker) OverrideValid() bool {
	return b.FailureRatio >= 0 && b.FailureRatio <= 100 && b.Window >= 0 && b.MinCalls >= 0 && b.OpenTime >= 0
}

// CircuitBreakerGeneral is the general block (providers.general.circuit_breaker, with no enabled).
func (c Config) CircuitBreakerGeneral() CircuitBreaker {
	return CircuitBreaker{FailureRatio: c.CircuitBreakerFailureRatio, Window: c.CircuitBreakerWindow,
		MinCalls: c.CircuitBreakerMinCalls, OpenTime: c.CircuitBreakerOpenTime}
}

// CircuitBreakerOverrides are the override blocks by the config path of the provider block (for
// example "internal.jiring_switch"). The App maps each to the provider name of the rows.
func (c Config) CircuitBreakerOverrides() map[string]CircuitBreaker {
	return map[string]CircuitBreaker{
		"internal.irancell": {c.IrancellCircuitBreakerFailureRatio, c.IrancellCircuitBreakerWindow,
			c.IrancellCircuitBreakerMinCalls, c.IrancellCircuitBreakerOpenTime},
		"internal.jiring_switch": {c.JiringSwitchCircuitBreakerFailureRatio, c.JiringSwitchCircuitBreakerWindow,
			c.JiringSwitchCircuitBreakerMinCalls, c.JiringSwitchCircuitBreakerOpenTime},
		"internal.parsian": {c.ParsianCircuitBreakerFailureRatio, c.ParsianCircuitBreakerWindow,
			c.ParsianCircuitBreakerMinCalls, c.ParsianCircuitBreakerOpenTime},
		"external.karashop": {c.KarashopCircuitBreakerFailureRatio, c.KarashopCircuitBreakerWindow,
			c.KarashopCircuitBreakerMinCalls, c.KarashopCircuitBreakerOpenTime},
		"external.qabzino": {c.QabzinoCircuitBreakerFailureRatio, c.QabzinoCircuitBreakerWindow,
			c.QabzinoCircuitBreakerMinCalls, c.QabzinoCircuitBreakerOpenTime},
		"external.tavanir": {c.TavanirCircuitBreakerFailureRatio, c.TavanirCircuitBreakerWindow,
			c.TavanirCircuitBreakerMinCalls, c.TavanirCircuitBreakerOpenTime},
	}
}

// circuitBreakerErrors checks the circuit breaker keys at start (task 060).
func (c Config) circuitBreakerErrors() []error {
	var errs []error
	g := c.CircuitBreakerGeneral()
	if g.FailureRatio < 1 || g.FailureRatio > 100 {
		errs = append(errs, errors.New("providers.general.circuit_breaker.failure_ratio must be 1 to 100 (percent)"))
	}
	if g.Window <= 0 {
		errs = append(errs, errors.New("providers.general.circuit_breaker.window must be a positive duration (for example 2m)"))
	}
	if g.MinCalls < 1 {
		errs = append(errs, errors.New("providers.general.circuit_breaker.min_calls must be 1 or more"))
	}
	if g.OpenTime <= 0 {
		errs = append(errs, errors.New("providers.general.circuit_breaker.open_time must be a positive duration (for example 1m)"))
	}
	overrides := c.CircuitBreakerOverrides()
	for _, k := range slices.Sorted(maps.Keys(overrides)) {
		o := overrides[k]
		path := "providers." + k + ".circuit_breaker."
		if o.FailureRatio < 0 || o.FailureRatio > 100 {
			errs = append(errs, errors.New(path+"failure_ratio must be 0 (the general value) to 100"))
		}
		if o.Window < 0 {
			errs = append(errs, errors.New(path+"window must not be negative (0: the general value)"))
		}
		if o.MinCalls < 0 {
			errs = append(errs, errors.New(path+"min_calls must not be negative (0: the general value)"))
		}
		if o.OpenTime < 0 {
			errs = append(errs, errors.New(path+"open_time must not be negative (0: the general value)"))
		}
	}
	return errs
}
