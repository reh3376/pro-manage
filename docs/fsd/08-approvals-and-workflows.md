# 08: Approvals, Workflows & Audit

## Decisions on record

- **2026-10-13:** Approvals are a core feature. pro_manage owns the workflow engine; MDEMG provides advisory + classify at the gates (`§06`).
- **2026-10-14:** **All thresholds are admin-configurable policy variables, never hard-coded,** with values that can differ per project. Most specific scope wins (project → portfolio → project type → global); values are locked into each workflow when it starts; project overrides are set only by `platform_admin` or `policy_owner`, never by anyone in that project's approval chain.
- **2026-10-13:** The primary approval instance in a TPM shop is the **CAR (Capital Authorization Request)**. "CapEx" is retained as the generic framing.
- **2026-10-13:** Workflow kinds in year one: **CAR, Baseline change, Design review sign-off, CQV sign-off, Milestone sign-off, Change request, Shutdown & tie-in plan, Early Management exit, Closeout, Benefits Realization review**, plus a configurable **Custom** kind for policies pro_manage doesn't ship with.
- **2026-10-14:** CAR submission requires every business-case promise to carry a CAR-time baseline and measurement basis. Early Management exit requires operator and maintenance certification on the new asset. A recurring **Project health review** (not a gate) is added.
- **2026-10-14:** EPCM administration workflows added: **RFI response, submittal review, change order, work package release, hold point release**. Each records cycle time; delays become project-loss events. Design reviews add constructability, commissionability, and safety-in-design lenses with a Construction Manager stage. Health reviews select one dominant project loss from data.
- **2026-10-13:** Pre-authorization workflow enforces **FEL stage** progression (FEL1 → FEL2 → FEL3) and **AACE Estimate Class** gates: Class 5 at concept, Class 3 required at CAR submission, Class 1 required at execution kickoff. Violations block submit (Tier A fail-closed).
- **2026-10-13:** Formal e-signature via `Sigi` (DocuSign at launch). Workflow kinds for which Sigi is mandatory are configurable per policy; the rest rely on the authenticated AuditEvent as the sign-off of record.
- **2026-10-13:** AuditEvents are append-only with chained signed hashes (tamper evidence). Retention aligned with the longest applicable regime (`§11`); baseline is 7 years for financial-impacting events.
- **2026-10-13:** Routing resolves approvers from **eligibility pools** by **amount thresholds + delegation chain**. Pools are populated by platform roles (`finance_approver_eligible`, etc.) + per-project overrides.

## Policy variables (configurable thresholds)

No threshold, amount, percentage, SLA, cadence, or window in any workflow is hard-coded. Each is a named **policy variable** that administrators set and can override per project. The numbers in the example tables below are placeholder defaults, written as `{variable}` references.

### How a variable is defined

- `PolicyVariable`: `key` (e.g. `car.threshold.vp_finance`), `type` ∈ `{money, percent, duration, integer, boolean, enum}`, `unit` / currency, `description`, `global_default`, `allowed_range` (min and max, enforced at every scope), `overridable_at[]` (which scopes may override it), `owner_role`.
- `PolicyValue`: `variable_key`, `scope_kind` ∈ `{global, project_type, portfolio, site, project}`, `scope_id`, `value`, `effective_from`, `effective_to?`, `set_by`, `reason`, `approved_by?`, version.

### How the effective value is resolved

Most specific scope wins: **project → site → portfolio (business unit) → project type → global default**. Site scope exists because approval limits, SLAs, and calendars differ by place (a site in another country keeps its own currency, holidays, and regulatory sign-offs) without every project there needing an override. Every screen that uses a variable can show its effective value and where it came from (for example "$7.5M, project override set by J. Smith on 2026-11-02, reason: JV partner agreement").

### When values are locked

A workflow captures the effective values of every variable it uses when it **starts** (for a CAR, at submission) and records them in `ApprovalWorkflow.policy_snapshot`. Changing a variable later does not change an approval that is already in flight. An administrator can deliberately re-base an in-flight workflow onto current values; that is a logged action with a reason, and any stage whose routing changes is re-opened.

### Who can change them

- `platform_admin` and the `policy_owner` platform role (typically Finance controls) set global, project-type, and portfolio values.
- Project-level overrides are set by the same roles only, never by the project's owner or by anyone in that project's approval chain (separation of duties).
- A project override needs a reason and an effective date, and may carry an expiry. Whether a second approver is required is itself a variable (`policy.override.second_approval_required`, default true for money and percent variables).
- Every change is versioned and written to the AuditEvent log. Overrides that would remove an approval stage entirely, or that sit at the edge of `allowed_range`, are flagged for review.

