package provider

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// switchRec records the callbacks of the Provider Switcher. Safe for concurrent use.
type switchRec struct {
	mu       sync.Mutex
	switches []string // bill_type/action/result
	trials   []string
	demoted  map[int64]bool
}

func (r *switchRec) attach(s *ProvidersService) {
	r.demoted = map[int64]bool{}
	s.onSwitch = func(b, a, res string) { r.mu.Lock(); r.switches = append(r.switches, b+"/"+a+"/"+res); r.mu.Unlock() }
	s.onTrial = func(b, a, res string) { r.mu.Lock(); r.trials = append(r.trials, b+"/"+a+"/"+res); r.mu.Unlock() }
	s.onDemoted = func(p Provider, on bool) { r.mu.Lock(); r.demoted[p.ID] = on; r.mu.Unlock() }
}

func (r *switchRec) get() (switches, trials []string, demoted map[int64]bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := map[int64]bool{}
	for k, v := range r.demoted {
		d[k] = v
	}
	return append([]string(nil), r.switches...), append([]string(nil), r.trials...), d
}

// fakeClock is a clock that a test moves. Safe for concurrent use.
type fakeClock struct{ ns atomic.Int64 }

func newClock() *fakeClock {
	c := &fakeClock{}
	c.ns.Store(time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC).UnixNano())
	return c
}
func (c *fakeClock) now() time.Time      { return time.Unix(0, c.ns.Load()) }
func (c *fakeClock) add(d time.Duration) { c.ns.Add(int64(d)) }
func row(id int64, name, billType, action string, prio int, status string) Provider {
	return Provider{ID: id, Name: name, BillType: billType, Action: action, Priority: prio, Status: status}
}

// waterRows are two enabled rows of (water, inquiry): 1 qabzino first, 2 parsian next.
func waterRows() []Provider {
	return []Provider{row(1, "qabzino", "water", "inquiry", 1, StatusEnable), row(2, "parsian", "water", "inquiry", 2, StatusEnable)}
}

// switcher returns a loaded service with a fake clock and a recorder.
func switcher(t *testing.T, rows []Provider) (*ProvidersService, *fakeRepo, *fakeLog, *fakeClock, *switchRec) {
	t.Helper()
	s, repo, log := loaded(t, rows)
	c, rec := newClock(), &switchRec{}
	s.now = c.now
	rec.attach(s)
	return s, repo, log, c, rec
}

func pickID(t *testing.T, s *ProvidersService, billType, action string) int64 {
	t.Helper()
	p, err := s.PickProvider(billType, action)
	assert.NoError(t, err)
	return p.ID
}

// TestDemoteMovesToEnd: a demoted row goes to the end of its group for the period; the pick is the
// next row; Priority and TmpPriority do not change; one info line, one switch, the gauge on.
func TestDemoteMovesToEnd(t *testing.T) {
	s, _, log, _, rec := switcher(t, waterRows())
	assert.NoError(t, s.Demote(1, time.Minute))
	assert.Equal(t, int64(2), pickID(t, s, "water", "inquiry"))
	g := s.Group("water", "inquiry")
	if assert.Equal(t, []int64{2, 1}, ids(g)) {
		assert.True(t, g[1].Demoted)
		assert.False(t, g[0].Demoted)
		assert.Equal(t, 1, g[1].Priority, "the stored priority does not change")
		assert.Equal(t, 1, g[1].TmpPriority)
	}
	sw, tr, dem := rec.get()
	assert.Equal(t, []string{"water/inquiry/demoted"}, sw)
	assert.Empty(t, tr)
	assert.Equal(t, map[int64]bool{1: true}, dem)
	lines := log.byLevel("info")
	if assert.Len(t, lines, 1) {
		assert.Equal(t, "providers: row demoted", lines[0].msg)
		assert.Equal(t, int64(1), lines[0].kv["id"])
		assert.Equal(t, int64(2), lines[0].kv["next"])
		assert.Equal(t, float64(60000), lines[0].kv["period_ms"])
	}
	// Demote again: no change, nothing counted.
	assert.NoError(t, s.Demote(1, time.Hour))
	sw, _, _ = rec.get()
	assert.Len(t, sw, 1)
}

