package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"qabz/core/fee"
)

// The queries of the fees table (bill: domain/repository/fee/fee_dto.go): the newest row that is
// not deleted, by created_at, one row. config is read as its JSON text.
// @todo bill orders by created_at DESC only; rows with the same created_at have no fixed order.
// Qabz adds id DESC. See docs/this_vs_bill.md, item 39.
const (
	feeColumns = `SELECT id, type, config::text FROM fees `
	feeOrder   = ` AND deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT 1`

	feeUserProviderSQL = feeColumns + `WHERE user_id = $1::uuid AND provider_id = $2` + feeOrder
	feeUserSQL         = feeColumns + `WHERE user_id = $1::uuid AND provider_id IS NULL` + feeOrder
	feeProviderSQL     = feeColumns + `WHERE user_id IS NULL AND provider_id = $1` + feeOrder
	// feeUserIDsSQL gives the user of every row that is not deleted (bill: GetUserIDs).
	feeUserIDsSQL = `SELECT user_id::text FROM fees WHERE user_id IS NOT NULL AND deleted_at IS NULL`
)

// FeeRepository implements fee.Repository on the fees table. Raw queries, no ORM.
type FeeRepository struct {
	db querier
}

var _ fee.Repository = (*FeeRepository)(nil)

// NewFeeRepository returns a repository that uses the client's pool.
func NewFeeRepository(c *Client) *FeeRepository {
	r := &FeeRepository{}
	if c != nil && c.pool != nil {
		r.db = c.pool
	}
	return r
}

var errNoFeePool = errors.New("fee repository: no database pool")

// UserIDs gives the user of every fees row that has one and is not deleted.
func (r *FeeRepository) UserIDs(ctx context.Context) ([]string, error) {
	if r == nil || r.db == nil {
		return nil, errNoFeePool
	}
	rows, err := r.db.Query(ctx, feeUserIDsSQL)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("fee repository: no rows from Query")
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// FindUserProvider gives the row of the user and the provider, or nil.
func (r *FeeRepository) FindUserProvider(ctx context.Context, userID string, providerID int64) (*fee.Row, error) {
	return r.one(ctx, feeUserProviderSQL, userID, providerID)
}

// FindUser gives the row of the user with no provider, or nil.
func (r *FeeRepository) FindUser(ctx context.Context, userID string) (*fee.Row, error) {
	return r.one(ctx, feeUserSQL, userID)
}

// FindProvider gives the row of the provider with no user, or nil.
func (r *FeeRepository) FindProvider(ctx context.Context, providerID int64) (*fee.Row, error) {
	return r.one(ctx, feeProviderSQL, providerID)
}

// one reads one row of a fees query; no row is nil and no error.
func (r *FeeRepository) one(ctx context.Context, sql string, args ...any) (*fee.Row, error) {
	if r == nil || r.db == nil {
		return nil, errNoFeePool
	}
	row := r.db.QueryRow(ctx, sql, args...)
	if row == nil {
		return nil, errors.New("fee repository: no row from QueryRow")
	}
	var x fee.Row
	var config string
	if err := row.Scan(&x.ID, &x.Type, &config); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	x.Config = []byte(config)
	return &x, nil
}
