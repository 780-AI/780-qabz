<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-16 — Created and done (operator request, during the design of task 053): no HTTP route of Qabz has the version `/v2`; every route is `/v1`.
-->
# 057_no_v2_routes — Every HTTP route is /v1

Status: done (2026-11-16; the pb commit waits for the operator to push; the operator QA waits for the operator)

## Description

Qabz is a new project, so it has one API version: `/v1`. `bill` has `/v2` routes only because it is old (the `/v1` order routes keep the first, short order token and the `/v2` routes have the full token). The proto of Qabz (`pb/proto/qabz/services.proto`, the part of `pb` that this project owns, conventions § 6) had both for every order: `NewOrder` on `/v1/<type>/order` and `NewOrderV2` on `/v2/<type>/order`. Qabz served only the V2 methods; the V1 methods answered `Unimplemented`.

Decision (operator, 2026-11-16): drop the V1 methods and the `/v2` routes; the V2 method becomes `NewOrder` on `/v1/<type>/order`, with the full token that Qabz already makes (the eleven `data` keys, `payAmount`, the fee). A new conventions rule (§ 18) says that no Qabz route has another version.

## Definition of Done

- `pb/proto/qabz/services.proto` has one `NewOrder` for each of `Service` (`/v1/services/order`), `Gas`, `Water`, `Phone`, `Tel`, `Electricity` and `MyBills`, all on `/v1`; no `NewOrderV2` and no `/v2` route. After `make proto`, the generated gateway has no `/v2` route of Qabz.
- The Go handlers are named `NewOrder` (mobile, tel, water, electricity); the answers and the token do not change; the old tests "`POST /v1/<type>/order` stays `Unimplemented`" are removed.
- The Postman collection, `system.html`, `this_vs_bill.md`, the README, `folder_structure.md`, `OPEN_TASKS.md` and the tasks that are not done name only `/v1` routes; the QA scenario is in `OPEN_TASKS.md`; conventions § 18 is the rule.
- Build, vet and tests (with `-race`, live tests included) pass.

## Implementation notes (implementer, 2026-11-16)

- **Proto:** the `NewOrder` (V1) block of each of the seven services is removed and `NewOrderV2` is renamed `NewOrder` with the `/v1` path (`pb/proto/qabz/services.proto`; the commit is in the `pb` submodule). `make proto` regenerated `src/api/` (git-ignored).
- **Go:** `NewOrderV2` → `NewOrder` in `entry/grpc/{phone,tel,water,electricity}*.go`, `utilityOrder.newOrderV2` → `newOrder`, the tests and the comments; the references to `bill`'s own `NewOrderV2` stay (it is bill's name). The two "positive control" checks of `TestDefaultBuilderServesWater` and `TestDefaultBuilderServesPhone` use `CompleteOrder` (still `Unimplemented`) in place of the old `NewOrder`.
- **Left as it was (history):** the change-log lines of the documents, `CHANGELOG.md` and `docs/tasks/done_tasks/` name the `/v2` routes of the time they were written. The `bill` rows of the endpoint table in `system.html` name `bill`'s real routes (`bill` is not Qabz). The Postman item "payment service invoice" calls `/payment/v2/product/invoice` of the payment service (another service; Qabz does not own that route).

## Verification record (implementer, 2026-11-16)

- `go build ./...`, `go vet ./...`; `make test GO_TEST_ARGS="-race -count=1"`: exit 0 (the live tests of the water and electricity orders ran with `-v` and did not skip).
- The generated gateway `src/api/pb/qabz/services.pb.gw.go` has the patterns `v1/{services,gas,water,mobile,tel,electricity,mybill}/order` and no Qabz `/v2` pattern (the two `v2` words in the file are the `grpc-gateway/v2` import path).
- Not done: the operator QA (Postman calls of the order routes on `/v1`; a call of an old `/v2` route answers HTTP 404); the push of the `pb` commit (the operator pushes; the pointer of `pb` in this repository needs that commit on the remote).
