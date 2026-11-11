<!-- VERSION: 0.3 -->
# 036_mobile_mci_inquiry — POST /v1/mobile/inquiry for the MCI operator

Status: pending

## Description

Task 024 (`mobile_inquiry_endpoint`) was split by operator into tasks 035 (Irancell), 036 (MCI) and 037 (Rightel). This task makes `Phone.Inquiry` (`POST /v1/mobile/inquiry`, `pb/proto/qabz/services.proto`) answer for the **MCI** operator. The request is `InquiryPhoneRequest` (`phone`, `operator`, `payId`, `billId`, `service`) and the response is `InquiryPhoneResponse` (`midTerm` and `fullTerm`, each with `billId`, `payId`, `price`, `paymentDeadline`, `providerId`), both in `pb/proto/bill/services.proto`.

**Shared part of tasks 035, 036 and 037.** The three tasks have one endpoint. The first of them that starts builds the shared part; the other two add one operator to it and do not copy it.
- The handler and the request check of `bill` (`validateInquiryRequest`: `phone` or `billId`; `operator` or `service`; `phone` or `payId`; valid enum values; a valid phone number; a `billId` that is sent is a valid bill ID).
- The operator is `request.operator`, or `request.service` when the operator is empty (`setOperator`). One table gives the bill type of each operator (`bill`: `operatorBillType`): Irancell → `irancell`, MCI → `mobile`, Rightel → `mobilemidtermRightel`.
- The inquiry ID is the phone, or the bill ID when the phone is empty (`controller.SetInquiryID`).
- The flow is the shared inquiry flow of tasks 021 to 023 (`core/inquiry`: one more entry per bill type in `billTypes`; `ProvidersService.PickProvider`, conventions § 13; `bill_inquiries` first, the provider only on a miss, one `bill_inquiry_requests` row, the error mapping of task 021). Do not copy the flow.
- One provider call gives a mid-term and a full-term inquiry. They are saved as separate `bill_inquiries` rows and returned in `midTerm` and `fullTerm` (`bill`: `handleResponseWithInquiry`). An answer from saved inquiries returns both the same way.

**Provider constraint (operator, 2026-11-11): an MCI inquiry MUST use Jiring Switch. When Jiring Switch is not available, it MUST use Parsian.** Jiring Switch is always the first provider; Parsian is only the fallback. No other provider answers an MCI inquiry. When the two are not available, the answer is the provider-down answer (`Unavailable`).

**Meaning of "not available" (operator, 2026-11-11): Jiring Switch does not respond.** The cause can be the future circuit breaker of the provider (open: no call is sent) or another reason (a failed call, a timeout, a refused connection: the provider-down result of the adapter). So one MCI request can have two provider calls: Jiring Switch first, then Parsian in the same request when Jiring Switch does not respond. An answer of Jiring Switch that is a business answer (for example no debt, or a refused phone number) is a response: Parsian is not called.

**Rule of the fallback (operator, 2026-11-11): "when provider 1 does not answer, the next provider in the row takes the job".** The order is the order of the sorted provider list of (`mobile`, `inquiry`) (`ProvidersService`, conventions § 13): Jiring Switch is the first row and Parsian is the next row. A provider that cannot answer (its row is disabled, its circuit breaker is open, or its call does not respond) gives the job to the next provider in the list. So Parsian also answers when the Jiring Switch row is disabled.

**What is special to MCI:**
- **Bill type `mobile`, providers `jiring-switch` (first) and `parsian` (fallback).** In the seed, Jiring Switch (id 28) is disabled and Parsian (id 39) is enabled for (`mobile`, `inquiry`): the seed order is the opposite of the constraint, so the dev database needs a change (or the seed, by an operator decision) before Jiring Switch can answer. The rows of `mobilemidterm` (ids 27 and 41) are not used by this endpoint: the operator table never gives that bill type.
- **`bill` gateways** (`gateway/inquiry/mobile_mci.go`): `ParsianMCIMobile` calls the `parsian-v2` microservice (`BillInquiry` with a tracking ID, a reference ID and the bill type `mobile_mci`; the answer has the bill ID, a `tid`, and a pay ID, an amount and a payment deadline for each inquiry). `JiringSwitchMCIMobile` calls the `jiring-switch` microservice (no payment deadline, no `tid`).
- **Cache skip.** `bill` does not use the saved inquiries of `mobile` when the two rows have the same inquiry type (`shouldSkipCache` in `service/billInquiry/inquiry.go`).
- **The two adapters are new.** Qabz has no Parsian and no Jiring Switch adapter (`src/provider/parsianproxy/` and `src/provider/jiring/` are empty folders). The `qabzino` service also has an MCI inquiry at Ayantech (`gateway/mci_mobile.go`), but no seed row gives Qabzino for `mobile`.

Help:
- `bill`: `controller/phone/inquiry.go`, `controller/phone/phone.go`, `gateway/inquiry/mobile_mci.go`, `gateway/inquiry/base.go` (`handlePhoneResponseWithoutInquiry`), `pkg/reference` (tracking and reference IDs), `app/inquiryGateway.go`, `service/billInquiry/inquiry.go`.
- The `parsian-v2` and `jiring-switch` services.
- The seed: `src/system/postgres/devdb/seed_providers.sql` (ids 27, 28, 39, 41).
- Task 023 (`docs/Plans/done_tasks/023_gas_inquiry_endpoint.md`) is the newest pattern: its design decisions, the `billTypes` table, the mock, the Postman collection, the metrics. Tasks 030 and 034 are the pattern of a direct provider adapter.
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).

