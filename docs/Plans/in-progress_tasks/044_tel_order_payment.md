<!-- VERSION: 0.2 -->
<!--
CHANGE LOG
v0.2 — 2026-11-15 — Started (operator: "Yes, start 044"): moved to in-progress_tasks; design written (corrections C1 to C4, D1 to D17; operator approval "Approve, all (a)": D6 (a), D8 (a), D14 (a)).
v0.1 — 2026-11-15 — Created (operator request): part 1 of supertask 043.
-->
# 044_tel_order_payment — tel order and payment through Parsian, Karashop settlement at once

Status: in-progress (design approved 2026-11-15; implementation next)

## Description

Part 1 of supertask 043 (`043_tel_payment.md`; read it first). The flow is the flow of `bill`, step by step, on the shared parts of task 039 (`core/order`, `core/fee`, the token signer):

1. **Order.** `Tel.NewOrderV2` (`POST /v2/tel/order`, request `NewOrderTelRequest`) makes an `orders` row with the status `initial` and returns the signed order token, as `POST /v2/mobile/order`. The order provider is Parsian (seed rows 50 `tel` and 51 `telmidterm`, action `order`), picked through `ProvidersService` (conventions § 13). The Karashop provider fee of `bill` (tel only) is in scope. `POST /v1/tel/order` stays `Unimplemented`, as in task 039.
2. **Payment.** The payment service calls `PaymentComplete/OrderComplete` (`pb/proto/qabz/internalservices.proto`; `bill`: `controller/payment/orderComplete.go`, `service/billPayment/completeOrder.go`). Qabz checks the order (status `initial`, the same price), saves `in-progress` with the RRN, pays through Parsian (`InternalInquiryService/PayBill` of parsian-v2), and saves the status of the answer: `success`, `failed`, or `pending` for each other answer (a timeout too). The status goes back in the answer of `OrderComplete`. The payment service refunds a `failed` payment (`bill` never refunds by itself; supertask 043).
3. **Karashop settlement at once.** On `success`, Qabz sends the payment info to Karashop (`SendPaymentInfo`) with no wait for the answer to the payment service, as `bill` (`go s.ConfirmPayment`). On a Karashop success, `provider_confirmed = true`. A failed settlement leaves `provider_confirmed = false` for the worker of task 046.

**Mocks (conventions § 15).** The Parsian mock gets the payment answers `success`, `failed` and `pending` (a timeout). Today `return_type` has only `success` and `error`, so the design step changes conventions § 15 or adds a key for the payment answer. The Karashop mock gets the settlement answers `success` and `error`.

Points for the design step (the operator approves the design before the code):
- The exact request check, price rules, error answers and the token `data` map of the tel order in `bill` (`controller/telephone/`), with the same rule as task 039: the same behavior as `bill`, exactly.
- The Parsian status names and the mapping of each Parsian answer code to `success`, `failed` or `pending` (`bill`: `gateway/payment/`, `validStatuses`).
- The new Karashop config key for the settlement URL (`bill`: `thirdparty.karashop.billPaymentInfoURL`) under `providers.external.karashop`.
- Which entry the payment service calls: `PaymentComplete/OrderComplete` (gRPC, as `bill`) or `POST /v1/tel/complete-order`.
- The "bill already paid" check (`shouldSkipNewOrder`): `bill` runs it for Parsian, so it is in scope here.
- Metrics and log lines (conventions §§ 10, 12): payments by bill type, provider and status; settlements by result.

## Definition of Done

- `POST /v2/tel/order` works for `tel` and `telmidterm` as in `bill`: one `orders` row `initial`, a signed token, the total price.
- `OrderComplete` for a tel order saves `in-progress`, pays through Parsian and saves and returns `success`, `failed` or `pending`. A second call for the same order and a wrong price give the answers of `bill`; Parsian is not called.
- On `success`, the Karashop settlement runs at once; a Karashop success sets `provider_confirmed = true`; a Karashop failure leaves it `false`, writes one error line and counts a metric.
- The Parsian mock answers `success`, `failed` and `pending`; the Karashop mock answers `success` and `error`; both only in dev mode (conventions § 15).
- Unit tests with mocks and stubs for each step and each status; integration tests (real Postgres, throw-away schema, conventions § 14) for the order rows of each status and `provider_confirmed`.
- Build, vet and tests (with `-race`) pass. `config.yml.example`, `system.html`, `this_vs_bill.md`, the Postman collection, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database after `devdb seed`, with the Parsian and Karashop mocks.

