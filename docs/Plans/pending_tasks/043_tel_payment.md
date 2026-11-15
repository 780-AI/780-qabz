<!-- VERSION: 0.2 -->
<!--
CHANGE LOG
v0.2 — 2026-11-15 — Operator decisions A (as bill), B (no reject, as bill) and C (a Kafka client, the message of bill exactly) recorded.
v0.1 — 2026-11-15 — Created (operator request): the supertask of the telephone payment; split into tasks 044, 045 and 046.
-->
# 043_tel_payment — telephone (landline) payment through Parsian (supertask)

Status: pending

## Description

The telephone (landline) payment works as the Irancell mobile order (task 039), but the order provider is **Parsian** (the parsian-v2 service, our own sibling service). This is the **first task that pays through Parsian**, so it has high importance. The supertask has three tasks; each has its own file, commit and verification:

1. **Task 044 `044_tel_order_payment`** — the order (`POST /v2/tel/order`), the payment step (the payment service calls `PaymentComplete/OrderComplete`, Qabz pays through Parsian), and the Karashop settlement at once after a payment success.
2. **Task 045 `045_parsian_status_worker`** — the worker that checks the Parsian status of each order that stays `pending` or `in-progress`. It also builds the **shared worker runner** (schedule, Redis lock, graceful stop) that task 046 uses.
3. **Task 046 `046_karashop_settle_worker`** — the worker that sends the settlement to Karashop again for each successful order that Karashop did not confirm.

**Order:** 044, then 045, then 046. Task 045 needs the orders of task 044; task 046 needs the runner of task 045.

**Rules for all three tasks (operator, 2026-11-15):**
- **Follow the `bill` routine.** `bill` is the reference, as in task 039. Each difference that a client, the payment service or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code.
- **Multi-instance deployment.** Many Qabz instances run at the same time. Only one instance runs a given worker at a time. `bill` uses a Redis lock (`SET NX` with a TTL), so Qabz also uses Redis.
- **One Redis key prefix.** Every Redis command of Qabz puts one prefix from the config in front of each key name. The operator named it `config.redis.prefix`; the Redis block is `database.redis`, so the key is `database.redis.prefix` (conventions § 8). The design step confirms the name. The key applies to every Redis key of Qabz, not only to the worker locks.
- **The interval of each worker is a config key.**
- **The real end-to-end test is done by the operator** (a real payment through the payment service, Parsian and Karashop).

**What the `bill` code does (read on 2026-11-15, `bill` commit `ff98e25`, not run).** The operator asked to check these points against the docs and the code:

| Operator statement | `bill` | Result |
|---|---|---|
| A payment with no success from Parsian is checked again by a worker. | `pkg/cronTask/parsianGetStatus.go` → `service/billPaymentInquiry/dto.go` `InquiryCron`: orders of the provider `parsian` with the last status `pending` or `in-progress`. Config `parsian`: `every: 15m`, `start: 30m`, `end: 10h`, `chunk: 100`, `lockTime: 30m`. | Correct. |
| The worker has ten hours to finalize, else a refund. | The worker reads only orders created between 10 h and 30 min ago. An order that is still pending after 10 h leaves the window; `bill` does nothing more with it (no refund, no message). A final status that comes after `ReverseDeadline` is saved with the message `reverse_deadline_exceeded` and is **not** sent to the payment service. | **Partly correct.** The 10 h is correct. `bill` does not refund after 10 h. Qabz does the same (operator decision A). |
| A status `failed` from Parsian is refunded at once. | `bill` never refunds by itself. In the payment step, the status goes back to the payment service in the answer of `OrderComplete`. In the worker, the status goes to the payment service on Kafka (topic `kafka.topic.payment.write.subject`, `paymentwriter`). **The payment service decides the refund** (`docs/initial-discovery/qabz-cron-diagram.html`). | **Correct in effect, but the payment service does the refund, not Qabz.** |
| After a payment success, Karashop settlement at once. | `service/billPayment/completeOrder.go`: on `success`, `go s.ConfirmPayment(...)` for `tel` and `telmidterm` (providers Parsian and EWays): Karashop `SendPaymentInfo`; on success `provider_confirmed = true`. | Correct. |
| A failed settlement goes to a Karashop worker, every X minutes, settle or reject. | `pkg/cronTask/paymentConfirm.go` → `ConfirmPaymentBunch`: orders with the last status `success`, `is_raw = false`, `provider_confirmed = false`, bill type `tel` and `telmidterm`, in a window (`confirmPayment.telephone`: `every`, `lockTime`, `start`, `end`, `chunk`). | **Partly correct.** `bill` only tries again until the order leaves the window. `bill` has **no reject**. Qabz does the same (operator decision B). |
| All workers use the same pattern and one instance runs a worker. | `pkg/cronTask/cron.go` (gocron, `Every(interval)`), `pkg/lock/lock.go`: `SETNX <service.name>:crons_<name>_lock1` with a TTL, extended per chunk (`ChargeTTL`); `Release` deletes the key. | Correct. |

**Operator decisions (2026-11-15):**
- **A. After 10 h with no final status: as `bill`.** Qabz does nothing more with the order (no refund, no report). It stays pending. A final status that comes after the reverse deadline is saved with the message `reverse_deadline_exceeded` and is not reported. No `this_vs_bill.md` item.
- **B. Karashop "reject": none, as `bill`.** The worker tries again at each run until the order is outside the window. There is no reject and no end state for "not settled". No `this_vs_bill.md` item.
- **C. Kafka: a Kafka client in Qabz, as `bill`.** The worker of task 045 sends `success` and `failed` to the payment service on Kafka. The topic name is a config key. **The message MUST NOT differ from the message of `bill`** (the details are in task 045).

**Risk (design step checks it).** If Qabz writes into the same `orders` table that the `bill` workers read, the `bill` workers also take the Qabz orders (both select by `provider_name` or `bill_type`). The design step confirms that Qabz has its own database (`database.postgres.db: qabz`).

Help:
- `bill`: `app/initCron.go`, `pkg/cronTask/` (`cron.go`, `parsianGetStatus.go`, `paymentConfirm.go`), `pkg/lock/lock.go`, `service/billPaymentInquiry/dto.go`, `service/billPayment/completeOrder.go`, `confirmPayment.go`, `controller/payment/orderComplete.go`, `gateway/confirmPayment/telephone.go`, `thirdparty/karashop/sendPaymentInfo.go`, `domain/repository/order/order_dto.go` (`GetAllPending`, `GetOrdersForConfirmPayment`), `config/internalProvider.go`, `config/confirmPayment.go`, `config.example.yaml`.
- Qabz: task 039 (the order base), task 025 (the tel inquiry, Karashop), `docs/initial-discovery/qabz-cron-diagram.html`, `qabz-diagram.html`, `src/provider/parsianproxy`, `src/provider/karashop`, `src/system/redis`.

## Definition of Done

- Tasks 044, 045 and 046 are done.
- Operator decisions A, B and C are applied, and each difference from `bill` is an item in `docs/this_vs_bill.md`.
- The operator ran the real end-to-end test (a real tel payment through the payment service, Parsian and Karashop) and confirmed it.
