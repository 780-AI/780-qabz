<!-- VERSION: 0.29 -->
<!-- Last Updated: 2026-11-16 -->
<!--
CHANGE LOG
v0.29 — 2026-11-16 — Task 058 (operator, 2026-11-16): § 13 gets one named exception to "one request, one provider": a payment that did not reach its provider for sure (`provider.ErrNotSent`) goes to the next enabled order row with a payment route; never after a timeout or an answer.
v0.28 — 2026-11-16 — Task 057 (operator): new § 18 — every HTTP route of Qabz is `/v1`; no `/v2` and no `V2` method.
v0.27 — 2026-11-16 — Task 050 (operator rule of 2026-11-15): § 10 — every failed step of a background worker writes one error line, also when the next run retries it (a failed read was "counted, no line"); the line has the worker name and the order ID when there is one; a call that the App stop canceled writes none.
v0.26 — 2026-11-15 — Task 051 (operator, decisions D1 to D4): § 8 — an on/off switch is a bool key named `enabled` (the one exception: `debug.auth.disable`), a renamed key goes in `renamedKeys`, checked by tests; § 15 — the mock switch is `debug.mocks.<provider>.enabled`; § 17 — the off switch is `logs.tracing.enabled` (with `logs.tracing.otlp_endpoint`).
v0.25 — 2026-11-15 — Task 047 (operator decisions D4, D10): § 17 — a failed span uses `jaeger.Fail` (the result and the error type), never `span.RecordError`; the gateway makes no HTTP span; the shared hooks that task 047 added; the test helper `jaegertest`.
v0.24 — 2026-11-15 — New § 17 (operator): every added or changed feature is traced with OpenTelemetry spans sent to Jaeger, one trace for each path of execution, from the entry to the last dependency call; the features that exist get their spans in task 047.
v0.23 — 2026-11-15 — Task 045: § 15 names `status_return_type` (`success`, `failed`, `pending`; fixture `provider.MockFixtures.Status`); new § 16 — every Redis key goes through `redis.Client.Key`, so it has `database.redis.prefix` (supertask 043 operator rule).
v0.22 — 2026-11-15 — Task 044 (operator decision D14 (a)): § 15 — one return-type key for each provider method (`return_type` = the inquiry; `<method>_return_type` for another method, for example `pay_return_type` with `success`, `failed`, `pending`); the mock answers a method only when the adapter gives its fixture.
v0.21 — 2026-11-14 — Section 7: the URL variable of the collection is `BASE_URL` (was `baseUrl`; operator); a request to another service (for example the payment service) uses `BASE_URL` with the path of that service; the collection pre-request script adds the web client headers.
v0.20 — 2026-11-14 — Section 8: the provider blocks are in `providers.internal` (our own services) or `providers.external` (third-party services); a new provider goes in the group of its kind (task 041).
v0.19 — 2026-11-11 — Section 13: one request calls one provider; what it returns (also an error or an answer that cannot be used) is the answer; only the circuit breaker of a provider starts a switch (the Provider Switcher), the same for every bill type, with no rule special to a bill type or a provider (operator, task 036).
v0.18 — 2026-11-09 — Section 15: every provider adapter has a development mock (`debug.mocks.<provider>`, `provider.NewMock`), applied in dev mode only (task 021).
v0.17 — 2026-11-09 — Section 14: tests never touch the development database; one throw-away run database per `make test` (`QABZ_TEST_RUN_DSN`), the `postgrestest` guard, `make test-db-clean` (task 020b); the guard checks the database the driver uses (`pgconn.ParseConfig`), no DSN or password on a command line, `config_generated.env` parsed (never sourced as shell), trap before create, `GO_TEST_PKGS` operator-approved, `make test-db-clean` also drops the `qabz_test_` roles and a test login role has a random password (decision 7); nothing outside the `qabz_test_` databases is touched, a role with objects or privileges outside them is kept and the command exits 1, no `REASSIGN OWNED` / `DROP OWNED` (decision 8); the kept line labels each place (`(not a test database)` only next to a non-`qabz_test_` database, a shared object by its catalog) and a 2BP01 with no `pg_shdepend` row has its own line (fix round 4). Section 11: measured with `make test` (the run database), not `QABZ_TEST_POSTGRES_DSN`.
v0.16 — 2026-11-08 — Section 13: providers only through `ProvidersService` (task 020). No other code reads the `providers` table or `postgres.ProviderRepository`.
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
- A changed default listen address (`server.http.addr`), which changes the `BASE_URL` variable.

