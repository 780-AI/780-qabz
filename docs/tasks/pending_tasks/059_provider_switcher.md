<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-17 — Created (operator decisions of 2026-11-17): the Provider Switcher, the demotion of a provider row and its return by a trial inquiry.
-->
# 059_provider_switcher — Provider Switcher: demote a provider row, trial inquiry, return to the original order

Status: pending

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
