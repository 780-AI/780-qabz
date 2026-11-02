package app

import (
	"errors"

	"qabz/config"
)

// Builder makes an App. Every dependency is an interface, so tests inject fakes.
// The logger, database, cache and metrics are required. The tracer is optional.
type Builder struct {
	cfg     config.Config
	logger  Logger
	db      Database
	cache   Cache
	metrics Metrics
	tracer  Tracer
}

// NewBuilder returns an empty Builder.
func NewBuilder() *Builder { return &Builder{} }

// WithConfig sets the settings.
func (b *Builder) WithConfig(c config.Config) *Builder { b.cfg = c; return b }

// WithLogger sets the logger.
func (b *Builder) WithLogger(l Logger) *Builder { b.logger = l; return b }

// WithDatabase sets the SQL store.
func (b *Builder) WithDatabase(d Database) *Builder { b.db = d; return b }

// WithCache sets the key-value store.
func (b *Builder) WithCache(c Cache) *Builder { b.cache = c; return b }

// WithMetrics sets the metrics endpoint.
func (b *Builder) WithMetrics(m Metrics) *Builder { b.metrics = m; return b }

// WithTracer sets the tracer. Skip it to turn tracing off.
func (b *Builder) WithTracer(t Tracer) *Builder { b.tracer = t; return b }

// Build checks that every required dependency is set and returns the App.
func (b *Builder) Build() (*App, error) {
	var errs []error
	if b.logger == nil {
		errs = append(errs, errors.New("app: logger is required"))
	}
	if b.db == nil {
		errs = append(errs, errors.New("app: database is required"))
	}
	if b.cache == nil {
		errs = append(errs, errors.New("app: cache is required"))
	}
	if b.metrics == nil {
		errs = append(errs, errors.New("app: metrics is required"))
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return &App{cfg: b.cfg, logger: b.logger, db: b.db, cache: b.cache, metrics: b.metrics, tracer: b.tracer}, nil
}
