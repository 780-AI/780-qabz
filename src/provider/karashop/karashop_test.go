package karashop

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/common/helpers"
	"qabz/provider"
)

const (
	testCode     = "780"
	testPassword = "secret-pass-123"
	testTel      = "982188776655"
)

// inner builds the inner answer of Karashop (the text of responseText).
func inner(telStatus int, billID, payID, amount string, resultCode string, statusCode int, trace string) string {
	return fmt.Sprintf(`{"result":{"data":{"telStatus":%d,"serviceStatus":"وصل","billId":%s,"paymentId":%s,"amount":%s,"cycleId":3},"status":{"code":%s,"message":null}},"status":{"code":%d,"message":"m"},"meta":{"transactionId":%q}}`,
		telStatus, billID, payID, amount, resultCode, statusCode, trace)
}

// outer wraps an inner answer as Karashop sends it: responseText is a JSON string.
func outer(t *testing.T, in string) string {
	t.Helper()
	text, err := json.Marshal(in)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	return `{"isSuccessful":true,"error":0,"errorDescription":"","responseStatusCode":200,"responseText":` + string(text) + `,"requestDate":"2026-11-14T10:00:00Z","responseDate":"2026-11-14T10:00:01Z","requestId":"r1"}`
}

func success(t *testing.T, billID, payID, amount, trace string) string {
	return outer(t, inner(1, billID, payID, amount, "null", 200, trace))
}

// fakeKarashop is a local Karashop server. answer gives the HTTP status and the body of each service.
type fakeKarashop struct {
	mu     sync.Mutex
	bodies []request
	delay  time.Duration
	answer func(service string) (int, string)
}

func (f *fakeKarashop) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body request
	_ = json.Unmarshal(raw, &body) // test code, not a hot path (conventions § 1)
	f.mu.Lock()
	f.bodies = append(f.bodies, body)
	f.mu.Unlock()
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-r.Context().Done():
			return
		}
	}
	code, out := f.answer(body.Service)
	w.WriteHeader(code)
	_, _ = io.WriteString(w, out)
}

func (f *fakeKarashop) services() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, b := range f.bodies {
		out = append(out, b.Service)
	}
	return out
}

func newAdapter(t *testing.T, f *fakeKarashop, timeout time.Duration) (*Adapter, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	a, err := New(Options{Enabled: true, Code: testCode, Password: testPassword, InquiryURL: srv.URL + "/inquiry", Timeout: timeout, HTTPClient: srv.Client()})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	return a, srv
}

func ask(a *Adapter) (*provider.InquiryResponse, error) {
	return a.Inquiry(context.Background(), provider.InquiryRequest{BillType: provider.BillTypeTel, UserID: "u", Dialer: "989121234567", InquiryID: testTel})
}

// bothAnswer gives the same answer to both services.
func bothAnswer(code int, body string) func(string) (int, string) {
	return func(string) (int, string) { return code, body }
}

// TestInquiryRequestBody: one inquiry is two POST calls with the credential, the number in the
// local form and each service name once.
func TestInquiryRequestBody(t *testing.T) {
	f := &fakeKarashop{answer: bothAnswer(200, success(t, "2101234560040", "35000042", "350000", "tx"))}
	a, _ := newAdapter(t, f, 5*time.Second)
	_, err := ask(a)
	if !assert.NoError(t, err) {
		return
	}
	assert.ElementsMatch(t, []string{"tci-end-getBillAccInfo", "tci-mid-getBillAccInfo"}, f.services())
	for _, b := range f.bodies {
		assert.Equal(t, credential{Code: testCode, Password: testPassword}, b.Credential)
		assert.Equal(t, []parameter{{Name: "telNumber", Value: "2188776655"}}, b.Parameters)
	}
}

// TestInquirySuccess: the full-term and the mid-term call give a final-term and a mid-term inquiry,
// each with the bill ID of its own call; the trace IDs are joined.
func TestInquirySuccess(t *testing.T) {
	f := &fakeKarashop{answer: func(service string) (int, string) {
		if service == serviceMidTerm {
			return 200, success(t, "2101234560040", "12000035", "120000", "tx-mid")
		}
		return 200, success(t, "2101234560041", "35000042", "350000", "tx-end")
	}}
	a, _ := newAdapter(t, f, 5*time.Second)
	resp, err := ask(a)
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	assert.Equal(t, provider.ResponseSuccess, resp.Status)
	assert.Equal(t, int32(200), resp.Code)
	assert.Equal(t, "2101234560041", resp.BillID, "the full-term bill ID")
	assert.Equal(t, "tx-end,tx-mid", resp.ProviderTraceID)
	assert.Equal(t, "تلفن معتبر است", resp.Message)
	assert.Equal(t, []provider.InquiryItem{
		{BillID: "2101234560040", PayID: "12000035", Amount: 120000, InquiryType: provider.InquiryTypeMidterm},
		{BillID: "2101234560041", PayID: "35000042", Amount: 350000, InquiryType: provider.InquiryTypeFinalTerm},
	}, resp.Inquiries)
	assert.Nil(t, resp.Details)
}

