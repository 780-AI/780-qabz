package provider

// The Provider Switcher (task 059; system.html, Provider Switcher; conventions § 13). It moves a
// provider row to the end of the order of its group for a period (Demote), lets one real inquiry
// try the row when the period ends (PickInquiry and Trial), and puts the row back in its original
// place after a good trial. The stored priority never changes. The state is in memory in each
// instance of the App: a restart clears it. Nothing here names a bill type or a provider: the order
// is the priority of the rows, and a group with one available row is never demoted because it has
// no next available row.

import (
	"errors"
	"maps"
	"slices"
	"sync/atomic"
	"time"
)

// Results of a change of the Provider Switcher, for Options.OnSwitch (a fixed set: metric labels).
const (
	SwitchDemoted   = "demoted"   // Demote moved the row to the end of its group
	SwitchNoNext    = "no_next"   // Demote did nothing: the group has no next available row
	SwitchRestored  = "restored"  // the row is back in its original place (a good trial, or no trial possible)
	SwitchRedemoted = "redemoted" // a failed trial demoted the row again for a new period
	SwitchCleared   = "cleared"   // a load or Disable removed or disabled the row: its demotion ends
)

// Results of a trial, for Trial.Done and Options.OnTrial (a fixed set: metric labels).
const (
	TrialOK       = "ok"       // the provider answered (a good or a business answer): the row returns
	TrialFailed   = "failed"   // the provider was down: the row is demoted again
	TrialReleased = "released" // the call gave no verdict (a cancel, a fault before the call): the claim is free again
)

// trialClaimTimeout is the age after which a trial claim that never ended (a request that did not
// call Trial.Done) can be claimed again. It is fixed (not a config key).
const trialClaimTimeout = time.Minute

var (
	// ErrNoNextProvider is the Demote error when the group has no next available row: nothing
	// changes and the requests keep going to the row. Test it with errors.Is.
	ErrNoNextProvider = errors.New("provider: no next available provider in the group")
	// ErrBadPeriod is the Demote error for a period that is not greater than 0.
	ErrBadPeriod = errors.New("provider: the demotion period must be greater than 0")
)

// demotion is the state of one demoted row. A demotion again (a failed trial) makes a new value,
// so a Trial of an old demotion changes nothing.
type demotion struct {
	id     int64
	key    groupKey
	name   string // providers.name of the row
	until  time.Time
	period time.Duration
	// claim is the Unix time in nanoseconds of the trial claim in progress, or 0.
	claim atomic.Int64
	// timer returns a demoted order row that has no trial when the period ends (open point 1,
	// operator (a)). Nil for an inquiry row.
	timer *time.Timer
}

// Trial is the trial of a demoted row that PickInquiry gave to one request. The zero value is no
// trial. The flow calls Done once, after the provider call.
type Trial struct {
	s *ProvidersService
	d *demotion
}

// Active reports whether the pick is a trial.
func (t Trial) Active() bool { return t.s != nil && t.d != nil }

// Done ends the trial with TrialOK (the row returns to its original place), TrialFailed (the row
// is demoted again for its period) or TrialReleased (no verdict: the next request after the period
// can try). Any other value is TrialReleased. A trial whose demotion ended or changed meanwhile
// (a load, Disable) changes nothing. Safe for concurrent use; it waits for the writer lock.
func (t Trial) Done(result string) {
	if !t.Active() {
		return
	}
	t.s.endTrial(t.d, result)
}

