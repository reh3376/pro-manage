// Package budget is pm-core's side of the ERP mirror (FSD §03 Budget, §10
// ERPi, roadmap "ERP mirror"): the budget versions, actuals, and commitments
// pulled from the ERP through the ERPi module, and the one controlled
// financial write, the budget release on CAR approval (§08 "On final approval
// (ERP-led profile)"). The ERP owns every mirrored row; pm-core adds the CAR
// linkage and never edits forecasts or actuals.
package budget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/erpi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
)

// OutboxKind is the outbox row that releases a budget at the ERP.
const OutboxKind = "erpi.budget.release"

// Service holds the dependencies. Erpi may be nil in pm-core (then releases
// are requested and only pm-workers, which has the client, performs them).
type Service struct {
	Store  *store.Store
	Chain  *audit.Chain
	Erpi   *erpi.Client
	Tenant string
	Log    *slog.Logger
}

// Configured reports whether an ERPi instance is wired (the ERP-led profile).
func (s *Service) Configured() bool { return s != nil && s.Erpi != nil }

// Provider names the instance for mirrored rows.
func (s *Service) provider() string {
	if s.Erpi == nil {
		return "unconfigured"
	}
	return s.Erpi.Provider
}

// ---------------------------------------------------------------- release

// ReleaseRequest is what an approval asks for.
type ReleaseRequest struct {
	ProjectID   uuid.UUID
	ProjectRef  string // the ERP's project id
	CARID       uuid.UUID
	CARCode     string
	Amount      string
	Currency    string
	Description string
	RequestedBy string
	AuditID     uuid.UUID
}

// Payload of an OutboxKind row.
type Payload struct {
	ReleaseID   string `json:"release_id"`
	Description string `json:"description"`
	CARCode     string `json:"car_code"`
}

// RequestRelease records the release and enqueues it, inside the caller's
// transaction (the approval's). Idempotent on the CAR. Returns the release id.
func (s *Service) RequestRelease(ctx context.Context, q *db.Queries, r ReleaseRequest, enqueue func(kind, sourceRef string, payload any) error) (uuid.UUID, error) {
	auditID := r.AuditID
	row, err := q.RequestBudgetRelease(ctx, db.RequestBudgetReleaseParams{
		ReleaseID: ids.New(), TenantID: s.Tenant, ProjectID: r.ProjectID, CarID: r.CARID, ErpInstance: s.provider(), ProjectRef: r.ProjectRef,
		Amount: r.Amount, Currency: r.Currency, RequestedBy: r.RequestedBy, AuditID: &auditID,
	})
	if errors.Is(err, pgx.ErrNoRows) { // already requested for this CAR
		existing, err := q.GetBudgetReleaseForCAR(ctx, r.CARID)
		if err != nil {
			return uuid.Nil, err
		}
		return existing.ReleaseID, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("budget: request release: %w", err)
	}
	if err := enqueue(OutboxKind, "release:"+row.ReleaseID.String(), Payload{ReleaseID: row.ReleaseID.String(), Description: r.Description, CARCode: r.CARCode}); err != nil {
		return uuid.Nil, err
	}
	return row.ReleaseID, nil
}

