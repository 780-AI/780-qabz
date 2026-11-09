<!-- VERSION: 0.1 -->
# 022_electricity_inquiry_endpoint — POST /v1/electricity/inquiry, the Tavanir provider adapter and its mock

Status: pending

## Description

Implement `Electricity.Inquiry` (`POST /v1/electricity/inquiry`, `pb/proto/qabz/services.proto`). The request is `InquiryBillPayRequest` (`billId`, `payId`, `participateCode`) and the response is `InquiryElectricityResponse` (`payId`, `billId`, `price`, `paymentDeadline` and the details `phase`, `companyName`, `tariffType`, `customerType`, `serialNumber`, `serviceAddress`, `servicePostCode`, `customerName`), both in `pb/proto/bill/services.proto`. This task follows task 021 (water) and reuses its flow, its tables and its mock mechanism. It adds the electricity bill type to the inquiry flow.

**The flow is the same as water (task 021):** pick the provider for (`electricity`, `inquiry`) with `ProvidersService.PickProvider` (conventions § 13); look in `bill_inquiries` first (same `inquiry_id`, same `provider_id`, newer than the cache TTL, `is_cached = false`, `is_paid = false`); only on a miss call the provider, write the `bill_inquiry_requests` row, and save the inquiries so the next request within the TTL is answered from the database. Use `core/inquiry` (`Service`) and `provider.Registry` from task 021. Do not copy the water flow: extend it for a second bill type.

**What is different from water:**
- **The provider is Tavanir, not Qabzino.** The adapter is in `provider/tavanir/` and implements `ProviderInterface`. It calls the Tavanir backend over gRPC (`InternalInquiryService/BillInquiry`, `pb/proto/tavanir/internalService.proto`). In this task only `Inquiry` does real work; the other methods return the testable "not supported" error. `BillDocument` (`POST /v1/electricity/inquiry/document`, `Electricity.InquiryDocument`) is NOT in this task.
- **The answer has details** (phase, company, tariff, customer type, serial number, address, post code, customer name). The details come from the provider answer; the provider `Details` of `ProviderInterface` and the way the details are saved in `bill_inquiries` (the `bill` `ElectricityDetails`: `city` and `region` are saved, the others are for display) and read back on a cache hit are part of the design step. A cache hit must give the same details as a provider call.
- **Config.** `providers.tavanir.grpc_address` and `providers.tavanir.timeout`, next to `providers.qabzino`. The electricity cache TTL key and default (`bill`: check its `caching` section) are decided in the design step.
- **Mock.** `debug.mocks.tavanir` (`enable`, `return_type`: `success` or `error`) uses the generic `provider.NewMock` of task 021 and works only in dev mode (task 020c). The fixed success answer is a valid electricity inquiry with pay ID, amount, payment deadline and all details.

Help:
- `bill` (`~/projects/bill`): `controller/electricity/inquiry.go` (handler, `validateInquiryRequest`, `setElectricityDetails`), `gateway/inquiry/electricity.go` (`TavanirElectricity.Execute`: the provider has no trace ID, details mapping), `gateway/inquiry/dto.go` (`ElectricityDetails`), `client/grpc/tavanir/`, config `tavanir.grpcAddress`.
- The seed has Tavanir enabled for (`electricity`, `inquiry`) (`src/system/postgres/devdb/seed_providers.sql`, id 2).
- Task 021 (`docs/Plans/done_tasks/021_water_inquiry_endpoint.md`) is the pattern: its design decisions, `docs/this_vs_bill.md` items 14 to 22 (the same differences apply: error mapping, one request row), the Postman collection and the metrics.
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).

Points for the design step:
- The electricity cache TTL key, default, and whether it is hot-reloadable (see task 021 decision 1).
- How the flow becomes generic for the bill type (water and electricity) with no copy of code, and how the details are carried by the flow, saved and read back.
- Whether `ProviderInterface` needs a change for the details (`system.html`, Provider Adapter Interface).
- The fixed mock answer (values) and the error of `return_type: error`.
- The answer when no provider is enabled for (`electricity`, `inquiry`).
- Metrics and log lines (conventions §§ 9, 12): reuse the task 021 metrics with `bill_type="electricity"`.

## Definition of Done

- `POST /v1/electricity/inquiry` works as described: database first, provider only on a miss, the answer and its details saved and returned the same on a cache hit.
- The Tavanir adapter exists; Tavanir config in `providers.tavanir`; `debug.mocks.tavanir` works only in dev mode. `config.yml` and `config.yml.example` updated (conventions § 8).
- Unit tests with mocks and stubs (no database, no Tavanir): a valid row in the database (no provider call), no row (one provider call, rows saved), an expired row, a row of another provider, a paid row, a provider error, no enabled provider, an invalid request; the Tavanir adapter with a fake gRPC client (request fields, answer and details mapping, empty pay ID, errors); the mock (`success`, `error`, off, rejected outside dev mode); the config keys.
- Integration tests (real Postgres, throw-away schema): first request saves the rows, the second request within the TTL is answered from the database with no provider call.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with `debug.mocks.tavanir.enable: true`.

1. `return_type: success`. Call `POST /v1/electricity/inquiry` with a new `billId`. Pass: a success answer with the mock values and details; one `bill_inquiry_requests` row and the `bill_inquiries` rows are in the database; a log line shows the provider (mock) call.
2. Call again with the same `billId` within the TTL. Pass: the same answer with the same details, no provider call (log line or metric shows the database source), no new provider request row.
3. `return_type: error`. Call with another new `billId`. Pass: the error answer; the request row has the failed status; no `bill_inquiries` row is saved.
4. With `return_type: error`, call with the `billId` of step 1 within the TTL. Pass: the success answer from the database (the provider is not called, so the mock error does not show).
