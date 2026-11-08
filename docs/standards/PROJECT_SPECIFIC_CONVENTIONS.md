<!-- VERSION: 0.15 -->
<!-- Last Updated: 2026-11-08 -->
<!--
CHANGE LOG
v0.15 — 2026-11-08 — Section 12: metrics for every feature. A feature metric counts each outcome that the `code` label cannot tell apart; metrics registered only when on; fixed-set labels; `system.html` row and tests; the `artifacts_agent` checks it.
v0.14 — 2026-11-08 — Section 11: coverage gate. Each changed Go package has 70.0% statement coverage or more, measured with the database tests included.
v0.13 — 2026-11-08 — Section 10: every fatal or critical error that is not a common error MUST be logged (one error line with the cause); common errors are not logged.
v0.12 — 2026-11-07 — Section 9: every recorded duration (metric, log field) is in milliseconds; metric names end with `_ms`.
v0.11 — 2026-11-07 — Section 8: layout examples no longer name `auth.jwt.*` (task 011 removed its only key).
v0.10 — 2026-11-07 — Section 8: every config key is in both `config.yml` (local, git-ignored) and `config.yml.example` (tracked, default or zero values); the two stay in sync.
v0.9 — 2026-11-07 — Section 7: the Postman collection `qabz.postman_collection.json` in the repo root is updated in the same commit as every HTTP API change.
v0.8 — 2026-11-04 — Section 6: `pb` submodule ownership. Only `pb/proto/qabz/` is writable; every other path in `pb/` is read-only for this repo.
v0.7 — 2026-11-04 — Section 5: standard library first. Do not write a function that the Go standard library already has.
v0.6 — 2026-11-04 — Section 4: nil safety. Do not use a value as non-nil without an `if` check or clear evidence in the code; tests stop after a failed guard check.
v0.5 — 2026-11-02 — Section 3: test assertions MUST use `github.com/stretchr/testify/assert`.
v0.4 — 2026-11-02 — Section 2: all Go code and `go.mod` live in `src/`; Go commands run there.
v0.3 — 2026-11-01 — Section 1: `encoding/json` (v1) is forbidden everywhere; the `code-reviewer` rejects it.
v0.2 — 2026-11-01 — Section 2: Go modules (`go mod tidy && go mod vendor`; `vendor/` git-ignored).
v0.1 — 2026-11-01 — Created. Definitions section, not numbered (hot path, memory pool, `Poolable`). Section 1: JSON unmarshalling policy (moved out of Implementation_Plan.md Step 5).
-->

# PROJECT_SPECIFIC_CONVENTIONS.md — Qabz conventions

This file is owned by the project. It is not part of the Standard DocSet suite, so a suite sync does not change it.
It is binding for all code in this repository (linked from `CLAUDE.md` § 1). A new convention gets its own numbered section here. The "Definitions" section is not numbered.

---

## Definitions

Terms that more than one rule uses are defined here. A rule that uses a term links to the "Definitions" section and does not define the term again.

**Hot path.** A hot path is code that runs for each request, message or processing cycle. The CPU runs these regions all the time. Examples: handlers, middleware, queue consumers, cron loops, and parsing of provider responses.

**Not a hot path.** Code that runs one time or rarely is not hot. Examples: reading the config, opening connections and other mapping at bootstrap.

**Doubt.** If it is not clear whether code is hot, the author states the reason in the change and the reviewer decides (Rule 10).

**Memory pool.** A memory pool is a pool of objects, built with `sync.Pool`, that keeps a preallocated list of objects of one specific kind.

An object type can be used in a memory pool only if it meets these conditions:
- It implements the `Poolable` interface (below).
- It has a `Reset()` function. `Reset()` sets every field of the object (the struct) to the zero value of the field's type.
- It has a unit test for `Reset()`. This is mandatory, with no exception for any type used in a memory pool. The test sets every field to a non-zero value, calls `Reset()`, and checks that the object equals its zero value. The test proves that every field is in `Reset()`.
- `Reset()` does not use the `reflect` package. `reflect` is a performance killer. This is the reason the unit test is needed: nothing else guarantees that a new field is added to `Reset()`.

**Poolable.** `Poolable` is an interface that has one function only: `Reset()`.

```go
type Poolable interface {
	Reset()
}
```

