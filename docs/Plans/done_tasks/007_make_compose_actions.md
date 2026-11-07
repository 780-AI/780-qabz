<!-- VERSION: 1.0 -->
# 007_make_compose_actions — `make compose` takes a docker compose action

Status: done

## Description

Replace the single `make compose` target (generate `config_generated.env`, then `docker compose up`) with four actions: `make compose up`, `make compose down`, `make compose ps` and `make compose logs`. Each action first generates `config_generated.env` from `config.yml` (as `make compose` does today), then runs `docker compose --env-file config_generated.env <action>`. A plain `make compose` with no action is removed: it prints the usage and fails.

## Definition of Done

- `make compose up|down|ps|logs` generate the env file, then run the matching `docker compose` action. If the generation fails, docker does not run.
- `make compose` with no action, or with an unknown action, prints the usage and exits non-zero.
- `make up` (or `down`, `ps`, `logs`) alone does not run docker silently; it prints the usage and fails.
- Extra docker compose flags pass through `ARGS` (for example `make compose up ARGS="-d --wait"`).
- Docs that name `make compose` say `make compose up`. The CHANGELOG has a QA scenario.

## Verification

With a valid `config.yml`: `make compose up ARGS="-d --wait"`, `make compose ps`, `make compose logs`, `make compose down`. Pass: `postgres` and `redis` come up healthy, `ps` lists them, `logs` prints their logs, `down` removes them. `make compose` alone exits non-zero with the usage.

## Decisions (Claude, 2026-11-07; the operator can change them)

- Each action passes straight to `docker compose`, so `make compose up` runs in the foreground (as the old `make compose` did). Flags go in `ARGS`, because `make` reads a flag such as `-d` as its own: `make compose up ARGS="-d --wait"`.
- `down`, `ps` and `logs` also generate the env file first: `docker compose` reads the variables for every action, and the compose file stops if the Postgres password is not set.
- The action check runs before the generator, so `make compose` with no action does not write the env file.
- The generator header now says `make env`, the target that writes the file again.

## Result (2026-11-07)

All Verification steps pass: `up ARGS="-d --wait"` (both `Healthy`), `ps`, `logs ARGS=--tail=3`, `down` (containers removed, volumes kept). `make compose`, `make compose foo`, `make compose up down` and `make up` print the usage and exit 2. `make compose ps CONFIG=nope.yml` stops at the generator. `make test-scripts`: 18 tests pass.