### Where variables are used

Approval-chain conditions (a stage is included when `amount >= {car.threshold.vp_finance}`), stage SLAs, MDEMG fail-closed tiers (`§06`), health-review cadence, Early Management exit criteria, the Benefits Realization window, change-order thresholds, and readiness overrides. Jiminy constraints are written against the variable rather than a number ("A CAR at or above the VP Finance threshold requires VP Finance co-sign"), and pm-core passes the project's effective values into the guidance context, so one constraint works for every project.

### Starter variables (placeholder defaults until the company's approval-limits matrix is loaded)

| Variable | Type | Placeholder default | Used by |
|---|---|---|---|
| `car.threshold.finance_approver` | money | $0 (always) | CAR stage 2 |
| `car.threshold.vp_finance` | money | $5M | CAR stage 3; MDEMG Tier A |
| `car.threshold.cfo` | money | $25M | CAR stage 4 |
| `car.threshold.ceo` | money | $100M | CAR stage 5 |
| `car.threshold.basis` | enum: `tic`, `tic_plus_contingency`, `total_project_cost` | `tic` | What the CAR amount is compared on |
| `car.threshold.currency` | enum: ISO 4217 | company reporting currency | Currency the thresholds are expressed in; requests convert at `fx.rate.*` on submission |
| `car.fel_funding.required_aace_class` | integer | 5 | Minimum estimate class behind a FEL-funding CAR |
| `car.fel_funding.cap_pct` | percent | 5% | FEL-funding CAR cap as a share of the current estimate |
| `car.full.required_aace_class` | integer | 3 | Minimum estimate class behind the full CAR |
| `car.full.requires_basic_design_review` | boolean | true | A closed `basic` design review with no open high findings before the full CAR |
| `car.supplemental.reapproval_pct` | percent | 10% | Cost growth on an approved CAR that forces a supplemental |
| `fx.rate.<from>.<to>` | decimal (dated) | per Finance | Currency conversion for thresholds and roll-ups |
| `capital_plan.fiscal_year_start` | enum: month | per Finance | Fiscal year the plan cycle runs on |
| `capital_plan.submission_deadline` | date (annual) | per Finance | Last day opportunities can be proposed for the coming year's plan |
| `capital_plan.hurdle.payback_years` | decimal | 3.0 | Screening hurdle for `cost_reduction` and `growth` opportunities (mandatory and compliance are exempt) |
| `capital_plan.hurdle.irr_pct` | percent | per Finance | Alternative or additional hurdle |
| `capital_plan.screening_weights` | JSON | benefit 40, cost 20, urgency 20, strategic fit 10, risk 10 | Weights behind `Opportunity.screening_score` |
| `capital_plan.approval.threshold_committee` | money | per DoA | Envelope above which the capital committee stage applies |
| `adoption.car_only_from` | date | unset | From this date a CAR is only approvable in pro_manage (`§16`) |
| `adoption.health_review_only_from` | date | unset | From this date high-value health reviews are recorded here |
| `adoption.status_deck_sunset` | date | unset | Legacy status deck retired for Governed and Full projects |
| `onboarding.required_level.<band>` | enum: `registered`, `governed`, `full` | `governed` above `{car.threshold.finance_approver}`, `full` above `{car.threshold.vp_finance}` | Onboarding level a project must reach by amount band (`§16`) |
| `risk.quantify_above_score` | integer | 12 | Risks at or above this score must carry three-point cost and schedule impacts |
| `contingency.coverage_warn_below` | decimal | 1.0 | Health-review flag when contingency remaining falls below the expected value of open risks |
| `document.ifc_requires_detailed_review` | boolean | true | A document cannot go IFC without a closed `detailed` design review for its scope |
| `procurement.float_warn_days` | integer | 14 | Delivery float below this flags the dependent package |
| `car.esign.required_above` | money | $1M | DocuSign required vs internal attestation |
| `baseline.variance.reforecast_pct` | percent | 10% | Jiminy warning; monthly re-forecast task |
| `baseline.variance.finance_pct` | percent | 5% | Baseline change stage 2 |
| `baseline.variance.vp_finance_pct` | percent | 10% | Baseline change stage 3; MDEMG Tier A |
| `change_order.threshold` | money | per contract | Change order stage 3; MDEMG Tier A |
| `mdemg.tier.<action_kind>` | enum: `A`, `B`, `C` | per the table in `§06` | Which MDEMG fail-closed tier each governed action kind runs at |
| `mdemg.classify.timeout_ms` | integer | 10 000 | Per-call budget for `/jiminy/classify`; a timeout counts as "unreachable" for the tier |
| `health_review.cadence` | duration | monthly | Health review |
| `health_review.high_value_threshold` | money | $25M | Biweekly cadence above this |
| `early_mgmt.exit.nameplate_pct` | percent | 95% | Startup curve exit criterion |
| `early_mgmt.exit.sustain_days` | integer | 14 | Startup curve exit criterion |
| `benefits.window_days` | integer | 365 | Benefits Realization window |
| `sla.<workflow>.<stage>` | duration | per example tables | Stage SLAs and nudges |

