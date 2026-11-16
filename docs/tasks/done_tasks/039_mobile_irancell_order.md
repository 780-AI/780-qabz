<!-- VERSION: 0.2 -->
# 039_mobile_irancell_order — POST /v2/mobile/order for the Irancell operator (the base of the payment tasks)

Status: done (2026-11-14; the QA scenario waits for the operator confirmation)

## Description

This is the first payment task of Qabz. It makes `Phone.NewOrderV2` (`POST /v2/mobile/order`, `pb/proto/qabz/services.proto`) answer for the **Irancell** operator: the request is checked, an order is saved, an order token (a signed JWT) is made and returned to the client. The request is `NewOrderPhoneRequest` (`phone`, `type`, `operator`, `paymentType`, `price`, `payId`, `billId`) and the response is `NewOrderResponse` (`serviceId`, `orderId` = the token, `price`), both in `pb/proto/bill/services.proto`. Requests and responses are not changed.

**`bill` is the reference. The sequence MUST be the sequence of `bill`, step by step** (`controller/phone/newOrder.go` `NewOrderV2` and `createNewOrderPhone`; `service/billPayment/newOrder.go` `NewOrder`):

1. **Bill type.** The operator `irancell` gives the bill type `irancell` for `midTerm` and for `fullTerm` (`phoneBillTypeMapper`).
2. **Request check.** The phone is normalized first, then checked (`validateNewOrderPhoneRequest`: the operator enum, the payment type enum, a mobile number in the 98 form, `billId` or `phone`). A failure is the invalid-parameter answer.
3. **Provider pick.** The provider of (`irancell`, `order`) (seed row 26), through `ProvidersService` (conventions § 13). No enabled provider is an error answer.
4. **Price.** The first rule that applies:
   - an empty `payId` and a `price` that is not 0: the price is the `price` of the request and the pay ID is the text `irancell` (no inquiry);
   - a numeric `billId` and a numeric `payId`: the raw inquiry (`inquiry.Service.RawInquiry`, task 023);
   - otherwise: a provider inquiry with the action `order`, and the inquiry of the type that the bill type gives.
5. **Zero check.** A price of 0 is an error answer (`AmountEqualToZero`).
6. **Fee.** The fee resolver gives the fee, the fee type, the applied fee config and the fee ID (see below).
7. **Total.** `price` = base price + fee + provider fee (the provider fee is 0 for Irancell). A total below 0 or above the `uint32` field is the invalid-parameter answer.
8. **Order row.** One row in `orders` with the status `initial`: a new order ID, the user, the bill type, the bill ID, the pay ID, the price, the provider ID and name, the option (the operator), `data` (the phone), the fee columns, `base_price`, `is_raw`, `provider_fee`, `bill_inquiry_id`.
9. **Token.** The payload is `serviceId`, `orderId`, `provider`, `price`, `userId` and the `data` map of `NewOrderV2` (`type`, `phone`, `service`, `billId`, `payId`, `payAmount`, `rawAmount`, `about`, `aboutIcon`, `receiptMobile`, `invoiceMobile`; `feeAmount` and `feeDescription` when the fee is above 0). It is signed with RS256, with the header `kid` and the type `JWT` (`pkg/crypt/jwtSign.go`, `controller/base.go`).
10. **Answer.** `serviceId` (`service.id`), `orderId` = the token, `price` = the total.

