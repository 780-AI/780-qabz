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
	ResultSkipped     = "skipped"      // the newest row is final already
	ResultReportError = "report_error" // the report failed: nothing saved, the next run asks again
	ResultSaveError   = "save_error"   // the save failed (after the report, or of a late status)
	ResultReadError   = "read_error"   // the newest row was not read
)

// Provider call results, for StatusOptions.OnCall (the labels of qabz_provider_calls_total).
const (
	callOK       = "ok"
	callDown     = "down"
	callCanceled = "canceled"
	callError    = "error"
)

// actionStatus is the action label of a status call.
const actionStatus = "status"

// Repository reads and writes the orders of the worker (postgres.OrderRepository).
type Repository interface {
	// PendingIDs gives one page of the order IDs of the provider whose newest row created from
	// now-end to now-start is pending or in-progress (bill: GetAllPending).
	PendingIDs(ctx context.Context, providerName string, limit, offset int, start, end time.Duration) ([]string, error)
	// Latest gives the newest row of the order created in the last window.
	Latest(ctx context.Context, orderID string, window time.Duration) (order.Order, error)
	// Insert adds a row (the late status; bill: NewOrder).
	Insert(ctx context.Context, o order.Order) error
	// InsertFinal adds a final row unless the newest row is final (bill: NewOrderWithFinalStatus).
	InsertFinal(ctx context.Context, o order.Order) error
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
}

// StatusWorker is the status worker of one provider (task 045: parsian).
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
// the run (bill: no timeout, context.Background()); the failures write one line and are counted.
//
// Run checks the orders of the window, one page after the other (bill: InquiryCron):
//  1. Read a page of order IDs (PendingIDs). A failed read ends the run with the error.
//  2. No ID: the run ends.
//  3. Charge the lock for the page (a lost lock ends the run), then check each order.
//
// The offset grows by the chunk each time; an order that the run made final stays in the list
// (C6) and is skipped at its check. A done ctx (the App stop) ends the run.
func (w *StatusWorker) Run(ctx context.Context, ext Charger) error {
	adapter, ok := w.o.Adapters.Get(w.o.Provider)
	if !ok || adapter == nil {
		return fmt.Errorf("reconcile: no adapter for the provider %s", w.o.Provider)
	}
	for offset := 0; ; offset += w.o.Chunk {
		ids, err := w.o.Repository.PendingIDs(ctx, w.o.Provider, w.o.Chunk, offset, w.o.Start, w.o.End)
		if err != nil {
			return fmt.Errorf("reconcile: read the pending orders: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		if ext != nil {
			if err := ext.Charge(ctx, len(ids)); err != nil {
				return fmt.Errorf("reconcile: keep the lock: %w", err)
			}
		}
		for _, id := range ids {
			if err := ctx.Err(); err != nil {
				return err
			}
			w.count(w.check(ctx, adapter, id))
		}
	}
}

// check asks the status of one order and acts on it. It returns the result of the order.
func (w *StatusWorker) check(ctx context.Context, adapter provider.ProviderInterface, orderID string) string {
	o, err := w.o.Repository.Latest(ctx, orderID, latestWindow)
	if err != nil {
		return ResultReadError // retried at the next run; counted, no line (conventions § 10)
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

	prefix := w.o.Provider + " status: "
	if o.CreatedAt.Before(w.now().Add(-w.o.ReverseDeadline)) {
		o.Message = ReverseDeadlineExceeded
		if err := w.o.Repository.Insert(ctx, o); err != nil {
			w.logError(prefix+"save failed", "order_id", orderID, "status", status, "error", err)
			return ResultSaveError
		}
		w.logInfo(prefix+"reverse deadline exceeded", "order_id", orderID, "status", status, "created_at", o.CreatedAt.String())
		return ResultLate
	}
	if err := w.o.Reporter.Report(ctx, orderID, status); err != nil {
		w.logError(prefix+"report failed", "order_id", orderID, "status", status, "error", err)
		return ResultReportError
	}
	if err := w.o.Repository.InsertFinal(ctx, o); err != nil {
		w.logError(prefix+"save failed", "order_id", orderID, "status", status, "error", err)
		return ResultSaveError
	}
	w.logInfo(prefix+"reported", "order_id", orderID, "status", status)
	if status == order.StatusSuccess {
		return ResultSuccess
	}
	return ResultFailed
}

// status asks the adapter. An error or no answer is pending (bill: a gRPC error is PENDING).
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
	if err != nil || resp == nil {
		return order.StatusPending
	}
	return resp.Status
}

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
