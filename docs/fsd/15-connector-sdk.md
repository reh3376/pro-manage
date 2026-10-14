# 15: Connector SDK

## Decisions on record

- **2026-10-14:** pro_manage ships a **Connector SDK** for building `<Type>i_<CODE>` connector modules (`§10`). Vendor-specific questions (which IdP, CMMS, MES/QMS, e-signature vendor) are answered by selecting or building a module at deployment, and the SDK keeps building a new module to days of work rather than a project.
- **2026-10-14:** The SDK is a **Phase 0 deliverable**, built before the first production modules (`ERPi_IFS`, `IdPi_SAML`, `IdPi_OIDC`), so every module shares the same plumbing for authentication, retries, idempotency, concurrency, polling, inbound events, observability, and testing.
- **2026-10-14:** **Go first, Python second.** Go is the default for modules; a Python runtime with the same contract exists for vendors whose SDKs or protocols are Python-only.
- **2026-10-14:** A module is registered by its **manifest**, not by a core release. The admin UI renders the module's configuration form from the manifest's config schema.

## Why an SDK

Connectors are where integration projects usually fail: each one reinvents token handling, retries, paging, mapping, and error handling slightly differently, and the inconsistencies surface in production as silent data drift or duplicate writes. High-integration modules such as `ERPi_IFS` need all of it at once: OAuth 2.0 tokens, OData paging, ETag concurrency, event push with a polling fallback, and idempotent writes. The SDK provides that plumbing once, tested once, so a module author writes only the vendor-specific mapping and calls.

## What the SDK contains

### 1. Module runtime (`sdk/connector-go`, `sdk/connector-py`)

- A base gRPC server that implements the standard RPCs every module needs (`HealthCheck`, `Capabilities`, lifecycle) and serves the connector type's contract (`pm.connector.v1.<Type>`).
- **Tier declaration:** the module declares which capability tiers it implements (for ERPi: `financials`, `project_controls`, `operational_writeback`, `budget_release`); the runtime answers `Capabilities` from that declaration and returns a typed "not supported" for anything undeclared.
- **Configuration and secrets:** config is validated against the manifest's schema at startup; secrets are fetched from the deployment vault by reference and refreshed when rotated. Secrets never appear in logs, config files, or images.
- **Observability built in:** structured JSON logs with the standard correlation fields (`§11`), Prometheus metrics (`connector_calls_total`, `connector_latency_seconds`, `connector_errors_total`, plus per-helper counters), and OpenTelemetry traces that join pm-core's traces.
- **Graceful lifecycle:** readiness only after config, secrets, and the upstream credential check succeed; draining on shutdown so in-flight writes finish or are safely retried.

### 2. Integration helpers

| Helper | What it does | Typical users |
|---|---|---|
| OAuth 2.0 client | Client-credentials and authorization-code flows; token caching and refresh; per-tenant clients | `ERPi_IFS`, `ERPi_NS`, `Doci_SP`, `Commsi_TEAMS` |
| SAML / OIDC toolkit | Metadata loading, signature and certificate validation, assertion and claims parsing, key and certificate rotation | `IdPi_SAML`, `IdPi_OIDC` |
| Resilient HTTP client | Timeouts, retries with exponential backoff and jitter, circuit breaker, per-destination rate limits, egress through the controlled proxy (`§11`) | All HTTP-based modules |
| OData helper | Query building (`$filter`, `$select`, `$expand`), server-driven paging, `$openapi` retrieval | `ERPi_IFS`, `ERPi_D365`, `ERPi_SAP` |
| Idempotency | Idempotency keys on every write (e.g. `car_id` for `ReleaseBudget`), a local ledger of completed writes, safe replay after a crash | Every module with a write tier |
| Concurrency | ETag / version handling: send the version on update, treat a mismatch as a conflict to re-read and re-evaluate, never force-overwrite | `ERPi_IFS`, `CMMSi_*` |
| Polling scheduler | Watermark-based incremental pulls (last-modified or sequence), configurable cadence per feed, backfill in chunks, leasing so only one replica polls a feed | All pull-based feeds |
| Inbound event receiver | Endpoint for pushed events (IFS Connect, Event Actions, DocuSign webhooks): authentication and signature verification, sender allowlist, de-duplication, ordering where needed, hand-off to the same pipeline polling uses | `ERPi_IFS`, `Sigi_DS`, `Doci_*` |
| Push/poll switch | One setting moves a feed between polling and push; with push on, polling drops to a slow reconciliation check | `ERPi_IFS` |
| Streaming | Server-streaming helpers for large result sets (actuals, readings, history) with back-pressure | `ERPi_*`, `Metrici_*`, `CMMSi_*` |

### 3. Mapping layer

- Field mappings between the vendor's objects and pro_manage's connector messages are written as **declarative YAML** (field paths, type conversions, code lookups, unit and currency handling), not hand-written code, wherever the mapping is mechanical.
- Mapping files are hashed in the UNTS registry (`§12`), so any change is reviewed and schema drift is visible in CI.
- A dry-run command applies a mapping to recorded vendor payloads and shows the resulting messages, so mapping changes can be checked before deployment.

### 4. Conformance kit

