package provider

import (
	"context"
	"errors"
)

// Bill types and actions that adapters and the flow use (providers.bill_type, providers.action).
const (
	BillTypeWater   = "water"
	ActionInquiry   = "inquiry"
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
	InquiryID string // the ID that the provider inquires: the bill ID of a water bill
}

// InquiryItem is one inquiry of an answer (one amount to pay).
type InquiryItem struct {
	PayID           string
	Amount          uint64 // rials
	PaymentDeadline string // as the provider sends it
	InquiryType     string // "regular" for a water bill
}

// InquiryResponse is the answer of ProviderInterface.Inquiry. A provider that answers "no
// inquiry" (a bill that is not found, a failed status) gives no Inquiries and a Status other than
// ResponseSuccess, with its Message. A paid bill is Status ResponseSuccess and one item with
// Amount 0 and no PayID.
type InquiryResponse struct {
	Status          string
	Code            int32
	Message         string
	ProviderTraceID string
	BillID          string
	Inquiries       []InquiryItem
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
