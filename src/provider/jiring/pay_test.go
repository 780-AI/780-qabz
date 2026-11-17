package jiring

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/jiringSwitch"
	"qabz/provider"
)

func payRequest(billType, inquiryType string) provider.PaymentRequest {
	return provider.PaymentRequest{
		BillType: billType, InquiryType: inquiryType, OrderID: "o-1", UserID: "u-1", Dialer: "989121234567",
		InquiryID: "989121234567", BillID: "1234567809158", PayID: "45000080", Amount: 450000, RRN: "445577",
	}
}

// TestPayMCI (task 058): the fields of bill's JiringSwitchMCIMobile for both terms; provider_order_id
// is the day of the year in Tehran and the RRN; the answer as it came; the call has the timeout.
func TestPayMCI(t *testing.T) {
	for _, c := range []struct{ billType, inquiryType string }{
		{provider.BillTypeMobile, provider.InquiryTypeFinalTerm},
		{provider.BillTypeMobileMidterm, provider.InquiryTypeMidterm},
	} {
		f := &fakeClient{payResp: &pb.PayResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceId: "t-9"}}
		a := newAdapter(t, f)
		// 2026-12-31 21:00 UTC is 2027-01-01 00:30 in Tehran: day 1, not day 365.
		a.now = func() time.Time { return time.Date(2026, 12, 31, 21, 0, 0, 0, time.UTC) }
		resp, err := a.Pay(context.Background(), payRequest(c.billType, c.inquiryType))
		if !assert.NoError(t, err, c.billType) || !assert.NotNil(t, resp) {
			continue
		}
		assert.Equal(t, &provider.PaymentResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceID: "t-9"}, resp)
		if assert.Len(t, f.pays, 1) {
			assert.Equal(t, &pb.PayRequest{
				UserId: "u-1", Dialer: "989121234567", OrderId: "o-1", ProviderOrderId: "1445577", InquiryId: "989121234567",
				InquiryType: c.inquiryType, BillId: "1234567809158", PayId: "45000080", Amount: 450000, PaymentRrn: "445577",
			}, f.pays[0], c.billType)
		}
		assert.True(t, f.dl, "the call has a deadline")
	}
	for _, st := range []string{"failed", "pending", "other", ""} {
		f := &fakeClient{payResp: &pb.PayResponse{Code: 400, Status: st, Message: "m"}}
		resp, err := newAdapter(t, f).Pay(context.Background(), payRequest(provider.BillTypeMobile, provider.InquiryTypeFinalTerm))
		if assert.NoError(t, err, st) && assert.NotNil(t, resp) {
			assert.Equal(t, st, resp.Status, "the status as it came: the flow maps it")
		}
	}
}

