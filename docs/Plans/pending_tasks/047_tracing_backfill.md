<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-15 — Created (operator request): add Jaeger tracing to every feature that exists; new features follow conventions § 17.
-->
# 047_tracing_backfill — Jaeger tracing for the features that exist

Status: pending

## Description

Conventions § 17 (2026-11-15) makes tracing binding for each new or changed feature: one trace for each path of execution, from the entry to the last dependency call. This task applies § 17 to every feature that exists before the rule. Read § 17 first; it is the spec of this task.

Today the tracer provider is set up (`src/common/monitoring/jaeger`, started in `src/app/defaults.go` when `logs.otlp_endpoint` is set), but no code makes a span. `docs/initial-discovery/system.html` (Logs section) says that every request and every provider call is a span, so the doc and the code do not agree.

**Part 1: the shared parts (once, used by every feature).**
- The tracing interceptor on the public and the internal gRPC servers (`src/entry/grpc/server.go`), and the trace context from the HTTP gateway (`src/entry/http/gateway.go`) to the gRPC server.
- The trace context and a client span on each outbound gRPC client (`provider/jiring`, `provider/parsianproxy`, `provider/qabzinoproxy`, `provider/irancellproxy`) and each outbound HTTP client (`provider/qabzino`, `provider/tavanir`, `provider/karashop`).
- One hook for Postgres (`system/postgres`, the pgx tracer), one for Redis (`system/redis`, a go-redis hook) and a span for each Kafka publish (`system/kafka`), with no header on the payment message (§ 17 exception).
- A Jaeger service in `docker-compose.yml` (OTLP gRPC port), and `logs.otlp_endpoint` explained in `config.yml.example`.
- A test helper with the in-memory exporter, for the span-tree tests.
- The design step picks the modules (conventions § 2 and § 5) and checks that each is vendored.

**Part 2: the spans of each feature.** For each path, the root span, the core-step spans (§ 17: a core function with more than one dependency call) and the error status on each failure path:
- Ping and services: `Greeting` (006), `Services/Get` (013).
- My bills: insert, list, delete (016, 017, 019).
- Inquiries: water (021), electricity (022), gas (023), landline (025), mobile Irancell, MCI, Rightel (035, 036, 037), with the provider pick, the saved-inquiry read and the provider call.
- Orders: the mobile Irancell order (039) and the landline order `POST /v2/tel/order` (044).
- `PaymentComplete/OrderComplete` (044) on the internal server; the settlement in the background is a new root with a link to the request span (§ 17).
- The provider list load and reload (020), and the config reload (003): a root span for each run.
- The Parsian status worker (045): a root span for each tick, and a child span for each order, provider call, publish and save.
- Middleware that runs on each request (auth, 009) shows in the server span (an error status on a refused token), not as a new trace.

**Out of scope.** Task 046 and every later feature get their spans in their own task (§ 17). Trace headers on the Kafka payment message need an operator decision (§ 17 exception).

## Definition of Done

- With `logs.otlp_endpoint` set to the compose Jaeger, one request of each endpoint in the list above shows in Jaeger as one trace: the server span and, under it, a span for each Postgres, Redis and provider call of the path.
- One tick of the Parsian status worker (with the mock) shows as one trace, with its order, provider, Kafka and Postgres spans.
- A failed provider call shows the `codes.Error` status on its span and on the parent span.
- No span has a phone number, a bill ID, a pay ID, a user ID, an amount, a token or a body (§ 17); a test checks it.
- Each feature has a span-tree test (in-memory exporter). The coverage gate (§ 11) and `make test` pass.
- With `logs.otlp_endpoint` empty, the App starts, makes no connection to Jaeger, and every test passes.
- `system.html` (Logs section) describes the tracing as built, in the same commit (Critical File).
- A QA scenario in `OPEN_TASKS.md` (Rule 11), and the feature-ON evidence (Rule 15): the Jaeger traces of the runs above.
