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

const mciPhone = "989121234568"

// mciResult is an answer of the MCI inquiry: one bill ID, a pay ID for each inquiry, a payment
// deadline on the mid-term inquiry only.
func mciResult() inquiry.Result {
	return inquiry.Result{BillID: "9120000036", Inquiries: []inquiry.Inquiry{
		{BillID: "9120000036", PayID: "21003611", Amount: 210000, PaymentDeadline: "2026-12-20", InquiryType: provider.InquiryTypeMidterm},
		{BillID: "9120000036", PayID: "61003622", Amount: 610000, InquiryType: provider.InquiryTypeFinalTerm},
	}}
}

const rightelPhone = "989201234567"

// rightelResult is an answer of the Rightel inquiry (the Qabzino mock values): one bill ID, a pay ID
// for each inquiry, no deadline.
func rightelResult() inquiry.Result {
	return inquiry.Result{BillID: "9200000034", Inquiries: []inquiry.Inquiry{
		{BillID: "9200000034", PayID: "23000341", Amount: 230000, InquiryType: provider.InquiryTypeMidterm},
		{BillID: "9200000034", PayID: "63000342", Amount: 630000, InquiryType: provider.InquiryTypeFinalTerm},
	}}
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

// TestPhoneInquiryOperators: the operator table. Irancell, mci (task 036; also a request with no
// operator: the enum value 0) and rightel (task 037) run the flow with their bill type.
func TestPhoneInquiryOperators(t *testing.T) {
	assert.Len(t, phoneOperators, 3, "mci, irancell, rightel; unknown has no bill type")
	assert.Equal(t, provider.BillTypeIrancell, phoneOperators[pb.InquiryPhoneRequest_irancell])
	assert.Equal(t, provider.BillTypeMobile, phoneOperators[pb.InquiryPhoneRequest_mci])
	assert.Equal(t, provider.BillTypeMobileMidTermRightel, phoneOperators[pb.InquiryPhoneRequest_rightel])
	assert.Equal(t, "mobilemidtermRightel", provider.BillTypeMobileMidTermRightel, "bill: entity.MOBILEMIDTERMRIGHTEL")

	// mci: the bill type "mobile" (bill: operatorBillType), the inquiry ID is the phone.
	for name, req := range map[string]*pb.InquiryPhoneRequest{
		"mci":                           {Phone: mciPhone, Operator: pb.InquiryPhoneRequest_mci},
		"no operator":                   {Phone: mciPhone},
		"no operator, service irancell": {Phone: mciPhone, Service: pb.InquiryPhoneRequest_irancell},
	} {
		log := &errLog{}
		f := &fakeInquirer{res: mciResult()}
		res, err := NewPhone(PhoneDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(), req)
		if !assert.NoError(t, err, name) || !assert.NotNil(t, res, name) || !assert.Len(t, f.reqs, 1, name) {
			continue
		}
		assert.Equal(t, inquiry.Request{BillType: "mobile", InquiryID: mciPhone, UserID: billsUser, Dialer: "989121234567"}, f.reqs[0], name)
		if assert.NotNil(t, res.GetMidTerm(), name) && assert.NotNil(t, res.GetFullTerm(), name) {
			assert.Equal(t, []string{"9120000036", "21003611", "2026-12-20"},
				[]string{res.GetMidTerm().GetBillId(), res.GetMidTerm().GetPayId(), res.GetMidTerm().GetPaymentDeadline()}, name)
			assert.Equal(t, uint32(210000), res.GetMidTerm().GetPrice(), name)
			assert.Equal(t, []string{"9120000036", "61003622", ""},
				[]string{res.GetFullTerm().GetBillId(), res.GetFullTerm().GetPayId(), res.GetFullTerm().GetPaymentDeadline()}, name)
			assert.Equal(t, uint32(610000), res.GetFullTerm().GetPrice(), name)
			assert.EqualValues(t, 0, res.GetMidTerm().GetProviderId(), name+": providerId is not set, as in bill")
		}
		assert.Empty(t, log.lines, name+": no log line")
	}

	// The failures of the flow have the answers of every other inquiry; the log lines name the bill type.
	for name, c := range map[string]struct {
		err  error
		code codes.Code
		line string
	}{
		"provider down": {fmt.Errorf("x: %w", provider.ErrProviderDown), codes.Unavailable, "mobile: provider inquiry failed"},
		"no provider":   {fmt.Errorf("x: %w", provider.ErrNoProvider), codes.Unavailable, "mobile: no enabled provider for inquiry"},
		"refused":       {&inquiry.RefusedError{Message: "not an MCI number"}, codes.FailedPrecondition, ""},
	} {
		log := &errLog{}
		_, err := NewPhone(PhoneDeps{Inquiry: &fakeInquirer{err: c.err}, Logger: log}).Inquiry(waterCtx(),
			&pb.InquiryPhoneRequest{Phone: mciPhone, Operator: pb.InquiryPhoneRequest_mci})
		assert.Equal(t, c.code, status.Code(err), name)
		if c.line == "" {
			assert.Empty(t, log.lines, name+": a business answer writes no log line")
		} else if assert.Len(t, log.lines, 1, name) {
			assert.Contains(t, log.lines[0], c.line, name)
		}
	}

	// rightel (task 037): the bill type "mobilemidtermRightel" (bill: operatorBillType), the inquiry
	// ID is the phone; the mid-term and the final-term inquiry go to midTerm and fullTerm.
	{
		log := &errLog{}
		f := &fakeInquirer{res: rightelResult()}
		res, err := NewPhone(PhoneDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(),
			&pb.InquiryPhoneRequest{Phone: rightelPhone, Operator: pb.InquiryPhoneRequest_rightel})
		if assert.NoError(t, err) && assert.NotNil(t, res) && assert.Len(t, f.reqs, 1) {
			assert.Equal(t, inquiry.Request{BillType: "mobilemidtermRightel", InquiryID: rightelPhone, UserID: billsUser, Dialer: "989121234567"}, f.reqs[0])
			assert.Equal(t, []string{"9200000034", "23000341", ""},
				[]string{res.GetMidTerm().GetBillId(), res.GetMidTerm().GetPayId(), res.GetMidTerm().GetPaymentDeadline()})
			assert.Equal(t, uint32(230000), res.GetMidTerm().GetPrice())
			assert.Equal(t, []string{"9200000034", "63000342", ""},
				[]string{res.GetFullTerm().GetBillId(), res.GetFullTerm().GetPayId(), res.GetFullTerm().GetPaymentDeadline()})
			assert.Equal(t, uint32(630000), res.GetFullTerm().GetPrice())
		}
		assert.Empty(t, log.lines, "rightel: no log line")
	}
	for name, c := range map[string]struct {
		err  error
		code codes.Code
		line string
	}{
		"rightel provider down": {fmt.Errorf("x: %w", provider.ErrProviderDown), codes.Unavailable, "mobilemidtermRightel: provider inquiry failed"},
		"rightel no provider":   {fmt.Errorf("x: %w", provider.ErrNoProvider), codes.Unavailable, "mobilemidtermRightel: no enabled provider for inquiry"},
		"rightel refused":       {&inquiry.RefusedError{Message: "not a Rightel number"}, codes.FailedPrecondition, ""},
	} {
		log := &errLog{}
		_, err := NewPhone(PhoneDeps{Inquiry: &fakeInquirer{err: c.err}, Logger: log}).Inquiry(waterCtx(),
			&pb.InquiryPhoneRequest{Phone: rightelPhone, Operator: pb.InquiryPhoneRequest_rightel})
		assert.Equal(t, c.code, status.Code(err), name)
		if c.line == "" {
			assert.Empty(t, log.lines, name+": a business answer writes no log line")
		} else if assert.Len(t, log.lines, 1, name) {
			assert.Contains(t, log.lines[0], c.line, name)
		}
	}
}

// TestPhoneInquiryOperatorNotAvailable: an operator with an empty bill type in the table is
// Unimplemented and the flow is not called (task 035, decision B). No operator has one since task
// 037, so the test empties the entry of rightel for its run.
func TestPhoneInquiryOperatorNotAvailable(t *testing.T) {
	saved := phoneOperators[pb.InquiryPhoneRequest_rightel]
	phoneOperators[pb.InquiryPhoneRequest_rightel] = ""
	t.Cleanup(func() { phoneOperators[pb.InquiryPhoneRequest_rightel] = saved })

	log := &errLog{}
	f := &fakeInquirer{res: rightelResult()}
	res, err := NewPhone(PhoneDeps{Inquiry: f, Logger: log}).Inquiry(waterCtx(),
		&pb.InquiryPhoneRequest{Phone: rightelPhone, Operator: pb.InquiryPhoneRequest_rightel})
	assert.Nil(t, res)
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "is not available yet")
	assert.Equal(t, 0, f.calls(), "the flow is not called")
	assert.Empty(t, log.lines, "no log line")
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
