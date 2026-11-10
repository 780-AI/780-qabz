package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/inquiry"
	"qabz/provider"
	"qabz/system/postgres"
)

const (
	inqBill  = "1234567890125"
	inqUser1 = "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"
	inqUser2 = "0b9d2c4e-8a3f-4e61-b7d2-5c1a9e0f3d22"
)

func liveInquiryRepo(t *testing.T, ctx context.Context) (*postgres.Client, *postgres.InquiryRepository) {
	t.Helper()
	c := seededClient(t, ctx)
	r := postgres.NewInquiryRepository(c)
	if !assert.NotNil(t, r) {
		t.FailNow()
	}
	return c, r
}

func inqRow(id, user string, provider int64) inquiry.Row {
	return inquiry.Row{ID: id, UserID: user, InquiryID: inqBill, ProviderID: provider, ProviderName: "qabzino", BillType: "water",
		ProviderTraceID: "trace-" + id, InquiryType: "regular", BillID: inqBill, PayID: "2500012345", PaymentDeadline: "2030-01-01", Amount: 25000000}
}

// TestLiveInquiryRows saves rows and reads them with a direct SQL query and through FindValid.
// The lookup rules are checked on the real table: provider, ttl, copy, paid, shared between users,
// the limit of 2, the order.
func TestLiveInquiryRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, r := liveInquiryRepo(t, ctx)

	id1 := "11111111-1111-4111-8111-111111111111"
	assert.NoError(t, r.SaveInquiries(ctx, []inquiry.Row{inqRow(id1, inqUser1, 36)}))

	// The columns as written.
	var userID, inquiryID, name, billType, tid, typ, billID, payID, deadline string
	var providerID, amount int64
	var cached, paid bool
	var extra *string
	var created, updated time.Time
	err := c.Pool().QueryRow(ctx, `SELECT user_id::text, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, bill_id, pay_id,
		payment_deadline, amount, extra_data::text, is_cached, is_paid, created_at, updated_at FROM bill_inquiries WHERE id = $1`, id1).
		Scan(&userID, &inquiryID, &providerID, &name, &billType, &tid, &typ, &billID, &payID, &deadline, &amount, &extra, &cached, &paid, &created, &updated)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []any{inqUser1, inqBill, int64(36), "qabzino", "water", "trace-" + id1, "regular", inqBill, "2500012345", "2030-01-01", int64(25000000), false, false},
		[]any{userID, inquiryID, providerID, name, billType, tid, typ, billID, payID, deadline, amount, cached, paid})
	assert.Nil(t, extra, "extra_data is NULL")
	assert.False(t, created.IsZero(), "created_at has its default")
	assert.False(t, updated.IsZero())

	// FindValid: another user's call finds the row (shared cache).
	got, err := r.FindValid(ctx, inqBill, time.Hour, 36)
	if assert.NoError(t, err) && assert.Len(t, got, 1) {
		assert.Equal(t, inqRow(id1, inqUser1, 36), got[0])
	}
	// Another provider, another bill, a ttl that is over: no row.
	got, err = r.FindValid(ctx, inqBill, time.Hour, 44)
	assert.NoError(t, err)
	assert.Empty(t, got, "another provider")
	got, err = r.FindValid(ctx, "9999999999999", time.Hour, 36)
	assert.NoError(t, err)
	assert.Empty(t, got, "another bill")
	time.Sleep(30 * time.Millisecond)
	got, err = r.FindValid(ctx, inqBill, 10*time.Millisecond, 36)
	assert.NoError(t, err)
	assert.Empty(t, got, "expired")
	got, err = r.FindValid(ctx, inqBill, 0, 36)
	assert.NoError(t, err)
	assert.Empty(t, got, "a zero ttl matches nothing")

	// A copy (is_cached) and a paid row are not found.
	cp := inqRow("22222222-2222-4222-8222-222222222222", inqUser2, 36)
	cp.IsCached = true
	assert.NoError(t, r.SaveInquiries(ctx, []inquiry.Row{cp}))
	got, err = r.FindValid(ctx, inqBill, time.Hour, 36)
	assert.NoError(t, err)
	assert.Len(t, got, 1, "the copy does not answer")
	_, err = c.Pool().Exec(ctx, `UPDATE bill_inquiries SET is_paid = true WHERE id = $1`, id1)
	assert.NoError(t, err)
	got, err = r.FindValid(ctx, inqBill, time.Hour, 36)
	assert.NoError(t, err)
	assert.Empty(t, got, "a paid row does not answer")
}

