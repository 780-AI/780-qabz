package postgres

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/valyala/fastjson"

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

// listSavedBillsSQL reads the enabled bills of a user, newest first (bill: GetAll in
// domain/repository/mybill). A NULL column reads as "", json_data as its JSON text.
// @todo bill orders by created_at DESC only, so rows with the same created_at come in any order;
// Qabz adds id DESC to make the order fixed. See docs/this_vs_bill.md, item 9.
const listSavedBillsSQL = `SELECT id::text, COALESCE(user_id::text, ''), COALESCE(name, ''), COALESCE(type, ''),
	COALESCE(bill_id, ''), COALESCE(data, ''), COALESCE(last_order_date, ''), COALESCE(status, ''),
	COALESCE(json_data::text, '')
FROM bills WHERE user_id = $1 AND status = 'enable'
ORDER BY created_at DESC, id DESC`

// The json_data keys of the bill entity.Data struct.
var (
	keyOperator = []byte("Operator")
	keyPhone    = []byte("Phone")
	keyIsPinned = []byte("IsPinned")
)

// uniqueViolation is the SQLSTATE of a broken unique key.
const uniqueViolation = "23505"

// querier is the part of the pgx pool that the repository uses (tests give a fake).
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// SavedBillRepository implements savedbill.Repository on the bills table.
type SavedBillRepository struct {
	db    querier
	newID func() string
	// parsers read json_data in List. List runs for each GET /v1/mybill, so it is a hot path
	// (conventions § 1). The zero value is ready to use.
	parsers fastjson.ParserPool
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

// List returns the enabled bills of the user, newest first. A user with no bills gives an empty
// list. A json_data that bill cannot read gives an error for the whole list, as in bill.
func (r *SavedBillRepository) List(ctx context.Context, userID string) ([]savedbill.Bill, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("saved bill repository: no database pool")
	}
	rows, err := r.db.Query(ctx, listSavedBillsSQL, userID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("saved bill repository: no rows from Query")
	}
	defer rows.Close()
	p := r.parsers.Get()
	if p == nil {
		return nil, errors.New("saved bill repository: no JSON parser")
	}
	defer r.parsers.Put(p)
	var out []savedbill.Bill
	for rows.Next() {
		var b savedbill.Bill
		var data string
		if err := rows.Scan(&b.ID, &b.UserID, &b.Name, &b.Type, &b.BillID, &b.Data, &b.LastOrderDate, &b.Status, &data); err != nil {
			return nil, err
		}
		if b.Operator, b.IsPinned, err = billData(p, data); err != nil {
			return nil, fmt.Errorf("saved bill repository: bill %s: %w", b.ID, err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// billData reads json_data (raw, its JSON text; "" for NULL) as bill does. go-pg decodes it with
// encoding/json into the struct entity.Data{Operator, Phone string; IsPinned bool}, so:
//   - a key matches its field without case ("operator" is Operator; "isPined" matches nothing);
//   - when two keys match one field, the later key in the object wins;
//   - other keys, and a null value, change nothing; a JSON null json_data gives the zero values;
//   - a value of the wrong JSON type, or a json_data that is not an object or null, is an error.
//
// The returned strings are copies: nothing refers to the parser after it goes back to the pool.
func billData(p *fastjson.Parser, raw string) (operator string, pinned bool, err error) {
	if raw == "" {
		return "", false, nil
	}
	if p == nil {
		return "", false, errors.New("json_data: no parser")
	}
	v, err := p.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("json_data: %w", err)
	}
	if v == nil {
		return "", false, errors.New("json_data: no value")
	}
	switch v.Type() {
	case fastjson.TypeNull:
		return "", false, nil
	case fastjson.TypeObject:
	default:
		return "", false, fmt.Errorf("json_data is a JSON %s, not an object", v.Type())
	}
	o, err := v.Object()
	if err != nil {
		return "", false, fmt.Errorf("json_data: %w", err)
	}
	if o == nil {
		return "", false, errors.New("json_data: no object")
	}
	o.Visit(func(key []byte, val *fastjson.Value) {
		if err != nil || val == nil {
			return
		}
		switch {
		case bytes.EqualFold(key, keyOperator):
			if s, ok, e := jsonString(val); e != nil {
				err = fmt.Errorf("json_data %s: %w", key, e)
			} else if ok {
				operator = s
			}
		case bytes.EqualFold(key, keyPhone): // not in the answer, but a wrong type fails bill too
			if _, _, e := jsonString(val); e != nil {
				err = fmt.Errorf("json_data %s: %w", key, e)
			}
		case bytes.EqualFold(key, keyIsPinned):
			switch val.Type() {
			case fastjson.TypeTrue:
				pinned = true
			case fastjson.TypeFalse:
				pinned = false
			case fastjson.TypeNull:
			default:
				err = fmt.Errorf("json_data %s: a JSON %s, not a bool", key, val.Type())
			}
		}
	})
	if err != nil {
		return "", false, err
	}
	return operator, pinned, nil
}

// jsonString returns a copy of the string in v. A JSON null gives ok false (the field keeps its
// value); another type is an error.
func jsonString(v *fastjson.Value) (s string, ok bool, err error) {
	if v == nil {
		return "", false, errors.New("no JSON value")
	}
	switch v.Type() {
	case fastjson.TypeString:
		b, err := v.StringBytes()
		if err != nil {
			return "", false, err
		}
		return string(b), true, nil
	case fastjson.TypeNull:
		return "", false, nil
	default:
		return "", false, fmt.Errorf("a JSON %s, not a string", v.Type())
	}
}
