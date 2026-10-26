# proto

Protobuf contracts for pro_manage's internal gRPC surfaces (FSD §05): `pm.core.v1` (`VersionService` today), `pm.sidecar.v1` (arrives with B-05), and `pm.connector.v1` (the common `ConnectorService` plus one service per connector type: `IdpiService`, `SigiService`, and `CommsiService` today, `ErpiService` next). Managed with [buf](https://buf.build); message names follow buf's standard (`<Rpc>Request` / `<Rpc>Response`). Generated Go lives in `services/pm-core/gen` and the connector modules import it through a `replace` until the Connector SDK is extracted (FSD §15).

```bash
make proto        # lint + generate Go (services/pm-core/gen) and Python (services/pm-sidecar/src/pm_sidecar/gen)
make proto-lint
make proto-breaking   # against main
```

Every RPC gets a UDTS spec (FSD §12) and the connector-type contracts are packaged as the Connector SDK conformance suite (FSD §15).
