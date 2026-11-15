<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-15 — Created (operator request): part 3 of supertask 043.
-->
# 046_karashop_settle_worker — the Karashop settlement worker

Status: pending

## Description

Part 3 of supertask 043 (`043_tel_payment.md`; read it first, with operator decision B). Needs task 044 (the settlement call) and task 045 (the shared worker runner).

As `bill` (`pkg/cronTask/paymentConfirm.go`, `service/billPayment/confirmPayment.go` `ConfirmPaymentBunch`):
- The worker runs on the shared runner of task 045: one instance at a time (Redis lock with `database.redis.prefix`), the interval in the config.
- Reads the orders with the last status `success`, `is_raw = false`, `provider_confirmed = false`, bill type `tel` and `telmidterm`, in the window of the config (`start`, `end`), in chunks (`chunk`).
- Sends the payment info to Karashop for each order (the same call as task 044: phone, base price, bill ID, pay ID, the Shamsi date and the time of `created_at`, RRN, payment provider).
- A Karashop success: `provider_confirmed = true`. A failure: nothing changes; the next run tries again.
- "Reject": operator decision B (`bill` has no reject; it tries until the order leaves the window).

**Config keys** (names for the design step, conventions § 8; `bill`: `confirmPayment.telephone`): the interval (`every`), the lock time, `start`, `end`, `chunk`, and an enable switch. `bill`'s example config has no values for them; the design step asks the operator for the defaults.

**Mock.** The Karashop settlement mock answers `success` and `error` (conventions § 15; the same mock as task 044).

## Definition of Done

- The worker uses the shared runner of task 045 (no copy of the runner).
- With two or more instances, only one runs the worker at a time.
- The worker runs at the interval of the config. The interval, the window and the chunk are config keys in `config.yml.example`.
- A Karashop success sets `provider_confirmed = true`; a failure leaves it `false` and the next run tries again; the result of operator decision B.
- Metrics and log lines (conventions §§ 10, 12): settlements by bill type and result.
- Unit tests with stubs; integration tests with real Postgres and real Redis.
- Build, vet and tests (with `-race`) pass. `config.yml.example`, `system.html`, `this_vs_bill.md`, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode, compose Postgres and Redis, the Karashop mock, a short interval (for example `10s`).

1. Make tel payments that end `success` with `provider_confirmed = false` (task 044, the Karashop mock at `error`). Set the mock to `success` and wait one interval. Pass: `provider_confirmed = true` for each order; the metric counts them.
2. The mock at `error`. Pass: no row changes; the next run tries again; one error line per order.
3. An order outside the window, a raw order, an order with `provider_confirmed = true`. Pass: the worker does not send them.
4. The result of operator decision B (for example after N tries or after the window).
5. Start two App instances. Pass: in each interval only one instance runs the worker.