// TestDemoteNoNext: a group with no next available row is never demoted: one row; the other row
// disabled; the other row with no adapter (Available false); the other row demoted already (both
// rows failed). A second enabled row that a load adds makes the first row demotable.
func TestDemoteNoNext(t *testing.T) {
	check := func(t *testing.T, s *ProvidersService, rec *switchRec, log *fakeLog, want int64) {
		t.Helper()
		assert.ErrorIs(t, s.Demote(1, time.Minute), ErrNoNextProvider)
		assert.Equal(t, want, pickID(t, s, "water", "inquiry"), "the requests keep going to the row")
		sw, _, _ := rec.get()
		assert.Contains(t, sw, "water/inquiry/no_next")
		lines := log.byLevel("info")
		if assert.NotEmpty(t, lines) {
			assert.Equal(t, "providers: demote skipped, no next available provider", lines[len(lines)-1].msg)
		}
	}
	t.Run("one row", func(t *testing.T) {
		s, repo, log, _, rec := switcher(t, waterRows()[:1])
		check(t, s, rec, log, 1)
		repo.set(waterRows(), nil) // the operator adds a second row
		assert.NoError(t, s.Load(context.Background()))
		assert.NoError(t, s.Demote(1, time.Minute))
		assert.Equal(t, int64(2), pickID(t, s, "water", "inquiry"))
	})
	t.Run("other disabled", func(t *testing.T) {
		rows := waterRows()
		rows[1].Status = StatusDisable
		s, _, log, _, rec := switcher(t, rows)
		check(t, s, rec, log, 1)
	})
	t.Run("other has no adapter", func(t *testing.T) {
		s, _, log, _, rec := switcher(t, waterRows())
		s.available = func(p Provider) bool { return p.Name != "parsian" }
		check(t, s, rec, log, 1)
	})
	t.Run("both failed", func(t *testing.T) {
		s, _, log, _, rec := switcher(t, waterRows())
		assert.NoError(t, s.Demote(2, time.Minute))
		check(t, s, rec, log, 1)
		_, _, dem := rec.get()
		assert.Equal(t, map[int64]bool{2: true}, dem, "the first row is not demoted")
	})
}

// TestDemoteErrors: a period of 0 or less, an unknown id, a disabled row.
func TestDemoteErrors(t *testing.T) {
	rows := append(waterRows(), row(3, "tavanir", "water", "inquiry", 3, StatusDisable))
	s, _, _, _, rec := switcher(t, rows)
	assert.ErrorIs(t, s.Demote(1, 0), ErrBadPeriod)
	assert.ErrorIs(t, s.Demote(1, -time.Second), ErrBadPeriod)
	assert.ErrorIs(t, s.Demote(99, time.Minute), ErrUnknownProvider)
	assert.NoError(t, s.Demote(3, time.Minute), "a disabled row: no change")
	sw, _, _ := rec.get()
	assert.Empty(t, sw)
	assert.Equal(t, []int64{1, 2, 3}, ids(s.Group("water", "inquiry")))
}

