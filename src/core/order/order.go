// Package order is the order flow of a bill payment (system.html, core/order; task 039). It copies
// the old bill service (service/billPayment/newOrder.go NewOrder): pick the order provider of the
// bill type, find the price, add the fee, and save one row in the orders table with the status
// initial. The token of the order is made by the handler (TokenPayload, common/crypto).
//
// The flow is the same for every bill type. What is special to a bill type is one entry of
// billTypes; an order task of another bill type adds its entry, not a copy of the flow.
package order

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	"github.com/google/uuid"

	"qabz/core/fee"
	"qabz/core/inquiry"
	"qabz/provider"
)

// ActionOrder is providers.action of the provider that is paid for an order (bill:
// entity.OrderAction).
const ActionOrder = "order"

// StatusInitial is orders.status of a new order (bill: entity.INITIAL).
const StatusInitial = "initial"

// Results of one Create call, for Options.OnOrder (a fixed set: metric labels).
const (
	ResultCreated       = "created"        // the orders row is saved
	ResultInvalid       = "invalid"        // the total does not fit the price field
	ResultNoProvider    = "no_provider"    // no enabled order provider for the bill type
	ResultZeroPrice     = "zero_price"     // the price is 0
	ResultInquiryFailed = "inquiry_failed" // the inquiry for the price failed, or has no inquiry of the type
	ResultError         = "error"          // a failed read or write, or a fault of the code
)

// NoProviderLabel is the provider label of a result with no picked provider.
const NoProviderLabel = "none"

// billType is what the flow knows of one bill type.
type billType struct {
	// directPrice is the first price rule of bill (NewOrder, only for the irancell bill type): a
	// request with no pay ID and a price that is not 0 is paid at that price, with the pay ID set
	// to the bill type text and no inquiry.
	directPrice bool
	// inquiryType is the inquiry type of the inquiry whose amount is the price (bill:
	// entity.BillTypeInquiryType; a bill type that is not in the map there gives
	// provider.InquiryTypeRegular).
	inquiryType string
}

// billTypes are the bill types that the flow supports, by the bill type that the handler gives.
//   - irancell (task 039): the direct price rule, and the final-term inquiry (bill:
//     BillTypeInquiryType[Irancell] is final_term; the request type midTerm or fullTerm does not
//     change it). bill's order cache has no TTL for irancell, so the inquiry never answers from
//     saved inquiries (inquiry.Request.NoCache).
//
// A later bill type that has an order cache TTL in bill (caching.paid* for gas, water, electricity,
// tel, mobile) needs that TTL in place of NoCache.
var billTypes = map[string]billType{
	provider.BillTypeIrancell: {directPrice: true, inquiryType: provider.InquiryTypeFinalTerm},
}

var (
	// ErrZeroPrice is the error of an order whose price is 0 (bill: AmountEqualToZero).
	ErrZeroPrice = errors.New("order: the price is 0")
	// ErrTotalTooLarge is the error of an order whose total (price + fee + provider fee) is below 0
	// or above the uint32 price field (bill: calculatePrice, "OVERFLOW").
	ErrTotalTooLarge = errors.New("order: the total does not fit the price field")
	// ErrNoInquiry is the error of an inquiry answer that has no inquiry of the type of the bill
	// type (bill: inquiryWithProvider "couldn't match inquiries").
	ErrNoInquiry = errors.New("order: no inquiry of the bill type in the answer")
	// ErrUnsupported is the error of a bill type that the flow does not know.
	ErrUnsupported = errors.New("order: unsupported bill type")
)

// InquiryError is the error of a failed inquiry for the price (the third price rule). Err is the
// error of inquiry.Service.Inquire, so the handler answers as the inquiry endpoint. Test it with
// errors.As.
type InquiryError struct{ Err error }

func (e *InquiryError) Error() string {
	if e == nil || e.Err == nil {
		return "order: inquiry failed"
	}
	return "order: inquiry failed: " + e.Err.Error()
}

