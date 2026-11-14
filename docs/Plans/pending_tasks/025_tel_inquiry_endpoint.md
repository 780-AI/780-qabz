<!-- VERSION: 0.3 -->
<!--
CHANGE LOG
v0.3 — 2026-11-14 — Operator decision: the tel inquiry uses Karashop only (and nothing else). The task now adds the Karashop adapter (direct HTTPS, config `providers.karashop.*`, mock); the Qabzino and Parsian tel adapters are out. The Karashop config values given by the operator are recorded by reference (no secret in this file).
v0.2 — 2026-11-11 — Names tasks 035 to 037 in place of task 024.
v0.1 — 2026-11-02 — Created.
-->
# 025_tel_inquiry_endpoint — POST /v1/tel/inquiry and the Karashop telephone adapter

Status: pending

## Description

Implement `Tel.Inquiry` (`POST /v1/tel/inquiry`, `pb/proto/qabz/services.proto`). The request is `InquiryTelRequest` (`phone`, `billId`, `payId`) and the response is `InquiryPhoneResponse` (`midTerm` and `fullTerm`, each with `billId`, `payId`, `price`, `paymentDeadline`, `providerId`), both in `pb/proto/bill/services.proto`. This task follows the water, electricity, gas and mobile inquiry tasks (021, 022, 023, 035 to 037) and reuses their flow, tables and mock mechanism. It adds the `tel` bill type to the inquiry flow.

**Operator decision (2026-11-14): the tel inquiry uses Karashop only, and nothing else.** No other provider answers a tel inquiry: not Qabzino, not Parsian. Use the `onlyProvider` rule of the flow (`core/inquiry` `billTypes`, as Irancell in task 035 and Rightel in task 037): when the pick for (`tel`, `inquiry`) is not Karashop, the answer is the one of "no enabled provider" and nobody is called. The Qabzino tel adapter and the Parsian tel adapter are NOT in this task and are not planned.

**The flow is the same as for the other bill types:** pick the provider for (`tel`, `inquiry`) with `ProvidersService.PickProvider` (conventions § 13); look in `bill_inquiries` first (same `inquiry_id`, same `provider_id`, newer than the cache TTL, `is_cached = false`, `is_paid = false`); only on a miss call the provider, write the `bill_inquiry_requests` row, and save the inquiries so the next request within the TTL is answered from the database. Use `core/inquiry` (`Service`), `provider.Registry` and the error mapping of task 021. Do not copy a flow: add one entry to `billTypes`. One request calls one provider (conventions § 13).

**What is special to this bill type:**
- **Two inquiries per request:** a mid-term and a full-term inquiry, saved as separate `bill_inquiries` rows and returned in `midTerm` and `fullTerm`, as in the mobile inquiry (tasks 035 to 037). Reuse the answer mapping of `Phone.Inquiry` (`src/entry/grpc/phone.go`) with no copy of code. The cache skip of `bill` (`shouldSkipCache`: two saved rows of the same inquiry type are a miss) applies to `tel`: set `skipSameTypePair`.
- **Karashop makes two calls.** `bill` sends two HTTP calls to Karashop for one tel request: `tci-end-getBillAccInfo` (bill type `tel`, full term) and `tci-mid-getBillAccInfo` (bill type `telmidterm`, mid term), and writes two request rows (`service/billInquiry/inquiry.go`, the step loop with `inquiry_step` and `GetPairType`). Decide in the design step how the Qabz flow (one call per request, one request row) carries this. Proposed: the adapter makes both calls inside one `Inquiry` call and returns the two inquiries; the flow writes one request row. The difference from `bill` becomes a numbered item in `docs/this_vs_bill.md`. Also decide: the provider pick and the `provider_id` of the saved rows (seed `tel` row 124325, `telmidterm` row 123456), and the answer when one of the two calls fails.
- **The inquiry ID is the phone number** (`bill`: `controller/telephone/inquiry.go`, `helper.NormalizePhoneNumber`, `controller.SetInquiryID(billId, phone)`). Karashop gets the number without `98` or `0` (`bill`: `thirdparty/karashop/dto.go` `prepareTelephoneNumber`). Check `helpers.NormalizePhone` and `helpers.ValidTel` (`src/common/helpers/phone.go`) before you write a new rule (conventions § 5).
- **A request with both `billId` and `payId` is a raw inquiry** (`bill`: `Telephone.rawInquiry`): reuse `inquiry.Service.RawInquiry` (task 023), as `Gas.rawInquiry` does. The answer has only `fullTerm` set, as in `bill`.
- **Details** (`bill`: `KarashopTelephone.setDetails`, the service status) are NOT in this task. Record the difference in `docs/this_vs_bill.md`.
- **Payment info** (`bill`: `SendPaymentInfo`, `billPaymentInfoURL`) is NOT in this task. It belongs to the tel order flow.

**The Karashop adapter** (`src/provider/karashop/`, the folder exists and is empty):
- A direct HTTPS client (`bill`: `thirdparty/karashop/`): `POST` to the inquiry URL, a JSON body with `credential` (`code`, `password`), `parameters` (`telNumber`) and `service`; the answer is a JSON object whose `responseText` is a JSON string with the inner answer (`result.data`: `telStatus`, `serviceStatus`, `billId`, `paymentId`, `amount`; `result.status.code`; `status.code`; `meta.transactionId`).
- The answer mapping of `bill` (`gateway/inquiry/karashopTranslator.go`: `telStatusMap`, `resultStatusCodeMap`, `statusCodeMap`; `gateway/inquiry/telephone.go` `KarashopTelephone.Execute`). Map each `bill` status to the Qabz answer types (success, refused with message, provider down) and list each mapping in the design.
- JSON reading follows conventions § 1 (the answer is read on a hot path: `fastjson`; `encoding/json` v1 is forbidden).
- A `providers.karashop.enabled` switch (false: every call fails as "provider down", nothing is sent to Karashop, the mock does not apply), as the other adapters.
- A mock (conventions § 15): `debug.mocks.karashop.enable` and `return_type`; fixed answers in `karashop.MockFixtures()` with a mid-term and a full-term inquiry; dev mode only.
- Metrics and log lines (conventions §§ 9, 10, 12): reuse the inquiry metrics with `bill_type="tel"` and the provider call metrics with `provider="karashop"`. The password never prints (`Secret` type).

