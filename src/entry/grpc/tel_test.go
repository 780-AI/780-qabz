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

// Task 025: POST /v1/tel/inquiry. The request is bill's InquiryTelRequest and the response bill's
// InquiryPhoneResponse, unchanged.

const (
	telPhone    = "982188776655"
	telBill     = "2101234560040" // a valid landline bill ID
	telMidPay   = "12000035"
	telFinalPay = "35000042"
)

// telResult is an answer of the landline inquiry; each part has its own bill ID.
func telResult() inquiry.Result {
	return inquiry.Result{BillID: "2101234560041", Inquiries: []inquiry.Inquiry{
		{BillID: telBill, PayID: telMidPay, Amount: 120000, InquiryType: provider.InquiryTypeMidterm},
		{BillID: "2101234560041", PayID: telFinalPay, Amount: 350000, InquiryType: provider.InquiryTypeFinalTerm},
	}}
}

func TestTelInquiryMapsRequestAndAnswer(t *testing.T) {
	f := &fakeInquirer{res: telResult()}
	log := &errLog{}
	res, err := NewTel(TelDeps{Inquiry: f, Raw: &fakeRaw{}, Logger: log}).Inquiry(waterCtx(), &pb.InquiryTelRequest{Phone: telPhone})
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Len(t, f.reqs, 1) {
		return
	}
	assert.Equal(t, inquiry.Request{BillType: "tel", InquiryID: telPhone, UserID: billsUser, Dialer: "989121234567"}, f.reqs[0],
		"the inquiry ID is the phone; the dialer is the mobile of the token")
	assert.Equal(t, []string{telBill, telMidPay, "2101234560041", telFinalPay},
		[]string{res.GetMidTerm().GetBillId(), res.GetMidTerm().GetPayId(), res.GetFullTerm().GetBillId(), res.GetFullTerm().GetPayId()},
		"each part has the bill ID of its own inquiry")
	assert.Equal(t, []uint32{120000, 350000}, []uint32{res.GetMidTerm().GetPrice(), res.GetFullTerm().GetPrice()})
	assert.EqualValues(t, 0, res.GetFullTerm().GetProviderId(), "providerId is not set, as in bill")
	assert.Empty(t, log.lines)
}

// TestTelInquiryPhoneForms: the phone is converted with bill's NormalizePhoneNumber before the
// check: the 98 form, the 0 form (11 bytes) and ten digits are one inquiry ID. A phone that is then
// not "98" and ten digits is invalid (no flow call, no log line).
func TestTelInquiryPhoneForms(t *testing.T) {
	for _, phone := range []string{"982188776655", "02188776655", "2188776655"} {
		f := &fakeInquirer{res: telResult()}
		_, err := NewTel(TelDeps{Inquiry: f}).Inquiry(waterCtx(), &pb.InquiryTelRequest{Phone: phone})
		if assert.NoError(t, err, phone) && assert.Len(t, f.reqs, 1, phone) {
			assert.Equal(t, telPhone, f.reqs[0].InquiryID, phone)
		}
	}
	for _, phone := range []string{"021887766", "+982188776655", "0098 21 8877 6655", "۰۲۱۸۸۷۷۶۶۵۵", "98218877665", "9821887766551", "abcdefghijkl"} {
		f := &fakeInquirer{res: telResult()}
		log := &errLog{}
		_, err := NewTel(TelDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), &pb.InquiryTelRequest{Phone: phone})
		assert.Equal(t, codes.InvalidArgument, status.Code(err), phone)
		assert.Equal(t, "invalid parameter", status.Convert(err).Message(), phone)
		assert.Equal(t, 0, f.calls(), phone)
		assert.Empty(t, log.lines, phone)
	}
}

