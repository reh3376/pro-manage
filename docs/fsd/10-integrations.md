# 10: Integrations Catalog

## Decisions on record

- **2026-10-13:** All external integrations are **modular** (`§01`, `§05`). Each is a separately-deployed container implementing a stable per-connector-type gRPC interface, admin-registered via the `connector_instances` registry.
- **2026-10-13:** Every connector type ships with a **UATS spec** covering its gRPC + HTTP surface. Every connector instance presents a **standard `HealthCheck` + `Capabilities`** RPC so pm-core can route around unhealthy or capability-mismatched instances.
- **2026-10-13:** Secrets per connector instance live in the deployment vault; the `config_ref` in `connector_instances` points to the vault entry.
- **2026-10-13:** Connectors never bypass pro_manage's authorization. All project/data access from pm-core to a connector is already pre-authorized at pm-core's edge.

This section is a catalog. The architecture, framework, and gRPC contract templates are in `§05`. The admin UX for routing and health is in `§05` and `§11`. This section records the specifics per module. New modules are built with the Connector SDK (`§15`); `pm-connector new --type <Type> --code <CODE>` scaffolds one.

## Connector module naming

Every backend implementation is a connector module named **`<Type>i_<CODE>`**:

- `<Type>i` is the connector type: `ERPi`, `IdPi`, `Sigi`, `Schedulei`, `Doci`, `Commsi`, `Metrici`, `HSEi`, `CMMSi`.
- `<CODE>` is a short uppercase code for the vendor or protocol, unique within the type: `ERPi_NS` is the NetSuite ERP connector, `ERPi_SAP` the SAP connector, `CMMSi_MX` the MaintainX connector.
- A module implements its type's gRPC contract and passes that type's UDTS specs, so swapping `ERPi_NS` for `ERPi_SAP` is a configuration change, not a core release.
- Modules are listed in a code registry (the tables below and the `connector_modules` table). New codes are added there when a module is written.
- **Instances vs modules:** a deployment registers one or more *instances* of a module in `connector_instances` (`§05`), e.g. `ERPi_NS` for one business unit and `ERPi_SAP` for another, each with its own vault config.
- **Container and proto names** are lowercase because Docker requires it: `ERPi_NS` ships as image `pm-erpi-ns` and service `pm.connector.v1.Erpi` with backend id `ns`.

Codes are assigned for the candidate backends below. The specific modules a deployment needs are chosen once the company's ERP, CMMS, MES/QMS, and identity provider are confirmed.

---

## ERPi: ERP Integration

### Purpose
Connect pro_manage to the company ERP. Every ERP module provides **financials** (actuals, commitments, invoices, chart of accounts, vendors). ERPs with a capital-projects module also provide **project controls** (project structure, budgets and forecasts, EAC, earned value, subcontracts, change orders), and when they do, the ERP is the system of record for those records and pro_manage mirrors them (see "Capability tiers" below and `§03` deployment profiles).

**Write-back is deliberately narrow:** operational records (tasks, work requests, notes, readiness flags, deviations) and **one controlled financial write**, releasing the authorized budget when a CAR is approved. pro_manage never edits forecasts, actuals, or other financial records in the ERP; those change only through the ERP's own controlled processes, so the audit trail stays in one place.

### Capability tiers

| Tier | Declared via `Capabilities` | pro_manage behavior |
|---|---|---|
| `financials` | Always | Pull actuals, commitments, invoices, CoA, vendors. pro_manage owns WBS, budgets, forecasts, EVM, contracts, change orders (profile **pro_manage-led**). |
| `project_controls` | ERP has a capital-projects module (e.g. IFS Cloud) | Additionally mirror project structure, budget and forecast versions, EAC, EVM measures, subcontracts, change orders, payment applications. The ERP owns them (profile **ERP-led**). |
| `operational_writeback` | ERP accepts tasks / work requests / notes against project objects | pro_manage may push deviations, readiness flags, and improvement actions as ERP tasks or notes. |
| `budget_release` | ERP supports creating or releasing a project budget via API | On CAR approval, pro_manage releases the authorized budget in the ERP (one audited write per approval). |

