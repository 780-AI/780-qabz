<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-16 — Created (operator request): irancell mobile bill payment (ordercomplete).
-->
# 056_irancell_payment — Irancell mobile bill payment (OrderComplete)

Status: pending

## Description

Pay an Irancell order that exists since task 039. The order endpoint (`POST /v2/mobile/order`, bill type `irancell`, order provider row 26 `irancell`) is done; the payment step is not: `irancellproxy.Adapter.Pay` returns `provider.ErrNotSupported` and `PayRoutes` has no `irancell` entry. Read tasks 044 (landline), 045 (status worker) and 048 (water) and `docs/this_vs_bill.md` items 38 to 47 first: the order flow (`core/order`, `billTypes`), the payment step (`order.Payments`, `PayRoutes` in `src/app/defaults.go`), the Parsian adapter `Pay` and the shared status worker already exist. The design step reads `../bill` (commit `ff98e25`) again and lists the steps for the operator before any code, as task 048 did.

**What `bill` does (read 2026-11-16):** the payment gateway (`gateway/payment/mobile_irancell.go`, `IrancellMobile`) sends the whole `PaymentRequest` as a **NATS request** (`orderbus.Nats.Request`) to the subject `irancell.natsSubject` (`irancell-bill`) and reads a `PaymentResponse` back; a failed request is a `pending` answer. It does not call the irancell gRPC service (the proto `pb/proto/irancell/internalService.proto` has no payment RPC for a bill; `GetCharge` and `OrderOffer` are other products). The status of the order is read later by `InternalService/GetStatus` (task 050, done). No settlement.

**Open points for the design step (decision for the operator, the first one blocks the rest):** (1) how Qabz sends the payment to the irancell service: a NATS client in Qabz (a new dependency and the keys `providers.internal.irancell.nats_subject` and a NATS address) that keeps `bill`'s message, or a payment RPC that the irancell service must add (a change of its proto in the `pb` submodule, conventions § 6); (2) the message bytes of `PaymentRequest` / `PaymentResponse` (a differential test against `bill`'s structs, as the Kafka message of task 045); (3) the "pending" rule for a timeout; (4) the mock (conventions § 15); (5) once the payment exists, the Irancell status worker of task 050 has work: the deploy config `get_status.enabled` is part of this task's follow-ups.

## Definition of Done

- The payment route of `irancell` exists and `Adapter.Pay` pays an Irancell order with the transport that the operator chose in the design step; the request and the answer match `bill` (or the new RPC) byte for byte, checked by a test.
- `OrderComplete` answers `success`, `failed` and `pending` for an Irancell order; a timeout is `pending`.
- `OrderComplete` itself gives the final status of a normal payment. An Irancell order that it leaves `pending` or `in-progress` (a timeout, a short downtime) is finalized later by the Irancell status worker (task 050), end to end (a live test and the feature-ON run).
- `docs/this_vs_bill.md` has an item for each difference from `bill`, with a `@todo` at the code; `system.html`, the config examples, the Postman collection, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Unit tests with stubs and live tests on the run database (conventions § 14), the metrics of conventions § 12, the spans of conventions § 17. Build, vet and tests (with `-race`) pass; each changed package is at 70.0 % or more.
- Feature ON (Rule 15): a run in dev mode with the mock, with the evidence analyzed: the order, `OrderComplete` for `success`, `failed` and `pending`, the Kafka report of the status worker for a pending order, and the metrics. The operator QA on stage stays open in `OPEN_TASKS.md`.
