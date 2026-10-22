-- +goose Up
-- The spine's governed action: CapitalAuthorization (FSD §03), the ApprovalWorkflow state
-- machine with stages (§08), GuidanceOutcome (§06 audit coupling), and the approver pools the
-- stages resolve against until IdPi claims mapping replaces them (§04).

CREATE TABLE estimate_artifacts (
    estimate_id               uuid PRIMARY KEY,
    tenant_id                 text NOT NULL DEFAULT 'default',
    project_id                uuid NOT NULL REFERENCES projects (project_id),
    aace_class                smallint NOT NULL CHECK (aace_class BETWEEN 1 AND 5),
    as_of                     date NOT NULL,
    prepared_by               text NOT NULL,
    reviewed_by               text,
    basis_of_estimate_doc_ref text,
    scope_definition_pct      numeric(5, 2),
    accuracy_range_low_pct    numeric(6, 2),
    accuracy_range_high_pct   numeric(6, 2),
    tic                       numeric(18, 2) NOT NULL,
    contingency_amount        numeric(18, 2) NOT NULL DEFAULT 0,
    contingency_method        text,
    currency                  text NOT NULL,
    created_at                timestamptz NOT NULL DEFAULT now(),
    created_by                text NOT NULL
);
CREATE INDEX estimate_artifacts_project_idx ON estimate_artifacts (project_id, as_of DESC);

-- One measurable commitment in a CAR's business case (§03 BusinessCasePromise). A `full` CAR cannot
-- be submitted until every promise on the project is `committed` with a baseline and a measurement basis.
CREATE TABLE business_case_promises (
    promise_id            uuid PRIMARY KEY,
    tenant_id             text NOT NULL DEFAULT 'default',
    project_id            uuid NOT NULL REFERENCES projects (project_id),
    kind                  text NOT NULL CHECK (kind IN ('loss_recovery', 'capacity', 'cost_per_unit', 'energy_per_unit', 'quality', 'safety', 'compliance', 'other')),
    metric_id             text NOT NULL DEFAULT '',
    loss_category_id      text,
    baseline_value        numeric(18, 4),
    baseline_period       text,
    promised_value        numeric(18, 4),
    promised_by_date      date,
    dollar_value_per_year numeric(18, 2),
    measurement_basis     text NOT NULL DEFAULT '',
    status                text NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'committed', 'realized', 'partially_realized', 'not_realized')),
    created_at            timestamptz NOT NULL DEFAULT now(),
    created_by            text NOT NULL,
    updated_at            timestamptz NOT NULL DEFAULT now(),
    updated_by            text NOT NULL,
    version               integer NOT NULL DEFAULT 1
);
CREATE INDEX business_case_promises_project_idx ON business_case_promises (project_id);

CREATE TABLE capital_authorizations (
    car_id                   uuid PRIMARY KEY,
    tenant_id                text NOT NULL DEFAULT 'default',
    project_id               uuid NOT NULL REFERENCES projects (project_id),
    code                     text NOT NULL,                       -- e.g. CR-2457
    kind                     text NOT NULL CHECK (kind IN ('fel_funding', 'full', 'supplemental')),
    amount_requested         numeric(18, 2) NOT NULL CHECK (amount_requested >= 0),
    currency                 text NOT NULL,
    basis                    text NOT NULL DEFAULT 'tic' CHECK (basis IN ('tic', 'tic_plus_contingency', 'total_project_cost')),
    estimate_id              uuid REFERENCES estimate_artifacts (estimate_id),
    fel_stage_at_submission  text,
    business_case            text NOT NULL DEFAULT '',
    supersedes_car_id        uuid REFERENCES capital_authorizations (car_id),
    status                   text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'in_flight', 'approved', 'rejected', 'cancelled')),
    workflow_id              uuid,
    authorized_amount        numeric(18, 2),
    authorized_at            timestamptz,
    erp_budget_ref           text,
    signed_artifact_ref      text,
    created_at               timestamptz NOT NULL DEFAULT now(),
    created_by               text NOT NULL,
    updated_at               timestamptz NOT NULL DEFAULT now(),
    updated_by               text NOT NULL,
    version                  integer NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, code)
);
CREATE INDEX cars_project_idx ON capital_authorizations (project_id, created_at DESC);

