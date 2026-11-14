// Package jiring is the Jiring Switch adapter (task 036): it calls the jiring-switch service, one
// of our own services, over gRPC (InternalInquiryService/BillInquiry, pb/proto/jiringSwitch). The
// jiring-switch service calls Jiring; Qabz does not. Only Inquiry of an MCI mobile bill (the bill
// type "mobile") is implemented; every other method returns provider.ErrNotSupported. The rules
// copy the old bill service (gateway/inquiry/mobile_mci.go, JiringSwitchMCIMobile, and base.go
// handlePhoneResponseWithoutInquiry).
package jiring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "qabz/api/pb/jiringSwitch"
	"qabz/provider"
)

// Name is the providers.name of the Jiring Switch provider (seed rows 27 and 28).
const Name = "jiring-switch"

// successMessage is the message of an answer with inquiries (bill: "the operation was done successfully").
const successMessage = "عملیات با موفقیت انجام شد"

var (
	// errNoAddress is the cause of every call of a client made with no address.
	errNoAddress = errors.New("providers.internal.jiring_switch.grpc_address is not set")
	// errDisabled is the cause of every call while providers.internal.jiring_switch.enabled is false.
	errDisabled = errors.New("providers.internal.jiring_switch.enabled is false")
)

// Client is the part of the generated gRPC client (pb.InternalInquiryServiceClient) that the
// adapter uses. Tests give a fake.
type Client interface {
	BillInquiry(ctx context.Context, in *pb.BillInquiryRequest, opts ...grpc.CallOption) (*pb.BillInquiryResponse, error)
}

// Options configure New.
type Options struct {
	// Enabled is providers.internal.jiring_switch.enabled. False: no call is sent; every Inquiry fails as
	// "provider down".
	Enabled bool
	// Client calls the jiring-switch service. Required.
	Client Client
	// Timeout limits each call. Required, positive.
	Timeout time.Duration
}

// Adapter implements provider.ProviderInterface for Jiring Switch. It is safe for concurrent use.
type Adapter struct {
	enabled bool
	client  Client
	timeout time.Duration
}

var _ provider.ProviderInterface = (*Adapter)(nil)

// New returns the adapter.
func New(opts Options) (*Adapter, error) {
	if opts.Client == nil {
		return nil, errors.New("jiring: client is required")
	}
	if opts.Timeout <= 0 {
		return nil, errors.New("jiring: timeout must be positive")
	}
	return &Adapter{enabled: opts.Enabled, client: opts.Client, timeout: opts.Timeout}, nil
}

// Dial returns a client for the jiring-switch service at addr (host:port of its internal gRPC
// server, no TLS, as in bill). The connection is lazy, so Dial does not need a running service; it
// lives as long as the process. An empty addr gives a client whose every call fails (so the app
// starts without the key and an inquiry then answers "provider down").
func Dial(addr string) (Client, error) {
	if addr == "" {
		return noClient{}, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("jiring: grpc client for %s: %w", addr, err)
	}
	return pb.NewInternalInquiryServiceClient(conn), nil
}

type noClient struct{}

func (noClient) BillInquiry(context.Context, *pb.BillInquiryRequest, ...grpc.CallOption) (*pb.BillInquiryResponse, error) {
	return nil, errNoAddress
}

// Name returns Name.
func (a *Adapter) Name() string { return Name }

// SupportsInquiry reports whether Inquiry answers the bill type: only the MCI mobile bill (task 036).
func (a *Adapter) SupportsInquiry(billType string) bool { return billType == provider.BillTypeMobile }

// Enabled reports providers.internal.jiring_switch.enabled.
func (a *Adapter) Enabled() bool { return a != nil && a.enabled }

