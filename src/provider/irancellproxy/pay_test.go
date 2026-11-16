package irancellproxy

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// fakeNATS is a Requester that records each request and gives a fixed answer or error.
type fakeNATS struct {
	mu       sync.Mutex
	answer   []byte
	err      error
	subjects []string
	data     [][]byte
	left     []time.Duration // the time left before the deadline of each request
}

func (f *fakeNATS) Request(ctx context.Context, subject string, data []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subjects = append(f.subjects, subject)
	f.data = append(f.data, append([]byte(nil), data...))
	if d, ok := ctx.Deadline(); ok {
		f.left = append(f.left, time.Until(d))
	}
	return f.answer, f.err
}

func newPayAdapter(t *testing.T, n Requester, payTimeout time.Duration) *Adapter {
	t.Helper()
	a, err := New(Options{Enabled: true, Client: &fakeClient{}, Timeout: time.Second, NATS: n, Subject: "irancell-bill", PayTimeout: payTimeout})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	return a
}

// payReq is the payment request of an Irancell order with a direct price (the pay ID irancell).
var payReq = provider.PaymentRequest{
	BillType: provider.BillTypeIrancell, InquiryType: provider.InquiryTypeFinalTerm, OrderID: "11111111-2222-3333-4444-555555555555",
	UserID: "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11", InquiryID: "989350000391", PayID: "irancell", Amount: 450000, RRN: "445577", Option: "irancell",
}

// goldenPay is the message of payReq as bill sends it: bill's PaymentRequest (ff98e25,
// gateway/payment/dto.go) marshalled with encoding/json v1, printed by a scratch program outside the
// repository on 2026-11-16.
const goldenPay = `{"inquiry_id":"989350000391","bill_id":"","pay_id":"irancell","order_id":"11111111-2222-3333-4444-555555555555","user_id":"6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11","dialer":"","rrn":"445577","amount":450000,"option":"irancell","type":"irancell","InquiryType":"final_term"}`

// TestPayMessageGolden: the message bytes are bill's, byte for byte, on the subject of the config,
// with the pay timeout as the limit.
func TestPayMessageGolden(t *testing.T) {
	n := &fakeNATS{answer: []byte(`{"code":200,"message":"ok","tid":"t-1","status":"success"}`)}
	_, err := newPayAdapter(t, n, 2*time.Second).Pay(context.Background(), payReq)
	if !assert.NoError(t, err) || !assert.Len(t, n.data, 1) {
		return
	}
	assert.Equal(t, goldenPay, string(n.data[0]))
	assert.Equal(t, []string{"irancell-bill"}, n.subjects)
	if assert.Len(t, n.left, 1) {
		assert.InDelta(t, 2*time.Second, n.left[0], float64(500*time.Millisecond), "the limit is the pay timeout")
	}
}

// TestPayMessageEscapes: the HTML and JS escapes and the largest amount are those of bill's v1
// Marshal (the second golden line of the same scratch program).
func TestPayMessageEscapes(t *testing.T) {
	b, err := marshalPayMessage(payMessage{InquiryID: "<a&b>", BillID: "x\u2028y", PayID: `"q"`, OrderID: "o", UserID: "u", Dialer: "989121234567",
		RRN: "1", Amount: 18446744073709551615, Option: "irancell", BillType: "irancell", InquiryType: "final_term"})
	if assert.NoError(t, err) {
		assert.Equal(t, `{"inquiry_id":"\u003ca\u0026b\u003e","bill_id":"x\u2028y","pay_id":"\"q\"","order_id":"o","user_id":"u","dialer":"989121234567","rrn":"1","amount":18446744073709551615,"option":"irancell","type":"irancell","InquiryType":"final_term"}`, string(b))
	}
}

// TestPayAnswers: the answer of the irancell service (entity.ChargeResponse) is kept as it came; the
// tid is not kept (bill: ref_id empty); a missing status stays empty (the order step makes it
// pending, as bill); keys in another case are not read.
func TestPayAnswers(t *testing.T) {
	for in, want := range map[string]provider.PaymentResponse{
		`{"code":200,"message":"ok","tid":"1100445577","status":"success"}`: {Code: 200, Message: "ok", Status: "success"},
		`{"code":400,"message":"no balance","tid":"t","status":"failed"}`:   {Code: 400, Message: "no balance", Status: "failed"},
		`{"code":500,"message":"later","status":"pending"}`:                 {Code: 500, Message: "later", Status: "pending"},
		`{"code":400,"message":"invalid phone"}`:                            {Code: 400, Message: "invalid phone"},
		`{"code":null,"message":null,"status":null}`:                        {},
		`{"CODE":200,"Status":"success"}`:                                   {},
		`{}`:                                                                {},
		`null`:                                                              {}, // bill's v1: no error, every field zero
	} {
		resp, err := newPayAdapter(t, &fakeNATS{answer: []byte(in)}, 0).Pay(context.Background(), payReq)
		if assert.NoError(t, err, in) && assert.NotNil(t, resp, in) {
			assert.Equal(t, want, *resp, in)
		}
	}
}

