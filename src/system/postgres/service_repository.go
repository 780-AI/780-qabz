package postgres

import (
	"context"
	"errors"

	"qabz/core/billtype"
)

// listServicesSQL reads the services table of the bill database, as bill does: the rows of one
// language, by the order column. id is a tie-breaker, so equal orders keep one fixed order.
// A NULL text or boolean column reads as "" or false (go-pg in bill reads it the same way), so
// one bad row does not fail the whole list.
const listServicesSQL = `SELECT COALESCE(name, ''), COALESCE(type, ''), COALESCE(tag, ''), COALESCE(is_active, false),
	COALESCE(inquiry, false), COALESCE(icon, '')
FROM services WHERE lang = $1 ORDER BY "order" ASC, id ASC`

// ServiceRepository implements billtype.ServiceRepository on the services table.
type ServiceRepository struct{ client *Client }

// NewServiceRepository returns a repository that uses the client's pool.
func NewServiceRepository(c *Client) *ServiceRepository { return &ServiceRepository{client: c} }

// ListServices returns the services of lang in display order. No rows give an empty list.
func (r *ServiceRepository) ListServices(ctx context.Context, lang string) ([]billtype.Service, error) {
	if r == nil || r.client == nil || r.client.pool == nil {
		return nil, errors.New("service repository: no database pool")
	}
	rows, err := r.client.pool.Query(ctx, listServicesSQL, lang)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []billtype.Service{}
	for rows.Next() {
		var s billtype.Service
		if err := rows.Scan(&s.Name, &s.Type, &s.Tag, &s.IsActive, &s.Inquiry, &s.Icon); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
