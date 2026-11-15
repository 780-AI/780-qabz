package grpcserver

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/core/inquiry"
	"qabz/core/order"
	"qabz/provider"
)

// fakeOrders records each order request and answers with o and err.
type fakeOrders struct {
	reqs []order.Request
	o    order.Order
	err  error
}

func (f *fakeOrders) Create(_ context.Context, req order.Request) (order.Order, error) {
	f.reqs = append(f.reqs, req)
	return f.o, f.err
}

// fakeSigner gives "h.<base64url payload>.s", so a test can read the payload back.
type fakeSigner struct {
	payloads [][]byte
	err      error
}

func (f *fakeSigner) Sign(payload []byte) (string, error) {
	f.payloads = append(f.payloads, payload)
	if f.err != nil {
		return "", f.err
	}
	return "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s", nil
}

func orderReq() *pb.NewOrderPhoneRequest {
	return &pb.NewOrderPhoneRequest{Phone: "09350000391", Operator: pb.NewOrderPhoneRequest_irancell,
		Type: pb.NewOrderPhoneRequest_fullTerm, PaymentType: pb.NewOrderPhoneRequest_direct, Price: 450000}
}

func savedOrder() order.Order {
	return order.Order{OrderID: "11111111-2222-3333-4444-555555555555", UserID: billsUser, BillType: "irancell", PayID: "irancell",
		Price: 459000, BasePrice: 450000, Fee: 9000, ProviderName: "irancell", ProviderID: 26}
}

func newOrderPhone(o *fakeOrders, s *fakeSigner, log *errLog) *Phone {
	deps := PhoneDeps{Orders: o, Signer: s, ServiceID: 105}
	if log != nil {
		deps.Logger = log
	}
	return NewPhone(deps)
}

// TestNewOrderV2MapsRequestAndAnswer: the order request of bill (the phone in the 98 form, the
// operator text as the option and the "service", the user and the dialer of the token), the token
// payload of bill and the answer.
func TestNewOrderV2MapsRequestAndAnswer(t *testing.T) {
	orders := &fakeOrders{o: savedOrder()}
	signer := &fakeSigner{}
	res, err := newOrderPhone(orders, signer, nil).NewOrderV2(waterCtx(), orderReq())
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, []order.Request{{BillType: "irancell", InquiryID: "989350000391", UserID: billsUser, Dialer: "989121234567",
		Option: "irancell", Price: 450000}}, orders.reqs)
	if !assert.Len(t, signer.payloads, 1) {
		return
	}
	want, err := order.MarshalPayload(order.TokenPayload(savedOrder(), 105, order.PhoneData(savedOrder(), "989350000391", "irancell")))
	assert.NoError(t, err)
	assert.Equal(t, string(want), string(signer.payloads[0]))
	assert.Contains(t, string(signer.payloads[0]), `"phone":"989350000391"`)
	assert.Contains(t, string(signer.payloads[0]), `"receiptMobile":"09350000391"`)
	assert.Equal(t, uint32(105), res.GetServiceId())
	assert.Equal(t, uint32(459000), res.GetPrice())
	assert.Equal(t, "h."+base64.RawURLEncoding.EncodeToString(want)+".s", res.GetOrderId())
}

