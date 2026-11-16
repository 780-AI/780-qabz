<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-16 — Created (operator request): rightel mobile bill order and payment.
-->
# 055_rightel_mobile_payment — Rightel mobile bill order and payment

Status: pending

## Description

Pay a Rightel mobile bill. Read tasks 044 (landline), 045 (status worker) and 048 (water) and `docs/this_vs_bill.md` items 38 to 47 first: the order flow (`core/order`, `billTypes`), the payment step (`order.Payments`, `PayRoutes` in `src/app/defaults.go`), the Parsian adapter `Pay` and the shared status worker already exist. The design step reads `../bill` (commit `ff98e25`) again and lists the steps for the operator before any code, as task 048 did.

**What `bill` does (read 2026-11-16):** `Phone.NewOrderV2` maps rightel `midTerm` to the bill type `mobilemidtermRightel` (only the mid term; the token keys are those of task 054; `bill` has a commented-out block that disabled Rightel payment because of the provider). The payment gateway (`gateway/payment/mobile_rightel.go`) has `ParsianRightelMobile` (`TopBill/Pay`, the inquiry type of the request) and `EWaysRightelMobile` (NATS). The "bill already paid" window is `parsian.settlementInquiryRightelMobile` (EWays: `ewaysV2.settlementInquiryRightelMobile`). No settlement. Seed order providers: row 37 `eways2` (**enabled**), row 500 `parsian` (disabled).

Today `POST /v2/mobile/order` answers `Unimplemented` for `rightel` (task 039); the Rightel inquiry (task 037, through Qabzino) already exists and only Qabzino may answer it.

**Open points for the design step:** (1) Qabz has no EWays adapter and no NATS, and the seed enables EWays for Rightel: confirm that Qabz pays Rightel through Parsian only (row 500 enabled in the Qabz database) as for the other types; (2) the `parsianproxy.Pay` bill type `mobilemidtermRightel`; (3) `settlement_inquiry_rightel_mobile` (`0s`, not read); (4) the Postman request.

## Definition of Done

- `POST /v2/mobile/order` for the operator `rightel` (`midTerm`) creates the order and the token as `bill`; a Rightel `fullTerm` request answers as `bill` (the table has no entry).
- ParsianProxy pays `mobilemidtermRightel`; `OrderComplete` answers `success`, `failed` and `pending`.
- `OrderComplete` itself gives the final status of a normal payment. An order that it leaves `pending` or `in-progress` (a Parsian timeout, a short downtime) is finalized later by the Parsian status worker (task 045) with no code change; a test with a `pending` Rightel order proves it.
- `docs/this_vs_bill.md` has an item for each difference from `bill`, with a `@todo` at the code; `system.html`, the config examples, the Postman collection, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Unit tests with stubs and live tests on the run database (conventions § 14), the metrics of conventions § 12, the spans of conventions § 17. Build, vet and tests (with `-race`) pass; each changed package is at 70.0 % or more.
- Feature ON (Rule 15): a run in dev mode with the mock, with the evidence analyzed: the order, `OrderComplete` for `success`, `failed` and `pending`, the Kafka report of the status worker for a pending order, and the metrics. The operator QA on stage stays open in `OPEN_TASKS.md`.
