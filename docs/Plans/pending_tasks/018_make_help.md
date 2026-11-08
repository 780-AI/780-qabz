<!-- VERSION: 0.1 -->
# 018_make_help — `make help` lists the Makefile targets

Status: pending

## Description

Add a `help` target to the `Makefile` that lists every target with a short description. The operator asked for it on 2026-11-08 ("a command ... that list the possible commands supported by Makefile").

## Definition of Done

- `make help` prints a usage line and one line per target with its description, read from the `## target:` comments of the `Makefile`, so a new target with such a comment is listed with no change to `help`.
- `make` with no target does not change (it still runs the first target, `venv`).
- `folder_structure.md`, the CHANGELOG and the QA scenario are updated.