Administrators can add variables; workflow and constraint definitions reference them by key.

## Workflow state machine

Each `ApprovalWorkflow` instance moves through:

```
                                   (user submits)
                              ──────────────────────────▶
                     ┌────────────────┐             ┌───────────────┐
    (draft optional)  │ draft (local) │ ──submit──▶ │   in_flight   │
                     └────────────────┘             └───────┬───────┘
                                                            │
                                                            │ stage N decision
                                                            │
                              ┌─────────────────────────────┤
                              │                             │
                           reject                        approve (last stage)
                              │                             │
                              ▼                             ▼
                       ┌────────────┐                ┌────────────┐
                       │  rejected  │                │  approved  │
                       └────────────┘                └────────────┘
                       │ (owner may                 │ (immutable; signed
                       │  revise + resubmit         │  artifact stored
                       │  → new workflow id)        │  via Sigi if applicable)
                       │
                       │
                     cancel / expire (SLA breach + policy)
                              │
                              ▼
                       ┌────────────┐    ┌───────────┐
                       │ cancelled  │    │  expired  │
                       └────────────┘    └───────────┘
```

- `in_flight` is further qualified by `current_stage`.
- `expired` is set by the SLA tick when the final SLA deadline passes without decision.
- `cancelled` is initiator- or owner-driven; captured with reason.

## Stage model

An `ApprovalWorkflow` has an ordered list of `ApprovalStage` rows:

- `stage_id`, `workflow_id`, `order`, `role_required`, `assignee_pool` (resolved at stage entry), `assignees[]` (specific people when known), `sla_duration`, `entered_at`, `completed_at?`, `completed_by?`, `decision ∈ {approve, reject, delegate}`, `comment?`, `guidance_id?`, `classification_verdict?`, `signed_envelope_ref?`.
- When a stage completes with `approve`, the workflow advances to the next stage; the final stage approving transitions the workflow to `approved`.
- `delegate` reassigns the stage to another eligible approver without changing state (logged as such).
- `reject` terminates the workflow.

### Example: Opportunity screening (gate 0)

Not a money gate; the decision to spend attention. Runs continuously, with a cut-off at `{capital_plan.submission_deadline}` for the coming year's plan.

| Order | Role required | SLA | Gate |
|---|---|---|---|
| 1 | `sponsor` (any owner-eligible user) proposes | n/a | One-page case: problem, origin and source (the priced loss, the obligation, the asset), order-of-magnitude cost, benefit kind and estimate, urgency |
| 2 | `pmo_lead` or `portfolio_executive` screens | 10 business days | `/jiminy/guide` surfaces Benefits Realization history and MP information for the asset class; the screening score is computed from `{capital_plan.screening_weights}`; hurdle per `{capital_plan.hurdle.*}` unless `urgency = mandatory` or origin is a compliance obligation |

Outcomes: `shortlisted` (eligible for the plan), `deferred` (with a revisit date), `rejected` (with reason). Rejections and deferrals are written to the project-less portfolio space as `decision` observations so the same idea proposed again next year meets its own history.

### Example: Capital plan approval (annual, with mid-year revisions)

| Order | Role required | Pool | SLA |
|---|---|---|---|
| 1 | `pmo_lead` assembles the draft plan: shortlisted opportunities ranked, continuing projects carried forward, envelope by portfolio | n/a | per cycle calendar |
| 2 | `portfolio_executive` approves the portfolio's lines | configured per portfolio | 10 business days |
| 3 | `finance_approver` (controller) validates the envelope, cash phasing, and carry-over | `finance_approver_eligible` | 10 business days |
| 4 (if envelope ≥ `{capital_plan.approval.threshold_committee}`) | `capital_committee` | named pool (CFO, COO, Capital Engineering leader, others per DoA) | per cycle calendar |

On approval: planned opportunities move to `planned`; in the ERP-led profile, lines release to the ERP capital budget as placeholders. A mid-year revision creates a new plan version with `supersedes_plan_id` and runs the same chain on the changed lines only. Converting a planned opportunity into a project is an owner action (UC-25) that needs no further approval; its first money comes through a CAR (`fel_funding` or `full`) against the plan line, and the plan shows planned vs authorized vs forecast per line.

