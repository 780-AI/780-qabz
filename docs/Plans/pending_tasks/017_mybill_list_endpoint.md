<!-- VERSION: 0.1 -->
# 017_mybill_list_endpoint — GET /v1/mybill returns the saved bills of the user

Status: pending

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
