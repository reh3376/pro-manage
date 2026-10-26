-- +goose Up
-- E-signature envelopes (FSD §10 Sigi, §08 "signed artifact stored via Sigi if applicable").
-- pm-core keeps the record; the provider (through the Sigi module) keeps the envelope; the
-- signed artifact is captured to the object store and hashed into the audit log.
CREATE TABLE signature_envelopes (
    envelope_id      uuid PRIMARY KEY,
    tenant_id        text NOT NULL DEFAULT 'default',
    project_id       uuid REFERENCES projects (project_id),
    subject_type     text NOT NULL,                        -- capital_authorization | change_request | closeout | ...
    subject_id       text NOT NULL,
    provider         text NOT NULL,                        -- stub | docusign | adobe_sign | onespan
    provider_ref     text,                                 -- the provider's envelope id, once created
    status           text NOT NULL DEFAULT 'requested' CHECK (status IN ('requested', 'created', 'sent', 'completed', 'declined', 'voided', 'expired', 'failed')),
    title            text NOT NULL,
    signers          jsonb NOT NULL DEFAULT '[]'::jsonb,   -- [{user_id, email, name, order, role, status, signed_at}]
    requested_by     text NOT NULL,
    requested_at     timestamptz NOT NULL DEFAULT now(),
    audit_id         uuid REFERENCES audit_events (audit_id),  -- the approval that required the signature (also the idempotency key)
    sent_at          timestamptz,
    completed_at     timestamptz,
    artifact_ref     text,                                 -- object-store key of the signed artifact
    artifact_sha256  text,
    artifact_bytes   bigint,
    last_checked_at  timestamptz,
    last_error       text,
    detail           text NOT NULL DEFAULT '',
    UNIQUE (subject_type, subject_id, audit_id)
);
CREATE INDEX signature_envelopes_open_idx ON signature_envelopes (status) WHERE status IN ('requested', 'created', 'sent');
CREATE INDEX signature_envelopes_subject_idx ON signature_envelopes (subject_type, subject_id);

-- +goose Down
DROP TABLE IF EXISTS signature_envelopes;
