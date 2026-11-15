package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"qabz/common/monitoring/jaeger/jaegertest"
)

// headerServer records the traceparent header of each request.
type headerServer struct {
	mu   sync.Mutex
	seen []string
}

func (h *headerServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.seen = append(h.seen, r.Header.Get("traceparent"))
	h.mu.Unlock()
	if strings.HasSuffix(r.URL.Path, "/down") {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}

func (h *headerServer) headers() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.seen...)
}

func post(t *testing.T, c *http.Client, ctx context.Context, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(`{"phone":"989121234567"}`))
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	res, err := c.Do(req)
	if !assert.NoError(t, err) || !assert.NotNil(t, res) {
		t.FailNow()
	}
	_ = res.Body.Close()
	return res.StatusCode
}

// TestNewHTTPClientTraces (task 047 D6): each request is a client span "HTTP POST", a child of the
// span in ctx, and sends traceparent of that span; a 5xx answer has the error status.
func TestNewHTTPClientTraces(t *testing.T) {
	sr := jaegertest.Install(t)
	h := &headerServer{}
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := NewHTTPClient(time.Second)
	assert.Equal(t, time.Second, c.Timeout)

	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	assert.Equal(t, http.StatusOK, post(t, c, ctx, srv.URL+"/inquiry"))
	assert.Equal(t, http.StatusServiceUnavailable, post(t, c, ctx, srv.URL+"/down"))
	parent.End()

	spans := jaegertest.ByName(sr, "HTTP POST")
	if !assert.Len(t, spans, 2) {
		return
	}
	for _, s := range spans {
		assert.Equal(t, trace.SpanKindClient, s.SpanKind())
		assert.Equal(t, parent.SpanContext().SpanID(), s.Parent().SpanID())
	}
	assert.Equal(t, codes.Unset, spans[0].Status().Code)
	assert.Equal(t, codes.Error, spans[1].Status().Code)
	seen := h.headers()
	if assert.Len(t, seen, 2) {
		assert.Equal(t, "00-"+spans[0].SpanContext().TraceID().String()+"-"+spans[0].SpanContext().SpanID().String()+"-01", seen[0])
	}
	jaegertest.AssertClean(t, sr.Ended(), "989121234567")
}

// TestNewHTTPClientOff (task 047 D2): with tracing off (no jaeger.New, the default globals) no
// traceparent is sent. The OpenTelemetry globals delegate to the first provider a process sets, so
// the check runs in a child process that sets none.
func TestNewHTTPClientOff(t *testing.T) {
	if os.Getenv("QABZ_TRACING_OFF_CHILD") == "1" {
		h := &headerServer{}
		srv := httptest.NewServer(h)
		defer srv.Close()
		ctx, span := otel.Tracer("test").Start(context.Background(), "parent")
		assert.Equal(t, http.StatusOK, post(t, NewHTTPClient(time.Second), ctx, srv.URL+"/inquiry"))
		span.End()
		assert.False(t, span.SpanContext().IsValid(), "the no-op provider makes no span")
		assert.Equal(t, []string{""}, h.headers(), "no traceparent with tracing off")
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestNewHTTPClientOff$", "-test.count=1")
	cmd.Env = append(os.Environ(), "QABZ_TRACING_OFF_CHILD=1")
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, string(out))
	assert.Contains(t, string(out), "PASS")
}
