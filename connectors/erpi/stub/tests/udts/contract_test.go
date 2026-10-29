// Package udts runs the ERPi_STUB UDTS conformance specs (docs/tests/udts/specs/erpi_stub_*)
// against a live module, in the same hand-coded-per-RPC style as the other
// modules (FSD §12, §15 conformance kit). Set UDTS_ERPI_TARGET (e.g. 127.0.0.1:7110).
// The proto hash pins run offline on every `go test`.
package udts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
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
		file := "pm/connector/v1/erpi.proto"
		if s.Service == "pm.connector.v1.ConnectorService" {
			file = "pm/connector/v1/connector.proto"
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
	target := os.Getenv("UDTS_ERPI_TARGET")
	if target == "" {
		t.Skip("UDTS_ERPI_TARGET not set (e.g. 127.0.0.1:7110)")
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

// schemaCode spells a gRPC code the way the UDTS schema does (OK, NOT_FOUND, ...).
func schemaCode(err error) string {
	switch status.Code(err) {
	case codes.OK:
		return "OK"
	case codes.InvalidArgument:
		return "INVALID_ARGUMENT"
	case codes.NotFound:
		return "NOT_FOUND"
	case codes.FailedPrecondition:
		return "FAILED_PRECONDITION"
	case codes.Unimplemented:
		return "UNIMPLEMENTED"
	case codes.Internal:
		return "INTERNAL"
	case codes.Unavailable:
		return "UNAVAILABLE"
	case codes.Unauthenticated:
		return "UNAUTHENTICATED"
	case codes.PermissionDenied:
		return "PERMISSION_DENIED"
	case codes.DeadlineExceeded:
		return "DEADLINE_EXCEEDED"
	default:
		return strings.ToUpper(status.Code(err).String())
	}
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

// first drains a server stream to its first message (the spec's field check runs on it)
// or to the error that opened it. A clean empty stream yields a nil message and nil error.
func first[M proto.Message](recv func() (M, error)) (proto.Message, error) {
	msg, err := recv()
	if errors.Is(err, io.EOF) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// Every spec runs offline for its proto pin and online for its RPC.

func TestConnectorHealthCheck(t *testing.T) {
	s := load(t, "erpi_stub_connector_healthcheck.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	resp, err := connectorv1.NewConnectorServiceClient(dial(t)).HealthCheck(ctx, &connectorv1.HealthCheckRequest{})
	check(t, s, resp, err)
}

func TestConnectorCapabilities(t *testing.T) {
	s := load(t, "erpi_stub_connector_capabilities.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	resp, err := connectorv1.NewConnectorServiceClient(dial(t)).Capabilities(ctx, &connectorv1.CapabilitiesRequest{})
	check(t, s, resp, err)
	if err == nil {
		if resp.GetModule() != "ERPi_STUB" || resp.GetContract() != "pm.connector.v1.ErpiService" {
			t.Fatalf("module %s contract %s", resp.GetModule(), resp.GetContract())
		}
		want := map[string]bool{"financials": true, "project_controls": true, "budget_release": true}
		for _, tier := range resp.GetTiers() {
			delete(want, tier)
		}
		if len(want) != 0 {
			t.Fatalf("tiers missing: %v (got %v)", want, resp.GetTiers())
		}
	}
}

func TestFetchProjectStructure(t *testing.T) {
	s := load(t, "erpi_stub_fetch_project_structure.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	var req connectorv1.FetchProjectStructureRequest
	unmarshal(t, s, &req)
	stream, err := connectorv1.NewErpiServiceClient(dial(t)).FetchProjectStructure(ctx, &req)
	if err != nil {
		check(t, s, nil, err)
		return
	}
	msg, err := first(stream.Recv)
	check(t, s, msg, err)
}

func TestFetchBudgetVersions(t *testing.T) {
	s := load(t, "erpi_stub_fetch_budget_versions.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	var req connectorv1.FetchBudgetVersionsRequest
	unmarshal(t, s, &req)
	stream, err := connectorv1.NewErpiServiceClient(dial(t)).FetchBudgetVersions(ctx, &req)
	if err != nil {
		check(t, s, nil, err)
		return
	}
	msg, err := first(stream.Recv)
	check(t, s, msg, err)
	if v := msg.(*connectorv1.FetchBudgetVersionsResponse).GetVersion(); v.GetTotal() == "" || v.GetCurrency() == "" {
		t.Fatalf("version lacks total or currency: %v", v)
	}
}

func TestFetchBudgetVersionsUnknownProject(t *testing.T) {
	s := load(t, "erpi_stub_fetch_budget_versions_unknown_project.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	var req connectorv1.FetchBudgetVersionsRequest
	unmarshal(t, s, &req)
	stream, err := connectorv1.NewErpiServiceClient(dial(t)).FetchBudgetVersions(ctx, &req)
	if err == nil {
		_, err = first(stream.Recv)
	}
	check(t, s, nil, err)
}

func TestFetchForecastVersions(t *testing.T) {
	s := load(t, "erpi_stub_fetch_forecast_versions.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	var req connectorv1.FetchForecastVersionsRequest
	unmarshal(t, s, &req)
	stream, err := connectorv1.NewErpiServiceClient(dial(t)).FetchForecastVersions(ctx, &req)
	if err != nil {
		check(t, s, nil, err)
		return
	}
	msg, err := first(stream.Recv)
	check(t, s, msg, err)
}

func TestFetchEac(t *testing.T) {
	s := load(t, "erpi_stub_fetch_eac.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	var req connectorv1.FetchEacRequest
	unmarshal(t, s, &req)
	resp, err := connectorv1.NewErpiServiceClient(dial(t)).FetchEac(ctx, &req)
	check(t, s, resp, err)
}

func TestFetchActuals(t *testing.T) {
	s := load(t, "erpi_stub_fetch_actuals.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	var req connectorv1.FetchActualsRequest
	unmarshal(t, s, &req)
	stream, err := connectorv1.NewErpiServiceClient(dial(t)).FetchActuals(ctx, &req)
	if err != nil {
		check(t, s, nil, err)
		return
	}
	msg, err := first(stream.Recv)
	check(t, s, msg, err)
	if a := msg.(*connectorv1.FetchActualsResponse).GetActual(); a.GetActualRef() == "" || a.GetAmount() == "" {
		t.Fatalf("actual lacks ref or amount: %v", a)
	}
}

func TestFetchCommitments(t *testing.T) {
	s := load(t, "erpi_stub_fetch_commitments.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	var req connectorv1.FetchCommitmentsRequest
	unmarshal(t, s, &req)
	stream, err := connectorv1.NewErpiServiceClient(dial(t)).FetchCommitments(ctx, &req)
	if err != nil {
		check(t, s, nil, err)
		return
	}
	msg, err := first(stream.Recv)
	check(t, s, msg, err)
}

func TestReleaseBudget(t *testing.T) {
	s := load(t, "erpi_stub_release_budget.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	var req connectorv1.ReleaseBudgetRequest
	unmarshal(t, s, &req)
	c := connectorv1.NewErpiServiceClient(dial(t))
	resp, err := c.ReleaseBudget(ctx, &req)
	check(t, s, resp, err)
	if err != nil {
		return
	}
	again, err := c.ReleaseBudget(ctx, &req)
	if err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if again.GetBudgetRef() != resp.GetBudgetRef() || again.GetStatus() != connectorv1.ReleaseStatus_RELEASE_STATUS_ALREADY_RELEASED {
		t.Fatalf("repeat must return the same budget_ref as ALREADY_RELEASED: first %s/%s, again %s/%s", resp.GetBudgetRef(), resp.GetStatus(), again.GetBudgetRef(), again.GetStatus())
	}
}

func TestReleaseBudgetMissingCar(t *testing.T) {
	s := load(t, "erpi_stub_release_budget_missing_car.udts.json")
	ctx, cancel := ctxFor(s)
	defer cancel()
	var req connectorv1.ReleaseBudgetRequest
	unmarshal(t, s, &req)
	resp, err := connectorv1.NewErpiServiceClient(dial(t)).ReleaseBudget(ctx, &req)
	check(t, s, resp, err)
}
