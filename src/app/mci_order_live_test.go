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
	"qabz/provider/parsianproxy"
)

// A valid MCI bill ID (type 5, the prefix 091) and pay ID (450 000 rials; the check digit of bill),
// so the MCI order takes the raw inquiry (task 054).
const (
	liveMCIBill       = "1234567809158"
	liveMCIPay        = "45000080"
	liveMCIOrderPhone = "989121234567"
)

func mciOrderReq(term pb.NewOrderPhoneRequest_PaymentTerms, billID, payID string) *pb.NewOrderPhoneRequest {
	return &pb.NewOrderPhoneRequest{Phone: "09121234567", Operator: pb.NewOrderPhoneRequest_mci, Type: term,
		PaymentType: pb.NewOrderPhoneRequest_direct, BillId: billID, PayId: payID}
}

// liveMCITerms are the two terms of an MCI order: the request term, the bill type, the provider row.
var liveMCITerms = []struct {
	term     pb.NewOrderPhoneRequest_PaymentTerms
	billType string
	row      int
}{
	{pb.NewOrderPhoneRequest_fullTerm, provider.BillTypeMobile, 48},        // seed row 48: enable since task 054
	{pb.NewOrderPhoneRequest_midTerm, provider.BillTypeMobileMidterm, 149}, // seed row 149
}

