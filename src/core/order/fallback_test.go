package order

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/provider"
)

// groups is a fixed Groups.
type groups map[string][]provider.Provider

func (g groups) Group(billType, action string) []provider.Provider {
	return g[billType+"/"+action]
}

// errNotSent is the error of an adapter whose payment did not reach its provider for sure.
var errNotSent = provider.DownError("test: pay", errors.New("connection refused"), true)

// fallbackFixture is the payment step of an MCI mid-term order of the Parsian row 149 with the
// group (mobilemidterm, order): Parsian 149, Jiring Switch 29 and a third provider 900 (disabled),
// all with a payment route.
type fallbackFixture struct {
	*payFixture
	jiring    *payAdapter
	third     *payAdapter
	groups    groups
	fallbacks []string
}

func newFallbackFixture(t *testing.T, opts func(*fallbackFixture, *PaymentOptions)) *fallbackFixture {
	t.Helper()
	f := &fallbackFixture{payFixture: &payFixture{
		repo: &payRepo{latest: Order{
			ID: "row-1", OrderID: "order-1", UserID: testUser, BillType: provider.BillTypeMobileMidterm, BillID: "1234567809158", PayID: "45000080",
			Price: 459000, Data: "989121234567", ProviderID: 149, ProviderName: "parsian", Status: StatusInitial, Fee: 9000, BasePrice: 450000,
			Option: "mci", CreatedAt: paidAt,
		}},
		parsian: &payAdapter{name: "parsian", payErr: errNotSent},
		log:     &payLog{},
	}}
	f.jiring = &payAdapter{name: "jiring-switch", payResp: &provider.PaymentResponse{Status: "success", Code: 200, Message: "ok", ProviderTraceID: "8800000058"}}
	f.third = &payAdapter{name: "third", payResp: &provider.PaymentResponse{Status: "success", Code: 200, Message: "ok3", ProviderTraceID: "3"}}
	f.groups = groups{"mobilemidterm/order": {
		{ID: 149, Name: "parsian", Status: provider.StatusEnable},
		{ID: 29, Name: "jiring-switch", Status: provider.StatusEnable},
		{ID: 900, Name: "third", Status: provider.StatusDisable},
	}}
	route := map[string]bool{provider.BillTypeMobile: true, provider.BillTypeMobileMidterm: true}
	o := PaymentOptions{
		Repository: f.repo,
		Adapters:   adapters{"parsian": f.parsian, "jiring-switch": f.jiring, "third": f.third},
		PayRoutes:  map[string]map[string]bool{"parsian": route, "jiring-switch": route, "third": route},
		Providers:  f.groups,
		Logger:     f.log,
		OnPayment:  func(bt, pro, r string) { f.results = append(f.results, bt+"/"+pro+"/"+r) },
		OnPay:      func(pro, status string, _ time.Duration) { f.pays = append(f.pays, pro+"/"+status) },
		OnFallback: func(bt, from, to string) { f.fallbacks = append(f.fallbacks, bt+"/"+from+"/"+to) },
	}
	if opts != nil {
		opts(f, &o)
	}
	p, err := NewPayments(o)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	n := 0
	p.newID = func() string { n++; return fmt.Sprintf("new-%d", n) }
	p.spawn = func(job func()) { job() }
	f.p = p
	return f
}

func completeMCI() CompleteRequest {
	return CompleteRequest{OrderID: "order-1", RRN: "445577", Price: 459000}
}

// providersOf gives the provider ID and name of each saved row.
func providersOf(rows []Order) []string {
	var out []string
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%d/%s/%s", r.ProviderID, r.ProviderName, r.Status))
	}
	return out
}

