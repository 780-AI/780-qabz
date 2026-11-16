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
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"qabz/common/monitoring/jaeger"
	"qabz/core/fee"
	"qabz/core/inquiry"
	"qabz/provider"
)

// createSpan is the span name of one Create call (conventions § 17; task 048).
const createSpan = "order.Service.Create"

// ActionOrder is providers.action of the provider that is paid for an order (bill:
// entity.OrderAction).
const ActionOrder = "order"

// The orders.status values (bill: entity.INITIAL, PROGRESS, SUCCESS, FAILED, PENDING). Each status
// change is a new orders row (bill: order.Repository.NewOrder); the newest row of an order ID is its
// status.
const (
	StatusInitial    = "initial"
	StatusInProgress = "in-progress"
	StatusSuccess    = provider.StatusSuccess
	StatusFailed     = provider.StatusFailed
	StatusPending    = provider.StatusPending
)

// Results of one Create call, for Options.OnOrder (a fixed set: metric labels).
const (
	ResultCreated       = "created"        // the orders row is saved
	ResultInvalid       = "invalid"        // the total does not fit the price field
	ResultNoProvider    = "no_provider"    // no enabled order provider for the bill type
	ResultZeroPrice     = "zero_price"     // the price is 0
	ResultInquiryFailed = "inquiry_failed" // the inquiry for the price failed, or has no inquiry of the type
	ResultAlreadyPaid   = "already_paid"   // the bill ID and pay ID have a pending or successful order (task 044)
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
	// inquiryFirst makes the provider inquiry the price rule before the raw inquiry (bill: NewOrder,
	// provider_fee_gateway AvailableBillTypes: the inquiry provider of the bill type has a provider
	// fee; task 044).
	inquiryFirst bool
	// inquiryBillType is the bill type of the inquiry for the price. Empty: the order bill type.
	inquiryBillType string
}

// billTypes are the bill types that the flow supports, by the bill type that the handler gives.
//
//   - irancell (task 039): the direct price rule, and the final-term inquiry (bill:
//     BillTypeInquiryType[Irancell] is final_term; the request type midTerm or fullTerm does not
//     change it). bill's order cache has no TTL for irancell, so the inquiry never answers from
//     saved inquiries (inquiry.Request.NoCache).
//
//   - tel and telmidterm (task 044): the inquiry first (Karashop has a provider fee), the
//     final-term or the mid-term inquiry of the inquiry of tel (one Karashop answer has both terms).
//     @todo bill asks the inquiry provider of the order bill type for one term. See
//     docs/this_vs_bill.md, item 42. The saved inquiries answer for Options.OrderCacheTTL (caching.paid_tel).
//
//   - water (task 048): the raw inquiry first (bill: no provider fee for water, so the second price
//     rule runs before the provider inquiry), else the regular inquiry of water (bill:
//     BillTypeInquiryType[WATER]). The saved inquiries answer for Options.OrderCacheTTL
//     (caching.paid_water).
//
// A bill type with no order cache TTL (Options.OrderCacheTTL gives 0) never answers from saved
// inquiries (inquiry.Request.NoCache).
var billTypes = map[string]billType{
	provider.BillTypeIrancell:   {directPrice: true, inquiryType: provider.InquiryTypeFinalTerm},
	provider.BillTypeTel:        {inquiryFirst: true, inquiryBillType: provider.BillTypeTel, inquiryType: provider.InquiryTypeFinalTerm},
	provider.BillTypeTelMidterm: {inquiryFirst: true, inquiryBillType: provider.BillTypeTel, inquiryType: provider.InquiryTypeMidterm},
	provider.BillTypeWater:      {inquiryType: provider.InquiryTypeRegular},
}

// InquiryTypeOf gives the inquiry type of an order bill type (bill: entity.BillTypeInquiryType), ""
// for a bill type that the flow does not know.
func InquiryTypeOf(billType string) string { return billTypes[billType].inquiryType }

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
	// ErrAlreadyPaid is the error of an order whose bill ID and pay ID have a pending or successful
	// order in the window of the order provider (bill: shouldSkipNewOrder, BillAlreadyPaidErrorType;
	// task 044).
	ErrAlreadyPaid = errors.New("order: the bill is paid already")
)

