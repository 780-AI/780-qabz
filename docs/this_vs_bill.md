<!-- VERSION: 0.8 -->
<!-- Last Updated: 2026-11-10 -->
<!--
CHANGE LOG
v0.8 — 2026-11-10 — Task 022: items 23 to 25 for POST /v1/electricity/inquiry (details saved in extra_data and returned from saved inquiries, an empty status of Tavanir, items 14-17 and 19-22 apply to electricity too).
v0.7 — 2026-11-09 — Task 021: items 14-22 for POST /v1/water/inquiry (invalid request and no user, no provider, provider down, provider has no inquiry, adapter details, request rows, inquiry_step, tiebreak of the lookup, amount above uint32).
v0.6 — 2026-11-08 — Task 020: item 13 for the provider pick (sorted order from memory, no Redis cache).
v0.5 — 2026-11-08 — Task 019b: item 12 for DELETE /v1/mybill/{id} (the success text); item 11 no longer names the success message as bill's.
v0.4 — 2026-11-08 — Task 019: item 11 for DELETE /v1/mybill/{id} (no log line for a call with no user). The answers of DELETE match bill, so they are not an item.
v0.3 — 2026-11-08 — Task 017: item 6 also covers GET /v1/mybill; items 9-10 for GET /v1/mybill (tiebreak of the order, error answer).
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

6. **`POST /v1/mybill` and `GET /v1/mybill`: a call with no user.**
   - **bill:** a call with no valid `user_id` fails the request check and gets the same `Unknown` answer as item 5 (`POST`); `GET` reads the bills of an empty user and fails in the database (`Unknown`).
   - **Qabz:** answers gRPC `PermissionDenied` (HTTP 403) with the message `unauthenticated`, the same answer as the JWT check (task 012). This happens only with `debug.auth.disable: true` and an empty `debug.auth.mock_user_id` (a set key gives every call that user); with the check on, a call with no valid token never reaches the handler.
   - **Reason:** a missing user is an auth fault, not a bad request.
   - **Code:** the `@todo` comments in `MyBills.Insert` and `MyBills.GetAll` in `src/entry/grpc/mybills.go`.

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

9. **`GET /v1/mybill`: the order of bills saved at the same time.**
   - **bill:** orders by `created_at DESC` only. Bills with the same `created_at` come in the order that Postgres gives, which can change from call to call.
   - **Qabz:** orders by `created_at DESC, id DESC`. Bills with the same `created_at` always come in the same order. All other rows come in the bill order (a NULL `created_at` first, as in bill).
   - **Reason:** a fixed order for the client and for the tests (task 017). Any order of tied rows is also a possible bill order, so a client sees no new behavior.
   - **Code:** the `@todo` comment on `listSavedBillsSQL` in `src/system/postgres/savedbill_repository.go`.

10. **`GET /v1/mybill`: the error answer.**
    - **bill:** a database error gives gRPC `Unknown` (code 2) with bill's Persian `database_error` message. This includes a `json_data` value that go-pg cannot decode (for example `"IsPinned": "true"`): the whole list fails.
    - **Qabz:** gives gRPC `Internal` (code 13) with the message `internal error` and no detail, and one error log line `mybill: list failed` with the cause. Both give HTTP 500. A bad `json_data` value fails the whole list too, as in bill (operator decision, task 017).
    - **Reason:** the same as item 2.
    - **Code:** the `@todo` comment at the `errInternal` answer in `MyBills.GetAll` in `src/entry/grpc/mybills.go`.