1. Make a tel order, then call `OrderComplete` with the Parsian mock at `success` and the Karashop mock at `success`. Pass: the order rows `initial`, `in-progress`, `success`; the answer status `success`; `provider_confirmed = true`.
2. The same with the Karashop mock at `error`. Pass: status `success`; `provider_confirmed = false`; one error line; the settlement metric counts the failure.
3. The Parsian mock at `failed`. Pass: the last row and the answer are `failed`; no Karashop call.
4. The Parsian mock at `pending` (timeout). Pass: the last row and the answer are `pending`; no Karashop call.
5. Call `OrderComplete` again for the order of step 1, then with a wrong price for a new order. Pass: the answers of `bill`; no Parsian call.
6. The real payment through the payment service, Parsian and Karashop is done by the operator (supertask 043).

## Design (implementer, 2026-11-15; operator approval 2026-11-15: "Approve, all (a)")

Read on 2026-11-15 from `bill` commit `ff98e25` (not run) and from the Qabz source at `51034bc`. Items marked **(decision)** need an operator choice; every other item follows `bill`. Each difference from `bill` that a client, the payment service or the database can see gets an item in `docs/this_vs_bill.md` and a `@todo` at the code.

### Corrections of this task file

- **C1. The Parsian RPC is `TopBill/Pay`** (`pb/proto/parsian-v2/internalBillService.proto`, `PayRequest` / `PayResponse`), not `InternalInquiryService/PayBill`.
- **C2. A `failed` payment is a gRPC error, not a status in the answer.** `bill` (`controller/payment/orderComplete.go`) returns `errors.New(order.Message)` (gRPC `Unknown`) for `failed`. It returns the `PaymentOrder` with `status` and `stepStatus.service` only for `success` and `pending`. Verification step 3 and the Definition of Done change to match.
- **C3. A second call and a wrong price are not errors.** `bill` (`orderValidator`) answers `pending` with the message `order has been processed before` or `payment price is not equal to order price`. Nothing is saved, and Parsian is not called.
- **C4. The seed row 50 (`tel`, `order`, Parsian) has the status `disable`.** Row 51 (`telmidterm`) is `enable`. With the seed, `POST /v2/tel/order` for `fullTerm` has no order provider (HTTP 503). The verification enables row 50 for the test. The seed does not change.

### The order (`POST /v2/tel/order`)

