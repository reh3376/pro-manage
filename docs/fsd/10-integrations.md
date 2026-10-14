# 10 — Integrations Catalog

## Decisions on record

- **2026-10-13** — All external integrations are **modular** (`§01`, `§05`). Each is a separately-deployed container implementing a stable per-connector-type gRPC interface, admin-registered via the `connector_instances` registry.
- **2026-10-13** — Every connector type ships with a **UATS spec** covering its gRPC + HTTP surface. Every connector instance presents a **standard `HealthCheck` + `Capabilities`** RPC so pm-core can route around unhealthy or capability-mismatched instances.
- **2026-10-13** — Secrets per connector instance live in the deployment vault; the `config_ref` in `connector_instances` points to the vault entry.
- **2026-10-13** — Connectors never bypass pro_manage's authorization. All project/data access from pm-core to a connector is already pre-authorized at pm-core's edge.

This section is a catalog. The architecture, framework, and gRPC contract templates are in `§05`. The admin UX for routing and health is in `§05` and `§11`. This section records the specifics per module.

---

## ERPi — ERP Integration

### Purpose
Pull financial actuals, commitments, invoices, chart of accounts, and vendors from the company ERP. Read-only. Writes stay in the ERP.

### gRPC interface (summary; see `pm.connector.v1.erpi.proto`)
- `HealthCheck`, `Capabilities`
- `FetchActuals(FetchActualsRequest) → stream Actual`
- `FetchCommitments(FetchCommitmentsRequest) → stream Commitment`
- `FetchInvoices(FetchInvoicesRequest) → stream Invoice`
- `FetchChartOfAccounts(FetchCoARequest) → stream Account`
- `FetchVendors(FetchVendorsRequest) → stream Vendor`

### Year-one backends
To be prioritized once the owner confirms the first production ERP (pending). Candidate backends:
- `erpi-sap` (SAP ECC via OData/BAPI; S/4HANA via CDS views)
- `erpi-oracle` (EBS tables or Fusion REST APIs)
- `erpi-netsuite` (SuiteTalk REST)
- `erpi-dynamics` (Dynamics 365 F&O REST)
- `erpi-odoo` (Odoo XML-RPC / JSON-RPC)

### Config knobs (per instance)
- Base URL / tenant id
- Auth: user+password, OAuth, service account, cert-based
- Chart-of-accounts filter (which accounts to include)
- Project-code mapping (how pm-core's `project.code` maps to the ERP's project / WBS / cost-center id)
- Contract mapping (which ERP purchase orders / commitments correspond to which pro_manage `Contract`; contract value and invoiced-to-date flow from here)
- Pull cadence per feed (actuals daily, commitments hourly, invoices weekly — defaults; backend overrides)
- Reasonable-value bounds for data quality

### Operational
- Rate-limited per vendor's API limits (SAP has strict concurrent-session limits; Oracle has per-user hourly quotas).
- Circuit-breaker on upstream errors.
- Backfill via `FetchActualsRequest.from_ts` works for all backends; ranges > 90 days are chunked.
- UNTS hashes the field-mapping YAML; any schema drift upstream is caught in CI.

### Open items
- Confirm first production backend (pending owner confirmation).
- Confirm whether multiple ERP instances will run concurrently (e.g., SAP for industrial division, NetSuite for services). Design supports this.

---

## IdPi — Identity Integration (SSO)

### Purpose
Front SSO for pro_manage. OIDC + SAML abstractions. Resolve IdP claims into pro_manage platform roles.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `AuthenticateInteractive(AuthRequest) → AuthResponse` (returns redirect to IdP)
- `ValidateToken(TokenRequest) → TokenValidation`
- `RefreshToken(RefreshRequest) → RefreshResponse`
- `ResolveClaimsToRoles(ClaimsRequest) → RolesResponse`
- `Logout(LogoutRequest) → LogoutResponse`

### Year-one backends
- `idpi-oidc` (generic OIDC — Entra ID, Okta, Keycloak, Google Workspace, etc. via the generic flow)
- `idpi-saml` (generic SAML 2.0 — ADFS, Shibboleth, PingFederate, etc.)
- Backend convenience wrappers (optional) for Okta, Entra ID, ADFS to simplify admin config.

