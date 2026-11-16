package grpcserver

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/core/order"
)

// orderInquiryID is bill's controller.SetInquiryID: the participate code when it is not empty, else
// the bill ID.
func orderInquiryID(req *pb.NewOrderBillPayRequest) string {
	if code := req.GetParticipateCode(); code != "" {
		return code
	}
	return req.GetBillId()
}

// utilityOrder is what NewOrder of a water, an electricity and a gas bill needs (bill: all three are
// controller.CreateNewOrder and the same token data keys; tasks 048, 052 and 053).
type utilityOrder struct {
	orders    OrderCreator
	signer    TokenSigner
	serviceID uint32
	logger    ErrorLogger
}

// newOrder makes an order of billType (POST /v1/water/order, task 048; POST /v1/electricity/order,
// task 052; POST /v1/gas/order, task 053; bill: controller/water, controller/electricity and
// controller/gas NewOrderV2 and controller.CreateNewOrder). The steps of bill:
//   - The request check (bill: validateNewOrderRequest): billId or payId is set. Else
//     InvalidArgument (HTTP 400) with no log line. @todo bill answers gRPC Unknown and writes one
//     error line. See docs/this_vs_bill.md, item 38.
//   - The order (order.Service.Create): the order provider (Parsian), the "bill already paid"
//     check, the raw inquiry of a numeric billId and payId (else the regular inquiry of the inquiry
//     ID for the price), the fee, the orders row with the status initial. The inquiry ID is
//     orderInquiryID.
//   - The token: the payload of bill with order.UtilityData, signed by the signer.
//
// The answers of each failure are those of POST /v1/tel/order (answerOrderError); the log lines
// start with the bill type.
func (u utilityOrder) newOrder(ctx context.Context, billType string, req *pb.NewOrderBillPayRequest) (*pb.NewOrderResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if u.orders == nil || u.signer == nil {
		return nil, status.Error(codes.Unimplemented, "method NewOrder not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		return nil, errRejected
	}
	if req.GetBillId() == "" && req.GetPayId() == "" { // req nil: invalid
		return nil, errInvalidParameter
	}
	o, err := u.orders.Create(ctx, order.Request{
		BillType:  billType,
		BillID:    req.GetBillId(),
		PayID:     req.GetPayId(),
		InquiryID: orderInquiryID(req),
		UserID:    claims.UserID,
		Dialer:    claims.Mobile,
	})
	if err != nil {
		return nil, answerOrderError(ctx, u.logger, billType, err)
	}
	payload, err := order.MarshalPayload(order.TokenPayload(o, u.serviceID, order.UtilityData(o)))
	if err == nil {
		var token string
		if token, err = u.signer.Sign(payload); err == nil {
			return &pb.NewOrderResponse{ServiceId: u.serviceID, OrderId: token, Price: o.Price}, nil
		}
	}
	if u.logger != nil {
		u.logger.Error(billType+": order token failed", "order_id", o.OrderID, "error", err)
	}
	return nil, errInternal
}
