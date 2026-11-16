package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/config"
	"qabz/core/order"
	"qabz/provider"
	"qabz/provider/qabzino"
)

// A valid Rightel bill ID (type 5, the prefix 920) and pay ID (230 000 rials; the check digit of
// bill), so the Rightel order takes the raw inquiry (task 055).
const (
	liveRightelBill       = "1234567892055"
	liveRightelPay        = "23000870"
	liveRightelOrderPhone = "989201234567"
	liveRightelRow        = 500 // seed row 500 (parsian, mobilemidtermRightel, order): enable since task 055
)

func rightelOrderReq(term pb.NewOrderPhoneRequest_PaymentTerms, billID, payID string) *pb.NewOrderPhoneRequest {
	return &pb.NewOrderPhoneRequest{Phone: "09201234567", Operator: pb.NewOrderPhoneRequest_rightel, Type: term,
		PaymentType: pb.NewOrderPhoneRequest_direct, BillId: billID, PayId: payID}
}

// TestLiveRightelOrderAndPayment is the feature-ON integration test of task 055: a Rightel order
// (the raw inquiry: 230 000 rials, the default fee 9 000), the token data of bill (the Rightel texts,
// the operator rightel), the span order.Service.Create with the bill type, the payment (the Parsian
// mock: success), the rows initial, in-progress and success, no settlement. A new order of the same
// bill is "already paid" in the window 360h and accepted after it. A fullTerm request is HTTP 400.
func TestLiveRightelOrderAndPayment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterOrder(t, ctx, config.MockReturnSuccess)
	sr := jaegertest.Install(t)
	bt := provider.BillTypeMobileMidTermRightel

	_, err := s.order(liveUserOne, rightelOrderReq(pb.NewOrderPhoneRequest_fullTerm, liveRightelBill, liveRightelPay))
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "bill has no fullTerm entry for rightel")

	res, err := s.order(liveUserOne, rightelOrderReq(pb.NewOrderPhoneRequest_midTerm, liveRightelBill, liveRightelPay))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(230_000+9000), res.GetPrice())
	orderID := s.orderIDOf(t, res.GetOrderId())
	_, payload := s.tokenParts(t, res.GetOrderId())
	data, _ := payload["data"].(map[string]any)
	assert.Equal(t, map[string]any{
		"type": bt, "phone": liveRightelOrderPhone, "service": "rightel", "billId": liveRightelBill, "payId": liveRightelPay,
		"payAmount": "239000", "rawAmount": "230000", "about": "قبض موبایل",
		"aboutIcon":     "https://public.780.ir/dl/Operator/logOrhightel2.svg",
		"receiptMobile": "09201234567", "invoiceMobile": "09201234567",
		"feeAmount": "9000", "feeDescription": "کارمزد",
	}, data)
	assert.Equal(t, "parsian", payload["provider"])
	assert.Zero(t, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "the raw inquiry: no request row")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND status = 'initial' AND bill_type = $2
		AND bill_id = $3 AND pay_id = $4 AND data = $5 AND option = 'rightel' AND provider_id = $6 AND provider_name = 'parsian'
		AND base_price = 230000 AND fee = 9000 AND bill_inquiry_id IS NULL`, orderID, bt, liveRightelBill, liveRightelPay, liveRightelOrderPhone, liveRightelRow))
	if creates := jaegertest.ByName(sr, "order.Service.Create"); assert.Len(t, creates, 1) {
		assert.Contains(t, creates[0].Attributes(), attribute.String("bill_type", bt))
		assert.Contains(t, creates[0].Attributes(), attribute.String("result", order.ResultCreated))
	}

	o, err := s.complete(t, orderID, res.GetPrice())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, order.StatusSuccess, o.Status)
	if completes := jaegertest.ByName(sr, "order.Payments.Complete"); assert.Len(t, completes, 1) {
		assert.Contains(t, completes[0].Attributes(), attribute.String("bill_type", bt))
	}
	st, confirmed := s.statuses(t, orderID)
	assert.Equal(t, []string{"initial", "in-progress", "success"}, st)
	assert.False(t, confirmed, "no settlement for Rightel")
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_total", map[string]string{"bill_type": bt, "provider": "parsian", "status": "success"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": bt, "provider": "parsian", "result": "created"}))

	// The bill is paid: a new order with its bill ID and pay ID is refused in the window (360h) and
	// accepted when the paid order is older than the window.
	_, err = s.order(liveUserOne, rightelOrderReq(pb.NewOrderPhoneRequest_midTerm, liveRightelBill, liveRightelPay))
	assert.Equal(t, codes.FailedPrecondition, status.Code(err), "already paid in the window")
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": bt, "provider": "parsian", "result": "already_paid"}))
	if _, err := s.db.Pool().Exec(ctx, `UPDATE orders SET created_at = created_at - INTERVAL '361 hours' WHERE order_id = $1`, orderID); !assert.NoError(t, err) {
		return
	}
	_, err = s.order(liveUserOne, rightelOrderReq(pb.NewOrderPhoneRequest_midTerm, liveRightelBill, liveRightelPay))
	assert.NoError(t, err, "an order of 361 hours ago is out of the window 360h")
}

// TestLiveRightelOrderInquiry: an order with no pay ID has no raw inquiry, so the price comes from
// the inquiry of mobilemidtermRightel (the Qabzino mock, seed row 34) with the phone: the mid term.
// The bill_inquiries row is the bill_inquiry_id of the order; a Qabzino row has no provider fee.
// After the payment the inquiry row of the pay ID is paid.
func TestLiveRightelOrderInquiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterOrder(t, ctx, config.MockReturnSuccess)
	bt := provider.BillTypeMobileMidTermRightel
	res, err := s.order(liveUserOne, rightelOrderReq(pb.NewOrderPhoneRequest_midTerm, "", ""))
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(qabzino.MockRightelMidTermAmount+9000), res.GetPrice())
	orderID := s.orderIDOf(t, res.GetOrderId())
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE provider_name = 'qabzino' AND bill_type = $1 AND inquiry_id = $2`, bt, liveRightelOrderPhone))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders o JOIN bill_inquiries b ON b.id::text = o.bill_inquiry_id::text
		WHERE o.order_id = $1 AND o.bill_type = $2 AND o.pay_id = $3 AND o.base_price = $4 AND COALESCE(o.provider_fee, 0) = 0`,
		orderID, bt, qabzino.MockRightelMidTermPayID, qabzino.MockRightelMidTermAmount))
	o, err := s.complete(t, orderID, res.GetPrice())
	if assert.NoError(t, err) {
		assert.Equal(t, order.StatusSuccess, o.Status)
	}
	stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	assert.NoError(t, s.payments.Stop(stopCtx), "the background side effects are done")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND pay_id = $2 AND is_paid`, liveRightelOrderPhone, qabzino.MockRightelMidTermPayID))
}