// ReleaseAtERP is the outbox handler: performs the release through the module
// and records the ERP's budget reference on the release row, the CAR, and the
// audit log. erpi.ErrRejected and erpi.ErrNotSupported are permanent (the row
// is marked failed and the drainer dead-letters); other errors retry.
func (s *Service) ReleaseAtERP(ctx context.Context, raw json.RawMessage) error {
	if s.Erpi == nil {
		return fmt.Errorf("%w: no ERPi module configured", erpi.ErrUnavailable)
	}
	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%w: payload: %v", erpi.ErrRejected, err)
	}
	id, err := ids.Parse(p.ReleaseID)
	if err != nil {
		return fmt.Errorf("%w: release id: %v", erpi.ErrRejected, err)
	}
	q := s.Store.Queries()
	row, err := q.GetBudgetRelease(ctx, db.GetBudgetReleaseParams{TenantID: s.Tenant, ReleaseID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: release %s unknown", erpi.ErrRejected, id)
	}
	if err != nil {
		return err
	}
	if row.Status == "released" {
		return nil // a retried row
	}
	approvalRef := ""
	if row.AuditID != nil {
		approvalRef = row.AuditID.String()
	}
	resp, err := s.Erpi.ReleaseBudget(ctx, &connectorv1.ReleaseBudgetRequest{
		CarId: row.CarID.String(), ProjectRef: row.ProjectRef, Amount: row.Amount, Currency: row.Currency, ApprovalRef: approvalRef,
		Description: p.Description, ReleasedBy: row.RequestedBy,
	})
	if err != nil {
		if errors.Is(err, erpi.ErrRejected) || errors.Is(err, erpi.ErrNotSupported) || errors.Is(err, erpi.ErrNotFound) {
			_ = q.MarkBudgetReleaseFailed(ctx, db.MarkBudgetReleaseFailedParams{ReleaseID: id, LastError: ptr(err.Error())})
			s.Log.Error("budget release refused by the ERP; the CAR stays approved with the release failed (§08)", "release", id, "car", row.CarID, "err", err)
			return s.auditFailure(ctx, row, err)
		}
		_ = q.MarkBudgetReleaseAttempt(ctx, db.MarkBudgetReleaseAttemptParams{ReleaseID: id, LastError: ptr(err.Error())})
		return err
	}
	if resp.GetStatus() == connectorv1.ReleaseStatus_RELEASE_STATUS_PENDING {
		_ = q.MarkBudgetReleaseAttempt(ctx, db.MarkBudgetReleaseAttemptParams{ReleaseID: id, LastError: ptr("erp: pending: " + resp.GetDetail())})
		return fmt.Errorf("%w: release pending at the ERP (%s)", erpi.ErrUnavailable, resp.GetDetail())
	}
	releasedAt := time.Now().UTC()
	if resp.GetReleasedAt() != nil {
		releasedAt = resp.GetReleasedAt().AsTime()
	}
	return s.Store.Tx(ctx, func(q *db.Queries) error {
		rel, err := q.MarkBudgetReleased(ctx, db.MarkBudgetReleasedParams{ReleaseID: id, ErpBudgetRef: ptr(resp.GetBudgetRef()), ReleasedAt: &releasedAt, Detail: resp.GetDetail()})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // raced with another worker
		}
		if err != nil {
			return err
		}
		if err := q.SetCARERPBudgetRef(ctx, db.SetCARERPBudgetRefParams{TenantID: s.Tenant, CarID: rel.CarID, ErpBudgetRef: ptr(resp.GetBudgetRef()), UpdatedBy: "pm-workers"}); err != nil {
			return err
		}
		pid := rel.ProjectID
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &pid, ActorUserID: "pm-workers", ActionType: "car.budget_released", SubjectType: "capital_authorization", SubjectID: rel.CarID.String(),
			After: map[string]any{"release_id": rel.ReleaseID, "erp_budget_ref": resp.GetBudgetRef(), "erp_status": resp.GetStatus().String(), "amount": rel.Amount, "currency": rel.Currency, "project_ref": rel.ProjectRef, "approval_audit_id": rel.AuditID, "erp_instance": rel.ErpInstance},
		})
		if err == nil {
			s.Log.Info("budget released at the ERP", "release", rel.ReleaseID, "car", rel.CarID, "erp_budget_ref", resp.GetBudgetRef(), "status", resp.GetStatus().String())
		}
		return err
	})
}

// auditFailure records the refused release (§08 "visible flag and an operational
// alert") and hands the permanent error back to the drainer.
func (s *Service) auditFailure(ctx context.Context, row db.BudgetRelease, cause error) error {
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		pid := row.ProjectID
		_, err := s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &pid, ActorUserID: "pm-workers", ActionType: "car.budget_release_failed", SubjectType: "capital_authorization", SubjectID: row.CarID.String(),
			After: map[string]any{"release_id": row.ReleaseID, "error": cause.Error(), "amount": row.Amount, "currency": row.Currency, "project_ref": row.ProjectRef},
		})
		return err
	})
	if err != nil {
		s.Log.Error("budget release failure not audited", "release", row.ReleaseID, "err", err)
	}
	return cause
}

// ---------------------------------------------------------------- mirror

// SyncResult says what one pull brought.
type SyncResult struct {
	ProjectID   uuid.UUID `json:"project_id"`
	ProjectRef  string    `json:"project_ref"`
	Baselines   int       `json:"baselines"`
	Forecasts   int       `json:"forecasts"`
	EAC         bool      `json:"eac"`
	Actuals     int       `json:"actuals"`
	Commitments int       `json:"commitments"`
	Skipped     []string  `json:"skipped,omitempty"` // feeds the instance does not declare
}

