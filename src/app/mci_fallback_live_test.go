package app

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"

	pb "qabz/api/pb/qabz"
	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/config"
	"qabz/core/order"
	"qabz/provider"
	"qabz/provider/jiring"
)

// newLiveFallback is the order stack of task 058: the Parsian switch off (every Parsian payment is
// "not sent"), Jiring Switch on with its mock (pay_return_type jiringPay), or off with jiringOn
// false.
func newLiveFallback(t *testing.T, ctx context.Context, jiringOn bool, jiringPay string) *liveWaterStack {
	t.Helper()
	return newLiveWaterCfg(t, ctx, config.MockReturnSuccess, "", func(c *config.Config) {
		c.ParsianEnabled = false
		c.JiringSwitchEnabled = jiringOn
		c.DebugMockJiringSwitchEnabled = true
		c.DebugMockJiringSwitchReturnType = config.MockReturnSuccess
		c.DebugMockJiringSwitchPayReturnType = jiringPay
	})
}

// liveRowsOf gives provider_id/provider_name/status of each row of the order, the oldest first.
func liveRowsOf(t *testing.T, s *liveWaterStack, orderID string) []string {
	t.Helper()
	rows, err := s.db.Pool().Query(context.Background(),
		`SELECT provider_id || '/' || provider_name || '/' || status FROM orders WHERE order_id = $1 ORDER BY created_at, status DESC`, orderID)
	if !assert.NoError(t, err) {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if assert.NoError(t, rows.Scan(&r)) {
			out = append(out, r)
		}
	}
	return out
}

// liveMCIJiringRows are the Jiring Switch order rows of the seed (task 058: enable).
var liveMCIJiringRows = map[string]int{provider.BillTypeMobile: 30, provider.BillTypeMobileMidterm: 29}

// TestLiveMCIFallback is the feature-ON integration test of task 058, for both terms: the Parsian
// switch is off, so the payment did not reach Parsian for sure; it goes to the next enabled row of
// the seed (Jiring Switch 30 or 29, priority 10) with the same RRN, and the Jiring Switch mock pays.
// The metrics and the span show both providers.
func TestLiveMCIFallback(t *testing.T) {
	for _, tc := range liveMCITerms {
		t.Run(tc.billType, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			s := newLiveFallback(t, ctx, true, config.MockReturnSuccess)
			sr := jaegertest.Install(t)
			res, err := s.order(liveUserOne, mciOrderReq(tc.term, liveMCIBill, liveMCIPay))
			if !assert.NoError(t, err) || !assert.NotNil(t, res) {
				return
			}
			orderID := s.orderIDOf(t, res.GetOrderId())
			o, err := s.complete(t, orderID, res.GetPrice())
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, order.StatusSuccess, o.Status)
			assert.Equal(t, jiring.MockPayTrace, o.RefID)
			jr := liveMCIJiringRows[tc.billType]
			assert.Equal(t, []string{
				strconv.Itoa(tc.row) + "/parsian/initial", strconv.Itoa(tc.row) + "/parsian/in-progress",
				strconv.Itoa(jr) + "/jiring-switch/in-progress", strconv.Itoa(jr) + "/jiring-switch/success",
			}, liveRowsOf(t, s, orderID))
			assert.Equal(t, 3, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND rrn = '778899'`, orderID), "the same RRN")
			assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND provider_name = 'jiring-switch'
				AND status = 'in-progress' AND message LIKE '%providers.internal.parsian.enabled is false%'`, orderID), "the Parsian error on the fallback row")
			assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_fallback_total", map[string]string{"bill_type": tc.billType, "from": "parsian", "to": "jiring-switch"}))
			assert.Equal(t, 1.0, s.metric(t, "qabz_order_payment_total", map[string]string{"bill_type": tc.billType, "provider": "jiring-switch", "status": "success"}))
			assert.Equal(t, 1, s.log.count("info", "order complete: payment not sent, next provider"))
			if completes := jaegertest.ByName(sr, "order.Payments.Complete"); assert.Len(t, completes, 1) {
				assert.Contains(t, completes[0].Attributes(), attribute.String("provider", "jiring-switch"))
				assert.Contains(t, completes[0].Attributes(), attribute.String("provider.first", "parsian"))
			}
			jaegertest.AssertClean(t, sr.Ended(), liveMCIBill, liveMCIPay, liveMCIOrderPhone, liveUserOne)

			// A second call: processed, no new row, no payment.
			o, err = s.payments.Complete(ctx, order.CompleteRequest{OrderID: orderID, RRN: "778899", Price: res.GetPrice()})
			assert.NoError(t, err)
			assert.Equal(t, order.Order{Status: order.StatusPending, Message: order.MessageProcessed}, o)
			assert.Len(t, liveRowsOf(t, s, orderID), 4)
		})
	}
}

