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

func telOrderReq() *pb.NewOrderTelRequest {
	return &pb.NewOrderTelRequest{Phone: "02188776655", Type: pb.NewOrderTelRequest_midTerm, PaymentType: pb.NewOrderTelRequest_direct}
}

func savedTelOrder() order.Order {
	return order.Order{OrderID: "11111111-2222-3333-4444-555555555544", UserID: billsUser, BillType: provider.BillTypeTelMidterm,
		BillID: "2101234560040", PayID: "12000035", Price: 150000, BasePrice: 120000, Fee: 9000, ProviderFee: 21000,
		ProviderName: "parsian", ProviderID: 51}
}

func newOrderTel(o *fakeOrders, s *fakeSigner, log *errLog) *Tel {
	deps := TelDeps{Orders: o, Signer: s, ServiceID: 105}
	if log != nil {
		deps.Logger = log
	}
	return NewTel(deps)
}

// TestTelNewOrderV2 (task 044): the order request of bill (the phone in the 98 form as the inquiry
// ID, the type midTerm as telmidterm, the IDs of the request, the user and the dialer), the token
// payload with TelData, and the answer.
func TestTelNewOrderV2(t *testing.T) {
	orders := &fakeOrders{o: savedTelOrder()}
	signer := &fakeSigner{}
	req := telOrderReq()
	req.BillId, req.PayId = "2101234560040", "12000035"
	res, err := newOrderTel(orders, signer, nil).NewOrderV2(waterCtx(), req)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []order.Request{{BillType: provider.BillTypeTelMidterm, BillID: "2101234560040", PayID: "12000035",
		InquiryID: "982188776655", UserID: billsUser, Dialer: "989121234567"}}, orders.reqs)
	want, err := order.MarshalPayload(order.TokenPayload(savedTelOrder(), 105, order.TelData(savedTelOrder(), "982188776655")))
	assert.NoError(t, err)
	if assert.Len(t, signer.payloads, 1) {
		assert.Equal(t, string(want), string(signer.payloads[0]))
		for _, part := range []string{`"invoicePhone":"02188776655"`, `"feeAmount":"30000"`, `"hafhashtadFee":"9000"`, `"providerFee":"21000"`,
			`"about":"قبض تلفن ثابت-میان دوره"`, `"type":"telmidterm"`, `"provider":"parsian"`} {
			assert.Contains(t, string(signer.payloads[0]), part)
		}
	}
	assert.Equal(t, uint32(105), res.GetServiceId())
	assert.Equal(t, uint32(150000), res.GetPrice())

	orders = &fakeOrders{o: savedTelOrder()}
	req = telOrderReq()
	req.Type = pb.NewOrderTelRequest_fullTerm
	_, err = newOrderTel(orders, &fakeSigner{}, nil).NewOrderV2(waterCtx(), req)
	assert.NoError(t, err)
	assert.Equal(t, provider.BillTypeTel, orders.reqs[0].BillType)
}

// TestTelNewOrderV2Request: bill's request check (billId or phone; paymentType of the PaymentTerms
// names, so 0 and 1); an unknown type is invalid. No order call; no log line.
func TestTelNewOrderV2Request(t *testing.T) {
	for name, mod := range map[string]func(r *pb.NewOrderTelRequest){
		"no phone no billId": func(r *pb.NewOrderTelRequest) { r.Phone = "" },
		"paymentType 2":      func(r *pb.NewOrderTelRequest) { r.PaymentType = 2 },
		"type 7":             func(r *pb.NewOrderTelRequest) { r.Type = 7 },
	} {
		orders, log := &fakeOrders{o: savedTelOrder()}, &errLog{}
		r := telOrderReq()
		mod(r)
		_, err := newOrderTel(orders, &fakeSigner{}, log).NewOrderV2(waterCtx(), r)
		assert.Equal(t, codes.InvalidArgument, status.Code(err), name)
		assert.Empty(t, orders.reqs, name)
		assert.Empty(t, log.lines, name)
	}
	_, err := newOrderTel(&fakeOrders{}, &fakeSigner{}, nil).NewOrderV2(waterCtx(), nil)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	// Only billId passes the check (bill), with an empty inquiry ID.
	orders := &fakeOrders{o: savedTelOrder()}
	r := telOrderReq()
	r.Phone, r.BillId = "", "2101234560040"
	_, err = newOrderTel(orders, &fakeSigner{}, nil).NewOrderV2(waterCtx(), r)
	assert.NoError(t, err)
	assert.Empty(t, orders.reqs[0].InquiryID)
}

// TestTelNewOrderV2Failures: the answers of the order errors; a bill that is paid already is
// FailedPrecondition with bill's text, no log line.
func TestTelNewOrderV2Failures(t *testing.T) {
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
		_, err := newOrderTel(&fakeOrders{err: c.err}, &fakeSigner{}, log).NewOrderV2(waterCtx(), telOrderReq())
		assert.Equal(t, c.code, status.Code(err), c.err.Error())
		assert.Len(t, log.lines, c.lines, c.err.Error())
	}
	_, err := newOrderTel(&fakeOrders{err: order.ErrAlreadyPaid}, &fakeSigner{}, nil).NewOrderV2(waterCtx(), telOrderReq())
	assert.Equal(t, alreadyPaidText, status.Convert(err).Message())

	log := &errLog{}
	_, err = newOrderTel(&fakeOrders{o: savedTelOrder()}, &fakeSigner{err: errors.New("no key")}, log).NewOrderV2(waterCtx(), telOrderReq())
	assert.Equal(t, codes.Internal, status.Code(err))
	assert.Equal(t, []string{"telmidterm: order token failed"}, log.lines)

	_, err = newOrderTel(&fakeOrders{}, &fakeSigner{}, nil).NewOrderV2(context.Background(), telOrderReq())
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "no user")
	ctx, cancel := context.WithCancel(waterCtx())
	cancel()
	_, err = newOrderTel(&fakeOrders{}, &fakeSigner{}, nil).NewOrderV2(ctx, telOrderReq())
	assert.Equal(t, codes.Canceled, status.Code(err))
}
