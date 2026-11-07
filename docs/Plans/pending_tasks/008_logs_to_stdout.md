<!-- VERSION: 0.1 -->
# 008_logs_to_stdout — App logs go to stdout

Status: pending

## Description

The App logger (`src/common/logging/zap/zap.go`) uses the zap production default, so every log line, info too, goes to stderr. Log tools and the VSCode Debug Console show stderr lines as errors. The team's other service (`traffick`, `pkg/logger/zap_dto.go`) writes its console logs to stdout. Send all App log lines to stdout. The JSON format and the levels stay the same.

## Definition of Done

- All App log lines (every level) go to stdout; none go to stderr.
- zap's own internal errors (a failed write) stay on stderr.
- Process-level errors that are not log lines (`app: ...` in `cmd/start-app`, the `migrate` errors and usage) stay on stderr; `migrate version` keeps its result on stdout.
- A unit test shows a log line on stdout and nothing on stderr.
- Build, vet and tests pass; the App started with `--config` shows its log lines on stdout (feature ON, Rule 15).
