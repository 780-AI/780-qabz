package inquiry

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

const (
	validBill = "1234567890125" // passes the check digit rule
	userA     = "6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11"
	userB     = "0b9d2c4e-8a3f-4e61-b7d2-5c1a9e0f3d22"
)

// memRow is a saved row with the columns that the lookup filters on.
type memRow struct {
	Row
	createdAt time.Time
	isPaid    bool
}

// memRepo is an in-memory Repository. FindValid applies the rules of the SQL lookup: the same
// inquiry_id, the same provider, newer than ttl, not a copy, not paid, newest first, at most 2.
type memRepo struct {
	mu       sync.Mutex
	rows     []memRow
	requests []RequestRow
	finished map[string][2]string // id -> status, trace

	findErr, saveErr, createErr, finishErr error
	findCalls, saveCalls                   int
	order                                  []string // calls in order
}

func newMemRepo() *memRepo { return &memRepo{finished: map[string][2]string{}} }

func (m *memRepo) FindValid(_ context.Context, inquiryID string, ttl time.Duration, providerID int64) ([]Row, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.findCalls++
	if m.findErr != nil {
		return nil, m.findErr
	}
	var out []Row
	for i := len(m.rows) - 1; i >= 0; i-- { // newest first
		r := m.rows[i]
		if r.InquiryID == inquiryID && r.ProviderID == providerID && time.Since(r.createdAt) < ttl && !r.IsCached && !r.isPaid {
			out = append(out, r.Row)
		}
		if len(out) == 2 {
			break
		}
	}
	return out, nil
}

func (m *memRepo) SaveInquiries(_ context.Context, rows []Row) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saveCalls++
	m.order = append(m.order, "save")
	if m.saveErr != nil {
		return m.saveErr
	}
	for _, r := range rows {
		m.rows = append(m.rows, memRow{Row: r, createdAt: time.Now()})
	}
	return nil
}

func (m *memRepo) CreateRequest(_ context.Context, r RequestRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.order = append(m.order, "create")
	if m.createErr != nil {
		return m.createErr
	}
	m.requests = append(m.requests, r)
	return nil
}

func (m *memRepo) FinishRequest(_ context.Context, id, status, trace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.order = append(m.order, "finish")
	if m.finishErr != nil {
		return m.finishErr
	}
	m.finished[id] = [2]string{status, trace}
	return nil
}

// seed adds a saved row.
func (m *memRepo) seed(r Row, age time.Duration, paid bool) {
	m.rows = append(m.rows, memRow{Row: r, createdAt: time.Now().Add(-age), isPaid: paid})
}

type fakeSelector struct {
	p   provider.Provider
	err error
}

func (f fakeSelector) PickProvider(billType, action string) (provider.Provider, error) {
	if billType != "water" || action != "inquiry" {
		return provider.Provider{}, errors.New("unexpected pick")
	}
	return f.p, f.err
}

type fakeAdapters map[string]provider.ProviderInterface

func (f fakeAdapters) Get(name string) (provider.ProviderInterface, bool) {
	p, ok := f[name]
	return p, ok
}

// stubProvider answers Inquiry with resp or err and counts the calls.
type stubProvider struct {
	provider.ProviderInterface
	name  string
	resp  *provider.InquiryResponse
	err   error
	calls int
	reqs  []provider.InquiryRequest
	run   func(ctx context.Context)
	// unsupported is a bill type that SupportsInquiry refuses ("": every bill type is supported).
	unsupported string
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) SupportsInquiry(billType string) bool {
	return s.unsupported == "" || billType != s.unsupported
}
func (s *stubProvider) Inquiry(ctx context.Context, req provider.InquiryRequest) (*provider.InquiryResponse, error) {
	s.calls++
	s.reqs = append(s.reqs, req)
	if s.run != nil {
		s.run(ctx)
	}
	return s.resp, s.err
}

type logLine struct {
	level, msg string
	kv         map[string]any
}

type recLog struct {
	mu    sync.Mutex
	lines []logLine
}

