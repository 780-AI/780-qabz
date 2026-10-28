<!-- VERSION: 0.3 -->
<!-- Last Updated: 2026-10-28 -->
<!--
CHANGE LOG
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
│   ├── standards/                     # the six topical standards
│   │   ├── CICD_STANDARDS.md
│   │   ├── ENGINEERING_STANDARDS.md
│   │   ├── MEMORY_PROTOCOL.md
│   │   ├── PROJECT_STANDARDS.md
│   │   ├── PROMPT_STANDARDS.md
│   │   └── VALIDATION_STANDARDS.md
│   ├── Plans/                         # committed plans (Rule 1)
│   │   ├── .gitkeep
│   │   └── OPEN_TASKS.md              # local-only register; gitignored
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
│   └── claude_project_data.sh         # project-data locality tool (Rule 14)
├── .github/
│   └── workflows/
│       └── governance.yml             # governance CI
└── .claude/                           # gitignored; project-data/ holds harness memory + transcripts (Rule 14)
```
