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
	testGasBill = "1234567890133" // the type digit 3: a gas bill ID
	testGasPay  = "1800012345"
	testGasCode = "0012345678" // a participate code
)

// newGasFixture is a flow for the gas order (task 053): the order provider Parsian (seed row 46),
// the already-paid window of 360h, the order cache TTL of 72h (caching.paid_gas), a raw inquiry
// that gives 180 000 rials, and a Qabzino answer with one regular inquiry.
func newGasFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.sel.pro = provider.Provider{ID: 46, Name: "parsian", BillType: provider.BillTypeGas, Action: ActionOrder, Status: provider.StatusEnable}
	f.svc.paidWin = map[string]map[string]time.Duration{"parsian": {provider.BillTypeGas: 360 * time.Hour}}
	f.svc.cacheTTL = func(bt string) time.Duration {
		if bt == provider.BillTypeGas {
			return 72 * time.Hour
		}
		return 0
	}
	f.raw.res, f.raw.err = inquiry.RawResult{BillID: testGasBill, PayID: testGasPay, Amount: 180000, BillType: provider.BillTypeGas}, nil
	f.inq.res = inquiry.Result{BillID: testGasBill, Inquiries: []inquiry.Inquiry{
		{ID: "gas-row", BillID: testGasBill, PayID: "2100012349", Amount: 210000, InquiryType: provider.InquiryTypeRegular},
	}}
	f.repo.feeRow = &InquiryFee{ProviderName: "qabzino", ProviderID: 35, BillType: provider.BillTypeGas}
	return f
}

func gasReq() Request {
	return Request{BillType: provider.BillTypeGas, BillID: testGasBill, PayID: testGasPay, InquiryID: testGasBill, UserID: testUser, Dialer: "989121234567"}
}

// TestCreateGasRaw: a numeric bill ID and pay ID give the price from the raw inquiry (bill:
// no provider fee for gas, so the raw rule runs first): no provider inquiry, no inquiry row
// read, no provider fee; the fee is added; the already-paid check has the request IDs and the
// window of gas.
func TestCreateGasRaw(t *testing.T) {
	f := newGasFixture(t)
	o, err := f.svc.Create(context.Background(), gasReq())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{"gas/" + testGasBill + "/" + testGasPay}, f.raw.calls)
	assert.Empty(t, f.inq.reqs, "no provider inquiry")
	assert.Empty(t, f.repo.readIDs, "no inquiry row: no provider fee read")
	assert.Equal(t, []string{"gas/order"}, f.sel.calls)
	assert.Equal(t, []string{testGasBill + "/" + testGasPay + "/360h0m0s"}, f.repo.lastStatusArgs)
	assert.Equal(t, Order{
		ID: "00000000-0000-0000-0000-000000000001", OrderID: "00000000-0000-0000-0000-000000000002", UserID: testUser,
		BillType: provider.BillTypeGas, BillID: testGasBill, PayID: testGasPay, Price: 189000, Data: testGasBill,
		ProviderID: 46, ProviderName: "parsian", Status: StatusInitial, Fee: 9000, FeeType: fee.TypeFixed,
		FeeConfig: []byte(`{"amount":9000}`), FeeID: o.FeeID, BasePrice: 180000, FeeSource: fee.SourceProvider,
	}, o)
	assert.Len(t, f.repo.inserted, 1)
	assert.Equal(t, []string{"gas/parsian/created"}, f.results)
}

// TestCreateGasInquiry: a failed raw inquiry goes on to the regular inquiry of gas with the
// participate code as the inquiry ID (bill: SetInquiryID) and the order cache TTL
// (caching.paid_gas); a Qabzino row has no provider fee.
func TestCreateGasInquiry(t *testing.T) {
	f := newGasFixture(t)
	f.raw.err = inquiry.ErrInvalidPayID
	req := gasReq()
	req.InquiryID = testGasCode
	o, err := f.svc.Create(context.Background(), req)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, testGasCode, o.Data, "orders.data is the inquiry ID")
	if assert.Len(t, f.inq.reqs, 1) {
		r := f.inq.reqs[0]
		assert.Equal(t, provider.BillTypeGas, r.BillType)
		assert.Equal(t, provider.InquiryTypeRegular, InquiryTypeOf(r.BillType))
		assert.Equal(t, testGasCode, r.InquiryID)
		assert.Equal(t, 72*time.Hour, r.CacheTTL)
		assert.False(t, r.NoCache)
	}
	assert.Equal(t, []any{"gas-row", "2100012349", int64(210000), uint32(219000), uint64(0)},
		[]any{o.BillInquiryID, o.PayID, o.BasePrice, o.Price, o.ProviderFee})
	assert.Equal(t, []string{"gas/parsian/created"}, f.results)
}

