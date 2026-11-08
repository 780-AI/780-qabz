<!-- VERSION: 1.34 -->
<!-- Last Updated: 2026-11-08 -->
<!--
CHANGE LOG
v1.34 — 2026-11-08 — Task 019 DELETE /v1/mybill/{id}: QA scenario added (feature-ON run done by the implementer, awaiting operator).
v1.33 — 2026-11-08 — Task 017 GET /v1/mybill: QA scenario added; task 016 feature-ON check run (awaiting operator); task 016 real-data follow-up done and removed.
v1.32 — 2026-11-08 — Session wrap-up (tasks 015 and 018 session): 3 lesson candidates added, 1 candidate gets more evidence; push of 323c7a4..f81f4b1 held.
v1.31 — 2026-11-08 — Task 018 `make help`: QA scenario added.
v1.30 — 2026-11-08 — Task 015 dev bill database: QA scenario added; task 016 follow-up updated (the dev database now has the bills table); postgrestest schema follow-up added.
v1.29 — 2026-11-08 — Session wrap-up (task 016 session): task 016 follow-up (real bills data shapes for task 017); 3 lesson candidates added.
v1.28 — 2026-11-08 — Lesson candidate "Logging of dependency failures" removed: codified as conventions § 10.
v1.27 — 2026-11-08 — Task 016 POST /v1/mybill: QA scenario added (feature-ON check waits for task 017).
v1.26 — 2026-11-08 — Session wrap-up: 3 lesson candidates added (helper assertions in § 3; reviewer agent before the commit; positive control must not share the input under test).
v1.25 — 2026-11-07 — Session wrap-up: 2 lesson candidates added (Step 5 policies to conventions; gate read before the guarded action).
v1.24 — 2026-11-07 — Task 014 request metrics: QA scenario added; 013 follow-up updated.
v1.23 — 2026-11-07 — Session wrap-up: Lesson candidates section added (2 candidates).
v1.22 — 2026-11-07 — Task 013 services endpoint: QA scenario and follow-ups added.
v1.21 — 2026-11-07 — Task 012 auth status 403: 009/010 scenarios now expect 403, 012 scenario added.
v1.20 — 2026-11-07 — Task 011 no audience check: 009/010 scenarios and follow-ups updated, 011 scenario added.
v1.19 — 2026-11-07 — Task 010 HMAC secret: 009 scenario and follow-ups updated, 010 scenario added.
v1.18 — 2026-11-07 — Task 009 auth middleware QA scenario and open points added.
v1.17 — 2026-11-07 — Task 008 logs-to-stdout QA scenario added.
v1.16 — 2026-11-07 — Task 007: `make compose up|down|ps|logs`; scenarios use `make compose up`; QA scenario added.
v1.15 — 2026-11-04 — Task 006 Greeting QA scenario added.
v1.14 — 2026-11-04 — PostgresDSN net/url QA scenario added.
v1.13 — 2026-11-04 — Nil-guard QA scenario added.
v1.12 — 2026-11-02 — Task 005: docker-compose and yml_to_env scenarios now use config.yml, config_generated.env and the Python generator; Config-from-YAML run scenario added.
v1.11 — 2026-11-02 — Migration scenario: no down command.
v1.10 — 2026-11-02 — Config hot-reload and migrations QA scenario added.
v1.9 — 2026-11-02 — App bootstrap QA scenario added.
v1.8 — 2026-11-02 — Project base skeleton QA scenario added; Go paths under src/.
v1.7 — 2026-11-01 — yml_to_env QA scenario extended for -u / --upper-case.
v1.6 — 2026-11-01 — yml_to_env QA scenario added.
v1.5 — 2026-11-01 — Step 4 QA scenario added.
v1.4 — 2026-11-01 — Go modules rule QA scenario added.
v1.3 — 2026-11-01 — Go module path and version decided (qabz, go 1.27.3).
v1.2 — 2026-11-01 — Step 3 QA scenario added.
v1.1 — 2026-11-01 — Code-layout decision recorded; Implementation_Plan Step 2 (pb submodule) done; Pending verification section added.
v1.0 — 2026-10-28 — Register committed to git (project deviation from L046).
-->

# OPEN_TASKS — open-items register

