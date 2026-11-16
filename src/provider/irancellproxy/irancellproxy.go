// Package irancellproxy is the Irancell adapter (task 035): it calls the irancell service, one of
// our own services, over gRPC (pb.InternalService/BillInquiry and GetStatus, pb/proto/irancell). The
// irancell service calls Irancell; Qabz does not. Inquiry of an Irancell mobile bill, Status of an
// order (task 050, the status worker) and Pay of an Irancell order (task 056: a NATS request, the
// irancell service has no payment RPC) are implemented; every other method returns
// provider.ErrNotSupported. The rules copy the old bill service (gateway/inquiry/mobile_irancell.go,
// IrancellMobile, and base.go handlePhoneResponseWithoutInquiry; gateway/paymentInquiry/irancell.go,
// IrancellPaymentInquiry; gateway/payment/mobile_irancell.go, IrancellMobile).
package irancellproxy

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/valyala/fastjson"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "qabz/api/pb/irancell"
	"qabz/provider"
)

// Name is the providers.name of the Irancell provider (seed row 38).
const Name = "irancell"

// successMessage is the message of an answer with inquiries (bill: "the operation was done successfully").
const successMessage = "عملیات با موفقیت انجام شد"

// purchaseType is the purchase_type of every GetStatus call (bill: "charge").
const purchaseType = "charge"

var (
	// errNoAddress is the cause of every call of a client made with no address.
	errNoAddress = errors.New("providers.internal.irancell.grpc_address is not set")
	// errDisabled is the cause of every call while providers.internal.irancell.enabled is false.
	errDisabled = errors.New("providers.internal.irancell.enabled is false")
	// errNoNATS is the cause of every payment while the adapter has no NATS client (task 056).
	errNoNATS = errors.New("no NATS client (nats.endpoints is empty or the client was not made)")
	// errNoSubject is the cause of every payment while providers.internal.irancell.nats_subject is empty.
	errNoSubject = errors.New("providers.internal.irancell.nats_subject is not set")
)

// defaultPayTimeout is the limit of a payment request when Options.PayTimeout is 0 (bill: the fixed
// 30 s of pkg/orderbus).
const defaultPayTimeout = 30 * time.Second

// Requester sends a NATS request and returns the bytes of the answer (system/nats.Client, task 056).
// Tests give a fake.
type Requester interface {
	Request(ctx context.Context, subject string, data []byte) ([]byte, error)
}

// Client is the part of the generated gRPC client (pb.InternalServiceClient) that the adapter
// uses. Tests give a fake.
type Client interface {
	BillInquiry(ctx context.Context, in *pb.BillInquiryRequest, opts ...grpc.CallOption) (*pb.BillInquiryResponse, error)
	GetStatus(ctx context.Context, in *pb.GetStatusRequest, opts ...grpc.CallOption) (*pb.GetStatusResponse, error)
}

// Options configure New.
type Options struct {
	// Enabled is providers.internal.irancell.enabled. False: no call is sent; every Inquiry fails as
	// "provider down".
	Enabled bool
	// Client calls the irancell service. Required.
	Client Client
	// Timeout limits each call. Required, positive.
	Timeout time.Duration
	// NATS sends the payment requests (task 056). Nil: every Pay fails as "provider down".
	NATS Requester
	// Subject is providers.internal.irancell.nats_subject. Empty: every Pay fails as "provider down".
	Subject string
	// PayTimeout limits each payment request (providers.internal.irancell.pay_timeout). 0: 30 s.
	// Not negative.
	PayTimeout time.Duration
}

// Adapter implements provider.ProviderInterface for Irancell. It is safe for concurrent use.
type Adapter struct {
	enabled    bool
	client     Client
	timeout    time.Duration
	nats       Requester
	subject    string
	payTimeout time.Duration
}

var _ provider.ProviderInterface = (*Adapter)(nil)

