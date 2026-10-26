package domain

// The toy CAR end to end (FSD roadmap Phase 0 exit test): EstimateArtifact and
// BusinessCasePromise feed a CapitalAuthorization; submission is the governed
// action `approval.car` (§06: guide on open, classify on submit under the kind's
// tier, GuidanceOutcome written with the AuditEvent, feedback through the
// outbox); the ApprovalWorkflow routes through the chain of §08 with approver
// pools; each decision is audited; final approval authorizes the amount.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/governance"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/notify"
	"github.com/reh3376/pro-manage/services/pm-core/internal/outbox"
	"github.com/reh3376/pro-manage/services/pm-core/internal/signing"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

// ActionCAR is the governed action kind (MDEMG tool_name and mdemg.tier.<kind>).
const ActionCAR = "approval.car"

var (
	carKinds      = set("fel_funding", "full", "supplemental")
	carBases      = set("tic", "tic_plus_contingency", "total_project_cost")
	promiseKinds  = set("loss_recovery", "capacity", "cost_per_unit", "energy_per_unit", "quality", "safety", "compliance", "other")
	promiseStates = set("proposed", "committed")
	pctRe         = regexp.MustCompile(`^-?\d{1,3}(\.\d{1,2})?$`)
	decimalRe     = regexp.MustCompile(`^-?\d{1,14}(\.\d{1,4})?$`)
)

// ---------------------------------------------------------------- estimates

// CreateEstimateInput is the API body for POST /projects/{id}/estimates.
type CreateEstimateInput struct {
	AaceClass             int     `json:"aace_class"`
	AsOf                  string  `json:"as_of"`
	PreparedBy            string  `json:"prepared_by"`
	ReviewedBy            *string `json:"reviewed_by"`
	BasisOfEstimateDocRef *string `json:"basis_of_estimate_doc_ref"`
	ScopeDefinitionPct    *string `json:"scope_definition_pct"`
	AccuracyRangeLowPct   *string `json:"accuracy_range_low_pct"`
	AccuracyRangeHighPct  *string `json:"accuracy_range_high_pct"`
	Tic                   string  `json:"tic"`
	ContingencyAmount     string  `json:"contingency_amount"`
	ContingencyMethod     *string `json:"contingency_method"`
	Currency              string  `json:"currency"`
}

// CreateEstimate records an EstimateArtifact with its audit event.
func (s *Service) CreateEstimate(ctx context.Context, actor Actor, projectID uuid.UUID, in CreateEstimateInput) (db.EstimateArtifact, error) {
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return db.EstimateArtifact{}, err
	}
	if in.AaceClass < 1 || in.AaceClass > 5 {
		return db.EstimateArtifact{}, validationError("aace_class", "AACE class 1 to 5")
	}
	asOf, err := time.Parse("2006-01-02", strings.TrimSpace(in.AsOf))
	if err != nil {
		return db.EstimateArtifact{}, validationError("as_of", "YYYY-MM-DD required")
	}
	in.PreparedBy = strings.TrimSpace(in.PreparedBy)
	if in.PreparedBy == "" {
		in.PreparedBy = actor.UserID
	}
	if _, err := parseMoney(in.Tic, "tic"); err != nil {
		return db.EstimateArtifact{}, err
	}
	if strings.TrimSpace(in.ContingencyAmount) == "" {
		in.ContingencyAmount = "0.00"
	}
	if _, err := parseMoney(in.ContingencyAmount, "contingency_amount"); err != nil {
		return db.EstimateArtifact{}, err
	}
	for _, f := range []struct {
		name string
		v    *string
	}{{"scope_definition_pct", in.ScopeDefinitionPct}, {"accuracy_range_low_pct", in.AccuracyRangeLowPct}, {"accuracy_range_high_pct", in.AccuracyRangeHighPct}} {
		if f.v != nil && !pctRe.MatchString(strings.TrimSpace(*f.v)) {
			return db.EstimateArtifact{}, validationError(f.name, "percent with up to 2 places, e.g. -15.00")
		}
	}
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	if in.Currency == "" {
		in.Currency = project.Currency
	}
	if len(in.Currency) != 3 {
		return db.EstimateArtifact{}, validationError("currency", "ISO 4217 code required")
	}
	var est db.EstimateArtifact
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		var err error
		est, err = q.CreateEstimateArtifact(ctx, db.CreateEstimateArtifactParams{
			EstimateID: ids.New(), TenantID: s.Tenant, ProjectID: project.ProjectID, AaceClass: int16(in.AaceClass), AsOf: asOf,
			PreparedBy: in.PreparedBy, ReviewedBy: in.ReviewedBy, BasisOfEstimateDocRef: in.BasisOfEstimateDocRef,
			ScopeDefinitionPct: in.ScopeDefinitionPct, AccuracyRangeLowPct: in.AccuracyRangeLowPct, AccuracyRangeHighPct: in.AccuracyRangeHighPct,
			Tic: strings.TrimSpace(in.Tic), ContingencyAmount: strings.TrimSpace(in.ContingencyAmount), ContingencyMethod: in.ContingencyMethod,
			Currency: in.Currency, CreatedBy: actor.UserID,
		})
		if err != nil {
			return fmt.Errorf("create estimate: %w", err)
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &project.ProjectID, ActorUserID: actor.UserID, ActionType: "estimate.create", SubjectType: "estimate_artifact",
			SubjectID: est.EstimateID.String(), After: est, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
	return est, err
}

// ---------------------------------------------------------------- promises

// CreatePromiseInput is the API body for POST /projects/{id}/promises.
type CreatePromiseInput struct {
	Kind               string  `json:"kind"`
	MetricID           string  `json:"metric_id"`
	LossCategoryID     *string `json:"loss_category_id"`
	BaselineValue      *string `json:"baseline_value"`
	BaselinePeriod     *string `json:"baseline_period"`
	PromisedValue      *string `json:"promised_value"`
	PromisedByDate     *string `json:"promised_by_date"`
	DollarValuePerYear *string `json:"dollar_value_per_year"`
	MeasurementBasis   string  `json:"measurement_basis"`
	Status             string  `json:"status"`
}