// TestInquiryTelStatusMap: telStatus 1, 3 and 4 are a success (4: no debt, a null amount is 0);
// telStatus 2 is a refusal with bill's text.
func TestInquiryTelStatusMap(t *testing.T) {
	for _, c := range []struct {
		telStatus int
		amount    string
		want      uint64
		message   string
	}{
		{1, "5000", 5000, "تلفن معتبر است"},
		{3, "6000", 6000, "تلفن معتبر است اما جز تلفن های خاص است"},
		{4, "null", 0, "تلفن معتبر است اما مشترک بدون بدهی است"},
		{4, "0", 0, "تلفن معتبر است اما مشترک بدون بدهی است"},
	} {
		f := &fakeKarashop{answer: bothAnswer(200, outer(t, inner(c.telStatus, "2101234560040", "null", c.amount, "null", 200, "tx")))}
		a, _ := newAdapter(t, f, 5*time.Second)
		resp, err := ask(a)
		if assert.NoError(t, err, c.telStatus) && assert.NotNil(t, resp) && assert.Len(t, resp.Inquiries, 2) {
			assert.Equal(t, provider.ResponseSuccess, resp.Status)
			assert.Equal(t, c.message, resp.Message)
			assert.Equal(t, []uint64{c.want, c.want}, []uint64{resp.Inquiries[0].Amount, resp.Inquiries[1].Amount})
			assert.Equal(t, "", resp.Inquiries[0].PayID, "a null pay ID is empty")
			assert.Equal(t, "tx", resp.ProviderTraceID, "the same trace ID is not joined")
		}
	}

	f := &fakeKarashop{answer: bothAnswer(200, outer(t, inner(2, "null", "null", "null", "null", 200, "tx2")))}
	a, _ := newAdapter(t, f, 5*time.Second)
	resp, err := ask(a)
	if assert.NoError(t, err) && assert.NotNil(t, resp) {
		assert.Equal(t, []string{provider.ResponseFailed, "تلفن نامعتبر است", "tx2"}, []string{resp.Status, resp.Message, resp.ProviderTraceID})
		assert.Equal(t, int32(2), resp.Code)
		assert.Empty(t, resp.Inquiries)
	}
}

// TestInquiryStatusCodeMap: status code 400 and 404 are a refusal with bill's text; every other
// code that is not a success, and a result status code, is provider down.
func TestInquiryStatusCodeMap(t *testing.T) {
	for code, text := range map[int]string{400: "خطا در داده‌های ورودی وجود دارد", 404: "پاسخی برای درخواست شما یافت نشد"} {
		f := &fakeKarashop{answer: bothAnswer(200, outer(t, inner(0, "null", "null", "null", "null", code, "tx")))}
		a, _ := newAdapter(t, f, 5*time.Second)
		resp, err := ask(a)
		if assert.NoError(t, err, code) && assert.NotNil(t, resp, code) {
			assert.Equal(t, []string{provider.ResponseFailed, text}, []string{resp.Status, resp.Message}, code)
			assert.Equal(t, int32(code), resp.Code)
		}
	}
	for _, code := range []int{200, 202, 401, 403, 410, 422, 429, 499, 500, 502, 503, 504, 520, 999} {
		f := &fakeKarashop{answer: bothAnswer(200, outer(t, inner(0, "null", "null", "null", "null", code, "tx")))}
		a, _ := newAdapter(t, f, 5*time.Second)
		resp, err := ask(a)
		assert.Nil(t, resp, code)
		if assert.ErrorIs(t, err, provider.ErrProviderDown, code) {
			assert.Contains(t, err.Error(), fmt.Sprintf("status code %d", code))
		}
	}
	for _, rc := range []string{"1", "4", "5", "6", "77"} {
		f := &fakeKarashop{answer: bothAnswer(200, outer(t, inner(0, "null", "null", "null", rc, 400, "tx")))}
		a, _ := newAdapter(t, f, 5*time.Second)
		_, err := ask(a)
		if assert.ErrorIs(t, err, provider.ErrProviderDown, rc) {
			assert.Contains(t, err.Error(), "result status code "+rc)
		}
	}
}

