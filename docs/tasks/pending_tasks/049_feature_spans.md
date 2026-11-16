<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-15 — Created (operator decision 2026-11-15, "Two tasks"): Part 2 of task 047 (the spans of each feature) moves to this task.
-->
# 049_feature_spans — the core spans of each feature

Status: pending

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
