// Package reconcile is what the workers run (system.html, core/reconcile). Task 045: the status
// worker asks the provider of the paid orders that have no final status, reports a final status to
// the payment service and saves it (bill: service/billPaymentInquiry InquiryCron).
package reconcile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"qabz/common/monitoring/jaeger"
	"qabz/core/order"
	"qabz/provider"
)

// ReverseDeadlineExceeded is the message of a final status that came after the reverse deadline
// (bill: entity.ReverseDeadlineExceeded). Such a status is saved and not reported.
const ReverseDeadlineExceeded = "reverse_deadline_exceeded"

// latestWindow is how old the newest row of an order may be when the worker reads it (bill:
// GetOrderByIDWithIntervals with 48 h).
const latestWindow = order.LatestWindow

// Results of one order of a run, for StatusOptions.OnOrder (a fixed set: metric labels).
const (
	ResultSuccess     = "success"      // reported and saved as success
	ResultFailed      = "failed"       // reported and saved as failed
	ResultPending     = "pending"      // the provider gave no final status: nothing changes
	ResultLate        = "late"         // a final status after the reverse deadline: saved, not reported
	ResultSkipped     = "skipped"      // the newest row is final already (at the read or at the save)
	ResultReportError = "report_error" // the report failed: the save rolled back, the next run asks again
	ResultSaveError   = "save_error"   // the save failed (nothing reported; or the commit after a good report; or a late status)
	ResultReadError   = "read_error"   // the newest row was not read
)

// fixedResults are the order results that gave the order a final status in the run: they are counted
// in StatusOptions.OnRunDone (task 050 D8).
var fixedResults = map[string]bool{ResultSuccess: true, ResultFailed: true, ResultLate: true}

// Provider call results, for StatusOptions.OnCall (the labels of qabz_provider_calls_total).
const (
	callOK       = "ok"
	callDown     = "down"
	callCanceled = "canceled"
	callError    = "error"
)

// actionStatus is the action label of a status call.
const actionStatus = "status"

// checkSpan is the span name of the check of one order (conventions § 17; task 049).
const checkSpan = "reconcile.StatusWorker.check"

// failedResults are the order results that are a failure: their span gets the status Error.
// pending, skipped, late, success and failed are normal outcomes of a check.
var failedResults = map[string]bool{ResultReadError: true, ResultSaveError: true, ResultReportError: true}

// Repository reads and writes the orders of the worker (postgres.OrderRepository).
type Repository interface {
	// PendingIDs gives one page of at most limit orders of the provider whose newest row created
	// from now-end to now-start is pending or in-progress, after the cursor, ordered by the cursor
	// (bill: GetAllPending; task 050 D4: a keyset page).
	PendingIDs(ctx context.Context, providerName string, limit int, after order.Cursor, start, end time.Duration) ([]order.Cursor, error)
	// Latest gives the newest row of the order created in the last window.
	Latest(ctx context.Context, orderID string, window time.Duration) (order.Order, error)
	// Insert adds a row (the late status; bill: NewOrder).
	Insert(ctx context.Context, o order.Order) error
	// InsertFinal adds a final row unless the newest row is final (order.ErrAlreadyFinal; bill:
	// NewOrderWithFinalStatus), and calls report after the insert and before the commit: an error of
	// report rolls back the insert and is returned (task 050 D6).
	InsertFinal(ctx context.Context, o order.Order, report func(context.Context) error) error
}

// Adapters gives the adapter of a provider by name (provider.Registry).
type Adapters interface {
	Get(name string) (provider.ProviderInterface, bool)
}

// Reporter reports a final status of an order to the payment service (Kafka, PaymentReporter).
type Reporter interface {
	Report(ctx context.Context, orderID, status string) error
}

// Charger keeps the lock of a run (entry/cron.Extender).
type Charger interface {
	Charge(ctx context.Context, remaining int) error
}

