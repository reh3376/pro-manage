# 01: Vision & Scope

## Decisions on record

- **2026-10-13:** Design for site-wide scale: 100–500 users, 200+ concurrent active projects.
- **2026-10-13:** Capital project budget range: $25K to $250M per project.
- **2026-10-13:** Access model: full ACL per section, per document, per field.
- **2026-10-13:** First-class metric families: Financial, Schedule/Milestones, Scope/Risk/Issues, Safety/Quality/Compliance + Resource utilization; extended 2026-10-13/14 with TPM / Plant Operations (OEE, MTBF, startup curve, …) and priced losses (`§07`). Plus read-only connectors to relational and time-series databases, and spreadsheet import/export.
- **2026-10-13:** MDEMG J17 + Jiminy integrated at runtime as advisory + memory layer, with pro_manage owning identity, ACL, workflows, and audit.
- **2026-10-13:** Approvals are a core feature.
- **2026-10-13:** Progress presentations include live dashboards, scheduled exports (PDF/PPTX), and on-demand deck generation. **2026-10-14:** plus user-built charts and graphs, saved as definitions and exportable as images and data (`§09`).
- **2026-10-13:** **All external-system connectors are modular.** No ERP, IdP, e-signature, schedule tool, document store, communication platform, or MES/WMS/QMS vendor is hard-wired into the core. Each is a pluggable module with a declared contract (gRPC interface + UATS-governed REST surface) that an administrator can configure, enable, or swap without a core release. First-class modules in year one: ERPi (ERP), IdPi (SSO), Sigi (e-signature), Schedulei (schedule tools), Doci (document stores), Commsi (notifications), Metrici (relational + time-series connectors), HSEi (MES/WMS/QMS and operations TPM platforms), CMMSi (CMMS read/write; added 2026-10-13).
- **2026-10-14:** **Connector modules are named `<Type>i_<CODE>`** (e.g. `ERPi_NS` for NetSuite, `ERPi_SAP` for SAP, `CMMSi_MX` for MaintainX). The company's specific ERP, CMMS, MES, and IdP are not yet known; pro_manage does not depend on knowing them, because the matching modules are selected at deployment. See `§10`.
- **2026-10-14:** **First target ERP: IFS Cloud** (`ERPi_IFS`). Because IFS has a capital-projects module, that deployment runs **ERP-led**: IFS is the system of record for project structure, budgets, forecasts/EAC, earned value, subcontracts, and change orders, and pro_manage mirrors them and owns the TPM and governance layer (CAR and gates, design reviews, MP information, work-package readiness, project losses, Benefits Realization). Write-back to the ERP is limited to operational records plus one controlled budget release per approved CAR. Deployments with a financials-only ERP run **pro_manage-led**. See `§03` and `§10`.
- **2026-10-14:** **No hard-coded thresholds.** CAR limits, variance thresholds, SLAs, cadences, and windows are admin-configurable policy variables that can be overridden per project (`§08`).
- **2026-10-14:** **Connector SDK** (`§15`). Vendor-specific choices (IdP, CMMS, MES/QMS, e-signature) are answered by selecting or building a module at deployment; the connector contract ships in Phase 0 and the SDK is extracted from the first two production modules, after which new modules are quick to build and consistent in auth, retries, idempotency, polling, events, and testing.
- **2026-10-14:** **Site is first-class and the deployment is multi-site by design.** Time zone, currency, units, business calendar, regulatory and privacy regimes, and connector routing hang off the Site; a site in another country is a configuration, not a fork (`§03`, `§11`).
- **2026-10-14:** **Capital authorization is staged** (FEL funding, full CAR, supplemental), with the estimate behind each kept as an artifact with its class and basis. Detailed engineering happens in Execute under the full CAR, as EPCM projects actually run (`§03`, `§08`).
- **2026-10-14:** **Engineering document control and procurement status are in scope; document storage and purchasing are not.** The register, revision status (IFR, IFA, IFC, as-built), transmittals, and expediting status are pro_manage's because package readiness and project losses depend on them; files live where they live and purchase orders stay in the ERP.
- **2026-10-14:** **Risks are quantified and contingency is tied to them**, with expected-value arithmetic in year one; Monte Carlo simulation is out of scope.
- **2026-10-14:** **The capital plan cycle is in scope.** Opportunities are first-class, screened with Benefits Realization history in view, ranked onto a fiscal-year plan, and converted into projects; the funnel from opportunity to realized benefit is measurable (`§03`, `§08`).
- **2026-10-14:** **Onboarding, migration, and adoption are specified, not assumed** (`§16`): three onboarding levels, a bulk-import tool with provenance, history imported never re-approved, waves tied to roadmap criteria, the CAR gate as the adoption lever, a named change lead, and an adoption scorecard.
- **2026-10-13:** Compliance regime: ISO (specific standards TBD). Baseline is ISO-aligned practices; SOX-grade audit log is a target quality bar even without an explicit SOX obligation.
- **2026-10-13:** **Target vertical: TPM-driven process industry** (distilling, bottling, food & beverage, chemicals, similar manufacturing). The FSD is written in the vocabulary of that vertical: **CAR, FEL, EEM, Vertical Startup, Benefits Realization, MP information, AACE Estimate Class, TIC vs LCC**. pro_manage is not pretending to be a generic PM tool.
- **2026-10-13:** The project lifecycle maps **one-to-one** onto the six EEM stages (Concept/Planning → Design → Fabrication/Build → Installation → Commissioning → Early Management Period), with explicit gates and exit criteria at each. See §03.
- **2026-10-13:** The organizing success question for every project, from the operator's seat: *"Did the asset start up clean, and did operations and maintenance inherit something they could run?"*
- **2026-10-14:** **Pillar scope.** pro_manage is the system for the **EEM pillar and capital-scale Focused Improvement** projects. It **integrates with, and does not replace,** the operations TPM stack (CMMS, connected-worker, OEE/loss platforms such as Cryotos, Fabrico, Redzone, L2L, Augmentir, MaintainX, Fiix). It reads losses, abnormality tags, and failure history in; it hands assets and turnover packages out. Informed by the owner's TPM software market review (Oct 2026), which found no PM application structured around TPM.
- **2026-10-14:** **Losses are the common currency.** Business cases state their promises as recoverable losses (Six Big Losses, priced in dollars); Benefits Realization verifies recovery of *those* losses against production data; the portfolio can be ranked by recoverable loss.
- **2026-10-14:** **Two views of TPM, both first-class.** *TPM for the asset* (EEM, MP information, verified loss recovery on the equipment being built) and *TPM of the project* (the eight pillars applied to project delivery under an EPCM model, with the project's own losses (waiting, rework, interface clashes, unready packages, safety stops, skill gaps) measured and attacked the way a plant attacks equipment losses). Informed by the owner's EPCM-pillar framework (Oct 2026). The *project as equipment* idea is a core concept of pro_manage and is developed in `§14`.
- **2026-10-14:** **EPCM delivery is supported natively.** The owner holds contracts; the EPCM partner and contractors are participants, not viewers: they update their own work packages, answer RFIs, and submit documents under a scoped contributor role (`§04`).
- **2026-10-14:** **Asset is a first-class entity**, mirrored from the plant CMMS. MP information, startup curve, OEE, turnover, and Benefits Realization all key off the same physical asset and asset class.

## Mission

pro_manage is the single pane of glass for the company's capital-project portfolio. From a $25,000 refurbishment to a $250,000,000 greenfield build, every project lives in pro_manage with the same shape: one owner, delegated team access, structured budgets and schedules, continuously-captured metrics, formal approval gates (CAR), and institutional memory that survives staff turnover and asset generations.

The organizing lens is **TPM / EEM**. A project is not just "money → schedule → done." It is **MP information harvested from running assets → design that incorporates it → vertical startup → benefits realization**. pro_manage makes that loop legible, auditable, and inheritable. Lessons learned at closeout flow directly into the governance substrate (MDEMG) and resurface automatically when a similar piece of equipment enters Design on the next project.

Governance is not bolted on. Every meaningful action (approving a CAR, re-baselining a budget past an AACE threshold, running a design review against the seven "easy-to" criteria, closing out an Early Management Period) flows through pro_manage's workflow engine with advisory guidance surfaced from MDEMG at the moment of decision. Nothing important happens without a record, and nothing important happens without the relevant policy, prior decision, or lesson learned being surfaced to the person acting.

The direct scorecard for every project, from a plant manager's seat: **the startup curve** (did we hit nameplate?), **the turnover package** (did Ops and Maintenance inherit something they could run?), and **benefits realization** (did the business case actually happen 6–12 months later?). pro_manage tracks all three as first-class.

## Problem statement

Today, at this company and in companies like it, capital projects are managed across a patchwork:

- **Budgets** live in Excel workbooks that diverge from accounting actuals pulled from the ERP.
- **Schedules** live in Primavera, MS Project, or Smartsheet, rarely synchronized with the budget workbook.
- **Approvals** happen in email threads with attached PDFs, with the audit trail distributed across inboxes.
- **Metrics** are pulled manually from plant historians, SCADA, QMS, and MES systems for weekly reports, with the pull method and transformation re-invented each cycle.
- **Decisions and their justifications** evaporate. Three years into a project, nobody remembers why a specific vendor was chosen or why a scope item was de-scoped.
- **Visibility** depends on who you cc. Executive stakeholders see slides prepared for last Tuesday's meeting; the state of the project between meetings is opaque.
- **Access control** is binary ("you have the SharePoint folder link or you don't"), when real projects need per-section, per-document, per-field distinctions (budgets to Finance, salaries to HR, drawings to Engineering, safety reports to HSE).

The cost: slow approvals, forecast surprises, re-litigated decisions, audit findings, and a dependency on specific individuals who hold the state of the project in their head.

## What pro_manage is

- A **project management system** with full lifecycle support from initiation to closeout.
- A **data curation platform** that lets the company harvest metrics from existing systems of record without re-entering data, and present them in context next to the decisions they shaped.
- A **metric tracking application** with first-class support for financial, schedule, scope/risk/issue, safety/quality/compliance, resource utilization, TPM / plant-operations measures (OEE, MTBF, startup curve), and priced losses.
- An **approval and audit backbone** with formal state machines for the CAR, baseline changes, design reviews, CQV, shutdown & tie-in plans, Early Management exit, closeout, and Benefits Realization, backed by a SOX-grade audit log.
- An **oversight-governed system** where advisory guidance from MDEMG surfaces at every decision gate: not as a replacement for pro_manage's authoritative rules, but as institutional memory and comprehension-tracked recommendation at the point of action.

## What pro_manage is not

- **Not an ERP.** Financial actuals come from the ERP via read-only connectors. GL postings, invoice processing, and procurement workflow stay in the ERP.
- **Not a schedule-tool replacement.** Primavera / MS Project / Smartsheet schedules are first-class imports but pro_manage does not aspire to be a critical-path planner.
- **Not an HR system.** Resource utilization is tracked at a project-allocation level; names, titles, and reporting lines come from the identity provider.
- **Not a document management system.** Documents are stored, versioned, and permission-filtered within pro_manage, but SharePoint/Drive/Confluence remain the canonical home for large collaborative docs when they already live there; pro_manage links and mirrors permissions.
- **Not an operations TPM platform.** Autonomous Maintenance (CIL/CILT) routines, abnormality tagging, Andon, shift huddles, daily management, and live OEE boards belong in CMMS / connected-worker / MES tools. pro_manage reads their losses, tags, and history and hands them turnover packages; it does not rebuild them.
- **Not a general-purpose "projects for any purpose" tool.** The domain model is tuned to capital projects. Software sprint tracking, marketing campaigns, and operational tickets belong in tools built for them (Jira, Asana, ServiceNow).

## In-scope capabilities (MVP + first year)

1. Project CRUD with lifecycle states mapped to **EEM stages** (Initiate≈Concept → Plan≈Design → Execute≈Fabrication/Install/Commission → Monitor≈Early Management Period → Close → Archived) with declared **gates and exit criteria** per stage.
2. **Project typing** (Greenfield, Brownfield expansion, Reliability, Efficiency, Compliance, Capacity debottleneck) and **TPM-pillar tagging**; hazardous-area classification + regulatory stamps on every project.
3. **FEL stage tracking** (FEL1 → FEL2 → FEL3) during the pre-authorization phase with AACE Estimate Class progression (Class 5 → Class 3 at CAR submission → Class 1 at execution).
4. Full-ACL team membership (per section, per document, per field) with owner delegation; operator + maintenance participation in **design reviews** is a first-class flow.
5. Budget management: TIC (Total Installed Cost) vs LCC (Lifecycle Cost); planned baseline with AACE class, approved baseline(s), actuals (connector-pulled from ERP), forecast (EAC), variance, **contingency drawdown**.
6. Schedule management: milestones, baseline vs actual, slip detection, dependency enforcement; **shutdown & tie-in planning** as a first-class workflow for brownfield projects. Earned-value (CPI, SPI, BCWP) when inputs are present.
7. Scope: WBS, deliverables, completion status, acceptance tracking. Risk register, issue log, change requests.
8. **Design review** workflow with the seven "easy-to" criteria (operate, maintain, clean, change over, inspect, keep safe, make quality product); findings and dispositions tracked.
9. **MP Information harvest** at closeout and periodically from running assets; feeds the MDEMG governance space as institutional learning that surfaces in future Design-stage guidance.
10. **CQV / FAT / SAT records** (Commissioning, Qualification, Validation; Factory and Site Acceptance Tests).
11. **Turnover Package**: equipment data, spares, PM tasks, procedures, cleaning procedures, calibration certs, standard work; writable to the plant **CMMS** via CMMSi.
12. **Early Management Period** with configurable exit criteria (startup curve, sustained OEE, quality holds).
13. **Benefits Realization window** (typically 6–12 months post-startup; length set per project by the `benefits.window_days` policy variable) with ROI calculated three times: business case, closeout, benefits realization.
14. Safety/quality/compliance tracking: incident counts, inspection records, regulatory sign-offs. Pulled from MES/WMS/QMS via HSEi.
15. Resource utilization: project allocation of named individuals and equipment; over/under-allocation detection.
16. Approval workflows: multi-stage **CAR (Capital Authorization Request)** by threshold; budget baseline changes; design-review sign-off; milestone sign-off; CQV sign-off; change requests and change orders; RFI responses and submittal reviews; work package and hold point releases; shutdown & tie-in plans; Early Management exit; closeout; Benefits Realization review. Routing, delegation, SLA.
17. Document management with permission mirroring from external document stores where used.
18. Live stakeholder dashboards (shared URLs, auto-refreshing, role-filtered).
19. Scheduled exports (PDF + PPTX) to distribution lists.
20. On-demand deck generation ("Build steering deck") from live data for upcoming meetings.
21. SSO (OIDC + SAML) with break-glass local admin accounts for recovery.
22. Full audit log of state-changing actions (user, action, before/after, timestamp, approving authority, MDEMG request id correlation).
23. MDEMG integration: `/jiminy/guide` on resource open, `/jiminy/classify` on gated submits, `/jiminy/feedback` on outcome. **Flagship use case: MP information harvested at closeout surfaces during Design-stage reviews for similar equipment on later projects.**
24. Spreadsheet import/export for budget workbooks, metric tables, and status roll-ups (xlsx and csv, with UATS-governed schema contracts).
25. Read-only connectors to relational databases (Postgres, Oracle, SQL Server) and time-series databases (Timescale, InfluxDB, OSIsoft PI, Historian) for metric harvest.
26. Email + Teams + Slack notifications: approval prompts, overdue nudges, status digests.
27. **Pluggable connector framework** with concrete year-one implementations:
    - **ERPi:** admin-configurable ERP connector. SAP (ECC/S/4HANA), Oracle (EBS/Fusion), NetSuite, Dynamics, Odoo.
    - **IdPi:** SSO connector with OIDC and SAML abstractions. Entra ID, Okta, ADFS, generic OIDC/SAML.
    - **Sigi:** e-signature connector. DocuSign, Adobe Sign, OneSpan.
    - **Schedulei:** schedule-tool import/export. MS Project, Primavera P6, Smartsheet.
    - **Doci:** document-store connector with permission mirroring. SharePoint, Google Drive, Confluence.
    - **Commsi:** notification delivery. Email (SMTP), Microsoft Teams, Slack.
    - **Metrici:** read-only metric harvest connectors. Relational, time-series, OSIsoft PI, OPC UA, generic Historian.
    - **HSEi:** MES/WMS/QMS connectors for safety, quality, and compliance signals.
    - **CMMSi:** CMMS read/write for MP-information harvest and turnover-package delivery. Maximo, SAP PM, Fiix, Infor EAM, Mapcon, MaintainX, Cryotos.
28. **Asset registry** (Asset / AssetClass), mirrored from the CMMS once in service; MP information, startup curve, turnover, and Benefits Realization key off the asset.
29. **Priced losses** (Six Big Losses + site loss tree) harvested from the operations TPM stack; business cases promise loss recovery against a CAR-time baseline; Benefits Realization verifies recovery; portfolio ranked by recoverable loss.
30. **Project health reviews:** recurring, pre-filled from live data, ending in owned actions.
31. **Phone-first field flows** (Phase 2): CQV/SAT walkdowns, punchlist capture, turnover inspections, hold-point inspections, and deviation tagging, with photo/QR proof and offline sync.
32. **Work packages with readiness** (Advanced Work Packaging style): engineering, procurement, and construction packages released only when information, materials, resources, and HSE prerequisites are in place; plan compliance tracked with variance reasons.
33. **EPCM project-administration objects:** contracts, RFIs, submittals, and change orders with cycle-time tracking, so waiting on information and approvals is measured rather than felt.
34. **Project losses:** a project-side loss taxonomy (waiting, rework, interface clash, unready package, safety stop, skill gap), priced where possible, feeding health reviews and focused-improvement work.
35. **Built-in quality during construction:** inspection & test plans with hold points, non-conformance data fed back into specifications.
36. **EPCM / contractor contributors:** external participants with edit rights scoped to the packages under their contract.
37. **Project-team competency matrix** (Phase 3): role competencies for owner, EPCM, and contractor staff, alongside the operator certification gate.
38. **Site registry and multi-site operation:** time zone, currency, units, business calendar, regulatory and privacy regimes, loss tree, and connector routing per site; policy variables scoped per site; cross-currency portfolio roll-ups with the rate recorded.
39. **Staged capital authorization:** FEL-funding, full, and supplemental CARs routed on cumulative amount, each backed by an estimate artifact (AACE class, basis of estimate, accuracy range); ERP budget release per approved CAR.
40. **Engineering document control:** master document register per package, revision status (IFR, IFA, IFC, as-built), transmittals in and out, IFC gated by detailed design review; feeds package readiness, turnover as-builts, and `waiting_information` losses. Not storage: files stay in MinIO, SharePoint, or an EDMS via Doci.
41. **Procurement and expediting status** for long-lead equipment and bulk materials against site-need dates, with delivery float feeding package readiness and `waiting_material` losses; purchase orders and receipts stay in the ERP.
42. **Risk quantification and contingency coverage:** three-point cost and schedule impacts on material risks, contingency basis recorded on every baseline, drawdowns naming the risk or change that consumed them, and contingency coverage (remaining vs expected value of open risks) on every health review.
43. **Opportunities and the capital plan cycle:** opportunity intake from the loss ranking, site requests, compliance obligations, and asset end of life; screening with hurdle policy and Benefits Realization history; fiscal-year capital plan with envelope, ranking, approval chain, and mid-year revisions; conversion to projects; funnel metrics.
44. **Onboarding and migration tooling:** onboarding levels, registration and history workbooks, bulk import with validation, dry run, idempotency, provenance, and reconciliation; adoption scorecard and feedback (`§16`).
45. **Charts and graphs built by users:** a chart builder over any data the user can see (metrics, plant and project losses, funnel, budget and schedule series, entity lists, imported workbook tables), with Pareto, S-curve, waterfall, timeline, startup curve, and the base types; saved as definitions that render identically on dashboards, in decks and exports, and as PNG / SVG / PDF / CSV downloads with a provenance footer (`§09`).
46. **In-app inbox and search:** one action list per person across every workflow and field item (`§08`); one role-filtered search box across projects, packages, documents, decisions, and people (`§09`). Accessibility to WCAG 2.1 AA (`§11`).

## Explicitly out of scope (first year)

- Multi-tenant hosting (single-company deployment).
- Native mobile apps. Field work is served by phone-first web views (Phase 2, `§09`); the rest of the app is responsive web.
- Public API for third-party developers (internal gRPC + REST are internal only in year one).
- In-app GL / invoice posting (that remains in the ERP).
- Automated contract redlining or e-signature (DocuSign or similar is called out as an integration seam, not an internal feature).
- Machine-learning-driven forecasting (we present data; forecasting lives in future phases).
- Monte Carlo cost and schedule risk simulation (P50/P80 contingency). Year one uses expected-value arithmetic on quantified risks; a simulation can be added later on the same data.
- Purchasing itself (RFQs issued, bids compared, purchase orders placed, goods receipted, invoices matched). pro_manage tracks procurement status and expediting; the ERP performs procurement.
- Engineering document storage and CAD management. pro_manage keeps the register, revision status, and transmittals; the files live in MinIO, SharePoint, or an EDMS.

## Success criteria

| Dimension | Target in year one |
|---|---|
| **Adoption** | ≥90% of in-flight capital projects >$1M onboarded within 6 months of production launch; ≥60% of projects $100K–$1M by end of year one. |
| **Visibility** | Stakeholder dashboards replace ≥80% of recurring hand-prepared status decks. Median time from a metric landing in a source system to appearing in a pro_manage dashboard: <15 minutes. |
| **Approval cycle time** | Median time-in-flight for **CAR** approvals reduced by ≥40% vs. current email-driven baseline. |
| **Startup curve** | For greenfield / major brownfield projects, time-to-nameplate measured and recorded; ≥50% of in-year startups meet their declared Early Management exit criteria without extensions. |
| **Benefits realization** | ≥70% of projects >$1M complete a formal Benefits Realization review 6–12 months post-startup, with actuals-vs-business-case variance quantified and recorded. |
| **MP information flow** | ≥90% of closeouts generate at least one MP-information observation into the governance space; measurable rate of Design-stage guidance referencing MP observations from prior projects. |
| **Forecast accuracy** | Baseline-to-actual variance surfaced within 24 hours of actual landing in the ERP (connector lag budget). |
| **Audit** | Zero audit findings tied to missing or unreconstructable approval chains in the year-one internal or ISO audit of in-scope projects. |
| **Governance adoption** | ≥90% of governed submits (CAR > threshold, baseline change > variance threshold) show evidence of Jiminy guidance consulted (via `guidance_id` referenced in the audit record). |
| **User satisfaction** | Net Promoter Score ≥ 40 among project owners after 6 months of production use. |
| **Operational** | p99 of core page loads ≤ 2.0s; MDEMG-dependent gates (classify) resolved within `mdemg.classify.timeout_ms` (default 10 s) when run inline and ≤ 1.5 s when pre-classified, with at least 80% of Tier A submits hitting the pre-classified path; no AI inference leaves the intranet; application monthly availability ≥ 99.5%. |

## Risks that shape scope

- **MDEMG beta status.** We pin against MDEMG JSON field shapes rather than specific constraint codes or wire formats, and keep an adapter layer so a MDEMG wire-format change touches one package. Fail-closed policy is tiered so a MDEMG outage degrades the application, not halts it.
- **ACL complexity.** Full per-section/doc/field ACL is powerful and expensive. Section 04 constrains the model to a tractable implementation (declarative scopes + inheritance) rather than a free-form ACL per node.
- **200+ projects on a single MDEMG instance.** Supported by the research but needs load validation at ~50 projects before confirming headroom.
- **Connector reliability.** Each ERP/MES/historian connector is a potential source of silent-drift bugs. UNTS hash integrity on connector schemas + UATS contract tests on each connector endpoint from day one.

## Open items for Section 01

- Identify the company's CMMS, MES/QMS, and IdP so the matching `<Type>i_<CODE>` modules are built first (the ERP is known: IFS Cloud). Not a design blocker: the modular connector approach means the core is built against connector contracts, not vendors.
- The consolidated, owner-facing list of every open item across the FSD, grouped by what each unblocks, is [open-items.md](./open-items.md).
- Confirm specific schedule tools in use (which Schedulei backends to prioritize).
- Confirm MES/WMS/QMS vendors in play (which HSEi backends to prioritize).
- Confirm which ISO standards are binding (ISO 9001, ISO 55000, ISO 27001, industry-specific). Shapes retention, access-review cadence, and change-control detail in §08 and §11.
- Confirm the company's identity provider so IdPi's first production backend is right. OIDC + SAML abstractions proceed regardless; this is about which backend is "day one."