### Example: CAR approval chain (year-one defaults, admin-tunable)

The CAR is staged (`§03 CapitalAuthorization`). Pre-conditions depend on `kind`:

| Kind | When | Pre-conditions for submission | Routing amount |
|---|---|---|---|
| `fel_funding` | Initiate or Plan, to pay for FEL2 and FEL3 work | `EstimateArtifact.aace_class ≤ {car.fel_funding.required_aace_class}` (Class 5 or better); amount ≤ `{car.fel_funding.cap_pct}` of the current estimate; a business case narrative (promises may still be `proposed`) | The request amount |
| `full` | End of Plan (FEL3) | `project.fel_stage == fel3`; `EstimateArtifact.aace_class ≤ {car.full.required_aace_class}` (Class 3 or better) with a basis-of-estimate document; approved Basis of Design; a `basic` design review closed with no `high`-severity findings open (when `{car.full.requires_basic_design_review}`); long-lead items identified as `ProcurementItem` rows; every `BusinessCasePromise` in `committed` state with a captured baseline and measurement basis. A CAR whose benefits cannot be measured later cannot be submitted now | The request amount |
| `supplemental` | Execute, when forecast cost growth exceeds `{car.supplemental.reapproval_pct}` or a change needs money | Names the CAR it supplements; a current EAC and the change requests or materialized risks that explain the growth | The **cumulative** authorized amount, so thresholds cannot be crept past in steps |

Detailed design reviews are not a CAR pre-condition: under EPCM, detailed engineering is paid for by the full CAR and happens in Execute, where `detailed` reviews gate documents going IFC (see "Document issue for construction" below).

| Order | Role required | Pool | SLA |
|---|---|---|---|
| 1 | `project_owner` | the project's Owner | 24h |
| 2 | `finance_approver` | `finance_approver_eligible` ∩ the project's site or portfolio, per the DoA | 48h |
| 3 (if amount ≥ `{car.threshold.vp_finance}`) | `vp_finance` | configured VPs | `{sla.car.vp_finance}` (72h) |
| 4 (if amount ≥ `{car.threshold.cfo}`) | `cfo` | configured CFO + 1 delegate | `{sla.car.cfo}` (72h) |
| 5 (if amount ≥ `{car.threshold.ceo}`) | `ceo` | configured CEO + 1 delegate | `{sla.car.ceo}` (7 days) |

Thresholds and SLAs are policy variables (see Policy variables above); the amount is compared on the basis set by `{car.threshold.basis}`, in `{car.threshold.currency}`, converted at the rate in force on submission and recorded in the policy snapshot. SLA clocks run in business days of the project's site calendar. Stage definitions and roles are stored with the workflow kind and are editable by `platform_admin` with a logged change.

**On final approval (ERP-led profile):** pm-core calls `ERPi.ReleaseBudget` to create or release the authorized budget in the ERP, keyed on the CAR id so retries are safe. The ERP's budget reference is stored on the CAR and in the AuditEvent. If the release fails, the CAR stays `approved` with a visible "budget release pending" flag and an operational alert; nobody re-approves.

### Example: Design review sign-off

The same chain serves every review stage; what the sign-off unlocks differs: a `basic` review is a pre-condition of the full CAR and of the `Plan → Execute` transition; a `detailed` review is a pre-condition of its documents going IFC; a `post_change` review follows a change that touches equipment.

| Order | Role required | SLA | Jiminy coupling |
|---|---|---|---|
| 1 | `facilitator` closes the review | at session+24h | `/jiminy/guide` surfaces MP-information from past assets of same equipment type |
| 2 | `operator_representative` + `maintenance_representative` acknowledge | 48h | n/a |
| 2b | `construction_manager` acknowledges constructability + commissionability findings | 48h | `/jiminy/guide` surfaces project-loss lessons (e.g. past submittal or interface problems) for this asset class |
| 3 | `project_owner` approves | 48h | `/jiminy/evaluate` on open `high`-severity findings; must be dispositioned |

### Example: CQV sign-off (per equipment / per batch)

| Order | Role required | SLA |
|---|---|---|
| 1 | `commissioning_engineer` closes the record (pass / pass-with-punchlist / fail) | at session+24h |
| 2 | `operator_representative` co-sign on `pass_with_punchlist` | 48h |
| 3 (regulated contexts only) | `qa_lead` final sign-off | 72h |

Fail routes back to engineering with a change request. Punchlist items block Early Management exit until closed.

### Example: Shutdown & tie-in plan (brownfield)

Required before any Execute-stage tie-in against a running plant. Captures the plan, operations' acknowledgment, and the restart criteria.

