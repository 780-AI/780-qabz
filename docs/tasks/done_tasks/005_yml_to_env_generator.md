<!-- VERSION: 1.0 -->
# 005_yml_to_env_generator — YAML to env generator

Status: done

## Description

An env file is generated from the YAML config automatically. This must happen: a developer never writes the env file by hand. The default output name is `config_generated.env`, a name that shows the developer it is auto-generated. The script also accepts a custom output (a path, a directory or a file name) as an argument; without it, the script writes `config_generated.env`. A header comment at the top says that the file is auto-generated from the YAML, that it must not be edited, and that it is mainly used by `docker-compose`. The generated file is git-ignored.

The first user of the generated file is `docker-compose.yml`, which reads the Postgres variables (`POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `POSTGRES_PORT`) and the Redis port from an env file. With task `004_use_yml_config`, `.env.example` is removed, and the YAML is the one source of these values. The repository already has a Bash converter, `scripts/yml_to_env/yml_to_env.sh`. The generator is a rewrite of it in Python: the code is more readable, and it handles the various types of YAML data (nested maps, arrays, booleans, numbers, durations, strings with special characters) better than Bash. The Bash converter and its Bash tests are replaced by the Python generator and its Python tests, and are removed in the same commit. The generator must map the YAML paths to the env names (for example `database.postgres.password` becomes `POSTGRES_PASSWORD`).

A `make compose` target runs two steps, in this order: (1) autogenerate `config_generated.env` (the default name) from the YAML, (2) `docker compose up`. A developer starts the stack with this target only.

Points for the design step: which YAML keys are written, how `make compose` is given the YAML path, the name of the output argument (for example `--output`) and what it accepts when the value is a directory, how the Python generator parses YAML (a parser library such as PyYAML, which then needs a pinned requirement and a documented install step, or a parser without a dependency), and how the options of the Bash converter (for example `-u` / `--upper-case`) carry over. Python is not in the project's primary languages in `CLAUDE.md` § 1 (Go, Bash) today. The design step decides whether `CLAUDE.md` § 1 and `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md` are updated; an edit of `CLAUDE.md` is a Major change (Critical File) and needs its own operator approval.

## Definition of Done

- The generator is a Python script, a rewrite of `scripts/yml_to_env/yml_to_env.sh`. The Bash converter and its tests are removed, with their manifest and `docs/folder_structure.md` entries, in the same commit. The Python files carry a VERSION header (`CLAUDE.md` § 7).
- A generator creates `config_generated.env` from the YAML config. No developer creates the file by hand.
- `config_generated.env` is the default output. The script also accepts a custom output path/name; the header comment and the "never a partial file" rule apply to it too.
- `make compose` runs two steps, in this order: (1) autogenerate `config_generated.env`, (2) `docker compose up`. If step 1 fails, step 2 does not run.
- The default file name is `config_generated.env`. Its first lines (a comment) say that it is auto-generated from the YAML, that it must not be edited, and that it is mainly used by `docker-compose`.
- `config_generated.env` is git-ignored (the default output).
- `docker-compose.yml` reads `config_generated.env`, and `make compose` works from a clean clone with only the YAML config present.
- A missing or invalid YAML file makes the generator fail with a clear message and a non-zero exit. It never writes a partial file.
- Tests cover (Python tests): the mapping of the YAML paths to env names, values of different types (string, int, bool, duration, float, array, nested map), the header (including the docker-compose sentence), quoting of values with spaces and special characters, a missing key, a missing YAML file, the default output name, and a custom output path/name.
- `docs/folder_structure.md`, the manifest and the CHANGELOG are updated in the same commit.

## Verification

In a tmp folder, run the script for a custom multi-level YAML file. Check that the output ENV file is in the target path (tmp), and that it contains the expected key names with their values.