**What the collection contains:**
- One request for each implemented endpoint, grouped in folders by gRPC service (for example `Service`). An endpoint that only returns Unimplemented (HTTP 501) is added when it is implemented.
- The URL uses the variable `{{BASE_URL}}` (operator, 2026-11-14; was `baseUrl`). Do not write a host or a port in a request. A request to another service that reads a Qabz answer (folder `Payment (external)`) uses `{{BASE_URL}}` and the path of that service (for example `{{BASE_URL}}/payment/...`). No two variables have names that differ only in letter case.
- The collection pre-request script adds the headers of the web client (the `Grpc-metadata-*` device headers, `Origin`, `User-Agent`, ...) to every request; a header that a request sets itself wins.
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

**Providers.** Each provider block is in one of two groups (task 041). `providers.internal.<provider>`: a provider that is one of our own services (for example `irancell`, `jiring_switch`, `parsian`, `payment_server`). `providers.external.<provider>`: a provider that is a third-party service (for example `karashop`, `qabzino`, `tavanir`). A new provider goes in the group of its kind. Only `providers.general` (the settings of `ProvidersService`) is outside the two groups. `TestProvidersGroups` in `src/config/example_test.go` checks it.

**On/off switches (task 051, operator 2026-11-15).** A key that turns a feature on or off is a bool named `enabled` (for example `metrics.prometheus.enabled`, `providers.internal.parsian.enabled`, `debug.mocks.parsian.enabled`, `logs.tracing.enabled`). Never `enable`, `disable`, `is_<x>` or `<x>_enabled`. The one exception is `debug.auth.disable` (operator decision D3). The settings of the feature go next to the switch, in the same block. `TestOnOffKeysNamedEnabled` in `src/config/example_test.go` fails on a path that ends in `.enable`, or in `.disable` (but `debug.auth.disable`). (`is_lockable` is a bill key that Qabz does not read; it is not a switch of Qabz.)

**A renamed key.** A rename adds the old path and the new path to `renamedKeys` in `src/config/config.go`. A config file that still has the old path does not load, and the error names the new path, so a renamed switch is never dropped with no message (task 051 decision D4). Outside `dev` the `debug` section is removed first, so an old `debug.*` key is dropped there as every `debug.*` key is.

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
- **A failed step of a background worker, also when the next run retries it** (operator, 2026-11-15, task 050): a failed page read, a failed read of one order, a failed or timed-out provider call, an answer that the worker cannot use (an unknown status text), a failed report, a failed save, a lost lock, a failed release, a panic. One error line for each, with the worker name, the order ID (when there is one) and the error. A provider answer that is a normal outcome (for example `pending`) is not an error. A call that the App stop canceled writes no line (the run ends).

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
- Start the compose Postgres (`make compose up`) and run the tests with `make test` from the repo root (§ 14). The target makes the throw-away run database and gives its DSN to the tests in `QABZ_TEST_RUN_DSN`, so the live (database) tests run. Do not set the variable by hand.
- Run `make test GO_TEST_ARGS="-race -count=1 -cover -v"` and read the `coverage:` value on the `ok` line of each changed package. To measure one package only, add `GO_TEST_PKGS=./<package>/`. Never print the password.
- Check in the `-v` output that the live tests ran and did not skip. A unit-only number (a plain `go test` with no run database, live tests skipped) is not the gate value.
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

## 13. Providers only through ProvidersService

**`provider.ProvidersService` (`src/provider/selector.go`) is the only way to read providers.** Every other part of the code (order, inquiry, fee, the provider adapters, the Provider Switcher, later endpoints and crons) gets a single provider, a list or any other provider data from this service, through its methods (`PickProvider`, `List`, `Group`) or the `provider.ProviderSelector` interface.