// TestTelInquiryRequestShapes (bill: validateInquiryTelRequest): phone or billId, and phone or
// payId; a billId that is sent is valid. billId and payId together are a raw inquiry.
func TestTelInquiryRequestShapes(t *testing.T) {
	cases := []struct {
		name     string
		req      *pb.InquiryTelRequest
		wantCode codes.Code
		wantFlow int
		wantRaw  int
	}{
		{"phone", &pb.InquiryTelRequest{Phone: telPhone}, codes.OK, 1, 0},
		{"phone and a valid billId", &pb.InquiryTelRequest{Phone: telPhone, BillId: telBill}, codes.OK, 1, 0},
		{"phone and payId", &pb.InquiryTelRequest{Phone: telPhone, PayId: telMidPay}, codes.OK, 1, 0},
		{"billId and payId", &pb.InquiryTelRequest{BillId: telBill, PayId: telMidPay}, codes.OK, 0, 1},
		{"phone, billId and payId", &pb.InquiryTelRequest{Phone: telPhone, BillId: telBill, PayId: telMidPay}, codes.OK, 0, 1},
		{"a bad phone with billId and payId (raw, as bill)", &pb.InquiryTelRequest{Phone: "x", BillId: telBill, PayId: telMidPay}, codes.OK, 0, 1},
		{"nothing", &pb.InquiryTelRequest{}, codes.InvalidArgument, 0, 0},
		{"only billId", &pb.InquiryTelRequest{BillId: telBill}, codes.InvalidArgument, 0, 0},
		{"only payId", &pb.InquiryTelRequest{PayId: telMidPay}, codes.InvalidArgument, 0, 0},
		{"phone and a bad billId", &pb.InquiryTelRequest{Phone: telPhone, BillId: "12345"}, codes.InvalidArgument, 0, 0},
		{"a bad billId with payId", &pb.InquiryTelRequest{BillId: "12345", PayId: telMidPay}, codes.InvalidArgument, 0, 0},
		{"nil request", nil, codes.InvalidArgument, 0, 0},
	}
	for _, c := range cases {
		f := &fakeInquirer{res: telResult()}
		raw := &fakeRaw{res: inquiry.RawResult{BillID: telBill, PayID: telMidPay, Amount: 120000, BillType: "tel"}}
		_, err := NewTel(TelDeps{Inquiry: f, Raw: raw}).Inquiry(waterCtx(), c.req)
		assert.Equal(t, c.wantCode, status.Code(err), c.name)
		assert.Equal(t, c.wantFlow, f.calls(), c.name)
		assert.Len(t, raw.calls, c.wantRaw, c.name)
	}
}

// TestTelRawInquiry: the answer of a raw inquiry has only fullTerm (price, payId, billId; no
// deadline) and an empty midTerm, as bill; the metric label is tel.
func TestTelRawInquiry(t *testing.T) {
	raw := &fakeRaw{res: inquiry.RawResult{BillID: telBill, PayID: telFinalPay, Amount: 350000, BillType: "tel"}}
	f := &fakeInquirer{}
	res, err := NewTel(TelDeps{Inquiry: f, Raw: raw}).Inquiry(waterCtx(), &pb.InquiryTelRequest{BillId: telBill, PayId: telFinalPay})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, []string{telBill, telFinalPay, ""}, []string{res.GetFullTerm().GetBillId(), res.GetFullTerm().GetPayId(), res.GetFullTerm().GetPaymentDeadline()})
		assert.Equal(t, uint32(350000), res.GetFullTerm().GetPrice())
		if assert.NotNil(t, res.GetMidTerm()) {
			assert.Equal(t, []string{"", "", ""}, []string{res.GetMidTerm().GetBillId(), res.GetMidTerm().GetPayId(), res.GetMidTerm().GetPaymentDeadline()})
			assert.Zero(t, res.GetMidTerm().GetPrice())
		}
	}
	assert.Equal(t, [][3]string{{"tel", telBill, telFinalPay}}, raw.calls)
	assert.Equal(t, 0, f.calls())
}

// TestTelRawInquiryErrors: the errors of the gas raw inquiry (answerRawInquiry); a server fault has
// one error line with the prefix "tel:"; no Raw is Unimplemented.
func TestTelRawInquiryErrors(t *testing.T) {
	cases := []struct {
		name     string
		res      inquiry.RawResult
		err      error
		wantCode codes.Code
		wantText string
		wantLog  []string
	}{
		{"bill ID", inquiry.RawResult{}, inquiry.ErrInvalidBillID, codes.InvalidArgument, "شناسه قبض نامعتبر است", nil},
		{"pay ID", inquiry.RawResult{}, inquiry.ErrInvalidPayID, codes.InvalidArgument, "شناسه پرداخت نامعتبر است", nil},
		{"bill type", inquiry.RawResult{}, inquiry.ErrUnknownBillType, codes.InvalidArgument, "نوع قبض نامعتبر است", nil},
		{"amount above the price field", inquiry.RawResult{Amount: math.MaxUint32 + 1}, nil, codes.InvalidArgument, "شناسه پرداخت نامعتبر است", nil},
		{"other", inquiry.RawResult{}, errors.New("broken"), codes.Internal, "internal error", []string{"tel: raw inquiry failed"}},
	}
	for _, c := range cases {
		log := &errLog{}
		res, err := NewTel(TelDeps{Inquiry: &fakeInquirer{}, Raw: &fakeRaw{res: c.res, err: c.err}, Logger: log}).Inquiry(waterCtx(),
			&pb.InquiryTelRequest{BillId: telBill, PayId: telFinalPay})
		assert.Nil(t, res, c.name)
		assert.Equal(t, c.wantCode, status.Code(err), c.name)
		assert.Equal(t, c.wantText, status.Convert(err).Message(), c.name)
		assert.Equal(t, c.wantLog, log.lines, c.name)
	}
	_, err := NewTel(TelDeps{Inquiry: &fakeInquirer{}}).Inquiry(waterCtx(), &pb.InquiryTelRequest{BillId: telBill, PayId: telFinalPay})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

// TestTelInquiryErrors: the error mapping of Water.Inquiry with the log prefix "tel:".
func TestTelInquiryErrors(t *testing.T) {
	cause := errors.New("dial 10.0.0.1")
	cases := []struct {
		name     string
		err      error
		wantCode codes.Code
		wantText string
		wantLine []string
	}{
		{"no provider (also another provider picked)", fmt.Errorf("pick: %w", provider.ErrNoProvider), codes.Unavailable, providerDownText, []string{"tel: no enabled provider for inquiry"}},
		{"provider down", fmt.Errorf("karashop: %w: %w", provider.ErrProviderDown, cause), codes.Unavailable, providerDownText, []string{"tel: provider inquiry failed"}},
		{"database", fmt.Errorf("save request row: %w", cause), codes.Internal, "internal error", []string{"tel: inquiry failed"}},
		{"refused", &inquiry.RefusedError{Message: "تلفن نامعتبر است"}, codes.FailedPrecondition, "تلفن نامعتبر است", nil},
		{"invalid", fmt.Errorf("%w: phone", inquiry.ErrInvalid), codes.InvalidArgument, "invalid parameter", nil},
	}
	for _, c := range cases {
		log := &errLog{}
		res, err := NewTel(TelDeps{Inquiry: &fakeInquirer{err: c.err}, Logger: log}).Inquiry(waterCtx(), &pb.InquiryTelRequest{Phone: telPhone})
		assert.Nil(t, res, c.name)
		assert.Equal(t, c.wantCode, status.Code(err), c.name)
		assert.Equal(t, c.wantText, status.Convert(err).Message(), c.name)
		if assert.Error(t, err, c.name) {
			assert.NotContains(t, err.Error(), "10.0.0.1", c.name)
		}
		assert.Equal(t, c.wantLine, log.lines, c.name)
	}
	// An amount above the uint32 price field: Internal with one line.
	log := &errLog{}
	f := &fakeInquirer{res: inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: math.MaxUint32 + 1, InquiryType: provider.InquiryTypeFinalTerm}}}}
	_, err := NewTel(TelDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), &pb.InquiryTelRequest{Phone: telPhone})
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, []string{"tel: inquiry amount does not fit the price field"}, log.lines)
}

