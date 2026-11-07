<!-- VERSION: 0.5 -->
# app

`app` is the bootstrap of the Qabz service. The type `App` owns every long-lived dependency: the logger, the Postgres database, the Redis cache, the metrics endpoint and the optional tracer. Entry points (for example `cmd/start-app`) build an `App` and run it. They hold no wiring of their own.

`App` depends only on small interfaces (`Logger`, `Database`, `Cache`, `Metrics`, `Tracer`), never on the concrete Postgres, Redis, zap or Prometheus types. Only `Config` is a concrete type. You make an `App` with `Builder`. `Build()` fails if the logger, database, cache or metrics is missing. `NewDefaultBuilder` is the one place that creates the real implementations from `Config`, which is read from a YAML file.

`Start` pings the database and the cache, prints their status in the log (host, port and database name only, never a password), and starts the metrics endpoint. If a step fails, it releases what it opened. `Stop` closes everything in reverse order and is safe to call twice. `Run` starts, waits for the context to end, then stops. Tests build an `App` with fakes and need no running server.

After the config reloader, `Start` starts the gRPC server (`entry/grpc`, address `server.grpc.addr`, default `:8082`) and then the HTTP gateway (`entry/http`, address `server.http.addr`, default `:8080`). The gateway serves the grpc-gateway routes and dials the local gRPC server, so each HTTP call runs through the real gRPC stack. `GET /v1/greeting` returns `{"message":"Hello"}` to ping the service.

Every gRPC call, and so every gateway route, goes through the JWT auth check (`entry/grpc/auth.go`, verifier in `common/auth`). A call without a valid `Authorization: Bearer <token>` gets gRPC `Unauthenticated` (HTTP 401). The check accepts only ES512, verifies with the issuer's public key (`auth.jwt.public_key_path`, a PEM file) and needs `exp`, `sub` (a UUID), `mobile`, `non_otp` (not `guest`) and an `aud` that contains `auth.jwt.audience`. Qabz never signs a token. Each rejection adds 1 to `qabz_auth_rejected_total{reason}` and writes no log line. `debug.auth.disable: true` turns the check off for development and logs one warning at start; the production config has no `debug` section, and a missing key keeps the check on. With the check on, `NewDefaultBuilder` fails when the key file or the audience is missing. The metrics endpoint is outside the check. `Stop` stops the metrics endpoint, the gateway, then the gRPC server. A failed `Start` stops only the parts that started.

Before the metrics endpoint starts, `Start` runs the SQL migrations (`database.migration.on_start`, default true; many containers can start together, because the migrator takes a Postgres advisory lock) and starts the optional config reloader. A `Config` field with the tag `hot_reload:"true"` is read again from the table `app_hot_configs` every `hot_reload_interval`. `App.Config()` returns a copy of the current settings, so do not keep it.

```go
cfg, _ := config.Boot(os.Args[1:])  // --config=path/to/config.yml is required; a bad file panics

b, err := app.NewDefaultBuilder(ctx, cfg)   // real zap, Postgres, Redis, Prometheus
if err != nil { return err }

a, err := b.Build()                 // or app.NewBuilder().WithLogger(fake)... in a test
if err != nil { return err }

return a.Run(ctx)                   // blocks until ctx is cancelled, then stops
```
