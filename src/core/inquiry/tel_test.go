package inquiry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// Task 025: the same flow answers the landline bill (the bill type tel). The inquiry ID is the
// landline number in the 98 form; one answer has a mid-term and a final-term inquiry, each can have
// its own bill ID (Karashop asks each part with its own call); only Karashop may answer (operator
// decision of 2026-11-14); two saved rows of the same inquiry type are a miss (bill: shouldSkipCache).

const (
	telNumber  = "982188776655"
	telNumber2 = "982133445566"
	telType    = provider.BillTypeTel
)

// karashopTelRow is the seed row 124325 (karashop-inquiry, tel).
var karashopTelRow = provider.Provider{ID: 124325, Name: "karashop", BillType: telType, Action: "inquiry", Status: provider.StatusEnable}

func telResp() *provider.InquiryResponse {
	return &provider.InquiryResponse{
		Status: provider.ResponseSuccess, ProviderTraceID: "tx-end,tx-mid", BillID: "2101234560041",
		Inquiries: []provider.InquiryItem{
			{BillID: "2101234560040", PayID: "12000035", Amount: 120000, InquiryType: provider.InquiryTypeMidterm},
			{BillID: "2101234560041", PayID: "35000042", Amount: 350000, InquiryType: provider.InquiryTypeFinalTerm},
		},
	}
}

type telRig struct {
	svc      *Service
	repo     *memRepo
	karashop *stubProvider
	others   map[string]*stubProvider
	selector *typeSelector
	inquiry  []string
	calls    []call
	tidAsked []string
}

// newTelRig is one Service with Karashop (id 124325) for tel. The adapters of the other providers of
// the seed that have tel rows (parsian, qabzino) are registered too, so a test can show that they are
// never called.
func newTelRig(t *testing.T, ttl time.Duration) *telRig {
	t.Helper()
	r := &telRig{repo: newMemRepo(), selector: &typeSelector{byType: map[string]provider.Provider{telType: karashopTelRow}}}
	r.karashop = &stubProvider{name: "karashop", resp: telResp()}
	r.others = map[string]*stubProvider{}
	adapters := fakeAdapters{"karashop": r.karashop}
	for _, name := range []string{"parsian", "qabzino", "eways2"} {
		s := &stubProvider{name: name, resp: telResp()}
		r.others[name] = s
		adapters[name] = s
	}
	svc, err := NewService(Options{
		Repository: r.repo, Providers: r.selector, Adapters: adapters,
		CacheTTL:       func(bt string) time.Duration { return map[string]time.Duration{telType: ttl}[bt] },
		Logger:         &recLog{},
		OnInquiry:      func(b, p, res string) { r.inquiry = append(r.inquiry, b+"/"+p+"/"+res) },
		OnProviderCall: func(p, a, res string, d time.Duration) { r.calls = append(r.calls, call{p, a, res, d}) },
	})
	if !assert.NoError(t, err) || !assert.NotNil(t, svc) {
		t.FailNow()
	}
	n := 0
	svc.newID = func() string { n++; return fmt.Sprintf("id-%d", n) }
	svc.newTID = func(billType string) (string, error) {
		r.tidAsked = append(r.tidAsked, billType)
		return "tid-" + billType, nil
	}
	r.svc = svc
	return r
}

func (r *telRig) ask(number, user string) (Result, error) {
	return r.svc.Inquire(context.Background(), Request{BillType: telType, InquiryID: number, UserID: user, Dialer: "989121234567"})
}

func (r *telRig) otherCalls() int {
	n := 0
	for _, s := range r.others {
		n += s.calls
	}
	return n
}

