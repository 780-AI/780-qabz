package order

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/inquiry"
	"qabz/provider"
)

const (
	testMCIBill = "9123456780858" // a numeric bill ID (the raw inquiry of the fake does not read it)
	testMCIPay  = "4500012345"
)

// newMCIFixture is a flow for the MCI order (task 054): the order provider Parsian (seed row 48 for
// mobile, 149 for mobilemidterm), the already-paid window 0 (bill: settlementInquiryMCIMobile), the
// order cache TTL of 1h for mobile only (caching.paid_mobile), a raw inquiry of 450 000 rials, and
// a Parsian answer with a mid-term and a final-term inquiry.
func newMCIFixture(t *testing.T, billType string) *fixture {
	t.Helper()
	f := newFixture(t)
	id := int64(48)
	if billType == provider.BillTypeMobileMidterm {
		id = 149
	}
	f.sel.pro = provider.Provider{ID: id, Name: "parsian", BillType: billType, Action: ActionOrder, Status: provider.StatusEnable}
	f.svc.paidWin = map[string]map[string]time.Duration{"parsian": {provider.BillTypeMobile: 0, provider.BillTypeMobileMidterm: 0}}
	f.svc.cacheTTL = func(bt string) time.Duration {
		if bt == provider.BillTypeMobile {
			return time.Hour
		}
		return 0
	}
	f.raw.res, f.raw.err = inquiry.RawResult{BillID: testMCIBill, PayID: testMCIPay, Amount: 450000, BillType: provider.BillTypeMobile}, nil
	f.inq.res = inquiry.Result{BillID: testMCIBill, Inquiries: []inquiry.Inquiry{
		{ID: "mid-row", BillID: testMCIBill, PayID: "1200012345", Amount: 120000, InquiryType: provider.InquiryTypeMidterm},
		{ID: "full-row", BillID: testMCIBill, PayID: "6200012345", Amount: 620000, InquiryType: provider.InquiryTypeFinalTerm},
	}}
	f.repo.feeRow = &InquiryFee{ProviderName: "parsian", ProviderID: 39, BillType: provider.BillTypeMobile}
	return f
}

func mciReq(billType string) Request {
	return Request{BillType: billType, BillID: testMCIBill, PayID: testMCIPay, InquiryID: testPhone, UserID: testUser, Dialer: "989121234567", Option: "mci"}
}

// TestCreateMCIRaw: for both terms a numeric bill ID and pay ID give the price from the raw
// inquiry (bill: no provider fee for MCI): no provider inquiry, no inquiry row; the already-paid
// check has the window 0 (bill: the orders of any time).
func TestCreateMCIRaw(t *testing.T) {
	for _, bt := range []string{provider.BillTypeMobile, provider.BillTypeMobileMidterm} {
		f := newMCIFixture(t, bt)
		o, err := f.svc.Create(context.Background(), mciReq(bt))
		if !assert.NoError(t, err, bt) {
			continue
		}
		assert.Equal(t, []string{bt + "/" + testMCIBill + "/" + testMCIPay}, f.raw.calls, bt)
		assert.Empty(t, f.inq.reqs, bt)
		assert.Empty(t, f.repo.readIDs, bt)
		assert.Equal(t, []string{bt + "/order"}, f.sel.calls, bt)
		assert.Equal(t, []string{testMCIBill + "/" + testMCIPay + "/0s"}, f.repo.lastStatusArgs, bt)
		assert.Equal(t, []any{bt, testMCIPay, int64(450000), uint32(459000), testPhone, "mci", "parsian"},
			[]any{o.BillType, o.PayID, o.BasePrice, o.Price, o.Data, o.Option, o.ProviderName}, bt)
		assert.Equal(t, []string{bt + "/parsian/created"}, f.results, bt)
	}
}

// TestCreateMCIInquiry: a failed raw inquiry goes on to the inquiry of mobile with the phone; a
// full-term order takes the final term with caching.paid_mobile, a mid-term order the mid term
// with no saved inquiry (bill has no TTL for mobilemidterm); a Parsian row has no provider fee.
func TestCreateMCIInquiry(t *testing.T) {
	for bt, want := range map[string][]any{
		provider.BillTypeMobile:        {"full-row", "6200012345", int64(620000), uint32(629000), time.Hour, false},
		provider.BillTypeMobileMidterm: {"mid-row", "1200012345", int64(120000), uint32(129000), time.Duration(0), true},
	} {
		f := newMCIFixture(t, bt)
		f.raw.err = inquiry.ErrInvalidPayID
		o, err := f.svc.Create(context.Background(), mciReq(bt))
		if !assert.NoError(t, err, bt) {
			continue
		}
		if assert.Len(t, f.inq.reqs, 1, bt) {
			r := f.inq.reqs[0]
			assert.Equal(t, provider.BillTypeMobile, r.BillType, bt)
			assert.Equal(t, testPhone, r.InquiryID, bt)
			assert.Equal(t, want[4], r.CacheTTL, bt)
			assert.Equal(t, want[5], r.NoCache, bt)
		}
		assert.Equal(t, want[:4], []any{o.BillInquiryID, o.PayID, o.BasePrice, o.Price}, bt)
		assert.Zero(t, o.ProviderFee, bt)
		assert.Equal(t, []string{bt + "/parsian/created"}, f.results, bt)
	}
}

