// Package karashop is the adapter of the Karashop provider (task 025). It calls the Karashop
// (digitalport) HTTPS API with the code, the password and the inquiry URL of providers.external.karashop, as
// bill does (thirdparty/karashop, gateway/inquiry/telephone.go KarashopTelephone and
// karashopTranslator.go). Only the inquiry of a landline bill (the bill type tel) is implemented;
// every other method and bill type returns provider.ErrNotSupported.
//
// The password is a secret: it is only in the request body, and no error text has the body. Tests
// use a local httptest server; no test calls Karashop.
package karashop

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/valyala/fastjson"

	"qabz/provider"
)

// Name is the providers.name of Karashop.
const Name = "karashop"

// The Karashop services of a landline inquiry (bill: karashop.End and karashop.Middle). bill asks
// both, in two steps: the bill type tel asks the full term, its pair telmidterm the mid term.
const (
	serviceFullTerm = "tci-end-getBillAccInfo"
	serviceMidTerm  = "tci-mid-getBillAccInfo"
)

// maxAnswerBytes limits the body that the adapter reads from Karashop.
const maxAnswerBytes = 1 << 20

// maxTraceLen is the length of the provider_trace_id and tid columns (VARCHAR(100)): a joined
// trace ID that is longer keeps only the full-term trace ID.
const maxTraceLen = 100

// telStatusSuccess are the result.data.telStatus values of a success, with bill's text (bill:
// telStatusMap).
var telStatusSuccess = map[int]string{
	1: "تلفن معتبر است",
	3: "تلفن معتبر است اما جز تلفن های خاص است",
	4: "تلفن معتبر است اما مشترک بدون بدهی است",
}

// The answers that are a refusal with a text for the user: the landline number is not valid
// (telStatus 2) and the status codes of a wrong input (bill: telStatusMap, statusCodeMap). Every
// other answer that is not a success is "provider down".
const (
	telStatusInvalid = 2
	invalidPhoneText = "تلفن نامعتبر است"
)

var statusRefused = map[int]string{
	http.StatusBadRequest: "خطا در داده‌های ورودی وجود دارد",
	http.StatusNotFound:   "پاسخی برای درخواست شما یافت نشد",
}

// Doer is the part of *http.Client that the adapter uses. Tests give a client of an httptest server.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Options configure New.
type Options struct {
	// Enabled is providers.external.karashop.enabled. False: no call is sent; every Inquiry fails as
	// provider down.
	Enabled bool
	// Code and Password are providers.external.karashop.code and providers.external.karashop.password (secret).
	// Empty: every Inquiry fails as provider down.
	Code, Password string
	// InquiryURL is providers.external.karashop.bill_inquiry_url. Empty: every Inquiry fails as provider down.
	InquiryURL string
	// Timeout limits each of the two calls of an Inquiry. Required, positive.
	Timeout time.Duration
	// HTTPClient sends the requests. Nil: a client with Timeout (the system TLS roots; bill skips the
	// certificate check, this adapter does not).
	HTTPClient Doer
}

// Adapter implements provider.ProviderInterface for Karashop. It is safe for concurrent use.
type Adapter struct {
	enabled    bool
	code       string
	password   string
	inquiryURL string
	timeout    time.Duration
	client     Doer
}

var _ provider.ProviderInterface = (*Adapter)(nil)

// New returns the adapter.
func New(opts Options) (*Adapter, error) {
	if opts.Timeout <= 0 {
		return nil, errors.New("karashop: timeout must be positive")
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: opts.Timeout}
	}
	return &Adapter{
		enabled:    opts.Enabled,
		code:       opts.Code,
		password:   opts.Password,
		inquiryURL: opts.InquiryURL,
		timeout:    opts.Timeout,
		client:     client,
	}, nil
}

// Name returns Name.
func (a *Adapter) Name() string { return Name }

// SupportsInquiry reports whether Inquiry answers the bill type: only tel.
func (a *Adapter) SupportsInquiry(billType string) bool { return billType == provider.BillTypeTel }

// Enabled reports providers.external.karashop.enabled.
func (a *Adapter) Enabled() bool { return a != nil && a.enabled }

type credential struct {
	Code     string `json:"code"`
	Password string `json:"password"`
}

type parameter struct {
	Name  string `json:"parameterName"`
	Value string `json:"parameterValue"`
}

