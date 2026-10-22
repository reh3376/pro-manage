-- +goose Up
-- Spine core tables (FSD §03): Site, Project, PolicyVariable / PolicyValue, AuditEvent.
-- Conventions (FSD §03 "Identifiers and versioning"): UUIDv7 primary keys generated in Go,
-- human-readable codes unique per tenant, every mutable row carries audit stamps and a
-- monotonic version for optimistic concurrency. tenant_id is always the same value in a
-- single-company install and is reserved for defense in depth.

CREATE TABLE sites (
    site_id               uuid PRIMARY KEY,
    tenant_id             text NOT NULL DEFAULT 'default',
    code                  text NOT NULL,
    name                  text NOT NULL,
    portfolio_id          uuid,
    country               text NOT NULL,                 -- ISO 3166-1 alpha-2
    time_zone             text NOT NULL,                 -- IANA
    currency              text NOT NULL,                 -- ISO 4217
    unit_system           text NOT NULL DEFAULT 'imperial' CHECK (unit_system IN ('imperial', 'metric')),
    business_calendar_id  text,
    regulatory_regime     text[] NOT NULL DEFAULT '{}',
    privacy_regime        text[] NOT NULL DEFAULT '{}',
    address               jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at            timestamptz NOT NULL DEFAULT now(),
    created_by            text NOT NULL,
    updated_at            timestamptz NOT NULL DEFAULT now(),
    updated_by            text NOT NULL,
    version               integer NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, code)
);

CREATE TABLE projects (
    project_id                     uuid PRIMARY KEY,
    tenant_id                      text NOT NULL DEFAULT 'default',
    code                           text NOT NULL,
    name                           text NOT NULL,
    description                    text NOT NULL DEFAULT '',
    portfolio_id                   uuid,
    site_id                        uuid NOT NULL REFERENCES sites (site_id),
    currency                       text NOT NULL,
    owner_user_id                  text NOT NULL,
    lifecycle_state                text NOT NULL DEFAULT 'initiate'
        CHECK (lifecycle_state IN ('initiate', 'plan', 'execute', 'monitor', 'close', 'archived')),
    fel_stage                      text NOT NULL DEFAULT 'fel1'
        CHECK (fel_stage IN ('fel1', 'fel2', 'fel3', 'n/a')),
    project_type                   text NOT NULL
        CHECK (project_type IN ('greenfield', 'brownfield_expansion', 'reliability', 'efficiency', 'compliance', 'capacity_debottleneck')),
    tpm_pillar_tags                text[] NOT NULL DEFAULT '{}',
    hazardous_area_classification  text NOT NULL DEFAULT 'none'
        CHECK (hazardous_area_classification IN ('class_i_div_1', 'class_i_div_2', 'class_ii_div_1', 'class_ii_div_2', 'none')),
    regulatory_stamps              text[] NOT NULL DEFAULT '{}',
    onboarding_level               text NOT NULL DEFAULT 'registered'
        CHECK (onboarding_level IN ('registered', 'governed', 'full')),
    imported                       boolean NOT NULL DEFAULT false,
    import_batch_id                text,
    start_date                     date,
    target_end_date                date,
    baseline_budget_total          numeric(18, 2),
    baseline_duration_days         integer,
    tags                           text[] NOT NULL DEFAULT '{}',
    created_at                     timestamptz NOT NULL DEFAULT now(),
    created_by                     text NOT NULL,
    updated_at                     timestamptz NOT NULL DEFAULT now(),
    updated_by                     text NOT NULL,
    version                        integer NOT NULL DEFAULT 1,
    UNIQUE (tenant_id, code)
);
CREATE INDEX projects_site_idx ON projects (site_id);
CREATE INDEX projects_owner_idx ON projects (owner_user_id);

-- Policy variables (FSD §08): admin-configurable thresholds, SLAs, cadences, windows.
CREATE TABLE policy_variables (
    key             text PRIMARY KEY,                     -- e.g. car.threshold.vp_finance
    type            text NOT NULL CHECK (type IN ('money', 'percent', 'duration', 'integer', 'boolean', 'enum', 'decimal', 'date', 'json')),
    unit            text,                                 -- currency code, 'pct', 'days', ...
    description     text NOT NULL,
    global_default  jsonb NOT NULL,
    allowed_range   jsonb,                                -- {"min":..,"max":..} or {"values":[..]}
    overridable_at  text[] NOT NULL DEFAULT '{project_type,portfolio,site,project}',
    owner_role      text NOT NULL DEFAULT 'policy_owner',
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      text NOT NULL,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      text NOT NULL,
    version         integer NOT NULL DEFAULT 1
);

CREATE TABLE policy_values (
    value_id        uuid PRIMARY KEY,
    variable_key    text NOT NULL REFERENCES policy_variables (key),
    scope_kind      text NOT NULL CHECK (scope_kind IN ('global', 'project_type', 'portfolio', 'site', 'project')),
    scope_id        text NOT NULL DEFAULT '',             -- '' for global; project_type name, portfolio/site/project id otherwise
    value           jsonb NOT NULL,
    effective_from  timestamptz NOT NULL DEFAULT now(),
    effective_to    timestamptz,
    set_by          text NOT NULL,
    reason          text NOT NULL DEFAULT '',
    approved_by     text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    version         integer NOT NULL DEFAULT 1,
    CHECK ((scope_kind = 'global') = (scope_id = ''))
);
CREATE INDEX policy_values_lookup_idx ON policy_values (variable_key, scope_kind, scope_id, effective_from DESC);

-- Audit events (FSD §08 "Audit log"): append-only, chained hash per tenant.
-- signed_hash = HMAC-SHA256(prev_hash || canonical(row), audit_chain_key), computed in pm-core
-- under a per-tenant advisory lock so seq order and the chain agree.
CREATE TABLE audit_events (
    audit_id                 uuid PRIMARY KEY,
    seq                      bigserial NOT NULL UNIQUE,
    tenant_id                text NOT NULL DEFAULT 'default',
    project_id               uuid,
    actor_user_id            text NOT NULL,
    action_type              text NOT NULL,               -- e.g. site.create, project.create, import.car
    subject_type             text NOT NULL,
    subject_id               text NOT NULL,
    before_state             jsonb,
    after_state              jsonb,
    at                       timestamptz NOT NULL DEFAULT now(),
    source_ip                text,
    request_id               text,
    guidance_id              text,
    classification_verdict   text,
    prev_hash                text NOT NULL,
    signed_hash              text NOT NULL
);
CREATE INDEX audit_events_project_idx ON audit_events (project_id, at DESC);
CREATE INDEX audit_events_subject_idx ON audit_events (subject_type, subject_id);

-- Immutability: no row is ever updated or deleted through the application.
-- The INSERT-only database role for the audit writer (FSD §08) is created by the
-- deployment, not by a migration, because role management needs instance privileges;
-- the triggers below hold regardless of role.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION audit_events_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only (%)', TG_OP;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER audit_events_no_update BEFORE UPDATE ON audit_events FOR EACH ROW EXECUTE FUNCTION audit_events_immutable();
CREATE TRIGGER audit_events_no_delete BEFORE DELETE ON audit_events FOR EACH ROW EXECUTE FUNCTION audit_events_immutable();

-- +goose Down
DROP TRIGGER IF EXISTS audit_events_no_delete ON audit_events;
DROP TRIGGER IF EXISTS audit_events_no_update ON audit_events;
DROP FUNCTION IF EXISTS audit_events_immutable();
DROP TABLE IF EXISTS audit_events;
DROP TABLE IF EXISTS policy_values;
DROP TABLE IF EXISTS policy_variables;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS sites;
