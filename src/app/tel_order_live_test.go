package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/config"
	"qabz/core/order"
	"qabz/provider/karashop"
)

// enableTelOrderSQL enables the Parsian order row of tel (row 50 is disabled in the seed; row 51,
// telmidterm, is enabled), so both terms have the order provider Parsian (task 044).
const enableTelOrderSQL = `UPDATE providers SET status = 'enable' WHERE id = 50;`

// newLiveTel is the live stack with the Parsian and Karashop mocks on (task 044): the payment
// answer pay and the settlement answer settle.
func newLiveTel(t *testing.T, ctx context.Context, pay, settle string) *liveWaterStack {
	t.Helper()
	return newLiveWaterCfg(t, ctx, config.MockReturnSuccess, enableTelOrderSQL, func(c *config.Config) {
		c.ParsianEnabled = true
		c.DebugMockParsianEnable = true
		c.DebugMockParsianReturnType = config.MockReturnSuccess
		c.DebugMockParsianPayReturnType = pay
		c.DebugMockKarashopSettleReturnType = settle
	})
}

func (s *liveWaterStack) telOrder(t *testing.T, typ pb.NewOrderTelRequest_PaymentTerms, billID, payID string) (*pb.NewOrderResponse, error) {
	t.Helper()
	ctx := auth.WithClaims(context.Background(), auth.Claims{UserID: liveUserOne, Mobile: "989121234567"})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return s.tel.NewOrderV2(ctx, &pb.NewOrderTelRequest{Phone: "02188776655", Type: typ, PaymentType: pb.NewOrderTelRequest_direct, BillId: billID, PayId: payID})
}

// orderIDOf reads the order ID of the token.
func (s *liveWaterStack) orderIDOf(t *testing.T, token string) string {
	t.Helper()
	_, payload := s.tokenParts(t, token)
	id, _ := payload["orderId"].(string)
	if !assert.NotEmpty(t, id) {
		t.FailNow()
	}
	return id
}

// statuses gives the statuses of the order rows, oldest first, and provider_confirmed of the newest.
func (s *liveWaterStack) statuses(t *testing.T, orderID string) ([]string, bool) {
	t.Helper()
	rows, err := s.db.Pool().Query(context.Background(), `SELECT status, COALESCE(provider_confirmed, false) FROM orders WHERE order_id = $1 ORDER BY created_at`, orderID)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	defer rows.Close()
	var out []string
	var confirmed bool
	for rows.Next() {
		var st string
		assert.NoError(t, rows.Scan(&st, &confirmed))
		out = append(out, st)
	}
	return out, confirmed
}

func (s *liveWaterStack) complete(t *testing.T, orderID string, price uint32) (order.Order, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	o, err := s.payments.Complete(ctx, order.CompleteRequest{OrderID: orderID, RRN: "778899", Price: price})
	// The settlement runs in the background: Stop waits for it (no new job after it).
	stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	assert.NoError(t, s.payments.Stop(stopCtx))
	return o, err
}

// TestLiveTelOrderAndPayment is the feature-ON integration test of task 044 (verification steps 1
// and 5): a telmidterm order (the Karashop mock: 120 000 rials, the line active, so the provider fee
// 21 000; the default fee 9 000), the payment (the Parsian mock: success), the rows initial,
// in-progress and success, the settlement (the Karashop mock: success) sets provider_confirmed, and
// the inquiry rows are paid. A second call answers pending with no new row; a new order of the same
// bill is "already paid".
func TestLiveTelOrderAndPayment(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	s := newLiveTel(t, ctx, config.MockReturnSuccess, config.MockReturnSuccess)
	res, err := s.telOrder(t, pb.NewOrderTelRequest_midTerm, "", "")
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, uint32(120000+9000+21000), res.GetPrice())
	orderID := s.orderIDOf(t, res.GetOrderId())
	_, payload := s.tokenParts(t, res.GetOrderId())
	data, _ := payload["data"].(map[string]any)
	assert.Equal(t, "telmidterm", data["type"])
	assert.Equal(t, "21000", data["providerFee"])
	assert.Equal(t, "30000", data["feeAmount"])
	assert.Equal(t, "parsian", payload["provider"])
	st, _ := s.statuses(t, orderID)
	assert.Equal(t, []string{"initial"}, st)

	o, err := s.complete(t, orderID, res.GetPrice())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, order.StatusSuccess, o.Status)
	st, confirmed := s.statuses(t, orderID)
	assert.Equal(t, []string{"initial", "in-progress", "success"}, st)
	assert.True(t, confirmed, "the Karashop settlement set provider_confirmed")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND rrn = '778899' AND status = 'success' AND ref_id IS NOT NULL`, orderID))
	assert.Positive(t, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE inquiry_id = '982188776655' AND pay_id = $1 AND is_paid`, karashop.MockMidTermPayID))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_total", map[string]string{"bill_type": "telmidterm", "provider": "parsian", "status": "success"}))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_settle_total", map[string]string{"bill_type": "telmidterm", "provider": "karashop", "result": "success", "source": "payment"}))

	// Step 5: a second call for the same order: pending, no new row.
	o, err = s.payments.Complete(ctx, order.CompleteRequest{OrderID: orderID, RRN: "778899", Price: res.GetPrice()})
	assert.NoError(t, err)
	assert.Equal(t, order.Order{Status: order.StatusPending, Message: order.MessageProcessed}, o)
	st, _ = s.statuses(t, orderID)
	assert.Len(t, st, 3)

	// The bill is paid: a new order with its bill ID and pay ID is refused.
	_, err = s.telOrder(t, pb.NewOrderTelRequest_midTerm, karashop.MockBillID, karashop.MockMidTermPayID)
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))

	// The saved mid-term row is paid, so the saved answer has only the final term: a new mid-term
	// order (no IDs in the request: no already-paid check) asks Karashop again (found in the
	// feature-ON run).
	calls := s.log.count("info", "provider mock: inquiry answered with no call to the provider")
	res, err = s.telOrder(t, pb.NewOrderTelRequest_midTerm, "", "")
	if assert.NoError(t, err) {
		assert.Equal(t, uint32(120000+9000+21000), res.GetPrice())
	}
	assert.Equal(t, calls+1, s.log.count("info", "provider mock: inquiry answered with no call to the provider"), "one more provider call")
}

