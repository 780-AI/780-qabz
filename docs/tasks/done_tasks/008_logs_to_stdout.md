<!-- VERSION: 1.0 -->
# 008_logs_to_stdout — App logs go to stdout

Status: done

## Description

The App logger (`src/common/logging/zap/zap.go`) uses the zap production default, so every log line, info too, goes to stderr. Log tools and the VSCode Debug Console show stderr lines as errors. The team's other service (`traffick`, `pkg/logger/zap_dto.go`) writes its console logs to stdout. Send all App log lines to stdout. The JSON format and the levels stay the same.

## Definition of Done

- All App log lines (every level) go to stdout; none go to stderr.
- zap's own internal errors (a failed write) stay on stderr.
- Process-level errors that are not log lines (`app: ...` in `cmd/start-app`, the `migrate` errors and usage) stay on stderr; `migrate version` keeps its result on stdout.
- A unit test shows a log line on stdout and nothing on stderr.
- Build, vet and tests pass; the App started with `--config` shows its log lines on stdout (feature ON, Rule 15).

## Decisions (operator, 2026-11-07)

- Use stdout, like `traffick`. A fixed rule, not a config key.
- Kept as is: the JSON format and the levels. `traffick` also sends logs to Logstash (ECS); that is not part of this task.

## Result (2026-11-07)

- `zap.go`: `OutputPaths` is `stdout`; `ErrorOutputPaths` stays `stderr`.
- New tests `TestNewWritesToStdout` (4 levels on stdout, one JSON line each, stderr empty) and `TestNewLevelFilters`. A deliberate failure (an overlay that sets `stderr`) fails `TestNewWritesToStdout`.
- `go build ./...`, `go vet ./...` and `go test -race -count=1 ./...` pass.
- Feature ON: the App with a copy of `config.yml` on free ports wrote 8 log lines (`postgres ok` to `app stopping`) to stdout and 0 bytes to stderr; `GET /v1/greeting` gave 200; no password in the log.
