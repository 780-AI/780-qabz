<!-- VERSION: 0.43 -->
<!-- Last Updated: 2026-11-11 -->
<!--
CHANGE LOG
v0.43 — 2026-11-11 — Task 037: provider/qabzino/ also inquires the Rightel mobile bill.
v0.42 — 2026-11-11 — Task 036: provider/jiring/ = the Jiring Switch adapter and provider/parsianproxy/ = ParsianProxy (gRPC clients of the jiring-switch and the parsian-v2 service; the MCI mobile bill); `make proto` also generates the two service clients (api/pb/jiringSwitch, api/pb/parsian-v2).
v0.41 — 2026-11-11 — Task 035: provider/irancellproxy/ = the Irancell adapter (gRPC client of the irancell service); entry/grpc phone.go (Phone.Inquiry = POST /v1/mobile/inquiry); `make proto` also generates the irancell service client (api/pb/irancell).
v0.40 — 2026-11-11 — Task 023: common/helpers payid.go (pay ID rules), core/inquiry raw.go (the raw inquiry), entry/grpc gas.go (Gas.Inquiry = POST /v1/gas/inquiry); provider/qabzino/ also inquires gas; core/inquiry has the table of bill types (water, electricity, gas).
v0.39 — 2026-11-10 — Task 034: provider/tavanir/ is the direct HTTPS client of the Tavanir endpoints (no gRPC client); `make proto` no longer generates the Tavanir backend client.
v0.38 — 2026-11-10 — Task 030: provider/qabzino/ is the direct HTTP adapter, provider/qabzinoproxy/ the former gRPC adapter (not used).
v0.37 — 2026-11-10 — Task 022: provider/tavanir/ (the Tavanir adapter), entry/grpc electricity.go, system/postgres inquiry_details.go (extra_data of an inquiry), core/inquiry carries the details; Makefile `proto` also generates the Tavanir client.
v0.36 — 2026-11-09 — Task 021: core/inquiry (the inquiry flow), provider/provider.go + registry.go + mock.go + qabzino/ (the first adapter), entry/grpc water.go, system/postgres inquiry repository; Makefile `proto` also generates the Qabzino client.
v0.35 — 2026-11-09 — Task 020b: Makefile line names `make test` and `make test-db-clean`; postgrestest/ gets the run-database guard (RunDSN) and testdb/ (the test database tool behind `make test`).
v0.34 — 2026-11-08 — Task 020: provider/selector.go (ProvidersService, the sorted provider list); system/postgres provider repository; postgrestest/ gets SeededSchema (devdb tables and seed).
v0.33 — 2026-11-08 — Task 019: entry/grpc MyBills.Delete; savedbill Store deletes (disables) bills.
v0.32 — 2026-11-08 — Task 017: entry/grpc MyBills.GetAll; savedbill Store lists bills.
v0.31 — 2026-11-08 — Task 018: Makefile line names `make help`.
v0.30 — 2026-11-08 — Task 015: cmd/devdb/ (dev-only seed / unseed of the bill tables) and system/postgres/devdb/ (bill schema and seed SQL); config/constants.go has the mode values; postgrestest/ gets EmptySchema.
v0.29 — 2026-11-08 — Task 016: common/helpers/ (bill ID, phone), core/savedbill/ (My Bills store), system/postgres saved bill repository + bills table in postgrestest/, entry/grpc MyBills.Insert.
v0.28 — 2026-11-07 — Task 014: common/monitoring/prometheus/requests.go (request metrics, Prometheus switch) and entry/grpc/requests.go (request metrics interceptor).
v0.27 — 2026-11-07 — Task 013: config/constants.go, core/billtype/ (service catalog), system/postgres service repository + postgrestest/, entry/grpc Service.Get; docs/this_vs_bill.md.
v0.26 — 2026-11-07 — `config.yml.example` (tracked config template) and `config.yml` (git-ignored) added to the root (conventions § 8).
v0.25 — 2026-11-07 — Task 010: common/auth/ comment says HMAC secret (was ES512 public key).
v0.24 — 2026-11-07 — Task 009: common/auth/ (JWT verifier, authtest/ test helpers) and entry/grpc/auth.go (auth interceptor).
v0.23 — 2026-11-07 — `qabz.postman_collection.json` added to the root (conventions § 7).
v0.22 — 2026-11-07 — Task 007: Makefile line names `make compose up|down|ps|logs`.
v0.21 — 2026-11-04 — Task 006: entry/grpc/ (gRPC server, Service with Greeting) and entry/http/ (grpc-gateway HTTP server) have code.
v0.18 — 2026-11-02 — No down migrations: migrate command is up, force V, version; 000001 has only the up file.
v0.20 — 2026-11-02 — Task 005: scripts/yml_to_env/ is Python (yml_to_env.py, test, requirements.txt); .env.example removed; Makefile gets venv, env, compose, test-scripts; .gitignore gets config.yml, config_generated.env, .venv/.
v0.19 — 2026-11-02 — config/ now maps YAML (config.go, decode.go, boot.go + tests); Config no longer reads ENV.
v0.17 — 2026-11-02 — Added config/hot.go, migrations/ (embedded SQL), system/postgres migrator + config repository, cmd/run.go and cmd/migrate/; main.go dispatches by argument.
v0.16 — 2026-11-02 — Added src/app/ (README.md, app, builder, defaults, tests); code in config/, common/logging/zap, common/monitoring/{prometheus,jaeger}, system/{postgres,redis}, cmd/start-app; main.go is one line.
v0.15 — 2026-11-02 — Added pending_tasks/task_counter.json.
v0.14 — 2026-11-02 — No layout change (version skipped; task files are not listed here).
v0.13 — 2026-11-02 — Added src/ (go.mod, go.sum, vendor/, main.go, api/ moved in, and the code folders; no system/etcd). cmd/ holds entry points, not main.go.
v0.12 — 2026-11-02 — Added docs/Plans/pending_tasks/, in-progress_tasks/, done_tasks/.
v0.11 — 2026-11-01 — Added scripts/yml_to_env/.
v0.10 — 2026-11-01 — Added docker-compose.yml, .env.example, .env (git-ignored).
v0.9 — 2026-11-01 — Added vendor/ (generated, git-ignored).
v0.8 — 2026-11-01 — Added go.mod.
v0.7 — 2026-11-01 — Added docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md.
v0.6 — 2026-11-01 — Added `Makefile`; `api/pb/` and `api/swagger/` (generated, git-ignored).
v0.5 — 2026-11-01 — Added `pb/` (git submodule), `.gitmodules` and `Implementation_Plan.md` to the tree.
v0.4 — 2026-10-28 — OPEN_TASKS.md is committed to git (project deviation from L046).
v0.3 — 2026-10-28 — Tree brought in line with the repo: every file listed, Critical files marked, branch glyphs fixed.
v0.2 — 2026-10-28 — Discovery HTML docs moved to docs/initial-discovery/.
v0.1 — 2026-10-28 — Initial layout at project bootstrap (Standard Set v2.8).
-->

