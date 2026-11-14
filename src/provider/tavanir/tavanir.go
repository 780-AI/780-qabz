// Package tavanir is the adapter of the Tavanir provider (part of Qabz, system.html). It is a client
// of the Tavanir (SAAPA) endpoints of providers.external.tavanir over HTTPS (task 034): it logs in with the
// user name and the password, keeps the token in memory, and asks the basic branch endpoint for the
// inquiry of an electricity bill. It calls no other service: the former gRPC client of the tavanir
// microservice is removed. The flow and the answer mapping copy that microservice
// (client/tavanir, service/tavanir/billInquiry.go, translator.go).
//
// Only Inquiry of an electricity bill is implemented; every other method and bill type returns
// provider.ErrNotSupported. There is no circuit breaker yet (operator decision of task 034: it is
// added to every provider at the end of the project).
//
// No test sends a call to Tavanir. Tests use a local httptest server.
package tavanir

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/valyala/fastjson"

	"qabz/provider"
)

// Name is the providers.name of Tavanir.
const Name = "tavanir"

// inquiryTypeRegular is the inquiry_type of an electricity inquiry (bill: entity.RegularInquiryType).
const inquiryTypeRegular = "regular"

// successMessage is the message of an answer with inquiries (bill: "the operation was done successfully").
const successMessage = "عملیات با موفقیت انجام شد"

// maxAnswerBytes limits the body that the adapter reads from Tavanir.
const maxAnswerBytes = 1 << 20

// statusOK is the status of a good answer of Tavanir (login and inquiry).
const statusOK = 200

// The messages for the user of the tavanir service (pkg/errorHandler/errors.go).
const (
	msgInvalidParameter = "خطای داخلی رخ داده است"
)

// refused are the status codes of Tavanir that give a failed answer (no inquiry) with a message for
// the user (the tavanir service: tavanirInquiryErrors in service/tavanir/translator.go). Every
// other code that is not statusOK is provider down: 500, 502, 506, 508, 509, 512, 513, 517, 524,
// 527 and 528 (the tavanir service answers them with its "provider is down" or "service is
// updating" text) and every code that the table does not have.
var refused = map[int]string{
	400: "فیلد شناسه قبض نامعتبر است",
	501: msgInvalidParameter,
	503: "کد ملی نامعتبر است",
	504: "شماره تلفن نا معتبر است",
	505: msgInvalidParameter,
	507: "شناسه قبض متعلق به برق نمی باشد",
	510: msgInvalidParameter,
	511: msgInvalidParameter,
	514: "کلمه عبور جدید و تایید کلمه عبور یکسان نمی باشد",
	515: msgInvalidParameter,
	516: "پاسخ یادآوری کلمه عبور صحیح نمی باشد",
	518: msgInvalidParameter,
	519: msgInvalidParameter,
	520: msgInvalidParameter,
	521: msgInvalidParameter,
	522: "کد ارسال شده اشتباه است",
	523: "اشکال در اتصال به سرور پیامک",
	525: "کد تایید معتبر نیست",
	526: "کد پیگیری نا معتبر است",
}

// Doer is the part of *http.Client that the adapter uses. Tests give a fake or a client of an
// httptest server.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Options configure New.
type Options struct {
	// Enabled is providers.external.tavanir.enabled. False: no call is sent; every Inquiry fails as provider
	// down.
	Enabled bool
	// AuthEndpoint is providers.external.tavanir.auth_endpoint (the login). Empty: every Inquiry fails as
	// provider down.
	AuthEndpoint string
	// BasicBranchEndpoint is providers.external.tavanir.basic_branch_endpoint (the inquiry). Empty: the same.
	BasicBranchEndpoint string
	// Username is providers.external.tavanir.username. Empty: the same.
	Username string
	// Password is providers.external.tavanir.password (secret). Empty: the same.
	Password string
	// Timeout limits each HTTP call (the login and the inquiry each). Required, positive.
	Timeout time.Duration
	// HTTPClient sends the requests. Nil: a client with Timeout (the system TLS roots; the tavanir
	// service skips the certificate check, this adapter does not).
	HTTPClient Doer
}