func (l *recLog) add(level, msg string, kv []any) {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, logLine{level, msg, m})
}
func (l *recLog) Info(msg string, kv ...any)  { l.add("info", msg, kv) }
func (l *recLog) Error(msg string, kv ...any) { l.add("error", msg, kv) }
func (l *recLog) count(level string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, x := range l.lines {
		if x.level == level {
			n++
		}
	}
	return n
}

type call struct {
	provider, action, result string
	d                        time.Duration
}

type rig struct {
	svc      *Service
	repo     *memRepo
	prov     *stubProvider
	log      *recLog
	inquiry  []string // "billType/provider/result"
	calls    []call
	selector fakeSelector
}

var qabzino = provider.Provider{ID: 36, Name: "qabzino", BillType: "water", Action: "inquiry", Status: provider.StatusEnable}

func successResp() *provider.InquiryResponse {
	return &provider.InquiryResponse{
		Status: provider.ResponseSuccess, ProviderTraceID: "trace-1", BillID: validBill,
		Inquiries: []provider.InquiryItem{{PayID: "2500012345", Amount: 25000000, PaymentDeadline: "2030/01/02", InquiryType: "regular"}},
	}
}

func newRig(t *testing.T, ttl time.Duration) *rig {
	t.Helper()
	return newRigStore(t, ttl, false)
}

// newRigStore is newRig with Options.NoStore (task 031).
func newRigStore(t *testing.T, ttl time.Duration, noStore bool) *rig {
	t.Helper()
	r := &rig{repo: newMemRepo(), log: &recLog{}, selector: fakeSelector{p: qabzino}}
	r.prov = &stubProvider{name: "qabzino", resp: successResp()}
	svc, err := NewService(Options{
		Repository: r.repo, Providers: r.selector, Adapters: fakeAdapters{"qabzino": r.prov},
		CacheTTL: func(string) time.Duration { return ttl }, Logger: r.log, NoStore: noStore,
		OnInquiry: func(b, p, res string) { r.inquiry = append(r.inquiry, b+"/"+p+"/"+res) },
		OnProviderCall: func(p, a, res string, d time.Duration) {
			r.calls = append(r.calls, call{p, a, res, d})
		},
	})
	if !assert.NoError(t, err) || !assert.NotNil(t, svc) {
		t.FailNow()
	}
	n := 0
	svc.newID = func() string { n++; return "id-" + string(rune('a'+n)) }
	svc.newTID = func(string) (string, error) { return "tid-1", nil }
	r.svc = svc
	return r
}

func (r *rig) ask(user string) (Result, error) {
	return r.svc.Inquire(context.Background(), Request{BillType: "water", InquiryID: validBill, UserID: user, Dialer: "989121234567"})
}

func TestNewServiceRequiresDependencies(t *testing.T) {
	r := newRig(t, time.Hour)
	for _, o := range []Options{
		{Providers: r.selector, Adapters: fakeAdapters{}},
		{Repository: r.repo, Adapters: fakeAdapters{}},
		{Repository: r.repo, Providers: r.selector},
	} {
		_, err := NewService(o)
		assert.Error(t, err)
	}
	var nilSvc *Service
	_, err := nilSvc.Inquire(context.Background(), Request{})
	assert.Error(t, err)
	_, err = (&Service{}).Inquire(context.Background(), Request{})
	assert.Error(t, err)
}

