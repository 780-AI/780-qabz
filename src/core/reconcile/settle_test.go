package reconcile

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/order"
	"qabz/provider"
)

// settleRepo serves ConfirmCandidates from a list of not confirmed orders per bill type, as the
// keyset SQL: order_id greater than after, in order_id order, at most limit. It records each call.
type settleRepo struct {
	orders  map[string][]order.Order // by bill type; the order of the slice does not matter
	calls   []string                 // "<bill type>>after"
	err     error
	from    time.Time
	to      time.Time
	limitOK int
}

func (r *settleRepo) ConfirmCandidates(_ context.Context, billType, after string, limit int, from, to time.Time) ([]order.Order, error) {
	r.calls = append(r.calls, billType+">"+after)
	if limit != r.limitOK || !from.Equal(r.from) || !to.Equal(r.to) {
		return nil, fmt.Errorf("unexpected ConfirmCandidates(%d, %s, %s)", limit, from, to)
	}
	if r.err != nil {
		return nil, r.err
	}
	all := append([]order.Order(nil), r.orders[billType]...)
	sort.Slice(all, func(i, j int) bool { return all[i].OrderID < all[j].OrderID })
	var page []order.Order
	for _, o := range all {
		if o.OrderID > after && len(page) < limit {
			page = append(page, o)
		}
	}
	return page, nil
}

// fakeSettler answers by order ID (default success); a success removes the order from the repo,
// as provider_confirmed = true does.
type fakeSettler struct {
	repo    *settleRepo
	answers map[string]string
	sent    []string
	cancel  func() // called after the first settlement, when set
}

func (s *fakeSettler) Settle(_ context.Context, o order.Order) string {
	s.sent = append(s.sent, o.OrderID)
	if s.cancel != nil {
		s.cancel()
	}
	result, ok := s.answers[o.OrderID]
	if !ok {
		result = provider.StatusSuccess
	}
	if result == provider.StatusSuccess {
		list := s.repo.orders[o.BillType]
		for i := range list {
			if list[i].OrderID == o.OrderID {
				s.repo.orders[o.BillType] = append(list[:i:i], list[i+1:]...)
				break
			}
		}
	}
	return result
}

type settleTestWorld struct {
	repo    *settleRepo
	settler *fakeSettler
	log     *logRec
	results map[string]int
	w       *SettleWorker
}

func ids(billType string, names ...string) []order.Order {
	out := make([]order.Order, 0, len(names))
	for _, n := range names {
		out = append(out, order.Order{OrderID: n, BillType: billType, Status: order.StatusSuccess})
	}
	return out
}

func newSettleTestWorld(t *testing.T) *settleTestWorld {
	t.Helper()
	repo := &settleRepo{
		orders: map[string][]order.Order{
			provider.BillTypeTel:        ids(provider.BillTypeTel, "a3", "a1", "a2", "a5", "a4"),
			provider.BillTypeTelMidterm: ids(provider.BillTypeTelMidterm, "b1"),
		},
		from: now.Add(-24 * time.Hour), to: now.Add(-5 * time.Minute), limitOK: 2,
	}
	wd := &settleTestWorld{repo: repo, settler: &fakeSettler{repo: repo, answers: map[string]string{}}, log: &logRec{}, results: map[string]int{}}
	w, err := NewSettleWorker(SettleOptions{Name: "telephone-confirmpayment", Interval: 15 * time.Minute, Start: 5 * time.Minute,
		End: 24 * time.Hour, Chunk: 2, Repository: repo, Settler: wd.settler, Logger: wd.log,
		OnOrder: func(name, result string) {
			assert.Equal(t, "telephone-confirmpayment", name)
			wd.results[result]++
		}})
	if !assert.NoError(t, err) || !assert.NotNil(t, w) {
		t.FailNow()
	}
	w.now = func() time.Time { return now }
	wd.w = w
	return wd
}

// TestSettleRunAllConfirmed: tel then telmidterm, pages of the chunk after the last order_id, the
// lock charged for each page, one count per order, one line per bill type.
func TestSettleRunAllConfirmed(t *testing.T) {
	wd := newSettleTestWorld(t)
	ch := &fakeCharger{}
	assert.NoError(t, wd.w.Run(context.Background(), ch))
	assert.Equal(t, []string{"a1", "a2", "a3", "a4", "a5", "b1"}, wd.settler.sent)
	assert.Equal(t, []string{"tel>", "tel>a2", "tel>a4", "telmidterm>"}, wd.repo.calls, "a short page ends the bill type")
	assert.Equal(t, []int{2, 2, 1, 1}, ch.charges)
	assert.Equal(t, map[string]int{provider.StatusSuccess: 6}, wd.results)
	assert.Equal(t, []string{"settle worker: bill type done", "settle worker: bill type done"}, wd.log.infos)
	assert.Empty(t, wd.log.errs)
}

