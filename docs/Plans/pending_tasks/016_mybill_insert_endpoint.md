<!-- VERSION: 0.1 -->
# 016_mybill_insert_endpoint — POST /v1/mybill saves a bill for the user

Status: pending

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
