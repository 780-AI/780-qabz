package provider

import (
	"context"
	"errors"
	"time"
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
	// BillTypeTelMidterm is the bill type of the mid term of a landline bill (bill: entity.TELMIDTERM;
	// task 044). It is an order bill type: its inquiry is the inquiry of BillTypeTel.
	BillTypeTelMidterm = "telmidterm"

	ActionInquiry = "inquiry"

	ResponseSuccess = "success" // InquiryResponse.Status of a call that the provider answered
	ResponseFailed  = "failed"  // InquiryResponse.Status of a call that failed

	// The inquiry types of a mobile and a landline bill (bill: entity.MidtermInquiryType,
	// FinalTermInquiryType): one answer has a mid-term and a final-term (full-term) inquiry.
	InquiryTypeMidterm   = "midterm"
	InquiryTypeFinalTerm = "final_term"

	// The statuses of a payment and of a settlement (bill: entity.SUCCESS, FAILED, PENDING; task 044).
	StatusSuccess = "success"
	StatusFailed  = "failed"
	StatusPending = "pending"
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

// The service status of a landline (bill: gateway/inquiry ServiceStatus; task 044): Karashop sends
// it as a text (TelServiceStatus maps it). The order flow reads it for the provider fee.
const (
	TelServiceActive                = "active"
	TelServicePartiallyDisconnected = "partiallyDisconnected"
	TelServiceFullyDisconnected     = "fullyDisconnected"
)

// TelServiceStatus maps the Karashop serviceStatus text to a TelService value (bill:
// ServiceStatusMap). Any other text gives "", as in bill.
func TelServiceStatus(text string) string {
	switch text {
	case "وصل":
		return TelServiceActive
	case "قطع یکطرفه":
		return TelServicePartiallyDisconnected
	case "قطع دوطرفه":
		return TelServiceFullyDisconnected
	}
	return ""
}

// TelDetails are the details of a landline inquiry (bill: gateway/inquiry TelephoneDetails; task
// 044). The client does not see them; the flow saves them with the inquiry (extra_data).
type TelDetails struct {
	ServiceStatus string // a TelService value, or "" when Karashop sent an unknown text
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
	Tel         *TelDetails
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

// PaymentRequest is the input of ProviderInterface.Pay (task 044; bill: payment_gateway.PaymentRequest).
type PaymentRequest struct {
	BillType    string // the order bill type, for example BillTypeTel
	InquiryType string // the inquiry type of the bill type (InquiryTypeFinalTerm, InquiryTypeMidterm)
	OrderID     string // orders.order_id
	UserID      string
	Dialer      string
	InquiryID   string // orders.data: the inquiry ID (the landline number)
	BillID      string
	PayID       string
	Amount      uint64 // rials: the base price (no fee)
	RRN         string // the reference of the payment service
}

// PaymentResponse is the answer of ProviderInterface.Pay: the answer of the provider as it came.
// Status is the status of the provider; the order flow keeps it when it is StatusSuccess,
// StatusFailed or StatusPending, and treats any other value as StatusPending (bill: validStatuses).
type PaymentResponse struct {
	Status          string
	Code            int32
	Message         string
	ProviderTraceID string
}

// SettleRequest is the input of ProviderInterface.Settle: the report of a payment to the provider
// of the bill (task 044; bill: confirm_payment_gateway.ConfirmPaymentRequest).
type SettleRequest struct {
	BillType        string
	TelephoneNumber string // orders.data: the landline number
	Amount          uint64 // rials: the base price
	BillID          string
	PayID           string
	RRN             string
	PaidAt          time.Time // created_at of the order (bill: the order CreatedAt)
	PaymentProvider string    // the provider that was paid (orders.provider_name)
}

// SettleResponse is the answer of ProviderInterface.Settle. Status is StatusSuccess, StatusFailed or
// StatusPending (bill: the code tables of the provider); only StatusSuccess confirms the payment.
type SettleResponse struct {
	Status          string
	Code            int32
	Message         string
	ProviderTraceID string
}

// StatusRequest is the input of ProviderInterface.Status: the status of a paid order (task 045;
// bill: payment_inquiry_gateway.PaymentInquiryRequest).
type StatusRequest struct {
	OrderID  string // orders.order_id
	BillType string // the order bill type, for example BillTypeTel
}

// StatusResponse is the answer of ProviderInterface.Status: the answer of the provider as it came.
// Only StatusSuccess and StatusFailed are final (bill: validStatusMap); any other Status means that
// the status is not known yet.
type StatusResponse struct {
	Status          string
	Message         string
	ProviderTraceID string
}

// The request and response types of the methods that no adapter implements yet. Each later task
// that implements one of them gives its type the fields it needs.
type (
	DocumentRequest  struct{}
	DocumentResponse struct{}
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
	// Pay pays a bill. An error that wraps ErrProviderDown means that the provider gave no answer
	// (a timeout too): the payment status is not known (pending). Other errors: ErrNotSupported.
	Pay(ctx context.Context, req PaymentRequest) (*PaymentResponse, error)
	// Status asks the status of a paid order (task 045). An error that wraps ErrProviderDown means
	// that the provider gave no answer (a timeout too): the status is not known (pending). Other
	// errors: ErrNotSupported.
	Status(ctx context.Context, req StatusRequest) (*StatusResponse, error)
	GetDocument(ctx context.Context, req DocumentRequest) (*DocumentResponse, error)
	// Settle reports a payment to the provider of the bill. An error means that the provider gave no
	// answer that can be read (bill: a failed settlement).
	Settle(ctx context.Context, req SettleRequest) (*SettleResponse, error)
}
