package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"qabz/core/order"
	"qabz/core/reconcile"
	"qabz/entry/cron"
	"qabz/provider"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
	"qabz/system/redis"
)

// liveRedisOf returns the address and password of the Redis of `make test` (QABZ_TEST_REDIS_URL)
// and a new key prefix of the test. Without the variable the test is skipped.
func liveRedisOf(t *testing.T) (addr, password, prefix string) {
	t.Helper()
	raw := os.Getenv("QABZ_TEST_REDIS_URL")
	if raw == "" {
		t.Skip("QABZ_TEST_REDIS_URL is not set (run make test)")
	}
	u, err := url.Parse(raw)
	if !assert.NoError(t, err) || !assert.NotNil(t, u) {
		t.FailNow()
	}
	if u.User != nil {
		password, _ = u.User.Password()
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); !assert.NoError(t, err) {
		t.FailNow()
	}
	return u.Host, password, "qabz_test_" + hex.EncodeToString(b) + ":"
}

// blockingStatus answers success for every order after release is closed, and counts its calls.
type blockingStatus struct {
	provider.ProviderInterface
	release chan struct{}
	mu      sync.Mutex
	calls   int
}

func (b *blockingStatus) Status(ctx context.Context, req provider.StatusRequest) (*provider.StatusResponse, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	select {
	case <-b.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return &provider.StatusResponse{Status: provider.StatusSuccess}, nil
}

type oneAdapter struct{ a provider.ProviderInterface }

func (o oneAdapter) Get(name string) (provider.ProviderInterface, bool) {
	return o.a, name == "parsian"
}

// TestLiveTwoRunnersOneLock (task 045): two App instances (two runners on two Redis clients) start
// the Parsian status worker at the same time. Only one takes the lock (the other counts no_lock),
// the lock key has the prefix, the pending order gets one success row and one Kafka message with
// the key bill and bill's value; after the run the own lock is deleted.
func TestLiveTwoRunnersOneLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	addr, password, prefix := liveRedisOf(t)
	db, err := postgres.New(ctx, postgrestest.SeededSchema(t, ctx, postgrestest.RunDSN(t)))
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	t.Cleanup(db.Close)

	const orderID = "cccccccc-0000-4000-8000-000000000451"
	o := order.Order{ID: "dddddddd-0000-4000-8000-000000000451", OrderID: orderID, BillType: "tel", BillID: "2101234560040", PayID: "12000035",
		Price: 150000, ProviderID: 50, ProviderName: "parsian", Status: order.StatusPending, RRN: "778899", BasePrice: 120000}
	if !assert.NoError(t, postgres.NewOrderRepository(db).Insert(ctx, o)) {
		return
	}
	_, err = db.Pool().Exec(ctx, `UPDATE orders SET created_at = NOW() - INTERVAL '1 hour' WHERE order_id = $1`, orderID)
	if !assert.NoError(t, err) {
		return
	}

	mc, err := ckafka.NewMockCluster(1)
	if !assert.NoError(t, err) || !assert.NotNil(t, mc) {
		return
	}
	defer mc.Close()
	if !assert.NoError(t, mc.CreateTopic("paymentwriter", 1, 1)) {
		return
	}

	cfg := testConfig(t)
	cfg.ParsianGetStatusEnabled = true
	cfg.ParsianGetStatusCronTime = time.Hour // one run each, at the start
	cfg.KafkaBrokers = []string{mc.BootstrapServers()}
	adapter := &blockingStatus{release: make(chan struct{})}

	var mu sync.Mutex
	results := map[string]int{}
	m := cronMetrics{onRun: func(name, result string) {
		mu.Lock()
		defer mu.Unlock()
		assert.Equal(t, "parsian-getstatus", name)
		results[result]++
	}}
	count := func(result string) int {
		mu.Lock()
		defer mu.Unlock()
		return results[result]
	}
	var jobs []*cronJobs
	for range 2 {
		cache := redis.New(addr, password, prefix)
		t.Cleanup(cache.Close)
		j, err := newStatusCrons(cfg, db, cache, oneAdapter{adapter}, m, &liveLogger{})
		if !assert.NoError(t, err) || !assert.NotNil(t, j) {
			return
		}
		jobs = append(jobs, j)
	}
	for _, j := range jobs {
		if !assert.NoError(t, j.Start(ctx)) {
			return
		}
	}
	defer func() {
		for _, j := range jobs {
			assert.NoError(t, j.Stop(context.Background()))
		}
	}()

	if !waitUntil(t, 15*time.Second, func() bool { return count(cron.ResultNoLock) == 1 }, "one runner has no lock") {
		return
	}
	rdb := goredis.NewClient(&goredis.Options{Addr: addr, Password: password})
	defer func() { _ = rdb.Close() }()
	keys, err := rdb.Keys(ctx, prefix+"*").Result()
	assert.NoError(t, err)
	assert.Equal(t, []string{prefix + "crons_parsian-getstatus_lock1"}, keys, "the lock key with the prefix, while the run holds it")

	close(adapter.release)
	if !waitUntil(t, 15*time.Second, func() bool { return count(cron.ResultDone) == 1 }, "the run is done") {
		return
	}
	assert.Equal(t, map[string]int{cron.ResultDone: 1, cron.ResultNoLock: 1}, results)

	got, err := postgres.NewOrderRepository(db).Latest(ctx, orderID, order.LatestWindow)
	if assert.NoError(t, err) {
		assert.Equal(t, order.StatusSuccess, got.Status)
		assert.Equal(t, "778899", got.RRN, "a copy of the pending row")
	}
	n, err := rdb.Exists(ctx, prefix+"crons_parsian-getstatus_lock1").Result()
	assert.NoError(t, err)
	assert.Zero(t, n, "the own lock is deleted after the run")
	adapter.mu.Lock()
	assert.Equal(t, 1, adapter.calls, "only one runner asked Parsian")
	adapter.mu.Unlock()

	c, err := ckafka.NewConsumer(&ckafka.ConfigMap{"bootstrap.servers": mc.BootstrapServers(), "group.id": "qabz-live", "enable.auto.commit": false})
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	defer func() { _ = c.Close() }()
	topic := "paymentwriter"
	if !assert.NoError(t, c.Assign([]ckafka.TopicPartition{{Topic: &topic, Partition: 0, Offset: ckafka.OffsetBeginning}})) {
		return
	}
	msg, err := c.ReadMessage(10 * time.Second)
	if assert.NoError(t, err) && assert.NotNil(t, msg) {
		want, _ := reconcile.StatusMessage(orderID, "success")
		assert.Equal(t, "bill", string(msg.Key))
		assert.Equal(t, string(want), string(msg.Value))
	}
	_, err = c.ReadMessage(500 * time.Millisecond)
	assert.Error(t, err, "one message only")
}
