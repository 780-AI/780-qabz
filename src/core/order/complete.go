package order

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"sync"
	"time"
	_ "time/tzdata" // the Asia/Tehran zone also where the system has no zone files

	"github.com/google/uuid"

	"qabz/provider"
)

// The payment step of an order (task 044; bill: controller/payment/orderComplete.go and
// service/billPayment/completeOrder.go CompleteOrder). The payment service calls it after the user
// paid: the order is checked, saved in-progress with the RRN, paid through the order provider, and
// the status of the answer is saved. On a success the settlement with the provider of the bill runs
// at once in the background (bill: go s.ConfirmPayment), with the side effects of bill's
// updateCache.

// The messages of a status change (bill's texts, saved in orders.message).
const (
	MessageProcessed      = "order has been processed before"
	MessageWrongPrice     = "payment price is not equal to order price"
	MessageNoProviderRow  = "error in getting provider from database"
	MessageInsertFailed   = "error in inserting new order"
	MessageNoPaymentRoute = "error in getting payment factory"
)

// LatestWindow is how old the newest row of an order may be (bill: order.Repository.Get, 48 h).
const LatestWindow = 48 * time.Hour

// The results of a Complete call, for Options.OnPayment (a fixed set: metric labels), with
// StatusSuccess, StatusFailed and StatusPending.
const (
	PaymentProcessed  = "processed"   // the order is not initial: answered pending, nothing saved
	PaymentWrongPrice = "wrong_price" // the price is not the order price: answered pending, nothing saved
	PaymentError      = "error"       // a failed read or write, or no payment route
)

// The results of a settlement, for Options.OnSettle: the settlement statuses of the provider and
// these.
const (
	SettleError       = "error"         // the call failed (provider.Settle returned an error)
	SettleMarkFailed  = "confirm_error" // Karashop confirmed, but provider_confirmed was not saved
	settleNoInquiryID = "no_data"       // the order has no inquiry ID (bill: no settlement)
)

var (
	// ErrPaymentDatabase is the error of a failed read or write of the payment step (bill:
	// DataBaseErrorType). Test it with errors.Is.
	ErrPaymentDatabase = errors.New("order payment: database error")
	// ErrPaymentRoute is the error of an order whose provider does not pay its bill type (bill:
	// BillPaymentFactory failed, InvalidParameterErrorType). Test it with errors.Is.
	ErrPaymentRoute = errors.New("order payment: the provider does not pay the bill type")
	// ErrNotFound is the error of Repository.Latest when the order has no row in the window.
	ErrNotFound = errors.New("order: not found")
)

// tehran is the zone of bills.last_order_date (bill: time.Now() of a process in Tehran).
var tehran = func() *time.Location {
	if loc, err := time.LoadLocation("Asia/Tehran"); err == nil {
		return loc
	}
	return time.FixedZone("IRST", 3*3600+1800)
}()

// PaymentRepository reads and writes the rows of the payment step.
type PaymentRepository interface {
	// Latest gives the newest orders row of the order ID created in the last window. None:
	// ErrNotFound (wrapped).
	Latest(ctx context.Context, orderID string, window time.Duration) (Order, error)
	// ProviderName gives providers.name of the provider row. No row is an error.
	ProviderName(ctx context.Context, providerID int64) (string, error)
	// Insert adds an orders row (a status change is a new row).
	Insert(ctx context.Context, o Order) error
	// ConfirmProvider sets provider_confirmed of the success row of the order created in the last
	// LatestWindow (bill: UpdateProviderConfirm).
	ConfirmProvider(ctx context.Context, orderID string) error
	// MarkInquiryPaid sets is_paid of the bill_inquiries rows of the inquiry ID and the pay ID that
	// are not copies (bill: UpdateIsPaid).
	MarkInquiryPaid(ctx context.Context, inquiryID, payID string) error
	// SetLastOrderDate sets last_order_date (and a NULL payment_deadline) of the saved bills of the
	// bill ID and the bill type (bill: mybill UpdateLastOrderDate).
	SetLastOrderDate(ctx context.Context, billID, billType, date string) error
}

// Adapters gives the adapter of a provider by name (provider.Registry).
type Adapters interface {
	Get(name string) (provider.ProviderInterface, bool)
}

