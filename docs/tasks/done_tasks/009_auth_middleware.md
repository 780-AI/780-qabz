<!-- VERSION: 1.0 -->
# 009_auth_middleware — JWT authentication for every endpoint

Status: done

## Description

Add an auth middleware. It intercepts every request that needs authentication. If the JWT is not correct (empty, expired, bad signature, malformed, and so on), it rejects the request with HTTP 401 (gRPC `Unauthenticated`).

By default, every endpoint needs JWT authentication, with no exception.

Add a config section for development only:

```yaml
debug:
  auth:
    disable: true
```

When `debug.auth.disable` is on, the auth middleware does not check requests (for development). The production config does not have this section at all, because of the security risk.

Default (operator, 2026-11-07): in the code, the field is a plain `bool` with no default, so its zero value `false` applies: a missing section or key means auth is ON. The development config file (`config.yml`, git-ignored) sets `disable: true`, so local runs have auth off. The repo has no tracked example config today; the key is documented where the config keys are documented.

Help: `bill` (read-only reference, `/Users/mostafatalebi/projects/bill`) checks the JWT statelessly in `app/router/middleware/jwt.go` with `pkg/oauth/jwt.go`. Its unused `pkg/auth` (ES512 only, public keys from etcd) is the better model. Do not copy the weak points found in `bill`: the token chooses the algorithm (HMAC, RSA or ECDSA), `exp` is not required, `aud` is not compared, the library `dgrijalva/jwt-go` is deprecated.

## Points for the design step