**Forbidden outside the service and its wiring:**
- A query on the `providers` table (raw SQL or any other access) in any package other than `postgres.ProviderRepository` (`src/system/postgres/provider_repository.go`).
- A call to `postgres.ProviderRepository` or to `provider.Repository.ListProviders` from any code other than `ProvidersService` itself. The composition root (`src/app/defaults.go`) only creates the repository and passes it to the service.
- A second cache or copy of the provider list (for example a Redis key or a package map) that is kept apart from the service.

**Allowed:** tests of the repository and of the service; the dev seed SQL of `system/postgres/devdb` (it writes the dev table, it does not read providers for the App).

**Changes at runtime** (order, status) go through the service too (`MoveUp`, `MoveDown`, `Enable`, `Disable`), so every reader sees the same list. A load replaces the list with the database rows (the database wins, operator decision of task 020).

**One request, one provider (operator, 2026-11-11, task 036).** A flow that calls a provider calls the pick of `PickProvider` once per request. What that provider returns is the answer of the request: a success, a business answer, an error, a timeout or an answer that cannot be used. The flow never calls the next provider of the list in the same request (no fallback loop in a flow, an adapter or a handler). A provider is switched only by the Provider Switcher (`system.html`, Provider Switcher), and only when the circuit breaker of that provider detects the condition. The switch is one generic mechanism: no rule for one bill type, one operator or one provider name; the order is the priority of the rows. A wanted order ("provider A first, B next") is set with the priority of the rows, not in code.

**One exception: a payment that was not sent (operator, 2026-11-16, task 058).** The payment step (`order.Payments.Complete`) goes to the next provider only when the payment did not reach the order provider for sure: the adapter error wraps `provider.ErrNotSent` (the switch of the provider is off, it has no address, or gRPC answers `Unavailable` before the request headers went to a connection, `provider.SentHandler`). The next provider is the next enabled row of (bill type, `order`) of `ProvidersService.Group` that has a payment route and an adapter; the step saves an `in-progress` row with it (the same RRN) before the call. A timeout, a cancel, any other error and every answer of a provider stay with the provider (`pending` or the answer): the provider can have paid. The exception is generic (every bill type, no provider name, the order of the rows). No other flow may go to a second provider; a new exception needs an operator decision and a line here.

**Why.** Operator decision, 2026-11-08 (task 020). One sorted, concurrent-safe list gives every caller the same deterministic pick, without a database read on the hot path. A second read path would pick a different provider (`bill` picks any enabled row, with no order) or bypass the runtime changes.

**Review.** The `code-reviewer` agent searches each change for `providers` in SQL text and for uses of `ProviderRepository` and `ListProviders`. A use outside the places above is a defect. A second provider call in one request (other than the exception of the payment step above), or a provider name in a switch or order rule, is a defect too. The verdict is NOT READY.

## 14. Tests never touch the development database

**No test may create, change, drop or reset any object (database, schema, table, row, migration state) in the development database** (`database.postgres.*` in `config.yml`, the compose `qabz` database). Every live (database) test works in the run database of `make test` only.

**Nothing outside the `qabz_test_` databases is touched** (operator rule, 2026-11-09, task 020b decision 8: "Nothing in the databases other than test_* databases must be touched."). No test, Make target or `testdb` verb may create, change, drop, reassign, grant or revoke anything inside a database whose name does not start with `qabz_test_` (the development database `qabz`, `postgres`, the templates, any other). Every connection that writes is to a `qabz_test_` database. The admin connection (database `postgres`) only reads catalogs (`pg_database`, `pg_roles`, `pg_shdepend`) and runs server-level `CREATE DATABASE` / `DROP DATABASE` of `qabz_test_` databases and `CREATE ROLE` / `DROP ROLE` of `qabz_test_` roles. A test that needs a database outside its clean (for example to keep a role) uses the run database with a longer clean prefix of its own; it creates no database without the `qabz_test_` prefix. `DROP OWNED` is not used even in a `qabz_test_` database: it also revokes the privileges of the role on shared objects (other databases).

