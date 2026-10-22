// Package udts runs pro_manage's UDTS contract specs (docs/tests/udts/specs)
// against a live pm-core gRPC endpoint, in MDEMG's hand-coded-per-RPC style
// (FSD §12, §15 conformance kit). Set UDTS_TARGET (e.g. 127.0.0.1:8090) to run;
// the tests skip when it is unset so `go test ./...` stays green offline.
//
// Each spec's config.proto_sha256, when present, must match the proto file it
// pins; a proto change without a spec update fails here before CI even dials.
package udts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	corev1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/core/v1"
)

// Spec is the UDTS spec shape (docs/tests/udts/schema/udts.schema.json).
type Spec struct {
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

var repoRoot = filepath.Join("..", "..", "..", "..")

func loadSpec(t *testing.T, name string) Spec {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "tests", "udts", "specs", name))
	if err != nil {
		t.Fatalf("read spec %s: %v", name, err)
	}
	var s Spec
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("parse spec %s: %v", name, err)
	}
	if s.UDTSVersion != "1.0.0" {
		t.Fatalf("spec %s: unsupported udts_version %q", name, s.UDTSVersion)
	}
	return s
}

// verifyProtoHash runs offline: the pinned proto must be unchanged.
func verifyProtoHash(t *testing.T, s Spec, protoRel string) {
	t.Helper()
	if s.Config.ProtoSHA256 == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, "proto", protoRel))
	if err != nil {
		t.Fatalf("read proto: %v", err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != s.Config.ProtoSHA256 {
		t.Fatalf("proto %s changed: spec pins %s, file is %s (update the spec and re-run make uxts-add-hashes)", protoRel, s.Config.ProtoSHA256, got)
	}
}

func dial(t *testing.T) *grpc.ClientConn {
	t.Helper()
	target := os.Getenv("UDTS_TARGET")
	if target == "" {
		t.Skip("UDTS_TARGET not set (e.g. 127.0.0.1:8090)")
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial %s: %v", target, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func timeout(s Spec) time.Duration {
	if s.Config.TimeoutMs > 0 {
		return time.Duration(s.Config.TimeoutMs) * time.Millisecond
	}
	return 5 * time.Second
}

func assertStatus(t *testing.T, s Spec, err error) {
	t.Helper()
	got := status.Code(err).String()
	if got != s.Expected.StatusCode {
		t.Fatalf("status: want %s, got %s (%v)", s.Expected.StatusCode, got, err)
	}
}

// hasField checks response_has_field against the JSON form of the response.
func hasField(t *testing.T, s Spec, resp any) {
	t.Helper()
	if s.Expected.HasField == "" {
		return
	}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m[s.Expected.HasField]; !ok {
		t.Fatalf("response lacks field %q: %s", s.Expected.HasField, b)
	}
}

func TestVersionServiceGetVersion(t *testing.T) {
	s := loadSpec(t, "pm_core_version_get.udts.json")
	verifyProtoHash(t, s, filepath.Join("pm", "core", "v1", "version.proto"))
	if s.Service != "pm.core.v1.VersionService" || s.Method != "GetVersion" {
		t.Fatalf("spec targets %s/%s, test expects VersionService/GetVersion", s.Service, s.Method)
	}
	conn := dial(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout(s))
	defer cancel()
	resp, err := corev1.NewVersionServiceClient(conn).GetVersion(ctx, &corev1.GetVersionRequest{})
	assertStatus(t, s, err)
	hasField(t, s, map[string]any{"service": resp.GetService(), "version": resp.GetVersion(), "git_sha": resp.GetGitSha()})
	if resp.GetService() != "pm-core" {
		t.Fatalf("service: want pm-core, got %q", resp.GetService())
	}
}

func TestGrpcHealthCheck(t *testing.T) {
	s := loadSpec(t, "pm_core_grpc_health.udts.json")
	if s.Service != "grpc.health.v1.Health" || s.Method != "Check" {
		t.Fatalf("spec targets %s/%s, test expects Health/Check", s.Service, s.Method)
	}
	conn := dial(t)
	ctx, cancel := context.WithTimeout(context.Background(), timeout(s))
	defer cancel()
	var req healthpb.HealthCheckRequest
	if len(s.Request) > 0 {
		_ = json.Unmarshal(s.Request, &struct {
			Service *string `json:"service"`
		}{Service: &req.Service})
	}
	resp, err := healthpb.NewHealthClient(conn).Check(ctx, &req)
	assertStatus(t, s, err)
	hasField(t, s, map[string]any{"status": resp.GetStatus().String()})
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("health: want SERVING, got %s", resp.GetStatus())
	}
}
