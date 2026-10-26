# connectors

Connector modules, one directory per `connectors/<type>/<code>/` (FSD §15 "Packaging and registration"; the module's name is `<Type>i_<CODE>`, e.g. `IdPi_OIDC`). Each module is a container that implements its type's gRPC contract from `proto/pm/connector/v1/` plus the common `pm.connector.v1.ConnectorService` (`HealthCheck`, `Capabilities`), carries a `connector.yaml` manifest whose capabilities are adjudicated in `docs/inventories/connector_capabilities_inventory.json`, and passes its UDTS conformance specs (`docs/tests/udts/specs/<module>_*.udts.json`) before it can be registered.

| Module | Status | Contract | Notes |
|---|---|---|---|
| `idpi/oidc` (`IdPi_OIDC`) | **built, slice 4a** | `pm.connector.v1.IdpiService` | Generic OpenID Connect against the dev Keycloak realm. Own Go module with a `replace` to `services/pm-core` for the generated contract. PKCE code flow in two RPCs, JWKS verification, refresh, logout with revocation, claims-to-roles mapping from `mapping.yaml`. pm-core's login, users, and sessions are slice 4b. |
| `sigi/stub` (`Sigi_STUB`) | **built, slice 6** | `pm.connector.v1.SigiService` | Development and CI e-signature backend: in-memory envelopes, `POST /dev/envelopes/{id}/sign` stands in for a signer, a minimal PDF with a SHA-256 as the artifact. pm-core's envelope lifecycle, reconciliation, and capture run against it unchanged. |
| `sigi/ds` (`Sigi_DS`) | planned (B-13) | `pm.connector.v1.SigiService` | DocuSign eSignature REST once the account exists. |
| `erpi/ifs` | planned | `pm.connector.v1.ErpiService` | Once the IFS items on the owner checklist are answered. |

Build order (roadmap, decided 2026-10-20): `IdPi_OIDC`, then `ERPi_IFS`; the Connector SDK (`sdk/`) is extracted from those two and `Sigi_DS` and `Commsi_SMTP` are the first modules built on it. Until then each module keeps a small `internal/runtime` (config, secrets, health listener) that the SDK will absorb.

## Running IdPi_OIDC

The compose dev stack starts it as `idpi-oidc` (gRPC `:7080`, HTTP `:7081`) against Keycloak with the confidential `pm-core` client. Keycloak advertises its browser-facing issuer (`http://localhost:8180/realms/northfield`) while in-network calls go through `http://keycloak:8080`, so the module takes `OIDC_ISSUER` (the `iss` tokens carry), `OIDC_DISCOVERY_URL`, and `OIDC_BACKCHANNEL_BASE` separately.

```bash
make test-udts        # UDTS specs against pm-core and against idpi-oidc
make test-idpi-live   # tokens, claims to roles, refresh, logout against the dev Keycloak (password grant via pm-dev-cli)
grpcurl -plaintext 127.0.0.1:7080 pm.connector.v1.ConnectorService/Capabilities
```
