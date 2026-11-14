<!-- VERSION: 0.1 -->
# 041_providers_internal_external — split the `providers` config section into `internal` and `external`

Status: pending

## Description

Today each provider is a direct child of `providers` (`providers.irancell.*`, `providers.karashop.*`, ...). This task puts each provider in one of two new sub-sections:

- **`providers.internal.*`**: a provider that is an internal service of our company (we call our own service). Today: `irancell` (the irancell service), `jiring_switch` (the jiring-switch service), `parsian` (the parsian-v2 service). A later internal service (for example the payment service) goes here.
- **`providers.external.*`**: a provider that is an external service (we call a third party). Today: `karashop` (the Karashop / digitalport HTTPS API), `qabzino` (the direct adapter calls Ayantech over HTTPS, task 030), `tavanir` (the Tavanir / SAAPA HTTPS API).

**New internal group `providers.internal.paymentServer`** (operator, 2026-11-14): the payment service, with the keys of `bill`:

```yml
paymentServer:
  host: "payment"
  port: 8084
  privateKeyPath: "paymentPrivateKey.pem"
```

These are the values of the local `config.yml`. The keys are not read yet (task 039 reads `privateKeyPath`).

Each key path changes, for example `providers.karashop.timeout` becomes `providers.external.karashop.timeout` and `providers.irancell.grpc_address` becomes `providers.internal.irancell.grpc_address`. The keys under each provider, their values, defaults and checks do not change. The runtime behavior does not change: this is a config layout change only.

Files that name the key paths today (read on 2026-11-14): `src/config/config.go` (the `path` tags), `src/config/config_test.go`, `src/app/defaults.go`, `src/app/*_test.go`, `src/provider/*/` (comments and tests), `config.yml.example`, the local `config.yml` (git-ignored, has the real values), `src/app/README.md`, `docs/initial-discovery/system.html`, `docs/this_vs_bill.md`, `docs/folder_structure.md`, `docs/Plans/OPEN_TASKS.md`, `qabz.postman_collection.json`. The implementer greps again before the edit.

Design (operator decisions, 2026-11-14):
- **`providers.general`** (`load_interval`) is a setting of `ProvidersService`, not a provider. It stays `providers.general`.
- **`debug.mocks.*`** is keyed by the provider name, not by the config path. It does not change.
- **`providers.qabzino.grpc_address`** (read only by QabzinoProxy, not wired into the App) moves with the other Qabzino keys to `providers.external.qabzino`.
- **The `paymentServer` key names are the names of `bill`** (camelCase: `paymentServer`, `privateKeyPath`), not snake_case. **The defaults are the zero values** (`""`, `0`, `""`) in the code and in `config.yml.example`; the values above go only in the local `config.yml`.
- **The old layout is forgotten.** Every file changes to the new layout. No check, warn line or compatibility for the old paths: the config loader ignores a missing path (`src/config/decode.go` `lookup`), so a config with the old layout loads the defaults (each provider disabled).
- The env names that `scripts/yml_to_env` makes from the key paths change (for example `PROVIDERS_EXTERNAL_KARASHOP_TIMEOUT`). No file reads the old names (`docker-compose.yml`, `Makefile`, `.github/workflows/`; grep on 2026-11-14).

## Definition of Done

- Each provider key is under `providers.internal.<provider>` or `providers.external.<provider>`, as listed in the Description. No key stays at `providers.<provider>`.
- `providers.internal.paymentServer.host`, `port` and `privateKeyPath` exist (`Config` fields, zero defaults, in both config files).
- `config.yml.example` and the local `config.yml` have the new layout, with the same values (conventions § 8). The real values in `config.yml` are moved, not changed and not shown in the chat.
- The `path` tags in `src/config/config.go` and every other reference to the old key paths (code, tests, docs, Postman) use the new paths; a grep for the old paths finds no match outside `CHANGELOG.md` and `done_tasks/`.
- No runtime behavior change: the same `Config` values for the same config content.
- `system.html`, the conventions doc (if a lasting rule is added: for example "a new provider goes under `internal` or `external`") and the CHANGELOG are updated in the same commit.

## Test

Run all existing tests, to make sure that the change breaks nothing:
- In `src/`: `go build ./... && go vet ./... && go test -race -count=1 ./...`.
- From the repo root: `make test GO_TEST_ARGS="-race -count=1"` (the live tests in a throw-away database) and `make test-scripts`.

Pass: all commands exit 0, no test fails, and `TestLocalConfigInSync` runs (it is not skipped) and passes.

## Verification

Feature ON (Rule 15): load the real config and check the lists.

1. Load `config.yml.example` and the local `config.yml` with the config loader of the App (`config.Parse` or `go run . --config=../config.yml`, then stop it).
   Pass: the load gives no error; the leaf paths under `providers.internal` are exactly the keys of `irancell`, `jiring_switch` and `parsian`; the leaf paths under `providers.external` are exactly the keys of `karashop`, `qabzino` and `tavanir`; for example `providers.external.karashop.enabled` and `providers.internal.irancell.grpc_address` exist, and `providers.karashop.enabled` does not.
2. Compare the `Config` values of the old and the new layout.
   Pass: each provider field (`KarashopEnabled`, `IrancellGRPCAddress`, `TavanirTimeout`, ...) has the same value as before the change.
3. Start the App with the local `config.yml` and call `POST /v1/tel/inquiry` (Karashop, external) and a mobile inquiry of Irancell (internal), with the mocks or the real services as the operator chooses.
   Pass: the answers are the same as before the change; the start log has the same warn lines (for example a disabled provider) as before.
