package qabzino

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// No test here reaches Ayantech (each call is charged): every endpoint is a local httptest server
// or a fake Doer.

const testToken = "test-token-not-real"

type seen struct {
	calls  atomic.Int32
	method string
	ctype  string
	body   []byte
}

// server answers every request with status and body, and records the request.
func server(t *testing.T, status int, body string) (*httptest.Server, *seen) {
	t.Helper()
	s := &seen{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		s.method = r.Method
		s.ctype = r.Header.Get("Content-Type")
		s.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, s
}

func newAdapter(t *testing.T, o Options) *Adapter {
	t.Helper()
	if o.Timeout == 0 {
		o.Timeout = 2 * time.Second
	}
	a, err := New(o)
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	a.newTrace = func() string { return "trace-1" }
	return a
}

var waterReq = provider.InquiryRequest{BillType: provider.BillTypeWater, UserID: "u", Dialer: "09120000000", InquiryID: "1234567890125"}

func TestNewRejectsBadTimeout(t *testing.T) {
	_, err := New(Options{})
	assert.Error(t, err)
	_, err = New(Options{Timeout: -time.Second})
	assert.Error(t, err)
}

func TestNameAndNotSupported(t *testing.T) {
	a := newAdapter(t, Options{})
	assert.Equal(t, "qabzino", a.Name())
	assert.False(t, a.Enabled())
	_, err := a.Pay(context.Background(), provider.PaymentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Status(context.Background(), provider.StatusRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.GetDocument(context.Background(), provider.DocumentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Settle(context.Background(), provider.SettleRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Inquiry(context.Background(), provider.InquiryRequest{BillType: provider.BillTypeElectricity})
	assert.ErrorIs(t, err, provider.ErrNotSupported, "electricity is not a Qabzino inquiry in this task")
	var nilAdapter *Adapter
	_, err = nilAdapter.Inquiry(context.Background(), waterReq)
	assert.Error(t, err)
	assert.False(t, nilAdapter.Enabled())
}

// TestInquiryRequestOnTheWire: one POST of JSON with the token, the bill ID and the trace number,
// to the water endpoint, and the success answer.
func TestInquiryRequestOnTheWire(t *testing.T) {
	srv, s := server(t, 200, `{"Parameters":{"Amount":25000000,"BillID":"1234567890125","PaymentID":"2500012345","PaymentDate":"1405/01/01","FullName":"x"},"Status":{"Code":"G00000","Description":"done"}}`)
	a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: srv.URL + "/water"})
	resp, err := a.Inquiry(context.Background(), waterReq)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.EqualValues(t, 1, s.calls.Load())
	assert.Equal(t, http.MethodPost, s.method)
	assert.Equal(t, "application/json", s.ctype)
	var got map[string]map[string]string
	if assert.NoError(t, json.Unmarshal(s.body, &got)) {
		assert.Equal(t, map[string]map[string]string{
			"Identity":   {"Token": testToken},
			"Parameters": {"WaterBillID": "1234567890125", "TraceNumber": "trace-1"},
		}, got)
	}
	assert.Equal(t, provider.ResponseSuccess, resp.Status)
	assert.EqualValues(t, 200, resp.Code)
	assert.Equal(t, "done", resp.Message)
	assert.Equal(t, "trace-1", resp.ProviderTraceID)
	assert.Equal(t, "1234567890125", resp.BillID)
	if assert.Len(t, resp.Inquiries, 1) {
		assert.Equal(t, provider.InquiryItem{PayID: "2500012345", Amount: 25_000_000, PaymentDeadline: "1405/01/01", InquiryType: "regular"}, resp.Inquiries[0])
	}
}

// TestInquirySuccessShapes: the amount comes from the pay ID; a pay ID that is too short or not a
// number keeps the amount of the answer; an empty pay ID is amount 0; an empty bill ID and an empty
// description fall back.
func TestInquirySuccessShapes(t *testing.T) {
	cases := []struct {
		name       string
		params     string
		wantAmount uint64
		wantPayID  string
	}{
		{"pay ID rule", `{"Amount":1,"PaymentID":"2500012345"}`, 25_000_000, "2500012345"},
		{"short pay ID keeps amount", `{"Amount":777,"PaymentID":"123"}`, 777, "123"},
		{"non-numeric pay ID keeps amount", `{"Amount":888,"PaymentID":"abcdefghij"}`, 888, "abcdefghij"},
		{"overflow keeps amount", `{"Amount":5,"PaymentID":"99999999999999999999912345"}`, 5, "99999999999999999999912345"},
		{"empty pay ID is amount 0", `{"Amount":900,"PaymentID":""}`, 0, ""},
	}
	for _, c := range cases {
		srv, _ := server(t, 200, `{"Parameters":`+c.params+`,"Status":{"Code":"G00000"}}`)
		a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: srv.URL})
		resp, err := a.Inquiry(context.Background(), waterReq)
		if !assert.NoError(t, err, c.name) || !assert.Len(t, resp.Inquiries, 1, c.name) {
			continue
		}
		assert.Equal(t, c.wantAmount, resp.Inquiries[0].Amount, c.name)
		assert.Equal(t, c.wantPayID, resp.Inquiries[0].PayID, c.name)
		assert.Equal(t, "1234567890125", resp.BillID, c.name+": the bill ID of the request when the answer has none")
		assert.Equal(t, successMessage, resp.Message, c.name+": the default message")
	}
}

func TestInquiryPaidBill(t *testing.T) {
	srv, _ := server(t, 200, `{"Parameters":null,"Status":{"Code":"GD2068","Description":"paid"}}`)
	a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: srv.URL})
	resp, err := a.Inquiry(context.Background(), waterReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) {
		assert.Equal(t, provider.ResponseSuccess, resp.Status)
		assert.Equal(t, paidMessage, resp.Message)
		assert.Equal(t, []provider.InquiryItem{{InquiryType: "regular"}}, resp.Inquiries)
		assert.Equal(t, "trace-1", resp.ProviderTraceID)
	}
}

