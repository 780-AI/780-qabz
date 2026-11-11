package inquiry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// Task 037: the same flow answers the Rightel mobile bill (the bill type mobilemidtermRightel). The
// inquiry ID is the phone; one answer has a mid-term and a final-term inquiry; only the Qabzino
// provider may answer (operator rule).

const (
	rtPhone  = "989201234567"
	rtPhone2 = "989209876543"
	rtType   = provider.BillTypeMobileMidTermRightel
)

// rightelRow is the seed row 34 (qabzino-rightel-inquiry).
var rightelRow = provider.Provider{ID: 34, Name: "qabzino", BillType: rtType, Action: "inquiry", Status: provider.StatusEnable}

func rightelResp() *provider.InquiryResponse {
	return &provider.InquiryResponse{
		Status: provider.ResponseSuccess, ProviderTraceID: "trace-rt", BillID: "9200000034",
		Inquiries: []provider.InquiryItem{
			{PayID: "23000341", Amount: 230000, InquiryType: provider.InquiryTypeMidterm},
			{PayID: "63000342", Amount: 630000, InquiryType: provider.InquiryTypeFinalTerm},
		},
	}
}

type rightelRig struct {
	svc      *Service
	repo     *memRepo
	qabzino  *stubProvider
	others   map[string]*stubProvider
	selector *typeSelector
	log      *recLog
	inquiry  []string
	calls    []call
	tidAsked []string
}

