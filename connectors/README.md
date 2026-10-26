# connectors

Connector modules, one directory per `connectors/<type>/<code>/` (FSD §15 "Packaging and registration"; the module's name is `<Type>i_<CODE>`, e.g. `IdPi_OIDC`). Each module is a container that implements its type's gRPC contract from `proto/pm/connector/v1/` plus the common `pm.connector.v1.ConnectorService` (`HealthCheck`, `Capabilities`), carries a `connector.yaml` manifest whose capabilities are adjudicated in `docs/inventories/connector_capabilities_inventory.json`, and passes its UDTS conformance specs (`docs/tests/udts/specs/<module>_*.udts.json`) before it can be registered.

| Module | Status | Contract | Notes |
|---|---|---|---|
| `idpi/oidc` (`IdPi_OIDC`) | **built, slice 4a** | `pm.connector.v1.IdpiService` | Generic OpenID Connect against the dev Keycloak realm. Own Go module with a `replace` to `services/pm-core` for the generated contract. PKCE code flow in two RPCs, JWKS verification, refresh, logout with revocation, claims-to-roles mapping from `mapping.yaml`. pm-core's login, users, and sessions are slice 4b. |
| `sigi/stub` (`Sigi_STUB`) | **built, slice 6** | `pm.connector.v1.SigiService` | Development and CI e-signature backend: in-memory envelopes, `POST /dev/envelopes/{id}/sign` stands in for a signer, a minimal PDF with a SHA-256 as the artifact. pm-core's envelope lifecycle, reconciliation, and capture run against it unchanged. |
| `commsi/smtp` (`Commsi_SMTP`) | **built, slice 7** | `pm.connector.v1.CommsiService` | Notifications as email over SMTP (Mailpit in dev): plain or STARTTLS, PLAIN auth, a dedupe window per key, deep links prefixed with the frontend URL. pm-workers delivers the outbox's `commsi.send` rows through it. |
| `sigi/ds` (`Sigi_DS`) | planned (B-13) | `pm.connector.v1.SigiService` | DocuSign eSignature REST once the account exists. |
| `erpi/ifs` | planned | `pm.connector.v1.ErpiService` | Once the IFS items on the owner checklist are answered. |

Build order (roadmap, decided 2026-10-20): `IdPi_OIDC`, then `ERPi_IFS`; the Connector SDK (`sdk/`) is extracted from those two and `Sigi_DS` and `Commsi_SMTP` are the first modules built on it. `Sigi_STUB` (slice 6) and `Commsi_SMTP` (slice 7) came earlier than planned because the spine's signed artifact and nudges needed backends; both are deliberately small and will be re-based on the SDK. Until then each module keeps its own process plumbing (config, secrets, health listener) that the SDK will absorb.

## Running Sigi_STUB

The compose dev stack starts it as `sigi-stub` (gRPC `:7090`, HTTP `:7091`). pm-workers creates envelopes there when a CAR at or above `car.esign.required_above` is approved; `POST http://127.0.0.1:7091/dev/envelopes/{provider_ref}/sign?user_id=<signer>` stands in for each signer (`decline=1` declines), and pm-workers captures the resulting PDF to RustFS. `make check-car` drives the whole lifecycle.

## Running IdPi_OIDC

The compose dev stack starts it as `idpi-oidc` (gRPC `:7080`, HTTP `:7081`) against Keycloak with the confidential `pm-core` client. Keycloak advertises its browser-facing issuer (`http://localhost:8180/realms/northfield`) while in-network calls go through `http://keycloak:8080`, so the module takes `OIDC_ISSUER` (the `iss` tokens carry), `OIDC_DISCOVERY_URL`, and `OIDC_BACKCHANNEL_BASE` separately.

```bash
make test-udts        # UDTS specs against pm-core and against idpi-oidc
make test-idpi-live   # tokens, claims to roles, refresh, logout against the dev Keycloak (password grant via pm-dev-cli)
grpcurl -plaintext 127.0.0.1:7080 pm.connector.v1.ConnectorService/Capabilities
```

## Running Commsi_SMTP

The compose dev stack starts it as `commsi-smtp` (gRPC `:7100`, HTTP `:7101`) against Mailpit (`SMTP_HOST=mailpit`, port 1025, no TLS). Every notification pm-core queues (approval requests, outcomes, signature requests, nudges, overdue, escalation, expiry) lands in Mailpit's UI at `http://localhost:8026`. In CI the dedupe window is shortened to 2 s (`.env.ci`) so the CAR check's repeat kinds stay distinct.