// CreatePromise records a BusinessCasePromise. A committed promise must carry
// its CAR-time baseline, period, promised value, metric, and measurement basis
// (§03: the third ROI calculation is only honest if the basis cannot drift).
func (s *Service) CreatePromise(ctx context.Context, actor Actor, projectID uuid.UUID, in CreatePromiseInput) (db.BusinessCasePromise, error) {
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return db.BusinessCasePromise{}, err
	}
	if !promiseKinds[in.Kind] {
		return db.BusinessCasePromise{}, validationError("kind", "one of loss_recovery, capacity, cost_per_unit, energy_per_unit, quality, safety, compliance, other")
	}
	if in.Status == "" {
		in.Status = "proposed"
	}
	if !promiseStates[in.Status] {
		return db.BusinessCasePromise{}, validationError("status", "proposed or committed")
	}
	for _, f := range []struct {
		name string
		v    *string
	}{{"baseline_value", in.BaselineValue}, {"promised_value", in.PromisedValue}} {
		if f.v != nil && !decimalRe.MatchString(strings.TrimSpace(*f.v)) {
			return db.BusinessCasePromise{}, validationError(f.name, "decimal with up to 4 places")
		}
	}
	if in.DollarValuePerYear != nil {
		if _, err := parseMoney(*in.DollarValuePerYear, "dollar_value_per_year"); err != nil {
			return db.BusinessCasePromise{}, err
		}
	}
	byDate, err := optionalDate(in.PromisedByDate, "promised_by_date")
	if err != nil {
		return db.BusinessCasePromise{}, err
	}
	in.MetricID = strings.TrimSpace(in.MetricID)
	in.MeasurementBasis = strings.TrimSpace(in.MeasurementBasis)
	if in.Status == "committed" {
		switch {
		case in.MetricID == "":
			return db.BusinessCasePromise{}, validationError("metric_id", "required for a committed promise")
		case in.BaselineValue == nil || in.BaselinePeriod == nil || strings.TrimSpace(*in.BaselinePeriod) == "":
			return db.BusinessCasePromise{}, validationError("baseline_value", "baseline_value and baseline_period are required for a committed promise")
		case in.PromisedValue == nil:
			return db.BusinessCasePromise{}, validationError("promised_value", "required for a committed promise")
		case in.MeasurementBasis == "":
			return db.BusinessCasePromise{}, validationError("measurement_basis", "required for a committed promise")
		}
	}
	var pr db.BusinessCasePromise
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		var err error
		pr, err = q.CreatePromise(ctx, db.CreatePromiseParams{
			PromiseID: ids.New(), TenantID: s.Tenant, ProjectID: project.ProjectID, Kind: in.Kind, MetricID: in.MetricID, LossCategoryID: in.LossCategoryID,
			BaselineValue: in.BaselineValue, BaselinePeriod: in.BaselinePeriod, PromisedValue: in.PromisedValue, PromisedByDate: byDate,
			DollarValuePerYear: in.DollarValuePerYear, MeasurementBasis: in.MeasurementBasis, Status: in.Status, CreatedBy: actor.UserID,
		})
		if err != nil {
			return fmt.Errorf("create promise: %w", err)
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &project.ProjectID, ActorUserID: actor.UserID, ActionType: "promise.create", SubjectType: "business_case_promise",
			SubjectID: pr.PromiseID.String(), After: pr, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
	return pr, err
}

// ListPromises for a project.
func (s *Service) ListPromises(ctx context.Context, projectID uuid.UUID) ([]db.BusinessCasePromise, error) {
	return s.Store.Queries().ListPromisesForProject(ctx, db.ListPromisesForProjectParams{TenantID: s.Tenant, ProjectID: projectID})
}

// ---------------------------------------------------------------- CAR: create and read

// CreateCARInput is the API body for POST /projects/{id}/cars (a draft).
type CreateCARInput struct {
	Code            *string `json:"code"`
	Kind            string  `json:"kind"`
	AmountRequested string  `json:"amount_requested"`
	Currency        *string `json:"currency"`
	Basis           *string `json:"basis"`
	EstimateID      *string `json:"estimate_id"`
	BusinessCase    string  `json:"business_case"`
	SupersedesCarID *string `json:"supersedes_car_id"`
}

// CARView is what the API returns for a CAR.
type CARView struct {
	CAR                  db.CapitalAuthorization `json:"car"`
	Estimate             *db.EstimateArtifact    `json:"estimate,omitempty"`
	Workflow             *db.ApprovalWorkflow    `json:"workflow,omitempty"`
	Stages               []db.ApprovalStage      `json:"stages"`
	CumulativeAuthorized string                  `json:"cumulative_authorized"` // approved CARs on the project, this one included once approved
}

// CreateCAR drafts a CAR. Nothing is governed yet: a draft is the user's own.
func (s *Service) CreateCAR(ctx context.Context, actor Actor, projectID uuid.UUID, in CreateCARInput) (CARView, error) {
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return CARView{}, err
	}
	if !carKinds[in.Kind] {
		return CARView{}, validationError("kind", "one of fel_funding, full, supplemental")
	}
	if _, err := parseMoney(in.AmountRequested, "amount_requested"); err != nil {
		return CARView{}, err
	}
	currency := project.Currency
	if in.Currency != nil && strings.TrimSpace(*in.Currency) != "" {
		currency = strings.ToUpper(strings.TrimSpace(*in.Currency))
		if len(currency) != 3 {
			return CARView{}, validationError("currency", "ISO 4217 code required")
		}
	}
	basis := "tic"
	if in.Basis != nil && *in.Basis != "" {
		basis = *in.Basis
		if !carBases[basis] {
			return CARView{}, validationError("basis", "one of tic, tic_plus_contingency, total_project_cost")
		}
	}
	estimateID, err := optionalUUID(in.EstimateID, "estimate_id")
	if err != nil {
		return CARView{}, err
	}
	if estimateID != nil {
		est, err := s.Store.Queries().GetEstimateArtifact(ctx, db.GetEstimateArtifactParams{TenantID: s.Tenant, EstimateID: *estimateID})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && est.ProjectID != project.ProjectID) {
			return CARView{}, validationError("estimate_id", "unknown estimate for this project")
		}
		if err != nil {
			return CARView{}, err
		}
	}
	supersedes, err := optionalUUID(in.SupersedesCarID, "supersedes_car_id")
	if err != nil {
		return CARView{}, err
	}
	if in.Kind == "supplemental" && supersedes == nil {
		return CARView{}, validationError("supersedes_car_id", "a supplemental names the CAR it adds to")
	}
	if supersedes != nil {
		prev, err := s.Store.Queries().GetCAR(ctx, db.GetCARParams{TenantID: s.Tenant, CarID: *supersedes})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && prev.ProjectID != project.ProjectID) {
			return CARView{}, validationError("supersedes_car_id", "unknown CAR for this project")
		}
		if err != nil {
			return CARView{}, err
		}
	}
	var car db.CapitalAuthorization
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		code := ""
		if in.Code != nil {
			code = strings.ToUpper(strings.TrimSpace(*in.Code))
		}
		if code == "" {
			n, err := q.NextCARSequence(ctx, s.Tenant)
			if err != nil {
				return err
			}
			code = fmt.Sprintf("CR-%d", n)
		} else if !codeRe.MatchString(code) {
			return validationError("code", "2 to 64 characters: A-Z, 0-9, '.', '_', '-'")
		}
		var err error
		car, err = q.CreateCAR(ctx, db.CreateCARParams{
			CarID: ids.New(), TenantID: s.Tenant, ProjectID: project.ProjectID, Code: code, Kind: in.Kind, AmountRequested: strings.TrimSpace(in.AmountRequested),
			Currency: currency, Basis: basis, EstimateID: estimateID, BusinessCase: strings.TrimSpace(in.BusinessCase), SupersedesCarID: supersedes, CreatedBy: actor.UserID,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("%w: CAR code %s", ErrConflict, code)
			}
			return fmt.Errorf("create car: %w", err)
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &project.ProjectID, ActorUserID: actor.UserID, ActionType: "car.create", SubjectType: "capital_authorization",
			SubjectID: car.CarID.String(), After: car, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
	if err != nil {
		return CARView{}, err
	}
	return s.view(ctx, car)
}

