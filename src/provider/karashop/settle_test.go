package karashop

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// settleBody is the inner answer of a settlement (resultCode is the JSON text of result.status.code).
func settleBody(t *testing.T, resultCode string, statusCode int) string {
	t.Helper()
	return outer(t, fmt.Sprintf(`{"result":{"data":1,"status":{"code":%s,"message":null}},"status":{"code":%d,"message":"m"},"meta":{"transactionId":"tx-settle"}}`, resultCode, statusCode))
}

func newSettleAdapter(t *testing.T, f *fakeKarashop) *Adapter {
	t.Helper()
	a, srv := newAdapter(t, f, 5*time.Second)
	a.paymentInfoURL = srv.URL + "/settle"
	return a
}

// paidAt is 2026-11-15 08:04:05 UTC: 11:34:05 in Tehran, the Shamsi date 1405-08-24.
var paidAt = time.Date(2026, 11, 15, 8, 4, 5, 0, time.UTC)

func settleRequest() provider.SettleRequest {
	return provider.SettleRequest{
		BillType: provider.BillTypeTelMidterm, TelephoneNumber: testTel, Amount: 120000, BillID: "2101234560040",
		PayID: "12000035", RRN: "778899", PaidAt: paidAt, PaymentProvider: "parsian",
	}
}

// TestSettleRequestBody (task 044): one POST to the payment-info URL with the credential, the
// service tci-SendPaymentInfo-1 and bill's nine parameters in bill's order.
func TestSettleRequestBody(t *testing.T) {
	f := &fakeKarashop{answer: func(string) (int, string) { return 200, settleBody(t, "null", 200) }}
	a := newSettleAdapter(t, f)
	resp, err := a.Settle(context.Background(), settleRequest())
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, &provider.SettleResponse{Status: provider.StatusSuccess, Code: 200, Message: "عملیات با موفقیت انجام شد", ProviderTraceID: "tx-settle"}, resp)
	if !assert.Len(t, f.bodies, 1) {
		return
	}
	b := f.bodies[0]
	assert.Equal(t, serviceSendPaymentInfo, b.Service)
	assert.Equal(t, credential{Code: testCode, Password: testPassword}, b.Credential)
	assert.Equal(t, []parameter{
		{Name: "telNumber", Value: "2188776655"},
		{Name: "amount", Value: "120000"},
		{Name: "billId", Value: "2101234560040"},
		{Name: "paymentId", Value: "12000035"},
		{Name: "bankName", Value: "parsian"},
		{Name: "referenceCode", Value: "778899"},
		{Name: "channelName", Value: "IPG_ghabzino"},
		{Name: "paymentDate", Value: "14050824"},
		{Name: "paymentTime", Value: "113405"},
	}, b.Parameters)
}

// TestSettleBanks: the bank of the paid provider (bill's map); an unknown provider gives "".
func TestSettleBanks(t *testing.T) {
	for prov, want := range map[string]string{"parsian": "parsian", "eways": "dey", "eways2": "dey", "other": ""} {
		f := &fakeKarashop{answer: func(string) (int, string) { return 200, settleBody(t, "null", 200) }}
		a := newSettleAdapter(t, f)
		req := settleRequest()
		req.PaymentProvider = prov
		_, err := a.Settle(context.Background(), req)
		if assert.NoError(t, err) && assert.Len(t, f.bodies, 1) {
			assert.Equal(t, parameter{Name: "bankName", Value: want}, f.bodies[0].Parameters[4], prov)
		}
	}
}

