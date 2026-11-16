<!-- VERSION: 1.0 -->
# 012_auth_status_403 — a rejected auth check gives HTTP 403

Status: done

## Description

A call that fails the JWT check gets gRPC `Unauthenticated`, which the gateway maps to HTTP 401 (task 009). Change it to HTTP 403 (operator, 2026-11-07).

- The interceptor (`src/entry/grpc/auth.go`) returns gRPC `PermissionDenied` in place of `Unauthenticated`. grpc-gateway maps `PermissionDenied` to HTTP 403, so the gateway needs no custom error handler, and direct gRPC clients get the matching code.
- The answer stays the same for every rejection, so the reason is still not sent to the client. The message text stays `unauthenticated`; only the code changes.
- The metric, the check itself and `debug.auth.disable` do not change.

Note: RFC 9110 § 15.5.2 uses 401 for missing or bad credentials and § 15.5.4 uses 403 for a known client without access. The operator chose 403 for every rejection.

## Definition of Done

- Every rejected call gets gRPC `PermissionDenied` and HTTP 403 (not 401, not 500); a valid call still gets 200.
- The gRPC and gateway auth tests check the new codes; build, vet and tests pass.
- The Postman collection (conventions § 7), `app/README.md` and the CHANGELOG say 403, in the same commit.

## Verification

Run the App with the check on and call `GET /v1/greeting` with no token. Pass: HTTP 403 `{"code":7,"message":"unauthenticated",...}`. With a valid token: HTTP 200.

## Result (2026-11-07)

- `entry/grpc/auth.go`: `errRejected` (was `errUnauthenticated`) is `status.Error(codes.PermissionDenied, "unauthenticated")`; every rejection path returns it.
- Tests: the gRPC auth tests expect `PermissionDenied`; the gateway table expects HTTP 403 for every rejected input. `go build`, `go vet`, `go test -race -count=1 ./...` pass.
- Deliberate failure (an overlay with `codes.Unauthenticated`): `entry/grpc` and `entry/http` fail.
- Feature ON (App binary, test secret): no token, a bad token and a token with another secret gave HTTP 403 `{"code":7,"message":"unauthenticated","details":[]}`; a valid token gave 200; the metric counted `missing`, `malformed`, `signature`.
- Postman collection: the Greeting description says 403 (conventions § 7).
