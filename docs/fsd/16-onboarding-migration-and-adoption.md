# 16: Onboarding, Data Migration, and Adoption

## Decisions on record

- **2026-10-14:** **Onboarding is a product feature, not a project task.** pro_manage ships the bulk-import tool, the onboarding levels, the reconciliation reports, and the adoption scorecard described here; they are built in Phase 2 and used through GA (`roadmap`).
- **2026-10-14:** **History is imported as history, never re-approved.** Approvals, baselines, and decisions that happened before a project entered pro_manage are loaded as `imported` records with their provenance (source document, date, who approved) and are visibly distinct from decisions made in the system. Nobody re-signs a CAR that was approved in an email three years ago.
- **2026-10-14:** **Projects onboard in waves, at the level they need.** Three onboarding levels (Registered, Governed, Full) let a $60,000 refurbishment live in pro_manage with ten fields while a $200M greenfield carries everything. Waves follow the roadmap's success criteria: pilot, then projects above $1M, then the rest.
- **2026-10-14:** **The CAR gate is the adoption lever.** From a date the Policy Owner sets, a CAR is only approvable in pro_manage. That one rule pulls every project above the threshold into the system, with its business-case promises, at the moment the company already pays attention to it. Everything else (status decks, health reviews, document control) follows because the project is already there.
- **2026-10-14:** **Change management has an owner and a budget line.** A named change lead (0.5 FTE from Phase 1), persona champions, and a training set per persona are in the team shape, not assumed.

## Why this section exists

The review of the FSD found that the plan said what pro_manage would be and said nothing about how 200 in-flight projects, their spreadsheets, their email approvals, and the people who run them would get into it. That is the part most internal systems fail at. This section is the plan.

## Onboarding levels

Every project is at one of three levels. The level is a field on the project, visible on dashboards, and a project moves up as the owner fills what the next level needs.

| Level | What exists in pro_manage | Who it is for | Minimum to reach it |
|---|---|---|---|
| **Registered** | Site, code, name, owner, project type, lifecycle state, authorized amount, a milestone or two, a link to wherever the real records live | Small projects; everything in flight on day one | 10 fields; five minutes with the registration form or one row in the registration workbook |
| **Governed** | Registered plus: the CAR(s) with estimate artifact and business-case promises, current budget baseline (or the ERP mirror), key milestones, risk register headline, the approval chain that applies | Any project that will ask for money or re-baseline while pro_manage is live | The CAR workbook import or an in-system CAR; promises with a baseline |
| **Full** | Everything the FSD describes for its lifecycle stage: design reviews, MP information, work packages, document register, procurement items, health reviews, Benefits Realization | Projects above the policy threshold; every project that starts in pro_manage from Initiate | The stage's gate pre-conditions in `§08` |

Policy: `onboarding.required_level.<project_type_or_amount>` sets the level a project must reach, for example `Governed` above `{car.threshold.finance_approver}` and `Full` above `{car.threshold.vp_finance}`. Below those, Registered is enough and nothing nags.

## Data migration

### Sources

| Source today | What it holds | How it enters pro_manage |
|---|---|---|
| Budget workbooks (xlsx) | Baselines, line items, contingency, forecasts | Budget-baseline import template (`§07`), one workbook per project, with the baseline's AACE class and date asked for on the cover sheet |
| The ERP (IFS Cloud in the first deployment) | Project structure, budgets, forecasts, actuals, commitments, subcontracts, change orders | Not migrated: mirrored through `ERPi_IFS` on first sync; the project is matched to its ERP id in the registration workbook |
| CAR packages (PDF, email threads, signed scans) | Authorized amounts, approvers, dates, business-case narratives | CAR import template: one row per authorization with amount, kind, date, approvers, and the scanned package attached; loaded as `CapitalAuthorization` rows with `status = approved`, `imported = true`, and the scan as the signed artifact |
| Schedules (MS Project, P6, Smartsheet) | Milestones, dependencies | `Schedulei` import, or the milestone workbook when the tool is not connected yet |
| Risk registers (xlsx) | Risks, owners, scores | Risk-register import template |
| Document registers (EPCM MDR in xlsx or an EDMS) | Document numbers, revisions, issue status | Register import template, or Doci mirroring where the EDMS is connected (`§10`) |
| Status decks (pptx) | Narrative history | Not migrated. The last deck is attached to the project as a document; history starts in pro_manage |
| Lessons-learned folders | MP information nobody reads | Harvested deliberately: the MP-information capture form is run against the folder for the asset classes the next projects need (`§06`), as a one-time governance-space seeding exercise |

### The bulk-import tool