// TestInquiryOneCallFails: one call that is down makes the answer down; one refused call makes it
// a refusal; the full term is reported first.
func TestInquiryOneCallFails(t *testing.T) {
	ok := success(t, "2101234560040", "35000042", "350000", "tx")
	refused := outer(t, inner(2, "null", "null", "null", "null", 200, "tx-r"))
	cases := []struct {
		name     string
		mid, end func() (int, string)
		wantDown string // "" : a refusal
	}{
		{"mid down", func() (int, string) { return 500, "x" }, func() (int, string) { return 200, ok }, "mid term"},
		{"end down", func() (int, string) { return 200, ok }, func() (int, string) { return 200, "{" }, "full term"},
		{"both down", func() (int, string) { return 502, "" }, func() (int, string) { return 503, "" }, "full term"},
		{"mid refused", func() (int, string) { return 200, refused }, func() (int, string) { return 200, ok }, ""},
		{"end refused", func() (int, string) { return 200, ok }, func() (int, string) { return 200, refused }, ""},
		{"end down, mid refused", func() (int, string) { return 200, refused }, func() (int, string) { return 500, "" }, "full term"},
	}
	for _, c := range cases {
		f := &fakeKarashop{answer: func(service string) (int, string) {
			if service == serviceMidTerm {
				return c.mid()
			}
			return c.end()
		}}
		a, _ := newAdapter(t, f, 5*time.Second)
		resp, err := ask(a)
		if c.wantDown != "" {
			assert.Nil(t, resp, c.name)
			if assert.ErrorIs(t, err, provider.ErrProviderDown, c.name) {
				assert.True(t, strings.HasPrefix(err.Error(), c.wantDown+":"), c.name+": "+err.Error())
			}
			continue
		}
		if assert.NoError(t, err, c.name) && assert.NotNil(t, resp, c.name) {
			assert.Equal(t, provider.ResponseFailed, resp.Status, c.name)
			assert.Equal(t, "tx-r", resp.ProviderTraceID, c.name)
			assert.Empty(t, resp.Inquiries, c.name)
		}
	}
}

// TestInquiryBrokenAnswers: an unreadable body, a responseText that is not a string, a responseText
// that is not JSON, a bad HTTP status and an amount that is not an integer are provider down. The
// error text never has the password.
func TestInquiryBrokenAnswers(t *testing.T) {
	for name, a := range map[string]func(string) (int, string){
		"not JSON":                bothAnswer(200, "<html>"),
		"responseText missing":    bothAnswer(200, `{"isSuccessful":true}`),
		"responseText an object":  bothAnswer(200, `{"responseText":{"result":{}}}`),
		"responseText not JSON":   bothAnswer(200, `{"responseText":"not json"}`),
		"HTTP 500":                bothAnswer(500, success(t, "1", "2", "3", "t")),
		"HTTP 401":                bothAnswer(401, ""),
		"amount a fraction":       bothAnswer(200, outer(t, inner(1, "1", "2", "12.5", "null", 200, "t"))),
		"amount a string":         bothAnswer(200, outer(t, inner(1, "1", "2", `"12"`, "null", 200, "t"))),
		"amount above int64":      bothAnswer(200, outer(t, inner(1, "1", "2", "99999999999999999999", "null", 200, "t"))),
		"empty inner object":      bothAnswer(200, outer(t, `{}`)),
		"responseText JSON null":  bothAnswer(200, `{"responseText":null}`),
		"responseText JSON array": bothAnswer(200, outer(t, `[1,2]`)),
	} {
		f := &fakeKarashop{answer: a}
		ad, _ := newAdapter(t, f, 5*time.Second)
		resp, err := ask(ad)
		assert.Nil(t, resp, name)
		if assert.ErrorIs(t, err, provider.ErrProviderDown, name) {
			assert.NotContains(t, err.Error(), testPassword, name)
		}
	}
}

