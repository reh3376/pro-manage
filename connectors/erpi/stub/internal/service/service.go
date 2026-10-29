// Package service implements pm.connector.v1.ConnectorService and the spine
// subset of pm.connector.v1.ErpiService over the in-memory ledger, plus the dev
// HTTP endpoints tests use to add projects, post actuals, and read releases.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/reh3376/pro-manage/connectors/erpi/stub/internal/ledger"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Manifest mirrors connector.yaml.
type Manifest struct {
	Module, Type, Code, Version, Contract string
	Capabilities                          []string
	Tiers                                 []string
}

// DefaultManifest is the stub's: every spine tier declared.
var DefaultManifest = Manifest{Module: "ERPi_STUB", Type: "ERPi", Code: "STUB", Contract: "pm.connector.v1.ErpiService",
	Capabilities: []string{"project_structure", "budget_versions", "forecast_versions", "eac", "actuals", "commitments", "budget_release", "dev_ledger"},
	Tiers:        []string{"financials", "project_controls", "budget_release"}}

// Service holds the ledger.
type Service struct {
	connectorv1.UnimplementedConnectorServiceServer
	connectorv1.UnimplementedErpiServiceServer
	Ledger   *ledger.Ledger
	Manifest Manifest
	Log      *slog.Logger
}

func (s *Service) tier(name string) bool {
	for _, t := range s.Manifest.Tiers {
		if t == name {
			return true
		}
	}
	return false
}

func (s *Service) need(tier string) error {
	if !s.tier(tier) {
		return status.Errorf(codes.FailedPrecondition, "tier %s not declared by this instance", tier)
	}
	return nil
}

// ---------------------------------------------------------------- ConnectorService

func (s *Service) HealthCheck(_ context.Context, _ *connectorv1.HealthCheckRequest) (*connectorv1.HealthCheckResponse, error) {
	return &connectorv1.HealthCheckResponse{Status: connectorv1.HealthCheckResponse_STATUS_SERVING, Detail: "in-memory ledger", Checks: map[string]string{"ledger": "ok"}}, nil
}

func (s *Service) Capabilities(_ context.Context, _ *connectorv1.CapabilitiesRequest) (*connectorv1.CapabilitiesResponse, error) {
	m := s.Manifest
	return &connectorv1.CapabilitiesResponse{Module: m.Module, Type: m.Type, Code: m.Code, Version: m.Version, Contract: m.Contract, Capabilities: m.Capabilities, Tiers: m.Tiers}, nil
}

// ---------------------------------------------------------------- ErpiService

func (s *Service) project(ref string) (ledger.Project, error) {
	if ref == "" {
		return ledger.Project{}, status.Error(codes.InvalidArgument, "project_ref is required")
	}
	p, err := s.Ledger.Get(ref)
	if errors.Is(err, ledger.ErrNotFound) {
		return ledger.Project{}, status.Errorf(codes.NotFound, "project %s not found", ref)
	}
	return p, err
}

