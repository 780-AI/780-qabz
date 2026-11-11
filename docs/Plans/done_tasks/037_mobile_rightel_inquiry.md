<!-- VERSION: 0.2 -->
# 037_mobile_rightel_inquiry — POST /v1/mobile/inquiry for the Rightel operator

Status: done

## Description

Task 024 (`mobile_inquiry_endpoint`) was split by operator into tasks 035 (Irancell), 036 (MCI) and 037 (Rightel). This task makes `Phone.Inquiry` (`POST /v1/mobile/inquiry`, `pb/proto/qabz/services.proto`) answer for the **Rightel** operator. The request is `InquiryPhoneRequest` (`phone`, `operator`, `payId`, `billId`, `service`) and the response is `InquiryPhoneResponse` (`midTerm` and `fullTerm`, each with `billId`, `payId`, `price`, `paymentDeadline`, `providerId`), both in `pb/proto/bill/services.proto`.

**Shared part of tasks 035, 036 and 037.** The three tasks have one endpoint. The first of them that starts builds the shared part; the other two add one operator to it and do not copy it.
- The handler and the request check of `bill` (`validateInquiryRequest`: `phone` or `billId`; `operator` or `service`; `phone` or `payId`; valid enum values; a valid phone number; a `billId` that is sent is a valid bill ID).
- The operator is `request.operator`, or `request.service` when the operator is empty (`setOperator`). One table gives the bill type of each operator (`bill`: `operatorBillType`): Irancell → `irancell`, MCI → `mobile`, Rightel → `mobilemidtermRightel`.
- The inquiry ID is the phone, or the bill ID when the phone is empty (`controller.SetInquiryID`).
- The flow is the shared inquiry flow of tasks 021 to 023 (`core/inquiry`: one more entry per bill type in `billTypes`; `ProvidersService.PickProvider`, conventions § 13; `bill_inquiries` first, the provider only on a miss, one `bill_inquiry_requests` row, the error mapping of task 021). Do not copy the flow.
- One provider call gives a mid-term and a full-term inquiry. They are saved as separate `bill_inquiries` rows and returned in `midTerm` and `fullTerm` (`bill`: `handleResponseWithInquiry`). An answer from saved inquiries returns both the same way.

**Provider constraint (operator, 2026-11-11): a Rightel inquiry MUST use Qabzino and nothing else.** No other provider answers a Rightel inquiry. When Qabzino is not available, the answer is the provider-down answer (`Unavailable`); there is no fallback.

**What is special to Rightel:**
- **Bill type `mobilemidtermRightel`, provider Qabzino** (seed id 34, `qabzino-rightel-inquiry`, enabled). The row 32 (`mobileRightel`, disabled) is not used by this endpoint: the operator table never gives that bill type.
- **`bill` gateway** (`gateway/inquiry/mobile_rightel.go`, `QabzinoRightelMobile`) calls the `qabzino` microservice with the bill type `rightel`. The `qabzino` service (`gateway/rightel_mobile.go`) calls Ayantech `RightelMobileBillInquiry` with `MobileNumber` (`preparePhoneRequest`: `0` and the ten digits) and `TraceNumber`. The answer has `MidTerm` and `FinalTerm`, each with `BillID`, `PaymentID` and `Amount`; the bill ID of the answer is the mid-term bill ID; `rightelMobileBillInquiryMapper` gives the status of each Ayantech code.
- **The adapter exists.** The direct Qabzino adapter (`provider/qabzino`, tasks 030 and 023) gets one more kind in its `kinds` table and one new config key for the Rightel endpoint. The HTTP code is not copied.

Help:
- `bill`: `controller/phone/inquiry.go`, `controller/phone/phone.go`, `gateway/inquiry/mobile_rightel.go`, `gateway/inquiry/base.go` (`handlePhoneResponseWithoutInquiry`), `app/inquiryGateway.go`, `service/billInquiry/inquiry.go`.
- The `qabzino` service: `gateway/rightel_mobile.go`, `gateway/base.go`, `gateway/translator.go`, `client/qabzino/dto_new.go` (`RightelBillInquiry2`), `config.example.yaml` (`rightelBillEndpoint`).
- The seed: `src/system/postgres/devdb/seed_providers.sql` (ids 32, 34).
- Task 023 (`docs/Plans/done_tasks/023_gas_inquiry_endpoint.md`) is the newest pattern: its design decisions (6 and 9 for the Qabzino kind and the mock), the `billTypes` table, the Postman collection, the metrics.
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).