> Committed to git (project deviation from L046 — see CLAUDE.md § 1); done = deleted.

## Plans in drafting / future

- **PLAN-001 — general implementation plan** (`docs/Plans/PLAN-001_implementation.md` + `PLAN-001_tracker.md`).
  Covers the phases in `docs/initial-discovery/implementation_phases.html`.
  Proposed structure "A" (2026-10-28): one plan, detailed in stages. Each step starts as a summary;
  § 5 / § 7 / § 8 / § 10 / § 12 are added just before the step starts; Rule 13 pass and approval
  ("APPROVE Step N") per step. Step 2.1 (Go module + gRPC health server) gets full detail first.
  Operator has not confirmed "A" yet. Next: § 15.9 intake + Phase 0 audit.

## Operator decisions needed (before PLAN-001 Step 2.1 detail)

- Plan structure: A (one plan, detailed in stages — recommended) or B (roadmap + one plan per step).
- Code layout: DECIDED 2026-10-28 — follow the "Directory Structure" in `system.html` as written.
  All folders at the repo root; no `internal/`. No doc change needed for the layout.
- gRPC port. (Go module `qabz` and `go 1.27.3`: DECIDED 2026-11-01.)
- DB engine + migration tool (check `database.html` first); grpc-gateway or plain gRPC only;
  (the `.proto` files live in the `pb/` submodule — Step 2 done.)
- `.go` versioning: VERSION header per file, or git history + tags (recommended). Affects
  CLAUDE.md § 7 and Rule 8.

## Pending verification (Rule 11)

- ⏸ Step 2 `pb` submodule — fresh clone check. Run `git clone --recurse-submodules <qabz repo URL> /tmp/qabz-check`.
  Pass: `/tmp/qabz-check/pb/proto` exists and `git -C /tmp/qabz-check/pb rev-parse HEAD` prints `dde218e14be8815b060c1cc93d64ae6ebdedd2cb`.
- ⏸ Step 3 `make proto`. Run `make proto` from a fresh clone with the submodule, twice.
  Pass: both runs exit 0; `src/api/pb/qabz/` has 5 `.go` files and `src/api/swagger/qabz/` has 2 `.json` files; `git status` does not list `src/api/`.
  Also pass: `PB_DIR=nope make proto` exits non-zero and prints the `git submodule update --init` hint.
- ⏸ Go module `qabz`. Run `go version` and `go build ./...` in `src/`.
  Pass: `go version` prints `go1.27.3` (the toolchain downloads by itself); after `make proto`, `go build ./...` and `go vet ./...` exit 0.
- ⏸ Go modules rule. In `src/` run `go mod tidy && go mod vendor && go build ./... && go vet ./...`, then `git status --short`.
  Pass: all commands exit 0 and `src/vendor/` is not listed.
- ⏸ Step 4 docker-compose (task 005). Create `config.yml` with `database.postgres.password` set, run `make compose up` (in a second terminal run `make compose ps`), then `make compose down ARGS=-v`.
  Pass: `config_generated.env` is created with the auto-generated header; `postgres` and `redis` are `healthy`; `docker compose exec redis redis-cli ping` prints `PONG`; with no `database.postgres.password` in `config.yml`, `docker compose --env-file config_generated.env config` fails.
- ⏸ `scripts/yml_to_env` (task 005). Run `make test-scripts`, then `make env CONFIG=<a multi-level yml> ENV_OUT=<tmp>/out.env`.
  Pass: the tests print `OK`; `<tmp>/out.env` exists with mode 0600, a 3-line header and the expected names and values (for example `database.postgres.password` is `POSTGRES_PASSWORD`).
- ⏸ Config from YAML (task 004, feature-ON). Run `make compose up` with a valid `config.yml`, then in `src/` run `go run . --config=../config.yml`.
  Pass: the App starts, pings Postgres and Redis, and serves the metrics address; without `--config` it panics.
- ⏸ Project base skeleton. In `src/` run `go mod tidy && go mod vendor && go build ./... && go vet ./... && go run .`, then `git status --short`.
  Pass: all commands exit 0; the last one prints `hello world`; `src/vendor/` is not listed; `src/go.mod` lists `pgx/v5`, `go-redis/v9`, `prometheus/client_golang`, `zap` and `go.opentelemetry.io/otel`.
