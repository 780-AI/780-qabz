<!-- VERSION: 0.2 -->
# 005_yml_to_env_generator — YAML to env generator

Status: pending

## Description

An env file is generated from the YAML config automatically. This must happen: a developer never writes the env file by hand. The generated file is named `config_generated.env`, a name that shows the developer it is auto-generated. A header comment at the top says that the file is auto-generated from the YAML, that it must not be edited, and that it is mainly used by `docker-compose`. The generated file is git-ignored.

The first user of the generated file is `docker-compose.yml`, which reads the Postgres variables (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_PORT`) and the Redis port from an env file. With task `004_use_yml_config`, `.env.example` is removed, and the YAML is the one source of these values. The repository already has a converter, `scripts/yml_to_env/yml_to_env.sh`; the design step decides whether to reuse it. The generator must map the YAML paths to the env names (for example `database.postgres.password` becomes `POSTGRES_PASSWORD`).

A `make compose` target runs two steps, in this order: (1) autogenerate `config_generated.env` from the YAML, (2) `docker compose up`. A developer starts the stack with this target only.

Points for the design step: which YAML keys are written, how `make compose` is given the YAML path, and whether the generator is the existing converter or a new one.

## Definition of Done

- A generator creates `config_generated.env` from the YAML config. No developer creates the file by hand.
- `make compose` runs two steps, in this order: (1) autogenerate `config_generated.env`, (2) `docker compose up`. If step 1 fails, step 2 does not run.
- The file name is `config_generated.env`. Its first lines (a comment) say that it is auto-generated from the YAML, that it must not be edited, and that it is mainly used by `docker-compose`.
- `config_generated.env` is git-ignored.
- `docker-compose.yml` reads `config_generated.env`, and `make compose` works from a clean clone with only the YAML config present.
- A missing or invalid YAML file makes the generator fail with a clear message and a non-zero exit. It never writes a partial file.
- Tests cover: the mapping of the YAML paths to env names, the header (including the docker-compose sentence), quoting of values with spaces and special characters, a missing key, and a missing YAML file.
- `docs/folder_structure.md`, the manifest and the CHANGELOG are updated in the same commit.
