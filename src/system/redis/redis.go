// Package redis is the Redis connection (go-redis). Every key of Qabz goes through Key, so it has
// the prefix of the config (database.redis.prefix, task 045).
package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

// Client wraps a go-redis client and the key prefix.
type Client struct {
	rdb    *goredis.Client
	prefix string
}

// New creates the client. It connects lazily, so call Ping to check the server. prefix is put in
// front of every key (database.redis.prefix); it can be empty.
func New(addr, password, prefix string) *Client {
	return &Client{rdb: goredis.NewClient(&goredis.Options{Addr: addr, Password: password}), prefix: prefix}
}

// Ping checks the connection.
func (c *Client) Ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }

// Close releases the connections.
func (c *Client) Close() { _ = c.rdb.Close() }

// Key returns the Redis key of name: the prefix, then name. Every command of Qabz uses it.
func (c *Client) Key(name string) string { return c.prefix + name }
