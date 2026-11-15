// Package httpserver is the HTTP entry point of the service (entry/http in system.html). It serves
// the grpc-gateway routes and sends each call to the gRPC server.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const readHeaderTimeout = 5 * time.Second

// Register adds the gateway routes of one gRPC service to mux. The generated
// Register<Service>Handler functions have this type.
type Register func(ctx context.Context, mux *runtime.ServeMux, conn *grpc.ClientConn) error

// Gateway serves the grpc-gateway routes on one address. It starts once and stops once.
type Gateway struct {
	addr     string
	target   func() string
	register []Register

	mu      sync.Mutex
	ln      net.Listener
	srv     *http.Server
	conn    *grpc.ClientConn
	done    chan error // gets the result of Serve
	stopped bool
}

// New creates the gateway. target returns the gRPC address to dial; it is read at Start, so it
// can return the address of a gRPC server that binds a free port.
func New(addr string, target func() string, register ...Register) (*Gateway, error) {
	if addr == "" {
		return nil, errors.New("http gateway: address is required")
	}
	if target == nil {
		return nil, errors.New("http gateway: gRPC target is required")
	}
	if len(register) == 0 {
		return nil, errors.New("http gateway: at least one service is required")
	}
	for i, r := range register {
		if r == nil {
			return nil, fmt.Errorf("http gateway: register function %d is nil", i)
		}
	}
	return &Gateway{addr: addr, target: target, register: register}, nil
}

// Start connects to the gRPC server, adds the routes, binds the address and serves in the background.
// If a step fails, Start closes what it opened.
func (g *Gateway) Start(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped {
		return errors.New("http gateway: already stopped")
	}
	if g.srv != nil {
		return errors.New("http gateway: already started")
	}
	if g.target == nil || len(g.register) == 0 {
		return errors.New("http gateway: not created with New")
	}
	target := g.target()
	if target == "" {
		return errors.New("http gateway: gRPC target is empty")
	}
	// The client span of each call (otelgrpc, the full gRPC method) sends traceparent to the gRPC
	// server, so the gateway call and the gRPC method are one trace (task 047, conventions § 17).
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	if err != nil {
		return fmt.Errorf("http gateway: grpc client for %s: %w", target, err)
	}
	if conn == nil {
		return fmt.Errorf("http gateway: grpc client for %s is nil", target)
	}
	mux := runtime.NewServeMux()
	if mux == nil {
		return errors.Join(errors.New("http gateway: runtime.NewServeMux returned nil"), conn.Close())
	}
	for _, r := range g.register {
		if err := r(ctx, mux, conn); err != nil {
			return errors.Join(fmt.Errorf("http gateway: register: %w", err), conn.Close())
		}
	}
	ln, err := net.Listen("tcp", g.addr)
	if err != nil {
		return errors.Join(fmt.Errorf("http gateway: listen %s: %w", g.addr, err), conn.Close())
	}
	if ln == nil {
		return errors.Join(fmt.Errorf("http gateway: listen %s returned no listener", g.addr), conn.Close())
	}
	srv := &http.Server{Handler: withTraceContext(mux), ReadHeaderTimeout: readHeaderTimeout}
	g.ln, g.srv, g.conn = ln, srv, conn
	g.done = make(chan error, 1)
	go func(done chan<- error) { done <- srv.Serve(ln) }(g.done)
	return nil
}

// Stop shuts the HTTP server down (it waits for running requests until ctx is done), then closes
// the gRPC connection. It is safe to call more than once and before Start.
func (g *Gateway) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	g.mu.Lock()
	if g.stopped || g.srv == nil {
		g.stopped = true
		g.mu.Unlock()
		return nil
	}
	g.stopped = true
	srv, conn, done := g.srv, g.conn, g.done
	g.mu.Unlock()

	var errs []error
	if err := srv.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("http gateway: shutdown: %w", err), srv.Close())
	}
	if err := <-done; err != nil && !errors.Is(err, http.ErrServerClosed) {
		errs = append(errs, fmt.Errorf("http gateway: %w", err))
	}
	if conn != nil {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("http gateway: close grpc client: %w", err))
		}
	}
	return errors.Join(errs...)
}

// Addr returns the bound address after Start, else the configured address.
func (g *Gateway) Addr() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ln == nil {
		return g.addr
	}
	if a := g.ln.Addr(); a != nil {
		return a.String()
	}
	return g.addr
}

// withTraceContext reads the W3C trace context of the request headers (traceparent) into the request
// context, so the client span of the gateway continues the trace of the app. It makes no span: an
// HTTP span has url.path, and DELETE /v1/mybill/{id} has the ID in the path (task 047 decision D4).
// With tracing off the global propagator is the no-op one and reads nothing.
func withTraceContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
