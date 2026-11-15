package reconcile

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/order"
	"qabz/provider"
)

var now = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

// fakeRepo gives pages of IDs and rows, and records the saves.
type fakeRepo struct {
	pages     [][]string
	pageErr   error
	calls     []int // the offsets of PendingIDs
	rows      map[string]order.Order
	readErr   error
	inserts   []order.Order
	finals    []order.Order
	insertErr error
	finalErr  error
}

func (f *fakeRepo) PendingIDs(_ context.Context, providerName string, limit, offset int, start, end time.Duration) ([]string, error) {
	if providerName != "parsian" || limit != 2 || start != 30*time.Minute || end != 10*time.Hour {
		return nil, fmt.Errorf("unexpected PendingIDs(%s, %d, %s, %s)", providerName, limit, start, end)
	}
	f.calls = append(f.calls, offset)
	if f.pageErr != nil {
		return nil, f.pageErr
	}
	page := offset / limit
	if page >= len(f.pages) {
		return nil, nil
	}
	return f.pages[page], nil
}

func (f *fakeRepo) Latest(_ context.Context, orderID string, window time.Duration) (order.Order, error) {
	if window != order.LatestWindow {
		return order.Order{}, errors.New("unexpected window")
	}
	if f.readErr != nil {
		return order.Order{}, f.readErr
	}
	o, ok := f.rows[orderID]
	if !ok {
		return order.Order{}, order.ErrNotFound
	}
	return o, nil
}

func (f *fakeRepo) Insert(_ context.Context, o order.Order) error {
	f.inserts = append(f.inserts, o)
	return f.insertErr
}

func (f *fakeRepo) InsertFinal(_ context.Context, o order.Order) error {
	f.finals = append(f.finals, o)
	return f.finalErr
}

// fakeAdapter answers Status from a map by order ID (an error for a missing ID).
type fakeAdapter struct {
	provider.ProviderInterface
	answers map[string]*provider.StatusResponse
	errs    map[string]error
	reqs    []provider.StatusRequest
}

func (a *fakeAdapter) Status(_ context.Context, req provider.StatusRequest) (*provider.StatusResponse, error) {
	a.reqs = append(a.reqs, req)
	if err := a.errs[req.OrderID]; err != nil {
		return nil, err
	}
	return a.answers[req.OrderID], nil
}

type fakeAdapters struct{ a provider.ProviderInterface }

func (f fakeAdapters) Get(name string) (provider.ProviderInterface, bool) {
	if name != "parsian" || f.a == nil {
		return nil, false
	}
	return f.a, true
}

type fakeReporter struct {
	reports []string
	err     error
}

func (f *fakeReporter) Report(_ context.Context, orderID, status string) error {
	f.reports = append(f.reports, orderID+"="+status)
	return f.err
}

type fakeCharger struct {
	charges []int
	err     error
}

func (f *fakeCharger) Charge(_ context.Context, remaining int) error {
	f.charges = append(f.charges, remaining)
	return f.err
}

type logRec struct {
	mu    sync.Mutex
	infos []string
	errs  []string
}

func (l *logRec) Info(msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, msg)
}

func (l *logRec) Error(msg string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, msg)
}

// world is one worker with its fakes.
type world struct {
	repo     *fakeRepo
	adapter  *fakeAdapter
	reporter *fakeReporter
	log      *logRec
	results  map[string]int
	calls    map[string]int
	w        *StatusWorker
}

func newWorld(t *testing.T, deadline time.Duration) *world {
	t.Helper()
	wd := &world{
		repo:     &fakeRepo{rows: map[string]order.Order{}},
		adapter:  &fakeAdapter{answers: map[string]*provider.StatusResponse{}, errs: map[string]error{}},
		reporter: &fakeReporter{},
		log:      &logRec{},
		results:  map[string]int{},
		calls:    map[string]int{},
	}
	w, err := NewStatusWorker(StatusOptions{
		Name: "parsian-getstatus", Provider: "parsian", Interval: 15 * time.Minute, Start: 30 * time.Minute, End: 10 * time.Hour,
		Chunk: 2, ReverseDeadline: deadline, SyncOperations: []time.Duration{time.Second},
		Repository: wd.repo, Adapters: fakeAdapters{wd.adapter}, Reporter: wd.reporter, Logger: wd.log,
		OnOrder: func(name, result string) {
			assert.Equal(t, "parsian-getstatus", name)
			wd.results[result]++
		},
		OnCall: func(providerName, action, result string, d time.Duration) {
			assert.Equal(t, "parsian", providerName)
			assert.Equal(t, "status", action)
			wd.calls[result]++
		},
	})
	if !assert.NoError(t, err) || !assert.NotNil(t, w) {
		t.FailNow()
	}
	w.now = func() time.Time { return now }
	n := 0
	w.newID = func() string { n++; return fmt.Sprintf("new-%d", n) }
	wd.w = w
	return wd
}

// row adds the newest row of an order with the status and the age.
func (wd *world) row(id, status string, age time.Duration) order.Order {
	o := order.Order{ID: "old-" + id, OrderID: id, BillType: "tel", ProviderName: "parsian", Status: status, RRN: "778899",
		Message: "parsianproxy: pay: provider: inquiry is down", StatusCode: 500, CreatedAt: now.Add(-age)}
	wd.repo.rows[id] = o
	return o
}

