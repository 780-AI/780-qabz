<!-- VERSION: 1.0 -->
# 018_make_help — `make help` lists the Makefile targets

Status: done

## Description

Add a `help` target to the `Makefile` that lists every target with a short description. The operator asked for it on 2026-11-08 ("a command ... that list the possible commands supported by Makefile").

## Definition of Done

- `make help` prints a usage line and one line per target with its description, read from the `## target:` comments of the `Makefile`, so a new target with such a comment is listed with no change to `help`.
- `make` with no target does not change (it still runs the first target, `venv`).
- `folder_structure.md`, the CHANGELOG and the QA scenario are updated.

## Result (2026-11-08)

- `Makefile`: target `help` (`.PHONY`). `awk` reads the `## target: text` lines of the Makefile; a `## ` line right after one is printed as its next line. `$(CONFIG)`, `$(ENV_OUT)` and `$(PB_DIR)` in a text show their current values (for example `make help CONFIG=other.yml`); `$(ARGS)` stays as it is.
- Checked with the macOS `awk` and with BusyBox `awk` (Linux, `alpine:3`): the same output. A copy of the Makefile with an extra `## foo-bar:` target lists it last. `make compose` and `make up` still print the usage and exit 2.
- Not changed: the default goal (`make` alone still runs `venv`).

## Verification (Rule 15, 2026-11-08, by the implementer)

`make help` prints the usage line and 6 targets (`help`, `venv`, `env`, `compose` with its second line, `test-scripts`, `proto`), exit 0. Verdict: matches the design. Operator confirmation pending (`OPEN_TASKS.md`).
