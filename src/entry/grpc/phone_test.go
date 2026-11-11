package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/core/inquiry"
	"qabz/provider"
)

// Task 035: POST /v1/mobile/inquiry for the Irancell operator. The request and the response are
// bill's InquiryPhoneRequest and InquiryPhoneResponse, unchanged.

const (
	irPhone = "989351234567"
	irBill  = "1234567890125" // a valid bill ID
)

func irancellReq() *pb.InquiryPhoneRequest {
	return &pb.InquiryPhoneRequest{Phone: irPhone, Operator: pb.InquiryPhoneRequest_irancell}
}

func irancellResult() inquiry.Result {
	return inquiry.Result{Inquiries: []inquiry.Inquiry{
		{Amount: 150000, InquiryType: provider.InquiryTypeMidterm},
		{Amount: 450000, InquiryType: provider.InquiryTypeFinalTerm},
	}}
}

func TestPhoneInquiryMapsRequestAndAnswer(t *testing.T) {
	f := &fakeInquirer{res: irancellResult()}
	res, err := NewPhone(PhoneDeps{Inquiry: f}).Inquiry(waterCtx(), irancellReq())
	if !assert.NoError(t, err) || !assert.NotNil(t, res) || !assert.Len(t, f.reqs, 1) {
		return
	}
	assert.Equal(t, inquiry.Request{BillType: "irancell", InquiryID: irPhone, UserID: billsUser, Dialer: "989121234567"}, f.reqs[0],
		"the inquiry ID is the phone; the dialer is the mobile of the token")
	if assert.NotNil(t, res.GetMidTerm()) && assert.NotNil(t, res.GetFullTerm()) {
		assert.Equal(t, uint32(150000), res.GetMidTerm().GetPrice())
		assert.Equal(t, uint32(450000), res.GetFullTerm().GetPrice())
		for name, part := range map[string]*pb.InquiryPhoneResponse_PhoneInquiry{"midTerm": res.GetMidTerm(), "fullTerm": res.GetFullTerm()} {
			assert.Equal(t, []string{"", "", ""}, []string{part.GetBillId(), part.GetPayId(), part.GetPaymentDeadline()}, name+": Irancell gives no IDs and no deadline")
			assert.EqualValues(t, 0, part.GetProviderId(), name+": providerId is not set, as in bill")
		}
	}
}

// TestPhoneInquiryAnswerParts: the midterm inquiry is midTerm, every other type is fullTerm; a
// part with no inquiry is an empty object, never nil; the bill ID and the IDs of an inquiry are sent.
func TestPhoneInquiryAnswerParts(t *testing.T) {
	cases := []struct {
		name               string
		res                inquiry.Result
		wantMid, wantFull  uint32
		wantMidPay, wantID string
	}{
		{"both", irancellResult(), 150000, 450000, "", ""},
		{"final term first", inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: 7, InquiryType: provider.InquiryTypeFinalTerm}, {Amount: 3, InquiryType: provider.InquiryTypeMidterm}}}, 3, 7, "", ""},
		{"only mid-term", inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: 3, InquiryType: provider.InquiryTypeMidterm}}}, 3, 0, "", ""},
		{"only final term", inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: 7, InquiryType: provider.InquiryTypeFinalTerm}}}, 0, 7, "", ""},
		{"no debt", inquiry.Result{Inquiries: []inquiry.Inquiry{{InquiryType: provider.InquiryTypeMidterm}, {InquiryType: provider.InquiryTypeFinalTerm}}}, 0, 0, "", ""},
		{"no inquiry", inquiry.Result{}, 0, 0, "", ""},
		{"another type is the full term, as in bill", inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: 9, InquiryType: "regular"}}}, 0, 9, "", ""},
		{"IDs of a provider that has them", inquiry.Result{BillID: irBill, Inquiries: []inquiry.Inquiry{{Amount: 3, PayID: "p-mid", PaymentDeadline: "2030-01-01", InquiryType: provider.InquiryTypeMidterm}}}, 3, 0, "p-mid", irBill},
	}
	for _, c := range cases {
		res, err := NewPhone(PhoneDeps{Inquiry: &fakeInquirer{res: c.res}}).Inquiry(waterCtx(), irancellReq())
		if !assert.NoError(t, err, c.name) || !assert.NotNil(t, res, c.name) || !assert.NotNil(t, res.MidTerm, c.name) || !assert.NotNil(t, res.FullTerm, c.name) {
			continue
		}
		assert.Equal(t, c.wantMid, res.MidTerm.GetPrice(), c.name)
		assert.Equal(t, c.wantFull, res.FullTerm.GetPrice(), c.name)
		assert.Equal(t, c.wantMidPay, res.MidTerm.GetPayId(), c.name)
		assert.Equal(t, c.wantID, res.MidTerm.GetBillId(), c.name)
	}
}

