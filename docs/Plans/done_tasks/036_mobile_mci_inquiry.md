<!-- VERSION: 0.5 -->
# 036_mobile_mci_inquiry — POST /v1/mobile/inquiry for the MCI operator

Status: done

## Description

Task 024 (`mobile_inquiry_endpoint`) was split by operator into tasks 035 (Irancell), 036 (MCI) and 037 (Rightel). This task makes `Phone.Inquiry` (`POST /v1/mobile/inquiry`, `pb/proto/qabz/services.proto`) answer for the **MCI** operator. The request is `InquiryPhoneRequest` (`phone`, `operator`, `payId`, `billId`, `service`) and the response is `InquiryPhoneResponse` (`midTerm` and `fullTerm`, each with `billId`, `payId`, `price`, `paymentDeadline`, `providerId`), both in `pb/proto/bill/services.proto`.

**Shared part of tasks 035, 036 and 037.** The three tasks have one endpoint. The first of them that starts builds the shared part; the other two add one operator to it and do not copy it.
- The handler and the request check of `bill` (`validateInquiryRequest`: `phone` or `billId`; `operator` or `service`; `phone` or `payId`; valid enum values; a valid phone number; a `billId` that is sent is a valid bill ID).
- The operator is `request.operator`, or `request.service` when the operator is empty (`setOperator`). One table gives the bill type of each operator (`bill`: `operatorBillType`): Irancell → `irancell`, MCI → `mobile`, Rightel → `mobilemidtermRightel`.
- The inquiry ID is the phone, or the bill ID when the phone is empty (`controller.SetInquiryID`).
- The flow is the shared inquiry flow of tasks 021 to 023 (`core/inquiry`: one more entry per bill type in `billTypes`; `ProvidersService.PickProvider`, conventions § 13; `bill_inquiries` first, the provider only on a miss, one `bill_inquiry_requests` row, the error mapping of task 021). Do not copy the flow.
- One provider call gives a mid-term and a full-term inquiry. They are saved as separate `bill_inquiries` rows and returned in `midTerm` and `fullTerm` (`bill`: `handleResponseWithInquiry`). An answer from saved inquiries returns both the same way.

**Provider order (operator, 2026-11-11):** Jiring Switch first, Parsian next. No other provider answers an MCI inquiry. The order is the order of the sorted provider list of (`mobile`, `inquiry`) (`ProvidersService`, conventions § 13: priority ascending). It is set only with the priority of the rows, not in code.

**One request, one provider (operator, 2026-11-11, replaces the fallback of v0.2 and v0.3).** "When a provider returns an error or anything that cannot be used, that is the answer. The switch happens only when the circuit breaker detects the condition."
- One MCI request calls one provider: the pick of `PickProvider`. Its answer is the answer of the request: a success, a business answer (no debt, a refused phone, `pending`), an error or a timeout (provider-down, HTTP 503). Parsian is NOT called in the same request after a failed Jiring Switch call.
- The move to the next provider is part of the Automatic Provider Switch (`system.html`, Provider Switcher): when the circuit breaker of a provider detects the condition, the switcher demotes it, and the next requests pick the next provider. It is one generic mechanism for every bill type. Nothing in this task is special to MCI, Jiring Switch or Parsian (no provider name in the flow, no fallback loop). The circuit breakers and the switcher are not built yet (task 034 follow-up (a)); until then no provider is switched.
- A disabled row is skipped by the pick itself (`PickProvider` gives the first enabled row), so Parsian answers when the Jiring Switch row is disabled. This is not a switch.
- The rule is a lasting rule of every flow: conventions § 13 ("One request, one provider") and `system.html` (Provider Switcher), same commit.

**What is special to MCI:**
- **Bill type `mobile`, providers `jiring-switch` (first) and `parsian` (next).** In the seed, Jiring Switch (id 28) is disabled with priority 10 and Parsian (id 39) is enabled with priority 2 for (`mobile`, `inquiry`): the seed order is the opposite of the operator order, so the dev database needs a change (`status = 'enable', priority = 1` on row 28), or the seed by an operator decision, before Jiring Switch can answer. The rows of `mobilemidterm` (ids 27 and 41) are not used by this endpoint: the operator table never gives that bill type.
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
- ~~The in-request fallback (points of v0.3)~~: dropped by the operator rule "one request, one provider" above. The flow is not changed for the fallback; the switch belongs to the Automatic Provider Switch.
- The order "Jiring Switch first, Parsian next" is kept by the priority of the rows only (no fixed rule for this operator). A runtime change of the order (`MoveUp` / `MoveDown`, later the switcher) is counted by `qabz_providers_changes_total`.
- Each adapter: a direct client of the provider (as tasks 030 and 034 did for Qabzino and Tavanir) or a client of the microservice. Its config keys under `providers.<name>` (conventions § 8).
- The cache TTL key of the bill type `mobile` and its default (`bill`: `caching.mobile`), and the cache skip rule above.
- The answer for an operator whose task is not done (when this task is the first of the three).
- The `midTerm` and `fullTerm` of the answer when the provider gives one inquiry, or no debt.
- The fixed mock answer and the error of `return_type: error`.
- Metrics and log lines (conventions §§ 9, 12): the task 021 metrics with the bill type of this operator.

