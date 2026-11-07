<!-- VERSION: 0.10 -->
# app

`app` is the bootstrap of the Qabz service. The type `App` owns every long-lived dependency: the logger, the Postgres database, the Redis cache, the metrics endpoint and the optional tracer. Entry points (for example `cmd/start-app`) build an `App` and run it. They hold no wiring of their own.

`App` depends only on small interfaces (`Logger`, `Database`, `Cache`, `Metrics`, `Tracer`), never on the concrete Postgres, Redis, zap or Prometheus types. Only `Config` is a concrete type. You make an `App` with `Builder`. `Build()` fails if the logger, database, cache or metrics is missing. `NewDefaultBuilder` is the one place that creates the real implementations from `Config`, which is read from a YAML file.

`Start` pings the database and the cache, prints their status in the log (host, port and database name only, never a password), and starts the metrics endpoint. If a step fails, it releases what it opened. `Stop` closes everything in reverse order and is safe to call twice. `Run` starts, waits for the context to end, then stops. Tests build an `App` with fakes and need no running server.

After the config reloader, `Start` starts the gRPC server (`entry/grpc`, address `server.grpc.addr`, default `:8082`) and then the HTTP gateway (`entry/http`, address `server.http.addr`, default `:8080`). The gateway serves the grpc-gateway routes and dials the local gRPC server, so each HTTP call runs through the real gRPC stack. `GET /v1/greeting` returns `{"message":"Hello"}` to ping the service.

`GET /v1/services` returns the service list (the bill types) from the `services` table of the `bill` database, in the same form as `bill`: the rows of the `Accept-Language` language (the raw header value; `fa-IR` when it is missing), by the `order` column, with `serviceId` from `service.id`. `NewDefaultBuilder` wires the handler (`entry/grpc`) to a `billtype.ServiceCatalog` (`core/billtype`) over `postgres.ServiceRepository`. The catalog keeps each language in memory for `config.CacheServicesTTL` (60s, a const) and never caches a failed read. A failed read gives gRPC `Internal` (HTTP 500) with no detail and writes one error log line with the cause (`services: list failed`); a call that the client canceled is not logged. Each call adds 1 to `qabz_services_cache_total{result="hit"|"miss"|"error"}`. One difference from `bill`: `inquiry` is sent as stored, not always `true` (`docs/this_vs_bill.md`, item 1).

Every gRPC call, and so every gateway route, goes through the JWT auth check (`entry/grpc/auth.go`, verifier in `common/auth`). A call without a valid `Authorization: Bearer <token>` gets gRPC `PermissionDenied` (HTTP 403) with the message "unauthenticated". The check accepts only HMAC tokens (`HS256`, `HS384`, `HS512`) signed with the secret that the auth service shares (`auth.secrets.hmac`, never printed) and needs `exp`, `sub` (a UUID), `mobile`, `non_otp` (not `guest`) and an `aud` string (any value: it is not compared, as in `bill`; it goes into the claims). Every other `alg` (`none`, RSA, ECDSA) is rejected. Qabz only verifies tokens. Each rejection adds 1 to `qabz_auth_rejected_total{reason}` and writes no log line. `debug.auth.disable: true` turns the check off for development and logs one warning at start; the production config has no `debug` section, and a missing key keeps the check on. With the check on, `NewDefaultBuilder` fails when the secret is missing. The metrics endpoint is outside the check. `Stop` stops the metrics endpoint, the gateway, then the gRPC server. A failed `Start` stops only the parts that started.

Before the metrics endpoint starts, `Start` runs the SQL migrations (`database.migration.on_start`, default true; many containers can start together, because the migrator takes a Postgres advisory lock) and starts the optional config reloader. A `Config` field with the tag `hot_reload:"true"` is read again from the table `app_hot_configs` every `hot_reload_interval`. `App.Config()` returns a copy of the current settings, so do not keep it.

```go
cfg, _ := config.Boot(os.Args[1:])  // --config=path/to/config.yml is required; a bad file panics
                                    // start from config.yml.example: cp config.yml.example config.yml

b, err := app.NewDefaultBuilder(ctx, cfg)   // real zap, Postgres, Redis, Prometheus
if err != nil { return err }

a, err := b.Build()                 // or app.NewBuilder().WithLogger(fake)... in a test
if err != nil { return err }

return a.Run(ctx)                   // blocks until ctx is cancelled, then stops
```
