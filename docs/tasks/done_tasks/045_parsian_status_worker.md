<!-- VERSION: 1.0 -->
<!--
CHANGE LOG
v1.0 — 2026-11-15 — Done (operator: "Yes, implement"): implemented as designed; the implementation record and the feature-ON evidence added; moved to done_tasks.
v0.6 — 2026-11-15 — Design change (operator: "Yes, fix it here", then "Add it to task 045"): D17, the currentStep of the OrderComplete answer (task 044) becomes the ecosystem name; kafka.service_name is replaced by one key service.ecosystem_name (default "bill") for every name that Qabz sends to another service (D10, D10a, D11, D17).
v0.5 — 2026-11-15 — Design change (operator: "Use partitioner like bill service", and the name "bill" from a config wherever a message needs a service name): Qabz picks the partition with bill's LeastBytes rule (D10b); new key kafka.service_name, default "bill", for the message key and the Kafka client ID (D10, D11); D10a updated.
v0.4 — 2026-11-15 — Design change (operator: "Use the official confluent pkg and not segmentio's"): the Kafka producer uses confluent-kafka-go v2 (D10, D10a, C5, the description), not segmentio/kafka-go; probe of the vendored cgo build recorded.
v0.3 — 2026-11-15 — Started (operator: "start task 45"): moved to in-progress_tasks; design written (corrections C1 to C6, D1 to D16; operator approval 2026-11-15 "Approve, all recommended": D4 (b), D12 (a), D14 (a)).
v0.2 — 2026-11-15 — Operator decisions A (nothing after 10 h, as bill) and C (a Kafka producer; the message of bill byte for byte; every kafka config key of bill, also the unused ones) recorded.
v0.1 — 2026-11-15 — Created (operator request): part 2 of supertask 043.
-->
# 045_parsian_status_worker — the Parsian status worker and the shared worker runner

Status: done (2026-11-15; the operator QA confirmation is open in `OPEN_TASKS.md`)

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

**The Kafka client (operator decision C).** `src/system/kafka/` gets a producer. `bill` uses `github.com/segmentio/kafka-go` (`client/broker/kafka_dto.go`: a `kafka.Writer` on the brokers with the `LeastBytes` balancer); Qabz uses the official Confluent client `github.com/confluentinc/confluent-kafka-go/v2` (operator, 2026-11-15; design D10). A new module needs the Go modules rule (conventions § 2) and the standard library first rule (§ 5).

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
- `service.ecosystem_name` (default `bill`) is the Kafka message key, the Kafka client ID and the `currentStep` of the `OrderComplete` answer (design D17).
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

## Design (implementer, 2026-11-15; operator approval 2026-11-15: "Approve, all recommended")

Read on 2026-11-15 from `bill` commit `ff98e25` (not run) and from the Qabz source at `a29cd50`. Items marked **(decision)** need an operator choice. Every other item follows `bill`. Each difference from `bill` that the payment service or the database can see gets an item in `docs/this_vs_bill.md` and a `@todo` at the code.

### Corrections of this task file

