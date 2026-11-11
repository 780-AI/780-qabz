package app

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"

	jspb "qabz/api/pb/jiringSwitch"
	pspb "qabz/api/pb/parsian-v2"
	"qabz/provider"
	"qabz/provider/jiring"
	"qabz/provider/parsianproxy"
)

const (
	jiringMockWarn      = "provider mock enabled (debug.mocks.jiring_switch): development only, no call to the Jiring Switch backend"
	jiringDisabledWarn  = "providers.jiring_switch.enabled is false: every inquiry to Jiring Switch fails as provider down; the Jiring Switch mock does not apply"
	parsianMockWarn     = "provider mock enabled (debug.mocks.parsian): development only, no call to the Parsian backend"
	parsianDisabledWarn = "providers.parsian.enabled is false: every inquiry to Parsian fails as provider down; the Parsian mock does not apply"
)

var mciRegistryReq = provider.InquiryRequest{BillType: "mobile", UserID: "u", Dialer: "989121234567", InquiryID: "989121112233"}

// TestNewProviderRegistryMCI (task 036): the Jiring Switch and the Parsian adapter are registered
// under the names of seed rows 28 and 39. Each mock applies only in dev mode, with its mock key and
// its enabled key true; enabled false wins over the mock (one warn line). The config key of Jiring
// Switch is jiring_switch (the providers.name has a hyphen).
func TestNewProviderRegistryMCI(t *testing.T) {
	type adapterCase struct {
		name, key            string
		mockWarn, disWarn    string
		midAmount, finAmount uint64
	}
	adapters := []adapterCase{
		{jiring.Name, "jiring_switch", jiringMockWarn, jiringDisabledWarn, jiring.MockMidtermAmount, jiring.MockFinalTermAmount},
		{parsianproxy.Name, "parsian", parsianMockWarn, parsianDisabledWarn, parsianproxy.MockMidtermAmount, parsianproxy.MockFinalTermAmount},
	}
	for _, a := range adapters {
		on := "providers: {" + a.key + ": {enabled: true, timeout: 300ms}}\n"
		cases := []struct {
			name     string
			yml      string
			wantMock bool
			wantErr  bool // a mock error answer
			wantWarn string
		}{
			{"dev, mock success", "mode: dev\n" + on + "debug: {mocks: {" + a.key + ": {enable: true, return_type: success}}}", true, false, a.mockWarn},
			{"dev, mock error", "mode: dev\n" + on + "debug: {mocks: {" + a.key + ": {enable: true, return_type: error}}}", true, true, a.mockWarn},
			{"dev, mock on, disabled", "mode: dev\ndebug: {mocks: {" + a.key + ": {enable: true}}}", false, false, a.disWarn},
			{"dev, mock off, disabled", "mode: dev\n", false, false, a.disWarn},
			{"stage, enable ignored, disabled", "mode: stage\ndebug: {mocks: {" + a.key + ": {enable: true}}}", false, false, a.disWarn},
			{"prod, mock ignored, enabled, no address", "mode: prod\n" + on + "debug: {mocks: {" + a.key + ": {enable: true}}}", false, false, ""},
			{"prod, enabled, local address (nothing listens)", "mode: prod\nproviders: {" + a.key + ": {enabled: true, grpc_address: \"127.0.0.1:1\", timeout: 2s}}", false, false, ""},
		}
		for _, c := range cases {
			name := a.name + ": " + c.name
			log := &liveLogger{}
			reg, err := newProviderRegistry(registryConfig(t, c.yml), log)
			if !assert.NoError(t, err, name) || !assert.NotNil(t, reg, name) {
				continue
			}
			p, ok := reg.Get(a.name)
			if !assert.True(t, ok, name) || !assert.NotNil(t, p, name) {
				continue
			}
			assert.Equal(t, a.name, p.Name(), name)
			// The registered value (the mock too) inquires only the MCI bill: the seed rows that
			// pick parsian for water and gas are answered as "no adapter".
			assert.True(t, p.SupportsInquiry(provider.BillTypeMobile), name)
			for _, bt := range []string{provider.BillTypeWater, provider.BillTypeGas, provider.BillTypeElectricity, provider.BillTypeIrancell} {
				assert.False(t, p.SupportsInquiry(bt), name+": "+bt)
			}
			for _, other := range []string{"qabzino", "tavanir", "irancell", jiring.Name, parsianproxy.Name} {
				_, ok = reg.Get(other)
				assert.True(t, ok, name+": "+other+" is registered")
			}
			resp, err := p.Inquiry(context.Background(), mciRegistryReq)
			switch {
			case c.wantMock && !c.wantErr:
				if assert.NoError(t, err, name) && assert.NotNil(t, resp, name) && assert.Len(t, resp.Inquiries, 2, name) {
					assert.Equal(t, a.midAmount, resp.Inquiries[0].Amount, name)
					assert.Equal(t, a.finAmount, resp.Inquiries[1].Amount, name)
				}
				assert.Equal(t, 1, log.count("info", mockInfoLine), name)
			case c.wantMock && c.wantErr:
				assert.ErrorIs(t, err, provider.ErrProviderDown, name)
				assert.Nil(t, resp, name)
			default:
				assert.ErrorIs(t, err, provider.ErrProviderDown, name+": the real adapter, no service")
				assert.Equal(t, 0, log.count("info", mockInfoLine), name)
			}
			assert.Equal(t, btoi(c.wantWarn == a.mockWarn), log.count("warn", a.mockWarn), name)
			assert.Equal(t, btoi(c.wantWarn == a.disWarn), log.count("warn", a.disWarn), name)
		}
	}
}

