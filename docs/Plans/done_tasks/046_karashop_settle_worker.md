<!-- VERSION: 1.0 -->
<!--
CHANGE LOG
v1.0 — 2026-11-15 — Done: implemented as designed; design change D13 (operator: "Qabz must not write NULL. It must write false", then "No NULL, if bill saves FALSE, then should Qabz"): the order insert writes is_raw and provider_confirmed as false; the implementation record and the feature-ON evidence added; moved to done_tasks.
v0.3 — 2026-11-15 — Started (operator: "Do task 46"): moved to in-progress_tasks; design written (corrections C1 to C4, D1 to D12; operator approval 2026-11-15 "Approve, all recommended": D3 (a), D5 (a), D6 the recommended defaults, D9 (a)).
v0.2 — 2026-11-15 — Operator decision B recorded: no reject, as bill.
v0.1 — 2026-11-15 — Created (operator request): part 3 of supertask 043.
-->
# 046_karashop_settle_worker — the Karashop settlement worker

Status: done (2026-11-15; the operator QA confirmation is open in `OPEN_TASKS.md`)

## Description

Part 3 of supertask 043 (`043_tel_payment.md`; read it first, with operator decision B of 2026-11-15). Needs task 044 (the settlement call) and task 045 (the shared worker runner).

As `bill` (`pkg/cronTask/paymentConfirm.go`, `service/billPayment/confirmPayment.go` `ConfirmPaymentBunch`):
- The worker runs on the shared runner of task 045: one instance at a time (Redis lock with `database.redis.prefix`), the interval in the config.
- Reads the orders with the last status `success`, `is_raw = false`, `provider_confirmed = false`, bill type `tel` and `telmidterm`, in the window of the config (`start`, `end`), in chunks (`chunk`).
- Sends the payment info to Karashop for each order (the same call as task 044: phone, base price, bill ID, pay ID, the Shamsi date and the time of `created_at`, RRN, payment provider).
- A Karashop success: `provider_confirmed = true`. A failure: nothing changes; the next run tries again.
- No reject (operator decision B, as `bill`): the worker tries again at each run until the order is outside the window. After the window the order stays `provider_confirmed = false`; nothing more happens.

**Config keys** (names for the design step, conventions § 8; `bill`: `confirmPayment.telephone`): the interval (`every`), the lock time, `start`, `end`, `chunk`, and an enable switch. `bill`'s example config has no values for them; the design step asks the operator for the defaults.

**Mock.** The Karashop settlement mock answers `success` and `error` (conventions § 15; the same mock as task 044).

## Definition of Done

- The worker uses the shared runner of task 045 (no copy of the runner).
- With two or more instances, only one runs the worker at a time.
- The worker runs at the interval of the config. The interval, the window and the chunk are config keys in `config.yml.example`.
- A Karashop success sets `provider_confirmed = true`; a failure leaves it `false` and the next run tries again; an order outside the window is not sent again (no reject).
- Metrics and log lines (conventions §§ 10, 12): settlements by bill type and result.
- Unit tests with stubs; integration tests with real Postgres and real Redis.
- Build, vet and tests (with `-race`) pass. `config.yml.example`, `system.html`, `this_vs_bill.md`, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode, compose Postgres and Redis, the Karashop mock, a short interval (for example `10s`).

1. Make tel payments that end `success` with `provider_confirmed = false` (task 044, the Karashop mock at `error`). Set the mock to `success` and wait one interval. Pass: `provider_confirmed = true` for each order; the metric counts them.
2. The mock at `error`. Pass: no row changes; the next run tries again; one error line per order.
3. An order outside the window, a raw order, an order with `provider_confirmed = true`. Pass: the worker does not send them.
4. An order that is still not settled when it is outside the window (change `created_at` in the dev database). Pass: the worker does not send it; `provider_confirmed` stays `false`; no reject.
5. Start two App instances. Pass: in each interval only one instance runs the worker.

## Design (implementer, 2026-11-15; operator approval 2026-11-15: "Approve, all recommended")

Read on 2026-11-15 from `bill` commit `ff98e25` (not run) and from the Qabz source at `92b7498`. Items marked **(decision)** need an operator choice. Every other item follows `bill`, or the task 044 and 045 code. Each difference from `bill` that a client, the payment service or the database can see gets an item in `docs/this_vs_bill.md` and a `@todo` at the code.

