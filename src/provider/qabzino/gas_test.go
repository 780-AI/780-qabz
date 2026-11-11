package qabzino

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"qabz/common/helpers"
	"qabz/provider"
)

// Task 023: the gas inquiry of the adapter. No test here reaches Ayantech: every endpoint is a
// local httptest server.

var gasReq = provider.InquiryRequest{BillType: provider.BillTypeGas, UserID: "u", Dialer: "09120000000", InquiryID: "0012345678"}

const gasSuccessBody = `{"Parameters":{"FullName":"n","Address":"addr","Amount":4567000,"BillID":"1234567890125","PaymentID":"3200012345","PaymentDate":"1405/01/01","ConsumptionType":"home","Tax":"1"},"Status":{"Code":"G00000","Description":"done"}}`

// TestGasInquiryRequestOnTheWire: one POST of JSON to the gas endpoint (not the water one) with the
// token, the inquiry ID as ParticipateCode, an empty GasBillID and the trace number; and the
// success answer with the amount of the answer and the details.
func TestGasInquiryRequestOnTheWire(t *testing.T) {
	gas, sg := server(t, 200, gasSuccessBody)
	water, sw := server(t, 200, `{"Status":{"Code":"GD9999"}}`)
	a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: water.URL, GasEndpoint: gas.URL + "/gas"})
	resp, err := a.Inquiry(context.Background(), gasReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.EqualValues(t, 1, sg.calls.Load())
	assert.EqualValues(t, 0, sw.calls.Load(), "a gas inquiry does not go to the water endpoint")
	assert.Equal(t, http.MethodPost, sg.method)
	assert.Equal(t, "application/json", sg.ctype)
	var got map[string]map[string]string
	if assert.NoError(t, json.Unmarshal(sg.body, &got)) {
		assert.Equal(t, map[string]map[string]string{
			"Identity":   {"Token": testToken},
			"Parameters": {"GasBillID": "", "ParticipateCode": "0012345678", "TraceNumber": "trace-1"},
		}, got)
	}
	assert.Equal(t, provider.ResponseSuccess, resp.Status)
	assert.EqualValues(t, 200, resp.Code)
	assert.Equal(t, "done", resp.Message)
	assert.Equal(t, "trace-1", resp.ProviderTraceID)
	assert.Equal(t, "1234567890125", resp.BillID)
	if assert.Len(t, resp.Inquiries, 1) {
		assert.Equal(t, provider.InquiryItem{PayID: "3200012345", Amount: 4_567_000, PaymentDeadline: "1405/01/01", InquiryType: "regular"},
			resp.Inquiries[0], "the amount of the answer, not the amount of the pay ID rule (32 000 000)")
	}
	assert.Equal(t, &provider.Details{Gas: &provider.GasDetails{City: "addr", Kind: "home"}}, resp.Details)
}

// TestWaterStillGoesToWaterEndpoint: with both endpoints set, a water inquiry goes to the water
// endpoint with the water Parameters and has no details (as before task 023).
func TestWaterStillGoesToWaterEndpoint(t *testing.T) {
	gas, sg := server(t, 200, gasSuccessBody)
	water, sw := server(t, 200, `{"Parameters":{"Amount":1,"BillID":"1234567890125","PaymentID":"2500012345","PaymentDate":"d"},"Status":{"Code":"G00000"}}`)
	a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: water.URL, GasEndpoint: gas.URL})
	resp, err := a.Inquiry(context.Background(), waterReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.EqualValues(t, 1, sw.calls.Load())
	assert.EqualValues(t, 0, sg.calls.Load())
	var got map[string]map[string]string
	if assert.NoError(t, json.Unmarshal(sw.body, &got)) {
		assert.Equal(t, map[string]string{"WaterBillID": "1234567890125", "TraceNumber": "trace-1"}, got["Parameters"])
	}
	assert.Nil(t, resp.Details)
	if assert.Len(t, resp.Inquiries, 1) {
		assert.EqualValues(t, 25_000_000, resp.Inquiries[0].Amount, "water: the pay ID rule")
	}
}

