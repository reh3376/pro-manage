module github.com/reh3376/pro-manage/connectors/idpi/oidc

go 1.26.0

require (
	github.com/coreos/go-oidc/v3 v3.21.0
	github.com/reh3376/pro-manage/services/pm-core v0.0.0
	go.yaml.in/yaml/v3 v3.0.5
	golang.org/x/oauth2 v0.37.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261005182115-fad411399dd8 // indirect
)

// The generated contract (gen/pm/connector/v1) lives in the pm-core module until
// the Connector SDK is extracted (FSD §15, roadmap: after IdPi_OIDC and ERPi_IFS).
replace github.com/reh3376/pro-manage/services/pm-core => ../../../services/pm-core
