package inquiry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// Task 035: the same flow answers the Irancell mobile bill. The inquiry ID is the phone; one
// answer has a mid-term and a final-term inquiry; only the Irancell provider may answer.

const (
	irPhone  = "989351234567"
	irPhone2 = "989359876543"
)

var irancellRow = provider.Provider{ID: 38, Name: "irancell", BillType: "irancell", Action: "inquiry", Status: provider.StatusEnable}

func irancellResp() *provider.InquiryResponse {
	return &provider.InquiryResponse{
		Status: provider.ResponseSuccess, ProviderTraceID: "trace-ir",
		Inquiries: []provider.InquiryItem{
			{Amount: 150000, InquiryType: provider.InquiryTypeMidterm},
			{Amount: 450000, InquiryType: provider.InquiryTypeFinalTerm},
		},
	}
}

type irancellRig struct {
	svc      *Service
	repo     *memRepo
	irancell *stubProvider
	qabzino  *stubProvider
	selector *typeSelector
	log      *recLog
	inquiry  []string
	calls    []call
	tidAsked []string
}

// newIrancellRig is one Service with the Irancell provider (id 38) for irancell and Qabzino (id 36)
// for water. The two adapters are registered.
func newIrancellRig(t *testing.T, ttl time.Duration) *irancellRig {
	t.Helper()
	r := &irancellRig{repo: newMemRepo(), log: &recLog{}, selector: &typeSelector{byType: map[string]provider.Provider{"irancell": irancellRow, "water": qabzino}}}
	r.irancell = &stubProvider{name: "irancell", resp: irancellResp()}
	r.qabzino = &stubProvider{name: "qabzino", resp: successResp()}
	svc, err := NewService(Options{
		Repository: r.repo, Providers: r.selector, Adapters: fakeAdapters{"irancell": r.irancell, "qabzino": r.qabzino},
		CacheTTL: func(string) time.Duration { return ttl }, Logger: r.log,
		OnInquiry:      func(b, p, res string) { r.inquiry = append(r.inquiry, b+"/"+p+"/"+res) },
		OnProviderCall: func(p, a, res string, d time.Duration) { r.calls = append(r.calls, call{p, a, res, d}) },
	})
	if !assert.NoError(t, err) || !assert.NotNil(t, svc) {
		t.FailNow()
	}
	n := 0
	svc.newID = func() string { n++; return "id-" + string(rune('a'+n)) }
	svc.newTID = func(billType string) (string, error) {
		r.tidAsked = append(r.tidAsked, billType)
		return "tid-" + billType, nil
	}
	r.svc = svc
	return r
}

func (r *irancellRig) ask(phone, user string) (Result, error) {
	return r.svc.Inquire(context.Background(), Request{BillType: provider.BillTypeIrancell, InquiryID: phone, UserID: user, Dialer: "989121234567"})
}

