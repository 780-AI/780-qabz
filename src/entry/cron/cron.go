// Package cron is the shared worker runner (task 045; system.html "Cron Interface"; bill:
// pkg/cronTask/cron.go and pkg/lock/lock.go). Each worker runs at its interval, in one instance at a
// time (a Redis lock with a TTL of bill's formula), and a panic or the App stop never stops another
// worker or the App. The runner has no rule of one worker: a worker is a Cron.
package cron

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"qabz/common/monitoring/jaeger"
)

// Cron is one worker. Run gets an Extender: after each page of work it calls Charge with the
// number of items of the page, so a long run keeps its lock (bill: lock.Extender.ChargeTTL).
type Cron interface {
	// Name names the worker: the lock key, the metric label cron and the log lines.
	Name() string
	// Interval is the time between two runs. Positive.
	Interval() time.Duration
	// Lock is the lock TTL policy of a run (bill: chunk and syncOperations).
	Lock() LockPolicy
	Run(ctx context.Context, ext Extender) error
}

// Extender keeps the lock of a run.
type Extender interface {
	// Charge makes the TTL of the lock at least the time of remaining items (base × remaining,
	// bill: ChargeTTL). The lock is lost: an error.
	Charge(ctx context.Context, remaining int) error
}

// LockPolicy is what bill's lock TTL needs (pkg/lock: LockRequest).
type LockPolicy struct {
	Chunk          int
	SyncOperations []time.Duration
}

// The constants of bill's lock (pkg/lock/lock.go).
const (
	defaultLockTime  = 4 * time.Hour
	defaultMinTTL    = 2 * time.Minute
	bufferPercentage = 10
	extraLockTime    = 5 * time.Minute
)

// TTL returns the TTL of a new lock and the base time of one item (bill: calculator.initLockTime
// plus 5 minutes). With no chunk or no sync operation (or a sum of 0): 4 h + 5 min and a base of 2
// min. Else base = sum × 1.1 and the TTL = base × chunk + 5 min.
func (p LockPolicy) TTL() (ttl, base time.Duration) {
	var total time.Duration
	for _, d := range p.SyncOperations {
		total += d
	}
	if p.Chunk == 0 || len(p.SyncOperations) == 0 || total == 0 {
		return defaultLockTime + extraLockTime, defaultMinTTL
	}
	total += time.Duration(float64(total) * float64(bufferPercentage) / 100)
	return total*time.Duration(p.Chunk) + extraLockTime, total
}

// Locker takes the lock of a run (system/redis: the key gets database.redis.prefix).
type Locker interface {
	// TryLock takes the key for ttl. Another holder has it: a nil Lock and a nil error.
	TryLock(ctx context.Context, key string, ttl time.Duration) (Lock, error)
}

// Lock is a lock that this instance holds.
type Lock interface {
	// ExtendAtLeast makes the TTL ttl when less is left. Lost: an error.
	ExtendAtLeast(ctx context.Context, ttl time.Duration) error
	// Release deletes the key only when it is still this lock.
	Release(ctx context.Context) error
}

// Logger gets the lines of the runner.
type Logger interface {
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// Run results, for Options.OnRun (a fixed set: metric labels).
const (
	ResultDone   = "done"    // the run took the lock and ended with no error
	ResultNoLock = "no_lock" // another instance holds the lock: no run
	ResultError  = "error"   // a failed lock, a run error, a panic or a stop during the run
)

// releaseTimeout limits the delete of the lock after a run (also after the App stop).
const releaseTimeout = 5 * time.Second

// errNoMoreLeft is the error of Charge with no item (bill: lock.NoMoreLeftErr).
var errNoMoreLeft = errors.New("cron: no item left to charge")

// Options configure NewRunner.
type Options struct {
	Crons  []Cron
	Locker Locker
	// LockKey gives the lock key of a worker name (with no prefix: the Locker adds it). Nil:
	// "crons_<name>_lock1" (bill).
	LockKey func(name string) string
	// LockTime gives the lock_time of a worker for the start line only (bill logs it and does not
	// use it). Nil: not logged.
	LockTime func(name string) time.Duration
	Logger   Logger
	// OnRun gets the result of each tick (ResultDone, ResultNoLock, ResultError). Nil: nothing.
	OnRun func(name, result string)
	// OnDuration gets the duration of each run that took the lock. Nil: nothing.
	OnDuration func(name string, d time.Duration)
}

// Runner runs the workers. Start starts one goroutine for each worker; Stop cancels the runs and
// waits for them.
type Runner struct {
	crons      []Cron
	locker     Locker
	lockKey    func(string) string
	lockTime   func(string) time.Duration
	log        Logger
	onRun      func(string, string)
	onDuration func(string, time.Duration)
	now        func() time.Time

	mu      sync.Mutex
	cancel  context.CancelFunc
	started bool
	wg      sync.WaitGroup
}

// NewRunner checks the options and returns the runner. Every worker needs a unique, non-empty
// name and a positive interval.
func NewRunner(o Options) (*Runner, error) {
	if o.Locker == nil {
		return nil, errors.New("cron: locker is required")
	}
	seen := map[string]bool{}
	for _, c := range o.Crons {
		if c == nil {
			return nil, errors.New("cron: a nil worker")
		}
		if c.Name() == "" || seen[c.Name()] {
			return nil, fmt.Errorf("cron: worker name %q is empty or not unique", c.Name())
		}
		seen[c.Name()] = true
		if c.Interval() <= 0 {
			return nil, fmt.Errorf("cron: worker %s: the interval must be positive", c.Name())
		}
	}
	key := o.LockKey
	if key == nil {
		key = func(name string) string { return "crons_" + name + "_lock1" }
	}
	return &Runner{crons: o.Crons, locker: o.Locker, lockKey: key, lockTime: o.LockTime, log: o.Logger,
		onRun: o.OnRun, onDuration: o.OnDuration, now: time.Now}, nil
}

// Start starts the workers. Each one runs at once, then at each interval (bill: gocron v1 starts
// at once). The runs do not use ctx: only Stop ends them.
func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return errors.New("cron: already started")
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.cancel, r.started = cancel, true
	for _, c := range r.crons {
		if r.log != nil {
			kv := []any{"name", c.Name(), "interval", c.Interval().String()}
			if r.lockTime != nil {
				kv = append(kv, "lock_time", r.lockTime(c.Name()).String())
			}
			ttl, _ := c.Lock().TTL()
			r.log.Info("cron: started", append(kv, "lock_ttl", ttl.String())...)
		}
		r.wg.Add(1)
		go r.loop(runCtx, c)
	}
	return nil
}

