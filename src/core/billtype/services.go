// Package billtype holds the shared bill-type types and the service list (the bill types that
// the app shows, from the services table of the bill database).
package billtype

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"
)

// DefaultLang is the language of the service list when the client sends none (the same as bill).
const DefaultLang = "fa-IR"

// Cache results for ServiceCatalog's OnResult callback.
const (
	CacheHit   = "hit"
	CacheMiss  = "miss"
	CacheError = "error"
)

const (
	// maxCachedLangs limits the number of languages in the cache. The language comes from a
	// client header, so without a limit a client can fill the memory with random values.
	maxCachedLangs = 16
	// maxCachedLangLen: a longer language value is never cached (it is not a real language tag).
	maxCachedLangLen = 35
)

// Service is one row of the services table: one bill type in one language.
type Service struct {
	Name     string
	Type     string
	Tag      string
	IsActive bool
	// Inquiry is the services.inquiry column, as stored.
	// @todo The old bill service ignores this column and always sends true (bill commit 00b3dfb,
	// "fix dev problem", 2023-02-14, no reason given). Qabz sends the stored value, so a row with
	// inquiry = false gives false here. See docs/this_vs_bill.md, item 1.
	Inquiry bool
	Icon    string
}

// ServiceRepository reads the service list of one language, in display order.
type ServiceRepository interface {
	ListServices(ctx context.Context, lang string) ([]Service, error)
}

// ServiceCatalog returns the service list and keeps it in memory for a TTL, per language.
// A failed read is not cached. It is safe for concurrent use.
type ServiceCatalog struct {
	repo     ServiceRepository
	ttl      time.Duration
	now      func() time.Time
	onResult func(result string)

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	services []Service
	expires  time.Time
}

// CatalogOptions configures NewServiceCatalog.
type CatalogOptions struct {
	// TTL is how long a list stays in the cache. It must be positive.
	TTL time.Duration
	// Now returns the current time. Nil means time.Now (tests set a fake clock).
	Now func() time.Time
	// OnResult is called once per List call with CacheHit, CacheMiss or CacheError. Nil: no call.
	OnResult func(result string)
}

// NewServiceCatalog returns a catalog that reads through repo.
func NewServiceCatalog(repo ServiceRepository, opts CatalogOptions) (*ServiceCatalog, error) {
	if repo == nil {
		return nil, errors.New("service catalog: repository is required")
	}
	if opts.TTL <= 0 {
		return nil, errors.New("service catalog: TTL must be positive")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &ServiceCatalog{repo: repo, ttl: opts.TTL, now: now, onResult: opts.OnResult,
		entries: make(map[string]cacheEntry)}, nil
}

// List returns the services of lang (DefaultLang when lang is empty). The caller can change the
// returned slice: it is a copy of the cached list.
func (c *ServiceCatalog) List(ctx context.Context, lang string) ([]Service, error) {
	if lang == "" {
		lang = DefaultLang
	}
	c.mu.Lock()
	e, ok := c.entries[lang]
	c.mu.Unlock()
	if ok && c.now().Before(e.expires) {
		c.report(CacheHit)
		return slices.Clone(e.services), nil
	}

	// The lock is not held during the query. Calls that miss at the same time each query once.
	services, err := c.repo.ListServices(ctx, lang)
	if err != nil {
		c.report(CacheError)
		return nil, err
	}
	if services == nil {
		services = []Service{}
	}
	c.store(lang, services)
	c.report(CacheMiss)
	return slices.Clone(services), nil
}

// store keeps services for lang, inside the limits on languages.
func (c *ServiceCatalog) store(lang string, services []Service) {
	if len(lang) > maxCachedLangLen {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if _, ok := c.entries[lang]; !ok && len(c.entries) >= maxCachedLangs {
		for k, e := range c.entries {
			if !now.Before(e.expires) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= maxCachedLangs {
			return // full of live entries: serve this language from the database only
		}
	}
	c.entries[lang] = cacheEntry{services: slices.Clone(services), expires: now.Add(c.ttl)}
}

func (c *ServiceCatalog) report(result string) {
	if c.onResult != nil {
		c.onResult(result)
	}
}
