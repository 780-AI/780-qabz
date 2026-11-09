<!-- VERSION: 0.1 -->
# 023_gas_inquiry_endpoint — POST /v1/gas/inquiry and the Qabzino gas adapter

Status: pending

## Description

Implement `Gas.Inquiry` (`POST /v1/gas/inquiry`, `pb/proto/qabz/services.proto`). The request is `InquiryBillPayRequest` (`billId`, `payId`, `participateCode`) and the response is `InquiryBillPayResponse` (`price`, `paymentDeadline`, `payId`, `billId`), both in `pb/proto/bill/services.proto`. This task follows tasks 021 (water) and 022 (electricity) and reuses their flow, tables and mock mechanism. It adds the `gas` bill type to the inquiry flow.

**The flow is the same as water (task 021) and electricity (task 022):** pick the provider for (`gas`, `inquiry`) with `ProvidersService.PickProvider` (conventions § 13); look in `bill_inquiries` first (same `inquiry_id`, same `provider_id`, newer than the cache TTL, `is_cached = false`, `is_paid = false`); only on a miss call the provider, write the `bill_inquiry_requests` row, and save the inquiries so the next request within the TTL is answered from the database. Use `core/inquiry` (`Service`), `provider.Registry` and the error mapping of task 021. Do not copy the water flow: extend it for one more bill type. A picked provider with no adapter answers `Unavailable`, as in task 021.

**What is special to this bill type:**
- **`participateCode` is part of the inquiry ID** (`bill`: `controller.SetInquiryID(billId, participateCode)`). The cache lookup uses that inquiry ID, so two participate codes of one bill ID are two cache keys.
- **Raw inquiry** (`bill`: `rawInquiry`): when the request has both `billId` and `payId`, `bill` answers from them with no provider (`raw.Service`). Decide in the design step whether this task includes it; if not, the difference is a `this_vs_bill.md` item.
- **Providers.** In the seed Qabzino gas (id 35) is disabled, Parsian (id 43) is enabled and gas-company (id 99) is disabled. This task adds only the Qabzino gas adapter (the Qabzino adapter already exists). The Parsian and gas-company adapters are NOT in this task: a picked provider with no adapter answers `Unavailable` (task 021 decision 2).

Help:
- `bill`: `controller/gas/inquiry.go` (handler, `validateInquiryRequest`, `rawInquiry`), `gateway/inquiry/gas.go` (`QabzinoGas`, `ParsianGas`, `GasCompanyGas`), `client/grpc/qabzino/`.
- The seed: `src/system/postgres/devdb/seed_providers.sql` (ids 35, 43, 99).
- Tasks 021 (`docs/Plans/done_tasks/021_water_inquiry_endpoint.md`) and 022 (`docs/Plans/pending_tasks/022_electricity_inquiry_endpoint.md`) are the pattern: their design decisions, `docs/this_vs_bill.md` items 14 to 22 (the same differences apply: error mapping, one request row), the Postman collection and the metrics.
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).

Points for the design step:
- The gas cache TTL key, default (see `bill`'s `caching` section) and whether it is hot-reloadable (task 021 decision 1).
- How the flow carries what is special to this bill type (listed above) with no copy of the water code.
- The fixed mock answer (values) and the error of `return_type: error`.
- The answer when no provider is enabled for (`gas`, `inquiry`).
- Metrics and log lines (conventions §§ 9, 12): reuse the task 021 metrics with `bill_type="gas"`.

## Definition of Done

- `POST /v1/gas/inquiry` works as described: database first, provider only on a miss, the answer saved (a different `participateCode` is a different cache key).
- The Qabzino adapter of task 021 (`provider/qabzino/`) serves `gas` too (`BillType = gas`, `haf80.api.qabzino.Inquiry/BillInquiry`); no new config key for the provider; `debug.mocks.qabzino` gives the fixed gas answer (a new fixture in `qabzino.MockFixtures`) and works only in dev mode. `config.yml` and `config.yml.example` updated (conventions § 8).
- Unit tests with mocks and stubs (no database, no Qabzino): a valid row in the database (no provider call), no row (one provider call, rows saved), an expired row, a row of another provider, a paid row, a provider error, no enabled provider, an invalid request; the Qabzino adapter with a fake gRPC client (request fields, answer mapping, empty pay ID, errors); the mock (`success`, `error`, off, rejected outside dev mode); the config keys.
- Integration tests (real Postgres, throw-away schema): first request saves the rows, the second request within the TTL is answered from the database with no provider call.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with `debug.mocks.qabzino.enable: true`. Make Qabzino the enabled provider of (`gas`, `inquiry`) in the dev database first (task 021 `Verification` run: `UPDATE providers ...`).

1. `return_type: success`. Call `POST /v1/gas/inquiry` with a new `billId` (and a `participateCode`). Pass: a success answer with the mock values; one `bill_inquiry_requests` row and the `bill_inquiries` rows are in the database; a log line shows the provider (mock) call.
2. Call again with the same `billId` within the TTL. Pass: the same answer, no provider call (log line or metric shows the database source), no new provider request row.
3. `return_type: error`. Call with another new `billId`. Pass: the error answer; the request row has the failed status; no `bill_inquiries` row is saved.
4. With `return_type: error`, call with the `billId` of step 1 within the TTL. Pass: the success answer from the database (the provider is not called, so the mock error does not show).
