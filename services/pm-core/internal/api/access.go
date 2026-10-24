package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/reh3376/pro-manage/services/pm-core/internal/acl"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
)

// The ACL edge of FSD §04: every project-scoped handler names the section and
// permission it needs; authorize loads the caller's project context once and
// asks the evaluator. Denials are logged with (user, action, target, reason).
// Platform-level writes (sites, approver pools) need the platform_admin role.

// authorize answers whether the actor may do perm on section of the project;
// on refusal it has already written the 403 (or 404 for an unknown project).
func (a *API) authorize(w http.ResponseWriter, r *http.Request, projectID uuid.UUID, section, perm string) (domain.Actor, bool) {
	actor := actorFrom(r.Context())
	if _, err := a.Domain.GetProject(r.Context(), projectID); err != nil {
		a.fail(w, r, err)
		return actor, false
	}
	c, err := a.Domain.Access(r.Context(), actor, projectID)
	if err != nil {
		a.fail(w, r, err)
		return actor, false
	}
	d := acl.Decide(c, section, perm)
	if !d.Allowed {
		a.Log.Warn("access denied", "user", actor.UserID, "action", perm, "target", "project:"+projectID.String()+"/"+section, "reason", d.Reason, "request_id", requestIDFrom(r.Context()))
		writeError(w, http.StatusForbidden, "forbidden", "no "+perm+" access to the "+section+" section of this project ("+d.Reason+")")
		return actor, false
	}
	return actor, true
}

// requirePlatformRole gates platform-level routes.
func (a *API) requirePlatformRole(w http.ResponseWriter, r *http.Request, role string) (domain.Actor, bool) {
	actor := actorFrom(r.Context())
	if !acl.HasPlatformRole(actor.PlatformRoles, role) {
		a.Log.Warn("access denied", "user", actor.UserID, "action", r.Method+" "+r.URL.Path, "target", "platform", "reason", "needs "+role, "request_id", requestIDFrom(r.Context()))
		writeError(w, http.StatusForbidden, "forbidden", "the "+role+" platform role is required")
		return actor, false
	}
	return actor, true
}

func (a *API) mountAccess(mux *http.ServeMux, wrap func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/v1/projects/{project_id}/access", wrap(a.projectAccess))
	mux.Handle("GET /api/v1/projects/{project_id}/members", wrap(a.listMembers))
	mux.Handle("POST /api/v1/projects/{project_id}/members", wrap(a.addMember))
	mux.Handle("DELETE /api/v1/projects/{project_id}/members/{user_id}", wrap(a.removeMember))
	mux.Handle("GET /api/v1/projects/{project_id}/grants", wrap(a.listGrants))
	mux.Handle("POST /api/v1/projects/{project_id}/grants", wrap(a.addGrant))
	mux.Handle("DELETE /api/v1/projects/{project_id}/grants/{grant_id}", wrap(a.removeGrant))
}

// projectAccess: the caller's effective permission per section (for the UI and tests). Any member or assignee may ask.
func (a *API) projectAccess(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, err := a.Domain.GetProject(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	c, err := a.Domain.Access(r.Context(), actorFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if c.BaseRole == "" && !c.StageAssignee && !acl.Decide(c, acl.Overview, acl.Read).Allowed {
		writeError(w, http.StatusForbidden, "forbidden", "not a member of this project")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user_id": c.UserID, "base_role": c.BaseRole, "platform_roles": orEmpty(c.PlatformRoles), "stage_assignee": c.StageAssignee, "grants": c.Grants, "sections": acl.Effective(c)})
}

func (a *API) listMembers(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, ok := a.authorize(w, r, id, acl.Team, acl.Read); !ok {
		return
	}
	items, err := a.Domain.ListMembers(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) addMember(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	actor, ok := a.authorize(w, r, id, acl.Team, acl.Write)
	if !ok {
		return
	}
	var in domain.MemberInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	m, err := a.Domain.AddMember(r.Context(), actor, id, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, m)
}

func (a *API) removeMember(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	actor, ok := a.authorize(w, r, id, acl.Team, acl.Write)
	if !ok {
		return
	}
	if err := a.Domain.RemoveMember(r.Context(), actor, id, strings.TrimSpace(r.PathValue("user_id"))); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listGrants(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, ok := a.authorize(w, r, id, acl.Team, acl.Read); !ok {
		return
	}
	items, err := a.Domain.ListGrants(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) addGrant(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	actor, ok := a.authorize(w, r, id, acl.Team, acl.Write)
	if !ok {
		return
	}
	var in domain.GrantInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	g, err := a.Domain.AddGrant(r.Context(), actor, id, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, g)
}

func (a *API) removeGrant(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	gid, err := pathUUID(r, "grant_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	actor, ok := a.authorize(w, r, id, acl.Team, acl.Write)
	if !ok {
		return
	}
	if err := a.Domain.RemoveGrant(r.Context(), actor, id, gid); err != nil {
		a.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// auditSummary is the projection of an AuditEvent for callers without read_full
// on the audit section (§04: editors see the summary, not before/after).
type auditSummary struct {
	AuditID     uuid.UUID `json:"audit_id"`
	Seq         int64     `json:"seq"`
	ActorUserID string    `json:"actor_user_id"`
	ActionType  string    `json:"action_type"`
	SubjectType string    `json:"subject_type"`
	SubjectID   string    `json:"subject_id"`
	At          string    `json:"at"`
	GuidanceID  *string   `json:"guidance_id,omitempty"`
	Verdict     *string   `json:"classification_verdict,omitempty"`
	Summary     bool      `json:"summary"`
}

func summarize(rows []db.AuditEvent) []auditSummary {
	out := make([]auditSummary, 0, len(rows))
	for _, e := range rows {
		out = append(out, auditSummary{AuditID: e.AuditID, Seq: e.Seq, ActorUserID: e.ActorUserID, ActionType: e.ActionType, SubjectType: e.SubjectType, SubjectID: e.SubjectID, At: e.At.UTC().Format("2006-01-02T15:04:05.000000Z"), GuidanceID: e.GuidanceID, Verdict: e.ClassificationVerdict, Summary: true})
	}
	return out
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

var _ = json.Marshal
