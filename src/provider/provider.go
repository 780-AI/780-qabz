package provider

import (
	"context"
	"errors"
)

// Bill types and actions that adapters and the flow use (providers.bill_type, providers.action).
const (
	BillTypeWater       = "water"
	BillTypeElectricity = "electricity"

	ActionInquiry = "inquiry"

	ResponseSuccess = "success" // InquiryResponse.Status of a call that the provider answered
	ResponseFailed  = "failed"  // InquiryResponse.Status of a call that failed
)

var (
	// ErrNotSupported is the error of a ProviderInterface method that the adapter does not
	// implement (yet). Test it with errors.Is.
	ErrNotSupported = errors.New("provider: method not supported")
	// ErrProviderDown is the error of an Inquiry call that did not reach the provider or got no
	// answer from it: a connection error, a timeout, a mock with return_type error. It is a
	// dependency failure (conventions § 10). Test it with errors.Is.
	ErrProviderDown = errors.New("provider: inquiry is down")
)

// InquiryRequest is the input of ProviderInterface.Inquiry.
type InquiryRequest struct {
	BillType  string // bill type, for example BillTypeWater
	UserID    string // the user, a UUID
	Dialer    string // the mobile number of the user
	InquiryID string // the ID that the provider inquires: the bill ID of a water or an electricity bill
}

// InquiryItem is one inquiry of an answer (one amount to pay).
type InquiryItem struct {
	PayID           string
	Amount          uint64 // rials
	PaymentDeadline string // as the provider sends it
	InquiryType     string // "regular" for a water and an electricity bill
}

// ElectricityDetails are the details of an electricity inquiry (bill: gateway/inquiry
// ElectricityDetails). The flow saves them with the inquiry (bill_inquiries.extra_data), so an
// answer from saved inquiries has the same details as an answer of the provider (task 022).
type ElectricityDetails struct {
	// Shown to the user.
	Phase           string
	CompanyName     string
	TariffType      string
	CustomerType    int32
	SerialNumber    string
	ServiceAddress  string
	ServicePostCode string
	CustomerName    string
	// Read by the order flow of bill (the fee rules).
	City   string
	Region string
}

// Details are the details of an inquiry answer, one field for each bill type that has details. A
// nil *Details and a Details with no field set mean "no details". The flow and the repository treat
// a Details value as read-only after the adapter returned it.
type Details struct {
	Electricity *ElectricityDetails
}

// InquiryResponse is the answer of ProviderInterface.Inquiry. A provider that answers "no
// inquiry" (a bill that is not found, a failed status) gives no Inquiries and a Status other than
// ResponseSuccess, with its Message. A paid bill is Status ResponseSuccess and one item with
// Amount 0 and no PayID. Details is nil for a provider or a bill type with no details.
type InquiryResponse struct {
	Status          string
	Code            int32
	Message         string
	ProviderTraceID string
	BillID          string
	Inquiries       []InquiryItem
	Details         *Details
}

// The request and response types of the methods that no adapter implements yet. Each later task
// that implements one of them gives its type the fields it needs.
type (
	PaymentRequest   struct{}
	PaymentResponse  struct{}
	StatusRequest    struct{}
	StatusResponse   struct{}
	DocumentRequest  struct{}
	DocumentResponse struct{}
	SettleRequest    struct{}
	SettleResponse   struct{}
)

// ProviderInterface is what every provider adapter implements (system.html, Provider Adapter
// Interface). A method that the adapter does not implement returns ErrNotSupported. Inquiry
// returns ErrProviderDown (wrapped) when the provider cannot be reached.
type ProviderInterface interface {
	// Name is the providers.name of the provider (for example "qabzino").
	Name() string
	Inquiry(ctx context.Context, req InquiryRequest) (*InquiryResponse, error)
	Pay(ctx context.Context, req PaymentRequest) (*PaymentResponse, error)
	Status(ctx context.Context, req StatusRequest) (*StatusResponse, error)
	GetDocument(ctx context.Context, req DocumentRequest) (*DocumentResponse, error)
	Settle(ctx context.Context, req SettleRequest) (*SettleResponse, error)
}