// TestGasInquirySuccessShapes: an empty pay ID is amount 0; an answer with no bill ID keeps it
// empty (the inquiry ID is a participate code); an empty description falls back; missing details
// are empty strings.
func TestGasInquirySuccessShapes(t *testing.T) {
	cases := []struct {
		name       string
		params     string
		wantAmount uint64
		wantPayID  string
		wantBillID string
	}{
		{"amount as sent", `{"Amount":777,"PaymentID":"3200012345","BillID":"b"}`, 777, "3200012345", "b"},
		{"empty pay ID is amount 0", `{"Amount":900,"PaymentID":"","BillID":"b"}`, 0, "", "b"},
		{"no pay ID key is amount 0", `{"Amount":900}`, 0, "", ""},
		{"no bill ID stays empty", `{"Amount":5,"PaymentID":"123"}`, 5, "123", ""},
	}
	for _, c := range cases {
		srv, _ := server(t, 200, `{"Parameters":`+c.params+`,"Status":{"Code":"G00000"}}`)
		a := newAdapter(t, Options{Enabled: true, Token: testToken, GasEndpoint: srv.URL})
		resp, err := a.Inquiry(context.Background(), gasReq)
		if !assert.NoError(t, err, c.name) || !assert.NotNil(t, resp, c.name) || !assert.Len(t, resp.Inquiries, 1, c.name) {
			continue
		}
		assert.Equal(t, c.wantAmount, resp.Inquiries[0].Amount, c.name)
		assert.Equal(t, c.wantPayID, resp.Inquiries[0].PayID, c.name)
		assert.Equal(t, c.wantBillID, resp.BillID, c.name)
		assert.Equal(t, successMessage, resp.Message, c.name)
		assert.Equal(t, &provider.Details{Gas: &provider.GasDetails{}}, resp.Details, c.name)
	}
}

// TestGasNoBillToPay: GD2038 is success with one inquiry of amount 0 and no pay ID. The water code
// of a paid bill (GD2068) is not a gas code: provider down.
func TestGasNoBillToPay(t *testing.T) {
	srv, _ := server(t, 200, `{"Parameters":null,"Status":{"Code":"GD2038","Description":"x"}}`)
	a := newAdapter(t, Options{Enabled: true, Token: testToken, GasEndpoint: srv.URL})
	resp, err := a.Inquiry(context.Background(), gasReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) {
		assert.Equal(t, provider.ResponseSuccess, resp.Status)
		assert.Equal(t, gasNoBillMessage, resp.Message)
		assert.Equal(t, []provider.InquiryItem{{InquiryType: "regular"}}, resp.Inquiries)
		assert.Equal(t, "trace-1", resp.ProviderTraceID)
		assert.Nil(t, resp.Details)
	}
	srv2, _ := server(t, 200, `{"Status":{"Code":"GD2068"}}`)
	a = newAdapter(t, Options{Enabled: true, Token: testToken, GasEndpoint: srv2.URL})
	_, err = a.Inquiry(context.Background(), gasReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	// And the gas code is not a water code.
	a = newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: srv.URL})
	_, err = a.Inquiry(context.Background(), waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
}

func TestGasRefusedCodes(t *testing.T) {
	for code, want := range map[string]int32{"GD2031": 2031, "GD2032": 2032, "GD2033": 2033, "GD2037": 2037} {
		srv, _ := server(t, 200, `{"Status":{"Code":"`+code+`","Description":"x"}}`)
		a := newAdapter(t, Options{Enabled: true, Token: testToken, GasEndpoint: srv.URL})
		resp, err := a.Inquiry(context.Background(), gasReq)
		if assert.NoError(t, err, code) && assert.NotNil(t, resp, code) {
			assert.Equal(t, provider.ResponseFailed, resp.Status, code)
			assert.Equal(t, want, resp.Code, code)
			assert.Equal(t, gasRefused[code], resp.Message, code)
			assert.NotEmpty(t, resp.Message, code)
			assert.Empty(t, resp.Inquiries, code)
		}
		// A gas code on a water inquiry is not refused: provider down (as before task 023).
		a = newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: srv.URL})
		_, err = a.Inquiry(context.Background(), waterReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown, code)
	}
}