Every type used in a memory pool implements `Poolable`. The pool code accepts and returns `Poolable` values, so a type without `Reset()` does not compile in a pool. The package that holds the interface is not decided yet.

---

## 1. JSON unmarshalling

This rule covers unmarshalling only. It does not cover marshalling. It uses the term "hot path" from the "Definitions" section.

**In a hot path:** unmarshal with `github.com/valyala/fastjson`.
- Take the parser from a `fastjson.ParserPool` and return it after use.
- Do not keep a parsed value after the parser goes back to the pool.

**Forbidden.** The standard `encoding/json` (v1) is illegal everywhere in the repository, in a hot path and outside it, in code and in tests. The `code-reviewer` agent finds every import of it and rejects the change.

**Outside a hot path:** unmarshal with the standard `encoding/json/v2`. This covers work that runs one time, for example reading the config at bootstrap.

**Review.** Each change that reads JSON states whether the read is on a hot path. The reviewer checks that statement (Rule 10).

**Check on 2026-11-01:** `encoding/json/v2` compiles with Go 1.27.1 and needs no `GOEXPERIMENT` flag. The module Go version must support `encoding/json/v2`.

**Open point:** the rule has no automatic check yet (for example a lint rule that bans `encoding/json` in hot-path packages). To be decided when the first Go code lands.

---

## 2. Go modules

**All Go code is in `src/`.** `go.mod`, `go.sum`, `vendor/`, `main.go` and every code folder are in `src/`. Run every Go command from `src/`.

**Always run `go mod tidy && go mod vendor`.** Never run `go mod tidy` alone. Run both commands together, in this order, after every change of the module requirements or of the imports.

**`src/vendor/` is git-ignored.** The `vendor/` directory is never committed. `.gitignore` lists `vendor/`. `go.mod` and `go.sum` are committed.

**Fresh clone.** Because `vendor/` is not in git, a fresh clone has no `vendor/`. Run `go mod tidy && go mod vendor` before the first build. The generated code in `src/api/pb/` is git-ignored too, so run `make proto` first. Otherwise `go mod tidy` removes the requirements that the generated code needs.

**Builds.** A Go build uses `vendor/` when it exists. CI and Docker builds run `go mod tidy && go mod vendor` before they build.

---

## 3. Test assertions

**Assertions MUST use `github.com/stretchr/testify/assert`.** Do not write manual `if got != want { t.Errorf(...) }` checks in place of an assertion. The `code-reviewer` agent rejects a test that does not use `assert` for its assertions.

---

## 4. Nil safety

**Do not assume that a value is non-nil.** Use a value that can be nil only after one of these is true on the same code path:
- An `if` check proves that the value is not nil (or, for an index, that the length is sufficient).
- The code gives clear evidence that the value cannot be nil. Example: a direct assignment such as `p := &T{}`, `m := make(map[string]int)` or `s := []int{1, 2}` in the same function, with no later assignment that can set it to nil.

A value returned from a function is not evidence. Check it, even when the function "always" returns a value. The function can change later.

**This covers:**
- Dereferencing a pointer, or reading a field through a pointer.
- Calling a method on a pointer or an interface value. This includes `err.Error()`: call it only after a check shows that `err` is not nil.
- Indexing a slice or an array pointer (`res[0]`). Check the length first.
- Writing to a map. Writing to a nil map panics. Reading from a nil map is safe.
- A type assertion. Use the two-value form (`v, ok := x.(T)`) and check `ok`, unless a failed assertion must panic.

**In tests.** A failed testify `assert` call does not stop the test. The next line runs. If that line uses a value that the failed check protected, the test panics. The panic stops all other tests in the package and hides the real failure. So, when a later line depends on a check, stop the test if the check fails. Every `assert` function returns `true` on a pass; use that value:

```go
// Wrong: if err is not nil, res can be nil or empty, and res[0] panics.
res, err := SomeCall(testMock)
assert.NoError(t, err)
assert.Equal(t, "MockValue", res[0].Name)

// Correct: the test stops when a check that a later line needs fails.
res, err := SomeCall(testMock)
if !assert.NoError(t, err) || !assert.Len(t, res, 1) {
	return
}
assert.Equal(t, "MockValue", res[0].Name)
```