Points for the design step:
- How the flow keeps the constraint when another provider row is enabled for the bill type `mobilemidtermRightel` (the pick must not give another provider to this operator).
- The name of the new config key (for example `providers.qabzino.rightel_bill_endpoint`, as the keys of task 029).
- The cache TTL key of the bill type `mobilemidtermRightel` and its default (`bill`: `caching.mobile`).
- The answer for an operator whose task is not done (when this task is the first of the three).
- The `midTerm` and `fullTerm` of the answer when the provider gives one inquiry, or no debt.
- The fixed mock answer (a new fixture in `qabzino.MockFixtures`) and the error of `return_type: error`.
- Metrics and log lines (conventions §§ 9, 12): the task 021 metrics with the bill type of this operator.

## Definition of Done

- `POST /v1/mobile/inquiry` with the Rightel operator works as described: database first, provider only on a miss, the mid-term and the full-term inquiry saved and returned.
- The shared part exists once (built here if this is the first of tasks 035 to 037, else reused with no copy).
- A Rightel inquiry is answered only by Qabzino: a test shows that no other provider is called, also when Qabzino is disabled or fails (the answer is `Unavailable`).
- The Qabzino adapter serves (`mobilemidtermRightel`, `inquiry`); `debug.mocks.qabzino` gives the fixed Rightel answer and works only in dev mode; the water and gas answers of the adapter do not change. `config.yml` and `config.yml.example` are updated (conventions § 8).
- Unit tests with mocks and stubs (no database, no provider): a valid row in the database (no provider call), no row (one provider call, rows saved), an expired row, a row of another provider, a paid row, a provider error, no enabled provider, an invalid request, each operator value; the adapter with a local `httptest` server (request fields, phone format, answer mapping, each status code class, errors); the mock (`success`, `error`, off, rejected outside dev mode); the config key.
- Integration tests (real Postgres, throw-away schema): the first request saves the rows, the second request within the TTL is answered from the database with no provider call.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with `providers.qabzino.enabled: true` and `debug.mocks.qabzino.enable: true`. The seed already has Qabzino enabled for (`mobilemidtermRightel`, `inquiry`) (id 34). Do NOT run a real inquiry as a check: Ayantech charges for each call.

1. `return_type: success`. Call `POST /v1/mobile/inquiry` with a new `phone` and the Rightel `operator`. Pass: a success answer with the mock values in `midTerm` and `fullTerm`; one `bill_inquiry_requests` row and the `bill_inquiries` rows are in the database; a log line shows the provider (mock) call.
2. Call again with the same `phone` within the TTL. Pass: the same answer, no provider call (the counter `db_hit` shows the database source), no new provider request row.
3. `return_type: error`. Call with another new `phone`. Pass: the error answer; the request row has the failed status; no `bill_inquiries` row is saved.
4. With `return_type: error`, call with the `phone` of step 1 within the TTL. Pass: the success answer from the database.
5. Disable the row 34 in the dev database (`UPDATE providers ...`) and wait for the provider reload. Call with another new `phone`. Pass: the provider-down answer (HTTP 503); no other provider is called; then enable the row again.

## Design decisions (2026-11-11)

