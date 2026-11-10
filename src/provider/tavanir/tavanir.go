// Package tavanir is the adapter of the Tavanir provider (part of Qabz, system.html). It calls the
// Tavanir backend over gRPC (haf80.api.tavanir.InternalInquiryService/BillInquiry, pb/proto/tavanir).
// Only Inquiry of an electricity bill is implemented (task 022); every other method returns
// provider.ErrNotSupported (the bill document, BillDocument, is a later task). The rules copy the
// old bill service (gateway/inquiry/electricity.go).
package tavanir

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "qabz/api/pb/tavanir"
	"qabz/provider"
)

// Name is the providers.name of Tavanir.
const Name = "tavanir"

// inquiryTypeRegular is the inquiry_type of an electricity inquiry (bill: entity.RegularInquiryType).
const inquiryTypeRegular = "regular"

// successMessage is the message of an answer with inquiries (bill: "the operation was done successfully").
const successMessage = "عملیات با موفقیت انجام شد"

// errNoAddress is the error of every call of a client made with no address.
var errNoAddress = errors.New("tavanir: providers.tavanir.grpc_address is not set")

// Client is the part of the generated gRPC client (pb.InternalInquiryServiceClient) that the
// adapter uses. Tests give a fake.
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

// Adapter implements provider.ProviderInterface for Tavanir. It is safe for concurrent use.
type Adapter struct {
	client  Client
	timeout time.Duration
}

var _ provider.ProviderInterface = (*Adapter)(nil)

// New returns the adapter.
func New(opts Options) (*Adapter, error) {
	if opts.Client == nil {
		return nil, errors.New("tavanir: client is required")
	}
	if opts.Timeout <= 0 {
		return nil, errors.New("tavanir: timeout must be positive")
	}
	return &Adapter{client: opts.Client, timeout: opts.Timeout}, nil
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
		return nil, fmt.Errorf("tavanir: grpc client for %s: %w", addr, err)
	}
	return pb.NewInternalInquiryServiceClient(conn), nil
}

type noClient struct{}

func (noClient) BillInquiry(context.Context, *pb.BillInquiryRequest, ...grpc.CallOption) (*pb.BillInquiryResponse, error) {
	return nil, errNoAddress
}

// Name returns Name.
func (a *Adapter) Name() string { return Name }

// Inquiry asks the backend for the inquiry of an electricity bill (bill: TavanirElectricity.Execute).
// A call that fails (a connection error, a timeout, a nil answer) returns an error that wraps
// provider.ErrProviderDown. An answer with inquiries is ResponseSuccess, with the amount, the pay
// ID and the payment deadline of each inquiry as the backend sends them (an empty pay ID stays
// empty) and the details of the answer (nil when the backend sends none). An answer with no
// inquiry keeps the status, code and message of the backend and has no details and no bill ID, as
// in bill; a "success" answer with no inquiry gives one inquiry with amount 0 (no debt). Tavanir
// sends no trace ID, so ProviderTraceID is always empty.
// @todo bill stores an empty status as an empty status; Qabz stores ResponseFailed, as for Qabzino.
// See docs/this_vs_bill.md, item 24.
func (a *Adapter) Inquiry(ctx context.Context, req provider.InquiryRequest) (*provider.InquiryResponse, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("tavanir: adapter not created with New")
	}
	if req.BillType != provider.BillTypeElectricity {
		return nil, fmt.Errorf("tavanir: inquiry of %q: %w", req.BillType, provider.ErrNotSupported)
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
		BillType:  req.BillType,
	})
	if err != nil {
		return nil, fmt.Errorf("tavanir: bill inquiry: %w: %w", provider.ErrProviderDown, err)
	}
	if resp == nil {
		return nil, fmt.Errorf("tavanir: bill inquiry: %w: empty answer", provider.ErrProviderDown)
	}
	base := resp.GetBaseResponse()
	if len(resp.GetInquiries()) > 0 {
		items := make([]provider.InquiryItem, 0, len(resp.GetInquiries()))
		for _, inq := range resp.GetInquiries() {
			if inq == nil {
				continue
			}
			items = append(items, provider.InquiryItem{
				PayID:           inq.GetPayId(),
				Amount:          inq.GetAmount(),
				PaymentDeadline: inq.GetPaymentDeadline(),
				InquiryType:     inquiryTypeRegular,
			})
		}
		if len(items) > 0 {
			return &provider.InquiryResponse{
				Status:    provider.ResponseSuccess,
				Code:      200,
				Message:   successMessage,
				BillID:    base.GetBillId(),
				Inquiries: items,
				Details:   details(resp.GetDetails()),
			}, nil
		}
	}
	out := &provider.InquiryResponse{
		Status:  base.GetStatus(),
		Code:    base.GetCode(),
		Message: base.GetMessage(),
	}
	if out.Status == "" {
		out.Status = provider.ResponseFailed
	}
	if out.Status == provider.ResponseSuccess { // no debt: the bill is paid
		out.Inquiries = []provider.InquiryItem{{InquiryType: inquiryTypeRegular}}
	}
	return out, nil
}

// details maps the details of the backend (bill: TavanirElectricity.Execute). Nil gives nil. As in
// bill, City is the company name: the backend has no city field, and the order flow of bill reads
// the saved city.
func details(d *pb.Details) *provider.Details {
	if d == nil {
		return nil
	}
	return &provider.Details{Electricity: &provider.ElectricityDetails{
		Phase:           d.GetPhase(),
		CompanyName:     d.GetCompanyName(),
		TariffType:      d.GetTariffType(),
		CustomerType:    d.GetCustomerType(),
		SerialNumber:    d.GetSerialNumber(),
		ServiceAddress:  d.GetServiceAddress(),
		ServicePostCode: d.GetServicePostCode(),
		CustomerName:    d.GetCustomerName(),
		City:            d.GetCompanyName(),
		Region:          d.GetRegion(),
	}}
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
