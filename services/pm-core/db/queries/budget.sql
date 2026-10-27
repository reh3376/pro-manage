-- ERP mirror and budget release (FSD §03 Budget, §08 "On final approval (ERP-led profile)", §10 ERPi).

-- name: UpsertBudgetVersion :exec
INSERT INTO budget_versions (version_id, tenant_id, project_id, kind, source, erp_instance, version_ref, name, status, total, currency, as_of, approved_at, lines, attributes)
VALUES ($1, $2, $3, $4, 'erpi', $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (project_id, kind, version_ref) DO UPDATE
SET name = EXCLUDED.name, status = EXCLUDED.status, total = EXCLUDED.total, currency = EXCLUDED.currency, as_of = EXCLUDED.as_of,
    approved_at = EXCLUDED.approved_at, lines = EXCLUDED.lines, attributes = EXCLUDED.attributes, erp_instance = EXCLUDED.erp_instance, pulled_at = now();

-- name: ListBudgetVersions :many
SELECT * FROM budget_versions WHERE tenant_id = $1 AND project_id = $2 ORDER BY kind, as_of DESC NULLS LAST, pulled_at DESC;

-- name: UpsertBudgetActual :exec
INSERT INTO budget_actuals (actual_id, tenant_id, project_id, erp_instance, actual_ref, wbs_path, account_code, period, amount, currency, source_doc_ref, posted_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (project_id, actual_ref) DO UPDATE
SET wbs_path = EXCLUDED.wbs_path, account_code = EXCLUDED.account_code, period = EXCLUDED.period, amount = EXCLUDED.amount, currency = EXCLUDED.currency,
    source_doc_ref = EXCLUDED.source_doc_ref, posted_at = EXCLUDED.posted_at, erp_instance = EXCLUDED.erp_instance, pulled_at = now();

-- name: ListBudgetActuals :many
SELECT * FROM budget_actuals WHERE tenant_id = $1 AND project_id = $2 ORDER BY period DESC, posted_at DESC NULLS LAST LIMIT $3;

-- name: SumBudgetActuals :one
SELECT COALESCE(SUM(amount), 0)::numeric(18, 2) AS total, COUNT(*) AS rows FROM budget_actuals WHERE tenant_id = $1 AND project_id = $2;

-- name: UpsertBudgetCommitment :exec
INSERT INTO budget_commitments (commitment_id, tenant_id, project_id, erp_instance, commitment_ref, wbs_path, account_code, vendor_ref, po_ref, amount, invoiced_to_date, currency, status, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (project_id, commitment_ref) DO UPDATE
SET wbs_path = EXCLUDED.wbs_path, account_code = EXCLUDED.account_code, vendor_ref = EXCLUDED.vendor_ref, po_ref = EXCLUDED.po_ref, amount = EXCLUDED.amount,
    invoiced_to_date = EXCLUDED.invoiced_to_date, currency = EXCLUDED.currency, status = EXCLUDED.status, created_at = EXCLUDED.created_at, erp_instance = EXCLUDED.erp_instance, pulled_at = now();

-- name: ListBudgetCommitments :many
SELECT * FROM budget_commitments WHERE tenant_id = $1 AND project_id = $2 ORDER BY status, created_at DESC NULLS LAST LIMIT $3;

-- name: SumBudgetCommitments :one
SELECT COALESCE(SUM(amount) FILTER (WHERE status = 'open'), 0)::numeric(18, 2) AS open_total,
       COALESCE(SUM(invoiced_to_date), 0)::numeric(18, 2) AS invoiced_total,
       COUNT(*) AS rows
FROM budget_commitments WHERE tenant_id = $1 AND project_id = $2;

-- name: RequestBudgetRelease :one
INSERT INTO budget_releases (release_id, tenant_id, project_id, car_id, erp_instance, project_ref, amount, currency, requested_by, audit_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (car_id) DO NOTHING
RETURNING *;

-- name: GetBudgetRelease :one
SELECT * FROM budget_releases WHERE tenant_id = $1 AND release_id = $2;

-- name: GetBudgetReleaseForCAR :one
SELECT * FROM budget_releases WHERE car_id = $1;

-- name: ListBudgetReleasesForProject :many
SELECT * FROM budget_releases WHERE tenant_id = $1 AND project_id = $2 ORDER BY requested_at DESC;

-- name: ListPendingBudgetReleases :many
SELECT * FROM budget_releases WHERE status = 'pending' ORDER BY requested_at LIMIT $1;

-- name: MarkBudgetReleased :one
UPDATE budget_releases
SET status = 'released', erp_budget_ref = $2, released_at = $3, detail = $4, attempts = attempts + 1, last_error = NULL
WHERE release_id = $1 AND status <> 'released'
RETURNING *;

-- name: MarkBudgetReleaseAttempt :exec
UPDATE budget_releases SET attempts = attempts + 1, last_error = $2 WHERE release_id = $1;

-- name: MarkBudgetReleaseFailed :exec
UPDATE budget_releases SET status = 'failed', attempts = attempts + 1, last_error = $2 WHERE release_id = $1;

-- name: SetCARERPBudgetRef :exec
UPDATE capital_authorizations SET erp_budget_ref = $3, updated_at = now(), updated_by = $4, version = version + 1
WHERE tenant_id = $1 AND car_id = $2;

-- name: SetProjectERPRef :exec
UPDATE projects SET erp_project_ref = $3, updated_at = now(), updated_by = $4, version = version + 1
WHERE tenant_id = $1 AND project_id = $2;

-- name: MarkProjectERPSynced :exec
UPDATE projects SET erp_synced_at = now(), erp_sync_error = NULL WHERE tenant_id = $1 AND project_id = $2;

-- name: MarkProjectERPSyncError :exec
UPDATE projects SET erp_sync_error = $3 WHERE tenant_id = $1 AND project_id = $2;

-- name: ListProjectsForERPSync :many
-- Every project the mirror follows: those with an ERP reference, least recently synced first.
SELECT * FROM projects WHERE tenant_id = $1 AND erp_project_ref IS NOT NULL ORDER BY erp_synced_at NULLS FIRST LIMIT $2;
