// Package qabzino is the direct adapter of the Qabzino provider (task 030). It calls Ayantech over
// HTTPS with the token and the endpoints of providers.qabzino, as the old qabzino service does
// (client/qabzino, gateway/water.go, gateway/gas.go), with no hop through that service. Inquiry of
// a water bill (task 030) and of a gas bill (task 023) is implemented; every other method and bill
// type returns provider.ErrNotSupported. The former gRPC adapter is package qabzinoproxy (not used).
//
// Every call to Ayantech is charged: no test sends one. Tests use a local httptest server.
package qabzino

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fastjson"

	"qabz/provider"
)

// Name is the providers.name of Qabzino.
const Name = "qabzino"

// inquiryTypeRegular is the inquiry_type of a water and a gas inquiry (bill: entity.RegularInquiryType).
const inquiryTypeRegular = "regular"

// successMessage is the message of an answer with inquiries (bill: "the operation was done successfully").
const successMessage = "عملیات با موفقیت انجام شد"

// maxAnswerBytes limits the body that the adapter reads from Ayantech.
const maxAnswerBytes = 1 << 20

// The Ayantech status codes of a water and a gas inquiry (bill: gateway/translator.go).
const (
	codeSuccess     = "G00000"
	codeWaterPaid   = "GD2068" // the bill is paid and no new bill is issued: success, no debt
	codeWaterDown   = "GD2066" // the water company cannot be reached: provider down
	codeWaterBadID  = "GD2061"
	codeWaterNotOwn = "GD2062"
	codeWaterNone   = "GD2067"

	codeGasNoBill      = "GD2038" // no bill to pay for the participate code: success, no debt
	codeGasDown        = "GD2036" // the gas company cannot be reached: provider down
	codeGasBadCode     = "GD2031"
	codeGasBadCode2    = "GD2032"
	codeGasBadBillID   = "GD2033"
	codeGasCodeUnknown = "GD2037"
)

// refused are the codes of a water inquiry that give a failed answer (no inquiry) with a message
// for the user (bill: waterBillInquiryErrors).
var refused = map[string]string{
	codeWaterBadID:  "محتوای شناسه قبض وارد شده صحیح نیست",
	codeWaterNotOwn: "شناسه ای قبض وارد شده، متعلق به شرکت آب فاضالب نیست",
	codeWaterNone:   "شناسه قبض آب وارد شده در سیستم یافت نشد",
}

// paidMessage is the message of GD2068 (bill: waterBillInquiryErrors).
const paidMessage = "این قبض قبال پرداخت شده و قبض جدیدی برای مشـتــرک صــادر نشــده است."

// gasRefused are the codes of a gas inquiry that give a failed answer (no inquiry) with a message
// for the user (bill: gasBillInquiryErrors).
var gasRefused = map[string]string{
	codeGasBadCode:     "محتوای کد اشتراک گاز وارد شده صحیح نیست",
	codeGasBadCode2:    "محتوای کد اشتراک گاز وارد شده صحیح نیست",
	codeGasBadBillID:   "محتوای شناسه قبض وارد شده صحیح نیست",
	codeGasCodeUnknown: "کد اشتراک گاز وارد شده در سیستم یافت نشد",
}

// gasNoBillMessage is the message of GD2038 (bill: gasBillInquiryErrors).
const gasNoBillMessage = "هیچ قبض قابل پرداختی برای اشتراک گاز استعلام شده ثبت نشده است"

// kind is what the adapter knows of the inquiry of one bill type: where to send it, the Parameters
// of the request, and how to read the answer. The rest of Inquiry is the same for every bill type.
type kind struct {
	// endpointKey is the config key of the endpoint, for the error text.
	endpointKey string
	// endpoint returns the endpoint of the adapter.
	endpoint func(a *Adapter) string
	// parameters returns the Parameters object of the request.
	parameters func(inquiryID, trace string) any
	// noDebtCode is the code of "no bill to pay": success with one inquiry of amount 0 and no pay ID.
	noDebtCode, noDebtMessage string
	// refused are the codes of a failed answer (no inquiry) with a message for the user.
	refused map[string]string
	// success builds the answer of G00000 from its Parameters object.
	success func(req provider.InquiryRequest, trace string, params *fastjson.Value, description string) *provider.InquiryResponse
}