| Order | Role required | SLA |
|---|---|---|
| 1 | `project_owner` presents the plan | 48h |
| 2 | `site_operations_lead` acknowledges + confirms production impact | 48h |
| 3 | `site_maintenance_lead` acknowledges | 48h |
| 4 | `finance_approver` if lost production > threshold | 72h |

### Example: Early Management exit

Project cannot move from `Monitor` to `Close` without this.

| Order | Role required | SLA | Exit criteria |
|---|---|---|---|
| 1 | `project_owner` requests exit | n/a | Startup curve sustained at ≥ `{early_mgmt.exit.nameplate_pct}` of nameplate for ≥ `{early_mgmt.exit.sustain_days}` days; CQV punchlist closed; TurnoverPackage.status == `approved`; **operators and maintenance technicians assigned to the asset certified** on its standard work, CIL routines, and changeovers (status read from the plant skills matrix / connected-worker platform via HSEi where available, else recorded in pro_manage) |
| 2 | `site_operations_lead` co-sign | 48h | n/a |
| 3 | `site_maintenance_lead` co-sign | 48h | n/a |
| 4 | `compliance_officer` co-sign for regulated contexts | 72h | n/a |

### Example: Benefits Realization review

Fired at `BenefitsRealization.window_closes_at`.

| Order | Role required | SLA |
|---|---|---|
| 1 | `project_owner` compiles variance report | 14d |
| 2 | `finance_approver` validates financial variance | 7d |
| 3 | `portfolio_executive` approves closure | 14d |

Outcome drives the project's final `Archived` transition and feeds a `decision` observation to MDEMG governance space with scope tags for similar future projects.

### Example: Document issue for construction (IFC)

| Order | Role required | SLA | Gate |
|---|---|---|---|
| 1 | `discipline_engineer` marks the revision ready for IFC | n/a | A `detailed` design review covering the document is closed with no open `high` findings (when `{document.ifc_requires_detailed_review}`); every RFI the revision answers is linked |
| 2 | `document_controller` issues the transmittal (`purpose = for_construction`) to the contracts that need it | 2 business days | Register entry updated; prior revision marked `superseded` |

Receiving contributors acknowledge the transmittal; unacknowledged transmittals past `response_due_at` nudge via Commsi. A planned IFC date that slips past a package's planned start starts accruing `waiting_information` loss against that package.

### Example: Procurement expediting (recurring, not a gate)

Not an approval; a cadence. The `procurement_lead` reviews every open `ProcurementItem` with `delivery_float_days < {procurement.float_warn_days}` at the health-review cadence, records expediting notes, and re-forecasts. A forecast that moves past `required_on_site_at` raises a readiness warning on the dependent package and, if the package is already released, a `waiting_material` loss. Purchase orders are placed and receipts recorded in the ERP; pro_manage mirrors the references.

### Example: RFI response

| Order | Role required | SLA |
|---|---|---|
| 1 | `discipline_engineer` answers | per policy (default 5 business days; 2 if the package is blocked) |
| 2 (if the answer changes scope or cost) | `project_owner` acknowledges; spawns a ChangeRequest / ChangeOrder | 48h |

Overdue RFIs nudge via Commsi; blocked-package days accrue as `waiting_information` losses.

### Example: Submittal review

| Order | Role required | SLA |
|---|---|---|
| 1 | `discipline_engineer` reviews and dispositions (approved / approved-as-noted / revise-resubmit / rejected) | per contract (default 10 business days) |
| 2 (vendor data for turnover-critical assets) | `maintenance_representative` reviews maintainability data | 5 business days |

Revise-resubmit cycles are counted per submittal; repeat offenders by supplier or asset class become MP / quality lessons at closeout.

### Example: Change order (contractual)

| Order | Role required | SLA |
|---|---|---|
| 1 | `construction_manager` validates scope and quantum | 5 business days |
| 2 | `project_owner` approves | 48h |
| 3 (if cost ≥ contract threshold or contingency would go negative) | `finance_approver` | 72h |

Approved change orders update the `Contract` value and, where they implement an internal `ChangeRequest`, close it.

**ERP-led profile:** the change order is created and approved in the ERP, not here. pro_manage mirrors it, shows Jiminy guidance when it is opened in pro_manage, links it to the internal `ChangeRequest`, and measures its cycle time from the ERP's timestamps (feeding the `waiting_approval` project loss). The chain above applies only in the pro_manage-led profile.

### Example: Work package release

| Order | Role required | SLA | Gate |
|---|---|---|---|
| 1 | `construction_manager` releases | n/a | All readiness constraints met (`§03 WorkPackage`), or an audited override with reason; override is Tier A in `§06` |
| 2 | Contractor `contributor` acknowledges release | 24h | n/a |

