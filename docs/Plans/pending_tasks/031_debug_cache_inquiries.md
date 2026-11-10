<!-- VERSION: 0.1 -->
# 031_debug_cache_inquiries — `debug.cache_inquiries`: a development switch for saving and reading inquiries in the database

Status: pending

## Description

Add the config key `debug.cache_inquiries` (bool, in the `debug` section, so it is for development only). It controls every use of the inquiry tables by the inquiry flow (`core/inquiry`, `Service.Inquire`).

- **Enabled (default `true`): the flow works as today.** The database is read first (`FindValid` in `fromDatabase`, with the copies it saves). On a miss the provider is called, the `bill_inquiry_requests` row is written (`CreateRequest`, `FinishRequest`) and the inquiries of the answer are saved (`SaveInquiries` in `fromProvider`).
- **Disabled: the inquiry flow does not touch the database.** It does not read saved inquiries (every call goes to the provider) and it writes nothing: no `bill_inquiries` row, no copy for the user, and no `bill_inquiry_requests` row.

The default is `true`, so a missing key or a mode other than `dev` (task 020c removes the whole `debug` section, so every `debug.*` key keeps its default) behaves as today. A `false` value is possible only in `dev` mode.

Points for the design step:
- The key name and place: `debug.cache_inquiries` as the operator named it, one flag for all bill types (it is independent of `caching.<bill type>`; the TTL keys keep their meaning when the flag is on).
- Where the flag is applied: in `core/inquiry` (an `Options` field, as `CacheTTL`), not in each handler, so every bill type (water, electricity, and tasks 023 to 025 later) follows it with no new code.
- Whether the flag is hot-reloadable (the `Config` field and `config.Holder`, as the TTLs) or read at start. Follow the pattern of the other `debug.*` keys and the TTL keys, and state the choice in the task.
- The request row with the flag off. The operator rule is "nothing stored regarding inquiries", so the `bill_inquiry_requests` rows are skipped too. The flow needs a request ID and a TID for the provider call only if the adapter needs them; check this against `provider.InquiryRequest` and keep the TID logic for the provider trace if it is used.
- A warn line at start when the flag is `false` (the mocks do the same), so a developer sees it.
- Metrics: the existing `qabz_inquiries_total` results stay; with the flag off a call is `provider_ok` or an error, never `db_hit`.
- `docs/this_vs_bill.md`: `bill` has no such key. Add a numbered item and a `@todo` at the code (task 013 practice).

## Definition of Done

- `debug.cache_inquiries` exists in `config.Config` with the default `true`, in `config.yml.example` (in the `debug` section, with a comment that it is development only) and in `system.html` (critical file, same commit). A mode other than `dev` ignores it (the default `true`), a test covers it.
- With `true`, the flow is unchanged: database first, provider only on a miss, the request row and the inquiries saved.
- With `false`: no read of saved inquiries, no `bill_inquiries` row (not even the copy for the user), no `bill_inquiry_requests` row, for a first call, a repeated call, a provider error and a refused answer. Each call goes to the provider. A start warn line names the flag.
- Unit tests with a fake repository (no database): flag on and off in `core/inquiry` (assert zero `FindValid`, `SaveInquiries`, `CreateRequest` and `FinishRequest` calls with the flag off, and the unchanged calls with it on, for success, provider error, refused answer and an existing valid row); the wiring in `app` (the config value reaches the service); the config key (default, YAML `false`, ignored outside `dev`).
- Integration tests (real Postgres, throw-away schema, as `TestLiveWaterInquiry*` and `TestLiveElectricityInquiry*`): with the flag on, the first call saves the rows and the second is answered from the database; with the flag off, two calls give two provider calls and `bill_inquiries` and `bill_inquiry_requests` have zero rows after both. A positive control: the same assertion fails when the flag check is taken out (scratch change).
- Build, vet and tests (with `-race`) pass; the changed code is covered by the tests. `this_vs_bill.md`, the app README (if the wiring text changes) and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with a mock provider (`debug.mocks.qabzino.enable: true`, `return_type: success`; Qabzino enabled for (`water`, `inquiry`) as in task 021 `Verification`). Count the rows before and after each step: `SELECT count(*) FROM bill_inquiries; SELECT count(*) FROM bill_inquiry_requests;`.

1. `debug.cache_inquiries: false`. Call `POST /v1/water/inquiry` with a new `billId` twice. Pass: both answers are the mock answer; the mock log line shows two provider calls; both row counts are unchanged (zero new rows); the start log has the warn line.
2. Same flag, `return_type: error`, a new `billId`. Pass: the error answer; both row counts are still unchanged.
3. `debug.cache_inquiries: true` (or the key removed). Call a new `billId` twice. Pass: one request row, one `bill_inquiries` row, then one copy row with `is_cached = true`; the second call writes no mock log line and `qabz_inquiries_total{result="db_hit"}` is 1.
4. `mode: "stage"` with `debug.cache_inquiries: false` in the file. Pass: the App starts with no warn line and the flow behaves as step 3 (the debug section is removed).
