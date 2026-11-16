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

func gasOrderReq() *pb.NewOrderBillPayRequest {
	return &pb.NewOrderBillPayRequest{BillId: "1234567890133", PayId: "1800012345", PaymentType: pb.NewOrderBillPayRequest_direct}
}

func savedGasOrder() order.Order {
	return order.Order{OrderID: "11111111-2222-3333-4444-555555555552", UserID: billsUser, BillType: provider.BillTypeGas,
		BillID: "1234567890133", PayID: "1800012345", Price: 189000, BasePrice: 180000, Fee: 9000, ProviderName: "parsian", ProviderID: 46}
}

func newOrderGas(o *fakeOrders, s *fakeSigner, log *errLog) *Gas {
	deps := GasDeps{Orders: o, Signer: s, ServiceID: 105}
	if log != nil {
		deps.Logger = log
	}
	return NewGas(deps)
}

// TestGasNewOrder (task 053): the order request of bill (the bill type gas, the
// IDs of the request, the bill ID as the inquiry ID, the user and the dialer), the token payload
// with UtilityData (bill's gas texts), and the answer.
func TestGasNewOrder(t *testing.T) {
	orders := &fakeOrders{o: savedGasOrder()}
	signer := &fakeSigner{}
	res, err := newOrderGas(orders, signer, nil).NewOrder(waterCtx(), gasOrderReq())
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []order.Request{{BillType: provider.BillTypeGas, BillID: "1234567890133", PayID: "1800012345",
		InquiryID: "1234567890133", UserID: billsUser, Dialer: "989121234567"}}, orders.reqs)
	want, err := order.MarshalPayload(order.TokenPayload(savedGasOrder(), 105, order.UtilityData(savedGasOrder())))
	assert.NoError(t, err)
	if assert.Len(t, signer.payloads, 1) {
		assert.Equal(t, string(want), string(signer.payloads[0]))
		for _, part := range []string{`"invoiceBillID":"1234567890133"`, `"receiptPayID":"1800012345"`, `"feeAmount":"9000"`,
			`"about":"قبض گاز"`, `"aboutIcon":"https://public.780.ir/dl/Operator/Gas.svg"`, `"type":"gas"`,
			`"provider":"parsian"`, `"price":189000`} {
			assert.Contains(t, string(signer.payloads[0]), part)
		}
	}
	assert.Equal(t, uint32(105), res.GetServiceId())
	assert.Equal(t, uint32(189000), res.GetPrice())
	assert.NotEmpty(t, res.GetOrderId())
}

// TestGasNewOrderInquiryID: bill's SetInquiryID: the participate code when it is set,
// else the bill ID.
func TestGasNewOrderInquiryID(t *testing.T) {
	orders := &fakeOrders{o: savedGasOrder()}
	req := gasOrderReq()
	req.ParticipateCode = "7700112233"
	_, err := newOrderGas(orders, &fakeSigner{}, nil).NewOrder(waterCtx(), req)
	if assert.NoError(t, err) && assert.Len(t, orders.reqs, 1) {
		assert.Equal(t, "7700112233", orders.reqs[0].InquiryID)
	}
}

// TestGasNewOrderRequest: bill's order request check (billId or payId), not the gas inquiry check:
// a request with only billId (invalid for POST /v1/gas/inquiry) is an order. A failed check is
// InvalidArgument with no order call and no log line.
func TestGasNewOrderRequest(t *testing.T) {
	for name, req := range map[string]*pb.NewOrderBillPayRequest{
		"empty":                   {},
		"nil":                     nil,
		"only a participate code": {ParticipateCode: "7700112233"},
	} {
		orders, log := &fakeOrders{o: savedGasOrder()}, &errLog{}
		_, err := newOrderGas(orders, &fakeSigner{}, log).NewOrder(waterCtx(), req)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), name)
		assert.Empty(t, orders.reqs, name)
		assert.Empty(t, log.lines, name)
	}
	for name, req := range map[string]*pb.NewOrderBillPayRequest{
		"only billId": {BillId: "1234567890133"},
		"only payId":  {PayId: "1800012345"},
	} {
		orders := &fakeOrders{o: savedGasOrder()}
		_, err := newOrderGas(orders, &fakeSigner{}, nil).NewOrder(waterCtx(), req)
		assert.NoError(t, err, name)
		assert.Len(t, orders.reqs, 1, name)
	}
}

// TestGasNewOrderFailures: the answers of the order errors are those of POST
// /v1/water/order; the log lines start with "gas: ".
func TestGasNewOrderFailures(t *testing.T) {
	for _, c := range []struct {
		err  error
		code codes.Code
	}{
		{order.ErrAlreadyPaid, codes.FailedPrecondition},
		{order.ErrZeroPrice, codes.FailedPrecondition},
		{fmt.Errorf("pick: %w", provider.ErrNoProvider), codes.Unavailable},
		{&order.InquiryError{Err: fmt.Errorf("x: %w", provider.ErrProviderDown)}, codes.Unavailable},
	} {
		_, err := newOrderGas(&fakeOrders{err: c.err}, &fakeSigner{}, &errLog{}).NewOrder(waterCtx(), gasOrderReq())
		assert.Equal(t, c.code, status.Code(err), c.err.Error())
	}
	log := &errLog{}
	_, err := newOrderGas(&fakeOrders{err: fmt.Errorf("pick: %w", provider.ErrNoProvider)}, &fakeSigner{}, log).NewOrder(waterCtx(), gasOrderReq())
	assert.Equal(t, codes.Unavailable, status.Code(err))
	assert.Equal(t, []string{"gas: no enabled provider for order"}, log.lines)

	log = &errLog{}
	_, err = newOrderGas(&fakeOrders{o: savedGasOrder()}, &fakeSigner{err: errors.New("no key")}, log).NewOrder(waterCtx(), gasOrderReq())
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, []string{"gas: order token failed"}, log.lines)

	_, err = newOrderGas(&fakeOrders{}, &fakeSigner{}, nil).NewOrder(context.Background(), gasOrderReq())
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "no user")
	ctx, cancel := context.WithCancel(waterCtx())
	cancel()
	_, err = newOrderGas(&fakeOrders{}, &fakeSigner{}, nil).NewOrder(ctx, gasOrderReq())
	assert.Equal(t, codes.Canceled, status.Code(err))
	_, err = NewGas(GasDeps{}).NewOrder(waterCtx(), gasOrderReq())
	assert.Equal(t, codes.Unimplemented, status.Code(err), "no orders or signer")
}
