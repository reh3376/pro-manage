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
To be prioritized once the owner confirms the first production ERP (expected 2026-10-14). Candidate backends:
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
- Pull cadence per feed (actuals daily, commitments hourly, invoices weekly — defaults; backend overrides)
- Reasonable-value bounds for data quality

### Operational
- Rate-limited per vendor's API limits (SAP has strict concurrent-session limits; Oracle has per-user hourly quotas).
- Circuit-breaker on upstream errors.
- Backfill via `FetchActualsRequest.from_ts` works for all backends; ranges > 90 days are chunked.
- UNTS hashes the field-mapping YAML; any schema drift upstream is caught in CI.

### Open items
- Confirm first production backend (owner intel 2026-10-14).
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
Formal e-signature on CapEx approvals, change requests, closeout, and other configured workflow kinds. Envelope lifecycle, status polling, webhook ingestion, signed-artifact capture.

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
- Decide templates mapping for CapEx vs change requests vs closeout.

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
- Mapping template (how source task fields map to pm-core milestones/activities)
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

## HSEi — Safety / Quality / Compliance Integration

### Purpose
Pull signals from MES, WMS, QMS, and HSE systems: incidents, inspections, non-conformances, sign-off states. Feeds pro_manage's safety/quality/compliance metrics and surfaces aging items for guidance.

### gRPC interface (summary)
- `HealthCheck`, `Capabilities`
- `FetchIncidents(FetchRequest) → stream Incident`
- `FetchInspections(FetchRequest) → stream Inspection`
- `FetchNonconformances(FetchRequest) → stream Nonconformance`
- `FetchSignoffs(FetchRequest) → stream Signoff`

### Year-one backends
- Vendor-specific adapters per the company's installed MES/WMS/QMS (owner's intel pending). Likely candidates: Honeywell MES, SAP MII, Siemens Opcenter, EtQ Reliance, MasterControl QMS, Intelex HSE.

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
