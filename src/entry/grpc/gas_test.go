package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"math"
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

// Task 023: POST /v1/gas/inquiry. The request and the response are bill's InquiryBillPayRequest and
// InquiryBillPayResponse, unchanged.

const (
	gasValidBill = "1234567890125" // a valid bill ID
	gasPay       = "3200012345"
	gasCode      = "0012345678" // a participate code: not a valid bill ID
)

func TestGasInquiryMapsRequestAndAnswer(t *testing.T) {
	f := &fakeInquirer{res: inquiry.Result{BillID: gasValidBill, Inquiries: []inquiry.Inquiry{
		{PayID: "first", Amount: 1, PaymentDeadline: "2029-01-01"},
		{PayID: gasPay, Amount: 32000000, PaymentDeadline: "2030-01-01"},
	}, Details: &provider.Details{Gas: &provider.GasDetails{City: "c", Kind: "k"}}}}
	svc := NewGas(GasDeps{Inquiry: f})
	res, err := svc.Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Len(t, f.reqs, 1) {
		return
	}
	assert.Equal(t, inquiry.Request{BillType: "gas", InquiryID: gasCode, UserID: billsUser, Dialer: "989121234567"}, f.reqs[0],
		"the inquiry ID is the participate code")
	assert.Equal(t, gasValidBill, res.GetBillId(), "the bill ID of the answer")
	assert.Equal(t, gasPay, res.GetPayId(), "the last inquiry is sent, as in bill")
	assert.Equal(t, uint32(32000000), res.GetPrice())
	assert.Equal(t, "2030-01-01", res.GetPaymentDeadline())
}

// TestGasInquiryRequestShapes: the request check of bill (validateInquiryRequest) and what each
// valid request does. The inquiry ID is always the participate code.
func TestGasInquiryRequestShapes(t *testing.T) {
	const (
		provider_ = "provider" // the flow is called with the participate code
		raw       = "raw"      // billId and payId: the raw inquiry; this test has no RawInquirer (GasDeps.Raw is nil), so Unimplemented (TestGasRawInquiry has one)
		invalid   = "invalid"  // InvalidArgument
	)
	cases := []struct {
		name                string
		billID, payID, code string
		want                string
	}{
		{"participate code only", "", "", gasCode, provider_},
		{"participate code and bill ID", gasValidBill, "", gasCode, provider_},
		{"participate code and pay ID", "", gasPay, gasCode, provider_},
		{"a participate code that is a bill ID", "", "", gasValidBill, provider_},
		{"bill ID and pay ID", gasValidBill, gasPay, "", raw},
		{"all three", gasValidBill, gasPay, gasCode, raw},
		{"nothing", "", "", "", invalid},
		{"bill ID only", gasValidBill, "", "", invalid},
		{"pay ID only", "", gasPay, "", invalid},
		{"participate code and a bad bill ID", "12345", "", gasCode, invalid},
		{"a bad bill ID and pay ID", "1234567890124", gasPay, "", invalid},
		{"all three, a bad bill ID", "abc", gasPay, gasCode, invalid},
	}
	for _, c := range cases {
		log := &errLog{}
		f := &fakeInquirer{res: inquiry.Result{BillID: "b", Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: 5}}}}
		res, err := NewGas(GasDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(),
			&pb.InquiryBillPayRequest{BillId: c.billID, PayId: c.payID, ParticipateCode: c.code})
		assert.Empty(t, log.lines, c.name+": no log line")
		switch c.want {
		case provider_:
			if assert.NoError(t, err, c.name) && assert.NotNil(t, res, c.name) && assert.Len(t, f.reqs, 1, c.name) {
				assert.Equal(t, c.code, f.reqs[0].InquiryID, c.name)
				assert.Equal(t, "gas", f.reqs[0].BillType, c.name)
			}
		case raw:
			assert.Nil(t, res, c.name)
			assert.Equal(t, codes.Unimplemented, status.Code(err), c.name)
			assert.Equal(t, 0, f.calls(), c.name+": no provider for a raw inquiry")
		default:
			assert.Nil(t, res, c.name)
			assert.Equal(t, codes.InvalidArgument, status.Code(err), c.name)
			assert.Equal(t, "invalid parameter", status.Convert(err).Message(), c.name)
			assert.Equal(t, 0, f.calls(), c.name+": the flow is not called")
		}
	}
}