// TestIrancellMissSavesTwoInquiries: the provider gets the phone; the mid-term and the final-term
// inquiry are two rows; the request row has the bill type and the final status.
func TestIrancellMissSavesTwoInquiries(t *testing.T) {
	r := newIrancellRig(t, time.Hour)
	res, err := r.ask(irPhone, userA)
	if !assert.NoError(t, err) || !assert.Len(t, res.Inquiries, 2) {
		return
	}
	assert.False(t, res.FromDatabase)
	assert.Equal(t, "", res.BillID, "Irancell gives no bill ID")
	assert.Nil(t, res.Details)
	assert.Equal(t, []string{provider.InquiryTypeMidterm, provider.InquiryTypeFinalTerm}, []string{res.Inquiries[0].InquiryType, res.Inquiries[1].InquiryType})
	assert.Equal(t, []uint64{150000, 450000}, []uint64{res.Inquiries[0].Amount, res.Inquiries[1].Amount})
	assert.Equal(t, []string{"", ""}, []string{res.Inquiries[0].PayID, res.Inquiries[1].PayID})

	if assert.Len(t, r.irancell.reqs, 1) {
		assert.Equal(t, provider.InquiryRequest{BillType: "irancell", UserID: userA, Dialer: "989121234567", InquiryID: irPhone}, r.irancell.reqs[0])
	}
	assert.Equal(t, 0, r.qabzino.calls, "no other provider is called")
	assert.Equal(t, []string{"irancell/inquiry"}, r.selector.picks)
	assert.Equal(t, []string{"irancell"}, r.tidAsked)

	if assert.Len(t, r.repo.requests, 1) {
		q := r.repo.requests[0]
		assert.Equal(t, []string{userA, irPhone, "irancell", "tid-irancell", "irancell", RequestInitial},
			[]string{q.UserID, q.InquiryID, q.BillType, q.TID, q.ProviderName, q.Status})
		assert.Equal(t, [2]string{provider.ResponseSuccess, "trace-ir"}, r.repo.finished[q.ID])
	}
	if assert.Len(t, r.repo.rows, 2) {
		for i, want := range []struct {
			typ    string
			amount uint64
		}{{provider.InquiryTypeMidterm, 150000}, {provider.InquiryTypeFinalTerm, 450000}} {
			row := r.repo.rows[i].Row
			assert.Equal(t, []string{userA, irPhone, "irancell", "irancell", "trace-ir", want.typ, "", "", ""},
				[]string{row.UserID, row.InquiryID, row.ProviderName, row.BillType, row.ProviderTraceID, row.InquiryType, row.BillID, row.PayID, row.PaymentDeadline})
			assert.Equal(t, want.amount, row.Amount)
			assert.EqualValues(t, 38, row.ProviderID)
			assert.False(t, row.IsCached)
			assert.Equal(t, res.Inquiries[i].ID, row.ID)
		}
	}
	assert.Equal(t, []string{"irancell/irancell/provider_ok"}, r.inquiry)
	if assert.Len(t, r.calls, 1) {
		assert.Equal(t, []string{"irancell", "inquiry", CallOK}, []string{r.calls[0].provider, r.calls[0].action, r.calls[0].result})
	}
}

// TestIrancellDatabaseAnswersBoth: with a cache TTL, the second call is answered from the two saved
// rows with no provider call; another phone is another key.
func TestIrancellDatabaseAnswersBoth(t *testing.T) {
	r := newIrancellRig(t, time.Hour)
	first, err := r.ask(irPhone, userA)
	if !assert.NoError(t, err) || !assert.Len(t, first.Inquiries, 2) {
		return
	}
	second, err := r.ask(irPhone, userB)
	if !assert.NoError(t, err) || !assert.Len(t, second.Inquiries, 2) {
		return
	}
	assert.True(t, second.FromDatabase)
	assert.Equal(t, 1, r.irancell.calls, "no second provider call")
	assert.Len(t, r.repo.requests, 1, "no second request row")
	got := map[string]uint64{}
	for _, in := range second.Inquiries {
		got[in.InquiryType] = in.Amount
	}
	assert.Equal(t, map[string]uint64{provider.InquiryTypeMidterm: 150000, provider.InquiryTypeFinalTerm: 450000}, got)
	assert.Len(t, r.repo.rows, 4, "two copies for the second user")
	for _, row := range r.repo.rows[2:] {
		assert.True(t, row.IsCached)
		assert.Equal(t, userB, row.UserID)
	}

	_, err = r.ask(irPhone2, userA)
	assert.NoError(t, err)
	assert.Equal(t, 2, r.irancell.calls, "another phone asks the provider")
	assert.Equal(t, []string{"irancell/irancell/provider_ok", "irancell/irancell/db_hit", "irancell/irancell/provider_ok"}, r.inquiry)
}

// TestIrancellCacheOffAsksEveryTime: the default of caching.irancell is 0 (bill has no cache for the
// bill type): every call asks the provider.
func TestIrancellCacheOffAsksEveryTime(t *testing.T) {
	r := newIrancellRig(t, 0)
	for range 3 {
		res, err := r.ask(irPhone, userA)
		assert.NoError(t, err)
		assert.False(t, res.FromDatabase)
	}
	assert.Equal(t, 3, r.irancell.calls)
	assert.Equal(t, 0, r.repo.findCalls, "the saved inquiries are not read")
	assert.Len(t, r.repo.requests, 3)
}

