package nats

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"qabz/common/monitoring/jaeger/jaegertest"
)

// liveURL is the NATS server of the live tests: QABZ_TEST_NATS_URL, else the compose service nats
// (profile nats, docker-compose.yml).
func liveURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("QABZ_TEST_NATS_URL")
	if u == "" {
		u = "nats://127.0.0.1:4222"
	}
	host := strings.TrimPrefix(u, "nats://")
	c, err := net.DialTimeout("tcp", host, 500*time.Millisecond)
	if err != nil {
		t.Skipf("no NATS server at %s (COMPOSE_PROFILES=nats make compose up): %v", u, err)
	}
	_ = c.Close()
	return u
}

// recorder records the OnRequest and OnConnected calls and the log lines.
type recorder struct {
	mu        sync.Mutex
	requests  []string // subject/result
	connected []bool
	lines     []string
}

func (r *recorder) onRequest(subject, result string, d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, subject+"/"+result)
}

func (r *recorder) onConnected(up bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connected = append(r.connected, up)
}

func (r *recorder) line(level, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, level+" "+msg)
}

func (r *recorder) Info(msg string, _ ...any)  { r.line("info", msg) }
func (r *recorder) Warn(msg string, _ ...any)  { r.line("warn", msg) }
func (r *recorder) Error(msg string, _ ...any) { r.line("error", msg) }

func (r *recorder) snapshot() ([]string, []bool, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...), append([]bool(nil), r.connected...), append([]string(nil), r.lines...)
}

func newClient(t *testing.T, url string, r *recorder) *Client {
	t.Helper()
	c, err := New(Options{Name: "qabz-test", Servers: []string{url}, AllowReconnect: true, MaxReconnect: 5, ReconnectWait: 50 * time.Millisecond,
		Timeout: time.Second, Logger: r, OnRequest: r.onRequest, OnConnected: r.onConnected})
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return c
}

// responder subscribes to subject on its own connection and answers with answer (after wait).
func responder(t *testing.T, url, subject string, wait time.Duration, answer func(data []byte) []byte) {
	t.Helper()
	nc, err := natsgo.Connect(url)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	t.Cleanup(nc.Close)
	_, err = nc.QueueSubscribe(subject, subject, func(m *natsgo.Msg) {
		time.Sleep(wait)
		_ = m.Respond(answer(m.Data))
	})
	if !assert.NoError(t, err) || !assert.NoError(t, nc.Flush()) {
		t.FailNow()
	}
}

func TestNewNeedsServers(t *testing.T) {
	_, err := New(Options{})
	assert.ErrorContains(t, err, "nats.endpoints")
}

func TestResultOf(t *testing.T) {
	for err, want := range map[error]string{
		nil:                             ResultOK,
		context.DeadlineExceeded:        ResultTimeout,
		context.Canceled:                ResultTimeout,
		natsgo.ErrTimeout:               ResultTimeout,
		natsgo.ErrNoResponders:          ResultNoResponders,
		natsgo.ErrConnectionClosed:      ResultNoConnection,
		natsgo.ErrConnectionDraining:    ResultNoConnection,
		errors.New("nats: bad subject"): ResultError,
	} {
		assert.Equal(t, want, resultOf(err), "%v", err)
	}
}

func TestNilClient(t *testing.T) {
	var c *Client
	_, err := c.Request(context.Background(), "s", nil)
	assert.Error(t, err)
	assert.NoError(t, c.Close(context.Background()))
}

// TestNewServerDown (operator decision Q2 (a) of task 056): with no server, New still returns the
// client, writes one error line and reports "down"; a request ends with its ctx (timeout).
func TestNewServerDown(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if !assert.NoError(t, err) {
		return
	}
	addr := lis.Addr().String()
	_ = lis.Close() // nothing listens on addr now
	r := &recorder{}
	c := newClient(t, "nats://"+addr, r)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = c.Request(ctx, "irancell-bill", []byte("{}"))
	assert.Error(t, err)
	reqs, conn, lines := r.snapshot()
	assert.Equal(t, []string{"irancell-bill/timeout"}, reqs)
	if assert.NotEmpty(t, conn) {
		assert.False(t, conn[0])
	}
	assert.Contains(t, lines, "error nats: connect failed, trying again in the background")
}

// TestLiveRequest: a request gets the answer of the responder, as bytes; one ok result; a client
// span nats.Request under the span of ctx with the subject and the result; no trace header.
func TestLiveRequest(t *testing.T) {
	url := liveURL(t)
	sr := jaegertest.Install(t)
	r := &recorder{}
	c := newClient(t, url, r)
	var gotHeader natsgo.Header
	var mu sync.Mutex
	nc, err := natsgo.Connect(url)
	if !assert.NoError(t, err) {
		return
	}
	t.Cleanup(nc.Close)
	_, err = nc.Subscribe("qabz-test-echo", func(m *natsgo.Msg) {
		mu.Lock()
		gotHeader = m.Header
		mu.Unlock()
		_ = m.Respond(append([]byte("echo:"), m.Data...))
	})
	if !assert.NoError(t, err) || !assert.NoError(t, nc.Flush()) {
		return
	}
	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	got, err := c.Request(rctx, "qabz-test-echo", []byte(`{"inquiry_id":"989350000391"}`))
	parent.End()
	if assert.NoError(t, err) {
		assert.Equal(t, `echo:{"inquiry_id":"989350000391"}`, string(got))
	}
	mu.Lock()
	assert.Empty(t, gotHeader, "no trace header in the message")
	mu.Unlock()
	reqs, conn, lines := r.snapshot()
	assert.Equal(t, []string{"qabz-test-echo/ok"}, reqs)
	assert.Contains(t, conn, true)
	assert.Contains(t, lines, "info nats: connected")
	if spans := jaegertest.ByName(sr, "nats.Request"); assert.Len(t, spans, 1) {
		s := spans[0]
		assert.Equal(t, trace.SpanKindClient, s.SpanKind())
		assert.Equal(t, parent.SpanContext().SpanID(), s.Parent().SpanID())
		assert.ElementsMatch(t, []attribute.KeyValue{attribute.String("messaging.system", "nats"),
			attribute.String("messaging.destination.name", "qabz-test-echo"), attribute.String("result", ResultOK)}, s.Attributes())
		assert.Equal(t, codes.Unset, s.Status().Code)
	}
	jaegertest.AssertClean(t, sr.Ended(), "989350000391")
}

// TestLiveFailures: no subscriber is no_responders; a slow responder is timeout; a closed client is
// no_connection. Each failed span has the error status.
func TestLiveFailures(t *testing.T) {
	url := liveURL(t)
	sr := jaegertest.Install(t)
	r := &recorder{}
	c := newClient(t, url, r)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := c.Request(ctx, "qabz-test-nobody", []byte("x"))
	assert.ErrorIs(t, err, natsgo.ErrNoResponders)

	responder(t, url, "qabz-test-slow", 500*time.Millisecond, func(d []byte) []byte { return d })
	short, scancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer scancel()
	_, err = c.Request(short, "qabz-test-slow", []byte("x"))
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	assert.NoError(t, c.Close(context.Background()))
	_, err = c.Request(ctx, "qabz-test-slow", []byte("x"))
	assert.Error(t, err)

	reqs, _, _ := r.snapshot()
	assert.Equal(t, []string{"qabz-test-nobody/no_responders", "qabz-test-slow/timeout", "qabz-test-slow/no_connection"}, reqs)
	for _, s := range jaegertest.ByName(sr, "nats.Request") {
		assert.Equal(t, codes.Error, s.Status().Code)
	}
}
