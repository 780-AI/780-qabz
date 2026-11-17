package app

import (
	"context"
	"sync"
	"testing"
	"time"

	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/config"
	"qabz/core/order"
	"qabz/core/reconcile"
	"qabz/entry/cron"
	"qabz/provider"
	"qabz/provider/parsianproxy"
	"qabz/system/postgres"
	"qabz/system/postgres/postgrestest"
	"qabz/system/redis"
)

// A valid water bill ID (type 1) and pay ID (25 000 000 rials; the check digit of bill), so the
// water order takes the raw inquiry (task 048).
const (
	liveWaterBill = "1234567890117"
	liveWaterPay  = "2500012310"
)

// newLiveWaterOrder is the live stack with the Parsian mock on (task 048): the payment answer pay.
// The seed has the Parsian water order row 45 enabled.
func newLiveWaterOrder(t *testing.T, ctx context.Context, pay string) *liveWaterStack {
	t.Helper()
	return newLiveWaterCfg(t, ctx, config.MockReturnSuccess, "", func(c *config.Config) {
		c.ParsianEnabled = true
		c.DebugMockParsianEnabled = true
		c.DebugMockParsianReturnType = config.MockReturnSuccess
		c.DebugMockParsianPayReturnType = pay
	})
}

func (s *liveWaterStack) waterOrder(t *testing.T, billID, payID string) (*pb.NewOrderResponse, error) {
	t.Helper()
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: liveUserOne, Mobile: "989121234567"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.water.NewOrder(ctx, &pb.NewOrderBillPayRequest{BillId: billID, PayId: payID, PaymentType: pb.NewOrderBillPayRequest_direct})
}

// TestLiveWaterOrderAndPayment is the feature-ON integration test of task 048: a water order (the
// raw inquiry: 25 000 000 rials, the default fee 9 000, no provider call), the token data of bill,
// the payment (the Parsian mock: success), the rows initial, in-progress and success, no
// settlement. A second call answers pending with no new row; a new order of the same bill is
// "already paid".
func TestLiveWaterOrderAndPayment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterOrder(t, ctx, config.MockReturnSuccess)
	res, err := s.waterOrder(t, liveWaterBill, liveWaterPay)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(25_000_000+9000), res.GetPrice())
	assert.Equal(t, uint32(105), res.GetServiceId())
	orderID := s.orderIDOf(t, res.GetOrderId())
	_, payload := s.tokenParts(t, res.GetOrderId())
	data, _ := payload["data"].(map[string]any)
	assert.Equal(t, map[string]any{
		"payId": liveWaterPay, "type": "water", "billId": liveWaterBill, "payAmount": "25009000", "rawAmount": "25000000",
		"about": "قبض آب", "aboutIcon": "https://public.780.ir/dl/Operator/AbvaFazelab.svg",
		"invoicePayID": liveWaterPay, "invoiceBillID": liveWaterBill, "receiptPayID": liveWaterPay, "receiptBillID": liveWaterBill,
		"feeAmount": "9000", "feeDescription": "کارمزد",
	}, data)
	assert.Equal(t, "parsian", payload["provider"])
	assert.Zero(t, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "the raw inquiry: no provider call")
	assert.Zero(t, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "no request row")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND status = 'initial' AND bill_type = 'water'
		AND bill_id = $2 AND pay_id = $3 AND data = $2 AND provider_id = 45 AND provider_name = 'parsian' AND base_price = 25000000
		AND fee = 9000 AND bill_inquiry_id IS NULL`, orderID, liveWaterBill, liveWaterPay))

	o, err := s.complete(t, orderID, res.GetPrice())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, order.StatusSuccess, o.Status)
	st, confirmed := s.statuses(t, orderID)
	assert.Equal(t, []string{"initial", "in-progress", "success"}, st)
	assert.False(t, confirmed, "no settlement for water")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND rrn = '778899' AND status = 'success' AND ref_id IS NOT NULL`, orderID))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_total", map[string]string{"bill_type": "water", "provider": "parsian", "status": "success"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": "water", "provider": "parsian", "result": "created"}))
	assert.Zero(t, s.log.count("info", "provider mock: settlement answered with no call to the provider"), "no settlement")
	assert.Zero(t, s.log.count("error", "settle: mark paid failed"))
	assert.Zero(t, s.log.count("error", "settle: set the last order date failed"))

	// A second call for the same order: pending, no new row.
	o, err = s.payments.Complete(ctx, order.CompleteRequest{OrderID: orderID, RRN: "778899", Price: res.GetPrice()})
	assert.NoError(t, err)
	assert.Equal(t, order.Order{Status: order.StatusPending, Message: order.MessageProcessed}, o)
	st, _ = s.statuses(t, orderID)
	assert.Len(t, st, 3)

	// The bill is paid: a new order with its bill ID and pay ID is refused.
	_, err = s.waterOrder(t, liveWaterBill, liveWaterPay)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": "water", "provider": "parsian", "result": "already_paid"}))
}

