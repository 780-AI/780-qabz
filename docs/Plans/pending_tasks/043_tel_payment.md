<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
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
| The worker has ten hours to finalize, else a refund. | The worker reads only orders created between 10 h and 30 min ago. An order that is still pending after 10 h leaves the window; `bill` does nothing more with it (no refund, no message). A final status that comes after `ReverseDeadline` is saved with the message `reverse_deadline_exceeded` and is **not** sent to the payment service. | **Partly correct.** The 10 h is correct. `bill` does not refund after 10 h. A refund after 10 h is a new rule (operator decision A below). |
| A status `failed` from Parsian is refunded at once. | `bill` never refunds by itself. In the payment step, the status goes back to the payment service in the answer of `OrderComplete`. In the worker, the status goes to the payment service on Kafka (topic `kafka.topic.payment.write.subject`, `paymentwriter`). **The payment service decides the refund** (`docs/initial-discovery/qabz-cron-diagram.html`). | **Correct in effect, but the payment service does the refund, not Qabz.** |
| After a payment success, Karashop settlement at once. | `service/billPayment/completeOrder.go`: on `success`, `go s.ConfirmPayment(...)` for `tel` and `telmidterm` (providers Parsian and EWays): Karashop `SendPaymentInfo`; on success `provider_confirmed = true`. | Correct. |
| A failed settlement goes to a Karashop worker, every X minutes, settle or reject. | `pkg/cronTask/paymentConfirm.go` → `ConfirmPaymentBunch`: orders with the last status `success`, `is_raw = false`, `provider_confirmed = false`, bill type `tel` and `telmidterm`, in a window (`confirmPayment.telephone`: `every`, `lockTime`, `start`, `end`, `chunk`). | **Partly correct.** `bill` only tries again until the order leaves the window. `bill` has **no reject**. A reject is a new rule (operator decision B below). |
| All workers use the same pattern and one instance runs a worker. | `pkg/cronTask/cron.go` (gocron, `Every(interval)`), `pkg/lock/lock.go`: `SETNX <service.name>:crons_<name>_lock1` with a TTL, extended per chunk (`ChargeTTL`); `Release` deletes the key. | Correct. |

**Operator decisions needed before the design of task 045 and task 046:**
- **A. After 10 h with no final status.** (1) As `bill`: nothing (the order stays pending, the operator team fixes it by hand). (2) Qabz sends `failed` to the payment service so that the payment service refunds. (2) is a new money rule; Parsian can still pay the bill after Qabz sends `failed` (a double loss risk).
- **B. Karashop "reject".** What does reject mean, and when? `bill` has no reject. Options: (1) as `bill`, try until the window ends; (2) mark the order as not settled after N tries or after the window, with an error log line and a metric for the operator team. Karashop settlement does not move money, so a reject is not a refund.
- **C. The Kafka report of the worker.** Qabz has no Kafka client yet (`src/system/kafka/` is empty). The worker of task 045 needs it to tell the payment service the final status, as `bill` does.

**Risk (design step checks it).** If Qabz writes into the same `orders` table that the `bill` workers read, the `bill` workers also take the Qabz orders (both select by `provider_name` or `bill_type`). The design step confirms that Qabz has its own database (`database.postgres.db: qabz`).

Help:
- `bill`: `app/initCron.go`, `pkg/cronTask/` (`cron.go`, `parsianGetStatus.go`, `paymentConfirm.go`), `pkg/lock/lock.go`, `service/billPaymentInquiry/dto.go`, `service/billPayment/completeOrder.go`, `confirmPayment.go`, `controller/payment/orderComplete.go`, `gateway/confirmPayment/telephone.go`, `thirdparty/karashop/sendPaymentInfo.go`, `domain/repository/order/order_dto.go` (`GetAllPending`, `GetOrdersForConfirmPayment`), `config/internalProvider.go`, `config/confirmPayment.go`, `config.example.yaml`.
- Qabz: task 039 (the order base), task 025 (the tel inquiry, Karashop), `docs/initial-discovery/qabz-cron-diagram.html`, `qabz-diagram.html`, `src/provider/parsianproxy`, `src/provider/karashop`, `src/system/redis`.

## Definition of Done

- Tasks 044, 045 and 046 are done.
- Operator decisions A, B and C are recorded in the task files, and each difference from `bill` is an item in `docs/this_vs_bill.md`.
- The operator ran the real end-to-end test (a real tel payment through the payment service, Parsian and Karashop) and confirmed it.
