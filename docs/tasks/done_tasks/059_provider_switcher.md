<!-- VERSION: 0.3 -->
<!--
CHANGE LOG
v0.3 — 2026-11-17 — Done: implemented (two sessions), all tests pass with the run database and -race, feature-ON run on the dev database passed; implementation notes and verification record added; moved to done_tasks.
v0.2 — 2026-11-17 — Started (operator request "Start task 59"): moved to in-progress_tasks; design D1 to D8 approved by the operator ("Approve design"); open points 1 (a), 2 (the flows keep PickProvider) and 3 (confirmed) decided; the feature-ON hook is a dev-only debug key.
v0.1 — 2026-11-17 — Created (operator decisions of 2026-11-17): the Provider Switcher, the demotion of a provider row and its return by a trial inquiry.
-->
# 059_provider_switcher — Provider Switcher: demote a provider row, trial inquiry, return to the original order

Status: done (2026-11-17)

## Description

`system.html` (Provider Switcher) names a gap: `bill` has no automatic provider switcher. This task builds the switcher. Task 060 (circuit breaker) builds the part that decides when to demote; until task 060 lands, nothing calls the switcher at runtime, so the behavior of the App does not change.

**Words.** A *provider row* is one row of the `providers` table: one (provider, bill type, action). A *group* is all rows of one (bill type, action). To *demote* a row is to move it to the end of the order of its group for a period. The stored `priority` never changes.

**Operator rules (2026-11-17):**
- **Demote only with a next available provider.** A row is demoted only when its group has a next available provider. With none, the switcher does nothing for that group and the requests keep going to the row.
- **No backup, no demotion, by nature.** A group with 1 provider is undemotable with no special code: the next provider is the provider itself, so a move to the end changes nothing. A group with 2 or more providers is demotable. The backup comes only from the `providers` rows: when an operator adds or enables a second row in a group (for example a second Irancell provider, after the first in the order), it becomes the backup at the next load, and the first provider becomes demotable, with no code change. No bill type is excluded by name.
- **"Only one provider" is data, not code.** That only one provider answers a bill type is the result of the `providers` rows (one enabled row in the group). The hard-coded `onlyProvider` rule of `core/inquiry` (Irancell, Rightel, landline; operator rules of 2026-11-11 and 2026-11-14) is removed: no `if` on a provider name or a bill type. The one exception in code stays: the task 058 payment fallback in the same request (`provider.ErrNotSent`, conventions § 13).
- **Available:** the row is `enable`, its provider has an adapter for the bill type and action, and the row is not demoted itself. When the promoted provider fails too, nothing more is done: the group waits until a trial succeeds.
- **The demotion state is in memory**, in each instance of the App. A restart clears it. No Redis, no database.
- **The trial is only an inquiry.** When the period ends, one real user inquiry goes to the demoted row. All other requests keep the current order. A good trial puts the row back in its original place; a failed trial demotes it again for a new period.
- **A demoted row of an `order` group** returns by an inquiry too (operator option (a)): the trial is one real user inquiry of the same bill type through the same provider. A payment is never a trial.
- **Generic.** No rule for a bill type, an operator or a provider name; the order is the priority of the rows (conventions § 13).

**Not in this task:** the error count and the thresholds, the config keys of the circuit breaker and the calls that start a demotion (task 060).

## Definition of Done

- `ProvidersService` (the only owner of the provider list, conventions § 13) has the demotion: a demote call with the row and the period, the "next available" rule, and the return to the original place. A demotion stays through the loads of the `providers` table for its period (today each load resets `TmpPriority`); a row that a load removes or disables loses its demotion.
- A group with one available row is never demoted; a second available row added in the database makes the first row demotable after the next load. 
- `onlyProvider` is removed from `core/inquiry` with its tests; the pick of `PickProvider` and the adapter check (`SupportsInquiry`) decide. Before the removal, the rows of the three groups (Irancell, Rightel, landline) in the dev seed, and the stage and prod rows (operator, a follow-up in `OPEN_TASKS.md`), are checked: only the wanted provider is `enable`. `docs/this_vs_bill.md` items 33, 36 and 37 and the three paragraphs of `system.html` are updated.
- The trial: after the period, exactly one inquiry request at a time gets the demoted row; its result (good or provider-down) restores the row or demotes it again. `PickProvider` stays a hot path with no database call and no wait for a writer.
- The request of one trial calls one provider (conventions § 13: one request, one provider). No flow calls a second provider.
- Metrics (conventions § 12; `system.html` Metrics): `qabz_provider_switches_total`, `qabz_provider_demoted`, `qabz_provider_trial_total`, with fixed labels only. One log line for each demotion, trial result and return (conventions § 10). Spans as conventions § 17.
- `system.html` (Provider Switcher: the new rules replace "while its breaker is open, the answer is provider-down"; the `ProviderSwitcher` interface as built), conventions § 13, `docs/this_vs_bill.md`, the app README, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Unit and live tests (conventions § 14), also with `-race` and concurrent requests: demote with and without a next available provider; both rows of a group failed; a load during a demotion; the trial goes to one request only; a good and a failed trial; an `order` row that returns by an inquiry trial. Each changed package is at 70.0 % or more.
- Feature ON (Rule 15): on the dev database with the mocks, a test hook or a debug call demotes a row; the log and the metrics show the switch, the trial and the return.

