package grpcserver

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"qabz/common/auth"
)

// TokenVerifier checks an Authorization value ("Bearer <token>"). *auth.Verifier implements it.
type TokenVerifier interface {
	VerifyAuthorization(authorization string) (auth.Claims, error)
}

// AuthOptions configure the auth check.
type AuthOptions struct {
	// Verifier checks the token. It is required unless Disabled is true.
	Verifier TokenVerifier
	// Disabled turns the check off (debug.auth.disable, development only).
	Disabled bool
	// OnReject, when set, is called with the reason of each rejected call (for a metric).
	OnReject func(reason string)
}

// Auth is the JWT check for every gRPC call. The HTTP gateway sends each call to the gRPC server,
// so the gateway routes get the same check.
type Auth struct {
	opts AuthOptions
}

// errUnauthenticated is the one answer for every rejected call: the reason is not sent to the client.
var errUnauthenticated = status.Error(codes.Unauthenticated, "unauthenticated")

// NewAuth returns the auth check. Without a Verifier it fails unless the check is disabled, so a
// missing key can never mean "no check".
func NewAuth(opts AuthOptions) (*Auth, error) {
	if !opts.Disabled && opts.Verifier == nil {
		return nil, errors.New("auth: a verifier is required when the check is on")
	}
	return &Auth{opts: opts}, nil
}

// Unary is the interceptor for unary calls.
func (a *Auth) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx, err := a.check(ctx)
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}
}

// Stream is the interceptor for streaming calls.
func (a *Auth) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if ss == nil {
			return errUnauthenticated
		}
		ctx, err := a.check(ss.Context())
		if err != nil {
			return err
		}
		return handler(srv, &authStream{ServerStream: ss, ctx: ctx})
	}
}

// ServerOptions returns the gRPC server options that install the check.
func (a *Auth) ServerOptions() []grpc.ServerOption {
	return []grpc.ServerOption{grpc.ChainUnaryInterceptor(a.Unary()), grpc.ChainStreamInterceptor(a.Stream())}
}

// check returns ctx with the claims, or the Unauthenticated error.
func (a *Auth) check(ctx context.Context) (context.Context, error) {
	if a == nil {
		return ctx, errUnauthenticated
	}
	if a.opts.Disabled {
		return ctx, nil
	}
	if a.opts.Verifier == nil {
		return ctx, a.rejected("no_verifier")
	}
	md, _ := metadata.FromIncomingContext(ctx) // a nil md has no values
	values := md.Get("authorization")
	if len(values) > 1 {
		return ctx, a.rejected(string(auth.ReasonMalformed))
	}
	value := ""
	if len(values) == 1 {
		value = values[0]
	}
	claims, err := a.opts.Verifier.VerifyAuthorization(value)
	if err != nil {
		reason := string(auth.ReasonOf(err))
		if reason == "" {
			reason = "error"
		}
		return ctx, a.rejected(reason)
	}
	return auth.WithClaims(ctx, claims), nil
}

func (a *Auth) rejected(reason string) error {
	if a.opts.OnReject != nil {
		a.opts.OnReject(reason)
	}
	return errUnauthenticated
}

// authStream carries the context with the claims to a stream handler.
type authStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *authStream) Context() context.Context { return s.ctx }
