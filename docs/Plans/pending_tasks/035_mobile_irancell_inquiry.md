<!-- VERSION: 0.1 -->
# 035_mobile_irancell_inquiry — POST /v1/mobile/inquiry for the Irancell operator

Status: pending

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
