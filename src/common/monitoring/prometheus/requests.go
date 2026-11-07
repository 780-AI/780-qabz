package prometheus

import (
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// RequestDurationName is the name of the request histogram (Implementation_Plan Step 6, task 014).
// Its _count series is the request counter: it adds 1 for each incoming request, by path and code,
// so there is no separate counter metric.
const RequestDurationName = "qabz_request_duration_ms"

// RequestDurationBuckets are the histogram buckets in milliseconds (5 ms to 10 s). Durations are
// recorded in milliseconds (conventions § 9).
var RequestDurationBuckets = []float64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// RequestMetrics record every incoming request and its duration, by path and code.
// A nil *RequestMetrics records nothing.
type RequestMetrics struct {
	duration *prometheus.HistogramVec
}

// NewRequestMetrics registers qabz_request_duration_ms{path,code} (histogram, RequestDurationBuckets)
// on s. With a disabled (or nil) s it registers nothing and returns nil, nil.
func NewRequestMetrics(s *Server) (*RequestMetrics, error) {
	reg := s.Registry()
	if reg == nil {
		return nil, nil
	}
	m := &RequestMetrics{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    RequestDurationName,
			Help:    "Duration of incoming requests in milliseconds, by path (the gRPC method) and code (the gRPC code). The _count series counts the requests.",
			Buckets: RequestDurationBuckets,
		}, []string{"path", "code"}),
	}
	if err := reg.Register(m.duration); err != nil {
		return nil, fmt.Errorf("requests: register metric: %w", err)
	}
	return m, nil
}

// Observe records d in milliseconds in the histogram with the labels path and code; this also adds 1
// to its _count series. The value is a float, so a duration under 1 ms is kept (for example 0.25).
func (m *RequestMetrics) Observe(path, code string, d time.Duration) {
	if m == nil || m.duration == nil {
		return
	}
	m.duration.WithLabelValues(path, code).Observe(float64(d) / float64(time.Millisecond))
}
