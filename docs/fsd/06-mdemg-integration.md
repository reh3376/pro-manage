# 06: MDEMG Integration (J17, Jiminy, Spaces)

## Decisions on record

- **2026-10-13:** Integration pattern: **Pattern A** (REST-only, no plugin) for MVP. Pattern B (thin plugin) added in Phase 2+.
- **2026-10-13:** Space model: **hybrid**: `space = project_id` for day-to-day observations/outcomes/escalation; `space = org.<orgId>.governance` for company-wide policy constraints **and MP information harvested from running assets**. pro_manage's guidance queries hit both.
- **2026-10-13:** **Flagship use case: MP information flow.** Lessons learned, failures, operability pain points, and maintainability frictions captured at Closeout (and periodically from running assets) are written as `obs_type=learning` observations into the governance space with `scope=equipment:<type>` tags. During Design-stage reviews on later projects for similar equipment, `/jiminy/guide` surfaces them automatically. This closes the EEM loop that TPM is built around.
- **2026-10-13:** Division of responsibilities: **pro_manage owns identity, ACL, workflow, SOX/ISO audit, deterministic rules, and fail-closed policy tiering**. MDEMG is advisory + memory. See `§04`, `§08`.
- **2026-10-13:** Trust mode: **EMA** (`JIMINY_TRUST_MODE=ema`). Not ratchet.
- **2026-10-13:** Metalearn disabled (`METALEARN_ENABLED=false`) to prevent cross-tenant leakage via `mdemg-global` promotion.
- **2026-10-13:** Human escalation thresholds (lower than the AI-agent defaults): `warn=1, escalate=2, block=3` ignores.
- **2026-10-13:** Session identity: `session_id = <user_id>:<project_id>` so escalation accumulates per (user, project), not per HTTP request.
- **2026-10-13:** MDEMG binds **loopback-only** on its host VM. pm-core reaches it over an internal, authenticated channel (overlay network). Port 9999 is never user-reachable.
- **2026-10-13:** pro_manage pins against **JSON field shapes** in responses, not against specific `constraint_code` strings (RSIC can retire codes without notice).

## Space model

```
       MDEMG (one instance per environment)
          │
          ├─── space="proj_<UUID-1>"         ◄── project P-1's observations, outcomes
          ├─── space="proj_<UUID-2>"         ◄── project P-2's observations, outcomes
          ├─── ...                           (one space per project, up to ~500)
          │
          └─── space="org.<orgId>.governance"◄── company-wide policy constraints,
                                                  authored by Governance Authors
```

**Why hybrid:**
- **Project spaces** give hard isolation. Observations about Project A's budget never leak to Project B's guidance, outcomes against A's constraints never influence B.
- **Governance space** holds the company's policy library: CAR thresholds, baseline-change rules, closeout checklists, HSE sign-off rules. Authored once, applied to every project.

**Call-time routing:**
- `/jiminy/guide` is called **twice** for governed actions: once against `project_id` space and once against `org.<orgId>.governance`. Results are merged, deduplicated by `constraint_code`, re-ranked, and projected through role-filter. pm-core caches the merged result briefly (per request).
- `/jiminy/classify` for critical actions is called **against both spaces** sequentially (governance first, since it holds the hard rules; project second, since it may have local overrides). First `deny` wins.
- `/v1/conversation/observe` writes to the **project space only**. Governance constraints are not derived from runtime observations; they are explicitly authored.
- `/jiminy/feedback` is written to the space the originating guidance came from. In practice, outcomes are logged against both spaces when both contributed items.

**No cross-space promotion.** `IncludeGlobalSpace` is explicitly disabled on every call; `METALEARN_ENABLED=false` in MDEMG config.

## Call surfaces

### GUIDE: advisory read on open

- **Endpoint:** `POST /v1/jiminy/guide`
- **When:** pm-core calls when a user opens a resource that supports governed actions (approval form, baseline-change form, closeout checklist, project-settings, etc.).
- **Payload:**
  ```json
  {
    "space_id":    "proj_<uuid>",
    "session_id":  "<user_id>:<project_id>",
    "context":     "User alice is reviewing CAR CR-2457 for $8.4M on project P-INDUSTRIAL-EAST-7",
    "file_path":   "pro_manage://projects/<project_id>/car/CR-2457",
    "agent_output":"<serialized readable summary of current form state>",
    "query":       "<optional user-typed search like 'car approval'>",
    "max_items":   20
  }
  ```
  Then an identical call with `space_id = "org.<orgId>.governance"`.
