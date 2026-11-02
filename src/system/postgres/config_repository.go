package postgres

import (
	"context"

	"qabz/config"
)

// ConfigRepository loads the hot-reloadable configs from the table app_hot_configs.
type ConfigRepository struct{ client *Client }

// NewConfigRepository returns a repository that uses the client's pool.
func NewConfigRepository(c *Client) *ConfigRepository { return &ConfigRepository{client: c} }

// Load reads all rows in one query, so the rows are one consistent set.
func (r *ConfigRepository) Load(ctx context.Context) ([]config.Row, error) {
	rows, err := r.client.pool.Query(ctx, `SELECT key, cnf_type, value FROM app_hot_configs`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []config.Row
	for rows.Next() {
		var row config.Row
		if err := rows.Scan(&row.Key, &row.Type, &row.Value); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
