# pro_manage: Functional Specification Document (FSD)

**Status:** Ratified and under construction: the spine is being built against this FSD one slice per PR, with each section's **Decisions on record** block carrying the implementation notes (see the root README's Status and the roadmap) · **Last updated:** 2026-10-26 · **Owner:** Roger Henley

---

## What pro_manage is

**pro_manage is an owner-side capital-project management, data-curation, and metrics system for a TPM-driven process industry.** It is built to carry a capital project from Front-End Loading through Vertical Startup to Benefits Realization, and to make the loop that TPM is actually about (**MP information harvested from running assets → design that incorporates it → vertical startup → benefits realization**) legible, auditable, and inheritable from one project to the next.

It is designed for a single company (not a SaaS tenant model), hosted on the company's intranet in containers, and tuned to the discipline of **Capital Authorization Requests (CAR), Front-End Loading (FEL), Early Equipment Management (EEM), and benefits realization** at a scale of 100–500 users across 200+ concurrent projects ranging from $25,000 refurbishments to $250,000,000 greenfields.

**Target context:** distilling, bottling, food & beverage manufacturing, chemicals, and similar process industries where TPM is the operating language.

**The one question the whole system is organized around**, phrased the way a plant manager would: *"Did the asset start up clean, and did Operations and Maintenance inherit something they could run?"* Every capability, every workflow, every metric in pro_manage ties back to that.

**Two views of TPM, both first-class.** *TPM for the asset*: EEM, MP information, and verified recovery of the losses the business case promised on the equipment being built. *TPM of the project*: the eight pillars applied to project delivery under an EPCM model, where the project itself is the "equipment" and its losses (waiting on information or approvals, rework, interface clashes, unready work packages, safety stops, skill gaps) are measured and attacked the way a plant attacks equipment losses.

**Works with the systems the company already runs.** pro_manage does not replace the ERP, the maintenance system, or the plant's data systems; it connects to them through pluggable connector modules named `<Type>i_<CODE>` (for example `ERPi_IFS` for IFS Cloud, `ERPi_NS` for NetSuite, `CMMSi_MX` for MaintainX), so changing vendors is a configuration change, not a rewrite. Nine connector types cover ERP, single sign-on, e-signature, scheduling tools, document stores, notifications, plant metrics (historians, OPC UA, databases), plant operations systems (MES, QMS, OEE and loss platforms), and the CMMS. When the ERP has a capital-projects module (the first target, IFS Cloud, does), the ERP stays the system of record for budgets, forecasts, earned value, subcontracts, and change orders; pro_manage mirrors them and adds the TPM and governance layer. With a financials-only ERP, pro_manage owns those records instead. Writes back to the ERP are limited to operational records plus one controlled budget release per approved CAR. Details: [integrations catalog (§10)](./10-integrations.md), [system of record by deployment profile (§03)](./03-domain-model.md#system-of-record-by-deployment-profile), and [connector framework (§05)](./05-architecture.md#modular-connector-framework).

---

## Core concept: the project as equipment

A TPM plant knows what share of planned time each line ran, how fast it ran, and how much it made right the first time, and it names, prices, and attacks every loss. Capital projects are usually reported as two variances, cost and schedule, that say *how far off* a project is but not *where the time and money went*. The days waiting on an RFI, the crew standing by because a package started without its materials, the rework after a failed hold point, the change order sitting in an approval queue: these losses disappear into "schedule slip."

pro_manage treats **the project as the equipment**. Its work fronts are the machines and its work packages the production runs. Work-front availability, productivity, and first-time-right stand in for OEE's availability, performance, and quality; a project-loss taxonomy stands in for the Six Big Losses; and all eight pillars are applied to project delivery. Losses are captured automatically from normal work (RFIs, submittals, package readiness, hold points, deviations), priced in the same dollars as plant losses, shown on a Project reliability board, and attacked one dominant loss at a time in each health review. Recurring project losses become lessons the next project sees in design review: MP information for projects.

