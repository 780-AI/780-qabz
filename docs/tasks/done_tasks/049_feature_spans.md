<!-- VERSION: 1.0 -->
<!--
CHANGE LOG
v1.0 — 2026-11-16 — Done: implemented as designed, with the implementation notes I1 to I6; the feature-ON evidence added; one open operator decision (I6); moved to done_tasks.
v0.3 — 2026-11-16 — Design approved (operator: "Approve D1-D6"); implementation started.
v0.2 — 2026-11-16 — Started (operator: "Do task 49"): moved to in-progress_tasks; design written (D1 to D6), waiting for the operator decisions.
v0.1 — 2026-11-15 — Created (operator decision 2026-11-15, "Two tasks"): Part 2 of task 047 (the spans of each feature) moves to this task.
-->
# 049_feature_spans — the core spans of each feature

Status: done (2026-11-16; the operator QA confirmation and the decision of I6 are open in `OPEN_TASKS.md`)

## Description

Part 2 of the tracing backfill (conventions § 17). Needs task 047: the shared parts (the gRPC and gateway tracing, the Postgres, Redis, Kafka and outbound client spans, the error helper `jaeger.Fail` and the test helper `jaegertest`). Read § 17 and the design of task 047 first.

For each path below: the core-step spans (§ 17: a core function with more than one dependency call), the error status on each failure path (the helper of task 047: the § 12 result), and a span-tree test (in-memory exporter, `jaegertest`).
- Ping and services: `Greeting` (006), `Services/Get` (013).
- My bills: insert, list, delete (016, 017, 019).
- Inquiries: water (021), electricity (022), gas (023), landline (025), mobile Irancell, MCI, Rightel (035, 036, 037), with the provider pick, the saved-inquiry read and the provider call.
- Orders: the mobile Irancell order (039) and the landline order `POST /v2/tel/order` (044).
- `PaymentComplete/OrderComplete` (044) on the internal server.
- The Parsian status worker (045): a child span for each order, under the root span of the tick (task 046).
- Middleware that runs on each request (auth, 009) shows in the server span (an error status on a refused token), not as a new trace.

**Out of scope.** The shared parts (task 047). Task 046 and every later feature get their spans in their own task (§ 17). Trace headers on the Kafka payment message need an operator decision (§ 17 exception).

## Definition of Done

- With `logs.otlp_endpoint` set to the compose Jaeger, one request of each endpoint in the list above shows in Jaeger as one trace with its core-step spans, and the Postgres, Redis and provider spans under them.
- One tick of the Parsian status worker (with the mock) shows as one trace, with a span for each order and its provider, Kafka and Postgres spans.
- A failed provider call shows the `codes.Error` status on its span and on the parent span.
- Each feature has a span-tree test that also checks for no forbidden attribute (§ 17). The coverage gate (§ 11) and `make test` pass.
- `system.html` (Logs section) describes the spans as built, in the same commit (Critical File).
- A QA scenario in `OPEN_TASKS.md` (Rule 11), and the feature-ON evidence (Rule 15): the Jaeger traces of the runs above.

## Design (implementer, 2026-11-16; operator decision 2026-11-16: "Approve D1-D6", all recommended)

Read on 2026-11-16 from the Qabz source at `058941f`. Config keys: tasks 047 and 051 renamed `logs.otlp_endpoint`; the switch is `logs.tracing.enabled` with `logs.tracing.otlp_endpoint` (the DoD below reads this way).

### What exists and what is missing

Already traced, by tasks 046 to 048: the gRPC server spans (all paths, `otelgrpc`), Postgres, Redis, Kafka and provider client spans (shared parts), `order.Service.Create` (the mobile and tel and water orders), `order.Payments.Complete` (`PaymentComplete/OrderComplete`), the background settlement root with its link, the cron tick root `cron <name>`, and the provider list load root. Missing: the core steps of the inquiries (`inquiry.Service.Inquire`), the per-order span of the status worker, the span-tree test of each path, and `system.html`.

### Decisions

