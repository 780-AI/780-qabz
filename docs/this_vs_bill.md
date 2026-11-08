<!-- VERSION: 0.2 -->
<!-- Last Updated: 2026-11-08 -->
<!--
CHANGE LOG
v0.2 — 2026-11-08 — Task 016: items 5-8 for POST /v1/mybill (invalid request answer, no user, no log line for a client error, unique-key retry).
v0.1 — 2026-11-07 — Created (task 013). Items 1-4: GET /v1/services (inquiry as stored, error answer, empty Accept-Language, cache).
-->

# Qabz vs. the bill service — differences

This file lists the places where Qabz behaves differently from the old `bill` service. Qabz copies the behavior of `bill` where there is no conflict. Each item here is a known, intended difference that a client or an operator can see.

When the `bill` service shuts down, this file becomes obsolete and is removed.

Each item gives the endpoint or area, the `bill` behavior, the Qabz behavior, and the reason. An item that has a `@todo` comment in the code names that place, so the two stay linked.

---

1. **`GET /v1/services`: the `inquiry` field is sent as stored.**
   - **bill:** ignores the `services.inquiry` column and sends `inquiry: true` for every service. The column has no effect. The value was hard-coded in bill commit `00b3dfb` ("fix dev problem", 2023-02-14), six days after the column was added (`68971a6`). The commit gives no reason.
   - **Qabz:** sends the value of `services.inquiry` as it is in the database. A row with `inquiry = false` gives `"inquiry": false`. A NULL gives `false`.
   - **Reason:** the endpoint returns the rows as they are (operator decision, task 013, 2026-11-07).
   - **Effect:** clients see a change only for rows where `inquiry` is not `true`. Find them with `SELECT type, lang, inquiry FROM services WHERE inquiry IS NOT TRUE;` on the `bill` database before Qabz takes this endpoint.
   - **Code:** the `@todo` comment on `Service.Inquiry` in `src/core/billtype/services.go` and on the `Inquiry` field in `Get` in `src/entry/grpc/service.go`.

2. **`GET /v1/services`: the error answer.**
   - **bill:** a database error gives gRPC `Unknown` (code 2) with bill's Persian `database_error` message.
   - **Qabz:** gives gRPC `Internal` (code 13) with the message `internal error` and no detail. Both give HTTP 500.
   - **Reason:** `Internal` is the correct gRPC code for a server fault; Qabz has no error-message table yet (`common/errors/` is empty).
   - **Code:** `errInternal` in `src/entry/grpc/service.go`.

3. **`GET /v1/services`: an empty `Accept-Language` header.**
   - **bill:** uses the empty value as the language (`WHERE lang = ''`), so the list is empty.
   - **Qabz:** treats an empty value as a missing header and uses `fa-IR`. A missing header gives `fa-IR` in both.
   - **Reason:** an empty list for an empty header has no use for a client.
   - **Code:** `requestLang` in `src/entry/grpc/service.go`.

4. **`GET /v1/services`: the cache.**
   - **bill:** caches the list in Redis for 1 hour, per language, shared by all containers.
   - **Qabz:** caches the list in memory for 60 seconds (`config.CacheServicesTTL`), per language, in each container.
   - **Reason:** operator decision (task 013). A change in the `services` table shows within 60 seconds.
   - **Code:** `ServiceCatalog` in `src/core/billtype/services.go`.

5. **`POST /v1/mybill`: the answer to an invalid request.**
   - **bill:** returns a plain Go error with bill's Persian invalid-parameter message. gRPC sends it as `Unknown` (code 2), so the gateway answers HTTP 500.
   - **Qabz:** answers gRPC `InvalidArgument` (code 3) with the message `invalid parameter` (HTTP 400). The rules that decide "invalid" are the same as in bill.
   - **Reason:** an invalid request is a client error, not a server fault (task 016 Definition of Done). Qabz has no error-message table yet (`common/errors/` is empty).
   - **Code:** the `@todo` comment on `errInvalidParameter` in `src/entry/grpc/mybills.go`.

6. **`POST /v1/mybill`: a call with no user.**
   - **bill:** a call with no valid `user_id` fails the request check and gets the same `Unknown` answer as item 5.
   - **Qabz:** answers gRPC `PermissionDenied` (HTTP 403) with the message `unauthenticated`, the same answer as the JWT check (task 012). This happens only with `debug.auth.disable: true` and an empty `debug.auth.mock_user_id` (a set key gives every call that user); with the check on, a call with no valid token never reaches the handler.
   - **Reason:** a missing user is an auth fault, not a bad request.
   - **Code:** the `@todo` comment in `MyBills.Insert` in `src/entry/grpc/mybills.go`.

7. **`POST /v1/mybill`: no log line for an invalid request.**
   - **bill:** writes one error-level log line for each invalid request, with the request and the user agent.
   - **Qabz:** writes no log line. The call is counted in `qabz_request_duration_ms{code="InvalidArgument"}`. A failed database write still writes one error line (`mybill: insert failed`).
   - **Reason:** the logging policy (Implementation_Plan Step 5): serious errors only, and a client error is not one.
   - **Code:** the `@todo` comment in `MyBills.Insert` in `src/entry/grpc/mybills.go`.

8. **`POST /v1/mybill`: a broken unique key.**
   - **bill:** go-pg `SelectOrInsert` tries up to 5 times. After the first two tries it waits with a backoff (about 1.75 s in total), then answers a server error. This happens when the user has a disabled (deleted) row with the same type and bill ID.
   - **Qabz:** looks for the enabled row once more, with no wait. A row that another call saved at the same time is found and its id is returned. Otherwise the answer is gRPC `Internal` (HTTP 500), as in bill (operator decision, task 016).
   - **Reason:** the extra tries cannot find a disabled row, so they only make the error slower.
   - **Code:** the `@todo` comment on `SavedBillRepository.Insert` in `src/system/postgres/savedbill_repository.go`.