// TestSettleRunOncePerRun (design D3 (a)): a settlement that fails stays not confirmed, but the run
// sends it only once and ends; the next run sends it again (bill: its run never ends).
func TestSettleRunOncePerRun(t *testing.T) {
	wd := newSettleTestWorld(t)
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5", "b1"} {
		wd.settler.answers[id] = order.SettleError
	}
	wd.settler.answers["a3"] = provider.StatusFailed
	assert.NoError(t, wd.w.Run(context.Background(), nil))
	assert.Equal(t, []string{"a1", "a2", "a3", "a4", "a5", "b1"}, wd.settler.sent, "each order once")
	assert.Equal(t, map[string]int{order.SettleError: 5, provider.StatusFailed: 1}, wd.results)

	wd.settler.sent = nil
	assert.NoError(t, wd.w.Run(context.Background(), nil))
	assert.Equal(t, []string{"a1", "a2", "a3", "a4", "a5", "b1"}, wd.settler.sent, "the next run tries again")
}

// TestSettleRunMixed: the confirmed orders leave the list; the page after the last order_id still
// reaches every other order (no order skipped, as an OFFSET would skip).
func TestSettleRunMixed(t *testing.T) {
	wd := newSettleTestWorld(t)
	wd.settler.answers["a2"] = order.SettleError
	wd.settler.answers["a4"] = provider.StatusPending
	assert.NoError(t, wd.w.Run(context.Background(), nil))
	assert.Equal(t, []string{"a1", "a2", "a3", "a4", "a5", "b1"}, wd.settler.sent)
	assert.Equal(t, ids(provider.BillTypeTel, "a2", "a4"), sortedByID(wd.repo.orders[provider.BillTypeTel]), "only the failed ones stay")
}

func sortedByID(list []order.Order) []order.Order {
	out := append([]order.Order(nil), list...)
	sort.Slice(out, func(i, j int) bool { return out[i].OrderID < out[j].OrderID })
	return out
}

// TestSettleRunErrors: a failed read and a lost lock end the run with the error; a done ctx ends
// the run; no order gives no charge.
func TestSettleRunErrors(t *testing.T) {
	wd := newSettleTestWorld(t)
	wd.repo.err = errors.New("db down")
	err := wd.w.Run(context.Background(), nil)
	assert.ErrorContains(t, err, "db down")
	assert.Empty(t, wd.settler.sent)

	wd = newSettleTestWorld(t)
	err = wd.w.Run(context.Background(), &fakeCharger{err: errors.New("lost")})
	assert.ErrorContains(t, err, "keep the lock")
	assert.Empty(t, wd.settler.sent)

	wd = newSettleTestWorld(t)
	ctx, cancel := context.WithCancel(context.Background())
	wd.settler.cancel = cancel
	err = wd.w.Run(ctx, nil)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"a1"}, wd.settler.sent, "the stop ends the run after the order in flight")

	wd = newSettleTestWorld(t)
	wd.repo.orders = map[string][]order.Order{}
	ch := &fakeCharger{}
	assert.NoError(t, wd.w.Run(context.Background(), ch))
	assert.Empty(t, ch.charges)
	assert.Equal(t, []string{"tel>", "telmidterm>"}, wd.repo.calls)
}

// stuckRepo gives the same full page each time (a defect of the store): the run stops.
type stuckRepo struct{}

func (stuckRepo) ConfirmCandidates(context.Context, string, string, int, time.Time, time.Time) ([]order.Order, error) {
	return ids(provider.BillTypeTel, "", ""), nil
}

// TestSettleRunNoProgress: a page whose last order_id does not move forward ends the run with an
// error, not a loop.
func TestSettleRunNoProgress(t *testing.T) {
	wd := newSettleTestWorld(t)
	wd.w.o.Repository = stuckRepo{}
	err := wd.w.Run(context.Background(), nil)
	assert.ErrorContains(t, err, "did not move forward")
	assert.Len(t, wd.settler.sent, 2)
}

// TestNewSettleWorkerRules: every rule of the options.
func TestNewSettleWorkerRules(t *testing.T) {
	ok := SettleOptions{Name: "n", Interval: time.Minute, Start: 0, End: time.Hour, Chunk: 1, Repository: &settleRepo{}, Settler: &fakeSettler{}}
	w, err := NewSettleWorker(ok)
	if assert.NoError(t, err) && assert.NotNil(t, w) {
		assert.Equal(t, "n", w.Name())
		assert.Equal(t, time.Minute, w.Interval())
		assert.Equal(t, 1, w.Chunk())
	}
	for name, change := range map[string]func(*SettleOptions){
		"no name":        func(o *SettleOptions) { o.Name = "" },
		"no interval":    func(o *SettleOptions) { o.Interval = 0 },
		"no chunk":       func(o *SettleOptions) { o.Chunk = 0 },
		"negative start": func(o *SettleOptions) { o.Start = -time.Second },
		"end = start":    func(o *SettleOptions) { o.Start, o.End = time.Hour, time.Hour },
		"no repository":  func(o *SettleOptions) { o.Repository = nil },
		"no settler":     func(o *SettleOptions) { o.Settler = nil },
	} {
		bad := ok
		change(&bad)
		_, err := NewSettleWorker(bad)
		assert.Error(t, err, name)
	}
}