// No saved row: one provider call, the request row and the inquiry rows are saved, and the next
// call within the TTL is answered from the database.
func TestMissCallsProviderOnceAndSaves(t *testing.T) {
	r := newRig(t, time.Hour)
	res, err := r.ask(userA)
	if !assert.NoError(t, err) || !assert.Len(t, res.Inquiries, 1) {
		return
	}
	assert.False(t, res.FromDatabase)
	assert.Equal(t, validBill, res.BillID)
	in := res.Inquiries[0]
	assert.Equal(t, "2500012345", in.PayID)
	assert.Equal(t, uint64(25000000), in.Amount)
	assert.Equal(t, "2030-01-02", in.PaymentDeadline, "the date is fixed to yyyy-mm-dd")
	assert.Equal(t, 1, r.prov.calls)
	if assert.Len(t, r.prov.reqs, 1) {
		assert.Equal(t, provider.InquiryRequest{BillType: "water", UserID: userA, Dialer: "989121234567", InquiryID: validBill}, r.prov.reqs[0])
	}
	// The request row: INITIAL first, then the provider status and trace.
	if assert.Len(t, r.repo.requests, 1) {
		q := r.repo.requests[0]
		assert.Equal(t, RequestInitial, q.Status)
		assert.Equal(t, "tid-1", q.TID)
		assert.Equal(t, "qabzino", q.ProviderName)
		assert.Equal(t, userA, q.UserID)
		assert.Equal(t, "989121234567", q.Dialer)
		assert.Equal(t, validBill, q.InquiryID)
		assert.Equal(t, [2]string{"success", "trace-1"}, r.repo.finished[q.ID])
	}
	assert.Equal(t, []string{"create", "finish", "save"}, r.repo.order, "the request row is final before the inquiries are saved, as in bill")
	// The saved inquiry row.
	if assert.Len(t, r.repo.rows, 1) {
		row := r.repo.rows[0].Row
		assert.Equal(t, Row{ID: in.ID, UserID: userA, InquiryID: validBill, ProviderID: 36, ProviderName: "qabzino", BillType: "water",
			ProviderTraceID: "trace-1", InquiryType: "regular", BillID: validBill, PayID: "2500012345", PaymentDeadline: "2030-01-02", Amount: 25000000}, row)
	}
	assert.Equal(t, []string{"water/qabzino/provider_ok"}, r.inquiry)
	if assert.Len(t, r.calls, 1) {
		assert.Equal(t, "qabzino", r.calls[0].provider)
		assert.Equal(t, "inquiry", r.calls[0].action)
		assert.Equal(t, CallOK, r.calls[0].result)
	}
	assert.Equal(t, 1, r.log.count("info"))
	assert.Equal(t, 0, r.log.count("error"))

	// The second call, within the TTL, for another user: from the database, no provider call.
	res2, err := r.ask(userB)
	if !assert.NoError(t, err) || !assert.Len(t, res2.Inquiries, 1) {
		return
	}
	assert.True(t, res2.FromDatabase)
	assert.Equal(t, 1, r.prov.calls, "no second provider call")
	assert.Len(t, r.repo.requests, 1, "no new request row")
	assert.Equal(t, in.PayID, res2.Inquiries[0].PayID)
	assert.Equal(t, in.Amount, res2.Inquiries[0].Amount)
	assert.Equal(t, in.PaymentDeadline, res2.Inquiries[0].PaymentDeadline)
	assert.Equal(t, validBill, res2.BillID)
	// A copy for user B, marked as a copy; it does not answer later calls.
	if assert.Len(t, r.repo.rows, 2) {
		cp := r.repo.rows[1].Row
		assert.True(t, cp.IsCached)
		assert.Equal(t, userB, cp.UserID)
		assert.Equal(t, res2.Inquiries[0].ID, cp.ID)
		assert.NotEqual(t, in.ID, cp.ID)
	}
	assert.Equal(t, []string{"water/qabzino/provider_ok", "water/qabzino/db_hit"}, r.inquiry)
	assert.Len(t, r.calls, 1, "a database answer is not a provider call")
	res3, err := r.ask(userA)
	assert.NoError(t, err)
	assert.True(t, res3.FromDatabase)
	assert.Equal(t, 1, r.prov.calls)
	assert.Len(t, r.repo.rows, 3, "copies are never used to answer: only the original row is")
}

func TestValidRowAnswersWithNoProviderCall(t *testing.T) {
	r := newRig(t, time.Hour)
	r.repo.seed(Row{ID: "x", UserID: userB, InquiryID: validBill, ProviderID: 36, ProviderName: "qabzino", BillType: "water",
		BillID: validBill, PayID: "1000012345", PaymentDeadline: "1405/01/02", Amount: 5000}, 10*time.Minute, false)
	res, err := r.ask(userA)
	if !assert.NoError(t, err) || !assert.Len(t, res.Inquiries, 1) {
		return
	}
	assert.True(t, res.FromDatabase)
	assert.Equal(t, 0, r.prov.calls)
	assert.Empty(t, r.repo.requests)
	assert.Equal(t, uint64(5000), res.Inquiries[0].Amount)
	assert.Equal(t, "1405-01-02", res.Inquiries[0].PaymentDeadline)
}

