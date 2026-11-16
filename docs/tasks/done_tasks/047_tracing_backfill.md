<!-- VERSION: 1.0 -->
<!--
CHANGE LOG
v1.0 — 2026-11-15 — Done: implemented as designed, with the implementation notes I1 to I6; the feature-ON evidence added; moved to done_tasks.
v0.2 — 2026-11-15 — Started (operator: "Do task 47"): moved to in-progress_tasks; split (operator decision S1 "Two tasks"): the spans of each feature (Part 2) move to new task 049; design written (D1 to D17; operator decisions S1, D4 (a), D10 (a), D7 (a), 2026-11-15, all recommended).
v0.1 — 2026-11-15 — Created (operator request): add Jaeger tracing to every feature that exists; new features follow conventions § 17.
-->
# 047_tracing_backfill — the shared tracing parts

Status: done (2026-11-15; the operator QA confirmation is open in `OPEN_TASKS.md`)

## Description

Conventions § 17 (2026-11-15) makes tracing binding for each new or changed feature: one trace for each path of execution, from the entry to the last dependency call. This task adds the **shared parts** that every feature uses. Read § 17 first; it is the spec of this task.

Today the tracer provider is set up (`src/common/monitoring/jaeger`, started in `src/app/defaults.go` when `logs.otlp_endpoint` is set), but only the task 046 code makes a span. `docs/initial-discovery/system.html` (Logs section) says that every request and every provider call is a span, so the doc and the code do not agree.

**The shared parts (once, used by every feature).**
- The tracing on the public and the internal gRPC servers, and the trace context from the HTTP gateway (`src/entry/http/gateway.go`) to the gRPC server.
- The trace context and a client span on each outbound gRPC client (`provider/jiring`, `provider/parsianproxy`, `provider/qabzinoproxy`, `provider/irancellproxy`) and each outbound HTTP client (`provider/qabzino`, `provider/tavanir`, `provider/karashop`).
- One hook for Postgres (`system/postgres`), one for Redis (`system/redis`) and a span for each Kafka publish (`system/kafka`), with no header on the payment message (§ 17 exception).
- The root span of each run that no request starts and that has no root yet (the provider list load and reload, the config reload), and no new root inside a path (the two `context.Background()` steps below).
- A Jaeger service in `docker-compose.yml` (OTLP gRPC port), and `logs.otlp_endpoint` explained in `config.yml.example`.
- A test helper with the in-memory exporter, for the span-tree tests, and one error helper for every span.

**Out of scope.** The core-step spans, the error status of each feature path and the span-tree test of each feature are task 049 (operator decision S1). Task 046 and every later feature get their spans in their own task (§ 17). Trace headers on the Kafka payment message need an operator decision (§ 17 exception).

## Definition of Done

- With `logs.otlp_endpoint` set to the compose Jaeger, one request of each public endpoint shows in Jaeger as one trace: the gateway client span, the gRPC server span and, under it, a span for each Postgres, Redis and provider call of the path. `OrderComplete` on the internal server is one trace, and the settlement in the background is a new root with a link to it.
- One tick of the Parsian status worker (with the mock) shows as one trace, with its Postgres, Redis, provider and Kafka spans.
- A failed provider call shows the `codes.Error` status on its client span.
- No span has a phone number, a bill ID, a pay ID, a user ID, an amount, a token or a body (§ 17); a test checks it.
- The shared parts have tests (in-memory exporter). The coverage gate (§ 11) and `make test` pass.
- With `logs.otlp_endpoint` empty, the App starts, makes no connection to Jaeger, sends no `traceparent` header, and every test passes.
- `system.html` (Logs section) describes the tracing as built, in the same commit (Critical File).
- A QA scenario in `OPEN_TASKS.md` (Rule 11), and the feature-ON evidence (Rule 15): the Jaeger traces of the runs above.

## Design (implementer, 2026-11-15; operator decisions 2026-11-15: S1, D4 (a), D7 (a), D10 (a), all recommended)

Read on 2026-11-15 from the Qabz source at `a8790dc`. The module versions were read from `proxy.golang.org` on 2026-11-15.

### Operator decisions

- **S1. Split.** Two tasks (operator: "Two tasks"). This task: the shared parts. Task 049: the spans of each feature. Each task is one commit.
- **D4. The HTTP gateway has no HTTP span** (operator: "No HTTP span"). `otelhttp` puts `url.path` in the server span, and `DELETE /v1/mybill/{id}` has the saved-bill ID in the path.
- **D7. Postgres: our own pgx tracer** (operator: "Own pgx tracer"). No new module; no SQL text and no arguments in the span.
- **D10. A failed span carries the result and the error type** (operator: "Result + error type"). `span.RecordError(err)` writes `err.Error()`, and an error text can have a phone or a bill ID in it. The full text stays in the log line. Conventions § 17 changes to match (same commit as the code).

