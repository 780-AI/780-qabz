package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"qabz/config"
)

const (
	pingTimeout = 5 * time.Second
	stopTimeout = 10 * time.Second
)

// Logger is the key-value logger the App needs.
type Logger interface {
	Debug(msg string, kv ...any)
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
	Sync() error
}

// Connection is a long-lived connection to a server.
type Connection interface {
	Ping(ctx context.Context) error
	Close()
}

// Database is the SQL store.
type Database interface{ Connection }

// Cache is the key-value store.
type Cache interface{ Connection }

// Metrics serves the metrics endpoint.
type Metrics interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Server is a network server that the App starts and stops (the gRPC server, the HTTP gateway).
type Server interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Migrator applies the SQL migrations. Many containers may call Up at the same time.
type Migrator interface {
	Up(ctx context.Context) error
}

// ConfigReloader reloads the hot-reloadable config entries at an interval.
type ConfigReloader interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Tracer exports traces.
type Tracer interface {
	Shutdown(ctx context.Context) error
}

// App owns every long-lived dependency. It starts and stops them in a fixed order.
// Make one with NewBuilder.
type App struct {
	cfg      *config.Holder[config.Config]
	logger   Logger
	db       Database
	cache    Cache
	metrics  Metrics
	tracer   Tracer         // nil when tracing is off
	migrator Migrator       // nil: no migration at start
	reloader ConfigReloader // nil: no hot-reload
	grpc     Server         // nil: no gRPC server
	http     Server         // nil: no HTTP gateway

	mu      sync.Mutex
	started bool
}

// Config returns a copy of the current settings. Hot-reloadable entries can change between calls, so do not keep the copy.
func (a *App) Config() config.Config { return a.cfg.Get() }

// Logger returns the shared logger.
func (a *App) Logger() Logger { return a.logger }

// Database returns the SQL store.
func (a *App) Database() Database { return a.db }

// Cache returns the key-value store.
func (a *App) Cache() Cache { return a.cache }

// Start pings the database and the cache, runs the migrations (when enabled), starts the config
// reloader, the gRPC server, the HTTP gateway and the metrics endpoint. If a step fails, Start stops
// what it opened and returns the error.
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return errors.New("app: already started")
	}
	cfg := a.cfg.Get()
	if err := a.ping(ctx, "postgres", cfg.PostgresTarget(), a.db); err != nil {
		return a.abort(err, running{})
	}
	if err := a.ping(ctx, "redis", cfg.RedisAddr(), a.cache); err != nil {
		return a.abort(err, running{})
	}
	if a.migrator != nil && cfg.MigrateOnStart {
		mctx, cancel := context.WithTimeout(ctx, cfg.MigrateTimeout)
		err := a.migrator.Up(mctx)
		cancel()
		if err != nil {
			a.logger.Error("migration failed", "error", err)
			return a.abort(err, running{})
		}
		a.logger.Info("migrations ok")
	}
	var run running
	if a.reloader != nil {
		if err := a.reloader.Start(ctx); err != nil {
			a.logger.Error("config reloader start failed", "error", err)
			return a.abort(fmt.Errorf("config reloader: %w", err), run)
		}
		run.reloader = true
		a.logger.Info("config reloader started", "interval", cfg.HotReloadInterval.String())
	}
	if a.grpc != nil {
		if err := a.grpc.Start(ctx); err != nil {
			a.logger.Error("grpc start failed", "addr", cfg.GRPCAddr, "error", err)
			return a.abort(fmt.Errorf("grpc: %w", err), run)
		}
		run.grpc = true
		a.logger.Info("grpc started", "addr", cfg.GRPCAddr)
	}
	if a.http != nil {
		if err := a.http.Start(ctx); err != nil {
			a.logger.Error("http start failed", "addr", cfg.HTTPAddr, "error", err)
			return a.abort(fmt.Errorf("http: %w", err), run)
		}
		run.http = true
		a.logger.Info("http started", "addr", cfg.HTTPAddr)
	}
	if err := a.metrics.Start(ctx); err != nil {
		a.logger.Error("metrics start failed", "addr", cfg.MetricsAddr, "error", err)
		return a.abort(fmt.Errorf("metrics: %w", err), run)
	}
	if m, ok := a.metrics.(interface{ Enabled() bool }); ok && !m.Enabled() {
		a.logger.Info("metrics disabled (metrics.prometheus.enabled: false)")
	} else {
		a.logger.Info("metrics started", "addr", cfg.MetricsAddr)
	}
	a.started = true
	a.logger.Info("app started")
	return nil
}

// Stop closes everything in reverse order and returns all errors together. It is safe to call more than once.
func (a *App) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.started {
		return nil
	}
	a.started = false
	a.logger.Info("app stopping")
	return a.release(ctx, running{reloader: true, grpc: true, http: true, metrics: true})
}

// Run starts the App, waits until ctx is done, then stops it.
func (a *App) Run(ctx context.Context) error {
	if err := a.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
	defer cancel()
	return a.Stop(stopCtx)
}

func (a *App) ping(ctx context.Context, name, target string, c Connection) error {
	pctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if err := c.Ping(pctx); err != nil {
		a.logger.Error(name+" ping failed", "target", target, "error", err)
		return fmt.Errorf("%s: %w", name, err)
	}
	a.logger.Info(name+" ok", "target", target)
	return nil
}

// running records which parts Start has started, so a failed Start stops only those parts.
type running struct{ reloader, grpc, http, metrics bool }

// abort releases the dependencies after a failed Start. run holds the parts that started.
func (a *App) abort(cause error, run running) error {
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	return errors.Join(cause, a.release(ctx, run))
}

// release stops the parts in run in reverse start order, then closes the tracer, the cache, the
// database and the logger. It never stops a part that did not start.
func (a *App) release(ctx context.Context, run running) error {
	var errs []error
	if run.metrics {
		errs = append(errs, a.metrics.Stop(ctx))
	}
	if a.http != nil && run.http {
		errs = append(errs, a.http.Stop(ctx))
	}
	if a.grpc != nil && run.grpc {
		errs = append(errs, a.grpc.Stop(ctx))
	}
	if a.reloader != nil && run.reloader {
		errs = append(errs, a.reloader.Stop(ctx))
	}
	if a.tracer != nil {
		errs = append(errs, a.tracer.Shutdown(ctx))
	}
	a.cache.Close()
	a.db.Close()
	_ = a.logger.Sync() // stdout/stderr sync can fail on some systems; not an app error
	return errors.Join(errs...)
}
