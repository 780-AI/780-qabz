package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

// redisURLEnv names the variable with the compose Redis of `make test` (testdb redis-url).
const redisURLEnv = "QABZ_TEST_REDIS_URL"

// liveRedis returns the address and password of the test Redis and a new key prefix of the test
// (qabz_test_<hex>:), so no key of another test or of the App is touched. Without the variable
// (a plain go test) the test is skipped. The keys of the test have short TTLs and expire.
func liveRedis(t *testing.T) (addr, password, prefix string) {
	t.Helper()
	raw := os.Getenv(redisURLEnv)
	if raw == "" {
		t.Skip(redisURLEnv + " is not set (run make test)")
	}
	u, err := url.Parse(raw)
	if !assert.NoError(t, err) || !assert.NotNil(t, u) {
		t.FailNow()
	}
	if u.User != nil {
		password, _ = u.User.Password()
	}
	b := make([]byte, 8)
	if _, err := rand.Read(b); !assert.NoError(t, err) {
		t.FailNow()
	}
	return u.Host, password, "qabz_test_" + hex.EncodeToString(b) + ":"
}

// rawClient is a go-redis client with no prefix, to read and change the keys as another holder.
func rawClient(t *testing.T, addr, password string) *goredis.Client {
	t.Helper()
	rdb := goredis.NewClient(&goredis.Options{Addr: addr, Password: password})
	t.Cleanup(func() { _ = rdb.Close() })
	return rdb
}

func TestKeyHasPrefix(t *testing.T) {
	c := New("127.0.0.1:1", "", "qabz:")
	defer c.Close()
	assert.Equal(t, "qabz:crons_x_lock1", c.Key("crons_x_lock1"))
	assert.Equal(t, "a", New("127.0.0.1:1", "", "").Key("a"), "an empty prefix adds nothing")
}

func TestTryLockErrors(t *testing.T) {
	var nilClient *Client
	_, err := nilClient.TryLock(context.Background(), "x", time.Second)
	assert.Error(t, err)

	c := New("127.0.0.1:1", "", "p:")
	defer c.Close()
	_, err = c.TryLock(context.Background(), "x", 0)
	assert.ErrorContains(t, err, "TTL must be positive")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	l, err := c.TryLock(ctx, "x", time.Second)
	assert.Error(t, err, "no server")
	assert.Nil(t, l)

	var nilLock *Lock
	assert.Error(t, nilLock.ExtendAtLeast(ctx, time.Second))
	assert.Error(t, nilLock.Release(ctx))
}

// TestLiveOneHolder: many callers at the same time, one gets the lock; the key has the prefix and a
// TTL; after the release another caller gets it.
func TestLiveOneHolder(t *testing.T) {
	addr, password, prefix := liveRedis(t)
	ctx := context.Background()
	a, b := New(addr, password, prefix), New(addr, password, prefix)
	defer a.Close()
	defer b.Close()

	var mu sync.Mutex
	var got []*Lock
	var wg sync.WaitGroup
	for i := range 10 {
		c := a
		if i%2 == 1 {
			c = b
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			l, err := c.TryLock(ctx, "crons_one_lock1", 5*time.Second)
			assert.NoError(t, err)
			if l != nil {
				mu.Lock()
				got = append(got, l)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if !assert.Len(t, got, 1, "exactly one holder") {
		return
	}
	assert.Equal(t, prefix+"crons_one_lock1", got[0].Key())

	rdb := rawClient(t, addr, password)
	keys, err := rdb.Keys(ctx, prefix+"*").Result()
	assert.NoError(t, err)
	assert.Equal(t, []string{prefix + "crons_one_lock1"}, keys, "every key has the prefix")
	ttl, err := rdb.PTTL(ctx, prefix+"crons_one_lock1").Result()
	assert.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 5*time.Second, "ttl %s", ttl)

	if !assert.NoError(t, got[0].Release(ctx)) {
		return
	}
	n, err := rdb.Exists(ctx, prefix+"crons_one_lock1").Result()
	assert.NoError(t, err)
	assert.Zero(t, n, "the release deletes the own key")
	l, err := b.TryLock(ctx, "crons_one_lock1", 5*time.Second)
	if assert.NoError(t, err) && assert.NotNil(t, l) {
		assert.NoError(t, l.Release(ctx))
	}
}

// TestLiveExtend: the TTL grows to the asked value; a longer TTL is not made shorter (bill:
// ChargeTTL).
func TestLiveExtend(t *testing.T) {
	addr, password, prefix := liveRedis(t)
	ctx := context.Background()
	c := New(addr, password, prefix)
	defer c.Close()
	rdb := rawClient(t, addr, password)

	l, err := c.TryLock(ctx, "ext", 2*time.Second)
	if !assert.NoError(t, err) || !assert.NotNil(t, l) {
		return
	}
	defer func() { _ = l.Release(ctx) }()
	assert.NoError(t, l.ExtendAtLeast(ctx, 20*time.Second))
	ttl, err := rdb.PTTL(ctx, l.Key()).Result()
	assert.NoError(t, err)
	assert.Greater(t, ttl, 10*time.Second, "extended")

	assert.NoError(t, l.ExtendAtLeast(ctx, 3*time.Second))
	ttl, err = rdb.PTTL(ctx, l.Key()).Result()
	assert.NoError(t, err)
	assert.Greater(t, ttl, 10*time.Second, "not made shorter")
}

// TestLiveOnlyOwnLock: when the key holds the token of another holder, ExtendAtLeast is
// ErrLockLost and Release does not delete the key (task 045: only the own lock).
func TestLiveOnlyOwnLock(t *testing.T) {
	addr, password, prefix := liveRedis(t)
	ctx := context.Background()
	c := New(addr, password, prefix)
	defer c.Close()
	rdb := rawClient(t, addr, password)

	l, err := c.TryLock(ctx, "own", 5*time.Second)
	if !assert.NoError(t, err) || !assert.NotNil(t, l) {
		return
	}
	// The TTL ended and another instance took the key.
	assert.NoError(t, rdb.Set(ctx, l.Key(), "other-token", 5*time.Second).Err())

	err = l.ExtendAtLeast(ctx, time.Minute)
	assert.True(t, errors.Is(err, ErrLockLost), "%v", err)
	assert.NoError(t, l.Release(ctx), "a key of another holder is not an error")
	v, err := rdb.Get(ctx, l.Key()).Result()
	assert.NoError(t, err)
	assert.Equal(t, "other-token", v, "the key of the other holder stays")

	// No key at all: the lock is lost too.
	assert.NoError(t, rdb.Del(ctx, l.Key()).Err())
	assert.True(t, errors.Is(l.ExtendAtLeast(ctx, time.Minute), ErrLockLost))
	assert.NoError(t, l.Release(ctx))
}
