<!-- VERSION: 0.9 -->
<!-- Last Updated: 2026-11-01 -->
<!--
CHANGE LOG
v0.9 — 2026-11-01 — yml_to_env: `-u` / `--upper-case` added; names are lower case by default.
v0.8 — 2026-11-01 — Step 4: `scripts/yml_to_env/` tool (YAML to .env, Bash) added and recorded.
v0.7 — 2026-11-01 — Step 4: app (`qabz`) named as a Docker service; `docker-compose.yml` added; details recorded.
v0.6 — 2026-11-01 — Step 5: Go module `qabz` created (go 1.27.3); decided details recorded.
v0.5 — 2026-11-01 — Step 5: pointer to the JSON policy in PROJECT_SPECIFIC_CONVENTIONS.md § 1.
v0.4 — 2026-11-01 — Step 3: `Makefile` with `make proto` added; details recorded.
v0.3 — 2026-11-01 — Step 2 done: `pb` submodule added; decided details recorded.
v0.2 — 2026-10-28 — Step list changed from a table to one section per step.
v0.1 — 2026-10-28 — Skeleton created. Step 1 recorded as skipped (already done). Further steps to be added later.
-->

# Implementation Plan

**Status:** DRAFT (sketching steps)
**Related:** `docs/Plans/OPEN_TASKS.md` (PLAN-001 entry), `docs/initial-discovery/implementation_phases.html`

> This file holds the step list for the Qabz implementation. Each step has its own section.
> Each step starts as a short summary. Detail (transformation rules, ALLOWLIST, QA, rollback, acceptance) is added just before the step starts.

---

## Step 1 — Bootstrap the project using Standard DocSet

**Status:** ⏭ skipped (already done)

The project is already bootstrapped with Standard DocSet. This step is recorded for documentation only. No work is needed.

---

## Step 2 — Add the protobuf repository as a git submodule

**Status:** ✅ done (2026-11-01)

Add `https://p-source.780.ir/general/backend/pb` to this repository as a git submodule. The submodule gives the project the protobuf definitions it needs before any Go code starts.

Decided and done (2026-11-01):
- Branch: `qabz_service`. The operator ran `git submodule add` and checked out this branch. `.gitmodules` has no `branch =` line, so the pin (not the branch name) controls the version.
- Commit pinned: `dde218e14be8815b060c1cc93d64ae6ebdedd2cb` ("qabz service:: duplicated bill directory to start qabz service from there on").
- Path: `pb/` at the repository root (`system.html`, "Directory Structure").
- Subdirectory for protobuf files: not decided. `pb/` has a `proto/` directory. Step 3 confirms the base path.

QA scenario (Rule 11): see `OPEN_TASKS.md`, "Pending verification". Verification of the new behavior (Rule 15): a fresh clone with `--recurse-submodules` shows the pinned commit.

---

## Step 3 — Add a Makefile to generate Go code from the protobuf files

**Status:** ✅ done (2026-11-01), with open points below

Add a `Makefile` with a target that generates Go code from the `.proto` files of the Step 2 submodule. The generated code is the base for the gRPC server and the providers in the next steps.

Decided and done (2026-11-01):
- Tool: `protoc` with `protoc-gen-go`, `protoc-gen-go-grpc`, `protoc-gen-grpc-gateway` and `protoc-gen-openapiv2`. The options follow the legacy projects' command.
- Scope: `pb/proto/qabz` only (`services.proto`, `internalservices.proto`). The variable `PB_DIR` selects another directory.
- Output: `api/pb/` (Go code) and `api/swagger/` (OpenAPI), both git-ignored. Developers run `make proto` after every fresh pull. The target deletes the old output of the directory first.
- Go module path: `qabz` (decided 2026-11-01, see Step 5). The `qabz` protos set `go_package = "micro/pb/proto/bill;bill"` and import only Google packages, so the generated code does not depend on the module path now. A later directory that imports other `pb/` packages needs `M` overrides that map to `qabz/api/pb/<dir>`.
- Not decided: whether `make proto` also runs in CI or in the Docker build (Step 4).

---

## Step 4 — Add a docker-compose file for local development

**Status:** ✅ done (2026-11-01), with open points below

Add a `docker-compose.yml` with three Docker services. The app itself is one of them:
- `postgres` — the database
- `redis` — the cache
- `qabz` — **the app itself**, run as a Docker service next to its dependencies. The service is completed in the steps that follow. At this step it is only a placeholder entry.

