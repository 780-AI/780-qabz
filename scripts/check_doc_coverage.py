#!/usr/bin/env python3
# ==============================================================
# VERSION: 0.1
# Doc-coverage governance — surface drift between code, docs, and the manifest.
#
# Initial scaffold (permissive — exits 0 when the required governance docs exist).
# Extend with project-specific checks as the project grows.
#
# CHANGE LOG
# v0.1 — 2026-10-28 — Bootstrap scaffold (Standard Set v2.8). Structural validation only.
# ==============================================================

import sys
from pathlib import Path

repo_root = Path(__file__).resolve().parent.parent

required = [
    "docs/documents_manifest.md",
    "docs/folder_structure.md",
    "CLAUDE.md",
]
missing = [p for p in required if not (repo_root / p).exists()]

if missing:
    print("ERROR: required governance docs missing:")
    for path in missing:
        print(f"  - {path}")
    sys.exit(1)

# TODO: extend with project-specific drift checks
#   - every tracked .md present in documents_manifest.md
#   - every .py/.sh in scripts/ carries a VERSION header

print("doc coverage: OK (scaffold — structural checks only)")
sys.exit(0)