// kinds are the bill types that the adapter inquires.
var kinds = map[string]kind{
	provider.BillTypeWater: {
		endpointKey: "water_bill_endpoint",
		endpoint:    func(a *Adapter) string { return a.waterEndpoint },
		parameters: func(inquiryID, trace string) any {
			return waterParameters{WaterBillID: inquiryID, TraceNumber: trace}
		},
		noDebtCode: codeWaterPaid, noDebtMessage: paidMessage,
		refused: refused,
		success: func(req provider.InquiryRequest, trace string, params *fastjson.Value, description string) *provider.InquiryResponse {
			return successAnswer(req, trace, waterAnswer{
				Amount:      params.GetUint64("Amount"),
				BillID:      string(params.GetStringBytes("BillID")),
				PaymentID:   string(params.GetStringBytes("PaymentID")),
				PaymentDate: string(params.GetStringBytes("PaymentDate")),
			}, description)
		},
	},
	provider.BillTypeGas: {
		endpointKey: "gas_bill_endpoint",
		endpoint:    func(a *Adapter) string { return a.gasEndpoint },
		parameters: func(inquiryID, trace string) any {
			return gasParameters{ParticipateCode: inquiryID, TraceNumber: trace}
		},
		noDebtCode: codeGasNoBill, noDebtMessage: gasNoBillMessage,
		refused: gasRefused,
		success: gasSuccessAnswer,
	},
}

// Doer is the part of *http.Client that the adapter uses. Tests give a fake or a client of an
// httptest server.
type Doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Options configure New.
type Options struct {
	// Enabled is providers.qabzino.enabled. False: no call is sent; every Inquiry fails as
	// provider down.
	Enabled bool
	// Token is providers.qabzino.token (secret). Empty: every Inquiry fails as provider down.
	Token string
	// WaterEndpoint is providers.qabzino.water_bill_endpoint. Empty: every water Inquiry fails as
	// provider down.
	WaterEndpoint string
	// GasEndpoint is providers.qabzino.gas_bill_endpoint. Empty: every gas Inquiry fails as provider
	// down.
	GasEndpoint string
	// Timeout limits each call. Required, positive.
	Timeout time.Duration
	// HTTPClient sends the requests. Nil: a client with Timeout (the system TLS roots; bill skips
	// the certificate check, this adapter does not).
	HTTPClient Doer
}

// Adapter implements provider.ProviderInterface for Qabzino. It is safe for concurrent use.
type Adapter struct {
	enabled       bool
	token         string
	waterEndpoint string
	gasEndpoint   string
	timeout       time.Duration
	client        Doer
	// newTrace makes the trace number of a call (bill: a new UUID). Tests replace it.
	newTrace func() string
}

var _ provider.ProviderInterface = (*Adapter)(nil)

// New returns the adapter.
func New(opts Options) (*Adapter, error) {
	if opts.Timeout <= 0 {
		return nil, errors.New("qabzino: timeout must be positive")
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: opts.Timeout}
	}
	return &Adapter{
		enabled:       opts.Enabled,
		token:         opts.Token,
		waterEndpoint: opts.WaterEndpoint,
		gasEndpoint:   opts.GasEndpoint,
		timeout:       opts.Timeout,
		client:        client,
		newTrace:      func() string { return uuid.NewString() },
	}, nil
}

// Name returns Name.
func (a *Adapter) Name() string { return Name }

// SupportsInquiry reports whether Inquiry answers the bill type: a bill type of kinds with all
// its parts (water and gas; task 036).
func (a *Adapter) SupportsInquiry(billType string) bool {
	k, ok := kinds[billType]
	return ok && k.endpoint != nil && k.parameters != nil && k.success != nil
}

// Enabled reports providers.qabzino.enabled.
func (a *Adapter) Enabled() bool { return a != nil && a.enabled }

type identity struct {
	Token string `json:"Token"`
}

type waterParameters struct {
	WaterBillID string `json:"WaterBillID"`
	TraceNumber string `json:"TraceNumber"`
}

// gasParameters are the Parameters of a gas inquiry (the qabzino service: GasBillRequestParameters).
// The inquiry ID is always sent as the participate code and GasBillID is always empty, as the
// qabzino service does.
type gasParameters struct {
	GasBillID       string `json:"GasBillID"`
	ParticipateCode string `json:"ParticipateCode"`
	TraceNumber     string `json:"TraceNumber"`
}

type request struct {
	Identity   identity `json:"Identity"`
	Parameters any      `json:"Parameters"` // waterParameters or gasParameters (kind.parameters)
}

// parsers holds the fastjson parsers of the answers. Reading the answer of Ayantech is on a hot
// path (every inquiry that the database cannot answer), so it uses fastjson (conventions § 1); the
// request is written with encoding/json/v2.
var parsers fastjson.ParserPool

// waterAnswer is the Parameters object of a G00000 answer.
type waterAnswer struct {
	Amount      uint64
	BillID      string
	PaymentID   string
	PaymentDate string
}

// down returns an error that wraps provider.ErrProviderDown.
func down(format string, args ...any) error {
	return fmt.Errorf("qabzino: %w: %s", provider.ErrProviderDown, fmt.Sprintf(format, args...))
}