// TestLoadKeepsDemotion: a load keeps the demotion of a row that stays enabled (the order of the
// load is the order of the priority, with the demoted row last); a load that disables, removes or
// moves the row clears it; a runtime Disable clears it too.
func TestLoadKeepsDemotion(t *testing.T) {
	s, repo, log, _, rec := switcher(t, waterRows())
	assert.NoError(t, s.Demote(1, time.Minute))
	assert.NoError(t, s.Load(context.Background()))
	assert.Equal(t, []int64{2, 1}, ids(s.Group("water", "inquiry")), "kept through the load")
	assert.Equal(t, int64(2), pickID(t, s, "water", "inquiry"))

	for name, change := range map[string]func([]Provider) []Provider{
		"disabled": func(r []Provider) []Provider { r[0].Status = StatusDisable; return r },
		"removed":  func(r []Provider) []Provider { return r[1:] },
		"moved":    func(r []Provider) []Provider { r[0].BillType = "gas"; return r },
	} {
		t.Run(name, func(t *testing.T) {
			repo.set(waterRows(), nil)
			assert.NoError(t, s.Load(context.Background()))
			assert.NoError(t, s.Demote(1, time.Minute))
			log.reset()
			repo.set(change(waterRows()), nil)
			assert.NoError(t, s.Load(context.Background()))
			_, _, dem := rec.get()
			assert.False(t, dem[1], "the gauge is off")
			for _, p := range s.List() {
				assert.False(t, p.Demoted, p.ID)
			}
			assert.Contains(t, msgs(log.byLevel("info")), "providers: demotion cleared, the row is removed or disabled")
			repo.set(waterRows(), nil)
			assert.NoError(t, s.Load(context.Background()))
			assert.Equal(t, int64(1), pickID(t, s, "water", "inquiry"), "a new load gives the original order")
		})
	}
	assert.NoError(t, s.Demote(1, time.Minute))
	assert.NoError(t, s.Disable(1))
	assert.NoError(t, s.Enable(1))
	assert.Equal(t, int64(1), pickID(t, s, "water", "inquiry"), "Disable cleared the demotion")
	sw, _, _ := rec.get()
	assert.Equal(t, "water/inquiry/cleared", sw[len(sw)-1])
}

func msgs(lines []logLine) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.msg)
	}
	return out
}

// TestTrialGood: before the period ends no trial; after it one PickInquiry gets the demoted row as
// a trial, the others get the next row; a good trial puts the row back.
func TestTrialGood(t *testing.T) {
	s, _, log, clock, rec := switcher(t, waterRows())
	assert.NoError(t, s.Demote(1, time.Minute))
	p, trial, err := s.PickInquiry("water")
	assert.NoError(t, err)
	assert.Equal(t, int64(2), p.ID)
	assert.False(t, trial.Active(), "the period is not over")

	clock.add(time.Minute)
	p, trial, err = s.PickInquiry("water")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), p.ID, "the trial goes to the demoted row")
	assert.True(t, trial.Active())
	p2, t2, _ := s.PickInquiry("water")
	assert.Equal(t, int64(2), p2.ID, "one trial at a time")
	assert.False(t, t2.Active())

	trial.Done(TrialOK)
	assert.Equal(t, []int64{1, 2}, ids(s.Group("water", "inquiry")), "back in its original place")
	assert.False(t, s.Group("water", "inquiry")[0].Demoted)
	p, trial, _ = s.PickInquiry("water")
	assert.Equal(t, int64(1), p.ID)
	assert.False(t, trial.Active())
	trial.Done(TrialFailed) // the zero Trial does nothing
	sw, tr, dem := rec.get()
	assert.Equal(t, []string{"water/inquiry/demoted", "water/inquiry/restored"}, sw)
	assert.Equal(t, []string{"water/inquiry/ok"}, tr)
	assert.False(t, dem[1])
	assert.Contains(t, msgs(log.byLevel("info")), "providers: trial ok, row back in its place")
}

// TestTrialFailed: a failed trial demotes the row again for a new period; the next trial comes
// only after that period.
func TestTrialFailed(t *testing.T) {
	s, _, log, clock, rec := switcher(t, waterRows())
	assert.NoError(t, s.Demote(1, time.Minute))
	clock.add(time.Minute)
	_, trial, _ := s.PickInquiry("water")
	if !assert.True(t, trial.Active()) {
		return
	}
	trial.Done(TrialFailed)
	assert.Equal(t, []int64{2, 1}, ids(s.Group("water", "inquiry")))
	clock.add(59 * time.Second)
	p, tr2, _ := s.PickInquiry("water")
	assert.Equal(t, int64(2), p.ID)
	assert.False(t, tr2.Active(), "a new period")
	clock.add(time.Second)
	p, tr2, _ = s.PickInquiry("water")
	assert.Equal(t, int64(1), p.ID)
	assert.True(t, tr2.Active())
	trial.Done(TrialOK) // the old trial: its demotion changed, nothing happens
	assert.Equal(t, []int64{2, 1}, ids(s.Group("water", "inquiry")))
	sw, tr, dem := rec.get()
	assert.Equal(t, []string{"water/inquiry/demoted", "water/inquiry/redemoted"}, sw)
	assert.Equal(t, []string{"water/inquiry/failed"}, tr)
	assert.True(t, dem[1])
	assert.Contains(t, msgs(log.byLevel("info")), "providers: trial failed, row demoted again")
}