// TestTelMissSavesTwoInquiries: no saved row: one Karashop call (the adapter makes its two calls
// inside it) with the number; one request row; the mid-term and the final-term inquiry are two rows,
// each with the bill ID of its own inquiry.
func TestTelMissSavesTwoInquiries(t *testing.T) {
	r := newTelRig(t, time.Hour)
	res, err := r.ask(telNumber, userA)
	if !assert.NoError(t, err) || !assert.Len(t, res.Inquiries, 2) {
		return
	}
	assert.False(t, res.FromDatabase)
	assert.Equal(t, "2101234560041", res.BillID)
	assert.Equal(t, []string{"2101234560040", "2101234560041"}, []string{res.Inquiries[0].BillID, res.Inquiries[1].BillID}, "the bill ID of each inquiry")
	assert.Equal(t, []uint64{120000, 350000}, []uint64{res.Inquiries[0].Amount, res.Inquiries[1].Amount})

	if assert.Len(t, r.karashop.reqs, 1) {
		assert.Equal(t, provider.InquiryRequest{BillType: telType, UserID: userA, Dialer: "989121234567", InquiryID: telNumber}, r.karashop.reqs[0])
	}
	assert.Equal(t, 0, r.otherCalls(), "no other provider is called")
	assert.Equal(t, []string{telType + "/inquiry"}, r.selector.picks)
	assert.Equal(t, []string{telType}, r.tidAsked, "one request row, as task 025 decided (bill writes two)")
	if assert.Len(t, r.repo.requests, 1) {
		q := r.repo.requests[0]
		assert.Equal(t, []string{userA, telNumber, telType, "tid-tel", "karashop", RequestInitial},
			[]string{q.UserID, q.InquiryID, q.BillType, q.TID, q.ProviderName, q.Status})
		assert.Equal(t, [2]string{provider.ResponseSuccess, "tx-end,tx-mid"}, r.repo.finished[q.ID])
	}
	if assert.Len(t, r.repo.rows, 2) {
		for i, want := range [][3]string{{provider.InquiryTypeMidterm, "2101234560040", "12000035"}, {provider.InquiryTypeFinalTerm, "2101234560041", "35000042"}} {
			row := r.repo.rows[i].Row
			assert.Equal(t, []string{userA, telNumber, "karashop", telType, "tx-end,tx-mid", want[0], want[1], want[2]},
				[]string{row.UserID, row.InquiryID, row.ProviderName, row.BillType, row.ProviderTraceID, row.InquiryType, row.BillID, row.PayID})
			assert.EqualValues(t, 124325, row.ProviderID)
			assert.False(t, row.IsCached)
		}
	}
	assert.Equal(t, []string{"tel/karashop/provider_ok"}, r.inquiry)
	if assert.Len(t, r.calls, 1) {
		assert.Equal(t, []string{"karashop", "inquiry", CallOK}, []string{r.calls[0].provider, r.calls[0].action, r.calls[0].result})
	}
}

// TestItemBillIDFallsBackToAnswer: an inquiry with no bill ID of its own gets the bill ID of the
// answer (every adapter but Karashop), so the rows of the other bill types do not change.
func TestItemBillIDFallsBackToAnswer(t *testing.T) {
	r := newTelRig(t, time.Hour)
	r.karashop.resp = telResp()
	r.karashop.resp.Inquiries[0].BillID = ""
	res, err := r.ask(telNumber, userA)
	if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 2) {
		assert.Equal(t, []string{"2101234560041", "2101234560041"}, []string{res.Inquiries[0].BillID, res.Inquiries[1].BillID})
	}
	if assert.Len(t, r.repo.rows, 2) {
		assert.Equal(t, "2101234560041", r.repo.rows[0].Row.BillID)
	}
}