**Operator decisions (2026-11-14):**
- **Only V2.** `bill` has `NewOrder` (no fee, five token keys) and `NewOrderV2` (the fee, the full token `data` map). Qabz keeps one version: V2. `POST /v1/mobile/order` stays `Unimplemented`.
- **Only Irancell.** The operators `mci` and `rightel` answer `Unimplemented` (HTTP 501) until their own tasks; `unknown` is the invalid-parameter answer.
- **The `price` of the request is used as in `bill`** (rule 4, first case).
- **Not in this task:** `POST /v1/mobile/complete-order`, the payment step on NATS, the order status cron, the "bill already paid" check (`shouldSkipNewOrder`: `bill` runs it only for Parsian, EWays and Jiring Switch) and the provider fee of Karashop.
- **The signing key is the general private key that `bill` uses.** A number of services share this key; Qabz uses it exactly as `bill` does: the path of the key file is a config key (`bill`: `paymentServer.privateKeyPath`; the Qabz key name follows conventions § 8), the key is read at start, the algorithm is RS256, the header `kid` is the service name of the config and the type is `JWT`. Qabz makes no key of its own. The key file is not in git; tests use a generated key.
- **A failed fee calculation gives a fee of 0 and the order continues, as in `bill`.** Qabz also writes one error log line and counts it in a metric (`bill` writes nothing).
- **Price and errors: the same behavior as `bill`, exactly.** Qabz copies each price rule and each error answer of `bill`, also where the result of `bill` is an error. For Irancell this includes the third price rule: an order with no `price` (and no numeric `billId` and `payId`) makes the provider inquiry with the action `order`, looks for an inquiry of the type `regular` (the bill type `irancell` is not in `BillTypeInquiryType`), finds none, because the Irancell inquiries have the types `midterm` and `final_term`, and gives the internal-error answer of `bill`. The request `type` does not choose an inquiry. (Read in the `bill` code on 2026-11-14, not run.) A difference from `bill` in price or error needs a new operator decision.
- **No fee cache for now.** `bill` caches the resolved fee and the list of users with a fee in Redis (`caching.provider`). Qabz reads the `fees` table on each order. A todo records the open decision (see the Definition of Done).

**This is the base task of every later payment task.** Each part below is built once, with no Irancell rule inside it, so that the order tasks of the other operators and bill types add only their own entry and their own handler:
- `core/order`: the order flow of steps 3 to 8 (a table of bill types as in `core/inquiry`, the order repository on `orders`).
- `core/fee`: the fee resolver and the fee calculation.
- `common/crypto`: the order token signer.
- The token payload and the `data` map builder; the metadata of a bill type (`about`, `aboutIcon`, `feeDescription`: `bill` `pkg/helper/helper.go` `GetBillMetaData`).

**The fee (`bill`: `service/feeResolver/fee_resolver.go`, `pkg/helper/fee.go`, `domain/entity/fee.go`).** The `fees` table and the fee columns of `orders` are already in the dev schema (`src/system/postgres/devdb/schema.sql`).
- The lookup takes the first row that exists: the row of the user and the provider, the row of the user, the row of the provider. With no row the fee is the default fee of `bill` (9000).
- The calculation by the `type` and `config` of the row: `fixed` (`{"amount": N}`), `percent` (`{"percent": P}`: amount × P / 100, rounded down), `stair` (a list of steps with `min`, `max` and a `fixed` or `percent` rule; the first step that contains the amount).
- In `bill` a failed lookup gives the default fee, and a failed calculation (a bad config, no step for the amount) gives a fee of 0; the order continues in both cases.

**The dev seed of `fees`.** `devdb seed` also inserts fee rows (dev mode only, as the other seeds; `unseed` deletes them). The rows use the default values of `bill`, and at least the order provider of Irancell (row 26) has a row, so that an order in development takes its fee from the database (`orders.fee_id` is set) and not from the default.

Help:
- `bill`: `controller/phone/newOrder.go`, `controller/base.go` (`GenerateJWT`), `service/billPayment/newOrder.go`, `service/feeResolver/fee_resolver.go`, `pkg/helper/fee.go` and `fee_test.go`, `pkg/helper/helper.go`, `pkg/helper/phoneNumberNormalizer.go` (`Replace98`), `pkg/crypt/jwtSign.go`, `domain/entity/payment.go` (`InitialOrderEnc`), `domain/entity/order.go`, `domain/repository/order/order_dto.go`, `app/application_dto.go` (the key is read at start), `config.example.yaml` (`paymentServer.privateKeyPath`).
- Qabz: `src/entry/grpc/phone.go` (the handler, the operator table), `src/core/inquiry` (the pattern of a shared flow), `helpers.NormalizeMobile` (task 038), `src/system/postgres/devdb/` (the seed files and their tests), the empty folders `src/core/order`, `src/core/fee` and `src/common/crypto`.
- Task 035 (`docs/tasks/done_tasks/035_mobile_irancell_inquiry.md`) is the pattern of an Irancell task; its follow-up (a) names the Irancell config keys that the order flow needs.
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice). When Qabz copies a rule from `bill`, a differential test against the `bill` code decides it (task 023 practice).

