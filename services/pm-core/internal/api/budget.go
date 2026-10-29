package api

import (
	"errors"
	"net/http"

	"github.com/reh3376/pro-manage/services/pm-core/internal/acl"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/erpi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
)

// The ERP mirror (FSD §03 Budget, §10 ERPi): what pm-core holds of the ERP's
// budget versions, actuals, commitments, and the releases it made there.
func (a *API) mountBudget(mux *http.ServeMux, wrap func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/v1/projects/{project_id}/budget", wrap(a.projectBudget))
	mux.Handle("POST /api/v1/projects/{project_id}/budget/sync", wrap(a.projectBudgetSync))
}

// projectBudget is the mirror summary (budget read). Without an ERPi instance it
// still answers, with the pm-core-only numbers (authorized total) and no mirror.
func (a *API) projectBudget(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, ok := a.authorize(w, r, id, acl.Budget, acl.Read); !ok {
		return
	}
	project, err := a.Domain.GetProject(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if a.Budget == nil {
		writeError(w, http.StatusNotFound, "not_found", "no ERP mirror on this deployment (ERPI_ADDR unset)")
		return
	}
	sum, err := a.Budget.ProjectSummary(r.Context(), project)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"summary": sum, "erp_configured": a.Budget.Configured()})
}

// projectBudgetSync pulls the project's feeds now (budget write). pm-workers does
// the same on its interval; this is for the owner who wants the mirror fresh
// before a decision, and for the checks.
func (a *API) projectBudgetSync(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "project_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, ok := a.authorize(w, r, id, acl.Budget, acl.Write); !ok {
		return
	}
	project, err := a.Domain.GetProject(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !a.Budget.Configured() {
		writeError(w, http.StatusServiceUnavailable, "erp_unconfigured", "no ERPi instance configured on this deployment")
		return
	}
	res, err := a.Budget.SyncProject(r.Context(), project)
	switch {
	case errors.Is(err, erpi.ErrNotFound):
		writeError(w, http.StatusUnprocessableEntity, "erp_project_unknown", err.Error())
		return
	case errors.Is(err, erpi.ErrUnavailable):
		writeError(w, http.StatusBadGateway, "erp_unavailable", err.Error())
		return
	case err != nil:
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, res)
}
