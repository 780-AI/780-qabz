// Package parsianproxy is the Parsian adapter (task 036): it calls the parsian-v2 service, one of
// our own services, over gRPC (TopBill/Inquiry and TopBill/Pay, pb/proto/parsian-v2). The parsian-v2
// service calls Parsian (TOP); Qabz does not. Inquiry of an MCI mobile bill (the bill type "mobile")
// and Pay of a landline bill (the bill types tel and telmidterm, task 044) are implemented; every
// other method returns provider.ErrNotSupported. The rules copy the old bill
// service (gateway/inquiry/mobile_mci.go, ParsianMCIMobile, and base.go
// handlePhoneResponseWithoutInquiry).
package parsianproxy

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "qabz/api/pb/parsian-v2"
	"qabz/provider"
)

// Name is the providers.name of the Parsian provider (seed rows 39 and 41).
const Name = "parsian"

// BillTypeMCI is the bill type that the parsian-v2 service knows for an MCI mobile bill (bill:
// entity.MOBILEMCI). It is not a providers.bill_type.
const BillTypeMCI = "mobile_mci"

// mciReference is the first digit of the tracking ID and the reference ID of an MCI inquiry (bill:
// pkg/reference MCIMobileReference).
const mciReference = 5

// successMessage is the message of an answer with inquiries (bill: "the operation was done successfully").
const successMessage = "عملیات با موفقیت انجام شد"

var (
	// errNoAddress is the cause of every call of a client made with no address.
	errNoAddress = errors.New("providers.internal.parsian.grpc_address is not set")
	// errDisabled is the cause of every call while providers.internal.parsian.enabled is false.
	errDisabled = errors.New("providers.internal.parsian.enabled is false")
)

// Client is the part of the generated gRPC client (pb.TopBillClient) that the adapter uses. Tests
// give a fake.
type Client interface {
	Inquiry(ctx context.Context, in *pb.BillInquiryRequest, opts ...grpc.CallOption) (*pb.BillInquiryResponse, error)
	Pay(ctx context.Context, in *pb.PayRequest, opts ...grpc.CallOption) (*pb.PayResponse, error)
	OrderInquiry(ctx context.Context, in *pb.OrderInquiryRequest, opts ...grpc.CallOption) (*pb.OrderInquiryResponse, error)
}

// Options configure New.
type Options struct {
	// Enabled is providers.internal.parsian.enabled. False: no call is sent; every Inquiry fails as
	// "provider down".
	Enabled bool
	// Client calls the parsian-v2 service. Required.
	Client Client
	// Timeout limits each call. Required, positive.
	Timeout time.Duration
}

// Adapter implements provider.ProviderInterface for Parsian. It is safe for concurrent use.
type Adapter struct {
	enabled bool
	client  Client
	timeout time.Duration
	// newRef makes a tracking ID or a reference ID. Tests replace it.
	newRef func() string
}

var _ provider.ProviderInterface = (*Adapter)(nil)

// New returns the adapter.
func New(opts Options) (*Adapter, error) {
	if opts.Client == nil {
		return nil, errors.New("parsianproxy: client is required")
	}
	if opts.Timeout <= 0 {
		return nil, errors.New("parsianproxy: timeout must be positive")
	}
	return &Adapter{enabled: opts.Enabled, client: opts.Client, timeout: opts.Timeout, newRef: newReference}, nil
}

// newReference makes a tracking ID or a reference ID of an MCI inquiry (bill:
// GenerateBillInquiryTrackingID, GenerateBillInquiryReferenceID): the prefix 5, a random number
// from 100 to 90000000099 and the day of the year.
func newReference() string {
	return fmt.Sprintf("%d%d%d", mciReference, rand.Int64N(90000000000)+100, time.Now().YearDay())
}

// Dial returns a client for the parsian-v2 service at addr (host:port of its internal gRPC server,
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
		return nil, fmt.Errorf("parsianproxy: grpc client for %s: %w", addr, err)
	}
	return pb.NewTopBillClient(conn), nil
}

type noClient struct{}

func (noClient) Inquiry(context.Context, *pb.BillInquiryRequest, ...grpc.CallOption) (*pb.BillInquiryResponse, error) {
	return nil, errNoAddress
}

func (noClient) Pay(context.Context, *pb.PayRequest, ...grpc.CallOption) (*pb.PayResponse, error) {
	return nil, errNoAddress
}

func (noClient) OrderInquiry(context.Context, *pb.OrderInquiryRequest, ...grpc.CallOption) (*pb.OrderInquiryResponse, error) {
	return nil, errNoAddress
}

// Name returns Name.
func (a *Adapter) Name() string { return Name }

// SupportsInquiry reports whether Inquiry answers the bill type: only the MCI mobile bill (Parsian answers other bill types in bill; this adapter does not) (task 036).
func (a *Adapter) SupportsInquiry(billType string) bool { return billType == provider.BillTypeMobile }

// Enabled reports providers.internal.parsian.enabled.
func (a *Adapter) Enabled() bool { return a != nil && a.enabled }