Points for the design step (the operator approves the design before the code):
- The exact error text and code of each failed step come from `bill` (operator rule: the same behavior exactly); the design lists them.
- **The request check of `bill` reads the payment type with the name table of the payment terms** (`ProtoBuffEnumValidate(pb.NewOrderPhoneRequest_PaymentTerms_name, paymentType)`): Qabz gives the same result as `bill` for each payment type value (operator rule); a differential test against the `bill` check decides it.
- The answer when the token cannot be signed is the internal-error answer, as in `bill` (`GenerateJWT`).
- The rows of the fee seed (which providers, which types) and their IDs.
- Metrics and log lines (conventions §§ 10, 12): orders by bill type, provider and result; the fee source (user row, provider row, default, error).

## Definition of Done

- `POST /v2/mobile/order` with the Irancell operator works as described: the steps of `bill` in the same order, one `orders` row with the status `initial`, a signed token and the total price in the answer.
- A test checks the token with the public key: the header (`alg`, `kid`, `typ`) and each payload field, with and without a fee.
- `core/order`, `core/fee`, the token signer and the token payload builder exist once and have no Irancell rule inside them; a later order task adds an entry, not a copy.
- The fee resolver gives the fee of `bill` for each lookup case and each fee type; the cases of `bill`'s `fee_test.go` pass, and a differential test against the `bill` code found no difference (the test is not kept).
- `devdb seed` inserts the fee rows in dev mode only and `devdb unseed` deletes them; the seed tests cover the new file.
- **The fee cache todo exists in three places:** a `@todo` at the fee resolver code, an item in `docs/this_vs_bill.md` (bill caches in Redis, Qabz reads the table on each order), and a follow-up line in `docs/Plans/OPEN_TASKS.md` that names the decision (no cache, an in-memory list with a reload interval as `ProvidersService`, or Redis).
- `mci` and `rightel` answer HTTP 501, `unknown` answers HTTP 400, `POST /v1/mobile/order` answers HTTP 501; no row and no token in each case.
- The signing key (the general key that `bill` uses) is read at start from the path of the config; a missing or bad key stops the App with a clear error. `config.yml` and `config.yml.example` are updated (conventions § 8). No private key is committed.
- A failed fee calculation gives a fee of 0, one error log line and a metric count; the order is saved.
- Unit tests with mocks and stubs (no database): each price rule, the zero price, the total above the field, no enabled provider, a failed insert, a failed signature, each fee case, an invalid request, each operator value. Integration tests (real Postgres, throw-away schema, conventions § 14): the order row and its fee columns, a fee from a `fees` row and the default fee.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection (`irancell_mobile_*` variables) and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database after `devdb seed`, with a test user and the signing key of the config (a generated key is enough in development).

1. Call `POST /v2/mobile/order` with `{"phone":"989350000391","operator":"irancell","type":"fullTerm","paymentType":"direct","price":450000}`. Pass: HTTP 200; `price` = 450000 + the fee of the seed row; one `orders` row (`initial`, bill type `irancell`, provider 26, `pay_id` `irancell`, `base_price` 450000, `fee` and `fee_id` of the seed row); the token passes the check with the public key and its payload has the same order ID, price and `data` values.
2. Delete the fee row of provider 26 (or run with a provider that has no row), and call again with a new phone. Pass: the fee is the default fee and `orders.fee_id` is NULL. Restore the row.
3. Call with the phone in the 0 form (`09350000391`). Pass: the same result as step 1, with the 98 form in the row and in the token.
4. Call with no `price` and no `payId` (the mock of the Irancell inquiry enabled). Pass (corrected by design decision 1: `bill` takes the final-term inquiry): HTTP 200; `price` = the final-term amount (450000 with the mock) + the fee; the row has `bill_inquiry_id` of the final-term row; one provider inquiry, also when a saved inquiry is valid.
5. Disable row 26, wait for the provider reload, call with a new phone. Pass: the error answer of "no enabled provider"; no `orders` row. Enable the row again.
6. Call with `"operator":"mci"`, then `"operator":"rightel"`, then `POST /v1/mobile/order`. Pass: HTTP 501 each time, no `orders` row.
7. Start the App with no signing key. Pass: the App stops with the error of the missing key.

