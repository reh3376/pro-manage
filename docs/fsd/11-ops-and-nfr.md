# 11: Operations & Non-Functional Requirements

## Decisions on record

- **2026-10-13:** Hosting: on-prem VMs with containers inside (docker-compose per VM), designed for later migration to Kubernetes without reshaping services.
- **2026-10-13:** Availability target: **99.5% monthly** for the pro_manage user-facing surface; **99.9% monthly** for the AuditEvent write path (we don't lose audit even if dashboards degrade).
- **2026-10-13:** Backup/DR: nightly logical + continuous WAL replication for Postgres; nightly snapshots for TimescaleDB and MinIO; MDEMG backs itself up per its own guidance. RTO ≤ 4h, RPO ≤ 15 min for primary data.
- **2026-10-13:** Default data retention: **7 years** for audit, approvals, signed artifacts; **2 years full-resolution + 10 years rollups** for metrics; **90 days** for ad-hoc export artifacts; aligned with ISO + SOX-grade baseline. Project-level **legal hold** freezes deletion regardless.
- **2026-10-13:** Secrets live in a deployment vault; code reads them at startup. Rotation on a documented schedule plus incident-driven.
- **2026-10-14:** **No AI inference leaves the intranet.** MDEMG's language model runs on the MDEMG host (`llama-server` on loopback serving MDEMG's fine-tuned `mdemg-llm-v1`); no hosted-LLM provider is configured in any environment and the egress proxy allow-list carries no LLM vendor. Decision and hardware shapes in `§06`.

## Performance targets (SLOs)

Measured at the user's browser (end-to-end) or at the pm-core edge (internal) as noted.

| Surface | p50 | p95 | p99 |
|---|---|---|---|
| Project page first-paint (SSR) | ≤ 600 ms | ≤ 1.2 s | ≤ 2.0 s |
| Dashboard tile refresh (cached) | ≤ 50 ms | ≤ 150 ms | ≤ 300 ms |
| Dashboard tile refresh (cache miss, aggregated layer) | ≤ 200 ms | ≤ 600 ms | ≤ 1.2 s |
| Dashboard tile refresh (drill-down, project scope) | ≤ 400 ms | ≤ 1.0 s | ≤ 2.0 s |
| Approval submit (Tier B, no classify blocking) | ≤ 300 ms | ≤ 800 ms | ≤ 1.5 s |
| Approval submit (Tier A, pre-classified verdict cached) | ≤ 400 ms | ≤ 1.0 s | ≤ 1.5 s |
| Approval submit (Tier A, classify inline, both spaces in parallel) | ≤ 4 s | ≤ 8 s | ≤ `mdemg.classify.timeout_ms` |
| MDEMG `/guide` read of a warmed result (edge) | ≤ 300 ms | ≤ 800 ms | ≤ 1.5 s |
| MDEMG `/classify` single call (edge; one local LLM call) | ≤ 3.5 s | ≤ 7 s | ≤ 10 s |
| MDEMG LLM endpoint, one chat completion on `pm-mdemg-01` | ≤ 3 s | ≤ 6 s | ≤ 9 s |
| Metric ingest (per-reading, pm-core edge) | ≤ 20 ms | ≤ 60 ms | ≤ 150 ms |
| Metric harvest tick (full batch, moderate-size source) | n/a | ≤ 30 s | ≤ 90 s |
| Spreadsheet import validation (10k rows) | ≤ 3 s | ≤ 8 s | ≤ 15 s |
| On-demand deck generation (typical project, 15 slides) | ≤ 10 s | ≤ 20 s | ≤ 30 s |
| Scheduled export dispatch (per recipient) | n/a | ≤ 5 s | ≤ 15 s |

