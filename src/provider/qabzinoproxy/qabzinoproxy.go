// Package qabzinoproxy is QabzinoProxy: the adapter that calls the old qabzino service over gRPC
// (haf80.api.qabzino.Inquiry/BillInquiry, pb/proto/qabzino). Task 030 renamed it from the Qabzino
// adapter (task 021) and replaced it with the direct adapter in package qabzino. It is kept but
// NOT used: the App does not build or register it, and its provider name differs from the
// providers.name "qabzino", so it cannot shadow the direct adapter. Only
// Inquiry of a water bill is implemented (task 021); every other method returns
// provider.ErrNotSupported. The rules copy the old bill service (gateway/inquiry/water.go).
package qabzinoproxy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "qabz/api/pb/qabzino"
	"qabz/provider"
)

// Name is the name of QabzinoProxy. It is not a providers.name of the seed: the proxy is not used.
const Name = "qabzinoproxy"

// inquiryTypeRegular is the inquiry_type of a water inquiry (bill: entity.RegularInquiryType).
const inquiryTypeRegular = "regular"

// successMessage is the message of an answer with inquiries (bill: "the operation was done successfully").
const successMessage = "عملیات با موفقیت انجام شد"

// errNoAddress is the error of every call of a client made with no address.
var errNoAddress = errors.New("qabzinoproxy: providers.qabzino.grpc_address is not set")

// Client is the part of the generated gRPC client (pb.InquiryClient) that the adapter uses. Tests
// give a fake.
type Client interface {
	BillInquiry(ctx context.Context, in *pb.BillInquiryRequest, opts ...grpc.CallOption) (*pb.BillInquiryResponse, error)
}

// Options configure New.
type Options struct {
	// Client calls the backend. Required.
	Client Client
	// Timeout limits each call. Required, positive.
	Timeout time.Duration
}

// Adapter implements provider.ProviderInterface for Qabzino. It is safe for concurrent use.
type Adapter struct {
	client  Client
	timeout time.Duration
	// newTrace makes the trace number of a call (bill: a new UUID). Tests replace it.
	newTrace func() string
}

var _ provider.ProviderInterface = (*Adapter)(nil)

// New returns the adapter.
func New(opts Options) (*Adapter, error) {
	if opts.Client == nil {
		return nil, errors.New("qabzinoproxy: client is required")
	}
	if opts.Timeout <= 0 {
		return nil, errors.New("qabzinoproxy: timeout must be positive")
	}
	return &Adapter{client: opts.Client, timeout: opts.Timeout, newTrace: func() string { return uuid.NewString() }}, nil
}

// Dial returns a client for the backend at addr (host:port, no TLS, as in bill). The connection is
// lazy, so Dial does not need a running backend; it lives as long as the process. An empty addr
// gives a client whose every call fails (so the app starts without the key and an inquiry then
// answers "provider down").
func Dial(addr string) (Client, error) {
	if addr == "" {
		return noClient{}, nil
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("qabzinoproxy: grpc client for %s: %w", addr, err)
	}
	return pb.NewInquiryClient(conn), nil
}

type noClient struct{}

func (noClient) BillInquiry(context.Context, *pb.BillInquiryRequest, ...grpc.CallOption) (*pb.BillInquiryResponse, error) {
	return nil, errNoAddress
}

// Name returns Name.
func (a *Adapter) Name() string { return Name }

// SupportsInquiry reports whether Inquiry answers the bill type: only the water bill (task 036).
func (a *Adapter) SupportsInquiry(billType string) bool { return billType == provider.BillTypeWater }

// Inquiry asks the backend for the inquiry of a water bill (bill: QabzinoWater.Execute). A call
// that fails (a connection error, a timeout, a nil answer) returns an error that wraps
// provider.ErrProviderDown. An answer with inquiries is ResponseSuccess; an answer with none
// keeps the status, code and message of the backend (an empty status is ResponseFailed). A
// "success" answer with no inquiry gives one inquiry with amount 0 (a paid bill: the backend
// sends no pay ID).
// @todo bill drops the trace ID of an answer with no inquiry. See docs/this_vs_bill.md, item 18.
func (a *Adapter) Inquiry(ctx context.Context, req provider.InquiryRequest) (*provider.InquiryResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("qabzinoproxy: adapter not created with New")
	}
	if !a.SupportsInquiry(req.BillType) {
		return nil, fmt.Errorf("qabzinoproxy: inquiry of %q: %w", req.BillType, provider.ErrNotSupported)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	resp, err := a.client.BillInquiry(ctx, &pb.BillInquiryRequest{
		UserId:      req.UserID,
		Dialer:      req.Dialer,
		InquiryId:   req.InquiryID,
		TraceNumber: a.newTrace(),
		BillType:    req.BillType,
	})
	if err != nil {
		return nil, fmt.Errorf("qabzinoproxy: bill inquiry: %w: %w", provider.ErrProviderDown, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("qabzinoproxy: bill inquiry: %w: empty answer", provider.ErrProviderDown)
	}
	base := resp.GetBaseResponse()
	if len(resp.GetInquiries()) > 0 {
		items := make([]provider.InquiryItem, 0, len(resp.GetInquiries()))
		for _, inq := range resp.GetInquiries() {
			if inq == nil {
				continue
			}
			amount := uint64(0)
			// The backend sends an empty pay ID for a paid bill, but its amount can be above zero.
			if inq.GetPayId() != "" {
				var ok bool
				if amount, ok = amountFromPayID(inq.GetPayId()); !ok {
					amount = inq.GetAmount()
				}
			}
			items = append(items, provider.InquiryItem{
				PayID:           inq.GetPayId(),
				Amount:          amount,
				PaymentDeadline: inq.GetPaymentDeadline(),
				InquiryType:     inquiryTypeRegular,
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
	}
	out := &provider.InquiryResponse{
		Status:          base.GetStatus(),
		Code:            base.GetCode(),
		Message:         base.GetMessage(),
		ProviderTraceID: base.GetTid(),
	}
	if out.Status == "" {
		out.Status = provider.ResponseFailed
	}
	if out.Status == provider.ResponseSuccess { // no debt: the bill is paid
		out.Inquiries = []provider.InquiryItem{{InquiryType: inquiryTypeRegular}}
	}
	return out, nil
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

// Mock fixtures (task 021). The values are fixed: a valid water inquiry. The pay ID follows the
// rule of amountFromPayID (the digits before the last 5 are the amount in thousands of rials):
// "25000" gives 25 000 000 rials.
const (
	mockPayID    = "2500012345"
	mockAmount   = 25_000_000
	mockDeadline = "2030-01-01"
	mockTrace    = "mock-qabzino-0001"
)

// MockFixtures returns the fixed answers of the Qabzino mock (debug.mocks.qabzino): Inquiry
// answers the bill of the request with one inquiry of 25 000 000 rials (pay ID 2500012345,
// deadline 2030-01-01).
func MockFixtures() provider.MockFixtures {
	return provider.MockFixtures{Inquiry: func(req provider.InquiryRequest) *provider.InquiryResponse {
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
