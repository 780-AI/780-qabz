<!-- VERSION: 0.1 -->
# 007_make_compose_actions — `make compose` takes a docker compose action

Status: pending

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
