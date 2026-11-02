<!-- VERSION: 0.4 -->
# config_hot_reload_003 — Config with hot-reload

Status: done

## Description

A migration creates the table `app_hot_configs` with these columns: `id`, `key` (`varchar(256)`), `cnf_type` (`varchar(64)`), `value` (`text`), `created_at`, `updated_at`. The types of `id`, `created_at` and `updated_at` and the allowed values of `cnf_type` need a design step first, as do the failure behavior and the consumer API.

Add hot-reload to `Config`. A field of the `Config` struct that has the tag `hot_reload:"true"` is read again from its source, which is Postgres, at a given interval. Fields without the tag are read one time at start, as today. The interval is the `Config` entry `hot_reload_interval`. It is required and it is not hot itself.

A `ConfigRepository` loads the hot configs from the table `app_hot_configs`. Only hot-reloadable configs exist in this table, never all configs. A row whose key is not a tagged field is ignored. When a value changed, the new value is assigned to the `Config` entry. Because the `Config` object is changed while the service runs, access to its values is guarded by a locking mechanism, so a concurrent read never panics or sees a half-written value.

## Definition of Done

- A migration creates the table `app_hot_configs` with the columns `id`, `key` `varchar(256)`, `cnf_type` `varchar(64)`, `value` `text`, `created_at`, `updated_at`.
- `Config` has the required entry `hot_reload_interval`.
- A `ConfigRepository` interface and a Postgres implementation load the rows of `app_hot_configs`.
- A `Config` field with `hot_reload:"true"` is reloaded at `hot_reload_interval`. A changed value is assigned to the entry. A field without the tag is not reloaded.
- Reads of `Config` values are safe while a reload runs. A test with `-race` proves it.
- A failed reload keeps the last good value and writes a log line, without secrets.
- Tests prove that a changed database value reaches the field within one interval, and that an untagged field does not change.
- `go mod tidy && go mod vendor`, `go build ./...` and `go vet ./...` exit 0.
- `docs/folder_structure.md`, the manifest and the CHANGELOG are updated in the same commit.
