package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/auth/authtest"
	"qabz/core/inquiry"
	"qabz/provider"
)

// fakeInquirer records each request and answers with res and err.
type fakeInquirer struct {
	mu   sync.Mutex
	reqs []inquiry.Request
	res  inquiry.Result
	err  error
	hook func()
}

func (f *fakeInquirer) Inquire(_ context.Context, req inquiry.Request) (inquiry.Result, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	hook := f.hook
	f.mu.Unlock()
	if hook != nil {
		hook()
	}
	return f.res, f.err
}

func (f *fakeInquirer) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func waterCtx() context.Context {
	return auth.WithClaims(context.Background(), auth.Claims{UserID: billsUser, Mobile: "989121234567"})
}

func TestWaterInquiryMapsRequestAndAnswer(t *testing.T) {
	f := &fakeInquirer{res: inquiry.Result{BillID: "1234567890125", Inquiries: []inquiry.Inquiry{
		{PayID: "first", Amount: 1, PaymentDeadline: "2029-01-01"},
		{PayID: "2500012345", Amount: 25000000, PaymentDeadline: "2030-01-01"},
	}}}
	svc := NewWater(WaterDeps{Inquiry: f})
	res, err := svc.Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1234567890125", PayId: "ignored", ParticipateCode: "ignored"})
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Len(t, f.reqs, 1) {
		return
	}
	assert.Equal(t, inquiry.Request{BillType: "water", InquiryID: "1234567890125", UserID: billsUser, Dialer: "989121234567"}, f.reqs[0],
		"payId and participateCode are ignored, as in bill")
	assert.Equal(t, "1234567890125", res.GetBillId())
	assert.Equal(t, "2500012345", res.GetPayId(), "the last inquiry is sent, as in bill")
	assert.Equal(t, uint32(25000000), res.GetPrice())
	assert.Equal(t, "2030-01-01", res.GetPaymentDeadline())
}

func TestWaterInquiryPaidBill(t *testing.T) {
	f := &fakeInquirer{res: inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: 0}}}}
	res, err := NewWater(WaterDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "x"})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(0), res.GetPrice())
		assert.Equal(t, "", res.GetPayId())
	}
}

func TestWaterInquiryNotAvailable(t *testing.T) {
	res, err := NewWater(WaterDeps{}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{})
	assert.Nil(t, res)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestWaterInquiryNoUser(t *testing.T) {
	f := &fakeInquirer{}
	svc := NewWater(WaterDeps{Inquiry: f})
	for name, ctx := range map[string]context.Context{
		"no claims":  context.Background(),
		"empty user": auth.WithClaims(context.Background(), auth.Claims{}),
	} {
		res, err := svc.Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1"})
		assert.Nil(t, res, name)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), name)
	}
	assert.Equal(t, 0, f.calls())
}

func TestWaterInquiryNilRequestAndContext(t *testing.T) {
	f := &fakeInquirer{err: inquiry.ErrInvalid}
	svc := NewWater(WaterDeps{Inquiry: f})
	_, err := svc.Inquiry(waterCtx(), nil)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, "", f.reqs[0].InquiryID)
	}
	var nilCtx context.Context
	_, err = svc.Inquiry(nilCtx, &pb.InquiryBillPayRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "a nil context has no user")
}

func TestWaterInquiryInvalidIsClientError(t *testing.T) {
	log := &errLog{}
	svc := NewWater(WaterDeps{Inquiry: &fakeInquirer{err: fmt.Errorf("%w: bill id", inquiry.ErrInvalid)}, Logger: log})
	_, err := svc.Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "bad"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, "invalid parameter", status.Convert(err).Message())
	assert.Empty(t, log.lines, "a client error writes no log line (conventions § 10)")
}