// New returns the adapter.
func New(opts Options) (*Adapter, error) {
	if opts.Client == nil {
		return nil, errors.New("irancellproxy: client is required")
	}
	if opts.Timeout <= 0 {
		return nil, errors.New("irancellproxy: timeout must be positive")
	}
	if opts.PayTimeout < 0 {
		return nil, errors.New("irancellproxy: pay timeout must not be negative")
	}
	payTimeout := opts.PayTimeout
	if payTimeout == 0 {
		payTimeout = defaultPayTimeout
	}
	return &Adapter{enabled: opts.Enabled, client: opts.Client, timeout: opts.Timeout, nats: opts.NATS, subject: opts.Subject, payTimeout: payTimeout}, nil
}

// Dial returns a client for the irancell service at addr (host:port of its internal gRPC server,
// no TLS, as in bill). The connection is lazy, so Dial does not need a running service; it lives as
// long as the process. An empty addr gives a client whose every call fails (so the app starts
// without the key and an inquiry then answers "provider down").
func Dial(addr string) (Client, error) {
	if addr == "" {
		return noClient{}, nil
	}
	// A client span for each call, and traceparent in the metadata (task 047, conventions § 17).
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	if err != nil {
		return nil, fmt.Errorf("irancellproxy: grpc client for %s: %w", addr, err)
	}
	return pb.NewInternalServiceClient(conn), nil
}

type noClient struct{}

func (noClient) BillInquiry(context.Context, *pb.BillInquiryRequest, ...grpc.CallOption) (*pb.BillInquiryResponse, error) {
	return nil, errNoAddress
}

func (noClient) GetStatus(context.Context, *pb.GetStatusRequest, ...grpc.CallOption) (*pb.GetStatusResponse, error) {
	return nil, errNoAddress
}

// Name returns Name.
func (a *Adapter) Name() string { return Name }

// SupportsInquiry reports whether Inquiry answers the bill type: only the Irancell mobile bill (task 036).
func (a *Adapter) SupportsInquiry(billType string) bool { return billType == provider.BillTypeIrancell }

// Enabled reports providers.internal.irancell.enabled.
func (a *Adapter) Enabled() bool { return a != nil && a.enabled }