### Corrections of this task file

- **C1. bill's run never ends while one order fails.** `ConfirmPaymentBunch` reads the same query again after each page, with no offset (`LIMIT chunk`, no `OFFSET`). A confirmed order leaves the list; a failed one stays. So one order that Karashop does not confirm (or one with an empty `data`, which bill skips) is read again at once, sent to Karashop again, and the loop never ends. `ChargeTTL` extends the lock at each page, so the lock never ends either. The window is computed once before the loop, so the order also never leaves the window in that run. See D3.
- **C2. bill's query takes the success rows, not the newest row.** `SELECT DISTINCT ON (order_id) * FROM orders WHERE created_at > now-end AND created_at < now-start AND bill_type = ? AND status = 'success' AND is_raw is false AND provider_confirmed is false ORDER BY order_id, created_at DESC LIMIT ?`. The filter is on each row before `DISTINCT ON`, so the result is the newest success row of each order. A success is final (task 045 `InsertFinal`), so this is the same as "the last status `success`". The bounds are strict (`>`, `<`). `is_raw IS false` and `provider_confirmed IS false` do not take a NULL.
- **C3. bill's `UpdateProviderConfirm` sets only the success rows of the last 48 h.** With an `end` greater than 48 h, a success row older than 48 h is sent to Karashop at each run and is never marked. See D6.
- **C4. bill sends the two bill types at the same time.** `paymentConfirm.go` runs `tel` and `telmidterm` in two goroutines with one extender. The lock has no sync operations (`GetSyncOperations` gives nil), so its TTL is bill's default: 4 h + 5 min, and each page charges 2 min × the orders of the page. `lockTime` is only logged (as task 045 C3).

### The worker (`src/core/reconcile/settle.go`, bill: `ConfirmPaymentBunch`)

- **D1. The steps of one run.**
  1. The window is computed once: from `now − end` to `now − start` (bill).
  2. For each bill type, `tel` first, then `telmidterm` (one after the other, not at the same time as bill; neither the client, the payment service nor the database sees the difference):
     1. Read one page (D3). A failed read ends the run with `error`.
     2. No row: the next bill type.
     3. Charge the lock for the page (the Extender of task 045; a lost lock ends the run).
     4. Settle each order (D2), with the context of the run (bill: `context.Background()`), so the App stop ends the call.
  3. The run ends with `done`.
- **D2. One settlement is the code of task 044, not a copy.** The settlement part of `order.Payments.settle` (`src/core/order/complete.go`: the route, the `no_data` check, the adapter, `Settle`, `ConfirmProvider`, the counter and the log lines) moves to a new exported type `order.Settler` with `Settle(ctx, o Order) string` (the result). `Payments.settle` and the worker both call it. The at-once settlement of task 044 does not change (the same calls, lines and metrics), and its tests stay green. The routes stay in `app/defaults.go` (`tel`, `telmidterm` × `parsian`, `eways`, `eways2` → `karashop`). An order with no route: the result `no_route`, no call (bill: `ConfirmPaymentFactory` error, a span log only).
- **D3. The pages (decision).** Choose one:
  - **(a, recommended)** Each order at most once in a run. New `OrderRepository.ConfirmCandidates(ctx, billType, after, limit, from, to)`: the query of C2 (the same filters, the same strict bounds, the newest success row of each order, `ORDER BY order_id`), with `AND order_id > $after` (keyset paging; the first page has the nil UUID). The next page starts after the last `order_id` of the page. A failed order is tried again at the next run (task text). A `this_vs_bill.md` item.
  - **(b)** As bill: the same query with no offset again after each page. A failed order is sent again at once, and the run never ends while one order fails (C1).
- **D4. The row of the query is the order of the settlement** (bill: `ord`): the phone (`data`), `base_price`, `bill_id`, `pay_id`, `created_at` of the success row (the Shamsi date and the time), `rrn`, `provider_name`. The same request as task 044.

### Config (`src/config/config.go`, both config files)

