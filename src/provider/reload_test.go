package provider

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestFailedLoadKeepsLastList: a failed load keeps the last good list, writes exactly one error
// line with the cause and returns the error; the next good load applies.
func TestFailedLoadKeepsLastList(t *testing.T) {
	rows := seedRows(t)
	s, repo, log := loaded(t, rows)
	want := s.List()
	log.reset()
	boom := errors.New("connection refused")
	repo.set(nil, boom)
	assert.ErrorIs(t, s.Load(context.Background()), boom)
	assert.Equal(t, want, s.List())
	p, err := s.PickProvider("gas", "inquiry")
	assert.NoError(t, err)
	assert.Equal(t, int64(43), p.ID)
	errs := log.byLevel("error")
	if assert.Len(t, errs, 1) {
		assert.Equal(t, "providers: load failed; keeping the last list", errs[0].msg)
		assert.Equal(t, boom, errs[0].kv["error"])
	}
	assert.Empty(t, log.byLevel("debug"), "no order line after a failed load")

	repo.set(rows[:3], nil)
	assert.NoError(t, s.Load(context.Background()))
	assert.Len(t, s.List(), 3)
}

// TestOrderLogLine: the debug line has the group, its order as id:tmp_priority:status and the pick;
// the first load logs every group; a removed group logs an empty order and pick 0.
func TestOrderLogLine(t *testing.T) {
	rows := []Provider{
		{ID: 43, BillType: "gas", Action: "inquiry", Priority: 4, Status: StatusEnable},
		{ID: 99, BillType: "gas", Action: "inquiry", Priority: 4, Status: StatusDisable},
		{ID: 35, BillType: "gas", Action: "inquiry", Priority: 10, Status: StatusDisable},
		{ID: 2, BillType: "electricity", Action: "inquiry", Priority: 9, Status: StatusEnable},
	}
	s, repo, log := loaded(t, rows)
	lines := log.byLevel("debug")
	if !assert.Len(t, lines, 2, "the first load logs every group") {
		return
	}
	assert.Equal(t, map[string]any{"bill_type": "electricity", "action": "inquiry", "order": "2:9:enable", "pick": int64(2)}, lines[0].kv)
	assert.Equal(t, map[string]any{"bill_type": "gas", "action": "inquiry", "order": "43:4:enable,99:4:disable,35:10:disable", "pick": int64(43)}, lines[1].kv)

	log.reset()
	rows[2].Priority, rows[0].Status = 1, StatusDisable
	repo.set(rows[:3], nil)
	assert.NoError(t, s.Load(context.Background()))
	lines = log.byLevel("debug")
	if !assert.Len(t, lines, 2) {
		return
	}
	assert.Equal(t, map[string]any{"bill_type": "gas", "action": "inquiry", "order": "35:1:disable,43:4:disable,99:4:disable", "pick": int64(0)}, lines[0].kv)
	assert.Equal(t, map[string]any{"bill_type": "electricity", "action": "inquiry", "order": "", "pick": int64(0)}, lines[1].kv)
}

func TestLoadCallback(t *testing.T) {
	var mu sync.Mutex
	var results []string
	repo := &fakeRepo{}
	repo.set(moveRows(), nil)
	s, err := NewProvidersService(repo, Options{Interval: func() time.Duration { return time.Hour }, Logger: &fakeLog{},
		OnLoad: func(result string, d time.Duration) {
			mu.Lock()
			defer mu.Unlock()
			results = append(results, result)
			assert.GreaterOrEqual(t, d, time.Duration(0))
		}})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	assert.NoError(t, s.Load(context.Background()))
	repo.set(nil, errors.New("down"))
	assert.Error(t, s.Load(context.Background()))
	assert.Equal(t, []string{LoadOK, LoadError}, results)
}

// startService starts a service whose interval is read from *interval (nanoseconds) and whose
// minimum is 5 ms. It stops the service at the end of the test.
func startService(t *testing.T, repo *fakeRepo, log *fakeLog, interval *atomic.Int64) *ProvidersService {
	t.Helper()
	s, err := NewProvidersService(repo, Options{Interval: func() time.Duration { return time.Duration(interval.Load()) }, Logger: log})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		t.FailNow()
	}
	s.minInterval = 5 * time.Millisecond
	if !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { assert.NoError(t, s.Stop(context.Background())) })
	return s
}

