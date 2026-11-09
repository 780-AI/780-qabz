// Package provider holds the providers of the bill service database (the providers table) and the
// code that picks one for a bill type and an action.
//
// ProvidersService is the only way to read providers (PROJECT_SPECIFIC_CONVENTIONS.md § 13): every
// other package gets a provider, a list or other provider data from it, never from the repository
// or the table.
package provider

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Status values of the providers.status column. Only StatusEnable makes a provider enabled (bill
// reads WHERE status = 'enable'); any other value, NULL ("") too, is disabled.
const (
	StatusEnable  = "enable"
	StatusDisable = "disable"
)

// MinLoadInterval is the lowest allowed providers.general.load_interval. The config check rejects
// a lower value at start; a lower hot-reloaded value is used as MinLoadInterval.
const MinLoadInterval = time.Second

// loadTimeout limits one load of Start and of the reload routine (the query and the scan of the
// rows). It is fixed (not a config key): a load that takes longer fails with
// context.DeadlineExceeded, is logged and counted as an error, and the last good list stays.
const loadTimeout = 10 * time.Second

// Results of a load, for Options.OnLoad.
const (
	LoadOK    = "ok"
	LoadError = "error"
)

// Changes and their results, for Options.OnChange.
const (
	ChangeMoveUp   = "move_up"
	ChangeMoveDown = "move_down"
	ChangeEnable   = "enable"
	ChangeDisable  = "disable"

	ResultChanged   = "changed"
	ResultUnchanged = "unchanged"
	ResultUnknownID = "unknown_id"
)

var (
	// ErrNoProvider is the PickProvider error for a group with no enabled provider, and for a
	// bill type and action that has no provider at all. Test it with errors.Is.
	ErrNoProvider = errors.New("provider: no enabled provider for the bill type and action")
	// ErrUnknownProvider is the error of MoveUp, MoveDown, Enable and Disable for an id that is
	// not in the list.
	ErrUnknownProvider = errors.New("provider: unknown provider id")
)

// Provider is one row of the providers table, plus the runtime key TmpPriority. It has no
// reference fields (no slice, map or pointer), so a copy shares nothing with the service.
type Provider struct {
	ID       int64
	Name     string
	Subject  string
	BillType string
	Action   string
	Type     string
	// Priority is the stored providers.priority. It never changes at runtime.
	Priority int
	// TmpPriority is the sort key. It is not a column: each load sets it to Priority, and only
	// MoveUp and MoveDown change it (system.html, Provider Switcher).
	TmpPriority int
	Timeout     int
	Status      string
	UpdatedAt   time.Time
	UseNats     bool
	InquiryStep int
	ProviderFee int64
}

// Enabled reports whether PickProvider can return the provider.
func (p Provider) Enabled() bool { return p.Status == StatusEnable }

// Repository reads every row of the providers table. Only ProvidersService calls it.
type Repository interface {
	ListProviders(ctx context.Context) ([]Provider, error)
}

// Logger is the logger the service needs.
type Logger interface {
	Debug(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// ProviderSelector is the provider list as the rest of the code uses it. ProvidersService
// implements it. Every method is safe for concurrent use.
type ProviderSelector interface {
	PickProvider(billType, action string) (Provider, error)
	List() []Provider
	Group(billType, action string) []Provider
	MoveUp(id int64) error
	MoveDown(id int64) error
	Enable(id int64) error
	Disable(id int64) error
}

// Options configures NewProvidersService.
type Options struct {
	// Interval returns the current providers.general.load_interval. The reload routine calls it
	// before each wait, so a hot-reloaded value applies from the next wait. Required.
	Interval func() time.Duration
	// Logger is required.
	Logger Logger
	// OnLoad is called once per load with LoadOK or LoadError and the load duration. Nil: no call.
	OnLoad func(result string, d time.Duration)
	// OnChange is called once per MoveUp, MoveDown, Enable and Disable call with the change and
	// its result. Nil: no call.
	OnChange func(change, result string)
	// OnActiveCount is called with the number of enabled providers in all groups of the new list
	// after each good load and after each Enable or Disable that changed the list. It is called
	// while the change holds the writer lock, so successive calls follow the order of the changes
	// and the last value always equals the current list. A failed load, a move and an unchanged
	// Enable or Disable do not call it. Nil: no call.
	OnActiveCount func(n int)
}

// groupKey is one (bill_type, action).
type groupKey struct{ billType, action string }

// group is the sorted providers of one groupKey. first is the index of the first enabled
// provider, or -1; enabled is the number of enabled providers. A group is never changed after it
// is stored in a snapshot.
type group struct {
	providers []Provider
	first     int
	enabled   int
}

// snapshot is one whole list. It is never changed after it is stored: a change builds a new one
// (copy on write), and unchanged groups are shared between snapshots.
type snapshot struct {
	groups  map[groupKey]*group
	keys    []groupKey         // every key, sorted by bill type, then action
	byID    map[int64]groupKey // the group of each provider id
	size    int                // the number of providers
	enabled int                // the number of enabled providers in all groups
}

var emptySnapshot = &snapshot{groups: map[groupKey]*group{}, byID: map[int64]groupKey{}}

// ProvidersService is the in-memory sorted list of the providers, grouped by (bill type, action).
// In a group the order is TmpPriority ascending, then ID ascending; nothing else changes it.
// It loads the rows at Start and again at each load interval; the database always wins: each load
// replaces the whole list (TmpPriority = Priority, the stored status, new rows added, removed rows
// dropped).
//
// ProvidersService is safe for concurrent use by any number of goroutines; a caller needs no lock.
// Readers (PickProvider, List, Group) read one atomic pointer to an immutable snapshot and never
// wait for a writer. Writers (a load, MoveUp, MoveDown, Enable, Disable) run one at a time and each
// stores a whole new snapshot, so a reader sees the list before or after a change, never a mix.
type ProvidersService struct {
	repo     Repository
	interval func() time.Duration
	log      Logger
	onLoad   func(result string, d time.Duration)
	onChange func(change, result string)
	onActive func(n int)
	// minInterval is MinLoadInterval (tests set a shorter one).
	minInterval time.Duration
	// loadTimeout is the loadTimeout constant (tests set a shorter one).
	loadTimeout time.Duration

	snap    atomic.Pointer[snapshot]
	writeMu sync.Mutex // one writer at a time: the apply of a load, a move, an enable or disable
	loadMu  sync.Mutex // one load at a time (the query runs outside writeMu)

	runMu  sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

var _ ProviderSelector = (*ProvidersService)(nil)

// errNotBuilt is the Load and Start error of a service not made by NewProvidersService.
var errNotBuilt = errors.New("providers: service not built with NewProvidersService")

// NewProvidersService returns a service with an empty list. Start (or Load) fills it.
func NewProvidersService(repo Repository, opts Options) (*ProvidersService, error) {
	if repo == nil {
		return nil, errors.New("providers: repository is required")
	}
	if opts.Interval == nil {
		return nil, errors.New("providers: load interval is required")
	}
	if opts.Logger == nil {
		return nil, errors.New("providers: logger is required")
	}
	s := &ProvidersService{repo: repo, interval: opts.Interval, log: opts.Logger, onLoad: opts.OnLoad,
		onChange: opts.OnChange, onActive: opts.OnActiveCount, minInterval: MinLoadInterval, loadTimeout: loadTimeout}
	s.snap.Store(emptySnapshot)
	return s, nil
}

// PickProvider returns the first enabled provider of (billType, action) in the sorted order, and
// follows no other logic. For the same group it returns the same provider until the list changes.
// A group with no enabled provider, or no group, gives ErrNoProvider. It is a hot path: it does not
// touch the database, does not wait for a writer and does not allocate. Safe for concurrent use.
// @todo bill (GetProviderByBillTypeAndAction) picks any enabled row that Postgres returns first
// (no ORDER BY, so priority is never used) and caches the pick in Redis per (bill type, action)
// for caching.provider. Qabz picks by the sorted order from memory and has no Redis cache. See
// docs/this_vs_bill.md, item 13.
func (s *ProvidersService) PickProvider(billType, action string) (Provider, error) {
	snap := s.snap.Load()
	if snap == nil {
		snap = emptySnapshot // a zero service
	}
	g := snap.groups[groupKey{billType, action}]
	if g == nil || g.first < 0 || g.first >= len(g.providers) {
		return Provider{}, ErrNoProvider
	}
	return g.providers[g.first], nil
}

// List returns every provider of every group, enabled and disabled: the groups by bill type, then
// action, and each group in its sorted order. The slice is a new copy. Safe for concurrent use.
func (s *ProvidersService) List() []Provider {
	snap := s.snap.Load()
	if snap == nil {
		snap = emptySnapshot // a zero service
	}
	out := make([]Provider, 0, snap.size)
	for _, k := range snap.keys {
		if g := snap.groups[k]; g != nil {
			out = append(out, g.providers...)
		}
	}
	return out
}

// Group returns the providers of (billType, action), enabled and disabled, in the sorted order.
// The slice is a new copy; an unknown group gives an empty slice. Safe for concurrent use.
func (s *ProvidersService) Group(billType, action string) []Provider {
	snap := s.snap.Load()
	if snap == nil {
		snap = emptySnapshot // a zero service
	}
	g := snap.groups[groupKey{billType, action}]
	if g == nil {
		return []Provider{}
	}
	return slices.Clone(g.providers)
}

// MoveUp moves the provider one place up in its group (operator decision 5: the group's
// TmpPriority values become 0..n-1 in the current order, then the two neighbors swap values).
// The first provider does not move. An unknown id gives ErrUnknownProvider. The next load resets
// the order. Safe for concurrent use.
func (s *ProvidersService) MoveUp(id int64) error { return s.move(id, -1, ChangeMoveUp) }

// MoveDown moves the provider one place down in its group, as MoveUp does. The last provider does
// not move. Safe for concurrent use.
func (s *ProvidersService) MoveDown(id int64) error { return s.move(id, 1, ChangeMoveDown) }

// Enable sets the runtime status of the provider to StatusEnable. It can be called again: an
// enabled provider stays as it is. An unknown id gives ErrUnknownProvider. The next load resets
// the status to the stored one. Safe for concurrent use.
func (s *ProvidersService) Enable(id int64) error { return s.setStatus(id, StatusEnable, ChangeEnable) }

// Disable sets the runtime status of the provider to StatusDisable, so PickProvider skips it. It
// can be called again. An unknown id gives ErrUnknownProvider. Safe for concurrent use.
func (s *ProvidersService) Disable(id int64) error {
	return s.setStatus(id, StatusDisable, ChangeDisable)
}

func (s *ProvidersService) move(id int64, step int, change string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cur := s.snap.Load()
	if cur == nil {
		cur = emptySnapshot // a zero service
	}
	k, g, i, ok := cur.find(id)
	if !ok {
		s.report(change, ResultUnknownID)
		return ErrUnknownProvider
	}
	j := i + step
	if j < 0 || j >= len(g.providers) {
		s.report(change, ResultUnchanged)
		return nil
	}
	ps := slices.Clone(g.providers)
	for r := range ps {
		ps[r].TmpPriority = r
	}
	ps[i].TmpPriority, ps[j].TmpPriority = ps[j].TmpPriority, ps[i].TmpPriority
	sortProviders(ps)
	s.snap.Store(cur.with(k, newGroup(ps)))
	s.report(change, ResultChanged)
	return nil
}

func (s *ProvidersService) setStatus(id int64, status, change string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cur := s.snap.Load()
	if cur == nil {
		cur = emptySnapshot // a zero service
	}
	k, g, i, ok := cur.find(id)
	if !ok {
		s.report(change, ResultUnknownID)
		return ErrUnknownProvider
	}
	if g.providers[i].Status == status {
		s.report(change, ResultUnchanged)
		return nil
	}
	ps := slices.Clone(g.providers)
	ps[i].Status = status
	next := cur.with(k, newGroup(ps))
	s.snap.Store(next)
	s.reportActive(next.enabled)
	s.report(change, ResultChanged)
	return nil
}

func (s *ProvidersService) report(change, result string) {
	if s.onChange != nil {
		s.onChange(change, result)
	}
}

// reportActive gives n to Options.OnActiveCount. The caller holds writeMu.
func (s *ProvidersService) reportActive(n int) {
	if s.onActive != nil {
		s.onActive(n)
	}
}

// Load reads every row through the repository and replaces the whole list (the database always
// wins). A failed load keeps the last good list, writes one error log line and returns the error.
// After a load, each group whose order changed gets one debug log line with its new order
// (operator decision 6). Loads run one at a time; the query does not block readers or other
// writers. Safe for concurrent use.
func (s *ProvidersService) Load(ctx context.Context) error { return s.load(ctx, nil) }

// load is Load. abort is nil (Load), the App.Start context (the first load in Start) or the reload
// routine's context (which Stop cancels). When abort is not nil and the load fails with
// context.Canceled after abort is done, the load was aborted (the App start is aborted, or Stop runs),
// not failed: no error line and no OnLoad call. A load canceled for another reason, or one that
// times out (context.DeadlineExceeded), is a failure.
func (s *ProvidersService) load(ctx, abort context.Context) error {
	if s.repo == nil || s.log == nil {
		return errNotBuilt
	}
	s.loadMu.Lock()
	defer s.loadMu.Unlock()
	start := time.Now()
	rows, err := s.repo.ListProviders(ctx)
	if err != nil {
		if abort != nil && abort.Err() != nil && errors.Is(err, context.Canceled) {
			return err
		}
		s.reportLoad(LoadError, time.Since(start))
		s.log.Error("providers: load failed; keeping the last list", "error", err)
		return err
	}
	next := buildSnapshot(rows)
	if next == nil {
		next = emptySnapshot
	}
	s.writeMu.Lock()
	prev := s.snap.Load()
	if prev == nil {
		prev = emptySnapshot // a zero service
	}
	s.snap.Store(next)
	s.reportActive(next.enabled)
	s.writeMu.Unlock()
	s.reportLoad(LoadOK, time.Since(start))
	s.logOrderChanges(prev, next)
	return nil
}

func (s *ProvidersService) reportLoad(result string, d time.Duration) {
	if s.onLoad != nil {
		s.onLoad(result, d)
	}
}

// logOrderChanges writes one debug line for each group whose order (the ids in order and their
// status) differs between prev and next. A removed group prints an empty order.
func (s *ProvidersService) logOrderChanges(prev, next *snapshot) {
	if prev == nil {
		prev = emptySnapshot
	}
	if next == nil {
		next = emptySnapshot
	}
	keys := slices.Clone(next.keys)
	for _, k := range prev.keys {
		if next.groups[k] == nil {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		pg, ng := prev.groups[k], next.groups[k]
		if sameOrder(pg, ng) {
			continue
		}
		var pick int64
		if ng != nil && ng.first >= 0 && ng.first < len(ng.providers) {
			pick = ng.providers[ng.first].ID
		}
		s.log.Debug("providers: group order", "bill_type", k.billType, "action", k.action,
			"order", orderText(ng), "pick", pick)
	}
}

// sameOrder reports whether a and b have the same ids in the same order with the same status.
func sameOrder(a, b *group) bool {
	var pa, pb []Provider
	if a != nil {
		pa = a.providers
	}
	if b != nil {
		pb = b.providers
	}
	return slices.EqualFunc(pa, pb, func(x, y Provider) bool { return x.ID == y.ID && x.Status == y.Status })
}

// orderText prints a group as "id:tmp_priority:status" items in order, joined by ",".
func orderText(g *group) string {
	if g == nil {
		return ""
	}
	var b strings.Builder
	for i, p := range g.providers {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(p.ID, 10))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(p.TmpPriority))
		b.WriteByte(':')
		b.WriteString(p.Status)
	}
	return b.String()
}

// Start loads the providers once, synchronously (operator decision 2), then starts the reload
// routine. A failed first load does not fail the start: it is logged, the list stays empty
// (PickProvider gives ErrNoProvider) and the routine tries again at each load interval. A first
// load that fails with context.Canceled because ctx itself is canceled (the App start is aborted)
// writes no error line and is not counted; Start still returns nil. Safe for concurrent use; a
// second Start returns an error.
func (s *ProvidersService) Start(ctx context.Context) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.cancel != nil {
		return errors.New("providers: already started")
	}
	if s.repo == nil || s.log == nil || s.interval == nil || s.minInterval <= 0 {
		return errNotBuilt
	}
	s.loadOnce(ctx, ctx)
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	s.cancel, s.done = cancel, make(chan struct{})
	go s.loop(loopCtx, s.done)
	return nil
}

// Stop ends the reload routine and waits for it. It is safe to call more than once and from any
// goroutine.
func (s *ProvidersService) Stop(ctx context.Context) error {
	s.runMu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel = nil
	s.runMu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// loadOnce runs one load with the load timeout. abort is the context whose cancel aborts the load
// (the App.Start context for the first load, the reload routine's context after it): such a load is
// not logged or counted as a failure (see load).
func (s *ProvidersService) loadOnce(ctx, abort context.Context) {
	d := s.loadTimeout
	if d <= 0 {
		d = loadTimeout // a zero service
	}
	lctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	_ = s.load(lctx, abort) // load logs and counts a failure; the last good list stays
}

// loop reloads at each interval until ctx is done. It reads the interval before each wait
// (operator decision 3: a hot-reloaded value applies without restart).
func (s *ProvidersService) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	var lastBad time.Duration // the low value that was logged, when warned
	warned := false
	next := func() time.Duration {
		d := s.interval()
		if d < s.minInterval {
			if !warned || d != lastBad {
				s.log.Warn("providers: load interval below the minimum; using the minimum",
					"interval_ms", float64(d)/float64(time.Millisecond), "min_ms", float64(s.minInterval)/float64(time.Millisecond))
				lastBad, warned = d, true
			}
			return s.minInterval
		}
		warned = false
		return d
	}
	t := time.NewTimer(next())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.loadOnce(ctx, ctx)
			t.Reset(next())
		}
	}
}