**Capacity assumptions behind targets:**
- 500 users, with ~100 concurrent active sessions during business hours.
- 200+ projects, ~50 active at any one time receiving write traffic.
- 10M metric readings/day across all projects (realistic for a mid-sized industrial footprint).
- Audit write rate: ~50k events/day on launch, growing with adoption.
- MDEMG LLM: 4 parallel inference slots (MDEMG's `--parallel 4`); at most a few concurrent Tier A classifications in business hours. Pre-classification spreads the load across the form-editing window instead of the submit instant. Guide warm-ups are queued, not dropped, when slots are busy.

**MDEMG LLM targets are hardware-bound.** The classify rows above assume MDEMG's reference hardware (Apple Silicon, M-series Max class) or an equivalent GPU-backed Linux host; they are confirmed by the Phase 0 load test on the chosen host and revised if that host is slower (`§06`, "Hardware").

**Headroom:** services sized to run at ≤ 50% CPU and ≤ 60% memory at p95 load. Alerts fire at 70%.

## Availability targets (SLAs)

| Component | Monthly target | Error budget |
|---|---|---|
| pro_manage user surface (frontend + core REST) | 99.5% | ~3.6h/month |
| AuditEvent write path (Postgres primary + hot standby) | 99.9% | ~43 min/month |
| MDEMG advisory path (including its local LLM endpoint) | 99.0% | ~7.2h/month (fail-closed tiers absorb outages per `§06`) |
| Metric harvest (per-connector) | 99.0% | ~7.2h/month per connector |
| Scheduled export delivery (within 24h of scheduled time) | 99.0% | ~7.2h/month |

Error budgets drive release-rate decisions: if the user surface burns > 50% of its budget in a week, pause non-critical releases.

## Backup, DR, RTO/RPO

- **Postgres**: nightly logical dumps + continuous WAL streaming to an off-box replica. RPO ≤ 15 min via WAL; RTO ≤ 4h for a full restore from logical + WAL replay.
- **TimescaleDB** (ours): nightly snapshots + daily continuous aggregates as compaction material. Metric data loss tolerable at ≤ 1h window if a disaster strikes mid-day. Backfill from source systems (via Metrici) can recover most lost windows.
- **MinIO**: multi-node erasure-coded; nightly snapshot copies to secondary tier (optional cold storage). RTO/RPO equivalent to Postgres for signed artifacts bound to AuditEvents.
- **MDEMG** (Neo4j + TSDB): follow MDEMG's own backup guidance. Loss of MDEMG does not threaten pro_manage's authoritative state; recovery restores guidance.
- **Redis**: ephemeral; sessions re-authenticate on recovery. Not backed up.
- **Vault**: operator-managed per the chosen vault tech; keys sealed/unsealed on restart.
- **DR drill**: quarterly, documented runbook, with a scoring template (actual vs target RTO/RPO).

## Observability (operational specifics)

- **Logs**: structured JSON to stdout, shipped to Loki or company log aggregator. Standard fields: `trace_id`, `request_id`, `session_id`, `user_id` (hashed where PII-sensitive), `project_id`, `audit_id`, `connector_instance_id`, MDEMG `request_id`.
- **Metrics**: Prometheus `/metrics` on every service. Dashboards:
  - **Service health:** RED (rate, errors, duration) per endpoint, per connector.
  - **Business health:** approvals in flight, approvals overdue, metric harvest lag, outbox depth, Jiminy guidance rate / block rate / override rate.
  - **Database health:** Postgres connections, replication lag, WAL volume; Timescale compression ratio and query latency; Redis evictions.
  - **MDEMG health:** advisory latency, classify error rate, pre-classification cache hit rate, outbox dispatch rate, failed-fire alerts, and the LLM endpoint state (`up`, `degraded`, `down`) and per-call latency scraped from MDEMG's watchdog and `llama-server` `/metrics`.
- **Traces**: OpenTelemetry; cross-service traces for approval flows, metric harvest, spreadsheet import.
- **Alerts:** routed via Commsi to on-call rotations:
  - P1 (page): user surface error rate > 1% for 5 min; AuditEvent write path down; Postgres primary unavailable; audit chain verification failure; Tier A classify failure rate > N/min.
  - P2 (ticket + Teams/Slack): any connector unhealthy > 30 min; MDEMG unavailable > 15 min; MDEMG LLM endpoint `down` > 5 min or `degraded` > 30 min; outbox depth > threshold; backup failure.
  - P3 (notice): schedule drift on exports; policy change deployed; expiring secrets within 30 days.

## Security posture

### Network
- All services behind the reverse proxy; only the proxy is reachable from the user network.
- MDEMG binds loopback-only inside its VM; pm-core reaches it via an authenticated internal channel.
- Connectors that need outbound access to vendor APIs have egress through a controlled proxy with per-destination allowlisting.
- **No LLM vendor is on the egress allow-list.** MDEMG's model runs on `pm-mdemg-01` and is reached only on that host's loopback. The only LLM-related outbound traffic is the one-time, SHA-verified download of model weights through the proxy (or a local mirror for an air-gapped site); see `§06`.
- **Inbound calls from cloud-hosted systems** (for example IFS Cloud event push to `ERPi_IFS`, DocuSign webhooks to `Sigi`) are the only traffic that originates outside the company network. They are disabled until the company's IT security team approves a path (DMZ reverse proxy, VPN, or private connection). When enabled, they terminate at the reverse proxy, are restricted to the sender's published address ranges, and require authentication on every request (OAuth or signed payloads); each such endpoint accepts only its own sender's messages. Connectors fall back to polling when no inbound path exists.
- All in-cluster traffic uses TLS; mTLS between internal services where feasible (gRPC default).

### Authentication / authorization
- SSO for every user action (`§04`). Local break-glass admin logged conspicuously.
- Session tokens stored in HttpOnly SameSite=Strict cookies; CSRF tokens on state-changing REST calls.
- Service-to-service auth: short-lived JWTs minted by pm-core for connectors; mutual auth where supported.
- MDEMG shared bearer rotated via overlap; vault-backed.

### Hardening
- CSP strict on the frontend; nonce-based scripts.
- Dependency scanning (Go, Python, npm) on every CI run.
- Container images rebuilt weekly against security advisories; latest-patched base images.
- Infrastructure-as-code (compose files, environment specs) under version control; drift detection on target VMs.
- Secrets never in images; vault-fetched at startup; not logged.

### Audit
- Access and change audit per `§04` + `§08`.
- Admin actions (config edits, user provisioning, override events) emit conspicuous audit rows + alerts.
- Audit chain nightly verification; mismatches alert P1.

### Multi-site operation
- **Time:** every timestamp is stored in UTC and displayed in the viewer's or the site's time zone as the screen declares; SLA clocks and "business day" arithmetic run on the project's site calendar (working days, holidays); scheduled exports fire in the recipient's time zone.
- **Money:** every money value carries its currency; the project's currency defaults to the site's. Cross-currency roll-ups use the dated `fx.rate.*` policy table and record the rate used; thresholds compare in `{car.threshold.currency}` at the rate in force on submission (`§03`, `§08`).
- **Units:** site `unit_system` drives display and workbook templates; metric definitions declare their unit and conversions are explicit, never implied.
- **Regulatory stamps and sign-offs** default from the site's `regulatory_regime[]` onto new projects (for example a Canadian site adds `cfia` and `excise_ca` where a US site adds `ttb`); the approval chains that name site roles resolve against that site's pools.
- **One deployment, many sites:** the application is hosted once on the company's network; users at every site reach it over the company WAN. Cross-border access to a single on-premises system is a legal question (below), not an architectural one.

### Privacy
- PII minimized. User identifiers in logs are hashed where feasible.
- **Per-site privacy regimes.** A site's `privacy_regime[]` (for example PIPEDA for a Canadian site, state privacy law elsewhere) determines what personal data the site's users and contributors may be asked for, the retention of their session and audit identifiers, and the notice shown at sign-in. Contractor and supplier contributors' personal data is limited to what federation or guest provisioning requires. Where a regime restricts cross-border processing, Legal confirms before that site goes live; the open item below carries it.
- External document content is not stored in MDEMG (we send readable summaries, not bodies).
- Those summaries are processed by a model running on the MDEMG host. No prompt, observation, or guidance is sent to a third-party AI service, in any environment.
- Document contents are stored in MinIO under a tenant-scoped keyspace; signed URLs are short-lived and audience-bound.

### Patching
- OS and runtime patches on a monthly cadence, out-of-band for critical CVEs.
- Database and language-runtime upgrades tested in staging first; rollback plan documented.

## Capacity and scaling

- Horizontal scaling for pm-core, pm-sidecar, pm-workers, pm-frontend, pm-frontend-public, and each connector.
- Vertical scaling for Postgres primary (within reason); hot standby for failover.
- Timescale as a single-node footprint with HA pair; shard later if needed.
- Redis as a Sentinel pair.
- MinIO scaling: 4+ nodes with erasure coding.
- MDEMG: follows its own capacity guidance. Its host is sized for the local model: 32 GB memory minimum (64 GB if `mdemg-llm-v2` is planned) on an Apple Silicon host, or a Linux VM with a 24 GB-class GPU passed through (`§06`, "Hardware"). The LLM does not scale horizontally in MVP; if classify queueing appears at 1.5× peak, the remedy is a larger host or a second `llama-server` behind MDEMG, planned as a Phase 4 item.
- **Scaling drill**: before year-two load appears, run a controlled load test at 1.5× expected peak to confirm headroom; publish a capacity-plan addendum.

## Release strategy

- **Branching**: trunk-based with short-lived feature branches merged via PR.
- **CI**: the merge-blocking UxTS set (UATS, UDTS, UNTS; UOTS and USTS from Phase 1), adjudication-registry verifiers, Go/Python/TS unit tests, Next.js build, and a compose spin-up smoke test on every PR. UBTS and UOBS run soft-fail on PR and in full nightly against staging. No `continue-on-error` on any test step (`§12`).
- **Observability artifacts are UOTS-pinned**: every dashboard JSON, alert rule file, and metric registry entry in this section has a spec; removing a required panel or renaming a metric fails the PR.
- **Staging**: all merged commits auto-deploy to staging. Nightly end-to-end tests against staging.
- **Production**: tagged releases; rolling deploy with health gating; feature flags for risky changes; MDEMG wire-format changes land behind an adapter version switch.
- **Rollback**: image tags are immutable; rollback is `docker-compose up` with the prior tag. DB migrations are forward-compatible and reversible for one release cycle.
- **Change windows**: production releases in business-hours-adjacent windows (early-morning or late-afternoon) with on-call coverage.

## Environments

- **Local dev** (developer workstation): docker-compose; seed data; mocked connectors by default; MDEMG dev profile.
- **CI**: ephemeral compose stacks; synthetic data; mocked external backends.
- **Staging**: full topology at reduced scale; sandbox credentials for IdPi, Sigi, ERPi.
- **Production**: full topology at site-wide scale.

## Data retention and legal hold

- **Audit, approvals, signed artifacts**: 7 years (SOX/ISO baseline).
- **AuditEvent chain-verification logs**: 10 years (long enough to prove the chain across prior audits).
- **Metric readings**: 2 years full-resolution; 10 years daily/weekly/monthly rollups.
- **Export artifacts**: 90 days ad-hoc; aligned with retention regime of the workflow they're attached to.
- **Session logs**: 180 days.
- **Legal hold** (per-project toggle, platform_admin-only): all deletion suspended, including downsampling. Hold is logged and surfaced in dashboards and admin UI.

## Compliance alignment

- **ISO 9001 (quality)**: AuditEvent + GuidanceOutcome + approval chain satisfy document-and-record control.
- **ISO 55000 (asset mgmt)** (if applicable): metric capture + lifecycle state model + closeout gates align.
- **ISO 27001 (infosec)** (if adopted): access control (`§04`), audit (`§08`), ops hardening (this section), and incident response procedures together address major control families.
- **SOX-equivalent**: AuditEvent integrity (chained hash) + approval chain preservation + separation of duties (owner cannot be their own approver on Tier A workflows).
- **Privacy law per site** (PIPEDA, applicable US state privacy law, and any other regime a site declares): PII minimization, per-site notice and retention, and Legal's confirmation of cross-border processing before a foreign site goes live.

Formal certification is out of scope for the FSD. This is implementation alignment, not a certification plan.

## Open items for Section 11

- Confirm which specific ISO standards the deployment must align with (owner's intel pending).
- Confirm with Legal, per site outside the home country, whether hosting that site's project and contributor data on the home-country deployment needs a data-transfer agreement or local notice (PIPEDA for a Canadian site is the first case), and which privacy regime values to seed on the Site.
- Confirm the company's reporting currency for `{car.threshold.currency}` and who maintains the `fx.rate.*` table (first instinct: Finance controls, monthly, matching the ERP's rates).
- Decide retention for Jiminy guidance snapshots that are *not* tied to a governed-action audit record (currently proposed 180 days in `§06`).
- Confirm company's existing log aggregator (Loki, Splunk, Elastic) and metric platform (Prometheus, Datadog); we may reuse existing company tooling rather than ship from scratch.
- Decide on-call rotation shape (which personas carry pager, business-hours only vs 24/7). Early-adoption bias: business-hours on-call with alerted ticketing after hours; full 24/7 after year one if adoption drives it.
- Confirm cold-storage tier for backups (second site, cloud, tape).
- Confirm with IT which `pm-mdemg-01` host shape they will operate (Apple Silicon host or GPU-backed Linux VM) and whether the server network can hold it; this gates the Phase 0 load test and the classify SLO rows above.
