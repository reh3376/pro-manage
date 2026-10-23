package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
)

// LoginInput is what a completed SSO login tells the domain (FSD §04).
type LoginInput struct {
	Subject       string
	Username      string
	Email         string
	DisplayName   string
	PlatformRoles []string
	Pools         []string
	Attributes    map[string]string
	SourceIP      string
	RequestID     string
}

// RecordLogin provisions or refreshes the user just in time, replaces the
// IdP-sourced approver-pool rows with the pools the claims resolved to (seed
// and manual rows are left alone), and audits the login. The audit actor and
// the pool rows key on the login handle, which is what every other row in
// the spine keys on today (backlog B-11 re-keys attribution to user_id).
func (s *Service) RecordLogin(ctx context.Context, in LoginInput) (db.User, error) {
	in.Subject, in.Username = strings.TrimSpace(in.Subject), strings.TrimSpace(in.Username)
	if in.Subject == "" || in.Username == "" {
		return db.User{}, validationError("subject", "subject and username are required")
	}
	if in.PlatformRoles == nil {
		in.PlatformRoles = []string{}
	}
	if in.Attributes == nil {
		in.Attributes = map[string]string{}
	}
	attrs, _ := jsonMarshal(in.Attributes)
	var siteID *uuid.UUID
	if code := strings.ToUpper(strings.TrimSpace(in.Attributes["site"])); code != "" {
		site, err := s.Store.Queries().GetSiteByCode(ctx, db.GetSiteByCodeParams{TenantID: s.Tenant, Code: code})
		if err == nil {
			siteID = &site.SiteID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, err
		}
	}
	var user db.User
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		var err error
		user, err = q.UpsertUserOnLogin(ctx, db.UpsertUserOnLoginParams{
			UserID: ids.New(), TenantID: s.Tenant, ExternalID: in.Subject, Username: in.Username, Email: in.Email, DisplayName: in.DisplayName,
			PlatformRoles: in.PlatformRoles, Attributes: attrs,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("%w: username %s belongs to another identity", ErrConflict, in.Username)
			}
			return fmt.Errorf("upsert user: %w", err)
		}
		if err := q.DeleteIdpPoolRows(ctx, in.Username); err != nil {
			return err
		}
		for _, pool := range in.Pools {
			if pool = strings.TrimSpace(pool); pool == "" {
				continue
			}
			if err := q.InsertPoolRow(ctx, db.InsertPoolRowParams{Pool: pool, UserID: in.Username, SiteID: siteID, AddedBy: "idp", Source: "idp"}); err != nil {
				return fmt.Errorf("pool %s: %w", pool, err)
			}
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ActorUserID: in.Username, ActionType: "user.login", SubjectType: "user", SubjectID: user.UserID.String(),
			After:    map[string]any{"username": in.Username, "platform_roles": in.PlatformRoles, "pools": in.Pools, "attributes": in.Attributes, "login_count": user.LoginCount},
			SourceIP: in.SourceIP, RequestID: in.RequestID,
		})
		return err
	})
	return user, err
}

// RecordLogout audits the end of a session.
func (s *Service) RecordLogout(ctx context.Context, actor Actor, sessionID string) error {
	return s.Store.Tx(ctx, func(q *db.Queries) error {
		_, err := s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ActorUserID: actor.UserID, ActionType: "user.logout", SubjectType: "session", SubjectID: sessionID,
			After: map[string]any{"username": actor.UserID}, SourceIP: actor.SourceIP, RequestID: actor.RequestID,
		})
		return err
	})
}

// GetUserByUsername for /auth/me and admin views.
func (s *Service) GetUserByUsername(ctx context.Context, username string) (db.User, error) {
	u, err := s.Store.Queries().GetUserByUsername(ctx, db.GetUserByUsernameParams{TenantID: s.Tenant, Username: username})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, fmt.Errorf("%w: user %s", ErrNotFound, username)
	}
	return u, err
}
