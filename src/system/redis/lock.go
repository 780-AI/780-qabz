package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// ErrLockLost is the error of Lock.ExtendAtLeast when the key does not hold the token of the lock
// any more (the TTL ended, or another instance took the key).
var ErrLockLost = errors.New("redis: the lock is not held any more")

// extendScript sets the TTL of the key to ARGV[2] ms when the key holds the token ARGV[1] and its
// TTL is less (bill: lock.Extender.ChargeTTL). It returns 0 when the key does not hold the token.
var extendScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) ~= ARGV[1] then return 0 end
local ttl = redis.call("PTTL", KEYS[1])
if ttl >= 0 and ttl < tonumber(ARGV[2]) then redis.call("PEXPIRE", KEYS[1], ARGV[2]) end
return 1`)

// releaseScript deletes the key only when it holds the token ARGV[1] (task 045: Qabz deletes only
// its own lock; bill's Release deletes the key of any holder).
var releaseScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) end
return 0`)

// Lock is a lock that this instance holds: a key with a random token of its own.
type Lock struct {
	c     *Client
	key   string
	token string
}

// TryLock takes the lock name (the key Key(name)) for ttl with SET NX (bill: lock.Acquire). The
// value is a new random token. It returns a nil Lock and a nil error when another holder has the
// key.
func (c *Client) TryLock(ctx context.Context, name string, ttl time.Duration) (*Lock, error) {
	if c == nil || c.rdb == nil {
		return nil, errors.New("redis: client not created with New")
	}
	if ttl <= 0 {
		return nil, errors.New("redis: the lock TTL must be positive")
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("redis: lock token: %w", err)
	}
	l := &Lock{c: c, key: c.Key(name), token: hex.EncodeToString(b)}
	ok, err := c.rdb.SetNX(ctx, l.key, l.token, ttl).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: lock %s: %w", l.key, err)
	}
	if !ok {
		return nil, nil
	}
	return l, nil
}

// Key returns the Redis key of the lock (with the prefix); "" for a nil lock.
func (l *Lock) Key() string {
	if l == nil {
		return ""
	}
	return l.key
}

// ExtendAtLeast makes the TTL of the lock ttl when less is left (bill: ChargeTTL). The key does not
// hold the token: ErrLockLost.
func (l *Lock) ExtendAtLeast(ctx context.Context, ttl time.Duration) error {
	if l == nil || l.c == nil {
		return errors.New("redis: lock not taken with TryLock")
	}
	n, err := extendScript.Run(ctx, l.c.rdb, []string{l.key}, l.token, ttl.Milliseconds()).Int()
	if err != nil {
		return fmt.Errorf("redis: extend lock %s: %w", l.key, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrLockLost, l.key)
	}
	return nil
}

// Release deletes the key when it still holds the token of the lock. A key of another holder, or
// no key, is not touched and is not an error.
func (l *Lock) Release(ctx context.Context) error {
	if l == nil || l.c == nil {
		return errors.New("redis: lock not taken with TryLock")
	}
	if err := releaseScript.Run(ctx, l.c.rdb, []string{l.key}, l.token).Err(); err != nil {
		return fmt.Errorf("redis: release lock %s: %w", l.key, err)
	}
	return nil
}
