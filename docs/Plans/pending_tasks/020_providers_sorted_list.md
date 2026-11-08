<!-- VERSION: 0.1 -->
# 020_providers_sorted_list — ProvidersService: the sorted provider list and PickProvider

Status: pending

## Description

Add `ProvidersService`, the in-memory sorted list of the providers in the `providers` table of the `bill` database. The service loads the rows through a repository (raw query, no ORM, as in tasks 013 and 016), sorts them, groups them by bill type and action, and gives them to the rest of the code. It reloads the rows from the database every `providers.general.load_interval`.

**ProvidersService is the only way to read providers.** Every other part of the code (order, inquiry, fee, the provider adapters, later endpoints) gets a single provider, a list or any other provider data from this service, never from the repository or the table. The task records this rule as a new section of `docs/standards/PROJECT_SPECIFIC_CONVENTIONS.md`, so the `code-reviewer` agent checks it.

**The sorted order is the core of the task. It MUST be deterministic:**
- Each group is the providers of one (`bill_type`, `action`), for example (`gas`, `order`) or (`gas`, `inquiry`).
- In a group the order is `tmp_priority` ascending (the lower value first), then `id` ascending. No other input changes the order: not the row order from the database, not map order, not the load time.
- The same rows give the same order, every time. The order changes only when a row in the database changes (`priority`, `status`, a new row, a removed row) and the next load reads it.

**`tmp_priority`** is a runtime key. It is not a column. After each load it equals `priority`. It is the key that a later task (the error-rate observer, not in this task) lowers when a provider fails, so the stored `priority` never changes at runtime (`system.html`, Provider Switcher: "the original order itself, the `priority` of the `providers` table, is not changed"). In this task only `MoveUp` and `MoveDown` change it.

**The service interface:**
- `PickProvider(billType, action)` returns the first **enabled** provider of the group, and nothing else. It does not and must not follow any other logic: no round robin, no random, no weights, no error-rate check. For the same group it always returns the same provider until the list changes. A group with no enabled provider gives an error that callers can test (`errors.Is`). It runs for each order and inquiry request: it is a hot path (conventions, Definitions). It must not touch the database, must not block on a reload or a move, and should not allocate.
- The list: every provider of every group (enabled and disabled), in the sorted order, and the list of one group. The returned values are copies; a caller cannot change the service state through them.
- `MoveUp(id)` and `MoveDown(id)` move a provider one place up or down in its group, through `tmp_priority`. A move of the first provider up, or the last one down, changes nothing. An unknown id gives an error.
- `Toggle` (or `Enable(id)` / `Disable(id)`, design step) sets the runtime status. `PickProvider` skips a disabled provider.
- A change (a load, a move, a toggle) is atomic: a reader sees the whole list before the change or the whole list after it, never a mix.

**The providers list (`providersList`) MUST be concurrent safe.** Many goroutines use it at the same time: every request calls `PickProvider` and the list, while the reload routine loads, and a move or a toggle (now manual, later the error-rate observer) changes it. Every function of the service is safe to call from any number of goroutines at the same time, with no data race, no lost change, no deadlock and no panic. A caller needs no lock of its own. Readers do not wait for writers: a load, move or toggle never blocks `PickProvider`. Writers (load, move, toggle) run one at a time, so two changes at the same time cannot overwrite each other; a move that runs during a load applies to the old list or to the new one, never to a mix. The Go doc comment of the type and of each exported function states that it is safe for concurrent use.

**Reload (operator decision, 2026-11-08): the database always wins.** For now the database is the only source of truth. Each load replaces the whole list with the rows as they are in the database: `tmp_priority` = `priority`, the status as stored, new rows added, removed rows dropped. So a runtime `MoveUp` / `MoveDown` / toggle lasts only until the next load. A later task may change this rule. If nothing changed in the database, the order after the load is the same as before.

Config: `providers.general.load_interval` (`time.Duration`): the interval at which the providers are loaded from the database. Add it to `config.yml` and `config.yml.example` (conventions § 8).

**Not in this task:** an HTTP or gRPC endpoint for providers (operator, 2026-11-08: none exists and none is added); the error-rate observer that changes `tmp_priority`; the provider adapters (`ProviderInterface`) and the Provider Switcher.

Help:
- The table: `src/system/postgres/devdb/schema.sql` (`providers`: `id bigserial`, `name`, `subject`, `bill_type`, `action`, `type`, `priority smallint`, `timeout`, `status` default `'enable'`, `updated_at`, `use_nats`, `inquiry_step`, `provider_fee`) and `docs/initial-discovery/database.html`. The dev database has 45 seed rows (task 015, `devdb seed`); groups have 1 to 3 providers there, and most priorities are 10.
- `bill` behavior (`~/projects/bill`, `domain/repository/provider/provider_dto.go`, `GetProviderByBillTypeAndAction`): `WHERE bill_type = ? AND action = ? AND status = 'enable' LIMIT 1` with **no ORDER BY**, so bill never uses `priority` and its pick is any enabled row that Postgres returns first. bill caches the pick in Redis per (bill type, action) for `caching.provider`. Qabz picks by the sorted order and has no Redis cache: a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice).
- `system.html` plans the code in `provider/` (`selector.go`: "ProviderSelector: picks the provider for a bill type") and puts "Provider order, status and priority (the `providers` table)" in the hot-reloadable list. `src/provider/` exists with empty adapter folders.
- The hot path: read the current list through one atomic pointer (`sync/atomic`) to an immutable snapshot, and build a new snapshot on each change (copy on write). Writers (load, move, toggle) run one at a time.

