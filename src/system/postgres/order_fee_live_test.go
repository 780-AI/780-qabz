package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"

	"qabz/core/fee"
	"qabz/core/inquiry"
	"qabz/core/order"
	"qabz/provider"
	"qabz/system/postgres"
)

// orderRow reads every column of an orders row as text ("<nil>" for NULL), by order_id.
func orderRow(t *testing.T, ctx context.Context, c *postgres.Client, orderID string) map[string]string {
	t.Helper()
	cols := []string{"uuid", "user_id", "bill_type", "bill_id", "pay_id", "option", "price", "data", "ref_id", "provider_id",
		"provider_name", "rrn", "message", "status", "status_code", "fee", "fee_type", "fee_config", "fee_id", "base_price",
		"is_raw", "provider_confirmed", "provider_fee", "bill_inquiry_id"}
	sql := "SELECT "
	for i, col := range cols {
		if i > 0 {
			sql += ", "
		}
		sql += "COALESCE(" + col + "::text, '<nil>')"
	}
	sql += ", created_at IS NOT NULL, updated_at IS NOT NULL FROM orders WHERE order_id = $1::uuid"
	vals := make([]string, len(cols))
	dest := make([]any, 0, len(cols)+2)
	for i := range vals {
		dest = append(dest, &vals[i])
	}
	var created, updated bool
	dest = append(dest, &created, &updated)
	if !assert.NoError(t, c.Pool().QueryRow(ctx, sql, orderID).Scan(dest...)) {
		t.FailNow()
	}
	assert.True(t, created && updated, "created_at and updated_at get the defaults")
	out := map[string]string{}
	for i, col := range cols {
		out[col] = vals[i]
	}
	return out
}

// TestLiveOrderInsert: a full row and a row of zero values, read back with a direct query: a zero
// value is NULL, as bill's go-pg insert.
func TestLiveOrderInsert(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	r := postgres.NewOrderRepository(c)
	feeID := int64(900001)
	full := order.Order{
		ID: "aaaaaaaa-0000-4000-8000-000000000001", OrderID: "bbbbbbbb-0000-4000-8000-000000000001", UserID: inqUser1,
		BillType: "irancell", BillID: "1234567890125", PayID: "irancell", Option: "irancell", Price: 459000, Data: "989350000391",
		ProviderID: 26, ProviderName: "irancell", Status: order.StatusInitial, Fee: 9000, FeeType: fee.TypeFixed,
		FeeConfig: []byte(`{"amount":9000}`), FeeID: &feeID, BasePrice: 450000, IsRaw: true, ProviderFee: 7,
		BillInquiryID: "cccccccc-0000-4000-8000-000000000001",
	}
	if !assert.NoError(t, r.Insert(ctx, full)) {
		return
	}
	assert.Equal(t, map[string]string{
		"uuid": full.ID, "user_id": inqUser1, "bill_type": "irancell", "bill_id": "1234567890125", "pay_id": "irancell",
		"option": "irancell", "price": "459000", "data": "989350000391", "ref_id": "<nil>", "provider_id": "26",
		"provider_name": "irancell", "rrn": "<nil>", "message": "<nil>", "status": "initial", "status_code": "<nil>",
		"fee": "9000", "fee_type": "fixed", "fee_config": `{"amount": 9000}`, "fee_id": "900001", "base_price": "450000",
		"is_raw": "true", "provider_confirmed": "<nil>", "provider_fee": "7", "bill_inquiry_id": full.BillInquiryID,
	}, orderRow(t, ctx, c, full.OrderID))

	zero := order.Order{ID: "aaaaaaaa-0000-4000-8000-000000000002", OrderID: "bbbbbbbb-0000-4000-8000-000000000002", Status: order.StatusInitial}
	if !assert.NoError(t, r.Insert(ctx, zero)) {
		return
	}
	got := orderRow(t, ctx, c, zero.OrderID)
	for col, v := range got {
		if col == "uuid" || col == "status" {
			continue
		}
		assert.Equal(t, "<nil>", v, col+": a zero value is NULL")
	}

	// The same uuid again breaks the primary key.
	assert.Error(t, r.Insert(ctx, zero))
}

// TestLiveInquiryProvider reads the provider and the bill type of a saved inquiry; no row is
// pgx.ErrNoRows. A landline row gives its service status (task 044).
func TestLiveInquiryProvider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	id := "dddddddd-0000-4000-8000-000000000001"
	row := inquiry.Row{ID: id, UserID: inqUser1, InquiryID: "989350000391", ProviderID: 38, ProviderName: "irancell",
		BillType: "irancell", InquiryType: "final_term", Amount: 450000}
	if !assert.NoError(t, postgres.NewInquiryRepository(c).SaveInquiries(ctx, []inquiry.Row{row})) {
		return
	}
	r := postgres.NewOrderRepository(c)
	got, err := r.InquiryFee(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, order.InquiryFee{ProviderName: "irancell", ProviderID: 38, BillType: "irancell"}, got)
	_, err = r.InquiryFee(ctx, "dddddddd-0000-4000-8000-000000000002")
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	tel := inquiry.Row{ID: "dddddddd-0000-4000-8000-000000000003", UserID: inqUser1, InquiryID: "982188776655", ProviderID: 124325,
		ProviderName: "karashop", BillType: "tel", InquiryType: "midterm", Amount: 120000,
		Details: &provider.Details{Tel: &provider.TelDetails{ServiceStatus: provider.TelServiceFullyDisconnected}}}
	if !assert.NoError(t, postgres.NewInquiryRepository(c).SaveInquiries(ctx, []inquiry.Row{tel})) {
		return
	}
	got, err = r.InquiryFee(ctx, tel.ID)
	assert.NoError(t, err)
	assert.Equal(t, order.InquiryFee{ProviderName: "karashop", ProviderID: 124325, BillType: "tel", ServiceStatus: "fullyDisconnected"}, got)
	// A service_status that is not a string reads as "".
	_, err = c.Pool().Exec(ctx, `UPDATE bill_inquiries SET extra_data = '{"service_status": 3}' WHERE id = $1`, tel.ID)
	assert.NoError(t, err)
	got, err = r.InquiryFee(ctx, tel.ID)
	assert.NoError(t, err)
	assert.Empty(t, got.ServiceStatus)
}

