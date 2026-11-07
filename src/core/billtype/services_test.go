package billtype

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeRepo returns rows per language and counts the calls per language.
type fakeRepo struct {
	mu    sync.Mutex
	rows  map[string][]Service
	err   error
	calls map[string]int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: map[string][]Service{}, calls: map[string]int{}}
}

func (f *fakeRepo) ListServices(_ context.Context, lang string) ([]Service, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[lang]++
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[lang], nil
}

func (f *fakeRepo) count(lang string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[lang]
}

func (f *fakeRepo) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// clock is a fake time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

var sample = []Service{
	{Name: "Water", Type: "water", Tag: "new", IsActive: true, Inquiry: true, Icon: "w.svg"},
	{Name: "Gas", Type: "gas", IsActive: false, Inquiry: false, Icon: "g.svg"},
}

// newCatalog returns a catalog with a 60s TTL, a fake clock and a result log. It stops the test on error.
func newCatalog(t *testing.T, repo ServiceRepository) (*ServiceCatalog, *clock, *[]string) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)}
	var mu sync.Mutex
	var results []string
	c, err := NewServiceCatalog(repo, CatalogOptions{TTL: 60 * time.Second, Now: clk.now,
		OnResult: func(r string) { mu.Lock(); results = append(results, r); mu.Unlock() }})
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	return c, clk, &results
}

func TestNewServiceCatalogRules(t *testing.T) {
	_, err := NewServiceCatalog(nil, CatalogOptions{TTL: time.Second})
	assert.ErrorContains(t, err, "repository is required")
	for _, ttl := range []time.Duration{0, -time.Second} {
		_, err = NewServiceCatalog(newFakeRepo(), CatalogOptions{TTL: ttl})
		assert.ErrorContains(t, err, "TTL must be positive")
	}
	c, err := NewServiceCatalog(newFakeRepo(), CatalogOptions{TTL: time.Second}) // nil Now and OnResult
	if assert.NoError(t, err) && assert.NotNil(t, c) {
		got, err := c.List(context.Background(), "")
		assert.NoError(t, err)
		assert.Equal(t, []Service{}, got)
	}
}

func TestListReturnsRowsAsIs(t *testing.T) {
	repo := newFakeRepo()
	repo.rows["fa-IR"] = sample
	c, _, results := newCatalog(t, repo)
	got, err := c.List(context.Background(), "fa-IR")
	assert.NoError(t, err)
	assert.Equal(t, sample, got)
	assert.Equal(t, []string{CacheMiss}, *results)
}

func TestListEmptyLangIsDefault(t *testing.T) {
	repo := newFakeRepo()
	repo.rows[DefaultLang] = sample
	c, _, _ := newCatalog(t, repo)
	got, err := c.List(context.Background(), "")
	assert.NoError(t, err)
	assert.Equal(t, sample, got)
	assert.Equal(t, 1, repo.count(DefaultLang))
	assert.Equal(t, 0, repo.count(""))
}

func TestListCachesUntilTTL(t *testing.T) {
	repo := newFakeRepo()
	repo.rows["fa-IR"] = sample
	c, clk, results := newCatalog(t, repo)
	ctx := context.Background()

	_, _ = c.List(ctx, "fa-IR")
	clk.add(59 * time.Second)
	got, err := c.List(ctx, "fa-IR")
	assert.NoError(t, err)
	assert.Equal(t, sample, got)
	assert.Equal(t, 1, repo.count("fa-IR"), "inside the TTL: no query")

	clk.add(time.Second) // exactly the TTL: expired
	_, _ = c.List(ctx, "fa-IR")
	assert.Equal(t, 2, repo.count("fa-IR"), "after the TTL: one new query")
	assert.Equal(t, []string{CacheMiss, CacheHit, CacheMiss}, *results)
}

