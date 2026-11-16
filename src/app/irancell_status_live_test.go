package app

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/irancell"
	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/core/order"
	"qabz/core/reconcile"
	"qabz/entry/cron"
	"qabz/provider"
	"qabz/provider/irancellproxy"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
	"qabz/system/redis"
)

// irancellOrderID is the order ID number n of the Irancell worker tests (task 050).
func irancellOrderID(n int) string { return fmt.Sprintf("cccccccc-0000-4000-8000-%012d", 500+n) }

// irancellStatusServer is the GetStatus of a fake irancell service: the status of each order from
// the map, a gRPC error for an order in fail; it records the requests.
type irancellStatusServer struct {
	pb.UnimplementedInternalServiceServer
	mu       sync.Mutex
	statuses map[string]string
	fail     map[string]bool
	reqs     []*pb.GetStatusRequest
}

func (s *irancellStatusServer) GetStatus(_ context.Context, in *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, in)
	if s.fail[in.GetOrderId()] {
		return nil, status.Error(codes.Unavailable, "irancell is down")
	}
	return &pb.GetStatusResponse{Status: s.statuses[in.GetOrderId()], Message: "ok", Tid: "8800"}, nil
}

// namedAdapter gives the adapter only for its name.
type namedAdapter struct {
	name string
	a    provider.ProviderInterface
}

func (n namedAdapter) Get(name string) (provider.ProviderInterface, bool) { return n.a, name == n.name }

// liveIrancellWorker is one Irancell status worker of the App on the run database, the test Redis
// and a mock Kafka cluster, with the real Irancell adapter over gRPC to a fake irancell service.
type liveIrancellWorker struct {
	db      *postgres.Client
	repo    *postgres.OrderRepository
	srv     *irancellStatusServer
	kafka   *ckafka.MockCluster
	log     *liveLogger
	mu      sync.Mutex
	runs    map[string]int
	results map[string]int
	fixed   []int
}

// newLiveIrancellWorker inserts one pending row for each status age (the order n gets the age
// ages[n-1]) and builds the server; run starts the worker with chunk and start.
func newLiveIrancellWorker(t *testing.T, ctx context.Context, ages []time.Duration) *liveIrancellWorker {
	t.Helper()
	db, err := postgres.New(ctx, postgrestest.SeededSchema(t, ctx, postgrestest.RunDSN(t)))
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	t.Cleanup(db.Close)
	w := &liveIrancellWorker{db: db, repo: postgres.NewOrderRepository(db), log: &liveLogger{}, runs: map[string]int{}, results: map[string]int{},
		srv: &irancellStatusServer{statuses: map[string]string{}, fail: map[string]bool{}}}
	for i, age := range ages {
		id := fmt.Sprintf("dddddddd-0000-4000-8000-%012d", 500+i+1)
		o := order.Order{ID: id, OrderID: irancellOrderID(i + 1), BillType: provider.BillTypeIrancell, Data: "989350000501", Price: 459_000,
			ProviderID: 26, ProviderName: irancellproxy.Name, Status: order.StatusPending, BasePrice: 450_000}
		if !assert.NoError(t, w.repo.Insert(ctx, o)) {
			t.FailNow()
		}
		if _, err := db.Pool().Exec(ctx, `UPDATE orders SET created_at = NOW() - make_interval(secs => $2) WHERE uuid = $1::uuid`, id, age.Seconds()); !assert.NoError(t, err) {
			t.FailNow()
		}
	}
	mc, err := ckafka.NewMockCluster(1)
	if !assert.NoError(t, err) || !assert.NotNil(t, mc) || !assert.NoError(t, mc.CreateTopic("paymentwriter", 1, 1)) {
		t.FailNow()
	}
	t.Cleanup(mc.Close)
	w.kafka = mc
	return w
}

