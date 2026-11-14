package provider

import (
	"context"
	"errors"
)

// Bill types and actions that adapters and the flow use (providers.bill_type, providers.action).
const (
	BillTypeWater       = "water"
	BillTypeElectricity = "electricity"
	BillTypeGas         = "gas"
	// BillTypeIrancell is the bill type of an Irancell mobile bill (bill: entity.Irancell; task 035).
	BillTypeIrancell = "irancell"
	// BillTypeMobile is the bill type of an MCI mobile bill (bill: entity.MOBILE; task 036).
	BillTypeMobile = "mobile"
	// BillTypeMobileMidTermRightel is the bill type of a Rightel mobile bill (bill:
	// entity.MOBILEMIDTERMRIGHTEL; task 037).
	BillTypeMobileMidTermRightel = "mobilemidtermRightel"
	// BillTypeTel is the bill type of a landline (telephone) bill (bill: entity.TEL; task 025).
	BillTypeTel = "tel"

	ActionInquiry = "inquiry"

	ResponseSuccess = "success" // InquiryResponse.Status of a call that the provider answered
	ResponseFailed  = "failed"  // InquiryResponse.Status of a call that failed

	// The inquiry types of a mobile and a landline bill (bill: entity.MidtermInquiryType,
	// FinalTermInquiryType): one answer has a mid-term and a final-term (full-term) inquiry.
	InquiryTypeMidterm   = "midterm"
	InquiryTypeFinalTerm = "final_term"
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
	BillType string // bill type, for example BillTypeWater
	UserID   string // the user, a UUID
	Dialer   string // the mobile number of the user
	// InquiryID is the ID that the provider inquires: the bill ID of a water or an electricity bill,
	// the participate code (کد اشتراک) of a gas bill, the phone (or the bill ID) of a mobile bill, the
	// landline number (98 form) of a landline bill.
	InquiryID string
}

// InquiryItem is one inquiry of an answer (one amount to pay).
type InquiryItem struct {
	// BillID is the bill ID of this inquiry when the provider gives each inquiry its own bill ID
	// (Karashop: one call for each part of a landline bill, task 025). Empty: the bill ID of the
	// answer (InquiryResponse.BillID), as for every other provider.
	BillID          string
	PayID           string
	Amount          uint64 // rials
	PaymentDeadline string // as the provider sends it
	InquiryType     string // "regular" for a water, an electricity and a gas bill; InquiryTypeMidterm or InquiryTypeFinalTerm for a mobile and a landline bill
}

// NoDebtMobileInquiries returns the inquiries of a mobile bill with no debt (bill:
// handlePhoneResponseWithoutInquiry): a mid-term and a final-term inquiry of amount 0. The slice is
// new on each call.
func NoDebtMobileInquiries() []InquiryItem {
	return []InquiryItem{{InquiryType: InquiryTypeMidterm}, {InquiryType: InquiryTypeFinalTerm}}
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

// GasDetails are the details of a gas inquiry (bill: gateway/inquiry GasDetails). The client does
// not see them (InquiryBillPayResponse has no field for them); the flow saves them with the inquiry
// (bill_inquiries.extra_data), as bill does (task 023).
type GasDetails struct {
	City string // Qabzino: the address of the answer (bill maps it so)
	Kind string // Qabzino: the consumption type of the answer
}

// Details are the details of an inquiry answer, one field for each bill type that has details. A
// nil *Details and a Details with no field set mean "no details". The flow and the repository treat
// a Details value as read-only after the adapter returned it.
type Details struct {
	Electricity *ElectricityDetails
	Gas         *GasDetails
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
	// SupportsInquiry reports whether Inquiry answers the bill type (for example "water"). A
	// providers row can name the provider for a bill type that its adapter does not inquire (the
	// seed has parsian for water and gas, its adapter answers only "mobile"): the inquiry flow then
	// does not call Inquiry and answers as for a provider with no adapter (task 036).
	SupportsInquiry(billType string) bool
	Inquiry(ctx context.Context, req InquiryRequest) (*InquiryResponse, error)
	Pay(ctx context.Context, req PaymentRequest) (*PaymentResponse, error)
	Status(ctx context.Context, req StatusRequest) (*StatusResponse, error)
	GetDocument(ctx context.Context, req DocumentRequest) (*DocumentResponse, error)
	Settle(ctx context.Context, req SettleRequest) (*SettleResponse, error)
}