CREATE TABLE approval_workflows (
    workflow_id      uuid PRIMARY KEY,
    tenant_id        text NOT NULL DEFAULT 'default',
    project_id       uuid REFERENCES projects (project_id),
    kind             text NOT NULL,                               -- car | baseline_change | ... (§03)
    subject_type     text NOT NULL,
    subject_id       text NOT NULL,
    initiated_by     text NOT NULL,
    initiated_at     timestamptz NOT NULL DEFAULT now(),
    current_stage    integer NOT NULL DEFAULT 1,
    status           text NOT NULL DEFAULT 'in_flight' CHECK (status IN ('in_flight', 'approved', 'rejected', 'cancelled', 'expired')),
    policy_snapshot  jsonb NOT NULL,                              -- effective policy values at start (§08)
    completed_at     timestamptz,
    version          integer NOT NULL DEFAULT 1
);
CREATE INDEX approval_workflows_subject_idx ON approval_workflows (subject_type, subject_id);

CREATE TABLE approval_stages (
    stage_id                 uuid PRIMARY KEY,
    workflow_id              uuid NOT NULL REFERENCES approval_workflows (workflow_id),
    stage_order              integer NOT NULL,
    role_required            text NOT NULL,
    assignees                text[] NOT NULL DEFAULT '{}',        -- resolved when the stage opens
    sla_minutes              integer NOT NULL,                    -- SLA from the policy snapshot (sla.car.<stage>), wall-clock in the toy; business days later
    entered_at               timestamptz,
    stage_due_at             timestamptz,
    completed_at             timestamptz,
    completed_by             text,
    decision                 text CHECK (decision IN ('approve', 'reject', 'delegate')),
    comment                  text,
    guidance_id              text,
    classification_verdict   text,
    fail_tier                text CHECK (fail_tier IN ('A', 'B', 'C')),
    UNIQUE (workflow_id, stage_order)
);

-- What the user saw and did at a governed action (§06 "Audit coupling"), written in the same
-- transaction as the AuditEvent and self-contained even if MDEMG later retires the constraint.
CREATE TABLE guidance_outcomes (
    outcome_id              uuid PRIMARY KEY,
    tenant_id               text NOT NULL DEFAULT 'default',
    audit_id                uuid NOT NULL REFERENCES audit_events (audit_id),
    project_id              uuid,
    user_id                 text NOT NULL,
    action_type             text NOT NULL,
    guidance_ids            text[] NOT NULL DEFAULT '{}',
    guidance_items_snapshot jsonb NOT NULL,
    classification_verdict  text,
    violated_codes          text[] NOT NULL DEFAULT '{}',
    fail_tier               text NOT NULL CHECK (fail_tier IN ('A', 'B', 'C')),
    user_response           text NOT NULL CHECK (user_response IN ('followed', 'ignored_with_reason', 'dismissed', 'blocked', 'overridden_by_admin', 'mdemg_unavailable')),
    dismiss_reason          text,
    override_admin_id       text,
    at                      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX guidance_outcomes_project_idx ON guidance_outcomes (project_id, at DESC);

-- Approver pools (§04 workflow roles): who may decide a stage. Dev seed fills them; IdPi claims
-- mapping (groups -> pools) replaces the manual rows.
CREATE TABLE approver_pools (
    pool       text NOT NULL,                                     -- finance_approver_eligible | vp_finance | cfo | ceo | ...
    user_id    text NOT NULL,
    site_id    uuid,                                              -- optional site scoping (finance_approver ∩ site)
    added_by   text NOT NULL,
    added_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (pool, user_id)
);

-- +goose Down
DROP TABLE IF EXISTS approver_pools;
DROP TABLE IF EXISTS guidance_outcomes;
DROP TABLE IF EXISTS approval_stages;
DROP TABLE IF EXISTS approval_workflows;
DROP TABLE IF EXISTS capital_authorizations;
DROP TABLE IF EXISTS business_case_promises;
DROP TABLE IF EXISTS estimate_artifacts;