// TestNewOrderV2Request: the request check of bill and the operator table. No call to the order
// flow for a rejected request; no log line.
func TestNewOrderV2Request(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(r *pb.NewOrderPhoneRequest)
		code   codes.Code
		calls  int
	}{
		{"valid 98 form", func(r *pb.NewOrderPhoneRequest) { r.Phone = "989350000391" }, codes.OK, 1},
		{"midTerm is irancell too", func(r *pb.NewOrderPhoneRequest) { r.Type = pb.NewOrderPhoneRequest_midTerm }, codes.OK, 1},
		{"wallet payment", func(r *pb.NewOrderPhoneRequest) { r.PaymentType = pb.NewOrderPhoneRequest_wallet }, codes.OK, 1},
		{"no price, no ids", func(r *pb.NewOrderPhoneRequest) { r.Price = 0 }, codes.OK, 1},
		{"empty phone", func(r *pb.NewOrderPhoneRequest) { r.Phone = "" }, codes.InvalidArgument, 0},
		{"landline", func(r *pb.NewOrderPhoneRequest) { r.Phone = "982188776655" }, codes.InvalidArgument, 0},
		{"operator out of the enum", func(r *pb.NewOrderPhoneRequest) { r.Operator = 9 }, codes.InvalidArgument, 0},
		{"payment type out of the enum", func(r *pb.NewOrderPhoneRequest) { r.PaymentType = 2 }, codes.InvalidArgument, 0},
		{"operator unknown", func(r *pb.NewOrderPhoneRequest) { r.Operator = pb.NewOrderPhoneRequest_unknown }, codes.InvalidArgument, 0},
		{"type out of the enum", func(r *pb.NewOrderPhoneRequest) { r.Type = 5 }, codes.InvalidArgument, 0},
		{"mci", func(r *pb.NewOrderPhoneRequest) { r.Operator = pb.NewOrderPhoneRequest_mci }, codes.Unimplemented, 0},
		{"rightel", func(r *pb.NewOrderPhoneRequest) { r.Operator = pb.NewOrderPhoneRequest_rightel }, codes.Unimplemented, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orders := &fakeOrders{o: savedOrder()}
			log := &errLog{}
			req := orderReq()
			tc.change(req)
			_, err := newOrderPhone(orders, &fakeSigner{}, log).NewOrderV2(waterCtx(), req)
			assert.Equal(t, tc.code, status.Code(err))
			assert.Len(t, orders.reqs, tc.calls)
			assert.Empty(t, log.lines)
		})
	}
	// A nil request is an empty phone.
	_, err := newOrderPhone(&fakeOrders{}, &fakeSigner{}, nil).NewOrderV2(waterCtx(), nil)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// TestNewOrderV2NegativePrice: as bill, the price is converted with uint64 (a negative price is a
// very large one).
func TestNewOrderV2NegativePrice(t *testing.T) {
	orders := &fakeOrders{o: savedOrder()}
	req := orderReq()
	req.Price = -1
	_, err := newOrderPhone(orders, &fakeSigner{}, nil).NewOrderV2(waterCtx(), req)
	assert.NoError(t, err)
	if assert.Len(t, orders.reqs, 1) {
		assert.Equal(t, uint64(18446744073709551615), orders.reqs[0].Price)
	}
}

