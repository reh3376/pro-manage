package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
)

// mountCAR registers the toy CAR surface (FSD §03 CapitalAuthorization, §06
// governed action, §08 chain). Every route is covered by
// docs/tests/uats/specs/pm-core/car.uats.json and scripts/car_check.sh.
func (a *API) mountCAR(mux *http.ServeMux, wrap func(http.HandlerFunc) http.Handler) {
	mux.Handle("POST /api/v1/projects/{project_id}/estimates", wrap(a.createEstimate))
	mux.Handle("GET /api/v1/projects/{project_id}/promises", wrap(a.listPromises))
	mux.Handle("POST /api/v1/projects/{project_id}/promises", wrap(a.createPromise))
	mux.Handle("GET /api/v1/projects/{project_id}/cars", wrap(a.listCARs))
	mux.Handle("POST /api/v1/projects/{project_id}/cars", wrap(a.createCAR))
	mux.Handle("GET /api/v1/projects/{project_id}/guidance-outcomes", wrap(a.guidanceOutcomes))
	mux.Handle("GET /api/v1/cars/{car_id}", wrap(a.getCAR))
	mux.Handle("GET /api/v1/cars/{car_id}/guidance", wrap(a.carGuidance))
	mux.Handle("POST /api/v1/cars/{car_id}/preclassify", wrap(a.preclassifyCAR))
	mux.Handle("POST /api/v1/cars/{car_id}/submit", wrap(a.submitCAR))
	mux.Handle("POST /api/v1/cars/{car_id}/decide", wrap(a.decideCAR))
	mux.Handle("GET /api/v1/inbox", wrap(a.inbox))
	mux.Handle("POST /api/v1/approver-pools", wrap(a.addPoolMember))
}

func (a *API) createEstimate(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in domain.CreateEstimateInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	est, err := a.Domain.CreateEstimate(r.Context(), actorFrom(r.Context()), id, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, est)
}

func (a *API) listPromises(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, err := a.Domain.GetProject(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	items, err := a.Domain.ListPromises(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createPromise(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in domain.CreatePromiseInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	p, err := a.Domain.CreatePromise(r.Context(), actorFrom(r.Context()), id, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, p)
}

func (a *API) listCARs(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, err := a.Domain.GetProject(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	items, err := a.Domain.ListCARs(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) createCAR(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in domain.CreateCARInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Domain.CreateCAR(r.Context(), actorFrom(r.Context()), id, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, v)
}

func (a *API) guidanceOutcomes(w http.ResponseWriter, r *http.Request) {
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
	items, err := a.Domain.GuidanceOutcomes(r.Context(), id, limit)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *API) getCAR(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "car_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Domain.GetCAR(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

// carGuidance: guide on open. Always 200 when the CAR exists; MDEMG trouble is
// reported inside the body (guidance.available=false), never as a failure.
func (a *API) carGuidance(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "car_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Domain.CARGuidance(r.Context(), actorFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func (a *API) preclassifyCAR(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "car_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	res, err := a.Domain.PreclassifyCAR(r.Context(), actorFrom(r.Context()), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

// submitCAR: 200 with the in-flight view and the governance record; 409
// governance_denied with the violated codes; 503 governance_unavailable under
// Tier A; 422 precondition_failed for the deterministic rules.
func (a *API) submitCAR(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "car_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in domain.SubmitCARInput
	if r.ContentLength != 0 {
		if err := decode(r, &in); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	res, err := a.Domain.SubmitCAR(r.Context(), actorFrom(r.Context()), id, in)
	if err != nil {
		var denied *domain.DeniedError
		if errors.As(err, &denied) {
			status, code := http.StatusConflict, "governance_denied"
			if errors.Is(err, domain.ErrGovernanceUnavailable) {
				status, code = http.StatusServiceUnavailable, "governance_unavailable"
			}
			httpx.WriteJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": err.Error()}, "governance": denied.Record})
			return
		}
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}

func (a *API) decideCAR(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "car_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var in domain.DecideInput
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	v, err := a.Domain.DecideCAR(r.Context(), actorFrom(r.Context()), id, in)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, v)
}

func (a *API) inbox(w http.ResponseWriter, r *http.Request) {
	items, err := a.Domain.Inbox(r.Context(), actorFrom(r.Context()))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type poolMemberBody struct {
	Pool   string  `json:"pool"`
	UserID string  `json:"user_id"`
	SiteID *string `json:"site_id"`
}

func (a *API) addPoolMember(w http.ResponseWriter, r *http.Request) {
	var in poolMemberBody
	if err := decode(r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	var site *uuid.UUID
	if in.SiteID != nil && *in.SiteID != "" {
		u, err := ids.Parse(*in.SiteID)
		if err != nil {
			a.fail(w, r, errors.Join(domain.ErrValidation, errors.New("site_id: uuid required")))
			return
		}
		site = &u
	}
	if err := a.Domain.AddPoolMember(r.Context(), actorFrom(r.Context()), in.Pool, in.UserID, site); err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"pool": in.Pool, "user_id": in.UserID, "site_id": in.SiteID})
}
