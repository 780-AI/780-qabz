<!-- VERSION: 0.1 -->
# 019_mybill_delete_endpoint — DELETE /v1/mybill/{id} deletes a saved bill of the user

Status: pending

## Description

Implement `MyBills.Delete` (`DELETE /v1/mybill/{id}`, `pb/proto/qabz/services.proto`). The call deletes a bill that the user of the JWT saved with `POST /v1/mybill` (task 016). The request is `DeleteBillRequest` (`id`) and the response is `BillActionResponse` (`message`, `pb/proto/bill/services.proto`). As in `bill`, a delete does not remove the row: it sets `bills.status` to `'disable'`, so `GET /v1/mybill` (task 017) no longer lists the bill. The handler writes through the `savedbill` repository interface (raw query, no ORM), as in tasks 016 and 017.

**Operator rule: the endpoint MUST behave exactly like `bill`.** Every behavior that a client or the database can see is copied: the id check, the rows that change, the answer on success, and the answer on each error. A difference is allowed only when the operator approves it in the design step. Each approved difference is a numbered item in `docs/this_vs_bill.md` with a `@todo` comment at the code (task 013 practice).

Help:
- The `bill` behavior is in `~/projects/bill`: `controller/myBill/manager.go` (`Delete`), `service/myBill/manager.go` (`Delete`), `domain/repository/mybill/bill_dto.go` (`Delete`), `pkg/errorHandler/errors.go` (the messages).
- What `bill` does, read from that code (2026-11-08). The design step confirms each point against the code again:
  1. The id is checked with `uuid.Parse` (`github.com/google/uuid` v1.6.0, bill's `go.mod`). An id that does not parse gives the `invalid_parameter_error` user error: a plain Go error, so gRPC sends `Unknown` (HTTP 500). `uuid.Parse` also accepts forms other than the standard one: upper case, `{...}`, 32 hex digits with no hyphens, and `urn:uuid:...`.
  2. The update is `UPDATE bills SET status = 'disable' WHERE id = <id as sent> AND user_id = <JWT user>` (go-pg `Model(&entity.Bill{Status: DISABLE}).Column("status").Where(...).Update()`). The id goes to Postgres as the client sent it, not in the parsed form. The status of the row before the update is not checked.
  3. The number of changed rows is not checked. An id that does not exist, a bill of another user and a bill that is already disabled all give success, and nothing changes.
  4. A database error gives the `database_error` user error (`Unknown`, HTTP 500). This includes an id that `uuid.Parse` accepts but Postgres does not: the `urn:uuid:` form (checked on the compose Postgres, 2026-11-08: upper case, `{...}` and no hyphens are accepted; `urn:uuid:...` gives "invalid input syntax for type uuid").
  5. On success the answer is HTTP 200 and `BillActionResponse.message` = the text of `invalid_parameter_error`: "خطای داخلی رخ داده است" ("an internal error has occurred"). `bill` sends this text on success.
  6. The user comes from the context value `user_id`, which bill's JWT middleware sets.
- The `bills` table and the throw-away test schema: `system/postgres/postgrestest.BillsSchema` (task 016). The dev database has the table since task 015 (`devdb seed`).

Points for the design step (the operator decides; the default is "match bill"):
- The error codes. Tasks 016 and 017 answer `InvalidArgument` (HTTP 400) for an invalid request, `PermissionDenied` for a call with no user, and `Internal` for a server fault (`docs/this_vs_bill.md` items 5, 6, 10); `bill` answers `Unknown` (HTTP 500) for all of them. Use the same rule as tasks 016 and 017, or match bill exactly for this endpoint.
- The success message. Match bill (the Persian "internal error" text on success), or send another text.
- An id that `uuid.Parse` accepts but that is not in the standard form: pass it to Postgres as sent (bill), or send the parsed form.
- Logs: conventions § 10 asks for one error line for a failed database write; a client error writes no line (bill logs both).
- The Step 6 middleware and monitoring rules of `Implementation_Plan.md` (metrics, recovery, logs). A delete counter (like `qabz_mybill_insert_total`), or only `qabz_request_duration_ms`.

## Definition of Done

- `DELETE /v1/mybill/{id}` sets `status = 'disable'` on the bill only when the bill belongs to the JWT user, and answers as bill does (HTTP 200 and the bill message). No row is removed. Other columns, other users' bills and other bills of the user do not change.
- Every case of the bill code above gives the bill answer, or the answer that the operator approved in the design step: a valid id of the user, an already disabled bill, an unknown id, a bill of another user, each id form that `uuid.Parse` accepts, an id that does not parse, an empty id, a database error, a call with no user.
- **Unit tests:** cover the handler, the store and the repository with mocks and fakes (no database), with every case above and every error path. Each server-fault answer has a test of its log line (conventions § 10).
- **Integration tests:** run against a temporary database (a throw-away schema with bill's `bills` table), through the gateway, the JWT check, the handler, the store and the repository. Each test inserts bills (for two users, enabled and disabled), sends `DELETE`, then checks the `bills` rows by a direct SQL query (only the target row changed, only `status`, nothing removed) and checks `GET /v1/mybill` (the bill is not listed; the other bills are).
- **Bill reference check:** for each id form, the result of the Qabz id check matches `uuid.Parse` of `github.com/google/uuid`, v1.6.0, the version that `bill` uses. Run it like the task 016 check-digit reference test, or as a scratch test outside the repo (record which).
- Deliberate failures (positive controls): break the `user_id` filter, the `status` value and the id check, one at a time; the tests must fail.
- Build, vet and tests pass. The Postman collection (conventions § 7), `docs/this_vs_bill.md` (only for approved differences), the docs and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), with real data in the dev database. Run the App against the dev database with `debug.auth.disable: true` and `debug.auth.mock_user_id` set (or with a real token). Save bills with `POST /v1/mybill` for two users; `DELETE` one bill of user A; `GET /v1/mybill` as A and as B. Then send `DELETE` for that bill again, for a bill of user B as user A, for an unknown id and for an invalid id.

Pass:
- The first `DELETE` gives HTTP 200 with the bill message. A direct SQL query shows the row with `status = 'disable'` and all other columns unchanged. `GET` as A no longer lists the bill, and lists the other bills of A. `GET` as B is unchanged.
- The second `DELETE`, the `DELETE` of B's bill as A and the unknown id give the bill answer (HTTP 200), and no row changes. B's bill is still listed for B.
- The invalid id gives the approved error answer and no row changes.
- The rows that the run added are deleted after the run.

If the operator can run the old `bill` service against a copy of the same data, send the same requests to both and compare the HTTP status, the body and the `bills` rows. This is the strongest evidence of "exactly like bill". Record whether this comparison ran.
