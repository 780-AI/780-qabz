package provider

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Task 060: the circuit breaker.

// breakerRig is a breaker with a test clock, a sync spawn and a recorded Demote. demoteErr is the
// error of each Demote; with nil the Demote calls Demoted(true) as the Provider Switcher does.
type breakerRig struct {
	b         *Breaker
	log       *fakeLog
	now       time.Time
	settings  BreakerSettings
	demoteErr error
	demotes   []string // id/period
	trans     []string // provider/to
	deferred  []func() // the Demote jobs when hold is true
	hold      bool
}

var cbRow = Provider{ID: 7, Name: "qabzino", BillType: "water", Action: "inquiry"}

func newBreakerRig(t *testing.T) *breakerRig {
	t.Helper()
	r := &breakerRig{log: &fakeLog{}, now: time.Unix(1_800_000_000, 0),
		settings: BreakerSettings{Enabled: true, FailureRatio: 30, Window: 2 * time.Minute, MinCalls: 20, OpenTime: time.Minute}}
	b, err := NewBreaker(BreakerOptions{
		Settings: func(Provider) BreakerSettings { return r.settings },
		Demote: func(id int64, period time.Duration) error {
			r.demotes = append(r.demotes, time.Duration(id).String()+"/"+period.String())
			if r.demoteErr == nil {
				r.b.Demoted(cbRow, true)
			}
			return r.demoteErr
		},
		Logger:       r.log,
		OnTransition: func(p Provider, to string) { r.trans = append(r.trans, p.Name+"/"+to) },
	})
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	b.now = func() time.Time { return r.now }
	b.spawn = func(job func()) {
		if r.hold {
			r.deferred = append(r.deferred, job)
			return
		}
		job()
	}
	r.b = b
	return r
}

// report reports ok good calls, then failed failed calls of cbRow.
func (r *breakerRig) report(ok, failed int) {
	for range ok {
		r.b.Report(cbRow, false)
	}
	for range failed {
		r.b.Report(cbRow, true)
	}
}

func (r *breakerRig) open() bool {
	row := r.b.row(cbRow.ID)
	row.mu.Lock()
	defer row.mu.Unlock()
	return row.open
}

// TestBreakerThreshold: the row opens only with min_calls calls or more and failure_ratio percent or
// more of them failed: 29 % does not open, 30 % opens; below min_calls nothing opens even at 100 %.
func TestBreakerThreshold(t *testing.T) {
	for _, tc := range []struct {
		name       string
		minCalls   int
		ok, failed int
		open       bool
	}{
		{"below min_calls", 20, 0, 19, false},
		{"at min_calls", 20, 0, 20, true},
		{"29 percent", 100, 71, 29, false},
		{"30 percent", 100, 70, 30, true},
		{"100 percent of 1", 1, 0, 1, true},
		{"only good calls", 1, 50, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newBreakerRig(t)
			r.settings.MinCalls = tc.minCalls
			r.report(tc.ok, tc.failed)
			assert.Equal(t, tc.open, r.open())
			if tc.open {
				assert.Equal(t, []string{"7ns/1m0s"}, r.demotes, "one Demote with open_time")
				assert.Equal(t, []string{"qabzino/open"}, r.trans)
				lines := r.log.byLevel("info")
				if assert.Len(t, lines, 1) {
					assert.Equal(t, "circuit breaker: open, row demoted", lines[0].msg)
					assert.Equal(t, tc.ok+tc.failed, lines[0].kv["calls"])
					assert.Equal(t, tc.failed, lines[0].kv["failed"])
					assert.Equal(t, int64(7), lines[0].kv["id"])
				}
			} else {
				assert.Empty(t, r.demotes)
				assert.Empty(t, r.trans)
			}
		})
	}
}