**One run database for each test run.** `make test` (repo root) is one test run: the whole `go test ./...`. `GO_TEST_PKGS` (operator-approved, 2026-11-09) can name fewer packages for a QA scenario or the § 11 gate; each such run still gets its own random database.
- The target creates ONE database `qabz_test_<16 random hex>` on the Postgres server, runs the tests, and drops it at the end (`DROP DATABASE ... WITH (FORCE)`), also when a test fails or the run is interrupted (Ctrl-C, SIGTERM: the shell trap drops it). The trap is set BEFORE the database is created; when the run stops before the database exists, the drop does nothing (`testdb: no database <name>, nothing to drop`).
- The tool behind the target is `src/system/postgres/postgrestest/testdb` (tests only; `admin-dsn <env file>`, `name`, `create <name>`, `drop <name>`, `clean`). Only the Makefile calls it.
- **No DSN or password on a command line** (any process list shows it). A `testdb` verb takes at most a file path or a database name as its argument; `create`, `drop` and `clean` read the admin DSN from `QABZ_TEST_ADMIN_DSN` in their environment. `create` and `drop` refuse a name that is not `qabz_test_` + letters, digits and `_` before they connect.
- **`config_generated.env` is never sourced as shell** (a password with a quote or a backtick would break it or run a command). `testdb admin-dsn` parses it (the `KEY=VALUE` format of `scripts/yml_to_env`, with its quoting).
- **What a killed run can leave.** A run that is killed so the shell cannot trap it (`kill -9`, a crash), or a test stopped before its `t.Cleanup` (a `-timeout`, a panic), can leave: the run database `qabz_test_<16 hex>`; the extra databases `qabz_test_...` that a test made for itself; the login roles `qabz_test_...` that a test made for itself. A SIGINT while `CREATE DATABASE` runs can also leave the run database (the drop of the trap can start before the server ends the create); `make test-db-clean` removes it.
- **Only `make test-db-clean` removes leftovers.** It drops every database whose name starts with `qabz_test_`, then every role whose name is `qabz_test_` + letters, digits and `_` (the databases first: a role that owns a database cannot be dropped before it). The prefix is matched with `starts_with`, never with a `LIKE` pattern (`_` is a wildcard there). A role that `DROP ROLE` cannot drop (SQLSTATE 2BP01: it still owns objects or has privileges in a database that is not a test database) is KEPT and nothing is changed: `testdb: kept role <name>: it has objects or privileges in <place list>; remove them by hand` (the list is read from `pg_shdepend`, each place labeled for what it is: `database <name>`, with ` (not a test database)` only when the name does not start with `qabz_test_`; a privilege on or the ownership of a database names that database, never the database of the connection; `a shared object (<catalog>)` for another shared object), for example `testdb: kept role qabz_test_r: it has objects or privileges in a shared object (pg_tablespace), database qabz_tmp (not a test database); remove them by hand`. When `DROP ROLE` reports 2BP01 but `pg_shdepend` has no row for the role any more, the line is `testdb: kept role <name>: DROP ROLE reports dependencies but none were found; run make test-db-clean again` (still kept and counted). Then a final `testdb: <n> test role(s) kept`, and the command exits 1 (so `make test-db-clean` fails visibly) after it finished everything else. `testdb` never runs `REASSIGN OWNED`, `DROP OWNED`, `GRANT`, `REVOKE` or `ALTER`. A role without the prefix, and the role of the admin connection, are never touched. It prints `testdb: dropped database <name>` / `testdb: dropped role <name>` for each and the two counts. Nothing removes leftovers by itself. `make test-db-clean` also drops the database of a `make test` that is still running: run it only when no `make test` is running.
- **No usable password in the repository.** A test that makes a login role gives it a password that is new for each run (fixed special characters and random hex from `crypto/rand`), so a role left by a killed run cannot be used with a password written in the code.

**Two DSNs, kept apart:**
- `QABZ_TEST_RUN_DSN`: the DSN of the run database. Only `make test` sets it. A test gets it only from `postgrestest.RunDSN(t)`. Without it the live tests skip (a plain `go test ./...` runs the unit tests only).
- `QABZ_TEST_ADMIN_DSN`: the admin DSN, server only (the maintenance database `postgres`). `testdb admin-dsn` builds it from `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_USER` and `POSTGRES_PASSWORD` of `config_generated.env`; the target gives it only to the `testdb` commands. No test gets it.
- `QABZ_TEST_POSTGRES_DSN` is no longer used. No test reads it, or any other DSN of the development database.

