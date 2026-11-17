package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

// TestDownError (task 058): ErrProviderDown always, ErrNotSent only when not sent, the cause kept.
func TestDownError(t *testing.T) {
	cause := errors.New("boom")
	err := DownError("x: pay", cause, false)
	assert.ErrorIs(t, err, ErrProviderDown)
	assert.ErrorIs(t, err, cause)
	assert.NotErrorIs(t, err, ErrNotSent)
	assert.Equal(t, "x: pay: provider: inquiry is down: boom", err.Error())

	err = DownError("x: pay", cause, true)
	assert.ErrorIs(t, err, ErrProviderDown)
	assert.ErrorIs(t, err, ErrNotSent)
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, "x: pay: provider: inquiry is down: provider: the request was not sent: boom", err.Error())

	err = DownError("x: pay", nil, true)
	assert.ErrorIs(t, err, ErrNotSent)
	assert.Equal(t, "x: pay: provider: inquiry is down: provider: the request was not sent", err.Error())

	grpcErr := status.Error(codes.Unavailable, "down")
	assert.Equal(t, codes.Unavailable, status.Code(DownError("x", grpcErr, true)), "the gRPC code stays readable")
}

// TestTrackSentAndHandler (task 058): only OutHeader marks the flag of its own call.
func TestTrackSentAndHandler(t *testing.T) {
	var h SentHandler
	ctx, sent := TrackSent(context.Background())
	other, otherSent := TrackSent(context.Background())
	assert.False(t, sent())
	assert.Equal(t, ctx, h.TagRPC(ctx, nil))
	assert.Equal(t, ctx, h.TagConn(ctx, nil))
	h.HandleConn(ctx, nil)
	h.HandleRPC(ctx, &stats.Begin{})
	assert.False(t, sent(), "Begin is not a sent header")
	h.HandleRPC(ctx, &stats.OutHeader{Client: true})
	assert.True(t, sent())
	assert.False(t, otherSent(), "another call keeps its own flag")
	h.HandleRPC(context.Background(), &stats.OutHeader{}) // no flag in the context: no panic
	var nilCtx context.Context
	h.HandleRPC(nilCtx, &stats.OutHeader{})
	_ = other

	nctx, nsent := TrackSent(nilCtx)
	assert.NotNil(t, nctx)
	assert.False(t, nsent())
}

// TestGRPCNotSent (task 058): only Unavailable with no header sent.
func TestGRPCNotSent(t *testing.T) {
	no := func() bool { return false }
	yes := func() bool { return true }
	unavailable := status.Error(codes.Unavailable, "refused")
	assert.True(t, GRPCNotSent(unavailable, no))
	assert.False(t, GRPCNotSent(unavailable, yes), "the headers went out")
	assert.False(t, GRPCNotSent(status.Error(codes.DeadlineExceeded, "t"), no), "a timeout is never not sent (operator rule)")
	assert.False(t, GRPCNotSent(errors.New("plain"), no))
	assert.False(t, GRPCNotSent(nil, no))
	assert.False(t, GRPCNotSent(unavailable, nil))
}