### Config knobs (per instance)
- Issuer / metadata URL
- Client credentials (public client with PKCE preferred for interactive)
- Scopes / claims requested
- JWKS cache TTL
- Claims-to-roles mapping file (per `§04`)
- Session lifetime defaults (overridable globally)

### Operational
- JWKS keys are cached and refreshed on `kid` miss.
- SAML certificate rotation: operator uploads new metadata; pm-core holds both old and new for a grace window.
- Failed logins and anomalous flows (missing claim, bad signature) are alertable.

### Open items
- Confirm the company's primary IdP (owner's intel pending; design is backend-agnostic regardless).
- Decide whether SCIM provisioning is in year one for user lifecycle (first instinct: JIT only, SCIM in Phase 2).

---

## Sigi — E-signature Integration

### Purpose
Formal e-signature on CAR approvals, change requests, closeout, and other configured workflow kinds. Envelope lifecycle, status polling, webhook ingestion, signed-artifact capture.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `CreateEnvelope(EnvelopeSpec) → Envelope`
- `GetEnvelopeStatus(EnvelopeRef) → EnvelopeStatus`
- `CancelEnvelope(EnvelopeRef) → CancelResponse`
- `DownloadSignedArtifact(EnvelopeRef) → stream FileChunk`
- `OnWebhook(Webhook) → WebhookAck` (backend → connector → pm-core)

### Year-one backends
- `sigi-docusign` (DocuSign eSignature REST)
- `sigi-adobesign` (Adobe Acrobat Sign)
- `sigi-onespan` (OneSpan Sign)

### Config knobs
- Account id / integration key
- Signer verification requirements (ID verification add-on)
- Envelope templates (map of workflow kind → provider template id)
- Expiration defaults
- Webhook signing secret

### Operational
- Webhooks must be delivered to a pm-core endpoint reachable from the provider (via reverse-proxy, with signature verification).
- Lost webhooks → periodic reconciliation via `GetEnvelopeStatus` for in-flight envelopes.
- Downloaded signed PDFs are hashed; the SHA-256 is captured in `AuditEvent` so later re-downloads can be verified.

### Open items
- Confirm DocuSign tenant model (one tenant for the whole company, or per-BU).
- Decide templates mapping for CARs vs change requests vs closeout.

---

## Schedulei — Schedule Tool Integration

### Purpose
Import and (optionally) export project schedules from external tools. Preserves the original file as attachment; maps to pm-core's `Milestone` + dependency model.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `ImportSchedule(ImportRequest) → ImportResult` (one-shot or streamed per project)
- `ExportSchedule(ExportRequest) → stream FileChunk`
- `ListProjects(ListRequest) → stream RemoteProjectRef` (for backends with a project API)
- `Subscribe(SubscribeRequest) → stream ScheduleEvent` (where supported)

### Year-one backends
- `schedulei-msproject` (MPP via XML/MPX export; .mpp read-only requires an unofficial lib — likely MVP uses XML export)
- `schedulei-p6` (Primavera P6 XER file; XER export/import)
- `schedulei-smartsheet` (Smartsheet API; project sheets)

### Config knobs
- Credentials per backend
- Mapping template (how source task fields map to pm-core milestones, activities, and work packages — P6 activity codes or WBS levels commonly carry the package id)
- Default mapping behaviors (every source milestone becomes a pro_manage milestone? only summary tasks?)
- Round-trip policy (allow export → edit → re-import, or import only)

### Operational
- Imports are chunky by nature; use streaming RPC.
- Large schedules (10k+ activities) imported into pm-core are summarized to the milestone + top-level dependency layer; detail stays attached as the original file.
- UNTS hashes mapping templates for drift detection.

### Open items
- Confirm which backends are in scope for year-one production use.
- Decide the round-trip story: if a scheduler edits in P6 and re-imports, do we overwrite or merge?

---

## Doci — Document Store Integration

### Purpose
Mirror external document stores (SharePoint, Drive, Confluence) into pro_manage for linked-reference, permission mirroring, and search.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `ListTree(ListRequest) → stream Node`
- `GetDocumentMetadata(DocRef) → DocMetadata`
- `GetDocumentContent(DocRef) → stream FileChunk` (optional; may defer to a presigned URL)
- `GetPermissions(DocRef) → PermissionSet`
- `OnChange(SubscribeRequest) → stream ChangeEvent`