**The guard.** `postgrestest.RunDSN` and every schema helper (`ServicesSchema`, `BillsSchema`, `EmptySchema`, `SeededSchema`) stop the test (`FailNow`) before they connect when the database name of the DSN does not start with `qabz_test_`. The name checked is the one the driver connects to (`pgconn.ParseConfig(dsn).Database`): a `?dbname=` or `?database=` parameter overrides the URL path, and `PGDATABASE` fills a DSN with no name; a check of the URL path alone is a defect. The tool refuses to create or drop a database whose name does not start with `qabz_test_`.

**Inside the run database:**
- The per-test isolation stays: each live test makes its own throw-away schema with a `postgrestest` helper (`qabz_test_<random>`, dropped at the end of the test), because the packages run in parallel in the same run database.
- A test may drop and recreate its OWN objects (its schema, or an extra database or a login role of its own named `qabz_test_...` + letters, digits and `_`, dropped at the end of the test) inside the run. Every database and role a test makes has this name, so that `make test-db-clean` removes it when the test is killed. It never drops, truncates or resets an object that it did not create.
- A test that needs an empty database (for example the migration tests) uses `postgrestest.EmptySchema`.

**What the target needs:**
- The compose Postgres running (`make compose up`) and `config.yml` (the target runs `make env`).
- A role with `CREATEDB` (to create and drop the run database). The compose role (`POSTGRES_USER`) is the superuser of the official image, so it has it (checked on 2026-11-09: `rolsuper = t`, `rolcreatedb = t`).
- No `psql` on the host: the tool uses the Go Postgres driver.

**Check.** A search of the Go code (not `vendor/`) for `DROP`, `TRUNCATE`, `CREATE SCHEMA`, `CREATE DATABASE`, `CREATE ROLE` and `DROP ROLE` shows only `postgrestest` (the schema helpers), the `testdb` tool and SQL that works inside a schema the test created. No test reads `QABZ_TEST_POSTGRES_DSN`, `config.yml` or the `POSTGRES_*` variables to reach a database.

**Why.** Operator decision, 2026-11-09 (task 020b). The task 003 live tests dropped `app_hot_configs` and `qabz_schema_migrations` in the database of `QABZ_TEST_POSTGRES_DSN`; with the development DSN, each full run deleted the migration state of the development database, and the config reloader of the running App wrote an error line at each interval.

**Review.** The `code-reviewer` agent checks each new or changed test that uses Postgres: the DSN comes from `postgrestest.RunDSN`, the test works in its own schema or its own objects, and nothing reads the development DSN. A test that can change the development database is a defect. The verdict is NOT READY.

## 15. Provider mocks (development only)

**Every provider adapter has a mock.** The mock lets a developer run the whole flow (provider pick, cache lookup, request rows, saving the answer) with no call to the provider's backend.

- **Config.** One block for each provider: `debug.mocks.<provider>.enabled` (bool, default `false`) and `debug.mocks.<provider>.return_type` (`success` or `error`, default `success`). Both keys are in `config.yml.example` and `config.yml` (§ 8). The `debug` section is removed in every mode but `dev` (task 020c), so a mock key in another mode is dropped, not an error.
- **Code.** The adapter package gives its fixed answers as `provider.MockFixtures` (for example `qabzino.MockFixtures()`); the composition root (`src/app/defaults.go`, `newProviderRegistry`) wraps the adapter with `provider.NewMock`. Do not write a second mock mechanism. With the mock off, `NewMock` returns the adapter itself (OFF is the same as no mock).
- **Dev mode twice.** The composition root applies a mock only when `Config.Mode` is `dev`, and writes one warn line when it does. The mock never runs in `stage` or `prod`.
- **`return_type: success`** is a valid answer with fixed values (the values are documented in the fixtures and in the Postman description of the endpoint). **`error`** is `provider.ErrProviderDown`, the same error as a failed call.
- **One return-type key for each method** (task 044, operator decision). `return_type` is the answer of `Inquiry`. Each other method that the mock answers has its own key, `debug.mocks.<provider>.<method>_return_type`, default `success`: `pay_return_type` (`success`, `failed`: a failed answer of the provider, `pending`: `provider.ErrProviderDown`, the answer of a timeout), `settle_return_type` (`success`, `error`) and `status_return_type` (task 045; the values of `pay_return_type`). The mock answers a method only when the adapter gives a fixture for it (`provider.MockFixtures.Pay`, `.Settle`, `.Status`); every other method goes to the adapter. A new method adds its key, its values and its fixture the same way; do not add values to `return_type`.
- **Settings are read at start.** A mock is not hot-reloadable.
- **Tests.** A test of the mock for each return type, with the mock off, and with the mode that is not dev.

