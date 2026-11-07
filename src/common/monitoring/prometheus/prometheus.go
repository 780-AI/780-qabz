// Package prometheus serves the metrics endpoint. It holds the Prometheus switch
// (metrics.prometheus.enabled): a disabled Server registers nothing, records nothing and does not listen.
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

// Server serves /metrics on its own registry. A disabled Server has no registry and no endpoint.
type Server struct {
	addr    string
	enabled bool
	reg     *prometheus.Registry // nil when disabled
	srv     *http.Server
	ln      net.Listener
}

// New creates the server with the Go runtime and process collectors. With enabled false it
// returns a disabled Server: Registry is nil, and Start and Stop do nothing.
func New(addr string, enabled bool) *Server {
	if !enabled {
		return &Server{addr: addr}
	}
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) { serve(reg, w) })
	return &Server{addr: addr, enabled: true, reg: reg, srv: &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}}
}

// Enabled reports whether metrics are on. A nil Server is disabled.
func (s *Server) Enabled() bool { return s != nil && s.enabled }

// Registry lets packages register their own collectors. It is nil when the Server is disabled (or nil):
// a package then creates no metric and records nothing.
func (s *Server) Registry() *prometheus.Registry {
	if s == nil {
		return nil
	}
	return s.reg
}

// Addr returns the bound address after Start.
func (s *Server) Addr() string {
	if s.ln == nil {
		return s.addr
	}
	return s.ln.Addr().String()
}

// Start binds the port and serves in the background. A disabled Server does not listen.
func (s *Server) Start(context.Context) error {
	if !s.Enabled() || s.srv == nil {
		return nil
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.ln = ln
	go func() { _ = s.srv.Serve(ln) }() // returns http.ErrServerClosed after Stop
	return nil
}

// Stop shuts the server down. A disabled Server has nothing to stop.
func (s *Server) Stop(ctx context.Context) error {
	if !s.Enabled() || s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

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