// TestTrialReleased: a trial with no verdict frees the claim; a claim that never ends can be taken
// again after the trial timeout; a trial of a row that a load cleared changes nothing.
func TestTrialReleased(t *testing.T) {
	s, repo, _, clock, rec := switcher(t, waterRows())
	assert.NoError(t, s.Demote(1, time.Minute))
	clock.add(time.Minute)
	_, trial, _ := s.PickInquiry("water")
	assert.True(t, trial.Active())
	trial.Done(TrialReleased)
	_, trial, _ = s.PickInquiry("water")
	assert.True(t, trial.Active(), "free again")

	clock.add(trialClaimTimeout - time.Second)
	_, t2, _ := s.PickInquiry("water")
	assert.False(t, t2.Active(), "the claim is young")
	clock.add(time.Second)
	_, t2, _ = s.PickInquiry("water")
	assert.True(t, t2.Active(), "an old claim can be taken again")

	repo.set(waterRows()[1:], nil)
	assert.NoError(t, s.Load(context.Background()))
	t2.Done(TrialFailed)
	trial.Done("other") // not a Trial value: released; the demotion is gone, nothing happens
	assert.Equal(t, []int64{2}, ids(s.Group("water", "inquiry")))
	_, tr, _ := rec.get()
	assert.Equal(t, []string{"water/inquiry/released"}, tr)
}

// TestOrderTrialByInquiry: a demoted order row is tried by one inquiry of the same bill type
// through the inquiry row of the same provider (also when that row is not the first of its group);
// a good trial puts the order row back. A payment is never a trial (PickProvider of order).
func TestOrderTrialByInquiry(t *testing.T) {
	rows := []Provider{
		row(39, "parsian", "mobile", "inquiry", 2, StatusEnable),
		row(28, "jiring-switch", "mobile", "inquiry", 10, StatusEnable),
		row(30, "jiring-switch", "mobile", "order", 1, StatusEnable),
		row(48, "parsian", "mobile", "order", 9, StatusEnable),
	}
	s, _, _, clock, rec := switcher(t, rows)
	assert.NoError(t, s.Demote(30, time.Minute))
	assert.Equal(t, int64(48), pickID(t, s, "mobile", "order"))
	clock.add(time.Minute)
	assert.Equal(t, int64(48), pickID(t, s, "mobile", "order"), "no payment trial")
	p, trial, err := s.PickInquiry("mobile")
	assert.NoError(t, err)
	assert.Equal(t, int64(28), p.ID, "the inquiry row of the provider of the order row")
	assert.True(t, trial.Active())
	p, t2, _ := s.PickInquiry("mobile")
	assert.Equal(t, int64(39), p.ID)
	assert.False(t, t2.Active())
	trial.Done(TrialOK)
	assert.Equal(t, int64(30), pickID(t, s, "mobile", "order"))
	sw, tr, _ := rec.get()
	assert.Equal(t, []string{"mobile/order/demoted", "mobile/order/restored"}, sw)
	assert.Equal(t, []string{"mobile/order/ok"}, tr)
}

