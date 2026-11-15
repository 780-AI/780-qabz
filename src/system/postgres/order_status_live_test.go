package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"

	"qabz/core/order"
	"qabz/system/postgres"
)

// statusOrderID is the order ID number n of the status worker tests (task 045).
func statusOrderID(n int) string { return fmt.Sprintf("cccccccc-0000-4000-8000-%012d", n) }

// statusRow is one orders row of order n with the status, the provider and the age.
type statusRow struct {
	n        int
	status   string
	provider string
	age      time.Duration
}

// insertRows inserts the rows and sets their created_at to NOW() - age. Each row gets its own uuid.
func insertRows(t *testing.T, ctx context.Context, c *postgres.Client, rows []statusRow) {
	t.Helper()
	r := postgres.NewOrderRepository(c)
	for i, row := range rows {
		id := fmt.Sprintf("dddddddd-0000-4000-8000-%012d", 1000*row.n+i)
		o := payOrder(id, row.status)
		o.OrderID, o.ProviderName = statusOrderID(row.n), row.provider
		if !assert.NoError(t, r.Insert(ctx, o)) {
			t.FailNow()
		}
		_, err := c.Pool().Exec(ctx, `UPDATE orders SET created_at = NOW() - make_interval(secs => $2) WHERE uuid = $1::uuid`, id, row.age.Seconds())
		if !assert.NoError(t, err) {
			t.FailNow()
		}
	}
}

// TestLivePendingIDs (task 045): bill's GetAllPending. Only the provider; only the newest row in the
// window; pending and in-progress only; the oldest first; LIMIT and OFFSET; a final row newer than
// the window keeps the order in the list (C6).
func TestLivePendingIDs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	const start, end = 30 * time.Minute, 10 * time.Hour

	insertRows(t, ctx, c, []statusRow{
		{1, order.StatusInitial, "parsian", 5 * time.Hour}, {1, order.StatusInProgress, "parsian", 5 * time.Hour}, {1, order.StatusPending, "parsian", 5*time.Hour - time.Second},
		{2, order.StatusInProgress, "parsian", 3 * time.Hour},
		{3, order.StatusPending, "parsian", 4 * time.Hour}, {3, order.StatusSuccess, "parsian", 4*time.Hour - time.Second}, // final: out
		{4, order.StatusPending, "irancell", 2 * time.Hour},                                                 // another provider: out
		{5, order.StatusPending, "parsian", 11 * time.Hour},                                                 // older than end: out
		{6, order.StatusPending, "parsian", 10 * time.Minute},                                               // newer than start: out
		{7, order.StatusPending, "parsian", 2 * time.Hour}, {7, order.StatusFailed, "parsian", time.Minute}, // final row newer than start: in (C6)
		{8, order.StatusInitial, "parsian", time.Hour}, // initial: out
	})

	all, err := r.PendingIDs(ctx, "parsian", 100, 0, start, end)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, []string{statusOrderID(1), statusOrderID(2), statusOrderID(7)}, all, "the oldest newest row first") {
		return // the pages below slice all
	}

	page1, err := r.PendingIDs(ctx, "parsian", 2, 0, start, end)
	assert.NoError(t, err)
	assert.Equal(t, all[:2], page1)
	page2, err := r.PendingIDs(ctx, "parsian", 2, 2, start, end)
	assert.NoError(t, err)
	assert.Equal(t, all[2:], page2)
	none, err := r.PendingIDs(ctx, "parsian", 2, 4, start, end)
	assert.NoError(t, err)
	assert.Empty(t, none)

	other, err := r.PendingIDs(ctx, "irancell", 100, 0, start, end)
	assert.NoError(t, err)
	assert.Equal(t, []string{statusOrderID(4)}, other)

	_, err = postgres.NewOrderRepository(nil).PendingIDs(ctx, "parsian", 1, 0, start, end)
	assert.Error(t, err)
}