Points for the design step:
- The package and names: `provider/` (as `system.html` plans) or `core/provider/`; `ProvidersService` and its Go interface; the provider type (which columns it carries).
- `MoveUp` / `MoveDown` when the neighbor has the same `tmp_priority` (common: most rows have 10). A swap of equal values changes nothing, so the design defines how the values change so that the provider really moves one place and the result stays a pure sort by (`tmp_priority`, `id`). Example: give the group's `tmp_priority` values new ranks, or let the move swap `id` order through a tiebreak field. State the rule and its proof in the tests.
- `Toggle` as one call that flips the status, or `Enable` / `Disable` (repeatable, safer for a background routine).
- The first load: if it fails, the start fails (conventions § 10: a failure that stops the start), or the App starts with an empty list. Later loads: a failed load keeps the last good list and writes one error log line (a failed background job).
- `providers.general.load_interval`: the default value, the lowest allowed value, and whether a change applies without restart (hot reload, task 003).
- Metrics (Implementation_Plan Step 5 / 6): for example the load result counter and the time of the last good load. Durations in milliseconds (conventions § 9).
- How the feature-ON run shows the order without an endpoint (see Verification): a debug-level log line of the order of each group after a load that changed it, or a development command.
- A test helper: `postgrestest` gets a `providers` table. Reuse the `devdb` schema (the task 015 follow-up in `OPEN_TASKS.md` asks for one definition of the bill tables) or copy it.

## Definition of Done

- `ProvidersService` loads, sorts, groups and reloads the providers as described above, and `PickProvider`, the list, `MoveUp`, `MoveDown` and the toggle work as described. The convention section "providers only through ProvidersService" is in `PROJECT_SPECIFIC_CONVENTIONS.md`.
- **Unit tests (no database), on every part of the code:**
  - Many fake providers: at least 200 providers in at least 10 groups, with equal priorities, gaps, negative and large values, disabled providers, a group with no enabled provider, an empty group, and the 45 seed rows. The same rows in many shuffled input orders give one identical sorted result, compared with an independent reference sort written in the test.
  - `PickProvider` for each group returns the first enabled provider, the same one on every call (thousands of calls), and the error for a group with no enabled provider or an unknown group.
  - `MoveUp` / `MoveDown`: each place in a group, the first and the last place, ties, an unknown id. Long random sequences (a fixed seed) of moves and toggles compared step by step with a simple reference model of the expected order.
  - Reload: the database always wins (runtime moves and toggles are gone after a load); a load with no change keeps the same order; a changed priority, a changed status, a new row and a removed row each change exactly the expected groups; a failed load keeps the last good list and logs one error line.
  - Copies: a caller that changes a returned list or provider does not change the service.
  - Concurrency (`go test -race`, also with `-count` > 1): many goroutines call `PickProvider` and the list while other goroutines load, move and toggle at the same time. Pass: no race report, no deadlock (the test has a timeout) and no panic; each reader sees a whole, sorted list; a reader is not blocked by a writer (a writer held in the middle of a change does not stop `PickProvider`); after all writers stop, the final list equals the result of the same changes applied one by one (no lost change).
  - Benchmarks of `PickProvider` (ns/op and allocs/op recorded in the task result).
  - The repository with a fake querier: the query, the scan of each column, NULL columns, and the error paths.
  - The config key: default, valid values, rejected values.
- **Integration tests (real Postgres, a throw-away schema with the `providers` table):** the repository reads the rows, compared with a direct SQL query; the running service with a short `load_interval` picks up, without restart, an `UPDATE` of `priority`, an `UPDATE` of `status`, an `INSERT` and a `DELETE`, and the list and `PickProvider` change to the expected result; a reload with no change keeps the order; the 45 seed rows (`devdb` seed SQL) load and sort as the reference sort says.
- Deliberate failures (positive controls), each caught by a test: sort by `priority` in place of `tmp_priority`; drop the `id` tiebreak; let `PickProvider` return a disabled provider; keep runtime changes after a load; a reader that is not atomic (the race test); two writers that are not serialized (the lost-change check).
- Build, vet and tests (with `-race`) pass. `config.yml.example`, the conventions doc, `docs/this_vs_bill.md`, the docs and the CHANGELOG are updated in the same commit. The Postman collection does not change (no endpoint).

## Verification

Feature ON (Rule 15), with the real dev database (45 seed rows, task 015). There is no endpoint, so the evidence is the running service.

1. Run the integration tests against the compose Postgres. Pass: all pass, including the reload cases.
2. Run the App against the dev database with a short `providers.general.load_interval` (for example `5s`) and the order output chosen in the design step. Record the order of a group with more than one provider (for example `gas`/`inquiry` or `telmidterm`/`order`) and its `PickProvider` result.
3. With SQL, change the `priority` of one provider of that group so it moves to the top, and set the `status` of the current first provider to `'disable'`. Wait one `load_interval`.
   Pass: the new order and the new pick show without restart, exactly as the reference sort says; the other groups do not change.
4. Wait for one more load with no database change. Pass: the order is the same.
5. Restore the rows to the seed values. Pass: after the next load the order is the first one again.

The order changes only when the database changes.
