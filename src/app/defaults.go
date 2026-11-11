package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/logging/zap"
	"qabz/common/monitoring/jaeger"
	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/core/billtype"
	"qabz/core/inquiry"
	"qabz/core/savedbill"
	grpcserver "qabz/entry/grpc"
	httpserver "qabz/entry/http"
	"qabz/migrations"
	"qabz/provider"
	"qabz/provider/irancellproxy"
	"qabz/provider/jiring"
	"qabz/provider/parsianproxy"
	"qabz/provider/qabzino"
	"qabz/provider/tavanir"
	"qabz/system/postgres"
	"qabz/system/redis"
)

// NewDefaultBuilder is the composition root: it creates the real zap logger, Postgres, Redis,
// Prometheus, the gRPC server, the HTTP gateway and (when logs.otlp_endpoint is set) tracing from cfg.
// The reloader runs only when Config has a hot_reload:"true" field.
// Connections are lazy, so this does not need running servers; Start pings them.
func NewDefaultBuilder(ctx context.Context, cfg config.Config) (*Builder, error) {
	logger, err := zap.New(cfg.LogLevel, cfg.ServiceName)
	if err != nil {
		return nil, err
	}
	b := NewBuilder().WithLogger(logger)

	// The auth check comes first: a missing secret stops the start before any connection.
	// metrics.prometheus.enabled: false gives a disabled Server; then no metric is created or registered.
	metrics := prometheus.New(cfg.MetricsAddr, cfg.MetricsEnabled)
	authCheck, err := newAuth(cfg, metrics, logger)
	if err == nil && authCheck == nil {
		err = errors.New("auth: newAuth returned nil")
	}
	if err != nil {
		return nil, err
	}

	db, err := postgres.New(ctx, cfg.PostgresDSN())
	if err != nil || db == nil {
		// pgx errors can quote the DSN: report the target only.
		return nil, errors.New("postgres: cannot create pool for " + cfg.PostgresTarget())
	}
	holder := config.NewHolder(cfg)
	b.WithConfigHolder(holder).WithDatabase(db)
	b.WithMigrator(postgres.NewMigrator(cfg.PostgresDSN(), migrations.FS))
	reloader, err := config.NewReloader[config.Config](postgres.NewConfigRepository(db), holder, cfg.HotReloadInterval, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	if reloader.HasHotFields() {
		b.WithConfigReloader(reloader)
	}
	b.WithCache(redis.New(cfg.RedisAddr(), cfg.RedisPassword.Reveal()))
	b.WithMetrics(metrics)

	providers, err := newProviders(holder, postgres.NewProviderRepository(db), metrics, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	b.WithProviders(providers)

	svc, err := newService(cfg, db, metrics, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	myBills, err := newMyBills(cfg, db, metrics, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	registry, err := newProviderRegistry(cfg, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	flow, err := newInquiryFlow(cfg, db, providers, registry, metrics, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	water, electricity, gas, phone := newWater(flow, logger), newElectricity(flow, logger), newGas(flow, logger), newPhone(flow, logger)
	// The request metrics come first in the chain, so a call that the auth check rejects is counted too.
	opts, err := newRequestMetrics(metrics)
	if err != nil {
		db.Close()
		return nil, err
	}
	opts = append(opts, authCheck.ServerOptions()...)
	gs, err := grpcserver.NewWithOptions(cfg.GRPCAddr, opts, grpcserver.RegisterService(svc), grpcserver.RegisterMyBills(myBills), grpcserver.RegisterWater(water), grpcserver.RegisterElectricity(electricity), grpcserver.RegisterGas(gas), grpcserver.RegisterPhone(phone))
	if err == nil && gs == nil {
		err = errors.New("grpc server: New returned nil")
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	gw, err := httpserver.New(cfg.HTTPAddr, gs.DialAddr, pb.RegisterServiceHandler, pb.RegisterMyBillsHandler, pb.RegisterWaterHandler, pb.RegisterElectricityHandler, pb.RegisterGasHandler, pb.RegisterPhoneHandler)
	if err == nil && gw == nil {
		err = errors.New("http gateway: New returned nil")
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	b.WithGRPCServer(gs).WithHTTPServer(gw)

	if cfg.OTLPEndpoint != "" {
		tr, err := jaeger.New(ctx, cfg.OTLPEndpoint, cfg.ServiceName)
		if err != nil {
			db.Close()
			return nil, err
		}
		b.WithTracer(tr)
	}
	return b, nil
}

// newAuth builds the JWT check of every gRPC call (and so of every gateway route). With
// debug.auth.disable it logs one warning, checks nothing and gives every call the user
// debug.auth.mock_user_id (when set); otherwise it needs
// auth.secrets.hmac. Each rejected call adds 1 to
// qabz_auth_rejected_total{reason} (only when metrics are on); a rejected call writes no log line.
func newAuth(cfg config.Config, metrics *prometheus.Server, logger Logger) (*grpcserver.Auth, error) {
	if cfg.DebugAuthDisable {
		logger.Warn("auth check disabled (debug.auth.disable): development only", "mock_user_id", cfg.DebugAuthMockUserID)
		return grpcserver.NewAuth(grpcserver.AuthOptions{Disabled: true, MockUserID: cfg.DebugAuthMockUserID})
	}
	verifier, err := auth.NewVerifier([]byte(cfg.JWTHMACSecret.Reveal()), nil)
	if err != nil {
		return nil, fmt.Errorf("%w (set auth.secrets.hmac, or debug.auth.disable in development)", err)
	}
	var onReject func(reason string) // nil: metrics are off, nothing is recorded
	if reg := metrics.Registry(); reg != nil {
		rejected := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_auth_rejected_total",
			Help: "Calls rejected by the JWT auth check, by reason.",
		}, []string{"reason"})
		if err := reg.Register(rejected); err != nil {
			return nil, fmt.Errorf("auth: register metric: %w", err)
		}
		onReject = func(reason string) { rejected.WithLabelValues(reason).Inc() }
	}
	return grpcserver.NewAuth(grpcserver.AuthOptions{Verifier: verifier, OnReject: onReject})
}

// newService builds the gRPC Service: GET /v1/services reads the services table through an
// in-memory cache (config.CacheServicesTTL). Each call adds 1 to
// qabz_services_cache_total{result="hit"|"miss"|"error"} (only when metrics are on); a failed read
// also writes one error log line.
func newService(cfg config.Config, db *postgres.Client, metrics *prometheus.Server, logger Logger) (*grpcserver.Service, error) {
	serviceID, err := grpcserver.ServiceIDFromConfig(cfg.ServiceID)
	if err != nil {
		return nil, err
	}
	var onResult func(result string) // nil: metrics are off, nothing is recorded
	if reg := metrics.Registry(); reg != nil {
		results := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_services_cache_total",
			Help: "GET /v1/services calls by cache result (hit, miss, error).",
		}, []string{"result"})
		if err := reg.Register(results); err != nil {
			return nil, fmt.Errorf("services: register metric: %w", err)
		}
		onResult = func(result string) { results.WithLabelValues(result).Inc() }
	}
	catalog, err := billtype.NewServiceCatalog(postgres.NewServiceRepository(db), billtype.CatalogOptions{
		TTL:      config.CacheServicesTTL,
		OnResult: onResult,
	})
	if err != nil {
		return nil, err
	}
	return grpcserver.NewService(grpcserver.ServiceDeps{Services: catalog, ServiceID: serviceID, Logger: logger}), nil
}

// newMyBills builds the gRPC MyBills service: POST /v1/mybill saves a bill in the bills table,
// GET /v1/mybill lists the user's bills (serviceId is config service.id) and DELETE
// /v1/mybill/{id} disables a bill of the user. Each saved bill adds 1 to
// qabz_mybill_insert_total{result="created"|"existing"} and each successful delete adds 1 to
// qabz_mybill_delete_total{type,result="found"|"not_found"} (type: bills.type of the bill, or
// "unknown" for not_found and a NULL type) (only when metrics are on); a failed
// write or read writes one error log line. Rejected and failed calls are in the code label of
// qabz_request_duration_ms.
func newMyBills(cfg config.Config, db *postgres.Client, metrics *prometheus.Server, logger Logger) (*grpcserver.MyBills, error) {
	serviceID, err := grpcserver.ServiceIDFromConfig(cfg.ServiceID)
	if err != nil {
		return nil, err
	}
	var onInsert func(result string)           // nil: metrics are off, nothing is recorded
	var onDelete func(billType, result string) // nil: metrics are off, nothing is recorded
	if reg := metrics.Registry(); reg != nil {
		inserts := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_mybill_insert_total",
			Help: "POST /v1/mybill saved bills by result (created, existing).",
		}, []string{"result"})
		if err := reg.Register(inserts); err != nil {
			return nil, fmt.Errorf("mybill: register metric: %w", err)
		}
		onInsert = func(result string) { inserts.WithLabelValues(result).Inc() }
		deletes := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_mybill_delete_total",
			Help: "DELETE /v1/mybill/{id} successful calls by bill type (bills.type; unknown for not_found or a NULL type) and result (found: a bill of the user has the id; not_found).",
		}, []string{"type", "result"})
		if err := reg.Register(deletes); err != nil {
			return nil, fmt.Errorf("mybill: register metric: %w", err)
		}
		onDelete = func(billType, result string) { deletes.WithLabelValues(billType, result).Inc() }
	}
	store, err := savedbill.NewStore(postgres.NewSavedBillRepository(db))
	if err != nil {
		return nil, err
	}
	return grpcserver.NewMyBills(grpcserver.MyBillsDeps{
		Bills:     store,
		Lister:    store,
		Deleter:   store,
		ServiceID: serviceID,
		Logger:    logger,
		OnInsert:  onInsert,
		OnDelete:  onDelete,
	}), nil
}