// TestBreakerWindow: calls older than the window leave it (in steps of window/20); a change of the
// window starts the count new.
func TestBreakerWindow(t *testing.T) {
	r := newBreakerRig(t)
	r.report(0, 19)
	r.now = r.now.Add(2*time.Minute + 6*time.Second) // past the window and its last bucket
	r.report(0, 1)
	assert.False(t, r.open(), "the old 19 left the window")
	r.report(0, 18)
	assert.False(t, r.open())
	r.now = r.now.Add(90 * time.Second) // still in the window
	r.report(0, 1)
	assert.True(t, r.open(), "20 in the window")

	r = newBreakerRig(t)
	r.report(0, 19)
	r.settings.Window = 5 * time.Minute
	r.report(0, 1)
	assert.False(t, r.open(), "a new window starts the count new")
}

// TestBreakerOpenAndClose: an open row counts nothing; a failed trial (no Demoted call) keeps it
// open; Demoted(false) closes it, writes one line and the count starts new; a second Demoted with
// the same state changes nothing.
func TestBreakerOpenAndClose(t *testing.T) {
	r := newBreakerRig(t)
	r.report(0, 20)
	assert.True(t, r.open())
	r.report(0, 100)
	assert.Len(t, r.demotes, 1, "no count while open")
	r.b.Demoted(cbRow, true)
	assert.Equal(t, []string{"qabzino/open"}, r.trans, "the same state")
	r.b.Demoted(cbRow, false)
	assert.False(t, r.open())
	assert.Equal(t, []string{"qabzino/open", "qabzino/closed"}, r.trans)
	assert.Equal(t, 1, countMsg(r.log, "info", "circuit breaker: closed, row back"))
	r.b.Demoted(cbRow, false)
	assert.Len(t, r.trans, 2)
	r.report(0, 19)
	assert.False(t, r.open(), "the count started new")
	r.report(0, 1)
	assert.True(t, r.open())
	assert.Len(t, r.demotes, 2)
}

// TestBreakerNoNext (decision 7): after a Demote with no next available row the row counts nothing
// for open_time, then starts new.
func TestBreakerNoNext(t *testing.T) {
	r := newBreakerRig(t)
	r.demoteErr = fmt.Errorf("wrap: %w", ErrNoNextProvider)
	r.report(0, 20)
	assert.False(t, r.open())
	assert.Len(t, r.demotes, 1)
	r.report(0, 100)
	assert.Len(t, r.demotes, 1, "paused")
	r.now = r.now.Add(time.Minute)
	r.report(0, 19)
	assert.Len(t, r.demotes, 1, "a new count")
	r.report(0, 1)
	assert.Len(t, r.demotes, 2)
	assert.Empty(t, r.trans)
}

// TestBreakerDemoteFails: a Demote that changes nothing (an unknown, disabled or demoted row: nil
// with no Demoted call, or another error) starts the count new with no pause.
func TestBreakerDemoteFails(t *testing.T) {
	for name, err := range map[string]error{"unknown": ErrUnknownProvider, "bad period": ErrBadPeriod} {
		r := newBreakerRig(t)
		r.demoteErr = err
		r.report(0, 20)
		r.report(0, 19)
		assert.Len(t, r.demotes, 1, name)
		r.report(0, 1)
		assert.Len(t, r.demotes, 2, name)
	}
}

// TestBreakerOneDemoteAtATime: while the Demote of a row runs, its reports are not counted and no
// second Demote starts.
func TestBreakerOneDemoteAtATime(t *testing.T) {
	r := newBreakerRig(t)
	r.hold = true
	r.report(0, 40)
	if assert.Len(t, r.deferred, 1) {
		r.deferred[0]()
	}
	assert.Len(t, r.demotes, 1)
	assert.True(t, r.open())
}

// TestBreakerOff: enabled false counts nothing and demotes nothing (the behavior before task 060); a
// hot change to true counts from then on; invalid settings count nothing.
func TestBreakerOff(t *testing.T) {
	r := newBreakerRig(t)
	r.settings.Enabled = false
	r.report(0, 100)
	assert.Empty(t, r.demotes)
	r.settings.Enabled = true
	r.report(0, 19)
	assert.False(t, r.open(), "the calls while off were not counted")
	r.report(0, 1)
	assert.True(t, r.open())

	r = newBreakerRig(t)
	r.settings.MinCalls = 0
	r.report(0, 100)
	assert.Empty(t, r.demotes, "invalid settings")
}

