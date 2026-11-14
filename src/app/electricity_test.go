package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	grpcserver "qabz/entry/grpc"
	"qabz/provider"
	"qabz/provider/tavanir"
)

const (
	tavanirMockWarn     = "provider mock enabled (debug.mocks.tavanir): development only, no call to the Tavanir backend"
	tavanirDisabledWarn = "providers.external.tavanir.enabled is false: every inquiry to Tavanir fails as provider down; the Tavanir mock does not apply"
	mockInfoLine        = "provider mock: inquiry answered with no call to the provider"
)

// TestNewProviderRegistryTavanir (task 022, task 034): the registry has the Tavanir adapter next to
// Qabzino; its mock wraps it only in dev mode with debug.mocks.tavanir.enable and
// providers.external.tavanir.enabled (false wins over the mock), and does not depend on the Qabzino mock.
func TestNewProviderRegistryTavanir(t *testing.T) {
	req := provider.InquiryRequest{BillType: "electricity", UserID: "u", InquiryID: "1234567890125"}
	const on = "providers: {external: {tavanir: {enabled: true}}}\n"
	cases := []struct {
		name     string
		yml      string
		wantMock bool
		wantErr  bool // a mock error answer
		wantWarn string
	}{
		{"dev, mock success", "mode: dev\n" + on + "debug: {mocks: {tavanir: {enable: true, return_type: success}}}", true, false, tavanirMockWarn},
		{"dev, mock error", "mode: dev\n" + on + "debug: {mocks: {tavanir: {enable: true, return_type: error}}}", true, true, tavanirMockWarn},
		{"dev, mock on, tavanir disabled", "mode: dev\ndebug: {mocks: {tavanir: {enable: true}}}", false, false, tavanirDisabledWarn},
		{"dev, mock off, tavanir disabled", "mode: dev\ndebug: {mocks: {tavanir: {enable: false}}}", false, false, tavanirDisabledWarn},
		{"dev, only the Qabzino mock", "mode: dev\nproviders: {external: {qabzino: {enabled: true}}}\ndebug: {mocks: {qabzino: {enable: true}}}", false, false, tavanirDisabledWarn},
		{"stage, enable ignored, tavanir disabled", "mode: stage\ndebug: {mocks: {tavanir: {enable: true}}}", false, false, tavanirDisabledWarn},
		{"prod, mock ignored, enabled, no credentials", "mode: prod\n" + on + "debug: {mocks: {tavanir: {enable: true, return_type: error}}}", false, false, ""},
		{"prod, enabled, local endpoint (nothing listens)", "mode: prod\nproviders: {external: {tavanir: {enabled: true, username: u, password: p, auth_endpoint: \"http://127.0.0.1:1/a\", basic_branch_endpoint: \"http://127.0.0.1:1/b\", timeout: 300ms}}}", false, false, ""},
	}
	for _, c := range cases {
		log := &liveLogger{}
		reg, err := newProviderRegistry(registryConfig(t, c.yml), log)
		if !assert.NoError(t, err, c.name) || !assert.NotNil(t, reg, c.name) {
			continue
		}
		p, ok := reg.Get("tavanir")
		if !assert.True(t, ok, c.name) || !assert.NotNil(t, p, c.name) {
			continue
		}
		assert.Equal(t, "tavanir", p.Name(), c.name)
		_, ok = reg.Get("qabzino")
		assert.True(t, ok, c.name+": Qabzino stays registered")
		resp, err := p.Inquiry(context.Background(), req)
		switch {
		case c.wantMock && !c.wantErr:
			if assert.NoError(t, err, c.name) && assert.NotNil(t, resp, c.name) && assert.Len(t, resp.Inquiries, 1, c.name) && assert.NotNil(t, resp.Details, c.name) {
				assert.Equal(t, tavanir.MockPayID, resp.Inquiries[0].PayID, c.name)
				if assert.NotNil(t, resp.Details.Electricity, c.name) {
					assert.Equal(t, tavanir.MockCustomerName, resp.Details.Electricity.CustomerName, c.name)
				}
			}
			assert.Equal(t, 1, log.count("info", mockInfoLine), c.name)
		case c.wantMock && c.wantErr:
			assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
			assert.Nil(t, resp, c.name)
		default:
			assert.ErrorIs(t, err, provider.ErrProviderDown, c.name+": the real adapter, no backend")
			assert.Equal(t, 0, log.count("info", mockInfoLine), c.name)
		}
		assert.Equal(t, btoi(c.wantWarn == tavanirMockWarn), log.count("warn", tavanirMockWarn), c.name)
		assert.Equal(t, btoi(c.wantWarn == tavanirDisabledWarn), log.count("warn", tavanirDisabledWarn), c.name)
	}
	// A nil logger writes nothing and does not panic.
	reg, err := newProviderRegistry(registryConfig(t, "mode: dev\n"+on+"debug: {mocks: {tavanir: {enable: true}}}"), nil)
	if assert.NoError(t, err) && assert.NotNil(t, reg) {
		if p, ok := reg.Get("tavanir"); assert.True(t, ok) && assert.NotNil(t, p) {
			_, err = p.Inquiry(context.Background(), req)
			assert.NoError(t, err)
		}
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

// TestRegisterAdapterRules: a nil registry or adapter is an error; a second adapter with the same
// name is rejected; a bad return type of a mock that is on is an error.
func TestRegisterAdapterRules(t *testing.T) {
	a, err := tavanir.New(tavanir.Options{Timeout: time.Second})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		return
	}
	reg := provider.NewRegistry()
	assert.Error(t, registerAdapter(nil, a, "Tavanir", "notice", provider.MockSettings{}, tavanir.MockFixtures(), nil))
	assert.Error(t, registerAdapter(reg, nil, "Tavanir", "notice", provider.MockSettings{}, tavanir.MockFixtures(), nil))
	assert.Error(t, registerAdapter(reg, a, "Tavanir", "notice", provider.MockSettings{Enable: true, ReturnType: "maybe"}, tavanir.MockFixtures(), nil))
	assert.NoError(t, registerAdapter(reg, a, "Tavanir", "notice", provider.MockSettings{}, tavanir.MockFixtures(), nil))
	assert.Error(t, registerAdapter(reg, a, "Tavanir", "notice", provider.MockSettings{}, tavanir.MockFixtures(), nil), "registered already")
}

// TestDefaultBuilderServesElectricity: the composition root registers the Electricity service. With
// debug.auth.disable and no mock user the call has no user: PermissionDenied; an unregistered
// service would answer Unimplemented.
func TestDefaultBuilderServesElectricity(t *testing.T) {
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
	c := pb.NewElectricityClient(conn)
	_, err = c.Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = c.InquiryDocument(ctx, &pb.InquiryBillPayRequest{}) // positive control: a method that is not written yet
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
