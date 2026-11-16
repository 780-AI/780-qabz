package grpcserver

import (
	"context"

	pb "qabz/api/pb/qabz"
	"qabz/provider"
)

// NewOrder makes a gas order (POST /v1/gas/order, task 053; bill: controller/gas NewOrderV2). The
// steps and the answers are those of utilityOrder.newOrder with the bill type gas, the same as the
// water and the electricity order: the request check is bill's order check (billId or payId), not
// the gas inquiry check (validGasInquiry); the inquiry ID is the participate code, else the bill ID.
func (g *Gas) NewOrder(ctx context.Context, req *pb.NewOrderBillPayRequest) (*pb.NewOrderResponse, error) {
	u := utilityOrder{orders: g.orders, signer: g.signer, serviceID: g.serviceID, logger: g.logger}
	return u.newOrder(ctx, provider.BillTypeGas, req)
}
