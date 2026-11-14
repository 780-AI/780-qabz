<!-- VERSION: 3.17 -->
<!-- Last Updated: 2026-11-11 -->
<!--
CHANGE LOG
v3.17 — 2026-11-11 — Added session-wrap-up (task 031 session) entry.
v3.16 — 2026-11-11 — Added 031-debug-cache-inquiries entry.
v3.15 — 2026-11-11 — Added session-wrap-up (task 037 session) entry.
v3.14 — 2026-11-11 — Added 037-mobile-rightel-inquiry entry.
v3.13 — 2026-11-11 — Added session-wrap-up (task 036 session) entry.
v3.12 — 2026-11-11 — Added 036-mobile-mci-inquiry entry.
v3.11 — 2026-11-11 — Added session-wrap-up (task 024 split, tasks 035 and 038 session) entry.
v3.10 — 2026-11-11 — Added 038-task entry.
v3.9 — 2026-11-11 — Added postman-bill-type-variables entry.
v3.8 — 2026-11-11 — Added 035-mobile-irancell-inquiry entry.
v3.7 — 2026-11-11 — Added 036-next-provider-rule entry.
v3.6 — 2026-11-11 — Added 036-jiring-not-available-meaning entry.
v3.5 — 2026-11-11 — Added 024-split-into-035-036-037 entry.
v3.4 — 2026-11-11 — Added session-wrap-up (task 023 session) entry.
v3.3 — 2026-11-11 — Added 023-gas-inquiry-endpoint entry.
v3.2 — 2026-11-10 — Added session-wrap-up (tasks 033 and 034 session) entry.
v3.1 — 2026-11-10 — Added 034-tavanir-direct-provider entry.
v3.0 — 2026-11-10 — Added 033-tavanir-provider-config entry.
v2.9 — 2026-11-10 — Added inquiry-database-answer-log-removed entry.
v2.8 — 2026-11-10 — Added 030-qabzino-direct-provider entry.
v2.7 — 2026-11-10 — Added 029-qabzino-provider-config entry.
v2.6 — 2026-11-10 — Added 022-electricity-inquiry-endpoint entry.
v2.5 — 2026-11-10 — Added 022-to-025-inquiry-tasks entry (tax, tax-organization and traffic-fine removed from the qabz proto).
v2.4 — 2026-11-09 — Added 021-water-inquiry-endpoint entry.
v2.3 — 2026-11-09 — Added 020c-debug-section-dev-only entry.
v2.2 — 2026-11-09 — Added 020b-test-database-isolation entry.
v2.1 — 2026-11-09 — Added 020b and 020c task entry.
v2.0 — 2026-11-09 — 020-providers-sorted-list entry: fix round 4 (operator decision 9, "Apply at once": a hot change of providers.general.load_interval applies at once; config.Holder.Changed).
v1.99 — 2026-11-09 — Added 021-task entry.
v1.98 — 2026-11-08 — 020-providers-sorted-list entry: fix round (active-count gauge, nil safety, shutdown cancel, maps.Clone, load timeout docs); fix round 2 (2026-11-09: aborted-start cancel rule, load-timeout and live Stop-during-query tests, test guards); fix round 3 (2026-11-09: live hot-reload test of providers.general.load_interval, test guards, change-metric pairs, aborted-load Help text).
v1.97 — 2026-11-08 — Added 020-providers-sorted-list entry.
v1.96 — 2026-11-08 — Added session-wrap-up (agents and conventions § 12 session) entry.
v1.95 — 2026-11-08 — Added 019b-mybill-delete-success-text entry.
v1.94 — 2026-11-08 — Added metrics-convention entry.
v1.93 — 2026-11-08 — Added 019b-task entry.
v1.92 — 2026-11-08 — Added coverage-gate-convention entry.
v1.91 — 2026-11-08 — Added 019-mybill-delete-endpoint entry.
v1.90 — 2026-11-08 — Added 020-task entry.
v1.89 — 2026-11-08 — Added 019-task entry.
v1.88 — 2026-11-08 — Added 017-mybill-list-endpoint entry.
v1.87 — 2026-11-08 — Added session-wrap-up (tasks 015 and 018 session) entry.
v1.86 — 2026-11-08 — Added 018-make-help entry.
v1.85 — 2026-11-08 — Added 018-task entry.
v1.84 — 2026-11-08 — Added 015-dev-bill-database entry.
v1.83 — 2026-11-08 — Added session-wrap-up (task 016 session) entry.
v1.82 — 2026-11-08 — Added error-logging-convention entry.
v1.81 — 2026-11-08 — Added 016-mybill-insert-endpoint entry.
v1.80 — 2026-11-08 — Added 017-task entry.
v1.79 — 2026-11-08 — Added 016-task entry.
v1.78 — 2026-11-08 — Added session-wrap-up (task 014 review session) entry.
v1.77 — 2026-11-07 — Added 014-review-test-fixes entry.
v1.76 — 2026-11-07 — Added session-wrap-up (task 014 session) entry.
v1.75 — 2026-11-07 — Added 014-request-metrics-middleware entry.
v1.74 — 2026-11-07 — Added durations-in-milliseconds-convention entry.
v1.73 — 2026-11-07 — Added session-wrap-up entry.
v1.72 — 2026-11-07 — Added 015-task entry.
v1.71 — 2026-11-07 — Added 014-task entry.
v1.70 — 2026-11-07 — Added 013-services-endpoint entry.
v1.69 — 2026-11-07 — Added 013-task entry.
v1.68 — 2026-11-07 — Added 012-auth-status-403 entry.
v1.67 — 2026-11-07 — Added 012-task entry.
v1.66 — 2026-11-07 — Added 011-jwt-no-audience-check entry.
v1.65 — 2026-11-07 — Added 011-task entry.
v1.64 — 2026-11-07 — Added config-example-convention entry.
v1.63 — 2026-11-07 — Added 010-jwt-hmac-secret entry.
v1.62 — 2026-11-07 — Added 010-task entry.
v1.61 — 2026-11-07 — Added 009-auth-middleware entry.
v1.60 — 2026-11-07 — Added 009-task entry.
v1.59 — 2026-11-07 — Added postman-collection-convention entry.
v1.58 — 2026-11-07 — Added 008-logs-to-stdout entry.
v1.57 — 2026-11-07 — Added 008-task entry.
v1.56 — 2026-11-07 — Added 007-make-compose-actions entry.
v1.55 — 2026-11-07 — Added 007-task entry.
v1.54 — 2026-11-04 — Added 006-greeting-ping entry.
v1.53 — 2026-11-04 — Added pb-ownership-convention entry.
v1.52 — 2026-11-04 — Added 006-task entry.
v1.51 — 2026-11-04 — Added PostgresDSN-net-url entry.
v1.50 — 2026-11-04 — Added standard-library-first entry.
v1.49 — 2026-11-04 — Added one-active-task entry.
v1.48 — 2026-11-04 — Added nil-safety-convention entry.
v1.47 — 2026-11-04 — Added nil-guards entry.
v1.46 — 2026-11-04 — Added Implementation-Plan-Step-4 entry.
v1.45 — 2026-11-02 — Added 005-yml-to-env-generator entry.
v1.44 — 2026-11-02 — Added 005-task-verification entry.
v1.43 — 2026-11-02 — Added 005-task-output-name entry.
v1.42 — 2026-11-02 — Added 004-yaml-config entry.
v1.41 — 2026-11-02 — Added 004-task-tests-pass entry.
v1.40 — 2026-11-02 — Added 004-task-rewrite entry.
v1.39 — 2026-11-02 — Added 004-task-type-rules entry.
v1.38 — 2026-11-02 — Added 004-task-slice-rule entry.
v1.37 — 2026-11-02 — Added 005-task-python entry.
v1.36 — 2026-11-02 — Added 004-task-hot-default entry.
v1.35 — 2026-11-02 — Added 004-task-metrics-key entry.
v1.34 — 2026-11-02 — Added 004-task-migration-keys entry.
v1.33 — 2026-11-02 — Added 004-task-key-placement entry.
v1.32 — 2026-11-02 — Added 005-task-update entry.
v1.31 — 2026-11-02 — Added 005-task entry.
v1.30 — 2026-11-02 — Added 004-task-key-mapping entry.
v1.29 — 2026-11-02 — Added 004-task-panic-rule entry.
v1.28 — 2026-11-02 — Added 004-task-answers entry.
v1.27 — 2026-11-02 — Added 004-task-update entry.
v1.26 — 2026-11-02 — Added task-ID-prefix and 004-task entry.
v1.25 — 2026-11-02 — Added no-down-migrations entry.
v1.24 — 2026-11-02 — Added config-hot-reload entry.
v1.23 — 2026-11-02 — Added config-hot-reload-task-reorder entry.
v1.22 — 2026-11-02 — Added config-hot-reload-task-update entry.
v1.21 — 2026-11-02 — Added config-hot-reload-task entry.
v1.20 — 2026-11-02 — Added app-bootstrap entry.
v1.19 — 2026-11-02 — Added task-counter entry.
v1.18 — 2026-11-02 — Added CLAUDE.md-tasks-rule entry.
v1.17 — 2026-11-02 — Added app-bootstrap-task entry.
v1.16 — 2026-11-02 — Added project-base-skeleton entry.
v1.15 — 2026-11-02 — Added task-folders entry.
v1.14 — 2026-11-01 — Added yml_to_env upper-case-flag entry.
v1.13 — 2026-11-01 — Added encoding/json-ban entry.
v1.12 — 2026-11-01 — Added yml_to_env entry.
v1.11 — 2026-11-01 — Added docker-compose entry.
v1.10 — 2026-11-01 — Added Go-modules-rule entry.
v1.9 — 2026-11-01 — Added Go-module entry.
v1.8 — 2026-11-01 — Added history-rewrite entry.
v1.7 — 2026-11-01 — Added project-conventions entry.
v1.6 — 2026-11-01 — Added Makefile (`make proto`) entry.
v1.5 — 2026-11-01 — Added pb-submodule entry.
v1.4 — 2026-10-28 — Added OPEN_TASKS-committed entry.
v1.3 — 2026-10-28 — Added folder_structure full-tree entry.
v1.2 — 2026-10-28 — Added initial-discovery move + HTML versioning entry.
v1.1 — 2026-10-28 — Added manifest README version-cell fix entry.
v1.0 — 2026-10-28 — Initial.
-->

# CHANGELOG