// TestPhoneInquiryRequestCheck: the request check of bill (validateInquiryRequest). An invalid
// request is InvalidArgument with no log line and no call of the flow.
func TestPhoneInquiryRequestCheck(t *testing.T) {
	ir := pb.InquiryPhoneRequest_irancell
	cases := []struct {
		name  string
		req   *pb.InquiryPhoneRequest
		valid bool
	}{
		{"phone and operator", &pb.InquiryPhoneRequest{Phone: irPhone, Operator: ir}, true},
		{"with a valid bill ID and a pay ID", &pb.InquiryPhoneRequest{Phone: irPhone, Operator: ir, BillId: irBill, PayId: "anything"}, true},
		{"service is not read", &pb.InquiryPhoneRequest{Phone: irPhone, Operator: ir, Service: pb.InquiryPhoneRequest_mci}, true},
		{"nil request", nil, false},
		{"no phone", &pb.InquiryPhoneRequest{Operator: ir}, false},
		{"no phone, bill ID and pay ID", &pb.InquiryPhoneRequest{Operator: ir, BillId: irBill, PayId: "1"}, false},
		{"phone with 0", &pb.InquiryPhoneRequest{Phone: "09351234567", Operator: ir}, false},
		{"phone without 98", &pb.InquiryPhoneRequest{Phone: "9351234567", Operator: ir}, false},
		{"a landline", &pb.InquiryPhoneRequest{Phone: "982112345678", Operator: ir}, false},
		{"phone too long", &pb.InquiryPhoneRequest{Phone: irPhone + "1", Operator: ir}, false},
		{"a bad bill ID", &pb.InquiryPhoneRequest{Phone: irPhone, Operator: ir, BillId: "1234567890124"}, false},
		{"an operator number outside the enum", &pb.InquiryPhoneRequest{Phone: irPhone, Operator: 9}, false},
		{"a service number outside the enum", &pb.InquiryPhoneRequest{Phone: irPhone, Operator: ir, Service: 9}, false},
		{"the operator unknown", &pb.InquiryPhoneRequest{Phone: irPhone, Operator: pb.InquiryPhoneRequest_unknown}, false},
	}
	for _, c := range cases {
		log := &errLog{}
		f := &fakeInquirer{res: irancellResult()}
		res, err := NewPhone(PhoneDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), c.req)
		assert.Empty(t, log.lines, c.name+": no log line")
		if c.valid {
			if assert.NoError(t, err, c.name) && assert.NotNil(t, res, c.name) && assert.Len(t, f.reqs, 1, c.name) {
				assert.Equal(t, irPhone, f.reqs[0].InquiryID, c.name)
				assert.Equal(t, "irancell", f.reqs[0].BillType, c.name)
			}
			continue
		}
		assert.Nil(t, res, c.name)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), c.name)
		assert.Equal(t, "invalid parameter", status.Convert(err).Message(), c.name)
		assert.Equal(t, 0, f.calls(), c.name+": the flow is not called")
	}
}

func TestValidPhoneInquiry(t *testing.T) {
	assert.True(t, validPhoneInquiry(&pb.InquiryPhoneRequest{Phone: irPhone}), "the unknown operator is a value of the enum: the operator table rejects it")
	assert.True(t, validPhoneInquiry(&pb.InquiryPhoneRequest{Phone: irPhone, Operator: pb.InquiryPhoneRequest_unknown}))
	assert.False(t, validPhoneInquiry(nil))
	assert.False(t, validPhoneInquiry(&pb.InquiryPhoneRequest{}))
	assert.False(t, validPhoneInquiry(&pb.InquiryPhoneRequest{Phone: irPhone, Operator: -1}))
}

// TestPhoneInquiryOperators: the operator table. Irancell runs the flow; mci (also a request with
// no operator: the enum value 0) and rightel are not available yet (tasks 036 and 037).
func TestPhoneInquiryOperators(t *testing.T) {
	assert.Len(t, phoneOperators, 3, "mci, irancell, rightel; unknown has no bill type")
	assert.Equal(t, provider.BillTypeIrancell, phoneOperators[pb.InquiryPhoneRequest_irancell])
	for name, req := range map[string]*pb.InquiryPhoneRequest{
		"mci":                           {Phone: irPhone, Operator: pb.InquiryPhoneRequest_mci},
		"no operator":                   {Phone: irPhone},
		"no operator, service irancell": {Phone: irPhone, Service: pb.InquiryPhoneRequest_irancell},
		"rightel":                       {Phone: irPhone, Operator: pb.InquiryPhoneRequest_rightel},
	} {
		log := &errLog{}
		f := &fakeInquirer{res: irancellResult()}
		res, err := NewPhone(PhoneDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), req)
		assert.Nil(t, res, name)
		assert.Equal(t, codes.Unimplemented, status.Code(err), name)
		assert.Contains(t, status.Convert(err).Message(), "is not available yet", name)
		assert.Equal(t, 0, f.calls(), name+": the flow is not called, so no other provider answers")
		assert.Empty(t, log.lines, name+": no log line")
	}
}