**Why.** Operator decision, 2026-11-09 (task 021). The feature-ON checks (Rule 15) and the front-end work need a provider answer in development; a mock inside the adapter keeps every other step of the flow real.

**Review.** The `code-reviewer` agent checks each new provider adapter for its mock, its config keys and its tests. A provider with no mock is a defect. The verdict is NOT READY.

## 16. Redis keys have the prefix

**Every Redis key of Qabz starts with `database.redis.prefix`** (default `qabz:`). Many services can share one Redis; the prefix keeps the Qabz keys apart (supertask 043, operator, 2026-11-15).

- `system/redis.Client` puts the prefix in front of every key: a command builds its key with `Client.Key(name)`. The client gives no raw go-redis client to other packages, so a key without the prefix cannot be written.
- A new Redis command is a method of `system/redis` that uses `Key`. Code outside `system/redis` passes only the name (for example the cron lock name `crons_<name>_lock1`).
- A live test uses its own random prefix (`qabz_test_<hex>:`), so it never touches a key of the App or of another test.

**Why.** Operator rule of supertask 043 (2026-11-15): the prefix is for every Redis key, not only for the worker locks.

**Review.** The `code-reviewer` agent searches each change for go-redis commands outside `system/redis` and for a key in `system/redis` that is not built with `Key`. Either is a defect. The verdict is NOT READY.


## 17. Tracing for every feature

**Every added or changed feature MUST be traced with OpenTelemetry spans that go to Jaeger, and the spans MUST cover the full path of execution of the feature.** A feature is what § 12 calls a feature. One path of execution is one trace, from its entry to its last dependency call. No step of the path is without a span, and no step starts a new trace.

**Root span.**
- A gRPC method or an HTTP endpoint gets its server span from the tracing interceptor of the gRPC server. The HTTP gateway sends the trace context to the gRPC server, so the gateway call and the gRPC method are one trace. A new service is registered on a server that has this interceptor. The gateway itself makes no HTTP span (an HTTP span has the URL path, and a path can have an ID): it reads `traceparent` of the request into the context, and its gRPC client span is the first span of the request (task 047 decision D4).
- A run that no request starts (a cron tick, a consumer message) starts its own root span, named `<kind> <name>` (for example `cron parsian-getstatus`).
- A background job that a request starts and that continues after the answer (for example the settlement of task 044) starts a new root span with a link to the span of the request. Thus the request trace ends with the answer, and the job can still be found from the request.

**Child spans.** Each step of the path is a child of the span in `ctx`:
- each Postgres query, each Redis command, each call to a provider or to another service (gRPC or HTTP) and each Kafka publish;
- each core function that makes more than one dependency call (for example the pick of the provider and the provider call of an inquiry), so the trace shows the steps of the feature and not only a flat list of calls.

**Context.**
- `ctx` goes down the full path. A function on the path never makes a new root with `context.Background()` or `context.TODO()`. Where a step must outlive the request (`context.WithoutCancel`), the span context stays in it.
- An outbound gRPC or HTTP call sends the W3C trace context, so a Qabz service that is traced too continues the same trace.
- **Exception: the Kafka message of the payment service** must be the message of `bill`, byte for byte, with no headers (task 045, supertask 043 decision C). The publish has its span, but the message gets no trace header. A trace header on that message needs an operator decision.

