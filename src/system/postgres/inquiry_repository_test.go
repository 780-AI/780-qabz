package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"qabz/core/inquiry"
	"qabz/provider"
)

// inqDB is a querier for the inquiry repository: Query gives rows, Exec gives tag and execErr. It
// records the calls.
type inqDB struct {
	rows     pgx.Rows
	queryErr error
	tag      pgconn.CommandTag
	execErr  error

	querySQL  string
	queryArgs []any
	execSQL   string
	execArgs  []any
	execs     int
}

func (f *inqDB) Query(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
	f.querySQL, f.queryArgs = sql, args
	return f.rows, f.queryErr
}

func (f *inqDB) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func (f *inqDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execs++
	f.execSQL, f.execArgs = sql, args
	return f.tag, f.execErr
}

// inqRows gives rows of 13 columns (the findValidInquiriesSQL columns). Scan of row scanErrAt
// (1-based) fails with scanErr; Err answers err.
type inqRows struct {
	pgx.Rows
	data      [][13]any
	next      int
	scanErrAt int
	scanErr   error
	err       error
	closed    bool
}

func (r *inqRows) Close()     { r.closed = true }
func (r *inqRows) Err() error { return r.err }
func (r *inqRows) Next() bool {
	if r.next >= len(r.data) {
		return false
	}
	r.next++
	return true
}

func (r *inqRows) Scan(dest ...any) error {
	if r.scanErrAt == r.next {
		return r.scanErr
	}
	if len(dest) != 13 {
		return errors.New("bad Scan call")
	}
	for i, d := range dest {
		switch p := d.(type) {
		case *string:
			*p = r.data[r.next-1][i].(string)
		case *int64:
			*p = r.data[r.next-1][i].(int64)
		default:
			return errors.New("bad Scan destination")
		}
	}
	return nil
}

func newInqRepo(db querier) *InquiryRepository { return &InquiryRepository{db: db} }

func TestInquiryRepositoryNoPool(t *testing.T) {
	ctx := context.Background()
	for name, r := range map[string]*InquiryRepository{
		"nil": nil, "zero": {}, "nil client": NewInquiryRepository(nil), "client without pool": NewInquiryRepository(&Client{}),
	} {
		_, err := r.FindValid(ctx, "1", time.Hour, 1)
		assert.Error(t, err, name)
		assert.Error(t, r.SaveInquiries(ctx, []inquiry.Row{{}}), name)
		assert.Error(t, r.CreateRequest(ctx, inquiry.RequestRow{}), name)
		assert.Error(t, r.FinishRequest(ctx, "1", "s", ""), name)
	}
}

func TestFindValidMapsRowsAndArgs(t *testing.T) {
	db := &inqDB{rows: &inqRows{data: [][13]any{
		{"id1", "u1", "bill1", int64(36), "qabzino", "water", "tid1", "regular", "b1", "p1", "2030-01-01", int64(25000000), ""},
		{"id2", "u2", "bill1", int64(36), "qabzino", "water", "", "", "", "", "", int64(0), `{"city":"ignored for water"}`},
	}}}
	got, err := newInqRepo(db).FindValid(context.Background(), "bill1", 90*time.Minute, 36)
	if !assert.NoError(t, err) || !assert.Len(t, got, 2) {
		return
	}
	assert.Equal(t, inquiry.Row{ID: "id1", UserID: "u1", InquiryID: "bill1", ProviderID: 36, ProviderName: "qabzino", BillType: "water",
		ProviderTraceID: "tid1", InquiryType: "regular", BillID: "b1", PayID: "p1", PaymentDeadline: "2030-01-01", Amount: 25000000}, got[0])
	assert.Equal(t, uint64(0), got[1].Amount)
	assert.Nil(t, got[1].Details, "a water row has no details, whatever extra_data holds")
	assert.Contains(t, db.querySQL, "COALESCE(extra_data::text, '')")
	assert.Equal(t, []any{"bill1", 5400.0, int64(36)}, db.queryArgs, "the ttl goes in seconds")
	assert.True(t, db.rows.(*inqRows).closed)
	for _, must := range []string{"is_cached = false", "is_paid = false", "provider_id = $3", "inquiry_id = $1", "make_interval", "LIMIT 2", "created_at DESC"} {
		assert.Contains(t, db.querySQL, must)
	}
	assert.NotContains(t, db.querySQL, "user_id =", "the cache is shared between users")
}