// buildSnapshot groups and sorts rows. TmpPriority is set to Priority. The input order does not
// change the result.
func buildSnapshot(rows []Provider) *snapshot {
	lists := make(map[groupKey][]Provider)
	for _, p := range rows {
		p.TmpPriority = p.Priority
		k := groupKey{p.BillType, p.Action}
		lists[k] = append(lists[k], p)
	}
	groups := make(map[groupKey]*group, len(lists))
	for k, ps := range lists {
		sortProviders(ps)
		groups[k] = newGroup(ps)
	}
	return newSnapshot(groups)
}

// newSnapshot builds the sorted keys and the id index of groups.
func newSnapshot(groups map[groupKey]*group) *snapshot {
	snap := &snapshot{groups: groups, keys: make([]groupKey, 0, len(groups)), byID: make(map[int64]groupKey)}
	for k, g := range groups {
		snap.keys = append(snap.keys, k)
		if g == nil {
			continue
		}
		snap.size += len(g.providers)
		snap.enabled += g.enabled
		for _, p := range g.providers {
			snap.byID[p.ID] = k
		}
	}
	slices.SortFunc(snap.keys, func(a, b groupKey) int {
		if c := cmp.Compare(a.billType, b.billType); c != 0 {
			return c
		}
		return cmp.Compare(a.action, b.action)
	})
	return snap
}

