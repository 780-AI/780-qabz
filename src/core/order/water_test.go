package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/core/fee"
	"qabz/core/inquiry"
	"qabz/provider"
)

const (
	testWaterBill = "1234567890125"
	testWaterPay  = "2500012340"
)

// newWaterFixture is a flow for the water order (task 048): the order provider Parsian (seed row
// 45), the already-paid window of 360h, the order cache TTL of 72h (caching.paid_water), a raw
// inquiry that gives 250 000 rials, and a provider inquiry answer with one regular inquiry.
func newWaterFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.sel.pro = provider.Provider{ID: 45, Name: "parsian", BillType: provider.BillTypeWater, Action: ActionOrder, Status: provider.StatusEnable}
	f.svc.paidWin = map[string]map[string]time.Duration{"parsian": {provider.BillTypeWater: 360 * time.Hour}}
	f.svc.cacheTTL = func(bt string) time.Duration {
		if bt == provider.BillTypeWater {
			return 72 * time.Hour
		}
		return 0
	}
	f.raw.res, f.raw.err = inquiry.RawResult{BillID: testWaterBill, PayID: testWaterPay, Amount: 250000, BillType: provider.BillTypeWater}, nil
	f.inq.res = inquiry.Result{BillID: testWaterBill, Inquiries: []inquiry.Inquiry{
		{ID: "water-row", BillID: testWaterBill, PayID: "3100012342", Amount: 310000, InquiryType: provider.InquiryTypeRegular},
	}}
	f.repo.feeRow = &InquiryFee{ProviderName: "qabzino", ProviderID: 36, BillType: provider.BillTypeWater}
	return f
}

func waterReq() Request {
	return Request{BillType: provider.BillTypeWater, BillID: testWaterBill, PayID: testWaterPay, InquiryID: testWaterBill, UserID: testUser, Dialer: "989121234567"}
}

// TestCreateWaterRaw: a numeric bill ID and pay ID give the price from the raw inquiry (bill: water
// has no provider fee, so the raw rule runs first): no provider inquiry, no inquiry row read, no
// provider fee; the fee is added; the already-paid check has the request IDs and the window.
func TestCreateWaterRaw(t *testing.T) {
	f := newWaterFixture(t)
	o, err := f.svc.Create(context.Background(), waterReq())
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, []string{"water/" + testWaterBill + "/" + testWaterPay}, f.raw.calls)
	assert.Empty(t, f.inq.reqs, "no provider inquiry")
	assert.Empty(t, f.repo.readIDs, "no inquiry row: no provider fee read")
	assert.Equal(t, []string{"water/order"}, f.sel.calls)
	assert.Equal(t, []string{testWaterBill + "/" + testWaterPay + "/360h0m0s"}, f.repo.lastStatusArgs)
	assert.Equal(t, Order{
		ID: "00000000-0000-0000-0000-000000000001", OrderID: "00000000-0000-0000-0000-000000000002", UserID: testUser,
		BillType: provider.BillTypeWater, BillID: testWaterBill, PayID: testWaterPay, Price: 259000, Data: testWaterBill,
		ProviderID: 45, ProviderName: "parsian", Status: StatusInitial, Fee: 9000, FeeType: fee.TypeFixed,
		FeeConfig: []byte(`{"amount":9000}`), FeeID: o.FeeID, BasePrice: 250000, FeeSource: fee.SourceProvider,
	}, o)
	assert.Len(t, f.repo.inserted, 1)
	assert.Equal(t, []string{"water/parsian/created"}, f.results)
}