// GetCAR by id.
func (s *Service) GetCAR(ctx context.Context, carID uuid.UUID) (CARView, error) {
	car, err := s.Store.Queries().GetCAR(ctx, db.GetCARParams{TenantID: s.Tenant, CarID: carID})
	if errors.Is(err, pgx.ErrNoRows) {
		return CARView{}, fmt.Errorf("%w: car %s", ErrNotFound, carID)
	}
	if err != nil {
		return CARView{}, err
	}
	return s.view(ctx, car)
}

// ListCARs for a project.
func (s *Service) ListCARs(ctx context.Context, projectID uuid.UUID) ([]db.CapitalAuthorization, error) {
	return s.Store.Queries().ListCARsForProject(ctx, db.ListCARsForProjectParams{TenantID: s.Tenant, ProjectID: projectID})
}

func (s *Service) view(ctx context.Context, car db.CapitalAuthorization) (CARView, error) {
	q := s.Store.Queries()
	v := CARView{CAR: car, Stages: []db.ApprovalStage{}}
	if car.EstimateID != nil {
		est, err := q.GetEstimateArtifact(ctx, db.GetEstimateArtifactParams{TenantID: s.Tenant, EstimateID: *car.EstimateID})
		if err == nil {
			v.Estimate = &est
		}
	}
	if car.WorkflowID != nil {
		wf, err := q.GetWorkflow(ctx, db.GetWorkflowParams{TenantID: s.Tenant, WorkflowID: *car.WorkflowID})
		if err != nil {
			return v, err
		}
		v.Workflow = &wf
		if v.Stages, err = q.ListStages(ctx, wf.WorkflowID); err != nil {
			return v, err
		}
	}
	sum, err := q.SumApprovedCARs(ctx, db.SumApprovedCARsParams{TenantID: s.Tenant, ProjectID: car.ProjectID})
	if err != nil {
		return v, err
	}
	v.CumulativeAuthorized = sum
	return v, nil
}

// ---------------------------------------------------------------- CAR: the governed submission

// Preview is what the CAR form shows before submission: the effective policy,
// the chain the amount would route through, and the tier the submit will run at.
type Preview struct {
	RoutingAmount string          `json:"routing_amount"`
	Chain         []StageDef      `json:"chain"`
	Tier          governance.Tier `json:"tier"`
	Policy        CARPolicy       `json:"policy"`
}

// GuidanceView is GET /cars/{id}/guidance: guide on open plus the preview.
type GuidanceView struct {
	Guidance governance.Guidance `json:"guidance"`
	Preview  Preview             `json:"preview"`
	Session  string              `json:"session_id"`
}

type submission struct {
	car     db.CapitalAuthorization
	project db.Project
	site    db.Site
	est     *db.EstimateArtifact
	policy  CARPolicy
	routing *big.Rat
	amount  *big.Rat
	chain   []StageDef
	tier    governance.Tier
}

// prepare loads everything a submit or preview needs and resolves the policy.
func (s *Service) prepare(ctx context.Context, car db.CapitalAuthorization, now time.Time) (submission, error) {
	sub := submission{car: car}
	var err error
	if sub.project, err = s.GetProject(ctx, car.ProjectID); err != nil {
		return sub, err
	}
	if sub.site, err = s.GetSite(ctx, sub.project.SiteID); err != nil {
		return sub, err
	}
	if sub.policy, err = resolveCARPolicy(ctx, s.Store.Queries(), sub.project, now); err != nil {
		return sub, err
	}
	if car.EstimateID != nil {
		est, err := s.Store.Queries().GetEstimateArtifact(ctx, db.GetEstimateArtifactParams{TenantID: s.Tenant, EstimateID: *car.EstimateID})
		if err != nil {
			return sub, err
		}
		sub.est = &est
	}
	sub.amount, _ = new(big.Rat).SetString(car.AmountRequested)
	sub.routing = sub.amount
	if car.Kind == "supplemental" {
		// Supplementals route on the cumulative authorized amount so thresholds cannot be crept past in steps (§08).
		sum, err := s.Store.Queries().SumApprovedCARs(ctx, db.SumApprovedCARsParams{TenantID: s.Tenant, ProjectID: car.ProjectID})
		if err != nil {
			return sub, err
		}
		approved, _ := new(big.Rat).SetString(sum)
		sub.routing = new(big.Rat).Add(approved, sub.amount)
	}
	sub.chain = BuildCARChain(sub.routing, sub.policy)
	sub.tier = CARTier(car.Kind, sub.routing, sub.policy)
	return sub, nil
}

func (sub submission) preview() Preview {
	return Preview{RoutingAmount: money(sub.routing), Chain: sub.chain, Tier: sub.tier, Policy: sub.policy}
}

// preconditions are the deterministic rules of §08 "Capital authorization" by kind.
func (s *Service) preconditions(ctx context.Context, sub submission) error {
	car := sub.car
	if strings.TrimSpace(car.BusinessCase) == "" {
		return fmt.Errorf("%w: business case narrative is required", ErrPrecondition)
	}
	switch car.Kind {
	case "fel_funding":
		if sub.est == nil {
			return fmt.Errorf("%w: a FEL-funding CAR needs an estimate artifact", ErrPrecondition)
		}
		if int(sub.est.AaceClass) > sub.policy.FELClass {
			return fmt.Errorf("%w: estimate is AACE class %d; class %d or better required (%s)", ErrPrecondition, sub.est.AaceClass, sub.policy.FELClass, keyFELClass)
		}
		tic, _ := new(big.Rat).SetString(sub.est.Tic)
		cap := new(big.Rat).Mul(tic, new(big.Rat).Quo(sub.policy.FELCapPct, big.NewRat(100, 1)))
		if sub.amount.Cmp(cap) > 0 {
			return fmt.Errorf("%w: FEL-funding amount %s exceeds %s%% of the current estimate (%s)", ErrPrecondition, money(sub.amount), sub.policy.FELCapPct.FloatString(2), money(cap))
		}
	case "full":
		if sub.project.FelStage != "fel3" {
			return fmt.Errorf("%w: the full CAR is submitted at FEL3; project is at %s", ErrPrecondition, sub.project.FelStage)
		}
		if sub.est == nil {
			return fmt.Errorf("%w: the full CAR needs a class %d or better estimate artifact", ErrPrecondition, sub.policy.FullClass)
		}
		if int(sub.est.AaceClass) > sub.policy.FullClass {
			return fmt.Errorf("%w: estimate is AACE class %d; class %d or better required (%s)", ErrPrecondition, sub.est.AaceClass, sub.policy.FullClass, keyFullClass)
		}
		if sub.est.BasisOfEstimateDocRef == nil || strings.TrimSpace(*sub.est.BasisOfEstimateDocRef) == "" {
			return fmt.Errorf("%w: the full CAR's estimate needs a basis-of-estimate document", ErrPrecondition)
		}
		promises, err := s.ListPromises(ctx, car.ProjectID)
		if err != nil {
			return err
		}
		if len(promises) == 0 {
			return fmt.Errorf("%w: the full CAR needs at least one committed business-case promise", ErrPrecondition)
		}
		for _, p := range promises {
			if p.Status != "committed" {
				return fmt.Errorf("%w: promise %s is %s; every promise must be committed with a baseline and measurement basis", ErrPrecondition, p.PromiseID, p.Status)
			}
		}
	case "supplemental":
		if car.SupersedesCarID == nil {
			return fmt.Errorf("%w: a supplemental names the CAR it adds to", ErrPrecondition)
		}
		prev, err := s.Store.Queries().GetCAR(ctx, db.GetCARParams{TenantID: s.Tenant, CarID: *car.SupersedesCarID})
		if err != nil {
			return err
		}
		if prev.Status != "approved" {
			return fmt.Errorf("%w: the superseded CAR %s is %s, not approved", ErrPrecondition, prev.Code, prev.Status)
		}
	}
	return nil
}

