-- +goose Up
-- `tags` is JSON (array of strings), not TEXT[]: array-type support was not
-- validated against SereneDB during the compatibility spike, so this avoids
-- an untested surface. See docs/specs for the full compatibility notes.
CREATE TABLE nodes (
    org               TEXT NOT NULL,
    name              TEXT NOT NULL,
    fqdn              TEXT,
    platform          TEXT,
    platform_family   TEXT,
    platform_version  TEXT,
    environment       TEXT,
    chef_version      TEXT,
    policy_name       TEXT,
    policy_group      TEXT,
    attributes        JSON NOT NULL DEFAULT '{}',
    tags              JSON NOT NULL DEFAULT '[]',
    last_ccr_status   TEXT,
    latest_run_id     UUID,
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org, name)
);

-- +goose Down
DROP TABLE nodes;
