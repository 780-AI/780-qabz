<!-- VERSION: 0.1 -->
<!--
CHANGE LOG
v0.1 — 2026-11-16 — Created (operator answers to task 054 Q1 to Q4): the MCI payment fallback from Parsian to Jiring Switch.
-->
# 058_mci_jiring_fallback — MCI payment fallback to the next provider (Jiring Switch)

Status: pending

## Description

Task 054 pays the MCI bill types `mobile` and `mobilemidterm` through Parsian only. This task adds the fallback: when Parsian is out of service for sure, `OrderComplete` pays through the next enabled provider row of (bill type, `order`). Today that is Jiring Switch (seed rows 30 and 29, priority 10; Parsian has 9). Read task 054 (the section "D2 revised", D2a to D2f, and the operator answers) first. `bill` has no payment fallback.

**Operator rules (2026-11-16):**
- **No fallback after a timeout.** A timeout (or any call whose status is not known) stays `pending`, as today; the Parsian status worker finalizes it. Only a call that did not reach Parsian for sure (the switch off, no address, a refused connection, gRPC `Unavailable` before the call) goes to the next provider.
- **No hard-coded provider order.** The order of the providers comes only from (1) the operator, who changes the `providers` rows in the database, and (2) the provider selector and the switcher of the future circuit breaker. The code does not name Jiring Switch as the fallback; it takes the next enabled row with a payment route.
- The dev seed rows 29 and 30 (`jiring-switch`, `order`) change to `enable` in this task (task 054 enables row 48).

## Definition of Done

- A sure Parsian "out of service" on `OrderComplete` of `mobile` or `mobilemidterm` saves a new `in-progress` row with the next provider (the same RRN) and pays through it; a timeout stays `pending` with no second call. With no next enabled provider, the answer is that of today.
- The Jiring Switch adapter has `Pay` (`InternalInquiryService/PayBill`, bill's `JiringSwitchMCIMobile` fields) and `Status` (`GetStatus`), with their mocks (conventions § 15).
- A Jiring Switch status worker (`providers.internal.jiring_switch.get_status.*`) on the shared status worker of task 050; the pending-order query takes the newest row of each order over all providers before it filters by the provider, so no worker writes over the row of another provider.
- The "already paid" windows have `jiring-switch` for `mobile` and `mobilemidterm` (bill: 0).
- `docs/this_vs_bill.md` has an item for the fallback and for the query change, with a `@todo` at the code; `system.html`, the config examples, the CHANGELOG and the QA scenario in `OPEN_TASKS.md` are updated in the same commit.
- Unit and live tests (conventions § 14), the metrics of conventions § 12 and the spans of conventions § 17 show both providers. Build, vet and tests (with `-race`) pass; each changed package is at 70.0 % or more.
- Feature ON (Rule 15): the Parsian switch off and the Jiring Switch mock on: an order paid by the fallback; a Parsian timeout that stays `pending`; the Jiring Switch worker finalizes a pending Jiring order.