// classifyPayload is the bounded summary MDEMG classifies (§06: never a full form body).
func (sub submission) classifyPayload() string {
	est := "no estimate"
	if sub.est != nil {
		est = fmt.Sprintf("AACE class %d estimate as of %s, TIC %s %s", sub.est.AaceClass, sub.est.AsOf.Format("2006-01-02"), sub.est.Tic, sub.est.Currency)
	}
	roles := make([]string, len(sub.chain))
	for i, st := range sub.chain {
		roles[i] = st.Role
	}
	bc := sub.car.BusinessCase
	if len(bc) > 600 {
		bc = bc[:600] + "..."
	}
	return fmt.Sprintf("Submit CAR %s (%s) on project %s (%s) at site %s: amount %s %s on basis %s; routing amount %s; %s; FEL stage %s; chain %s; thresholds vp_finance %s, cfo %s, ceo %s %s. Business case: %s",
		sub.car.Code, sub.car.Kind, sub.project.Code, sub.project.Name, sub.site.Code, money(sub.amount), sub.car.Currency, sub.car.Basis, money(sub.routing),
		est, sub.project.FelStage, strings.Join(roles, " > "), money(sub.policy.ThrVPFinance), money(sub.policy.ThrCFO), money(sub.policy.ThrCEO), sub.policy.Currency, bc)
}

func sessionID(actor Actor, projectID uuid.UUID) string {
	return actor.UserID + ":" + projectID.String()
}

// CARGuidance implements guide on open for the CAR form.
func (s *Service) CARGuidance(ctx context.Context, actor Actor, carID uuid.UUID) (GuidanceView, error) {
	car, err := s.Store.Queries().GetCAR(ctx, db.GetCARParams{TenantID: s.Tenant, CarID: carID})
	if errors.Is(err, pgx.ErrNoRows) {
		return GuidanceView{}, fmt.Errorf("%w: car %s", ErrNotFound, carID)
	}
	if err != nil {
		return GuidanceView{}, err
	}
	sub, err := s.prepare(ctx, car, time.Now().UTC())
	if err != nil {
		return GuidanceView{}, err
	}
	g := s.gate()
	sid := sessionID(actor, car.ProjectID)
	hint := fmt.Sprintf("CAR %s (%s) %s %s on %s; routes %s; tier %s", car.Code, car.Kind, money(sub.amount), car.Currency, sub.project.Code, money(sub.routing), sub.tier)
	return GuidanceView{Guidance: g.GuideOnOpen(ctx, g.Spaces(car.ProjectID.String()), sid, hint), Preview: sub.preview(), Session: sid}, nil
}

// PreclassifyResult is POST /cars/{id}/preclassify.
type PreclassifyResult struct {
	Tier        governance.Tier `json:"tier"`
	Verdict     string          `json:"verdict,omitempty"`
	Cached      bool            `json:"cached"`
	Unavailable bool            `json:"unavailable"`
	CacheKey    string          `json:"cache_key"`
	Error       string          `json:"error,omitempty"`
}

// PreclassifyCAR runs the classify for the settled form and caches the verdict
// so the submit is usually instant (§06 pre-classification). Tier C is a no-op.
func (s *Service) PreclassifyCAR(ctx context.Context, actor Actor, carID uuid.UUID) (PreclassifyResult, error) {
	car, err := s.Store.Queries().GetCAR(ctx, db.GetCARParams{TenantID: s.Tenant, CarID: carID})
	if errors.Is(err, pgx.ErrNoRows) {
		return PreclassifyResult{}, fmt.Errorf("%w: car %s", ErrNotFound, carID)
	}
	if err != nil {
		return PreclassifyResult{}, err
	}
	if car.Status != "draft" {
		return PreclassifyResult{}, fmt.Errorf("%w: CAR %s is %s", ErrInvalidState, car.Code, car.Status)
	}
	sub, err := s.prepare(ctx, car, time.Now().UTC())
	if err != nil {
		return PreclassifyResult{}, err
	}
	if err := s.preconditions(ctx, sub); err != nil {
		return PreclassifyResult{}, err
	}
	if sub.tier == governance.TierC {
		return PreclassifyResult{Tier: sub.tier}, nil
	}
	v := s.classify(ctx, actor, sub)
	return PreclassifyResult{Tier: sub.tier, Verdict: v.Decision.Verdict, Cached: v.Cached, Unavailable: v.Unavailable, CacheKey: v.CacheKey, Error: v.Error}, nil
}

func (s *Service) classify(ctx context.Context, actor Actor, sub submission) governance.Verdict {
	snapshot, _ := jsonMarshal(sub.policy)
	g := s.gate()
	return g.Classify(mdemgadapter.WithRequestID(ctx, actor.RequestID), g.Spaces(sub.car.ProjectID.String()), governance.ClassifyInput{
		ActionKind: ActionCAR, UserID: actor.UserID, ProjectID: sub.car.ProjectID.String(), SessionID: sessionID(actor, sub.car.ProjectID),
		Payload: sub.classifyPayload(), PolicySnapshot: snapshot, Timeout: sub.policy.ClassifyTimeout,
	})
}

// SubmitCARInput is the API body for POST /cars/{id}/submit: what the user saw
// and did with the guidance (from GET .../guidance).
type SubmitCARInput struct {
	GuidanceIDs   []string                    `json:"guidance_ids"`
	GuidanceSeen  []mdemgadapter.GuidanceItem `json:"guidance_seen"`
	Dismissed     bool                        `json:"dismissed"`
	DismissReason string                      `json:"dismiss_reason"`
}

// GovernanceRecord is returned with the submit (and in the error body on deny or refusal).
type GovernanceRecord struct {
	ActionKind    string          `json:"action_kind"`
	Tier          governance.Tier `json:"tier"`
	Verdict       string          `json:"verdict,omitempty"`
	ViolatedCodes []string        `json:"violated_codes"`
	DeniedBy      string          `json:"denied_by,omitempty"`
	UserResponse  string          `json:"user_response"`
	Cached        bool            `json:"cached"`
	GuidanceIDs   []string        `json:"guidance_ids"`
	MDEMGError    string          `json:"mdemg_error,omitempty"`
	AuditID       string          `json:"audit_id"`
}

// SubmitResult is the 200 body.
type SubmitResult struct {
	View       CARView          `json:"car_view"`
	Governance GovernanceRecord `json:"governance"`
}

// DeniedError carries the governance record behind ErrGovernanceDenied or ErrGovernanceUnavailable.
type DeniedError struct {
	Sentinel error
	Record   GovernanceRecord
}

func (e *DeniedError) Error() string {
	if e.Sentinel == ErrGovernanceDenied {
		return fmt.Sprintf("%v: %s", e.Sentinel, strings.Join(e.Record.ViolatedCodes, ", "))
	}
	return fmt.Sprintf("%v: %s", e.Sentinel, e.Record.MDEMGError)
}

