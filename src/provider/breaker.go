package provider

// The circuit breaker (task 060; system.html, Provider Switcher; conventions § 13). It counts the
// results of the provider calls of each provider row (one provider, bill type and action) in a
// sliding window. When the row has MinCalls calls or more in the window and FailureRatio percent or
// more of them failed, it asks the Provider Switcher to demote the row for OpenTime
// (ProvidersService.Demote). The breaker never blocks a call: it only demotes, and only when the
// group has a next available row. The state is in memory in each instance of the App; the key is
// the row id, so a load of the providers table keeps the count of a row that stays.
//
// The state of a row: closed (it counts), open (the row is demoted: its results are not counted)
// and, after a Demote that found no next available row, a pause of OpenTime with no count. The
// Provider Switcher drives open and closed through Breaker.Demoted (Options.OnDemoted).

import (
	"errors"
	"sync"
	"time"
)

// Transitions of a breaker, for BreakerOptions.OnTransition (a fixed set: metric labels).
const (
	BreakerOpen   = "open"   // the row was demoted
	BreakerClosed = "closed" // the row returned: the count starts new
)

// breakerBuckets is the number of time buckets of the sliding window: old calls leave the window
// in steps of window/breakerBuckets. Fixed (not a config key).
const breakerBuckets = 20

// BreakerSettings are the settings of the breaker of one row. Read at each report, so a hot change
// applies at once. The values are already checked (the App gives only valid values).
type BreakerSettings struct {
	// Enabled false: no count and no demotion (the behavior before task 060).
	Enabled bool
	// FailureRatio is the percent of failed calls (1 to 100) at which the row opens.
	FailureRatio int
	// Window is the length of the sliding window (greater than 0).
	Window time.Duration
	// MinCalls is the smallest number of calls in the window for which the ratio is trusted (1 or more).
	MinCalls int
	// OpenTime is the demotion period of the row (greater than 0).
	OpenTime time.Duration
}

// valid reports whether s can be used to count (a switch that is off is valid).
func (s BreakerSettings) valid() bool {
	return s.FailureRatio >= 1 && s.FailureRatio <= 100 && s.Window > 0 && s.MinCalls >= 1 && s.OpenTime > 0
}

// BreakerOptions configure NewBreaker.
type BreakerOptions struct {
	// Settings gives the settings of the row p. Called at each report, before the lock. Required.
	Settings func(p Provider) BreakerSettings
	// Demote demotes the row id for period (ProvidersService.Demote). Called in its own goroutine,
	// one at a time for each row. Required.
	Demote func(id int64, period time.Duration) error
	// Logger gets one info line for each open and each close. Required.
	Logger Logger
	// OnTransition is called with the row and BreakerOpen or BreakerClosed at each change of the
	// state. Nil: no call.
	OnTransition func(p Provider, to string)
}

// Breaker is the circuit breaker of all provider rows. Safe for concurrent use.
type Breaker struct {
	settings     func(p Provider) BreakerSettings
	demote       func(id int64, period time.Duration) error
	log          Logger
	onTransition func(p Provider, to string)
	rowsMu       sync.RWMutex
	rows         map[int64]*breakerRow // the state of each row id; an entry is never removed (bounded by the rows)

	now   func() time.Time // replaced by tests
	spawn func(job func()) // replaced by tests (run the job at once)
}

// breakerBucket is the count of one time bucket. epoch is the number of the bucket since the Unix
// epoch (the time divided by the bucket width); a slot with an old epoch is empty.
type breakerBucket struct {
	epoch         int64
	total, failed int
}

// breakerRow is the state of one row. mu is held only for short work in memory.
type breakerRow struct {
	mu       sync.Mutex
	window   time.Duration // the window of the buckets; a change starts the count new
	buckets  [breakerBuckets]breakerBucket
	open     bool      // the row is demoted: results are not counted
	demoting bool      // a Demote of this row runs
	pausedTo time.Time // after no next available row: no count before this time
	// total and failed are the counts that started the last Demote (for the open log line).
	total, failed int
}

// NewBreaker returns the breaker.
func NewBreaker(o BreakerOptions) (*Breaker, error) {
	switch {
	case o.Settings == nil:
		return nil, errors.New("circuit breaker: settings are required")
	case o.Demote == nil:
		return nil, errors.New("circuit breaker: demote is required")
	case o.Logger == nil:
		return nil, errors.New("circuit breaker: logger is required")
	}
	return &Breaker{settings: o.Settings, demote: o.Demote, log: o.Logger, onTransition: o.OnTransition,
		rows: map[int64]*breakerRow{}, now: time.Now, spawn: func(job func()) { go job() }}, nil
}