// TestCreateMCIAlreadyPaid: a pending or successful order of the bill ID and pay ID is
// ErrAlreadyPaid with the window 0; a failed one is not.
func TestCreateMCIAlreadyPaid(t *testing.T) {
	for _, bt := range []string{provider.BillTypeMobile, provider.BillTypeMobileMidterm} {
		for _, last := range []string{StatusPending, StatusSuccess} {
			f := newMCIFixture(t, bt)
			f.repo.lastStatus = last
			_, err := f.svc.Create(context.Background(), mciReq(bt))
			assert.ErrorIs(t, err, ErrAlreadyPaid, bt+last)
			assert.Empty(t, f.raw.calls, bt+last)
			assert.Empty(t, f.repo.inserted, bt+last)
			assert.Equal(t, []string{bt + "/parsian/already_paid"}, f.results, bt+last)
		}
		f := newMCIFixture(t, bt)
		f.repo.lastStatus = StatusFailed
		_, err := f.svc.Create(context.Background(), mciReq(bt))
		assert.NoError(t, err, bt)
	}
}

// TestInquiryTypeOfMCI: bill's BillTypeInquiryType[MOBILE] final_term, [MOBILEMIDTERM] mid term.
func TestInquiryTypeOfMCI(t *testing.T) {
	assert.Equal(t, provider.InquiryTypeFinalTerm, InquiryTypeOf(provider.BillTypeMobile))
	assert.Equal(t, provider.InquiryTypeMidterm, InquiryTypeOf(provider.BillTypeMobileMidterm))
}

// TestPhoneDataMCI: the token keys of bill's Phone.NewOrderV2 with bill's MCI texts, for both terms.
func TestPhoneDataMCI(t *testing.T) {
	for _, bt := range []string{provider.BillTypeMobile, provider.BillTypeMobileMidterm} {
		o := Order{BillType: bt, BillID: testMCIBill, PayID: testMCIPay, Price: 459000, BasePrice: 450000, Fee: 9000}
		assert.Equal(t, map[string]string{
			"type": bt, "phone": testPhone, "service": "mci", "billId": testMCIBill, "payId": testMCIPay,
			"payAmount": "459000", "rawAmount": "450000", "about": "قبض موبایل",
			"aboutIcon":     "https://public.780.ir/dl/Operator/logOhamrahaval2.svg",
			"receiptMobile": "09350000391", "invoiceMobile": "09350000391",
			"feeAmount": "9000", "feeDescription": "کارمزد",
		}, PhoneData(o, testPhone, "mci"), bt)
	}
}

// newMCIPayFixture is the payment step with an MCI order of Parsian (the routes of the App).
func newMCIPayFixture(t *testing.T, billType string) *payFixture {
	t.Helper()
	f := newPayFixture(t)
	f.p.payRoutes = map[string]map[string]bool{"parsian": {provider.BillTypeMobile: true, provider.BillTypeMobileMidterm: true}}
	f.repo.latest = Order{
		ID: "row-1", OrderID: "order-1", UserID: testUser, BillType: billType, BillID: testMCIBill, PayID: testMCIPay,
		Price: 459000, Data: testPhone, Option: "mci", ProviderID: 149, ProviderName: "parsian", Status: StatusInitial, Fee: 9000,
		BasePrice: 450000, CreatedAt: paidAt,
	}
	return f
}

// TestCompleteMCISuccess: the payment request of bill's ParsianMCIMobile (the inquiry type of the
// term, the phone as the inquiry ID, the base price); after the success the side effects run (bill:
// updateCache: is_paid by the inquiry ID, the phone, and the pay ID; last_order_date by the bill ID)
// and nothing is settled.
func TestCompleteMCISuccess(t *testing.T) {
	for bt, it := range map[string]string{provider.BillTypeMobile: provider.InquiryTypeFinalTerm, provider.BillTypeMobileMidterm: provider.InquiryTypeMidterm} {
		f := newMCIPayFixture(t, bt)
		o, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445577", Price: 459000})
		if !assert.NoError(t, err, bt) {
			continue
		}
		assert.Equal(t, StatusSuccess, o.Status, bt)
		assert.Equal(t, []string{StatusInProgress, StatusSuccess}, f.repo.statuses(), bt)
		if assert.Len(t, f.parsian.payReqs, 1, bt) {
			assert.Equal(t, provider.PaymentRequest{
				BillType: bt, InquiryType: it, OrderID: "order-1", UserID: testUser,
				InquiryID: testPhone, BillID: testMCIBill, PayID: testMCIPay, Amount: 450000, RRN: "445577",
			}, f.parsian.payReqs[0], bt)
		}
		assert.Equal(t, []string{testPhone + "/" + testMCIPay}, f.repo.paid, bt, "bill: UpdateIsPaid(inquiry ID, pay ID)")
		assert.Equal(t, []string{testMCIBill + "/" + bt + "/2026-11-15"}, f.repo.dates, bt)
		assert.Empty(t, f.karashop.settles, bt)
		assert.Empty(t, f.settled, bt)
		assert.Equal(t, []string{bt + "/parsian/success"}, f.results, bt)
	}
}

// TestCompleteMCINoRoute: an MCI order of a provider with no route (the Jiring Switch row when it
// is the order provider, until task 058) is saved failed with MessageNoPaymentRoute and no
// provider is called.
func TestCompleteMCINoRoute(t *testing.T) {
	f := newMCIPayFixture(t, provider.BillTypeMobile)
	f.p.payRoutes = map[string]map[string]bool{"parsian": {provider.BillTypeGas: true}}
	_, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445577", Price: 459000})
	assert.ErrorIs(t, err, ErrPaymentRoute)
	assert.Equal(t, []string{StatusInProgress, StatusFailed}, f.repo.statuses())
	assert.Empty(t, f.parsian.payReqs)
}
