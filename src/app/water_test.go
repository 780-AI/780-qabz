package app

import (
	"context"
	"testing"
	"time"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/monitoring/prometheus"
	"qabz/config"
	grpcserver "qabz/entry/grpc"
	"qabz/provider"
	"qabz/system/postgres"
)

// TestMockConstantsMatchConfig: the return types of provider and config are the same strings.
func TestMockConstantsMatchConfig(t *testing.T) {
	assert.Equal(t, config.MockReturnSuccess, provider.MockReturnSuccess)
	assert.Equal(t, config.MockReturnError, provider.MockReturnError)
}

func registryConfig(t *testing.T, yml string) config.Config {
	t.Helper()
	c, err := config.Parse([]byte("database: {postgres: {password: pw}}\n" + yml))
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	return c
}

// qabzinoDisabledWarn is the start warn line of Qabzino with providers.qabzino.enabled false (task 030).
const qabzinoDisabledWarn = "providers.qabzino.enabled is false: every inquiry to Qabzino fails as provider down; the Qabzino mock does not apply"

// TestNewProviderRegistry: the registry has the Qabzino adapter; the mock wraps it only in dev mode
// with debug.mocks.qabzino.enable (decision 4).
func TestNewProviderRegistry(t *testing.T) {
	req := provider.InquiryRequest{BillType: "water", UserID: "u", InquiryID: "1234567890125"}
	cases := []struct {
		name     string
		yml      string
		wantMock bool
		wantErr  bool // a mock error answer
		wantWarn string
	}{
		{"dev, mock success", "mode: dev\nproviders: {qabzino: {enabled: true}}\ndebug: {mocks: {qabzino: {enable: true, return_type: success}}}", true, false, "provider mock enabled (debug.mocks.qabzino): development only, no call to the Qabzino backend"},
		{"dev, mock error", "mode: dev\nproviders: {qabzino: {enabled: true}}\ndebug: {mocks: {qabzino: {enable: true, return_type: error}}}", true, true, "provider mock enabled (debug.mocks.qabzino): development only, no call to the Qabzino backend"},
		{"dev, mock on, qabzino disabled", "mode: dev\ndebug: {mocks: {qabzino: {enable: true, return_type: success}}}", false, false, qabzinoDisabledWarn},
		{"dev, mock off, qabzino disabled", "mode: dev\ndebug: {mocks: {qabzino: {enable: false}}}", false, false, qabzinoDisabledWarn},
		{"stage, enable ignored, qabzino disabled", "mode: stage\ndebug: {mocks: {qabzino: {enable: true}}}", false, false, qabzinoDisabledWarn},
		{"prod, enabled, no token", "mode: prod\nproviders: {qabzino: {enabled: true, timeout: 300ms}, tavanir: {enabled: true, timeout: 300ms}}", false, false, ""},
		{"prod, enabled, local endpoint (nothing listens)", "mode: prod\nproviders: {qabzino: {enabled: true, token: t, water_bill_endpoint: \"http://127.0.0.1:1/w\", timeout: 300ms}, tavanir: {enabled: true, timeout: 300ms}}", false, false, ""},
	}
	for _, c := range cases {
		log := &liveLogger{}
		reg, err := newProviderRegistry(registryConfig(t, c.yml), log)
		if !assert.NoError(t, err, c.name) || !assert.NotNil(t, reg, c.name) {
			continue
		}
		p, ok := reg.Get("qabzino")
		if !assert.True(t, ok, c.name) {
			continue
		}
		resp, err := p.Inquiry(context.Background(), req)
		switch {
		case c.wantMock && !c.wantErr:
			if assert.NoError(t, err, c.name) && assert.NotNil(t, resp, c.name) && assert.Len(t, resp.Inquiries, 1, c.name) {
				assert.Equal(t, "2500012345", resp.Inquiries[0].PayID, c.name)
			}
			assert.Equal(t, 1, log.count("info", "provider mock: inquiry answered with no call to the provider"), c.name)
		case c.wantMock && c.wantErr:
			assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
		default:
			assert.ErrorIs(t, err, provider.ErrProviderDown, c.name+": the real adapter, no backend")
			assert.Equal(t, 0, log.count("info", "provider mock: inquiry answered with no call to the provider"), c.name)
		}
		if c.wantWarn != "" {
			assert.Equal(t, 1, log.count("warn", c.wantWarn), c.name)
		} else {
			assert.Equal(t, 0, len(log.lines)-log.count("info", "provider mock: inquiry answered with no call to the provider")-log.count("warn", tavanirDisabledWarn)-log.count("warn", irancellDisabledWarn), c.name+": no warn line of Qabzino")
		}
	}
	// A nil logger writes nothing and does not panic.
	reg, err := newProviderRegistry(registryConfig(t, "mode: dev\nproviders: {qabzino: {enabled: true}}\ndebug: {mocks: {qabzino: {enable: true}}}"), nil)
	if assert.NoError(t, err) && assert.NotNil(t, reg) {
		p, _ := reg.Get("qabzino")
		_, err = p.Inquiry(context.Background(), req)
		assert.NoError(t, err)
	}
}