// waitFor polls cond every millisecond for up to 3 s.
func waitFor(t *testing.T, cond func() bool, msg string) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return assert.Fail(t, "timeout: "+msg)
}

// TestStartLoadsBeforeReturn: the first load runs inside Start (operator decision 2).
func TestStartLoadsBeforeReturn(t *testing.T) {
	repo, log := &fakeRepo{}, &fakeLog{}
	repo.set(seedRows(t), nil)
	var interval atomic.Int64
	interval.Store(int64(time.Hour))
	s := startService(t, repo, log, &interval)
	assert.Len(t, s.List(), 45, "loaded when Start returns")
	assert.Equal(t, 1, repo.count())
	assert.ErrorContains(t, s.Start(context.Background()), "already started")
}

// TestStartWithFailedFirstLoad: a failed first load does not fail Start; it writes one error line,
// the list is empty (PickProvider gives ErrNoProvider), and the routine loads again at the interval.
func TestStartWithFailedFirstLoad(t *testing.T) {
	repo, log := &fakeRepo{}, &fakeLog{}
	repo.set(nil, errors.New("database down"))
	var interval atomic.Int64
	interval.Store(int64(20 * time.Millisecond))
	s := startService(t, repo, log, &interval)
	assert.Empty(t, s.List())
	_, err := s.PickProvider("gas", "inquiry")
	assert.ErrorIs(t, err, ErrNoProvider)
	assert.Len(t, log.byLevel("error"), 1)

	repo.set(seedRows(t), nil)
	if !waitFor(t, func() bool { return len(s.List()) == 45 }, "the routine loads again") {
		return
	}
	p, err := s.PickProvider("gas", "inquiry")
	assert.NoError(t, err)
	assert.Equal(t, int64(43), p.ID)
}

// TestIntervalIsReadEachTick: a changed interval applies from the next wait without restart, and a
// value below the minimum uses the minimum and writes one warn line.
func TestIntervalIsReadEachTick(t *testing.T) {
	repo, log := &fakeRepo{}, &fakeLog{}
	repo.set(moveRows(), nil)
	var interval atomic.Int64
	interval.Store(int64(10 * time.Millisecond))
	s := startService(t, repo, log, &interval)
	if !waitFor(t, func() bool { return repo.count() >= 4 }, "loads at 10 ms") {
		assert.NoError(t, s.Stop(context.Background()))
		return
	}

	interval.Store(int64(time.Hour))
	time.Sleep(30 * time.Millisecond) // the wait in progress ends; the next one is 1 h
	n := repo.count()
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, n, repo.count(), "no load during a 1 h interval")

	s2Repo, s2Log := &fakeRepo{}, &fakeLog{}
	s2Repo.set(moveRows(), nil)
	var low atomic.Int64
	low.Store(0) // below the 5 ms minimum
	s2 := startService(t, s2Repo, s2Log, &low)
	if !waitFor(t, func() bool { return s2Repo.count() >= 5 }, "loads at the minimum") {
		assert.NoError(t, s2.Stop(context.Background()))
		return
	}
	warns := s2Log.byLevel("warn")
	if assert.Len(t, warns, 1, "one warn line for the same low value") {
		assert.Equal(t, "providers: load interval below the minimum; using the minimum", warns[0].msg)
	}
}

func TestStopEndsTheRoutine(t *testing.T) {
	repo, log := &fakeRepo{}, &fakeLog{}
	repo.set(moveRows(), nil)
	s, err := NewProvidersService(repo, Options{Interval: func() time.Duration { return 5 * time.Millisecond }, Logger: log})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	s.minInterval = time.Millisecond
	if !assert.NoError(t, s.Start(context.Background())) {
		return
	}
	if !waitFor(t, func() bool { return repo.count() >= 3 }, "loads") {
		assert.NoError(t, s.Stop(context.Background()))
		return
	}
	assert.NoError(t, s.Stop(context.Background()))
	n := repo.count()
	time.Sleep(30 * time.Millisecond)
	assert.Equal(t, n, repo.count(), "no load after Stop")
	assert.NoError(t, s.Stop(context.Background()), "Stop twice is fine")

	// Stop with a held load: the canceled context ends the query.
	repo.mu.Lock()
	repo.block = make(chan struct{})
	repo.mu.Unlock()
	s2, err := NewProvidersService(repo, Options{Interval: func() time.Duration { return time.Millisecond }, Logger: log})
	if !assert.NoError(t, err) || !assert.NotNil(t, s2) {
		return
	}
	s2.minInterval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	assert.NoError(t, s2.Start(ctx), "the first load times out with ctx; Start does not fail")
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	assert.NoError(t, s2.Stop(stopCtx))

	close(repo.block)
}

