package order

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/core/fee"
	"qabz/core/inquiry"
	"qabz/provider"
)

const (
	testElecBill = "1234567890125" // the type digit 2: an electricity bill ID
	testElecPay  = "1800012345"
)

// newElectricityFixture is a flow for the electricity order (task 052): the order provider Parsian
// (seed row 47), the already-paid window of 360h, the order cache TTL of 72h
// (caching.paid_electricity), a raw inquiry that gives 180 000 rials, and a Tavanir answer with one
// regular inquiry.
func newElectricityFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.sel.pro = provider.Provider{ID: 47, Name: "parsian", BillType: provider.BillTypeElectricity, Action: ActionOrder, Status: provider.StatusEnable}
	f.svc.paidWin = map[string]map[string]time.Duration{"parsian": {provider.BillTypeElectricity: 360 * time.Hour}}
	f.svc.cacheTTL = func(bt string) time.Duration {
		if bt == provider.BillTypeElectricity {
			return 72 * time.Hour
		}
		return 0
	}
	f.raw.res, f.raw.err = inquiry.RawResult{BillID: testElecBill, PayID: testElecPay, Amount: 180000, BillType: provider.BillTypeElectricity}, nil
	f.inq.res = inquiry.Result{BillID: testElecBill, Inquiries: []inquiry.Inquiry{
		{ID: "elec-row", BillID: testElecBill, PayID: "2100012349", Amount: 210000, InquiryType: provider.InquiryTypeRegular},
	}}
	f.repo.feeRow = &InquiryFee{ProviderName: "tavanir", ProviderID: 2, BillType: provider.BillTypeElectricity}
	return f
}

func electricityReq() Request {
	return Request{BillType: provider.BillTypeElectricity, BillID: testElecBill, PayID: testElecPay, InquiryID: testElecBill, UserID: testUser, Dialer: "989121234567"}
}

// TestCreateElectricityRaw: a numeric bill ID and pay ID give the price from the raw inquiry (bill:
// no provider fee for electricity, so the raw rule runs first): no provider inquiry, no inquiry row
// read, no provider fee; the fee is added; the already-paid check has the request IDs and the
// window of electricity.
func TestCreateElectricityRaw(t *testing.T) {
	f := newElectricityFixture(t)
	o, err := f.svc.Create(context.Background(), electricityReq())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{"electricity/" + testElecBill + "/" + testElecPay}, f.raw.calls)
	assert.Empty(t, f.inq.reqs, "no provider inquiry")
	assert.Empty(t, f.repo.readIDs, "no inquiry row: no provider fee read")
	assert.Equal(t, []string{"electricity/order"}, f.sel.calls)
	assert.Equal(t, []string{testElecBill + "/" + testElecPay + "/360h0m0s"}, f.repo.lastStatusArgs)
	assert.Equal(t, Order{
		ID: "00000000-0000-0000-0000-000000000001", OrderID: "00000000-0000-0000-0000-000000000002", UserID: testUser,
		BillType: provider.BillTypeElectricity, BillID: testElecBill, PayID: testElecPay, Price: 189000, Data: testElecBill,
		ProviderID: 47, ProviderName: "parsian", Status: StatusInitial, Fee: 9000, FeeType: fee.TypeFixed,
		FeeConfig: []byte(`{"amount":9000}`), FeeID: o.FeeID, BasePrice: 180000, FeeSource: fee.SourceProvider,
	}, o)
	assert.Len(t, f.repo.inserted, 1)
	assert.Equal(t, []string{"electricity/parsian/created"}, f.results)
}

// TestCreateElectricityInquiry: a failed raw inquiry goes on to the regular inquiry of electricity
// (Tavanir in the seed) with the order cache TTL (caching.paid_electricity); a Tavanir row has no
// provider fee.
func TestCreateElectricityInquiry(t *testing.T) {
	f := newElectricityFixture(t)
	f.raw.err = inquiry.ErrInvalidPayID
	o, err := f.svc.Create(context.Background(), electricityReq())
	if !assert.NoError(t, err) {
		return
	}
	if assert.Len(t, f.inq.reqs, 1) {
		r := f.inq.reqs[0]
		assert.Equal(t, provider.BillTypeElectricity, r.BillType)
		assert.Equal(t, testElecBill, r.InquiryID)
		assert.Equal(t, 72*time.Hour, r.CacheTTL)
		assert.False(t, r.NoCache)
	}
	assert.Equal(t, []any{"elec-row", "2100012349", int64(210000), uint32(219000), uint64(0)},
		[]any{o.BillInquiryID, o.PayID, o.BasePrice, o.Price, o.ProviderFee})
	assert.Equal(t, []string{"electricity/parsian/created"}, f.results)
}

