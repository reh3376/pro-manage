package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/reh3376/pro-manage/services/pm-core/internal/acl"
	"github.com/reh3376/pro-manage/services/pm-core/internal/blobs"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/sigi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
)

// The e-signature surface (FSD §10 Sigi): what a CAR's signature state is,
// the captured artifact, and the provider webhook that nudges reconciliation.
func (a *API) mountSigning(mux *http.ServeMux, wrap func(http.HandlerFunc) http.Handler, public func(http.Handler) http.Handler) {
	mux.Handle("GET /api/v1/cars/{car_id}/signature", wrap(a.carSignature))
	mux.Handle("GET /api/v1/cars/{car_id}/signed-artifact", wrap(a.carSignedArtifact))
	// Webhooks carry their own authentication (the provider's signature, verified by the module); no session.
	mux.Handle("POST /webhooks/sigi/{provider}", public(http.HandlerFunc(a.sigiWebhook)))
}

// carSignature: the CAR's signed_artifact_ref (attestation or envelope) and the envelope record when one exists.
func (a *API) carSignature(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "car_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, ok := a.authorizeCAR(w, r, id, acl.Read); !ok {
		return
	}
	v, err := a.Domain.GetCAR(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	out := map[string]any{"car_id": v.CAR.CarID, "status": v.CAR.Status, "signed_artifact_ref": v.CAR.SignedArtifactRef, "mode": "none"}
	if ref := v.CAR.SignedArtifactRef; ref != nil {
		switch {
		case strings.HasPrefix(*ref, "attestation:"):
			out["mode"] = "attestation"
		case strings.HasPrefix(*ref, "envelope:"):
			out["mode"] = "esignature"
		}
	}
	if a.Signing != nil {
		env, err := a.Signing.Latest(r.Context(), "capital_authorization", id.String())
		if err != nil {
			a.fail(w, r, err)
			return
		}
		if env != nil {
			out["envelope"] = env
			out["artifact_available"] = env.ArtifactRef != nil && a.Signing.Blobs != nil
		}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

// carSignedArtifact streams the captured artifact (budget read; the artifact is the approved CAR).
func (a *API) carSignedArtifact(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r, "car_id")
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if _, ok := a.authorizeCAR(w, r, id, acl.Read); !ok {
		return
	}
	if a.Signing == nil {
		writeError(w, http.StatusNotFound, "not_found", "signing is not configured")
		return
	}
	env, err := a.Signing.Latest(r.Context(), "capital_authorization", id.String())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if env == nil || env.ArtifactRef == nil {
		writeError(w, http.StatusNotFound, "not_found", "no signed artifact captured for this CAR yet")
		return
	}
	data, mediaType, err := a.Signing.Artifact(r.Context(), *env)
	if errors.Is(err, blobs.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "artifact missing from the object store")
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("X-Artifact-SHA256", strOr(env.ArtifactSha256))
	w.Header().Set("Content-Disposition", `attachment; filename="`+env.EnvelopeID.String()+`.pdf"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// sigiWebhook forwards a provider payload to the module for verification, then reconciles.
func (a *API) sigiWebhook(w http.ResponseWriter, r *http.Request) {
	if a.Signing == nil || a.Signing.Sigi == nil {
		writeError(w, http.StatusServiceUnavailable, "signing_unavailable", "no Sigi module configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "body", err.Error())
		return
	}
	headers := map[string]string{}
	for k, v := range r.Header {
		if len(v) > 0 && (strings.HasPrefix(k, "X-") || k == "Content-Type") {
			headers[k] = v[0]
		}
	}
	resp, err := a.Signing.Webhook(r.Context(), r.PathValue("provider"), body, headers)
	if err != nil {
		if errors.Is(err, sigi.ErrUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "signing_unavailable", err.Error())
			return
		}
		a.fail(w, r, err)
		return
	}
	if !resp.GetAccepted() {
		a.Log.Warn("sigi webhook rejected", "provider", r.PathValue("provider"), "detail", resp.GetDetail())
		writeError(w, http.StatusUnauthorized, "webhook_rejected", resp.GetDetail())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"accepted": true, "envelope": resp.GetEnvelopeId(), "status": resp.GetStatus().String()})
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
