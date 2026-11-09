package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth/authtest"
	"qabz/core/inquiry"
	grpcserver "qabz/entry/grpc"
	"qabz/provider"
)

// fakeWaterFlow answers Inquire with res and err and records the requests.
type fakeWaterFlow struct {
	mu   sync.Mutex
	reqs []inquiry.Request
	res  inquiry.Result
	err  error
}

func (f *fakeWaterFlow) Inquire(_ context.Context, req inquiry.Request) (inquiry.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reqs = append(f.reqs, req)
	return f.res, f.err
}

// waterStack starts a gRPC server with the JWT check on, the Water service over flow, and a gateway in front of it.
func waterStack(t *testing.T, flow grpcserver.WaterInquirer) *Gateway {
	t.Helper()
	a, err := grpcserver.NewAuth(grpcserver.AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", a.ServerOptions(), grpcserver.RegisterWater(grpcserver.NewWater(grpcserver.WaterDeps{Inquiry: flow})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterWaterHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })
	return gw
}

func postWater(t *testing.T, gw *Gateway, body string, withToken bool) (int, []byte) {
	t.Helper()
	return callMyBillPath(t, gw, http.MethodPost, "/v1/water/inquiry", body, withToken)
}

func TestWaterInquiryRouteSuccess(t *testing.T) {
	f := &fakeWaterFlow{res: inquiry.Result{BillID: "1234567890125", Inquiries: []inquiry.Inquiry{{PayID: "2500012345", Amount: 25000000, PaymentDeadline: "2030-01-01"}}}}
	gw := waterStack(t, f)
	code, body := postWater(t, gw, `{"billId":"1234567890125","payId":"x"}`, true)
	assert.Equal(t, http.StatusOK, code, string(body))
	var res pb.InquiryBillPayResponse
	if assert.NoError(t, protojson.Unmarshal(body, &res), string(body)) {
		assert.Equal(t, uint32(25000000), res.GetPrice())
		assert.Equal(t, "2030-01-01", res.GetPaymentDeadline())
		assert.Equal(t, "2500012345", res.GetPayId())
		assert.Equal(t, "1234567890125", res.GetBillId())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, authtest.UserID, f.reqs[0].UserID)
		assert.Equal(t, "1234567890125", f.reqs[0].InquiryID)
	}
}

func TestWaterInquiryRouteStatusCodes(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		token bool
		want  int
	}{
		{"no token", nil, false, http.StatusForbidden},
		{"invalid", inquiry.ErrInvalid, true, http.StatusBadRequest},
		{"no provider", fmt.Errorf("x: %w", provider.ErrNoProvider), true, http.StatusServiceUnavailable},
		{"provider down", fmt.Errorf("x: %w", provider.ErrProviderDown), true, http.StatusServiceUnavailable},
		{"provider refused", &inquiry.RefusedError{Message: "bill not found"}, true, http.StatusBadRequest},
		{"database", fmt.Errorf("x: %w", assert.AnError), true, http.StatusInternalServerError},
	}
	for _, c := range cases {
		f := &fakeWaterFlow{err: c.err}
		gw := waterStack(t, f)
		code, body := postWater(t, gw, `{"billId":"1234567890125"}`, c.token)
		assert.Equal(t, c.want, code, c.name+": "+string(body))
	}
}
