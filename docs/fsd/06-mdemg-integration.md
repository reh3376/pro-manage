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
- **2026-10-14:** **The language model behind Jiminy runs on the MDEMG host; no prompt, observation, or guidance leaves the intranet.** MDEMG's shipped production runtime is a local `llama.cpp` `llama-server` on loopback (`127.0.0.1:8102`) serving MDEMG's own fine-tuned model (`mdemg-llm-v1`: Qwen3-14B plus an MDEMG-trained LoRA adapter, GGUF Q5_K_M, about 10.5 GB). pro_manage deployments configure no hosted-LLM provider in any environment. See "Language model and data boundary" below.
- **2026-10-14:** **Model and runtime are pinned together with the MDEMG version.** `mdemg-llm-v1` at Q5_K_M is the MVP pin; weights are verified by SHA against MDEMG's quant manifest. Moving to a newer MDEMG model (for example `mdemg-llm-v2`, a 27B base that needs 24 to 48 GB of memory) is a planned upgrade with a load test, not a config drift.
- **2026-10-14:** **Classify budgets reflect a local LLM, not a cached lookup.** MDEMG's measured p50 for one LLM call on its reference hardware is about 3 s. The two classify calls (governance and project spaces) run **in parallel**, the per-call timeout is a policy variable (`mdemg.classify.timeout_ms`, default 10 000), and pm-core pre-classifies a settled form so the verdict is usually ready before the user clicks submit.
- **2026-10-14:** **The fail-closed tier of each action kind is a policy variable** (`mdemg.tier.<action_kind>`, values `A | B | C`), resolved like every other policy variable in `§08`. The tier table below holds the global defaults. Lowering a tier is a logged policy change, not a code change.

## Space model

```
       MDEMG (one instance per environment)
          │
          ├─── space="proj_<UUID-1>"         ◄── project P-1's observations, outcomes
          ├─── space="proj_<UUID-2>"         ◄── project P-2's observations, outcomes
          ├─── ...                           (one space per project, up to ~500)
          │
          ├─── space="portfolio_<UUID>"      ◄── portfolio-level decisions with no project yet:
          │                                     opportunity screening, capital plan outcomes
          │
          └─── space="org.<orgId>.governance"◄── company-wide policy constraints,
                                                  authored by Governance Authors
```

**Why hybrid:**
- **Project spaces** give hard isolation. Observations about Project A's budget never leak to Project B's guidance, outcomes against A's constraints never influence B.
- **Governance space** holds the company's policy library: CAR thresholds, baseline-change rules, closeout checklists, HSE sign-off rules. Authored once, applied to every project.

**Call-time routing:**
- `/jiminy/guide` is called **twice** for governed actions: once against `project_id` space and once against `org.<orgId>.governance`. Results are merged, deduplicated by `constraint_code`, re-ranked, and projected through role-filter. pm-core caches the merged result briefly (per request).
- `/jiminy/classify` for critical actions is called **against both spaces in parallel** (governance holds the hard rules; project may hold local overrides). Both verdicts are required; any `deny` wins. Parallel rather than sequential because each call is a local LLM call measured in seconds, not milliseconds (see "Language model and data boundary").
- `/v1/conversation/observe` writes **project events to the project space** and **institutional learnings to the governance space** (MP information, recurring project-loss and quality patterns, Benefits Realization reports; see the OBSERVE table). Governance *constraints* are never derived automatically from observations: a Governance Author promotes a learning into a constraint deliberately.
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
- **Latency budget:** p99 ≤ 1.5s for the two `latest` reads combined. The warm computation behind them (retrieval, constraint classification, and the `jiminy.synthesize` narrative, which MDEMG runs fire-and-forget and measures at tens of seconds) is not on the page-open path: if the warmed result is not ready yet, the page renders without cards and the cards arrive over SSE when it is. Guide failure is non-blocking: if MDEMG is unreachable, the page renders without guidance cards and an operational alert fires.

### CLASSIFY: strict gate on submit

- **Endpoint:** `POST /v1/jiminy/classify`
- **When:** pm-core calls when a user submits a **governed** action. Governance Author labels each action kind with a **fail-closed tier**:
  - **Tier A: fail-closed on error.** Critical gates: CAR ≥ threshold, baseline changes ≥ variance threshold, closeout sign-off, project ownership transfer. If classify errors or times out, pm-core **refuses the write** with "Governance system unavailable" and offers admin override.
  - **Tier B: fail-open with surfaced warning.** Non-critical governed actions. If classify errors, pm-core proceeds but records a visible warning in the audit record and operationally alerts.
  - **Tier C: bypass.** Read-only and low-governance writes (comments, personal filters). No classify call.