// TestPayBadAnswer: an answer that bill's v1 decode rejects is "provider down" (bill: pending).
func TestPayBadAnswer(t *testing.T) {
	for _, in := range []string{``, `not json`, `[1]`, `"x"`, `{"code":"200"}`, `{"code":3000000000}`, `{"code":200.5}`, `{"message":1}`, `{"status":true}`} {
		resp, err := newPayAdapter(t, &fakeNATS{answer: []byte(in)}, 0).Pay(context.Background(), payReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown, in)
		assert.Nil(t, resp, in)
	}
}

// TestPayFailures: the switch off, no NATS client, no subject: "provider down" with no message; a
// failed request is "provider down"; another bill type is not supported.
func TestPayFailures(t *testing.T) {
	n := &fakeNATS{}
	off, err := New(Options{Client: &fakeClient{}, Timeout: time.Second, NATS: n, Subject: "irancell-bill"})
	if !assert.NoError(t, err) {
		return
	}
	noSubject, err := New(Options{Enabled: true, Client: &fakeClient{}, Timeout: time.Second, NATS: n})
	if !assert.NoError(t, err) {
		return
	}
	for name, a := range map[string]*Adapter{"off": off, "no NATS": newPayAdapter(t, nil, 0), "no subject": noSubject} {
		_, err := a.Pay(context.Background(), payReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown, name)
	}
	assert.Empty(t, n.data, "no message is sent")

	cause := errors.New("nats: request irancell-bill: context deadline exceeded")
	_, err = newPayAdapter(t, &fakeNATS{err: cause}, 0).Pay(context.Background(), payReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, cause)

	other := payReq
	other.BillType = provider.BillTypeMobile
	_, err = newPayAdapter(t, n, 0).Pay(context.Background(), other)
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	assert.Empty(t, n.data)
	assert.True(t, SupportsPay(provider.BillTypeIrancell))
	assert.False(t, SupportsPay(provider.BillTypeMobile))

	var nilAdapter *Adapter
	_, err = nilAdapter.Pay(context.Background(), payReq)
	assert.Error(t, err)
}

// TestPayTimeoutOption: 0 is the default 30 s; a negative value is rejected.
func TestPayTimeoutOption(t *testing.T) {
	n := &fakeNATS{answer: []byte(`{"status":"success"}`)}
	_, err := newPayAdapter(t, n, 0).Pay(context.Background(), payReq)
	if assert.NoError(t, err) && assert.Len(t, n.left, 1) {
		assert.InDelta(t, defaultPayTimeout, n.left[0], float64(time.Second))
	}
	_, err = New(Options{Client: &fakeClient{}, Timeout: time.Second, PayTimeout: -time.Second})
	assert.Error(t, err)
}

// TestMockPay (conventions § 15): the mock answers the payment by pay_return_type with no request;
// success is the success answer of the irancell service with no trace ID.
func TestMockPay(t *testing.T) {
	n := &fakeNATS{}
	for rt, check := range map[string]func(*provider.PaymentResponse, error){
		provider.MockReturnSuccess: func(r *provider.PaymentResponse, err error) {
			if assert.NoError(t, err) && assert.NotNil(t, r) {
				assert.Equal(t, provider.PaymentResponse{Status: provider.StatusSuccess, Code: 200, Message: successMessage}, *r)
			}
		},
		provider.MockReturnFailed: func(r *provider.PaymentResponse, err error) {
			if assert.NoError(t, err) && assert.NotNil(t, r) {
				assert.Equal(t, provider.StatusFailed, r.Status)
			}
		},
		provider.MockReturnPending: func(_ *provider.PaymentResponse, err error) { assert.ErrorIs(t, err, provider.ErrProviderDown) },
	} {
		m, err := provider.NewMock(newPayAdapter(t, n, 0), provider.MockSettings{Enable: true, ReturnType: provider.MockReturnSuccess, PayReturnType: rt}, MockFixtures(), nil)
		if !assert.NoError(t, err, rt) {
			continue
		}
		check(m.Pay(context.Background(), payReq))
	}
	assert.Empty(t, n.data, "the mock sends no message")

	// Mock off: the adapter answers (over the fake NATS).
	n = &fakeNATS{answer: []byte(`{"code":200,"status":"success"}`)}
	m, err := provider.NewMock(newPayAdapter(t, n, 0), provider.MockSettings{}, MockFixtures(), nil)
	if assert.NoError(t, err) {
		_, err = m.Pay(context.Background(), payReq)
		assert.NoError(t, err)
		assert.Len(t, n.data, 1)
	}
}
