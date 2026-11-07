<!-- VERSION: 0.60 -->
# Document & Code Manifest

## Module Quick Index

| Module / Section | Entry range |
|---|---|
| Governance + project docs (contract, standards, session docs, READMEs) | 1-17 |
| Scripts | 18-20, 31-32 |
| Initial-discovery design docs (HTML) | 21-27 |
| Plans (register, implementation plan, task template) | 28-29, 33 |
| Code module docs | 34 |
| Project conventions | 30 |

## Manifest Table

| # | Title | Version | Category | Path |
|---|---|---|---|---|
| 1 | Operating Contract | v1.7 | Governance | `CLAUDE.md` |
| 2 | Project README | v1.1 | Docs | `README.md` |
| 3 | Changelog | v1.59 | Governance | `CHANGELOG.md` |
| 4 | Docs README | v1.2 | Docs | `docs/README.md` |
| 5 | Document & Code Manifest | v0.60 | Governance | `docs/documents_manifest.md` |
| 6 | Folder Structure | v0.23 | Governance | `docs/folder_structure.md` |
| 7 | Session Opener | v1.9 | Governance | `docs/SESSION_OPENER.md` |
| 8 | Session Closer | v1.8 | Governance | `docs/SESSION_CLOSER.md` |
| 9 | Standards Log | v1.2 | Governance | `docs/STANDARDS_LOG.md` |
| 10 | Set Version Adopted | v0.1 | Governance | `docs/SET_VERSION_ADOPTED.md` |
| 11 | Machine Migration | v0.1 | Governance | `docs/MACHINE_MIGRATION.md` |
| 12 | CI/CD Standards | v1.1 | Governance | `docs/standards/CICD_STANDARDS.md` |
| 13 | Engineering Standards | v1.1 | Governance | `docs/standards/ENGINEERING_STANDARDS.md` |
| 14 | Memory Protocol | v1.2 | Governance | `docs/standards/MEMORY_PROTOCOL.md` |
| 15 | Project Standards | v1.2 | Governance | `docs/standards/PROJECT_STANDARDS.md` |
| 16 | Prompt Standards | v1.1 | Governance | `docs/standards/PROMPT_STANDARDS.md` |
| 17 | Validation Standards | v1.0 | Governance | `docs/standards/VALIDATION_STANDARDS.md` |
| 18 | Doc Coverage Check | v0.1 | Scripts | `scripts/check_doc_coverage.py` |
| 19 | Test Coverage Drift Check | v0.1 | Scripts | `scripts/check_test_coverage_drift.py` |
| 20 | Project Data Locality Tool | v0.3 | Scripts | `scripts/claude_project_data.sh` |
| 21 | System Architecture (Critical) | v1.2 | Design | `docs/initial-discovery/system.html` |
| 22 | Database Design (Critical) | v1.0 | Design | `docs/initial-discovery/database.html` |
| 23 | Discovery Index | v1.0 | Design | `docs/initial-discovery/index.html` |
| 24 | Service Dependencies | v1.0 | Design | `docs/initial-discovery/service_dep.html` |
| 25 | Payment Flow Diagram | v1.0 | Design | `docs/initial-discovery/qabz-diagram.html` |
| 26 | Cron Jobs Diagram | v1.0 | Design | `docs/initial-discovery/qabz-cron-diagram.html` |
| 27 | Implementation Phases | v1.0 | Design | `docs/initial-discovery/implementation_phases.html` |
| 28 | Open Tasks Register | v1.17 | Governance | `docs/Plans/OPEN_TASKS.md` |
| 29 | Implementation Plan | v0.12 | Plan | `docs/Plans/Implementation_Plan.md` |
| 30 | Project Specific Conventions | v0.9 | Governance | `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` |
| 31 | YAML to env generator | v0.2 | Scripts | `scripts/yml_to_env/yml_to_env.py` |
| 32 | YAML to env generator tests | v0.1 | Scripts | `scripts/yml_to_env/yml_to_env_test.py` |
| 33 | Task Template | v0.3 | Plan | `docs/Plans/pending_tasks/task_template.md` |
| 34 | App module README | v0.4 | Docs | `src/app/README.md` |

## Last Updated

