package grpcserver

import (
	"context"

	pb "qabz/api/pb/qabz"
	"qabz/provider"
)

// NewOrder makes a water order (POST /v1/water/order, task 048; bill: controller/water
// NewOrderV2). The steps and the answers are those of utilityOrder.newOrder with the bill type
// water.
func (w *Water) NewOrder(ctx context.Context, req *pb.NewOrderBillPayRequest) (*pb.NewOrderResponse, error) {
	u := utilityOrder{orders: w.orders, signer: w.signer, serviceID: w.serviceID, logger: w.logger}
	return u.newOrder(ctx, provider.BillTypeWater, req)
}
