<!-- VERSION: 0.2 -->
# 034_tavanir_direct_provider — the Tavanir provider is a direct client of the Tavanir (SAAPA) endpoints; no gRPC, no rename, no circuit breaker

Status: done

Depends on task 033 (the config keys), done.

## Operator decisions (2026-11-10, they replace the matching text of the Description below)

1. **Tavanir stays `tavanir`.** Nothing is renamed: there is no `tavanirproxy`. The package `src/provider/tavanir/` keeps its name and its provider name.
2. **No circuit breaker for now.** A circuit breaker is added to every provider at the end of the project. The keys `providers.tavanir.circuit_breaker.*` (task 033) stay parsed and are not used.
3. **Tavanir implements `ProviderInterface` and is, inside, a client of the Tavanir endpoints of the config.** It does not call the microservice named `tavanir`, not at all. So the gRPC client of that microservice is removed from `provider/tavanir`, the key `providers.tavanir.grpc_address` is removed (nothing reads it), and `make proto` no longer generates the Tavanir backend client.
4. Accepted with the design: `providers.tavanir.enabled` (default `false`); after an HTTP 401 the call logs in again and is sent one more time.

## Result

- `src/provider/tavanir/tavanir.go`: login (`auth_endpoint`, `username`, `password`), the token in memory (one login at a time; a 401 drops the token only when it is still the token of that call; one new login and one more send; a second 401 is provider down), the inquiry (`basic_branch_endpoint`, `Authorization: Bearer <token>`), the mapping with details, the status codes (19 refused codes give `failed` with the text; every other code is provider down), the system TLS roots, `providers.tavanir.timeout` for each HTTP call.
- The chain of `esrv.saapa.ir` verifies with the system roots (handshake only, 2026-11-10; no HTTP request was sent).
- Metrics (conventions § 12): no new metric. The login is part of the provider call that `qabz_provider_calls_total` and `qabz_provider_call_duration_ms` already count (`provider="tavanir"`); a failed login counts as `down`.
- Verification evidence (2026-11-10): `make test GO_TEST_ARGS="-race -count=1 -cover -v"`: 648 pass, 0 fail, 0 skip; `provider/tavanir` 98.0%, `app` 88.9%, `config` 97.6%. Positive controls (temporary overlay, source unchanged): with no token drop 4 tests fail; with no login lock 2 tests fail. Feature ON in the running App (ports 18080 / 18082 / 19090, scratch config with no real credential and no real endpoint): (1) mock on, `enabled: true`: HTTP 200 with the mock answer and details, the second call from the database (`provider_ok` 1, `db_hit` 1, one mock line); (2) mock on, `enabled: false`: HTTP 503, the warn line at start, one error line `providers.tavanir.enabled is false`, no mock line; (3) mock off, the real adapter against a local fake Tavanir server: one login for two bills, the token rejected once gives a second login and one more send, HTTP 200 with the mapped details (`payId` `2200012345`, deadline `2031-03-04`); status 507 gives HTTP 400 with `شناسه قبض متعلق به برق نمی باشد`; status 502 gives HTTP 503 and one error line `status 502`; the log has no password and no token. The test rows were deleted. No call reached `esrv.saapa.ir`.

## Description

Operator decision (2026-11-10): the Tavanir provider of qabz calls Tavanir (SAAPA, `esrv.saapa.ir`) itself over HTTPS. It does NOT call the internal `tavanir` service (gRPC) any more. The `tavanir` project is in the context only as a reference for the flow and the answer mapping.

Three changes, as task 030 did for Qabzino:

1. **A switch `providers.tavanir.enabled`** (bool, default `false`), as `providers.qabzino.enabled` (task 030): `false` means every Tavanir call fails as "provider down", nothing is sent to Tavanir, and the mock does not bypass it; one warn line at start. Decide in the design step whether this task needs it (recommended: yes, same reason: a fresh config never sends a call).
2. **Rename the gRPC adapter to TavanirProxy.** `src/provider/tavanir/` (task 022, gRPC to the `tavanir` service) becomes `src/provider/tavanirproxy/` (package `tavanirproxy`, provider name `tavanirproxy`), kept with its tests and NOT wired into the App. `providers.tavanir.grpc_address` stays and is read only by TavanirProxy.
3. **A new direct adapter `src/provider/tavanir/`** that implements `ProviderInterface` (`Inquiry` of `electricity`; every other method and bill type returns `provider.ErrNotSupported`). It uses the keys of task 033.

