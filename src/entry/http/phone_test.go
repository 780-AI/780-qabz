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

// phoneStack starts a gRPC server with the JWT check on, the Phone service over flow, and a gateway in front of it.
func phoneStack(t *testing.T, flow grpcserver.Inquirer) *Gateway {
	t.Helper()
	a, err := grpcserver.NewAuth(grpcserver.AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", a.ServerOptions(), grpcserver.RegisterPhone(grpcserver.NewPhone(grpcserver.PhoneDeps{Inquiry: flow})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterPhoneHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })
	return gw
}

func postPhone(t *testing.T, gw *Gateway, body string, withToken bool) (int, []byte) {
	t.Helper()
	return callMyBillPath(t, gw, http.MethodPost, "/v1/mobile/inquiry", body, withToken)
}

func irancellFlowResult() inquiry.Result {
	return inquiry.Result{Inquiries: []inquiry.Inquiry{
		{Amount: 150000, InquiryType: provider.InquiryTypeMidterm},
		{Amount: 450000, InquiryType: provider.InquiryTypeFinalTerm},
	}}
}

// TestPhoneInquiryRouteSuccess (task 035): POST /v1/mobile/inquiry takes bill's request fields and
// answers bill's response: midTerm and fullTerm, each with billId, payId, price, paymentDeadline
// and providerId.
func TestPhoneInquiryRouteSuccess(t *testing.T) {
	f := &fakeWaterFlow{res: irancellFlowResult()}
	gw := phoneStack(t, f)
	code, body := postPhone(t, gw, `{"phone":"989351234567","operator":"irancell","billId":"","payId":"","service":"mci"}`, true)
	assert.Equal(t, http.StatusOK, code, string(body))
	var res pb.InquiryPhoneResponse
	if assert.NoError(t, protojson.Unmarshal(body, &res), string(body)) {
		assert.Equal(t, uint32(150000), res.GetMidTerm().GetPrice())
		assert.Equal(t, uint32(450000), res.GetFullTerm().GetPrice())
	}
	var fields map[string]any
	if assert.NoError(t, json.Unmarshal(body, &fields), string(body)) {
		part := func(price float64) map[string]any {
			return map[string]any{"billId": "", "payId": "", "price": price, "paymentDeadline": "", "providerId": float64(0)}
		}
		assert.Equal(t, map[string]any{"midTerm": part(150000), "fullTerm": part(450000)}, fields, "the JSON of the answer: bill's fields")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, inquiry.Request{BillType: "irancell", InquiryID: "989351234567", UserID: authtest.UserID, Dialer: f.reqs[0].Dialer}, f.reqs[0])
	}
}

func TestPhoneInquiryRouteStatusCodes(t *testing.T) {
	const irancell = `{"phone":"989351234567","operator":"irancell"}`
	cases := []struct {
		name  string
		body  string
		err   error
		token bool
		want  int
		calls int
	}{
		{"no token", irancell, nil, false, http.StatusForbidden, 0},
		{"the operator as a number", `{"phone":"989351234567","operator":1}`, nil, true, http.StatusOK, 1},
		{"nothing", `{}`, nil, true, http.StatusBadRequest, 0},
		{"no phone", `{"operator":"irancell","billId":"1234567890125","payId":"1"}`, nil, true, http.StatusBadRequest, 0},
		{"a bad phone", `{"phone":"09351234567","operator":"irancell"}`, nil, true, http.StatusBadRequest, 0},
		{"a bad bill ID", `{"phone":"989351234567","operator":"irancell","billId":"12345"}`, nil, true, http.StatusBadRequest, 0},
		{"an operator number that is not in the enum", `{"phone":"989351234567","operator":9}`, nil, true, http.StatusBadRequest, 0},
		{"an operator name that is not in the enum is read as 0 (mci) by the gateway", `{"phone":"989351234567","operator":"mtn"}`, nil, true, http.StatusNotImplemented, 0},
		{"the operator unknown", `{"phone":"989351234567","operator":"unknown"}`, nil, true, http.StatusBadRequest, 0},
		{"mci: not available yet", `{"phone":"989121234567","operator":"mci"}`, nil, true, http.StatusNotImplemented, 0},
		{"no operator is mci", `{"phone":"989121234567"}`, nil, true, http.StatusNotImplemented, 0},
		{"rightel: not available yet", `{"phone":"989201234567","operator":"rightel"}`, nil, true, http.StatusNotImplemented, 0},
		{"flow invalid", irancell, inquiry.ErrInvalid, true, http.StatusBadRequest, 1},
		{"no provider", irancell, fmt.Errorf("x: %w", provider.ErrNoProvider), true, http.StatusServiceUnavailable, 1},
		{"provider down", irancell, fmt.Errorf("x: %w", provider.ErrProviderDown), true, http.StatusServiceUnavailable, 1},
		{"provider refused", irancell, &inquiry.RefusedError{Message: "not an Irancell number"}, true, http.StatusBadRequest, 1},
		{"database", irancell, fmt.Errorf("x: %w", assert.AnError), true, http.StatusInternalServerError, 1},
	}
	for _, c := range cases {
		f := &fakeWaterFlow{err: c.err, res: irancellFlowResult()}
		gw := phoneStack(t, f)
		code, body := postPhone(t, gw, c.body, c.token)
		assert.Equal(t, c.want, code, c.name+": "+string(body))
		f.mu.Lock()
		assert.Len(t, f.reqs, c.calls, c.name)
		f.mu.Unlock()
	}
}
