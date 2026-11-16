// Package nats is the NATS client of Qabz (task 056): request/reply over one connection, for the
// Irancell bill payment (bill: client/broker nats_dto.go and pkg/orderbus nats.go, a JSON
// EncodedConn of nats.go v1.11.0). Qabz marshals and parses the bytes itself, so the client sends and
// returns raw bytes (nats.go has deprecated EncodedConn).
package nats

import (
	"context"
	"errors"
	"fmt"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"qabz/common/monitoring/jaeger"
)

// pingInterval is how often the client pings the server (bill: PingInterval 60 s).
const pingInterval = 60 * time.Second

// Request results, for Options.OnRequest (a fixed set: metric labels).
const (
	ResultOK           = "ok"
	ResultTimeout      = "timeout"       // ctx ended before the answer (also a request that waited for a reconnect)
	ResultNoResponders = "no_responders" // the server knows no subscriber of the subject
	ResultNoConnection = "no_connection" // the connection is closed
	ResultError        = "error"         // every other failure
)

// Logger writes the connection events. Nil: no line.
type Logger interface {
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// Options configure New. The fields are bill's NATS settings (config section nats).
type Options struct {
	// Name is the client name that the server shows (bill: service.name).
	Name string
	// Servers are the URLs of the cluster (nats.endpoints). At least one.
	Servers []string
	// User and Password are the credentials (nats.username, nats.password). Empty: none.
	User     string
	Password string
	// Secure turns TLS on (nats.auth; bill passes the key to nats.Options.Secure).
	Secure bool
	// AllowReconnect, MaxReconnect and ReconnectWait are the reconnect settings (nats.allow_reconnect,
	// nats.max_reconnect, nats.reconnect_wait).
	AllowReconnect bool
	MaxReconnect   int
	ReconnectWait  time.Duration
	// Timeout limits each connect attempt (nats.timeout).
	Timeout time.Duration
	// Logger writes one line for each connection event. Nil: no line.
	Logger Logger
	// OnRequest gets the subject, the result and the duration of each Request. Nil: nothing is
	// recorded.
	OnRequest func(subject, result string, d time.Duration)
	// OnConnected gets true when the connection is up and false when it is down. Nil: nothing is
	// recorded.
	OnConnected func(up bool)
}

// Client sends NATS requests. It is safe for concurrent use.
type Client struct {
	conn      *natsgo.Conn
	onRequest func(string, string, time.Duration)
	now       func() time.Time
}

// @todo docs/this_vs_bill.md item 49: the App starts when the server does not answer (bill stops its
// start); the client tries again in the background (RetryOnFailedConnect).
//
// New connects to the servers. When no server answers, New still returns the client: it writes one
// error line and connects again in the background (operator decision Q2 (a) of task 056); a request
// before the connection waits in the reconnect buffer and ends with ctx (ResultTimeout).
func New(opts Options) (*Client, error) {
	if len(opts.Servers) == 0 {
		return nil, errors.New("nats: at least one server is required (nats.endpoints)")
	}
	logInfo, logWarn, logError := func(string, ...any) {}, func(string, ...any) {}, func(string, ...any) {}
	if opts.Logger != nil {
		logInfo, logWarn, logError = opts.Logger.Info, opts.Logger.Warn, opts.Logger.Error
	}
	connected := func(up bool) {
		if opts.OnConnected != nil {
			opts.OnConnected(up)
		}
	}
	no := natsgo.Options{
		Name:                 opts.Name,
		Servers:              opts.Servers,
		User:                 opts.User,
		Password:             opts.Password,
		Secure:               opts.Secure,
		AllowReconnect:       opts.AllowReconnect,
		MaxReconnect:         opts.MaxReconnect,
		ReconnectWait:        opts.ReconnectWait,
		Timeout:              opts.Timeout,
		PingInterval:         pingInterval,
		RetryOnFailedConnect: true,
		ConnectedCB: func(c *natsgo.Conn) {
			connected(true)
			logInfo("nats: connected", "server", c.ConnectedUrlRedacted())
		},
		ReconnectedCB: func(c *natsgo.Conn) {
			connected(true)
			logInfo("nats: reconnected", "server", c.ConnectedUrlRedacted())
		},
		DisconnectedErrCB: func(_ *natsgo.Conn, err error) {
			connected(false)
			logWarn("nats: disconnected", "error", err)
		},
		ClosedCB: func(*natsgo.Conn) { connected(false) },
		AsyncErrorCB: func(_ *natsgo.Conn, _ *natsgo.Subscription, err error) {
			logError("nats: async error", "error", err)
		},
	}
	conn, err := no.Connect()
	if err != nil {
		return nil, fmt.Errorf("nats: connect: %w", err)
	}
	if conn == nil {
		return nil, errors.New("nats: Connect returned nil")
	}
	if conn.IsConnected() {
		connected(true)
	} else {
		connected(false)
		logError("nats: connect failed, trying again in the background", "servers", len(opts.Servers))
	}
	return &Client{conn: conn, onRequest: opts.OnRequest, now: time.Now}, nil
}

// Request sends data to the subject and waits for one answer or for ctx. ctx must have a deadline
// (nats.go: ErrNoDeadline, the result error); the Irancell adapter sets pay_timeout. Each request is a client
// span nats.Request, a child of the span in ctx; the message gets no trace header (bill sends none
// and the irancell service reads none).
func (c *Client) Request(ctx context.Context, subject string, data []byte) ([]byte, error) {
	if c == nil || c.conn == nil {
		return nil, errors.New("nats: client not created with New")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := otel.Tracer("qabz/system/nats").Start(ctx, "nats.Request", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("messaging.system", "nats"), attribute.String("messaging.destination.name", subject)))
	defer span.End()
	start := c.now()
	msg, err := c.conn.RequestWithContext(ctx, subject, data)
	result := resultOf(err)
	if err == nil && msg == nil {
		result, err = ResultError, errors.New("nats: empty answer")
	}
	span.SetAttributes(attribute.String("result", result))
	if err != nil {
		jaeger.Fail(span, result, err)
	}
	if c.onRequest != nil {
		c.onRequest(subject, result, c.now().Sub(start))
	}
	if err != nil {
		return nil, fmt.Errorf("nats: request %s: %w", subject, err)
	}
	return msg.Data, nil
}

// resultOf maps an error of RequestWithContext to a request result.
func resultOf(err error) string {
	switch {
	case err == nil:
		return ResultOK
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.Is(err, natsgo.ErrTimeout):
		return ResultTimeout
	case errors.Is(err, natsgo.ErrNoResponders):
		return ResultNoResponders
	case errors.Is(err, natsgo.ErrConnectionClosed), errors.Is(err, natsgo.ErrConnectionDraining):
		return ResultNoConnection
	default:
		return ResultError
	}
}

// Close closes the connection. The App calls it after the servers, the workers and the background
// jobs stop (app.release), so no request is in flight; a request after Close is no_connection.
func (c *Client) Close(context.Context) error {
	if c == nil || c.conn == nil {
		return nil
	}
	c.conn.Close()
	return nil
}
