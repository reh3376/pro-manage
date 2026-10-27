module github.com/reh3376/pro-manage/connectors/erpi/stub

go 1.26.0

require (
	github.com/reh3376/pro-manage/services/pm-core v0.0.0
	google.golang.org/grpc v1.84.0
	google.golang.org/protobuf v1.36.12
)

require (
	golang.org/x/net v0.61.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261005182115-fad411399dd8 // indirect
)

// The generated contract (gen/pm/connector/v1) lives in the pm-core module until
// the Connector SDK is extracted (FSD §15).
replace github.com/reh3376/pro-manage/services/pm-core => ../../../services/pm-core