func TestInquiryRefusedCodes(t *testing.T) {
	for code, want := range map[string]int32{"GD2061": 2061, "GD2062": 2062, "GD2067": 2067} {
		srv, _ := server(t, 200, `{"Status":{"Code":"`+code+`","Description":"x"}}`)
		a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: srv.URL})
		resp, err := a.Inquiry(context.Background(), waterReq)
		if assert.NoError(t, err, code) && assert.NotNil(t, resp, code) {
			assert.Equal(t, provider.ResponseFailed, resp.Status, code)
			assert.Equal(t, want, resp.Code, code)
			assert.Equal(t, refused[code], resp.Message, code)
			assert.Empty(t, resp.Inquiries, code)
		}
	}
}

// TestInquiryProviderDown: GD2066, an unknown code, an empty status, HTTP 500, bad JSON, an empty
// body and a success whose parameters are not an object are all provider down.
func TestInquiryProviderDown(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"GD2066", 200, `{"Status":{"Code":"GD2066"}}`},
		{"unknown code", 200, `{"Status":{"Code":"GD9999"}}`},
		{"token rejected", 200, `{"Status":{"Code":"GD2001"}}`},
		{"empty status", 200, `{}`},
		{"HTTP 500", 500, `{"Status":{"Code":"G00000"}}`},
		{"HTTP 401", 401, ``},
		{"bad JSON", 200, `<html>`},
		{"empty body", 200, ``},
		{"parameters not an object", 200, `{"Parameters":"x","Status":{"Code":"G00000"}}`},
	}
	for _, c := range cases {
		srv, _ := server(t, c.status, c.body)
		a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: srv.URL})
		resp, err := a.Inquiry(context.Background(), waterReq)
		assert.Nil(t, resp, c.name)
		assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
		if err != nil {
			assert.NotContains(t, err.Error(), testToken, c.name+": the token is never in an error")
		}
	}
}

// TestInquiryDisabledAndNotConfigured: no HTTP call is sent when the switch is off or the token or
// the endpoint is empty.
func TestInquiryDisabledAndNotConfigured(t *testing.T) {
	srv, s := server(t, 200, `{"Status":{"Code":"G00000"}}`)
	cases := []struct {
		name string
		o    Options
	}{
		{"disabled", Options{Enabled: false, Token: testToken, WaterEndpoint: srv.URL}},
		{"no token", Options{Enabled: true, WaterEndpoint: srv.URL}},
		{"no endpoint", Options{Enabled: true, Token: testToken}},
	}
	for _, c := range cases {
		a := newAdapter(t, c.o)
		resp, err := a.Inquiry(context.Background(), waterReq)
		assert.Nil(t, resp, c.name)
		assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
	}
	assert.EqualValues(t, 0, s.calls.Load(), "no request reached the server")
	assert.True(t, newAdapter(t, Options{Enabled: true}).Enabled())
}