func (e *DeniedError) Unwrap() error { return e.Sentinel }

// SubmitCAR is the governed action. Order: deterministic preconditions, chain
// and tier, classify (cached or inline), tier resolution, then one transaction
// that moves the CAR in flight, creates the workflow and stages, opens stage 1,
// appends the AuditEvent with the GuidanceOutcome, and enqueues feedback and
// the observation. A deny or a Tier A refusal is itself audited (with its
// GuidanceOutcome) and the CAR stays a draft.
func (s *Service) SubmitCAR(ctx context.Context, actor Actor, carID uuid.UUID, in SubmitCARInput) (SubmitResult, error) {
	car, err := s.Store.Queries().GetCAR(ctx, db.GetCARParams{TenantID: s.Tenant, CarID: carID})
	if errors.Is(err, pgx.ErrNoRows) {
		return SubmitResult{}, fmt.Errorf("%w: car %s", ErrNotFound, carID)
	}
	if err != nil {
		return SubmitResult{}, err
	}
	if car.Status != "draft" {
		return SubmitResult{}, fmt.Errorf("%w: CAR %s is %s", ErrInvalidState, car.Code, car.Status)
	}
	now := time.Now().UTC()
	sub, err := s.prepare(ctx, car, now)
	if err != nil {
		return SubmitResult{}, err
	}
	if err := s.preconditions(ctx, sub); err != nil {
		return SubmitResult{}, err
	}

	// Resolve the pools before classifying: an empty pool is a precondition failure, not a governance one.
	assignees := make([][]string, len(sub.chain))
	for i, st := range sub.chain {
		if st.Pool == "project_owner" {
			assignees[i] = []string{sub.project.OwnerUserID}
			continue
		}
		members, err := s.Store.Queries().PoolMembers(ctx, db.PoolMembersParams{Pool: st.Pool, SiteID: &sub.site.SiteID})
		if err != nil {
			return SubmitResult{}, err
		}
		if len(members) == 0 {
			return SubmitResult{}, fmt.Errorf("%w: approver pool %s is empty for site %s (stage %d)", ErrPrecondition, st.Pool, sub.site.Code, st.Order)
		}
		assignees[i] = members
	}

	userResponse := governance.ResponseFollowed
	if in.Dismissed {
		userResponse = governance.ResponseDismissed
		if strings.TrimSpace(in.DismissReason) != "" {
			userResponse = governance.ResponseIgnoredWithReason
		}
	}
	if in.GuidanceIDs == nil {
		in.GuidanceIDs = []string{}
	}
	if in.GuidanceSeen == nil {
		in.GuidanceSeen = []mdemgadapter.GuidanceItem{}
	}
	var v governance.Verdict
	if sub.tier != governance.TierC {
		v = s.classify(ctx, actor, sub)
	}
	res := governance.Resolve(sub.tier, v, userResponse)
	rec := GovernanceRecord{ActionKind: ActionCAR, Tier: sub.tier, Verdict: res.Verdict, ViolatedCodes: v.Decision.ViolatedCodes, DeniedBy: v.Decision.DeniedBy,
		UserResponse: res.UserResponse, Cached: v.Cached, GuidanceIDs: in.GuidanceIDs, MDEMGError: v.Error}
	if rec.ViolatedCodes == nil {
		rec.ViolatedCodes = []string{}
	}
	snapshot, _ := jsonMarshal(sub.policy)
	seen, _ := jsonMarshal(in.GuidanceSeen)
	var dismiss *string
	if r := strings.TrimSpace(in.DismissReason); r != "" {
		dismiss = &r
	}
	var verdict *string
	if res.Verdict != "" {
		verdict = &res.Verdict
	}
	guidanceID := ""
	if len(in.GuidanceIDs) > 0 {
		guidanceID = in.GuidanceIDs[0]
	}

	if !res.Proceed {
		// Denied or refused: audit the attempt with its GuidanceOutcome; the CAR stays a draft.
		action := "car.submit.denied"
		feedbackOutcome := mdemgadapter.OutcomeBlockedTruePositive
		sentinel := ErrGovernanceDenied
		if res.Refused {
			action, feedbackOutcome, sentinel = "car.submit.refused", "", ErrGovernanceUnavailable
		}
		err := s.Store.Tx(ctx, func(q *db.Queries) error {
			ev, err := s.Chain.Append(ctx, q, audit.Event{
				TenantID: s.Tenant, ProjectID: &car.ProjectID, ActorUserID: actor.UserID, ActionType: action, SubjectType: "capital_authorization", SubjectID: car.CarID.String(),
				Before: car, After: map[string]any{"governance": rec, "policy_snapshot": sub.policy}, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
				GuidanceID: guidanceID, ClassificationVerdict: res.Verdict,
			})
			if err != nil {
				return err
			}
			rec.AuditID = ev.AuditID.String()
			if _, err := q.InsertGuidanceOutcome(ctx, db.InsertGuidanceOutcomeParams{
				OutcomeID: ids.New(), TenantID: s.Tenant, AuditID: ev.AuditID, ProjectID: &car.ProjectID, UserID: actor.UserID, ActionType: ActionCAR,
				GuidanceIds: in.GuidanceIDs, GuidanceItemsSnapshot: seen, ClassificationVerdict: verdict, ViolatedCodes: rec.ViolatedCodes,
				FailTier: string(sub.tier), UserResponse: res.UserResponse, DismissReason: dismiss,
			}); err != nil {
				return fmt.Errorf("guidance outcome: %w", err)
			}
			if feedbackOutcome != "" {
				return s.enqueueFeedback(ctx, q, ev.AuditID, actor, car.ProjectID, in.GuidanceIDs, feedbackOutcome, "CAR "+car.Code+" submission blocked: "+strings.Join(rec.ViolatedCodes, ", "))
			}
			return nil
		})
		if err != nil {
			return SubmitResult{}, err
		}
		return SubmitResult{Governance: rec}, &DeniedError{Sentinel: sentinel, Record: rec}
	}

	var after db.CapitalAuthorization
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		wfID := ids.New()
		wf, err := q.CreateWorkflow(ctx, db.CreateWorkflowParams{
			WorkflowID: wfID, TenantID: s.Tenant, ProjectID: &car.ProjectID, Kind: "car", SubjectType: "capital_authorization", SubjectID: car.CarID.String(),
			InitiatedBy: actor.UserID, PolicySnapshot: snapshot,
		})
		if err != nil {
			return fmt.Errorf("create workflow: %w", err)
		}
		for i, st := range sub.chain {
			p := db.CreateStageParams{StageID: ids.New(), WorkflowID: wf.WorkflowID, StageOrder: st.Order, RoleRequired: st.Role, Assignees: assignees[i], SlaMinutes: int32(st.SLAHours * 60)}
			if i == 0 {
				due := now.Add(time.Duration(st.SLAHours) * time.Hour)
				p.EnteredAt, p.StageDueAt = &now, &due
			}
			if _, err := q.CreateStage(ctx, p); err != nil {
				return fmt.Errorf("create stage %d: %w", st.Order, err)
			}
		}
		fel := sub.project.FelStage
		after, err = q.SetCARInFlight(ctx, db.SetCARInFlightParams{TenantID: s.Tenant, CarID: car.CarID, WorkflowID: &wfID, FelStageAtSubmission: &fel, UpdatedBy: actor.UserID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: CAR %s was submitted concurrently", ErrInvalidState, car.Code)
		}
		if err != nil {
			return err
		}
		ev, err := s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &car.ProjectID, ActorUserID: actor.UserID, ActionType: "car.submit", SubjectType: "capital_authorization", SubjectID: car.CarID.String(),
			Before: car, After: map[string]any{"car": after, "governance": rec, "chain": sub.chain, "policy_snapshot": sub.policy}, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
			GuidanceID: guidanceID, ClassificationVerdict: res.Verdict,
		})
		if err != nil {
			return err
		}
		rec.AuditID = ev.AuditID.String()
		if _, err := q.InsertGuidanceOutcome(ctx, db.InsertGuidanceOutcomeParams{
			OutcomeID: ids.New(), TenantID: s.Tenant, AuditID: ev.AuditID, ProjectID: &car.ProjectID, UserID: actor.UserID, ActionType: ActionCAR,
			GuidanceIds: in.GuidanceIDs, GuidanceItemsSnapshot: seen, ClassificationVerdict: verdict, ViolatedCodes: rec.ViolatedCodes,
			FailTier: string(sub.tier), UserResponse: res.UserResponse, DismissReason: dismiss,
		}); err != nil {
			return fmt.Errorf("guidance outcome: %w", err)
		}
		feedback := mdemgadapter.OutcomeFollowed
		if res.UserResponse == governance.ResponseIgnoredWithReason || res.UserResponse == governance.ResponseDismissed {
			feedback = mdemgadapter.OutcomeIgnoredWithReason
		}
		if res.Verdict != "" { // no feedback when MDEMG never answered
			if err := s.enqueueFeedback(ctx, q, ev.AuditID, actor, car.ProjectID, in.GuidanceIDs, feedback, "CAR "+car.Code+" submitted; routed "+roleList(sub.chain)); err != nil {
				return err
			}
		}
		firstDue := now.Add(time.Duration(sub.chain[0].SLAHours) * time.Hour)
		if err := s.notifyTx(ctx, q, notify.Notification{
			Kind: notify.KindApprovalRequested, SubjectType: "capital_authorization", SubjectID: car.CarID.String(), Recipients: assignees[0],
			Subject:  fmt.Sprintf("CAR %s awaits your decision (stage 1, %s)", after.Code, sub.chain[0].Role),
			Body:     fmt.Sprintf("%s submitted CAR %s (%s) for %s %s on project %s (%s). You are stage 1 of %d (%s). The stage is due %s.", actor.UserID, after.Code, after.Kind, after.AmountRequested, after.Currency, sub.project.Code, sub.project.Name, len(sub.chain), sub.chain[0].Role, firstDue.Format(time.RFC1123)),
			DeepLink: "/cars/" + car.CarID.String(), Metadata: map[string]string{"code": after.Code, "project": sub.project.Code, "amount": after.AmountRequested + " " + after.Currency, "stage": "1"}, DueAt: &firstDue,
		}); err != nil {
			return err
		}
		return outbox.Enqueue(ctx, q, s.Tenant, outbox.KindMDEMGObserve, ev.AuditID.String(), mdemgadapter.ObserveRequest{
			SpaceID: mdemgadapter.ProjectSpace(car.ProjectID.String()), SessionID: sessionID(actor, car.ProjectID),
			Content: fmt.Sprintf("CAR %s (%s) for %s %s submitted on project %s by %s; routes %s; governance tier %s verdict %s (%s).", after.Code, after.Kind, after.AmountRequested, after.Currency, sub.project.Code, actor.UserID, roleList(sub.chain), sub.tier, res.Verdict, res.UserResponse),
			ObsType: "decision", Tags: []string{"car", "submitted", "kind:" + after.Kind, "tier:" + string(sub.tier)},
			StructuredData: map[string]any{"car_id": after.CarID.String(), "code": after.Code, "amount": after.AmountRequested, "currency": after.Currency, "routing_amount": money(sub.routing), "stages": len(sub.chain), "verdict": res.Verdict},
			SourceRef:      ev.AuditID.String(),
		})
	})
	if err != nil {
		return SubmitResult{}, err
	}
	view, err := s.view(ctx, after)
	return SubmitResult{View: view, Governance: rec}, err
}