// TestLiveWaterPaymentFailedAndPending: the Parsian mock at failed (the failed row; a second call
// answers failed with the message of that row, no new row: task 048 D11 (a)) and at pending (the
// pending row; a second call answers pending).
func TestLiveWaterPaymentFailedAndPending(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	for pay, want := range map[string]string{config.MockReturnFailed: order.StatusFailed, config.MockReturnPending: order.StatusPending} {
		s := newLiveWaterOrder(t, ctx, pay)
		res, err := s.waterOrder(t, liveWaterBill, liveWaterPay)
		if !assert.NoError(t, err, pay) {
			continue
		}
		orderID := s.orderIDOf(t, res.GetOrderId())
		o, err := s.complete(t, orderID, res.GetPrice())
		assert.NoError(t, err, pay)
		assert.Equal(t, want, o.Status, pay)
		st, _ := s.statuses(t, orderID)
		assert.Equal(t, []string{"initial", "in-progress", want}, st, pay)

		o, err = s.payments.Complete(ctx, order.CompleteRequest{OrderID: orderID, RRN: "778899", Price: res.GetPrice()})
		assert.NoError(t, err, pay)
		if want == order.StatusFailed {
			assert.Equal(t, order.StatusFailed, o.Status, "a failed order answers failed")
			assert.Equal(t, provider.MockPayFailedMessage, o.Message)
		} else {
			assert.Equal(t, order.Order{Status: order.StatusPending, Message: order.MessageProcessed}, o, pay)
		}
		st, _ = s.statuses(t, orderID)
		assert.Len(t, st, 3, pay+": no new row")
		assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_total", map[string]string{"bill_type": "water", "provider": "parsian", "status": "processed"}), pay)
	}
}

// statusByOrder answers the status of each order from a map (pending for an unknown order).
type statusByOrder struct {
	provider.ProviderInterface
	mu       sync.Mutex
	statuses map[string]string
	reqs     []provider.StatusRequest
}

func (s *statusByOrder) Status(_ context.Context, req provider.StatusRequest) (*provider.StatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, req)
	st, ok := s.statuses[req.OrderID]
	if !ok {
		st = provider.StatusPending
	}
	return &provider.StatusResponse{Status: st, ProviderTraceID: "9900"}, nil
}

// TestLiveWaterStatusWorker (task 048 D13): the Parsian status worker finalizes two pending water
// orders (liveStatusWorker).
func TestLiveWaterStatusWorker(t *testing.T) {
	liveStatusWorker(t, provider.BillTypeWater, 45, liveWaterBill, liveWaterPay, "48")
}

// liveStatusWorker: the Parsian status worker finalizes two pending orders of billType (Parsian
// answers success and failed): one final row each and one Kafka message each with bill's value on
// paymentwriter; the request to Parsian has the bill type. A later OrderComplete of the failed order
// answers failed. providerID is the Parsian order row of the bill type in the seed; tag (two digits)
// makes the order IDs of each caller different. Water (task 048 D13) and electricity (task 052 D11)
// use it.
func liveStatusWorker(t *testing.T, billType string, providerID int64, billID, payID, tag string) {
	t.Helper()
	liveProviderStatusWorker(t, parsianproxy.Name, func(cfg *config.Config) {
		cfg.ParsianGetStatusEnabled = true
		cfg.ParsianGetStatusCronTime = time.Hour // one run, at the start
	}, billType, providerID, billID, payID, tag)
}

