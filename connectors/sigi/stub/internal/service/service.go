// Package service implements pm.connector.v1.ConnectorService and
// pm.connector.v1.SigiService over the in-memory envelope store, plus the dev
// HTTP endpoint that stands in for a signer clicking "sign".
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/reh3376/pro-manage/connectors/sigi/stub/internal/envelopes"
	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
)

// Manifest mirrors connector.yaml.
type Manifest struct {
	Module, Type, Code, Version, Contract string
	Capabilities                          []string
}

// DefaultManifest is the stub's.
var DefaultManifest = Manifest{Module: "Sigi_STUB", Type: "Sigi", Code: "STUB", Contract: "pm.connector.v1.SigiService",
	Capabilities: []string{"envelope_create", "envelope_status", "envelope_cancel", "signed_artifact", "webhook", "dev_sign"}}

// Service holds the store.
type Service struct {
	connectorv1.UnimplementedConnectorServiceServer
	connectorv1.UnimplementedSigiServiceServer
	Store         *envelopes.Store
	Manifest      Manifest
	Log           *slog.Logger
	WebhookSecret string
}

// ---------------------------------------------------------------- ConnectorService

func (s *Service) HealthCheck(_ context.Context, _ *connectorv1.HealthCheckRequest) (*connectorv1.HealthCheckResponse, error) {
	return &connectorv1.HealthCheckResponse{Status: connectorv1.HealthCheckResponse_STATUS_SERVING, Detail: "in-memory stub", Checks: map[string]string{"store": "ok"}}, nil
}

func (s *Service) Capabilities(_ context.Context, _ *connectorv1.CapabilitiesRequest) (*connectorv1.CapabilitiesResponse, error) {
	m := s.Manifest
	return &connectorv1.CapabilitiesResponse{Module: m.Module, Type: m.Type, Code: m.Code, Version: m.Version, Contract: m.Contract, Capabilities: m.Capabilities, Tiers: []string{}}, nil
}

// ---------------------------------------------------------------- SigiService

func (s *Service) CreateEnvelope(_ context.Context, req *connectorv1.CreateEnvelopeRequest) (*connectorv1.CreateEnvelopeResponse, error) {
	if req.GetSubjectType() == "" || req.GetSubjectId() == "" || req.GetTitle() == "" {
		return nil, status.Error(codes.InvalidArgument, "subject_type, subject_id, and title are required")
	}
	e := envelopes.Envelope{IdempotencyKey: req.GetIdempotencyKey(), SubjectType: req.GetSubjectType(), SubjectID: req.GetSubjectId(), Title: req.GetTitle(),
		Document: req.GetDocument(), DocumentType: req.GetDocumentMediaType(), Metadata: req.GetMetadata()}
	for _, sg := range req.GetSigners() {
		if sg.GetUserId() == "" {
			return nil, status.Error(codes.InvalidArgument, "every signer needs a user_id")
		}
		e.Signers = append(e.Signers, envelopes.Signer{UserID: sg.GetUserId(), Email: sg.GetEmail(), Name: sg.GetName(), Order: sg.GetOrder(), Role: sg.GetRole()})
	}
	created, dup := s.Store.Create(e)
	if !dup {
		s.Log.Info("envelope created", "envelope", created.ID, "subject", created.SubjectType+":"+created.SubjectID, "signers", len(created.Signers))
	}
	urls := make([]string, 0, len(created.Signers))
	for _, sg := range created.Signers {
		urls = append(urls, "/dev/envelopes/"+created.ID+"/sign?user_id="+sg.UserID)
	}
	return &connectorv1.CreateEnvelopeResponse{EnvelopeId: created.ID, Status: toStatus(created.Status), SigningUrls: urls, CreatedAt: timestamppb.New(created.CreatedAt)}, nil
}

func (s *Service) GetEnvelopeStatus(_ context.Context, req *connectorv1.GetEnvelopeStatusRequest) (*connectorv1.GetEnvelopeStatusResponse, error) {
	e, err := s.Store.Get(req.GetEnvelopeId())
	if err != nil {
		return nil, grpcErr(err)
	}
	resp := &connectorv1.GetEnvelopeStatusResponse{EnvelopeId: e.ID, Status: toStatus(e.Status), ArtifactSha256: e.ArtifactSHA256, Detail: e.Detail}
	if !e.CompletedAt.IsZero() {
		resp.CompletedAt = timestamppb.New(e.CompletedAt)
	}
	for _, sg := range e.Signers {
		st := &connectorv1.SignerStatus{Signer: &connectorv1.Signer{UserId: sg.UserID, Email: sg.Email, Name: sg.Name, Order: sg.Order, Role: sg.Role}, Status: sg.Status}
		if !sg.SignedAt.IsZero() {
			st.SignedAt = timestamppb.New(sg.SignedAt)
		}
		resp.Signers = append(resp.Signers, st)
	}
	return resp, nil
}

func (s *Service) CancelEnvelope(_ context.Context, req *connectorv1.CancelEnvelopeRequest) (*connectorv1.CancelEnvelopeResponse, error) {
	e, err := s.Store.Cancel(req.GetEnvelopeId(), req.GetReason())
	if err != nil {
		return nil, grpcErr(err)
	}
	return &connectorv1.CancelEnvelopeResponse{Status: toStatus(e.Status)}, nil
}