// Logger gets the lines of the worker.
type Logger interface {
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// StatusOptions configure NewStatusWorker.
type StatusOptions struct {
	// Name is the name of the worker (get_status.name): the metric label cron.
	Name string
	// Provider is the providers.name of the orders and of the adapter (bill: "parsian").
	Provider string
	// Interval is the time between two runs (get_status.cron_time).
	Interval time.Duration
	// Start and End give the window of the orders: created from now-End to now-Start.
	Start, End time.Duration
	// Chunk is the page size. Positive.
	Chunk int
	// ReverseDeadline: a final status of an order whose newest row is older is saved with the
	// message ReverseDeadlineExceeded and not reported (0: every final status, as bill).
	ReverseDeadline time.Duration
	// SyncOperations give the lock TTL (bill: syncOperations).
	SyncOperations []time.Duration

	Repository Repository
	Adapters   Adapters
	Reporter   Reporter
	Logger     Logger // nil: no line
	// OnOrder gets the result of each order (Result* constants). Nil: nothing.
	OnOrder func(name, result string)
	// OnCall gets each status call: provider, action "status", result (ok, down, canceled, error)
	// and duration. Nil: nothing.
	OnCall func(providerName, action, result string, d time.Duration)
	// OnRunDone gets, once at the end of each run (also an empty run and a run that ends with an
	// error), the number of orders that the run gave a final status: the results success, failed
	// and late (task 050 D8). Nil: nothing.
	OnRunDone func(name string, fixed int)
}

// StatusWorker is the status worker of one provider (task 045: parsian; task 050: irancell).
type StatusWorker struct {
	o     StatusOptions
	now   func() time.Time
	newID func() string
}

// NewStatusWorker checks the options and returns the worker.
func NewStatusWorker(o StatusOptions) (*StatusWorker, error) {
	switch {
	case o.Name == "" || o.Provider == "":
		return nil, errors.New("reconcile: the name and the provider are required")
	case o.Interval <= 0 || o.Chunk <= 0:
		return nil, errors.New("reconcile: the interval and the chunk must be positive")
	case o.Start < 0 || o.End <= o.Start || o.ReverseDeadline < 0:
		return nil, errors.New("reconcile: the window must have 0 <= start < end and the reverse deadline must not be negative")
	case o.Repository == nil || o.Adapters == nil || o.Reporter == nil:
		return nil, errors.New("reconcile: the repository, the adapters and the reporter are required")
	}
	return &StatusWorker{o: o, now: time.Now, newID: uuid.NewString}, nil
}

// Name, Interval and SyncOperations give what the runner needs.
func (w *StatusWorker) Name() string                    { return w.o.Name }
func (w *StatusWorker) Interval() time.Duration         { return w.o.Interval }
func (w *StatusWorker) Chunk() int                      { return w.o.Chunk }
func (w *StatusWorker) SyncOperations() []time.Duration { return w.o.SyncOperations }

// @todo docs/this_vs_bill.md item 43: each call has the timeout of the adapter and the context of
// the run (bill: no timeout, context.Background()); every failure writes one line and is counted
// (task 050 D7); keyset pages in place of bill's OFFSET (task 050 D4).
//
// Run checks the orders of the window, one page after the other (bill: InquiryCron):
//  1. Read a page of orders after the cursor (PendingIDs). A failed read ends the run with the
//     error (the runner writes the line cron: run failed).
//  2. No order: the run ends.
//  3. Charge the lock for the page (a lost lock ends the run), then check each order.
//  4. The cursor becomes the last order of the page. A page shorter than the chunk is the last one.
//
// An order that the run made final stays in the list (task 045 C6) only while it is older than the
// cursor, so it is not read again in the run. A done ctx (the App stop) ends the run. At each end,
// OnRunDone gets the number of orders that the run made final.
func (w *StatusWorker) Run(ctx context.Context, ext Charger) error {
	fixed := 0
	defer func() {
		if w.o.OnRunDone != nil {
			w.o.OnRunDone(w.o.Name, fixed)
		}
	}()
	adapter, ok := w.o.Adapters.Get(w.o.Provider)
	if !ok || adapter == nil {
		return fmt.Errorf("reconcile: no adapter for the provider %s", w.o.Provider)
	}
	var after order.Cursor
	for {
		page, err := w.o.Repository.PendingIDs(ctx, w.o.Provider, w.o.Chunk, after, w.o.Start, w.o.End)
		if err != nil {
			return fmt.Errorf("reconcile: read the pending orders: %w", err)
		}
		if len(page) == 0 {
			return nil
		}
		if ext != nil {
			if err := ext.Charge(ctx, len(page)); err != nil {
				return fmt.Errorf("reconcile: keep the lock: %w", err)
			}
		}
		for _, p := range page {
			if err := ctx.Err(); err != nil {
				return err
			}
			result := w.checkTraced(ctx, adapter, p.OrderID)
			w.count(result)
			if fixedResults[result] {
				fixed++
			}
		}
		if len(page) < w.o.Chunk {
			return nil
		}
		after = page[len(page)-1]
	}
}

// checkTraced is check in one span checkSpan, a child of the span in ctx (the root span of the
// tick, entry/cron; conventions § 17; task 049), with the provider, the order ID and the result of
// the order. A read, report or save failure gives the status Error.
func (w *StatusWorker) checkTraced(ctx context.Context, adapter provider.ProviderInterface, orderID string) string {
	ctx, span := otel.Tracer("qabz/core/reconcile").Start(ctx, checkSpan)
	defer span.End()
	result := w.check(ctx, adapter, orderID)
	span.SetAttributes(attribute.String("provider", w.o.Provider), attribute.String("order.id", orderID),
		attribute.String("result", result))
	if failedResults[result] {
		jaeger.Fail(span, result, nil)
	}
	return result
}

// check asks the status of one order and acts on it. It returns the result of the order.
func (w *StatusWorker) check(ctx context.Context, adapter provider.ProviderInterface, orderID string) string {
	prefix := w.o.Provider + " status: "
	o, err := w.o.Repository.Latest(ctx, orderID, latestWindow)
	if err != nil {
		// Retried at the next run, but a line all the same (task 050 D7, conventions § 10).
		w.logError(prefix+"read failed", "name", w.o.Name, "order_id", orderID, "error", err)
		return ResultReadError
	}
	if o.Status == order.StatusSuccess || o.Status == order.StatusFailed {
		return ResultSkipped
	}
	status := w.status(ctx, adapter, o)
	if status != order.StatusSuccess && status != order.StatusFailed {
		return ResultPending
	}
	o.Status = status
	o.ID = w.newID()

	if o.CreatedAt.Before(w.now().Add(-w.o.ReverseDeadline)) {
		o.Message = ReverseDeadlineExceeded
		if err := w.o.Repository.Insert(ctx, o); err != nil {
			w.logError(prefix+"save failed", "name", w.o.Name, "order_id", orderID, "status", status, "error", err)
			return ResultSaveError
		}
		w.logInfo(prefix+"reverse deadline exceeded", "name", w.o.Name, "order_id", orderID, "status", status, "created_at", o.CreatedAt.String())
		return ResultLate
	}
	// Save, report, commit (task 050 D6): the report runs inside the save transaction.
	var reportErr error
	reported := false
	err = w.o.Repository.InsertFinal(ctx, o, func(ctx context.Context) error {
		reportErr = w.o.Reporter.Report(ctx, orderID, status)
		reported = reportErr == nil
		return reportErr
	})
	stopped := ctx.Err() != nil // the App stop canceled the run: counted, no line (conventions § 10)
	switch {
	case reportErr != nil && stopped:
		return ResultReportError
	case reportErr != nil:
		w.logError(prefix+"report failed", "name", w.o.Name, "order_id", orderID, "status", status, "error", reportErr)
		return ResultReportError
	case errors.Is(err, order.ErrAlreadyFinal):
		// Another path saved a final row after the read: not an error, nothing reported.
		w.logInfo(prefix+"already final", "name", w.o.Name, "order_id", orderID, "status", status)
		return ResultSkipped
	case err != nil && stopped:
		return ResultSaveError
	case err != nil:
		// reported true: the commit failed after a delivered report; the next run reports again.
		w.logError(prefix+"save failed", "name", w.o.Name, "order_id", orderID, "status", status, "reported", reported, "error", err)
		return ResultSaveError
	}
	w.logInfo(prefix+"reported", "name", w.o.Name, "order_id", orderID, "status", status)
	if status == order.StatusSuccess {
		return ResultSuccess
	}
	return ResultFailed
}

// status asks the adapter. An error or no answer is pending (bill: a gRPC error is PENDING). A failed
// call (not one that the App stop canceled), no answer and a status text that is not an order status
// (success, failed, pending, in-progress) each write one error line (task 050 D7).
func (w *StatusWorker) status(ctx context.Context, adapter provider.ProviderInterface, o order.Order) string {
	start := w.now()
	resp, err := adapter.Status(ctx, provider.StatusRequest{OrderID: o.OrderID, BillType: o.BillType})
	result := callOK
	switch {
	case err != nil && ctx.Err() != nil:
		result = callCanceled
	case errors.Is(err, provider.ErrProviderDown):
		result = callDown
	case err != nil || resp == nil:
		result = callError
	}
	if w.o.OnCall != nil {
		w.o.OnCall(w.o.Provider, actionStatus, result, w.now().Sub(start))
	}
	prefix := w.o.Provider + " status: "
	switch {
	case result == callCanceled:
		return order.StatusPending // the App stop: the run ends, no line
	case err != nil:
		w.logError(prefix+"call failed", "name", w.o.Name, "order_id", o.OrderID, "error", err)
		return order.StatusPending
	case resp == nil:
		w.logError(prefix+"call failed", "name", w.o.Name, "order_id", o.OrderID, "error", errEmptyAnswer)
		return order.StatusPending
	}
	switch resp.Status {
	case order.StatusSuccess, order.StatusFailed, order.StatusPending, order.StatusInProgress:
	default:
		w.logError(prefix+"unknown status", "name", w.o.Name, "order_id", o.OrderID, "status", resp.Status,
			"error", errUnknownStatus)
	}
	return resp.Status
}

var (
	// errEmptyAnswer is the cause of a status call that gave no answer and no error.
	errEmptyAnswer = errors.New("reconcile: the provider gave no answer")
	// errUnknownStatus is the cause of a status text that is not an order status.
	errUnknownStatus = errors.New("reconcile: unknown status text")
)

func (w *StatusWorker) count(result string) {
	if w.o.OnOrder != nil {
		w.o.OnOrder(w.o.Name, result)
	}
}

func (w *StatusWorker) logInfo(msg string, kv ...any) {
	if w.o.Logger != nil {
		w.o.Logger.Info(msg, kv...)
	}
}

func (w *StatusWorker) logError(msg string, kv ...any) {
	if w.o.Logger != nil {
		w.o.Logger.Error(msg, kv...)
	}
}
