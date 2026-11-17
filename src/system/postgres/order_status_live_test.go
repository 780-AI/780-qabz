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

// cursorIDs gives the order IDs of the cursors.
func cursorIDs(cs []order.Cursor) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.OrderID)
	}
	return out
}

// TestLivePendingIDs (task 045; task 050 D4): bill's GetAllPending with a keyset page. Only the
// provider; only the newest row in the window; pending and in-progress only; the oldest first; LIMIT
// and the cursor (created_at of the newest row in the window, order_id); a final row newer than the
// window keeps the order in the list (C6).
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

	all, err := r.PendingIDs(ctx, "parsian", 100, order.Cursor{}, start, end)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Equal(t, []string{statusOrderID(1), statusOrderID(2), statusOrderID(7)}, cursorIDs(all), "the oldest newest row first") {
		return // the pages below slice all
	}
	for _, cur := range all {
		assert.False(t, cur.CreatedAt.IsZero(), "the cursor has the created_at of the newest row in the window")
	}

	page1, err := r.PendingIDs(ctx, "parsian", 2, order.Cursor{}, start, end)
	assert.NoError(t, err)
	assert.Equal(t, all[:2], page1)
	page2, err := r.PendingIDs(ctx, "parsian", 2, page1[1], start, end)
	assert.NoError(t, err)
	assert.Equal(t, all[2:], page2)
	none, err := r.PendingIDs(ctx, "parsian", 2, page2[0], start, end)
	assert.NoError(t, err)
	assert.Empty(t, none)

	other, err := r.PendingIDs(ctx, "irancell", 100, order.Cursor{}, start, end)
	assert.NoError(t, err)
	assert.Equal(t, []string{statusOrderID(4)}, cursorIDs(other))

	_, err = postgres.NewOrderRepository(nil).PendingIDs(ctx, "parsian", 1, order.Cursor{}, start, end)
	assert.Error(t, err)
}

// TestLivePendingIDsFallback (task 058): an order that went from Parsian to Jiring Switch has rows of
// two providers. Only the provider of its newest row in the window gets it: the Parsian worker does
// not see the old Parsian in-progress row, the Jiring Switch worker sees the pending order.
func TestLivePendingIDsFallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	const start, end = 30 * time.Minute, 10 * time.Hour
	insertRows(t, ctx, c, []statusRow{
		// 31: Parsian not reached, Jiring Switch pending: the Jiring order.
		{31, order.StatusInitial, "parsian", 2 * time.Hour}, {31, order.StatusInProgress, "parsian", 2*time.Hour - time.Second},
		{31, order.StatusInProgress, "jiring-switch", 2*time.Hour - 2*time.Second}, {31, order.StatusPending, "jiring-switch", 2*time.Hour - 3*time.Second},
		// 32: Parsian not reached, Jiring Switch success: no worker.
		{32, order.StatusInProgress, "parsian", 3 * time.Hour}, {32, order.StatusInProgress, "jiring-switch", 3*time.Hour - time.Second},
		{32, order.StatusSuccess, "jiring-switch", 3*time.Hour - 2*time.Second},
		// 33: a plain Parsian pending order.
		{33, order.StatusPending, "parsian", 4 * time.Hour},
		// 34: the Jiring rows are newer than start: the newest row in the window is the Parsian
		// in-progress row, as before task 058 for a row out of the window. The worker skips it: its
		// read of the order (Latest, 48 h) gives the Jiring row (reconcile TestCheckOtherProvider).
		{34, order.StatusInProgress, "parsian", 2 * time.Hour}, {34, order.StatusPending, "jiring-switch", 10 * time.Minute},
	})
	parsian, err := r.PendingIDs(ctx, "parsian", 100, order.Cursor{}, start, end)
	if assert.NoError(t, err) {
		assert.Equal(t, []string{statusOrderID(33), statusOrderID(34)}, cursorIDs(parsian), "no order whose newest row is of Jiring Switch")
	}
	jiring, err := r.PendingIDs(ctx, "jiring-switch", 100, order.Cursor{}, start, end)
	if assert.NoError(t, err) {
		assert.Equal(t, []string{statusOrderID(31)}, cursorIDs(jiring))
	}
}

