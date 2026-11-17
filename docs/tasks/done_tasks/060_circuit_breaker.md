<!-- VERSION: 0.3 -->
<!--
CHANGE LOG
v0.3 — 2026-11-17 — Done: implemented, all tests pass with the run database and -race, feature-ON run on the dev database passed; implementation notes and verification record added; moved to done_tasks.
v0.2 — 2026-11-17 — Started (operator: "OK go on"): moved to in-progress_tasks; design D1 to D9 and decisions 1 to 8 approved; decision 5 changed by the operator (no switch for each provider for the time being).
v0.1 — 2026-11-17 — Created (operator decisions of 2026-11-17): the circuit breaker of each provider row, its config keys and the start of a demotion.
-->
# 060_circuit_breaker — circuit breaker: the error rate of each provider row starts a demotion

Status: done

## Description

`system.html` names a gap: `bill` has no circuit breaker, and no provider adapter of Qabz has one (operator decision of task 034: all providers get it at the end of the project). This task adds it. The breaker counts the error rate of each provider row. At the threshold it asks the Provider Switcher (task 059) to demote the row. It needs task 059.

**Operator rules (2026-11-17):**
- **The count is for each provider row** (one provider, bill type and action), not for each provider: Qabzino can fail for one bill type and work for another.
- **The rule to open:** in the sliding `window`, the row has `min_calls` calls or more and the failed calls are `failure_ratio` percent or more. `min_calls` is the smallest number of calls for which the percentage is trusted; it is not a number of failed calls.
- **A failed call** is only a provider-down result: a timeout, a connection error, or an answer that maps to HTTP 503 (an error that wraps `provider.ErrProviderDown`). A business answer (no debt, a refused bill ID, an invalid number) is a good call.
- **The breaker never blocks a call.** It only demotes, and only when the group has a next available provider (task 059). With none, it does nothing for that group and the requests keep going to the row.
- **`open_time`** is the time a row stays demoted before the trial inquiry (task 059). It is the one key for the open time and the demotion period.
- **Config**, all hot-reloadable; each provider can override each key:

```yaml
providers:
  general:
    circuit_breaker:
      enabled: false      # false: no count, no demotion (the behavior before this task)
      failure_ratio: 30   # percent
      window: "2m"
      min_calls: 20
      open_time: "1m"
  external:
    tavanir:
      circuit_breaker:    # optional; a key that is not set takes the general value
        window: "5m"
```

## Definition of Done

- A breaker for each provider row with the rule above, on a sliding window. The counts are in memory in each instance; a load of the `providers` table keeps the count of a row that stays.
- Each flow that calls a provider reports the result of the call for its row: the inquiry flow, the payment step (`order.Payments.Complete`) and the status workers (see open point 1). The report is not on the path of the answer: it adds no database call and no wait.
- At the threshold the breaker calls the demotion of task 059 with `open_time`. The result of the trial inquiry closes the breaker (the count starts new) or demotes again.
- The config keys of the snippet: the general block with defaults and checks (`failure_ratio` 1 to 100, durations greater than 0, `min_calls` 1 or more), and the override keys for every provider, `internal` and `external` (conventions § 8: `Config`, `config.yml.example` and the config example of `system.html` have the same keys). The `@todo` of task 040 in `src/config/config.go` is removed.
- `enabled: false` is the behavior before this task: no count, no demotion (rollback).
- Metrics (conventions § 12): `qabz_circuit_breaker_state` and `qabz_circuit_breaker_transitions_total`, with fixed labels only. One log line for each open and each close (conventions § 10).
- `system.html` (Provider Switcher, Hot Reload table, Metrics), conventions § 13, `docs/this_vs_bill.md`, the app README, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Unit and live tests (conventions § 14), also with `-race`: below `min_calls` nothing opens; 29 % does not open and 30 % opens; old calls leave the window; a business answer is a good call; the override of one provider; a hot reload of each key; `enabled: false`. Each changed package is at 70.0 % or more.
- Feature ON (Rule 15): on the dev database with the mocks and `enabled: true`, the first provider of a group with two enabled rows returns errors: the log and the metrics show the open, the switch to the next provider, the trial inquiry after `open_time` and the return. The same test on a group with one row shows no demotion.

## Open points for the design (the operator decides before the implementation)