// run starts the worker (one run at the start), waits for the end of the run and stops it.
func (w *liveIrancellWorker) run(t *testing.T, ctx context.Context, chunk int, start time.Duration) bool {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return false
	}
	gs := grpc.NewServer()
	pb.RegisterInternalServiceServer(gs, w.srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	client, err := irancellproxy.Dial(lis.Addr().String())
	if !assert.NoError(t, err) {
		return false
	}
	adapter, err := irancellproxy.New(irancellproxy.Options{Enabled: true, Client: client, Timeout: 5 * time.Second})
	if !assert.NoError(t, err) {
		return false
	}

	addr, password, prefix := liveRedisOf(t)
	cfg := testConfig(t)
	cfg.IrancellGetStatusEnabled = true
	cfg.IrancellGetStatusCronTime = time.Hour // one run, at the start
	cfg.IrancellGetStatusChunk = chunk
	cfg.IrancellGetStatusStart = start
	cfg.KafkaBrokers = []string{w.kafka.BootstrapServers()}
	m := cronMetrics{
		onRun: func(_, result string) { w.mu.Lock(); defer w.mu.Unlock(); w.runs[result]++ },
		onOrder: func(name, result string) {
			w.mu.Lock()
			defer w.mu.Unlock()
			assert.Equal(t, "irancell-get-status", name)
			w.results[result]++
		},
		onRunDone: func(name string, fixed int) { w.mu.Lock(); defer w.mu.Unlock(); w.fixed = append(w.fixed, fixed) },
	}
	cache := redis.New(addr, password, prefix)
	t.Cleanup(cache.Close)
	jobs, err := newCrons(cfg, w.db, cache, namedAdapter{irancellproxy.Name, adapter}, m, w.log)
	if !assert.NoError(t, err) || !assert.NotNil(t, jobs) || !assert.NoError(t, jobs.Start(ctx)) {
		return false
	}
	defer func() { assert.NoError(t, jobs.Stop(context.Background())) }()
	return waitUntil(t, 20*time.Second, func() bool { w.mu.Lock(); defer w.mu.Unlock(); return w.runs[cron.ResultDone] == 1 }, "the run is done")
}

// messages reads the Kafka messages of paymentwriter (key and value).
func (w *liveIrancellWorker) messages(t *testing.T, want int) [][2]string {
	t.Helper()
	c, err := ckafka.NewConsumer(&ckafka.ConfigMap{"bootstrap.servers": w.kafka.BootstrapServers(), "group.id": "qabz-live-irancell", "enable.auto.commit": false})
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return nil
	}
	defer func() { _ = c.Close() }()
	topic := "paymentwriter"
	if !assert.NoError(t, c.Assign([]ckafka.TopicPartition{{Topic: &topic, Partition: 0, Offset: ckafka.OffsetBeginning}})) {
		return nil
	}
	var out [][2]string
	deadline := time.Now().Add(10 * time.Second)
	for len(out) < want && time.Now().Before(deadline) {
		msg, err := c.ReadMessage(time.Second)
		if err == nil && msg != nil {
			out = append(out, [2]string{string(msg.Key), string(msg.Value)})
		}
	}
	return out
}

