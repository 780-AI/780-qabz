package grpcserver

import (
	"context"
	"math"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/helpers"
	"qabz/core/inquiry"
	"qabz/provider"
)

// phoneOperators gives the bill type of each operator of a mobile inquiry (bill: controller/phone
// operatorBillType). It is the one table of the endpoint: task 035 filled the entry of Irancell, task
// 036 the entry of MCI, task 037 the entry of Rightel. An operator with an empty bill type is known but
// not available yet (Unimplemented; no operator has one since task 037); an operator that is not in
// the table is invalid (InvalidArgument).
//
//	bill: mci -> "mobile", rightel -> "mobilemidtermRightel", irancell -> "irancell".
var phoneOperators = map[pb.InquiryPhoneRequest_OperatorEnum]string{
	pb.InquiryPhoneRequest_irancell: provider.BillTypeIrancell,
	pb.InquiryPhoneRequest_mci:      provider.BillTypeMobile,
	pb.InquiryPhoneRequest_rightel:  provider.BillTypeMobileMidTermRightel,
}

// PhoneDeps are the dependencies of Phone. A zero value is valid: Inquiry then answers
// codes.Unimplemented.
type PhoneDeps struct {
	// Inquiry runs the flow (the same inquiry.Service as Water). Nil: Inquiry is not available.
	Inquiry Inquirer
	// Logger gets one error line for each server fault. Nil: no log line.
	Logger ErrorLogger
}

// Phone implements the gRPC service haf80.api.bill.Phone (the mobile bill). Methods that are not
// written yet (the order methods) return codes.Unimplemented through the embedded type.
type Phone struct {
	pb.UnimplementedPhoneServer // embedded by value, as protoc-gen-go-grpc requires

	inquiry Inquirer
	logger  ErrorLogger
}

// NewPhone returns the Phone implementation.
func NewPhone(deps PhoneDeps) *Phone {
	return &Phone{inquiry: deps.Inquiry, logger: deps.Logger}
}

// RegisterPhone returns a Register that adds svc to a gRPC server. For a nil svc it returns nil, so
// New rejects it.
func RegisterPhone(svc pb.PhoneServer) Register {
	if svc == nil {
		return nil
	}
	if p, ok := svc.(*Phone); ok && p == nil { // a typed nil pointer in the interface
		return nil
	}
	return func(r grpc.ServiceRegistrar) { pb.RegisterPhoneServer(r, svc) }
}

// validPhoneInquiry is the request check of bill (controller/phone validateInquiryRequest): the
// phone is a mobile number in the 98 form (989 and nine digits; an empty phone is invalid, so the
// "phone or billId" and "phone or payId" rules of bill never decide), a billId that is there is a
// valid bill ID, and operator and service are values of the enum. The payId is not checked.
func validPhoneInquiry(req *pb.InquiryPhoneRequest) bool {
	if !helpers.ValidMobile(req.GetPhone()) {
		return false
	}
	if id := req.GetBillId(); id != "" && !helpers.ValidBillID(id) {
		return false
	}
	_, okOperator := pb.InquiryPhoneRequest_OperatorEnum_name[int32(req.GetOperator())]
	_, okService := pb.InquiryPhoneRequest_OperatorEnum_name[int32(req.GetService())]
	return okOperator && okService
}

// Inquiry answers the inquiry of a mobile bill (POST /v1/mobile/inquiry; tasks 035 to 037): the
// saved inquiries of the phone when they are still valid, else the provider of the operator
// (inquiry.Service). The flow and the answers of every failure are those of Water.Inquiry
// (answerInquiryError; the log lines start with the bill type, for example "irancell:" or, for the
// MCI operator, "mobile:", for the Rightel operator "mobilemidtermRightel:").
//
// What is special to the mobile bill, as in bill:
//   - The request check is validPhoneInquiry (InvalidArgument, no log line).
//   - The operator of the request gives the bill type (phoneOperators). As in bill, only operator
//     is read: service is never used (the text of an enum value is never empty, so bill's
//     "operator, or service when the operator is empty" always takes operator), and a request with
//     no operator is mci (the enum value 0), so it runs the MCI inquiry (task 036).
//   - The inquiry ID is the phone (bill: controller.SetInquiryID(billId, phone); the phone is never
//     empty after the request check).
//   - The answer has a mid-term and a full-term inquiry: the inquiry of the type
//     provider.InquiryTypeMidterm goes to midTerm, every other one to fullTerm (the last one of
//     each wins); a part with no inquiry is an empty object. providerId is not set, as in bill.
//
// An operator with an empty bill type in phoneOperators is Unimplemented (no log line): task 035,
// decision B; since task 037 no operator has one. The operator "unknown" has no bill type:
// InvalidArgument.
// @todo bill answers the operator "unknown" with the raw error of the provider lookup (gRPC
// Unknown, HTTP 500). See docs/this_vs_bill.md, item 31.
func (p *Phone) Inquiry(ctx context.Context, req *pb.InquiryPhoneRequest) (*pb.InquiryPhoneResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if p.inquiry == nil {
		return nil, status.Error(codes.Unimplemented, "method Inquiry not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		// Only with debug.auth.disable and an empty debug.auth.mock_user_id: the call has no user.
		// @todo bill answers gRPC Unknown here. See docs/this_vs_bill.md, item 14.
		return nil, errRejected
	}
	if !validPhoneInquiry(req) { // req nil: an empty phone, invalid
		// @todo bill answers gRPC Unknown (HTTP 500). See docs/this_vs_bill.md, item 14.
		return nil, errInvalidParameter
	}
	billType, known := phoneOperators[req.GetOperator()]
	if !known {
		return nil, errInvalidParameter
	}
	if billType == "" {
		return nil, status.Error(codes.Unimplemented, "inquiry of the operator "+req.GetOperator().String()+" is not available yet")
	}
	res, err := p.inquiry.Inquire(ctx, inquiry.Request{
		BillType:  billType,
		InquiryID: req.GetPhone(),
		UserID:    claims.UserID,
		Dialer:    claims.Mobile,
	})
	if err != nil {
		return nil, answerInquiryError(ctx, p.logger, billType, err)
	}
	out := &pb.InquiryPhoneResponse{
		MidTerm:  &pb.InquiryPhoneResponse_PhoneInquiry{},
		FullTerm: &pb.InquiryPhoneResponse_PhoneInquiry{},
	}
	for _, in := range res.Inquiries {
		if in.Amount > math.MaxUint32 {
			// The proto has uint32 for the price. bill converts without a check (a wrong price).
			// @todo See docs/this_vs_bill.md, item 22.
			if p.logger != nil {
				p.logger.Error(billType+": inquiry amount does not fit the price field", "amount", in.Amount)
			}
			return nil, errInternal
		}
		part := &pb.InquiryPhoneResponse_PhoneInquiry{
			BillId:          res.BillID,
			PayId:           in.PayID,
			Price:           uint32(in.Amount),
			PaymentDeadline: in.PaymentDeadline,
		}
		if in.InquiryType == provider.InquiryTypeMidterm {
			out.MidTerm = part
		} else {
			out.FullTerm = part
		}
	}
	return out, nil
}
