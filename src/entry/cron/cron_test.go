package cron

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"qabz/common/monitoring/jaeger/jaegertest"
)

// fakeLock records the extends and the release of one lock.
type fakeLock struct {
	mu        sync.Mutex
	extends   []time.Duration
	released  int
	extendErr error
}

func (l *fakeLock) ExtendAtLeast(_ context.Context, ttl time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.extends = append(l.extends, ttl)
	return l.extendErr
}

func (l *fakeLock) Release(context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.released++
	return nil
}

// fakeLocker gives a lock (or none, or an error) and records the keys and TTLs.
type fakeLocker struct {
	mu    sync.Mutex
	keys  []string
	ttls  []time.Duration
	none  bool
	err   error
	locks []*fakeLock
}

func (f *fakeLocker) TryLock(_ context.Context, key string, ttl time.Duration) (Lock, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keys, f.ttls = append(f.keys, key), append(f.ttls, ttl)
	if f.err != nil {
		return nil, f.err
	}
	if f.none {
		return nil, nil
	}
	l := &fakeLock{}
	f.locks = append(f.locks, l)
	return l, nil
}

func (f *fakeLocker) released() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, l := range f.locks {
		l.mu.Lock()
		n += l.released
		l.mu.Unlock()
	}
	return n
}

// fakeCron counts its runs; run gives the behavior of each run.
type fakeCron struct {
	name     string
	interval time.Duration
	policy   LockPolicy
	mu       sync.Mutex
	runs     int
	run      func(ctx context.Context, ext Extender) error
}

func (c *fakeCron) Name() string            { return c.name }
func (c *fakeCron) Interval() time.Duration { return c.interval }
func (c *fakeCron) Lock() LockPolicy        { return c.policy }
func (c *fakeCron) Run(ctx context.Context, ext Extender) error {
	c.mu.Lock()
	c.runs++
	c.mu.Unlock()
	if c.run != nil {
		return c.run(ctx, ext)
	}
	return nil
}

func (c *fakeCron) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.runs
}

// recorder records the log lines and the metric callbacks.
type recorder struct {
	mu        sync.Mutex
	infos     []string
	errs      []string
	results   []string
	durations int
}

func (r *recorder) Info(msg string, _ ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.infos = append(r.infos, msg)
}

func (r *recorder) Error(msg string, _ ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, msg)
}

func (r *recorder) onRun(_, result string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result)
}

func (r *recorder) onDuration(string, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.durations++
}

func (r *recorder) snapshot() (errs, results []string, durations int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.errs...), append([]string(nil), r.results...), r.durations
}

func newRunner(t *testing.T, l Locker, rec *recorder, crons ...Cron) *Runner {
	t.Helper()
	r, err := NewRunner(Options{Crons: crons, Locker: l, Logger: rec, OnRun: rec.onRun, OnDuration: rec.onDuration})
	if !assert.NoError(t, err) || !assert.NotNil(t, r) {
		t.FailNow()
	}
	return r
}

// waitFor waits until cond is true or 5 s pass.
func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return assert.Fail(t, "the condition did not become true in time")
}

// TestLockPolicyTTL: bill's formula; with the operator values (chunk 100; 30s, 3ms, 3ms) the TTL
// is 60 min 0.66 s; no chunk or no sync operation gives 4 h 5 min and a base of 2 min.
func TestLockPolicyTTL(t *testing.T) {
	ttl, base := LockPolicy{Chunk: 100, SyncOperations: []time.Duration{30 * time.Second, 3 * time.Millisecond, 3 * time.Millisecond}}.TTL()
	assert.Equal(t, 33006600*time.Microsecond, base, "30.006 s x 1.1")
	assert.Equal(t, 60*time.Minute+660*time.Millisecond, ttl)

	for _, p := range []LockPolicy{{}, {Chunk: 100}, {SyncOperations: []time.Duration{time.Second}}, {Chunk: 5, SyncOperations: []time.Duration{0, 0}}} {
		ttl, base := p.TTL()
		assert.Equal(t, 4*time.Hour+5*time.Minute, ttl, "%+v", p)
		assert.Equal(t, 2*time.Minute, base, "%+v", p)
	}
}

func TestNewRunnerRules(t *testing.T) {
	ok := &fakeCron{name: "a", interval: time.Second}
	_, err := NewRunner(Options{Crons: []Cron{ok}})
	assert.ErrorContains(t, err, "locker is required")
	for _, crons := range [][]Cron{
		{nil},
		{&fakeCron{name: "", interval: time.Second}},
		{ok, &fakeCron{name: "a", interval: time.Second}},
		{&fakeCron{name: "b", interval: 0}},
	} {
		_, err := NewRunner(Options{Crons: crons, Locker: &fakeLocker{}})
		assert.Error(t, err)
	}
}

