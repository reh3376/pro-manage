# 03 — Domain Model

## Decisions on record

- **2026-10-13** — The **Project** is the central aggregate. Every data entity lives inside exactly one Project (or inside the Portfolio, which is above projects).
- **2026-10-13** — Project lifecycle states map **one-to-one to the six EEM stages** and are: `Initiate → Plan → Execute → Monitor → Close → Archived`. Movement between states is a controlled transition (`§08`) with distinct permissions and explicit **gates and exit criteria** per stage.
- **2026-10-13** — Projects are typed (`greenfield | brownfield_expansion | reliability | efficiency | compliance | capacity_debottleneck`), carry **TPM-pillar tags** and **FEL stage** attributes, and record **hazardous-area classification** and **regulatory stamps** (NFPA 30 / 70E / 652/61, TTB, FDA, EPA, etc.).
- **2026-10-13** — The **AACE Estimate Class** (5 → 1) is a required attribute of every BudgetBaseline. Workflow gates in `§08` enforce class progression (Class 5 at concept, Class 3 at CAR submission, Class 1 at execution).
- **2026-10-13** — Budget is modeled with **TIC** (Total Installed Cost) and **LCC** (Lifecycle Cost) as separate sub-entities. TIC is tracked throughout the project; LCC is estimated during Design and compared against actuals during Benefits Realization.
- **2026-10-13** — **MP (Maintenance Prevention) information**, **DesignReview**, **CommissioningRecord**, **TurnoverPackage**, and **BenefitsRealization** are first-class entities; see Entity catalog.
- **2026-10-13** — **Postgres** holds the authoritative transactional model. **pro_manage's own TimescaleDB** holds metric time series. **MDEMG's Neo4j** holds derived memory and guidance substrate — never the source of truth for pro_manage domain data. **S3-compatible object storage** (on-prem MinIO or equivalent) holds attachment binaries; `Doci` holds pointers for externally-stored documents.
- **2026-10-13** — Every mutable entity records its `created_at`, `created_by`, `updated_at`, `updated_by`, `version` (monotonic int), and `tenant_id` (always the same for a single-company install, but reserved for defense-in-depth).

## Model overview

```
                        ┌─────────────────┐
                        │    Portfolio    │  (one per BU, or one per company)
                        └────────┬────────┘
                                 │ 1..n
                                 ▼
                        ┌─────────────────┐          ┌───────────────────┐
                        │     Project     │────1..n──│  ProjectMember    │
                        │  (aggregate)    │          │  (user + ACL)     │
                        └────────┬────────┘          └─────────┬─────────┘
                                 │                              │ grants
       ┌─────────┬───────────┬───┴────┬─────────┬────────┐     │
       ▼         ▼           ▼        ▼         ▼        ▼     ▼
   Budget   Schedule   Scope/WBS  Risk/      Document  Metric  ACLGrant
     │       │ │        │         Issue       │         │       (per section,
     │ EAC   │ │        │ WBS                 │ Doci    │        per doc,
     │ actls │ │ depen- │ Deliv  Risk Issue   │ links   │        per field)
     │       │ │ dencies│ erable Chg Rqst     │         │ Metrici
     ▼       ▼ ▼        ▼                     ▼         ▼
  (ERPi    (Schedulei           (OSS +     Document  TSDB (ours)
   actuals)  import)             local)    binaries  + readings
                                            (S3)
                                 │
                                 ▼
                        ┌─────────────────────┐
                        │  ApprovalWorkflow   │─────┐
                        │  (per gate: CapEx,  │     │
                        │   baseline change,  │     │ fed by
                        │   milestone, close) │     │ constraints
                        └─────────────────────┘     ▼
                                 │             MDEMG / Jiminy
                                 ▼             (space=project_id +
                        ┌─────────────────────┐  space=org.<id>.governance)
                        │     AuditEvent      │
                        │ (immutable, SOX-grade)│
                        └─────────────────────┘
```

## Entity catalog

Each entity below is described conceptually. Detailed table schemas, indexes, and migration DDL live in a separate design doc (`docs/design/db/*`), referenced from here when drafted. Fields marked `derived` are computed from others and are not stored directly unless caching requires it.

### Portfolio
Group of projects visible as a unit to a Portfolio Executive. One company may have multiple portfolios (e.g., by business unit) or exactly one.