// Adapter implements provider.ProviderInterface for Tavanir. It is safe for concurrent use.
type Adapter struct {
	enabled        bool
	authEndpoint   string
	branchEndpoint string
	username       string
	password       string
	timeout        time.Duration
	client         Doer

	// login is the login lock (capacity 1): one login at a time. It is a channel, not a mutex, so a
	// caller that waits for the login of another caller stops when its context ends.
	login chan struct{}
	// mu guards token.
	mu sync.Mutex
	// token is the token of the last good login. Empty: the next call logs in.
	token string
}

var _ provider.ProviderInterface = (*Adapter)(nil)

// New returns the adapter. It sends no call: the first Inquiry logs in.
func New(opts Options) (*Adapter, error) {
	if opts.Timeout <= 0 {
		return nil, errors.New("tavanir: timeout must be positive")
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: opts.Timeout}
	}
	return &Adapter{
		enabled:        opts.Enabled,
		authEndpoint:   opts.AuthEndpoint,
		branchEndpoint: opts.BasicBranchEndpoint,
		username:       opts.Username,
		password:       opts.Password,
		timeout:        opts.Timeout,
		client:         client,
		login:          make(chan struct{}, 1),
	}, nil
}

// Name returns Name.
func (a *Adapter) Name() string { return Name }

// SupportsInquiry reports whether Inquiry answers the bill type: only the electricity bill (task 036).
func (a *Adapter) SupportsInquiry(billType string) bool {
	return billType == provider.BillTypeElectricity
}

// Enabled reports providers.external.tavanir.enabled.
func (a *Adapter) Enabled() bool { return a != nil && a.enabled }

// loginRequest is the body of the login (the tavanir service: entity.AuthenticationReq).
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// branchRequest is the body of the inquiry (the tavanir service: entity.Request).
type branchRequest struct {
	BillIdentifier string `json:"bill_identifier"`
	MobileNo       string `json:"MobileNo"`
}

// parsers holds the fastjson parsers of the answers. Reading the answer of Tavanir is on a hot path
// (every inquiry that the database cannot answer), so it uses fastjson (conventions § 1); the
// requests are written with encoding/json/v2.
var parsers fastjson.ParserPool

// errUnauthorized is the result of a data call that Tavanir answered with HTTP 401: the token is no
// longer valid.
var errUnauthorized = errors.New("tavanir: HTTP status 401")

// down returns an error that wraps provider.ErrProviderDown.
func down(format string, args ...any) error {
	return fmt.Errorf("tavanir: %w: %s", provider.ErrProviderDown, fmt.Sprintf(format, args...))
}

// downCause is down with the cause kept in the chain (so the flow sees context.Canceled).
func downCause(what string, cause error) error {
	return fmt.Errorf("tavanir: %w: %s: %w", provider.ErrProviderDown, what, cause)
}

