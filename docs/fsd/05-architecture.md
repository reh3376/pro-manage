# 05: Architecture

## Decisions on record

- **2026-10-13:** Backend: **Go** monolithic core (`pm-core`) plus **Python sidecar** (`pm-sidecar`) for scripting-heavy work (document rendering, spreadsheet I/O, data transforms, derived-metric computation).
- **2026-10-13:** Frontend: **Next.js (TypeScript/React)**, served as a container alongside the backend.
- **2026-10-13:** Internal APIs: **gRPC** between services (core ↔ sidecar, core ↔ connector modules, workers ↔ core). External APIs (frontend ↔ core, external stakeholders ↔ core) are **HTTP REST + Server-Sent Events / WebSockets** for live dashboards.
- **2026-10-20:** **Engineering toolchain for the spine** (owner decisions at the start of development): a **monorepo in this repository** (`services/pm-core`, `services/pm-sidecar`, `services/pm-workers`, `apps/pm-frontend`, `connectors/<Type>i_<CODE>/`, `proto/`, `docs/`); Go 1.26; Python 3.14 via `uv` (every sidecar dependency ships a wheel for it); Node 22 with `pnpm`, Next.js 16, TypeScript 7; Postgres 16 and TimescaleDB 2.25 (pg16); Redis 7; MinIO; `buf` for protobuf; **goose** for SQL migrations embedded in the pm-core binary; **pgx + sqlc** for database access (hand-written SQL, generated typed Go) so row security, audit-in-transaction, and optimistic-concurrency checks stay visible in review. Dev secrets are gitignored `.env` files behind a provider interface; the dev IdP is Keycloak with a seeded realm; the dev MDEMG is a dedicated Compose project (`§06`).
- **2026-10-13:** All databases run in **Docker containers**: Postgres (OLTP), TimescaleDB (ours, metric readings), Redis (session/cache), MinIO (S3-compatible object store). MDEMG runs its own stack adjacent (its own Neo4j, Timescale, Grafana, neural sidecar).
- **2026-10-13:** Hosting footprint: **on-prem VMs with containers inside**, running docker-compose per VM. Compose topology designed to be upgraded to Kubernetes later without reshaping services.
- **2026-10-13:** **Pluggable connector framework**: each integration module (ERPi, IdPi, Sigi, Schedulei, Doci, Commsi, Metrici, HSEi, CMMSi) is a separate container exposing a gRPC interface defined per-module. pm-core discovers and routes to connectors via an admin-managed registry.

## Logical architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│                         Users (browsers)                               │
│  Owners · Teammates · Executives · Finance · HSE · Admins · External   │
└─────┬──────────────────────────────────────────────────────┬───────────┘
      │ HTTPS                                                 │ HTTPS (shared dashboards)
      ▼                                                       ▼
┌───────────────────────────────┐              ┌──────────────────────────┐
│   pm-frontend (Next.js)       │              │  pm-frontend-public      │
│   SSR+CSR, auth guards        │              │  (read-only renderer     │
│                               │              │   for tokenized shares)  │
└────────────┬──────────────────┘              └──────────────┬───────────┘
             │ REST + SSE/WS                                   │
             │ (OIDC-protected)                                │
             ▼                                                 ▼
┌───────────────────────────────────────────────────────────────────────┐
│                        pm-core (Go)                                   │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐ ┌──────────────┐  │
│  │ Edge /       │ │ Service      │ │ Projection / │ │ Workflow     │  │
│  │ Auth / RBAC  │ │ Handlers     │ │ Masking      │ │ Engine (§08) │  │
│  └──────┬───────┘ └──────┬───────┘ └──────┬───────┘ └──────┬───────┘  │
│         ├────────────────┴────────────────┴────────────────┤          │
│         │                                                  │          │
│  ┌──────▼──────────┐    ┌──────────────────┐    ┌─────────▼────────┐  │
│  │ Postgres driver │    │ TSDB driver (our)│    │ Outbox + MDEMG   │  │
│  │ (OLTP)          │    │ (metric readings)│    │ dispatcher       │  │
│  └──────┬──────────┘    └────────┬─────────┘    └─────────┬────────┘  │
└─────────┼────────────────────────┼────────────────────────┼──────────┘
          │                        │                        │