func TestRowsThatDoNotAnswer(t *testing.T) {
	base := Row{ID: "x", UserID: userB, InquiryID: validBill, ProviderID: 36, ProviderName: "qabzino", BillType: "water", BillID: validBill, PayID: "p", Amount: 1}
	cases := []struct {
		name string
		mod  func(*Row)
		age  time.Duration
		paid bool
	}{
		{"expired", func(*Row) {}, 2 * time.Hour, false},
		{"another provider", func(r *Row) { r.ProviderID = 44 }, time.Minute, false},
		{"paid", func(*Row) {}, time.Minute, true},
		{"a copy", func(r *Row) { r.IsCached = true }, time.Minute, false},
		{"another bill", func(r *Row) { r.InquiryID = "9999999999999" }, time.Minute, false},
	}
	for _, c := range cases {
		r := newRig(t, time.Hour)
		row := base
		c.mod(&row)
		r.repo.seed(row, c.age, c.paid)
		res, err := r.ask(userA)
		if assert.NoError(t, err, c.name) {
			assert.False(t, res.FromDatabase, c.name)
		}
		assert.Equal(t, 1, r.prov.calls, c.name+": the provider is called")
	}
}

func TestCacheOff(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Second} {
		r := newRig(t, ttl)
		r.repo.seed(Row{ID: "x", InquiryID: validBill, ProviderID: 36, BillID: validBill, PayID: "p"}, time.Second, false)
		_, err := r.ask(userA)
		assert.NoError(t, err)
		assert.Equal(t, 1, r.prov.calls, "ttl %v: no cache", ttl)
		assert.Equal(t, 0, r.repo.findCalls, "ttl %v: no lookup", ttl)
	}
	// A nil CacheTTL is the same.
	r := newRig(t, time.Hour)
	svc, err := NewService(Options{Repository: r.repo, Providers: r.selector, Adapters: fakeAdapters{"qabzino": r.prov}})
	if assert.NoError(t, err) {
		_, err = svc.Inquire(context.Background(), Request{BillType: "water", InquiryID: validBill, UserID: userA})
		assert.NoError(t, err)
		assert.Equal(t, 0, r.repo.findCalls)
	}
}

func TestInvalidRequests(t *testing.T) {
	r := newRig(t, time.Hour)
	for name, req := range map[string]Request{
		"empty bill":      {BillType: "water", InquiryID: "", UserID: userA},
		"short bill":      {BillType: "water", InquiryID: "12345", UserID: userA},
		"letters":         {BillType: "water", InquiryID: "12345678901ab", UserID: userA},
		"bad check digit": {BillType: "water", InquiryID: "1234567890127", UserID: userA},
		"no user":         {BillType: "water", InquiryID: validBill, UserID: ""},
		"bad user":        {BillType: "water", InquiryID: validBill, UserID: "not-a-uuid"},
		"upper-case user": {BillType: "water", InquiryID: validBill, UserID: "6F1C1E0A-3B7E-4C2B-9A51-2F0F6D1D8C11"},
	} {
		_, err := r.svc.Inquire(context.Background(), req)
		assert.ErrorIs(t, err, ErrInvalid, name)
	}
	assert.Equal(t, 0, r.prov.calls)
	assert.Empty(t, r.repo.order, "nothing is written")
	assert.Empty(t, r.inquiry, "an invalid request is not an inquiry result")
	_, err := r.svc.Inquire(context.Background(), Request{BillType: "gas", InquiryID: validBill, UserID: userA})
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalid, "an unsupported bill type is a code fault")
}

func TestNoProvider(t *testing.T) {
	r := newRig(t, time.Hour)
	r.svc.providers = fakeSelector{err: provider.ErrNoProvider}
	_, err := r.ask(userA)
	assert.ErrorIs(t, err, provider.ErrNoProvider)
	assert.Equal(t, []string{"water/none/no_provider"}, r.inquiry)
	assert.Equal(t, 0, r.prov.calls)
	assert.Empty(t, r.repo.order)

	r.inquiry = nil
	r.svc.providers = fakeSelector{err: errors.New("boom")}
	_, err = r.ask(userA)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, provider.ErrNoProvider)
	assert.Equal(t, []string{"water/none/error"}, r.inquiry)
}