// TestRunsAtStartAndAtInterval: the first run at the start, then one at each interval; the lock key
// of bill; the TTL of the policy; each run releases its lock; Stop ends the loop.
func TestRunsAtStartAndAtInterval(t *testing.T) {
	l, rec := &fakeLocker{}, &recorder{}
	c := &fakeCron{name: "parsian-getstatus", interval: 20 * time.Millisecond, policy: LockPolicy{Chunk: 2, SyncOperations: []time.Duration{time.Second}}}
	r := newRunner(t, l, rec, c)
	if !assert.NoError(t, r.Start(context.Background())) {
		return
	}
	assert.Error(t, r.Start(context.Background()), "a second start")
	if !waitFor(t, func() bool { return c.count() >= 3 }) {
		assert.NoError(t, r.Stop(context.Background()))
		return
	}
	assert.NoError(t, r.Stop(context.Background()))
	assert.NoError(t, r.Stop(context.Background()), "a second stop does nothing")
	n := c.count()
	time.Sleep(60 * time.Millisecond)
	assert.Equal(t, n, c.count(), "no run after Stop")

	l.mu.Lock()
	if assert.NotEmpty(t, l.keys) && assert.NotEmpty(t, l.ttls) {
		assert.Equal(t, "crons_parsian-getstatus_lock1", l.keys[0])
		assert.Equal(t, 2*1100*time.Millisecond+5*time.Minute, l.ttls[0])
	}
	l.mu.Unlock()
	assert.Equal(t, n, l.released(), "every run releases its lock")
	errs, results, durations := rec.snapshot()
	assert.Empty(t, errs)
	assert.Equal(t, n, durations)
	for _, res := range results {
		assert.Equal(t, ResultDone, res)
	}
	assert.Contains(t, rec.infos, "cron: started")
}

// TestNoLock: another holder has the lock: no run, result no_lock, no line.
func TestNoLock(t *testing.T) {
	l, rec := &fakeLocker{none: true}, &recorder{}
	c := &fakeCron{name: "a", interval: time.Hour}
	r := newRunner(t, l, rec, c)
	r.tick(context.Background(), c)
	assert.Zero(t, c.count())
	errs, results, durations := rec.snapshot()
	assert.Empty(t, errs)
	assert.Equal(t, []string{ResultNoLock}, results)
	assert.Zero(t, durations)
}

// TestLockError: a failed lock: no run, result error, one error line; with a stopped ctx no line.
func TestLockError(t *testing.T) {
	l, rec := &fakeLocker{err: errors.New("redis down")}, &recorder{}
	c := &fakeCron{name: "a", interval: time.Hour}
	r := newRunner(t, l, rec, c)
	r.tick(context.Background(), c)
	assert.Zero(t, c.count())
	errs, results, _ := rec.snapshot()
	assert.Equal(t, []string{"cron: lock failed"}, errs)
	assert.Equal(t, []string{ResultError}, results)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.tick(ctx, c)
	errs, _, _ = rec.snapshot()
	assert.Len(t, errs, 1, "no line for a lock that the stop ended")
}

// TestRunErrorAndPanic: a run error and a panic give the result error and one line each, release
// the lock, and do not stop the next tick.
func TestRunErrorAndPanic(t *testing.T) {
	l, rec := &fakeLocker{}, &recorder{}
	calls := 0
	c := &fakeCron{name: "a", interval: time.Hour, run: func(context.Context, Extender) error {
		calls++
		switch calls {
		case 1:
			return errors.New("db down")
		case 2:
			panic("boom")
		}
		return nil
	}}
	r := newRunner(t, l, rec, c)
	for range 3 {
		r.tick(context.Background(), c)
	}
	errs, results, durations := rec.snapshot()
	assert.Equal(t, []string{"cron: run failed", "cron: panic"}, errs)
	assert.Equal(t, []string{ResultError, ResultError, ResultDone}, results)
	assert.Equal(t, 3, durations)
	assert.Equal(t, 3, l.released(), "the panic releases the lock too")
}

// TestCharge: Charge makes the TTL base x remaining (bill's ChargeTTL); no item is an error; a lost
// lock is the error of the run.
func TestCharge(t *testing.T) {
	l, rec := &fakeLocker{}, &recorder{}
	var chargeErrs []error
	c := &fakeCron{name: "a", interval: time.Hour, policy: LockPolicy{Chunk: 10, SyncOperations: []time.Duration{time.Second}},
		run: func(ctx context.Context, ext Extender) error {
			chargeErrs = append(chargeErrs, ext.Charge(ctx, 7), ext.Charge(ctx, 0))
			return nil
		}}
	r := newRunner(t, l, rec, c)
	r.tick(context.Background(), c)
	if assert.Len(t, chargeErrs, 2) && assert.Len(t, l.locks, 1) {
		assert.NoError(t, chargeErrs[0])
		assert.ErrorIs(t, chargeErrs[1], errNoMoreLeft)
		assert.Equal(t, []time.Duration{7 * 1100 * time.Millisecond}, l.locks[0].extends)
	}

	lost := &fakeLock{extendErr: errors.New("lost")}
	e := &extender{lock: lost, base: time.Second}
	assert.Error(t, e.Charge(context.Background(), 1))
}

