<!-- VERSION: 1.21 -->
<!-- Last Updated: 2026-11-02 -->
<!--
CHANGE LOG
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
