package postgres

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"qabz/core/savedbill"
)

// findSavedBillSQL finds an enabled bill of the user with the same bill_id or the same data, of
// any type (bill: SelectOrInsert in domain/repository/mybill). bill has no ORDER BY; the order
// here only makes the pick fixed when more than one row matches.
const findSavedBillSQL = `SELECT id::text FROM bills
WHERE user_id = $1 AND status = 'enable' AND (bill_id = $2 OR data = $3)
ORDER BY created_at, id LIMIT 1`

// insertSavedBillSQL adds a bill as bill (go-pg) does: an empty string is stored as NULL, json_data
// is NULL with no operator, else {"Operator": ..., "Phone": "", "IsPinned": false} (the bill
// entity.Data keys), and created_at gets its column default.
const insertSavedBillSQL = `INSERT INTO bills (id, user_id, name, type, bill_id, status, data, json_data)
VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, ''), $6, NULLIF($7, ''),
	CASE WHEN $8::text = '' THEN NULL
	ELSE jsonb_build_object('Operator', $8::text, 'Phone', '', 'IsPinned', false) END)`

// getSavedBillSQL reads one bill. A NULL column reads as "".
const getSavedBillSQL = `SELECT id::text, COALESCE(user_id::text, ''), COALESCE(name, ''), COALESCE(type, ''),
	COALESCE(bill_id, ''), COALESCE(data, ''), COALESCE(json_data->>'Operator', ''), COALESCE(status, '')
FROM bills WHERE id = $1`

// uniqueViolation is the SQLSTATE of a broken unique key.
const uniqueViolation = "23505"

// querier is the part of the pgx pool that the repository uses (tests give a fake).
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// SavedBillRepository implements savedbill.Repository on the bills table.
type SavedBillRepository struct {
	db    querier
	newID func() string
}

// NewSavedBillRepository returns a repository that uses the client's pool.
func NewSavedBillRepository(c *Client) *SavedBillRepository {
	r := &SavedBillRepository{newID: func() string { return uuid.NewV4().String() }}
	if c != nil && c.pool != nil {
		r.db = c.pool
	}
	return r
}

// Insert saves b unless the user already has an enabled bill with the same bill_id or data. The
// id of a new row is a random UUID. When the insert breaks a unique key (bills has UNIQUE
// (user_id, type, bill_id)), Insert looks for the row again: a call that saved the same bill at
// the same time is found and its id returned. A key broken by a row that is not enabled gives an
// error, as in bill.
// @todo bill (go-pg SelectOrInsert) tries up to 5 times with a backoff (about 1.75 s) before the
// error; Qabz looks once more, with no wait. See docs/this_vs_bill.md, item 8.
func (r *SavedBillRepository) Insert(ctx context.Context, b savedbill.Bill) (string, bool, error) {
	if r == nil || r.db == nil || r.newID == nil {
		return "", false, errors.New("saved bill repository: no database pool")
	}
	id, found, err := r.find(ctx, b)
	if err != nil || found {
		return id, false, err
	}
	id = r.newID()
	_, err = r.db.Exec(ctx, insertSavedBillSQL, id, b.UserID, b.Name, b.Type, b.BillID, b.Status, b.Data, b.Operator)
	if err == nil {
		return id, true, nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr == nil || pgErr.Code != uniqueViolation {
		return "", false, err
	}
	id, found, findErr := r.find(ctx, b)
	if findErr != nil {
		return "", false, findErr
	}
	if !found {
		return "", false, fmt.Errorf("saved bill repository: insert breaks a unique key and no enabled row matches: %w", err)
	}
	return id, false, nil
}

// find returns the id of the user's enabled bill with the same bill_id or data, if there is one.
func (r *SavedBillRepository) find(ctx context.Context, b savedbill.Bill) (string, bool, error) {
	row := r.db.QueryRow(ctx, findSavedBillSQL, b.UserID, b.BillID, b.Data)
	if row == nil {
		return "", false, errors.New("saved bill repository: no row from QueryRow")
	}
	var id string
	switch err := row.Scan(&id); {
	case errors.Is(err, pgx.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, err
	default:
		return id, true, nil
	}
}

// Get returns the bill with the id, or savedbill.ErrNotFound.
func (r *SavedBillRepository) Get(ctx context.Context, id string) (savedbill.Bill, error) {
	if r == nil || r.db == nil {
		return savedbill.Bill{}, errors.New("saved bill repository: no database pool")
	}
	row := r.db.QueryRow(ctx, getSavedBillSQL, id)
	if row == nil {
		return savedbill.Bill{}, errors.New("saved bill repository: no row from QueryRow")
	}
	var b savedbill.Bill
	err := row.Scan(&b.ID, &b.UserID, &b.Name, &b.Type, &b.BillID, &b.Data, &b.Operator, &b.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return savedbill.Bill{}, savedbill.ErrNotFound
	}
	if err != nil {
		return savedbill.Bill{}, err
	}
	return b, nil
}
