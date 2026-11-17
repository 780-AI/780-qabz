package inquiry

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"

	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/provider"
)

// Task 059: the inquiry flow and the Provider Switcher, with a real provider.ProvidersService.

// rowsRepo is a provider.Repository with fixed rows.
type rowsRepo struct{ rows []provider.Provider }

func (r rowsRepo) ListProviders(context.Context) ([]provider.Provider, error) { return r.rows, nil }

// switchLog is a provider.Logger that records the info messages. Safe for concurrent use.
type switchLog struct {
	mu   sync.Mutex
	info []string
}

func (l *switchLog) Debug(string, ...any) {}
func (l *switchLog) Warn(string, ...any)  {}
func (l *switchLog) Error(string, ...any) {}
func (l *switchLog) Info(msg string, _ ...any) {
	l.mu.Lock()
	l.info = append(l.info, msg)
	l.mu.Unlock()
}

// switchRig is a flow on a ProvidersService with two water rows: 1 qabzino first, 2 tavanir next.
type switchRig struct {
	svc     *Service
	list    *provider.ProvidersService
	repo    *memRepo
	qabzino *stubProvider
	tavanir *stubProvider
	inquiry []string
	results []string // id/down or id/answer of OnProviderResult
	trials  []string
}

func newSwitchRig(t *testing.T) *switchRig {
	t.Helper()
	r := &switchRig{repo: newMemRepo()}
	list, err := provider.NewProvidersService(rowsRepo{rows: []provider.Provider{
		{ID: 1, Name: "qabzino", BillType: "water", Action: "inquiry", Priority: 1, Status: provider.StatusEnable},
		{ID: 2, Name: "tavanir", BillType: "water", Action: "inquiry", Priority: 2, Status: provider.StatusEnable},
	}}, provider.Options{
		Interval: func() time.Duration { return time.Hour }, Logger: &switchLog{},
		OnTrial: func(b, a, res string) { r.trials = append(r.trials, b+"/"+a+"/"+res) },
	})
	if !assert.NoError(t, err) || !assert.NoError(t, list.Load(context.Background())) {
		t.FailNow()
	}
	r.list = list
	r.qabzino = &stubProvider{name: "qabzino", resp: successResp()}
	r.tavanir = &stubProvider{name: "tavanir", resp: successResp()}
	svc, err := NewService(Options{
		Repository: r.repo, Providers: list, Adapters: fakeAdapters{"qabzino": r.qabzino, "tavanir": r.tavanir},
		CacheTTL:  func(string) time.Duration { return time.Hour },
		OnInquiry: func(b, p, res string) { r.inquiry = append(r.inquiry, b+"/"+p+"/"+res) },
		OnProviderResult: func(p provider.Provider, down bool) {
			res := "answer"
			if down {
				res = "down"
			}
			r.results = append(r.results, fmt.Sprintf("%d/%s", p.ID, res))
		},
	})
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	svc.newTID = func(string) (string, error) { return "tid-1", nil }
	r.svc = svc
	return r
}

func (r *switchRig) ask() (Result, error) {
	return r.svc.Inquire(context.Background(), Request{BillType: "water", InquiryID: validBill, UserID: userA})
}

// demoteDue demotes row 1 for 1 ms and waits until the period is over.
func (r *switchRig) demoteDue(t *testing.T) {
	t.Helper()
	assert.NoError(t, r.list.Demote(1, time.Millisecond))
	time.Sleep(5 * time.Millisecond)
}

// TestSwitchDemotedRowNotPicked: after a demotion the next row answers, one call; before the
// period ends no request goes to the demoted row.
func TestSwitchDemotedRowNotPicked(t *testing.T) {
	r := newSwitchRig(t)
	assert.NoError(t, r.list.Demote(1, time.Hour))
	_, err := r.ask()
	assert.NoError(t, err)
	assert.Equal(t, 0, r.qabzino.calls)
	assert.Equal(t, 1, r.tavanir.calls)
	assert.Equal(t, []string{"water/tavanir/provider_ok"}, r.inquiry)
}

