package httpserver

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth/authtest"
	"qabz/core/inquiry"
	grpcserver "qabz/entry/grpc"
	"qabz/provider"
)

// gasStack starts a gRPC server with the JWT check on, the Gas service over flow, and a gateway in front of it.
func gasStack(t *testing.T, flow grpcserver.Inquirer) *Gateway {
	t.Helper()
	a, err := grpcserver.NewAuth(grpcserver.AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", a.ServerOptions(), grpcserver.RegisterGas(grpcserver.NewGas(grpcserver.GasDeps{Inquiry: flow})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterGasHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })
	return gw
}

func postGas(t *testing.T, gw *Gateway, body string, withToken bool) (int, []byte) {
	t.Helper()
	return callMyBillPath(t, gw, http.MethodPost, "/v1/gas/inquiry", body, withToken)
}

// TestGasInquiryRouteSuccess (task 023): POST /v1/gas/inquiry takes bill's request fields (billId,
// payId, participateCode) and answers bill's response fields (price, paymentDeadline, payId,
// billId) and no other field.
func TestGasInquiryRouteSuccess(t *testing.T) {
	f := &fakeWaterFlow{res: inquiry.Result{BillID: "1234567890125", Inquiries: []inquiry.Inquiry{{PayID: "3200012345", Amount: 32000000, PaymentDeadline: "2030-01-01"}},
		Details: &provider.Details{Gas: &provider.GasDetails{City: "c", Kind: "k"}}}}
	gw := gasStack(t, f)
	code, body := postGas(t, gw, `{"participateCode":"0012345678","billId":"1234567890125"}`, true)
	assert.Equal(t, http.StatusOK, code, string(body))
	var res pb.InquiryBillPayResponse
	if assert.NoError(t, protojson.Unmarshal(body, &res), string(body)) {
		assert.Equal(t, uint32(32000000), res.GetPrice())
		assert.Equal(t, "2030-01-01", res.GetPaymentDeadline())
		assert.Equal(t, "3200012345", res.GetPayId())
		assert.Equal(t, "1234567890125", res.GetBillId())
	}
	var fields map[string]any
	if assert.NoError(t, json.Unmarshal(body, &fields), string(body)) {
		assert.Equal(t, map[string]any{"price": float64(32000000), "paymentDeadline": "2030-01-01", "payId": "3200012345", "billId": "1234567890125"},
			fields, "the JSON of the answer: bill's four fields; the details are not sent")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, inquiry.Request{BillType: "gas", InquiryID: "0012345678", UserID: authtest.UserID, Dialer: f.reqs[0].Dialer}, f.reqs[0])
	}
}

func TestGasInquiryRouteStatusCodes(t *testing.T) {
	const byCode = `{"participateCode":"0012345678"}`
	cases := []struct {
		name  string
		body  string
		err   error
		token bool
		want  int
		calls int
	}{
		{"no token", byCode, nil, false, http.StatusForbidden, 0},
		{"nothing", `{}`, nil, true, http.StatusBadRequest, 0},
		{"bill ID only", `{"billId":"1234567890125"}`, nil, true, http.StatusBadRequest, 0},
		{"a bad bill ID", `{"billId":"12345","participateCode":"0012345678"}`, nil, true, http.StatusBadRequest, 0},
		{"bill ID and pay ID, no raw inquirer", `{"billId":"1234567890125","payId":"3200012345"}`, nil, true, http.StatusNotImplemented, 0},
		{"flow invalid", byCode, inquiry.ErrInvalid, true, http.StatusBadRequest, 1},
		{"no provider", byCode, fmt.Errorf("x: %w", provider.ErrNoProvider), true, http.StatusServiceUnavailable, 1},
		{"provider down", byCode, fmt.Errorf("x: %w", provider.ErrProviderDown), true, http.StatusServiceUnavailable, 1},
		{"provider refused", byCode, &inquiry.RefusedError{Message: "code not found"}, true, http.StatusBadRequest, 1},
		{"database", byCode, fmt.Errorf("x: %w", assert.AnError), true, http.StatusInternalServerError, 1},
	}
	for _, c := range cases {
		f := &fakeWaterFlow{err: c.err, res: inquiry.Result{Inquiries: []inquiry.Inquiry{{}}}}
		gw := gasStack(t, f)
		code, body := postGas(t, gw, c.body, c.token)
		assert.Equal(t, c.want, code, c.name+": "+string(body))
		f.mu.Lock()
		assert.Len(t, f.reqs, c.calls, c.name)
		f.mu.Unlock()
	}
}

// TestGasRawInquiryRoute: over HTTP, with the real flow as the raw inquirer, a request with billId
// and payId answers bill's fields from the two IDs; a bad pay ID is HTTP 400 with bill's text.
func TestGasRawInquiryRoute(t *testing.T) {
	flow, err := inquiry.NewService(inquiry.Options{Repository: noRepo{}, Providers: noSelector{}, Adapters: noAdapters{}})
	if !assert.NoError(t, err) || !assert.NotNil(t, flow) {
		t.FailNow()
	}
	a, err := grpcserver.NewAuth(grpcserver.AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", a.ServerOptions(), grpcserver.RegisterGas(grpcserver.NewGas(grpcserver.GasDeps{Inquiry: flow, Raw: flow})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterGasHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })

	code, body := postGas(t, gw, `{"billId":"1234567890125","payId":"3200012345","participateCode":""}`, true)
	assert.Equal(t, http.StatusOK, code, string(body))
	var res pb.InquiryBillPayResponse
	if assert.NoError(t, protojson.Unmarshal(body, &res), string(body)) {
		assert.Equal(t, []any{uint32(32000000), "3200012345", "1234567890125", ""}, []any{res.GetPrice(), res.GetPayId(), res.GetBillId(), res.GetPaymentDeadline()})
	}
	for name, c := range map[string][2]string{
		"bad pay ID":        {`{"billId":"1234567890125","payId":"2500012345"}`, "شناسه پرداخت نامعتبر است"},
		"unknown bill type": {`{"billId":"123456789069","payId":"3200012345"}`, "نوع قبض نامعتبر است"},
		"bad bill ID":       {`{"billId":"1234567890124","payId":"3200012345"}`, "invalid parameter"},
	} {
		code, body = postGas(t, gw, c[0], true)
		assert.Equal(t, http.StatusBadRequest, code, name+": "+string(body))
		assert.Contains(t, string(body), c[1], name)
	}
	code, _ = postGas(t, gw, `{"billId":"1234567890125","payId":"3200012345"}`, false)
	assert.Equal(t, http.StatusForbidden, code, "no token")
}

// noRepo, noSelector and noAdapters fail the test's flow if a raw inquiry touches them.
type (
	noRepo     struct{ inquiry.Repository }
	noSelector struct{}
	noAdapters struct{}
)

func (noSelector) PickProvider(string, string) (provider.Provider, error) {
	return provider.Provider{}, assert.AnError
}
func (noAdapters) Get(string) (provider.ProviderInterface, bool) { return nil, false }