# Folder structure

Authoritative layout reference (CLAUDE.md § 13).

```
.
├── CLAUDE.md                          # operating contract (Critical)
├── README.md                          # project entry point
├── CHANGELOG.md                       # cross-cutting project history
├── .gitignore
├── config.yml                         # local config (git-ignored): real values and secrets; same keys as config.yml.example
├── config.yml.example                 # config template: every key with its default or zero value (conventions § 8)
├── docker-compose.yml                 # local dev stack: postgres, redis, qabz (profile `app`)
├── qabz.postman_collection.json      # Postman collection of the HTTP API; updated with every API change (conventions § 7)
├── Makefile                           # `make help` (lists the targets), `make proto` (also the Qabzino backend client of QabzinoProxy), `make compose up|down|ps|logs` (YAML -> config_generated.env -> docker compose <action>), `make test-scripts`, `make test` (all Go tests in one throw-away database `qabz_test_<hex>`), `make test-db-clean` (drops leftover `qabz_test_%` databases, then leftover `qabz_test_` roles; keeps a role with objects or privileges in a non-test database and exits 1)
├── src/                               # all Go code and the Go module
│   ├── go.mod                         # Go module `qabz` (go 1.27.3)
│   ├── go.sum                         # module checksums
│   ├── vendor/                        # `go mod vendor` output; git-ignored
│   ├── main.go                        # one-line main: cmd.Run(os.Args[1:])
│   ├── app/                           # App bootstrap: builder, lifecycle, interfaces (see its README.md)
│   │   ├── README.md
│   │   ├── app.go  builder.go  defaults.go
│   │   └── app_test.go  defaults_test.go
│   ├── cmd/                           # entry points (no main.go here)
│   │   ├── run.go                     # Run(args): start-app (default), migrate or devdb
│   │   ├── start-app/                 # start.go: Run(args) loads --config, builds and runs the App
│   │   ├── migrate/                   # migrate.go: up, force V, version
│   │   └── devdb/                     # devdb.go: seed | unseed of the bill tables; development only (mode: dev)
│   ├── config/                        # settings: Config from YAML (config.go; example_test.go keeps config.yml.example in sync), consts (constants.go: CacheServicesTTL, the mode values dev / stage / prod), YAML mapper (decode.go), --config boot (boot.go); hot-reload: Holder, Reloader (hot.go)
│   ├── migrations/                    # embedded SQL files (migrations.go) + 000001_app_hot_configs.up.sql (no down files)
│   ├── system/                        # what the service connects to
│   │   └── postgres/  redis/          # pgx pool, migrator, config repository, services, saved bill, provider and inquiry repositories (bill tables; inquiry_details.go = the extra_data JSON of an inquiry: the electricity details); devdb/ = the bill tables as bill's code makes them + the services / providers seed (embedded SQL; development only); postgrestest/ = RunDSN (the run database of `make test`, `QABZ_TEST_RUN_DSN`, guarded to `qabz_test_*`) and throw-away schemas in it, empty, with the services or bills table, or seeded with the devdb tables and rows (tests only); postgrestest/testdb/ = the test database tool behind `make test` and `make test-db-clean` (tests only); go-redis client (kafka/, nats/ still empty)
│   ├── common/                        # shared by every package
│   │   ├── logging/                   # zap/ (logger, done), logstash/ (log shipping, empty)
│   │   ├── monitoring/                # jaeger/ (tracing), prometheus/ (metrics endpoint with the Prometheus switch; requests.go: request duration histogram in ms, its _count = request counter)
│   │   ├── auth/                      # JWT verifier (HMAC HS256/384/512, shared secret); authtest/ = test secret, tokens and the shared JWT input table (tests only)
│   │   ├── helpers/                   # bill ID check (billid.go), pay ID check, amount of a pay ID and bill type of a bill ID (payid.go), phone normalization and checks (phone.go); bill rules
│   │   └── errors/  validation/  crypto/
│   ├── core/                          # order, inquiry (inquiry.go: Service = the inquiry flow of every bill type (water, electricity, gas; billTypes = the tid prefix and the inquiry ID rule of each): provider pick, saved-inquiry lookup, provider call, request rows, details of the answer; Repository), savedbill, fee, reconcile, billtype (services.go: Service, ServiceRepository, ServiceCatalog = in-memory cache of the service list); savedbill (savedbill.go: Bill, Repository, Store = My Bills rules: Insert, List, Delete)
│   ├── provider/                      # selector.go: ProvidersService = the in-memory sorted provider list (PickProvider, List, Group, MoveUp/MoveDown, Enable/Disable, reload); the only way to read providers (conventions § 13); provider.go: ProviderInterface and its request / response types; registry.go: ProviderRegistry (name -> adapter); mock.go: NewMock (the dev mock of every adapter, conventions § 15); qabzino/ = the Qabzino adapter (water, gas and the Rightel mobile bill; direct HTTPS calls to Ayantech with the token and endpoint of providers.qabzino, `providers.qabzino.enabled` switch, fixed mock answers); qabzinoproxy/ = QabzinoProxy, the former gRPC adapter (not used); tavanir/ = the Tavanir adapter (electricity with details; direct HTTPS client of the Tavanir endpoints: login, token in memory, `providers.tavanir.enabled` switch, fixed mock answers); irancellproxy/ = IrancellProxy, the Irancell adapter (the Irancell mobile bill; a gRPC client of the irancell service, our own service, at providers.irancell.grpc_address; `providers.irancell.enabled` switch, fixed mock answers); jiring/ = the Jiring Switch adapter and parsianproxy/ = ParsianProxy (the MCI mobile bill, task 036; gRPC clients of the jiring-switch and the parsian-v2 service, our own services, at providers.jiring_switch.grpc_address and providers.parsian.grpc_address; `enabled` switches, fixed mock answers); adapter folder karashop (empty)
│   ├── entry/                         # grpc/: gRPC server + Service (Greeting, Get = GET /v1/services) + Water (water.go: Inquiry = POST /v1/water/inquiry; answerInquiryError = the error answers of every inquiry) + Electricity (electricity.go: Inquiry = POST /v1/electricity/inquiry) + Gas (gas.go: Inquiry = POST /v1/gas/inquiry, the inquiry ID is the participate code) + Phone (phone.go: Inquiry = POST /v1/mobile/inquiry; the operator table gives the bill type; Irancell since task 035) + MyBills (mybills.go: Insert = POST /v1/mybill, GetAll = GET /v1/mybill, Delete = DELETE /v1/mybill/{id}) + request metrics interceptor (requests.go, runs first) + JWT auth interceptor (auth.go); http/: grpc-gateway HTTP server; cron/ (empty)
│   └── api/                           # generated by `make proto`; git-ignored
│       ├── pb/                        # Go, gRPC and gateway code
│       └── swagger/                   # OpenAPI files
├── .gitmodules                        # git submodule config (pb)
├── pb/                                # git submodule: shared protobuf definitions (read-only; not manifest-tracked)
├── archive/                           # historical material; not manifest-tracked
│   └── LESSONS.md                     # numbered lessons (L0NN) behind the rules
├── docs/
│   ├── README.md                      # docs entry point
│   ├── documents_manifest.md          # version index (Critical)
│   ├── folder_structure.md            # this file
│   ├── SESSION_OPENER.md              # session-start pre-flight (Rule 9)
│   ├── SESSION_CLOSER.md              # session wrap-up routine (Rule 12)
│   ├── STANDARDS_LOG.md               # standards-suite adoption log
│   ├── SET_VERSION_ADOPTED.md         # adopted Standard Set version
│   ├── MACHINE_MIGRATION.md           # out-of-folder state catalog (Rule 14)
│   ├── this_vs_bill.md                # numbered differences from the bill service; removed when bill shuts down
│   ├── standards/                     # the six topical standards + project conventions
│   │   ├── CICD_STANDARDS.md
│   │   ├── ENGINEERING_STANDARDS.md
│   │   ├── MEMORY_PROTOCOL.md
│   │   ├── PROJECT_STANDARDS.md
│   │   ├── PROJECT_SPECIFIC_CONVENTIONS.md  # project-owned binding code conventions
│   │   ├── PROMPT_STANDARDS.md
│   │   └── VALIDATION_STANDARDS.md
│   ├── Plans/                         # committed plans (Rule 1)
│   │   ├── .gitkeep
│   │   ├── Implementation_Plan.md     # step list for the Qabz implementation (PLAN-001 draft)
│   │   ├── OPEN_TASKS.md              # open-tasks register (committed; deviates from L046)
│   │   ├── pending_tasks/             # task files not started
│   │   │   ├── task_counter.json      # next_task_id; incremented for each new task
│   │   │   └── task_template.md       # task file rules and format (template lives only here)
│   │   ├── in-progress_tasks/         # task files being worked on
│   │   │   └── .gitkeep
│   │   └── done_tasks/                # finished task files
│   │       └── .gitkeep
│   └── initial-discovery/             # discovery and design documents (HTML)
│       ├── index.html                 # entry page linking the others
│       ├── system.html                # system architecture (Critical)
│       ├── database.html              # database design (Critical)
│       ├── service_dep.html           # service dependencies
│       ├── qabz-diagram.html          # payment flow diagram
│       ├── qabz-cron-diagram.html     # cron jobs diagram
│       └── implementation_phases.html # implementation phases
├── scripts/
│   ├── check_doc_coverage.py          # governance: manifest / doc coverage (Rule 2)
│   ├── check_test_coverage_drift.py   # governance: test coverage drift (Rule 2)
│   ├── doc_coverage_allowlist.toml    # justified doc-coverage exceptions
│   ├── validation_ownership.toml      # source → tests + docs ownership map
│   ├── claude_project_data.sh         # project-data locality tool (Rule 14)
│   └── yml_to_env/                    # Python tool: YAML config to the env file for docker-compose
│       ├── yml_to_env.py              # the generator (Python 3.9+, PyYAML)
│       ├── yml_to_env_test.py         # its tests (unittest)
│       └── requirements.txt           # pinned PyYAML
├── .github/
│   └── workflows/
│       └── governance.yml             # governance CI
└── .claude/                           # gitignored; project-data/ holds harness memory + transcripts (Rule 14)
```
