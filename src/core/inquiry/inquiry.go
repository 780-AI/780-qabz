// Package inquiry is the bill inquiry flow (system.html, core/inquiry). It copies the old bill
// service (service/billInquiry/inquiry.go): pick the provider of the bill type, answer from the
// saved inquiries when they are still valid, else call the provider and save the answer. The rows
// are in the bill_inquiries and bill_inquiry_requests tables of the bill database. The flow is the
// same for every bill type: task 021 implements water, task 022 adds electricity and the details of
// an answer (provider.Details), task 023 adds gas (the inquiry ID is the participate code), task 035
// adds the Irancell mobile bill (the inquiry ID is the phone; only the Irancell provider answers),
// task 036 adds the MCI mobile bill (the bill type "mobile"; the inquiry ID is the phone).
//
// One request calls one provider: the first enabled provider of the sorted list (PickProvider).
// What that provider returns is the answer, also when it is an error or an answer that cannot be
// used. The flow never calls a second provider in the same request: moving a bill type to the next
// provider is the job of the Provider Switcher, only when the circuit breaker of the provider
// detects the condition, for every bill type in the same way (system.html, Provider Switcher;
// conventions § 13).
package inquiry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"

	"qabz/common/helpers"
	"qabz/provider"
)

// RequestInitial is bill_inquiry_requests.status of a row before the provider answers (bill:
// entity.INITIAL).
const RequestInitial = "initial"

// Results of one Inquire call, for Options.OnInquiry (a fixed set: metric labels).
const (
	ResultDBHit           = "db_hit"           // answered from saved inquiries, no provider call
	ResultProviderOK      = "provider_ok"      // the provider answered with inquiries
	ResultProviderRefused = "provider_refused" // the provider answered with no inquiry
	ResultProviderDown    = "provider_down"    // the call to the provider failed
	ResultNoProvider      = "no_provider"      // no enabled provider for the bill type
	ResultCanceled        = "canceled"         // the client left during the provider call
	ResultError           = "error"            // any other failure (database, adapter missing, ...)
)

// Results of one provider call, for Options.OnProviderCall (a fixed set: metric labels).
const (
	CallOK       = "ok"
	CallRefused  = "refused"
	CallDown     = "down"
	CallCanceled = "canceled"
	CallError    = "error"
)

// NoProviderLabel is the provider label of a result with no picked provider.
const NoProviderLabel = "none"

// finishTimeout limits the write of the final status of a request row. It runs on a context that
// the client cannot cancel, so a client that leaves during the provider call does not leave the
// row at RequestInitial. It is fixed (not a config key).
const finishTimeout = 5 * time.Second

// billType is what the flow knows of one bill type.
type billType struct {
	// tidPrefix is the first digit of the tid of a request row (bill: pkg/reference WaterReference,
	// ElectricityReference, GasReference, IrancellReference).
	tidPrefix int
	// validInquiryID is the rule of Request.InquiryID. Never nil.
	validInquiryID func(id string) bool
	// invalid names the inquiry ID in the text of the ErrInvalid error.
	invalid string
	// onlyProvider is the providers.name of the one provider that may answer the bill type, or ""
	// when every provider of the sorted list may. When the pick gives another provider, the flow
	// answers as "no enabled provider" and calls nobody.
	onlyProvider string
	// skipSameTypePair makes two saved rows of the same inquiry type (two mid-term or two
	// final-term rows) a miss: the pair is not one answer of a provider, so the provider is asked
	// again (bill: shouldSkipCache, for the bill types mobile and tel).
	skipSameTypePair bool
}

