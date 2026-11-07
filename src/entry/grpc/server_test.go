package grpcserver

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
)

// startServer starts a Server with the Service on a free port. It stops the test if a step fails.
func startServer(t *testing.T) *Server {
	t.Helper()
	s, err := New("127.0.0.1:0", RegisterService(NewService(ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		t.FailNow()
	}
	if !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s
}

// client dials s. It stops the test if the dial fails.
func client(t *testing.T, s *Server) pb.ServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(s.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewServiceClient(conn)
}

func TestNewErrors(t *testing.T) {
	_, err := New("", RegisterService(NewService(ServiceDeps{})))
	assert.ErrorContains(t, err, "address is required")
	_, err = New("127.0.0.1:0")
	assert.ErrorContains(t, err, "at least one service")
	_, err = New("127.0.0.1:0", RegisterService(NewService(ServiceDeps{})), nil)
	assert.ErrorContains(t, err, "register function 1 is nil")
}

func TestGreetingOverGRPC(t *testing.T) {
	s := startServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := client(t, s).Greeting(ctx, &pb.GreetingRequest{})
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		return
	}
	assert.Equal(t, "Hello", res.GetMessage())
}

func TestUnwrittenMethodIsUnimplemented(t *testing.T) {
	s := startServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client(t, s).NewOrder(ctx, &pb.RawOrderRequest{})
	if assert.Error(t, err) {
		assert.Equal(t, codes.Unimplemented, status.Code(err))
	}
}

func TestGreetingDirect(t *testing.T) {
	svc := NewService(ServiceDeps{})
	res, err := svc.Greeting(context.Background(), nil)
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, GreetingText, res.GetMessage())
	}

	res, err = svc.Greeting(nil, &pb.GreetingRequest{}) //nolint:staticcheck // a nil context is the case under test
	if assert.NoError(t, err) && assert.NotNil(t, res) {
		assert.Equal(t, GreetingText, res.GetMessage())
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err = svc.Greeting(ctx, &pb.GreetingRequest{})
	assert.Nil(t, res)
	if assert.Error(t, err) {
		assert.Equal(t, codes.Canceled, status.Code(err))
	}
}

func TestStartStopLifecycle(t *testing.T) {
	s, err := New("127.0.0.1:0", RegisterService(NewService(ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	assert.Equal(t, "127.0.0.1:0", s.Addr(), "before Start: the configured address")
	assert.NoError(t, s.Stop(context.Background()), "Stop before Start is a no-op")
	assert.ErrorContains(t, s.Start(context.Background()), "already stopped")

	s, err = New("127.0.0.1:0", RegisterService(NewService(ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	if !assert.NoError(t, s.Start(context.Background())) {
		return
	}
	assert.NotEqual(t, "127.0.0.1:0", s.Addr(), "after Start: the bound port")
	assert.ErrorContains(t, s.Start(context.Background()), "already started")
	assert.NoError(t, s.Stop(context.Background()))
	assert.NoError(t, s.Stop(context.Background()), "second Stop is a no-op")
	assert.NoError(t, s.Stop(nil), "a nil context does not panic") //nolint:staticcheck
}

func TestStopWithDoneContextForcesStop(t *testing.T) {
	s := startServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.NoError(t, s.Stop(ctx))
}

func TestStartFailsWhenPortIsTaken(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) || !assert.NotNil(t, ln) {
		return
	}
	defer ln.Close()
	s, err := New(ln.Addr().String(), RegisterService(NewService(ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	assert.ErrorContains(t, s.Start(context.Background()), "listen")
	assert.NoError(t, s.Stop(context.Background()), "a failed Start leaves nothing to stop")
}

func TestDialAddr(t *testing.T) {
	for in, want := range map[string]string{
		":8082":           "127.0.0.1:8082",
		"0.0.0.0:8082":    "127.0.0.1:8082",
		"[::]:8082":       "127.0.0.1:8082",
		"127.0.0.1:8082":  "127.0.0.1:8082",
		"example.org:80":  "example.org:80",
		"no-port-address": "no-port-address",
	} {
		assert.Equal(t, want, dialAddr(in), in)
	}
}

func TestDialAddrOfDefaultListenIsIPv4Loopback(t *testing.T) {
	s, err := New(":0", RegisterService(NewService(ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		return
	}
	if !assert.NoError(t, s.Start(context.Background())) {
		return
	}
	defer s.Stop(context.Background())
	host, _, err := net.SplitHostPort(s.DialAddr())
	if assert.NoError(t, err) {
		assert.Equal(t, "127.0.0.1", host, "listen %s", s.Addr())
	}
}

func TestZeroServerDoesNotPanic(t *testing.T) {
	var s Server
	assert.ErrorContains(t, s.Start(context.Background()), "not created with New")
	assert.NoError(t, s.Stop(context.Background()))
}

func TestRegisterServiceRejectsNil(t *testing.T) {
	assert.Nil(t, RegisterService(nil))
	var typed *Service
	assert.Nil(t, RegisterService(typed))
	_, err := New("127.0.0.1:0", RegisterService(nil))
	assert.ErrorContains(t, err, "register function 0 is nil")
}

type noAddrListener struct{ net.Listener }

func (noAddrListener) Addr() net.Addr { return nil }

func TestBoundAddrFallback(t *testing.T) {
	assert.Equal(t, "fb", boundAddr(nil, "fb"))
	assert.Equal(t, "fb", boundAddr(noAddrListener{}, "fb"))
}
