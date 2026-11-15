package order

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// payRepo is a PaymentRepository in memory.
type payRepo struct {
	mu        sync.Mutex
	latest    Order
	latestErr error
	nameErr   error
	inserts   []Order
	insertErr func(o Order) error
	confirmed []string
	confErr   error
	paid      []string
	dates     []string
}

func (r *payRepo) Latest(_ context.Context, orderID string, window time.Duration) (Order, error) {
	if window != LatestWindow {
		return Order{}, fmt.Errorf("window %s", window)
	}
	if r.latestErr != nil {
		return Order{}, r.latestErr
	}
	if orderID != r.latest.OrderID {
		return Order{}, ErrNotFound
	}
	return r.latest, nil
}

func (r *payRepo) ProviderName(ctx context.Context, id int64) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if r.nameErr != nil {
		return "", r.nameErr
	}
	if id == 51 {
		return "parsian", nil
	}
	return "other", nil
}

func (r *payRepo) Insert(_ context.Context, o Order) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.insertErr != nil {
		if err := r.insertErr(o); err != nil {
			return err
		}
	}
	r.inserts = append(r.inserts, o)
	return nil
}

func (r *payRepo) ConfirmProvider(_ context.Context, orderID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.confirmed = append(r.confirmed, orderID)
	return r.confErr
}

func (r *payRepo) MarkInquiryPaid(_ context.Context, inquiryID, payID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paid = append(r.paid, inquiryID+"/"+payID)
	return nil
}

func (r *payRepo) SetLastOrderDate(_ context.Context, billID, billType, date string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dates = append(r.dates, billID+"/"+billType+"/"+date)
	return nil
}

func (r *payRepo) statuses() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var s []string
	for _, o := range r.inserts {
		s = append(s, o.Status)
	}
	return s
}

// payAdapter is a provider whose Pay and Settle give fixed answers.
type payAdapter struct {
	provider.ProviderInterface
	name      string
	payResp   *provider.PaymentResponse
	payErr    error
	payReqs   []provider.PaymentRequest
	settle    *provider.SettleResponse
	settleErr error
	settles   []provider.SettleRequest
}

func (a *payAdapter) Name() string { return a.name }

func (a *payAdapter) Pay(_ context.Context, req provider.PaymentRequest) (*provider.PaymentResponse, error) {
	a.payReqs = append(a.payReqs, req)
	return a.payResp, a.payErr
}

func (a *payAdapter) Settle(_ context.Context, req provider.SettleRequest) (*provider.SettleResponse, error) {
	a.settles = append(a.settles, req)
	return a.settle, a.settleErr
}

type adapters map[string]provider.ProviderInterface

func (m adapters) Get(name string) (provider.ProviderInterface, bool) {
	a, ok := m[name]
	return a, ok
}

type payLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *payLog) Info(msg string, _ ...any) { l.add("info: " + msg) }
func (l *payLog) Error(msg string, _ ...any) {
	l.add("error: " + msg)
}
func (l *payLog) add(s string) { l.mu.Lock(); l.lines = append(l.lines, s); l.mu.Unlock() }

type payFixture struct {
	repo     *payRepo
	parsian  *payAdapter
	karashop *payAdapter
	log      *payLog
	results  []string
	settled  []string
	pays     []string
	p        *Payments
}

var paidAt = time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC)