// TestOrderNoInquiryRow (open point 1, operator (a)): a demoted order row whose provider has no
// available inquiry row of the bill type returns with no trial when its period ends (a timer); a
// load after the period does the same.
func TestOrderNoInquiryRow(t *testing.T) {
	rows := []Provider{
		row(29, "jiring-switch", "mobilemidterm", "order", 1, StatusEnable),
		row(149, "parsian", "mobilemidterm", "order", 9, StatusEnable),
	}
	s, _, log, _, rec := switcher(t, rows)
	s.now = time.Now // the timer runs on the real clock
	assert.NoError(t, s.Demote(29, 30*time.Millisecond))
	assert.Equal(t, int64(149), pickID(t, s, "mobilemidterm", "order"))
	assert.Eventually(t, func() bool {
		p, _ := s.PickProvider("mobilemidterm", "order")
		return p.ID == 29
	}, 2*time.Second, 5*time.Millisecond)
	sw, tr, dem := rec.get()
	assert.Equal(t, []string{"mobilemidterm/order/demoted", "mobilemidterm/order/restored"}, sw)
	assert.Empty(t, tr, "no trial")
	assert.False(t, dem[29])
	assert.Contains(t, msgs(log.byLevel("info")), "providers: row back in its place with no trial (no inquiry row of the provider)")

	// A load after the period (the timer is late) returns it too.
	s2, _, _, clock, rec2 := switcher(t, rows)
	assert.NoError(t, s2.Demote(29, time.Hour))
	clock.add(time.Hour)
	assert.NoError(t, s2.Load(context.Background()))
	assert.Equal(t, int64(29), pickID(t, s2, "mobilemidterm", "order"))
	sw, _, _ = rec2.get()
	assert.Equal(t, "mobilemidterm/order/restored", sw[len(sw)-1])
}

// TestPickInquiryFastPath: with no due demotion, PickInquiry is the pick of PickProvider and makes
// no allocation; an unknown bill type gives ErrNoProvider.
func TestPickInquiryFastPath(t *testing.T) {
	s, _, _, _, _ := switcher(t, waterRows())
	assert.Zero(t, testing.AllocsPerRun(100, func() { _, _, _ = s.PickInquiry("water") }))
	assert.NoError(t, s.Demote(1, time.Minute))
	assert.Zero(t, testing.AllocsPerRun(100, func() { _, _, _ = s.PickInquiry("water") }), "a demotion that is not due")
	_, _, err := s.PickInquiry("gas")
	assert.ErrorIs(t, err, ErrNoProvider)
	var zero ProvidersService
	_, trial, err := zero.PickInquiry("water")
	assert.ErrorIs(t, err, ErrNoProvider)
	assert.False(t, trial.Active())
}

// TestTrialConcurrent (-race): many requests at the same time after the period: exactly one gets
// the trial; a load, a Demote and the end of the trial run at the same time as the picks.
func TestTrialConcurrent(t *testing.T) {
	s, _, _, clock, rec := switcher(t, waterRows())
	assert.NoError(t, s.Demote(1, time.Minute))
	clock.add(time.Minute)
	var trials atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	var won Trial
	var wonMu sync.Mutex
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			p, tr, err := s.PickInquiry("water")
			if !assert.NoError(t, err) {
				return
			}
			if tr.Active() {
				trials.Add(1)
				assert.Equal(t, int64(1), p.ID)
				wonMu.Lock()
				won = tr
				wonMu.Unlock()
			} else {
				assert.Equal(t, int64(2), p.ID)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_ = s.Load(context.Background())
		_ = s.Demote(2, time.Minute) // no next: row 1 is demoted
	}()
	close(start)
	wg.Wait()
	assert.Equal(t, int32(1), trials.Load(), "one trial")
	won.Done(TrialOK)
	var wg2 sync.WaitGroup
	for range 32 {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			p, tr, _ := s.PickInquiry("water")
			assert.Equal(t, int64(1), p.ID)
			assert.False(t, tr.Active())
		}()
	}
	wg2.Wait()
	_, tr, _ := rec.get()
	assert.Equal(t, []string{"water/inquiry/ok"}, tr)
}

// TestOrderTextDemoted: the debug line of a group order marks a demoted row.
func TestOrderTextDemoted(t *testing.T) {
	g := newGroup([]Provider{{ID: 2, TmpPriority: 2, Status: StatusEnable}, {ID: 1, TmpPriority: 1, Status: StatusEnable, Demoted: true}})
	assert.Equal(t, "2:2:enable,1:1:enable:demoted", orderText(g))
}
