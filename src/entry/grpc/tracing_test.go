package grpcserver

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth/authtest"
	"qabz/common/monitoring/jaeger/jaegertest"
	"qabz/core/billtype"
	"qabz/core/inquiry"
	"qabz/core/order"
	"qabz/core/savedbill"
)

// coreSpan is the span that spanCore makes in the context that the handler gives it: the place of
// the core spans (inquiry.Service.Inquire, order.Service.Create, ...) in the App.
const coreSpan = "core"

// spanCore is every core dependency of the handlers. Each call makes one span coreSpan under the
// span in ctx and answers err (nil: a zero answer).
type spanCore struct{ err error }

func (c *spanCore) span(ctx context.Context) error {
	_, s := otel.Tracer("test").Start(ctx, coreSpan)
	s.End()
	return c.err
}

func (c *spanCore) Inquire(ctx context.Context, _ inquiry.Request) (inquiry.Result, error) {
	return inquiry.Result{}, c.span(ctx)
}
func (c *spanCore) RawInquiry(string, string, string) (inquiry.RawResult, error) {
	return inquiry.RawResult{}, errors.New("not used")
}
func (c *spanCore) Create(ctx context.Context, _ order.Request) (order.Order, error) {
	return order.Order{}, c.span(ctx)
}
func (c *spanCore) Complete(ctx context.Context, _ order.CompleteRequest) (order.Order, error) {
	return order.Order{}, c.span(ctx)
}
func (c *spanCore) List(ctx context.Context, _ string) ([]billtype.Service, error) {
	return nil, c.span(ctx)
}
func (c *spanCore) Insert(ctx context.Context, _ string, _ savedbill.InsertRequest) (savedbill.Bill, bool, error) {
	return savedbill.Bill{ID: "b-1"}, true, c.span(ctx)
}
func (c *spanCore) Delete(ctx context.Context, _, _ string) (string, bool, error) {
	return "", false, c.span(ctx)
}

// billLister adapts spanCore to BillLister (List has another signature than ServiceLister).
type billLister struct{ c *spanCore }

func (l billLister) List(ctx context.Context, _ string) ([]savedbill.Bill, error) { return nil, l.c.span(ctx) }

// tracedServers starts the public server (the auth check) and the internal server, both with the
// tracing handler, as app/defaults.go builds them, and returns a connection to each.
func tracedServers(t *testing.T, core *spanCore) (public, internal *grpc.ClientConn) {
	t.Helper()
	check, err := NewAuth(AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	trace := []grpc.ServerOption{grpc.StatsHandler(otelgrpc.NewServerHandler())}
	signer := &fakeSigner{}
	pub, err := NewWithOptions("127.0.0.1:0", append(append([]grpc.ServerOption(nil), trace...), check.ServerOptions()...),
		RegisterService(NewService(ServiceDeps{Services: core})),
		RegisterMyBills(NewMyBills(MyBillsDeps{Bills: core, Lister: billLister{core}, Deleter: core})),
		RegisterWater(NewWater(WaterDeps{Inquiry: core, Orders: core, Signer: signer, ServiceID: 105})),
		RegisterElectricity(NewElectricity(ElectricityDeps{Inquiry: core})),
		RegisterGas(NewGas(GasDeps{Inquiry: core, Raw: core})),
		RegisterPhone(NewPhone(PhoneDeps{Inquiry: core, Orders: core, Signer: signer, ServiceID: 105})),
		RegisterTel(NewTel(TelDeps{Inquiry: core, Raw: core, Orders: core, Signer: signer, ServiceID: 105})))
	if !assert.NoError(t, err) || !assert.NoError(t, pub.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = pub.Stop(context.Background()) })
	in, err := NewWithOptions("127.0.0.1:0", trace,
		RegisterPaymentComplete(NewPaymentComplete(PaymentCompleteDeps{Payments: core, ServiceID: 105, EcosystemName: "bill"})))
	if !assert.NoError(t, err) || !assert.NoError(t, in.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = in.Stop(context.Background()) })
	dial := func(s *Server) *grpc.ClientConn {
		conn, err := grpc.NewClient(s.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if !assert.NoError(t, err) {
			t.FailNow()
		}
		t.Cleanup(func() { _ = conn.Close() })
		return conn
	}
	return dial(pub), dial(in)
}

// tracedCall is one call of each path of task 049.
type tracedCall struct {
	method   string // the server span name (the full gRPC method, no leading slash)
	internal bool
	core     bool // the handler calls a core dependency
	call     func(ctx context.Context, public, internal *grpc.ClientConn) error
}

