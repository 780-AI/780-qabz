package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"

	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/core/reconcile"
	"qabz/entry/cron"
	"qabz/provider/parsianproxy"
	"qabz/system/kafka"
	"qabz/system/postgres"
	"qabz/system/redis"
)

// cronMetrics are the callbacks of the workers (task 045). With metrics off every one is nil and
// nothing is recorded.
type cronMetrics struct {
	onRun      func(name, result string)
	onDuration func(name string, d time.Duration)
	onOrder    func(name, result string)
	onCall     func(providerName, action, result string, d time.Duration)
	onPublish  func(result string, d time.Duration)
}

// newCronMetrics registers the metrics of the workers (conventions § 12; the names of system.html):
//   - qabz_cron_runs_total{cron,result}: each tick (done, no_lock, error);
//   - qabz_cron_duration_ms{cron}: each run that took the lock;
//   - qabz_cron_orders_processed_total{cron,result}: each order of a run (reconcile.Result*);
//   - qabz_kafka_publish_total{result} (ok, error) and qabz_kafka_publish_duration_ms;
//   - the status calls on qabz_provider_calls_total and qabz_provider_call_duration_ms (action
//     status), the collectors that newInquiryFlow registered (call it after newInquiryFlow).
func newCronMetrics(metrics *prometheus.Server) (cronMetrics, error) {
	var m cronMetrics
	reg := metrics.Registry()
	if reg == nil {
		return m, nil
	}
	runs := promclient.NewCounterVec(promclient.CounterOpts{
		Name: "qabz_cron_runs_total",
		Help: "Worker ticks by worker (cron: the get_status name, for example parsian-getstatus) and result (done; no_lock: another instance holds the lock; error: a failed lock, a run error, a panic or the App stop during the run).",
	}, []string{"cron", "result"})
	duration := promclient.NewHistogramVec(promclient.HistogramOpts{
		Name:    "qabz_cron_duration_ms",
		Help:    "Duration of each worker run that took the lock, in milliseconds, by worker (cron).",
		Buckets: []float64{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 300000, 900000},
	}, []string{"cron"})
	orders := promclient.NewCounterVec(promclient.CounterOpts{
		Name: "qabz_cron_orders_processed_total",
		Help: "Orders of the status worker by worker (cron) and result (success, failed: reported and saved; pending: no final status; late: after the reverse deadline, saved and not reported; skipped: final already; report_error; save_error; read_error).",
	}, []string{"cron", "result"})
	publishes := promclient.NewCounterVec(promclient.CounterOpts{
		Name: "qabz_kafka_publish_total",
		Help: "Kafka messages of the status worker to the payment service by result (ok: delivered; error: not delivered, or the wait ended).",
	}, []string{"result"})
	publishDuration := promclient.NewHistogram(promclient.HistogramOpts{
		Name:    "qabz_kafka_publish_duration_ms",
		Help:    "Duration of each Kafka publish (metadata, produce and delivery report) in milliseconds, for every result.",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
	})
	for _, c := range []promclient.Collector{runs, duration, orders, publishes, publishDuration} {
		if err := reg.Register(c); err != nil {
			return m, fmt.Errorf("cron: register metric: %w", err)
		}
	}
	calls, err := registerOrExisting[*promclient.CounterVec](reg, promclient.NewCounterVec(providerCallsOpts, providerCallLabels))
	if err != nil {
		return m, err
	}
	callDuration, err := registerOrExisting[*promclient.HistogramVec](reg, promclient.NewHistogramVec(providerCallDurationOpts, providerCallDurationLabels))
	if err != nil {
		return m, err
	}
	m.onRun = func(name, result string) { runs.WithLabelValues(name, result).Inc() }
	m.onDuration = func(name string, d time.Duration) {
		duration.WithLabelValues(name).Observe(float64(d) / float64(time.Millisecond))
	}
	m.onOrder = func(name, result string) { orders.WithLabelValues(name, result).Inc() }
	m.onCall = func(providerName, action, result string, d time.Duration) {
		calls.WithLabelValues(providerName, action, result).Inc()
		callDuration.WithLabelValues(providerName, action).Observe(float64(d) / float64(time.Millisecond))
	}
	m.onPublish = func(result string, d time.Duration) {
		publishes.WithLabelValues(result).Inc()
		publishDuration.Observe(float64(d) / float64(time.Millisecond))
	}
	return m, nil
}

