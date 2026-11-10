<!-- VERSION: 0.1 -->
# 029_qabzino_provider_config — Qabzino provider config: token and bill endpoints

Status: done

## Description

Add four keys to the `providers.qabzino` section: `token` (a secret), `electricity_bill_endpoint`, `gas_bill_endpoint` and `water_bill_endpoint` (the Ayantech inquiry URLs that the old `qabzino` service reads from its own config as `qabzino.token` and `qabzino.*BillEndpoint`). The keys are config only: no code reads them yet. The existing keys `grpc_address` and `timeout` stay.

The real token and URLs go only into `config.yml` (git-ignored). The versioned `config.yml.example` and the code defaults hold the default values: an empty token and the public endpoint URLs of the `qabzino` `config.example.yaml`.

## Definition of Done

- `config.Config` has `QabzinoToken` (`Secret`, never printed), `QabzinoElectricityBillEndpoint`, `QabzinoGasBillEndpoint` and `QabzinoWaterBillEndpoint`, with the defaults above.
- `config.yml.example` has the four keys with default values; the real values are only in `config.yml`.
- A config test checks the defaults, the values read from YAML and that the token does not print.
- `system.html`, the CHANGELOG and the manifest are updated.
- Build, vet and tests (with `-race`) pass.

## Verification

Feature ON is not possible: no code reads the keys yet. Check the keys with `go test -race -count=1 -run QabzinoProviderKeys ./config/` from `src/`. Pass: the test passes.