// newProviders builds the provider list (task 020): ProvidersService on repo (the providers table). App
// Start loads it once before the servers start, then it reloads at providers.general.load_interval,
// read from holder before each wait (hot-reloadable); holder.Changed wakes the routine, so a hot
// change of the effective interval applies at once (operator decision 9). With metrics on, each load adds 1 to
// qabz_providers_loads_total{result="ok"|"error"} and observes qabz_providers_load_duration_ms (an
// aborted load, canceled by Stop or by the App.Start context, is neither counted nor timed); a
// good load sets qabz_providers_last_load_success_timestamp_ms; each MoveUp, MoveDown, Enable and
// Disable adds 1 to qabz_providers_changes_total{change,result}; qabz_providers_active_count is set
// to the number of enabled providers in all groups after each good load and each Enable or Disable
// that changed the list (a failed load keeps the last value). A failed load writes one error log
// line.
func newProviders(holder *config.Holder[config.Config], repo provider.Repository, metrics *prometheus.Server, logger Logger) (*provider.ProvidersService, error) {
	if holder == nil {
		return nil, errors.New("providers: config holder is required")
	}
	var onLoad func(result string, d time.Duration) // nil: metrics are off, nothing is recorded
	var onChange func(change, result string)        // nil: metrics are off, nothing is recorded
	var onActive func(n int)                        // nil: metrics are off, nothing is recorded
	if reg := metrics.Registry(); reg != nil {
		loads := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_providers_loads_total",
			Help: "Loads of the providers table by ProvidersService, by result (ok, error). An aborted load (canceled by Stop or by the App.Start context) is neither counted nor timed.",
		}, []string{"result"})
		duration := promclient.NewHistogram(promclient.HistogramOpts{
			Name:    "qabz_providers_load_duration_ms",
			Help:    "Duration of each load of the providers table (ok and error) in milliseconds. An aborted load (canceled by Stop or by the App.Start context) is neither counted nor timed.",
			Buckets: []float64{1, 2.5, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
		})
		lastOK := promclient.NewGauge(promclient.GaugeOpts{
			Name: "qabz_providers_last_load_success_timestamp_ms",
			Help: "Unix time in milliseconds of the last good load of the providers table (0: none yet).",
		})
		changes := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_providers_changes_total",
			Help: "Runtime changes of the provider list, by change (move_up, move_down, enable, disable) and result (changed, unchanged, unknown_id).",
		}, []string{"change", "result"})
		active := promclient.NewGauge(promclient.GaugeOpts{
			Name: "qabz_providers_active_count",
			Help: "Enabled providers in all groups of the current provider list (set after each good load and each Enable or Disable that changed the list).",
		})
		for _, c := range []promclient.Collector{loads, duration, lastOK, changes, active} {
			if err := reg.Register(c); err != nil {
				return nil, fmt.Errorf("providers: register metric: %w", err)
			}
		}
		onLoad = func(result string, d time.Duration) {
			loads.WithLabelValues(result).Inc()
			duration.Observe(float64(d) / float64(time.Millisecond))
			if result == provider.LoadOK {
				lastOK.Set(float64(time.Now().UnixMilli()))
			}
		}
		onChange = func(change, result string) { changes.WithLabelValues(change, result).Inc() }
		onActive = func(n int) { active.Set(float64(n)) }
	}
	return provider.NewProvidersService(repo, provider.Options{
		Interval:      func() time.Duration { return holder.Get().ProvidersLoadInterval },
		Changed:       holder.Changed,
		Logger:        logger,
		OnLoad:        onLoad,
		OnChange:      onChange,
		OnActiveCount: onActive,
	})
}

