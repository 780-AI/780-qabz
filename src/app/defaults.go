package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/crypto"
	"qabz/common/logging/zap"
	"qabz/common/monitoring/jaeger"
	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/core/billtype"
	"qabz/core/fee"
	"qabz/core/inquiry"
	"qabz/core/order"
	"qabz/core/savedbill"
	grpcserver "qabz/entry/grpc"
	httpserver "qabz/entry/http"
	"qabz/migrations"
	"qabz/provider"
	"qabz/provider/irancellproxy"
	"qabz/provider/jiring"
	"qabz/provider/karashop"
	"qabz/provider/parsianproxy"
	"qabz/provider/qabzino"
	"qabz/provider/tavanir"
	"qabz/system/postgres"
	"qabz/system/redis"
)

// NewDefaultBuilder is the composition root: it creates the real zap logger, Postgres, Redis,
// Prometheus, the gRPC server, the HTTP gateway and (when logs.tracing.enabled is true) tracing from cfg.
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
	cache := redis.New(cfg.RedisAddr(), cfg.RedisPassword.Reveal(), cfg.RedisPrefix)
	b.WithCache(cache)
	b.WithMetrics(metrics)

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
	// The NATS client of the Irancell payment (task 056); nil when it is not needed. A later failed
	// step closes it (built is set only at the end).
	natsClient, err := newNATS(cfg, metrics, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	built := false
	var payer irancellproxy.Requester // nil: no NATS client (an untyped nil, not a nil *nats.Client)
	if natsClient != nil {
		payer = natsClient
		b.WithBroker(natsClient)
		defer func() {
			if !built {
				_ = natsClient.Close(ctx)
			}
		}()
	}
	registry, err := newProviderRegistryWith(cfg, logger, payer)
	if err != nil {
		db.Close()
		return nil, err
	}
	// The provider list after the adapters (task 059): the Provider Switcher asks which rows have an
	// adapter (providerAvailable).
	providers, err := newProviders(holder, postgres.NewProviderRepository(db), metrics, logger, providerAvailable(registry, payRoutes()))
	if err != nil {
		db.Close()
		return nil, err
	}
	b.WithProviders(providers)
	flow, err := newInquiryFlow(cfg, db, providers, registry, metrics, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	orders, err := newOrders(cfg, db, providers, flow, metrics, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	payments, err := newPayments(db, registry, providers, metrics, logger)
	if err != nil {
		db.Close()
		return nil, err
	}
	// A missing or bad key stops the start (task 039): every order needs the signature.
	signer, err := crypto.LoadSigner(cfg.PaymentServerPrivateKeyPath, cfg.OrderTokenKID())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("order token: %w (set providers.internal.payment_server.private_key_path)", err)
	}
	serviceID, err := grpcserver.ServiceIDFromConfig(cfg.ServiceID)
	if err != nil {
		db.Close()
		return nil, err
	}
	water := newWater(flow, orders, signer, uint32(serviceID), logger)
	electricity := newElectricity(flow, orders, signer, uint32(serviceID), logger)
	gas := newGas(flow, orders, signer, uint32(serviceID), logger)
	tel := newTel(flow, orders, signer, uint32(serviceID), logger)
	phone := newPhone(flow, orders, signer, uint32(serviceID), logger)
	// The request metrics come first in the chain, so a call that the auth check rejects is counted too.
	metricOpts, err := newRequestMetrics(metrics)
	if err != nil {
		db.Close()
		return nil, err
	}
	// The tracing handler (task 047, conventions § 17) is the first option of both servers: one server
	// span for each call, named with the full gRPC method, a child of the traceparent in the metadata.
	metricOpts = append([]grpc.ServerOption{grpc.StatsHandler(otelgrpc.NewServerHandler())}, metricOpts...)
	opts := append(append([]grpc.ServerOption(nil), metricOpts...), authCheck.ServerOptions()...)
	gs, err := grpcserver.NewWithOptions(cfg.GRPCAddr, opts, grpcserver.RegisterService(svc), grpcserver.RegisterMyBills(myBills), grpcserver.RegisterWater(water), grpcserver.RegisterElectricity(electricity), grpcserver.RegisterGas(gas), grpcserver.RegisterPhone(phone), grpcserver.RegisterTel(tel))
	if err == nil && gs == nil {
		err = errors.New("grpc server: New returned nil")
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	gw, err := httpserver.New(cfg.HTTPAddr, gs.DialAddr, pb.RegisterServiceHandler, pb.RegisterMyBillsHandler, pb.RegisterWaterHandler, pb.RegisterElectricityHandler, pb.RegisterGasHandler, pb.RegisterPhoneHandler, pb.RegisterTelHandler)
	if err == nil && gw == nil {
		err = errors.New("http gateway: New returned nil")
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	b.WithGRPCServer(gs).WithHTTPServer(gw).WithBackground(payments)

	// The internal gRPC server (task 044; bill: service.internalGrpc): PaymentComplete only, with the
	// tracing, the request metrics and no auth check, as bill. Off while server.internal_grpc.addr is empty.
	if cfg.InternalGRPCAddr != "" {
		internal, err := grpcserver.NewWithOptions(cfg.InternalGRPCAddr, metricOpts, grpcserver.RegisterPaymentComplete(grpcserver.NewPaymentComplete(grpcserver.PaymentCompleteDeps{
			Payments: payments, ServiceID: uint32(serviceID), EcosystemName: cfg.ServiceEcosystemName, Logger: logger,
		})))
		if err == nil && internal == nil {
			err = errors.New("internal grpc server: New returned nil")
		}
		if err != nil {
			db.Close()
			return nil, err
		}
		b.WithInternalGRPCServer(internal)
	} else {
		logger.Warn("internal gRPC server off (server.internal_grpc.addr is empty): the payment service cannot call OrderComplete")
	}

	if cfg.TracingEnabled {
		tr, err := jaeger.New(ctx, cfg.TracingOTLPEndpoint, cfg.ServiceName)
		if err != nil {
			db.Close()
			return nil, err
		}
		b.WithTracer(tr)
	}

	// The workers (tasks 045, 046 and 050): the metrics exist with metrics on; the Parsian status
	// worker only with providers.internal.parsian.get_status.enabled, the Irancell status worker only
	// with providers.internal.irancell.get_status.enabled, and their Kafka producer only with one of
	// them; the settlement worker only with confirm_payment.telephone.enabled. Built last, so no later
	// failed step leaves the producer open.
	cronMetrics, err := newCronMetrics(metrics)
	if err != nil {
		db.Close()
		return nil, err
	}
	if !cfg.ParsianGetStatusEnabled {
		logger.Info("parsian status worker off (providers.internal.parsian.get_status.enabled: false)")
	}
	if !cfg.IrancellGetStatusEnabled {
		logger.Info("irancell status worker off (providers.internal.irancell.get_status.enabled: false)")
	}
	if !cfg.ConfirmPaymentTelephoneEnabled {
		logger.Info("settlement worker off (confirm_payment.telephone.enabled: false)")
	}
	if !cfg.JiringSwitchGetStatusEnabled {
		logger.Info("jiring switch status worker off (providers.internal.jiring_switch.get_status.enabled: false)")
	}
	if cfg.StatusWorkerEnabled() || cfg.ConfirmPaymentTelephoneEnabled {
		jobs, err := newCrons(cfg, db, cache, registry, cronMetrics, logger)
		if err != nil {
			db.Close()
			if b.tracer != nil {
				_ = b.tracer.Shutdown(ctx)
			}
			return nil, err
		}
		b.WithCrons(jobs)
	}
	built = true
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
//
// The Provider Switcher (task 059): available tells which rows have an adapter (nil: every enabled
// row). With metrics on, each change adds 1 to qabz_provider_switches_total{bill_type,action,result}
// (demoted, no_next, restored, redemoted, cleared); qabz_provider_demoted{provider,bill_type,action}
// is 1 while the row is demoted and 0 after it returns; each end of a trial adds 1 to
// qabz_provider_trial_total{bill_type,action,result} (ok, failed, released).
func newProviders(holder *config.Holder[config.Config], repo provider.Repository, metrics *prometheus.Server, logger Logger, available func(provider.Provider) bool) (*provider.ProvidersService, error) {
	if holder == nil {
		return nil, errors.New("providers: config holder is required")
	}
	var onLoad func(result string, d time.Duration)             // nil: metrics are off, nothing is recorded
	var onChange func(change, result string)                    // nil: metrics are off, nothing is recorded
	var onActive func(n int)                                    // nil: metrics are off, nothing is recorded
	var onSwitch, onTrial func(billType, action, result string) // nil: metrics are off
	var onDemoted func(p provider.Provider, demoted bool)       // nil: metrics are off
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
		switches := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_provider_switches_total",
			Help: "Changes of the Provider Switcher (task 059) by bill type, action (inquiry, order) and result (demoted; no_next: no next available row, nothing changed; restored: back in its place; redemoted: a failed trial; cleared: a load or Disable removed or disabled the row).",
		}, []string{"bill_type", "action", "result"})
		demoted := promclient.NewGaugeVec(promclient.GaugeOpts{
			Name: "qabz_provider_demoted",
			Help: "1 while the provider row (provider, bill type, action) is demoted by the Provider Switcher (task 059), 0 after it returns.",
		}, []string{"provider", "bill_type", "action"})
		trials := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_provider_trial_total",
			Help: "Trial inquiries of a demoted provider row (task 059) by bill type, action of the demoted row (inquiry, order) and result (ok: the row returns; failed: demoted again; released: no verdict, the next request can try).",
		}, []string{"bill_type", "action", "result"})
		for _, c := range []promclient.Collector{loads, duration, lastOK, changes, active, switches, demoted, trials} {
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
		onSwitch = func(billType, action, result string) { switches.WithLabelValues(billType, action, result).Inc() }
		onTrial = func(billType, action, result string) { trials.WithLabelValues(billType, action, result).Inc() }
		onDemoted = func(p provider.Provider, on bool) {
			v := 0.0
			if on {
				v = 1
			}
			demoted.WithLabelValues(p.Name, p.BillType, p.Action).Set(v)
		}
	}
	return provider.NewProvidersService(repo, provider.Options{
		Interval:      func() time.Duration { return holder.Get().ProvidersLoadInterval },
		Changed:       holder.Changed,
		Logger:        logger,
		OnLoad:        onLoad,
		OnChange:      onChange,
		OnActiveCount: onActive,
		Available:     available,
		OnSwitch:      onSwitch,
		OnDemoted:     onDemoted,
		OnTrial:       onTrial,
	})
}

