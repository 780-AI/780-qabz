package app

import (
	"context"
	"errors"
	"fmt"

	promclient "github.com/prometheus/client_golang/prometheus"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/logging/zap"
	"qabz/common/monitoring/jaeger"
	"qabz/common/monitoring/prometheus"
	"qabz/config"
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

	// The auth check comes first: a bad key or audience stops the start before any connection.
	metrics := prometheus.New(cfg.MetricsAddr)
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

	gs, err := grpcserver.NewWithOptions(cfg.GRPCAddr, authCheck.ServerOptions(), grpcserver.RegisterService(grpcserver.NewService()))
	if err == nil && gs == nil {
		err = errors.New("grpc server: New returned nil")
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	gw, err := httpserver.New(cfg.HTTPAddr, gs.DialAddr, pb.RegisterServiceHandler)
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
// debug.auth.disable it logs one warning and checks nothing; otherwise it needs
// auth.jwt.public_key_path and auth.jwt.audience. Each rejected call adds 1 to
// qabz_auth_rejected_total{reason}; a rejected call writes no log line.
func newAuth(cfg config.Config, metrics *prometheus.Server, logger Logger) (*grpcserver.Auth, error) {
	if cfg.DebugAuthDisable {
		logger.Warn("auth check disabled (debug.auth.disable): development only")
		return grpcserver.NewAuth(grpcserver.AuthOptions{Disabled: true})
	}
	verifier, err := auth.LoadVerifier(cfg.JWTPublicKeyPath, cfg.JWTAudience)
	if err != nil {
		return nil, fmt.Errorf("%w (set auth.jwt.public_key_path and auth.jwt.audience, or debug.auth.disable in development)", err)
	}
	rejected := promclient.NewCounterVec(promclient.CounterOpts{
		Name: "qabz_auth_rejected_total",
		Help: "Calls rejected by the JWT auth check, by reason.",
	}, []string{"reason"})
	if metrics != nil && metrics.Registry() != nil {
		if err := metrics.Registry().Register(rejected); err != nil {
			return nil, fmt.Errorf("auth: register metric: %w", err)
		}
	}
	return grpcserver.NewAuth(grpcserver.AuthOptions{
		Verifier: verifier,
		OnReject: func(reason string) { rejected.WithLabelValues(reason).Inc() },
	})
}