// newProviderRegistry builds the provider adapters: Qabzino (task 021; since task 030 the direct
// HTTP adapter with providers.qabzino.enabled, token, water_bill_endpoint, gas_bill_endpoint (task
// 023) and timeout, not the gRPC QabzinoProxy) and Tavanir (task 022; since task 034 the direct HTTP client of the Tavanir
// endpoints with providers.tavanir.enabled, auth_endpoint, basic_branch_endpoint, username, password
// and timeout; it calls no other service) and Irancell (task 035: a gRPC client of the irancell
// service with providers.irancell.enabled, grpc_address and timeout), and Jiring Switch and Parsian
// (task 036, the MCI mobile bill: gRPC clients of the jiring-switch and the parsian-v2 service with
// providers.jiring_switch.* and providers.parsian.*). The mock of a provider
// (debug.mocks.<provider>) is applied only when Config.Mode is dev and its enable key is true (one
// warn line); then Inquiry answers a fixed answer and never calls the backend. Every provider that
// is added later follows the same pattern (registerAdapter) with its own debug.mocks.<provider>. A
// nil logger writes no line.
func newProviderRegistry(cfg config.Config, logger Logger) (*provider.Registry, error) {
	registry := provider.NewRegistry()
	dev := cfg.Mode == config.ModeDev

	qabzinoAdapter, err := qabzino.New(qabzino.Options{
		Enabled:       cfg.QabzinoEnabled,
		Token:         cfg.QabzinoToken.Reveal(),
		WaterEndpoint: cfg.QabzinoWaterBillEndpoint,
		GasEndpoint:   cfg.QabzinoGasBillEndpoint,
		Timeout:       cfg.QabzinoTimeout,
	})
	if err != nil {
		return nil, err
	}
	// providers.qabzino.enabled: false wins over the mock. The adapter then fails every call as
	// "provider down" and the mock must not answer instead of it (task 030).
	qabzinoMock := provider.MockSettings{Enable: dev && cfg.DebugMockQabzinoEnable && cfg.QabzinoEnabled, ReturnType: cfg.DebugMockQabzinoReturnType}
	if err := registerAdapter(registry, qabzinoAdapter, "Qabzino", disabledNotice("qabzino", "Qabzino", cfg.QabzinoEnabled),
		qabzinoMock, qabzino.MockFixtures(), logger); err != nil {
		return nil, err
	}

	tavanirAdapter, err := tavanir.New(tavanir.Options{
		Enabled:             cfg.TavanirEnabled,
		AuthEndpoint:        cfg.TavanirAuthEndpoint,
		BasicBranchEndpoint: cfg.TavanirBasicBranchEndpoint,
		Username:            cfg.TavanirUsername,
		Password:            cfg.TavanirPassword.Reveal(),
		Timeout:             cfg.TavanirTimeout,
	})
	if err != nil {
		return nil, err
	}
	// providers.tavanir.enabled: false wins over the mock, as for Qabzino (task 034).
	tavanirMock := provider.MockSettings{Enable: dev && cfg.DebugMockTavanirEnable && cfg.TavanirEnabled, ReturnType: cfg.DebugMockTavanirReturnType}
	if err := registerAdapter(registry, tavanirAdapter, "Tavanir", disabledNotice("tavanir", "Tavanir", cfg.TavanirEnabled),
		tavanirMock, tavanir.MockFixtures(), logger); err != nil {
		return nil, err
	}

	// Irancell (task 035): a gRPC client of the irancell service (our own service). The connection
	// is lazy: the start does not need the service.
	irancellClient, err := irancellproxy.Dial(cfg.IrancellGRPCAddress)
	if err != nil {
		return nil, err
	}
	irancellAdapter, err := irancellproxy.New(irancellproxy.Options{
		Enabled: cfg.IrancellEnabled,
		Client:  irancellClient,
		Timeout: cfg.IrancellTimeout,
	})
	if err != nil {
		return nil, err
	}
	// providers.irancell.enabled: false wins over the mock, as for Qabzino and Tavanir.
	irancellMock := provider.MockSettings{Enable: dev && cfg.DebugMockIrancellEnable && cfg.IrancellEnabled, ReturnType: cfg.DebugMockIrancellReturnType}
	if err := registerAdapter(registry, irancellAdapter, "Irancell", disabledNotice("irancell", "Irancell", cfg.IrancellEnabled),
		irancellMock, irancellproxy.MockFixtures(), logger); err != nil {
		return nil, err
	}

	// Jiring Switch (task 036): a gRPC client of the jiring-switch service (our own service). The
	// connection is lazy: the start does not need the service.
	jiringClient, err := jiring.Dial(cfg.JiringSwitchGRPCAddress)
	if err != nil {
		return nil, err
	}
	jiringAdapter, err := jiring.New(jiring.Options{
		Enabled: cfg.JiringSwitchEnabled,
		Client:  jiringClient,
		Timeout: cfg.JiringSwitchTimeout,
	})
	if err != nil {
		return nil, err
	}
	// providers.jiring_switch.enabled: false wins over the mock, as for the other providers.
	jiringMock := provider.MockSettings{Enable: dev && cfg.DebugMockJiringSwitchEnable && cfg.JiringSwitchEnabled, ReturnType: cfg.DebugMockJiringSwitchReturnType}
	if err := registerAdapter(registry, jiringAdapter, "Jiring Switch", disabledNotice("jiring_switch", "Jiring Switch", cfg.JiringSwitchEnabled),
		jiringMock, jiring.MockFixtures(), logger); err != nil {
		return nil, err
	}

	// Parsian (task 036): a gRPC client of the parsian-v2 service (our own service), lazy too.
	parsianClient, err := parsianproxy.Dial(cfg.ParsianGRPCAddress)
	if err != nil {
		return nil, err
	}
	parsianAdapter, err := parsianproxy.New(parsianproxy.Options{
		Enabled: cfg.ParsianEnabled,
		Client:  parsianClient,
		Timeout: cfg.ParsianTimeout,
	})
	if err != nil {
		return nil, err
	}
	// providers.parsian.enabled: false wins over the mock, as for the other providers.
	parsianMock := provider.MockSettings{Enable: dev && cfg.DebugMockParsianEnable && cfg.ParsianEnabled, ReturnType: cfg.DebugMockParsianReturnType}
	if err := registerAdapter(registry, parsianAdapter, "Parsian", disabledNotice("parsian", "Parsian", cfg.ParsianEnabled),
		parsianMock, parsianproxy.MockFixtures(), logger); err != nil {
		return nil, err
	}
	return registry, nil
}