// TestLiveInquiryTwoRowsAndNulls saves three rows in one call (an empty value is NULL) and checks
// that FindValid gives the newest two, in a fixed order.
func TestLiveInquiryTwoRowsAndNulls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, r := liveInquiryRepo(t, ctx)
	empty := inquiry.Row{ID: "33333333-3333-4333-8333-333333333333", UserID: inqUser1, InquiryID: inqBill, ProviderID: 36, ProviderName: "qabzino", BillType: "water"}
	b := inqRow("44444444-4444-4444-8444-444444444444", inqUser1, 36)
	d := inqRow("55555555-5555-4555-8555-555555555555", inqUser1, 36)
	assert.NoError(t, r.SaveInquiries(ctx, []inquiry.Row{empty, b, d}))

	var tid, typ, billID, payID, deadline *string
	var amount *int64
	err := c.Pool().QueryRow(ctx, `SELECT tid, inquiry_type, bill_id, pay_id, payment_deadline, amount FROM bill_inquiries WHERE id = $1`, empty.ID).
		Scan(&tid, &typ, &billID, &payID, &deadline, &amount)
	if assert.NoError(t, err) {
		assert.Nil(t, tid)
		assert.Nil(t, typ)
		assert.Nil(t, billID)
		assert.Nil(t, payID)
		assert.Nil(t, deadline)
		if assert.NotNil(t, amount) {
			assert.Equal(t, int64(0), *amount)
		}
	}
	got, err := r.FindValid(ctx, inqBill, time.Hour, 36)
	if assert.NoError(t, err) && assert.Len(t, got, 2, "the limit is 2") {
		// Same created_at (one statement): id DESC fixes the order.
		assert.Equal(t, []string{d.ID, b.ID}, []string{got[0].ID, got[1].ID})
	}

	// A broken row stops the whole write: all or none.
	before := countRows(t, ctx, c, "bill_inquiries")
	bad := inqRow("66666666-6666-4666-8666-666666666666", inqUser1, 36)
	dup := bad
	assert.Error(t, r.SaveInquiries(ctx, []inquiry.Row{bad, dup}), "the same id twice breaks the primary key")
	assert.Equal(t, before, countRows(t, ctx, c, "bill_inquiries"), "nothing is written")
	tooLong := inqRow("77777777-7777-4777-8777-777777777777", inqUser1, 36)
	tooLong.ProviderName = "a-name-longer-than-15"
	assert.Error(t, r.SaveInquiries(ctx, []inquiry.Row{tooLong}))
}

func countRows(t *testing.T, ctx context.Context, c *postgres.Client, table string) int {
	t.Helper()
	var n int
	if !assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&n)) {
		t.FailNow()
	}
	return n
}

