<!-- VERSION: 0.1 -->
# 039_mobile_irancell_order — POST /v2/mobile/order for the Irancell operator (the base of the payment tasks)

Status: pending

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
- Task 035 (`docs/Plans/done_tasks/035_mobile_irancell_inquiry.md`) is the pattern of an Irancell task; its follow-up (a) names the Irancell config keys that the order flow needs.
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
4. Call with no `price` and no `payId` (the mock of the Irancell inquiry enabled). Pass: the internal-error answer of `bill` after one provider inquiry; no `orders` row and no token.
5. Disable row 26, wait for the provider reload, call with a new phone. Pass: the error answer of "no enabled provider"; no `orders` row. Enable the row again.
6. Call with `"operator":"mci"`, then `"operator":"rightel"`, then `POST /v1/mobile/order`. Pass: HTTP 501 each time, no `orders` row.
7. Start the App with no signing key. Pass: the App stops with the error of the missing key.