// registerAdapter wraps the adapter with its mock (provider.NewMock: off returns the adapter
// itself) and adds it to the registry. title is the name of the provider in the warn lines (for
// example "Qabzino"); the config keys use adapter.Name() with "_" for "-" (the provider
// "jiring-switch" has the key jiring_switch). It writes one warn line when the mock is
// on, else one with notice when notice is not empty (the provider cannot work as configured). A
// nil logger writes no line.
func registerAdapter(registry *provider.Registry, adapter provider.ProviderInterface, title string, notice string, mock provider.MockSettings, fixtures provider.MockFixtures, logger Logger) error {
	if registry == nil || adapter == nil {
		return errors.New("providers: registry and adapter are required")
	}
	var mockLog provider.MockLogger // nil: no line
	if logger != nil {
		mockLog = logger
		name := strings.ReplaceAll(adapter.Name(), "-", "_")
		switch {
		case mock.Enable:
			logger.Warn("provider mock enabled (debug.mocks."+name+"): development only, no call to the "+title+" backend", "return_type", mock.ReturnType)
		case notice != "":
			logger.Warn(notice)
		}
	}
	p, err := provider.NewMock(adapter, mock, fixtures, mockLog)
	if err != nil {
		return err
	}
	return registry.Register(p)
}

// disabledNotice is the warn text of a provider with providers.<name>.enabled false ("" when
// enabled). title is the name of the provider in the text (for example "Qabzino").
func disabledNotice(name, title string, enabled bool) string {
	if enabled {
		return ""
	}
	return "providers." + name + ".enabled is false: every inquiry to " + title + " fails as provider down; the " + title + " mock does not apply"
}

