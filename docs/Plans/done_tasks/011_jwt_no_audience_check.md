<!-- VERSION: 1.0 -->
# 011_jwt_no_audience_check — accept any aud string, as bill does

Status: done

## Description

Task 009 compares the token's `aud` with `auth.jwt.audience`. The real `aud` value of the auth service tokens is not known. `bill` (`pkg/oauth/jwt.go`) does not compare `aud`: it only needs `aud` to be a string, and puts it in the request context.

Do the same (operator, 2026-11-07):
- Remove the config key `auth.jwt.audience` (code, `config.yml.example`, `config.yml`). The auth config is only `auth.secrets.hmac`.
- `aud` is required and must be a JSON string, as in `bill`. Any value is accepted. A missing `aud`, or an `aud` that is not a string (an array, a number), is rejected with the reason `audience`.
- The `aud` value goes into the claims in the context (`Claims.Audience`), as `bill` puts it in its context.
- The other checks stay.

Risk (accepted by the operator): any valid token of the auth service works on Qabz, also a token that was issued for another service.

## Definition of Done

- A valid token with any `aud` string reaches the handler, with the `aud` in the claims; a token without `aud` or with a non-string `aud` gets HTTP 401 / gRPC `Unauthenticated`.
- `auth.jwt.audience` is gone from the code and both config files; with the check on, only `auth.secrets.hmac` is required.
- The shared JWT table, the start-rule tests and the config tests are updated; build, vet and tests pass.
- `app/README.md`, conventions § 8 (layout examples) and the CHANGELOG are updated in the same commit.

## Verification

Remove `auth.jwt.audience` from `config.yml`, keep `auth.secrets.hmac`, run the App with the check on and call `GET /v1/greeting` with a token from the auth service. Pass: HTTP 200 `{"message":"Hello"}`.

## Result (2026-11-07)

- `common/auth/jwt.go`: `NewVerifier(secret, now)` has no audience. `readClaims` needs `aud` to be a JSON string (any value, also empty) and copies it to the new `Claims.Audience`; a missing or non-string `aud` is `audience`. `hasAudience` is gone.
- `config`: `auth.jwt.audience` (`JWTAudience`) is removed. `app/defaults.go` needs only `auth.secrets.hmac` with the check on.
- `config.yml.example` and the local `config.yml` lost `auth.jwt.audience`; the sync tests (conventions § 8) pass.
- Tests: the shared table has 43 inputs (7 valid). New: any `aud`, empty `aud` (valid); `aud` array, `aud` number (rejected). `TestAudienceInClaims` checks the value in the claims. `go build`, `go vet`, `go test -race -count=1 ./...` pass; `common/auth` 95.5%.
- Deliberate failure (an overlay without the `aud` check): `no aud`, `aud array` and `aud is a number` fail.
- Feature ON (App binary, compose Postgres and Redis, test secret, config with only `auth.secrets.hmac`): `aud` "qabz", "bill" and "" gave 200; no `aud`, an array and a number gave 401; the metric showed `audience` 3; the secret was in no log.