// TestSettleAnswers: bill's two code tables. A result code of the table is failed (it wins over the
// status code); an unknown result code goes on to the status code; an unknown status code is
// pending with the "provider down" text. The HTTP status is not checked, as bill.
func TestSettleAnswers(t *testing.T) {
	cases := []struct {
		resultCode string
		statusCode int
		httpStatus int
		status     string
		code       int32
	}{
		{"null", 200, 200, provider.StatusSuccess, 200},
		{"null", 202, 200, provider.StatusSuccess, 200},
		{"null", 400, 200, provider.StatusFailed, 400},
		{"null", 401, 200, provider.StatusFailed, 400},
		{"null", 429, 200, provider.StatusFailed, 400},
		{"null", 499, 200, provider.StatusPending, 500},
		{"null", 500, 200, provider.StatusPending, 500},
		{"null", 503, 200, provider.StatusPending, 500},
		{"null", 504, 200, provider.StatusFailed, 400},
		{"null", 520, 200, provider.StatusFailed, 400},
		{"null", 999, 200, provider.StatusPending, 500},
		{"6", 200, 200, provider.StatusFailed, 400},
		{"1", 500, 200, provider.StatusFailed, 400},
		{"7", 200, 200, provider.StatusSuccess, 200},
		{"null", 200, 500, provider.StatusSuccess, 200},
	}
	for _, c := range cases {
		name := fmt.Sprintf("result %s status %d http %d", c.resultCode, c.statusCode, c.httpStatus)
		f := &fakeKarashop{answer: func(string) (int, string) { return c.httpStatus, settleBody(t, c.resultCode, c.statusCode) }}
		resp, err := newSettleAdapter(t, f).Settle(context.Background(), settleRequest())
		if assert.NoError(t, err, name) && assert.NotNil(t, resp, name) {
			assert.Equal(t, c.status, resp.Status, name)
			assert.Equal(t, c.code, resp.Code, name)
			assert.NotEmpty(t, resp.Message, name)
			assert.Equal(t, "tx-settle", resp.ProviderTraceID, name)
		}
	}
	f := &fakeKarashop{answer: func(string) (int, string) { return 200, settleBody(t, "6", 200) }}
	resp, err := newSettleAdapter(t, f).Settle(context.Background(), settleRequest())
	if assert.NoError(t, err) {
		assert.Equal(t, "اطلاعات ارسالی تکراریست", resp.Message)
	}
	f = &fakeKarashop{answer: func(string) (int, string) { return 200, settleBody(t, "null", 777) }}
	resp, err = newSettleAdapter(t, f).Settle(context.Background(), settleRequest())
	if assert.NoError(t, err) {
		assert.Equal(t, providerDownText, resp.Message)
	}
}

// TestSettleFailures: a body that cannot be read, a transport error and a timeout are errors that
// wrap ErrProviderDown; the switch, the credential and the URL are checked before any call; a bill
// type that is not a landline is ErrNotSupported.
func TestSettleFailures(t *testing.T) {
	for _, body := range []string{"not json", `{"responseText":3}`, `{"responseText":"no json"}`} {
		f := &fakeKarashop{answer: func(string) (int, string) { return 200, body }}
		_, err := newSettleAdapter(t, f).Settle(context.Background(), settleRequest())
		assert.ErrorIs(t, err, provider.ErrProviderDown, body)
	}

	f := &fakeKarashop{delay: time.Second, answer: func(string) (int, string) { return 200, settleBody(t, "null", 200) }}
	a, srv := newAdapter(t, f, 50*time.Millisecond)
	a.paymentInfoURL = srv.URL
	_, err := a.Settle(context.Background(), settleRequest())
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "the cause is kept")

	_, err = (&Adapter{enabled: true, code: "c", password: "p", paymentInfoURL: "http://x", timeout: time.Second, client: failDoer{}}).Settle(context.Background(), settleRequest())
	assert.ErrorIs(t, err, provider.ErrProviderDown)

	for name, mod := range map[string]func(*Adapter){
		"disabled": func(a *Adapter) { a.enabled = false },
		"no code":  func(a *Adapter) { a.code = "" },
		"no pass":  func(a *Adapter) { a.password = "" },
		"no url":   func(a *Adapter) { a.paymentInfoURL = "" },
	} {
		f := &fakeKarashop{answer: func(string) (int, string) { return 200, settleBody(t, "null", 200) }}
		a := newSettleAdapter(t, f)
		mod(a)
		_, err := a.Settle(context.Background(), settleRequest())
		assert.ErrorIs(t, err, provider.ErrProviderDown, name)
		assert.Empty(t, f.bodies, name+": nothing is sent")
	}

	f = &fakeKarashop{answer: func(string) (int, string) { return 200, settleBody(t, "null", 200) }}
	a = newSettleAdapter(t, f)
	for _, bt := range []string{provider.BillTypeWater, provider.BillTypeMobile, ""} {
		req := settleRequest()
		req.BillType = bt
		_, err := a.Settle(context.Background(), req)
		assert.ErrorIs(t, err, provider.ErrNotSupported, bt)
	}
	assert.Empty(t, f.bodies)
	assert.True(t, SupportsSettle(provider.BillTypeTel))
	assert.True(t, SupportsSettle(provider.BillTypeTelMidterm))
	var nilAdapter *Adapter
	_, err = nilAdapter.Settle(context.Background(), settleRequest())
	assert.Error(t, err)
}

