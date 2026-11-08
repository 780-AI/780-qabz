<!-- VERSION: 1.0 -->
# 017_mybill_list_endpoint — GET /v1/mybill returns the saved bills of the user

Status: done

## Description

Implement `MyBill.GetAll` (`GET /v1/mybill`, `pb/proto/qabz/services.proto`). The call returns the bills that the user of the JWT saved with `POST /v1/mybill` (task 016). The response is `BillsResponse` (`serviceId`, `data` of `BillResponse`, `pb/proto/bill/services.proto`). The rows come from the `bills` table of the `bill` database, through the same repository interface as task 016 (raw query, no ORM).

Help:
- The `bill` behavior is in `~/projects/bill`: `controller/myBill/manager.go` (`GetAll`) and `domain/repository/mybill/bill_dto.go` (`GetAll`). The query is `user_id = <JWT user>` and `status = 'enable'`, `ORDER BY created_at DESC`.
- `bill` maps the rows per type: for `tel` and `mobile` the phone is `data` (or `bill_id` when `data` is empty), and a second row with the same phone is skipped; for `gas` the participate code is `data`. `operator` and `isPined` come from `json_data`.
- `serviceId` is the config key `service.id` (task 013).

Points for the design step:
- `system.html` gives the new path as `GET /v3/mybill`; the proto has `/v1/mybill`. Use the same choice as task 016.
- Keep the per-type mapping and the duplicate-phone skip of `bill`, or not.
- A user with no saved bills: an empty `data` list, HTTP 200.
- Every difference from `bill` is a numbered item in `docs/this_vs_bill.md` with a `@todo` comment at the code (task 013 practice).
- The Step 6 middleware and monitoring rules of `Implementation_Plan.md` (metrics, recovery, logs).

## Definition of Done

