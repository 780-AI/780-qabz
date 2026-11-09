<!-- VERSION: 0.1 -->
# 020c_debug_section_dev_only — The `debug` config section exists only in `mode: dev`

Status: pending

## Description

**Rule (operator, 2026-11-09): the only difference between the modes is the `debug` section of the config.** In `mode: dev` the `debug` section may be present. In `mode: stage` and `mode: prod` it must be removed entirely. Running the App in dev mode changes nothing else: it never drops migrations or any other database object.

The code does not enforce this today. `Config.validate()` (`src/config/config.go:147`) checks only that `mode` is `dev`, `stage` or `prod`. A `stage` or `prod` config with a `debug` section, for example `debug.auth.disable: true`, is accepted and its debug behavior runs. Only a comment in `config.yml.example:52` states the rule. Task 021 adds more keys under `debug` (`debug.mocks.<provider>`), so the check must exist before it.

**The change:**
- In `mode: stage` or `mode: prod`, a config that has a `debug` key at any level (even an empty `debug:` section) stops the start with a clear error, for example `debug section is allowed only in mode dev (mode is "prod")`.
- In `mode: dev` the `debug` section stays optional and works as today.
- The check reads the raw config map, so a `debug` key with default or zero values is also rejected (a struct-only check cannot see it).

The rule is recorded as a new section of `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` (with § 8 Config files), so the `code-reviewer` agent checks that every new `debug.*` key is dev-only.

Points for the design step:
- The `devdb` commands run only in `mode: dev` (`src/cmd/devdb/devdb.go:41`). This is a second difference between the modes. Decide with the operator: `devdb` stays dev-only as a development tool, or the check goes.
- Hot reload: `app_hot_configs` rows for `debug.*` keys (if a `debug` field ever becomes hot) must also be rejected outside dev.

## Definition of Done

- `mode: stage` and `mode: prod` with a `debug` section (full, partial or empty) stop the start with the error above; `mode: dev` with and without a `debug` section starts as today. A test covers each case.
- A positive control: with the check removed (scratch change), the stage and prod tests fail.
- `config.yml.example` says the rule next to the `debug` section; the new conventions section is in `PROJECT_SPECIFIC_CONVENTIONS.md`.
- Build, vet and tests (with `-race`) pass. The docs and the CHANGELOG are updated in the same commit.
