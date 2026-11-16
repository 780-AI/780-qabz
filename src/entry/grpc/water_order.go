package grpcserver

import (
	"context"

	pb "qabz/api/pb/qabz"
	"qabz/provider"
)

// NewOrderV2 makes a water order (POST /v2/water/order, task 048; bill: controller/water
// NewOrderV2). The steps and the answers are those of utilityOrder.newOrderV2 with the bill type
// water.
func (w *Water) NewOrderV2(ctx context.Context, req *pb.NewOrderBillPayRequest) (*pb.NewOrderResponse, error) {
	u := utilityOrder{orders: w.orders, signer: w.signer, serviceID: w.serviceID, logger: w.logger}
	return u.newOrderV2(ctx, provider.BillTypeWater, req)
}