1. **The calls of the status workers.** A status check is a call to the provider of an `order` row. Decide whether it counts for that row or only payments count.
2. **A payment timeout.** It stays `pending` (the provider can have paid) and is a provider-down result. Confirm that it counts as a failed call for the `order` row.
3. **The library.** `github.com/sony/gobreaker` (what the `tavanir` service uses) counts in fixed intervals and blocks calls, which this design does not want. Recommendation: a small own sliding-window counter, no new dependency.
4. **The label of the breaker metrics.** The count is for each row, so the labels are `provider`, `bill_type` and `action` (values of the `providers` table, a bounded set).

## Design (implementer, 2026-11-17; approved by the operator: "OK go on")

Read on 2026-11-17 from the Qabz source at `35c6543`.

- **D1. The state.** `src/provider/breaker.go` (new): one counter for each provider row id, in memory. The counter is a ring of 20 time buckets, each `window/20` wide (a bucketed sliding window: old calls leave in steps of `window/20`, 6 s for `2m`; the memory is fixed at any traffic). A change of the `window` of a row starts its count new. The id is the key, so a load keeps the count of a row that stays.
- **D2. The rule to open.** `total >= min_calls` and `failed*100 >= failure_ratio*total` (whole numbers: 29 % does not open, 30 % opens).
- **D3. The reports.** A report takes a short lock in memory: no database call, no wait. The `Demote` at the threshold runs in its own goroutine, one at a time for each row. Inquiry: `provider_ok` and `provider_refused` are good, `provider_down` is failed; a saved answer, `canceled` and `error` are not counted; a trial is not counted (the switcher gets its result). Payment: each `Pay` call reports for its row (`o.ProviderID`, then the next row after a task 058 fallback); an error that wraps `provider.ErrProviderDown` (also `ErrNotSent` and a timeout) is failed, any answer (also `failed`) is good; a canceled call is not counted.
- **D4. Open and close.** The `OnDemoted` callback of `ProvidersService` drives the state: `true` opens the row (its results are ignored while it is open); `false` (a good trial, a return with no trial, `cleared`) closes it and the count starts new. A failed trial demotes the row again in the switcher: the breaker stays open.
- **D5. The config.** `providers.general.circuit_breaker.{enabled,failure_ratio,window,min_calls,open_time}` (defaults `false`, 30, `2m`, 20, `1m`), and for each of the 6 providers with a config block (`internal`: `irancell`, `jiring_switch`, `parsian`; `external`: `karashop`, `qabzino`, `tavanir`) `circuit_breaker.{failure_ratio,window,min_calls,open_time}`: 0 (the default) takes the general value. All keys are hot-reloadable. A row whose provider has no config block takes the general values. The `@todo` of task 040 is removed.
- **D6. Metrics and log.** `qabz_circuit_breaker_state{provider,bill_type,action}` (0 closed, 1 open) and `qabz_circuit_breaker_transitions_total{provider,bill_type,action,to}` (`to`: `open`, `closed`). One info line for each open (with the counts) and each close.
- **D7. `enabled: false`:** no count, no demotion (the behavior before this task). The key is read at each report, so a hot change applies at once; a row that is open when the key goes to `false` returns by its trial as before.
- **D8. Tests** with a test clock for the window and the ratio; live tests on the run database, also with `-race`.
- **D9. Feature ON** as task 059: a fake jiring-switch service on the dev database; also one group with one row.

**Decisions (operator, 2026-11-17):**
1. Status workers: only payments count (a status call is another API, in batches).
2. A payment timeout counts as a failed call of the `order` row.
3. Own sliding-window counter, no new dependency.
4. The metric labels are `provider`, `bill_type`, `action`.
5. No switch for each provider for the time being (operator, after the first choice `disabled: true` broke conventions § 8): only the four number keys have an override.
6. Hot values are not checked by the reloader: a value out of range is ignored (an override falls back to the general value, a general value to its default), with one warn line at the change. At start `Validate` refuses it.
7. After `no_next` the row does not count for `open_time`, then starts new (at most one `demote skipped` line for each `open_time`).
8. `debug.provider_switcher.demote_on_down` and its hook are removed (an old `config.yml` with the key stays valid: the decoder reads only tagged paths).

## Implementation notes (2026-11-17)