// TestStopCancelsTheRun: Stop cancels the context of a running run and waits for it; with no
// error line (the stop is not a fault); a run that does not end makes Stop return when its ctx is
// done.
func TestStopCancelsTheRun(t *testing.T) {
	l, rec := &fakeLocker{}, &recorder{}
	started := make(chan struct{})
	c := &fakeCron{name: "a", interval: time.Hour, run: func(ctx context.Context, _ Extender) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	r := newRunner(t, l, rec, c)
	if !assert.NoError(t, r.Start(context.Background())) {
		return
	}
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	assert.NoError(t, r.Stop(ctx))
	errs, results, _ := rec.snapshot()
	assert.Empty(t, errs)
	assert.Equal(t, []string{ResultError}, results)
	assert.Equal(t, 1, l.released(), "the lock is released after the stop")

	release := make(chan struct{})
	started2 := make(chan struct{})
	stuck := &fakeCron{name: "b", interval: time.Hour, run: func(context.Context, Extender) error {
		close(started2)
		<-release
		return nil
	}}
	r2 := newRunner(t, &fakeLocker{}, &recorder{}, stuck)
	if !assert.NoError(t, r2.Start(context.Background())) {
		return
	}
	<-started2
	short, cancel2 := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel2()
	assert.ErrorContains(t, r2.Stop(short), "did not end in time")
	close(release)
}

// TestStartCtxDoesNotStopTheRuns: the runs do not end when the ctx of Start ends; only Stop ends them.
func TestStartCtxDoesNotStopTheRuns(t *testing.T) {
	c := &fakeCron{name: "a", interval: 10 * time.Millisecond}
	r := newRunner(t, &fakeLocker{}, &recorder{}, c)
	ctx, cancel := context.WithCancel(context.Background())
	err := r.Start(ctx)
	cancel()
	if !assert.NoError(t, err) {
		return
	}
	waitFor(t, func() bool { return c.count() >= 3 })
	assert.NoError(t, r.Stop(context.Background()))
	never := &Runner{}
	assert.NoError(t, never.Stop(context.Background()), "Stop before Start")
}

// TestRootSpan (conventions § 17; task 047): each tick is one root span "cron <name>" with the
// attributes cron and result, started before the lock; a step of the run is its child; a failed run
// and a failed lock have the status Error and an exception event with only the error type; a tick
// with no lock has the result no_lock and no error status.
func TestRootSpan(t *testing.T) {
	sr := jaegertest.Install(t)

	l, rec := &fakeLocker{}, &recorder{}
	fail := false
	c := &fakeCron{name: "a", interval: time.Hour, run: func(ctx context.Context, _ Extender) error {
		_, child := otel.Tracer("test").Start(ctx, "step")
		child.End()
		if fail {
			return errors.New("db down for 989121234567")
		}
		return nil
	}}
	r := newRunner(t, l, rec, c)
	r.tick(context.Background(), c)
	fail = true
	r.tick(context.Background(), c)
	l.mu.Lock()
	l.none = true // another holder
	l.mu.Unlock()
	r.tick(context.Background(), c)
	l.mu.Lock()
	l.none, l.err = false, errors.New("redis down")
	l.mu.Unlock()
	r.tick(context.Background(), c)

	spans := sr.Ended()
	if !assert.Len(t, spans, 6, "two runs (a root and a step each), a no-lock root, a lock-error root") {
		return
	}
	jaegertest.AssertClean(t, spans, "989121234567")
	for i, want := range []struct {
		result string
		code   codes.Code
	}{{ResultDone, codes.Unset}, {ResultError, codes.Error}} {
		step, root := spans[2*i], spans[2*i+1]
		assert.Equal(t, "step", step.Name())
		assert.Equal(t, "cron a", root.Name())
		assert.False(t, root.Parent().IsValid(), "a root span")
		assert.Equal(t, root.SpanContext().SpanID(), step.Parent().SpanID(), "the step is a child of the root")
		assert.Equal(t, want.code, root.Status().Code)
		assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("cron", "a"), attribute.String("result", want.result)}, root.Attributes())
	}
	if assert.Len(t, spans[3].Events(), 1) {
		assert.Equal(t, []attribute.KeyValue{attribute.String("exception.type", "*errors.errorString")}, spans[3].Events()[0].Attributes)
	}
	noLock, lockErr := spans[4], spans[5]
	assert.Equal(t, "cron a", noLock.Name())
	assert.Equal(t, codes.Unset, noLock.Status().Code)
	assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("cron", "a"), attribute.String("result", ResultNoLock)}, noLock.Attributes())
	assert.Equal(t, codes.Error, lockErr.Status().Code)
	assert.Equal(t, ResultError, lockErr.Status().Description)
	assert.Len(t, lockErr.Events(), 1)
}

// TestRunnerNames: the names of the workers, in order.
func TestRunnerNames(t *testing.T) {
	r, err := NewRunner(Options{Crons: []Cron{&fakeCron{name: "a", interval: time.Second}, &fakeCron{name: "b", interval: time.Second}}, Locker: &fakeLocker{}})
	if assert.NoError(t, err) && assert.NotNil(t, r) {
		assert.Equal(t, []string{"a", "b"}, r.Names())
	}
}
