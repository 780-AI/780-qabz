package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	ckafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"

	pb "qabz/api/pb/irancell"
	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/common/monitoring/prometheus"
	"qabz/config"
	"qabz/core/order"
	"qabz/entry/cron"
	"qabz/provider"
	"qabz/provider/irancellproxy"
	"qabz/system/nats"
	"qabz/system/postgres"
	"qabz/system/redis"
)

// liveNATSURL is the NATS server of the live tests (QABZ_TEST_NATS_URL, else the compose service
// nats); the test skips when it does not answer.
func liveNATSURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("QABZ_TEST_NATS_URL")
	if u == "" {
		u = "nats://127.0.0.1:4222"
	}
	c, err := net.DialTimeout("tcp", strings.TrimPrefix(u, "nats://"), 500*time.Millisecond)
	if err != nil {
		t.Skipf("no NATS server at %s (COMPOSE_PROFILES=nats make compose up): %v", u, err)
	}
	_ = c.Close()
	return u
}

// irancellResponder is a fake irancell service on NATS: it reads the subject in the queue group
// irancell-bill, as the real one (app/application_dto.go:453), records each message and answers.
type irancellResponder struct {
	mu   sync.Mutex
	msgs []string
}

func newIrancellResponder(t *testing.T, url, subject, answer string) *irancellResponder {
	t.Helper()
	r := &irancellResponder{}
	nc, err := natsgo.Connect(url)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	t.Cleanup(nc.Close)
	_, err = nc.QueueSubscribe(subject, "irancell-bill", func(m *natsgo.Msg) {
		r.mu.Lock()
		r.msgs = append(r.msgs, string(m.Data))
		r.mu.Unlock()
		_ = m.Respond([]byte(answer))
	})
	if !assert.NoError(t, err) || !assert.NoError(t, nc.Flush()) {
		t.FailNow()
	}
	return r
}

func (r *irancellResponder) messages() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.msgs...)
}

// liveIrancellNATS is an order stack with the NATS client of the App (newNATS, its own metrics) on
// the subject of the test; the Irancell mock is off.
type liveIrancellNATS struct {
	*liveWaterStack
	natsMetrics *prometheus.Server
	subject     string
}

func newLiveIrancellNATS(t *testing.T, ctx context.Context, url string) *liveIrancellNATS {
	t.Helper()
	subject := fmt.Sprintf("qabz-test-irancell-bill-%d", time.Now().UnixNano())
	natsMetrics := prometheus.New("127.0.0.1:0", true)
	cfg := testConfig(t)
	cfg.IrancellEnabled, cfg.IrancellNATSSubject, cfg.NATSEndpoints, cfg.IrancellPayTimeout = true, subject, []string{url}, 2*time.Second
	client, err := newNATS(cfg, natsMetrics, &liveLogger{})
	if !assert.NoError(t, err) || !assert.NotNil(t, client) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	s := newLiveStack(t, ctx, config.MockReturnSuccess, "", func(c *config.Config) {
		c.DebugMockIrancellEnabled = false // the payment goes over NATS
		c.IrancellNATSSubject, c.IrancellPayTimeout = subject, 2*time.Second
	}, client)
	return &liveIrancellNATS{liveWaterStack: s, natsMetrics: natsMetrics, subject: subject}
}

// irancellOrder makes an Irancell order with a direct price (450 000 + the seed fee 9 000).
func (s *liveWaterStack) irancellOrder(t *testing.T) string {
	t.Helper()
	res, err := s.order(liveUserOne, irancellOrderReq("09350000391", 450000))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Equal(t, uint32(459000), res.GetPrice()) {
		t.FailNow()
	}
	return s.orderIDOf(t, res.GetOrderId())
}

// irancellSuccess is the answer of the irancell service to a paid bill (entity.ChargeResponse).
const irancellSuccess = `{"code":200,"message":"عملیات با موفقیت انجام شد","tid":"1100778899","status":"success"}`

// TestLiveIrancellPaymentNATS is the feature-ON integration test of task 056: an Irancell order is
// paid over the compose NATS through a fake irancell service. The message on the wire is bill's; the
// answer gives the status (success, failed; an answer with no status and no responder are pending);
// ref_id stays empty (Q1 (a)); the span nats.Request is in the trace of the payment; the metrics
// count each request.
func TestLiveIrancellPaymentNATS(t *testing.T) {
	url := liveNATSURL(t)
	for _, tc := range []struct {
		name, answer string
		responder    bool
		want         string
		code         int
		result       string
	}{
		{"success", irancellSuccess, true, order.StatusSuccess, 200, nats.ResultOK},
		{"failed", `{"code":400,"message":"insufficient balance","tid":"1100778899","status":"failed"}`, true, order.StatusFailed, 400, nats.ResultOK},
		{"bad phone: no status", `{"code":400,"message":"invalid phone"}`, true, order.StatusPending, 400, nats.ResultOK},
		{"no responder", "", false, order.StatusPending, 500, nats.ResultNoResponders},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			s := newLiveIrancellNATS(t, ctx, url)
			var r *irancellResponder
			if tc.responder {
				r = newIrancellResponder(t, url, s.subject, tc.answer)
			}
			sr := jaegertest.Install(t)
			orderID := s.irancellOrder(t)
			o, err := s.complete(t, orderID, 459000)
			assert.NoError(t, err)
			assert.Equal(t, tc.want, o.Status)
			st, _ := s.statuses(t, orderID)
			assert.Equal(t, []string{"initial", "in-progress", tc.want}, st)
			var refID, msg string
			var code int
			if assert.NoError(t, s.db.Pool().QueryRow(ctx, `SELECT COALESCE(ref_id, ''), COALESCE(message, ''), COALESCE(status_code, 0) FROM orders
				WHERE order_id = $1 AND status = $2`, orderID, tc.want).Scan(&refID, &msg, &code)) {
				assert.Empty(t, refID, "the tid is not kept (bill)")
				assert.Equal(t, tc.code, code)
				if !tc.responder {
					assert.Contains(t, msg, "no responders")
				}
			}
			if r != nil && assert.Len(t, r.messages(), 1) {
				assert.Equal(t, `{"inquiry_id":"989350000391","bill_id":"","pay_id":"irancell","order_id":"`+orderID+`","user_id":"`+liveUserOne+
					`","dialer":"","rrn":"778899","amount":450000,"option":"irancell","type":"irancell","InquiryType":"final_term"}`, r.messages()[0])
			}
			completes, requests := jaegertest.ByName(sr, "order.Payments.Complete"), jaegertest.ByName(sr, "nats.Request")
			if assert.Len(t, completes, 1) && assert.Len(t, requests, 1) {
				assert.Equal(t, completes[0].SpanContext().TraceID(), requests[0].SpanContext().TraceID())
				assert.Contains(t, requests[0].Attributes(), attribute.String("messaging.destination.name", s.subject))
				assert.Contains(t, requests[0].Attributes(), attribute.String("result", tc.result))
			}
			v, _ := metricValue(t, s.natsMetrics.Registry(), "qabz_nats_requests_total", map[string]string{"subject": s.subject, "result": tc.result})
			assert.Equal(t, 1.0, v)
			assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_total", map[string]string{"bill_type": "irancell", "provider": "irancell", "status": tc.want}))
		})
	}
}