func (e *InquiryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Request is one order.
type Request struct {
	BillType  string // for example provider.BillTypeIrancell
	BillID    string // the billId of the request
	PayID     string // the payId of the request
	InquiryID string // the inquiry ID of the bill type (the phone of a mobile bill); orders.data
	UserID    string // a UUID
	Dialer    string // the mobile of the user
	Option    string // orders.option (the operator of a mobile bill)
	Price     uint64 // the price of the request (bill: uint64(request.Price))
}

// Order is one row of the orders table. An empty string, a 0 and a nil are stored as NULL, as bill
// (go-pg stores a zero value as NULL).
type Order struct {
	ID            string // orders.uuid
	OrderID       string // orders.order_id
	UserID        string
	BillType      string
	BillID        string
	PayID         string
	Option        string
	Price         uint32 // the total: base price + fee + provider fee
	Data          string // the inquiry ID
	ProviderID    int64
	ProviderName  string
	Status        string
	Fee           int64
	FeeType       string
	FeeConfig     []byte // the applied fee config (JSON), nil: NULL
	FeeID         *int64
	BasePrice     int64
	IsRaw         bool
	ProviderFee   uint64
	BillInquiryID string
	// FeeSource is how the fee was found (fee.Source values). Not a column.
	FeeSource string
}

// Selector picks the provider of a bill type and an action (provider.ProvidersService).
type Selector interface {
	PickProvider(billType, action string) (provider.Provider, error)
}

// Inquirer answers the inquiry of a bill (inquiry.Service).
type Inquirer interface {
	Inquire(ctx context.Context, req inquiry.Request) (inquiry.Result, error)
}

// RawInquirer answers an inquiry from the bill ID and the pay ID (inquiry.Service.RawInquiry).
type RawInquirer interface {
	RawInquiry(endpointBillType, billID, payID string) (inquiry.RawResult, error)
}

// FeeResolver gives the fee of an order (fee.Resolver).
type FeeResolver interface {
	Resolve(ctx context.Context, userID string, providerID int64, amount int64) (fee.Result, error)
}

// Repository reads and writes the order tables.
type Repository interface {
	// Insert adds the orders row.
	Insert(ctx context.Context, o Order) error
	// InquiryProvider gives provider_name and bill_type of the bill_inquiries row with the id (bill:
	// bill_inquiry.Repository.GetById). No row is an error.
	InquiryProvider(ctx context.Context, billInquiryID string) (providerName, billType string, err error)
}

// Logger writes the errors that the flow handles itself (the App logger has it).
type Logger interface {
	Error(msg string, kv ...any)
}

// Options configure NewService.
type Options struct {
	// Providers, Inquiry, Raw, Fees and Repository are required.
	Providers  Selector
	Inquiry    Inquirer
	Raw        RawInquirer
	Fees       FeeResolver
	Repository Repository
	// Logger gets one error line for a failed fee (the order goes on, as bill). Nil: no line.
	Logger Logger
	// OnOrder is called once per Create call of a known bill type, with the bill type, the
	// provider name (NoProviderLabel when none was picked) and one of the Result values. Nil: no
	// call.
	OnOrder func(billType, providerName, result string)
}

// Service is the order flow. It is safe for concurrent use.
type Service struct {
	providers Selector
	inquiry   Inquirer
	raw       RawInquirer
	fees      FeeResolver
	repo      Repository
	log       Logger
	onOrder   func(billType, providerName, result string)

	newID func() string // replaced by tests
}

// NewService returns the flow.
func NewService(o Options) (*Service, error) {
	switch {
	case o.Providers == nil:
		return nil, errors.New("order: provider selector is required")
	case o.Inquiry == nil:
		return nil, errors.New("order: inquirer is required")
	case o.Raw == nil:
		return nil, errors.New("order: raw inquirer is required")
	case o.Fees == nil:
		return nil, errors.New("order: fee resolver is required")
	case o.Repository == nil:
		return nil, errors.New("order: repository is required")
	}
	return &Service{
		providers: o.Providers, inquiry: o.Inquiry, raw: o.Raw, fees: o.Fees, repo: o.Repository,
		log: o.Logger, onOrder: o.OnOrder, newID: uuid.NewString,
	}, nil
}

// price is the price of an order before the fee (bill: NewOrderInquiry).
type price struct {
	billInquiryID, billID, payID string
	amount                       uint64
}

// Create saves a new order (bill: payment_service.NewOrder, the same steps in the same order):
//  1. It picks the order provider of the bill type (ActionOrder): an error that wraps
//     provider.ErrNoProvider when none is enabled.
//  2. The price, by the first rule that applies:
//     a. directPrice of the bill type, no pay ID and a price that is not 0: the price of the
//     request, the pay ID is the bill type text, no bill ID;
//     b. a bill ID and a pay ID that are both numeric (strconv.Atoi, as bill): the raw inquiry; a
//     failed raw inquiry goes on to rule c, as bill;
//     c. the inquiry of the bill type with no saved inquiry (NoCache): the first inquiry of the
//     answer with the inquiry type of the bill type; its ID is bill_inquiry_id (none when the
//     answer was not saved: inquiry.Result.Unsaved). A failed inquiry is *InquiryError, an answer
//     with no such inquiry is ErrNoInquiry.
//  3. A price of 0 is ErrZeroPrice.
//  4. The provider fee: when the price came from an inquiry row, the row is read
//     (Repository.InquiryProvider; a failed read is an error). The provider fee of every provider
//     and bill type of the flow is 0 (bill: only Karashop has a provider fee, for tel).
//  5. The fee (FeeResolver). A failed fee does not stop the order: it is logged (one error line).
//  6. The total = price + fee + provider fee; below 0 or above math.MaxUint32 is ErrTotalTooLarge.
//  7. The orders row with StatusInitial and a new order ID. A failed write is an error.
func (s *Service) Create(ctx context.Context, req Request) (Order, error) {
	if s == nil || s.providers == nil || s.inquiry == nil || s.raw == nil || s.fees == nil || s.repo == nil {
		return Order{}, errors.New("order: service not created with NewService")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bt, ok := billTypes[req.BillType]
	if !ok {
		return Order{}, fmt.Errorf("%w %q", ErrUnsupported, req.BillType)
	}

	pro, err := s.providers.PickProvider(req.BillType, ActionOrder)
	if err != nil {
		result := ResultError
		if errors.Is(err, provider.ErrNoProvider) {
			result = ResultNoProvider
		}
		s.count(req.BillType, NoProviderLabel, result)
		return Order{}, fmt.Errorf("pick provider for %s %s: %w", req.BillType, ActionOrder, err)
	}

	p, err := s.price(ctx, req, bt)
	if err != nil {
		result := ResultError
		var inqErr *InquiryError
		if errors.As(err, &inqErr) || errors.Is(err, ErrNoInquiry) {
			result = ResultInquiryFailed
		}
		s.count(req.BillType, pro.Name, result)
		return Order{}, err
	}
	if p.amount == 0 {
		s.count(req.BillType, pro.Name, ResultZeroPrice)
		return Order{}, ErrZeroPrice
	}

	providerFee := uint64(0)
	if p.billInquiryID != "" {
		// bill reads the row for its provider fee (feeableBillType). No provider of the flow has
		// one: the fee is 0, but a failed read fails the order, as bill.
		if _, _, err := s.repo.InquiryProvider(ctx, p.billInquiryID); err != nil {
			s.count(req.BillType, pro.Name, ResultError)
			return Order{}, fmt.Errorf("order: read the inquiry %s for the provider fee: %w", p.billInquiryID, err)
		}
	}

	f, err := s.fees.Resolve(ctx, req.UserID, pro.ID, int64(p.amount))
	if err != nil && s.log != nil {
		s.log.Error(req.BillType+": order fee failed", "provider", pro.Name, "fee_source", f.Source, "fee", f.Fee, "error", err)
	}

	total := int64(p.amount) + f.Fee + int64(providerFee)
	if total < 0 || total > math.MaxUint32 {
		s.count(req.BillType, pro.Name, ResultInvalid)
		return Order{}, fmt.Errorf("%w: %d", ErrTotalTooLarge, total)
	}

	o := Order{
		ID: s.newID(), OrderID: s.newID(), UserID: req.UserID, BillType: req.BillType, BillID: p.billID,
		PayID: p.payID, Price: uint32(total), ProviderID: pro.ID, ProviderName: pro.Name, Status: StatusInitial,
		Option: req.Option, Data: req.InquiryID, Fee: f.Fee, FeeType: f.Type, FeeConfig: f.Config, FeeID: f.ID,
		BasePrice: int64(p.amount), ProviderFee: providerFee, BillInquiryID: p.billInquiryID, FeeSource: f.Source,
	}
	if err := s.repo.Insert(ctx, o); err != nil {
		s.count(req.BillType, pro.Name, ResultError)
		return Order{}, fmt.Errorf("order: insert: %w", err)
	}
	s.count(req.BillType, pro.Name, ResultCreated)
	return o, nil
}

// price is step 2 of Create.
func (s *Service) price(ctx context.Context, req Request, bt billType) (price, error) {
	if bt.directPrice && req.PayID == "" && req.Price != 0 {
		return price{payID: req.BillType, amount: req.Price}, nil
	}
	if numeric(req.BillID) && numeric(req.PayID) {
		// bill ignores the error of the raw inquiry and goes on to the provider inquiry.
		if raw, err := s.raw.RawInquiry(req.BillType, req.BillID, req.PayID); err == nil {
			return price{billID: raw.BillID, payID: raw.PayID, amount: raw.Amount}, nil
		}
	}
	res, err := s.inquiry.Inquire(ctx, inquiry.Request{
		BillType: req.BillType, InquiryID: req.InquiryID, UserID: req.UserID, Dialer: req.Dialer, NoCache: true,
	})
	if err != nil {
		return price{}, &InquiryError{Err: err}
	}
	for _, in := range res.Inquiries {
		if in.InquiryType == bt.inquiryType {
			id := in.ID
			if res.Unsaved {
				// debug.cache_inquiries false (development only): the inquiry has no row, so the
				// order has no bill_inquiry_id and reads no row. @todo bill has no such switch. See
				// docs/this_vs_bill.md, item 34.
				id = ""
			}
			return price{billInquiryID: id, billID: in.BillID, payID: in.PayID, amount: in.Amount}, nil
		}
	}
	return price{}, ErrNoInquiry
}

// numeric is bill's utils.IsNumericOnly: the text is an int for strconv.Atoi (a sign is accepted,
// an empty text and a number out of the int range are not).
func numeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func (s *Service) count(billType, providerName, result string) {
	if s.onOrder != nil {
		s.onOrder(billType, providerName, result)
	}
}