// TestBreakerRowsApart: each row has its own count.
func TestBreakerRowsApart(t *testing.T) {
	r := newBreakerRig(t)
	other := Provider{ID: 8, Name: "tavanir", BillType: "water", Action: "inquiry"}
	r.report(0, 19)
	for range 19 {
		r.b.Report(other, true)
	}
	assert.Empty(t, r.demotes)
}

// TestNewBreakerChecks: the required options; a nil or zero Breaker does nothing.
func TestNewBreakerChecks(t *testing.T) {
	set := func(Provider) BreakerSettings { return BreakerSettings{} }
	dem := func(int64, time.Duration) error { return nil }
	for name, o := range map[string]BreakerOptions{
		"settings": {Demote: dem, Logger: &fakeLog{}},
		"demote":   {Settings: set, Logger: &fakeLog{}},
		"logger":   {Settings: set, Demote: dem},
	} {
		_, err := NewBreaker(o)
		assert.ErrorContains(t, err, "circuit breaker: "+name, name)
	}
	var nilB *Breaker
	assert.NotPanics(t, func() {
		nilB.Report(cbRow, true)
		nilB.Demoted(cbRow, true)
		(&Breaker{}).Report(cbRow, true)
		(&Breaker{}).Demoted(cbRow, true)
	})
	zero := &Breaker{}
	assert.NotNil(t, zero.row(1), "a zero Breaker gets a row map")
}

// TestBreakerWithSwitcher: the breaker on a real ProvidersService, with many goroutines (run with
// -race): the first row of a group with two rows opens once, the requests go to the next row, and a
// return closes it; a group with one row is never demoted.
func TestBreakerWithSwitcher(t *testing.T) {
	log := &fakeLog{}
	var b *Breaker
	s := newTestService(t, &fakeRepo{rows: []Provider{
		{ID: 1, Name: "qabzino", BillType: "water", Action: "inquiry", Priority: 1, Status: StatusEnable},
		{ID: 2, Name: "tavanir", BillType: "water", Action: "inquiry", Priority: 2, Status: StatusEnable},
		{ID: 3, Name: "karashop", BillType: "tel", Action: "inquiry", Priority: 1, Status: StatusEnable},
	}}, log)
	s.onDemoted = func(p Provider, on bool) { b.Demoted(p, on) }
	if !assert.NoError(t, s.Load(context.Background())) {
		return
	}
	var err error
	var mu sync.Mutex
	var trans []string
	b, err = NewBreaker(BreakerOptions{
		Settings: func(Provider) BreakerSettings {
			return BreakerSettings{Enabled: true, FailureRatio: 50, Window: time.Minute, MinCalls: 10, OpenTime: time.Hour}
		},
		Demote: s.Demote, Logger: log,
		OnTransition: func(p Provider, to string) { mu.Lock(); trans = append(trans, p.Name+"/"+to); mu.Unlock() },
	})
	if !assert.NoError(t, err) {
		return
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				p, _ := s.PickProvider("water", ActionInquiry)
				b.Report(p, p.ID == 1)
				b.Report(Provider{ID: 3, Name: "karashop", BillType: "tel", Action: "inquiry"}, true)
			}
		}()
	}
	wg.Wait()
	assert.Eventually(t, func() bool {
		p, _ := s.PickProvider("water", ActionInquiry)
		return p.ID == 2
	}, 3*time.Second, time.Millisecond)
	mu.Lock()
	assert.Equal(t, []string{"qabzino/open"}, trans)
	mu.Unlock()
	assert.NoError(t, s.Disable(1), "a Disable ends the demotion")
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(trans) == 2 && trans[1] == "qabzino/closed"
	}, 3*time.Second, time.Millisecond)
	p, _ := s.PickProvider("tel", ActionInquiry)
	assert.False(t, p.Demoted, "one row: never demoted")
	assert.GreaterOrEqual(t, countMsg(log, "info", "providers: demote skipped, no next available provider"), 1)
}

// countMsg is the number of lines of level with the message msg.
func countMsg(l *fakeLog, level, msg string) int {
	n := 0
	for _, x := range l.byLevel(level) {
		if x.msg == msg {
			n++
		}
	}
	return n
}