- `GET /v1/mybill` (or the path from the design step) returns only the bills of the JWT user, as `BillsResponse`, HTTP 200. Bills of other users and disabled rows are not in the list.
- **Unit tests:** cover the handler, the row mapping and the repository logic with mocks and fakes (no database), including the error paths.
- **Database tests:** run against a temporary database (a throw-away schema with bill's `bills` table). Each test inserts bills (for two users, and some disabled rows), then reads them with the repository and through the endpoint, and compares with a direct SQL query on `bills`. The values and the order must match.
- Build, vet and tests pass. The Postman collection (conventions § 7), the docs and the CHANGELOG are updated in the same commit.

## Verification

This endpoint and `POST /v1/mybill` (task 016) verify each other (Rule 15, feature ON). Run the App against the dev database, save bills with `POST`, then read them with `GET`. Pass: `GET` returns each saved bill with the values that `POST` sent and the `id` that `POST` returned; a call with the token of another user does not return them.

The same run closes the pending verification of task 016 in `OPEN_TASKS.md`.

## Decisions (operator, 2026-11-08)

- Path: `GET /v1/mybill`, as in the proto (the same choice as task 016).
- Mapping: match bill. For `tel` and `mobile`, `billId` and `phone` are the phone (`data`, or `bill_id` when `data` is empty), and a later row with a phone already in the list is left out (the `tel` and `mobile` phones share one set, as in bill); for `gas`, `participateCode` is `data`.
- A row whose type is not in the `BillResponse` enum (old types such as `mobileIrancell`): match bill. It is sent as `gas` (enum value 0) with `bill_id`.
- A `json_data` value of the wrong JSON type (for example `"IsPinned": "true"`): match bill. The whole call fails with `Internal` (HTTP 500) and one log line.
- My decisions, recorded in `docs/this_vs_bill.md`: a call with no user is `PermissionDenied` (item 6, as for POST); the order gets `id DESC` after `created_at DESC`, so rows saved at the same time have a fixed order (item 9); a failed read is `Internal` with no detail (item 10, bill: `Unknown`).
- `json_data` is read the way bill reads it: go-pg decodes it with `encoding/json` into `entity.Data`, so a key matches without case (`operator` is read as `Operator`; `isPined` is not `IsPinned`, so it is `false`, as in bill), a later key wins, and a JSON `null` changes nothing. This answers the task 016 follow-up about the real data shapes. GET reads on each request, so it is a hot path: fastjson (conventions § 1).

## Result (2026-11-08)

- `core/savedbill/savedbill.go`: `Bill` gets `IsPinned` and `LastOrderDate`; `Repository.List`; `Store.List` (no user: `ErrInvalid`, the repository is not called).
- `system/postgres/savedbill_repository.go`: `listSavedBillsSQL` (raw SQL: `user_id = $1 AND status = 'enable' ORDER BY created_at DESC, id DESC`; NULL as `""`; `json_data` as text), `SavedBillRepository.List`, `billData` (json_data with a `fastjson.ParserPool`, keys matched with `bytes.EqualFold`, strings copied before the parser goes back to the pool) and `jsonString`. The `querier` interface gets `Query`.
- `entry/grpc/mybills.go`: `BillLister`, `MyBillsDeps.Lister` and `ServiceID`, `MyBills.GetAll`, `billResponses` (the bill mapping). Logs: `mybill: list failed` with the cause, only for a failed read.
- `app/defaults.go`: `newMyBills` takes the config (for `service.id`) and wires the store as lister. No new metric: `qabz_request_duration_ms{path="/haf80.api.bill.MyBills/GetAll"}` counts the calls.
- Tests, unit (no database): store (`List` passes the user, no user, error, zero store); repository with a fake querier and fake rows (mapping of 4 row shapes, empty, query / scan / rows / nil-rows errors, bad `json_data` names the row, no pool); `TestBillData` (27 inputs); handler (the mapping of 12 rows with the duplicate skip, the old type and the empty phone; empty; no user; internal with the log line; canceled; nil context; no lister; over gRPC with the real JWT check); gateway (`GET /v1/mybill` JSON names, `"data":[]`, 403); wiring (`GetAll` registered, reaches the database with the mock user, `service.id` out of range).
- Tests, database (`QABZ_TEST_POSTGRES_DSN`, throw-away schema): `TestLiveSavedBillList` (rows of two users, disabled rows, rows saved by `Insert`, a NULL `created_at`, 8 rows with one `created_at` in mixed id order, the real `json_data` shapes) compares `List` with a direct SQL query on `bills`, value by value and in order, and with the expected id order; a bad `json_data` fails the list. `TestLiveMyBillListEndToEnd`: 3 bills by `POST` through the gateway, a row of another user and a disabled row by SQL, then `GET`: the 3 bills newest first with the POST ids and values, the same ids and order as a direct SQL query, 403 without a token.
- `encoding/json` check: `billData` gave the same result as `encoding/json` v1 into bill's `entity.Data` for 30 inputs (case, duplicate keys, the Kelvin sign, null, wrong types, non-objects). The check ran from the scratchpad through `go test -overlay`, because conventions § 1 forbids `encoding/json` in the repository. With `bytes.Equal` in place of `bytes.EqualFold` it failed (positive control).
- `go build`, `go vet`, `go test -race -count=1 ./...` pass, with the live tests on the compose Postgres. Coverage: `core/savedbill` 100%, `entry/grpc` 98.0%, `system/postgres` 88.4% (unit only).
- Deliberate failure (overlays), each caught: no duplicate-phone skip fails `TestGetAllMapsBills`; no `status = 'enable'` fails both live tests; no `user_id` filter fails both live tests; no `id DESC` fails `TestLiveSavedBillList` (the first version with 2 tied rows passed by chance, so the test now has 8 tied rows in mixed order); no log line fails `TestGetAllErrorIsInternal`; `bytes.Equal` for the key fails 3 tests; no type check on `IsPinned` fails 3 tests.
- Postman: request `GetAll` in the folder `MyBills` (conventions § 7).

## Verification (Rule 15, feature ON)

Done 2026-11-08 by the implementer, with the operator's debug App and the compose containers left running. Two copies of the built App ran against the compose dev database with `debug.auth.disable: true`: copy A on ports 18080/18082/19090 with a new test user, copy B on 28080/28082/29090 with another new test user.

- A `GET` before any save: HTTP 200 `{"serviceId":106,"data":[]}`.
- A `POST` water `123455` "آب خانه", mobile `09121234567` operator `irancell`, gas `500003` participate code `7788990`: each HTTP 200 with a new id. The same water bill again: HTTP 200 with the first id. A bad bill ID `123456`: HTTP 400 `invalid parameter`.
- A `GET`: HTTP 200, the 3 bills newest first (gas, mobile, water) with the ids that POST returned. Water: name, type and `billId` as sent. Mobile: the default name, `operator` `irancell`, `phone` and `billId` `989121234567` (the phone as stored after the POST normalization; the POST answer echoed `09121234567`, as bill does). Gas: `billId` `500003`, `participateCode` `7788990`.
- B `GET`: HTTP 200 `"data":[]`. The bills of A are not in it.
- An old-shape row added by SQL for A (`tel`, `data` NULL, `json_data` `{"operator": "tci", "isPined": true}`) and a disabled row: `GET` shows the tel row with `phone` = `bill_id`, `operator` `tci`, `isPined` false (as bill reads it); the disabled row is not in the list.
- Metrics of A: `qabz_mybill_insert_total` `created` 3, `existing` 1; `qabz_request_duration_ms_count` `GetAll` OK 2, `Insert` OK 4, `Insert` InvalidArgument 1. No error log line in either copy; both stopped with `app stopping`.
- After the run the rows of the two test users were deleted (5 rows) and the scratch configs removed.

Verdict: the output matches the design. `GET` returns each saved bill with the values that `POST` stored and the `id` that `POST` returned, newest first, and another user does not see them. This run is also the feature-ON check of task 016 (`OPEN_TASKS.md`). Awaiting operator confirmation.

Not covered at runtime: `GET` without the `bills` table (HTTP 500 and the `mybill: list failed` line) is covered by `TestGetAllErrorIsInternal` only.
