package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/order"
	"qabz/system/postgres"
)

// The rows of the payment tests (task 044).
const (
	payOrderID = "bbbbbbbb-0000-4000-8000-000000000044"
	payBillID  = "2101234560040"
	payPayID   = "12000035"
	payTel     = "982188776655"
)

func payOrder(id, status string) order.Order {
	return order.Order{
		ID: id, OrderID: payOrderID, UserID: inqUser1, BillType: "telmidterm", BillID: payBillID, PayID: payPayID,
		Price: 150000, Data: payTel, ProviderID: 51, ProviderName: "parsian", Status: status, Fee: 9000, BasePrice: 120000,
		ProviderFee: 21000, BillInquiryID: "mid-row",
	}
}

// TestLiveOrderPaymentRows: each status is a new row; Latest gives the newest (with rrn, ref_id,
// message, status_code and created_at); an old row is out of the window; an unknown order is
// ErrNotFound; an order ID that is not a UUID is an error.
func TestLiveOrderPaymentRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)

	initial := payOrder("aaaaaaaa-0000-4000-8000-000000000441", order.StatusInitial)
	if !assert.NoError(t, r.Insert(ctx, initial)) {
		return
	}
	got, err := r.Latest(ctx, payOrderID, order.LatestWindow)
	if !assert.NoError(t, err) {
		return
	}
	assert.False(t, got.CreatedAt.IsZero())
	got.CreatedAt = time.Time{}
	assert.Equal(t, initial, got)

	progress := initial
	progress.ID, progress.Status, progress.RRN = "aaaaaaaa-0000-4000-8000-000000000442", order.StatusInProgress, "778899"
	done := progress
	done.ID, done.Status, done.RefID, done.Message, done.StatusCode = "aaaaaaaa-0000-4000-8000-000000000443", order.StatusSuccess, "777", "ok", 200
	for _, o := range []order.Order{progress, done} {
		if !assert.NoError(t, r.Insert(ctx, o)) {
			return
		}
	}
	got, err = r.Latest(ctx, payOrderID, order.LatestWindow)
	if assert.NoError(t, err) {
		got.CreatedAt = time.Time{}
		assert.Equal(t, done, got)
	}
	var n int
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM orders WHERE order_id = $1`, payOrderID).Scan(&n))
	assert.Equal(t, 3, n, "one row for each status")

	// Out of the window.
	_, err = c.Pool().Exec(ctx, `UPDATE orders SET created_at = NOW() - INTERVAL '49 hours' WHERE order_id = $1`, payOrderID)
	assert.NoError(t, err)
	_, err = r.Latest(ctx, payOrderID, order.LatestWindow)
	assert.ErrorIs(t, err, order.ErrNotFound)
	_, err = r.Latest(ctx, "bbbbbbbb-0000-4000-8000-000000000999", order.LatestWindow)
	assert.ErrorIs(t, err, order.ErrNotFound)
	_, err = r.Latest(ctx, "not-a-uuid", order.LatestWindow)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, order.ErrNotFound)
}

// TestLiveOrderConfirmProvider: only the success row of the order in the last 48 h gets
// provider_confirmed.
func TestLiveOrderConfirmProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	for i, status := range []string{order.StatusInitial, order.StatusInProgress, order.StatusSuccess} {
		o := payOrder("aaaaaaaa-0000-4000-8000-00000000045"+string(rune('0'+i)), status)
		if !assert.NoError(t, r.Insert(ctx, o)) {
			return
		}
	}
	assert.NoError(t, r.ConfirmProvider(ctx, payOrderID))
	rows, err := c.Pool().Query(ctx, `SELECT status, COALESCE(provider_confirmed::text, '<nil>') FROM orders WHERE order_id = $1 ORDER BY status`, payOrderID)
	if !assert.NoError(t, err) {
		return
	}
	got := map[string]string{}
	for rows.Next() {
		var s, v string
		assert.NoError(t, rows.Scan(&s, &v))
		got[s] = v
	}
	rows.Close()
	assert.Equal(t, map[string]string{"initial": "<nil>", "in-progress": "<nil>", "success": "true"}, got)
	assert.NoError(t, r.ConfirmProvider(ctx, "bbbbbbbb-0000-4000-8000-000000000998"), "no row is not an error")
}

// TestLiveOrderLastStatus: the newest status of the bill ID and the pay ID; the window (whole
// minutes); no row is "".
func TestLiveOrderLastStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	status, err := r.LastStatus(ctx, payBillID, payPayID, 360*time.Hour)
	assert.NoError(t, err)
	assert.Empty(t, status)

	if !assert.NoError(t, r.Insert(ctx, payOrder("aaaaaaaa-0000-4000-8000-000000000461", order.StatusInitial))) {
		return
	}
	if !assert.NoError(t, r.Insert(ctx, payOrder("aaaaaaaa-0000-4000-8000-000000000462", order.StatusPending))) {
		return
	}
	status, err = r.LastStatus(ctx, payBillID, payPayID, 360*time.Hour)
	assert.NoError(t, err)
	assert.Equal(t, order.StatusPending, status)
	_, err = c.Pool().Exec(ctx, `UPDATE orders SET created_at = NOW() - INTERVAL '2 hours'
		- CASE status WHEN 'initial' THEN INTERVAL '1 second' ELSE INTERVAL '0' END WHERE order_id = $1`, payOrderID)
	assert.NoError(t, err)
	status, err = r.LastStatus(ctx, payBillID, payPayID, time.Hour)
	assert.NoError(t, err)
	assert.Empty(t, status, "out of the window")
	status, err = r.LastStatus(ctx, payBillID, payPayID, 0)
	assert.NoError(t, err)
	assert.Equal(t, order.StatusPending, status, "0: no window")
	status, err = r.LastStatus(ctx, payBillID, payPayID, 30*time.Second)
	assert.NoError(t, err)
	assert.Equal(t, order.StatusPending, status, "under one minute: no window, as bill")
	status, err = r.LastStatus(ctx, "", "", 0)
	assert.NoError(t, err)
	assert.Empty(t, status, "an empty bill ID does not match a NULL")
}

// TestLiveOrderProviderRows: the name and the provider fee of a seed row (NULL fee is 0); no row is
// an error.
func TestLiveOrderProviderRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	name, err := r.ProviderName(ctx, 51)
	assert.NoError(t, err)
	assert.Equal(t, "parsian", name)
	fee, err := r.ProviderFee(ctx, 124325)
	assert.NoError(t, err)
	assert.Zero(t, fee, "the seed has NULL")
	_, err = c.Pool().Exec(ctx, `UPDATE providers SET provider_fee = 5000 WHERE id = 124325`)
	assert.NoError(t, err)
	fee, err = r.ProviderFee(ctx, 124325)
	assert.NoError(t, err)
	assert.Equal(t, uint64(5000), fee)
	_, err = r.ProviderName(ctx, 999999)
	assert.Error(t, err)
	_, err = r.ProviderFee(ctx, 999999)
	assert.Error(t, err)
}

// TestLiveOrderSideEffects: MarkInquiryPaid sets is_paid of the rows that are not copies;
// SetLastOrderDate sets last_order_date and a NULL payment_deadline of the saved bills of the bill
// ID and the bill type only.
func TestLiveOrderSideEffects(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	_, err := c.Pool().Exec(ctx, `INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, pay_id, is_cached)
		VALUES ('dddddddd-0000-4000-8000-000000000441', $1, $2, 124325, 'karashop', 'tel', $3, false),
		       ('dddddddd-0000-4000-8000-000000000442', $1, $2, 124325, 'karashop', 'tel', $3, true),
		       ('dddddddd-0000-4000-8000-000000000443', $1, $2, 124325, 'karashop', 'tel', '35000042', false)`, inqUser1, payTel, payPayID)
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, r.MarkInquiryPaid(ctx, payTel, payPayID))
	var paid []string
	rows, err := c.Pool().Query(ctx, `SELECT id::text FROM bill_inquiries WHERE is_paid ORDER BY id`)
	if assert.NoError(t, err) {
		for rows.Next() {
			var id string
			assert.NoError(t, rows.Scan(&id))
			paid = append(paid, id)
		}
		rows.Close()
	}
	assert.Equal(t, []string{"dddddddd-0000-4000-8000-000000000441"}, paid)

	_, err = c.Pool().Exec(ctx, `INSERT INTO bills (id, user_id, name, type, bill_id, payment_deadline)
		VALUES ('eeeeeeee-0000-4000-8000-000000000441', $1, 'n', 'telmidterm', $2, '2026-12-01'),
		       ('eeeeeeee-0000-4000-8000-000000000442', $1, 'n', 'tel', $2, '2026-12-01')`, inqUser1, payBillID)
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, r.SetLastOrderDate(ctx, payBillID, "telmidterm", "2026-11-15"))
	got := map[string]string{}
	rows, err = c.Pool().Query(ctx, `SELECT type, COALESCE(last_order_date, '<nil>') || '|' || COALESCE(payment_deadline, '<nil>') FROM bills WHERE bill_id = $1`, payBillID)
	if assert.NoError(t, err) {
		for rows.Next() {
			var k, v string
			assert.NoError(t, rows.Scan(&k, &v))
			got[k] = v
		}
		rows.Close()
	}
	assert.Equal(t, map[string]string{"telmidterm": "2026-11-15|<nil>", "tel": "<nil>|2026-12-01"}, got)
}
