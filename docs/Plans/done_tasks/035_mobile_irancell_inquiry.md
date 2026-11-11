<!-- VERSION: 0.2 -->
# 035_mobile_irancell_inquiry — POST /v1/mobile/inquiry for the Irancell operator

Status: done

## Description

Task 024 (`mobile_inquiry_endpoint`) was split by operator into tasks 035 (Irancell), 036 (MCI) and 037 (Rightel). This task makes `Phone.Inquiry` (`POST /v1/mobile/inquiry`, `pb/proto/qabz/services.proto`) answer for the **Irancell** operator. The request is `InquiryPhoneRequest` (`phone`, `operator`, `payId`, `billId`, `service`) and the response is `InquiryPhoneResponse` (`midTerm` and `fullTerm`, each with `billId`, `payId`, `price`, `paymentDeadline`, `providerId`), both in `pb/proto/bill/services.proto`.

**Shared part of tasks 035, 036 and 037.** The three tasks have one endpoint. The first of them that starts builds the shared part; the other two add one operator to it and do not copy it.
- The handler and the request check of `bill` (`validateInquiryRequest`: `phone` or `billId`; `operator` or `service`; `phone` or `payId`; valid enum values; a valid phone number; a `billId` that is sent is a valid bill ID).
- The operator is `request.operator`, or `request.service` when the operator is empty (`setOperator`). One table gives the bill type of each operator (`bill`: `operatorBillType`): Irancell → `irancell`, MCI → `mobile`, Rightel → `mobilemidtermRightel`.
- The inquiry ID is the phone, or the bill ID when the phone is empty (`controller.SetInquiryID`).
- The flow is the shared inquiry flow of tasks 021 to 023 (`core/inquiry`: one more entry per bill type in `billTypes`; `ProvidersService.PickProvider`, conventions § 13; `bill_inquiries` first, the provider only on a miss, one `bill_inquiry_requests` row, the error mapping of task 021). Do not copy the flow.
- One provider call gives a mid-term and a full-term inquiry. They are saved as separate `bill_inquiries` rows and returned in `midTerm` and `fullTerm` (`bill`: `handleResponseWithInquiry`). An answer from saved inquiries returns both the same way.

**Provider constraint (operator, 2026-11-11): an Irancell inquiry MUST use the Irancell provider.** No other provider answers an Irancell inquiry (not Qabzino, not Parsian, not Jiring Switch). When the Irancell provider is not available, the answer is the provider-down answer (`Unavailable`); there is no fallback.

**What is special to Irancell:**
- **Bill type `irancell`, provider `irancell`** (seed id 38, `irancell-inquiry`, enabled). `bill` (`gateway/inquiry/mobile_irancell.go`, `IrancellMobile`) calls the `irancell` microservice (`BillInquiry` with the user ID, the dialer and the inquiry ID).
- **The answer has no pay ID and no bill ID:** each inquiry has only an amount and an inquiry type; a negative amount becomes 0. A failed call gives a pending answer, not a failed one.
- **The Qabzino rows for Irancell are not used:** the seed rows 31 (`mobileIrancell`) and 33 (`mobilemidtermIrancell`) have no gateway in `bill`, and the operator table never gives those bill types. Task 024 named them as the adapter of this operator; that was not correct.
- **The adapter is new.** Qabz has no Irancell adapter (`src/provider/irancellproxy/` is an empty folder).

Help:
- `bill`: `controller/phone/inquiry.go`, `controller/phone/phone.go`, `gateway/inquiry/mobile_irancell.go`, `gateway/inquiry/base.go` (`handlePhoneResponseWithoutInquiry`), `app/inquiryGateway.go`, `service/billInquiry/inquiry.go`.
- The `irancell` service: `service/bill/`.
- The seed: `src/system/postgres/devdb/seed_providers.sql` (ids 26, 31, 33, 38).
- Task 023 (`docs/Plans/done_tasks/023_gas_inquiry_endpoint.md`) is the newest pattern: its design decisions, the `billTypes` table, the mock, the Postman collection, the metrics. Tasks 030 and 034 are the pattern of a direct provider adapter.
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).

Points for the design step:
- How the flow keeps the constraint when another provider row is enabled for the bill type `irancell` (the pick must not give another provider to this operator).
- The adapter: a direct client of Irancell (as tasks 030 and 034 did for Qabzino and Tavanir) or a client of the `irancell` microservice. Its config keys under `providers.irancell` (conventions § 8).
- The cache TTL key of the bill type `irancell` and its default (see the caching maps of `bill` in `service/billInquiry/inquiry.go`).
- The answer for an operator whose task is not done (when this task is the first of the three).
- The `midTerm` and `fullTerm` of the answer when the provider gives one inquiry, or no debt.
- The fixed mock answer and the error of `return_type: error`.
- Metrics and log lines (conventions §§ 9, 12): the task 021 metrics with the bill type of this operator.

## Definition of Done