## Design decisions (implementer, 2026-11-14; operator approval: "For all Ds, follow bill. Also improve on error messages as mentioned in d3")

The `bill` differences that a client or the database can see are items 38 and 39 of `docs/this_vs_bill.md` (and the additions to items 34 and 35), each with a `@todo` at the code.

1. **D1, a correction of this task file: the third price rule of Irancell succeeds in `bill`.** The Description says that `irancell` is not in `BillTypeInquiryType`; the `bill` code (commit `ff98e25`, `domain/entity/inquiry.go:38`) has `Irancell: FinalTermInquiryType`. So an order with no `price` (and no numeric `billId` and `payId`) makes the Irancell inquiry and takes the final-term inquiry as the price, also for the type `midTerm`; `bill_inquiry_id` is that row. Qabz follows `bill`. Verification step 4 is changed to match (HTTP 200).
2. **D2, the error answers (operator: improve them).** As the inquiry endpoints: an invalid request, the operator `unknown`, a payment term that the operator does not have and a total that does not fit are HTTP 400 `invalid parameter` (no log line); a price of 0 is `FailedPrecondition` (HTTP 400) with `bill`'s text `مبلغ قبض صفر میباشد` (no log line); no enabled order provider is HTTP 503 (one error line); a failed inquiry has the answer of `POST /v1/mobile/inquiry`; an answer with no final-term inquiry, a failed read or write and a failed token are HTTP 500 (one error line); no user is HTTP 403. `bill` answers each of them with gRPC `Unknown` (HTTP 500). Item 38.
3. **D3, the operator `unknown`.** It passes `bill`'s request check (`ProtoBuffEnumValidate` skips the value `unknown`) and fails at the provider lookup (HTTP 500); Qabz answers HTTP 400 (part of D2, item 38).
4. **D4, no saved inquiry for an order (follows `bill`).** `bill`'s order cache has no TTL for `irancell`, so the inquiry of an order always calls the provider. New field `inquiry.Request.NoCache`; the answer is still saved.
5. **D5, the provider fee (follows `bill`).** The `bill_inquiries` row of an inquiry price is read (`OrderRepository.InquiryProvider`); a failed read fails the order (HTTP 500). The provider fee is 0 for every provider of the flow (`bill`: only Karashop tel). With `debug.cache_inquiries: false` (development only) the inquiry is not saved (`inquiry.Result.Unsaved`), so the order has no `bill_inquiry_id` and reads no row (item 34; found in the feature-ON run with the operator's local config).
6. **D6, the fee (follows `bill`).** `core/fee`: the lookup of `bill` (user and provider, user, provider; the user rows only for a user in the list of users with a fee), the newest row that is not deleted (Qabz adds `id DESC` for rows with the same `created_at`, as item 21), the default 9000 (no type, config, ID), a failed read gives the default, a failed calculation gives 0 with the type `fixed` and the row ID; the calculation of `fixed`, `percent` and `stair` with the JSON rules of `encoding/json` v1 (`DisallowUnknownFields`, keys without case, the last of two keys wins, integers only), read with fastjson (a hot path, conventions § 1). Each failure writes one error line (`bill` writes nothing). No cache: the `fees` table is read on each order (item 39; follow-up (a) in the register). A temporary differential test against `bill`'s own `pkg/helper/fee.go` (2 000 000 random configs, 104 818 with a fee) found no difference; a deliberate change (the `min` key with case) made it fail (positive control). The test is not kept.
7. **D7, the fee seed.** One row: id 900001 (high, so that it does not collide with a row added by hand), provider 26, `fixed` `{"amount": 9000}`; `unseed` deletes only that id; the fees id sequence is not moved.
8. **D8, the token.** `common/crypto.Signer`: RS256 with the standard library (`crypto/rsa`, no new module), the header `{"alg":"RS256","kid":<service.name>,"typ":"JWT"}` (the bytes of go-jose v2: sorted keys), the key a PKCS #1 PEM file at `providers.internal.payment_server.private_key_path`, read at start (no path, a missing file or a bad key stops the App). The payload (`order.MarshalPayload`) is `encoding/json/v2` with sorted map keys and the escapes of `encoding/json` v1, so the bytes are those of `bill`'s `json.Marshal`. go-jose itself was not run (not in the local module cache; follow-up (c)).
9. **D9, metrics and logs.** `qabz_order_create_total{bill_type,provider,result}` (`created`, `invalid`, `no_provider`, `zero_price`, `inquiry_failed`, `error`) and `qabz_fee_resolve_total{source}` (`user_provider`, `user`, `provider`, `default`, `lookup_error`, `calc_error`). The planned `qabz_orders_total` counts status changes and comes with the order status work. Log lines: `irancell: no enabled provider for order`, `irancell: order inquiry has no inquiry of the bill type`, `irancell: order failed`, `irancell: order token failed`, `irancell: order fee failed`, `fee: read the users with a fee failed`, and the inquiry lines of the mobile inquiry.
10. **D10, the shared parts.** `core/order` (the flow; `billTypes` has the one entry `irancell`: the direct price rule and the final-term inquiry type; `token.go`: the payload, the `data` map of a mobile order and the metadata of `GetBillMetaData`), `core/fee`, `common/crypto`, `postgres.OrderRepository`, `postgres.FeeRepository`. The handler table `orderOperators` (`entry/grpc/phone_order.go`) has the entry of Irancell; `mci` and `rightel` are HTTP 501; `POST /v1/mobile/order` stays HTTP 501.

## Verification record (implementer, 2026-11-14)

- `make test GO_TEST_ARGS="-race -count=1 -cover -v"` (compose Postgres, a run database `qabz_test_<16 hex>` created and dropped): every package passes except `provider/karashop` `TestInquiryOneCallFails` (not changed by this task; flaky: 1 failure in 8 runs alone, register follow-up (d)). Coverage of the changed packages: `app` 87.2%, `common/crypto` 91.2%, `core/fee` 96.6%, `core/inquiry` 99.3%, `core/order` 98.9%, `entry/grpc` 98.9%, `system/postgres` 91.9%, `system/postgres/devdb` 82.4%, `cmd/devdb` 72.5% (gate 70.0%).
- Feature ON (Rule 15) on an App copy (ports 18080 / 18082 / 19090, a scratch config from the local `config.yml`, a generated key, the Irancell mock, a new test user), against the compose dev database after `devdb seed` (it printed `fees=1`):
  1. The order with `price` 450000: HTTP 200, `price` 459000, `serviceId` 106; the row `initial`, `irancell`, provider 26, `pay_id` `irancell`, `bill_id` NULL, `base_price` 450000, `fee` 9000, `fee_type` `fixed`, `fee_id` 900001, `data` `989350000391`, `option` `irancell`; `openssl dgst -sha256 -verify` with the public key: `Verified OK`; header `{"alg":"RS256","kid":"qabz","typ":"JWT"}`; the payload has the order ID of the row, price 459000 and the `data` map of `bill` (`feeAmount` 9000, `feeDescription`, `receiptMobile` `09350000391`).
  2. The fee row soft-deleted: HTTP 200, `price` 459000, `fee` 9000, `fee_type`, `fee_id` and `fee_config` NULL; the row restored.
  3. The phone `09350000391`: the same row values and token data as step 1.
  4. No `price`, type `midTerm`: HTTP 200, `price` 459000; the row has `base_price` 450000 and `bill_inquiry_id` of the final-term `bill_inquiries` row (amount 450000); one provider call (mock).
  5. Row 26 disabled, after the provider reload (18 s): HTTP 503 with the provider-down text, no row, one line `irancell: no enabled provider for order`; the row enabled again.
  6. `mci` and `rightel`: HTTP 501; `POST /v1/mobile/order`: HTTP 501; `unknown`: HTTP 400.
  7. No key path: exit 1, `app: order token: crypto: no private key path (set providers.internal.payment_server.private_key_path)`; a missing file: exit 1, `... crypto: read private key <path>: ... no such file or directory ...`.
  Also: with `debug.cache_inquiries: false`, no `price`: HTTP 200, `bill_inquiry_id` NULL, no `bill_inquiries` row. Metrics at the end of the run: `qabz_order_create_total{...,result="created"} 4`, `{provider="none",result="no_provider"} 1`, `qabz_fee_resolve_total{source="provider"} 3`, `{source="default"} 1`. The test rows (5 orders, 2 inquiries, 1 request row) were deleted; the fee seed row stays.
