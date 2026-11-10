<!-- VERSION: 0.2 -->
# 022_electricity_inquiry_endpoint — POST /v1/electricity/inquiry, the Tavanir provider adapter and its mock

Status: done

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

## Design decisions (implementer, 2026-11-10)

The "Points for the design step" are decided as follows. The `bill` differences that a client or the database can see are items 23 to 25 of `docs/this_vs_bill.md`, each with a `@todo` at the code.

1. **Cache TTL.** Key `caching.electricity` (`Config.CachingElectricity`), default `1h` (as in `bill`). `0` turns the cache off; a negative value is rejected at start. Not hot-reloadable, for the reason of task 021 decision 1. The paid-bill TTL (`caching.paidElectricity`) belongs to the order flow and is not added.
2. **Details: saved in `extra_data`, all of them.** `bill` saves only `city` and `region` and answers a cache hit with no details (`getFromCache` returns `Details: nil`). The Definition of Done asks for the same details on a cache hit, so Qabz saves the fields that the client sees as more keys of the same JSON object (`phase`, `company_name`, `tariff_type`, `customer_type`, `serial_number`, `service_address`, `service_post_code`, `customer_name`), next to bill's `city` and `region`. No schema change. Effect: `bill_inquiries.extra_data` now holds the customer name, the address and the post code (item 23). A row that bill wrote answers with empty detail fields and `customerType` `"0"`. An `extra_data` that cannot be read is logged and the provider is asked.
3. **`ProviderInterface`: no new method.** `InquiryResponse` gets the field `Details *provider.Details` (`Details.Electricity`); an adapter with no details leaves it nil (`system.html`, Provider Adapter Interface, updated). `PaymentRequest`, `DocumentRequest` and the others are unchanged.
4. **One flow for every bill type.** `core/inquiry.Service` is unchanged in structure: a bill type is supported when it has a `tid` prefix (`electricity`: 2, as bill's `ElectricityReference`); `Result` and `Row` carry `*provider.Details`; `Options.CacheTTL` already takes the bill type. The composition root builds the flow once (`newInquiryFlow`, which registers the three metrics once) and gives it to `newWater` and `newElectricity`. The error mapping of the handlers is one function (`answerInquiryError`; the log line starts with the bill type), so the water log lines are unchanged. The JSON of `extra_data` is written and read in `system/postgres/inquiry_details.go` (read with fastjson: a hot path, conventions § 1).
5. **Tavanir adapter.** As `TavanirElectricity.Execute` of `bill`: the request has no trace number; the amount, the pay ID and the deadline of each inquiry are passed on unchanged (no amount-from-pay-ID rule, an empty pay ID keeps its amount); the inquiry type is `regular`; no trace ID; an answer with no inquiry has no bill ID and no details, and a `success` one is one inquiry with amount 0; the saved `city` is the company name (bill maps it so). An empty status is stored as `failed` (item 24, as Qabzino).
6. **Mock.** `debug.mocks.tavanir.enable` (default `false`) and `.return_type` (`success` or `error`), with the rules of conventions § 15. `return_type: success`: bill ID of the request, pay ID `1800012345`, 18 000 000 rials, deadline `2030-01-01`, no trace ID, phase `تک فاز`, company `توزیع برق تهران بزرگ`, tariff `خانگی`, customer type `1`, serial number `12345678`, address `تهران، خیابان آزادی، پلاک ۱`, post code `1234567890`, customer name `مشترک آزمایشی`, region `تهران` (the `Mock...` constants of `provider/tavanir`). `return_type: error` is `provider.ErrProviderDown`.
7. **No enabled provider, errors, metrics, logs.** The same as water (task 021 decisions 4, 6 and 7): `Unavailable` (HTTP 503) with the provider-down text and one error line; `FailedPrecondition` (HTTP 400) for a provider with no inquiry; the three metrics with `bill_type="electricity"` and `provider="tavanir"` (no new metric); the log lines `electricity: no enabled provider for inquiry`, `electricity: provider inquiry failed`, `electricity: inquiry failed`, `electricity: inquiry amount does not fit the price field`.
8. **Other.** (a) `customerType` is the number of the provider as text (bill: `fmt.Sprint`); an answer with no details has an empty `customerType`. (b) `make proto` also generates the Tavanir client (`pb/proto/tavanir/internalService.proto`, read-only) into `src/api/pb/tavanir/` (git-ignored). (c) The metric registration error prefix changed from `water: register metric` to `inquiry: register metric` (the flow is shared). (d) `Electricity.InquiryDocument` stays `Unimplemented` (HTTP 501), as the task says.

## Verification record (implementer, 2026-11-10)

Feature ON (Rule 15), the compose dev database, `mode: dev`, `debug.auth.disable: true` with a mock user, `debug.mocks.tavanir.enable: true`, ports 18080 / 18082 / 19090. The seed has Tavanir (id 2) enabled for (`electricity`, `inquiry`).

1. `return_type: success`, `POST /v1/electricity/inquiry` `{"billId":"4410000000009"}`: HTTP 200 with the mock values and all eight details. Database: one `bill_inquiry_requests` row (`success`, no trace, `tavanir`, `tid` starts with 2); one `bill_inquiries` row (`is_cached` false) whose `extra_data` has the ten keys. Log: `provider mock: inquiry answered with no call to the provider`, `inquiry: provider call`. PASS.
2. The same call again: the same body, byte for byte. Log: `inquiry: answered from database, no provider call`, no second mock line. Still one request row; one copy row (`is_cached` true) with the same `extra_data`. Metrics: `provider_ok` 1, `db_hit` 1, `qabz_provider_calls_total{result="ok"}` 1. PASS.
3. Restart with `return_type: error`, `{"billId":"4410000009987"}`: HTTP 503 with the provider-down text; the request row has `failed`; no `bill_inquiries` row for the bill; one error line `electricity: provider inquiry failed`; `provider_down` 1. PASS.
4. With `return_type: error`, `{"billId":"4410000000009"}`: HTTP 200 with the answer and the details of step 1, from the database. PASS.

Also: `{"billId":"12345"}` gives HTTP 400 `invalid parameter`; `POST /v1/electricity/inquiry/document` gives HTTP 501. The test rows were deleted after the run. Not tested: a call to a real Tavanir backend (none in development).