// liveProviderStatusWorker is liveStatusWorker for the worker of providerName that on turns on
// (task 058: the Jiring Switch worker too).
func liveProviderStatusWorker(t *testing.T, providerName string, on func(*config.Config), billType string, providerID int64, billID, payID, tag string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	addr, password, prefix := liveRedisOf(t)
	db, err := postgres.New(ctx, postgrestest.SeededSchema(t, ctx, postgrestest.RunDSN(t)))
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	t.Cleanup(db.Close)

	okID, failID := "cccccccc-0000-4000-8000-000000000"+tag+"1", "cccccccc-0000-4000-8000-000000000"+tag+"2"
	repo := postgres.NewOrderRepository(db)
	for i, id := range []string{okID, failID} {
		o := order.Order{ID: "dddddddd-0000-4000-8000-000000000" + tag + []string{"1", "2"}[i], OrderID: id,
			BillType: billType, BillID: billID, PayID: payID, Data: billID, Price: 25_009_000,
			ProviderID: providerID, ProviderName: providerName, Status: order.StatusPending, RRN: "778899", BasePrice: 25_000_000}
		if !assert.NoError(t, repo.Insert(ctx, o)) {
			return
		}
	}
	if _, err := db.Pool().Exec(ctx, `UPDATE orders SET created_at = NOW() - INTERVAL '1 hour'`); !assert.NoError(t, err) {
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
	on(&cfg)
	cfg.KafkaBrokers = []string{mc.BootstrapServers()}
	adapter := &statusByOrder{statuses: map[string]string{okID: provider.StatusSuccess, failID: provider.StatusFailed}}

	var mu sync.Mutex
	results := map[string]int{}
	m := cronMetrics{onRun: func(_, result string) {
		mu.Lock()
		defer mu.Unlock()
		results[result]++
	}}
	cache := redis.New(addr, password, prefix)
	t.Cleanup(cache.Close)
	jobs, err := newCrons(cfg, db, cache, namedAdapter{name: providerName, a: adapter}, m, &liveLogger{})
	if !assert.NoError(t, err) || !assert.NotNil(t, jobs) || !assert.NoError(t, jobs.Start(ctx)) {
		return
	}
	defer func() { assert.NoError(t, jobs.Stop(context.Background())) }()
	if !waitUntil(t, 15*time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return results[cron.ResultDone] == 1 }, "the run is done") {
		return
	}

	for id, want := range map[string]string{okID: order.StatusSuccess, failID: order.StatusFailed} {
		got, err := repo.Latest(ctx, id, order.LatestWindow)
		if assert.NoError(t, err, id) {
			assert.Equal(t, want, got.Status, id)
			assert.Equal(t, billType, got.BillType, id)
		}
	}
	adapter.mu.Lock()
	assert.ElementsMatch(t, []provider.StatusRequest{{OrderID: okID, BillType: billType}, {OrderID: failID, BillType: billType}}, adapter.reqs)
	adapter.mu.Unlock()

	c, err := ckafka.NewConsumer(&ckafka.ConfigMap{"bootstrap.servers": mc.BootstrapServers(), "group.id": "qabz-live-" + providerName + "-" + billType, "enable.auto.commit": false})
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	defer func() { _ = c.Close() }()
	topic := "paymentwriter"
	if !assert.NoError(t, c.Assign([]ckafka.TopicPartition{{Topic: &topic, Partition: 0, Offset: ckafka.OffsetBeginning}})) {
		return
	}
	okMsg, err := reconcile.StatusMessage(okID, order.StatusSuccess)
	if !assert.NoError(t, err) {
		return
	}
	failMsg, err := reconcile.StatusMessage(failID, order.StatusFailed)
	if !assert.NoError(t, err) {
		return
	}
	var got []string
	for range 2 {
		msg, err := c.ReadMessage(10 * time.Second)
		if assert.NoError(t, err) && assert.NotNil(t, msg) {
			assert.Equal(t, "bill", string(msg.Key))
			got = append(got, string(msg.Value))
		}
	}
	assert.ElementsMatch(t, []string{string(okMsg), string(failMsg)}, got)
	_, err = c.ReadMessage(500 * time.Millisecond)
	assert.Error(t, err, "two messages only")

	// The worker saved the failed row: OrderComplete now answers failed (task 048 D11), no new row.
	payments, err := newPayments(db, oneAdapter{adapter}, nil, nil, nil)
	if !assert.NoError(t, err) {
		return
	}
	o, err := payments.Complete(ctx, order.CompleteRequest{OrderID: failID, RRN: "778899", Price: 25_009_000})
	if assert.NoError(t, err) {
		assert.Equal(t, order.StatusFailed, o.Status)
	}
	var n int
	assert.NoError(t, db.Pool().QueryRow(ctx, `SELECT count(*) FROM orders WHERE order_id = $1`, failID).Scan(&n))
	assert.Equal(t, 2, n, "pending and failed only")
}