// SyncProject pulls every feed the instance declares for one project and
// upserts the mirror. A feed the instance does not support is skipped, not an
// error (the pro_manage-led profile mirrors financials only).
func (s *Service) SyncProject(ctx context.Context, project db.Project) (SyncResult, error) {
	res := SyncResult{ProjectID: project.ProjectID}
	if s.Erpi == nil {
		return res, fmt.Errorf("%w: no ERPi module configured", erpi.ErrUnavailable)
	}
	if project.ErpProjectRef == nil || *project.ErpProjectRef == "" {
		return res, fmt.Errorf("%w: project %s has no ERP reference", erpi.ErrNotFound, project.Code)
	}
	ref := *project.ErpProjectRef
	res.ProjectRef = ref
	q := s.Store.Queries()
	fail := func(err error) (SyncResult, error) {
		_ = q.MarkProjectERPSyncError(ctx, db.MarkProjectERPSyncErrorParams{TenantID: s.Tenant, ProjectID: project.ProjectID, ErpSyncError: ptr(err.Error())})
		return res, err
	}

	versions, err := s.Erpi.BudgetVersions(ctx, ref, nil)
	switch {
	case errors.Is(err, erpi.ErrNotSupported):
		res.Skipped = append(res.Skipped, "budget_versions")
	case err != nil:
		return fail(err)
	default:
		for _, v := range versions {
			if err := s.upsertVersion(ctx, q, project, "baseline", v); err != nil {
				return fail(err)
			}
			res.Baselines++
		}
	}
	forecasts, err := s.Erpi.ForecastVersions(ctx, ref, nil)
	switch {
	case errors.Is(err, erpi.ErrNotSupported):
		res.Skipped = append(res.Skipped, "forecast_versions")
	case err != nil:
		return fail(err)
	default:
		for _, v := range forecasts {
			if err := s.upsertVersion(ctx, q, project, "forecast", v); err != nil {
				return fail(err)
			}
			res.Forecasts++
		}
	}
	eac, err := s.Erpi.Eac(ctx, ref)
	switch {
	case errors.Is(err, erpi.ErrNotSupported), errors.Is(err, erpi.ErrNotFound):
		res.Skipped = append(res.Skipped, "eac")
	case err != nil:
		return fail(err)
	default:
		lines, _ := json.Marshal(linesOf(eac.GetLines()))
		v := &connectorv1.BudgetVersion{VersionRef: "current", ProjectRef: ref, Name: "EAC " + eac.GetMethod(), Status: "approved", Total: eac.GetEacAmount(), Currency: eac.GetCurrency(), AsOf: eac.GetAsOf()}
		if err := s.upsertVersionRaw(ctx, q, project, "eac", v, lines, map[string]string{"method": eac.GetMethod()}); err != nil {
			return fail(err)
		}
		res.EAC = true
	}
	actuals, err := s.Erpi.Actuals(ctx, ref, nil, nil)
	switch {
	case errors.Is(err, erpi.ErrNotSupported):
		res.Skipped = append(res.Skipped, "actuals")
	case err != nil:
		return fail(err)
	default:
		for _, a := range actuals {
			if err := q.UpsertBudgetActual(ctx, db.UpsertBudgetActualParams{
				ActualID: ids.New(), TenantID: s.Tenant, ProjectID: project.ProjectID, ErpInstance: s.provider(), ActualRef: a.GetActualRef(),
				WbsPath: a.GetWbsPath(), AccountCode: a.GetAccountCode(), Period: a.GetPeriod(), Amount: decimal(a.GetAmount()), Currency: orDefault(a.GetCurrency(), project.Currency),
				SourceDocRef: optional(a.GetSourceDocRef()), PostedAt: when(a.GetPostedAt()),
			}); err != nil {
				return fail(err)
			}
			res.Actuals++
		}
	}
	commitments, err := s.Erpi.Commitments(ctx, ref, true)
	switch {
	case errors.Is(err, erpi.ErrNotSupported):
		res.Skipped = append(res.Skipped, "commitments")
	case err != nil:
		return fail(err)
	default:
		for _, cm := range commitments {
			if err := q.UpsertBudgetCommitment(ctx, db.UpsertBudgetCommitmentParams{
				CommitmentID: ids.New(), TenantID: s.Tenant, ProjectID: project.ProjectID, ErpInstance: s.provider(), CommitmentRef: cm.GetCommitmentRef(),
				WbsPath: cm.GetWbsPath(), AccountCode: cm.GetAccountCode(), VendorRef: optional(cm.GetVendorRef()), PoRef: optional(cm.GetPoRef()),
				Amount: decimal(cm.GetAmount()), InvoicedToDate: decimal(orDefault(cm.GetInvoicedToDate(), "0")), Currency: orDefault(cm.GetCurrency(), project.Currency),
				Status: orDefault(cm.GetStatus(), "open"), CreatedAt: when(cm.GetCreatedAt()),
			}); err != nil {
				return fail(err)
			}
			res.Commitments++
		}
	}
	if err := q.MarkProjectERPSynced(ctx, db.MarkProjectERPSyncedParams{TenantID: s.Tenant, ProjectID: project.ProjectID}); err != nil {
		return res, err
	}
	return res, nil
}