func TestNewStatusWorkerRules(t *testing.T) {
	base := StatusOptions{Name: "n", Provider: "p", Interval: time.Minute, Start: time.Minute, End: time.Hour, Chunk: 1,
		Repository: &fakeRepo{}, Adapters: fakeAdapters{}, Reporter: &fakeReporter{}}
	for _, change := range []func(*StatusOptions){
		func(o *StatusOptions) { o.Name = "" },
		func(o *StatusOptions) { o.Provider = "" },
		func(o *StatusOptions) { o.Interval = 0 },
		func(o *StatusOptions) { o.Chunk = 0 },
		func(o *StatusOptions) { o.Start = -time.Second },
		func(o *StatusOptions) { o.End = o.Start },
		func(o *StatusOptions) { o.ReverseDeadline = -time.Second },
		func(o *StatusOptions) { o.Repository = nil },
		func(o *StatusOptions) { o.Adapters = nil },
		func(o *StatusOptions) { o.Reporter = nil },
	} {
		o := base
		change(&o)
		_, err := NewStatusWorker(o)
		assert.Error(t, err, "%+v", o)
	}
	w, err := NewStatusWorker(base)
	if assert.NoError(t, err) && assert.NotNil(t, w) {
		assert.Equal(t, "n", w.Name())
		assert.Equal(t, time.Minute, w.Interval())
		assert.Equal(t, 1, w.Chunk())
	}
}

// TestRunPages: pages of the chunk size with the offset growing by the chunk until a page is empty;
// the lock is charged with the size of each page.
func TestRunPages(t *testing.T) {
	wd := newWorld(t, 10*time.Hour)
	wd.repo.pages = [][]string{{"a", "b"}, {"c"}}
	for _, id := range []string{"a", "b", "c"} {
		wd.row(id, order.StatusSuccess, time.Hour) // final already: skipped
	}
	ch := &fakeCharger{}
	assert.NoError(t, wd.w.Run(context.Background(), ch))
	assert.Equal(t, []int{0, 2, 4}, wd.repo.calls)
	assert.Equal(t, []int{2, 1}, ch.charges)
	assert.Equal(t, map[string]int{ResultSkipped: 3}, wd.results)
	assert.Empty(t, wd.adapter.reqs, "a final order is not asked")
}

// TestRunErrors: no adapter, a failed read of the IDs and a lost lock end the run with an error;
// an empty first page ends it with none and charges nothing.
func TestRunErrors(t *testing.T) {
	wd := newWorld(t, 10*time.Hour)
	ch := &fakeCharger{}
	assert.NoError(t, wd.w.Run(context.Background(), ch))
	assert.Empty(t, ch.charges)

	wd.repo.pageErr = errors.New("db down")
	assert.ErrorContains(t, wd.w.Run(context.Background(), ch), "db down")
	wd.repo.pageErr = nil

	wd.repo.pages = [][]string{{"a"}}
	wd.row("a", order.StatusPending, time.Hour)
	assert.ErrorContains(t, wd.w.Run(context.Background(), &fakeCharger{err: errors.New("lock lost")}), "lock lost")
	assert.Empty(t, wd.adapter.reqs, "nothing is asked with a lost lock")

	wd.w.o.Adapters = fakeAdapters{}
	assert.ErrorContains(t, wd.w.Run(context.Background(), ch), "no adapter")

	// A stopped ctx ends the run before the next order.
	wd2 := newWorld(t, 10*time.Hour)
	wd2.repo.pages = [][]string{{"a"}}
	wd2.row("a", order.StatusPending, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, wd2.w.Run(ctx, nil), context.Canceled)
}

// TestCheckResults (task 045 D6): each answer of the provider gives the result of bill.
func TestCheckResults(t *testing.T) {
	wd := newWorld(t, 10*time.Hour)
	wd.repo.pages = [][]string{{"ok", "fail"}, {"wait", "down"}, {"blank", "gone"}}
	okRow := wd.row("ok", order.StatusPending, time.Hour)
	wd.row("fail", order.StatusInProgress, 2*time.Hour)
	wd.row("wait", order.StatusPending, time.Hour)
	wd.row("down", order.StatusPending, time.Hour)
	wd.row("blank", order.StatusPending, time.Hour)
	wd.adapter.answers["ok"] = &provider.StatusResponse{Status: "success", Message: "paid", ProviderTraceID: "99"}
	wd.adapter.answers["fail"] = &provider.StatusResponse{Status: "failed"}
	wd.adapter.answers["wait"] = &provider.StatusResponse{Status: "in-progress"}
	wd.adapter.errs["down"] = fmt.Errorf("x: %w", provider.ErrProviderDown)
	wd.adapter.answers["blank"] = &provider.StatusResponse{}
	// "gone" has no row: a failed read.

	assert.NoError(t, wd.w.Run(context.Background(), &fakeCharger{}))
	assert.Equal(t, map[string]int{ResultSuccess: 1, ResultFailed: 1, ResultPending: 3, ResultReadError: 1}, wd.results)
	assert.Equal(t, map[string]int{"ok": 4, "down": 1}, wd.calls)
	assert.Equal(t, []string{"ok=success", "fail=failed"}, wd.reporter.reports)
	if assert.Len(t, wd.repo.finals, 2) {
		want := okRow
		want.ID, want.Status = "new-1", order.StatusSuccess
		assert.Equal(t, want, wd.repo.finals[0], "a copy of the newest row with the new status only (C2)")
		assert.Equal(t, order.StatusFailed, wd.repo.finals[1].Status)
	}
	assert.Empty(t, wd.repo.inserts)
	if assert.NotEmpty(t, wd.adapter.reqs) {
		assert.Equal(t, provider.StatusRequest{OrderID: "ok", BillType: "tel"}, wd.adapter.reqs[0])
	}
	assert.Equal(t, []string{"parsian status: reported", "parsian status: reported"}, wd.log.infos)
	assert.Empty(t, wd.log.errs, "a pending answer and a failed read write no line")
}