Decided and done (2026-11-01):
- Images: `postgres:18-alpine` and `redis:8-alpine` (major-version tags; the latest majors on Docker Hub on 2026-11-01).
- Credentials: variables in `.env` (git-ignored). `.env.example` is committed and has an empty `POSTGRES_PASSWORD`. The compose file stops with an error if `POSTGRES_PASSWORD` is not set. User, database and ports have defaults (`qabz`, `qabz`, 5432, 6379).
- Ports: published on `127.0.0.1` only. Host ports are set with `POSTGRES_PORT` and `REDIS_PORT`.
- Volumes: named volumes `postgres_data` and `redis_data`. The Postgres volume is mounted at `/var/lib/postgresql`, as Postgres 18 requires.
- Health checks: `pg_isready` and `redis-cli ping`, every 5 seconds.
- Start order: `qabz` waits for `postgres` and `redis` to be healthy (`depends_on` with `service_healthy`).
- The `qabz` service has the profile `app` and builds from `Dockerfile` in the repository root. The Dockerfile does not exist yet, so `docker compose up -d` starts only `postgres` and `redis`. `docker compose --profile app up -d --build` starts the app after the Dockerfile exists.

Tool `scripts/yml_to_env/` (added 2026-11-01, operator request): converts a YAML config file into environment variables in the `.env` format, so Compose can read them.
- Language: Bash, in `scripts/yml_to_env/yml_to_env.sh`. A first version in Go was removed on the same day; the operator chose Bash because it is portable and needs no build. It needs bash 3.2 or newer (the macOS default) and no other tool, except `mktemp` for `-o`.
- Usage: `bash scripts/yml_to_env/yml_to_env.sh [-u|--upper-case] [-o out.env] <file.yml | ->`. Output goes to stdout, or to a file with mode 0600. A bad input writes nothing and leaves an existing output file untouched.
- Names: nested keys join with `_` (`db:` with `host:` gives `db_host`). The variable names are lower case by default (`DB:` with `Host:` gives `db_host`); `-u` or `--upper-case` makes them upper case (`DB_HOST`, the form the compose file uses). Only names change, never values. If the case change makes two names equal (`Db` and `db`), the tool stops with an error. A list item gets its index (`db_0`, `srv_0_host`). A null gives an empty value. `{}` and `[]` give nothing.
- Values: written as they are in the YAML text (`1.10` stays `1.10`). A value with a space or a special character is single-quoted; a value with `'` or a line break is double-quoted with escapes. Checked with Docker Compose 5.5.1: every test value came back unchanged.
- Supported YAML is a strict subset, because a full YAML parser cannot be written in plain Bash: block mappings and block lists with space indentation; plain, single-quoted and double-quoted scalars; inline comments; one document. Anything else stops with an error and a line number (exit 1): anchors and aliases, tags, merge keys, block scalars (`|`, `>`), flow collections with content (`[a, b]`), quoted keys, tabs, several documents, a top level that is not a mapping, an invalid variable name (`my-key`), two keys with the same name.
- Tests: `scripts/yml_to_env/yml_to_env_test.sh` (102 checks). They pass on bash 3.2.57 (macOS), 4.4.23 and 5.2.37 (Docker images). A deliberate break of the `_` rule made 20 checks fail; a deliberate break of `-u` made 10 checks fail.
- Not decided: whether `docker-compose.yml` and the `.env` setup use this tool. The compose file uses upper-case names (`POSTGRES_PASSWORD`), so it needs the `-u` flag. Also open: a `make` target that runs the tool.

Not decided (open points):
- The Dockerfile, the app's own settings and ports for the `qabz` service: Step 5 and later steps.
- Whether Kafka, NATS and etcd also get services: they are outside this step.

---

## Step 5 — Code: base skeleton and system modules

**Status:** ⏸ pending

Create the base code skeleton of the service and the system modules that connect it to its infrastructure. The structure follows the "Directory Structure" section of `docs/initial-discovery/system.html`:
- `cmd/` — `start-app` and `migrate` commands
- `config/` — settings
- `system/` — `postgres`, `redis`, `kafka`, `nats`, `etcd` (connections, made once at startup)
- `common/` — shared packages needed by the system modules (for example logging)
- `common/monitoring/` — the base of the monitoring package (`prometheus/` and `jaeger/`): setup and registration only. The use of it in middleware belongs to Step 6. The Jaeger part is to be decided later.

**Prometheus switch (mandatory, 2026-10-28):** Prometheus comes with a setting in `config/`. A call to record a metric has an effect only when the setting is on. When the setting is off, every call has no effect: nothing is recorded and no metric is registered. The setting is on by default. It is off only when it is explicitly set to off. A test must show this for the default, the explicit on and the explicit off. Tests can also turn metrics off, so a test can run without any metric registered.

**Testable app (mandatory, 2026-10-28):** the service has an app object that tests can construct anywhere. The app receives its dependencies from the caller. Dependencies are injectable: the databases (Postgres, Redis and the others), the config, and the other external services. The app does not read global state or create its own connections inside its constructor. A test passes a test database, a test config or a mock in the same way that `start-app` passes the real ones.