11. **`DELETE /v1/mybill/{id}`: no log line for a call with no user.**
    - **bill:** the handler reads the user with a plain type assertion (`ctx.Value("user_id").(string)`), so a call with no user panics. The recovery handler prints the stack and writes one warn-level line `service has panicked!`, then answers bill's `invalid_parameter_error` (`Unknown`, HTTP 500). A call with an empty user goes to the database and fails there (`database_error`, one error line).
    - **Qabz:** gives the same answer (`Unknown`, HTTP 500, the same Persian text) and writes no log line. This happens only with `debug.auth.disable: true` and an empty `debug.auth.mock_user_id`. The error answers of `DELETE` are bill's (operator decision, task 019): `Unknown` for an id that does not parse, an empty id and a database error. Like bill, Qabz writes no line for an invalid id and one error line for a database error (`mybill: delete failed`, with the cause).
    - **Reason:** conventions § 10: a client error is a common error and writes no log line, although the code is `Unknown` (operator decision 4, task 019).
    - **Code:** the `@todo` comment at the no-user answer in `MyBills.Delete` in `src/entry/grpc/mybills.go`.

12. **`DELETE /v1/mybill/{id}`: the success message.**
    - **bill:** a successful delete answers HTTP 200 with `message` = the text of `invalid_parameter_error`, "خطای داخلی رخ داده است" ("an internal error has occurred"). This includes the cases that change nothing (an unknown id, a bill of another user, a bill that is already disabled).
    - **Qabz:** answers HTTP 200 with `message` = "عملیات با موفقیت انجام شد" ("the operation was done successfully") in each of these cases. The HTTP status and every failure answer (`Unknown`, HTTP 500, with bill's text) stay as in bill.
    - **Reason:** clients read the error text on success as a failure (operator decision, task 019b, 2026-11-08).
    - **Code:** the `@todo` comment on `deleteSuccessText` in `src/entry/grpc/mybills.go`.

13. **Providers: the pick of a provider for a bill type and an action.**
    - **bill:** `GetProviderByBillTypeAndAction` (`domain/repository/provider/provider_dto.go`) runs `WHERE bill_type = ? AND action = ? AND status = 'enable' LIMIT 1` with no `ORDER BY`. The `priority` column is never used: the pick is any enabled row that Postgres returns first, and it can change with the physical row order. The pick is cached in Redis per (bill type, action) for `caching.provider`, so a change of the table shows only after the cache entry ends.
    - **Qabz:** `ProvidersService` (`src/provider/selector.go`) keeps every row in memory, sorted in each (bill type, action) group by `tmp_priority` (equal to `priority` after each load), then `id`. `PickProvider` returns the first enabled provider of that order: always the same provider for the same rows. There is no Redis cache: the list is loaded again from the table every `providers.general.load_interval` (default 1 minute), and a change shows after the next load.
    - **Reason:** a deterministic pick by the stored priority, read with no database or cache call on the hot path (operator, task 020, 2026-11-08; `system.html`, Provider Switcher).
    - **Code:** the `@todo` comment on `ProvidersService.PickProvider` in `src/provider/selector.go`.

14. **`POST /v1/water/inquiry`: an invalid request and a call with no user.**
    - **bill:** an invalid `billId` (empty, not 6 to 13 digits, wrong check digit) returns bill's `invalid_parameter_error` as a plain Go error: gRPC `Unknown`, HTTP 500, with the Persian text. A call with no `user_id` panics (`uuid.MustParse`) and the recovery handler answers the same.
    - **Qabz:** an invalid `billId` answers gRPC `InvalidArgument` (HTTP 400) with `invalid parameter` and writes no log line. A call with no user answers `PermissionDenied` (HTTP 403, `unauthenticated`). The rules that decide "invalid" are the same as in bill (`helpers.ValidBillID`).
    - **Reason:** the same as items 5 and 6.
    - **Code:** the `@todo` comments in `Water.Inquiry` and `Water.answerError` in `src/entry/grpc/water.go`.

15. **`POST /v1/water/inquiry`: no enabled provider.**
    - **bill:** `GetProviderByBillTypeAndAction` returns the raw database error (`pg: no rows in result set`), so the client gets gRPC `Unknown` (HTTP 500) with that text.
    - **Qabz:** answers gRPC `Unavailable` (HTTP 503) with the provider-down text of item 16 and writes one error log line (`water: no enabled provider for inquiry`). The same answer is given when the picked provider has no adapter in the registry (bill: `invalid_parameter_error`, `Unknown`).
    - **Reason:** a missing provider is an operations fault, not a server bug; no internal text reaches the client. Task 021 design decision 5.
    - **Code:** the `@todo` comment on `errProviderDown` in `src/entry/grpc/water.go`.

16. **`POST /v1/water/inquiry`: the call to the provider fails.**
    - **bill:** the gateway turns a gRPC error into `provider_inquiry_is_down_error`, and the service returns its text as a plain error: gRPC `Unknown`, HTTP 500.
    - **Qabz:** answers gRPC `Unavailable` (HTTP 503) with the same Persian text ("the inquiry from the provider is temporarily disabled") and writes one error log line (`water: provider inquiry failed`, with the cause), as conventions § 10 asks for a failed dependency call. The request row gets the status `failed`. A client that leaves during the call gets its context error and no log line.
    - **Reason:** the same as item 15.
    - **Code:** the `@todo` comment on `errProviderDown` in `src/entry/grpc/water.go`.

17. **`POST /v1/water/inquiry`: the provider has no inquiry for the bill.**
    - **bill:** an answer with no inquiries and a status other than `success` returns the provider message as a plain error: gRPC `Unknown`, HTTP 500.
    - **Qabz:** answers gRPC `FailedPrecondition` (HTTP 400) with the message of the provider (`inquiry failed` when it sent none). No log line: it is not a server fault. The request row gets the status and the trace ID of the provider; no inquiry is saved.
    - **Reason:** the provider answered; the bill cannot be inquired. It is not a server fault (task 021 design decision 4).
    - **Code:** the `@todo` comment in `Water.answerError` in `src/entry/grpc/water.go`.

18. **Qabzino adapter: the trace ID and an empty status.**
    - **bill:** for an answer with no inquiry, `QabzinoWater.Execute` drops the trace ID (`tid`) of the provider, so `bill_inquiry_requests.provider_trace_id` is empty. An empty status is stored as an empty status.
    - **Qabz:** keeps the trace ID in the request row for every answer, and stores `failed` for an empty status. The `billId` of an answer with no inquiry stays empty, as in bill.
    - **Reason:** the trace ID is what an operator needs to ask Qabzino about a failed call; an empty status has no meaning.
    - **Code:** the `@todo` comment on `Adapter.Inquiry` in `src/provider/qabzino/qabzino.go`.

19. **`bill_inquiry_requests`: one row for each provider call.**
    - **bill:** writes two rows for each call with the same `tid`: one with the status `initial`, one (new `id`) with the final status. The table has `UNIQUE (tid, status)` for this.
    - **Qabz:** writes one row with the status `initial` and updates it (`status`, `provider_trace_id`) after the call. A call that Qabz never finishes (a crash) leaves the `initial` row, as in bill.
    - **Reason:** the task Description asks for one row ("INITIAL, then the provider status and trace ID"). A reader that counts rows gets one row for each call; a reader of bill's pairs (two rows with one `tid`) must read one row.
    - **Code:** the `@todo` comment on `finishInquiryRequestSQL` in `src/system/postgres/inquiry_repository.go`.

20. **`POST /v1/water/inquiry`: `inquiry_step`.**
    - **bill:** repeats the provider call `providers.inquiry_step` times (for the paired types, tel and mobile, the second call uses the pair type; for water the pair type is water itself, so the call and the request rows repeat).
    - **Qabz:** one provider call, whatever `providers.inquiry_step` holds. The column is read and kept in `provider.Provider`, but the water flow does not use it.
    - **Reason:** the loop exists for the paired types; water has one inquiry. Task 021 design decision 3.
    - **Code:** `Service.fromProvider` in `src/core/inquiry/inquiry.go` (no loop).

21. **`POST /v1/water/inquiry`: the order of saved rows with the same time.**
    - **bill:** the lookup orders by `created_at DESC` and takes 2 rows. Rows saved in one transaction have the same `created_at`, so their order is not fixed.
    - **Qabz:** orders by `created_at DESC, id DESC`. Any order of tied rows is also a possible bill order.
    - **Reason:** a fixed answer for the client and for the tests.
    - **Code:** the `@todo` comment on `findValidInquiriesSQL` in `src/system/postgres/inquiry_repository.go`.

22. **`POST /v1/water/inquiry`: an amount above 4 294 967 295 rials.**
    - **bill:** `uint32(inq.Amount)` cuts the value, so the client gets a wrong price.
    - **Qabz:** answers gRPC `Internal` (HTTP 500) and writes one error log line (`water: inquiry amount does not fit the price field`).
    - **Reason:** a wrong price is worse than an error. The proto field is `uint32`; the amount of a water bill is far below the limit.
    - **Code:** the `@todo` comment in `Water.Inquiry` in `src/entry/grpc/water.go`.

23. **`POST /v1/electricity/inquiry`: the details of an answer from saved inquiries, and `extra_data`.**
    - **bill:** saves only `city` and `region` of the electricity details in `bill_inquiries.extra_data` (the other fields of `ElectricityDetails` have `json:"-"`). An answer from saved inquiries (`getFromCache`) has `Details: nil`, so the client gets the price, the pay ID and the deadline with empty `phase`, `companyName`, `tariffType`, `customerType`, `serialNumber`, `serviceAddress`, `servicePostCode` and `customerName`. Only the first call within `caching.electricity` has the details.
    - **Qabz:** saves the details that the client sees too, as more keys of the same JSON object: `city`, `region` (as bill) and `phase`, `company_name`, `tariff_type`, `customer_type` (a number), `serial_number`, `service_address`, `service_post_code`, `customer_name`. An empty value is not written, as in bill. An answer from saved inquiries has the same details as the answer of the provider. The copies of a database answer (`is_cached = true`) keep `extra_data`, as in bill. A saved row that bill wrote (only `city` and `region`) answers with empty detail fields and `customerType` `"0"`; an `extra_data` that is not a JSON object is logged (`inquiry: read saved inquiries failed`) and the provider is asked.
    - **Database effect:** `bill_inquiries.extra_data` of an electricity row now holds the customer name, the service address and the post code (personal data that bill did not store). A reader of `city` and `region` (the fee rules of bill's order flow) is not affected: the two keys are unchanged.
    - **Reason:** the task Definition of Done: "the answer and its details saved and returned the same on a cache hit" (task 022, design decision 2).
    - **Code:** the `@todo` comment on `electricityExtra` in `src/system/postgres/inquiry_details.go`.

24. **Tavanir adapter: an empty status.**
    - **bill:** an answer of Tavanir with no inquiry and an empty status is stored with an empty status in `bill_inquiry_requests`.
    - **Qabz:** stores `failed`, as for Qabzino (item 18). Tavanir sends no trace ID, so `provider_trace_id` stays NULL (as in bill). As in bill, an answer with no inquiry has no bill ID and no details; a `success` answer with no inquiry is one inquiry with amount 0; the saved `city` is the company name (bill maps it so: the backend has no city field).
    - **Reason:** the same as item 18.
    - **Code:** the `@todo` comment on `Adapter.Inquiry` in `src/provider/tavanir/tavanir.go`.

25. **`POST /v1/electricity/inquiry`: the differences of the water inquiry apply.**
    - **bill / Qabz:** the flow and the error mapping are the same code as for water (`core/inquiry`, `answerInquiryError` in `src/entry/grpc/water.go`), so items 14 (invalid request, no user), 15 (no enabled provider), 16 (the provider call fails), 17 (the provider has no inquiry), 19 (one request row), 20 (`inquiry_step`: one provider call, whatever the column holds), 21 (order of saved rows) and 22 (an amount above the `uint32` price field) apply to electricity with the log prefix `electricity:`.
    - **Reason:** one flow for every bill type (task 022: "Do not copy the water flow").
    - **Code:** `Electricity.Inquiry` in `src/entry/grpc/electricity.go` (the `@todo` comments name items 14 and 22).