// activeRecorder records the OnActiveCount values. Safe for concurrent use.
type activeRecorder struct {
	mu     sync.Mutex
	values []int
}

func (r *activeRecorder) set(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.values = append(r.values, n)
}

// last returns the last value and the number of calls.
func (r *activeRecorder) last() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.values) == 0 {
		return -1, 0
	}
	return r.values[len(r.values)-1], len(r.values)
}

// enabledIn counts the enabled providers of a list.
func enabledIn(ps []Provider) int {
	n := 0
	for _, p := range ps {
		if p.Enabled() {
			n++
		}
	}
	return n
}

// newActiveService returns a service on repo whose OnActiveCount values go to rec.
func newActiveService(t *testing.T, repo *fakeRepo, rec *activeRecorder) *ProvidersService {
	t.Helper()
	s, err := NewProvidersService(repo, Options{Interval: func() time.Duration { return time.Hour }, Logger: &fakeLog{},
		OnActiveCount: rec.set})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		t.FailNow()
	}
	return s
}

// TestActiveCount: OnActiveCount gives the number of enabled providers in all groups after a good
// load, after an Enable or Disable that changed the list; an unchanged Enable or Disable, an
// unknown id, a move and a failed load do not call it, so the last value stays; an empty list
// gives 0.
func TestActiveCount(t *testing.T) {
	repo, rec := &fakeRepo{}, &activeRecorder{}
	repo.set(moveRows(), nil)
	s := newActiveService(t, repo, rec)
	_, calls := rec.last()
	assert.Zero(t, calls, "no call before the first load")

	assert.NoError(t, s.Load(context.Background()))
	v, calls := rec.last()
	assert.Equal(t, 5, v, "50, 20, 40, 10 and 9 are enabled; 30 is not")
	assert.Equal(t, 1, calls)

	assert.NoError(t, s.Enable(30))
	v, calls = rec.last()
	assert.Equal(t, 6, v, "after Enable")
	assert.Equal(t, 2, calls)
	assert.NoError(t, s.Disable(9))
	v, calls = rec.last()
	assert.Equal(t, 5, v, "after Disable in another group")
	assert.Equal(t, 3, calls)

	assert.NoError(t, s.Enable(30))
	assert.NoError(t, s.Disable(9))
	assert.ErrorIs(t, s.Enable(77), ErrUnknownProvider)
	assert.NoError(t, s.MoveUp(20))
	assert.NoError(t, s.MoveDown(20))
	v, calls = rec.last()
	assert.Equal(t, 5, v, "unchanged Enable / Disable, unknown id and moves keep the value")
	assert.Equal(t, 3, calls, "and do not call OnActiveCount")

	repo.set(nil, errors.New("down"))
	assert.Error(t, s.Load(context.Background()))
	v, calls = rec.last()
	assert.Equal(t, 5, v, "a failed load keeps the value")
	assert.Equal(t, 3, calls)
	assert.Equal(t, v, enabledIn(s.List()), "the value equals the list")

	repo.set(moveRows(), nil)
	assert.NoError(t, s.Load(context.Background()))
	v, _ = rec.last()
	assert.Equal(t, 5, v, "the load resets the runtime status (database wins)")

	repo.set(nil, nil)
	assert.NoError(t, s.Load(context.Background()))
	v, _ = rec.last()
	assert.Equal(t, 0, v, "an empty list")

	rows := manyRows()
	repo.set(rows, nil)
	assert.NoError(t, s.Load(context.Background()))
	v, _ = rec.last()
	assert.Equal(t, enabledIn(rows), v, "the many fake rows")
	assert.Positive(t, v)
}

