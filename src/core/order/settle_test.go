package order

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"qabz/provider"
)

// recordSpans makes the global tracer provider a recorder for the test.
func recordSpans(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return sr
}

// allowedSettleKeys are the only span attributes of a settlement (conventions § 17: no phone, no
// bill ID, no pay ID, no amount, no user ID).
var allowedSettleKeys = map[attribute.Key]bool{"order.id": true, "bill_type": true, "provider": true, "result": true, "source": true}

type settleWorld struct {
	repo     *payRepo
	karashop *payAdapter
	log      *payLog
	counted  []string
	s        *Settler
}

func newSettleWorld(t *testing.T) *settleWorld {
	t.Helper()
	w := &settleWorld{
		repo:     &payRepo{},
		karashop: &payAdapter{name: "karashop", settle: &provider.SettleResponse{Status: provider.StatusSuccess, Code: 200}},
		log:      &payLog{},
	}
	s, err := NewSettler(SettlerOptions{
		Repository: w.repo, Adapters: adapters{"karashop": w.karashop},
		Routes:   map[string]map[string]string{provider.BillTypeTel: {"parsian": "karashop"}},
		Source:   SettleSourceWorker,
		Logger:   w.log,
		OnSettle: func(bt, pro, r string) { w.counted = append(w.counted, bt+"/"+pro+"/"+r) },
	})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		t.FailNow()
	}
	w.s = s
	return w
}

func paidOrder() Order {
	return Order{ID: "row-9", OrderID: "order-9", UserID: testUser, BillType: provider.BillTypeTel, BillID: "2101234560040",
		PayID: "12000035", Data: testTel, ProviderName: "parsian", Status: StatusSuccess, BasePrice: 120000, RRN: "778899", CreatedAt: paidAt}
}

// TestSettlerSuccess: the request has the order fields (bill: ConfirmPayment), provider_confirmed is
// saved, one count and one info line; one span with the allowed attributes and no error status.
func TestSettlerSuccess(t *testing.T) {
	sr := recordSpans(t)
	w := newSettleWorld(t)
	assert.Equal(t, provider.StatusSuccess, w.s.Settle(context.Background(), paidOrder()))
	if assert.Len(t, w.karashop.settles, 1) {
		assert.Equal(t, provider.SettleRequest{BillType: provider.BillTypeTel, TelephoneNumber: testTel, Amount: 120000,
			BillID: "2101234560040", PayID: "12000035", RRN: "778899", PaidAt: paidAt, PaymentProvider: "parsian"}, w.karashop.settles[0])
	}
	assert.Equal(t, []string{"order-9"}, w.repo.confirmed)
	assert.Equal(t, []string{"tel/karashop/success"}, w.counted)
	assert.Equal(t, []string{"info: settle: karashop confirmed"}, w.log.lines)

	spans := sr.Ended()
	if assert.Len(t, spans, 1) {
		assert.Equal(t, settleSpan, spans[0].Name())
		assert.Equal(t, codes.Unset, spans[0].Status().Code)
		assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("order.id", "order-9"), attribute.String("bill_type", "tel"),
			attribute.String("provider", "karashop"), attribute.String("result", "success"), attribute.String("source", "worker")}, spans[0].Attributes())
	}
}

// TestSettlerResults: every result but success leaves provider_confirmed unset, is counted once,
// and its span has the status Error and only allowed attributes; a child of the span in ctx.
func TestSettlerResults(t *testing.T) {
	cases := []struct {
		name   string
		change func(*settleWorld, *Order)
		result string
		calls  int
		lines  int
	}{
		{"no route", func(_ *settleWorld, o *Order) { o.ProviderName = "other" }, SettleNoRoute, 0, 0},
		{"no data", func(_ *settleWorld, o *Order) { o.Data = "" }, settleNoInquiryID, 0, 0},
		{"no adapter", func(w *settleWorld, _ *Order) { w.s.adapters = adapters{} }, SettleError, 0, 1},
		{"call error", func(w *settleWorld, _ *Order) { w.karashop.settle, w.karashop.settleErr = nil, errors.New("down") }, SettleError, 1, 1},
		{"nil answer", func(w *settleWorld, _ *Order) { w.karashop.settle = nil }, SettleError, 1, 1},
		{"failed", func(w *settleWorld, _ *Order) {
			w.karashop.settle = &provider.SettleResponse{Status: provider.StatusFailed, Code: 400}
		}, provider.StatusFailed, 1, 1},
		{"pending", func(w *settleWorld, _ *Order) {
			w.karashop.settle = &provider.SettleResponse{Status: provider.StatusPending, Code: 500}
		}, provider.StatusPending, 1, 1},
		{"confirm fails", func(w *settleWorld, _ *Order) { w.repo.confErr = errors.New("db") }, SettleMarkFailed, 1, 1},
	}
	for _, c := range cases {
		sr := recordSpans(t)
		w := newSettleWorld(t)
		o := paidOrder()
		c.change(w, &o)
		ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
		assert.Equal(t, c.result, w.s.Settle(ctx, o), c.name)
		parent.End()
		assert.Len(t, w.karashop.settles, c.calls, c.name)
		if c.result != SettleMarkFailed {
			assert.Empty(t, w.repo.confirmed, c.name)
		}
		assert.Len(t, w.counted, 1, c.name)
		assert.Len(t, w.log.lines, c.lines, c.name)
		spans := sr.Ended()
		if !assert.Len(t, spans, 2, c.name) {
			continue
		}
		span := spans[0]
		assert.Equal(t, settleSpan, span.Name(), c.name)
		assert.Equal(t, parent.SpanContext().SpanID(), span.Parent().SpanID(), c.name)
		assert.Equal(t, codes.Error, span.Status().Code, c.name)
		for _, kv := range span.Attributes() {
			assert.True(t, allowedSettleKeys[kv.Key], "%s: attribute %s is not allowed", c.name, kv.Key)
		}
		assert.Contains(t, span.Attributes(), attribute.String("result", c.result), c.name)
	}
}

// TestNewSettlerRejects: the repository, the adapters and a known source are required; a nil
// Settler gives error and no route.
func TestNewSettlerRejects(t *testing.T) {
	_, err := NewSettler(SettlerOptions{Adapters: adapters{}, Source: SettleSourceWorker})
	assert.Error(t, err)
	_, err = NewSettler(SettlerOptions{Repository: &payRepo{}, Source: SettleSourceWorker})
	assert.Error(t, err)
	_, err = NewSettler(SettlerOptions{Repository: &payRepo{}, Adapters: adapters{}, Source: "x"})
	assert.Error(t, err)
	var nilS *Settler
	assert.Equal(t, SettleError, nilS.Settle(context.Background(), paidOrder()))
	_, ok := nilS.Route(provider.BillTypeTel, "parsian")
	assert.False(t, ok)
}