func newPayFixture(t *testing.T) *payFixture {
	t.Helper()
	f := &payFixture{
		repo: &payRepo{latest: Order{
			ID: "row-1", OrderID: "order-1", UserID: testUser, BillType: provider.BillTypeTelMidterm, BillID: "2101234560040", PayID: "12000035",
			Price: 150000, Data: testTel, ProviderID: 51, ProviderName: "parsian", Status: StatusInitial, Fee: 9000, BasePrice: 120000,
			ProviderFee: 21000, BillInquiryID: "mid-row", CreatedAt: paidAt,
		}},
		parsian:  &payAdapter{name: "parsian", payResp: &provider.PaymentResponse{Status: "success", Code: 200, Message: "ok", ProviderTraceID: "777"}},
		karashop: &payAdapter{name: "karashop", settle: &provider.SettleResponse{Status: provider.StatusSuccess, Code: 200}},
		log:      &payLog{},
	}
	p, err := NewPayments(PaymentOptions{
		Repository: f.repo,
		Adapters:   adapters{"parsian": f.parsian, "karashop": f.karashop},
		PayRoutes:  map[string]map[string]bool{"parsian": {provider.BillTypeTel: true, provider.BillTypeTelMidterm: true}},
		SettleRoutes: map[string]map[string]string{
			provider.BillTypeTel:        {"parsian": "karashop", "eways": "karashop", "eways2": "karashop"},
			provider.BillTypeTelMidterm: {"parsian": "karashop", "eways": "karashop", "eways2": "karashop"},
		},
		Logger:    f.log,
		OnPayment: func(bt, pro, r string) { f.results = append(f.results, bt+"/"+pro+"/"+r) },
		OnPay:     func(pro, status string, _ time.Duration) { f.pays = append(f.pays, pro+"/"+status) },
		OnSettle:  func(bt, pro, r string) { f.settled = append(f.settled, bt+"/"+pro+"/"+r) },
	})
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	n := 0
	p.newID = func() string { n++; return fmt.Sprintf("new-%d", n) }
	p.now = func() time.Time { return time.Date(2026, 11, 14, 21, 0, 0, 0, time.UTC) } // 00:30 on the 15th in Tehran
	p.spawn = func(job func()) { job() }                                               // at once, for the test
	f.p = p
	return f
}

func completeReq() CompleteRequest {
	return CompleteRequest{OrderID: "order-1", RRN: "778899", Price: 150000}
}

// TestCompleteSuccess: in-progress, then success; the payment request has the order fields; the
// background job marks the inquiry paid, sets the last order date (Tehran) and settles with
// Karashop; provider_confirmed is set.
func TestCompleteSuccess(t *testing.T) {
	f := newPayFixture(t)
	o, err := f.p.Complete(context.Background(), completeReq())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, StatusSuccess, o.Status)
	assert.Equal(t, "777", o.RefID)
	assert.Equal(t, []string{StatusInProgress, StatusSuccess}, f.repo.statuses())
	for i, row := range f.repo.inserts {
		assert.Equal(t, fmt.Sprintf("new-%d", i+1), row.ID, "a new uuid for each row")
		assert.Equal(t, "778899", row.RRN)
		assert.Equal(t, "order-1", row.OrderID)
		assert.Equal(t, uint32(150000), row.Price)
	}
	assert.Equal(t, "777", f.repo.inserts[1].RefID)
	assert.Equal(t, "ok", f.repo.inserts[1].Message)
	assert.Equal(t, int32(200), f.repo.inserts[1].StatusCode)
	assert.Empty(t, f.repo.inserts[0].Message, "in-progress keeps the empty message of the initial row")
	if assert.Len(t, f.parsian.payReqs, 1) {
		assert.Equal(t, provider.PaymentRequest{
			BillType: provider.BillTypeTelMidterm, InquiryType: provider.InquiryTypeMidterm, OrderID: "order-1", UserID: testUser,
			InquiryID: testTel, BillID: "2101234560040", PayID: "12000035", Amount: 120000, RRN: "778899",
		}, f.parsian.payReqs[0])
	}
	assert.Equal(t, []string{testTel + "/12000035"}, f.repo.paid)
	assert.Equal(t, []string{"2101234560040/telmidterm/2026-11-15"}, f.repo.dates)
	if assert.Len(t, f.karashop.settles, 1) {
		assert.Equal(t, provider.SettleRequest{
			BillType: provider.BillTypeTelMidterm, TelephoneNumber: testTel, Amount: 120000, BillID: "2101234560040",
			PayID: "12000035", RRN: "778899", PaidAt: paidAt, PaymentProvider: "parsian",
		}, f.karashop.settles[0])
	}
	assert.Equal(t, []string{"order-1"}, f.repo.confirmed)
	assert.Equal(t, []string{"telmidterm/parsian/success"}, f.results)
	assert.Equal(t, []string{"parsian/success"}, f.pays)
	assert.Equal(t, []string{"telmidterm/karashop/success"}, f.settled)
	assert.Equal(t, []string{"info: settle: karashop confirmed"}, f.log.lines)
}