// newInquiryFlow builds the inquiry flow (inquiry.Service) that every inquiry endpoint uses (water:
// task 021; electricity: task 022; gas: task 023; the Irancell mobile bill: task 035, caching.irancell; the MCI mobile bill: task 036, caching.mobile). It picks the provider with ProvidersService, answers from the
// saved inquiries of bill_inquiries when they are younger than the cache TTL of the bill type
// (caching.water, caching.electricity, caching.gas; read at start), else calls the adapter of the provider and
// saves the answer. With metrics on, each inquiry adds 1 to
// qabz_inquiries_total{bill_type,provider,result} (result: db_hit, provider_ok, provider_refused,
// provider_down, no_provider, canceled, error, raw; provider "none" when no provider was picked); each
// provider call adds 1 to qabz_provider_calls_total{provider,action,result} (ok, refused, down,
// canceled, error) and observes qabz_provider_call_duration_ms{provider,action}. Rejected calls are
// in the code label of qabz_request_duration_ms. A failed lookup or copy or save of inquiries
// writes one error log line.
func newInquiryFlow(cfg config.Config, db *postgres.Client, providers inquiry.Selector, adapters inquiry.Adapters, metrics *prometheus.Server, logger Logger) (*inquiry.Service, error) {
	var onInquiry func(billType, providerName, result string)             // nil: metrics are off, nothing is recorded
	var onCall func(providerName, action, result string, d time.Duration) // nil: metrics are off, nothing is recorded
	if reg := metrics.Registry(); reg != nil {
		inquiries := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_inquiries_total",
			Help: "Bill inquiries by bill type (water, electricity, gas, irancell, mobile), provider (none when no provider was picked) and result (db_hit: answered from saved inquiries; provider_ok; provider_refused: the provider has no inquiry; provider_down: the call failed; no_provider; canceled: the client left during the provider call; error; raw: a gas request with billId and payId, answered from the two IDs with no provider).",
		}, []string{"bill_type", "provider", "result"})
		calls := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_provider_calls_total",
			Help: "Calls to a provider by provider, action (inquiry) and result (ok, refused: the provider has no inquiry, down: the call failed, canceled: the client left, error).",
		}, []string{"provider", "action", "result"})
		duration := promclient.NewHistogramVec(promclient.HistogramOpts{
			Name:    "qabz_provider_call_duration_ms",
			Help:    "Duration of each call to a provider in milliseconds, by provider and action (inquiry), for every result.",
			Buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000},
		}, []string{"provider", "action"})
		for _, c := range []promclient.Collector{inquiries, calls, duration} {
			if err := reg.Register(c); err != nil {
				return nil, fmt.Errorf("inquiry: register metric: %w", err)
			}
		}
		onInquiry = func(billType, providerName, result string) {
			inquiries.WithLabelValues(billType, providerName, result).Inc()
		}
		onCall = func(providerName, action, result string, d time.Duration) {
			calls.WithLabelValues(providerName, action, result).Inc()
			duration.WithLabelValues(providerName, action).Observe(float64(d) / float64(time.Millisecond))
		}
	}
	var flowLog inquiry.Logger // nil: no line
	if logger != nil {
		flowLog = logger
	}
	ttl := map[string]time.Duration{
		provider.BillTypeWater:       cfg.CachingWater,
		provider.BillTypeElectricity: cfg.CachingElectricity,
		provider.BillTypeGas:         cfg.CachingGas,
		provider.BillTypeIrancell:    cfg.CachingIrancell,
		provider.BillTypeMobile:      cfg.CachingMobile,
	}
	return inquiry.NewService(inquiry.Options{
		Repository:     postgres.NewInquiryRepository(db),
		Providers:      providers,
		Adapters:       adapters,
		CacheTTL:       func(billType string) time.Duration { return ttl[billType] }, // read-only map; no entry: 0, no cache
		Logger:         flowLog,
		OnInquiry:      onInquiry,
		OnProviderCall: onCall,
	})
}