- **Key fields:** `portfolio_id`, `name`, `description`, `tenant_id`, standard audit stamps.
- **Relationships:** 1..n Projects.
- **Permissions:** Executives read; Platform Admin manages structure.

### Project (central aggregate)
A single capital project with one Owner and a bounded team.

- **Key fields:** `project_id`, `code` (human-readable PK, e.g. `P-INDUSTRIAL-EAST-7`), `name`, `description`, `portfolio_id`, `owner_user_id`, `lifecycle_state` (EEM-mapped), `fel_stage` (`fel1 | fel2 | fel3 | n/a` — n/a once past authorization), `project_type` (`greenfield | brownfield_expansion | reliability | efficiency | compliance | capacity_debottleneck`), `tpm_pillar_tags[]` (`eem | focused_improvement | planned_maintenance | autonomous_maintenance | quality_maintenance | …`), `hazardous_area_classification` (`class_i_div_1 | class_i_div_2 | class_ii_div_1 | class_ii_div_2 | none`), `regulatory_stamps[]` (`nfpa_30 | nfpa_70e | nfpa_652 | nfpa_61 | ttb | fda | epa | osha | msha | …`), `start_date`, `target_end_date`, `baseline_budget_total`, `baseline_duration_days`, `template_id?`, `tags[]`, standard audit stamps.
- **MDEMG binding:** every project maps to one `space_id = project_id` in MDEMG. The project's `code` is the human-readable form; the UUID `project_id` is what crosses the wire.
- **Lifecycle (EEM mapping):** `Initiate` ≈ Concept/Planning · `Plan` ≈ Design · `Execute` ≈ Fabrication → Installation → Commissioning · `Monitor` ≈ Early Management Period · `Close` ≈ Normal operations handover (Benefits Realization window begins) · `Archived`. Transitions gated by `§08`.
- **Permissions:** Owner has all; Teammates per ACL grants; Executives read within their portfolio.

### ProjectMember
A user's membership in a specific project with an assigned base role.

- **Key fields:** `project_id`, `user_id`, `base_role` ∈ `{owner, editor, viewer}`, `invited_by`, `invited_at`, `activated_at`, `expires_at?` (for time-boxed access like auditors).
- Relationships: belongs to Project; links to User (resolved via IdPi).

### ACLGrant
A fine-grained grant on top of a ProjectMember's base role. The full-ACL model lives here. See `§04` for the enforcement details and model constraints.

- **Key fields:** `grant_id`, `project_id`, `user_id`, `scope_kind` ∈ `{section, document, field}`, `scope_target` (e.g., `section:budget`, `document:doc_123`, `field:budget.salary`), `permission` ∈ `{read, write, deny}`, `granted_by`, `granted_at`, `expires_at?`.
- **Default:** Absence of grant means the base role determines access; an explicit `deny` can mask a base-role-granted access (e.g., Editor across the project, but Deny on `field:budget.salary`).

### Budget
The financial state of the project. Modeled with distinct **TIC** (Total Installed Cost — tracked throughout the project) and **LCC** (Lifecycle Cost — estimated during Design, validated during Benefits Realization).

- **Sub-entities:**
  - `BudgetBaseline` — one `current=true` row + historical rows. Each baseline has `baseline_id`, `baseline_total` (TIC), `aace_class` ∈ `{5, 4, 3, 2, 1}`, `fel_stage_at_capture`, `approved_at`, `approved_by`, `source` (`manual|import|workflow`), `change_request_id?`, `contingency_amount`, `contingency_basis`.
  - `BudgetLineItem` — WBS-aligned planned amounts. `line_id`, `wbs_path`, `account_code`, `planned`, `baseline_id`.
  - `BudgetActual` — read-only, written by **ERPi** on sync. `actual_id`, `wbs_path`, `account_code`, `period`, `amount`, `source_doc_id?`, `erpi_pulled_at`.
  - `BudgetCommitment` — POs issued, from ERPi.
  - `BudgetForecast` (EAC) — point-in-time Estimate At Completion. `forecast_id`, `as_of`, `wbs_path`, `eac_amount`, `method` (`manual|ratio|regression|…`).
  - `LCCEstimate` — Lifecycle Cost estimate. `lcc_id`, `as_of`, `period_years`, `tic` (snapshot), `opex_annual`, `maint_annual`, `energy_annual`, `end_of_life_cost`, `total_lcc`, `method_note`.
  - `ContingencyDrawdown` — running record of contingency spent vs project percent-complete. Early-warning signal (burning contingency faster than progress).