// SyncDue pulls the projects least recently synced, up to limit. Returns how many it pulled and the first error.
func (s *Service) SyncDue(ctx context.Context, limit int32) (int, error) {
	projects, err := s.Store.Queries().ListProjectsForERPSync(ctx, db.ListProjectsForERPSyncParams{TenantID: s.Tenant, Limit: limit})
	if err != nil {
		return 0, err
	}
	var first error
	n := 0
	for _, p := range projects {
		if _, err := s.SyncProject(ctx, p); err != nil {
			s.Log.Warn("erp sync", "project", p.Code, "err", err)
			if first == nil {
				first = err
			}
			continue
		}
		n++
	}
	return n, first
}

func (s *Service) upsertVersion(ctx context.Context, q *db.Queries, project db.Project, kind string, v *connectorv1.BudgetVersion) error {
	lines, _ := json.Marshal(linesOf(v.GetLines()))
	return s.upsertVersionRaw(ctx, q, project, kind, v, lines, v.GetAttributes())
}

func (s *Service) upsertVersionRaw(ctx context.Context, q *db.Queries, project db.Project, kind string, v *connectorv1.BudgetVersion, lines []byte, attrs map[string]string) error {
	if attrs == nil {
		attrs = map[string]string{}
	}
	ab, _ := json.Marshal(attrs)
	if v.GetVersionRef() == "" {
		return fmt.Errorf("%w: %s version without a version_ref", erpi.ErrRejected, kind)
	}
	return q.UpsertBudgetVersion(ctx, db.UpsertBudgetVersionParams{
		VersionID: ids.New(), TenantID: s.Tenant, ProjectID: project.ProjectID, Kind: kind, ErpInstance: s.provider(), VersionRef: v.GetVersionRef(),
		Name: v.GetName(), Status: orDefault(v.GetStatus(), "approved"), Total: decimal(v.GetTotal()), Currency: orDefault(v.GetCurrency(), project.Currency),
		AsOf: when(v.GetAsOf()), ApprovedAt: when(v.GetApprovedAt()), Lines: lines, Attributes: ab,
	})
}

// ---------------------------------------------------------------- read side

// Summary is what the project page and the API show of the mirror.
type Summary struct {
	ProjectID        uuid.UUID          `json:"project_id"`
	ERPProjectRef    *string            `json:"erp_project_ref"`
	ERPInstance      string             `json:"erp_instance"`
	SyncedAt         *time.Time         `json:"synced_at"`
	SyncError        *string            `json:"sync_error"`
	Currency         string             `json:"currency"`
	Baseline         *db.BudgetVersion  `json:"baseline"` // latest approved baseline
	Forecast         *db.BudgetVersion  `json:"forecast"` // latest forecast
	EAC              *db.BudgetVersion  `json:"eac"`
	Versions         []db.BudgetVersion `json:"versions"`
	ActualsTotal     string             `json:"actuals_total"`
	ActualsRows      int64              `json:"actuals_rows"`
	CommitmentsOpen  string             `json:"commitments_open"`
	InvoicedTotal    string             `json:"invoiced_total"`
	CommitmentsRows  int64              `json:"commitments_rows"`
	AuthorizedTotal  string             `json:"authorized_total"` // approved CARs (pm-core's own number, for the variance line)
	Releases         []db.BudgetRelease `json:"releases"`
	ReleasedTotal    string             `json:"released_total"`
	RemainingToSpend string             `json:"remaining_to_spend"` // baseline minus actuals minus open commitments, when a baseline exists
}

