# Roadmap: Phased Delivery, Milestones, Dependencies, Risks

Status: **Draft v0, under active construction** · Last updated: 2026-10-14

Companion to the FSD at `docs/fsd/`. The roadmap is cut around a **thin spine** (below) that is the same under either delivery scenario; the phase timings in weeks assume the small-team scenario, and the owner-plus-AI scenario runs the same order without dates. Dates firm up once the scenario, the pilot projects, and the ERP intel are confirmed.

## Guiding principles

- **Spine before layers.** The spine (the smallest set of features that lets one real project get its full CAR approved in pro_manage with guidance, audit, and e-signature, against a budget mirrored from the ERP) is finished before any layer starts. Layers are pulled by a real need (a wave, a persona, a gate that a pilot project hits), never scheduled because the FSD describes them. Unfinished layers roll to the next phase; the spine never does.
- **Ship the backbone early.** Auth, ACL, audit, workflow engine, MDEMG wiring. Everything else layers on top.
- **One vertical slice first.** A single project through its full lifecycle, end-to-end, before breadth.
- **Contract from day one, SDK after two real modules.** The connector contract (gRPC per type, `HealthCheck` and `Capabilities`, registry, UDTS conformance specs) ships in Phase 0. The Connector SDK (`§15`) is **extracted** from the first two production modules (`IdPi_OIDC`, then `ERPi_IFS`) once both exist, so its helpers are proven by use rather than guessed. The third module is the first one built on it.
- **Automate the discipline early.** UATS + UDTS + UNTS specs land with the first code they govern, not retroactively. The UxTS scaffold (vendored runners, schemas, framework matrix, hash gate, inventory pre-commit hook) is a Phase 0 deliverable in its own right, because it is the forcing function every later phase relies on (`§12`).
- **MDEMG churn is absorbed by the adapter.** All MDEMG calls go through `pkg/mdemgadapter` from the first commit.

## The thin spine

The FSD describes a large system. The spine is the part that must exist before pro_manage is useful to anyone, stated so that it can be finished by a very small team and judged by one test: **a real project's full CAR, approved in pro_manage, with a Jiminy card the approver saw, an audit row that proves it, a DocuSign envelope, and a budget mirrored from IFS.**

| Spine element | What "done" means | FSD |
|---|---|---|
| Identity and membership | SSO through `IdPi_OIDC`; break-glass admin; `ProjectMember` with the four base roles; section-level ACL (document- and field-level are layers). **Status 2026-10-24:** SSO (4a, 4b) and membership with section ACL (5) are in; break-glass admin is open (OI-19) | `§04` |
| Site and Project | `Site` with time zone, currency, calendar; `Project` with lifecycle `Initiate → Plan → Execute`, FEL stage, type, onboarding level | `§03` |
| Policy variables | `PolicyVariable` / `PolicyValue` with the five scopes, resolution, snapshot on workflow start, the policy editor, the starter CAR thresholds loaded from the DoA | `§08` |
| The CAR | `CapitalAuthorization` (`full` first; `fel_funding` and `supplemental` follow in the same phase if cheap), `EstimateArtifact`, `BusinessCasePromise` required at submission, the CAR chain with eligibility pools, SLA tick and nudges, Sigi (DocuSign) envelope and signed artifact | `§03`, `§08` |
| Audit | `AuditEvent` in the same transaction as every mutation, chained hash, nightly verification, approval-chain retrieval | `§08` |
| MDEMG | `pkg/mdemgadapter`; guide on open (warmed), classify on Tier A submit with pre-classification, feedback and observe through the outbox; governance space seeded from the capital policy; local LLM on `pm-mdemg-01` | `§06` |
| ERP mirror | `ERPi_IFS` read-only pulls (structure, budgets, forecasts, EAC, actuals, commitments) and `ReleaseBudget` on CAR approval; polling only | `§10` |
| Notifications | `Commsi_SMTP` for approvals and nudges (Teams or Slack is a layer) | `§10` |
| One screen per persona in the chain | Project page, CAR form, approval screen with guidance rail, workflow inbox, audit retrieval | `§09` |
| Test governance | UATS, UDTS, UNTS from the first endpoint; USTS on the auth boundary before the pilot; no `continue-on-error` | `§12` |