## Open points for the design (the operator decides before the implementation)

1. **An `order` row with no inquiry row.** When the provider of a demoted `order` row has no enabled inquiry row for the same bill type, no trial is possible. Options: the row returns with no trial when the period ends, or it stays demoted until the next load.
2. **The `Call` method of `system.html`.** The flows call `PickProvider` today. Recommendation: the flows stay as they are; the switcher changes the order inside `ProvidersService` and gets the trial result from the flow.
3. **The task 058 payment fallback** (`provider.ErrNotSent`) and a demoted row: the fallback reads `Group`, so it sees the demoted order. Confirm that this is wanted.

## Design (implementer, 2026-11-17; approved by the operator: "Approve design")

Read on 2026-11-17 from the Qabz source at `b96af6f`.

- **D1. The state.** The demotions live in `ProvidersService` (`src/provider/selector.go`): a map of the row id to the end time, the period and a trial flag, under the writer lock. Each snapshot gets the demoted ids. A demoted row sorts after every row that is not demoted; inside each part the order stays `TmpPriority`, then `ID`. `Priority` never changes. `Provider.Demoted` shows the state in `Group` and `List`. A load keeps the demotion of a row that stays enabled; a row that the load removes or disables loses it.
- **D2. `Demote(id, period)`.** It demotes only when the group has a next available row: enabled, not demoted, and `Options.Available(p)` true (the App: an inquiry row needs an adapter with `SupportsInquiry`; an order row needs an adapter and a payment route, as `nextProvider` of task 058). No next available row: nothing changes (`ResultUnchanged`, result `no_next`). A demoted row: nothing changes. A group with one row is never demoted, with no special code.
- **D3. The trial (inquiry only).** `PickInquiry(billType)` returns `(Provider, Trial, error)`. Its fast path reads the earliest end time of the snapshot: before it, the pick is the one of `PickProvider` (no database, no lock, no allocation). After it, one request claims the trial of a due row (a compare-and-swap of the trial flag) and gets that row. The flow calls `Trial.Done(result)` after the call: a good or a business answer puts the row back; a provider-down result demotes it again for the same period; any other result releases the claim. A trial reads no saved answer, so the provider is called. A claim older than one minute (a fixed constant) can be claimed again.
- **D4. A demoted `order` row.** Its trial is a `PickInquiry` of the same bill type through the available inquiry row of the same provider (not demoted itself); the result applies to the order row. A payment is never a trial. **Open point 1 (operator: (a)):** when the provider has no such inquiry row, the row returns with no trial when its period ends (a timer; a load checks it again).
- **D5. No `Call` method (open point 2).** The flows keep `PickProvider`; the inquiry flow uses `PickInquiry`. `Demote` and `PickInquiry` are in `ProviderSelector`. `system.html` shows the interface as built.
- **D6. `onlyProvider` removed** from `core/inquiry` with its tests. The dev seed has one enabled row in each of the three groups (Irancell row 38; Rightel row 34; landline row 124325, rows 40 and 52 disabled). The stage and prod rows are an operator follow-up in `OPEN_TASKS.md`. Behavior change: a second enabled row in these groups now answers (before: "no provider").
- **D7. Metrics, logs, spans.** `qabz_provider_switches_total{bill_type,action,result}` (`demoted`, `no_next`, `restored`, `redemoted`), `qabz_provider_demoted{provider,bill_type,action}` (gauge, 1 or 0), `qabz_provider_trial_total{bill_type,action,result}` (`ok`, `failed`, `released`). One info line for each demotion, trial result and return. The span `inquiry.Service.Inquire` gets `provider.trial` (true) on a trial.
- **D8. The feature-ON hook (operator: the dev-only debug key).** `debug.provider_switcher.demote_on_down` (a duration; `0`, the default, is off; removed outside dev mode with the debug section): in dev, each provider-down inquiry that is not a trial demotes its row for that period. Task 060 replaces it with the circuit breaker.
- **Open point 3 (operator: confirmed).** The task 058 payment fallback reads `Group`, so it sees the demoted order: it reaches a demoted row only after the other rows.