// TestInquiryFieldForms: a negative amount is 0; a bill ID or a pay ID as a string of digits is
// kept; another form gives an empty ID; a missing full-term bill ID takes the mid-term one.
func TestInquiryFieldForms(t *testing.T) {
	f := &fakeKarashop{}
	f.answer = func(service string) (int, string) {
		if service == serviceMidTerm {
			return 200, outer(t, inner(1, `"2101234560040"`, `"12000035"`, "-7", "null", 200, "m"))
		}
		return 200, outer(t, inner(1, "null", `"35x"`, "9", "null", 200, "e"))
	}
	a, _ := newAdapter(t, f, 5*time.Second)
	resp, err := ask(a)
	if assert.NoError(t, err) && assert.NotNil(t, resp) && assert.Len(t, resp.Inquiries, 2) {
		assert.Equal(t, "2101234560040", resp.BillID, "the mid-term bill ID when the full-term one is empty")
		assert.Equal(t, provider.InquiryItem{BillID: "2101234560040", PayID: "12000035", Amount: 0, InquiryType: provider.InquiryTypeMidterm}, resp.Inquiries[0])
		assert.Equal(t, provider.InquiryItem{BillID: "", PayID: "", Amount: 9, InquiryType: provider.InquiryTypeFinalTerm}, resp.Inquiries[1])
	}
	for v, want := range map[string]string{`123`: "123", `"0045"`: "0045", `-1`: "", `1.5`: "", `""`: "", `true`: "", `{}`: ""} {
		var got string
		raw := outer(t, inner(1, v, "null", "1", "null", 200, "t"))
		p, err := readAnswer([]byte(raw))
		if assert.NoError(t, err, v) {
			got = p.billID
		}
		assert.Equal(t, want, got, v)
	}
}