- Workbook templates for registration, CAR history, budget baseline, milestones, risks, and document register, each governed by a UATS spec on its import endpoint with hashed fixture workbooks (`§12`).
- Validation before commit: every row either imports cleanly or reports its cell-level errors; a workbook with errors imports nothing. Error rate target in the roadmap: under 2% invalid after validation.
- Dry run: the tool shows what would be created, matched (to an existing project, asset, or ERP record), or skipped, before anything is written.
- Idempotent: re-importing the same workbook updates rather than duplicates, keyed on the project code and the row's natural key.
- Provenance: every imported row carries `imported = true`, `import_batch_id`, `source_file_sha256`, and `source_ref` (sheet and row). Imported approvals produce `AuditEvent` rows of `action_type = import.*`, with the importer as actor and the original approver and date in the payload, so the audit chain is honest about what happened in the system and what was loaded into it.
- Reconciliation report per batch: imported totals vs the ERP mirror (authorized vs budget, actuals vs workbook), with the differences listed for the owner to resolve or accept with a reason.
- Who runs it: project owners for their own projects at Registered and Governed levels; a migration team (Product/PMO plus the change lead) for the wave imports; Platform Admin for registration workbooks across a portfolio.

### Cut-over rules

- **History is imported, decisions are made.** Imported records are read-only except for pro_manage-only attributes. Correcting an imported record is a new, audited edit with a reason; the imported values stay in the row's history.
- **Baselines:** the current approved baseline is imported as `BudgetBaseline.current = true` with its class and date; earlier baselines are imported only if the owner wants variance history. In the ERP-led profile, the baseline is the ERP's and the import only adds the pro_manage attributes.
- **Approvals in flight at cut-over** finish where they started. A CAR half-way through an email chain on the cut-over date is approved by email and then imported; the next one starts in pro_manage.
- **Lifecycle state** is set at import from the owner's declaration and the dates on record; gate pre-conditions are not retro-checked. A project imported in `Execute` is not asked for a basic design review it held in 2024.
- **Promises:** projects imported at Governed level or above must state their business-case promises, even approximately, because Benefits Realization will ask. Where no baseline production data exists, the promise is marked `baseline_estimated` and the Benefits Realization review says so.
- **Assets:** in-service assets come from the CMMS mirror; planned assets on imported projects are created from the project's equipment list workbook or added by the owner as the project reaches Full level.
- **MDEMG spaces:** a project space is created at import; imported history is written as `decision` observations with `imported` tags so Jiminy's guidance can cite it, and the governance space is seeded from the company's existing capital policy and the lessons-learned harvest before the first wave.

### Waves

| Wave | When (roadmap) | Who | Level | Exit |
|---|---|---|---|---|
| 0: Pilot | Phase 1 | 3 real projects with the pilot team | Full | Pilot-Readiness Review |
| 1: Governed core | Phase 2 | Every in-flight project above `{car.threshold.finance_approver}` | Governed; Full for those above `{car.threshold.vp_finance}` | ≥ 50 projects; CAR-only-in-pro_manage date announced |
| 2: Everything in flight | Phase 3 | All remaining in-flight projects | Registered minimum | ≥ 90% of projects above $1M at Governed or Full |
| 3: Everything new | Phase 4 to GA | Every new project from Initiate | Per policy | 100% of CARs approved in pro_manage |

Each wave has a migration playbook: the workbook set, the owner briefing, the validation session, the reconciliation review, and the sign-off that the project's owner accepts the imported state.

## Adoption and change management

### Who owns it

- **Executive sponsor:** the Capital Engineering leader. Sets the CAR-only date, chairs the adoption review, and removes the legacy reports.
- **Change lead (0.5 FTE from Phase 1):** owns this section's plan, the training set, the champion network, and the adoption scorecard.
- **Persona champions:** one project owner, one finance approver, one site operations or maintenance lead, one EPCM contributor, one construction manager, recruited from the pilot. They are the first line of help and the source of the "what's in it for me" stories.
- **Policy Owner and Governance Author:** load the DoA and the capital policy before wave 1 so the first governed CAR routes correctly and the first guidance card says something the approver recognizes.

### What's in it for each persona

Adoption follows self-interest. The table is the message, per persona, and the thing each one stops doing.

| Persona | What they get | What they stop doing |
|---|---|---|
| Project Owner | One place for the CAR, the baseline, the health review, and the steering deck; MP information from the last similar asset shown while designing | Rebuilding status decks; chasing approvals by email; re-explaining the project's history |
| Finance Approver | CARs with the estimate class, the promises, and the chain visible; thresholds that match the DoA; a signed artifact in the audit record | Reading approval chains out of email; reconciling workbook totals to the ERP by hand |
| Portfolio Executive | A live portfolio ranked by recoverable loss, a capital plan with a funnel behind it, decks generated from live data | Waiting for last Tuesday's slides |
| Site operations and maintenance | A seat in the design review, a turnover package that lands in their CMMS, an Early Management exit they co-sign | Inheriting assets nobody asked them about |
| EPCM / contractor contributors | Their packages, RFIs, submittals, and transmittals in one place with the owner's answers and SLAs visible | Parallel registers; "did you get my RFI?" |
| Construction Manager | Readiness that is computed, not asserted; a project-loss Pareto to run the week from | Discovering an unready package on Monday morning |
| Compliance / HSE | An audit chain that reconstructs any approval; hold points and inspections with evidence | Audit-season archaeology |
| Platform Admin | Connector modules that swap without a release; a framework matrix that says what is tested | Being the only person who knows how the integrations work |

