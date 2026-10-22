-- name: EnqueueOutbox :one
-- Idempotent on (kind, source_ref): re-enqueueing the same audit event is a no-op.
INSERT INTO outbox (outbox_id, tenant_id, kind, payload, source_ref)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (kind, source_ref) DO NOTHING
RETURNING *;

-- name: ClaimOutboxBatch :many
-- Claims due pending rows for one worker; SKIP LOCKED lets several workers drain concurrently.
WITH due AS (
    SELECT outbox_id FROM outbox
    WHERE status = 'pending' AND next_attempt_at <= $1
      AND (locked_at IS NULL OR locked_at < $1 - interval '10 minutes')
    ORDER BY next_attempt_at
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
UPDATE outbox o SET locked_by = $3, locked_at = $1
FROM due WHERE o.outbox_id = due.outbox_id
RETURNING o.*;

-- name: MarkOutboxDelivered :exec
UPDATE outbox SET status = 'delivered', delivered_at = $2, locked_by = NULL, locked_at = NULL, last_error = NULL
WHERE outbox_id = $1;

-- name: MarkOutboxRetry :exec
UPDATE outbox SET attempts = attempts + 1, next_attempt_at = $2, last_error = $3, locked_by = NULL, locked_at = NULL
WHERE outbox_id = $1;

-- name: MarkOutboxFailed :exec
UPDATE outbox SET status = 'failed', attempts = attempts + 1, last_error = $2, locked_by = NULL, locked_at = NULL
WHERE outbox_id = $1;

-- name: OutboxStats :many
SELECT kind, status, count(*)::bigint AS n, min(next_attempt_at)::timestamptz AS oldest_due
FROM outbox WHERE tenant_id = $1
GROUP BY kind, status ORDER BY kind, status;

-- name: GetOutbox :one
SELECT * FROM outbox WHERE outbox_id = $1;

-- name: RequeueOutboxFailed :execrows
-- Operator action (admin surface later): put dead-lettered rows back in play.
UPDATE outbox SET status = 'pending', next_attempt_at = now(), attempts = 0, last_error = NULL
WHERE tenant_id = $1 AND status = 'failed' AND (sqlc.narg('kind')::text IS NULL OR kind = sqlc.narg('kind')::text);
