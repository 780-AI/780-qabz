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

// Task 036: the same flow answers the MCI mobile bill (the bill type "mobile"). The inquiry ID is
// the phone; one answer has a mid-term and a final-term inquiry. One request calls one provider:
// the pick of the sorted list. What that provider returns is the answer; the flow never calls a
// second provider in the same request (operator rule: the Provider Switcher moves a bill type to
// the next provider, not the flow).

const mciPhone = "989121234568"

var (
	jiringRow  = provider.Provider{ID: 28, Name: "jiring-switch", BillType: "mobile", Action: "inquiry", Status: provider.StatusEnable}
	parsianRow = provider.Provider{ID: 39, Name: "parsian", BillType: "mobile", Action: "inquiry", Status: provider.StatusEnable}
)

func jiringResp() *provider.InquiryResponse {
	return &provider.InquiryResponse{
		Status: provider.ResponseSuccess, BillID: "9120000036",
		Inquiries: []provider.InquiryItem{
			{PayID: "21003611", Amount: 210000, InquiryType: provider.InquiryTypeMidterm},
			{PayID: "61003622", Amount: 610000, InquiryType: provider.InquiryTypeFinalTerm},
		},
	}
}

func parsianResp() *provider.InquiryResponse {
	return &provider.InquiryResponse{
		Status: provider.ResponseSuccess, BillID: "9120000039", ProviderTraceID: "trace-pa",
		Inquiries: []provider.InquiryItem{
			{PayID: "22003911", Amount: 220000, PaymentDeadline: "2026/12/20", InquiryType: provider.InquiryTypeMidterm},
			{PayID: "62003922", Amount: 620000, PaymentDeadline: "2026/12/20", InquiryType: provider.InquiryTypeFinalTerm},
		},
	}
}

type mobileRig struct {
	svc      *Service
	repo     *memRepo
	jiring   *stubProvider
	parsian  *stubProvider
	selector *typeSelector
	log      *recLog
	inquiry  []string
	calls    []call
	tidAsked []string
}