### Mandatory-use policy

- From `{adoption.car_only_from}` (a policy variable, set by the Policy Owner with the sponsor's sign-off), the CAR chain in `§08` is the only route to capital authorization; Finance does not accept a CAR outside it.
- From `{adoption.health_review_only_from}`, project health reviews above the high-value threshold are recorded in pro_manage.
- From `{adoption.status_deck_sunset}`, the legacy status deck template is retired for projects at Governed level or above; the scheduled export (`§09`) replaces it.
- Each date is announced one wave ahead, with the champions' stories attached.

### Training

Per persona, short, and tied to the first thing each person will do:

- Project Owner: register and onboard a project; raise a CAR; run a health review (90 minutes, hands-on with their own project).
- Finance Approver: approve a CAR in the system, with a Jiminy card and a Sigi envelope (30 minutes).
- Site leads and representatives: the phone-first flows, the design review, the Early Management exit (45 minutes, on a phone).
- Contributors: the contract-scoped view, package progress, RFIs, submittals, transmittals (45 minutes, delivered to the EPCM's team as part of their onboarding).
- Construction Manager: readiness, release, hold points, the reliability dashboard (60 minutes).
- Platform Admin and Policy Owner: modules, policy variables, pools, the framework matrix (half day).
- Governance Author: constraint authoring and effectiveness review (half day, with the MDEMG maintainer).

Training materials ship with each phase (roadmap, Phase 4 documentation hardening) and are versioned with the release they describe.

### Support model

- In-app help on every governed form (what this gate needs and why), linked to the FSD section.
- Champions as first line, the change lead as second, the product team as third; a weekly office hour during each wave.
- A feedback entity: `AdoptionFeedback` (`user_id`, persona, screen, free text, severity), triaged weekly; the top three friction points per wave become roadmap items.

### Adoption scorecard

Measured monthly from pro_manage's own data and shown on an admin dashboard; reviewed at the adoption review chaired by the sponsor.

| Measure | Wave 1 target | GA target |
|---|---|---|
| Projects above $1M at Governed or Full level | ≥ 25% | ≥ 90% |
| CARs approved in pro_manage (share of all CARs) | ≥ 50% | 100% |
| Status decks replaced by scheduled exports | ≥ 30% | ≥ 80% |
| Health reviews recorded in the system (above the high-value threshold) | ≥ 50% | ≥ 95% |
| Contributors active in the last 30 days (share of provisioned) | ≥ 40% | ≥ 70% |
| Governed submits with guidance consulted (`guidance_id` in audit) | ≥ 70% | ≥ 90% |
| Closeouts generating MP information | ≥ 50% | ≥ 90% |
| Project owner NPS | ≥ 20 | ≥ 40 |
| Median onboarding time per project (Registered to Governed) | ≤ 5 business days | ≤ 2 business days |

The targets are the `§01` success criteria and the roadmap's phase criteria, stated as a monthly trend rather than a year-end verdict.

### Risks specific to adoption

| Risk | Mitigation |
|---|---|
| Owners register projects and stop (Registered forever) | The CAR-only date; required level by threshold; the scorecard shows who is stuck |
| EPCM partners refuse a second system | Contributor scope is their packages only; contributors' onboarding is part of the contract; transmittals and RFIs with SLAs are a benefit to them, and the champions say so |
| Imported history is distrusted | Provenance on every row; reconciliation report; imported records visibly marked; the original scans attached |
| Finance keeps a parallel approval path "just for this one" | The mandatory-use policy is a Finance policy, signed by the sponsor and the Policy Owner, not an IT request |
| Guidance cards are noise at first | Governance space seeded from the real capital policy before wave 1; false-positive feedback loop in `§06`; tiers start at B where the constraint library is thin |

## Open items for Section 16

- Confirm the three pilot projects and the pilot team (roadmap).
- Confirm the inventory of in-flight projects (count, amount band, site, where their records live) so wave 1 can be sized; first instinct: a registration workbook circulated to owners in Phase 1.
- Confirm who the change lead is and whether the role is internal or contracted.
- Confirm the CAR-only date the sponsor is willing to set (first instinct: the start of Phase 3, announced at the end of Phase 1).
- Decide whether the lessons-learned harvest into the governance space is done by the Governance Author alone or in workshops with the site maintenance leads (first instinct: workshops, one per major asset class).
- Decide whether imported CAR scans are OCR-indexed for search (first instinct: yes, via pm-sidecar, metadata only, no content to MDEMG).