- **Response consumption:** pm-core takes the structured `guidance[]` array. Each item: `{type, priority, content, confidence, escalation_level, constraint_code, tier, annotations, source_nodes}`. pm-core projects through the role filter (`§04`) and renders in the UI as guidance cards.
- **Pre-warm:** `POST /v1/jiminy/warm` fires fire-and-forget on page open to pre-compute. The actual `guide` call then uses `/v1/jiminy/latest` to read the warmed result in <100ms.
- **Latency budget:** p99 ≤ 1.5s for the two guide calls combined. Guide failure is non-blocking: if MDEMG is unreachable, the page renders without guidance cards and an operational alert fires.

### CLASSIFY: strict gate on submit

- **Endpoint:** `POST /v1/jiminy/classify`
- **When:** pm-core calls when a user submits a **governed** action. Governance Author labels each action kind with a **fail-closed tier**:
  - **Tier A: fail-closed on error.** Critical gates: CAR ≥ threshold, baseline changes ≥ variance threshold, closeout sign-off, project ownership transfer. If classify errors or times out, pm-core **refuses the write** with "Governance system unavailable" and offers admin override.
  - **Tier B: fail-open with surfaced warning.** Non-critical governed actions. If classify errors, pm-core proceeds but records a visible warning in the audit record and operationally alerts.
  - **Tier C: bypass.** Read-only and low-governance writes (comments, personal filters). No classify call.
- **Payload shape identical to `/guide` but adds `tool_name` (the action kind) and `agent_output` is the proposed mutation payload.**
- **Response:** `{verdict, denial_reason, violated_codes[], confidence, escalation_level}`.
- **Latency budget:** p99 ≤ 1.5s per call. 5s is the MDEMG server-side cap and is also a Tier-A refusal threshold.
- **Tier mapping (initial, Governance Author tunable):** Thresholds in the table are policy variables (`§08`), resolved per project, so the same tier rule applies the right limit to each project.
  | Action kind | Tier | Why |
  |---|---|---|
  | `approval.car` (≥ `{car.threshold.vp_finance}`) | A | Dollars + policy stakes |
  | `approval.car` (< `{car.threshold.vp_finance}`) | B | Lower stakes; warning OK |
  | `approval.baseline_change` (variance ≥ `{baseline.variance.vp_finance_pct}`) | A | Material financial |
  | `approval.baseline_change` (variance < `{baseline.variance.vp_finance_pct}`) | B | n/a |
  | `approval.milestone_signoff` | B | Mechanical rules already enforced |
  | `approval.design_review_signoff` | B | Findings dispositioned in-process; guidance advisory |
  | `approval.cqv_signoff` | B | Test evidence is the control |
  | `approval.shutdown_tiein_plan` | A | Production and safety exposure on a running plant |
  | `approval.early_mgmt_exit` | A | Hands the asset to operations |
  | `approval.benefits_realization_review` | B | Retrospective; no write to the asset |
  | `approval.change_order` (≥ contract threshold) | A | Contractual dollars |
  | `approval.change_order` (< threshold) | B | n/a |
  | `approval.work_package_release` (readiness override) | A | Starting work that isn't ready is the loss we're preventing |
  | `approval.work_package_release` (all constraints met) | C | Mechanical; no judgment to advise |
  | `approval.hold_point_release` | B | Inspection evidence is the control |
  | `rfi.answer`, `submittal.review` | B | Engineering judgment; guidance advisory |
  | `approval.closeout` | A | Final gate |
  | `project.transfer_ownership` | A | High-impact admin action |
  | `project.set_lifecycle_state → Archived` | A | Irreversible-ish |
  | `document.publish_restricted` | A | Sensitive data visibility |
  | Everything else state-changing | B | Default |
  | Reads, personal preferences, drafts | C | Not governed |

### FEEDBACK: close the loop

