# Roadmap: Phased Delivery, Milestones, Dependencies, Risks

Status: **Draft v0, under active construction** · Last updated: 2026-10-14

Companion to the FSD at `docs/fsd/`. Phase timings assume a dedicated small-team delivery (sizing in "Team shape" below). Dates will firm up once team shape and ERP intel are confirmed.

## Guiding principles

- **Ship the backbone early.** Auth, ACL, audit, workflow engine, MDEMG wiring. Everything else layers on top.
- **One vertical slice first.** A single project through its full lifecycle, end-to-end, before breadth.
- **Modular from day one.** The connector framework ships in Phase 0 even if only one backend is wired initially.
- **Automate the discipline early.** UATS + UDTS + UNTS specs land with the first code they govern, not retroactively.
- **MDEMG churn is absorbed by the adapter.** All MDEMG calls go through `pkg/mdemgadapter` from the first commit.

## Phases at a glance

| Phase | Weeks | Theme | Headline deliverable |
|---|---|---|---|
| 0 | 1–6 | Foundation | Dev environment + CI + auth + MDEMG wiring reachable; first governed `/approvals` round-trip works end-to-end on a toy project |
| 1 | 7–16 | MVP vertical slice | One project through Initiate → Execute, with real budget/schedule/approvals, Sigi, one ERPi backend, SSO, Jiminy guidance surfaced |
| 2 | 17–28 | Breadth | Risk/issue/scope, documents+Doci, metrics via Metrici, HSEi (first vendor), spreadsheet I/O, scheduled exports, on-demand decks, external shares |
| 3 | 29–40 | Depth | More connector backends, portfolio dashboards, resource allocation, closeout + legal hold, UPMTS, delegation richness |
| 4 | 41–52 | Harden & scale | Perf tuning, load testing at 1.5× peak, DR drill, UOTS, security review, Kubernetes-readiness verification |
| GA | year-two | Rollout | Site-wide rollout to all projects + Pattern B plugin + additional connector backends per demand |

## Phase 0: Foundation (weeks 1–6)

### Deliverables
- Monorepo with Go, Python, TS/Next.js layouts; `/docs/fsd/` already in place.
- CI pipeline running UATS + UDTS + UNTS + Go/Python/TS unit tests on every PR.
- `docker-compose.dev.yml` spins up Postgres, pm-TSDB, Redis, MinIO, MDEMG dev profile, pm-core, pm-sidecar, pm-workers, pm-frontend.
- Vault wired for secrets; initial secrets provisioned.
- `IdPi` module shell + one backend (OIDC generic) authenticating locally against Keycloak-dev; local break-glass admin flow working.
- `pm-core` edge: auth middleware + ACL plumbing (section-level grants minimum).
- `MDEMG` reachable from pm-core; `pkg/mdemgadapter` with `/guide`, `/classify`, `/feedback`, `/conversation/observe` wrappers; outbox worker drains to MDEMG.
- First governance space seeded (`space=org.pm_dev.governance`); 2–3 toy constraints authored.
- `pm.core.v1.ProjectService` + `ApprovalWorkflow` plumbing, enough to run a toy CAR approval end-to-end.
- Prometheus/Grafana/Loki/Jaeger stack reachable; standard dashboards for RED metrics.
- Connector framework: registry table, admin UI stub, standard `HealthCheck`/`Capabilities` RPCs implemented.
- First UATS, UDTS, UNTS specs for the thin surfaces that exist.

### Success criteria
- A new developer can `make dev` and have the full stack running inside 30 minutes.
- A CAR approval from a seeded test user runs end-to-end: form render → `/jiminy/guide` renders a card → submit → `/jiminy/classify` round-trip → DB-written AuditEvent + GuidanceOutcome → `/jiminy/feedback` outbox dispatched.
- CI green on every PR.

### Dependencies
- Vault tech selection (Vault / cloud KMS / on-prem equivalent).
- IdP sandbox (local Keycloak suffices for Phase 0).
- MDEMG dev profile pinned at a specific tag.

## Phase 1: MVP vertical slice (weeks 7–16)

