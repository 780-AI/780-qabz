<!-- VERSION: 0.4 -->
<!--
CHANGE LOG
v0.4 — 2026-11-14 — Done: design decisions, implementation record and verification record added; status done; moved to `done_tasks/`.
v0.3 — 2026-11-14 — Operator decision: the tel inquiry uses Karashop only (and nothing else). The task now adds the Karashop adapter (direct HTTPS, config `providers.karashop.*`, mock); the Qabzino and Parsian tel adapters are out. The Karashop config values given by the operator are recorded by reference (no secret in this file).
v0.2 — 2026-11-11 — Names tasks 035 to 037 in place of task 024.
v0.1 — 2026-11-02 — Created.
-->
# 025_tel_inquiry_endpoint — POST /v1/tel/inquiry and the Karashop telephone adapter

Status: done

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

## Design decisions (2026-11-14)

The operator said "Questions answered" for the points of the design step. The task text decides the provider ("Karashop only") and proposes the shape of the two calls; the implementer decided the rest as below, each by the practice of an earlier task. Each difference from `bill` is in `docs/this_vs_bill.md`, items 36 and 37.

1. **Two Karashop calls, one flow call (the proposal of the task).** The adapter sends `tci-end-getBillAccInfo` (full term) and `tci-mid-getBillAccInfo` (mid term) at the same time inside one `Inquiry` call and returns a mid-term and a final-term inquiry. The flow writes one request row (bill type `tel`, `tid` prefix 4) and saves two `bill_inquiries` rows. The calls run at the same time so the wait is one call, not two; each call has `providers.karashop.timeout`.
2. **The bill ID of each part.** `provider.InquiryItem` gets an optional `BillID`: the flow saves it on the row of that inquiry (else the bill ID of the answer, as before for every other adapter). The answer mapping of `Phone.Inquiry` moves to `phoneInquiryResponse` (shared by `Phone` and `Tel`) and sends the bill ID of each inquiry; for the mobile bill types it is the same value as before (one bill ID per answer). The bill ID of the answer is the full-term one (the mid-term one when it is empty). The trace ID is the two transaction IDs joined with `,` (only the full-term one when the join is longer than 100 bytes, the column length).
3. **The answer of each call** (bill: `telStatusMap`, `resultStatusCodeMap`, `statusCodeMap`): `telStatus` 1, 3, 4: success (a null amount is 0, a negative amount is 0). `telStatus` 2, `status.code` 400 and 404: refused with bill's text (HTTP 400). Every other answer: provider down (HTTP 503): `result.status.code` set (codes 1 to 6: method, definitions, province header, access, provider down, duplicate data: faults of the integration, not of the user), `status.code` 200 or 202 with no success `telStatus`, 401, 403, 410, 422, 429, 499, 500, 502, 503, 504, 520 and every unknown code (item 26 practice).
4. **When one of the two calls fails:** a provider-down call makes the inquiry provider down and stops the other call at once (code review, 2026-11-14; the error of the call that failed first is reported); else a refused call makes it refused (the full term is reported first). Nothing is saved, so the cache never holds one part only.
5. **The provider pick and the rows.** `billTypes` gets `tel` with `onlyProvider: karashop` and `skipSameTypePair: true`. The pick of (`tel`, `inquiry`) is the seed row 124325 (priority 9; Parsian 40 and Qabzino 52 are disabled); the saved rows get `provider_id` 124325, as bill (bill saves every step with the picked row of `tel`). The row 123456 (`telmidterm`) is not used.
6. **When Karashop is not the enabled provider:** the answer of "no enabled provider" (HTTP 503, one error line `tel: no enabled provider for inquiry`, no row, no call), the mechanism of tasks 035 and 037.
7. **The request check** (bill: `validateInquiryTelRequest`): phone or `billId`, and phone or `payId`; a `billId` that is sent is valid. `billId` and `payId` together: the raw inquiry (`inquiry.Service.RawInquiry`; its error mapping is shared with gas as `answerRawInquiry`); the answer has only `fullTerm`, as bill. Else `helpers.NormalizePhone` (bill's `NormalizePhoneNumber`) and `helpers.ValidTel` (`98` and ten digits): no new rule (conventions § 5); an invalid phone is HTTP 400 (bill sends it to the provider).
8. **`caching.tel`:** default `1h` (bill), `0` turns the cache off, a negative value is rejected, not hot-reloadable (task 021 decision 1).
9. **Config:** `providers.karashop.enabled` (default `false`), `timeout` (default `30s`, bill), `code`, `password` (`Secret`), `bill_inquiry_url`, `bill_payment_info_url` (not read); no default for the credential and the URLs. `config.yml` has the values of bill's `config.example.yaml` with `enabled: true`; `config.yml.example` has the dummied copy.
10. **Mock** (`debug.mocks.karashop`, dev mode only, `providers.karashop.enabled` true): bill ID `2101234560040` (a valid bill ID of the type tel), mid-term 120 000 rials (pay ID `12000035`), final-term 350 000 rials (pay ID `35000042`); both pay IDs are valid and their amount rule gives the amount; no deadline. `return_type: error` is provider down.
11. **JSON:** the answer is read on a hot path with `fastjson` (two pooled parsers: the outer object and the JSON text of `responseText`); the request is written with `encoding/json/v2`.
12. **TLS:** the adapter checks the certificate (bill skips the check). The chain of `core.digitalport.ir` (`*.digitalport.ir`, Certum) verified on 2026-11-14.
13. **Metrics and logs:** no new metric. `qabz_inquiries_total{bill_type="tel"}` (also `raw`), `qabz_provider_calls_total{provider="karashop"}` and `qabz_provider_call_duration_ms` (the two calls together) count it; the log prefix is `tel:`; the password is never in a log line or an error text.
14. **Not in this task:** the details (`serviceStatus`), the payment info (`SendPaymentInfo`), the tel order endpoints (HTTP 501).

## Implementation record (2026-11-14)

- `provider`: `BillTypeTel`; `InquiryItem.BillID`.
- `core/inquiry`: the `tel` entry of `billTypes` (`tid` prefix 4, `helpers.ValidTel`, `onlyProvider: karashop`, `skipSameTypePair`); the row bill ID from the inquiry when set.
- `provider/karashop` (new): the adapter, `MockFixtures`.
- `entry/grpc`: `tel.go` (new: `Tel`, `RegisterTel`, `validTelRequest`); `phoneInquiryResponse` (from `Phone.Inquiry`); `answerRawInquiry` (from `Gas.rawInquiry`).
- `config`: the `providers.karashop.*` keys, `caching.tel`, `debug.mocks.karashop.*`, their defaults and checks.
- `app`: the Karashop adapter in `newProviderRegistry`, `newTel`, the Tel gRPC service and gateway handler, the TTL of `tel`, the help text of `qabz_inquiries_total`.
- Tests: `provider/karashop/karashop_test.go`, `core/inquiry/tel_test.go`, `entry/grpc/tel_test.go`, `entry/http/tel_test.go`, `app/tel_test.go`, `app/tel_live_test.go`, `config` `TestParseKarashopKeys`; four old tests that named `tel` as an unknown bill type or counted the bill types are updated.
- Docs: `system.html` v2.10, `this_vs_bill.md` v0.17 (items 36, 37), `folder_structure.md` v0.44, `src/app/README.md` v0.31, `config.yml.example`, `qabz.postman_collection.json` (folder `Tel`).

## Verification record (2026-11-14, implementer)

- Build, vet, `gofmt`: clean. `make test GO_TEST_ARGS="-race -count=1 -cover"`: every package passes but `qabz/config` `TestLocalConfigInSync` (the old task 023 follow-up (c): the local `config.yml` has the `providers.tavanir.circuit_breaker` block commented out; the new keys are in both files). Coverage: `app` 87.7%, `config` 96.9% (with that test skipped), `core/inquiry` 99.3%, `entry/grpc` 98.8%, `entry/http` 88.4%, `provider` 96.4%, `provider/karashop` 96.8%.
- Live tests (the run database): `TestLiveTelInquiryFlow`, `TestLiveTelMockError`, `TestLiveTelOnlyKarashop` pass, no skip.
- Positive controls (each test failed with the rule removed, then passed after the restore): `onlyProvider` → `TestTelOnlyKarashop`; `skipSameTypePair` → `TestTelSavedRowsThatDoNotAnswer`; the row bill ID → `TestTelMissSavesTwoInquiries`; the `telStatus` 2 refusal → `TestInquiryTelStatusMap`; the mid-term error → `TestInquiryOneCallFails`; the `ValidTel` check → `TestTelInquiryPhoneForms`; the bill ID of each part → `TestTelInquiryMapsRequestAndAnswer`.
- Feature ON (Rule 15): an App copy on ports 18080 / 18082 / 19090 (the operator debug App on 8080 stayed up), config copies in the scratchpad with `debug.mocks.karashop.enable: true`, `providers.general.load_interval: 5s`, `debug.auth.disable: true`. No call to Karashop.
  1. Mock `success`, `{"phone":"02199250251"}`: HTTP 200, `midTerm` `2101234560040` / `12000035` / 120000, `fullTerm` `2101234560040` / `35000042` / 350000, empty deadlines. One request row (`tel`, `karashop`, `success`, trace `mock-karashop-0001`, `tid` prefix 4, inquiry ID `982199250251`: the 98 form); two `bill_inquiries` rows of provider 124325 (`midterm`, `final_term`). Log lines `provider mock: inquiry answered with no call to the provider` and `inquiry: provider call` (2 inquiries). Pass.
  2. `{"phone":"982199250251"}`: the same answer; still one request row; two copies (`is_cached`); one mock line; `qabz_inquiries_total{bill_type="tel",provider="karashop",result="db_hit"} 1`, `provider_ok 1`, `qabz_provider_calls_total{provider="karashop",result="ok"} 1`. Pass.
  3. Restart with mock `error`, `982199250252`: HTTP 503 `{"code":14,...}` (provider-down text); one request row `failed`, no trace; no `bill_inquiries` row; one error line `tel: provider inquiry failed`. Pass.
  4. Mock `error`, the number of step 1: HTTP 200 with the saved answer; `db_hit 1`. Pass.
  5. Restart with mock `success`; row 124325 disabled, wait 7 s: `982199250253` HTTP 503; then row 52 (Qabzino `tel`) enabled, wait 7 s: `982199250254` HTTP 503 with the error `only "karashop" may answer, the pick is "qabzino"`. No request row, no mock line; `qabz_inquiries_total{bill_type="tel",provider="none",result="no_provider"} 2`. Pass. Rows 124325 (enable) and 52 (disable) restored.
  - Also: `{"phone":"12345"}` HTTP 400 `invalid parameter`; `{"billId":"2101234560040","payId":"35000042"}` HTTP 200 with only `fullTerm` (350000) and an empty `midTerm`.
- After the run: the test rows of the numbers `9821992502__` deleted (6 inquiry rows, 2 request rows). The providers rows 35, 36 and 43 differ from the seed (24 enabled, the seed has 23); this run did not change them.
- Step 6 (the first real call) is for the operator.
- Code review (`code-reviewer` agent, 2026-11-14): NOT READY with 4 nil-safety defects (no nil check of the HTTP response in `karashop.go`; three test indexes or `err.Error()` with no guard). All four fixed; the minor point "a failed call waits for the other call" fixed too (`TestInquiryOneFailureStopsTheOther`, `TestInquiryEmptyAnswer`; positive controls: both failed with the stop or the cause rule removed). `make test GO_TEST_ARGS="-race -count=1"` after the fixes: every package passes but the old `TestLocalConfigInSync`; `provider/karashop` coverage 97.0%.

