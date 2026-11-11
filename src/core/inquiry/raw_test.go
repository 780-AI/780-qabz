package inquiry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Task 023: the raw inquiry (bill: raw.Service.Inquiry). The answer comes from the bill ID and the
// pay ID: no provider pick, no adapter, no repository call.

const rawPay = "3200012345" // a valid pay ID: 32 000 000 rials

func TestRawInquiry(t *testing.T) {
	r := newGasRig(t, time.Hour)
	res, err := r.svc.RawInquiry("gas", "123456789034", rawPay)
	if assert.NoError(t, err) {
		assert.Equal(t, RawResult{BillID: "123456789034", PayID: rawPay, Amount: 32_000_000, BillType: "gas"}, res)
	}
	// As in bill, the bill ID can be of another bill type than the endpoint.
	res, err = r.svc.RawInquiry("gas", validBill, "4500147")
	if assert.NoError(t, err) {
		assert.Equal(t, RawResult{BillID: validBill, PayID: "4500147", Amount: 45_000, BillType: "electricity"}, res)
	}
	assert.Equal(t, []string{"gas/none/raw", "gas/none/raw"}, r.inquiry, "the label is the bill type of the endpoint")
	assert.Empty(t, r.selector.picks, "no provider pick")
	assert.Equal(t, 0, r.gas.calls, "no provider call")
	assert.Equal(t, 0, r.repo.findCalls+r.repo.saveCalls, "no saved-inquiry read or write")
	assert.Empty(t, r.repo.requests, "no request row")
}

// TestRawInquiryErrors: the checks and their order are those of bill; each error wraps ErrInvalid
// and nothing is counted.
func TestRawInquiryErrors(t *testing.T) {
	r := newGasRig(t, time.Hour)
	cases := []struct {
		name, billID, payID string
		want                error
	}{
		{"bad bill ID", "1234567890124", rawPay, ErrInvalidBillID},
		{"empty bill ID", "", rawPay, ErrInvalidBillID},
		{"bad bill ID wins over a bad pay ID", "12345", "1", ErrInvalidBillID},
		{"bad pay ID check digit", validBill, "2500012345", ErrInvalidPayID},
		{"short pay ID", validBill, "12345", ErrInvalidPayID},
		{"empty pay ID", validBill, "", ErrInvalidPayID},
		{"bad pay ID wins over an unknown bill type", "123456789069", "2500012345", ErrInvalidPayID},
		{"unknown bill type", "123456789069", rawPay, ErrUnknownBillType},
		{"unknown mobile prefix", "12345612350", rawPay, ErrUnknownBillType},
	}
	for _, c := range cases {
		res, err := r.svc.RawInquiry("gas", c.billID, c.payID)
		assert.Equal(t, RawResult{}, res, c.name)
		assert.ErrorIs(t, err, c.want, c.name)
		assert.ErrorIs(t, err, ErrInvalid, c.name)
	}
	assert.Empty(t, r.inquiry, "a request that breaks a rule is not counted")
	// The three errors are different errors.
	assert.NotErrorIs(t, ErrInvalidPayID, ErrInvalidBillID)
	assert.NotErrorIs(t, ErrUnknownBillType, ErrInvalidPayID)

	var nilSvc *Service
	_, err := nilSvc.RawInquiry("gas", validBill, rawPay)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrInvalid)

	// No OnInquiry hook: no panic.
	svc, err := NewService(Options{Repository: newMemRepo(), Providers: r.selector, Adapters: fakeAdapters{}})
	if assert.NoError(t, err) && assert.NotNil(t, svc) {
		_, err = svc.RawInquiry("gas", validBill, rawPay)
		assert.NoError(t, err)
	}
}
