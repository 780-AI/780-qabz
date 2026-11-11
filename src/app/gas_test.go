package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	grpcserver "qabz/entry/grpc"
	"qabz/provider"
)

var gasRegistryReq = provider.InquiryRequest{BillType: "gas", UserID: "u", InquiryID: "0012345678"}

// TestProviderRegistryServesGas (task 023): the Qabzino adapter of the registry answers a gas
// inquiry. Dev mode with the mock: the gas fixture. The real adapter: one POST to
// providers.qabzino.gas_bill_endpoint (a local server here; no call to Ayantech), none to the
// water endpoint. providers.qabzino.enabled false: provider down, no call.
func TestProviderRegistryServesGas(t *testing.T) {
	log := &liveLogger{}
	reg, err := newProviderRegistry(registryConfig(t, "mode: dev\nproviders: {qabzino: {enabled: true}}\ndebug: {mocks: {qabzino: {enable: true, return_type: success}}}"), log)
	if !assert.NoError(t, err) || !assert.NotNil(t, reg) {
		return
	}
	p, ok := reg.Get("qabzino")
	if !assert.True(t, ok) || !assert.NotNil(t, p) {
		return
	}
	resp, err := p.Inquiry(context.Background(), gasRegistryReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) && assert.Len(t, resp.Inquiries, 1) {
		assert.Equal(t, "3200012345", resp.Inquiries[0].PayID, "the gas fixture")
		assert.EqualValues(t, 32_000_000, resp.Inquiries[0].Amount)
	}

	var gasCalls, waterCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gas" {
			gasCalls.Add(1)
		} else {
			waterCalls.Add(1)
		}
		_, _ = w.Write([]byte(`{"Parameters":{"Amount":700,"BillID":"1234567890125","PaymentID":"p1","PaymentDate":"d","Address":"a","ConsumptionType":"k"},"Status":{"Code":"G00000"}}`))
	}))
	defer srv.Close()
	yml := "providers: {qabzino: {enabled: ENABLED, token: t, water_bill_endpoint: \"" + srv.URL + "/water\", gas_bill_endpoint: \"" + srv.URL + "/gas\", timeout: 2s}}"
	reg, err = newProviderRegistry(registryConfig(t, "mode: prod\n"+strings.Replace(yml, "ENABLED", "true", 1)), nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, reg) {
		return
	}
	p, ok = reg.Get("qabzino")
	if !assert.True(t, ok) || !assert.NotNil(t, p) {
		return
	}
	resp, err = p.Inquiry(context.Background(), gasRegistryReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) && assert.Len(t, resp.Inquiries, 1) {
		assert.EqualValues(t, 700, resp.Inquiries[0].Amount)
		assert.Equal(t, &provider.Details{Gas: &provider.GasDetails{City: "a", Kind: "k"}}, resp.Details)
	}
	assert.EqualValues(t, 1, gasCalls.Load())
	assert.EqualValues(t, 0, waterCalls.Load())

	reg, err = newProviderRegistry(registryConfig(t, "mode: prod\n"+strings.Replace(yml, "ENABLED", "false", 1)), nil)
	if assert.NoError(t, err) && assert.NotNil(t, reg) {
		if p, ok = reg.Get("qabzino"); assert.True(t, ok) && assert.NotNil(t, p) {
			_, err = p.Inquiry(context.Background(), gasRegistryReq)
			assert.ErrorIs(t, err, provider.ErrProviderDown)
		}
	}
	assert.EqualValues(t, 1, gasCalls.Load(), "enabled false: no call")
}

// TestNewGasWithoutFlow: no flow gives a service whose Inquiry is not available (no typed nil in
// the interface).
func TestNewGasWithoutFlow(t *testing.T) {
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"})
	svc := newGas(nil, nil)
	if assert.NotNil(t, svc) {
		_, err := svc.Inquiry(ctx, &pb.InquiryBillPayRequest{ParticipateCode: "0012345678"})
		assert.Equal(t, codes.Unimplemented, status.Code(err))
	}
}

// TestDefaultBuilderServesGas: the composition root registers the Gas service. With
// debug.auth.disable and no mock user the call has no user: PermissionDenied; a method that is not
// written yet answers Unimplemented.
func TestDefaultBuilderServesGas(t *testing.T) {
	b, err := NewDefaultBuilder(context.Background(), testConfig(t))
	if !assert.NoError(t, err) || !assert.NotNil(t, b) {
		return
	}
	a := build(t, b)
	defer a.Database().Close()
	defer a.Cache().Close()
	gs, ok := b.grpc.(*grpcserver.Server)
	if !assert.True(t, ok) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		return
	}
	defer func() { assert.NoError(t, gs.Stop(context.Background())) }()
	conn, err := grpc.NewClient(gs.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := pb.NewGasClient(conn)
	_, err = c.Inquiry(ctx, &pb.InquiryBillPayRequest{ParticipateCode: "0012345678"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = c.NewOrder(ctx, &pb.NewOrderBillPayRequest{}) // positive control: a method that is not written yet
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
