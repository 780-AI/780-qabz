<!-- VERSION: 1.11 -->
<!-- Last Updated: 2026-11-02 -->
<!--
CHANGE LOG
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
- ⏸ Step 4 docker-compose. Copy `.env.example` to `.env`, set `POSTGRES_PASSWORD`, run `docker compose up -d --wait`, then `docker compose ps`.
  Pass: `postgres` and `redis` are `healthy`; `docker compose exec redis redis-cli ping` prints `PONG`; `docker compose config` without `POSTGRES_PASSWORD` fails. Run `docker compose down -v` after.
- ⏸ `scripts/yml_to_env`. Run `bash scripts/yml_to_env/yml_to_env_test.sh`, then
  `printf 'db:\n  Host: localhost\n  pass: p w\n' | bash scripts/yml_to_env/yml_to_env.sh -`, then the same with `-u` (and with `--upper-case`).
  Pass: the tests print `102 passed, 0 failed` and exit 0; the output is `db_host=localhost` and `db_pass='p w'`; with `-u` it is `DB_HOST=localhost` and `DB_PASS='p w'`.
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

## Maintenance / housekeeping

- Fill `SESSION_OPENER.md` § 2 environment mapping (stage / production hosts) — before plan Step 3.
