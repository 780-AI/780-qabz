<!-- VERSION: 0.2 -->
<!-- Last Updated: 2026-10-28 -->
<!--
CHANGE LOG
v0.2 — 2026-10-28 — Discovery HTML docs moved to docs/initial-discovery/.
v0.1 — 2026-10-28 — Initial layout at project bootstrap (Standard Set v2.8).
-->

# Folder structure

Authoritative layout reference (CLAUDE.md § 13).

```
.
├── CLAUDE.md                 # operating contract
├── README.md
├── CHANGELOG.md
├── archive/                  # historical material (LESSONS.md); not manifest-tracked
├── docs/
│   ├── standards/            # the six topical standards
│   ├── Plans/                # committed plans (OPEN_TASKS.md is local-only, gitignored)
│   ├── initial-discovery/    # discovery and design documents (HTML: system, database, phases, diagrams)
│   ├── SESSION_OPENER.md / SESSION_CLOSER.md
│   ├── STANDARDS_LOG.md, SET_VERSION_ADOPTED.md, MACHINE_MIGRATION.md
│   ├── documents_manifest.md, folder_structure.md
├── scripts/                  # governance scripts + project-data locality tool
├── .github/workflows/        # governance CI
└── .claude/                  # gitignored; project-data/ holds harness memory + transcripts (Rule 14)
```