// TestLiveTelPaymentStatuses (verification steps 2, 3, 4 and 5): the Karashop settlement fails
// (provider_confirmed false, one error line, the metric); the Parsian mock at failed and at pending
// (no settlement); a wrong price (pending, no new row, no payment).
func TestLiveTelPaymentStatuses(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	s := newLiveTel(t, ctx, config.MockReturnSuccess, config.MockReturnError)
	res, err := s.telOrder(t, pb.NewOrderTelRequest_fullTerm, "", "")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, uint32(350000+9000+21000), res.GetPrice())
	orderID := s.orderIDOf(t, res.GetOrderId())
	o, err := s.complete(t, orderID, res.GetPrice())
	assert.NoError(t, err)
	assert.Equal(t, order.StatusSuccess, o.Status)
	st, confirmed := s.statuses(t, orderID)
	assert.Equal(t, []string{"initial", "in-progress", "success"}, st)
	assert.False(t, confirmed, "the settlement failed")
	assert.Equal(t, 1, s.log.count("error", "settle: karashop failed"))
	assert.Equal(t, 1.0, s.metric(t, "qabz_order_settle_total", map[string]string{"bill_type": "tel", "provider": "karashop", "result": "error", "source": "payment"}))

	for pay, want := range map[string]string{config.MockReturnFailed: order.StatusFailed, config.MockReturnPending: order.StatusPending} {
		s := newLiveTel(t, ctx, pay, config.MockReturnSuccess)
		res, err := s.telOrder(t, pb.NewOrderTelRequest_midTerm, "", "")
		if !assert.NoError(t, err, pay) {
			continue
		}
		orderID := s.orderIDOf(t, res.GetOrderId())
		o, err := s.complete(t, orderID, res.GetPrice())
		assert.NoError(t, err, pay)
		assert.Equal(t, want, o.Status, pay)
		st, confirmed := s.statuses(t, orderID)
		assert.Equal(t, []string{"initial", "in-progress", want}, st, pay)
		assert.False(t, confirmed, pay)
		assert.Zero(t, s.metric(t, "qabz_order_settle_total", map[string]string{"bill_type": "telmidterm", "provider": "karashop", "result": "success", "source": "payment"}), pay+": no settlement")
		assert.Zero(t, s.count(t, `SELECT count(*) FROM bill_inquiries WHERE is_paid`), pay+": not paid")
	}

	s = newLiveTel(t, ctx, config.MockReturnSuccess, config.MockReturnSuccess)
	res, err = s.telOrder(t, pb.NewOrderTelRequest_midTerm, "", "")
	if !assert.NoError(t, err) {
		return
	}
	orderID = s.orderIDOf(t, res.GetOrderId())
	o, err = s.complete(t, orderID, res.GetPrice()+1)
	assert.NoError(t, err)
	assert.Equal(t, order.Order{Status: order.StatusPending, Message: order.MessageWrongPrice}, o)
	st, _ = s.statuses(t, orderID)
	assert.Equal(t, []string{"initial"}, st)
}