- **D1. The request check (`bill`: `createNewOrderTelephone`).** `phone` goes through `NormalizePhoneNumber` of `bill` (98 prefix, or a 10 or 11 digit number). The request needs `billId` or `phone`. `paymentType` must be a known enum value. The bill type comes from `type` (`midTerm` gives `telmidterm`, `fullTerm` gives `tel`), not from `paymentType`, as in `bill`. A failed check is `invalid parameter` (HTTP 400), as in task 039 D2.
- **D2. The "bill already paid" check (`shouldSkipNewOrder`).** The order provider is Parsian, so Qabz reads the newest `orders` row with the `billId` and `payId` of the request, created in the last `providers.internal.parsian.settlement_inquiry_telephone`. The key exists and is not read yet; `bill` has `360h`, and Qabz gets the same default. A last status of `pending` or `success` gives `bill`'s error code 8 with the text `قبض وارد شده قبلاً پرداخت شده است، لطفا تا تسویه شدن قبض صبر کنید`. Qabz answers it as `FailedPrecondition` (HTTP 400), with the same rule as task 039 D2. A failed read does not stop the order.
- **D3. The price: the inquiry comes first.** For `tel` and `telmidterm`, `bill` makes the provider inquiry before the raw-inquiry rule (`AvailableBillTypes`: the inquiry provider is Karashop, which has a provider fee). `core/order` gets the flag `inquiryFirst`. The inquiry type is `final_term` for `tel` and `midterm` for `telmidterm`.
- **D4. Saved inquiries for an order.** `bill` reads saved inquiries for an order for `caching.paidTel: 72h`. Qabz adds `caching.paid_tel: "72h"`. The order flow passes this TTL, in place of `NoCache` (task 039 D4).
- **D5. The `telmidterm` inquiry.** Qabz has one inquiry bill type, `tel`: one Karashop answer has the mid-term and the full-term inquiry (task 025). A `telmidterm` order makes the inquiry of `tel` and takes the `midterm` inquiry. The rows, the price and the provider are the same as in `bill`. This is not a `this_vs_bill.md` item.
- **D6. The Karashop provider fee (`feeableBillType`) (decision).** `bill` reads the `bill_inquiries` row of the price. For the provider Karashop and the bill types `tel` and `telmidterm`, `bill` does this:
  - `extra_data.service_status` is `partiallyDisconnected` or `fullyDisconnected` (Karashop `serviceStatus` `قطع یکطرفه` or `قطع دوطرفه`): the fee is `providers.provider_fee` of the inquiry provider row. The seed has NULL there, so the fee is 0.
  - Any other status: a fixed **21000**, with the comment `todo: we have to remove that, tis so wrong`.
  
  So a connected line pays 21000 more, and a disconnected line pays 0 with the seed. The Qabz Karashop adapter (task 025) does not save `serviceStatus`. This task adds the telephone details (`provider.TelDetails{ServiceStatus}`, saved in `extra_data` as `service_status`, with `bill`'s map of the three texts). Choose one:
  - **(a, recommended)** Copy `bill` exactly: 21000 as a constant, with a `@todo`.
  - **(b)** Put the 21000 in a config key, `providers.external.karashop.default_provider_fee`, with the default 21000.
- **D7. The token `data` map (`bill`: `NewOrderV2`).** The keys are `type`, `phone`, `billId`, `payId`, `payAmount`, `rawAmount`, `about`, `aboutIcon`, `invoicePhone`, `receiptPhone` (`Replace98` of the phone). When `fee > 0`, the map also has `feeAmount` (fee + provider fee), `feeDescription` and `hafhashtadFee`. When `providerFee > 0`, it also has `providerFee`. Note: when the fee is 0, `feeAmount` does not contain the provider fee, as in `bill`. `POST /v1/tel/order` stays `Unimplemented`.

### The payment (`OrderComplete`)

- **D8. The entry: a second, internal gRPC server (decision).** `bill` registers `PaymentComplete` only on its internal gRPC server (`service.internalGrpc`, port `:8082`). That server has no JWT check and no HTTP gateway. Qabz has one gRPC server (`server.grpc.addr`, `:8082`), and that server has the auth check. Choose one:
  - **(a, recommended)** A new key, `server.internal_grpc.addr`. The server has no auth check and only `PaymentComplete`, and it gets the request metrics and the recovery. An empty value (the default) means no internal server, so the change has no effect until the key is set.
  - **(b)** `POST /v1/tel/complete-order` on the public server. This is not `bill`'s entry.
- **D9. The request check.** `payload.serviceId` must equal `service.id`, `stepStatus.payment` must be `success`, and `payload.rrn` must not be empty. Else Qabz answers `invalid parameter` and writes one error line. `version` + 1 and `currentStep` = `service.name` are set on the answer.
- **D10. The steps (`bill`: `CompleteOrder`).** One new `orders` row for each status, with a new `uuid` and a new `created_at`, as in `bill`:
  1. Read the newest row of the `orderId`, created in the last 48 h. None found, or a failed read: the database error.
  2. The status is not `initial`, or the price is wrong: answer `pending` (C3).
  3. Read the provider row. A failed read: save `failed` (`error in getting provider from database`, 400) and answer the database error.
  4. Save `in-progress` with the RRN. A failed write: save `failed` and answer the database error.
  5. Pick the payment adapter by bill type and provider name. None found: save `failed` and answer `invalid parameter`.
  6. Pay through Parsian. The status of the answer is kept when it is `success`, `failed` or `pending`; any other status is `pending`. Save the status, `ref_id` (`provider_trace_id`), `message` and `status_code`. A failed write: answer `pending` with no saved row, as in `bill`.
  7. On `success`: mark the inquiry as paid and update the last order date of the saved bill (D13), then start the Karashop settlement (D12).
- **D11. The Parsian adapter `Pay`.** `parsianproxy` implements `ProviderInterface.Pay` for `tel` and `telmidterm` (the stub types `PaymentRequest` / `PaymentResponse` get the fields). `bill` maps the fields so: `provider_order_id` = the RRN as an int, and the RRN must be numeric, else `failed` (400) with no call. `amount` = `base_price`. `inquiry_id` = `orders.data`. `inquiry_type` = `final_term` or `midterm`. A gRPC error (a timeout too) is `pending` (500) with the error text as the message. `bill` sets no own timeout. Qabz uses the existing `providers.internal.parsian.timeout` (`10s`). The payment service can wait longer than that, so this is a `this_vs_bill.md` item. `enabled: false` gives `pending` with no call.

### The Karashop settlement at once

- **D12. `SendPaymentInfo` (`bill`: `ConfirmPayment`, `gateway/confirmPayment`, `thirdparty/karashop`).** It runs only for `tel` and `telmidterm` with the order providers `parsian`, `eways` and `eways2`. The body is the service `tci-SendPaymentInfo-1` with the parameters `telNumber`, `amount` (`base_price`), `billId`, `paymentId`, `bankName` (`parsian` gives `parsian`; `eways` and `eways2` give `dey`), `referenceCode` (the RRN), `channelName` `IPG_ghabzino`, `paymentDate` (the Shamsi date `YYYYMMDD` of the `initial` row `created_at`) and `paymentTime` (`HHMMSS`). The URL is `providers.external.karashop.bill_payment_info_url`, which exists and is now read. The answer uses `bill`'s two code tables. Only `success` (HTTP 200 or 202) sets `provider_confirmed = true` on the `success` row (created in the last 48 h). It runs in its own goroutine: it does not wait for the answer to the payment service, it recovers from a panic, it has the Karashop timeout, and the App stop waits for it. Qabz adds one error line and a metric for each failure (`bill` writes no line).
  - The time zone of `paymentDate` and `paymentTime` is **Asia/Tehran**, set in the code. `bill` uses the time zone of the process.
- **D13. The `bill` side effects of a success (`updateCache`).** `bill` sets `is_paid` on the `bill_inquiries` rows with the inquiry ID and the pay ID. It also sets the last order date of the `bills` row (bill ID, bill type). Qabz does both, so a new inquiry of a paid bill shows it as paid. A failed update writes one error line and does not change the answer.

### Mocks, metrics, logs

- **D14. The mocks (conventions § 15 changes) (decision).** One new key for each provider method:
  - `debug.mocks.parsian.pay_return_type`: `success`, `failed` or `pending`. `pending` is the answer of a timeout, with no call.
  - `debug.mocks.karashop.settle_return_type`: `success` or `error`.
  
  The default is `success`. The existing `return_type` stays the key of the inquiry mock. § 15 gets the rule "one return-type key for each method, `<method>_return_type`". Choose one:
  - **(a, recommended)** Use this rule.
  - **(b)** One `return_type` key for all methods. This needs `failed` and `pending` also for the inquiry mock.
- **D15. Metrics (conventions § 12).**
  - `qabz_order_create_total` gets the bill types `tel` and `telmidterm`, and the result `already_paid`.
  - `qabz_order_payment_total{bill_type,provider,status}`, where `status` is `success`, `failed`, `pending`, `invalid`, `processed` or `error`.
  - `qabz_provider_pay_duration_ms{provider,status}`.
  - `qabz_order_settle_total{bill_type,provider,result}`, where `result` is `success`, `failed` or `pending`.
- **D16. Log lines.** `tel: order ...` as in task 039. `order complete: invalid payment request`. `order complete: failed payment` (info, as `bill`). `order complete: successful payment` (info). `settle: karashop failed` (error). `settle: mark paid failed` (error).

### Risk check (supertask 043)

- **D17. A separate database.** `config.yml.example` has `database.postgres.db: qabz`, so the `bill` workers do not read the Qabz `orders` rows. The operator confirms that stage and prod also use their own database.