- The same applies to the error case: after `assert.Error(t, err)`, call `err.Error()` only inside `if assert.Error(t, err) { ... }`.
- In a test helper that returns a value to the test, call `t.FailNow()` when the guard check fails. Do not return a nil value to the caller.
- A check that no later line depends on does not need a guard. Let it fail and let the test continue, so one run shows all failures.

**Review.** The `code-reviewer` agent reads each changed use of a value that can be nil. A use with no `if` check and no clear evidence is a defect.

---

## 5. Standard library first

**Before you write a function (a helper, a utility or any other function), check that the Go standard library does not already have it.** If it does, use the standard library. Example: use `net/url` to escape a URL, not a hand-written escaper.

**Review.** The `code-reviewer` agent rejects a new function that copies a function of the standard library.

---

## 6. `pb` submodule ownership

**Only `pb/proto/qabz/` belongs to this project.** The `pb` submodule is a shared repository. Other teams own every other path in it.

**Writable:** `pb/proto/qabz/` and its contents only.

**Read-only — do not touch:** every other path in `pb/`. This includes every other folder in `pb/proto/` (for example `pb/proto/bill/`, `pb/proto/commons/`), `pb/aux/`, `pb/testdata/`, `pb/README.md` and `pb/azure-pipelines.yml`.
- Do not create, edit, delete, rename, move or reformat a file there.
- Do not write tool output there. Generated code goes to `src/api/pb/` and `src/api/swagger/`, never into `pb/`.
- A commit in the `pb` repository contains paths under `proto/qabz/` only.

**Reading is allowed.** `make proto` reads `pb/proto/` and `pb/aux/` as `protoc` import paths. A proto in `pb/proto/qabz/` can import a proto from another folder. Reading does not change a file.

**Need a change outside `pb/proto/qabz/`?** Do not make it from this repo. Ask the operator. The owner of that folder makes the change.

**Check.** Run from the repo root. The check passes when it prints nothing (`grep` exits 1):

```bash
{ git -C pb diff --name-only --no-renames "$(git rev-parse HEAD:pb)"; \
  git -C pb ls-files --others --exclude-standard; } | grep -v '^proto/qabz/'
```

The first command lists each file changed since the `pb` commit that this repo records (committed or not). The second lists new untracked files. Each printed path is outside `proto/qabz/` and is a defect.

**Review.** The `code-reviewer` agent runs the check on every review. A printed path is a defect.

---

## 7. Postman collection

**The Postman collection `qabz.postman_collection.json` lives in the repository root. It MUST stay up to date with the HTTP API.** Every change to the API updates the collection in the same commit. The collection uses the Postman Collection v2.1 format.

**An API change is any change that a client of the HTTP API (the grpc-gateway) can see:**
- A new endpoint, or a removed endpoint.
- A changed HTTP method or path (the `google.api.http` rule in `pb/proto/qabz/`).
- A changed request or response: a field added, removed or renamed, a changed type, a changed required field, a changed example body.
- Changed headers, query parameters, authentication, or status codes.
- A changed default listen address (`server.http.addr`), which changes the `baseUrl` variable.

**What the collection contains:**
- One request for each implemented endpoint, grouped in folders by gRPC service (for example `Service`). An endpoint that only returns Unimplemented (HTTP 501) is added when it is implemented.
- The URL uses the variable `{{baseUrl}}`. Do not write a host or a port in a request.
- A request with a body has a valid example body.
- Each request has a short description: the gRPC method name and the expected result.
- No secret: no real password, token or key. Use a variable (for example `{{token}}`) and leave its value empty.

**Check.** The file is valid JSON:

```bash
python3 -m json.tool qabz.postman_collection.json >/dev/null && echo OK
```

**Review.** The `code-reviewer` agent checks each change to `pb/proto/qabz/`, `src/entry/http/` and `src/entry/grpc/`. If the change is an API change and the collection is not updated in the same change, that is a defect. The verdict is NOT READY.

## 8. Config files

**Every config key MUST be in two files in the repository root, and the two files MUST have the same keys:**
- `config.yml`: the real config of the local run. It is git-ignored and is never committed. It holds the real values, secrets too.
- `config.yml.example`: the template. It is committed. Each key has its default value (the value in `defaults()` in `src/config/config.go`), or the zero value (`""`, `0`, `false`) when the key has no default. It has no real secret, host, token or key.

**A config change updates both files in the same change:** a new key, a removed or renamed key, or a changed default. A new key is a `path` tag on a `Config` field in `src/config/config.go`. A secret key uses the `Secret` type, so its value never prints.

