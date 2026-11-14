package app

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	irpb "qabz/api/pb/irancell"
	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	grpcserver "qabz/entry/grpc"
	"qabz/provider"
	"qabz/provider/irancellproxy"
)

const (
	irancellMockWarn     = "provider mock enabled (debug.mocks.irancell): development only, no call to the Irancell backend"
	irancellDisabledWarn = "providers.internal.irancell.enabled is false: every inquiry to Irancell fails as provider down; the Irancell mock does not apply"
)

var irancellRegistryReq = provider.InquiryRequest{BillType: "irancell", UserID: "u", Dialer: "989121234567", InquiryID: "989351234567"}

// TestNewProviderRegistryIrancell (task 035): the Irancell adapter is registered under the name of
// seed row 38. The mock applies only in dev mode, with the mock key and providers.internal.irancell.enabled
// true; enabled false wins over the mock (one warn line).
func TestNewProviderRegistryIrancell(t *testing.T) {
	const on = "providers: {internal: {irancell: {enabled: true, timeout: 300ms}}}\n"
	cases := []struct {
		name     string
		yml      string
		wantMock bool
		wantErr  bool // a mock error answer
		wantWarn string
	}{
		{"dev, mock success", "mode: dev\n" + on + "debug: {mocks: {irancell: {enable: true, return_type: success}}}", true, false, irancellMockWarn},
		{"dev, mock error", "mode: dev\n" + on + "debug: {mocks: {irancell: {enable: true, return_type: error}}}", true, true, irancellMockWarn},
		{"dev, mock on, irancell disabled", "mode: dev\ndebug: {mocks: {irancell: {enable: true}}}", false, false, irancellDisabledWarn},
		{"dev, mock off, irancell disabled", "mode: dev\n", false, false, irancellDisabledWarn},
		{"stage, enable ignored, irancell disabled", "mode: stage\ndebug: {mocks: {irancell: {enable: true}}}", false, false, irancellDisabledWarn},
		{"prod, mock ignored, enabled, no address", "mode: prod\n" + on + "debug: {mocks: {irancell: {enable: true}}}", false, false, ""},
		{"prod, enabled, local address (nothing listens)", "mode: prod\nproviders: {internal: {irancell: {enabled: true, grpc_address: \"127.0.0.1:1\", timeout: 2s}}}", false, false, ""},
	}
	for _, c := range cases {
		log := &liveLogger{}
		reg, err := newProviderRegistry(registryConfig(t, c.yml), log)
		if !assert.NoError(t, err, c.name) || !assert.NotNil(t, reg, c.name) {
			continue
		}
		p, ok := reg.Get("irancell")
		if !assert.True(t, ok, c.name) || !assert.NotNil(t, p, c.name) {
			continue
		}
		assert.Equal(t, "irancell", p.Name(), c.name)
		for _, other := range []string{"qabzino", "tavanir"} {
			_, ok = reg.Get(other)
			assert.True(t, ok, c.name+": "+other+" stays registered")
		}
		resp, err := p.Inquiry(context.Background(), irancellRegistryReq)
		switch {
		case c.wantMock && !c.wantErr:
			if assert.NoError(t, err, c.name) && assert.NotNil(t, resp, c.name) && assert.Len(t, resp.Inquiries, 2, c.name) {
				assert.EqualValues(t, irancellproxy.MockMidtermAmount, resp.Inquiries[0].Amount, c.name)
				assert.EqualValues(t, irancellproxy.MockFinalTermAmount, resp.Inquiries[1].Amount, c.name)
			}
			assert.Equal(t, 1, log.count("info", mockInfoLine), c.name)
		case c.wantMock && c.wantErr:
			assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
			assert.Nil(t, resp, c.name)
		default:
			assert.ErrorIs(t, err, provider.ErrProviderDown, c.name+": the real adapter, no service")
			assert.Equal(t, 0, log.count("info", mockInfoLine), c.name)
		}
		assert.Equal(t, btoi(c.wantWarn == irancellMockWarn), log.count("warn", irancellMockWarn), c.name)
		assert.Equal(t, btoi(c.wantWarn == irancellDisabledWarn), log.count("warn", irancellDisabledWarn), c.name)
	}
}

// fakeIrancellService is the internal gRPC server of the irancell service, for the real adapter.
type fakeIrancellService struct {
	irpb.UnimplementedInternalServiceServer
	reqs chan *irpb.BillInquiryRequest
}

func (f *fakeIrancellService) BillInquiry(_ context.Context, in *irpb.BillInquiryRequest) (*irpb.BillInquiryResponse, error) {
	f.reqs <- in
	return &irpb.BillInquiryResponse{
		BaseResponse: &irpb.BillBaseResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceId: "tr-1"},
		Inquiries: []*irpb.BillInquiryResponse_Inquiry{
			{Amount: 70000, InquiryType: "midterm"},
			{Amount: 90000, InquiryType: "final_term"},
		},
	}, nil
}

// TestProviderRegistryIrancellRealCall: with providers.internal.irancell.grpc_address, the adapter of the
// registry calls the BillInquiry method of the irancell service (a local gRPC server here) with the
// phone, the user and the dialer.
func TestProviderRegistryIrancellRealCall(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	srv := grpc.NewServer()
	fake := &fakeIrancellService{reqs: make(chan *irpb.BillInquiryRequest, 1)}
	irpb.RegisterInternalServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	reg, err := newProviderRegistry(registryConfig(t, "mode: prod\nproviders: {internal: {irancell: {enabled: true, grpc_address: \""+lis.Addr().String()+"\", timeout: 5s}}}"), nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, reg) {
		return
	}
	p, ok := reg.Get("irancell")
	if !assert.True(t, ok) || !assert.NotNil(t, p) {
		return
	}
	resp, err := p.Inquiry(context.Background(), irancellRegistryReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) && assert.Len(t, resp.Inquiries, 2) {
		assert.Equal(t, provider.ResponseSuccess, resp.Status)
		assert.Equal(t, "tr-1", resp.ProviderTraceID)
		assert.EqualValues(t, 70000, resp.Inquiries[0].Amount)
		assert.Equal(t, provider.InquiryTypeFinalTerm, resp.Inquiries[1].InquiryType)
	}
	select {
	case in := <-fake.reqs:
		assert.Equal(t, []string{"u", "989121234567", "989351234567"}, []string{in.GetUserId(), in.GetDialer(), in.GetInquiryId()})
	default:
		assert.Fail(t, "the irancell service got no call")
	}
}

// TestNewPhoneWithoutFlow: no flow gives a service whose Inquiry is not available (no typed nil in
// the interface).
func TestNewPhoneWithoutFlow(t *testing.T) {
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"})
	svc := newPhone(nil, nil)
	if assert.NotNil(t, svc) {
		_, err := svc.Inquiry(ctx, &pb.InquiryPhoneRequest{Phone: "989351234567", Operator: pb.InquiryPhoneRequest_irancell})
		assert.Equal(t, codes.Unimplemented, status.Code(err))
	}
}

// TestDefaultBuilderServesPhone: the composition root registers the Phone service. With
// debug.auth.disable and no mock user the call has no user: PermissionDenied; a method that is not
// written yet answers Unimplemented.
func TestDefaultBuilderServesPhone(t *testing.T) {
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
	c := pb.NewPhoneClient(conn)
	_, err = c.Inquiry(ctx, &pb.InquiryPhoneRequest{Phone: "989351234567", Operator: pb.InquiryPhoneRequest_irancell})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = c.NewOrder(ctx, &pb.NewOrderPhoneRequest{}) // positive control: a method that is not written yet
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}