// request is the body of a Karashop call (bill: karashop.RequestBody).
type request struct {
	Credential credential  `json:"credential"`
	Parameters []parameter `json:"parameters"`
	Service    string      `json:"service"`
}

// localForm gives the landline number in the form that Karashop reads: the number with no "98" and
// no "0" in front (bill: prepareTelephoneNumber). The flow sends the 98 form (helpers.ValidTel).
func localForm(tel string) string {
	switch {
	case len(tel) >= 2 && tel[:2] == "98":
		return tel[2:]
	case len(tel) >= 1 && tel[0] == '0':
		return tel[1:]
	default:
		return tel
	}
}

// parsers holds the fastjson parsers of the answers. Reading the answer of Karashop is on a hot path
// (every landline inquiry that the database cannot answer), so it uses fastjson (conventions § 1);
// the request is written with encoding/json/v2. An answer needs two parsers at the same time: the
// outer object and the JSON text of its responseText.
var parsers fastjson.ParserPool

// down returns an error that wraps provider.ErrProviderDown.
func down(format string, args ...any) error {
	return fmt.Errorf("karashop: %w: %s", provider.ErrProviderDown, fmt.Sprintf(format, args...))
}

// downCause is down with the cause kept in the chain (so the flow sees context.Canceled).
func downCause(what string, cause error) error {
	return fmt.Errorf("karashop: %w: %s: %w", provider.ErrProviderDown, what, cause)
}

// part is the answer of one Karashop call: a success (billID, payID, amount) or a refusal
// (refused true, with code and message). An answer that is neither is an error of the call.
type part struct {
	refused bool
	code    int32
	message string
	billID  string
	payID   string
	amount  uint64
	trace   string
}