**Names, attributes and errors.**
- A span name comes from a small fixed set: the full gRPC method, `<package>.<Function>`, or the name that the instrumentation gives (for example the SQL operation). Never put an ID or other free input in a span name.
- An attribute uses a value from a small fixed set (the bill type, the provider name, the operator, the result of § 12) or the internal order ID (`order.id`, so one payment can be followed). Never put a phone number, a bill ID, a pay ID, a user ID, an amount, a token, a password, a key or a request or answer body in a span.
- A failure on the path calls `jaeger.Fail(span, result, err)` (`common/monitoring/jaeger`) on the span where it happens: the status `codes.Error` with the result as the description, and an event `exception` with only the Go type of `err`. Never call `span.RecordError(err)` in Qabz code: it writes the error text, and an error text can have a phone or a bill ID in it (task 047 decision D10). The log line has the text. The instrumentation libraries keep their own error data. The result of the span is the same result that the feature metric of § 12 counts.

**Off switch.** Tracing is off when `logs.tracing.enabled` is `false` (the default; task 051). When it is `true`, `logs.tracing.otlp_endpoint` (the `host:port` of the OTLP gRPC receiver) is required: an empty value is a config error at start. Then the global tracer provider is the no-op provider of OpenTelemetry, and the spans cost almost nothing. Feature code always gets its tracer with `otel.Tracer(...)` and never checks whether tracing is on.

**Libraries.** Use the OpenTelemetry API (`go.opentelemetry.io/otel`) and the official instrumentation for gRPC and HTTP. The Postgres, Redis and Kafka spans come from one shared hook of `system/postgres`, `system/redis` and `system/kafka`, so feature code does not make them by hand. A new module follows § 2 (Go modules) and § 5 (standard library first). Task 047 added them: the `otelgrpc` server handler on both gRPC servers, the `otelgrpc` client handler on the gateway and on each gRPC provider client, `provider.NewHTTPClient` (`otelhttp`) for the HTTP adapters, the pgx tracer of `system/postgres` (the SQL operation only, no SQL text, no argument), `redisotel` (no argument) and `kafka.Publish`, and a Jaeger service in `docker-compose.yml` (profile `tracing`). A new client of a dependency uses these, not a client of its own.

**Tests.** A test with the in-memory exporter (`go.opentelemetry.io/otel/sdk/trace/tracetest`; the helper `common/monitoring/jaeger/jaegertest`: `Install`, `ByName`, `AssertClean`) checks the span tree of each feature: the root span, a child span for each dependency call of the path, the parent of each span, and the `codes.Error` status on a failure path. A test also checks that no span has a forbidden attribute (above).

**Design doc.** `docs/initial-discovery/system.html` (a Critical File, `CLAUDE.md` § 4) says how Qabz is traced. A change to the shape of the traces updates it in the same commit.

**Why.** Operator decision, 2026-11-15. `system.html` says that every request and every call to a provider is a span, so one payment can be followed from the request of the app to the answer of the provider. The tracer setup exists since task 002 (`common/monitoring/jaeger`), but no code made a span, so a trace in Jaeger was empty. The features that exist get the shared spans in task 047 and their own core-step spans in task 049.

**Review.** The `code-reviewer` agent checks each feature of the change against this section, with § 12 (metrics). A step of the path with no span, a new root inside the path, a span name or attribute with free or personal input, a failure with no error status, or no span-tree test is a defect. The verdict is NOT READY.

## 18. Every HTTP route is /v1

**Every HTTP route of Qabz has the version `/v1`. No route has `/v2` (or any other version), and no gRPC method has a `V2` name.** Qabz is a new project, so it has one API version. `bill` has `/v2` routes only because it is old: its `/v1` routes keep a first behavior that its clients still use. Qabz copies the behavior of the newest `bill` route (for example the full order token of `NewOrderV2`) and serves it on `/v1` under the plain method name (`NewOrder`).

- A new rpc in `pb/proto/qabz/` has a `/v1/...` route. A second version of a method is not added; a change of an answer is a change of the `/v1` method (a Major change with the operator's decision).
- Names that only look like a version are not routes and stay: the Go module paths (`grpc-gateway/v2`, `confluent-kafka-go/v2`, `encoding/json/v2`) and the routes of other services that Qabz calls (for example the payment service).
- The history (change logs, `CHANGELOG.md`, done tasks) keeps the routes of its time.