// TestSwitchTrialResults: the trial calls the demoted provider (also when a valid saved answer of
// it exists: the trial reads no saved answer); a good and a refused answer put the row back; a
// provider-down answer demotes it again; a fault with no verdict releases the trial. The flow never
// calls a second provider; OnProviderResult is not called for a trial (task 060).
func TestSwitchTrialResults(t *testing.T) {
	for _, tc := range []struct {
		name      string
		set       func(r *switchRig)
		wantTrial string
		wantPick  int64 // the pick of the next request
	}{
		{"ok", func(r *switchRig) {}, "water/inquiry/ok", 1},
		{"refused", func(r *switchRig) {
			r.qabzino.resp = &provider.InquiryResponse{Status: provider.ResponseFailed, Message: "no bill"}
		}, "water/inquiry/ok", 1},
		{"down", func(r *switchRig) { r.qabzino.err = provider.ErrProviderDown }, "water/inquiry/failed", 2},
		{"no verdict", func(r *switchRig) { r.qabzino.err = errors.New("bad answer") }, "water/inquiry/released", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newSwitchRig(t)
			r.repo.seed(Row{ID: "old", InquiryID: validBill, ProviderID: 1, ProviderName: "qabzino", BillType: "water", PayID: "1", Amount: 9}, time.Minute, false)
			tc.set(r)
			r.demoteDue(t)
			_, _ = r.ask()
			assert.Equal(t, 1, r.qabzino.calls, "the trial calls the demoted provider")
			assert.Equal(t, 0, r.tavanir.calls, "one request, one provider")
			assert.Equal(t, 0, r.repo.findCalls, "a trial reads no saved answer")
			assert.Equal(t, []string{tc.wantTrial}, r.trials)
			assert.Empty(t, r.results, "no OnProviderResult for a trial")
			p, err := r.list.PickProvider("water", "inquiry")
			assert.NoError(t, err)
			assert.Equal(t, tc.wantPick, p.ID)
		})
	}
}

// TestSwitchOnProviderResult (task 060): a provider-down result that is not a trial calls
// OnProviderResult with the row and down true; a good and a refused answer with down false; an
// error that is not provider down, a canceled call and a saved answer do not call it.
func TestSwitchOnProviderResult(t *testing.T) {
	r := newSwitchRig(t)
	ask := func(ctx context.Context, noCache bool) error {
		_, err := r.svc.Inquire(ctx, Request{BillType: "water", InquiryID: validBill, UserID: userA, NoCache: noCache})
		return err
	}
	assert.NoError(t, ask(context.Background(), true))
	assert.Equal(t, []string{"1/answer"}, r.results, "ok")
	assert.NoError(t, ask(context.Background(), false))
	assert.Equal(t, []string{"1/answer"}, r.results, "a saved answer calls no provider")
	r.qabzino.resp = &provider.InquiryResponse{Status: provider.ResponseFailed, Message: "no bill"}
	assert.Error(t, ask(context.Background(), true))
	assert.Equal(t, []string{"1/answer", "1/answer"}, r.results, "a business answer is a good call")
	r.qabzino.err = provider.ErrProviderDown
	assert.ErrorIs(t, ask(context.Background(), true), provider.ErrProviderDown)
	assert.Equal(t, []string{"1/answer", "1/answer", "1/down"}, r.results)
	r.qabzino.err = errors.New("bad answer")
	assert.Error(t, ask(context.Background(), true))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.qabzino.err = provider.ErrProviderDown
	assert.Error(t, ask(ctx, true))
	assert.Equal(t, []string{"1/answer", "1/answer", "1/down"}, r.results, "an error that is not provider down, a cancel")
}

// TestSwitchTrialSpan: the span of a trial inquiry has provider.trial true; a plain inquiry has no
// such attribute.
func TestSwitchTrialSpan(t *testing.T) {
	sr := jaegertest.Install(t)
	r := newSwitchRig(t)
	_, _ = r.ask()
	r.demoteDue(t)
	_, _ = r.ask()
	spans := jaegertest.ByName(sr, inquireSpan)
	if assert.Len(t, spans, 2) {
		assert.NotContains(t, spans[0].Attributes(), attribute.Bool("provider.trial", true))
		assert.Contains(t, spans[1].Attributes(), attribute.Bool("provider.trial", true))
	}
	jaegertest.AssertClean(t, sr.Ended())
}