// TestLiveRightelPaymentFailedAndPending: the Parsian mock at failed (the failed row; a second call
// answers failed, no new row) and at pending (the pending row).
func TestLiveRightelPaymentFailedAndPending(t *testing.T) {
	for pay, want := range map[string]string{config.MockReturnFailed: order.StatusFailed, config.MockReturnPending: order.StatusPending} {
		t.Run(pay, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			s := newLiveWaterOrder(t, ctx, pay)
			res, err := s.order(liveUserOne, rightelOrderReq(pb.NewOrderPhoneRequest_midTerm, liveRightelBill, liveRightelPay))
			if !assert.NoError(t, err) {
				return
			}
			orderID := s.orderIDOf(t, res.GetOrderId())
			o, err := s.complete(t, orderID, res.GetPrice())
			assert.NoError(t, err)
			assert.Equal(t, want, o.Status)
			st, _ := s.statuses(t, orderID)
			assert.Equal(t, []string{"initial", "in-progress", want}, st)
			o, err = s.payments.Complete(ctx, order.CompleteRequest{OrderID: orderID, RRN: "778899", Price: res.GetPrice()})
			assert.NoError(t, err)
			if want == order.StatusFailed {
				assert.Equal(t, order.StatusFailed, o.Status, "a failed order answers failed")
			} else {
				assert.Equal(t, order.Order{Status: order.StatusPending, Message: order.MessageProcessed}, o)
			}
			st, _ = s.statuses(t, orderID)
			assert.Len(t, st, 3, "no new row")
		})
	}
}

// TestLiveRightelStatusWorker (task 055 D10): the Parsian status worker finalizes a pending Rightel
// order of Parsian (row 500) with no code change (liveStatusWorker).
func TestLiveRightelStatusWorker(t *testing.T) {
	liveStatusWorker(t, provider.BillTypeMobileMidTermRightel, liveRightelRow, liveRightelBill, liveRightelPay, "56")
}