### Deliverables
- **Projects:** full CRUD; template system shipped with 2 templates; lifecycle transitions `Initiate → Plan → Execute`.
- **Assets & business case:** Asset / AssetClass registry (planned assets owned in pro_manage; in-service mirrored later via CMMSi); LossCategory seeded with the Six Big Losses; `BusinessCasePromise` with CAR-time baseline and measurement basis required for CAR submission.
- **`ERPi_IFS` and the ERP-led profile:** mirror IFS project structure, budgets/forecasts/EAC, earned value, subcontracts, and change orders; CAR approval releases the budget in IFS. Confirm licensed IFS modules and API access first.
- **Project health review:** recurring review with pre-filled agenda and owned actions.
- **Membership & ACL:** ProjectMember, ACLGrant; the four enforcement layers of `§04` (edge, service, projection, DB row-security on sensitive tables).
- **Budget:** manual baseline entry, baseline-change workflow, manual actuals (ERPi integration begins in parallel), variance computed.
- **Schedule:** manual milestone entry + dependencies; completion rule enforcement; (Schedulei imports deferred to Phase 2).
- **Approvals:** CAR + Baseline change workflows live; multi-stage routing via eligibility pools; delegation (basic); SLA tick + nudges via Commsi; admin override path; signed-hash audit chain.
- **Sigi:** DocuSign integration live for CAR ≥ configurable threshold; signed-artifact capture to MinIO.
- **IdPi:** Entra ID *or* Okta backend production-ready (whichever is the owner's primary); SAML backend scaffolded (not necessarily live).
- **Commsi:** SMTP + Teams (or Slack, per company culture) notifications for approvals + overdue nudges.
- **ERPi:** one backend wired against the primary ERP (owner confirms 2026-10-14 which); `FetchActuals` live, `FetchCommitments` live, `FetchInvoices` live for in-scope projects.
- **Dashboards:** Project Overview dashboard template; 6–8 built-in tiles bound to the headline metrics; live SSE updates working.
- **MDEMG:** governance space seeded with ~20 real constraints authored by Governance Author (compliance team); project spaces created per project.
- **Audit:** nightly chain verification job; retrieval UI for approval chains.
- **Frontend:** authenticated UI for Project Owner + Teammate (Editor/Viewer) flows; polished enough for 3–5 pilot users.

### Success criteria
- Three pilot projects live, in-scope for the pilot team's actual work.
- Approval cycle time (CAR) on pilot projects reduced by ≥ 30% vs the pilot team's prior baseline.
- Zero audit findings in a dry-run compliance walkthrough.
- Pilot user NPS ≥ +20.
- SLOs hit for every metric in `§11` at the pilot load.

### Dependencies
- ERP backend chosen (owner intel).
- Identity provider confirmed (owner intel).
- Governance Author (compliance team member) committed to constraint authoring part-time.

## Phase 2: Breadth (weeks 17–28)

### Deliverables
- **Scope / WBS / Deliverables:** full management; change-request workflow live.
- **Risk / Issue management:** risk register with aging rules + Jiminy guidance; issue log.
- **Documents:** local-mode doc management (upload/version/permission); first `Doci` backend (SharePoint or Drive); permission mirroring + drift detection.
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
- **On-demand deck generation:** "Build steering deck" button functional against 2–3 templates.
- **External stakeholder sharing:** tokenized dashboard links; attestation flow; `pm-frontend-public` live.
- **Schedulei:** first schedule-tool backend (likely MS Project XML or P6 XER).
- **Portfolio dashboards:** Executive Portfolio template live.
- **UBTS + USTS:** Phase 2 UxTS adoption; CI gates in place.
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
- **UPMTS** (custom UxTS framework): governs derived-metric contracts.
- **Delegation richness:** chained delegation, OOO calendars, workflow-specific overrides.
- **Portfolio-level approvals** (if applicable): cross-project CapEx roll-ups for executive view.
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
- **UOTS:** operations spec coverage of all compose/K8s manifests.
- Security review (internal or third-party).
- Kubernetes-readiness verification: a parallel K8s cluster running the same images with the same manifests, serving a canary of traffic.
- Documentation hardening: operational runbooks, admin UX docs, user onboarding materials.
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
- Phase-2 UxTS frameworks completed and in CI (UAMS, remaining).
- Explore ML-driven forecasting as a sidecar module (opt-in, scoped to specific metrics).

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
| ERP connector effort larger than estimated | Medium | Medium | Scope Phase 1 ERPi to actuals-only; defer commitments/invoices if needed |
| Full-ACL complexity delivers slower than hoped | Medium | High | Ship section-level in Phase 1; defer document-level and field-level to Phase 2 if under pressure |
| 200+ projects migration from current tools is heavy | High | Medium | Build a bulk-import tool as a Phase 2 scope; onboard in waves |
| Governance Author bandwidth insufficient | Medium | High | Secure part-time commitment early; make constraint authoring UX fast |
| Load at 100–500 users surfaces infra gaps | Medium | Medium | Load test at 1.5× peak in Phase 4; horizontal headroom in design |
| Jiminy false-positive block rate too high | Low | Medium | Lower initial Tier A tier mapping; invest in `blocked_false_positive` feedback cycle |
| Compliance regime adds unexpected constraints (ISO specifics TBD) | Medium | Low–Medium | Design is ISO-aligned by default; adjust retention/approval detail as standards firm up |
| Platform admin UX becomes a bottleneck for connector-heavy ops | Medium | Low | Invest in admin UX in Phase 1 instead of deferring |
| Key-person dependency on individual connector authors | Medium | Low | Each connector type documented; backends paired-reviewed |

## Team shape (indicative)

A **5–7 person core team** for Phase 0–2:

- 1 × Tech Lead / Architect
- 2 × Go backend engineers
- 1 × Python backend engineer (pm-sidecar, derived metrics, render)
- 2 × TS/React frontend engineers (one leads dashboards, one leads workflow UIs)
- 1 × Platform engineer (vault, CI/CD, observability, deployment)
- 0.5 × Governance Author (from compliance team)
- 0.25 × Product / PMO (translating owner needs, pilot coordination)

Grows to **7–9** for Phase 3–4 as connector breadth and ops hardening expand.

## Milestone artifacts

Each phase produces concrete, named artifacts that enable the next phase:

- **Phase 0 end:** `MVP-Spec-Addendum v0.1` (narrows the FSD to Phase-1 actionable features); `demo video` of the end-to-end CAR round-trip.
- **Phase 1 end:** `Pilot-Readiness Review` document; onboarding runbooks for pilot projects.
- **Phase 2 end:** `Breadth Review` and `Governance Effectiveness Baseline`.
- **Phase 3 end:** `DR Drill Report`, `Capacity-Plan Addendum`, `Pattern-B Readiness Assessment`.
- **Phase 4 end:** `GA Readiness Review`, `Security Review Report`, `Rollout Plan`.

## Open items for the roadmap

- Confirm the start date and team shape.
- Confirm the pilot-team identity and the 3 pilot projects (should be real, in-flight work).
- Confirm the MDEMG version pin for the project.
- Confirm which Phase 2 connector backends are highest-value for the owner's company (shapes effort allocation).
- Decide whether a parallel K8s cluster is stood up in Phase 4 or deferred.
