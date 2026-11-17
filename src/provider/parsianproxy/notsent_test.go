package parsianproxy

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/parsian-v2"
	"qabz/provider"
)

// unavailableServer answers every payment with the gRPC status Unavailable: the call reached it.
type unavailableServer struct {
	pb.UnimplementedTopBillServer
}

func (unavailableServer) Pay(context.Context, *pb.PayRequest) (*pb.PayResponse, error) {
	return nil, status.Error(codes.Unavailable, "parsian is busy")
}

// TestPayNotSentRealGRPC (task 058): with the real gRPC client of Dial, a refused connection is
// ErrNotSent (the payment step can go to the next provider); an Unavailable answer of the server is
// only ErrProviderDown (the call reached the service: pending).
func TestPayNotSentRealGRPC(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	closed := lis.Addr().String()
	assert.NoError(t, lis.Close()) // nothing listens on the address now: the connection is refused

	c, err := Dial(closed)
	if !assert.NoError(t, err) {
		return
	}
	a, err := New(Options{Enabled: true, Client: c, Timeout: 5 * time.Second})
	if !assert.NoError(t, err) {
		return
	}
	_, err = a.Pay(context.Background(), payRequest())
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.ErrorIs(t, err, provider.ErrNotSent, "a refused connection did not reach the service")
	assert.Equal(t, codes.Unavailable, status.Code(err))

	lis, err = net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	srv := grpc.NewServer()
	pb.RegisterTopBillServer(srv, unavailableServer{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	c, err = Dial(lis.Addr().String())
	if !assert.NoError(t, err) {
		return
	}
	a, err = New(Options{Enabled: true, Client: c, Timeout: 5 * time.Second})
	if !assert.NoError(t, err) {
		return
	}
	_, err = a.Pay(context.Background(), payRequest())
	assert.ErrorIs(t, err, provider.ErrProviderDown)
	assert.NotErrorIs(t, err, provider.ErrNotSent, "the server answered Unavailable: the call reached it")
	assert.Equal(t, codes.Unavailable, status.Code(err))
}
