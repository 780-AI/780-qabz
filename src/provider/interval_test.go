package provider

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Tests of operator decision 9: a hot change of the effective load interval stops the wait in
// progress; the next load runs at (start of the last load + new interval), at once when that time
// is past. Real short durations; every wait is bounded.

// fakeInterval is a hot-reloadable interval with a change notification, like config.Holder:
// changed returns a channel that the next set closes. Safe for concurrent use.
type fakeInterval struct {
	mu sync.Mutex
	d  time.Duration
	ch chan struct{}
}

func newFakeInterval(d time.Duration) *fakeInterval { return &fakeInterval{d: d} }

func (f *fakeInterval) get() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.d
}

func (f *fakeInterval) set(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.d = d
	if f.ch != nil {
		close(f.ch)
		f.ch = nil
	}
}

func (f *fakeInterval) changed() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ch == nil {
		f.ch = make(chan struct{})
	}
	return f.ch
}

// timedRepo records the start and the end time of each ListProviders call. Each call takes delay.
// Safe for concurrent use.
type timedRepo struct {
	mu     sync.Mutex
	rows   []Provider
	delay  time.Duration
	starts []time.Time
	ends   []time.Time
}

func (r *timedRepo) ListProviders(ctx context.Context) ([]Provider, error) {
	r.mu.Lock()
	r.starts = append(r.starts, time.Now())
	d := r.delay
	rows := append([]Provider(nil), r.rows...)
	r.mu.Unlock()
	if d > 0 {
		select {
		case <-time.After(d):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	r.mu.Lock()
	r.ends = append(r.ends, time.Now())
	r.mu.Unlock()
	return rows, nil
}

// startAt returns the start time of call i (0 is the first load, in Start).
func (r *timedRepo) startAt(i int) (time.Time, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i < 0 || i >= len(r.starts) {
		return time.Time{}, false
	}
	return r.starts[i], true
}

func (r *timedRepo) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.starts)
}

func (r *timedRepo) ended() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.ends)
}

// startWithInterval starts a service on repo whose interval is fi; wake=false gives no Changed
// (the wake-up off, the positive control). The service stops at the end of the test.
func startWithInterval(t *testing.T, repo Repository, fi *fakeInterval, wake bool, minInterval time.Duration, log *fakeLog) *ProvidersService {
	t.Helper()
	opts := Options{Interval: fi.get, Logger: log}
	if wake {
		opts.Changed = fi.changed
	}
	s, err := NewProvidersService(repo, opts)
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		t.FailNow()
	}
	s.minInterval = minInterval
	if !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { assert.NoError(t, s.Stop(context.Background())) })
	return s
}

