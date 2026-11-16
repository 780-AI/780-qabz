<!-- VERSION: 0.3 -->
<!--
CHANGE LOG
v0.3 — 2026-11-16 — Moved back to pending (the active task was task 057, no `/v2` routes, operator request); the design uses the route `POST /v1/gas/order` and the rpc `Gas.NewOrder` (conventions § 18); `caching.paid_gas` explained to the operator; still waits for the operator approval.
v0.2 — 2026-11-16 — Started (operator request "do task 53. Make sure it is also Parsian only."): moved to in-progress_tasks; design written (D1 to D11); waits for the operator approval.
v0.1 — 2026-11-16 — Created (operator request): gas bill order and payment.
-->
# 053_gas_payment — Gas bill order and payment

Status: pending (the design waits for the operator approval)

## Description

Pay a gas bill, as the water bill (task 048). Read tasks 044 (landline), 045 (status worker) and 048 (water) and `docs/this_vs_bill.md` items 38 to 47 first: the order flow (`core/order`, `billTypes`), the payment step (`order.Payments`, `PayRoutes` in `src/app/defaults.go`), the Parsian adapter `Pay` and the shared status worker already exist. The design step reads `../bill` (commit `ff98e25`) again and lists the steps for the operator before any code, as task 048 did.

**What `bill` does (read 2026-11-16):** `Gas.NewOrder` / `NewOrderV2` of `bill` (`controller/gas/newOrder.go`) are the electricity controller plus the participate code (`span` tag only; the inquiry ID is the participate code when it is set, as water, task 048 D3). The V2 token `data` keys are those of electricity. The payment gateway (`gateway/payment/gas.go`) has `ParsianGas` (`TopBill/Pay`, `inquiry_type` `regular`) and `EWaysGas` (NATS). The "bill already paid" window is `parsian.settlementInquiryGas`. No settlement. Seed order providers: row 46 `parsian` (enabled), row 18 `eways2` (disabled).

**Open points for the design step:** (1) Parsian only, as water and electricity; (2) the price rules and the saved-inquiry key (for example `caching.paid_gas`); the gas inquiry of Qabz is by participate code through Qabzino; (3) `providers.internal.parsian.settlement_inquiry_gas` is `0s` and not read: its default; (4) the Postman request.

## Definition of Done

- `POST /v1/gas/order` creates the order and the signed token as `bill` (the participate code is the inquiry ID when set).
- ParsianProxy pays `gas`; `OrderComplete` answers `success`, `failed` and `pending` as for water.
- `OrderComplete` itself gives the final status of a normal payment. An order that it leaves `pending` or `in-progress` (a Parsian timeout, a short downtime) is finalized later by the Parsian status worker (task 045) with no code change; a test with a `pending` gas order proves it.
- `docs/this_vs_bill.md` has an item for each difference from `bill`, with a `@todo` at the code; `system.html`, the config examples, the Postman collection, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Unit tests with stubs and live tests on the run database (conventions § 14), the metrics of conventions § 12, the spans of conventions § 17. Build, vet and tests (with `-race`) pass; each changed package is at 70.0 % or more.
- Feature ON (Rule 15): a run in dev mode with the mock, with the evidence analyzed: the order, `OrderComplete` for `success`, `failed` and `pending`, the Kafka report of the status worker for a pending order, and the metrics. The operator QA on stage stays open in `OPEN_TASKS.md`.

## Design (implementer, 2026-11-16; waits for the operator approval)

Read on 2026-11-16 from `bill` commit `ff98e25` (not run) and from the Qabz source at `6ed4509`. The operator said "Parsian only" (D2). Every other item follows `bill` and the electricity payment (task 052); no item needs a new operator choice. Each difference from `bill` that a client, the payment service or the database can see gets an item in `docs/this_vs_bill.md` and a `@todo` at the code.

**Summary.** In `bill` the gas order and payment are the electricity order and payment with other texts: the same `controller.CreateNewOrder` (`validateNewOrderRequest`: `billId` or `payId`; `SetInquiryID`), the same eleven token keys, no provider fee (`AvailableBillTypes` has only the Karashop bill types), no settlement, `BillTypeInquiryType[GAS]` is `regular`, and `ParsianGas` has the fields of `ParsianElectricity`. So task 053 adds `gas` entries to the shared code of tasks 048 and 052; it adds no new flow.

### The order (`POST /v1/gas/order`)