// PaymentLogger writes the lines of the payment step (the App logger has it).
type PaymentLogger interface {
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// PaymentOptions configure NewPayments.
type PaymentOptions struct {
	// Repository and Adapters are required.
	Repository PaymentRepository
	Adapters   Adapters
	// PayRoutes are the bill types that each order provider pays (bill: the payment gateway
	// map; task 044: parsian, tel and telmidterm). Read-only.
	PayRoutes map[string]map[string]bool
	// SettleRoutes give the provider of the settlement by bill type and order provider (bill: the
	// confirm payment gateway map; task 044: tel and telmidterm, providers parsian, eways and eways2,
	// Karashop). No entry: no settlement. Read-only.
	SettleRoutes map[string]map[string]string
	// SettleTimeout limits the database writes of one background job. Positive; 0 is 30s.
	SettleTimeout time.Duration
	// Logger gets the lines of the step. Nil: no line.
	Logger PaymentLogger
	// OnPayment is called once per Complete call with the bill type, the order provider (NoProviderLabel
	// when the order was not read) and the result (a status or a Payment value). Nil: no call.
	OnPayment func(billType, providerName, result string)
	// OnPay is called after each provider payment call with the provider, the status of the
	// answer and the duration. Nil: no call.
	OnPay func(providerName, status string, d time.Duration)
	// OnSettle is called once per settlement with the bill type, the settlement provider and the
	// result (a settlement status or a Settle value). Nil: no call.
	OnSettle func(billType, providerName, result string)
}

// Payments is the payment step. It is safe for concurrent use. Stop waits for the background jobs.
type Payments struct {
	repo          PaymentRepository
	adapters      Adapters
	payRoutes     map[string]map[string]bool
	settleRoutes  map[string]map[string]string
	settleTimeout time.Duration
	log           PaymentLogger
	onPayment     func(billType, providerName, result string)
	onPay         func(providerName, status string, d time.Duration)
	onSettle      func(billType, providerName, result string)

	mu      sync.Mutex
	jobs    sync.WaitGroup
	stopped bool

	newID func() string    // replaced by tests
	now   func() time.Time // replaced by tests
	spawn func(job func()) // replaced by tests (run the job at once)
}

// NewPayments returns the payment step.
func NewPayments(o PaymentOptions) (*Payments, error) {
	if o.Repository == nil {
		return nil, errors.New("order payment: repository is required")
	}
	if o.Adapters == nil {
		return nil, errors.New("order payment: adapters are required")
	}
	if o.SettleTimeout < 0 {
		return nil, errors.New("order payment: settle timeout must not be negative")
	}
	if o.SettleTimeout == 0 {
		o.SettleTimeout = 30 * time.Second
	}
	p := &Payments{
		repo: o.Repository, adapters: o.Adapters, payRoutes: o.PayRoutes, settleRoutes: o.SettleRoutes,
		settleTimeout: o.SettleTimeout, log: o.Logger, onPayment: o.OnPayment, onPay: o.OnPay, onSettle: o.OnSettle,
		newID: uuid.NewString, now: time.Now,
	}
	p.spawn = func(job func()) { go job() }
	return p, nil
}

// CompleteRequest is one call of the payment service.
type CompleteRequest struct {
	OrderID string
	RRN     string
	Price   uint32
	Dialer  string // bill sends an empty dialer (its TODO); the payment service gives none
}

// Complete runs the payment step (bill: CompleteOrder, the same steps in the same order):
//  1. It reads the newest row of the order (LatestWindow). None, or a failed read:
//     ErrPaymentDatabase.
//  2. A row that is not initial is answered pending with MessageProcessed; a price that is not the
//     order price is answered pending with MessageWrongPrice. Nothing is saved, no provider call.
//  3. It reads the name of the order provider. A failed read saves failed (MessageNoProviderRow,
//  400. and returns ErrPaymentDatabase.
//  4. It saves in-progress with the RRN. A failed write saves failed (MessageInsertFailed, 400) and
//     returns ErrPaymentDatabase.
//  5. The provider must pay the bill type (PayRoutes; a raw order is the bill type "raw") and have
//     an adapter. Else it saves failed (MessageNoPaymentRoute, 400) and returns ErrPaymentRoute.
//  6. It pays (provider.Pay). An error is pending (500, the error text). A status of the answer
//     other than success, failed and pending is pending (bill: validStatuses). It saves the status,
//     ref_id, message and status_code. A failed write is answered pending (MessageInsertFailed,
//  500. with no saved row, as bill.
//  7. On success it starts the background job (settle): the side effects and the settlement.
//
// The returned Order is the last status (its Status and Message go back to the payment service).
// The reads and writes after step 2 do not stop when ctx is done: bill's take no context, so a
// payment service that leaves does not lose the status of a payment that was sent, and no order is
// saved as failed before its payment was sent. Only the provider call takes ctx.
func (p *Payments) Complete(ctx context.Context, req CompleteRequest) (Order, error) {
	if p == nil || p.repo == nil || p.adapters == nil {
		return Order{}, errors.New("order payment: not created with NewPayments")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	o, err := p.repo.Latest(ctx, req.OrderID, LatestWindow)
	if err != nil {
		p.countPayment("", NoProviderLabel, PaymentError)
		if !errors.Is(err, ErrNotFound) && ctx.Err() == nil && p.log != nil {
			p.log.Error("order complete: read the order failed", "order_id", req.OrderID, "error", err)
		}
		return Order{}, fmt.Errorf("%w: read the order %s: %w", ErrPaymentDatabase, req.OrderID, err)
	}
	if o.Status != StatusInitial {
		p.countPayment(o.BillType, o.ProviderName, PaymentProcessed)
		return Order{Status: StatusPending, Message: MessageProcessed}, nil
	}
	if o.Price != req.Price {
		p.countPayment(o.BillType, o.ProviderName, PaymentWrongPrice)
		return Order{Status: StatusPending, Message: MessageWrongPrice}, nil
	}

	wctx := context.WithoutCancel(ctx) // the reads and writes of the step after the checks (bill's take no context)
	name, err := p.repo.ProviderName(wctx, o.ProviderID)
	if err != nil {
		p.saveFailed(wctx, o, MessageNoProviderRow)
		p.countPayment(o.BillType, o.ProviderName, PaymentError)
		return Order{}, fmt.Errorf("%w: read the provider %d: %w", ErrPaymentDatabase, o.ProviderID, err)
	}

	o.RRN = req.RRN
	o.Status = StatusInProgress
	if err := p.insert(wctx, &o); err != nil {
		p.saveFailed(wctx, o, MessageInsertFailed)
		p.countPayment(o.BillType, name, PaymentError)
		return Order{}, fmt.Errorf("%w: save in-progress: %w", ErrPaymentDatabase, err)
	}

	billType := o.BillType
	if o.IsRaw {
		billType = "raw" // bill: entity.RAW
	}
	adapter, ok := p.adapters.Get(name)
	if !p.payRoutes[name][billType] || !ok || adapter == nil {
		p.saveFailed(wctx, o, MessageNoPaymentRoute)
		p.countPayment(o.BillType, name, PaymentError)
		return Order{}, fmt.Errorf("%w: %s %s", ErrPaymentRoute, name, billType)
	}

	start := time.Now()
	resp, payErr := adapter.Pay(ctx, provider.PaymentRequest{
		BillType: o.BillType, InquiryType: InquiryTypeOf(o.BillType), OrderID: o.OrderID, UserID: o.UserID,
		Dialer: req.Dialer, InquiryID: o.Data, BillID: o.BillID, PayID: o.PayID, Amount: uint64(max(o.BasePrice, 0)), RRN: o.RRN,
	})
	o.Status, o.RefID, o.Message, o.StatusCode = StatusPending, "", "", http.StatusInternalServerError
	switch {
	case payErr != nil:
		o.Message = payErr.Error()
	case resp == nil:
		o.Message = "empty payment answer"
	default:
		if resp.Status == StatusSuccess || resp.Status == StatusFailed || resp.Status == StatusPending {
			o.Status = resp.Status
		}
		o.RefID, o.Message, o.StatusCode = resp.ProviderTraceID, resp.Message, resp.Code
	}
	if p.onPay != nil {
		p.onPay(name, o.Status, time.Since(start))
	}
	if err := p.insert(wctx, &o); err != nil {
		if p.log != nil {
			p.log.Error("order complete: save the payment status failed", "order_id", o.OrderID, "status", o.Status, "error", err)
		}
		p.countPayment(o.BillType, name, PaymentError)
		return Order{OrderID: o.OrderID, UserID: o.UserID, BillID: o.BillID, BillType: o.BillType, Data: o.Data,
			ProviderName: name, Status: StatusPending, Message: MessageInsertFailed, StatusCode: http.StatusInternalServerError}, nil
	}
	p.countPayment(o.BillType, name, o.Status)
	if o.Status == StatusSuccess {
		p.background(o)
	}
	return o, nil
}

// insert saves o as a new row (a new uuid, the created_at of the database).
func (p *Payments) insert(ctx context.Context, o *Order) error {
	o.ID = p.newID()
	return p.repo.Insert(ctx, *o)
}

// saveFailed saves a failed row with the message and the code 400 (bill: a failed step). A failed
// write is logged: bill ignores it.
func (p *Payments) saveFailed(ctx context.Context, o Order, message string) {
	o.Status, o.Message, o.StatusCode = StatusFailed, message, http.StatusBadRequest
	if err := p.insert(ctx, &o); err != nil && p.log != nil {
		p.log.Error("order complete: save the failed status failed", "order_id", o.OrderID, "error", err)
	}
}

func (p *Payments) countPayment(billType, providerName, result string) {
	if p.onPayment != nil {
		p.onPayment(billType, providerName, result)
	}
}

// background starts the job of a successful payment, unless Stop was called (then the job is
// dropped with one error line: the settlement worker of task 046 settles the order later).
func (p *Payments) background(o Order) {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		if p.log != nil {
			p.log.Error("order complete: stopping, the settlement is not started", "order_id", o.OrderID)
		}
		return
	}
	p.jobs.Add(1)
	p.mu.Unlock()
	p.spawn(func() {
		defer p.jobs.Done()
		p.settle(o)
	})
}

// settle is the background job of a successful payment (bill: go s.updateCache and go
// s.ConfirmPayment). A panic is recovered and logged.
func (p *Payments) settle(o Order) {
	defer func() {
		if r := recover(); r != nil && p.log != nil {
			p.log.Error("order complete: settlement panic", "order_id", o.OrderID, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), p.settleTimeout)
	defer cancel()

	// bill: updateCache.
	if err := p.repo.MarkInquiryPaid(ctx, o.Data, o.PayID); err != nil && p.log != nil {
		p.log.Error("settle: mark paid failed", "order_id", o.OrderID, "error", err)
	}
	if err := p.repo.SetLastOrderDate(ctx, o.BillID, o.BillType, p.now().In(tehran).Format("2006-01-02")); err != nil && p.log != nil {
		p.log.Error("settle: set the last order date failed", "order_id", o.OrderID, "error", err)
	}

	// bill: ConfirmPayment.
	settler, ok := p.settleRoutes[o.BillType][o.ProviderName]
	if !ok {
		return
	}
	if o.Data == "" {
		p.countSettle(o.BillType, settler, settleNoInquiryID)
		return
	}
	adapter, ok := p.adapters.Get(settler)
	if !ok || adapter == nil {
		p.countSettle(o.BillType, settler, SettleError)
		if p.log != nil {
			p.log.Error("settle: "+settler+" failed", "order_id", o.OrderID, "error", "no adapter")
		}
		return
	}
	resp, err := adapter.Settle(ctx, provider.SettleRequest{
		BillType: o.BillType, TelephoneNumber: o.Data, Amount: uint64(max(o.BasePrice, 0)), BillID: o.BillID,
		PayID: o.PayID, RRN: o.RRN, PaidAt: o.CreatedAt, PaymentProvider: o.ProviderName,
	})
	switch {
	case err != nil:
		p.countSettle(o.BillType, settler, SettleError)
		if p.log != nil {
			p.log.Error("settle: "+settler+" failed", "order_id", o.OrderID, "error", err)
		}
		return
	case resp == nil || resp.Status != provider.StatusSuccess:
		status := SettleError
		var code int32
		var message string
		if resp != nil {
			status, code, message = resp.Status, resp.Code, resp.Message
		}
		p.countSettle(o.BillType, settler, status)
		if p.log != nil {
			p.log.Error("settle: "+settler+" failed", "order_id", o.OrderID, "status", status, "code", code, "message", message)
		}
		return
	}
	if err := p.repo.ConfirmProvider(ctx, o.OrderID); err != nil {
		p.countSettle(o.BillType, settler, SettleMarkFailed)
		if p.log != nil {
			p.log.Error("settle: save provider_confirmed failed", "order_id", o.OrderID, "error", err)
		}
		return
	}
	p.countSettle(o.BillType, settler, provider.StatusSuccess)
	if p.log != nil {
		p.log.Info("settle: "+settler+" confirmed", "order_id", o.OrderID)
	}
}

func (p *Payments) countSettle(billType, providerName, result string) {
	if p.onSettle != nil {
		p.onSettle(billType, providerName, result)
	}
}

// Stop stops new background jobs and waits for the running ones until ctx is done.
func (p *Payments) Stop(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	done := make(chan struct{})
	go func() {
		p.jobs.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("order payment: background jobs still running: %w", ctx.Err())
	}
}