┌─────────▼─────────┐   ┌──────────▼───────┐   ┌───────────▼──────────┐
│   Postgres        │   │   TimescaleDB    │   │  MDEMG (adjacent)    │
│   (authoritative) │   │   (ours)         │   │  Neo4j · TSDB (its)  │
└───────────────────┘   └──────────────────┘   │  Grafana · neural    │
                                               │  Jiminy · J17 svc    │
                                               └──────────────────────┘
                             ▲
                             │ (optional: Grafana pulls MDEMG metrics only)
                             │
          ┌──────────────────┼────────────────────────────────────────┐
          │                  │                                        │
┌─────────▼──────┐   ┌───────▼───────┐   ┌─────────────┐   ┌──────────▼────────┐
│  pm-sidecar    │   │  pm-workers   │   │   Redis     │   │   MinIO (S3)      │
│  (Python)      │   │  (Go)         │   │  session+   │   │   doc binaries +  │
│  render + xlsx │   │  schedulers + │   │  cache      │   │   exports         │
│  derived metr. │   │  outbox drain │   │             │   │                   │
└───────┬────────┘   └───────┬───────┘   └─────────────┘   └───────────────────┘
        │ gRPC               │ gRPC                                             
        └─────────┬──────────┘                                                  
                  │                                                             
                  │ gRPC (connector framework)                                  
                  ▼                                                             
