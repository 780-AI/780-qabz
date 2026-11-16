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
	testRightelBill  = "9201234560817" // a numeric bill ID (the raw inquiry of the fake does not read it)
	testRightelPay   = "2300045678"
	testRightelPhone = "989201234567"
	rtBillType       = provider.BillTypeMobileMidTermRightel
)

// newRightelFixture is a flow for the Rightel order (task 055): the order provider Parsian (seed
// row 500), the already-paid window 360h (bill: settlementInquiryRightelMobile), the order cache TTL
// of 1h (caching.paid_mobile, as bill), a raw inquiry of 230 000 rials, and a Qabzino answer with a
// mid-term and a final-term inquiry.
func newRightelFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.sel.pro = provider.Provider{ID: 500, Name: "parsian", BillType: rtBillType, Action: ActionOrder, Status: provider.StatusEnable}
	f.svc.paidWin = map[string]map[string]time.Duration{"parsian": {rtBillType: 360 * time.Hour}}
	f.svc.cacheTTL = func(bt string) time.Duration {
		if bt == rtBillType {
			return time.Hour
		}
		return 0
	}
	f.raw.res, f.raw.err = inquiry.RawResult{BillID: testRightelBill, PayID: testRightelPay, Amount: 230000, BillType: rtBillType}, nil
	f.inq.res = inquiry.Result{BillID: testRightelBill, Inquiries: []inquiry.Inquiry{
		{ID: "mid-row", BillID: testRightelBill, PayID: "1100045678", Amount: 110000, InquiryType: provider.InquiryTypeMidterm},
		{ID: "full-row", BillID: testRightelBill, PayID: "3300045678", Amount: 330000, InquiryType: provider.InquiryTypeFinalTerm},
	}}
	f.repo.feeRow = &InquiryFee{ProviderName: "qabzino", ProviderID: 34, BillType: rtBillType}
	return f
}

func rightelReq() Request {
	return Request{BillType: rtBillType, BillID: testRightelBill, PayID: testRightelPay, InquiryID: testRightelPhone, UserID: testUser,
		Dialer: "989121234567", Option: "rightel"}
}

// TestCreateRightelRaw: a numeric bill ID and pay ID give the price from the raw inquiry (bill: no
// provider fee for Rightel): no provider inquiry, no inquiry row; the already-paid check has the
// window 360h.
func TestCreateRightelRaw(t *testing.T) {
	f := newRightelFixture(t)
	o, err := f.svc.Create(context.Background(), rightelReq())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{rtBillType + "/" + testRightelBill + "/" + testRightelPay}, f.raw.calls)
	assert.Empty(t, f.inq.reqs)
	assert.Empty(t, f.repo.readIDs)
	assert.Equal(t, []string{rtBillType + "/order"}, f.sel.calls)
	assert.Equal(t, []string{testRightelBill + "/" + testRightelPay + "/360h0m0s"}, f.repo.lastStatusArgs)
	assert.Equal(t, []any{rtBillType, testRightelPay, int64(230000), uint32(239000), testRightelPhone, "rightel", "parsian"},
		[]any{o.BillType, o.PayID, o.BasePrice, o.Price, o.Data, o.Option, o.ProviderName})
	assert.Equal(t, []string{rtBillType + "/parsian/created"}, f.results)
}

// TestCreateRightelInquiry: a failed raw inquiry goes on to the inquiry of mobilemidtermRightel
// itself (bill: the inquiry of the order bill type) with the phone and caching.paid_mobile; the
// mid-term inquiry is the price; the row of the inquiry (Qabzino) has no provider fee.
func TestCreateRightelInquiry(t *testing.T) {
	f := newRightelFixture(t)
	f.raw.err = inquiry.ErrInvalidPayID
	o, err := f.svc.Create(context.Background(), rightelReq())
	if !assert.NoError(t, err) {
		return
	}
	if assert.Len(t, f.inq.reqs, 1) {
		r := f.inq.reqs[0]
		assert.Equal(t, []any{rtBillType, testRightelPhone, time.Hour, false}, []any{r.BillType, r.InquiryID, r.CacheTTL, r.NoCache})
	}
	assert.Equal(t, []any{"mid-row", "1100045678", int64(110000), uint32(119000)}, []any{o.BillInquiryID, o.PayID, o.BasePrice, o.Price})
	assert.Zero(t, o.ProviderFee)
	assert.Equal(t, []string{rtBillType + "/parsian/created"}, f.results)
}

