package grpcserver

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/helpers"
	"qabz/core/order"
	"qabz/provider"
)

// OrderCreator saves an order (order.Service).
type OrderCreator interface {
	Create(ctx context.Context, req order.Request) (order.Order, error)
}

// TokenSigner signs the payload of an order token (crypto.Signer).
type TokenSigner interface {
	Sign(payload []byte) (string, error)
}

// zeroPriceText is bill's text of an order whose price is 0 (errorHandler AmountEqualToZero, code
// 10): "the amount of the bill is zero".
const zeroPriceText = "مبلغ قبض صفر میباشد"

// errZeroPrice is the answer of an order whose price is 0. The client sees the text of bill.
// @todo bill answers gRPC Unknown (HTTP 500). See docs/this_vs_bill.md, item 38.
var errZeroPrice = status.Error(codes.FailedPrecondition, zeroPriceText)

// alreadyPaidText is bill's text of a bill that is paid already (BillAlreadyPaidErrorType, task 044).
const alreadyPaidText = "قبض وارد شده قبلاً پرداخت شده است، لطفا تا تسویه شدن قبض صبر کنید"

// errAlreadyPaid is the answer of an order of a bill with a pending or successful order (task 044):
// FailedPrecondition (HTTP 400) with bill's text. @todo bill answers gRPC Unknown (HTTP 500). See
// docs/this_vs_bill.md, item 38.
var errAlreadyPaid = status.Error(codes.FailedPrecondition, alreadyPaidText)

// orderOperators gives the bill type of each operator and payment term of a mobile order (bill:
// controller/phone phoneBillTypeMapper). It is the one table of the endpoint: task 039 filled the
// entry of Irancell, task 054 the entry of MCI. An operator with a nil map is known but not available
// yet (Unimplemented: rightel until its own task); an operator that is not in the table (unknown) and a payment
// term that is not in the map of the operator are invalid (InvalidArgument).
//
//	bill: mci midTerm -> "mobilemidterm", fullTerm -> "mobile"; rightel midTerm -> "mobilemidtermRightel";
//	irancell midTerm and fullTerm -> "irancell".
var orderOperators = map[pb.NewOrderPhoneRequest_OperatorEnum]map[pb.NewOrderPhoneRequest_PaymentTerms]string{
	pb.NewOrderPhoneRequest_irancell: {
		pb.NewOrderPhoneRequest_midTerm:  provider.BillTypeIrancell,
		pb.NewOrderPhoneRequest_fullTerm: provider.BillTypeIrancell,
	},
	pb.NewOrderPhoneRequest_mci: {
		pb.NewOrderPhoneRequest_midTerm:  provider.BillTypeMobileMidterm,
		pb.NewOrderPhoneRequest_fullTerm: provider.BillTypeMobile,
	},
	pb.NewOrderPhoneRequest_rightel: nil,
}

// validPhoneOrder is the request check of bill (controller/phone validateNewOrderPhoneRequest) on a
// phone that helpers.NormalizeMobile already converted: the operator is a value of the enum (as
// bill, the value unknown passes), the payment type is a value of the PaymentTerms enum (bill reads
// it with the name table of the payment terms; both enums have the values 0 and 1, so the result
// is the same), the phone is a mobile number in the 98 form, and billId or phone is there (always
// true with a valid phone). The type, the price, the payId and the billId are not checked.
func validPhoneOrder(req *pb.NewOrderPhoneRequest, phone string) bool {
	if !helpers.ValidMobile(phone) {
		return false
	}
	_, okOperator := pb.NewOrderPhoneRequest_OperatorEnum_name[int32(req.GetOperator())]
	_, okPayment := pb.NewOrderPhoneRequest_PaymentTerms_name[int32(req.GetPaymentType())]
	return okOperator && okPayment
}

