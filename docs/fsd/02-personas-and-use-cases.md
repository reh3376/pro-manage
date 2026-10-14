# 02: Personas & Use Cases

## Decisions on record

- **2026-10-13:** Projects have exactly one Owner. Access is delegated by the Owner within an ACL that supports section-, document-, and field-level grants.
- **2026-10-13:** External stakeholders can be granted view-only access to specific dashboards without needing a full user license (shared URL + optional attestation).

## Personas

The thirteen canonical roles below cover every pro_manage interaction in year one. Role is distinct from permission: a user may hold several roles across different projects and the portfolio.

### Primary roles (people who do work in pro_manage daily)

#### Project Owner
Owns one or more projects end-to-end. Accountable for budget, schedule, scope, risk, and closeout. The single person pro_manage holds responsible for a project.

- Can create projects, invite teammates, assign per-section/doc/field ACLs.
- Approves day-to-day changes within their authority; routes above-threshold items upward.
- Can delegate ownership (hand off, with Platform Admin assist for audit).
- Sees every section of their project without exception. Receives all Jiminy guidance for actions they take.

#### Project Teammate: Editor
A person the Owner has granted edit rights to on specific sections, documents, or fields.

- Common grants: engineer edits scope + risk + schedule, scheduler edits schedule + resources, financial analyst edits budget + forecast, HSE lead edits safety + compliance tracking.
- Cannot see sections they weren't granted (budget sections hidden by default from non-finance teammates; salaries hidden from everyone except named Finance Approvers; drawings hidden from non-Engineering).
- Submits changes; some may require Owner (or above) approval depending on `§08` thresholds.
- Receives Jiminy guidance filtered to their role's scope.

#### Project Teammate: Viewer
Read-only access to specific sections of a project.

- Common grants: cross-functional reviewer, auditor-in-residence, external consultant.
- Can comment where comments are enabled; cannot change state.

#### Portfolio Executive
Reads across many projects. Does not perform day-to-day work in individual projects. May approve CARs above a certain threshold (`§08`).

- Primary surface is cross-project dashboards (`§09`) and the on-demand steering deck generator.
- Receives scheduled executive digests (`Commsi`).
- Jiminy guidance aggregates portfolio-level signals when they engage with a project directly.

#### Finance Approver
Approves budget-related gates (CAR thresholds, baseline changes). May be a chain (Director → VP Finance → CFO) depending on amount.

- Not necessarily a project teammate. Enters a project only at the point of approval, with `§08` surfacing a scoped read of the relevant sections.
- Jiminy constraints often target this persona (CAR > $X requires co-sign, variance > Y% requires re-forecast).

#### Compliance / HSE Officer
Reviews safety, quality, and ISO-compliance tracking across projects. May approve safety-related sign-offs. Often also serves as **Governance Author** (below), authoring Jiminy constraints for the org.

- Reads safety/quality sections of any project under their jurisdiction.
- Flags aged incidents, missed inspections, overdue sign-offs.

#### Site Operations & Maintenance Representatives
The people who will run and maintain the asset. EEM treats them as participants, not an audience. Four workflow roles (`§04`, `§08`), usually held by people who don't own projects:

- **Operator representative** and **maintenance representative**: take part in "easy-to" design reviews, commissioning rehearsal, and turnover inspections; author and confirm MP information.
- **Site operations lead** and **site maintenance lead**: acknowledge shutdown & tie-in plans, co-sign Early Management exit, and accept the TurnoverPackage on behalf of the site.
- Scoped to their site's projects and assets; read + comment by default, plus sign-off rights on the stages that name their role. Mostly use the phone-first field views (`§09`).

#### Commissioning Engineer / QA Lead
Runs FAT, SAT, and commissioning (and, in regulated contexts, qualification and validation). Closes `CommissioningRecord`s with pass / pass-with-punchlist / fail and owns punchlist closure. The QA lead gives final CQV sign-off where regulation requires it. Works largely in the field views.

