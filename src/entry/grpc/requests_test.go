package grpcserver

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth/authtest"
)

type observed struct {
	path, code string
	d          time.Duration
}

// recorder is a RequestObserver that keeps every call. The server calls it from its own goroutines.
type recorder struct {
	mu    sync.Mutex
	calls []observed
}

func (r *recorder) observe(path, code string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, observed{path: path, code: code, d: d})
}

func (r *recorder) all() []observed {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]observed(nil), r.calls...)
}

// newRequests returns Requests over a new recorder. It stops the test if NewRequests fails.
func newRequests(t *testing.T) (*Requests, *recorder) {
	t.Helper()
	rec := &recorder{}
	r, err := NewRequests(rec.observe)
	if !assert.NoError(t, err) || !assert.NotNil(t, r) {
		t.FailNow()
	}
	return r, rec
}

func TestNewRequestsNeedsObserver(t *testing.T) {
	_, err := NewRequests(nil)
	assert.ErrorContains(t, err, "observer is required")
}

func TestRequestsUnaryRecords(t *testing.T) {
	cases := []struct {
		name     string
		info     *grpc.UnaryServerInfo
		err      error
		wantPath string
		wantCode string
	}{
		{name: "ok", info: &grpc.UnaryServerInfo{FullMethod: "/svc/A"}, wantPath: "/svc/A", wantCode: "OK"},
		{name: "status error", info: &grpc.UnaryServerInfo{FullMethod: "/svc/A"}, err: status.Error(codes.NotFound, "x"), wantPath: "/svc/A", wantCode: "NotFound"},
		{name: "plain error", info: &grpc.UnaryServerInfo{FullMethod: "/svc/B"}, err: errors.New("boom"), wantPath: "/svc/B", wantCode: "Unknown"},
		{name: "no info", info: nil, wantPath: UnknownPath, wantCode: "OK"},
		{name: "empty method", info: &grpc.UnaryServerInfo{}, wantPath: UnknownPath, wantCode: "OK"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, rec := newRequests(t)
			res, err := r.Unary()(context.Background(), "req", tc.info, func(context.Context, any) (any, error) {
				time.Sleep(20 * time.Millisecond)
				return "res", tc.err
			})
			assert.Equal(t, "res", res, "the response passes through")
			assert.Equal(t, tc.err, err, "the error passes through")
			calls := rec.all()
			if !assert.Len(t, calls, 1) {
				return
			}
			assert.Equal(t, tc.wantPath, calls[0].path)
			assert.Equal(t, tc.wantCode, calls[0].code)
			assert.GreaterOrEqual(t, calls[0].d, 20*time.Millisecond)
			assert.Less(t, calls[0].d, 5*time.Second)
		})
	}
}

type nopStream struct{ grpc.ServerStream }

func TestRequestsStreamRecords(t *testing.T) {
	r, rec := newRequests(t)
	ss := &nopStream{}
	err := r.Stream()(nil, ss, &grpc.StreamServerInfo{FullMethod: "/svc/S"}, func(_ any, got grpc.ServerStream) error {
		assert.Same(t, ss, got, "the stream passes through")
		time.Sleep(10 * time.Millisecond)
		return status.Error(codes.Canceled, "gone")
	})
	assert.Equal(t, codes.Canceled, status.Code(err))
	calls := rec.all()
	if assert.Len(t, calls, 1) {
		assert.Equal(t, observed{path: "/svc/S", code: "Canceled", d: calls[0].d}, calls[0])
		assert.GreaterOrEqual(t, calls[0].d, 10*time.Millisecond)
	}

	err = r.Stream()(nil, ss, nil, func(any, grpc.ServerStream) error { return nil })
	assert.NoError(t, err)
	calls = rec.all()
	if assert.Len(t, calls, 2) {
		assert.Equal(t, UnknownPath, calls[1].path)
		assert.Equal(t, "OK", calls[1].code)
	}
}

// TestRequestsBeforeAuth: with the request options before the auth check (the order of
// app.NewDefaultBuilder), a rejected call is measured with its code, a valid call with OK, and an
// unwritten method with Unimplemented. The path is always the full gRPC method.
func TestRequestsBeforeAuth(t *testing.T) {
	r, rec := newRequests(t)
	a, err := NewAuth(AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		return
	}
	opts := append(r.ServerOptions(), a.ServerOptions()...)
	s, err := NewWithOptions("127.0.0.1:0", opts, RegisterService(NewService(ServiceDeps{})))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Start(context.Background())) {
		return
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	c := client(t, s)

	valid := ""
	for _, tc := range authtest.Cases(t) {
		if tc.Valid {
			valid = tc.Authorization
			break
		}
	}
	if !assert.NotEmpty(t, valid) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	authed := metadata.AppendToOutgoingContext(ctx, "authorization", valid)

	_, err = c.Greeting(ctx, &pb.GreetingRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	_, err = c.Greeting(authed, &pb.GreetingRequest{})
	assert.NoError(t, err)
	_, err = c.NewOrder(authed, &pb.RawOrderRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	calls := rec.all()
	if !assert.Len(t, calls, 3) {
		return
	}
	greeting := pb.Service_Greeting_FullMethodName
	assert.Equal(t, []observed{
		{path: greeting, code: "PermissionDenied"},
		{path: greeting, code: "OK"},
		{path: pb.Service_NewOrder_FullMethodName, code: "Unimplemented"},
	}, []observed{
		{path: calls[0].path, code: calls[0].code},
		{path: calls[1].path, code: calls[1].code},
		{path: calls[2].path, code: calls[2].code},
	})
}