- ⏸ App bootstrap. In `src/` run `go mod tidy && go mod vendor && go build ./... && go vet ./... && go test ./...`.
  Pass: all exit 0, no test fails. Then run the service against compose (feature ON, Rule 15): `POSTGRES_PASSWORD=x docker compose up -d --wait postgres redis`,
  `POSTGRES_PASSWORD=x go run .` (set `POSTGRES_PORT` / `REDIS_PORT` if the defaults are taken), `curl localhost:9090/metrics`, then Ctrl-C.
  Pass: log lines `postgres ok`, `redis ok`, `metrics started`, `app started`, `app stopping`; `/metrics` returns `go_goroutines`; the log has no password; exit code 0.
  Also pass: without `POSTGRES_PASSWORD` the binary prints `config: POSTGRES_PASSWORD is required` and exits 2; with compose stopped it logs `postgres ping failed` and exits 1.
  Done 2026-11-02 by the implementer on ports 55432 / 56379: all pass (awaiting operator confirmation).
- ⏸ Config hot-reload and migrations. In `src/` run `go mod tidy && go mod vendor && go build ./... && go vet ./... && go test -race ./...`. Start compose
  (`POSTGRES_PASSWORD=x docker compose up -d --wait postgres redis`, other ports if taken), then run
  `QABZ_TEST_POSTGRES_DSN=postgres://qabz:x@127.0.0.1:5432/qabz go test -race -count=1 -run Live ./system/postgres/`.
  Pass: all exit 0; the live tests (8 callers of `Up` at once, repository, end-to-end hot-reload) pass.
  Then start two `go run .` processes at the same time with different `METRICS_ADDR` on an empty database.
  Pass: both log `migrations ok` and `app started`; `go run . migrate version` prints `version=1 dirty=false`; `\d app_hot_configs` shows the six columns.
  Also pass: after `UPDATE qabz_schema_migrations SET dirty=true` the start fails with "Dirty database version 1" and `migrate force 1` fixes it; `MIGRATE_ON_START=false` logs no `migrations ok`; `go run . migrate down 1` exits 2 (`unknown action "down"`).
  Done 2026-11-02 by the implementer (ports 55432 / 56379): all pass (awaiting operator confirmation).
  Not covered: a real tagged `Config` field in the running service (none is tagged yet).
- ⏸ Nil guards (2026-11-04). In `src/` run `go mod tidy && go mod vendor && go build ./... && go vet ./... && go test -race -count=1 ./...`.
  Pass: all exit 0. Then make one check fail on purpose (for example a `go test -overlay` copy of `config/config.go` whose `Parse` returns a nil error) and run `go test ./config/`.
  Pass: the output shows `FAIL` with "An error is expected but got nil" and has no `panic: runtime error` line.
  Done 2026-11-04 by the implementer: all pass (awaiting operator confirmation).
- ⏸ `PostgresDSN` with `net/url` (2026-11-04). In `src/` run `go test -race -count=1 -run DSN -v ./config/`.
  Pass: `TestPostgresDSNEscapes` (3 cases) and `TestPostgresDSNPgxParses` pass.
  Then (feature ON) set a `database.postgres.password` with `@`, `/` and `:` in `config.yml`, run `make compose up`, then in `src/` `go run . --config=../config.yml`.
  Pass: the log shows `postgres ok` and `app started`.
  Unit checks done 2026-11-04 by the implementer; the compose run is not done (awaiting operator).
- ⏸ Task 006 Greeting (2026-11-04). In `src/` run `go build ./... && go vet ./... && go test -race -count=1 ./...`.
  Pass: all exit 0. Then (feature ON) put `server.grpc.addr` and `server.http.addr` in `config.yml`, run `make compose up`, then in `src/` `go run . --config=../config.yml`.
  Send `GET http://localhost:8080/v1/greeting` from Postman.
  Pass: HTTP status 200 and the body `{"message":"Hello"}`; the log shows `grpc started` and `http started`.
  Done 2026-11-04 by the implementer with curl: all pass (awaiting the operator Postman call).