- `POST /v1/mobile/inquiry` with the Irancell operator works as described: database first, provider only on a miss, the mid-term and the full-term inquiry saved and returned.
- The shared part exists once (built here if this is the first of tasks 035 to 037, else reused with no copy).
- An Irancell inquiry is answered only by the Irancell provider: a test shows that no other provider is called, also when the Irancell provider is disabled or fails (the answer is `Unavailable`).
- The Irancell adapter is registered for (`irancell`, `inquiry`); it has a mock that works only in dev mode. `config.yml` and `config.yml.example` are updated (conventions § 8).
- Unit tests with mocks and stubs (no database, no provider): a valid row in the database (no provider call), no row (one provider call, rows saved), an expired row, a row of another provider, a paid row, a provider error, no enabled provider, an invalid request, each operator value; the adapter (request fields, answer mapping, negative amount, errors); the mock (`success`, `error`, off, rejected outside dev mode); the config keys.
- Integration tests (real Postgres, throw-away schema): the first request saves the rows, the second request within the TTL is answered from the database with no provider call.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with the mock of the Irancell adapter enabled. The seed already has the provider of (`irancell`, `inquiry`) enabled (id 38).

1. `return_type: success`. Call `POST /v1/mobile/inquiry` with a new `phone` and the Irancell `operator`. Pass: a success answer with the mock values in `midTerm` and `fullTerm`; one `bill_inquiry_requests` row and the `bill_inquiries` rows are in the database; a log line shows the provider (mock) call.
2. Call again with the same `phone` within the TTL. Pass: the same answer, no provider call (the counter `db_hit` shows the database source), no new provider request row.
3. `return_type: error`. Call with another new `phone`. Pass: the error answer; the request row has the status that the design step gives for a failed call; no `bill_inquiries` row is saved.
4. With `return_type: error`, call with the `phone` of step 1 within the TTL. Pass: the success answer from the database.
5. Disable the row 38 in the dev database (`UPDATE providers ...`) and wait for the provider reload. Call with another new `phone`. Pass: the provider-down answer (HTTP 503); no other provider is called; then enable the row again.

## Design decisions (implementer, 2026-11-11; operator approval "Approve as recommended")

The `bill` differences that a client or the database can see are items 30 and 31 of `docs/this_vs_bill.md`, each with a `@todo` at the code.

1. **The adapter calls our own `irancell` service (operator, 2026-11-11).** `provider/irancellproxy` (IrancellProxy, name `irancell` = seed row 38) is a gRPC client of `pb.InternalService/BillInquiry` (`pb/proto/irancell/internalService.proto`, read-only; `make proto` generates it into `src/api/pb/irancell` with an `M` option, because the proto names the Go package of another repo). Request: `user_id`, `dialer`, `inquiry_id` (the phone), as `bill`. Answer: one item for each inquiry (amount, `midterm` or `final_term`); a negative amount is 0; a success answer with no inquiry is a mid-term and a final-term inquiry of amount 0; no bill ID, no pay ID, no deadline. A failed call (no address, a connection error, a timeout, a nil answer) is `provider.ErrProviderDown`.
2. **Config.** The operator's keys of `bill` under `providers.irancell` (`name`, `grpc_address`, `nats_subject`, the `get_status` block; the list `syncOperations` is three keys under `get_status.sync_operations`), snake case as the other keys; only `grpc_address` is read now (the others belong to the order flow). New keys of the adapter: `providers.irancell.enabled` (default `false`; `false` sends nothing, every call is provider down, the switch wins over the mock) and `providers.irancell.timeout` (default `10s`, positive). Mock: `debug.mocks.irancell.enable` / `return_type`.
3. **Decision A, cache: `caching.irancell`, default `0` (no cache, as `bill`).** A negative value is rejected; not hot-reloadable (read at start, as `caching.water`). With a TTL, the saved mid-term and final-term rows of the phone answer with no provider call.
4. **Decision B, MCI and Rightel before tasks 036 and 037: `Unimplemented` (HTTP 501), no log line, no provider call.** The operator table `phoneOperators` (`entry/grpc/phone.go`) has an empty bill type for them; the operator `unknown` is not in the table: `InvalidArgument` (HTTP 400). As in `bill`, only `operator` is read (`service` never decides) and a request with no operator is MCI (the enum value 0). The gateway reads an operator name that is not in the enum as MCI (its default decoder drops unknown enum names).
5. **Decision C, a failed call: the request row is `failed`** (as every other bill type in Qabz; `bill` writes `pending`). Item 30 of `this_vs_bill.md`.
6. **Decision D, `providerId` in the answer: 0, as `bill`.** Requests and responses are not changed.
7. **Only the Irancell provider (operator rule).** The entry of the bill type `irancell` in `billTypes` (`core/inquiry`) has `onlyProvider: "irancell"`. When the pick of `ProvidersService` is another provider (for example a Qabzino row of the bill type `irancell`), the flow calls nobody, reads no saved inquiry, writes no row and answers as "no enabled provider" (HTTP 503, one error line `irancell: no enabled provider for inquiry`, `qabz_inquiries_total{provider="none",result="no_provider"}`). No fallback. The seed rows 31 and 33 (Qabzino, bill types `mobileIrancell` and `mobilemidtermIrancell`) are never picked: the operator table never gives those bill types.
8. **The shared part (first of tasks 035 to 037).** The handler `Phone.Inquiry` makes `bill`'s request check (`validPhoneInquiry`: a mobile number in the 98 form, which also covers "phone or billId" and "phone or payId"; a `billId` that is sent is a valid bill ID; enum values) and maps the answer: the `midterm` inquiry goes to `midTerm`, every other type to `fullTerm`; a part with no inquiry is an empty object. Tasks 036 and 037 add their bill type to `phoneOperators` and their entry to `billTypes`.
9. **Flow entry.** `billTypes["irancell"]`: `tid` prefix 8 (`bill`: `IrancellReference`), inquiry ID rule `helpers.ValidMobile`. `provider.BillTypeIrancell`, `provider.InquiryTypeMidterm` and `InquiryTypeFinalTerm` are new constants.
10. **Mock.** `irancellproxy.MockFixtures`: a mid-term inquiry of 150 000 rials and a final-term inquiry of 450 000 rials, trace `mock-irancell-0001`, no bill ID, no pay ID, no deadline. `return_type: error` is `provider.ErrProviderDown`.
11. **Metrics and logs.** No new metric: the three inquiry metrics count `bill_type="irancell"` (the Help text names it). Log lines: `irancell: no enabled provider for inquiry`, `irancell: provider inquiry failed`, `irancell: inquiry failed`, `irancell: inquiry amount does not fit the price field`.

