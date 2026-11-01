<!-- VERSION: 0.11 -->
# Document & Code Manifest

## Module Quick Index

| Module / Section | Entry range |
|---|---|
| Governance + project docs (contract, standards, session docs, READMEs) | 1-17 |
| Scripts | 18-20 |
| Initial-discovery design docs (HTML) | 21-27 |
| Plans (register, implementation plan) | 28-29 |
| Project conventions | 30 |

## Manifest Table

| # | Title | Version | Category | Path |
|---|---|---|---|---|
| 1 | Operating Contract | v1.3 | Governance | `CLAUDE.md` |
| 2 | Project README | v1.1 | Docs | `README.md` |
| 3 | Changelog | v1.10 | Governance | `CHANGELOG.md` |
| 4 | Docs README | v1.2 | Docs | `docs/README.md` |
| 5 | Document & Code Manifest | v0.11 | Governance | `docs/documents_manifest.md` |
| 6 | Folder Structure | v0.9 | Governance | `docs/folder_structure.md` |
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
| 21 | System Architecture (Critical) | v1.0 | Design | `docs/initial-discovery/system.html` |
| 22 | Database Design (Critical) | v1.0 | Design | `docs/initial-discovery/database.html` |
| 23 | Discovery Index | v1.0 | Design | `docs/initial-discovery/index.html` |
| 24 | Service Dependencies | v1.0 | Design | `docs/initial-discovery/service_dep.html` |
| 25 | Payment Flow Diagram | v1.0 | Design | `docs/initial-discovery/qabz-diagram.html` |
| 26 | Cron Jobs Diagram | v1.0 | Design | `docs/initial-discovery/qabz-cron-diagram.html` |
| 27 | Implementation Phases | v1.0 | Design | `docs/initial-discovery/implementation_phases.html` |
| 28 | Open Tasks Register | v1.4 | Governance | `docs/Plans/OPEN_TASKS.md` |
| 29 | Implementation Plan | v0.6 | Plan | `docs/Plans/Implementation_Plan.md` |
| 30 | Project Specific Conventions | v0.2 | Governance | `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` |

## Last Updated

- **Date**: 2026-11-01
- **Change**: Go modules rule: #30 conventions v0.2, `vendor/` git-ignored. Bumped #3 CHANGELOG v1.10, #5 manifest v0.11, #6 folder_structure v0.9, #28 OPEN_TASKS v1.4.
- **Previous change**: 2026-11-01 — Go module added (`go.mod`, `go.sum`). Bumped #3 CHANGELOG v1.9, #5 manifest v0.10, #6 folder_structure v0.8, #28 OPEN_TASKS v1.3, #29 plan v0.6.
- **Earlier change**: 2026-11-01 — History rewrite entry (author email change; Rule 3 waived). Bumped #3 CHANGELOG v1.8, #5 manifest v0.9.
- **Earlier change**: 2026-11-01 — Added #30 `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` v0.1 (shared definitions incl. hot path, memory pool and Poolable, JSON policy). Bumped #1 CLAUDE.md v1.3, #3 CHANGELOG v1.7, #5 manifest v0.8, #6 folder_structure v0.7, #29 plan v0.5. Quick Index row added.
- **Earlier change**: 2026-11-01 — Added #29 `docs/Plans/Implementation_Plan.md` v0.4 (Step 2 done: `pb` submodule; Step 3 `Makefile` added). Bumped #3 CHANGELOG v1.6, #5 manifest v0.7, #6 folder_structure v0.6, #28 OPEN_TASKS v1.2. Quick Index row updated. The `pb/` submodule is not manifest-tracked (external repository content).
- **Earlier change**: 2026-10-28 — Added #28 `docs/Plans/OPEN_TASKS.md` v1.0 (register now committed to git; project deviation from L046). Bumped #1 CLAUDE.md v1.2, #3 CHANGELOG v1.4, #5 manifest v0.5, #6 folder_structure v0.4, #9 STANDARDS_LOG v1.2. Quick Index row added.
- **Earlier change**: 2026-10-28 — #6 `docs/folder_structure.md` v0.2 → v0.3 (tree lists every file in the repo); #3 CHANGELOG v1.3; #5 manifest v0.4.
- **Previous audit**: none