// Inquiry asks Karashop for the inquiry of a landline bill. One inquiry is two Karashop calls, sent
// at the same time: tci-end-getBillAccInfo (the full term) and tci-mid-getBillAccInfo (the mid
// term), each with the number in the local form (localForm). bill sends the same two calls in two
// steps of its flow, with one request row for each (task 025, design decision 1).
//
// The answer of each call (bill: HandleKarashopResponse; task 025, design decision 3):
//   - telStatus 1, 3 or 4: success, one inquiry with the bill ID, the pay ID and the amount of the
//     answer (a missing or null amount is 0, a negative amount is 0; 4 is "no debt").
//   - telStatus 2 (the number is not valid), status code 400 or 404: a refusal with bill's text.
//   - Every other answer (result.status.code set, status code 401, 403, 410, 422, 429, 499, 500,
//     502, 503, 504, 520, 200 or 202 with no success telStatus, or an unknown code), a transport
//     error, an HTTP status other than 200 and a body that cannot be read: provider down.
//
// The two calls together: a provider-down call makes the answer provider down and stops the other
// call (the error of the call that failed first is reported; the full term when both failed on
// their own); else a refused call makes the answer a refusal (the full term first); else the
// answer is a success with a mid-term and a final-term inquiry. Each inquiry has the bill ID of its
// own call; the bill ID of the answer is the full-term one (the mid-term one when it is empty). The
// trace ID is the transaction ID of the full-term call and of the mid-term call, joined with ","
// (only the full-term one when the two are the same or the join is longer than 100 bytes).
// @todo bill answers every code of its tables with a text, the pending codes as pending, and keeps
// the inquiry of one call when the other fails. See docs/this_vs_bill.md, item 37.
func (a *Adapter) Inquiry(ctx context.Context, req provider.InquiryRequest) (*provider.InquiryResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("karashop: adapter not created with New")
	}
	if !a.SupportsInquiry(req.BillType) {
		return nil, fmt.Errorf("karashop: inquiry of %q: %w", req.BillType, provider.ErrNotSupported)
	}
	if !a.enabled {
		return nil, down("providers.external.karashop.enabled is false")
	}
	if a.code == "" || a.password == "" || a.inquiryURL == "" {
		return nil, down("providers.external.karashop.code, password and bill_inquiry_url are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tel := localForm(req.InquiryID)

	// A call that fails stops the other one: the inquiry is down already, so it does not wait for
	// the other call (up to the timeout).
	callCtx, stop := context.WithCancel(ctx)
	defer stop()
	var full, mid part
	var fullErr, midErr error
	var wg sync.WaitGroup
	wg.Go(func() {
		if full, fullErr = a.call(callCtx, serviceFullTerm, tel); fullErr != nil {
			stop()
		}
	})
	wg.Go(func() {
		if mid, midErr = a.call(callCtx, serviceMidTerm, tel); midErr != nil {
			stop()
		}
	})
	wg.Wait()

	// The error of the call that failed first is the cause; the other call may then have only the
	// cancel of stop (the client did not leave: ctx is not done).
	stoppedByOther := func(err error) bool { return errors.Is(err, context.Canceled) && ctx.Err() == nil }
	switch {
	case fullErr != nil && (midErr == nil || !stoppedByOther(fullErr)):
		return nil, fmt.Errorf("full term: %w", fullErr)
	case midErr != nil:
		return nil, fmt.Errorf("mid term: %w", midErr)
	case full.refused:
		return &provider.InquiryResponse{Status: provider.ResponseFailed, Code: full.code, Message: full.message, ProviderTraceID: full.trace}, nil
	case mid.refused:
		return &provider.InquiryResponse{Status: provider.ResponseFailed, Code: mid.code, Message: mid.message, ProviderTraceID: mid.trace}, nil
	}
	billID := full.billID
	if billID == "" {
		billID = mid.billID
	}
	return &provider.InquiryResponse{
		Status:          provider.ResponseSuccess,
		Code:            http.StatusOK,
		Message:         full.message,
		ProviderTraceID: joinTrace(full.trace, mid.trace),
		BillID:          billID,
		Inquiries: []provider.InquiryItem{
			{BillID: mid.billID, PayID: mid.payID, Amount: mid.amount, InquiryType: provider.InquiryTypeMidterm},
			{BillID: full.billID, PayID: full.payID, Amount: full.amount, InquiryType: provider.InquiryTypeFinalTerm},
		},
	}, nil
}

// joinTrace is the trace ID of a two-call answer (Inquiry).
func joinTrace(full, mid string) string {
	switch {
	case mid == "" || mid == full:
		return full
	case full == "":
		return mid
	case len(full)+1+len(mid) > maxTraceLen:
		return full
	}
	return full + "," + mid
}

// call sends one Karashop call (bill: karashop.BillInquiry) and reads its answer.
func (a *Adapter) call(ctx context.Context, service, tel string) (part, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	body, err := json.Marshal(request{
		Credential: credential{Code: a.code, Password: a.password},
		Parameters: []parameter{{Name: "telNumber", Value: tel}},
		Service:    service,
	})
	if err != nil {
		return part{}, downCause("encode request", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.inquiryURL, bytes.NewReader(body))
	if err != nil {
		return part{}, downCause("build request", err) // the message has the URL, never the body
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return part{}, downCause("call", err)
	}
	if resp == nil || resp.Body == nil {
		return part{}, down("empty answer")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))
	if err != nil {
		return part{}, downCause("read answer", err)
	}
	if resp.StatusCode != http.StatusOK {
		return part{}, down("HTTP status %d", resp.StatusCode)
	}
	return readAnswer(raw)
}

// readAnswer reads the body of a Karashop answer: a JSON object whose responseText is a JSON text
// (a string) with the inner answer (bill: BillInquiryRaw.CreateResponse). The returned strings are
// copies: nothing refers to a parser after it goes back to the pool.
func readAnswer(raw []byte) (part, error) {
	outerParser := parsers.Get()
	defer parsers.Put(outerParser)
	outer, err := outerParser.ParseBytes(raw)
	if err != nil {
		return part{}, downCause("decode answer", err)
	}
	text := outer.Get("responseText")
	if text == nil || text.Type() != fastjson.TypeString {
		return part{}, down("responseText is not a string")
	}
	innerParser := parsers.Get()
	defer parsers.Put(innerParser)
	ans, err := innerParser.ParseBytes(text.GetStringBytes())
	if err != nil {
		return part{}, downCause("decode responseText", err)
	}

	trace := string(ans.GetStringBytes("meta", "transactionId"))
	telStatus := ans.GetInt("result", "data", "telStatus")
	if msg, ok := telStatusSuccess[telStatus]; ok {
		data := ans.Get("result", "data")
		amount, err := amountOf(data.Get("amount"))
		if err != nil {
			return part{}, err
		}
		return part{
			message: msg,
			billID:  digitsOf(data.Get("billId")),
			payID:   digitsOf(data.Get("paymentId")),
			amount:  amount,
			trace:   trace,
		}, nil
	}
	if telStatus == telStatusInvalid {
		return part{refused: true, code: telStatusInvalid, message: invalidPhoneText, trace: trace}, nil
	}
	if c := ans.Get("result", "status", "code"); c != nil && c.Type() != fastjson.TypeNull {
		return part{}, down("result status code %s", c.String())
	}
	status := ans.GetInt("status", "code")
	if msg, ok := statusRefused[status]; ok {
		return part{refused: true, code: int32(status), message: msg, trace: trace}, nil
	}
	return part{}, down("status code %d, tel status %d", status, telStatus)
}

// digitsOf gives the text of a bill ID or a pay ID: a JSON number with no sign, no fraction and no
// exponent, or a JSON string of digits. Anything else (missing, null, another form) gives "", as a
// nil value in bill.
func digitsOf(v *fastjson.Value) string {
	if v == nil {
		return ""
	}
	var s string
	switch v.Type() {
	case fastjson.TypeNumber:
		s = v.String()
	case fastjson.TypeString:
		s = string(v.GetStringBytes())
	default:
		return ""
	}
	if s == "" {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return ""
		}
	}
	return s
}