- **D5. The place of the block (decision).** bill: `confirmPayment.telephone.*` (top level). Choose one:
  - **(a, recommended)** `confirm_payment.telephone.*` (bill's path in snake case; it is not a provider, as the `kafka` block of task 045 D11).
  - **(b)** `providers.external.karashop.confirm_payment.*` (next to the Karashop adapter).
- **D6. The keys and the defaults (decision on the values).** bill's example config has no values (task text). The start check runs only with `enabled: true`.

  | Key | Recommended default | Read / check |
  |---|---|---|
  | `enabled` (new) | `false` | `false` = no worker (rollback guarantee: OFF is the behavior of today) |
  | `name` | `telephone-confirmpayment` | the lock key `qabz:crons_<name>_lock1`, the metric label `cron`, the log lines; not empty |
  | `every` | `15m` | the interval; positive |
  | `lock_time` | `30m` | only logged, as bill (C4) |
  | `start` | `5m` | the window; 0 or more. 5 min: the at-once settlement of task 044 ends in 30 s (the Karashop timeout), so the worker does not send an order while the at-once call still runs |
  | `end` | `24h` | greater than `start`, and **not greater than 48 h** (C3: else an old order is sent at each run and never marked). The 48 h check is a Qabz rule (the App does not start) |
  | `chunk` | `100` | positive |

  The lock TTL is bill's default (4 h + 5 min, 2 min per order; C4), because the worker has no sync operations, as bill.

### Wiring, metrics, logs, traces

- **D7. One runner for every worker.** `app/crons.go`: the runner of task 045 gets the list of the enabled workers (the Parsian status worker, the settlement worker). The Kafka producer is made only when the Parsian worker is on. With both workers off: no runner (as today). The `lock_time` of each worker is logged by its name.
- **D8. The mock.** The existing key `debug.mocks.karashop.settle_return_type` (`success`, `error`; task 044). No new key.
- **D9. Metrics (conventions § 12) (decision).**
  - **(a, recommended)** `qabz_order_settle_total{bill_type,provider,result}` (task 044) gets the label `source`: `payment` (the at-once settlement) or `worker`. The results: `success`, `failed`, `pending`, `error`, `confirm_error`, `no_data` and the new `no_route`. Each order of a run is also counted in `qabz_cron_orders_processed_total{cron,result}` (task 045), and each run in `qabz_cron_runs_total` and `qabz_cron_duration_ms`.
  - **(b)** No change to the task 044 metric; a new metric `qabz_cron_settle_total{cron,bill_type,provider,result}`.
- **D10. Log lines.** The lines of task 044 (`settle: karashop failed`, error, with `order_id`, `status`, `code`, `message`; `settle: karashop confirmed`, info) with a new key `source` (`payment`, `worker`). A failed page read ends the run: one `cron: run failed` line (task 045). One info line at the end of each bill type with the count of orders (bill: `confirm bunch is done successfully`).
- **D11. Traces (conventions § 17; task 047 is not done, so this task adds the spans of its own code).**
  - The root span `cron <name>` for each run that took the lock, in `Runner.tick` (§ 17: a cron tick starts its own root). This is the shared runner, so the Parsian status worker gets its root span too; its child spans stay for task 047.
  - `reconcile.SettleWorker.Run` is not a separate span (the root covers it). A child span `order.Settler.Settle` for each order, with the attributes `order.id`, `bill_type`, `provider` and `result` (§ 17: no phone, no bill ID, no amount), and `codes.Error` for every result that is not `success`.
  - The Postgres, Redis and Karashop HTTP spans come from the shared hooks of task 047.
  - The at-once settlement of task 044 calls the same `Settle`, so it also gets the span, as a root (the request has no span until task 047; the link to the request span is added by task 047, § 17).
  - A test with `tracetest` checks the tree (root, one child per order, the error status, no forbidden attribute). New vendored package: `go.opentelemetry.io/otel/sdk/trace/tracetest` (the `sdk` module is a dependency already).
- **D12. The differences from bill** (one `this_vs_bill.md` item 45): the pages (D3), the bill types one after the other (D1), the context of the run and the Karashop timeout (bill: `context.Background()`, the same adapter timeout), the lock deleted after each run (as item 43), the 48 h check (D6), the config path and the switch, the log lines and metrics. Operator decision B (no reject) is the same as bill: no item.

### Design change (implementation, 2026-11-15)

- **D13. The stored `false` (operator, 2026-11-15).** The live test found that bill's query never finds an order. bill writes DEFAULT for a `false` (go-pg `NullZero`, `insert.go`: zero value → `DEFAULT`): `is_raw` gets `FALSE` (bill migration 21), but `provider_confirmed` has no column default (bill migration 24), so it is NULL, and `provider_confirmed is false` does not match a NULL. The Qabz insert (task 044) wrote NULL for both. Operator: "Qabz must not write NULL. It must write false", and "No NULL, if bill saves FALSE, then should Qabz". The insert now writes both columns as they are (`false` too). The query is bill's query. Rows written before task 046 keep NULL (follow-up (b) in `OPEN_TASKS.md`). A `this_vs_bill.md` item 45 point.

### Files (planned)

`src/core/reconcile/settle.go` (new) and its test, `src/core/order/complete.go` (`Settler`), `src/system/postgres/order_repository.go` (`ConfirmCandidates`), `src/entry/cron/cron.go` (the root span), `src/config/config.go`, `src/app/crons.go`, `src/app/defaults.go`, the tests of these packages and the live tests (real Postgres and Redis), `src/go.mod` / `vendor` (tracetest; `vendor/` is not committed), `config.yml`, `config.yml.example`, `docs/initial-discovery/system.html`, `docs/this_vs_bill.md`, `CHANGELOG.md`, `docs/documents_manifest.md`, `docs/Plans/OPEN_TASKS.md`, this task file.

## Implementation (2026-11-15)

Implemented as designed (D1 to D12, decisions D3 (a), D5 (a), D6 the recommended defaults, D9 (a)) with design change D13. What the design did not name, and how it was done:

- **The Settler and the routes.** `Payments` keeps the task 044 rule "an order with no settlement route is not settled and not counted": it calls `Settler.Route` first. The worker calls `Settle` directly, so it counts `no_route`. The routes are one function `settleRoutes` in `app/defaults.go` for both.
- **The metric.** `qabz_order_settle_total` is registered once with `registerOrExisting` by the payment step and by the worker metrics (the same description, the label `source`).
- **A short page ends a bill type.** A page with fewer than `chunk` orders is the last one (one query less than bill). A page whose last `order_id` does not move forward ends the run with an error (a guard, never a loop).
- **The root span** is in `Runner.tick` (`trace.WithNewRoot`), so the Parsian status worker gets its root span too. `go.opentelemetry.io/otel/trace` is now a direct dependency; `go.opentelemetry.io/otel/sdk/trace/tracetest` is used by tests only (vendored; `vendor/` is not committed).

Verification:

- **Tests:** `make test GO_TEST_ARGS="-race -count=1 -cover"`: every package passes; `app` 87.6 %, `config` 97.3 %, `core/order` 93.9 %, `core/reconcile` 98.3 %, `entry/cron` 98.1 %, `system/postgres` 90.4 %. The live tests ran (not skipped). Positive controls: bill's paging (the page after `""` each time) makes `TestSettleRunOncePerRun` time out; no error status fails `TestSettlerResults` (8 assertions); a `phone` attribute fails it (`attribute phone is not allowed`); with `is_raw` NULL `TestLiveConfirmCandidates` found 0 orders.
- **Feature ON (Rule 15),** the compose Postgres and Redis, the Karashop mock, `every: 10s`, App copies on other ports (the operator's debug App on 8080 / 8082 / 9090 was not touched); six success rows in the dev database (A `tel` 1 h, B `telmidterm` 2 h, C raw, D confirmed, E 30 h, F 2 min):
  - Mock `error`, 25 s: three runs; each run sent A and B once (one `settle: karashop failed` line each, `source` `worker`) and logged `settle worker: bill type done` with 1 order per bill type; no row changed; C, D, E and F never sent (steps 2, 3, 4).
  - Mock `success`, two instances: at the first tick instance 2 ran and instance 1 counted `no_lock`; at the second tick instance 1 ran and found 0 orders; each instance `done` 1 and `no_lock` 1 (step 5); A and B `provider_confirmed = true`, `qabz_order_settle_total{bill_type="tel"|"telmidterm",provider="karashop",result="success",source="worker"}` 1 each, `qabz_cron_orders_processed_total{result="success"}` 2 (step 1).
  - The QA rows were deleted from the dev database after the run.
