<!-- VERSION: 0.3 -->
# 019_mybill_delete_endpoint — DELETE /v1/mybill/{id} deletes a saved bill of the user

Status: done

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

## Decisions (operator, 2026-11-08)

The answers to "Points for the design step":
- Error codes: match bill exactly. An id that does not parse, an empty id, a call with no user and a database error all answer gRPC `Unknown` (HTTP 500) with bill's text. Not a difference from bill, so no `this_vs_bill.md` item for the codes. Confirmed in bill's code: a call with no user panics at `ctx.Value("user_id").(string)`; the recovery handler (`app/router/router_dto.go` `panicRecover`) answers `invalid_parameter_error`.
- Success message: match bill. HTTP 200, `message` = the `invalid_parameter_error` text "خطای داخلی رخ داده است".
- Id form: pass as sent (bill). The check is `uuid.Parse` of `github.com/google/uuid` v1.6.0; the id goes to Postgres as the client sent it, so `urn:uuid:` gives the database error.
- Logs and metrics: conventions § 10. One error line for the database error (`mybill: delete failed`, with a test); no line for a client error, although it answers `Unknown`. bill writes a warn line for a call with no user, so this is item 11 of `docs/this_vs_bill.md`, with a `@todo` at the code. bill writes no line for an invalid id, the same as Qabz. A delete counter `qabz_mybill_delete_total{result}` like the insert counter.
- Delete counter labels (operator, fix round 2026-11-08: "a delete counter with bill type label"): `type` = `bills.type` of the matched bill, `unknown` for `not_found` and for a NULL type; read with `RETURNING type` on bill's UPDATE (the rows written do not change). Implementer's choice for `result`: `found` (a bill of the user has the id; Postgres matches an already disabled row too, because the SQL is bill's and does not check the status) or `not_found`. The id check uses `github.com/google/uuid` itself (now a direct requirement): the Go standard library `uuid.Parse` (Go 1.27) differs from it (it rejects `URN:UUID:` in upper case and checks the braces; google/uuid v1.6.0 accepts any first and last character in the 38-character form).

## Result (2026-11-08)

- `core/savedbill/savedbill.go`: `StatusDisable`, `Repository.Disable`, `Store.Delete` (no user or an id that `uuid.Parse` rejects: `ErrInvalid`, the repository is not called; the id goes on as sent).
- `system/postgres/savedbill_repository.go`: `disableSavedBillSQL` (raw SQL, bill's statement: `UPDATE bills SET status = 'disable' WHERE id = $1::text::uuid AND user_id = $2::text::uuid RETURNING type`; the text casts make Postgres read the values as sent) and `SavedBillRepository.Disable` (QueryRow: a returned row is found, with its type, "" for NULL; `pgx.ErrNoRows` is not found).
- `entry/grpc/mybills.go`: `BillDeleter`, `MyBillsDeps.Deleter` and `OnDelete`, `DeleteFound` / `DeleteNotFound`, `DeleteTypeUnknown`, `billInternalErrorText`, `errBillUnknown`, `MyBills.Delete`. The comment at the client-error answers cites operator decisions 1 and 4 (no log line although the code is `Unknown`).
- `app/defaults.go`: the store is the deleter; metric `qabz_mybill_delete_total{type,result}`.
- Tests, unit (no database): store (every id form, 9 invalid cases, the repository error, zero store; `TestDeleteMatchesBillUUIDParse`); repository with a fake querier (the statement, the arguments as sent, matched rows, error, no pool); handler (success with and without a matching bill, client errors through the real store with no log line, no user before the deleter, the database error with exactly one log line, canceled, no deleter, over gRPC with the real JWT check); gateway (the route, escaped ids, `urn:uuid:` not read as a verb, empty id, 500 with `"code":2`, 403); wiring (the counter, the no-user and mock-user answers).
- Tests, database (`QABZ_TEST_POSTGRES_DSN`, throw-away schema), run on the compose Postgres: `TestLiveSavedBillDisable` and `TestLiveMyBillDeleteEndToEnd` (bills of two users, enabled and disabled, POST and SQL; after each `DELETE` every row is compared with a direct SQL query, only the target row's `status` changes and nothing is removed, and `GET /v1/mybill` lists the other bills).
- Bill reference check (in the repo, like the task 016 check-digit test): `TestDeleteMatchesBillUUIDParse` compares the store's id check with `uuid.Parse` of `github.com/google/uuid` for 3 793 inputs (each known length and the lengths around them, every position changed to 10 probe characters) and checks that the module is v1.6.0 (build info). Positive control: with the standard library `uuid` in the store, it and `TestDeleteIDForms` fail.
- Deliberate failures (overlays), each caught: no `user_id` filter fails `TestSavedBillDisableSQL` and both live tests; `status = 'disabled'` fails `TestSavedBillDisableSQL` and both live tests; no id check fails `TestDeleteInvalid`, `TestDeleteMatchesBillUUIDParse`, `TestDeleteClientErrors` and `TestMyBillDeleteOverHTTP` (the live tests pass, because Postgres then rejects the id with the same answer); no log line on the database error fails `TestDeleteErrorIsUnknown`; a log line on a client error fails `TestDeleteClientErrors`. Without the text casts only `TestSavedBillDisableSQL` fails: pgx v5 also passes `{...}` to Postgres.
- Postman: request `Delete` in the folder `MyBills` (conventions § 7).
- Fix round (type label): repository, store and handler return the bill type; `OnDelete(type, result)`; tests: repository (a type, a NULL type, no row, nil row), store (type passed through), handler (`mobile found`, NULL type → `unknown found`, `unknown not_found`), both live tests assert the type (`TestLiveMyBillDeleteEndToEnd` checks the counter labels of each call). Positive controls: label forced to `water` fails `TestDeleteSuccess` and `TestLiveMyBillDeleteEndToEnd`; `RETURNING name` fails `TestSavedBillDisableSQL` and both live tests. The three earlier deliberate failures were re-run and are still caught.

## Verification (Rule 15, feature ON)

Done 2026-11-08 by the implementer, with the compose containers running. Three copies of the built App ran against the compose dev database with `debug.auth.disable: true`: A (ports 18080/18082/19090) and B (28080/28082/29090) with two new test users, C (38080/38082/39090) with an empty `mock_user_id`.

- POST as A: water, mobile, gas (3 new ids); as B: water (1 id).
- `DELETE` A's water bill as A: HTTP 200 `{"message":"خطای داخلی رخ داده است"}`. A direct SQL query (all columns of the 4 rows, before and after): only that row's `status` changed, `enable` to `disable`. `GET` as A: the gas and mobile bills; `GET` as B: B's bill.
- As A: the same bill again, B's bill and an unknown id: each HTTP 200 with the same message. `not-a-uuid`, the empty id and `urn:uuid:<mobile id>`: each HTTP 500 `{"code":2, "message":"خطای داخلی رخ داده است", "details":[]}`. The mobile id in upper case and the gas id as `%7B<id>%7D`: HTTP 200, and those rows are disabled. As C (no user): HTTP 500 with the same body.
- Final SQL diff: only A's three bills changed, only `status`; B's row unchanged; no row removed. `GET` as A: `[]`; as B: B's bill.
- Logs: A has exactly one error line, `mybill: delete failed` with `invalid input syntax for type uuid: "urn:uuid:..." (SQLSTATE 22P02)`; no line for the invalid or empty id; B and C have none (only the start warning of the disabled check).
- Metrics of A (before the `type` label): `qabz_mybill_delete_total` `found` 4, `not_found` 2; `qabz_request_duration_ms_count` `Delete` OK 6, Unknown 3. Of C: `Delete` Unknown 1.
- After the run the 4 rows of the two test users were deleted and the scratch configs removed.

- Metrics re-check after the `type` label (same day; copies A and B with two new test users; POST water and mobile as A, water as B; a NULL-type row for A by SQL). As A: DELETE water, water again, mobile (upper case), the NULL-type row, B's bill, an unknown id (all HTTP 200), `not-a-uuid` and `urn:uuid:` (HTTP 500). SQL diff: only A's three rows changed to `disable`; B's row unchanged. Counter of A: `{result="found",type="water"} 2`, `{result="found",type="mobile"} 1`, `{result="found",type="unknown"} 1`, `{result="not_found",type="unknown"} 2`; B: none. One error line in A (the `urn:uuid:` id). 4 rows deleted after.

Verdict: the output matches the design: bill's answers, only the target row's `status` changes, and one log line only for the server fault. Not run: the same requests against the old `bill` service on a copy of the data (no bill instance on this machine). Awaiting operator confirmation.