// Report counts one result of a provider call of the row p: failed is true for a provider-down
// result (an error that wraps ErrProviderDown), false for any answer of the provider (a business
// answer too). A result that gives no verdict (a cancel, a fault before the call, a saved answer, a
// trial) is not reported. Report takes a short lock in memory: no database call and no wait. At
// the threshold it starts the Demote of the row in its own goroutine. A nil Breaker, or one not
// made by NewBreaker, does nothing. Safe for concurrent use.
func (b *Breaker) Report(p Provider, failed bool) {
	if !b.built() {
		return
	}
	s := b.settings(p)
	if !s.Enabled || !s.valid() {
		return
	}
	r := b.row(p.ID)
	now := b.now()
	r.mu.Lock()
	if r.open || r.demoting || now.Before(r.pausedTo) {
		r.mu.Unlock()
		return
	}
	if r.window != s.Window {
		r.reset()
		r.window = s.Window
	}
	total, bad := r.add(now, failed)
	if total < s.MinCalls || bad*100 < s.FailureRatio*total {
		r.mu.Unlock()
		return
	}
	r.demoting, r.total, r.failed = true, total, bad
	r.mu.Unlock()
	b.spawn(func() { b.startDemote(p, r, s.OpenTime, now) })
}

// startDemote runs the Demote of the row p at the threshold. The Provider Switcher calls Demoted
// (open) during a good Demote. With no next available row the row pauses for openTime and then
// counts new; any other result ends the try and the count starts new.
func (b *Breaker) startDemote(p Provider, r *breakerRow, openTime time.Duration, at time.Time) {
	err := b.demote(p.ID, openTime)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.demoting = false
	if errors.Is(err, ErrNoNextProvider) {
		r.pausedTo = at.Add(openTime)
	}
	if !r.open {
		r.reset() // no demotion (no next row, the row is disabled or gone): the count starts new
	}
}

// Demoted is the OnDemoted callback of the Provider Switcher (Options.OnDemoted): true opens the
// row (its results are not counted while it is demoted), false closes it (a good trial, a return
// with no trial, a load or Disable that cleared it) and the count starts new. A failed trial does
// not call it: the row stays open. A nil Breaker, or one not made by NewBreaker, does nothing. Safe
// for concurrent use; it never calls the Provider Switcher.
func (b *Breaker) Demoted(p Provider, demoted bool) {
	if !b.built() {
		return
	}
	r := b.row(p.ID)
	r.mu.Lock()
	if r.open == demoted {
		r.mu.Unlock()
		return
	}
	r.open = demoted
	r.reset()
	total, failed := r.total, r.failed
	r.total, r.failed = 0, 0
	r.mu.Unlock()
	to := BreakerClosed
	if demoted {
		to = BreakerOpen
		b.log.Info("circuit breaker: open, row demoted", "id", p.ID, "provider", p.Name, "bill_type", p.BillType,
			"action", p.Action, "calls", total, "failed", failed)
	} else {
		b.log.Info("circuit breaker: closed, row back", "id", p.ID, "provider", p.Name, "bill_type", p.BillType, "action", p.Action)
	}
	if b.onTransition != nil {
		b.onTransition(p, to)
	}
}

// built reports whether b was made by NewBreaker (every field that Report and Demoted call is set).
func (b *Breaker) built() bool {
	return b != nil && b.settings != nil && b.demote != nil && b.log != nil && b.now != nil && b.spawn != nil
}

// row is the state of the row id (a new one on the first use).
func (b *Breaker) row(id int64) *breakerRow {
	b.rowsMu.RLock()
	r := b.rows[id]
	b.rowsMu.RUnlock()
	if r != nil {
		return r
	}
	b.rowsMu.Lock()
	defer b.rowsMu.Unlock()
	if b.rows == nil {
		b.rows = map[int64]*breakerRow{} // a zero Breaker
	}
	if r = b.rows[id]; r == nil {
		r = &breakerRow{}
		b.rows[id] = r
	}
	return r
}

// add counts one call at now and returns the counts of the window. The caller holds mu and has set
// window.
func (r *breakerRow) add(now time.Time, failed bool) (total, bad int) {
	width := int64(r.window / breakerBuckets)
	if width < 1 {
		width = 1
	}
	epoch := now.UnixNano() / width
	slot := &r.buckets[epoch%breakerBuckets]
	if slot.epoch != epoch {
		*slot = breakerBucket{epoch: epoch}
	}
	slot.total++
	if failed {
		slot.failed++
	}
	for _, bk := range r.buckets {
		if bk.epoch > epoch-breakerBuckets && bk.epoch <= epoch {
			total += bk.total
			bad += bk.failed
		}
	}
	return total, bad
}

// reset empties the window. The caller holds mu.
func (r *breakerRow) reset() {
	r.buckets = [breakerBuckets]breakerBucket{}
}
