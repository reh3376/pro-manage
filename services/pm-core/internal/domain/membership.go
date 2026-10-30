package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/reh3376/pro-manage/services/pm-core/internal/acl"
	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
)

var (
	baseRoles   = set(acl.Owner, acl.Editor, acl.Viewer, acl.Contributor)
	permissions = set(acl.Read, acl.Write, acl.ReadFull, acl.Deny)
	sections    = func() map[string]bool { return set(acl.Sections...) }()
)

// Access loads the caller's project context for the evaluator (FSD §04 "Grant
// evaluation" step 1): membership, unexpired grants, platform roles, and
// whether the caller sits on an open stage of an in-flight workflow.
func (s *Service) Access(ctx context.Context, actor Actor, projectID uuid.UUID) (acl.Caller, error) {
	q := s.Store.Queries()
	c := acl.Caller{UserID: actor.UserID, PlatformRoles: actor.PlatformRoles}
	m, err := q.GetProjectMember(ctx, db.GetProjectMemberParams{ProjectID: projectID, UserID: actor.UserID})
	switch {
	case err == nil:
		if m.ExpiresAt == nil || m.ExpiresAt.After(time.Now()) {
			c.BaseRole = m.BaseRole
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return c, err
	}
	grants, err := q.ListGrantsForUser(ctx, db.ListGrantsForUserParams{ProjectID: projectID, UserID: actor.UserID})
	if err != nil {
		return c, err
	}
	for _, g := range grants {
		c.Grants = append(c.Grants, acl.Grant{ScopeKind: g.ScopeKind, ScopeTarget: g.ScopeTarget, Permission: g.Permission})
	}
	if c.StageAssignee, err = q.IsOpenStageAssignee(ctx, db.IsOpenStageAssigneeParams{TenantID: s.Tenant, ProjectID: &projectID, Column3: actor.UserID}); err != nil {
		return c, err
	}
	return c, nil
}

// ListVisibleProjects: the projects the actor may see (membership, open stage,
// or a platform role that reads across projects), optionally filtered by site.
func (s *Service) ListVisibleProjects(ctx context.Context, actor Actor, siteID *uuid.UUID) ([]db.Project, error) {
	seeAll := acl.HasPlatformRole(actor.PlatformRoles, acl.PortfolioExecutive) || acl.HasPlatformRole(actor.PlatformRoles, acl.ComplianceOfficer)
	return s.Store.Queries().ListVisibleProjects(ctx, db.ListVisibleProjectsParams{TenantID: s.Tenant, SiteID: siteID, SeeAll: seeAll, UserID: actor.UserID})
}

// ---------------------------------------------------------------- members

// MemberInput is the API body for POST /projects/{id}/members.
type MemberInput struct {
	UserID    string  `json:"user_id"`
	BaseRole  string  `json:"base_role"`
	ExpiresAt *string `json:"expires_at"` // RFC 3339, optional
}

// AddMember invites or re-roles a member (team write; the API checks). A
// project keeps at least one owner.
func (s *Service) AddMember(ctx context.Context, actor Actor, projectID uuid.UUID, in MemberInput) (db.ProjectMember, error) {
	in.UserID = strings.TrimSpace(in.UserID)
	if in.UserID == "" {
		return db.ProjectMember{}, validationError("user_id", "required")
	}
	if !baseRoles[in.BaseRole] {
		return db.ProjectMember{}, validationError("base_role", "one of owner, editor, viewer, contributor")
	}
	var exp *time.Time
	if in.ExpiresAt != nil && strings.TrimSpace(*in.ExpiresAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*in.ExpiresAt))
		if err != nil {
			return db.ProjectMember{}, validationError("expires_at", "RFC 3339 timestamp required")
		}
		exp = &t
	}
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return db.ProjectMember{}, err
	}
	var m db.ProjectMember
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		before, err := q.GetProjectMember(ctx, db.GetProjectMemberParams{ProjectID: projectID, UserID: in.UserID})
		var prev any
		if err == nil {
			prev = before
			if before.BaseRole == acl.Owner && in.BaseRole != acl.Owner {
				n, err := q.CountProjectOwners(ctx, projectID)
				if err != nil {
					return err
				}
				if n <= 1 {
					return fmt.Errorf("%w: a project keeps at least one owner", ErrPrecondition)
				}
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		m, err = q.UpsertProjectMember(ctx, db.UpsertProjectMemberParams{ProjectID: projectID, UserID: in.UserID, BaseRole: in.BaseRole, InvitedBy: actor.UserID, ExpiresAt: exp})
		if err != nil {
			return fmt.Errorf("upsert member: %w", err)
		}
		if err := q.LinkMemberIdentity(ctx, db.LinkMemberIdentityParams{ProjectID: projectID, UserID: in.UserID, TenantID: s.Tenant}); err != nil {
			return err
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &projectID, ActorUserID: actor.UserID, ActorID: actor.ID, ActionType: "member.upsert", SubjectType: "project_member",
			SubjectID: projectID.String() + ":" + in.UserID, Before: prev, After: m, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
	return m, err
}

// RemoveMember drops a membership (not the last owner).
func (s *Service) RemoveMember(ctx context.Context, actor Actor, projectID uuid.UUID, userID string) error {
	return s.Store.Tx(ctx, func(q *db.Queries) error {
		before, err := q.GetProjectMember(ctx, db.GetProjectMemberParams{ProjectID: projectID, UserID: userID})
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: member %s", ErrNotFound, userID)
		}
		if err != nil {
			return err
		}
		if before.BaseRole == acl.Owner {
			n, err := q.CountProjectOwners(ctx, projectID)
			if err != nil {
				return err
			}
			if n <= 1 {
				return fmt.Errorf("%w: a project keeps at least one owner", ErrPrecondition)
			}
		}
		if _, err := q.DeleteProjectMember(ctx, db.DeleteProjectMemberParams{ProjectID: projectID, UserID: userID}); err != nil {
			return err
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &projectID, ActorUserID: actor.UserID, ActorID: actor.ID, ActionType: "member.remove", SubjectType: "project_member",
			SubjectID: projectID.String() + ":" + userID, Before: before, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
}

// ListMembers of a project.
func (s *Service) ListMembers(ctx context.Context, projectID uuid.UUID) ([]db.ProjectMember, error) {
	return s.Store.Queries().ListProjectMembers(ctx, projectID)
}

// ---------------------------------------------------------------- grants

// GrantInput is the API body for POST /projects/{id}/grants.
type GrantInput struct {
	UserID      string  `json:"user_id"`
	ScopeKind   string  `json:"scope_kind"`   // section (document and field are layers; accepted and stored, not yet enforced)
	ScopeTarget string  `json:"scope_target"` // section:<name>
	Permission  string  `json:"permission"`   // read | write | read_full | deny
	ExpiresAt   *string `json:"expires_at"`
	Reason      string  `json:"reason"`
}

// AddGrant records an explicit grant on top of the base role.
func (s *Service) AddGrant(ctx context.Context, actor Actor, projectID uuid.UUID, in GrantInput) (db.AclGrant, error) {
	in.UserID = strings.TrimSpace(in.UserID)
	if in.UserID == "" {
		return db.AclGrant{}, validationError("user_id", "required")
	}
	if in.ScopeKind == "" {
		in.ScopeKind = "section"
	}
	if in.ScopeKind != "section" && in.ScopeKind != "document" && in.ScopeKind != "field" {
		return db.AclGrant{}, validationError("scope_kind", "one of section, document, field")
	}
	if in.ScopeKind == "section" {
		name := strings.TrimPrefix(in.ScopeTarget, "section:")
		if !sections[name] {
			return db.AclGrant{}, validationError("scope_target", "section:<name> with a known section: "+strings.Join(acl.Sections, ", "))
		}
		in.ScopeTarget = "section:" + name
	} else if !strings.HasPrefix(in.ScopeTarget, in.ScopeKind+":") {
		return db.AclGrant{}, validationError("scope_target", in.ScopeKind+":<target> required")
	}
	if !permissions[in.Permission] {
		return db.AclGrant{}, validationError("permission", "one of read, write, read_full, deny")
	}
	if in.Permission == acl.ReadFull && in.ScopeTarget != "section:audit" {
		return db.AclGrant{}, validationError("permission", "read_full applies to section:audit only")
	}
	var exp *time.Time
	if in.ExpiresAt != nil && strings.TrimSpace(*in.ExpiresAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*in.ExpiresAt))
		if err != nil {
			return db.AclGrant{}, validationError("expires_at", "RFC 3339 timestamp required")
		}
		exp = &t
	}
	if _, err := s.GetProject(ctx, projectID); err != nil {
		return db.AclGrant{}, err
	}
	var g db.AclGrant
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		var err error
		g, err = q.InsertGrant(ctx, db.InsertGrantParams{GrantID: ids.New(), ProjectID: projectID, UserID: in.UserID, ScopeKind: in.ScopeKind, ScopeTarget: in.ScopeTarget, Permission: in.Permission, GrantedBy: actor.UserID, ExpiresAt: exp, Reason: strings.TrimSpace(in.Reason)})
		if err != nil {
			return fmt.Errorf("insert grant: %w", err)
		}
		if err := q.LinkGrantIdentity(ctx, db.LinkGrantIdentityParams{GrantID: g.GrantID, TenantID: s.Tenant}); err != nil {
			return err
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &projectID, ActorUserID: actor.UserID, ActorID: actor.ID, ActionType: "grant.add", SubjectType: "acl_grant",
			SubjectID: g.GrantID.String(), After: g, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
	return g, err
}

// RemoveGrant deletes a grant.
func (s *Service) RemoveGrant(ctx context.Context, actor Actor, projectID, grantID uuid.UUID) error {
	return s.Store.Tx(ctx, func(q *db.Queries) error {
		before, err := q.GetGrant(ctx, grantID)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && before.ProjectID != projectID) {
			return fmt.Errorf("%w: grant %s", ErrNotFound, grantID)
		}
		if err != nil {
			return err
		}
		if _, err := q.DeleteGrant(ctx, db.DeleteGrantParams{GrantID: grantID, ProjectID: projectID}); err != nil {
			return err
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: &projectID, ActorUserID: actor.UserID, ActorID: actor.ID, ActionType: "grant.remove", SubjectType: "acl_grant",
			SubjectID: grantID.String(), Before: before, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
}

// ListGrants of a project.
func (s *Service) ListGrants(ctx context.Context, projectID uuid.UUID) ([]db.AclGrant, error) {
	return s.Store.Queries().ListProjectGrants(ctx, projectID)
}
