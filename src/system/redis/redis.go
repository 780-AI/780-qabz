// Package redis is the Redis connection (go-redis).
package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

// Client wraps a go-redis client.
type Client struct{ rdb *goredis.Client }

// New creates the client. It connects lazily, so call Ping to check the server.
func New(addr, password string) *Client {
	return &Client{rdb: goredis.NewClient(&goredis.Options{Addr: addr, Password: password})}
}

// Ping checks the connection.
func (c *Client) Ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }

// Close releases the connections.
func (c *Client) Close() { _ = c.rdb.Close() }

// Raw gives repositories the go-redis client. Do not close it from outside.
func (c *Client) Raw() *goredis.Client { return c.rdb }