// providerAvailable is the Available callback of the Provider Switcher (task 059): an inquiry row
// needs an adapter that inquires its bill type (SupportsInquiry), an order row an adapter and a
// payment route of its bill type (routes, as the payment fallback of task 058). Any other action is
// not available.
func providerAvailable(adapters inquiry.Adapters, routes map[string]map[string]bool) func(provider.Provider) bool {
	return func(p provider.Provider) bool {
		a, ok := adapters.Get(p.Name)
		if !ok || a == nil {
			return false
		}
		switch p.Action {
		case provider.ActionInquiry:
			return a.SupportsInquiry(p.BillType)
		case order.ActionOrder:
			return routes[p.Name][p.BillType]
		}
		return false
	}
}

// newProviderRegistry builds the provider adapters: Qabzino (task 021; since task 030 the direct
// HTTP adapter with providers.external.qabzino.enabled, token, water_bill_endpoint, gas_bill_endpoint (task
// 023), rightel_bill_endpoint (task 037) and timeout, not the gRPC QabzinoProxy) and Tavanir (task 022; since task 034 the direct HTTP client of the Tavanir
// endpoints with providers.external.tavanir.enabled, auth_endpoint, basic_branch_endpoint, username, password
// and timeout; it calls no other service) and Irancell (task 035: a gRPC client of the irancell
// service with providers.internal.irancell.enabled, grpc_address and timeout; task 056: the payment
// over the NATS client of newNATS with nats_subject and pay_timeout), and Jiring Switch and Parsian
// (task 036, the MCI mobile bill: gRPC clients of the jiring-switch and the parsian-v2 service with
// providers.internal.jiring_switch.* and providers.internal.parsian.*), and Karashop (task 025, the landline bill: the
// direct HTTPS client of the Karashop API with providers.external.karashop.enabled, code, password,
// bill_inquiry_url and timeout). The mock of a provider
// (debug.mocks.<provider>) is applied only when Config.Mode is dev and its enable key is true (one
// warn line); then Inquiry answers a fixed answer and never calls the backend. Every provider that
// is added later follows the same pattern (registerAdapter) with its own debug.mocks.<provider>. A
// nil logger writes no line.
func newProviderRegistry(cfg config.Config, logger Logger) (*provider.Registry, error) {
	return newProviderRegistryWith(cfg, logger, nil)
}