- ⏸ Task 007 `make compose` actions (2026-11-07). With a valid `config.yml` run `make compose up ARGS="-d --wait"`, `make compose ps`, `make compose logs`, `make compose down`.
  Pass: `postgres` and `redis` are `Healthy`; `ps` lists both; `logs` prints their logs; `down` removes the containers and keeps the volumes.
  Also pass: `make compose`, `make compose foo` and `make up` each print the usage and exit 2; `make compose ps CONFIG=nope.yml` stops at the generator and does not run docker.
  Done 2026-11-07 by the implementer: all pass (awaiting operator confirmation).
- ⏸ Task 008 logs to stdout (2026-11-07). In `src/` run `go test -race -count=1 -v -run 'TestNew' ./common/logging/zap/`.
  Pass: `TestNewWritesToStdout` and `TestNewLevelFilters` pass.
  Then (feature ON) run `make compose up ARGS="-d --wait"`, then in `src/` `go run . --config=../config.yml >/tmp/out.log 2>/tmp/err.log`, wait for `app started` in `/tmp/out.log`, and stop it with Ctrl-C.
  Pass: `/tmp/out.log` has the JSON lines `postgres ok` to `app stopping`; `/tmp/err.log` is empty. In the VSCode Debug Console the log lines are no longer orange-red.
  Done 2026-11-07 by the implementer: all pass (awaiting operator confirmation; the VSCode check is for the operator).
- ⏸ Task 009 auth middleware (2026-11-07). In `src/` run `go test -race -count=1 ./common/auth/... ./entry/... ./app/`.
  Pass: all pass (the JWT input table on the verifier, 4 config entries × the table on gRPC, the table through the gateway, the start rules).
  Then (feature ON) remove `debug.auth.disable` from `config.yml` (or set it to `false`), set `auth.secrets.hmac` (task 010 replaced `auth.jwt.public_key_path`; task 011 removed `auth.jwt.audience`), run the App and call `GET /v1/greeting` with no token.
  Pass: HTTP 403 `{"code":7,"message":"unauthenticated",...}` (task 012; was 401). With a valid token from the auth service: HTTP 200 `{"message":"Hello"}`. With `debug.auth.disable: true` and no token: HTTP 200 and one `auth check disabled` warning at start.
  Done 2026-11-07 by the implementer with a scratch key pair: all pass (awaiting operator confirmation with a real token).
- ⏸ Task 010 HMAC secret (2026-11-07). In `src/` run `go test -race -count=1 ./common/auth/... ./entry/... ./app/ ./config/`.
  Pass: all pass (the table has `HS256`, `HS384`, `HS512` valid; another secret, `none`, `RS256`, `ES512`, `PS256`, lower-case `hs256` and an HS512 header over an HS256 signature rejected).
  Then (feature ON) put the auth service secret in `auth.secrets.hmac` in `config.yml`, remove `debug.auth.disable`, run the App and call `GET /v1/greeting` with no token, then with a token from the auth service.
  Pass: no token gives HTTP 403 (task 012); the real token gives HTTP 200 `{"message":"Hello"}`; the secret is not in the log.
  Done 2026-11-07 by the implementer with a test secret: all pass (awaiting operator confirmation with a real token).
- ⏸ Task 011 no audience check (2026-11-07). In `src/` run `go test -race -count=1 ./common/auth/... ./entry/... ./app/ ./config/`.
  Pass: all pass (any `aud` string, also empty, is valid and reaches `Claims.Audience`; a missing `aud`, an array or a number is rejected).
  Then (feature ON) run the App with only `auth.secrets.hmac` in the `auth` section and call `GET /v1/greeting` with a token from the auth service.
  Pass: HTTP 200 `{"message":"Hello"}`.
  Done 2026-11-07 by the implementer with a test secret and tokens with different `aud` values: all pass (awaiting operator confirmation with a real token).
- ⏸ Task 012 auth status 403 (2026-11-07). In `src/` run `go test -race -count=1 ./entry/...`.
  Pass: every rejected input gives gRPC `PermissionDenied` and, through the gateway, HTTP 403.
  Then (feature ON) run the App with the check on and call `GET /v1/greeting` with no token, then with a token from the auth service.
  Pass: no token gives HTTP 403 `{"code":7,"message":"unauthenticated",...}`; the real token gives HTTP 200.
  Done 2026-11-07 by the implementer with a test secret: all pass (awaiting operator confirmation with a real token).
