package grpcserver

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/core/order"
)

type fakePayments struct {
	reqs []order.CompleteRequest
	o    order.Order
	err  error
}

func (f *fakePayments) Complete(_ context.Context, req order.CompleteRequest) (order.Order, error) {
	f.reqs = append(f.reqs, req)
	return f.o, f.err
}

type payLogger struct {
	mu    sync.Mutex
	lines []string
}

func (l *payLogger) Info(msg string, _ ...any)  { l.add("info: " + msg) }
func (l *payLogger) Error(msg string, _ ...any) { l.add("error: " + msg) }
func (l *payLogger) add(s string)               { l.mu.Lock(); l.lines = append(l.lines, s); l.mu.Unlock() }

func paymentReq() *pb.PaymentOrder {
	return &pb.PaymentOrder{
		Id: "p-1", Version: 3, CurrentStep: "payment", Status: "x", Type: "t",
		Payload:    &pb.PaymentOrder_Payload{OrderId: "order-1", UserId: billsUser, PaymentId: "pay-1", ServiceId: 105, Price: 150000, Rrn: "778899"},
		StepStatus: &pb.PaymentOrder_StepStatus{Payment: "success"},
	}
}

func newPaymentComplete(p *fakePayments, log *payLogger) *PaymentComplete {
	deps := PaymentCompleteDeps{Payments: p, ServiceID: 105, EcosystemName: "bill"}
	if log != nil { // a typed nil in the interface is not "no logger"
		deps.Logger = log
	}
	return NewPaymentComplete(deps)
}

// TestOrderCompleteSuccess (task 044): the request goes back with version + 1, currentStep the
// service name, status and stepStatus.service the status, data {"refID": ...}; one info line.
func TestOrderCompleteSuccess(t *testing.T) {
	for _, st := range []string{order.StatusSuccess, order.StatusPending} {
		p := &fakePayments{o: order.Order{Status: st, RefID: "777", OrderID: "order-1"}}
		log := &payLogger{}
		res, err := newPaymentComplete(p, log).OrderComplete(context.Background(), paymentReq())
		if !assert.NoError(t, err, st) || !assert.NotNil(t, res, st) {
			continue
		}
		assert.Equal(t, []order.CompleteRequest{{OrderID: "order-1", RRN: "778899", Price: 150000}}, p.reqs, st)
		assert.Equal(t, int32(4), res.GetVersion(), st)
		assert.Equal(t, "bill", res.GetCurrentStep(), st)
		assert.Equal(t, st, res.GetStatus(), st)
		assert.Equal(t, st, res.GetStepStatus().GetService(), st)
		assert.Equal(t, "success", res.GetStepStatus().GetPayment(), st)
		assert.Equal(t, map[string]string{"refID": "777"}, res.GetData(), st)
		assert.Equal(t, "p-1", res.GetId(), st)
		assert.Equal(t, []string{"info: order complete: successful payment"}, log.lines, st)
	}
}

// TestOrderCompleteFailed: a failed payment is gRPC Unknown with the message of the provider; a
// failed step is Unknown with bill's text; each writes one info line.
func TestOrderCompleteFailed(t *testing.T) {
	log := &payLogger{}
	_, err := newPaymentComplete(&fakePayments{o: order.Order{Status: order.StatusFailed, Message: "insufficient"}}, log).OrderComplete(context.Background(), paymentReq())
	assert.Equal(t, codes.Unknown, status.Code(err))
	assert.Equal(t, "insufficient", status.Convert(err).Message())
	assert.Equal(t, []string{"info: order complete: failed payment"}, log.lines)

	for _, e := range []error{order.ErrPaymentDatabase, order.ErrPaymentRoute, errors.New("x")} {
		log := &payLogger{}
		_, err := newPaymentComplete(&fakePayments{err: e}, log).OrderComplete(context.Background(), paymentReq())
		assert.Equal(t, codes.Unknown, status.Code(err), e.Error())
		assert.Equal(t, paymentErrorText, status.Convert(err).Message(), e.Error())
		assert.Equal(t, []string{"info: order complete: failed payment"}, log.lines, e.Error())
	}
}

// TestOrderCompleteInvalid: another service ID, a payment step that is not success, no RRN, no
// payload or no stepStatus, a nil request: bill's error, one error line, no payment.
func TestOrderCompleteInvalid(t *testing.T) {
	for name, mod := range map[string]func(r *pb.PaymentOrder){
		"service id":   func(r *pb.PaymentOrder) { r.Payload.ServiceId = 106 },
		"payment step": func(r *pb.PaymentOrder) { r.StepStatus.Payment = "failed" },
		"no rrn":       func(r *pb.PaymentOrder) { r.Payload.Rrn = "" },
		"no payload":   func(r *pb.PaymentOrder) { r.Payload = nil },
		"no step":      func(r *pb.PaymentOrder) { r.StepStatus = nil },
	} {
		p, log := &fakePayments{}, &payLogger{}
		r := paymentReq()
		mod(r)
		_, err := newPaymentComplete(p, log).OrderComplete(context.Background(), r)
		assert.Equal(t, codes.Unknown, status.Code(err), name)
		assert.Equal(t, paymentErrorText, status.Convert(err).Message(), name)
		assert.Empty(t, p.reqs, name)
		assert.Equal(t, []string{"error: order complete: invalid payment request"}, log.lines, name)
	}
	_, err := newPaymentComplete(&fakePayments{}, nil).OrderComplete(context.Background(), nil)
	assert.Equal(t, codes.Unknown, status.Code(err))
}

func TestOrderCompleteNotAvailable(t *testing.T) {
	_, err := NewPaymentComplete(PaymentCompleteDeps{}).OrderComplete(context.Background(), paymentReq())
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	assert.Nil(t, RegisterPaymentComplete(nil))
	var typedNil *PaymentComplete
	assert.Nil(t, RegisterPaymentComplete(typedNil))
	assert.NotNil(t, RegisterPaymentComplete(NewPaymentComplete(PaymentCompleteDeps{})))
}