// TestTelInquiryNotAvailableAndNoUser: no flow is Unimplemented; a call with no user is rejected; a
// canceled context is its own code.
func TestTelInquiryNotAvailableAndNoUser(t *testing.T) {
	_, err := NewTel(TelDeps{}).Inquiry(waterCtx(), &pb.InquiryTelRequest{Phone: telPhone})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	f := &fakeInquirer{res: telResult()}
	_, err = NewTel(TelDeps{Inquiry: f}).Inquiry(context.Background(), &pb.InquiryTelRequest{Phone: telPhone})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = NewTel(TelDeps{Inquiry: f}).Inquiry(auth.WithClaims(context.Background(), auth.Claims{}), &pb.InquiryTelRequest{Phone: telPhone})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, 0, f.calls())

	ctx, cancel := context.WithCancel(waterCtx())
	cancel()
	_, err = NewTel(TelDeps{Inquiry: f}).Inquiry(ctx, &pb.InquiryTelRequest{Phone: telPhone})
	assert.Equal(t, codes.Canceled, status.Code(err))
	_, err = NewTel(TelDeps{Inquiry: f}).Inquiry(nil, &pb.InquiryTelRequest{Phone: telPhone}) //nolint:staticcheck // the nil-context branch
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestRegisterTelRejectsNil(t *testing.T) {
	assert.Nil(t, RegisterTel(nil))
	var typedNil *Tel
	assert.Nil(t, RegisterTel(typedNil))
	assert.NotNil(t, RegisterTel(NewTel(TelDeps{})))
}

// TestTelInquiryOverGRPCWithAuth: through a real gRPC server with the JWT check on, the user of the
// token reaches the flow; a call with no token is rejected before it; the order methods are
// Unimplemented.
func TestTelInquiryOverGRPCWithAuth(t *testing.T) {
	a, err := NewAuth(AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	f := &fakeInquirer{res: telResult()}
	s, err := NewWithOptions("127.0.0.1:0", a.ServerOptions(), RegisterTel(NewTel(TelDeps{Inquiry: f})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	conn, err := grpc.NewClient(s.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := pb.NewTelClient(conn)

	_, err = c.Inquiry(context.Background(), &pb.InquiryTelRequest{Phone: telPhone})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Equal(t, 0, f.calls())

	token := authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
	res, err := c.Inquiry(ctx, &pb.InquiryTelRequest{Phone: telPhone})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(350000), res.GetFullTerm().GetPrice())
	}
	_, err = c.NewOrder(ctx, &pb.NewOrderTelRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	f.mu.Lock()
	defer f.mu.Unlock()
	if assert.Len(t, f.reqs, 1) {
		assert.Equal(t, authtest.UserID, f.reqs[0].UserID)
		assert.Equal(t, telPhone, f.reqs[0].InquiryID)
	}
}
