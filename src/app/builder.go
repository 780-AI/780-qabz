package app

import (
	"errors"

	"qabz/config"
)

// Builder makes an App. Every dependency is an interface, so tests inject fakes.
// The config, logger, database, cache and metrics are required. The tracer, migrator, config reloader,
// provider list, gRPC server, HTTP gateway, internal gRPC server, background jobs, workers and the
// message broker are optional.
type Builder struct {
	cfg       *config.Holder[config.Config]
	logger    Logger
	db        Database
	cache     Cache
	metrics   Metrics
	tracer    Tracer
	migrator  Migrator
	reloader  ConfigReloader
	providers Providers
	grpc      Server
	http      Server
	internal  Server
	bg        Background
	crons     Crons
	broker    Broker
}

// NewBuilder returns an empty Builder.
func NewBuilder() *Builder { return &Builder{} }

// WithConfig sets the settings. It wraps them in a new Holder.
func (b *Builder) WithConfig(c config.Config) *Builder { b.cfg = config.NewHolder(c); return b }

// WithConfigHolder sets the settings holder. Use it when a reloader shares the holder with the App.
func (b *Builder) WithConfigHolder(h *config.Holder[config.Config]) *Builder { b.cfg = h; return b }

// WithMigrator sets the migrator. Skip it to run no migration at start.
func (b *Builder) WithMigrator(m Migrator) *Builder { b.migrator = m; return b }

// WithConfigReloader sets the hot-reload loop. Skip it to turn hot-reload off.
func (b *Builder) WithConfigReloader(r ConfigReloader) *Builder { b.reloader = r; return b }

// WithProviders sets the provider list. Skip it to load no providers.
func (b *Builder) WithProviders(p Providers) *Builder { b.providers = p; return b }

// WithGRPCServer sets the gRPC server. Skip it to run no gRPC server.
func (b *Builder) WithGRPCServer(s Server) *Builder { b.grpc = s; return b }

// WithHTTPServer sets the HTTP gateway. Skip it to run no HTTP gateway.
func (b *Builder) WithHTTPServer(s Server) *Builder { b.http = s; return b }

// WithInternalGRPCServer sets the internal gRPC server (task 044). Skip it to run none.
func (b *Builder) WithInternalGRPCServer(s Server) *Builder { b.internal = s; return b }

// WithBackground sets the background jobs that Stop waits for (task 044). Skip it for none.
func (b *Builder) WithBackground(bg Background) *Builder { b.bg = bg; return b }

// WithCrons sets the workers (task 045). Skip it for none.
func (b *Builder) WithCrons(c Crons) *Builder { b.crons = c; return b }

// WithBroker sets the message broker client (task 056: the NATS client). Skip it when there is none.
func (b *Builder) WithBroker(br Broker) *Builder { b.broker = br; return b }

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
	if b.cfg == nil {
		errs = append(errs, errors.New("app: config is required"))
	}
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
	return &App{cfg: b.cfg, logger: b.logger, db: b.db, cache: b.cache, metrics: b.metrics, tracer: b.tracer,
		migrator: b.migrator, reloader: b.reloader, providers: b.providers, grpc: b.grpc, http: b.http, internal: b.internal, bg: b.bg, crons: b.crons, broker: b.broker}, nil
}