// TestCompleteSettleFails: a Karashop error, a failed or pending answer, a failed confirm: the
// payment stays success, provider_confirmed is not set, one error line, the metric has the result.
func TestCompleteSettleFails(t *testing.T) {
	cases := []struct {
		name   string
		resp   *provider.SettleResponse
		err    error
		confE  error
		result string
	}{
		{"error", nil, errors.New("down"), nil, SettleError},
		{"failed", &provider.SettleResponse{Status: provider.StatusFailed, Code: 400}, nil, nil, provider.StatusFailed},
		{"pending", &provider.SettleResponse{Status: provider.StatusPending, Code: 500}, nil, nil, provider.StatusPending},
		{"nil answer", nil, nil, nil, SettleError},
		{"confirm fails", &provider.SettleResponse{Status: provider.StatusSuccess}, nil, errors.New("db"), SettleMarkFailed},
	}
	for _, c := range cases {
		f := newPayFixture(t)
		f.karashop.settle, f.karashop.settleErr, f.repo.confErr = c.resp, c.err, c.confE
		o, err := f.p.Complete(context.Background(), completeReq())
		if assert.NoError(t, err, c.name) {
			assert.Equal(t, StatusSuccess, o.Status, c.name)
		}
		assert.Equal(t, []string{"telmidterm/karashop/" + c.result}, f.settled, c.name)
		assert.Len(t, f.log.lines, 1, c.name)
		assert.Contains(t, f.log.lines[0], "error: settle:", c.name)
		if c.confE == nil {
			assert.Empty(t, f.repo.confirmed, c.name)
		}
	}
}

// TestCompleteStatuses: failed and pending answers are saved and returned, no background job; an
// unknown status is pending; a Pay error is pending (500, the error text).
func TestCompleteStatuses(t *testing.T) {
	for status, want := range map[string]string{"failed": StatusFailed, "pending": StatusPending, "done": StatusPending, "": StatusPending} {
		f := newPayFixture(t)
		f.parsian.payResp = &provider.PaymentResponse{Status: status, Code: 400, Message: "m"}
		o, err := f.p.Complete(context.Background(), completeReq())
		if assert.NoError(t, err, status) {
			assert.Equal(t, want, o.Status, status)
			assert.Equal(t, "m", o.Message, status)
		}
		assert.Equal(t, []string{StatusInProgress, want}, f.repo.statuses(), status)
		assert.Empty(t, f.karashop.settles, status)
		assert.Empty(t, f.repo.paid, status)
	}
	f := newPayFixture(t)
	f.parsian.payResp, f.parsian.payErr = nil, fmt.Errorf("x: %w", provider.ErrProviderDown)
	o, err := f.p.Complete(context.Background(), completeReq())
	if assert.NoError(t, err) {
		assert.Equal(t, StatusPending, o.Status)
		assert.Equal(t, int32(500), o.StatusCode)
		assert.Contains(t, o.Message, "inquiry is down")
	}
	assert.Equal(t, []string{"parsian/pending"}, f.pays)
}

// TestCompleteChecks: a processed order and a wrong price answer pending with bill's text, with no
// row and no provider call; a missing order is ErrPaymentDatabase.
func TestCompleteChecks(t *testing.T) {
	for _, status := range []string{StatusInProgress, StatusSuccess, StatusFailed, StatusPending} {
		f := newPayFixture(t)
		f.repo.latest.Status = status
		o, err := f.p.Complete(context.Background(), completeReq())
		if assert.NoError(t, err, status) {
			assert.Equal(t, Order{Status: StatusPending, Message: MessageProcessed}, o, status)
		}
		assert.Empty(t, f.repo.inserts, status)
		assert.Empty(t, f.parsian.payReqs, status)
		assert.Equal(t, []string{"telmidterm/parsian/processed"}, f.results, status)
	}
	f := newPayFixture(t)
	req := completeReq()
	req.Price = 150001
	o, err := f.p.Complete(context.Background(), req)
	if assert.NoError(t, err) {
		assert.Equal(t, Order{Status: StatusPending, Message: MessageWrongPrice}, o)
	}
	assert.Empty(t, f.repo.inserts)
	assert.Empty(t, f.parsian.payReqs)

	f = newPayFixture(t)
	req = completeReq()
	req.OrderID = "other"
	_, err = f.p.Complete(context.Background(), req)
	assert.ErrorIs(t, err, ErrPaymentDatabase)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Empty(t, f.log.lines, "a missing order is not logged as a fault")

	f = newPayFixture(t)
	f.repo.latestErr = errors.New("db down")
	_, err = f.p.Complete(context.Background(), completeReq())
	assert.ErrorIs(t, err, ErrPaymentDatabase)
	assert.Equal(t, []string{"error: order complete: read the order failed"}, f.log.lines)
	assert.Equal(t, []string{"/none/error"}, f.results)
}

