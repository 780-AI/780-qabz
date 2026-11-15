package order

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"qabz/common/monitoring/jaeger"
	"qabz/provider"
)

// The settlement of a paid order with the provider of the bill (bill: payment_service
// ConfirmPayment). Task 044 runs it at once after a successful payment (Payments); task 046 runs it
// again from the settlement worker (core/reconcile) for each order that the provider did not
// confirm. Both use one Settler, so the call, the log lines and the metric are the same.

// SettleNoRoute is the result of an order whose bill type and order provider have no settlement
// provider (bill: ConfirmPaymentFactory fails; only the worker counts it).
const SettleNoRoute = "no_route"

// The sources of a settlement, for the log lines and the metric label source.
const (
	SettleSourcePayment = "payment" // the at-once settlement after OrderComplete (task 044)
	SettleSourceWorker  = "worker"  // the settlement worker (task 046)
)

// settleSpan is the span name of one settlement (conventions § 17).
const settleSpan = "order.Settler.Settle"

// SettleRepository saves the confirmation of the provider.
type SettleRepository interface {
	// ConfirmProvider sets provider_confirmed of the success row of the order created in the last
	// LatestWindow (bill: UpdateProviderConfirm).
	ConfirmProvider(ctx context.Context, orderID string) error
}

// SettlerOptions configure NewSettler.
type SettlerOptions struct {
	// Repository and Adapters are required.
	Repository SettleRepository
	Adapters   Adapters
	// Routes give the provider of the settlement by bill type and order provider (bill: the
	// confirm payment gateway map). No entry: no settlement. Read-only.
	Routes map[string]map[string]string
	// Source is SettleSourcePayment or SettleSourceWorker: the key source of the log lines.
	Source string
	// Logger gets the lines. Nil: no line.
	Logger PaymentLogger
	// OnSettle is called once per settlement with the bill type, the settlement provider ("" for
	// SettleNoRoute) and the result. Nil: no call.
	OnSettle func(billType, providerName, result string)
}

// Settler settles one paid order. It is safe for concurrent use.
type Settler struct {
	repo     SettleRepository
	adapters Adapters
	routes   map[string]map[string]string
	source   string
	log      PaymentLogger
	onSettle func(billType, providerName, result string)
}

// NewSettler returns the Settler.
func NewSettler(o SettlerOptions) (*Settler, error) {
	if o.Repository == nil || o.Adapters == nil {
		return nil, errors.New("order settle: repository and adapters are required")
	}
	if o.Source != SettleSourcePayment && o.Source != SettleSourceWorker {
		return nil, errors.New("order settle: source must be payment or worker")
	}
	return &Settler{repo: o.Repository, adapters: o.Adapters, routes: o.Routes, source: o.Source, log: o.Logger, onSettle: o.OnSettle}, nil
}

// Route gives the settlement provider of the bill type and the order provider.
func (s *Settler) Route(billType, providerName string) (string, bool) {
	if s == nil {
		return "", false
	}
	settler, ok := s.routes[billType][providerName]
	return settler, ok
}

// Settle reports the paid order o to its settlement provider and, on a success, saves
// provider_confirmed (bill: ConfirmPayment). It returns the result, which is also counted:
//   - SettleNoRoute: no settlement provider for the bill type and the order provider; no call;
//   - no_data: the order has no number (bill: no settlement);
//   - SettleError: no adapter, or the call failed;
//   - the status of the answer (failed, pending) when it is not success;
//   - SettleMarkFailed: the provider confirmed, but provider_confirmed was not saved;
//   - success: confirmed and saved.
//
// Every result but success leaves provider_confirmed false, so the settlement worker tries again.
// One span settleSpan, a child of the span in ctx (conventions § 17), has the order ID, the bill
// type, the provider and the result, and the status Error for every result but success.
func (s *Settler) Settle(ctx context.Context, o Order) (result string) {
	if s == nil {
		return SettleError
	}
	ctx, span := otel.Tracer("qabz/core/order").Start(ctx, settleSpan)
	settler, ok := s.Route(o.BillType, o.ProviderName)
	defer func() {
		span.SetAttributes(attribute.String("order.id", o.OrderID), attribute.String("bill_type", o.BillType),
			attribute.String("provider", settler), attribute.String("result", result), attribute.String("source", s.source))
		if result != provider.StatusSuccess {
			jaeger.Fail(span, result, nil)
		}
		span.End()
	}()
	if !ok {
		s.count(o.BillType, "", SettleNoRoute)
		return SettleNoRoute
	}
	if o.Data == "" {
		s.count(o.BillType, settler, settleNoInquiryID)
		return settleNoInquiryID
	}
	adapter, ok := s.adapters.Get(settler)
	if !ok || adapter == nil {
		s.count(o.BillType, settler, SettleError)
		s.logError("settle: "+settler+" failed", "order_id", o.OrderID, "source", s.source, "error", "no adapter")
		return SettleError
	}
	resp, err := adapter.Settle(ctx, provider.SettleRequest{
		BillType: o.BillType, TelephoneNumber: o.Data, Amount: uint64(max(o.BasePrice, 0)), BillID: o.BillID,
		PayID: o.PayID, RRN: o.RRN, PaidAt: o.CreatedAt, PaymentProvider: o.ProviderName,
	})
	switch {
	case err != nil:
		jaeger.Fail(span, SettleError, err)
		s.count(o.BillType, settler, SettleError)
		s.logError("settle: "+settler+" failed", "order_id", o.OrderID, "source", s.source, "error", err)
		return SettleError
	case resp == nil || resp.Status != provider.StatusSuccess:
		status := SettleError
		var code int32
		var message string
		if resp != nil {
			status, code, message = resp.Status, resp.Code, resp.Message
		}
		s.count(o.BillType, settler, status)
		s.logError("settle: "+settler+" failed", "order_id", o.OrderID, "source", s.source, "status", status, "code", code, "message", message)
		return status
	}
	if err := s.repo.ConfirmProvider(ctx, o.OrderID); err != nil {
		jaeger.Fail(span, SettleMarkFailed, err)
		s.count(o.BillType, settler, SettleMarkFailed)
		s.logError("settle: save provider_confirmed failed", "order_id", o.OrderID, "source", s.source, "error", err)
		return SettleMarkFailed
	}
	s.count(o.BillType, settler, provider.StatusSuccess)
	if s.log != nil {
		s.log.Info("settle: "+settler+" confirmed", "order_id", o.OrderID, "source", s.source)
	}
	return provider.StatusSuccess
}

func (s *Settler) count(billType, providerName, result string) {
	if s.onSettle != nil {
		s.onSettle(billType, providerName, result)
	}
}

func (s *Settler) logError(msg string, kv ...any) {
	if s.log != nil {
		s.log.Error(msg, kv...)
	}
}
