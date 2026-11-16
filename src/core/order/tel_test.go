package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/inquiry"
	"qabz/provider"
)

const testTel = "982188776655"

// newTelFixture is a flow for the landline order (task 044): the order provider Parsian, the
// already-paid window of 360h, the order cache TTL of 72h, and a Karashop inquiry answer with both
// terms.
func newTelFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.sel.pro = provider.Provider{ID: 51, Name: "parsian", BillType: provider.BillTypeTelMidterm, Action: ActionOrder, Status: provider.StatusEnable}
	f.svc.paidWin = map[string]map[string]time.Duration{"parsian": {provider.BillTypeTel: 360 * time.Hour, provider.BillTypeTelMidterm: 360 * time.Hour}}
	f.svc.cacheTTL = func(bt string) time.Duration {
		if bt == provider.BillTypeTel || bt == provider.BillTypeTelMidterm {
			return 72 * time.Hour
		}
		return 0
	}
	f.inq.res = inquiry.Result{BillID: "2101234560041", Inquiries: []inquiry.Inquiry{
		{ID: "mid-row", BillID: "2101234560040", PayID: "12000035", Amount: 120000, InquiryType: provider.InquiryTypeMidterm},
		{ID: "full-row", BillID: "2101234560041", PayID: "35000042", Amount: 350000, InquiryType: provider.InquiryTypeFinalTerm},
	}}
	f.repo.feeRow = &InquiryFee{ProviderName: "karashop", ProviderID: 124325, BillType: provider.BillTypeTel, ServiceStatus: provider.TelServiceActive}
	return f
}

func telReq(billType string) Request {
	return Request{BillType: billType, InquiryID: testTel, UserID: testUser, Dialer: "989121234567", BillID: "2101234560040", PayID: "12000035"}
}

// TestCreateTel: the inquiry comes first (also with a numeric bill ID and pay ID: no raw inquiry),
// with the inquiry bill type tel and the order cache TTL; telmidterm takes the mid-term inquiry, tel
// the final-term one; a connected Karashop line adds KarashopProviderFee.
func TestCreateTel(t *testing.T) {
	for billType, want := range map[string]struct {
		row, billID, payID string
		amount             int64
	}{
		provider.BillTypeTelMidterm: {"mid-row", "2101234560040", "12000035", 120000},
		provider.BillTypeTel:        {"full-row", "2101234560041", "35000042", 350000},
	} {
		f := newTelFixture(t)
		o, err := f.svc.Create(context.Background(), telReq(billType))
		if !assert.NoError(t, err, billType) {
			continue
		}
		assert.Empty(t, f.raw.calls, billType+": the inquiry comes first")
		if assert.Len(t, f.inq.reqs, 1, billType) {
			assert.Equal(t, inquiry.Request{BillType: provider.BillTypeTel, InquiryID: testTel, UserID: testUser, Dialer: "989121234567", CacheTTL: 72 * time.Hour}, f.inq.reqs[0], billType)
		}
		assert.Equal(t, want.row, o.BillInquiryID, billType)
		assert.Equal(t, want.billID, o.BillID, billType)
		assert.Equal(t, want.payID, o.PayID, billType)
		assert.Equal(t, want.amount, o.BasePrice, billType)
		assert.Equal(t, uint64(21000), o.ProviderFee, billType)
		assert.Equal(t, uint32(want.amount+9000+21000), o.Price, billType)
		assert.Equal(t, "parsian", o.ProviderName, billType)
		assert.Equal(t, []string{"2101234560040/12000035/360h0m0s"}, f.repo.lastStatusArgs, billType+": the request IDs, the window")
		assert.Equal(t, []string{billType + "/parsian/created"}, f.results, billType)
		assert.Equal(t, []string{want.row}, f.repo.readIDs, billType)
	}
}

// TestCreateTelNoOrderCache: caching.paid_tel 0: no saved inquiry (NoCache).
func TestCreateTelNoOrderCache(t *testing.T) {
	f := newTelFixture(t)
	f.svc.cacheTTL = nil
	_, err := f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
	if assert.NoError(t, err) && assert.Len(t, f.inq.reqs, 1) {
		assert.True(t, f.inq.reqs[0].NoCache)
		assert.Zero(t, f.inq.reqs[0].CacheTTL)
	}
}

