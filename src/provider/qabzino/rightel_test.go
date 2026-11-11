package qabzino

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// Task 037: the Rightel mobile inquiry of the adapter (the qabzino service: RightelMobile.Execute).
// No test here reaches Ayantech: every endpoint is a local httptest server.

var rightelReq = provider.InquiryRequest{BillType: provider.BillTypeMobileMidTermRightel, UserID: "u", Dialer: "989120000000", InquiryID: "989201234567"}

const rightelSuccessBody = `{"Parameters":{"Address":"a","MobileNumber":"09201234567","FullName":"n",` +
	`"MidTerm":{"BillID":"9201234560","PaymentID":"12300341","Amount":123000,"PaymentDate":"1405/01/01","CurrentDate":"c","PreviousDate":"p","ExtraInfo":null},` +
	`"FinalTerm":{"BillID":"9201234569","PaymentID":"45600342","Amount":456000}},"Status":{"Code":"G00000","Description":"done"}}`

// TestRightelInquiryRequestOnTheWire: one POST of JSON to the Rightel endpoint (not the water or
// the gas one) with the token, the phone in the 0 form and the trace number; the success answer has
// the mid-term bill ID and a mid-term and a final-term inquiry with their pay IDs and amounts as
// they are, and no deadline.
func TestRightelInquiryRequestOnTheWire(t *testing.T) {
	rightel, sr := server(t, 200, rightelSuccessBody)
	water, sw := server(t, 200, `{"Status":{"Code":"GD9999"}}`)
	gas, sg := server(t, 200, `{"Status":{"Code":"GD9999"}}`)
	a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: water.URL, GasEndpoint: gas.URL, RightelEndpoint: rightel.URL + "/rightel"})
	resp, err := a.Inquiry(context.Background(), rightelReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.EqualValues(t, 1, sr.calls.Load())
	assert.EqualValues(t, 0, sw.calls.Load()+sg.calls.Load(), "a Rightel inquiry goes only to the Rightel endpoint")
	assert.Equal(t, http.MethodPost, sr.method)
	assert.Equal(t, "application/json", sr.ctype)
	var got map[string]map[string]string
	if assert.NoError(t, json.Unmarshal(sr.body, &got)) {
		assert.Equal(t, map[string]map[string]string{
			"Identity":   {"Token": testToken},
			"Parameters": {"MobileNumber": "09201234567", "TraceNumber": "trace-1"},
		}, got)
	}
	assert.Equal(t, provider.ResponseSuccess, resp.Status)
	assert.EqualValues(t, 200, resp.Code)
	assert.Equal(t, "done", resp.Message)
	assert.Equal(t, "trace-1", resp.ProviderTraceID)
	assert.Equal(t, "9201234560", resp.BillID, "the bill ID of the mid-term part")
	assert.Equal(t, []provider.InquiryItem{
		{PayID: "12300341", Amount: 123000, InquiryType: provider.InquiryTypeMidterm},
		{PayID: "45600342", Amount: 456000, InquiryType: provider.InquiryTypeFinalTerm},
	}, resp.Inquiries, "the amounts of the answer, no pay ID rule, no deadline")
	assert.Nil(t, resp.Details)
}

// TestZeroFormPhone: the rule of the qabzino service (preparePhoneRequest).
func TestZeroFormPhone(t *testing.T) {
	for in, want := range map[string]string{
		"989201234567": "09201234567",
		"9201234567":   "09201234567",
		"98":           "",
		"09201234567":  "",
		"":             "",
	} {
		assert.Equal(t, want, zeroFormPhone(in), in)
	}
}

// TestRightelSuccessShapes: empty parts give empty pay IDs and amount 0 (as the qabzino service);
// an empty description falls back to the success text.
func TestRightelSuccessShapes(t *testing.T) {
	srv, _ := server(t, 200, `{"Parameters":{"MidTerm":{},"FinalTerm":{"Amount":5}},"Status":{"Code":"G00000"}}`)
	resp, err := newAdapter(t, Options{Enabled: true, Token: testToken, RightelEndpoint: srv.URL}).Inquiry(context.Background(), rightelReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, successMessage, resp.Message)
	assert.Equal(t, "", resp.BillID)
	assert.Equal(t, []provider.InquiryItem{
		{InquiryType: provider.InquiryTypeMidterm},
		{Amount: 5, InquiryType: provider.InquiryTypeFinalTerm},
	}, resp.Inquiries)
}

// TestRightelNoBillToPay: GD2268 is success with no debt: a mid-term and a final-term inquiry of
// amount 0 (bill: handlePhoneResponseWithoutInquiry).
func TestRightelNoBillToPay(t *testing.T) {
	srv, _ := server(t, 200, `{"Status":{"Code":"GD2268","Description":"x"}}`)
	resp, err := newAdapter(t, Options{Enabled: true, Token: testToken, RightelEndpoint: srv.URL}).Inquiry(context.Background(), rightelReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, provider.ResponseSuccess, resp.Status)
	assert.Equal(t, rightelNoBillMessage, resp.Message)
	assert.Equal(t, "trace-1", resp.ProviderTraceID)
	assert.Equal(t, provider.NoDebtMobileInquiries(), resp.Inquiries)
}