func TestNewOrderV2NotAvailableAndNoUser(t *testing.T) {
	_, err := NewPhone(PhoneDeps{Orders: &fakeOrders{}}).NewOrderV2(waterCtx(), orderReq())
	assert.Equal(t, codes.Unimplemented, status.Code(err), "no signer")
	_, err = NewPhone(PhoneDeps{Signer: &fakeSigner{}}).NewOrderV2(waterCtx(), orderReq())
	assert.Equal(t, codes.Unimplemented, status.Code(err), "no order flow")

	orders := &fakeOrders{}
	svc := newOrderPhone(orders, &fakeSigner{}, nil)
	_, err = svc.NewOrderV2(context.Background(), orderReq())
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "no claims")
	_, err = svc.NewOrderV2(auth.WithClaims(context.Background(), auth.Claims{}), orderReq())
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "no user")
	assert.Empty(t, orders.reqs)

	ctx, cancel := context.WithCancel(waterCtx())
	cancel()
	_, err = svc.NewOrderV2(ctx, orderReq())
	assert.Equal(t, codes.Canceled, status.Code(err))
	//nolint:staticcheck // a nil context is the case under test
	_, err = svc.NewOrderV2(nil, orderReq())
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// TestNewOrderV2Failures: each failure of the order flow and of the token gives its answer and,
// for a server fault, one error line with the cause (never sent to the client).
func TestNewOrderV2Failures(t *testing.T) {
	cause := errors.New("connection refused to 10.0.0.1:5432")
	for _, tc := range []struct {
		name     string
		err      error
		signErr  error
		code     codes.Code
		text     string
		wantLine string
	}{
		{"total too large", fmt.Errorf("%w: 5000000000", order.ErrTotalTooLarge), nil, codes.InvalidArgument, "invalid parameter", ""},
		{"zero price", order.ErrZeroPrice, nil, codes.FailedPrecondition, zeroPriceText, ""},
		{"no provider", fmt.Errorf("pick: %w", provider.ErrNoProvider), nil, codes.Unavailable, providerDownText, "irancell: no enabled provider for order"},
		{"no inquiry of the type", order.ErrNoInquiry, nil, codes.Internal, "internal error", "irancell: order inquiry has no inquiry of the bill type"},
		{"insert", fmt.Errorf("order: insert: %w", cause), nil, codes.Internal, "internal error", "irancell: order failed"},
		{"inquiry provider down", &order.InquiryError{Err: fmt.Errorf("x: %w", provider.ErrProviderDown)}, nil, codes.Unavailable, providerDownText, "irancell: provider inquiry failed"},
		{"inquiry no provider", &order.InquiryError{Err: fmt.Errorf("x: %w", provider.ErrNoProvider)}, nil, codes.Unavailable, providerDownText, "irancell: no enabled provider for inquiry"},
		{"inquiry refused", &order.InquiryError{Err: &inquiry.RefusedError{Message: "no bill"}}, nil, codes.FailedPrecondition, "no bill", ""},
		{"inquiry database", &order.InquiryError{Err: cause}, nil, codes.Internal, "internal error", "irancell: inquiry failed"},
		{"sign", nil, cause, codes.Internal, "internal error", "irancell: order token failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := &errLog{}
			orders := &fakeOrders{o: savedOrder(), err: tc.err}
			res, err := newOrderPhone(orders, &fakeSigner{err: tc.signErr}, log).NewOrderV2(waterCtx(), orderReq())
			assert.Nil(t, res)
			assert.Equal(t, tc.code, status.Code(err))
			assert.Equal(t, tc.text, status.Convert(err).Message())
			assert.False(t, strings.Contains(err.Error(), "10.0.0.1"), "the cause is not sent to the client")
			if tc.wantLine == "" {
				assert.Empty(t, log.lines, "a client error writes no line")
				return
			}
			if assert.Len(t, log.lines, 1) && assert.Len(t, log.kvs, 1) && assert.GreaterOrEqual(t, len(log.kvs[0]), 2) {
				assert.Equal(t, tc.wantLine, log.lines[0])
				kv := log.kvs[0][len(log.kvs[0])-2:] // the last pair is the error
				assert.Equal(t, "error", kv[0])
				want := tc.err
				var inqErr *order.InquiryError
				if errors.As(tc.err, &inqErr) && inqErr != nil {
					want = inqErr.Err // the inquiry answer logs the error of the inquiry
				}
				if tc.signErr != nil {
					want = tc.signErr
				}
				assert.Equal(t, want, kv[1], "the log has the cause")
			}
		})
	}
	// No logger: no panic.
	_, err := newOrderPhone(&fakeOrders{err: cause}, &fakeSigner{}, nil).NewOrderV2(waterCtx(), orderReq())
	assert.Equal(t, codes.Internal, status.Code(err))
	// The client left during the order: not a server error.
	ctx, cancel := context.WithCancel(waterCtx())
	orders := &fakeOrders{err: cause}
	log := &errLog{}
	svc := newOrderPhone(orders, &fakeSigner{}, log)
	cancel()
	assert.Equal(t, codes.Canceled, status.Code(svc.answerOrderError(ctx, "irancell", cause)))
	assert.Empty(t, log.lines)
}

func TestValidPhoneOrder(t *testing.T) {
	assert.True(t, validPhoneOrder(orderReq(), "989350000391"))
	assert.False(t, validPhoneOrder(orderReq(), ""))
	assert.True(t, validPhoneOrder(&pb.NewOrderPhoneRequest{Operator: pb.NewOrderPhoneRequest_unknown}, "989350000391"),
		"as bill, the value unknown passes the check")
}
