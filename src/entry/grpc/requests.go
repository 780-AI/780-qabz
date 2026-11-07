package grpcserver

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// UnknownPath is the path label of a call without method information. The gRPC server calls the
// interceptors only for registered methods, so the label has a fixed set of values.
const UnknownPath = "unknown"

// RequestObserver records one finished call: path is the full gRPC method (for example
// "/haf80.api.bill.Service/Get"), code is the gRPC code name (for example "OK"), d is the duration.
type RequestObserver func(path, code string, d time.Duration)

// Requests measures every gRPC call. The HTTP gateway sends each call to the gRPC server, so the
// gateway routes are measured too. Install its options before the auth check, so a rejected call
// (PermissionDenied, HTTP 403) is also measured.
type Requests struct {
	observe RequestObserver
}

// NewRequests returns the interceptors. observe is required.
func NewRequests(observe RequestObserver) (*Requests, error) {
	if observe == nil {
		return nil, errors.New("requests: an observer is required")
	}
	return &Requests{observe: observe}, nil
}

// Unary is the interceptor for unary calls.
func (r *Requests) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		path := UnknownPath
		if info != nil && info.FullMethod != "" {
			path = info.FullMethod
		}
		start := time.Now()
		res, err := handler(ctx, req)
		r.record(path, err, time.Since(start))
		return res, err
	}
}

// Stream is the interceptor for streaming calls. The duration covers the whole stream.
func (r *Requests) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		path := UnknownPath
		if info != nil && info.FullMethod != "" {
			path = info.FullMethod
		}
		start := time.Now()
		err := handler(srv, ss)
		r.record(path, err, time.Since(start))
		return err
	}
}

// ServerOptions returns the gRPC server options that install the interceptors.
func (r *Requests) ServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{grpc.ChainUnaryInterceptor(r.Unary()), grpc.ChainStreamInterceptor(r.Stream())}
}

// record sends one call to the observer. A nil error is "OK"; an error without a gRPC status is "Unknown".
func (r *Requests) record(path string, err error, d time.Duration) {
	if r == nil || r.observe == nil {
		return
	}
	r.observe(path, status.Code(err).String(), d)
}
