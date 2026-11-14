// Package irancellproxy is the Irancell adapter (task 035): it calls the irancell service, one of
// our own services, over gRPC (pb.InternalService/BillInquiry, pb/proto/irancell). The irancell
// service calls Irancell; Qabz does not. Only Inquiry of an Irancell mobile bill is implemented;
// every other method returns provider.ErrNotSupported. The rules copy the old bill service
// (gateway/inquiry/mobile_irancell.go, IrancellMobile, and base.go
// handlePhoneResponseWithoutInquiry).
package irancellproxy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "qabz/api/pb/irancell"
	"qabz/provider"
)

// Name is the providers.name of the Irancell provider (seed row 38).
const Name = "irancell"

// successMessage is the message of an answer with inquiries (bill: "the operation was done successfully").
const successMessage = "عملیات با موفقیت انجام شد"

var (
	// errNoAddress is the cause of every call of a client made with no address.
	errNoAddress = errors.New("providers.internal.irancell.grpc_address is not set")
	// errDisabled is the cause of every call while providers.internal.irancell.enabled is false.
	errDisabled = errors.New("providers.internal.irancell.enabled is false")
)

// Client is the part of the generated gRPC client (pb.InternalServiceClient) that the adapter
// uses. Tests give a fake.
type Client interface {
	BillInquiry(ctx context.Context, in *pb.BillInquiryRequest, opts ...grpc.CallOption) (*pb.BillInquiryResponse, error)
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
}

// Adapter implements provider.ProviderInterface for Irancell. It is safe for concurrent use.
type Adapter struct {
	enabled bool
	client  Client
	timeout time.Duration
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
	return &Adapter{enabled: opts.Enabled, client: opts.Client, timeout: opts.Timeout}, nil
}

// Dial returns a client for the irancell service at addr (host:port of its internal gRPC server,
// no TLS, as in bill). The connection is lazy, so Dial does not need a running service; it lives as
// long as the process. An empty addr gives a client whose every call fails (so the app starts
// without the key and an inquiry then answers "provider down").
func Dial(addr string) (Client, error) {
	if addr == "" {
		return noClient{}, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("irancellproxy: grpc client for %s: %w", addr, err)
	}
	return pb.NewInternalServiceClient(conn), nil
}

type noClient struct{}

func (noClient) BillInquiry(context.Context, *pb.BillInquiryRequest, ...grpc.CallOption) (*pb.BillInquiryResponse, error) {
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

// Mock fixtures (task 035). The values are fixed: a mid-term inquiry of 150 000 rials and a
// final-term inquiry of 450 000 rials, with no bill ID, no pay ID and no payment deadline (as a
// real Irancell answer).
const (
	MockMidtermAmount   = 150_000
	MockFinalTermAmount = 450_000
	MockTrace           = "mock-irancell-0001"
)

// MockFixtures returns the fixed answers of the Irancell mock (debug.mocks.irancell): Inquiry
// answers with the two inquiries above.
func MockFixtures() provider.MockFixtures {
	return provider.MockFixtures{Inquiry: func(provider.InquiryRequest) *provider.InquiryResponse {
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