Points for the design step:
- The fallback in the flow: the flow of tasks 021 to 023 calls one provider for each request. Decide how it calls Parsian after Jiring Switch does not respond, with no copy of the flow: the `bill_inquiry_requests` rows (one for each provider call), the metrics of each call, the `provider_id` of the saved inquiries, the lookup of saved inquiries (rows of Jiring Switch and rows of Parsian), and the time limit of the full request.
- Which results of the Jiring Switch adapter are "does not respond" (the provider-down result) and which are a response.
- The circuit breaker does not exist yet (task 034 follow-up (a): every provider gets one at the end of the project). Make the fallback so that an open breaker gives the same provider-down result and needs no change of the flow.
- The next-provider step uses the sorted provider list (no fixed provider names in the flow). Decide how the flow reads "the next provider" from `ProvidersService` (today `PickProvider` gives one provider), and how the list has Jiring Switch before Parsian for (`mobile`, `inquiry`) (the seed has the opposite state).
- The scope of the rule: this task applies it to MCI only. Irancell (task 035) and Rightel (task 037) have one provider and no fallback. Ask the operator before the rule is applied to water, electricity, gas or tel; if it becomes a lasting rule of the flow, it goes into the conventions doc (§ 13) in the same commit.
- How the order "Jiring Switch first, Parsian second" is kept: by the sorted provider list (`ProvidersService`, conventions § 13, the priority of the rows) or by a fixed rule for this operator. `MoveUp` / `MoveDown` must not be able to break the constraint without a trace.
- Each adapter: a direct client of the provider (as tasks 030 and 034 did for Qabzino and Tavanir) or a client of the microservice. Its config keys under `providers.<name>` (conventions § 8).
- The cache TTL key of the bill type `mobile` and its default (`bill`: `caching.mobile`), and the cache skip rule above.
- The answer for an operator whose task is not done (when this task is the first of the three).
- The `midTerm` and `fullTerm` of the answer when the provider gives one inquiry, or no debt.
- The fixed mock answer and the error of `return_type: error`.
- Metrics and log lines (conventions §§ 9, 12): the task 021 metrics with the bill type of this operator.

## Definition of Done

- `POST /v1/mobile/inquiry` with the MCI operator works as described: database first, provider only on a miss, the mid-term and the full-term inquiry saved and returned.
- The shared part exists once (built here if this is the first of tasks 035 to 037, else reused with no copy).
- The Jiring Switch adapter and the Parsian adapter are registered for (`mobile`, `inquiry`). Jiring Switch answers when it is available; Parsian answers only when Jiring Switch is not available; no other provider is called. Each adapter has a mock that works only in dev mode. `config.yml` and `config.yml.example` are updated (conventions § 8).
- Unit tests with mocks and stubs (no database, no provider): a valid row in the database (no provider call), no row (one provider call, rows saved), an expired row, a row of another provider, a paid row, two saved rows of the same inquiry type, a provider error, no enabled provider, an invalid request, each operator value; the provider order (Jiring Switch responds: only Jiring Switch is called, also for a business answer; Jiring Switch does not respond (provider down, timeout): Parsian is called in the same request; the Jiring Switch row is disabled: Parsian is called; the two do not respond: `Unavailable`); each adapter (request fields, answer mapping, errors); the mock (`success`, `error`, off, rejected outside dev mode); the config keys.
- Integration tests (real Postgres, throw-away schema): the first request saves the rows, the second request within the TTL is answered from the database with no provider call.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with the mocks of the Jiring Switch and Parsian adapters enabled. Enable the Jiring Switch row (id 28) of (`mobile`, `inquiry`) in the dev database first and keep the Parsian row (id 39) enabled (task 021 `Verification` run: `UPDATE providers ...`).

1. `return_type: success`. Call `POST /v1/mobile/inquiry` with a new `phone` and the MCI `operator`. Pass: a success answer with the Jiring Switch mock values in `midTerm` and `fullTerm`; the rows have the Jiring Switch provider; Parsian is not called; one `bill_inquiry_requests` row and the `bill_inquiries` rows are in the database; a log line shows the provider (mock) call.
2. Call again with the same `phone` within the TTL. Pass: the same answer, no provider call (the counter `db_hit` shows the database source), no new provider request row.
3. With the two mocks on `return_type: error`, call with the `phone` of step 1 within the TTL. Pass: the success answer from the database.
4. Set the Jiring Switch mock to `return_type: error` (it does not respond) and the Parsian mock to `return_type: success`. Call with another new `phone`. Pass: a success answer with the Parsian mock values in the same request; the saved inquiries have the Parsian provider; the request rows and the metrics show the failed Jiring Switch call and the Parsian call.
5. Disable the Jiring Switch row (id 28) and wait for the provider reload, with the Parsian mock on `return_type: success`. Call with another new `phone`. Pass: a success answer with the Parsian mock values; no Jiring Switch call. Then enable the row again.
6. Set the two mocks to `return_type: error`. Call with another new `phone`. Pass: the provider-down answer (HTTP 503); the request rows have the failed status; no `bill_inquiries` row is saved; no other provider is called. Then restore the provider rows.
