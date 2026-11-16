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
	"qabz/common/auth"
	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/config"
	"qabz/core/order"
	"qabz/provider"
	"qabz/provider/tavanir"
)

// A valid electricity bill ID (type 2) and pay ID (18 000 000 rials; the check digits of bill), so
// the electricity order takes the raw inquiry (task 052).
const (
	liveElecBill = "1234567890125"
	liveElecPay  = "1800000030"
)

func (s *liveWaterStack) electricityOrder(t *testing.T, billID, payID string) (*pb.NewOrderResponse, error) {
	t.Helper()
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: liveUserOne, Mobile: "989121234567"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.electricity.NewOrder(ctx, &pb.NewOrderBillPayRequest{BillId: billID, PayId: payID, PaymentType: pb.NewOrderBillPayRequest_direct})
}

// TestLiveElectricityOrderAndPayment is the feature-ON integration test of task 052: an electricity
// order (the raw inquiry: 18 000 000 rials, the default fee 9 000, no provider call), the token data
// of bill (the about text with its first space, D7 (a)), the span order.Service.Create with the bill
// type, the payment (the Parsian mock: success), the rows initial, in-progress and success, no
// settlement. A second call answers pending with no new row; a new order of the same bill is
// "already paid" (settlement_inquiry_electricity).
func TestLiveElectricityOrderAndPayment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterOrder(t, ctx, config.MockReturnSuccess)
	sr := jaegertest.Install(t)
	res, err := s.electricityOrder(t, liveElecBill, liveElecPay)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(18_000_000+9000), res.GetPrice())
	assert.Equal(t, uint32(105), res.GetServiceId())
	orderID := s.orderIDOf(t, res.GetOrderId())
	_, payload := s.tokenParts(t, res.GetOrderId())
	data, _ := payload["data"].(map[string]any)
	assert.Equal(t, map[string]any{
		"payId": liveElecPay, "type": "electricity", "billId": liveElecBill, "payAmount": "18009000", "rawAmount": "18000000",
		"about": " قبض برق", "aboutIcon": "https://public.780.ir/dl/Operator/Bargh.svg",
		"invoicePayID": liveElecPay, "invoiceBillID": liveElecBill, "receiptPayID": liveElecPay, "receiptBillID": liveElecBill,
		"feeAmount": "9000", "feeDescription": "کارمزد",
	}, data)
	assert.Equal(t, "parsian", payload["provider"])
	assert.Zero(t, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "the raw inquiry: no request row")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND status = 'initial' AND bill_type = 'electricity'
		AND bill_id = $2 AND pay_id = $3 AND data = $2 AND provider_id = 47 AND provider_name = 'parsian' AND base_price = 18000000
		AND fee = 9000 AND bill_inquiry_id IS NULL`, orderID, liveElecBill, liveElecPay))
	if creates := jaegertest.ByName(sr, "order.Service.Create"); assert.Len(t, creates, 1) {
		assert.Contains(t, creates[0].Attributes(), attribute.String("bill_type", "electricity"))
		assert.Contains(t, creates[0].Attributes(), attribute.String("result", order.ResultCreated))
	}

	o, err := s.complete(t, orderID, res.GetPrice())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, order.StatusSuccess, o.Status)
	if completes := jaegertest.ByName(sr, "order.Payments.Complete"); assert.Len(t, completes, 1) {
		assert.Contains(t, completes[0].Attributes(), attribute.String("bill_type", "electricity"))
	}
	st, confirmed := s.statuses(t, orderID)
	assert.Equal(t, []string{"initial", "in-progress", "success"}, st)
	assert.False(t, confirmed, "no settlement for electricity")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND rrn = '778899' AND status = 'success' AND ref_id IS NOT NULL`, orderID))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_total", map[string]string{"bill_type": "electricity", "provider": "parsian", "status": "success"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": "electricity", "provider": "parsian", "result": "created"}))
	assert.Zero(t, s.log.count("info", "provider mock: settlement answered with no call to the provider"), "no settlement")

	// A second call for the same order: pending, no new row.
	o, err = s.payments.Complete(ctx, order.CompleteRequest{OrderID: orderID, RRN: "778899", Price: res.GetPrice()})
	assert.NoError(t, err)
	assert.Equal(t, order.Order{Status: order.StatusPending, Message: order.MessageProcessed}, o)
	st, _ = s.statuses(t, orderID)
	assert.Len(t, st, 3)

	// The bill is paid: a new order with its bill ID and pay ID is refused.
	_, err = s.electricityOrder(t, liveElecBill, liveElecPay)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": "electricity", "provider": "parsian", "result": "already_paid"}))
}

