# 09: Presentations & Reporting

## Decisions on record

- **2026-10-13:** Presentations are a top-value function with three delivery modes shipping day one: **live dashboards, scheduled exports, and on-demand deck generation**.
- **2026-10-13:** Dashboards are **composed of tiles** bound to the metric/entity model (`§03`, `§07`). Tiles are role-filtered at render time.
- **2026-10-13:** Exports (PDF + PPTX) are rendered by **pm-sidecar** using versioned templates. The authoring source of truth for the template is Git-tracked; the active deployed template is cached in Postgres with a hash.
- **2026-10-13:** External stakeholder sharing is via **tokenized dashboard links** (short-lived, revocable) with per-view attestation and audit, as defined in `§04`.
- **2026-10-14:** Added **Loss & opportunity** and **Startup & Early Management** dashboard presets, and **mobile field flows** (CQV/SAT walkdowns, punchlists, turnover inspections, design-review input) with photo/QR proof, Phase 2.
- **2026-10-14:** Added a **Project reliability** preset (project-loss Pareto, package readiness, plan compliance, RFI/submittal aging) and contractor-facing field flows (progress updates, deviation tagging, hold-point inspections).
- **2026-10-13:** All charts follow a single, cohesive visual language (`dataviz` skill guidance), with a validated palette, light/dark mode, and consistent typography.

## Dashboards

### Dashboard model

- `Dashboard`: `dashboard_id`, `name`, `description`, `bindings` (list of `{project_id | portfolio_id}` the dashboard scopes to), `layout` (grid), `tiles[]`, `owner_user_id`, `visibility` ∈ `{project, portfolio, global}`, `share_links[]`, standard audit stamps.
- `Tile`: `tile_id`, `kind` (`kpi | timeseries | category | tabular | gauge | status_grid | map`), `title`, `data_binding` (metric_id + aggregation + filters, or query-shape for tabulars), `role_visibility` (optional restriction), `size` (`1x1 | 2x1 | 2x2 | 4x2 | …`).

### Dashboard types (year-one presets)

- **Project overview:** one per project. Shows headline KPIs (budget variance, schedule slip, open high risks, overdue approvals), milestone timeline, recent decisions, Jiminy insights.
- **Executive portfolio:** one per portfolio. Grid of projects with health indicators, drill-down. Aggregates CapEx committed, forecast variance, projects at risk.
- **Finance:** budget-heavy view. Baseline vs actuals vs EAC across projects, cash-flow curves, approvals in flight.
- **HSE:** safety and compliance across projects. Incidents, inspection pass rate, aged findings, upcoming sign-offs.
- **Resources:** allocation heatmap, overallocation flags, equipment utilization.
- **Loss & opportunity:** portfolio ranked by recoverable loss: top priced losses by asset and Six Big Losses bucket, which are already addressed by an open project, and which are not (FEL1 candidates). For closed projects, promised vs verified recovery. Gives Finance, CI, and Engineering one shared priority list.
- **Capital plan & funnel:** the fiscal-year plan by portfolio: envelope vs planned vs authorized vs forecast per line, the opportunity funnel (proposed → screened → planned → converted → realized) with values, unplanned projects, carry-over, and the screening backlog ahead of the submission deadline. The Portfolio Executive's and PMO Lead's working view.
- **Adoption scorecard (admin):** onboarding level distribution, CARs in-system share, deck replacement, contributor activity, guidance consulted, and the other `§16` measures as monthly trends.
- **Startup & Early Management:** startup curve vs nameplate per asset in Early Management, exit-criteria checklist status (incl. operator certification), and punchlist burn-down.
- **Project reliability:** the project-side counterpart to an OEE board (`§14`): project-loss Pareto by category and package, package readiness for the next N weeks, weekly plan compliance with variance reasons, RFI / submittal / change-order aging, open deviations, and hold-point first-pass rate. Contributors see a version filtered to their own contract's packages.

Admins can create and share custom dashboards. Users with `write:dashboards` grant on a project can modify project-scope dashboards.

### Chart grammar

All charts share a visual system. Choices and defaults (see the `dataviz` skill for the engineering detail):

- **Palette**: a single curated 8-color categorical palette; one sequential and one diverging scale for ordered data. Light and dark themes. No chart invents colors.
- **Encoding rules**: time → x axis, value → y axis, category → color. Where ordering matters, use the sequential scale rather than categorical.
- **Chart types** per data shape:
  - Scalar KPI → labeled large number with trend arrow and sparkline.
  - Timeseries → line chart (preferred) or small multiples when multi-project.
  - Category series → ordered bar (horizontal when many categories).
  - Status grid → heatmap-style cells for cross-project rollups.
  - Gauge → only for bounded 0–100% metrics, never for budget variance.
  - Map → geo projects (phase 2).
- **Accessibility**: all encodings pair color with pattern/shape or label; contrast is ≥ WCAG AA. Tooltips expose precise values.
- **Interaction**: hover for details, click to drill (same tile → scoped detail; or new tile linked), range brush for timeseries, link to the governing audit record where applicable.

### Role filtering

- Every tile has `data_binding` that is evaluated through the caller's effective grants.
- A tile that would be entirely empty for a caller renders a "No data visible for your role" tombstone, so the user understands the structure exists but is restricted, rather than hiding the tile silently.
- Field-level restrictions (`§04`) cause specific columns to be omitted in tabular tiles.

### Freshness and updates

- Dashboards subscribe to `tile_updates` SSE channels on open. When pm-core receives a mutation that affects a bound tile, it invalidates the Redis cache and pushes a hint to the SSE channel. The frontend fetches the fresh tile.
- Freshness tolerance per tile (default 60s for live, 15 min for aggregated, 24h for compliance). Stale indicators appear in the tile corner when the tolerance is exceeded.
- Executive views (portfolio-wide) prefer the continuous aggregate layer in pm-TSDB (`§07`) so renders stay fast at ~500 projects.