### gRPC interface (summary; see `pm.connector.v1.erpi.proto`)
- `HealthCheck`, `Capabilities`
- `FetchActuals(FetchActualsRequest) → stream Actual`
- `FetchCommitments(FetchCommitmentsRequest) → stream Commitment`
- `FetchInvoices(FetchInvoicesRequest) → stream Invoice`
- `FetchChartOfAccounts(FetchCoARequest) → stream Account`
- `FetchVendors(FetchVendorsRequest) → stream Vendor`
- `FetchPurchaseOrders(FetchPORequest) → stream PurchaseOrder` and `FetchReceipts(FetchReceiptsRequest) → stream Receipt`: PO lines, promised dates, and goods receipts for the project's commitments, mirrored onto `ProcurementItem` rows (`§03`); the ERP performs procurement, pro_manage tracks expediting
- *project_controls tier:* `FetchProjectStructure` (projects, sub-projects, activities, milestones), `FetchBudgetVersions`, `FetchForecastVersions`, `FetchEac`, `FetchEarnedValue` (BCWS/BCWP/ACWP, CPI/SPI), `FetchSubcontracts`, `FetchChangeOrders`, `FetchPaymentApplications`, `SubscribeProjectEvents` (where the ERP can emit events: new change order, forecast version published, budget variance threshold crossed)
- *operational_writeback tier:* `CreateTask`, `AddNote`, `SetReadinessFlag` against a project or activity reference
- *budget_release tier:* `ReleaseBudget(car_id, project_ref, amount, approval_ref)`; idempotent on `car_id`

### Year-one backends
**First target: `ERPi_IFS` (IFS Cloud)**, the ERP in use at the first target deployment. IFS Cloud combines ERP, enterprise asset management (EAM), and field service on one data model and has a capital-projects module (WBS, parallel forecasting, EAC workbench, earned value, subcontracts with payment applications and retentions, change control, project-to-asset handover). `ERPi_IFS` is expected to declare all four tiers, making that deployment **ERP-led**. Still to confirm: which IFS modules are licensed (Projects, EAM, HCM, shop floor / MES), and the projection names for the Projects module in that environment (see below).

Other candidate backends:
- `ERPi_SAP` (SAP ECC via OData/BAPI; S/4HANA via CDS views)
- `ERPi_ORA` (EBS tables or Fusion REST APIs)
- `ERPi_NS` (SuiteTalk REST)
- `ERPi_D365` (Dynamics 365 F&O REST)
- `ERPi_ODOO` (Odoo XML-RPC / JSON-RPC)

Further ERP modules follow the same pattern (e.g. `ERPi_INFOR`, `ERPi_EPICOR`, `ERPi_SAGE`).

### Config knobs (per instance)
- Base URL / tenant id
- Auth: user+password, OAuth, service account, cert-based
- Chart-of-accounts filter (which accounts to include)
- Project-code mapping (how pm-core's `project.code` maps to the ERP's project / WBS / cost-center id)
- Contract mapping (which ERP purchase orders / commitments correspond to which pro_manage `Contract`; contract value and invoiced-to-date flow from here)
- Pull cadence per feed (actuals daily, commitments hourly, invoices weekly as defaults; backend overrides)
- Reasonable-value bounds for data quality

### Operational
- Rate-limited per vendor's API limits (SAP has strict concurrent-session limits; Oracle has per-user hourly quotas).
- Circuit-breaker on upstream errors.
- Backfill via `FetchActualsRequest.from_ts` works for all backends; ranges > 90 days are chunked.
- UNTS hashes the field-mapping YAML; any schema drift upstream is caught in CI.

### IFS Cloud interface (`ERPi_IFS`)

Based on the owner's research into IFS Cloud integration (October 2026; validate against the deployed IFS version).

