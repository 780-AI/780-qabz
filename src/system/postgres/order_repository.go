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
// zero value is stored as NULL: an empty text, a 0 and a nil config or fee ID. created_at and
// updated_at get their column defaults (bill sets the zero value, so go-pg writes the default). A
// status change of the payment step (task 044) is a new row with the same order_id and the columns
// rrn, ref_id, message and status_code.
//
// @todo docs/this_vs_bill.md item 45: is_raw and provider_confirmed are written as they are, false
// too, never NULL (task 046, operator 2026-11-15: "No NULL, if bill saves FALSE, then should
// Qabz"). bill writes DEFAULT for a false (go-pg): is_raw gets FALSE (bill migration 21), the same;
// provider_confirmed has no default, so bill stores NULL and its settlement query finds no order.
const insertOrderSQL = `INSERT INTO orders (uuid, order_id, user_id, bill_type, bill_id, pay_id, option, price, data,
	provider_id, provider_name, status, fee, fee_type, fee_config, fee_id, base_price, is_raw, provider_fee, bill_inquiry_id,
	rrn, ref_id, message, status_code, provider_confirmed)
VALUES ($1::uuid, $2::uuid, NULLIF($3, '')::uuid, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''),
	NULLIF($8::bigint, 0), NULLIF($9, ''), NULLIF($10::bigint, 0), NULLIF($11, ''), NULLIF($12, ''),
	NULLIF($13::bigint, 0), NULLIF($14, ''), NULLIF($15::text, '')::jsonb, $16::integer, NULLIF($17::bigint, 0),
	$18::boolean, NULLIF($19::bigint, 0), NULLIF($20, ''),
	NULLIF($21, ''), NULLIF($22, ''), NULLIF($23, ''), NULLIF($24::integer, 0), $25::boolean)`

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

// pendingIDsSQL reads one page of the order IDs whose newest row in the window has the status
// pending or in-progress, for one provider (task 045; bill: order.Repository.GetAllPending, the same
// query). The window filters the rows before the newest row is taken, as bill: a final row newer
// than the window keeps the order in the list (task 045 C6). The window is in seconds before NOW()
// (the clock of the database, as latestOrderSQL).
const pendingIDsSQL = `WITH ranked AS (
  SELECT order_id, status, created_at,
    ROW_NUMBER() OVER (PARTITION BY order_id ORDER BY created_at DESC) AS rn
  FROM orders
  WHERE provider_name = $1
    AND created_at >= NOW() - make_interval(secs => $2::double precision)
    AND created_at <= NOW() - make_interval(secs => $3::double precision)
)
SELECT order_id::text FROM ranked
WHERE rn = 1 AND status IN ('pending', 'in-progress')
ORDER BY created_at ASC
LIMIT $4 OFFSET $5`

// nilUUID is the order_id before every order (the first page of ConfirmCandidates).
const nilUUID = "00000000-0000-0000-0000-000000000000"

// confirmCandidatesSQL is bill's GetOrdersForConfirmPayment query (the filters are on each row
// before DISTINCT ON, so it gives the newest success row of each order), with a keyset on order_id
// in place of bill's repeated first page (task 046, design D3).
const confirmCandidatesSQL = `SELECT DISTINCT ON (order_id) ` + orderColumns + ` FROM orders
WHERE created_at > $2 AND created_at < $3
  AND bill_type = $1
  AND status = 'success'
  AND is_raw IS false
  AND provider_confirmed IS false
  AND order_id > $4::uuid
ORDER BY order_id, created_at DESC
LIMIT $5`

// lockLatestSQL reads the status of the newest row of an order of the last 24 h and locks it, in
// the transaction of InsertFinal (bill: NewOrderWithFinalStatus, SELECT ... FOR UPDATE).
const lockLatestSQL = `SELECT COALESCE(status, '') FROM orders
WHERE order_id = $1::uuid AND created_at BETWEEN NOW() - INTERVAL '24 hours' AND NOW()
ORDER BY created_at DESC LIMIT 1 FOR UPDATE`

// ErrAlreadyFinal is the error of InsertFinal when the newest row of the order is success or
// failed (bill: "order is already in final status"). Test it with errors.Is.
var ErrAlreadyFinal = errors.New("order repository: order is already in final status")

// beginner starts a transaction (*pgxpool.Pool).
type beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// OrderRepository implements order.Repository and order.PaymentRepository on the orders,
// bill_inquiries, providers and bills tables, and the repository of the status worker (task 045).
// Raw queries, no ORM.
type OrderRepository struct {
	db   querier
	pool beginner // InsertFinal (task 045)
}

var (
	_ order.Repository        = (*OrderRepository)(nil)
	_ order.PaymentRepository = (*OrderRepository)(nil)
)

// NewOrderRepository returns a repository that uses the client's pool.
func NewOrderRepository(c *Client) *OrderRepository {
	r := &OrderRepository{}
	if c != nil && c.pool != nil {
		r.db, r.pool = c.pool, c.pool
	}
	return r
}

var errNoOrderPool = errors.New("order repository: no database pool")

