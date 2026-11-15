package httpserver

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"

	pb "qabz/api/pb/qabz"
	"qabz/common/monitoring/jaeger/jaegertest"
	grpcserver "qabz/entry/grpc"
)

const greetingMethod = "haf80.api.bill.Service/Greeting"

// tracedStack is stack with the tracing handler on the gRPC server, as app/defaults.go builds it.
func tracedStack(t *testing.T) *Gateway {
	t.Helper()
	gs, err := grpcserver.NewWithOptions("127.0.0.1:0", []grpc.ServerOption{grpc.StatsHandler(otelgrpc.NewServerHandler())},
		grpcserver.RegisterService(grpcserver.NewService(grpcserver.ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, gs) || !assert.NoError(t, gs.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gs.Stop(context.Background()) })
	gw, err := New("127.0.0.1:0", gs.DialAddr, pb.RegisterServiceHandler)
	if !assert.NoError(t, err) || !assert.NotNil(t, gw) || !assert.NoError(t, gw.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = gw.Stop(context.Background()) })
	return gw
}

// greet sends GET /v1/greeting with the traceparent header (none when empty).
func greet(t *testing.T, gw *Gateway, traceparent string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+gw.Addr()+"/v1/greeting", nil)
	if !assert.NoError(t, err) || !assert.NotNil(t, req) {
		t.FailNow()
	}
	if traceparent != "" {
		req.Header.Set("traceparent", traceparent)
	}
	res, err := http.DefaultClient.Do(req)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		t.FailNow()
	}
	_ = res.Body.Close()
	return res.StatusCode
}

// spansOf waits until the client and the server span of one call ended (the server span can end
// a moment after the answer) and returns them.
func spansOf(t *testing.T, ended func() []sdktrace.ReadOnlySpan) (client, server sdktrace.ReadOnlySpan) {
	t.Helper()
	assert.Eventually(t, func() bool { return len(ended()) >= 2 }, 2*time.Second, 10*time.Millisecond)
	for _, s := range ended() {
		switch s.SpanKind() {
		case trace.SpanKindClient:
			client = s
		case trace.SpanKindServer:
			server = s
		}
	}
	if !assert.NotNil(t, client, "the client span of the gateway") || !assert.NotNil(t, server, "the gRPC server span") {
		t.FailNow()
	}
	return client, server
}

// TestGatewayContinuesTheTrace (task 047 D4): the gateway reads traceparent and makes no HTTP span;
// its gRPC client span is a child of the app span, and the gRPC server span is a child of the client
// span: one trace.
func TestGatewayContinuesTheTrace(t *testing.T) {
	sr := jaegertest.Install(t)
	gw := tracedStack(t)

	const traceID, parentID = "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7"
	assert.Equal(t, http.StatusOK, greet(t, gw, "00-"+traceID+"-"+parentID+"-01"))
	client, server := spansOf(t, sr.Ended)

	assert.Len(t, sr.Ended(), 2, "no HTTP span")
	assert.Equal(t, greetingMethod, client.Name())
	assert.Equal(t, greetingMethod, server.Name())
	assert.Equal(t, traceID, client.SpanContext().TraceID().String())
	assert.Equal(t, parentID, client.Parent().SpanID().String(), "the client span continues the app span")
	assert.Equal(t, traceID, server.SpanContext().TraceID().String())
	assert.Equal(t, client.SpanContext().SpanID(), server.Parent().SpanID(), "the server span is a child of the client span")
	jaegertest.AssertClean(t, sr.Ended(), "/v1/greeting")
}

// TestGatewayStartsATrace: with no traceparent, the client span of the gateway is the root.
func TestGatewayStartsATrace(t *testing.T) {
	sr := jaegertest.Install(t)
	gw := tracedStack(t)

	assert.Equal(t, http.StatusOK, greet(t, gw, ""))
	client, server := spansOf(t, sr.Ended)
	assert.False(t, client.Parent().IsValid(), "a root span")
	assert.Equal(t, client.SpanContext().TraceID(), server.SpanContext().TraceID())
	assert.Equal(t, client.SpanContext().SpanID(), server.Parent().SpanID())
}