- **API:** OData v4 REST "projections" (the same services IFS's own web client uses), at `https://<env>.ifs.cloud/main/ifsapplications/projection/v1/<ProjectionName>.svc/...`, with standard query options (`$filter`, `$select`, `$expand`, `$top`, `$orderby`). Direct database access is not available in IFS's managed cloud and is not used.
- **Discovery:** projections are found with IFS's in-application **API Explorer**; each publishes an OpenAPI description at its `$openapi` endpoint, which becomes the input to the module's UDTS contract tests. Prefer **Integration- or Premium-class projections**, which IFS keeps stable across upgrades. Projection names for the Projects module (project structure, forecasts, change orders, subcontracts) are identified in the target environment, not assumed here.
- **Authentication:** OAuth 2.0 **client credentials** through IFS IAM, using a dedicated IAM client tied to a service account with a least-privilege permission set covering only the projections `ERPi_IFS` uses. The client secret lives in the deployment vault; tokens are cached and refreshed by the connector.
- **Writes:** IFS uses **ETags** for optimistic concurrency, so every write (tasks, notes, readiness flags, `ReleaseBudget`) sends the record's ETag and treats a mismatch as a conflict to re-read, never as a forced overwrite.
- **Events from IFS:** IFS can call out to an endpoint the connector exposes, via **IFS Connect** (REST sender, routing rules) or **Event Actions** of type REST Call. `SubscribeProjectEvents` uses this when available. IFS Cloud is hosted outside the company network, so receiving these calls requires an approved inbound path (see `§11`). **Until that path is approved, the module polls** on a schedule using `$filter` on last-modified timestamps; once approved, IFS pushes events and polling drops to a slow reconciliation check.
- **Load:** prefer event-driven updates and narrow queries (`$select` only the fields mirrored); poll cadences are set per feed so pro_manage never becomes a heavy load on the company's IFS tenant.
- **No middleware required.** IFS also integrates through platforms such as Boomi or MuleSoft, but the connector module already provides the retries, idempotency, mapping, and error handling middleware would. If the company already standardizes on a platform, `ERPi_IFS` can sit behind it instead.
- **Rollout order:** read-only pulls first (project structure, then budgets and forecasts, then subcontracts and change orders), then operational write-back, then `ReleaseBudget`; failure cases (IFS unavailable, token expiry, ETag conflict, invalid data) tested at each step.

### IFS Cloud identifier mapping (`ERPi_IFS`)

| IFS Cloud object | pro_manage entity |
|---|---|
| Project | `Project` (`code` ↔ IFS project id) |
| Sub-project / activity | `WBSNode`; `WorkPackage.wbs_id` points at the activity |
| Milestone | `Milestone` |
| Budget / forecast version, EAC | `BudgetBaseline`, `BudgetForecast` (mirrored) |
| Subcontract | `Contract` |
| Change order | `ChangeOrder` (mirrored) |
| Purchase order / commitment | `BudgetCommitment` |
| Equipment object (EAM) | `Asset.cmms_ref` |
| Work order / work request (EAM) | target of operational write-back; MP-information source |

### Open items
- Confirm the IFS Cloud modules licensed at the first target deployment (Projects, EAM, HCM, shop floor / MES).
- Identify the Projects-module projection names in the target environment via the API Explorer and capture their OpenAPI descriptions for the UDTS specs.
- Agree with the company's IT security team whether IFS Cloud may reach an inbound endpoint for event push (and through what path), or whether `ERPi_IFS` stays polling-only.
- Set up the IFS IAM client, service account, and permission set for `ERPi_IFS`.
- Confirm whether multiple ERP instances will run concurrently (e.g., SAP for industrial division, NetSuite for services). Design supports this.

---

## IdPi: Identity Integration (SSO)

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
- `IdPi_OIDC` (generic OIDC: Entra ID, Okta, Keycloak, Google Workspace, etc. via the generic flow)
- `IdPi_SAML` (generic SAML 2.0: ADFS, Shibboleth, PingFederate, etc.)
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

## Sigi: E-signature Integration

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
- `Sigi_DS` (DocuSign eSignature REST)
- `Sigi_AS` (Adobe Acrobat Sign)
- `Sigi_OS` (OneSpan Sign)

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

## Schedulei: Schedule Tool Integration

### Purpose
Import and (optionally) export project schedules from external tools. Preserves the original file as attachment; maps to pm-core's `Milestone` + dependency model.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `ImportSchedule(ImportRequest) → ImportResult` (one-shot or streamed per project)
- `ExportSchedule(ExportRequest) → stream FileChunk`
- `ListProjects(ListRequest) → stream RemoteProjectRef` (for backends with a project API)
- `Subscribe(SubscribeRequest) → stream ScheduleEvent` (where supported)

### Year-one backends
- `Schedulei_MSP` (MPP via XML/MPX export; .mpp read-only requires an unofficial lib, so MVP likely uses XML export)
- `Schedulei_P6` (Primavera P6 XER file; XER export/import)
- `Schedulei_SS` (Smartsheet API; project sheets)

### Config knobs
- Credentials per backend
- Mapping template (how source task fields map to pm-core milestones, activities, and work packages; P6 activity codes or WBS levels commonly carry the package id)
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

## Doci: Document Store Integration

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
- `Doci_SP` (Microsoft Graph / SharePoint REST)
- `Doci_GD` (Google Drive v3 API)
- `Doci_CF` (Confluence Cloud REST)

### Config knobs
- OAuth client config per backend
- Root scope (which site collection / drive / space to index)
- Permission-mirror policy (strict: reject access if upstream disagrees; soft: track drift and surface)

### Operational
- Change subscriptions reduce load vs re-scan.
- Permission drift between upstream and pro_manage is surfaced to project Owners.
- Content is **not** copied by default; pro_manage holds metadata + a reference. Owners can opt-in to local cache for offline reliability.

### Engineering document management systems
Where the company or its EPCM runs an EDMS (Aconex, ProjectWise, IFS Document Management, or a SharePoint library with revision metadata), a Doci module can mirror the **document register and revision status** (`§03 DocumentRevision`) in addition to files and permissions: `ListRegister(ProjectRef) → stream RegisterEntry` and `GetRevisionStatus(DocRef) → RevisionStatus` are optional capabilities declared in the module manifest. pro_manage remains the system that ties revisions to packages, readiness, and losses; the EDMS remains where the files and the formal issue happen if the company prefers it that way.

### Open items
- Confirm the preferred backends (likely SharePoint if the company is M365; Drive if Google Workspace).
- Decide the strict vs soft permission-mirror default.
- Confirm whether the EPCM's EDMS should be mirrored (register and revision status) or whether contributors raise transmittals in pro_manage directly; this decides whether a `Doci_ACX`-class module is in year one.

---

## Commsi: Communication / Notification Integration

### Purpose
Deliver notifications (approval prompts, overdue nudges, digests, alerts, dashboard shares) through channels users already use.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `SendNotification(SendRequest) → SendResult` (unified payload across channels)
- `SendDigest(DigestRequest) → SendResult` (batched)
- `OnDeliveryReceipt(Receipt) → Ack` (for channels that support it)

### Year-one backends
- `Commsi_SMTP` (email)
- `Commsi_TEAMS` (Microsoft Teams via adaptive cards + bot or webhook)
- `Commsi_SLACK` (Slack API + blocks)

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

## Metrici: Metric Harvest Integration

### Purpose
Read-only metric harvesting from relational DBs, time-series DBs, plant historians, and OPC UA endpoints. Feeds pm-core's TSDB (`§07`).

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `DiscoverSchema(DiscoverRequest) → SchemaResponse`
- `ReadBatch(ReadRequest) → stream Reading`
- `Subscribe(SubscribeRequest) → stream Reading` (where supported)

### Year-one backends
- `Metrici_PG` (generic Postgres with configurable queries)
- `Metrici_ORA`
- `Metrici_MSSQL`
- `Metrici_TS`
- `Metrici_INFLUX`
- `Metrici_PI` (OSIsoft PI Web API)
- `Metrici_OPCUA` (generic OPC UA)
- `Metrici_HIST` (generic Historian: Honeywell PHD, Wonderware, Aveva, etc.)

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
- Decide whether `Metrici_OPCUA` needs certificate-based auth from day one.

---

## HSEi: Plant Operations Signals (Safety / Quality / Compliance / Losses)

### Purpose
Pull signals from MES, WMS, QMS, HSE, and operations TPM / connected-worker systems: incidents, inspections, non-conformances, sign-off states, **priced loss events, operator abnormality tags, and skill certifications**. Feeds pro_manage's safety/quality/compliance and loss metrics, MP-information harvest, the Early Management certification gate, and Benefits Realization verification. Read-only.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `FetchIncidents(FetchRequest) → stream Incident`
- `FetchInspections(FetchRequest) → stream Inspection`
- `FetchNonconformances(FetchRequest) → stream Nonconformance`
- `FetchSignoffs(FetchRequest) → stream Signoff`
- `FetchLossEvents(FetchRequest) → stream LossEvent`: OEE / Six Big Losses events per asset, with category and (where the platform prices them) dollar value.
- `FetchAbnormalityTags(FetchRequest) → stream AbnormalityTag`: operator-raised tags from autonomous-maintenance (CIL/CILT) routines; feeds MP-information pattern detection (`§06`).
- `FetchSkillCertifications(FetchRequest) → stream Certification`: operator/technician certification status per asset or standard-work item; feeds the Early Management exit gate (`§08`).

Not every backend supports every RPC; `Capabilities` declares which, and pm-core degrades gracefully (e.g. certification recorded manually in pro_manage when no skills platform is connected).

### Year-one backends
Modules are named `HSEi_<CODE>` per vendor (e.g. `HSEi_OPCENTER`, `HSEi_MII`, `HSEi_ETQ`, `HSEi_FABRICO`, `HSEi_CRYOTOS`) once the installed systems are known. **`HSEi_IFS`** is a candidate if the plants use IFS Cloud as their MES: IFS's shop-floor projections carry downtime, scrap, and quantities completed per operation, which map directly to priced loss events, through the same API and authentication as `ERPi_IFS`.
- Vendor-specific adapters per the company's installed MES/WMS/QMS (owner's intel pending). Likely candidates: Honeywell MES, SAP MII, Siemens Opcenter, EtQ Reliance, MasterControl QMS, Intelex HSE.
- **Operations TPM / connected-worker platforms** (loss events, abnormality tags, skills): candidates from the owner's market review: Fabrico (priced Six Big Losses, verified recovery), Cryotos (pillar-structured, abnormality register), Redzone, L2L, LeanSuite (abnormality tagging, operator certification), Augmentir and Poka/IFS Connected Worker (skills matrix, guided work). pro_manage integrates with these; it does not replicate them (`§01`).

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
- Decide whether MES/QMS write-back is desired (first instinct: no; HSEi stays read-only to simplify audit).