// TestIrancellNoDebt: a success answer of two inquiries of amount 0 is a normal answer.
func TestIrancellNoDebt(t *testing.T) {
	r := newIrancellRig(t, time.Hour)
	r.irancell.resp = &provider.InquiryResponse{Status: provider.ResponseSuccess, Inquiries: []provider.InquiryItem{
		{InquiryType: provider.InquiryTypeMidterm}, {InquiryType: provider.InquiryTypeFinalTerm},
	}}
	res, err := r.ask(irPhone, userA)
	if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 2) {
		assert.Equal(t, []uint64{0, 0}, []uint64{res.Inquiries[0].Amount, res.Inquiries[1].Amount})
	}
	assert.Len(t, r.repo.rows, 2)
}

// TestIrancellInvalidInquiryID: the inquiry ID must be a mobile number in the 98 form.
func TestIrancellInvalidInquiryID(t *testing.T) {
	for _, id := range []string{"", "09351234567", "9351234567", "98935123456", "9893512345678", "989351234 67", validBill, "982112345678"} {
		r := newIrancellRig(t, time.Hour)
		_, err := r.ask(id, userA)
		assert.ErrorIs(t, err, ErrInvalid, id)
		assert.Equal(t, 0, r.irancell.calls, id)
		assert.Empty(t, r.selector.picks, id+": no pick for an invalid request")
		assert.Empty(t, r.repo.requests, id)
		assert.Empty(t, r.inquiry, id+": an invalid request is not counted")
	}
	r := newIrancellRig(t, time.Hour)
	_, err := r.ask(irPhone, "not-a-uuid")
	assert.ErrorIs(t, err, ErrInvalid)
}

// TestIrancellOnlyTheIrancellProvider (operator rule): when the pick for the bill type irancell is
// another provider, nobody is called, also when that provider has an adapter. The answer is the one
// of "no enabled provider".
func TestIrancellOnlyTheIrancellProvider(t *testing.T) {
	for _, other := range []provider.Provider{
		{ID: 31, Name: "qabzino", BillType: "irancell", Action: "inquiry", Status: provider.StatusEnable},
		{ID: 39, Name: "parsian", BillType: "irancell", Action: "inquiry", Status: provider.StatusEnable},
		{ID: 28, Name: "jiring-switch", BillType: "irancell", Action: "inquiry", Status: provider.StatusEnable},
		{ID: 77, Name: "Irancell", BillType: "irancell", Action: "inquiry", Status: provider.StatusEnable}, // the name is exact
	} {
		r := newIrancellRig(t, time.Hour)
		r.selector.byType["irancell"] = other
		// A saved row of that provider must not answer either.
		r.repo.seed(Row{ID: "old", InquiryID: irPhone, ProviderID: other.ID, ProviderName: other.Name, BillType: "irancell", InquiryType: provider.InquiryTypeMidterm, Amount: 9}, time.Minute, false)
		res, err := r.ask(irPhone, userA)
		assert.Empty(t, res.Inquiries, other.Name)
		if assert.ErrorIs(t, err, provider.ErrNoProvider, other.Name) {
			assert.Contains(t, err.Error(), `only "irancell" may answer`, other.Name)
			assert.Contains(t, err.Error(), other.Name)
		}
		assert.Equal(t, 0, r.irancell.calls, other.Name)
		assert.Equal(t, 0, r.qabzino.calls, other.Name+": the other provider is not called")
		assert.Equal(t, 0, r.repo.findCalls, other.Name+": its saved rows are not read")
		assert.Empty(t, r.repo.requests, other.Name+": no request row")
		assert.Equal(t, []string{"irancell/none/no_provider"}, r.inquiry, other.Name)
		assert.Empty(t, r.calls, other.Name)
	}
	// The other bill types are free: Qabzino still answers water in the same Service.
	r := newIrancellRig(t, time.Hour)
	_, err := r.svc.Inquire(context.Background(), Request{BillType: "water", InquiryID: validBill, UserID: userA})
	assert.NoError(t, err)
	assert.Equal(t, 1, r.qabzino.calls)
	assert.Equal(t, 0, r.irancell.calls)
}