// newProviderRegistryWith is newProviderRegistry with the NATS client of the Irancell payment (task
// 056; nil: every Irancell payment is pending, with no message sent).
func newProviderRegistryWith(cfg config.Config, logger Logger, payer irancellproxy.Requester) (*provider.Registry, error) {
	registry := provider.NewRegistry()
	dev := cfg.Mode == config.ModeDev

	qabzinoAdapter, err := qabzino.New(qabzino.Options{
		Enabled:         cfg.QabzinoEnabled,
		Token:           cfg.QabzinoToken.Reveal(),
		WaterEndpoint:   cfg.QabzinoWaterBillEndpoint,
		GasEndpoint:     cfg.QabzinoGasBillEndpoint,
		RightelEndpoint: cfg.QabzinoRightelBillEndpoint,
		Timeout:         cfg.QabzinoTimeout,
	})
	if err != nil {
		return nil, err
	}
	// providers.external.qabzino.enabled: false wins over the mock. The adapter then fails every call as
	// "provider down" and the mock must not answer instead of it (task 030).
	qabzinoMock := provider.MockSettings{Enable: dev && cfg.DebugMockQabzinoEnabled && cfg.QabzinoEnabled, ReturnType: cfg.DebugMockQabzinoReturnType}
	if err := registerAdapter(registry, qabzinoAdapter, "Qabzino", disabledNotice("external.qabzino", "Qabzino", cfg.QabzinoEnabled),
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
	// providers.external.tavanir.enabled: false wins over the mock, as for Qabzino (task 034).
	tavanirMock := provider.MockSettings{Enable: dev && cfg.DebugMockTavanirEnabled && cfg.TavanirEnabled, ReturnType: cfg.DebugMockTavanirReturnType}
	if err := registerAdapter(registry, tavanirAdapter, "Tavanir", disabledNotice("external.tavanir", "Tavanir", cfg.TavanirEnabled),
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
		Enabled:    cfg.IrancellEnabled,
		Client:     irancellClient,
		Timeout:    cfg.IrancellTimeout,
		NATS:       payer,                   // task 056: the payment over NATS
		Subject:    cfg.IrancellNATSSubject, // task 056
		PayTimeout: cfg.IrancellPayTimeout,  // task 056
	})
	if err != nil {
		return nil, err
	}
	// providers.internal.irancell.enabled: false wins over the mock, as for Qabzino and Tavanir.
	irancellMock := provider.MockSettings{Enable: dev && cfg.DebugMockIrancellEnabled && cfg.IrancellEnabled, ReturnType: cfg.DebugMockIrancellReturnType,
		StatusReturnType: cfg.DebugMockIrancellStatusReturnType, // task 050: the status of an order
		PayReturnType:    cfg.DebugMockIrancellPayReturnType}    // task 056: the payment
	if err := registerAdapter(registry, irancellAdapter, "Irancell", disabledNotice("internal.irancell", "Irancell", cfg.IrancellEnabled),
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
	// providers.internal.jiring_switch.enabled: false wins over the mock, as for the other providers.
	jiringMock := provider.MockSettings{Enable: dev && cfg.DebugMockJiringSwitchEnabled && cfg.JiringSwitchEnabled, ReturnType: cfg.DebugMockJiringSwitchReturnType,
		PayReturnType: cfg.DebugMockJiringSwitchPayReturnType, StatusReturnType: cfg.DebugMockJiringSwitchStatusReturnType} // task 058: the payment and the status
	if err := registerAdapter(registry, jiringAdapter, "Jiring Switch", disabledNotice("internal.jiring_switch", "Jiring Switch", cfg.JiringSwitchEnabled),
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
	// providers.internal.parsian.enabled: false wins over the mock, as for the other providers.
	parsianMock := provider.MockSettings{Enable: dev && cfg.DebugMockParsianEnabled && cfg.ParsianEnabled, ReturnType: cfg.DebugMockParsianReturnType,
		PayReturnType: cfg.DebugMockParsianPayReturnType, StatusReturnType: cfg.DebugMockParsianStatusReturnType}
	if err := registerAdapter(registry, parsianAdapter, "Parsian", disabledNotice("internal.parsian", "Parsian", cfg.ParsianEnabled),
		parsianMock, parsianproxy.MockFixtures(), logger); err != nil {
		return nil, err
	}

	// Karashop (task 025): a direct HTTPS client of the Karashop API, as Qabzino and Tavanir.
	karashopAdapter, err := karashop.New(karashop.Options{
		Enabled:        cfg.KarashopEnabled,
		Code:           cfg.KarashopCode,
		Password:       cfg.KarashopPassword.Reveal(),
		InquiryURL:     cfg.KarashopBillInquiryURL,
		PaymentInfoURL: cfg.KarashopBillPaymentInfoURL, // task 044: the settlement
		Timeout:        cfg.KarashopTimeout,
	})
	if err != nil {
		return nil, err
	}
	// providers.external.karashop.enabled: false wins over the mock, as for the other providers.
	karashopMock := provider.MockSettings{Enable: dev && cfg.DebugMockKarashopEnabled && cfg.KarashopEnabled, ReturnType: cfg.DebugMockKarashopReturnType,
		SettleReturnType: cfg.DebugMockKarashopSettleReturnType}
	if err := registerAdapter(registry, karashopAdapter, "Karashop", disabledNotice("external.karashop", "Karashop", cfg.KarashopEnabled),
		karashopMock, karashop.MockFixtures(), logger); err != nil {
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
			logger.Warn("provider mock enabled (debug.mocks."+name+"): development only, no call to the "+title+" backend", "return_type", mock.ReturnType,
				"pay_return_type", mock.PayReturnType, "settle_return_type", mock.SettleReturnType, "status_return_type", mock.StatusReturnType)
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

// disabledNotice is the warn text of a provider with providers.<key>.enabled false ("" when
// enabled). key is the group and the provider key (for example "external.qabzino", task 041);
// title is the name of the provider in the text (for example "Qabzino").
func disabledNotice(key, title string, enabled bool) string {
	if enabled {
		return ""
	}
	return "providers." + key + ".enabled is false: every inquiry to " + title + " fails as provider down; the " + title + " mock does not apply"
}

// newInquiryFlow builds the inquiry flow (inquiry.Service) that every inquiry endpoint uses (water:
// task 021; electricity: task 022; gas: task 023; the Irancell mobile bill: task 035, caching.irancell; the MCI mobile bill: task 036, caching.mobile; the landline bill: task 025, caching.tel). It picks the provider with ProvidersService, answers from the
// saved inquiries of bill_inquiries when they are younger than the cache TTL of the bill type
// (caching.water, caching.electricity, caching.gas, ..., caching.tel; read at start), else calls the adapter of the provider and
// saves the answer. With metrics on, each inquiry adds 1 to
// qabz_inquiries_total{bill_type,provider,result} (result: db_hit, provider_ok, provider_refused,
// provider_down, no_provider, canceled, error, raw; provider "none" when no provider was picked); each
// provider call adds 1 to qabz_provider_calls_total{provider,action,result} (ok, refused, down,
// canceled, error) and observes qabz_provider_call_duration_ms{provider,action}. Rejected calls are
// in the code label of qabz_request_duration_ms. A failed lookup or copy or save of inquiries
// writes one error log line. With debug.cache_inquiries false (task 031, development only) the flow
// does not read or write the inquiry tables (inquiry.Options.NoStore): every inquiry calls the
// provider; one warn line at start names the key.
func newInquiryFlow(cfg config.Config, db *postgres.Client, providers inquiry.Selector, adapters inquiry.Adapters, metrics *prometheus.Server, logger Logger) (*inquiry.Service, error) {
	var onInquiry func(billType, providerName, result string)             // nil: metrics are off, nothing is recorded
	var onCall func(providerName, action, result string, d time.Duration) // nil: metrics are off, nothing is recorded
	if reg := metrics.Registry(); reg != nil {
		inquiries := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_inquiries_total",
			Help: "Bill inquiries by bill type (water, electricity, gas, irancell, mobile, mobilemidtermRightel, tel), provider (none when no provider was picked) and result (db_hit: answered from saved inquiries; provider_ok; provider_refused: the provider has no inquiry; provider_down: the call failed; no_provider; canceled: the client left during the provider call; error; raw: a gas or tel request with billId and payId, answered from the two IDs with no provider).",
		}, []string{"bill_type", "provider", "result"})
		calls := promclient.NewCounterVec(providerCallsOpts, providerCallLabels)
		duration := promclient.NewHistogramVec(providerCallDurationOpts, providerCallDurationLabels)
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
		// Rightel (task 037): the key of MCI, as bill (operator decision).
		provider.BillTypeMobileMidTermRightel: cfg.CachingMobile,
		provider.BillTypeTel:                  cfg.CachingTel, // task 025
	}
	if !cfg.DebugCacheInquiries && logger != nil {
		logger.Warn("inquiry store disabled (debug.cache_inquiries: false): development only, every inquiry calls the provider and nothing is saved")
	}
	onDown := demoteOnDown(cfg.DebugProviderSwitcherDemoteOnDown, providers, logger)
	return inquiry.NewService(inquiry.Options{
		NoStore:        !cfg.DebugCacheInquiries,
		Repository:     postgres.NewInquiryRepository(db),
		Providers:      providers,
		Adapters:       adapters,
		CacheTTL:       func(billType string) time.Duration { return ttl[billType] }, // read-only map; no entry: 0, no cache
		Logger:         flowLog,
		OnInquiry:      onInquiry,
		OnProviderCall: onCall,
		OnProviderDown: onDown,
	})
}

// demoter is the Demote method of the Provider Switcher (provider.ProvidersService).
type demoter interface {
	Demote(id int64, period time.Duration) error
}

// demoteOnDown is the feature-ON hook of task 059 (debug.provider_switcher.demote_on_down, dev
// only): with a period greater than 0 and a selector that can demote, each provider-down inquiry
// that is not a trial demotes its row for the period (one warn line at start names the key). The
// Provider Switcher writes the log line and counts the result (no_next too), so an error of Demote
// is not logged again. 0, or a selector with no Demote: nil (no hook). Task 060 replaces it.
func demoteOnDown(period time.Duration, providers inquiry.Selector, logger Logger) func(provider.Provider) {
	d, ok := providers.(demoter)
	if period <= 0 || !ok {
		return nil
	}
	if logger != nil {
		logger.Warn("provider switcher: debug.provider_switcher.demote_on_down is on: development only, each provider-down inquiry demotes its provider row",
			"period_ms", float64(period)/float64(time.Millisecond))
	}
	return func(p provider.Provider) { _ = d.Demote(p.ID, period) }
}

// The provider call metrics (task 045: the status worker uses them too, with the action status).
// newInquiryFlow registers them; newCronMetrics takes the registered ones.
var (
	providerCallsOpts = promclient.CounterOpts{
		Name: "qabz_provider_calls_total",
		Help: "Calls to a provider by provider, action (inquiry; status: the status worker, task 045) and result (ok, refused: the provider has no inquiry, down: the call failed, canceled: the client left or the App stopped, error).",
	}
	providerCallLabels       = []string{"provider", "action", "result"}
	providerCallDurationOpts = promclient.HistogramOpts{
		Name:    "qabz_provider_call_duration_ms",
		Help:    "Duration of each call to a provider in milliseconds, by provider and action (inquiry, status), for every result.",
		Buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000},
	}
	providerCallDurationLabels = []string{"provider", "action"}
)

// newWater builds the gRPC Water service: POST /v1/water/inquiry (task 021) on the inquiry flow and
// POST /v1/water/order (task 048) on the order flow, with the token signed by signer. A server fault
// writes one error log line (grpcserver.Water). A nil flow gives a service whose Inquiry answers
// Unimplemented; a nil orders or signer gives a service whose NewOrder answers Unimplemented.
func newWater(flow *inquiry.Service, orders *order.Service, signer *crypto.Signer, serviceID uint32, logger Logger) *grpcserver.Water {
	deps := grpcserver.WaterDeps{ServiceID: serviceID}
	if flow != nil {
		deps.Inquiry = flow
	}
	if orders != nil {
		deps.Orders = orders
	}
	if signer != nil {
		deps.Signer = signer
	}
	if logger != nil {
		deps.Logger = logger
	}
	return grpcserver.NewWater(deps)
}

// newElectricity builds the gRPC Electricity service: POST /v1/electricity/inquiry (task 022) on
// the same inquiry flow as Water and POST /v1/electricity/order (task 052) on the order flow, with
// the token signed by signer. A server fault writes one error log line (grpcserver.Electricity). A
// nil flow gives a service whose Inquiry answers Unimplemented; a nil orders or signer gives a
// service whose NewOrder answers Unimplemented.
func newElectricity(flow *inquiry.Service, orders *order.Service, signer *crypto.Signer, serviceID uint32, logger Logger) *grpcserver.Electricity {
	deps := grpcserver.ElectricityDeps{ServiceID: serviceID}
	if flow != nil {
		deps.Inquiry = flow
	}
	if orders != nil {
		deps.Orders = orders
	}
	if signer != nil {
		deps.Signer = signer
	}
	if logger != nil {
		deps.Logger = logger
	}
	return grpcserver.NewElectricity(deps)
}

// newGas builds the gRPC Gas service: POST /v1/gas/inquiry (task 023) on the same inquiry flow as
// Water (the flow also answers the raw inquiry, a request with billId and payId) and POST
// /v1/gas/order (task 053) on the order flow, with the token signed by signer. A server fault
// writes one error log line (grpcserver.Gas). A nil flow gives a service whose Inquiry answers
// Unimplemented; a nil orders or signer gives a service whose NewOrder answers Unimplemented.
func newGas(flow *inquiry.Service, orders *order.Service, signer *crypto.Signer, serviceID uint32, logger Logger) *grpcserver.Gas {
	deps := grpcserver.GasDeps{ServiceID: serviceID}
	if flow != nil {
		deps.Inquiry = flow
		deps.Raw = flow
	}
	if orders != nil {
		deps.Orders = orders
	}
	if signer != nil {
		deps.Signer = signer
	}
	if logger != nil {
		deps.Logger = logger
	}
	return grpcserver.NewGas(deps)
}

// newPhone builds the gRPC Phone service: POST /v1/mobile/inquiry (task 035: the Irancell operator; task 036: the MCI operator)
// on the same inquiry flow as Water, and POST /v1/mobile/order (task 039: the Irancell operator) on
// the order flow, with the order token signed by signer and serviceId = service.id. A server fault
// writes one error log line (grpcserver.Phone). A nil flow gives a service whose Inquiry answers
// Unimplemented; a nil orders or signer gives a service whose NewOrder answers Unimplemented.
func newPhone(flow *inquiry.Service, orders *order.Service, signer *crypto.Signer, serviceID uint32, logger Logger) *grpcserver.Phone {
	deps := grpcserver.PhoneDeps{ServiceID: serviceID}
	if flow != nil {
		deps.Inquiry = flow
	}
	if orders != nil {
		deps.Orders = orders
	}
	if signer != nil {
		deps.Signer = signer
	}
	if logger != nil {
		deps.Logger = logger
	}
	return grpcserver.NewPhone(deps)
}

// newOrders builds the order flow (order.Service, task 039) that every order endpoint uses: the
// order provider from ProvidersService, the price (the direct price, the raw inquiry or the inquiry
// flow), the fee (fee.Resolver on the fees table, read on each order) and the orders row. With
// metrics on, each order adds 1 to qabz_order_create_total{bill_type,provider,result} (result:
// created, invalid, no_provider, zero_price, inquiry_failed, error; provider "none" when no provider
// was picked) and each fee adds 1 to qabz_fee_resolve_total{source} (user_provider, user, provider,
// default, lookup_error, calc_error). A failed fee writes one error log line and the order goes on
// (bill gives the default fee for a failed read and 0 for a failed calculation); a failed read of
// the users with a fee writes one error line too.
//
// Task 044: the landline bill types tel and telmidterm. Their saved inquiries give the price for
// caching.paid_tel; the "bill already paid" check of the order provider parsian uses
// providers.internal.parsian.settlement_inquiry_telephone; result already_paid.
//
// Task 054: the MCI bill types mobile and mobilemidterm. The saved inquiries of mobile give the
// price for caching.paid_mobile (mobilemidterm has none, as bill); the "bill already paid" check of
// parsian uses providers.internal.parsian.settlement_inquiry_mci_mobile for both (0: any time).
//
// Task 055: the Rightel bill type mobilemidtermRightel. Its saved inquiries give the price for
// caching.paid_mobile (as bill); the "bill already paid" check of parsian uses
// providers.internal.parsian.settlement_inquiry_rightel_mobile (default 360h).
func newOrders(cfg config.Config, db *postgres.Client, providers order.Selector, flow *inquiry.Service, metrics *prometheus.Server, logger Logger) (*order.Service, error) {
	if flow == nil {
		return nil, errors.New("order: inquiry flow is required")
	}
	var onOrder func(billType, providerName, result string) // nil: metrics are off, nothing is recorded
	var onResolve func(source string)                       // nil: metrics are off, nothing is recorded
	if reg := metrics.Registry(); reg != nil {
		orders := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_order_create_total",
			Help: "New orders (POST /v1/mobile/order, POST /v1/tel/order, POST /v1/water/order, POST /v1/electricity/order, POST /v1/gas/order) by bill type (irancell, mobile, mobilemidterm, mobilemidtermRightel, tel, telmidterm, water, electricity, gas), order provider (none when no provider was picked) and result (created: the orders row is saved; invalid: the total does not fit the price field; no_provider; zero_price; inquiry_failed: the inquiry for the price failed or has no inquiry of the bill type; already_paid: the bill ID and pay ID have a pending or successful order; error).",
		}, []string{"bill_type", "provider", "result"})
		fees := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_fee_resolve_total",
			Help: "Fees of new orders by source (user_provider, user, provider: the fees row that was used; default: no row, the default fee; lookup_error: the read failed, the default fee; calc_error: the row gives no fee, 0).",
		}, []string{"source"})
		for _, c := range []promclient.Collector{orders, fees} {
			if err := reg.Register(c); err != nil {
				return nil, fmt.Errorf("order: register metric: %w", err)
			}
		}
		onOrder = func(billType, providerName, result string) {
			orders.WithLabelValues(billType, providerName, result).Inc()
		}
		onResolve = func(source string) { fees.WithLabelValues(source).Inc() }
	}
	var feeLog fee.Logger     // nil: no line
	var orderLog order.Logger // nil: no line
	if logger != nil {
		feeLog, orderLog = logger, logger
	}
	resolver, err := fee.NewResolver(fee.Options{Repository: postgres.NewFeeRepository(db), Logger: feeLog, OnResolve: onResolve})
	if err != nil {
		return nil, err
	}
	orderTTL := map[string]time.Duration{
		provider.BillTypeTel: cfg.CachingPaidTel, provider.BillTypeTelMidterm: cfg.CachingPaidTel,
		provider.BillTypeWater:       cfg.CachingPaidWater,       // task 048 (bill: caching.paidWater)
		provider.BillTypeElectricity: cfg.CachingPaidElectricity, // task 052 (bill: caching.paidElectricity)
		provider.BillTypeGas:         cfg.CachingPaidGas,         // task 053 (bill: caching.paidGas)
		provider.BillTypeMobile:      cfg.CachingPaidMobile,      // task 054 (bill: caching.paidMobile; mobilemidterm has none)

		provider.BillTypeMobileMidTermRightel: cfg.CachingPaidMobile, // task 055 (bill: caching.paidMobile)
	}
	mciWindow := cfg.ParsianSettlementInquiryMCIMobile // task 054 (bill: parsian.settlementInquiryMCIMobile)
	telWindow := cfg.ParsianSettlementInquiryTelephone
	return order.NewService(order.Options{
		Providers:     providers,
		Inquiry:       flow,
		Raw:           flow,
		Fees:          resolver,
		Repository:    postgres.NewOrderRepository(db),
		Logger:        orderLog,
		OnOrder:       onOrder,
		OrderCacheTTL: func(billType string) time.Duration { return orderTTL[billType] }, // read-only map; no entry: no saved inquiry
		AlreadyPaidWindows: map[string]map[string]time.Duration{
			parsianproxy.Name: {
				provider.BillTypeTel: telWindow, provider.BillTypeTelMidterm: telWindow,
				provider.BillTypeWater:       cfg.ParsianSettlementInquiryWater,       // task 048 (bill: parsian.settlementInquiryWater)
				provider.BillTypeElectricity: cfg.ParsianSettlementInquiryElectricity, // task 052 (bill: parsian.settlementInquiryElectricity)
				provider.BillTypeGas:         cfg.ParsianSettlementInquiryGas,         // task 053 (bill: parsian.settlementInquiryGas)
				provider.BillTypeMobile:      mciWindow, provider.BillTypeMobileMidterm: mciWindow,

				provider.BillTypeMobileMidTermRightel: cfg.ParsianSettlementInquiryRightelMobile, // task 055 (bill: parsian.settlementInquiryRightelMobile)
			},
			// Task 058: bill's fixed 0 of Jiring Switch for both MCI terms (no config key in bill).
			jiring.Name: {provider.BillTypeMobile: 0, provider.BillTypeMobileMidterm: 0},
		},
	})
}