// TestPayDownAndNotSent (task 058): every failure is ErrProviderDown; only the switch off and no
// address are ErrNotSent with a fake client; other bill types are not paid.
func TestPayDownAndNotSent(t *testing.T) {
	req := payRequest(provider.BillTypeMobile, provider.InquiryTypeFinalTerm)

	_, err := newAdapter(t, &fakeClient{payErr: errors.New("boom")}).Pay(context.Background(), req)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.NotErrorIs(t, err, provider.ErrNotSent)

	_, err = newAdapter(t, &fakeClient{}).Pay(context.Background(), req)
	assert.ErrorIs(t, err, provider.ErrProviderDown, "a nil answer")
	assert.NotErrorIs(t, err, provider.ErrNotSent, "a nil answer")

	f := &fakeClient{payResp: &pb.PayResponse{Status: "success"}}
	off, _ := New(Options{Enabled: false, Client: f, Timeout: time.Second})
	_, err = off.Pay(context.Background(), req)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, provider.ErrNotSent, "the switch is off")
	assert.ErrorIs(t, err, errDisabled)
	assert.Empty(t, f.pays, "the switch is off: no call")

	c, _ := Dial("")
	a, _ := New(Options{Enabled: true, Client: c, Timeout: time.Second})
	_, err = a.Pay(context.Background(), req)
	assert.ErrorIs(t, err, provider.ErrNotSent, "no address")
	assert.ErrorIs(t, err, errNoAddress)

	f = &fakeClient{block: true}
	a, _ = New(Options{Enabled: true, Client: f, Timeout: 20 * time.Millisecond})
	_, err = a.Pay(context.Background(), req)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotErrorIs(t, err, provider.ErrNotSent, "a timeout is not a fallback (operator rule)")

	f = &fakeClient{payResp: &pb.PayResponse{Status: "success"}}
	for _, bt := range []string{provider.BillTypeTel, provider.BillTypeIrancell, provider.BillTypeMobileMidTermRightel, ""} {
		_, err := newAdapter(t, f).Pay(context.Background(), payRequest(bt, ""))
		assert.ErrorIs(t, err, provider.ErrNotSupported, bt)
	}
	assert.Empty(t, f.pays)
	assert.True(t, SupportsPay(provider.BillTypeMobile))
	assert.True(t, SupportsPay(provider.BillTypeMobileMidterm))
	assert.False(t, SupportsPay(provider.BillTypeWater))

	var nilAdapter *Adapter
	_, err = nilAdapter.Pay(context.Background(), req)
	assert.Error(t, err)
	var nilCtx context.Context
	f = &fakeClient{payResp: &pb.PayResponse{Status: "success"}}
	_, err = newAdapter(t, f).Pay(nilCtx, req)
	assert.NoError(t, err, "a nil context is the background context")
}

// unavailableServer answers every payment with the gRPC status Unavailable: the call reached it.
type unavailableServer struct {
	pb.UnimplementedInternalInquiryServiceServer
}

func (unavailableServer) PayBill(context.Context, *pb.PayRequest) (*pb.PayResponse, error) {
	return nil, status.Error(codes.Unavailable, "jiring is busy")
}

