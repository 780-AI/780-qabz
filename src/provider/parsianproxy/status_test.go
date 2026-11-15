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

var statusReq = provider.StatusRequest{OrderID: "0b6c1d6e-6f1a-4c3e-9a51-6f8f4a0c0045", BillType: provider.BillTypeTel}

// TestStatusRequest (task 045): the request of bill's ParsianPaymentInquiry (order_id, bill_type);
// the call has the timeout; the answer is given as it came, for every status text.
func TestStatusRequest(t *testing.T) {
	for _, status := range []string{"success", "failed", "pending", "in-progress", "other", ""} {
		f := &fakeClient{statusResp: &pb.OrderInquiryResponse{Status: status, Message: "m", ProviderTraceId: 9900000044}}
		resp, err := newAdapter(t, f).Status(context.Background(), statusReq)
		if !assert.NoError(t, err, status) || !assert.NotNil(t, resp, status) {
			continue
		}
		assert.Equal(t, &provider.StatusResponse{Status: status, Message: "m", ProviderTraceID: "9900000044"}, resp)
		if assert.Len(t, f.statuses, 1) {
			assert.Equal(t, &pb.OrderInquiryRequest{OrderId: statusReq.OrderID, BillType: "tel"}, f.statuses[0])
		}
		assert.True(t, f.dl, "the call has a deadline")
	}
}

// TestStatusDown (task 045): a gRPC error, a timeout, an empty answer, the switch off and no
// address are ErrProviderDown (bill: pending); the switch off and no address call nothing.
func TestStatusDown(t *testing.T) {
	_, err := newAdapter(t, &fakeClient{statusErr: errors.New("unavailable")}).Status(context.Background(), statusReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorContains(t, err, "unavailable", "the error text is kept (bill: the message)")

	_, err = newAdapter(t, &fakeClient{}).Status(context.Background(), statusReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown, "an empty answer")

	a, err := New(Options{Enabled: true, Client: &fakeClient{block: true}, Timeout: 20 * time.Millisecond})
	if assert.NoError(t, err) {
		_, err = a.Status(context.Background(), statusReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown, "a timeout")
	}

	f := &fakeClient{statusResp: &pb.OrderInquiryResponse{Status: "success"}}
	off, err := New(Options{Enabled: false, Client: f, Timeout: time.Second})
	if assert.NoError(t, err) {
		_, err = off.Status(context.Background(), statusReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown)
		assert.ErrorIs(t, err, errDisabled)
	}
	assert.Empty(t, f.statuses, "the switch off sends nothing")

	c, err := Dial("")
	if assert.NoError(t, err) {
		_, err = newAdapter(t, c).Status(context.Background(), statusReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown)
		assert.ErrorIs(t, err, errNoAddress)
	}
	var nilAdapter *Adapter
	_, err = nilAdapter.Status(context.Background(), statusReq)
	assert.Error(t, err)
}

// TestMockStatus (task 045): the success answer of the mock; new on each call.
func TestMockStatus(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Status) {
		return
	}
	r := f.Status(statusReq)
	if !assert.NotNil(t, r) {
		return
	}
	assert.Equal(t, provider.StatusSuccess, r.Status)
	assert.Equal(t, MockPayTrace, r.ProviderTraceID)
	r.Status = "x"
	if again := f.Status(statusReq); assert.NotNil(t, again) {
		assert.Equal(t, provider.StatusSuccess, again.Status, "a new value on each call")
	}
}
