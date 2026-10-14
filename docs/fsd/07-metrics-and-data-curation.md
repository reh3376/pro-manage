# 07: Metrics & Data Curation

## Decisions on record

- **2026-10-13:** First-class metric families: **Financial, Schedule/Milestones, Scope/Risk/Issues, Safety/Quality/Compliance, Resource utilization, and TPM / Plant Operations** (post-startup).
- **2026-10-13:** **TPM / Plant Operations** is the metric family that proves a capital project actually delivered. During Early Management Period and Benefits Realization, pro_manage pulls OEE, MTBF, MTTR, FPY/RFT, EPU, OLE, Startup Curve, Changeover Time, and Capacity Utilization via **Metrici** (plant historian / OPC UA) and **HSEi** (MES/QMS) and compares against the project's business-case promises.
- **2026-10-14:** **Priced losses** (Six Big Losses + site loss tree) are a first-class metric family harvested from the operations TPM stack. Business cases promise loss recovery in dollars; Benefits Realization verifies recovery of those losses on those assets against a CAR-time baseline.
- **2026-10-14:** **Project losses** (the project's own losses under an EPCM model) are a first-class family alongside plant losses, with project-administration cycle times, package readiness, and plan compliance. A composite "package effectiveness" score is deliberately *not* a headline metric: composites are easy to game; the component losses are what health reviews act on.
- **2026-10-13:** **TIC** (Total Installed Cost) and **LCC** (Lifecycle Cost) are separate first-class concepts. TIC is tracked throughout the project; LCC is estimated during Design and compared to actuals during Benefits Realization.
- **2026-10-13:** Harvest architecture: pluggable `Metrici` connectors (relational, time-series, generic Historian / OPC UA) with per-backend throttles and schema contracts governed by UATS/UNTS.
- **2026-10-13:** Metric readings land in **pro_manage's own TimescaleDB** (not MDEMG's). ERP actuals come via `ERPi` and settle into both the financial domain tables (Postgres) and the metric time series (ours).
- **2026-10-13:** Spreadsheet import/export is first-class, with UATS-governed schema contracts for every supported workbook template.
- **2026-10-13:** Derived metrics (CPI, SPI, EAC, variance) are computed in **pm-sidecar** (Python) via a sandboxed formula engine with an allowlist of operations.
- **2026-10-13:** Data quality is modeled explicitly: every reading carries a `quality ∈ {good, questionable, bad}` with a `source_ref`. "Bad" readings are stored but excluded from aggregations by default.

## Metric families (year-one catalog)

Each family ships with a built-in set of standard metrics (admin-extensible via `MetricDefinition`). Baseline list:

### Financial
*In the ERP-led profile, EAC and earned-value measures (CPI, SPI, BCWS/BCWP/ACWP) are read from the ERP rather than derived in pm-sidecar, so pro_manage and the ERP never show different numbers for the same project.*

- `budget.baseline_total` (per baseline revision)
- `budget.committed` (POs issued, from ERPi commitments)
- `budget.actual` (invoiced + paid, from ERPi actuals)
- `budget.forecast_eac` (point-in-time Estimate at Completion)
- `budget.variance_pct` = `(committed + remaining_eac − baseline_total) / baseline_total`
- `budget.cash_flow_actual` (period-based)
- `budget.cost_performance_index` (CPI = BCWP / ACWP): derived; requires earned-value inputs
- `budget.schedule_performance_index` (SPI = BCWP / BCWS): derived

### Schedule
- `schedule.milestones_total`, `milestones_completed`, `milestones_overdue`
- `schedule.critical_path_slip_days` (if the schedule import includes CP info)
- `schedule.baseline_variance_days` (per milestone)
- `schedule.aged_milestones` (count open > N days)
- `schedule.on_time_delivery_rate` (closed on-time / closed total)

### Scope / Risk / Issues
- `scope.completion_pct` (weighted by WBS)
- `scope.deliverables_accepted` / `scope.deliverables_outstanding`
- `scope.change_requests_open`, `scope.change_requests_impact_sum`
- `risk.register_count`, `risk.high_or_critical_count`, `risk.aged_without_update`
- `issue.open_count`, `issue.aged_without_update`

