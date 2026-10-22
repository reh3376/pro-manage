package api

import (
	"net/http"

	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

// mountMDEMG registers the MDEMG dependency endpoints. The governed-action
// endpoints (guide on open, classify on submit) live with the CAR in car.go.
func (a *API) mountMDEMG(mux *http.ServeMux, wrap func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/v1/mdemg/status", wrap(a.mdemgStatus))
}

// mdemgStatus reports what pm-core sees of its MDEMG: reachability, Jiminy
// enabled, latency, and the configured base URL (never the key). 200 when
// reachable, 503 otherwise, so dashboards and the smoke test can use it.
func (a *API) mdemgStatus(w http.ResponseWriter, r *http.Request) {
	if a.MDEMG == nil {
		httpx.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{"configured": false, "reachable": false})
		return
	}
	h := a.MDEMG.Health(mdemgadapter.WithRequestID(r.Context(), requestIDFrom(r.Context())))
	status := http.StatusOK
	if !h.Reachable {
		status = http.StatusServiceUnavailable
	}
	httpx.WriteJSON(w, status, map[string]any{
		"configured": true, "base_url": a.MDEMGBaseURL, "reachable": h.Reachable, "jiminy_enabled": h.JiminyEnabled,
		"status": h.Status, "latency_ms": h.LatencyMs, "error": h.Error,
	})
}
