# proto

Protobuf contracts for pro_manage's internal gRPC surfaces (FSD §05): `pm.core.v1`, `pm.sidecar.v1`, and one `pm.connector.v1.<type>` per connector type. Managed with [buf](https://buf.build).

```bash
make proto        # lint + generate Go (services/pm-core/gen) and Python (services/pm-sidecar/src/pm_sidecar/gen)
make proto-lint
make proto-breaking   # against main
```

Every RPC gets a UDTS spec (FSD §12) and the connector-type contracts are packaged as the Connector SDK conformance suite (FSD §15).