// billTypes are the bill types that the flow supports, by the bill type that the handler gives
// (Request.BillType). The flow does not read the bill type from the inquiry ID. To add a bill type,
// add an entry.
//   - water, electricity: the inquiry ID is the bill ID (helpers.ValidBillID).
//   - gas (task 023): the inquiry ID is the participate code. Its only rule is "not empty", as in
//     bill (controller/gas validateInquiryRequest). A code longer than the inquiry_id columns
//     (VARCHAR(15)) fails at the request row, as in bill (a database error, Internal).
//   - irancell (task 035): the inquiry ID is the phone of the request, a mobile number in the 98 form
//     (bill: controller/phone validateInquiryRequest needs a valid phone, so SetInquiryID always
//     gives the phone). Only the Irancell provider may answer (operator rule of task 035).
//   - mobile (task 036, the MCI operator): the inquiry ID is the phone, as irancell. Every provider
//     of the sorted list of (mobile, inquiry) may answer (the seed has Jiring Switch and Parsian);
//     the flow has no rule that is special to this bill type but the cache skip of bill.
var billTypes = map[string]billType{
	provider.BillTypeWater:       {tidPrefix: 1, validInquiryID: helpers.ValidBillID, invalid: "bill id"},
	provider.BillTypeElectricity: {tidPrefix: 2, validInquiryID: helpers.ValidBillID, invalid: "bill id"},
	provider.BillTypeGas:         {tidPrefix: 3, validInquiryID: notEmpty, invalid: "participate code"},
	provider.BillTypeIrancell:    {tidPrefix: 8, validInquiryID: helpers.ValidMobile, invalid: "phone", onlyProvider: irancellProvider},
	provider.BillTypeMobile:      {tidPrefix: 5, validInquiryID: helpers.ValidMobile, invalid: "phone", skipSameTypePair: true},
}

// irancellProvider is the providers.name of the Irancell provider (the adapter
// provider/irancellproxy has the same name).
const irancellProvider = "irancell"

// notEmpty is the rule of an inquiry ID that only has to be there.
func notEmpty(id string) bool { return id != "" }

var (
	// ErrInvalid is the error of a request that breaks a rule. Test it with errors.Is.
	ErrInvalid = errors.New("inquiry: invalid request")
	// ErrNoAdapter is the error when the provider that was picked has no adapter in the registry.
	ErrNoAdapter = errors.New("inquiry: no adapter for the provider")
)

// RefusedError is the error of a provider that answered with no inquiry (a bill that is not
// found, a failed status). Message is the text of the provider. Test it with errors.As.
type RefusedError struct {
	Message string
	Code    int32
}

func (e *RefusedError) Error() string { return "inquiry: the provider has no inquiry: " + e.Message }

// Request is one inquiry.
type Request struct {
	BillType  string // for example provider.BillTypeWater
	InquiryID string // the bill ID; for gas the participate code; for irancell and mobile the phone (billTypes)
	UserID    string // a UUID
	Dialer    string // the mobile of the user (bill_inquiry_requests.dialer)
}

// Inquiry is one inquiry of the answer.
type Inquiry struct {
	ID              string
	BillID          string
	PayID           string
	Amount          uint64
	PaymentDeadline string // yyyy-mm-dd, or "" when the date did not parse
	InquiryType     string
}

// Result is the answer of Inquire.
type Result struct {
	BillID    string
	Inquiries []Inquiry
	// Details are the details of the answer (electricity), or nil. Saved inquiries give the details
	// that were saved with them, so the two sources answer the same. Read-only.
	Details *provider.Details
	// FromDatabase is true when the answer came from saved inquiries (no provider call).
	FromDatabase bool
}

// Row is one row of bill_inquiries. An empty string is a NULL column.
type Row struct {
	ID              string
	UserID          string
	InquiryID       string
	ProviderID      int64
	ProviderName    string
	BillType        string
	ProviderTraceID string // the tid column
	InquiryType     string
	BillID          string
	PayID           string
	PaymentDeadline string
	Amount          uint64
	IsCached        bool
	// Details is the extra_data column: the details of the answer, or nil (NULL). Read-only.
	Details *provider.Details
}

// RequestRow is one row of bill_inquiry_requests.
type RequestRow struct {
	ID              string
	UserID          string
	Dialer          string
	InquiryID       string
	BillType        string
	TID             string
	ProviderName    string
	ProviderTraceID string
	Status          string
}