- **Derived:** `variance_pct = (actual + remaining_commit - baseline_total) / baseline_total`; `cpi = bcwp/acwp`; `spi = bcwp/bcws` (when earned-value inputs are present); `contingency_burn_ratio = contingency_used_pct / completion_pct`.
- **Permissions:** Finance Approvers always have `read` on budget sections of projects in their jurisdiction. Owner has `read` + controlled `write`. Teammate access is grant-driven. **Sensitive sub-fields** (vendor costs, bid prices, unit rates) remain deny-by-default per `§04`.

### Schedule
Time dimension of the project.

- **Sub-entities:**
  - `Milestone` — `milestone_id`, `name`, `baseline_date`, `target_date`, `actual_date?`, `status`, `dependencies[]` (other milestone_ids), `completion_pct`.
  - `ScheduleBaseline` — snapshot of all milestones at a given approved point. Mirrors `BudgetBaseline` shape.
- **Imports:** `Schedulei` imports from MS Project (.mpp via XML export), Primavera P6 (XER), Smartsheet (API). On import, mapping produces milestones + dependencies; original file is retained as attachment.
- **Mechanical rule:** cannot mark `complete` with open dependencies (UC-05). Enforced in-process, pre-MDEMG.

### WBS & Deliverables (Scope)
Work breakdown and deliverable-level tracking.

- **Sub-entities:**
  - `WBSNode` — `wbs_id`, `project_id`, `parent_wbs_id?`, `code`, `name`, `description`, `completion_pct`.
  - `Deliverable` — `deliverable_id`, `wbs_id`, `name`, `description`, `due_date?`, `accepted_at?`, `accepted_by?`, `acceptance_criteria`.
  - `ChangeRequest` — `cr_id`, `project_id`, `title`, `description`, `impact_budget_pct?`, `impact_schedule_days?`, `status` ∈ `{draft, submitted, approved, rejected, implemented}`, approval workflow ref.

### Risk / Issue
Risk register + issue log.

- `Risk` — `risk_id`, `project_id`, `title`, `description`, `likelihood` (1-5), `impact` (1-5), `score` (derived), `response_strategy` (`avoid|mitigate|transfer|accept`), `owner_user_id`, `due_date`, `status` ∈ `{open, mitigating, closed}`, `last_reviewed_at`.
- `Issue` — `issue_id`, `project_id`, `title`, `description`, `severity` ∈ `{low, med, high, critical}`, `status` ∈ `{open, in_progress, resolved}`, `assignee_user_id`, `opened_at`, `resolved_at?`.
- **Aging rules** (per Jiminy constraint corpus): risks aged >14d without `last_reviewed_at` update, issues aged >30d without resolution — surfaced in guidance.

### Document
Project-attached artifacts.

- **Modes:**
  - **Local:** `document_id`, `project_id`, `name`, `mime_type`, `size_bytes`, `storage_key` (S3), `versions[]`, `uploaded_by`, `uploaded_at`, `tags[]`.
  - **External (via Doci):** same fields + `doci_backend` (`sharepoint|drive|confluence|…`), `external_id`, `external_url`, `external_permissions_mirror` (snapshot of upstream ACL for drift detection).
- **Permissions:** standard ACL applies. For external docs, pro_manage mirrors permissions from the external store on sync and surfaces drift.

### Metric
Two layers: definitions and readings.

- `MetricDefinition` — `metric_id`, `name` (e.g., `budget_variance_pct`, `cpi`, `safety_incident_count`, `equipment_uptime_pct`), `category` ∈ `{financial, schedule, scope, safety, quality, compliance, resource, custom}`, `unit`, `aggregation` (`sum|avg|last|…`), `scope` (`project|portfolio|global`), `source` (`derived|erpi|metrici|manual`), `formula?` (for derived), `connector_mapping_id?` (for connector-sourced metrics).
- `MetricReading` — `reading_id` (composite: `metric_id + project_id + ts`), `value`, `quality` (`good|questionable|bad`), `source_ref`. Stored in **pro_manage's own TimescaleDB**.
- **Harvest cadence:** per-connector (Metrici) — ranges from seconds (plant historian) to daily (ERP actuals).

