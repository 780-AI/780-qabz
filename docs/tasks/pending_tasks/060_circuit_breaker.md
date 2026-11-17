<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-17 — Created (operator decisions of 2026-11-17): the circuit breaker of each provider row, its config keys and the start of a demotion.
-->
# 060_circuit_breaker — circuit breaker: the error rate of each provider row starts a demotion

Status: pending

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