- **Startup warning.** When auth is disabled, the App logs one warning at startup (allowed by the logging policy: bootstrapping).
- **Algorithm and key.** One fixed asymmetric algorithm (which one: ask the auth team; `bill`'s `pkg/auth` uses ES512) and public keys only. The key source: a config value, a file, or etcd with reload as in `bill`. Qabz never holds a key that can sign a user token.
- **Required claims.** `exp` required; `sub` (UUID), `mobile`, `aud` (compared with an expected value), and `non_otp`. Whether guest users (`non_otp == "guest"`) are refused, as in `bill`.
- **Scope of "every endpoint".** The public gRPC server and the HTTP gateway: yes, `Greeting` too. Open: the metrics endpoint (separate HTTP server, not an API), and a future internal server for the payment callback (`PaymentComplete/OrderComplete`), which in `bill` has no auth.
- **Where it runs.** A gRPC unary (and stream) interceptor, so the gateway and direct gRPC calls get the same check. The claims go into the request context for the handlers.
- **Other headers.** Whether `User-Agent` and `Platform` are required, as in `bill`.
- **Monitoring.** A Prometheus counter of rejected requests by reason; no log line per rejected request (logging policy).
- **Postman.** The collection gets a `{{token}}` variable and an `Authorization: Bearer {{token}}` header on every request (conventions § 7).

## Tests

Tests make it easy to check the middleware with different config entries and different JWT inputs. They are normal Go unit tests in a `_test.go` file next to the middleware source file, in the same package (no separate test folder). A table-driven test runs every combination of the two lists below; `go test` on that package runs it. Adding a new case is one new row.

Config entries:
- No `debug` section.
- `debug:` present, no `auth` key.
- `debug.auth.disable: false`.
- `debug.auth.disable: true`.

JWT inputs:
- No `Authorization` header; an empty header; `Bearer` with no token.
- A wrong scheme (for example `Basic ...`); a malformed token (not three parts, bad base64, bad JSON).
- Expired (`exp` in the past); no `exp`; not yet valid (`nbf` in the future).
- Signed with another key; the payload changed after signing.
- Another algorithm: `alg: none`, HS256 signed with the public key as the secret, an algorithm other than the fixed one.
- A missing or bad required claim (`sub` not a UUID, no `mobile`, wrong `aud`); a guest token, if guests are refused.
- A valid token.

Expected: with auth on (the first three config entries), only the valid token reaches the handler, and every other input gets gRPC `Unauthenticated`; with `disable: true`, every input reaches the handler. The valid case also checks that the claims are in the handler's context.

Rules for the tests:
- The test makes its own key pair and signs its own tokens. No real key and no real token in the repo.
- The same table runs against the gRPC interceptor and once through the HTTP gateway, which must return HTTP 401 (not 500) for a rejected request.
- The test controls the clock, so the `exp` and `nbf` cases do not depend on the time of the run.
- Assertions use `testify/assert` (conventions § 3) and follow the nil-guard rule (conventions § 4).

## Definition of Done

- Every endpoint rejects a request without a valid JWT with HTTP 401 / gRPC `Unauthenticated`: empty, malformed, expired, wrong signature, wrong algorithm, missing required claim.
- A request with a valid JWT reaches the handler, with the claims in the context.
- `debug.auth.disable: true` turns the check off. With the key or the `debug` section missing, auth is on (the `bool` zero value); a test shows both cases.
- The middleware's `_test.go` file (next to its source file) covers every config entry and every JWT input listed in "Tests"; build, vet and tests pass.
- Config docs, the Postman collection and the CHANGELOG are updated in the same commit.

## Verification

Run the App with auth on (remove `debug.auth.disable` from `config.yml`, or set it to `false`) and call `GET /v1/greeting` with no token. Pass: HTTP 401. Then call it with a valid test token. Pass: HTTP 200 and `{"message":"Hello"}`. Then set `debug.auth.disable: true`, call it with no token. Pass: HTTP 200.

## Decisions (operator, 2026-11-07)

- Qabz only verifies tokens; it never signs one.
- Required claims: `exp`, `sub` (a UUID) and `mobile`, `aud` compared with `auth.jwt.audience`; guests (`non_otp == "guest"`) are refused.
- The metrics endpoint stays outside the auth check.
- Algorithm and key source: the operator skipped the question. Claude chose ES512 (the algorithm of `bill`'s `pkg/auth`) and a PEM file path (`auth.jwt.public_key_path`). **Confirm both with the auth team.**

## Result (2026-11-07)

- `common/auth/jwt.go`: `Verifier` (ES512 only, P-521 public key, standard `crypto` plus `fastjson` for the hot-path JSON, no JWT library). The signature is checked before the claims are read. `exp` required; `nbf` checked when present; `crit` refused. Rejection reasons are metric labels only.
- `common/auth/context.go`: `WithClaims` / `ClaimsFrom` for the handlers.
- `entry/grpc/auth.go`: `Auth` with unary and stream interceptors; one answer for every rejection (`Unauthenticated`, "unauthenticated"). `NewWithOptions` installs it. Without a verifier it refuses to build unless disabled.
- `config`: `auth.jwt.public_key_path`, `auth.jwt.audience`, `debug.auth.disable` (a plain `bool`: missing means the check is on).
- `app/defaults.go`: builds the check first; with the check on, a missing or bad key file or audience stops the start; with `debug.auth.disable` one warning is logged. Counter `qabz_auth_rejected_total{reason}`.
- Tests next to the code: `common/auth/jwt_test.go` (33 JWT inputs), `entry/grpc/auth_test.go` (4 config entries × 33 inputs, claims in the context, two Authorization values, stream), `entry/http/auth_test.go` (33 inputs through the gateway: 401 / 200), `app/defaults_test.go` (start rules). The shared input table is in `common/auth/authtest` (test keys and tokens, tests only), with its own tests in `authtest_test.go`.
- Deliberate failures: accepting any `alg`, not requiring `exp`, allowing guests, ignoring `disable` (both ways) and skipping the `aud` check each made tests fail.
- Feature ON run: no token → HTTP 401; a scratch valid token → HTTP 200 `{"message":"Hello"}`; a bad token → 401; the metric showed `missing` 1 and `malformed` 1; no log line per rejection. With `debug.auth.disable: true` → 200 and the warning. Auth on without a key → exit 1 with a clear message.
- The local `config.yml` (git-ignored) got `debug.auth.disable: true`.

## Open items at close

- `docs/initial-discovery/system.html` (Critical File) plans the JWT public keys in etcd with live reload. This task reads a PEM file once at start. Moving to etcd with reload is a follow-up and needs the Critical File workflow.
- The algorithm (ES512) and the audience value are not confirmed by the auth team yet.