// TestRightelRefusedCodes: GD2261 and GD2267 are a failed answer with the code and the user text.
func TestRightelRefusedCodes(t *testing.T) {
	for code, n := range map[string]int32{"GD2261": 2261, "GD2267": 2267} {
		srv, _ := server(t, 200, `{"Status":{"Code":"`+code+`"}}`)
		resp, err := newAdapter(t, Options{Enabled: true, Token: testToken, RightelEndpoint: srv.URL}).Inquiry(context.Background(), rightelReq)
		if !assert.NoError(t, err, code) || !assert.NotNil(t, resp, code) {
			continue
		}
		assert.Equal(t, provider.ResponseFailed, resp.Status, code)
		assert.Equal(t, n, resp.Code, code)
		assert.Equal(t, rightelRefused[code], resp.Message, code)
		assert.NotEmpty(t, resp.Message, code)
		assert.Empty(t, resp.Inquiries, code)
	}
}

// TestRightelProviderDown: GD2266 (the Rightel server cannot be reached), the codes of the other
// kinds, an unknown code, an empty status, a bad HTTP status, bad JSON and parameters that are not
// an object are provider down.
func TestRightelProviderDown(t *testing.T) {
	for name, c := range map[string]struct {
		status int
		body   string
	}{
		"GD2266":                   {200, `{"Status":{"Code":"GD2266"}}`},
		"water code GD2068":        {200, `{"Status":{"Code":"GD2068"}}`},
		"gas code GD2038":          {200, `{"Status":{"Code":"GD2038"}}`},
		"unknown code":             {200, `{"Status":{"Code":"GD9999"}}`},
		"empty status":             {200, `{}`},
		"HTTP 500":                 {500, rightelSuccessBody},
		"bad JSON":                 {200, `<html>`},
		"parameters not an object": {200, `{"Parameters":"x","Status":{"Code":"G00000"}}`},
	} {
		srv, _ := server(t, c.status, c.body)
		resp, err := newAdapter(t, Options{Enabled: true, Token: testToken, RightelEndpoint: srv.URL}).Inquiry(context.Background(), rightelReq)
		assert.Nil(t, resp, name)
		assert.ErrorIs(t, err, provider.ErrProviderDown, name)
		if err != nil {
			assert.NotContains(t, err.Error(), testToken, name+": the token is never in an error")
		}
	}
}

// TestRightelDisabledAndNotConfigured: no HTTP call when the switch is off, or the token or the
// Rightel endpoint is empty; the error names the Rightel key.
func TestRightelDisabledAndNotConfigured(t *testing.T) {
	srv, s := server(t, 200, rightelSuccessBody)
	for name, c := range map[string]struct {
		o    Options
		text string
	}{
		"disabled":            {Options{Enabled: false, Token: testToken, RightelEndpoint: srv.URL}, "providers.qabzino.enabled is false"},
		"no token":            {Options{Enabled: true, RightelEndpoint: srv.URL}, "rightel_bill_endpoint"},
		"no rightel endpoint": {Options{Enabled: true, Token: testToken, WaterEndpoint: srv.URL, GasEndpoint: srv.URL}, "rightel_bill_endpoint"},
	} {
		resp, err := newAdapter(t, c.o).Inquiry(context.Background(), rightelReq)
		assert.Nil(t, resp, name)
		assert.ErrorIs(t, err, provider.ErrProviderDown, name)
		assert.ErrorContains(t, err, c.text, name)
	}
	assert.EqualValues(t, 0, s.calls.Load(), "no request reached the server")
}

// TestRightelMockFixture: the fixed Rightel answer; the water and gas fixtures do not change.
func TestRightelMockFixture(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Inquiry) {
		return
	}
	r := f.Inquiry(rightelReq)
	if !assert.NotNil(t, r) {
		return
	}
	assert.Equal(t, provider.ResponseSuccess, r.Status)
	assert.Equal(t, MockRightelBillID, r.BillID)
	assert.Equal(t, []provider.InquiryItem{
		{PayID: "23000341", Amount: 230_000, InquiryType: provider.InquiryTypeMidterm},
		{PayID: "63000342", Amount: 630_000, InquiryType: provider.InquiryTypeFinalTerm},
	}, r.Inquiries)
	assert.Nil(t, r.Details)
	assert.NotSame(t, r, f.Inquiry(rightelReq), "a new value on each call")

	if g := f.Inquiry(gasReq); assert.NotNil(t, g) && assert.Len(t, g.Inquiries, 1) {
		assert.Equal(t, MockGasBillID, g.BillID)
		assert.EqualValues(t, mockGasAmount, g.Inquiries[0].Amount)
	}
	if w := f.Inquiry(waterReq); assert.NotNil(t, w) && assert.Len(t, w.Inquiries, 1) {
		assert.Equal(t, waterReq.InquiryID, w.BillID)
		assert.EqualValues(t, mockAmount, w.Inquiries[0].Amount)
	}
}
