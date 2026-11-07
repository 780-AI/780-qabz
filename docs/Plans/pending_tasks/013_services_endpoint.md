<!-- VERSION: 0.1 -->
# 013_services_endpoint — GET /v1/services returns the service list

Status: pending

## Description

Implement `Service.Get` (`GET /v1/services`, `pb/proto/qabz/services.proto`). It returns the list of services (bill types) from the `services` table of the `bill` database, as `ServicesResponse` (`name`, `isActive`, `tag`, `type`, `inquiry`, `icon`). The response matches the `bill` service where there is no conflict.

Decisions (operator, 2026-11-07):
- Table: `services` (the operator text said "providers"; the proto fields match `services`, not `providers`).
- Repository: the handler reads the list through a repository interface; the Postgres implementation uses a raw query (no ORM).
- Cache: in memory (per container) for `CacheServicesTTL time.Duration`, a const in a new file `src/config/constants.go`. Default `60 * time.Second`.
- No migration: the table belongs to the `bill` database and already exists.

Points for the design step:
- The `bill` behavior to match: the value of `serviceId`, the row order (`order` column?), whether inactive rows or a `lang` filter apply.
- The cache is safe for concurrent calls, and a database error is not cached.
- The middleware and monitoring rules of `Implementation_Plan.md` Step 6 (duration metric, recovery, tracing; serious errors only in logs).

## Definition of Done

- `GET /v1/services` returns the rows of `services` as `ServicesResponse`, with HTTP 200.
- A second call inside `CacheServicesTTL` does not query the database; a call after it does.
- Unit tests cover the handler, the cache and the error paths.
- Integration test: a test database with a mock `services` table and mock rows; the endpoint returns exactly these rows, as-is.
- Build, vet and tests pass. The Postman collection (conventions § 7), the docs and the CHANGELOG are updated in the same commit.

## Verification

Insert test services into the dev database of the App, run the App and call `GET /v1/services`. Pass: the inserted services are in the response, as-is. Then delete the inserted rows.
