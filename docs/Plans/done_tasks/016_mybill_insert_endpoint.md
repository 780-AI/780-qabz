<!-- VERSION: 1.0 -->
# 016_mybill_insert_endpoint — POST /v1/mybill saves a bill for the user

Status: done

## Description

Implement `MyBill.Insert` (`POST /v1/mybill`, `pb/proto/qabz/services.proto`). The call saves a bill (My Bills) for the user of the JWT. The request is `InsertBillRequest` and the response is `BillResponse` (`pb/proto/bill/services.proto`). The row goes into the `bills` table of the `bill` database. The handler writes through a repository interface; the Postgres implementation uses a raw query (no ORM), as in task 013.

Help:
- The `bill` behavior is in `~/projects/bill`: `controller/myBill/manager.go` (`Insert`, `insertRequestValidator`), `domain/repository/mybill/bill_dto.go` (`Insert`) and `domain/entity/bill.go` (the `bills` columns and the default names).
- The `bills` columns are also in `docs/initial-discovery/database.html`. The table has no columns for `operator`, `phone` or `isPined`: `bill` keeps the operator in `json_data` and the inquiry ID (bill ID, participate code or phone) in `data`.
- The user ID comes from the JWT claims (task 009).
- Task 015 creates the `bills` table in the dev database. The tests do not need it: they use a throw-away schema (`system/postgres/postgrestest/`).

Points for the design step:
- `system.html` gives the new path as `POST /v3/mybill`; the proto has `/v1/mybill`. Which path.
- The validation rules per bill type (`gas`, `water`, `tel`, `mobile`, `electricity`): bill ID format, phone normalization, participate code. Copy `bill` or not.
- A bill that already exists: `bill` uses `SelectOrInsert` on `user_id`, `status = 'enable'` and (`bill_id` or `data`), and returns the existing row with no error. Keep that, or give an error.
- The default name when `name` is empty, and the `mci` default operator for `mobile`.
- `bill` logs an invalid request at error level. The logging policy says a client error writes no line.
- Every difference from `bill` is a numbered item in `docs/this_vs_bill.md` with a `@todo` comment at the code (task 013 practice).
- The Step 6 middleware and monitoring rules of `Implementation_Plan.md` (metrics, recovery, logs).

## Definition of Done

