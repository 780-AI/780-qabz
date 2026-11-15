package postgres

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"

	"qabz/core/order"
)

// insertOrderSQL adds an orders row (bill: order.Repository.NewOrder, go-pg Insert). As go-pg, a
// zero value is stored as NULL: an empty text, a 0, false and a nil config or fee ID. created_at and
// updated_at get their column defaults (bill sets the zero value, so go-pg writes the default). A
// status change of the payment step (task 044) is a new row with the same order_id and the columns
// rrn, ref_id, message and status_code.
const insertOrderSQL = `INSERT INTO orders (uuid, order_id, user_id, bill_type, bill_id, pay_id, option, price, data,
	provider_id, provider_name, status, fee, fee_type, fee_config, fee_id, base_price, is_raw, provider_fee, bill_inquiry_id,
	rrn, ref_id, message, status_code, provider_confirmed)
VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
	NULLIF($8::bigint, 0), NULLIF($9, ''), NULLIF($10::bigint, 0), NULLIF($11, ''), NULLIF($12, ''),
	NULLIF($13::bigint, 0), NULLIF($14, ''), NULLIF($15::text, '')::jsonb, $16::integer, NULLIF($17::bigint, 0),
	NULLIF($18::boolean, false), NULLIF($19::bigint, 0), NULLIF($20, ''),
	NULLIF($21, ''), NULLIF($22, ''), NULLIF($23, ''), NULLIF($24::integer, 0), NULLIF($25::boolean, false))`

// inquiryFeeSQL reads what the provider fee needs of a bill_inquiries row (bill:
// bill_inquiry.Repository.GetById): the provider, the bill type and extra_data.service_status (a
// landline row, task 044; a status that is not a JSON string reads as "").
const inquiryFeeSQL = `SELECT provider_name, provider_id, bill_type,
	COALESCE(CASE WHEN jsonb_typeof(extra_data -> 'service_status') = 'string' THEN extra_data ->> 'service_status' END, '')
FROM bill_inquiries WHERE id = $1::uuid`

// providerFeeSQL reads providers.provider_fee (bill: provider.Repository.Get); NULL is 0.
const providerFeeSQL = `SELECT COALESCE(provider_fee, 0) FROM providers WHERE id = $1`

// providerNameSQL reads providers.name (bill: provider.Repository.Get in CompleteOrder).
const providerNameSQL = `SELECT COALESCE(name, '') FROM providers WHERE id = $1`

// lastStatusSQL and lastStatusWindowSQL read the status of the newest orders row of a bill ID and
// a pay ID (bill: GetLastStatusByBillIDPayID): with a window of whole minutes, only the rows of the
// window (bill: "created_at > NOW() - INTERVAL 'n minute'"; a window under one minute is no window).
const (
	lastStatusSQL       = `SELECT COALESCE(status, '') FROM orders WHERE bill_id = $1 AND pay_id = $2 ORDER BY created_at DESC LIMIT 1`
	lastStatusWindowSQL = `SELECT COALESCE(status, '') FROM orders WHERE bill_id = $1 AND pay_id = $2
	AND created_at > NOW() - make_interval(mins => $3::integer) ORDER BY created_at DESC LIMIT 1`
)

// orderColumns are the columns of an order that is read (NULL gives the zero value).
const orderColumns = `uuid::text, order_id::text, COALESCE(user_id::text, ''), COALESCE(bill_type, ''), COALESCE(bill_id, ''),
	COALESCE(pay_id, ''), COALESCE(option, ''), COALESCE(price, 0), COALESCE(data, ''), COALESCE(ref_id, ''),
	COALESCE(provider_id, 0), COALESCE(provider_name, ''), COALESCE(rrn, ''), COALESCE(message, ''), COALESCE(status, ''),
	COALESCE(status_code, 0), created_at, COALESCE(fee, 0), COALESCE(fee_type, ''), COALESCE(fee_config::text, ''),
	fee_id, COALESCE(base_price, 0), COALESCE(is_raw, false), COALESCE(provider_confirmed, false),
	COALESCE(provider_fee, 0), COALESCE(bill_inquiry_id, '')`