// TestCreateWaterInquiry: a failed raw inquiry goes on to the regular inquiry of water with the
// order cache TTL (caching.paid_water); the row of a provider other than Karashop has no provider
// fee.
func TestCreateWaterInquiry(t *testing.T) {
	f := newWaterFixture(t)
	f.raw.err = inquiry.ErrInvalidPayID
	o, err := f.svc.Create(context.Background(), waterReq())
	if !assert.NoError(t, err) {
		return
	}
	if assert.Len(t, f.inq.reqs, 1) {
		assert.Equal(t, inquiry.Request{BillType: provider.BillTypeWater, InquiryID: testWaterBill, UserID: testUser, Dialer: "989121234567", CacheTTL: 72 * time.Hour}, f.inq.reqs[0])
	}
	assert.Equal(t, "water-row", o.BillInquiryID)
	assert.Equal(t, "3100012342", o.PayID)
	assert.Equal(t, int64(310000), o.BasePrice)
	assert.Zero(t, o.ProviderFee)
	assert.Equal(t, uint32(319000), o.Price)
	assert.Equal(t, []string{"water-row"}, f.repo.readIDs)

	// An answer with no regular inquiry is ErrNoInquiry.
	f = newWaterFixture(t)
	f.raw.err = inquiry.ErrInvalidPayID
	f.inq.res.Inquiries[0].InquiryType = provider.InquiryTypeMidterm
	_, err = f.svc.Create(context.Background(), waterReq())
	assert.ErrorIs(t, err, ErrNoInquiry)
	assert.Equal(t, []string{"water/parsian/inquiry_failed"}, f.results)

	// A non-numeric pay ID skips the raw inquiry (bill: shouldInquiryWithPayID).
	f = newWaterFixture(t)
	req := waterReq()
	req.PayID = ""
	_, err = f.svc.Create(context.Background(), req)
	assert.NoError(t, err)
	assert.Empty(t, f.raw.calls)
	assert.Len(t, f.inq.reqs, 1)
}

// TestCreateWaterAlreadyPaid: a pending or successful order of the bill ID and pay ID in the window
// stops the order (bill: shouldSkipNewOrder); another status or a failed read goes on.
func TestCreateWaterAlreadyPaid(t *testing.T) {
	for status, paid := range map[string]bool{StatusPending: true, StatusSuccess: true, StatusFailed: false, StatusInitial: false, "": false} {
		f := newWaterFixture(t)
		f.repo.lastStatus = status
		_, err := f.svc.Create(context.Background(), waterReq())
		if paid {
			assert.ErrorIs(t, err, ErrAlreadyPaid, status)
			assert.Equal(t, []string{"water/parsian/already_paid"}, f.results, status)
			assert.Empty(t, f.raw.calls, status)
		} else {
			assert.NoError(t, err, status)
		}
	}
	f := newWaterFixture(t)
	f.repo.lastStatusErr = errors.New("db down")
	_, err := f.svc.Create(context.Background(), waterReq())
	assert.NoError(t, err, "a failed read does not stop the order")
	assert.Equal(t, []string{"water: order already-paid check failed"}, f.log.lines)
}

// TestUtilityData: the token keys of bill's Water.NewOrderV2 for a water order, with and without a fee.
func TestUtilityData(t *testing.T) {
	o := Order{BillType: provider.BillTypeWater, BillID: testWaterBill, PayID: testWaterPay, Price: 259000, BasePrice: 250000, Fee: 9000}
	assert.Equal(t, map[string]string{
		"payId": testWaterPay, "type": "water", "billId": testWaterBill, "payAmount": "259000", "rawAmount": "250000",
		"about": "قبض آب", "aboutIcon": "https://public.780.ir/dl/Operator/AbvaFazelab.svg",
		"invoicePayID": testWaterPay, "invoiceBillID": testWaterBill, "receiptPayID": testWaterPay, "receiptBillID": testWaterBill,
		"feeAmount": "9000", "feeDescription": "کارمزد",
	}, UtilityData(o))
	o.Fee = 0
	noFee := UtilityData(o)
	assert.Len(t, noFee, 11)
	assert.NotContains(t, noFee, "feeAmount")
	assert.NotContains(t, noFee, "feeDescription")
}

// newWaterPayFixture is the payment step with a water order of Parsian (the routes of the App).
func newWaterPayFixture(t *testing.T) *payFixture {
	t.Helper()
	f := newPayFixture(t)
	f.p.payRoutes = map[string]map[string]bool{"parsian": {provider.BillTypeTel: true, provider.BillTypeTelMidterm: true, provider.BillTypeWater: true}}
	f.repo.latest = Order{
		ID: "row-1", OrderID: "order-1", UserID: testUser, BillType: provider.BillTypeWater, BillID: testWaterBill, PayID: testWaterPay,
		Price: 259000, Data: testWaterBill, ProviderID: 45, ProviderName: "parsian", Status: StatusInitial, Fee: 9000, BasePrice: 250000,
		CreatedAt: paidAt,
	}
	return f
}