┌──────────────────────────────────────────────────────────────────────────────┐
│  Connector modules (each a separate container, admin-registered)             │
│                                                                              │
│   ERPi_{SAP,ORA,NS,D365,ODOO…}            Schedulei_{MSP,P6,SS}              │
│   IdPi_{OIDC,SAML}                        Doci_{SP,GD,CF}                    │
│   Sigi_{DS,AS,OS}                         Metrici_{PG,ORA,MSSQL,TS,INFLUX,   │
│   Commsi_{SMTP,TEAMS,SLACK}                        PI,OPCUA,HIST}            │
│   CMMSi_{MAX,SAPPM,FIIX,MX,…}             HSEi_{<vendor>}                    │
└──────────────────────────────────────────────────────────────────────────────┘
```

## Service inventory

| Service | Language | Role | Scale shape |
|---|---|---|---|
| **pm-core** | Go | Transactional heart. Serves REST+WS to the frontend; gRPC to sidecar/workers/connectors. Edge auth, RBAC, service handlers, projection, workflow engine (resolves policy variables per project, `§08`), outbox author. | Horizontal (stateless; stickiness only for WS). Multiple replicas behind an in-cluster load balancer. |
| **pm-sidecar** | Python 3.14 | Scripting-heavy ops: PDF/PPTX render for exports, chart rendering (PNG/SVG/PDF) from `ChartDefinition` through the shared chart grammar (`§09`), xlsx import/export, custom derived-metric formulas, ad-hoc data transforms, image/thumbnail handling for documents. gRPC server; stateless. | Horizontal. |
| **pm-workers** | Go | Scheduled jobs (metric harvest triggers, scheduled exports, periodic ACL review notices, SLA tick, auto-close of inert sessions), outbox drain to MDEMG and `Commsi`, long-running imports. | Horizontal with per-job leasing (one worker claims a tick). |
| **pm-frontend** | TS/React (Next.js) | Authenticated UI. SSR for first paint, CSR for interaction. Reads live data via SSE/WS. Includes phone-first field views (CQV/SAT walkdowns, punchlists, turnover inspections) with photo/QR capture and offline sync, Phase 2 (`§09`). | Horizontal (stateless). |
| **pm-frontend-public** | TS/React (Next.js, slim bundle) | Read-only renderer for tokenized dashboard shares. Hardened: no auth-ed APIs reachable, strict CSP, no editorial UI. | Horizontal. |
| **ERPi** (per backend) | Go (Python runtime for Python-only vendor SDKs, `§15`) | Financials tier: actuals, commitments, invoices, chart of accounts, vendors. Project-controls tier where the ERP has one: structure, budgets, forecasts, EAC, earned value, subcontracts, change orders. Operational write-back and budget release where supported (`§10`). | One per configured ERP backend. Horizontal within a backend. |
| **IdPi** | Go | OIDC + SAML abstractions. Session issuing. Claims resolution. | Horizontal. |
| **Sigi** | Go | E-signature envelope lifecycle. | Horizontal. |
| **Schedulei** | Go | Schedule import/export. | Horizontal, batch-shaped. |
| **Doci** | Go | External document store proxy + permission mirror. | Horizontal. |
| **Commsi** | Go | Notification delivery with per-channel backoff and dedupe. | Horizontal. |
| **Metrici** | Go (connector shell) + per-backend adapters (Go, or Python if backend SDK is Python-only) | Read-only metric harvest. | Horizontal; backend-specific throttles. |
| **HSEi** | Go | Read-only plant operations signals: MES/WMS/QMS/HSE events, priced loss events, operator abnormality tags, skill certifications. | Horizontal. |
| **CMMSi** | Go | CMMS read (failure history, aged work, asset mirror) and write (turnover-package delivery: equipment, spares, PM tasks, procedures). Idempotent writes. | One per configured CMMS backend. |
| **Postgres** | n/a | OLTP source of truth. | Primary + hot standby; async WAL replica off-box for DR. |
| **TimescaleDB (ours)** | n/a | Metric readings and loss events. | Primary with a streaming standby; tiered retention. |
| **Redis** | n/a | Session store, rate-limit counters, warmed-guidance snapshot cache. | Sentinel pair. |
| **MinIO** | n/a | Object storage for documents, exports, archived signed envelopes. | 4-node erasure-coded, or SAN-backed if the company has standard storage. |
| **MDEMG stack** | Go + Python | Advisory layer. Operates alongside pm-core on an internal Docker network, reachable only from pm-core. Includes its local language model (`llama-server` serving `mdemg-llm-v1` on the host's loopback); nothing it processes leaves the intranet (`§06`). | Follows MDEMG's own deployment guide; the LLM is bound to one host. |

### gRPC surface (internal)

Internal gRPC is grouped into services with stable `.proto` contracts governed by UDTS:

- `pm.core.v1.ProjectService`: project CRUD, member management, ACL grants.
- `pm.core.v1.BudgetService`, `.ScheduleService`, `.ScopeService`, `.RiskService`, `.DocumentService`, `.MetricService`, `.WorkflowService`, `.AuditService`, `.GuidanceService`: per-section surfaces.
- `pm.sidecar.v1.RenderService`: PDF/PPTX render; input is a template id + bound data payload, output is a stream of chunks + a final `storage_key`.
- `pm.sidecar.v1.SpreadsheetService`: xlsx/csv import/export.
- `pm.sidecar.v1.DerivedMetricService`: formula evaluation with versioned, sandboxed Python.
- `pm.connector.v1.Erpi`, `.Idpi`, `.Sigi`, `.Schedulei`, `.Doci`, `.Commsi`, `.Metrici`, `.Hsei`, `.Cmmsi`: one service per connector type (see "Modular connector framework" below).

Every gRPC contract has a UDTS spec in `docs/api-spec/udts/` and the generated code is governed by UNTS hash integrity so renames are visible in CI.

### REST surface (external, authenticated)

- Namespace: `/api/v1/*`. OIDC bearer tokens required.
- Resource-oriented: `/api/v1/projects/{id}/budget`, `/api/v1/projects/{id}/approvals`, etc.
- Live updates via SSE at `/api/v1/projects/{id}/stream` (per-section channels gated by ACL).
- Covered by **UATS** contract tests from day one.

## Modular connector framework

All external-system connectors share a common shell + a per-connector-type interface. The framework is the single place pm-core talks to the outside world (except MDEMG, which is itself called by pm-core directly over its REST surface).

### Framework pieces

- **Connector interface (gRPC):** every connector-type service declares its RPCs in its own `.proto`. Health, capability discovery, and lifecycle RPCs are standard; backend-specific RPCs are per-connector-type (ERPi has `FetchActuals`; Sigi has `CreateEnvelope`; etc.).
- **Module naming:** each backend implementation is a module named `<Type>i_<CODE>`, e.g. `ERPi_NS` (NetSuite), `CMMSi_MX` (MaintainX). Full convention and code registry in `§10`.
- **Connector SDK:** modules are built on the SDK in `§15`, which supplies the base gRPC server, tier declaration, vault-backed config, auth and integration helpers, the conformance kit, and the module manifest that drives registration and the admin configuration form.
- **Capability tiers and deployment profile:** connector types can declare tiers beyond their base contract (for ERPi: `financials`, `project_controls`, `operational_writeback`, `budget_release`; see `§10`). pm-core reads each ERPi instance's tiers at startup to set the profile (**ERP-led** or **pro_manage-led**, `§03`) for the portfolios routed to that instance, which decides whether their project-controls records are owned locally or mirrored.
- **Connector registry (Postgres):** `connector_instances` table with `instance_id`, `type` (`erpi|idpi|sigi|…`), `module` (e.g. `ERPi_SAP`, `ERPi_NS`), `name` (admin-chosen), `endpoint` (gRPC target), `enabled`, `config_ref` (vault pointer), `health_status`, `last_health_at`. An `ERPi_SAP` instance and an `ERPi_NS` instance can coexist for different business units.
- **Routing (pm-core):** when pm-core needs an operation, it looks up registered connector instances for that `type`, filters by `enabled` + `health_status`, picks one (round-robin for stateless; routed by `project_id → connector_instance_id` mapping for stateful cases).
- **Config:** per-instance config lives in the deployment vault; the `config_ref` on the registry row points to it. Admin UI edits the ref; it does not reveal secret values.
- **Health check:** every connector exposes `HealthCheck() → Status`. pm-core polls on a configurable cadence (default 60s); unhealthy instances are routed away.
- **UDTS contract:** every connector-type `.proto` has UDTS specs. Modules of the same type (e.g. `ERPi_SAP` vs `ERPi_NS`) run the same contract tests, so swapping backends surfaces behavior gaps immediately.

### Per-connector-type contract (sketch)

#### ERPi
```proto
service Erpi {
  rpc HealthCheck(HealthRequest) returns (HealthResponse);
  rpc Capabilities(CapabilitiesRequest) returns (CapabilitiesResponse);

  rpc FetchActuals(FetchActualsRequest) returns (stream Actual);
  rpc FetchCommitments(FetchCommitmentsRequest) returns (stream Commitment);
  rpc FetchInvoices(FetchInvoicesRequest) returns (stream Invoice);
  rpc FetchChartOfAccounts(FetchCoARequest) returns (stream Account);
  rpc FetchVendors(FetchVendorsRequest) returns (stream Vendor);
}
```

#### IdPi
```proto
service Idpi {
  rpc AuthenticateInteractive(AuthRequest) returns (AuthResponse);
  rpc ValidateToken(TokenRequest) returns (TokenValidation);
  rpc RefreshToken(RefreshRequest) returns (RefreshResponse);
  rpc ResolveClaimsToRoles(ClaimsRequest) returns (RolesResponse);
  rpc Logout(LogoutRequest) returns (LogoutResponse);
}
```

#### Sigi
```proto
service Sigi {
  rpc CreateEnvelope(EnvelopeSpec) returns (Envelope);
  rpc GetEnvelopeStatus(EnvelopeRef) returns (EnvelopeStatus);
  rpc CancelEnvelope(EnvelopeRef) returns (CancelResponse);
  rpc DownloadSignedArtifact(EnvelopeRef) returns (stream FileChunk);
  rpc OnWebhook(Webhook) returns (WebhookAck);  // backend notifies on sign events
}
```

#### Metrici
```proto
service Metrici {
  rpc DiscoverSchema(DiscoverRequest) returns (SchemaResponse);
  rpc ReadBatch(ReadRequest) returns (stream Reading);  // historian pull
  rpc Subscribe(SubscribeRequest) returns (stream Reading);  // live, when backend supports
}
```

Other connector types follow the same shape: `HealthCheck`, `Capabilities`, and type-specific RPCs. Full per-type RPC lists, including CMMSi's symmetric read/write surface and HSEi's loss, abnormality-tag, and certification feeds, are in `§10`.

### Admin surface for connectors

Platform Admin UI exposes:
- Register a new connector instance (pick type, pick backend, name it, point to its endpoint, link to a vault config).
- Enable / disable / probe-health per instance.
- Route rules by site or portfolio (e.g., "all projects at site S use the `CMMSi_MX` instance for that plant"; "all projects in portfolio X use the `ERPi_SAP` primary instance; fail over to the `ERPi_SAP` DR instance"). A `Site`'s `default_connector_instances` (`§03`) is the first routing rule consulted.
- View recent calls, errors, latencies per instance.

## Deployment topology (on-prem VMs with containers)

Baseline topology for a site-wide (100–500 users, 200+ projects) deployment:

```
VM: pm-app-01            VM: pm-app-02           Host: pm-mdemg-01 (*)
  pm-core x N              pm-core x N             mdemg
  pm-sidecar x M           pm-sidecar x M          mdemg-neo4j
  pm-workers x K           pm-workers x K          mdemg-tsdb
  pm-frontend x 2          pm-frontend x 2         mdemg-grafana
                                                   mdemg-neural
                                                   llama-server (:8102, loopback; local LLM)

(*) Apple Silicon host as MDEMG packages it, or a Linux VM with GPU
    pass-through; the model never runs off this host. See §06.

VM: pm-data-01           VM: pm-data-02 (standby)
  postgres-primary         postgres-standby (streaming replica)
  tsdb-ours                tsdb-ours-standby
  redis-sentinel           redis-sentinel
  minio-node-1-2           minio-node-3-4

VM(s): pm-connectors-{01..N}
  Any mix of connector containers; one container per instance per backend.
  Scaled per-connector-type based on throughput needs.

VM: pm-proxy
  Reverse proxy / ingress (nginx or envoy) terminating TLS;
  routes to pm-frontend (user traffic) and pm-core API (internal).
  Hosts the IdPi callback endpoints.

VM: pm-observe
  Prometheus, Loki (logs), Alertmanager, Grafana.
```

- Internal docker network per VM for intra-VM traffic; a dedicated overlay or VLAN per host-pair for cross-VM traffic (encrypted at the network layer).
- MDEMG binds loopback-only inside its host. pm-core reaches it via an internal, authenticated gRPC/HTTP tunnel (an SSH-forwarded socket or a dedicated routable address within the overlay). MDEMG's language model is served on the same host's loopback and is reachable from nothing but MDEMG.
- No service except the reverse proxy is reachable from the user network. Everything else is private.
- Inbound calls from cloud-hosted systems (IFS Cloud event push, DocuSign webhooks) also terminate at the reverse proxy, are off until IT approves an inbound path, and are restricted and authenticated per `§11`; until then the affected connectors poll.

### Readiness for Kubernetes

The compose topology is explicitly shaped so that each service is:
- Stateless or has a well-defined state store it points to (no in-container state).
- Discoverable via environment variables (service URL overrides).
- Instrumented with Prometheus metrics on a standard port.
- Health-checked via a standard endpoint.

Later migration to Kubernetes is a lift-and-shift, not a redesign.

## Observability

- **Logs:** structured JSON to stdout; shipped to Loki (or company log aggregator) with correlation ids (`trace_id`, `request_id`, `session_id`, `audit_id`, MDEMG `request_id`).
- **Metrics:** Prometheus scrapes `/metrics` on every service. Standard RED metrics plus domain counters (approvals in-flight, metric harvest lag, Jiminy guidance rate, outbox depth, connector error rates, etc.).
- **Traces:** OpenTelemetry; traces exported to Jaeger or Tempo. End-to-end tracing is a hard requirement: a slow approval flow should be diagnosable across pm-core → connector → MDEMG → sidecar.
- **Dashboards:** operational dashboards for ops (service health, DB health, connector health, outbox depth, MDEMG health). Business dashboards built on top of pm-core are a different surface (`§09`).
- **Alerts:** Alertmanager routes to Commsi (email + Teams/Slack) with on-call rotations configurable per class.

## Data flow: illustrative sequences

### DF-01: User approves a CAR (UC-02)
```
browser ──► pm-core.Edge (REST POST)
pm-core.Edge ──► pm-core.Service.Workflow (gRPC in-process)
pm-core.Service.Workflow ──► MDEMG /v1/jiminy/classify (REST) x2 in parallel
                              [cached pre-classified verdict if payload unchanged;
                               else budget = mdemg.classify.timeout_ms]
pm-core.Service.Workflow ──► Sigi.CreateEnvelope (gRPC)
Sigi backend ─webhook──► pm-core.Edge (/sigi/webhook)           [later: user signs]
pm-core.Service.Workflow ──► MDEMG /v1/jiminy/feedback (REST async via outbox)
pm-core ──► postgres (AuditEvent + GuidanceOutcome in same txn)
pm-core ──► Commsi.SendNotification (gRPC)                      [notify next approver]
```

### DF-02: Metric harvest tick (UC-09)
```
pm-workers.scheduler fires on cron
pm-workers ──► Metrici.ReadBatch (gRPC, streaming)
Metrici backend ──pulls──► external historian/DB
pm-workers ──► pm-core.MetricService.Ingest (gRPC)
pm-core ──► tsdb-ours (bulk write)
pm-core ──► Redis (invalidate dashboard caches)
pm-core ──► SSE push to subscribed browser sessions
```

### DF-03: Spreadsheet import (UC-10)
```
browser ──► pm-core.Edge (REST POST, multipart xlsx)
pm-core ──► MinIO (stash raw file)
pm-core ──► pm-sidecar.SpreadsheetService.ValidateImport (gRPC, stream)
pm-sidecar ──► returns row-by-row validation
browser renders validation UI
user accepts ──► pm-core.Edge
pm-core ──► pm-sidecar.SpreadsheetService.ApplyImport (gRPC)
pm-sidecar ──► returns committed BudgetBaseline + line items
pm-core ──► postgres (txn: baseline + lines + audit)
pm-core ──► MDEMG /v1/conversation/observe (via outbox)
```

### DF-04: On-demand steering deck (UC-08)
```
browser ──► pm-core.Edge (REST POST)
pm-core ──► pm-core.Service.Export (gather bound data from postgres + tsdb)
pm-core ──► pm-sidecar.RenderService.RenderPPTX (gRPC, template + data payload)
pm-sidecar ──► returns stream of chunks
pm-core ──► MinIO (store output)
pm-core ──► browser (presigned URL)
```

## Development and environments

- **local dev:** docker-compose on a developer workstation runs the full stack minus some connectors. Developer-mode seeds the fictional **Northfield Spirits Co.** (sites `NF-KY`, Kentucky, USD, TTB; `NF-ON`, Ontario, CAD, CFIA and excise) with five projects across the type and amount range. MDEMG runs as a dedicated Compose project with its own Neo4j and TimescaleDB, the server image pinned to `v0.11.0-beta.3`, sharing the workstation's `llama-server` on `:8102` via `host.docker.internal` (`§06`). Keycloak provides SSO with a seeded realm; Mailpit catches Commsi mail.
- **CI:** every PR runs the merge-blocking UxTS set (UATS, UDTS, UNTS from Phase 0; UOTS and USTS from Phase 1), the adjudication-registry verifiers, Go unit tests, Python unit tests, Next.js type check + build, and a smoke-test compose spin-up with synthetic data and the pinned MDEMG. No test step carries `continue-on-error` (`§12`).
- **staging:** full topology at reduced scale; mirrors production integrations with sandbox credentials for IdPi, Sigi, ERPi, etc.
- **production:** the deployment topology above.

## Open items for Section 05

- Confirm Kubernetes migration timeline target (year two? year three?). Shapes how much compose-only tooling we invest in now.
- Decide where PDF/PPTX templates are authored and stored: in the repo (Git-tracked)? In an admin UI (DB-stored)? Both?
- Decide on message bus for the outbox: Postgres-backed outbox + worker (simple, our default), vs. NATS/Kafka for higher-throughput asynchronous flows. First instinct: Postgres outbox for MVP; revisit if we see >1M events/day.
- Decide whether pm-sidecar runs the Python-based Metrici backends (keeping all Python together) or whether each Python-dependent backend ships as its own container (keeping the sidecar lean). First instinct: separate per-backend containers for isolation.
- Confirm whether derived-metric formulas run in a sandboxed Python (via pm-sidecar) or in a safer expression language (CEL, Starlark). The research signal: user said "Python sidecar for scripting." Default to sandboxed Python with an allowlist.
