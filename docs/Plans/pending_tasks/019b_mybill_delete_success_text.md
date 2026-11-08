<!-- VERSION: 0.1 -->
# 019b_mybill_delete_success_text — DELETE /v1/mybill/{id} sends a success text on success

Status: pending

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