// Inquiry asks Tavanir for the inquiry of an electricity bill (the tavanir service:
// GetBasicBranchData and prepareResponse). The answer rules:
//   - status 200 with no error item: success with one inquiry (the pay ID, the debt, the deadline
//     cut to yyyy-mm-dd), the bill ID and the details.
//   - a status of refused: a failed answer with no inquiry, the code and a message for the user.
//   - every other status, a transport error, a timeout, an HTTP status other than 200, an
//     unreadable body or a failed login: an error that wraps provider.ErrProviderDown.
//
// The token of the login is reused. An HTTP 401 of the inquiry drops the token (only when it is
// still the token of this call), then the call logs in again and is sent one more time; a second
// 401 is provider down. ProviderTraceID is empty, as before task 034.
// @todo the tavanir service answers the "down" codes (500, 502, 506, 508, 509, 512, 513, 517, 524,
// 527, 528) and the unknown codes with a failed or a pending status and a message; this adapter
// treats them as provider down. See docs/this_vs_bill.md, item 27.
func (a *Adapter) Inquiry(ctx context.Context, req provider.InquiryRequest) (*provider.InquiryResponse, error) {
	if a == nil || a.client == nil || a.login == nil {
		return nil, errors.New("tavanir: adapter not created with New")
	}
	if !a.SupportsInquiry(req.BillType) {
		return nil, fmt.Errorf("tavanir: inquiry of %q: %w", req.BillType, provider.ErrNotSupported)
	}
	if !a.enabled {
		return nil, down("providers.external.tavanir.enabled is false")
	}
	if a.authEndpoint == "" || a.branchEndpoint == "" || a.username == "" || a.password == "" {
		return nil, down("providers.external.tavanir.auth_endpoint, basic_branch_endpoint, username and password are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(branchRequest{BillIdentifier: req.InquiryID, MobileNo: req.Dialer})
	if err != nil {
		return nil, downCause("encode request", err)
	}
	// Two tries: the second one only after a 401, with a new token.
	for range 2 {
		token, err := a.getToken(ctx)
		if err != nil {
			return nil, err
		}
		raw, err := a.post(ctx, a.branchEndpoint, body, token)
		if errors.Is(err, errUnauthorized) {
			a.dropToken(token)
			continue
		}
		if err != nil {
			return nil, err
		}
		return answer(req, raw)
	}
	return nil, down("HTTP status 401 again after a new login")
}

// getToken returns the token, and logs in when there is none. Only one login runs at a time; the
// callers that wait for it then use its token and send no login of their own.
func (a *Adapter) getToken(ctx context.Context) (string, error) {
	if t := a.currentToken(); t != "" {
		return t, nil
	}
	select {
	case a.login <- struct{}{}:
	case <-ctx.Done():
		return "", downCause("wait for login", ctx.Err())
	}
	defer func() { <-a.login }()
	if t := a.currentToken(); t != "" { // another caller logged in while this one waited
		return t, nil
	}
	token, err := a.authenticate(ctx)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.token = token
	a.mu.Unlock()
	return token, nil
}

func (a *Adapter) currentToken() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.token
}

// dropToken removes the token after a 401, but only when it is still the token that the failed call
// used. A late 401 of an old token thus does not remove the token of a newer login, so many calls
// that fail together cause one new login, not one each.
func (a *Adapter) dropToken(used string) {
	a.mu.Lock()
	if a.token == used {
		a.token = ""
	}
	a.mu.Unlock()
}

// authenticate logs in (the tavanir service: Authenticate). A status 200 with a token that is not
// empty is a success; everything else is a failed login (provider down). The error never has the
// password or the token.
func (a *Adapter) authenticate(ctx context.Context) (string, error) {
	body, err := json.Marshal(loginRequest{Username: a.username, Password: a.password})
	if err != nil {
		return "", downCause("encode login", err)
	}
	raw, err := a.post(ctx, a.authEndpoint, body, "")
	if err != nil {
		if errors.Is(err, errUnauthorized) {
			return "", down("login: HTTP status 401")
		}
		return "", fmt.Errorf("login: %w", err)
	}
	parser := parsers.Get()
	defer parsers.Put(parser)
	ans, err := parser.ParseBytes(raw)
	if err != nil {
		return "", down("login: the answer is not JSON") // the cause can quote the body, which can hold a token
	}
	status := ans.GetInt("status")
	token := string(ans.GetStringBytes("data", "token"))
	if status != statusOK || token == "" {
		return "", down("login failed with status %d", status)
	}
	return token, nil
}

// post sends one POST of JSON, limited by the timeout, and returns the body of an HTTP 200 answer.
// token, when not empty, goes in the Authorization header. An HTTP 401 returns errUnauthorized;
// every other failure wraps provider.ErrProviderDown.
func (a *Adapter) post(ctx context.Context, endpoint string, body []byte, token string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, downCause("build request", err) // the message has the endpoint, never a secret
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, downCause("call", err)
	}
	if resp == nil || resp.Body == nil {
		return nil, down("empty answer")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))
	if err != nil {
		return nil, downCause("read answer", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return raw, nil
	case http.StatusUnauthorized:
		return nil, errUnauthorized
	}
	return nil, down("HTTP status %d", resp.StatusCode)
}

