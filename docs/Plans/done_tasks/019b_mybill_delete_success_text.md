<!-- VERSION: 0.3 -->
# 019b_mybill_delete_success_text — DELETE /v1/mybill/{id} sends a success text on success

Status: done

## Description

Fix-forward of task 019 (Rule 3 letter suffix). Task 019 copied bill: a successful `DELETE /v1/mybill/{id}` answers HTTP 200 with `BillActionResponse.message` = "خطای داخلی رخ داده است" ("an internal error has occurred"), bill's `invalid_parameter_error` text. Clients read it as a failure. Operator decision (2026-11-08): on success the message is "عملیات با موفقیت انجام شد" ("the operation was done successfully").

Only the success answer changes. HTTP status 200 and every other behavior stay as in task 019: the soft delete (`status = 'disable'`), the cases that succeed with no change (unknown id, another user's bill, an already disabled bill), every failure answer (gRPC `Unknown`, HTTP 500, with bill's text), the logs and the `qabz_mybill_delete_total{type,result}` counter.

This is a difference from bill: a new numbered item 12 in `docs/this_vs_bill.md` with a `@todo` comment at the code (task 013 practice). The success text and the failure text become two separate constants in `src/entry/grpc/mybills.go`.

## Definition of Done

- A successful DELETE answers HTTP 200 with `{"message": "عملیات با موفقیت انجام شد"}`, for each success case of task 019. Every failure answer is unchanged (HTTP 500, `code: 2`, bill's text).
- Unit tests, the gateway test and the live end-to-end test check the new success text and that the failure text did not change. A deliberate failure (the old text on success) makes the tests fail.
- `docs/this_vs_bill.md` item 12 and the `@todo` at the code; the Postman `Delete` request (description and test) and `src/app/README.md` show the new text; CHANGELOG with a QA scenario in `docs/Plans/OPEN_TASKS.md`. The Task 019 QA scenario there is updated to the new success text.
- Build, vet and tests pass; coverage per conventions § 11 (database tests included).

## Verification

Feature ON (Rule 15): run the App against the dev database with `debug.auth.disable: true` and a test user. `POST /v1/mybill` a bill, then `DELETE` it, `DELETE` it again, and `DELETE` an unknown id and `not-a-uuid`.

Pass: each of the first three answers HTTP 200 `{"message": "عملیات با موفقیت انجام شد"}`; `not-a-uuid` answers HTTP 500 `{"code": 2, "message": "خطای داخلی رخ داده است", ...}`; the row is `disable` after the first DELETE; the test rows are deleted after the run.

## Result (2026-11-08)

- `src/entry/grpc/mybills.go`: `billInternalErrorText` is now the failure text only; new `deleteSuccessText` = "عملیات با موفقیت انجام شد" with the `@todo` to item 12; `MyBills.Delete` answers it on success.
- Tests: `TestDeleteSuccess` and `TestDeleteOverGRPCWithAuth` (handler, gRPC), `TestMyBillDeleteOverHTTP` (gateway, also the escaped forms), `TestLiveMyBillDeleteEndToEnd` (compose Postgres: each HTTP 200 case has the success text and not the error text; each HTTP 500 case has bill's text and `"code":2`). The failure tests are unchanged and still check bill's text.
- Deliberate failures: the old text on success fails `TestDeleteSuccess`, `TestDeleteOverGRPCWithAuth`, `TestMyBillDeleteOverHTTP` and `TestLiveMyBillDeleteEndToEnd`; the success text on failure (`errBillUnknown`) fails `TestDeleteClientErrors`, `TestDeleteNoUserSkipsTheDeleter`, `TestDeleteErrorIsUnknown`, `TestMyBillDeleteOverHTTP` and `TestLiveMyBillDeleteEndToEnd`.
- Coverage (conventions § 11, DSN set, live tests ran): `entry/grpc` 98.2% (the changed package); `entry/http` 88.4%.
- Docs: `docs/this_vs_bill.md` item 12 (item 11 text corrected), Postman `Delete` (description and test `has the success message`), `src/app/README.md`, CHANGELOG, OPEN_TASKS (019b scenario; 019 scenario updated).

## Verification result (Rule 15, feature ON)

Done 2026-11-08 by the implementer: the built App on the compose dev database (ports 18080/18082/19090), `debug.auth.disable: true`, a new test user.
- POST a water bill; DELETE it: HTTP 200 `{"message":"عملیات با موفقیت انجام شد"}`; DELETE it again: the same; DELETE `00000000-0000-4000-8000-0000000000ff`: the same; DELETE `not-a-uuid`: HTTP 500 `{"code":2,"message":"خطای داخلی رخ داده است","details":[]}`.
- A second run with SQL between the calls (a mobile bill): `enable` before, `disable` after the first DELETE (HTTP 200, success text), `disable` after the second (HTTP 200, success text). `GET /v1/mybill`: `"data":[]`.
- `qabz_mybill_delete_total`: `{result="found",type="water"} 2`, `{result="found",type="mobile"} 2`, `{result="not_found",type="unknown"} 1`. No error line in the log (`not-a-uuid` is a client error).
- The 2 rows of the test user were deleted after the run; the scratch config was removed.

Verdict: the output matches the design: the success text on each success case, bill's failure answer unchanged. Awaiting operator confirmation.