// TestNewInquiryFlowRules: with metrics on the three metrics are registered once (a second call
// conflicts, also when only one is taken); with metrics off nothing is registered. The flow is
// built once for all inquiry endpoints (task 022).
func TestNewInquiryFlowRules(t *testing.T) {
	cfg := testConfig(t)
	db, err := postgres.New(context.Background(), cfg.PostgresDSN()) // lazy: no server needed
	if !assert.NoError(t, err) || !assert.NotNil(t, db) {
		t.FailNow()
	}
	defer db.Close()
	reg, err := newProviderRegistry(cfg, nil)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	sel := &provider.ProvidersService{}

	on := prometheus.New("127.0.0.1:0", true)
	flow, err := newInquiryFlow(cfg, db, sel, reg, on, nil)
	assert.NoError(t, err)
	assert.NotNil(t, flow)
	for _, name := range []string{"qabz_inquiries_total", "qabz_provider_calls_total", "qabz_provider_call_duration_ms"} {
		assert.True(t, registered(t, on.Registry(), name), name)
	}
	_, err = newInquiryFlow(cfg, db, sel, reg, on, nil)
	assert.ErrorContains(t, err, "inquiry: register metric")

	taken := prometheus.New("127.0.0.1:0", true)
	if assert.NotNil(t, taken.Registry()) {
		assert.NoError(t, taken.Registry().Register(promclient.NewCounter(promclient.CounterOpts{Name: "qabz_provider_calls_total", Help: "taken"})))
		flow, err = newInquiryFlow(cfg, db, sel, reg, taken, nil)
		assert.ErrorContains(t, err, "inquiry: register metric")
		assert.Nil(t, flow)
	}

	off := prometheus.New("127.0.0.1:0", false)
	for range 2 {
		flow, err := newInquiryFlow(cfg, db, sel, reg, off, nil)
		assert.NoError(t, err)
		assert.NotNil(t, flow)
		assert.NotNil(t, newWater(flow, nil))
		assert.NotNil(t, newElectricity(flow, nil))
	}
	// A flow that cannot be built: the selector is required.
	_, err = newInquiryFlow(cfg, db, nil, reg, off, nil)
	assert.Error(t, err)

	// No flow: the services exist and their Inquiry is not available (no typed nil in the interface).
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"})
	_, err = newWater(nil, nil).Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = newElectricity(nil, nil).Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

// TestDefaultBuilderServesWater: the composition root registers the Water service. With
// debug.auth.disable and no mock user the call has no user: PermissionDenied; an unregistered
// service would answer Unimplemented.
func TestDefaultBuilderServesWater(t *testing.T) {
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
	c := pb.NewWaterClient(conn)
	_, err = c.Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = c.NewOrder(ctx, &pb.NewOrderBillPayRequest{}) // positive control: a method that is not written yet
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
