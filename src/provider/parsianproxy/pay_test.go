package parsianproxy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pb "qabz/api/pb/parsian-v2"
	"qabz/provider"
)

func payRequest() provider.PaymentRequest {
	return provider.PaymentRequest{
		BillType: provider.BillTypeTelMidterm, InquiryType: provider.InquiryTypeMidterm, OrderID: "o-1", UserID: "u-1",
		Dialer: "989121234567", InquiryID: "982188776655", BillID: "2101234560040", PayID: "12000035", Amount: 120000, RRN: "778899",
	}
}

// TestPayRequest (task 044): the fields of bill's ParsianTelephone; the call has the timeout; the
// answer of the service is kept as it came.
func TestPayRequest(t *testing.T) {
	f := &fakeClient{payResp: &pb.PayResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceId: 12345}}
	resp, err := newAdapter(t, f).Pay(context.Background(), payRequest())
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, &provider.PaymentResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceID: "12345"}, resp)
	if assert.Len(t, f.pays, 1) {
		assert.Equal(t, &pb.PayRequest{
			UserId: "u-1", Dialer: "989121234567", OrderId: "o-1", ProviderOrderId: 778899, InquiryId: "982188776655",
			BillType: "telmidterm", InquiryType: "midterm", BillId: "2101234560040", PayId: "12000035", Amount: 120000, PaymentRrn: "778899",
		}, f.pays[0])
	}
	assert.True(t, f.dl, "the call has a deadline")

	for _, status := range []string{"failed", "pending", "other", ""} {
		f := &fakeClient{payResp: &pb.PayResponse{Code: 400, Status: status, Message: "m"}}
		resp, err := newAdapter(t, f).Pay(context.Background(), payRequest())
		if assert.NoError(t, err, status) {
			assert.Equal(t, status, resp.Status, "the status as it came: the flow maps it")
			assert.Equal(t, "0", resp.ProviderTraceID)
		}
	}
}

// TestPayWater (task 048): a water bill has the fields of bill's ParsianWater: the same as
// ParsianTelephone, with the inquiry type regular and the bill ID as the inquiry ID.
func TestPayWater(t *testing.T) {
	f := &fakeClient{payResp: &pb.PayResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceId: 77}}
	req := provider.PaymentRequest{
		BillType: provider.BillTypeWater, InquiryType: provider.InquiryTypeRegular, OrderID: "o-2", UserID: "u-2",
		InquiryID: "1234567890125", BillID: "1234567890125", PayID: "2500012340", Amount: 25000000, RRN: "445566",
	}
	resp, err := newAdapter(t, f).Pay(context.Background(), req)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, &provider.PaymentResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceID: "77"}, resp)
	if assert.Len(t, f.pays, 1) {
		assert.Equal(t, &pb.PayRequest{
			UserId: "u-2", OrderId: "o-2", ProviderOrderId: 445566, InquiryId: "1234567890125", BillType: "water",
			InquiryType: "regular", BillId: "1234567890125", PayId: "2500012340", Amount: 25000000, PaymentRrn: "445566",
		}, f.pays[0])
	}
}

// TestPayElectricity (task 052): an electricity bill has the fields of bill's ParsianElectricity: the
// same as ParsianWater (the inquiry type regular, the bill ID as the inquiry ID).
func TestPayElectricity(t *testing.T) {
	f := &fakeClient{payResp: &pb.PayResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceId: 78}}
	req := provider.PaymentRequest{
		BillType: provider.BillTypeElectricity, InquiryType: provider.InquiryTypeRegular, OrderID: "o-3", UserID: "u-3",
		InquiryID: "1234567890125", BillID: "1234567890125", PayID: "1800012345", Amount: 18000000, RRN: "445577",
	}
	assert.True(t, SupportsPay(provider.BillTypeElectricity))
	resp, err := newAdapter(t, f).Pay(context.Background(), req)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, &provider.PaymentResponse{Code: 200, Status: "success", Message: "ok", ProviderTraceID: "78"}, resp)
	if assert.Len(t, f.pays, 1) {
		assert.Equal(t, &pb.PayRequest{
			UserId: "u-3", OrderId: "o-3", ProviderOrderId: 445577, InquiryId: "1234567890125", BillType: "electricity",
			InquiryType: "regular", BillId: "1234567890125", PayId: "1800012345", Amount: 18000000, PaymentRrn: "445577",
		}, f.pays[0])
	}
}

// TestPayRRN: an RRN that is not an integer is failed (400) with the error text and no call, as bill.
func TestPayRRN(t *testing.T) {
	for _, rrn := range []string{"", "abc", "12.5", "99999999999999999999"} {
		f := &fakeClient{payResp: &pb.PayResponse{Status: "success"}}
		req := payRequest()
		req.RRN = rrn
		resp, err := newAdapter(t, f).Pay(context.Background(), req)
		if assert.NoError(t, err, rrn) && assert.NotNil(t, resp, rrn) {
			assert.Equal(t, provider.StatusFailed, resp.Status, rrn)
			assert.Equal(t, int32(400), resp.Code, rrn)
			assert.Contains(t, resp.Message, "strconv.Atoi", rrn)
		}
		assert.Empty(t, f.pays, rrn)
	}
}

// TestPayDown: a failed call, a nil answer, the switch off, no address and a timeout wrap
// ErrProviderDown; another bill type is ErrNotSupported (no call).
func TestPayDown(t *testing.T) {
	f := &fakeClient{payErr: errors.New("unavailable")}
	_, err := newAdapter(t, f).Pay(context.Background(), payRequest())
	assert.ErrorIs(t, err, provider.ErrProviderDown)

	f = &fakeClient{}
	_, err = newAdapter(t, f).Pay(context.Background(), payRequest())
	assert.ErrorIs(t, err, provider.ErrProviderDown, "a nil answer")

	f = &fakeClient{payResp: &pb.PayResponse{Status: "success"}}
	a, _ := New(Options{Enabled: false, Client: f, Timeout: time.Second})
	_, err = a.Pay(context.Background(), payRequest())
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, errDisabled)
	assert.Empty(t, f.pays, "the switch is off: no call")

	c, _ := Dial("")
	a, _ = New(Options{Enabled: true, Client: c, Timeout: time.Second})
	_, err = a.Pay(context.Background(), payRequest())
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, errNoAddress)

	f = &fakeClient{block: true}
	a, _ = New(Options{Enabled: true, Client: f, Timeout: 20 * time.Millisecond})
	_, err = a.Pay(context.Background(), payRequest())
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	f = &fakeClient{payResp: &pb.PayResponse{Status: "success"}}
	for _, bt := range []string{provider.BillTypeMobile, provider.BillTypeGas, ""} {
		req := payRequest()
		req.BillType = bt
		_, err := newAdapter(t, f).Pay(context.Background(), req)
		assert.ErrorIs(t, err, provider.ErrNotSupported, bt)
	}
	assert.Empty(t, f.pays)
	assert.True(t, SupportsPay(provider.BillTypeTel))
	assert.True(t, SupportsPay(provider.BillTypeWater), "task 048")
	var nilAdapter *Adapter
	_, err = nilAdapter.Pay(context.Background(), payRequest())
	assert.Error(t, err)
}

// TestMockPay: the success answer of the mock; new on each call.
func TestMockPay(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Pay) {
		return
	}
	r := f.Pay(payRequest())
	assert.Equal(t, provider.StatusSuccess, r.Status)
	assert.Equal(t, MockPayTrace, r.ProviderTraceID)
	r.Status = "x"
	assert.Equal(t, provider.StatusSuccess, f.Pay(payRequest()).Status)
}
