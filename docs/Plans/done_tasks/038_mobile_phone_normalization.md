<!-- VERSION: 0.2 -->
# 038_mobile_phone_normalization — normalize the incoming mobile number to the 98 form

Status: done

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

## Design decisions (2026-11-14)

- **The function:** `helpers.NormalizeMobile(s) string` returns the 98 form, or `""` when the input is not a mobile number. `ValidMobile` of the result is the check. `NormalizePhone` is not changed.
- **The rule order:** (1) each Persian digit (U+06F0 to U+06F9) and Arabic digit (U+0660 to U+0669) becomes its ASCII digit; (2) separators are dropped: white space (`unicode.IsSpace`, so also a tab and a no-break space), `-`, `(`, `)`; (3) one `+` is valid, only before the first digit, and only with `98` after it; any other character makes the input invalid (a dot, a slash, a letter, a digit of another script, a byte that is not UTF-8); (4) the prefix by the number of digits: `+98` or `0098` and ten digits, `98` and ten digits, `0` and ten digits, ten digits; the ten digits must start with `9`.
- **Limits:** an input above 64 bytes is invalid before the loop; more than 14 digits stop the loop. One pass, one fixed buffer, no regular expression (the hot path).
- **My Bills (operator, 2026-11-14: "Yes, My Bills too"):** the mobile type of `POST /v1/mybill` uses `NormalizeMobile` first. When it gives `""`, bill's rule (`NormalizePhone`, then `ValidMobile`) still decides, so every input that bill accepts stays valid with the same saved value (bill accepts eleven bytes with any first byte, for example `19121234567`). The `tel` type is not changed.
- **A phone in another form with a valid `billId`:** the phone is the inquiry ID, as before.
- **Metrics and logs (conventions § 12, § 10):** no new metric and no new log line. The outcome of the request check is in the `code` label of `qabz_request_duration_ms`; an invalid phone is a client error (no log line).
- **No lasting rule** is added to the conventions doc.

## Implementation record (2026-11-14)

- `src/common/helpers/phone.go`: `NormalizeMobile` and `maxMobileInput`.
- `src/entry/grpc/phone.go`: `Phone.Inquiry` converts the phone first; `validPhoneInquiry(req, phone)` checks the converted phone; the inquiry ID is the converted phone. MCI and Rightel use the same handler.
- `src/core/savedbill/savedbill.go`: `inquiryID` uses `NormalizeMobile` for the mobile type, then bill's rule.
- Tests: `TestNormalizeMobile`, `TestNormalizeMobileKeepsValid` (helpers); `TestPhoneInquiryRequestCheck`, `TestValidPhoneInquiry` (handler); `TestPhoneInquiryRoutePhoneForms`, `TestPhoneInquiryRouteStatusCodes` (gateway, JSON bodies); `TestInsertValid`, `TestInsertInvalid` (My Bills); `TestLiveIrancellOnlyIrancell` (its "bad phone" is now a landline; the 0 form is valid).
- Docs: `docs/this_vs_bill.md` item 35, `system.html` v2.9, `src/app/README.md`, the Postman descriptions of Phone / Inquiry and MyBills / Insert, `OPEN_TASKS.md` (QA scenario; the follow-ups (e) of tasks 036 and 037 removed).

## Verification record (2026-11-14, implementer)

- Build, vet, `gofmt`: clean. Full `make test GO_TEST_ARGS="-race -count=1 -cover -v"`: 999 PASS, 0 SKIP, 2 FAIL in the first run: `TestLiveIrancellOnlyIrancell` (its invalid phone was the 0 form, which is valid now; the test was changed to a landline and `make test GO_TEST_PKGS=./app/` then passed) and the known `qabz/config` `TestLocalConfigInSync` (task 023 follow-up (c), not from this task).
- Coverage gate (conventions § 11, from the full run with the live tests): `common/helpers` 98.9%, `core/savedbill` 100.0%, `entry/grpc` 98.7%. PASS.
- Positive control: with `NormalizeMobile` replaced by "the 98 form only" (`go test -overlay`), `TestNormalizeMobile`, `TestPhoneInquiryRequestCheck`, `TestPhoneInquiryRoutePhoneForms`, `TestPhoneInquiryRouteStatusCodes` and `TestInsertValid` FAIL.
- Differential check (a temporary test, not kept): 3 000 000 random inputs; for each of the 1 214 124 inputs that the old My Bills mobile rule accepts, the new rule gives the same value (409 989 of them through bill's rule).
- Feature ON (Rule 15): an App copy on ports 18080 / 18082 / 19090 (the operator debug App on 8080 stayed up), a config copy in the scratchpad: dev mode, `debug.auth.disable: true`, `providers.irancell.enabled: true` with an empty `grpc_address`, `debug.mocks.irancell.enable: true`, `caching.irancell: "1h"`, every other provider `enabled: false`. No call to a real provider. The phone was `989350000381` (not the operator's number of the task text).
  1. `{"phone":"09350000381","operator":"irancell"}`: HTTP 200, `midTerm.price` 150000, `fullTerm.price` 450000; one request row (`inquiry_id` `989350000381`, `success`); two `bill_inquiries` rows (`midterm`, `final_term`, provider 38, `inquiry_id` `989350000381`); the log lines `provider mock: inquiry answered with no call to the provider` and `inquiry: provider call`. Pass.
  2. `+98 935 000 0381`, `9350000381` and `۰۹۳۵۰۰۰۰۳۸۱`: HTTP 200 three times with the same answer; still one request row; three copies of each inquiry (`is_cached`); `qabz_inquiries_total{bill_type="irancell",provider="irancell",result="db_hit"} 3`, `result="provider_ok"} 1`, `qabz_provider_calls_total{provider="irancell",result="ok"} 1`; no new log line. Pass.
  3. `02112345678` and `0935000038`: HTTP 400 `{"code":3,"message":"invalid parameter"}`; no row; no log line; `qabz_request_duration_ms_count{code="InvalidArgument",path="/haf80.api.bill.Phone/Inquiry"} 2`. Pass.
  4. My Bills: `{"type":"mobile","phone":"+98 935 000 0382","operator":"irancell"}` and then `"phone":"(0935) 000-0382"`: the same `id` twice; one `bills` row with `data` `989350000382`; `qabz_mybill_insert_total` `created` 1, `existing` 1; `{"type":"mobile","phone":"+982112345678"}` and `{"type":"tel","phone":"+982112345678"}`: HTTP 400. Pass. Seen and not changed: the `phone` field of the answer is the phone as sent.
- After the run: the App copy stopped (`app stopping`, 0 error lines); the test rows deleted (1 `bills` row, 8 `bill_inquiries` rows, 1 request row); the tables have their counts of before the run (13 and 22); providers row 38 is `enable`.
- Not run: a real call of a provider (operator only); the MCI and Rightel operators in the App (they use the same handler line; the unit and live tests of both pass).