// Demote moves the row id to the end of the order of its group for period (task 059). It demotes
// only when the group has a next available row: enabled, not demoted, and Options.Available true.
// With none it changes nothing, writes one info line and returns ErrNoNextProvider. A row that is
// demoted already or is disabled is not changed (nil). An unknown id gives ErrUnknownProvider. The
// stored priority never changes. Safe for concurrent use.
func (s *ProvidersService) Demote(id int64, period time.Duration) error {
	if period <= 0 {
		return ErrBadPeriod
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cur := s.current()
	k, g, i, ok := cur.find(id)
	if !ok {
		return ErrUnknownProvider
	}
	p := g.providers[i]
	if p.Demoted || !p.Enabled() {
		return nil
	}
	next, ok := s.nextAvailable(g, id)
	if !ok {
		s.switched(k, SwitchNoNext)
		s.info("providers: demote skipped, no next available provider", p, "period_ms", ms(period))
		return ErrNoNextProvider
	}
	s.newDemotion(k, p, period)
	s.store(cur.regroup(k, s.demotions))
	s.switched(k, SwitchDemoted)
	s.demotedChanged(p, true)
	s.info("providers: row demoted", p, "period_ms", ms(period), "next", next.ID, "next_provider", next.Name)
	return nil
}

// PickInquiry is PickProvider(billType, ActionInquiry) with the trial of the Provider Switcher
// (task 059). Before the earliest end of a demotion it is the plain pick (no database, no lock, no
// allocation). After it, one request at a time gets a trial: a due demoted inquiry row of the
// group, or, for a due demoted order row of the bill type, the available inquiry row (not demoted)
// of the same provider. The flow calls Trial.Done after the call. Every other request gets the
// plain pick. Safe for concurrent use.
func (s *ProvidersService) PickInquiry(billType string) (Provider, Trial, error) {
	snap := s.current()
	if snap.dueAt != 0 {
		if now := s.clock(); now.UnixNano() >= snap.dueAt {
			if p, d, ok := s.claimTrial(snap, billType, now); ok {
				return p, Trial{s: s, d: d}, nil
			}
		}
	}
	p, err := snap.pick(billType, ActionInquiry)
	return p, Trial{}, err
}

// claimTrial finds a due demotion for an inquiry of billType and claims its trial.
func (s *ProvidersService) claimTrial(snap *snapshot, billType string, now time.Time) (Provider, *demotion, bool) {
	inq := snap.groups[groupKey{billType, ActionInquiry}]
	if inq == nil {
		return Provider{}, nil, false
	}
	for _, p := range inq.providers {
		if d := snap.demoted[p.ID]; d != nil && p.Enabled() && !now.Before(d.until) && s.claim(d, now) {
			return p, d, true
		}
	}
	ord := snap.groups[groupKey{billType, actionOrder}]
	if ord == nil {
		return Provider{}, nil, false
	}
	for _, op := range ord.providers {
		d := snap.demoted[op.ID]
		if d == nil || now.Before(d.until) {
			continue
		}
		if r, ok := s.trialRow(inq, op.Name); ok && s.claim(d, now) {
			return r, d, true
		}
	}
	return Provider{}, nil, false
}

// trialRow is the inquiry row of the provider name that can carry the trial of a demoted order row:
// enabled, not demoted, available.
func (s *ProvidersService) trialRow(inq *group, name string) (Provider, bool) {
	if inq == nil {
		return Provider{}, false
	}
	for _, r := range inq.providers {
		if r.Name == name && r.Enabled() && !r.Demoted && s.isAvailable(r) {
			return r, true
		}
	}
	return Provider{}, false
}

// claim takes the trial of d: free, or claimed longer than the trial timeout ago.
func (s *ProvidersService) claim(d *demotion, now time.Time) bool {
	timeout := s.trialTimeout
	if timeout <= 0 {
		timeout = trialClaimTimeout // a zero service
	}
	c := d.claim.Load()
	if c != 0 && now.Sub(time.Unix(0, c)) < timeout {
		return false
	}
	return d.claim.CompareAndSwap(c, now.UnixNano())
}

// endTrial is Trial.Done.
func (s *ProvidersService) endTrial(d *demotion, result string) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.demotions[d.id] != d {
		return // the demotion ended or changed meanwhile
	}
	cur := s.current()
	_, g, i, ok := cur.find(d.id)
	if !ok {
		return
	}
	p := g.providers[i]
	switch result {
	case TrialOK:
		s.trialed(d.key, TrialOK)
		s.store(s.restore(cur, d))
		s.info("providers: trial ok, row back in its place", p)
	case TrialFailed:
		s.trialed(d.key, TrialFailed)
		s.newDemotion(d.key, p, d.period)
		s.store(cur.regroup(d.key, s.demotions))
		s.switched(d.key, SwitchRedemoted)
		s.info("providers: trial failed, row demoted again", p, "period_ms", ms(d.period))
	default:
		s.trialed(d.key, TrialReleased)
		d.claim.Store(0)
	}
}