func TestNoAdapter(t *testing.T) {
	r := newRig(t, time.Hour)
	r.svc.adapters = fakeAdapters{}
	_, err := r.ask(userA)
	assert.ErrorIs(t, err, ErrNoAdapter)
	assert.Equal(t, []string{"water/qabzino/error"}, r.inquiry)
	assert.Empty(t, r.repo.order)
	r.svc.adapters = fakeAdapters{"qabzino": nil}
	_, err = r.ask(userA)
	assert.ErrorIs(t, err, ErrNoAdapter, "a nil adapter")
}

// An adapter that does not inquire the bill type (task 036: the seed picks parsian for water, its
// adapter answers only mobile) gives the answer of no adapter: no call, no row, and a valid saved
// row of the provider does not answer.
func TestAdapterDoesNotSupportBillType(t *testing.T) {
	r := newRig(t, time.Hour)
	r.prov.unsupported = provider.BillTypeWater
	r.repo.seed(Row{ID: "x", InquiryID: validBill, ProviderID: 36, BillID: validBill, PayID: "p"}, time.Second, false)
	_, err := r.ask(userA)
	assert.ErrorIs(t, err, ErrNoAdapter)
	assert.ErrorContains(t, err, `"qabzino" for the bill type "water"`)
	assert.Equal(t, []string{"water/qabzino/error"}, r.inquiry)
	assert.Equal(t, 0, r.prov.calls, "Inquiry is not called")
	assert.Empty(t, r.repo.order, "no request row, no saved inquiry")
	assert.Empty(t, r.calls, "no provider call is counted")
}

func TestProviderDown(t *testing.T) {
	r := newRig(t, time.Hour)
	r.prov.resp, r.prov.err = nil, errors.Join(provider.ErrProviderDown, errors.New("connection refused"))
	_, err := r.ask(userA)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Equal(t, []string{"water/qabzino/provider_down"}, r.inquiry)
	if assert.Len(t, r.repo.requests, 1) {
		assert.Equal(t, [2]string{"failed", ""}, r.repo.finished[r.repo.requests[0].ID], "the request row has the failed status")
	}
	assert.Empty(t, r.repo.rows, "no inquiry is saved")
	if assert.Len(t, r.calls, 1) {
		assert.Equal(t, CallDown, r.calls[0].result)
	}
}

// The valid row of an earlier answer still answers when the provider is down now.
func TestValidRowAnswersWhileProviderDown(t *testing.T) {
	r := newRig(t, time.Hour)
	r.prov.resp, r.prov.err = nil, provider.ErrProviderDown
	r.repo.seed(Row{ID: "x", InquiryID: validBill, ProviderID: 36, BillID: validBill, PayID: "p", Amount: 9}, time.Minute, false)
	res, err := r.ask(userA)
	assert.NoError(t, err)
	assert.True(t, res.FromDatabase)
	assert.Equal(t, 0, r.prov.calls)
}

func TestProviderRefused(t *testing.T) {
	for name, resp := range map[string]*provider.InquiryResponse{
		"failed status":     {Status: "failed", Code: 4, Message: "bill not found", ProviderTraceID: "t7"},
		"success no items":  {Status: "success", Message: "x", ProviderTraceID: "t7"},
		"items not success": {Status: "failed", Message: "no", Inquiries: []provider.InquiryItem{{PayID: "p"}}},
	} {
		r := newRig(t, time.Hour)
		r.prov.resp = resp
		_, err := r.ask(userA)
		var refused *RefusedError
		if assert.ErrorAs(t, err, &refused, name) && assert.NotNil(t, refused, name) {
			assert.Equal(t, resp.Message, refused.Message, name)
			assert.Equal(t, resp.Code, refused.Code, name)
		}
		assert.Equal(t, []string{"water/qabzino/provider_refused"}, r.inquiry, name)
		if assert.Len(t, r.repo.requests, 1, name) {
			assert.Equal(t, [2]string{resp.Status, resp.ProviderTraceID}, r.repo.finished[r.repo.requests[0].ID], name)
		}
		assert.Empty(t, r.repo.rows, name+": no inquiry is saved")
		assert.Equal(t, 0, r.log.count("error"), name+": a refused inquiry is not a server fault")
		if assert.Len(t, r.calls, 1, name) {
			assert.Equal(t, CallRefused, r.calls[0].result, name)
		}
	}
}