## Definition of Done

- `POST /v1/mobile/inquiry` with the MCI operator works as described: database first, provider only on a miss, the mid-term and the full-term inquiry saved and returned.
- The shared part exists once (built here if this is the first of tasks 035 to 037, else reused with no copy).
- The Jiring Switch adapter and the Parsian adapter are registered for (`mobile`, `inquiry`). The first enabled row of the sorted list answers; one request calls one provider; its error is the answer (no call to the next provider in the same request). Each adapter has a mock that works only in dev mode. `config.yml` and `config.yml.example` are updated (conventions § 8).
- Unit tests with mocks and stubs (no database, no provider): a valid row in the database (no provider call), no row (one provider call, rows saved), an expired row, a row of another provider, a paid row, two saved rows of the same inquiry type, a provider error, no enabled provider, an invalid request, each operator value; the provider order (Jiring Switch is the pick: only Jiring Switch is called, also for a business answer; Jiring Switch fails (provider down, timeout, `pending`): its answer is the answer and Parsian is not called; Parsian is the pick (the Jiring Switch row is disabled): only Parsian is called); each adapter (request fields, answer mapping, errors); the mock (`success`, `error`, off, rejected outside dev mode); the config keys.
- Integration tests (real Postgres, throw-away schema): the first request saves the rows, the second request within the TTL is answered from the database with no provider call.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with the mocks of the Jiring Switch and Parsian adapters enabled (`providers.jiring_switch.enabled` and `providers.parsian.enabled` true, `debug.mocks.jiring_switch` and `debug.mocks.parsian` on). First run `UPDATE providers SET status = 'enable', priority = 1 WHERE id = 28;` (Jiring Switch first) and keep the Parsian row (id 39) enabled; restore row 28 (`status = 'disable', priority = 10`) at the end.

1. `return_type: success` on the two mocks. Call `POST /v1/mobile/inquiry` with a new `phone` and the MCI `operator`. Pass: a success answer with the Jiring Switch mock values in `midTerm` and `fullTerm`; the rows have the Jiring Switch provider; Parsian is not called; one `bill_inquiry_requests` row and two `bill_inquiries` rows.
2. Call again with the same `phone` within the TTL. Pass: the same answer, no provider call (the counter `db_hit`), no new request row.
3. With the two mocks on `return_type: error`, call with the `phone` of step 1 within the TTL. Pass: the success answer from the database.
4. Jiring Switch mock `return_type: error`, Parsian mock `return_type: success`. Call with another new `phone`. Pass: the provider-down answer (HTTP 503) — the error of Jiring Switch is the answer; one request row (`failed`, Jiring Switch); no `bill_inquiries` row; Parsian is not called (`qabz_provider_calls_total{provider="parsian"}` does not change).
5. Disable the Jiring Switch row (id 28) and wait for the provider reload, Parsian mock on `success`. Call with another new `phone`. Pass: a success answer with the Parsian mock values (the pick skips the disabled row); no Jiring Switch call.

## Implementation record (2026-11-11)

- Flow (`core/inquiry`): one more entry in `billTypes` (`mobile`: `tid` prefix 5, the phone, `skipSameTypePair` for the `bill` cache skip). No fallback code, no provider name. The package comment states the one-request-one-provider rule.
- Adapters: `provider/jiring` (gRPC client of `jiring-switch`, `InternalInquiryService/BillInquiry`) and `provider/parsianproxy` (gRPC client of `parsian-v2`, `TopBill/Inquiry`, bill type `mobile_mci`), each with `enabled`, `timeout`, `grpc_address` under `providers.jiring_switch` / `providers.parsian` and a dev mock. `make proto` generates their clients.
- Handler: `phoneOperators[mci] = mobile`.
- Bill types of an adapter (operator decision, 2026-11-11, "Keep 503, no row"): `ProviderInterface.SupportsInquiry(billType)` in every adapter. The Parsian adapter made the seed rows of `parsian` for water (44) and gas (43) reach an adapter that answers only `mobile` (HTTP 500 and a `failed` row; four live tests in `app` failed). The flow now answers such a pick as "no adapter" before the saved-inquiry read and the request row: HTTP 503, no row, as before this task. The mock has the same bill types (it embeds the adapter).
- Config: `caching.mobile` (default `5h`, as `bill`), the provider keys and the two mocks; `config.yml.example` and the local `config.yml`.
- Docs: `system.html` (MCI paragraph, the generic switch rule in Provider Switcher), conventions § 13, `this_vs_bill.md` item 32, the Postman collection.