- ⏸ Task 013 `GET /v1/services` (2026-11-07). In `src/` run `go test -race -count=1 ./core/billtype/ ./entry/... ./config/ ./app/`, then with `QABZ_TEST_POSTGRES_DSN` set `go test -race -count=1 -run Live ./system/postgres/ ./entry/http/`.
  Pass: all pass (`TestLiveServicesEndToEnd` returns the mock rows exactly as inserted; the live tests use a throw-away schema and drop it).
  Then (feature ON) create bill's `services` table in the dev database if it does not exist, insert test rows (some with `inquiry = false`, two languages), set `service.id` in `config.yml`, run the App and call `GET /v1/services` with and without `Accept-Language: en-US`.
  Pass: HTTP 200, `serviceId` = `service.id`, the rows of the language in `order` order exactly as inserted (`inquiry: false` stays false); a second call inside 60s does not see a row change; `qabz_services_cache_total` counts `hit` and `miss`. Also pass: without the `services` table the call gives HTTP 500 `internal error` and the App log has one error line `services: list failed` with the cause. Then delete the test rows (and the table, if you created it).
  Done 2026-11-07 by the implementer on the compose dev database: all pass (awaiting operator confirmation).
- ⏸ Task 014 request metrics and Prometheus switch (2026-11-07). In `src/` run `go test -race -count=1 ./common/monitoring/prometheus/ ./entry/... ./config/ ./app/`.
  Pass: all pass (switch default / on / off, histogram in ms with sub-ms values, count and labels, rejected call counted, gateway 404 not counted, no metric registered with the switch off).
  Then (feature ON) run `make compose up ARGS="-d --wait"` and the App, call `GET /v1/greeting` 3 times and `GET /v1/services` once, then `curl localhost:9090/metrics`.
  Pass: `qabz_request_duration_ms_count` is 3 for `path="/haf80.api.bill.Service/Greeting"` and 1 for `path="/haf80.api.bill.Service/Get"`; the sums are in milliseconds; no series has a raw URL as `path`; `/metrics` has no `qabz_requests_total`.
  Also pass: with the auth check on, a call with no token adds 1 to `qabz_request_duration_ms_count{code="PermissionDenied"}`; with `metrics.prometheus.enabled: false` the log shows `metrics disabled (metrics.prometheus.enabled: false)`, `curl localhost:9090/metrics` fails, and `GET /v1/greeting` still answers.
  Done 2026-11-07 by the implementer: all pass (awaiting operator confirmation).
- ⏸ Task 018 `make help` (2026-11-08). In the repo root run `make help`, then `make help CONFIG=other.yml`.
  Pass: a usage line, then one line per target (`help`, `venv`, `env`, `compose` with a second line, `test-scripts`, `proto`) with its description; exit 0; with `CONFIG=other.yml` the `env` and `compose` lines name `other.yml`. `make` alone still creates the venv, as before.
  Done 2026-11-08 by the implementer (macOS awk and BusyBox awk): all pass (awaiting operator confirmation).
- ⏸ Task 015 dev bill database (2026-11-08). In `src/` run `go test -race -count=1 ./config/ ./cmd/... ./system/postgres/devdb/`, then with `QABZ_TEST_POSTGRES_DSN` set `go test -race -count=1 -run Live ./system/postgres/devdb/`.
  Pass: all pass (the mode table; the guard for every mode with no connection outside `dev`; seed, second seed, unseed and second unseed on an empty throw-away schema; a failed seed changes nothing).
  Then (feature ON, Rule 15) with `mode: "dev"` in `config.yml` run `go run . devdb --config=../config.yml seed`, start the App and call `GET /v1/services`; run `seed` again; set `mode: "prod"` (then `"stage"`) and run `seed` and `unseed`; set `mode: "dev"` and run `unseed`; then `seed` again to keep the dev data.
  Pass: the first seed prints `inserted services=5 providers=45` and makes the 10 bill tables; `GET /v1/services` gives HTTP 200 with the 5 services in `id` order; the second seed prints `inserted services=0 providers=0` and no row changes; each `prod` / `stage` run exits 1 with `devdb: refused: ...` and changes nothing; without `mode` the App and the command stop (`config: mode is required`); `unseed` prints `deleted services=5 providers=45` and the tables stay.
  Done 2026-11-08 by the implementer on the compose dev database: all pass (awaiting operator confirmation). The dev database is left seeded.