func TestInquiryTransportErrors(t *testing.T) {
	// A closed server: connection refused on the local machine only.
	srv, _ := server(t, 200, `{}`)
	url := srv.URL
	srv.Close()
	a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: url})
	_, err := a.Inquiry(context.Background(), waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)

	// A bad URL.
	a = newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: "http://bad host/"})
	_, err = a.Inquiry(context.Background(), waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), testToken)
	}
}

// TestInquiryTimeoutAndCancel: a slow server and the timeout; a canceled context.
func TestInquiryTimeoutAndCancel(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer slow.Close()
	defer close(release)
	a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: slow.URL, Timeout: 100 * time.Millisecond})
	start := time.Now()
	_, err := a.Inquiry(context.Background(), waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Less(t, time.Since(start), 2*time.Second, "the timeout ended the call")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a = newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: slow.URL})
	_, err = a.Inquiry(ctx, waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.Canceled, "the cause stays visible to the flow")
}

// fakeDoer replaces the HTTP client; it also shows that the adapter uses the client it is given.
type fakeDoer struct {
	req *http.Request
	err error
}

func (f *fakeDoer) Do(r *http.Request) (*http.Response, error) {
	f.req = r
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"Parameters":{"PaymentID":"2500012345"},"Status":{"Code":"G00000"}}`))}, nil
}

func TestInquiryUsesGivenClientAndNilContext(t *testing.T) {
	d := &fakeDoer{}
	a := newAdapter(t, Options{Enabled: true, Token: testToken, WaterEndpoint: "https://ayantech.invalid/water", HTTPClient: d})
	//lint:ignore SA1012 a nil context is a case the adapter handles
	resp, err := a.Inquiry(nil, waterReq) //nolint:staticcheck
	if assert.NoError(t, err) && assert.NotNil(t, resp) {
		assert.Equal(t, "https://ayantech.invalid/water", d.req.URL.String())
	}
	d.err = errors.New("boom")
	_, err = a.Inquiry(context.Background(), waterReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
}

func TestDefaultTraceIsAUUID(t *testing.T) {
	a, err := New(Options{Timeout: time.Second})
	if assert.NoError(t, err) {
		x, y := a.newTrace(), a.newTrace()
		assert.Len(t, x, 36)
		assert.NotEqual(t, x, y)
	}
}

func TestAmountFromPayID(t *testing.T) {
	for in, want := range map[string]uint64{"2500012345": 25_000_000, "100000": 1000} {
		got, ok := amountFromPayID(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "12345", "abcdef", "99999999999999999999912345"} {
		_, ok := amountFromPayID(in)
		assert.False(t, ok, in)
	}
}

func TestMockFixtures(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Inquiry) {
		return
	}
	r := f.Inquiry(provider.InquiryRequest{InquiryID: "42"})
	assert.Equal(t, "42", r.BillID)
	assert.Equal(t, provider.ResponseSuccess, r.Status)
	if assert.Len(t, r.Inquiries, 1) {
		assert.Equal(t, mockPayID, r.Inquiries[0].PayID)
		amount, _ := amountFromPayID(mockPayID)
		assert.EqualValues(t, mockAmount, amount, "the pay ID and the amount agree")
	}
	assert.NotSame(t, r, f.Inquiry(provider.InquiryRequest{}), "a new value on each call")
}

// SupportsInquiry gives true only for the bill types that Inquiry answers (task 036); Inquiry of
// any other bill type is ErrNotSupported. A nil adapter answers the same.
func TestSupportsInquiry(t *testing.T) {
	want := map[string]bool{
		provider.BillTypeWater:       true,
		provider.BillTypeElectricity: false,
		provider.BillTypeGas:         true,
		provider.BillTypeIrancell:    false,
		provider.BillTypeMobile:      false,
		// task 037
		provider.BillTypeMobileMidTermRightel: true,
		"mobileRightel":                       false, // the seed row 32: not used by Qabz
		"tel":                                 false,
		"mobilemidterm":                       false,
		"":                                    false,
	}
	var nilAdapter *Adapter
	for billType, ok := range want {
		assert.Equal(t, ok, (&Adapter{}).SupportsInquiry(billType), billType)
		assert.Equal(t, ok, nilAdapter.SupportsInquiry(billType), billType)
	}
}
