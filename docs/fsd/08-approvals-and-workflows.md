# 08 — Approvals, Workflows & Audit

## Decisions on record

- **2026-10-13** — Approvals are a core feature. pro_manage owns the workflow engine; MDEMG provides advisory + classify at the gates (`§06`).
- **2026-10-13** — The primary approval instance in a TPM shop is the **CAR (Capital Authorization Request)**. "CapEx" is retained as the generic framing.
- **2026-10-13** — Workflow kinds in year one: **CAR, Baseline change, Design review sign-off, CQV sign-off, Milestone sign-off, Change request, Shutdown & tie-in plan, Early Management exit, Closeout, Benefits Realization review**, plus a configurable **Custom** kind for policies pro_manage doesn't ship with.
- **2026-10-14** — CAR submission requires every business-case promise to carry a CAR-time baseline and measurement basis. Early Management exit requires operator and maintenance certification on the new asset. A recurring **Project health review** (not a gate) is added.
- **2026-10-14** — EPCM administration workflows added: **RFI response, submittal review, change order, work package release, hold point release**. Each records cycle time; delays become project-loss events. Design reviews add constructability, commissionability, and safety-in-design lenses with a Construction Manager stage. Health reviews select one dominant project loss from data.
- **2026-10-13** — Pre-authorization workflow enforces **FEL stage** progression (FEL1 → FEL2 → FEL3) and **AACE Estimate Class** gates: Class 5 at concept, Class 3 required at CAR submission, Class 1 required at execution kickoff. Violations block submit (Tier A fail-closed).
- **2026-10-13** — Formal e-signature via `Sigi` (DocuSign at launch). Workflow kinds for which Sigi is mandatory are configurable per policy; the rest rely on the authenticated AuditEvent as the sign-off of record.
- **2026-10-13** — AuditEvents are append-only with chained signed hashes (tamper evidence). Retention aligned with the longest applicable regime (`§11`); baseline is 7 years for financial-impacting events.
- **2026-10-13** — Routing resolves approvers from **eligibility pools** by **amount thresholds + delegation chain**. Pools are populated by platform roles (`finance_approver_eligible`, etc.) + per-project overrides.

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

### Example — CAR approval chain (year-one defaults, admin-tunable)

Pre-conditions for CAR submission: `project.fel_stage == fel3`, current `BudgetBaseline.aace_class ∈ {3, 2, 1}`, approved Basis of Design, at least one Design Review of `stage=detailed` closed with no `high`-severity findings open, and every `BusinessCasePromise` in `committed` state with a captured baseline and measurement basis (`§03`). A CAR whose benefits can't be measured later can't be submitted now.

| Order | Role required | Pool | SLA |
|---|---|---|---|
| 1 | `project_owner` | the project's Owner | 24h |
| 2 | `finance_approver` | `finance_approver_eligible` ∩ `project.business_unit = X` | 48h |
| 3 (if amount ≥ $5M) | `vp_finance` | configured VPs | 72h |
| 4 (if amount ≥ $25M) | `cfo` | configured CFO + 1 delegate | 72h |
| 5 (if amount ≥ $100M) | `ceo` | configured CEO + 1 delegate | 7 days |

Thresholds and roles are stored in the `WorkflowPolicy` table and are editable by `platform_admin` with a logged change.

### Example — Design review sign-off

Required before transitioning Project from `Plan` to `Execute`.

| Order | Role required | SLA | Jiminy coupling |
|---|---|---|---|
| 1 | `facilitator` closes the review | at session+24h | `/jiminy/guide` surfaces MP-information from past assets of same equipment type |
| 2 | `operator_representative` + `maintenance_representative` acknowledge | 48h | — |
| 2b | `construction_manager` acknowledges constructability + commissionability findings | 48h | `/jiminy/guide` surfaces project-loss lessons (e.g. past submittal or interface problems) for this asset class |
| 3 | `project_owner` approves | 48h | `/jiminy/evaluate` on open `high`-severity findings — must be dispositioned |

### Example — CQV sign-off (per equipment / per batch)

| Order | Role required | SLA |
|---|---|---|
| 1 | `commissioning_engineer` closes the record (pass / pass-with-punchlist / fail) | at session+24h |
| 2 | `operator_representative` co-sign on `pass_with_punchlist` | 48h |
| 3 (regulated contexts only) | `qa_lead` final sign-off | 72h |

Fail routes back to engineering with a change request. Punchlist items block Early Management exit until closed.

### Example — Shutdown & tie-in plan (brownfield)

Required before any Execute-stage tie-in against a running plant. Captures the plan, operations' acknowledgment, and the restart criteria.

| Order | Role required | SLA |
|---|---|---|
| 1 | `project_owner` presents the plan | 48h |
| 2 | `site_operations_lead` acknowledges + confirms production impact | 48h |
| 3 | `site_maintenance_lead` acknowledges | 48h |
| 4 | `finance_approver` if lost production > threshold | 72h |

### Example — Early Management exit

Project cannot move from `Monitor` to `Close` without this.

