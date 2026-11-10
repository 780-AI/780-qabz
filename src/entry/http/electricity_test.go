package httpserver

import (
	"context"
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

// electricityStack starts a gRPC server with the JWT check on, the Electricity service over flow,
// and a gateway in front of it.
func electricityStack(t *testing.T, flow grpcserver.Inquirer) *Gateway {
	t.Helper()
	a, err := grpcserver.NewAuth(grpcserver.AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", a.ServerOptions(), grpcserver.RegisterElectricity(grpcserver.NewElectricity(grpcserver.ElectricityDeps{Inquiry: flow})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterElectricityHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })
	return gw
}

func postElectricity(t *testing.T, gw *Gateway, body string, withToken bool) (int, []byte) {
	t.Helper()
	return callMyBillPath(t, gw, http.MethodPost, "/v1/electricity/inquiry", body, withToken)
}

func TestElectricityInquiryRouteSuccess(t *testing.T) {
	f := &fakeWaterFlow{res: inquiry.Result{BillID: "1234567890125",
		Details: &provider.Details{Electricity: &provider.ElectricityDetails{Phase: "تک فاز", CompanyName: "co", TariffType: "ta", CustomerType: 1,
			SerialNumber: "se", ServiceAddress: "ad", ServicePostCode: "po", CustomerName: "na", City: "ci", Region: "re"}},
		Inquiries: []inquiry.Inquiry{{PayID: "1800012345", Amount: 18000000, PaymentDeadline: "2030-01-01"}}}}
	gw := electricityStack(t, f)
	code, body := postElectricity(t, gw, `{"billId":"1234567890125","payId":"x"}`, true)
	assert.Equal(t, http.StatusOK, code, string(body))
	var res pb.InquiryElectricityResponse
	if assert.NoError(t, protojson.Unmarshal(body, &res), string(body)) {
		assert.Equal(t, uint32(18000000), res.GetPrice())
		assert.Equal(t, "2030-01-01", res.GetPaymentDeadline())
		assert.Equal(t, "1800012345", res.GetPayId())
		assert.Equal(t, "1234567890125", res.GetBillId())
		assert.Equal(t, []string{"تک فاز", "co", "ta", "1", "se", "ad", "po", "na"},
			[]string{res.GetPhase(), res.GetCompanyName(), res.GetTariffType(), res.GetCustomerType(), res.GetSerialNumber(), res.GetServiceAddress(), res.GetServicePostCode(), res.GetCustomerName()})
	}
	for _, key := range []string{`"phase"`, `"companyName"`, `"tariffType"`, `"customerType"`, `"serialNumber"`, `"serviceAddress"`, `"servicePostCode"`, `"customerName"`} {
		assert.Contains(t, string(body), key, "the JSON names of the client")
	}
	assert.NotContains(t, string(body), "region", "city and region are not sent")
	f.mu.Lock()
	defer f.mu.Unlock()
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, authtest.UserID, f.reqs[0].UserID)
		assert.Equal(t, "1234567890125", f.reqs[0].InquiryID)
		assert.Equal(t, "electricity", f.reqs[0].BillType)
	}
}

func TestElectricityInquiryRouteStatusCodes(t *testing.T) {
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
		gw := electricityStack(t, &fakeWaterFlow{err: c.err})
		code, body := postElectricity(t, gw, `{"billId":"1234567890125"}`, c.token)
		assert.Equal(t, c.want, code, c.name+": "+string(body))
	}
	// The document route exists in the proto and is not written yet: HTTP 501.
	gw := electricityStack(t, &fakeWaterFlow{})
	code, body := callMyBillPath(t, gw, http.MethodPost, "/v1/electricity/inquiry/document", `{"billId":"1234567890125"}`, true)
	assert.Equal(t, http.StatusNotImplemented, code, string(body))
}
