<!-- VERSION: 1.0 -->
<!-- Last Updated: 2026-10-28 -->
<!--
CHANGE LOG
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
- Code layout: standard Go layout at repo root — `cmd/qabz/`, `internal/`, `api/proto/`,
  `migrations/`, `deploy/`; no `src/` (recommended 2026-10-28; not confirmed).
- Go module path, Go version, gRPC port.
- DB engine + migration tool (check `database.html` first); grpc-gateway or plain gRPC only;
  where the existing `.proto` files live.
- `.go` versioning: VERSION header per file, or git history + tags (recommended). Affects
  CLAUDE.md § 7 and Rule 8.

## Maintenance / housekeeping

- Fill `SESSION_OPENER.md` § 2 environment mapping (stage / production hosts) — before plan Step 3.