// with returns a copy of s with group k replaced by g. The ids of the group must not change.
func (s *snapshot) with(k groupKey, g *group) *snapshot {
	enabled := s.enabled
	if old := s.groups[k]; old != nil {
		enabled -= old.enabled
	}
	if g != nil {
		enabled += g.enabled
	}
	groups := maps.Clone(s.groups)
	if groups == nil {
		groups = make(map[groupKey]*group, 1)
	}
	groups[k] = g
	return &snapshot{groups: groups, keys: s.keys, byID: s.byID, size: s.size, enabled: enabled}
}

// find returns the group of id and the index of id in it.
func (s *snapshot) find(id int64) (groupKey, *group, int, bool) {
	k, ok := s.byID[id]
	if !ok {
		return groupKey{}, nil, 0, false
	}
	g := s.groups[k]
	if g == nil {
		return groupKey{}, nil, 0, false
	}
	i := slices.IndexFunc(g.providers, func(p Provider) bool { return p.ID == id })
	if i < 0 {
		return groupKey{}, nil, 0, false
	}
	return k, g, i, true
}

func newGroup(ps []Provider) *group {
	g := &group{providers: ps, first: -1}
	for i, p := range ps {
		if !p.Enabled() {
			continue
		}
		if g.first < 0 {
			g.first = i
		}
		g.enabled++
	}
	return g
}

// sortProviders sorts by TmpPriority ascending, then ID ascending.
func sortProviders(ps []Provider) {
	slices.SortFunc(ps, func(a, b Provider) int {
		if c := cmp.Compare(a.TmpPriority, b.TmpPriority); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
}