---

## CMMSi: Computerized Maintenance Management System Integration

### Purpose
Bidirectional integration with the plant CMMS. The symmetry makes this distinct from HSEi:
- **Read**: harvest MP (Maintenance Prevention) information (equipment failure history, repeat work orders, high-MTTR assets, aged open work) into pro_manage's `MPInformation` entity, which then surfaces via MDEMG during Design-stage reviews on later projects.
- **Write**: at Early Management exit, push the TurnoverPackage into the CMMS (equipment master data, spares, PM task library, operating procedures, cleaning procedures, LOTO procedures, calibration schedule) so Operations and Maintenance inherit a loaded, ready-to-run asset.

### gRPC interface (summary; see `pm.connector.v1.cmmsi.proto`)
- `HealthCheck`, `Capabilities`
- **Read side:**
  - `FetchEquipmentHistory(FetchEquipmentHistoryRequest) → stream EquipmentHistoryEntry`: failures, work orders, downtime by asset.
  - `FetchHighFailureAssets(FetchHighFailureAssetsRequest) → stream AssetFailureSummary`: top-N by failure count / MTBF / MTTR across a filter.
  - `FetchAgedWork(FetchAgedWorkRequest) → stream WorkOrder`: open work orders aged past threshold.
  - `FetchSpareUsage(FetchSpareUsageRequest) → stream SpareUsageEntry`: spare parts consumption, lead-time outliers.
