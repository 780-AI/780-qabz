<!-- VERSION: 0.1 -->
# 021_water_inquiry_endpoint — POST /v1/water/inquiry, the first provider (Qabzino) and the provider mocks

Status: pending

## Description

Implement `Water.Inquiry` (`POST /v1/water/inquiry`, `pb/proto/qabz/services.proto`). The request is `InquiryBillPayRequest` (`billId`, `payId`, `participateCode`) and the response is `InquiryBillPayResponse` (`price`, `paymentDeadline`, `payId`, `billId`), both in `pb/proto/bill/services.proto`. This is the first inquiry endpoint of Qabz.

**The flow (as in `bill`):**
1. Pick the provider for (`water`, `inquiry`) with `ProvidersService.PickProvider` (task 020, conventions § 13). Never read the `providers` table directly.
2. Look in the database first. Find valid saved inquiries of the same bill in `bill_inquiries`: the same `inquiry_id` (the `billId` that the client sends), the same `provider_id`, `created_at` newer than the cache TTL, `is_cached = false`, `is_paid = false`. If rows are found, answer from them and do NOT call the provider.
3. Only if no valid row is found, send the request to the provider (Qabzino). Write the `bill_inquiry_requests` row (status `INITIAL`, then the provider status and trace ID), and save each inquiry of the answer as a `bill_inquiries` row, so the next request within the TTL is answered from the database.

**Qabzino is the first provider that implements `ProviderInterface`** (`system.html`, Provider Adapter Interface; `provider/provider.go`). The adapter is in `provider/qabzino/`. It calls the Qabzino backend over gRPC (`haf80.api.qabzino.Inquiry/BillInquiry`, `pb/proto/qabzino/internalService.proto`) with `BillType = water`. In this task only `Inquiry` does real work; the other methods of the interface return a testable "not supported" error.

**Config.** Qabzino is the first subsection of the `providers` section (next to `providers.general`), for example `providers.qabzino.grpc_address` and `providers.qabzino.timeout`. `system.html` names the section `provider:`; the code uses `providers:` and `system.html` is updated to match (critical file, same commit).

**Provider mocks (development only).** A new subsection `debug.mocks`, one block per provider:

```yml
debug:
  mocks:
    qabzino:
      enable: true
      return_type: "success"   # "success" or "error"
```

- With `enable: true`, the Qabzino adapter does not call the backend. It returns a fixed answer at real app runtime: a success answer (a valid water inquiry with pay ID, amount and payment deadline) when `return_type` is `success`, and a provider error when it is `error`. Everything before the provider call (provider pick, cache lookup, request rows, saving the answer) runs as in production.
- Mocks work only in dev mode (`mode: dev`). The design step decides how the config check treats `enable: true` in another mode (reject at start is the safe default).
- The same pattern repeats for every later provider (`debug.mocks.<provider>`), so the mechanism is generic, not Qabzino-specific code spread in the flow.

Help:
- `bill` (`~/projects/bill`): `controller/water/inquiry.go` (the handler, `InquiryID = billId`), `service/billInquiry/inquiry.go` (`Inquiry`: provider, cache, `inquiry_step` loop, request rows, `handleResponseWithInquiry`, `getFromCache`), `domain/repository/billInquiries/bill_inquiry_dto.go` (`GetValidInquiryByInquiryID`), `gateway/inquiry/water.go` (`QabzinoWater.Execute`: amount from the pay ID, a paid bill has an empty pay ID), `client/grpc/qabzino/`, config `caching.water: 1h` and `caching.paidWater: 72h`, `qabzino.grpcAddress`.
- In `bill` a cache hit also writes copies of the rows for the current user with `is_cached = true`. The cache is shared between users (the lookup has no user filter).
- The tables: `bill_inquiries` and `bill_inquiry_requests` in `src/system/postgres/devdb/schema.sql` and `docs/initial-discovery/database.html`. Raw queries, no ORM (tasks 013, 016).
- `bill` is the reference. Each difference that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).

Points for the design step:
- The config key and default of the water cache TTL (`bill`: `caching.water`, `1h`), and whether it is hot-reloadable.
- `ProviderRegistry` (`provider/registry.go`) in this task or later; how the handler gets the adapter of the picked provider name.
- `inquiry_step` (the `bill` loop for paired types): needed for water or not.
- The fixed mock answers (values), and the error that `return_type: error` gives (for example the "provider inquiry is down" user error of `bill`).
- Mock config: `return_type` values checked at start; mock settings hot-reloadable or not; behavior outside dev mode.
- The answer when no provider is enabled for (`water`, `inquiry`) (`ErrNoProvider`).
- Metrics and log lines (conventions §§ 9, 12): for example inquiries by source (`db` / `provider`) and result, and the provider call duration.

## Definition of Done

- `POST /v1/water/inquiry` works as described: database first, provider only on a miss, the answer saved.
- `ProviderInterface` and the Qabzino adapter exist; Qabzino config in `providers.qabzino`; `debug.mocks.qabzino` works only in dev mode. `config.yml` and `config.yml.example` updated (conventions § 8).
- Unit tests with mocks and stubs (no database, no Qabzino): a valid row in the database (no provider call), no row (one provider call, rows saved), an expired row, a row of another provider, a paid row, a provider error, no enabled provider, an invalid request; the Qabzino adapter with a fake gRPC client (request fields, answer mapping, empty pay ID, errors); the mock (`success`, `error`, off, rejected outside dev mode); the config keys.
- Integration tests (real Postgres, throw-away schema): first request saves the rows, the second request within the TTL is answered from the database with no provider call.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with `debug.mocks.qabzino.enable: true`.

1. `return_type: success`. Call `POST /v1/water/inquiry` with a new `billId`. Pass: a success answer with the mock values; one `bill_inquiry_requests` row and the `bill_inquiries` rows are in the database; a log line shows the provider (mock) call.
2. Call again with the same `billId` within the TTL. Pass: the same answer, no provider call (log line or metric shows the database source), no new provider request row.
3. `return_type: error`. Call with another new `billId`. Pass: the error answer; the request row has the failed status; no `bill_inquiries` row is saved.
4. With `return_type: error`, call with the `billId` of step 1 within the TTL. Pass: the success answer from the database (the provider is not called, so the mock error does not show).
