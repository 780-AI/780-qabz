// Package prometheus serves the metrics endpoint.
package prometheus

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/common/expfmt"
)

// Server serves /metrics on its own registry.
type Server struct {
	addr string
	reg  *prometheus.Registry
	srv  *http.Server
	ln   net.Listener
}

// New creates the server with the Go runtime and process collectors.
func New(addr string) *Server {
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) { serve(reg, w) })
	return &Server{addr: addr, reg: reg, srv: &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}}
}

// Registry lets packages register their own collectors.
func (s *Server) Registry() *prometheus.Registry { return s.reg }

// Addr returns the bound address after Start.
func (s *Server) Addr() string {
	if s.ln == nil {
		return s.addr
	}
	return s.ln.Addr().String()
}

// Start binds the port and serves in the background.
func (s *Server) Start(context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	go func() { _ = s.srv.Serve(ln) }() // returns http.ErrServerClosed after Stop
	return nil
}

// Stop shuts the server down.
func (s *Server) Stop(ctx context.Context) error { return s.srv.Shutdown(ctx) }

// serve writes the registry in the Prometheus text format. It avoids promhttp, which pulls in a compression dependency.
func serve(reg *prometheus.Registry, w http.ResponseWriter) {
	families, err := reg.Gather()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", string(expfmt.NewFormat(expfmt.TypeTextPlain)))
	enc := expfmt.NewEncoder(w, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, f := range families {
		if err := enc.Encode(f); err != nil {
			return
		}
	}
}
