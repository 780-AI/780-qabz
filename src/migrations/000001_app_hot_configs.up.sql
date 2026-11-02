BEGIN;

CREATE TABLE app_hot_configs (
    id         serial       PRIMARY KEY,
    key        varchar(256) NOT NULL UNIQUE,
    cnf_type   varchar(64)  NOT NULL,
    value      text         NOT NULL,
    created_at timestamp    NOT NULL DEFAULT now(),
    updated_at timestamp    NOT NULL DEFAULT now()
);

COMMIT;