// newMobileRig is one Service with the two adapters of the MCI bill. pick is the provider row that
// the selector gives for (mobile, inquiry): the first enabled row of the sorted list.
func newMobileRig(t *testing.T, ttl time.Duration, pick provider.Provider) *mobileRig {
	t.Helper()
	r := &mobileRig{repo: newMemRepo(), log: &recLog{}, selector: &typeSelector{byType: map[string]provider.Provider{"mobile": pick}}}
	r.jiring = &stubProvider{name: "jiring-switch", resp: jiringResp()}
	r.parsian = &stubProvider{name: "parsian", resp: parsianResp()}
	svc, err := NewService(Options{
		Repository: r.repo, Providers: r.selector, Adapters: fakeAdapters{"jiring-switch": r.jiring, "parsian": r.parsian},
		CacheTTL: func(string) time.Duration { return ttl }, Logger: r.log,
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

func (r *mobileRig) ask(phone, user string) (Result, error) {
	return r.svc.Inquire(context.Background(), Request{BillType: provider.BillTypeMobile, InquiryID: phone, UserID: user, Dialer: "989121234567"})
}

// TestMobileJiringAnswers: Jiring Switch is the pick. It gets the phone; the two inquiries are two
// rows with the bill ID, the pay IDs and the Jiring Switch provider; Parsian is not called.
func TestMobileJiringAnswers(t *testing.T) {
	r := newMobileRig(t, time.Hour, jiringRow)
	res, err := r.ask(mciPhone, userA)
	if !assert.NoError(t, err) || !assert.Len(t, res.Inquiries, 2) {
		return
	}
	assert.False(t, res.FromDatabase)
	assert.Equal(t, "9120000036", res.BillID)
	assert.Equal(t, []string{provider.InquiryTypeMidterm, provider.InquiryTypeFinalTerm}, []string{res.Inquiries[0].InquiryType, res.Inquiries[1].InquiryType})
	assert.Equal(t, []uint64{210000, 610000}, []uint64{res.Inquiries[0].Amount, res.Inquiries[1].Amount})
	assert.Equal(t, []string{"21003611", "61003622"}, []string{res.Inquiries[0].PayID, res.Inquiries[1].PayID})

	if assert.Len(t, r.jiring.reqs, 1) {
		assert.Equal(t, provider.InquiryRequest{BillType: "mobile", UserID: userA, Dialer: "989121234567", InquiryID: mciPhone}, r.jiring.reqs[0])
	}
	assert.Equal(t, 0, r.parsian.calls, "Parsian is not called")
	assert.Equal(t, []string{"mobile/inquiry"}, r.selector.picks, "one pick for one request")
	assert.Equal(t, []string{"mobile"}, r.tidAsked)

	if assert.Len(t, r.repo.requests, 1) {
		q := r.repo.requests[0]
		assert.Equal(t, []string{userA, mciPhone, "mobile", "tid-mobile", "jiring-switch", RequestInitial},
			[]string{q.UserID, q.InquiryID, q.BillType, q.TID, q.ProviderName, q.Status})
		assert.Equal(t, [2]string{provider.ResponseSuccess, ""}, r.repo.finished[q.ID], "Jiring Switch gives no trace ID")
	}
	if assert.Len(t, r.repo.rows, 2) {
		for i, want := range []struct {
			typ, payID string
			amount     uint64
		}{{provider.InquiryTypeMidterm, "21003611", 210000}, {provider.InquiryTypeFinalTerm, "61003622", 610000}} {
			row := r.repo.rows[i].Row
			assert.Equal(t, []string{userA, mciPhone, "jiring-switch", "mobile", "", want.typ, "9120000036", want.payID, ""},
				[]string{row.UserID, row.InquiryID, row.ProviderName, row.BillType, row.ProviderTraceID, row.InquiryType, row.BillID, row.PayID, row.PaymentDeadline})
			assert.Equal(t, want.amount, row.Amount)
			assert.EqualValues(t, 28, row.ProviderID)
			assert.False(t, row.IsCached)
		}
	}
	assert.Equal(t, []string{"mobile/jiring-switch/provider_ok"}, r.inquiry)
	if assert.Len(t, r.calls, 1) {
		assert.Equal(t, []string{"jiring-switch", "inquiry", CallOK}, []string{r.calls[0].provider, r.calls[0].action, r.calls[0].result})
	}
}

// TestMobileParsianAnswersWhenItIsThePick: when the pick is Parsian (the Jiring Switch row is
// disabled, or a provider switch moved it), Parsian answers with its trace ID and its payment
// deadline (fixed to yyyy-mm-dd); Jiring Switch is not called.
func TestMobileParsianAnswersWhenItIsThePick(t *testing.T) {
	r := newMobileRig(t, time.Hour, parsianRow)
	res, err := r.ask(mciPhone, userA)
	if !assert.NoError(t, err) || !assert.Len(t, res.Inquiries, 2) {
		return
	}
	assert.Equal(t, "9120000039", res.BillID)
	assert.Equal(t, []string{"2026-12-20", "2026-12-20"}, []string{res.Inquiries[0].PaymentDeadline, res.Inquiries[1].PaymentDeadline})
	assert.Equal(t, 1, r.parsian.calls)
	assert.Equal(t, 0, r.jiring.calls, "Jiring Switch is not called")
	if assert.Len(t, r.repo.requests, 1) {
		assert.Equal(t, "parsian", r.repo.requests[0].ProviderName)
		assert.Equal(t, [2]string{provider.ResponseSuccess, "trace-pa"}, r.repo.finished[r.repo.requests[0].ID])
	}
	if assert.Len(t, r.repo.rows, 2) {
		assert.EqualValues(t, 39, r.repo.rows[0].ProviderID)
		assert.Equal(t, "trace-pa", r.repo.rows[0].ProviderTraceID)
	}
	assert.Equal(t, []string{"mobile/parsian/provider_ok"}, r.inquiry)
}

// TestMobileProviderErrorIsTheAnswer (operator rule of task 036): when the picked provider fails or
// answers with something that is not usable, that is the answer of the request. The flow does not
// call the next provider: no second pick, no second request row, no call to Parsian.
func TestMobileProviderErrorIsTheAnswer(t *testing.T) {
	cases := []struct {
		name       string
		resp       *provider.InquiryResponse
		err        error
		wantDown   bool
		wantStatus string
		wantResult string
		wantCall   string
	}{
		{"the call fails (provider down, a timeout)", nil, fmt.Errorf("jiring: %w: timeout", provider.ErrProviderDown), true, provider.ResponseFailed, ResultProviderDown, CallDown},
		{"another adapter error", nil, errors.New("boom"), false, provider.ResponseFailed, ResultError, CallError},
		{"an empty answer", nil, nil, false, provider.ResponseFailed, ResultError, CallError},
		{"a pending answer (Jiring itself did not respond)", &provider.InquiryResponse{Status: "pending", Code: 500, Message: "later"}, nil, false, "pending", ResultProviderRefused, CallRefused},
		{"a failed answer (a business answer)", &provider.InquiryResponse{Status: provider.ResponseFailed, Code: 400, Message: "not an MCI number"}, nil, false, provider.ResponseFailed, ResultProviderRefused, CallRefused},
	}
	for _, c := range cases {
		r := newMobileRig(t, time.Hour, jiringRow)
		r.jiring.resp, r.jiring.err = c.resp, c.err
		res, err := r.ask(mciPhone, userA)
		if !assert.Error(t, err, c.name) {
			continue
		}
		assert.Empty(t, res.Inquiries, c.name)
		assert.Equal(t, c.wantDown, errors.Is(err, provider.ErrProviderDown), c.name)
		var refused *RefusedError
		if c.wantResult == ResultProviderRefused && assert.ErrorAs(t, err, &refused, c.name) && assert.NotNil(t, refused, c.name) {
			assert.Equal(t, c.resp.Message, refused.Message, c.name)
		}
		assert.Equal(t, 1, r.jiring.calls, c.name)
		assert.Equal(t, 0, r.parsian.calls, c.name+": the next provider is not called in the same request")
		assert.Equal(t, []string{"mobile/inquiry"}, r.selector.picks, c.name+": one pick")
		if assert.Len(t, r.repo.requests, 1, c.name+": one request row") {
			assert.Equal(t, "jiring-switch", r.repo.requests[0].ProviderName, c.name)
			assert.Equal(t, c.wantStatus, r.repo.finished[r.repo.requests[0].ID][0], c.name)
		}
		assert.Empty(t, r.repo.rows, c.name+": no inquiry is saved")
		assert.Equal(t, []string{"mobile/jiring-switch/" + c.wantResult}, r.inquiry, c.name)
		if assert.Len(t, r.calls, 1, c.name) {
			assert.Equal(t, []string{"jiring-switch", c.wantCall}, []string{r.calls[0].provider, r.calls[0].result}, c.name)
		}
	}
}

// TestMobileNoProvider: no enabled provider of (mobile, inquiry) is ErrNoProvider and
// nobody is called.
func TestMobileNoProvider(t *testing.T) {
	r := newMobileRig(t, time.Hour, jiringRow)
	delete(r.selector.byType, "mobile")
	_, err := r.ask(mciPhone, userA)
	assert.ErrorIs(t, err, provider.ErrNoProvider)
	assert.Equal(t, 0, r.jiring.calls+r.parsian.calls)
	assert.Empty(t, r.repo.requests)
	assert.Equal(t, []string{"mobile/none/no_provider"}, r.inquiry)
}

// TestMobileDatabaseAnswers: the second call within the TTL is answered from the two saved rows of
// the picked provider with no provider call.
func TestMobileDatabaseAnswers(t *testing.T) {
	r := newMobileRig(t, time.Hour, jiringRow)
	first, err := r.ask(mciPhone, userA)
	if !assert.NoError(t, err) || !assert.Len(t, first.Inquiries, 2) {
		return
	}
	second, err := r.ask(mciPhone, userB)
	if !assert.NoError(t, err) || !assert.Len(t, second.Inquiries, 2) {
		return
	}
	assert.True(t, second.FromDatabase)
	assert.Equal(t, "9120000036", second.BillID)
	assert.Equal(t, 1, r.jiring.calls, "no second provider call")
	assert.Len(t, r.repo.requests, 1, "no second request row")
	got := map[string]uint64{}
	for _, in := range second.Inquiries {
		got[in.InquiryType] = in.Amount
	}
	assert.Equal(t, map[string]uint64{provider.InquiryTypeMidterm: 210000, provider.InquiryTypeFinalTerm: 610000}, got)
	assert.Equal(t, []string{"mobile/jiring-switch/provider_ok", "mobile/jiring-switch/db_hit"}, r.inquiry)

	// With the provider down, the saved rows still answer.
	r.jiring.resp, r.jiring.err = nil, provider.ErrProviderDown
	third, err := r.ask(mciPhone, userA)
	assert.NoError(t, err)
	assert.True(t, third.FromDatabase)
	assert.Equal(t, 1, r.jiring.calls)
}

// TestMobileRowsThatDoNotAnswer: rows of the other provider, expired rows, paid rows and copies do
// not answer; the picked provider is asked.
func TestMobileRowsThatDoNotAnswer(t *testing.T) {
	pair := func(providerID int64, name string) []Row {
		return []Row{
			{ID: "s1", UserID: userB, InquiryID: mciPhone, ProviderID: providerID, ProviderName: name, BillType: "mobile", InquiryType: provider.InquiryTypeMidterm, Amount: 1},
			{ID: "s2", UserID: userB, InquiryID: mciPhone, ProviderID: providerID, ProviderName: name, BillType: "mobile", InquiryType: provider.InquiryTypeFinalTerm, Amount: 2},
		}
	}
	cases := []struct {
		name string
		seed func(m *memRepo)
	}{
		{"rows of the other provider (Parsian) while Jiring Switch is the pick", func(m *memRepo) {
			for _, r := range pair(39, "parsian") {
				m.seed(r, time.Minute, false)
			}
		}},
		{"expired rows", func(m *memRepo) {
			for _, r := range pair(28, "jiring-switch") {
				m.seed(r, 2*time.Hour, false)
			}
		}},
		{"paid rows", func(m *memRepo) {
			for _, r := range pair(28, "jiring-switch") {
				m.seed(r, time.Minute, true)
			}
		}},
		{"copies", func(m *memRepo) {
			for _, r := range pair(28, "jiring-switch") {
				r.IsCached = true
				m.seed(r, time.Minute, false)
			}
		}},
	}
	for _, c := range cases {
		r := newMobileRig(t, time.Hour, jiringRow)
		c.seed(r.repo)
		res, err := r.ask(mciPhone, userA)
		if assert.NoError(t, err, c.name) {
			assert.False(t, res.FromDatabase, c.name)
			assert.Equal(t, 1, r.jiring.calls, c.name)
			assert.Equal(t, 0, r.parsian.calls, c.name)
		}
	}
}

// TestMobileCacheSkipSameTypePair (bill: shouldSkipCache): two saved rows of the same inquiry type
// are not one answer, so the provider is asked. A mid-term and a final-term row answer; one saved
// row answers too (bill skips only a pair). The rule is for the bill type mobile only.
func TestMobileCacheSkipSameTypePair(t *testing.T) {
	row := func(id, typ string) Row {
		return Row{ID: id, UserID: userB, InquiryID: mciPhone, ProviderID: 28, ProviderName: "jiring-switch", BillType: "mobile", InquiryType: typ, Amount: 5}
	}
	cases := []struct {
		name     string
		types    []string
		wantDB   bool
		wantCall int
	}{
		{"two mid-term rows", []string{provider.InquiryTypeMidterm, provider.InquiryTypeMidterm}, false, 1},
		{"two final-term rows", []string{provider.InquiryTypeFinalTerm, provider.InquiryTypeFinalTerm}, false, 1},
		{"a mid-term and a final-term row", []string{provider.InquiryTypeMidterm, provider.InquiryTypeFinalTerm}, true, 0},
		{"one row", []string{provider.InquiryTypeMidterm}, true, 0},
		{"two rows of another type", []string{"regular", "regular"}, true, 0},
	}
	for _, c := range cases {
		r := newMobileRig(t, time.Hour, jiringRow)
		for i, typ := range c.types {
			r.repo.seed(row(fmt.Sprintf("s%d", i), typ), time.Minute, false)
		}
		res, err := r.ask(mciPhone, userA)
		if assert.NoError(t, err, c.name) {
			assert.Equal(t, c.wantDB, res.FromDatabase, c.name)
			assert.Equal(t, c.wantCall, r.jiring.calls, c.name)
		}
	}

	// The same pair does not stop another bill type (irancell) from answering from the database.
	ir := newIrancellRig(t, time.Hour)
	for i := range 2 {
		ir.repo.seed(Row{ID: fmt.Sprintf("i%d", i), UserID: userB, InquiryID: irPhone, ProviderID: 38, ProviderName: "irancell", BillType: "irancell", InquiryType: provider.InquiryTypeMidterm, Amount: 5}, time.Minute, false)
	}
	res, err := ir.ask(irPhone, userA)
	if assert.NoError(t, err) {
		assert.True(t, res.FromDatabase, "irancell has no cache skip, as in bill")
		assert.Equal(t, 0, ir.irancell.calls)
	}
}

// TestMobileCacheOffAndInvalid: with no TTL every call asks the provider; the inquiry ID must be a
// mobile number in the 98 form.
func TestMobileCacheOffAndInvalid(t *testing.T) {
	r := newMobileRig(t, 0, jiringRow)
	for range 2 {
		_, err := r.ask(mciPhone, userA)
		assert.NoError(t, err)
	}
	assert.Equal(t, 2, r.jiring.calls)
	assert.Equal(t, 0, r.repo.findCalls, "the saved inquiries are not read")

	for _, phone := range []string{"", "09121234568", "1234567890125", "98912123456"} {
		_, err := r.ask(phone, userA)
		assert.ErrorIs(t, err, ErrInvalid, phone)
		if err != nil {
			assert.Contains(t, err.Error(), "phone", phone)
		}
	}
	_, err := r.ask(mciPhone, "not-a-uuid")
	assert.ErrorIs(t, err, ErrInvalid)
	assert.Equal(t, 2, r.jiring.calls, "an invalid request calls nobody")
}

func TestGenerateTIDMobile(t *testing.T) {
	for range 20 {
		tid, err := generateTID(provider.BillTypeMobile)
		if !assert.NoError(t, err) {
			return
		}
		assert.Regexp(t, `^5[0-9]{3,14}$`, tid, "bill: MCIMobileReference is 5")
	}
}