// TestCreateTelProviderFee: bill's feeableBillType. Disconnected: providers.provider_fee of the
// inquiry provider (NULL is 0); connected or unknown: 21000; a row of another provider: 0; a
// failed read fails the order.
func TestCreateTelProviderFee(t *testing.T) {
	cases := []struct {
		name    string
		row     InquiryFee
		dbFee   uint64
		want    uint64
		readsDB bool
	}{
		{"active", InquiryFee{ProviderName: "karashop", ProviderID: 9, BillType: "tel", ServiceStatus: "active"}, 5000, 21000, false},
		{"unknown status", InquiryFee{ProviderName: "karashop", ProviderID: 9, BillType: "tel"}, 5000, 21000, false},
		{"partially", InquiryFee{ProviderName: "karashop", ProviderID: 9, BillType: "tel", ServiceStatus: "partiallyDisconnected"}, 5000, 5000, true},
		{"fully, NULL fee", InquiryFee{ProviderName: "karashop", ProviderID: 9, BillType: "telmidterm", ServiceStatus: "fullyDisconnected"}, 0, 0, true},
		{"other provider", InquiryFee{ProviderName: "qabzino", ProviderID: 9, BillType: "tel", ServiceStatus: "active"}, 5000, 0, false},
		{"karashop, other bill type", InquiryFee{ProviderName: "karashop", ProviderID: 9, BillType: "water"}, 5000, 0, false},
	}
	for _, c := range cases {
		f := newTelFixture(t)
		f.repo.feeRow = &c.row
		f.repo.providerFee = c.dbFee
		o, err := f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
		if assert.NoError(t, err, c.name) {
			assert.Equal(t, c.want, o.ProviderFee, c.name)
			assert.Equal(t, uint32(350000+9000+c.want), o.Price, c.name)
		}
		if c.readsDB {
			assert.Equal(t, []int64{9}, f.repo.feeProviderIDs, c.name)
		} else {
			assert.Empty(t, f.repo.feeProviderIDs, c.name)
		}
	}

	f := newTelFixture(t)
	f.repo.feeRow = &InquiryFee{ProviderName: "karashop", ProviderID: 9, BillType: "tel", ServiceStatus: "fullyDisconnected"}
	f.repo.providerFeeErr = errors.New("db down")
	_, err := f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
	assert.ErrorContains(t, err, "provider fee")
	assert.Empty(t, f.repo.inserted)
	assert.Equal(t, []string{"tel/parsian/error"}, f.results)

	f = newTelFixture(t)
	f.repo.readErr = errors.New("no row")
	_, err = f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
	assert.Error(t, err)
	assert.Empty(t, f.repo.inserted)
}

// TestCreateTelAlreadyPaid: a pending or successful last order is ErrAlreadyPaid (no inquiry, no
// row); another status, no row and a failed read (logged) go on; a provider with no window has no
// check.
func TestCreateTelAlreadyPaid(t *testing.T) {
	for _, status := range []string{StatusPending, StatusSuccess} {
		f := newTelFixture(t)
		f.repo.lastStatus = status
		_, err := f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
		assert.ErrorIs(t, err, ErrAlreadyPaid, status)
		assert.Empty(t, f.inq.reqs, status)
		assert.Empty(t, f.repo.inserted, status)
		assert.Equal(t, []string{"tel/parsian/already_paid"}, f.results, status)
	}
	for _, status := range []string{"", StatusInitial, StatusInProgress, StatusFailed} {
		f := newTelFixture(t)
		f.repo.lastStatus = status
		_, err := f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
		assert.NoError(t, err, status)
	}
	f := newTelFixture(t)
	f.repo.lastStatusErr = errors.New("db down")
	_, err := f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
	assert.NoError(t, err)
	assert.Equal(t, []string{"tel: order already-paid check failed"}, f.log.lines)

	f = newTelFixture(t)
	f.sel.pro.Name = "eways2"
	_, err = f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
	assert.NoError(t, err)
	assert.Empty(t, f.repo.lastStatusArgs, "no window for eways2")
}