// TestTelDatabaseAnswers: a valid saved pair answers with no provider call and no request row, with
// the bill ID of each row; another number is another key.
func TestTelDatabaseAnswers(t *testing.T) {
	r := newTelRig(t, time.Hour)
	if _, err := r.ask(telNumber, userA); !assert.NoError(t, err) {
		return
	}
	res, err := r.ask(telNumber, userB)
	if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 2) {
		assert.True(t, res.FromDatabase)
		got := map[string]string{}
		for _, in := range res.Inquiries {
			got[in.InquiryType] = in.BillID + "/" + in.PayID
		}
		assert.Equal(t, map[string]string{provider.InquiryTypeMidterm: "2101234560040/12000035", provider.InquiryTypeFinalTerm: "2101234560041/35000042"}, got)
	}
	assert.Equal(t, 1, r.karashop.calls, "no second provider call")
	assert.Len(t, r.repo.requests, 1, "no second request row")

	_, err = r.ask(telNumber2, userA)
	assert.NoError(t, err)
	assert.Equal(t, 2, r.karashop.calls, "another number asks the provider")
	assert.Equal(t, []string{"tel/karashop/provider_ok", "tel/karashop/db_hit", "tel/karashop/provider_ok"}, r.inquiry)
}

// TestTelSavedRowsThatDoNotAnswer: an expired row, a row of another provider (also of the Karashop
// telmidterm row 123456), a paid row and two saved rows of the same inquiry type are a miss.
func TestTelSavedRowsThatDoNotAnswer(t *testing.T) {
	row := func(id string, providerID int64, typ string) Row {
		return Row{ID: id, InquiryID: telNumber, ProviderID: providerID, ProviderName: "karashop", BillType: telType, InquiryType: typ, Amount: 9}
	}
	for name, seed := range map[string]struct {
		rows []Row
		age  time.Duration
		paid bool
	}{
		"expired":              {[]Row{row("o", 124325, provider.InquiryTypeMidterm)}, 2 * time.Hour, false},
		"another provider row": {[]Row{row("o", 123456, provider.InquiryTypeMidterm)}, time.Minute, false},
		"paid":                 {[]Row{row("o", 124325, provider.InquiryTypeMidterm)}, time.Minute, true},
		"two mid-term rows":    {[]Row{row("a", 124325, provider.InquiryTypeMidterm), row("b", 124325, provider.InquiryTypeMidterm)}, time.Minute, false},
		"two final-term rows":  {[]Row{row("a", 124325, provider.InquiryTypeFinalTerm), row("b", 124325, provider.InquiryTypeFinalTerm)}, time.Minute, false},
	} {
		r := newTelRig(t, time.Hour)
		for _, x := range seed.rows {
			r.repo.seed(x, seed.age, seed.paid)
		}
		res, err := r.ask(telNumber, userA)
		if assert.NoError(t, err, name) {
			assert.False(t, res.FromDatabase, name)
		}
		assert.Equal(t, 1, r.karashop.calls, name)
	}

	r := newTelRig(t, time.Hour)
	r.repo.seed(row("a", 124325, provider.InquiryTypeMidterm), time.Minute, false)
	r.repo.seed(row("b", 124325, provider.InquiryTypeFinalTerm), time.Minute, false)
	res, err := r.ask(telNumber, userA)
	if assert.NoError(t, err) {
		assert.True(t, res.FromDatabase, "a mid-term and a final-term row answer")
	}
	assert.Equal(t, 0, r.karashop.calls)

	r = newTelRig(t, 0)
	_, _ = r.ask(telNumber, userA)
	_, _ = r.ask(telNumber, userA)
	assert.Equal(t, 2, r.karashop.calls, "caching.tel 0: no cache")
}