// TestCreateElectricityAlreadyPaid: a pending or successful order of the bill ID and pay ID in the
// window of electricity is ErrAlreadyPaid, with no price step and no row; a failed one is not.
func TestCreateElectricityAlreadyPaid(t *testing.T) {
	for _, last := range []string{StatusPending, StatusSuccess} {
		f := newElectricityFixture(t)
		f.repo.lastStatus = last
		_, err := f.svc.Create(context.Background(), electricityReq())
		assert.ErrorIs(t, err, ErrAlreadyPaid, last)
		assert.Empty(t, f.raw.calls, last)
		assert.Empty(t, f.repo.inserted, last)
		assert.Equal(t, []string{"electricity/parsian/already_paid"}, f.results, last)
	}
	f := newElectricityFixture(t)
	f.repo.lastStatus = StatusFailed
	_, err := f.svc.Create(context.Background(), electricityReq())
	assert.NoError(t, err)
}

// TestInquiryTypeOfElectricity: the inquiry type of an electricity order is regular (bill:
// BillTypeInquiryType[ELECTRICITY]).
func TestInquiryTypeOfElectricity(t *testing.T) {
	assert.Equal(t, provider.InquiryTypeRegular, InquiryTypeOf(provider.BillTypeElectricity))
}

// TestUtilityDataElectricity: the token keys of bill's Electricity.NewOrderV2, with the texts of
// bill: the about text starts with a space (operator decision D7 (a)).
func TestUtilityDataElectricity(t *testing.T) {
	o := Order{BillType: provider.BillTypeElectricity, BillID: testElecBill, PayID: testElecPay, Price: 189000, BasePrice: 180000, Fee: 9000}
	assert.Equal(t, map[string]string{
		"payId": testElecPay, "type": "electricity", "billId": testElecBill, "payAmount": "189000", "rawAmount": "180000",
		"about": " قبض برق", "aboutIcon": "https://public.780.ir/dl/Operator/Bargh.svg",
		"invoicePayID": testElecPay, "invoiceBillID": testElecBill, "receiptPayID": testElecPay, "receiptBillID": testElecBill,
		"feeAmount": "9000", "feeDescription": "کارمزد",
	}, UtilityData(o))
	o.Fee = 0
	assert.Len(t, UtilityData(o), 11)
}

// newElectricityPayFixture is the payment step with an electricity order of Parsian (the routes of
// the App).
func newElectricityPayFixture(t *testing.T) *payFixture {
	t.Helper()
	f := newPayFixture(t)
	f.p.payRoutes = map[string]map[string]bool{"parsian": {
		provider.BillTypeTel: true, provider.BillTypeTelMidterm: true, provider.BillTypeWater: true, provider.BillTypeElectricity: true,
	}}
	f.repo.latest = Order{
		ID: "row-1", OrderID: "order-1", UserID: testUser, BillType: provider.BillTypeElectricity, BillID: testElecBill, PayID: testElecPay,
		Price: 189000, Data: testElecBill, ProviderID: 47, ProviderName: "parsian", Status: StatusInitial, Fee: 9000, BasePrice: 180000,
		CreatedAt: paidAt,
	}
	return f
}

// TestCompleteElectricitySuccess: the payment request of an electricity order (inquiry type
// regular, the bill ID as the inquiry ID, the base price); after the success the side effects run
// and nothing is settled (no settlement route for electricity).
func TestCompleteElectricitySuccess(t *testing.T) {
	f := newElectricityPayFixture(t)
	o, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445577", Price: 189000})
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, StatusSuccess, o.Status)
	assert.Equal(t, []string{StatusInProgress, StatusSuccess}, f.repo.statuses())
	if assert.Len(t, f.parsian.payReqs, 1) {
		assert.Equal(t, provider.PaymentRequest{
			BillType: provider.BillTypeElectricity, InquiryType: provider.InquiryTypeRegular, OrderID: "order-1", UserID: testUser,
			InquiryID: testElecBill, BillID: testElecBill, PayID: testElecPay, Amount: 180000, RRN: "445577",
		}, f.parsian.payReqs[0])
	}
	assert.Equal(t, []string{testElecBill + "/" + testElecPay}, f.repo.paid)
	assert.Equal(t, []string{testElecBill + "/electricity/2026-11-15"}, f.repo.dates)
	assert.Empty(t, f.karashop.settles, "no settlement for electricity")
	assert.Empty(t, f.settled, "nothing counted")
	assert.Equal(t, []string{"electricity/parsian/success"}, f.results)
}

// TestCompleteElectricityNoRoute: without the electricity route (the routes of task 048) the order
// is saved failed with MessageNoPaymentRoute and Parsian is not called (positive control of the
// route).
func TestCompleteElectricityNoRoute(t *testing.T) {
	f := newElectricityPayFixture(t)
	f.p.payRoutes = map[string]map[string]bool{"parsian": {provider.BillTypeTel: true, provider.BillTypeTelMidterm: true, provider.BillTypeWater: true}}
	_, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445577", Price: 189000})
	assert.ErrorIs(t, err, ErrPaymentRoute)
	assert.Equal(t, []string{StatusInProgress, StatusFailed}, f.repo.statuses())
	assert.Empty(t, f.parsian.payReqs)
}
