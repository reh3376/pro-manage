# 03: Domain Model

## Decisions on record

- **2026-10-13:** The **Project** is the central aggregate. Every data entity lives inside exactly one Project (or inside the Portfolio, which is above projects).
- **2026-10-13:** Project lifecycle states map **one-to-one to the six EEM stages** and are: `Initiate → Plan → Execute → Monitor → Close → Archived`. Movement between states is a controlled transition (`§08`) with distinct permissions and explicit **gates and exit criteria** per stage.
- **2026-10-13:** Projects are typed (`greenfield | brownfield_expansion | reliability | efficiency | compliance | capacity_debottleneck`), carry **TPM-pillar tags** and **FEL stage** attributes, and record **hazardous-area classification** and **regulatory stamps** (NFPA 30 / 70E / 652/61, TTB, FDA, EPA, etc.).
- **2026-10-13:** The **AACE Estimate Class** (5 → 1) is a required attribute of every BudgetBaseline. Workflow gates in `§08` enforce class progression (Class 5 at concept, Class 3 at CAR submission, Class 1 at execution).
- **2026-10-13:** Budget is modeled with **TIC** (Total Installed Cost) and **LCC** (Lifecycle Cost) as separate sub-entities. TIC is tracked throughout the project; LCC is estimated during Design and compared against actuals during Benefits Realization.
- **2026-10-13:** **MP (Maintenance Prevention) information**, **DesignReview**, **CommissioningRecord**, **TurnoverPackage**, and **BenefitsRealization** are first-class entities; see Entity catalog.
- **2026-10-14:** **Asset** and **AssetClass** are first-class, mirrored from the plant CMMS via CMMSi. The CMMS stays the system of record for in-service assets; pro_manage owns *planned* assets (created during Design) until turnover pushes them into the CMMS. MP information, startup curve, OEE readings, TurnoverPackage, and BenefitsRealization key off `asset_id` / `asset_class_id` rather than free-form equipment tags.
- **2026-10-14:** **Loss** and **LossCategory** are first-class. Categories default to the **Six Big Losses** (breakdowns, setup/changeover, minor stops, reduced speed, startup rejects, production rejects) and are extensible per site loss tree. Business-case promises are expressed as recoverable losses priced in dollars.
- **2026-10-14:** **ProjectHealthReview** is first-class: the capital-side analogue of an operations huddle (see `§08`).
- **2026-10-14:** **EPCM delivery objects** are first-class: `Contract`, `WorkPackage` (with readiness constraints and plan compliance), `RFI`, `Submittal`, `ChangeOrder`, `InspectionTestPlan` / `HoldPoint`, `ProjectDeviation`, and `ProjectLossEvent`. `ChangeRequest` stays the owner's internal scope/budget change; `ChangeOrder` is the contractual change under a `Contract` that may implement it.
- **2026-10-14:** DesignReview adds three lenses beyond the seven "easy-to" criteria: **constructability, commissionability, and safety in design**.
- **2026-10-14:** **Site is first-class.** Every project and asset belongs to one `Site`, which carries time zone, currency, units, business calendar, regulatory regime, privacy regime, and the connector instances that serve it. Portfolio stays the business-unit grouping above projects; Site is the physical place. Policy variables gain a `site` scope.
- **2026-10-14:** **Capital authorization is staged.** `CapitalAuthorization` (the CAR) is its own entity with `kind ∈ {fel_funding, full, supplemental}`. FEL funding releases money for front-end engineering at Class 5 or 4; the full CAR authorizes execution at Class 3 on a basic design; supplementals re-authorize cost growth. Detailed engineering happens in Execute under the full CAR, which is how EPCM projects actually run. The estimate behind each CAR is an `EstimateArtifact` with its basis, class, and accuracy range.
- **2026-10-14:** **Engineering document control is a project-controls concern, not storage.** `DocumentRevision` (issue status IFR, IFA, IFC, as-built), `Transmittal`, and a per-package master document register are first-class. Files may live in SharePoint or an EDMS via Doci; the revision status and the register are pro_manage's regardless, because work-package readiness and `waiting_information` losses depend on them.
- **2026-10-14:** **Procurement status is tracked; procurement is not performed.** `ProcurementItem` carries expediting status and the site-need date for long-lead equipment and bulk materials; the purchase order, receipt, and payment stay in the ERP (`§10`).
- **2026-10-14:** **Opportunity precedes Project, and the capital plan is a first-class cycle.** An `Opportunity` is an idea with a priced loss or benefit behind it (from the Loss & opportunity ranking, a site request, a compliance obligation, or an asset nearing end of life). Opportunities are screened, ranked, and placed on a fiscal-year `CapitalPlan`; a planned opportunity becomes a Project at Initiate. Benefits Realization history for similar opportunities surfaces during screening.
- **2026-10-14:** **Imported history is marked as such.** Records loaded from spreadsheets, CAR scans, and legacy tools carry `imported = true`, a batch id, and a source reference, and are never re-approved (`§16`).
- **2026-10-14:** **Risks are quantified and contingency is tied to them.** `Risk` carries three-point cost and schedule impacts; `BudgetBaseline.contingency_basis` names how contingency was set; `ContingencyDrawdown` entries name the risk or change that consumed them. Expected-value arithmetic only in year one; Monte Carlo simulation is out of scope (`§01`).
- **2026-10-13:** **Postgres** holds the authoritative transactional model. **pro_manage's own TimescaleDB** holds metric time series. **MDEMG's Neo4j** holds derived memory and guidance substrate, never the source of truth for pro_manage domain data. **S3-compatible object storage** (on-prem MinIO or equivalent) holds attachment binaries; `Doci` holds pointers for externally-stored documents.
- **2026-10-13:** Every mutable entity records its `created_at`, `created_by`, `updated_at`, `updated_by`, `version` (monotonic int), and `tenant_id` (always the same for a single-company install, but reserved for defense-in-depth).

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
                        │  (per gate: CAR,    │     │
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

### Opportunity
A candidate for capital before it is a project: the idea, the loss or obligation behind it, and a rough size. The entry point of the capital plan cycle.

- **Key fields:** `opportunity_id`, `code`, `title`, `site_id`, `portfolio_id`, `origin` ∈ `{loss_ranking, site_request, compliance_obligation, asset_end_of_life, growth, mp_information, other}`, `source_ref?` (the `LossCategory` and assets, the regulatory stamp, the asset, or the MP information that prompted it), `sponsor_user_id`, `project_type` (proposed), `asset_ids[]`, `problem_statement`, `benefit_kind` (as `BusinessCasePromise.kind`), `priced_benefit_per_year` (from the loss ranking where available), `order_of_magnitude_cost` (Class 5 or a range), `payback_years_est`, `irr_est?`, `urgency` ∈ `{mandatory, this_cycle, next_cycle, someday}`, `screening_score` (derived from a policy-weighted formula: benefit, cost, urgency, strategic fit, risk), `status` ∈ `{proposed, screened, shortlisted, planned, deferred, rejected, converted}`, `capital_plan_line_id?`, `project_id?` (once converted), standard audit stamps.
- **Lifecycle:** `proposed → screened` (Portfolio Executive or PMO reviews the one-page case) `→ shortlisted` (passes the hurdle policy) `→ planned` (placed on a CapitalPlan line) `→ converted` (a Project is created at Initiate with FEL1, carrying the opportunity's promises as `proposed` BusinessCasePromises) or `deferred` / `rejected` with a reason.
- **MDEMG binding:** on screening, `/jiminy/guide` is called with `context="Screening opportunity for <asset_class> at <site>"` so Benefits Realization outcomes of similar past projects ("projects of this kind delivered 60% of promised changeover recovery") and relevant MP information surface before the number is believed.

### CapitalPlan / CapitalPlanLine
The fiscal-year capital plan: which opportunities and continuing projects get money, in which periods, against the envelope.

- `CapitalPlan`: `plan_id`, `fiscal_year`, `portfolio_id` (or company-wide), `envelope_amount`, `currency`, `status` ∈ `{draft, under_review, approved, revised, closed}`, `version`, `approved_at`, `approved_by`, `workflow_id` (`§08`), `supersedes_plan_id?` (a mid-year revision).
- `CapitalPlanLine`: `line_id`, `plan_id`, `opportunity_id?` or `project_id?` (continuing projects carry forward), `planned_amount_by_period[]` (quarters or months), `priority_rank`, `category` ∈ `{growth, cost_reduction, reliability, compliance, safety, sustaining, other}`, `committed` (bool: approved CAR exists), `status`.
- **Derived:** `plan.envelope_remaining`, `plan.committed_pct`, `plan.carryover_from_prior_year`, and per line the variance between planned, authorized (CAR), and forecast (EAC).
- **ERP-led profile:** where the ERP carries a capital budget, the plan's approved lines release to it as budget placeholders, and CAR approvals later release the authorized amount against the line (`ReleaseBudget` with `plan_line_ref`). Otherwise the plan is pro_manage's.
- **Why first-class:** the loss ranking and Benefits Realization history are only worth having if they shape which projects get money. The plan is where that happens, and the funnel from opportunity to realized benefit is only measurable if the opportunity is a row.

### Site
The physical place a project builds at and an asset lives in. One company may run several sites, possibly in more than one country; everything that differs by place hangs off the Site rather than being repeated per project.

- **Key fields:** `site_id`, `code`, `name`, `portfolio_id?` (the business unit it reports into), `country`, `time_zone` (IANA), `currency` (ISO 4217), `unit_system` ∈ `{imperial, metric}`, `business_calendar_id` (working days and holidays, used by SLAs), `regulatory_regime[]` (for example `ttb`, `cfia`, `excise_ca`, `osha`, `provincial_ohs`), `privacy_regime[]` (for example `pipeda`, `state_privacy`), `loss_tree_root_id?` (the site's loss tree under `LossCategory`), `address`, `default_connector_instances` (which ERPi, CMMSi, HSEi, and Metrici instances serve this site; `§05` routing), `site_operations_lead_pool`, `site_maintenance_lead_pool`, standard audit stamps.
- **Relationships:** 1..n Projects (a project has exactly one primary site; a multi-site effort is modelled as one project per site under a Program, see Open items), 1..n Assets.
- **Used by:** policy-variable resolution (`site` scope, `§08`), SLA clocks (business days in the site's calendar and time zone), money display and portfolio roll-up (`§07`), approval pools (`finance_approver ∩ site or portfolio` per the DoA), HSE and regulatory stamps defaulted onto new projects, and connector routing.
- **Permissions:** Platform Admin manages structure; site leads are assigned per site.

### Project (central aggregate)
A single capital project with one Owner and a bounded team.

- **Key fields:** `project_id`, `code` (human-readable PK, e.g. `P-INDUSTRIAL-EAST-7`), `name`, `description`, `portfolio_id`, `site_id`, `currency` (defaults to the site's), `opportunity_id?` (the opportunity it was converted from), `capital_plan_line_id?`, `onboarding_level` ∈ `{registered, governed, full}` (`§16`), `imported` (bool) with `import_batch_id?`, `owner_user_id`, `lifecycle_state` (EEM-mapped), `fel_stage` (`fel1 | fel2 | fel3 | n/a`; n/a once past authorization), `project_type` (`greenfield | brownfield_expansion | reliability | efficiency | compliance | capacity_debottleneck`), `tpm_pillar_tags[]` (`eem | focused_improvement | planned_maintenance | autonomous_maintenance | quality_maintenance | …`), `hazardous_area_classification` (`class_i_div_1 | class_i_div_2 | class_ii_div_1 | class_ii_div_2 | none`), `regulatory_stamps[]` (`nfpa_30 | nfpa_70e | nfpa_652 | nfpa_61 | ttb | fda | epa | osha | msha | …`), `start_date`, `target_end_date`, `baseline_budget_total`, `baseline_duration_days`, `template_id?`, `tags[]`, standard audit stamps.
- **MDEMG binding:** every project maps to one `space_id = project_id` in MDEMG. The project's `code` is the human-readable form; the UUID `project_id` is what crosses the wire.
- **Lifecycle (EEM mapping):** `Initiate` ≈ Concept/Planning · `Plan` ≈ Design · `Execute` ≈ Fabrication → Installation → Commissioning · `Monitor` ≈ Early Management Period · `Close` ≈ Normal operations handover (Benefits Realization window begins) · `Archived`. Transitions gated by `§08`.
- **Permissions:** Owner has all; Teammates per ACL grants; Executives read within their portfolio.

### ProjectMember
A user's membership in a specific project with an assigned base role.

- **Key fields:** `project_id`, `user_id`, `base_role` ∈ `{owner, editor, viewer, contributor}` (`contributor` = external EPCM/contractor staff, scoped by `Contract`; see `§04`), `invited_by`, `invited_at`, `activated_at`, `expires_at?` (for time-boxed access like auditors).
- Relationships: belongs to Project; links to User (resolved via IdPi).

### ACLGrant
A fine-grained grant on top of a ProjectMember's base role. The full-ACL model lives here. See `§04` for the enforcement details and model constraints.

- **Key fields:** `grant_id`, `project_id`, `user_id`, `scope_kind` ∈ `{section, document, field}`, `scope_target` (e.g., `section:budget`, `document:doc_123`, `field:budget.salary`), `permission` ∈ `{read, write, deny}`, `granted_by`, `granted_at`, `expires_at?`.
- **Default:** Absence of grant means the base role determines access; an explicit `deny` can mask a base-role-granted access (e.g., Editor across the project, but Deny on `field:budget.salary`).

### Budget
*In the ERP-led profile, baselines, forecasts, EAC, and commitments are mirrored from the ERP (see System of record by deployment profile).*

The financial state of the project. Modeled with distinct **TIC** (Total Installed Cost, tracked throughout the project) and **LCC** (Lifecycle Cost, estimated during Design and validated during Benefits Realization).

- **Sub-entities:**
  - `BudgetBaseline`: one `current=true` row + historical rows. Each baseline has `baseline_id`, `baseline_total` (TIC), `aace_class` ∈ `{5, 4, 3, 2, 1}`, `estimate_id` (the `EstimateArtifact` it came from), `fel_stage_at_capture`, `approved_at`, `approved_by`, `source` (`manual|import|workflow`), `change_request_id?`, `contingency_amount`, `contingency_basis` ∈ `{percent_of_tic, risk_based_expected_value, risk_based_p50, risk_based_p80, deterministic_allowance}`, `contingency_basis_note`.
  - `EstimateArtifact`: the estimate behind a baseline or a CAR, kept so "what did we know when we asked for the money" is answerable. `estimate_id`, `project_id`, `aace_class`, `as_of`, `prepared_by` (person or EPCM organization), `reviewed_by`, `basis_of_estimate_doc_ref`, `scope_definition_pct` (the engineering completeness the class was earned on), `accuracy_range_low_pct`, `accuracy_range_high_pct`, `tic`, `contingency_amount`, `contingency_method`, `escalation_assumptions`, `exclusions`, `currency`, `exchange_rate_snapshot?`. A baseline cannot claim a class its estimate artifact does not carry.
  - `BudgetLineItem`: WBS-aligned planned amounts. `line_id`, `wbs_path`, `account_code`, `planned`, `baseline_id`.
  - `BudgetActual`: read-only, written by **ERPi** on sync. `actual_id`, `wbs_path`, `account_code`, `period`, `amount`, `source_doc_id?`, `erpi_pulled_at`.
  - `BudgetCommitment`: POs issued, from ERPi.
  - `BudgetForecast` (EAC): point-in-time Estimate At Completion. `forecast_id`, `as_of`, `wbs_path`, `eac_amount`, `method` (`manual|ratio|regression|…`).
  - `LCCEstimate`: Lifecycle Cost estimate. `lcc_id`, `as_of`, `period_years`, `tic` (snapshot), `opex_annual`, `maint_annual`, `energy_annual`, `end_of_life_cost`, `total_lcc`, `method_note`.
  - `ContingencyDrawdown`: running record of contingency spent vs project percent-complete. Each entry: `drawdown_id`, `amount`, `at`, `approved_by`, `risk_id?` (the risk that materialized), `change_request_id?` or `change_order_id?` (the change that consumed it), `reason`. Early-warning signal (burning contingency faster than progress), and, because entries name their cause, a record of which risks actually bit.
- **Derived:** `variance_pct = (actual + remaining_commit - baseline_total) / baseline_total`; `cpi = bcwp/acwp`; `spi = bcwp/bcws` (when earned-value inputs are present); `contingency_burn_ratio = contingency_used_pct / completion_pct`; `contingency_coverage = contingency_remaining / Σ(open risk likelihood × likely cost impact)` (expected value of open quantified risks vs what is left to cover them; below 1.0 is a health-review flag).
- **Money and currency:** every money field carries its currency (the project's, defaulting to the site's). Portfolio roll-ups across currencies convert at the rate table the `policy_owner` maintains (`fx.rate.<from>.<to>`, dated), and the rate used is stored with the roll-up. CAR thresholds are compared in the currency the policy value is expressed in, converting the request amount at the rate in force at submission and recording it in the `policy_snapshot`.
- **Permissions:** Finance Approvers always have `read` on budget sections of projects in their jurisdiction. Owner has `read` + controlled `write`. Teammate access is grant-driven. **Sensitive sub-fields** (vendor costs, bid prices, unit rates) remain deny-by-default per `§04`.

### Schedule
Time dimension of the project.

- **Sub-entities:**
  - `Milestone`: `milestone_id`, `name`, `baseline_date`, `target_date`, `actual_date?`, `status`, `dependencies[]` (other milestone_ids), `completion_pct`.
  - `ScheduleBaseline`: snapshot of all milestones at a given approved point. Mirrors `BudgetBaseline` shape.
- **Imports:** `Schedulei` imports from MS Project (.mpp via XML export), Primavera P6 (XER), Smartsheet (API). On import, mapping produces milestones + dependencies; original file is retained as attachment.
- **Mechanical rule:** cannot mark `complete` with open dependencies (UC-05). Enforced in-process, pre-MDEMG.

### WBS & Deliverables (Scope)
*In the ERP-led profile, the WBS (projects, sub-projects, activities) is mirrored from the ERP; deliverables, acceptance, and change requests remain pro_manage records.*

Work breakdown and deliverable-level tracking.

- **Sub-entities:**
  - `WBSNode`: `wbs_id`, `project_id`, `parent_wbs_id?`, `code`, `name`, `description`, `completion_pct`.
  - `Deliverable`: `deliverable_id`, `wbs_id`, `name`, `description`, `due_date?`, `accepted_at?`, `accepted_by?`, `acceptance_criteria`.
  - `ChangeRequest`: `cr_id`, `project_id`, `title`, `description`, `impact_budget_pct?`, `impact_schedule_days?`, `status` ∈ `{draft, submitted, approved, rejected, implemented}`, approval workflow ref.

### Risk / Issue
Risk register + issue log.

- `Risk`: `risk_id`, `project_id`, `title`, `description`, `likelihood` (1-5, with a mapped probability band), `impact` (1-5), `score` (derived), `response_strategy` (`avoid|mitigate|transfer|accept`), `owner_user_id`, `due_date`, `status` ∈ `{open, mitigating, closed, materialized}`, `last_reviewed_at`, and **quantification** (required above a policy-set score, `risk.quantify_above_score`): `cost_impact_low`, `cost_impact_likely`, `cost_impact_high`, `schedule_impact_days_low/likely/high`, `quantified_by`, `quantified_at`. `expected_cost = probability × cost_impact_likely` (derived). When a risk materializes it links to the `ContingencyDrawdown` entry or change that paid for it, so the register learns which risks were real.
- `Issue`: `issue_id`, `project_id`, `title`, `description`, `severity` ∈ `{low, med, high, critical}`, `status` ∈ `{open, in_progress, resolved}`, `assignee_user_id`, `opened_at`, `resolved_at?`.
- **Aging rules** (per Jiminy constraint corpus): risks aged >14d without `last_reviewed_at` update, issues aged >30d without resolution; both are surfaced in guidance.

### Document
Project-attached artifacts.

- **Modes:**
  - **Local:** `document_id`, `project_id`, `name`, `mime_type`, `size_bytes`, `storage_key` (S3), `versions[]`, `uploaded_by`, `uploaded_at`, `tags[]`.
  - **External (via Doci):** same fields + `doci_backend` (`sharepoint|drive|confluence|…`), `external_id`, `external_url`, `external_permissions_mirror` (snapshot of upstream ACL for drift detection).
- **Permissions:** standard ACL applies. For external docs, pro_manage mirrors permissions from the external store on sync and surfaces drift.

### DocumentRevision / Transmittal / DocumentRegister (engineering document control)
What a drawing's status is, who has been issued it, and which documents a package is waiting on. This is project control, not storage: the file may live in MinIO, SharePoint, or an engineering document management system via Doci; the register and the revision status are pro_manage's either way.

- `DocumentRegister` entry (the master document register, MDR): `register_id`, `project_id`, `package_id?`, `document_number` (per the project numbering scheme), `title`, `discipline`, `document_type` ∈ `{drawing, datasheet, specification, calculation, procedure, vendor_document, report, other}`, `responsible_org` (owner, EPCM, designer, or supplier), `planned_ifr_at`, `planned_ifc_at`, `current_revision_id?`, `required_for_package_release` (bool), `required_for_turnover` (bool, as-built expected).
- `DocumentRevision`: `revision_id`, `register_id`, `document_id` (the stored or linked file), `revision_code` (A, B, … for pre-IFC; 0, 1, … for IFC and after, per the numbering scheme), `issue_status` ∈ `{ifr, ifa, ifc, as_built, superseded, void}` (issued for review, for approval, for construction), `issued_at`, `issued_by`, `supersedes_revision_id?`, `transmittal_id?`, `review_comments_ref?`, `change_driver` (`rfi_id?`, `change_order_id?`, `design_review_finding_id?`). Only one non-superseded revision per register entry. A revision cannot reach `ifc` without a closed `detailed` design review for its scope where one is required, or an audited override.
- `Transmittal`: the formal issue of document revisions between organizations. `transmittal_id`, `project_id`, `contract_id?`, `direction` ∈ `{outgoing, incoming}`, `from_org`, `to_org`, `purpose` ∈ `{for_review, for_approval, for_construction, for_information, as_built}`, `revision_ids[]`, `sent_at`, `response_due_at?`, `acknowledged_at?`, `acknowledged_by?`. Incoming transmittals from the EPCM or suppliers are raised by contributors; `Submittal` rows link to the transmittal that carried them.
- **What it feeds:** the work-package readiness constraint "approved-for-construction drawings current" becomes a mechanical check (every `required_for_package_release` register entry under the package has a current `ifc` revision); IFC-on-time and revision churn metrics (`§07`); `waiting_information` project losses when a package waits on a planned IFC that slipped; and the TurnoverPackage's as-built section, which is complete when every `required_for_turnover` entry has an `as_built` revision.
- **System of record:** pro_manage in both deployment profiles. Where the company or its EPCM runs an EDMS (Aconex, ProjectWise, IFS Document Management, SharePoint libraries with revision metadata), a Doci module mirrors the register and revision status in rather than replacing it (`§10`).

### ProcurementItem (procurement and expediting status)
Long-lead equipment and bulk materials tracked against the date the site needs them. The purchase order, receipt, and payment are the ERP's; the expediting status and the link to the schedule and the package are pro_manage's.

- **Key fields:** `item_id`, `project_id`, `package_id` (the procurement package), `asset_id?`, `register_entries[]` (vendor documents expected), `description`, `supplier` (ERP vendor ref), `erpi_po_ref?`, `erpi_commitment_ref?`, `long_lead` (bool), `required_on_site_at` (from the schedule or the construction package's planned start), `promised_delivery_at`, `forecast_delivery_at`, `status` ∈ `{planned, rfq, bid_evaluation, awarded, in_fabrication, fat_scheduled, fat_complete, shipped, received, inspected, issued_to_package, rejected}`, `expediting_notes[]` (dated, by whom), `fat_record_id?` (the `CommissioningRecord` of kind `fat`), `receiving_inspection_ref?`, `procurement_lead_user_id`.
- **Derived:** `delivery_float_days = required_on_site_at − forecast_delivery_at`; negative float is a readiness risk on the dependent construction package and a leading indicator in health reviews.
- **What it feeds:** work-package readiness ("materials and long-lead items on site"), `waiting_material` project losses when a released package waits on an item, and the TurnoverPackage spares list (items tagged as spares).

### Metric
Two layers: definitions and readings.

- `MetricDefinition`: `metric_id`, `name` (e.g., `budget_variance_pct`, `cpi`, `safety_incident_count`, `equipment_uptime_pct`), `category` ∈ `{financial, schedule, scope, safety, quality, compliance, resource, custom}`, `unit`, `aggregation` (`sum|avg|last|…`), `scope` (`project|portfolio|global`), `source` (`derived|erpi|metrici|manual`), `formula?` (for derived), `connector_mapping_id?` (for connector-sourced metrics).
- `MetricReading`: `reading_id` (composite: `metric_id + project_id + ts`), `value`, `quality` (`good|questionable|bad`), `source_ref`. Stored in **pro_manage's own TimescaleDB**.
- **Harvest cadence:** per-connector (Metrici), ranging from seconds (plant historian) to daily (ERP actuals).

### CapitalAuthorization (CAR)
The owner's authorization of money for a project, staged the way capital actually gets released.

- **Key fields:** `car_id`, `project_id`, `kind` ∈ `{fel_funding, full, supplemental}`, `amount_requested`, `currency`, `basis` (what the amount is measured on, from `car.threshold.basis`), `estimate_id` (the `EstimateArtifact`; its `aace_class` must meet the kind's required class), `fel_stage_at_submission`, `business_case_ref` (promises and narrative), `supersedes_car_id?` (a supplemental names the CAR it adds to), `cumulative_authorized` (derived: this plus every earlier approved CAR on the project), `workflow_id` (the `ApprovalWorkflow` of kind `car`), `status` ∈ `{draft, in_flight, approved, rejected, cancelled}`, `authorized_amount`, `authorized_at`, `erp_budget_ref?` (set by `ReleaseBudget` in the ERP-led profile), `signed_artifact_ref?`.
- **Kinds:**
  - `fel_funding`: releases money for FEL2 and FEL3 work (studies, basic engineering, long-lead quotations) before the project is sanctioned. Required class per `car.fel_funding.required_aace_class` (default 5); amount capped at `car.fel_funding.cap_pct` of the current estimate (default 5%). Does not move the lifecycle state.
  - `full`: the sanction. Required class per `car.full.required_aace_class` (default 3), a closed `basic` design review (`§08`), and committed business-case promises. Approval of the full CAR plus the basic design review sign-off moves the project `Plan → Execute`.
  - `supplemental`: re-authorization when forecast cost growth exceeds `car.supplemental.reapproval_pct` or a scope change needs money. Routed on the **cumulative** authorized amount, so a project cannot creep past a threshold in small steps.
- **Why an entity, not just a workflow:** the CAR is what Finance, the ERP budget, the business-case promises, and Benefits Realization all key off. Keeping it as a row with its estimate and cumulative amount is what makes "authorized vs estimate at the time vs final cost" reportable without reconstructing it from workflow history.

### ApprovalWorkflow
A multi-stage approval instance (`§08` details the state machine).

- `ApprovalWorkflow`: `workflow_id`, `project_id?` (null for portfolio-level kinds), `kind` ∈ `{opportunity_screening, capital_plan, car, baseline_change, design_review_signoff, cqv_signoff, milestone_signoff, change_request, change_order, rfi_response, submittal_review, work_package_release, hold_point_release, shutdown_tiein_plan, early_mgmt_exit, closeout, benefits_realization_review, custom}`, `subject_ref` (what's being approved: a budget baseline id, a change request id, …), `initiated_by`, `initiated_at`, `current_stage`, `status` ∈ `{in_flight, approved, rejected, cancelled, expired}`, `signed_artifact_ref?` (Sigi reference when applicable), `policy_snapshot` (effective policy-variable values captured when the workflow started; see `§08`).
- `ApprovalStage`: `stage_id`, `workflow_id`, `order`, `role_required` (e.g., `vp_finance`), `assignees[]`, `sla_duration`, `completed_at?`, `completed_by?`, `decision` ∈ `{approve, reject, delegate}`, `comment?`, `guidance_id?` (MDEMG reference captured when the approver viewed guidance), `signed_envelope_ref?` (Sigi envelope ID).

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

- `Resource`: `resource_id`, `kind` ∈ `{person, equipment}`, `name`, `external_ref?` (HR system employee id, asset tag, etc.).
- `Allocation`: `allocation_id`, `resource_id`, `project_id`, `start`, `end`, `percent` (0.0–1.0 for people; usage factor for equipment).
- **Overallocation detection:** sum of `percent` across concurrent allocations > 1.0 raises a flag; surfaced in guidance and dashboards.

### Template
A re-usable starter for a Project (or a Milestone set, Risk categories, etc.).

- `ProjectTemplate`: `template_id`, `name`, `description`, `wbs_structure` (JSON), `default_milestones` (JSON), `default_risks` (JSON), `default_metric_definitions_ref[]`, `default_tpm_pillar_tags[]`, `default_regulatory_stamps[]`, `created_by`, `created_at`.
- Projects can be created from templates or converted to templates.

### DesignReview
A formal review event during Design (Plan) stage against the seven "easy-to" criteria. Operator and maintenance participation is first-class.

- **Key fields:** `review_id`, `project_id`, `scope` (free-text + optional reference to a design document), `stage` (`basic | detailed | pre_approval | post_change`; `basic` reviews the FEL3 basic design before the full CAR, `detailed` reviews detailed engineering in Execute before documents go IFC, `post_change` follows a change that touches equipment), `register_entries[]` (the documents reviewed), `scheduled_at`, `participants[]` (resolved to users; **must include at least one operator role and one maintenance role** for Design reviews of equipment-touching scope), `facilitator_user_id`, `status` ∈ `{planned, in_session, closed, cancelled}`.
- **Criteria:** every DesignReview carries findings against the seven "easy-to" lenses (`easy_to_operate`, `easy_to_maintain`, `easy_to_clean`, `easy_to_change_over`, `easy_to_inspect`, `easy_to_keep_safe`, `easy_to_make_quality_product`) plus three project-delivery lenses (`constructability`, `commissionability`, `safety_in_design`). Each lens: `rating` (1–5 or `n/a`), `findings[]`, `dispositions[]`. A Construction Manager is a required participant when the constructability lens is in scope.
- **Dispositions** carry `finding_id`, `action` (`design_change | accept | mitigate | defer`), `owner_user_id`, `due_date`, `status`, `resolution_note`, `change_request_id?` (if the disposition triggers a CR).
- **Permissions:** owner, editors with design-review write grant, operators + maintenance participants in `read+comment` mode, platform roles per `§04`.

### MPInformation
Maintenance-Prevention information: observations harvested from running assets that feed future project designs. The institutional-memory substrate of EEM.

- **Key fields:** `mp_id`, `source_project_id?` (where the observation originated; may be from a running asset not tracked in pro_manage), `source_asset_id?`, `asset_class_id` (e.g. `boiler`, `column_still`, `bottling_line`, `fermenter`, `conveyor`, `heat_exchanger`, …), `source_kind` ∈ `{closeout_capture, cmms_failure_history, abnormality_tag_pattern, loss_pattern, manual}`, `authored_by_user_id`, `authored_at`, `content` (narrative), `category` ∈ `{failure, operability, maintainability, cleanability, changeover, safety, quality, energy, cost, other}`, `loss_category_id?` (which of the Six Big Losses it drove), `severity_at_source` (`low | med | high | critical`), `evidence_refs[]` (document / incident / metric / tag references), `linked_mp_ids[]` (related observations).
- **MDEMG binding:** each MPInformation row is also written as an `obs_type=learning` observation into the governance space (`space=org.<id>.governance`) with `scope=equipment:<asset_class>` tags. Jiminy's retrieval surfaces these during Design-stage guidance for the same asset class on later projects.
- **Lifecycle:** `draft → published → superseded?` Governance Authors can promote repeatedly-cited MPInformation into formal constraints (via `§06`).

### CommissioningRecord
Captures FAT, SAT, and (where applicable) formal CQV activity.

- **Key fields:** `record_id`, `project_id`, `kind` ∈ `{fat, sat, commissioning, qualification, validation}`, `equipment_ref` (what asset the record is for), `vendor` (for FAT), `location`, `scheduled_at`, `conducted_at`, `conducted_by_user_id`, `witnesses[]`, `test_protocol_ref` (document ref), `result` ∈ `{pass, pass_with_punchlist, fail, retest_scheduled}`, `punchlist_items[]`, `signed_envelope_ref?` (via Sigi where formal), `attached_evidence[]`.
- **Linked to:** scheduled Milestones (so completion of CQV becomes a milestone event) and workflow stages (CQV sign-off gate; see `§08`).

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
- **CMMS delivery:** on `approved`, the package is pushed to the plant CMMS via **CMMSi**: equipment hierarchy, spares, PM tasks, and procedures are created or updated. Delivery is idempotent; retries are safe. Delivery status is logged and the audit trail links the CMMS record IDs back to the pro_manage package.

### BenefitsRealization
Tracks whether the business case promise actually materialized post-startup. The third ROI calculation of a project.

- **Key fields:** `br_id`, `project_id`, `window_opened_at` (defaults to project `Close` date), `window_duration_days` (from the `benefits.window_days` policy variable, overridable per project), `window_closes_at`, `status` ∈ `{open, in_progress, closed_realized, closed_partial, closed_not_realized}`, `business_case_promises[]` (see BusinessCasePromise), `measurements[]` (periodic snapshots pulled from Metrici/HSEi during the window), `final_variance_report` (narrative + numeric: promised vs delivered, variance explanation), `signed_envelope_ref?` (if final review is formally signed via Sigi).
- **Verified recovery:** measurement is against the *specific* losses the business case promised to recover, on the *specific* assets, compared with a pre-project baseline captured at CAR time, not against generic KPIs. A promise of "recover $1.2M/yr of changeover loss on Line 3" is closed by measuring changeover loss on Line 3's assets during the window.
- **Workflow (`§08`):** BenefitsRealization carries its own closeout workflow. The project does not move to `Archived` until BenefitsRealization is closed (except when the window itself is explicitly waived with admin override + audited reason).
- **MDEMG binding:** at closeout of BenefitsRealization, pro_manage writes a `decision` observation to the governance space summarizing what was realized vs promised, with `scope` tags that let similar future projects surface it during their own business-case drafting.

### BusinessCasePromise
One measurable commitment in a CAR's business case. The bridge between FEL, the CAR, and Benefits Realization.

- **Key fields:** `promise_id`, `project_id`, `kind` ∈ `{loss_recovery, capacity, cost_per_unit, energy_per_unit, quality, safety, compliance, other}`, `asset_ids[]`, `loss_category_id?` (for `loss_recovery`), `metric_id` (how it will be measured), `baseline_value`, `baseline_period` (captured at CAR time from production data), `promised_value`, `promised_by_date`, `dollar_value_per_year` (the priced loss or saving), `measurement_basis` (narrative: what counts, over what period), `status` ∈ `{proposed, committed, realized, partially_realized, not_realized}`.
- **Why first-class:** ROI is calculated three times (business case, closeout, Benefits Realization). Storing the promise with its basis at CAR time is what makes the third calculation honest: the measurement basis cannot drift after the fact.

### Asset / AssetClass
The physical equipment a project designs, installs, or modifies.

- `AssetClass`: `asset_class_id`, `name` (`boiler`, `column_still`, `bottling_line`, …), `parent_class_id?`, `default_turnover_template_id?`, `default_easy_to_checklist_id?`.
- `Asset`: `asset_id`, `asset_class_id`, `site_id`, `cmms_ref?` (CMMS equipment id once in service), `name`, `tag_number`, `status` ∈ `{planned, under_construction, commissioning, early_management, in_service, retired}`, `project_ids[]` (projects that touched it), `nameplate` (JSON: rated capacity, speed, etc.).
- **System of record:** pro_manage owns `planned → early_management` assets it creates during Design. On TurnoverPackage delivery, the asset is upserted into the CMMS via CMMSi and `cmms_ref` is stored; from `in_service` onward the CMMS is authoritative and pro_manage mirrors.
- **Why it matters:** startup curve, OEE, losses, MP information, turnover, and Benefits Realization all attach to the same `asset_id`, so "what did we learn from the last three boilers?" is a lookup by `asset_class_id`, not a fuzzy tag match.

### Loss / LossCategory
Production losses, in the vocabulary operations already uses.

- `LossCategory`: `loss_category_id`, `name`, `six_big_losses_bucket` ∈ `{breakdowns, setup_changeover, minor_stops, reduced_speed, startup_rejects, production_rejects, none}`, `oee_component` ∈ `{availability, performance, quality}`, `parent_id?` (site loss tree), `default_cost_basis` (how to price an hour or unit of this loss).
- `LossEvent` (time-series, pm-TSDB): `asset_id`, `loss_category_id`, `ts_start`, `ts_end`, `quantity` (minutes / units), `priced_value` (dollars), `source` (`metrici | hsei | cmmsi | manual`), `source_ref`. Harvested from MES / OEE platforms; pro_manage does not originate loss events.
- **Uses:** business-case baselines, Benefits Realization verification, portfolio ranking by recoverable loss (`§09`), and MP-information harvest (repeated losses on an asset class).

### PolicyVariable / PolicyValue
Admin-configurable thresholds, SLAs, cadences, and windows used by workflows and governance, with values that can be overridden per project. Defined in `§08` (Policy variables).

- `PolicyVariable`: `key`, `type`, `unit`, `description`, `global_default`, `allowed_range`, `overridable_at[]`, `owner_role`.
- `PolicyValue`: `variable_key`, `scope_kind` ∈ `{global, project_type, portfolio, site, project}`, `scope_id`, `value`, `effective_from`, `effective_to?`, `set_by`, `reason`, `approved_by?`, version. Resolution: project → site → portfolio → project type → global.

### ProjectHealthReview
A periodic structured review of an in-flight project; the capital-side counterpart to an operations huddle.

- **Key fields:** `review_id`, `project_id`, `held_at`, `facilitator_user_id`, `participants[]`, `scope_stability` (1–5), `estimate_maturity_vs_stage` (AACE class vs FEL/lifecycle stage), `critical_path_float_days`, `top_risks[]` (with owners), `change_order_trend`, `contingency_burn_ratio` (from §03 Budget), `ops_maint_turnover_readiness` (1–5), `pm_concerns_not_on_register` (free text), `actions[]` (owner, due date), `dominant_project_loss` (the loss category selected from data for focused improvement this cycle), `overall` ∈ `{green, amber, red}`.
- Produces actions with owners, not just a grade. Workflow and cadence in `§08`.

### Contract
A commercial agreement the owner holds with an EPCM partner, designer, supplier, or construction contractor. Under EPCM the owner holds these contracts; the EPCM partner coordinates them.

- **Key fields:** `contract_id`, `project_id`, `counterparty_org` (name + IdP federation ref), `kind` ∈ `{epcm_services, engineering, supply, construction, installation, commissioning_support, other}`, `contracting_model` ∈ `{epcm, epc, design_build, lump_sum, unit_rate, reimbursable}`, `value`, `erpi_commitment_ref?` (PO/commitment in the ERP), `work_package_ids[]`, `start`, `end`, `status`.
- **Access:** a Contract scopes what its counterparty's contributors can see and edit (`§04`). Contract value is visible to the counterparty; other contracts' commercials are not.

### WorkPackage
A unit of planned work in the Advanced Work Packaging sense: engineering (EWP), procurement, or construction/installation (CWP / IWP) packages.

- **Key fields:** `package_id`, `project_id`, `kind` ∈ `{engineering, procurement, construction, installation, commissioning}`, `wbs_id`, `contract_id?`, `asset_ids[]`, `package_lead_user_id`, `planned_start`, `planned_finish`, `status` ∈ `{planned, ready, released, in_progress, complete, on_hold}`, `progress_pct`, `quantities[]` (planned vs installed, for earned value).
- **Readiness constraints** (all must be met, or overridden with an audited reason, before `released`): approved-for-construction drawings current (mechanical: every `required_for_package_release` register entry has a current `ifc` revision), no blocking RFIs open, materials and long-lead items on site (mechanical: every `ProcurementItem` feeding the package is `received` or later), crew and equipment assigned, permits in hand, HSE prerequisites (job hazard analysis, permits to work) complete, hold points defined in the ITP. Each constraint: `kind`, `status`, `owner`, `evidence_ref`.
- **Plan compliance:** a weekly record of packages planned to complete vs actually completed (percent plan complete), with a `variance_reason` per miss drawn from the project-loss categories, so the reasons accumulate as data.

### RFI / Submittal / ChangeOrder
The administrative flow that EPCM projects run on, modeled so its delays are measured.

- `RFI`: `rfi_id`, `project_id`, `package_id?`, `raised_by` (often a contributor), `discipline`, `question`, `attachments[]`, `assigned_to`, `due_at`, `answered_at?`, `answer`, `resulting_change_order_id?`, `resulting_document_revisions[]`, `status`.
- `Submittal`: `submittal_id`, `project_id`, `contract_id`, `package_id?`, `kind` ∈ `{shop_drawing, product_data, sample, method_statement, vendor_data, test_report}`, `revision`, `submitted_at`, `review_due_at`, `disposition` ∈ `{approved, approved_as_noted, revise_resubmit, rejected}`, `reviewed_by`, `document_refs[]` (via Doci or local).
- `ChangeOrder`: `co_id`, `project_id`, `contract_id`, `change_request_id?` (the internal CR it implements), `description`, `cost_impact`, `schedule_impact_days`, `status`, approval workflow ref. Distinct from `ChangeRequest` (owner-internal scope/budget change).
- **Cycle time** (raised → closed) is stored on every row; aging feeds the `waiting_*` project losses.

### InspectionTestPlan / HoldPoint
Built-in quality during construction (the quality-maintenance pillar applied to the project).

- `InspectionTestPlan`: per package or asset: the ordered inspections and tests, acceptance criteria, and which are **hold points** (work cannot proceed until released) vs **witness points** (notify, may proceed).
- `HoldPoint`: `hold_point_id`, `itp_id`, `package_id`, `criteria`, `inspector_role`, `status` ∈ `{pending, released, failed}`, `released_by`, `released_at`, `evidence[]` (photos, test results).
- Failed inspections create non-conformances; recurring non-conformance causes are proposed as specification lessons (MPInformation with `source_kind = quality_pattern`) for future projects.

### ProjectDeviation
A tag raised by anyone close to the work (a site supervisor, contractor lead, or owner's rep) when something is off on the project itself: an interface clash, incomplete prior work, a missing material, an emerging risk. The project-side counterpart to an operator's abnormality tag.

- **Key fields:** `deviation_id`, `project_id`, `package_id?`, `asset_id?`, `raised_by`, `raised_at`, `category` (a project-loss category), `description`, `photos[]`, `location`, `assigned_to`, `resolved_at?`, `escalated_to?` (RFI, issue, risk, or change request).

### ProjectLossEvent / ProjectLossCategory
The losses of the project itself, measured the way a plant measures equipment losses.

- `ProjectLossCategory`: default set: `waiting_information` (RFIs, drawings not yet IFC), `waiting_approval` (submittals, change orders, internal approvals), `waiting_material` (released package waiting on a procurement item), `rework` (non-conformance, design error), `interface_clash`, `unready_package` (released or started without readiness), `safety_stop`, `skill_gap_error`; extensible.
- `ProjectLossEvent` (pm-TSDB): `project_id`, `package_id?`, `category`, `ts_start`, `ts_end`, `duration_days`, `cost` (rework cost, or delay days × the project's cost-of-delay rate where configured), `source_ref` (RFI, deviation, NCR, plan-compliance miss, incident).
- Most events are derived automatically: RFI and submittal aging, plan-compliance misses with their variance reasons, non-conformances, deviations, and HSE stops.

### TeamCompetency (Phase 3)
Role-based competency for the project team (owner, EPCM, and contractor staff), complementing the operator certification gate.

- `CompetencyRequirement` (per workflow role or package kind) and `TeamCompetency` (`user_id`, `competency`, `level`, `evidence`, `expires_at?`). Gaps on a package's assigned team surface as a readiness warning, not a block.

## System of record by deployment profile

Which system owns project-controls records depends on what the ERP connector provides (`§10` capability tiers). The profile is set per ERP instance from its module's `Capabilities`. ERP instances are routed by site or portfolio (`§05`), so a company running one ERP with project controls and another without runs ERP-led for the sites or portfolios on the first and pro_manage-led for the rest. It is never a per-project choice.

| Record | **ERP-led** profile (ERP has a projects module, e.g. IFS Cloud) | **pro_manage-led** profile (financials-only ERP) |
|---|---|---|
| Project structure (projects, sub-projects, activities, milestones) | ERP owns; pro_manage mirrors and adds pro_manage-only attributes (lifecycle state, FEL stage, TPM tags, readiness) | pro_manage owns |
| Budget baselines, forecast versions, EAC | ERP owns; pro_manage mirrors. AACE class and CAR linkage are pro_manage attributes on the mirrored baseline | pro_manage owns |
| Earned value (BCWS/BCWP/ACWP, CPI/SPI) | ERP computes; pro_manage reads | pro_manage computes (`§07`) |
| Contracts / subcontracts, payment applications | ERP owns; pro_manage mirrors as `Contract` and adds contributor scoping | pro_manage owns `Contract`; values from ERP commitments |
| Change orders | ERP owns the record and its approval; pro_manage mirrors, measures cycle time from ERP timestamps, and adds Jiminy guidance on open | pro_manage owns (`§08`) |
| Actuals, commitments, invoices, purchase orders, receipts | ERP owns (both profiles) | ERP owns |
| Procurement expediting status and site-need dates (`ProcurementItem`) | pro_manage owns; PO and receipt references mirrored from the ERP | pro_manage owns |
| Document register, revision status, transmittals | pro_manage owns; an EDMS or ERP document module can be mirrored in via Doci | pro_manage owns |
| CAR (staged), estimate artifacts, gate workflows, design reviews, MP information, work-package readiness, RFIs, submittals, ITPs, deviations, project losses, health reviews, Benefits Realization, audit | pro_manage owns (both profiles) | pro_manage owns |
| Authorized budget after CAR approval | pro_manage approves; released into the ERP via `ReleaseBudget` (`budget_release` tier) | pro_manage owns |
| Opportunities, screening, capital plan lines and ranking | pro_manage owns; approved plan lines may release to the ERP capital budget as placeholders | pro_manage owns |

Mirrored records are read-only in pro_manage except for pro_manage-only attributes. Conflicts resolve to the system of record, and drift between the mirror and the source is surfaced, not silently overwritten.

## Multi-store map

| Store | Lives in | Holds | Written by | Read by |
|---|---|---|---|---|
| **Postgres** (pro_manage) | Docker | All transactional domain (`Opportunity`, `CapitalPlan`, `Site`, `Project`, members, ACLs, budgets, estimates, CARs, schedule, scope, risk, docs metadata, document register and revisions, transmittals, procurement items, workflows, templates, assets, loss categories, business-case promises, health reviews, contracts, work packages, RFIs, submittals, change orders, ITPs, deviations, audit, outcomes) | Go backend | Go backend; Python sidecar for analytics/exports |
| **TimescaleDB** (pro_manage) | Docker | Metric readings, loss events, project loss events | Metrici / HSEi connectors, Python sidecar for derived metrics | Go backend, dashboards |
| **Neo4j (MDEMG)** | Docker | Observations, constraints, outcomes graph, decisions, concepts | pro_manage via `/v1/conversation/observe`; MDEMG internal | pro_manage via `/v1/jiminy/*` |
| **TimescaleDB (MDEMG)** | Docker | MDEMG metrics and effectiveness telemetry | MDEMG internal | Platform operators (Grafana) |
| **Redis** (pro_manage) | Docker | Session, cache, rate-limit, warmed-guidance snapshots | Go backend | Go backend |
| **S3-compatible object store** (MinIO) | Docker (or external bucket) | Document binaries, export artifacts (PDF/PPTX), signed envelopes | Go backend | Go backend; signed URLs to authenticated users |
| **External systems** (via connector modules) | Off-cluster | Source-of-truth for actuals (ERPi), schedules (Schedulei), docs (Doci), identities (IdPi), signatures (Sigi), notifications (Commsi), raw metrics (Metrici), HSE / MES / OEE-and-loss signals and abnormality tags (HSEi), in-service assets and failure history (CMMSi) | Those systems | pro_manage connectors |

## Identifiers and versioning

- Primary keys are UUIDv7 (time-ordered). Human-readable codes (e.g. `Project.code`) are indexed, unique within tenant, and are *not* primary keys.
- Every mutable row carries a `version` (monotonic int) and participates in optimistic-concurrency updates (`UPDATE … WHERE version = :expected`). Conflicts raise typed errors.
- "Soft delete" is modeled as a terminal lifecycle state (`Archived`), not a deleted flag; the audit log and reporting continue to resolve historical state.

## Lifecycle state machine (Project)

```
 ┌──────────┐ concept approved ┌──────┐ full CAR approved +┌─────────┐ CQV complete,    ┌─────────┐
 │ Initiate ├─────────────────▶│ Plan ├───────────────────▶│ Execute ├─────────────────▶│ Monitor │
 └──────────┘ (FEL1 → FEL2)    └──────┘ basic design review └─────────┘ first production └────┬────┘
                              (FEL3)   sign-off (§08)                                        │ Early Management
                                                                                             │ exit (§08)
                                                                                             ▼
                       ┌──────────┐  Benefits Realization review closed   ┌───────┐
                       │ Archived │◀──────────────────────────────────────┤ Close │
                       └──────────┘  (or window waived, audited override) └───────┘
```

Transitions only move forward; there is no toggling between states. Each arrow is a gate with its own workflow in `§08`.

- **Before Initiate:** an `Opportunity` is screened and placed on a `CapitalPlan` line; converting it creates the project at Initiate with the opportunity's promises carried in as `proposed`. Projects may also be created directly (compliance obligations, emergencies) and are then back-linked to a plan line or flagged as unplanned on the plan's variance.
- **Initiate:** Concept / planning (FEL1 → FEL2). Owner only at first. Business case drafted, MP information gathered, Class 5 estimate. A `fel_funding` CAR may be raised here to pay for FEL2 and FEL3 work. Edits unrestricted; no audit chain required for exploratory data.
- **Plan:** Front-end engineering (FEL3). Team invited; basis of design, basic design (FEED) and its `basic` design review, Class 3 estimate as an `EstimateArtifact`, business-case promises baselined, long-lead items identified. Writes audit. Ends with the `full` CAR; its approval and the basic design-review sign-off gate the move to Execute.
- **Execute:** Detailed engineering, procurement, fabrication, installation, and commissioning against the authorized baselines. Detailed design reviews gate documents going IFC; readiness gates package release; expediting tracks long-lead items to the site-need date. Writes audit *and* enforce the approval gates in `§08` (IFC issue, change orders, package release, hold points, CQV, supplemental CARs). This is where most project time is spent.
- **Monitor:** Early Management Period. The asset is producing, with extra monitoring; the project still owns it. Punchlist and turnover work continue. Exit only through the Early Management exit workflow (startup curve, punchlist, turnover package, certification).
- **Close:** Turnover delivered to operations and maintenance; Benefits Realization window open. Writes blocked except for allowed corrections (with elevated approval); reads unrestricted.
- **Archived:** Read-only, after the Benefits Realization review closes (or the window is waived with an audited override). Visible only to those with historical access rights or to compliance roles. Deletion only via an explicit admin workflow with full audit.

## Open items for Section 03

- Confirm whether a **Program** layer (parent of multiple related Projects) is needed in year one, or if the Portfolio layer is enough. A multi-site effort (the same scope delivered at two sites) is the strongest case for it, since a project has one site.
- Confirm the company's document numbering scheme and revision-code convention (letters before IFC, numbers after is the common one) so the register and `revision_code` validation match what the EPCM already issues.
- Confirm whether the EPCM runs an EDMS (Aconex, ProjectWise, or similar) that should be mirrored through Doci rather than having contributors raise transmittals in pro_manage directly.
- Confirm the company's fiscal year, the capital plan cycle calendar (submission, review, approval dates), and the hurdle rates (IRR, payback) so the screening score and `capital_plan.*` policy variables start with real values.
- Confirm the probability bands behind the 1 to 5 likelihood scale for risk quantification, and whether Finance wants contingency expressed as expected value or a P-value (the year-one arithmetic supports expected value; P50/P80 need a simulation that is out of scope).
- Define the exact set of `MetricDefinition.category` values and their nested sub-categories (how granular do we go? Per-ISO-metric, per-industry-metric?).
- Decide whether `BudgetActual` and `MetricReading` share a storage layer (one TSDB) or stay separated. First instinct: separate, because actuals have strict financial-period semantics; metric readings are continuous.
- Decide on the schema contract between pro_manage and `ERPi` for actuals (common chart of accounts? WBS mapping table? Both?). Depends on which ERP comes first (pending owner confirmation).