- **D1. The entry.** `Gas.NewOrder` (`NewOrderBillPayRequest` → `NewOrderResponse`) on the shared handler `utilityOrder.newOrder` (new `src/entry/grpc/gas_order.go`), as electricity: the request check (`billId` or `payId`, else HTTP 400 with no log line; `this_vs_bill.md` item 38 is extended), no user HTTP 403, the inquiry ID `orderInquiryID` (the participate code when it is set, else `billId`; bill: `SetInquiryID`), `order.Service.Create`, the token. `GasDeps` gets `Orders`, `Signer` and `ServiceID`; `newGas` gets the order flow, the signer and `service.id`. `POST /v1/gas/complete-order` stays `Unimplemented`. Note: the order request check is not the gas inquiry check (`validGasInquiry`), as in bill.
- **D2. The route: Parsian only (operator: "Make sure it is also Parsian only").** `bill` has two order providers for gas: `ParsianGas` (seed row 46, `enable`) and `EWaysGas` (NATS, seed row 18, `disable`). Qabz has no NATS and no EWays adapter. Only `parsian` gets a `gas` entry in `PayRoutes` and in the "already paid" windows; an enabled `eways2` row gives the answer of a provider with no payment route (the case 2 `failed` answer of task 048), as for water and electricity.
- **D3. The flow (`core/order`, a new `billTypes` entry `gas`: `{inquiryType: regular}`, the same as `water` and `electricity`).**
  1. The order provider of (`gas`, `order`): seed row 46, `parsian`.
  2. The "bill already paid" check (D4).
  3. The price: the raw inquiry when `billId` and `payId` are numeric (bill: `shouldInquiryWithPayID`); else (or when it fails) the provider inquiry of `gas` with the inquiry type `regular` and the inquiry ID of D1, with saved inquiries for `caching.paid_gas` (D5). **As water, not as electricity:** the gas inquiry provider of the seed is `parsian` (row 43, `enable`), which has no gas inquiry in Qabz, so this path answers as `POST /v1/gas/inquiry` does today (HTTP 503). With the Qabzino row 35 enabled, Qabzino gets the inquiry ID as the participate code (bill: the same).
  4. A price of 0 is `FailedPrecondition` with bill's text; the provider fee is 0; the fee is `fee.Resolver` (default 9000; `bill`'s `GasFee` is also 9000); the `orders` row `initial`.
- **D4. The "bill already paid" window.** `providers.internal.parsian.settlement_inquiry_gas` exists, is `0s` and is not read. It gets the default `360h` (`bill`'s example value) and is read, as for water and electricity. The config check rejects a negative value. The `config.yml.example` value changes from `"0s"` to `"360h"`.
- **D5. Saved inquiries for a gas order.** A new key `caching.paid_gas`, default `72h` (`bill`: `caching.paidGas: 72h`). It applies only when the price comes from the provider inquiry.
- **D6. The token `data` map.** `order.UtilityData` (the keys of `bill`'s `Gas.NewOrder`, the same as water and electricity). The metadata of `gas` (bill: `GasIcon`, `AboutIconGas`, `FeeDescriptionGas`): `about` `قبض گاز` (no space at the start), `aboutIcon` `https://public.780.ir/dl/Operator/Gas.svg`, `feeDescription` `کارمزد`.

### The payment (`OrderComplete`, sync)

- **D7. The payment route.** `PayRoutes` of `parsian` gets `gas`; `parsianproxy.Pay` pays `gas` (`payBillTypes`; bill: `ParsianGas`, the fields of `ParsianElectricity`: `inquiry_type` `regular`, `provider_order_id` from the RRN, `amount` the base price, `inquiry_id` `orders.data`). The Parsian mock answers `gas` with no change.
- **D8. After a success.** bill's `updateCache` side effects (the `is_paid` of the `bill_inquiries` rows and `last_order_date` of the saved bills of the bill ID and `gas`). No settlement: `settleRoutes` has no `gas` entry (bill: the confirm-payment map has only `tel` and `telmidterm`).
- **D9. The `failed` answer.** No change: the cases 1 to 3 of task 048 (D10, D11) are in the shared code. `gas` is added to `TestCompleteFailedOrderAnswersFailed`. Task 048 D12 stays open; it applies to gas too.

### The Parsian status worker (async)

- **D10. No code change.** The worker reads the `pending` and `in-progress` orders by provider only; it asks `TopBill/OrderInquiry` with `bill_type` `gas`. A live test (`liveStatusWorker` with `gas`, row 46) finalizes a `pending` gas order (`success` and `failed`) and checks the Kafka report.

### Tests, docs, metrics

- **D11.** Unit tests: `core/order/gas_test.go` (raw; the provider inquiry with the participate code as the inquiry ID and `caching.paid_gas`; already paid; the inquiry type; the data map with and without a fee; the payment request; no settlement; no route), `entry/grpc/gas_order_test.go`, `TestPayGas`, `TestParseGasOrderKeys`; `gas` in `TestCompleteFailedOrderAnswersFailed`; the fake order repository knows row 46. Old tests that use `gas` as an unknown order bill type (`TestCreateUnsupportedAndNotBuilt`, `TestInquiryTypeOf`, `TestReplace98AndMetadata`) use `unknown` (tasks 054 to 056 then need no change there); `TestParseJiringSwitchAndParsianBillKeys` expects `360h` for gas. Live tests (run database, conventions § 14): a gas order and its payment rows for `success`, `failed` and `pending` (with the span `order.Service.Create` `bill_type=gas`); a gas order whose raw inquiry fails, priced by the Qabzino mock (row 35 enabled in the test); the status worker. Metrics: the existing ones with `bill_type="gas"` (conventions § 12); the help texts name `gas`. Docs in the same commit: `config.yml.example`, `system.html`, `this_vs_bill.md` (item 38 extended), the Postman item Gas / Order, `src/app/README.md`, `folder_structure.md` (new files), the CHANGELOG, the manifest and the QA scenario in `OPEN_TASKS.md`.
- **Feature ON (Rule 15):** an App copy on other ports with the Parsian mock (`success`, `failed`, `pending`) and the status worker with the compose Kafka, as task 052; also one order whose raw inquiry fails, priced by the Qabzino mock. The operator QA on stage stays open in `OPEN_TASKS.md`.