// Repository reads and writes the bill_inquiries and bill_inquiry_requests tables.
type Repository interface {
	// FindValid returns up to two rows of the bill (inquiry_id) that the provider (providerID)
	// answered less than ttl ago, newest first, that are not copies (is_cached false) and not paid
	// (is_paid false). The rows of every user count (the cache is shared). No row is not an error.
	FindValid(ctx context.Context, inquiryID string, ttl time.Duration, providerID int64) ([]Row, error)
	// SaveInquiries adds the rows in one write: all or none.
	SaveInquiries(ctx context.Context, rows []Row) error
	// CreateRequest adds a request row.
	CreateRequest(ctx context.Context, r RequestRow) error
	// FinishRequest sets the status and the provider trace ID of the request row with the id.
	FinishRequest(ctx context.Context, id, status, providerTraceID string) error
}

// Selector picks the provider of a bill type and an action (provider.ProvidersService).
type Selector interface {
	PickProvider(billType, action string) (provider.Provider, error)
}

// Adapters gives the adapter of a provider name (provider.ProviderRegistry).
type Adapters interface {
	Get(name string) (provider.ProviderInterface, bool)
}

// Logger writes the info lines and the errors of the flow (the App logger has both).
type Logger interface {
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// Options configure NewService.
type Options struct {
	// Repository, Providers and Adapters are required.
	Repository Repository
	Providers  Selector
	Adapters   Adapters
	// CacheTTL returns how long saved inquiries of a bill type answer the same bill again. 0 or
	// less: no cache for the bill type. Nil: no cache.
	CacheTTL func(billType string) time.Duration
	// Logger gets the info lines and the errors that the flow handles itself. Nil: no log line.
	Logger Logger
	// OnInquiry is called once per Inquire call that passed the request check, with the bill type,
	// the provider name (NoProviderLabel when none was picked) and one of the Result values. Nil:
	// no call.
	OnInquiry func(billType, providerName, result string)
	// OnProviderCall is called once per provider call with the provider name, the action, one of
	// the Call values and the duration of the call. Nil: no call.
	OnProviderCall func(providerName, action, result string, d time.Duration)
}

// Service is the inquiry flow. It is safe for concurrent use.
type Service struct {
	repo           Repository
	providers      Selector
	adapters       Adapters
	cacheTTL       func(billType string) time.Duration
	log            Logger
	onInquiry      func(billType, providerName, result string)
	onProviderCall func(providerName, action, result string, d time.Duration)

	// newID and newTID are replaced by tests.
	newID  func() string
	newTID func(billType string) (string, error)
}

// NewService returns the flow.
func NewService(o Options) (*Service, error) {
	switch {
	case o.Repository == nil:
		return nil, errors.New("inquiry: repository is required")
	case o.Providers == nil:
		return nil, errors.New("inquiry: provider selector is required")
	case o.Adapters == nil:
		return nil, errors.New("inquiry: adapters are required")
	}
	return &Service{
		repo: o.Repository, providers: o.Providers, adapters: o.Adapters, cacheTTL: o.CacheTTL,
		log: o.Logger, onInquiry: o.OnInquiry, onProviderCall: o.OnProviderCall,
		newID: uuid.NewString, newTID: generateTID,
	}, nil
}

// generateTID makes the tid of a request row (bill: GenerateBillInquiryTidIDByBillType): the type
// prefix, a random number from 100 to 90000000099 and the day of the year.
func generateTID(billType string) (string, error) {
	bt, ok := billTypes[billType]
	if !ok {
		return "", fmt.Errorf("inquiry: no tid prefix for bill type %q", billType)
	}
	return fmt.Sprintf("%d%d%d", bt.tidPrefix, rand.Int64N(90000000000)+100, time.Now().YearDay()), nil
}

// Inquire answers the inquiry of a bill:
//  1. A request that breaks a rule (the inquiry ID of the bill type, user) gives an error that wraps ErrInvalid.
//  2. It picks the provider of the bill type (an error that wraps provider.ErrNoProvider when none
//     is enabled, or when the bill type has one provider that may answer and the pick is another
//     one) and its adapter (ErrNoAdapter, also when the adapter does not support the bill type:
//     ProviderInterface.SupportsInquiry).
//  3. Saved inquiries of the same bill and provider that are valid answer the call with no
//     provider call, with the details that were saved with them; copies for the user are saved
//     (is_cached true). For the bill type mobile, two saved rows of the same inquiry type do not
//     answer (bill: shouldSkipCache). A failed read or a failed copy write is logged and does not fail the call.
//  4. Else it writes the request row (RequestInitial), calls the provider, writes the final status
//     and the provider trace ID, and saves each inquiry of the answer with the details. A call
//     that fails gives an error that wraps provider.ErrProviderDown; an answer with no inquiry
//     gives *RefusedError.
//
// Every other error is a failure of the database or of the code, with the stage in its text.
func (s *Service) Inquire(ctx context.Context, req Request) (Result, error) {
	if s == nil || s.repo == nil || s.providers == nil || s.adapters == nil {
		return Result{}, errors.New("inquiry: service not created with NewService")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bt, ok := billTypes[req.BillType]
	if !ok || bt.validInquiryID == nil {
		return Result{}, fmt.Errorf("inquiry: unsupported bill type %q", req.BillType)
	}
	if !bt.validInquiryID(req.InquiryID) {
		return Result{}, fmt.Errorf("%w: %s", ErrInvalid, bt.invalid)
	}
	if u, err := uuid.Parse(req.UserID); err != nil || u.String() != req.UserID {
		return Result{}, fmt.Errorf("%w: user", ErrInvalid)
	}

	pro, err := s.providers.PickProvider(req.BillType, provider.ActionInquiry)
	if err != nil {
		label, result := NoProviderLabel, ResultError
		if errors.Is(err, provider.ErrNoProvider) {
			result = ResultNoProvider
		}
		s.countInquiry(req.BillType, label, result)
		return Result{}, fmt.Errorf("pick provider for %s %s: %w", req.BillType, provider.ActionInquiry, err)
	}
	if bt.onlyProvider != "" && pro.Name != bt.onlyProvider {
		// The bill type has one provider that may answer. Another provider is never called: the
		// answer is the one of "no enabled provider".
		s.countInquiry(req.BillType, NoProviderLabel, ResultNoProvider)
		return Result{}, fmt.Errorf("pick provider for %s %s: %w: only %q may answer, the pick is %q",
			req.BillType, provider.ActionInquiry, provider.ErrNoProvider, bt.onlyProvider, pro.Name)
	}
	adapter, ok := s.adapters.Get(pro.Name)
	if !ok || adapter == nil {
		s.countInquiry(req.BillType, pro.Name, ResultError)
		return Result{}, fmt.Errorf("%w %q", ErrNoAdapter, pro.Name)
	}
	if !adapter.SupportsInquiry(req.BillType) {
		// The adapter does not inquire this bill type: the same answer as no adapter (no saved
		// inquiry read, no request row, no call), as before the adapter existed (task 036).
		s.countInquiry(req.BillType, pro.Name, ResultError)
		return Result{}, fmt.Errorf("%w %q for the bill type %q", ErrNoAdapter, pro.Name, req.BillType)
	}

	if res, ok := s.fromDatabase(ctx, req, pro, bt.skipSameTypePair); ok {
		s.countInquiry(req.BillType, pro.Name, ResultDBHit)
		return res, nil
	}
	res, result, err := s.fromProvider(ctx, req, pro, adapter)
	s.countInquiry(req.BillType, pro.Name, result)
	return res, err
}

func (s *Service) countInquiry(billType, providerName, result string) {
	if s.onInquiry != nil {
		s.onInquiry(billType, providerName, result)
	}
}

// fromDatabase answers from valid saved inquiries (bill: getFromCache). ok is false when there is
// no valid row, the cache is off, the read failed (logged), or skipSameTypePair is set and the two
// rows have the same mobile inquiry type (bill: shouldSkipCache).
func (s *Service) fromDatabase(ctx context.Context, req Request, pro provider.Provider, skipSameTypePair bool) (Result, bool) {
	var ttl time.Duration
	if s.cacheTTL != nil {
		ttl = s.cacheTTL(req.BillType)
	}
	if ttl <= 0 {
		return Result{}, false
	}
	rows, err := s.repo.FindValid(ctx, req.InquiryID, ttl, pro.ID)
	if err != nil {
		// bill also goes on to the provider after a failed read. A client that left is not a fault.
		if ctx.Err() == nil && s.log != nil {
			s.log.Error("inquiry: read saved inquiries failed", "bill_type", req.BillType, "provider", pro.Name, "error", err)
		}
		return Result{}, false
	}
	if len(rows) == 0 {
		return Result{}, false
	}
	if skipSameTypePair && len(rows) == 2 && rows[0].InquiryType == rows[1].InquiryType &&
		(rows[0].InquiryType == provider.InquiryTypeMidterm || rows[0].InquiryType == provider.InquiryTypeFinalTerm) {
		return Result{}, false
	}
	res := Result{FromDatabase: true, Inquiries: make([]Inquiry, 0, len(rows))}
	copies := make([]Row, 0, len(rows))
	for _, r := range rows {
		res.BillID = r.BillID
		if r.Details != nil {
			res.Details = r.Details // as the inquiry that the handler sends: the last one wins
		}
		c := r
		c.ID, c.UserID, c.IsCached = s.newID(), req.UserID, true
		copies = append(copies, c)
		res.Inquiries = append(res.Inquiries, Inquiry{
			ID: c.ID, BillID: r.BillID, PayID: r.PayID, Amount: r.Amount,
			PaymentDeadline: fixPaymentDeadline(r.PaymentDeadline), InquiryType: r.InquiryType,
		})
	}
	// The copies are the user's own rows (bill does the same). The answer does not depend on them.
	if err := s.repo.SaveInquiries(ctx, copies); err != nil && ctx.Err() == nil && s.log != nil {
		s.log.Error("inquiry: save copies of saved inquiries failed", "bill_type", req.BillType, "provider", pro.Name, "error", err)
	}
	return res, true
}

// fromProvider calls the provider and saves the answer (bill: the step loop with one step). The
// second value is the Result of the call, for the metric.
func (s *Service) fromProvider(ctx context.Context, req Request, pro provider.Provider, adapter provider.ProviderInterface) (Result, string, error) {
	tid, err := s.newTID(req.BillType)
	if err != nil {
		return Result{}, ResultError, fmt.Errorf("make tid: %w", err)
	}
	row := RequestRow{
		ID: s.newID(), UserID: req.UserID, Dialer: req.Dialer, InquiryID: req.InquiryID,
		BillType: req.BillType, TID: tid, ProviderName: pro.Name, Status: RequestInitial,
	}
	if err := s.repo.CreateRequest(ctx, row); err != nil {
		return Result{}, ResultError, fmt.Errorf("save request row: %w", err)
	}

	start := time.Now()
	resp, callErr := adapter.Inquiry(ctx, provider.InquiryRequest{
		BillType: req.BillType, UserID: req.UserID, Dialer: req.Dialer, InquiryID: req.InquiryID,
	})
	d := time.Since(start)

	switch {
	case callErr != nil && ctx.Err() != nil:
		s.callDone(pro.Name, CallCanceled, d)
		s.finish(row.ID, provider.ResponseFailed, "")
		return Result{}, ResultCanceled, ctx.Err()
	case callErr != nil:
		result, outcome := CallError, ResultError
		if errors.Is(callErr, provider.ErrProviderDown) {
			result, outcome = CallDown, ResultProviderDown
		}
		s.callDone(pro.Name, result, d)
		s.finish(row.ID, provider.ResponseFailed, "")
		return Result{}, outcome, fmt.Errorf("provider %s inquiry: %w", pro.Name, callErr)
	case resp == nil:
		s.callDone(pro.Name, CallError, d)
		s.finish(row.ID, provider.ResponseFailed, "")
		return Result{}, ResultError, fmt.Errorf("provider %s inquiry: empty answer", pro.Name)
	}

	if s.log != nil {
		s.log.Info("inquiry: provider call", "bill_type", req.BillType, "provider", pro.Name, "inquiry_id", req.InquiryID,
			"status", resp.Status, "inquiries", len(resp.Inquiries), "duration_ms", float64(d)/float64(time.Millisecond))
	}
	if err := s.finishErr(row.ID, resp.Status, resp.ProviderTraceID); err != nil {
		s.callDone(pro.Name, CallError, d)
		return Result{}, ResultError, fmt.Errorf("save request status: %w", err)
	}
	if resp.Status != provider.ResponseSuccess || len(resp.Inquiries) == 0 {
		s.callDone(pro.Name, CallRefused, d)
		return Result{}, ResultProviderRefused, &RefusedError{Message: resp.Message, Code: resp.Code}
	}
	s.callDone(pro.Name, CallOK, d)

	res := Result{BillID: resp.BillID, Details: resp.Details, Inquiries: make([]Inquiry, 0, len(resp.Inquiries))}
	rows := make([]Row, 0, len(resp.Inquiries))
	for _, it := range resp.Inquiries {
		r := Row{
			ID: s.newID(), UserID: req.UserID, InquiryID: req.InquiryID, ProviderID: pro.ID, ProviderName: pro.Name,
			BillType: req.BillType, ProviderTraceID: resp.ProviderTraceID, InquiryType: it.InquiryType, BillID: resp.BillID,
			PayID: it.PayID, PaymentDeadline: fixPaymentDeadline(it.PaymentDeadline), Amount: it.Amount,
			Details: resp.Details,
		}
		rows = append(rows, r)
		res.Inquiries = append(res.Inquiries, Inquiry{
			ID: r.ID, BillID: r.BillID, PayID: r.PayID, Amount: r.Amount, PaymentDeadline: r.PaymentDeadline, InquiryType: r.InquiryType,
		})
	}
	// bill ignores the result of this write: the answer does not depend on it (a failed write only
	// means that the next call asks the provider again). Qabz logs it.
	if err := s.repo.SaveInquiries(ctx, rows); err != nil && s.log != nil {
		s.log.Error("inquiry: save inquiries failed", "bill_type", req.BillType, "provider", pro.Name, "error", err)
	}
	return res, ResultProviderOK, nil
}

func (s *Service) callDone(providerName, result string, d time.Duration) {
	if s.onProviderCall != nil {
		s.onProviderCall(providerName, provider.ActionInquiry, result, d)
	}
}

// finishErr writes the final status of a request row on a context that the client cannot cancel.
func (s *Service) finishErr(id, status, traceID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), finishTimeout)
	defer cancel()
	return s.repo.FinishRequest(ctx, id, status, traceID)
}

