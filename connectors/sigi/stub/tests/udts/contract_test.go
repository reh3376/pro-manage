// Package udts runs the Sigi_STUB UDTS conformance specs (docs/tests/udts/specs/sigi_stub_*)
// against a live module, in the same hand-coded-per-RPC style as pm-core's
// (FSD §12, §15 conformance kit). Set UDTS_SIGI_TARGET (e.g. 127.0.0.1:7090).
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
		file := "pm/connector/v1/sigi.proto" // SigiService specs pin the type contract ...
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
	target := os.Getenv("UDTS_SIGI_TARGET")
	if target == "" {
		t.Skip("UDTS_SIGI_TARGET not set (e.g. 127.0.0.1:7090)")
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
	s := load(t, "sigi_stub_connector_healthcheck.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	resp, err := connectorv1.NewConnectorServiceClient(dial(t)).HealthCheck(ctx, &connectorv1.HealthCheckRequest{})
	check(t, s, resp, err)
}

func TestConnectorCapabilities(t *testing.T) {
	s := load(t, "sigi_stub_connector_capabilities.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	resp, err := connectorv1.NewConnectorServiceClient(dial(t)).Capabilities(ctx, &connectorv1.CapabilitiesRequest{})
	check(t, s, resp, err)
	if err == nil && resp.GetModule() != "Sigi_STUB" {
		t.Fatalf("module: %s", resp.GetModule())
	}
}

func TestCreateEnvelope(t *testing.T) {
	s := load(t, "sigi_stub_create_envelope.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.CreateEnvelopeRequest{}
	unmarshal(t, s, req)
	c := connectorv1.NewSigiServiceClient(dial(t))
	resp, err := c.CreateEnvelope(ctx, req)
	check(t, s, resp, err)
	if err == nil {
		again, err := c.CreateEnvelope(ctx, req)
		if err != nil || again.GetEnvelopeId() != resp.GetEnvelopeId() {
			t.Fatalf("idempotency: %v %s vs %s", err, again.GetEnvelopeId(), resp.GetEnvelopeId())
		}
	}
}

func TestCreateEnvelopeMissingSubject(t *testing.T) {
	s := load(t, "sigi_stub_create_envelope_missing_subject.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.CreateEnvelopeRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewSigiServiceClient(dial(t)).CreateEnvelope(ctx, req)
	check(t, s, resp, err)
}

func TestGetEnvelopeStatusUnknown(t *testing.T) {
	s := load(t, "sigi_stub_get_envelope_status_unknown.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.GetEnvelopeStatusRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewSigiServiceClient(dial(t)).GetEnvelopeStatus(ctx, req)
	check(t, s, resp, err)
}

func TestCancelEnvelopeUnknown(t *testing.T) {
	s := load(t, "sigi_stub_cancel_envelope_unknown.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.CancelEnvelopeRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewSigiServiceClient(dial(t)).CancelEnvelope(ctx, req)
	check(t, s, resp, err)
}

func TestDownloadSignedArtifactUnknown(t *testing.T) {
	s := load(t, "sigi_stub_download_signed_artifact_unknown.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.DownloadSignedArtifactRequest{}
	unmarshal(t, s, req)
	stream, err := connectorv1.NewSigiServiceClient(dial(t)).DownloadSignedArtifact(ctx, req)
	if err == nil {
		_, err = stream.Recv()
	}
	check(t, s, nil, err)
}

func TestOnWebhookBadSignature(t *testing.T) {
	s := load(t, "sigi_stub_on_webhook_bad_signature.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	req := &connectorv1.OnWebhookRequest{}
	unmarshal(t, s, req)
	resp, err := connectorv1.NewSigiServiceClient(dial(t)).OnWebhook(ctx, req)
	check(t, s, resp, err)
	if err == nil && resp.GetAccepted() {
		t.Fatal("a bad signature must not be accepted")
	}
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
