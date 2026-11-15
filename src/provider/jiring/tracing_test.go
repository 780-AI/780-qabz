package jiring

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	pb "qabz/api/pb/jiringSwitch"
	"qabz/common/monitoring/jaeger/jaegertest"
)

// metaServer records the traceparent metadata of each call.
type metaServer struct {
	pb.UnimplementedInternalInquiryServiceServer
	mu   sync.Mutex
	seen []string
}

func (m *metaServer) BillInquiry(ctx context.Context, _ *pb.BillInquiryRequest) (*pb.BillInquiryResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	m.mu.Lock()
	m.seen = append(m.seen, md.Get("traceparent")...)
	m.mu.Unlock()
	return &pb.BillInquiryResponse{}, nil
}

// TestDialTraces (task 047 D5): the client of Dial makes a client span (the full gRPC method), a
// child of the span in ctx, and sends its traceparent in the metadata.
func TestDialTraces(t *testing.T) {
	sr := jaegertest.Install(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	srv := grpc.NewServer()
	m := &metaServer{}
	pb.RegisterInternalInquiryServiceServer(srv, m)
	go func() { _ = srv.Serve(ln) }()
	defer srv.Stop()

	c, err := Dial(ln.Addr().String())
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	_, err = c.BillInquiry(ctx, &pb.BillInquiryRequest{})
	assert.NoError(t, err)
	parent.End()

	var client trace.SpanContext
	for _, s := range sr.Ended() {
		if s.SpanKind() == trace.SpanKindClient {
			client = s.SpanContext()
			assert.Equal(t, parent.SpanContext().SpanID(), s.Parent().SpanID())
			assert.Contains(t, s.Name(), "/BillInquiry")
		}
	}
	if assert.True(t, client.IsValid(), "a client span") {
		m.mu.Lock()
		defer m.mu.Unlock()
		assert.Equal(t, []string{"00-" + client.TraceID().String() + "-" + client.SpanID().String() + "-01"}, m.seen)
	}
	jaegertest.AssertClean(t, sr.Ended())
}