func roleList(chain []StageDef) string {
	roles := make([]string, len(chain))
	for i, st := range chain {
		roles[i] = st.Role
	}
	return strings.Join(roles, " > ")
}

// enqueueFeedback writes one feedback row per guidance id the user saw (§06: the
// outcome goes to every space that contributed), keyed on the audit id.
func (s *Service) enqueueFeedback(ctx context.Context, q *db.Queries, auditID uuid.UUID, actor Actor, projectID uuid.UUID, guidanceIDs []string, outcome, summary string) error {
	for i, gid := range guidanceIDs {
		if gid == "" {
			continue
		}
		space := mdemgadapter.GovernanceSpace(s.gate().OrgID)
		if i > 0 {
			space = mdemgadapter.ProjectSpace(projectID.String())
		}
		if err := outbox.Enqueue(ctx, q, s.Tenant, outbox.KindMDEMGFeedback, fmt.Sprintf("%s:%d", auditID, i), mdemgadapter.FeedbackRequest{
			GuidanceID: gid, ActionSummary: summary, SpaceID: space, SessionID: sessionID(actor, projectID), Outcome: outcome,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- CAR: decisions

// DecideInput is POST /cars/{id}/decide.
type DecideInput struct {
	Decision   string  `json:"decision"` // approve | reject
	Comment    string  `json:"comment"`
	GuidanceID *string `json:"guidance_id"` // what the approver viewed, if anything
}

// DecideCAR records the current stage's decision by one of its assignees and
// advances or closes the workflow. Approvals beyond the last stage approve the
// CAR and authorize the requested amount; any rejection rejects the CAR.
func (s *Service) DecideCAR(ctx context.Context, actor Actor, carID uuid.UUID, in DecideInput) (CARView, error) {
	if in.Decision != "approve" && in.Decision != "reject" {
		return CARView{}, validationError("decision", "approve or reject")
	}
	car, err := s.Store.Queries().GetCAR(ctx, db.GetCARParams{TenantID: s.Tenant, CarID: carID})
	if errors.Is(err, pgx.ErrNoRows) {
		return CARView{}, fmt.Errorf("%w: car %s", ErrNotFound, carID)
	}
	if err != nil {
		return CARView{}, err
	}
	if car.Status != "in_flight" || car.WorkflowID == nil {
		return CARView{}, fmt.Errorf("%w: CAR %s is %s", ErrInvalidState, car.Code, car.Status)
	}
	q := s.Store.Queries()
	wf, err := q.GetWorkflow(ctx, db.GetWorkflowParams{TenantID: s.Tenant, WorkflowID: *car.WorkflowID})
	if err != nil {
		return CARView{}, err
	}
	stages, err := q.ListStages(ctx, wf.WorkflowID)
	if err != nil {
		return CARView{}, err
	}
	var current *db.ApprovalStage
	for i := range stages {
		if stages[i].StageOrder == wf.CurrentStage {
			current = &stages[i]
		}
	}
	if current == nil || current.CompletedAt != nil {
		return CARView{}, fmt.Errorf("%w: workflow %s has no open stage", ErrInvalidState, wf.WorkflowID)
	}
	eligible := false
	for _, a := range current.Assignees {
		if a == actor.UserID {
			eligible = true
		}
	}
	if !eligible {
		return CARView{}, fmt.Errorf("%w: %s is not an assignee of stage %d (%s)", ErrForbidden, actor.UserID, current.StageOrder, current.RoleRequired)
	}
	last := current.StageOrder == stages[len(stages)-1].StageOrder
	now := time.Now().UTC()
	var after db.CapitalAuthorization
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		var comment *string
		if c := strings.TrimSpace(in.Comment); c != "" {
			comment = &c
		}
		tier := string(governance.TierC) // stage decisions are not classified in the toy; the submit was
		stage, err := q.DecideStage(ctx, db.DecideStageParams{WorkflowID: wf.WorkflowID, StageOrder: current.StageOrder, CompletedBy: &actor.UserID, Decision: &in.Decision, Comment: comment, GuidanceID: in.GuidanceID, FailTier: &tier})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: stage %d was decided concurrently", ErrInvalidState, current.StageOrder)
		}
		if err != nil {
			return err
		}
		action := "car.stage.approve"
		after = car
		switch {
		case in.Decision == "reject":
			action = "car.reject"
			if err := q.CompleteWorkflow(ctx, db.CompleteWorkflowParams{TenantID: s.Tenant, WorkflowID: wf.WorkflowID, Status: "rejected"}); err != nil {
				return err
			}
			if after, err = q.SetCAROutcome(ctx, db.SetCAROutcomeParams{TenantID: s.Tenant, CarID: car.CarID, Status: "rejected", UpdatedBy: actor.UserID}); err != nil {
				return err
			}
		case last:
			action = "car.approve"
			if err := q.CompleteWorkflow(ctx, db.CompleteWorkflowParams{TenantID: s.Tenant, WorkflowID: wf.WorkflowID, Status: "approved"}); err != nil {
				return err
			}
			amt := car.AmountRequested
			if after, err = q.SetCAROutcome(ctx, db.SetCAROutcomeParams{TenantID: s.Tenant, CarID: car.CarID, Status: "approved", AuthorizedAmount: &amt, AuthorizedAt: &now, UpdatedBy: actor.UserID}); err != nil {
				return err
			}
		default:
			next := current.StageOrder + 1
			if err := q.AdvanceWorkflow(ctx, db.AdvanceWorkflowParams{TenantID: s.Tenant, WorkflowID: wf.WorkflowID, CurrentStage: next}); err != nil {
				return err
			}
			var sla int32
			for _, st := range stages {
				if st.StageOrder == next {
					sla = st.SlaMinutes
				}
			}
			due := now.Add(time.Duration(sla) * time.Minute)
			if err := q.OpenStage(ctx, db.OpenStageParams{WorkflowID: wf.WorkflowID, StageOrder: next, EnteredAt: &now, StageDueAt: &due}); err != nil {
				return err
			}
			for _, st := range stages {
				if st.StageOrder == next {
					if err := s.notifyTx(ctx, q, notify.Notification{
						Kind: notify.KindApprovalRequested, SubjectType: "capital_authorization", SubjectID: car.CarID.String(), Recipients: st.Assignees,
						Subject:  fmt.Sprintf("CAR %s awaits your decision (stage %d, %s)", car.Code, next, st.RoleRequired),
						Body:     fmt.Sprintf("%s approved stage %d of CAR %s (%s %s). You are stage %d of %d (%s). The stage is due %s.", actor.UserID, current.StageOrder, car.Code, car.AmountRequested, car.Currency, next, len(stages), st.RoleRequired, due.Format(time.RFC1123)),
						DeepLink: "/cars/" + car.CarID.String(), Metadata: map[string]string{"code": car.Code, "stage": fmt.Sprint(next)}, DueAt: &due,
					}); err != nil {
						return err
					}
				}
			}
		}
		gid := ""
		if in.GuidanceID != nil {
			gid = *in.GuidanceID
		}
		ev, err := s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &car.ProjectID, ActorUserID: actor.UserID, ActionType: action, SubjectType: "capital_authorization", SubjectID: car.CarID.String(),
			Before: car, After: map[string]any{"car": after, "stage": stage}, SourceIP: actor.SourceIP, RequestID: actor.RequestID, GuidanceID: gid,
		})
		if err != nil {
			return err
		}
		if action == "car.stage.approve" {
			return nil
		}
		if action == "car.approve" {
			// §08: DocuSign at or above car.esign.required_above (from the workflow's policy
			// snapshot), the audited approval itself as the attestation of record below it.
			// Re-read the stages so the decision just recorded is among the signers.
			signed, err := q.ListStages(ctx, wf.WorkflowID)
			if err != nil {
				return err
			}
			if err := s.signOrAttest(ctx, q, actor, &after, wf, signed, ev); err != nil {
				return err
			}
		}
		// The owner hears the outcome (§08 workflow events).
		verb := "approved"
		if action == "car.reject" {
			verb = "rejected"
		}
		if err := s.notifyTx(ctx, q, notify.Notification{
			Kind: notify.KindApprovalDecided, SubjectType: "capital_authorization", SubjectID: car.CarID.String(), Recipients: []string{wf.InitiatedBy},
			Subject:  fmt.Sprintf("CAR %s %s", car.Code, verb),
			Body:     fmt.Sprintf("CAR %s (%s %s) was %s by %s at stage %d (%s).", car.Code, car.AmountRequested, car.Currency, verb, actor.UserID, current.StageOrder, current.RoleRequired),
			DeepLink: "/cars/" + car.CarID.String(), Metadata: map[string]string{"code": car.Code, "outcome": after.Status},
		}); err != nil {
			return err
		}
		return outbox.Enqueue(ctx, q, s.Tenant, outbox.KindMDEMGObserve, ev.AuditID.String(), mdemgadapter.ObserveRequest{
			SpaceID: mdemgadapter.ProjectSpace(car.ProjectID.String()), SessionID: sessionID(actor, car.ProjectID),
			Content: fmt.Sprintf("CAR %s (%s) %s by %s at stage %d (%s); amount %s %s.", car.Code, car.Kind, after.Status, actor.UserID, current.StageOrder, current.RoleRequired, car.AmountRequested, car.Currency),
			ObsType: "decision", Tags: []string{"car", after.Status, "kind:" + car.Kind},
			StructuredData: map[string]any{"car_id": car.CarID.String(), "code": car.Code, "status": after.Status, "stage": current.StageOrder, "role": current.RoleRequired},
			SourceRef:      ev.AuditID.String(),
		})
	})
	if err != nil {
		return CARView{}, err
	}
	return s.view(ctx, after)
}

