package httpserver

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth/authtest"
	"qabz/core/inquiry"
	grpcserver "qabz/entry/grpc"
	"qabz/provider"
)

// telStack starts a gRPC server with the JWT check on, the Tel service over flow (the same value
// answers the raw inquiry when raw is not nil), and a gateway in front of it.
func telStack(t *testing.T, flow grpcserver.Inquirer, raw grpcserver.RawInquirer) *Gateway {
	t.Helper()
	a, err := grpcserver.NewAuth(grpcserver.AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", a.ServerOptions(), grpcserver.RegisterTel(grpcserver.NewTel(grpcserver.TelDeps{Inquiry: flow, Raw: raw})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterTelHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })
	return gw
}

func telFlowResult() inquiry.Result {
	return inquiry.Result{BillID: "2101234560040", Inquiries: []inquiry.Inquiry{
		{BillID: "2101234560040", PayID: "12000035", Amount: 120000, InquiryType: provider.InquiryTypeMidterm},
		{BillID: "2101234560040", PayID: "35000042", Amount: 350000, InquiryType: provider.InquiryTypeFinalTerm},
	}}
}

// TestTelInquiryRouteSuccess (task 025): POST /v1/tel/inquiry takes bill's request fields (phone,
// billId, payId) and answers bill's response: midTerm and fullTerm, each with billId, payId, price,
// paymentDeadline and providerId.
func TestTelInquiryRouteSuccess(t *testing.T) {
	f := &fakeWaterFlow{res: telFlowResult()}
	gw := telStack(t, f, nil)
	code, body := callMyBillPath(t, gw, http.MethodPost, "/v1/tel/inquiry", `{"phone":"02188776655","billId":"","payId":""}`, true)
	assert.Equal(t, http.StatusOK, code, string(body))
	var fields map[string]any
	if assert.NoError(t, json.Unmarshal(body, &fields), string(body)) {
		part := func(payID string, price float64) map[string]any {
			return map[string]any{"billId": "2101234560040", "payId": payID, "price": price, "paymentDeadline": "", "providerId": float64(0)}
		}
		assert.Equal(t, map[string]any{"midTerm": part("12000035", 120000), "fullTerm": part("35000042", 350000)}, fields, "the JSON of the answer: bill's fields")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, inquiry.Request{BillType: "tel", InquiryID: "982188776655", UserID: authtest.UserID, Dialer: f.reqs[0].Dialer}, f.reqs[0])
	}
}

func TestTelInquiryRouteStatusCodes(t *testing.T) {
	const tel = `{"phone":"982188776655"}`
	cases := []struct {
		name  string
		body  string
		err   error
		token bool
		want  int
		calls int
	}{
		{"no token", tel, nil, false, http.StatusForbidden, 0},
		{"nothing", `{}`, nil, true, http.StatusBadRequest, 0},
		{"only billId", `{"billId":"2101234560040"}`, nil, true, http.StatusBadRequest, 0},
		{"a bad phone", `{"phone":"12345"}`, nil, true, http.StatusBadRequest, 0},
		{"raw inquiry with no raw service", `{"billId":"2101234560040","payId":"35000042"}`, nil, true, http.StatusNotImplemented, 0},
		{"provider down", tel, fmt.Errorf("x: %w", provider.ErrProviderDown), true, http.StatusServiceUnavailable, 1},
		{"no provider", tel, fmt.Errorf("x: %w", provider.ErrNoProvider), true, http.StatusServiceUnavailable, 1},
		{"provider refused", tel, &inquiry.RefusedError{Message: "تلفن نامعتبر است"}, true, http.StatusBadRequest, 1},
		{"database", tel, fmt.Errorf("x: %w", assert.AnError), true, http.StatusInternalServerError, 1},
	}
	for _, c := range cases {
		f := &fakeWaterFlow{err: c.err, res: telFlowResult()}
		gw := telStack(t, f, nil)
		code, body := callMyBillPath(t, gw, http.MethodPost, "/v1/tel/inquiry", c.body, c.token)
		assert.Equal(t, c.want, code, c.name+": "+string(body))
		f.mu.Lock()
		assert.Len(t, f.reqs, c.calls, c.name)
		f.mu.Unlock()
	}
}
