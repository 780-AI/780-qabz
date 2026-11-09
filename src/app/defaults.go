package app

import (
	"context"
	"errors"
	"fmt"
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
	"qabz/core/savedbill"
	grpcserver "qabz/entry/grpc"
	httpserver "qabz/entry/http"
	"qabz/migrations"
	"qabz/provider"
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
	// The request metrics come first in the chain, so a call that the auth check rejects is counted too.
	opts, err := newRequestMetrics(metrics)
	if err != nil {
		db.Close()
		return nil, err
	}
	opts = append(opts, authCheck.ServerOptions()...)
	gs, err := grpcserver.NewWithOptions(cfg.GRPCAddr, opts, grpcserver.RegisterService(svc), grpcserver.RegisterMyBills(myBills))
	if err == nil && gs == nil {
		err = errors.New("grpc server: New returned nil")
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	gw, err := httpserver.New(cfg.HTTPAddr, gs.DialAddr, pb.RegisterServiceHandler, pb.RegisterMyBillsHandler)
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
// read from holder before each wait (hot-reloadable). With metrics on, each load adds 1 to
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
		Logger:        logger,
		OnLoad:        onLoad,
		OnChange:      onChange,
		OnActiveCount: onActive,
	})
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