// InboxItem is one stage waiting on the actor.
type InboxItem struct {
	db.OpenStagesForUserRow
	Overdue  bool   `json:"overdue"`
	SLAState string `json:"sla_state"` // ok | nudged | overdue | escalated (§08 SLA tick)
}

// Inbox lists the open stages assigned to the actor, soonest due first.
func (s *Service) Inbox(ctx context.Context, actor Actor) ([]InboxItem, error) {
	rows, err := s.Store.Queries().OpenStagesForUser(ctx, db.OpenStagesForUserParams{TenantID: s.Tenant, UserID: actor.UserID})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	items := make([]InboxItem, 0, len(rows))
	for _, r := range rows {
		state := "ok"
		switch {
		case r.EscalatedAt != nil:
			state = "escalated"
		case r.OverdueAt != nil || (r.StageDueAt != nil && r.StageDueAt.Before(now)):
			state = "overdue"
		case r.NudgedAt != nil:
			state = "nudged"
		}
		items = append(items, InboxItem{OpenStagesForUserRow: r, Overdue: state == "overdue" || state == "escalated", SLAState: state})
	}
	return items, nil
}

// GuidanceOutcomes lists what users saw and did on a project's governed actions.
func (s *Service) GuidanceOutcomes(ctx context.Context, projectID uuid.UUID, limit int32) ([]db.GuidanceOutcome, error) {
	return s.Store.Queries().ListGuidanceOutcomesForProject(ctx, db.ListGuidanceOutcomesForProjectParams{TenantID: s.Tenant, ProjectID: &projectID, Limit: limit})
}