### Safety / Quality / Compliance
- `safety.incidents_mtd` (month-to-date)
- `safety.near_miss_count`
- `quality.inspection_pass_rate`
- `quality.nonconformance_open`
- `compliance.signoffs_due`, `compliance.signoffs_overdue`
- `hse.open_findings_aged` (per aging bucket)

### Resource utilization
- `resource.named_allocation_pct` (person-level, concurrent sum)
- `resource.overallocation_count` (people above 1.0)
- `resource.equipment_uptime_pct` (from Historian / OPC)
- `resource.contractor_headcount`

### TPM / Plant Operations (post-startup)

This family only becomes meaningful once an asset is in Early Management or beyond. It is the direct scorecard for **vertical startup** and **benefits realization**. All of it harvests from the running plant via **Metrici** (historian / OPC UA / SCADA) and **HSEi** (MES / QMS).

- `tpm.oee`: Overall Equipment Effectiveness. **Availability × Performance × Quality.**
  - `tpm.oee.availability`: Run time ÷ planned production time.
  - `tpm.oee.performance`: (Ideal cycle time × total count) ÷ run time.
  - `tpm.oee.quality`: Good units first-time ÷ total units.
- `tpm.teep`: Total Effective Equipment Performance. OEE × Loading. Reveals hidden capacity.
- `tpm.capacity_utilization`: Actual output ÷ installed capacity.
- `tpm.mtbf`: Mean Time Between Failures (operating time ÷ failure count).
- `tpm.mttr`: Mean Time To Repair (repair downtime ÷ repair count).
- `tpm.mttf`: Mean Time To Failure (non-repairable components).
- `tpm.fpy_rft`: First Pass Yield / Right First Time.
- `tpm.startup_curve`: Daily rate as % of nameplate, from Day 1 of production. Primary EEM scorecard.
- `tpm.time_to_nameplate`: Days from first production to sustained target rate (point metric, computed at achievement).
- `tpm.planned_maintenance_pct`: Planned maintenance hours ÷ total maintenance hours.
- `tpm.changeover_time`: Per changeover.
- `tpm.cpu`: Cost Per Unit (production cost ÷ units).
- `tpm.copq`: Cost of Poor Quality (scrap + rework + reblend + returns + inspection + downgrades).
- `tpm.epu`: Energy Per Unit (energy used ÷ output; e.g., therms per proof gallon for a distillery).
- `tpm.ole`: Overall Labor Effectiveness.
- `tpm.cycle_time`: Actual vs engineering-standard cycle time.
- `tpm.schedule_adherence`: Actual output ÷ scheduled output.

**Industry-specific metrics are template-driven.** For distilling: `distillery.yield_pg_per_bushel`, `distillery.proof_gallon_per_day`, `distillery.beer_system_water_use_gpd`. For bottling: `bottling.line_speed_bpm`, `bottling.changeover_time_sku_to_sku`. Templates ship with the pm-core release; new industries add their own via the admin `MetricDefinition` surface.

**Business-case linking.** Every TPM metric used in a project's business case is captured as a `BusinessCasePromise` (`§03`), with baseline, promised value, measurement basis, and dollar value. During the realization window, actuals are measured on the promised assets and the delta is recorded.

### Losses (priced)

Losses are the common currency between operations and capital. pro_manage does not originate loss data; it harvests `LossEvent`s from the plant's MES / OEE / loss platforms via HSEi (and Metrici where losses are derived from historian signals), categorized by the **Six Big Losses** and the site loss tree (`§03 LossCategory`), and priced in dollars per the category's cost basis.

- `loss.priced_annualized`: per asset and loss category, trailing 12 months, dollars. The baseline for any `loss_recovery` business-case promise.
- `loss.by_six_big`: breakdowns, setup/changeover, minor stops, reduced speed, startup rejects, production rejects.
- `loss.recoverable_candidates`: top losses by dollar value on assets with no open project addressing them. Feeds portfolio ranking (`§09`) and FEL1 idea generation.
- `loss.recovered`: for a closed project, priced loss during the Benefits Realization window vs the CAR-time baseline on the same assets and categories. This is **verified recovery**: the promise and the measurement use the same assets, categories, and pricing basis, fixed at CAR time.

Pricing basis (cost per downtime hour, per scrapped unit, etc.) is owned by Finance and versioned; a pricing change does not retroactively alter a committed promise's basis.