// newPayments builds the payment step of the orders (order.Payments, task 044): the payment
// service calls OrderComplete on the internal gRPC server. The order provider parsian pays the
// bill types tel, telmidterm, water, electricity, gas, mobile, mobilemidterm and mobilemidtermRightel
// (parsianproxy.Pay; water since task 048, electricity since task 052, gas since task 053, the MCI
// bill types since task 054, Rightel since task 055), and the order provider irancell pays the bill
// type irancell (irancellproxy.Pay over NATS, task 056), and the order provider jiring-switch pays the
// bill types mobile and mobilemidterm (jiring.Pay, task 058). A payment that did not reach its
// provider for sure (provider.ErrNotSent) goes to the next enabled order row of groups with a
// payment route (task 058; groups nil: no next provider); after a success of tel or telmidterm the Karashop
// settlement runs in the background (karashop.Settle) for the order providers parsian, eways and
// eways2, as bill (water, electricity, gas, MCI, Rightel and Irancell have no settlement). With
// metrics on:
//   - qabz_order_payment_total{bill_type,provider,status}: each OrderComplete call (success, failed,
//     pending, processed, wrong_price, error; provider "none" when the order was not read);
//   - qabz_provider_pay_duration_ms{provider,status}: each payment call to a provider;
//   - qabz_order_payment_fallback_total{bill_type,from,to}: each move of a payment to the next
//     provider (task 058);
//   - qabz_order_settle_total{bill_type,provider,result,source}: each settlement (success, failed,
//     pending, error, confirm_error, no_data), source payment (the settlement worker of task 046
//     counts on the same metric with source worker).
func newPayments(db *postgres.Client, adapters order.Adapters, groups order.Groups, metrics *prometheus.Server, logger Logger) (*order.Payments, error) {
	var onPayment, onSettle, onFallback func(billType, providerName, result string)
	var onPay func(providerName, status string, d time.Duration)
	if reg := metrics.Registry(); reg != nil {
		payments := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_order_payment_total",
			Help: "OrderComplete calls of the payment service by bill type, order provider (none when the order was not read) and result (success, failed, pending: the status of the payment; processed: the order is not initial, answered pending, or failed for a failed order; wrong_price; error: a failed read or write, or the provider does not pay the bill type).",
		}, []string{"bill_type", "provider", "status"})
		duration := promclient.NewHistogramVec(promclient.HistogramOpts{
			Name:    "qabz_provider_pay_duration_ms",
			Help:    "Duration of each payment call to a provider in milliseconds, by provider and the status of the payment (success, failed, pending).",
			Buckets: []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000},
		}, []string{"provider", "status"})
		fallbacks := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_order_payment_fallback_total",
			Help: "OrderComplete payments that went to the next provider (task 058) because the payment did not reach the provider for sure (the switch off, no address, gRPC Unavailable before the request was sent), by bill type, from (the provider that was not reached) and to (the next enabled order provider with a payment route).",
		}, []string{"bill_type", "from", "to"})
		for _, c := range []promclient.Collector{payments, duration, fallbacks} {
			if err := reg.Register(c); err != nil {
				return nil, fmt.Errorf("payment: register metric: %w", err)
			}
		}
		onFallback = func(billType, from, to string) {
			fallbacks.WithLabelValues(billType, from, to).Inc()
		}
		settles, err := registerOrExisting[*promclient.CounterVec](reg, promclient.NewCounterVec(settleOpts, settleLabels))
		if err != nil {
			return nil, err
		}
		onPayment = func(billType, providerName, result string) {
			payments.WithLabelValues(billType, providerName, result).Inc()
		}
		onPay = func(providerName, status string, d time.Duration) {
			duration.WithLabelValues(providerName, status).Observe(float64(d) / float64(time.Millisecond))
		}
		onSettle = func(billType, providerName, result string) {
			settles.WithLabelValues(billType, providerName, result, order.SettleSourcePayment).Inc()
		}
	}
	var payLog order.PaymentLogger // nil: no line
	if logger != nil {
		payLog = logger
	}
	return order.NewPayments(order.PaymentOptions{
		Repository:   postgres.NewOrderRepository(db),
		Adapters:     adapters,
		PayRoutes:    payRoutes(),
		Providers:    groups,
		OnFallback:   onFallback,
		SettleRoutes: settleRoutes(),
		Logger:       payLog,
		OnPayment:    onPayment,
		OnPay:        onPay,
		OnSettle:     onSettle,
	})
}