It builds on Last Planner, Advanced Work Packaging, Koskela's flow view of construction, and earlier work adapting OEE loss groups to site workflow (Kalsaas); what is new is the combination, and a first prior-art search found no source that presents it. This is the idea pro_manage is built to develop. The full statement, mappings, related work, and open questions are in [§14](./14-project-as-equipment.md).

---

## Why it exists

Capital projects in a TPM-driven plant are rarely well-served by generic PM tools. The real work spans:

- **Front-End Loading** with AACE Class 5 → 3 → 1 estimate maturity, pre-authorization gates, and a defensible CAR;
- **Design reviews** against the seven "easy-to" criteria (operate, maintain, clean, change over, inspect, keep safe, make quality product), with operator and maintenance participation;
- **MP (Maintenance Prevention) information** harvested from running assets and *actually fed back* into the next design, not left in a lessons-learned folder;
- **Commissioning / Qualification / Validation**, Factory and Site Acceptance Tests, and a **Turnover Package** (equipment data, spares, PM tasks, procedures, calibration, standard work) complete at handover, not chased afterward;
- **Vertical Startup** measured by the actual startup curve, with an explicit **Early Management Period** that doesn't end until declared exit criteria are met;
- **Benefits Realization** confirmed 6–12 months post-startup, with ROI recomputed against the business case promise.

In most companies, this work is spread across Excel workbooks, email approval threads, SharePoint folders, Primavera files, and periodic ERP pulls, with the audit trail distributed across inboxes and institutional memory lost at every turnover. pro_manage is an owner-side instrument that holds the whole picture in one shape with one audit log, and makes governance an in-band part of the work rather than a separate ritual.

---

## The organizing idea: lifecycle = EEM stages

pro_manage's project lifecycle maps one-to-one onto the six EEM stages, each with explicit gates and exit criteria:

| pro_manage state | EEM stage | Primary artifacts |
|---|---|---|
| **Initiate** | Concept / Planning | Business case, Project Success Criteria, MP-information harvest from similar equipment, AACE Class 5 estimate |
| **Plan** | Design (basic → detailed) | Basis of Design, "easy-to" design reviews, AACE Class 3 at CAR submission, risk register, LCC estimate |
| **Execute** | Fabrication / Build → Installation → Commissioning | FAT at vendor, SAT in plant, CQV records, shutdown & tie-in planning, construction management |
| **Monitor** | Early Management Period | Startup curve tracking, OEE / MTBF / FPY monitoring, exit criteria toward handover |
| **Close** | Normal operations handover | Turnover Package to Operations + Maintenance (via CMMSi), Benefits Realization window begins |
| **Archived** | n/a | Post-benefits-realization retention; readable, un-editable |

Every gate carries a **CAR / baseline-change / design-review / CQV / shutdown & tie-in / Early-Management-exit / Benefits-Realization** workflow with multi-stage routing, delegation, SLA, and (where applicable) formal e-signature via DocuSign.

---

## The MDEMG governance substrate (the flagship AI integration)

