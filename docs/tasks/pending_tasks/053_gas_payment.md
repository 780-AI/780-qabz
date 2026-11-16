<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-16 — Created (operator request): gas bill order and payment.
-->
# 053_gas_payment — Gas bill order and payment

Status: pending

## Description

Pay a gas bill, as the water bill (task 048). Read tasks 044 (landline), 045 (status worker) and 048 (water) and `docs/this_vs_bill.md` items 38 to 47 first: the order flow (`core/order`, `billTypes`), the payment step (`order.Payments`, `PayRoutes` in `src/app/defaults.go`), the Parsian adapter `Pay` and the shared status worker already exist. The design step reads `../bill` (commit `ff98e25`) again and lists the steps for the operator before any code, as task 048 did.

**What `bill` does (read 2026-11-16):** `Gas.NewOrder` / `NewOrderV2` (`controller/gas/newOrder.go`) are the electricity controller plus the participate code (`span` tag only; the inquiry ID is the participate code when it is set, as water, task 048 D3). The V2 token `data` keys are those of electricity. The payment gateway (`gateway/payment/gas.go`) has `ParsianGas` (`TopBill/Pay`, `inquiry_type` `regular`) and `EWaysGas` (NATS). The "bill already paid" window is `parsian.settlementInquiryGas`. No settlement. Seed order providers: row 46 `parsian` (enabled), row 18 `eways2` (disabled).

**Open points for the design step:** (1) Parsian only, as water and electricity; (2) the price rules and the saved-inquiry key (for example `caching.paid_gas`); the gas inquiry of Qabz is by participate code through Qabzino; (3) `providers.internal.parsian.settlement_inquiry_gas` is `0s` and not read: its default; (4) the Postman request.

## Definition of Done

- `POST /v2/gas/order` creates the order and the signed token as `bill` (the participate code is the inquiry ID when set); `POST /v1/gas/order` stays `Unimplemented`.
- ParsianProxy pays `gas`; `OrderComplete` answers `success`, `failed` and `pending` as for water.
- `OrderComplete` itself gives the final status of a normal payment. An order that it leaves `pending` or `in-progress` (a Parsian timeout, a short downtime) is finalized later by the Parsian status worker (task 045) with no code change; a test with a `pending` gas order proves it.
- `docs/this_vs_bill.md` has an item for each difference from `bill`, with a `@todo` at the code; `system.html`, the config examples, the Postman collection, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Unit tests with stubs and live tests on the run database (conventions § 14), the metrics of conventions § 12, the spans of conventions § 17. Build, vet and tests (with `-race`) pass; each changed package is at 70.0 % or more.
- Feature ON (Rule 15): a run in dev mode with the mock, with the evidence analyzed: the order, `OrderComplete` for `success`, `failed` and `pending`, the Kafka report of the status worker for a pending order, and the metrics. The operator QA on stage stays open in `OPEN_TASKS.md`.
