-- name: UpsertUserOnLogin :one
-- Just-in-time provisioning (§04): the IdP subject is the identity; the profile and roles refresh on every login.
INSERT INTO users (user_id, tenant_id, external_id, username, email, display_name, platform_roles, attributes, last_login_at, login_count)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), 1)
ON CONFLICT (tenant_id, external_id) DO UPDATE SET
    username = EXCLUDED.username, email = EXCLUDED.email, display_name = EXCLUDED.display_name,
    platform_roles = EXCLUDED.platform_roles, attributes = EXCLUDED.attributes,
    last_login_at = now(), login_count = users.login_count + 1
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users WHERE tenant_id = $1 AND username = $2;

-- name: GetUser :one
SELECT * FROM users WHERE tenant_id = $1 AND user_id = $2;

-- name: ListUsers :many
SELECT * FROM users WHERE tenant_id = $1 ORDER BY username;

-- name: DeleteIdpPoolRows :exec
DELETE FROM approver_pools WHERE user_id = $1 AND source = 'idp';

-- name: InsertPoolRow :exec
INSERT INTO approver_pools (pool, user_id, site_id, added_by, source) VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (pool, user_id) DO UPDATE SET site_id = EXCLUDED.site_id, added_by = EXCLUDED.added_by, source = EXCLUDED.source, added_at = now()
WHERE approver_pools.source = 'idp';