// latestOrderSQL reads the newest row of an order created in the last window (bill:
// order.Repository.Get: "order_id=? AND created_at BETWEEN now-48h AND now").
const latestOrderSQL = `SELECT ` + orderColumns + ` FROM orders
WHERE order_id = $1::uuid AND created_at BETWEEN NOW() - make_interval(secs => $2::double precision) AND NOW()
ORDER BY created_at DESC LIMIT 1`

// confirmProviderSQL sets provider_confirmed of the success row of an order created in the last
// 48 h (bill: UpdateProviderConfirm).
const confirmProviderSQL = `UPDATE orders SET provider_confirmed = true, updated_at = NOW()
WHERE order_id = $1::uuid AND created_at BETWEEN NOW() - make_interval(secs => $2::double precision) AND NOW() AND status = 'success'`

// markInquiryPaidSQL sets is_paid of the inquiry rows that are not copies (bill: UpdateIsPaid). bill
// writes ORDER BY and LIMIT 1, which go-pg does not put in an UPDATE: every matching row is set.
const markInquiryPaidSQL = `UPDATE bill_inquiries SET is_paid = true, updated_at = NOW()
WHERE inquiry_id = $1 AND pay_id = $2 AND is_cached = false`

// setLastOrderDateSQL sets the last order date of the saved bills (bill: mybill UpdateLastOrderDate:
// the columns last_order_date and payment_deadline, the zero value "" written as NULL).
const setLastOrderDateSQL = `UPDATE bills SET last_order_date = NULLIF($3, ''), payment_deadline = NULL
WHERE bill_id = $1 AND type = $2`

// OrderRepository implements order.Repository and order.PaymentRepository on the orders,
// bill_inquiries, providers and bills tables. Raw queries, no ORM.
type OrderRepository struct {
	db querier
}

var (
	_ order.Repository        = (*OrderRepository)(nil)
	_ order.PaymentRepository = (*OrderRepository)(nil)
)

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
		o.BasePrice, o.IsRaw, int64(o.ProviderFee), o.BillInquiryID, o.RRN, o.RefID, o.Message, o.StatusCode, o.ProviderConfirmed)
	return err
}

// InquiryFee reads the provider fee fields of the bill_inquiries row with the id. No row is an error
// (pgx.ErrNoRows).
func (r *OrderRepository) InquiryFee(ctx context.Context, billInquiryID string) (order.InquiryFee, error) {
	if r == nil || r.db == nil {
		return order.InquiryFee{}, errNoOrderPool
	}
	row := r.db.QueryRow(ctx, inquiryFeeSQL, billInquiryID)
	if row == nil {
		return order.InquiryFee{}, errors.New("order repository: no row from QueryRow")
	}
	var f order.InquiryFee
	if err := row.Scan(&f.ProviderName, &f.ProviderID, &f.BillType, &f.ServiceStatus); err != nil {
		return order.InquiryFee{}, err
	}
	return f, nil
}

// ProviderFee gives providers.provider_fee of the row (NULL is 0). No row is an error. A negative
// value is an error.
func (r *OrderRepository) ProviderFee(ctx context.Context, providerID int64) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, errNoOrderPool
	}
	row := r.db.QueryRow(ctx, providerFeeSQL, providerID)
	if row == nil {
		return 0, errors.New("order repository: no row from QueryRow")
	}
	var fee int64
	if err := row.Scan(&fee); err != nil {
		return 0, err
	}
	if fee < 0 {
		return 0, fmt.Errorf("order repository: provider %d has a negative provider_fee %d", providerID, fee)
	}
	return uint64(fee), nil
}

// ProviderName gives providers.name of the row. No row is an error.
func (r *OrderRepository) ProviderName(ctx context.Context, providerID int64) (string, error) {
	if r == nil || r.db == nil {
		return "", errNoOrderPool
	}
	row := r.db.QueryRow(ctx, providerNameSQL, providerID)
	if row == nil {
		return "", errors.New("order repository: no row from QueryRow")
	}
	var name string
	if err := row.Scan(&name); err != nil {
		return "", err
	}
	return name, nil
}

