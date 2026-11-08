package app

import (
	"context"
	"errors"
	"fmt"

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

// newMyBills builds the gRPC MyBills service: POST /v1/mybill saves a bill in the bills table and
// GET /v1/mybill lists the user's bills (serviceId is config service.id). Each saved bill adds 1
// to qabz_mybill_insert_total{result="created"|"existing"} (only when metrics are on); a failed
// write or read writes one error log line. Rejected and failed calls are in the code label of
// qabz_request_duration_ms.
func newMyBills(cfg config.Config, db *postgres.Client, metrics *prometheus.Server, logger Logger) (*grpcserver.MyBills, error) {
	serviceID, err := grpcserver.ServiceIDFromConfig(cfg.ServiceID)
	if err != nil {
		return nil, err
	}
	var onInsert func(result string) // nil: metrics are off, nothing is recorded
	if reg := metrics.Registry(); reg != nil {
		inserts := promclient.NewCounterVec(promclient.CounterOpts{
			Name: "qabz_mybill_insert_total",
			Help: "POST /v1/mybill saved bills by result (created, existing).",
		}, []string{"result"})
		if err := reg.Register(inserts); err != nil {
			return nil, fmt.Errorf("mybill: register metric: %w", err)
		}
		onInsert = func(result string) { inserts.WithLabelValues(result).Inc() }
	}
	store, err := savedbill.NewStore(postgres.NewSavedBillRepository(db))
	if err != nil {
		return nil, err
	}
	return grpcserver.NewMyBills(grpcserver.MyBillsDeps{
		Bills:     store,
		Lister:    store,
		ServiceID: serviceID,
		Logger:    logger,
		OnInsert:  onInsert,
	}), nil
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