// payRoutes are the bill types that each order provider pays (order.PaymentOptions.PayRoutes; the
// Provider Switcher reads them too: providerAvailable, task 059). A new map for each call.
func payRoutes() map[string]map[string]bool {
	return map[string]map[string]bool{
		parsianproxy.Name: {provider.BillTypeTel: true, provider.BillTypeTelMidterm: true, provider.BillTypeWater: true, provider.BillTypeElectricity: true, provider.BillTypeGas: true,
			provider.BillTypeMobile: true, provider.BillTypeMobileMidterm: true, // task 054; task 058: also jiring-switch
			provider.BillTypeMobileMidTermRightel: true}, // task 055: Parsian only (operator); bill also has EWaysRightelMobile (NATS)
		irancellproxy.Name: {provider.BillTypeIrancell: true},                                     // task 056: the irancell service over NATS (bill: IrancellMobile)
		jiring.Name:        {provider.BillTypeMobile: true, provider.BillTypeMobileMidterm: true}, // task 058 (bill: JiringSwitchMCIMobile)
	}
}

// settleOpts and settleLabels describe qabz_order_settle_total, which the payment step and the
// settlement worker share (registerOrExisting).
var (
	settleOpts = promclient.CounterOpts{
		Name: "qabz_order_settle_total",
		Help: "Settlements of paid orders by bill type, settlement provider (karashop; empty for no_route), result (success: confirmed and saved; failed, pending: the answer of the provider; error: the call failed; confirm_error: provider_confirmed was not saved; no_data: the order has no number; no_route: no settlement provider) and source (payment: at once after OrderComplete; worker: the settlement worker).",
	}
	settleLabels = []string{"bill_type", "provider", "result", "source"}
)