// Inquiry asks the irancell service for the inquiry of a mobile bill (bill: IrancellMobile.Execute).
// The inquiry ID is the phone (or the bill ID) of the request, sent as it is.
//   - A call that fails (the switch is off, no address, a connection error, a timeout, a nil
//     answer) returns an error that wraps provider.ErrProviderDown.
//   - An answer with inquiries is ResponseSuccess: one item for each inquiry, with its inquiry
//     type (provider.InquiryTypeMidterm, provider.InquiryTypeFinalTerm) and its amount; a negative
//     amount is 0. Irancell gives no bill ID, no pay ID and no payment deadline.
//   - An answer with no inquiry keeps the status, code and message of the service (an empty status
//     is ResponseFailed). When its status is "success" (no debt), it gives a mid-term and a
//     final-term inquiry of amount 0.
//
// @todo bill answers a failed call with the status "pending" (pendingInquiryResponse), so its
// request row is "pending"; here the call fails and the flow writes "failed". See
// docs/this_vs_bill.md, item 30.
func (a *Adapter) Inquiry(ctx context.Context, req provider.InquiryRequest) (*provider.InquiryResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("irancellproxy: adapter not created with New")
	}
	if !a.SupportsInquiry(req.BillType) {
		return nil, fmt.Errorf("irancellproxy: inquiry of %q: %w", req.BillType, provider.ErrNotSupported)
	}
	if !a.enabled {
		return nil, fmt.Errorf("irancellproxy: bill inquiry: %w: %w", provider.ErrProviderDown, errDisabled)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.BillInquiry(ctx, &pb.BillInquiryRequest{
		UserId:    req.UserID,
		Dialer:    req.Dialer,
		InquiryId: req.InquiryID,
	})
	if err != nil {
		return nil, fmt.Errorf("irancellproxy: bill inquiry: %w: %w", provider.ErrProviderDown, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("irancellproxy: bill inquiry: %w: empty answer", provider.ErrProviderDown)
	}
	base := resp.GetBaseResponse()
	items := make([]provider.InquiryItem, 0, len(resp.GetInquiries()))
	for _, inq := range resp.GetInquiries() {
		if inq == nil {
			continue
		}
		amount := uint64(0)
		if v := inq.GetAmount(); v > 0 {
			amount = uint64(v)
		}
		items = append(items, provider.InquiryItem{Amount: amount, InquiryType: inq.GetInquiryType()})
	}
	if len(items) > 0 {
		return &provider.InquiryResponse{
			Status:          provider.ResponseSuccess,
			Code:            200,
			Message:         successMessage,
			ProviderTraceID: base.GetProviderTraceId(),
			Inquiries:       items,
		}, nil
	}
	out := &provider.InquiryResponse{
		Status:          base.GetStatus(),
		Code:            base.GetCode(),
		Message:         base.GetMessage(),
		ProviderTraceID: base.GetProviderTraceId(),
	}
	if out.Status == "" {
		out.Status = provider.ResponseFailed
	}
	if out.Status == provider.ResponseSuccess { // no debt
		out.Inquiries = []provider.InquiryItem{
			{InquiryType: provider.InquiryTypeMidterm},
			{InquiryType: provider.InquiryTypeFinalTerm},
		}
	}
	return out, nil
}

// SupportsPay reports whether Pay pays the bill type: only irancell (task 056).
func SupportsPay(billType string) bool { return billType == provider.BillTypeIrancell }

// payMessage is the NATS payment message of bill (gateway/payment dto.go PaymentRequest, marshalled
// by the JSON EncodedConn of nats.go v1.11.0, that is encoding/json v1): the same names in the same
// order. InquiryType has no tag in bill, so its key is the Go name. The irancell service reads it as
// entity.BillRequest.
type payMessage struct {
	InquiryID   string `json:"inquiry_id"`
	BillID      string `json:"bill_id"`
	PayID       string `json:"pay_id"`
	OrderID     string `json:"order_id"`
	UserID      string `json:"user_id"`
	Dialer      string `json:"dialer"`
	RRN         string `json:"rrn"`
	Amount      uint64 `json:"amount"`
	Option      string `json:"option"`
	BillType    string `json:"type"`
	InquiryType string
}

// marshalPayMessage gives the bytes of m as bill's encoding/json v1 Marshal gives them (the struct
// order, the HTML and JS escapes of v1, invalid UTF-8 written as U+FFFD with no error, as v1), with
// encoding/json/v2 (conventions § 1: v1 is forbidden).
func marshalPayMessage(m payMessage) ([]byte, error) {
	return json.Marshal(m, json.Deterministic(true), jsontext.EscapeForHTML(true), jsontext.EscapeForJS(true), jsontext.AllowInvalidUTF8(true))
}

// parsers holds the fastjson parsers of the payment answers (a provider answer is a hot path,
// conventions § 1).
var parsers fastjson.ParserPool

// parsePayAnswer reads the answer of the irancell service (entity.ChargeResponse: code, message,
// tid, status). It must be a JSON object (null is an answer with no field, as bill's v1); code, when it is there and not null, a whole number that
// fits int32, and message and status strings (bill decodes them into an int32 and strings: anything
// else is a decode error, so pending). status is kept as it came (the order step makes an unknown or empty status pending, as bill's validStatuses).
//
// @todo docs/this_vs_bill.md item 49: bill decodes with encoding/json v1, which matches the keys with
// no case ("Code", "STATUS"); the irancell service writes them in lower case, so only those are read.
// The tid is not kept (operator decision Q1 (a) of task 056): bill's PaymentResponse has no field for
// it, so ref_id stays empty, as bill.
func parsePayAnswer(data []byte) (*provider.PaymentResponse, error) {
	p := parsers.Get()
	defer parsers.Put(p)
	v, err := p.ParseBytes(data)
	if err != nil {
		return nil, fmt.Errorf("the answer is not JSON: %w", err)
	}
	out := &provider.PaymentResponse{}
	if v.Type() == fastjson.TypeNull { // bill's v1 decode of null: no error, every field zero (pending, code 0)
		return out, nil
	}
	if v.Type() != fastjson.TypeObject {
		return nil, errors.New("the answer is not a JSON object")
	}
	for key, dst := range map[string]*string{"message": &out.Message, "status": &out.Status} {
		f := v.Get(key)
		switch {
		case f == nil || f.Type() == fastjson.TypeNull:
		case f.Type() == fastjson.TypeString:
			*dst = string(f.GetStringBytes())
		default: // bill: a decode error, so pending
			return nil, fmt.Errorf("the %s of the answer is not a string: %s", key, f.String())
		}
	}
	if c := v.Get("code"); c != nil && c.Type() != fastjson.TypeNull {
		n, err := c.Int64()
		if err != nil || n < math.MinInt32 || n > math.MaxInt32 {
			return nil, fmt.Errorf("the code of the answer is not an int32: %s", c.String())
		}
		out.Code = int32(n)
	}
	return out, nil
}

// Pay pays an Irancell order through the irancell service (task 056; bill: IrancellMobile.Execute of
// gateway/payment/mobile_irancell.go): one NATS request on providers.internal.irancell.nats_subject
// with bill's message (payMessage) and the limit providers.internal.irancell.pay_timeout.
//   - Not the bill type irancell: provider.ErrNotSupported.
//   - The switch off, no NATS client, no subject: an error that wraps provider.ErrProviderDown, with
//     no message sent.
//   - A failed request (a timeout, no connection, no responder) or an answer that is not a JSON object
//     (or whose code is not an int32): an error that wraps provider.ErrProviderDown (bill: pending,
//     code 500, the error text; the order step does the same).
//   - Else the code, status and message of the answer, as they came; ProviderTraceID is empty
//     (parsePayAnswer).
func (a *Adapter) Pay(ctx context.Context, req provider.PaymentRequest) (*provider.PaymentResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("irancellproxy: adapter not created with New")
	}
	if !SupportsPay(req.BillType) {
		return nil, fmt.Errorf("irancellproxy: pay %q: %w", req.BillType, provider.ErrNotSupported)
	}
	switch {
	case !a.enabled:
		return nil, fmt.Errorf("irancellproxy: pay: %w: %w", provider.ErrProviderDown, errDisabled)
	case a.nats == nil:
		return nil, fmt.Errorf("irancellproxy: pay: %w: %w", provider.ErrProviderDown, errNoNATS)
	case a.subject == "":
		return nil, fmt.Errorf("irancellproxy: pay: %w: %w", provider.ErrProviderDown, errNoSubject)
	}
	data, err := marshalPayMessage(payMessage{
		InquiryID: req.InquiryID, BillID: req.BillID, PayID: req.PayID, OrderID: req.OrderID, UserID: req.UserID,
		Dialer: req.Dialer, RRN: req.RRN, Amount: req.Amount, Option: req.Option, BillType: req.BillType, InquiryType: req.InquiryType,
	})
	if err != nil {
		return nil, fmt.Errorf("irancellproxy: pay: message: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, a.payTimeout)
	defer cancel()
	answer, err := a.nats.Request(ctx, a.subject, data)
	if err != nil {
		return nil, fmt.Errorf("irancellproxy: pay: %w: %w", provider.ErrProviderDown, err)
	}
	resp, err := parsePayAnswer(answer)
	if err != nil {
		return nil, fmt.Errorf("irancellproxy: pay: %w: %w", provider.ErrProviderDown, err)
	}
	return resp, nil
}

// Status asks the irancell service for the status of an order (task 050; bill:
// IrancellPaymentInquiry.Execute): GetStatus with the order ID and the purchase type "charge", with
// the timeout of the adapter.
//   - A call that fails (the switch is off, no address, a connection error, a timeout, a nil answer)
//     returns an error that wraps provider.ErrProviderDown (bill: the status pending with the error
//     text; the status worker counts it pending).
//   - Else the status, the message and the trace ID (tid) of the answer, as they are.
func (a *Adapter) Status(ctx context.Context, req provider.StatusRequest) (*provider.StatusResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("irancellproxy: adapter not created with New")
	}
	if !a.enabled {
		return nil, fmt.Errorf("irancellproxy: get status: %w: %w", provider.ErrProviderDown, errDisabled)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.GetStatus(ctx, &pb.GetStatusRequest{OrderId: req.OrderID, PurchaseType: purchaseType})
	if err != nil {
		return nil, fmt.Errorf("irancellproxy: get status: %w: %w", provider.ErrProviderDown, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("irancellproxy: get status: %w: empty answer", provider.ErrProviderDown)
	}
	return &provider.StatusResponse{
		Status:          resp.GetStatus(),
		Message:         resp.GetMessage(),
		ProviderTraceID: resp.GetTid(),
	}, nil
}

// GetDocument is not supported yet.
func (*Adapter) GetDocument(context.Context, provider.DocumentRequest) (*provider.DocumentResponse, error) {
	return nil, provider.ErrNotSupported
}

// Settle is not supported yet.
func (*Adapter) Settle(context.Context, provider.SettleRequest) (*provider.SettleResponse, error) {
	return nil, provider.ErrNotSupported
}

// Mock fixtures (task 035). The values are fixed: a mid-term inquiry of 150 000 rials and a
// final-term inquiry of 450 000 rials, with no bill ID, no pay ID and no payment deadline (as a
// real Irancell answer).
const (
	MockMidtermAmount   = 150_000
	MockFinalTermAmount = 450_000
	MockTrace           = "mock-irancell-0001"
)

// MockFixtures returns the fixed answers of the Irancell mock (debug.mocks.irancell): Inquiry
// answers with the two inquiries above; Status answers success (task 050; the failed and pending
// answers come from debug.mocks.irancell.status_return_type); Pay answers success (task 056; the
// failed and pending answers come from debug.mocks.irancell.pay_return_type).
func MockFixtures() provider.MockFixtures {
	return provider.MockFixtures{Status: mockStatus, Pay: mockPay, Inquiry: func(provider.InquiryRequest) *provider.InquiryResponse {
		return &provider.InquiryResponse{
			Status:          provider.ResponseSuccess,
			Code:            200,
			Message:         successMessage,
			ProviderTraceID: MockTrace,
			Inquiries: []provider.InquiryItem{
				{Amount: MockMidtermAmount, InquiryType: provider.InquiryTypeMidterm},
				{Amount: MockFinalTermAmount, InquiryType: provider.InquiryTypeFinalTerm},
			},
		}
	}}
}

// mockPay is the success answer of the Irancell mock to a payment (task 056): the answer of the
// irancell service to a success (code 200, its success text), with no trace ID, as Pay gives it
// (the tid is not kept). The failed and pending answers come from debug.mocks.irancell.pay_return_type.
func mockPay(provider.PaymentRequest) *provider.PaymentResponse {
	return &provider.PaymentResponse{Status: provider.StatusSuccess, Code: 200, Message: successMessage}
}

// mockStatus is the success answer of the Irancell mock to a status call (task 050).
func mockStatus(provider.StatusRequest) *provider.StatusResponse {
	return &provider.StatusResponse{Status: provider.StatusSuccess, Message: successMessage, ProviderTraceID: MockTrace}
}
