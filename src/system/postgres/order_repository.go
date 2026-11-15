package postgres

import (
	"context"
	"errors"
	"math"

	"qabz/core/order"
)

// insertOrderSQL adds an orders row (bill: order.Repository.NewOrder, go-pg Insert). As go-pg, a
// zero value is stored as NULL: an empty text, a 0, false and a nil config or fee ID. The columns
// that the order does not set (ref_id, rrn, message, status_code, provider_confirmed) are NULL;
// created_at and updated_at get their column defaults.
const insertOrderSQL = `INSERT INTO orders (uuid, order_id, user_id, bill_type, bill_id, pay_id, option, price, data,
	provider_id, provider_name, status, fee, fee_type, fee_config, fee_id, base_price, is_raw, provider_fee, bill_inquiry_id)
VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
	NULLIF($8::bigint, 0), NULLIF($9, ''), NULLIF($10::bigint, 0), NULLIF($11, ''), NULLIF($12, ''),
	NULLIF($13::bigint, 0), NULLIF($14, ''), NULLIF($15::text, '')::jsonb, $16::integer, NULLIF($17::bigint, 0),
	NULLIF($18::boolean, false), NULLIF($19::bigint, 0), NULLIF($20, ''))`

// inquiryProviderSQL reads the provider and the bill type of a bill_inquiries row (bill:
// bill_inquiry.Repository.GetById, for the provider fee).
const inquiryProviderSQL = `SELECT provider_name, bill_type FROM bill_inquiries WHERE id = $1::uuid`

// OrderRepository implements order.Repository on the orders and bill_inquiries tables. Raw
// queries, no ORM.
type OrderRepository struct {
	db querier
}

var _ order.Repository = (*OrderRepository)(nil)

// NewOrderRepository returns a repository that uses the client's pool.
func NewOrderRepository(c *Client) *OrderRepository {
	r := &OrderRepository{}
	if c != nil && c.pool != nil {
		r.db = c.pool
	}
	return r
}

var errNoOrderPool = errors.New("order repository: no database pool")

// Insert adds the orders row of o.
func (r *OrderRepository) Insert(ctx context.Context, o order.Order) error {
	if r == nil || r.db == nil {
		return errNoOrderPool
	}
	if o.ProviderFee > math.MaxInt64 {
		return errors.New("order repository: the provider fee does not fit bigint")
	}
	_, err := r.db.Exec(ctx, insertOrderSQL, o.ID, o.OrderID, o.UserID, o.BillType, o.BillID, o.PayID, o.Option,
		int64(o.Price), o.Data, o.ProviderID, o.ProviderName, o.Status, o.Fee, o.FeeType, string(o.FeeConfig), o.FeeID,
		o.BasePrice, o.IsRaw, int64(o.ProviderFee), o.BillInquiryID)
	return err
}

// InquiryProvider gives provider_name and bill_type of the bill_inquiries row with the id. No row
// is an error (pgx.ErrNoRows).
func (r *OrderRepository) InquiryProvider(ctx context.Context, billInquiryID string) (string, string, error) {
	if r == nil || r.db == nil {
		return "", "", errNoOrderPool
	}
	row := r.db.QueryRow(ctx, inquiryProviderSQL, billInquiryID)
	if row == nil {
		return "", "", errors.New("order repository: no row from QueryRow")
	}
	var name, billType string
	if err := row.Scan(&name, &billType); err != nil {
		return "", "", err
	}
	return name, billType, nil
}
