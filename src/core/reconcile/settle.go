package reconcile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"qabz/core/order"
	"qabz/provider"
)

// The settlement worker (task 046; bill: pkg/cronTask/paymentConfirm.go and
// service/billPayment ConfirmPaymentBunch): it settles again each paid landline order that the
// settlement provider (Karashop) did not confirm. A failed settlement changes nothing, so the next
// run tries again until the order leaves the window. There is no reject (supertask 043, operator
// decision B, as bill).

// SettleBillTypes are the bill types of the settlement worker, in the order of a run (bill: tel
// and telmidterm).
var SettleBillTypes = []string{provider.BillTypeTel, provider.BillTypeTelMidterm}

// ResultNoRoute is the result of an order with no settlement provider (order.SettleNoRoute).
const ResultNoRoute = order.SettleNoRoute

// SettleRepository reads the orders of the settlement worker (postgres.OrderRepository).
type SettleRepository interface {
	// ConfirmCandidates gives one page of the paid, not confirmed orders of the bill type created
	// after from and before to, with an order_id greater than after ("" is the first page), in
	// order_id order (bill: GetOrdersForConfirmPayment).
	ConfirmCandidates(ctx context.Context, billType, after string, limit int, from, to time.Time) ([]order.Order, error)
}

// Settler settles one order and returns the result (order.Settler).
type Settler interface {
	Settle(ctx context.Context, o order.Order) string
}

// SettleOptions configure NewSettleWorker.
type SettleOptions struct {
	// Name is the name of the worker (confirm_payment.telephone.name): the metric label cron.
	Name string
	// Interval is the time between two runs (confirm_payment.telephone.every).
	Interval time.Duration
	// Start and End give the window of the orders: created after now-End and before now-Start.
	Start, End time.Duration
	// Chunk is the page size. Positive.
	Chunk int

	Repository SettleRepository
	Settler    Settler
	Logger     Logger // nil: no line
	// OnOrder gets the result of each order (the results of order.Settler.Settle). Nil: nothing.
	OnOrder func(name, result string)
}

// SettleWorker is the settlement worker.
type SettleWorker struct {
	o   SettleOptions
	now func() time.Time
}

// NewSettleWorker checks the options and returns the worker.
func NewSettleWorker(o SettleOptions) (*SettleWorker, error) {
	switch {
	case o.Name == "":
		return nil, errors.New("reconcile: the name is required")
	case o.Interval <= 0 || o.Chunk <= 0:
		return nil, errors.New("reconcile: the interval and the chunk must be positive")
	case o.Start < 0 || o.End <= o.Start:
		return nil, errors.New("reconcile: the window must have 0 <= start < end")
	case o.Repository == nil || o.Settler == nil:
		return nil, errors.New("reconcile: the repository and the settler are required")
	}
	return &SettleWorker{o: o, now: time.Now}, nil
}

// Name, Interval and Chunk give what the runner needs.
func (w *SettleWorker) Name() string            { return w.o.Name }
func (w *SettleWorker) Interval() time.Duration { return w.o.Interval }
func (w *SettleWorker) Chunk() int              { return w.o.Chunk }

// Run settles the orders of the window, one bill type after the other and one page after the
// other (bill: ConfirmPaymentBunch):
//  1. The window is computed once: created after now-End and before now-Start.
//  2. For each bill type of SettleBillTypes: read a page (a failed read ends the run with the
//     error); no order: the next bill type; charge the lock for the page (a lost lock ends the
//     run); settle each order with the context of the run; the next page starts after the last
//     order_id of the page.
//
// @todo docs/this_vs_bill.md item 45: each order at most once per run (bill reads the first page
// again, so its run never ends while one settlement fails); the bill types one after the other
// (bill: at the same time); the context of the run (bill: context.Background()).
//
// A done ctx (the App stop) ends the run.
func (w *SettleWorker) Run(ctx context.Context, ext Charger) error {
	now := w.now()
	from, to := now.Add(-w.o.End), now.Add(-w.o.Start)
	for _, billType := range SettleBillTypes {
		n, err := w.runBillType(ctx, ext, billType, from, to)
		if err != nil {
			return err
		}
		if w.o.Logger != nil {
			w.o.Logger.Info("settle worker: bill type done", "name", w.o.Name, "bill_type", billType, "orders", n)
		}
	}
	return nil
}

// runBillType settles the orders of one bill type and returns their count.
func (w *SettleWorker) runBillType(ctx context.Context, ext Charger, billType string, from, to time.Time) (int, error) {
	after, total := "", 0
	for {
		orders, err := w.o.Repository.ConfirmCandidates(ctx, billType, after, w.o.Chunk, from, to)
		if err != nil {
			return total, fmt.Errorf("reconcile: read the orders to settle (%s): %w", billType, err)
		}
		if len(orders) == 0 {
			return total, nil
		}
		if ext != nil {
			if err := ext.Charge(ctx, len(orders)); err != nil {
				return total, fmt.Errorf("reconcile: keep the lock: %w", err)
			}
		}
		for _, o := range orders {
			if err := ctx.Err(); err != nil {
				return total, err
			}
			result := w.o.Settler.Settle(ctx, o)
			total++
			if w.o.OnOrder != nil {
				w.o.OnOrder(w.o.Name, result)
			}
		}
		next := orders[len(orders)-1].OrderID
		if next == "" || next <= after {
			return total, fmt.Errorf("reconcile: the page of %s did not move forward after %q", billType, after)
		}
		after = next
		if len(orders) < w.o.Chunk {
			return total, nil
		}
	}
}