// TestLiveInsertFinal (task 045): bill's NewOrderWithFinalStatus. A pending order gets the final
// row; a second final row is ErrAlreadyFinal and is not saved; an order with no row of the last 24 h
// is pgx.ErrNoRows.
func TestLiveInsertFinal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	insertRows(t, ctx, c, []statusRow{{11, order.StatusPending, "parsian", time.Hour}, {12, order.StatusPending, "parsian", 25 * time.Hour}})

	pending, err := r.Latest(ctx, statusOrderID(11), order.LatestWindow)
	if !assert.NoError(t, err) {
		return
	}
	final := pending
	final.ID, final.Status = "eeeeeeee-0000-4000-8000-000000000001", order.StatusSuccess
	if !assert.NoError(t, r.InsertFinal(ctx, final)) {
		return
	}
	got, err := r.Latest(ctx, statusOrderID(11), order.LatestWindow)
	if assert.NoError(t, err) {
		got.CreatedAt, final.CreatedAt = time.Time{}, time.Time{}
		assert.Equal(t, final, got, "the copy with the new status")
	}

	again := final
	again.ID, again.Status = "eeeeeeee-0000-4000-8000-000000000002", order.StatusFailed
	err = r.InsertFinal(ctx, again)
	assert.True(t, errors.Is(err, postgres.ErrAlreadyFinal), "%v", err)
	var n int
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM orders WHERE order_id = $1`, statusOrderID(11)).Scan(&n))
	assert.Equal(t, 2, n, "nothing saved after a final status")

	old, err := r.Latest(ctx, statusOrderID(12), order.LatestWindow)
	if assert.NoError(t, err) {
		old.ID, old.Status = "eeeeeeee-0000-4000-8000-000000000003", order.StatusSuccess
		err = r.InsertFinal(ctx, old)
		assert.True(t, errors.Is(err, pgx.ErrNoRows), "no row of the last 24 h: %v", err)
	}

	assert.Error(t, postgres.NewOrderRepository(nil).InsertFinal(ctx, final))
}

// confirmRow is one orders row of the settlement tests (task 046): order n, the bill type, the
// status, is_raw, provider_confirmed and the age.
type confirmRow struct {
	n         int
	billType  string
	status    string
	raw       bool
	confirmed bool
	age       time.Duration
}

func insertConfirmRows(t *testing.T, ctx context.Context, c *postgres.Client, rows []confirmRow) {
	t.Helper()
	r := postgres.NewOrderRepository(c)
	for i, row := range rows {
		id := fmt.Sprintf("eeeeeeee-0000-4000-8000-%012d", 1000*row.n+i)
		o := payOrder(id, row.status)
		o.OrderID, o.BillType, o.IsRaw, o.ProviderConfirmed, o.RRN = statusOrderID(row.n), row.billType, row.raw, row.confirmed, "rrn-1"
		if !assert.NoError(t, r.Insert(ctx, o)) {
			t.FailNow()
		}
		_, err := c.Pool().Exec(ctx, `UPDATE orders SET created_at = NOW() - make_interval(secs => $2) WHERE uuid = $1::uuid`, id, row.age.Seconds())
		if !assert.NoError(t, err) {
			t.FailNow()
		}
	}
}

// TestLiveConfirmCandidates (task 046): bill's GetOrdersForConfirmPayment. Only the bill type; only
// success rows in the window (strict bounds); not raw; not confirmed; the newest success row of
// each order with every column; order_id order; the keyset page after an order_id; a confirmed
// order leaves the list.
func TestLiveConfirmCandidates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)

	insertConfirmRows(t, ctx, c, []confirmRow{
		{1, "tel", order.StatusInProgress, false, false, 3 * time.Hour}, {1, "tel", order.StatusSuccess, false, false, 3*time.Hour - time.Second},
		{2, "tel", order.StatusSuccess, false, false, 2 * time.Hour},
		{3, "tel", order.StatusSuccess, false, false, 4 * time.Hour}, {3, "tel", order.StatusSuccess, false, false, 4*time.Hour - 2*time.Second}, // two success rows: the newest
		{4, "tel", order.StatusSuccess, false, true, time.Hour},          // confirmed: out
		{5, "tel", order.StatusSuccess, true, false, time.Hour},          // raw: out
		{6, "tel", order.StatusPending, false, false, time.Hour},         // not paid: out
		{7, "tel", order.StatusFailed, false, false, time.Hour},          // failed: out
		{8, "tel", order.StatusSuccess, false, false, 30 * time.Hour},    // older than end: out
		{9, "tel", order.StatusSuccess, false, false, time.Minute},       // newer than start: out
		{10, "telmidterm", order.StatusSuccess, false, false, time.Hour}, // another bill type: out of tel
	})
	now := time.Now()
	from, to := now.Add(-24*time.Hour), now.Add(-5*time.Minute)

	all, err := r.ConfirmCandidates(ctx, "tel", "", 100, from, to)
	if !assert.NoError(t, err) || !assert.Len(t, all, 3) {
		return
	}
	assert.Equal(t, []string{statusOrderID(1), statusOrderID(2), statusOrderID(3)}, []string{all[0].OrderID, all[1].OrderID, all[2].OrderID})
	assert.Equal(t, "eeeeeeee-0000-4000-8000-000000003004", all[2].ID, "the newest success row of order 3")
	got := all[0]
	assert.Equal(t, []string{"tel", payBillID, payPayID, payTel, "parsian", "rrn-1", order.StatusSuccess},
		[]string{got.BillType, got.BillID, got.PayID, got.Data, got.ProviderName, got.RRN, got.Status})
	assert.Equal(t, int64(120000), got.BasePrice)
	assert.False(t, got.IsRaw)
	assert.False(t, got.ProviderConfirmed)
	assert.WithinDuration(t, now.Add(-3*time.Hour+time.Second), got.CreatedAt, time.Minute)

	page, err := r.ConfirmCandidates(ctx, "tel", "", 2, from, to)
	if assert.NoError(t, err) && assert.Len(t, page, 2) {
		next, err := r.ConfirmCandidates(ctx, "tel", page[1].OrderID, 2, from, to)
		if assert.NoError(t, err) && assert.Len(t, next, 1) {
			assert.Equal(t, statusOrderID(3), next[0].OrderID)
		}
	}
	mid, err := r.ConfirmCandidates(ctx, "telmidterm", "", 100, from, to)
	if assert.NoError(t, err) && assert.Len(t, mid, 1) {
		assert.Equal(t, statusOrderID(10), mid[0].OrderID)
	}

	if !assert.NoError(t, r.ConfirmProvider(ctx, statusOrderID(2))) {
		return
	}
	left, err := r.ConfirmCandidates(ctx, "tel", "", 100, from, to)
	if assert.NoError(t, err) && assert.Len(t, left, 2) {
		assert.Equal(t, []string{statusOrderID(1), statusOrderID(3)}, []string{left[0].OrderID, left[1].OrderID})
	}

	_, err = r.ConfirmCandidates(ctx, "tel", "not-a-uuid", 1, from, to)
	assert.Error(t, err)
	var nilR *postgres.OrderRepository
	_, err = nilR.ConfirmCandidates(ctx, "tel", "", 1, from, to)
	assert.Error(t, err)
}
