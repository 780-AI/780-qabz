<!-- VERSION: 1.0 -->
# 014_request_metrics_middleware — request duration middleware with a request counter per path

Status: done

## Description

Add the request duration middleware that `Implementation_Plan.md` Step 6 requires for every endpoint. It does not exist in the code yet: the only interceptor today is the auth check (`src/entry/grpc/auth.go`). The middleware also records a Prometheus metric that counts each incoming request, with the request path as a label.

- Duration: a histogram of the time of each request, in seconds (`system.html`: `qabz_` prefix, times in seconds).
- Count: a counter that adds 1 for each incoming request, labeled with the request path.
- The metrics follow the Prometheus switch of Step 5: with metrics off, they have no effect.

Points for the design step:
- Layer: a gRPC interceptor (it sees every call, direct gRPC and through the gateway) or an HTTP middleware on the gateway (it sees only HTTP calls, but has the real path).
- Path label: a raw path (for example `/v1/mybill/<uuid>`) gives one series per ID, so the metric count has no limit. Use a fixed value per route: the gRPC method (`/haf80.api.bill.Service/Get`) or the HTTP route pattern (`/v1/mybill/{id}`). An unknown route gets one fixed value.
- Other labels: the status code (gRPC code or HTTP status) and the method.
- Metric names, for example `qabz_requests_total{path,...}` and `qabz_request_duration_seconds{path,...}`, and the histogram buckets.
- Order with the auth check: whether a rejected call (HTTP 403) is counted.

## Definition of Done

- Every incoming request adds 1 to the request counter with its path label, and records its duration in seconds.
- The path label has a fixed set of values (no IDs).
- With metrics off, nothing is recorded or registered.
- Unit tests cover the counter, the labels, the duration and the switch; build, vet and tests pass.
- The new metrics are in `system.html` (Critical File workflow), `app/README.md` and the CHANGELOG, in the same commit.

## Verification

Run the App, call `GET /v1/greeting` 3 times and `GET /v1/services` once, then read `/metrics`. Pass: the counter shows 3 for the Greeting path and 1 for the Services path, and the duration histogram has a count for each path.

## Decisions (operator, 2026-11-07)

- The Prometheus switch of Step 5 did not exist in the code. The operator chose to add it in this task ("Add the switch"): config key `metrics.prometheus.enabled`, default `true`, off only with an explicit `false`. It covers every `qabz` metric, the two older ones too.
- Durations are in milliseconds ("Generally duration must be recorded in milliseconds"): the histogram is `qabz_request_duration_ms`, not `_seconds`. The general rule is `PROJECT_SPECIFIC_CONVENTIONS.md` § 9 (its own commit, before this one).
- No separate counter: the operator removed `qabz_requests_total` ("remove qabz_requests_total"), because the histogram's `_count` series already adds 1 for each request with the same labels. That series meets the "request counter" item of the Definition of Done.
- Design accepted with the switch decision: a gRPC interceptor, first in the chain (before the auth check); labels `path` (the full gRPC method) and `code` (the gRPC code name); no `method` label.

## Result (2026-11-07)

- `config/config.go`: `MetricsEnabled` (`metrics.prometheus.enabled`, default `true`); `config.yml.example` and `config.yml` have the key.
- `common/monitoring/prometheus/prometheus.go`: `New(addr, enabled)`. A disabled `Server` has no registry (`Registry()` is nil), registers no collector, and its `Start` / `Stop` do nothing; `Enabled()`.
- `common/monitoring/prometheus/requests.go`: `NewRequestMetrics` registers `qabz_request_duration_ms{path,code}` (buckets 5 to 10000 ms; its `_count` is the request counter); returns nil with metrics off; `Observe` records `float64(d) / float64(time.Millisecond)`.
- `entry/grpc/requests.go`: `NewRequests(observer)`, unary and stream interceptors; `path` = `info.FullMethod` (`unknown` without it), `code` = `status.Code(err).String()`.
- `app/defaults.go`: the request options come before the auth options; `newAuth` and `newService` create and register their metrics only with metrics on. `app/app.go`: `Start` logs `metrics disabled (metrics.prometheus.enabled: false)` in place of `metrics started`.
- An unknown HTTP route ends in the gateway (404) and never reaches gRPC, so it is not counted; the gRPC server calls interceptors only for registered methods, so `path` has a fixed set of values.
- Tests: config switch (default, explicit on, explicit off, wrong type); disabled and nil server; histogram count, labels, sum and buckets in ms, sub-ms value, `/metrics` text; interceptor (codes, unknown path, duration, stream); rejected call counted before auth (gRPC and through the composition root); gateway calls counted with the gRPC path and a 404 not counted; the wiring registers all three `qabz` metrics with the switch on and none with it off; the `Start` log line. `go build`, `go vet`, `go test -race -count=1 ./...` pass.
- Deliberate failures: auth before metrics fails `TestDefaultBuilderCountsRejectedCalls`; a switch that is ignored fails 5 tests; two observations per call fail 3 count tests; `d.Seconds()` in place of milliseconds fails `TestRequestMetricsRecord`.
- Feature ON (App binary, compose dev database): 3 x `GET /v1/greeting` (200) and 1 x `GET /v1/services` (500: the dev database has no `services` table yet, task 015) gave `qabz_request_duration_ms_count` 3 for `/haf80.api.bill.Service/Greeting` code `OK` and 1 for `/haf80.api.bill.Service/Get` code `Internal`; the Get sum 8.2 ms in the `le="10"` bucket; the Greeting sum 0.005 ms. `GET /v1/mybill/1234` (404) was not counted. With the auth check on, a call with no token (403) was counted with `code="PermissionDenied"` and `qabz_auth_rejected_total{reason="missing"}` 1. With `enabled: false` the log said `metrics disabled`, port 9090 did not listen and the API answered.
- Postman: not affected (no client-visible change).

## Open items at close (2026-11-07)

- Step 6 middleware still missing: panic recovery and tracing spans per request (separate tasks).
