<!-- VERSION: 0.2 -->
<!--
CHANGE LOG
v0.2 — 2026-11-15 — Operator decisions A (nothing after 10 h, as bill) and C (a Kafka producer; the message of bill byte for byte; every kafka config key of bill, also the unused ones) recorded.
v0.1 — 2026-11-15 — Created (operator request): part 2 of supertask 043.
-->
# 045_parsian_status_worker — the Parsian status worker and the shared worker runner

Status: pending

## Description

Part 2 of supertask 043 (`043_tel_payment.md`; read it first, with operator decisions A and C of 2026-11-15). Needs task 044.

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
- `success` or `failed`: reports the status to the payment service on Kafka, then saves the final status (`bill` order: `verifyPaymentOrder`, then `NewOrderWithFinalStatus`; a failed publish saves nothing, so the next run asks again). The payment service refunds a `failed` order.
- Each other answer: nothing changes; the next run asks again.
- A final status that comes after the reverse deadline: saved with the message `reverse_deadline_exceeded`, not reported (as `bill`).
- After 10 h with no final status: nothing (operator decision A, as `bill`). The order is outside the window and stays pending.

**The Kafka client (operator decision C).** `src/system/kafka/` gets a producer, as `bill` (`client/broker/kafka_dto.go`, `github.com/segmentio/kafka-go`: a `kafka.Writer` on the brokers with the `LeastBytes` balancer). A new module needs the Go modules rule (conventions § 2) and the standard library first rule (§ 5).

**The Kafka config: every key of `bill`, also the keys that are not used (operator, 2026-11-15).** The `kafka` block of `bill` (`config.example.yaml`, `config.Kafka`) is copied key by key. The Qabz names follow conventions § 8 (snake case; the design step fixes the place of the block, for example `kafka.*` or `broker.kafka.*`). The defaults are the values of `bill`'s example config:

| `bill` key | Default | Used by Qabz |
|---|---|---|
| `kafka.brokers` | `["localhost:9092"]` | yes: the writer address |
| `kafka.topic.payment.write.subject` | `paymentwriter` | yes: the topic of the status message |
| `kafka.topic.payment.write.group` | `paymentwriter` | no |
| `kafka.topic.payment.read.subject` | `payment` | no |
| `kafka.topic.payment.read.group` | `paymentBill` | no |
| `kafka.username` | `""` | no |
| `kafka.password` | `""` (the `Secret` type, § 8) | no |
| `kafka.auth` | `false` | no |
| `kafka.timeout` | `5s` | no |

A key that is not used has a comment `not read yet (bill has it and does not use it)` in `config.yml.example`, as the other keys of that kind (for example `providers.internal.irancell.nats_subject`). Both `config.yml` and `config.yml.example` get the block (§ 8).

**The message MUST NOT differ from `bill` (operator, 2026-11-15).** `bill`: `service/billPaymentInquiry/dto.go` `verifyPaymentOrder`, `domain/entity/payment.go`:
- Key: the bytes `bill` (`kafka_dto.go` sets the key `bill` on every message). A different key (for example `qabz`) needs an operator decision.
- No headers. The value is `encoding/json` of `entity.Payment`, with the fields in this order and only `order_id`, `status` and `step_status.service` set:
  `{"id":"","current_step":"","payload":{"order_id":"<order ID>","user_id":"","payment_id":"","service_id":0,"price":0,"rrn":""},"status":"<success|failed>","step_status":{"payment":"","service":"<success|failed>"},"type":"","version":0,"data":{}}`
- A test compares the bytes of the Qabz message with the bytes of `bill`'s `json.Marshal` (differential test, task 023 practice).
- `bill`'s writer does not use `kafka.username`, `kafka.password`, `kafka.auth` and `kafka.timeout`; Qabz has the keys but does not use them either (the same behavior as `bill`).
- A Parsian `success` found by the worker does not start the Karashop settlement in `bill` (the line is a comment). The worker of task 046 takes the order later. The design step keeps this or asks the operator.

**Config keys** (names for the design step, conventions § 8): the interval (`bill`: `every: 15m`), `start` (`30m`), `end` (`10h`), `chunk` (`100`), the lock time (`30m`), the reverse deadline, and an enable switch.

**Mock.** The Parsian mock answers the status `success`, `failed` and `pending` (conventions § 15; the same change as task 044).

## Definition of Done

- The shared worker runner exists once, with no Parsian rule inside it; task 046 adds a worker, not a copy.
- With two or more instances, only one runs the worker at a time; every Redis key starts with `database.redis.prefix`.
- The worker runs at the interval of the config. The interval, the window, the chunk and the lock time are config keys in `config.yml.example`.
- Each Parsian status gives the result above: `success` and `failed` reported and saved, `pending` unchanged, an order outside the window (older than 10 h) not read, the reverse deadline as `bill`.
- The Kafka producer exists in `src/system/kafka/`; the config has every `kafka` key of `bill` with `bill`'s defaults (the producer reads only the brokers and the write topic); the message key and value are byte for byte the message of `bill` (a test proves it).
- Metrics and log lines (conventions §§ 10, 12): runs by worker and result (`done`, `no_lock`, `error`), orders by status, run duration in ms (§ 9).
- Unit tests with stubs (Redis, Parsian, the order store, the reporter); integration tests with real Postgres and real Redis: two runners at the same time, one gets the lock; the prefix is on each key.
- Build, vet and tests (with `-race`) pass. `config.yml.example`, `system.html`, `this_vs_bill.md`, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode, compose Postgres and Redis, the Parsian mock, a short interval (for example `10s`).

1. Make three tel orders that end `pending` (task 044, the Parsian mock at `pending`). Set the mock to `success` and wait one interval. Pass: the orders are `success`; one Kafka message per order on the topic of the config (read with a console consumer), with the key `bill` and the value of `bill` (status `success`); the metrics count them.
2. The same with the mock at `failed`. Pass: the orders are `failed`; one message per order with status `failed`, so the payment service can refund.
3. The mock at `pending`. Pass: no row changes and no message; the next run asks again.
4. An order older than `end` (change `created_at` in the dev database). Pass: the worker does not read it; it stays pending; no message.
5. Kafka stopped, the mock at `success`. Pass: no row changes; one error line; after Kafka starts again, the next run reports and saves the order.
6. Start two App instances. Pass: in each interval only one instance logs the run; the other logs or counts `no_lock`; `redis-cli KEYS '<prefix>*'` shows the lock key with the prefix.
7. Stop the App during a run. Pass: the run stops; the App exits in the time limit.