Admin can add `MetricDefinition`s per-project or per-portfolio for domain-specific KPIs.

## MetricDefinition vs MetricReading

- `MetricDefinition` is **what a metric means**. Lives in Postgres. Fields recap: `metric_id`, `name`, `category`, `unit`, `aggregation` (`sum|avg|last|…`), `scope` (`project|portfolio|global`), `source` (`derived|erpi|metrici|manual`), `formula?`, `connector_mapping_id?`, `quality_rules?`, `retention_policy`, `active`.
- `MetricReading` is **a measured value at a time**. Lives in pm-TimescaleDB. Fields: `metric_id`, `project_id`, `ts`, `value`, `quality`, `source_ref`, optional `tags` (JSON).
- A metric can be read-sourced (connector pulls raw readings) **or** derived (sidecar computes from other readings on a schedule or on-demand). Hybrid metrics (ingested raw + corrected via a formula) are modeled as two definitions linked by `derived_from`.

## Metric harvest architecture

```
                              ┌───────────────────┐
                              │  Metric Harvester │
                              │   (pm-workers)    │
                              └─────────┬─────────┘
                                        │ gRPC
       ┌────────────────────────────────┼─────────────────────────────┐
       │                                │                             │
       ▼                                ▼                             ▼
┌──────────────┐              ┌──────────────────┐           ┌─────────────────┐
│ Metrici-     │              │ Metrici-timescale│           │ Metrici-pi      │
│ postgres     │              │  (or influx)     │           │ (OSIsoft PI)    │
│ (reads source│              │  (reads TSDB)    │           │                 │
│  DB)         │              │                  │           │                 │
└──────┬───────┘              └────────┬─────────┘           └────────┬────────┘
       │                               │                              │
       ▼                               ▼                              ▼
   External OLTP               External TSDB                   Plant historian
   (read-only user)            (read-only user)                (OSIsoft API)
```

**Harvest modes:**
- **Poll**: default. Scheduler fires at the configured cadence per MetricDefinition; connector reads the batch since `last_ts_pulled`, streams back.
- **Subscribe**: for connectors that support live push (OPC UA subscriptions, some historian APIs), pm-workers maintains a persistent stream.
- **Event-driven**: webhook-style when the upstream can call us (e.g., ERPi fires on a new invoice posted).

**Backfill:** admins can trigger a backfill for a MetricDefinition over a time window. pm-workers chunks the window, calls the connector per chunk, writes to pm-TSDB with `ingest_source=backfill`. Chunking budget prevents overwhelm on large sources.

**Rate-limiting & circuit breaking:** per-connector-instance; configurable; health status reflects breaker state.

## Derived metrics (pm-sidecar)

The sidecar runs a sandboxed formula engine for derived metrics. Formulas are declarative:

```yaml
# Formula definition (admin-UI authored; stored in MetricDefinition.formula)
inputs:
  bcwp: metric_ref("budget.earned_value", project)
  acwp: metric_ref("budget.actual", project)
expression: "safe_div(bcwp.last, acwp.last)"
output_unit: "ratio"
quality:
  bad_if: "bcwp.quality == 'bad' or acwp.quality == 'bad'"
```

- Allowed operations: arithmetic, time-window aggregations (`sum_window(ref, '7d')`, `avg_window`, `last_before`), safe division, boolean, min/max.
- Disallowed: file I/O, network, subprocess, imports beyond the allowed math library.
- Formulas are versioned (`formula_version`); a changed formula produces a new MetricDefinition version so historical readings remain reproducible.
- Evaluation cadence: on schedule (per `MetricDefinition.aggregation.cadence`) or on-demand (dashboard refresh triggers a push-compute if the reading is stale beyond its freshness tolerance).

### Project losses (delivery)

TPM applied to the project itself: the project is the "equipment," and its losses are measured and attacked the same way. Concept, OEE mapping, and open questions in `§14`. Sourced from pro_manage's own EPCM objects (`§03`), so no connector is needed.

