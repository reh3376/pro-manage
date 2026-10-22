-- +goose Up
-- Postgres outbox (FSD §05 DF-01, §06 OBSERVE/FEEDBACK "Delivery"): every state-changing
-- transaction that must reach MDEMG (or Commsi, later) writes a row here in the same
-- transaction as its mutation and AuditEvent. pm-workers drains it with at-least-once
-- delivery and exponential backoff; a user's request is never coupled to MDEMG availability.

CREATE TABLE outbox (
    outbox_id        uuid PRIMARY KEY,
    tenant_id        text NOT NULL DEFAULT 'default',
    kind             text NOT NULL,                       -- mdemg.observe | mdemg.feedback | commsi.notify (later)
    payload          jsonb NOT NULL,
    source_ref       text NOT NULL,                       -- audit_id (or other natural key) for idempotent delivery
    status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivered', 'failed')),
    attempts         integer NOT NULL DEFAULT 0,
    next_attempt_at  timestamptz NOT NULL DEFAULT now(),
    last_error       text,
    locked_by        text,
    locked_at        timestamptz,
    delivered_at     timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (kind, source_ref)
);
CREATE INDEX outbox_pending_idx ON outbox (status, next_attempt_at) WHERE status = 'pending';

-- +goose Down
DROP TABLE IF EXISTS outbox;