// TestPayNotSentRealGRPC (task 058): with the real client of Dial, a refused connection is
// ErrNotSent; an Unavailable answer of a server that got the call is only ErrProviderDown.
func TestPayNotSentRealGRPC(t *testing.T) {
	req := payRequest(provider.BillTypeMobileMidterm, provider.InquiryTypeMidterm)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	closed := lis.Addr().String()
	assert.NoError(t, lis.Close())
	c, err := Dial(closed)
	if !assert.NoError(t, err) {
		return
	}
	a, _ := New(Options{Enabled: true, Client: c, Timeout: 5 * time.Second})
	_, err = a.Pay(context.Background(), req)
	assert.ErrorIs(t, err, provider.ErrNotSent, "a refused connection")
	assert.Equal(t, codes.Unavailable, status.Code(err))

	lis, err = net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	srv := grpc.NewServer()
	pb.RegisterInternalInquiryServiceServer(srv, unavailableServer{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	c, err = Dial(lis.Addr().String())
	if !assert.NoError(t, err) {
		return
	}
	a, _ = New(Options{Enabled: true, Client: c, Timeout: 5 * time.Second})
	_, err = a.Pay(context.Background(), req)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.NotErrorIs(t, err, provider.ErrNotSent, "the server answered: the call reached it")
}

// TestStatus (task 058): GetStatus with the order ID and the purchase type bill; the answer as it
// came with tid as the trace ID; the failures are ErrProviderDown (never ErrNotSent).
func TestStatus(t *testing.T) {
	f := &fakeClient{statusResp: &pb.GetStatusResponse{Status: "success", Message: "ok", Tid: "77"}}
	resp, err := newAdapter(t, f).Status(context.Background(), provider.StatusRequest{OrderID: "o-1", BillType: provider.BillTypeMobile})
	if assert.NoError(t, err) && assert.NotNil(t, resp) {
		assert.Equal(t, &provider.StatusResponse{Status: "success", Message: "ok", ProviderTraceID: "77"}, resp)
	}
	if assert.Len(t, f.statuses, 1) {
		assert.Equal(t, &pb.GetStatusRequest{OrderId: "o-1", PurchaseType: "bill"}, f.statuses[0])
	}
	assert.True(t, f.dl)

	for name, f := range map[string]*fakeClient{
		"error":   {statusErr: errors.New("boom")},
		"nil":     {},
		"timeout": {block: true},
	} {
		a, _ := New(Options{Enabled: true, Client: f, Timeout: 20 * time.Millisecond})
		_, err := a.Status(context.Background(), provider.StatusRequest{OrderID: "o-1"})
		assert.ErrorIs(t, err, provider.ErrProviderDown, name)
		assert.NotErrorIs(t, err, provider.ErrNotSent, name)
	}
	f = &fakeClient{statusResp: &pb.GetStatusResponse{Status: "success"}}
	off, _ := New(Options{Enabled: false, Client: f, Timeout: time.Second})
	_, err = off.Status(context.Background(), provider.StatusRequest{OrderID: "o-1"})
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, errDisabled)
	assert.Empty(t, f.statuses)
	c, _ := Dial("")
	a, _ := New(Options{Enabled: true, Client: c, Timeout: time.Second})
	_, err = a.Status(context.Background(), provider.StatusRequest{OrderID: "o-1"})
	assert.ErrorIs(t, err, errNoAddress)
	var nilAdapter *Adapter
	_, err = nilAdapter.Status(context.Background(), provider.StatusRequest{})
	assert.Error(t, err)
	var nilCtx context.Context
	_, err = newAdapter(t, &fakeClient{statusResp: &pb.GetStatusResponse{}}).Status(nilCtx, provider.StatusRequest{})
	assert.NoError(t, err)
}

// TestMockPayAndStatus (task 058): the success answers of the mock (a new value each call), and
// pay_return_type / status_return_type through provider.NewMock with no call to the service.
func TestMockPayAndStatus(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Pay) || !assert.NotNil(t, f.Status) {
		return
	}
	req := payRequest(provider.BillTypeMobile, provider.InquiryTypeFinalTerm)
	p := f.Pay(req)
	assert.Equal(t, &provider.PaymentResponse{Code: 200, Status: provider.StatusSuccess, Message: successMessage, ProviderTraceID: MockPayTrace}, p)
	p.Status = "x"
	assert.Equal(t, provider.StatusSuccess, f.Pay(req).Status)
	assert.Equal(t, &provider.StatusResponse{Status: provider.StatusSuccess, Message: successMessage, ProviderTraceID: MockPayTrace},
		f.Status(provider.StatusRequest{}))

	real := newAdapter(t, &fakeClient{payErr: errors.New("must not be called"), statusErr: errors.New("must not be called")})
	for _, rt := range []string{provider.MockReturnSuccess, provider.MockReturnFailed, provider.MockReturnPending} {
		m, err := provider.NewMock(real, provider.MockSettings{Enable: true, ReturnType: provider.MockReturnSuccess, PayReturnType: rt, StatusReturnType: rt}, f, nil)
		if !assert.NoError(t, err, rt) || !assert.NotNil(t, m) {
			continue
		}
		pr, perr := m.Pay(context.Background(), req)
		sr, serr := m.Status(context.Background(), provider.StatusRequest{OrderID: "o-1"})
		switch rt {
		case provider.MockReturnPending:
			assert.ErrorIs(t, perr, provider.ErrProviderDown)
			assert.NotErrorIs(t, perr, provider.ErrNotSent, "the mock pending is a timeout: no fallback")
			assert.ErrorIs(t, serr, provider.ErrProviderDown)
		default:
			if assert.NoError(t, perr) && assert.NotNil(t, pr) {
				assert.Equal(t, rt, pr.Status)
			}
			if assert.NoError(t, serr) && assert.NotNil(t, sr) {
				assert.Equal(t, rt, sr.Status)
			}
		}
	}
	assert.Empty(t, real.client.(*fakeClient).pays, "the mock calls nothing")
}
