<!-- VERSION: 0.1 -->
# 004_use_yml_config — Use a YAML config file

Status: pending

## Description

The App's config loader gets a YAML file as its source. The App uses this source by default. No other source (ENV, for example) is used for the time being. How the App is given the path of the YAML file needs a design step.

A `Config` struct field is mapped to a YAML entry with the tag `path`, a dotted path into the YAML tree. Example: ``PgHost string `path:"database.postgres.host"` ``.

A committed example file (`config.example.yml`) shows every entry and is kept in source control. The real file is git-ignored. `.env.example` is removed.

The task comes with a good amount of unit tests. The tests cover YAML keys of different types and depth: flat keys, a single key, arrays, maps, and several levels of nesting.

## Definition of Done

- When the App runs, it loads all the config entries of the passed YAML file.
- A `Config` field with the tag `path:"a.b.c"` gets the value at that path of the YAML tree.
- The App uses the YAML source by default. No other source is used.
- A committed `config.example.yml` shows all entries; the real config file is git-ignored; `.env.example` is removed, and the docs that referred to it are updated.
- Unit tests cover: flat keys, a single key, nested maps (several levels), arrays (of scalars and of maps), maps with different value types (string, int, bool, duration, float), a missing path, a wrong type, and an empty file.
- The tests keep the existing behavior of the hot-reload tag, the migrations and the secret masking.
- `go mod tidy && go mod vendor`, `go build ./...` and `go vet ./...` exit 0.
- `docs/folder_structure.md`, the manifest and the CHANGELOG are updated in the same commit.