// TestCompleteFallbackPays (task 058): a Parsian payment that was not sent goes to the next enabled
// row with a route (Jiring Switch 29): a new in-progress row with the same RRN and the Parsian error,
// then the success of Jiring Switch. The metrics, the line and the span show both providers.
func TestCompleteFallbackPays(t *testing.T) {
	sr := jaegertest.Install(t)
	f := newFallbackFixture(t, nil)
	o, err := f.p.Complete(context.Background(), completeMCI())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, StatusSuccess, o.Status)
	assert.Equal(t, "jiring-switch", o.ProviderName)
	assert.Equal(t, int64(29), o.ProviderID)
	assert.Equal(t, "8800000058", o.RefID)
	assert.Equal(t, []string{"149/parsian/in-progress", "29/jiring-switch/in-progress", "29/jiring-switch/success"}, providersOf(f.repo.inserts))
	for _, row := range f.repo.inserts {
		assert.Equal(t, "445577", row.RRN, "the same RRN on every row")
	}
	assert.Equal(t, errNotSent.Error(), f.repo.inserts[1].Message, "the error of the provider that was not reached")
	assert.Len(t, f.parsian.payReqs, 1)
	if assert.Len(t, f.jiring.payReqs, 1) && assert.Len(t, f.parsian.payReqs, 1) {
		assert.Equal(t, f.parsian.payReqs[0], f.jiring.payReqs[0], "the same payment request")
		assert.Equal(t, provider.InquiryTypeMidterm, f.jiring.payReqs[0].InquiryType)
	}
	assert.Empty(t, f.third.payReqs, "a disabled row is not used")
	assert.Equal(t, []string{"parsian/pending", "jiring-switch/success"}, f.pays)
	assert.Equal(t, []string{"mobilemidterm/parsian/jiring-switch"}, f.fallbacks)
	assert.Equal(t, []string{"mobilemidterm/jiring-switch/success"}, f.results)
	assert.Equal(t, []string{"info: order complete: payment not sent, next provider"}, f.log.lines)

	spans := jaegertest.ByName(sr, completeSpan)
	if assert.Len(t, spans, 1) {
		assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("order.id", "order-1"),
			attribute.String("bill_type", provider.BillTypeMobileMidterm), attribute.String("provider", "jiring-switch"),
			attribute.String("result", StatusSuccess), attribute.String("provider.first", "parsian")}, spans[0].Attributes())
		assert.Equal(t, codes.Unset, spans[0].Status().Code)
	}
	jaegertest.AssertClean(t, sr.Ended(), "989121234567", "1234567809158", "45000080", "445577", testUser)
}

// TestCompleteNoFallbackAfterTimeout (task 058, operator rule): a provider down error that is not
// ErrNotSent (a timeout) and every answer of the provider stay with the provider: no second call.
func TestCompleteNoFallbackAfterTimeout(t *testing.T) {
	sr := jaegertest.Install(t)
	for name, c := range map[string]struct {
		resp   *provider.PaymentResponse
		err    error
		status string
	}{
		"timeout": {err: fmt.Errorf("test: pay: %w: %w", provider.ErrProviderDown, context.DeadlineExceeded), status: StatusPending},
		"failed":  {resp: &provider.PaymentResponse{Status: "failed", Code: 400, Message: "no"}, status: StatusFailed},
		"pending": {resp: &provider.PaymentResponse{Status: "pending", Code: 500}, status: StatusPending},
		"other":   {err: errors.New("not supported"), status: StatusPending},
	} {
		f := newFallbackFixture(t, nil)
		f.parsian.payResp, f.parsian.payErr = c.resp, c.err
		o, err := f.p.Complete(context.Background(), completeMCI())
		if !assert.NoError(t, err, name) {
			continue
		}
		assert.Equal(t, c.status, o.Status, name)
		assert.Equal(t, "parsian", o.ProviderName, name)
		assert.Empty(t, f.jiring.payReqs, name)
		assert.Empty(t, f.fallbacks, name)
		assert.Equal(t, []string{"149/parsian/in-progress", "149/parsian/" + c.status}, providersOf(f.repo.inserts), name)
	}
	for _, s := range jaegertest.ByName(sr, completeSpan) {
		for _, kv := range s.Attributes() {
			assert.NotEqual(t, attribute.Key("provider.first"), kv.Key, "no provider.first with no fallback")
		}
	}
}