// answer maps the body of an inquiry answer (the tavanir service: billInquiryMapper and
// prepareResponse).
func answer(req provider.InquiryRequest, raw []byte) (*provider.InquiryResponse, error) {
	parser := parsers.Get()
	defer parsers.Put(parser)
	ans, err := parser.ParseBytes(raw)
	if err != nil {
		return nil, downCause("decode answer", err)
	}
	if ans.Type() != fastjson.TypeObject {
		return nil, down("the answer is not an object")
	}
	status := ans.GetInt("status")
	if status != statusOK {
		if msg, ok := refused[status]; ok {
			return &provider.InquiryResponse{Status: provider.ResponseFailed, Code: int32(status), Message: msg}, nil
		}
		return nil, down("status %d", status)
	}
	if n := len(ans.GetArray("error")); n > 0 {
		return nil, down("status 200 with %d error items", n)
	}
	data := ans.Get("data")
	if data == nil || data.Type() != fastjson.TypeObject {
		return nil, down("data of status 200 is not an object")
	}
	billID := integerText(flexible(data.Get("bill_identifier")))
	if billID == "" {
		billID = req.InquiryID
	}
	debt := data.GetFloat64("total_bill_debt")
	if debt < 0 {
		debt = 0
	}
	company := string(data.GetStringBytes("company_name"))
	name := strings.TrimSpace(string(data.GetStringBytes("customer_name")) + " " + string(data.GetStringBytes("customer_family")))
	return &provider.InquiryResponse{
		Status:  provider.ResponseSuccess,
		Code:    statusOK,
		Message: successMessage,
		BillID:  billID,
		Inquiries: []provider.InquiryItem{{
			PayID:           integerText(flexible(data.Get("payment_identifier"))),
			Amount:          uint64(debt),
			PaymentDeadline: dateOnly(string(data.GetStringBytes("payment_dead_line"))),
			InquiryType:     inquiryTypeRegular,
		}},
		// As before task 034 (and as bill), City is the company name and Region is the city of Tavanir.
		Details: &provider.Details{Electricity: &provider.ElectricityDetails{
			Phase:           strconv.Itoa(data.GetInt("phase")),
			CompanyName:     company,
			TariffType:      strconv.Itoa(data.GetInt("tariff_type")),
			CustomerType:    int32(data.GetInt("customer_type")),
			SerialNumber:    flexible(data.Get("serial_number")),
			ServiceAddress:  string(data.GetStringBytes("service_add")),
			ServicePostCode: flexible(data.Get("service_post_code")),
			CustomerName:    name,
			City:            company,
			Region:          string(data.GetStringBytes("city")),
		}},
	}, nil
}

// flexible reads a field that Tavanir sends as a string or as a number: the string, or the digits of
// the number as they are in the answer. A missing field, null and every other type give "".
func flexible(v *fastjson.Value) string {
	if v == nil {
		return ""
	}
	switch v.Type() {
	case fastjson.TypeString:
		return string(v.GetStringBytes())
	case fastjson.TypeNumber:
		return v.String()
	}
	return ""
}