// TestCreateRightelAlreadyPaid: a pending or successful order of the bill ID and pay ID in the
// window is ErrAlreadyPaid; a failed one is not.
func TestCreateRightelAlreadyPaid(t *testing.T) {
	for _, last := range []string{StatusPending, StatusSuccess} {
		f := newRightelFixture(t)
		f.repo.lastStatus = last
		_, err := f.svc.Create(context.Background(), rightelReq())
		assert.ErrorIs(t, err, ErrAlreadyPaid, last)
		assert.Empty(t, f.raw.calls, last)
		assert.Empty(t, f.repo.inserted, last)
		assert.Equal(t, []string{rtBillType + "/parsian/already_paid"}, f.results, last)
	}
	f := newRightelFixture(t)
	f.repo.lastStatus = StatusFailed
	_, err := f.svc.Create(context.Background(), rightelReq())
	assert.NoError(t, err)
}

// TestInquiryTypeOfRightel: bill's BillTypeInquiryType[MOBILEMIDTERMRIGHTEL] is the mid term.
func TestInquiryTypeOfRightel(t *testing.T) {
	assert.Equal(t, provider.InquiryTypeMidterm, InquiryTypeOf(rtBillType))
}

// TestPhoneDataRightel: the token keys of bill's Phone.NewOrderV2 with bill's Rightel texts.
func TestPhoneDataRightel(t *testing.T) {
	o := Order{BillType: rtBillType, BillID: testRightelBill, PayID: testRightelPay, Price: 239000, BasePrice: 230000, Fee: 9000}
	assert.Equal(t, map[string]string{
		"type": rtBillType, "phone": testRightelPhone, "service": "rightel", "billId": testRightelBill, "payId": testRightelPay,
		"payAmount": "239000", "rawAmount": "230000", "about": "قبض موبایل",
		"aboutIcon":     "https://public.780.ir/dl/Operator/logOrhightel2.svg",
		"receiptMobile": "09201234567", "invoiceMobile": "09201234567",
		"feeAmount": "9000", "feeDescription": "کارمزد",
	}, PhoneData(o, testRightelPhone, "rightel"))
}

// newRightelPayFixture is the payment step with a Rightel order of Parsian (the route of the App).
func newRightelPayFixture(t *testing.T) *payFixture {
	t.Helper()
	f := newPayFixture(t)
	f.p.payRoutes = map[string]map[string]bool{"parsian": {rtBillType: true}}
	f.repo.latest = Order{
		ID: "row-1", OrderID: "order-1", UserID: testUser, BillType: rtBillType, BillID: testRightelBill, PayID: testRightelPay,
		Price: 239000, Data: testRightelPhone, Option: "rightel", ProviderID: 500, ProviderName: "parsian", Status: StatusInitial, Fee: 9000,
		BasePrice: 230000, CreatedAt: paidAt,
	}
	return f
}

// TestCompleteRightelSuccess: the payment request of bill's ParsianRightelMobile (the inquiry type
// midterm, the phone as the inquiry ID, the base price); after the success the side effects run
// (bill: updateCache) and nothing is settled.
func TestCompleteRightelSuccess(t *testing.T) {
	f := newRightelPayFixture(t)
	o, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445580", Price: 239000})
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, StatusSuccess, o.Status)
	assert.Equal(t, []string{StatusInProgress, StatusSuccess}, f.repo.statuses())
	if assert.Len(t, f.parsian.payReqs, 1) {
		assert.Equal(t, provider.PaymentRequest{
			BillType: rtBillType, InquiryType: provider.InquiryTypeMidterm, OrderID: "order-1", UserID: testUser,
			InquiryID: testRightelPhone, BillID: testRightelBill, PayID: testRightelPay, Amount: 230000, RRN: "445580",
		}, f.parsian.payReqs[0])
	}
	assert.Equal(t, []string{testRightelPhone + "/" + testRightelPay}, f.repo.paid, "bill: UpdateIsPaid(inquiry ID, pay ID)")
	assert.Equal(t, []string{testRightelBill + "/" + rtBillType + "/2026-11-15"}, f.repo.dates)
	assert.Empty(t, f.karashop.settles)
	assert.Empty(t, f.settled)
	assert.Equal(t, []string{rtBillType + "/parsian/success"}, f.results)
}

// TestCompleteRightelNoRoute: a Rightel order of a provider with no route (the eways2 row when it
// is the order provider; Qabz has no EWays adapter) is saved failed with no provider call. The fake
// repository names every row other than the Parsian rows "other", a provider with no route.
func TestCompleteRightelNoRoute(t *testing.T) {
	f := newRightelPayFixture(t)
	f.repo.latest.ProviderName, f.repo.latest.ProviderID = "eways2", 37
	_, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445580", Price: 239000})
	assert.ErrorIs(t, err, ErrPaymentRoute)
	assert.Equal(t, []string{StatusInProgress, StatusFailed}, f.repo.statuses())
	assert.Empty(t, f.parsian.payReqs)
}