// TestInquiryTimeout: a call that takes longer than the timeout is provider down with the deadline
// in the chain; a client that leaves gives context.Canceled in the chain.
func TestInquiryTimeout(t *testing.T) {
	f := &fakeKarashop{delay: 2 * time.Second, answer: bothAnswer(200, success(t, "1", "2", "3", "t"))}
	a, _ := newAdapter(t, f, 100*time.Millisecond)
	start := time.Now()
	resp, err := ask(a)
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Less(t, time.Since(start), time.Second, "the two calls run at the same time, each with the timeout")

	a, _ = newAdapter(t, f, 5*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	_, err = a.Inquiry(ctx, provider.InquiryRequest{BillType: provider.BillTypeTel, InquiryID: testTel})
	assert.ErrorIs(t, err, context.Canceled)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
}

// TestInquiryNotSent: enabled false, an empty credential or URL, and another bill type send nothing.
func TestInquiryNotSent(t *testing.T) {
	f := &fakeKarashop{answer: bothAnswer(200, success(t, "1", "2", "3", "t"))}
	srv := httptest.NewServer(f)
	defer srv.Close()
	for name, o := range map[string]Options{
		"disabled":      {Enabled: false, Code: testCode, Password: testPassword, InquiryURL: srv.URL},
		"no code":       {Enabled: true, Password: testPassword, InquiryURL: srv.URL},
		"no password":   {Enabled: true, Code: testCode, InquiryURL: srv.URL},
		"no URL":        {Enabled: true, Code: testCode, Password: testPassword},
		"malformed URL": {Enabled: true, Code: testCode, Password: testPassword, InquiryURL: "http://[::1"},
	} {
		o.Timeout, o.HTTPClient = time.Second, srv.Client()
		a, err := New(o)
		if !assert.NoError(t, err, name) {
			continue
		}
		assert.Equal(t, o.Enabled, a.Enabled(), name)
		resp, err := ask(a)
		assert.Nil(t, resp, name)
		if assert.ErrorIs(t, err, provider.ErrProviderDown, name) {
			assert.NotContains(t, err.Error(), testPassword, name)
		}
	}
	assert.Empty(t, f.services(), "nothing is sent")

	a, _ := newAdapter(t, f, time.Second)
	for _, bt := range []string{provider.BillTypeWater, provider.BillTypeMobile, "telmidterm", ""} {
		_, err := a.Inquiry(context.Background(), provider.InquiryRequest{BillType: bt, InquiryID: testTel})
		assert.ErrorIs(t, err, provider.ErrNotSupported, bt)
		assert.False(t, a.SupportsInquiry(bt), bt)
	}
	assert.True(t, a.SupportsInquiry(provider.BillTypeTel))
	assert.Empty(t, f.services(), "nothing is sent")
}

func TestNewAndOtherMethods(t *testing.T) {
	_, err := New(Options{Timeout: 0})
	assert.Error(t, err)
	a, err := New(Options{Timeout: time.Second})
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "karashop", a.Name())
	var nilAdapter *Adapter
	assert.False(t, nilAdapter.Enabled())
	_, err = nilAdapter.Inquiry(context.Background(), provider.InquiryRequest{BillType: provider.BillTypeTel})
	assert.Error(t, err)
	_, err = a.Pay(context.Background(), provider.PaymentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Status(context.Background(), provider.StatusRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.GetDocument(context.Background(), provider.DocumentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Settle(context.Background(), provider.SettleRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	// A nil context is a background context.
	_, err = a.Inquiry(nil, provider.InquiryRequest{BillType: provider.BillTypeTel}) //nolint:staticcheck // the nil-context branch
	assert.ErrorIs(t, err, provider.ErrProviderDown)
}

func TestLocalFormAndJoinTrace(t *testing.T) {
	for in, want := range map[string]string{"982188776655": "2188776655", "02188776655": "2188776655", "2188776655": "2188776655", "": "", "0": "", "98": ""} {
		assert.Equal(t, want, localForm(in), in)
	}
	long := strings.Repeat("a", 60)
	for _, c := range [][3]string{
		{"e", "m", "e,m"}, {"e", "e", "e"}, {"e", "", "e"}, {"", "m", "m"}, {"", "", ""}, {long, long + "b", long},
	} {
		assert.Equal(t, c[2], joinTrace(c[0], c[1]), c[0]+"|"+c[1])
	}
	assert.LessOrEqual(t, len(joinTrace(strings.Repeat("x", 49), strings.Repeat("y", 50))), maxTraceLen)
}

// TestMockFixtures: the mock answer is a valid landline bill: the bill ID is valid and of the type
// tel, each pay ID is valid and its amount rule gives the amount. It is new on each call.
func TestMockFixtures(t *testing.T) {
	f := MockFixtures()
	if !assert.NotNil(t, f.Inquiry) {
		return
	}
	resp := f.Inquiry(provider.InquiryRequest{BillType: provider.BillTypeTel, InquiryID: testTel})
	if !assert.NotNil(t, resp) || !assert.Len(t, resp.Inquiries, 2) {
		return
	}
	assert.Equal(t, provider.ResponseSuccess, resp.Status)
	assert.True(t, helpers.ValidBillID(resp.BillID))
	bt, ok := helpers.BillTypeOfBillID(resp.BillID)
	assert.True(t, ok)
	assert.Equal(t, helpers.BillTypeTel, bt)
	for _, in := range resp.Inquiries {
		assert.True(t, helpers.ValidPayID(in.PayID), in.PayID)
		amount, ok := helpers.PayIDAmount(in.PayID)
		assert.True(t, ok, in.PayID)
		assert.Equal(t, in.Amount, amount, in.PayID)
	}
	assert.Equal(t, []string{provider.InquiryTypeMidterm, provider.InquiryTypeFinalTerm}, []string{resp.Inquiries[0].InquiryType, resp.Inquiries[1].InquiryType})
	resp.Inquiries[0].Amount = 1
	again := f.Inquiry(provider.InquiryRequest{})
	if assert.NotNil(t, again) && assert.Len(t, again.Inquiries, 2) {
		assert.Equal(t, uint64(MockMidTermAmount), again.Inquiries[0].Amount, "a new value on each call")
	}
}

// nilDoer answers (nil, nil): a broken client.
type nilDoer struct{}

func (nilDoer) Do(*http.Request) (*http.Response, error) { return nil, nil }

// TestInquiryEmptyAnswer: a client that gives no response and no error is provider down, not a panic.
func TestInquiryEmptyAnswer(t *testing.T) {
	a, err := New(Options{Enabled: true, Code: testCode, Password: testPassword, InquiryURL: "http://127.0.0.1:1/k", Timeout: time.Second, HTTPClient: nilDoer{}})
	if !assert.NoError(t, err) {
		return
	}
	resp, err := ask(a)
	assert.Nil(t, resp)
	if assert.ErrorIs(t, err, provider.ErrProviderDown) {
		assert.Contains(t, err.Error(), "empty answer")
	}
}

// TestInquiryOneFailureStopsTheOther: a call that fails stops the other call at once (no wait for
// the timeout), and the answer names the call that failed first.
func TestInquiryOneFailureStopsTheOther(t *testing.T) {
	for _, slow := range []string{serviceMidTerm, serviceFullTerm} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body request
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			if body.Service == slow {
				select {
				case <-time.After(5 * time.Second):
				case <-r.Context().Done():
					return
				}
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		a, err := New(Options{Enabled: true, Code: testCode, Password: testPassword, InquiryURL: srv.URL, Timeout: 10 * time.Second, HTTPClient: srv.Client()})
		if !assert.NoError(t, err) {
			srv.Close()
			continue
		}
		start := time.Now()
		_, err = ask(a)
		assert.Less(t, time.Since(start), 2*time.Second, slow+": the slow call is stopped")
		if assert.ErrorIs(t, err, provider.ErrProviderDown, slow) {
			assert.Contains(t, err.Error(), "HTTP status 500", slow+": the cause is the call that failed")
			assert.NotErrorIs(t, err, context.Canceled, slow)
		}
		srv.Close()
	}
}
