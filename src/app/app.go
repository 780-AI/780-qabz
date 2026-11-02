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

// Tracer exports traces.
type Tracer interface {
	Shutdown(ctx context.Context) error
}

// App owns every long-lived dependency. It starts and stops them in a fixed order.
// Make one with NewBuilder.
type App struct {
	cfg     config.Config
	logger  Logger
	db      Database
	cache   Cache
	metrics Metrics
	tracer  Tracer // nil when tracing is off

	mu      sync.Mutex
	started bool
}

// Config returns the settings the App was built with.
func (a *App) Config() config.Config { return a.cfg }

// Logger returns the shared logger.
func (a *App) Logger() Logger { return a.logger }

// Database returns the SQL store.
func (a *App) Database() Database { return a.db }

// Cache returns the key-value store.
func (a *App) Cache() Cache { return a.cache }

// Start checks the database and the cache with a ping and starts the metrics endpoint.
// If a step fails, Start stops what it opened and returns the error.
func (a *App) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.started {
		return errors.New("app: already started")
	}
	if err := a.ping(ctx, "postgres", a.cfg.PostgresTarget(), a.db); err != nil {
		return a.abort(err)
	}
	if err := a.ping(ctx, "redis", a.cfg.RedisAddr(), a.cache); err != nil {
		return a.abort(err)
	}
	if err := a.metrics.Start(ctx); err != nil {
		a.logger.Error("metrics start failed", "addr", a.cfg.MetricsAddr, "error", err)
		return a.abort(fmt.Errorf("metrics: %w", err))
	}
	a.logger.Info("metrics started", "addr", a.cfg.MetricsAddr)
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
	return a.release(ctx, true)
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
func (a *App) abort(cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), stopTimeout)
	defer cancel()
	return errors.Join(cause, a.release(ctx, false))
}

// release stops the metrics endpoint only if it runs, so a failed Start does not stop what never started.
func (a *App) release(ctx context.Context, metricsRunning bool) error {
	var errs []error
	if metricsRunning {
		errs = append(errs, a.metrics.Stop(ctx))
	}
	if a.tracer != nil {
		errs = append(errs, a.tracer.Shutdown(ctx))
	}
	a.cache.Close()
	a.db.Close()
	_ = a.logger.Sync() // stdout/stderr sync can fail on some systems; not an app error
	return errors.Join(errs...)
}
