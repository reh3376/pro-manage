package api

import (
	"net/http"

	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/outbox"
)

func (a *API) mountOutbox(mux *http.ServeMux, wrap func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/v1/outbox/stats", wrap(a.outboxStats))
	mux.Handle("POST /api/v1/outbox/requeue-failed", wrap(a.outboxRequeueFailed))
}

// outboxStats: per kind and status counts plus totals (operations dashboard, smoke test).
func (a *API) outboxStats(w http.ResponseWriter, r *http.Request) {
	s, err := outbox.StatsFor(r.Context(), a.Domain.Store.Queries(), a.Domain.Tenant)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}

// outboxRequeueFailed puts dead-lettered rows back in play (operator action; ?kind= narrows it).
func (a *API) outboxRequeueFailed(w http.ResponseWriter, r *http.Request) {
	var kind *string
	if k := r.URL.Query().Get("kind"); k != "" {
		kind = &k
	}
	n, err := a.Domain.Store.Queries().RequeueOutboxFailed(r.Context(), db.RequeueOutboxFailedParams{TenantID: a.Domain.Tenant, Kind: kind})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"requeued": n})
}
