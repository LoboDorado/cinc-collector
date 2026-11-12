-- +goose Up
-- `uploaded_by` references `tokens`, so this migration must run after
-- 00001_create_tokens.sql: SereneDB has no ALTER TABLE ... ADD FOREIGN KEY,
-- so the FK must be declared inline here, not added later.
CREATE TABLE profiles (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org          TEXT NOT NULL,
    name         TEXT NOT NULL,
    version      TEXT NOT NULL,
    title        TEXT,
    summary      TEXT,
    owner        TEXT,
    sha256       TEXT NOT NULL,
    storage_key  TEXT NOT NULL,
    size_bytes   BIGINT NOT NULL,
    metadata     JSON NOT NULL DEFAULT '{}',
    uploaded_by  UUID REFERENCES tokens(id),
    uploaded_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org, name, version)
);

-- +goose Down
DROP TABLE profiles;
