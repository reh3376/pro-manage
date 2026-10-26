-- name: ListOpenStagesForTick :many
-- Every open stage of an in-flight workflow, with what the tick needs from the workflow.
SELECT s.*, w.kind AS workflow_kind, w.subject_type, w.subject_id, w.project_id, w.policy_snapshot, w.initiated_by
FROM approval_stages s JOIN approval_workflows w ON w.workflow_id = s.workflow_id
WHERE w.tenant_id = $1 AND w.status = 'in_flight' AND w.current_stage = s.stage_order AND s.completed_at IS NULL AND s.entered_at IS NOT NULL
ORDER BY s.stage_due_at
LIMIT $2;

-- name: MarkStageNudged :execrows
UPDATE approval_stages SET nudged_at = $2 WHERE stage_id = $1 AND nudged_at IS NULL;

-- name: MarkStageOverdue :execrows
UPDATE approval_stages SET overdue_at = $2, nudged_at = COALESCE(nudged_at, $2) WHERE stage_id = $1 AND overdue_at IS NULL;

-- name: MarkStageEscalated :execrows
UPDATE approval_stages SET escalated_at = $2, stage_due_at = $3 WHERE stage_id = $1 AND escalated_at IS NULL;

-- name: ExpireWorkflow :execrows
UPDATE approval_workflows SET status = 'expired', completed_at = now(), version = version + 1
WHERE tenant_id = $1 AND workflow_id = $2 AND status = 'in_flight';

-- name: SetCARExpired :one
UPDATE capital_authorizations SET status = 'expired', updated_at = now(), updated_by = $3, version = version + 1
WHERE tenant_id = $1 AND car_id = $2 AND status = 'in_flight'
RETURNING *;