| Order | Role required | SLA | Exit criteria |
|---|---|---|---|
| 1 | `project_owner` requests exit | — | Startup curve sustained at ≥X% nameplate for ≥Y days; CQV punchlist closed; TurnoverPackage.status == `approved`; **operators and maintenance technicians assigned to the asset certified** on its standard work, CIL routines, and changeovers (status read from the plant skills matrix / connected-worker platform via HSEi where available, else recorded in pro_manage) |
| 2 | `site_operations_lead` co-sign | 48h | — |
| 3 | `site_maintenance_lead` co-sign | 48h | — |
| 4 | `compliance_officer` co-sign for regulated contexts | 72h | — |

### Example — Benefits Realization review

Fired at `BenefitsRealization.window_closes_at`.

| Order | Role required | SLA |
|---|---|---|
| 1 | `project_owner` compiles variance report | 14d |
| 2 | `finance_approver` validates financial variance | 7d |
| 3 | `portfolio_executive` approves closure | 14d |

Outcome drives the project's final `Archived` transition and feeds a `decision` observation to MDEMG governance space with scope tags for similar future projects.

### Example — RFI response

| Order | Role required | SLA |
|---|---|---|
| 1 | `discipline_engineer` answers | per policy (default 5 business days; 2 if the package is blocked) |
| 2 (if the answer changes scope or cost) | `project_owner` acknowledges; spawns a ChangeRequest / ChangeOrder | 48h |

Overdue RFIs nudge via Commsi; blocked-package days accrue as `waiting_information` losses.

### Example — Submittal review

| Order | Role required | SLA |
|---|---|---|
| 1 | `discipline_engineer` reviews and dispositions (approved / approved-as-noted / revise-resubmit / rejected) | per contract (default 10 business days) |
| 2 (vendor data for turnover-critical assets) | `maintenance_representative` reviews maintainability data | 5 business days |

Revise-resubmit cycles are counted per submittal; repeat offenders by supplier or asset class become MP / quality lessons at closeout.

### Example — Change order (contractual)

| Order | Role required | SLA |
|---|---|---|
| 1 | `construction_manager` validates scope and quantum | 5 business days |
| 2 | `project_owner` approves | 48h |
| 3 (if cost ≥ contract threshold or contingency would go negative) | `finance_approver` | 72h |

Approved change orders update the `Contract` value and, where they implement an internal `ChangeRequest`, close it.

### Example — Work package release

| Order | Role required | SLA | Gate |
|---|---|---|---|
| 1 | `construction_manager` releases | — | All readiness constraints met (`§03 WorkPackage`), or an audited override with reason; override is Tier A in `§06` |
| 2 | Contractor `contributor` acknowledges release | 24h | — |

### Example — Hold point release

| Order | Role required | SLA |
|---|---|---|
| 1 | `inspector` named in the ITP verifies criteria with evidence (photos, test results) | per ITP |

Work on the package cannot progress past the hold point until released. A failed inspection opens a non-conformance and a `rework` loss.

### Example — Project health review (recurring, not a gate)

The capital-side counterpart to an operations huddle: short, regular, ends with owned actions rather than a grade. Records a `ProjectHealthReview` (`§03`).

- **Cadence:** monthly by default during `Plan` and `Execute`; biweekly for projects ≥ $25M or flagged `red`; on demand by a Portfolio Executive.
- **Facilitator:** someone other than the project owner (peer PM, portfolio lead, or CI lead).
- **Agenda (pre-filled from live data):** scope stability, estimate maturity vs stage (AACE class vs FEL/lifecycle stage), critical-path float, top risks and owners, change-order trend, contingency burn vs % complete, Operations/Maintenance readiness for turnover, the project-loss Pareto (from which the review selects one **dominant loss** to attack this cycle, with an owner and a countermeasure), RFI / submittal aging, plan compliance with variance reasons, and one open prompt: *"What are you worried about that isn't on the register?"*
- **Output:** actions with owners and dates (tracked like issues), an overall green/amber/red, and an optional `/jiminy/feedback`-style observation to the project space so recurring concerns become visible guidance.
- **No approval chain** — it is a review, not a gate. A `red` result notifies the Portfolio Executive via Commsi.

### Example — Baseline change

| Order | Role required | Pool | SLA | Jiminy coupling |
|---|---|---|---|---|
| 1 | `project_owner` | project Owner | 24h | `/jiminy/guide` on open ⇒ warn if variance > 10% |
| 2 (if variance > 5%) | `finance_approver` | per BU | 48h | — |
| 3 (if variance > 10%) | `vp_finance` | configured | 72h | `/jiminy/classify` is Tier A (fail-closed) |

### Example — Closeout

Enforces the "closeout plan on file" constraint (from the Governance space). Final stage is `project_owner` + `compliance_officer` co-sign. Milestone-dependency rules enforced in-process beforehand.

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
  - If the workflow kind × amount tier is Tier A → `/jiminy/classify` × 2 (governance then project). First `deny` wins; blocks submit unless admin override.
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
  - `audit_id`, `tenant_id`, `project_id`, `actor_user_id`, `action_type` (fine-grained — e.g., `approval.car.stage_approve`), `subject_type`, `subject_id`, `before` (JSON), `after` (JSON), `at`, `source_ip`, `request_id`, `guidance_id?`, `classification_verdict?`, `signed_hash`
  - `signed_hash = HMAC-SHA256(prev_hash || row_canonical_payload, audit_chain_key)` — a per-tenant chain so tampering is detectable without an external ledger.
  - A nightly job verifies the chain and writes a verification record; mismatches alert immediately.