// newWater builds the gRPC Water service: POST /v1/water/inquiry (task 021) on the inquiry flow. A
// server fault writes one error log line (grpcserver.Water). A nil flow gives a service whose
// Inquiry answers Unimplemented.
func newWater(flow *inquiry.Service, logger Logger) *grpcserver.Water {
	deps := grpcserver.WaterDeps{}
	if flow != nil {
		deps.Inquiry = flow
	}
	if logger != nil {
		deps.Logger = logger
	}
	return grpcserver.NewWater(deps)
}

// newElectricity builds the gRPC Electricity service: POST /v1/electricity/inquiry (task 022) on
// the same inquiry flow as Water. A server fault writes one error log line
// (grpcserver.Electricity). A nil flow gives a service whose Inquiry answers Unimplemented.
func newElectricity(flow *inquiry.Service, logger Logger) *grpcserver.Electricity {
	deps := grpcserver.ElectricityDeps{}
	if flow != nil {
		deps.Inquiry = flow
	}
	if logger != nil {
		deps.Logger = logger
	}
	return grpcserver.NewElectricity(deps)
}

// newGas builds the gRPC Gas service: POST /v1/gas/inquiry (task 023) on the same inquiry flow as
// Water; the flow also answers the raw inquiry (a request with billId and payId). A server fault writes one error log line (grpcserver.Gas). A nil flow gives a service
// whose Inquiry answers Unimplemented.
func newGas(flow *inquiry.Service, logger Logger) *grpcserver.Gas {
	deps := grpcserver.GasDeps{}
	if flow != nil {
		deps.Inquiry = flow
		deps.Raw = flow
	}
	if logger != nil {
		deps.Logger = logger
	}
	return grpcserver.NewGas(deps)
}