// Inquiry asks the jiring-switch service for the inquiry of an MCI mobile bill (bill:
// JiringSwitchMCIMobile.Execute). The inquiry ID is the phone of the request, sent as it is.
//   - A call that fails (the switch is off, no address, a connection error, a timeout, a nil
//     answer) returns an error that wraps provider.ErrProviderDown.
//   - An answer with inquiries is ResponseSuccess: the bill ID of the answer and one item for each
//     inquiry, with its pay ID, its amount and its inquiry type (provider.InquiryTypeMidterm,
//     provider.InquiryTypeFinalTerm). Jiring Switch gives no payment deadline and no trace ID.
//   - An answer with no inquiry keeps the status, code and message of the service (an empty status
//     is ResponseFailed). When its status is "success" (no debt), it gives a mid-term and a
//     final-term inquiry of amount 0. The service answers the status "pending" (code 500) when
//     Jiring itself did not respond: that is an answer of the provider, not a failed call (operator
//     rule of task 036: what a provider returns is the answer; only the Provider Switcher moves a
//     bill type to another provider).
//
// @todo bill picks any enabled row of (mobile, inquiry); Qabz picks by priority, and an empty status
// of an answer with no inquiry is failed. See docs/this_vs_bill.md, item 32.
func (a *Adapter) Inquiry(ctx context.Context, req provider.InquiryRequest) (*provider.InquiryResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("jiring: adapter not created with New")
	}
	if !a.SupportsInquiry(req.BillType) {
		return nil, fmt.Errorf("jiring: inquiry of %q: %w", req.BillType, provider.ErrNotSupported)
	}
	if !a.enabled {
		return nil, fmt.Errorf("jiring: bill inquiry: %w: %w", provider.ErrProviderDown, errDisabled)
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
		return nil, fmt.Errorf("jiring: bill inquiry: %w: %w", provider.ErrProviderDown, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("jiring: bill inquiry: %w: empty answer", provider.ErrProviderDown)
	}
	base := resp.GetBaseResponse()
	items := make([]provider.InquiryItem, 0, len(resp.GetInquiries()))
	for _, inq := range resp.GetInquiries() {
		if inq == nil {
			continue
		}
		items = append(items, provider.InquiryItem{PayID: inq.GetPayId(), Amount: inq.GetAmount(), InquiryType: inq.GetInquiryType()})
	}
	if len(items) > 0 {
		return &provider.InquiryResponse{
			Status:    provider.ResponseSuccess,
			Code:      200,
			Message:   successMessage,
			BillID:    base.GetBillId(),
			Inquiries: items,
		}, nil
	}
	// bill (handlePhoneResponseWithoutInquiry): the answer has no bill ID here.
	out := &provider.InquiryResponse{Status: base.GetStatus(), Code: base.GetCode(), Message: base.GetMessage()}
	if out.Status == "" {
		out.Status = provider.ResponseFailed
	}
	if out.Status == provider.ResponseSuccess { // no debt
		out.Inquiries = provider.NoDebtMobileInquiries()
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

// Mock fixtures (task 036). The values are fixed: a mid-term inquiry of 210 000 rials and a
// final-term inquiry of 610 000 rials, each with a pay ID, and one bill ID; no payment deadline and
// no trace ID (as a real Jiring Switch answer).
const (
	MockBillID          = "9120000036"
	MockMidtermPayID    = "21003611"
	MockMidtermAmount   = 210_000
	MockFinalTermPayID  = "61003622"
	MockFinalTermAmount = 610_000
)

// MockFixtures returns the fixed answers of the Jiring Switch mock (debug.mocks.jiring_switch):
// Inquiry answers with the two inquiries above.
func MockFixtures() provider.MockFixtures {
	return provider.MockFixtures{Inquiry: func(provider.InquiryRequest) *provider.InquiryResponse {
		return &provider.InquiryResponse{
			Status:  provider.ResponseSuccess,
			Code:    200,
			Message: successMessage,
			BillID:  MockBillID,
			Inquiries: []provider.InquiryItem{
				{PayID: MockMidtermPayID, Amount: MockMidtermAmount, InquiryType: provider.InquiryTypeMidterm},
				{PayID: MockFinalTermPayID, Amount: MockFinalTermAmount, InquiryType: provider.InquiryTypeFinalTerm},
			},
		}
	}}
}
