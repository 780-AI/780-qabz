<!-- VERSION: 0.1 -->
# 025_tel_inquiry_endpoint — POST /v1/tel/inquiry and the Qabzino telephone adapter

Status: pending

## Description

Implement `Tel.Inquiry` (`POST /v1/tel/inquiry`, `pb/proto/qabz/services.proto`). The request is `InquiryTelRequest` (`phone`, `billId`, `payId`) and the response is `InquiryPhoneResponse` (`midTerm` and `fullTerm`, each with `billId`, `payId`, `price`, `paymentDeadline`, `providerId`), both in `pb/proto/bill/services.proto`. This task follows tasks 021 (water) and 022 (electricity) and reuses their flow, tables and mock mechanism. It adds the `tel` bill type to the inquiry flow.

**The flow is the same as water (task 021) and electricity (task 022):** pick the provider for (`tel`, `inquiry`) with `ProvidersService.PickProvider` (conventions § 13); look in `bill_inquiries` first (same `inquiry_id`, same `provider_id`, newer than the cache TTL, `is_cached = false`, `is_paid = false`); only on a miss call the provider, write the `bill_inquiry_requests` row, and save the inquiries so the next request within the TTL is answered from the database. Use `core/inquiry` (`Service`), `provider.Registry` and the error mapping of task 021. Do not copy the water flow: extend it for one more bill type. A picked provider with no adapter answers `Unavailable`, as in task 021.

**What is special to this bill type:**
- **Two inquiries per request:** a mid-term (`telmidterm`) and a full-term (`tel`) inquiry, saved as separate rows and returned in `midTerm` and `fullTerm`, as in task 024. Decide in the design step how the flow handles the two provider types; share the mechanism with task 024 if that task is done first, with no copy of code.
- **The inquiry ID is the phone number** (`bill`: `controller/telephone/inquiry.go`, validation, area code).
- **Details** (`bill`: `KarashopTelephone.setDetails`) belong to the Karashop provider and are not in this task.
- **Providers.** This task adds only the Qabzino telephone adapter (seed ids 52 and 53, `qabzino-tel-inquiry`, `qabzino-telmidterm-inquiry`). The Parsian and Karashop adapters are NOT in this task: a picked provider with no adapter answers `Unavailable` (task 021 decision 2).

Help:
- `bill`: `controller/telephone/inquiry.go`, `gateway/inquiry/telephone.go` (`ParsianTelephone`, `QabzinoTelephone`, `KarashopTelephone`), `client/grpc/qabzino/`.
- The seed: `src/system/postgres/devdb/seed_providers.sql` (ids 40, 42, 52, 53, 123456, 124325).
- Tasks 021 (`docs/Plans/done_tasks/021_water_inquiry_endpoint.md`) and 022 (`docs/Plans/pending_tasks/022_electricity_inquiry_endpoint.md`) are the pattern: their design decisions, `docs/this_vs_bill.md` items 14 to 22 (the same differences apply: error mapping, one request row), the Postman collection and the metrics.
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).

Points for the design step:
- The telephone cache TTL key, default (see `bill`'s `caching` section) and whether it is hot-reloadable (task 021 decision 1).
- How the flow carries what is special to this bill type (listed above) with no copy of the water code.
- The fixed mock answer (values) and the error of `return_type: error`.
- The answer when no provider is enabled for (`tel`, `inquiry`).
- Metrics and log lines (conventions §§ 9, 12): reuse the task 021 metrics with `bill_type="tel"`.

## Definition of Done

- `POST /v1/tel/inquiry` works as described: database first, provider only on a miss, the answer saved for both the mid-term and the full-term inquiry.
- The Qabzino adapter of task 021 (`provider/qabzino/`) serves `tel` too (`BillType = tel`, `haf80.api.qabzino.Inquiry/BillInquiry`); no new config key for the provider; `debug.mocks.qabzino` gives the fixed tel answer (a new fixture in `qabzino.MockFixtures`) and works only in dev mode. `config.yml` and `config.yml.example` updated (conventions § 8).
- Unit tests with mocks and stubs (no database, no Qabzino): a valid row in the database (no provider call), no row (one provider call, rows saved), an expired row, a row of another provider, a paid row, a provider error, no enabled provider, an invalid request; the Qabzino adapter with a fake gRPC client (request fields, answer mapping, empty pay ID, errors); the mock (`success`, `error`, off, rejected outside dev mode); the config keys.
- Integration tests (real Postgres, throw-away schema): first request saves the rows, the second request within the TTL is answered from the database with no provider call.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with `debug.mocks.qabzino.enable: true`. Make Qabzino the enabled provider of (`tel`, `inquiry`) in the dev database first (task 021 `Verification` run: `UPDATE providers ...`).

1. `return_type: success`. Call `POST /v1/tel/inquiry` with a new `billId` (a `phone`). Pass: a success answer with the mock values for both `midTerm` and `fullTerm`; one `bill_inquiry_requests` row and the `bill_inquiries` rows are in the database; a log line shows the provider (mock) call.
2. Call again with the same `billId` within the TTL. Pass: the same answer, no provider call (log line or metric shows the database source), no new provider request row.
3. `return_type: error`. Call with another new `billId`. Pass: the error answer; the request row has the failed status; no `bill_inquiries` row is saved.
4. With `return_type: error`, call with the `billId` of step 1 within the TTL. Pass: the success answer from the database (the provider is not called, so the mock error does not show).