// TestCompleteWaterSuccess: the payment request of a water order (inquiry type regular, the bill ID
// as the inquiry ID, the base price); after the success the side effects run and nothing is
// settled (no settlement route for water).
func TestCompleteWaterSuccess(t *testing.T) {
	f := newWaterPayFixture(t)
	o, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445566", Price: 259000})
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, StatusSuccess, o.Status)
	assert.Equal(t, []string{StatusInProgress, StatusSuccess}, f.repo.statuses())
	if assert.Len(t, f.parsian.payReqs, 1) {
		assert.Equal(t, provider.PaymentRequest{
			BillType: provider.BillTypeWater, InquiryType: provider.InquiryTypeRegular, OrderID: "order-1", UserID: testUser,
			InquiryID: testWaterBill, BillID: testWaterBill, PayID: testWaterPay, Amount: 250000, RRN: "445566",
		}, f.parsian.payReqs[0])
	}
	assert.Equal(t, []string{testWaterBill + "/" + testWaterPay}, f.repo.paid)
	assert.Equal(t, []string{testWaterBill + "/water/2026-11-15"}, f.repo.dates)
	assert.Empty(t, f.karashop.settles, "no settlement for water")
	assert.Empty(t, f.repo.confirmed)
	assert.Empty(t, f.settled, "nothing counted")
	assert.Equal(t, []string{"water/parsian/success"}, f.results)
}

// TestCompleteWaterNoRoute: without the water route (the routes of task 044) the order is saved
// failed with MessageNoPaymentRoute and Parsian is not called (positive control of the route).
func TestCompleteWaterNoRoute(t *testing.T) {
	f := newWaterPayFixture(t)
	f.p.payRoutes = map[string]map[string]bool{"parsian": {provider.BillTypeTel: true, provider.BillTypeTelMidterm: true}}
	_, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445566", Price: 259000})
	assert.ErrorIs(t, err, ErrPaymentRoute)
	assert.Equal(t, []string{StatusInProgress, StatusFailed}, f.repo.statuses())
	assert.Empty(t, f.parsian.payReqs)
}

// TestCompleteFailedOrderAnswersFailed (task 048 D11 (a)): a second call for an order whose newest
// row is failed answers failed with the message of that row (MessageProcessed when it has none),
// with no row and no provider call, for every bill type.
func TestCompleteFailedOrderAnswersFailed(t *testing.T) {
	for _, billType := range []string{provider.BillTypeWater, provider.BillTypeElectricity, provider.BillTypeGas, provider.BillTypeTel, provider.BillTypeTelMidterm} {
		for message, want := range map[string]string{"mock: the payment failed": "mock: the payment failed", "": MessageProcessed} {
			f := newWaterPayFixture(t)
			f.repo.latest.BillType = billType
			f.repo.latest.Status, f.repo.latest.Message, f.repo.latest.StatusCode = StatusFailed, message, 400
			o, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445566", Price: 259000})
			if assert.NoError(t, err, billType) {
				assert.Equal(t, StatusFailed, o.Status, billType)
				assert.Equal(t, want, o.Message, billType)
				assert.Equal(t, "order-1", o.OrderID, billType)
				assert.Equal(t, billType, o.BillType, billType)
				assert.Equal(t, "parsian", o.ProviderName, billType)
			}
			assert.Empty(t, f.repo.inserts, billType)
			assert.Empty(t, f.parsian.payReqs, billType)
			assert.Empty(t, f.repo.paid, billType)
			assert.Equal(t, []string{billType + "/parsian/processed"}, f.results, billType)
		}
	}
	// A step before the payment failed (case 2): the second call gets the error of the first call
	// (the general text in the handler), never the internal message of the row (conventions § 10).
	for message, want := range map[string]error{MessageNoProviderRow: ErrPaymentDatabase, MessageInsertFailed: ErrPaymentDatabase, MessageNoPaymentRoute: ErrPaymentRoute} {
		f := newWaterPayFixture(t)
		f.repo.latest.Status, f.repo.latest.Message, f.repo.latest.StatusCode = StatusFailed, message, 400
		o, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445566", Price: 259000})
		assert.ErrorIs(t, err, want, message)
		assert.Equal(t, Order{}, o, message)
		assert.Empty(t, f.repo.inserts, message)
		assert.Empty(t, f.parsian.payReqs, message)
		assert.Equal(t, []string{"water/parsian/processed"}, f.results, message)
	}
	// A wrong price of a failed order is still failed: the status check comes first.
	f := newWaterPayFixture(t)
	f.repo.latest.Status = StatusFailed
	o, err := f.p.Complete(context.Background(), CompleteRequest{OrderID: "order-1", RRN: "445566", Price: 1})
	if assert.NoError(t, err) {
		assert.Equal(t, StatusFailed, o.Status)
	}
}

