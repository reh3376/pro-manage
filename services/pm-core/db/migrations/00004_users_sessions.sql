-- +goose Up
-- Users provisioned just in time on first SSO login (FSD §04 "User model"). Sessions and
-- pending logins live in Redis (§04 "Session model"); this table is the durable identity record.
CREATE TABLE users (
    user_id        uuid PRIMARY KEY,
    tenant_id      text NOT NULL DEFAULT 'default',
    external_id    text NOT NULL,                        -- the IdP's stable subject (OIDC sub)
    username       text NOT NULL,                        -- the login handle (preferred_username); what audit rows and pools key on today
    email          text NOT NULL DEFAULT '',
    display_name   text NOT NULL DEFAULT '',
    platform_roles text[] NOT NULL DEFAULT '{}',         -- §04 platform roles from the claims mapping, refreshed on every login
    attributes     jsonb NOT NULL DEFAULT '{}'::jsonb,   -- mapping-derived attributes (site, employee, external)
    is_local       boolean NOT NULL DEFAULT false,       -- break-glass accounts (none yet)
    is_active      boolean NOT NULL DEFAULT true,
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_login_at  timestamptz,
    login_count    integer NOT NULL DEFAULT 0,
    UNIQUE (tenant_id, external_id),
    UNIQUE (tenant_id, username)
);

-- Pool rows now record where they came from: the seed, an admin, or the IdP claims mapping.
-- IdP-sourced rows are replaced on every login; the others are left alone.
ALTER TABLE approver_pools ADD COLUMN source text NOT NULL DEFAULT 'manual' CHECK (source IN ('seed', 'manual', 'idp'));
UPDATE approver_pools SET source = 'seed' WHERE added_by = 'seed';

-- +goose Down
ALTER TABLE approver_pools DROP COLUMN source;
DROP TABLE IF EXISTS users;