**Layout.** Auth secrets are under `auth.secrets` (for example `auth.secrets.hmac`). Other keys follow the area they configure (`database.*`, `server.*`, `metrics.*`, `debug.*`). Do not put a config value in a separate file.

**Check.** The tests in `src/config/example_test.go` fail when:
- `config.yml.example` and the `Config` path tags do not have the same keys.
- A value in `config.yml.example` is not the default or zero value.
- The local `config.yml` and `config.yml.example` do not have the same keys. This test is skipped where `config.yml` does not exist, for example in CI.

**Review.** The `code-reviewer` agent checks each change to `src/config/`. A config change without the matching change to `config.yml.example` is a defect. The verdict is NOT READY.

## 9. Durations in milliseconds

**Every duration that the code records MUST be in milliseconds.** This covers metrics (histograms and gauges of a time, for example a latency or an age) and measured durations in log fields.

- A metric name ends with `_ms` (for example `qabz_request_duration_ms`), not `_seconds`.
- Record the value as a float: `float64(d) / float64(time.Millisecond)`. Do not use `d.Milliseconds()`: it returns an integer and drops the part under 1 ms (a 250 µs call becomes 0).
- Histogram buckets are in milliseconds (for example `5, 10, 25, ..., 10000`).

**Not covered:**
- Config values: Go duration strings such as `"30s"` and `time.Duration` fields in `Config`.
- The default metrics of the Prometheus client (`go_*`, `process_*`). They keep their own units.

**Why.** Operator decision, 2026-11-07 (task 014). It deviates from the Prometheus base-unit naming (`_seconds`): dashboards and alert rules for `qabz` read milliseconds.

**Review.** The `code-reviewer` agent checks each new or changed metric and log field that holds a duration. A duration not in milliseconds, or a duration metric whose name does not end with `_ms`, is a defect. The verdict is NOT READY.

## 10. Logging of fatal and critical errors

**Every fatal or critical error that is not a common error MUST be logged.** The code writes one log line at level `error` (or higher) with the cause. A failure that the client or the operator sees must never be silent.

**Fatal or critical errors (log them):**
- A failed call to a dependency in a request path: the database, the cache, a provider, another service. The client gets gRPC `Internal` (HTTP 500) or another server-fault code, with no detail.
- An unexpected fault in the code: a broken invariant, a nil result where a value is required, a panic that a recovery middleware catches.
- A failure that stops the start or ends the process: a missing or bad config value, a failed connection or migration, a listener that cannot bind. Before the logger exists, print the error to stderr in place of a log line.
- A failed background job (a cron run, a consumer message) that is not retried.

