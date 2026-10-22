// Package api is pm-core's REST edge (FSD §05): JSON in and out, one error
// envelope, request ids, and the actor resolution that the ACL layer (§04)
// will replace when IdPi_OIDC lands. Every handler is covered by a UATS spec
// (docs/tests/uats/specs/pm-core) and adjudicated in the routes inventory.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/policy"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

// API holds the dependencies handlers need.
type API struct {
	Log            *slog.Logger
	Domain         *domain.Service
	Chain          *audit.Chain
	MDEMG          *mdemgadapter.Client // nil when MDEMG_BASE_URL is unset
	MDEMGBaseURL   string
	DevActorHeader bool // when true, X-Dev-Actor names the acting user (development only; replaced by IdPi_OIDC)
}

type ctxKey int

const actorKey ctxKey = 1

// Mount registers /api/v1 routes on mux.
func (a *API) Mount(mux *http.ServeMux) {
	wrap := func(h http.HandlerFunc) http.Handler { return a.requestID(a.requireActor(h)) }
	mux.Handle("GET /api/v1/sites", wrap(a.listSites))
	mux.Handle("POST /api/v1/sites", wrap(a.createSite))
	mux.Handle("GET /api/v1/sites/{site_id}", wrap(a.getSite))
	mux.Handle("GET /api/v1/projects", wrap(a.listProjects))
	mux.Handle("POST /api/v1/projects", wrap(a.createProject))
	mux.Handle("GET /api/v1/projects/{project_id}", wrap(a.getProject))
	mux.Handle("GET /api/v1/projects/{project_id}/audit", wrap(a.projectAudit))
	mux.Handle("GET /api/v1/projects/{project_id}/policy/{key}", wrap(a.projectPolicy))
	mux.Handle("GET /api/v1/policy-variables", wrap(a.listPolicyVariables))
	mux.Handle("GET /api/v1/audit/verify", wrap(a.verifyAudit))
	a.mountMDEMG(mux, wrap)
	a.mountOutbox(mux, wrap)
	a.mountCAR(mux, wrap)
}

// ---------------------------------------------------------------- middleware

func (a *API) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = ids.New().String()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey(2), id)))
	})
}

// requireActor resolves who is calling. Until SSO lands (IdPi_OIDC slice) the
// only supported mode is the development header; with the header disabled every
// /api/v1 call is 401, which is the safe default for any non-dev deployment.
func (a *API) requireActor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.DevActorHeader {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "sign-in is not available yet; PM_DEV_ACTOR_HEADER is off")
			return
		}
		user := strings.TrimSpace(r.Header.Get("X-Dev-Actor"))
		if user == "" {
			writeError(w, http.StatusUnauthorized, "unauthenticated", "X-Dev-Actor header required in development mode")
			return
		}
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		actor := domain.Actor{UserID: user, SourceIP: host, RequestID: requestIDFrom(r.Context())}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, actor)))
	})
}

func actorFrom(ctx context.Context) domain.Actor {
	a, _ := ctx.Value(actorKey).(domain.Actor)
	return a
}

func requestIDFrom(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey(2)).(string)
	return s
}

// ---------------------------------------------------------------- responses

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var b errorBody
	b.Error.Code = code
	b.Error.Message = msg
	httpx.WriteJSON(w, status, b)
}

func (a *API) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		writeError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error())
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, policy.ErrUnknownVariable):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, domain.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, domain.ErrPrecondition):
		writeError(w, http.StatusUnprocessableEntity, "precondition_failed", err.Error())
	case errors.Is(err, domain.ErrInvalidState):
		writeError(w, http.StatusConflict, "invalid_state", err.Error())
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", err.Error())
	default:
		a.Log.Error("request failed", "path", r.URL.Path, "request_id", requestIDFrom(r.Context()), "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal error; see request_id in logs")
	}
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errors.Join(domain.ErrValidation, errors.New("body: "+err.Error()))
	}
	return nil
}

func pathUUID(r *http.Request, name string) (uuid.UUID, error) {
	u, err := ids.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, errors.Join(domain.ErrValidation, errors.New(name+": uuid required"))
	}
	return u, nil
}

// ---------------------------------------------------------------- sites

func (a *API) listSites(w http.ResponseWriter, r *http.Request) {
	items, err := a.Domain.ListSites(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createSite(w http.ResponseWriter, r *http.Request) {
	var in domain.CreateSiteInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	site, err := a.Domain.CreateSite(r.Context(), actorFrom(r.Context()), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, site)
}

func (a *API) getSite(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "site_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	site, err := a.Domain.GetSite(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, site)
}

// ---------------------------------------------------------------- projects

func (a *API) listProjects(w http.ResponseWriter, r *http.Request) {
	var siteID *uuid.UUID
	if s := r.URL.Query().Get("site_id"); s != "" {
		u, err := ids.Parse(s)
		if err != nil {
			a.fail(w, r, errors.Join(domain.ErrValidation, errors.New("site_id: uuid required")))
			return
		}
		siteID = &u
	}
	items, err := a.Domain.ListProjects(r.Context(), siteID)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createProject(w http.ResponseWriter, r *http.Request) {
	var in domain.CreateProjectInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	p, err := a.Domain.CreateProject(r.Context(), actorFrom(r.Context()), in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

func (a *API) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p, err := a.Domain.GetProject(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, p)
}

func (a *API) projectAudit(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, err := a.Domain.GetProject(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	limit := int32(50)
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 500 {
			limit = int32(n)
		}
	}
	items, err := a.Domain.ProjectAudit(r.Context(), id, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) projectPolicy(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p, err := a.Domain.GetProject(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	scope := policy.Scope{ProjectID: p.ProjectID.String(), SiteID: p.SiteID.String(), ProjectType: p.ProjectType}
	if p.PortfolioID != nil {
		scope.PortfolioID = p.PortfolioID.String()
	}
	res, err := policy.Resolve(r.Context(), a.Domain.Store.Queries(), r.PathValue("key"), scope, time.Now().UTC())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// ---------------------------------------------------------------- policy + audit

func (a *API) listPolicyVariables(w http.ResponseWriter, r *http.Request) {
	items, err := a.Domain.Store.Queries().ListPolicyVariables(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) verifyAudit(w http.ResponseWriter, r *http.Request) {
	res, err := a.Chain.Verify(r.Context(), a.Domain.Store.Queries(), a.Domain.Tenant)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	status := http.StatusOK
	if !res.Valid {
		status = http.StatusConflict
	}
	httpx.WriteJSON(w, status, res)
}
