<!-- VERSION: 0.1 -->
# 005_yml_to_env_generator — YAML to env generator

Status: pending

## Description

An env file is generated from the YAML config automatically. This must happen: a developer never writes the env file by hand. The generated file has a name that shows the developer it is auto-generated, and a header comment that says so and says not to edit it. The generated file is git-ignored.

The first user of the generated file is `docker-compose.yml`, which reads the Postgres variables (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_PORT`) and the Redis port from an env file. With task `004_use_yml_config`, `.env.example` is removed, and the YAML is the one source of these values. The repository already has a converter, `scripts/yml_to_env/yml_to_env.sh`; the design step decides whether to reuse it. The generator must map the YAML paths to the env names (for example `database.postgres.password` becomes `POSTGRES_PASSWORD`).

Points for the design step: the exact file name, when the generation runs (for example a `make` target that compose uses, or a step before `docker compose up`), what happens when the YAML file is missing, and which YAML keys are written.

## Definition of Done

- A generator creates the env file from the YAML config. It runs automatically before `docker compose` needs the file, so no developer creates the file by hand.
- The generated file has a name that shows it is auto-generated (for example it contains the word `generated`), and its first lines say that it is auto-generated from the YAML and must not be edited.
- The generated file is git-ignored.
- `docker-compose.yml` reads the generated file, and `docker compose up` works from a clean clone with only the YAML config present.
- A missing or invalid YAML file makes the generator fail with a clear message and a non-zero exit. It never writes a partial file.
- Tests cover: the mapping of the YAML paths to env names, the header, quoting of values with spaces and special characters, a missing key, and a missing YAML file.
- `docs/folder_structure.md`, the manifest and the CHANGELOG are updated in the same commit.
