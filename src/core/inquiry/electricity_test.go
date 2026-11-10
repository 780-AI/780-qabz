package inquiry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// Task 022: the same flow answers a second bill type (electricity, provider Tavanir) and carries
// the details of the answer.

var tavanir = provider.Provider{ID: 2, Name: "tavanir", BillType: "electricity", Action: "inquiry", Status: provider.StatusEnable}

// typeSelector picks the provider of each bill type and records the picks.
type typeSelector struct {
	byType map[string]provider.Provider
	picks  []string
}

func (s *typeSelector) PickProvider(billType, action string) (provider.Provider, error) {
	s.picks = append(s.picks, billType+"/"+action)
	p, ok := s.byType[billType]
	if !ok {
		return provider.Provider{}, provider.ErrNoProvider
	}
	return p, nil
}

func electricityDetails() *provider.Details {
	return &provider.Details{Electricity: &provider.ElectricityDetails{
		Phase: "ph", CompanyName: "co", TariffType: "ta", CustomerType: 2, SerialNumber: "se",
		ServiceAddress: "ad", ServicePostCode: "po", CustomerName: "na", City: "co", Region: "re",
	}}
}

func electricityResp(d *provider.Details) *provider.InquiryResponse {
	return &provider.InquiryResponse{
		Status: provider.ResponseSuccess, BillID: validBill, Details: d,
		Inquiries: []provider.InquiryItem{{PayID: "1800012345", Amount: 18000000, PaymentDeadline: "2030-01-01", InquiryType: "regular"}},
	}
}

type multiRig struct {
	svc      *Service
	repo     *memRepo
	water    *stubProvider
	power    *stubProvider
	selector *typeSelector
	inquiry  []string
	ttlAsked []string
	tidAsked []string
}