// waitCount waits up to max for repo to have n calls and reports whether it did.
func waitCount(repo *timedRepo, n int, max time.Duration) bool {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if repo.count() >= n {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return repo.count() >= n
}

// longToShort runs a service at 1 h, changes the interval to 100 ms 20 ms after the first load and
// reports whether the second load came within 1 s, and its time after the first load.
func longToShort(t *testing.T, wake bool) (bool, time.Duration) {
	t.Helper()
	repo := &timedRepo{rows: moveRows()}
	fi := newFakeInterval(time.Hour)
	startWithInterval(t, repo, fi, wake, 10*time.Millisecond, &fakeLog{})
	time.Sleep(20 * time.Millisecond)
	fi.set(100 * time.Millisecond)
	if !waitCount(repo, 2, time.Second) {
		return false, 0
	}
	t0, ok0 := repo.startAt(0)
	t1, ok1 := repo.startAt(1)
	if !assert.True(t, ok0 && ok1) {
		return false, 0
	}
	return true, t1.Sub(t0)
}

// TestIntervalChangeLongToShort: from 1 h to 100 ms, the next load comes about 100 ms after the
// last load start, not after 1 h; then the loads go on every 100 ms. Positive control: with the
// wake-up off the same change gives no load within 1 s.
func TestIntervalChangeLongToShort(t *testing.T) {
	t.Run("wake-up on", func(t *testing.T) {
		came, after := longToShort(t, true)
		if !assert.True(t, came, "a load within 1 s after the change from 1 h to 100 ms") {
			return
		}
		assert.GreaterOrEqual(t, after, 90*time.Millisecond, "the load waits for (last start + 100 ms)")
		assert.Less(t, after, 400*time.Millisecond, "the load runs about 100 ms after the last start")
	})
	t.Run("positive control: wake-up off", func(t *testing.T) {
		came, _ := longToShort(t, false)
		assert.False(t, came, "without the wake-up the 1 h wait goes on")
	})
}

// TestIntervalChangeLoadsGoOn: after a change from 1 h to 50 ms the routine keeps the new interval.
func TestIntervalChangeLoadsGoOn(t *testing.T) {
	repo := &timedRepo{rows: moveRows()}
	fi := newFakeInterval(time.Hour)
	startWithInterval(t, repo, fi, true, 10*time.Millisecond, &fakeLog{})
	fi.set(50 * time.Millisecond)
	assert.True(t, waitCount(repo, 6, 2*time.Second), "5 loads at 50 ms within 2 s")
}

// TestIntervalChangePastDueLoadsAtOnce: when (last start + new interval) is past, the load runs at
// once, not after a new wait of the new interval.
func TestIntervalChangePastDueLoadsAtOnce(t *testing.T) {
	repo := &timedRepo{rows: moveRows()}
	fi := newFakeInterval(time.Hour)
	startWithInterval(t, repo, fi, true, 10*time.Millisecond, &fakeLog{})
	time.Sleep(400 * time.Millisecond)
	changed := time.Now()
	fi.set(300 * time.Millisecond) // due 100 ms ago
	if !assert.True(t, waitCount(repo, 2, time.Second), "a load after the change") {
		return
	}
	t1, ok := repo.startAt(1)
	if !assert.True(t, ok) {
		return
	}
	assert.Less(t, t1.Sub(changed), 150*time.Millisecond, "the load runs at once (a new wait would be 300 ms)")
}

// TestIntervalChangeWaitsFromLastStart: when (last start + new interval) is still ahead, the load
// runs at that time, not a whole new interval after the change.
func TestIntervalChangeWaitsFromLastStart(t *testing.T) {
	repo := &timedRepo{rows: moveRows()}
	fi := newFakeInterval(time.Hour)
	startWithInterval(t, repo, fi, true, 10*time.Millisecond, &fakeLog{})
	time.Sleep(400 * time.Millisecond)
	fi.set(800 * time.Millisecond) // due at first start + 800 ms; a new wait would end at + 1200 ms
	if !assert.True(t, waitCount(repo, 2, 2*time.Second), "a load after the change") {
		return
	}
	t0, ok0 := repo.startAt(0)
	t1, ok1 := repo.startAt(1)
	if !assert.True(t, ok0 && ok1) {
		return
	}
	after := t1.Sub(t0)
	assert.GreaterOrEqual(t, after, 790*time.Millisecond)
	assert.Less(t, after, 1050*time.Millisecond, "due at (last start + 800 ms), not 800 ms after the change")
}

// TestIntervalChangeShortToLong: from 200 ms to 900 ms right after a load, the next load comes at
// (that load start + 900 ms), not after 200 ms.
func TestIntervalChangeShortToLong(t *testing.T) {
	repo := &timedRepo{rows: moveRows()}
	fi := newFakeInterval(200 * time.Millisecond)
	startWithInterval(t, repo, fi, true, 10*time.Millisecond, &fakeLog{})
	if !assert.True(t, waitCount(repo, 2, 2*time.Second), "the first routine load") {
		return
	}
	fi.set(900 * time.Millisecond)
	l1, ok := repo.startAt(1)
	if !assert.True(t, ok) {
		return
	}
	time.Sleep(time.Until(l1.Add(650 * time.Millisecond)))
	assert.Equal(t, 2, repo.count(), "no load before (last start + 900 ms)")
	if !assert.True(t, waitCount(repo, 3, time.Second), "the load at (last start + 900 ms)") {
		return
	}
	l2, ok := repo.startAt(2)
	if assert.True(t, ok) {
		assert.GreaterOrEqual(t, l2.Sub(l1), 890*time.Millisecond)
		assert.Less(t, l2.Sub(l1), 1300*time.Millisecond)
	}
}

// TestIntervalChangeSameEffectiveValue: a change with the same effective interval (a new low value
// that also gives the minimum, the same value again, the minimum itself) restarts nothing and runs
// no extra load. Each load takes 300 ms, so the normal next load (wait after the load end) starts
// 700 ms after the last start; a restart would start it 400 ms after. A new low value still gets
// its one warn line.
func TestIntervalChangeSameEffectiveValue(t *testing.T) {
	repo := &timedRepo{rows: moveRows(), delay: 300 * time.Millisecond}
	fi := newFakeInterval(100 * time.Millisecond) // below the 400 ms minimum
	log := &fakeLog{}
	startWithInterval(t, repo, fi, true, 400*time.Millisecond, log)
	if !assert.True(t, waitCount(repo, 2, 3*time.Second), "the first routine load") {
		return
	}
	l1, ok := repo.startAt(1)
	if !assert.True(t, ok) {
		return
	}
	deadline := time.Now().Add(2 * time.Second)
	for repo.ended() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !assert.Equal(t, 2, repo.ended(), "the first routine load ends") {
		return
	}
	// The routine reads the value it is woken for; the pauses let it see each one.
	fi.set(200 * time.Millisecond) // a new low value: the minimum again
	time.Sleep(20 * time.Millisecond)
	fi.set(200 * time.Millisecond) // the same value again
	time.Sleep(20 * time.Millisecond)
	fi.set(400 * time.Millisecond) // the minimum itself
	time.Sleep(time.Until(l1.Add(600 * time.Millisecond)))
	assert.Equal(t, 2, repo.count(), "no restart and no extra load")
	if !assert.True(t, waitCount(repo, 3, 2*time.Second), "the normal next load") {
		return
	}
	l2, ok := repo.startAt(2)
	if assert.True(t, ok) {
		assert.GreaterOrEqual(t, l2.Sub(l1), 690*time.Millisecond, "the wait in progress went on")
	}
	warns := log.byLevel("warn")
	if assert.Len(t, warns, 2, "one warn line per new low value (100 ms, 200 ms)") {
		assert.Equal(t, 100.0, warns[0].kv["interval_ms"])
		assert.Equal(t, 200.0, warns[1].kv["interval_ms"])
	}
}

// TestIntervalChangeDoesNotInterruptALoad: a change during a load does not cancel it; the load
// ends good when its query returns.
func TestIntervalChangeDoesNotInterruptALoad(t *testing.T) {
	repo, log, rec := &fakeRepo{}, &fakeLog{}, &loadRecorder{}
	repo.set(moveRows(), nil)
	fi := newFakeInterval(20 * time.Millisecond)
	s, err := NewProvidersService(repo, Options{Interval: fi.get, Changed: fi.changed, Logger: log, OnLoad: rec.add})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	s.minInterval = 10 * time.Millisecond
	if !assert.NoError(t, s.Start(context.Background())) {
		return
	}
	block := make(chan struct{})
	repo.mu.Lock()
	repo.block = block
	held := repo.calls
	repo.mu.Unlock()
	if !waitFor(t, func() bool { return repo.count() > held }, "a load is held in its query") {
		close(block)
		assert.NoError(t, s.Stop(context.Background()))
		return
	}
	fi.set(time.Hour)
	fi.set(15 * time.Millisecond)
	fi.set(time.Hour)
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, held+1, repo.count(), "the held load is the only one in progress")
	repo.mu.Lock()
	repo.block = nil
	repo.mu.Unlock()
	close(block)
	ok := waitFor(t, func() bool { return len(rec.get()) == 2 }, "the held load ends")
	assert.NoError(t, s.Stop(context.Background()))
	if ok {
		assert.Equal(t, []string{LoadOK, LoadOK}, rec.get(), "the held load was not interrupted")
	}
	assert.Empty(t, log.byLevel("error"))
}

