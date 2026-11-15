<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-15 — Created (operator request): part 2 of supertask 043.
-->
# 045_parsian_status_worker — the Parsian status worker and the shared worker runner

Status: pending

## Description

Part 2 of supertask 043 (`043_tel_payment.md`; read it first, with operator decisions A and C). Needs task 044.

**The shared worker runner (built once here, used by task 046 and each later worker).** As `bill` (`pkg/cronTask/cron.go`, `pkg/lock/lock.go`):
- Each worker has a name, an interval (a config key, positive), and runs again at each interval.
- **One instance at a time.** Before a run, the instance takes a Redis lock: `SET <prefix><key> NX` with a TTL. An instance with no lock skips the run (no error answer). The TTL grows with each chunk of orders (`bill`: `ChargeTTL`), so a long run keeps the lock.
- **The key prefix.** Every Redis key of Qabz (the lock keys too) starts with `database.redis.prefix` (supertask 043). The key name follows `bill` (`<service.name>:crons_<name>_lock1`) or a new name; the design decides.
- A stop of the App cancels the run and waits for it with a limit (`bill`: 30 s).
- One panic in a run does not stop the App or the other workers.
- Improvement for the design step: `bill`'s `Release` deletes the key also when the lock is no longer the lock of this instance. Qabz deletes only its own lock (a value per run, checked before the delete).

**The Parsian status worker.** As `bill` (`pkg/cronTask/parsianGetStatus.go`, `service/billPaymentInquiry/dto.go` `InquiryCron`):
- Reads the orders of the provider `parsian` with the last status `pending` or `in-progress`, created between `end` (10 h) and `start` (30 min) ago, in chunks (`chunk`, 100).
- Asks parsian-v2 for the status of each order (`InternalInquiryService/GetStatus`).
- `success` or `failed`: saves the final status and reports it to the payment service (Kafka in `bill`; operator decision C). The payment service refunds a `failed` order.
- Each other answer: nothing changes; the next run asks again.
- A final status that comes after the reverse deadline: saved with the message `reverse_deadline_exceeded`, not reported (as `bill`).
- After 10 h with no final status: operator decision A.
- A Parsian `success` found by the worker does not start the Karashop settlement in `bill` (the line is a comment). The worker of task 046 takes the order later. The design step keeps this or asks the operator.

**Config keys** (names for the design step, conventions § 8): the interval (`bill`: `every: 15m`), `start` (`30m`), `end` (`10h`), `chunk` (`100`), the lock time (`30m`), the reverse deadline, and an enable switch.

**Mock.** The Parsian mock answers the status `success`, `failed` and `pending` (conventions § 15; the same change as task 044).

## Definition of Done

- The shared worker runner exists once, with no Parsian rule inside it; task 046 adds a worker, not a copy.
- With two or more instances, only one runs the worker at a time; every Redis key starts with `database.redis.prefix`.
- The worker runs at the interval of the config. The interval, the window, the chunk and the lock time are config keys in `config.yml.example`.
- Each Parsian status gives the result above: `success` and `failed` saved and reported, `pending` unchanged, an order outside the window not read, the reverse deadline as `bill`, and the 10 h rule of operator decision A.
- Metrics and log lines (conventions §§ 10, 12): runs by worker and result (`done`, `no_lock`, `error`), orders by status, run duration in ms (§ 9).
- Unit tests with stubs (Redis, Parsian, the order store, the reporter); integration tests with real Postgres and real Redis: two runners at the same time, one gets the lock; the prefix is on each key.
- Build, vet and tests (with `-race`) pass. `config.yml.example`, `system.html`, `this_vs_bill.md`, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode, compose Postgres and Redis, the Parsian mock, a short interval (for example `10s`).

1. Make three tel orders that end `pending` (task 044, the Parsian mock at `pending`). Set the mock to `success` and wait one interval. Pass: the orders are `success`; one report per order; the metrics count them.
2. The same with the mock at `failed`. Pass: the orders are `failed` and reported, so the payment service can refund.
3. The mock at `pending`. Pass: no row changes; the next run asks again.
4. An order older than `end` (change `created_at` in the dev database). Pass: the worker does not read it; the result of operator decision A.
5. Start two App instances. Pass: in each interval only one instance logs the run; the other logs or counts `no_lock`; `redis-cli KEYS '<prefix>*'` shows the lock key with the prefix.
6. Stop the App during a run. Pass: the run stops; the App exits in the time limit.
