package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"qabz/provider"
)

// listProvidersSQL reads every row of the providers table of the bill database. A NULL column reads
// as its zero value ("", 0, false), as go-pg in bill reads it, so one bad row does not fail the
// whole list; updated_at is scanned as a nullable value. There is no ORDER BY: ProvidersService
// sorts the rows, and the row order does not change its result.
const listProvidersSQL = `SELECT id, COALESCE(name, ''), COALESCE(subject, ''), COALESCE(bill_type, ''),
	COALESCE(action, ''), COALESCE(type, ''), COALESCE(priority, 0), COALESCE(timeout, 0),
	COALESCE(status, ''), updated_at, COALESCE(use_nats, false), COALESCE(inquiry_step, 0),
	COALESCE(provider_fee, 0)
FROM providers`

// rowsQuerier is the part of the pgx pool that ProviderRepository uses (tests give a fake).
type rowsQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// ProviderRepository implements provider.Repository on the providers table. Only
// provider.ProvidersService uses it (PROJECT_SPECIFIC_CONVENTIONS.md § 13).
type ProviderRepository struct{ db rowsQuerier }

// NewProviderRepository returns a repository that uses the client's pool.
func NewProviderRepository(c *Client) *ProviderRepository {
	r := &ProviderRepository{}
	if c != nil && c.pool != nil {
		r.db = c.pool
	}
	return r
}

// ListProviders returns every row of the providers table, in no fixed order. No rows give an empty
// list.
func (r *ProviderRepository) ListProviders(ctx context.Context) ([]provider.Provider, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("provider repository: no database pool")
	}
	rows, err := r.db.Query(ctx, listProvidersSQL)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("provider repository: query returned no rows object")
	}
	defer rows.Close()
	out := []provider.Provider{}
	for rows.Next() {
		var p provider.Provider
		var updated *time.Time
		if err := rows.Scan(&p.ID, &p.Name, &p.Subject, &p.BillType, &p.Action, &p.Type, &p.Priority,
			&p.Timeout, &p.Status, &updated, &p.UseNats, &p.InquiryStep, &p.ProviderFee); err != nil {
			return nil, err
		}
		if updated != nil {
			p.UpdatedAt = *updated
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