## Verification record (implementer, 2026-11-11)

`make test GO_TEST_ARGS="-race -count=1 -cover -v"` (compose Postgres, run database `qabz_test_4b805ec640c02444`, created and dropped; 910 pass, 0 skipped): the live tests `TestLiveIrancellInquiryFlow`, `TestLiveIrancellCacheOff`, `TestLiveIrancellMockError` and `TestLiveIrancellOnlyIrancell` pass. One test fails and is not caused by this task: `TestLocalConfigInSync` (the local `config.yml` has the `providers.tavanir.circuit_breaker` block commented out; task 023 follow-up (c)). Coverage of the changed packages: `app` 88.7%, `config` 96.8% (measured with `-skip TestLocalConfigInSync`), `core/inquiry` 99.3%, `entry/grpc` 98.7%, `provider` 96.7%, `provider/irancellproxy` 98.1%.

Feature ON (Rule 15), the compose dev database, a copy of the App on ports 18080 / 18082 / 19090, `mode: dev`, `debug.auth.disable: true` with a test user, `providers.irancell.enabled: true` with an empty `grpc_address` (no call could reach the irancell service), `debug.mocks.irancell.enable: true`, `caching.irancell: 1h` (to exercise steps 2 and 4), `providers.qabzino.enabled` and `providers.tavanir.enabled` false. The seed has row 38 (Irancell, `irancell`, `inquiry`) enabled.

1. `return_type: success`, `POST /v1/mobile/inquiry` `{"phone":"989350000351","operator":"irancell"}`: HTTP 200 `{"midTerm":{"billId":"","payId":"","price":150000,"paymentDeadline":"","providerId":0},"fullTerm":{...,"price":450000,...}}`. Database: one `bill_inquiry_requests` row (`success`, `irancell`, bill type `irancell`, trace `mock-irancell-0001`, `tid` starts with 8); two `bill_inquiries` rows (`midterm` 150000 and `final_term` 450000, provider 38, `is_cached` false, `bill_id` and `pay_id` NULL). Log: one `provider mock: inquiry answered with no call to the provider`. PASS.
2. The same phone again: the same body; no second mock line; still one request row; two copy rows (`is_cached` true); `qabz_inquiries_total{bill_type="irancell",provider="irancell",result="db_hit"} 1`, `{...result="provider_ok"} 1`, `qabz_provider_calls_total{...provider="irancell",result="ok"} 1`. PASS.
3. Restart with `return_type: error`, `{"phone":"989350000352","operator":"irancell"}`: HTTP 503 with the provider-down text; the request row has `failed` and no trace; no `bill_inquiries` row; one error line `irancell: provider inquiry failed`; `provider_down` 1. PASS.
4. With `return_type: error`, the phone of step 1: HTTP 200 with the answer of step 1 from the database (`db_hit` 1). PASS.
5. Row 38 disabled (the Qabzino row 31 of `mobileIrancell` stays enabled), wait for the reload (`load_interval: 1s`), `{"phone":"989350000353","operator":"irancell"}`: HTTP 503; no request row; one error line `irancell: no enabled provider for inquiry`; `{provider="none",result="no_provider"} 1`. Row 38 enabled again. PASS. (A Qabzino row of the bill type `irancell` that the pick gives is tested in `TestLiveIrancellOnlyIrancell`: nobody is called.)

Also: `{"phone":"989121234567","operator":"mci"}`, `{"operator":"rightel",...}` and a request with no operator give HTTP 501; `"operator":"unknown"` and `"phone":"09350000351"` give HTTP 400. After the run the App copy was stopped and the test rows were deleted (6 `bill_inquiries`, 2 `bill_inquiry_requests`).

Not run: a real call to the `irancell` service (the real adapter is tested against a local gRPC server in `TestProviderRegistryIrancellRealCall`).
