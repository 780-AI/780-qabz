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
// reloader and the metrics endpoint. If a step fails, Start stops what it opened and returns the error.
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return errors.New("app: already started")
	}
	cfg := a.cfg.Get()
	if err := a.ping(ctx, "postgres", cfg.PostgresTarget(), a.db); err != nil {
		return a.abort(err)
	}
	if err := a.ping(ctx, "redis", cfg.RedisAddr(), a.cache); err != nil {
		return a.abort(err)
	}
	if a.migrator != nil && cfg.MigrateOnStart {
		mctx, cancel := context.WithTimeout(ctx, cfg.MigrateTimeout)
		err := a.migrator.Up(mctx)
		cancel()
		if err != nil {
			a.logger.Error("migration failed", "error", err)
			return a.abort(err)
		}
		a.logger.Info("migrations ok")
	}
	if a.reloader != nil {
		if err := a.reloader.Start(ctx); err != nil {
			a.logger.Error("config reloader start failed", "error", err)
			return a.abort(fmt.Errorf("config reloader: %w", err))
		}
		a.logger.Info("config reloader started", "interval", cfg.HotReloadInterval.String())
	}
	if err := a.metrics.Start(ctx); err != nil {
		a.logger.Error("metrics start failed", "addr", cfg.MetricsAddr, "error", err)
		return a.abort(fmt.Errorf("metrics: %w", err), true)
	}
	a.logger.Info("metrics started", "addr", cfg.MetricsAddr)
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
	return a.release(ctx, true, true)
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

// abort releases the dependencies after a failed Start.
// reloaderRunning is true when the failure came after the reloader started.
func (a *App) abort(cause error, reloaderRunning ...bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	return errors.Join(cause, a.release(ctx, false, len(reloaderRunning) > 0 && reloaderRunning[0]))
}

// release stops the metrics endpoint and the reloader only if they run, so a failed Start does not stop what never started.
func (a *App) release(ctx context.Context, metricsRunning, reloaderRunning bool) error {
	var errs []error
	if metricsRunning {
		errs = append(errs, a.metrics.Stop(ctx))
	}
	if a.reloader != nil && reloaderRunning {
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
