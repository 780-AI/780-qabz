<!-- VERSION: 1.1 -->
<!-- Last Updated: 2026-11-01 -->
<!--
CHANGE LOG
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
- Go module path, Go version, gRPC port.
- DB engine + migration tool (check `database.html` first); grpc-gateway or plain gRPC only;
  (the `.proto` files live in the `pb/` submodule — Step 2 done.)
- `.go` versioning: VERSION header per file, or git history + tags (recommended). Affects
  CLAUDE.md § 7 and Rule 8.

## Pending verification (Rule 11)

- ⏸ Step 2 `pb` submodule — fresh clone check. Run `git clone --recurse-submodules <qabz repo URL> /tmp/qabz-check`.
  Pass: `/tmp/qabz-check/pb/proto` exists and `git -C /tmp/qabz-check/pb rev-parse HEAD` prints `dde218e14be8815b060c1cc93d64ae6ebdedd2cb`.

## Maintenance / housekeeping

- Fill `SESSION_OPENER.md` § 2 environment mapping (stage / production hosts) — before plan Step 3.
