-- name: ListPolicyVariables :many
SELECT * FROM policy_variables ORDER BY key;

-- name: GetPolicyVariable :one
SELECT * FROM policy_variables WHERE key = $1;

-- name: UpsertPolicyVariable :one
INSERT INTO policy_variables (key, type, unit, description, global_default, allowed_range, overridable_at, owner_role, created_by, updated_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
ON CONFLICT (key) DO UPDATE SET
    type = EXCLUDED.type, unit = EXCLUDED.unit, description = EXCLUDED.description,
    global_default = EXCLUDED.global_default, allowed_range = EXCLUDED.allowed_range,
    overridable_at = EXCLUDED.overridable_at, owner_role = EXCLUDED.owner_role,
    updated_at = now(), updated_by = EXCLUDED.updated_by, version = policy_variables.version + 1
RETURNING *;

-- name: InsertPolicyValue :one
INSERT INTO policy_values (value_id, variable_key, scope_kind, scope_id, value, effective_from, effective_to, set_by, reason, approved_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: ListEffectivePolicyValues :many
-- All values of one variable that are in force now, across every scope; the resolver picks the most specific.
SELECT * FROM policy_values
WHERE variable_key = $1
  AND effective_from <= $2
  AND (effective_to IS NULL OR effective_to > $2)
ORDER BY effective_from DESC;