- **Endpoint:** `POST /v1/jiminy/feedback`
- **When:** pm-core writes feedback at the earliest confirmed outcome: `followed` on comply, `ignored_with_reason` on dismiss, `blocked_true_positive` when a Tier A refusal is accepted, `blocked_false_positive` when an admin override follows a Tier A refusal. Fires via the outbox worker (not inline) so the request response isn't held hostage.
- **Payload:**
  ```json
  {
    "guidance_id":   "<from /guide>",
    "action_summary":"User alice attached CFO signoff and approved the $8.4M CAR",
    "space_id":      "proj_<uuid>",
    "session_id":    "<user_id>:<project_id>",
    "outcome":       "followed"
  }
  ```
  Explicit `outcome` is always sent; pro_manage never leaves MDEMG to classify.

### OBSERVE: the write path

- **Endpoint:** `POST /v1/conversation/observe`
- **What pm-core writes as observations:**
  | pro_manage event | obs_type | space | tags/metadata |
  |---|---|---|---|
  | Approval granted/denied (CAR / baseline / closeout / Early Management exit) | `decision` | project | `[approval, kind:car, stage:N, project:XYZ]` + outcome |
  | Budget baseline changed | `decision` | project | `[budget, baseline, aace_class:N]` + before/after amount |
  | AACE class progression | `decision` | project | `[aace, from:5, to:3, fel_stage:fel3]` |
  | Milestone slipped | `blocker` or `progress` | project | `[milestone, slip_days:N]` |
  | Risk raised / aged | `blocker` | project | `[risk, severity:H]` |
  | Metric anomaly / OEE below target | `insight` or `error` | project | `[metric, kpi:oee, project:XYZ]` |
  | CQV punchlist item | `blocker` | project | `[cqv, equipment:boiler, item_id:N]` |
  | **MP information harvested at Closeout** (per observation) | **`learning`** | **governance** | `[mp_info, equipment:<type>, category:failure \| operability \| maintainability \| …, source_project:XYZ]` |
  | MP information harvested from running asset (periodic) | `learning` | governance | same as above |
  | Benefits Realization variance report | `decision` | governance | `[benefits_realization, promised:X, delivered:Y, variance_pct:Z, equipment:<type>]` |
  | Design-review disposition | `decision` | project | `[design_review, criterion:easy_to_maintain, disposition:design_change]` |
  | Lesson learned (manual tag by operator / maintainer) | `learning` | governance (if generalized) or project | `[lesson, project:XYZ, scope:equipment:<type>]` |
  | Policy exception granted | `decision` | project | `[policy_exception, scope:X]` |
  | Recurring project-loss pattern at closeout (e.g. submittals for one equipment class repeatedly revise-resubmit) | `learning` | governance | `[project_loss, category:<cat>, package_kind:<kind>, asset_class:<cls>, contracting_model:<model>]` |
  | Recurring non-conformance cause | `learning` | governance | `[quality_pattern, asset_class:<cls>, spec_ref:<doc>]` |

### MP information flow: the flagship use case

This is the single highest-value integration pattern in pro_manage. It operationalizes the EEM loop: *lessons from running assets → design of the next asset.*

**At Closeout (per project, in UC-15):**
1. pm-core prompts the Owner + Operations lead + Maintenance lead to capture `MPInformation` observations (see `§03`). The UI makes this a required step for closeout.
2. For each `MPInformation`, pm-core writes:
   - A row to Postgres (`mp_information` table, project-scoped).
   - An observation to MDEMG governance space: `POST /v1/conversation/observe` with `obs_type=learning`, `space_id=org.<id>.governance`, `tags=[mp_info, equipment:<type>, category:<category>]`, `content=<narrative>`, `structured_data={source_project_id, severity_at_source, authored_by, authored_at}`.
3. Governance Authors review promoted observations periodically and may codify repeatedly-cited lessons into formal constraints via `POST /v1/jiminy/rules`.

**Periodically from running assets (via CMMSi and HSEi):**
- pm-workers scheduled job pulls `FetchHighFailureAssets` and `FetchAgedWork` from CMMSi (failure history), and `FetchAbnormalityTags` / `FetchLossEvents` from HSEi (operator abnormality tags and priced losses from the operations TPM stack, such as Cryotos-, Fabrico-, Redzone-, and L2L-class platforms).
- Candidate detection runs per **asset class**: repeat failures, recurring abnormality-tag patterns (e.g. the same leak point tagged across three fillers), and top recoverable losses. Each pattern becomes a proposed `MPInformation` (`source_kind = cmms_failure_history | abnormality_tag_pattern | loss_pattern`) for the Maintenance/Operations lead to confirm or reject. Nothing is published to the governance space without human confirmation.
- Rationale: operators already record what's hard to run, clean, and maintain in their tagging tools, every shift. That is the richest MP-information source the plant has; pro_manage's job is to carry it to the next design, not to recapture it.

