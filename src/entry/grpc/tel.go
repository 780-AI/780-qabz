package grpcserver

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/helpers"
	"qabz/core/inquiry"
	"qabz/provider"
)

// TelDeps are the dependencies of Tel. A zero value is valid: Inquiry then answers
// codes.Unimplemented.
type TelDeps struct {
	// Inquiry runs the flow (the same inquiry.Service as Water). Nil: Inquiry is not available.
	Inquiry Inquirer
	// Raw answers a request with both billId and payId. Nil: such a request is Unimplemented.
	Raw RawInquirer
	// Logger gets one error line for each server fault. Nil: no log line.
	Logger ErrorLogger
}

// Tel implements the gRPC service haf80.api.bill.Tel (the landline bill). Methods that are not
// written yet (the order methods) return codes.Unimplemented through the embedded type.
type Tel struct {
	pb.UnimplementedTelServer // embedded by value, as protoc-gen-go-grpc requires

	inquiry Inquirer
	raw     RawInquirer
	logger  ErrorLogger
}

// NewTel returns the Tel implementation.
func NewTel(deps TelDeps) *Tel {
	return &Tel{inquiry: deps.Inquiry, raw: deps.Raw, logger: deps.Logger}
}

// RegisterTel returns a Register that adds svc to a gRPC server. For a nil svc it returns nil, so
// New rejects it.
func RegisterTel(svc pb.TelServer) Register {
	if svc == nil {
		return nil
	}
	if t, ok := svc.(*Tel); ok && t == nil { // a typed nil pointer in the interface
		return nil
	}
	return func(r grpc.ServiceRegistrar) { pb.RegisterTelServer(r, svc) }
}

// validTelRequest is the request check of bill (controller/telephone validateInquiryTelRequest):
// the request has phone or billId, and phone or payId; a billId that is there is a valid bill ID. So
// a request with only billId, only payId, or nothing, is invalid. The phone itself is checked only
// for an inquiry (Tel.Inquiry), as bill does not check it for a raw inquiry.
func validTelRequest(phone, billID, payID string) bool {
	if phone == "" && (billID == "" || payID == "") {
		return false
	}
	return billID == "" || helpers.ValidBillID(billID)
}

// Inquiry answers the inquiry of a landline bill (POST /v1/tel/inquiry, task 025): the saved
// inquiries of the number when they are still valid, else Karashop (inquiry.Service; the bill type
// tel has one provider that may answer). The flow and the answers of every failure are those of
// Water.Inquiry (answerInquiryError; the log lines start with "tel:"). The answer is the one of a
// mobile inquiry (phoneInquiryResponse): a mid-term and a full-term part.
//
// What is special to the landline bill, as in bill:
//   - The request check is validTelRequest (InvalidArgument, no log line).
//   - A request with both billId and payId is a raw inquiry (rawInquiry), with or without a phone.
//   - Else the phone is converted to the 98 form (helpers.NormalizePhone, bill's
//     NormalizePhoneNumber) and is the inquiry ID: the key of the saved inquiries and what Karashop
//     gets.
//
// The converted phone must be a landline number in the 98 form ("98" and ten digits,
// helpers.ValidTel), else InvalidArgument with no log line and no provider call.
// @todo bill sends any phone to the provider with no check. See docs/this_vs_bill.md, item 36.
func (t *Tel) Inquiry(ctx context.Context, req *pb.InquiryTelRequest) (*pb.InquiryPhoneResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if t.inquiry == nil {
		return nil, status.Error(codes.Unimplemented, "method Inquiry not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		// Only with debug.auth.disable and an empty debug.auth.mock_user_id: the call has no user.
		// @todo bill answers gRPC Unknown here. See docs/this_vs_bill.md, item 14.
		return nil, errRejected
	}
	phone, billID, payID := req.GetPhone(), req.GetBillId(), req.GetPayId() // req nil: all empty
	if !validTelRequest(phone, billID, payID) {
		// @todo bill answers gRPC Unknown (HTTP 500). See docs/this_vs_bill.md, item 14.
		return nil, errInvalidParameter
	}
	if billID != "" && payID != "" {
		return t.rawInquiry(billID, payID)
	}
	phone = helpers.NormalizePhone(phone)
	if !helpers.ValidTel(phone) {
		return nil, errInvalidParameter
	}
	res, err := t.inquiry.Inquire(ctx, inquiry.Request{
		BillType:  provider.BillTypeTel,
		InquiryID: phone,
		UserID:    claims.UserID,
		Dialer:    claims.Mobile,
	})
	if err != nil {
		return nil, answerInquiryError(ctx, t.logger, provider.BillTypeTel, err)
	}
	return phoneInquiryResponse(t.logger, provider.BillTypeTel, res)
}

// rawInquiry answers a request with both billId and payId from the two IDs, with no provider, no
// saved inquiry and no row (bill: Telephone.rawInquiry, raw.Service.Inquiry): the answer has only
// fullTerm (price, payId, billId; no paymentDeadline) and an empty midTerm, as in bill. The errors
// are those of the gas raw inquiry (answerRawInquiry; docs/this_vs_bill.md, item 28).
func (t *Tel) rawInquiry(billID, payID string) (*pb.InquiryPhoneResponse, error) {
	res, err := answerRawInquiry(t.raw, t.logger, provider.BillTypeTel, billID, payID)
	if err != nil {
		return nil, err
	}
	return &pb.InquiryPhoneResponse{
		MidTerm:  &pb.InquiryPhoneResponse_PhoneInquiry{},
		FullTerm: &pb.InquiryPhoneResponse_PhoneInquiry{BillId: res.BillID, PayId: res.PayID, Price: uint32(res.Amount)},
	}, nil
}