// TestIrancellNotAvailable: no enabled provider, a provider that is down and a missing adapter. No
// other provider is called in any of them.
func TestIrancellNotAvailable(t *testing.T) {
	// The Irancell row is disabled: the selector has no provider for the bill type.
	r := newIrancellRig(t, time.Hour)
	delete(r.selector.byType, "irancell")
	_, err := r.ask(irPhone, userA)
	assert.ErrorIs(t, err, provider.ErrNoProvider)
	assert.Equal(t, 0, r.irancell.calls+r.qabzino.calls)
	assert.Equal(t, []string{"irancell/none/no_provider"}, r.inquiry)

	// The call fails: the request row is failed, no inquiry is saved.
	r = newIrancellRig(t, time.Hour)
	r.irancell.resp, r.irancell.err = nil, errors.Join(provider.ErrProviderDown, errors.New("connection refused"))
	_, err = r.ask(irPhone, userA)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Equal(t, 1, r.irancell.calls)
	assert.Equal(t, 0, r.qabzino.calls, "no fallback")
	if assert.Len(t, r.repo.requests, 1) {
		assert.Equal(t, [2]string{provider.ResponseFailed, ""}, r.repo.finished[r.repo.requests[0].ID])
	}
	assert.Empty(t, r.repo.rows)
	assert.Equal(t, []string{"irancell/irancell/provider_down"}, r.inquiry)

	// The provider refuses (for example not an Irancell number): RefusedError with its message.
	r = newIrancellRig(t, time.Hour)
	r.irancell.resp = &provider.InquiryResponse{Status: "failed", Code: 400, Message: "not found", ProviderTraceID: "t"}
	_, err = r.ask(irPhone, userA)
	var refused *RefusedError
	if assert.ErrorAs(t, err, &refused) && assert.NotNil(t, refused) {
		assert.Equal(t, "not found", refused.Message)
	}
	assert.Empty(t, r.repo.rows)
	assert.Equal(t, []string{"irancell/irancell/provider_refused"}, r.inquiry)

	// The adapter is not registered.
	r = newIrancellRig(t, time.Hour)
	svc, err := NewService(Options{Repository: r.repo, Providers: r.selector, Adapters: fakeAdapters{"qabzino": r.qabzino}})
	if assert.NoError(t, err) && assert.NotNil(t, svc) {
		_, err = svc.Inquire(context.Background(), Request{BillType: "irancell", InquiryID: irPhone, UserID: userA})
		assert.ErrorIs(t, err, ErrNoAdapter)
		assert.Equal(t, 0, r.qabzino.calls)
	}
}

// TestIrancellSavedRowsWhileProviderDown: valid saved rows answer while the provider is down.
func TestIrancellSavedRowsWhileProviderDown(t *testing.T) {
	r := newIrancellRig(t, time.Hour)
	_, err := r.ask(irPhone, userA)
	if !assert.NoError(t, err) {
		return
	}
	r.irancell.resp, r.irancell.err = nil, provider.ErrProviderDown
	res, err := r.ask(irPhone, userA)
	if assert.NoError(t, err) {
		assert.True(t, res.FromDatabase)
		assert.Len(t, res.Inquiries, 2)
	}
	assert.Equal(t, 1, r.irancell.calls)
}

// TestIrancellTID: the tid of a request row starts with 8 (bill: IrancellReference).
func TestIrancellTID(t *testing.T) {
	tid, err := generateTID(provider.BillTypeIrancell)
	if assert.NoError(t, err) && assert.NotEmpty(t, tid) {
		assert.Equal(t, byte('8'), tid[0])
	}
}