### Year-one backends
- `doci-sharepoint` (Microsoft Graph / SharePoint REST)
- `doci-drive` (Google Drive v3 API)
- `doci-confluence` (Confluence Cloud REST)

### Config knobs
- OAuth client config per backend
- Root scope (which site collection / drive / space to index)
- Permission-mirror policy (strict: reject access if upstream disagrees; soft: track drift and surface)

### Operational
- Change subscriptions reduce load vs re-scan.
- Permission drift between upstream and pro_manage is surfaced to project Owners.
- Content is **not** copied by default — pro_manage holds metadata + a reference. Owners can opt-in to local cache for offline reliability.

### Open items
- Confirm the preferred backends (likely SharePoint if the company is M365; Drive if Google Workspace).
- Decide the strict vs soft permission-mirror default.

---

## Commsi — Communication / Notification Integration

### Purpose
Deliver notifications (approval prompts, overdue nudges, digests, alerts, dashboard shares) through channels users already use.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `SendNotification(SendRequest) → SendResult` (unified payload across channels)
- `SendDigest(DigestRequest) → SendResult` (batched)
- `OnDeliveryReceipt(Receipt) → Ack` (for channels that support it)

### Year-one backends
- `commsi-smtp` (email)
- `commsi-teams` (Microsoft Teams via adaptive cards + bot or webhook)
- `commsi-slack` (Slack API + blocks)

### Config knobs
- SMTP server, from-address, DKIM/SPF alignment
- Teams tenant + app registration
- Slack workspace OAuth + default channel
- Per-channel rate limits
- Dedupe window per `(user, notification_kind, subject_id)`

### Operational
- Email size caps → use presigned MinIO links in the body for attachments.
- Teams/Slack formatting: adaptive cards with action buttons (deep-link to pro_manage).
- Failed deliveries are retried with exponential backoff; final failure logs an operational alert.

### Open items
- Confirm which channels launch day one (Teams vs Slack depends on company culture).
- Decide default digest cadence per persona (daily morning for executives; realtime for approvals).

---

## Metrici — Metric Harvest Integration

### Purpose
Read-only metric harvesting from relational DBs, time-series DBs, plant historians, and OPC UA endpoints. Feeds pm-core's TSDB (`§07`).

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `DiscoverSchema(DiscoverRequest) → SchemaResponse`
- `ReadBatch(ReadRequest) → stream Reading`
- `Subscribe(SubscribeRequest) → stream Reading` (where supported)

### Year-one backends
- `metrici-postgres` (generic Postgres with configurable queries)
- `metrici-oracle`
- `metrici-sqlserver`
- `metrici-timescale`
- `metrici-influx`
- `metrici-pi` (OSIsoft PI Web API)
- `metrici-opcua` (generic OPC UA)
- `metrici-historian` (generic Historian — Honeywell PHD, Wonderware, Aveva, etc.)

### Config knobs
- Connection string + credentials (read-only account strongly recommended)
- Query templates (for relational backends: SELECT per MetricDefinition)
- Tag mappings (for historians: pi-tag → MetricDefinition)
- Rate limits, chunking policy
- Reasonable-value bounds per metric (data quality)

### Operational
- The read-only DB user is a hard requirement; connector refuses to run with elevated privileges.
- Queries run against the source DB can be heavy; schedule and chunk accordingly, and consider mirroring to a reporting replica upstream.
- Historian pulls can be high-volume; use binning upstream where possible.

### Open items
- Prioritize backends per the company's actual stack (owner's intel pending).
- Decide whether `metrici-opcua` needs certificate-based auth from day one.

---

## HSEi — Plant Operations Signals (Safety / Quality / Compliance / Losses)