// TestGasProviderDown: GD2036, GD2019, a water refused code, an unknown code, an empty status, a
// bad HTTP status, bad JSON and a success whose parameters are not an object are provider down.
func TestGasProviderDown(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"GD2036", 200, `{"Status":{"Code":"GD2036"}}`},
		{"GD2019", 200, `{"Status":{"Code":"GD2019"}}`},
		{"water code GD2061", 200, `{"Status":{"Code":"GD2061"}}`},
		{"unknown code", 200, `{"Status":{"Code":"GD9999"}}`},
		{"empty status", 200, `{}`},
		{"HTTP 500", 500, gasSuccessBody},
		{"bad JSON", 200, `<html>`},
		{"parameters not an object", 200, `{"Parameters":[1],"Status":{"Code":"G00000"}}`},
	}
	for _, c := range cases {
		srv, _ := server(t, c.status, c.body)
		a := newAdapter(t, Options{Enabled: true, Token: testToken, GasEndpoint: srv.URL})
		resp, err := a.Inquiry(context.Background(), gasReq)
		assert.Nil(t, resp, c.name)
		assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
		if err != nil {
			assert.NotContains(t, err.Error(), testToken, c.name+": the token is never in an error")
		}
	}
}

// TestGasDisabledAndNotConfigured: no HTTP call when the switch is off, or the token or the gas
// endpoint is empty. A water endpoint does not serve gas, and the error names the gas key.
func TestGasDisabledAndNotConfigured(t *testing.T) {
	srv, s := server(t, 200, gasSuccessBody)
	cases := []struct {
		name string
		o    Options
		text string
	}{
		{"disabled", Options{Enabled: false, Token: testToken, GasEndpoint: srv.URL}, "providers.qabzino.enabled is false"},
		{"no token", Options{Enabled: true, GasEndpoint: srv.URL}, "gas_bill_endpoint"},
		{"no gas endpoint", Options{Enabled: true, Token: testToken, WaterEndpoint: srv.URL}, "gas_bill_endpoint"},
	}
	for _, c := range cases {
		a := newAdapter(t, c.o)
		resp, err := a.Inquiry(context.Background(), gasReq)
		assert.Nil(t, resp, c.name)
		assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
		assert.ErrorContains(t, err, c.text, c.name)
	}
	assert.EqualValues(t, 0, s.calls.Load(), "no request reached the server")
	// The water error text is the one of before task 023.
	_, err := newAdapter(t, Options{Enabled: true, Token: testToken, GasEndpoint: srv.URL}).Inquiry(context.Background(), waterReq)
	assert.ErrorContains(t, err, "providers.qabzino.token and providers.qabzino.water_bill_endpoint are required")
}

func TestGasMockFixture(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Inquiry) {
		return
	}
	r := f.Inquiry(gasReq)
	if !assert.NotNil(t, r) {
		return
	}
	assert.Equal(t, provider.ResponseSuccess, r.Status)
	assert.Equal(t, MockGasBillID, r.BillID, "a fixed bill ID: the request has a participate code")
	assert.True(t, helpers.ValidBillID(MockGasBillID))
	if assert.Len(t, r.Inquiries, 1) {
		assert.Equal(t, provider.InquiryItem{PayID: "3200012345", Amount: 32_000_000, PaymentDeadline: "2030-01-01", InquiryType: "regular"}, r.Inquiries[0])
		amount, _ := amountFromPayID(mockGasPayID)
		assert.EqualValues(t, mockGasAmount, amount, "the pay ID and the amount agree")
	}
	if assert.NotNil(t, r.Details) && assert.NotNil(t, r.Details.Gas) {
		assert.NotEmpty(t, r.Details.Gas.City)
		assert.NotEmpty(t, r.Details.Gas.Kind)
	}
	assert.Nil(t, r.Details.Electricity)
	assert.NotSame(t, r, f.Inquiry(gasReq), "a new value on each call")
	// The water fixture is unchanged.
	w := f.Inquiry(waterReq)
	if assert.NotNil(t, w) && assert.Len(t, w.Inquiries, 1) {
		assert.Equal(t, []any{waterReq.InquiryID, mockPayID, uint64(mockAmount)}, []any{w.BillID, w.Inquiries[0].PayID, w.Inquiries[0].Amount})
		assert.Nil(t, w.Details)
	}
}