func tracedCalls() []tracedCall {
	return []tracedCall{
		{"haf80.api.bill.Service/Greeting", false, false, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewServiceClient(c).Greeting(ctx, &pb.GreetingRequest{})
			return err
		}},
		{"haf80.api.bill.Service/Get", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewServiceClient(c).Get(ctx, &emptypb.Empty{})
			return err
		}},
		{"haf80.api.bill.MyBills/Insert", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewMyBillsClient(c).Insert(ctx, &pb.InsertBillRequest{Type: pb.InsertBillRequest_water})
			return err
		}},
		{"haf80.api.bill.MyBills/GetAll", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewMyBillsClient(c).GetAll(ctx, &emptypb.Empty{})
			return err
		}},
		{"haf80.api.bill.MyBills/Delete", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewMyBillsClient(c).Delete(ctx, &pb.DeleteBillRequest{Id: "8f3a1c2e-4b5d-4e6f-9a7b-1c2d3e4f5a6b"})
			return err
		}},
		{"haf80.api.bill.Water/Inquiry", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewWaterClient(c).Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
			return err
		}},
		{"haf80.api.bill.Electricity/Inquiry", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewElectricityClient(c).Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
			return err
		}},
		{"haf80.api.bill.Gas/Inquiry", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewGasClient(c).Inquiry(ctx, &pb.InquiryBillPayRequest{ParticipateCode: gasCode})
			return err
		}},
		{"haf80.api.bill.Tel/Inquiry", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewTelClient(c).Inquiry(ctx, &pb.InquiryTelRequest{Phone: telPhone})
			return err
		}},
		{"haf80.api.bill.Phone/Inquiry", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewPhoneClient(c).Inquiry(ctx, irancellReq())
			return err
		}},
		{"haf80.api.bill.Phone/NewOrderV2", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewPhoneClient(c).NewOrderV2(ctx, orderReq())
			return err
		}},
		{"haf80.api.bill.Tel/NewOrderV2", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewTelClient(c).NewOrderV2(ctx, telOrderReq())
			return err
		}},
		{"haf80.api.bill.Water/NewOrderV2", false, true, func(ctx context.Context, c, _ *grpc.ClientConn) error {
			_, err := pb.NewWaterClient(c).NewOrderV2(ctx, waterOrderReq())
			return err
		}},
		{"haf80.api.bill.PaymentComplete/OrderComplete", true, true, func(ctx context.Context, _, c *grpc.ClientConn) error {
			_, err := pb.NewPaymentCompleteClient(c).OrderComplete(ctx, paymentReq())
			return err
		}},
	}
}

// serverSpan waits for the one server span of method among the spans that ended after skip, and
// returns it with the spans of the call.
func serverSpan(t *testing.T, sr *tracetest.SpanRecorder, skip int, method string) (sdktrace.ReadOnlySpan, []sdktrace.ReadOnlySpan) {
	t.Helper()
	var found sdktrace.ReadOnlySpan
	var spans []sdktrace.ReadOnlySpan
	assert.Eventually(t, func() bool {
		spans = sr.Ended()[skip:]
		for _, s := range spans {
			if s.Name() == method {
				found = s
				return true
			}
		}
		return false
	}, 2*time.Second, 5*time.Millisecond, method)
	if found == nil {
		t.FailNow()
	}
	return found, spans
}

// TestServerSpanTrees (conventions § 17; task 049 D4): for each path, the server span of the
// tracing handler is the root of the call, and the core step that the handler calls is its child
// (the handler gives the core the context of the call). A failure of the core gives the answer an
// error code; the server span then has the status that the tracing handler gives (D5). No span has
// the token, the user, the phone, the bill ID or the pay ID of the requests.
func TestServerSpanTrees(t *testing.T) {
	sr := jaegertest.Install(t)
	core := &spanCore{}
	public, internal := tracedServers(t, core)
	token := "Bearer " + authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", token)

	for _, failing := range []bool{false, true} {
		if failing {
			core.err = errors.New("db down for 989351234567")
		}
		for _, tc := range tracedCalls() {
			skip := len(sr.Ended())
			err := tc.call(ctx, public, internal)
			if failing && tc.core {
				assert.Error(t, err, tc.method)
			}
			server, spans := serverSpan(t, sr, skip, tc.method)
			assert.False(t, server.Parent().IsValid(), "%s: the server span is the root (no traceparent)", tc.method)
			var cores []sdktrace.ReadOnlySpan
			for _, s := range spans {
				if s.Name() == coreSpan {
					cores = append(cores, s)
				}
			}
			if !tc.core {
				assert.Empty(t, cores, tc.method)
				continue
			}
			if assert.Len(t, cores, 1, tc.method) {
				assert.Equal(t, server.SpanContext().SpanID(), cores[0].Parent().SpanID(), "%s: the core step is a child of the server span", tc.method)
				assert.Equal(t, server.SpanContext().TraceID(), cores[0].SpanContext().TraceID(), tc.method)
			}
		}
	}
	jaegertest.AssertClean(t, sr.Ended(), token[len("Bearer "):], authtest.UserID, billsUser, "989351234567", irPhone, telPhone,
		"1234567890125", "2500012340", gasCode, "778899", "db down")
}

// TestRejectedTokenSpan (task 049, the auth check): a call with a bad token has its server span
// with the status Error and the description AuthRejected, no core step, and no token in a span.
func TestRejectedTokenSpan(t *testing.T) {
	sr := jaegertest.Install(t)
	public, _ := tracedServers(t, &spanCore{})
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer not-a-token-989121234567")
	_, err := pb.NewWaterClient(public).Inquiry(ctx, &pb.InquiryBillPayRequest{BillId: "1234567890125"})
	assert.Error(t, err)
	server, spans := serverSpan(t, sr, 0, "haf80.api.bill.Water/Inquiry")
	assert.Equal(t, otelcodes.Error, server.Status().Code)
	assert.Equal(t, AuthRejected, server.Status().Description)
	for _, s := range spans {
		assert.NotEqual(t, coreSpan, s.Name(), "a rejected call reaches no handler")
	}
	jaegertest.AssertClean(t, sr.Ended(), "not-a-token", "989121234567", "1234567890125")
}