// newRightelRig is one Service with Qabzino (id 34) for mobilemidtermRightel. The adapters of the
// other providers of the seed are registered too, so a test can show that they are never called.
func newRightelRig(t *testing.T, ttl time.Duration) *rightelRig {
	t.Helper()
	r := &rightelRig{repo: newMemRepo(), log: &recLog{}, selector: &typeSelector{byType: map[string]provider.Provider{rtType: rightelRow}}}
	r.qabzino = &stubProvider{name: "qabzino", resp: rightelResp()}
	r.others = map[string]*stubProvider{}
	adapters := fakeAdapters{"qabzino": r.qabzino}
	for _, name := range []string{"parsian", "jiring-switch", "irancell", "tavanir"} {
		s := &stubProvider{name: name, resp: rightelResp()}
		r.others[name] = s
		adapters[name] = s
	}
	svc, err := NewService(Options{
		Repository: r.repo, Providers: r.selector, Adapters: adapters,
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

func (r *rightelRig) ask(phone, user string) (Result, error) {
	return r.svc.Inquire(context.Background(), Request{BillType: rtType, InquiryID: phone, UserID: user, Dialer: "989121234567"})
}

func (r *rightelRig) otherCalls() int {
	n := 0
	for _, s := range r.others {
		n += s.calls
	}
	return n
}

// TestRightelMissSavesTwoInquiries: no saved row: one Qabzino call with the phone; the mid-term and
// the final-term inquiry are two rows with the bill ID; one request row with the final status.
func TestRightelMissSavesTwoInquiries(t *testing.T) {
	r := newRightelRig(t, time.Hour)
	res, err := r.ask(rtPhone, userA)
	if !assert.NoError(t, err) || !assert.Len(t, res.Inquiries, 2) {
		return
	}
	assert.False(t, res.FromDatabase)
	assert.Equal(t, "9200000034", res.BillID)
	assert.Equal(t, []string{provider.InquiryTypeMidterm, provider.InquiryTypeFinalTerm}, []string{res.Inquiries[0].InquiryType, res.Inquiries[1].InquiryType})
	assert.Equal(t, []uint64{230000, 630000}, []uint64{res.Inquiries[0].Amount, res.Inquiries[1].Amount})
	assert.Equal(t, []string{"23000341", "63000342"}, []string{res.Inquiries[0].PayID, res.Inquiries[1].PayID})

	if assert.Len(t, r.qabzino.reqs, 1) {
		assert.Equal(t, provider.InquiryRequest{BillType: rtType, UserID: userA, Dialer: "989121234567", InquiryID: rtPhone}, r.qabzino.reqs[0])
	}
	assert.Equal(t, 0, r.otherCalls(), "no other provider is called")
	assert.Equal(t, []string{rtType + "/inquiry"}, r.selector.picks)
	assert.Equal(t, []string{rtType}, r.tidAsked)

	if assert.Len(t, r.repo.requests, 1) {
		q := r.repo.requests[0]
		assert.Equal(t, []string{userA, rtPhone, rtType, "tid-" + rtType, "qabzino", RequestInitial},
			[]string{q.UserID, q.InquiryID, q.BillType, q.TID, q.ProviderName, q.Status})
		assert.Equal(t, [2]string{provider.ResponseSuccess, "trace-rt"}, r.repo.finished[q.ID])
	}
	if assert.Len(t, r.repo.rows, 2) {
		for i, want := range []struct {
			typ, payID string
			amount     uint64
		}{{provider.InquiryTypeMidterm, "23000341", 230000}, {provider.InquiryTypeFinalTerm, "63000342", 630000}} {
			row := r.repo.rows[i].Row
			assert.Equal(t, []string{userA, rtPhone, "qabzino", rtType, "trace-rt", want.typ, "9200000034", want.payID, ""},
				[]string{row.UserID, row.InquiryID, row.ProviderName, row.BillType, row.ProviderTraceID, row.InquiryType, row.BillID, row.PayID, row.PaymentDeadline})
			assert.Equal(t, want.amount, row.Amount)
			assert.EqualValues(t, 34, row.ProviderID)
			assert.False(t, row.IsCached)
		}
	}
	assert.Equal(t, []string{rtType + "/qabzino/provider_ok"}, r.inquiry)
	if assert.Len(t, r.calls, 1) {
		assert.Equal(t, []string{"qabzino", "inquiry", CallOK}, []string{r.calls[0].provider, r.calls[0].action, r.calls[0].result})
	}
}

// TestRightelDatabaseAnswers: a valid saved pair answers with no provider call and no request row;
// another phone is another key.
func TestRightelDatabaseAnswers(t *testing.T) {
	r := newRightelRig(t, time.Hour)
	if _, err := r.ask(rtPhone, userA); !assert.NoError(t, err) {
		return
	}
	res, err := r.ask(rtPhone, userB)
	if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 2) {
		assert.True(t, res.FromDatabase)
		assert.Equal(t, "9200000034", res.BillID)
		got := map[string]uint64{}
		for _, in := range res.Inquiries {
			got[in.InquiryType] = in.Amount
		}
		assert.Equal(t, map[string]uint64{provider.InquiryTypeMidterm: 230000, provider.InquiryTypeFinalTerm: 630000}, got)
	}
	assert.Equal(t, 1, r.qabzino.calls, "no second provider call")
	assert.Len(t, r.repo.requests, 1, "no second request row")

	_, err = r.ask(rtPhone2, userA)
	assert.NoError(t, err)
	assert.Equal(t, 2, r.qabzino.calls, "another phone asks the provider")
	assert.Equal(t, []string{rtType + "/qabzino/provider_ok", rtType + "/qabzino/db_hit", rtType + "/qabzino/provider_ok"}, r.inquiry)
}

// TestRightelSavedRowsThatDoNotAnswer: an expired row, a row of another provider and a paid row are
// a miss: Qabzino is asked.
func TestRightelSavedRowsThatDoNotAnswer(t *testing.T) {
	for name, seed := range map[string]struct {
		providerID int64
		age        time.Duration
		paid       bool
	}{
		"expired":          {34, 2 * time.Hour, false},
		"another provider": {32, time.Minute, false},
		"paid":             {34, time.Minute, true},
	} {
		r := newRightelRig(t, time.Hour)
		r.repo.seed(Row{ID: "old", InquiryID: rtPhone, ProviderID: seed.providerID, ProviderName: "qabzino", BillType: rtType,
			InquiryType: provider.InquiryTypeMidterm, Amount: 9}, seed.age, seed.paid)
		res, err := r.ask(rtPhone, userA)
		if assert.NoError(t, err, name) {
			assert.False(t, res.FromDatabase, name)
		}
		assert.Equal(t, 1, r.qabzino.calls, name)
	}
}

// TestRightelSameTypePairAnswers: bill has no cache skip for mobilemidtermRightel (shouldSkipCache:
// only mobile and tel), so two saved rows of the same inquiry type answer.
func TestRightelSameTypePairAnswers(t *testing.T) {
	r := newRightelRig(t, time.Hour)
	for _, id := range []string{"m1", "m2"} {
		r.repo.seed(Row{ID: id, InquiryID: rtPhone, ProviderID: 34, ProviderName: "qabzino", BillType: rtType,
			InquiryType: provider.InquiryTypeMidterm, Amount: 7}, time.Minute, false)
	}
	res, err := r.ask(rtPhone, userA)
	if assert.NoError(t, err) {
		assert.True(t, res.FromDatabase)
	}
	assert.Equal(t, 0, r.qabzino.calls)
}

// TestRightelOnlyQabzino (operator rule): when the pick for mobilemidtermRightel is another
// provider, nobody is called and its saved rows are not read, also when that provider has an
// adapter. The answer is the one of "no enabled provider".
func TestRightelOnlyQabzino(t *testing.T) {
	for _, other := range []provider.Provider{
		{ID: 39, Name: "parsian", BillType: rtType, Action: "inquiry", Status: provider.StatusEnable},
		{ID: 28, Name: "jiring-switch", BillType: rtType, Action: "inquiry", Status: provider.StatusEnable},
		{ID: 38, Name: "irancell", BillType: rtType, Action: "inquiry", Status: provider.StatusEnable},
		{ID: 77, Name: "Qabzino", BillType: rtType, Action: "inquiry", Status: provider.StatusEnable}, // the name is exact
	} {
		r := newRightelRig(t, time.Hour)
		r.selector.byType[rtType] = other
		r.repo.seed(Row{ID: "old", InquiryID: rtPhone, ProviderID: other.ID, ProviderName: other.Name, BillType: rtType, InquiryType: provider.InquiryTypeMidterm, Amount: 9}, time.Minute, false)
		res, err := r.ask(rtPhone, userA)
		assert.Empty(t, res.Inquiries, other.Name)
		if assert.ErrorIs(t, err, provider.ErrNoProvider, other.Name) {
			assert.Contains(t, err.Error(), `only "qabzino" may answer`, other.Name)
		}
		assert.Equal(t, 0, r.qabzino.calls+r.otherCalls(), other.Name+": nobody is called")
		assert.Equal(t, 0, r.repo.findCalls, other.Name+": its saved rows are not read")
		assert.Empty(t, r.repo.requests, other.Name+": no request row")
		assert.Equal(t, []string{rtType + "/none/no_provider"}, r.inquiry, other.Name)
	}
}

// TestRightelNotAvailable: Qabzino disabled (no pick), Qabzino down and a Qabzino refusal. No other
// provider is called in any of them.
func TestRightelNotAvailable(t *testing.T) {
	r := newRightelRig(t, time.Hour)
	delete(r.selector.byType, rtType)
	_, err := r.ask(rtPhone, userA)
	assert.ErrorIs(t, err, provider.ErrNoProvider)
	assert.Equal(t, 0, r.qabzino.calls+r.otherCalls())
	assert.Equal(t, []string{rtType + "/none/no_provider"}, r.inquiry)

	r = newRightelRig(t, time.Hour)
	r.qabzino.resp, r.qabzino.err = nil, errors.Join(provider.ErrProviderDown, errors.New("status code GD2266"))
	_, err = r.ask(rtPhone, userA)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Equal(t, 1, r.qabzino.calls)
	assert.Equal(t, 0, r.otherCalls(), "no fallback")
	if assert.Len(t, r.repo.requests, 1) {
		assert.Equal(t, [2]string{provider.ResponseFailed, ""}, r.repo.finished[r.repo.requests[0].ID])
	}
	assert.Empty(t, r.repo.rows)
	assert.Equal(t, []string{rtType + "/qabzino/provider_down"}, r.inquiry)

	r = newRightelRig(t, time.Hour)
	r.qabzino.resp = &provider.InquiryResponse{Status: provider.ResponseFailed, Code: 2267, Message: "not a Rightel line", ProviderTraceID: "t"}
	_, err = r.ask(rtPhone, userA)
	var refused *RefusedError
	if assert.ErrorAs(t, err, &refused) && assert.NotNil(t, refused) {
		assert.Equal(t, int32(2267), refused.Code)
	}
	assert.Empty(t, r.repo.rows)
	assert.Equal(t, 0, r.otherCalls())
	assert.Equal(t, []string{rtType + "/qabzino/provider_refused"}, r.inquiry)
}

// TestRightelInvalidInquiryID: the inquiry ID must be a mobile number in the 98 form.
func TestRightelInvalidInquiryID(t *testing.T) {
	for _, id := range []string{"", "09201234567", "9201234567", "98920123456", validBill} {
		r := newRightelRig(t, time.Hour)
		_, err := r.ask(id, userA)
		assert.ErrorIs(t, err, ErrInvalid, id)
		assert.Equal(t, 0, r.qabzino.calls, id)
		assert.Empty(t, r.selector.picks, id)
		assert.Empty(t, r.inquiry, id)
	}
}

// TestRightelTID: the tid of a request row starts with 6 (bill: RightelMobileReference).
func TestRightelTID(t *testing.T) {
	tid, err := generateTID(rtType)
	if assert.NoError(t, err) && assert.NotEmpty(t, tid) {
		assert.Equal(t, byte('6'), tid[0])
	}
}