- Immutable via DB constraints:
  - `BEFORE UPDATE` and `BEFORE DELETE` triggers reject.
  - Postgres role used by pm-core for audit writes has `INSERT`-only on this table.
  - Backup/export pipelines consume audit via a read-only replica.

### Audit retrieval (UC-12)

- Compliance Officers (and time-boxed auditors) query by `(project_id, action_type, from, to)`. Response includes referenced artifacts: Sigi signed envelopes, MDEMG guidance snapshots, before/after payloads.
- Query results are read-only, time-stamped at retrieval, and themselves logged (meta-audit) — "who looked at what approval chain and when."

## Edge cases

- **Approver leaves the company mid-workflow:** `IdPi` deprovision marks the user `is_active=false`. pm-core re-resolves the pool and reassigns via delegation chain. If the user had an in-flight Sigi envelope, pm-core voids and re-issues.
- **Owner changes mid-workflow:** the new owner inherits all open workflows automatically. Stage history preserved under original approvers; new stages route to the new owner's chain.
- **Workflow expires but the business still needs it:** owner reinitiates; new workflow id; audit log links the two.
- **Classify false positive (deny when policy actually allows):** admin override + `blocked_false_positive` feedback; Governance Author reviews the constraint; may re-scope or retire via tombstone.
- **Classify false negative (allow when policy would have denied):** detected post-hoc by audit review; Governance Author authors a new constraint; `missed_violation` feedback captured retrospectively.
- **Delegate approves outside their authority:** pm-core enforces `role_required` at stage completion time, not at assignment time. A delegate who isn't eligible for the required role can't complete the stage.
- **Workflow policy change mid-flight:** in-flight workflows run under their **captured policy snapshot** (recorded at workflow creation). Policy changes affect new workflows only. Visible on the workflow for transparency.

## UX patterns

- **Workflow dashboard:** a user's view of workflows they initiated, workflows awaiting their decision, workflows they're observing. Grouped by SLA proximity.
- **Approval screen:** on open, pm-core warms Jiminy. Primary view shows the subject (CAR details, baseline diff, closeout checklist). Right rail shows guidance cards. Submit button is masked by the classify result when Tier A deny.
- **Delegate screen:** reassign button → pick from eligible pool filtered by availability.
- **Historical workflow:** read-only reconstruction of every stage, every decision, every guidance item shown, every Sigi event — anchored by `audit_id` and linkable.

## Admin surface

Platform Admin UI exposes:

- `WorkflowPolicy` editor — amount thresholds, SLA durations, stage definitions, Sigi requirement toggles. Changes are versioned and logged.
- Eligibility pool management — add/remove users from `finance_approver_eligible`, `vp_finance`, etc. Also links to IdPi claim mappings so admins can see "this role is populated by the AD group `pro_manage.finance_approvers`."
- Delegation chain editor — users can edit their own; admins can edit anyone's with audit.
- Workflow kind registry — enable `custom` kinds with declared schemas; admins can wire them to Sigi/MDEMG/tier policy.
- Audit chain verification dashboard — chain-verification success/failure history, nightly job state.

## Open items for Section 08

- **CAR thresholds and chain (owner gathering).** The stage table above is a placeholder. To replace it, collect the company's **Delegation of Authority (DoA) / approval-limits matrix** and its capital policy, specifically:
  - Approval limits by role and amount (who can approve up to what), and whether limits differ by business unit, site, or project type.
  - What the threshold is measured against: TIC only, TIC + contingency, or total project cost including expensed portions.
  - Supplemental and overrun rules: when a cost increase on an approved CAR needs re-approval (e.g. > 10% or > $X).
  - Board-level thresholds and any capital committee steps.
  - Which approvals require formal e-signature (DocuSign) vs internal attestation.
  - Required CAR content and attachments (business case template, AACE class, IRR/NPV/payback hurdle rates, risk register).
  - Delegation rules when an approver is out (who can sign on their behalf, and up to what limit).
  These populate the `WorkflowPolicy` table directly; no design change is expected.
- Confirm whether `Change request` is a workflow kind of its own, or always a nested sub-step of a Budget or Scope change. First instinct: standalone, with references into Budget and Scope.
- Decide whether "re-submit after reject" creates a brand-new workflow id or extends the existing one. First instinct: new id, with explicit linkage, so the audit record for each attempt is clean.
- Confirm which workflow kinds are mandatory-Sigi vs. optional-Sigi.
- Confirm whether an approver can approve while on OOO with a dated-override ("I'll approve from vacation this once"). First instinct: yes, with a confirm-OOO dialog that logs the override.
- Decide whether external stakeholders can be approvers on specific workflow kinds (e.g., JV partner co-signs on a CAR). If yes, their IdPi federation path and their eligibility pool model needs attention.
