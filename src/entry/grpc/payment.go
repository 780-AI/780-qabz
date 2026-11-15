package grpcserver

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/core/order"
	"qabz/provider"
)

// paymentErrorText is bill's text of every failed step of OrderComplete (DataBaseErrorType,
// InvalidParameterErrorType: the same user message).
const paymentErrorText = "خطای داخلی رخ داده است"

// errPayment is the answer of a failed step: gRPC Unknown with bill's text. bill returns a plain
// error, which grpc-go sends as Unknown with the error text; the payment service gets the same.
var errPayment = status.Error(codes.Unknown, paymentErrorText)

// OrderCompleter runs the payment step (order.Payments).
type OrderCompleter interface {
	Complete(ctx context.Context, req order.CompleteRequest) (order.Order, error)
}

// PaymentLogger writes the lines of OrderComplete (the App logger has it).
type PaymentLogger interface {
	Info(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// PaymentCompleteDeps are the dependencies of PaymentComplete.
type PaymentCompleteDeps struct {
	// Payments runs the payment step. Nil: OrderComplete answers Unimplemented.
	Payments OrderCompleter
	// ServiceID is service.id: a request for another service is invalid.
	ServiceID uint32
	// ServiceName is service.name: currentStep of the answer.
	ServiceName string
	// Logger gets the lines of bill. Nil: no line.
	Logger PaymentLogger
}

// PaymentComplete implements the internal gRPC service haf80.api.bill.PaymentComplete (task 044;
// bill: controller/payment Payment). It is served only on the internal gRPC server
// (server.internal_grpc.addr), with no auth check, as bill.
type PaymentComplete struct {
	pb.UnimplementedPaymentCompleteServer // embedded by value, as protoc-gen-go-grpc requires

	payments    OrderCompleter
	serviceID   uint32
	serviceName string
	logger      PaymentLogger
}

// NewPaymentComplete returns the PaymentComplete implementation.
func NewPaymentComplete(deps PaymentCompleteDeps) *PaymentComplete {
	return &PaymentComplete{payments: deps.Payments, serviceID: deps.ServiceID, serviceName: deps.ServiceName, logger: deps.Logger}
}

// RegisterPaymentComplete returns a Register that adds svc to a gRPC server. For a nil svc it
// returns nil, so New rejects it.
func RegisterPaymentComplete(svc pb.PaymentCompleteServer) Register {
	if svc == nil {
		return nil
	}
	if p, ok := svc.(*PaymentComplete); ok && p == nil {
		return nil
	}
	return func(r grpc.ServiceRegistrar) { pb.RegisterPaymentCompleteServer(r, svc) }
}

// OrderComplete is the call of the payment service after a payment (bill: Payment.OrderComplete):
//   - version + 1 and currentStep = service.name are set on the request, which is the answer.
//   - The request check: payload.serviceId is service.id, stepStatus.payment is "success" and
//     payload.rrn is not empty. Else one error line "order complete: invalid payment request" and
//     the error of bill. A request with no payload or no stepStatus is invalid (bill fails on it).
//   - The payment step (order.Payments.Complete). A failed step: one info line "order complete:
//     failed payment" and the error of bill.
//   - A failed payment: one info line "order complete: failed payment" and an error with the
//     message of the provider (gRPC Unknown, as bill's errors.New(order.Message)). The payment
//     service refunds it.
//   - Else (success or pending): status and stepStatus.service are the status, data is {"refID":
//     ref_id}, one info line "order complete: successful payment" (bill writes it for pending too).
func (p *PaymentComplete) OrderComplete(ctx context.Context, req *pb.PaymentOrder) (*pb.PaymentOrder, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p.payments == nil {
		return nil, status.Error(codes.Unimplemented, "method OrderComplete not available")
	}
	if req == nil {
		req = &pb.PaymentOrder{}
	}
	req.Version++
	req.CurrentStep = p.serviceName
	payload, step := req.GetPayload(), req.GetStepStatus()
	if payload == nil || step == nil || payload.GetServiceId() != p.serviceID || step.GetPayment() != provider.StatusSuccess || payload.GetRrn() == "" {
		p.log(true, "order complete: invalid payment request", "order_id", payload.GetOrderId(), "request", fmt.Sprintf("%+v", req))
		return nil, errPayment
	}
	o, err := p.payments.Complete(ctx, order.CompleteRequest{OrderID: payload.GetOrderId(), RRN: payload.GetRrn(), Price: payload.GetPrice()})
	if err != nil {
		p.log(false, "order complete: failed payment", "order_id", payload.GetOrderId(), "status", order.StatusFailed, "error", err.Error(),
			"not_found", errors.Is(err, order.ErrNotFound))
		return nil, errPayment
	}
	kv := []any{"user_id", o.UserID, "order_id", o.OrderID, "bill_id", o.BillID, "inquiry_id", o.Data, "bill_type", o.BillType,
		"status", o.Status, "provider_name", o.ProviderName}
	if o.Status == order.StatusFailed {
		p.log(false, "order complete: failed payment", append(kv, "error", o.Message)...)
		return nil, status.Error(codes.Unknown, o.Message)
	}
	req.Status = o.Status
	step.Service = o.Status
	req.Data = map[string]string{"refID": o.RefID}
	p.log(false, "order complete: successful payment", kv...)
	return req, nil
}

func (p *PaymentComplete) log(isError bool, msg string, kv ...any) {
	if p.logger == nil {
		return
	}
	if isError {
		p.logger.Error(msg, kv...)
		return
	}
	p.logger.Info(msg, kv...)
}
