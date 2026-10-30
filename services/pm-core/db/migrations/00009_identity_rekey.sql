-- +goose Up
-- Attribution re-key (backlog B-11, FSD §04 "User model"): the IdP subject is the identity and
-- users.user_id its stable key; the login handle (preferred_username) becomes a display value that
-- an IdP may rename. Every row that keys on the handle gains the stable reference beside it, so a
-- renamed account keeps its memberships, grants, pools, ownerships, and history.

-- Audit rows carry the actor's stable id from here on (chain version 2 signs it; version 1 rows
-- predate the column and verify exactly as before). audit_events is append-only, so no backfill:
-- rows written before this migration keep the handle alone, which is what they signed.
ALTER TABLE audit_events ADD COLUMN actor_id uuid;
ALTER TABLE audit_events ADD COLUMN chain_version smallint NOT NULL DEFAULT 1;

ALTER TABLE project_members ADD COLUMN user_ref uuid REFERENCES users (user_id);
ALTER TABLE acl_grants      ADD COLUMN user_ref uuid REFERENCES users (user_id);
ALTER TABLE approver_pools  ADD COLUMN user_ref uuid REFERENCES users (user_id);
ALTER TABLE projects        ADD COLUMN owner_ref uuid REFERENCES users (user_id);

-- Backfill from the users who have already signed in; rows for handles that have not logged in yet
-- (seed users before their first SSO login) are attached on that first login.
UPDATE project_members m SET user_ref = u.user_id FROM users u WHERE m.user_ref IS NULL AND u.username = m.user_id;
UPDATE acl_grants g      SET user_ref = u.user_id FROM users u WHERE g.user_ref IS NULL AND u.username = g.user_id;
UPDATE approver_pools p  SET user_ref = u.user_id FROM users u WHERE p.user_ref IS NULL AND u.username = p.user_id;
UPDATE projects pr       SET owner_ref = u.user_id FROM users u WHERE pr.owner_ref IS NULL AND u.tenant_id = pr.tenant_id AND u.username = pr.owner_user_id;

CREATE INDEX project_members_user_ref_idx ON project_members (user_ref);
CREATE INDEX acl_grants_user_ref_idx ON acl_grants (user_ref);
CREATE INDEX approver_pools_user_ref_idx ON approver_pools (user_ref);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_id, at DESC);

-- +goose Down
DROP INDEX IF EXISTS audit_events_actor_idx;
DROP INDEX IF EXISTS approver_pools_user_ref_idx;
DROP INDEX IF EXISTS acl_grants_user_ref_idx;
DROP INDEX IF EXISTS project_members_user_ref_idx;
ALTER TABLE projects DROP COLUMN IF EXISTS owner_ref;
ALTER TABLE approver_pools DROP COLUMN IF EXISTS user_ref;
ALTER TABLE acl_grants DROP COLUMN IF EXISTS user_ref;
ALTER TABLE project_members DROP COLUMN IF EXISTS user_ref;
ALTER TABLE audit_events DROP COLUMN IF EXISTS chain_version;
ALTER TABLE audit_events DROP COLUMN IF EXISTS actor_id;