// TestLivePendingIDsSameTime (task 050 D4): orders whose newest rows have the same created_at are
// ordered and paged by order_id, so a page boundary in such a group skips and repeats nothing.
func TestLivePendingIDsSameTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	rows := make([]statusRow, 0, 5)
	for n := 21; n <= 25; n++ {
		rows = append(rows, statusRow{n, order.StatusPending, "parsian", time.Hour})
	}
	insertRows(t, ctx, c, rows)
	_, err := c.Pool().Exec(ctx, `UPDATE orders SET created_at = date_trunc('second', NOW()) - INTERVAL '1 hour' WHERE order_id::text LIKE 'cccccccc-%'`)
	if !assert.NoError(t, err) {
		return
	}
	var got []string
	after := order.Cursor{}
	for i := 0; i < 5; i++ {
		page, err := r.PendingIDs(ctx, "parsian", 2, after, 30*time.Minute, 10*time.Hour)
		if !assert.NoError(t, err) || len(page) == 0 {
			break
		}
		got = append(got, cursorIDs(page)...)
		after = page[len(page)-1]
	}
	assert.Equal(t, []string{statusOrderID(21), statusOrderID(22), statusOrderID(23), statusOrderID(24), statusOrderID(25)}, got)
}

// TestLiveInsertFinal (task 045; task 050 D5, D6): bill's NewOrderWithFinalStatus. A pending order
// gets the final row; the report runs after the insert, inside the transaction (the row is not seen
// from another connection before the commit); a second final row is ErrAlreadyFinal, is not saved
// and is not reported; an order of 25 h gets its final row (one window with the read: bill locks
// only the last 24 h and fails); an order with no row of the last 48 h is pgx.ErrNoRows; a failed
// report rolls the insert back.
func TestLiveInsertFinal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	insertRows(t, ctx, c, []statusRow{
		{11, order.StatusPending, "parsian", time.Hour}, {12, order.StatusPending, "parsian", 25 * time.Hour},
		{13, order.StatusPending, "parsian", 49 * time.Hour}, {14, order.StatusInProgress, "irancell", time.Hour},
	})
	count := func(n int) int {
		var k int
		assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM orders WHERE order_id = $1`, statusOrderID(n)).Scan(&k))
		return k
	}

	pending, err := r.Latest(ctx, statusOrderID(11), order.LatestWindow)
	if !assert.NoError(t, err) {
		return
	}
	final := pending
	final.ID, final.Status = "eeeeeeee-0000-4000-8000-000000000001", order.StatusSuccess
	reports := 0
	seenOutside := -1
	if !assert.NoError(t, r.InsertFinal(ctx, final, func(context.Context) error {
		reports++
		seenOutside = count(11) // another connection: the insert is not committed yet
		return nil
	})) {
		return
	}
	assert.Equal(t, 1, reports)
	assert.Equal(t, 1, seenOutside, "the report runs before the commit")
	got, err := r.Latest(ctx, statusOrderID(11), order.LatestWindow)
	if assert.NoError(t, err) {
		got.CreatedAt, final.CreatedAt = time.Time{}, time.Time{}
		assert.Equal(t, final, got, "the copy with the new status")
	}

	again := final
	again.ID, again.Status = "eeeeeeee-0000-4000-8000-000000000002", order.StatusFailed
	err = r.InsertFinal(ctx, again, func(context.Context) error { reports++; return nil })
	assert.True(t, errors.Is(err, postgres.ErrAlreadyFinal), "%v", err)
	assert.True(t, errors.Is(err, order.ErrAlreadyFinal), "the order sentinel")
	assert.Equal(t, 1, reports, "a final row: nothing reported")
	assert.Equal(t, 2, count(11), "nothing saved after a final status")

	old, err := r.Latest(ctx, statusOrderID(12), order.LatestWindow)
	if assert.NoError(t, err) {
		old.ID, old.Status = "eeeeeeee-0000-4000-8000-000000000003", order.StatusSuccess
		assert.NoError(t, r.InsertFinal(ctx, old, nil), "an order of 25 h is saved (task 050 D5)")
		assert.Equal(t, 2, count(12))
	}

	gone := pending
	gone.OrderID, gone.ID = statusOrderID(13), "eeeeeeee-0000-4000-8000-000000000004"
	err = r.InsertFinal(ctx, gone, nil)
	assert.True(t, errors.Is(err, pgx.ErrNoRows), "no row of the last 48 h: %v", err)

	ir, err := r.Latest(ctx, statusOrderID(14), order.LatestWindow)
	if assert.NoError(t, err) {
		ir.ID, ir.Status = "eeeeeeee-0000-4000-8000-000000000005", order.StatusSuccess
		err = r.InsertFinal(ctx, ir, func(context.Context) error { return errors.New("kafka down") })
		assert.ErrorContains(t, err, "kafka down")
		assert.Equal(t, 1, count(14), "a failed report saves nothing (task 050 D6)")
		latest, err := r.Latest(ctx, statusOrderID(14), order.LatestWindow)
		if assert.NoError(t, err) {
			assert.Equal(t, order.StatusInProgress, latest.Status)
		}
	}

	assert.Error(t, postgres.NewOrderRepository(nil).InsertFinal(ctx, final, nil))
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
