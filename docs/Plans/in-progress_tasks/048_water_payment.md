<!-- VERSION: 0.2 -->
<!--
CHANGE LOG
v0.2 — 2026-11-15 — Started (operator request "Do task 48"): moved to in-progress_tasks; design written (corrections C1 to C3, D1 to D14); operator approval "Approve, D11 (a)" (D12 not answered: it stays open as a stage QA item).
v0.1 — 2026-11-15 — Created (operator request): water bill payment through Parsian, the sync answer of OrderComplete and the async report of the Parsian status worker.
-->
# 048_water_payment — water bill payment through Parsian

Status: in-progress (design approved 2026-11-15)

## Description

The water bill is paid through **Parsian** (the parsian-v2 service), as the landline payment (tasks 043 to 046). Follow the `bill` routine; each difference that a client, the payment service or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code. The design step reads `bill` for the water order and payment.

**Two paths of a payment (operator, 2026-11-15):**
- **Sync (the payment step).** The payment service calls `PaymentComplete/OrderComplete`. When the order status is `failed` (for any reason), the payment service **must** get `failed` in the answer. The payment service does the refund, not Qabz.
- **Async (the Parsian status worker, task 045).** A timeout or a `pending` answer of Parsian keeps the order `pending` (or `in-progress`). The worker asks Parsian again. A final `success` or `failed` is reported on the Kafka topic of the payment service. The topic comes from the config and has the same value as `bill`.

**Start state (read on 2026-11-15, not run):**
- Water has only the inquiry (`POST /v1/water/inquiry`, task 021). There is no water order endpoint. `PayRoutes` of Parsian has only `tel` and `telmidterm` (`src/app/defaults.go:823`).
- The status worker reads the orders by provider only, not by bill type (`pendingIDsSQL`, `src/system/postgres/order_repository.go:90`). Water orders of Parsian are in its window with no change. The design confirms the report message of a water order.
- The Kafka topic is `kafka.topic.payment.write.subject`, default `paymentwriter`, the value of `bill` (task 045). The design confirms that no new key is necessary.
- `providers.internal.parsian.settlement_inquiry_water` ("bill already paid" window) is `0s` and not read.
- The settlement routes (`settleRoutes`, `src/app/defaults.go:847`) have no water entry. The design reads whether `bill` settles a water payment with a provider after success.

**The `failed` answer: check these cases in the design (the operator asked for a double check):**
1. Parsian answers `failed` at the payment step. Now: gRPC `Unknown` with the message of the provider, no answer body (`src/entry/grpc/payment.go:116`, as `bill` `errors.New(order.Message)`). The design confirms that the payment service reads this as `failed`.
2. A step before the Parsian call fails (no provider row, a failed write, no payment route). Now: a `failed` row is saved and the answer is gRPC `Unknown` with the general text (`src/core/order/complete.go:236-256`).
3. The newest row of the order is already `failed`, and the payment service calls `OrderComplete`. Now: the answer is `pending` with "order has been processed before" (`src/core/order/complete.go:224`, as `bill`). **This does not agree with the operator rule.** The design decides the fix and records a `this_vs_bill.md` item.

## Definition of Done

