<!-- VERSION: 0.1 -->
# 010_jwt_hmac_secret — JWT check with an HMAC secret in place of the ES512 public key

Status: pending

## Description

Task 009 checks the JWT with ES512 and a P-521 public key file (`auth.jwt.public_key_path`). The auth service signs its tokens with a shared secret, as in `bill` (`pkg/oauth/jwt.go`, `JWT.HMACSecret`). In `bill`'s config the `RSASecret` and `ECDSASecret` values are not PEM keys, so only the HMAC branch can verify a token there.

Change the check to HMAC only (operator, 2026-11-07):
- Replace `auth.jwt.public_key_path` with `auth.jwt.hmac_secret`, a `config.Secret` (the raw secret string, used as bytes, as in `bill`). It never prints in a log.
- Accept `HS256`, `HS384` and `HS512`, all with the same secret (`bill` accepts every HMAC variant; the variant of the auth service is not known). Reject every other `alg`: `none`, `RS*`, `ES*`, `PS*`. No RSA or ECDSA key, so no algorithm confusion between symmetric and asymmetric keys.
- Keep everything else of task 009: `exp`, `nbf`, `aud`, `sub` (UUID), `mobile`, `non_otp` (no guests), `crit` refused, `debug.auth.disable`, the 401 answer, the metric, the start failure when the secret or the audience is missing.
- Compare the signature in constant time (`hmac.Equal`).
- The real secret goes only in the local `config.yml` (git-ignored). Tests use test secrets only.

This reverses the task 009 decision "Qabz never signs a token": with a shared secret, Qabz could sign one. Qabz still only verifies.

## Definition of Done

- A token signed with `auth.jwt.hmac_secret` and `HS256`, `HS384` or `HS512` reaches the handler; a token with another secret, another algorithm or `alg: none` gets HTTP 401 / gRPC `Unauthenticated`.
- With the check on, the start fails when `auth.jwt.hmac_secret` or `auth.jwt.audience` is missing; `auth.jwt.public_key_path` is gone.
- The shared JWT input table, the gRPC table, the gateway table and the start-rule tests use the secret; build, vet and tests pass.
- Config docs (`config.go`, `app/README.md`), `folder_structure.md` and the CHANGELOG are updated in the same commit.

## Verification

Put `auth.jwt.hmac_secret` and `auth.jwt.audience` in `config.yml`, remove `debug.auth.disable` (or set it to `false`), run the App and call `GET /v1/greeting` with no token. Pass: HTTP 401. Then call it with a token from the auth service. Pass: HTTP 200 and `{"message":"Hello"}`.

## Open items

- The operator's secret has 20 bytes. RFC 7518 § 3.2 asks for at least the hash size (32 bytes for HS256). Qabz needs only a non-empty secret, to stay compatible. Ask the auth team for a longer secret.