// Inquiry asks the parsian-v2 service for the inquiry of an MCI mobile bill (bill:
// ParsianMCIMobile.Execute). The inquiry ID is the phone of the request, sent as it is, with a new
// tracking ID, a new reference ID and the bill type BillTypeMCI.
//   - A call that fails (the switch is off, no address, a connection error, a timeout, a nil
//     answer) returns an error that wraps provider.ErrProviderDown.
//   - An answer with inquiries is ResponseSuccess: the bill ID and the tid of the answer and one
//     item for each inquiry, with its pay ID, its amount, its payment deadline and its inquiry type
//     (provider.InquiryTypeMidterm, provider.InquiryTypeFinalTerm).
//   - An answer with no inquiry keeps the status, code, message, tid and bill ID of the service (an
//     empty status is ResponseFailed). When its status is "success" (no debt), it gives a mid-term
//     and a final-term inquiry of amount 0.
//
// @todo bill picks any enabled row of (mobile, inquiry); Qabz picks by priority, and an empty status
// of an answer with no inquiry is failed. See docs/this_vs_bill.md, item 32.
func (a *Adapter) Inquiry(ctx context.Context, req provider.InquiryRequest) (*provider.InquiryResponse, error) {
	if a == nil || a.client == nil || a.newRef == nil {
		return nil, errors.New("parsianproxy: adapter not created with New")
	}
	if !a.SupportsInquiry(req.BillType) {
		return nil, fmt.Errorf("parsianproxy: inquiry of %q: %w", req.BillType, provider.ErrNotSupported)
	}
	if !a.enabled {
		return nil, fmt.Errorf("parsianproxy: bill inquiry: %w: %w", provider.ErrProviderDown, errDisabled)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.Inquiry(ctx, &pb.BillInquiryRequest{
		UserId:      req.UserID,
		Dialer:      req.Dialer,
		InquiryId:   req.InquiryID,
		TrackingId:  a.newRef(),
		ReferenceId: a.newRef(),
		BillType:    BillTypeMCI,
	})
	if err != nil {
		return nil, fmt.Errorf("parsianproxy: bill inquiry: %w: %w", provider.ErrProviderDown, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("parsianproxy: bill inquiry: %w: empty answer", provider.ErrProviderDown)
	}
	base := resp.GetBaseResponse()
	items := make([]provider.InquiryItem, 0, len(resp.GetInquiries()))
	for _, inq := range resp.GetInquiries() {
		if inq == nil {
			continue
		}
		items = append(items, provider.InquiryItem{
			PayID: inq.GetPayId(), Amount: inq.GetAmount(), PaymentDeadline: inq.GetPaymentDeadline(), InquiryType: inq.GetInquiryType(),
		})
	}
	if len(items) > 0 {
		return &provider.InquiryResponse{
			Status:          provider.ResponseSuccess,
			Code:            200,
			Message:         successMessage,
			ProviderTraceID: base.GetTid(),
			BillID:          base.GetBillId(),
			Inquiries:       items,
		}, nil
	}
	out := &provider.InquiryResponse{
		Status:          base.GetStatus(),
		Code:            base.GetCode(),
		Message:         base.GetMessage(),
		ProviderTraceID: base.GetTid(),
		BillID:          base.GetBillId(),
	}
	if out.Status == "" {
		out.Status = provider.ResponseFailed
	}
	if out.Status == provider.ResponseSuccess { // no debt
		out.Inquiries = provider.NoDebtMobileInquiries()
	}
	return out, nil
}

// payBillTypes are the bill types that Pay pays (task 044; bill: ParsianTelephone for TEL and
// TELMIDTERM).
var payBillTypes = map[string]bool{provider.BillTypeTel: true, provider.BillTypeTelMidterm: true}

// SupportsPay reports whether Pay pays the bill type: tel and telmidterm.
func SupportsPay(billType string) bool { return payBillTypes[billType] }

// Pay pays a landline bill through the parsian-v2 service (TopBill/Pay; bill: ParsianTelephone.Execute
// of gateway/payment/telephone.go). The request has the fields of bill: provider_order_id is the RRN
// as an integer, payment_rrn the RRN, amount the base price, inquiry_id the landline number and
// bill_type the order bill type.
//   - An RRN that is not an integer (strconv.Atoi, as bill) is a failed answer (code 400, the
//     error text as the message) with no call.
//   - A call that fails (the switch is off, no address, a connection error, a timeout, a nil answer)
//     returns an error that wraps provider.ErrProviderDown: the status is not known (bill: pending,
//     code 500, the error text).
//   - Else the answer of the service as it came (code, status, message, provider_trace_id as text).
//
// @todo bill sets no timeout of its own (the deadline of the payment service applies); Qabz limits
// the call with providers.internal.parsian.timeout. See docs/this_vs_bill.md, item 40.
func (a *Adapter) Pay(ctx context.Context, req provider.PaymentRequest) (*provider.PaymentResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("parsianproxy: adapter not created with New")
	}
	if !SupportsPay(req.BillType) {
		return nil, fmt.Errorf("parsianproxy: pay of %q: %w", req.BillType, provider.ErrNotSupported)
	}
	tid, err := strconv.Atoi(req.RRN)
	if err != nil {
		return &provider.PaymentResponse{Code: http.StatusBadRequest, Status: provider.StatusFailed, Message: err.Error()}, nil
	}
	if !a.enabled {
		return nil, fmt.Errorf("parsianproxy: pay: %w: %w", provider.ErrProviderDown, errDisabled)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.Pay(ctx, &pb.PayRequest{
		UserId:          req.UserID,
		Dialer:          req.Dialer,
		OrderId:         req.OrderID,
		ProviderOrderId: int64(tid),
		InquiryId:       req.InquiryID,
		BillType:        req.BillType,
		InquiryType:     req.InquiryType,
		BillId:          req.BillID,
		PayId:           req.PayID,
		Amount:          req.Amount,
		PaymentRrn:      req.RRN,
	})
	if err != nil {
		return nil, fmt.Errorf("parsianproxy: pay: %w: %w", provider.ErrProviderDown, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("parsianproxy: pay: %w: empty answer", provider.ErrProviderDown)
	}
	return &provider.PaymentResponse{
		Code:            resp.GetCode(),
		Status:          resp.GetStatus(),
		Message:         resp.GetMessage(),
		ProviderTraceID: strconv.FormatUint(resp.GetProviderTraceId(), 10),
	}, nil
}

// Status asks parsian-v2 the status of a paid order (task 045; bill: payment_inquiry_gateway
// ParsianPaymentInquiry, TopBill/OrderInquiry with order_id and bill_type). The status of the answer
// is given as it came; only success and failed are final. A gRPC error (a timeout too), an empty
// answer and providers.internal.parsian.enabled false are ErrProviderDown (bill: pending, the error
// text as the message). Each call has providers.internal.parsian.timeout (bill sets none).
func (a *Adapter) Status(ctx context.Context, req provider.StatusRequest) (*provider.StatusResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("parsianproxy: adapter not created with New")
	}
	if !a.enabled {
		return nil, fmt.Errorf("parsianproxy: order inquiry: %w: %w", provider.ErrProviderDown, errDisabled)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.OrderInquiry(ctx, &pb.OrderInquiryRequest{OrderId: req.OrderID, BillType: req.BillType})
	if err != nil {
		return nil, fmt.Errorf("parsianproxy: order inquiry: %w: %w", provider.ErrProviderDown, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("parsianproxy: order inquiry: %w: empty answer", provider.ErrProviderDown)
	}
	return &provider.StatusResponse{
		Status:          resp.GetStatus(),
		Message:         resp.GetMessage(),
		ProviderTraceID: strconv.FormatUint(resp.GetProviderTraceId(), 10),
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

// Mock fixtures (task 036). The values are fixed and differ from the Jiring Switch mock, so a test
// run shows which provider answered: a mid-term inquiry of 220 000 rials and a final-term inquiry of
// 620 000 rials, each with a pay ID and the payment deadline 2026-12-20, one bill ID and a trace ID.
const (
	MockBillID          = "9120000039"
	MockMidtermPayID    = "22003911"
	MockMidtermAmount   = 220_000
	MockFinalTermPayID  = "62003922"
	MockFinalTermAmount = 620_000
	MockPaymentDeadline = "2026-12-20"
	MockTrace           = "mock-parsian-0001"
)

// MockPayTrace is the provider trace ID of a payment that the Parsian mock answers as a success
// (task 044).
const MockPayTrace = "9900000044"

// MockFixtures returns the fixed answers of the Parsian mock (debug.mocks.parsian): Inquiry answers
// with the two inquiries above; Pay (task 044) answers a success with the trace ID MockPayTrace.
func MockFixtures() provider.MockFixtures {
	return provider.MockFixtures{Pay: mockPay, Status: mockStatus, Inquiry: func(provider.InquiryRequest) *provider.InquiryResponse {
		return &provider.InquiryResponse{
			Status:          provider.ResponseSuccess,
			Code:            200,
			Message:         successMessage,
			ProviderTraceID: MockTrace,
			BillID:          MockBillID,
			Inquiries: []provider.InquiryItem{
				{PayID: MockMidtermPayID, Amount: MockMidtermAmount, PaymentDeadline: MockPaymentDeadline, InquiryType: provider.InquiryTypeMidterm},
				{PayID: MockFinalTermPayID, Amount: MockFinalTermAmount, PaymentDeadline: MockPaymentDeadline, InquiryType: provider.InquiryTypeFinalTerm},
			},
		}
	}}
}

// mockStatus is the success answer of the Parsian mock to a status call (task 045).
func mockStatus(provider.StatusRequest) *provider.StatusResponse {
	return &provider.StatusResponse{Status: provider.StatusSuccess, Message: successMessage, ProviderTraceID: MockPayTrace}
}

// mockPay is the success answer of the Parsian mock to a payment.
func mockPay(provider.PaymentRequest) *provider.PaymentResponse {
	return &provider.PaymentResponse{Code: http.StatusOK, Status: provider.StatusSuccess, Message: successMessage, ProviderTraceID: MockPayTrace}
}