### Platform roles (people who run pro_manage itself)

#### Platform Administrator
Operates pro_manage. Configures connector modules (ERPi, IdPi, Sigi, Schedulei, Doci, Commsi, Metrici, HSEi), manages SSO claims-to-role mapping, triggers backups, monitors health, provisions break-glass accounts.

- Does not have blanket read access to project content. Admin privileges target *configuration*, not *data*. A Platform Admin can see that Project P-7 exists and who its Owner is, but cannot read its budget unless explicitly granted. (See `§04` for the admin audit discipline that enforces this.)
- Local break-glass admin account exists for recovery scenarios when SSO is down; use is logged and reviewed.

#### Governance Author
Authors Jiminy constraints via a dedicated admin UI that fronts MDEMG's `/v1/jiminy/rules` endpoint. May be the same person as Compliance Officer or a separate role.

- Writes new constraints, retires stale ones (via tombstone), re-scopes constraints by `scope` family.
- Reviews constraint effectiveness metrics from MDEMG and tunes thresholds.
- Does *not* have blanket read access to projects; their work is in the governance space (`space=org.<id>.governance`), not individual project spaces.

#### Construction Manager
Owner-side or EPCM-side lead for site execution: sequencing, contractor coordination, package release, interface management, and site HSE. Takes part in design reviews to bring constructability and commissionability into the design (`§08`). Owns plan-compliance review for construction packages.

### External roles (people outside the owner's company)

#### EPCM / Contractor Contributor
Staff of the EPCM partner, designers, suppliers, or construction contractors working under a `Contract` (`§03`). Under EPCM the owner holds the contracts and these people act as the owner's extended team, so they need to *work* in pro_manage, not just watch.

- Scoped by contract: they see and update only the work packages, RFIs, submittals, and change orders under their organization's contract(s), plus the project sections those packages need.
- Can update package progress and readiness, raise and answer RFIs, upload submittals, raise deviation tags, record hold-point inspections, and respond to punchlist items assigned to them.
- Never see budget lines beyond their own contract value, other contractors' commercial data, or internal approvals.
- Sign in via federated identity (their own IdP trusted through IdPi) or company-issued guest accounts (`§04`).

#### External Stakeholder
A specific view-only user of a specific dashboard. Could be a contractor PM, a joint-venture partner's rep, an external investor, a regulator during an audit window.

- Access is per-dashboard, not per-project. Owner (or above) grants a dashboard URL to a specific identity (SSO) or a tokenized link (short-lived, revocable).
- Cannot see data the dashboard doesn't display, cannot act on anything.
- Access and views are logged for audit.

## Use cases (headline)

Each use case lists its primary actor, the trigger, the main success scenario in one line, and the acceptance criteria. Numbering is stable: later sections reference these as UC-NN.

### UC-01: Create project and invite team
- **Actor:** Project Owner · **Trigger:** New capital project authorized.
- **Flow:** Owner creates project, sets lifecycle state to Initiate, defines WBS skeleton, invites teammates with per-section ACL grants.
- **Accept when:** Teammates appear in Membership with correct grants; attempted access outside grants is denied and logged.

### UC-02: Approve a CAR (Capital Authorization Request) above threshold
- **Actor:** Finance Approver · **Trigger:** Submitter clicks "Submit for approval" on a $10M CAR whose business-case promises carry CAR-time baselines (`§08` pre-conditions).
- **Flow:** `Sigi` creates the envelope (DocuSign at launch); Jiminy guidance surfaces at approver's view (`/jiminy/guide`), approver signs or returns for revision, outcome recorded via `/jiminy/feedback`.
- **Accept when:** Approval chain preserved end-to-end in audit log; approver saw guidance at decision time; signed artifact stored and linked to the project.