// TestLiveInquiryRequestRow checks the request row: INITIAL first, then the status and the trace.
func TestLiveInquiryRequestRow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, r := liveInquiryRepo(t, ctx)
	q := inquiry.RequestRow{ID: "88888888-8888-4888-8888-888888888888", UserID: inqUser1, Dialer: "989121234567", InquiryID: inqBill,
		BillType: "water", TID: "112345678901234", ProviderName: "qabzino", Status: inquiry.RequestInitial}
	assert.NoError(t, r.CreateRequest(ctx, q))

	read := func() (status string, trace *string, tid, dialer, name, bt string, created time.Time) {
		err := c.Pool().QueryRow(ctx, `SELECT status, provider_trace_id, tid, dialer, provider_name, bill_type, created_at FROM bill_inquiry_requests WHERE id = $1`, q.ID).
			Scan(&status, &trace, &tid, &dialer, &name, &bt, &created)
		assert.NoError(t, err)
		return
	}
	status, trace, tid, dialer, name, bt, created := read()
	assert.Equal(t, "initial", status)
	assert.Nil(t, trace)
	assert.Equal(t, []string{"112345678901234", "989121234567", "qabzino", "water"}, []string{tid, dialer, name, bt})
	assert.False(t, created.IsZero())

	assert.NoError(t, r.FinishRequest(ctx, q.ID, "success", "provider-trace"))
	status, trace, _, _, _, _, _ = read()
	assert.Equal(t, "success", status)
	if assert.NotNil(t, trace) {
		assert.Equal(t, "provider-trace", *trace)
	}
	assert.NoError(t, r.FinishRequest(ctx, q.ID, "failed", ""))
	status, trace, _, _, _, _, _ = read()
	assert.Equal(t, "failed", status)
	assert.Nil(t, trace, "an empty trace is NULL")
	assert.Equal(t, 1, countRows(t, ctx, c, "bill_inquiry_requests"), "one row, updated in place")

	assert.ErrorContains(t, r.FinishRequest(ctx, "99999999-9999-4999-8999-999999999999", "failed", ""), "0 rows updated")
	assert.Error(t, r.CreateRequest(ctx, q), "the primary key is unique")
}

