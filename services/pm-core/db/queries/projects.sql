-- name: CreateProject :one
INSERT INTO projects (
    project_id, tenant_id, code, name, description, portfolio_id, site_id, currency, owner_user_id,
    lifecycle_state, fel_stage, project_type, tpm_pillar_tags, hazardous_area_classification,
    regulatory_stamps, onboarding_level, start_date, target_end_date, baseline_budget_total,
    baseline_duration_days, tags, created_by, updated_by
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9,
    $10, $11, $12, $13, $14,
    $15, $16, $17, $18, $19,
    $20, $21, $22, $22
) RETURNING *;

-- name: GetProject :one
SELECT * FROM projects WHERE tenant_id = $1 AND project_id = $2;

-- name: GetProjectByCode :one
SELECT * FROM projects WHERE tenant_id = $1 AND code = $2;

-- name: ListProjects :many
SELECT * FROM projects
WHERE tenant_id = $1
  AND (sqlc.narg('site_id')::uuid IS NULL OR site_id = sqlc.narg('site_id')::uuid)
ORDER BY code;