**Logging policy (mandatory, 2026-10-28):** logs report serious errors only. Metrics handle most errors, not logs. This avoids log spam.
- Allowed: error-level logs for serious errors, and info logs for bootstrapping (for example "Database connected successfully").
- Not allowed: info or debug logs in request paths or loops.
- Each layer decides which errors are serious enough for a log and which only need a metric.

**JSON policy (mandatory, 2026-11-01):** the rule lives in `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` § 1 (the term "hot path" is defined in its "Definitions" section) and applies to all code, not only this step. Each sub-task states which of its JSON reads are hot.

Go module (decided 2026-11-01): `go.mod` at the repository root. Module path `qabz`. Go version `go 1.27.3`, the latest stable release on 2026-11-01. Go 1.27.3 includes `encoding/json/v2` without a flag (checked on 1.27.1). A machine with an older Go downloads the toolchain by itself (`GOTOOLCHAIN=auto`; checked on 2026-11-01). `go mod tidy` added `google.golang.org/grpc`, `google.golang.org/protobuf`, `github.com/grpc-ecosystem/grpc-gateway/v2` and `google.golang.org/genproto/googleapis/api`, because the generated `api/pb` code imports them. Open point: `api/pb/` is git-ignored, so `go mod tidy` on a clone without `make proto` removes these requirements. Whether `make proto` runs `go mod tidy` is not decided.

Layout decision (2026-10-28): the layout is the one in the design doc. All folders stay at the repository root. There is no `internal/` folder.

The database layer uses raw queries. It does not use an ORM (`system.html`, section "Database").

Details are not decided yet. They are set when this step starts:
- Which `common/` packages belong to this step and which go to a later step
- Which system modules to build now (all five, or a first set)
- Libraries for each connection (for example the Postgres driver and the Kafka client)
- How the JSON policy is enforced automatically (open point in `PROJECT_SPECIFIC_CONVENTIONS.md` § 1)
- Whether the other Step 5 policies (Prometheus switch, testable app, logging) also move to `PROJECT_SPECIFIC_CONVENTIONS.md`
- Whether the `.go` files carry a VERSION header (CLAUDE.md § 7 covers `.py` and `.sh` only)

---

## Step 6 — Code: Endpoints implementation

**Status:** ⏸ pending

This is the longest step. Implement the endpoints one by one, according to the docs and specifications. The sources are `docs/initial-discovery/system.html` and `docs/initial-discovery/implementation_phases.html` (phase 4). The protobuf messages come from the Step 2 submodule.

Each endpoint is its own sub-task. Each sub-task has its own QA scenario (Rule 11) and its own verification of the new behavior (Rule 15). Because the step is long, it needs the sibling tracker `PLAN-001_tracker.md` (CLAUDE.md § 15.7).

**Test requirement (mandatory, 2026-10-28):** every endpoint must have aggressive test coverage. An endpoint is not done until both kinds of test exist and pass:
- **Unit tests** — cover the endpoint logic, including edge cases and error paths.
- **Integration tests** — run against a real database and against mocks of the outbound providers and other external services.

An endpoint without both kinds of test does not close its sub-task.

**Middleware requirement (mandatory, 2026-10-28):** every endpoint is bundled with these middleware:
- **Prometheus request duration** — records the duration of each request as a metric (`qabz_` prefix, time in seconds, per `system.html`).
- **Recovery** — catches a panic in the handler and returns an error instead of crashing the process.
- **Tracing** — creates a trace span for each request. The Jaeger details are to be decided later.

The middleware tests are part of the endpoint test requirement above. The base of `common/monitoring/` is built in Step 5. This step uses it and invokes it in the middleware. The Prometheus middleware follows the Prometheus switch from Step 5: with the switch off, it has no effect.

**Monitoring requirement (mandatory, 2026-10-28):** every feature comes with the monitoring it needs. A feature is not done without it. The implementer decides what each layer needs, and states it in the sub-task.
- **Prometheus** — metrics for the behavior of the feature (counts, errors, durations). Names follow `system.html`: `qabz_` prefix, time in seconds. Errors are handled by metrics first.
- **Logs** — follow the Step 5 logging policy: serious errors only. No info logs in request paths.
- **Jaeger** — to be decided later. Do not design the Jaeger part until the operator decides it.

Each sub-task lists the metrics and the logs it adds. The tests check the metrics (CLAUDE.md Rule 15 evidence). Tests that construct the app turn metrics off or use a test config, per the Step 5 rules.

Details are not decided yet. They are set when this step starts:
- The list of endpoints and their order
- Whether the outbound providers (`ParsianProxy`, `JiringProxy`, `IrancellProxy`, `Qabzino`, `Tavanir`) are sub-tasks of this step or a separate step
- The spec source for each endpoint