// TestActiveCountFollowsEveryChange: in a long random sequence of moves and toggles, and with
// concurrent writers, the last OnActiveCount value always equals the enabled count of the list.
func TestActiveCountFollowsEveryChange(t *testing.T) {
	rows := manyRows()
	repo, rec := &fakeRepo{}, &activeRecorder{}
	repo.set(rows, nil)
	s := newActiveService(t, repo, rec)
	if !assert.NoError(t, s.Load(context.Background())) {
		return
	}
	if !assert.NotEmpty(t, rows) {
		return
	}
	r := rand.New(rand.NewPCG(20, 1108))
	for step := range 2000 {
		id := rows[r.IntN(len(rows))].ID
		switch r.IntN(4) {
		case 0:
			_ = s.MoveUp(id)
		case 1:
			_ = s.MoveDown(id)
		case 2:
			_ = s.Enable(id)
		default:
			_ = s.Disable(id)
		}
		v, _ := rec.last()
		if !assert.Equal(t, enabledIn(s.List()), v, "step %d", step) {
			return
		}
	}

	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wr := rand.New(rand.NewPCG(uint64(w), 7))
			for range 300 {
				id := rows[wr.IntN(len(rows))].ID
				if wr.IntN(2) == 0 {
					_ = s.Enable(id)
				} else {
					_ = s.Disable(id)
				}
				if wr.IntN(50) == 0 {
					_ = s.Load(context.Background())
				}
			}
		}()
	}
	if !waitGroupOrFail(t, &wg, 10*time.Second) {
		return
	}
	v, _ := rec.last()
	assert.Equal(t, enabledIn(s.List()), v, "after concurrent writers")
}

// TestStopCancelIsNotAFailure: a load that Stop cancels (context.Canceled after the routine's
// context is done) writes no error line and makes no OnLoad call. Positive control: a Load whose
// caller canceled its own context is still logged and counted as a failure.
func TestStopCancelIsNotAFailure(t *testing.T) {
	repo, log := &fakeRepo{}, &fakeLog{}
	repo.set(moveRows(), nil)
	var mu sync.Mutex
	var results []string
	s, err := NewProvidersService(repo, Options{Interval: func() time.Duration { return time.Millisecond }, Logger: log,
		OnLoad: func(result string, _ time.Duration) {
			mu.Lock()
			defer mu.Unlock()
			results = append(results, result)
		}})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	s.minInterval = time.Millisecond
	if !assert.NoError(t, s.Start(context.Background())) {
		return
	}
	if !waitFor(t, func() bool { return repo.count() >= 2 }, "the routine loads") {
		assert.NoError(t, s.Stop(context.Background()))
		return
	}

	block := make(chan struct{})
	defer close(block)
	repo.mu.Lock()
	repo.block = block
	held := repo.calls
	repo.mu.Unlock()
	if !waitFor(t, func() bool { return repo.count() > held }, "a load is held in its query") {
		assert.NoError(t, s.Stop(context.Background()))
		return
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	assert.NoError(t, s.Stop(stopCtx))

	assert.Empty(t, log.byLevel("error"), "no error line for a shutdown cancel")
	mu.Lock()
	assert.NotContains(t, results, LoadError, "no error count for a shutdown cancel")
	assert.Contains(t, results, LoadOK)
	n := len(results)
	mu.Unlock()

	// Positive controls: the same canceled query through Load (no routine context) is a failure.
	ctx, cancelLoad := context.WithCancel(context.Background())
	cancelLoad()
	assert.ErrorIs(t, s.Load(ctx), context.Canceled)
	assert.Len(t, log.byLevel("error"), 1, "a Load canceled by its caller is logged")
	mu.Lock()
	if !assert.Len(t, results, n+1) {
		mu.Unlock()
		return
	}
	assert.Equal(t, LoadError, results[len(results)-1])
	mu.Unlock()
}

// loadRecorder records the OnLoad results. Safe for concurrent use.
type loadRecorder struct {
	mu      sync.Mutex
	results []string
}

func (r *loadRecorder) add(result string, _ time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result)
}

func (r *loadRecorder) get() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.results...)
}