func TestAdapterOtherError(t *testing.T) {
	r := newRig(t, time.Hour)
	r.prov.resp, r.prov.err = nil, provider.ErrNotSupported
	_, err := r.ask(userA)
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	assert.NotErrorIs(t, err, provider.ErrProviderDown)
	assert.Equal(t, []string{"water/qabzino/error"}, r.inquiry)
	assert.Equal(t, CallError, r.calls[0].result)
	// A nil answer with no error.
	r = newRig(t, time.Hour)
	r.prov.resp, r.prov.err = nil, nil
	_, err = r.ask(userA)
	assert.Error(t, err)
	assert.Equal(t, []string{"water/qabzino/error"}, r.inquiry)
}

func TestClientLeavesDuringProviderCall(t *testing.T) {
	r := newRig(t, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	r.prov.resp, r.prov.err = nil, errors.Join(provider.ErrProviderDown, context.Canceled)
	r.prov.run = func(context.Context) { cancel() }
	_, err := r.svc.Inquire(ctx, Request{BillType: "water", InquiryID: validBill, UserID: userA})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, []string{"water/qabzino/canceled"}, r.inquiry)
	if assert.Len(t, r.repo.requests, 1) {
		assert.Equal(t, [2]string{"failed", ""}, r.repo.finished[r.repo.requests[0].ID],
			"the row is closed on a context that the client cannot cancel")
	}
	assert.Equal(t, CallCanceled, r.calls[0].result)
	assert.Equal(t, 0, r.log.count("error"))
}

func TestDatabaseFailures(t *testing.T) {
	boom := errors.New("db down")

	// A failed lookup is logged and the provider is asked (as bill does).
	r := newRig(t, time.Hour)
	r.repo.findErr = boom
	res, err := r.ask(userA)
	assert.NoError(t, err)
	assert.False(t, res.FromDatabase)
	assert.Equal(t, 1, r.prov.calls)
	assert.Equal(t, 1, r.log.count("error"))

	// A failed copy write is logged; the answer is still the saved one.
	r = newRig(t, time.Hour)
	r.repo.seed(Row{ID: "x", InquiryID: validBill, ProviderID: 36, BillID: validBill, PayID: "p", Amount: 3}, time.Minute, false)
	r.repo.saveErr = boom
	res, err = r.ask(userA)
	assert.NoError(t, err)
	assert.True(t, res.FromDatabase)
	assert.Equal(t, 1, r.log.count("error"))

	// A failed request row stops before the provider call.
	r = newRig(t, time.Hour)
	r.repo.createErr = boom
	_, err = r.ask(userA)
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, 0, r.prov.calls)
	assert.Equal(t, []string{"water/qabzino/error"}, r.inquiry)

	// A failed status write after a good answer is an error (as in bill).
	r = newRig(t, time.Hour)
	r.repo.finishErr = boom
	_, err = r.ask(userA)
	assert.ErrorIs(t, err, boom)
	assert.Equal(t, []string{"water/qabzino/error"}, r.inquiry)
	assert.Equal(t, CallError, r.calls[0].result)

	// A failed status write after a failed call is logged; the answer is the call error.
	r = newRig(t, time.Hour)
	r.prov.resp, r.prov.err = nil, provider.ErrProviderDown
	r.repo.finishErr = boom
	_, err = r.ask(userA)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Equal(t, 1, r.log.count("error"))

	// A failed save of the inquiries is logged; the answer is the provider answer (as in bill).
	r = newRig(t, time.Hour)
	r.repo.saveErr = boom
	res, err = r.ask(userA)
	assert.NoError(t, err)
	assert.Len(t, res.Inquiries, 1)
	assert.Equal(t, 1, r.log.count("error"))
	assert.Equal(t, []string{"water/qabzino/provider_ok"}, r.inquiry)
}