## Implementation notes (implementer, 2026-11-17)

- `src/provider/switcher.go` (new) holds the switcher; `selector.go` keeps the list, sorts a demoted row after the rows that are not demoted (then `TmpPriority`, `ID`) and gets `Options.Available`, `OnSwitch`, `OnDemoted`, `OnTrial`. `Demote` returns `provider.ErrNoNextProvider` (result `no_next`) with no next available row, `ErrBadPeriod` for a period of 0 or less, `ErrUnknownProvider` for an unknown id, and nil (no change) for a demoted or disabled row. A runtime `Disable` clears a demotion as a load does (result `cleared`, an extra fixed label value beyond D7).
- `src/app/defaults.go`: the provider list is built after the adapter registry, so `providerAvailable` (an inquiry row: `SupportsInquiry`; an order row: an adapter and a route of `payRoutes`, the routes of the task 058 fallback, now one function) can check the adapters. `demoteOnDown` is the D8 hook (`inquiry.Options.OnProviderDown`).
- `src/core/inquiry/inquiry.go`: `Selector` is `PickInquiry`; a trial skips the saved answers and maps its result (`ok`/`refused` → `TrialOK`, `down` → `TrialFailed`, any other → `TrialReleased`); the span gets `provider.trial`. `onlyProvider` and the three provider-name constants are removed.
- **`this_vs_bill.md` item numbers:** the DoD names items 33, 36 and 37; the items with the "only one provider" rule are 31 (Irancell), 33 (Rightel) and 36 (landline); item 37 is the Karashop adapter and has no such rule. Items 31, 32, 33 and 36 are updated and the switcher is the new item 51.

## Verification record (implementer, 2026-11-17)

- **Go tests:** `make test` (the run database; live tests ran): every package `ok` except the old flaky `TestInquiryOneCallFails` of `provider/karashop` (not changed by this task; it passed on 3 of 5 single runs, the known flake of the task 058 scenario). `go test -race -cover` of `provider` (95.1 %), `core/inquiry` (98.9 %), `app` (86.0 %; 88.2 % with the live tests), `config` (97.7 %), `core/order` (95.2 %), `entry/grpc` (98.8 %), `entry/http` (88.9 %): all `ok`. `TestLiveSwitcherInquiry`, `TestLiveSwitcherOrderTrial`, `TestLiveSwitcherOneRow` ran.
- **Feature ON (Rule 15):** the dev database as it is ((`mobile`, `inquiry`): Jiring Switch row 28 priority 1, Parsian row 39 priority 2; no row changed; `debug.cache_inquiries: false`, so no inquiry row was written). An App copy (ports 18080 / 18082 / 19090) with Jiring Switch on a scratch fake gRPC service (`127.0.0.1:18484`, down while a flag file exists), the Parsian mock `success`, `debug.provider_switcher.demote_on_down: 15s`. `POST /v1/mobile/inquiry` (`mci`):
  1. Start: one warn line `provider switcher: debug.provider_switcher.demote_on_down is on ...` (`period_ms` 15000).
  2. Jiring up → 200 (fake: `BillInquiry: ok`). Flag on → 503 (fake: `down`); info `providers: row demoted` (`id` 28, `next` 39, `next_provider` parsian); `qabz_provider_switches_total{mobile,inquiry,demoted}` 1; `qabz_provider_demoted{jiring-switch,mobile,inquiry}` 1.
  3. Two requests → 200, 200; `qabz_provider_calls_total{parsian,inquiry,ok}` 2; the fake got no call.
  4. Flag off, 16 s later: the trial → 200 (fake: `ok`); info `providers: trial ok, row back in its place`; `qabz_provider_trial_total{mobile,inquiry,ok}` 1, `switches_total{restored}` 1, the gauge 0. The next request → 200 by Jiring Switch (fake: `ok`): the original order.
  5. Flag on → 503 (demoted again, `switches_total{demoted}` 2); 16 s later the trial → 503 (fake: `down`); info `providers: trial failed, row demoted again`; `trial_total{failed}` 1, `switches_total{redemoted}` 1, the gauge 1. The next request → 200 by Parsian (`calls_total{parsian,ok}` 3).
  - Each request called one provider (the fake log and `qabz_provider_calls_total` add up: Jiring 3 ok + 3 down, Parsian 3 ok for 9 requests).
  - **Verdict:** the output matches the design (D1 to D3, D7, D8): a demotion only with a next available row, one trial request after the period, return on a good trial, a new demotion on a failed trial, the stored priority unchanged.
- **Not done:** the stage and prod provider rows of the three groups (operator follow-up in `OPEN_TASKS.md`); the order-row trial and the one-row group were verified by the live tests, not on the dev App.