### UC-03: Change budget baseline with variance over threshold
- **Actor:** Project Owner · **Trigger:** Owner proposes a +12% baseline change.
- **Flow:** Form render triggers `/jiminy/guide` → "Variance >10% requires monthly re-forecast cycle"; owner acknowledges; `/jiminy/evaluate` returns a warning (not a block); on submit, re-forecast task is auto-scheduled; outcome recorded.
- **Accept when:** Re-forecast task appears on the owner's queue; audit record references the `guidance_id`.

### UC-04: Submit weekly status with aged risks
- **Actor:** Project Owner · **Trigger:** Owner opens the Status Post form; three risks have been open > 14 days without update.
- **Flow:** `/jiminy/guide` returns a `warned`-level item ("Risks aged >14d must be updated before status post"); user either updates the risks and resubmits (`outcome=followed`), or submits with a dismissal reason (`outcome=ignored`). On repeated ignore, escalation ratchets to `escalated` and the next attempt is a hard block via `/jiminy/classify`.
- **Accept when:** Dismissals captured with reason; escalation state visible in the audit log; third attempt is blocked.

### UC-05: Mark milestone complete with open dependencies
- **Actor:** Project Teammate (Editor) · **Trigger:** User clicks "Complete" on M3; M2 is still open.
- **Flow:** pro_manage enforces the mechanical rule in-process (no MDEMG round-trip); error returned with the specific unmet dependency.
- **Accept when:** Mechanical rule enforced locally with <50ms latency; user sees a specific actionable message, not a generic "cannot complete."

### UC-06: External stakeholder opens a live dashboard
- **Actor:** External Stakeholder · **Trigger:** Follows a shared dashboard URL.
- **Flow:** `IdPi` challenges for identity (SSO or tokenized link); dashboard renders with role-filtered content; viewer action logged.
- **Accept when:** No section outside the dashboard scope is reachable; views are logged with viewer identity + timestamp.

### UC-07: Scheduled export to steering committee
- **Actor:** System (scheduled) · **Trigger:** Weekly cron on Friday 5pm.
- **Flow:** For each configured recipient list, pro_manage renders a PDF + PPTX from the current dashboard state and dispatches via `Commsi` (email).
- **Accept when:** All configured recipients received the artifact; render pipeline is idempotent; failures retried with backoff; delivery status visible on the dashboard config page.

### UC-08: On-demand steering deck generation
- **Actor:** Project Owner · **Trigger:** Owner clicks "Build steering deck for Tuesday's meeting."
- **Flow:** pro_manage assembles a PPTX from a template + live data (budget state, schedule state, open risks, upcoming milestones, Jiminy insights for the project), owner reviews/edits, downloads or sends via `Commsi`.
- **Accept when:** Deck generation completes in <30s for a typical project; owner can edit before send; final version is stored and linked to the project record.

### UC-09: Metric harvest from external time-series system
- **Actor:** System (scheduled, per connector) · **Trigger:** Configured poll interval or event trigger.
- **Flow:** `Metrici` pulls new values from the configured TSDB (TimescaleDB, InfluxDB, OSIsoft PI, Historian), transforms per the configured mapping, writes to pro_manage's own Timescale, updates project dashboards.
- **Accept when:** New values appear in pro_manage within 15 minutes of landing in the source; schema drift is flagged (via UNTS hash on the mapping) rather than silently passing wrong data.

### UC-10: Import a budget baseline from an xlsx workbook
- **Actor:** Project Owner (or Teammate Editor with budget edit grant) · **Trigger:** User uploads a workbook on the Budget page.
- **Flow:** Workbook is validated against a UATS-governed schema; mismatches surface inline with cell references; on accept, the baseline is committed with full before/after captured in audit.
- **Accept when:** Invalid workbooks are rejected with actionable per-cell messages; valid imports update the baseline and generate the audit record.