// LastStatus gives the status of the newest orders row with the bill ID and the pay ID in the last
// window (whole minutes, as bill; under one minute: any time). No row: "".
func (r *OrderRepository) LastStatus(ctx context.Context, billID, payID string, window time.Duration) (string, error) {
	if r == nil || r.db == nil {
		return "", errNoOrderPool
	}
	var row pgx.Row
	if minutes := int64(window / time.Minute); minutes > 0 {
		row = r.db.QueryRow(ctx, lastStatusWindowSQL, billID, payID, minutes)
	} else {
		row = r.db.QueryRow(ctx, lastStatusSQL, billID, payID)
	}
	if row == nil {
		return "", errors.New("order repository: no row from QueryRow")
	}
	var status string
	if err := row.Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return status, nil
}

// Latest gives the newest orders row of the order ID created in the last window. None (also an
// order ID that is not a UUID: bill's query fails too) wraps order.ErrNotFound for no row.
func (r *OrderRepository) Latest(ctx context.Context, orderID string, window time.Duration) (order.Order, error) {
	if r == nil || r.db == nil {
		return order.Order{}, errNoOrderPool
	}
	row := r.db.QueryRow(ctx, latestOrderSQL, orderID, window.Seconds())
	if row == nil {
		return order.Order{}, errors.New("order repository: no row from QueryRow")
	}
	var o order.Order
	var price, statusCode int64
	var feeConfig string
	var feeID *int64
	var providerFee int64
	err := row.Scan(&o.ID, &o.OrderID, &o.UserID, &o.BillType, &o.BillID, &o.PayID, &o.Option, &price, &o.Data, &o.RefID,
		&o.ProviderID, &o.ProviderName, &o.RRN, &o.Message, &o.Status, &statusCode, &o.CreatedAt, &o.Fee, &o.FeeType,
		&feeConfig, &feeID, &o.BasePrice, &o.IsRaw, &o.ProviderConfirmed, &providerFee, &o.BillInquiryID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return order.Order{}, fmt.Errorf("%w: %s", order.ErrNotFound, orderID)
		}
		return order.Order{}, err
	}
	if price < 0 || price > math.MaxUint32 {
		return order.Order{}, fmt.Errorf("order repository: order %s has the price %d, out of the uint32 range", orderID, price)
	}
	if statusCode < math.MinInt32 || statusCode > math.MaxInt32 || providerFee < 0 {
		return order.Order{}, fmt.Errorf("order repository: order %s has a status_code or provider_fee out of range", orderID)
	}
	o.Price, o.StatusCode, o.ProviderFee, o.FeeID = uint32(price), int32(statusCode), uint64(providerFee), feeID
	if feeConfig != "" {
		o.FeeConfig = []byte(feeConfig)
	}
	return o, nil
}

// ConfirmProvider sets provider_confirmed of the success row of the order (created in the last
// order.LatestWindow). No such row is not an error (bill).
func (r *OrderRepository) ConfirmProvider(ctx context.Context, orderID string) error {
	if r == nil || r.db == nil {
		return errNoOrderPool
	}
	_, err := r.db.Exec(ctx, confirmProviderSQL, orderID, order.LatestWindow.Seconds())
	return err
}

// MarkInquiryPaid sets is_paid of the bill_inquiries rows of the inquiry ID and the pay ID that are
// not copies.
func (r *OrderRepository) MarkInquiryPaid(ctx context.Context, inquiryID, payID string) error {
	if r == nil || r.db == nil {
		return errNoOrderPool
	}
	_, err := r.db.Exec(ctx, markInquiryPaidSQL, inquiryID, payID)
	return err
}

// SetLastOrderDate sets last_order_date (and a NULL payment_deadline) of the saved bills of the
// bill ID and the bill type.
func (r *OrderRepository) SetLastOrderDate(ctx context.Context, billID, billType, date string) error {
	if r == nil || r.db == nil {
		return errNoOrderPool
	}
	_, err := r.db.Exec(ctx, setLastOrderDateSQL, billID, billType, date)
	return err
}