// ProjectSummary assembles the mirror for one project.
func (s *Service) ProjectSummary(ctx context.Context, project db.Project) (Summary, error) {
	q := s.Store.Queries()
	out := Summary{ProjectID: project.ProjectID, ERPProjectRef: project.ErpProjectRef, ERPInstance: s.provider(), SyncedAt: project.ErpSyncedAt, SyncError: project.ErpSyncError, Currency: project.Currency,
		Versions: []db.BudgetVersion{}, Releases: []db.BudgetRelease{}, ActualsTotal: "0.00", CommitmentsOpen: "0.00", InvoicedTotal: "0.00", AuthorizedTotal: "0.00", ReleasedTotal: "0.00", RemainingToSpend: ""}
	versions, err := q.ListBudgetVersions(ctx, db.ListBudgetVersionsParams{TenantID: s.Tenant, ProjectID: project.ProjectID})
	if err != nil {
		return out, err
	}
	out.Versions = versions
	for i := range versions {
		v := versions[i]
		switch v.Kind {
		case "baseline":
			if out.Baseline == nil && v.Status == "approved" {
				out.Baseline = &versions[i]
			}
		case "forecast":
			if out.Forecast == nil {
				out.Forecast = &versions[i]
			}
		case "eac":
			if out.EAC == nil {
				out.EAC = &versions[i]
			}
		}
	}
	act, err := q.SumBudgetActuals(ctx, db.SumBudgetActualsParams{TenantID: s.Tenant, ProjectID: project.ProjectID})
	if err != nil {
		return out, err
	}
	out.ActualsTotal, out.ActualsRows = act.Total, act.Rows
	cm, err := q.SumBudgetCommitments(ctx, db.SumBudgetCommitmentsParams{TenantID: s.Tenant, ProjectID: project.ProjectID})
	if err != nil {
		return out, err
	}
	out.CommitmentsOpen, out.InvoicedTotal, out.CommitmentsRows = cm.OpenTotal, cm.InvoicedTotal, cm.Rows
	authorized, err := q.SumApprovedCARs(ctx, db.SumApprovedCARsParams{TenantID: s.Tenant, ProjectID: project.ProjectID})
	if err != nil {
		return out, err
	}
	out.AuthorizedTotal = authorized
	releases, err := q.ListBudgetReleasesForProject(ctx, db.ListBudgetReleasesForProjectParams{TenantID: s.Tenant, ProjectID: project.ProjectID})
	if err != nil {
		return out, err
	}
	out.Releases = releases
	released := new(big.Rat)
	for _, r := range releases {
		if r.Status == "released" {
			if x, ok := new(big.Rat).SetString(r.Amount); ok {
				released.Add(released, x)
			}
		}
	}
	out.ReleasedTotal = released.FloatString(2)
	if out.Baseline != nil {
		rem, _ := new(big.Rat).SetString(out.Baseline.Total)
		if a, ok := new(big.Rat).SetString(out.ActualsTotal); ok && rem != nil {
			rem.Sub(rem, a)
		}
		if c, ok := new(big.Rat).SetString(out.CommitmentsOpen); ok && rem != nil {
			rem.Sub(rem, c)
		}
		if rem != nil {
			out.RemainingToSpend = rem.FloatString(2)
		}
	}
	return out, nil
}

// ReleaseForCAR returns the release row for a CAR, nil when none was requested.
func (s *Service) ReleaseForCAR(ctx context.Context, carID uuid.UUID) (*db.BudgetRelease, error) {
	row, err := s.Store.Queries().GetBudgetReleaseForCAR(ctx, carID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// ---------------------------------------------------------------- helpers

type line struct {
	WbsPath     string `json:"wbs_path"`
	AccountCode string `json:"account_code"`
	Amount      string `json:"amount"`
}

func linesOf(in []*connectorv1.BudgetLine) []line {
	out := make([]line, 0, len(in))
	for _, l := range in {
		out = append(out, line{WbsPath: l.GetWbsPath(), AccountCode: l.GetAccountCode(), Amount: decimal(l.GetAmount())})
	}
	return out
}

// decimal normalizes an ERP amount string for numeric(18,2); unparsable input becomes 0.00 rather than failing the whole pull.
func decimal(s string) string {
	x, ok := new(big.Rat).SetString(s)
	if !ok {
		return "0.00"
	}
	return x.FloatString(2)
}

func when(t interface {
	AsTime() time.Time
	IsValid() bool
}) *time.Time {
	if t == nil || !t.IsValid() {
		return nil
	}
	v := t.AsTime()
	return &v
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func ptr[T any](v T) *T { return &v }
