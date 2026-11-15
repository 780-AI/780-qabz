package app

import (
	"context"
	"testing"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"

	"qabz/common/monitoring/prometheus"
	"qabz/core/reconcile"
	"qabz/entry/cron"
	"qabz/system/kafka"
	"qabz/system/postgres"
	"qabz/system/redis"
)

var cronMetricNames = []string{"qabz_cron_runs_total", "qabz_cron_duration_ms", "qabz_cron_orders_processed_total",
	"qabz_kafka_publish_total", "qabz_kafka_publish_duration_ms"}

// TestCronMetrics (task 045): with metrics on, each callback records its label values, the status
// calls go to the provider call metrics that newInquiryFlow registered; with metrics off every
// callback is nil and nothing is registered; a second call conflicts.
func TestCronMetrics(t *testing.T) {
	off, err := newCronMetrics(prometheus.New("127.0.0.1:0", false))
	assert.NoError(t, err)
	assert.Nil(t, off.onRun)
	assert.Nil(t, off.onDuration)
	assert.Nil(t, off.onOrder)
	assert.Nil(t, off.onCall)
	assert.Nil(t, off.onPublish)

	on := prometheus.New("127.0.0.1:0", true)
	reg := on.Registry()
	if !assert.NotNil(t, reg) {
		return
	}
	// The inquiry flow registered the provider call metrics first (the order of the composition root).
	calls := promclient.NewCounterVec(providerCallsOpts, providerCallLabels)
	if !assert.NoError(t, reg.Register(calls)) || !assert.NoError(t, reg.Register(promclient.NewHistogramVec(providerCallDurationOpts, providerCallDurationLabels))) {
		return
	}
	m, err := newCronMetrics(on)
	if !assert.NoError(t, err) || !assert.NotNil(t, m.onRun) {
		return
	}
	for _, result := range []string{cron.ResultDone, cron.ResultNoLock, cron.ResultError} {
		m.onRun("parsian-getstatus", result)
	}
	m.onDuration("parsian-getstatus", 1500*time.Microsecond)
	for _, result := range []string{reconcile.ResultSuccess, reconcile.ResultFailed, reconcile.ResultPending, reconcile.ResultLate,
		reconcile.ResultSkipped, reconcile.ResultReportError, reconcile.ResultSaveError, reconcile.ResultReadError} {
		m.onOrder("parsian-getstatus", result)
	}
	m.onCall("parsian", "status", "ok", time.Millisecond)
	m.onPublish(kafka.ResultOK, time.Millisecond)
	m.onPublish(kafka.ResultError, time.Millisecond)

	families, err := reg.Gather()
	if !assert.NoError(t, err) {
		return
	}
	got := map[string]int{}
	for _, f := range families {
		got[f.GetName()] = len(f.GetMetric())
	}
	assert.Equal(t, 3, got["qabz_cron_runs_total"], "done, no_lock, error")
	assert.Equal(t, 1, got["qabz_cron_duration_ms"])
	assert.Equal(t, 8, got["qabz_cron_orders_processed_total"], "every result")
	assert.Equal(t, 2, got["qabz_kafka_publish_total"], "ok, error")
	assert.Equal(t, 1, got["qabz_kafka_publish_duration_ms"])
	var sample dto.Metric
	if assert.NoError(t, calls.WithLabelValues("parsian", "status", "ok").Write(&sample)) {
		assert.Equal(t, 1.0, sample.GetCounter().GetValue(), "the registered provider call counter")
	}

	_, err = newCronMetrics(on)
	assert.ErrorContains(t, err, "cron: register metric")
}

// TestRegisterOrExisting: a free name is registered; the same description gives the registered
// collector; another description with the same name is an error.
func TestRegisterOrExisting(t *testing.T) {
	reg := promclient.NewRegistry()
	first, err := registerOrExisting[*promclient.CounterVec](reg, promclient.NewCounterVec(providerCallsOpts, providerCallLabels))
	if !assert.NoError(t, err) {
		return
	}
	again, err := registerOrExisting[*promclient.CounterVec](reg, promclient.NewCounterVec(providerCallsOpts, providerCallLabels))
	assert.NoError(t, err)
	assert.Same(t, first, again)
	_, err = registerOrExisting[*promclient.CounterVec](reg, promclient.NewCounterVec(promclient.CounterOpts{Name: providerCallsOpts.Name, Help: "other"}, []string{"x"}))
	assert.Error(t, err)
}