- `src/provider/breaker.go` (new): `Breaker`, `BreakerSettings`, `BreakerOptions`, `Report`, `Demoted`; the row state in a map with a lock (no `sync.Map`, no type assertion); a nil or zero `Breaker` does nothing.
- `src/core/inquiry/inquiry.go`: `Options.OnProviderDown` is replaced by `Options.OnProviderResult(p, down)` (ok and refused: false; provider down: true; a trial, a cancel, an error and a saved answer: no call).
- `src/core/order/complete.go`: `PaymentOptions.OnPayResult(p, down)` after each `Pay` call (the row `o.ProviderID`, also after a task 058 move); a canceled call and an error that does not wrap `ErrProviderDown` are not reported.
- `src/config/config.go`: 5 general keys and 4 override keys for each of the 6 providers (29 hot fields); `CircuitBreaker`, `GeneralValid`, `OverrideValid`, `CircuitBreakerGeneral`, `CircuitBreakerOverrides`, the defaults constants and the start checks; the `@todo` of task 040 and `debug.provider_switcher.demote_on_down` removed.
- `src/app/breaker.go` (new): `newBreaker` (the two metrics), `breakerSettings` (the table is built again only after `config.Holder.Changed` is closed; decision 6 with one warn line for each changed bad value), `breakerProviders` (config block → provider name). `src/app/defaults.go`: `newProvidersAndBreaker` (the builder and the live tests use it), `newProviders`, `newInquiryFlow` and `newPayments` take the breaker; `demoteOnDown` removed.
- `config.yml.example` and the local `config.yml`: the new keys; the `debug.provider_switcher` block removed.
- Two test defects found and fixed during the run (not code defects): a live test read a log line before the breaker goroutine wrote it; a hot-reload test had its cases in a map (random order).

## Verification record (2026-11-17)

- **Go tests:** `make test GO_TEST_ARGS="-race -count=1 -cover -v"` with the run database: see the QA scenario in `OPEN_TASKS.md` for the result and the coverage of each changed package.
- **Feature ON (Rule 15):** the dev database as it is ((`mobile`, `inquiry`): Jiring Switch row 28 priority 1, Parsian row 39 priority 2; (`irancell`, `inquiry`): row 38 only; no row changed; `debug.cache_inquiries: false`). An App copy (ports 18080 / 18082 / 19090) with Jiring Switch on the scratch fake gRPC service of task 059 (`127.0.0.1:18484`, down while a flag file exists), the Parsian mock `success`, the Irancell mock `error`, `providers.general.circuit_breaker: {enabled: true, failure_ratio: 50, window: 1m, min_calls: 3, open_time: 15s}`. `POST /v1/mobile/inquiry`:
  1. Jiring up → 200 (fake: `ok`).
  2. Flag on, three requests → 503, 503, 200. At the third call of the row (1 ok, 2 down: 66 % of 3) the breaker opened: info `circuit breaker: open, row demoted` (`id` 28, `calls` 3, `failed` 2), `providers: row demoted` (`id` 28); `qabz_circuit_breaker_state{jiring-switch,mobile,inquiry}` 1, `qabz_circuit_breaker_transitions_total{...,to="open"}` 1, `qabz_provider_switches_total{mobile,inquiry,demoted}` 1, `qabz_provider_demoted` 1. The third request went to Parsian.
  3. Two requests → 200, 200 by Parsian (`qabz_provider_calls_total{parsian,inquiry,ok}` 3); the fake got no call (2 down, 1 ok).
  4. Flag off, 16 s later, two requests → 200, 200 by Jiring Switch (fake: 3 ok in all): the trial was good (`providers: trial ok, row back in its place`, `qabz_provider_trial_total{mobile,inquiry,ok}` 1, `restored` 1), the breaker closed (`circuit breaker: closed, row back`, the state 0, `to="closed"` 1), and the next request had the original order.
  5. The group with one row (`irancell`), four requests with the Irancell mock `error` → 503 × 4; one `no_next` (`providers: demote skipped, no next available provider`, `id` 38) at the third call, and no new try at the fourth (decision 7: no count for `open_time`); no breaker open line and no breaker metric for Irancell.
  - Six error lines: the 2 Jiring and 4 Irancell 503 answers (conventions § 10). Each request called one provider.
  - **Verdict:** the output matches the design (D1 to D4, D6, D7; decisions 1 to 7): the row opens only at the threshold, the switch goes to the next row, the trial after `open_time` closes the breaker and returns the row, and a group with one row is never demoted.
- **Not done:** the payment path and the hot reload were verified by the unit and live tests, not on the dev App; the stage QA is an operator step (`OPEN_TASKS.md`).