### Example: Hold point release

| Order | Role required | SLA |
|---|---|---|
| 1 | `inspector` named in the ITP verifies criteria with evidence (photos, test results) | per ITP |

Work on the package cannot progress past the hold point until released. A failed inspection opens a non-conformance and a `rework` loss.

### Example: Project health review (recurring, not a gate)

The capital-side counterpart to an operations huddle: short, regular, ends with owned actions rather than a grade. Records a `ProjectHealthReview` (`§03`).

- **Cadence:** monthly by default during `Plan` and `Execute`; biweekly for projects at or above `{health_review.high_value_threshold}` or flagged `red` (cadence set by `{health_review.cadence}`); on demand by a Portfolio Executive.
- **Facilitator:** someone other than the project owner (peer PM, portfolio lead, or CI lead).
- **Agenda (pre-filled from live data):** scope stability, estimate maturity vs stage (AACE class vs FEL/lifecycle stage), critical-path float, top risks and owners, contingency coverage (remaining contingency vs the expected value of open quantified risks; flag below `{contingency.coverage_warn_below}`), change-order trend, contingency burn vs % complete, IFC on-time and documents holding packages, long-lead items with negative or thin delivery float, Operations/Maintenance readiness for turnover, the project-loss Pareto (from which the review selects one **dominant loss** to attack this cycle, with an owner and a countermeasure), RFI / submittal aging, plan compliance with variance reasons, and one open prompt: *"What are you worried about that isn't on the register?"*
- **Output:** actions with owners and dates (tracked like issues), an overall green/amber/red, and an optional `/jiminy/feedback`-style observation to the project space so recurring concerns become visible guidance.
- **No approval chain:** it is a review, not a gate. A `red` result notifies the Portfolio Executive via Commsi.

### Example: Baseline change

| Order | Role required | Pool | SLA | Jiminy coupling |
|---|---|---|---|---|
| 1 | `project_owner` | project Owner | 24h | `/jiminy/guide` on open ⇒ warn if variance > `{baseline.variance.reforecast_pct}` |
| 2 (if variance > `{baseline.variance.finance_pct}`) | `finance_approver` | per BU | 48h | n/a |
| 3 (if variance > `{baseline.variance.vp_finance_pct}`) | `vp_finance` | configured | 72h | `/jiminy/classify` is Tier A (fail-closed) |

### Example: Closeout

Enforces the "closeout plan on file" constraint (from the Governance space). Final stage is `project_owner` + `compliance_officer` co-sign. Milestone-dependency rules enforced in-process beforehand.

**Capitalization handoff (pre-condition, ERP-led profile).** Before the final stage, Finance confirms that the project's asset-under-construction balance has been settled to fixed assets in the ERP: pm-core reads `ERPi.FetchCapitalizationStatus` and the closeout checklist shows settled vs outstanding; a `finance_approver` stage (`capitalization_confirmed`) sits before the co-sign and cannot complete while the outstanding balance exceeds `{closeout.capitalization.max_outstanding}` (policy variable, default $0) unless deferred with a reason and a date. pro_manage never performs the settlement; it proves it happened. In the pro_manage-led profile the stage is a Finance attestation with the fixed-asset references typed in.

## Routing & assignment

- When a stage is entered, pm-core resolves the `assignee_pool`:
  - Base pool from `role_required` + optional filter (business unit, portfolio, project-level override list).
  - **Delegation chain:** each eligible user may declare an active delegate (with duration). pm-core walks the chain to resolve a live assignee set.
  - **Out-of-office:** users may mark themselves OOO for a window; pm-core prefers non-OOO assignees. If none available, falls through to delegates.
  - **Workflow-specific overrides:** the Project Owner may name a specific approver on a specific workflow (within the eligible pool), e.g., "escalate to VP-Finance-North for this project."
- If the pool resolves to **empty**, the workflow pauses with a visible blocker and alerts Platform Admin via Commsi. SLA tick is paused.
- If the pool resolves to **multiple**, pm-core notifies all of them; **first to decide wins** (approve or reject). On delegate, the assignee set updates.

## SLA, nudges, auto-escalation

- Each stage has `sla_duration`. pm-core writes `stage_due_at = entered_at + sla_duration`.
- A pm-workers tick scans stages: at `75% elapsed` → nudge via Commsi; at `100%` → final nudge + flag the workflow as "overdue" in dashboards; at policy-configured `overdue_grace` → auto-escalate to the next tier in the delegation chain, or mark `expired`.
- Expiration policy per workflow kind:
  - CAR: auto-escalate once, then expire if still undecided.
  - Baseline change: expire; owner must re-initiate.
  - Milestone sign-off: expire; sign-off gets re-queued on next cycle.
  - Closeout: no expiration; just stays overdue until decision.

