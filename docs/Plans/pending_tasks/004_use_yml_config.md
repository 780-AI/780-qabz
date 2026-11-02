<!-- VERSION: 0.3 -->
# 004_use_yml_config — Use a YAML config file

Status: pending

## Description

The App's config loader gets a YAML file as its source. The App uses this source by default. No other source (ENV, for example) is used for the time being. The App receives the path of the YAML file with the command-line flag `--config=path/to/config.yml`. The `migrate` command takes the same flag, because it loads the same `Config`. What happens when the flag is missing needs a design step.

All existing keys of the ENV source move to the YAML config: `SERVICE_NAME`, `LOG_LEVEL`, `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`, `METRICS_ADDR`, `OTLP_ENDPOINT`, `HOT_RELOAD_INTERVAL`, `MIGRATE_ON_START` and `MIGRATE_TIMEOUT`. Each key keeps its default and its rule (for example `POSTGRES_PASSWORD` is still required). The YAML file has a section `database`. It has a subsection `postgres` with the Postgres keys (host, port, user, password, db), and a sibling subsection `redis` with the Redis keys (host, port, password). The place of the other keys (`service_name`, `log_level`, `metrics_addr`, `otlp_endpoint`, `hot_reload_interval`, `migrate_on_start`, `migrate_timeout`) needs a design step.

Every hot-reloadable config entry (a field with the tag `hot_reload:"true"`) MUST have a sensible default. This is mandatory: when a hot-reload fails, or the source has no row for the entry, the field keeps a safe value. A hot-reloadable field without a default is a bootstrap error, so the App does not start. How a default is declared needs a design step.

A `Config` struct field is mapped to a YAML entry with the tag `path`, a dotted path into the YAML tree. Example: ``PgHost string `path:"database.postgres.host"` ``.

A committed example file (`config.example.yml`) shows every entry and is kept in source control. The real file is git-ignored. `.env.example` is removed.

The task comes with a good amount of unit tests. The tests cover YAML keys of different types and depth: flat keys, a single key, arrays, maps, and several levels of nesting.

## Definition of Done

- When the App runs, it loads all the config entries of the passed YAML file.
- Every key that `Config` reads from ENV today has a YAML entry, with the same default and the same rules. `Config` no longer reads ENV.
- A `Config` field with the tag `path:"a.b.c"` gets the value at that path of the YAML tree.
- The App receives the YAML path with `--config=path/to/config.yml`; `start-app` and `migrate` both accept it.
- The App uses the YAML source by default. No other source is used.
- The YAML file has `database.postgres.*` and, as a sibling, `database.redis.*`. The loader reads these paths into the `Config` fields.
- Every `hot_reload:"true"` field has a sensible default. A hot field without a default makes the App fail at start. A test proves the failure, and a test proves that a failed reload leaves the default.
- A committed `config.example.yml` shows all entries (every key in the list above); the real config file is git-ignored; `.env.example` is removed, and the docs that referred to it are updated.
- Unit tests cover: flat keys, a single key, nested maps (several levels), arrays (of scalars and of maps), maps with different value types (string, int, bool, duration, float), a missing path, a wrong type, an empty file, and a missing or unreadable `--config` file.
- The tests keep the existing behavior of the hot-reload tag, the migrations and the secret masking.
- `go mod tidy && go mod vendor`, `go build ./...` and `go vet ./...` exit 0.
- `docs/folder_structure.md`, the manifest and the CHANGELOG are updated in the same commit.
