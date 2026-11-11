package tavanir

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/provider"
)

// No test here reaches Tavanir (SAAPA): every endpoint is a local httptest server or a fake Doer.

const (
	testUser     = "test-user"
	testPassword = "test-password-not-real"
	testToken    = "test-token-not-real"
)

// goodData is the data object of a good inquiry answer, with numbers in the flexible fields.
const goodData = `{"ispaid":false,"bill_identifier":1234567890125,"payment_identifier":1800012345,"total_bill_debt":18000000,` +
	`"payment_dead_line":"2030-01-02T10:20:30+03:30","company_name":"co","city":"region-1","phase":1,"tariff_type":11,"customer_type":1,` +
	`"customer_name":"Ali","customer_family":"Ahmadi","service_add":"addr","serial_number":12345678,"service_post_code":1234567890}`

func goodAnswer(data string) string {
	return `{"timeStamp":"x","status":200,"message":"ok","data":` + data + `,"error":null,"traceno":"t-1"}`
}

// fake is a local Tavanir: /auth and /branch. It records what it gets.
type fake struct {
	srv *httptest.Server

	mu          sync.Mutex
	logins      int
	loginBodies [][]byte
	branches    int
	branchBody  []byte
	branchAuth  []string
	ctypes      []string
	methods     []string

	// tokens are the tokens of the logins, in order; the last one repeats.
	tokens []string
	// loginStatus and loginBody, when set, replace the good login answer.
	loginStatus int
	loginBody   string
	// loginDelay holds each login.
	loginDelay time.Duration
	// branch answers an inquiry call: the HTTP status and the body. Nil: 200 and goodAnswer(goodData).
	branch func(n int, auth string) (int, string)
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{tokens: []string{testToken}}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.logins++
		n := f.logins
		f.loginBodies = append(f.loginBodies, body)
		f.ctypes = append(f.ctypes, r.Header.Get("Content-Type"))
		f.methods = append(f.methods, r.Method)
		delay, status, answer := f.loginDelay, f.loginStatus, f.loginBody
		token := f.tokens[min(n, len(f.tokens))-1]
		f.mu.Unlock()
		time.Sleep(delay)
		if status == 0 {
			status = 200
		}
		if answer == "" {
			answer = `{"status":200,"message":"ok","data":{"id":1,"username":"` + testUser + `","token":"` + token + `"},"error":null}`
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	})
	mux.HandleFunc("/branch", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		auth := r.Header.Get("Authorization")
		f.mu.Lock()
		f.branches++
		n := f.branches
		f.branchBody = body
		f.branchAuth = append(f.branchAuth, auth)
		f.ctypes = append(f.ctypes, r.Header.Get("Content-Type"))
		f.methods = append(f.methods, r.Method)
		branch := f.branch
		f.mu.Unlock()
		status, answer := 200, goodAnswer(goodData)
		if branch != nil {
			status, answer = branch(n, auth)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, answer)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) counts() (logins, branches int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.logins, f.branches
}

// options are the options of an adapter that is on and uses the fake.
func (f *fake) options() Options {
	return Options{Enabled: true, AuthEndpoint: f.srv.URL + "/auth", BasicBranchEndpoint: f.srv.URL + "/branch",
		Username: testUser, Password: testPassword}
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
	return a
}

var powerReq = provider.InquiryRequest{BillType: provider.BillTypeElectricity, UserID: "u", Dialer: "09120000000", InquiryID: "1234567890125"}

func TestNewRejectsBadTimeout(t *testing.T) {
	_, err := New(Options{})
	assert.Error(t, err)
	_, err = New(Options{Timeout: -time.Second})
	assert.Error(t, err)
}