// Insert adds the orders row of o.
func (r *OrderRepository) Insert(ctx context.Context, o order.Order) error {
	if r == nil || r.db == nil {
		return errNoOrderPool
	}
	return insertOrder(ctx, r.db, o)
}

// insertOrder runs insertOrderSQL for o on q (the pool or a transaction).
func insertOrder(ctx context.Context, q querier, o order.Order) error {
	if o.ProviderFee > math.MaxInt64 {
		return errors.New("order repository: the provider fee does not fit bigint")
	}
	_, err := q.Exec(ctx, insertOrderSQL, o.ID, o.OrderID, o.UserID, o.BillType, o.BillID, o.PayID, o.Option,
		int64(o.Price), o.Data, o.ProviderID, o.ProviderName, o.Status, o.Fee, o.FeeType, string(o.FeeConfig), o.FeeID,
		o.BasePrice, o.IsRaw, int64(o.ProviderFee), o.BillInquiryID, o.RRN, o.RefID, o.Message, o.StatusCode, o.ProviderConfirmed)
	return err
}

// PendingIDs gives one page (limit, offset) of the order IDs of the provider whose newest row
// created from now-end to now-start has the status pending or in-progress, the oldest first (task
// 045; bill: GetAllPending).
func (r *OrderRepository) PendingIDs(ctx context.Context, providerName string, limit, offset int, start, end time.Duration) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, errNoOrderPool
	}
	rows, err := r.db.Query(ctx, pendingIDsSQL, providerName, end.Seconds(), start.Seconds(), limit, offset)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("order repository: no rows from Query")
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// InsertFinal adds o, a row with a final status, in one transaction (task 045; bill:
// NewOrderWithFinalStatus): it locks the newest row of the order of the last 24 h; no row is an
// error (pgx.ErrNoRows), a final status there is ErrAlreadyFinal, and nothing is saved.
func (r *OrderRepository) InsertFinal(ctx context.Context, o order.Order) (err error) {
	if r == nil || r.pool == nil {
		return errNoOrderPool
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	if tx == nil {
		return errors.New("order repository: no transaction from Begin")
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	row := tx.QueryRow(ctx, lockLatestSQL, o.OrderID)
	if row == nil {
		err = errors.New("order repository: no row from QueryRow")
		return err
	}
	var status string
	if err = row.Scan(&status); err != nil {
		return err
	}
	if status == order.StatusSuccess || status == order.StatusFailed {
		return fmt.Errorf("%w: %s", ErrAlreadyFinal, o.OrderID)
	}
	if err = insertOrder(ctx, tx, o); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
	o, err := scanOrder(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return order.Order{}, fmt.Errorf("%w: %s", order.ErrNotFound, orderID)
	}
	return o, err
}

// rowScanner is a pgx.Row or pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanOrder reads one row of orderColumns.
func scanOrder(row rowScanner) (order.Order, error) {
	var o order.Order
	var price, statusCode int64
	var feeConfig string
	var feeID *int64
	var providerFee int64
	err := row.Scan(&o.ID, &o.OrderID, &o.UserID, &o.BillType, &o.BillID, &o.PayID, &o.Option, &price, &o.Data, &o.RefID,
		&o.ProviderID, &o.ProviderName, &o.RRN, &o.Message, &o.Status, &statusCode, &o.CreatedAt, &o.Fee, &o.FeeType,
		&feeConfig, &feeID, &o.BasePrice, &o.IsRaw, &o.ProviderConfirmed, &providerFee, &o.BillInquiryID)
	if err != nil {
		return order.Order{}, err
	}
	if price < 0 || price > math.MaxUint32 {
		return order.Order{}, fmt.Errorf("order repository: order %s has the price %d, out of the uint32 range", o.OrderID, price)
	}
	if statusCode < math.MinInt32 || statusCode > math.MaxInt32 || providerFee < 0 {
		return order.Order{}, fmt.Errorf("order repository: order %s has a status_code or provider_fee out of range", o.OrderID)
	}
	o.Price, o.StatusCode, o.ProviderFee, o.FeeID = uint32(price), int32(statusCode), uint64(providerFee), feeID
	if feeConfig != "" {
		o.FeeConfig = []byte(feeConfig)
	}
	return o, nil
}

// ConfirmCandidates gives one page of the paid orders of the bill type that the provider did not
// confirm (task 046; bill: GetOrdersForConfirmPayment): the newest success row of each order
// created after from and before to (strict, as bill), not raw, provider_confirmed false, with an
// order_id greater than after (keyset paging; "" is the first page), in order_id order.
//
// @todo docs/this_vs_bill.md item 45: bill reads the same query again with no offset, so a failed
// settlement is read again at once and its run never ends; Qabz reads each order once per run.
func (r *OrderRepository) ConfirmCandidates(ctx context.Context, billType, after string, limit int, from, to time.Time) ([]order.Order, error) {
	if r == nil || r.db == nil {
		return nil, errNoOrderPool
	}
	if after == "" {
		after = nilUUID
	}
	rows, err := r.db.Query(ctx, confirmCandidatesSQL, billType, from, to, after, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("order repository: no rows from Query")
	}
	defer rows.Close()
	var out []order.Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
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
