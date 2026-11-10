package grpcserver

import (
	"context"
	"math"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/core/inquiry"
	"qabz/provider"
)

// ElectricityDeps are the dependencies of Electricity. A zero value is valid: Inquiry then answers
// codes.Unimplemented.
type ElectricityDeps struct {
	// Inquiry runs the flow (the same inquiry.Service as Water). Nil: Inquiry is not available.
	Inquiry Inquirer
	// Logger gets one error line for each server fault. Nil: no log line.
	Logger ErrorLogger
}

// Electricity implements the gRPC service haf80.api.bill.Electricity. Methods that are not written
// yet (InquiryDocument, the order methods) return codes.Unimplemented through the embedded type.
type Electricity struct {
	pb.UnimplementedElectricityServer // embedded by value, as protoc-gen-go-grpc requires

	inquiry Inquirer
	logger  ErrorLogger
}

// NewElectricity returns the Electricity implementation.
func NewElectricity(deps ElectricityDeps) *Electricity {
	return &Electricity{inquiry: deps.Inquiry, logger: deps.Logger}
}

// RegisterElectricity returns a Register that adds svc to a gRPC server. For a nil svc it returns
// nil, so New rejects it.
func RegisterElectricity(svc pb.ElectricityServer) Register {
	if svc == nil {
		return nil
	}
	if p, ok := svc.(*Electricity); ok && p == nil { // a typed nil pointer in the interface
		return nil
	}
	return func(r grpc.ServiceRegistrar) { pb.RegisterElectricityServer(r, svc) }
}

// Inquiry answers the inquiry of an electricity bill (POST /v1/electricity/inquiry, task 022): the
// saved inquiries of the bill when they are still valid, else the provider (inquiry.Service). The
// flow and the answers of every failure are those of Water.Inquiry (answerInquiryError; the log
// lines start with "electricity:"). Only billId is used, as in bill (payId and participateCode are
// ignored); when the answer has more than one inquiry, the last one is sent.
//
// The details (phase, company, tariff, customer type, serial number, address, post code, customer
// name) are those of the answer, from the provider or from the saved inquiries (the same values).
// An answer with no details has empty detail fields, as in bill; with details, customerType is the
// number of the provider as text (bill: fmt.Sprint).
func (e *Electricity) Inquiry(ctx context.Context, req *pb.InquiryBillPayRequest) (*pb.InquiryElectricityResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if e.inquiry == nil {
		return nil, status.Error(codes.Unimplemented, "method Inquiry not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		// Only with debug.auth.disable and an empty debug.auth.mock_user_id: the call has no user.
		// @todo bill answers gRPC Unknown here. See docs/this_vs_bill.md, item 14.
		return nil, errRejected
	}
	res, err := e.inquiry.Inquire(ctx, inquiry.Request{
		BillType:  provider.BillTypeElectricity,
		InquiryID: req.GetBillId(), // req nil: an empty bill ID
		UserID:    claims.UserID,
		Dialer:    claims.Mobile,
	})
	if err != nil {
		return nil, answerInquiryError(ctx, e.logger, provider.BillTypeElectricity, err)
	}
	out := &pb.InquiryElectricityResponse{BillId: res.BillID}
	if res.Details != nil && res.Details.Electricity != nil {
		d := res.Details.Electricity
		out.Phase = d.Phase
		out.CompanyName = d.CompanyName
		out.TariffType = d.TariffType
		out.CustomerType = strconv.FormatInt(int64(d.CustomerType), 10)
		out.SerialNumber = d.SerialNumber
		out.ServiceAddress = d.ServiceAddress
		out.ServicePostCode = d.ServicePostCode
		out.CustomerName = d.CustomerName
	}
	for _, in := range res.Inquiries {
		if in.Amount > math.MaxUint32 {
			// The proto has uint32 for the price. bill converts without a check (a wrong price).
			// @todo See docs/this_vs_bill.md, item 22.
			if e.logger != nil {
				e.logger.Error("electricity: inquiry amount does not fit the price field", "amount", in.Amount)
			}
			return nil, errInternal
		}
		out.PayId = in.PayID
		out.PaymentDeadline = in.PaymentDeadline
		out.Price = uint32(in.Amount)
	}
	return out, nil
}
