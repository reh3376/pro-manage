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

-- name: GetUserBySubject :one
SELECT * FROM users WHERE tenant_id = $1 AND external_id = $2;

-- Identity attach (B-11): rows keyed on a handle before its user signed in get the stable reference
-- on that first login; rows created afterwards get it at insert through the Link* queries.

-- name: AttachMemberIdentity :exec
UPDATE project_members SET user_ref = $1 WHERE user_id = $2 AND user_ref IS NULL;

-- name: AttachGrantIdentity :exec
UPDATE acl_grants SET user_ref = $1 WHERE user_id = $2 AND user_ref IS NULL;

-- name: AttachPoolIdentity :exec
UPDATE approver_pools SET user_ref = $1 WHERE user_id = $2 AND user_ref IS NULL;

-- name: AttachProjectOwnerIdentity :exec
UPDATE projects SET owner_ref = $1 WHERE tenant_id = $2 AND owner_user_id = $3 AND owner_ref IS NULL;

-- name: LinkMemberIdentity :exec
UPDATE project_members m SET user_ref = u.user_id FROM users u
WHERE m.project_id = $1 AND m.user_id = $2 AND u.tenant_id = $3 AND u.username = m.user_id;

-- name: LinkGrantIdentity :exec
UPDATE acl_grants g SET user_ref = u.user_id FROM users u
WHERE g.grant_id = $1 AND u.tenant_id = $2 AND u.username = g.user_id;

-- name: LinkPoolIdentity :exec
UPDATE approver_pools p SET user_ref = u.user_id FROM users u
WHERE p.pool = $1 AND p.user_id = $2 AND u.tenant_id = $3 AND u.username = p.user_id;

-- name: LinkProjectOwnerIdentity :exec
UPDATE projects pr SET owner_ref = u.user_id FROM users u
WHERE pr.project_id = $1 AND u.tenant_id = pr.tenant_id AND u.username = pr.owner_user_id;

-- Rename cascade (B-11): when the IdP renames a login handle, every row that shows the handle
-- follows the stable reference; history (completed stages, requested_by, audit) keeps the old
-- handle, which is what it recorded at the time.

-- name: RenameMemberHandle :execrows
UPDATE project_members SET user_id = $2 WHERE user_ref = $1;

-- name: RenameGrantHandle :execrows
UPDATE acl_grants SET user_id = $2 WHERE user_ref = $1;

-- name: RenamePoolHandle :execrows
UPDATE approver_pools SET user_id = $2 WHERE user_ref = $1;

-- name: RenameProjectOwnerHandle :execrows
UPDATE projects SET owner_user_id = $2, updated_at = now(), updated_by = $2, version = version + 1 WHERE owner_ref = $1;

-- name: RenameOpenStageAssignee :execrows
-- Open stages route to the new handle; completed ones keep the handle that decided.
UPDATE approval_stages SET assignees = array_replace(assignees, $1::text, $2::text)
WHERE completed_at IS NULL AND $1::text = ANY (assignees);