- Each connector type's UDTS contract specs (`§12`) are packaged as a **conformance suite** that any module of that type must pass: the same tests whether the module targets SAP, NetSuite, or IFS.
- A **mock pm-core** drives the module through the contract, including failure cases: upstream unavailable, token expiry, rate limiting, ETag conflict, duplicate event delivery, malformed data.
- **Recorded fixtures:** vendor responses captured from sandboxes (scrubbed of personal and commercial data) let the suite run in CI without vendor access. A live mode runs the same suite nightly against vendor sandboxes where available.
- Passing the conformance suite for the declared tiers is required before a module version can be registered.

### 5. Scaffolding CLI

`pm-connector new --type ERPi --code IFS` generates a working skeleton:

- module source with the tier interfaces stubbed
- the manifest (`connector.yaml`), config schema, and an empty mapping file
- Dockerfile (image `pm-erpi-ifs`, following the naming rule in `§10`)
- CI workflow running unit tests, the conformance suite, UNTS checks, and an image build
- a README template for the module's vendor notes and open questions

Further commands: `pm-connector test` (conformance suite locally), `pm-connector map --dry-run` (mapping check), `pm-connector package` (versioned image plus manifest).

### 6. Module manifest

Every module carries a manifest that pm-core reads at registration. Example for `ERPi_IFS`:

```yaml
module: ERPi_IFS
type: ERPi
code: IFS
version: 0.1.0
contract: pm.connector.v1.Erpi >= 1.0, < 2.0
runtime: go
image: pm-erpi-ifs
tiers: [financials, project_controls, operational_writeback, budget_release]
config_schema:
  base_url:        { type: url, required: true }      # https://<env>.ifs.cloud
  projections:     { type: map, required: true }      # feed -> projection name, from API Explorer
  poll_cadence:    { type: map, default: { actuals: 24h, commitments: 1h, project_structure: 1h } }
  event_push:      { type: bool, default: false }     # true only after IT approves an inbound path
secrets:
  - iam_client_id
  - iam_client_secret
egress: ["<env>.ifs.cloud:443"]
inbound:
  - name: ifs_events
    auth: oauth2_or_signed
    enabled_by: event_push
```

The admin UI uses `config_schema` to render the configuration form, `secrets` to prompt for vault references, and `egress` / `inbound` to tell the Platform Admin exactly which network rules the module needs.

### 7. Packaging and registration

- Modules are versioned container images plus their manifest. Registering a module adds it to the `connector_modules` table; creating an instance (`§05`) binds it to configuration, secrets, and routing.
- Upgrades are rolling: a new module version is registered, an instance is switched to it, health is confirmed, and the old version is retired. Rollback is switching the instance back.
- Source layout in the repository: `sdk/` for the SDK, `connectors/<type>/<code>/` for modules (e.g. `connectors/erpi/ifs/`).

### 8. Versioning and compatibility

- Each connector-type contract is versioned (`pm.connector.v1.Erpi` 1.x). Additive changes (new optional fields, new RPCs in a new tier) are minor versions; breaking changes require a new major version and a deprecation window in which pm-core serves both.
- A **compatibility matrix** (module version × contract version × pm-core version) is published with each release and checked at registration: pm-core refuses to enable a module whose declared contract range it does not serve.

## What a module author writes, and what the SDK provides

| The module author writes | The SDK provides |
|---|---|
| Vendor-specific calls (which endpoints, which projections, request shapes) | gRPC server, standard RPCs, tier declaration |
| Mapping YAML for mechanical field translation | Mapping engine, dry-run, UNTS hashing |
| Code for non-mechanical logic (e.g. reconciling a vendor's change-order states to pro_manage's) | Retries, backoff, circuit breaking, rate limits |
| Manifest values (tiers, config fields, secrets, network needs) | Token handling, idempotency, ETag concurrency, polling, inbound events |
| Recorded fixtures from the vendor sandbox | Conformance suite, mock pm-core, CI template |

## Security requirements every module meets (enforced by the SDK and the conformance kit)

- Least-privilege upstream credentials, declared in the manifest and held in the vault.
- Egress only to the hosts declared in `egress`, through the controlled proxy.
- Inbound endpoints only as declared, disabled by default, authenticated on every request, sender-allowlisted (`§11`).
- No personal or commercial data in logs or recorded fixtures.
- No direct database access to vendor systems unless the vendor documents it as a supported integration path.
- Writes only within the module's declared tiers; anything else is rejected by the runtime.

## Example: `ERPi_IFS` on the SDK

The interface described in `§10` maps onto SDK helpers almost entirely: OAuth 2.0 client credentials (OAuth client), OData projections and paging (OData helper), ETag writes (concurrency), idempotent `ReleaseBudget` keyed on the CAR id (idempotency), last-modified polling with a switch to IFS Connect push once approved (polling scheduler, inbound receiver, push/poll switch), and the field mapping between IFS objects and pro_manage entities (mapping layer). The module-specific work is identifying the Projects-module projections in the target environment, writing the mapping, and handling IFS-specific state reconciliation.

## Open items for Section 15

- Confirm whether third parties (vendors, integrators) may build modules against the SDK; if so, decide the signing and review process for externally built modules.
- Decide whether the Python runtime is built in Phase 0 or deferred until the first Python-only vendor appears. First instinct: defer, but keep the contract and conformance kit language-neutral.
- Decide the policy for recorded fixtures: who scrubs them, and where they are stored.
- Consider aligning the scaffold and validate commands with MDEMG's plugin tooling, which follows the same scaffold, validate, and register pattern.