// TestLiveIrancellPaymentMock (conventions § 15): with the Irancell mock, the payment is answered by
// debug.mocks.irancell.pay_return_type with no NATS client.
func TestLiveIrancellPaymentMock(t *testing.T) {
	for pay, want := range map[string]string{config.MockReturnSuccess: order.StatusSuccess, config.MockReturnFailed: order.StatusFailed, config.MockReturnPending: order.StatusPending} {
		t.Run(pay, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			s := newLiveWaterCfg(t, ctx, config.MockReturnSuccess, "", func(c *config.Config) { c.DebugMockIrancellPayReturnType = pay })
			orderID := s.irancellOrder(t)
			o, err := s.complete(t, orderID, 459000)
			assert.NoError(t, err)
			assert.Equal(t, want, o.Status)
			st, _ := s.statuses(t, orderID)
			assert.Equal(t, []string{"initial", "in-progress", want}, st)
			assert.Equal(t, 1, s.log.count("info", "provider mock: payment answered with no call to the provider"))
		})
	}
}

// TestLiveIrancellPendingFinalized (task 056 D12): an Irancell order that OrderComplete leaves
// pending (no responder on NATS) is finalized by the Irancell status worker of task 050 with no code
// change: GetStatus with the order ID answers success, the worker saves the success row.
func TestLiveIrancellPendingFinalized(t *testing.T) {
	url := liveNATSURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveIrancellNATS(t, ctx, url)
	orderID := s.irancellOrder(t)
	o, err := s.complete(t, orderID, 459000)
	if !assert.NoError(t, err) || !assert.Equal(t, order.StatusPending, o.Status) {
		return
	}
	if _, err := s.db.Pool().Exec(ctx, `UPDATE orders SET created_at = created_at - INTERVAL '1 hour' WHERE order_id = $1`, orderID); !assert.NoError(t, err) {
		return
	}

	srv := &irancellStatusServer{statuses: map[string]string{orderID: provider.StatusSuccess}, fail: map[string]bool{}}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	gs := grpc.NewServer()
	pb.RegisterInternalServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	client, err := irancellproxy.Dial(lis.Addr().String())
	if !assert.NoError(t, err) {
		return
	}
	adapter, err := irancellproxy.New(irancellproxy.Options{Enabled: true, Client: client, Timeout: 5 * time.Second})
	if !assert.NoError(t, err) {
		return
	}
	mc, err := ckafka.NewMockCluster(1)
	if !assert.NoError(t, err) || !assert.NotNil(t, mc) || !assert.NoError(t, mc.CreateTopic("paymentwriter", 1, 1)) {
		return
	}
	defer mc.Close()
	addr, password, prefix := liveRedisOf(t)
	cfg := testConfig(t)
	cfg.IrancellGetStatusEnabled = true
	cfg.IrancellGetStatusCronTime = time.Hour // one run, at the start
	cfg.KafkaBrokers = []string{mc.BootstrapServers()}
	var mu sync.Mutex
	runs := map[string]int{}
	m := cronMetrics{onRun: func(_, result string) { mu.Lock(); defer mu.Unlock(); runs[result]++ }}
	cache := redis.New(addr, password, prefix)
	t.Cleanup(cache.Close)
	jobs, err := newCrons(cfg, s.db, cache, namedAdapter{irancellproxy.Name, adapter}, m, &liveLogger{})
	if !assert.NoError(t, err) || !assert.NotNil(t, jobs) || !assert.NoError(t, jobs.Start(ctx)) {
		return
	}
	defer func() { assert.NoError(t, jobs.Stop(context.Background())) }()
	if !waitUntil(t, 20*time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return runs[cron.ResultDone] == 1 }, "the run is done") {
		return
	}
	got, err := postgres.NewOrderRepository(s.db).Latest(ctx, orderID, order.LatestWindow)
	if assert.NoError(t, err) {
		assert.Equal(t, order.StatusSuccess, got.Status)
	}
	srv.mu.Lock()
	if assert.Len(t, srv.reqs, 1) {
		assert.Equal(t, orderID, srv.reqs[0].GetOrderId())
	}
	srv.mu.Unlock()
}
