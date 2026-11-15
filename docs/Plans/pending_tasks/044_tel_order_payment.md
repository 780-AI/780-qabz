<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-15 — Created (operator request): part 1 of supertask 043.
-->
# 044_tel_order_payment — tel order and payment through Parsian, Karashop settlement at once

Status: pending

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