// fakeJiringService is the internal gRPC server of the jiring-switch service, for the real adapter.
type fakeJiringService struct {
	jspb.UnimplementedInternalInquiryServiceServer
	reqs chan *jspb.BillInquiryRequest
}

func (f *fakeJiringService) BillInquiry(_ context.Context, in *jspb.BillInquiryRequest) (*jspb.BillInquiryResponse, error) {
	f.reqs <- in
	return &jspb.BillInquiryResponse{
		BaseResponse: &jspb.BillBaseResponse{Code: 200, Status: "success", Message: "ok", BillId: "9120000001"},
		Inquiries: []*jspb.BillInquiryResponse_Inquiry{
			{PayId: "70001", Amount: 70000, InquiryType: "midterm"},
			{PayId: "90001", Amount: 90000, InquiryType: "final_term"},
		},
	}, nil
}

// fakeParsianService is the internal gRPC server of the parsian-v2 service, for the real adapter.
type fakeParsianService struct {
	pspb.UnimplementedTopBillServer
	reqs chan *pspb.BillInquiryRequest
}

func (f *fakeParsianService) Inquiry(_ context.Context, in *pspb.BillInquiryRequest) (*pspb.BillInquiryResponse, error) {
	f.reqs <- in
	return &pspb.BillInquiryResponse{
		BaseResponse: &pspb.BillBaseResponse{Code: 200, Status: "success", Message: "ok", Tid: "ps-1", BillId: "9120000002"},
		Inquiries: []*pspb.BillInquiryResponse_Inquiry{
			{PayId: "70002", Amount: 71000, PaymentDeadline: "2026-12-01", InquiryType: "midterm"},
			{PayId: "90002", Amount: 91000, PaymentDeadline: "2026-12-01", InquiryType: "final_term"},
		},
	}, nil
}

// serveGRPC starts a local gRPC server with register and returns its address. The server stops at
// the end of the test.
func serveGRPC(t *testing.T, register func(*grpc.Server)) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	srv := grpc.NewServer()
	register(srv)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

// TestProviderRegistryMCIRealCall: with grpc_address, the adapters of the registry call the
// jiring-switch and the parsian-v2 service (local gRPC servers here) with the phone, the user and
// the dialer; Parsian also gets the bill type mobile_mci and a tracking and a reference ID.
func TestProviderRegistryMCIRealCall(t *testing.T) {
	jf := &fakeJiringService{reqs: make(chan *jspb.BillInquiryRequest, 1)}
	jAddr := serveGRPC(t, func(s *grpc.Server) { jspb.RegisterInternalInquiryServiceServer(s, jf) })
	pf := &fakeParsianService{reqs: make(chan *pspb.BillInquiryRequest, 1)}
	pAddr := serveGRPC(t, func(s *grpc.Server) { pspb.RegisterTopBillServer(s, pf) })

	reg, err := newProviderRegistry(registryConfig(t, "mode: prod\nproviders: {jiring_switch: {enabled: true, grpc_address: \""+jAddr+
		"\", timeout: 5s}, parsian: {enabled: true, grpc_address: \""+pAddr+"\", timeout: 5s}}"), nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, reg) {
		return
	}

	p, ok := reg.Get(jiring.Name)
	if assert.True(t, ok) && assert.NotNil(t, p) {
		resp, err := p.Inquiry(context.Background(), mciRegistryReq)
		if assert.NoError(t, err) && assert.NotNil(t, resp) && assert.Len(t, resp.Inquiries, 2) {
			assert.Equal(t, []string{provider.ResponseSuccess, "9120000001", ""}, []string{resp.Status, resp.BillID, resp.ProviderTraceID})
			assert.EqualValues(t, 70000, resp.Inquiries[0].Amount)
			assert.Equal(t, provider.InquiryTypeFinalTerm, resp.Inquiries[1].InquiryType)
		}
		select {
		case in := <-jf.reqs:
			assert.Equal(t, []string{"u", "989121234567", "989121112233"}, []string{in.GetUserId(), in.GetDialer(), in.GetInquiryId()})
		default:
			assert.Fail(t, "the jiring-switch service got no call")
		}
	}

	p, ok = reg.Get(parsianproxy.Name)
	if assert.True(t, ok) && assert.NotNil(t, p) {
		resp, err := p.Inquiry(context.Background(), mciRegistryReq)
		if assert.NoError(t, err) && assert.NotNil(t, resp) && assert.Len(t, resp.Inquiries, 2) {
			assert.Equal(t, []string{provider.ResponseSuccess, "9120000002", "ps-1"}, []string{resp.Status, resp.BillID, resp.ProviderTraceID})
			assert.EqualValues(t, 91000, resp.Inquiries[1].Amount)
			assert.Equal(t, "2026-12-01", resp.Inquiries[0].PaymentDeadline)
		}
		select {
		case in := <-pf.reqs:
			assert.Equal(t, []string{"u", "989121234567", "989121112233", parsianproxy.BillTypeMCI},
				[]string{in.GetUserId(), in.GetDialer(), in.GetInquiryId(), in.GetBillType()})
			assert.NotEmpty(t, in.GetTrackingId())
			assert.NotEmpty(t, in.GetReferenceId())
		default:
			assert.Fail(t, "the parsian-v2 service got no call")
		}
	}
}