## Verification record (2026-11-11, implementer)

- Build, vet, `gofmt`: clean. `go test -race -count=1 ./...` in `src/`: all packages pass but `qabz/config` `TestLocalConfigInSync` (the old task 023 follow-up (c): the local `config.yml` has the `providers.tavanir.circuit_breaker` block commented out; not from this task).
- Live tests (`make test GO_TEST_PKGS=./app/ GO_TEST_ARGS="-race -count=1 -v -run MCI"`, the run database): `TestLiveMCIInquiryFlow`, `TestLiveMCIParsianWithSeed`, `TestLiveMCIErrorIsTheAnswer` pass, no skip.
- Feature ON (Rule 15): an App copy on ports 18080 / 18082 / 19090 (the operator debug App on 8080 stayed up), config copies in the scratchpad with `providers.jiring_switch.enabled` and `providers.parsian.enabled` true, both mocks on; providers row 28 set to `enable`, priority 1 before the run.
  1. Phone `989120000361`, `"operator":"mci"`, both mocks `success`: HTTP 200, `midTerm` 210000 / pay ID `21003611`, `fullTerm` 610000 / `61003622`, bill ID `9120000036` (the Jiring Switch mock); one request row (`mobile`, `jiring-switch`, `success`, `tid` `580211888165315`: prefix 5); two `bill_inquiries` rows of provider 28 (`midterm`, `final_term`). Pass.
  2. The same call: the same answer; `qabz_inquiries_total{bill_type="mobile",provider="jiring-switch",result="db_hit"} 1`, `qabz_provider_calls_total{provider="jiring-switch",result="ok"} 1` (no second call); no new request row. Pass.
  3. Restart with both mocks `error`, the phone of step 1: HTTP 200 with the saved answer, `db_hit` 1, no provider call. Pass.
  4. Jiring Switch mock `error`, Parsian mock `success`, phone `989120000362`: HTTP 503 `{"code":14,...}` (provider-down text); one request row (`jiring-switch`, `failed`); no `bill_inquiries` row; one error line `mobile: provider inquiry failed`; no `qabz_provider_calls_total{provider="parsian"}` series. Pass: the error of the pick is the answer.
  5. Row 28 disabled, wait for the reload (70 s), phone `989120000363`: HTTP 200 with the Parsian mock (bill ID `9120000039`, 220000 / 620000, deadline `2026-12-20`); request row `parsian`, `success`, trace `mock-parsian-0001`; two rows of provider 39; Jiring Switch call count unchanged. Pass.
  - Also: `"operator":"rightel"` HTTP 501, `"operator":"unknown"` HTTP 400, no operator HTTP 200 (MCI, as bill).
- After the run: row 28 restored (`disable`, priority 10), the test rows of the phones `98912000036_` deleted (10 inquiry rows, 4 request rows).
- `SupportsInquiry` check (after the operator decision): the four live tests `TestLiveElectricityInquiryFlow`, `TestLiveGasInquiryFlow`, `TestLiveGasInquiryProviderCases`, `TestLiveWaterInquiryProviderCases` failed before the change (gRPC `Internal`, one request row) and pass after it (the positive control). Feature ON: providers row 36 (Qabzino water) disabled, so parsian row 44 is the water pick; the Parsian mock on; `POST /v1/water/inquiry` `{"billId":"1234567890125"}`: HTTP 503 `{"code":14,...}`, `qabz_inquiries_total{bill_type="water",provider="parsian",result="error"} 1`, one error line `water: provider inquiry failed` with `no adapter for the provider "parsian" for the bill type "water"`, no mock line, no request row. Row 36 enabled again. Pass.
- Config (operator, 2026-11-11): bill's keys of the two service clients added under `providers.jiring_switch` and `providers.parsian` (`name`, `get_status.*`, Parsian `settlement_inquiry_*`), with the operator values in the local `config.yml` (`grpc_address` `jiring-switch:8084` and `parsian-v2:8084`) and empty values in `config.yml.example`. The seven real Qabzino and Tavanir endpoint URLs left `config.yml.example` and `system.html`; their code default is now `""` (the example test compares the example with the defaults).
- Real calls (2026-11-11, mocks off, `enabled: true` in a config copy): Jiring Switch (row 28 enabled, priority 1) and then Parsian (row 28 disabled) each answered HTTP 503 in less than 0.1 s with one error line `mobile: provider inquiry failed` (`rpc error: code = Unavailable desc = name resolver error: produced zero addresses`). The hosts `jiring-switch` and `parsian-v2` have no DNS answer on this machine. The real path up to the gRPC client works; an answer of the services was not seen. Row 28 restored, the two request rows deleted. Awaiting a reachable address (for example a port-forward) and operator confirmation.

