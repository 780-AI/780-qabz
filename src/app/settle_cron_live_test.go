package app

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"

	"qabz/core/order"
	"qabz/entry/cron"
	"qabz/provider"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
	"qabz/system/redis"
)

// blockingSettle answers each settlement after release is closed: success, or the status in fail
// for the order ID. It records the order IDs.
type blockingSettle struct {
	provider.ProviderInterface
	release chan struct{}
	fail    map[string]string // by bill ID
	mu      sync.Mutex
	sent    []string // bill IDs
}

func (b *blockingSettle) Settle(ctx context.Context, req provider.SettleRequest) (*provider.SettleResponse, error) {
	b.mu.Lock()
	b.sent = append(b.sent, req.BillID)
	b.mu.Unlock()
	select {
	case <-b.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if status, ok := b.fail[req.BillID]; ok {
		return &provider.SettleResponse{Status: status, Code: 400}, nil
	}
	return &provider.SettleResponse{Status: provider.StatusSuccess, Code: 200}, nil
}

func (b *blockingSettle) sentIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := append([]string(nil), b.sent...)
	sort.Strings(out)
	return out
}

type karashopOnly struct{ a provider.ProviderInterface }

func (k karashopOnly) Get(name string) (provider.ProviderInterface, bool) {
	return k.a, name == "karashop"
}

// TestLiveSettleWorker (task 046, verification steps 1 to 5 with stubs): two App instances (two
// runners on two Redis clients) start the settlement worker at the same time. Only one takes the
// lock (the lock key has the prefix); it sends only the paid, not raw, not confirmed orders of tel
// and telmidterm in the window; a Karashop success sets provider_confirmed, a failure leaves it
// false; the settlements are counted with source worker; the own lock is deleted after the run.
func TestLiveSettleWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	addr, password, prefix := liveRedisOf(t)
	db, err := postgres.New(ctx, postgrestest.SeededSchema(t, ctx, postgrestest.RunDSN(t)))
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	t.Cleanup(db.Close)

	// One success row per order; the bill ID names the case.
	rows := []struct {
		bill, billType string
		raw, confirmed bool
		age            time.Duration
	}{
		{"ok-tel", "tel", false, false, time.Hour},
		{"ok-mid", "telmidterm", false, false, 2 * time.Hour},
		{"fail-tel", "tel", false, false, 3 * time.Hour},
		{"raw", "tel", true, false, time.Hour},
		{"confirmed", "tel", false, true, time.Hour},
		{"too-old", "tel", false, false, 25 * time.Hour},
		{"too-new", "tel", false, false, time.Minute},
		{"water", "water", false, false, time.Hour},
	}
	repo := postgres.NewOrderRepository(db)
	orderOf := map[string]string{}
	for i, r := range rows {
		id := fmt.Sprintf("cccccccc-0000-4000-8000-%012d", 4600+i)
		orderOf[r.bill] = id
		o := order.Order{ID: fmt.Sprintf("dddddddd-0000-4000-8000-%012d", 4600+i), OrderID: id, BillType: r.billType, BillID: r.bill,
			PayID: "12000035", Price: 150000, Data: "02112345678", ProviderID: 50, ProviderName: "parsian", Status: order.StatusSuccess,
			RRN: "778899", BasePrice: 120000, IsRaw: r.raw, ProviderConfirmed: r.confirmed}
		if !assert.NoError(t, repo.Insert(ctx, o)) {
			return
		}
		_, err = db.Pool().Exec(ctx, `UPDATE orders SET created_at = NOW() - make_interval(secs => $2) WHERE order_id = $1`, id, r.age.Seconds())
		if !assert.NoError(t, err) {
			return
		}
	}

	cfg := testConfig(t)
	cfg.ConfirmPaymentTelephoneEnabled = true
	cfg.ConfirmPaymentTelephoneEvery = time.Hour // one run each, at the start
	cfg.ConfirmPaymentTelephoneChunk = 1         // a page per order: the keyset paging
	adapter := &blockingSettle{release: make(chan struct{}), fail: map[string]string{"fail-tel": provider.StatusFailed}}

	var mu sync.Mutex
	runs, settles := map[string]int{}, map[string]int{}
	m := cronMetrics{
		onRun: func(name, result string) {
			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, "telephone-confirmpayment", name)
			runs[result]++
		},
		onSettle: func(billType, providerName, result string) {
			mu.Lock()
			defer mu.Unlock()
			settles[billType+"/"+providerName+"/"+result]++
		},
	}
	count := func(result string) int {
		mu.Lock()
		defer mu.Unlock()
		return runs[result]
	}
	var jobs []*cronJobs
	for range 2 {
		cache := redis.New(addr, password, prefix)
		t.Cleanup(cache.Close)
		j, err := newCrons(cfg, db, cache, karashopOnly{adapter}, m, &liveLogger{})
		if !assert.NoError(t, err) || !assert.NotNil(t, j) {
			return
		}
		assert.Nil(t, j.producer, "no Kafka producer with the status worker off")
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
	assert.Equal(t, []string{prefix + "crons_telephone-confirmpayment_lock1"}, keys, "the lock key with the prefix, while the run holds it")

	close(adapter.release)
	if !waitUntil(t, 15*time.Second, func() bool { return count(cron.ResultDone) == 1 }, "the run is done") {
		return
	}
	mu.Lock()
	assert.Equal(t, map[string]int{cron.ResultDone: 1, cron.ResultNoLock: 1}, runs)
	assert.Equal(t, map[string]int{"tel/karashop/success": 1, "telmidterm/karashop/success": 1, "tel/karashop/failed": 1}, settles)
	mu.Unlock()
	assert.Equal(t, []string{"fail-tel", "ok-mid", "ok-tel"}, adapter.sentIDs(), "only the orders of the query, each once")

	for bill, want := range map[string]bool{"ok-tel": true, "ok-mid": true, "fail-tel": false, "raw": false, "too-old": false, "too-new": false, "water": false} {
		var confirmed bool
		err := db.Pool().QueryRow(ctx, `SELECT provider_confirmed FROM orders WHERE order_id = $1`, orderOf[bill]).Scan(&confirmed)
		if assert.NoError(t, err, bill) {
			assert.Equal(t, want, confirmed, bill)
		}
	}
	n, err := rdb.Exists(ctx, prefix+"crons_telephone-confirmpayment_lock1").Result()
	assert.NoError(t, err)
	assert.Zero(t, n, "the own lock is deleted after the run")
}