// TestReportAndSaveErrors: a failed report saves nothing (the next run asks again); a failed save
// after the report is save_error; both write one error line.
func TestReportAndSaveErrors(t *testing.T) {
	wd := newWorld(t, 10*time.Hour)
	wd.repo.pages = [][]string{{"a"}}
	wd.row("a", order.StatusPending, time.Hour)
	wd.adapter.answers["a"] = &provider.StatusResponse{Status: "success"}
	wd.reporter.err = errors.New("kafka down")
	assert.NoError(t, wd.w.Run(context.Background(), &fakeCharger{}))
	assert.Equal(t, map[string]int{ResultReportError: 1}, wd.results)
	assert.Empty(t, wd.repo.finals)
	assert.Equal(t, []string{"parsian status: report failed"}, wd.log.errs)

	wd.reporter.err = nil
	wd.repo.finalErr = errors.New("already final")
	assert.NoError(t, wd.w.Run(context.Background(), &fakeCharger{}))
	assert.Equal(t, 1, wd.results[ResultSaveError])
	assert.Equal(t, []string{"a=success", "a=success"}, wd.reporter.reports)
	assert.Equal(t, []string{"parsian status: report failed", "parsian status: save failed"}, wd.log.errs)
}

// TestReverseDeadline (bill): a final status of an order older than the deadline is saved with the
// message reverse_deadline_exceeded (Insert, no final check) and not reported; a deadline of 0
// makes every final status late.
func TestReverseDeadline(t *testing.T) {
	wd := newWorld(t, 10*time.Hour)
	wd.repo.pages = [][]string{{"late", "fresh"}}
	wd.row("late", order.StatusPending, 10*time.Hour+time.Second)
	wd.row("fresh", order.StatusPending, 10*time.Hour-time.Second)
	wd.adapter.answers["late"] = &provider.StatusResponse{Status: "success"}
	wd.adapter.answers["fresh"] = &provider.StatusResponse{Status: "failed"}
	assert.NoError(t, wd.w.Run(context.Background(), &fakeCharger{}))
	assert.Equal(t, map[string]int{ResultLate: 1, ResultFailed: 1}, wd.results)
	assert.Equal(t, []string{"fresh=failed"}, wd.reporter.reports)
	if assert.Len(t, wd.repo.inserts, 1) {
		assert.Equal(t, ReverseDeadlineExceeded, wd.repo.inserts[0].Message)
		assert.Equal(t, order.StatusSuccess, wd.repo.inserts[0].Status)
	}
	assert.Contains(t, wd.log.infos, "parsian status: reverse deadline exceeded")

	zero := newWorld(t, 0)
	zero.repo.pages = [][]string{{"a"}}
	zero.row("a", order.StatusPending, time.Minute)
	zero.adapter.answers["a"] = &provider.StatusResponse{Status: "success"}
	zero.repo.insertErr = errors.New("db down")
	assert.NoError(t, zero.w.Run(context.Background(), &fakeCharger{}))
	assert.Empty(t, zero.reporter.reports, "a deadline of 0: nothing is reported")
	assert.Equal(t, map[string]int{ResultSaveError: 1}, zero.results)
	assert.Equal(t, []string{"parsian status: save failed"}, zero.log.errs)
}

// TestStatusCallResults: the provider call results (ok, down, canceled, error).
func TestStatusCallResults(t *testing.T) {
	wd := newWorld(t, 10*time.Hour)
	o := order.Order{OrderID: "x", BillType: "tel"}

	wd.adapter.errs["x"] = errors.New("not supported")
	assert.Equal(t, order.StatusPending, wd.w.status(context.Background(), wd.adapter, o))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wd.adapter.errs["x"] = context.Canceled
	assert.Equal(t, order.StatusPending, wd.w.status(ctx, wd.adapter, o))
	delete(wd.adapter.errs, "x")
	assert.Equal(t, order.StatusPending, wd.w.status(context.Background(), wd.adapter, o), "no answer")
	assert.Equal(t, map[string]int{"error": 2, "canceled": 1}, wd.calls)
}