// integerText writes a number text as a whole number with no exponent and no fraction ("1.2e+3"
// and "1200.0" give "1200"), as utils.ParseNumericToString of the tavanir service does for the bill
// ID and the pay ID. A text that is not a number stays as it is. The precision of 256 bits keeps
// every digit of a number of up to 77 digits (the tavanir service uses 64 bits: 19 digits).
func integerText(s string) string {
	if s == "" {
		return ""
	}
	f, _, err := big.ParseFloat(s, 10, 256, big.ToNearestEven)
	if err != nil || f == nil {
		return s
	}
	i, _ := f.Int(nil)
	if i == nil {
		return s
	}
	return i.String()
}

// dateOnly cuts a date and time of Tavanir to yyyy-mm-dd (the tavanir service: removeTime). A text
// that is not a date and time gives "".
// @todo the tavanir service reads RFC 3339 only; this adapter also reads a time with no zone
// ("2026-01-02T15:04:05"). See docs/this_vs_bill.md, item 27.
func dateOnly(s string) string {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format(time.DateOnly)
		}
	}
	return ""
}

// Pay is not supported yet.
func (*Adapter) Pay(context.Context, provider.PaymentRequest) (*provider.PaymentResponse, error) {
	return nil, provider.ErrNotSupported
}

// Status is not supported yet.
func (*Adapter) Status(context.Context, provider.StatusRequest) (*provider.StatusResponse, error) {
	return nil, provider.ErrNotSupported
}

// GetDocument is not supported yet (providers.external.tavanir.bill_document_endpoint is not used).
func (*Adapter) GetDocument(context.Context, provider.DocumentRequest) (*provider.DocumentResponse, error) {
	return nil, provider.ErrNotSupported
}

// Settle is not supported yet.
func (*Adapter) Settle(context.Context, provider.SettleRequest) (*provider.SettleResponse, error) {
	return nil, provider.ErrNotSupported
}

// Mock fixtures (task 022). The values are fixed: a valid electricity inquiry with all details.
// The pay ID follows the rule of a bill pay ID (the digits before the last 5 are the amount in
// thousands of rials): "18000" gives 18 000 000 rials.
const (
	MockPayID           = "1800012345"
	MockAmount          = 18_000_000
	MockDeadline        = "2030-01-01"
	MockPhase           = "تک فاز"
	MockCompanyName     = "توزیع برق تهران بزرگ"
	MockTariffType      = "خانگی"
	MockCustomerType    = 1
	MockSerialNumber    = "12345678"
	MockServiceAddress  = "تهران، خیابان آزادی، پلاک ۱"
	MockServicePostCode = "1234567890"
	MockCustomerName    = "مشترک آزمایشی"
	MockRegion          = "تهران"
)

// MockFixtures returns the fixed answers of the Tavanir mock (debug.mocks.tavanir): Inquiry answers
// the bill of the request with one inquiry of 18 000 000 rials (pay ID 1800012345, deadline
// 2030-01-01), no trace ID (as Tavanir) and the Mock... details.
func MockFixtures() provider.MockFixtures {
	return provider.MockFixtures{Inquiry: func(req provider.InquiryRequest) *provider.InquiryResponse {
		return &provider.InquiryResponse{
			Status:  provider.ResponseSuccess,
			Code:    200,
			Message: successMessage,
			BillID:  req.InquiryID,
			Inquiries: []provider.InquiryItem{{
				PayID:           MockPayID,
				Amount:          MockAmount,
				PaymentDeadline: MockDeadline,
				InquiryType:     inquiryTypeRegular,
			}},
			Details: &provider.Details{Electricity: &provider.ElectricityDetails{
				Phase:           MockPhase,
				CompanyName:     MockCompanyName,
				TariffType:      MockTariffType,
				CustomerType:    MockCustomerType,
				SerialNumber:    MockSerialNumber,
				ServiceAddress:  MockServiceAddress,
				ServicePostCode: MockServicePostCode,
				CustomerName:    MockCustomerName,
				City:            MockCompanyName,
				Region:          MockRegion,
			}},
		}
	}}
}