// TestStatusCronAdapters: the lock policy of the worker; the Redis locker gives a nil Lock (no
// typed nil) when it has no lock.
func TestStatusCronAdapters(t *testing.T) {
	w, err := reconcile.NewStatusWorker(reconcile.StatusOptions{Name: "n", Provider: "parsian", Interval: time.Minute, Start: time.Minute,
		End: time.Hour, Chunk: 100, SyncOperations: []time.Duration{30 * time.Second, 3 * time.Millisecond, 3 * time.Millisecond},
		Repository: postgres.NewOrderRepository(nil), Adapters: cacheTestAdapters{}, Reporter: &reconcile.PaymentReporter{}})
	if !assert.NoError(t, err) || !assert.NotNil(t, w) {
		return
	}
	c := statusCron{w}
	assert.Equal(t, cron.LockPolicy{Chunk: 100, SyncOperations: []time.Duration{30 * time.Second, 3 * time.Millisecond, 3 * time.Millisecond}}, c.Lock())

	l := redisLocker{c: redis.New("127.0.0.1:1", "", "p:")}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	lk, err := l.TryLock(ctx, "k", time.Second)
	assert.Error(t, err)
	assert.True(t, lk == nil, "no typed nil in the interface")
}

// TestNewStatusCronsRules: no Redis client and a bad Kafka config stop the build.
func TestNewStatusCronsRules(t *testing.T) {
	cfg := testConfig(t)
	cfg.ParsianGetStatusEnabled = true
	_, err := newStatusCrons(cfg, nil, nil, cacheTestAdapters{}, cronMetrics{}, nil)
	assert.ErrorContains(t, err, "Redis client is required")

	cache := redis.New("127.0.0.1:1", "", "p:")
	defer cache.Close()
	bad := cfg
	bad.KafkaBrokers = nil
	_, err = newStatusCrons(bad, nil, cache, cacheTestAdapters{}, cronMetrics{}, nil)
	assert.ErrorContains(t, err, "at least one broker")
	bad = cfg
	bad.KafkaPaymentWriteSubject = ""
	_, err = newStatusCrons(bad, nil, cache, cacheTestAdapters{}, cronMetrics{}, nil)
	assert.ErrorContains(t, err, "topic")
	bad = cfg
	bad.ParsianGetStatusChunk = 0
	_, err = newStatusCrons(bad, nil, cache, cacheTestAdapters{}, cronMetrics{}, nil)
	assert.ErrorContains(t, err, "chunk")

	jobs, err := newStatusCrons(cfg, nil, cache, cacheTestAdapters{}, cronMetrics{}, nil)
	if assert.NoError(t, err) && assert.NotNil(t, jobs) {
		assert.NoError(t, jobs.Stop(context.Background()), "a stop before the start closes the producer")
	}
}

// TestDefaultBuilderCrons (task 045): with the worker switch off (the default) the App has no
// workers and one info line names the key; with it on, the App has the workers and the cron
// metrics are registered in both cases (with metrics on).
func TestDefaultBuilderCrons(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := testConfig(t)
		cfg.ParsianGetStatusEnabled = enabled
		b, err := NewDefaultBuilder(context.Background(), cfg)
		if !assert.NoError(t, err) || !assert.NotNil(t, b) {
			continue
		}
		a := build(t, b)
		assert.Equal(t, enabled, b.crons != nil, "enabled=%v", enabled)
		m := metricsServer(t, b)
		for _, name := range cronMetricNames {
			assert.True(t, registered(t, m.Registry(), name), name)
		}
		if jobs, ok := b.crons.(*cronJobs); ok && jobs != nil {
			assert.NoError(t, jobs.Stop(context.Background()))
		}
		a.Database().Close()
		a.Cache().Close()
	}
}
