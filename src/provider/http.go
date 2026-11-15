package provider

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// NewHTTPClient is the default HTTP client of the HTTP adapters (Qabzino, Tavanir, Karashop): the
// timeout, the system TLS roots, and a client span for each request ("HTTP <method>", with the
// method, the status code and the URL of the config) that sends traceparent (task 047, conventions
// § 17). With tracing off the span is no-op and no header is sent.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: otelhttp.NewTransport(http.DefaultTransport,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return "HTTP " + r.Method })),
	}
}