// TestLiveWaterSpans (conventions § 17): the span tree of the water order and of its payment on the
// real Postgres: order.Service.Create is a child of the request span and the parent of the Postgres
// spans of the order (the already-paid read, the fee reads, the insert); order.Payments.Complete is
// a child of its request span and the parent of the Postgres spans of the payment. No span has the
// bill ID, the pay ID, the RRN or the user.
func TestLiveWaterSpans(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterOrder(t, ctx, config.MockReturnSuccess)
	sr := jaegertest.Install(t)

	octx := auth.WithClaims(context.Background(), auth.Claims{UserID: liveUserOne, Mobile: "989121234567"})
	octx, request := otel.Tracer("test").Start(octx, "request order")
	res, err := s.water.NewOrder(octx, &pb.NewOrderBillPayRequest{BillId: liveWaterBill, PayId: liveWaterPay})
	request.End()
	if !assert.NoError(t, err) {
		return
	}
	creates := jaegertest.ByName(sr, "order.Service.Create")
	if !assert.Len(t, creates, 1) {
		return
	}
	assert.Equal(t, request.SpanContext().SpanID(), creates[0].Parent().SpanID())
	ops := childNames(sr.Ended(), creates[0])
	assert.Contains(t, ops, "SELECT", "the already-paid read and the fee reads")
	assert.Contains(t, ops, "INSERT", "the orders row")

	cctx, request2 := otel.Tracer("test").Start(context.Background(), "request complete")
	o, err := s.payments.Complete(cctx, order.CompleteRequest{OrderID: s.orderIDOf(t, res.GetOrderId()), RRN: "778899", Price: res.GetPrice()})
	request2.End()
	assert.NoError(t, err)
	assert.Equal(t, order.StatusSuccess, o.Status)
	stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	assert.NoError(t, s.payments.Stop(stopCtx))
	completes := jaegertest.ByName(sr, "order.Payments.Complete")
	if assert.Len(t, completes, 1) {
		assert.Equal(t, request2.SpanContext().SpanID(), completes[0].Parent().SpanID())
		ops := childNames(sr.Ended(), completes[0])
		assert.Contains(t, ops, "SELECT", "the order and the provider reads")
		assert.Contains(t, ops, "INSERT", "the in-progress and the success rows")
	}
	jaegertest.AssertClean(t, sr.Ended(), liveWaterBill, liveWaterPay, "778899", liveUserOne)
}

// childNames gives the names of the direct children of parent.
func childNames(spans []sdktrace.ReadOnlySpan, parent sdktrace.ReadOnlySpan) []string {
	var out []string
	for _, sp := range spans {
		if sp.Parent().SpanID() == parent.SpanContext().SpanID() {
			out = append(out, sp.Name())
		}
	}
	return out
}