// amountOf reads the amount (rials) of an answer: missing or null is 0 (bill), a negative amount is
// 0 (bill converts it to a very large number). A value that is not an integer is provider down.
func amountOf(v *fastjson.Value) (uint64, error) {
	if v == nil || v.Type() == fastjson.TypeNull {
		return 0, nil
	}
	if v.Type() != fastjson.TypeNumber {
		return 0, down("amount is not a number")
	}
	n, err := strconv.ParseInt(v.String(), 10, 64)
	if err != nil {
		return 0, down("amount %s is not an integer", v.String())
	}
	if n < 0 {
		return 0, nil
	}
	return uint64(n), nil
}

// Pay is not supported yet.
func (*Adapter) Pay(context.Context, provider.PaymentRequest) (*provider.PaymentResponse, error) {
	return nil, provider.ErrNotSupported
}

// Status is not supported yet.
func (*Adapter) Status(context.Context, provider.StatusRequest) (*provider.StatusResponse, error) {
	return nil, provider.ErrNotSupported
}

// GetDocument is not supported yet.
func (*Adapter) GetDocument(context.Context, provider.DocumentRequest) (*provider.DocumentResponse, error) {
	return nil, provider.ErrNotSupported
}

// Settle is not supported yet.
func (*Adapter) Settle(context.Context, provider.SettleRequest) (*provider.SettleResponse, error) {
	return nil, provider.ErrNotSupported
}

// The mock answer (debug.mocks.karashop). The values are fixed, not read from the number: a valid
// landline bill ID (helpers.ValidBillID, the bill type tel of helpers.BillTypeOfBillID) and two
// valid pay IDs (helpers.ValidPayID) whose amount rule (helpers.PayIDAmount) gives the amount. Tests
// check all of it.
const (
	MockBillID          = "2101234560040"
	MockMidTermPayID    = "12000035"
	MockMidTermAmount   = 120_000
	MockFinalTermPayID  = "35000042"
	MockFinalTermAmount = 350_000
	mockTrace           = "mock-karashop-0001"
	mockMessage         = "تلفن معتبر است"
)

// MockFixtures returns the fixed answer of the Karashop mock: the bill ID MockBillID with a mid-term
// inquiry of 120 000 rials (pay ID 12000035) and a final-term inquiry of 350 000 rials (pay ID
// 35000042), no deadline (Karashop sends none).
func MockFixtures() provider.MockFixtures {
	return provider.MockFixtures{Inquiry: func(provider.InquiryRequest) *provider.InquiryResponse {
		return &provider.InquiryResponse{
			Status:          provider.ResponseSuccess,
			Code:            http.StatusOK,
			Message:         mockMessage,
			ProviderTraceID: mockTrace,
			BillID:          MockBillID,
			Inquiries: []provider.InquiryItem{
				{PayID: MockMidTermPayID, Amount: MockMidTermAmount, InquiryType: provider.InquiryTypeMidterm},
				{PayID: MockFinalTermPayID, Amount: MockFinalTermAmount, InquiryType: provider.InquiryTypeFinalTerm},
			},
		}
	}}
}