// TestLiveElectricityOrderTavanir: an order with no pay ID has no raw inquiry, so the price comes
// from the regular inquiry of the electricity inquiry provider of the seed (Tavanir, row 2; the
// mock): one request row, the bill_inquiries row is the bill_inquiry_id of the order, and no
// provider fee. After a success the row is is_paid.
func TestLiveElectricityOrderTavanir(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveWaterOrder(t, ctx, config.MockReturnSuccess)
	res, err := s.electricityOrder(t, liveElecBill, "")
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(tavanir.MockAmount+9000), res.GetPrice())
	orderID := s.orderIDOf(t, res.GetOrderId())
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE provider_name = 'tavanir' AND bill_type = 'electricity'`))
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders o JOIN bill_inquiries b ON b.id::text = o.bill_inquiry_id::text
		WHERE o.order_id = $1 AND o.pay_id = $2 AND o.base_price = $3 AND COALESCE(o.provider_fee, 0) = 0 AND b.provider_name = 'tavanir'`,
		orderID, tavanir.MockPayID, tavanir.MockAmount))
	assert.Equal(t, 1.0, s.metric(t, "qabz_inquiries_total", map[string]string{"bill_type": "electricity", "provider": "tavanir", "result": "provider_ok"}))

	o, err := s.complete(t, orderID, res.GetPrice())
	if assert.NoError(t, err) {
		assert.Equal(t, order.StatusSuccess, o.Status)
	}
	stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	assert.NoError(t, s.payments.Stop(stopCtx), "the background side effects are done")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND pay_id = $2 AND is_paid`, liveElecBill, tavanir.MockPayID))
}

// TestLiveElectricityPaymentFailedAndPending: the Parsian mock at failed (the failed row; a second
// call answers failed with the message of that row, no new row: task 048 D11 (a)) and at pending
// (the pending row; a second call answers pending).
func TestLiveElectricityPaymentFailedAndPending(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	for pay, want := range map[string]string{config.MockReturnFailed: order.StatusFailed, config.MockReturnPending: order.StatusPending} {
		s := newLiveWaterOrder(t, ctx, pay)
		res, err := s.electricityOrder(t, liveElecBill, liveElecPay)
		if !assert.NoError(t, err, pay) {
			continue
		}
		orderID := s.orderIDOf(t, res.GetOrderId())
		o, err := s.complete(t, orderID, res.GetPrice())
		assert.NoError(t, err, pay)
		assert.Equal(t, want, o.Status, pay)
		st, _ := s.statuses(t, orderID)
		assert.Equal(t, []string{"initial", "in-progress", want}, st, pay)

		o, err = s.payments.Complete(ctx, order.CompleteRequest{OrderID: orderID, RRN: "778899", Price: res.GetPrice()})
		assert.NoError(t, err, pay)
		if want == order.StatusFailed {
			assert.Equal(t, order.StatusFailed, o.Status, "a failed order answers failed")
			assert.Equal(t, provider.MockPayFailedMessage, o.Message)
		} else {
			assert.Equal(t, order.Order{Status: order.StatusPending, Message: order.MessageProcessed}, o, pay)
		}
		st, _ = s.statuses(t, orderID)
		assert.Len(t, st, 3, pay+": no new row")
		assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_total", map[string]string{"bill_type": "electricity", "provider": "parsian", "status": "processed"}), pay)
	}
}

// TestLiveElectricityStatusWorker (task 052 D11): the Parsian status worker finalizes two pending
// electricity orders of Parsian (row 47) with no code change (liveStatusWorker).
func TestLiveElectricityStatusWorker(t *testing.T) {
	liveStatusWorker(t, provider.BillTypeElectricity, 47, liveElecBill, liveElecPay, "52")
}
