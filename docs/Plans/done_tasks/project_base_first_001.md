<!-- VERSION: 0.1 -->
# project_base_first_001 — folder structure, main file and base packages

Status: done

## Description

Create the folder structure (all code in `src/`) from "Directory Structure" in `docs/initial-discovery/system.html`. Add a hello-world `main.go` in `src/`. `cmd/` holds entry points, not `main.go`. Add the base packages to `go.mod`: Postgres (`pgx`), Redis (`go-redis`), Prometheus (`client_golang`), `zap`, and tracing for Jaeger (OpenTelemetry with the OTLP exporter). No business code. The main file only prints hello world, and it imports the packages so that `go mod tidy` keeps them.

## Definition of Done

- All folders from "Directory Structure" exist. Empty folders hold a `.gitkeep`.
- All Go code, `go.mod`, `go.sum` and `main.go` are in `src/`. `src/main.go` prints hello world. `cmd/` has no `main.go`.
- `go.mod` requires the five base packages. `go mod tidy && go mod vendor` exits 0.
- `go build ./...` and `go vet ./...` exit 0. `go run .` prints hello world.
- `docs/folder_structure.md`, the manifest and the CHANGELOG are updated in the same commit.