**Karashop config (operator, 2026-11-14).** The operator gave the values of the Karashop block. They are the same as the `karashop` block of `bill`'s `config.example.yaml` (lines 199 to 204): `code`, `password`, `timeout` (30s), `billInquiryURL` and `billPaymentInfoURL` (`https://core.digitalport.ir`). In the implementation:
- Add the keys under `providers.karashop` in the repo style (snake case): `enabled`, `code`, `password` (a `Secret`), `timeout`, `bill_inquiry_url`, `bill_payment_info_url` (not read yet: order flow), and `caching.tel` (`bill`: `caching.tel`, `1h`).
- Copy the real values into `config.yml` (git-ignored) with `enabled: true`.
- Put a dummied copy in `config.yml.example` (conventions § 8): empty `code`, `password` and URLs, `enabled: false`, the default timeout. Never put the real password or a real URL in a tracked file.

Help:
- `bill`: `controller/telephone/inquiry.go`, `gateway/inquiry/telephone.go` (`KarashopTelephone`), `gateway/inquiry/karashopTranslator.go`, `thirdparty/karashop/` (`karashop.go`, `billInquiry.go`, `dto.go`), `app/inquiryGateway.go` (`initKarashopProvider`), `config/thirdParty.go`, `service/billInquiry/inquiry.go` (step loop, cache skip).
- The seed: `src/system/postgres/devdb/seed_providers.sql` (ids 123456 `telmidterm` and 124325 `tel`, both `karashop`, `enable`).
- Tasks 021 (`docs/Plans/done_tasks/021_water_inquiry_endpoint.md`), 023 (raw inquiry), 035 and 037 (two inquiries per answer, `onlyProvider`) are the pattern: their design decisions, `docs/this_vs_bill.md` items 14 to 22 (the same differences apply: error mapping, one request row), the Postman collection and the metrics.
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).

Points for the design step:
- How the two Karashop calls map to one flow call (see above), and which seed row gives the `provider_id` of the saved rows.
- The `caching.tel` default (`1h`, as `bill`) and whether it is hot-reloadable (task 021 decision 1).
- The fixed mock answer (values) and the error of `return_type: error`.
- The answer when Karashop is not the enabled provider of (`tel`, `inquiry`).
- The request check of `InquiryTelRequest` (`bill`: `validateInquiryTelRequest`).

## Definition of Done

- `POST /v1/tel/inquiry` works as described: database first, Karashop only on a miss, the answer saved for both the mid-term and the full-term inquiry; any provider other than Karashop is never called.
- The Karashop adapter in `provider/karashop/` with the `providers.karashop.*` keys, its `enabled` switch and its mock (`debug.mocks.karashop`, dev mode only). `config.yml` has the real values; `config.yml.example` has the dummied copy (conventions § 8).
- Unit tests with mocks and stubs (no database, no Karashop): a valid row in the database (no provider call), no row (one provider call, rows saved), an expired row, a row of another provider, a paid row, two saved rows of the same inquiry type (a miss), a provider error, no enabled provider, a provider other than Karashop picked, an invalid request, a raw inquiry; the Karashop adapter with a fake HTTP server (request body, both service names, the phone form, the answer mapping of each status map, a broken answer, a timeout, `enabled: false`); the mock (`success`, `error`, off, rejected outside dev mode); the config keys.
- Integration tests (real Postgres, throw-away run database, conventions § 14): first request saves the rows, the second request within the TTL is answered from the database with no provider call.
- Build, vet and tests (with `-race`) pass; coverage gate (conventions § 11). `system.html`, `this_vs_bill.md`, `folder_structure.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit. The `code-reviewer` agent runs before the commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with `debug.mocks.karashop.enable: true`. NO call to the real Karashop server in steps 1 to 5. Karashop is the enabled provider of (`tel`, `inquiry`) in the seed; check it first.

1. `return_type: success`. Call `POST /v1/tel/inquiry` with a new `phone`. Pass: a success answer with the mock values for both `midTerm` and `fullTerm`; one `bill_inquiry_requests` row and two `bill_inquiries` rows are in the database; a log line shows the provider (mock) call.
2. Call again with the same `phone` within the TTL. Pass: the same answer, no provider call (the counter `qabz_inquiries_total{bill_type="tel",result="db_hit"}`), no new request row.
3. `return_type: error`. Call with another new `phone`. Pass: the error answer; the request row has the failed status; no `bill_inquiries` row is saved.
4. With `return_type: error`, call with the `phone` of step 1 within the TTL. Pass: the success answer from the database (the provider is not called, so the mock error does not show).
5. Disable the Karashop `tel` row (wait for the provider reload) and call with a new `phone`. Pass: the "no enabled provider" answer, no request row, no provider call. Enable the row again.
6. Operator only: the first real call, with `debug.mocks.karashop.enable: false` and a real phone. Pass: the real mid-term and full-term debt, or the refused text of Karashop.