### UC-11: Owner delegates project to a successor
- **Actor:** Project Owner (outgoing) · **Trigger:** Role change or project handoff.
- **Flow:** Outgoing owner initiates handoff; Jiminy guidance surfaces the handoff checklist constraint; successor accepts; Platform Admin counter-approves per `§08`; audit record captures the full chain.
- **Accept when:** Successor's name appears on the project; outgoing owner's edit rights revert to their prior ACL (if any); audit record is complete and un-editable.

### UC-12: Retrieve approval chain for an old project (audit)
- **Actor:** Compliance Officer (or external auditor via a limited-scope session) · **Trigger:** Audit inquiry.
- **Flow:** User searches the audit log for the project / timeframe / action type; pro_manage returns the full chain including signed documents (via `Sigi` references), approver identities, timestamps, and the `guidance_id` references for each gate.
- **Accept when:** Chain is reconstructible for any approved action from the past 7 years (ISO/SOX retention baseline); referenced artifacts (signed PDFs, backup e-sig records) are retrievable.

### UC-13: Author a new governance constraint
- **Actor:** Governance Author · **Trigger:** Policy change (e.g., CAR threshold raised).
- **Flow:** User opens the Governance admin UI, drafts the constraint text, picks `constraint_type` + `scope`, submits. pro_manage calls `/v1/jiminy/rules` behind the write flag; if MDEMG's dedup returns a near-match, the user chooses to use the existing rule, override, or revise.
- **Accept when:** Constraint appears in guidance within 60s of creation; constraint can be tombstoned by the author (via `/v1/jiminy/rules/{code}/tombstone`); every author action is logged.

### UC-14: Governed action attempted while MDEMG is unreachable
- **Actor:** Project Owner · **Trigger:** User submits a Tier A governed action (e.g., >$5M CAR approval) during a MDEMG outage.
- **Flow:** `/jiminy/classify` times out or errors; pro_manage's edge applies the Tier A fail-closed policy; user sees "Governance system unavailable. This action requires policy classification. Options: retry, or request a Platform Admin override." Admin override is a separate logged action.
- **Accept when:** Tier A action is refused in a reachable UI; Tier B actions degrade with a visible warning but proceed; Tier C actions proceed silently; retry attempts succeed once MDEMG returns.

### UC-15: Project closeout
- **Actor:** Project Owner · **Trigger:** Owner marks the project ready for closeout.
- **Flow:** Jiminy surfaces the closeout checklist constraint ("Projects at >80% schedule complete must have an approved closeout plan on file"); outstanding items are listed; Owner completes them; final sign-off chain runs via `Sigi`; project moves to Closed state; archive view becomes read-only.
- **Accept when:** All constraint-required items are satisfied before closeout; the final audit record is complete and un-editable; downstream readers (dashboards, external viewers) continue to resolve historical data.

### UC-16: Run a project health review
- **Actor:** Facilitator (peer PM, portfolio lead, or CI lead) with the Project Owner · **Trigger:** Monthly cadence, a `red` flag, or a Portfolio Executive request.
- **Flow:** pro_manage pre-fills the agenda from live data (scope stability, AACE class vs stage, critical-path float, top risks, change-order trend, contingency burn vs % complete, Ops/Maintenance turnover readiness); the group adds the "what worries you that isn't on the register?" items; actions are assigned with owners and dates.
- **Accept when:** A `ProjectHealthReview` is recorded with owned actions; a `red` result notifies the Portfolio Executive.

### UC-17: Field walkdown with punchlist capture
- **Actor:** Commissioning Engineer with operator and maintenance representatives · **Trigger:** SAT or turnover inspection scheduled on an asset.
- **Flow:** On a phone, the engineer scans the asset QR, steps through the protocol, attaches photos per step, and records pass / fail / punchlist items with owners. Works offline; syncs on reconnect.
- **Accept when:** The `CommissioningRecord` and punchlist items exist with photo evidence; punchlist items block Early Management exit until closed with an "after" photo.