// TestLiveMCINoFallback (task 058, operator rules): a Parsian timeout (the Parsian mock pending)
// stays pending with no Jiring Switch call; a Jiring Switch that is not reached either leaves the
// order pending with the Jiring Switch row (no next provider is left).
func TestLiveMCINoFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	s := newLiveWaterOrder(t, ctx, config.MockReturnPending)
	res, err := s.order(liveUserOne, mciOrderReq(pb.NewOrderPhoneRequest_midTerm, liveMCIBill, liveMCIPay))
	if !assert.NoError(t, err) {
		return
	}
	orderID := s.orderIDOf(t, res.GetOrderId())
	o, err := s.complete(t, orderID, res.GetPrice())
	if assert.NoError(t, err) {
		assert.Equal(t, order.StatusPending, o.Status)
	}
	assert.Equal(t, []string{"149/parsian/initial", "149/parsian/in-progress", "149/parsian/pending"}, liveRowsOf(t, s, orderID), "a timeout: no fallback")
	assert.Zero(t, s.log.count("info", "order complete: payment not sent, next provider"))

	s = newLiveFallback(t, ctx, false, config.MockReturnSuccess)
	res, err = s.order(liveUserOne, mciOrderReq(pb.NewOrderPhoneRequest_fullTerm, liveMCIBill, liveMCIPay))
	if !assert.NoError(t, err) {
		return
	}
	orderID = s.orderIDOf(t, res.GetOrderId())
	o, err = s.complete(t, orderID, res.GetPrice())
	if assert.NoError(t, err) {
		assert.Equal(t, order.StatusPending, o.Status)
		assert.Equal(t, "jiring-switch", o.ProviderName)
	}
	assert.Equal(t, []string{"48/parsian/initial", "48/parsian/in-progress", "30/jiring-switch/in-progress", "30/jiring-switch/pending"},
		liveRowsOf(t, s, orderID), "both not reached: pending with the last provider")
	assert.Equal(t, 1, s.count(t, `SELECT count(*) FROM orders WHERE order_id = $1 AND status = 'pending' AND status_code = 500
		AND message LIKE '%providers.internal.jiring_switch.enabled is false%'`, orderID))
}

// TestLiveJiringSwitchStatusWorker (task 058): the Jiring Switch status worker finalizes pending
// Jiring Switch MCI orders (rows 30 and 29) on the shared worker, with the Kafka report.
func TestLiveJiringSwitchStatusWorker(t *testing.T) {
	on := func(cfg *config.Config) {
		cfg.JiringSwitchGetStatusEnabled = true
		cfg.JiringSwitchGetStatusCronTime = time.Hour // one run, at the start
	}
	liveProviderStatusWorker(t, jiring.Name, on, provider.BillTypeMobile, 30, liveMCIBill, liveMCIPay, "58")
	liveProviderStatusWorker(t, jiring.Name, on, provider.BillTypeMobileMidterm, 29, liveMCIBill, liveMCIPay, "59")
}
