-- +goose Up
-- The SLA tick (FSD §08 "SLA, nudges, auto-escalation"): what pm-workers has already done to a stage,
-- so each step happens once, and the expired outcome for a CAR whose chain ran out of time.
ALTER TABLE approval_stages
    ADD COLUMN nudged_at    timestamptz,   -- 75% of the SLA elapsed: nudge sent
    ADD COLUMN overdue_at   timestamptz,   -- 100%: final nudge sent, workflow flagged overdue
    ADD COLUMN escalated_at timestamptz;   -- overdue grace passed: escalated once (the due date was extended by the grace)

ALTER TABLE capital_authorizations DROP CONSTRAINT capital_authorizations_status_check;
ALTER TABLE capital_authorizations ADD CONSTRAINT capital_authorizations_status_check
    CHECK (status IN ('draft', 'in_flight', 'approved', 'rejected', 'cancelled', 'expired'));

-- +goose Down
ALTER TABLE capital_authorizations DROP CONSTRAINT capital_authorizations_status_check;
ALTER TABLE capital_authorizations ADD CONSTRAINT capital_authorizations_status_check
    CHECK (status IN ('draft', 'in_flight', 'approved', 'rejected', 'cancelled'));
ALTER TABLE approval_stages DROP COLUMN nudged_at, DROP COLUMN overdue_at, DROP COLUMN escalated_at;
