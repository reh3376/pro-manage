# connectors

Connector modules, one directory per `<Type>i_<CODE>` (FSD §10 naming, §15 SDK). Each module is a container that implements its type's gRPC contract from `proto/pm/connector/v1/` and passes that type's UDTS conformance suite before it can be registered.

Build order (roadmap, decided 2026-10-20):

1. `IdPi_OIDC` against the dev Keycloak realm, built plainly against the contract (spine).
2. `ERPi_IFS` once the IFS items on the owner checklist are answered.
3. The Connector SDK is extracted from those two; `Sigi_DS` and `Commsi_SMTP` are the first modules built on it.

Nothing lives here yet beyond this note; the contract arrives with slice 1.