- **Payload shape identical to `/guide` but adds `tool_name` (the action kind) and `agent_output` is the proposed mutation payload.**
- **Response:** `{verdict, denial_reason, violated_codes[], confidence, escalation_level}`.
- **Latency budget:** each classify call is one local LLM call (`jiminy.evaluate_llm`), so the budget is seconds, not milliseconds. Per-call timeout is the policy variable `mdemg.classify.timeout_ms` (default 10 000); the two space calls run in parallel, so the submit-path budget is one timeout, not two. A timeout behaves as "unreachable" for the action's tier.
- **Pre-classification:** when a governed form has been idle for a short settle time (default 2 s) and is valid, pm-core classifies the current payload in the background and caches the verdict by `sha256(action_kind, payload, policy_snapshot)`. On submit, a cache hit for the identical payload is used directly; a miss runs the classify inline with a visible "checking governance" state. In practice most Tier A submits see a cached verdict. The cache lives in Redis, is scoped to the user and project, and expires after 10 minutes.
- **Tier mapping (global defaults, set by the policy owner as `mdemg.tier.<action_kind>`):** Thresholds in the table are policy variables (`§08`), resolved per project, so the same tier rule applies the right limit to each project. The tier itself is also a policy variable, so a pilot can start an action kind at Tier B and raise it to A once the constraint library and the false-positive rate justify it.
  | Action kind | Tier | Why |
  |---|---|---|
  | `approval.capital_plan` | A | The year's money; the committee's decision of record |
  | `opportunity.screen` | B | Guidance is the point (Benefits Realization history); a screening is reversible |
  | `approval.car` (`full` or `supplemental`, cumulative ≥ `{car.threshold.vp_finance}`) | A | Dollars + policy stakes |
  | `approval.car` (below the threshold, or `fel_funding`) | B | Lower stakes; warning OK |
  | `document.issue_ifc` | B | Detailed design review is the control; guidance surfaces MP information and past revision churn for the asset class |
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
  | Opportunity screened, deferred, or rejected; capital plan approved or revised | `decision` | portfolio space (`space = portfolio_<id>`, a project-less space per portfolio) | `[capital_plan, fiscal_year:N, opportunity:<code>, outcome:<o>, asset_class:<cls>]` |
  | Imported history at onboarding (prior approvals, baselines) | `decision` | project | `[imported, source:<kind>, approved_at:<date>]` |
  | Design-review disposition | `decision` | project | `[design_review, criterion:easy_to_maintain, disposition:design_change]` |
  | Lesson learned (manual tag by operator / maintainer) | `learning` | governance (if generalized) or project | `[lesson, project:XYZ, scope:equipment:<type>]` |
  | Policy exception granted | `decision` | project | `[policy_exception, scope:X]` |
  | Recurring project-loss pattern at closeout (e.g. submittals for one equipment class repeatedly revise-resubmit) | `learning` | governance | `[project_loss, category:<cat>, package_kind:<kind>, asset_class:<cls>, contracting_model:<model>]` |
  | Recurring non-conformance cause | `learning` | governance | `[quality_pattern, asset_class:<cls>, spec_ref:<doc>]` |

- **Delivery:** via **Postgres outbox**: every state-changing transaction in pm-core writes a mutation + an `AuditEvent` + an outbox row. A pm-workers process drains the outbox and POSTs to MDEMG with retries. **At-least-once delivery.** MDEMG is idempotent-tolerant for our usage: we include a `source_ref` (= `audit_id`) so dedup is per audit event.
- **Why outbox (not inline):** we never couple a user's transaction to MDEMG availability. If MDEMG is down for 2 hours, outbox depth grows, drains when it returns. pro_manage's audit log remains the authoritative record regardless.

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

## Language model and data boundary

Everything Jiminy does with language (classifying an action against a constraint, synthesizing guidance, reformulating constraints into imperatives, minting constraint codes, re-ranking retrieval) is an LLM call. Where that model runs decides whether pro_manage's data stays on the intranet. The decision is that it does.

