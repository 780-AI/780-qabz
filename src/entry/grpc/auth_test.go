package grpcserver

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	pb "qabz/api/pb/qabz"
	"qabz/common/auth"
	"qabz/common/auth/authtest"
	"qabz/config"
)

// configEntries are the config entries of the auth tests. Only debug.auth.disable: true turns the
// check off; a missing section or key keeps it on (the bool zero value).
var configEntries = []struct {
	name     string
	yml      string
	disabled bool
}{
	{name: "no debug section", yml: `database: {postgres: {password: x}}`},
	{name: "debug without auth", yml: `{database: {postgres: {password: x}}, debug: {}}`},
	{name: "disable false", yml: `{database: {postgres: {password: x}}, debug: {auth: {disable: false}}}`},
	{name: "disable true", yml: `{database: {postgres: {password: x}}, debug: {auth: {disable: true}}}`, disabled: true},
}

// claimsService answers Greeting and records the claims the handler saw.
type claimsService struct {
	pb.UnimplementedServiceServer
	mu     sync.Mutex
	claims *auth.Claims
}

func (s *claimsService) Greeting(ctx context.Context, _ *pb.GreetingRequest) (*pb.GreetingResponse, error) {
	c, ok := auth.ClaimsFrom(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims = nil
	if ok {
		s.claims = &c
	}
	return &pb.GreetingResponse{Message: GreetingText}, nil
}

func (s *claimsService) seen() *auth.Claims {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claims
}

// startAuthServer starts a gRPC server with the auth check built from cfg, as NewDefaultBuilder
// does, and returns a client and the reasons the check reported.
func startAuthServer(t *testing.T, cfg config.Config, svc pb.ServiceServer) (pb.ServiceClient, *[]string) {
	t.Helper()
	var mu sync.Mutex
	reasons := []string{}
	opts := AuthOptions{Disabled: cfg.DebugAuthDisable, OnReject: func(r string) { mu.Lock(); reasons = append(reasons, r); mu.Unlock() }}
	if !cfg.DebugAuthDisable {
		opts.Verifier = authtest.Verifier(t)
	}
	a, err := NewAuth(opts)
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		t.FailNow()
	}
	s, err := NewWithOptions("127.0.0.1:0", a.ServerOptions(), RegisterService(svc))
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Start(context.Background())) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	conn, err := grpc.NewClient(s.DialAddr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = conn.Close() })
	return pb.NewServiceClient(conn), &reasons
}

// TestAuthConfigAndTokenTable runs every config entry with every JWT input of the shared table.
func TestAuthConfigAndTokenTable(t *testing.T) {
	cases := authtest.Cases(t)
	for _, entry := range configEntries {
		t.Run(entry.name, func(t *testing.T) {
			cfg, err := config.Parse([]byte(entry.yml))
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, entry.disabled, cfg.DebugAuthDisable)
			svc := &claimsService{}
			client, reasons := startAuthServer(t, cfg, svc)

			for _, tc := range cases {
				t.Run(tc.Name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					if !tc.NoHeader {
						ctx = metadata.AppendToOutgoingContext(ctx, "authorization", tc.Authorization)
					}
					before := len(*reasons)
					resp, err := client.Greeting(ctx, &pb.GreetingRequest{})

					if entry.disabled || tc.Valid {
						if assert.NoError(t, err) && assert.NotNil(t, resp) {
							assert.Equal(t, GreetingText, resp.GetMessage())
						}
						seen := svc.seen()
						if entry.disabled {
							assert.Nil(t, seen, "no claims while the check is off")
						} else if assert.NotNil(t, seen) {
							assert.Equal(t, authtest.UserID, seen.UserID)
						}
						return
					}
					assert.Equal(t, codes.Unauthenticated, status.Code(err))
					assert.Equal(t, "unauthenticated", status.Convert(err).Message(), "the reason is not sent to the client")
					if assert.Len(t, *reasons, before+1) {
						assert.Equal(t, tc.Reason, (*reasons)[before])
					}
				})
			}
		})
	}
}

func TestAuthRejectsTwoAuthorizationValues(t *testing.T) {
	cfg, err := config.Parse([]byte(configEntries[0].yml))
	if !assert.NoError(t, err) {
		return
	}
	client, reasons := startAuthServer(t, cfg, &claimsService{})
	good := "Bearer " + authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	ctx := metadata.AppendToOutgoingContext(context.Background(), "authorization", good, "authorization", good)
	_, err = client.Greeting(ctx, &pb.GreetingRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Equal(t, []string{"malformed"}, *reasons)
}

func TestNewAuthNeedsVerifierWhenOn(t *testing.T) {
	_, err := NewAuth(AuthOptions{})
	assert.Error(t, err)
	a, err := NewAuth(AuthOptions{Disabled: true})
	assert.NoError(t, err)
	assert.NotNil(t, a)
}

// fakeVerifier returns err for every token.
type fakeVerifier struct{ err error }

func (f fakeVerifier) VerifyAuthorization(string) (auth.Claims, error) { return auth.Claims{}, f.err }

func TestAuthCheckEdgeCases(t *testing.T) {
	var nilAuth *Auth
	_, err := nilAuth.check(context.Background())
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	// A Verifier error that is not an *auth.Error is reported as "error".
	var got []string
	a := &Auth{opts: AuthOptions{Verifier: fakeVerifier{err: context.Canceled}, OnReject: func(r string) { got = append(got, r) }}}
	_, err = a.check(context.Background())
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Equal(t, []string{"error"}, got)

	// An Auth built without NewAuth and with no Verifier rejects every call.
	got = nil
	b := &Auth{opts: AuthOptions{OnReject: func(r string) { got = append(got, r) }}}
	_, err = b.check(context.Background())
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	assert.Equal(t, []string{"no_verifier"}, got)

	// Without OnReject a rejection still works.
	c := &Auth{opts: AuthOptions{Verifier: fakeVerifier{err: context.Canceled}}}
	_, err = c.check(context.Background())
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// fakeStream is a ServerStream with a fixed context.
type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f fakeStream) Context() context.Context { return f.ctx }

func TestAuthStreamInterceptor(t *testing.T) {
	a, err := NewAuth(AuthOptions{Verifier: authtest.Verifier(t)})
	if !assert.NoError(t, err) || !assert.NotNil(t, a) {
		return
	}
	intercept := a.Stream()

	var seen *auth.Claims
	handler := func(_ any, ss grpc.ServerStream) error {
		if c, ok := auth.ClaimsFrom(ss.Context()); ok {
			seen = &c
		}
		return nil
	}
	good := "Bearer " + authtest.Sign(t, authtest.Secret, authtest.Header("HS256"), authtest.ValidClaims())
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", good))
	assert.NoError(t, intercept(nil, fakeStream{ctx: ctx}, &grpc.StreamServerInfo{}, handler))
	if assert.NotNil(t, seen) {
		assert.Equal(t, authtest.UserID, seen.UserID)
	}

	err = intercept(nil, fakeStream{ctx: context.Background()}, &grpc.StreamServerInfo{}, handler)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
	err = intercept(nil, nil, &grpc.StreamServerInfo{}, handler)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestNewWithOptionsRejectsNilOption(t *testing.T) {
	_, err := NewWithOptions("127.0.0.1:0", []grpc.ServerOption{nil}, RegisterService(NewService()))
	assert.Error(t, err)
}