// newDemotion stores a new demotion of p for period (it replaces the one of p, and stops its timer)
// and starts the timer of an order row. The caller holds writeMu and stores the new snapshot.
func (s *ProvidersService) newDemotion(k groupKey, p Provider, period time.Duration) *demotion {
	if old := s.demotions[p.ID]; old != nil && old.timer != nil {
		old.timer.Stop()
	}
	d := &demotion{id: p.ID, key: k, name: p.Name, until: s.clock().Add(period), period: period}
	if k.action == actionOrder {
		d.timer = time.AfterFunc(period, func() { s.expire(d) })
	}
	if s.demotions == nil {
		s.demotions = map[int64]*demotion{}
	}
	s.demotions[p.ID] = d
	return d
}

// expire is the timer of a demoted order row: when its period ends and no trial is possible (the
// provider has no available inquiry row of the bill type), the row returns with no trial.
func (s *ProvidersService) expire(d *demotion) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.demotions[d.id] != d {
		return
	}
	cur := s.current()
	if _, ok := s.trialRow(cur.groups[groupKey{d.key.billType, ActionInquiry}], d.name); ok {
		return // an inquiry will try it
	}
	s.returnNoTrial(cur, d)
}

// returnNoTrial puts a due order row with no trial back. The caller holds writeMu.
func (s *ProvidersService) returnNoTrial(cur *snapshot, d *demotion) {
	_, g, i, ok := cur.find(d.id)
	s.store(s.restore(cur, d))
	if ok {
		s.info("providers: row back in its place with no trial (no inquiry row of the provider)", g.providers[i])
	}
}

// restore ends the demotion d and returns the new snapshot with the row in its original place. The
// caller holds writeMu and stores the snapshot.
func (s *ProvidersService) restore(cur *snapshot, d *demotion) *snapshot {
	return s.end(cur, d, SwitchRestored)
}

// clear ends the demotion d of a row that a load or Disable removed or disabled. The caller holds
// writeMu and stores the snapshot.
func (s *ProvidersService) clear(cur *snapshot, d *demotion) *snapshot {
	next := s.end(cur, d, SwitchCleared)
	s.log.Info("providers: demotion cleared, the row is removed or disabled", "id", d.id, "provider", d.name,
		"bill_type", d.key.billType, "action", d.key.action)
	return next
}

// end ends the demotion d with the OnSwitch result and returns the new snapshot (with the current
// demotions). The caller holds writeMu.
func (s *ProvidersService) end(cur *snapshot, d *demotion, result string) *snapshot {
	if d.timer != nil {
		d.timer.Stop()
	}
	delete(s.demotions, d.id)
	s.switched(d.key, result)
	s.demotedChanged(Provider{ID: d.id, Name: d.name, BillType: d.key.billType, Action: d.key.action}, false)
	if _, _, _, ok := cur.find(d.id); !ok {
		return cur.withDemotions(s.demotions)
	}
	return cur.regroup(d.key, s.demotions).withDemotions(s.demotions)
}