**What MDEMG ships.** MDEMG's production LLM path is local by design: its hot path is enforcement and "cannot have a network dependency". The runtime is `llama.cpp` `llama-server` bound to `127.0.0.1:8102`, serving `mdemg-llm-v1` (Qwen3-14B base with a LoRA adapter trained on MDEMG's own constraint, action, and outcome data; distributed as GGUF in Q4_K_M, Q5_K_M, and Q8_0 tiers, with Q5_K_M as the production canonical). MDEMG refuses to start when that endpoint is unreachable (`MDEMG_ALLOW_NO_LLM` escape hatch), probes it every 5 s with an `up`, `degraded`, `down` watchdog, and fast-fails calls while it is down. All MDEMG call sites are single-shot structured output; there is no tool calling and no multi-turn. A hosted provider (`EMERGENCE_PROVIDER=openai`) exists in MDEMG only as an optional path for a few classifiers.

**What pro_manage requires of every environment.**
- `LLM_ENDPOINT` on the MDEMG host points at the loopback `llama-server`. No hosted-LLM provider key is issued to MDEMG in any pro_manage environment, including local dev and CI, and the controlled egress proxy (`§11`) has no allow-list entry for an LLM vendor, so a misconfiguration fails rather than leaks.
- `MDEMG_ALLOW_NO_LLM` stays unset in staging and production. An MDEMG that starts without its model would answer classify calls with fallbacks pro_manage has not validated.
- The model is pinned next to the MDEMG version in the deployment manifest (`mdemg-llm-v1`, Q5_K_M, SHA from MDEMG's quant manifest). `mdemg model verify` runs in the deployment health check.
- Weights are fetched once per host through the egress proxy (`mdemg model pull`, Ollama Library as the distribution channel) or, for an air-gapped site, from a file or object-store mirror with a local manifest (`--manifest`). Pulling weights is outbound download of a public artifact, not data egress.

**What leaves pm-core for MDEMG** is unchanged by this decision and is already minimized: readable summaries and structured tags, never document bodies (see "Non-goals and limits"). The point of the local model is that even those summaries, which name projects, amounts, equipment, and people's roles, never leave the company network.

**Hardware.** MDEMG's packaged runtime targets Apple Silicon (a launchd-supervised `llama-server`; Linux and CUDA packaging are deferred on MDEMG's side, though `llama.cpp` itself runs there). pro_manage's hosts are Linux VMs. Two acceptable shapes for `pm-mdemg-01`, in order of preference until IT weighs in:
1. **A dedicated Apple Silicon host** (Mac mini or Mac Studio class) on the server network running MDEMG as packaged. Memory: 32 GB minimum for `mdemg-llm-v1` Q5_K_M with the rest of the MDEMG stack (MDEMG's floor for the model alone is 14 GB, 24 GB recommended); 64 GB if `mdemg-llm-v2` is on the horizon. This is the path with the least divergence from MDEMG's tested configuration.
2. **A Linux VM with a GPU pass-through** (24 GB VRAM class) running `llama-server` built with CUDA under a systemd unit that pro_manage supplies, with the same flags MDEMG uses (`--ctx-size 32768 --parallel 4 --cont-batching --jinja`). This is the shape most IT groups will prefer to host; it is also the one MDEMG has not benchmarked, so it carries a load test in Phase 0.

CPU-only inference of a 14B model is rejected for anything on the submit path: it turns a 3 s classify into tens of seconds and a 50 s synthesize into minutes.

**Sizing the call budget.** MDEMG's own measurements on its reference hardware: p50 about 3 s per LLM call after the `llama-server` cutover; `jiminy.synthesize` about 50 s (fire-and-forget on MDEMG's warm path); retrieval re-rank p95 about 11 s. Four parallel slots share an 8K-token context each, so a prompt that grows with state starves its own output. pm-core therefore keeps `context` and `agent_output` payloads bounded (summaries capped at a configured token budget) and never sends full form bodies.

**Why not a hosted model.** Latency and quality were not the deciding factors; the data boundary was. A hosted provider would require a data-processing agreement, a per-field review of every summary pm-core sends, and an exception to the "no outbound traffic except approved connector egress" rule in `§11`, in exchange for a model that has not been fine-tuned on Jiminy's classification task. If the company ever approves a hosted model for other reasons, re-opening this decision is a §06 change plus a §11 egress entry, not an architecture change: MDEMG already speaks the OpenAI-compatible protocol to its local server.

## Resilience and the trust boundary