// TestCompleteFailedSteps: bill saves failed for a failed provider read, a failed in-progress write
// and no payment route; a failed write of the final status answers pending with no row.
func TestCompleteFailedSteps(t *testing.T) {
	f := newPayFixture(t)
	f.repo.nameErr = errors.New("no provider")
	_, err := f.p.Complete(context.Background(), completeReq())
	assert.ErrorIs(t, err, ErrPaymentDatabase)
	if assert.Len(t, f.repo.inserts, 1) {
		assert.Equal(t, StatusFailed, f.repo.inserts[0].Status)
		assert.Equal(t, MessageNoProviderRow, f.repo.inserts[0].Message)
		assert.Equal(t, int32(400), f.repo.inserts[0].StatusCode)
		assert.Empty(t, f.repo.inserts[0].RRN, "the RRN is not set yet")
	}

	f = newPayFixture(t)
	f.repo.insertErr = func(o Order) error {
		if o.Status == StatusInProgress {
			return errors.New("write failed")
		}
		return nil
	}
	_, err = f.p.Complete(context.Background(), completeReq())
	assert.ErrorIs(t, err, ErrPaymentDatabase)
	assert.Equal(t, []string{StatusFailed}, f.repo.statuses())
	assert.Equal(t, MessageInsertFailed, f.repo.inserts[0].Message)
	assert.Empty(t, f.parsian.payReqs)

	for name, mod := range map[string]func(f *payFixture){
		"no route":   func(f *payFixture) { f.repo.latest.BillType = provider.BillTypeIrancell },
		"raw order":  func(f *payFixture) { f.repo.latest.IsRaw = true },
		"no adapter": func(f *payFixture) { f.p.adapters = adapters{} },
	} {
		f := newPayFixture(t)
		mod(f)
		_, err := f.p.Complete(context.Background(), completeReq())
		assert.ErrorIs(t, err, ErrPaymentRoute, name)
		assert.Equal(t, []string{StatusInProgress, StatusFailed}, f.repo.statuses(), name)
		assert.Equal(t, MessageNoPaymentRoute, f.repo.inserts[1].Message, name)
		assert.Empty(t, f.parsian.payReqs, name)
	}

	f = newPayFixture(t)
	f.repo.insertErr = func(o Order) error {
		if o.Status == StatusSuccess {
			return errors.New("write failed")
		}
		return nil
	}
	o, err := f.p.Complete(context.Background(), completeReq())
	if assert.NoError(t, err) {
		assert.Equal(t, StatusPending, o.Status)
		assert.Equal(t, MessageInsertFailed, o.Message)
	}
	assert.Equal(t, []string{StatusInProgress}, f.repo.statuses())
	assert.Empty(t, f.karashop.settles, "no settlement: the success is not saved")
}

// TestCompleteClientLeaves: the payment service leaves during the payment: the status of the answer
// is still saved (the writes do not take the cancel).
func TestCompleteClientLeaves(t *testing.T) {
	f := newPayFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	f.repo.insertErr = func(Order) error { cancel(); return nil }
	ctxRepo := &ctxCheckRepo{payRepo: f.repo}
	f.p.repo = ctxRepo
	o, err := f.p.Complete(ctx, completeReq())
	assert.NoError(t, err)
	assert.Equal(t, StatusSuccess, o.Status)
	assert.Equal(t, []string{StatusInProgress, StatusSuccess}, f.repo.statuses())
	assert.Zero(t, ctxRepo.canceledWrites, "no write saw a canceled context")
}

type ctxCheckRepo struct {
	*payRepo
	canceledWrites int
}