func TestWaterInquiryServerFaultsAreLogged(t *testing.T) {
	cause := errors.New("connection refused to 10.0.0.1:9000")
	cases := []struct {
		name     string
		err      error
		wantCode codes.Code
		wantText string
		wantLine string
	}{
		{"no provider", fmt.Errorf("pick provider: %w", provider.ErrNoProvider), codes.Unavailable, providerDownText, "water: no enabled provider for inquiry"},
		{"provider down", fmt.Errorf("qabzino: %w: %w", provider.ErrProviderDown, cause), codes.Unavailable, providerDownText, "water: provider inquiry failed"},
		{"no adapter", fmt.Errorf("%w %q", inquiry.ErrNoAdapter, "parsian"), codes.Unavailable, providerDownText, "water: provider inquiry failed"},
		{"database", fmt.Errorf("save request row: %w", cause), codes.Internal, "internal error", "water: inquiry failed"},
	}
	for _, c := range cases {
		log := &errLog{}
		svc := NewWater(WaterDeps{Inquiry: &fakeInquirer{err: c.err}, Logger: log})
		res, err := svc.Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1234567890125"})
		assert.Nil(t, res, c.name)
		assert.Equal(t, c.wantCode, status.Code(err), c.name)
		assert.Equal(t, c.wantText, status.Convert(err).Message(), c.name)
		assert.NotContains(t, err.Error(), "10.0.0.1", c.name+": the cause is not sent to the client")
		if assert.Len(t, log.lines, 1, c.name+": one error line") && assert.Len(t, log.kvs, 1, c.name) {
			assert.Equal(t, c.wantLine, log.lines[0], c.name)
			assert.Equal(t, []any{"error", c.err}, log.kvs[0], c.name+": the log has the cause")
		}
	}
	// No logger: no panic.
	_, err := NewWater(WaterDeps{Inquiry: &fakeInquirer{err: cause}}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestWaterInquiryProviderRefused(t *testing.T) {
	log := &errLog{}
	f := &fakeInquirer{err: fmt.Errorf("x: %w", &inquiry.RefusedError{Message: "bill not found"})}
	_, err := NewWater(WaterDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, "bill not found", status.Convert(err).Message())
	assert.Empty(t, log.lines, "a refused inquiry is not a server fault")

	f.err = &inquiry.RefusedError{}
	_, err = NewWater(WaterDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, "inquiry failed", status.Convert(err).Message(), "a default text when the provider sent none")
}

func TestWaterInquiryClientLeaves(t *testing.T) {
	log := &errLog{}
	ctx, cancel := context.WithCancel(waterCtx())
	f := &fakeInquirer{err: errors.Join(provider.ErrProviderDown, context.Canceled), hook: cancel}
	_, err := NewWater(WaterDeps{Inquiry: f, Logger: log}).Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Empty(t, log.lines, "a client that left is not a server fault")

	// A context that is canceled before the call: the flow is not called.
	f2 := &fakeInquirer{}
	ctx2, cancel2 := context.WithCancel(waterCtx())
	cancel2()
	_, err = NewWater(WaterDeps{Inquiry: f2}).Inquiry(ctx2, &pb.InquiryBillPayRequest{})
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Equal(t, 0, f2.calls())
}

func TestWaterInquiryAmountAboveUint32(t *testing.T) {
	log := &errLog{}
	f := &fakeInquirer{res: inquiry.Result{Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: math.MaxUint32 + 1}}}}
	res, err := NewWater(WaterDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Nil(t, res)
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Len(t, log.lines, 1)
	f.res = inquiry.Result{Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: math.MaxUint32}}}
	res, err = NewWater(WaterDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(math.MaxUint32), res.GetPrice(), "the largest value that fits")
	}
	// No logger: no panic.
	f.res = inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: math.MaxUint32 + 1}}}
	_, err = NewWater(WaterDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "1"})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestRegisterWaterRejectsNil(t *testing.T) {
	assert.Nil(t, RegisterWater(nil))
	var typedNil *Water
	assert.Nil(t, RegisterWater(typedNil))
	assert.NotNil(t, RegisterWater(NewWater(WaterDeps{})))
}

// TestWaterInquiryOverGRPCWithAuth: through a real gRPC server with the JWT check on, the user and
// the mobile of the token reach the flow; a call with no token is rejected before it.
func TestWaterInquiryOverGRPCWithAuth(t *testing.T) {
	a, err := NewAuth(AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	f := &fakeInquirer{res: inquiry.Result{BillID: "b", Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: 5, PaymentDeadline: "2030-01-01"}}}}
	s, err := NewWithOptions("127.0.0.1:0", a.ServerOptions(), RegisterWater(NewWater(WaterDeps{Inquiry: f})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	conn, err := grpc.NewClient(s.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := pb.NewWaterClient(conn)

	_, err = c.Inquiry(context.Background(), &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, 0, f.calls())

	token := authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	res, err := c.Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(5), res.GetPrice())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, authtest.UserID, f.reqs[0].UserID)
	}
}