func TestListCachesPerLang(t *testing.T) {
	repo := newFakeRepo()
	repo.rows["fa-IR"] = sample[:1]
	repo.rows["en-US"] = sample[1:]
	c, _, _ := newCatalog(t, repo)
	ctx := context.Background()
	fa, _ := c.List(ctx, "fa-IR")
	en, _ := c.List(ctx, "en-US")
	assert.Equal(t, sample[:1], fa)
	assert.Equal(t, sample[1:], en)
	_, _ = c.List(ctx, "fa-IR")
	_, _ = c.List(ctx, "en-US")
	assert.Equal(t, 1, repo.count("fa-IR"))
	assert.Equal(t, 1, repo.count("en-US"))
}

func TestListErrorIsNotCached(t *testing.T) {
	repo := newFakeRepo()
	repo.rows["fa-IR"] = sample
	repo.setErr(errors.New("db down"))
	c, _, results := newCatalog(t, repo)
	ctx := context.Background()

	got, err := c.List(ctx, "fa-IR")
	assert.EqualError(t, err, "db down")
	assert.Nil(t, got)

	repo.setErr(nil)
	got, err = c.List(ctx, "fa-IR")
	assert.NoError(t, err)
	assert.Equal(t, sample, got)
	assert.Equal(t, 2, repo.count("fa-IR"), "the error was not cached")
	assert.Equal(t, []string{CacheError, CacheMiss}, *results)
}

func TestListEmptyResultIsCached(t *testing.T) {
	repo := newFakeRepo() // no rows: the repository returns nil
	c, _, _ := newCatalog(t, repo)
	for range 2 {
		got, err := c.List(context.Background(), "fa-IR")
		assert.NoError(t, err)
		assert.NotNil(t, got)
		assert.Empty(t, got)
	}
	assert.Equal(t, 1, repo.count("fa-IR"))
}

func TestListReturnsACopy(t *testing.T) {
	repo := newFakeRepo()
	repo.rows["fa-IR"] = []Service{{Name: "Water", Type: "water"}}
	c, _, _ := newCatalog(t, repo)
	first, err := c.List(context.Background(), "fa-IR")
	if !assert.NoError(t, err) || !assert.Len(t, first, 1) {
		return
	}
	first[0].Name = "changed by caller"
	second, err := c.List(context.Background(), "fa-IR")
	if !assert.NoError(t, err) || !assert.Len(t, second, 1) {
		return
	}
	assert.Equal(t, "Water", second[0].Name, "the cache is not changed through a returned slice")
	assert.Equal(t, "Water", repo.rows["fa-IR"][0].Name, "the repository rows are not changed")
}

func TestListLimitsCachedLangs(t *testing.T) {
	repo := newFakeRepo()
	c, clk, _ := newCatalog(t, repo)
	ctx := context.Background()
	for i := range maxCachedLangs {
		_, _ = c.List(ctx, "l"+strconv.Itoa(i))
	}
	// The cache is full of live entries: a new language is read each time and not stored.
	_, _ = c.List(ctx, "extra")
	_, _ = c.List(ctx, "extra")
	assert.Equal(t, 2, repo.count("extra"))
	assert.Len(t, c.entries, maxCachedLangs)

	// After the TTL the expired entries make room.
	clk.add(time.Minute)
	_, _ = c.List(ctx, "extra")
	_, _ = c.List(ctx, "extra")
	assert.Equal(t, 3, repo.count("extra"))
	assert.Len(t, c.entries, 1)

	// A very long value is never cached.
	long := strings.Repeat("x", maxCachedLangLen+1)
	_, _ = c.List(ctx, long)
	_, _ = c.List(ctx, long)
	assert.Equal(t, 2, repo.count(long))
}

func TestListConcurrent(t *testing.T) {
	repo := newFakeRepo()
	repo.rows["fa-IR"] = sample
	c, clk, _ := newCatalog(t, repo)
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%16 == 0 {
				clk.add(30 * time.Second)
			}
			got, err := c.List(context.Background(), "fa-IR")
			assert.NoError(t, err)
			assert.Equal(t, sample, got)
		}(i)
	}
	wg.Wait()
}
