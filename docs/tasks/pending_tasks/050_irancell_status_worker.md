<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-15 — Created (operator request): the Irancell status worker, as the Parsian status worker of task 045, with the defects of bill's worker fixed, every error logged and a metric of the statuses fixed in each run.
-->
# 050_irancell_status_worker — the Irancell status worker

Status: pending

## Description

A status worker for the orders of the provider `irancell`, as the Parsian status worker of task 045. Read task 045 and `docs/this_vs_bill.md` item 43 first. Needs task 045 (the shared worker runner, `reconcile.StatusWorker`, the Kafka reporter) and task 047 (tracing).

**bill's worker** (`../bill`): `pkg/cronTask/irancellGetStatus.go` (`IrancellGetStatusTask`, config `irancell.getStatus.*`) runs the shared `InquiryCron` (`service/billPaymentInquiry/dto.go`) with the name `irancell`. The status call is `gateway/paymentInquiry/irancell.go`: gRPC `InternalService/GetStatus` of the `irancell` service with `OrderId` and `PurchaseType: "charge"`. A gRPC error is `pending`; else the status is `response.Status`, the provider trace ID is `response.Tid` and the message is `response.Message`. Only `success` and `failed` are final.

**Reuse, no copy.** `reconcile.StatusWorker` (`src/core/reconcile/status.go`) takes the provider as an option. The Irancell worker is a second instance with `Provider: irancellproxy.Name`, wired in `src/app/crons.go` next to the Parsian worker. The new code is:
- `Adapter.Status` in `src/provider/irancellproxy/irancellproxy.go` (now `ErrNotSupported`): the `GetStatus` call above, with the timeout `providers.internal.irancell.timeout`. A failed call is `provider.ErrProviderDown`.
- The Irancell mock answers `success`, `failed` and `pending` for the status (conventions § 15).
- Config `providers.internal.irancell.get_status.*` with the same keys as the Parsian worker (`enabled`, default `false`; `name`; `cron_time`; `start`; `end`; `chunk`; `lock_time`; `reverse_deadline`; `sync_operations`), in `config.yml.example` and `config.yml` (§ 8). bill's example config has no Irancell worker values, so the design step asks the operator for the defaults.

**Defects of bill's worker that this task MUST fix.** For each one, the design step checks if task 045 already fixed it in the shared code. If yes, a test proves it for the Irancell worker. If no, the fix goes into the shared code, so the Parsian worker gets it too (the design names that change to the operator).
1. **Skipped orders in the pages.** bill reads pages with `OFFSET` and adds `chunk` each time, but an order that the run made final leaves the result set, so other pending orders move back past the offset and that run does not check them. The fix: no pending order of the window is skipped in a run (for example a keyset page on `created_at, order_id`, or a snapshot of the IDs at the start of the run). A test with more pending orders than `chunk`, and some of them made final, proves it.
2. **Kafka before the database.** bill reports on Kafka and saves after. A failed save after a good report leaves the payment service with the final status and the order with `pending`, and the next run reports again. The fix: the design gives a way with no lost status and no wrong state (for example the save first and the report after, with a retry of a failed report), and asks the operator if the payment service accepts a second report of the same order.
3. **Two different windows.** bill reads the newest row of the last 48 h (`GetOrderByIDWithIntervals`) but the final save locks the newest row of the last 24 h (`NewOrderWithFinalStatus`). An order from 24 h to 48 h old fails at the save each time. The fix: one window for the read and the save (`order.LatestWindow`).
4. **No timeout and no stop.** bill asks with `context.Background()`. The call uses the context of the run and the adapter timeout (task 045 has this; a test proves it for Irancell).
5. **Errors with no log line.** bill skips most errors with `continue` and writes nothing.

**Every error writes a log line (operator, 2026-11-15).** A failed page read, a failed order read (now counted with no line: `ResultReadError`), a failed or timed-out status call, an unknown status text from Irancell, a failed report, a failed save, a lost lock, a failed release and a panic each write one error line with the worker name, the order ID (when there is one) and the error. This changes conventions § 10 ("counted, no line") for the read error: the design step names the change and, if it goes into the shared worker, the Parsian worker writes the same lines.

**A metric of the statuses fixed in each run (operator, 2026-11-15).** A Prometheus metric gives the number of orders that the run moved to a final status (`success`, `failed`, and the late saves), for each run and each worker. Name and type are a design step (conventions §§ 9, 12), for example a histogram `qabz_cron_orders_fixed` with the label `cron`, observed once at the end of each run (also 0), next to the counters that exist (`qabz_cron_runs_total`, `qabz_cron_duration_ms`, `qabz_cron_orders_processed_total`, `qabz_provider_calls_total` with action `status`).

**Tracing (conventions § 17).** The root span of each tick, a child span for each order, and the `GetStatus` call, Kafka and Postgres spans under it.

**Open point for the design step.** Qabz has no Irancell payment yet (`Adapter.Pay` is `ErrNotSupported`), so the pending Irancell orders come only from bill in a shared database. The design step confirms the source of the orders with the operator and that bill's own Irancell worker and this worker do not run on the same orders at the same time.

## Definition of Done

- The Irancell worker is a config of the shared `reconcile.StatusWorker`, not a copy; `Adapter.Status` calls `GetStatus` with `PurchaseType: "charge"` and the adapter timeout; the mock answers the three statuses.
- `providers.internal.irancell.get_status.*` exists in `config.yml.example` and `config.yml`, with `enabled: false` as the default (OFF is the behavior of today).
- Each status gives: `success` and `failed` reported and saved, `pending` and an error change nothing, a final status after the reverse deadline saved and not reported.
- Defects 1 to 5 above are fixed, each with a test that fails on bill's behavior.
- Every error path writes one error line (a test for each path).
- The metric of fixed statuses for each run exists and a test checks its value after a run with known results (also a run with 0).
- Unit tests with stubs; integration tests with real Postgres and Redis. Build, vet and tests (with `-race`) pass.
- `system.html`, `this_vs_bill.md` (a new item), the conventions doc (if § 10 changes), the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode, compose Postgres, Redis and Jaeger, the Irancell mock, a short interval (for example `10s`), and more pending Irancell orders than `chunk`. Expected: each pending order of the window gets its final status in one run, the Kafka messages match bill's bytes, the error lines show for a forced failure, the fixed-status metric shows the count of the run, and one trace for each tick shows in Jaeger.
