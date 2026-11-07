<!-- VERSION: 0.1 -->
# 015_dev_bill_database — dev-only commands to create, seed and unseed the bill database

Status: pending

## Description

Add two App commands for development only:
- **Create and seed:** creates the full `bill` service database schema (every table that `bill` has) in the configured Postgres database, then inserts the rows of the "Seed" section below.
- **Unseed:** undoes the seed.

A new config key `mode` accepts `dev`, `stage` or `prod`. Both commands run only when `mode` is `dev`. In any other mode they stop before they connect to the database: they print an error and exit with a non-zero code. They are for development and nothing else.

Help:
- The schema source is the legacy `bill` repository (`~/projects/bill`): `pkg/migrations/` (go-pg models in `domain/entity/` plus raw SQL) gives the end state of each table. `docs/initial-discovery/database.html` lists the `bill` tables (`orders`, `bill_inquiries`, `bill_inquiry_requests`, `providers`, `fees`, `services`, `bills`, `errors`). Compare the two; the column types and defaults must match `bill`.
- The commands are dispatched in `src/cmd/run.go`, like `migrate`, and take `--config`.
- The schema is not a qabz migration: qabz does not own the `bill` tables (`system.html`, Issue #2), so nothing goes into `src/migrations/`.

Points for the design step:
- The command names (for example `devdb seed` and `devdb unseed`).
- The meaning of "unseed": delete only the seeded rows (by `id`), or also drop the tables that the create step made. A table that existed before the command must not be dropped.
- Running twice: the create step does not fail when the tables or the rows already exist (for example `CREATE TABLE IF NOT EXISTS` and `ON CONFLICT (id) DO NOTHING`), and does not change existing rows.
- The default of `mode` when the key is missing. A missing key must not allow the commands (for example the default is `prod`, or the key is required).
- Whether `mode` changes anything else now (for example `debug.auth.disable`). If not, the task says so.
- A second guard against a wrong database (for example only a local host), in case `mode: dev` is set by mistake on a shared database.
- The `services` and `providers` sequences, if `bill` uses them: after an insert with fixed `id` values, set each sequence past the largest `id`.

## Seed

The rows as the operator gave them (2026-11-07), one JSON object per line. A `null` is a database NULL. `updated_at` keeps its time zone.

### `services` (5 rows)

```json
{"lang":"fa-IR","is_active":true,"id":1,"name":"برق","tag":null,"inquiry":true,"updated_at":"2022-02-05T10:53:35.586761+03:30","order":0,"type":"electricity","icon":null},
{"lang":"fa-IR","is_active":true,"id":2,"name":"گاز","tag":null,"inquiry":true,"updated_at":"2022-02-05T10:53:35.586762+03:30","order":0,"type":"gas","icon":null},
{"lang":"fa-IR","is_active":true,"id":3,"name":"تلفن همراه","tag":null,"inquiry":true,"updated_at":"2022-02-05T10:53:35.586762+03:30","order":0,"type":"mobile","icon":null},
{"lang":"fa-IR","is_active":true,"id":5,"name":"آب","tag":null,"inquiry":true,"updated_at":"2022-02-05T10:53:35.586762+03:30","order":0,"type":"water","icon":null},
{"lang":"fa-IR","is_active":true,"id":4,"name":"تلفن","tag":null,"inquiry":true,"updated_at":"2022-02-05T10:53:35.586762+03:30","order":0,"type":"tel","icon":null}
```

### `providers` (45 rows)

```json
{"bill_type":"irancell","priority":10,"id":26,"name":"irancell","updated_at":"2022-04-20T21:32:06.0725+04:30","status":"enable","timeout":500,"action":"order","subject":"irancell-bill","type":"retryable","use_nats":true},
{"bill_type":"electricity","priority":9,"id":2,"name":"tavanir","updated_at":"2022-05-23T08:03:28.988789+04:30","status":"enable","timeout":100,"action":"inquiry","subject":"tavanir-inquiry","type":"retryable","use_nats":true},
{"bill_type":"irancell","priority":1,"id":38,"name":"irancell","updated_at":"2024-07-07T14:17:54.006434+03:30","status":"enable","timeout":500,"action":"inquiry","subject":"irancell-inquiry","type":"retryable","use_nats":false},
{"bill_type":"mobilemidtermRightel","priority":10,"id":34,"name":"qabzino","updated_at":"2023-07-02T09:29:48.373168+03:30","status":"enable","timeout":1000,"action":"inquiry","subject":"qabzino-rightel-inquiry","type":"retryable","use_nats":false},
{"bill_type":"mobilemidtermIrancell","priority":10,"id":33,"name":"qabzino","updated_at":"2023-07-02T09:29:48.373168+03:30","status":"disable","timeout":1000,"action":"inquiry","subject":"qabzino-irancell-inquiry","type":"retryable","use_nats":false},
{"bill_type":"tel","priority":9,"id":50,"name":"parsian","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"disable","timeout":100,"action":"order","subject":"parsian-bill-payment","type":"retryable","use_nats":false},
{"bill_type":"mobile","priority":9,"id":48,"name":"parsian","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"disable","timeout":100,"action":"order","subject":"parsian-bill-payment","type":"retryable","use_nats":false},
{"bill_type":"mobilemidtermRightel","priority":10,"id":37,"name":"eways2","updated_at":"2023-07-02T09:29:48.373168+03:30","status":"enable","timeout":1000,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"telmidterm","priority":10,"id":20,"name":"eways2","updated_at":"2026-10-24T12:08:31.649964+03:30","status":"disable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"water","priority":9,"id":45,"name":"parsian","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"enable","timeout":100,"action":"order","subject":"parsian-bill-payment","type":"retryable","use_nats":false},
{"bill_type":"tel","priority":10,"id":52,"name":"qabzino","updated_at":"2023-07-02T09:29:48.373168+03:30","status":"disable","timeout":1000,"action":"inquiry","subject":"qabzino-tel-inquiry","type":"retryable","use_nats":false},
{"bill_type":"telmidterm","priority":10,"id":600,"name":"eways2","updated_at":"2026-10-24T12:08:31.649964+03:30","status":"disable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"mobilemidterm","priority":9,"id":149,"name":"parsian","updated_at":"2026-10-26T19:46:20.796405+03:30","status":"enable","timeout":100,"action":"order","subject":"parsian-bill-payment","type":"retryable","use_nats":false},
{"bill_type":"water","priority":10,"id":36,"name":"qabzino","updated_at":"2023-09-17T09:29:48.373168+03:30","status":"disable","timeout":1000,"action":"inquiry","subject":"qabzino-water-inquiry","type":"retryable","use_nats":false},
{"bill_type":"telmidterm","priority":1,"id":123456,"name":"karashop","updated_at":"2026-04-13T12:23:25.147376+03:30","status":"enable","timeout":500,"action":"inquiry","subject":"karashop-inquiry","type":null,"use_nats":false},
{"bill_type":"mobilemidterm","priority":10,"id":29,"name":"jiring-switch","updated_at":"2026-10-26T19:46:20.796405+03:30","status":"disable","timeout":500,"action":"order","subject":"jiring-switch-bill-payment","type":"retryable","use_nats":true},
{"bill_type":"tel","priority":7,"id":40,"name":"parsian","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"disable","timeout":200,"action":"inquiry","subject":"parsian-bill-inquiry","type":"retryable","use_nats":false},
{"bill_type":"mobileRightel","priority":10,"id":32,"name":"qabzino","updated_at":"2026-10-24T16:12:14.604445+03:30","status":"disable","timeout":1000,"action":"inquiry","subject":"qabzino-rightel-inquiry","type":"retryable","use_nats":false},
{"bill_type":"telmidterm","priority":10,"id":53,"name":"qabzino","updated_at":"2023-07-02T09:29:48.373168+03:30","status":"disable","timeout":1000,"action":"inquiry","subject":"qabzino-telmidterm-inquiry","type":"retryable","use_nats":false},
{"bill_type":"trafficFine","priority":10,"id":24,"name":"eways2","updated_at":"2022-03-10T12:42:16.895653+03:30","status":"enable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"tel","priority":9,"id":124325,"name":"karashop","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"enable","timeout":1000,"action":"inquiry","subject":"karashop-inquiry","type":"retryable","use_nats":false},
{"bill_type":"gas","priority":4,"id":99,"name":"gas-company","updated_at":"2026-10-11T16:54:04.730445+03:30","status":"disable","timeout":100,"action":"inquiry","subject":"gas-company-inquiry","type":"retryable","use_nats":false},
{"bill_type":"tax","priority":10,"id":22,"name":"eways2","updated_at":"2022-03-10T12:42:16.895653+03:30","status":"enable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"gas","priority":10,"id":18,"name":"eways2","updated_at":"2022-03-10T12:42:16.895653+03:30","status":"disable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"raw","priority":10,"id":49,"name":"eways2","updated_at":"2022-03-10T12:42:16.895653+03:30","status":"enable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"taxOrganization","priority":10,"id":23,"name":"eways2","updated_at":"2022-03-10T12:42:16.895653+03:30","status":"enable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"water","priority":9,"id":44,"name":"parsian","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"enable","timeout":100,"action":"inquiry","subject":"parsian-bill-inquiry","type":"retryable","use_nats":false},
{"bill_type":"mobile","priority":10,"id":28,"name":"jiring-switch","updated_at":"2022-07-04T10:29:48.373168+04:30","status":"disable","timeout":100,"action":"inquiry","subject":"jiring-switch-bill-inquiry","type":"retryable","use_nats":true},
{"bill_type":"mobile","priority":10,"id":30,"name":"jiring-switch","updated_at":"2026-10-13T17:51:31.691191+03:30","status":"disable","timeout":500,"action":"order","subject":"jiring-switch-bill-payment","type":"retryable","use_nats":true},
{"bill_type":"gas","priority":4,"id":43,"name":"parsian","updated_at":"2026-10-26T18:30:45.059563+03:30","status":"enable","timeout":100,"action":"inquiry","subject":"parsian-bill-inquiry","type":"retryable","use_nats":false},
{"bill_type":"gas","priority":10,"id":35,"name":"qabzino","updated_at":"2026-10-26T18:30:45.059563+03:30","status":"disable","timeout":10000,"action":"inquiry","subject":"qabzino-gas-inquiry","type":"retryable","use_nats":false},
{"bill_type":"tel","priority":10,"id":21,"name":"eways2","updated_at":"2022-03-10T12:42:16.895653+03:30","status":"disable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"mobileIrancell","priority":10,"id":31,"name":"qabzino","updated_at":"2026-10-24T10:16:46.558778+03:30","status":"enable","timeout":1000,"action":"inquiry","subject":"qabzino-irancell-inquiry","type":"retryable","use_nats":false},
{"bill_type":"gas","priority":9,"id":46,"name":"parsian","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"enable","timeout":100,"action":"order","subject":"parsian-bill-payment","type":"retryable","use_nats":false},
{"bill_type":"telmidterm","priority":9,"id":51,"name":"parsian","updated_at":"2026-10-24T12:08:31.649964+03:30","status":"enable","timeout":100,"action":"order","subject":"parsian-bill-payment","type":"retryable","use_nats":false},
{"bill_type":"mobilemidtermRightel","priority":9,"id":500,"name":"parsian","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"disable","timeout":100,"action":"order","subject":"parsian-bill-payment","type":"retryable","use_nats":false},
{"bill_type":"mobilemidterm","priority":10,"id":27,"name":"jiring-switch","updated_at":"2026-10-26T18:41:03.33415+03:30","status":"enable","timeout":100,"action":"inquiry","subject":"jiring-switch-bill-inquiry","type":"retryable","use_nats":true},
{"bill_type":"electricity","priority":10,"id":17,"name":"eways2","updated_at":"2026-10-13T17:15:37.270428+03:30","status":"disable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"mobilemidterm","priority":4,"id":41,"name":"parsian","updated_at":"2026-10-26T18:41:03.33415+03:30","status":"disable","timeout":100,"action":"inquiry","subject":"parsian-bill-inquiry","type":"retryable","use_nats":false},
{"bill_type":"raw","priority":10,"id":601,"name":"parsian","updated_at":"2022-03-10T12:42:16.895653+03:30","status":"disable","timeout":500,"action":"order","subject":"parsian-bill-payment","type":"retryable","use_nats":true},
{"bill_type":"water","priority":10,"id":19,"name":"eways2","updated_at":"2022-03-10T12:42:16.895653+03:30","status":"disable","timeout":500,"action":"order","subject":"eways2-bill","type":"retryable","use_nats":true},
{"bill_type":"electricity","priority":9,"id":47,"name":"parsian","updated_at":"2026-10-13T17:15:37.270428+03:30","status":"enable","timeout":100,"action":"order","subject":"parsian-bill-payment","type":"retryable","use_nats":false},
{"bill_type":"mobile","priority":2,"id":39,"name":"parsian","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"enable","timeout":100,"action":"inquiry","subject":"parsian-bill-inquiry","type":"retryable","use_nats":false},
{"bill_type":"telmidterm","priority":9,"id":42,"name":"parsian","updated_at":"2022-05-28T14:36:32.109064+04:30","status":"enable","timeout":100,"action":"inquiry","subject":"parsian-bill-inquiry","type":"retryable","use_nats":false},
{"bill_type":"electricitybillfile","priority":9,"id":55,"name":"tavanir","updated_at":"2022-05-23T08:03:28.988789+04:30","status":"enable","timeout":100,"action":"inquiry","subject":"tavanir-inquiry","type":"retryable","use_nats":true}
```

The `providers` rows give no value for `inquiry_step` and `provider_fee`; they get the `bill` column defaults.

## Definition of Done

- The config key `mode` (`dev`, `stage`, `prod`) is in `config.go`, `config.yml.example` and `config.yml` (conventions § 8); any other value stops the start with an error.
- With `mode: dev`, the create command makes every `bill` table with the `bill` column types and defaults, and inserts exactly the seed rows above (`GET /v1/services` then returns the 5 services).
- A second run of the create command does not fail and does not change the data.
- The unseed command removes what the create command added, per the design decision.
- With `mode` set to `stage` or `prod`, or missing, both commands exit with a non-zero code before they connect to the database, and change nothing.
- Unit tests cover the mode guard for every mode; live tests cover create, a second create, and unseed. Build, vet and tests pass.
- `app/README.md`, `folder_structure.md` and the CHANGELOG are updated in the same commit.

## Verification

With `mode: dev`, run the create command, then the App, and call `GET /v1/services`. Pass: the 5 services from the seed, in the `id` tie order (all have `order` 0). Run the create command again: no error, same rows. Run the unseed command: the seeded rows are gone. Set `mode: prod` and run both commands: each exits with a non-zero code and the database does not change.
