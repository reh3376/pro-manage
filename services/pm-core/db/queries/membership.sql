-- name: UpsertProjectMember :one
INSERT INTO project_members (project_id, user_id, base_role, invited_by, activated_at, expires_at)
VALUES ($1, $2, $3, $4, now(), $5)
ON CONFLICT (project_id, user_id) DO UPDATE SET base_role = EXCLUDED.base_role, expires_at = EXCLUDED.expires_at, invited_by = EXCLUDED.invited_by, invited_at = now()
RETURNING *;

-- name: GetProjectMember :one
SELECT * FROM project_members WHERE project_id = $1 AND user_id = $2;

-- name: ListProjectMembers :many
SELECT * FROM project_members WHERE project_id = $1 ORDER BY base_role, user_id;

-- name: DeleteProjectMember :execrows
DELETE FROM project_members WHERE project_id = $1 AND user_id = $2;

-- name: CountProjectOwners :one
SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND base_role = 'owner' AND (expires_at IS NULL OR expires_at > now());

-- name: InsertGrant :one
INSERT INTO acl_grants (grant_id, project_id, user_id, scope_kind, scope_target, permission, granted_by, expires_at, reason)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (project_id, user_id, scope_kind, scope_target, permission) DO UPDATE SET granted_by = EXCLUDED.granted_by, granted_at = now(), expires_at = EXCLUDED.expires_at, reason = EXCLUDED.reason
RETURNING *;

-- name: ListGrantsForUser :many
SELECT * FROM acl_grants WHERE project_id = $1 AND user_id = $2 AND (expires_at IS NULL OR expires_at > now()) ORDER BY scope_kind, scope_target;

-- name: ListProjectGrants :many
SELECT * FROM acl_grants WHERE project_id = $1 ORDER BY user_id, scope_kind, scope_target;

-- name: GetGrant :one
SELECT * FROM acl_grants WHERE grant_id = $1;

-- name: DeleteGrant :execrows
DELETE FROM acl_grants WHERE grant_id = $1 AND project_id = $2;

-- name: IsOpenStageAssignee :one
-- Workflow roles grant the read access a stage needs (§04): true while the user sits on an open stage of an in-flight workflow on the project.
SELECT EXISTS (
    SELECT 1 FROM approval_stages s JOIN approval_workflows w ON w.workflow_id = s.workflow_id
    WHERE w.tenant_id = $1 AND w.project_id = $2 AND w.status = 'in_flight' AND s.completed_at IS NULL AND $3::text = ANY (s.assignees)
) AS assigned;

-- name: ListVisibleProjects :many
-- Projects the user can see: membership (unexpired), or an open stage assignment, or every project for the platform roles the caller passes.
SELECT p.* FROM projects p
WHERE p.tenant_id = $1
  AND (sqlc.narg('site_id')::uuid IS NULL OR p.site_id = sqlc.narg('site_id')::uuid)
  AND (
    sqlc.arg(see_all)::boolean
    OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.project_id AND m.user_id = sqlc.arg(user_id)::text AND (m.expires_at IS NULL OR m.expires_at > now()))
    OR EXISTS (SELECT 1 FROM approval_stages s JOIN approval_workflows w ON w.workflow_id = s.workflow_id
               WHERE w.project_id = p.project_id AND w.status = 'in_flight' AND s.completed_at IS NULL AND sqlc.arg(user_id)::text = ANY (s.assignees))
  )
ORDER BY p.code;