## E-signature coupling (Sigi)

For workflow kinds where formal e-sig is mandatory (CAR ≥ threshold, Change requests touching contractual terms, Closeout):

- At stage entry for the signing stage, pm-core calls `Sigi.CreateEnvelope(spec)` with the subject (budget baseline + diff, change request text, closeout certificate).
- `assignees` of the stage are mapped to Sigi recipients.
- pm-core stores `signed_envelope_ref` on the stage; polls `GetEnvelopeStatus` + listens for Sigi webhooks (per `§05` DF-01).
- Sign event: pm-core marks the stage decided, advances or completes the workflow.
- Decline / voided: pm-core marks the stage rejected or the workflow cancelled; captures the Sigi event reason.
- The **signed PDF** is downloaded via `Sigi.DownloadSignedArtifact` and stored in MinIO; `AuditEvent` captures the storage key + SHA-256 so the artifact survives independent of Sigi's retention.

For workflow kinds **without** formal e-sig, the authenticated AuditEvent (session, user, before/after, timestamp) is the sign-off of record.

## MDEMG coupling at every gate

Per `§06`:

- **On stage render** (approver views the stage): pm-core calls `/jiminy/guide` × 2 (project + governance spaces), merges, role-filters, renders cards.
- **On stage submit** (approver clicks Approve / Reject):
  - If the workflow kind × amount tier is Tier A (`mdemg.tier.<action_kind>`) → `/jiminy/classify` × 2 (governance and project, in parallel), or the cached pre-classified verdict for an unchanged payload. Any `deny` wins; blocks submit unless admin override.
  - If Tier B → `/jiminy/evaluate` (or just `/guide` re-read) → warning banner, allow submit.
  - If Tier C → no call.
- **After decision** (committed to Postgres with AuditEvent): pm-core enqueues `/jiminy/feedback` to the outbox with `outcome ∈ {followed | ignored_with_reason | blocked_true_positive | blocked_false_positive | overridden_by_admin}`.

Every stage record carries `guidance_id`, `classification_verdict`, and a snapshot (`guidance_items_snapshot`) in the sibling `GuidanceOutcome` row.

## Admin override

- Available only when a Tier A classify returned `deny` and the approver (or Platform Admin) believes the deny is wrong, or when MDEMG is unreachable and a Tier A action must proceed.
- Requires: Platform Admin re-authentication; explicit reason text (free-form, logged); link to the approving stage.
- Produces its own AuditEvent (`action_type = admin_override_classify_deny`) and a `/jiminy/feedback outcome=blocked_false_positive` or `=overridden_by_admin`.
- Conspicuous alert via Commsi to a designated audit distribution.

## Audit log

- Append-only; writes happen in the same transaction as the mutation.
- Each row:
  - `audit_id`, `tenant_id`, `project_id`, `actor_user_id`, `action_type` (fine-grained, e.g., `approval.car.stage_approve`), `subject_type`, `subject_id`, `before` (JSON), `after` (JSON), `at`, `source_ip`, `request_id`, `guidance_id?`, `classification_verdict?`, `signed_hash`
  - `signed_hash = HMAC-SHA256(prev_hash || row_canonical_payload, audit_chain_key)`: a per-tenant chain so tampering is detectable without an external ledger.
  - A nightly job verifies the chain and writes a verification record; mismatches alert immediately.
- Immutable via DB constraints:
  - `BEFORE UPDATE` and `BEFORE DELETE` triggers reject.
  - Postgres role used by pm-core for audit writes has `INSERT`-only on this table.
  - Backup/export pipelines consume audit via a read-only replica.

### Audit retrieval (UC-12)

- Compliance Officers (and time-boxed auditors) query by `(project_id, action_type, from, to)`. Response includes referenced artifacts: Sigi signed envelopes, MDEMG guidance snapshots, before/after payloads.
- Query results are read-only, time-stamped at retrieval, and themselves logged (meta-audit): "who looked at what approval chain and when."

## Edge cases