func TestNameAndNotSupported(t *testing.T) {
	a := newAdapter(t, Options{})
	assert.Equal(t, "tavanir", a.Name())
	assert.False(t, a.Enabled())
	_, err := a.Pay(context.Background(), provider.PaymentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Status(context.Background(), provider.StatusRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.GetDocument(context.Background(), provider.DocumentRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	_, err = a.Settle(context.Background(), provider.SettleRequest{})
	assert.ErrorIs(t, err, provider.ErrNotSupported)
	for _, billType := range []string{provider.BillTypeWater, "gas", ""} {
		_, err = a.Inquiry(context.Background(), provider.InquiryRequest{BillType: billType})
		assert.ErrorIs(t, err, provider.ErrNotSupported, billType)
	}
	var nilAdapter *Adapter
	_, err = nilAdapter.Inquiry(context.Background(), powerReq)
	assert.Error(t, err)
	assert.False(t, nilAdapter.Enabled())
	_, err = (&Adapter{}).Inquiry(context.Background(), powerReq)
	assert.Error(t, err, "an adapter not made with New")
}

// TestInquiryOnTheWire: one login (POST of JSON with the user name and the password), then one
// inquiry (POST of JSON with the bill ID and the mobile number, the Bearer token), and the success
// answer with the details.
func TestInquiryOnTheWire(t *testing.T) {
	f := newFake(t)
	a := newAdapter(t, f.options())
	assert.True(t, a.Enabled())
	resp, err := a.Inquiry(nil, powerReq) //nolint:staticcheck // a nil context is accepted
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	logins, branches := f.counts()
	assert.Equal(t, []int{1, 1}, []int{logins, branches})
	assert.Equal(t, []string{http.MethodPost, http.MethodPost}, f.methods)
	assert.Equal(t, []string{"application/json", "application/json"}, f.ctypes)
	var login map[string]string
	if assert.Len(t, f.loginBodies, 1) && assert.NoError(t, json.Unmarshal(f.loginBodies[0], &login)) {
		assert.Equal(t, map[string]string{"username": testUser, "password": testPassword}, login)
	}
	var branch map[string]string
	if assert.NoError(t, json.Unmarshal(f.branchBody, &branch)) {
		assert.Equal(t, map[string]string{"bill_identifier": "1234567890125", "MobileNo": "09120000000"}, branch)
	}
	assert.Equal(t, []string{"Bearer " + testToken}, f.branchAuth)

	assert.Equal(t, provider.ResponseSuccess, resp.Status)
	assert.EqualValues(t, 200, resp.Code)
	assert.Equal(t, successMessage, resp.Message)
	assert.Equal(t, "", resp.ProviderTraceID)
	assert.Equal(t, "1234567890125", resp.BillID)
	if assert.Len(t, resp.Inquiries, 1) {
		assert.Equal(t, provider.InquiryItem{PayID: "1800012345", Amount: 18_000_000, PaymentDeadline: "2030-01-02", InquiryType: "regular"}, resp.Inquiries[0])
	}
	if assert.NotNil(t, resp.Details) && assert.NotNil(t, resp.Details.Electricity) {
		assert.Equal(t, provider.ElectricityDetails{
			Phase: "1", CompanyName: "co", TariffType: "11", CustomerType: 1, SerialNumber: "12345678", ServiceAddress: "addr",
			ServicePostCode: "1234567890", CustomerName: "Ali Ahmadi", City: "co", Region: "region-1",
		}, *resp.Details.Electricity)
	}
}

// TestInquirySuccessShapes: the flexible fields are numbers or strings; a missing or null field
// gives an empty value; the deadline is cut to the date.
func TestInquirySuccessShapes(t *testing.T) {
	type want struct {
		billID, payID, deadline, serial, post, name string
		amount                                      uint64
	}
	cases := []struct {
		name string
		data string
		want want
	}{
		{"strings", `{"bill_identifier":"0012345","payment_identifier":"0018000","total_bill_debt":5,"payment_dead_line":"2031-02-03T00:00:00Z","serial_number":"A-12","service_post_code":"0012","customer_name":"A","customer_family":"B"}`,
			want{"12345", "18000", "2031-02-03", "A-12", "0012", "A B", 5}},
		{"numbers with exponent and fraction", `{"bill_identifier":1.234567890125e+12,"payment_identifier":18000.0,"total_bill_debt":7.0,"payment_dead_line":"2031-02-03T04:05:06","serial_number":77,"service_post_code":1234567890}`,
			want{"1234567890125", "18000", "2031-02-03", "77", "1234567890", "", 7}},
		{"missing and null", `{"bill_identifier":null,"payment_identifier":null,"serial_number":null,"service_post_code":null,"payment_dead_line":"tomorrow","customer_name":"Only"}`,
			want{"1234567890125", "", "", "", "", "Only", 0}},
		{"not a number text stays", `{"bill_identifier":"abc","payment_identifier":"","total_bill_debt":-5,"service_post_code":true}`,
			want{"abc", "", "", "", "", "", 0}},
		{"empty object", `{}`, want{"1234567890125", "", "", "", "", "", 0}},
	}
	for _, c := range cases {
		f := newFake(t)
		f.branch = func(int, string) (int, string) { return 200, goodAnswer(c.data) }
		resp, err := newAdapter(t, f.options()).Inquiry(context.Background(), powerReq)
		if !assert.NoError(t, err, c.name) || !assert.NotNil(t, resp, c.name) || !assert.Len(t, resp.Inquiries, 1, c.name) ||
			!assert.NotNil(t, resp.Details, c.name) || !assert.NotNil(t, resp.Details.Electricity, c.name) {
			continue
		}
		d := resp.Details.Electricity
		assert.Equal(t, c.want, want{resp.BillID, resp.Inquiries[0].PayID, resp.Inquiries[0].PaymentDeadline, d.SerialNumber, d.ServicePostCode, d.CustomerName, resp.Inquiries[0].Amount}, c.name)
		assert.Equal(t, provider.ResponseSuccess, resp.Status, c.name)
	}
	// An empty error array is "no error item": success.
	f := newFake(t)
	f.branch = func(int, string) (int, string) {
		return 200, `{"status":200,"data":` + goodData + `,"error":[]}`
	}
	resp, err := newAdapter(t, f.options()).Inquiry(context.Background(), powerReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) {
		assert.Equal(t, provider.ResponseSuccess, resp.Status)
	}
}

// TestInquiryStatusCodes: each class of the Tavanir status. A refused code is a failed answer with
// its text and code; every other code is provider down.
func TestInquiryStatusCodes(t *testing.T) {
	for code, msg := range refused {
		f := newFake(t)
		f.branch = func(int, string) (int, string) {
			return 200, `{"status":` + strconv.Itoa(code) + `,"message":"m","data":null,"error":[{"errorCode":1,"errorMsg":"e"}]}`
		}
		resp, err := newAdapter(t, f.options()).Inquiry(context.Background(), powerReq)
		if !assert.NoError(t, err, code) || !assert.NotNil(t, resp, code) {
			continue
		}
		assert.Equal(t, provider.InquiryResponse{Status: provider.ResponseFailed, Code: int32(code), Message: msg}, *resp, code)
	}
	assert.Len(t, refused, 19)
	assert.Equal(t, "فیلد شناسه قبض نامعتبر است", refused[400])
	assert.Equal(t, "شناسه قبض متعلق به برق نمی باشد", refused[507])

	downCodes := []int{500, 502, 506, 508, 509, 512, 513, 517, 524, 527, 528 /* the down codes of the tavanir service */, 0, 201, 404, 529, 999, -1}
	for _, code := range downCodes {
		_, isRefused := refused[code]
		assert.False(t, isRefused, code)
		f := newFake(t)
		f.branch = func(int, string) (int, string) {
			return 200, `{"status":` + strconv.Itoa(code) + `,"message":"m","data":` + goodData + `}`
		}
		resp, err := newAdapter(t, f.options()).Inquiry(context.Background(), powerReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown, code)
		assert.Nil(t, resp, code)
		if err != nil {
			assert.Contains(t, err.Error(), "status "+strconv.Itoa(code), code)
		}
	}
}

// TestInquiryBadAnswers: answers that cannot be used are provider down.
func TestInquiryBadAnswers(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"HTTP 500", 500, goodAnswer(goodData)},
		{"HTTP 403", 403, goodAnswer(goodData)},
		{"bad JSON", 200, `{"status":200,`},
		{"empty body", 200, ``},
		{"not an object", 200, `[1,2]`},
		{"no status", 200, `{"data":` + goodData + `}`},
		{"status 200 with an error item", 200, `{"status":200,"data":` + goodData + `,"error":[{"errorCode":5,"errorMsg":"x"}]}`},
		{"data is not an object", 200, `{"status":200,"data":"x"}`},
		{"no data", 200, `{"status":200}`},
	}
	for _, c := range cases {
		f := newFake(t)
		f.branch = func(int, string) (int, string) { return c.status, c.body }
		resp, err := newAdapter(t, f.options()).Inquiry(context.Background(), powerReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
		assert.Nil(t, resp, c.name)
		logins, branches := f.counts()
		assert.Equal(t, []int{1, 1}, []int{logins, branches}, c.name+": no retry for an answer that is not 401")
	}
}

// TestTokenReuseAndNewLoginAfter401: two calls use one login. A 401 drops the token: the call logs
// in again and is sent one more time with the new token. Positive control: with a branch that never
// answers 401 there is no second login (the first part), so the second login comes from the 401.
func TestTokenReuseAndNewLoginAfter401(t *testing.T) {
	f := newFake(t)
	f.tokens = []string{"token-1", "token-2"}
	a := newAdapter(t, f.options())
	for range 2 {
		_, err := a.Inquiry(context.Background(), powerReq)
		assert.NoError(t, err)
	}
	logins, branches := f.counts()
	assert.Equal(t, []int{1, 2}, []int{logins, branches}, "the token is reused: one login for two calls")
	assert.Equal(t, []string{"Bearer token-1", "Bearer token-1"}, f.branchAuth)

	// Tavanir now rejects token-1 (it expired).
	f.mu.Lock()
	f.branch = func(_ int, auth string) (int, string) {
		if auth == "Bearer token-1" {
			return 401, ``
		}
		return 200, goodAnswer(goodData)
	}
	f.mu.Unlock()
	resp, err := a.Inquiry(context.Background(), powerReq)
	if assert.NoError(t, err) && assert.NotNil(t, resp) {
		assert.Equal(t, provider.ResponseSuccess, resp.Status)
	}
	logins, branches = f.counts()
	assert.Equal(t, []int{2, 4}, []int{logins, branches}, "one new login and one more send")
	assert.Equal(t, []string{"Bearer token-1", "Bearer token-1", "Bearer token-1", "Bearer token-2"}, f.branchAuth)
	assert.Equal(t, "token-2", a.currentToken())

	_, err = a.Inquiry(context.Background(), powerReq)
	assert.NoError(t, err)
	logins, _ = f.counts()
	assert.Equal(t, 2, logins, "the new token is reused")
}

// TestSecond401IsProviderDown: when the new token is rejected too, the call stops after two sends
// and the token is dropped.
func TestSecond401IsProviderDown(t *testing.T) {
	f := newFake(t)
	f.branch = func(int, string) (int, string) { return 401, `{"status":401}` }
	a := newAdapter(t, f.options())
	resp, err := a.Inquiry(context.Background(), powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.Nil(t, resp)
	logins, branches := f.counts()
	assert.Equal(t, []int{2, 2}, []int{logins, branches})
	assert.Equal(t, "", a.currentToken())
}

// TestDropTokenOnlyDropsTheUsedToken: a late 401 of an old token does not remove a newer token.
func TestDropTokenOnlyDropsTheUsedToken(t *testing.T) {
	a := newAdapter(t, Options{})
	a.token = "new"
	a.dropToken("old")
	assert.Equal(t, "new", a.currentToken())
	a.dropToken("new")
	assert.Equal(t, "", a.currentToken())
}

// TestConcurrentCallsMakeOneLogin: many calls at once, with no token, make one login. Then all of
// them get a 401 for that token at once: still one new login (no stampede).
func TestConcurrentCallsMakeOneLogin(t *testing.T) {
	const callers = 40
	f := newFake(t)
	f.tokens = []string{"token-1", "token-2"}
	f.loginDelay = 50 * time.Millisecond
	a := newAdapter(t, f.options())
	run := func() (failed int32) {
		var wg sync.WaitGroup
		var bad atomic.Int32
		for range callers {
			wg.Go(func() {
				if resp, err := a.Inquiry(context.Background(), powerReq); err != nil || resp == nil || resp.Status != provider.ResponseSuccess {
					bad.Add(1)
				}
			})
		}
		wg.Wait()
		return bad.Load()
	}
	assert.EqualValues(t, 0, run())
	logins, branches := f.counts()
	assert.Equal(t, []int{1, callers}, []int{logins, branches}, "one login for all callers")

	f.mu.Lock()
	f.branch = func(_ int, auth string) (int, string) {
		if auth == "Bearer token-1" {
			return 401, ``
		}
		return 200, goodAnswer(goodData)
	}
	f.mu.Unlock()
	assert.EqualValues(t, 0, run())
	logins, branches = f.counts()
	assert.Equal(t, 2, logins, "one new login after the 401 of all callers")
	// A caller that starts after the first 401 already has the new token: one send, not two.
	assert.Greater(t, branches, 2*callers, "a rejected caller is sent one more time")
	assert.LessOrEqual(t, branches, 3*callers)
}

// TestLoginFailures: a login that is not status 200 with a token is provider down, sends no
// inquiry, keeps no token, and the next call logs in again.
func TestLoginFailures(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"status not 200", 200, `{"status":401,"message":"bad user","data":null,"error":[{"errorCode":1,"errorMsg":"x"}]}`},
		{"empty token", 200, `{"status":200,"data":{"token":""}}`},
		{"no data", 200, `{"status":200}`},
		{"HTTP 401", 401, `{}`},
		{"HTTP 500", 500, `{"status":200,"data":{"token":"` + testToken + `"}}`},
		{"bad JSON", 200, `{"status":200,"data":{"token":"` + testToken + `"`},
	}
	for _, c := range cases {
		f := newFake(t)
		f.loginStatus, f.loginBody = c.status, c.body
		a := newAdapter(t, f.options())
		for range 2 {
			resp, err := a.Inquiry(context.Background(), powerReq)
			assert.ErrorIs(t, err, provider.ErrProviderDown, c.name)
			assert.Nil(t, resp, c.name)
			if err != nil {
				assert.NotContains(t, err.Error(), testToken, c.name)
				assert.NotContains(t, err.Error(), testPassword, c.name)
			}
		}
		logins, branches := f.counts()
		assert.Equal(t, []int{2, 0}, []int{logins, branches}, c.name+": a failed login is not kept and no inquiry is sent")
		assert.Equal(t, "", a.currentToken(), c.name)
	}
}

// TestDisabledAndMissingConfig: with the switch off, or a required value empty, no call is sent.
func TestDisabledAndMissingConfig(t *testing.T) {
	f := newFake(t)
	cases := map[string]func(o *Options){
		"disabled":           func(o *Options) { o.Enabled = false },
		"no username":        func(o *Options) { o.Username = "" },
		"no password":        func(o *Options) { o.Password = "" },
		"no auth endpoint":   func(o *Options) { o.AuthEndpoint = "" },
		"no branch endpoint": func(o *Options) { o.BasicBranchEndpoint = "" },
	}
	for name, change := range cases {
		o := f.options()
		change(&o)
		resp, err := newAdapter(t, o).Inquiry(context.Background(), powerReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown, name)
		assert.Nil(t, resp, name)
	}
	logins, branches := f.counts()
	assert.Equal(t, []int{0, 0}, []int{logins, branches}, "nothing is sent")
	// Positive control: the same fake and options, unchanged, do send.
	_, err := newAdapter(t, f.options()).Inquiry(context.Background(), powerReq)
	assert.NoError(t, err)
	logins, branches = f.counts()
	assert.Equal(t, []int{1, 1}, []int{logins, branches})
}

// TestTimeoutAndCancel: a call that is slower than the timeout, and a call whose context is
// canceled, are provider down with the cause in the chain.
func TestTimeoutAndCancel(t *testing.T) {
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(slow.Close)
	t.Cleanup(func() { close(release) })

	// The login is slow.
	a := newAdapter(t, Options{Enabled: true, AuthEndpoint: slow.URL, BasicBranchEndpoint: slow.URL, Username: testUser, Password: testPassword, Timeout: 50 * time.Millisecond})
	_, err := a.Inquiry(context.Background(), powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// The inquiry is slow (the login is good).
	f := newFake(t)
	o := f.options()
	o.BasicBranchEndpoint, o.Timeout = slow.URL, 50*time.Millisecond
	_, err = newAdapter(t, o).Inquiry(context.Background(), powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// A canceled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = newAdapter(t, f.options()).Inquiry(ctx, powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestWaiterStopsWithItsContext: a caller that waits for the login of another caller stops when
// its own context ends, and the login of the first caller is not disturbed.
func TestWaiterStopsWithItsContext(t *testing.T) {
	f := newFake(t)
	f.loginDelay = 300 * time.Millisecond
	a := newAdapter(t, f.options())
	first := make(chan error, 1)
	go func() {
		_, err := a.Inquiry(context.Background(), powerReq)
		first <- err
	}()
	assert.Eventually(t, func() bool { logins, _ := f.counts(); return logins == 1 }, 2*time.Second, 5*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := a.Inquiry(ctx, powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NoError(t, <-first)
	logins, branches := f.counts()
	assert.Equal(t, []int{1, 1}, []int{logins, branches}, "the waiter sent nothing")
}

// doer is a fake Doer.
type doer func(*http.Request) (*http.Response, error)

func (d doer) Do(r *http.Request) (*http.Response, error) { return d(r) }

type badBody struct{}

func (badBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (badBody) Close() error             { return nil }

// TestTransportFailures: a transport error, a nil answer, a body that cannot be read and an
// endpoint that is not a URL are provider down; no error has the password or the token.
func TestTransportFailures(t *testing.T) {
	cases := map[string]doer{
		"transport error": func(*http.Request) (*http.Response, error) { return nil, errors.New("connection refused") },
		"nil answer":      func(*http.Request) (*http.Response, error) { return nil, nil },
		"unreadable body": func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: badBody{}}, nil
		},
	}
	for name, d := range cases {
		a := newAdapter(t, Options{Enabled: true, AuthEndpoint: "http://tavanir.test/auth", BasicBranchEndpoint: "http://tavanir.test/branch",
			Username: testUser, Password: testPassword, HTTPClient: d})
		resp, err := a.Inquiry(context.Background(), powerReq)
		assert.ErrorIs(t, err, provider.ErrProviderDown, name)
		assert.Nil(t, resp, name)
		if err != nil {
			assert.NotContains(t, err.Error(), testPassword, name)
		}
	}
	a := newAdapter(t, Options{Enabled: true, AuthEndpoint: "http://bad url\x7f", BasicBranchEndpoint: "http://x", Username: testUser, Password: testPassword})
	_, err := a.Inquiry(context.Background(), powerReq)
	assert.ErrorIs(t, err, provider.ErrProviderDown)

	// The login is good, then the inquiry fails in the transport: the error has no token.
	f := newFake(t)
	o := f.options()
	client := f.srv.Client()
	o.HTTPClient = doer(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, "/branch") {
			return nil, errors.New("reset by peer")
		}
		return client.Do(r)
	})
	_, err = newAdapter(t, o).Inquiry(context.Background(), powerReq)
	if assert.ErrorIs(t, err, provider.ErrProviderDown) {
		assert.NotContains(t, err.Error(), testToken)
		assert.NotContains(t, err.Error(), testPassword)
		assert.Contains(t, err.Error(), "reset by peer")
	}
}

// TestHelpers: integerText and dateOnly.
func TestHelpers(t *testing.T) {
	for in, want := range map[string]string{"": "", "12": "12", "0012": "12", "1.2e+3": "1200", "1200.0": "1200", "abc": "abc", "99999999999999999999": "99999999999999999999"} {
		assert.Equal(t, want, integerText(in), in)
	}
	for in, want := range map[string]string{"2030-01-02T10:20:30Z": "2030-01-02", "2030-01-02T10:20:30.5+03:30": "2030-01-02", "2030-01-02T10:20:30": "2030-01-02", "2030-01-02": "", "": "", "x": ""} {
		assert.Equal(t, want, dateOnly(in), in)
	}
	assert.Equal(t, "", flexible(nil))
}

// TestMockFixtures: the mock answers the bill of the request with the fixed inquiry and details.
func TestMockFixtures(t *testing.T) {
	fx := MockFixtures()
	if !assert.NotNil(t, fx.Inquiry) {
		return
	}
	resp := fx.Inquiry(powerReq)
	if !assert.NotNil(t, resp) || !assert.Len(t, resp.Inquiries, 1) || !assert.NotNil(t, resp.Details) || !assert.NotNil(t, resp.Details.Electricity) {
		return
	}
	assert.Equal(t, []any{provider.ResponseSuccess, int32(200), "", powerReq.InquiryID}, []any{resp.Status, resp.Code, resp.ProviderTraceID, resp.BillID})
	assert.Equal(t, provider.InquiryItem{PayID: MockPayID, Amount: MockAmount, PaymentDeadline: MockDeadline, InquiryType: "regular"}, resp.Inquiries[0])
	assert.Equal(t, provider.ElectricityDetails{Phase: MockPhase, CompanyName: MockCompanyName, TariffType: MockTariffType, CustomerType: MockCustomerType,
		SerialNumber: MockSerialNumber, ServiceAddress: MockServiceAddress, ServicePostCode: MockServicePostCode, CustomerName: MockCustomerName,
		City: MockCompanyName, Region: MockRegion}, *resp.Details.Electricity)
}

// SupportsInquiry gives true only for the bill types that Inquiry answers (task 036); Inquiry of
// any other bill type is ErrNotSupported. A nil adapter answers the same.
func TestSupportsInquiry(t *testing.T) {
	want := map[string]bool{
		provider.BillTypeWater:       false,
		provider.BillTypeElectricity: true,
		provider.BillTypeGas:         false,
		provider.BillTypeIrancell:    false,
		provider.BillTypeMobile:      false,
		"tel":                        false,
		"mobilemidterm":              false,
		"":                           false,
	}
	var nilAdapter *Adapter
	for billType, ok := range want {
		assert.Equal(t, ok, (&Adapter{}).SupportsInquiry(billType), billType)
		assert.Equal(t, ok, nilAdapter.SupportsInquiry(billType), billType)
	}
}
