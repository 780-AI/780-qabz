<!-- VERSION: 1.0 -->
# 010_jwt_hmac_secret — JWT check with an HMAC secret in place of the ES512 public key

Status: done

## Description

Task 009 checks the JWT with ES512 and a P-521 public key file (`auth.jwt.public_key_path`). The auth service signs its tokens with a shared secret, as in `bill` (`pkg/oauth/jwt.go`, `JWT.HMACSecret`). In `bill`'s config the `RSASecret` and `ECDSASecret` values are not PEM keys, so only the HMAC branch can verify a token there.

Change the check to HMAC only (operator, 2026-11-07):
- Replace `auth.jwt.public_key_path` with `auth.secrets.hmac`, a `config.Secret` (the raw secret string, used as bytes, as in `bill`). It never prints in a log.
- Accept `HS256`, `HS384` and `HS512`, all with the same secret (`bill` accepts every HMAC variant; the variant of the auth service is not known). Reject every other `alg`: `none`, `RS*`, `ES*`, `PS*`. No RSA or ECDSA key, so no algorithm confusion between symmetric and asymmetric keys.
- Keep everything else of task 009: `exp`, `nbf`, `aud`, `sub` (UUID), `mobile`, `non_otp` (no guests), `crit` refused, `debug.auth.disable`, the 401 answer, the metric, the start failure when the secret or the audience is missing.
- Compare the signature in constant time (`hmac.Equal`).
- The real secret goes only in the local `config.yml` (git-ignored). Tests use test secrets only.

This reverses the task 009 decision "Qabz never signs a token": with a shared secret, Qabz could sign one. Qabz still only verifies.

## Definition of Done

- A token signed with `auth.secrets.hmac` and `HS256`, `HS384` or `HS512` reaches the handler; a token with another secret, another algorithm or `alg: none` gets HTTP 401 / gRPC `Unauthenticated`.
- With the check on, the start fails when `auth.secrets.hmac` or `auth.jwt.audience` is missing; `auth.jwt.public_key_path` is gone.
- The shared JWT input table, the gRPC table, the gateway table and the start-rule tests use the secret; build, vet and tests pass.
- Config docs (`config.go`, `app/README.md`), `folder_structure.md` and the CHANGELOG are updated in the same commit.

## Verification

Put `auth.secrets.hmac` and `auth.jwt.audience` in `config.yml`, remove `debug.auth.disable` (or set it to `false`), run the App and call `GET /v1/greeting` with no token. Pass: HTTP 401. Then call it with a token from the auth service. Pass: HTTP 200 and `{"message":"Hello"}`.

## Decisions (operator, 2026-11-07)

- HMAC only, with one secret, in the style of `bill`'s `JWT.HMACSecret`. No `RSASecret` / `ECDSASecret` keys: with `bill`'s values they cannot verify a token, and mixing key kinds when the token chooses the algorithm is the weakness task 009 did not copy.
- The auth secrets sit in the main `config.yml` under `auth.secrets` (the HMAC secret is `auth.secrets.hmac`), not in a separate file. The pending version named the key `auth.jwt.hmac_secret`; the operator changed it before the push.

## Result (2026-11-07)

- `common/auth/jwt.go`: `NewVerifier(secret, audience, now)` keeps a copy of the secret. `checkHeader` maps `HS256` / `HS384` / `HS512` (exact names) to SHA-256 / SHA-384 / SHA-512; any other `alg` is `algorithm`. The signature is an HMAC of `header.payload`, compared with `hmac.Equal` before any claim is read. `LoadVerifier`, `ParsePublicKey` and the ECDSA code are gone. The claim checks are unchanged.
- `config`: `auth.secrets.hmac` (`config.Secret`) replaces `auth.jwt.public_key_path`. `app/defaults.go` builds the verifier from it; a missing secret or audience stops the start.
- `common/auth/authtest`: test secret, `Sign` (HMAC from the header alg, so a test can lie about the alg), `Verifier`, and the shared table: 41 inputs (6 valid: HS256, HS384, HS512, lower-case scheme, aud array, nbf in the past). New rejections: another secret, short and zero signature, HS512 header over an HS256 signature, no alg, lower-case `hs256`, `RS256`, `ES512`, `PS256`, `nbf` as a string.
- Tests: `go build`, `go vet`, `go test -race -count=1 ./...` pass; `common/auth` 94.2%, `authtest` 96.0%.
- Deliberate failures (overlays): skipping the HMAC compare fails 5 signature cases; accepting any alg fails 5 algorithm cases; both fail `common/auth`, `entry/grpc` and `entry/http`.
- Feature ON (App binary, compose Postgres and Redis, test secret): no token 401; HS256, HS384, HS512 tokens 200; another secret, `RS256` header, expired and guest 401; the metric counted `missing`, `signature`, `algorithm`, `expired`, `guest` once each; the secret was in no log; without the secret the start exits 1 with `auth: HMAC secret is required`.
- The local `config.yml` (git-ignored) is not changed: it has `debug.auth.disable: false` and no auth keys, so the App does not start until the operator adds `auth.secrets.hmac` and `auth.jwt.audience`.

## Open items

- The operator's secret has 20 bytes. RFC 7518 § 3.2 asks for at least the hash size (32 bytes for HS256). Qabz needs only a non-empty secret, to stay compatible. Ask the auth team for a longer secret.
