package grpcserver

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/helpers"
	"qabz/core/order"
	"qabz/provider"
)

// telOrderBillTypes maps the request type to the order bill type (bill: telephoneBillTypeMapper).
var telOrderBillTypes = map[pb.NewOrderTelRequest_PaymentTerms]string{
	pb.NewOrderTelRequest_midTerm:  provider.BillTypeTelMidterm,
	pb.NewOrderTelRequest_fullTerm: provider.BillTypeTel,
}

// validTelOrder is the request check of bill (createNewOrderTelephone,
// validateNewOrderTelephoneRequest): billId or phone is set, and paymentType is a value of the
// PaymentTerms enum. bill checks paymentType (not type) against the PaymentTerms names, so the
// paymentType values 0 and 1 pass; Qabz does the same.
func validTelOrder(req *pb.NewOrderTelRequest) bool {
	if req.GetBillId() == "" && req.GetPhone() == "" {
		return false
	}
	_, ok := pb.NewOrderTelRequest_PaymentTerms_name[int32(req.GetPaymentType())]
	return ok
}

// NewOrderV2 makes a landline order (POST /v2/tel/order, task 044; bill: controller/telephone
// NewOrderV2). The steps of bill:
//   - The phone is converted to the 98 form (helpers.NormalizePhone, bill's NormalizePhoneNumber).
//   - The request check is validTelOrder: InvalidArgument (HTTP 400), no log line.
//   - The bill type is telmidterm for the type midTerm and tel for fullTerm. Another type is
//     InvalidArgument. @todo bill gives an empty bill type and fails at the provider lookup (gRPC
//     Unknown). See docs/this_vs_bill.md, item 38.
//   - The order (order.Service.Create): the order provider (Parsian), the "bill already paid"
//     check, the inquiry of the number for the price, the fee and the Karashop provider fee, the
//     orders row with the status initial. The inquiry ID is the converted phone.
//   - The token: the payload of bill with TelData, signed by the signer.
//
// The answers of each failure are those of POST /v2/mobile/order (answerOrderError); a bill that is
// paid already is FailedPrecondition with bill's text.
func (t *Tel) NewOrderV2(ctx context.Context, req *pb.NewOrderTelRequest) (*pb.NewOrderResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if t.orders == nil || t.signer == nil {
		return nil, status.Error(codes.Unimplemented, "method NewOrderV2 not available")
	}
	claims, ok := auth.ClaimsFrom(ctx)
	if !ok || claims.UserID == "" {
		return nil, errRejected
	}
	if !validTelOrder(req) { // req nil: no billId and no phone, invalid
		return nil, errInvalidParameter
	}
	billType, ok := telOrderBillTypes[req.GetType()]
	if !ok {
		return nil, errInvalidParameter
	}
	phone := helpers.NormalizePhone(req.GetPhone())
	o, err := t.orders.Create(ctx, order.Request{
		BillType:  billType,
		BillID:    req.GetBillId(),
		PayID:     req.GetPayId(),
		InquiryID: phone,
		UserID:    claims.UserID,
		Dialer:    claims.Mobile,
	})
	if err != nil {
		return nil, answerOrderError(ctx, t.logger, billType, err)
	}
	payload, err := order.MarshalPayload(order.TokenPayload(o, t.serviceID, order.TelData(o, phone)))
	if err == nil {
		var token string
		if token, err = t.signer.Sign(payload); err == nil {
			return &pb.NewOrderResponse{ServiceId: t.serviceID, OrderId: token, Price: o.Price}, nil
		}
	}
	if t.logger != nil {
		t.logger.Error(billType+": order token failed", "order_id", o.OrderID, "error", err)
	}
	return nil, errInternal
}