## [2026-11-11] — Session wrap-up (task 031 session)
- **Type:** Minor (documentation; no code, no new files).
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.60: 2 lesson candidates, more evidence for 2); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior. The push of 921f145..374585e (task 031) is done.
- **QA scenarios:** none (documentation only).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — Task 031: `debug.cache_inquiries`, a development switch of the inquiry tables
- **Type:** Major (a new config key, domain logic of the inquiry flow, Critical File `system.html`)
- **Files:** `src/config/config.go` (`DebugCacheInquiries`, path `debug.cache_inquiries`, default `true`) + `config_test.go`; `src/core/inquiry/inquiry.go` (`Options.NoStore`: no `FindValid`, no request row, no status, no saved inquiry or copy, no tid) + `nostore_test.go` (new), `inquiry_test.go` (`newRigStore`); `src/app/defaults.go` (`newInquiryFlow` sets `NoStore` and writes one warn line) + `cacheinquiries_test.go` (new), `cacheinquiries_live_test.go` (new); `config.yml.example` (and the local `config.yml`); `docs/initial-discovery/system.html` (v2.8); `docs/this_vs_bill.md` (v0.15, item 34); `src/app/README.md` (v0.29); `docs/documents_manifest.md`; `docs/Plans/OPEN_TASKS.md` (v1.59); the task file v0.2 (design, implementation and verification record; moved to `done_tasks/`).
- **Impact:** Yes, behavior changes only with the new key set to `false` in dev mode. (1) New key `debug.cache_inquiries` (default `true`; development only: a mode other than `dev` removes the debug section, so stage and prod always have `true`; read at start). (2) `true`, or no key: the inquiry flow is unchanged for every bill type. (3) `false`: the flow of every bill type does not read or write `bill_inquiries` and `bill_inquiry_requests`: no saved inquiry answers (the `caching.*` TTLs do not apply), and no request row, no inquiry and no copy is written, also for a provider error or a refused answer; every inquiry calls the provider; the answers, the error mapping, the log lines and the metrics are the same (never `db_hit`); one warn line at start names the key. `bill` has no such key (`this_vs_bill.md` item 34).
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 031 `debug.cache_inquiries`": unit and live tests (`make test`); feature ON against the compose dev database with the Qabzino mock (steps 2a to 2d). Run by the implementer on 2026-11-11: all pass (evidence in the task file).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — Session wrap-up (task 037 session)
- **Type:** Minor (documentation; no code, no new files).
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.58: 2 lesson candidates, more evidence for 3; the operator report of the first real Rightel call, `GD2266`); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior. The push of bc27f56..359f2c9 (tasks 036 and 037, the task 036 wrap-up) is done.
- **QA scenarios:** none (documentation only).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — Task 037: `POST /v1/mobile/inquiry` for the Rightel operator, the Qabzino Rightel inquiry
- **Type:** Major (endpoint behavior, domain logic, a new config key, Critical File `system.html`)
- **Files:** `src/provider/provider.go` (`BillTypeMobileMidTermRightel`); `src/provider/qabzino/qabzino.go` (the `mobilemidtermRightel` kind: `RightelEndpoint`, `rightelParameters`, `zeroFormPhone`, `rightelSuccessAnswer`, the Rightel codes; `kind.noDebt`; the Rightel mock fixture and the `MockRightel...` constants) + `rightel_test.go` (new), `qabzino_test.go`; `src/core/inquiry/inquiry.go` (the `mobilemidtermRightel` entry of `billTypes`: `tid` prefix 6, the phone rule, `onlyProvider: qabzino`) + `rightel_test.go` (new), `gas_test.go`; `src/entry/grpc/phone.go` (`phoneOperators[rightel] = mobilemidtermRightel`) + `phone_test.go`; `src/entry/http/phone_test.go`; `src/app/defaults.go` (`RightelEndpoint`, `caching.mobile` for the new bill type) + `rightel_live_test.go` (new), `phone_live_test.go`; `src/config/config.go` (`providers.qabzino.rightel_bill_endpoint`) + `config_test.go`; `config.yml.example` (and the local `config.yml`, with the real URL); `qabz.postman_collection.json` (request Phone / Inquiry Rightel, variables `rightel_mobile_phone`, `rightel_mobile_billId`, `rightel_mobile_payId`; the Irancell item no longer names Rightel as HTTP 501); `docs/initial-discovery/system.html` (v2.7); `docs/this_vs_bill.md` (v0.14, item 33; item 31 updated); `docs/folder_structure.md` (v0.43); `src/app/README.md` (v0.28); `docs/documents_manifest.md`; `docs/Plans/OPEN_TASKS.md` (v1.57); the task file v0.2 (implementation and verification record; moved to `done_tasks/`).
- **Impact:** Yes, behavior changes. (1) `POST /v1/mobile/inquiry` with `"operator":"rightel"` no longer answers HTTP 501: it runs the shared inquiry flow with the bill type `mobilemidtermRightel` (the inquiry ID is the phone, `tid` prefix 6); a mid-term and a final-term inquiry are saved as two `bill_inquiries` rows and returned in `midTerm` and `fullTerm`. (2) Only Qabzino may answer (operator rule): when the pick of (`mobilemidtermRightel`, `inquiry`) is another provider, or no row is enabled, nobody is called and the answer is HTTP 503 (the mechanism of Irancell, `onlyProvider`). (3) The Qabzino adapter sends Ayantech `RightelMobileBillInquiry` with the phone in the 0 form; `G00000` gives the mid-term bill ID and the two inquiries (no deadline); `GD2268` is no debt; `GD2261` and `GD2267` are refused (HTTP 400); `GD2266` and every other code are provider down (HTTP 503). (4) New config key `providers.qabzino.rightel_bill_endpoint` (no default; empty: every Rightel call fails as provider down). The cache TTL is `caching.mobile` (the key of MCI, as bill; operator decision); no new key for it. (5) The Qabzino mock answers a Rightel inquiry with bill ID `9200000034`, 230 000 and 630 000 rials. (6) No new metric: the three inquiry metrics count `bill_type="mobilemidtermRightel"`. Water, gas, electricity, Irancell and MCI are unchanged (their tests pass with no change of an expected value). No operator answers HTTP 501 now.
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 037 `POST /v1/mobile/inquiry` for the Rightel operator": unit and live tests (`make test`); feature ON against the compose dev database with the Qabzino mock (steps 1-5 of the task Verification). Run by the implementer on 2026-11-11: all pass (evidence in the task file).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — Session wrap-up (task 036 session)
- **Type:** Minor (documentation; no code, no new files).
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.56: 4 lesson candidates, more evidence for 2); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior. The push of d5f748f (task 036) is held for the operator.
- **QA scenarios:** none (documentation only).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — Task 036: `POST /v1/mobile/inquiry` for the MCI operator, the Jiring Switch and Parsian adapters
- **Type:** Major (endpoint behavior, new packages `provider/jiring` and `provider/parsianproxy`, domain logic, new config keys, a lasting rule in the conventions doc, Critical File `system.html`)
- **Files:** `src/provider/jiring/jiring.go` and `src/provider/parsianproxy/parsianproxy.go` (new: the adapters, `Dial`, `MockFixtures`, the `Mock...` constants; the `.gitkeep` files are removed) + tests; `src/provider/provider.go` (`BillTypeMobile`, `NoDebtMobileInquiries`; `ProviderInterface.SupportsInquiry`) + `mock_test.go`; `SupportsInquiry` in every adapter (`provider/qabzino`, `qabzinoproxy`, `tavanir`, `irancellproxy`, `jiring`, `parsianproxy`; each `Inquiry` uses it) + one test each; `src/core/inquiry/inquiry.go` (the `mobile` entry of `billTypes`: `tid` prefix 5, the phone rule, `skipSameTypePair`; a pick whose adapter does not inquire the bill type is `ErrNoAdapter`; the package comment states "one request, one provider") + `mobile_test.go`, `inquiry_test.go`, `electricity_test.go`, `gas_test.go`; `src/entry/grpc/phone.go` (`phoneOperators[mci] = mobile`) + `phone_test.go`; `src/entry/http/phone_test.go`; `src/app/defaults.go` (the two adapters in `newProviderRegistry`, `caching.mobile`; the mock warn line names `debug.mocks.jiring_switch`) + `mci_test.go`, `mci_live_test.go`, `phone_live_test.go`, `water_test.go`; `src/config/config.go` (the new keys; bill's keys of the two service clients; no default for the seven Qabzino and Tavanir endpoint URLs) + `config_test.go`; `Makefile` (`proto` also generates the jiring-switch and parsian-v2 service clients into `src/api/pb/jiringSwitch` and `src/api/pb/parsian-v2`); `config.yml.example` (and the local `config.yml`); `qabz.postman_collection.json` (request Phone / Inquiry MCI, variables `mci_mobile_phone`, `mci_mobile_billId`, `mci_mobile_payId`); `docs/initial-discovery/system.html` (v2.6); `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.19, § 13 "One request, one provider"); `docs/this_vs_bill.md` (v0.13, item 32; item 31 updated); `docs/folder_structure.md` (v0.42); `src/app/README.md` (v0.27); `docs/documents_manifest.md`; `docs/Plans/OPEN_TASKS.md` (v1.55); the task file v0.5 (verification record; moved to `done_tasks/`).
- **Impact:** Yes, behavior changes. (1) `POST /v1/mobile/inquiry` with `"operator":"mci"` (and a request with no operator, as bill) no longer answers HTTP 501: it runs the shared inquiry flow with the bill type `mobile` (the inquiry ID is the phone, `tid` prefix 5); a mid-term and a final-term inquiry are saved as two `bill_inquiries` rows and returned in `midTerm` and `fullTerm`. Two saved rows of the same inquiry type do not answer (bill: `shouldSkipCache`). (2) The provider is the first enabled row of (`mobile`, `inquiry`) by priority: Jiring Switch (row 28) or Parsian (row 39). One request calls one provider; its error is the answer (HTTP 503, request row `failed`); the flow never calls the next provider (operator rule, conventions § 13). In the dev seed Parsian answers (row 28 is disabled). (3) The Jiring Switch adapter calls the jiring-switch service (`InternalInquiryService/BillInquiry`); the Parsian adapter calls the parsian-v2 service (`TopBill/Inquiry`, bill type `mobile_mci`); both over gRPC, no TLS, a lazy connection. (4) New config keys (both config files): `providers.jiring_switch.enabled` and `providers.parsian.enabled` (`false`), `timeout` (`10s`), `grpc_address` (empty: provider down); `caching.mobile` (`5h`, as bill); `debug.mocks.jiring_switch` and `debug.mocks.parsian` (`enable: false`, `return_type: success`); bill's keys of the two service clients (operator, 2026-11-11), with no default and not read yet: `providers.jiring_switch.name` and `get_status.*`, `providers.parsian.name`, `get_status.*` and `settlement_inquiry_water`, `_electricity`, `_gas`, `_telephone`, `_mci_mobile`, `_rightel_mobile` (bill's `0` is written `"0s"`: a duration is a string). (5) No new metric: the three inquiry metrics count `bill_type="mobile"`. (6) `ProviderInterface` has the new method `SupportsInquiry(billType)`: when the picked provider's adapter does not inquire the bill type, the flow answers as for a provider with no adapter (HTTP 503, one error line, no saved-inquiry read, no request row, no call; operator decision). Without it, the new Parsian adapter would have changed the answer of the seed rows of `parsian` for water and gas from HTTP 503 to HTTP 500 with a `failed` request row. (7) `config.yml.example` and `system.html` have no real URL any more (operator): the seven endpoint keys `providers.qabzino.electricity_bill_endpoint`, `gas_bill_endpoint`, `water_bill_endpoint` and `providers.tavanir.auth_endpoint`, `basic_branch_endpoint`, `bill_document_endpoint`, `branch_debit_endpoint` lose their default (it was the real Ayantech or SAAPA URL; now `""`). A `config.yml` without these keys now makes every Qabzino or Tavanir call fail as provider down; the local `config.yml` has the real URLs, so nothing changes on this machine. Water, electricity, gas and Irancell are unchanged otherwise (their tests pass with no change of an expected value, but the two default tests of the endpoint keys). Rightel still answers HTTP 501 (task 037).
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 036 `POST /v1/mobile/inquiry` for the MCI operator": unit and live tests (`make test`); feature ON against the compose dev database with both mocks (steps 1-5 of the task Verification: the Jiring Switch answer and rows, the second call from the database, both mocks on `error` and the saved answer, the Jiring Switch error is the answer with no Parsian call, row 28 disabled gives the Parsian answer; a water inquiry with the Parsian pick gives HTTP 503 and no row). Run by the implementer on 2026-11-11: all pass (evidence in the task file).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — Session wrap-up (task 024 split, tasks 035 and 038 session)
- **Type:** Minor (documentation; no code, no new files).
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.54: 4 lesson candidates, more evidence for 2, task 035 follow-up (c) updated); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior. The session commits 736dffb..6cbfc45 are pushed; this commit is pushed with the wrap-up.
- **QA scenarios:** none (documentation only).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — New task 038: normalize the incoming mobile number
- **Type:** Minor (task file; no code).
- **Files:** `docs/Plans/pending_tasks/038_mobile_phone_normalization.md` (new task, v0.1); `docs/Plans/pending_tasks/task_counter.json` (next ID 039); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior. The task (operator request after the report that `09373969041` gets HTTP 400 on `POST /v1/mobile/inquiry`) normalizes every incoming mobile number to the 98 form before the request check: a leading `0`, no prefix, `+98`, `0098`, separators, Persian and Arabic digits. Task files are not manifest-tracked.
- **QA scenarios:** none (task file only; the task has its own Verification section).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — Postman: collection variables for the request IDs of each bill type
- **Type:** Minor (Postman collection; no code).
- **Files:** `qabz.postman_collection.json` (new collection variables `water_billId`, `water_payId`, `electricity_billId`, `electricity_payId`, `gas_billId`, `gas_payId`, `gas_participateCode`, `irancell_mobile_billId`, `irancell_mobile_payId`, `irancell_mobile_phone`; the bodies of Water, Electricity, Gas and Phone Inquiry and of MyBills Insert use them; the file is written again, so three older requests change format only); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior and none on the API (operator request). The default values are those that the bodies had before; MyBills Insert now sends `{{water_billId}}` (default `1234567890125`, a valid bill ID) in place of `123455`. With both `gas_billId` and `gas_payId` set, a gas request is the raw inquiry (the variable description says so).
- **QA scenarios:** none (Postman only). Check: `python3 -m json.tool qabz.postman_collection.json` passes.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — Task 035: `POST /v1/mobile/inquiry` for the Irancell operator, the Irancell adapter (the irancell service)
- **Type:** Major (new endpoint, new package `provider/irancellproxy`, domain logic, new config keys, Critical File `system.html`)
- **Files:** `src/provider/irancellproxy/irancellproxy.go` (new: the adapter, `Dial`, `MockFixtures`, the `Mock...` constants; the `.gitkeep` is removed) + tests; `src/provider/provider.go` (`BillTypeIrancell`, `InquiryTypeMidterm`, `InquiryTypeFinalTerm`); `src/core/inquiry/inquiry.go` (the `irancell` entry of `billTypes`: `tid` prefix 8, the phone rule, `onlyProvider`) + `irancell_test.go`, `gas_test.go`; `src/entry/grpc/phone.go` (new: `Phone.Inquiry`, `phoneOperators`, `validPhoneInquiry`) + tests; `src/entry/http/phone_test.go`; `src/app/defaults.go` (the Irancell adapter in `newProviderRegistry`, `caching.irancell`, `newPhone`, the Phone service and gateway handler) + `phone_test.go`, `phone_live_test.go`, `water_live_test.go`, `water_test.go`; `src/config/config.go` + tests; `Makefile` (`proto` also generates the irancell service client into `src/api/pb/irancell`); `config.yml.example` (and the local `config.yml`); `qabz.postman_collection.json` (new folder Phone, request Inquiry); `docs/initial-discovery/system.html` (v2.5); `docs/this_vs_bill.md` (v0.12, items 30 and 31); `docs/folder_structure.md` (v0.41); `src/app/README.md` (v0.26); `docs/documents_manifest.md`; `docs/Plans/OPEN_TASKS.md` (v1.53); the task file v0.2 (design decisions, verification record; moved to `done_tasks/`).
- **Impact:** Yes, behavior changes. (1) New endpoint `POST /v1/mobile/inquiry` (`Phone.Inquiry`, bill's request and answer unchanged): the operator `irancell` runs the shared inquiry flow with the bill type `irancell` (the inquiry ID is the phone, `tid` prefix 8); one answer has a mid-term and a final-term inquiry, saved as two `bill_inquiries` rows and returned in `midTerm` and `fullTerm`; `providerId` is 0, as bill. The operators `mci` and `rightel` (and a request with no operator) answer HTTP 501 until tasks 036 and 037; `unknown` and an invalid request answer HTTP 400. (2) Only the Irancell provider (providers row 38) may answer: when the provider pick gives another provider, nobody is called and the answer is HTTP 503 (operator rule). (3) The Irancell adapter calls the irancell service (our own service) over gRPC (`pb.InternalService/BillInquiry`); a failed call is HTTP 503 and writes `failed` on the request row (bill writes `pending`: `this_vs_bill.md` item 30). (4) New config keys (both config files): `providers.irancell.enabled` (`false`), `providers.irancell.timeout` (`10s`), `providers.irancell.grpc_address`, `name`, `nats_subject` and the `get_status` block (bill's keys; no default; only `grpc_address` is read), `caching.irancell` (`0s`: no cache, as bill), `debug.mocks.irancell.enable` (`false`) and `return_type` (`success`). (5) No new metric: the three inquiry metrics count `bill_type="irancell"`. Water, electricity and gas are unchanged (their tests pass with no change of an expected value).
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 035 `POST /v1/mobile/inquiry` for the Irancell operator": unit and live tests (`make test`); feature ON against the compose dev database with `debug.mocks.irancell.enable: true` (steps 1-5 of the task Verification: success answer and rows, second call from the database, `return_type: error`, the saved answer still served while the mock errors, row 38 disabled). Run by the implementer on 2026-11-11: all pass (evidence in the task file).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-11] — Task 036 (MCI inquiry): the next provider in the list takes the job
- **Type:** Minor (task file; no code).
- **Files:** `docs/Plans/pending_tasks/036_mobile_mci_inquiry.md` (v0.3); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior. The task records the operator rule "when provider 1 does not answer, the next provider in the row takes the job": the order is the sorted provider list of (`mobile`, `inquiry`), Jiring Switch first and Parsian next; a disabled row, an open circuit breaker or a call with no response gives the job to the next provider. The open point of the disabled Jiring Switch row is closed (Parsian answers). The task applies the rule to MCI only; a wider scope is a point for the operator.
- **QA scenarios:** none (task file only; the task has its own Verification section, now six steps).
- **Author:** Mostafa Talebi / Claude Fable 5.2

## [2026-11-11] — Task 036 (MCI inquiry): meaning of "Jiring Switch is not available"
- **Type:** Minor (task file; no code).
- **Files:** `docs/Plans/pending_tasks/036_mobile_mci_inquiry.md` (v0.2); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior. The task records the operator decision: "not available" means that Jiring Switch does not respond (the future circuit breaker, a failed call, a timeout). Parsian is then called in the same request. A business answer of Jiring Switch is a response, so Parsian is not called. The design points, the tests and the Verification steps follow this meaning; the case of a disabled Jiring Switch row is a point to confirm with the operator.
- **QA scenarios:** none (task file only; the task has its own Verification section).
- **Author:** Mostafa Talebi / Claude Fable 5.2

## [2026-11-11] — Task 024 (mobile inquiry) split into tasks 035 (Irancell), 036 (MCI) and 037 (Rightel)
- **Type:** Minor (task files; no code).
- **Files:** `docs/Plans/pending_tasks/035_mobile_irancell_inquiry.md`, `036_mobile_mci_inquiry.md`, `037_mobile_rightel_inquiry.md` (new tasks, v0.1); `docs/Plans/pending_tasks/024_mobile_inquiry_endpoint.md` (removed: replaced by the three tasks; the ID 024 is not reused); `docs/Plans/pending_tasks/task_counter.json` (next ID 038); `docs/Plans/pending_tasks/025_tel_inquiry_endpoint.md` (v0.2: names the new tasks in place of 024); `docs/Plans/OPEN_TASKS.md` (v1.52: the task 023 follow-up (a) names the new tasks); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior. One endpoint (`POST /v1/mobile/inquiry`), one task per operator; the first of the three that starts builds the shared part (handler, request check, operator table, the two inquiries of an answer). The new tasks correct task 024 from the `bill` code: Irancell uses the bill type `irancell` and the provider `irancell` (seed id 38), not the Qabzino rows 31 and 33; MCI uses `mobile` (Parsian id 39, Jiring Switch id 28); Rightel uses `mobilemidtermRightel` and Qabzino (id 34). Each task states the provider constraint of the operator (2026-11-11): Irancell only the Irancell provider; MCI Jiring Switch first and Parsian when Jiring Switch is not available; Rightel only Qabzino. Task files are not manifest-tracked.
- **QA scenarios:** none (task files only; each task has its own Verification section).
- **Author:** Mostafa Talebi / Claude Fable 5.2

## [2026-11-11] — Session wrap-up (task 023 session)
- **Type:** Minor (documentation; no code, no new files).
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.51: 3 lesson candidates, more evidence for one, the operator report for task 023, follow-up (e) updated); `CHANGELOG.md`; `docs/documents_manifest.md`.
- **Impact:** None on runtime behavior. The commit `67ca1d3` (task 023) and this commit are pushed.
- **QA scenarios:** none (documentation only).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-11] — Task 023: `POST /v1/gas/inquiry`, the Qabzino adapter inquires gas
- **Type:** Major (new endpoint, domain logic, config key, Critical File `system.html`)
- **Files:** `src/entry/grpc/gas.go` (new: `Gas.Inquiry`, bill's request check, the participate code as the inquiry ID, the raw inquiry) + `gas_test.go`; `src/core/inquiry/raw.go` + `raw_test.go` (new: `RawInquiry`, `ResultRaw`); `src/common/helpers/payid.go` + `payid_test.go` (new: `ValidPayID`, `PayIDAmount`, `BillTypeOfBillID`, the rules of bill's `pkg/helper/bill_type.go`); `src/entry/http/gas_test.go`; `src/core/inquiry/inquiry.go` (`billTypes`: the `tid` prefix and the inquiry ID rule of each bill type; gas: prefix 3, not empty) + `gas_test.go`, `inquiry_test.go`, `electricity_test.go` (the "unsupported bill type" example is `mobile`, not `gas`); `src/provider/provider.go` (`BillTypeGas`, `GasDetails`, `Details.Gas`); `src/provider/qabzino/qabzino.go` (`kinds`: one table for water and gas; `GasEndpoint`; the gas codes; the gas mock fixture) + `gas_test.go`; `src/system/postgres/inquiry_details.go` (gas `extra_data`: `city`, `kind`) + `inquiry_details_test.go`; `src/config/config.go` (`CachingGas`, `caching.gas`, default `1h`) + `config_test.go`; `src/app/defaults.go` (`newGas`, the Gas service and gateway handler, `GasEndpoint`, the gas TTL) + `gas_test.go`, `gas_live_test.go`, `water_live_test.go` (the live stack has the Gas service); `config.yml.example`; `config.yml` (git-ignored; `caching.gas`); `qabz.postman_collection.json` (Gas / Inquiry); `docs/initial-discovery/system.html` (v2.4), `docs/this_vs_bill.md` (items 28 and 29, item 26), `docs/folder_structure.md`, `src/app/README.md`; `docs/Plans/OPEN_TASKS.md`; task file `023_gas_inquiry_endpoint.md` (in `done_tasks/`).
- **Impact:** Behavior changes: `POST /v1/gas/inquiry` answered HTTP 501 before; it now runs the shared inquiry flow (database first, provider on a miss, the answer saved; the participate code is the cache key). The request and the response are bill's `InquiryBillPayRequest` and `InquiryBillPayResponse`, unchanged; no proto file is changed. **No change to the water and electricity endpoints:** the same requests, answers, error texts, rows and log lines (their tests pass with no change of an expected value). A water inquiry of Qabzino goes to the water endpoint as before. The seed is not changed: parsian is the enabled gas provider and has no adapter, so a gas inquiry is HTTP 503 until the Qabzino gas row (id 35) is enabled. A gas request with both `billId` and `payId` is bill's raw inquiry: HTTP 200 `{price, payId, billId}` read from the two IDs (no provider, no row, no log line, no `paymentDeadline`); an invalid pay ID is HTTP 400 `شناسه پرداخت نامعتبر است` (bill: HTTP 500 with the same text, `this_vs_bill.md` item 28). **Config:** new key `caching.gas` (default `1h`); `providers.qabzino.gas_bill_endpoint` is now read. No new metric: the three inquiry metrics get `bill_type="gas"`; `qabz_inquiries_total` has a new `result` value, `raw` (provider `none`).
- **QA scenarios:** `make test GO_TEST_ARGS="-race -count=1 -cover -v"` (869 pass): 0 skipped, the five gas live tests pass; one failure that this task does not cause (`TestLocalConfigInSync`: the local `config.yml` has the Tavanir `circuit_breaker` block commented out). Feature ON, dev mode, the Qabzino mock on, providers row 35 enabled: the first call gives the mock answer and saves the rows under the participate code; the second call comes from the database; another code is another key; `return_type: error` gives HTTP 503 for a new code and the saved answer for the first code; a request with only `billId` gives HTTP 400; a request with `billId` and `payId` gives the price from the pay ID with no provider call (also with the mock on `error`) and a wrong pay ID gives HTTP 400. The pay ID, amount and bill type rules were checked against the `bill` code on 400 000 random inputs (no difference). All pass (2026-11-11). No call to Ayantech. Details in `docs/Plans/OPEN_TASKS.md` and the task file.
- **Author:** Mostafa Talebi / Claude Fable 5.2

## [2026-11-10] — Inquiry answered from the database writes no log line
- **Type:** Minor (a log line removed; the counter already existed). Not a task file: task 030 is the one active task.
- **Files:** `src/core/inquiry/inquiry.go` (the `inquiry: answered from database, no provider call` info line removed from `fromDatabase`); `src/app/water_live_test.go`, `src/app/electricity_live_test.go` (the log assertion became "no such line"; the existing `qabz_inquiries_total{result="db_hit"}` assertions stay); `docs/Plans/OPEN_TASKS.md` (v1.46: QA steps 2 of tasks 021 and 022 check the counter).
- **Impact:** Yes, a log line is gone. A database answer is now counted only by `qabz_inquiries_total{bill_type,provider,result="db_hit"}` (a Prometheus counter, with metrics on). No change to answers, rows or other metrics.
- **QA scenarios:** `make test` (the two live tests check that the line is not written and that `db_hit` is counted); feature ON: call the same bill twice, then `curl localhost:9090/metrics`. Pass: the second call has no `answered from database` line and `qabz_inquiries_total{...result="db_hit"}` is 1.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-10] — Session wrap-up (tasks 033 and 034 session)
- **Type:** Minor (docs only)
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.49), `docs/documents_manifest.md` (v2.3), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. One lesson candidate added (a submodule pointer must be pushed after its submodule) and more evidence for "Run the `code-reviewer` agent before the task commit". The push of `27b69a0..52e71cd` is done: `pb` (`ac2e2eac`) to `qabz_service` first, then `main`.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-10] — Task 034: direct Tavanir (SAAPA) adapter, `providers.tavanir.enabled`; the gRPC client is removed
- **Type:** Major (provider adapter replaced, config key added and removed, Critical File `system.html`)
- **Files:** `src/provider/tavanir/tavanir.go` + `tavanir_test.go` (rewritten: an HTTPS client of the Tavanir endpoints; login, token in memory, inquiry with details, status codes; the package and the provider name stay `tavanir`); `src/config/config.go` (`TavanirEnabled`, default `false`; `TavanirGRPCAddress` removed) + `config_test.go`; `src/app/defaults.go` (registers the direct adapter; `disabledNotice` replaces `qabzinoNotice` and `noAddressNotice`; `enabled: false` wins over the mock) + `electricity_test.go`, `water_test.go`, `water_live_test.go`; `config.yml.example` (`enabled: false` in place of `grpc_address`); `config.yml` (git-ignored; `enabled: true` in place of `grpc_address`); `Makefile` (`make proto` no longer generates the Tavanir backend client); `docs/initial-discovery/system.html`, `docs/this_vs_bill.md` (item 27, item 24), `docs/folder_structure.md`, `src/app/README.md`; `docs/Plans/OPEN_TASKS.md`; task file `034_tavanir_direct_provider.md` (in `done_tasks/`).
- **Impact:** Behavior changes. An electricity inquiry no longer goes to the `tavanir` microservice: Qabz logs in to Tavanir (SAAPA) and calls its basic branch endpoint itself. With the default config (`enabled: false`) every Tavanir call is "provider down" (HTTP 503) and nothing is sent. **Config migration:** remove `providers.tavanir.grpc_address`, add `providers.tavanir.enabled`. Operator decisions: no rename (no `tavanirproxy`), no circuit breaker for now (the `circuit_breaker` keys are parsed and not used), no call to the `tavanir` microservice. `POST /v1/electricity/inquiry` keeps its request and its answer fields (no Postman change). No new metric (the login is part of the counted provider call).
- **QA scenarios:** `make test GO_TEST_ARGS="-race -count=1 -v"` (648 pass, no skip; `provider/tavanir` uses a local `httptest` server only). Feature ON, dev mode: mock on with `enabled: true` gives the mock answer and the second call from the database; `enabled: false` gives HTTP 503, the warn line and no mock line; the real adapter against a local fake Tavanir server (login, token reuse, new login after a 401, refused code 507, down code 502). All pass (2026-11-10). No call to `esrv.saapa.ir`; the first real call is for the operator. Details in `docs/Plans/OPEN_TASKS.md` and the task file.
- **Author:** Mostafa Talebi / Claude Fable 5.2

