<!-- VERSION: 0.1 -->
# 033_tavanir_provider_config — Tavanir provider config: four endpoints, credentials and circuit breaker

Status: done

## Description

Operator decision (2026-11-10): qabz does not send requests to the `tavanir` service. It implements the Tavanir provider itself, as task 030 did for Qabzino. This task adds the config that the direct Tavanir adapter needs (the `tavanir` service reads it from its own config as `tavanir.*`). The keys are config only: no code reads them yet. The existing keys `grpc_address` and `timeout` stay.

New keys under `providers.tavanir` (snake_case, as the other keys): `auth_endpoint`, `basic_branch_endpoint`, `bill_document_endpoint`, `branch_debit_endpoint` (the esrv.saapa.ir URLs), `username`, `password` (a `Secret`, never printed), and a `circuit_breaker` block: `name`, `max_requests` (uint32), `interval`, `timeout` (durations), `failure_ratio` (percent, 0 to 100, as in the `tavanir` service where it is divided by 100).

The real username and password go only into `config.yml` (git-ignored; `providers.tavanir.timeout` there is `120s`). The versioned `config.yml.example` and the code defaults hold the endpoint URLs, the circuit breaker values of the `tavanir` config, and an empty username and password.

## Definition of Done

- `config.Config` has `TavanirAuthEndpoint`, `TavanirBasicBranchEndpoint`, `TavanirBillDocumentEndpoint`, `TavanirBranchDebitEndpoint`, `TavanirUsername`, `TavanirPassword` (`Secret`), `TavanirCircuitBreakerName`, `TavanirCircuitBreakerMaxRequests`, `TavanirCircuitBreakerInterval`, `TavanirCircuitBreakerTimeout` and `TavanirCircuitBreakerFailureRatio`, with the defaults above. A `failure_ratio` above 100 is rejected.
- `config.yml.example` has the keys with default values and an empty username and password; the real values are only in `config.yml`.
- A config test checks the defaults, the values read from YAML, the `failure_ratio` check and that the password does not print.
- `system.html`, the CHANGELOG and the manifest are updated. Build, vet and tests (with `-race`) pass.

## Verification

Feature ON is not possible: no code reads the keys yet. Check with `go test -race -count=1 -run TavanirProviderKeys ./config/` from `src/`. Pass: the test passes.