// failDoer fails every call.
type failDoer struct{}

func (failDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}

// TestSettleNoSecretInError: no error text has the password.
func TestSettleNoSecretInError(t *testing.T) {
	_, err := (&Adapter{enabled: true, code: "c", password: testPassword, paymentInfoURL: "http://x", timeout: time.Second, client: failDoer{}}).Settle(context.Background(), settleRequest())
	if assert.Error(t, err) {
		assert.NotContains(t, err.Error(), testPassword)
	}
}

// TestShamsiDate: known dates of the Jalali calendar (Nowruz, the ends of the months, a leap year)
// and the date of an instant in Tehran.
func TestShamsiDate(t *testing.T) {
	for g, want := range map[[3]int]string{
		{2024, 3, 20}:  "14030101",
		{2025, 3, 21}:  "14040101",
		{2026, 3, 21}:  "14050101",
		{2026, 3, 20}:  "14041229",
		{2025, 3, 20}:  "14031230", // 1403 is a leap year
		{2026, 9, 22}:  "14050631",
		{2026, 9, 23}:  "14050701",
		{2026, 11, 15}: "14050824",
		{2000, 1, 1}:   "13781011",
	} {
		assert.Equal(t, want, shamsiDate(time.Date(g[0], time.Month(g[1]), g[2], 12, 0, 0, 0, time.UTC)), fmt.Sprint(g))
	}
	// 2026-11-14 21:00 UTC is 2026-11-15 00:30 in Tehran: the next Shamsi day.
	late := time.Date(2026, 11, 14, 21, 0, 0, 0, time.UTC)
	assert.Equal(t, "14050824", shamsiDate(late.In(tehran)))
	assert.Equal(t, "003000", late.In(tehran).Format("150405"))
}

// TestMockSettle: the mock answer of a settlement is the success answer; it is new on each call.
func TestMockSettle(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Settle) {
		return
	}
	a := f.Settle(settleRequest())
	assert.Equal(t, provider.StatusSuccess, a.Status)
	assert.Equal(t, int32(200), a.Code)
	a.Status = "x"
	assert.Equal(t, provider.StatusSuccess, f.Settle(settleRequest()).Status)
	resp := f.Inquiry(provider.InquiryRequest{})
	assert.Equal(t, &provider.Details{Tel: &provider.TelDetails{ServiceStatus: provider.TelServiceActive}}, resp.Details)
}

// TestServiceStatusInquiry: the serviceStatus text of the full-term call gives the details; an
// unknown text gives "".
func TestServiceStatusInquiry(t *testing.T) {
	answer := func(status string) string {
		in := fmt.Sprintf(`{"result":{"data":{"telStatus":1,"serviceStatus":%q,"billId":2101234560041,"paymentId":35000042,"amount":350000},"status":{"code":null}},"status":{"code":200},"meta":{"transactionId":"t"}}`, status)
		b, _ := json.Marshal(in)
		return `{"responseText":` + string(b) + `}`
	}
	for text, want := range map[string]string{"وصل": provider.TelServiceActive, "قطع یکطرفه": provider.TelServicePartiallyDisconnected, "قطع دوطرفه": provider.TelServiceFullyDisconnected, "x": ""} {
		f := &fakeKarashop{answer: func(string) (int, string) { return 200, answer(text) }}
		a, _ := newAdapter(t, f, 5*time.Second)
		resp, err := ask(a)
		if assert.NoError(t, err, text) && assert.NotNil(t, resp.Details) && assert.NotNil(t, resp.Details.Tel) {
			assert.Equal(t, want, resp.Details.Tel.ServiceStatus, text)
		}
	}
}