Everything else in the FSD is a **layer** (catalogue below), pulled in by a wave, a persona, or a gate a pilot project actually reaches.

## Delivery scenarios

The roadmap is honest about who is building it. Two scenarios, same order, different clocks:

| | **A: owner plus AI collaborator** (the repo today) | **B: small team** (5 to 7, "Team shape" below) |
|---|---|---|
| Clock | Stages, not weeks; the spine is the first milestone, layers follow one at a time | Phases 0 to 4 over 52 weeks as tabled |
| Spine | Whole spine before any layer; expect it to take the bulk of the first six months of part-time effort | Phases 0 and 1 |
| Layers | One at a time, pulled by the pilot project's next gate; connectors beyond `IdPi_OIDC`, `ERPi_IFS`, `Sigi_DS`, and `Commsi_SMTP` only when a wave needs them | Per phase as tabled, in catalogue priority |
| Deferred to year two or later | Mobile offline sync, Kubernetes readiness, Pattern B plugin, resource allocation, Schedulei, second Doci and HSEi backends, UVTS candidate, portfolio-level approvals | As tabled |
| Risk | Single point of failure (the owner); mitigated by the FSD, the UxTS gates, and `pkg/mdemgadapter` making the codebase legible to a future team | Hiring and ramp; mitigated by the spine being small enough to finish while the team forms |

The success criteria per phase apply to both; in scenario A they are read as "at the end of this stage" rather than "by week N".

**Decided 2026-10-20: Scenario A.** Development started on that basis with these settled choices (recorded in `§05`, `§06`, `§12`, and `docs/fsd/open-items.md`): monorepo in this repository, one PR per slice; this workstation with Docker Desktop; a dedicated MDEMG Compose stack pinned to `v0.11.0-beta.3` with `apikey` auth; Go 1.26, Python 3.14, Next.js 16, goose, pgx + sqlc; `.env` secrets behind a provider interface; Keycloak as the dev IdP; infrastructure-first build order; UxTS runners vendored at `c1f14f74`; stub MDEMG in PR CI with live UATS locally; Northfield Spirits Co. as the seed company; `IdPi_OIDC` as the first module.

## Phases at a glance

| Phase | Weeks | Theme | Headline deliverable |
|---|---|---|---|
| 0 | 1–6 | Foundation | Dev environment + CI + auth + MDEMG wiring reachable; first governed `/approvals` round-trip works end-to-end on a toy project |
| 1 | 7–16 | MVP vertical slice | One project through Initiate → Execute, with real budget/schedule/approvals, Sigi, one ERPi backend, SSO, Jiminy guidance surfaced |
| 2 | 17–28 | Breadth | Risk/issue/scope, documents+Doci, document control, procurement status, metrics via Metrici, HSEi (first vendor), spreadsheet I/O, scheduled exports, on-demand decks, external shares, onboarding wave 1 with the bulk-import tool |
| 3 | 29–40 | Depth | More connector backends, capital plan cycle, portfolio dashboards, resource allocation, closeout + legal hold, UPMTS, UVTS guidance-relevance candidate, delegation richness, onboarding wave 2 |
| 4 | 41–52 | Harden & scale | Perf tuning, load testing at 1.5× peak, DR drill, manifest pin tests, security review, Kubernetes-readiness verification |
| GA | year-two | Rollout | Site-wide rollout to all projects + Pattern B plugin + additional connector backends per demand |

## Phase 0: Foundation (weeks 1–6)