// registerOrExisting registers c, or gives the collector that is registered under the same description.
func registerOrExisting[T promclient.Collector](reg promclient.Registerer, c T) (T, error) {
	err := reg.Register(c)
	if err == nil {
		return c, nil
	}
	var are promclient.AlreadyRegisteredError
	if errors.As(err, &are) {
		if existing, ok := are.ExistingCollector.(T); ok {
			return existing, nil
		}
	}
	var zero T
	return zero, fmt.Errorf("cron: register metric: %w", err)
}

// cronJobs are the runner and the Kafka producer that its worker uses. Stop ends the runs first,
// then sends what the producer has left and closes it.
type cronJobs struct {
	runner   *cron.Runner
	producer *kafka.Producer
}

func (j *cronJobs) Start(ctx context.Context) error { return j.runner.Start(ctx) }

func (j *cronJobs) Stop(ctx context.Context) error {
	return errors.Join(j.runner.Stop(ctx), j.producer.Close(ctx))
}

// statusCron makes a reconcile.StatusWorker a cron.Cron.
type statusCron struct{ *reconcile.StatusWorker }

func (c statusCron) Lock() cron.LockPolicy {
	return cron.LockPolicy{Chunk: c.Chunk(), SyncOperations: c.SyncOperations()}
}

func (c statusCron) Run(ctx context.Context, ext cron.Extender) error {
	return c.StatusWorker.Run(ctx, ext)
}

// redisLocker makes the Redis client a cron.Locker (the key gets database.redis.prefix).
type redisLocker struct{ c *redis.Client }

func (l redisLocker) TryLock(ctx context.Context, key string, ttl time.Duration) (cron.Lock, error) {
	lk, err := l.c.TryLock(ctx, key, ttl)
	if err != nil || lk == nil {
		return nil, err // no typed nil in the interface
	}
	return lk, nil
}

// newStatusCrons builds the Parsian status worker (task 045; bill: ParsianGetStatusTask) and its
// runner: the orders of the provider parsian from the get_status keys, the adapter of the registry
// (the mock in dev mode), the reports on Kafka (kafka.brokers, kafka.topic.payment.write.subject,
// the key and the client ID service.ecosystem_name) and the Redis lock.
func newStatusCrons(cfg config.Config, db *postgres.Client, cache *redis.Client, adapters reconcile.Adapters, m cronMetrics, logger Logger) (*cronJobs, error) {
	if cache == nil {
		return nil, errors.New("cron: the Redis client is required")
	}
	producer, err := kafka.New(kafka.Options{Brokers: cfg.KafkaBrokers, ClientID: cfg.ServiceEcosystemName, OnPublish: m.onPublish})
	if err != nil {
		return nil, err
	}
	reporter, err := reconcile.NewPaymentReporter(producer, cfg.KafkaPaymentWriteSubject, cfg.ServiceEcosystemName)
	if err != nil {
		_ = producer.Close(context.Background())
		return nil, err
	}
	var workerLog reconcile.Logger // nil: no line
	var runnerLog cron.Logger      // nil: no line
	if logger != nil {
		workerLog, runnerLog = logger, logger
	}
	worker, err := reconcile.NewStatusWorker(reconcile.StatusOptions{
		Name:            cfg.ParsianGetStatusName,
		Provider:        parsianproxy.Name,
		Interval:        cfg.ParsianGetStatusCronTime,
		Start:           cfg.ParsianGetStatusStart,
		End:             cfg.ParsianGetStatusEnd,
		Chunk:           cfg.ParsianGetStatusChunk,
		ReverseDeadline: cfg.ParsianGetStatusReverseDeadline,
		SyncOperations:  []time.Duration{cfg.ParsianGetStatusSyncAPICall, cfg.ParsianGetStatusSyncGetOrder, cfg.ParsianGetStatusSyncInsertOrder},
		Repository:      postgres.NewOrderRepository(db),
		Adapters:        adapters,
		Reporter:        reporter,
		Logger:          workerLog,
		OnOrder:         m.onOrder,
		OnCall:          m.onCall,
	})
	if err != nil {
		_ = producer.Close(context.Background())
		return nil, err
	}
	lockTime := cfg.ParsianGetStatusLockTime
	runner, err := cron.NewRunner(cron.Options{
		Crons:      []cron.Cron{statusCron{worker}},
		Locker:     redisLocker{c: cache},
		LockTime:   func(string) time.Duration { return lockTime }, // bill: only logged
		Logger:     runnerLog,
		OnRun:      m.onRun,
		OnDuration: m.onDuration,
	})
	if err != nil {
		_ = producer.Close(context.Background())
		return nil, err
	}
	return &cronJobs{runner: runner, producer: producer}, nil
}