// TestLiveMCIOrderAndPayment is the feature-ON integration test of task 054, for both terms: an MCI
// order (the raw inquiry: 450 000 rials, the default fee 9 000), the token data of bill (the MCI
// texts, the operator mci), the span order.Service.Create with the bill type, the payment (the
// Parsian mock: success), the rows initial, in-progress and success, no settlement. A new order of
// the same bill is "already paid" (settlement_inquiry_mci_mobile 0: the orders of any time).
func TestLiveMCIOrderAndPayment(t *testing.T) {
	for _, tc := range liveMCITerms {
		t.Run(tc.billType, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			s := newLiveWaterOrder(t, ctx, config.MockReturnSuccess)
			sr := jaegertest.Install(t)
			res, err := s.order(liveUserOne, mciOrderReq(tc.term, liveMCIBill, liveMCIPay))
			if !assert.NoError(t, err) || !assert.NotNil(t, res) {
				return
			}
			assert.Equal(t, uint32(450_000+9000), res.GetPrice())
			orderID := s.orderIDOf(t, res.GetOrderId())
			_, payload := s.tokenParts(t, res.GetOrderId())
			data, _ := payload["data"].(map[string]any)
			assert.Equal(t, map[string]any{
				"type": tc.billType, "phone": liveMCIOrderPhone, "service": "mci", "billId": liveMCIBill, "payId": liveMCIPay,
				"payAmount": "459000", "rawAmount": "450000", "about": "قبض موبایل",
				"aboutIcon":     "https://public.780.ir/dl/Operator/logOhamrahaval2.svg",
				"receiptMobile": "09121234567", "invoiceMobile": "09121234567",
				"feeAmount": "9000", "feeDescription": "کارمزد",
			}, data)
			assert.Equal(t, "parsian", payload["provider"])
			assert.Zero(t, s.count(t, `SELECT count(*) FROM bill_inquiry_requests`), "the raw inquiry: no request row")
			assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND status = 'initial' AND bill_type = $2
				AND bill_id = $3 AND pay_id = $4 AND data = $5 AND option = 'mci' AND provider_id = $6 AND provider_name = 'parsian'
				AND base_price = 450000 AND fee = 9000 AND bill_inquiry_id IS NULL`, orderID, tc.billType, liveMCIBill, liveMCIPay, liveMCIOrderPhone, tc.row))
			if creates := jaegertest.ByName(sr, "order.Service.Create"); assert.Len(t, creates, 1) {
				assert.Contains(t, creates[0].Attributes(), attribute.String("bill_type", tc.billType))
				assert.Contains(t, creates[0].Attributes(), attribute.String("result", order.ResultCreated))
			}

			o, err := s.complete(t, orderID, res.GetPrice())
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, order.StatusSuccess, o.Status)
			if completes := jaegertest.ByName(sr, "order.Payments.Complete"); assert.Len(t, completes, 1) {
				assert.Contains(t, completes[0].Attributes(), attribute.String("bill_type", tc.billType))
			}
			st, confirmed := s.statuses(t, orderID)
			assert.Equal(t, []string{"initial", "in-progress", "success"}, st)
			assert.False(t, confirmed, "no settlement for MCI")
			assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_total", map[string]string{"bill_type": tc.billType, "provider": "parsian", "status": "success"}))
			assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": tc.billType, "provider": "parsian", "result": "created"}))

			// The bill is paid: a new order with its bill ID and pay ID is refused (the window 0 of bill
			// looks at the orders of any time).
			if _, err := s.db.Pool().Exec(ctx, `UPDATE orders SET created_at = created_at - INTERVAL '400 days' WHERE order_id = $1`, orderID); !assert.NoError(t, err) {
				return
			}
			_, err = s.order(liveUserOne, mciOrderReq(tc.term, liveMCIBill, liveMCIPay))
			assert.Equal(t, codes.FailedPrecondition, status.Code(err), "an order of 400 days ago is in the window 0")
			assert.Equal(t, 1.0, s.metric(t, "qabz_order_create_total", map[string]string{"bill_type": tc.billType, "provider": "parsian", "result": "already_paid"}))
		})
	}
}

// TestLiveMCIOrderInquiry: an order with no pay ID has no raw inquiry, so the price comes from the
// inquiry of mobile (the Parsian inquiry mock, seed row 39) with the phone: a full-term order takes
// the final term, a mid-term order the mid term (one answer has both terms). The bill_inquiries row
// is the bill_inquiry_id of the order; a Parsian row has no provider fee.
func TestLiveMCIOrderInquiry(t *testing.T) {
	for _, tc := range liveMCITerms {
		t.Run(tc.billType, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			s := newLiveWaterOrder(t, ctx, config.MockReturnSuccess)
			res, err := s.order(liveUserOne, mciOrderReq(tc.term, "", ""))
			if !assert.NoError(t, err) || !assert.NotNil(t, res) {
				return
			}
			pay, amount := parsianproxy.MockFinalTermPayID, int64(parsianproxy.MockFinalTermAmount)
			if tc.billType == provider.BillTypeMobileMidterm {
				pay, amount = parsianproxy.MockMidtermPayID, int64(parsianproxy.MockMidtermAmount)
			}
			assert.Equal(t, uint32(amount+9000), res.GetPrice())
			orderID := s.orderIDOf(t, res.GetOrderId())
			assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiry_requests WHERE provider_name = 'parsian' AND bill_type = 'mobile' AND inquiry_id = $1`, liveMCIOrderPhone))
			assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders o JOIN bill_inquiries b ON b.id::text = o.bill_inquiry_id::text
				WHERE o.order_id = $1 AND o.bill_type = $2 AND o.pay_id = $3 AND o.base_price = $4 AND COALESCE(o.provider_fee, 0) = 0`,
				orderID, tc.billType, pay, amount))
			o, err := s.complete(t, orderID, res.GetPrice())
			if assert.NoError(t, err) {
				assert.Equal(t, order.StatusSuccess, o.Status)
			}
			stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			assert.NoError(t, s.payments.Stop(stopCtx), "the background side effects are done")
			assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = $1 AND pay_id = $2 AND is_paid`, liveMCIOrderPhone, pay))
		})
	}
}

// TestLiveMCIPaymentFailedAndPending: the Parsian mock at failed (the failed row; a second call
// answers failed, no new row) and at pending (the pending row), for both terms.
func TestLiveMCIPaymentFailedAndPending(t *testing.T) {
	for _, tc := range liveMCITerms {
		for pay, want := range map[string]string{config.MockReturnFailed: order.StatusFailed, config.MockReturnPending: order.StatusPending} {
			t.Run(tc.billType+"/"+pay, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
				s := newLiveWaterOrder(t, ctx, pay)
				res, err := s.order(liveUserOne, mciOrderReq(tc.term, liveMCIBill, liveMCIPay))
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
}

// TestLiveMCIStatusWorker (task 054 D10): the Parsian status worker finalizes pending MCI orders of
// Parsian (rows 48 and 149) with no code change (liveStatusWorker).
func TestLiveMCIStatusWorker(t *testing.T) {
	liveStatusWorker(t, provider.BillTypeMobile, 48, liveMCIBill, liveMCIPay, "54")
	liveStatusWorker(t, provider.BillTypeMobileMidterm, 149, liveMCIBill, liveMCIPay, "55")
}