### ApprovalWorkflow
A multi-stage approval instance (`§08` details the state machine).

- `ApprovalWorkflow` — `workflow_id`, `project_id`, `kind` ∈ `{capex, baseline_change, milestone_signoff, change_request, closeout, custom}`, `subject_ref` (what's being approved: a budget baseline id, a change request id, …), `initiated_by`, `initiated_at`, `current_stage`, `status` ∈ `{in_flight, approved, rejected, cancelled, expired}`, `signed_artifact_ref?` (Sigi reference when applicable).
- `ApprovalStage` — `stage_id`, `workflow_id`, `order`, `role_required` (e.g., `vp_finance`), `assignees[]`, `sla_duration`, `completed_at?`, `completed_by?`, `decision` ∈ `{approve, reject, delegate}`, `comment?`, `guidance_id?` (MDEMG reference captured when the approver viewed guidance), `signed_envelope_ref?` (Sigi envelope ID).

### AuditEvent (immutable)
The authoritative, append-only log of state-changing actions. SOX-/ISO-grade.

- **Key fields:** `audit_id`, `tenant_id`, `project_id?`, `actor_user_id`, `action_type` (fine-grained enum), `subject_type`, `subject_id`, `before` (JSON), `after` (JSON), `at` (ts), `source_ip`, `request_id` (correlates to MDEMG request logs), `guidance_id?` (if MDEMG was consulted), `classification_verdict?` (if `/jiminy/classify` was called), `signed_hash` (chain of hashes for tamper-evidence).
- **Immutability:** append-only via application-level writes; database constraints block updates/deletes. Retention aligned to the longest applicable regime (`§11`).

### GuidanceOutcome
pro_manage's local record of MDEMG guidance presented and the user's response. Captured so pro_manage's audit log is self-contained even when MDEMG is unreachable or has retired a constraint.

- **Key fields:** `outcome_id`, `project_id`, `user_id`, `action_type`, `guidance_id` (MDEMG), `guidance_items_snapshot` (JSON of what the user saw), `user_response` ∈ `{followed, ignored_with_reason, dismissed, blocked, overridden_by_admin}`, `dismiss_reason?`, `override_admin_id?`, `at`.
- Written at the same transaction as the AuditEvent for a governed action.

### ResourceAllocation
Who or what is loaded on which project, over time.

- `Resource` — `resource_id`, `kind` ∈ `{person, equipment}`, `name`, `external_ref?` (HR system employee id, asset tag, etc.).
- `Allocation` — `allocation_id`, `resource_id`, `project_id`, `start`, `end`, `percent` (0.0–1.0 for people; usage factor for equipment).
- **Overallocation detection:** sum of `percent` across concurrent allocations > 1.0 raises a flag; surfaced in guidance and dashboards.

### Template
A re-usable starter for a Project (or a Milestone set, Risk categories, etc.).

- `ProjectTemplate` — `template_id`, `name`, `description`, `wbs_structure` (JSON), `default_milestones` (JSON), `default_risks` (JSON), `default_metric_definitions_ref[]`, `default_tpm_pillar_tags[]`, `default_regulatory_stamps[]`, `created_by`, `created_at`.
- Projects can be created from templates or converted to templates.

### DesignReview
A formal review event during Design (Plan) stage against the seven "easy-to" criteria. Operator and maintenance participation is first-class.

- **Key fields:** `review_id`, `project_id`, `scope` (free-text + optional reference to a design document), `stage` (`basic | detailed | pre_approval | post_change`), `scheduled_at`, `participants[]` (resolved to users; **must include at least one operator role and one maintenance role** for Design reviews of equipment-touching scope), `facilitator_user_id`, `status` ∈ `{planned, in_session, closed, cancelled}`.
- **Criteria:** every DesignReview carries findings against the seven "easy-to" lenses (`easy_to_operate`, `easy_to_maintain`, `easy_to_clean`, `easy_to_change_over`, `easy_to_inspect`, `easy_to_keep_safe`, `easy_to_make_quality_product`). Each lens: `rating` (1–5 or `n/a`), `findings[]`, `dispositions[]`.
- **Dispositions** carry `finding_id`, `action` (`design_change | accept | mitigate | defer`), `owner_user_id`, `due_date`, `status`, `resolution_note`, `change_request_id?` (if the disposition triggers a CR).
- **Permissions:** owner, editors with design-review write grant, operators + maintenance participants in `read+comment` mode, platform roles per `§04`.

### MPInformation
Maintenance-Prevention information — observations harvested from running assets that feed future project designs. The institutional-memory substrate of EEM.

- **Key fields:** `mp_id`, `source_project_id?` (where the observation originated — may be from a running asset not tracked in pro_manage), `authored_by_user_id`, `authored_at`, `content` (narrative), `category` ∈ `{failure, operability, maintainability, cleanability, changeover, safety, quality, energy, cost, other}`, `equipment_scope` (tags like `boiler`, `column_still`, `bottling_line`, `fermenter`, `conveyor`, `heat_exchanger`, …), `severity_at_source` (`low | med | high | critical`), `evidence_refs[]` (document / incident / metric references), `linked_mp_ids[]` (related observations).
- **MDEMG binding:** each MPInformation row is also written as an `obs_type=learning` observation into the governance space (`space=org.<id>.governance`) with `scope=equipment:<type>` tags. Jiminy's retrieval surfaces these during Design-stage guidance for similar equipment on later projects.
- **Lifecycle:** `draft → published → superseded?` Governance Authors can promote repeatedly-cited MPInformation into formal constraints (via `§06`).

### CommissioningRecord
Captures FAT, SAT, and (where applicable) formal CQV activity.

- **Key fields:** `record_id`, `project_id`, `kind` ∈ `{fat, sat, commissioning, qualification, validation}`, `equipment_ref` (what asset the record is for), `vendor` (for FAT), `location`, `scheduled_at`, `conducted_at`, `conducted_by_user_id`, `witnesses[]`, `test_protocol_ref` (document ref), `result` ∈ `{pass, pass_with_punchlist, fail, retest_scheduled}`, `punchlist_items[]`, `signed_envelope_ref?` (via Sigi where formal), `attached_evidence[]`.
- **Linked to:** scheduled Milestones (so completion of CQV becomes a milestone event) and workflow stages (CQV sign-off gate — see `§08`).

### TurnoverPackage
The structured handover artifact assembled before Early Management exit. The package Operations and Maintenance actually inherit.

- **Key fields:** `package_id`, `project_id`, `status` ∈ `{assembling, under_review, approved, delivered_to_cmms}`, `completeness_score` (0.0–1.0, computed from required-section fill), `approved_at?`, `approved_by?`, `delivered_to_cmms_at?`.
- **Required sections** (year-one defaults; template-tunable):
  - Equipment master data (nameplate, serial numbers, drawings, warranties)
  - Spares list (part numbers, suppliers, min/max, lead times, criticality)
  - PM task library (frequencies, procedures, estimated time, required skills)
  - Operating procedures (standard work)
  - Cleaning procedures (CIP recipes where applicable)
  - Calibration schedule and certificates
  - LOTO procedures
  - Emergency procedures
  - As-built P&IDs, electrical drawings, control-system docs
  - Training materials
- **CMMS delivery:** on `approved`, the package is pushed to the plant CMMS via **CMMSi** — equipment hierarchy, spares, PM tasks, procedures created/updated. Delivery is idempotent; retries are safe. Delivery status is logged and the audit trail links the CMMS record IDs back to the pro_manage package.

### BenefitsRealization
Tracks whether the business case promise actually materialized post-startup. The third ROI calculation of a project.

- **Key fields:** `br_id`, `project_id`, `window_opened_at` (defaults to project `Close` date), `window_duration_days` (default 180–365, policy-configurable), `window_closes_at`, `status` ∈ `{open, in_progress, closed_realized, closed_partial, closed_not_realized}`, `business_case_promises[]` (what the CAR committed to deliver — reductions in EPU, increases in capacity, OEE targets, CPU targets, …), `measurements[]` (periodic snapshots pulled from Metrici/HSEi during the window), `final_variance_report` (narrative + numeric: promised vs delivered, variance explanation), `signed_envelope_ref?` (if final review is formally signed via Sigi).
- **Workflow (`§08`):** BenefitsRealization carries its own closeout workflow. The project does not move to `Archived` until BenefitsRealization is closed (except when the window itself is explicitly waived with admin override + audited reason).
- **MDEMG binding:** at closeout of BenefitsRealization, pro_manage writes a `decision` observation to the governance space summarizing what was realized vs promised, with `scope` tags that let similar future projects surface it during their own business-case drafting.

## Multi-store map

| Store | Lives in | Holds | Written by | Read by |
|---|---|---|---|---|
| **Postgres** (pro_manage) | Docker | All transactional domain (`Project`, members, ACLs, budgets, schedule, scope, risk, docs metadata, workflows, templates, audit, outcomes) | Go backend | Go backend; Python sidecar for analytics/exports |
| **TimescaleDB** (pro_manage) | Docker | Metric readings | Metrici connectors, Python sidecar for derived metrics | Go backend, dashboards |
| **Neo4j (MDEMG)** | Docker | Observations, constraints, outcomes graph, decisions, concepts | pro_manage via `/v1/conversation/observe`; MDEMG internal | pro_manage via `/v1/jiminy/*` |
| **TimescaleDB (MDEMG)** | Docker | MDEMG metrics and effectiveness telemetry | MDEMG internal | Platform operators (Grafana) |
| **Redis** (pro_manage) | Docker | Session, cache, rate-limit, warmed-guidance snapshots | Go backend | Go backend |
| **S3-compatible object store** (MinIO) | Docker (or external bucket) | Document binaries, export artifacts (PDF/PPTX), signed envelopes | Go backend | Go backend; signed URLs to authenticated users |
| **External systems** (via connector modules) | Off-cluster | Source-of-truth for actuals (ERPi), schedules (Schedulei), docs (Doci), identities (IdPi), signatures (Sigi), notifications (Commsi), raw metrics (Metrici), HSE signals (HSEi) | Those systems | pro_manage connectors |

## Identifiers and versioning

- Primary keys are UUIDv7 (time-ordered). Human-readable codes (e.g. `Project.code`) are indexed, unique within tenant, and are *not* primary keys.
- Every mutable row carries a `version` (monotonic int) and participates in optimistic-concurrency updates (`UPDATE … WHERE version = :expected`). Conflicts raise typed errors.
- "Soft delete" is modeled as a terminal lifecycle state (`Archived`), not a deleted flag — the audit log and reporting continue to resolve historical state.

## Lifecycle state machine (Project)

```
          ┌────────┐  owner-authorized   ┌──────┐  plan review   ┌─────────┐
          │Initiate├────────────────────▶│ Plan ├───────────────▶│ Execute │
          └────────┘                     └──────┘                └────┬────┘
                                                                      │ monitor
                                                                      ▼
                                        ┌────────┐   closeout gate   ┌──────────┐
                                        │ Monitor│◀──────────────────│  Execute │
                                        └───┬────┘ (toggles freely)  └──────────┘
                                            │  all closeout gates satisfied
                                            ▼
                                       ┌───────┐  retention window elapsed  ┌─────────┐
                                       │ Close ├───────────────────────────▶│Archived │
                                       └───────┘                            └─────────┘
```

- **Initiate** — Owner only. Project exists, no team yet. Edits unrestricted; no audit chain required for exploratory data.
- **Plan** — Team invited. Baselines being drafted. Writes audit; approvals not yet mandatory.
- **Execute** — Baselines approved. Writes audit *and* enforce approval gates per `§08`. This is where most project time is spent.
- **Monitor** — Variant of Execute (same rules) for projects in long-tail monitoring (e.g., warranty period, performance test).
- **Close** — All closeout gates satisfied. Writes blocked except for allowed corrections (with elevated approval); reads unrestricted.
- **Archived** — Read-only. Visible only to those with historical access rights or to compliance roles. Deletion only via an explicit admin workflow with full audit.

## Open items for Section 03

- Confirm whether a **Program** layer (parent of multiple related Projects) is needed in year one, or if the Portfolio layer is enough.
- Define the exact set of `MetricDefinition.category` values and their nested sub-categories (how granular do we go? Per-ISO-metric, per-industry-metric?).
- Decide whether `BudgetActual` and `MetricReading` share a storage layer (one TSDB) or stay separated. First instinct: separate — actuals have strict financial-period semantics; metric readings are continuous.
- Decide on the schema contract between pro_manage and `ERPi` for actuals (common chart of accounts? WBS mapping table? Both?). Depends on which ERP comes first (owner intel 2026-10-14).
