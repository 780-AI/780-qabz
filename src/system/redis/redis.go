// Package redis is the Redis connection (go-redis). Every key of Qabz goes through Key, so it has
// the prefix of the config (database.redis.prefix, task 045).
package redis

import (
	"context"

	"github.com/redis/go-redis/extra/redisotel/v9"
	goredis "github.com/redis/go-redis/v9"
)

// Client wraps a go-redis client and the key prefix.
type Client struct {
	rdb    *goredis.Client
	prefix string
}

// New creates the client. It connects lazily, so call Ping to check the server. prefix is put in
// front of every key (database.redis.prefix); it can be empty. Each command and each pipeline is a
// client span (redisotel, task 047 decision D8) with the command name and no argument: the keys can
// have a phone or a bill ID.
func New(addr, password, prefix string) *Client {
	rdb := goredis.NewClient(&goredis.Options{Addr: addr, Password: password})
	// InstrumentTracing only adds a hook; it fails only for a client type it does not know.
	_ = redisotel.InstrumentTracing(rdb, redisotel.WithDBStatement(false), redisotel.WithCallerEnabled(false))
	return &Client{rdb: rdb, prefix: prefix}
}

// Ping checks the connection.
func (c *Client) Ping(ctx context.Context) error { return c.rdb.Ping(ctx).Err() }

// Close releases the connections.
func (c *Client) Close() { _ = c.rdb.Close() }

// Key returns the Redis key of name: the prefix, then name. Every command of Qabz uses it.
func (c *Client) Key(name string) string { return c.prefix + name }
