<!-- VERSION: 1.56 -->
<!-- Last Updated: 2026-11-07 -->
<!--
CHANGE LOG
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