// TestCreateGasAlreadyPaid: a pending or successful order of the bill ID and pay ID in the
// window of gas is ErrAlreadyPaid, with no price step and no row; a failed one is not.
func TestCreateGasAlreadyPaid(t *testing.T) {
	for _, last := range []string{StatusPending, StatusSuccess} {
		f := newGasFixture(t)
		f.repo.lastStatus = last
		_, err := f.svc.Create(context.Background(), gasReq())
		assert.ErrorIs(t, err, ErrAlreadyPaid, last)
		assert.Empty(t, f.raw.calls, last)
		assert.Empty(t, f.repo.inserted, last)
		assert.Equal(t, []string{"gas/parsian/already_paid"}, f.results, last)
	}
	f := newGasFixture(t)
	f.repo.lastStatus = StatusFailed
	_, err := f.svc.Create(context.Background(), gasReq())
	assert.NoError(t, err)
}

// TestInquiryTypeOfGas: the inquiry type of a gas order is regular (bill:
// BillTypeInquiryType[GAS]).
func TestInquiryTypeOfGas(t *testing.T) {
	assert.Equal(t, provider.InquiryTypeRegular, InquiryTypeOf(provider.BillTypeGas))
}

// TestUtilityDataGas: the token keys of bill's Gas.NewOrderV2, with bill's gas texts (no space at
// the start of the about text).
func TestUtilityDataGas(t *testing.T) {
	o := Order{BillType: provider.BillTypeGas, BillID: testGasBill, PayID: testGasPay, Price: 189000, BasePrice: 180000, Fee: 9000}
	assert.Equal(t, map[string]string{
		"payId": testGasPay, "type": "gas", "billId": testGasBill, "payAmount": "189000", "rawAmount": "180000",
		"about": "قبض گاز", "aboutIcon": "https://public.780.ir/dl/Operator/Gas.svg",
		"invoicePayID": testGasPay, "invoiceBillID": testGasBill, "receiptPayID": testGasPay, "receiptBillID": testGasBill,
		"feeAmount": "9000", "feeDescription": "کارمزد",
	}, UtilityData(o))
	o.Fee = 0
	assert.Len(t, UtilityData(o), 11)
}

// newGasPayFixture is the payment step with a gas order of Parsian (the routes of the App).
func newGasPayFixture(t *testing.T) *payFixture {
	t.Helper()
	f := newPayFixture(t)
	f.p.payRoutes = map[string]map[string]bool{"parsian": {
		provider.BillTypeTel: true, provider.BillTypeTelMidterm: true, provider.BillTypeWater: true, provider.BillTypeGas: true,
	}}
	f.repo.latest = Order{
		ID: "row-1", OrderID: "order-1", UserID: testUser, BillType: provider.BillTypeGas, BillID: testGasBill, PayID: testGasPay,
		Price: 189000, Data: testGasBill, ProviderID: 46, ProviderName: "parsian", Status: StatusInitial, Fee: 9000, BasePrice: 180000,
		CreatedAt: paidAt,
	}
	return f
}

// TestCompleteGasSuccess: the payment request of a gas order (inquiry type
// regular, the bill ID as the inquiry ID, the base price); after the success the side effects run
// and nothing is settled (no settlement route for gas).
func TestCompleteGasSuccess(t *testing.T) {
	f := newGasPayFixture(t)
	o, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445577", Price: 189000})
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, StatusSuccess, o.Status)
	assert.Equal(t, []string{StatusInProgress, StatusSuccess}, f.repo.statuses())
	if assert.Len(t, f.parsian.payReqs, 1) {
		assert.Equal(t, provider.PaymentRequest{
			BillType: provider.BillTypeGas, InquiryType: provider.InquiryTypeRegular, OrderID: "order-1", UserID: testUser,
			InquiryID: testGasBill, BillID: testGasBill, PayID: testGasPay, Amount: 180000, RRN: "445577",
		}, f.parsian.payReqs[0])
	}
	assert.Equal(t, []string{testGasBill + "/" + testGasPay}, f.repo.paid)
	assert.Equal(t, []string{testGasBill + "/gas/2026-11-15"}, f.repo.dates)
	assert.Empty(t, f.karashop.settles, "no settlement for gas")
	assert.Empty(t, f.settled, "nothing counted")
	assert.Equal(t, []string{"gas/parsian/success"}, f.results)
}

// TestCompleteGasNoRoute: without the gas route (the routes of task 052) the order
// is saved failed with MessageNoPaymentRoute and Parsian is not called (positive control of the
// route).
func TestCompleteGasNoRoute(t *testing.T) {
	f := newGasPayFixture(t)
	f.p.payRoutes = map[string]map[string]bool{"parsian": {provider.BillTypeTel: true, provider.BillTypeTelMidterm: true, provider.BillTypeWater: true, provider.BillTypeElectricity: true}}
	_, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445577", Price: 189000})
	assert.ErrorIs(t, err, ErrPaymentRoute)
	assert.Equal(t, []string{StatusInProgress, StatusFailed}, f.repo.statuses())
	assert.Empty(t, f.parsian.payReqs)
}