- ⏸ Task 015 follow-up: the test helper `postgrestest` makes `services` and `bills` from `database.html` (`services.id integer`, `updated_at timestamp`, `bills.created_at timestamp`); bill's code (and `devdb`) has `bigserial` and `timestamptz`. Decide whether the live tests use the `devdb` schema in place of the copies (one definition of the bill tables).
- ⏸ Task 016 `POST /v1/mybill` (2026-11-08). In `src/` run `go test -race -count=1 ./common/helpers/ ./core/savedbill/ ./entry/... ./config/ ./app/`, then with `QABZ_TEST_POSTGRES_DSN` set `go test -race -count=1 -run 'Live|BillsSchema' ./system/postgres/... ./entry/http/`.
  Pass: all pass (the live tests check each saved row by a direct SQL query and by `Repository.Get`, use a throw-away schema and drop it).
  Then (feature ON, Rule 15, together with task 017): with the `bills` table in the dev database (task 015), run the App with `debug.auth.disable: true` and `debug.auth.mock_user_id` set (or with a real token), `POST /v1/mybill` a water bill with a valid bill ID, a mobile bill with `phone`, and the same water bill again; then `GET /v1/mybill`.
  Pass: each POST gives HTTP 200; the second water POST returns the first `id`; GET lists each saved bill with the values that POST sent and the `id` that POST returned; a bad bill ID gives HTTP 400 `invalid parameter` and no row; `qabz_mybill_insert_total` counts `created` 2 and `existing` 1. Without the `bills` table POST gives HTTP 500 and the log has one `mybill: insert failed` line with the cause.
  Unit and live checks done 2026-11-08 by the implementer; the App run without the `bills` table gave HTTP 500 and the `mybill: insert failed` line (`relation "bills" does not exist`).
  Feature-ON run done 2026-11-08 by the implementer with task 017 (two App copies with new test users on the dev database): all pass (awaiting operator confirmation); details in `done_tasks/017_mybill_list_endpoint.md`.
- ⏸ Task 017 `GET /v1/mybill` (2026-11-08). In `src/` run `go test -race -count=1 ./core/savedbill/ ./entry/... ./app/ ./system/postgres/`, then with `QABZ_TEST_POSTGRES_DSN` set `go test -race -count=1 -run 'Live' ./system/postgres/ ./entry/http/`.
  Pass: all pass (`TestLiveSavedBillList` and `TestLiveMyBillListEndToEnd` compare the list with a direct SQL query on `bills`, values and order; throw-away schemas, dropped).
  Then (feature ON, Rule 15) run the App on the dev database (task 015) with `debug.auth.disable: true` and `debug.auth.mock_user_id` set; `GET /v1/mybill`; `POST /v1/mybill` a water bill, a mobile bill with `phone` and `operator`, a gas bill with `participateCode`; `GET` again; then set another `mock_user_id`, restart and `GET`.
  Pass: the first `GET` gives HTTP 200 `"data":[]`; the second gives the 3 bills newest first with the ids of the POST answers, the mobile `phone` and `billId` as `98...`, the gas `participateCode`, `serviceId` = `service.id`; the other user gets `"data":[]`. A row with `json_data` `{"operator": "tci", "isPined": true}` shows `operator` `tci` and `isPined` false; a row with `status = 'disable'` is not listed.
  Done 2026-11-08 by the implementer (ports 18080 / 28080, two new test users, rows deleted after the run): all pass (awaiting operator confirmation). Not run in the App: `GET` without the `bills` table (unit test only).