### External stakeholder dashboards

- Tokenized links: short-lived (default 7 days, renewable), revocable, viewable only.
- Share creation (UC-06): owner picks a dashboard, defines audience, optional allowlist of identities, optional attestation requirement.
- On open: `pm-frontend-public` (`§05`) renders. No authenticated APIs are reachable. First-time viewers may be challenged for identity (name + email) and acceptance of terms.
- Every view is logged (`AuditEvent action_type=dashboard_external_view`), with viewer identity + IP + timestamp.
- Owner can see view counts and identities and revoke the link anytime.

## Field (mobile) flows

Some capital work happens at the equipment, not at a desk. These flows get a phone-first UI (Phase 2; responsive web until then), borrowing from the operator-facing TPM tools' pattern of simple checklists with photo or QR proof:

- **CQV / SAT walkdowns:** step through the test protocol at the asset; photo evidence per step; pass / fail / punchlist captured on the spot.
- **Punchlist capture and closeout:** photo, location (asset QR), severity, owner; closure requires an "after" photo.
- **Turnover inspections:** verify spares on the shelf, labels, LOTO points, and procedures posted, against the TurnoverPackage checklist.
- **Hold-point inspections:** inspector steps through the ITP criteria at the work face, attaches evidence, and releases or fails the hold point.
- **Deviation tagging:** anyone close to the work (owner's rep, supervisor, contractor lead) tags an interface clash, missing material, or incomplete prior work with a photo and location; it routes to an owner and can be escalated to an RFI, issue, or change request.
- **Contractor progress updates:** contributors record installed quantities and close assigned punchlist items, scoped to their own packages.
- **Design-review input at the equipment:** operators and maintenance techs comment on a model, drawing, or existing asset with photos, feeding the "easy-to" findings.

Asset QR codes resolve to the pro_manage asset page (role-filtered). Offline capture with later sync is required for plant areas without coverage; conflicts resolve last-write-wins per field with the full history kept in audit.

## Scheduled exports

### Scheduling

- Each dashboard can have `ExportSchedule` rows: `kind` ∈ `{pdf, pptx}`, `cadence` (cron or named, e.g. `weekly_fri_1700`), `recipients[]` (user ids, email addresses, Teams channel refs via Commsi), `template_id`, `filter_overrides`, `active`.
- pm-workers scheduler fires at the cadence; invokes pm-core to assemble bound data; dispatches to pm-sidecar render; stores artifact in MinIO; sends via Commsi.

### Templates

- Templates for PDF and PPTX are Git-tracked under `templates/exports/` in the pro_manage repo. Each template declares the data bindings it expects (same binding model as dashboard tiles).
- Deployed templates are hashed and cached in Postgres (`ExportTemplate` row); hash mismatch between source and deployed is a release-time check.
- Admin UI shows available templates with previews. Admins can activate/deactivate and set defaults per workflow.
- Custom templates can be added by uploading a `.pptx` or an HTML+CSS stencil for PDF; a UATS spec validates the data-binding manifest.

### Delivery guarantees

- Scheduled runs are idempotent per `(schedule_id, cycle_id)`. Failures retry with exponential backoff and surface in a dashboard-config diagnostics panel.
- Delivery via `Commsi` honors channel-specific limits (email size caps → store artifact in MinIO, send presigned link + inline summary).

## On-demand deck generation

### Flow

- Project Owner (or anyone with `read:dashboards` grant) hits "Build steering deck."
- Modal: choose template (defaults by workflow kind: "Monthly steering," "Quarterly review," "Closeout review"), choose included sections.
- pm-core assembles bound data (budget state, schedule state, risks, metrics, outstanding approvals, recent decisions, Jiminy insights relevant to the project).
- pm-sidecar renders PPTX.
- Owner reviews in a preview, can edit annotations/notes before send.
- Owner downloads or dispatches via Commsi.

### Deck templates ship with year one

- Monthly steering deck
- Quarterly executive review
- Change-request briefing
- Closeout review
- Audit prep brief

Each is Git-tracked with a demo render under `templates/exports/examples/`.

### "Live-data snapshot" semantics

- Every deck is stamped with the data as-of timestamp per section (sections may draw from caches of different freshnesses).
- The generated PPTX stores the stamp in each slide footer and in the file metadata.
- Future retrievals of the same deck produce a *new* file reflecting *then-current* data; the original is preserved in MinIO.

## Reporting beyond dashboards

- **Metric exports**: raw metric data as CSV / Parquet, scoped by project + time + metric set. Role-filtered.
- **Audit exports**: compliance-flavored exports of AuditEvent rows for a project / time / action-type window; integrity verifiable via the chained hash.
- **Approval chain exports**: PDFs reconstructing a complete workflow with every stage, decision, guidance snapshot, and signed envelope link.

All exports are logged with retrievable requests ("who exported what, when").

## Open items for Section 09

- Confirm the first batch of custom chart types desired by executives (e.g., S-curves for cash flow). First instinct: ship the base library in year one, admin-authorable S-curves and advanced charts in Phase 2.
- Decide whether live dashboards support anonymous (unauthenticated) tokenized viewing for truly external parties, or whether even tokenized viewers must self-attest identity. Current draft: self-attest (lightest audit still exists).
- Confirm PDF rendering engine for the sidecar (Chromium headless vs ReportLab/WeasyPrint). Chromium is heavier but handles any CSS; WeasyPrint is lighter but has rendering limits. First instinct: Chromium headless.
- Decide the default retention for generated export artifacts in MinIO (space-management). First instinct: 90 days for ad-hoc, aligned with retention regime for workflow-attached artifacts.
