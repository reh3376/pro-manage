-- +goose Up
-- ERP mirror (FSD §03 Budget "In the ERP-led profile ... mirrored from the ERP", §10 ERPi): what pm-core
-- keeps of the ERP's budget versions, actuals, and commitments, pulled through the ERPi module by
-- pm-workers, plus the one controlled financial write, the budget release on CAR approval (§08
-- "On final approval (ERP-led profile)"). The ERP owns every mirrored row; pm-core never edits them.

CREATE TABLE budget_versions (
    version_id    uuid PRIMARY KEY,
    tenant_id     text NOT NULL DEFAULT 'default',
    project_id    uuid NOT NULL REFERENCES projects (project_id),
    kind          text NOT NULL CHECK (kind IN ('baseline', 'forecast', 'eac')),
    source        text NOT NULL DEFAULT 'erpi',                 -- erpi | manual | import (manual and import arrive with the pro_manage-led profile)
    erp_instance  text NOT NULL DEFAULT '',                     -- which ERPi instance (provider code) the row came from
    version_ref   text NOT NULL,                                -- ERP id of the version (the upsert key with project and kind)
    name          text NOT NULL DEFAULT '',
    status        text NOT NULL DEFAULT 'approved',             -- draft | approved | superseded
    total         numeric(18, 2) NOT NULL,
    currency      text NOT NULL,
    as_of         timestamptz,
    approved_at   timestamptz,
    lines         jsonb NOT NULL DEFAULT '[]'::jsonb,           -- [{wbs_path, account_code, amount}]
    attributes    jsonb NOT NULL DEFAULT '{}'::jsonb,
    pulled_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, kind, version_ref)
);
CREATE INDEX budget_versions_project_idx ON budget_versions (project_id, kind, as_of DESC);

CREATE TABLE budget_actuals (
    actual_id      uuid PRIMARY KEY,
    tenant_id      text NOT NULL DEFAULT 'default',
    project_id     uuid NOT NULL REFERENCES projects (project_id),
    erp_instance   text NOT NULL DEFAULT '',
    actual_ref     text NOT NULL,                               -- ERP id (the upsert key with project)
    wbs_path       text NOT NULL DEFAULT '',
    account_code   text NOT NULL DEFAULT '',
    period         text NOT NULL DEFAULT '',                    -- YYYY-MM
    amount         numeric(18, 2) NOT NULL,
    currency       text NOT NULL,
    source_doc_ref text,
    posted_at      timestamptz,
    pulled_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, actual_ref)
);
CREATE INDEX budget_actuals_project_idx ON budget_actuals (project_id, period);

CREATE TABLE budget_commitments (
    commitment_id    uuid PRIMARY KEY,
    tenant_id        text NOT NULL DEFAULT 'default',
    project_id       uuid NOT NULL REFERENCES projects (project_id),
    erp_instance     text NOT NULL DEFAULT '',
    commitment_ref   text NOT NULL,                             -- ERP id (the upsert key with project)
    wbs_path         text NOT NULL DEFAULT '',
    account_code     text NOT NULL DEFAULT '',
    vendor_ref       text,
    po_ref           text,
    amount           numeric(18, 2) NOT NULL,
    invoiced_to_date numeric(18, 2) NOT NULL DEFAULT 0,
    currency         text NOT NULL,
    status           text NOT NULL DEFAULT 'open',              -- open | closed
    created_at       timestamptz,
    pulled_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, commitment_ref)
);
CREATE INDEX budget_commitments_project_idx ON budget_commitments (project_id, status);

-- One row per approved CAR in the ERP-led profile: requested in the approval's transaction, released
-- by pm-workers through the module (outbox kind erpi.budget.release), idempotent on car_id at the ERP.
CREATE TABLE budget_releases (
    release_id     uuid PRIMARY KEY,
    tenant_id      text NOT NULL DEFAULT 'default',
    project_id     uuid NOT NULL REFERENCES projects (project_id),
    car_id         uuid NOT NULL UNIQUE REFERENCES capital_authorizations (car_id),
    erp_instance   text NOT NULL DEFAULT '',
    project_ref    text NOT NULL,                               -- the ERP project id the release targets
    amount         numeric(18, 2) NOT NULL,
    currency       text NOT NULL,
    status         text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'released', 'failed')),
    erp_budget_ref text,
    requested_by   text NOT NULL,
    requested_at   timestamptz NOT NULL DEFAULT now(),
    audit_id       uuid REFERENCES audit_events (audit_id),     -- the approval
    released_at    timestamptz,
    attempts       integer NOT NULL DEFAULT 0,
    last_error     text,
    detail         text NOT NULL DEFAULT ''
);
CREATE INDEX budget_releases_project_idx ON budget_releases (project_id, status);

-- When the project's mirror was last pulled, so the sync loop and the UI can say how fresh it is.
ALTER TABLE projects ADD COLUMN erp_project_ref text;          -- the ERP's id for this project (defaults to code in the stub mapping)
ALTER TABLE projects ADD COLUMN erp_synced_at timestamptz;
ALTER TABLE projects ADD COLUMN erp_sync_error text;

-- +goose Down
ALTER TABLE projects DROP COLUMN IF EXISTS erp_sync_error;
ALTER TABLE projects DROP COLUMN IF EXISTS erp_synced_at;
ALTER TABLE projects DROP COLUMN IF EXISTS erp_project_ref;
DROP TABLE IF EXISTS budget_releases;
DROP TABLE IF EXISTS budget_commitments;
DROP TABLE IF EXISTS budget_actuals;
DROP TABLE IF EXISTS budget_versions;