func TestValidGasInquiry(t *testing.T) {
	assert.True(t, validGasInquiry("", "", "c"))
	assert.True(t, validGasInquiry(gasValidBill, gasPay, ""))
	assert.True(t, validGasInquiry(gasValidBill, "", "c"))
	assert.False(t, validGasInquiry("", "", ""))
	assert.False(t, validGasInquiry(gasValidBill, "", ""))
	assert.False(t, validGasInquiry("", gasPay, ""))
	assert.False(t, validGasInquiry("12345", "", "c"))
}

func TestGasInquiryPaidBill(t *testing.T) {
	f := &fakeInquirer{res: inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: 0}}}}
	res, err := NewGas(GasDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(0), res.GetPrice())
		assert.Equal(t, "", res.GetPayId())
		assert.Equal(t, "", res.GetBillId())
	}
}

func TestGasInquiryNotAvailable(t *testing.T) {
	res, err := NewGas(GasDeps{}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Nil(t, res)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestGasInquiryNoUser(t *testing.T) {
	f := &fakeInquirer{}
	svc := NewGas(GasDeps{Inquiry: f})
	for name, ctx := range map[string]context.Context{
		"no claims":  context.Background(),
		"empty user": auth.WithClaims(context.Background(), auth.Claims{}),
	} {
		res, err := svc.Inquiry(ctx, &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
		assert.Nil(t, res, name)
		assert.Equal(t, codes.PermissionDenied, status.Code(err), name)
	}
	var nilCtx context.Context
	_, err := svc.Inquiry(nilCtx, &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "a nil context has no user")
	assert.Equal(t, 0, f.calls())
}

func TestGasInquiryNilRequest(t *testing.T) {
	f := &fakeInquirer{}
	_, err := NewGas(GasDeps{Inquiry: f}).Inquiry(waterCtx(), nil)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, 0, f.calls())
}

func TestGasInquiryFlowInvalidIsClientError(t *testing.T) {
	log := &errLog{}
	svc := NewGas(GasDeps{Inquiry: &fakeInquirer{err: fmt.Errorf("%w: user", inquiry.ErrInvalid)}, Logger: log})
	_, err := svc.Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, "invalid parameter", status.Convert(err).Message())
	assert.Empty(t, log.lines, "a client error writes no log line (conventions § 10)")
}

func TestGasInquiryServerFaultsAreLogged(t *testing.T) {
	cause := errors.New("connection refused to 10.0.0.1:9000")
	cases := []struct {
		name     string
		err      error
		wantCode codes.Code
		wantText string
		wantLine string
	}{
		{"no provider", fmt.Errorf("pick provider: %w", provider.ErrNoProvider), codes.Unavailable, providerDownText, "gas: no enabled provider for inquiry"},
		{"provider down", fmt.Errorf("qabzino: %w: %w", provider.ErrProviderDown, cause), codes.Unavailable, providerDownText, "gas: provider inquiry failed"},
		{"no adapter", fmt.Errorf("%w %q", inquiry.ErrNoAdapter, "parsian"), codes.Unavailable, providerDownText, "gas: provider inquiry failed"},
		{"database", fmt.Errorf("save request row: %w", cause), codes.Internal, "internal error", "gas: inquiry failed"},
	}
	for _, c := range cases {
		log := &errLog{}
		svc := NewGas(GasDeps{Inquiry: &fakeInquirer{err: c.err}, Logger: log})
		res, err := svc.Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
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
	_, err := NewGas(GasDeps{Inquiry: &fakeInquirer{err: cause}}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestGasInquiryProviderRefused(t *testing.T) {
	log := &errLog{}
	f := &fakeInquirer{err: fmt.Errorf("x: %w", &inquiry.RefusedError{Message: "code not found"})}
	_, err := NewGas(GasDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, "code not found", status.Convert(err).Message())
	assert.Empty(t, log.lines, "a refused inquiry is not a server fault")
}

func TestGasInquiryClientLeaves(t *testing.T) {
	log := &errLog{}
	ctx, cancel := context.WithCancel(waterCtx())
	f := &fakeInquirer{err: errors.Join(provider.ErrProviderDown, context.Canceled), hook: cancel}
	_, err := NewGas(GasDeps{Inquiry: f, Logger: log}).Inquiry(ctx, &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Empty(t, log.lines, "a client that left is not a server fault")

	f2 := &fakeInquirer{}
	ctx2, cancel2 := context.WithCancel(waterCtx())
	cancel2()
	_, err = NewGas(GasDeps{Inquiry: f2}).Inquiry(ctx2, &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Equal(t, 0, f2.calls())
}

func TestGasInquiryAmountAboveUint32(t *testing.T) {
	log := &errLog{}
	f := &fakeInquirer{res: inquiry.Result{Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: math.MaxUint32 + 1}}}}
	res, err := NewGas(GasDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Nil(t, res)
	assert.Equal(t, codes.Internal, status.Code(err))
	if assert.Len(t, log.lines, 1) {
		assert.Equal(t, "gas: inquiry amount does not fit the price field", log.lines[0])
	}
	f.res = inquiry.Result{Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: math.MaxUint32}}}
	res, err = NewGas(GasDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(math.MaxUint32), res.GetPrice(), "the largest value that fits")
	}
	// No logger: no panic.
	f.res = inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: math.MaxUint32 + 1}}}
	_, err = NewGas(GasDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestRegisterGasRejectsNil(t *testing.T) {
	assert.Nil(t, RegisterGas(nil))
	var typedNil *Gas
	assert.Nil(t, RegisterGas(typedNil))
	assert.NotNil(t, RegisterGas(NewGas(GasDeps{})))
}

// TestGasInquiryOverGRPCWithAuth: through a real gRPC server with the JWT check on, the user of the
// token reaches the flow; a call with no token is rejected before it; the order methods are
// Unimplemented.
func TestGasInquiryOverGRPCWithAuth(t *testing.T) {
	a, err := NewAuth(AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	f := &fakeInquirer{res: inquiry.Result{BillID: "b", Inquiries: []inquiry.Inquiry{{PayID: "p", Amount: 5, PaymentDeadline: "2030-01-01"}}}}
	s, err := NewWithOptions("127.0.0.1:0", a.ServerOptions(), RegisterGas(NewGas(GasDeps{Inquiry: f})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	conn, err := grpc.NewClient(s.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := pb.NewGasClient(conn)

	_, err = c.Inquiry(context.Background(), &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, 0, f.calls())

	token := authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	res, err := c.Inquiry(ctx, &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(5), res.GetPrice())
		assert.Equal(t, "b", res.GetBillId())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, authtest.UserID, f.reqs[0].UserID)
		assert.Equal(t, gasCode, f.reqs[0].InquiryID)
	}
}

// fakeRaw answers RawInquiry with res and err and records the calls.
type fakeRaw struct {
	calls [][3]string
	res   inquiry.RawResult
	err   error
}

func (f *fakeRaw) RawInquiry(endpointBillType, billID, payID string) (inquiry.RawResult, error) {
	f.calls = append(f.calls, [3]string{endpointBillType, billID, payID})
	return f.res, f.err
}

// TestGasRawInquiry: a request with billId and payId is answered by the raw inquiry, with or
// without a participate code: price, payId and billId, no paymentDeadline, no call of the flow.
func TestGasRawInquiry(t *testing.T) {
	for name, code := range map[string]string{"no participate code": "", "with a participate code (ignored)": gasCode} {
		log := &errLog{}
		f := &fakeInquirer{}
		raw := &fakeRaw{res: inquiry.RawResult{BillID: gasValidBill, PayID: gasPay, Amount: 32000000, BillType: "electricity"}}
		res, err := NewGas(GasDeps{Inquiry: f, Raw: raw, Logger: log}).Inquiry(waterCtx(),
			&pb.InquiryBillPayRequest{BillId: gasValidBill, PayId: gasPay, ParticipateCode: code})
		if assert.NoError(t, err, name) && assert.NotNil(t, res, name) {
			assert.Equal(t, uint32(32000000), res.GetPrice(), name)
			assert.Equal(t, gasPay, res.GetPayId(), name)
			assert.Equal(t, gasValidBill, res.GetBillId(), name)
			assert.Equal(t, "", res.GetPaymentDeadline(), name+": bill sends no deadline for a raw inquiry")
		}
		assert.Equal(t, [][3]string{{"gas", gasValidBill, gasPay}}, raw.calls, name)
		assert.Equal(t, 0, f.calls(), name+": the flow (provider, saved inquiries) is not called")
		assert.Empty(t, log.lines, name)
	}
}

// TestGasRawInquiryErrors: a raw inquiry that breaks a rule is InvalidArgument with the text of
// bill and no log line; any other error is Internal with one error line.
func TestGasRawInquiryErrors(t *testing.T) {
	cases := []struct {
		name     string
		res      inquiry.RawResult
		err      error
		wantCode codes.Code
		wantText string
		wantLog  int
	}{
		{"bill ID", inquiry.RawResult{}, inquiry.ErrInvalidBillID, codes.InvalidArgument, "شناسه قبض نامعتبر است", 0},
		{"pay ID", inquiry.RawResult{}, inquiry.ErrInvalidPayID, codes.InvalidArgument, "شناسه پرداخت نامعتبر است", 0},
		{"bill type", inquiry.RawResult{}, fmt.Errorf("x: %w", inquiry.ErrUnknownBillType), codes.InvalidArgument, "نوع قبض نامعتبر است", 0},
		{"amount above the price field", inquiry.RawResult{Amount: math.MaxUint32 + 1}, nil, codes.InvalidArgument, "شناسه پرداخت نامعتبر است", 0},
		{"other", inquiry.RawResult{}, errors.New("broken"), codes.Internal, "internal error", 1},
	}
	for _, c := range cases {
		log := &errLog{}
		f := &fakeInquirer{}
		res, err := NewGas(GasDeps{Inquiry: f, Raw: &fakeRaw{res: c.res, err: c.err}, Logger: log}).Inquiry(waterCtx(),
			&pb.InquiryBillPayRequest{BillId: gasValidBill, PayId: gasPay})
		assert.Nil(t, res, c.name)
		assert.Equal(t, c.wantCode, status.Code(err), c.name)
		assert.Equal(t, c.wantText, status.Convert(err).Message(), c.name)
		assert.Len(t, log.lines, c.wantLog, c.name)
		assert.Equal(t, 0, f.calls(), c.name)
	}
	// The largest price that fits, and no logger with a server fault: no panic.
	res, err := NewGas(GasDeps{Inquiry: &fakeInquirer{}, Raw: &fakeRaw{res: inquiry.RawResult{Amount: math.MaxUint32}}}).Inquiry(waterCtx(),
		&pb.InquiryBillPayRequest{BillId: gasValidBill, PayId: gasPay})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(math.MaxUint32), res.GetPrice())
	}
	_, err = NewGas(GasDeps{Inquiry: &fakeInquirer{}, Raw: &fakeRaw{err: errors.New("broken")}}).Inquiry(waterCtx(),
		&pb.InquiryBillPayRequest{BillId: gasValidBill, PayId: gasPay})
	assert.Equal(t, codes.Internal, status.Code(err))
	// The request check comes first: a bad bill ID never reaches the raw inquiry.
	raw := &fakeRaw{}
	_, err = NewGas(GasDeps{Inquiry: &fakeInquirer{}, Raw: raw}).Inquiry(waterCtx(), &pb.InquiryBillPayRequest{BillId: "12345", PayId: gasPay})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, "invalid parameter", status.Convert(err).Message())
	assert.Empty(t, raw.calls)
}
