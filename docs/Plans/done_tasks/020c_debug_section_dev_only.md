<!-- VERSION: 0.3 -->
# 020c_debug_section_dev_only — A mode other than `dev` fully removes the `debug` config section

Status: done

## Description

**Rule (operator, 2026-11-09): a mode other than `dev` fully removes the `debug` section of the config.**

Today the App reads the `debug` section in every mode. For example, `mode: prod` with `debug.auth.disable: true` turns the auth check off.

**The change:** when `mode` is not `dev`, `config.Parse` (`src/config/config.go`) removes the whole `debug` section from the YAML tree before it reads the values. Every `debug.*` field then keeps its default, exactly as if the section were not in the file. A value in the removed section is not checked, so it cannot stop the start. In `mode: dev` the `debug` section is read as today.

## Definition of Done

- With `mode: stage` or `mode: prod`, a config with a `debug` section gives the same `Config` as the same config without the section. With `mode: dev`, the `debug` values are read as today. A test covers each case.
- A positive control: with the removal taken out (scratch change), the stage and prod tests fail.
- `config.yml.example` states the rule next to the `debug` section.
- Build, vet and tests (with `-race`) pass. The CHANGELOG is updated in the same commit.