func (s *Service) DownloadSignedArtifact(req *connectorv1.DownloadSignedArtifactRequest, stream connectorv1.SigiService_DownloadSignedArtifactServer) error {
	e, err := s.Store.Get(req.GetEnvelopeId())
	if err != nil {
		return grpcErr(err)
	}
	if e.Status != envelopes.Completed {
		return status.Error(codes.FailedPrecondition, "envelope is "+e.Status+", not completed")
	}
	const chunk = 32 * 1024
	for i := 0; i < len(e.Artifact); i += chunk {
		end := min(i+chunk, len(e.Artifact))
		msg := &connectorv1.DownloadSignedArtifactResponse{Chunk: e.Artifact[i:end]}
		if i == 0 {
			msg.MediaType, msg.TotalBytes = "application/pdf", int64(len(e.Artifact))
		}
		if err := stream.Send(msg); err != nil {
			return err
		}
	}
	return nil
}

// webhookBody is the stub's own webhook shape: {"envelope_id","status"} signed with HMAC-SHA256 in X-Stub-Signature.
type webhookBody struct {
	EnvelopeID string `json:"envelope_id"`
	Status     string `json:"status"`
}

func (s *Service) OnWebhook(_ context.Context, req *connectorv1.OnWebhookRequest) (*connectorv1.OnWebhookResponse, error) {
	if req.GetProvider() != "stub" {
		return &connectorv1.OnWebhookResponse{Accepted: false, Detail: "provider " + req.GetProvider() + " is not this module"}, nil
	}
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	mac.Write(req.GetBody())
	want := hex.EncodeToString(mac.Sum(nil))
	if got := req.GetHeaders()["X-Stub-Signature"]; !hmac.Equal([]byte(got), []byte(want)) {
		return &connectorv1.OnWebhookResponse{Accepted: false, Detail: "bad signature"}, nil
	}
	var b webhookBody
	if err := json.Unmarshal(req.GetBody(), &b); err != nil || b.EnvelopeID == "" {
		return &connectorv1.OnWebhookResponse{Accepted: false, Detail: "undecodable body"}, nil
	}
	e, err := s.Store.Get(b.EnvelopeID)
	if err != nil {
		return &connectorv1.OnWebhookResponse{Accepted: false, EnvelopeId: b.EnvelopeID, Detail: "unknown envelope"}, nil
	}
	return &connectorv1.OnWebhookResponse{Accepted: true, EnvelopeId: e.ID, Status: toStatus(e.Status)}, nil
}

// SignWebhook renders the stub's webhook for an envelope (what a provider would push to pm-core).
func (s *Service) SignWebhook(envelopeID, st string) (body []byte, signature string) {
	body, _ = json.Marshal(webhookBody{EnvelopeID: envelopeID, Status: st})
	mac := hmac.New(sha256.New, []byte(s.WebhookSecret))
	mac.Write(body)
	return body, hex.EncodeToString(mac.Sum(nil))
}

// ---------------------------------------------------------------- dev HTTP

// DevMux serves the stand-in signer: POST /dev/envelopes/{id}/sign?user_id=&decline=1
// completes (or declines) a signer; GET /dev/envelopes/{id} shows the envelope.
func (s *Service) DevMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /dev/envelopes/{id}/sign", func(w http.ResponseWriter, r *http.Request) {
		e, err := s.Store.Sign(r.PathValue("id"), r.URL.Query().Get("user_id"), r.URL.Query().Get("decline") == "1")
		if err != nil {
			code := http.StatusConflict
			if errors.Is(err, envelopes.ErrNotFound) {
				code = http.StatusNotFound
			}
			if errors.Is(err, envelopes.ErrNoSigner) {
				code = http.StatusUnprocessableEntity
			}
			writeJSON(w, code, map[string]any{"error": err.Error()})
			return
		}
		s.Log.Info("dev sign", "envelope", e.ID, "status", e.Status)
		writeJSON(w, http.StatusOK, view(e))
	})
	mux.HandleFunc("GET /dev/envelopes/{id}", func(w http.ResponseWriter, r *http.Request) {
		e, err := s.Store.Get(r.PathValue("id"))
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, view(e))
	})
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"envelopes": s.Store.Len()})
	})
	return mux
}

func view(e envelopes.Envelope) map[string]any {
	signers := make([]map[string]any, 0, len(e.Signers))
	for _, sg := range e.Signers {
		m := map[string]any{"user_id": sg.UserID, "role": sg.Role, "order": sg.Order, "status": sg.Status}
		if !sg.SignedAt.IsZero() {
			m["signed_at"] = sg.SignedAt.UTC().Format(time.RFC3339)
		}
		signers = append(signers, m)
	}
	return map[string]any{"envelope_id": e.ID, "status": e.Status, "subject_type": e.SubjectType, "subject_id": e.SubjectID, "title": e.Title,
		"signers": signers, "artifact_sha256": e.ArtifactSHA256, "artifact_bytes": len(e.Artifact), "detail": e.Detail}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func toStatus(s string) connectorv1.EnvelopeStatus {
	switch strings.ToLower(s) {
	case envelopes.Created:
		return connectorv1.EnvelopeStatus_ENVELOPE_STATUS_CREATED
	case envelopes.Sent:
		return connectorv1.EnvelopeStatus_ENVELOPE_STATUS_SENT
	case envelopes.Completed:
		return connectorv1.EnvelopeStatus_ENVELOPE_STATUS_COMPLETED
	case envelopes.Declined:
		return connectorv1.EnvelopeStatus_ENVELOPE_STATUS_DECLINED
	case envelopes.Voided:
		return connectorv1.EnvelopeStatus_ENVELOPE_STATUS_VOIDED
	}
	return connectorv1.EnvelopeStatus_ENVELOPE_STATUS_UNSPECIFIED
}

func grpcErr(err error) error {
	switch {
	case errors.Is(err, envelopes.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, envelopes.ErrClosed):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, envelopes.ErrNoSigner):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
