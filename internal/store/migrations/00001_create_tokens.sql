-- +goose Up
-- `org` is the token's home org. Chef Automate's own data-collector
-- protocol is not org-scoped: chef-client messages carry an
-- "organization_name" field, but InSpec's automate-reporter compliance
-- reports do not carry any org field at all. A token's org is the fallback
-- (and, for compliance reports, the only source) of org scoping -- see
-- docs/specs for the full explanation. It is nullable because a token used
-- only for queries (e.g. Cinc Console's service account) doesn't need one.
CREATE TABLE tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    token_hash   TEXT NOT NULL UNIQUE,
    org          TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

-- +goose Down
DROP TABLE tokens;
