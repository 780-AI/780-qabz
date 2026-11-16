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
	"qabz/core/order"
	"qabz/provider"
)

func electricityOrderReq() *pb.NewOrderBillPayRequest {
	return &pb.NewOrderBillPayRequest{BillId: "1234567890125", PayId: "1800012345", PaymentType: pb.NewOrderBillPayRequest_direct}
}

func savedElectricityOrder() order.Order {
	return order.Order{OrderID: "11111111-2222-3333-4444-555555555552", UserID: billsUser, BillType: provider.BillTypeElectricity,
		BillID: "1234567890125", PayID: "1800012345", Price: 189000, BasePrice: 180000, Fee: 9000, ProviderName: "parsian", ProviderID: 47}
}

func newOrderElectricity(o *fakeOrders, s *fakeSigner, log *errLog) *Electricity {
	deps := ElectricityDeps{Orders: o, Signer: s, ServiceID: 105}
	if log != nil {
		deps.Logger = log
	}
	return NewElectricity(deps)
}

// TestElectricityNewOrder (task 052): the order request of bill (the bill type electricity, the
// IDs of the request, the bill ID as the inquiry ID, the user and the dialer), the token payload
// with UtilityData (the about text of bill with its first space, D7 (a)), and the answer.
func TestElectricityNewOrder(t *testing.T) {
	orders := &fakeOrders{o: savedElectricityOrder()}
	signer := &fakeSigner{}
	res, err := newOrderElectricity(orders, signer, nil).NewOrder(waterCtx(), electricityOrderReq())
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []order.Request{{BillType: provider.BillTypeElectricity, BillID: "1234567890125", PayID: "1800012345",
		InquiryID: "1234567890125", UserID: billsUser, Dialer: "989121234567"}}, orders.reqs)
	want, err := order.MarshalPayload(order.TokenPayload(savedElectricityOrder(), 105, order.UtilityData(savedElectricityOrder())))
	assert.NoError(t, err)
	if assert.Len(t, signer.payloads, 1) {
		assert.Equal(t, string(want), string(signer.payloads[0]))
		for _, part := range []string{`"invoiceBillID":"1234567890125"`, `"receiptPayID":"1800012345"`, `"feeAmount":"9000"`,
			`"about":" قبض برق"`, `"aboutIcon":"https://public.780.ir/dl/Operator/Bargh.svg"`, `"type":"electricity"`,
			`"provider":"parsian"`, `"price":189000`} {
			assert.Contains(t, string(signer.payloads[0]), part)
		}
	}
	assert.Equal(t, uint32(105), res.GetServiceId())
	assert.Equal(t, uint32(189000), res.GetPrice())
	assert.NotEmpty(t, res.GetOrderId())
}

// TestElectricityNewOrderInquiryID: bill's SetInquiryID: the participate code when it is set,
// else the bill ID.
func TestElectricityNewOrderInquiryID(t *testing.T) {
	orders := &fakeOrders{o: savedElectricityOrder()}
	req := electricityOrderReq()
	req.ParticipateCode = "7700112233"
	_, err := newOrderElectricity(orders, &fakeSigner{}, nil).NewOrder(waterCtx(), req)
	if assert.NoError(t, err) && assert.Len(t, orders.reqs, 1) {
		assert.Equal(t, "7700112233", orders.reqs[0].InquiryID)
	}
}

// TestElectricityNewOrderRequest: bill's request check (billId or payId). A failed check is
// InvalidArgument with no order call and no log line.
func TestElectricityNewOrderRequest(t *testing.T) {
	for name, req := range map[string]*pb.NewOrderBillPayRequest{
		"empty":                   {},
		"nil":                     nil,
		"only a participate code": {ParticipateCode: "7700112233"},
	} {
		orders, log := &fakeOrders{o: savedElectricityOrder()}, &errLog{}
		_, err := newOrderElectricity(orders, &fakeSigner{}, log).NewOrder(waterCtx(), req)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), name)
		assert.Empty(t, orders.reqs, name)
		assert.Empty(t, log.lines, name)
	}
	for name, req := range map[string]*pb.NewOrderBillPayRequest{
		"only billId": {BillId: "1234567890125"},
		"only payId":  {PayId: "1800012345"},
	} {
		orders := &fakeOrders{o: savedElectricityOrder()}
		_, err := newOrderElectricity(orders, &fakeSigner{}, nil).NewOrder(waterCtx(), req)
		assert.NoError(t, err, name)
		assert.Len(t, orders.reqs, 1, name)
	}
}

// TestElectricityNewOrderFailures: the answers of the order errors are those of POST
// /v1/water/order; the log lines start with "electricity: ".
func TestElectricityNewOrderFailures(t *testing.T) {
	for _, c := range []struct {
		err  error
		code codes.Code
	}{
		{order.ErrAlreadyPaid, codes.FailedPrecondition},
		{order.ErrZeroPrice, codes.FailedPrecondition},
		{fmt.Errorf("pick: %w", provider.ErrNoProvider), codes.Unavailable},
		{&order.InquiryError{Err: fmt.Errorf("x: %w", provider.ErrProviderDown)}, codes.Unavailable},
	} {
		_, err := newOrderElectricity(&fakeOrders{err: c.err}, &fakeSigner{}, &errLog{}).NewOrder(waterCtx(), electricityOrderReq())
		assert.Equal(t, c.code, status.Code(err), c.err.Error())
	}
	log := &errLog{}
	_, err := newOrderElectricity(&fakeOrders{err: fmt.Errorf("pick: %w", provider.ErrNoProvider)}, &fakeSigner{}, log).NewOrder(waterCtx(), electricityOrderReq())
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, []string{"electricity: no enabled provider for order"}, log.lines)

	log = &errLog{}
	_, err = newOrderElectricity(&fakeOrders{o: savedElectricityOrder()}, &fakeSigner{err: errors.New("no key")}, log).NewOrder(waterCtx(), electricityOrderReq())
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, []string{"electricity: order token failed"}, log.lines)

	_, err = newOrderElectricity(&fakeOrders{}, &fakeSigner{}, nil).NewOrder(context.Background(), electricityOrderReq())
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "no user")
	ctx, cancel := context.WithCancel(waterCtx())
	cancel()
	_, err = newOrderElectricity(&fakeOrders{}, &fakeSigner{}, nil).NewOrder(ctx, electricityOrderReq())
	assert.Equal(t, codes.Canceled, status.Code(err))
	_, err = NewElectricity(ElectricityDeps{}).NewOrder(waterCtx(), electricityOrderReq())
	assert.Equal(t, codes.Unimplemented, status.Code(err), "no orders or signer")
}
