<!-- VERSION: 1.3 -->
<!-- Last Updated: 2026-10-28 -->
<!--
CHANGE LOG
v1.3 — 2026-10-28 — Added folder_structure full-tree entry.
v1.2 — 2026-10-28 — Added initial-discovery move + HTML versioning entry.
v1.1 — 2026-10-28 — Added manifest README version-cell fix entry.
v1.0 — 2026-10-28 — Initial.
-->

# CHANGELOG

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
