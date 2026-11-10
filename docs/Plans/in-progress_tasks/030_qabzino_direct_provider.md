<!-- VERSION: 0.1 -->
# 030_qabzino_direct_provider — `providers.qabzino.enabled` and a direct Qabzino (Ayantech) adapter; the gRPC adapter becomes QabzinoProxy

Status: in-progress

Priority (operator, 2026-11-10): this task is done next, before tasks 023 to 025.

## Description

Three changes, in this order.

1. **New key `providers.qabzino.enabled`** (bool). When it is `false`, Qabzino cannot be used: every Qabzino call fails as "provider down" and nothing is sent to Ayantech; a mock (`debug.mocks.qabzino`) does not bypass it. Default `false` (versioned `config.yml.example` and code), so a fresh config never sends a charged call. A start with `enabled: false` writes one warn line.
2. **Rename the gRPC adapter to QabzinoProxy.** `src/provider/qabzino/` (task 021, calls the old `qabzino` service over gRPC) becomes `src/provider/qabzinoproxy/` (package `qabzinoproxy`, provider name `qabzinoproxy`). It is kept, with its tests, but it is NOT wired into the App: nothing builds or registers it. `providers.qabzino.grpc_address` stays in the config and is read only by QabzinoProxy (unused for now).
3. **New direct adapter `src/provider/qabzino/`.** It implements `ProviderInterface` and calls Ayantech over HTTPS as the old `qabzino` service does (`client/qabzino/dto_new.go`, `gateway/water.go`): `POST` JSON `{"Identity":{"Token":<token>},"Parameters":{"WaterBillID":<bill ID>,"TraceNumber":<new UUID>}}` to the endpoint; the answer is `{"Parameters":{...},"Status":{"Code","Description"}}`. It uses `providers.qabzino.token` and `providers.qabzino.water_bill_endpoint` and `timeout`. The App registers this adapter under `qabzino` (the `providers.name` of the seed rows).

Scope of the direct adapter in this task: `Inquiry` of a **water** bill (the only Qabzino inquiry endpoint of qabz today). Every other method and bill type returns `provider.ErrNotSupported`. The keys `electricity_bill_endpoint` and `gas_bill_endpoint` stay parsed but unused: no route uses Qabzino for electricity (Tavanir, task 022), and gas needs the participate code that `provider.InquiryRequest` has no field for (task 023).

Answer mapping (as `waterBillInquiryMapper`): code `G00000` is success with one inquiry (`PaymentID`, `Amount`, `PaymentDate`; `BillID`); `GD2068` (already paid) is success with one inquiry of amount 0; `GD2061`, `GD2062`, `GD2067` are `failed` with bill's Persian text and the code; `GD2066` and every other code, a bad HTTP status, an unreadable body or a transport error are `provider.ErrProviderDown` (wrapped, with the code). The trace ID is the `TraceNumber` the adapter sent.

No real call to Ayantech in any test or verification: each call is charged. Tests use a local `httptest` server (and an injected HTTP doer); the feature-ON check uses the mock or the disabled switch only.

## Definition of Done

- `providers.qabzino.enabled` exists (`Config.QabzinoEnabled`, default `false`), in `config.yml.example` (default) and `config.yml` (`true`); disabled: no HTTP call, "provider down", the mock does not bypass it, one warn line at start.
- `provider/qabzinoproxy/` holds the former gRPC adapter and its tests (renamed), is not referenced by `app`.
- `provider/qabzino/` is the direct HTTP adapter described above; the App registers it with `token`, `water_bill_endpoint` and `timeout`; the mock (`MockFixtures`) still works through it in dev mode.
- Unit tests with a local `httptest` server only (no Ayantech): request body and headers, the token, the endpoint, success, paid (`GD2068`), refused codes, `GD2066`, unknown code, HTTP 500, bad JSON, timeout, canceled context, disabled, empty token and empty endpoint, other bill types; config and app wiring tests; the existing water flow tests pass.
- `system.html`, `this_vs_bill.md` (each visible difference with a `@todo`), the app README, the Postman collection if affected, the CHANGELOG and the manifest are updated in the same commit.
- Build, vet and all tests (`make test`, with `-race`) pass.

## Verification

No call reaches Ayantech. Run `make test` from the repo root (compose up). Pass: all pass, no skip. Feature ON (Rule 15), dev mode, compose dev database, `debug.mocks.qabzino.enable: true`: (1) `enabled: true` and the water provider (id 36) enabled: `POST /v1/water/inquiry` gives the mock answer (as task 021 step 1); (2) `enabled: false`: the same call gives HTTP 503 with the provider-down text, one error line, and no mock line (the switch wins); the start log has the warn line.
