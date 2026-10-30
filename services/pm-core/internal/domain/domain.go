// Package domain holds the spine's domain services: Site and Project today,
// each mutation written with its AuditEvent in one transaction (FSD §03, §08).
package domain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/budget"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/governance"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/notify"
	"github.com/reh3376/pro-manage/services/pm-core/internal/outbox"
	"github.com/reh3376/pro-manage/services/pm-core/internal/signing"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

// Sentinel errors the API maps to HTTP statuses.
var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("already exists")
	ErrValidation = errors.New("validation")
	// ErrPrecondition: a deterministic rule of the FSD blocks the action (FEL stage,
	// estimate class, promises, policy load). 422 with code precondition_failed.
	ErrPrecondition = errors.New("precondition")
	// ErrInvalidState: the subject is not in a state that allows the action. 409.
	ErrInvalidState = errors.New("invalid state")
	// ErrForbidden: the actor is not eligible for the action (not an assignee). 403.
	ErrForbidden = errors.New("forbidden")
	// ErrGovernanceDenied: MDEMG classified the submission as deny. 409 with the codes.
	ErrGovernanceDenied = errors.New("governance denied")
	// ErrGovernanceUnavailable: Tier A and MDEMG unreachable. 503.
	ErrGovernanceUnavailable = errors.New("governance unavailable")
)

// Actor is who performs a mutation, as resolved by the API layer.
type Actor struct {
	UserID        string     // the login handle (preferred_username): the display attribution and the key rows still carry
	ID            *uuid.UUID // users.user_id, the stable identity (B-11); nil for a dev-header caller with no users row
	Subject       string     // the IdP's stable id when the actor came from a session
	PlatformRoles []string   // §04 platform roles from the session; empty for the dev header
	SessionID     string
	SourceIP      string
	RequestID     string
}

// HasRole reports a platform role.
func (a Actor) HasRole(role string) bool {
	for _, r := range a.PlatformRoles {
		if r == role {
			return true
		}
	}
	return false
}

// Service is the spine's domain service.
type Service struct {
	Store  *store.Store
	Chain  *audit.Chain
	Tenant string
	// Gate is the MDEMG governance gate (§06). Nil means not configured: Tier A
	// governed actions refuse, Tier B proceed with a recorded warning.
	Gate *governance.Gate
	Log  *slog.Logger
	// Signing requests e-signature envelopes on final approvals (§08, §10); nil means
	// every approval is attested by its audit row alone.
	Signing *signing.Service
	// Notifier enqueues Commsi notifications for workflow events (§08, §10); nil means none are sent.
	Notifier *notify.Notifier
	// Budget requests the ERP budget release on final CAR approval (§08 "ERP-led profile", §10 ERPi);
	// nil or unconfigured means the pro_manage-led profile, where the approval is the whole record.
	Budget *budget.Service
}

// notifyTx enqueues a notification inside q's transaction; recipients without an email are logged, never fatal.
func (s *Service) notifyTx(ctx context.Context, q *db.Queries, n notify.Notification) error {
	if s.Notifier == nil {
		return nil
	}
	_, skipped, err := s.Notifier.Enqueue(ctx, q, n)
	if len(skipped) > 0 {
		s.logger().Warn("notification recipients without email skipped", "kind", n.Kind, "skipped", skipped)
	}
	return err
}

func (s *Service) gate() *governance.Gate {
	if s.Gate != nil {
		return s.Gate
	}
	return &governance.Gate{OrgID: "unconfigured", Log: s.logger()}
}

func (s *Service) logger() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

var codeRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{1,63}$`)

func validationError(field, msg string) error {
	return fmt.Errorf("%w: %s: %s", ErrValidation, field, msg)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ---------------------------------------------------------------- Sites

// CreateSiteInput is the API's request body.
type CreateSiteInput struct {
	Code               string         `json:"code"`
	Name               string         `json:"name"`
	PortfolioID        *string        `json:"portfolio_id"`
	Country            string         `json:"country"`
	TimeZone           string         `json:"time_zone"`
	Currency           string         `json:"currency"`
	UnitSystem         string         `json:"unit_system"`
	BusinessCalendarID *string        `json:"business_calendar_id"`
	RegulatoryRegime   []string       `json:"regulatory_regime"`
	PrivacyRegime      []string       `json:"privacy_regime"`
	Address            map[string]any `json:"address"`
}

func (in *CreateSiteInput) validate() error {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.Country = strings.ToUpper(strings.TrimSpace(in.Country))
	in.Currency = strings.ToUpper(strings.TrimSpace(in.Currency))
	in.TimeZone = strings.TrimSpace(in.TimeZone)
	if !codeRe.MatchString(in.Code) {
		return validationError("code", "2 to 64 characters: A-Z, 0-9, '.', '_', '-'")
	}
	if in.Name == "" {
		return validationError("name", "required")
	}
	if len(in.Country) != 2 {
		return validationError("country", "ISO 3166-1 alpha-2 code required")
	}
	if len(in.Currency) != 3 {
		return validationError("currency", "ISO 4217 code required")
	}
	if _, err := time.LoadLocation(in.TimeZone); err != nil || in.TimeZone == "" {
		return validationError("time_zone", "IANA time zone required")
	}
	if in.UnitSystem == "" {
		in.UnitSystem = "imperial"
	}
	if in.UnitSystem != "imperial" && in.UnitSystem != "metric" {
		return validationError("unit_system", "imperial or metric")
	}
	if in.RegulatoryRegime == nil {
		in.RegulatoryRegime = []string{}
	}
	if in.PrivacyRegime == nil {
		in.PrivacyRegime = []string{}
	}
	if in.Address == nil {
		in.Address = map[string]any{}
	}
	return nil
}

// CreateSite inserts a site and its audit event in one transaction.
func (s *Service) CreateSite(ctx context.Context, actor Actor, in CreateSiteInput) (db.Site, error) {
	if err := in.validate(); err != nil {
		return db.Site{}, err
	}
	portfolio, err := optionalUUID(in.PortfolioID, "portfolio_id")
	if err != nil {
		return db.Site{}, err
	}
	addr, _ := jsonMarshal(in.Address)
	var site db.Site
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		var err error
		site, err = q.CreateSite(ctx, db.CreateSiteParams{
			SiteID: ids.New(), TenantID: s.Tenant, Code: in.Code, Name: in.Name, PortfolioID: portfolio,
			Country: in.Country, TimeZone: in.TimeZone, Currency: in.Currency, UnitSystem: in.UnitSystem,
			BusinessCalendarID: in.BusinessCalendarID, RegulatoryRegime: in.RegulatoryRegime, PrivacyRegime: in.PrivacyRegime,
			Address: addr, CreatedBy: actor.UserID,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("%w: site code %s", ErrConflict, in.Code)
			}
			return fmt.Errorf("create site: %w", err)
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ActorUserID: actor.UserID, ActorID: actor.ID, ActionType: "site.create", SubjectType: "site", SubjectID: site.SiteID.String(),
			After: site, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
	return site, err
}

// GetSite by id.
func (s *Service) GetSite(ctx context.Context, id uuid.UUID) (db.Site, error) {
	site, err := s.Store.Queries().GetSite(ctx, db.GetSiteParams{TenantID: s.Tenant, SiteID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Site{}, fmt.Errorf("%w: site %s", ErrNotFound, id)
	}
	return site, err
}

// ListSites for the tenant.
func (s *Service) ListSites(ctx context.Context) ([]db.Site, error) {
	return s.Store.Queries().ListSites(ctx, s.Tenant)
}

// ---------------------------------------------------------------- Projects

// CreateProjectInput is the API's request body.
type CreateProjectInput struct {
	Code                        string   `json:"code"`
	Name                        string   `json:"name"`
	Description                 string   `json:"description"`
	PortfolioID                 *string  `json:"portfolio_id"`
	SiteID                      string   `json:"site_id"`
	Currency                    string   `json:"currency"`
	OwnerUserID                 string   `json:"owner_user_id"`
	FelStage                    string   `json:"fel_stage"`
	ProjectType                 string   `json:"project_type"`
	TpmPillarTags               []string `json:"tpm_pillar_tags"`
	HazardousAreaClassification string   `json:"hazardous_area_classification"`
	RegulatoryStamps            []string `json:"regulatory_stamps"`
	OnboardingLevel             string   `json:"onboarding_level"`
	StartDate                   *string  `json:"start_date"`
	TargetEndDate               *string  `json:"target_end_date"`
	BaselineBudgetTotal         *string  `json:"baseline_budget_total"`
	BaselineDurationDays        *int32   `json:"baseline_duration_days"`
	Tags                        []string `json:"tags"`
}

var (
	projectTypes   = set("greenfield", "brownfield_expansion", "reliability", "efficiency", "compliance", "capacity_debottleneck")
	felStages      = set("fel1", "fel2", "fel3", "n/a")
	hazClasses     = set("class_i_div_1", "class_i_div_2", "class_ii_div_1", "class_ii_div_2", "none")
	onboardingLvls = set("registered", "governed", "full")
	moneyRe        = regexp.MustCompile(`^\d{1,15}(\.\d{1,2})?$`)
)

func set(vals ...string) map[string]bool {
	m := map[string]bool{}
	for _, v := range vals {
		m[v] = true
	}
	return m
}

func (in *CreateProjectInput) validate(actor Actor, site db.Site) error {
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	if !codeRe.MatchString(in.Code) {
		return validationError("code", "2 to 64 characters: A-Z, 0-9, '.', '_', '-'")
	}
	if in.Name == "" {
		return validationError("name", "required")
	}
	if !projectTypes[in.ProjectType] {
		return validationError("project_type", "one of greenfield, brownfield_expansion, reliability, efficiency, compliance, capacity_debottleneck")
	}
	if in.FelStage == "" {
		in.FelStage = "fel1"
	}
	if !felStages[in.FelStage] {
		return validationError("fel_stage", "one of fel1, fel2, fel3, n/a")
	}
	if in.HazardousAreaClassification == "" {
		in.HazardousAreaClassification = "none"
	}
	if !hazClasses[in.HazardousAreaClassification] {
		return validationError("hazardous_area_classification", "invalid value")
	}
	if in.OnboardingLevel == "" {
		in.OnboardingLevel = "registered"
	}
	if !onboardingLvls[in.OnboardingLevel] {
		return validationError("onboarding_level", "one of registered, governed, full")
	}
	if in.Currency == "" {
		in.Currency = site.Currency
	}
	in.Currency = strings.ToUpper(in.Currency)
	if len(in.Currency) != 3 {
		return validationError("currency", "ISO 4217 code required")
	}
	if in.OwnerUserID == "" {
		in.OwnerUserID = actor.UserID
	}
	if in.BaselineBudgetTotal != nil && !moneyRe.MatchString(*in.BaselineBudgetTotal) {
		return validationError("baseline_budget_total", "decimal string with up to 2 places, e.g. 1500000.00")
	}
	if in.BaselineDurationDays != nil && *in.BaselineDurationDays < 0 {
		return validationError("baseline_duration_days", "must be >= 0")
	}
	for _, f := range []*[]string{&in.TpmPillarTags, &in.RegulatoryStamps, &in.Tags} {
		if *f == nil {
			*f = []string{}
		}
	}
	return nil
}

// CreateProject inserts a project and its audit event in one transaction.
func (s *Service) CreateProject(ctx context.Context, actor Actor, in CreateProjectInput) (db.Project, error) {
	siteID, err := ids.Parse(strings.TrimSpace(in.SiteID))
	if err != nil {
		return db.Project{}, validationError("site_id", "uuid required")
	}
	site, err := s.GetSite(ctx, siteID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return db.Project{}, validationError("site_id", "unknown site")
		}
		return db.Project{}, err
	}
	if err := in.validate(actor, site); err != nil {
		return db.Project{}, err
	}
	portfolio, err := optionalUUID(in.PortfolioID, "portfolio_id")
	if err != nil {
		return db.Project{}, err
	}
	start, err := optionalDate(in.StartDate, "start_date")
	if err != nil {
		return db.Project{}, err
	}
	end, err := optionalDate(in.TargetEndDate, "target_end_date")
	if err != nil {
		return db.Project{}, err
	}
	if start != nil && end != nil && end.Before(*start) {
		return db.Project{}, validationError("target_end_date", "must not precede start_date")
	}
	var project db.Project
	pid := ids.New()
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		var err error
		project, err = q.CreateProject(ctx, db.CreateProjectParams{
			ProjectID: pid, TenantID: s.Tenant, Code: in.Code, Name: in.Name, Description: in.Description,
			PortfolioID: portfolio, SiteID: site.SiteID, Currency: in.Currency, OwnerUserID: in.OwnerUserID,
			LifecycleState: "initiate", FelStage: in.FelStage, ProjectType: in.ProjectType, TpmPillarTags: in.TpmPillarTags,
			HazardousAreaClassification: in.HazardousAreaClassification, RegulatoryStamps: in.RegulatoryStamps,
			OnboardingLevel: in.OnboardingLevel, StartDate: start, TargetEndDate: end,
			BaselineBudgetTotal: in.BaselineBudgetTotal, BaselineDurationDays: in.BaselineDurationDays, Tags: in.Tags,
			CreatedBy: actor.UserID,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("%w: project code %s", ErrConflict, in.Code)
			}
			return fmt.Errorf("create project: %w", err)
		}
		// The owner is the first member (§03 ProjectMember, §04 base roles).
		if _, err := q.UpsertProjectMember(ctx, db.UpsertProjectMemberParams{ProjectID: pid, UserID: project.OwnerUserID, BaseRole: "owner", InvitedBy: actor.UserID}); err != nil {
			return fmt.Errorf("owner membership: %w", err)
		}
		// B-11: attach the stable identity when the owner has signed in before; otherwise the first login attaches it.
		if err := q.LinkProjectOwnerIdentity(ctx, pid); err != nil {
			return err
		}
		if err := q.LinkMemberIdentity(ctx, db.LinkMemberIdentityParams{ProjectID: pid, UserID: project.OwnerUserID, TenantID: s.Tenant}); err != nil {
			return err
		}
		ev, err := s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &pid, ActorUserID: actor.UserID, ActorID: actor.ID, ActionType: "project.create", SubjectType: "project",
			SubjectID: pid.String(), After: project, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		if err != nil {
			return err
		}
		// FSD §06 OBSERVE: project events go to the project space through the outbox,
		// in this transaction, keyed on the audit id so delivery is idempotent.
		return outbox.Enqueue(ctx, q, s.Tenant, outbox.KindMDEMGObserve, ev.AuditID.String(), mdemgadapter.ObserveRequest{
			SpaceID:        mdemgadapter.ProjectSpace(pid.String()),
			SessionID:      actor.UserID + ":" + pid.String(),
			Content:        fmt.Sprintf("Project %s (%s) created at site %s by %s: %s, type %s, FEL stage %s.", project.Code, project.Name, site.Code, actor.UserID, project.LifecycleState, project.ProjectType, project.FelStage),
			ObsType:        "decision",
			Tags:           []string{"project", "created", "type:" + project.ProjectType, "site:" + site.Code},
			StructuredData: map[string]any{"project_id": pid.String(), "code": project.Code, "site_id": site.SiteID.String(), "project_type": project.ProjectType, "lifecycle_state": project.LifecycleState},
			SourceRef:      ev.AuditID.String(),
		})
	})
	return project, err
}

// GetProject by id.
func (s *Service) GetProject(ctx context.Context, id uuid.UUID) (db.Project, error) {
	p, err := s.Store.Queries().GetProject(ctx, db.GetProjectParams{TenantID: s.Tenant, ProjectID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Project{}, fmt.Errorf("%w: project %s", ErrNotFound, id)
	}
	return p, err
}

// ListProjects, optionally filtered by site.
func (s *Service) ListProjects(ctx context.Context, siteID *uuid.UUID) ([]db.Project, error) {
	return s.Store.Queries().ListProjects(ctx, db.ListProjectsParams{TenantID: s.Tenant, SiteID: siteID})
}

// ---------------------------------------------------------------- helpers

func optionalUUID(s *string, field string) (*uuid.UUID, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	u, err := ids.Parse(strings.TrimSpace(*s))
	if err != nil {
		return nil, validationError(field, "uuid required")
	}
	return &u, nil
}

func optionalDate(s *string, field string) (*time.Time, error) {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", strings.TrimSpace(*s))
	if err != nil {
		return nil, validationError(field, "YYYY-MM-DD required")
	}
	return &t, nil
}