// newPhone builds the gRPC Phone service: POST /v1/mobile/inquiry (task 035: the Irancell operator; task 036: the MCI operator)
// on the same inquiry flow as Water. A server fault writes one error log line (grpcserver.Phone).
// A nil flow gives a service whose Inquiry answers Unimplemented.
func newPhone(flow *inquiry.Service, logger Logger) *grpcserver.Phone {
	deps := grpcserver.PhoneDeps{}
	if flow != nil {
		deps.Inquiry = flow
	}
	if logger != nil {
		deps.Logger = logger
	}
	return grpcserver.NewPhone(deps)
}

// newRequestMetrics returns the gRPC server options that measure every call (and so every gateway
// route): qabz_request_duration_ms{path,code}, whose _count series counts the requests. With metrics
// off it returns no option, so nothing is registered or recorded.
func newRequestMetrics(metrics *prometheus.Server) ([]grpc.ServerOption, error) {
	m, err := prometheus.NewRequestMetrics(metrics)
	if err != nil || m == nil {
		return nil, err
	}
	requests, err := grpcserver.NewRequests(m.Observe)
	if err != nil {
		return nil, err
	}
	if requests == nil {
		return nil, errors.New("requests: NewRequests returned nil")
	}
	return requests.ServerOptions(), nil
}