// Stop cancels the runs and waits for them until ctx is done. It is safe to call more than once.
func (r *Runner) Stop(ctx context.Context) error {
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	r.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("cron: stop: the runs did not end in time: %w", ctx.Err())
	}
}

// loop runs c at once and at each tick until ctx is done. A run longer than the interval skips the
// ticks in between (one run at a time in one instance).
func (r *Runner) loop(ctx context.Context, c Cron) {
	defer r.wg.Done()
	r.tick(ctx, c)
	t := time.NewTicker(c.Interval())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if ctx.Err() == nil {
				r.tick(ctx, c)
			}
		}
	}
}

// tick takes the lock and runs c once. A panic is recovered (bill: wrapTask): one error line with
// the stack, the result error.
//
// @todo docs/this_vs_bill.md item 43: Qabz deletes its own lock after each run (bill never deletes
// it, so bill runs about once per lock TTL); the key has database.redis.prefix, not service.name.
func (r *Runner) tick(ctx context.Context, c Cron) {
	name := c.Name()
	result := ResultError
	// The root span of the tick (conventions § 17: a cron tick starts its own root, "cron <name>").
	// It starts before the lock (task 047), so the Redis lock command is in the tick trace too. The
	// steps of the worker are its children. no_lock is the normal result of the other instances: no
	// error status. This defer runs last, after the recover below has set the result.
	ctx, span := otel.Tracer("qabz/entry/cron").Start(ctx, "cron "+name, trace.WithNewRoot(),
		trace.WithAttributes(attribute.String("cron", name)))
	defer func() {
		span.SetAttributes(attribute.String("result", result))
		if result != ResultDone && result != ResultNoLock {
			jaeger.Fail(span, result, nil)
		}
		span.End()
	}()
	defer func() {
		if p := recover(); p != nil {
			result = ResultError
			if r.log != nil {
				r.log.Error("cron: panic", "name", name, "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
			}
		}
		if r.onRun != nil {
			r.onRun(name, result)
		}
	}()

	ttl, base := c.Lock().TTL()
	lock, err := r.locker.TryLock(ctx, r.lockKey(name), ttl)
	if err != nil {
		jaeger.Fail(span, ResultError, err)
		if ctx.Err() == nil && r.log != nil {
			r.log.Error("cron: lock failed", "name", name, "error", err)
		}
		return
	}
	if lock == nil {
		result = ResultNoLock
		return
	}
	start := r.now()
	defer func() {
		if r.onDuration != nil {
			r.onDuration(name, r.now().Sub(start))
		}
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), releaseTimeout)
		defer cancel()
		if err := lock.Release(rctx); err != nil && r.log != nil {
			r.log.Error("cron: release failed", "name", name, "error", err)
		}
	}()
	if err := c.Run(ctx, &extender{lock: lock, base: base}); err != nil {
		jaeger.Fail(span, ResultError, err)
		if ctx.Err() == nil && r.log != nil {
			r.log.Error("cron: run failed", "name", name, "error", err)
		}
		return
	}
	result = ResultDone
}

// extender is the Extender of one run.
type extender struct {
	lock Lock
	base time.Duration
}

// Charge makes the TTL at least base × remaining (bill: ChargeTTL; no item: an error).
func (e *extender) Charge(ctx context.Context, remaining int) error {
	if remaining <= 0 {
		return errNoMoreLeft
	}
	return e.lock.ExtendAtLeast(ctx, e.base*time.Duration(remaining))
}