## [2026-11-10] — Task 033: Tavanir provider config (endpoints, credentials, circuit breaker); 030 closed
- **Type:** Major (new config keys, Critical File `system.html`)
- **Files:** `src/config/config.go` (11 new `providers.tavanir.*` fields: `auth_endpoint`, `basic_branch_endpoint`, `bill_document_endpoint`, `branch_debit_endpoint`, `username`, `password` as a `Secret`, `circuit_breaker.{name,max_requests,interval,timeout,failure_ratio}`; `failure_ratio` above 100 is rejected) + `config_test.go` (`TestParseTavanirProviderKeys`); `config.yml.example` (default URLs and circuit breaker, empty username and password); `config.yml` (git-ignored; the real username and password, `timeout: "120s"`); `docs/initial-discovery/system.html`; task file `033_tavanir_provider_config.md` (in `done_tasks/`); task 030 moved to `done_tasks/`; new pending task files `031_debug_cache_inquiries.md` and `034_tavanir_direct_provider.md`; `task_counter.json` (next ID 035; ID 032 is not used).
- **Impact:** No runtime impact: config only, no code reads the keys. Operator decision: qabz implements the Tavanir provider itself and does not call the `tavanir` service (`providers.tavanir.grpc_address` stays and is read by the current adapter only).
- **QA scenarios:** `go test -race -count=1 -run TavanirProviderKeys ./config/` from `src/`. Pass: the test passes (defaults, values, `failure_ratio` 100 valid and 101 rejected, the password does not print). Also: the real `config.yml` parses (checked 2026-11-10 with a scratch test: timeout 2m0s, password masked).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-10] — Task 030: direct Qabzino (Ayantech) adapter, `providers.qabzino.enabled`, QabzinoProxy
- **Type:** Major (new config key, provider adapter replaced, Critical File `system.html`)
- **Files:** `src/provider/qabzino/qabzino.go` + `qabzino_test.go` (new: the direct HTTPS adapter for a water inquiry; token, `water_bill_endpoint`, timeout; `MockFixtures`); `src/provider/qabzinoproxy/` (the former gRPC adapter, renamed with its tests, not used); `src/config/config.go` (`QabzinoEnabled`, default `false`; the comment of `QabzinoGRPCAddress`) + `config_test.go`; `src/app/defaults.go` (registers the direct adapter; `registerAdapter` takes a warn text in place of a no-address flag; `enabled: false` wins over the mock) + `water_test.go`, `electricity_test.go`, `water_live_test.go`; `config.yml.example` (`enabled: false`); `config.yml` (git-ignored; `enabled: true`); `docs/initial-discovery/system.html`, `docs/this_vs_bill.md` (item 26), `docs/folder_structure.md`, `src/app/README.md`; task file `030_qabzino_direct_provider.md`; `task_counter.json` (next ID 031).
- **Impact:** Yes, behavior changes. (1) Qabzino inquiries no longer go through the old qabzino service over gRPC: the App calls Ayantech over HTTPS (the TLS certificate is checked; bill skips it). (2) New key `providers.qabzino.enabled`, default `false`: with `false` every Qabzino inquiry is "provider down" (HTTP 503) and nothing is sent; the dev mock does not apply. A config that relied on `providers.qabzino.grpc_address` or on the mock alone needs `enabled: true`. (3) Water only; `electricity_bill_endpoint` and `gas_bill_endpoint` are still unused. No database change.
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` -> Pending verification -> "Task 030": `make test` (every adapter test uses a local `httptest` server; no call reaches Ayantech) and a feature-ON check with the mock and with `enabled: false`.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-10] — Task 029: Qabzino provider config: token and bill endpoints
- **Type:** Major (new config keys, Critical File `system.html`)
- **Files:** `src/config/config.go` (`QabzinoToken` as `Secret`, `QabzinoElectricityBillEndpoint`, `QabzinoGasBillEndpoint`, `QabzinoWaterBillEndpoint`, the defaults) + `config_test.go` (`TestParseQabzinoProviderKeys`); `config.yml.example` (the four keys with default values); `config.yml` (git-ignored; the real token and URLs); `docs/initial-discovery/system.html`; `docs/Plans/in-progress_tasks/029_qabzino_provider_config.md` (new); `docs/Plans/pending_tasks/task_counter.json` (next ID 030).
- **Impact:** No runtime impact: four new keys under `providers.qabzino`; no code reads them yet. The token is a `Secret` and never prints.
- **QA scenarios:** `go test -race -count=1 -run QabzinoProviderKeys ./config/` from `src/`. Pass: the test passes (defaults, values, token masked).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-10] — Task 022: POST /v1/electricity/inquiry, the Tavanir adapter and its mock
- **Type:** Major (new endpoint, new package `provider/tavanir`, domain logic, new config keys, Critical File `system.html`)
- **Files:** `src/provider/tavanir/tavanir.go` (new: the adapter, `Dial`, `MockFixtures`, the `Mock...` constants) + tests; `src/provider/provider.go` (`BillTypeElectricity`, `Details`, `ElectricityDetails`, `InquiryResponse.Details`); `src/core/inquiry/inquiry.go` (the `tid` prefix of electricity, `Result.Details`, `Row.Details`) + `electricity_test.go`; `src/system/postgres/inquiry_details.go` (new: the `extra_data` JSON of an inquiry) + tests; `src/system/postgres/inquiry_repository.go` (`extra_data` written and read) + unit and live tests; `src/entry/grpc/electricity.go` (new: `Electricity.Inquiry`) + tests; `src/entry/grpc/water.go` (`Inquirer`, `answerInquiryError`: the error answers of every inquiry); `src/entry/http/electricity_test.go`; `src/app/defaults.go` (`registerAdapter`, `newInquiryFlow`, `newElectricity`; `newWater` takes the flow) + `electricity_test.go`, `electricity_live_test.go`, `water_test.go`, `water_live_test.go`; `src/config/config.go` + tests; `Makefile` (`proto` also generates the Tavanir client); `config.yml.example` (and the local `config.yml`); `qabz.postman_collection.json` (new folder Electricity, request Inquiry, with tests for the details); `docs/initial-discovery/system.html` (v1.9); `docs/this_vs_bill.md` (v0.8, items 23-25); `docs/folder_structure.md` (v0.37); `src/app/README.md` (v0.22); `docs/documents_manifest.md`; `docs/Plans/OPEN_TASKS.md`; the task file v0.2 (design decisions, verification record; moved to `done_tasks/`); `src/provider/tavanir/.gitkeep` removed.
- **Impact:** Yes, behavior changes. (1) New endpoint `POST /v1/electricity/inquiry` (`Electricity.Inquiry`): the same flow as water (`core/inquiry`) with the bill type `electricity`: the provider of (`electricity`, `inquiry`) is picked with `ProvidersService` (the seed: Tavanir); valid saved rows of `bill_inquiries` younger than `caching.electricity` answer with no provider call; else one `bill_inquiry_requests` row is written, the Tavanir adapter is called and each inquiry is saved. The answer has the details (`phase`, `companyName`, `tariffType`, `customerType`, `serialNumber`, `serviceAddress`, `servicePostCode`, `customerName`). The error answers are those of water (400, 403, 503, 500). (2) Database: the details are saved in `bill_inquiries.extra_data` as one JSON object: `city` and `region` (as `bill`) and, new, `phase`, `company_name`, `tariff_type`, `customer_type`, `serial_number`, `service_address`, `service_post_code`, `customer_name`. So an answer from saved inquiries has the same details as the provider answer (`bill` answers a cache hit with empty details), and the column now holds the customer name, the address and the post code (`this_vs_bill.md` item 23; awaits operator confirmation). A water row keeps a NULL `extra_data`. (3) New config keys (both config files): `providers.tavanir.grpc_address` (empty), `providers.tavanir.timeout` (`10s`), `caching.electricity` (`1h`), `debug.mocks.tavanir.enable` (`false`), `debug.mocks.tavanir.return_type` (`success`); none is hot-reloadable. A config with no Tavanir address starts with one warn line and every electricity inquiry answers HTTP 503. (4) Metrics: no new metric; `qabz_inquiries_total`, `qabz_provider_calls_total` and `qabz_provider_call_duration_ms` now also have the series `bill_type="electricity"` and `provider="tavanir"`; the three are registered once by the shared flow (the registration error text is now `inquiry: register metric`). (5) Water: no behavior change (the same answers and log lines; `newWater` only gets the shared flow). `POST /v1/electricity/inquiry/document` stays HTTP 501.
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 022 `POST /v1/electricity/inquiry`": unit and live tests (`make test`); feature ON against the compose dev database with `debug.mocks.tavanir.enable: true` (steps 1-4 of the task Verification: success answer with details and rows, second call from the database with the same details and no provider call, `return_type: error`, the saved answer still served while the mock errors). Run by the implementer on 2026-11-10: all pass (evidence in the task file).
- **Author:** Mostafa Talebi / Claude Fable 5.2

## [2026-11-10] — Inquiry tasks 022 to 025; tax, tax-organization and traffic-fine removed from the qabz proto
- **Type:** Major (public API change: `pb/proto/qabz/services.proto`)
- **Files:** `docs/Plans/pending_tasks/022_electricity_inquiry_endpoint.md`, `023_gas_inquiry_endpoint.md`, `024_mobile_inquiry_endpoint.md`, `025_tel_inquiry_endpoint.md` (new tasks, same Definition of Done and Verification as task 021); `docs/Plans/pending_tasks/task_counter.json` (next ID 029; IDs 026 to 028 are not reused); `pb/proto/qabz/services.proto` (`pb` submodule: services `TrafficFine`, `Tax`, `TaxOrganization` and their nine messages removed; `RawOrderRequest.BillType` values `tax`, `taxOrganization`, `trafficFine` removed and numbers 5 to 7 and the three names reserved); `src/api/pb/qabz/` is regenerated (git-ignored). Tasks 026 to 028 were added and removed within the same day, so they have no entry.
- **Impact:** Yes, the API changes. The six endpoints `POST /v1/tax/inquiry`, `/v1/tax/order`, `/v1/tax-organization/inquiry`, `/v1/tax-organization/order`, `/v1/traffic-fine/inquiry` and `/v1/traffic-fine/order` no longer exist in Qabz (the operator: these bill types are not part of the new service). No Go code and no Postman request used them. The new tasks add no runtime behavior.
- **QA scenarios:** none needed for the tasks. For the proto: `make proto` and `cd src && go build ./... && go vet ./... && go test ./entry/...` pass; `services_grpc.pb.go` has no `TrafficFine`, `Tax` or `TaxOrganization`.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-09] — Task 021: POST /v1/water/inquiry, the first provider (Qabzino) and the provider mocks
- **Type:** Major (new endpoint, new packages `core/inquiry` and `provider/qabzino`, domain logic, new config keys, new metrics, Critical File `system.html`)
- **Files:** `src/core/inquiry/inquiry.go` (new: `Service`, the flow) + tests; `src/provider/provider.go` (new: `ProviderInterface` and its types), `registry.go` (new: `ProviderRegistry`), `mock.go` (new: `NewMock`) + tests; `src/provider/qabzino/qabzino.go` (new: the adapter, `Dial`, `MockFixtures`) + tests; `src/system/postgres/inquiry_repository.go` (new) + unit and live tests; `src/entry/grpc/water.go` (new: `Water.Inquiry`) + tests; `src/entry/http/water_test.go`; `src/app/defaults.go` (`newProviderRegistry`, `newWater`, wiring) + `defaults_test.go`, `water_test.go`, `water_live_test.go`; `src/config/config.go`, `constants.go` + tests; `Makefile` (`proto` also generates the Qabzino client); `config.yml.example` (and the local `config.yml`); `qabz.postman_collection.json` (Water / Inquiry); `docs/initial-discovery/system.html` (v1.8); `docs/this_vs_bill.md` (v0.7, items 14-22); `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.18, § 15); `docs/folder_structure.md` (v0.36); `src/app/README.md` (v0.21); `docs/documents_manifest.md`; `docs/Plans/OPEN_TASKS.md`; the task file v0.3 (design decisions, operator confirmation; moved to `done_tasks/`). Postman: the Water / Inquiry test checks that `price` is a number (0 and negative values are valid, operator and PM).
- **Impact:** Yes, behavior changes. (1) New endpoint `POST /v1/water/inquiry` (`Water.Inquiry`): the provider of (`water`, `inquiry`) is picked with `ProvidersService`; valid saved rows of `bill_inquiries` (same `inquiry_id` and provider, younger than `caching.water`, not a copy, not paid; any user's rows) answer with no provider call and a copy is saved for the user (`is_cached = true`); else a `bill_inquiry_requests` row (`initial`) is written, the Qabzino adapter is called, the row gets the status and the trace ID, and each inquiry is saved. Answers: `InvalidArgument` (HTTP 400) for an invalid `billId`; `PermissionDenied` (403) with no user; `Unavailable` (503) with no enabled provider, a failed call or an adapter missing, with one error log line; `FailedPrecondition` (400) with the provider message when it has no inquiry; `Internal` (500) otherwise. The differences from `bill` are items 14-22 of `docs/this_vs_bill.md`. (2) New config keys (both config files): `providers.qabzino.grpc_address` (empty), `providers.qabzino.timeout` (`10s`), `caching.water` (`1h`), `debug.mocks.qabzino.enable` (`false`), `debug.mocks.qabzino.return_type` (`success`); none is hot-reloadable. A config without them starts as before (the defaults apply), but every Qabzino call fails as "provider down" until `providers.qabzino.grpc_address` is set (one warn line at start). (3) Provider mocks (development only, conventions § 15): with `mode: dev` and `debug.mocks.qabzino.enable: true` the Qabzino adapter answers a fixed answer and never calls the backend; in any other mode the key is dropped with the `debug` section. (4) New metrics `qabz_inquiries_total{bill_type,provider,result}`, `qabz_provider_calls_total{provider,action,result}`, `qabz_provider_call_duration_ms{provider,action}` (only with metrics on). (5) `make proto` also generates `src/api/pb/qabzino/` (git-ignored): run it before the first build after the pull. No database migration; no change to the `bill` tables.
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 021 `POST /v1/water/inquiry`": unit and live tests (`make test`); feature ON against the compose dev database with `debug.mocks.qabzino.enable: true` (steps 1-4 of the task Verification: success answer and rows, second call from the database with no provider call, `return_type: error`, the saved answer still served while the mock errors); mock off in another mode.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-09] — Task 020c: a mode other than dev fully removes the debug config section
- **Type:** Major (config behavior in `stage` and `prod`)
- **Files:** `src/config/config.go` (`Parse` removes the top-level `debug` key when `mode` is not `dev`; comments of `Mode`, `DebugAuthDisable`, `Parse`), `src/config/constants.go` (`debugSection`), `src/config/config_test.go` (`TestParseDebugSectionOnlyInDev`), `config.yml.example` (comment of the `debug` section; YAML, no row), `src/app/README.md` (v0.20), `docs/Plans/OPEN_TASKS.md` (v1.41), `docs/documents_manifest.md` (v1.4), `CHANGELOG.md` (this entry); task file `020c_debug_section_dev_only.md` (v0.3; not manifest-tracked). Go files have no VERSION header, so no rows.
- **Task rewrite:** the operator rewrote the task on 2026-11-09 (v0.2): "a mode other than "dev" fully removes debug section of the config", and nothing else. The first text (v0.1: stop the start with an error, a conventions section, the `devdb` and hot-reload points) is replaced; the `devdb` dev-only guard is not changed.
- **Impact:** Yes, behavior changes in `mode: stage` and `mode: prod`: `config.Parse` deletes the whole `debug` section from the YAML tree before it reads the values, so every `debug.*` field keeps its default (`debug.auth.disable` false, so the auth check is on; `debug.auth.mock_user_id` empty). A value in the removed section is not read or checked, so a bad value there does not stop the start. A missing or wrong `mode` also removes the section (the start stops anyway with the mode error). `mode: dev` is unchanged (a null `debug:` in dev is still an error, as before).
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 020c debug section dev only": the unit test (stage and prod with a full, partial, empty, null or bad section equal the config without it; dev reads and checks it); the positive control (`go test -overlay` with the delete taken out: the 8 stage / prod cases with values fail, the empty case passes as it changes nothing); the App binary with `debug.auth.disable: true`: dev gives HTTP 200 with the `auth check disabled` warning, stage and prod give HTTP 403 and no warning. Done by the implementer on 2026-11-09, awaiting operator confirmation.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-09] — Task 020b: tests never touch the development database (one throw-away run database per make test)
- **Type:** Major (test infrastructure: new Make targets, new test tool package, new binding convention § 14; no App code changes)
- **Files:** `Makefile` (`test`, `test-db-clean`, `GO_TEST_ARGS`, `GO_TEST_PKGS`); `src/system/postgres/postgrestest/testdb/main.go` (new) + `main_test.go` (new); `src/system/postgres/postgrestest/postgrestest.go` (`RunDBPrefix`, `RunDSNEnv`, `RunDBName` on `pgconn.ParseConfig`, `RunDSN`, guard in every schema helper) + `postgrestest_test.go`; live tests on `postgrestest.RunDSN`: `src/system/postgres/live_test.go` (now package `postgres_test`, `reset()` removed, each test in its own `EmptySchema`), `provider_live_test.go`, `service_repository_live_test.go`, `savedbill_repository_live_test.go`, `src/system/postgres/devdb/devdb_live_test.go`, `src/entry/http/services_live_test.go`, `src/entry/http/mybills_live_test.go`, `src/app/providers_live_test.go`; `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.17, § 14, § 11), `docs/folder_structure.md` (v0.35), `docs/Plans/OPEN_TASKS.md` (v1.40), `docs/documents_manifest.md` (v1.3), `CHANGELOG.md` (this entry); task file `020b_test_database_isolation.md` (v0.6, operator decisions 1-8 of 2026-11-09, fix rounds 1, 2, 3 and 4; not manifest-tracked). Go files have no VERSION header, so no rows.
- **Impact:** No App runtime impact (no non-test App code changed; no endpoint, config key, metric or log line of the App). Yes, the way to run the tests changes: (1) `make test` runs `go test ./...` (default `-race -count=1 -cover`) in ONE new database `qabz_test_<16 hex>` on the compose Postgres and drops it at the end, also on a test failure, Ctrl-C or SIGTERM; it needs `config.yml`, the compose Postgres and a role with `CREATEDB` (the compose role is the superuser). (2) The live tests read only `QABZ_TEST_RUN_DSN`, which only `make test` sets; `QABZ_TEST_POSTGRES_DSN` is no longer read, so a plain `go test ./...` skips the live tests (as before without a DSN). (3) A guard stops every live test whose run DSN names a database that does not start with `qabz_test_`; it checks the database the driver really connects to (`pgconn.ParseConfig`: a `?dbname=` / `?database=` override or `PGDATABASE` counts, not only the URL path). (4) The task 003 live tests no longer drop `app_hot_configs` / `qabz_schema_migrations` in the DSN database (that dropped the migration state of the development database on 2026-11-09). (5) `make test-db-clean` drops the leftovers of killed runs: every `qabz_test_%` database, then every `qabz_test_` role (fix round 2, decision 7 "Clean roles, random pw"; prefix matched with `starts_with`; a role without the prefix and the admin role are never touched; one line per dropped role and a count). Decision 8 (operator, fix round 3): "Nothing in the databases other than test_* databases must be touched." `testdb` never writes inside a database: no `REASSIGN OWNED` / `DROP OWNED` (the fix round 2 path, which changed owners and grants in a non-test database, is removed); the admin connection only reads catalogs and runs server-level `CREATE` / `DROP DATABASE` and `DROP ROLE` for `qabz_test_` names. A `qabz_test_` role that `DROP ROLE` cannot drop (2BP01: it still has objects or privileges in a database that is not a test database) is KEPT and nothing is changed: `testdb: kept role <name>: it has objects or privileges in <place list>; remove them by hand` (the list read from `pg_shdepend`; a privilege on a database names that database; fix round 4: each place labeled for what it is, `database <name>` with ` (not a test database)` only when the name does not start with `qabz_test_`, `a shared object (<catalog>)` for another shared object), and a final `testdb: <n> test role(s) kept`; then `make test-db-clean` exits 1 (now also said in `make help`). A 2BP01 with no `pg_shdepend` row left has its own line (fix round 4): `testdb: kept role <name>: DROP ROLE reports dependencies but none were found; run make test-db-clean again` (still kept, counted, exit 1). Nothing removes leftovers by itself; it also drops the database of a running `make test`, so run it only when none is running. (6) The test tool `testdb` (`admin-dsn <env file>`, `name`, `create <name>`, `drop <name>`, `clean`) never gets a DSN or password as an argument (the admin DSN is in its environment), parses `config_generated.env` itself (the file is no longer sourced as shell, so a password with `'`, `"`, a backtick or `$` works), and the trap is set before the database is created (a drop before create does nothing). `GO_TEST_PKGS` is operator-approved (decision 6). (7) A test login role (`TestLiveSpecialPassword`) gets a new random password for each run (the special characters plus random hex from `crypto/rand`), so no usable password is in the repository (decision 7). (8) Conventions § 11 measures coverage with `make test`; § 14 lists what a killed run can leave and the `CREATE ROLE` / `DROP ROLE` check. The local `.claude/agents/*.md` files (git-ignored) still name `QABZ_TEST_POSTGRES_DSN` and need an operator update.
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 020b test database isolation": the development database record is the same before and after `make test`; the run database is created and dropped; no `qabz_test_%` database or schema left; 0 skips with `-v`; a real failing run (`GO_TEST_ARGS="-count=1 -run TestLiveConcurrentUp -timeout 1ms" GO_TEST_PKGS=./system/postgres/`: `panic: test timed out`, make exit 2) and a Ctrl-C still drop the database (a Ctrl-C before create: `nothing to drop`); `make test-db-clean` drops 2 leftovers; no password or `postgres://` in the process list during a run; `TestLiveSpecialPassword` passes; killed `TestLiveSpecialPassword` runs (`-timeout 150ms`) leave `qabz_test_` roles and databases, and `make test-db-clean` drops them all (one `testdb: dropped role <name>` line each, 0 left); `TestLiveCleanRoles` (fix round 3) drops a left role whose dependencies were only in a dropped test database, keeps a role with dependencies in a database outside the clean (names it, exit 1, that database's owners, ACLs and `pg_shdepend` rows unchanged), drops it once the dependencies are gone, and keeps a role without the prefix; fix round 4: the kept role also has `CONNECT` on a second `qabz_test_` database of the test outside the clean prefix (`<tag>xdb`), and the kept line names both databases (a shared `pg_database` row names the database it is about); with the `roleDependencies` query changed to `COALESCE(d.datname, current_database())` and no `od` join (a `go test -overlay`), `TestLiveCleanRoles` FAILS (the line names only the run database); `TestRoleVerdict` (pg_shdepend rows → drop / keep with the labeled place list); `TestRoleOutcome` (DROP ROLE result + rows → dropped / error / kept line, including 2BP01 with no rows); the positive control (the old `REASSIGN OWNED` / `DROP OWNED` path in a `go test -overlay`) fails `TestLiveCleanRoles` because the outside database changed; `make test-db-clean` drops a hand-made leftover `qabz_test_` role and its database (exit 0), and the development database record (databases, roles, schemas, relations with owners and ACLs, `pg_shdepend`) is the same before and after `make test` and after `make test-db-clean`; the dev DSN as `QABZ_TEST_RUN_DSN`, and a `qabz_test_` path with `?dbname=qabz` / `?database=qabz`, fail the live tests with `is not a test run database` and change nothing. Done by the implementer on 2026-11-09 (fix round 1 re-run the same day; the first round's `-bogusflag` failure step did not fail and was replaced; fix rounds 2, 3 and 4 re-run the same day), awaiting operator confirmation.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-09] — New tasks: 020b_test_database_isolation, 020c_debug_section_dev_only
- **Type:** Minor (docs only; two new task files)
- **Files:** `docs/Plans/pending_tasks/020b_test_database_isolation.md` (v0.1), `docs/Plans/pending_tasks/020c_debug_section_dev_only.md` (v0.1), `docs/documents_manifest.md` (v1.2), `CHANGELOG.md` (this entry). Letter suffixes on 020, so both tasks sort before 021 and are done next (operator request); the task counter does not change (next ID 022).
- **Impact:** No runtime impact. 020b: tests never touch the development database; each test run creates one random throw-away database (`qabz_test_<random>`), runs in it and drops it at the end; `reset()` of the task 003 live tests no longer drops `app_hot_configs` / `qabz_schema_migrations` in the DSN database; new conventions section. 020c: the `debug` config section is allowed only in `mode: dev`; `stage` / `prod` with a `debug` section stop the start; new conventions section.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-09] — New task: 021_water_inquiry_endpoint
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/021_water_inquiry_endpoint.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 022), `docs/documents_manifest.md` (v1.0), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds `POST /v1/water/inquiry` (database first, Qabzino only when no valid `bill_inquiries` row exists, the answer saved), `ProviderInterface` with Qabzino as its first implementation (config `providers.qabzino`), and the development-only provider mocks `debug.mocks.<provider>` (`enable`, `return_type: success | error`).
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Task 020: ProvidersService, the sorted provider list and PickProvider
- **Type:** Major (new package `provider`, domain logic, new config key, App start order, Critical File `system.html`)
- **Files:** `src/provider/selector.go` (new: `ProvidersService`, `ProviderSelector`, `Provider`, `Repository`) and its tests (`helpers_test.go`, `selector_test.go`, `reload_test.go`, `concurrency_test.go`); `src/system/postgres/provider_repository.go` (new) with `provider_repository_test.go` and `provider_live_test.go`; `src/system/postgres/postgrestest/postgrestest.go` (`SeededSchema`) and its test; `src/config/config.go` (`ProvidersLoadInterval`, check), `src/config/constants.go` (`ProvidersMinLoadInterval`), `src/config/config_test.go`, `src/config/hot_test.go`; `src/app/app.go` (`Providers`, start and stop order), `src/app/builder.go` (`WithProviders`), `src/app/defaults.go` (`newProviders`, 5 metrics), `src/app/defaults_test.go`, `src/app/providers_test.go` (new), `src/app/providers_live_test.go` (new, fix round 3); `config.yml.example` (and the local, git-ignored `config.yml`); `src/app/README.md` (v0.18), `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.16, § 13), `docs/this_vs_bill.md` (v0.6, item 13), `docs/initial-discovery/system.html` (v1.6), `docs/folder_structure.md` (v0.34), `docs/Plans/OPEN_TASKS.md` (v1.38), `docs/documents_manifest.md` (v0.99), `CHANGELOG.md` (this entry); task file `020_providers_sorted_list.md` (not manifest-tracked). Go files have no VERSION header, so no rows.
- **Impact:** Yes, behavior changes. (1) App start now loads the `providers` table of the `bill` database once, after the config reloader and before the gRPC server; a failed first load writes one error line (`providers: load failed; keeping the last list`) and the App still starts with an empty list (operator decision 2). (2) A reload routine loads the table every `providers.general.load_interval` (new key, default `1m`, at least `1s`; a lower value stops the start). Each load replaces the whole list (the database wins) and has a fixed 10 s timeout (not a config key); a failed load keeps the last good list and writes one error line; a load that `Stop` cancels at shutdown is not a failure (no error line, no `error` count), and neither is a first load canceled because the `App.Start` context itself is canceled (fix round 2, 2026-11-09; `Start` returns as before); a load canceled for another reason, or one that times out, is still logged and counted. (3) The key is the first `hot_reload:"true"` field, so the config reloader of task 003 now runs at start: it reads `app_hot_configs` every `hot_reload_interval` (a `providers_load_interval` / `duration` row changes the interval without restart; a hot value under `1s` is used as `1s` with one warn line). (4) After each load, one debug line `providers: group order` per group whose order or status changed. (5) New metrics: `qabz_providers_loads_total{result}`, `qabz_providers_load_duration_ms`, `qabz_providers_last_load_success_timestamp_ms`, `qabz_providers_changes_total{change,result}` and the gauge `qabz_providers_active_count` (no labels: the enabled providers in all groups of the current list, set after each good load and each `Enable` / `Disable` that changed the list; a failed load keeps the value) (only with metrics on). `PickProvider` (first enabled provider by `tmp_priority`, then `id`; 0 allocations) and `MoveUp` / `MoveDown` / `Enable` / `Disable` have no caller yet: no endpoint, no Postman change. New binding convention § 13: providers only through `ProvidersService`. Difference from `bill` (sorted pick, no Redis cache): `this_vs_bill.md` item 13. Fix round 3 (2026-11-09): the `# HELP` text of `qabz_providers_loads_total` and `qabz_providers_load_duration_ms` (and their `system.html` rows) states that an aborted load (canceled by `Stop` or by the `App.Start` context) is neither counted nor timed; no other runtime change (tests only). `providers.general.load_interval` stays hot-reloadable (operator, 2026-11-09), now proved by a live test. Fix round 4 (2026-11-09, operator decision 9, "Apply at once"): yes, behavior changes: a hot change of the effective `providers.general.load_interval` (the value after the 1 s minimum) now applies at once: the reload routine stops the wait in progress and runs the next load at (start of the last load + new interval), at once when that time is past (before: the new value applied only after the wait in progress ended, up to the old interval, for example 1 h). The same effective value (for example `200ms` → `300ms`, both 1 s, or the same value again) restarts nothing and runs no extra load; a load in progress is not interrupted; the normal wait after a load is unchanged. The routine learns of the change from the new `config.Holder.Changed()` (a channel that the next `Set` closes; existing `Get` / `Set` callers unchanged; the config reloader sets the holder only after a reload that changed a value) through the new `provider.Options.Changed`. No metric, log line, endpoint or Postman change; `PickProvider` stays at 0 allocs/op. Files: `src/config/hot.go`, `src/config/hot_test.go`, `src/provider/selector.go`, `src/provider/interval_test.go` (new), `src/app/defaults.go`, `src/app/providers_live_test.go` (starts at 1 h), `src/app/README.md` (v0.19), `docs/initial-discovery/system.html` (v1.7), `docs/Plans/OPEN_TASKS.md` (v1.39), `docs/documents_manifest.md` (v1.1), the task file.
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 020 `ProvidersService`": unit, race, benchmark and live tests; feature ON on the dev database with `logs.level: debug` and a 5 s interval: one order line per group before `grpc started`; an SQL change of `gas`/`inquiry` (priority and status) shows as one new line with the new order and pick within one interval; no line when nothing changes; the restore gives the first order; a database outage gives one error line per failed load and the App keeps serving; `qabz_providers_active_count` is 23 for the seed rows, follows an SQL status change after the next load, and returns to 23 after the restore; a shutdown writes no load error line; a SIGINT during a first load that waits on a held `LOCK TABLE providers` writes no load error line (fix round 2). Fix round 3: the live test `TestLiveProvidersLoadIntervalHotReload` (real migrator, config reloader and `app_hot_configs` row on a throw-away schema: `1s` gives loads every second after the 4 s wait in progress, `200ms` uses 1 s with one warn line); feature ON on the dev database with migrations on: an `app_hot_configs` row `2s` gives about 5 loads in 10 s, `200ms` about 10 loads in 10 s with one warn line, and both `# HELP` texts name the aborted load. Fix round 4: the interval-change unit tests (`IntervalChange*`, `StopDuringWaitAfterChange`, `ConcurrentIntervalChanges`, `HolderChanged*`); the live test from 1 h (the second load within 3 s of the `1s` row); feature ON on the dev database from `1h`: an `app_hot_configs` row `2s` gives a load within about 2 s of `config reloaded`, then one every 2 s.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Session wrap-up (agents and conventions § 12 session)
- **Type:** Minor (docs only)
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.36), `docs/documents_manifest.md` (v0.97), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Two lesson candidates added (one writer per working tree; test each command in an agent file) and more evidence for "Run the `code-reviewer` agent before the task commit". The local agents changed this session (git-ignored, not in a commit): `supervisor` renamed to `task-master`, which now runs `code-reviewer` and the new `artifacts_agent` in parallel; `task_executor` reports its metrics, log lines and Postman changes. The push of 95c29ce..HEAD waits for the operator.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Task 019b: DELETE /v1/mybill/{id} sends a success text on success
- **Type:** Minor (fix-forward of task 019, Rule 3; one answer text, operator decision)
- **Files:** `src/entry/grpc/mybills.go` (`deleteSuccessText`, the success answer), `src/entry/grpc/mybills_test.go`, `src/entry/http/mybills_test.go`, `src/entry/http/mybills_live_test.go`, `qabz.postman_collection.json` (`Delete` description and test), `src/app/README.md` (v0.16), `docs/this_vs_bill.md` (v0.5, item 12), `docs/Plans/OPEN_TASKS.md` (v1.35), `docs/documents_manifest.md` (v0.96), `CHANGELOG.md` (this entry); task file moved to `done_tasks/` (not manifest-tracked). Go files have no VERSION header, so no rows.
- **Impact:** Yes, behavior changes. A successful `DELETE /v1/mybill/{id}` answers HTTP 200 with `{"message":"عملیات با موفقیت انجام شد"}` ("the operation was done successfully") in place of bill's `invalid_parameter_error` text "خطای داخلی رخ داده است", which clients read as a failure (operator decision, 2026-11-08). This applies to each success case of task 019 (a bill of the user, an already disabled bill, an unknown id, a bill of another user). Not changed: HTTP 200, the soft delete, every failure answer (gRPC `Unknown`, HTTP 500, bill's text), the logs and `qabz_mybill_delete_total{type,result}`. A difference from bill: `docs/this_vs_bill.md` item 12, `@todo` at `deleteSuccessText`.
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md` → Pending verification → "Task 019b `DELETE /v1/mybill/{id}` success text": the unit, gateway and live tests check the success text and bill's failure text; feature ON on the dev database: POST a bill, DELETE it, DELETE it again and DELETE an unknown id give HTTP 200 with the success text; `not-a-uuid` gives HTTP 500 `code 2` with bill's text; the row is `disable` after the first DELETE. The Task 019 scenario now expects the new success text.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Conventions § 12: metrics for every feature
- **Type:** Minor (governance doc; no code change)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.15), `docs/documents_manifest.md` (v0.95), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. New binding rule: every added or changed feature is measurable. Each endpoint goes through the `qabz_request_duration_ms` interceptor; a feature whose outcomes the `code` label cannot tell apart has a feature metric that counts each outcome (a plain read with only success or a gRPC error needs none, and the author says so); dependency calls and jobs get a `_ms` histogram and a counter with the names planned in `system.html`. Metrics are registered only when `metrics.Registry()` is not nil, names start with `qabz_`, label values come from a small fixed set (no IDs, amounts or error text), each new metric gets a `system.html` row and tests (registered-names list, a value check per outcome). The current code already follows the rule. The new local `artifacts_agent` agent (git-ignored, with the `task-master` agent that replaces `supervisor`) checks it with § 7 and § 10.
- **QA scenarios:** None needed (Rule 11 exempts governance-doc edits with no behavior impact).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — New task: 019b_mybill_delete_success_text
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/019b_mybill_delete_success_text.md` (v0.1; letter suffix of task 019, so the task counter does not change), `docs/documents_manifest.md` (v0.94), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task changes the success message of `DELETE /v1/mybill/{id}` from bill's "internal error" text to "عملیات با موفقیت انجام شد" (operator decision); failure answers stay as bill's; a new difference from bill (`this_vs_bill.md` item 12).
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Conventions § 11: coverage gate with the database tests
- **Type:** Minor (governance doc; no code change)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.14), `docs/documents_manifest.md` (v0.93), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. New binding rule: each changed Go package has 70.0% statement coverage or more, measured with the database tests included (`QABZ_TEST_POSTGRES_DSN` set, live tests run and do not skip); with no database the gate is "not measured" and the change cannot pass. Before, the 70% gate lived only in the local `code-reviewer` agent and did not say how to measure; a unit-only run failed task 019 (`system/postgres` 67.7% unit-only, 89.0% with its database tests). The local `code-reviewer` agent (git-ignored) checks the rule. No CI job runs Go tests today, so the review is the only place the gate is measured.
- **QA scenarios:** None needed (Rule 11 exempts governance-doc edits with no behavior impact).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Task 019_mybill_delete_endpoint: DELETE /v1/mybill/{id}
- **Type:** Major (new endpoint)
- **Files:** `src/core/savedbill/savedbill.go` (`Store.Delete`, `Repository.Disable`, `StatusDisable`) + test, `src/system/postgres/savedbill_repository.go` (`Disable`, `disableSavedBillSQL`) + unit and live tests, `src/entry/grpc/mybills.go` (`Delete`, `BillDeleter`, `OnDelete`) + test, `src/entry/http/mybills_test.go`, `mybills_live_test.go`, `src/app/defaults.go` (wiring, `qabz_mybill_delete_total`) + test, `src/go.mod` (`github.com/google/uuid` v1.6.0 direct, was indirect), `qabz.postman_collection.json`, `docs/this_vs_bill.md` (v0.4), `docs/folder_structure.md` (v0.33), `src/app/README.md` (v0.15), `docs/Plans/OPEN_TASKS.md` (v1.34), task file moved to `done_tasks/`, `docs/documents_manifest.md` (v0.92), `CHANGELOG.md` (this entry).
- **Impact:** Yes, behavior changes. New endpoint `DELETE /v1/mybill/{id}` (gRPC `MyBills.Delete`), exactly as bill (operator decisions, task 019): the id is checked with `uuid.Parse` of `github.com/google/uuid` v1.6.0 and sent to Postgres as sent; `UPDATE bills SET status = 'disable' WHERE id AND user_id` (no row removed, only `status` changes; `RETURNING type` only reads the type for the metric); an unknown id, another user's bill or an already disabled bill still succeed. Success: HTTP 200 `{"message":"خطای داخلی رخ داده است"}` (bill's text). An invalid or empty id, a call with no user and a database error (also `urn:uuid:` ids, which Postgres rejects): gRPC `Unknown` (HTTP 500) with the same text. Only the database error writes a log line (`mybill: delete failed`). New metric `qabz_mybill_delete_total{type,result}` (`type` = `bills.type` of the bill, `unknown` for `not_found` or a NULL type; `result` = `found` | `not_found`). Difference from bill: `docs/this_vs_bill.md` item 11 (no log line for a call with no user). `POST` / `GET /v1/mybill` and other endpoints unchanged.
- **QA scenarios:** `OPEN_TASKS.md` > Pending verification > Task 019 (⏸; the feature-ON run is done by the implementer; awaiting operator confirmation).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — New task: 020_providers_sorted_list
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/020_providers_sorted_list.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 021), `docs/documents_manifest.md` (v0.91), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds `ProvidersService`: the deterministic sorted provider list per (bill type, action), `PickProvider`, `MoveUp` / `MoveDown`, toggle, the runtime key `tmp_priority`, and a reload every `providers.general.load_interval` in which the database always wins. No endpoint. Unit tests with many fake providers, integration tests on a real Postgres, and a feature-ON run on the dev database.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — New task: 019_mybill_delete_endpoint
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/019_mybill_delete_endpoint.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 020), `docs/documents_manifest.md` (v0.90), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds `DELETE /v1/mybill/{id}`, which must behave exactly like bill (soft delete to `status = 'disable'`), with unit tests, integration tests on a throw-away schema and a feature-ON run on the dev database.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Task 017_mybill_list_endpoint: GET /v1/mybill
- **Type:** Major (new endpoint)
- **Files:** `src/core/savedbill/savedbill.go` (`List`, `IsPinned`, `LastOrderDate`) + test, `src/system/postgres/savedbill_repository.go` (`List`, `billData`) + unit and live tests, `src/entry/grpc/mybills.go` (`GetAll`) + test, `src/entry/http/mybills_test.go`, `mybills_live_test.go`, `src/app/defaults.go` + test, `qabz.postman_collection.json`, `docs/this_vs_bill.md` (v0.3), `docs/folder_structure.md` (v0.32), `src/app/README.md` (v0.14), `docs/Plans/OPEN_TASKS.md` (v1.33), task file moved to `done_tasks/`, `docs/documents_manifest.md` (v0.89), `CHANGELOG.md` (this entry).
- **Impact:** Yes, behavior changes. New endpoint `GET /v1/mybill` (gRPC `MyBills.GetAll`): the enabled bills of the JWT user from bill's `bills` table, newest first, mapped as bill maps them (tel/mobile phone and the duplicate-phone skip, gas participate code, old types as `gas`, `operator` and `isPined` from `json_data` read as bill's `encoding/json` reads it); `serviceId` = `service.id`; no bills: `"data":[]`. A failed read or a bad `json_data` value: HTTP 500 and one `mybill: list failed` log line. Differences from bill: `docs/this_vs_bill.md` items 6, 9, 10. `POST /v1/mybill` and other endpoints unchanged.
- **QA scenarios:** `OPEN_TASKS.md` > Pending verification > Task 017 (⏸; the feature-ON run is done by the implementer and is also the feature-ON check of task 016; awaiting operator confirmation).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Session wrap-up (tasks 015 and 018 session)
- **Type:** Minor (docs only)
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.32), `docs/documents_manifest.md` (v0.88), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Three lesson candidates added (generate a copied schema with the source's own tool; one minimal test config helper; push needs the operator) and more evidence for "Shared local runtime". The push of 323c7a4..f81f4b1 was blocked by the agent's permission check and waits for the operator.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Task 018_make_help: make help
- **Type:** Minor (a new Makefile target that only prints; no change to the other targets)
- **Files:** `Makefile`, `docs/folder_structure.md` (v0.31), `docs/Plans/OPEN_TASKS.md` (v1.31), task file moved to `done_tasks/`, `docs/documents_manifest.md` (v0.87), `CHANGELOG.md` (this entry).
- **Impact:** Yes, a new command: `make help` prints a usage line and each target with its description, read from the `## target:` comments of the Makefile (variables such as `$(CONFIG)` show their values). No other target changes; `make` alone still runs `venv`.
- **QA scenarios:** `OPEN_TASKS.md` > Pending verification > Task 018 (⏸; done by the implementer, awaiting operator confirmation).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — New task: 018_make_help
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/018_make_help.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 019), `docs/documents_manifest.md` (v0.86), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds a `make help` target that lists the Makefile targets.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Task 015_dev_bill_database: devdb seed / unseed and the mode key
- **Type:** Major (new command, new packages, new required config key)
- **Files:** `src/config/config.go`, `constants.go` + tests (`config_test.go`, `example_test.go`, `boot_test.go`), `config.yml.example`, `src/cmd/devdb/` (new) + test, `src/cmd/run.go` + test, `src/system/postgres/devdb/` (new: `devdb.go`, `schema.sql`, `seed_services.sql`, `seed_providers.sql`, `unseed_services.sql`, `unseed_providers.sql`, unit and live tests), `src/system/postgres/postgrestest/` (`EmptySchema`), the config YAML in `src/app/`, `src/cmd/migrate/`, `src/cmd/start-app/` and `src/entry/grpc/` tests, `docs/folder_structure.md` (v0.30), `src/app/README.md` (v0.13), `docs/Plans/OPEN_TASKS.md` (v1.30), task file moved to `done_tasks/`, `docs/documents_manifest.md` (v0.85), `CHANGELOG.md` (this entry).
- **Impact:** Yes, behavior changes. New required config key `mode` (`dev`, `stage`, `prod`): a config without it, or with another value, no longer loads, so every existing `config.yml` must add it (breaking for local configs; the local `config.yml` has `mode: "dev"`). `mode` changes nothing else in the App. New development command `devdb --config=... seed | unseed`: `seed` creates the 10 tables of the `bill` database as bill's code makes them (no `outboxes`: bill's code does not create it) and inserts the 5 `services` and 45 `providers` seed rows; `unseed` deletes those rows by `id`. One transaction each; a second run changes nothing. With `mode` `stage` or `prod` both exit 1 before they connect. Not a qabz migration; `src/migrations/` is unchanged.
- **QA scenarios:** `OPEN_TASKS.md` > Pending verification > Task 015 (⏸; done by the implementer on the compose dev database, awaiting operator confirmation).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Session wrap-up: task 016 follow-up and lesson candidates (task 016 session)
- **Type:** Minor (docs only)
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.29), `docs/documents_manifest.md` (v0.84), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. A task 016 follow-up for task 017 (real `bills` data: one row with lower-case `json_data` keys, `data` empty in most old rows) and three lesson candidates for an operator decision: shared local runtime (do not stop or collide with the operator's services), commit a task file when it is created, a dev switch that removes a check must supply what the check provided.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Conventions § 10: logging of fatal and critical errors
- **Type:** Minor (governance doc; no code change)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.13), `docs/Plans/OPEN_TASKS.md` (v1.28), `docs/documents_manifest.md` (v0.83), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. New binding rule: every fatal or critical error that is not a common error (failed dependency call, unexpected fault, start or process failure, failed background job) MUST write one `error` log line with the cause; common errors (client errors, canceled calls) are not logged. Every server-fault answer needs a test of its log line. The current code already follows the rule (both `Internal` answers log the cause; start failures go to stderr). The lesson candidate "Logging of dependency failures" is codified and removed from OPEN_TASKS. The local `code-reviewer` agent (git-ignored) checks the rule.
- **QA scenarios:** None needed (Rule 11 exempts governance-doc edits with no behavior impact).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Task 016_mybill_insert_endpoint: POST /v1/mybill
- **Type:** Major (new endpoint, new packages, new config key)
- **Files:** `src/common/helpers/` (new: `billid.go`, `phone.go`, tests), `src/core/savedbill/` (new), `src/system/postgres/savedbill_repository.go` (new) + unit and live tests, `src/system/postgres/postgrestest/` (`BillsSchema`), `src/entry/grpc/mybills.go` (new) + test, `src/entry/grpc/auth.go` (`MockUserID`) + test, `src/entry/http/mybills_test.go`, `mybills_live_test.go` (new), `src/app/defaults.go` + test, `src/config/config.go` + test, `config.yml.example`, `qabz.postman_collection.json`, `docs/this_vs_bill.md` (v0.2), `docs/folder_structure.md` (v0.29), `src/app/README.md` (v0.12), `docs/Plans/OPEN_TASKS.md` (v1.27), task file moved to `done_tasks/`, `docs/documents_manifest.md` (v0.82), `CHANGELOG.md` (this entry).
- **Impact:** Yes, behavior changes. New endpoint `POST /v1/mybill` (gRPC `MyBills.Insert`): saves a bill for the JWT user in bill's `bills` table with bill's rules (type, bill ID check digit, 0/98 phone normalization, default name, `mci` for mobile); a bill the user already has gives the existing id. Invalid request: HTTP 400; failed write: HTTP 500 and one `mybill: insert failed` log line. New metric `qabz_mybill_insert_total{result}`. New config key `debug.auth.mock_user_id` (development only): with `debug.auth.disable: true`, every call gets this fake user. Differences from bill: `docs/this_vs_bill.md` items 5-8. Other endpoints unchanged.
- **QA scenarios:** `OPEN_TASKS.md` > Pending verification > Task 016 (⏸; the feature-ON check waits for task 017 by operator decision).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — New task: 017_mybill_list_endpoint
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/017_mybill_list_endpoint.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 018), `docs/documents_manifest.md` (v0.81), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds `GET /v1/mybill` (the saved bills of the JWT user), with unit tests on mocks and fakes and database tests. It is the GET counterpart of task 016: the two endpoints verify each other.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — New task: 016_mybill_insert_endpoint
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/016_mybill_insert_endpoint.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 017), `docs/documents_manifest.md` (v0.80), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds `POST /v1/mybill` (save a bill for the JWT user in bill's `bills` table), with unit tests on mocks and fakes and database tests that check each row by a direct SQL query and a repository lookup. Its feature-ON verification waits for the GET counterpart.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-08] — Session wrap-up: lesson candidates (task 014 review session)
- **Type:** Minor (docs only)
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.26), `docs/documents_manifest.md` (v0.79), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Three lesson candidates queued for an operator decision: how a test helper fails (conventions § 3), the `code-reviewer` agent before the task commit, and a positive-control recurrence detail (L082).
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Task 014 review fixes: two test files
- **Type:** Minor (tests only; no runtime code)
- **Files:** `src/common/monitoring/prometheus/requests_test.go`, `src/config/config_test.go`, `docs/documents_manifest.md` (v0.78), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The `code-reviewer` agent found these items in commit 3138bf1 (already pushed, so the fix is a new commit, Rule 3). (1) The test helper `family` checked "the metric is registered" with a manual `t.Fatalf`; it now uses `assert.Failf` and then `t.FailNow()` (conventions § 3). (2) The doc comment of `TestParseAuthKeys` was above `TestParseMetricsSwitch`; it is back above its own test.
- **QA scenarios:** (1) In `src/` `go vet ./...`, `go build ./...`, `go test -race -count=1 ./common/monitoring/prometheus/ ./config/`: all pass (done, 2026-11-07). (2) Deliberate failure: a `go test -overlay` copy of the test that asks `family` for a missing name gives `FAIL` with "metric family not found" and no panic (done, 2026-11-07).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Session wrap-up: lesson candidates (task 014 session)
- **Type:** Minor (docs only)
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.25), `docs/documents_manifest.md` (v0.77), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Two lesson candidates queued for an operator decision: move the Implementation_Plan Step 5 policies into the conventions doc, and a recurrence of L084 (a gate read after the guarded action).
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Task 014_request_metrics_middleware: request metrics and the Prometheus switch
- **Type:** Major (new interceptor, new metrics, new config key; Critical File `system.html`)
- **Files:** `src/common/monitoring/prometheus/` (`prometheus.go`, `prometheus_test.go`, `requests.go` and `requests_test.go` new), `src/entry/grpc/` (`requests.go`, `requests_test.go` new), `src/entry/http/requests_test.go` (new), `src/config/` (`config.go`, `config_test.go`), `src/app/` (`app.go`, `app_test.go`, `defaults.go`, `defaults_test.go`, `README.md` v0.11), `src/go.mod` (`prometheus/client_model` now direct, same version), `config.yml.example`, `docs/initial-discovery/system.html` (v1.4, Critical File), `docs/folder_structure.md` (v0.28), `docs/Plans/OPEN_TASKS.md` (v1.24), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.76), `CHANGELOG.md` (this entry).
- **Impact:** Yes. Every gRPC call, and so every gateway route, records its duration in the histogram `qabz_request_duration_ms{path,code}` (milliseconds, buckets 5 to 10000, conventions § 9); its `_count` series is the request counter, so there is no separate `qabz_requests_total`. `path` is the gRPC method, never the raw URL; `code` is the gRPC code name. The interceptor runs before the auth check, so a rejected call (403) is counted; an unknown HTTP route (404 in the gateway) is not. New config key `metrics.prometheus.enabled` (default `true`): with `false`, no `qabz` metric is created or registered (requests, `qabz_auth_rejected_total`, `qabz_services_cache_total`), nothing is recorded, the metrics endpoint does not listen, and the start log says `metrics disabled`. `prometheus.New` now takes `enabled`. A local `config.yml` must add `metrics.prometheus.enabled` (the sync test fails otherwise). No API change, no migration.
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md`, "Task 014 request metrics and Prometheus switch" (⏸, implementer run passed).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Conventions § 9: durations in milliseconds
- **Type:** Minor (a new project convention; a Critical File doc edit with no code change)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.12), `docs/initial-discovery/system.html` (v1.3, Critical File), `docs/documents_manifest.md` (v0.75), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact (no metric records a duration yet). New rule: every recorded duration (metric, log field) is in milliseconds, recorded as a float (`float64(d) / float64(time.Millisecond)`, not the integer `d.Milliseconds()`), and duration metric names end with `_ms`. Config durations and the default `go_*` / `process_*` metrics are not covered. This deviates from the Prometheus base-unit naming (`_seconds`) by operator decision. `system.html`: the rule "times are in seconds" is replaced, and the planned metrics are renamed (`qabz_order_complete_duration_ms`, `qabz_provider_call_duration_ms`, `qabz_cron_duration_ms`, `qabz_oldest_pending_order_age_ms`, `qabz_dependency_check_duration_ms`, `qabz_db_query_duration_ms`). The `code-reviewer` agent (local, git-ignored) enforces the rule.
- **QA scenarios:** None needed (Rule 11 exempts docs; task 014 applies the rule to the first duration metric).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Session wrap-up: lesson candidates
- **Type:** Minor (docs only)
- **Files:** `docs/Plans/OPEN_TASKS.md` (v1.23), `docs/documents_manifest.md` (v0.74), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Two lesson candidates queued for an operator decision: error logs for dependency failures in a request path, and the `docs/this_vs_bill.md` + `@todo` rule.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — New task: 015_dev_bill_database
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/015_dev_bill_database.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 016), `docs/documents_manifest.md` (v0.73), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds dev-only commands that create the full `bill` database schema, seed `services` (5 rows) and `providers` (45 rows), and unseed them, plus a config key `mode` (`dev`, `stage`, `prod`) that blocks the commands outside `dev`.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — New task: 014_request_metrics_middleware
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/014_request_metrics_middleware.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 015), `docs/documents_manifest.md` (v0.72), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds the request duration middleware of Implementation_Plan Step 6 (it does not exist yet) with a Prometheus counter of each incoming request, labeled with the request path.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Task 013_services_endpoint: GET /v1/services
- **Type:** Major (a new endpoint, a new config key, new packages)
- **Files:** `src/config/` (`constants.go` new, `config.go`, `config_test.go`), `src/core/billtype/` (`services.go`, `services_test.go` new; `.gitkeep` removed), `src/system/postgres/` (`service_repository.go`, `service_repository_live_test.go`, `postgrestest/postgrestest.go`, `postgrestest/postgrestest_test.go` new), `src/entry/grpc/` (`service.go`, `service_test.go` new, `server_test.go`, `auth_test.go`), `src/entry/http/` (`services_test.go`, `services_live_test.go` new, `gateway_test.go`, `auth_test.go`), `src/app/` (`defaults.go`, `defaults_test.go`, `README.md` v0.10), `config.yml.example`, `qabz.postman_collection.json`, `docs/this_vs_bill.md` (new, v0.1), `docs/folder_structure.md` (v0.27), `docs/Plans/OPEN_TASKS.md` (v1.22), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.71), `CHANGELOG.md` (this entry).
- **Impact:** Yes. `GET /v1/services` (gRPC `Service.Get`) was HTTP 501; now it returns the service list of the `services` table (bill database) for the `Accept-Language` language, by `order`, cached in memory per language for `CacheServicesTTL` (60s). New config key `service.id` (default 0) is sent as `serviceId`. New metric `qabz_services_cache_total{result}`; a failed read writes one error log line with the cause (the client gets no detail). Difference from `bill`: `inquiry` is the stored value, not always `true` (`docs/this_vs_bill.md` items 1-4: `inquiry`, error code, empty header, cache). `NewService` now takes `ServiceDeps`. No migration.
- **QA scenarios:** `docs/Plans/OPEN_TASKS.md`, "Task 013 `GET /v1/services`" (⏸, implementer run passed).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — New task: 013_services_endpoint
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/013_services_endpoint.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 014), `docs/documents_manifest.md` (v0.70), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds `GET /v1/services`: the `services` table of the `bill` database through a repository interface, cached in memory for `CacheServicesTTL` (default 60s).
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Task 012_auth_status_403: a rejected auth check gives HTTP 403
- **Type:** Major (API behavior: status code)
- **Files:** `src/entry/grpc/auth.go`, `src/entry/grpc/auth_test.go`, `src/entry/http/auth_test.go`, `src/app/README.md` (v0.9), `qabz.postman_collection.json`, `docs/Plans/OPEN_TASKS.md` (v1.21), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.69), `CHANGELOG.md` (this entry).
- **Impact:** Yes. A call that fails the JWT check now gets gRPC `PermissionDenied` (code 7) and, through the gateway, HTTP 403, in place of `Unauthenticated` (code 16) / HTTP 401. The message stays "unauthenticated"; the check, the metric and `debug.auth.disable` are unchanged. A client that looks for 401 to refresh its token must look for 403.
- **QA scenarios:** (1) `go build`, `go vet`, `go test -race -count=1 ./...` pass (done, 2026-11-07). (2) Deliberate failure (the old code) fails `entry/grpc` and `entry/http` (done, 2026-11-07). (3) Feature ON: no token, bad token, another secret 403; valid token 200 (done, 2026-11-07). (4) The operator calls greeting with a real token and with no token (pending).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — New task: 012_auth_status_403
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/012_auth_status_403.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 013), `docs/documents_manifest.md` (v0.68), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task changes the answer of a rejected auth check from gRPC `Unauthenticated` / HTTP 401 to gRPC `PermissionDenied` / HTTP 403; the message, the check and the metric stay.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Task 011_jwt_no_audience_check: aud is not compared
- **Type:** Major (auth behavior and a config key removed)
- **Files:** `src/common/auth/` (`jwt.go`, `jwt_test.go`, `authtest/authtest.go`, `authtest/authtest_test.go`), `src/config/` (`config.go`, `config_test.go`), `src/app/` (`defaults.go`, `defaults_test.go`, `README.md` v0.8), `config.yml.example`, `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.11), `docs/Plans/OPEN_TASKS.md` (v1.20), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.67), `CHANGELOG.md` (this entry).
- **Impact:** Yes. `aud` is no longer compared with a value, as in `bill`: any `aud` string (also empty) is accepted and goes into `Claims.Audience`; a missing `aud`, an array or a number still gets HTTP 401. `auth.jwt.audience` is removed: with the check on, only `auth.secrets.hmac` is needed. Risk accepted by the operator: a token that the auth service issued for another service also works on Qabz. A local `config.yml` must drop `auth.jwt.audience` (the sync test fails otherwise).
- **QA scenarios:** (1) `go build`, `go vet`, `go test -race -count=1 ./...` pass (done, 2026-11-07). (2) Deliberate failure (no `aud` check) fails 3 table cases (done, 2026-11-07). (3) Feature ON with a test secret: `aud` "qabz", "bill", "" 200; no `aud`, array, number 401; metric `audience` 3 (done, 2026-11-07). (4) The operator calls greeting with a real token from the auth service (pending).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — New task: 011_jwt_no_audience_check
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/011_jwt_no_audience_check.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 012), `docs/documents_manifest.md` (v0.66), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task removes `auth.jwt.audience` and the `aud` comparison, as `bill` does: `aud` must still be a string, any value is accepted, and it goes into the claims. It records the accepted risk (a token issued for another service also works on Qabz).
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Conventions § 8: config.yml and config.yml.example
- **Type:** Minor (a new project convention; a new tracked template file; new tests; no runtime change)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.10), `config.yml.example` (new), `src/config/example_test.go` (new), `src/app/README.md` (v0.7), `docs/folder_structure.md` (v0.26), `docs/documents_manifest.md` (v0.65), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Every config key is now in both `config.yml` (local, git-ignored, real values) and `config.yml.example` (committed, default or zero values, no secret); a config change updates both in the same change. Auth secrets sit under `auth.secrets`. New tests fail when the example and the `Config` path tags differ, when an example value is not the default or zero value, or when the local `config.yml` has other keys than the example (skipped without a local `config.yml`). The local `config.yml` (not committed) got the missing keys.
- **QA scenarios:** (1) In `src/` run `go test -count=1 -v -run 'Example|LocalConfig|LeafPaths' ./config/`. Pass: the 4 tests pass (done, 2026-11-07). (2) Positive controls: a key removed from the example, a non-default value, a password in the example, and a key removed from `config.yml` each fail a test (done, 2026-11-07).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Task 010_jwt_hmac_secret: JWT check with an HMAC secret
- **Type:** Major (auth behavior and a config key change)
- **Files:** `src/common/auth/` (`jwt.go`, `jwt_test.go`, `authtest/authtest.go`, `authtest/authtest_test.go`), `src/config/` (`config.go`, `config_test.go`), `src/app/` (`defaults.go`, `defaults_test.go`, `README.md` v0.6), `src/entry/grpc/auth_test.go`, `src/entry/http/auth_test.go`, `docs/folder_structure.md` (v0.25), `docs/Plans/OPEN_TASKS.md` (v1.19), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.64), `CHANGELOG.md` (this entry).
- **Impact:** Yes. The JWT must be signed with HMAC (`HS256`, `HS384` or `HS512`) and the secret in the new key `auth.secrets.hmac`; ES512 tokens and every other `alg` get HTTP 401. `auth.jwt.public_key_path` is removed: with the check on, a config without `auth.jwt.hmac_secret` (or `auth.jwt.audience`) does not start (exit 1). The other checks, the metric and `debug.auth.disable` are unchanged. The task 009 decision "Qabz never signs a token" no longer holds: the shared secret could sign one; Qabz still only verifies.
- **QA scenarios:** (1) `go build`, `go vet`, `go test -race -count=1 ./...` pass (done, 2026-11-07). (2) Deliberate failures (no HMAC compare, any alg accepted) fail the tests (done, 2026-11-07). (3) Feature ON with a test secret: no token 401, HS256/HS384/HS512 200, another secret / RS256 / expired / guest 401, metric counts, no secret in the log, no secret → exit 1 (done, 2026-11-07). (4) The operator calls greeting with a real token from the auth service (pending).
- **Open:** the HMAC variant and the `aud` value need confirmation from the auth team; the current secret has 20 bytes, under the 32 bytes RFC 7518 § 3.2 asks for HS256.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — New task: 010_jwt_hmac_secret
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/010_jwt_hmac_secret.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 011), `docs/documents_manifest.md` (v0.63), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task changes the JWT check of task 009 from ES512 with a public key file to HMAC only (`HS256`, `HS384`, `HS512`) with one secret, `auth.jwt.hmac_secret`, as the auth service and `bill` use. Every other algorithm is rejected; the other checks stay. It records that this reverses the task 009 decision "Qabz never signs a token".
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Task 009_auth_middleware: JWT auth check on every endpoint
- **Type:** Major (new package `common/auth`; new interceptor; new config keys; every endpoint now needs a token)
- **Files:** `src/common/auth/` (new: `jwt.go`, `context.go`, `jwt_test.go`, `authtest/authtest.go`, `authtest/authtest_test.go`), `src/entry/grpc/` (`auth.go` new, `auth_test.go` new, `server.go`), `src/entry/http/auth_test.go` (new), `src/config/config.go`, `src/app/` (`defaults.go`, `defaults_test.go`, `README.md` v0.5), `src/go.mod`, `src/go.sum` (`github.com/valyala/fastjson` v1.6.10; the `sub` UUID check uses the standard-library `uuid` package, so `github.com/google/uuid` stays indirect), `qabz.postman_collection.json`, `docs/folder_structure.md` (v0.24), `docs/Plans/OPEN_TASKS.md` (v1.18), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.62), `CHANGELOG.md` (this entry).
- **Impact:** Yes. Every gRPC call and every gateway route now needs `Authorization: Bearer <JWT>`; otherwise gRPC `Unauthenticated` / HTTP 401 with the message "unauthenticated". The token must be ES512, signed by the key in `auth.jwt.public_key_path` (PEM, read at start), with `exp` in the future, `nbf` not in the future, `aud` containing `auth.jwt.audience`, `sub` a UUID, `mobile`, and `non_otp` not `guest`. Qabz never signs a token. With the check on, the start fails (exit 1) when the key file or the audience is missing. `debug.auth.disable: true` turns the check off and logs one warning; a missing `debug` section keeps the check on. New metric `qabz_auth_rejected_total{reason}`; no log line per rejection. The metrics endpoint is not checked. A local config without the new keys must add `debug.auth.disable: true` (development) or the two `auth.jwt` keys. The Postman collection has a `{{token}}` variable and bearer auth.
- **QA scenarios:** (1) `go build`, `go vet`, `go test -race -count=1 ./...` pass (done, 2026-11-07). (2) Deliberate failures (any `alg`, no `exp` check, guests allowed, `disable` ignored both ways, no `aud` check) each fail the tests (done, 2026-11-07). (3) Feature ON with a scratch key: no token 401, valid token 200, bad token 401, metric counts, `disable: true` 200 with the warning, no key → exit 1 (done, 2026-11-07). (4) The operator calls greeting with a real token (pending).
- **Open:** ES512 and the `aud` value need confirmation from the auth team; `system.html` plans etcd keys with reload (follow-up, Critical File).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — New task: 009_auth_middleware
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/009_auth_middleware.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 010), `docs/documents_manifest.md` (v0.61), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task adds a JWT auth middleware for every endpoint (401 on an empty, expired or wrong token) and a development-only config key `debug.auth.disable`. It records the design points, including the default: in the code the `bool` zero value (auth on when the key is missing); the development `config.yml` sets `disable: true`. It also defines the test matrix: four config entries times the JWT inputs (missing, malformed, expired, wrong key or algorithm, bad claims, valid), on gRPC and through the HTTP gateway.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Conventions § 7: Postman collection
- **Type:** Minor (a new project convention; a new tool file with no runtime effect)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.9), `qabz.postman_collection.json` (new), `docs/folder_structure.md` (v0.23), `docs/documents_manifest.md` (v0.60), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. New rule: the Postman collection `qabz.postman_collection.json` (Postman Collection v2.1) lives in the repo root, and every HTTP API change (endpoint, method, path, request or response field, header, status code, default HTTP address) updates it in the same commit. It has one request per implemented endpoint, uses `{{baseUrl}}` (default `http://localhost:8080`), and holds no secret. The `code-reviewer` agent rejects an API change without a collection update. The collection starts with `Service / Greeting` (`GET /v1/greeting`) and two Postman tests (status 200, `message` is `Hello`).
- **QA scenarios:** None needed for the rule (Rule 11 exempts docs). Checked on 2026-11-07: `python3 -m json.tool qabz.postman_collection.json` passes; `curl http://localhost:8080/v1/greeting` on the running App gives `{"message":"Hello"}` 200, the result the collection request expects. Importing the collection into Postman and running it is for the operator.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Task 008_logs_to_stdout: App log lines go to stdout
- **Type:** Major (runtime behavior: the output stream of the App logs)
- **Files:** `src/common/logging/zap/zap.go`, `src/common/logging/zap/zap_test.go`, `docs/Plans/OPEN_TASKS.md` (v1.17), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.59), `CHANGELOG.md` (this entry).
- **Impact:** Yes. Every App log line (all levels, same JSON) now goes to stdout in place of stderr, as in the `traffick` service. zap's own internal errors stay on stderr. Process errors that are not log lines (`app: ...` from `start-app`, the `migrate` errors and usage) stay on stderr; `migrate` does not use the logger, so `migrate version` keeps a clean stdout. A tool that reads the App logs from stderr only must read stdout now.
- **QA scenarios:** (1) `go build`, `go vet`, `go test -race -count=1 ./...` pass; the new `TestNewWritesToStdout` fails on a deliberate stderr build (done, 2026-11-07). (2) Feature ON: the App on free ports wrote 8 log lines to stdout and 0 bytes to stderr; `GET /v1/greeting` 200 (done, 2026-11-07). (3) The operator checks the VSCode Debug Console colors (pending).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — New task: 008_logs_to_stdout
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/008_logs_to_stdout.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 009), `docs/documents_manifest.md` (v0.58), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task sends the App log lines to stdout, as the `traffick` service does.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — Task 007_make_compose_actions: `make compose up|down|ps|logs`
- **Type:** Minor (developer tooling; no App code, no config key)
- **Files:** `Makefile`, `docker-compose.yml` (comment), `scripts/yml_to_env/yml_to_env.py` (v0.2, header text), `docs/Plans/Implementation_Plan.md` (v0.12), `docs/Plans/OPEN_TASKS.md` (v1.16), `docs/folder_structure.md` (v0.22), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.57), `CHANGELOG.md` (this entry).
- **Impact:** Yes, for developers only. `make compose` alone is removed: it prints the usage and exits 2. `make compose up|down|ps|logs` generate `config_generated.env` from `config.yml`, then run `docker compose --env-file config_generated.env <action> $(ARGS)`; if the generation fails, docker does not run. `ARGS` passes flags (`make compose up ARGS="-d --wait"`, `make compose down ARGS=-v`). An unknown action, more than one action, or an action word alone (`make up`) prints the usage and exits 2. The generated file header now says `make env` in place of `make compose`. No App runtime impact.
- **QA scenarios:** (1) `make compose up ARGS="-d --wait"`: `postgres` and `redis` `Healthy`; `make compose ps` lists both; `make compose logs ARGS=--tail=3` prints both logs; `make compose down` removes the containers and keeps the volumes (done, 2026-11-07). (2) `make compose`, `make compose foo`, `make compose up down`, `make up`: usage, exit 2; `make compose ps CONFIG=nope.yml` stops at the generator (done, 2026-11-07). (3) `make test-scripts`: 18 tests pass (done, 2026-11-07). Operator confirmation pending in `OPEN_TASKS.md`.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-07] — New task: 007_make_compose_actions
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/007_make_compose_actions.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 008), `docs/documents_manifest.md` (v0.56), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task replaces `make compose` with `make compose up|down|ps|logs`.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-04] — Task 006_greeting_ping: gRPC server, HTTP gateway and Greeting
- **Type:** Major (new packages `entry/grpc` and `entry/http`; new config keys; new public endpoint; `pb` submodule pointer)
- **Files:** `pb` (submodule pointer to `6f38ff16` on branch `qabz_service`: `Greeting` in `proto/qabz/services.proto`), `src/entry/grpc/` (new: `server.go`, `service.go`, `server_test.go`; `.gitkeep` removed), `src/entry/http/` (new: `gateway.go`, `gateway_test.go`; `.gitkeep` removed), `src/app/` (`app.go`, `builder.go`, `defaults.go`, `app_test.go`, `defaults_test.go`, `README.md` v0.4), `src/config/` (`config.go`, `config_test.go`), `docs/folder_structure.md` (v0.21), `docs/Plans/OPEN_TASKS.md` (v1.15), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.55), `CHANGELOG.md` (this entry).
- **Impact:** Yes. The App now starts a gRPC server (`server.grpc.addr`, default `:8082`) and a grpc-gateway HTTP server (`server.http.addr`, default `:8080`) after the config reloader and before the metrics endpoint; `Stop` stops them in reverse order, and a failed `Start` stops only the parts that started. The gateway dials the local gRPC server (a listen address with no host or an unspecified IP, also `[::]`, becomes `127.0.0.1`, so it works where IPv6 is off). `haf80.api.bill.Service/Greeting` (`GET /v1/greeting`) returns `{"message":"Hello"}`; the other `Service` methods return Unimplemented (HTTP 501). Config rules: each address is `host:port` with a port from 0 to 65535 (0 picks a free port), and the two addresses must differ. The ports 8080 and 8082 must be free where the App runs.
- **QA scenarios:** (1) In `src/` `go build ./... && go vet ./... && go test -race -count=1 ./...`: all pass; coverage `app` 90.0%, `config` 96.6%, `entry/grpc` 95.1%, `entry/http` 88.4% (done, 2026-11-04). The JSON read in `gateway_test.go` uses `encoding/json/v2`; it is test code, not a hot path (conventions § 1). (2) Feature ON: the App with `--config=../config.yml` against the compose Postgres and Redis; `curl -i http://127.0.0.1:8080/v1/greeting` gave `HTTP/1.1 200 OK` and `{"message":"Hello"}`; `/v1/services` gave 501; the log shows `grpc started`, `http started`, `app started`, `app stopping` and no password (done, 2026-11-04). (3) The same call from Postman (pending, operator). The push of `pb` commit `6f38ff16` to `origin/qabz_service` failed on 2026-11-04 (`p-source.780.ir:443` not reachable); push `pb` before this repository.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-04] — Conventions § 6: `pb` submodule ownership
- **Type:** Minor (docs only; a new project convention)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.8), `docs/documents_manifest.md` (v0.54), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Only `pb/proto/qabz/` is writable by this repo. Every other path in the `pb` submodule (other `pb/proto/*` folders, `pb/aux/`, `pb/testdata/`, the root files) is read-only: no edit, no delete, no tool output, and a `pb` commit has `proto/qabz/` paths only. Reading for `protoc` imports stays allowed. The section gives a check command; the `code-reviewer` agent runs it.
- **QA scenarios:** None needed (Rule 11 exempts pure docs). The check command was tested on 2026-11-04: on the clean submodule it prints nothing; on a scratch clone with an edit in `proto/bill/` and new files in `aux/` and `proto/qabz/`, it prints the `proto/bill/` and `aux/` paths only.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-04] — New task: 006_greeting_ping
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/006_greeting_ping.md` (v0.1), `docs/Plans/pending_tasks/task_counter.json` (next ID 007), `docs/documents_manifest.md` (v0.53), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 006 adds a `Greeting` method (`GreetingRequest`, `GreetingResponse`) to the `qabz` proto to ping the service; a call to `.../greeting` returns "Hello" with HTTP 200. Design points recorded: the proto is in the `pb` submodule (separate repository), which service gets the method, the response field, the HTTP path, and the missing gRPC / gateway server in the App.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-04] — PostgresDSN uses net/url
- **Type:** Minor (refactor to the standard library, conventions § 5; the decoded DSN is the same)
- **Files:** `src/config/config.go`, `src/config/config_test.go`, `docs/Plans/OPEN_TASKS.md` (v1.14), `docs/documents_manifest.md` (v0.52), `CHANGELOG.md` (this entry).
- **Impact:** Small. `PostgresDSN` builds a `url.URL` (`url.UserPassword`, and `url.PathEscape` for the database name); the hand-written `urlEscape` is removed. The values that pgx decodes do not change. The DSN text can change: `url.UserPassword` does not escape the legal characters `$ & + , ; =` in the user and password (before, they became `%24`, `%26` and so on). A `/` in the database name stays escaped as `%2F`.
- **QA scenarios:** (1) In `src/` `go build ./... && go vet ./... && go test -race -count=1 ./...`: all pass; `config` coverage 96.5% (done, 2026-11-04). (2) `TestPostgresDSNEscapes` (password, user, db cases) and `TestPostgresDSNPgxParses` (pgx decodes user, password and db back) pass (done, 2026-11-04). (3) Deliberate failure: a `-overlay` copy of `config.go` without `RawPath` fails the `db` case (done, 2026-11-04). (4) The App connects to the compose Postgres with `--config` (pending, needs the Docker daemon).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-04] — Conventions § 5: standard library first
- **Type:** Minor (docs only; a new binding code rule in the project conventions doc)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.7), `docs/documents_manifest.md` (v0.51), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. New rule: before you write a function, check that the Go standard library does not already have it; if it does, use the standard library. The `code-reviewer` agent (local file) checks the rule. Existing case: `urlEscape` in `src/config/config.go` copies `net/url` (not changed here).
- **QA scenarios:** None needed (Rule 11 exempts governance-doc edits with no behavior impact).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-04] — CLAUDE.md § 1: one active task only
- **Type:** Major (Critical File edit; docs only)
- **Files:** `CLAUDE.md` (v1.7), `docs/documents_manifest.md` (v0.50), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. New § 1 rule: at most one task file is in `docs/Plans/in-progress_tasks/` at a time. Before a task moves from `pending_tasks/` to `in-progress_tasks/`, the folder must be empty; otherwise finish the active task or ask the operator.
- **QA scenarios:** None needed (Rule 11 exempts governance-doc edits with no behavior impact).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-04] — Conventions § 4: nil safety
- **Type:** Minor (docs only; a new binding code rule in the project conventions doc)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.6), `docs/documents_manifest.md` (v0.49), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. New rule for all Go code: use a value that can be nil (pointer, interface, `err`, slice index, map write, type assertion) only after an `if` check, or when the code gives clear evidence that it cannot be nil (a direct non-nil assignment in the same function). A function result is not evidence. In tests, a failed `assert` does not stop the test, so a check that a later line needs is guarded (`if !assert.NoError(t, err) || !assert.Len(t, res, 1) { return }`; `t.FailNow()` in helpers). The `code-reviewer` agent (local file) checks the rule. The nil-guards change of the same date already follows it.
- **QA scenarios:** None needed (Rule 11 exempts governance-doc edits with no behavior impact).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-04] — Nil guards in the Go tests and the config code
- **Type:** Minor (test hardening; two input guards in the config package)
- **Files:** `src/config/decode.go`, `src/config/hot.go`, `src/config/decode_test.go`, `src/config/config_test.go`, `src/config/boot_test.go`, `src/config/hot_test.go`, `src/app/app_test.go`, `src/app/defaults_test.go`, `src/system/postgres/live_test.go`, `src/system/postgres/migrator_test.go`, `src/system/postgres/postgres_test.go`, `src/common/monitoring/jaeger/jaeger_test.go`, `src/common/monitoring/prometheus/prometheus_test.go`, `src/common/logging/zap/zap_test.go`, `docs/Plans/OPEN_TASKS.md` (v1.13), `docs/documents_manifest.md` (v0.48), `CHANGELOG.md` (this entry).
- **Impact:** Small behavior change in two functions; tests only elsewhere. `decodeStruct` returns an error (not a panic) when the target is not a settable struct. `NewReloader` returns an error when the repository, the holder or the logger is nil (before, the nil value crashed later, at the first reload). The App calls both with real values, so the running App does not change. In the tests, every use of a value that can be nil after a failed check is now guarded: `err.Error()` runs only after `assert.Error` passed; a nil `*App`, `*Builder`, `*Client`, `*Reloader`, `*Tracer`, `*Logger` or `*http.Response` stops the test (`if !assert.NoError(...) { return }`, or `t.FailNow()` in the helpers `build`, `defaultApp`, `newClient`, `newRel`); `rows[0]` and `order[0]` run only after a length check; the test helper `decodeDoc` checks its type assertion; `TestBootLoads` wraps `Boot` in `assert.NotPanics`. A failed check now gives a normal test failure, and the other tests still run. All assertions still use testify `assert` (conventions § 3).
- **QA scenarios:** (1) In `src/` `go build ./... && go vet ./... && go test -race -count=1 ./...`: all pass (done, 2026-11-04). (2) Deliberate failure: a `go test -overlay` copy of `config.go` whose `Parse` returns a nil error gives `FAIL` with "An error is expected but got nil" and 0 `panic: runtime error` lines (done, 2026-11-04). (3) New tests `TestDecodeBadTarget` and the nil cases in `TestHotFieldsErrors` pass (done, 2026-11-04).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-04] — Implementation Plan Step 4: Python generator and config.yml
- **Type:** Minor (plan text follows the code of task 005)
- **Files:** `docs/Plans/Implementation_Plan.md` (v0.11), `docs/documents_manifest.md` (v0.47), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Step 4 now describes the Python generator `scripts/yml_to_env/yml_to_env.py` (PyYAML 6.0.2, 18 unittest tests, `make venv` / `make env` / `make compose` / `make test-scripts`) in place of the removed Bash tool, and `config.yml` plus the generated `config_generated.env` in place of `.env` and `.env.example`. The "not decided: does compose use this tool" point is closed (task 005 decided it).
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-11-02] — Task 005_yml_to_env_generator: Python generator and `make compose`
- **Type:** Major (new script, Makefile targets, compose env source, removed files)
- **Files:** `scripts/yml_to_env/` (`yml_to_env.py` v0.1, `yml_to_env_test.py` v0.1, `requirements.txt`; the Bash `yml_to_env.sh` and `yml_to_env_test.sh` are removed), `Makefile`, `docker-compose.yml`, `.gitignore`, `.env.example` (removed), `docs/folder_structure.md` (v0.20), `docs/Plans/OPEN_TASKS.md` (v1.12), task file moved to `done_tasks/` (v1.0), `docs/documents_manifest.md` (v0.46), `CHANGELOG.md` (this entry).
- **Impact:** Yes. Compose variables now come from `config_generated.env`, generated from `config.yml` (git-ignored). `make compose` runs `make env` (venv with pinned PyYAML 6.0.2, then the generator), then `docker compose --env-file config_generated.env up`; if step 1 fails, step 2 does not run. The generator: default output `config_generated.env` in the current directory; `--output` takes a file or a directory; names are upper case with the leading `database` part dropped (`database.postgres.password` is `POSTGRES_PASSWORD`; `--strip`, `--lower-case`); nested keys join with `_`, list items get an index; a 3-line header says auto-generated, do not edit, mainly used by docker-compose; the file is written whole (temp file, then rename, mode 0600) or not at all. Floats keep their YAML text. `.env.example` is gone. Python is now a project language: `CLAUDE.md` § 1 and the conventions doc are not updated yet (a Critical File edit, needs operator approval).
- **QA scenarios:** (1) `make test-scripts`: 18 tests pass; a deliberate bug failed 8 of them (done, 2026-11-02). (2) `make env` on a custom multi-level YAML wrote the expected names and values to a tmp path, mode 0600; `docker compose --env-file <it> config` resolved the user, password, db and both ports (done, 2026-11-02). (3) `make compose` with a missing YAML stops at step 1 and does not run docker (done, 2026-11-02). (4) `make compose` with a valid `config.yml` brings `postgres` and `redis` up healthy (pending, needs the Docker daemon).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 005_yml_to_env_generator verification
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/005_yml_to_env_generator.md` (v0.5), `docs/documents_manifest.md` (v0.45), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 005 gets a final "Verification" section: run the script in a tmp folder on a custom multi-level YAML and check the output file path, key names and values.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 005_yml_to_env_generator output name
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/005_yml_to_env_generator.md` (v0.4), `docs/documents_manifest.md` (v0.44), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. `config_generated.env` is now the default output name; the script also accepts a custom output path/name. The Definition of Done and the test list carry this.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task 004_use_yml_config: Config from a YAML file
- **Type:** Major (config source; `cmd` entry points)
- **Files:** `src/config/` (`config.go`, new `decode.go`, new `boot.go` and their tests), `src/cmd/` (`run.go`, `start-app/start.go`, `migrate/migrate.go` and tests), `src/app/defaults_test.go`, `src/app/README.md` (v0.3), `src/go.mod` / `go.sum` (`go.yaml.in/yaml/v3` is a direct requirement), `docs/folder_structure.md` (v0.19), task file moved to `done_tasks/`, `docs/documents_manifest.md` (v0.43), `CHANGELOG.md` (this entry).
- **Impact:** Yes, behavior changes. `Config` no longer reads ENV. The App and `migrate` require `--config=path/to/config.yml`; a missing flag, a missing or unreadable file, invalid YAML or a broken rule (required `database.postgres.password`, port range, positive durations) panics. Keys: `service_name`, `hot_reload_interval`, `logs.{level,otlp_endpoint}`, `metrics.prometheus.addr`, `database.postgres.*`, `database.redis.*`, `database.migration.{on_start,timeout}`; a missing key keeps its default. The mapper assigns a YAML array to a slice/array, an object to a struct or `map[string]T`, a scalar to an int/uint/bool/string/`time.Duration`; any other pairing is an error. `bool` is supported besides the task table because `database.migration.on_start` is a bool. Not done (out of the rewritten task): `config.example.yml`, removal of `.env.example`, the `docker-compose.yml` env source (task 005).
- **QA scenarios:** (1) `go test ./...` passes (done). (2) `qabz` with no flag, with a missing file and with `database.postgres.port: "x"` panics with a path-only message (done, 2026-11-02). (3) `qabz --config=<valid file>` starts against a real Postgres and Redis (pending, needs the compose stack and a YAML file; docker-compose still reads `.env` until task 005).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config tests pass
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.14), `docs/documents_manifest.md` (v0.42), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The Definition of Done of task 004 gets the line "All tests for configs must pass."
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task rewrite: 004_use_yml_config
- **Type:** Minor (docs only; a task file rewrite)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.13), `docs/documents_manifest.md` (v0.41), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 004 is rewritten short: the `--config` rule, one YAML example, the YAML-to-Go type table, three steps and a short Definition of Done. The key placement, hot-reload default, example-file and `.env.example` text of v0.12 is removed.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config YAML-to-Go type rules
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.12), `docs/documents_manifest.md` (v0.40), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The slice-only rule of v0.11 becomes three rules: an array goes to a slice or array, an object goes to a struct or `map[string]T`, a scalar goes to its matching scalar type. Any other pairing is a parsing error. The Definition of Done and the test list carry the rules.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config slice type rule
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.11), `docs/documents_manifest.md` (v0.39), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 004 now requires that a slice field such as `[]string` accepts only a YAML array; a YAML object or scalar (or a wrong element type) is a parsing error. The Definition of Done and the test list carry the rule.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 005_yml_to_env_generator in Python
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/005_yml_to_env_generator.md` (v0.3), `docs/documents_manifest.md` (v0.38), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 005 now rewrites the generator in Python (more readable, better handling of data types) and removes the Bash converter and its tests. The design step also decides the YAML parser and whether `CLAUDE.md` § 1 lists Python.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config hot-reload default
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.10), `docs/documents_manifest.md` (v0.37), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 004 now defines the default of a hot-reloadable field as its entry in the config (found by the dotted `path`); a missing entry fails the start. All config names use dot notation. No design step is left open in task 004.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config metrics key
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.9), `docs/documents_manifest.md` (v0.36), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. `METRICS_ADDR` gets the YAML path `metrics.prometheus.addr` (top-level section `metrics`, subsection `prometheus`). No key of task 004 is open for design any more, except the declaration of hot-reload defaults.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config migration keys
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.8), `docs/documents_manifest.md` (v0.35), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The migration keys move from `logs` to `database.migration.on_start` and `database.migration.timeout`; `logs` keeps `otlp_endpoint` and `level`.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config key placement
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.7), `docs/documents_manifest.md` (v0.34), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 004 now places `service_name` and `hot_reload_interval` at the top level of the YAML file, and puts `otlp_endpoint`, `level`, `migrate_on_start` and `migrate_timeout` in the section `logs`. `metrics_addr` is still an open design item.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 005_yml_to_env_generator
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/005_yml_to_env_generator.md` (v0.2), `docs/documents_manifest.md` (v0.33), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 005 now fixes the file name `config_generated.env`, a `make compose` target with two steps (autogenerate, then `docker compose up`), and a header comment that also says the file is mainly used by `docker-compose`.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — New task: 005_yml_to_env_generator
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/005_yml_to_env_generator.md` (v0.1, new), `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.6, points to task 005 for compose), `docs/Plans/pending_tasks/task_counter.json` (`next_task_id` = `006`), `docs/documents_manifest.md` (v0.32), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 005: an env file is auto-generated from the YAML config, has a name and header that show it is generated, is git-ignored, and is what `docker-compose.yml` reads. This answers the open compose question of task 004.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config (key mapping)
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.5), `docs/documents_manifest.md` (v0.31), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 004 now has the explicit mapping `POSTGRES_X` to `database.postgres.x` and `REDIS_X` to `database.redis.x` (8 keys), with a YAML example.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config (panic on missing config)
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.4), `docs/documents_manifest.md` (v0.30), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 004 now says: if `--config` is missing, or the file does not exist or cannot be read or parsed, for any reason, the App panics and fails with a non-zero exit, and never starts with defaults. This closes the earlier open point about a missing flag.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config (operator answers)
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.3), `docs/documents_manifest.md` (v0.29), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Task 004 now records: the path comes from `--config=path/to/config.yml` (for `start-app` and `migrate`); the YAML has `database.postgres.*` with `database.redis.*` as a sibling; every hot-reloadable entry must have a sensible default, and a hot field without one is a bootstrap error. The hot-reload default rule is placed in task 004, not in the finished task 003.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: 004_use_yml_config
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.2), `docs/documents_manifest.md` (v0.28), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task now says all existing ENV keys (15 keys, listed in the task) move to the YAML config, and `Config` no longer reads ENV.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task IDs move to the start of the name; new task 004
- **Type:** Major (Critical File edit: `CLAUDE.md`)
- **Files:** `CLAUDE.md` (v1.6, § 1 Tasks), `docs/Plans/pending_tasks/task_template.md` (v0.3), `docs/Plans/pending_tasks/task_counter.json` (`next_task_id` = `005`), `docs/Plans/pending_tasks/004_use_yml_config.md` (v0.1, new), task files renamed in `done_tasks/` (`001_project_base_first.md`, `002_app_bootstrap.md`, `003_config_hot_reload.md`), `docs/documents_manifest.md` (v0.27), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. A task name is now `<ID>_name.md`; this rule replaces the earlier rule that put the ID at the end. Older CHANGELOG and manifest entries keep the old names as history. New task `004_use_yml_config`: the App loads its config from a YAML file (default and only source for now), fields map to entries with the tag `path:"a.b.c"`, `config.example.yml` is committed, `.env.example` is removed. The task text says how the App gets the YAML path needs a design step.
- **QA scenarios:** None needed (Rule 11 exempts governance-doc edits with no behavior impact).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — No down migrations
- **Type:** Major (command removed; Critical File edit: `system.html`)
- **Files:** `src/migrations/000001_app_hot_configs.down.sql` (deleted), `src/system/postgres/migrator.go` (`Down` removed), `src/cmd/migrate/migrate.go` (`down` action removed), tests, `src/migrations/migrations.go`, `docs/initial-discovery/system.html` (v1.2), `docs/folder_structure.md` (v0.18), `docs/Plans/OPEN_TASKS.md` (v1.11), `docs/documents_manifest.md` (v0.26), this entry.
- **Impact:** Yes. There is no rollback. `migrate` has `up`, `force V` and `version`; `migrate down` exits 2. Migration files are `NNNNNN_name.up.sql` only (a test enforces it). A change is rolled forward with a new migration. This supersedes the "up/down" and `down N` wording in the entry below. Live tests now reset the database by dropping the two tables.
- **QA scenarios:** see `docs/Plans/OPEN_TASKS.md`, "Config hot-reload and migrations". Live run 2026-11-02: live tests pass (81.5%); two binaries at once both log `migrations ok`; `migrate version` is `version=1 dirty=false`; `migrate down 1` exits 2.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Config hot-reload and migrations (`config_hot_reload_003`)
- **Type:** Major (new migration and runner, new config keys, Critical File edit: `system.html`)
- **Files:** `src/migrations/` (embed + `000001_app_hot_configs` up/down), `src/system/postgres/{migrator,config_repository}.go`, `src/config/{config,hot}.go`, `src/app/{app,builder,defaults}.go` + tests + README (v0.2), `src/cmd/{run.go,migrate/}`, `src/main.go`, `src/go.mod` / `go.sum` (golang-migrate v4.20.1), `docs/initial-discovery/system.html` (v1.1), `docs/folder_structure.md` (v0.17), `docs/documents_manifest.md` (v0.25), `docs/Plans/OPEN_TASKS.md` (v1.10), task file moved to `done_tasks/`, this entry.
- **Impact:** Yes. (1) `start-app` now applies pending SQL migrations at start (`MIGRATE_ON_START`, default true; `MIGRATE_TIMEOUT`, default 60s) and creates `app_hot_configs`. Several containers can start together: golang-migrate holds a Postgres advisory lock, state is in `qabz_schema_migrations`. A dirty migration stops the start; an operator runs `migrate force`. (2) New command line: `qabz migrate up | down N | force V | version`; no argument or `start-app` starts the service. (3) `Config` gets `HotReloadInterval` (`HOT_RELOAD_INTERVAL`, default 30s). A field tagged `hot_reload:"true"` (string, int, bool, time.Duration; key = field name in snake case) is reloaded from `app_hot_configs` at that interval; the config sits in a `Holder` (atomic pointer, readers get a copy). **No `Config` field is tagged yet**, so nothing reloads in production until a field is tagged; the reloader is created only when a tagged field exists. (4) `App.Config()` now returns a copy of the current value.
- **Defaults I chose (operator may change):** `key` is `UNIQUE`; all columns `NOT NULL`; timestamps default `now()`; one binary with a subcommand; the migration state table is `qabz_schema_migrations`; `cnf_type` values are `string`, `int`, `bool`, `duration`.
- **QA scenarios:** see `docs/Plans/OPEN_TASKS.md`, "Config hot-reload and migrations".
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task reorder: config_hot_reload_003
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/config_hot_reload_003.md` (v0.3), `docs/documents_manifest.md` (v0.24), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The migration paragraph is now first in "Description"; the migration bullet was already first in "Definition of Done".
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task update: config_hot_reload_003
- **Type:** Minor (docs only; a task file edit)
- **Files:** `docs/Plans/pending_tasks/config_hot_reload_003.md` (v0.2), `docs/documents_manifest.md` (v0.23), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task now names the `ConfigRepository`, the required `hot_reload_interval` entry, the locking need, and the migration for the table `app_hot_configs` (columns `id`, `key`, `cnf_type`, `value`, `created_at`, `updated_at`).
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — New task: config_hot_reload_003
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/config_hot_reload_003.md` (v0.1, new), `docs/Plans/pending_tasks/task_counter.json` (`next_task_id` = `004`), `docs/documents_manifest.md` (v0.22), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task file is not in the manifest, as with the other task files.
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — App bootstrap (`app_bootstrap_002`)
- **Type:** Major (new package, new config keys, `main.go` change)
- **Files:** `src/app/` (App, Builder, `NewDefaultBuilder`, tests, README), `src/config/`, `src/common/logging/zap/`, `src/common/monitoring/{prometheus,jaeger}/`, `src/system/{postgres,redis}/`, `src/cmd/start-app/start.go`, `src/main.go`, `src/go.mod` / `go.sum`; task file moved to `done_tasks/`; `docs/folder_structure.md` (v0.16), `docs/documents_manifest.md` (v0.21), `docs/Plans/OPEN_TASKS.md` (v1.9), this entry.
- **Impact:** Yes. The binary now loads `Config` from ENV (`POSTGRES_PASSWORD` required; `SERVICE_NAME`, `LOG_LEVEL`, `POSTGRES_HOST/PORT/USER/DB`, `REDIS_HOST/PORT/PASSWORD`, `METRICS_ADDR`, `OTLP_ENDPOINT` optional), pings Postgres and Redis, serves `/metrics`, and stops on SIGINT/SIGTERM. `main.go` no longer prints hello world. `App` uses interfaces only (except `Config`) and is made with a `Builder`. Metrics use `prometheus/common/expfmt` instead of `promhttp`, because `promhttp` needs `klauspost/compress`, which the module proxy refused (403) on 2026-11-02.
- **QA scenarios:** see `docs/Plans/OPEN_TASKS.md`, "App bootstrap".
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Task counter
- **Type:** Major (Critical File edit: `CLAUDE.md`)
- **Files:** `docs/Plans/pending_tasks/task_counter.json` (new; `next_task_id` = `003` after `app_bootstrap_002` took 002), `docs/Plans/pending_tasks/task_template.md` (v0.2), `CLAUDE.md` (v1.5), `docs/folder_structure.md` (v0.15), `docs/documents_manifest.md` (v0.20), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Each new task name ends with the ID from `task_counter.json`, and the counter is incremented in the same commit. `project_base_first_001` already used ID 001. `app_bootstrap` was made before the counter, so it is renamed to `app_bootstrap_002` and the counter is set to `003`.
- **QA scenarios:** None needed (Rule 11 exempts governance-doc edits with no behavior impact).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — CLAUDE.md: tasks rule
- **Type:** Major (Critical File edit: `CLAUDE.md`)
- **Files:** `CLAUDE.md` (v1.4; § 1 gets a "Tasks" paragraph), `docs/documents_manifest.md` (v0.19), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. Process rule: a task is defined in `docs/Plans/{pending|in-progress|done}_tasks/`, and its status and folder are updated after each task operation, in the same commit.
- **QA scenarios:** None needed (Rule 11 exempts governance-doc edits with no behavior impact).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — New task: app_bootstrap_002
- **Type:** Minor (docs only; a new task file)
- **Files:** `docs/Plans/pending_tasks/app_bootstrap_002.md` (v0.1, new), `docs/documents_manifest.md` (v0.18), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact. The task file is not in the manifest, as with the other task files (open question from the task-folders entry).
- **QA scenarios:** None needed (Rule 11 exempts pure docs).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — Project base: folder structure, hello-world main, base packages
- **Type:** Major (new packages and folders; new requirements in `go.mod`; `folder_structure.md` change)
- **Files:** `src/main.go` (new), `cmd/start-app/.gitkeep`, `.gitkeep` in every empty folder of the "Directory Structure" in `system.html`, except `system/etcd/` (operator: etcd is not used) (new), `src/go.mod`, `src/go.sum` (moved to `src/`; requirements added), `Makefile` (`PB_OUT`, `SWAG_OUT` now under `src/api/`), `.gitignore` (`src/api/…`, `src/vendor/`), `docs/Plans/Implementation_Plan.md` (v0.10), `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.4), `docs/Plans/done_tasks/project_base_first_001.md` (new task file; status done, moved by operator request), `docs/folder_structure.md` (v0.13), `docs/Plans/OPEN_TASKS.md` (v1.8), `docs/documents_manifest.md` (v0.17), `CHANGELOG.md` (this entry).
- **Impact:** No domain impact. `go run .` (from `src/`) prints `hello world` and does nothing else. `go.mod` now requires `pgx/v5`, `go-redis/v9`, `prometheus/client_golang`, `zap`, and OpenTelemetry (`otel`, `otel/sdk`, OTLP gRPC trace exporter). The OTLP exporter is the Jaeger path: Jaeger accepts OTLP, and the older Jaeger client is deprecated. The five packages are kept by blank imports in `main.go`; the blank imports go when real code uses the packages. The task name `project_base_first-001` is written `project_base_first_001`, because the task template allows only lowercase letters, numbers and `_`.
- **QA scenarios:** See `OPEN_TASKS.md` "Pending verification" (Project base skeleton). `make proto` writes to `src/api/` now; it is not re-run here (the `protoc` tools may be missing), so the existing QA scenario for it is the check.
- **Not changed:** `docs/initial-discovery/system.html` (Critical File) still shows `api/pb/` at the root and etcd; the design needs an operator decision.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-02] — docs/Plans: task folders and task template
- **Type:** Major (new top-level areas under `docs/Plans/`; Critical File edit: manifest) / Docs / Governance
- **Files:** `docs/Plans/pending_tasks/task_template.md` (v0.1, new), `docs/Plans/in-progress_tasks/.gitkeep`, `docs/Plans/done_tasks/.gitkeep` (new), `docs/documents_manifest.md` (v0.16; #33 added, #3 and #6 bumped, Last Updated rotated), `docs/folder_structure.md` (v0.12), `CHANGELOG.md` (this entry).
- **Impact:** No runtime impact — docs only. Task files (`*.md` in the three task folders) are not given manifest rows: they move between folders, and a row per task would need a manifest edit on each move. **This is an open question for the operator:** Rule 8 says every committed `.md` file must be in the manifest, so either record an exception or track the tasks. The manifest cell for #3 CHANGELOG said v1.13 while the header said v1.14; it is corrected to v1.15 here.
- **QA scenarios:** None needed (Rule 11 exempts pure docs). Check: `python3 scripts/check_doc_coverage.py` exits 0.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — scripts/yml_to_env: -u / --upper-case flag; names lower case by default
- **Type:** Major (CLI change; the default output changes)
- **Files:** `scripts/yml_to_env/yml_to_env.sh` (v0.2), `scripts/yml_to_env/yml_to_env_test.sh` (v0.2), `docs/Plans/Implementation_Plan.md` (v0.9), `docs/Plans/OPEN_TASKS.md` (v1.7), `docs/documents_manifest.md` (v0.15; #31, #32, #28, #29 bumped), `CHANGELOG.md` (this entry).
- **Impact:** Yes, the default behavior changes. Before: variable names kept the key case of the YAML (`DB: {Host: x}` gave `DB_Host=x`). Now: names are lower case by default (`db_host=x`); `-u` or `--upper-case` gives `DB_HOST=x`. Values never change. If the case change makes two names equal (`Db` and `db`), the tool stops with exit 1 and writes nothing. The tool has no user in the repository yet (`docker-compose.yml` does not call it), so no migration is needed. `docker-compose.yml` and `.env.example` use upper-case names, so a future call must use `-u`. Exit code 2 (usage) for an unknown option, including `--upper-case=value`.
- **QA scenarios:** ⏸ See `OPEN_TASKS.md`, "Pending verification" (yml_to_env). **Verification run (Rule 15, 2026-11-01):** `bash scripts/yml_to_env/yml_to_env_test.sh` printed `102 passed, 0 failed` on bash 3.2.57 (macOS), 4.4.23 and 5.2.37 (Docker images). Deliberate-failure check: `-u` set to do nothing made 10 checks fail. Manual run: `-u` on `db: {Host: localhost, pass: p w}` gave `DB_HOST=localhost` and `DB_PASS='p w'`; without the flag, `db_host=localhost`. Not run: `shellcheck` (not installed).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — Convention: `encoding/json` (v1) is forbidden; the code-reviewer rejects it
- **Type:** Major (binding convention; Critical File edit: manifest)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.3; § 1 "Forbidden"), `docs/documents_manifest.md` (v0.14; #30 bumped), `CHANGELOG.md` (this entry). The `code-reviewer` agent (`.claude/agents/code-reviewer.md`) also changed; it is local and git-ignored, so it is not in this commit.
- **Impact:** No runtime impact — no Go code exists yet. `encoding/json` (v1) is illegal in all Go code, in hot paths and outside them, in tests too. The `code-reviewer` agent greps for the import path on every review and returns NOT READY on any hit; `encoding/json/v2` stays allowed outside hot paths.
- **QA scenarios:** none that need the operator — documentation plus a local agent rule (Rule 11 exempt for the docs). **Verification run (Rule 15, 2026-11-01):** the agent's grep was run on a fixture folder: it reported a plain import, an aliased import and a grouped import in a test file, and it reported nothing for `encoding/json/v2`, `vendor/` and `pb/`. The repository has no hit.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — scripts/yml_to_env: YAML config to .env variables (Bash)
- **Type:** Major (new scripts; new top-level area under `scripts/`)
- **Files:** `scripts/yml_to_env/yml_to_env.sh` (new, v0.1), `scripts/yml_to_env/yml_to_env_test.sh` (new, v0.1), `docs/Plans/Implementation_Plan.md` (v0.8), `docs/Plans/OPEN_TASKS.md` (v1.6), `docs/folder_structure.md` (v0.11), `docs/documents_manifest.md` (v0.13; #31 and #32 added), `CHANGELOG.md` (this entry). A first Go version of the tool was written and removed the same day, before any commit; `go.mod` and `go.sum` have no trace of it.
- **Impact:** No runtime impact — a developer tool; the service does not use it. Nested keys become `a_b` names, list items get an index (`a_0`), and the output is `NAME=value` lines in the `.env` format. It reads a strict YAML subset and stops with an error on anything else (anchors, flow collections with content, block scalars, tabs, several documents). `docker-compose.yml` does not use the tool yet.
- **QA scenarios:** ⏸ See `OPEN_TASKS.md`, "Pending verification" (yml_to_env). **Verification run (Rule 15, 2026-11-01):** `bash scripts/yml_to_env/yml_to_env_test.sh` printed `84 passed, 0 failed` on bash 3.2.57 (macOS), 4.4.23 and 5.3.20 (Docker images); changing the `_` join to `.` made 20 checks fail (deliberate-failure check) and restoring it made them pass again. A YAML file with 13 values (spaces, `#`, `$`, `"`, `'`, `\`, line break, empty, null, a list) was converted and read back through `docker compose config`: every value matched. Not run: `shellcheck` (not installed).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — Implementation Plan Step 4: docker-compose for local development
- **Type:** Major (new top-level files; CI/Docker area)
- **Files:** `docker-compose.yml` (new), `.env.example` (new), `.gitignore` (`.env`), `docs/Plans/Implementation_Plan.md` (v0.7; Step 4 names the app `qabz` as a Docker service), `docs/Plans/OPEN_TASKS.md` (v1.5), `docs/folder_structure.md` (v0.10), `docs/documents_manifest.md` (v0.12), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact — no service code exists yet. Developers can start Postgres 18 and Redis 8 with `docker compose up -d`; `POSTGRES_PASSWORD` must be set in `.env`. The `qabz` service is a placeholder behind the profile `app` and needs a Dockerfile (Step 5 and later).
- **QA scenarios:** ⏸ See `OPEN_TASKS.md`, "Pending verification" (Step 4). **Verification run (Rule 15, 2026-11-01):** `docker compose config` without `POSTGRES_PASSWORD` exit 1, with it exit 0; with test values and host ports 15432/16379, `up -d --wait` made `postgres` and `redis` healthy, Postgres 18.6 answered `select version()`, Redis answered `PONG`, and `down -v` removed everything. The first run on the default ports failed: host port 6379 was already in use by another container on the developer machine.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — Convention: always `go mod tidy && go mod vendor`; `vendor/` git-ignored
- **Type:** Major (binding convention; Critical File edit: manifest)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (v0.2; new § 2 Go modules), `.gitignore` (`vendor/`), `docs/folder_structure.md` (v0.9), `docs/documents_manifest.md` (v0.11; #30 bumped), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact. Developers, CI and Docker builds must run `go mod tidy && go mod vendor` (never `go mod tidy` alone). `vendor/` is not committed, so a fresh clone runs `make proto`, then `go mod tidy && go mod vendor`, before the first build. CI and the Docker image (Step 4) must do the same.
- **QA scenarios:** ⏸ In the repository root run `go mod tidy && go mod vendor && go build ./... && go vet ./...`, then `git status --short`. Pass: all commands exit 0 and `vendor/` is not listed. **Verification run (Rule 15, 2026-11-01):** the commands exited 0 (vendor/ is 15 MB) and `git check-ignore` matched `.gitignore:9:vendor/`.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — Initialize the Go module `qabz`
- **Type:** Major (new top-level file; decides plan Step 5 details)
- **Files:** `go.mod` (new: `module qabz`, `go 1.27.3`; `go mod tidy` added the four direct requirements the generated `api/pb` code imports: `google.golang.org/grpc` v1.85.0, `google.golang.org/protobuf` v1.36.12, `github.com/grpc-ecosystem/grpc-gateway/v2` v2.33.0, `google.golang.org/genproto/googleapis/api`), `go.sum` (new), `docs/Plans/Implementation_Plan.md` (v0.6), `docs/Plans/OPEN_TASKS.md` (v1.3), `docs/folder_structure.md` (v0.8), `docs/documents_manifest.md` (v0.10), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact — no Go code exists yet. `go 1.27.3` is the latest stable Go release on 2026-11-01. A machine with an older Go downloads the 1.27.3 toolchain by itself (`GOTOOLCHAIN=auto`; checked on 2026-11-01, it works). With `GOTOOLCHAIN=local` and Go 1.27.1, `go build` stops with "go.mod requires go >= 1.27.3". The requirements in `go.mod` come from the generated code in `api/pb/` (git-ignored): on a fresh clone, run `make proto` first, and run `go mod tidy` after any change to the generated imports (without `api/pb/`, tidy would remove them). The `code-reviewer` coverage gate now applies once `.go` files exist.
- **QA scenarios:** ⏸ `go version` in the repository prints `go1.27.3` and, after `make proto`, `go build ./...` and `go vet ./...` exit 0. **Verification run (Rule 15, 2026-11-01):** `go version` printed `go1.27.3`; `go build ./...` and `go vet ./...` exited 0 with the generated `api/pb/qabz` present; the first build, before `go mod tidy`, failed with "no required module provides package" and the tidy fixed it.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — History rewrite: author and committer email changed
- **Type:** Major (history rewrite of pushed commits; operator-authorized waiver of Rule 3)
- **Files:** none — only commit metadata changed (the old Gmail address to `mostafa.t@780.team`, author and committer, all commits on `main`). `CHANGELOG.md` (this entry), `docs/documents_manifest.md` (v0.9)
- **Impact:** No runtime impact. Every commit SHA on `main` changed and `main` was force-pushed (`--force-with-lease`). Other clones must reset to the new `main`; old SHA links break. Rule 3 (no force-push of pushed commits) was waived by the operator for this one operation, on 2026-11-01. Backup of the old history: local branch `backup/pre-email-rewrite-2026-11-01` (not pushed).
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — Project conventions: JSON unmarshalling policy
- **Type:** Major (Critical File edit: CLAUDE.md, manifest; new standards file)
- **Files:** `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (new, v0.1; "Definitions" section (not numbered) incl. "hot path", "memory pool" and "Poolable", § 1 JSON policy), `CLAUDE.md` (v1.3; § 1 link inside the PROJECT-OWNED block), `docs/Plans/Implementation_Plan.md` (v0.5; Step 5 points to the policy), `docs/folder_structure.md` (v0.7), `docs/documents_manifest.md` (v0.8; #30 added, #1/#3/#5/#6 bumped), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact — no Go code exists yet. Future code must unmarshal JSON with `github.com/valyala/fastjson` in hot paths and with `encoding/json/v2` elsewhere. The rule is binding and does not expire with the plan.
- **QA scenarios:** none — documentation only (Rule 11 exempt). An automatic check is an open point in the new file.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — Implementation Plan Step 3: `make proto` generates Go code from `pb/proto/qabz`
- **Type:** Major (new file, new top-level area `api/`) / Tooling
- **Files:** `Makefile` (new), `.gitignore` (`api/pb/`, `api/swagger/`), `docs/Plans/Implementation_Plan.md` (v0.4), `docs/Plans/OPEN_TASKS.md` (v1.2), `docs/folder_structure.md` (v0.6), `docs/documents_manifest.md` (v0.7), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact — no Go code uses the output yet. A developer now needs `protoc` and four plugins on `PATH` and runs `make proto` after each fresh pull. The `Makefile` is not manifest-tracked (Rule 8 covers `.py`/`.sh` with a VERSION header).
- **QA scenarios:** ⏸ See `OPEN_TASKS.md`, "Pending verification" (Step 3).
- **Verification run (Rule 15, 2026-11-01):** `make proto` ran twice, exit 0 both times; it wrote 5 files in `api/pb/qabz/` and 2 in `api/swagger/qabz/`; `git status` did not list `api/`. `PB_DIR=nope make proto` exited 2 with the hint.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-11-01] — Implementation Plan Step 2: add `pb` protobuf repository as a git submodule
- **Type:** Major (new top-level area; Critical File edit: manifest) / Dependency
- **Files:** `.gitmodules` (new), `pb` (gitlink, pinned to `dde218e`, branch `qabz_service`), `docs/Plans/Implementation_Plan.md` (v0.3; new in git, Step 2 marked done), `docs/Plans/OPEN_TASKS.md` (v1.1; layout decision, QA scenario), `docs/folder_structure.md` (v0.5), `docs/documents_manifest.md` (v0.6; #29 added), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact — no Go code exists yet. A clone now needs `git clone --recurse-submodules` (or `git submodule update --init`) to get `pb/`. The submodule URL needs access to `p-source.780.ir`. The GitHub Actions governance workflow does not fetch submodules; it does not need them.
- **QA scenarios:** ⏸ Fresh clone with `--recurse-submodules`: `pb/proto` exists and `pb` is at `dde218e14be8815b060c1cc93d64ae6ebdedd2cb`.
- **Author:** Mostafa Talebi / Claude Sonnet 5.5

## [2026-10-28] — Open-tasks register: commit to git
- **Type:** Major (Critical File edits: CLAUDE.md, manifest) / Governance
- **Files:** `.gitignore` (OPEN_TASKS exclusion removed), `docs/Plans/OPEN_TASKS.md` (now tracked, v1.0 header), `CLAUDE.md` (v1.2; § 1 PROJECT-OWNED note recording the deviation from L046), `docs/documents_manifest.md` (v0.5; #28 added, #1/#3/#5/#6/#9 bumped, Last Updated rotated), `docs/folder_structure.md` (v0.4; tree comment), `docs/STANDARDS_LOG.md` (v1.2; E001 proposes the option upstream), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact — governance only. The register is now versioned and pushed; register edits ride in the same commit as the work they record. Suite-owned text (SESSION_OPENER/CLOSER, LESSONS L046) still says "gitignored"; the CLAUDE.md § 1 note overrides it locally until the vault adopts E001.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-10-28] — Folder structure: list every file in the repo
- **Type:** Major (Critical File edit: manifest) / Docs / Governance
- **Files:** `docs/folder_structure.md` (v0.3; tree now lists each file, including the seven `docs/initial-discovery/` pages, `docs/README.md`, the `scripts/*.toml` configs and `.github/workflows/governance.yml`; Critical files marked; `└──` branch glyphs corrected), `docs/documents_manifest.md` (v0.4; #3, #5, #6 bumped; Last Updated rotated), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact — layout documentation only.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-10-28] — Design docs: move to docs/initial-discovery/, version and register HTML
- **Type:** Major (Critical File edits: CLAUDE.md, manifest, system.html, database.html) / Docs / Governance
- **Files:** `docs/initial-discovery/*.html` (7 files moved from `docs/`; each gains a hidden `<!-- VERSION: 1.0 -->` header + CHANGE LOG after the doctype — page content unchanged), `docs/autocharge_feature.html` (deleted by operator decision), `CLAUDE.md` (v1.1; § 4 critical-file paths and § 13 domain-behavior pointer updated), `docs/folder_structure.md` (v0.2; `docs/*.html` → `docs/initial-discovery/`), `docs/README.md` (v1.2; index link), `docs/documents_manifest.md` (v0.3; entries #21–27 added, version cells bumped, Last Updated rotated), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact — no code exists yet. Bookmarks or links to the old `docs/*.html` paths break; links between the discovery pages are relative and keep working. Governance scripts do not yet enforce VERSION headers on `.html`; the manifest stamps are maintained by hand.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## [2026-10-28] — Manifest: README version cells match their headers
- **Type:** Major (Critical File edit) / Governance / Manifest
- **Files:** `docs/documents_manifest.md` (entries #2 `README.md` and #4 `docs/README.md` version `-` → v1.1 to match the VERSION headers added at bootstrap; #3 and #5 bumped for this commit; Last Updated rotated), `CHANGELOG.md` (this entry)
- **Impact:** No runtime impact — manifest bookkeeping only.
- **Author:** Mostafa Talebi / Claude Opus 5.5

## 2026-10-28 — Project bootstrap

- Bootstrapped governance from Standard Set v2.8 (vault commit 8c152cf): `CLAUDE.md`, `docs/standards/`, session opener/closer, standards log, governance scripts, GitHub Actions governance workflow, project-data locality script.