- **Write side:**
  - `UpsertEquipment(UpsertEquipmentRequest) → UpsertResponse`: create or update the equipment hierarchy.
  - `UpsertSpares(UpsertSparesRequest) → UpsertResponse`: spares with suppliers, min/max, criticality.
  - `UpsertPMTasks(UpsertPMTasksRequest) → UpsertResponse`: PM task library with frequencies and procedures.
  - `UpsertProcedures(UpsertProceduresRequest) → UpsertResponse`: operating / cleaning / LOTO procedures.
  - `UpsertCalibrationSchedule(UpsertCalibrationScheduleRequest) → UpsertResponse`: calibration intervals + certificate refs.
  - `AttachCMMSArtifact(AttachCMMSArtifactRequest) → UpsertResponse`: bind pro_manage document refs to CMMS equipment records.
- **Status:**
  - `GetTurnoverStatus(GetTurnoverStatusRequest) → TurnoverStatus`: pro_manage polls after push to confirm equipment/spares/PMs landed; drift is logged.

All write operations are **idempotent** (keyed on pro_manage's turnover package id + sub-item id); retries are safe. Delivery outcome is persisted in `TurnoverPackage.delivered_to_cmms_at` and linked to the audit record.

### Year-one backends
Candidates (prioritize based on company's installed CMMS):
- `CMMSi_MAX` (IBM Maximo: enterprise standard in heavy industry)
- `CMMSi_SAPPM` (SAP Plant Maintenance: common where SAP ERP is in play)
- `CMMSi_FIIX` (Fiix by Rockwell Automation: mid-market, common in food/beverage)
- `CMMSi_EAM` (Infor EAM)
- `CMMSi_MAPCON` (Mapcon: smaller shops)
- `CMMSi_UPKEEP` (UpKeep: SMB)
- `CMMSi_MX` (MaintainX: mobile-first, common where operators raise requests directly)
- `CMMSi_IFS` (IFS Cloud EAM). If the plant's maintenance system is IFS EAM, turnover goes through IFS's **native project-to-asset handover** (equipment objects, spare parts, maintenance plans created from the project) rather than a separate equipment write; `CMMSi_IFS` then reads failure history and work orders for MP information and confirms that handover completed. If the plant runs a different CMMS, the standard CMMSi write path applies. Which case applies at the first target deployment is not yet known; both are supported.
- `CMMSi_CRY` (Cryotos: TPM-pillar-structured CMMS; also a candidate HSEi source for abnormality tags and OEE)

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
- Decide whether connectors can be contributed by third parties post-launch. If yes, we need a lightweight signing/validation story for the gRPC interface + UATS specs.
- Decide the policy on connector containers being on the same VM as pm-core vs. on a dedicated "connector VM" for defense-in-depth. First instinct: dedicated connector VM(s), especially for backends that need VPN or elevated network access.
