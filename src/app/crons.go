package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"

	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/core/order"
	"qabz/core/reconcile"
	"qabz/entry/cron"
	"qabz/provider/irancellproxy"
	"qabz/provider/jiring"
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
	// onRunDone observes qabz_cron_orders_fixed: the orders that a status worker run made final
	// (task 050 D8).
	onRunDone func(name string, fixed int)
	onCall    func(providerName, action, result string, d time.Duration)
	onPublish func(result string, d time.Duration)
	// onSettle counts a settlement of the settlement worker on qabz_order_settle_total (source
	// worker; task 046).
	onSettle func(billType, providerName, result string)
}

// newCronMetrics registers the metrics of the workers (conventions § 12; the names of system.html):
//   - qabz_cron_runs_total{cron,result}: each tick (done, no_lock, error);
//   - qabz_cron_duration_ms{cron}: each run that took the lock;
//   - qabz_cron_orders_processed_total{cron,result}: each order of a run (reconcile.Result*);
//   - qabz_cron_orders_fixed{cron}: the orders that one status worker run made final (task 050);
//   - qabz_kafka_publish_total{result} (ok, error) and qabz_kafka_publish_duration_ms;
//   - the settlements of the settlement worker on qabz_order_settle_total (source worker; task 046);
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
		Help: "Orders of a worker run by worker (cron) and result. The status worker: success, failed (reported and saved); pending (no final status); late (after the reverse deadline, saved and not reported); skipped (final already); report_error; save_error; read_error. The settlement worker: the result of the settlement (success, failed, pending, error, confirm_error, no_data, no_route; see qabz_order_settle_total).",
	}, []string{"cron", "result"})
	fixed := promclient.NewHistogramVec(promclient.HistogramOpts{
		Name:    "qabz_cron_orders_fixed",
		Help:    "Orders that one status worker run moved to a final status (success, failed and the late saves after the reverse deadline), by worker (cron); one observation at the end of each run that took the lock, also 0 and also a run that ended with an error.",
		Buckets: []float64{0, 1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500},
	}, []string{"cron"})
	publishes := promclient.NewCounterVec(promclient.CounterOpts{
		Name: "qabz_kafka_publish_total",
		Help: "Kafka messages of the status worker to the payment service by result (ok: delivered; error: not delivered, or the wait ended).",
	}, []string{"result"})
	publishDuration := promclient.NewHistogram(promclient.HistogramOpts{
		Name:    "qabz_kafka_publish_duration_ms",
		Help:    "Duration of each Kafka publish (metadata, produce and delivery report) in milliseconds, for every result.",
		Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000},
	})
	for _, c := range []promclient.Collector{runs, duration, orders, fixed, publishes, publishDuration} {
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
	settles, err := registerOrExisting[*promclient.CounterVec](reg, promclient.NewCounterVec(settleOpts, settleLabels))
	if err != nil {
		return m, err
	}
	m.onSettle = func(billType, providerName, result string) {
		settles.WithLabelValues(billType, providerName, result, order.SettleSourceWorker).Inc()
	}
	m.onRun = func(name, result string) { runs.WithLabelValues(name, result).Inc() }
	m.onDuration = func(name string, d time.Duration) {
		duration.WithLabelValues(name).Observe(float64(d) / float64(time.Millisecond))
	}
	m.onOrder = func(name, result string) { orders.WithLabelValues(name, result).Inc() }
	m.onRunDone = func(name string, n int) { fixed.WithLabelValues(name).Observe(float64(n)) }
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

// cronJobs are the runner and the Kafka producer of the status workers (nil while both are off). Stop ends the runs first, then sends what the producer has left and closes it.
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

// settleCron makes a reconcile.SettleWorker a cron.Cron. The lock has no sync operations, so its
// TTL is bill's default (4 h + 5 min, 2 min per order; bill: GetSyncOperations gives nil).
type settleCron struct{ *reconcile.SettleWorker }

func (c settleCron) Lock() cron.LockPolicy { return cron.LockPolicy{Chunk: c.Chunk()} }

func (c settleCron) Run(ctx context.Context, ext cron.Extender) error {
	return c.SettleWorker.Run(ctx, ext)
}

// newCrons builds the enabled workers and one runner for them (the Redis lock):
//   - the Parsian status worker (task 045; bill: ParsianGetStatusTask) with
//     providers.internal.parsian.get_status.enabled: the orders of the provider parsian from the
//     get_status keys, the adapter of the registry (the mock in dev mode), the reports on Kafka
//     (kafka.brokers, kafka.topic.payment.write.subject, the key and the client ID
//     service.ecosystem_name, each publish limited by kafka.timeout);
//   - the Irancell status worker (task 050; bill: IrancellGetStatusTask) with
//     providers.internal.irancell.get_status.enabled: the same worker for the provider irancell,
//     with its own get_status keys, and the same Kafka producer and reporter;
//   - the Jiring Switch status worker (task 058; bill: JiringSwitchGetStatusTask) with
//     providers.internal.jiring_switch.get_status.enabled: the same worker for the provider
//     jiring-switch (the orders of an MCI payment that Jiring Switch left pending, also after a
//     fallback from Parsian), with its own get_status keys, and the same Kafka producer and reporter;
//   - the settlement worker (task 046; bill: ConfirmPaymentTelephone) with
//     confirm_payment.telephone.enabled: the paid landline orders that Karashop did not confirm,
//     settled with the Karashop adapter of the registry (the mock in dev mode) by order.Settler.
//
// With no worker enabled: an error (the caller builds no runner then).
func newCrons(cfg config.Config, db *postgres.Client, cache *redis.Client, adapters reconcile.Adapters, m cronMetrics, logger Logger) (*cronJobs, error) {
	if cache == nil {
		return nil, errors.New("cron: the Redis client is required")
	}
	var workerLog reconcile.Logger // nil: no line
	var runnerLog cron.Logger      // nil: no line
	var settleLog order.PaymentLogger
	if logger != nil {
		workerLog, runnerLog, settleLog = logger, logger, logger
	}
	var crons []cron.Cron
	lockTimes := map[string]time.Duration{}
	var producer *kafka.Producer
	fail := func(err error) (*cronJobs, error) {
		_ = producer.Close(context.Background())
		return nil, err
	}
	var reporter *reconcile.PaymentReporter
	if cfg.StatusWorkerEnabled() {
		var err error
		producer, err = kafka.New(kafka.Options{Brokers: cfg.KafkaBrokers, ClientID: cfg.ServiceEcosystemName, OnPublish: m.onPublish})
		if err != nil {
			return nil, err
		}
		reporter, err = reconcile.NewPaymentReporter(producer, cfg.KafkaPaymentWriteSubject, cfg.ServiceEcosystemName, cfg.KafkaTimeout)
		if err != nil {
			return fail(err)
		}
	}
	statusWorker := func(o reconcile.StatusOptions, lockTime time.Duration) error {
		o.Repository, o.Adapters, o.Reporter = postgres.NewOrderRepository(db), adapters, reporter
		o.Logger, o.OnOrder, o.OnCall, o.OnRunDone = workerLog, m.onOrder, m.onCall, m.onRunDone
		worker, err := reconcile.NewStatusWorker(o)
		if err != nil {
			return err
		}
		crons = append(crons, statusCron{worker})
		lockTimes[worker.Name()] = lockTime
		return nil
	}
	if cfg.ParsianGetStatusEnabled {
		if err := statusWorker(reconcile.StatusOptions{
			Name:            cfg.ParsianGetStatusName,
			Provider:        parsianproxy.Name,
			Interval:        cfg.ParsianGetStatusCronTime,
			Start:           cfg.ParsianGetStatusStart,
			End:             cfg.ParsianGetStatusEnd,
			Chunk:           cfg.ParsianGetStatusChunk,
			ReverseDeadline: cfg.ParsianGetStatusReverseDeadline,
			SyncOperations:  []time.Duration{cfg.ParsianGetStatusSyncAPICall, cfg.ParsianGetStatusSyncGetOrder, cfg.ParsianGetStatusSyncInsertOrder},
		}, cfg.ParsianGetStatusLockTime); err != nil {
			return fail(err)
		}
	}
	if cfg.IrancellGetStatusEnabled {
		if err := statusWorker(reconcile.StatusOptions{
			Name:            cfg.IrancellGetStatusName,
			Provider:        irancellproxy.Name,
			Interval:        cfg.IrancellGetStatusCronTime,
			Start:           cfg.IrancellGetStatusStart,
			End:             cfg.IrancellGetStatusEnd,
			Chunk:           cfg.IrancellGetStatusChunk,
			ReverseDeadline: cfg.IrancellGetStatusReverseDeadline,
			SyncOperations:  []time.Duration{cfg.IrancellGetStatusSyncAPICall, cfg.IrancellGetStatusSyncGetOrder, cfg.IrancellGetStatusSyncInsertOrder},
		}, cfg.IrancellGetStatusLockTime); err != nil {
			return fail(err)
		}
	}
	if cfg.JiringSwitchGetStatusEnabled {
		if err := statusWorker(reconcile.StatusOptions{
			Name:            cfg.JiringSwitchGetStatusName,
			Provider:        jiring.Name,
			Interval:        cfg.JiringSwitchGetStatusCronTime,
			Start:           cfg.JiringSwitchGetStatusStart,
			End:             cfg.JiringSwitchGetStatusEnd,
			Chunk:           cfg.JiringSwitchGetStatusChunk,
			ReverseDeadline: cfg.JiringSwitchGetStatusReverseDeadline,
			SyncOperations:  []time.Duration{cfg.JiringSwitchGetStatusSyncAPICall, cfg.JiringSwitchGetStatusSyncGetOrder, cfg.JiringSwitchGetStatusSyncInsertOrder},
		}, cfg.JiringSwitchGetStatusLockTime); err != nil {
			return fail(err)
		}
	}
	if cfg.ConfirmPaymentTelephoneEnabled {
		repo := postgres.NewOrderRepository(db)
		settler, err := order.NewSettler(order.SettlerOptions{
			Repository: repo, Adapters: adapters, Routes: settleRoutes(), Source: order.SettleSourceWorker,
			Logger: settleLog, OnSettle: m.onSettle,
		})
		if err != nil {
			return fail(err)
		}
		worker, err := reconcile.NewSettleWorker(reconcile.SettleOptions{
			Name:       cfg.ConfirmPaymentTelephoneName,
			Interval:   cfg.ConfirmPaymentTelephoneEvery,
			Start:      cfg.ConfirmPaymentTelephoneStart,
			End:        cfg.ConfirmPaymentTelephoneEnd,
			Chunk:      cfg.ConfirmPaymentTelephoneChunk,
			Repository: repo,
			Settler:    settler,
			Logger:     workerLog,
			OnOrder:    m.onOrder,
		})
		if err != nil {
			return fail(err)
		}
		crons = append(crons, settleCron{worker})
		lockTimes[worker.Name()] = cfg.ConfirmPaymentTelephoneLockTime
	}
	if len(crons) == 0 {
		return nil, errors.New("cron: no worker is enabled")
	}
	runner, err := cron.NewRunner(cron.Options{
		Crons:      crons,
		Locker:     redisLocker{c: cache},
		LockTime:   func(name string) time.Duration { return lockTimes[name] }, // bill: only logged; read-only map
		Logger:     runnerLog,
		OnRun:      m.onRun,
		OnDuration: m.onDuration,
	})
	if err != nil {
		return fail(err)
	}
	return &cronJobs{runner: runner, producer: producer}, nil
}
