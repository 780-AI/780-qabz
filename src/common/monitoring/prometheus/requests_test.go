package prometheus

import (
	"context"
	"io"
	"math"
	"net/http"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

// TestDisabledServer: with the switch off nothing is registered and nothing listens.
func TestDisabledServer(t *testing.T) {
	s := New("127.0.0.1:0", false)
	if !assert.NotNil(t, s) {
		return
	}
	assert.False(t, s.Enabled())
	assert.Nil(t, s.Registry())
	assert.NoError(t, s.Start(context.Background()))
	assert.Equal(t, "127.0.0.1:0", s.Addr(), "a disabled Server does not bind a port")
	assert.NoError(t, s.Stop(context.Background()))

	var none *Server
	assert.False(t, none.Enabled())
	assert.Nil(t, none.Registry())
}

func TestEnabledServer(t *testing.T) {
	s := New("127.0.0.1:0", true)
	if assert.NotNil(t, s) {
		assert.True(t, s.Enabled())
		assert.NotNil(t, s.Registry())
	}
}

// TestRequestMetricsDisabled: with metrics off (or no Server) no metric is created; Observe on the
// nil result does nothing.
func TestRequestMetricsDisabled(t *testing.T) {
	for name, s := range map[string]*Server{"disabled": New("127.0.0.1:0", false), "nil": nil} {
		t.Run(name, func(t *testing.T) {
			m, err := NewRequestMetrics(s)
			assert.NoError(t, err)
			assert.Nil(t, m)
			assert.NotPanics(t, func() { m.Observe("/a", "OK", time.Second) })
		})
	}
}

func TestRequestMetricsRegisterTwice(t *testing.T) {
	s := New("127.0.0.1:0", true)
	_, err := NewRequestMetrics(s)
	if !assert.NoError(t, err) {
		return
	}
	_, err = NewRequestMetrics(s)
	assert.ErrorContains(t, err, "requests: register metric")
}

// family returns the gathered metric family with the name, or stops the test.
func family(t *testing.T, s *Server, name string) *dto.MetricFamily {
	t.Helper()
	families, err := s.Registry().Gather()
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	for _, f := range families {
		if f != nil && f.GetName() == name {
			return f
		}
	}
	assert.Failf(t, "metric family not found", "%s", name)
	t.FailNow()
	return nil
}

// series returns the metric of f with the labels path and code, or nil.
func series(f *dto.MetricFamily, path, code string) *dto.Metric {
	for _, m := range f.GetMetric() {
		labels := map[string]string{}
		for _, l := range m.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		if len(labels) == 2 && labels["path"] == path && labels["code"] == code {
			return m
		}
	}
	return nil
}

// TestRequestMetricsRecord: the histogram records the duration in milliseconds by path and code, and
// its sample count (the _count series) adds 1 per call: it is the request counter.
func TestRequestMetricsRecord(t *testing.T) {
	s := New("127.0.0.1:0", true)
	m, err := NewRequestMetrics(s)
	if !assert.NoError(t, err) || !assert.NotNil(t, m) {
		return
	}
	for range 3 {
		m.Observe("/svc/A", "OK", 20*time.Millisecond)
	}
	m.Observe("/svc/C", "OK", 250*time.Microsecond)
	m.Observe("/svc/B", "PermissionDenied", 1500*time.Millisecond)

	duration := family(t, s, RequestDurationName)
	assert.Equal(t, dto.MetricType_HISTOGRAM, duration.GetType())
	assert.Len(t, duration.GetMetric(), 3, "one series per path and code")
	if a := series(duration, "/svc/A", "OK"); assert.NotNil(t, a) {
		assert.Equal(t, uint64(3), a.GetHistogram().GetSampleCount())
		assert.InDelta(t, 60.0, a.GetHistogram().GetSampleSum(), 1e-9, "3 x 20 ms")
	}
	if c := series(duration, "/svc/C", "OK"); assert.NotNil(t, c) {
		assert.InDelta(t, 0.25, c.GetHistogram().GetSampleSum(), 1e-9, "a duration under 1 ms is kept")
	}
	if b := series(duration, "/svc/B", "PermissionDenied"); assert.NotNil(t, b) {
		h := b.GetHistogram()
		assert.Equal(t, uint64(1), h.GetSampleCount())
		assert.InDelta(t, 1500.0, h.GetSampleSum(), 1e-9, "the duration is in milliseconds")
		for _, bucket := range h.GetBucket() {
			want := uint64(0)
			if bucket.GetUpperBound() >= 1500 {
				want = 1
			}
			assert.Equal(t, want, bucket.GetCumulativeCount(), "bucket le=%v", bucket.GetUpperBound())
		}
		if assert.NotEmpty(t, h.GetBucket()) {
			assert.InDelta(t, 5.0, h.GetBucket()[0].GetUpperBound(), 1e-12)
			assert.InDelta(t, 10000.0, h.GetBucket()[len(h.GetBucket())-1].GetUpperBound(), 1e-12)
			assert.False(t, math.IsInf(h.GetBucket()[len(h.GetBucket())-1].GetUpperBound(), 1))
		}
	}
}

// TestRequestMetricsOnEndpoint: the recorded series are on /metrics.
func TestRequestMetricsOnEndpoint(t *testing.T) {
	s := New("127.0.0.1:0", true)
	m, err := NewRequestMetrics(s)
	if !assert.NoError(t, err) || !assert.NotNil(t, m) || !assert.NoError(t, s.Start(context.Background())) {
		return
	}
	defer func() { assert.NoError(t, s.Stop(context.Background())) }()
	m.Observe("/svc/A", "OK", time.Millisecond)
	resp, err := http.Get("http://" + s.Addr() + "/metrics")
	if !assert.NoError(t, err) || !assert.NotNil(t, resp) {
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Contains(t, string(body), `qabz_request_duration_ms_count{code="OK",path="/svc/A"} 1`)
}
