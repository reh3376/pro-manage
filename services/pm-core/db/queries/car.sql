-- name: CreateEstimateArtifact :one
INSERT INTO estimate_artifacts (estimate_id, tenant_id, project_id, aace_class, as_of, prepared_by, reviewed_by, basis_of_estimate_doc_ref,
    scope_definition_pct, accuracy_range_low_pct, accuracy_range_high_pct, tic, contingency_amount, contingency_method, currency, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING *;

-- name: GetEstimateArtifact :one
SELECT * FROM estimate_artifacts WHERE tenant_id = $1 AND estimate_id = $2;

-- name: CreatePromise :one
INSERT INTO business_case_promises (promise_id, tenant_id, project_id, kind, metric_id, loss_category_id, baseline_value, baseline_period,
    promised_value, promised_by_date, dollar_value_per_year, measurement_basis, status, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14)
RETURNING *;

-- name: ListPromisesForProject :many
SELECT * FROM business_case_promises WHERE tenant_id = $1 AND project_id = $2 ORDER BY created_at;

-- name: CreateCAR :one
INSERT INTO capital_authorizations (car_id, tenant_id, project_id, code, kind, amount_requested, currency, basis, estimate_id,
    fel_stage_at_submission, business_case, supersedes_car_id, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $13)
RETURNING *;

-- name: GetCAR :one
SELECT * FROM capital_authorizations WHERE tenant_id = $1 AND car_id = $2;

-- name: ListCARsForProject :many
SELECT * FROM capital_authorizations WHERE tenant_id = $1 AND project_id = $2 ORDER BY created_at DESC;

-- name: SumApprovedCARs :one
SELECT COALESCE(SUM(authorized_amount), 0)::numeric(18,2) AS total
FROM capital_authorizations WHERE tenant_id = $1 AND project_id = $2 AND status = 'approved';

-- name: NextCARSequence :one
SELECT COUNT(*)::int + 1 AS n FROM capital_authorizations WHERE tenant_id = $1;

-- name: SetCARInFlight :one
UPDATE capital_authorizations
SET status = 'in_flight', workflow_id = $3, fel_stage_at_submission = $4, updated_at = now(), updated_by = $5, version = version + 1
WHERE tenant_id = $1 AND car_id = $2 AND status = 'draft'
RETURNING *;

-- name: SetCAROutcome :one
UPDATE capital_authorizations
SET status = $3, authorized_amount = $4, authorized_at = $5, updated_at = now(), updated_by = $6, version = version + 1
WHERE tenant_id = $1 AND car_id = $2 AND status = 'in_flight'
RETURNING *;

-- name: CreateWorkflow :one
INSERT INTO approval_workflows (workflow_id, tenant_id, project_id, kind, subject_type, subject_id, initiated_by, policy_snapshot)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetWorkflow :one
SELECT * FROM approval_workflows WHERE tenant_id = $1 AND workflow_id = $2;

-- name: AdvanceWorkflow :exec
UPDATE approval_workflows SET current_stage = $3, version = version + 1 WHERE tenant_id = $1 AND workflow_id = $2;

-- name: CompleteWorkflow :exec
UPDATE approval_workflows SET status = $3, completed_at = now(), version = version + 1 WHERE tenant_id = $1 AND workflow_id = $2;

-- name: CreateStage :one
INSERT INTO approval_stages (stage_id, workflow_id, stage_order, role_required, assignees, sla_minutes, entered_at, stage_due_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListStages :many
SELECT * FROM approval_stages WHERE workflow_id = $1 ORDER BY stage_order;

-- name: GetStage :one
SELECT * FROM approval_stages WHERE workflow_id = $1 AND stage_order = $2;

-- name: OpenStage :exec
UPDATE approval_stages SET entered_at = $3, stage_due_at = $4 WHERE workflow_id = $1 AND stage_order = $2;

-- name: DecideStage :one
UPDATE approval_stages
SET completed_at = now(), completed_by = $3, decision = $4, comment = $5, guidance_id = $6, classification_verdict = $7, fail_tier = $8
WHERE workflow_id = $1 AND stage_order = $2 AND completed_at IS NULL
RETURNING *;

-- name: InsertGuidanceOutcome :one
INSERT INTO guidance_outcomes (outcome_id, tenant_id, audit_id, project_id, user_id, action_type, guidance_ids, guidance_items_snapshot,
    classification_verdict, violated_codes, fail_tier, user_response, dismiss_reason, override_admin_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
RETURNING *;

-- name: ListGuidanceOutcomesForProject :many
SELECT * FROM guidance_outcomes WHERE tenant_id = $1 AND project_id = $2 ORDER BY at DESC LIMIT $3;

-- name: PoolMembers :many
SELECT user_id FROM approver_pools WHERE pool = $1 AND (site_id IS NULL OR site_id = sqlc.narg('site_id')::uuid) ORDER BY user_id;

-- name: UpsertPoolMember :exec
INSERT INTO approver_pools (pool, user_id, site_id, added_by) VALUES ($1, $2, $3, $4)
ON CONFLICT (pool, user_id) DO NOTHING;

-- name: OpenStagesForUser :many
-- The inbox: stages awaiting this user's decision on in-flight workflows.
SELECT s.*, w.kind AS workflow_kind, w.subject_type, w.subject_id, w.project_id
FROM approval_stages s JOIN approval_workflows w ON w.workflow_id = s.workflow_id
WHERE w.tenant_id = $1 AND w.status = 'in_flight' AND w.current_stage = s.stage_order
  AND s.completed_at IS NULL AND sqlc.arg(user_id)::text = ANY (s.assignees)
ORDER BY s.stage_due_at;
