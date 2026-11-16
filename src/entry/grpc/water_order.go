package grpcserver

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/core/order"
	"qabz/provider"
)

// waterInquiryID is bill's controller.SetInquiryID: the participate code when it is not empty, else
// the bill ID.
func waterInquiryID(req *pb.NewOrderBillPayRequest) string {
	if code := req.GetParticipateCode(); code != "" {
		return code
	}
	return req.GetBillId()
}

// NewOrderV2 makes a water order (POST /v2/water/order, task 048; bill: controller/water NewOrderV2
// and controller.CreateNewOrder). The steps of bill:
//   - The request check (bill: validateNewOrderRequest): billId or payId is set. Else
//     InvalidArgument (HTTP 400) with no log line. @todo bill answers gRPC Unknown and writes one
//     error line. See docs/this_vs_bill.md, item 38.
//   - The order (order.Service.Create): the order provider (Parsian), the "bill already paid"
//     check, the raw inquiry of a numeric billId and payId (else the regular inquiry of the inquiry
//     ID for the price), the fee, the orders row with the status initial. The inquiry ID is
//     waterInquiryID.
//   - The token: the payload of bill with WaterData, signed by the signer.
//
// The answers of each failure are those of POST /v2/tel/order (answerOrderError).
func (w *Water) NewOrderV2(ctx context.Context, req *pb.NewOrderBillPayRequest) (*pb.NewOrderResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if w.orders == nil || w.signer == nil {
		return nil, status.Error(codes.Unimplemented, "method NewOrderV2 not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		return nil, errRejected
	}
	if req.GetBillId() == "" && req.GetPayId() == "" { // req nil: invalid
		return nil, errInvalidParameter
	}
	billType := provider.BillTypeWater
	o, err := w.orders.Create(ctx, order.Request{
		BillType:  billType,
		BillID:    req.GetBillId(),
		PayID:     req.GetPayId(),
		InquiryID: waterInquiryID(req),
		UserID:    claims.UserID,
		Dialer:    claims.Mobile,
	})
	if err != nil {
		return nil, answerOrderError(ctx, w.logger, billType, err)
	}
	payload, err := order.MarshalPayload(order.TokenPayload(o, w.serviceID, order.WaterData(o)))
	if err == nil {
		var token string
		if token, err = w.signer.Sign(payload); err == nil {
			return &pb.NewOrderResponse{ServiceId: w.serviceID, OrderId: token, Price: o.Price}, nil
		}
	}
	if w.logger != nil {
		w.logger.Error(billType+": order token failed", "order_id", o.OrderID, "error", err)
	}
	return nil, errInternal
}