// newMultiRig is one Service for water (Qabzino) and electricity (Tavanir), with a TTL per bill type.
func newMultiRig(t *testing.T, ttl map[string]time.Duration, details *provider.Details) *multiRig {
	t.Helper()
	r := &multiRig{repo: newMemRepo(), selector: &typeSelector{byType: map[string]provider.Provider{"water": qabzino, "electricity": tavanir}}}
	r.water = &stubProvider{name: "qabzino", resp: successResp()}
	r.power = &stubProvider{name: "tavanir", resp: electricityResp(details)}
	svc, err := NewService(Options{
		Repository: r.repo, Providers: r.selector, Adapters: fakeAdapters{"qabzino": r.water, "tavanir": r.power},
		CacheTTL: func(billType string) time.Duration {
			r.ttlAsked = append(r.ttlAsked, billType)
			return ttl[billType]
		},
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

func (r *multiRig) ask(billType, user string) (Result, error) {
	return r.svc.Inquire(context.Background(), Request{BillType: billType, InquiryID: validBill, UserID: user, Dialer: "989121234567"})
}

// TestElectricityDetailsSavedAndReturnedTheSame: the provider answer and the answer from saved
// inquiries have the same details; the details are saved with the row and with the copy.
func TestElectricityDetailsSavedAndReturnedTheSame(t *testing.T) {
	want := electricityDetails()
	r := newMultiRig(t, map[string]time.Duration{"electricity": time.Hour}, electricityDetails())

	res, err := r.ask("electricity", userA)
	if !assert.NoError(t, err) || !assert.Len(t, res.Inquiries, 1) {
		return
	}
	assert.False(t, res.FromDatabase)
	assert.Equal(t, want, res.Details)
	assert.Equal(t, uint64(18000000), res.Inquiries[0].Amount)
	assert.Equal(t, 1, r.power.calls)
	assert.Equal(t, 0, r.water.calls, "the adapter of the picked provider only")
	if assert.Len(t, r.power.reqs, 1) {
		assert.Equal(t, provider.InquiryRequest{BillType: "electricity", UserID: userA, Dialer: "989121234567", InquiryID: validBill}, r.power.reqs[0])
	}
	assert.Equal(t, []string{"electricity/inquiry"}, r.selector.picks)
	assert.Equal(t, []string{"electricity"}, r.tidAsked, "the tid has the prefix of the bill type")
	if assert.Len(t, r.repo.requests, 1) {
		q := r.repo.requests[0]
		assert.Equal(t, []string{"electricity", "tavanir", "tid-electricity"}, []string{q.BillType, q.ProviderName, q.TID})
		assert.Equal(t, [2]string{"success", ""}, r.repo.finished[q.ID], "Tavanir has no trace ID")
	}
	if assert.Len(t, r.repo.rows, 1) {
		row := r.repo.rows[0].Row
		assert.Equal(t, want, row.Details, "the details are saved with the inquiry")
		assert.Equal(t, []any{"electricity", int64(2), "tavanir", ""}, []any{row.BillType, row.ProviderID, row.ProviderName, row.ProviderTraceID})
	}

	// The second call (another user), then a third: from the database, the same details.
	for i, user := range []string{userB, userA, userB} {
		res2, err := r.ask("electricity", user)
		if !assert.NoError(t, err) || !assert.Len(t, res2.Inquiries, 1) {
			return
		}
		assert.True(t, res2.FromDatabase, i)
		assert.Equal(t, res.Details, res2.Details, "a cache hit gives the same details as the provider call")
		assert.Equal(t, res.BillID, res2.BillID)
		assert.Equal(t, res.Inquiries[0].PayID, res2.Inquiries[0].PayID)
		assert.Equal(t, res.Inquiries[0].Amount, res2.Inquiries[0].Amount)
		assert.Equal(t, res.Inquiries[0].PaymentDeadline, res2.Inquiries[0].PaymentDeadline)
		if assert.Len(t, r.repo.rows, 2+i) {
			cp := r.repo.rows[1+i].Row
			assert.True(t, cp.IsCached)
			assert.Equal(t, user, cp.UserID)
			assert.Equal(t, want, cp.Details, "the copy keeps the details (bill copies extra_data too)")
		}
	}
	assert.Equal(t, 1, r.power.calls, "no second provider call")
	assert.Len(t, r.repo.requests, 1)
	assert.Equal(t, []string{"electricity/tavanir/provider_ok", "electricity/tavanir/db_hit", "electricity/tavanir/db_hit", "electricity/tavanir/db_hit"}, r.inquiry)
}

// TestElectricityNoDetails: an answer with no details has none from the provider and none from the
// database.
func TestElectricityNoDetails(t *testing.T) {
	r := newMultiRig(t, map[string]time.Duration{"electricity": time.Hour}, nil)
	res, err := r.ask("electricity", userA)
	if assert.NoError(t, err) {
		assert.Nil(t, res.Details)
	}
	res, err = r.ask("electricity", userB)
	if assert.NoError(t, err) {
		assert.True(t, res.FromDatabase)
		assert.Nil(t, res.Details)
	}
	if assert.Len(t, r.repo.rows, 2) {
		assert.Nil(t, r.repo.rows[0].Details)
		assert.Nil(t, r.repo.rows[1].Details)
	}
}

// TestSavedRowsDetailsLastWins: with two saved rows the details are those of the last row that has
// any, as the inquiry that the handler sends.
func TestSavedRowsDetailsLastWins(t *testing.T) {
	r := newMultiRig(t, map[string]time.Duration{"electricity": time.Hour}, nil)
	older := &provider.Details{Electricity: &provider.ElectricityDetails{Phase: "older"}}
	newer := &provider.Details{Electricity: &provider.ElectricityDetails{Phase: "newer"}}
	base := Row{UserID: userB, InquiryID: validBill, ProviderID: 2, ProviderName: "tavanir", BillType: "electricity", BillID: validBill}
	a, b, c := base, base, base
	a.ID, a.Details = "a", older
	b.ID, b.Details = "b", newer
	r.repo.seed(a, 20*time.Minute, false)
	r.repo.seed(b, 10*time.Minute, false) // FindValid gives b (newest) first, then a
	res, err := r.ask("electricity", userA)
	if assert.NoError(t, err) && assert.Len(t, res.Inquiries, 2) {
		assert.Same(t, older, res.Details, "the last row of the answer")
	}
	// A last row with no details keeps the details of the row before it.
	r2 := newMultiRig(t, map[string]time.Duration{"electricity": time.Hour}, nil)
	c.ID = "c"
	r2.repo.seed(c, 20*time.Minute, false)
	r2.repo.seed(b, 10*time.Minute, false)
	res, err = r2.ask("electricity", userA)
	if assert.NoError(t, err) {
		assert.Same(t, newer, res.Details)
	}
}

// TestTwoBillTypesOneService: each bill type has its provider, its TTL and its saved rows; a saved
// row of one type does not answer the other.
func TestTwoBillTypesOneService(t *testing.T) {
	r := newMultiRig(t, map[string]time.Duration{"water": time.Hour}, electricityDetails()) // electricity: no cache
	for range 2 {
		res, err := r.ask("electricity", userA)
		if assert.NoError(t, err) {
			assert.False(t, res.FromDatabase, "caching.electricity is 0: the provider answers each call")
		}
	}
	assert.Equal(t, 2, r.power.calls)
	res, err := r.ask("water", userA)
	if assert.NoError(t, err) {
		assert.False(t, res.FromDatabase, "the electricity rows of the same bill ID do not answer water (another provider)")
		assert.Nil(t, res.Details, "water has no details")
	}
	res, err = r.ask("water", userA)
	if assert.NoError(t, err) {
		assert.True(t, res.FromDatabase)
	}
	assert.Equal(t, 1, r.water.calls)
	assert.Equal(t, []string{"electricity", "electricity", "water", "water"}, r.ttlAsked, "the TTL is asked for the bill type of the request")
	assert.Equal(t, []string{"electricity/tavanir/provider_ok", "electricity/tavanir/provider_ok", "water/qabzino/provider_ok", "water/qabzino/db_hit"}, r.inquiry)

	// A bill type with no enabled provider, and one that the flow does not support.
	delete(r.selector.byType, "electricity")
	_, err = r.ask("electricity", userA)
	assert.ErrorIs(t, err, provider.ErrNoProvider)
	if assert.NotEmpty(t, r.inquiry) {
		assert.Equal(t, "electricity/none/no_provider", r.inquiry[len(r.inquiry)-1])
	}
	_, err = r.ask("gas", userA)
	assert.ErrorContains(t, err, `unsupported bill type "gas"`)
}

func TestGenerateTIDElectricity(t *testing.T) {
	for range 20 {
		tid, err := generateTID("electricity")
		if !assert.NoError(t, err) {
			return
		}
		assert.Regexp(t, `^2[0-9]{3,14}$`, tid, "bill: ElectricityReference is 2")
	}
}
