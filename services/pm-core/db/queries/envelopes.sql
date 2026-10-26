-- name: RequestEnvelope :one
INSERT INTO signature_envelopes (envelope_id, tenant_id, project_id, subject_type, subject_id, provider, title, signers, requested_by, audit_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (subject_type, subject_id, audit_id) DO NOTHING
RETURNING *;

-- name: GetEnvelope :one
SELECT * FROM signature_envelopes WHERE tenant_id = $1 AND envelope_id = $2;

-- name: GetEnvelopeByAudit :one
SELECT * FROM signature_envelopes WHERE subject_type = $1 AND subject_id = $2 AND audit_id = $3;

-- name: LatestEnvelopeForSubject :one
SELECT * FROM signature_envelopes WHERE tenant_id = $1 AND subject_type = $2 AND subject_id = $3 ORDER BY requested_at DESC LIMIT 1;

-- name: MarkEnvelopeCreated :exec
UPDATE signature_envelopes SET provider_ref = $2, status = $3, signers = $4, sent_at = now(), last_error = NULL WHERE envelope_id = $1;

-- name: MarkEnvelopeFailed :exec
UPDATE signature_envelopes SET status = 'failed', last_error = $2 WHERE envelope_id = $1;

-- name: UpdateEnvelopeStatus :exec
UPDATE signature_envelopes SET status = $2, signers = $3, detail = $4, last_checked_at = now(), last_error = NULL WHERE envelope_id = $1;

-- name: MarkEnvelopeCheckError :exec
UPDATE signature_envelopes SET last_checked_at = now(), last_error = $2 WHERE envelope_id = $1;

-- name: MarkEnvelopeCompleted :one
UPDATE signature_envelopes
SET status = 'completed', signers = $2, completed_at = $3, artifact_ref = $4, artifact_sha256 = $5, artifact_bytes = $6, last_checked_at = now(), last_error = NULL
WHERE envelope_id = $1 AND status <> 'completed'
RETURNING *;

-- name: ListOpenEnvelopes :many
-- Reconciliation (§10 "lost webhooks"): every envelope still waiting on the provider.
SELECT * FROM signature_envelopes WHERE status IN ('created', 'sent') ORDER BY requested_at LIMIT $1;

-- name: SetCARSignedArtifact :exec
UPDATE capital_authorizations SET signed_artifact_ref = $3, updated_at = now(), updated_by = $4, version = version + 1
WHERE tenant_id = $1 AND car_id = $2;
