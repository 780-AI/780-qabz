package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/core/inquiry"
	"qabz/core/order"
	"qabz/provider"
)

func waterOrderReq() *pb.NewOrderBillPayRequest {
	return &pb.NewOrderBillPayRequest{BillId: "1234567890125", PayId: "2500012340", PaymentType: pb.NewOrderBillPayRequest_direct}
}

func savedWaterOrder() order.Order {
	return order.Order{OrderID: "11111111-2222-3333-4444-555555555548", UserID: billsUser, BillType: provider.BillTypeWater,
		BillID: "1234567890125", PayID: "2500012340", Price: 259000, BasePrice: 250000, Fee: 9000, ProviderName: "parsian", ProviderID: 45}
}

func newOrderWater(o *fakeOrders, s *fakeSigner, log *errLog) *Water {
	deps := WaterDeps{Orders: o, Signer: s, ServiceID: 105}
	if log != nil {
		deps.Logger = log
	}
	return NewWater(deps)
}

// TestWaterNewOrderV2 (task 048): the order request of bill (the bill type water, the IDs of the
// request, the bill ID as the inquiry ID, the user and the dialer), the token payload with
// WaterData, and the answer.
func TestWaterNewOrderV2(t *testing.T) {
	orders := &fakeOrders{o: savedWaterOrder()}
	signer := &fakeSigner{}
	res, err := newOrderWater(orders, signer, nil).NewOrderV2(waterCtx(), waterOrderReq())
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []order.Request{{BillType: provider.BillTypeWater, BillID: "1234567890125", PayID: "2500012340",
		InquiryID: "1234567890125", UserID: billsUser, Dialer: "989121234567"}}, orders.reqs)
	want, err := order.MarshalPayload(order.TokenPayload(savedWaterOrder(), 105, order.WaterData(savedWaterOrder())))
	assert.NoError(t, err)
	if assert.Len(t, signer.payloads, 1) {
		assert.Equal(t, string(want), string(signer.payloads[0]))
		for _, part := range []string{`"invoiceBillID":"1234567890125"`, `"receiptPayID":"2500012340"`, `"feeAmount":"9000"`,
			`"about":"قبض آب"`, `"type":"water"`, `"provider":"parsian"`, `"price":259000`} {
			assert.Contains(t, string(signer.payloads[0]), part)
		}
	}
	assert.Equal(t, uint32(105), res.GetServiceId())
	assert.Equal(t, uint32(259000), res.GetPrice())
	assert.NotEmpty(t, res.GetOrderId())
}

// TestWaterNewOrderV2InquiryID: bill's SetInquiryID: the participate code when it is set, else the
// bill ID.
func TestWaterNewOrderV2InquiryID(t *testing.T) {
	orders := &fakeOrders{o: savedWaterOrder()}
	req := waterOrderReq()
	req.ParticipateCode = "7700112233"
	_, err := newOrderWater(orders, &fakeSigner{}, nil).NewOrderV2(waterCtx(), req)
	if assert.NoError(t, err) && assert.Len(t, orders.reqs, 1) {
		assert.Equal(t, "7700112233", orders.reqs[0].InquiryID)
	}
}

// TestWaterNewOrderV2Request: bill's request check (billId or payId; no paymentType check). A failed
// check is InvalidArgument with no order call and no log line.
func TestWaterNewOrderV2Request(t *testing.T) {
	for name, req := range map[string]*pb.NewOrderBillPayRequest{
		"empty":                   {},
		"nil":                     nil,
		"only a participate code": {ParticipateCode: "7700112233"},
	} {
		orders, log := &fakeOrders{o: savedWaterOrder()}, &errLog{}
		_, err := newOrderWater(orders, &fakeSigner{}, log).NewOrderV2(waterCtx(), req)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), name)
		assert.Empty(t, orders.reqs, name)
		assert.Empty(t, log.lines, name)
	}
	for name, req := range map[string]*pb.NewOrderBillPayRequest{
		"only billId":   {BillId: "1234567890125"},
		"only payId":    {PayId: "2500012340"},
		"paymentType 9": {BillId: "1234567890125", PaymentType: 9},
	} {
		orders := &fakeOrders{o: savedWaterOrder()}
		_, err := newOrderWater(orders, &fakeSigner{}, nil).NewOrderV2(waterCtx(), req)
		assert.NoError(t, err, name)
		assert.Len(t, orders.reqs, 1, name)
	}
}

// TestWaterNewOrderV2Failures: the answers of the order errors are those of POST /v2/tel/order; the
// log lines start with "water: ".
func TestWaterNewOrderV2Failures(t *testing.T) {
	cases := []struct {
		err   error
		code  codes.Code
		lines int
	}{
		{order.ErrAlreadyPaid, codes.FailedPrecondition, 0},
		{order.ErrZeroPrice, codes.FailedPrecondition, 0},
		{fmt.Errorf("x: %w", order.ErrTotalTooLarge), codes.InvalidArgument, 0},
		{fmt.Errorf("pick: %w", provider.ErrNoProvider), codes.Unavailable, 1},
		{order.ErrNoInquiry, codes.Internal, 1},
		{errors.New("db down"), codes.Internal, 1},
		{&order.InquiryError{Err: fmt.Errorf("x: %w", provider.ErrProviderDown)}, codes.Unavailable, 1},
		{&order.InquiryError{Err: inquiry.ErrInvalidBillID}, codes.InvalidArgument, 0},
	}
	for _, c := range cases {
		log := &errLog{}
		_, err := newOrderWater(&fakeOrders{err: c.err}, &fakeSigner{}, log).NewOrderV2(waterCtx(), waterOrderReq())
		assert.Equal(t, c.code, status.Code(err), c.err.Error())
		assert.Len(t, log.lines, c.lines, c.err.Error())
	}
	log := &errLog{}
	_, err := newOrderWater(&fakeOrders{err: fmt.Errorf("pick: %w", provider.ErrNoProvider)}, &fakeSigner{}, log).NewOrderV2(waterCtx(), waterOrderReq())
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, []string{"water: no enabled provider for order"}, log.lines)

	log = &errLog{}
	_, err = newOrderWater(&fakeOrders{o: savedWaterOrder()}, &fakeSigner{err: errors.New("no key")}, log).NewOrderV2(waterCtx(), waterOrderReq())
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, []string{"water: order token failed"}, log.lines)

	_, err = newOrderWater(&fakeOrders{}, &fakeSigner{}, nil).NewOrderV2(context.Background(), waterOrderReq())
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "no user")
	ctx, cancel := context.WithCancel(waterCtx())
	cancel()
	_, err = newOrderWater(&fakeOrders{}, &fakeSigner{}, nil).NewOrderV2(ctx, waterOrderReq())
	assert.Equal(t, codes.Canceled, status.Code(err))
	_, err = NewWater(WaterDeps{}).NewOrderV2(waterCtx(), waterOrderReq())
	assert.Equal(t, codes.Unimplemented, status.Code(err), "no orders or signer")
	_, err = newOrderWater(&fakeOrders{}, &fakeSigner{}, nil).NewOrder(waterCtx(), waterOrderReq())
	assert.Equal(t, codes.Unimplemented, status.Code(err), "POST /v1/water/order stays Unimplemented")
}
