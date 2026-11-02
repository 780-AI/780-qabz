<!-- VERSION: 0.1 -->
# app_bootstrap_002 — App bootstrap

Status: pending

## Description

Create an app bootstrap, a type named `App`. Entry points execute it. It manages everything: all long-lived types, dependency management, start and stop. Uber Fx can be used. The first modules are the databases (Postgres, Redis), logs (zap), monitoring (Prometheus, tracing) and `Config`. `Config` is mapped from environment variables (ENV).

## Definition of Done

- `App` exists. It owns the creation, start and stop of the databases, logs, monitoring and `Config`.
- `Config` is read from ENV. Defaults and required values are defined. Secrets are never logged.
- `main.go` has a single line for the entry point.
- The database connections work (checked with a ping). Their status is printed in the log, without passwords.
- Tests cover `Config` mapping and `App` with injected test dependencies.
- `go mod tidy && go mod vendor`, `go build ./...` and `go vet ./...` exit 0.
- `docs/folder_structure.md`, the manifest and the CHANGELOG are updated in the same commit.
