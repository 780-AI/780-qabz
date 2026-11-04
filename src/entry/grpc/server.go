// Package grpcserver is the gRPC entry point of the service (entry/grpc in system.html).
package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"google.golang.org/grpc"
)

// Register adds one or more services to the gRPC server.
type Register func(grpc.ServiceRegistrar)

// Server serves gRPC on one address. It starts once and stops once.
type Server struct {
	addr string
	srv  *grpc.Server

	mu      sync.Mutex
	ln      net.Listener
	done    chan error // gets the result of Serve
	stopped bool
}

// New creates the server and registers the services. A server needs at least one Register.
func New(addr string, register ...Register) (*Server, error) {
	if addr == "" {
		return nil, errors.New("grpc server: address is required")
	}
	if len(register) == 0 {
		return nil, errors.New("grpc server: at least one service is required")
	}
	for i, r := range register {
		if r == nil {
			return nil, fmt.Errorf("grpc server: register function %d is nil", i)
		}
	}
	srv := grpc.NewServer()
	if srv == nil {
		return nil, errors.New("grpc server: grpc.NewServer returned nil")
	}
	for _, r := range register {
		r(srv)
	}
	return &Server{addr: addr, srv: srv}, nil
}

// Start binds the address and serves in the background.
func (s *Server) Start(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv == nil {
		return errors.New("grpc server: not created with New")
	}
	if s.stopped {
		return errors.New("grpc server: already stopped")
	}
	if s.ln != nil {
		return errors.New("grpc server: already started")
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("grpc server: listen %s: %w", s.addr, err)
	}
	if ln == nil {
		return fmt.Errorf("grpc server: listen %s returned no listener", s.addr)
	}
	s.ln = ln
	s.done = make(chan error, 1)
	go func(srv *grpc.Server, done chan<- error) { done <- srv.Serve(ln) }(s.srv, s.done)
	return nil
}

// Stop ends the server. It waits for running calls until ctx is done, then closes them.
// It is safe to call more than once and before Start.
func (s *Server) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	s.mu.Lock()
	if s.stopped || s.ln == nil {
		s.stopped = true
		s.mu.Unlock()
		return nil
	}
	s.stopped = true
	srv, done := s.srv, s.done
	s.mu.Unlock()
	if srv == nil || done == nil { // only a Server that New created and Start started gets here
		return nil
	}

	graceful := make(chan struct{})
	go func() { srv.GracefulStop(); close(graceful) }()
	select {
	case <-graceful:
	case <-ctx.Done():
		srv.Stop()
		<-graceful
	}
	if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		return fmt.Errorf("grpc server: %w", err)
	}
	return nil
}

// Addr returns the bound address after Start, else the configured address.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln == nil {
		return s.addr
	}
	return boundAddr(s.ln, s.addr)
}

// DialAddr returns an address that a client on the same host can dial. A listen address with no
// host or an unspecified IP (":8082", "0.0.0.0:8082", "[::]:8082") becomes 127.0.0.1. A wildcard
// listen in Go is dual-stack, so 127.0.0.1 works also where IPv6 is off (::1 does not).
func (s *Server) DialAddr() string {
	return dialAddr(s.Addr())
}

func dialAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// boundAddr returns the address of ln, or fallback when ln has no address.
func boundAddr(ln net.Listener, fallback string) string {
	if ln == nil {
		return fallback
	}
	a := ln.Addr()
	if a == nil {
		return fallback
	}
	return a.String()
}