// newLoadService returns a service on repo (1 h interval) whose OnLoad results go to rec.
func newLoadService(t *testing.T, repo *fakeRepo, log *fakeLog, rec *loadRecorder) *ProvidersService {
	t.Helper()
	s, err := NewProvidersService(repo, Options{Interval: func() time.Duration { return time.Hour }, Logger: log, OnLoad: rec.add})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		t.FailNow()
	}
	return s
}

// TestLoadTimeout: a load of Start (and of the reload routine) that takes longer than the load
// timeout fails with context.DeadlineExceeded: exactly one error line with the cause, one
// result="error" count, and the last good list stays. The production timeout is the loadTimeout
// constant (10 s).
func TestLoadTimeout(t *testing.T) {
	repo, log, rec := &fakeRepo{}, &fakeLog{}, &loadRecorder{}
	repo.set(moveRows(), nil)
	s := newLoadService(t, repo, log, rec)
	assert.Equal(t, loadTimeout, s.loadTimeout, "the constructor sets the constant")
	assert.Equal(t, 10*time.Second, loadTimeout)
	if !assert.NoError(t, s.Load(context.Background())) {
		return
	}
	want := s.List()

	block := make(chan struct{})
	defer close(block)
	repo.mu.Lock()
	repo.block = block // the query never ends by itself
	repo.mu.Unlock()
	s.loadTimeout = 30 * time.Millisecond
	began := time.Now()
	if !assert.NoError(t, s.Start(context.Background()), "a timed-out first load does not fail Start") {
		return
	}
	t.Cleanup(func() { assert.NoError(t, s.Stop(context.Background())) })
	assert.Less(t, time.Since(began), 2*time.Second, "the timeout ends the load")

	errs := log.byLevel("error")
	if assert.Len(t, errs, 1, "exactly one error line") {
		assert.Equal(t, "providers: load failed; keeping the last list", errs[0].msg)
		cause, _ := errs[0].kv["error"].(error)
		assert.ErrorIs(t, cause, context.DeadlineExceeded)
	}
	assert.Equal(t, []string{LoadOK, LoadError}, rec.get(), "one error count")
	assert.Equal(t, want, s.List(), "the last list stays")
}

// TestStartCancelIsNotAFailure: when the App.Start context itself is canceled during the first load
// (context.Canceled), the load writes no error line and makes no OnLoad call, and Start returns nil.
// Positive control: a first load that fails with context.Canceled while the Start context is not
// canceled (a cancel for another reason) is logged and counted.
func TestStartCancelIsNotAFailure(t *testing.T) {
	repo, log, rec := &fakeRepo{}, &fakeLog{}, &loadRecorder{}
	repo.set(moveRows(), nil)
	block := make(chan struct{})
	defer close(block)
	repo.mu.Lock()
	repo.block = block
	repo.mu.Unlock()
	s := newLoadService(t, repo, log, rec)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for repo.count() == 0 { // the first load is in its query
			time.Sleep(time.Millisecond)
		}
		cancel()
	}()
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()
	select {
	case err := <-done:
		assert.NoError(t, err, "Start returns as before")
	case <-time.After(3 * time.Second):
		assert.Fail(t, "Start did not return after its context was canceled")
		return
	}
	t.Cleanup(func() { assert.NoError(t, s.Stop(context.Background())) })
	assert.Equal(t, 1, repo.count())
	assert.Empty(t, log.byLevel("error"), "no error line for an aborted start")
	assert.Empty(t, rec.get(), "no OnLoad call for an aborted start")
	assert.Empty(t, s.List())

	// Positive control: context.Canceled from the query, but the Start context is not canceled.
	repo2, log2, rec2 := &fakeRepo{}, &fakeLog{}, &loadRecorder{}
	repo2.set(nil, context.Canceled)
	s2 := newLoadService(t, repo2, log2, rec2)
	if !assert.NoError(t, s2.Start(context.Background())) {
		return
	}
	t.Cleanup(func() { assert.NoError(t, s2.Stop(context.Background())) })
	assert.Len(t, log2.byLevel("error"), 1, "a load canceled for another reason is logged")
	assert.Equal(t, []string{LoadError}, rec2.get(), "and counted")
}
