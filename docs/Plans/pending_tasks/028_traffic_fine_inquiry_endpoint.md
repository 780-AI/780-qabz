<!-- VERSION: 0.1 -->
# 028_traffic_fine_inquiry_endpoint — POST /v1/traffic-fine/inquiry (parity with bill: no provider)

Status: pending

## Description

Implement `TrafficFine.Inquiry` (`POST /v1/traffic-fine/inquiry`, `pb/proto/qabz/services.proto`). The request is `InquiryTrafficFineRequest` (`billId`, `payId`) and the response is `InquiryTrafficFineResponse` (`price`), both in `pb/proto/bill/services.proto`. This task follows tasks 021 (water), 022 (electricity) and the other inquiry tasks, so that every bill type has one inquiry task.

**In `bill` this endpoint has no provider flow.** `controller/trafficfine/dto.go (`Controller.Inquiry`)` returns the internal-service user error (`InternalServiceErrorType`) for every request, and the seed has no `providers` row for (`trafficFine`, `inquiry`). So this task does NOT add a provider, an adapter, a mock or a config key. It makes Qabzino answer the same as `bill`: the endpoint exists, validates nothing beyond the proto, writes no `bill_inquiries` or `bill_inquiry_requests` row, and returns the same gRPC code, HTTP status and text as `bill` (check them against `bill`; the error mapping of task 021 does not apply to this error). If the operator later wants a real inquiry for this bill type, that is a new task that follows the pattern of tasks 021 and 022.

Help:
- `bill`: `controller/trafficfine/dto.go (`Controller.Inquiry`)`; `docs/this_vs_bill.md` items 14 to 22 and the Postman collection of tasks 021 and 022 are the pattern.
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).

Points for the design step:
- Where the handler lives (`entry/grpc/`, as `water.go`) and the exact error (code, HTTP status, text) of `bill`.
- A metric and a log line (conventions §§ 9, 12): reuse `qabz_inquiries_total{bill_type="trafficFine",...}` with a `result` value for "not supported", and one info line per call; no error log line unless the design step says so.

## Definition of Done

- `POST /v1/traffic-fine/inquiry` answers as `bill` does, with no database write and no provider call.
- Unit tests with mocks and stubs (no database): the answer of a valid request, the answer of an invalid request (a missing field), the metric.
- Integration tests (real Postgres, throw-away schema): a call writes no row in `bill_inquiries` and `bill_inquiry_requests`.
- Build, vet and tests (with `-race`) pass. `system.html`, `this_vs_bill.md`, the conventions doc (if a lasting rule is added), the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database.

1. Call `POST /v1/traffic-fine/inquiry` with a valid body. Pass: the same code, HTTP status and text as `bill`; no row is added to `bill_inquiries` or `bill_inquiry_requests`; the metric counts the call.
2. Call again with the same body. Pass: the same answer; still no row; no provider call (there is no provider).
3. Call with an invalid body (a missing required field). Pass: HTTP 400 with the invalid-parameter answer of the proto validation (as the other endpoints).