// TestCreateTelNoInquiryOfType: an answer with no inquiry of the term is ErrNoInquiry; a failed
// inquiry is *InquiryError.
func TestCreateTelNoInquiryOfType(t *testing.T) {
	f := newTelFixture(t)
	f.inq.res.Inquiries = f.inq.res.Inquiries[1:] // only the final term
	_, err := f.svc.Create(context.Background(), telReq(provider.BillTypeTelMidterm))
	assert.ErrorIs(t, err, ErrNoInquiry)

	f = newTelFixture(t)
	f.inq.err = errors.New("down")
	_, err = f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
	var inqErr *InquiryError
	assert.ErrorAs(t, err, &inqErr)
}

func TestInquiryTypeOf(t *testing.T) {
	assert.Equal(t, provider.InquiryTypeFinalTerm, InquiryTypeOf(provider.BillTypeTel))
	assert.Equal(t, provider.InquiryTypeMidterm, InquiryTypeOf(provider.BillTypeTelMidterm))
	assert.Equal(t, provider.InquiryTypeFinalTerm, InquiryTypeOf(provider.BillTypeIrancell))
	assert.Equal(t, provider.InquiryTypeRegular, InquiryTypeOf(provider.BillTypeWater), "task 048: bill RegularInquiryType")
	assert.Empty(t, InquiryTypeOf("gas"))
}

// seqInquirer answers each Inquire call with the next result.
type seqInquirer struct {
	res  []inquiry.Result
	reqs []inquiry.Request
}

func (s *seqInquirer) Inquire(_ context.Context, req inquiry.Request) (inquiry.Result, error) {
	s.reqs = append(s.reqs, req)
	r := s.res[0]
	if len(s.res) > 1 {
		s.res = s.res[1:]
	}
	return r, nil
}

// TestCreateTelSavedAnswerWithoutTheTerm (found in the feature-ON run): the mid-term inquiry was
// paid, so the saved answer has only the final term: the order asks the provider again with no
// cache and takes the mid term of that answer. A saved answer with the term is used as it is.
func TestCreateTelSavedAnswerWithoutTheTerm(t *testing.T) {
	f := newTelFixture(t)
	fresh := f.inq.res
	saved := inquiry.Result{FromDatabase: true, Inquiries: fresh.Inquiries[1:]} // only the final term
	seq := &seqInquirer{res: []inquiry.Result{saved, fresh}}
	f.svc.inquiry = seq
	o, err := f.svc.Create(context.Background(), telReq(provider.BillTypeTelMidterm))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "mid-row", o.BillInquiryID)
	if assert.Len(t, seq.reqs, 2) {
		assert.Equal(t, 72*time.Hour, seq.reqs[0].CacheTTL)
		assert.False(t, seq.reqs[0].NoCache)
		assert.True(t, seq.reqs[1].NoCache, "the second call asks the provider")
		assert.Zero(t, seq.reqs[1].CacheTTL)
	}

	f = newTelFixture(t)
	seq = &seqInquirer{res: []inquiry.Result{saved, fresh}}
	f.svc.inquiry = seq
	o, err = f.svc.Create(context.Background(), telReq(provider.BillTypeTel))
	assert.NoError(t, err)
	assert.Equal(t, "full-row", o.BillInquiryID)
	assert.Len(t, seq.reqs, 1, "the saved answer has the final term")

	// A provider answer with no inquiry of the term is ErrNoInquiry, with no second call.
	f = newTelFixture(t)
	seq = &seqInquirer{res: []inquiry.Result{{Inquiries: fresh.Inquiries[1:]}}}
	f.svc.inquiry = seq
	_, err = f.svc.Create(context.Background(), telReq(provider.BillTypeTelMidterm))
	assert.ErrorIs(t, err, ErrNoInquiry)
	assert.Len(t, seq.reqs, 1)
}