- **Approver leaves the company mid-workflow:** `IdPi` deprovision marks the user `is_active=false`. pm-core re-resolves the pool and reassigns via delegation chain. If the user had an in-flight Sigi envelope, pm-core voids and re-issues.
- **Owner changes mid-workflow:** the new owner inherits all open workflows automatically. Stage history preserved under original approvers; new stages route to the new owner's chain.
- **Workflow expires but the business still needs it:** owner reinitiates; new workflow id; audit log links the two.
- **Classify false positive (deny when policy actually allows):** admin override + `blocked_false_positive` feedback; Governance Author reviews the constraint; may re-scope or retire via tombstone.
- **Classify false negative (allow when policy would have denied):** detected post-hoc by audit review; Governance Author authors a new constraint; `missed_violation` feedback captured retrospectively.
- **Delegate approves outside their authority:** pm-core enforces `role_required` at stage completion time, not at assignment time. A delegate who isn't eligible for the required role can't complete the stage.
- **Policy variable change mid-flight:** in-flight workflows run under their **policy snapshot** (effective variable values captured when the workflow started). Changes affect new workflows only, unless an administrator deliberately re-bases the workflow (logged, with reason; affected stages re-open). The snapshot is visible on the workflow.

## UX patterns

- **In-app inbox:** every item that needs a person's action or attention lands in one list: stages awaiting their decision, transmittals to acknowledge, RFIs assigned, hold points to inspect, punchlist items, health-review actions, deviations routed to them, imports to reconcile, and Jiminy escalations. Each row carries its due date, SLA state, project, and a one-click path to the screen; rows clear when the underlying item does, never by dismissal. Commsi notifications link into the inbox rather than duplicating it, and a daily digest summarizes what is open. The inbox is part of the spine (`roadmap`).
- **Workflow dashboard:** a user's view of workflows they initiated, workflows awaiting their decision, workflows they're observing. Grouped by SLA proximity.
- **Approval screen:** on open, pm-core warms Jiminy. Primary view shows the subject (CAR details, baseline diff, closeout checklist). Right rail shows guidance cards. Submit button is masked by the classify result when Tier A deny.
- **Delegate screen:** reassign button → pick from eligible pool filtered by availability.
- **Historical workflow:** read-only reconstruction of every stage, every decision, every guidance item shown, every Sigi event, anchored by `audit_id` and linkable.

## Admin surface

Platform Admin UI exposes:

- **Policy variable editor:** define variables, set global / project-type / portfolio values, and set per-project overrides (with reason, effective date, optional expiry, and second approval where required). Shows each project's effective values and their sources. Changes are versioned and logged.
- Workflow kind editor: stage definitions, roles, and Sigi requirement, with conditions written against policy variables.
- Eligibility pool management: add/remove users from `finance_approver_eligible`, `vp_finance`, etc. Also links to IdPi claim mappings so admins can see "this role is populated by the AD group `pro_manage.finance_approvers`."
- Delegation chain editor: users can edit their own; admins can edit anyone's with audit.
- Workflow kind registry: enable `custom` kinds with declared schemas; admins can wire them to Sigi/MDEMG/tier policy.
- Audit chain verification dashboard: chain-verification success/failure history, nightly job state.

## Open items for Section 08

- **CAR thresholds and chain (owner gathering).** The stage table above is a placeholder. To replace it, collect the company's **Delegation of Authority (DoA) / approval-limits matrix** and its capital policy, specifically:
  - Approval limits by role and amount (who can approve up to what), and whether limits differ by business unit, site, or project type (the policy-variable scopes `portfolio`, `site`, and `project_type` exist for exactly this).
  - Whether FEL funding is authorized separately from the full CAR today (and its cap), or whether the company sanctions once at FEL3; the `fel_funding` kind can be disabled by policy if the latter.
  - What the threshold is measured against: TIC only, TIC + contingency, or total project cost including expensed portions.
  - Supplemental and overrun rules: when a cost increase on an approved CAR needs re-approval (e.g. > 10% or > $X).
  - Board-level thresholds and any capital committee steps.
  - Which approvals require formal e-signature (DocuSign) vs internal attestation.
  - Required CAR content and attachments (business case template, AACE class, IRR/NPV/payback hurdle rates, risk register).
  - Delegation rules when an approver is out (who can sign on their behalf, and up to what limit).
  These become the global and portfolio values of the policy variables; projects that need different limits get project-level overrides. No design change is expected.
- Confirm whether `Change request` is a workflow kind of its own, or always a nested sub-step of a Budget or Scope change. First instinct: standalone, with references into Budget and Scope.
- Decide whether "re-submit after reject" creates a brand-new workflow id or extends the existing one. First instinct: new id, with explicit linkage, so the audit record for each attempt is clean.
- Confirm which workflow kinds are mandatory-Sigi vs. optional-Sigi.
- Confirm whether an approver can approve while on OOO with a dated-override ("I'll approve from vacation this once"). First instinct: yes, with a confirm-OOO dialog that logs the override.
- Decide whether external stakeholders can be approvers on specific workflow kinds (e.g., JV partner co-signs on a CAR). If yes, their IdPi federation path and their eligibility pool model needs attention.