### Deliverables
- Monorepo with Go, Python, TS/Next.js layouts; `/docs/fsd/` already in place.
- CI pipeline running UATS + UDTS + UNTS + Go/Python/TS unit tests on every PR, with no `continue-on-error` on any test step.
- **UxTS scaffold:** vendored runner primitives and reference runners pinned to an MDEMG tag; `schema/`, `specs/`, `runners/` directories per framework; `docs/tests/UXTS_FRAMEWORK_MATRIX.md` with a CI disposition per framework; `add-hashes` / `verify-hashes` wired; UNTS registry; the five adjudication registries with their verifiers and the inventory pre-commit hook (`§12`).
- `docker-compose.dev.yml` spins up Postgres, pm-TSDB, Redis, RustFS (S3), Keycloak, Mailpit, and (with `--profile stub`) the stub MDEMG, alongside pm-core, pm-sidecar, pm-workers, pm-frontend.
- Vault wired for secrets; initial secrets provisioned.
- `IdPi` module shell + one backend (OIDC generic) authenticating locally against Keycloak-dev; local break-glass admin flow working. **Module side met 2026-10-22 (slice 4a):** `connectors/idpi/oidc` serves the contract against the dev Keycloak with UDTS specs for every RPC and a live token test. **Login met 2026-10-23 (slice 4b):** pm-core signs users in through the module (`make check-login` drives Keycloak's login form over HTTP and runs in CI), provisions them just in time, fills pools from claims, and resolves the actor from a Redis session. The break-glass admin flow is still open (OI-19).
- `pm-core` edge: auth middleware + ACL plumbing (section-level grants minimum).
- `MDEMG` reachable from pm-core; `pkg/mdemgadapter` with `/guide`, `/classify`, `/feedback`, `/conversation/observe` wrappers; outbox worker drains to MDEMG.
- First governance space seeded (`space=org.pm_dev.governance`); 2–3 toy constraints authored.
- `pm.core.v1.ProjectService` + `ApprovalWorkflow` plumbing, enough to run a toy CAR approval end-to-end.
- Prometheus/Grafana/Loki/Jaeger stack reachable; standard dashboards for RED metrics.
- Connector framework: registry table, admin UI stub, standard `HealthCheck`/`Capabilities` RPCs implemented; the per-type gRPC contracts and their UDTS conformance specs (`§15` conformance kit, the part of the SDK that is the contract itself).
- `IdPi_OIDC` built as a plain Go module against the contract (no SDK yet); its auth, config, and retry code is written to be lifted later. `IdPi_SAML` only if the company's IdP needs it (owner intel).
- First UATS, UDTS, UNTS specs for the thin surfaces that exist, including UATS specs against the pinned MDEMG for the field shapes `§06` relies on, and the recorded ULTS prompt hashes of the MDEMG tasks pro_manage consumes.

### Success criteria
- A new developer can `make dev` and have the full stack running inside 30 minutes.
- A CAR approval from a seeded test user runs end-to-end: form render → `/jiminy/guide` renders a card → submit → `/jiminy/classify` round-trip → DB-written AuditEvent + GuidanceOutcome → `/jiminy/feedback` outbox dispatched. **Met 2026-10-22 against the stub MDEMG** (`scripts/car_check.sh`, run in CI; slice 3): guide on open via warm + latest on both spaces, pre-classified verdict honoured on submit, the `§08` chain built from the policy snapshot with approver pools, every stage decision audited, feedback and observe drained by pm-workers. Still to run against the live MDEMG (backlog B-02).
- CI green on every PR.

### Dependencies
- Vault tech selection (Vault / cloud KMS / on-prem equivalent).
- IdP sandbox (local Keycloak suffices for Phase 0).
- MDEMG dev profile pinned at a specific tag, with the model pin (`mdemg-llm-v1` Q5_K_M) and the `pm-mdemg-01` host shape decided with IT; Phase 0 ends with a measured classify p50/p95 on that host (`§06`, `§11`).

## Phase 1: MVP vertical slice (weeks 7–16)

### Deliverables
- **Sites:** Site registry (time zone, currency, units, calendar, regulatory and privacy regimes, connector routing); every project and asset bound to a site; policy variables resolve through the `site` scope.
- **Projects:** full CRUD; template system shipped with 2 templates; lifecycle transitions `Initiate → Plan → Execute`.
- **Staged CAR:** `CapitalAuthorization` with `fel_funding`, `full`, and `supplemental` kinds, each backed by an `EstimateArtifact`; supplementals routed on cumulative amount; `basic` design review as the full-CAR pre-condition.
- **Assets & business case:** Asset / AssetClass registry (planned assets owned in pro_manage; in-service mirrored later via CMMSi); LossCategory seeded with the Six Big Losses; `BusinessCasePromise` with CAR-time baseline and measurement basis required for CAR submission.
- **`ERPi_IFS` and the ERP-led profile:** mirror IFS project structure, budgets/forecasts/EAC, earned value, subcontracts, and change orders; CAR approval releases the budget in IFS. Before building: confirm licensed IFS modules, set up the IAM client and service account, and identify the Projects projections in the API Explorer. Build order: read-only pulls, then operational write-back, then budget release; polling until IT approves an inbound path for IFS event push.
- **Project health review:** recurring review with pre-filled agenda and owned actions.
- **Membership & ACL:** ProjectMember, ACLGrant; the four enforcement layers of `§04` (edge, service, projection, DB row-security on sensitive tables).
- **Budget:** in the ERP-led profile (IFS), baselines, forecasts, EAC, commitments, and actuals are mirrored through `ERPi_IFS`; pro_manage adds AACE class, CAR linkage, contingency drawdown, and variance views. Manual baseline entry and the pro_manage-led baseline-change chain are also built, for the pro_manage-led profile and for projects not yet in IFS.
- **Schedule:** manual milestone entry + dependencies; completion rule enforcement; (Schedulei imports deferred to Phase 2).
- **Approvals:** CAR + Baseline change workflows live, with all thresholds and SLAs as policy variables (global, project-type, portfolio, and per-project overrides) and a policy editor; multi-stage routing via eligibility pools; delegation (basic); SLA tick + nudges via Commsi; admin override path; signed-hash audit chain.
- **Sigi:** DocuSign integration live for CAR ≥ configurable threshold; signed-artifact capture to the object store.
- **IdPi:** `IdPi_OIDC` production-ready (`IdPi_SAML` if the company's IdP needs it; pending owner intel).
- **Connector SDK extraction (`§15`):** once `IdPi_OIDC` and `ERPi_IFS` both work in production, the shared parts (module server, vault-backed config, OAuth and OIDC helpers, resilient HTTP and OData clients, idempotency, ETags, polling scheduler, mapping layer, manifest, scaffolding CLI) are lifted into the SDK and both modules are re-based on it with no behaviour change (their UDTS specs prove it). `Sigi_DS` and `Commsi_SMTP` are the first modules built on the SDK, in this phase or the next. Python runtime deferred until a Python-only vendor requires it.
- **Commsi:** SMTP + Teams (or Slack, per company culture) notifications for approvals + overdue nudges.
- **Dashboards:** Project Overview dashboard template; 6–8 built-in tiles bound to the headline metrics; live SSE updates working.
- **MDEMG:** governance space seeded with ~20 real constraints authored by Governance Author (compliance team); project spaces created per project.
- **Audit:** nightly chain verification job; retrieval UI for approval chains.
- **UOTS + USTS merge-blocking before the pilot:** every dashboard, alert rule, and metric registry entry spec-pinned; every protected endpoint proven to reject unauthenticated and wrong-role callers, contributor scoping and share-token exposure covered. The five adjudication registries populated (policy variables, connector capabilities, metrics, routes, loss categories) with the pre-commit gate on.
- **Frontend:** authenticated UI for Project Owner + Teammate (Editor/Viewer) flows; polished enough for 3–5 pilot users.

### Success criteria
- Three pilot projects live, in-scope for the pilot team's actual work.
- Approval cycle time (CAR) on pilot projects reduced by ≥ 30% vs the pilot team's prior baseline.
- Zero audit findings in a dry-run compliance walkthrough.
- Pilot user NPS ≥ +20.
- SLOs hit for every metric in `§11` at the pilot load.

### Dependencies
- IFS Cloud: licensed modules confirmed, IAM client and service account provisioned, Projects-module projections identified in the API Explorer.
- Identity provider confirmed (owner intel).
- Governance Author (compliance team member) committed to constraint authoring part-time.

## Phase 2: Breadth (weeks 17–28)

### Deliverables
- **Scope / WBS / Deliverables:** full management; change-request workflow live.
- **Risk / Issue management:** risk register with aging rules + Jiminy guidance; issue log; three-point quantification above the policy score, contingency basis on baselines, drawdowns naming their cause, contingency coverage on the health-review agenda.
- **Documents:** local-mode doc management (upload/version/permission); first `Doci` backend (SharePoint or Drive); permission mirroring + drift detection.
- **Engineering document control:** master document register per package, `DocumentRevision` issue statuses, transmittals, the IFC-issue workflow gated by detailed design review, and the mechanical "drawings current" readiness check. EDMS mirroring via Doci only if the EPCM runs one (open item in `§10`).
- **Onboarding wave 1 (`§16`):** bulk-import tool (registration, CAR history, budget baseline, milestones, risks, document register workbooks) with validation, dry run, idempotency, provenance, and reconciliation; onboarding levels on the project; every in-flight project above the finance-approver threshold at Governed level; the CAR-only date announced; adoption scorecard dashboard live.
- **Procurement and expediting:** `ProcurementItem` with delivery float against site-need dates; `ERPi` `FetchPurchaseOrders` / `FetchReceipts` mirrored onto items; expediting cadence; `waiting_material` losses.
- **Metrics via Metrici:** relational connectors (Postgres, SQL Server), Timescale/Influx, and the first historian backend; derived-metric formulas via pm-sidecar; dashboards against real harvested data.
- **HSEi:** first MES/WMS/QMS backend wired for one site; incidents + inspections flowing.
- **Spreadsheet import/export:** xlsx budget baseline + risk register + milestones; UATS specs for each template.
- **Loss harvest:** HSEi `FetchLossEvents` from the plant's OEE/loss platform; priced-loss metrics; **Loss & opportunity** dashboard; portfolio ranking by recoverable loss.
- **Operations-TPM MP sources:** HSEi `FetchAbnormalityTags` + CMMSi failure history feeding per-asset-class MP-information candidates for human confirmation.
- **Mobile field flows:** phone-first CQV/SAT walkdowns, punchlist capture, turnover inspections, with photo/QR proof and offline sync.
- **Certification gate:** HSEi `FetchSkillCertifications` (or manual record) wired into Early Management exit.
- **Validate the project-as-equipment loss model (`§14`):** before relying on it, run the project-loss accounting retrospectively on two or three completed projects (from their RFI logs, NCRs, submittal registers, and schedules) and check that the categories capture where the time actually went. Done first in this phase, so `ProjectLossCategory` is adjusted before the EPCM objects below are built on it. Complete the remaining prior-art checks listed in `§14` in parallel, and use the retrospective to answer the Kalsaas and Bølviken critiques with a worked example.
- **EPCM delivery objects:** Contracts, work packages with readiness constraints and plan compliance, RFIs, submittals, change orders, ITPs and hold points, project deviations.
- **Contributor access:** scoped `contributor` role for EPCM/contractor staff, federated identity via IdPi.
- **Project losses:** project-loss events derived from the above; **Project reliability** dashboard; health reviews select a dominant loss.
- **Design-review lenses:** constructability, commissionability, safety in design; Construction Manager stage.
- **Scheduled exports:** PDF + PPTX generation via pm-sidecar; weekly steering export working for pilot projects.
- **Chart builder:** `ChartDefinition` with the grammar's types (incl. Pareto, S-curve, waterfall, timeline), grouping and filters; charts pin to dashboards and deck slots; PNG / SVG / PDF / CSV export with provenance footer; frontend and pm-sidecar render from the same definition, with the render-fidelity reference set in CI (`§09`). Calculated fields and annotations follow in Phase 3.
- **On-demand deck generation:** "Build steering deck" button functional against 2–3 templates.
- **External stakeholder sharing:** tokenized dashboard links; attestation flow; `pm-frontend-public` live.
- **Schedulei:** first schedule-tool backend (likely MS Project XML or P6 XER).
- **Portfolio dashboards:** Executive Portfolio template live.
- **UBTS + UOBS:** soft-fail on PR, full nightly against staging; `§11` SLOs as fixed `p99_ms` baselines. **UAMS** specs for IdPi and connector auth methods (spec-only until the upstream runner exists). USTS and UOTS are already merge-blocking from Phase 1.
- **Observability:** business-health dashboards live; alert rotation documented.

### Success criteria
- ≥ 10 projects onboarded.
- Scheduled exports replace a measurable chunk of hand-prepared decks.
- Spreadsheet imports have measured error rate < 2% invalid-after-validation; valid imports complete in < 15s for 10k-row workbooks.
- First HSE incident surfaced via guidance in a real project.
- Error budget burn stays within target.

## Phase 3: Depth (weeks 29–40)

### Deliverables
- Additional ERPi, Schedulei, Doci, Metrici, HSEi backends per demand.
- **Project-team competency matrix:** role competencies for owner, EPCM, and contractor staff; gaps surface as package readiness warnings.
- **Resource allocation:** Resource + Allocation entities; overallocation detection; dashboards.
- **Closeout workflow:** full lifecycle to `Close` + `Archived`.
- **Legal hold:** admin-triggered retention freeze; dashboards reflect holds.
- **UPMTS** (pro_manage-minted UxTS framework): governs derived-metric contracts; replaces the Go pin tests used for the few derived metrics that shipped earlier.
- **UVTS candidate:** a graded corpus of design-screen queries and the MP information each should surface, live-gated on governance-space changes (`§12`).
- **Delegation richness:** chained delegation, OOO calendars, workflow-specific overrides.
- **Capital plan cycle:** `Opportunity` intake and screening (with Benefits Realization history via guidance), `CapitalPlan` with envelope and ranking, the plan approval chain, conversion to projects, the Capital plan & funnel dashboard; run for the first time on the coming fiscal year's plan.
- **Onboarding wave 2 (`§16`):** all remaining in-flight projects at Registered level or above; the CAR-only date in force; legacy status deck sunset for Governed and Full projects.
- **Governance effectiveness dashboard:** per-constraint surfacing/followed/ignored metrics drawn from MDEMG.

### Success criteria
- ≥ 50 projects onboarded (≥ 25% of in-flight capital projects > $1M).
- Governance effectiveness dashboard surfaces 2+ constraints as "ineffective" (candidates for revision or retirement); Governance Author acts on at least one.
- First successful disaster-recovery drill completed.
- UPMTS catches at least one real derived-metric regression in CI.

## Phase 4: Harden & scale (weeks 41–52)

### Deliverables
- Load testing at 1.5× year-two projected peak; capacity-plan addendum published.
- Full DR drill with scored outcomes.
- **Manifest pin tests:** Go pin tests over compose and Kubernetes manifests (resource limits, probes, non-root, no secrets in env). Not a new UxTS framework; MDEMG lists K8s/Helm as an open UOTS gap, and pin tests are the right size (`§12`).
- Security review (internal or third-party).
- Kubernetes-readiness verification: a parallel K8s cluster running the same images with the same manifests, serving a canary of traffic.
- Documentation hardening: operational runbooks, admin UX docs, the per-persona training set versioned with the release (`§16`).
- **Onboarding wave 3 (`§16`):** every new project starts in pro_manage from Initiate (or from a plan line); adoption review chaired by the sponsor against the scorecard's GA targets.
- Governance space promoted from Phase 1 pilot content to the final production constraint corpus.

### Success criteria
- ≥ 90% of in-flight capital projects > $1M onboarded.
- All year-one success criteria from `§01` met.
- Zero P1 incidents in the last quarter.

## Year-two: GA + beyond

### Deliverables
- Site-wide rollout to all project tiers (including < $1M).
- Pattern B MDEMG plugin (`pro-manage-module`) exposing project domain to MDEMG at ingestion/APE/CRUD levels; new MCP tool surface for AI assistants acting on behalf of users.
- Mobile responsive polish on the frontend.
- Additional connector backends per demand.
- Framework matrix complete: every planned framework at its stated CI disposition, matrix "last verified" within 30 days, UAMS promoted from spec-only if the upstream runner shipped.
- Explore ML-driven forecasting as a sidecar module (opt-in, scoped to specific metrics).

## Layer catalogue

Everything outside the spine, with what pulls it in and the earliest phase it is scheduled under scenario B. Under scenario A the "pulled by" column is the only schedule.

| Layer | Pulled by | Earliest phase | FSD |
|---|---|---|---|
| Document- and field-level ACL | A pilot project with budget lines that teammates must not see | 1 | `§04` |
| Baseline change chain; contingency drawdown; variance views | First re-baseline on a pilot project | 1 | `§03`, `§08` |
| Staged CAR (`fel_funding`, `supplemental`) | First FEL request or overrun on a pilot project | 1 | `§03` |
| Project health review | First monthly review of a pilot project | 1 | `§08` |
| Asset / AssetClass, Loss / LossCategory, loss-priced promises | First CAR whose business case is a loss recovery | 1 | `§03`, `§07` |
| Dashboards (Project overview), SSE updates | Pilot owners asking for status without a deck | 1 | `§09` |
| Design review ("easy-to" plus delivery lenses), MP-information capture | First pilot project entering Plan | 1 to 2 | `§03`, `§08`, `§06` |
| Scope / WBS, risk register with quantification, issues, change requests | First pilot project in Execute | 2 | `§03` |
| Documents (local + first Doci), document control (register, revisions, transmittals, IFC issue) | First package waiting on drawings | 2 | `§03`, `§08` |
| EPCM delivery objects (contracts, work packages, RFIs, submittals, change orders, ITPs, deviations), contributor role, project losses, Project reliability dashboard | First EPCM-delivered pilot project; the §14 retrospective done first | 2 | `§03`, `§14` |
| Procurement items and expediting | First long-lead item on a pilot project | 2 | `§03` |
| Metrici connectors, derived metrics, TPM / plant-ops family | First asset in Early Management | 2 | `§07`, `§10` |
| HSEi (one backend): losses, abnormality tags, certifications; CMMSi (one backend): assets, failure history, turnover delivery | First turnover; first MP harvest from running assets | 2 | `§10` |
| Spreadsheet import / export; bulk-import tool; onboarding wave 1 | Wave 1 | 2 | `§07`, `§16` |
| Chart builder; scheduled exports; on-demand decks; external shares | Steering meetings on pilot projects | 2 | `§09` |
| Mobile field flows (online first; offline sync later) | Commissioning on a pilot project | 2 (offline: year two) | `§09` |
| Commsi Teams or Slack; Schedulei; second Doci, HSEi, CMMSi backends | Demand from a wave | 2 to 3 | `§10` |
| CQV records, Early Management exit, turnover package, Benefits Realization, closeout, capitalization handoff | First pilot project reaching Monitor | 2 to 3 | `§03`, `§08` |
| Capital plan cycle (opportunities, plan, funnel) | The coming fiscal year's planning round | 3 | `§03`, `§08` |
| Resource allocation; team competency; delegation richness; legal hold | Portfolio-level demand | 3 | `§03`, `§08` |
| UPMTS; UBTS and UOBS full runs; UVTS candidate | Enough derived metrics and SLO data to govern | 2 to 3 | `§12` |
| Load test, DR drill, security review, manifest pins, Kubernetes readiness | Before site-wide rollout | 4 | `§11` |
| Pattern B MDEMG plugin; ML forecasting | Year two | GA+ | `§06` |

## Dependencies and critical path

```
Phase 0 ──▶ Phase 1 ─┬──▶ Phase 2 ─────▶ Phase 3 ────▶ Phase 4
                     │
                     ├──▶ ERPi backend 1   ──▶ ERPi backend 2+
                     │
                     ├──▶ IdPi primary     ──▶ IdPi secondary backends
                     │
                     └──▶ Sigi (DocuSign)  ──▶ Sigi (Adobe/OneSpan)

Governance Author onboarding ──▶ constraint corpus ──▶ Phase 1 goes live
MDEMG stack pinned             ──▶ pkg/mdemgadapter ──▶ all phases
Vault provisioned              ──▶ Phase 0 complete
```

**Critical-path blockers to flag:**
- ERP backend confirmation (pending): sets Phase 1 scope.
- IdP intel: sets Phase 1 scope.
- Governance Author staffing: sets Phase 1 scope.
- MDEMG version pin: must be ratified before Phase 0 ends.

## Risk register

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| MDEMG beta wire-format changes break pro_manage | Medium | Medium | Adapter layer, UATS specs, pinned MDEMG version per release |
| IT will not host the MDEMG LLM in either packaged shape (Apple Silicon host or GPU VM) | Medium | High | Raise in Phase 0 week 1; fallback is a CPU-only host with every action kind at Tier B until a suitable host exists, never a hosted LLM |
| Local-LLM classify latency on the chosen host exceeds budget | Medium | Medium | Pre-classification cache, parallel space calls, `mdemg.classify.timeout_ms` tunable; tier down to B per action kind via policy variable |
| ERP connector effort larger than estimated | Medium | Medium | Scope Phase 1 ERPi to actuals-only; defer commitments/invoices if needed |
| Full-ACL complexity delivers slower than hoped | Medium | High | Ship section-level in Phase 1; defer document-level and field-level to Phase 2 if under pressure |
| 200+ projects migration from current tools is heavy | High | Medium | Bulk-import tool and onboarding levels in Phase 2; waves with playbooks and reconciliation (`§16`) |
| Adoption stalls at registration; parallel approval paths survive | Medium | High | CAR-only date set by the sponsor as Finance policy; required level by amount band; adoption scorecard reviewed monthly; champions per persona (`§16`) |
| Governance Author bandwidth insufficient | Medium | High | Secure part-time commitment early; make constraint authoring UX fast |
| Load at 100–500 users surfaces infra gaps | Medium | Medium | Load test at 1.5× peak in Phase 4; horizontal headroom in design |
| Jiminy false-positive block rate too high | Low | Medium | Lower initial Tier A tier mapping; invest in `blocked_false_positive` feedback cycle |
| Compliance regime adds unexpected constraints (ISO specifics TBD) | Medium | Low–Medium | Design is ISO-aligned by default; adjust retention/approval detail as standards firm up |
| Platform admin UX becomes a bottleneck for connector-heavy ops | Medium | Low | Invest in admin UX in Phase 1 instead of deferring |
| Key-person dependency on individual connector authors | Medium | Low | Each connector type documented; backends paired-reviewed |
| Scenario A: the owner is the single point of failure and the plan is larger than one person | High | High | The thin spine is the only committed scope; layers are pulled, not scheduled; the FSD, UxTS gates, and adapter boundaries keep the codebase legible for a future team; re-assess the scenario at the end of the spine |
| SDK extracted too early (guessed helpers) or too late (third module copies code) | Medium | Medium | Extract exactly after `IdPi_OIDC` and `ERPi_IFS` are both in production; UDTS specs prove the re-base changed nothing |

## Team shape (indicative)

**Scenario A (owner plus AI collaborator):** the owner as architect, product owner, and developer, with Claude Code as the drafting and implementation collaborator under the PR workflow in the README; a Governance Author and a Policy Owner from the company part-time once the pilot starts; the change lead role held by the owner until a wave needs more. No dates; the spine, then layers by pull.

**Scenario B:** a **5–7 person core team** for Phase 0–2:

- 1 × Tech Lead / Architect
- 2 × Go backend engineers
- 1 × Python backend engineer (pm-sidecar, derived metrics, render)
- 2 × TS/React frontend engineers (one leads dashboards, one leads workflow UIs)
- 1 × Platform engineer (vault, CI/CD, observability, deployment)
- 0.5 × Governance Author (from compliance team)
- 0.5 × Change Lead (adoption plan, champions, training, scorecard; `§16`)
- 0.25 × Product / PMO (translating owner needs, pilot coordination, wave imports with the Change Lead)

Grows to **7–9** for Phase 3–4 as connector breadth and ops hardening expand.

## Milestone artifacts

Each phase produces concrete, named artifacts that enable the next phase:

- **Phase 0 end:** `MVP-Spec-Addendum v0.1` (narrows the FSD to Phase-1 actionable features); `demo video` of the end-to-end CAR round-trip; `docs/tests/UXTS_FRAMEWORK_MATRIX.md` v1 with a CI disposition per framework.
- **Phase 1 end:** `Pilot-Readiness Review` document; onboarding runbooks for pilot projects; framework matrix showing UATS, UDTS, UNTS, UOTS, USTS merge-blocking and the adjudication registries green.
- **Phase 2 end:** `Breadth Review`, `Governance Effectiveness Baseline`, `Migration Playbook v1` and the wave-1 reconciliation report, first `Adoption Scorecard`.
- **Phase 3 end:** `DR Drill Report`, `Capacity-Plan Addendum`, `Pattern-B Readiness Assessment`, the first approved `Capital Plan` produced in the system.
- **Phase 4 end:** `GA Readiness Review`, `Security Review Report`, `Rollout Plan`.

## Open items for the roadmap

- ~~Choose the delivery scenario.~~ **Decided 2026-10-20: Scenario A.** Re-assess at the end of the spine.
- Confirm the pilot-team identity and the 3 pilot projects (should be real, in-flight work).
- Confirm the MDEMG version pin and model pin for the project, and the `pm-mdemg-01` host shape.
- Confirm which Phase 2 connector backends are highest-value for the owner's company (shapes effort allocation).
- Decide whether a parallel K8s cluster is stood up in Phase 4 or deferred.