- **C1. The Parsian RPC is `TopBill/OrderInquiry`** (`pb/proto/parsian-v2/internalBillService.proto`, `OrderInquiryRequest{order_id, bill_type}`, `OrderInquiryResponse{status, message, provider_trace_id}`), not `InternalInquiryService/GetStatus` (`bill`: `gateway/paymentInquiry/parsian.go`). A gRPC error (a timeout too) is `pending`. The `status` text of the answer is used as it is; only `success` and `failed` are final.
- **C2. The saved final row is a copy of the newest row with a new status only.** `bill` sets `ord.Status = resp.Status` on the newest row and inserts it again (new `uuid`, new `created_at`). The `message`, `ref_id` and `status_code` of the newest row stay. The `message` and `provider_trace_id` of the Parsian answer are not saved.
- **C3. `bill` does not use `lockTime` for the lock, and never deletes the lock.** The TTL is `sum(syncOperations) × 1.1 × chunk + 5 min` (`pkg/lock/lock.go`; `lockTime` is only logged). `wrapTask` never calls `Release`, so the key stays until the TTL ends. With the values of D2 the TTL is 60 min 0.66 s. The ticks at 15, 30 and 45 min find the lock. The tick at 60 min comes 0.66 s before the TTL ends, so it finds the lock too, unless it is late. The worker runs about every 75 min (on all instances together), not every 15 min. See D4.
- **C4. The `bill` example config does not fill the worker.** `config.example.yaml` has the keys at `parsian.*` (`every`, `start`, `end`, `chunk`, `lockTime`, `syncOperations`), but `bill` reads `parsian.getStatus.*` (viper, `config.GetStatusConfig`). With that file the worker has `CronTime` 0 and is not scheduled (`invalid cron time`). The real values are in the deploy config of `bill`, which is not in the repository. The local `config.yml` (the operator's values) has the full block; D2 uses those values.
- **C5. The `bill` Kafka write waits about 1 s for each message.** `bill`'s `kafka.Writer` sets only `Addr` and `Balancer`. The kafka-go defaults apply: `BatchTimeout` 1 s, `RequiredAcks` none, synchronous `WriteMessages`. One message is sent when the batch time ends, so a chunk with 100 final answers takes about 100 s in `bill`. Qabz uses the Confluent client (D10), so it does not have this timing.
- **C6. The worker reads each order again until the order leaves the window.** `GetAllPending` filters the rows on `created_at` before it takes the newest row. A final row saved by the worker is newer than `start`, so it is outside the window, and the order is read again as `pending` in each later run. The worker then reads the newest row (48 h), sees a final status and skips it (no Parsian call). The `OFFSET` paging works because the list does not get shorter in a run. Qabz copies this.

### The shared worker runner

- **D1. Place and shape (`system.html`, "Cron Interface", "Directory Structure").**
  - `src/entry/cron/`: the `Cron` interface (`Name`, `Interval`, `Run(ctx, lock)`), the registry and the runner. No Parsian rule in it.
  - `src/system/redis/`: the lock (`Acquire`, `Extend`, `Release`) on the go-redis client, and the key prefix.
  - `src/core/reconcile/`: what the worker does (the Parsian status check). Task 046 adds its worker here and registers it in the runner.
  - The App starts the runner after the servers and stops it with the background jobs (before the database closes), as `order.Payments` (task 044).
  - Each worker runs in its own goroutine with a `time.Ticker` (standard library first, conventions § 5; no gocron). The first run is at the start, as gocron v1 (`StartImmediately`), then one run at each interval. A run that takes longer than the interval skips the ticks in between (one run at a time in one instance).
  - A panic in a run is recovered: one error line with the stack, the result `error`, and the next tick runs again (`bill`: `wrapTask`).
  - The App stop cancels the context of the run and waits for it until the stop deadline of the App (`stopTimeout`, 10 s). `bill` waits 30 s for its crons. The Parsian call and the database calls end at the cancel, so 10 s is enough. Not a `this_vs_bill.md` item (no client, payment service or database sees it).
- **D2. Config of the Parsian worker.** The keys exist since task 036 (`providers.internal.parsian.get_status.*`, "not read yet"). They become read, with the values of the local `config.yml` as the new defaults (C4):

  | Key | Default | Read |
  |---|---|---|
  | `get_status.enabled` (new) | `false` | yes: `false` = no worker (rollback guarantee: OFF is the behavior of today) |
  | `get_status.name` | `parsian-getstatus` | yes: the lock key, the metric label `cron` and the log lines |
  | `get_status.cron_time` | `15m` | yes: the interval; positive when `enabled` (else the App does not start) |
  | `get_status.start` | `30m` | yes |
  | `get_status.end` | `10h` | yes; greater than `start` |
  | `get_status.chunk` | `100` | yes; positive |
  | `get_status.reverse_deadline` | `10h` | yes |
  | `get_status.sync_operations.*` | `30s`, `3ms`, `3ms` | yes: the lock TTL (D4) |
  | `get_status.lock_time` | `30m` | logged only, as `bill` (C3) |
  | `get_status.is_lockable` | `true` | not read: Qabz always takes the lock (supertask 043, multi-instance) |

  With `reverse_deadline` = `end` (10 h), the `reverse_deadline_exceeded` path is almost never reached. A `reverse_deadline` of 0 makes every final answer late (`bill`). The start check accepts 0, as `bill`.
- **D3. The Redis key prefix.** New key `database.redis.prefix`, default `qabz:`. The `system/redis` client puts it in front of every key name, so no caller can forget it. Today the only Redis keys of Qabz are the lock keys. The lock key is `<prefix>crons_<name>_lock1` (`bill`: `<service.name>:crons_<name>_lock1`), for example `qabz:crons_parsian-getstatus_lock1`.
- **D4. The lock TTL and the release (decision).** In all options, the lock is `SET <key> <token> NX PX <ttl>`. The token is 16 random bytes from `crypto/rand` for each run. An extend or a delete runs only when the key still has the token of the run (a Lua script: check, then `PEXPIRE` or `DEL`). An instance with no lock counts `no_lock` and writes no error line. The extend after each chunk is `bill`'s `ChargeTTL`: when the TTL left is less than `base × orders in the chunk`, the TTL becomes that value. A lost lock stops the run with `error`. Choose one:
  - **(a)** The same as `bill`: TTL = `sum(sync_operations) × 1.1 × chunk + 5 min`, and no delete after the run. The worker runs about every 75 min (C3).
  - **(b, recommended)** The TTL of `bill`, and Qabz deletes its own lock at the end of each run (the task text: "Qabz deletes only its own lock"). The worker runs at each interval (15 min). The payment service gets the final statuses up to 60 min earlier than from `bill`. This is a `this_vs_bill.md` item.
  - **(c)** As (b), but the start TTL is `lock_time` (30 min) in place of the `sync_operations` formula.
- **D5. Errors of the runner.** A failed Redis call when the lock is taken: the result `error`, one error line (conventions § 10: a failed background job), no run. A failed delete: one error line; the key ends with its TTL.

### The Parsian status worker (`src/core/reconcile/`, `bill`: `InquiryCron`)

- **D6. The steps of one run.**
  1. `start` and `end` give the window (`now − end` to `now − start`).
  2. Read one chunk of order IDs (`bill`'s `GetAllPending` SQL: rows of the provider `parsian` in the window, the newest row of each order, status `pending` or `in-progress`, `ORDER BY created_at`, `LIMIT chunk OFFSET n`). The provider name is the `providers.name` `parsian` saved in `orders.provider_name`; no read of the `providers` table (conventions § 13). A failed read ends the run with `error`.
  3. No ID: the run ends with `done`. Else extend the lock (D4), and the offset grows by `chunk`.
  4. For each ID, with its own context (the run context, so the App stop ends it; `bill` uses `context.Background()`):
     - Read the newest row of the last 48 h (`OrderRepository.Latest`, task 044). A failed read: next ID.
     - The status is final (`success`, `failed`): next ID (C6).
     - Ask Parsian (D8). An answer that is not `success` or `failed`: next ID.
     - The newest row is older than `reverse_deadline`: save the copy with the new status and the message `reverse_deadline_exceeded` (`bill`: `NewOrder`, with no check), no report, next ID.
     - Report the status on Kafka (D10). A failed publish: next ID, nothing saved.
     - Save the copy with the new status (D7). A failed save: next ID. The message is already sent; the next run sees no final row, asks again and sends a second message (as `bill`).
  5. Go to step 2.
- **D7. The save of the final status (`bill`: `NewOrderWithFinalStatus`).** New `OrderRepository.InsertFinal`. In one transaction: read the newest row of the order of the last 24 h with `FOR UPDATE`; a final status there is an error (`order is already in final status`), nothing saved; else insert the copy (new `uuid`, `created_at` = now). The reverse-deadline save uses the existing `Insert`.
- **D8. The Parsian adapter `Status`.** `parsianproxy` implements `ProviderInterface.Status` (the stub types `StatusRequest` / `StatusResponse` get the fields `OrderID`, `BillType` and `Status`, `Message`, `ProviderTraceID`). The call is `TopBill/OrderInquiry` with `order_id` and `bill_type`, with the timeout `providers.internal.parsian.timeout` (10 s; `bill` sets none). A gRPC error (a timeout too) or `enabled: false` gives `pending` with the error text as the message. The adapter comes from the provider registry by the name `parsian` (`order.Adapters`, as in task 044).
- **D9. A Parsian `success` does not start the Karashop settlement** (as `bill`: the line is a comment). The worker of task 046 takes the order.

### Kafka (`src/system/kafka/`, operator decision C)

- **D10. The producer (design change 2026-11-15, operator: "Use the official confluent pkg and not segmentio's").** New module `github.com/confluentinc/confluent-kafka-go/v2` `v2.16.1` (librdkafka 2.16.0), after `go mod tidy && go mod vendor` (conventions § 2). A `kafka.Producer` with `bootstrap.servers` = `kafka.brokers` joined with commas, `client.id` = `service.ecosystem_name` (D11), and the librdkafka defaults for every other setting. It is created only when the worker is enabled. At the App stop: `Flush` until the stop deadline, then `Close`. `Publish(ctx, topic, value)` calls `Produce` with one message on the partition of D10b (the key `[]byte(service.ecosystem_name)`, `bill` by default, no headers, the value `json.Marshal` of the Qabz copy of `entity.Payment`: the same field order and tags; only `payload.order_id`, `status` and `step_status.service` set; `data` an empty map) and its own delivery channel. It waits for the delivery report or `ctx`. An error in the report, or `ctx` done, is a failed publish (D6: nothing saved). When `ctx` ends first, librdkafka can still deliver the message later; the next run then sends a second message for the order (the same result as a failed save in D6, as `bill`). A test compares the key and value bytes with the bytes of the `bill` struct marshaled in the test (differential test, task 023 practice). The message bytes are the same as `bill`'s; only the client is different.
- **D10a. The differences that come from the client.** Each one is a `this_vs_bill.md` item:
  - **Acks:** librdkafka waits for all in-sync replicas (`acks=all`); `bill` waits for no broker answer (`RequiredAcks` none). A Qabz publish is a success only when the brokers have the message.
  - **Client ID:** Qabz sends the client ID `service.ecosystem_name` (`bill`). `bill`'s writer sends an empty client ID (kafka-go `DefaultTransport`, no `ClientID`). librdkafka would send `rdkafka`. The operator rule: a service name in a message is `bill`, from the config, because Qabz is not known in the ecosystem yet.
  - **Timing:** no 1 s batch wait for each message (C5); the default `linger.ms` is 5 ms.
  - **Build:** the client uses cgo with the librdkafka static library in the module. Every build needs `CGO_ENABLED=1` and a C compiler (`-race` needs cgo too). A later Dockerfile (there is none now; `docker-compose.yml` names one) needs a glibc base image, or `-tags musl` on Alpine. Probe on 2026-11-15 (a scratch module, this Mac): `go mod vendor` copied the `librdkafka_*.a` files of darwin, glibc and musl into `vendor/`, the build passed, and `kafka.LibraryVersion()` printed `2.16.0`.
- **D10b. The partition: `bill`'s `LeastBytes` (operator, 2026-11-15: "Use partitioner like bill service").** librdkafka has no such partitioner, so Qabz sets `TopicPartition.Partition` of each message itself, with the rule of kafka-go `v0.4.31` `LeastBytes.Balance` (read in the module cache on 2026-11-15):
  - One counter list for each producer (all topics share it, as one `bill` writer). Each counter has a partition number and a byte count.
  - When the number of partitions differs from the number of counters, the list is made again with all counts 0, sorted by partition number.
  - The message goes to the counter with the fewest bytes; on a tie, the first one (the lowest partition number). Then that count grows by `len(key) + len(value)`.
  - The counts are in memory, for each instance, and start at 0 at each start (as `bill`).
  - The partition list comes from `Producer.GetMetadata` for the topic, kept for 6 s (the `MetadataTTL` default of the kafka-go `Transport`). A failed metadata read is a failed publish (D6: nothing saved).
  - A unit test checks the rule on a fixed sequence (ties, a change of the partition count) against the result of the kafka-go rule, written out in the test.
- **D11. The Kafka config.** A new top-level block `kafka` (the path of `bill`; it is not a provider, so not under `providers`; conventions § 8 "the area they configure"): `kafka.brokers` (`["localhost:9092"]`), `kafka.topic.payment.write.subject` (`paymentwriter`), and the unused keys `kafka.topic.payment.write.group`, `kafka.topic.payment.read.subject`, `kafka.topic.payment.read.group`, `kafka.username`, `kafka.password` (the `Secret` type), `kafka.auth`, `kafka.timeout`, with the defaults of the task table and the comment `not read yet (bill has it and does not use it)`. When the worker is enabled, `kafka.brokers` must not be empty and the write subject must not be empty (else the App does not start).
- **D12. Kafka for the verification (decision).** The verification reads the messages with a console consumer, and the compose file has no Kafka. Choose one:
  - **(a, recommended)** Add one single-node Kafka (KRaft, the image `apache/kafka`, a fixed version) to `docker-compose.yml` in the compose profile `kafka`, so `make compose up` does not start it; `make compose up ARGS="--profile kafka"` starts it. The port is `9092` on `localhost`.
  - **(b)** No compose change; the operator gives a Kafka for the verification.

### The ecosystem name

- **D17. One config key for the name that other services see (operator, 2026-11-15: a name in a message is `bill`, from a config; Qabz is not known in the ecosystem yet).** New key `service.ecosystem_name`, default `bill`, next to `service.name` and `service.id`. It must not be empty (else the App does not start). It is used for:
  - the Kafka message key and the Kafka client ID (D10, D10a);
  - `currentStep` of the `OrderComplete` answer (task 044 D9; `src/entry/grpc/payment.go` `PaymentComplete`, today `service.name`, so the payment service gets `qabz`). `bill` sets `currentStep` = its `service.name`, which is `bill` in the deployed service. With the default, the answer is the same as `bill`'s, so this is not a `this_vs_bill.md` item.

  `service.name` (`qabz`) stays the name of the log lines and the tracing. The order token `kid` keeps its own key (`providers.internal.payment_server.kid`, task 042; the local `config.yml` has `bill`); its empty-value fallback stays `service.name`. The test of task 044 that expects `currentStep` `qabz` changes to `bill`, and a test checks that `currentStep` follows the key. Verification: in step 1 of the task 044 scenario (`OrderComplete` with the mocks), the answer has `currentStep` `bill`; with `service.ecosystem_name: "x"`, it has `x`.

### Mocks, metrics, logs

- **D13. The Parsian mock (conventions § 15).** New key `debug.mocks.parsian.status_return_type`: `success`, `failed` or `pending` (`pending` = `provider.ErrProviderDown`, the answer of a timeout), default `success`. `provider.MockFixtures` gets `Status`. Only in dev mode, as the other mock keys.
- **D14. Metrics (conventions §§ 9, 12; the names that `system.html` plans) (decision on the label set only).**
  - `qabz_cron_runs_total{cron,result}`, `result`: `done`, `no_lock`, `error` (a panic too).
  - `qabz_cron_duration_ms{cron}`: each run that took the lock.
  - `qabz_cron_orders_processed_total{cron,result}`, `result`: `success`, `failed` (reported and saved), `pending` (no final answer), `late` (reverse deadline), `skipped` (final already), `report_error`, `save_error`, `read_error`.
  - `qabz_provider_calls_total` and `qabz_provider_call_duration_ms` with `action="status"` (the existing metrics).
  - `qabz_kafka_publish_total{result}` (`ok`, `error`) and `qabz_kafka_publish_duration_ms`.
  - The gauges `qabz_pending_orders` and `qabz_oldest_pending_order_age_ms` of `system.html` come later (they need a count query); the task says so. Choose: **(a, recommended)** this set; **(b)** also the two gauges in this task.
- **D15. Log lines.** `cron: started` (info, name, interval, lock time) for each worker at start. `cron: no lock` is not logged (counted). `cron: run failed` (error, `name`, `error`). `cron: panic` (error, `name`, `stack`). `cron: release failed` (error). `parsian status: report failed` (error, `order_id`). `parsian status: save failed` (error, `order_id`). `parsian status: reported` (info, `order_id`, `status`; `bill`: `bill has called payment to verify a pending order to <status> status`). `parsian status: reverse deadline exceeded` (info, `order_id`). A failed read of one order and a Parsian `pending` write no line (counted), as conventions § 10 (retried at the next run).

### Risk check (supertask 043)

- **D16. A separate database and Redis.** `database.postgres.db: qabz` (task 044 D17). The lock keys have the prefix `qabz:`, so they cannot meet the `bill` lock keys (`bill:crons_...`) when the two services share a Redis. Kafka: the `bill` worker and the Qabz worker write to the same topic with the same key; the payment service cannot tell them apart (as wanted, operator decision C).

### Files (planned)

`src/entry/cron/` (new), `src/core/reconcile/` (new), `src/system/redis/` (prefix, lock), `src/system/kafka/` (new), `src/system/postgres/order_repository.go` (`PendingIDs`, `InsertFinal`), `src/provider/provider.go`, `src/provider/mock.go`, `src/provider/parsianproxy/`, `src/config/config.go`, `src/app/` (wiring, metrics), `src/entry/grpc/payment.go` (D17), `src/go.mod`, `src/go.sum` (confluent-kafka-go v2), `config.yml`, `config.yml.example`, `docker-compose.yml` (D12 a), `docs/initial-discovery/system.html`, `docs/this_vs_bill.md`, `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (§ 15: the `status_return_type` key), `CHANGELOG.md`, `docs/documents_manifest.md`, `docs/Plans/OPEN_TASKS.md`, this task file.

## Implementation (2026-11-15)

Implemented as designed (D1 to D17, decisions D4 (b), D12 (a), D14 (a), the Confluent client, bill's partition rule, `service.ecosystem_name`). What the design did not name, and how it was done:

- **Redis in the live tests.** `make test` also gives the compose Redis to the tests in `QABZ_TEST_REDIS_URL` (a new verb of the test tool, `testdb redis-url <env file>`; no password on a command line). Each live Redis test uses its own random prefix (`qabz_test_<hex>:`) and short TTLs, so no key of the App is touched.
- **Conventions § 16.** The supertask rule "every Redis key has the prefix" is binding in the conventions doc (memory rule: lasting rules go there). `redis.Client.Raw()` (not used) is removed, so a key without the prefix cannot be written from outside `system/redis`.
- **The provider call metrics.** The worker uses the counters that the inquiry flow registers (`qabz_provider_calls_total`, `qabz_provider_call_duration_ms`, action `status`), taken from the registry (`registerOrExisting`); the Help texts name the action `status`.
- **The stop.** The workers stop after the servers and before the background jobs; the Kafka producer is flushed and closed after the workers.
- **The message JSON (code-reviewer finding).** Conventions § 1 forbids `encoding/json` v1 everywhere, in tests too. The message is marshaled with `encoding/json/v2` and the v1 options of `order.MarshalPayload` (deterministic, the HTML and U+2028/U+2029 escapes, a nil map as `null`); the bytes are bill's for every input that can reach it (a UUID text and `success` or `failed`). v1 writes an invalid UTF-8 byte as the escape `\ufffd` and v2 cannot, so such an input is an error (the report fails, nothing is saved) rather than other bytes; the test checks the escapes against v1's bytes written out and the error.
- **Other code-reviewer fixes (2026-11-15):** a nil check of the row in `InsertFinal`; length and nil guards in five tests (conventions § 4); a nil-receiver check in `Lock.Key`; the workers are built last in `NewDefaultBuilder`, so no later failed step leaves the Kafka producer open. The publish wait (up to `message.timeout.ms`, 300 s, after the metadata) is follow-up (d) in `OPEN_TASKS.md`.

Verification:

- **Tests:** `make test GO_TEST_ARGS="-race -count=1 -cover"`: every package passes; the changed packages have 87.3 % (`app`), 97.2 % (`config`), 97.5 % (`core/reconcile`), 97.9 % (`entry/cron`), 98.4 % (`entry/grpc`), 96.0 % (`provider`), 96.6 % (`provider/parsianproxy`), 87.3 % (`system/kafka`), 90.9 % (`system/postgres`), 90.6 % (`system/redis`), 92.4 % (`testdb`). The live tests ran (not skipped). Positive controls: a wrong tie rule fails `TestLeastBytesFixed` and `TestLeastBytesDifferential`; a wrong JSON name fails `TestStatusMessageBytes`.
- **Feature ON (Rule 15),** the compose Postgres, Redis and Kafka, the Parsian mock, `cron_time: 10s`, an App copy on other ports (the operator's debug App was on 8080 / 8082 / 9090 and was not touched):
  - Mock `pending`: orders 1 to 3 asked at the start and again 10 s later (the own lock was deleted after the first run); no row, no topic, no message; the 11 h order never asked (step 3, step 4).
  - Mock `success`, two instances at the same time: instance 2 counted `no_lock` at the first tick (step 6); instance 1 reported orders 2 and 3; its first publish (order 1) failed because the topic did not exist yet (`Unknown topic or partition`), so nothing was saved; instance 2 reported order 1 at its next run. One message per order, key `bill`, the value of bill; the partitions follow bill's rule for each instance (step 1).
  - Mock `failed`: orders 5 and 6 got a `failed` row and one `failed` message each (step 2).
  - Kafka stopped, mock `success`: one error line `parsian status: report failed` (`Local: Broker transport failure`), the row of order 7 unchanged; Kafka started again: the next run reported and saved it (step 5).
  - The App exited within 3 s after each Ctrl-C; a stop during a long run was not exercised live (the runs took milliseconds; `TestStopCancelsTheRun` covers it) (step 7).
  - D17: `OrderComplete` with a wrong price answered `status=pending currentStep=bill`, and `currentStep=x` with `service.ecosystem_name: "x"`.
  - The 14 QA rows were deleted from the dev database after the run; the Kafka service was stopped.