// TestStopDuringWaitAfterChange: Stop ends the routine promptly in a wait that a change started.
func TestStopDuringWaitAfterChange(t *testing.T) {
	repo := &timedRepo{rows: moveRows()}
	fi := newFakeInterval(time.Hour)
	s, err := NewProvidersService(repo, Options{Interval: fi.get, Changed: fi.changed, Logger: &fakeLog{}})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	s.minInterval = 10 * time.Millisecond
	if !assert.NoError(t, s.Start(context.Background())) {
		return
	}
	fi.set(2 * time.Hour)
	time.Sleep(20 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	begin := time.Now()
	assert.NoError(t, s.Stop(ctx))
	assert.Less(t, time.Since(begin), 500*time.Millisecond, "Stop is prompt")
	fi.set(10 * time.Millisecond) // a change after Stop starts nothing
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, repo.count(), "no load after Stop")
}

// TestConcurrentIntervalChanges (run with -race): many goroutines change the interval among long
// values while the routine runs; then the last value, 50 ms, is set. The routine ends on it: the
// loads come every 50 ms (a lost last value would leave a long wait).
func TestConcurrentIntervalChanges(t *testing.T) {
	repo := &timedRepo{rows: moveRows()}
	fi := newFakeInterval(time.Hour)
	startWithInterval(t, repo, fi, true, 10*time.Millisecond, &fakeLog{})
	long := []time.Duration{time.Hour, 2 * time.Hour, 30 * time.Minute, 45 * time.Minute}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				fi.set(long[(g+i)%len(long)])
			}
		}(g)
	}
	if !waitGroupOrFail(t, &wg, 5*time.Second) {
		return
	}
	// Loads that the changes started before the last value are past; count from here.
	time.Sleep(20 * time.Millisecond)
	n := repo.count()
	fi.set(50 * time.Millisecond)
	assert.True(t, waitCount(repo, n+5, 2*time.Second), "the routine ends on the last value (50 ms)")
}