func (s *Service) FetchProjectStructure(req *connectorv1.FetchProjectStructureRequest, stream connectorv1.ErpiService_FetchProjectStructureServer) error {
	if err := s.need("project_controls"); err != nil {
		return err
	}
	p, err := s.project(req.GetProjectRef())
	if err != nil {
		return err
	}
	for _, n := range p.Nodes {
		if err := stream.Send(&connectorv1.FetchProjectStructureResponse{Node: &connectorv1.ProjectNode{NodeRef: n.Ref, ParentRef: n.ParentRef, Kind: n.Kind, Code: n.Code, Name: n.Name, Status: n.Status,
			PlannedStart: ts(n.PlannedStart), PlannedFinish: ts(n.PlannedFinish), Attributes: n.Attributes}}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) FetchBudgetVersions(req *connectorv1.FetchBudgetVersionsRequest, stream connectorv1.ErpiService_FetchBudgetVersionsServer) error {
	if err := s.need("project_controls"); err != nil {
		return err
	}
	p, err := s.project(req.GetProjectRef())
	if err != nil {
		return err
	}
	for _, v := range p.Baselines {
		if since := req.GetSince(); since != nil && since.IsValid() && !v.AsOf.After(since.AsTime()) {
			continue
		}
		if err := stream.Send(&connectorv1.FetchBudgetVersionsResponse{Version: version(p.Ref, v)}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) FetchForecastVersions(req *connectorv1.FetchForecastVersionsRequest, stream connectorv1.ErpiService_FetchForecastVersionsServer) error {
	if err := s.need("project_controls"); err != nil {
		return err
	}
	p, err := s.project(req.GetProjectRef())
	if err != nil {
		return err
	}
	for _, v := range p.Forecasts {
		if since := req.GetSince(); since != nil && since.IsValid() && !v.AsOf.After(since.AsTime()) {
			continue
		}
		if err := stream.Send(&connectorv1.FetchForecastVersionsResponse{Version: version(p.Ref, v)}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) FetchEac(_ context.Context, req *connectorv1.FetchEacRequest) (*connectorv1.FetchEacResponse, error) {
	if err := s.need("project_controls"); err != nil {
		return nil, err
	}
	p, err := s.project(req.GetProjectRef())
	if err != nil {
		return nil, err
	}
	if p.Eac == nil {
		return nil, status.Errorf(codes.NotFound, "project %s has no EAC", p.Ref)
	}
	return &connectorv1.FetchEacResponse{ProjectRef: p.Ref, AsOf: ts(p.Eac.AsOf), EacAmount: p.Eac.Amount, Currency: p.Eac.Currency, Method: p.Eac.Method, Lines: lines(p.Eac.Lines)}, nil
}

func (s *Service) FetchActuals(req *connectorv1.FetchActualsRequest, stream connectorv1.ErpiService_FetchActualsServer) error {
	if err := s.need("financials"); err != nil {
		return err
	}
	p, err := s.project(req.GetProjectRef())
	if err != nil {
		return err
	}
	for _, a := range p.Actuals {
		if from := req.GetFromTs(); from != nil && from.IsValid() && a.PostedAt.Before(from.AsTime()) {
			continue
		}
		if to := req.GetToTs(); to != nil && to.IsValid() && !a.PostedAt.Before(to.AsTime()) {
			continue
		}
		if err := stream.Send(&connectorv1.FetchActualsResponse{Actual: &connectorv1.Actual{ActualRef: a.Ref, ProjectRef: p.Ref, WbsPath: a.WbsPath, AccountCode: a.AccountCode, Period: a.Period,
			Amount: a.Amount, Currency: a.Currency, SourceDocRef: a.SourceDocRef, PostedAt: ts(a.PostedAt)}}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) FetchCommitments(req *connectorv1.FetchCommitmentsRequest, stream connectorv1.ErpiService_FetchCommitmentsServer) error {
	if err := s.need("financials"); err != nil {
		return err
	}
	p, err := s.project(req.GetProjectRef())
	if err != nil {
		return err
	}
	for _, c := range p.Commitments {
		if c.Status == "closed" && !req.GetIncludeClosed() {
			continue
		}
		if err := stream.Send(&connectorv1.FetchCommitmentsResponse{Commitment: &connectorv1.Commitment{CommitmentRef: c.Ref, ProjectRef: p.Ref, WbsPath: c.WbsPath, AccountCode: c.AccountCode,
			VendorRef: c.VendorRef, PoRef: c.PORef, Amount: c.Amount, InvoicedToDate: c.InvoicedToDate, Currency: c.Currency, Status: c.Status, CreatedAt: ts(c.CreatedAt)}}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ReleaseBudget(_ context.Context, req *connectorv1.ReleaseBudgetRequest) (*connectorv1.ReleaseBudgetResponse, error) {
	if err := s.need("budget_release"); err != nil {
		return nil, err
	}
	if req.GetCarId() == "" || req.GetProjectRef() == "" || req.GetAmount() == "" {
		return nil, status.Error(codes.InvalidArgument, "car_id, project_ref, and amount are required")
	}
	r, repeat, err := s.Ledger.Release(ledger.Release{CARID: req.GetCarId(), ProjectRef: req.GetProjectRef(), Amount: req.GetAmount(), Currency: req.GetCurrency(),
		ApprovalRef: req.GetApprovalRef(), PlanLineRef: req.GetPlanLineRef(), Description: req.GetDescription(), ReleasedBy: req.GetReleasedBy()})
	switch {
	case errors.Is(err, ledger.ErrNotFound):
		return nil, status.Errorf(codes.NotFound, "project %s not found", req.GetProjectRef())
	case errors.Is(err, ledger.ErrInvalid):
		return nil, status.Error(codes.InvalidArgument, err.Error())
	case err != nil:
		return nil, err
	}
	st := connectorv1.ReleaseStatus_RELEASE_STATUS_RELEASED
	detail := "released in the stub ledger as an approved budget version"
	if repeat {
		st = connectorv1.ReleaseStatus_RELEASE_STATUS_ALREADY_RELEASED
		detail = "car_id already released; returning the earlier budget reference"
	} else {
		s.Log.Info("budget released", "budget_ref", r.BudgetRef, "car_id", r.CARID, "project_ref", r.ProjectRef, "amount", r.Amount, "currency", r.Currency)
	}
	return &connectorv1.ReleaseBudgetResponse{BudgetRef: r.BudgetRef, Status: st, ReleasedAt: ts(r.ReleasedAt), Detail: detail}, nil
}

// ---------------------------------------------------------------- dev HTTP

// DevMux serves the stand-in ERP back office: list projects, add one, post an
// actual, read the releases. Dev and CI only (never deployed).
func (s *Service) DevMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /dev/projects", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"projects": s.Ledger.Refs()})
	})
	mux.HandleFunc("POST /dev/projects", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Ref, Code, Name, Currency, Status string
			Baseline                          string `json:"baseline"` // optional approved baseline total
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Ref == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ref required"})
			return
		}
		if in.Code == "" {
			in.Code = in.Ref
		}
		if in.Currency == "" {
			in.Currency = "USD"
		}
		p := ledger.Project{Ref: in.Ref, Code: in.Code, Name: in.Name, Currency: in.Currency, Status: in.Status,
			Nodes: []ledger.Node{{Ref: in.Ref, Kind: "project", Code: in.Code, Name: in.Name, Status: in.Status}}}
		if in.Baseline != "" {
			now := time.Now().UTC()
			p.Baselines = []ledger.Version{{Ref: "BL-1", Name: "Baseline 1", Status: "approved", Total: in.Baseline, Currency: in.Currency, AsOf: now, ApprovedAt: now}}
		}
		if err := s.Ledger.Add(p); err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, ledger.ErrExists) {
				code = http.StatusConflict
			}
			writeJSON(w, code, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"ref": p.Ref})
	})
	mux.HandleFunc("GET /dev/projects/{ref}", func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Ledger.Get(r.PathValue("ref"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ref": p.Ref, "code": p.Code, "name": p.Name, "currency": p.Currency, "status": p.Status,
			"baselines": len(p.Baselines), "forecasts": len(p.Forecasts), "actuals": len(p.Actuals), "commitments": len(p.Commitments), "eac": p.Eac != nil})
	})
	mux.HandleFunc("POST /dev/projects/{ref}/actuals", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Ref, WbsPath, AccountCode, Period, Amount, Currency, SourceDocRef string
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		a, err := s.Ledger.PostActual(r.PathValue("ref"), ledger.Actual{Ref: in.Ref, WbsPath: in.WbsPath, AccountCode: in.AccountCode, Period: in.Period, Amount: in.Amount, Currency: in.Currency, SourceDocRef: in.SourceDocRef})
		if err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, ledger.ErrNotFound) {
				code = http.StatusNotFound
			}
			writeJSON(w, code, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"actual_ref": a.Ref, "amount": a.Amount, "period": a.Period})
	})
	mux.HandleFunc("GET /dev/releases", func(w http.ResponseWriter, _ *http.Request) {
		rels := s.Ledger.Releases()
		out := make([]map[string]any, 0, len(rels))
		for _, r := range rels {
			out = append(out, map[string]any{"budget_ref": r.BudgetRef, "car_id": r.CARID, "project_ref": r.ProjectRef, "amount": r.Amount, "currency": r.Currency,
				"approval_ref": r.ApprovalRef, "released_by": r.ReleasedBy, "released_at": r.ReleasedAt.UTC().Format(time.RFC3339)})
		}
		writeJSON(w, http.StatusOK, map[string]any{"releases": out})
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"projects": s.Ledger.Len(), "releases": len(s.Ledger.Releases())})
	})
	return mux
}

func version(projectRef string, v ledger.Version) *connectorv1.BudgetVersion {
	return &connectorv1.BudgetVersion{VersionRef: v.Ref, ProjectRef: projectRef, Name: v.Name, Status: v.Status, Total: v.Total, Currency: v.Currency,
		AsOf: ts(v.AsOf), ApprovedAt: ts(v.ApprovedAt), Lines: lines(v.Lines), Attributes: v.Attributes}
}

func lines(in []ledger.Line) []*connectorv1.BudgetLine {
	out := make([]*connectorv1.BudgetLine, 0, len(in))
	for _, l := range in {
		out = append(out, &connectorv1.BudgetLine{WbsPath: l.WbsPath, AccountCode: l.AccountCode, Amount: l.Amount})
	}
	return out
}

func ts(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
