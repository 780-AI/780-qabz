<!-- VERSION: 0.2 -->
# 031_debug_cache_inquiries — `debug.cache_inquiries`: a development switch for saving and reading inquiries in the database

Status: done

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

## Design decisions (2026-11-11, operator approved "Cool")

1. **Key:** `debug.cache_inquiries` (`config.Config.DebugCacheInquiries`, bool, default `true`). A mode other than `dev` removes the debug section (task 020c), so the value is `true` there.
2. **Where:** in `core/inquiry` only, `Options.NoStore bool`. The zero value is the flow as before, so every existing `Options{}` (and test) is unchanged. The App sets `NoStore: !cfg.DebugCacheInquiries` in `newInquiryFlow`. Every bill type follows it with no handler change.
3. **Not hot-reloadable:** read when the flow is built at start, as the `caching.*` keys and the other `debug.*` keys.
4. **With the flag off:** no `FindValid`, no copy `SaveInquiries`, no `CreateRequest`, no `FinishRequest`, no answer `SaveInquiries`. No `tid` is made: `provider.InquiryRequest` has no TID field, so no adapter uses it. The provider trace ID is not kept (only the request row had it). The answer, the error mapping, the `inquiry: provider call` line and the metrics are unchanged; the result is never `db_hit`. The `id` of an inquiry in the answer is a new UUID that is not a row.
5. **Start warn line:** `inquiry store disabled (debug.cache_inquiries: false): development only, every inquiry calls the provider and nothing is saved`.
6. **Docs:** `config.yml.example` and the local `config.yml` (debug section), `system.html`, `this_vs_bill.md` item 34 with a `@todo` at the code, `src/app/README.md`.

## Implementation record (2026-11-11)

- `src/config/config.go`: `DebugCacheInquiries` (path `debug.cache_inquiries`, default `true`) + `config_test.go` `TestParseCacheInquiries` (default in each mode, `false` in dev, ignored in stage and prod, not a bool rejected).
- `src/core/inquiry/inquiry.go`: `Options.NoStore`, `Service.noStore`; `Inquire` skips `fromDatabase`; `fromProvider` skips the tid, the request row, the status and the saved answer; `finish` writes nothing. The `@todo` names item 34. Tests: `nostore_test.go` (new; success twice with a valid saved row, provider down, refused, other error, empty answer, client leaves: zero Repository calls, no tid, no error line; the same rig with the store on answers from the saved row); `inquiry_test.go` (`newRigStore`).
- `src/app/defaults.go`: `newInquiryFlow` sets `NoStore` and writes the warn line. Tests: `cacheinquiries_test.go` (new; the key reaches the flow: with `false` a call succeeds with no database (port 1) and one warn line; with `true` it fails at the request row and no warn line), `cacheinquiries_live_test.go` (new; real Postgres).
- `config.yml.example`, `config.yml`: `debug.cache_inquiries: true`.

## Verification record (2026-11-11, implementer)

- Build, vet, `gofmt`: clean. `make test GO_TEST_ARGS="-race -count=1"`: every package passes but `qabz/config` `TestLocalConfigInSync`, the old task 023 follow-up (c) (the five `providers.tavanir.circuit_breaker` keys are commented out in the local `config.yml`); the new key is in both files.
- Live: `TestLiveNoCacheInquiries` (two calls, two provider calls, zero rows in both tables; a valid saved row does not answer; a mock error gives HTTP 503 and zero rows) and `TestLiveWaterInquiryFlow` (the flag on, unchanged) pass, no skip.
- Positive control (`go test -overlay`, scratch copies only): with `NoStore` not set in `newInquiryFlow`, `TestLiveNoCacheInquiries` and `TestNewInquiryFlowCacheInquiries` fail; with `NewService` ignoring `Options.NoStore`, `TestNoStoreOptionIsKept`, `TestNoStoreSuccessCallsProviderEveryTime` and `TestNoStoreFailedCalls` fail. The real files pass.
- Feature ON (Rule 15): an App copy on ports 18080 / 18082 / 19090 (the operator debug App on 8080 stayed up), config copies in the scratchpad with no real token and no real endpoint, `providers.qabzino.enabled: true`, `debug.mocks.qabzino.enable: true`, `debug.auth.disable: true`; the dev database has Qabzino (row 36) as the first enabled water provider. No call to Ayantech. Row counts before: `bill_inquiries` 13, `bill_inquiry_requests` 22.
  1. `cache_inquiries: false`, mock `success`, bill `3131000000017` twice: both HTTP 200 with the mock answer (25 000 000, pay ID `2500012345`, `2030-01-01`); two mock lines; counts 13 / 22 (unchanged); the start log has the warn line; `qabz_inquiries_total{bill_type="water",provider="qabzino",result="provider_ok"} 2`, no `db_hit`. Pass.
  2. `false`, mock `error`, bill `3131000000025`: HTTP 503 with the provider-down text; one line `water: provider inquiry failed`; counts 13 / 22. Pass.
  3. `true`, mock `success`, bill `3131000000033` twice: no warn line; one mock line; one request row `success`, one `bill_inquiries` row and one copy (`is_cached = true`); `db_hit` 1, `provider_ok` 1. Pass.
  4. `mode: stage`, the file has `cache_inquiries: false` (and the mock on; a scratch HMAC secret and a local test token, the auth check is on in stage): no warn line, no mock line (the debug section is removed). Bill `3131000000041` twice: two request rows `failed` (the store is used, so the key is ignored); HTTP 503 because the real adapter has an empty endpoint and sends nothing. Pass (the flow is the store-on flow; the mock answer of step 3 cannot show outside dev).
- After the run: the test rows of the bills `313100000___` were deleted (2 inquiry rows, 3 request rows); counts are 13 / 22 again.
