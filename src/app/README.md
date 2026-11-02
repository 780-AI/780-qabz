<!-- VERSION: 0.1 -->
# app

`app` is the bootstrap of the Qabz service. The type `App` owns every long-lived dependency: the logger, the Postgres database, the Redis cache, the metrics endpoint and the optional tracer. Entry points (for example `cmd/start-app`) build an `App` and run it. They hold no wiring of their own.

`App` depends only on small interfaces (`Logger`, `Database`, `Cache`, `Metrics`, `Tracer`), never on the concrete Postgres, Redis, zap or Prometheus types. Only `Config` is a concrete type. You make an `App` with `Builder`. `Build()` fails if the logger, database, cache or metrics is missing. `NewDefaultBuilder` is the one place that creates the real implementations from `Config`, which is read from ENV.

`Start` pings the database and the cache, prints their status in the log (host, port and database name only, never a password), and starts the metrics endpoint. If a step fails, it releases what it opened. `Stop` closes everything in reverse order and is safe to call twice. `Run` starts, waits for the context to end, then stops. Tests build an `App` with fakes and need no running server.

```go
cfg, err := config.Load()           // from ENV; POSTGRES_PASSWORD is required
if err != nil { return err }

b, err := app.NewDefaultBuilder(ctx, cfg)   // real zap, Postgres, Redis, Prometheus
if err != nil { return err }

a, err := b.Build()                 // or app.NewBuilder().WithLogger(fake)... in a test
if err != nil { return err }

return a.Run(ctx)                   // blocks until ctx is cancelled, then stops
```