- **D1. Inquiry spans.** `core/inquiry`: span `inquiry.Inquire` (attributes `bill_type`, `provider`, `result` = the § 12 result of the metric), with the children `inquiry.fromDatabase` (the saved-inquiry read) and `inquiry.fromProvider` (the request row, the provider call, the saves). The provider pick and the checks stay inside `inquiry.Inquire` (no dependency call, only memory). One change covers water, electricity, gas, tel and the three mobile operators (one flow). A failure calls `jaeger.Fail(span, result, err)` on the span where it happens and on `inquiry.Inquire`.
- **D2. Status worker.** `core/reconcile`: span `reconcile.Check` for each order, child of the tick root `cron parsian-getstatus` (attributes `provider`, `order.id`, `result` = the result of the order count). `Fail` for `read_error`, `save_error`, `report_error`; `pending`, `skipped`, `success`, `failed`, `late` are not errors (a pending order is the normal case). The provider, Kafka and Postgres spans of the order are children by `ctx`. The settle worker (046) keeps its own spans (out of scope).
- **D3. No new core span where a function has one dependency call** (§ 17 says "more than one"): `Greeting`, `Services/Get`, my bills insert, list and delete (one repository call each), `RawInquiry` (no dependency call). Their trace is the server span with the Postgres span under it. Only the span-tree test is added for them.
- **D4. Span-tree tests.** One test for each path of the list, with `jaegertest`: the gRPC server span as root, the core spans and the dependency spans (fakes with the real hooks where the shared test already has them) with the right parent, the `codes.Error` status on a failure path (a provider that is down), and `AssertClean` with the phone, bill ID, pay ID, user ID of the test. The order paths (mobile Irancell, tel) and `OrderComplete` test the spans that tasks 046 to 048 made, as the task asks.
- **D5. The "parent span" of the DoD.** A failed provider call shows `codes.Error` on the provider client span and on `inquiry.fromProvider` and `inquiry.Inquire`. The gRPC server span has the status that `otelgrpc` gives from the gRPC code of the answer; I do not force it. Operator: confirm.
- **D6. Docs and QA.** `system.html` (Logs section; Critical File) describes the spans as built, same commit. A QA scenario in `OPEN_TASKS.md` (Rule 11) and the feature-ON run with the compose Jaeger (Rule 15): one request of each endpoint, one status worker tick (mock), a failed provider call; the trace of each is the evidence.

### Operator decisions (2026-11-16: "Approve D1-D6")

1. D1 to D4, D6 as written (recommended: yes).
2. D3: no new span for the single-call functions (recommended: yes; the alternative is a span for each, which is noise).
3. D5: the server span is not forced to `Error` (recommended: yes).

### Files (ALLOWLIST, planned)

`src/core/inquiry/inquiry.go` and its tests, `src/core/reconcile/status.go` and its test, the `entry/grpc` tests of each path in the list, `docs/initial-discovery/system.html`, `docs/Plans/OPEN_TASKS.md`, this task file, `CHANGELOG.md`, `docs/documents_manifest.md`.

## Implementation (2026-11-16)

Files: `src/core/inquiry/inquiry.go` (the three spans; `Inquire` now counts once from the new `inquire`), `src/core/inquiry/tracing_test.go` (new), `src/core/reconcile/status.go` (`checkTraced`), `src/core/reconcile/tracing_test.go` (new), `src/entry/grpc/auth.go` (the refused-token status), `src/entry/grpc/tracing_test.go` (new), `docs/initial-discovery/system.html`, `docs/Plans/OPEN_TASKS.md`.

- **I1. Span names.** The names follow the existing `order.Service.Create`: `inquiry.Service.Inquire`, `inquiry.Service.fromDatabase`, `inquiry.Service.fromProvider`, `reconcile.StatusWorker.check` (the design wrote `inquiry.Inquire` and `reconcile.Check`). Conventions § 17: `<package>.<Function>`, a fixed set.
- **I2. The refused token (task description, middleware item).** `otelgrpc` does not mark `PermissionDenied` as an error on a server span, so the auth check calls `jaeger.Fail` on the span in `ctx` (the server span) with `rejected` (`grpcserver.AuthRejected`), no reason, no token. The status `Unset` that `otelgrpc` sets at the end does not replace `Error` (OpenTelemetry SDK rule). This is the failure on the span where it happens (§ 17); D5 is about provider failures and does not change.
- **I3. No span for a request that breaks a rule.** `Inquire` starts its span after the checks of step 1, so an unknown bill type text never reaches a span and an invalid request (no dependency call) makes none; the metric does not count it either.
- **I4. Cache off.** With a cache TTL of 0 (for example `caching.irancell: 0s`) `fromDatabase` still has its span with `result=miss` and no query under it (seen in the Irancell trace).
- **I5. The gRPC test.** `entry/grpc/tracing_test.go` runs the public server (the real auth check) and the internal server with the tracing handler, as `app/defaults.go`, and a fake core that makes one span from the context it gets: it proves that each of the 14 paths gives the context of the call to the core. The core trees are tested in `core/inquiry`, `core/reconcile` and (tasks 046 to 048) `core/order`.
- **I6. Open: a provider message in a span status** (the task 047 follow-up (c) check). `otelgrpc` writes the gRPC status message as the description of a span with `Error`: the client span of a gRPC provider call gets the text of that service, and the server span of `OrderComplete` gets the Parsian message of a failed payment (`codes.Unknown`, `o.Message`). A provider text could have a phone or a bill ID. The fixed texts of Qabz on the other server spans are safe (seen live: the "provider temporarily inactive" text on the water, gas and Rightel server spans). Not changed here: an operator decision in `OPEN_TASKS.md` (a: an exporter wrapper that clears the description of the `otelgrpc` spans; b: accept).