- **The constraint:** the `billTypes` entry of `mobilemidtermRightel` has `onlyProvider: qabzino` (the mechanism of Irancell, task 035). When the pick gives another provider, the flow calls nobody, reads no saved inquiry, writes no row and answers HTTP 503.
- **Config key:** `providers.qabzino.rightel_bill_endpoint` (no default, as the other endpoints since task 036).
- **Cache TTL (operator, 2026-11-11, "caching.mobile"):** the bill type uses `caching.mobile` (default `5h`), as bill. No same-type skip (bill's `shouldSkipCache` is only for `mobile` and `tel`).
- **No operator is "not available":** the HTTP 501 branch stays for a future operator (`TestPhoneInquiryOperatorNotAvailable`).
- **Answer:** `G00000`: the mid-term bill ID, a mid-term and a final-term inquiry with the pay ID and amount of each part, no deadline (as the qabzino service). `GD2268`: no debt (`provider.NoDebtMobileInquiries`). `GD2261`, `GD2267`: refused with bill's texts. `GD2266` and every other code: provider down (item 26 practice).
- **Mock:** bill ID `9200000034`, mid-term 230 000 (pay ID `23000341`), final-term 630 000 (pay ID `63000342`), no deadline; `return_type: error` is provider down.
- **Metrics and logs:** no new metric; the task 021 metrics count `bill_type="mobilemidtermRightel"`; the log prefix is `mobilemidtermRightel:`.

## Implementation record (2026-11-11)

- Flow (`core/inquiry`): one more `billTypes` entry (`tid` prefix 6, the phone rule, `onlyProvider: qabzino`). No other flow change.
- Adapter (`provider/qabzino`): one more kind (`rightel_bill_endpoint`, `rightelParameters` with `zeroFormPhone`, `rightelSuccessAnswer`, the Rightel codes); the kind table gets `noDebt` (nil: the one regular inquiry of water and gas, unchanged). The mock fixture answers by bill type.
- Handler: `phoneOperators[rightel] = mobilemidtermRightel`. App: `RightelEndpoint` and the TTL map.
- Docs: `system.html`, `this_vs_bill.md` item 33, `src/app/README.md`, the config examples, the Postman collection.

## Verification record (2026-11-11, implementer)

- Build, vet, `gofmt`: clean. `make test GO_TEST_ARGS="-race -count=1"`: all packages pass but `qabz/config` `TestLocalConfigInSync` (the old task 023 follow-up (c): the local `config.yml` has the `providers.tavanir.circuit_breaker` block commented out; not from this task; the new key `rightel_bill_endpoint` is in both files).
- Live tests (the run database): `TestLiveRightelInquiryFlow`, `TestLiveRightelMockError`, `TestLiveRightelOnlyQabzino` pass, no skip; the Irancell and MCI live tests still pass.
- Positive control: with `onlyProvider` removed from the Rightel entry, `TestRightelOnlyQabzino` fails; with `noDebt` set to nil, `TestRightelNoBillToPay` fails. Both pass after the restore.
- Feature ON (Rule 15): an App copy on ports 18080 / 18082 / 19090 (the operator debug App on 8080 stayed up), config copies in the scratchpad with `providers.qabzino.enabled: true`, `debug.mocks.qabzino.enable: true`, `debug.auth.disable: true`. No call to Ayantech. In the dev database the other `mobilemidtermRightel` rows (37 `eways2`, 500 `parsian`) are `order` rows, so they do not touch the inquiry pick.
  1. Mock `success`, phone `989200000371`, `"operator":"rightel"`: HTTP 200, `midTerm` bill ID `9200000034`, pay ID `23000341`, 230000; `fullTerm` `63000342`, 630000; empty deadlines. One request row (`mobilemidtermRightel`, `qabzino`, `success`, `tid` `61328859912315`: prefix 6, trace `mock-qabzino-0001`); two `bill_inquiries` rows of provider 34 (`midterm`, `final_term`). The log line `inquiry: provider call` (provider `qabzino`, 2 inquiries). Pass.
  2. The same call: the same answer; `qabz_inquiries_total{bill_type="mobilemidtermRightel",provider="qabzino",result="db_hit"} 1`, `qabz_provider_calls_total{provider="qabzino",result="ok"} 1` (no second call); no new request row; two copies (`is_cached`). Pass.
  3. Restart with mock `error`, phone `989200000372`: HTTP 503 `{"code":14,...}` (provider-down text); one request row `failed`; no `bill_inquiries` row; one error line `mobilemidtermRightel: provider inquiry failed`. Pass.
  4. Mock `error`, phone of step 1: HTTP 200 with the saved answer, `db_hit` 1. Pass.
  5. Restart with mock `success`, row 34 disabled, wait 70 s for the reload, phone `989200000373`: HTTP 503 `{"code":14,...}`; no request row; one error line `mobilemidtermRightel: no enabled provider for inquiry`; `qabz_inquiries_total{...,provider="none",result="no_provider"} 1`; no provider call. Pass. Row 34 enabled again.
- After the run: row 34 is `enable`; the test rows of the phones `98920000037_` deleted (6 inquiry rows, 2 request rows).
