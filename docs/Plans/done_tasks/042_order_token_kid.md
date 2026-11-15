<!-- VERSION: 0.2 -->
<!--
CHANGE LOG
v0.2 — 2026-11-15 — Done: verification record added.
v0.1 — 2026-11-15 — Created; operator decision: a separate key for the token kid, local value "bill".
-->
# 042_order_token_kid — a config key for the `kid` of the order token

Status: done

## Description

The order token (task 039) has the header `kid` = `service.name` (`qabz`). The payment service finds the public key by `kid`. On 2026-11-14 an Invoice call with a Qabz token failed in the payment service with `invalid request` (`extractOrder()`). The likely cause: the payment service has no key for `qabz`. `bill` signs with `kid` = `bill`.

This task adds the key `providers.internal.payment_server.kid`:

- Empty (the default): the `kid` is `service.name`, as before.
- Not empty: the `kid` is this value. Logs, metrics, tracing and the `serviceId` of the token do not change.

Read at start (not hot-reloadable), as `private_key_path`. The local `config.yml` sets `kid: "bill"` (operator, 2026-11-15).

Risk (told to the operator): with `kid: bill` the payment service treats the token as a `bill` token. If it sends the order-complete callback by `kid` or by `serviceId`, that step can still fail. The lasting fix is a `qabz` key in the payment service; this key is a stopgap.

## Definition of Done

- `Config.PaymentServerKID` reads `providers.internal.payment_server.kid`; `Config.OrderTokenKID()` gives the key, or `service.name` when it is empty.
- The App signs the order token with `OrderTokenKID()`.
- Tests: the default, a set value, and the method.
- `config.yml.example`, `system.html`, `src/app/README.md`, `CHANGELOG.md` and the manifest name the key. QA scenario in `OPEN_TASKS.md`.
- Feature-ON run: with `kid: bill` the token header is `{"alg":"RS256","kid":"bill","typ":"JWT"}` and the signature passes the check with the public key.

## Verification (2026-11-15)

- `make test` with `-run 'OrderTokenKID|PaymentServerKeys|ExampleHasEveryKey|LocalConfigInSync|LiveIrancellOrder|DefaultBuilder'`: all pass; `TestLiveIrancellOrderTokenKID` and `TestOrderTokenKID` ran (verbose run). `TestExampleHasEveryKey` failed until `config.yml.example` had the key (positive control of that check).
- Feature ON: an App copy (ports 18080 / 18082 / 19090) with the local `config.yml` (`kid: "bill"`): `POST /v2/mobile/order` gave `price` 459000, `serviceId` 149; the token header is `{"alg":"RS256","kid":"bill","typ":"JWT"}`; `openssl dgst -sha256 -verify` with the public key of `paymentPrivateKey.pem`: `Verified OK`; no error line in the App log. The test `orders` row was deleted.
- Not done: the Invoice call to the payment service with this token (operator, QA scenario (2) in `OPEN_TASKS.md`).