// TestLiveIrancellStatusWorker (task 050): the Irancell worker of the App on the real Postgres,
// Redis and a mock Kafka, with the real adapter over gRPC. Six pending orders, chunk 2, start 0
// (the final rows of the run are inside the window, so the orders that the run made final leave the
// list: bill's OFFSET pages skip orders 3 and 4 here, task 050 C1 (a), D4). Every order of the window
// is asked once with the purchase type charge; success and failed are saved and reported with bill's
// bytes; pending (a text) and a gRPC error change nothing; the gRPC error writes one error line; the
// metric of fixed statuses gets 4 for the run.
func TestLiveIrancellStatusWorker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	w := newLiveIrancellWorker(t, ctx, []time.Duration{6 * time.Hour, 5 * time.Hour, 4 * time.Hour, 3 * time.Hour, 2 * time.Hour, time.Hour})
	want := map[int]string{1: order.StatusSuccess, 2: order.StatusFailed, 3: order.StatusSuccess, 4: order.StatusFailed}
	for n, st := range want {
		w.srv.statuses[irancellOrderID(n)] = st
	}
	w.srv.statuses[irancellOrderID(5)] = order.StatusPending
	w.srv.fail[irancellOrderID(6)] = true
	if !w.run(t, ctx, 2, 0) {
		return
	}

	w.srv.mu.Lock()
	asked := map[string]int{}
	for _, r := range w.srv.reqs {
		asked[r.GetOrderId()]++
		assert.Equal(t, "charge", r.GetPurchaseType())
	}
	w.srv.mu.Unlock()
	for n := 1; n <= 6; n++ {
		assert.Equal(t, 1, asked[irancellOrderID(n)], "order %d is asked once in the run (no skip, no repeat)", n)
	}
	for n := 1; n <= 6; n++ {
		got, err := w.repo.Latest(ctx, irancellOrderID(n), order.LatestWindow)
		if !assert.NoError(t, err, n) {
			continue
		}
		if st, ok := want[n]; ok {
			assert.Equal(t, st, got.Status, n)
		} else {
			assert.Equal(t, order.StatusPending, got.Status, "order %d: nothing changes", n)
		}
	}

	w.mu.Lock()
	assert.Equal(t, map[string]int{reconcile.ResultSuccess: 2, reconcile.ResultFailed: 2, reconcile.ResultPending: 2}, w.results)
	assert.Equal(t, []int{4}, w.fixed, "one observation for the run: 4 fixed")
	w.mu.Unlock()
	assert.Equal(t, 1, w.log.count("error", "irancell status: call failed"), "the gRPC error")
	assert.Equal(t, 4, w.log.count("info", "irancell status: reported"))

	msgs := w.messages(t, 4)
	var wantMsgs [][2]string
	for n, st := range want {
		v, err := reconcile.StatusMessage(irancellOrderID(n), st)
		if assert.NoError(t, err) {
			wantMsgs = append(wantMsgs, [2]string{"bill", string(v)})
		}
	}
	assert.ElementsMatch(t, wantMsgs, msgs, "the key bill and bill's value, one message for each final status")
}

// TestLiveIrancellStatusEmptyRun (task 050 D8): a run with no pending Irancell order observes 0.
func TestLiveIrancellStatusEmptyRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	w := newLiveIrancellWorker(t, ctx, nil)
	if !w.run(t, ctx, 2, 30*time.Minute) {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	assert.Equal(t, []int{0}, w.fixed)
	assert.Empty(t, w.results)
}

// TestLiveIrancellStatusSpans (conventions § 17; task 050 D10): the tick of the Irancell worker is a
// root span "cron irancell-get-status"; the check of the order is its child; the gRPC client span of
// GetStatus, kafka.Publish and the Postgres spans are children of the check. No span has the phone.
func TestLiveIrancellStatusSpans(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	w := newLiveIrancellWorker(t, ctx, []time.Duration{time.Hour})
	w.srv.statuses[irancellOrderID(1)] = order.StatusSuccess
	sr := jaegertest.Install(t)
	if !w.run(t, ctx, 100, 30*time.Minute) {
		return
	}
	roots := jaegertest.ByName(sr, "cron irancell-get-status")
	checks := jaegertest.ByName(sr, "reconcile.StatusWorker.check")
	if !assert.Len(t, roots, 1) || !assert.Len(t, checks, 1) {
		return
	}
	assert.False(t, roots[0].Parent().IsValid(), "a root span")
	assert.Equal(t, roots[0].SpanContext().SpanID(), checks[0].Parent().SpanID())
	ops := childNames(sr.Ended(), checks[0])
	hasGRPC := false
	for _, op := range ops {
		if strings.HasSuffix(op, "InternalService/GetStatus") {
			hasGRPC = true
		}
	}
	assert.True(t, hasGRPC, "the GetStatus client span: %v", ops)
	assert.Contains(t, ops, "kafka.Publish")
	assert.Contains(t, ops, "SELECT", "the newest row and the lock")
	assert.Contains(t, ops, "INSERT", "the final row")
	jaegertest.AssertClean(t, sr.Ended(), "989350000501")
}