### UC-18: Verify benefits realization against the business case
- **Actor:** Project Owner, then Finance Approver and Portfolio Executive · **Trigger:** Benefits Realization window closes.
- **Flow:** pro_manage compares priced losses and promised KPIs on the promised assets during the window against the CAR-time baselines; the owner writes the variance narrative; Finance validates; the Portfolio Executive approves closure; a `decision` observation goes to the MDEMG governance space for similar future projects.
- **Accept when:** Every `BusinessCasePromise` has a realized / partially realized / not realized status with numbers on its original measurement basis; the project can move to `Archived`.

### UC-19: Raise and answer an RFI
- **Actor:** Contractor Contributor raises; EPCM or owner engineer answers · **Trigger:** A drawing conflicts with site conditions.
- **Flow:** The contributor raises an RFI against the affected work package with photos; it routes to the responsible discipline engineer with an SLA; the answer is recorded, linked to any resulting drawing revision or change order, and the package's readiness updates.
- **Accept when:** RFI age and cycle time are tracked; the time the package spent waiting is recorded as a project loss (`waiting_information`); the answer is visible to everyone on that package.

### UC-20: Release a work package
- **Actor:** Construction Manager · **Trigger:** A construction work package is scheduled to start next week.
- **Flow:** pro_manage shows the readiness checklist: approved-for-construction drawings, open RFIs, materials on site, crew and equipment, permits, HSE prerequisites, hold points defined. Unmet items block release (or need a logged override); release notifies the contractor.
- **Accept when:** Packages are only released when ready or with an audited override; weekly plan compliance (released and completed vs planned) is recorded with variance reasons.

### UC-21: Contractor updates package progress
- **Actor:** Contractor Contributor · **Trigger:** End of shift or week.
- **Flow:** On a phone, the contributor updates quantities installed, flags deviations, and closes assigned punchlist items with photos; the owner's team reviews.
- **Accept when:** Progress feeds earned value and plan compliance; the contributor saw only their own contract's packages throughout.

## Key journeys

Journeys are end-to-end narratives that cross multiple use cases and give sections 03–09 a concrete reference point.

### Journey A: Project lifecycle (Initiate → Close)
UC-01 → UC-03 (several times) → UC-02 → UC-04 (weekly) → UC-05 (as milestones close) → UC-17 (commissioning and turnover walkdowns) → UC-15 → UC-18. Threaded through: UC-16 health reviews during Plan and Execute, continuous UC-09 metric and loss harvest, UC-13 constraint evolution, UC-12 occasional audit retrievals.

### Journey B: Approval cycle (CAR > threshold)
Submitter opens request → `/jiminy/guide` warms → Finance Approver receives notification via `Commsi` → opens request → `/jiminy/guide` returns high-priority item (CAR > $5M needs VP Finance co-sign) → approver reviews → either returns for revision or signs via `Sigi` (UC-02) → `/jiminy/feedback` records outcome → audit record finalized.

### Journey C: Weekly status (UC-04 narrative in full)
Owner opens Status Post form → `/jiminy/guide` surfaces aged-risk warning + any other applicable items → owner either resolves issues (updates risks, closes items) and submits (`outcome=followed`), or submits with dismissal reasons (`outcome=ignored`) → next week's attempt sees `warned` escalation state if ignores accumulate.

### Journey D: Presentation flow
Live dashboards (UC-06) run continuously → scheduled exports (UC-07) fire weekly to steering → on-demand decks (UC-08) prepared for named meetings → audit log captures every send + every external viewer session.

## Open items for Section 02

- Confirm whether external stakeholders will primarily use their own corporate SSO (federated to IdPi) or tokenized shared links. Likely both; priority order affects `§04` and `§06`.
- Confirm whether a "Portfolio Manager" role separate from Portfolio Executive is needed (e.g., a PMO lead who allocates portfolio-wide resources but doesn't approve). Could fold into Portfolio Executive if ACL grants are enough.
- Determine whether read-only auditors need a time-boxed session type (automatic access expiry after a window) or standard access with manual revocation.