func TestFindValidNoRowIsEmpty(t *testing.T) {
	got, err := newInqRepo(&inqDB{rows: &inqRows{}}).FindValid(context.Background(), "b", time.Hour, 1)
	assert.NoError(t, err)
	assert.Empty(t, got)
}

func TestFindValidErrors(t *testing.T) {
	boom := errors.New("boom")
	ctx := context.Background()
	_, err := newInqRepo(&inqDB{queryErr: boom}).FindValid(ctx, "b", time.Hour, 1)
	assert.ErrorIs(t, err, boom)
	_, err = newInqRepo(&inqDB{}).FindValid(ctx, "b", time.Hour, 1) // nil rows, nil error
	assert.Error(t, err)
	row := [13]any{"i", "u", "b", int64(1), "p", "w", "", "", "", "", "", int64(1), ""}
	_, err = newInqRepo(&inqDB{rows: &inqRows{data: [][13]any{row}, scanErrAt: 1, scanErr: boom}}).FindValid(ctx, "b", time.Hour, 1)
	assert.ErrorIs(t, err, boom)
	_, err = newInqRepo(&inqDB{rows: &inqRows{data: [][13]any{row}, err: boom}}).FindValid(ctx, "b", time.Hour, 1)
	assert.ErrorIs(t, err, boom)
	neg := row
	neg[11] = int64(-1)
	_, err = newInqRepo(&inqDB{rows: &inqRows{data: [][13]any{neg}}}).FindValid(ctx, "b", time.Hour, 1)
	assert.ErrorContains(t, err, "negative amount")
	bad := row
	bad[5], bad[12] = "electricity", `["not an object"]`
	_, err = newInqRepo(&inqDB{rows: &inqRows{data: [][13]any{bad}}}).FindValid(ctx, "b", time.Hour, 1)
	assert.ErrorContains(t, err, "inquiry i: extra_data is a JSON array, not an object", "the flow then asks the provider")
}

// TestFindValidReadsElectricityDetails: extra_data of an electricity row becomes the details of
// the row (task 022); a NULL extra_data gives no details.
func TestFindValidReadsElectricityDetails(t *testing.T) {
	db := &inqDB{rows: &inqRows{data: [][13]any{
		{"id1", "u1", "bill1", int64(2), "tavanir", "electricity", "", "regular", "b1", "p1", "2030-01-01", int64(18000000),
			`{"city": "c", "region": "r", "phase": "ph", "company_name": "co", "tariff_type": "ta", "customer_type": 3, "serial_number": "se", "service_address": "ad", "service_post_code": "po", "customer_name": "na"}`},
		{"id2", "u1", "bill1", int64(2), "tavanir", "electricity", "", "regular", "b1", "p1", "2030-01-01", int64(1), ""},
	}}}
	got, err := newInqRepo(db).FindValid(context.Background(), "bill1", time.Hour, 2)
	if !assert.NoError(t, err) || !assert.Len(t, got, 2) || !assert.NotNil(t, got[0].Details) {
		return
	}
	assert.Equal(t, &provider.ElectricityDetails{Phase: "ph", CompanyName: "co", TariffType: "ta", CustomerType: 3, SerialNumber: "se",
		ServiceAddress: "ad", ServicePostCode: "po", CustomerName: "na", City: "c", Region: "r"}, got[0].Details.Electricity)
	assert.Nil(t, got[1].Details)
}

func TestInsertInquiriesSQL(t *testing.T) {
	one := insertInquiriesSQL(1)
	assert.Equal(t, 1, strings.Count(one, "VALUES "))
	assert.Contains(t, one, "INSERT INTO bill_inquiries (id, user_id, inquiry_id, provider_id, provider_name, bill_type, tid, inquiry_type, bill_id, pay_id, payment_deadline, amount, is_cached, extra_data, is_paid)")
	assert.Contains(t, one, "($1, $2, $3, $4, $5, $6, NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, ''), $12, $13, NULLIF($14::text, '')::jsonb, false)")
	two := insertInquiriesSQL(2)
	assert.Contains(t, two, "), ($15, $16, $17")
	assert.Contains(t, two, "$26, $27, NULLIF($28::text, '')::jsonb, false)")
	assert.NotContains(t, two, "$29")
}

