package grpcserver

import (
	"context"
	"errors"
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

// The texts of a raw inquiry that breaks a rule (bill: raw.ErrInvalidBillID, raw.ErrInvalidPayID,
// raw.ErrUnknownBillType).
const (
	rawInvalidBillIDText   = "شناسه قبض نامعتبر است"
	rawInvalidPayIDText    = "شناسه پرداخت نامعتبر است"
	rawUnknownBillTypeText = "نوع قبض نامعتبر است"
)

// errRawNotAvailable is the answer of a raw inquiry when the service has no RawInquirer.
var errRawNotAvailable = status.Error(codes.Unimplemented, "inquiry with billId and payId is not available")

// RawInquirer answers a raw inquiry: from the bill ID and the pay ID, with no provider
// (inquiry.Service.RawInquiry).
type RawInquirer interface {
	RawInquiry(endpointBillType, billID, payID string) (inquiry.RawResult, error)
}

// GasDeps are the dependencies of Gas. A zero value is valid: Inquiry then answers
// codes.Unimplemented.
type GasDeps struct {
	// Inquiry runs the flow (the same inquiry.Service as Water). Nil: Inquiry is not available.
	Inquiry Inquirer
	// Raw answers a request with both billId and payId. Nil: such a request is Unimplemented.
	Raw RawInquirer
	// Logger gets one error line for each server fault. Nil: no log line.
	Logger ErrorLogger
}

// Gas implements the gRPC service haf80.api.bill.Gas. Methods that are not written yet (the order
// methods) return codes.Unimplemented through the embedded type.
type Gas struct {
	pb.UnimplementedGasServer // embedded by value, as protoc-gen-go-grpc requires

	inquiry Inquirer
	raw     RawInquirer
	logger  ErrorLogger
}

// NewGas returns the Gas implementation.
func NewGas(deps GasDeps) *Gas {
	return &Gas{inquiry: deps.Inquiry, raw: deps.Raw, logger: deps.Logger}
}

// RegisterGas returns a Register that adds svc to a gRPC server. For a nil svc it returns nil, so
// New rejects it.
func RegisterGas(svc pb.GasServer) Register {
	if svc == nil {
		return nil
	}
	if p, ok := svc.(*Gas); ok && p == nil { // a typed nil pointer in the interface
		return nil
	}
	return func(r grpc.ServiceRegistrar) { pb.RegisterGasServer(r, svc) }
}

// validGasInquiry is the request check of bill (controller/gas validateInquiryRequest): the
// request has participateCode or billId, and participateCode or payId; a billId that is there is a
// valid bill ID. So a request with only billId, or with nothing, is invalid.
func validGasInquiry(billID, payID, participateCode string) bool {
	if participateCode == "" && (billID == "" || payID == "") {
		return false
	}
	return billID == "" || helpers.ValidBillID(billID)
}

// Inquiry answers the inquiry of a gas bill (POST /v1/gas/inquiry, task 023): the saved inquiries
// of the participate code when they are still valid, else the provider (inquiry.Service). The flow
// and the answers of every failure are those of Water.Inquiry (answerInquiryError; the log lines
// start with "gas:"). When the answer has more than one inquiry, the last one is sent.
//
// What is special to gas, as in bill:
//   - The request check is validGasInquiry (InvalidArgument, no log line).
//   - The inquiry ID is the participate code (bill: controller.SetInquiryID): it is the key of the
//     saved inquiries and what the provider gets. The billId of the answer is the one of the
//     provider, not the one of the request.
//   - A request with both billId and payId is a raw inquiry (rawInquiry), with or without a
//     participate code: the answer comes from the two IDs, with no provider.
func (g *Gas) Inquiry(ctx context.Context, req *pb.InquiryBillPayRequest) (*pb.InquiryBillPayResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if g.inquiry == nil {
		return nil, status.Error(codes.Unimplemented, "method Inquiry not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		// Only with debug.auth.disable and an empty debug.auth.mock_user_id: the call has no user.
		// @todo bill answers gRPC Unknown here. See docs/this_vs_bill.md, item 14.
		return nil, errRejected
	}
	billID, payID, code := req.GetBillId(), req.GetPayId(), req.GetParticipateCode() // req nil: all empty
	if !validGasInquiry(billID, payID, code) {
		// @todo bill answers gRPC Unknown (HTTP 500). See docs/this_vs_bill.md, item 14.
		return nil, errInvalidParameter
	}
	if billID != "" && payID != "" {
		return g.rawInquiry(billID, payID)
	}
	res, err := g.inquiry.Inquire(ctx, inquiry.Request{
		BillType:  provider.BillTypeGas,
		InquiryID: code, // not empty here: validGasInquiry and the raw inquiry check
		UserID:    claims.UserID,
		Dialer:    claims.Mobile,
	})
	if err != nil {
		return nil, answerInquiryError(ctx, g.logger, provider.BillTypeGas, err)
	}
	out := &pb.InquiryBillPayResponse{BillId: res.BillID}
	for _, in := range res.Inquiries {
		if in.Amount > math.MaxUint32 {
			// The proto has uint32 for the price. bill converts without a check (a wrong price).
			// @todo See docs/this_vs_bill.md, item 22.
			if g.logger != nil {
				g.logger.Error("gas: inquiry amount does not fit the price field", "amount", in.Amount)
			}
			return nil, errInternal
		}
		out.PayId = in.PayID
		out.PaymentDeadline = in.PaymentDeadline
		out.Price = uint32(in.Amount)
	}
	return out, nil
}

// rawInquiry answers a request with both billId and payId from the two IDs, with no provider, no
// saved inquiry and no row (bill: Gas.rawInquiry, raw.Service.Inquiry): the price is read from the
// pay ID. The answer has price, payId and billId, and no paymentDeadline, as in bill. As in bill,
// the bill ID can be of another bill type than gas.
//
// A pay ID that is not valid and a bill ID of an unknown bill type are InvalidArgument with the
// text of bill, and no log line (client errors, conventions § 10).
// @todo bill answers gRPC Unknown (HTTP 500) with the same texts. bill converts a price above the
// uint32 field without a check (a wrong price); Qabz answers the pay ID text. See
// docs/this_vs_bill.md, item 28.
func (g *Gas) rawInquiry(billID, payID string) (*pb.InquiryBillPayResponse, error) {
	res, err := answerRawInquiry(g.raw, g.logger, provider.BillTypeGas, billID, payID)
	if err != nil {
		return nil, err
	}
	return &pb.InquiryBillPayResponse{Price: uint32(res.Amount), PayId: res.PayID, BillId: res.BillID}, nil
}

// answerRawInquiry runs a raw inquiry (raw.RawInquiry) for the endpoint of billType and maps its
// errors (Gas.rawInquiry, Tel.rawInquiry): a nil raw is Unimplemented; a bill ID or a pay ID that is
// not valid, a bill ID of an unknown bill type and a price above the uint32 price field are
// InvalidArgument with the text of bill and no log line; any other error is Internal with one error
// line (the prefix is the bill type). A nil error means that res.Amount fits uint32.
func answerRawInquiry(raw RawInquirer, logger ErrorLogger, billType, billID, payID string) (inquiry.RawResult, error) {
	if raw == nil {
		return inquiry.RawResult{}, errRawNotAvailable
	}
	res, err := raw.RawInquiry(billType, billID, payID)
	switch {
	case errors.Is(err, inquiry.ErrInvalidBillID):
		return inquiry.RawResult{}, status.Error(codes.InvalidArgument, rawInvalidBillIDText)
	case errors.Is(err, inquiry.ErrInvalidPayID):
		return inquiry.RawResult{}, status.Error(codes.InvalidArgument, rawInvalidPayIDText)
	case errors.Is(err, inquiry.ErrUnknownBillType):
		return inquiry.RawResult{}, status.Error(codes.InvalidArgument, rawUnknownBillTypeText)
	case err != nil:
		if logger != nil {
			logger.Error(billType+": raw inquiry failed", "error", err)
		}
		return inquiry.RawResult{}, errInternal
	case res.Amount > math.MaxUint32:
		// The price is the user's own pay ID: a client error, not a server fault (no log line).
		return inquiry.RawResult{}, status.Error(codes.InvalidArgument, rawInvalidPayIDText)
	}
	return res, nil
}