// NewOrder saves the order of a mobile bill and answers the signed order token (POST
// /v1/mobile/order; task 039: the Irancell operator; task 054: the MCI operator). The steps are those of bill
// (controller/phone NewOrderV2 and createNewOrderPhone):
//  1. The phone is converted to the 98 form (helpers.NormalizeMobile, task 038) and the request is
//     checked (validPhoneOrder): InvalidArgument, no log line.
//     @todo bill accepts only the 98 form here. See docs/this_vs_bill.md, item 35.
//  2. The operator and the payment term give the bill type (orderOperators).
//  3. The order flow (order.Service.Create) saves the order: the order provider, the price, the
//     fee and the orders row.
//  4. The token: the payload of bill (order.TokenPayload with order.PhoneData), signed RS256.
//  5. The answer: serviceId, orderId = the token, price = the total.
//
// The answers of a failure (@todo bill answers each of them with gRPC Unknown, HTTP 500, and a
// Persian text; see docs/this_vs_bill.md, item 38):
//   - No user: PermissionDenied (as the inquiry; item 14).
//   - An invalid request, the operator unknown, a payment term that the operator does not have, a
//     total that does not fit the price field: InvalidArgument, no log line.
//   - rightel: Unimplemented, no log line.
//   - A price of 0: FailedPrecondition with the text of bill, no log line.
//   - No enabled order provider: Unavailable (the text of the inquiry), one log line.
//   - A failed inquiry for the price: the answer of the inquiry endpoint (answerInquiryError).
//   - An inquiry answer with no inquiry of the bill type, a failed read or write, a failed token:
//     Internal, one log line.
//
// The log lines start with the bill type (for example "irancell: order failed").
func (p *Phone) NewOrder(ctx context.Context, req *pb.NewOrderPhoneRequest) (*pb.NewOrderResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if p.orders == nil || p.signer == nil {
		return nil, status.Error(codes.Unimplemented, "method NewOrder not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		// @todo bill saves an order with no user_id and fails at the insert (gRPC Unknown). See
		// docs/this_vs_bill.md, item 38.
		return nil, errRejected
	}
	phone := helpers.NormalizeMobile(req.GetPhone()) // "" when it is not a mobile number
	if !validPhoneOrder(req, phone) {                // req nil: an empty phone, invalid
		return nil, errInvalidParameter
	}
	terms, known := orderOperators[req.GetOperator()]
	if !known {
		return nil, errInvalidParameter
	}
	if terms == nil {
		return nil, status.Error(codes.Unimplemented, "order of the operator "+req.GetOperator().String()+" is not available yet")
	}
	billType, ok := terms[req.GetType()]
	if !ok {
		return nil, errInvalidParameter
	}
	operator := req.GetOperator().String()
	o, err := p.orders.Create(ctx, order.Request{
		BillType:  billType,
		BillID:    req.GetBillId(),
		PayID:     req.GetPayId(),
		InquiryID: phone,
		UserID:    claims.UserID,
		Dialer:    claims.Mobile,
		Option:    operator,
		Price:     uint64(req.GetPrice()), // as bill: a negative price is a very large one
	})
	if err != nil {
		return nil, p.answerOrderError(ctx, billType, err)
	}
	payload, err := order.MarshalPayload(order.TokenPayload(o, p.serviceID, order.PhoneData(o, phone, operator)))
	if err == nil {
		var token string
		if token, err = p.signer.Sign(payload); err == nil {
			return &pb.NewOrderResponse{ServiceId: p.serviceID, OrderId: token, Price: o.Price}, nil
		}
	}
	if p.logger != nil {
		p.logger.Error(billType+": order token failed", "order_id", o.OrderID, "error", err)
	}
	return nil, errInternal
}

// answerOrderError is the answer of a failed order.Service.Create (see NewOrder).
func (p *Phone) answerOrderError(ctx context.Context, billType string, err error) error {
	return answerOrderError(ctx, p.logger, billType, err)
}

// answerOrderError maps an error of order.Service.Create to the answer of the order endpoints
// (task 039 design decision 2; task 044: ErrAlreadyPaid is FailedPrecondition with bill's text).
func answerOrderError(ctx context.Context, logger ErrorLogger, billType string, err error) error {
	logError := func(msg string) {
		if logger != nil {
			logger.Error(billType+": "+msg, "error", err)
		}
	}
	var inqErr *order.InquiryError
	switch {
	case errors.As(err, &inqErr) && inqErr != nil:
		return answerInquiryError(ctx, logger, billType, inqErr.Err)
	case ctx.Err() != nil:
		return status.FromContextError(ctx.Err()).Err() // the client left: not a server error
	case errors.Is(err, order.ErrTotalTooLarge):
		return errInvalidParameter
	case errors.Is(err, order.ErrZeroPrice):
		return errZeroPrice
	case errors.Is(err, order.ErrAlreadyPaid):
		return errAlreadyPaid
	case errors.Is(err, provider.ErrNoProvider):
		logError("no enabled provider for order")
		return errProviderDown
	case errors.Is(err, order.ErrNoInquiry):
		logError("order inquiry has no inquiry of the bill type")
		return errInternal
	default:
		logError("order failed")
		return errInternal
	}
}
