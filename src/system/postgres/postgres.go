// Package postgres is the PostgreSQL connection (pgx pool).
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Client wraps a pgx pool.
type Client struct{ pool *pgxpool.Pool }

// New opens the pool. pgx connects lazily, so call Ping to check the server. Each query of the pool
// is a span (the tracer of tracer.go; task 047).
func New(ctx context.Context, dsn string) (*Client, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	if cfg == nil || cfg.ConnConfig == nil {
		return nil, errors.New("postgres: ParseConfig returned no config")
	}
	cfg.ConnConfig.Tracer = tracer{}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
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