- `project_loss.by_category`: days and dollars by `ProjectLossCategory` (waiting on information, waiting on approval, waiting on material, rework, interface clash, unready package, safety stop, skill-gap error), per project and package, trailing period. The Pareto a health review uses to pick its dominant loss.
- `doc.ifc_on_time_pct`, `doc.revision_churn`: register entries issued IFC by their planned date; revisions per document after IFC (design stability, and a leading indicator of rework).
- `doc.packages_held_by_documents`: construction packages due to start in the next N weeks with a required document not yet IFC.
- `procurement.items_negative_float`, `procurement.min_float_days`: long-lead items forecast to land after the site needs them; the thinnest float on the project.
- `contingency.coverage`: contingency remaining ÷ expected value of open quantified risks (`§03`).
- `project_loss.cost_of_delay`: delay days × the project's configured cost-of-delay rate (where Finance has set one).
- `admin.rfi_cycle_time`, `admin.rfi_open_aged`: median raise-to-answer days; count open past SLA.
- `admin.submittal_cycle_time`, `admin.submittal_first_pass_rate`: review days; share approved on first submission (a project-side first-pass yield).
- `admin.change_order_cycle_time`, `admin.approval_wait_days`: including internal CAR / baseline approvals.
- `package.readiness_pct`: share of packages due to start in the next N weeks with all readiness constraints met.
- `package.plan_compliance`: percent plan complete: packages completed as planned ÷ packages planned, weekly, with variance reasons.
- `quality.ncr_count`, `quality.rework_cost`, `quality.hold_point_first_pass_rate`.
- `hse.leading_indicators` per package: near misses reported, job hazard analyses complete before start, permits-to-work compliance.
- `team.competency_gap_count` (Phase 3): required competencies unmet on assigned package teams.

## Rollups

Rollup scope is defined per MetricDefinition:

- **Project scope** metrics live per `project_id`.
- **Portfolio scope** metrics are computed from project-scope metrics via a declared aggregation (`sum | avg | weighted_avg(weight=…)`).
- **Global** metrics span portfolios (company-wide).

Rollups are refreshed on a cadence (default: every 5 minutes for executive dashboards; configurable per MetricDefinition).

Rolled-up readings are themselves stored in pm-TSDB with `scope=portfolio` and `project_id=NULL`.

## Baselines & variance

For metrics that have a plan vs actual story (budget, schedule, TPM targets):

- A `MetricBaseline` row captures the planned value for the metric at a point in time, tied to a project baseline (Budget baseline, Schedule baseline, or BusinessCasePromise from §03).
- At read time, pm-core joins current readings against the current baseline to compute variance.
- Baseline changes themselves fire a `Budget baseline change` workflow (§08); the old baseline is retained for historical variance queries.
- **AACE Class** is carried alongside each `BudgetBaseline` (see §03). Variance dashboards surface the AACE Class so stakeholders see the estimate maturity behind the number ("Class 5 = ±100% expected band, so a 40% swing is in-band").

## TIC vs LCC

The two financial dimensions pro_manage treats distinctly:

- **TIC (Total Installed Cost):** all capital to put the asset in service. Lives in `BudgetBaseline.baseline_total`. Progresses from AACE Class 5 at concept to Class 1 at execution. Primary variance tracking during Execute stage.
- **LCC (Lifecycle Cost):** forecasted total cost of ownership over the asset's expected life. Captured in `LCCEstimate` entities (§03). Estimated at Design stage as part of the business case; validated against operational cost actuals during Benefits Realization and after.

Dashboards present TIC and LCC together on projects where both exist. The Business Case view carries explicit prompts: "LCC estimated? Business case rejection likely without it on >$5M projects."

## Data quality

