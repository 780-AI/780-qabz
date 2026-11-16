<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-16 — Created (operator request): mci mobile bill order and payment.
-->
# 054_mci_mobile_payment — MCI mobile bill order and payment

Status: pending

## Description

Pay an MCI mobile bill. Read tasks 044 (landline), 045 (status worker) and 048 (water) and `docs/this_vs_bill.md` items 38 to 47 first: the order flow (`core/order`, `billTypes`), the payment step (`order.Payments`, `PayRoutes` in `src/app/defaults.go`), the Parsian adapter `Pay` and the shared status worker already exist. The design step reads `../bill` (commit `ff98e25`) again and lists the steps for the operator before any code, as task 048 did.

**What `bill` does (read 2026-11-16):** `Phone.NewOrderV2` (`controller/phone/newOrder.go`) maps the operator and the term to a bill type: mci `fullTerm` to `mobile`, mci `midTerm` to `mobilemidterm`. The V2 token `data` keys are `type`, `phone`, `service` (the operator), `billId`, `payId`, `payAmount`, `rawAmount`, `about`, `aboutIcon`, `receiptMobile`, `invoiceMobile` (the phone in the 0 form) and, with a fee, `feeAmount` and `feeDescription`; task 039 already builds this for Irancell. The payment gateway (`gateway/payment/mobile_mci.go`) has two providers: `JiringSwitchMCIMobile` (`BillPay` of the `jiring-switch` service; `provider_order_id` = day of the year and the RRN) and `ParsianMCIMobile` (`TopBill/Pay`, the inquiry type of the request). The "bill already paid" window is `parsian.settlementInquiryMCIMobile` (Jiring Switch: 0). No settlement. Seed order providers: row 30 `jiring-switch` for `mobile` and row 29 for `mobilemidterm` (both disabled), row 48 `parsian` for `mobile` (disabled), row 149 `parsian` for `mobilemidterm` (enabled).

Today `POST /v1/mobile/order` answers `Unimplemented` for the operator `mci` (the table `orderOperators` in `src/entry/grpc/phone_order.go` has a nil entry, task 039); the MCI inquiry (task 036) already exists.

**Open points for the design step:** (1) the payment provider: Jiring Switch (`BillPay`, needs the adapter `Pay` and its mock), Parsian, or both by the provider pick of the seed; confirm with the operator which rows are enabled; (2) the bill types `mobile` and `mobilemidterm` in `billTypes` and in `PayRoutes` (Qabz has one inquiry bill type `mobile` that answers both terms; `bill` has two order bill types); (3) the price rule (direct price or the inquiry); (4) the key `settlement_inquiry_mci_mobile` (`0s`, not read); (5) the Postman request.

## Definition of Done

- `POST /v1/mobile/order` for the operator `mci` (`fullTerm` and `midTerm`) creates the order and the token as `bill`; the Irancell behavior of task 039 does not change.
- The chosen provider(s) pay `mobile` and `mobilemidterm`; `OrderComplete` answers `success`, `failed` and `pending`; the adapter `Pay` and its mock follow conventions § 15.
- `OrderComplete` itself gives the final status of a normal payment. An order that it leaves `pending` or `in-progress` (a provider timeout, a short downtime) is finalized later by the status worker of that provider: the Parsian worker (task 045) with no code change; Jiring Switch has bill keys `get_status.*` and needs its own worker only if the design chooses that provider (operator decision).
- `docs/this_vs_bill.md` has an item for each difference from `bill`, with a `@todo` at the code; `system.html`, the config examples, the Postman collection, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Unit tests with stubs and live tests on the run database (conventions § 14), the metrics of conventions § 12, the spans of conventions § 17. Build, vet and tests (with `-race`) pass; each changed package is at 70.0 % or more.
- Feature ON (Rule 15): a run in dev mode with the mock, with the evidence analyzed: the order, `OrderComplete` for `success`, `failed` and `pending`, the Kafka report of the status worker for a pending order, and the metrics. The operator QA on stage stays open in `OPEN_TASKS.md`.
