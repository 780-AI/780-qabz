<!-- VERSION: 0.1 -->
# 014_request_metrics_middleware — request duration middleware with a request counter per path

Status: pending

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