**At Design stage on a new project (in UC-01 → UC-02 flow):**
- When a user opens a Design screen for a specific equipment type (indicated by WBS tags or project typing), pm-core calls `/jiminy/guide` with:
  - `context="Designing a <equipment_type> for project X; sub-system <sub>"`
  - `space_id` × 2 (project + governance)
  - `query="MP information equipment:<type>"` (optional, helps retrieval)
- Jiminy returns relevant `learning`-type observations and any promoted `constraint`-type items.
- pm-core renders them as **MP Advisory Cards** in the Design UI, grouped by the "easy-to" criterion each MP item speaks to. The DesignReview form becomes implicitly pre-populated with "did we address these?"

**At Benefits Realization closeout (new):**
- The final variance report is written as a `decision`-type observation to the governance space with scope tags that match similar future projects (`[benefits_realization, equipment:<type>, kpi:<metric>]`). Later projects' Business Case drafting surfaces these: "projects building similar <equipment_type> have historically delivered X% of promised EPU reduction."
- **Delivery:** via **Postgres outbox**: every state-changing transaction in pm-core writes a mutation + an `AuditEvent` + an outbox row. A pm-workers process drains the outbox and POSTs to MDEMG with retries. **At-least-once delivery.** MDEMG is idempotent-tolerant for our usage: we include a `source_ref` (= `audit_id`) so dedup is per audit event.
- **Why outbox (not inline):** we never couple a user's transaction to MDEMG availability. If MDEMG is down for 2 hours, outbox depth grows, drains when it returns. pro_manage's audit log remains the authoritative record regardless.

## Session identity

- `session_id = <user_id>:<project_id>`. Not per-HTTP-request. Not `space_id` fallback.
- The same user's actions on Project A and Project B accumulate separate Jiminy escalation state.
- Rationale: a user who repeatedly ignores a safety constraint on Project A should see escalation on A, not have it reset by their next click on B, and not have it pool with other users under the project's space fallback.
- Checkpoint / resume (`/v1/jiminy/checkpoint`, `/v1/jiminy/resume-protocol`) are **optional** in MVP; we add them in Phase 2 to improve recovery when a pm-core replica restarts mid-session.

## Rendering guidance in the UI

pm-core returns a projected `guidance[]` to the frontend. The frontend renders each item based on its `type`, `priority`, and `escalation_level`:

| Visual | Trigger |
|---|---|
| Dismissable info banner | `priority=low` or `type=note` |
| Yellow caution card | `priority=medium`, `escalation_level ≤ surfaced` |
| Orange warning card w/ dismiss-with-reason | `priority=high` + `escalation_level ≤ warned` |
| Red blocking modal | `priority=high` + `escalation_level ≥ escalated` (also surface Tier A classify-will-deny) |
| Audit-stamped admin override | Only reachable from a red block, requires platform_admin re-auth + reason |

Content is the item's `content` string; the UI never shows raw `constraint_code` to end users (compact mnemonics are developer-facing only). Admin UI surfaces the `constraint_code` for debugging.

## Role-filtered guidance

MDEMG has no notion of role. pm-core applies a role filter to every `guidance[]` list before rendering:

- Each constraint carries, via `annotations.roles` (a free-form field, repurposed), the roles entitled to see it (`["owner", "finance_approver"]`, etc.).
- `annotations.scope` identifies the governance family (`capex`, `baseline`, `safety`, …).
- pm-core filters: `item.annotations.roles contains user.active_role_in_this_project` OR annotations is empty (unrestricted).
- Governance Authors must stamp `annotations.roles` + `annotations.scope` when authoring; the admin UI makes this obvious.

If `annotations` is not round-tripped reliably by MDEMG (open question with the maintainer), fallback is a side-table in pm-core (`constraint_code → roles → scope`) populated when the governance author creates the constraint.

## Constraint lifecycle (Governance Author UX)