// settleRoutes give the settlement provider by bill type and order provider (task 044; bill: the
// confirm payment gateway map): tel and telmidterm, the order providers parsian, eways and eways2,
// Karashop. A new map each call (read-only for the users).
func settleRoutes() map[string]map[string]string {
	settle := func() map[string]string {
		return map[string]string{parsianproxy.Name: karashop.Name, "eways": karashop.Name, "eways2": karashop.Name}
	}
	return map[string]map[string]string{provider.BillTypeTel: settle(), provider.BillTypeTelMidterm: settle()}
}

// newTel builds the gRPC Tel service: POST /v1/tel/inquiry (task 025, the landline bill) on the same
// inquiry flow as Water; the flow also answers the raw inquiry (a request with billId and payId).
// POST /v1/tel/order (task 044) on the order flow, with the token signed by signer. A server fault
// writes one error log line (grpcserver.Tel). A nil flow gives a service whose Inquiry answers
// Unimplemented; a nil orders or signer gives a service whose NewOrder answers Unimplemented.
func newTel(flow *inquiry.Service, orders *order.Service, signer *crypto.Signer, serviceID uint32, logger Logger) *grpcserver.Tel {
	deps := grpcserver.TelDeps{ServiceID: serviceID}
	if flow != nil {
		deps.Inquiry = flow
		deps.Raw = flow
	}
	if orders != nil {
		deps.Orders = orders
	}
	if signer != nil {
		deps.Signer = signer
	}
	if logger != nil {
		deps.Logger = logger
	}
	return grpcserver.NewTel(deps)
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