**Common errors (do not log them):**
- A client error: an invalid request (`InvalidArgument`), a rejected token (`PermissionDenied`), a missing row (`NotFound`), a duplicate.
- A call that the client canceled, or a deadline that the client set.
- These are counted by metrics (`qabz_request_duration_ms{code}` and the feature's own metrics). A log line for each of them fills the log and hides the real faults.

**How to log:**
- One line per failure, at the place where the error becomes the answer (the handler) or ends the work. Do not log the same error again in each layer.
- The message names the feature and the action (for example `mybill: insert failed`), and the key `error` holds the cause.
- The cause is never sent to the client: the answer has no detail (for example `internal error`).
- No secret in a log line: no password, token, key or full DSN (`Secret` values print `***`).

**Check.** Every answer with a server-fault code (`Internal`, `Unknown`, `DataLoss`, `Unavailable`) has a log line on the same path. A test checks the log line for each such path (for example `TestInsertErrorIsInternal`: one line, the message and the cause), and a test checks that a client error writes no line.

**Why.** Operator decision, 2026-11-08 (task 016). An HTTP 500 with no log line cannot be investigated. Task 013 first returned 500 for a failed database read with no log line (lesson candidate of 2026-11-07); this rule makes the practice binding.

**Review.** The `code-reviewer` agent finds each server-fault answer and each fatal start path. A path with no log line, or a common error that writes a log line, is a defect. The verdict is NOT READY.

## 11. Coverage gate

**The statement coverage of each changed Go package MUST be 70.0% or more, measured with the database tests included.** 69.9% fails. Do not round up.

**Changed package.** A package that has a new or modified non-test `.go` file. A package with only changed test files is not gated.

**How to measure:**
- Set `QABZ_TEST_POSTGRES_DSN` to a real Postgres (the compose Postgres), so that the live (database) tests run. Never print the password.
- Run `go test -race -count=1 -v -coverprofile=<file> <package>` in `src/`, then read the total with `go tool cover -func=<file>`. Keep the profile outside the repo.
- Check in the `-v` output that the live tests ran and did not skip. A unit-only number (no DSN, live tests skipped) is not the gate value.
- If no database is available, the gate is "not measured". A change cannot pass a gate that was not measured.

**Not counted:** generated code (`api/pb/`, `api/swagger/`), the `pb/` submodule and test files. A package with no tests has 0% and fails.

**Why.** Operator decision, 2026-11-08 (task 019). The repository code of `system/postgres` is mostly tested by the live tests on a throw-away schema; a unit-only measurement reported 67.7% for a package at 89.0% with its database tests, and failed a change that had raised the coverage.

**Review.** The `code-reviewer` agent measures each changed package as above and reports package, coverage and PASS or FAIL. A FAIL, or a gate that was not measured, is a defect. The verdict is NOT READY.

## 12. Metrics for every feature

**Every added or changed feature MUST be measurable with Prometheus metrics.** A feature is a gRPC method or HTTP endpoint, a call to a dependency (the database, the cache, a provider, another service), a background job (a cron, a consumer, a worker), or a new start path.

**Request metric.** Every gRPC method and HTTP endpoint is counted by `qabz_request_duration_ms` (the `Requests` interceptor in `src/entry/grpc/requests.go`). A new service is registered on the gRPC server that has this interceptor. Its `code` label already counts client errors and server faults.

**Feature metric.** A feature whose outcomes the `code` label cannot tell apart has a feature metric that counts each outcome. Examples: `created` or `existing` (`qabz_mybill_insert_total`), `found` or `not_found` (`qabz_mybill_delete_total`), a cache `hit`, `miss` or `error` (`qabz_services_cache_total`).
- A plain read whose only outcomes are success or a gRPC error (for example `GET /v1/mybill`) needs no feature metric. The author states this in the change.
- Each outcome that the code can reach is counted once, on the path where it happens. A reachable outcome that counts nothing is a defect.

**Dependency calls and jobs.** A call to a provider or another external service has a latency histogram (`_ms`, § 9) and a call and error counter. A background job has a run counter by result and a duration histogram. Use the names planned in `docs/initial-discovery/system.html` (for example `qabz_provider_call_duration_ms`, `qabz_provider_calls_total`, `qabz_cron_runs_total`, `qabz_cron_duration_ms`).

**How to define a metric** (the pattern of `newMyBills` in `src/app/defaults.go`):
- Create and register the metric only when `metrics.Registry()` is not nil. When metrics are off (`metrics.prometheus.enabled: false`), the callback is nil and the feature records nothing.
- Return a registration error with the feature prefix (for example `mybill: register metric: %w`).
- The name starts with `qabz_`. A counter ends with `_total`. A duration follows § 9 (`_ms`, a float in milliseconds).
- The `Help` text names the endpoint or job and each label with its values.
- Each label value comes from a small fixed set. Never use a user ID, a bill ID, a phone number, an amount, an error text or any other free input as a label value: the number of series has no limit.

**Design doc.** Each new metric gets a row in the "Logs, Metrics and Alerts" tables of `docs/initial-discovery/system.html` in the same commit (a Critical File, `CLAUDE.md` § 4). A metric that `system.html` plans for the feature area is implemented with the feature, or the task Description says that it comes later.

**Tests.**
- The metric name is in the registered-metrics list of `src/app/defaults_test.go`. `TestMetricsOffRegistersNothing` then checks that nothing is registered when metrics are off.
- A test checks the value of the metric for each outcome label value.

**Why.** Operator decision, 2026-11-08. `system.html` names the lack of custom metrics as the main gap of the current system. The pattern exists in tasks 013, 014, 016 and 019 but was not binding, so a new feature could ship without a way to measure it.

**Review.** The `artifacts_agent` agent checks each feature of the change against this section, with § 7 (Postman) and § 10 (logging). A missing feature metric, a free-input label, a metric not registered behind the switch, a missing `system.html` row or a missing test is a defect. The verdict is NOT READY.
