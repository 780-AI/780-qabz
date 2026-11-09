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
	"qabz/core/inquiry"
	"qabz/provider"
)

// providerDownText is the text of bill's provider_inquiry_is_down_error: "the inquiry from the
// provider is temporarily disabled".
const providerDownText = "استعلام از سوی تامین کننده موقتا غیر فعال می باشد"

// providerRefusedDefaultText is the message of a provider that has no inquiry and sent no
// message of its own.
const providerRefusedDefaultText = "inquiry failed"

// errProviderDown is the answer when no provider can answer: none is enabled for water inquiry, the
// call to the provider failed, or the picked provider has no adapter.
// @todo bill sends the same text as a plain error (gRPC Unknown, HTTP 500) for a failed call, and
// the raw database error for a missing provider. See docs/this_vs_bill.md, items 15 and 16.
var errProviderDown = status.Error(codes.Unavailable, providerDownText)

// WaterInquirer answers the inquiry of a bill (inquiry.Service).
type WaterInquirer interface {
	Inquire(ctx context.Context, req inquiry.Request) (inquiry.Result, error)
}

// WaterDeps are the dependencies of Water. A zero value is valid: Inquiry then answers
// codes.Unimplemented.
type WaterDeps struct {
	// Inquiry runs the flow. Nil: Inquiry is not available.
	Inquiry WaterInquirer
	// Logger gets one error line for each server fault. Nil: no log line.
	Logger ErrorLogger
}

// Water implements the gRPC service haf80.api.bill.Water. Methods that are not written yet return
// codes.Unimplemented through the embedded type.
type Water struct {
	pb.UnimplementedWaterServer // embedded by value, as protoc-gen-go-grpc requires

	inquiry WaterInquirer
	logger  ErrorLogger
}

// NewWater returns the Water implementation.
func NewWater(deps WaterDeps) *Water {
	return &Water{inquiry: deps.Inquiry, logger: deps.Logger}
}

// RegisterWater returns a Register that adds svc to a gRPC server. For a nil svc it returns nil, so
// New rejects it.
func RegisterWater(svc pb.WaterServer) Register {
	if svc == nil {
		return nil
	}
	if p, ok := svc.(*Water); ok && p == nil { // a typed nil pointer in the interface
		return nil
	}
	return func(r grpc.ServiceRegistrar) { pb.RegisterWaterServer(r, svc) }
}

// Inquiry answers the inquiry of a water bill (POST /v1/water/inquiry): the saved inquiries of the
// bill when they are still valid, else the provider (inquiry.Service). Only billId is used, as in
// bill (payId and participateCode are ignored). As in bill, when the answer has more than one
// inquiry, the last one is sent.
//
// Answers: a request that breaks a rule is InvalidArgument (no log line); a call with no user is
// PermissionDenied; no enabled provider, a failed call to the provider or a provider with no
// adapter is Unavailable with providerDownText and one error log line; a provider that has no
// inquiry is FailedPrecondition with the message of the provider (no log line); any other failure
// is Internal with no detail and one error log line. A client that left gets its context error.
func (w *Water) Inquiry(ctx context.Context, req *pb.InquiryBillPayRequest) (*pb.InquiryBillPayResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if w.inquiry == nil {
		return nil, status.Error(codes.Unimplemented, "method Inquiry not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		// Only with debug.auth.disable and an empty debug.auth.mock_user_id: the call has no user.
		// @todo bill answers gRPC Unknown here. See docs/this_vs_bill.md, item 14.
		return nil, errRejected
	}
	res, err := w.inquiry.Inquire(ctx, inquiry.Request{
		BillType:  provider.BillTypeWater,
		InquiryID: req.GetBillId(), // req nil: an empty bill ID
		UserID:    claims.UserID,
		Dialer:    claims.Mobile,
	})
	if err != nil {
		return nil, w.answerError(ctx, err)
	}
	out := &pb.InquiryBillPayResponse{BillId: res.BillID}
	for _, in := range res.Inquiries {
		if in.Amount > math.MaxUint32 {
			// The proto has uint32 for the price. bill converts without a check (a wrong price).
			// @todo See docs/this_vs_bill.md, item 22.
			if w.logger != nil {
				w.logger.Error("water: inquiry amount does not fit the price field", "amount", in.Amount)
			}
			return nil, errInternal
		}
		out.PayId = in.PayID
		out.PaymentDeadline = in.PaymentDeadline
		out.Price = uint32(in.Amount)
	}
	return out, nil
}

// answerError turns an error of the flow into the gRPC answer and writes the log line of a
// server fault (conventions § 10).
func (w *Water) answerError(ctx context.Context, err error) error {
	var refused *inquiry.RefusedError
	switch {
	case errors.Is(err, inquiry.ErrInvalid):
		// @todo bill answers gRPC Unknown (HTTP 500). See docs/this_vs_bill.md, item 14.
		return errInvalidParameter
	case ctx.Err() != nil:
		return status.FromContextError(ctx.Err()).Err() // the client left: not a server error
	case errors.As(err, &refused) && refused != nil:
		// @todo bill answers gRPC Unknown (HTTP 500) with the message. See docs/this_vs_bill.md, item 17.
		msg := refused.Message
		if msg == "" {
			msg = providerRefusedDefaultText
		}
		return status.Error(codes.FailedPrecondition, msg)
	case errors.Is(err, provider.ErrNoProvider):
		w.logError("water: no enabled provider for inquiry", err)
		return errProviderDown
	case errors.Is(err, provider.ErrProviderDown), errors.Is(err, inquiry.ErrNoAdapter):
		w.logError("water: provider inquiry failed", err)
		return errProviderDown
	default:
		w.logError("water: inquiry failed", err)
		return errInternal
	}
}

func (w *Water) logError(msg string, err error) {
	if w.logger != nil {
		w.logger.Error(msg, "error", err)
	}
}
