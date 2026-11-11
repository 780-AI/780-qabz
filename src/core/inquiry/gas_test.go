package inquiry

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// Task 023: the same flow answers gas (provider Qabzino). The inquiry ID is the participate code,
// not a bill ID: it is the key of the saved inquiries and what the provider gets.

const (
	gasCode  = "0012345678" // a participate code: not a valid bill ID
	gasCode2 = "0098765432"
	gasBill  = "1234567890125" // the bill ID that the provider answers
)

var qabzinoGas = provider.Provider{ID: 35, Name: "qabzino", BillType: "gas", Action: "inquiry", Status: provider.StatusEnable}

func gasDetails() *provider.Details {
	return &provider.Details{Gas: &provider.GasDetails{City: "ci", Kind: "ki"}}
}

func gasResp() *provider.InquiryResponse {
	return &provider.InquiryResponse{
		Status: provider.ResponseSuccess, BillID: gasBill, ProviderTraceID: "trace-g", Details: gasDetails(),
		Inquiries: []provider.InquiryItem{{PayID: "3200012345", Amount: 32000000, PaymentDeadline: "2030-01-01", InquiryType: "regular"}},
	}
}

type gasRig struct {
	svc      *Service
	repo     *memRepo
	gas      *stubProvider
	selector *typeSelector
	inquiry  []string
	tidAsked []string
}

