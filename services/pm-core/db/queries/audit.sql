-- name: LockAuditChain :exec
-- Serializes appends per tenant so seq order and the hash chain agree (transaction-scoped advisory lock).
SELECT pg_advisory_xact_lock(hashtext('audit_chain:' || $1::text));

-- name: LastAuditHash :one
SELECT signed_hash FROM audit_events WHERE tenant_id = $1 ORDER BY seq DESC LIMIT 1;

-- name: InsertAuditEvent :one
INSERT INTO audit_events (
    audit_id, tenant_id, project_id, actor_user_id, action_type, subject_type, subject_id,
    before_state, after_state, at, source_ip, request_id, guidance_id, classification_verdict,
    prev_hash, signed_hash, actor_id, chain_version
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13, $14,
    $15, $16, $17, $18
) RETURNING *;

-- name: ListAuditEventsForProject :many
SELECT * FROM audit_events WHERE tenant_id = $1 AND project_id = $2 ORDER BY seq DESC LIMIT $3;

-- name: ListAuditEventsInOrder :many
-- Chain verification walks every row in seq order.
SELECT * FROM audit_events WHERE tenant_id = $1 AND seq > $2 ORDER BY seq ASC LIMIT $3;