Verification:

- **Tests:** `make test GO_TEST_ARGS="-race -count=1 -cover -v"`: exit 0, every package passes, no test skips (the live tests ran). Changed packages: `core/inquiry` 98.9 %, `core/reconcile` 98.4 %, `entry/grpc` 98.6 %. Positive controls (each check failed once on a known-bad input): `fromProvider` without its span in `ctx` fails the 7 cases of `TestInquireSpanTree`; the fake core with `context.Background()` fails `TestServerSpanTrees` (26 assertions); the auth check without `jaeger.Fail` fails `TestRejectedTokenSpan` (status and description).
- **Feature ON (Rule 15),** the compose Postgres, Redis, Kafka and Jaeger (2.23.0), an App copy on the ports 18080 / 18082 / 18083 / 19090 (the operator's App on 8080 / 8082 / 9090 was not touched), `logs.tracing.enabled: true` with `localhost:4317`, the mocks of Tavanir, Irancell, Jiring Switch, Parsian and Karashop, the Qabzino endpoints at a local server that answers HTTP 503, `debug.cache_inquiries: true`. Each request sent its own `traceparent`; the traces were read from the Jaeger API:
  - Greeting: the gateway client span and the server span only. Services, MyBills Insert / GetAll / Delete: the server span with the query spans under it (D3: no core span).
  - Electricity (Tavanir mock), landline (Karashop mock), Irancell, MCI (Parsian mock): `inquiry.Service.Inquire` (`result=provider_ok`) under the server span; under it `fromDatabase` (`result=miss`, the `SELECT`; Irancell: no `SELECT`, I4) and `fromProvider` (`INSERT` of the request row, `UPDATE` of the status, `INSERT` of the answer).
  - Water, gas, Rightel (Qabzino at the 503 server): `HTTP POST` with `Error` under `fromProvider`; `fromProvider` and `Inquire` with `Error` and `provider_down`; the server spans with `Error` (gRPC `Unavailable`, the fixed text of Qabz). The 503 server got `traceparent` with the trace ID of each request.
  - A second landline inquiry: `Inquire` `result=db_hit`, `fromDatabase` `result=hit` (`SELECT`, `INSERT` of the copy), no `fromProvider`.
  - Orders: Irancell and water `order.Service.Create` (`result=created`) with their queries; the landline order also has `inquiry.Service.Inquire` (`db_hit`) with `fromDatabase` under `order.Service.Create`.
  - `OrderComplete` (a scratch gRPC client with a `traceparent`, water with the pay mock at `success`, then the landline order with the mock at `pending`): one trace each, `order.Payments.Complete` under the server span (`result=success`, then `pending`).
  - One status worker tick: `cron parsian-getstatus` (`result=done`) with the lock spans and, for the pending landline order, `reconcile.StatusWorker.check` (`provider=parsian`, `order.id`, `result=success`) with the `SELECT`, `kafka.Publish` (`result=ok`), `BEGIN`, `SELECT`, `INSERT`, `COMMIT` under it; the log line `parsian status: reported`. The Parsian mock makes no call, so no provider span (as designed).
  - The refused token (`debug.auth.disable: false`, `Bearer not-a-token`): HTTP 403; the gateway client span `Error` (`unauthenticated`), the server span `Error` with `rejected`, no other span.
  - The QA rows were deleted from the dev database after the run (8 `orders`, 11 `bill_inquiries`, 7 `bill_inquiry_requests` rows, 1 `bills` row); the Kafka message of the tick stays in the dev topic `paymentwriter`. The scratch client was built under the gitignored `src/tmp/` and removed.
- **Not done:** the operator QA confirmation, and the decision of I6 (both in `OPEN_TASKS.md`).