- ⏸ Task 019 `DELETE /v1/mybill/{id}` (2026-11-08). In `src/` run `go test -race -count=1 ./core/savedbill/ ./entry/... ./app/ ./system/postgres/`, then with `QABZ_TEST_POSTGRES_DSN` set `go test -race -count=1 -run 'Live' ./system/postgres/ ./entry/http/`.
  Pass: all pass (`TestDeleteMatchesBillUUIDParse` is the bill reference check of the id; `TestLiveSavedBillDisable` and `TestLiveMyBillDeleteEndToEnd` check every `bills` row by a direct SQL query after each call and `GET /v1/mybill`; throw-away schemas, dropped).
  Then (feature ON, Rule 15) run App copies on the dev database (task 015) with `debug.auth.disable: true`: A and B with two new `mock_user_id` values, C with `mock_user_id: ""`. `POST /v1/mybill` 3 bills as A and 1 as B; `DELETE` a bill of A as A; `GET` as A and B; then as A `DELETE` the same bill again, B's bill, an unknown id, `not-a-uuid`, an empty id (`/v1/mybill/`), `urn:uuid:<A's bill>`, A's second bill in upper case and A's third bill as `%7B<id>%7D`; as C `DELETE` a bill of A.
  Pass: each `DELETE` of a valid id gives HTTP 200 `{"message":"خطای داخلی رخ داده است"}` (bill's text). A direct SQL query shows only `status` changed to `disable`, only on A's targeted bills (upper case and `{...}` find the row); B's bill and every other column unchanged; no row removed. `GET` as A no longer lists a deleted bill and lists the others; `GET` as B is unchanged. `not-a-uuid`, the empty id, `urn:uuid:` and the call of C give HTTP 500 `{"code":2,...}` with the same text and change no row. The log of A has exactly one error line, `mybill: delete failed` for the `urn:uuid:` id (SQLSTATE 22P02); C has none. `qabz_mybill_delete_total` of A: `{type="water",result="found"}` 2, `{type="mobile",result="found"}` 1, `{type="gas",result="found"}` 1, `{type="unknown",result="not_found"}` 2 (a bill with a NULL `type` counts as `type="unknown"`). Delete the test rows after the run.
  Done 2026-11-08 by the implementer (ports 18080 / 28080 / 38080, two new test users, 4 rows deleted after the run): all pass (awaiting operator confirmation). Metrics re-check after the `type` label was added (same day, two new test users, a NULL-type row added by SQL, 4 rows deleted after): `{result="found",type="water"}` 2, `{result="found",type="mobile"}` 1, `{result="found",type="unknown"}` 1 (the NULL type), `{result="not_found",type="unknown"}` 2. Not run: the same requests against the old `bill` service on a copy of the data (no bill instance on this machine).
- ⏸ Task 013 follow-ups: confirm `service.id` (a key with default 0, or a hard-coded 105); check the real `bill` database for `inquiry IS NOT TRUE` rows (`docs/this_vs_bill.md` item 1); the Step 6 middleware: request metrics done in task 014; panic recovery and tracing for all endpoints still need their own tasks.
- ⏸ Task 009/010/011 follow-ups: confirm the HMAC variant with the auth team; ask for a secret of at least 32 bytes (RFC 7518 § 3.2; the current one has 20); `system.html` plans the JWT keys in etcd with live reload (Critical File workflow).

## Maintenance / housekeeping

- Fill `SESSION_OPENER.md` § 2 environment mapping (stage / production hosts) — before plan Step 3.

## Lesson candidates

Found in the 2026-11-07 session wrap-up. Each one waits for an operator decision: codify (a section in `PROJECT_SPECIFIC_CONVENTIONS.md`) or discard.

- **Differences from bill.** Every intended difference from the old `bill` service is a numbered item in `docs/this_vs_bill.md` and has a `@todo` comment at the code that makes it (task 013, operator). Candidate rule for the conventions doc, so later tasks follow it. Recommendation: codify.
- **Step 5 policies are not enforced.** The Prometheus switch was "mandatory" in Implementation_Plan Step 5, but no task built it; task 014 found it missing and added it. The other Step 5 policies (testable app, logging policy) are also only in the plan, so the `code-reviewer` agent does not check them. Candidate: move the Step 5 policies (Prometheus switch, testable app, logging) into `PROJECT_SPECIFIC_CONVENTIONS.md` (the plan's own open point), and check the code against each one. Recommendation: codify.
- **Read a gate before the guarded action.** In the task 014 session, a content check of the staged files and `git commit` ran in one command, so the commit ran before the check output was read (the content was correct). Suite lesson L084 already says that each gate is its own step; this is a recurrence. Candidate: an `E0NN` entry in `docs/STANDARDS_LOG.md` (recurrence evidence for L084). Recommendation: queue (log it if it happens again).

Found in the 2026-11-08 session wrap-up (task 014 review session):

- **Assertions in test helpers.** The `code-reviewer` agent rejected a `t.Fatalf` in the lookup helper `family` (task 014) as a "borderline call": conventions § 3 does not say how a helper fails, and its "setup error" exception exists only in the agent file. Candidate rule for § 3: a helper that checks product behavior fails with `assert.Fail*` (or another `assert`) and then `t.FailNow()`; `t.Fatal*` is allowed only for a setup error (a temp dir, a listener), named in the text. Recommendation: codify.
- **Run the `code-reviewer` agent before the task commit.** The `t.Fatalf` and a misplaced test comment in task 014 passed the in-chat Rule 10 review and reached `origin/main`; the agent found them only after the push, so the fix needed a second commit (317522b). Candidate: the commit pattern of a task runs the `code-reviewer` agent on the staged change before `git add`, and its verdict is part of the Rule 10 table. Recommendation: codify (conventions doc or the task template Definition of Done).
- **A positive control must not share the input under test.** The first deliberate failure for the `family` fix renamed the constant `RequestDurationName`; the test reads the same constant, so it still passed. A changed test input (a missing name) was needed. Suite lesson L082 covers positive controls; this is a recurrence detail. Candidate: an `E0NN` entry in `docs/STANDARDS_LOG.md`. Recommendation: queue (log it if it happens again).

Found in the 2026-11-08 session wrap-up (task 016 session):

- **Shared local runtime.** The agent ran `make compose down` at the end of a check, but the operator's debug session used the same Postgres and Redis (the agent's earlier `make compose up` was a no-op because they already ran). In the same session the agent's `curl` reached the operator's debug App on `:8080` instead of its own copy, which could not bind. Candidate: before starting or stopping local services, check what is already running (`docker ps`, `lsof -iTCP -sTCP:LISTEN`); stop only what the agent started; run an agent copy of the App on other ports. Candidate home: a SESSION_OPENER § 3.2 row plus a feedback memory. Recommendation: codify.
- **Commit the task file when it is created.** Tasks 016 and 017 were created and 016 was implemented before any of them was committed, so the five commits had to be rebuilt from saved copies to keep one commit per task. Candidate: a new task file (with the counter bump) is committed right after it is written, before work on it starts (extends the memory "One commit per task"). Recommendation: codify.
- **Dev auth switch gave no user.** With `debug.auth.disable`, endpoints that need a user answered 403; fixed in task 016 with `debug.auth.mock_user_id`. Candidate: a development switch that removes a check must also supply what the check provided (here the user). Recommendation: queue (log it if it happens again).

Found in the 2026-11-08 session wrap-up (tasks 015 and 018 session):

- **Generate a copied schema with the source's own tool.** For task 015 the agent printed the `bill` tables with go-pg's own `CreateTableQuery` from bill's entities (copied into the scratchpad), in place of reading the struct tags by hand. The output found facts that a reading misses: `uint32` gives `bigint`, a `uint32` primary key gives `bigserial`, and bill's migration 13 fails on a new database. Candidate rule for the conventions doc: when qabz copies a schema or a behavior from another codebase, it produces it with that codebase's own code where it can, and records how. Recommendation: codify.
- **A required config key changes every test config.** The required key `mode` (task 015) needed `mode: dev` in about 45 YAML strings in 6 test files. Candidate: one test helper that returns a minimal valid config YAML (required keys only), so the next required key is one change. Recommendation: queue (decide when the next required key comes).
- **Push needs the operator.** The agent's `git push` was blocked by the Claude Code permission check; the 3 commits wait for the operator. Candidate: the wrap-up asks the operator to push (or to add a permission rule) and the register records the held push. Recommendation: discard (a tool setting, not a project rule), unless it happens again.
- More evidence for **Shared local runtime** (above): in this session the agent checked `docker ps` and the listening ports first, used the running compose Postgres, ran its own App copy on ports 18080/18082/19090, and stopped only that copy. The operator's debug App on `:8080` and the containers stayed up. The practice worked; codify is still recommended.

