// Package postgres is the PostgreSQL connection (pgx pool).
package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client wraps a pgx pool.
type Client struct{ pool *pgxpool.Pool }

// New opens the pool. pgx connects lazily, so call Ping to check the server.
func New(ctx context.Context, dsn string) (*Client, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	return &Client{pool: pool}, nil
}

// Ping checks the connection.
func (c *Client) Ping(ctx context.Context) error { return c.pool.Ping(ctx) }

// Close releases the pool.
func (c *Client) Close() { c.pool.Close() }

// Pool gives repositories the pool. Do not close it from outside.
func (c *Client) Pool() *pgxpool.Pool { return c.pool }