- A water order can be created and paid through Parsian (`OrderComplete`), as the landline payment.
- Sync: in each case where the order status is `failed`, the `OrderComplete` answer is `failed` (cases 1 to 3 above); tests cover each case.
- Async: a water order that stays `pending` or `in-progress` is finalized by the Parsian status worker; `success` and `failed` are reported on the Kafka topic from the config (`bill`'s value).
- Each difference from `bill` is in `docs/this_vs_bill.md`.
- Feature-ON run by the implementer (Rule 15). The operator QA is done only on stage, after the deploy; it stays open in `OPEN_TASKS.md` until then.

## Design (implementer, 2026-11-15; operator approval 2026-11-15: "Approve, D11 (a)")

Read on 2026-11-15 from `bill` commit `ff98e25` (not run) and from the Qabz source at `d9a90b5`. Items marked **(decision)** need an operator choice; every other item follows `bill` or the landline payment (task 044). Each difference from `bill` that a client, the payment service or the database can see gets an item in `docs/this_vs_bill.md` and a `@todo` at the code.

### Corrections of this task file

- **C1. The water order is cheaper than the landline order.** `bill` (`controller/water/newOrder.go`, `controller.CreateNewOrder`) has no provider fee for water (only Karashop `tel`/`telmidterm`/`telephone` have one) and no settlement after a success (the confirm-payment map has only `tel` and `telmidterm`). So the water payment has no Karashop step and no settlement worker.
- **C2. The price of a water order normally comes from the raw inquiry, not from a provider.** `bill` water is not in `AvailableBillTypes` (no provider fee), so the second price rule runs first: a numeric `billId` and `payId` give the price from the pay ID (`raw.Service.Inquiry`, in Qabz `inquiry.Service.RawInquiry`), with no provider call and no `bill_inquiries` row. Only a failed raw inquiry goes on to the provider inquiry of `water`. With the seed, the water inquiry provider is `parsian` (row 44), which has no water inquiry in Qabz, so that path answers as `POST /v1/water/inquiry` does today.
- **C3. `Water.CompleteOrder` (`POST /v1/water/complete-order`) is not in scope.** `bill` does not implement it either; it stays `Unimplemented`. The payment step is `PaymentComplete/OrderComplete` on the internal gRPC server (task 044 D8).

### The order (`POST /v2/water/order`)

- **D1. The entry.** `Water.NewOrderV2` (`NewOrderBillPayRequest` → `NewOrderResponse`; the proto has it). `POST /v1/water/order` stays `Unimplemented`, as `POST /v1/mobile/order` and `POST /v1/tel/order` (task 039).
- **D2. The request check (`bill`: `validateNewOrderRequest`).** `billId` or `payId` must be set; nothing else is checked (no `paymentType` check). A failed check is `invalid parameter` (HTTP 400) with no log line, as `POST /v2/tel/order` (bill: gRPC `Unknown` and one error line; `this_vs_bill.md` item 38 is extended). No user is HTTP 403, as item 14.
- **D3. The inquiry ID (`bill`: `SetInquiryID`).** `participateCode` when it is not empty, else `billId`. It is `orders.data` and the `inquiry_id` of the Parsian payment.
- **D4. The flow (`core/order`, a new `billTypes` entry `water`).** The steps of `Service.Create` with no change of the flow:
  1. The order provider of (`water`, `order`): seed row 45, `parsian`, `enable`.
  2. The "bill already paid" check (D5).
  3. The price: the raw inquiry when `billId` and `payId` are numeric (C2); else (or when it fails) the provider inquiry of `water` with the inquiry type `regular` (`bill`: `BillTypeInquiryType[WATER]`), with saved inquiries for `caching.paid_water` (D6).
  4. A price of 0 is `FailedPrecondition` with bill's text, as the other orders.
  5. The provider fee: 0 (the row of a raw price is not read; a provider inquiry row of `parsian` or `qabzino` has none).
  6. The fee: `fee.Resolver` (default 9000, as `bill`'s `DefaultFee`).
  7. The `orders` row `initial` and the token (D7).
- **D5. The "bill already paid" window (`bill`: `getNewOrderConflictProviders`, Parsian `WATER`).** The key `providers.internal.parsian.settlement_inquiry_water` exists, is `0s` and is not read. It gets the default `360h` (the value of `bill`'s example) and is read, as `settlement_inquiry_telephone` in task 044 D2. `0` means no window (every older order counts), as `bill`. The answer of a paid bill is the one of `POST /v2/tel/order`: `FailedPrecondition` (HTTP 400) with bill's text.
- **D6. Saved inquiries for a water order (`bill`: `caching.paidWater: 72h`).** A new key `caching.paid_water`, default `72h`, as `caching.paid_tel`. It applies only when the price comes from the provider inquiry (C2).
- **D7. The token `data` map (`bill`: `Water.NewOrderV2`).** The keys are `payId`, `type` (`water`), `billId`, `payAmount`, `rawAmount`, `about`, `aboutIcon`, `invoicePayID`, `invoiceBillID`, `receiptPayID`, `receiptBillID`. When `fee > 0`, also `feeAmount` (the fee) and `feeDescription`. The texts of `water` (`bill`: `helper.GetBillMetaData`): about `قبض آب`, icon `https://public.780.ir/dl/Operator/AbvaFazelab.svg`, fee description `کارمزد`.

### The payment (`OrderComplete`, sync)

- **D8. The payment route.** `PayRoutes` of `parsian` gets `water`; `parsianproxy.Pay` pays `water` (`bill`: `ParsianWater`, the same fields as `ParsianTelephone`). `inquiry_type` is `regular` (`bill` writes `RegularInquiryType`; Qabz gives `InquiryTypeOf("water")`, which is `regular`). `amount` is `base_price`; `inquiry_id` is `orders.data`. The Parsian mock (`debug.mocks.parsian.pay_return_type`) answers `water` with no change.
- **D9. After a success.** The side effects of `bill`'s `updateCache` run (task 044 D13): `is_paid` of the `bill_inquiries` rows of the inquiry ID and the pay ID, and `last_order_date` of the saved bills of the bill ID and `water`. No settlement: `settleRoutes` has no `water` entry (C1), so nothing is sent and nothing is counted (`Payments.settle`, `Settler.Route`).

### The `failed` answer (operator double check)

The payment service code is not on this machine. Every service of the ecosystem that has `OrderComplete` (`bill`, `recharge`, `traffick`) answers a failed order with a gRPC error and no body; a success or a pending order is a `PaymentOrder` with `status`. So the only `failed` answer that the payment service can get is a gRPC error. Each case:

- **D10. Case 1, Parsian answers `failed` at the payment step.** The `failed` row is saved, and the answer is gRPC `Unknown` with the message of Parsian, no body (`src/entry/grpc/payment.go:116`, as `bill` `errors.New(order.Message)`). This is the `failed` answer. No change.
  - Case 2, a step before the Parsian call fails (no provider row, a failed write, no payment route): a `failed` row is saved, and the answer is gRPC `Unknown` with `خطای داخلی رخ داده است` (`src/core/order/complete.go:236-256`). This is the `failed` answer. No change. The same answer is given when the order is not found or the request check fails (no row is saved then; nothing was paid).
  - A status of Parsian other than `success`, `failed` and `pending`, or a failed write of the final status: the answer is `pending` and the newest row stays `in-progress`; the order is not `failed`, and the worker (D13) reports its final status later. As `bill`. No change.
- **D11. Case 3, the newest row of the order is already `failed` (decision).** Now `OrderComplete` answers `pending` with `order has been processed before` (`src/core/order/complete.go:224`, as `bill` `orderValidator`). This does not agree with the operator rule. Fix in `Payments.Complete` step 2: when the newest row is `failed`, the answer is `failed`, with nothing saved and no provider call. Every other status that is not `initial` (`in-progress`, `pending`, `success`) stays `pending` with `order has been processed before`, as `bill`. The fix applies to every bill type (also `tel` and `telmidterm`), because it is in the shared step. A new `this_vs_bill.md` item 46 and a `@todo`. Choose the message of the answer:
  - **(a, recommended)** The message of the `failed` row (the text that the first call got, for example the Parsian message). When it is empty: `order has been processed before`.
  - **(b)** Always `order has been processed before`.
  
  The metric stays `qabz_order_payment_total{status="processed"}` (the order is not `initial`); the log line is `order complete: failed payment` (info), as case 1.
- **D12. Confirm (open):** the operator confirms that the payment service reads a gRPC error of `OrderComplete` as `failed` and refunds it (the code of the payment service is not here; D10 rests on the code of the three other services). The approval did not answer it; it stays open in `OPEN_TASKS.md` for the stage QA.

### The Parsian status worker (async)

- **D13. No code change.** The worker reads the `pending` and `in-progress` orders by provider only (`pendingIDsSQL`, `src/system/postgres/order_repository.go:90`), so a water order of `parsian` is in its window. It asks `TopBill/OrderInquiry` with `order_id` and `bill_type` `water` (`bill`: `ParsianPaymentInquiry`, the same fields). The report message (`reconcile.StatusMessage`) has only `payload.order_id`, `status` and `step_status.service`, so a water order gives the same bytes as a tel order, on `kafka.topic.payment.write.subject` (default `paymentwriter`, `bill`'s value) with the key `service.ecosystem_name`. No new key. A new test checks a water order end to end: a `pending` water order is finalized `success` and `failed` and reported.

### Tests, docs, metrics

- **D14.** Unit tests: the water request check and the inquiry ID, the price rules (raw first, the provider inquiry with `caching.paid_water`), the already-paid window, the token `data` map (with and without a fee), `parsianproxy.Pay` for `water` (`inquiry_type` `regular`), `PayRoutes`, no settlement after a water success, the case 3 answer (with a positive control). Live tests (throw-away database, conventions § 14): a water order and its payment rows for `success`, `failed` (also a second call after `failed`) and `pending`, and the worker on a water order. The metrics are the existing ones with `bill_type="water"` (conventions § 12); the spans are the existing ones (conventions § 17). Docs in the same commit: `config.yml.example` (the two keys), `system.html` (the water order and payment), `this_vs_bill.md` (item 38 extended, item 46), the Postman collection (Water / Order V2), the CHANGELOG, the manifest and the QA scenario in `OPEN_TASKS.md`.
