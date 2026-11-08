-- The tables of the bill service database, for development only (task 015). Not a qabz migration:
-- qabz does not own these tables (system.html, Issue #2).
--
-- Source: the bill code (~/projects/bill, pkg/migrations/ 1 to 27 and the go-pg entities in
-- domain/entity/). Each table is the state that bill's migrations leave on a new database: the
-- CREATE TABLE that go-pg v10.9.3 makes from the entity (printed with orm.CreateTableQuery), plus
-- the later raw SQL changes that still apply on top of it (named at each table). A raw SQL
-- "ADD COLUMN" of a column that the entity already has does nothing on a new database, so it is
-- not here (for example migrations 18 and 19 on orders.fee and orders.base_price).
--
-- Not here: outboxes. Migration 4 (outboxes) is commented out in bill, so bill makes no outboxes
-- table; migration 13 (ALTER TABLE outboxes) works only on an old database that has it.
--
-- Every statement has IF NOT EXISTS: a table or an index that already exists is not changed.

-- 1_errors.go (entity.Error)
CREATE TABLE IF NOT EXISTS "errors" ("id" bigserial, "message" text, "description" text, "response_code" bigint, "service_code" bigint, "http_code" bigint, "lang" text, "status" text, PRIMARY KEY ("id"));

-- 2_providers.go (entity.Provider; 9_add_use_nats_field.go and 24_karashop_changes.go add columns
-- that the entity already has)
CREATE TABLE IF NOT EXISTS "providers" ("id" bigserial, "name" varchar(64), "subject" varchar(64), "bill_type" varchar(64), "action" varchar(16), "type" varchar(32), "priority" smallint, "timeout" integer, "status" varchar(32) DEFAULT 'enable', "updated_at" timestamptz DEFAULT now(), "use_nats" boolean DEFAULT true, "inquiry_step" smallint DEFAULT 1, "provider_fee" bigint, PRIMARY KEY ("id"));

-- 3_services.go (entity.Service; 8, 14 and 15 add columns that the entity already has)
CREATE TABLE IF NOT EXISTS "services" ("id" bigserial, "name" varchar(64), "type" varchar(32) NOT NULL, "tag" varchar(32), "order" smallint DEFAULT 0, "is_active" boolean DEFAULT true, "lang" varchar(32) DEFAULT 'fa-IR', "inquiry" boolean DEFAULT true, "icon" text, "updated_at" timestamptz DEFAULT now(), PRIMARY KEY ("id"));

-- 5_order.go (entity.Order) + 21_set_default_is_raw_to_order_.go (is_raw DEFAULT FALSE)
CREATE TABLE IF NOT EXISTS "orders" ("uuid" uuid, "order_id" uuid, "user_id" uuid, "bill_type" varchar(64), "bill_id" varchar(32), "pay_id" varchar(32), "option" varchar(32), "price" bigint, "data" varchar(256), "ref_id" varchar(32), "provider_id" bigint, "provider_name" varchar(64), "rrn" varchar(64), "message" varchar(256), "status" varchar(32), "status_code" integer, "created_at" timestamptz DEFAULT now(), "updated_at" timestamptz DEFAULT now(), "fee" bigint, "fee_type" varchar(16), "fee_config" jsonb, "fee_id" integer, "base_price" bigint, "is_raw" bool DEFAULT FALSE, "provider_confirmed" boolean, "provider_fee" bigint, "bill_inquiry_id" varchar(50), PRIMARY KEY ("uuid"));

-- 6_mybills.go (entity.Bill)
CREATE TABLE IF NOT EXISTS "bills" ("id" uuid, "user_id" uuid, "name" varchar(128), "type" varchar(32), "bill_id" varchar(32), "payment_deadline" varchar(32), "status" varchar(32) DEFAULT 'enable', "last_order_date" text, "created_at" timestamptz DEFAULT now(), "data" varchar(100), "json_data" jsonb, PRIMARY KEY ("id"), UNIQUE ("user_id", "type", "bill_id"));

-- 10_plate.go (entity.Plate)
CREATE TABLE IF NOT EXISTS "plates" ("id" uuid, "user_id" uuid, "status" varchar(20) DEFAULT 'enabled', "name" varchar(128), "code" varchar(20), "letter" varchar(20), "serial" varchar(20), "province_number" varchar(20), "create_date" timestamptz DEFAULT now(), PRIMARY KEY ("id"));

-- 11_toll.go (entity.Toll)
CREATE TABLE IF NOT EXISTS "tolls" ("id" uuid, "plate_id" uuid, "user_id" uuid, "order_id" uuid, "status" varchar(20) DEFAULT 'enable', "price" bigint, "plate" varchar(40), "place" varchar(250), "tolled_at" timestamptz, "created_at" timestamptz DEFAULT now(), PRIMARY KEY ("id"));

-- 22_add_bill_inquiries.go + 24_karashop_changes.go (tid TYPE VARCHAR(100))
CREATE TABLE IF NOT EXISTS bill_inquiries
(
    "id"               uuid                        NOT NULL PRIMARY KEY,
    "user_id"          uuid                        NOT NULL,
    "inquiry_id"       VARCHAR(15)                 NOT NULL,
    "provider_id"      BIGINT                      NOT NULL,
    "provider_name"    VARCHAR(15)                 NOT NULL,
    "bill_type"        VARCHAR(20)                 NOT NULL,
    "tid"              VARCHAR(100)                NULL,
    "inquiry_type"     VARCHAR(10)                 NULL,
    "bill_id"          VARCHAR(15)                 NULL,
    "pay_id"           VARCHAR(15)                 NULL,
    "payment_deadline" VARCHAR(10)                 NULL,
    "amount"           BIGINT                      NULL,
    "extra_data"       jsonb                       NULL,
    "is_cached"        BOOL                        DEFAULT FALSE,
    "is_paid"          BOOL                        DEFAULT FALSE,
    "updated_at"       TIMESTAMP(6) WITH TIME ZONE NOT NULL DEFAULT (CURRENT_TIMESTAMP(6)),
    "created_at"       TIMESTAMP(6) WITH TIME ZONE NOT NULL DEFAULT (CURRENT_TIMESTAMP(6))
);
CREATE INDEX IF NOT EXISTS "idx_inquiry_id_created_at" ON "bill_inquiries" ("inquiry_id", created_at DESC);

-- 23_bill_inquiry_requests.go + 24_karashop_changes.go (provider_trace_id TYPE VARCHAR(100))
CREATE TABLE IF NOT EXISTS bill_inquiry_requests
(
    "id"                uuid                        NOT NULL PRIMARY KEY,
    "user_id"           uuid                        NOT NULL,
    "dialer"            VARCHAR(12)                 NOT NULL,
    "inquiry_id"        VARCHAR(15)                 NOT NULL,
    "bill_type"         VARCHAR(50)                 NOT NULL,
    "tid"               VARCHAR(30)                 NOT NULL,
    "provider_name"     VARCHAR(64)                 NOT NULL,
    "provider_trace_id" VARCHAR(100)                NULL,
    "status"            VARCHAR(30)                 NOT NULL,
    "created_at"        TIMESTAMP(6) WITH TIME ZONE NOT NULL DEFAULT (CURRENT_TIMESTAMP(6)),

    CONSTRAINT "bir_tid_status_uq" UNIQUE ("tid", "status")
);
CREATE INDEX IF NOT EXISTS "bir_inquiry_id_idx" ON "bill_inquiry_requests" ("inquiry_id");
CREATE INDEX IF NOT EXISTS "bir_created_at_idx" ON "bill_inquiry_requests" ("created_at" DESC);

-- 27_fees.go (entity.Fee and its three indexes)
CREATE TABLE IF NOT EXISTS "fees" ("id" bigserial, "title" varchar(128) NOT NULL, "provider_id" integer, "type" varchar(16) NOT NULL, "config" jsonb NOT NULL DEFAULT '{}', "user_id" uuid, "created_at" timestamptz DEFAULT now(), "updated_at" timestamptz DEFAULT now(), "deleted_at" timestamptz, PRIMARY KEY ("id"));
CREATE INDEX IF NOT EXISTS fees_user_provider_idx
ON fees (user_id, provider_id, created_at DESC)
WHERE user_id IS NOT NULL AND provider_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS fees_user_only_idx
ON fees (user_id, created_at DESC)
WHERE user_id IS NOT NULL AND provider_id IS NULL AND deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS fees_provider_only_idx
ON fees (provider_id, created_at DESC)
WHERE user_id IS NULL AND provider_id IS NOT NULL AND deleted_at IS NULL;