// TestLiveFeeLookups: each query of bill on the real table: the newest row that is not deleted,
// the user and provider kinds kept apart, no row is nil, the users with a fee.
func TestLiveFeeLookups(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := seededClient(t, ctx)
	_, err := c.Pool().Exec(ctx, `INSERT INTO fees (id, title, provider_id, type, config, user_id, created_at, deleted_at) VALUES
		(1, 'p old', 26, 'fixed', '{"amount": 1}', NULL, now() - interval '2 hour', NULL),
		(2, 'p deleted', 26, 'fixed', '{"amount": 2}', NULL, now(), now()),
		(3, 'u+p', 26, 'percent', '{"percent": 3}', $1, now() - interval '1 hour', NULL),
		(4, 'u only', NULL, 'stair', '[{"min": 0, "max": 9, "type": "fixed", "amount": 4}]', $1, now(), NULL),
		(5, 'u2 deleted', NULL, 'fixed', '{"amount": 5}', $2, now(), now()),
		(6, 'p tie a', 27, 'fixed', '{"amount": 6}', NULL, '2026-01-01T00:00:00Z', NULL),
		(7, 'p tie b', 27, 'fixed', '{"amount": 7}', NULL, '2026-01-01T00:00:00Z', NULL)`, inqUser1, inqUser2)
	if !assert.NoError(t, err) {
		return
	}
	r := postgres.NewFeeRepository(c)

	ids, err := r.UserIDs(ctx)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{inqUser1, inqUser1}, ids, "a deleted row does not count")

	got, err := r.FindProvider(ctx, 26)
	if assert.NoError(t, err) && assert.NotNil(t, got) {
		// 900001 (the dev seed, created now) is newer than row 1; row 2 is deleted.
		assert.Equal(t, fee.Row{ID: 900001, Type: "fixed", Config: []byte(`{"amount": 9000}`)}, *got)
	}
	got, err = r.FindProvider(ctx, 27)
	if assert.NoError(t, err) && assert.NotNil(t, got) {
		assert.Equal(t, int64(7), got.ID, "the same created_at: the larger id")
	}
	got, err = r.FindUserProvider(ctx, inqUser1, 26)
	if assert.NoError(t, err) && assert.NotNil(t, got) {
		assert.Equal(t, fee.Row{ID: 3, Type: "percent", Config: []byte(`{"percent": 3}`)}, *got)
	}
	got, err = r.FindUser(ctx, inqUser1)
	if assert.NoError(t, err) && assert.NotNil(t, got) {
		assert.Equal(t, int64(4), got.ID)
	}
	for name, find := range map[string]func() (*fee.Row, error){
		"provider with no row":    func() (*fee.Row, error) { return r.FindProvider(ctx, 999) },
		"user deleted":            func() (*fee.Row, error) { return r.FindUser(ctx, inqUser2) },
		"user with no row":        func() (*fee.Row, error) { return r.FindUserProvider(ctx, inqUser2, 26) },
		"user row is not a match": func() (*fee.Row, error) { return r.FindUserProvider(ctx, inqUser1, 27) },
	} {
		got, err := find()
		assert.NoError(t, err, name)
		assert.Nil(t, got, name)
	}

	// The resolver on the real table: the user row wins over the provider row.
	res, err := fee.NewResolver(fee.Options{Repository: r})
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		f, err := res.Resolve(ctx, inqUser1, 26, 1000)
		assert.NoError(t, err)
		assert.Equal(t, []any{int64(30), fee.TypePercent, fee.SourceUserProvider}, []any{f.Fee, f.Type, f.Source})
	}

	// A user ID that is not a UUID is an error of the database, as bill.
	_, err = r.FindUser(ctx, "not-a-uuid")
	assert.Error(t, err)
}

func TestNoPoolRepositories(t *testing.T) {
	ctx := context.Background()
	o := postgres.NewOrderRepository(nil)
	assert.Error(t, o.Insert(ctx, order.Order{}))
	_, err := o.InquiryFee(ctx, "x")
	assert.Error(t, err)
	_, err = o.ProviderFee(ctx, 1)
	assert.Error(t, err)
	_, err = o.ProviderName(ctx, 1)
	assert.Error(t, err)
	_, err = o.LastStatus(ctx, "b", "p", time.Hour)
	assert.Error(t, err)
	_, err = o.Latest(ctx, "x", time.Hour)
	assert.Error(t, err)
	assert.Error(t, o.ConfirmProvider(ctx, "x"))
	assert.Error(t, o.MarkInquiryPaid(ctx, "x", "y"))
	assert.Error(t, o.SetLastOrderDate(ctx, "x", "y", "z"))
	f := postgres.NewFeeRepository(nil)
	_, err = f.UserIDs(ctx)
	assert.Error(t, err)
	_, err = f.FindProvider(ctx, 1)
	assert.Error(t, err)
	assert.Error(t, postgres.NewOrderRepository(nil).Insert(ctx, order.Order{ProviderFee: 1 << 63}))
}
