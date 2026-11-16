<!-- VERSION: 0.58 -->
<!-- Last Updated: 2026-11-16 -->
<!--
CHANGE LOG
v0.58 — 2026-11-16 — Task 055: core/order mobilemidtermRightel in billTypes; entry/grpc phone_order.go has the Rightel order; provider/parsianproxy pays the Rightel bill type; no new production file.
v0.57 — 2026-11-16 — Task 054: core/order mobile and mobilemidterm in billTypes; entry/grpc phone_order.go has the MCI order; provider/parsianproxy pays the MCI bill types; no new production file.
v0.56 — 2026-11-16 — Task 053: entry/grpc gas_order.go (Gas.NewOrder = POST /v1/gas/order); core/order gas in billTypes; provider/parsianproxy pays gas; the tree line names the order handlers NewOrder (task 057 missed it).
v0.55 — 2026-11-16 — Task 057: the order handlers are `NewOrder` on `/v1` (was `NewOrderV2` on `/v2`); no new file.
v0.54 — 2026-11-16 — Task 052: entry/grpc utility_order.go (the shared order handler of water and electricity) and electricity_order.go (Electricity.NewOrderV2 = POST /v2/electricity/order); core/order electricity in billTypes; provider/parsianproxy pays electricity.
v0.53 — 2026-11-16 — Task 050: core/reconcile StatusWorker also runs for Irancell (keyset pages, save-report-commit); provider/irancellproxy Status (GetStatus).
v0.52 — 2026-11-16 — The task folders moved from docs/Plans/ to docs/tasks/ (task files are not listed here).
v0.51 — 2026-11-15 — Task 048: entry/grpc water_order.go (Water.NewOrderV2 = POST /v2/water/order); core/order water in billTypes; provider/parsianproxy pays water.
v0.50 — 2026-11-15 — Task 047: common/monitoring/jaeger/jaegertest/ (the span-tree test helper); system/postgres tracer.go; provider/http.go (the traced HTTP client).
v0.49 — 2026-11-15 — Task 046: core/order settle.go (Settler = the settlement of a paid order, shared by the payment step and the worker), core/reconcile settle.go (SettleWorker = the Karashop settlement worker).
v0.48 — 2026-11-15 — Task 045: entry/cron (the shared worker runner), core/reconcile (status.go: StatusWorker = the Parsian status worker; report.go: the Kafka status message), system/kafka (the producer, confluent-kafka-go), system/redis lock.go (the lock with the key prefix); docker-compose.yml has the kafka service (profile `kafka`).
v0.47 — 2026-11-15 — Task 044: core/order complete.go (Payments = the payment step of OrderComplete and the settlement after a success); provider/parsianproxy Pay (TopBill/Pay); provider/karashop settle.go (Settle = SendPaymentInfo); entry/grpc tel_order.go (Tel.NewOrderV2 = POST /v2/tel/order) and payment.go (PaymentComplete.OrderComplete on the internal gRPC server).
v0.46 — 2026-11-14 — Task 039: core/order (order.go: the order flow; token.go: the token payload), core/fee (fee.go: the resolver; calc.go: the fee calculation), common/crypto (signer.go: the order token signer), entry/grpc phone_order.go (Phone.NewOrderV2 = POST /v2/mobile/order), system/postgres order and fee repositories, devdb fees seed.
v0.45 — 2026-11-14 — Task 041: the provider/ line names the new config path of the providers.
v0.44 — 2026-11-14 — Task 025: provider/karashop/ = the Karashop adapter (the landline bill; direct HTTPS client of the Karashop API); entry/grpc tel.go (Tel.Inquiry = POST /v1/tel/inquiry).
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
├── docker-compose.yml                 # local dev stack: postgres, redis, kafka (profile `kafka`, task 045), qabz (profile `app`)
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
│   │   └── postgres/  redis/          # pgx pool (tracer.go: the span of each query), migrator, config repository, services, saved bill, provider, inquiry, order and fee repositories (bill tables; inquiry_details.go = the extra_data JSON of an inquiry: the electricity details); devdb/ = the bill tables as bill's code makes them + the services / providers / fees seed (embedded SQL; development only); postgrestest/ = RunDSN (the run database of `make test`, `QABZ_TEST_RUN_DSN`, guarded to `qabz_test_*`) and throw-away schemas in it, empty, with the services or bills table, or seeded with the devdb tables and rows (tests only); postgrestest/testdb/ = the test database tool behind `make test` and `make test-db-clean` (tests only); go-redis client with the key prefix (redis.go: Key) and the worker lock (lock.go: TryLock, ExtendAtLeast, Release of the own token only); kafka/ = the producer of the status worker (confluent-kafka-go; balancer.go = bill's LeastBytes partition rule; task 045); nats/ still empty
│   ├── common/                        # shared by every package
│   │   ├── logging/                   # zap/ (logger, done), logstash/ (log shipping, empty)
│   │   ├── monitoring/                # jaeger/ (tracing: the provider, the W3C propagator, Fail = the error status of a span; jaegertest/ = the in-memory recorder and AssertClean for the span-tree tests), prometheus/ (metrics endpoint with the Prometheus switch; requests.go: request duration histogram in ms, its _count = request counter)
│   │   ├── auth/                      # JWT verifier (HMAC HS256/384/512, shared secret); authtest/ = test secret, tokens and the shared JWT input table (tests only)
│   │   ├── helpers/                   # bill ID check (billid.go), pay ID check, amount of a pay ID and bill type of a bill ID (payid.go), phone normalization and checks (phone.go); bill rules
│   │   ├── crypto/                    # signer.go: Signer = the order token signer (RS256 with the general private key of providers.internal.payment_server.private_key_path, header kid and typ JWT, as bill)
│   │   └── errors/  validation/
│   ├── core/                          # order (order.go: Service = the order flow of every bill type (billTypes = the price rules of each; irancell since task 039; tel and telmidterm since task 044; water since task 048; electricity since task 052; gas since task 053; mobile and mobilemidterm since task 054; mobilemidtermRightel since task 055): order provider pick, "bill already paid" check, price (direct, raw, inquiry), zero check, provider fee (Karashop), fee, total, orders row; Repository; token.go: the token payload and the data maps of bill; complete.go: Payments = the payment step of OrderComplete (one orders row per status, the provider payment) and the background settlement after a success, task 044; settle.go: Settler = the settlement of a paid order with its provider (Karashop), used by Payments and by the settlement worker, task 046), fee (fee.go: Resolver = the fee lookup of bill (user and provider, user, provider, default 9000); calc.go: Calculate = fixed, percent, stair, with bill's JSON rules), inquiry (inquiry.go: Service = the inquiry flow of every bill type (water, electricity, gas; billTypes = the tid prefix and the inquiry ID rule of each): provider pick, saved-inquiry lookup, provider call, request rows, details of the answer; Repository), savedbill, reconcile (status.go: StatusWorker = the status worker of a provider (Parsian, task 045; Irancell, task 050): the pending orders of the window in keyset pages, the status call, InsertFinal with the report before the commit, the count of fixed orders of each run; report.go: Payment = bill's Kafka status message, PaymentReporter; settle.go: SettleWorker = the settlement worker (task 046): the paid, not confirmed tel and telmidterm orders of the window, keyset pages, order.Settler), billtype (services.go: Service, ServiceRepository, ServiceCatalog = in-memory cache of the service list); savedbill (savedbill.go: Bill, Repository, Store = My Bills rules: Insert, List, Delete)
│   ├── provider/                      # http.go: NewHTTPClient = the default client of the HTTP adapters (a client span, traceparent); selector.go: ProvidersService = the in-memory sorted provider list (PickProvider, List, Group, MoveUp/MoveDown, Enable/Disable, reload); the only way to read providers (conventions § 13); provider.go: ProviderInterface and its request / response types; registry.go: ProviderRegistry (name -> adapter); mock.go: NewMock (the dev mock of every adapter, conventions § 15); qabzino/ = the Qabzino adapter (water, gas and the Rightel mobile bill; direct HTTPS calls to Ayantech with the token and endpoint of providers.external.qabzino, `providers.external.qabzino.enabled` switch, fixed mock answers); qabzinoproxy/ = QabzinoProxy, the former gRPC adapter (not used); tavanir/ = the Tavanir adapter (electricity with details; direct HTTPS client of the Tavanir endpoints: login, token in memory, `providers.external.tavanir.enabled` switch, fixed mock answers); irancellproxy/ = IrancellProxy, the Irancell adapter (the Irancell mobile bill, and Status = GetStatus of an order for the status worker, task 050; a gRPC client of the irancell service, our own service, at providers.internal.irancell.grpc_address; `providers.internal.irancell.enabled` switch, fixed mock answers); jiring/ = the Jiring Switch adapter and parsianproxy/ = ParsianProxy (the MCI mobile bill, task 036; gRPC clients of the jiring-switch and the parsian-v2 service, our own services, at providers.internal.jiring_switch.grpc_address and providers.internal.parsian.grpc_address; `enabled` switches, fixed mock answers); karashop/ = the Karashop adapter (the landline bill, task 025; direct HTTPS calls to the Karashop API with the code, password and inquiry URL of providers.external.karashop, two calls per inquiry (full term, mid term), `providers.external.karashop.enabled` switch, fixed mock answers; settle.go: Settle = SendPaymentInfo after a landline payment, task 044); parsianproxy/ also pays the landline bill (Pay = TopBill/Pay, task 044), the water bill (task 048), the electricity bill (task 052), the gas bill (task 053), the MCI bill (task 054) and the Rightel bill (task 055)
│   ├── entry/                         # grpc/: gRPC server + Service (Greeting, Get = GET /v1/services) + Water (water.go: Inquiry = POST /v1/water/inquiry; answerInquiryError = the error answers of every inquiry; water_order.go: NewOrder = POST /v1/water/order, task 048; utility_order.go: the shared order handler of water, electricity and gas, tasks 052 and 053) + Electricity (electricity.go: Inquiry = POST /v1/electricity/inquiry; electricity_order.go: NewOrder = POST /v1/electricity/order, task 052) + Gas (gas.go: Inquiry = POST /v1/gas/inquiry, the inquiry ID is the participate code; gas_order.go: NewOrder = POST /v1/gas/order, task 053) + Phone (phone.go: Inquiry = POST /v1/mobile/inquiry; the operator table gives the bill type; Irancell since task 035; phone_order.go: NewOrder = POST /v1/mobile/order, the Irancell order since task 039, the MCI order since task 054, the Rightel order since task 055) + Tel (tel.go: Inquiry = POST /v1/tel/inquiry, the landline bill, task 025; the answer of Phone, the raw inquiry of Gas; tel_order.go: NewOrder = POST /v1/tel/order, task 044) + PaymentComplete (payment.go: OrderComplete, served only on the internal gRPC server server.internal_grpc.addr, no auth check, task 044) + MyBills (mybills.go: Insert = POST /v1/mybill, GetAll = GET /v1/mybill, Delete = DELETE /v1/mybill/{id}) + request metrics interceptor (requests.go, runs first) + JWT auth interceptor (auth.go); http/: grpc-gateway HTTP server; cron/: the shared worker runner (cron.go: Cron, LockPolicy = bill's lock TTL, Runner = one goroutine and ticker per worker, the Redis lock, panic recovery, the stop; task 045)
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
│   │   └── OPEN_TASKS.md              # open-tasks register (committed; deviates from L046)
│   ├── tasks/                         # task files (CLAUDE.md § 1)
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
