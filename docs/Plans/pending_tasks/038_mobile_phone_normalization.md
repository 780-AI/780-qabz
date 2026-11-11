<!-- VERSION: 0.1 -->
# 038_mobile_phone_normalization — normalize the incoming mobile number to the 98 form

Status: pending

## Description

A client can send a mobile number in many forms. Today `POST /v1/mobile/inquiry` (task 035) accepts only the 98 form (`989xxxxxxxxx`, the rule of `bill`), so `09373969041` gets HTTP 400 `invalid parameter` (operator report, 2026-11-11). This task normalizes every incoming mobile number to the 98 form **before** the request check. The phone is then valid in any common form, and the cache key, the request rows and the provider call all use one form.

**The target form:** `98` + `9` + nine digits (12 digits), for example `989373969041`. It is the form that `helpers.ValidMobile` checks and that the providers get today.

**Forms to accept (operator):**

| Input | Normalized |
|---|---|
| `989373969041` (98 form) | unchanged |
| `09373969041` (leading `0`) | `989373969041` |
| `9373969041` (no `0`, no `98`) | `989373969041` |
| `+989373969041` | `989373969041` |
| `00989373969041` (international `00`) | `989373969041` |
| `+98 937 396 9041`, `0937-396-9041`, `(0937) 396 9041` (spaces, dashes, brackets) | `989373969041` |
| `۰۹۳۷۳۹۶۹۰۴۱` (Persian digits), `٠٩٣٧٣٩٦٩٠٤١` (Arabic digits) | `989373969041` |

Any input that does not become a valid 98-form mobile number after the normalization is invalid, as today (HTTP 400, no log line, no provider call). Examples: a landline (`02112345678`), a number of the wrong length, letters.

**Where it applies:**
- `Phone.Inquiry` (`src/entry/grpc/phone.go`, the shared part of tasks 035 to 037): the phone is normalized before `validPhoneInquiry`, and the normalized phone is the inquiry ID. So MCI and Rightel (tasks 036, 037) get it with no more work.
- The design step decides whether the other places that read a mobile number use it too (`core/savedbill` My Bills insert, which uses `helpers.NormalizePhone` today; the tel inquiry of task 025 is a landline and is NOT in this task).

**Not changed:** `helpers.NormalizePhone` (the copy of `bill`'s `NormalizePhoneNumber`, used by My Bills and later by the landline inquiry) keeps its behavior. The new rule is a new function for mobile numbers (for example `helpers.NormalizeMobile`), so a landline is never changed by the mobile rule.

Help:
- `src/common/helpers/phone.go` (`NormalizePhone`, `ValidMobile`) and its tests.
- `src/entry/grpc/phone.go` (`validPhoneInquiry`, `Phone.Inquiry`), `src/core/inquiry/inquiry.go` (`billTypes["irancell"]`: the inquiry ID rule `helpers.ValidMobile`).
- `bill`: `pkg/helper/phoneNumberNormalizer.go` (`NormalizePhoneNumber`), `controller/phone/inquiry.go` (no normalization in the mobile inquiry), `controller/phone/newOrder.go:158` (the mobile order normalizes).
- Each difference from `bill` that a client or the database can see is a numbered item in `docs/this_vs_bill.md` with a `@todo` at the code (task 013 practice): `bill` rejects every form but the 98 form in the mobile inquiry.

Points for the design step:
- The exact rule order (strip separators, convert Persian and Arabic digits, then the prefix cases), and which separators are accepted. Use the standard library where it has the function (conventions § 5, for example `unicode.IsDigit`, `strings.Map`).
- Whether My Bills (`POST /v1/mybill`) uses the new rule for a mobile `phone` (a behavior change of a done endpoint; ask the operator).
- A request with a valid phone in another form and a valid `billId`: the phone is the inquiry ID, as today.
- No metric is needed (the request check outcome is in the `code` label of `qabz_request_duration_ms`); state it in the change (conventions § 12).

## Definition of Done

- Every form in the table above gives the 98 form; an input that cannot become a valid mobile number is invalid (HTTP 400, no log line, no provider call).
- `POST /v1/mobile/inquiry` accepts the phone in every form of the table; the inquiry ID, the request row, the saved inquiries and the provider call use the 98 form, so two forms of one number are one cache key.
- `helpers.NormalizePhone` and every caller of it behave as before (their tests pass with no change of an expected value).
- Unit tests: a table test of the new function (every form of the table, the invalid cases, empty input, very long input); handler tests of `Phone.Inquiry` with the forms; a gateway test with a JSON body in the 0 form.
- Build, vet and tests (with `-race`) pass; the coverage gate of the changed packages (conventions § 11). `this_vs_bill.md`, the Postman description of Phone / Inquiry, the app README, the CHANGELOG and (if a lasting rule is added) the conventions doc are updated in the same commit.

## Verification

Feature ON (Rule 15), in dev mode against the compose dev database, with `providers.irancell.enabled: true`, `debug.mocks.irancell.enable: true` and `caching.irancell: "1h"`.

1. `POST /v1/mobile/inquiry` `{"phone":"09373969041","operator":"irancell"}`. Pass: HTTP 200 with the mock answer; the `bill_inquiry_requests` row and the `bill_inquiries` rows have `inquiry_id` `989373969041`.
2. The same number as `+98 937 396 9041`, then as `9373969041`. Pass: HTTP 200 from the database (`qabz_inquiries_total{bill_type="irancell",result="db_hit"}` 2), no new request row.
3. `{"phone":"02112345678","operator":"irancell"}` and `{"phone":"0937396904","operator":"irancell"}`. Pass: HTTP 400 `invalid parameter`, no row, no log line.
4. Delete the test rows when done.
