<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-14 — Created and done in the same session (operator request during task 025).
-->
# 040_remove_circuit_breaker_keys — remove the unused providers.tavanir.circuit_breaker keys

Status: done

## Description

The config test `TestLocalConfigInSync` failed on the operator machine: the local `config.yml` has the `providers.tavanir.circuit_breaker` block commented out, and `config.yml.example` has it (task 023 follow-up (c)). No code reads the five keys: no provider has a circuit breaker yet (operator decision of task 034: every provider gets one at the end of the project).

Operator decision (2026-11-14): keep the comment in `config.yml` (a comment is not a key), remove the block from `config.yml.example`, and add a `@todo` for the circuit breaker feature. Conventions § 8 needs `config.yml.example` and the `Config` path tags to have the same keys (`TestExampleHasEveryKey` failed with only the example changed), so the five `Config` fields, their defaults and the `failure_ratio` check go too.

## Definition of Done

- `providers.tavanir.circuit_breaker.*` is not in `Config`, its defaults, its checks, `config.yml.example` and the config example of `system.html`.
- A `@todo` at the Tavanir keys in `src/config/config.go` names the old values and what the circuit breaker feature must add (keys for every provider, both config files, the two planned metrics).
- A `config.yml` that still has the block (commented or not) is valid: a test checks it.
- `make test` passes on the operator machine, `TestLocalConfigInSync` included.

## Verification record (2026-11-14, implementer)

- Positive control: with the five fields kept and the block removed from the example, `TestExampleHasEveryKey` fails; with the fields removed, every `config` test passes, `TestLocalConfigInSync` too.
- `make test GO_TEST_ARGS="-race -count=1"`: see the commit record in `CHANGELOG.md`.
- No runtime behavior change: no code read the keys; the decoder reads only tagged paths, so an old block in a stage or prod `config.yml` is ignored. No feature-ON check (Rule 11 exempt class: no behavior change).