func TestSaveInquiries(t *testing.T) {
	db := &inqDB{tag: pgconn.NewCommandTag("INSERT 0 2")}
	r := newInqRepo(db)
	rows := []inquiry.Row{
		{ID: "a", UserID: "u", InquiryID: "b", ProviderID: 36, ProviderName: "qabzino", BillType: "water", ProviderTraceID: "t", InquiryType: "regular", BillID: "bb", PayID: "p", PaymentDeadline: "2030-01-01", Amount: 7},
		{ID: "c", UserID: "u", InquiryID: "b", ProviderID: 36, ProviderName: "qabzino", BillType: "water", IsCached: true},
	}
	assert.NoError(t, r.SaveInquiries(context.Background(), rows))
	assert.Equal(t, 1, db.execs, "one statement: all or none")
	if !assert.Len(t, db.execArgs, 28) {
		return
	}
	assert.Equal(t, []any{"a", "u", "b", int64(36), "qabzino", "water", "t", "regular", "bb", "p", "2030-01-01", int64(7), false, ""}, db.execArgs[:14],
		"no details: extra_data is \"\" (NULL)")
	assert.Equal(t, true, db.execArgs[26])
	assert.Equal(t, int64(0), db.execArgs[25])

	// Details go to extra_data as JSON text (task 022).
	withDetails := []inquiry.Row{{ID: "e", BillType: "electricity", Details: &provider.Details{Electricity: &provider.ElectricityDetails{City: "c", Region: "r", CustomerType: 2}}}}
	assert.NoError(t, r.SaveInquiries(context.Background(), withDetails))
	if assert.Len(t, db.execArgs, 14) {
		assert.Equal(t, `{"city":"c","region":"r","customer_type":2}`, db.execArgs[13])
	}

	db.execs = 0
	assert.NoError(t, r.SaveInquiries(context.Background(), nil))
	assert.Equal(t, 0, db.execs, "no row, no statement")

	boom := errors.New("boom")
	db.execErr = boom
	assert.ErrorIs(t, r.SaveInquiries(context.Background(), rows), boom)

	db.execErr = nil
	big := []inquiry.Row{{ID: "x", Amount: 1 << 63}}
	assert.ErrorContains(t, r.SaveInquiries(context.Background(), big), "bigint")
}

func TestCreateAndFinishRequest(t *testing.T) {
	db := &inqDB{tag: pgconn.NewCommandTag("INSERT 0 1")}
	r := newInqRepo(db)
	q := inquiry.RequestRow{ID: "i", UserID: "u", Dialer: "d", InquiryID: "b", BillType: "water", TID: "t", ProviderName: "qabzino", Status: "initial"}
	assert.NoError(t, r.CreateRequest(context.Background(), q))
	assert.Equal(t, []any{"i", "u", "d", "b", "water", "t", "qabzino", "", "initial"}, db.execArgs)
	assert.Contains(t, db.execSQL, "NULLIF($8, '')")
	boom := errors.New("boom")
	db.execErr = boom
	assert.ErrorIs(t, r.CreateRequest(context.Background(), q), boom)

	db = &inqDB{tag: pgconn.NewCommandTag("UPDATE 1")}
	r = newInqRepo(db)
	assert.NoError(t, r.FinishRequest(context.Background(), "i", "success", "trace"))
	assert.Equal(t, []any{"i", "success", "trace"}, db.execArgs)
	db.tag = pgconn.NewCommandTag("UPDATE 0")
	assert.ErrorContains(t, r.FinishRequest(context.Background(), "i", "success", ""), "0 rows updated")
	db.execErr = boom
	assert.ErrorIs(t, r.FinishRequest(context.Background(), "i", "success", ""), boom)
}