func (r *ctxCheckRepo) Insert(ctx context.Context, o Order) error {
	if ctx.Err() != nil {
		r.canceledWrites++
	}
	return r.payRepo.Insert(ctx, o)
}

// TestCompleteNoSettleRoute: an order provider with no settlement route, or no inquiry ID: no
// settlement; the side effects still run.
func TestCompleteNoSettleRoute(t *testing.T) {
	f := newPayFixture(t)
	f.p.settleRoutes = nil
	_, err := f.p.Complete(context.Background(), completeReq())
	assert.NoError(t, err)
	assert.Empty(t, f.karashop.settles)
	assert.Len(t, f.repo.paid, 1)

	f = newPayFixture(t)
	f.repo.latest.Data = ""
	_, err = f.p.Complete(context.Background(), completeReq())
	assert.NoError(t, err)
	assert.Empty(t, f.karashop.settles)
	assert.Equal(t, []string{"telmidterm/karashop/no_data"}, f.settled)
}

// TestPaymentsStop: Stop waits for a running job; after Stop a success starts no job (one line); a
// Stop whose context ends first returns an error.
func TestPaymentsStop(t *testing.T) {
	f := newPayFixture(t)
	release := make(chan struct{})
	started := make(chan struct{})
	f.p.spawn = func(job func()) { go func() { close(started); <-release; job() }() }
	_, err := f.p.Complete(context.Background(), completeReq())
	assert.NoError(t, err)
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	assert.Error(t, f.p.Stop(ctx), "the job is still running")
	cancel()
	close(release)
	assert.NoError(t, f.p.Stop(context.Background()))
	assert.Len(t, f.karashop.settles, 1)

	f.repo.latest.Status = StatusInitial
	f.repo.inserts = nil
	_, err = f.p.Complete(context.Background(), completeReq())
	assert.NoError(t, err)
	assert.Len(t, f.karashop.settles, 1, "no new job after Stop")
	assert.Contains(t, f.log.lines, "error: order complete: stopping, the settlement is not started")
	var nilP *Payments
	assert.NoError(t, nilP.Stop(context.Background()))
}

// TestSettlePanic: a panic in the job is recovered and logged.
func TestSettlePanic(t *testing.T) {
	f := newPayFixture(t)
	f.p.adapters = adapters{"parsian": f.parsian, "karashop": panicAdapter{}}
	_, err := f.p.Complete(context.Background(), completeReq())
	assert.NoError(t, err)
	assert.Contains(t, f.log.lines, "error: order complete: settlement panic")
}

type panicAdapter struct{ provider.ProviderInterface }

func (panicAdapter) Settle(context.Context, provider.SettleRequest) (*provider.SettleResponse, error) {
	panic("boom")
}

func TestNewPaymentsRejects(t *testing.T) {
	_, err := NewPayments(PaymentOptions{Adapters: adapters{}})
	assert.Error(t, err)
	_, err = NewPayments(PaymentOptions{Repository: &payRepo{}})
	assert.Error(t, err)
	_, err = NewPayments(PaymentOptions{Repository: &payRepo{}, Adapters: adapters{}, SettleTimeout: -1})
	assert.Error(t, err)
	var nilP *Payments
	_, err = nilP.Complete(context.Background(), completeReq())
	assert.Error(t, err)
}

// TestCompleteClientLeftBeforeTheProviderRead: the payment service left after the checks: the
// provider read and the writes still run, so the order is paid and saved, not saved as failed.
func TestCompleteClientLeftBeforeTheProviderRead(t *testing.T) {
	f := newPayFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	f.p.repo = &cancelAfterLatest{payRepo: f.repo, cancel: cancel}
	o, err := f.p.Complete(ctx, completeReq())
	assert.NoError(t, err)
	assert.Equal(t, StatusSuccess, o.Status, "the mock adapter ignores the canceled context")
	assert.Equal(t, []string{StatusInProgress, StatusSuccess}, f.repo.statuses())
}

type cancelAfterLatest struct {
	*payRepo
	cancel context.CancelFunc
}

func (r *cancelAfterLatest) Latest(ctx context.Context, orderID string, window time.Duration) (Order, error) {
	o, err := r.payRepo.Latest(ctx, orderID, window)
	r.cancel()
	return o, err
}