// keepDemotions applies the demotions to a new list of a load: a row that is gone, has moved to
// another group or is disabled loses its demotion; a due order row with no trial possible returns;
// every other demotion stays. The caller holds writeMu.
func (s *ProvidersService) keepDemotions(next *snapshot) *snapshot {
	if len(s.demotions) == 0 {
		return next
	}
	now := s.clock()
	keys := map[groupKey]bool{}
	for _, d := range s.sortedDemotions() {
		k, g, i, ok := next.find(d.id)
		if !ok || k != d.key || !g.providers[i].Enabled() {
			next = s.clear(next, d)
			continue
		}
		keys[k] = true
	}
	for k := range keys {
		next = next.regroup(k, s.demotions)
	}
	for _, d := range s.sortedDemotions() {
		if d.key.action == actionOrder && !now.Before(d.until) {
			if _, ok := s.trialRow(next.groups[groupKey{d.key.billType, ActionInquiry}], d.name); !ok {
				next = s.end(next, d, SwitchRestored)
				s.log.Info("providers: row back in its place with no trial (no inquiry row of the provider)", "id", d.id,
					"provider", d.name, "bill_type", d.key.billType, "action", d.key.action)
			}
		}
	}
	return next.withDemotions(s.demotions)
}

// sortedDemotions are the demotions by id (a fixed order for the log and the tests).
func (s *ProvidersService) sortedDemotions() []*demotion {
	ds := slices.Collect(maps.Values(s.demotions))
	slices.SortFunc(ds, func(a, b *demotion) int {
		switch {
		case a.id < b.id:
			return -1
		case a.id > b.id:
			return 1
		}
		return 0
	})
	return ds
}

// nextAvailable is the first row of g after the order that is not id, enabled, not demoted and
// available.
func (s *ProvidersService) nextAvailable(g *group, id int64) (Provider, bool) {
	for _, q := range g.providers {
		if q.ID != id && q.Enabled() && !q.Demoted && s.isAvailable(q) {
			return q, true
		}
	}
	return Provider{}, false
}

func (s *ProvidersService) isAvailable(p Provider) bool {
	return s.available == nil || s.available(p)
}

// store publishes next with the current demotions. The caller holds writeMu.
func (s *ProvidersService) store(next *snapshot) {
	s.snap.Store(next.withDemotions(s.demotions))
}

func (s *ProvidersService) clock() time.Time {
	if s.now == nil {
		return time.Now() // a zero service
	}
	return s.now()
}

func (s *ProvidersService) switched(k groupKey, result string) {
	if s.onSwitch != nil {
		s.onSwitch(k.billType, k.action, result)
	}
}

func (s *ProvidersService) trialed(k groupKey, result string) {
	if s.onTrial != nil {
		s.onTrial(k.billType, k.action, result)
	}
}

func (s *ProvidersService) demotedChanged(p Provider, demoted bool) {
	if s.onDemoted != nil {
		s.onDemoted(p, demoted)
	}
}

// info writes one info line about the row p.
func (s *ProvidersService) info(msg string, p Provider, kv ...any) {
	s.log.Info(msg, append([]any{"id", p.ID, "provider", p.Name, "bill_type", p.BillType, "action", p.Action}, kv...)...)
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// actionOrder is providers.action of a payment row (order.ActionOrder; the provider package does not
// import core/order).
const actionOrder = "order"

// regroup returns a copy of snap with group k sorted again with the Demoted flag of each row from
// dem.
func (snap *snapshot) regroup(k groupKey, dem map[int64]*demotion) *snapshot {
	g := snap.groups[k]
	if g == nil {
		return snap
	}
	ps := slices.Clone(g.providers)
	for i := range ps {
		ps[i].Demoted = dem[ps[i].ID] != nil
	}
	sortProviders(ps)
	return snap.with(k, newGroup(ps))
}

// withDemotions returns a copy of snap with a copy of dem and its earliest end time.
func (snap *snapshot) withDemotions(dem map[int64]*demotion) *snapshot {
	c := *snap
	c.demoted, c.dueAt = nil, 0
	if len(dem) > 0 {
		c.demoted = maps.Clone(dem)
		for _, d := range dem {
			if u := d.until.UnixNano(); c.dueAt == 0 || u < c.dueAt {
				c.dueAt = u
			}
		}
	}
	return &c
}