**The flow of the direct adapter** (from `tavanir`: `client/tavanir/tavanir_dto.go`, `tokenmanager_dao.go`, `service/tavanir/billInquiry.go`, `translator.go`, `pkg/restRequester/dto.go`; cite file and line in the design):
- **Authenticate:** `POST` JSON `{"username","password"}` to `providers.tavanir.auth_endpoint`; the answer is `{"status","message","data":{"token",...},"error":[...]}`; status 200 and a token are a success.
- **Token manager:** the token is kept in memory and reused. A `401` from a data endpoint drops it, and the next call authenticates again. `tavanir` does this with no lock; qabz serves calls at once, so the design must make it safe for concurrent use (one login at a time, no stampede after a 401) and decide if the failed call retries once with a new token.
- **No database in the adapter (checked 2026-11-10).** The `tavanir` service has Postgres, Redis, NATS and etcd, but its Tavanir flow needs only one database item: `Authenticate` decides success with `errors.Status.Get(entity.CODE, status)`, a table `errors` that the service loads from Postgres every hour (seeded by `cmd/seed_cmd.go`: code 200 is `SUCCESS`, the others `FAILED`). The direct adapter replaces it with a fixed rule: login `status` 200 with a non-empty token is success, anything else is a failed login (provider down). The inquiry mapping (`translator.go`) is in code, with no database. The adapter reads no table of the `tavanir` service and needs no Postgres, Redis, NATS or etcd.
- **Inquiry:** `POST` JSON `{"bill_identifier","MobileNo"}` to `providers.tavanir.basic_branch_endpoint` with the header `Authorization: Bearer <token>` (`tavanir` sends `AuthenticationType` + `AuthenticationToken` as `Authorization`). The answer maps as `prepareResponse`: one inquiry (`payment_identifier`, `total_bill_debt`, `payment_dead_line` cut to `yyyy-mm-dd`), `bill_identifier` as the bill ID, and the details (`phase`, `company_name`, `tariff_type`, `customer_type`, `serial_number`, `service_add`, `service_post_code`, customer name and family, `city` as region) in the same `provider.ElectricityDetails` as task 022. The mapping uses the numeric/string tolerance of the fields (`bill_identifier`, `payment_identifier`, `serial_number`, `service_post_code` can be numbers or strings).
- **Status codes:** `status` 200 (with no `error` items) is success; the codes 400 to 528 of `translator.go` give a failed answer with the Persian text, or provider down (500, 502, 506, 508, 509, 512, 513, 517, 524, 527, 528 in `tavanir` use the provider-down text). Decide the exact split, in the same way as task 030 (item 26 of `this_vs_bill.md`).
- **Circuit breaker:** `providers.tavanir.circuit_breaker.*` (task 033) as in `pkg/circuitbreaker/circuit.go` of `tavanir` (opens when `requests >= max_requests` and the failure ratio >= `failure_ratio / 100`, per `interval`; `timeout` is the open time). An open breaker is provider down. `bill` has none (system.html "Gap"); this adds one to qabz.
- **Timeout:** `providers.tavanir.timeout` limits each HTTP call.

**Not in this task:** `GetLastBillDocument` and `getbranchdebit` (document and debit): `ProviderInterface.GetDocument` stays `ErrNotSupported`, as `tavanir` serves the document by gRPC only for payment. The keys stay unused. The design says so in `this_vs_bill.md` if `bill` used them.

Points for the design step:
- The HTTP client: the system TLS roots (no `InsecureSkipVerify`), as task 030. Check that the `esrv.saapa.ir` chain verifies with them; if not, surface it to the operator, do not skip the check silently.
- The circuit breaker library: `github.com/sony/gobreaker` (what `tavanir` uses) would be a new dependency (`go mod vendor`, conventions § 1 on libraries); or a small own implementation. Surface it as an operator choice.
- Logging: the log never contains the password or the token (conventions § 12).
- The mock: `debug.mocks.tavanir` (task 022) keeps working through the direct adapter in dev mode; `enabled: false` wins over it.
- Existing tests of task 022 (`app` wiring, `electricity_live_test.go`) use the gRPC adapter's mock fixtures; they move to the direct adapter.
- Electricity inquiry stays `POST /v1/electricity/inquiry` with the same answer fields.

## Definition of Done

- `providers.tavanir.enabled` (if kept) exists with default `false` in code and `config.yml.example`, `true` in `config.yml`; disabled: no HTTP call, "provider down", one warn line.
- `provider/tavanirproxy/` holds the former gRPC adapter and its tests and is not referenced by `app`. `provider/tavanir/` is the direct HTTP adapter above, registered under `tavanir` with the keys of task 033.
- Authentication, token reuse, token drop on 401, the concurrency rule, the inquiry mapping with details, the status codes and the circuit breaker work as described.
- Unit tests with a local `httptest` server only (no call to `esrv.saapa.ir`): the login request body and answer; the Bearer header; token reused for two calls and a new login after a 401; many concurrent calls make one login; the success mapping with details (numbers and strings for the flexible fields); each status code class (success, failed with text, provider down); HTTP 500, bad JSON, timeout, canceled context; disabled, empty username, password or endpoint; the breaker opens and closes; other bill types give `ErrNotSupported`; the password and the token are never in an error or a log. A positive control for the token-drop and the breaker tests.
- Integration tests (real Postgres, throw-away schema): the electricity flow of task 022 runs on the direct adapter's mock (first call saves, second from the database).
- `system.html`, `this_vs_bill.md` (each visible difference with a `@todo`), the app README, the CHANGELOG and the manifest are updated in the same commit. Build, vet and all tests (`make test`, with `-race`) pass.

## Verification

No call reaches Tavanir (SAAPA): tests use a local `httptest` server; the feature-ON check uses the mock or the disabled switch only. `make test` from the repo root (compose up). Feature ON (Rule 15), dev mode, compose dev database, `debug.mocks.tavanir.enable: true`: (1) `enabled: true`: `POST /v1/electricity/inquiry` gives the mock answer with details (as task 022 step 1) and a second call is answered from the database; (2) `enabled: false`: the same call gives HTTP 503, one error line and no mock line (the switch wins); the start log has the warn line. A real call to Tavanir is a separate step that only the operator runs, with the real credentials in `config.yml`.
