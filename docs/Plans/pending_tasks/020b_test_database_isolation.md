<!-- VERSION: 0.1 -->
# 020b_test_database_isolation — Tests never touch the development database: one random throw-away database for each test run

Status: pending

## Description

**Rule (operator, 2026-11-09): what the tests create and drop must never affect the development database.** No test may create, change, drop or reset any object (table, schema, row, migration state) in the database that the App uses in development (`database.postgres.*` in `config.yml`, the compose `qabz` database).

The tests break this rule today. `reset()` in `src/system/postgres/live_test.go:31-37` (the task 003 live tests) runs `DROP TABLE IF EXISTS app_hot_configs, qabz_schema_migrations` in the database that `QABZ_TEST_POSTGRES_DSN` names. When that DSN points to the development database, each full live test run deletes its migration state, and the config reloader then writes an error line at each `hot_reload_interval` until the App runs the migrations again (seen on 2026-11-09 during task 020). The other live tests use `postgrestest` throw-away schemas (`qabz_test_<random>`), but these schemas are still created inside the development database.

**The new model:**
- One throw-away database for each test run: the whole suite of a run, not each test method. It has a random name (for example `qabz_test_<random hex>`), so each run starts from a clean database and two runs at the same time cannot collide.
- The run creates the database on the Postgres server that `QABZ_TEST_POSTGRES_DSN` names, runs the migrations and the seeds that the tests need in it, and drops it at the end of the run (`DROP DATABASE ... WITH (FORCE)`), also when a test fails.
- `QABZ_TEST_POSTGRES_DSN` is used only to reach the server (as an admin connection). No test connects to the database named in it for its work, and no test changes it.
- Inside the run database, the existing per-test isolation stays possible (for example the `postgrestest` schemas), but every object lives in the run database only.
- Every live test moves to this model: the task 003 tests (`live_test.go`, `reset()` is removed), `postgrestest`, `devdb` live tests, and the live tests of `entry/http`, `app` and `system/postgres`.

The rule is recorded as a new section of `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md`, so the `code-reviewer` agent checks it, and § 11 (coverage gate) is updated to describe the new DSN use.

Points for the design step:
- The unit of "one test run": Go runs each package as its own test binary, often in parallel. Options: one database per package run (`TestMain` in each package with live tests, through one shared helper in `postgrestest`), or one database for the whole `go test ./...` (a Make target that creates the database, passes its DSN, and drops it). Recommended: per package through `TestMain` and one helper, because it also works with a plain `go test <pkg>` and in parallel.
- A run that is killed (Ctrl-C, timeout) cannot drop its database. Decide how leftovers are found and removed (for example the helper drops `qabz_test_%` databases older than a fixed age at start, or a `make` target does it).
- The Postgres role that the DSN uses needs `CREATEDB`. Check the compose role, and document it.
- A guard: the helper refuses to run if the random database name equals the database named in the DSN or in `config.yml`.

## Definition of Done

- No test code creates, drops, truncates or resets anything outside the random run database. A search of the test code for `DROP`, `TRUNCATE`, `CREATE SCHEMA`, `CREATE DATABASE` shows only the shared helper and work inside the run database.
- `reset()` in `src/system/postgres/live_test.go` no longer touches the database in the DSN.
- Proof with the compose database: record the tables, schemas and `qabz_schema_migrations` row of the development database, run `go test -race -count=1 ./...` with `QABZ_TEST_POSTGRES_DSN` set, record them again. Pass: nothing changed, `app_hot_configs` and `qabz_schema_migrations` are still there, no `qabz_test_%` database and no `qabz_test_%` schema is left.
- A positive control: a test that drops a table in the DSN database (scratch change) is caught by the guard or by the proof above.
- The new conventions section is in `PROJECT_SPECIFIC_CONVENTIONS.md`, and § 11 is updated. The live tests still run (no skip) and the coverage gate still passes.
- Build, vet and tests (with `-race`) pass. The docs and the CHANGELOG are updated in the same commit.
