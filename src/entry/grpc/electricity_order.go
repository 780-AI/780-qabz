package grpcserver

import (
	"context"

	pb "qabz/api/pb/qabz"
	"qabz/provider"
)

// NewOrder makes an electricity order (POST /v1/electricity/order, task 052; bill:
// controller/electricity NewOrderV2). The steps and the answers are those of
// utilityOrder.newOrder with the bill type electricity, the same as the water order.
func (e *Electricity) NewOrder(ctx context.Context, req *pb.NewOrderBillPayRequest) (*pb.NewOrderResponse, error) {
	u := utilityOrder{orders: e.orders, signer: e.signer, serviceID: e.serviceID, logger: e.logger}
	return u.newOrder(ctx, provider.BillTypeElectricity, req)
}
