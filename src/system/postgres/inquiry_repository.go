package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"qabz/core/inquiry"
)

// findValidInquiriesSQL reads the saved inquiries that can answer a bill again (bill:
// GetValidInquiryByInquiryID): the same inquiry_id and provider_id, newer than the ttl, not a copy
// (is_cached = false) and not paid (is_paid = false); the rows of every user count. The
// interval is built from the seconds of $2 (bill builds it as SQL text). A NULL column reads as
// "" (amount 0).
// @todo bill orders by created_at DESC only; rows saved in one transaction have the same
// created_at, so their order is not fixed. Qabz adds id DESC. See docs/this_vs_bill.md, item 21.
const findValidInquiriesSQL = `SELECT id::text, user_id::text, inquiry_id, provider_id, provider_name, bill_type,
	COALESCE(tid, ''), COALESCE(inquiry_type, ''), COALESCE(bill_id, ''), COALESCE(pay_id, ''),
	COALESCE(payment_deadline, ''), COALESCE(amount, 0)
FROM bill_inquiries
WHERE inquiry_id = $1 AND created_at > NOW() - make_interval(secs => $2::float8)
	AND is_cached = false AND provider_id = $3 AND is_paid = false
ORDER BY created_at DESC, id DESC
LIMIT 2`

// insertInquiryColumns are the columns that SaveInquiries writes, in the order of its arguments.
// An empty string is stored as NULL, as bill (go-pg) does. extra_data stays NULL (the Qabzino
// water answer has no details). created_at and updated_at get their column defaults.
var insertInquiryColumns = []struct {
	name string
	cast string // wraps the placeholder: "" (as is) or NULLIF(..., '')
}{
	{"id", ""}, {"user_id", ""}, {"inquiry_id", ""}, {"provider_id", ""}, {"provider_name", ""},
	{"bill_type", ""}, {"tid", "nullif"}, {"inquiry_type", "nullif"}, {"bill_id", "nullif"}, {"pay_id", "nullif"},
	{"payment_deadline", "nullif"}, {"amount", ""}, {"is_cached", ""},
}

// insertInquiriesSQL builds one INSERT with a row of placeholders for each of n inquiries, so the
// rows are written together: all or none. is_paid is written as false (bill writes the zero value).
func insertInquiriesSQL(n int) string {
	var b strings.Builder
	b.WriteString("INSERT INTO bill_inquiries (")
	for i, c := range insertInquiryColumns {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(c.name)
	}
	b.WriteString(", is_paid) VALUES ")
	arg := 0
	for r := 0; r < n; r++ {
		if r > 0 {
			b.WriteString(", ")
		}
		b.WriteString("(")
		for i, c := range insertInquiryColumns {
			if i > 0 {
				b.WriteString(", ")
			}
			arg++
			if c.cast == "nullif" {
				b.WriteString("NULLIF($" + strconv.Itoa(arg) + ", '')")
			} else {
				b.WriteString("$" + strconv.Itoa(arg))
			}
		}
		b.WriteString(", false)")
	}
	return b.String()
}

// createInquiryRequestSQL adds a request row (bill: CreateBillInquiryRequest); created_at gets its
// column default.
const createInquiryRequestSQL = `INSERT INTO bill_inquiry_requests (id, user_id, dialer, inquiry_id, bill_type, tid, provider_name, provider_trace_id, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9)`

// finishInquiryRequestSQL sets the final status and the provider trace ID of a request row.
// @todo bill writes a second row for the final status (the same tid, a new id; the table has
// UNIQUE (tid, status)). Qabz updates the first row. See docs/this_vs_bill.md, item 19.
const finishInquiryRequestSQL = `UPDATE bill_inquiry_requests SET status = $2, provider_trace_id = NULLIF($3, '') WHERE id = $1`

// InquiryRepository implements inquiry.Repository on the bill_inquiries and bill_inquiry_requests
// tables. Raw queries, no ORM.
type InquiryRepository struct{ db querier }

var _ inquiry.Repository = (*InquiryRepository)(nil)

// NewInquiryRepository returns a repository that uses the client's pool.
func NewInquiryRepository(c *Client) *InquiryRepository {
	r := &InquiryRepository{}
	if c != nil && c.pool != nil {
		r.db = c.pool
	}
	return r
}

var errNoInquiryPool = errors.New("inquiry repository: no database pool")

// FindValid returns the saved inquiries that can answer the bill again (see findValidInquiriesSQL).
func (r *InquiryRepository) FindValid(ctx context.Context, inquiryID string, ttl time.Duration, providerID int64) ([]inquiry.Row, error) {
	if r == nil || r.db == nil {
		return nil, errNoInquiryPool
	}
	rows, err := r.db.Query(ctx, findValidInquiriesSQL, inquiryID, float64(ttl)/float64(time.Second), providerID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("inquiry repository: no rows from Query")
	}
	defer rows.Close()
	var out []inquiry.Row
	for rows.Next() {
		var x inquiry.Row
		var amount int64
		if err := rows.Scan(&x.ID, &x.UserID, &x.InquiryID, &x.ProviderID, &x.ProviderName, &x.BillType,
			&x.ProviderTraceID, &x.InquiryType, &x.BillID, &x.PayID, &x.PaymentDeadline, &amount); err != nil {
			return nil, err
		}
		if amount < 0 {
			return nil, fmt.Errorf("inquiry repository: inquiry %s has a negative amount", x.ID)
		}
		x.Amount = uint64(amount)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveInquiries adds the rows with one INSERT: all or none. No row is not an error.
func (r *InquiryRepository) SaveInquiries(ctx context.Context, rows []inquiry.Row) error {
	if r == nil || r.db == nil {
		return errNoInquiryPool
	}
	if len(rows) == 0 {
		return nil
	}
	args := make([]any, 0, len(rows)*len(insertInquiryColumns))
	for _, x := range rows {
		if x.Amount > math.MaxInt64 {
			return fmt.Errorf("inquiry repository: inquiry %s has an amount that does not fit bigint", x.ID)
		}
		args = append(args, x.ID, x.UserID, x.InquiryID, x.ProviderID, x.ProviderName, x.BillType, x.ProviderTraceID,
			x.InquiryType, x.BillID, x.PayID, x.PaymentDeadline, int64(x.Amount), x.IsCached)
	}
	_, err := r.db.Exec(ctx, insertInquiriesSQL(len(rows)), args...)
	return err
}

// CreateRequest adds a request row.
func (r *InquiryRepository) CreateRequest(ctx context.Context, q inquiry.RequestRow) error {
	if r == nil || r.db == nil {
		return errNoInquiryPool
	}
	_, err := r.db.Exec(ctx, createInquiryRequestSQL, q.ID, q.UserID, q.Dialer, q.InquiryID, q.BillType, q.TID,
		q.ProviderName, q.ProviderTraceID, q.Status)
	return err
}

// FinishRequest sets the status and the provider trace ID of the request row with the id. A row
// that does not exist is an error.
func (r *InquiryRepository) FinishRequest(ctx context.Context, id, status, providerTraceID string) error {
	if r == nil || r.db == nil {
		return errNoInquiryPool
	}
	tag, err := r.db.Exec(ctx, finishInquiryRequestSQL, id, status, providerTraceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("inquiry repository: request %s: %d rows updated, want 1", id, tag.RowsAffected())
	}
	return nil
}
