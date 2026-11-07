<!-- VERSION: 1.0 -->
# 013_services_endpoint — GET /v1/services returns the service list

Status: done

## Description

Implement `Service.Get` (`GET /v1/services`, `pb/proto/qabz/services.proto`). It returns the list of services (bill types) from the `services` table of the `bill` database, as `ServicesResponse` (`name`, `isActive`, `tag`, `type`, `inquiry`, `icon`). The response matches the `bill` service where there is no conflict.

Decisions (operator, 2026-11-07):
- Table: `services`.
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

## Decisions (operator, 2026-11-07)

- Match `bill` (`~/projects/bill`, `controller/services/dao.go`, `domain/repository/service/service_dao.go`): rows of the `Accept-Language` language (raw `grpcgateway-accept-language` value, `fa-IR` when missing), `ORDER BY "order"` (plus `id` as a tie-breaker), active and inactive rows.
- `inquiry` is sent as stored. `bill` always sends `true`. The difference has a `@todo` comment in the code and is item 1 of the new `docs/this_vs_bill.md`.
- `serviceId`: a new config key `service.id` (default 0; the local `config.yml` has bill's 105). The operator did not choose between the key and a hard-coded 105; the key was the recommendation.
- The cache key is the language. At most 16 languages are cached, and a language value longer than 35 bytes is never cached, so random header values cannot fill the memory.

## Result (2026-11-07)

- `config/constants.go`: `CacheServicesTTL = 60 * time.Second`. `config/config.go`: `ServiceID` (`service.id`, 0 to 2147483647); `config.yml.example` and `config.yml` have the key.
- `core/billtype/services.go`: `Service`, `ServiceRepository`, `ServiceCatalog` (per-language TTL cache, failed reads not cached, copies returned, `OnResult` hit/miss/error).
- `system/postgres/service_repository.go`: raw query on `services`; NULL text or boolean reads as `""` or `false`. `system/postgres/postgrestest/`: throw-away schema with bill's `services` table (tests only), with its own live test.
- `entry/grpc/service.go`: `NewService(ServiceDeps)`, `Get`; a failed read gives `Internal` with no detail and one error log line with the cause; a canceled call gives `Canceled` and no log line. `app/defaults.go`: wiring and the metric `qabz_services_cache_total{result}`.
- Tests: unit (catalog, handler, config, wiring, gateway with a fake list) and integration (`TestLiveListServices`, `TestLiveServicesEndToEnd`: mock rows in a mock table through repository, cache, gRPC and gateway come back exactly as inserted). `go build`, `go vet`, `go test -race -count=1 ./...` pass; the live tests pass on the compose Postgres.
- Deliberate failure (overlays): `inquiry` forced to `true` fails 4 tests (handler, gateway, end-to-end); no cache store fails 4 catalog tests; no language limit fails `TestListLimitsCachedLangs`.
- Feature ON (App binary, dev database): bill's `services` table and 4 QA rows inserted; `GET /v1/services` gave HTTP 200, `serviceId` 105 and the 3 `fa-IR` rows in order, as inserted (`inquiry: false` kept); `Accept-Language: en-US` gave the 1 `en-US` row; after a delete of the rows the next call still gave the list (cache); metric `hit` 1, `miss` 2. With the table dropped, a fresh App gave HTTP 500 `{"code":13,"message":"internal error"}` and metric `error` 1. After the operator saw no log line, an error log was added (`services: list failed` with the cause): the same run then logged `relation "services" does not exist (SQLSTATE 42P01)`; the response stayed without detail. The QA rows and the table were removed after the check.
- Postman: new request `Service / Services` (conventions § 7).
- Code review (`code-reviewer` agent): NOT READY for one defect, `postgrestest` without tests (0%). Fixed with `postgrestest_test.go` (78.1% with the live DSN). Minor points fixed: `COALESCE(type, '')`, a length check on `rand.Text()`, the shared test helper moved to `services_test.go`, three more differences in `docs/this_vs_bill.md`. Accepted as is: a canceled read counts as `error` in the metric; `ServiceIDFromConfig` repeats the config range check.

## Open items at close (2026-11-07)

- The Step 6 middleware (request duration, recovery, tracing) does not exist yet for any endpoint, Greeting included. It is cross-cutting, so it is a separate task, not part of this one.
- Confirm `service.id` (key, default 0) or a hard-coded 105.
- Check the real `bill` database for rows with `inquiry IS NOT TRUE` before Qabz serves this endpoint (`docs/this_vs_bill.md`, item 1).