- **MDEMG unreachable:** Tier A actions refuse (fail-closed), Tier B proceeds with a warning in the AuditEvent (`guidance_outcome.user_response = "mdemg_unavailable"`), Tier C unaffected. An operational alert fires via Commsi.
- **MDEMG's model down:** MDEMG's watchdog marks the endpoint `down` and classify calls fast-fail; to pm-core this is "unreachable" and the tier rules above apply. The LLM host is therefore part of the Tier A availability chain and is monitored as such (`§11`).
- **MDEMG slow:** per-call timeouts (`guide` read `1.5s`; `classify` per `mdemg.classify.timeout_ms`, default `10s`); timeouts behave as "unreachable" for the tier. A pre-classified verdict (see CLASSIFY) is honoured even if MDEMG has become slow between settle and submit, provided the payload and policy snapshot are unchanged.
- **MDEMG corrupt or wrong (worst case):** pm-core still has the authoritative audit log, the deterministic rules (approval thresholds enforced in-process regardless of what Jiminy says), and the GuidanceOutcome snapshot. A full MDEMG rebuild from backups does not corrupt pro_manage state.
- **MDEMG beta churn:** pm-core isolates all MDEMG calls in `pkg/mdemgadapter`. Field renames land in one place. UATS specs covering the integration surface detect behavior drift in CI.

## Secrets

- `AUTH_API_KEYS` (or `AUTH_JWT_SECRET`): pro_manage → MDEMG auth. Rotated via overlap (`AUTH_API_KEYS` comma-list) quarterly or on incident.
- `J17_TICKET_SECRET`: set explicitly (do not rely on auto-generation; MDEMG warns on auto). Rotation triggers all session resumption to fall back to full re-guidance. That is acceptable, so we rotate semiannually.
- MDEMG's own secrets (`NEO4J_PASS` and the like) are MDEMG's concern; the Platform Admin owns them in the vault. **No hosted-LLM provider key exists in any pro_manage environment**: the model is local (see "Language model and data boundary"), and a deployment check fails if `OPENAI_API_KEY` or an `EMERGENCE_PROVIDER` other than the local endpoint is present on the MDEMG host.

## Audit coupling

- Every governed action's `AuditEvent` row carries `guidance_id`, `classification_verdict`, `violated_codes` (if any), `fail_tier` (A/B/C), `user_response` (followed/ignored/blocked/overridden), and a snapshot of `guidance[]` the user saw (`GuidanceOutcome.guidance_items_snapshot`).
- Correlation to MDEMG logs happens via MDEMG's `X-Request-ID` header, captured on every call.
- Audit retrieval (UC-12) can reconstruct, for any approved action, both what the user did and what guidance they saw at the time, even if MDEMG has since retired the constraint.

## Non-goals and limits

- MDEMG does not enforce ACL (pm-core does; see `§04`).
- MDEMG does not drive workflow state (pm-core does; see `§08`).
- MDEMG does not store sensitive bodies (we send summaries, not document contents).
- MDEMG does not send anything to a hosted LLM. Its model runs on the MDEMG host; nothing pm-core sends it leaves the intranet.
- MDEMG does not expose authenticated per-user identity (the shared key is pro_manage's; MDEMG only sees "pro_manage calling").
- MDEMG does not provide SOX/ISO audit; pro_manage's AuditEvent chain is the authoritative record.
- No MCP tools from pro_manage in MVP (Pattern B defers this).
- No `space_id` hierarchy (`org.A.subA.proj`) because MDEMG doesn't support it today.

## Open items for Section 06

- Confirm with MDEMG maintainer: does `annotations.roles` round-trip through MDEMG's retrieval reliably? If not, implement the pm-core side-table fallback from day one.
- Define the operational alert policy on classify-fail-closed events: paging? Email to on-call? Threshold on frequency?
- Decide the retention of GuidanceOutcome snapshots: they can be large. First instinct: 7-year retention on actions tied to approved governed workflows, 180-day retention on everyday snapshots.
- Confirm MDEMG version pin for MVP (`v0.11.0-beta.1` or latest-stable at FSD time of ratification), together with the model pin (`mdemg-llm-v1` Q5_K_M) and its manifest SHA.
- Decide whether session resumption / checkpointing is in MVP or Phase 2 (first instinct: Phase 2).
- Decide the `pm-mdemg-01` host shape with IT: Apple Silicon host as MDEMG packages it, or a Linux VM with GPU pass-through that pro_manage packages. Phase 0 cannot finish without a measured classify p50/p95 on the chosen host.
- Confirm with the MDEMG maintainer whether `/v1/jiminy/classify` has a non-LLM fast path (vector match against escalated constraints) that could answer the common "no applicable constraint" case in milliseconds, and whether MDEMG enforces a server-side cap on classify time that would sit below `mdemg.classify.timeout_ms`.
- Decide the token budget for `context` and `agent_output` summaries pm-core sends (MDEMG's slots are 8K tokens shared between prompt and output; first instinct: cap pm-core payloads at 2 000 tokens).