// TestCreateSpan (conventions § 17): one span order.Service.Create, a child of the span in ctx, with
// the bill type, the provider and the result; the status Error on a failure; no bill ID, pay ID or
// user in a span.
func TestCreateSpan(t *testing.T) {
	sr := jaegertest.Install(t)
	f := newWaterFixture(t)
	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	_, err := f.svc.Create(ctx, waterReq())
	parent.End()
	assert.NoError(t, err)
	spans := jaegertest.ByName(sr, createSpan)
	if assert.Len(t, spans, 1) {
		assert.Equal(t, parent.SpanContext().SpanID(), spans[0].Parent().SpanID())
		assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("bill_type", "water"), attribute.String("provider", "parsian"),
			attribute.String("result", ResultCreated)}, spans[0].Attributes())
		assert.Equal(t, codes.Unset, spans[0].Status().Code)
	}

	f = newWaterFixture(t)
	f.repo.lastStatus = StatusSuccess
	_, err = f.svc.Create(context.Background(), waterReq())
	assert.ErrorIs(t, err, ErrAlreadyPaid)
	f = newWaterFixture(t)
	f.sel.err = errors.New("db down")
	_, err = f.svc.Create(context.Background(), waterReq())
	assert.Error(t, err)
	f = newWaterFixture(t)
	_, err = f.svc.Create(context.Background(), Request{BillType: "gas 1234567890117"})
	assert.ErrorIs(t, err, ErrUnsupported)
	spans = jaegertest.ByName(sr, createSpan)
	if assert.Len(t, spans, 4) {
		for i, want := range []string{ResultAlreadyPaid, ResultError, ResultError} {
			assert.Equal(t, codes.Error, spans[i+1].Status().Code, want)
			assert.Equal(t, want, spans[i+1].Status().Description, want)
		}
		assert.NotContains(t, spans[3].Attributes(), attribute.String("bill_type", "gas 1234567890117"), "an unknown bill type is not in the span")
	}
	jaegertest.AssertClean(t, sr.Ended(), testWaterBill, testWaterPay, testUser, "db down", "1234567890117")
}

// TestCompleteSpan (conventions § 17): one span order.Payments.Complete, a child of the span in ctx,
// with the order ID, the bill type, the provider and the result; Error for a failed payment and for
// an error; no bill ID, pay ID, RRN or user in a span.
func TestCompleteSpan(t *testing.T) {
	sr := jaegertest.Install(t)
	req := CompleteRequest{OrderID: "order-1", RRN: "445566", Price: 259000}
	f := newWaterPayFixture(t)
	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	_, err := f.p.Complete(ctx, req)
	parent.End()
	assert.NoError(t, err)
	spans := jaegertest.ByName(sr, completeSpan)
	if assert.Len(t, spans, 1) {
		assert.Equal(t, parent.SpanContext().SpanID(), spans[0].Parent().SpanID())
		assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("order.id", "order-1"), attribute.String("bill_type", "water"),
			attribute.String("provider", "parsian"), attribute.String("result", StatusSuccess)}, spans[0].Attributes())
		assert.Equal(t, codes.Unset, spans[0].Status().Code)
	}

	f = newWaterPayFixture(t)
	f.parsian.payResp = &provider.PaymentResponse{Status: StatusFailed, Code: 400, Message: "refused"}
	_, err = f.p.Complete(context.Background(), req)
	assert.NoError(t, err)
	f = newWaterPayFixture(t)
	f.repo.latestErr = errors.New("db down")
	_, err = f.p.Complete(context.Background(), req)
	assert.Error(t, err)
	f = newWaterPayFixture(t)
	f.repo.latest.Status = StatusPending
	_, err = f.p.Complete(context.Background(), req)
	assert.NoError(t, err)
	spans = jaegertest.ByName(sr, completeSpan)
	if assert.Len(t, spans, 4) {
		assert.Equal(t, codes.Error, spans[1].Status().Code)
		assert.Equal(t, StatusFailed, spans[1].Status().Description)
		assert.Equal(t, codes.Error, spans[2].Status().Code)
		assert.Equal(t, PaymentError, spans[2].Status().Description)
		assert.Contains(t, spans[2].Attributes(), attribute.String("provider", NoProviderLabel))
		assert.Equal(t, codes.Unset, spans[3].Status().Code, "processed (pending) is not a failure")
		assert.Contains(t, spans[3].Attributes(), attribute.String("result", PaymentProcessed))
	}
	jaegertest.AssertClean(t, sr.Ended(), testWaterBill, testWaterPay, testUser, "445566", "db down", "refused")
}