### Purpose
Pull signals from MES, WMS, QMS, HSE, and operations TPM / connected-worker systems: incidents, inspections, non-conformances, sign-off states, **priced loss events, operator abnormality tags, and skill certifications**. Feeds pro_manage's safety/quality/compliance and loss metrics, MP-information harvest, the Early Management certification gate, and Benefits Realization verification. Read-only.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `FetchIncidents(FetchRequest) → stream Incident`
- `FetchInspections(FetchRequest) → stream Inspection`
- `FetchNonconformances(FetchRequest) → stream Nonconformance`
- `FetchSignoffs(FetchRequest) → stream Signoff`
- `FetchLossEvents(FetchRequest) → stream LossEvent` — OEE / Six Big Losses events per asset, with category and (where the platform prices them) dollar value.
- `FetchAbnormalityTags(FetchRequest) → stream AbnormalityTag` — operator-raised tags from autonomous-maintenance (CIL/CILT) routines; feeds MP-information pattern detection (`§06`).
- `FetchSkillCertifications(FetchRequest) → stream Certification` — operator/technician certification status per asset or standard-work item; feeds the Early Management exit gate (`§08`).

Not every backend supports every RPC; `Capabilities` declares which, and pm-core degrades gracefully (e.g. certification recorded manually in pro_manage when no skills platform is connected).

### Year-one backends
- Vendor-specific adapters per the company's installed MES/WMS/QMS (owner's intel pending). Likely candidates: Honeywell MES, SAP MII, Siemens Opcenter, EtQ Reliance, MasterControl QMS, Intelex HSE.
- **Operations TPM / connected-worker platforms** (loss events, abnormality tags, skills): candidates from the owner's market review — Fabrico (priced Six Big Losses, verified recovery), Cryotos (pillar-structured, abnormality register), Redzone, L2L, LeanSuite (abnormality tagging, operator certification), Augmentir and Poka/IFS Connected Worker (skills matrix, guided work). pro_manage integrates with these; it does not replicate them (`§01`).

### Config knobs
- Vendor API endpoint + auth
- Project / site filter
- Pull cadence per feed
- Aging thresholds (fed into Jiminy constraints as inputs; constraints themselves live in the governance space)

### Operational
- These systems often require VPN or site-specific access; coordinate with site IT.
- Severity classification mapping from vendor codes to pro_manage severity levels (`low | med | high | critical`) is per-backend.

