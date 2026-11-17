package provider

import (
	"context"
	"errors"
	"sync/atomic"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

// ErrNotSent is the error of a payment call that did not reach the provider for sure (task 058): the
// switch of the provider is off, it has no address, or the gRPC call failed with Unavailable before
// its request headers went to a connection. It always comes with ErrProviderDown, so the code that
// tests ErrProviderDown sees no change. Only this error lets the payment step go to the next
// provider (order.Payments, conventions § 13): a timeout, a cancel or any answer of the server is
// ErrProviderDown only (the status is not known). Test it with errors.Is.
var ErrNotSent = errors.New("provider: the request was not sent")

// DownError wraps err of a failed call with ErrProviderDown, and with ErrNotSent too when notSent
// (task 058). what names the adapter and the call, for example "jiring: pay".
func DownError(what string, err error, notSent bool) error {
	if notSent {
		return &downError{what: what, err: err, notSent: true}
	}
	return &downError{what: what, err: err}
}

type downError struct {
	what    string
	err     error
	notSent bool
}

func (e *downError) Error() string {
	text := e.what + ": " + ErrProviderDown.Error()
	if e.notSent {
		text += ": " + ErrNotSent.Error()
	}
	if e.err != nil {
		text += ": " + e.err.Error()
	}
	return text
}

func (e *downError) Unwrap() []error {
	errs := []error{ErrProviderDown}
	if e.notSent {
		errs = append(errs, ErrNotSent)
	}
	if e.err != nil {
		errs = append(errs, e.err)
	}
	return errs
}

// sentKey is the context key of the flag of one call (TrackSent).
type sentKey struct{}

// TrackSent returns a context for one gRPC call of a client that has SentHandler, and a function
// that reports whether the request headers of the call went to a connection (task 058). Before the
// call ends the answer can change from false to true.
func TrackSent(ctx context.Context) (context.Context, func() bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	flag := new(atomic.Bool)
	return context.WithValue(ctx, sentKey{}, flag), flag.Load
}

// GRPCNotSent reports whether err of a gRPC call made with the context of TrackSent did not reach
// the server for sure: the code Unavailable and no request headers on a connection. A nil err, a
// nil sent, another code (DeadlineExceeded too) or headers that went out: false.
func GRPCNotSent(err error, sent func() bool) bool {
	if err == nil || sent == nil {
		return false
	}
	return status.Code(err) == codes.Unavailable && !sent()
}

// SentHandler is a gRPC client stats handler (grpc.WithStatsHandler) that marks the flag of
// TrackSent at stats.OutHeader. grpc-go (v1.85.0, internal/transport http2_client.go NewStream)
// sends OutHeader only after the headers frame of the call is queued on a live connection; a call
// that fails before that (a refused connection, no address that resolves) has no OutHeader. A
// header that was queued but did not go out is counted as sent: the safe side (no fallback).
type SentHandler struct{}

var _ stats.Handler = SentHandler{}

// TagRPC returns ctx.
func (SentHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context { return ctx }

// HandleRPC marks the flag of the call at OutHeader.
func (SentHandler) HandleRPC(ctx context.Context, s stats.RPCStats) {
	if _, ok := s.(*stats.OutHeader); !ok || ctx == nil {
		return
	}
	if flag, ok := ctx.Value(sentKey{}).(*atomic.Bool); ok && flag != nil {
		flag.Store(true)
	}
}

// TagConn returns ctx.
func (SentHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context { return ctx }

// HandleConn does nothing.
func (SentHandler) HandleConn(context.Context, stats.ConnStats) {}