- `POST /v1/mybill` (or the path from the design step) saves the bill for the JWT user and returns `BillResponse` with the new `id`, HTTP 200.
- An invalid request gives `InvalidArgument` (HTTP 400) and writes nothing to the database.
- **Unit tests:** cover the handler, the validation and the repository logic with mocks and fakes (no database), including the error paths.
- **Database tests:** run against a temporary database (a throw-away schema with bill's `bills` table). Each test inserts a bill, then checks that the row exists in two ways: a direct SQL query on `bills`, and a lookup through the repository. The values must match the request.
- Build, vet and tests pass. The Postman collection (conventions § 7), the docs and the CHANGELOG are updated in the same commit.

## Verification

The feature-ON verification (Rule 15) of this endpoint waits for its GET counterpart (`GET /v1/mybill`, task 017). Then each endpoint verifies the other: save bills with `POST`, read them with `GET`, and compare. Pass: `GET` returns each saved bill with the values that `POST` sent and the `id` that `POST` returned.

Until then, this task records the verification as ⏸ pending in `OPEN_TASKS.md` and does not close it. The unit and database tests above are the evidence at task close.

## Decisions (operator, 2026-11-08)

- Path: `POST /v1/mybill`, as in the proto. No `pb` change; the `/v3` rename comes later for all endpoints together.
- A bill that the user already has: match bill. An enabled row of the user with the same `bill_id` or the same `data` (any type) gives that row's id, HTTP 200, and no new row.
- A save that breaks UNIQUE (`user_id`, `type`, `bill_id`) because of a disabled row: match bill, HTTP 500 (`Internal`).
- Validation: copy bill exactly (type enum, bill ID check digit, mobile `^989\d{9}$` and tel `^98\d{10}$` after the 0/98 normalization, gas participate code not checked).
- My decisions, recorded in `docs/this_vs_bill.md` items 5-8: an invalid request is `InvalidArgument` (HTTP 400, bill: `Unknown`/500); a call with no user (only with `debug.auth.disable`) is `PermissionDenied`; a client error writes no log line; after a broken unique key Qabz looks once more with no wait (bill: 5 tries with a backoff).

## Result (2026-11-08)

- `common/helpers/`: `ValidBillID` (bill mod 11 check digit), `NormalizePhone`, `ValidMobile`, `ValidTel`.
- `core/savedbill/savedbill.go`: `Bill`, `InsertRequest`, `Repository` (`Insert`, `Get`), `Store.Insert` (rules by type, default name, `mci` for `mobile`, `ErrInvalid`).
- `system/postgres/savedbill_repository.go`: raw SQL on `bills`. Find (enabled, same `bill_id` or `data`), else insert (UUID v4 id; empty string as NULL; `json_data` = `{"Operator", "Phone": "", "IsPinned": false}` or NULL, the go-pg shape of bill), one more find after SQLSTATE 23505. `postgrestest.BillsSchema`: bill's `bills` table with the unique key (tests only).
- `entry/grpc/mybills.go`: `MyBills.Insert`; the answer echoes the request as bill does (`operator` and `phone` as sent, `isPined` false). `app/defaults.go`: wiring, `RegisterMyBills`, `pb.RegisterMyBillsHandler`, metric `qabz_mybill_insert_total{result="created"|"existing"}`. Logs: `mybill: insert failed` with the cause, only for a failed write.
- Tests, unit (no database): helpers (with a copy of bill's `IsValidBillId` as a reference over 408 bases), store (9 valid, 12 invalid cases, errors), repository with a fake querier (new, existing, race, disabled conflict, other errors, Get), handler (mapping, every type, invalid, no user, internal, context, over gRPC with the real JWT check), gateway with a fake store, wiring.
- Tests, database (`QABZ_TEST_POSTGRES_DSN`, throw-away schema): `TestLiveSavedBillInsert` checks each row by a direct SQL query and by `Repository.Get`; `TestLiveSavedBillDuplicates`; `TestLiveSavedBillDisabledConflict`; `TestLiveMyBillInsertEndToEnd` (gateway → JWT → handler → store → repository → SQL check and `Get`). Coverage of the new code 100% except two nil-row guards.
- `go build`, `go vet`, `go test -race -count=1 ./...` pass, with the live tests on the compose Postgres.
- Deliberate failure (overlays): no `NULLIF` on `bill_id` fails the live insert test; no `mci` default fails 2 store cases; a log line on an invalid request fails the handler test; no retry after 23505 fails 4 tests (3 unit, 1 live); a changed check-digit rule fails the bill reference test.
- Postman: new folder `MyBills`, request `Insert` (conventions § 7).

## Change after the operator's first run (2026-11-08)

- The operator called `POST /v1/mybill` with `debug.auth.disable: true` and got HTTP 403: the disabled check put no user into the call. Operator decision: a new config key `debug.auth.mock_user_id` (a lower-case UUID, default `""`). With the check off, the auth interceptor gives every call that user as the claims' `UserID`; with the key empty, a call has no user (403 as before); with the check on, the key has no effect. The local `config.yml` has a generated test UUID.
- `config/config.go` (key and UUID check), `config.yml.example`, `entry/grpc/auth.go` (`AuthOptions.MockUserID`), `app/defaults.go` (wiring; the start warning names the mock user). Tests: config (valid, default, 6 bad forms), interceptor (unary, stream, no key, check on ignores the key), wiring (with the key Insert reaches the database). Deliberate failure: without the claim injection, 2 tests fail.

## Verification (Rule 15)

⏸ Pending, by operator decision in the task: the feature-ON check runs with task 017 (`GET /v1/mybill`). The scenario is in `OPEN_TASKS.md`.
