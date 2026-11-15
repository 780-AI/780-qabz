<!-- VERSION: 0.2 -->
<!--
CHANGE LOG
v0.2 — 2026-11-15 — Started (operator: "add an enable [bool] config key"): moved to in-progress_tasks; operator decisions 2026-11-15, all recommended: D1 (a), D2 (a), D3 (a), D4 (a); design D5 to D8 written.
v0.1 — 2026-11-15 — Created (operator request): a bool switch for tracing, and one name (`enabled`) for every on/off config key.
-->
# 051_tracing_switch_and_enabled_keys — the tracing switch and one name for the on/off keys

Status: in-progress (design approved 2026-11-15)

## Description

Operator request, 2026-11-15: "add an enable [bool] config key" for tracing, and "make sure such keys are consistent (I don't like to have enabled at one place and enable at the other)".

**Tracing switch.** Today tracing is on when `logs.otlp_endpoint` is not empty (task 003; task 047 uses it; `src/app/defaults.go:179`). A bool key turns tracing on and off, and the endpoint key gives only the address. The design step decides the place and the names of the keys (decision D1) and what a switch that is on with an empty endpoint does (D2). With the switch off (the default), the behavior is the same as today with an empty endpoint: no span, no `traceparent` header, no connection.

**One name for the on/off keys.** The config of today (`src/config/config.go`, `config.yml.example`):
- `enabled` (9 keys): `metrics.prometheus.enabled`, `providers.internal.{irancell,jiring_switch,parsian}.enabled`, `providers.internal.parsian.get_status.enabled`, `providers.external.{qabzino,tavanir,karashop}.enabled`, `confirm_payment.telephone.enabled`.
- `enable` (6 keys): `debug.mocks.{qabzino,tavanir,irancell,jiring_switch,parsian,karashop}.enable`.
- `disable` (1 key): `debug.auth.disable` (a negative switch; decision D3).
- Not changed: `is_lockable` (2 keys; bill keys that Qabz does not read).

`enabled` is the name of most keys, so the 6 `enable` keys become `enabled` (also the Go fields `DebugMock<Provider>Enable` become `...Enabled`). The new tracing switch is `enabled` too.

**An old key in a local config.** The loader does not reject a key that it does not know, so an old `debug.mocks.<provider>.enable: true` in a `config.yml` is dropped with no message, and the mock goes off. The design step decides how a renamed key is handled (D4).

**Conventions.** § 8 gets the rule: an on/off switch is a bool key named `enabled` (not `enable`, not `disable`, not `is_<x>`), default `false` unless the operator decides otherwise. § 15 (`debug.mocks.<provider>.enable`) changes to `enabled`. A test in `src/config/example_test.go` fails on a path that ends in `.enable` or `.disable`.

**Decisions for the operator (the design step).**
- **D1. The tracing keys.** (a) `logs.tracing.enabled` and `logs.tracing.otlp_endpoint` (the endpoint moves into the block). (b) `logs.otlp_enabled` next to `logs.otlp_endpoint`. (c) bill's names in snake case: `jaeger.enabled`, `jaeger.host_port` (still the OTLP gRPC address, port 4317, not bill's agent port 6831) and `jaeger.log_spans`.
- **D2. On, with an empty endpoint.** (a) The App does not start (a config error that names the key). (b) Tracing stays off with one warning line.
- **D3. `debug.auth.disable`.** (a) Keep it (a different word, not a variant of `enabled`). (b) Change it to `debug.auth.enabled` (default `true`; the value is the opposite of today).
- **D4. A renamed key in a config file.** (a) The App does not start when a config file has an old key; the error names the new key. (b) The old key is dropped with no message (as today for any unknown key).

## Design (operator decisions 2026-11-15, all recommended)

- **D1 (a).** `logs.tracing.enabled` (bool, default `false`) and `logs.tracing.otlp_endpoint` (string, default `""`; `host:port` of the OTLP gRPC receiver, no scheme). `logs.otlp_endpoint` is removed. Go fields: `TracingEnabled`, `TracingOTLPEndpoint` (was `OTLPEndpoint`). Env names from `yml_to_env`: `LOGS_TRACING_ENABLED`, `LOGS_TRACING_OTLP_ENDPOINT`.
- **D2 (a).** `validate` adds the error `logs.tracing.otlp_endpoint is required while logs.tracing.enabled is true`. `src/app/defaults.go` starts tracing only when `TracingEnabled` is true. The switch off with an endpoint set: tracing is off (no span, no header, no connection).
- **D3 (a).** `debug.auth.disable` stays. The § 8 rule names it as the one exception.
- **D4 (a).** A table `renamedKeys` in `src/config/config.go` (old path → new path): `logs.otlp_endpoint` → `logs.tracing.otlp_endpoint` and `debug.mocks.<provider>.enable` → `debug.mocks.<provider>.enabled` for the 6 mocks. `Parse` checks it after the debug section is removed (outside `dev` the debug keys are dropped as before, so an old mock key does not stop a prod start) and before the decode. Each old key found gives the error `<old> was renamed to <new>`; all are returned together with `errors.Join`.
- **D5. The mock keys.** `debug.mocks.<provider>.enable` → `.enabled`; Go fields `DebugMock<Provider>Enable` → `DebugMock<Provider>Enabled` (config, `src/app/defaults.go`, the tests). `provider.MockSettings.Enable` is a Go field, not a config key: not changed.
- **D6. The rule test.** `TestOnOffKeysNamedEnabled` in `src/config/example_test.go`: no `Config` path ends in `.enable`, and none ends in `.disable` except `debug.auth.disable`. A positive control: the check fails on a list with `debug.mocks.x.enable`.
- **D7. Docs.** Conventions § 8 (the rule), § 15 (`.enabled`), § 17 (the switch); `system.html` Logs section; `src/app/README.md`; `config.yml.example` and the local `config.yml`; the compose comment (`logs.tracing.*`). Task 050 names no mock key: not changed.
- **D8. No bill difference.** bill has `jaeger.hostPort` and no switch; `this_vs_bill.md` gets no item in this task (D1 (a), not bill names). The operator's open question on bill's Jaeger keys is answered by D1.

## Definition of Done

- Tracing is on only when the switch is `true`; with the switch `false` (the default) there is no span, no header and no connection, also when an endpoint is set. D2 is implemented and tested.
- No config path ends in `.enable`; every on/off key is `enabled` (D3 applied); a test in `example_test.go` proves it. D4 is implemented and tested.
- `config.yml.example` and the local `config.yml` have the same keys (§ 8).
- Conventions §§ 8 and 15, `system.html`, `src/app/README.md`, `this_vs_bill.md` (if D1 (c)), the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Build, vet and `make test` (with `-race`) pass.

## Verification

Feature ON (Rule 15): the compose Jaeger, the switch `true` with `localhost:4317`: one request shows as one trace in Jaeger. The switch `false` with the endpoint still set: no connection to port 4317 (`lsof`) and no trace. A mock with `enabled: true` answers; a config with the old `enable` key gives the result of D4.