- **Author** (via Governance admin UI → `POST /v1/jiminy/rules` behind `JIMINY_RULES_UI_WRITE_ENABLED=true`):
  - Draft content, pick `constraint_type` (`must | must_not | should | note`), pick `scope` family, pick target roles.
  - MDEMG dedup gate (cosine ≥ 0.75) returns similar existing rules; author chooses to adopt, override (`?override_dedup=true`), or revise text.
  - On accept, constraint is live in the next guide call.
- **Review effectiveness:** admin dashboard pulls `/v1/jiminy/protocol/metrics` + `/v1/jiminy/protocol/tier-effectiveness` + `/v1/jiminy/latest` and surfaces per-constraint: times surfaced, times followed, times ignored, times blocked, average confidence.
- **Retire** (via `POST /v1/jiminy/rules/{code}/tombstone`): author tombstones. pm-core caches the tombstone and stops surfacing even if MDEMG's downstream retrieval is slow to reflect it.

**pro_manage never calls `/v1/jiminy/protocol/learn` or `/v1/jiminy/extension`:** those are RSIC/evolution surfaces that stay inside MDEMG.

## Resilience and the trust boundary

- **MDEMG unreachable:** Tier A actions refuse (fail-closed), Tier B proceeds with a warning in the AuditEvent (`guidance_outcome.user_response = "mdemg_unavailable"`), Tier C unaffected. An operational alert fires via Commsi.
- **MDEMG slow:** per-call timeouts (`guide=1.5s`, `classify=1.5s`); timeouts behave as "unreachable" for the tier.
- **MDEMG corrupt or wrong (worst case):** pm-core still has the authoritative audit log, the deterministic rules (approval thresholds enforced in-process regardless of what Jiminy says), and the GuidanceOutcome snapshot. A full MDEMG rebuild from backups does not corrupt pro_manage state.
- **MDEMG beta churn:** pm-core isolates all MDEMG calls in `pkg/mdemgadapter`. Field renames land in one place. UATS specs covering the integration surface detect behavior drift in CI.

## Secrets

- `AUTH_API_KEYS` (or `AUTH_JWT_SECRET`): pro_manage → MDEMG auth. Rotated via overlap (`AUTH_API_KEYS` comma-list) quarterly or on incident.
- `J17_TICKET_SECRET`: set explicitly (do not rely on auto-generation; MDEMG warns on auto). Rotation triggers all session resumption to fall back to full re-guidance. That is acceptable, so we rotate semiannually.
- MDEMG's own secrets (`NEO4J_PASS`, `OPENAI_API_KEY`, etc.) are MDEMG's concern; the Platform Admin owns them in the vault.

## Audit coupling

- Every governed action's `AuditEvent` row carries `guidance_id`, `classification_verdict`, `violated_codes` (if any), `fail_tier` (A/B/C), `user_response` (followed/ignored/blocked/overridden), and a snapshot of `guidance[]` the user saw (`GuidanceOutcome.guidance_items_snapshot`).
- Correlation to MDEMG logs happens via MDEMG's `X-Request-ID` header, captured on every call.
- Audit retrieval (UC-12) can reconstruct, for any approved action, both what the user did and what guidance they saw at the time, even if MDEMG has since retired the constraint.

## Non-goals and limits

- MDEMG does not enforce ACL (pm-core does; see `§04`).
- MDEMG does not drive workflow state (pm-core does; see `§08`).
- MDEMG does not store sensitive bodies (we send summaries, not document contents).
- MDEMG does not expose authenticated per-user identity (the shared key is pro_manage's; MDEMG only sees "pro_manage calling").
- MDEMG does not provide SOX/ISO audit; pro_manage's AuditEvent chain is the authoritative record.
- No MCP tools from pro_manage in MVP (Pattern B defers this).
- No `space_id` hierarchy (`org.A.subA.proj`) because MDEMG doesn't support it today.

## Open items for Section 06

- Confirm with MDEMG maintainer: does `annotations.roles` round-trip through MDEMG's retrieval reliably? If not, implement the pm-core side-table fallback from day one.
- Define the operational alert policy on classify-fail-closed events: paging? Email to on-call? Threshold on frequency?
- Decide the retention of GuidanceOutcome snapshots: they can be large. First instinct: 7-year retention on actions tied to approved governed workflows, 180-day retention on everyday snapshots.
- Confirm MDEMG version pin for MVP (`v0.11.0-beta.1` or latest-stable at FSD time of ratification).
- Decide whether session resumption / checkpointing is in MVP or Phase 2 (first instinct: Phase 2).
