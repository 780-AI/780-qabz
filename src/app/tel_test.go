package app

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/provider"
	"qabz/provider/karashop"
)

const (
	karashopMockWarn     = "provider mock enabled (debug.mocks.karashop): development only, no call to the Karashop backend"
	karashopDisabledWarn = "providers.karashop.enabled is false: every inquiry to Karashop fails as provider down; the Karashop mock does not apply"
)

// TestNewProviderRegistryKarashop (task 025): the Karashop adapter is registered under the name of
// the seed rows 123456 and 124325 and inquires only the bill type tel. Its mock applies only in dev
// mode, with debug.mocks.karashop.enable and providers.karashop.enabled true; enabled false wins
// over the mock (one warn line).
func TestNewProviderRegistryKarashop(t *testing.T) {
	req := provider.InquiryRequest{BillType: provider.BillTypeTel, UserID: "u", Dialer: "989121234567", InquiryID: "982188776655"}
	on := "providers: {karashop: {enabled: true, timeout: 300ms}}\n"
	cases := []struct {
		name     string
		yml      string
		wantMock bool
		wantErr  bool // a mock error answer
		wantWarn string
	}{
		{"dev, mock success", "mode: dev\n" + on + "debug: {mocks: {karashop: {enable: true, return_type: success}}}", true, false, karashopMockWarn},
		{"dev, mock error", "mode: dev\n" + on + "debug: {mocks: {karashop: {enable: true, return_type: error}}}", true, true, karashopMockWarn},
		{"dev, mock on, disabled", "mode: dev\ndebug: {mocks: {karashop: {enable: true}}}", false, false, karashopDisabledWarn},
		{"dev, mock off, disabled", "mode: dev\n", false, false, karashopDisabledWarn},
		{"stage, enable ignored, disabled", "mode: stage\ndebug: {mocks: {karashop: {enable: true}}}", false, false, karashopDisabledWarn},
		{"prod, mock ignored, enabled, no credential", "mode: prod\n" + on + "debug: {mocks: {karashop: {enable: true}}}", false, false, ""},
		{"prod, enabled, local URL (nothing listens)", "mode: prod\nproviders: {karashop: {enabled: true, code: c, password: p, bill_inquiry_url: \"http://127.0.0.1:1/k\", timeout: 2s}}", false, false, ""},
	}
	for _, c := range cases {
		log := &liveLogger{}
		reg, err := newProviderRegistry(registryConfig(t, c.yml), log)
		if !assert.NoError(t, err, c.name) || !assert.NotNil(t, reg, c.name) {
			continue
		}
		p, ok := reg.Get(karashop.Name)
		if !assert.True(t, ok, c.name) || !assert.NotNil(t, p, c.name) {
			continue
		}
		assert.Equal(t, "karashop", p.Name(), c.name)
		assert.True(t, p.SupportsInquiry(provider.BillTypeTel), c.name)
		for _, bt := range []string{provider.BillTypeWater, provider.BillTypeGas, provider.BillTypeMobile, "telmidterm"} {
			assert.False(t, p.SupportsInquiry(bt), c.name+": "+bt)
		}
		resp, err := p.Inquiry(context.Background(), req)
		switch {
		case c.wantMock && !c.wantErr:
			if assert.NoError(t, err, c.name) && assert.NotNil(t, resp, c.name) && assert.Len(t, resp.Inquiries, 2, c.name) {
				assert.Equal(t, uint64(karashop.MockMidTermAmount), resp.Inquiries[0].Amount, c.name)
				assert.Equal(t, uint64(karashop.MockFinalTermAmount), resp.Inquiries[1].Amount, c.name)
			}
			assert.Equal(t, 1, log.count("info", mockInfoLine), c.name)
		case c.wantMock && c.wantErr:
			assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
			assert.Nil(t, resp, c.name)
		default:
			assert.ErrorIs(t, err, provider.ErrProviderDown, c.name+": the real adapter, no server")
			assert.Equal(t, 0, log.count("info", mockInfoLine), c.name)
		}
		assert.Equal(t, btoi(c.wantWarn == karashopMockWarn), log.count("warn", karashopMockWarn), c.name)
		assert.Equal(t, btoi(c.wantWarn == karashopDisabledWarn), log.count("warn", karashopDisabledWarn), c.name)
	}
}

// TestNewTel: a nil flow gives a Tel service whose Inquiry answers Unimplemented.
func TestNewTel(t *testing.T) {
	tel := newTel(nil, nil)
	if assert.NotNil(t, tel) {
		_, err := tel.Inquiry(context.Background(), &pb.InquiryTelRequest{Phone: "982188776655"})
		assert.Equal(t, codes.Unimplemented, status.Code(err))
	}
}
