// Package udts runs the IdPi_OIDC UDTS conformance specs (docs/tests/udts/specs/idpi_oidc_*)
// against a live module, in the same hand-coded-per-RPC style as pm-core's
// (FSD §12, §15 conformance kit). Set UDTS_IDPI_TARGET (e.g. 127.0.0.1:7080).
// The proto hash pins run offline on every `go test`.
package udts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

type spec struct {
	UDTSVersion string          `json:"udts_version"`
	Service     string          `json:"service"`
	Method      string          `json:"method"`
	Request     json.RawMessage `json:"request"`
	Expected    struct {
		StatusCode string `json:"status_code"`
		HasField   string `json:"response_has_field"`
	} `json:"expected"`
	Config struct {
		TimeoutMs   int    `json:"timeout_ms"`
		ProtoSHA256 string `json:"proto_sha256"`
	} `json:"config"`
}

var repoRoot = filepath.Join("..", "..", "..", "..", "..")

func load(t *testing.T, name string) spec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "tests", "udts", "specs", name))
	if err != nil {
		t.Fatalf("read spec %s: %v", name, err)
	}
	var s spec
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse spec %s: %v", name, err)
	}
	if s.Config.ProtoSHA256 != "" {
		file := "pm/connector/v1/idpi.proto" // IdpiService specs pin the type contract ...
		if s.Service == "pm.connector.v1.ConnectorService" {
			file = "pm/connector/v1/connector.proto" // ... and ConnectorService specs the common one
		}
		b, err := os.ReadFile(filepath.Join(repoRoot, "proto", file))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if got := hex.EncodeToString(sum[:]); got != s.Config.ProtoSHA256 {
			t.Fatalf("proto %s changed: spec %s pins %s, file is %s", file, name, s.Config.ProtoSHA256, got)
		}
	}
	return s
}

func dial(t *testing.T) *grpc.ClientConn {
	t.Helper()
	target := os.Getenv("UDTS_IDPI_TARGET")
	if target == "" {
		t.Skip("UDTS_IDPI_TARGET not set (e.g. 127.0.0.1:7080)")
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func ctxFor(s spec) (context.Context, context.CancelFunc) {
	d := 5 * time.Second
	if s.Config.TimeoutMs > 0 {
		d = time.Duration(s.Config.TimeoutMs) * time.Millisecond
	}
	return context.WithTimeout(context.Background(), d)
}

func check(t *testing.T, s spec, resp proto.Message, err error) {
	t.Helper()
	if got := schemaCode(err); got != s.Expected.StatusCode {
		t.Fatalf("status: want %s, got %s (%v)", s.Expected.StatusCode, got, err)
	}
	if err != nil || s.Expected.HasField == "" {
		return
	}
	b, _ := protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true}.Marshal(resp)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if _, ok := m[s.Expected.HasField]; !ok {
		t.Fatalf("response lacks field %q: %s", s.Expected.HasField, b)
	}
}

func unmarshal(t *testing.T, s spec, m proto.Message) {
	t.Helper()
	if len(s.Request) == 0 {
		return
	}
	if err := protojson.Unmarshal(s.Request, m); err != nil {
		t.Fatalf("request: %v", err)
	}
}

// Every spec runs offline for its proto pin and online for its RPC.

func TestConnectorHealthCheck(t *testing.T) {
	s := load(t, "idpi_oidc_connector_healthcheck.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	resp, err := connectorv1.NewConnectorServiceClient(dial(t)).HealthCheck(ctx, &connectorv1.HealthCheckRequest{})
	check(t, s, resp, err)
}

func TestConnectorCapabilities(t *testing.T) {
	s := load(t, "idpi_oidc_connector_capabilities.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	resp, err := connectorv1.NewConnectorServiceClient(dial(t)).Capabilities(ctx, &connectorv1.CapabilitiesRequest{})
	check(t, s, resp, err)
	if err == nil && resp.GetModule() != "IdPi_OIDC" {
		t.Fatalf("module: %s", resp.GetModule())
	}
}

func TestAuthenticateInteractive(t *testing.T) {
	s := load(t, "idpi_oidc_authenticate_interactive.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.AuthenticateInteractiveRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewIdpiServiceClient(dial(t)).AuthenticateInteractive(ctx, req)
	check(t, s, resp, err)
}

func TestCompleteAuthenticationBadCode(t *testing.T) {
	s := load(t, "idpi_oidc_complete_authentication_bad_code.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.CompleteAuthenticationRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewIdpiServiceClient(dial(t)).CompleteAuthentication(ctx, req)
	check(t, s, resp, err)
}

func TestValidateTokenGarbage(t *testing.T) {
	s := load(t, "idpi_oidc_validate_token_garbage.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.ValidateTokenRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewIdpiServiceClient(dial(t)).ValidateToken(ctx, req)
	check(t, s, resp, err)
	if err == nil && resp.GetValid() {
		t.Fatal("garbage token must not validate")
	}
}

func TestRefreshTokenGarbage(t *testing.T) {
	s := load(t, "idpi_oidc_refresh_token_garbage.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.RefreshTokenRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewIdpiServiceClient(dial(t)).RefreshToken(ctx, req)
	check(t, s, resp, err)
}

func TestResolveClaimsToRoles(t *testing.T) {
	s := load(t, "idpi_oidc_resolve_claims_to_roles.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.ResolveClaimsToRolesRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewIdpiServiceClient(dial(t)).ResolveClaimsToRoles(ctx, req)
	check(t, s, resp, err)
	if err == nil && len(resp.GetPlatformRoles()) == 0 {
		t.Fatalf("expected a platform role for the spec's groups: %+v", resp)
	}
}

func TestLogoutWithoutTokens(t *testing.T) {
	s := load(t, "idpi_oidc_logout.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.LogoutRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewIdpiServiceClient(dial(t)).Logout(ctx, req)
	check(t, s, resp, err)
}

// schemaCode renders a gRPC code the way the UDTS schema spells it (OK, INVALID_ARGUMENT, UNAVAILABLE, ...).
func schemaCode(err error) string {
	switch status.Code(err) {
	case codes.OK:
		return "OK"
	case codes.InvalidArgument:
		return "INVALID_ARGUMENT"
	case codes.NotFound:
		return "NOT_FOUND"
	case codes.Internal:
		return "INTERNAL"
	case codes.Unavailable:
		return "UNAVAILABLE"
	case codes.Unauthenticated:
		return "UNAUTHENTICATED"
	case codes.PermissionDenied:
		return "PERMISSION_DENIED"
	default:
		return strings.ToUpper(status.Code(err).String())
	}
}