// finish is finishErr for a failed call: the answer is already an error, so a failed write is only logged.
func (s *Service) finish(id, status, traceID string) {
	if err := s.finishErr(id, status, traceID); err != nil && s.log != nil {
		s.log.Error("inquiry: save request status failed", "request_id", id, "status", status, "error", err)
	}
}

// paymentDeadlineLayouts are the date formats that fixPaymentDeadline tries, in this order (bill:
// validateAndFixPaymentDeadline).
var paymentDeadlineLayouts = []string{
	"2006-01-02", "2006/01/02",
	"02-01-2006", "02/01/2006", "02.01.2006",
	"01-02-2006", "01/02/2006", "01.02.2006",
	"2006-01-02 15:04", "2006-01-02 15:04:05", "2006/01/02 15:04:05", "02-01-2006 15:04:05", "02/01/2006 15:04:05",
	"2006-01-02 03:04PM", "2006-01-02 03:04:05PM",
	time.RFC3339,
}

// fixPaymentDeadline turns a date in one of the known formats into yyyy-mm-dd. A date that no
// format reads (and an empty date) gives "".
func fixPaymentDeadline(date string) string {
	if date == "" {
		return ""
	}
	for _, layout := range paymentDeadlineLayouts {
		if t, err := time.Parse(layout, date); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}