func TestTIDFailureAndNilHooks(t *testing.T) {
	r := newRig(t, time.Hour)
	r.svc.newTID = func(string) (string, error) { return "", errors.New("no tid") }
	_, err := r.ask(userA)
	assert.ErrorContains(t, err, "no tid")
	assert.Equal(t, []string{"water/qabzino/error"}, r.inquiry)

	// No hooks, no logger: nothing panics.
	r = newRig(t, time.Hour)
	svc, err := NewService(Options{Repository: r.repo, Providers: r.selector, Adapters: fakeAdapters{"qabzino": r.prov}, CacheTTL: func(string) time.Duration { return time.Hour }})
	if !assert.NoError(t, err) {
		return
	}
	var nilCtx context.Context
	_, err = svc.Inquire(nilCtx, Request{BillType: "water", InquiryID: validBill, UserID: userA})
	assert.NoError(t, err)
	_, err = svc.Inquire(context.Background(), Request{BillType: "water", InquiryID: validBill, UserID: userA})
	assert.NoError(t, err, "second call from the database")
}

func TestPaidBillAnswer(t *testing.T) {
	r := newRig(t, time.Hour)
	r.prov.resp = &provider.InquiryResponse{Status: "success", ProviderTraceID: "t", Inquiries: []provider.InquiryItem{{InquiryType: "regular"}}}
	res, err := r.ask(userA)
	if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 1) {
		assert.Equal(t, uint64(0), res.Inquiries[0].Amount)
		assert.Equal(t, "", res.Inquiries[0].PayID)
		assert.Equal(t, "", res.Inquiries[0].PaymentDeadline)
		assert.Equal(t, "", res.BillID)
	}
	assert.Len(t, r.repo.rows, 1, "the no-debt row is saved too, as in bill")
}

func TestGenerateTID(t *testing.T) {
	seen := map[string]bool{}
	for range 50 {
		tid, err := generateTID("water")
		if !assert.NoError(t, err) {
			return
		}
		assert.Regexp(t, `^1[0-9]{3,14}$`, tid)
		assert.LessOrEqual(t, len(tid), 30, "bill_inquiry_requests.tid is VARCHAR(30)")
		seen[tid] = true
	}
	assert.Greater(t, len(seen), 40, "random")
	_, err := generateTID("freewayToll")
	assert.Error(t, err)
}

func TestFixPaymentDeadline(t *testing.T) {
	for in, want := range map[string]string{
		"":                          "",
		"2030-01-02":                "2030-01-02",
		"2030/01/02":                "2030-01-02",
		"25-12-2030":                "2030-12-25",
		"25/12/2030":                "2030-12-25",
		"25.12.2030":                "2030-12-25",
		"2030-01-02 10:30":          "2030-01-02",
		"2030-01-02 10:30:15":       "2030-01-02",
		"2030-01-02T10:30:15+03:30": "2030-01-02",
		"1405/09/15":                "1405-09-15",
		"garbage":                   "",
		"2030-13-45":                "",
	} {
		assert.Equal(t, want, fixPaymentDeadline(in), in)
	}
}

func TestRefusedErrorText(t *testing.T) {
	assert.Contains(t, (&RefusedError{Message: "x"}).Error(), "x")
}

// safeProvider is a provider that is safe for concurrent calls.
type safeProvider struct {
	provider.ProviderInterface
	mu    sync.Mutex
	calls int
}

func (*safeProvider) Name() string                { return "qabzino" }
func (*safeProvider) SupportsInquiry(string) bool { return true }
func (s *safeProvider) Inquiry(context.Context, provider.InquiryRequest) (*provider.InquiryResponse, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return successResp(), nil
}

// Concurrent calls need no lock in the flow (the repository and the adapters are safe).
func TestConcurrentInquire(t *testing.T) {
	repo := newMemRepo()
	prov := &safeProvider{}
	svc, err := NewService(Options{Repository: repo, Providers: fakeSelector{p: qabzino}, Adapters: fakeAdapters{"qabzino": prov},
		CacheTTL: func(string) time.Duration { return time.Hour }})
	if !assert.NoError(t, err) {
		return
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Inquire(context.Background(), Request{BillType: "water", InquiryID: validBill, UserID: userA})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	assert.GreaterOrEqual(t, prov.calls, 1)
}
