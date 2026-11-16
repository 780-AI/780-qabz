<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-16 — Created (operator request): electricity bill order and payment.
-->
# 052_electricity_payment — Electricity bill order and payment

Status: pending

## Description

Pay an electricity bill, as the water bill (task 048). Read tasks 044 (landline), 045 (status worker) and 048 (water) and `docs/this_vs_bill.md` items 38 to 47 first: the order flow (`core/order`, `billTypes`), the payment step (`order.Payments`, `PayRoutes` in `src/app/defaults.go`), the Parsian adapter `Pay` and the shared status worker already exist. The design step reads `../bill` (commit `ff98e25`) again and lists the steps for the operator before any code, as task 048 did.

**What `bill` does (read 2026-11-16):** `Electricity.NewOrder` / `NewOrderV2` (`controller/electricity/newOrder.go`) call `controller.CreateNewOrder` with `entity.ELECTRICITY`; the V2 token `data` keys are `payId`, `type`, `billId`, `payAmount`, `rawAmount`, `about`, `aboutIcon`, `invoicePayID`, `invoiceBillID`, `receiptPayID`, `receiptBillID` and, with a fee, `feeAmount` and `feeDescription` (V1 has `payId`, `type`, `billId`). The payment gateway (`gateway/payment/electricity.go`) has two providers: `ParsianElectricity` (`TopBill/Pay`, `inquiry_type` `regular`, `provider_order_id` from the RRN) and `EWaysElectricity` (NATS). The "bill already paid" window is `parsian.settlementInquiryElectricity`. No settlement (the confirm-payment map has only `tel` and `telmidterm`). Seed order providers: row 47 `parsian` (enabled), row 17 `eways2` (disabled).

**Open points for the design step:** (1) the same as water: the route is Parsian only (Qabz has no NATS and no EWays adapter); confirm with the operator; (2) the price rules (raw inquiry first or the Tavanir inquiry; `bill`'s `AvailableBillTypes`; the saved-inquiry key, for example `caching.paid_electricity`); (3) the key `providers.internal.parsian.settlement_inquiry_electricity` is `0s` and not read: its default for Qabz; (4) the Postman request.

## Definition of Done

- `POST /v2/electricity/order` creates the order and the signed token as `bill`; `POST /v1/electricity/order` stays `Unimplemented` as for water.
- ParsianProxy pays `electricity` (`PayRoutes` of `parsian`); `OrderComplete` answers `success`, `failed` (every failed case, task 048 D10 and D11) and `pending` as for water.
- `OrderComplete` itself gives the final status of a normal payment. An order that it leaves `pending` or `in-progress` (a Parsian timeout, a short downtime) is finalized later by the Parsian status worker (task 045) with no code change; a test with a `pending` electricity order proves it.
- `docs/this_vs_bill.md` has an item for each difference from `bill`, with a `@todo` at the code; `system.html`, the config examples, the Postman collection, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Unit tests with stubs and live tests on the run database (conventions § 14), the metrics of conventions § 12, the spans of conventions § 17. Build, vet and tests (with `-race`) pass; each changed package is at 70.0 % or more.
- Feature ON (Rule 15): a run in dev mode with the mock, with the evidence analyzed: the order, `OrderComplete` for `success`, `failed` and `pending`, the Kafka report of the status worker for a pending order, and the metrics. The operator QA on stage stays open in `OPEN_TASKS.md`.