- **Date**: 2026-11-07
- **Change**: Conventions § 7 Postman collection; new `qabz.postman_collection.json` (JSON, no VERSION header, so no row). Bumped #30 PROJECT_SPECIFIC_CONVENTIONS v0.9, #3 CHANGELOG v1.59, #5 manifest v0.60, #6 folder_structure v0.23.
- **Previous change**: 2026-11-07 — Task 008 `logs_to_stdout` done: App log lines go to stdout; task file moved to `done_tasks/` (not manifest-tracked). Go files have no VERSION header, so they have no rows. Bumped #3 CHANGELOG v1.58, #5 manifest v0.59, #28 OPEN_TASKS v1.17.
- **Earlier change**: 2026-11-07 — New task file `008_logs_to_stdout.md` v0.1 (not manifest-tracked); task counter at 009. Bumped #3 CHANGELOG v1.57, #5 manifest v0.58.
- **Earlier change**: 2026-11-07 — Task 007 `make_compose_actions` done: `make compose up|down|ps|logs`; task file moved to `done_tasks/` (not manifest-tracked). Bumped #3 CHANGELOG v1.56, #5 manifest v0.57, #6 folder_structure v0.22, #28 OPEN_TASKS v1.16, #29 Implementation Plan v0.12, #31 yml_to_env v0.2.
- **Earlier change**: 2026-11-07 — New task file `007_make_compose_actions.md` v0.1 (not manifest-tracked); task counter at 008. Bumped #3 CHANGELOG v1.55, #5 manifest v0.56.
- **Earlier change**: 2026-11-04 — Task 006 `greeting_ping` done: gRPC server, HTTP gateway, `Greeting`; task file moved to `done_tasks/` (not manifest-tracked). Go files have no VERSION header, so they have no rows. Bumped #3 CHANGELOG v1.54, #5 manifest v0.55, #6 folder_structure v0.21, #28 OPEN_TASKS v1.15, #34 app README v0.4.
- **Earlier change**: 2026-11-04 — Conventions § 6 `pb` submodule ownership (only `pb/proto/qabz/` is writable). Bumped #30 PROJECT_SPECIFIC_CONVENTIONS v0.8, #3 CHANGELOG v1.53, #5 manifest v0.54.
- **Earlier change**: 2026-11-04 — New task file `006_greeting_ping.md` v0.1 (not manifest-tracked); task counter at 007. Bumped #3 CHANGELOG v1.52, #5 manifest v0.53.
- **Earlier change**: 2026-11-04 — `PostgresDSN` uses `net/url` (Go files have no VERSION header, so they have no rows). Bumped #3 CHANGELOG v1.51, #5 manifest v0.52, #28 OPEN_TASKS v1.14.
- **Earlier change**: 2026-11-04 — Conventions § 5 standard library first. Bumped #30 PROJECT_SPECIFIC_CONVENTIONS v0.7, #3 CHANGELOG v1.50, #5 manifest v0.51.
- **Earlier change**: 2026-11-04 — CLAUDE.md § 1 one-active-task rule. Bumped #1 CLAUDE.md v1.7, #3 CHANGELOG v1.49, #5 manifest v0.50.
- **Earlier change**: 2026-11-04 — Conventions § 4 nil safety added. Bumped #30 PROJECT_SPECIFIC_CONVENTIONS v0.6, #3 CHANGELOG v1.48, #5 manifest v0.49.
- **Earlier change**: 2026-11-04 — Nil guards in the Go tests and the config code (Go files have no VERSION header, so they have no rows). Bumped #3 CHANGELOG v1.47, #5 manifest v0.48, #28 OPEN_TASKS v1.13.
- **Earlier change**: 2026-11-04 — Implementation Plan Step 4 follows task 005 (Python generator, `config.yml`). Bumped #29 Implementation Plan v0.11, #3 CHANGELOG v1.46, #5 manifest v0.47.
- **Earlier change**: 2026-11-02 — Task 005 `yml_to_env_generator` done: Python generator, `make compose`. #31 and #32 are now the Python files (v0.1; the Bash files are removed). Bumped #3 CHANGELOG v1.45, #5 manifest v0.46, #6 folder_structure v0.20, #28 OPEN_TASKS v1.12. Task file moved to `done_tasks/` (not manifest-tracked).
- **Earlier change**: 2026-11-02 — Task file `005_yml_to_env_generator.md` updated to v0.5 (Verification section; not manifest-tracked). Bumped #3 CHANGELOG v1.44, #5 manifest v0.45.
- **Earlier change**: 2026-11-02 — Task file `005_yml_to_env_generator.md` updated to v0.4 (default output name, custom output; not manifest-tracked). Bumped #3 CHANGELOG v1.43, #5 manifest v0.44.
- **Earlier change**: 2026-11-02 — Task 004 `use_yml_config` done: Config from YAML. Bumped #3 CHANGELOG v1.42, #5 manifest v0.43, #6 folder_structure v0.19, #34 app README v0.3. Task file moved to `done_tasks/` (not manifest-tracked). Go files have no VERSION header, so they have no rows.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.14 (Definition of Done: all config tests pass; not manifest-tracked). Bumped #3 CHANGELOG v1.41, #5 manifest v0.42.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` rewritten short, v0.13 (not manifest-tracked). Bumped #3 CHANGELOG v1.40, #5 manifest v0.41.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.12 (YAML-to-Go type rules; not manifest-tracked). Bumped #3 CHANGELOG v1.39, #5 manifest v0.40.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.11 (slice fields accept only a YAML array; not manifest-tracked). Bumped #3 CHANGELOG v1.38, #5 manifest v0.39.
- **Earlier change**: 2026-11-02 — Task file `005_yml_to_env_generator.md` updated to v0.3 (generator rewritten in Python; not manifest-tracked). Bumped #3 CHANGELOG v1.37, #5 manifest v0.38.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.10 (hot-reload default is the config entry; not manifest-tracked). Bumped #3 CHANGELOG v1.36, #5 manifest v0.37.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.9 (`metrics.prometheus.addr`; not manifest-tracked). Bumped #3 CHANGELOG v1.35, #5 manifest v0.36.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.8 (migration keys to `database.migration`; not manifest-tracked). Bumped #3 CHANGELOG v1.34, #5 manifest v0.35.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.7 (key placement: top-level and `logs`; not manifest-tracked). Bumped #3 CHANGELOG v1.33, #5 manifest v0.34.
- **Earlier change**: 2026-11-02 — Task file `005_yml_to_env_generator.md` updated to v0.2 (file name, `make compose`; not manifest-tracked). Bumped #3 CHANGELOG v1.32, #5 manifest v0.33.
- **Earlier change**: 2026-11-02 — New task file `005_yml_to_env_generator.md`; task 004 v0.6 (not manifest-tracked). Bumped #3 CHANGELOG v1.31, #5 manifest v0.32.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.5 (key mapping table; not manifest-tracked). Bumped #3 CHANGELOG v1.30, #5 manifest v0.31.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.4 (panic on missing config; not manifest-tracked). Bumped #3 CHANGELOG v1.29, #5 manifest v0.30.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.3 with the operator answers (not manifest-tracked). Bumped #3 CHANGELOG v1.28, #5 manifest v0.29.
- **Earlier change**: 2026-11-02 — Task file `004_use_yml_config.md` updated to v0.2 (not manifest-tracked). Bumped #3 CHANGELOG v1.27, #5 manifest v0.28.
- **Earlier change**: 2026-11-02 — Task IDs now start the task name (CLAUDE.md v1.6, template v0.3); task files renamed; new task `004_use_yml_config` (not manifest-tracked). Bumped #1 CLAUDE.md v1.6, #3 CHANGELOG v1.26, #5 manifest v0.27, #33 template v0.3.
- **Earlier change**: 2026-11-02 — No down migrations. #21 system.html v1.2. Bumped #3 CHANGELOG v1.25, #5 manifest v0.26, #6 folder_structure v0.18, #28 OPEN_TASKS v1.11.
- **Earlier change**: 2026-11-02 — Config hot-reload and migrations (`config_hot_reload_003` done). #21 system.html v1.1, #34 app README v0.2. Bumped #3 CHANGELOG v1.24, #5 manifest v0.25, #6 folder_structure v0.17, #28 OPEN_TASKS v1.10.
- **Earlier change**: 2026-11-02 — Task file `config_hot_reload_003.md` reordered to v0.3 (migration first; not manifest-tracked). Bumped #3 CHANGELOG v1.23, #5 manifest v0.24.
- **Earlier change**: 2026-11-02 — Task file `config_hot_reload_003.md` updated to v0.2 (not manifest-tracked). Bumped #3 CHANGELOG v1.22, #5 manifest v0.23.
- **Earlier change**: 2026-11-02 — New task file `config_hot_reload_003.md` (not manifest-tracked). Bumped #3 CHANGELOG v1.21, #5 manifest v0.22.
- **Earlier change**: 2026-11-02 — App bootstrap (`app_bootstrap_002` done). Added #34 `src/app/README.md` v0.1. Bumped #3 CHANGELOG v1.20, #5 manifest v0.21, #6 folder_structure v0.16, #28 OPEN_TASKS v1.9. Go files have no VERSION header (Go versioning rule still open), so they have no rows.
- **Earlier change**: 2026-11-02 — Task counter: `task_counter.json` added (not manifest-tracked, not `.md`); #33 task template v0.2. Bumped #1 CLAUDE.md v1.5, #3 CHANGELOG v1.19, #5 manifest v0.20, #6 folder_structure v0.15.
- **Earlier change**: 2026-11-02 — CLAUDE.md v1.4 (§ 1 Tasks rule). Bumped #1 CLAUDE.md v1.4, #3 CHANGELOG v1.18, #5 manifest v0.19.
- **Earlier change**: 2026-11-02 — New task file `app_bootstrap_002.md` (not manifest-tracked). Bumped #3 CHANGELOG v1.17, #5 manifest v0.18.
- **Earlier change**: 2026-11-02 — Project base skeleton (folders, `src/` with `main.go`, `go.mod`, base packages). Bumped #3 CHANGELOG v1.16, #5 manifest v0.17, #6 folder_structure v0.13, #28 OPEN_TASKS v1.8, #29 plan v0.10, #30 conventions v0.4. `main.go` has no VERSION header (the Go versioning rule is still open), so it has no manifest row.
- **Earlier change**: 2026-11-02 — Added #33 `docs/Plans/pending_tasks/task_template.md` v0.1 (task folders `pending_tasks/`, `in-progress_tasks/`, `done_tasks/`). Bumped #3 CHANGELOG v1.15, #5 manifest v0.16, #6 folder_structure v0.12. Quick Index row updated. Task files themselves are not manifest-tracked (see CHANGELOG impact).
- **Earlier change**: 2026-11-01 — `yml_to_env` gets `-u` / `--upper-case`; names lower case by default: #31 and #32 v0.2. Bumped #3 CHANGELOG v1.14, #5 manifest v0.15, #28 OPEN_TASKS v1.7, #29 plan v0.9.
- **Earlier change**: 2026-11-01 — `encoding/json` (v1) forbidden: #30 conventions v0.3. Bumped #3 CHANGELOG v1.13, #5 manifest v0.14.
- **Earlier change**: 2026-11-01 — Added #31 `scripts/yml_to_env/yml_to_env.sh` and #32 `yml_to_env_test.sh` (Bash). Bumped #3 CHANGELOG v1.12, #5 manifest v0.13, #6 folder_structure v0.11, #28 OPEN_TASKS v1.6, #29 plan v0.8.
- **Earlier change**: 2026-11-01 — Step 4 `docker-compose.yml` added. Bumped #3 CHANGELOG v1.11, #5 manifest v0.12, #6 folder_structure v0.10, #28 OPEN_TASKS v1.5, #29 plan v0.7.
- **Earlier change**: 2026-11-01 — Go modules rule: #30 conventions v0.2, `vendor/` git-ignored. Bumped #3 CHANGELOG v1.10, #5 manifest v0.11, #6 folder_structure v0.9, #28 OPEN_TASKS v1.4.
- **Earlier change**: 2026-11-01 — Go module added (`go.mod`, `go.sum`). Bumped #3 CHANGELOG v1.9, #5 manifest v0.10, #6 folder_structure v0.8, #28 OPEN_TASKS v1.3, #29 plan v0.6.
- **Earlier change**: 2026-11-01 — History rewrite entry (author email change; Rule 3 waived). Bumped #3 CHANGELOG v1.8, #5 manifest v0.9.
- **Earlier change**: 2026-11-01 — Added #30 `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` v0.1 (shared definitions incl. hot path, memory pool and Poolable, JSON policy). Bumped #1 CLAUDE.md v1.3, #3 CHANGELOG v1.7, #5 manifest v0.8, #6 folder_structure v0.7, #29 plan v0.5. Quick Index row added.
- **Earlier change**: 2026-11-01 — Added #29 `docs/Plans/Implementation_Plan.md` v0.4 (Step 2 done: `pb` submodule; Step 3 `Makefile` added). Bumped #3 CHANGELOG v1.6, #5 manifest v0.7, #6 folder_structure v0.6, #28 OPEN_TASKS v1.2. Quick Index row updated. The `pb/` submodule is not manifest-tracked (external repository content).
- **Earlier change**: 2026-10-28 — Added #28 `docs/Plans/OPEN_TASKS.md` v1.0 (register now committed to git; project deviation from L046). Bumped #1 CLAUDE.md v1.2, #3 CHANGELOG v1.4, #5 manifest v0.5, #6 folder_structure v0.4, #9 STANDARDS_LOG v1.2. Quick Index row added.
- **Earlier change**: 2026-10-28 — #6 `docs/folder_structure.md` v0.2 → v0.3 (tree lists every file in the repo); #3 CHANGELOG v1.3; #5 manifest v0.4.
- **Previous audit**: none