- `quality ∈ {good, questionable, bad}` on every reading.
- Connectors flag `questionable` when the upstream value crosses a reasonableness bound (per MetricDefinition `quality_rules`): e.g., a safety-incident count that drops in a reporting period.
- `bad` is set when the connector detects a schema mismatch or a null value where none is expected. Bad readings are stored (don't silently drop) but excluded from aggregations and dashboards by default; admins can view "all including bad."
- Late-arriving data (reading `ts` earlier than existing readings): accepted, with `ingested_at > ts`. Dashboards may re-render affected windows.
- Out-of-order within a batch: accepted. pm-TSDB sorts by `ts` on query.
- **Metric schema drift** (upstream changes shape, e.g., renaming a column): UNTS hash integrity on the `connector_mapping` fires in CI/CD and alerts operators. Readings pulled with a mismatched schema are tagged `bad` and the connector enters a "needs mapping update" state.

## Spreadsheet import / export

### Imports

Supported inputs: `xlsx`, `csv`. Each import template is governed by a **UATS spec** that defines the expected sheet structure, column headers, data types, and reasonable-value bounds.

Supported in year one (admin can add more):
- **Budget baseline workbook:** WBS rows × account columns, with totals, revision notes, approver (shadow row for pm-core to resolve against users).
- **Budget forecast workbook:** same shape as baseline + EAC column.
- **Milestone list:** ordered milestones with baseline and target dates, dependencies.
- **Risk register:** risks with likelihood, impact, response, owner.
- **Manual metric table:** generic (metric_id, ts, value, quality) rows for metrics that have no connector.
- **Resource plan:** allocations for named resources over time windows.

Flow (UC-10): user uploads → pm-sidecar validates → pm-core applies on accept. Validation surfaces per-cell errors to the UI.

### Exports

For every import type above, pm-core can export the current state as the matching workbook template, allowing round-trips (export, edit in Excel, re-import). The exports include a hidden metadata sheet with the pm-core project id and the current revision hash, so pm-core can detect stale re-imports.

### Beyond templates

Admins can author custom export templates via a simple schema (one worksheet per report; column mapping to metric_ids / entity fields). pm-sidecar renders. UATS spec is required.

## Dashboards query layer

Dashboards (`§09`) don't query pm-TSDB directly; they go through a pm-core query API:

- **`DashboardService.ResolveTile(tile_id, context)`** returns the data for a dashboard tile, scoped to the caller's effective grants and the dashboard's bound projects.
- Each tile has a declared data shape: `scalar`, `timeseries`, `category_series`, `tabular`, `kpi_grid`. The visualization library in the frontend (`§09`) matches shape to chart.
- Query results are cached in Redis with short TTLs (default 60s for live dashboards; longer for exported snapshots).
- Live dashboards subscribe via SSE to `tile_updates` channels; cache invalidation on write cascades to the SSE channel so dashboards re-render.

Query performance:
- pm-TSDB continuous aggregates maintain pre-computed per-day/per-week/per-month rollups so executive queries scan narrow partitions.
- The `MetricDefinition.scope=portfolio/global` rollup rows are first-choice; project-level ad-hoc queries only happen when drill-down is requested.

## Retention & downsampling

- **Raw readings:** retained per MetricDefinition `retention_policy` (default 2 years at full resolution).
- **Continuous aggregates:** per-day, per-week, per-month roll-ups kept at longer retention (7 years day, 10 years month), enough for ISO/SOX historical lookback.
- **Dropped data:** archived to object storage (MinIO) as parquet before being removed from pm-TSDB. Admin-triggered re-ingestion is possible.
- **Legal hold:** a project under legal hold has retention frozen; no deletion or downsampling occurs until the hold is released.

## Baseline captures for governance

When a workflow approves a baseline (budget or schedule), pm-core:
1. Writes the new `BudgetBaseline` / `ScheduleBaseline` to Postgres.
2. Writes a `metric_baseline` snapshot row to pm-TSDB (so historic variance queries can resolve the baseline as of any date).
3. Writes a `decision` observation to MDEMG via the outbox: `{obs_type:"decision", content:"Budget baseline raised 42.0M → 48.5M on project P-X", structured_data:{...diffs...}}`.
4. Writes an AuditEvent + GuidanceOutcome bound to the originating approval workflow.

## Open items for Section 07

- Confirm the specific chart of accounts shape expected from ERPi (sized once the first ERP backend is known).
- Decide whether safety / HSE metrics are also captured in pm-TSDB or linked by reference into the HSE system (open: do we duplicate for availability, or always live-fetch?). First instinct: duplicate with daily cadence to pm-TSDB for dashboard performance; live-fetch for drill-down.
- Decide the earned-value adoption bar: EVM is powerful but not every $25K project will have BCWS/BCWP inputs. First instinct: optional per-project, gated by a project-settings toggle; derived metrics skip when inputs absent.
- Confirm whether we need user-authored custom charts beyond the built-in tiles. First instinct: yes but in Phase 2; year one ships curated tiles.
- Confirm acceptable retention bounds against ISO standards once the specific standards are confirmed.