// TestCompleteNoNextProvider (task 058, operator): a payment that was not sent with no next provider
// is pending with the error of the call, as before task 058 (one provider, the error stays).
func TestCompleteNoNextProvider(t *testing.T) {
	cases := map[string]func(*fallbackFixture, *PaymentOptions){
		"no Providers": func(_ *fallbackFixture, o *PaymentOptions) { o.Providers = nil },
		"only one row": func(f *fallbackFixture, o *PaymentOptions) {
			o.Providers = groups{"mobilemidterm/order": f.groups["mobilemidterm/order"][:1]}
		},
		"no group": func(_ *fallbackFixture, o *PaymentOptions) { o.Providers = groups{} },
		"next is disabled": func(f *fallbackFixture, _ *PaymentOptions) {
			f.groups["mobilemidterm/order"][1].Status = provider.StatusDisable
		},
		"next has no route": func(_ *fallbackFixture, o *PaymentOptions) {
			o.PayRoutes = map[string]map[string]bool{"parsian": {provider.BillTypeMobileMidterm: true}}
		},
		"next has no adapter": func(f *fallbackFixture, o *PaymentOptions) {
			o.Adapters = adapters{"parsian": f.parsian}
		},
	}
	for name, change := range cases {
		f := newFallbackFixture(t, change)
		o, err := f.p.Complete(context.Background(), completeMCI())
		if !assert.NoError(t, err, name) {
			continue
		}
		assert.Equal(t, StatusPending, o.Status, name)
		assert.Equal(t, errNotSent.Error(), o.Message, name)
		assert.Equal(t, int32(http.StatusInternalServerError), o.StatusCode, name)
		assert.Equal(t, []string{"149/parsian/in-progress", "149/parsian/pending"}, providersOf(f.repo.inserts), name)
		assert.Empty(t, f.jiring.payReqs, name)
		assert.Empty(t, f.fallbacks, name)
		assert.Equal(t, []string{"mobilemidterm/parsian/pending"}, f.results, name)
	}
}

// TestCompleteFallbackChain (task 058): a next provider that is not reached either goes to the one
// after it while one is defined; when none is left, the last provider has the pending row.
func TestCompleteFallbackChain(t *testing.T) {
	f := newFallbackFixture(t, nil)
	f.jiring.payResp, f.jiring.payErr = nil, errNotSent
	f.groups["mobilemidterm/order"][2].Status = provider.StatusEnable
	o, err := f.p.Complete(context.Background(), completeMCI())
	if assert.NoError(t, err) {
		assert.Equal(t, StatusSuccess, o.Status)
		assert.Equal(t, "third", o.ProviderName)
	}
	assert.Equal(t, []string{"149/parsian/in-progress", "29/jiring-switch/in-progress", "900/third/in-progress", "900/third/success"},
		providersOf(f.repo.inserts))
	assert.Equal(t, []string{"mobilemidterm/parsian/jiring-switch", "mobilemidterm/jiring-switch/third"}, f.fallbacks)
	assert.Equal(t, []string{"parsian/pending", "jiring-switch/pending", "third/success"}, f.pays)

	f = newFallbackFixture(t, nil)
	f.jiring.payResp, f.jiring.payErr = nil, errNotSent
	o, err = f.p.Complete(context.Background(), completeMCI())
	if assert.NoError(t, err) {
		assert.Equal(t, StatusPending, o.Status)
		assert.Equal(t, "jiring-switch", o.ProviderName)
	}
	assert.Equal(t, []string{"149/parsian/in-progress", "29/jiring-switch/in-progress", "29/jiring-switch/pending"}, providersOf(f.repo.inserts))
	assert.Len(t, f.parsian.payReqs, 1, "a provider that was tried is not tried again")
	assert.Equal(t, []string{"mobilemidterm/jiring-switch/pending"}, f.results)
}

// TestCompleteFallbackSaveFails (task 058): when the in-progress row of the next provider is not
// saved, the next provider is not paid; one error line; the answer is the pending of the first call.
func TestCompleteFallbackSaveFails(t *testing.T) {
	f := newFallbackFixture(t, nil)
	f.repo.insertErr = func(o Order) error {
		if o.ProviderName == "jiring-switch" {
			return errors.New("db down")
		}
		return nil
	}
	o, err := f.p.Complete(context.Background(), completeMCI())
	if assert.NoError(t, err) {
		assert.Equal(t, StatusPending, o.Status)
		assert.Equal(t, "parsian", o.ProviderName)
	}
	assert.Empty(t, f.jiring.payReqs)
	assert.Empty(t, f.fallbacks)
	assert.Equal(t, []string{"149/parsian/in-progress", "149/parsian/pending"}, providersOf(f.repo.inserts))
	assert.Equal(t, []string{"error: order complete: save the next provider failed"}, f.log.lines)
}

// TestPayStatus: the status label of OnPay.
func TestPayStatus(t *testing.T) {
	assert.Equal(t, StatusPending, payStatus(nil, nil))
	assert.Equal(t, StatusPending, payStatus(&provider.PaymentResponse{Status: "success"}, errors.New("x")))
	assert.Equal(t, StatusPending, payStatus(&provider.PaymentResponse{Status: "odd"}, nil))
	for _, s := range []string{StatusSuccess, StatusFailed, StatusPending} {
		assert.Equal(t, s, payStatus(&provider.PaymentResponse{Status: s}, nil))
	}
}