// AddPoolMember is the dev/admin path until IdPi claims mapping fills pools (§04).
func (s *Service) AddPoolMember(ctx context.Context, actor Actor, pool, userID string, siteID *uuid.UUID) error {
	pool, userID = strings.TrimSpace(pool), strings.TrimSpace(userID)
	if pool == "" || userID == "" {
		return validationError("pool", "pool and user_id are required")
	}
	return s.Store.Tx(ctx, func(q *db.Queries) error {
		if err := q.UpsertPoolMember(ctx, db.UpsertPoolMemberParams{Pool: pool, UserID: userID, SiteID: siteID, AddedBy: actor.UserID}); err != nil {
			return err
		}
		_, err := s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ActorUserID: actor.UserID, ActionType: "pool.add_member", SubjectType: "approver_pool", SubjectID: pool,
			After: map[string]any{"pool": pool, "user_id": userID, "site_id": siteID}, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
}

// signOrAttest decides the signature path for an approved CAR (FSD §08).
func (s *Service) signOrAttest(ctx context.Context, q *db.Queries, actor Actor, car *db.CapitalAuthorization, wf db.ApprovalWorkflow, stages []db.ApprovalStage, approval db.AuditEvent) error {
	threshold := esignThreshold(wf.PolicySnapshot)
	amount, _ := new(big.Rat).SetString(car.AmountRequested)
	if s.Signing == nil || threshold == nil || amount.Cmp(threshold) < 0 {
		// Internal attestation: the approval's audit row is the record.
		ref := "attestation:" + approval.AuditID.String()
		if err := q.SetCARSignedArtifact(ctx, db.SetCARSignedArtifactParams{TenantID: s.Tenant, CarID: car.CarID, SignedArtifactRef: &ref, UpdatedBy: actor.UserID}); err != nil {
			return err
		}
		car.SignedArtifactRef = &ref
		_, err := s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &car.ProjectID, ActorUserID: actor.UserID, ActionType: "car.attested", SubjectType: "capital_authorization", SubjectID: car.CarID.String(),
			After: map[string]any{"attestation": ref, "amount": car.AmountRequested, "threshold_esign": money(threshold)}, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	}
	// E-signature: every approver signs, in stage order.
	project, err := q.GetProject(ctx, db.GetProjectParams{TenantID: s.Tenant, ProjectID: car.ProjectID})
	if err != nil {
		return err
	}
	var signers []signing.Signer
	for _, st := range stages {
		if st.CompletedBy == nil || st.Decision == nil || *st.Decision != "approve" {
			continue
		}
		name, email := *st.CompletedBy, ""
		if u, err := q.GetUserByUsername(ctx, db.GetUserByUsernameParams{TenantID: s.Tenant, Username: *st.CompletedBy}); err == nil {
			name, email = u.DisplayName, u.Email
		}
		signers = append(signers, signing.Signer{UserID: *st.CompletedBy, Email: email, Name: name, Order: st.StageOrder, Role: st.RoleRequired})
	}
	title := fmt.Sprintf("CAR %s (%s) %s %s, project %s", car.Code, car.Kind, car.AmountRequested, car.Currency, project.Code)
	doc := fmt.Sprintf("Capital Authorization Request %s\nProject %s: %s\nKind: %s\nAmount: %s %s (basis %s)\nApproved %s\n\nBusiness case:\n%s",
		car.Code, project.Code, project.Name, car.Kind, car.AmountRequested, car.Currency, car.Basis, approval.At.UTC().Format(time.RFC3339), car.BusinessCase)
	pid := car.ProjectID
	envID, err := s.Signing.Request(ctx, q, signing.Request{
		ProjectID: &pid, SubjectType: "capital_authorization", SubjectID: car.CarID.String(), Title: title, Document: doc, Signers: signers,
		Metadata: map[string]string{"project": project.Code, "car": car.Code, "amount": car.AmountRequested + " " + car.Currency}, RequestedBy: actor.UserID, AuditID: approval.AuditID,
	}, func(kind, sourceRef string, payload any) error {
		return outbox.Enqueue(ctx, q, s.Tenant, kind, sourceRef, payload)
	})
	if err != nil {
		return err
	}
	ref := "envelope:" + envID.String() + ":pending"
	if err := q.SetCARSignedArtifact(ctx, db.SetCARSignedArtifactParams{TenantID: s.Tenant, CarID: car.CarID, SignedArtifactRef: &ref, UpdatedBy: actor.UserID}); err != nil {
		return err
	}
	handles := make([]string, 0, len(signers))
	for _, sg := range signers {
		handles = append(handles, sg.UserID)
	}
	if err := s.notifyTx(ctx, q, notify.Notification{
		Kind: notify.KindSignatureRequest, SubjectType: "capital_authorization", SubjectID: car.CarID.String(), Recipients: handles,
		Subject:  fmt.Sprintf("Signature requested: CAR %s", car.Code),
		Body:     fmt.Sprintf("CAR %s (%s %s, project %s) is approved and at or above the e-signature threshold. An envelope is being prepared with you as a signer; you will receive the provider's signing request.", car.Code, car.AmountRequested, car.Currency, project.Code),
		DeepLink: "/cars/" + car.CarID.String(), Metadata: map[string]string{"code": car.Code, "envelope_id": envID.String()},
	}); err != nil {
		return err
	}
	car.SignedArtifactRef = &ref
	_, err = s.Chain.Append(ctx, q, audit.Event{
		TenantID: s.Tenant, ProjectID: &car.ProjectID, ActorUserID: actor.UserID, ActionType: "car.signature_requested", SubjectType: "capital_authorization", SubjectID: car.CarID.String(),
		After: map[string]any{"envelope_id": envID, "signers": signers, "amount": car.AmountRequested, "threshold_esign": money(threshold)}, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
	})
	return err
}

// esignThreshold reads car.esign.required_above from a workflow's policy snapshot;
// nil when the snapshot predates the key (then the approval is attested).
func esignThreshold(snapshot []byte) *big.Rat {
	var m struct {
		ThresholdESign string `json:"threshold_esign"`
	}
	if err := json.Unmarshal(snapshot, &m); err != nil || m.ThresholdESign == "" {
		return nil
	}
	x, ok := new(big.Rat).SetString(m.ThresholdESign)
	if !ok {
		return nil
	}
	return x
}