// TestTelOnlyKarashop (operator decision of 2026-11-14): when the pick for tel is another provider,
// nobody is called and its saved rows are not read. The answer is the one of "no enabled provider".
func TestTelOnlyKarashop(t *testing.T) {
	for _, other := range []provider.Provider{
		{ID: 40, Name: "parsian", BillType: telType, Action: "inquiry", Status: provider.StatusEnable},
		{ID: 52, Name: "qabzino", BillType: telType, Action: "inquiry", Status: provider.StatusEnable},
		{ID: 21, Name: "eways2", BillType: telType, Action: "inquiry", Status: provider.StatusEnable},
		{ID: 77, Name: "Karashop", BillType: telType, Action: "inquiry", Status: provider.StatusEnable}, // the name is exact
	} {
		r := newTelRig(t, time.Hour)
		r.selector.byType[telType] = other
		r.repo.seed(Row{ID: "old", InquiryID: telNumber, ProviderID: other.ID, ProviderName: other.Name, BillType: telType, InquiryType: provider.InquiryTypeMidterm, Amount: 9}, time.Minute, false)
		res, err := r.ask(telNumber, userA)
		assert.Empty(t, res.Inquiries, other.Name)
		if assert.ErrorIs(t, err, provider.ErrNoProvider, other.Name) {
			assert.Contains(t, err.Error(), `only "karashop" may answer`, other.Name)
		}
		assert.Equal(t, 0, r.karashop.calls+r.otherCalls(), other.Name+": nobody is called")
		assert.Equal(t, 0, r.repo.findCalls, other.Name+": its saved rows are not read")
		assert.Empty(t, r.repo.requests, other.Name+": no request row")
		assert.Equal(t, []string{"tel/none/no_provider"}, r.inquiry, other.Name)
	}
}

// TestTelNotAvailable: Karashop disabled (no pick), Karashop down and a Karashop refusal. No other
// provider is called in any of them.
func TestTelNotAvailable(t *testing.T) {
	r := newTelRig(t, time.Hour)
	delete(r.selector.byType, telType)
	_, err := r.ask(telNumber, userA)
	assert.ErrorIs(t, err, provider.ErrNoProvider)
	assert.Equal(t, 0, r.karashop.calls+r.otherCalls())
	assert.Equal(t, []string{"tel/none/no_provider"}, r.inquiry)

	r = newTelRig(t, time.Hour)
	r.karashop.resp, r.karashop.err = nil, errors.Join(provider.ErrProviderDown, errors.New("mid term: HTTP status 500"))
	_, err = r.ask(telNumber, userA)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Equal(t, 1, r.karashop.calls)
	assert.Equal(t, 0, r.otherCalls(), "no fallback")
	if assert.Len(t, r.repo.requests, 1) {
		assert.Equal(t, [2]string{provider.ResponseFailed, ""}, r.repo.finished[r.repo.requests[0].ID])
	}
	assert.Empty(t, r.repo.rows)
	assert.Equal(t, []string{"tel/karashop/provider_down"}, r.inquiry)

	r = newTelRig(t, time.Hour)
	r.karashop.resp = &provider.InquiryResponse{Status: provider.ResponseFailed, Code: 2, Message: "تلفن نامعتبر است", ProviderTraceID: "t"}
	_, err = r.ask(telNumber, userA)
	var refused *RefusedError
	if assert.ErrorAs(t, err, &refused) && assert.NotNil(t, refused) {
		assert.Equal(t, "تلفن نامعتبر است", refused.Message)
	}
	assert.Empty(t, r.repo.rows)
	assert.Equal(t, []string{"tel/karashop/provider_refused"}, r.inquiry)
}

// TestTelInvalidInquiryID: the inquiry ID must be a landline number in the 98 form.
func TestTelInvalidInquiryID(t *testing.T) {
	for _, id := range []string{"", "02188776655", "2188776655", "98218877665", "9821887766551", "98218877665a", validBill} {
		r := newTelRig(t, time.Hour)
		_, err := r.ask(id, userA)
		assert.ErrorIs(t, err, ErrInvalid, id)
		assert.Equal(t, 0, r.karashop.calls, id)
		assert.Empty(t, r.selector.picks, id)
		assert.Empty(t, r.inquiry, id)
	}
}

// TestTelTID: the tid of a request row starts with 4 (bill: TelephoneReference).
func TestTelTID(t *testing.T) {
	tid, err := generateTID(telType)
	if assert.NoError(t, err) && assert.NotEmpty(t, tid) {
		assert.Equal(t, byte('4'), tid[0])
	}
}
