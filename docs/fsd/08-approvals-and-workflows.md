# 08 — Approvals, Workflows & Audit

## Decisions on record

- **2026-10-13** — Approvals are a core feature. pro_manage owns the workflow engine; MDEMG provides advisory + classify at the gates (`§06`).
- **2026-10-13** — Workflow kinds in year one: **CapEx, Baseline change, Milestone sign-off, Change request, Closeout**, plus a configurable **Custom** kind for policies pro_manage doesn't ship with.
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

### Example — CapEx approval chain (year-one defaults, admin-tunable)

| Order | Role required | Pool | SLA |
|---|---|---|---|
| 1 | `project_owner` | the project's Owner | 24h |
| 2 | `finance_approver` | `finance_approver_eligible` ∩ `project.business_unit = X` | 48h |
| 3 (if amount ≥ $5M) | `vp_finance` | configured VPs | 72h |
| 4 (if amount ≥ $25M) | `cfo` | configured CFO + 1 delegate | 72h |
| 5 (if amount ≥ $100M) | `ceo` | configured CEO + 1 delegate | 7 days |

Thresholds and roles are stored in the `WorkflowPolicy` table and are editable by `platform_admin` with a logged change.

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
  - CapEx: auto-escalate once, then expire if still undecided.
  - Baseline change: expire; owner must re-initiate.
  - Milestone sign-off: expire; sign-off gets re-queued on next cycle.
  - Closeout: no expiration; just stays overdue until decision.

## E-signature coupling (Sigi)

For workflow kinds where formal e-sig is mandatory (CapEx ≥ threshold, Change requests touching contractual terms, Closeout):

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
  - `audit_id`, `tenant_id`, `project_id`, `actor_user_id`, `action_type` (fine-grained — e.g., `approval.capex.stage_approve`), `subject_type`, `subject_id`, `before` (JSON), `after` (JSON), `at`, `source_ip`, `request_id`, `guidance_id?`, `classification_verdict?`, `signed_hash`
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
- **Approval screen:** on open, pm-core warms Jiminy. Primary view shows the subject (CapEx details, baseline diff, closeout checklist). Right rail shows guidance cards. Submit button is masked by the classify result when Tier A deny.
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

- Confirm the dollar thresholds (sample above is a plausible baseline; the owner's company will have real figures).
- Confirm whether `Change request` is a workflow kind of its own, or always a nested sub-step of a Budget or Scope change. First instinct: standalone, with references into Budget and Scope.
- Decide whether "re-submit after reject" creates a brand-new workflow id or extends the existing one. First instinct: new id, with explicit linkage, so the audit record for each attempt is clean.
- Confirm which workflow kinds are mandatory-Sigi vs. optional-Sigi.
- Confirm whether an approver can approve while on OOO with a dated-override ("I'll approve from vacation this once"). First instinct: yes, with a confirm-OOO dialog that logs the override.
- Decide whether external stakeholders can be approvers on specific workflow kinds (e.g., JV partner co-signs on CapEx). If yes, their IdPi federation path and their eligibility pool model needs attention.