// TestLiveInquiryExtraData (task 022): the details of an electricity inquiry are written to
// extra_data (jsonb) and read back unchanged, for more than one shape; a row with no details has a
// NULL extra_data; three successive writes and reads of the same bill keep the values.
func TestLiveInquiryExtraData(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, r := liveInquiryRepo(t, ctx)
	row := func(id, bill string, d *provider.Details) inquiry.Row {
		return inquiry.Row{ID: id, UserID: inqUser1, InquiryID: bill, ProviderID: 2, ProviderName: "tavanir", BillType: "electricity",
			InquiryType: "regular", BillID: bill, PayID: "1800012345", PaymentDeadline: "2030-01-01", Amount: 18000000, Details: d}
	}
	full := &provider.ElectricityDetails{Phase: "تک فاز", CompanyName: "co \"quoted\"", TariffType: "ta", CustomerType: -3, SerialNumber: "se",
		ServiceAddress: "line 1\nline 2 \\ end", ServicePostCode: "po", CustomerName: "na'me", City: "ci", Region: "re"}
	shapes := []struct {
		id, bill  string
		details   *provider.Details
		wantExtra string // the jsonb text; "" for NULL
	}{
		{"a1111111-1111-4111-8111-111111111111", "1000000000001", &provider.Details{Electricity: full}, "x"},
		{"a2222222-2222-4222-8222-222222222222", "1000000000002", &provider.Details{Electricity: &provider.ElectricityDetails{City: "c", Region: "r"}}, `{"city": "c", "region": "r"}`},
		{"a3333333-3333-4333-8333-333333333333", "1000000000003", &provider.Details{Electricity: &provider.ElectricityDetails{}}, `{}`},
		{"a4444444-4444-4444-8444-444444444444", "1000000000004", nil, ""},
		{"a5555555-5555-4555-8555-555555555555", "1000000000005", &provider.Details{}, ""},
	}
	for _, sh := range shapes {
		if !assert.NoError(t, r.SaveInquiries(ctx, []inquiry.Row{row(sh.id, sh.bill, sh.details)}), sh.id) {
			continue
		}
		var extra *string
		var typ *string
		if !assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT extra_data::text, jsonb_typeof(extra_data) FROM bill_inquiries WHERE id = $1`, sh.id).Scan(&extra, &typ), sh.id) {
			continue
		}
		switch sh.wantExtra {
		case "":
			assert.Nil(t, extra, sh.id+": NULL, not a JSON null")
		case "x":
			if assert.NotNil(t, typ, sh.id) {
				assert.Equal(t, "object", *typ, sh.id)
			}
		default:
			if assert.NotNil(t, extra, sh.id) {
				assert.Equal(t, sh.wantExtra, *extra, sh.id)
			}
		}
		got, err := r.FindValid(ctx, sh.bill, time.Hour, 2)
		if !assert.NoError(t, err, sh.id) || !assert.Len(t, got, 1, sh.id) {
			continue
		}
		if sh.details == nil || sh.details.Electricity == nil {
			assert.Nil(t, got[0].Details, sh.id)
		} else if assert.NotNil(t, got[0].Details, sh.id) {
			assert.Equal(t, sh.details.Electricity, got[0].Details.Electricity, sh.id+": read back unchanged")
		}
	}

	// Each key of the full row, by a direct SQL query.
	var city, customer, address string
	var customerType int
	err := c.Pool().QueryRow(ctx, `SELECT extra_data->>'city', extra_data->>'customer_name', extra_data->>'service_address', (extra_data->>'customer_type')::int
		FROM bill_inquiries WHERE id = $1`, shapes[0].id).Scan(&city, &customer, &address, &customerType)
	if assert.NoError(t, err) {
		assert.Equal(t, []any{"ci", "na'me", "line 1\nline 2 \\ end", -3}, []any{city, customer, address, customerType})
	}

	// Three successive writes of what was read (the copies of a cache hit): the values stay the same.
	prev, err := r.FindValid(ctx, shapes[0].bill, time.Hour, 2)
	if !assert.NoError(t, err) || !assert.Len(t, prev, 1) {
		return
	}
	for i, id := range []string{"b1111111-1111-4111-8111-111111111111", "b2222222-2222-4222-8222-222222222222", "b3333333-3333-4333-8333-333333333333"} {
		cp := prev[0]
		cp.ID, cp.UserID, cp.IsCached = id, inqUser2, true
		if !assert.NoError(t, r.SaveInquiries(ctx, []inquiry.Row{cp}), i) {
			return
		}
		var same bool
		err := c.Pool().QueryRow(ctx, `SELECT c.extra_data = o.extra_data FROM bill_inquiries c, bill_inquiries o WHERE c.id = $1 AND o.id = $2`, id, shapes[0].id).Scan(&same)
		assert.NoError(t, err, i)
		assert.True(t, same, i)
		prev, err = r.FindValid(ctx, shapes[0].bill, time.Hour, 2)
		if !assert.NoError(t, err, i) || !assert.Len(t, prev, 1, "copies do not answer") || !assert.NotNil(t, prev[0].Details) {
			return
		}
		assert.Equal(t, full, prev[0].Details.Electricity, i)
	}

	// A water row keeps a NULL extra_data, and extra_data of another writer does not become details.
	_, err = c.Pool().Exec(ctx, `UPDATE bill_inquiries SET bill_type = 'water' WHERE id = $1`, shapes[1].id)
	assert.NoError(t, err)
	got, err := r.FindValid(ctx, shapes[1].bill, time.Hour, 2)
	if assert.NoError(t, err) && assert.Len(t, got, 1) {
		assert.Nil(t, got[0].Details)
	}
	// An extra_data that is not an object is an error of the read.
	_, err = c.Pool().Exec(ctx, `UPDATE bill_inquiries SET extra_data = '"text"'::jsonb WHERE id = $1`, shapes[2].id)
	assert.NoError(t, err)
	_, err = r.FindValid(ctx, shapes[2].bill, time.Hour, 2)
	assert.ErrorContains(t, err, "extra_data is a JSON string, not an object")
	// A JSON null is no details.
	_, err = c.Pool().Exec(ctx, `UPDATE bill_inquiries SET extra_data = 'null'::jsonb WHERE id = $1`, shapes[2].id)
	assert.NoError(t, err)
	got, err = r.FindValid(ctx, shapes[2].bill, time.Hour, 2)
	if assert.NoError(t, err) && assert.Len(t, got, 1) {
		assert.Nil(t, got[0].Details)
	}
}
