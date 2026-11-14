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
	// QabzinoElectricityBillEndpoint, QabzinoGasBillEndpoint, QabzinoRightelBillEndpoint and
	// QabzinoWaterBillEndpoint are the Ayantech inquiry URLs (bill: qabzino.electricityBillEndpoint
	// and so on; the qabzino service: rightelBillEndpoint). Not checked. The water one (task 030),
	// the gas one (task 023) and the Rightel one (task 037) are read; the electricity one is unused so far.
	// No default (task 036, operator): the real URLs are only in config.yml; an empty URL makes
	// every call fail as "provider down".
	QabzinoElectricityBillEndpoint string `path:"providers.qabzino.electricity_bill_endpoint"`
	QabzinoGasBillEndpoint         string `path:"providers.qabzino.gas_bill_endpoint"`
	QabzinoRightelBillEndpoint     string `path:"providers.qabzino.rightel_bill_endpoint"`
	QabzinoWaterBillEndpoint       string `path:"providers.qabzino.water_bill_endpoint"`

	// TavanirEnabled turns the Tavanir provider on (task 034). False (the default): every call to
	// Tavanir fails as "provider down", nothing is sent to Tavanir and the mock
	// (debug.mocks.tavanir) does not apply. Not hot-reloadable.
	TavanirEnabled bool `path:"providers.tavanir.enabled"`
	// TavanirTimeout limits each HTTP call to Tavanir (the login and the inquiry each). Positive. Not hot-reloadable.
	TavanirTimeout time.Duration `path:"providers.tavanir.timeout"`
	// The config of the Tavanir adapter (task 033; the tavanir service: tavanir.authEndpoint and so
	// on). The adapter (task 034) uses auth_endpoint, basic_branch_endpoint, username and password;
	// bill_document_endpoint and branch_debit_endpoint are not used yet. The password is a Secret
	// (never printed). The endpoints have no default (task 036, operator): the real URLs are only in
	// config.yml; an empty auth or basic branch URL makes every call fail as "provider down".
	TavanirAuthEndpoint         string `path:"providers.tavanir.auth_endpoint"`
	TavanirBasicBranchEndpoint  string `path:"providers.tavanir.basic_branch_endpoint"`
	TavanirBillDocumentEndpoint string `path:"providers.tavanir.bill_document_endpoint"`
	TavanirBranchDebitEndpoint  string `path:"providers.tavanir.branch_debit_endpoint"`
	TavanirUsername             string `path:"providers.tavanir.username"`
	TavanirPassword             Secret `path:"providers.tavanir.password"`
	// @todo circuit breaker: no provider has one yet (operator decision of task 034: every provider
	// gets one at the end of the project). Task 040 removed the unused keys
	// providers.tavanir.circuit_breaker.* (name "tavanir", max_requests 100000, interval 1s, timeout
	// 10s, failure_ratio 99 percent, as the tavanir service). When the circuit breaker feature lands,
	// add its keys for every provider (in config.yml and config.yml.example, conventions § 8) and the
	// metrics qabz_circuit_breaker_state and qabz_circuit_breaker_transitions_total (system.html,
	// Metrics). A config.yml that still has the old block is valid: the decoder reads only tagged paths.

	// The config of the Irancell provider (task 035): the keys that bill has for its irancell
	// service client (bill: irancell.name, irancell.grpcAddress, irancell.natsSubject and the
	// irancell.getStatus block), in the layout of this project, and the switch and the timeout of the
	// adapter. The bill keys have no default: the real values are only in config.yml. Not
	// hot-reloadable.
	//   - IrancellGRPCAddress is the host:port of the internal gRPC server of the irancell service
	//     (pb.InternalService, no TLS, as in bill). The Irancell adapter (provider/irancellproxy)
	//     reads it. Empty: every Irancell call fails as "provider down".
	//   - IrancellName and IrancellNATSSubject (the subject of the bill payment) are not read yet:
	//     they belong to the order flow.
	//   - The get_status keys are the settings of bill's cron that asks the status of pending Irancell
	//     orders. Not read yet: Qabz has no order flow and no such cron. The three sync_operations
	//     keys are bill's list getStatus.syncOperations (one single-key map for each item) as three
	//     keys, because a Config field maps one path.
	//   - IrancellEnabled turns the Irancell provider on. False (the default): every call fails as
	//     "provider down", nothing is sent to the irancell service and the mock
	//     (debug.mocks.irancell) does not apply, as for Qabzino and Tavanir.
	//   - IrancellTimeout limits each call to the irancell service. Positive.
	IrancellEnabled                  bool          `path:"providers.irancell.enabled"`
	IrancellTimeout                  time.Duration `path:"providers.irancell.timeout"`
	IrancellName                     string        `path:"providers.irancell.name"`
	IrancellGRPCAddress              string        `path:"providers.irancell.grpc_address"`
	IrancellNATSSubject              string        `path:"providers.irancell.nats_subject"`
	IrancellGetStatusCronTime        time.Duration `path:"providers.irancell.get_status.cron_time"`
	IrancellGetStatusName            string        `path:"providers.irancell.get_status.name"`
	IrancellGetStatusStart           time.Duration `path:"providers.irancell.get_status.start"`
	IrancellGetStatusEnd             time.Duration `path:"providers.irancell.get_status.end"`
	IrancellGetStatusChunk           int           `path:"providers.irancell.get_status.chunk"`
	IrancellGetStatusEvery           time.Duration `path:"providers.irancell.get_status.every"`
	IrancellGetStatusLockTime        time.Duration `path:"providers.irancell.get_status.lock_time"`
	IrancellGetStatusIsLockable      bool          `path:"providers.irancell.get_status.is_lockable"`
	IrancellGetStatusReverseDeadline time.Duration `path:"providers.irancell.get_status.reverse_deadline"`
	IrancellGetStatusSyncAPICall     time.Duration `path:"providers.irancell.get_status.sync_operations.api_call_get_status"`
	IrancellGetStatusSyncGetOrder    time.Duration `path:"providers.irancell.get_status.sync_operations.get_order_query"`
	IrancellGetStatusSyncInsertOrder time.Duration `path:"providers.irancell.get_status.sync_operations.insert_order_query"`

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
	// layout of this project, as for Irancell. They have no default (the real values are only in
	// config.yml) and are not read yet: they belong to the order flow and its status cron. The three
	// sync_operations keys are bill's list getStatus.syncOperations as three keys, as for Irancell.
	// Not hot-reloadable.
	JiringSwitchEnabled     bool          `path:"providers.jiring_switch.enabled"`
	JiringSwitchTimeout     time.Duration `path:"providers.jiring_switch.timeout"`
	JiringSwitchGRPCAddress string        `path:"providers.jiring_switch.grpc_address"`
	ParsianEnabled          bool          `path:"providers.parsian.enabled"`
	ParsianTimeout          time.Duration `path:"providers.parsian.timeout"`
	ParsianGRPCAddress      string        `path:"providers.parsian.grpc_address"`

	JiringSwitchName                     string        `path:"providers.jiring_switch.name"`
	JiringSwitchGetStatusName            string        `path:"providers.jiring_switch.get_status.name"`
	JiringSwitchGetStatusChunk           int           `path:"providers.jiring_switch.get_status.chunk"`
	JiringSwitchGetStatusStart           time.Duration `path:"providers.jiring_switch.get_status.start"`
	JiringSwitchGetStatusEnd             time.Duration `path:"providers.jiring_switch.get_status.end"`
	JiringSwitchGetStatusCronTime        time.Duration `path:"providers.jiring_switch.get_status.cron_time"`
	JiringSwitchGetStatusLockTime        time.Duration `path:"providers.jiring_switch.get_status.lock_time"`
	JiringSwitchGetStatusIsLockable      bool          `path:"providers.jiring_switch.get_status.is_lockable"`
	JiringSwitchGetStatusReverseDeadline time.Duration `path:"providers.jiring_switch.get_status.reverse_deadline"`
	JiringSwitchGetStatusSyncAPICall     time.Duration `path:"providers.jiring_switch.get_status.sync_operations.api_call_get_status"`
	JiringSwitchGetStatusSyncGetOrder    time.Duration `path:"providers.jiring_switch.get_status.sync_operations.get_order_query"`
	JiringSwitchGetStatusSyncInsertOrder time.Duration `path:"providers.jiring_switch.get_status.sync_operations.insert_order_query"`

	ParsianName                           string        `path:"providers.parsian.name"`
	ParsianGetStatusName                  string        `path:"providers.parsian.get_status.name"`
	ParsianGetStatusChunk                 int           `path:"providers.parsian.get_status.chunk"`
	ParsianGetStatusStart                 time.Duration `path:"providers.parsian.get_status.start"`
	ParsianGetStatusEnd                   time.Duration `path:"providers.parsian.get_status.end"`
	ParsianGetStatusCronTime              time.Duration `path:"providers.parsian.get_status.cron_time"`
	ParsianGetStatusLockTime              time.Duration `path:"providers.parsian.get_status.lock_time"`
	ParsianGetStatusIsLockable            bool          `path:"providers.parsian.get_status.is_lockable"`
	ParsianGetStatusReverseDeadline       time.Duration `path:"providers.parsian.get_status.reverse_deadline"`
	ParsianGetStatusSyncAPICall           time.Duration `path:"providers.parsian.get_status.sync_operations.api_call_get_status"`
	ParsianGetStatusSyncGetOrder          time.Duration `path:"providers.parsian.get_status.sync_operations.get_order_query"`
	ParsianGetStatusSyncInsertOrder       time.Duration `path:"providers.parsian.get_status.sync_operations.insert_order_query"`
	ParsianSettlementInquiryWater         time.Duration `path:"providers.parsian.settlement_inquiry_water"`
	ParsianSettlementInquiryElectricity   time.Duration `path:"providers.parsian.settlement_inquiry_electricity"`
	ParsianSettlementInquiryGas           time.Duration `path:"providers.parsian.settlement_inquiry_gas"`
	ParsianSettlementInquiryTelephone     time.Duration `path:"providers.parsian.settlement_inquiry_telephone"`
	ParsianSettlementInquiryMCIMobile     time.Duration `path:"providers.parsian.settlement_inquiry_mci_mobile"`
	ParsianSettlementInquiryRightelMobile time.Duration `path:"providers.parsian.settlement_inquiry_rightel_mobile"`

	// The config of the Karashop provider (task 025, the landline bill; bill: thirdparty.karashop):
	// the adapter (provider/karashop) calls the Karashop HTTPS API itself.
	//   - KarashopEnabled turns the provider on. False (the default): every call fails as "provider
	//     down", nothing is sent to Karashop and the mock (debug.mocks.karashop) does not apply.
	//   - KarashopTimeout limits each Karashop call (an inquiry is two calls, sent at the same time).
	//     Positive. Default 30s (bill: karashop.timeout).
	//   - KarashopCode and KarashopPassword are the credential of every call; the password is a
	//     Secret (never printed). KarashopBillInquiryURL is the inquiry URL. They have no default:
	//     the real values are only in config.yml. An empty one makes every call fail as "provider down".
	//   - KarashopBillPaymentInfoURL (bill: billPaymentInfoURL) is not read yet: it belongs to the
	//     landline order flow (SendPaymentInfo).
	// Not hot-reloadable.
	KarashopEnabled            bool          `path:"providers.karashop.enabled"`
	KarashopTimeout            time.Duration `path:"providers.karashop.timeout"`
	KarashopCode               string        `path:"providers.karashop.code"`
	KarashopPassword           Secret        `path:"providers.karashop.password"`
	KarashopBillInquiryURL     string        `path:"providers.karashop.bill_inquiry_url"`
	KarashopBillPaymentInfoURL string        `path:"providers.karashop.bill_payment_info_url"`

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

	// DebugMockIrancellEnable makes the Irancell adapter answer with a fixed answer, with no call to
	// the irancell service (task 035). Development only, as DebugMockQabzinoEnable. Not hot-reloadable.
	DebugMockIrancellEnable bool `path:"debug.mocks.irancell.enable"`
	// DebugMockIrancellReturnType is the answer of the Irancell mock: MockReturnSuccess or
	// MockReturnError. It has no effect while the mock is off. Development only.
	DebugMockIrancellReturnType string `path:"debug.mocks.irancell.return_type"`

	// DebugMockJiringSwitchEnable and DebugMockParsianEnable make the Jiring Switch and the Parsian
	// adapter answer with a fixed answer, with no call to the jiring-switch or the parsian-v2
	// service (task 036). Development only, as DebugMockQabzinoEnable. Not hot-reloadable. The
	// return types are MockReturnSuccess or MockReturnError; no effect while the mock is off.
	DebugMockJiringSwitchEnable     bool   `path:"debug.mocks.jiring_switch.enable"`
	DebugMockJiringSwitchReturnType string `path:"debug.mocks.jiring_switch.return_type"`
	DebugMockParsianEnable          bool   `path:"debug.mocks.parsian.enable"`
	DebugMockParsianReturnType      string `path:"debug.mocks.parsian.return_type"`

	// DebugMockKarashopEnable makes the Karashop adapter answer with a fixed answer, with no call to
	// Karashop (task 025). Development only, as DebugMockQabzinoEnable. Not hot-reloadable. The
	// return type is MockReturnSuccess or MockReturnError; no effect while the mock is off.
	DebugMockKarashopEnable     bool   `path:"debug.mocks.karashop.enable"`
	DebugMockKarashopReturnType string `path:"debug.mocks.karashop.return_type"`

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

		QabzinoTimeout:      10 * time.Second,
		TavanirTimeout:      10 * time.Second,
		IrancellTimeout:     10 * time.Second,
		JiringSwitchTimeout: 10 * time.Second,
		ParsianTimeout:      10 * time.Second,
		KarashopTimeout:     30 * time.Second,

		CachingWater:                    time.Hour,
		CachingElectricity:              time.Hour,
		CachingGas:                      time.Hour,
		CachingMobile:                   5 * time.Hour,
		CachingTel:                      time.Hour,
		DebugMockQabzinoReturnType:      MockReturnSuccess,
		DebugMockTavanirReturnType:      MockReturnSuccess,
		DebugMockIrancellReturnType:     MockReturnSuccess,
		DebugMockJiringSwitchReturnType: MockReturnSuccess,
		DebugMockParsianReturnType:      MockReturnSuccess,
		DebugMockKarashopReturnType:     MockReturnSuccess,
		DebugCacheInquiries:             true,
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
	if c.IrancellTimeout <= 0 {
		errs = append(errs, errors.New("providers.irancell.timeout must be a positive duration (for example 10s)"))
	}
	if c.CachingIrancell < 0 {
		errs = append(errs, errors.New("caching.irancell must not be negative (0 turns the cache off)"))
	}
	if !ValidMockReturnType(c.DebugMockIrancellReturnType) {
		errs = append(errs, fmt.Errorf("debug.mocks.irancell.return_type must be %q or %q", MockReturnSuccess, MockReturnError))
	}
	if c.JiringSwitchTimeout <= 0 {
		errs = append(errs, errors.New("providers.jiring_switch.timeout must be a positive duration (for example 10s)"))
	}
	if c.ParsianTimeout <= 0 {
		errs = append(errs, errors.New("providers.parsian.timeout must be a positive duration (for example 10s)"))
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
		errs = append(errs, errors.New("providers.karashop.timeout must be a positive duration (for example 30s)"))
	}
	if c.CachingTel < 0 {
		errs = append(errs, errors.New("caching.tel must not be negative (0 turns the cache off)"))
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