### Modules (conventions §§ 2 and 5)

- **D1. New modules** (official OpenTelemetry and go-redis instrumentation; each matches the versions in `go.mod`):
  - `go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc` v0.72.0 (needs otel v1.47.0, grpc v1.83.2 or later; we have v1.85.0);
  - `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` v0.72.0 (the outbound HTTP clients only, D6);
  - `github.com/redis/go-redis/extra/redisotel/v9` v9.23.0 (the same version as `go-redis/v9`).
  - `go.opentelemetry.io/otel/propagation` is in the `otel` module (no new module). `go mod tidy && go mod vendor`; `vendor/` is not committed.
- The libraries also make OpenTelemetry metrics through the global meter provider. Qabz sets no meter provider, so these metrics are no-op. Prometheus does not change (§ 12: tracing is not a feature; no new metric).

### Setup

- **D2. The propagator.** `jaeger.New` also sets `otel.SetTextMapPropagator(propagation.TraceContext{})` (W3C `traceparent` only, no baggage: baggage can carry free input). With `logs.otlp_endpoint` empty, `jaeger.New` is not called: the global tracer provider and the global propagator stay the no-op ones, so no span is recorded, no header is sent or read, and no connection is made (the off switch of § 17). The global provider and propagator of OpenTelemetry delegate, so the clients and handlers made before `jaeger.New` use them too (a test checks it: D15).
- **D3. The gRPC servers.** `grpc.StatsHandler(otelgrpc.NewServerHandler())` is the first server option of the public and the internal gRPC servers (`app/defaults.go`). The server span name is the full gRPC method (§ 17). It reads `traceparent` from the incoming metadata, so the payment service (internal server) continues its own trace. No message events (the `otelgrpc` default), so no body.
- **D4. The HTTP gateway** (`entry/http/gateway.go`).
  - A small handler around the gateway mux reads the W3C context from the request headers into the request context (`otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))`). It makes no span.
  - The gateway gRPC connection gets `grpc.WithStatsHandler(otelgrpc.NewClientHandler())`: a client span named with the full gRPC method, and `traceparent` in the outgoing metadata. So one request is one trace: the gateway client span (the root, or a child of the app's `traceparent`), then the gRPC server span.

### Outbound calls

- **D5. Outbound gRPC.** The four `grpc.NewClient` calls (`provider/jiring`, `parsianproxy`, `qabzinoproxy`, `irancellproxy`) get `grpc.WithStatsHandler(otelgrpc.NewClientHandler())`: a client span (the full gRPC method) and `traceparent` in the metadata.
- **D6. Outbound HTTP.** The default client of `provider/qabzino`, `provider/tavanir` and `provider/karashop` (the one made when `HTTPClient` is nil) gets `Transport: otelhttp.NewTransport(http.DefaultTransport)`. The span name is `HTTP POST` (a span name formatter: the method only). The span has the method, the status code and `url.full`: the endpoint of the config (each endpoint is a POST to a fixed URL; the number, the bill ID and the password are in the body). `traceparent` goes to the external provider (§ 17: each outbound call sends the W3C context). A test client given in `HTTPClient` is used as it is.

### Storage and Kafka

- **D7. Postgres** (`system/postgres/tracer.go`, new). A `pgx.QueryTracer` (`TraceQueryStart`, `TraceQueryEnd`), set in `postgres.New` (`pgxpool.ParseConfig`, `cfg.ConnConfig.Tracer`, `pgxpool.NewWithConfig`). One client span for each query (also the `begin` and `commit` of a transaction; see I2 for `Ping`). The span name is the SQL operation: the first word of the SQL, in upper case, from a fixed set (`SELECT`, `INSERT`, `UPDATE`, `DELETE`, `WITH`, `BEGIN`, `COMMIT`, `ROLLBACK`, `CREATE`, `DROP`, `ALTER`), else `QUERY`. Attributes: `db.system.name` = `postgresql`, `db.operation.name` = the name. No SQL text, no arguments. A query error: `jaeger.Fail(span, "error", err)` (D10). The migrator connection (`golang-migrate`, start only) is not traced.
- **D8. Redis** (`system/redis/redis.go`). `redisotel.InstrumentTracing(rdb, redisotel.WithDBStatement(false), redisotel.WithCallerEnabled(false))`. One client span for each command (the name is the command, for example `get`, `evalsha`), and for each pipeline. No arguments: the keys can have a phone or a bill ID. `redisotel` records its own errors (the Redis error text, for example `WRONGTYPE`; `redis.Nil` is not an error); that text has no input.
- **D9. Kafka** (`system/kafka/kafka.go`). `Publish` makes a producer span `kafka.Publish` with `messaging.system` = `kafka`, `messaging.destination.name` = the topic (from the config) and `result` (`ok`, `error`: the result of `qabz_kafka_publish_total`). An error: `jaeger.Fail`. The message gets no header (§ 17 exception, task 045 decision C).

### Errors, roots, context

- **D10. The error helper** `jaeger.Fail(span trace.Span, result string, err error)` (`common/monitoring/jaeger`): `span.SetStatus(codes.Error, result)` (the result of § 12) and, when `err` is not nil, an event `exception` with only `exception.type` (the Go type of the error, for example `*pgconn.PgError`). It replaces `span.RecordError` in `entry/cron/cron.go` and `core/order/settle.go` (task 046). Conventions § 17 ("Names, attributes and errors") says: call `jaeger.Fail`, never `span.RecordError`. The instrumentation libraries keep their own error handling (`otelgrpc` sets the gRPC status code and the status message; `otelhttp` and `redisotel` record the transport or Redis error text, which has the endpoint of the config and no input).
- **D11. No new root inside a path** (§ 17 "Context").
  - `core/inquiry`: `finishErr` and `finish` take `ctx` and use `context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)` (was `context.Background()`), so the final status write is in the request trace and the client still cannot cancel it.
  - `core/order`: `Payments.background(o)` becomes `background(ctx, o)`. The settlement starts a new root span `order.Payments.settle` with `trace.WithNewRoot()` and `trace.WithLinks(trace.LinkFromContext(ctx))` (§ 17: a background job that a request starts; task 046 follow-up (c)). Its context stays `context.Background()` with `settleTimeout` (the request can end first). `MarkInquiryPaid`, `SetLastOrderDate` and `order.Settler.Settle` are its children. The span has `order.id`, `bill_type` and `provider`.
- **D12. The roots of the runs that no request starts** (§ 17: `<kind> <name>`).
  - `provider.ProvidersService`: each load (the first load at start and each reload of the loop) is a root span `providers load` with `result` (`ok`, `error`, the result of `qabz_providers_loads_total`).
  - `config.Reloader.Reload`: each run is a root span `config reload` with `result` (`ok`, `error`).
  - **`entry/cron` (changes task 046):** the root span `cron <name>` starts before the lock, not after it, so the Redis lock command is in the tick trace. A tick with no lock has the result `no_lock` and no error status (it is the normal result of the other instances). Each other result keeps the task 046 status.
  - A single call at start (the Postgres and Redis `Ping`) is its own one-span trace.

### Compose, config, tests, docs

- **D13. Jaeger in compose.** A service `jaeger` (image `jaegertracing/jaeger:2.23.0`, the all-in-one with the in-memory store), profile `tracing` (`make compose up` does not start it, as `kafka`): `COMPOSE_PROFILES=tracing make compose up ARGS="-d --wait"` (see I5). Ports `127.0.0.1:${JAEGER_UI_PORT:-16686}:16686` (the UI) and `127.0.0.1:${JAEGER_OTLP_PORT:-4317}:4317` (OTLP gRPC). `config.yml.example`: `otlp_endpoint: ""  # host:port of the OTLP gRPC receiver (the compose Jaeger: "localhost:4317"); empty: tracing is off`.
- **D14. The test helper** `common/monitoring/jaeger/jaegertest` (test code imports it; it imports `tracetest`):
  - `Install(t) *tracetest.SpanRecorder`: sets a global tracer provider with the recorder and the `TraceContext` propagator, and puts back the old ones in `t.Cleanup` (a test that uses it does not call `t.Parallel`).
  - `AssertClean(t, spans, inputs ...string)`: every attribute key of each span and event is in a fixed allow-list (`bill_type`, `provider`, `operator`, `result`, `source`, `cron`, `order.id`, `exception.type`, and the keys of the instrumentation: `rpc.*`, `db.system.name`, `db.operation.name`, `db.system`, `server.*`, `network.*`, `http.request.method`, `http.response.status_code`, `url.full`, `messaging.*`, `error.type`), and no span name, attribute value, event attribute or status description has any of `inputs` (the phone, the bill ID, the pay ID, the user ID, the amount, the token of the test).
- **D15. Tests of the shared parts** (in-memory exporter; positive controls: each check fails once on a known-bad input):
  - gateway → gRPC (`entry/http`): an HTTP request with a `traceparent` gives a client span and a server span in that trace, the server span a child of the client span; with no `traceparent`, the client span is a root; the helper and the gRPC client are made before `Install` (D2).
  - `system/postgres` (live tests): a query gives one span with the operation name; a failed query gives the error status and only `exception.type`; `AssertClean` with the query arguments.
  - `system/redis` (live tests): a command gives one span, with no key in it.
  - `system/kafka`: a publish gives `kafka.Publish` (the fake producer of the tests), the error status on a failed publish, and no header on the message.
  - Outbound: one HTTP adapter (Karashop, `httptest` server, the default client) gets `traceparent` and makes the `HTTP POST` span; one gRPC adapter (a test gRPC server) gets `traceparent` in the metadata. Off: with the default globals (no `Install`), no `traceparent` is sent.
  - The context fixes: the inquiry `finish` write is in the request trace; the settlement root is a new trace with a link to the request span; `providers load`, `config reload` and `cron <name>` (with `no_lock`) are roots with the result.
  - `jaeger.Fail`: the status, the result and only `exception.type`.
- **D16. Docs.** `system.html` (Logs section: the tracing as built; the "as implemented so far" paragraph of task 046 is replaced), `docker-compose.yml` header comment, `config.yml.example`, `src/app/README.md` (the tracer line), conventions § 17 (D10, and "the gateway has no HTTP span" in "Root span"), `CHANGELOG.md`, `docs/documents_manifest.md`, `docs/Plans/OPEN_TASKS.md` (QA scenario; the task 046 follow-up (c) and the task 013 follow-up), this task file. `docs/this_vs_bill.md`: no item (bill's spans are not seen by a client, the payment service or the database).
- **D17. Verification (Rule 15).** The compose Jaeger, the App with `logs.otlp_endpoint: "localhost:4317"`, `mode: dev`, the provider mocks: one call of each public endpoint, one `OrderComplete` on the internal server, one tick of the Parsian status worker (mock, compose Kafka). The traces are read from the Jaeger API (`/api/traces?service=qabz`) and checked against D3 to D12. Then `logs.otlp_endpoint: ""`: the App starts, `lsof` shows no connection to port 4317, and an outbound mock call has no `traceparent`.

### Files (planned)

`src/common/monitoring/jaeger/jaeger.go` (propagator, `Fail`) and its test, `src/common/monitoring/jaeger/jaegertest/jaegertest.go` (new), `src/app/defaults.go`, `src/entry/http/gateway.go`, `src/provider/{jiring,parsianproxy,qabzinoproxy,irancellproxy,qabzino,tavanir,karashop}/*.go`, `src/system/postgres/{postgres.go,tracer.go}`, `src/system/redis/redis.go`, `src/system/kafka/kafka.go`, `src/core/inquiry/inquiry.go`, `src/core/order/{complete.go,settle.go}`, `src/provider/selector.go`, `src/config/hot.go`, `src/entry/cron/cron.go`, the tests of these packages, `src/go.mod`, `src/go.sum`, `docker-compose.yml`, `config.yml.example`, `docs/initial-discovery/system.html`, `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md`, `src/app/README.md`, `CHANGELOG.md`, `docs/documents_manifest.md`, `docs/Plans/OPEN_TASKS.md`, this task file.

## Implementation (2026-11-15)

Implemented as designed (D1 to D17). What the design did not name, and how it was done:

- **I1. The globals delegate once (D2).** The OpenTelemetry global provider and propagator forward to the first ones that a process sets. In the App that is `jaeger.New`, so the handlers and clients made before it work (seen in the feature-ON run: the gRPC servers and the gateway are made before `jaeger.New`). In tests, many `jaegertest.Install` calls replace the globals, so each test makes its handlers after `Install`. The "off" test (`provider.TestNewHTTPClientOff`) runs itself again in a child process that sets no global.
- **I2. No `Ping` span.** pgx sends `Ping` and the health ping of the pool to `pgconn` directly; they do not reach the query tracer, so they make no span (and no lone trace every second).
- **I3. One shared HTTP client.** `provider.NewHTTPClient(timeout)` (new `src/provider/http.go`) is the default client of the Qabzino, Tavanir and Karashop adapters: the timeout, the default transport wrapped by `otelhttp.NewTransport`, the span name `HTTP <method>`.
- **I4. The allow-list of `jaegertest.AssertClean`.** The live Redis test found the `redisotel` keys `db.connection_string` (`tcp://<host:port of the config>`, no password) and `db.redis.num_cmd` (a count): allowed. `redisotel` and `otelhttp` record their own errors with `exception.message`: allowed only on a span of an instrumentation library, never on a span of Qabz code (scope `qabz/...`); its value is still checked against the test inputs.
- **I5. The compose profiles.** `make compose up ARGS="--profile ..."` does not work (`unknown flag: --profile`: the flag must come before `up`). The compose header now says `COMPOSE_PROFILES=tracing make compose up ARGS="-d --wait"`, and the same for the `kafka` line of task 045, which had the same defect.
- **I6. The cron root before the lock (D12).** A failed lock now also has the error status and the `exception` event (`jaeger.Fail`); `TestRootSpan` covers done, error, `no_lock` and a lock error.

Verification:

- **Tests:** `make test GO_TEST_ARGS="-race -count=1 -cover"`: every package passes, the live tests ran. Changed packages: `app` 87.6 %, `common/monitoring/jaeger` 92.3 %, `jaegertest` 98.0 %, `config` 97.4 %, `core/inquiry` 98.7 %, `core/order` 94.0 %, `entry/cron` 98.1 %, `entry/http` 88.9 %, `provider` 96.1 %, `provider/irancellproxy` 98.1 %, `jiring` 98.0 %, `karashop` 95.3 %, `parsianproxy` 96.6 %, `qabzino` 98.2 %, `qabzinoproxy` 98.3 %, `tavanir` 98.0 %, `system/kafka` 88.7 %, `system/postgres` 90.4 %, `system/redis` 88.9 %. Positive controls (each check failed once on a known-bad input): the gateway with no `withTraceContext` fails `TestGatewayContinuesTheTrace`; `finishErr` with `context.Background()` fails the 3 cases of `TestFinishKeepsTheRequestSpan`; `span.RecordError` in the Settler fails `TestSettlerResults` (`exception.message` not allowed, the text "down"); `Install` in the child process fails `TestNewHTTPClientOff`; each forbidden case of `TestAssertCleanFails`.
- **Feature ON (Rule 15),** the compose Postgres, Redis, Kafka and Jaeger (2.23.0), an App copy on the ports 18080 / 18082 / 18083 / 19090 (the operator's debug App on 8080 / 8082 / 9090 was not touched), `logs.otlp_endpoint: localhost:4317`, the mocks of Parsian, Karashop, Tavanir and Irancell, the Qabzino endpoints at a local fake server that answers HTTP 503, `debug.cache_inquiries: true`. Each request sent its own `traceparent`; the traces were read from the Jaeger API:
  - Greeting, Services, MyBills Insert / GetAll / Delete, the water, electricity, gas, landline and mobile (Irancell, MCI, Rightel) inquiries, the Irancell and landline orders: each is one trace whose root is a child of the `traceparent` of the request; under it the gateway client span, the gRPC server span (the full method), and the Postgres spans (`SELECT`, `INSERT`, `UPDATE`) of the path. No HTTP span.
  - Water, gas and Rightel (Qabzino at the fake 503 server): the `HTTP POST` span and the server spans have the error status; the fake server got `traceparent` with the trace ID of the request.
  - Irancell with the mock off and the address `127.0.0.1:1`: the client span `pb.InternalService/BillInquiry` has the error status.
  - `OrderComplete` on the internal server (a scratch gRPC client with a `traceparent`): one trace, a child of the span of the client. With the Parsian pay mock at `success`: a new root `order.Payments.settle` (`order.id`, `bill_type`, `provider`) with a link to the server span of that `OrderComplete` call; under it the two `UPDATE` writes and `order.Settler.Settle` (`result=success`).
  - One tick of the Parsian status worker (the `pending` order of the first `OrderComplete`, the mock at `success`): one trace `cron parsian-getstatus` (`result=done`) with the Redis `set` (the lock), the page query, `evalsha` (the charge), `kafka.Publish` (`paymentwriter`, `result=ok`), the `BEGIN`, `SELECT`, `INSERT`, `COMMIT` of the final row and the release `evalsha`; `qabz_cron_orders_processed_total{result="success"}` 1.
  - `providers load` and `config reload`: roots with `result=ok` and their `SELECT` as a child.
  - Off: with `logs.otlp_endpoint: ""` the App started and stopped, `lsof` showed no connection to port 4317, the fake server got no `traceparent`, and Jaeger had no trace of the request.
  - The QA rows were deleted from the dev database after the run (8 `orders`, 11 `bill_inquiries`, 9 `bill_inquiry_requests` rows, 1 `bills` row).
  - Not covered live: a real call to a Qabz service that is traced too (none runs here); the mocked providers make no call, so they have no provider span (as designed).