func TestPhoneInquiryNotAvailableAndNoUser(t *testing.T) {
	_, err := NewPhone(PhoneDeps{}).Inquiry(waterCtx(), irancellReq())
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	f := &fakeInquirer{res: irancellResult()}
	svc := NewPhone(PhoneDeps{Inquiry: f})
	_, err = svc.Inquiry(context.Background(), irancellReq())
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "no claims")
	var nilCtx context.Context
	_, err = svc.Inquiry(nilCtx, irancellReq())
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "a nil context has no user")
	assert.Equal(t, 0, f.calls())

	ctx, cancel := context.WithCancel(waterCtx())
	cancel()
	_, err = svc.Inquiry(ctx, irancellReq())
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Equal(t, 0, f.calls())
}

// TestPhoneInquiryFailures: the answers of the flow errors are those of water. The log line of a
// server fault starts with the bill type; a client error writes none (conventions § 10).
func TestPhoneInquiryFailures(t *testing.T) {
	cause := errors.New("connection refused to 10.0.0.1:8084")
	cases := []struct {
		name     string
		err      error
		wantCode codes.Code
		wantText string
		wantLine string // "" for no log line
	}{
		{"flow invalid", fmt.Errorf("%w: phone", inquiry.ErrInvalid), codes.InvalidArgument, "invalid parameter", ""},
		{"refused", &inquiry.RefusedError{Message: "not an Irancell number"}, codes.FailedPrecondition, "not an Irancell number", ""},
		{"no provider", fmt.Errorf("pick provider: %w", provider.ErrNoProvider), codes.Unavailable, providerDownText, "irancell: no enabled provider for inquiry"},
		{"another provider was picked", fmt.Errorf("pick provider: %w: only %q may answer, the pick is %q", provider.ErrNoProvider, "irancell", "qabzino"), codes.Unavailable, providerDownText, "irancell: no enabled provider for inquiry"},
		{"provider down", fmt.Errorf("irancellproxy: %w: %w", provider.ErrProviderDown, cause), codes.Unavailable, providerDownText, "irancell: provider inquiry failed"},
		{"no adapter", fmt.Errorf("%w %q", inquiry.ErrNoAdapter, "irancell"), codes.Unavailable, providerDownText, "irancell: provider inquiry failed"},
		{"database", fmt.Errorf("save request row: %w", cause), codes.Internal, "internal error", "irancell: inquiry failed"},
	}
	for _, c := range cases {
		log := &errLog{}
		res, err := NewPhone(PhoneDeps{Inquiry: &fakeInquirer{err: c.err}, Logger: log}).Inquiry(waterCtx(), irancellReq())
		assert.Nil(t, res, c.name)
		assert.Equal(t, c.wantCode, status.Code(err), c.name)
		assert.Equal(t, c.wantText, status.Convert(err).Message(), c.name)
		if assert.Error(t, err, c.name) {
			assert.NotContains(t, err.Error(), "10.0.0.1", c.name+": the cause is not sent to the client")
		}
		if c.wantLine == "" {
			assert.Empty(t, log.lines, c.name)
		} else if assert.Len(t, log.lines, 1, c.name+": one error line") && assert.Len(t, log.kvs, 1, c.name) {
			assert.Equal(t, c.wantLine, log.lines[0], c.name)
			assert.Equal(t, []any{"error", c.err}, log.kvs[0], c.name+": the log has the cause")
		}
	}
	// No logger: no panic.
	_, err := NewPhone(PhoneDeps{Inquiry: &fakeInquirer{err: cause}}).Inquiry(waterCtx(), irancellReq())
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestPhoneInquiryAmountAboveUint32(t *testing.T) {
	log := &errLog{}
	f := &fakeInquirer{res: inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: math.MaxUint32 + 1, InquiryType: provider.InquiryTypeFinalTerm}}}}
	res, err := NewPhone(PhoneDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), irancellReq())
	assert.Nil(t, res)
	assert.Equal(t, codes.Internal, status.Code(err))
	if assert.Len(t, log.lines, 1) {
		assert.Equal(t, "irancell: inquiry amount does not fit the price field", log.lines[0])
	}
	// The largest price that fits. No logger and a too large amount: no panic.
	f = &fakeInquirer{res: inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: math.MaxUint32, InquiryType: provider.InquiryTypeMidterm}}}}
	res, err = NewPhone(PhoneDeps{Inquiry: f}).Inquiry(waterCtx(), irancellReq())
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, uint32(math.MaxUint32), res.GetMidTerm().GetPrice())
	}
	f = &fakeInquirer{res: inquiry.Result{Inquiries: []inquiry.Inquiry{{Amount: math.MaxUint32 + 1}}}}
	_, err = NewPhone(PhoneDeps{Inquiry: f}).Inquiry(waterCtx(), irancellReq())
	assert.Equal(t, codes.Internal, status.Code(err))
}

func TestRegisterPhoneRejectsNil(t *testing.T) {
	assert.Nil(t, RegisterPhone(nil))
	var typedNil *Phone
	assert.Nil(t, RegisterPhone(typedNil))
	assert.NotNil(t, RegisterPhone(NewPhone(PhoneDeps{})))
	_, err := New("127.0.0.1:0", RegisterPhone(nil))
	assert.Error(t, err)
}