pro_manage integrates with **MDEMG** (Multi-Dimensional Emergent Memory Graph, [github.com/reh3376/mdemg](https://github.com/reh3376/mdemg)) as the advisory and institutional-memory layer. MDEMG is deliberately *not* used as a policy or workflow engine. pro_manage owns identity, authorization, approval state machines, deterministic rules, and the SOX/ISO-grade audit log. MDEMG does three things pro_manage cannot do as well on its own:

1. **At every decision point**, surfaces the applicable constraints, prior decisions, and lessons learned, rendered as cards in the UI, filtered by role.
2. **On critical gates** (CAR above threshold, baseline change > variance, closeout, ownership transfer), classifies the proposed action against the constraint corpus and can refuse the write in fail-closed mode.
3. **Flagship use case: closes the MP-information loop.** At Closeout, pro_manage prompts the owner, maintenance, and operators to capture lessons as scoped observations into MDEMG's governance space (`org.<id>.governance`). When a *different* project enters Design for similar equipment months or years later, those lessons surface automatically in the design-review guidance. This is the discipline EEM calls for, built into the workflow rather than left to a lessons-learned folder nobody opens.

pro_manage pins against JSON field shapes rather than specific constraint codes, isolates every MDEMG call behind an adapter layer, and tiers actions as fail-closed / fail-open / bypass per policy, so MDEMG availability affects guidance quality, never application integrity.

---

## Capabilities at a glance (grouped by lifecycle stage)

**Initiate (Concept / Planning)**
- Project typing (Greenfield · Brownfield expansion · Reliability · Efficiency · Compliance · Capacity debottleneck); TPM-pillar tagging; hazardous-area classification; regulatory stamps
- Business case + Project Success Criteria capture, with promises stated as recoverable losses (Six Big Losses, priced in dollars) against a baseline captured from production data
- MP-information retrieval from governance space (Jiminy guidance)
- AACE Class 5 estimate anchor

**Plan (Design)**
- FEL stage progression (FEL1 → FEL2 → FEL3) with AACE class maturation
- Full-ACL team membership (per section, per document, per field) + delegated access
- Design-review workflow with the seven "easy-to" criteria plus constructability, commissionability, and safety in design; operator, maintenance, and construction-manager participation
- Basis of design, risk register, LCC estimate, TIC baseline
- CAR submission with multi-stage approval chain

**Execute (Fabricate / Install / Commission)**
- Construction management + contractor tracking
- Shutdown & tie-in planning for brownfield work (sequenced around the production schedule, not the construction schedule)
- FAT / SAT / CQV records; non-conformance tracking
- Operator + maintenance participation in commissioning readiness and startup rehearsal
- Work packages released only when ready (drawings, RFIs, materials, crew, permits, HSE); weekly plan compliance with variance reasons
- RFIs, submittals, and change orders with cycle times; delays recorded as project losses
- Inspection & test plans with hold points; deviation tagging by anyone close to the work
- EPCM partner and contractor staff work in pro_manage on their own packages, scoped by contract
- Change-request workflow with budget/schedule impact
- Earned-value (CPI, SPI, BCWP) where inputs available

**Monitor (Early Management Period)**
- Startup curve + time-to-nameplate tracked daily
- Operational KPIs harvested post-startup: OEE (Availability × Performance × Quality), TEEP, MTBF, MTTR, FPY/RFT, EPU, OLE, Changeover Time
- Early Management exit criteria, configurable per project, including operator and maintenance certification on the new asset

**Close (Handover + Benefits)**
- Turnover Package: equipment master, spares, PM tasks, standard work, calibration; writable to the plant **CMMSi**
- Benefits Realization window (default 6–12 months); **verified recovery** of the promised losses on the same assets, and ROI recomputed against the business case
- MP-information harvest to governance space

**Across every stage**
- Live dashboards, scheduled exports (PDF + PPTX), on-demand steering-deck generation
- External stakeholder sharing via tokenized links
- SOX/ISO-grade audit log with chained signed hashes
- Jiminy guidance at every governed action
- Recurring project health reviews ending in owned actions
- Every threshold, SLA, cadence, and window is an admin-configurable policy variable, with per-project overrides (no hard-coded limits)
- Phone-first field flows for walkdowns, punchlists, and turnover inspections (Phase 2)

---

## Architecture at a glance

- **pm-core (Go):** transactional backbone. REST + SSE/WebSocket for the frontend; gRPC to sidecar, workers, and connectors. Edge auth, ACL enforcement at four layers (edge, service, projection, DB row-security), workflow engine, outbox dispatcher.
- **pm-sidecar (Python):** PDF/PPTX render, chart rendering for exports (same definitions and grammar as the frontend), xlsx import/export, sandboxed derived-metric formulas (CPI/SPI/EAC, custom KPIs).
- **pm-workers (Go):** scheduled jobs (metric harvest, exports, nudges, SLA tick), outbox drain to MDEMG and Commsi, long-running imports.
- **pm-frontend (Next.js, TypeScript):** authenticated SSR+CSR UI; separate hardened `pm-frontend-public` for tokenized dashboard shares.
- **Modular connector framework:** one gRPC contract per connector type (**ERPi, IdPi, Sigi, Schedulei, Doci, Commsi, Metrici, HSEi, CMMSi**), admin-registered, UDTS-governed. Admins swap backends without a core release. Modules are built on the **Connector SDK** (§15): shared runtime, auth and integration helpers, conformance tests, and a manifest the admin UI reads, so adding a vendor needs no core release.
- **Data stores:** Postgres (OLTP source of truth), TimescaleDB (metrics), Redis (sessions/cache), an S3-compatible object store (RustFS in dev), all containerized. **MDEMG** stack runs adjacent on an isolated loopback network, with its language model served locally on the same host (no project data goes to a third-party AI service; §06).
- **Deployment:** on-prem VMs with containers, docker-compose topology designed for later Kubernetes migration without reshaping services.
- **Test governance:** **UxTS**, MDEMG's family of hashed, schema-governed contract specs. UATS (REST), UDTS (gRPC), UNTS (hash registry) from Phase 0; UOTS (dashboards, alerts, metric registry) and USTS (auth boundaries, exposure, injection) before the pilot; UBTS, UOBS, UAMS in Phase 2; a pro_manage-minted UPMTS for derived-metric formulas in Phase 3. Companion adjudication registries make every policy variable, connector capability, metric, route, and loss category prove it has a consumer (§12).

---

## Design principles

- **The plant is the customer.** Every design decision ends at "did the asset start up clean, and can Operations and Maintenance run it?" If the FSD can't answer yes, the FSD is wrong.
- **Owner-side, not vendor-side.** This is an instrument for the people buying and running the asset, not for the engineering firm building it.
- **Operator and maintenance participation is a feature, not a courtesy.** Design reviews, commissioning rehearsals, turnover sign-off, and MP-information authoring all route through the people who will live with the asset. The system refuses to pretend they can be an audience rather than participants.
- **Governance in-band, not alongside.** Advisory guidance surfaces at the decision point. Audit happens in the same transaction as the mutation.
- **MP information must flow.** The single most under-practiced TPM discipline (feeding what was learned from the last asset into the design of the next) is a first-class feature with explicit UX and automation, not an unread folder.
- **Integrate with the operations TPM stack; don't rebuild it.** Autonomous maintenance, abnormality tagging, OEE boards, and daily management already live in CMMS and connected-worker tools. pro_manage is the EEM-pillar system: it reads their losses, tags, and failure history in and hands assets and turnover packages back out.
- **Modularity for the external world.** Every ERP, IdP, CMMS, MES, QMS, scheduler, document store, and signer is a pluggable module. Zero vendor lock-in in the design.
- **Isolate the beta.** MDEMG is beta software. The adapter layer absorbs churn so the core is stable even when MDEMG moves.
- **Contracts are frozen, hashed, and gated.** Every contract the FSD names gets a UxTS spec with a hash; changing it is an explicit commit that CI sees. Silent drift is the failure mode this design refuses to tolerate, and no test step is allowed to soft-fail by accident (§12).
- **No inference leaves the building.** The only AI in the loop runs on a host on the company network. That is a design rule, not a deployment option.
- **Boring where it should be boring.** Go, Postgres, Timescale, Redis, an S3 object store, docker-compose. Nothing novel in the plumbing.

---

## Reading the FSD

The FSD is split into focused sections. For a complete picture, read in order. For a role-specific tour, use the quick paths.

### Sections

| § | File | What's in it |
|---|---|---|
| 01 | [vision-and-scope.md](./01-vision-and-scope.md) | Mission, problem statement, in/out of scope, success criteria (incl. startup curve, benefits realization, MP flow) |
| 02 | [personas-and-use-cases.md](./02-personas-and-use-cases.md) | 18 personas (incl. policy owner, site operations & maintenance reps, commissioning engineer, construction manager, document controller, procurement lead, PMO lead, change lead, EPCM / contractor contributors), 26 use cases, 5 end-to-end journeys |
| 03 | [domain-model.md](./03-domain-model.md) | Site, Project, members, ACLs, budget (TIC/LCC, estimate artifacts, quantified risks and contingency), staged CAR, schedule, scope, documents and document control (register, revisions, transmittals), procurement items, metrics, approvals, EEM entities (DesignReview, MPInformation, CommissioningRecord, TurnoverPackage, BenefitsRealization), Asset/AssetClass, Loss/LossCategory, BusinessCasePromise, ProjectHealthReview; EPCM objects (Contract, WorkPackage, RFI, Submittal, ChangeOrder, ITP/HoldPoint, ProjectDeviation, ProjectLoss) |
| 04 | [access-control-and-identity.md](./04-access-control-and-identity.md) | SSO (OIDC + SAML), platform / project / workflow roles, contract-scoped EPCM contributors, full ACL per section/doc/field, 4-layer enforcement |
| 05 | [architecture.md](./05-architecture.md) | Service inventory, gRPC/REST surfaces, modular connector framework, deployment topology |
| 06 | [mdemg-integration.md](./06-mdemg-integration.md) | J17/Jiminy call surfaces, hybrid space model, fail-closed tiering, MP-information flow (incl. abnormality-tag and loss patterns from the operations TPM stack) |
| 07 | [metrics-and-data-curation.md](./07-metrics-and-data-curation.md) | Financial, Schedule, Scope/Risk, Safety/Quality/Compliance, TPM/Plant-Ops, priced-loss, and project-loss families; verified recovery; package readiness and plan compliance; connectors; spreadsheets |
| 08 | [approvals-and-workflows.md](./08-approvals-and-workflows.md) | Policy variables (configurable thresholds, site and per-project overrides), staged CAR workflow (FEL funding, full, supplemental), baseline change, IFC issue, expediting cadence, design review, CQV, shutdown & tie-in, Early Management exit, closeout, Benefits Realization, project health review, RFI / submittal / change order, work package release, hold points, Sigi, admin override, audit chain |
| 09 | [presentations-and-reporting.md](./09-presentations-and-reporting.md) | Live dashboards (incl. Loss & opportunity, Capital plan & funnel, Startup & Early Management, Project reliability), chart grammar (Pareto, S-curve, waterfall, timeline), user-built charts and graphs with image and data export, scheduled exports, on-demand decks, external sharing, phone-first field flows |
| 10 | [integrations.md](./10-integrations.md) | ERPi, IdPi, Sigi, Schedulei, Doci, Commsi, Metrici, HSEi, CMMSi |
| 11 | [ops-and-nfr.md](./11-ops-and-nfr.md) | SLOs, availability, backup/DR, observability, security, retention |
| 12 | [test-governance-uxts.md](./12-test-governance-uxts.md) | UxTS as forcing function: spec anatomy, hash gate, pro_manage framework matrix (UATS, UDTS, UNTS, UOTS, USTS, UOBS, UBTS, UAMS, UPMTS, UVTS candidate), adjudication registries |
| 13 | [glossary.md](./13-glossary.md) | Terms and acronyms (TPM, EEM, CAR, AACE, LCC, TIC, OEE, MTBF, …) |
| 14 | [project-as-equipment.md](./14-project-as-equipment.md) | **Core concept.** The project as equipment: OEE and Six Big Losses mapped to project delivery, the eight pillars applied, version-1 implementation, open questions, prior art |
| 15 | [connector-sdk.md](./15-connector-sdk.md) | Connector SDK: module runtime, integration helpers (OAuth, SAML/OIDC, OData, idempotency, ETags, polling, inbound events), mapping layer, conformance kit, scaffolding CLI, module manifest, versioning |
| 16 | [onboarding-migration-and-adoption.md](./16-onboarding-migration-and-adoption.md) | Onboarding levels, data migration sources and the bulk-import tool, cut-over rules (history imported, never re-approved), waves, change management (sponsor, change lead, champions), mandatory-use dates, training, support, adoption scorecard |
| n/a | [roadmap.md](./roadmap.md) | Thin spine, delivery scenarios, layer catalogue, phased delivery, milestones, dependencies, risks |
| n/a | [open-items.md](./open-items.md) | **Owner decision checklist:** every open item across the FSD, grouped by what it unblocks (Phase 0, first real CAR, wave 1, later, technical defaults), with an answer column |

### Quick paths

- **Capital Engineering leader:** §14, §01, §08, §03, §07, §09, §16
- **PMO / change lead:** §16, §02, §08 (capital plan cycle), §09 (Capital plan & funnel, Adoption scorecard)
- **Plant operations / maintenance stakeholder:** §14, §01 (success criteria), §07 (TPM/Plant-Ops family), §08 (Early Management exit, Turnover), §06 (MP-information flow)
- **Platform engineer:** §05, §15, §06, §11, §12
- **Finance / Compliance:** §04, §07 (TIC/LCC, Benefits Realization), §08 (CAR + chained-hash audit), §11
- **AI / governance stakeholder:** §06, §12

---

## Roadmap at a glance

| Phase | Weeks | Theme | Headline |
|---|---|---|---|
| 0 | 1–6 | Foundation | Dev env + CI + UxTS scaffold + auth (`IdPi_OIDC`) + connector contract + MDEMG adapter + first governed round-trip on a toy project |
| 1 | 7–16 | MVP vertical slice | One real project through Initiate → Execute with CAR (loss-baselined business case) + asset registry + Jiminy guidance + Sigi + one ERPi backend + SSO |
| 2 | 17–28 | Breadth | Risk/scope/documents, document control, procurement status, Metrici connectors, HSEi (incl. loss events, abnormality tags), CMMSi, spreadsheet I/O, dashboards, mobile field flows, external shares, onboarding wave 1 with the bulk-import tool |
| 3 | 29–40 | Depth | More backends, capital plan cycle, resource allocation, closeout + Benefits Realization window, legal hold, UPMTS, onboarding wave 2 |
| 4 | 41–52 | Harden & scale | Load testing, DR drill, security review, K8s-readiness, site-wide rollout prep |
| GA | year-two | Rollout | Site-wide, Pattern-B MDEMG plugin, additional backends per demand |

The roadmap is cut around a **thin spine** (one real project's full CAR approved in pro_manage with guidance, audit, e-signature, and an ERP-mirrored budget) that is finished before any layer starts, and it states two delivery scenarios (owner plus AI collaborator, or a small team) that run the same order on different clocks. See [roadmap.md](./roadmap.md) for the spine, the layer catalogue, milestones, dependencies, team shape, and risk register.

---

## Working agreement for this document

- Each section is self-contained; cross-references use relative links.
- Decisions made in dialogue with the owner are captured here, not re-litigated. A **Decisions on record** block at the top of each section records the choice and date.
- Open items for later resolution are tracked under **Open items for Section N** at the bottom of each section.
- The source of truth for MDEMG capabilities is the MDEMG repo at the pinned version. Integration-relevant findings from the owner's MDEMG research are captured in `§06`; supporting research (MDEMG briefs, TPM software market review) is held privately by the owner.

---

## Attribution

**Author / Owner:** Roger Henley.
**Drafting assistance:** Claude (Anthropic) via Claude Code, under direction of the owner, 2026-10-13 onward.
**MDEMG** by Roger Henley (separate project): [github.com/reh3376/mdemg](https://github.com/reh3376/mdemg).