### Open items
- Confirm vendor list (owner's intel pending). Likely a longer tail than other connector types.
- Decide whether MES/QMS write-back is desired (first instinct: no — HSEi stays read-only to simplify audit).

---

## CMMSi — Computerized Maintenance Management System Integration

### Purpose
Bidirectional integration with the plant CMMS. The symmetry makes this distinct from HSEi:
- **Read**: harvest MP (Maintenance Prevention) information — equipment failure history, repeat work orders, high-MTTR assets, aged open work — into pro_manage's `MPInformation` entity, which then surfaces via MDEMG during Design-stage reviews on later projects.
- **Write**: at Early Management exit, push the TurnoverPackage into the CMMS — equipment master data, spares, PM task library, operating procedures, cleaning procedures, LOTO procedures, calibration schedule — so Operations and Maintenance inherit a loaded, ready-to-run asset.

### gRPC interface (summary; see `pm.connector.v1.cmmsi.proto`)
- `HealthCheck`, `Capabilities`
- **Read side:**
  - `FetchEquipmentHistory(FetchEquipmentHistoryRequest) → stream EquipmentHistoryEntry` — failures, work orders, downtime by asset.
  - `FetchHighFailureAssets(FetchHighFailureAssetsRequest) → stream AssetFailureSummary` — top-N by failure count / MTBF / MTTR across a filter.
  - `FetchAgedWork(FetchAgedWorkRequest) → stream WorkOrder` — open work orders aged past threshold.
  - `FetchSpareUsage(FetchSpareUsageRequest) → stream SpareUsageEntry` — spare parts consumption, lead-time outliers.
- **Write side:**
  - `UpsertEquipment(UpsertEquipmentRequest) → UpsertResponse` — create or update the equipment hierarchy.
  - `UpsertSpares(UpsertSparesRequest) → UpsertResponse` — spares with suppliers, min/max, criticality.
  - `UpsertPMTasks(UpsertPMTasksRequest) → UpsertResponse` — PM task library with frequencies and procedures.
  - `UpsertProcedures(UpsertProceduresRequest) → UpsertResponse` — operating / cleaning / LOTO procedures.
  - `UpsertCalibrationSchedule(UpsertCalibrationScheduleRequest) → UpsertResponse` — calibration intervals + certificate refs.
  - `AttachCMMSArtifact(AttachCMMSArtifactRequest) → UpsertResponse` — bind pro_manage document refs to CMMS equipment records.
- **Status:**
  - `GetTurnoverStatus(GetTurnoverStatusRequest) → TurnoverStatus` — pro_manage polls after push to confirm equipment/spares/PMs landed; drift is logged.

All write operations are **idempotent** (keyed on pro_manage's turnover package id + sub-item id); retries are safe. Delivery outcome is persisted in `TurnoverPackage.delivered_to_cmms_at` and linked to the audit record.

### Year-one backends
Candidates (prioritize based on company's installed CMMS):
- `cmmsi-maximo` (IBM Maximo — enterprise standard in heavy industry)
- `cmmsi-sap-pm` (SAP Plant Maintenance — common where SAP ERP is in play)
- `cmmsi-fiix` (Fiix by Rockwell Automation — mid-market, common in food/beverage)
- `cmmsi-infor-eam` (Infor EAM)
- `cmmsi-mapcon` (Mapcon — smaller shops)
- `cmmsi-upkeep` (UpKeep — SMB)
- `cmmsi-maintainx` (MaintainX — mobile-first, common where operators raise requests directly)
- `cmmsi-cryotos` (Cryotos — TPM-pillar-structured CMMS; also a candidate HSEi source for abnormality tags and OEE)

### Config knobs (per instance)
- Base URL / tenant id / client credentials
- Equipment hierarchy mapping (how pro_manage's project equipment relates to the CMMS site/area/line/asset tree)
- Default criticality mapping (pro_manage severity → CMMS criticality code)
- Policy on existing records (block overwrite / merge / replace)
- Read cadence per feed (MP information harvest: daily for recent, weekly for aged, monthly for historical trends)

### Operational
- Read-side pulls are rate-limited per vendor.
- Write-side pushes happen in a single transaction per turnover package where the backend supports it; otherwise pm-core sequences and tracks progress with checkpoints.
- Permission mirroring: pro_manage's documents attached to equipment inherit CMMS-side visibility rules (surfaced for owner review).
- Schema drift (CMMS field renames, custom-field changes): UNTS hashes the mapping YAML; drift fires in CI/CD.

### Open items
- Confirm the company's CMMS vendor (owner's intel pending; likely one of SAP PM or Fiix for a mid-sized distillery). Priority of year-one backend follows.
- Decide the write-side policy on CMMS records that pre-exist pro_manage: merge (preserve CMMS-side edits) vs replace (pro_manage is source-of-truth for turnover). First instinct: merge, with conflict surfaces to the owner.
- Decide whether in-flight work orders (not from a pro_manage turnover) are read into pro_manage's audit view or stay strictly in the CMMS. First instinct: expose as read-only context on the Early Management dashboard; don't duplicate.
- Decide whether MP information harvest triggers automatically on new MPInformation observations in the CMMS (via webhook) or on polling.

---

## Shared operational concerns

All connectors share:

- **Admin UI** entry under `/admin/connectors`: register instance → test → enable → monitor.
- **Metrics**: per-instance Prometheus series (`connector_calls_total`, `connector_latency_seconds`, `connector_errors_total`).
- **Logging**: structured JSON with `instance_id`, `backend`, `operation`, `correlation_id`.
- **Health**: standard `HealthCheck` RPC; pm-core polls at configurable cadence.
- **Capability drift**: `Capabilities` RPC lets pm-core discover what a backend supports (not every ERP exposes `FetchCommitments`); pm-core routes accordingly and dashboards reflect gracefully when a feed is unavailable.

## Open items for Section 10

- Confirm backend priorities per module (ERPi first, HSEi vendor list, Commsi channels, IdPi backend) once owner intel lands.
- Decide whether connectors can be contributed by third parties post-launch — if yes, we need a lightweight signing/validation story for the gRPC interface + UATS specs.
- Decide the policy on connector containers being on the same VM as pm-core vs. on a dedicated "connector VM" for defense-in-depth. First instinct: dedicated connector VM(s), especially for backends that need VPN or elevated network access.