// downCause is down with the cause kept in the chain (so the flow sees context.Canceled).
func downCause(what string, cause error) error {
	return fmt.Errorf("qabzino: %w: %s: %w", provider.ErrProviderDown, what, cause)
}

// Inquiry asks Ayantech for the inquiry of a water bill or a gas bill (bill: Water.Execute and
// Gas.Execute of qabzino). The bill type of the request selects the endpoint, the Parameters and
// the codes (kinds). The answer rules:
//   - G00000: success with one inquiry (the pay ID, the amount, the deadline). Water: the amount
//     comes from the pay ID (successAnswer). Gas: the amount of the answer, and the details
//     (gasSuccessAnswer).
//   - GD2068 (water, already paid) and GD2038 (gas, no bill to pay): success with one inquiry of
//     amount 0 and no pay ID.
//   - Water GD2061, GD2062, GD2067 and gas GD2031, GD2032, GD2033, GD2037: a failed answer with no
//     inquiry, the code and a message for the user.
//   - Every other code (GD2066 and GD2036: the company cannot be reached), a transport error, a bad
//     HTTP status or an unreadable body: an error that wraps provider.ErrProviderDown.
//
// The trace ID of the answer is the TraceNumber that the adapter sent (Ayantech has none).
// @todo bill answers a failed status with a message for each of about 40 codes; this adapter
// refuses only the codes above and treats every other code as provider down. See
// docs/this_vs_bill.md, item 26.
func (a *Adapter) Inquiry(ctx context.Context, req provider.InquiryRequest) (*provider.InquiryResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("qabzino: adapter not created with New")
	}
	k := kinds[req.BillType]
	if !a.SupportsInquiry(req.BillType) {
		return nil, fmt.Errorf("qabzino: inquiry of %q: %w", req.BillType, provider.ErrNotSupported)
	}
	if !a.enabled {
		return nil, down("providers.qabzino.enabled is false")
	}
	endpoint := k.endpoint(a)
	if a.token == "" || endpoint == "" {
		return nil, down("providers.qabzino.token and providers.qabzino.%s are required", k.endpointKey)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	trace := a.newTrace()
	body, err := json.Marshal(request{
		Identity:   identity{Token: a.token},
		Parameters: k.parameters(req.InquiryID, trace),
	})
	if err != nil {
		return nil, downCause("encode request", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, downCause("build request", err) // the message has the endpoint, never the token
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, downCause("call", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))
	if err != nil {
		return nil, downCause("read answer", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, down("HTTP status %d", resp.StatusCode)
	}
	parser := parsers.Get()
	defer parsers.Put(parser)
	ans, err := parser.ParseBytes(raw)
	if err != nil {
		return nil, downCause("decode answer", err)
	}
	code := string(ans.GetStringBytes("Status", "Code"))
	switch {
	case code == codeSuccess:
		params := ans.Get("Parameters")
		if params == nil || params.Type() != fastjson.TypeObject {
			return nil, down("parameters of %s are not an object", code)
		}
		return k.success(req, trace, params, string(ans.GetStringBytes("Status", "Description"))), nil
	case code != "" && code == k.noDebtCode:
		return &provider.InquiryResponse{
			Status:          provider.ResponseSuccess,
			Code:            http.StatusOK,
			Message:         k.noDebtMessage,
			ProviderTraceID: trace,
			Inquiries:       []provider.InquiryItem{{InquiryType: inquiryTypeRegular}},
		}, nil
	}
	if msg, ok := k.refused[code]; ok {
		n, _ := strconv.Atoi(code[2:]) // GD2061 -> 2061, as bill
		return &provider.InquiryResponse{
			Status:          provider.ResponseFailed,
			Code:            int32(n),
			Message:         msg,
			ProviderTraceID: trace,
		}, nil
	}
	return nil, down("status code %q", code)
}

// gasSuccessAnswer builds the answer of G00000 of a gas inquiry (bill: prepareGasInquiryResponse of
// qabzino and QabzinoGas.Execute): the amount of the answer as it is (no pay ID rule), 0 for an
// empty pay ID (a paid bill); the bill ID of the answer (the inquiry ID is the participate code, so
// there is no fallback); the details are the address (as city) and the consumption type (as kind).
// The returned strings are copies: nothing refers to the parser after it goes back to the pool.
func gasSuccessAnswer(_ provider.InquiryRequest, trace string, params *fastjson.Value, description string) *provider.InquiryResponse {
	payID := string(params.GetStringBytes("PaymentID"))
	amount := params.GetUint64("Amount")
	if payID == "" {
		amount = 0
	}
	if description == "" {
		description = successMessage
	}
	return &provider.InquiryResponse{
		Status:          provider.ResponseSuccess,
		Code:            http.StatusOK,
		Message:         description,
		ProviderTraceID: trace,
		BillID:          string(params.GetStringBytes("BillID")),
		Inquiries: []provider.InquiryItem{{
			PayID:           payID,
			Amount:          amount,
			PaymentDeadline: string(params.GetStringBytes("PaymentDate")),
			InquiryType:     inquiryTypeRegular,
		}},
		Details: &provider.Details{Gas: &provider.GasDetails{
			City: string(params.GetStringBytes("Address")),
			Kind: string(params.GetStringBytes("ConsumptionType")),
		}},
	}
}

// successAnswer builds the answer of G00000 of a water inquiry.
func successAnswer(req provider.InquiryRequest, trace string, w waterAnswer, description string) *provider.InquiryResponse {
	amount := uint64(0)
	// An empty pay ID is a paid bill: amount 0 even when the amount above is not.
	if w.PaymentID != "" {
		var ok bool
		if amount, ok = amountFromPayID(w.PaymentID); !ok {
			amount = w.Amount
		}
	}
	billID := w.BillID
	if billID == "" {
		billID = req.InquiryID
	}
	if description == "" {
		description = successMessage
	}
	return &provider.InquiryResponse{
		Status:          provider.ResponseSuccess,
		Code:            http.StatusOK,
		Message:         description,
		ProviderTraceID: trace,
		BillID:          billID,
		Inquiries: []provider.InquiryItem{{
			PayID:           w.PaymentID,
			Amount:          amount,
			PaymentDeadline: w.PaymentDate,
			InquiryType:     inquiryTypeRegular,
		}},
	}
}

// amountFromPayID reads the amount from a pay ID (bill: helper.GetFromPayIdPrice): the digits
// before the last 5 are the amount in thousands of rials. ok is false for a pay ID that is
// shorter than 6, whose amount part is not a number, or whose amount overflows.
func amountFromPayID(payID string) (amount uint64, ok bool) {
	if len(payID) < 6 {
		return 0, false
	}
	n, err := strconv.ParseUint(payID[:len(payID)-5], 10, 64)
	if err != nil || n > math.MaxUint64/1000 {
		return 0, false
	}
	return n * 1000, true
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

// Mock fixtures (task 021: water; task 023: gas). The values are fixed: a valid inquiry. The water
// pay ID follows the rule of amountFromPayID (the digits before the last 5 are the amount in
// thousands of rials): "25000" gives 25 000 000 rials. The gas pay ID follows the same rule
// ("32000": 32 000 000 rials), so the amount and the pay ID agree.
const (
	mockPayID    = "2500012345"
	mockAmount   = 25_000_000
	mockDeadline = "2030-01-01"
	mockTrace    = "mock-qabzino-0001"

	// MockGasBillID is the bill ID of the gas mock answer: the request has a participate code, not
	// a bill ID. It is a valid bill ID (helpers.ValidBillID; a test checks it).
	MockGasBillID = "1234567890125"
	mockGasPayID  = "3200012345"
	mockGasAmount = 32_000_000
	mockGasCity   = "تهران، خیابان آزادی، پلاک ۱"
	mockGasKind   = "خانگی"
)

// MockFixtures returns the fixed answers of the Qabzino mock (debug.mocks.qabzino). Inquiry answers
// by the bill type of the request:
//   - gas: the bill ID MockGasBillID with one inquiry of 32 000 000 rials (pay ID 3200012345,
//     deadline 2030-01-01) and the details (city, kind).
//   - every other bill type (water): the bill of the request with one inquiry of 25 000 000 rials
//     (pay ID 2500012345, deadline 2030-01-01).
func MockFixtures() provider.MockFixtures {
	return provider.MockFixtures{Inquiry: func(req provider.InquiryRequest) *provider.InquiryResponse {
		if req.BillType == provider.BillTypeGas {
			return &provider.InquiryResponse{
				Status:          provider.ResponseSuccess,
				Code:            200,
				Message:         successMessage,
				ProviderTraceID: mockTrace,
				BillID:          MockGasBillID,
				Inquiries: []provider.InquiryItem{{
					PayID:           mockGasPayID,
					Amount:          mockGasAmount,
					PaymentDeadline: mockDeadline,
					InquiryType:     inquiryTypeRegular,
				}},
				Details: &provider.Details{Gas: &provider.GasDetails{City: mockGasCity, Kind: mockGasKind}},
			}
		}
		return &provider.InquiryResponse{
			Status:          provider.ResponseSuccess,
			Code:            200,
			Message:         successMessage,
			ProviderTraceID: mockTrace,
			BillID:          req.InquiryID,
			Inquiries: []provider.InquiryItem{{
				PayID:           mockPayID,
				Amount:          mockAmount,
				PaymentDeadline: mockDeadline,
				InquiryType:     inquiryTypeRegular,
			}},
		}
	}}
}
