<!-- VERSION: 0.1 -->
# config_hot_reload_003 — Config with hot-reload

Status: pending

## Description

Add hot-reload to `Config`. A field of the `Config` struct that has the tag `hot_reload:"true"` is read again from its source, which is the database, at a given interval. Fields without the tag are read one time at start, as today. The interval is a setting. The tag name and the interval setting are the only design decisions in this task text; the table layout, the failure behavior and the consumer API need a design step first.

## Definition of Done

- A `Config` field with `hot_reload:"true"` is reloaded from the database at the given interval.
- A `Config` field without the tag is not reloaded.
- Tests prove that a changed database value reaches the field within one interval, and that an untagged field does not change.
- A failed reload keeps the last good value and writes a log line, without secrets.
- `go mod tidy && go mod vendor`, `go build ./...` and `go vet ./...` exit 0.
- `docs/folder_structure.md`, the manifest and the CHANGELOG are updated in the same commit.
