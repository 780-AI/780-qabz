package grpcserver

import (
	"context"

	pb "qabz/api/pb/qabz"
	"qabz/provider"
)

// NewOrderV2 makes an electricity order (POST /v2/electricity/order, task 052; bill:
// controller/electricity NewOrderV2). The steps and the answers are those of
// utilityOrder.newOrderV2 with the bill type electricity, the same as the water order.
// POST /v1/electricity/order (NewOrder) stays Unimplemented, as for water.
func (e *Electricity) NewOrderV2(ctx context.Context, req *pb.NewOrderBillPayRequest) (*pb.NewOrderResponse, error) {
	u := utilityOrder{orders: e.orders, signer: e.signer, serviceID: e.serviceID, logger: e.logger}
	return u.newOrderV2(ctx, provider.BillTypeElectricity, req)
}