// KarashopProviderFee is the provider fee of a Karashop landline inquiry when the line is not
// disconnected (bill: feeableBillType, defaultFee). @todo bill marks it "todo: we have to remove
// that, tis so wrong"; Qabz keeps it as bill (task 044, operator decision D6 (a)).
const KarashopProviderFee = 21000

// karashopName is the providers.name of Karashop (provider/karashop.Name; core does not import an
// adapter).
const karashopName = "karashop"

// karashopFeeBillTypes are the bill types of a Karashop inquiry that has a provider fee (bill:
// app/providerFeeGateway.go: tel, telmidterm, telephone).
var karashopFeeBillTypes = map[string]bool{provider.BillTypeTel: true, provider.BillTypeTelMidterm: true, "telephone": true}

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

// InquiryFee is what the provider fee reads of a bill_inquiries row (bill: bill_inquiry GetById).
type InquiryFee struct {
	ProviderName  string
	ProviderID    int64
	BillType      string
	ServiceStatus string // extra_data.service_status of a landline row, "" for none
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
	// The columns of a status change (task 044; bill: CompleteOrder). Empty on a new order.
	RRN               string
	RefID             string // orders.ref_id: the trace ID of the provider
	Message           string
	StatusCode        int32
	ProviderConfirmed bool
	// CreatedAt is orders.created_at of a row that was read. Insert does not write it: the
	// database sets it (bill sets the zero value).
	CreatedAt time.Time
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
	// InquiryFee reads the bill_inquiries row with the id for the provider fee (bill:
	// bill_inquiry.Repository.GetById). No row is an error.
	InquiryFee(ctx context.Context, billInquiryID string) (InquiryFee, error)
	// ProviderFee gives providers.provider_fee of the provider row (NULL is 0). No row is an error.
	ProviderFee(ctx context.Context, providerID int64) (uint64, error)
	// LastStatus gives the status of the newest orders row with the bill ID and the pay ID, created
	// in the last window (0: any time); "" when there is none (bill: GetLastStatusByBillIDPayID).
	LastStatus(ctx context.Context, billID, payID string, window time.Duration) (string, error)
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
	// Logger gets one error line for a failed fee and for a failed "already paid" read (the order
	// goes on, as bill). Nil: no line.
	Logger Logger
	// OrderCacheTTL gives how long saved inquiries give the price of an order of the bill type
	// (bill: the caching TTL of the order action, caching.paid*). Nil or 0: no saved inquiry.
	OrderCacheTTL func(billType string) time.Duration
	// AlreadyPaidWindows are the windows of the "bill already paid" check, by order provider name
	// and bill type (bill: getNewOrderConflictProviders; task 044: parsian, tel and telmidterm,
	// providers.internal.parsian.settlement_inquiry_telephone). No entry: no check. Read-only.
	AlreadyPaidWindows map[string]map[string]time.Duration
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
	cacheTTL  func(billType string) time.Duration
	paidWin   map[string]map[string]time.Duration

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
		log: o.Logger, onOrder: o.OnOrder, cacheTTL: o.OrderCacheTTL, paidWin: o.AlreadyPaidWindows, newID: uuid.NewString,
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
//  2. The "bill already paid" check, when AlreadyPaidWindows has the provider and the bill type:
//     the newest order of the bill ID and the pay ID of the request in the window is pending or
//     success: ErrAlreadyPaid. A failed read is logged and the order goes on (bill).
//  3. The price, by the first rule that applies:
//     a. directPrice of the bill type, no pay ID and a price that is not 0: the price of the
//     request, the pay ID is the bill type text, no bill ID;
//     b. (not for inquiryFirst) a bill ID and a pay ID that are both numeric (strconv.Atoi, as
//     bill): the raw inquiry; a failed raw inquiry goes on to rule c, as bill;
//     c. the inquiry of the inquiry bill type, with saved inquiries for OrderCacheTTL (none: NoCache):
//     the first inquiry of the answer with the inquiry type of the bill type; its ID is
//     bill_inquiry_id (none when the answer was not saved: inquiry.Result.Unsaved). A failed
//     inquiry is *InquiryError, an answer with no such inquiry is ErrNoInquiry.
//  4. A price of 0 is ErrZeroPrice.
//  5. The provider fee: when the price came from an inquiry row, the row is read
//     (Repository.InquiryFee; a failed read is an error). A Karashop landline row has a provider
//     fee (providerFee); every other row has none.
//  6. The fee (FeeResolver). A failed fee does not stop the order: it is logged (one error line).
//  7. The total = price + fee + provider fee; below 0 or above math.MaxUint32 is ErrTotalTooLarge.
//  8. The orders row with StatusInitial and a new order ID. A failed write is an error.
//
// One span createSpan, a child of the span in ctx (conventions § 17; task 048), has the bill type,
// the order provider and the result (the Result values), and the status Error for every error.
func (s *Service) Create(ctx context.Context, req Request) (Order, error) {
	if s == nil || s.providers == nil || s.inquiry == nil || s.raw == nil || s.fees == nil || s.repo == nil {
		return Order{}, errors.New("order: service not created with NewService")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := otel.Tracer("qabz/core/order").Start(ctx, createSpan)
	providerName, result := NoProviderLabel, ResultError
	o, err := s.create(ctx, req, func(p, r string) {
		providerName, result = p, r
		s.count(req.BillType, p, r)
	})
	if _, known := billTypes[req.BillType]; known { // an unknown text is not put in the span
		span.SetAttributes(attribute.String("bill_type", req.BillType))
	}
	span.SetAttributes(attribute.String("provider", providerName), attribute.String("result", result))
	if err != nil {
		jaeger.Fail(span, result, err)
	}
	span.End()
	return o, err
}

// create is Create with the span in ctx; done gets the provider name and the result once per call
// of a known bill type.
func (s *Service) create(ctx context.Context, req Request, done func(providerName, result string)) (Order, error) {
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
		done(NoProviderLabel, result)
		return Order{}, fmt.Errorf("pick provider for %s %s: %w", req.BillType, ActionOrder, err)
	}

	if s.alreadyPaid(ctx, req, pro.Name) {
		done(pro.Name, ResultAlreadyPaid)
		return Order{}, ErrAlreadyPaid
	}

	p, err := s.price(ctx, req, bt)
	if err != nil {
		result := ResultError
		var inqErr *InquiryError
		if errors.As(err, &inqErr) || errors.Is(err, ErrNoInquiry) {
			result = ResultInquiryFailed
		}
		done(pro.Name, result)
		return Order{}, err
	}
	if p.amount == 0 {
		done(pro.Name, ResultZeroPrice)
		return Order{}, ErrZeroPrice
	}

	providerFee := uint64(0)
	if p.billInquiryID != "" {
		// bill reads the row for its provider fee (feeableBillType); a failed read fails the order.
		providerFee, err = s.providerFee(ctx, p.billInquiryID)
		if err != nil {
			done(pro.Name, ResultError)
			return Order{}, err
		}
	}

	f, err := s.fees.Resolve(ctx, req.UserID, pro.ID, int64(p.amount))
	if err != nil && s.log != nil {
		s.log.Error(req.BillType+": order fee failed", "provider", pro.Name, "fee_source", f.Source, "fee", f.Fee, "error", err)
	}

	total := int64(p.amount) + f.Fee + int64(providerFee)
	if providerFee > math.MaxInt64 || total < 0 || total > math.MaxUint32 {
		done(pro.Name, ResultInvalid)
		return Order{}, fmt.Errorf("%w: %d", ErrTotalTooLarge, total)
	}

	o := Order{
		ID: s.newID(), OrderID: s.newID(), UserID: req.UserID, BillType: req.BillType, BillID: p.billID,
		PayID: p.payID, Price: uint32(total), ProviderID: pro.ID, ProviderName: pro.Name, Status: StatusInitial,
		Option: req.Option, Data: req.InquiryID, Fee: f.Fee, FeeType: f.Type, FeeConfig: f.Config, FeeID: f.ID,
		BasePrice: int64(p.amount), ProviderFee: providerFee, BillInquiryID: p.billInquiryID, FeeSource: f.Source,
	}
	if err := s.repo.Insert(ctx, o); err != nil {
		done(pro.Name, ResultError)
		return Order{}, fmt.Errorf("order: insert: %w", err)
	}
	done(pro.Name, ResultCreated)
	return o, nil
}

// alreadyPaid is step 2 of Create (bill: shouldSkipNewOrder).
func (s *Service) alreadyPaid(ctx context.Context, req Request, providerName string) bool {
	window, ok := s.paidWin[providerName][req.BillType]
	if !ok {
		return false
	}
	status, err := s.repo.LastStatus(ctx, req.BillID, req.PayID, window)
	if err != nil {
		if ctx.Err() == nil && s.log != nil {
			s.log.Error(req.BillType+": order already-paid check failed", "provider", providerName, "error", err)
		}
		return false
	}
	return status == StatusPending || status == StatusSuccess
}

// providerFee is step 5 of Create (bill: feeableBillType): a Karashop row of a landline bill type
// has the provider fee providers.provider_fee of its provider when the line is partially or fully
// disconnected, else KarashopProviderFee. Every other row has none.
func (s *Service) providerFee(ctx context.Context, billInquiryID string) (uint64, error) {
	row, err := s.repo.InquiryFee(ctx, billInquiryID)
	if err != nil {
		return 0, fmt.Errorf("order: read the inquiry %s for the provider fee: %w", billInquiryID, err)
	}
	if row.ProviderName != karashopName || !karashopFeeBillTypes[row.BillType] {
		return 0, nil
	}
	if row.ServiceStatus != provider.TelServicePartiallyDisconnected && row.ServiceStatus != provider.TelServiceFullyDisconnected {
		return KarashopProviderFee, nil
	}
	fee, err := s.repo.ProviderFee(ctx, row.ProviderID)
	if err != nil {
		return 0, fmt.Errorf("order: read the provider fee of provider %d: %w", row.ProviderID, err)
	}
	return fee, nil
}

// price is step 3 of Create.
func (s *Service) price(ctx context.Context, req Request, bt billType) (price, error) {
	if bt.directPrice && req.PayID == "" && req.Price != 0 {
		return price{payID: req.BillType, amount: req.Price}, nil
	}
	if !bt.inquiryFirst && numeric(req.BillID) && numeric(req.PayID) {
		// bill ignores the error of the raw inquiry and goes on to the provider inquiry.
		if raw, err := s.raw.RawInquiry(req.BillType, req.BillID, req.PayID); err == nil {
			return price{billID: raw.BillID, payID: raw.PayID, amount: raw.Amount}, nil
		}
	}
	inquiryBillType := bt.inquiryBillType
	if inquiryBillType == "" {
		inquiryBillType = req.BillType
	}
	var ttl time.Duration
	if s.cacheTTL != nil {
		ttl = s.cacheTTL(req.BillType)
	}
	ireq := inquiry.Request{
		BillType: inquiryBillType, InquiryID: req.InquiryID, UserID: req.UserID, Dialer: req.Dialer,
		NoCache: ttl <= 0, CacheTTL: ttl,
	}
	res, err := s.inquiry.Inquire(ctx, ireq)
	if err != nil {
		return price{}, &InquiryError{Err: err}
	}
	if res.FromDatabase && !hasInquiryType(res, bt.inquiryType) {
		// The saved inquiries of the number have no unpaid inquiry of the term (the other term was
		// paid, so only its row is still valid): ask the provider. bill asks the inquiry provider
		// of the order bill type, whose saved rows have only that term, so it gets no saved answer
		// and calls the provider too (docs/this_vs_bill.md, item 42).
		ireq.NoCache, ireq.CacheTTL = true, 0
		if res, err = s.inquiry.Inquire(ctx, ireq); err != nil {
			return price{}, &InquiryError{Err: err}
		}
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

// hasInquiryType reports whether the answer has an inquiry of the type.
func hasInquiryType(res inquiry.Result, inquiryType string) bool {
	for _, in := range res.Inquiries {
		if in.InquiryType == inquiryType {
			return true
		}
	}
	return false
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