// newGasRig is one Service with Qabzino as the provider of gas (id 35) and of water (id 36).
func newGasRig(t *testing.T, ttl time.Duration) *gasRig {
	t.Helper()
	r := &gasRig{repo: newMemRepo(), selector: &typeSelector{byType: map[string]provider.Provider{"gas": qabzinoGas, "water": qabzino}}}
	r.gas = &stubProvider{name: "qabzino", resp: gasResp()}
	svc, err := NewService(Options{
		Repository: r.repo, Providers: r.selector, Adapters: fakeAdapters{"qabzino": r.gas},
		CacheTTL:  func(billType string) time.Duration { return ttl },
		OnInquiry: func(b, p, res string) { r.inquiry = append(r.inquiry, b+"/"+p+"/"+res) },
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

func (r *gasRig) ask(billType, inquiryID, user string) (Result, error) {
	return r.svc.Inquire(context.Background(), Request{BillType: billType, InquiryID: inquiryID, UserID: user, Dialer: "989121234567"})
}

// TestGasMissThenDatabase: a participate code that is not a bill ID is accepted for gas; the
// provider gets it; the rows are saved under it; the second call is answered from the database
// with the same values and details.
func TestGasMissThenDatabase(t *testing.T) {
	r := newGasRig(t, time.Hour)
	res, err := r.ask("gas", gasCode, userA)
	if !assert.NoError(t, err) || !assert.Len(t, res.Inquiries, 1) {
		return
	}
	assert.False(t, res.FromDatabase)
	assert.Equal(t, gasBill, res.BillID, "the bill ID of the provider, not the participate code")
	assert.Equal(t, gasDetails(), res.Details)
	assert.Equal(t, Inquiry{ID: res.Inquiries[0].ID, BillID: gasBill, PayID: "3200012345", Amount: 32000000, PaymentDeadline: "2030-01-01", InquiryType: "regular"}, res.Inquiries[0])
	if assert.Len(t, r.gas.reqs, 1) {
		assert.Equal(t, provider.InquiryRequest{BillType: "gas", UserID: userA, Dialer: "989121234567", InquiryID: gasCode}, r.gas.reqs[0])
	}
	assert.Equal(t, []string{"gas/inquiry"}, r.selector.picks)
	assert.Equal(t, []string{"gas"}, r.tidAsked)
	if assert.Len(t, r.repo.requests, 1) {
		q := r.repo.requests[0]
		assert.Equal(t, []string{"gas", "qabzino", "tid-gas", gasCode, userA}, []string{q.BillType, q.ProviderName, q.TID, q.InquiryID, q.UserID})
		assert.Equal(t, [2]string{"success", "trace-g"}, r.repo.finished[q.ID])
	}
	if assert.Len(t, r.repo.rows, 1) {
		row := r.repo.rows[0].Row
		assert.Equal(t, []any{"gas", int64(35), "qabzino", gasCode, gasBill, "trace-g", false},
			[]any{row.BillType, row.ProviderID, row.ProviderName, row.InquiryID, row.BillID, row.ProviderTraceID, row.IsCached})
		assert.Equal(t, gasDetails(), row.Details, "the details are saved with the inquiry")
	}

	res2, err := r.ask("gas", gasCode, userB)
	if !assert.NoError(t, err) || !assert.Len(t, res2.Inquiries, 1) {
		return
	}
	assert.True(t, res2.FromDatabase)
	assert.Equal(t, res.BillID, res2.BillID)
	assert.Equal(t, res.Details, res2.Details)
	assert.Equal(t, []any{res.Inquiries[0].PayID, res.Inquiries[0].Amount, res.Inquiries[0].PaymentDeadline},
		[]any{res2.Inquiries[0].PayID, res2.Inquiries[0].Amount, res2.Inquiries[0].PaymentDeadline})
	assert.Equal(t, 1, r.gas.calls, "no second provider call")
	assert.Len(t, r.repo.requests, 1, "no new request row")
	if assert.Len(t, r.repo.rows, 2) {
		cp := r.repo.rows[1].Row
		assert.Equal(t, []any{true, userB, gasCode}, []any{cp.IsCached, cp.UserID, cp.InquiryID})
	}
	assert.Equal(t, []string{"gas/qabzino/provider_ok", "gas/qabzino/db_hit"}, r.inquiry)
}

// TestGasParticipateCodeIsTheCacheKey: another participate code is another key (a provider call),
// and a saved row of the first code does not answer it.
func TestGasParticipateCodeIsTheCacheKey(t *testing.T) {
	r := newGasRig(t, time.Hour)
	for _, code := range []string{gasCode, gasCode2} {
		res, err := r.ask("gas", code, userA)
		if assert.NoError(t, err, code) {
			assert.False(t, res.FromDatabase, code)
		}
	}
	assert.Equal(t, 2, r.gas.calls, "two participate codes: two provider calls")
	if assert.Len(t, r.gas.reqs, 2) {
		assert.Equal(t, []string{gasCode, gasCode2}, []string{r.gas.reqs[0].InquiryID, r.gas.reqs[1].InquiryID})
	}
	for _, code := range []string{gasCode, gasCode2} {
		res, err := r.ask("gas", code, userB)
		if assert.NoError(t, err, code) {
			assert.True(t, res.FromDatabase, code)
		}
	}
	assert.Equal(t, 2, r.gas.calls)
}

// TestGasRowsThatDoNotAnswer: an expired row, a row of another provider, a paid row and a copy do
// not answer; the provider is called.
func TestGasRowsThatDoNotAnswer(t *testing.T) {
	base := Row{UserID: userB, InquiryID: gasCode, ProviderID: 35, ProviderName: "qabzino", BillType: "gas", BillID: gasBill, PayID: "old", Amount: 1}
	cases := []struct {
		name string
		seed func(m *memRepo)
	}{
		{"expired", func(m *memRepo) { m.seed(base, 2*time.Hour, false) }},
		{"other provider", func(m *memRepo) { x := base; x.ProviderID = 43; m.seed(x, time.Minute, false) }},
		{"paid", func(m *memRepo) { m.seed(base, time.Minute, true) }},
		{"copy", func(m *memRepo) { x := base; x.IsCached = true; m.seed(x, time.Minute, false) }},
		{"other participate code", func(m *memRepo) { x := base; x.InquiryID = gasCode2; m.seed(x, time.Minute, false) }},
	}
	for _, c := range cases {
		r := newGasRig(t, time.Hour)
		c.seed(r.repo)
		res, err := r.ask("gas", gasCode, userA)
		if assert.NoError(t, err, c.name) && assert.Len(t, res.Inquiries, 1, c.name) {
			assert.False(t, res.FromDatabase, c.name)
			assert.Equal(t, "3200012345", res.Inquiries[0].PayID, c.name)
		}
		assert.Equal(t, 1, r.gas.calls, c.name)
	}
	// Positive control: the same row, valid, answers with no provider call.
	r := newGasRig(t, time.Hour)
	r.repo.seed(base, time.Minute, false)
	res, err := r.ask("gas", gasCode, userA)
	if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 1) {
		assert.True(t, res.FromDatabase)
		assert.Equal(t, "old", res.Inquiries[0].PayID)
	}
	assert.Equal(t, 0, r.gas.calls)
}

// TestInquiryIDRulePerBillType: the rule of the inquiry ID is the one of the bill type that the
// handler gives. Gas: not empty (any text, also one that is not a bill ID, and a valid bill ID).
// Water and electricity: a valid bill ID, as before task 023.
func TestInquiryIDRulePerBillType(t *testing.T) {
	r := newGasRig(t, 0)
	r.selector.byType["electricity"] = tavanir
	r.svc.adapters = fakeAdapters{"qabzino": r.gas, "tavanir": r.gas}
	cases := []struct {
		billType, id string
		valid        bool
		text         string
	}{
		{"gas", gasCode, true, ""},
		{"gas", "1", true, ""},
		{"gas", validBill, true, ""},
		{"gas", "abc-12", true, ""},
		{"gas", strings.Repeat("9", 40), true, ""}, // as in bill: no length rule in the flow
		{"gas", "", false, "participate code"},
		{"water", validBill, true, ""},
		{"water", gasCode, false, "bill id"},
		{"water", "", false, "bill id"},
		{"water", "1234567890124", false, "bill id"}, // a wrong check digit
		{"electricity", validBill, true, ""},
		{"electricity", gasCode, false, "bill id"},
	}
	for _, c := range cases {
		before := r.gas.calls
		_, err := r.ask(c.billType, c.id, userA)
		name := c.billType + " " + c.id
		if c.valid {
			assert.NoError(t, err, name)
			assert.Equal(t, before+1, r.gas.calls, name)
			continue
		}
		assert.ErrorIs(t, err, ErrInvalid, name)
		assert.ErrorContains(t, err, c.text, name)
		assert.Equal(t, before, r.gas.calls, name+": no provider call for an invalid request")
	}
	// The user rule is the same for gas.
	_, err := r.ask("gas", gasCode, "not-a-uuid")
	assert.ErrorIs(t, err, ErrInvalid)
	assert.ErrorContains(t, err, "user")
}

// TestGasFailures: no enabled provider, a provider that is down, a provider with no inquiry and a
// provider with no adapter give the same errors as for water, with the gas labels.
func TestGasFailures(t *testing.T) {
	r := newGasRig(t, time.Hour)
	r.gas.resp, r.gas.err = nil, provider.ErrProviderDown
	_, err := r.ask("gas", gasCode, userA)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	if assert.Len(t, r.repo.requests, 1) {
		assert.Equal(t, [2]string{"failed", ""}, r.repo.finished[r.repo.requests[0].ID])
	}
	assert.Empty(t, r.repo.rows, "a failed call saves no inquiry")

	r.gas.err, r.gas.resp = nil, &provider.InquiryResponse{Status: provider.ResponseFailed, Code: 2037, Message: "no such code"}
	_, err = r.ask("gas", gasCode, userA)
	var refused *RefusedError
	if assert.ErrorAs(t, err, &refused) && assert.NotNil(t, refused) {
		assert.Equal(t, "no such code", refused.Message)
		assert.EqualValues(t, 2037, refused.Code)
	}
	assert.Empty(t, r.repo.rows)

	r.svc.adapters = fakeAdapters{}
	_, err = r.ask("gas", gasCode, userA)
	assert.ErrorIs(t, err, ErrNoAdapter)

	delete(r.selector.byType, "gas")
	_, err = r.ask("gas", gasCode, userA)
	assert.ErrorIs(t, err, provider.ErrNoProvider)
	assert.Equal(t, []string{"gas/qabzino/provider_down", "gas/qabzino/provider_refused", "gas/qabzino/error", "gas/none/no_provider"}, r.inquiry)
}

// TestGasPaidAnswer: a success answer with one inquiry of amount 0 and no pay ID (GD2038, no bill
// to pay) is an answer, and it is saved.
func TestGasPaidAnswer(t *testing.T) {
	r := newGasRig(t, time.Hour)
	r.gas.resp = &provider.InquiryResponse{Status: provider.ResponseSuccess, Inquiries: []provider.InquiryItem{{InquiryType: "regular"}}}
	res, err := r.ask("gas", gasCode, userA)
	if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 1) {
		assert.Equal(t, []any{"", uint64(0), ""}, []any{res.Inquiries[0].PayID, res.Inquiries[0].Amount, res.BillID})
	}
	assert.Len(t, r.repo.rows, 1)
}

func TestGenerateTIDGas(t *testing.T) {
	for range 20 {
		tid, err := generateTID("gas")
		if !assert.NoError(t, err) {
			return
		}
		assert.Regexp(t, `^3[0-9]{3,14}$`, tid, "bill: GasReference is 3")
	}
}

// TestBillTypesTable: every supported bill type has a prefix of its own and a rule.
func TestBillTypesTable(t *testing.T) {
	assert.Len(t, billTypes, 4)
	prefixes := map[int]string{}
	for name, bt := range billTypes {
		assert.NotNil(t, bt.validInquiryID, name)
		assert.NotEmpty(t, bt.invalid, name)
		assert.Empty(t, prefixes[bt.tidPrefix], "prefix %d is used twice", bt.tidPrefix)
		prefixes[bt.tidPrefix] = name
	}
	assert.Equal(t, map[int]string{1: "water", 2: "electricity", 3: "gas", 8: "irancell"}, prefixes)
}
