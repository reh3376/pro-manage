// Package signing is pm-core's side of e-signature (FSD §08 "signed artifact
// stored via Sigi if applicable", §10 Sigi): the envelope record, its creation
// at the provider through the outbox, reconciliation of open envelopes from
// pm-workers (status, artifact download, hash check, object-store capture,
// audit), and provider webhooks. Which actions need a signature is policy
// (car.esign.required_above); below the threshold the audited approval is the
// attestation of record.
package signing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/blobs"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/sigi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
)

// OutboxKind is the outbox row that creates an envelope at the provider.
const OutboxKind = "sigi.envelope.create"

// Payload of an OutboxKind row.
type Payload struct {
	EnvelopeID string `json:"envelope_id"`
}

// Signer is the JSON shape kept in signature_envelopes.signers.
type Signer struct {
	UserID   string     `json:"user_id"`
	Email    string     `json:"email"`
	Name     string     `json:"name"`
	Order    int32      `json:"order"`
	Role     string     `json:"role"`
	Status   string     `json:"status"`
	SignedAt *time.Time `json:"signed_at,omitempty"`
}

// Service holds the dependencies. Sigi and Blobs may be nil in pm-core (then
// envelopes are requested but only pm-workers, which has both, moves them).
type Service struct {
	Store  *store.Store
	Chain  *audit.Chain
	Blobs  *blobs.Store
	Sigi   *sigi.Client
	Tenant string
	Log    *slog.Logger
}

// Request records an envelope to be created and enqueues its creation, inside
// the caller's transaction (the approval's). Idempotent on (subject, audit).
type Request struct {
	ProjectID   *uuid.UUID
	SubjectType string
	SubjectID   string
	Title       string
	Document    string // text the module renders into the artifact
	Signers     []Signer
	Metadata    map[string]string
	RequestedBy string
	AuditID     uuid.UUID
}

// Request inserts the row and the outbox job. Returns the envelope id.
func (s *Service) Request(ctx context.Context, q *db.Queries, r Request, enqueue func(kind, sourceRef string, payload any) error) (uuid.UUID, error) {
	provider := "unconfigured"
	if s.Sigi != nil {
		provider = s.Sigi.Provider
	}
	for i := range r.Signers {
		if r.Signers[i].Status == "" {
			r.Signers[i].Status = "pending"
		}
	}
	signers, _ := json.Marshal(r.Signers)
	auditID := r.AuditID
	row, err := q.RequestEnvelope(ctx, db.RequestEnvelopeParams{
		EnvelopeID: ids.New(), TenantID: s.Tenant, ProjectID: r.ProjectID, SubjectType: r.SubjectType, SubjectID: r.SubjectID,
		Provider: provider, Title: r.Title, Signers: signers, RequestedBy: r.RequestedBy, AuditID: &auditID,
	})
	if errors.Is(err, pgx.ErrNoRows) { // already requested for this approval
		existing, err := q.GetEnvelopeByAudit(ctx, db.GetEnvelopeByAuditParams{SubjectType: r.SubjectType, SubjectID: r.SubjectID, AuditID: &auditID})
		if err != nil {
			return uuid.Nil, err
		}
		return existing.EnvelopeID, nil
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("signing: request envelope: %w", err)
	}
	payload := map[string]any{"envelope_id": row.EnvelopeID.String(), "document": r.Document, "metadata": r.Metadata}
	if err := enqueue(OutboxKind, "envelope:"+row.EnvelopeID.String(), payload); err != nil {
		return uuid.Nil, err
	}
	return row.EnvelopeID, nil
}

// createPayload is what the outbox row carries.
type createPayload struct {
	EnvelopeID string            `json:"envelope_id"`
	Document   string            `json:"document"`
	Metadata   map[string]string `json:"metadata"`
}

// CreateAtProvider is the outbox handler: creates the envelope at the provider
// and records the provider reference. Returns sigi.ErrRejected for a payload
// the module refuses (the drainer dead-letters) and transient errors otherwise.
func (s *Service) CreateAtProvider(ctx context.Context, raw json.RawMessage) error {
	if s.Sigi == nil {
		return fmt.Errorf("%w: no Sigi module configured", sigi.ErrUnavailable)
	}
	var p createPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%w: payload: %v", sigi.ErrRejected, err)
	}
	id, err := ids.Parse(p.EnvelopeID)
	if err != nil {
		return fmt.Errorf("%w: envelope id: %v", sigi.ErrRejected, err)
	}
	q := s.Store.Queries()
	row, err := q.GetEnvelope(ctx, db.GetEnvelopeParams{TenantID: s.Tenant, EnvelopeID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: envelope %s unknown", sigi.ErrRejected, id)
	}
	if err != nil {
		return err
	}
	if row.ProviderRef != nil && *row.ProviderRef != "" {
		return nil // already created (a retried row)
	}
	var signers []Signer
	_ = json.Unmarshal(row.Signers, &signers)
	req := &connectorv1.CreateEnvelopeRequest{
		SubjectType: row.SubjectType, SubjectId: row.SubjectID, Title: row.Title, Document: []byte(p.Document), DocumentMediaType: "text/plain",
		Metadata: p.Metadata, IdempotencyKey: row.EnvelopeID.String(),
	}
	for _, sg := range signers {
		req.Signers = append(req.Signers, &connectorv1.Signer{UserId: sg.UserID, Email: sg.Email, Name: sg.Name, Order: sg.Order, Role: sg.Role})
	}
	resp, err := s.Sigi.Create(ctx, req)
	if err != nil {
		if errors.Is(err, sigi.ErrRejected) {
			_ = q.MarkEnvelopeFailed(ctx, db.MarkEnvelopeFailedParams{EnvelopeID: id, LastError: ptr(err.Error())})
		}
		return err
	}
	if err := q.MarkEnvelopeCreated(ctx, db.MarkEnvelopeCreatedParams{EnvelopeID: id, ProviderRef: ptr(resp.GetEnvelopeId()), Status: statusName(resp.GetStatus()), Signers: row.Signers}); err != nil {
		return err
	}
	s.Log.Info("envelope created at provider", "envelope", id, "provider", row.Provider, "provider_ref", resp.GetEnvelopeId(), "status", statusName(resp.GetStatus()))
	return nil
}

// Reconcile polls every open envelope (§10 "lost webhooks"). Returns how many it touched.
func (s *Service) Reconcile(ctx context.Context, limit int32) (int, error) {
	if s.Sigi == nil {
		return 0, nil
	}
	rows, err := s.Store.Queries().ListOpenEnvelopes(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("signing: list open: %w", err)
	}
	for _, row := range rows {
		if err := s.reconcileOne(ctx, row); err != nil {
			s.Log.Warn("envelope reconcile", "envelope", row.EnvelopeID, "err", err)
			_ = s.Store.Queries().MarkEnvelopeCheckError(ctx, db.MarkEnvelopeCheckErrorParams{EnvelopeID: row.EnvelopeID, LastError: ptr(err.Error())})
		}
	}
	return len(rows), nil
}

// ReconcileRef refreshes one envelope by provider reference (after a webhook).
func (s *Service) ReconcileRef(ctx context.Context, providerRef string) error {
	rows, err := s.Store.Queries().ListOpenEnvelopes(ctx, 500)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ProviderRef != nil && *row.ProviderRef == providerRef {
			return s.reconcileOne(ctx, row)
		}
	}
	return nil
}

func (s *Service) reconcileOne(ctx context.Context, row db.SignatureEnvelope) error {
	if row.ProviderRef == nil || *row.ProviderRef == "" {
		return nil // creation still queued
	}
	q := s.Store.Queries()
	st, err := s.Sigi.Status(ctx, *row.ProviderRef)
	if err != nil {
		return err
	}
	signers := make([]Signer, 0, len(st.GetSigners()))
	for _, ss := range st.GetSigners() {
		sg := Signer{UserID: ss.GetSigner().GetUserId(), Email: ss.GetSigner().GetEmail(), Name: ss.GetSigner().GetName(), Order: ss.GetSigner().GetOrder(), Role: ss.GetSigner().GetRole(), Status: ss.GetStatus()}
		if ss.GetSignedAt() != nil {
			t := ss.GetSignedAt().AsTime()
			sg.SignedAt = &t
		}
		signers = append(signers, sg)
	}
	signersJSON, _ := json.Marshal(signers)
	name := statusName(st.GetStatus())
	if name != "completed" {
		return q.UpdateEnvelopeStatus(ctx, db.UpdateEnvelopeStatusParams{EnvelopeID: row.EnvelopeID, Status: name, Signers: signersJSON, Detail: st.GetDetail()})
	}
	if s.Blobs == nil {
		return errors.New("completed but no object store configured to capture the artifact")
	}
	data, mediaType, err := s.Sigi.Download(ctx, *row.ProviderRef)
	if err != nil {
		return err
	}
	key := blobs.Key("envelopes", row.Provider, row.EnvelopeID.String()+ext(mediaType))
	sha, err := s.Blobs.Put(ctx, key, data, mediaType)
	if err != nil {
		return err
	}
	if want := st.GetArtifactSha256(); want != "" && !strings.EqualFold(want, sha) {
		return fmt.Errorf("artifact hash mismatch: provider says %s, download is %s", want, sha)
	}
	completedAt := time.Now().UTC()
	if st.GetCompletedAt() != nil {
		completedAt = st.GetCompletedAt().AsTime()
	}
	return s.Store.Tx(ctx, func(q *db.Queries) error {
		done, err := q.MarkEnvelopeCompleted(ctx, db.MarkEnvelopeCompletedParams{EnvelopeID: row.EnvelopeID, Signers: signersJSON, CompletedAt: &completedAt, ArtifactRef: &key, ArtifactSha256: &sha, ArtifactBytes: ptr(int64(len(data)))})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // raced with another reconcile
		}
		if err != nil {
			return err
		}
		actionType := done.SubjectType + ".signed"
		if done.SubjectType == "capital_authorization" {
			carID, err := ids.Parse(done.SubjectID)
			if err == nil {
				ref := "envelope:" + done.EnvelopeID.String()
				if err := q.SetCARSignedArtifact(ctx, db.SetCARSignedArtifactParams{TenantID: s.Tenant, CarID: carID, SignedArtifactRef: &ref, UpdatedBy: "sigi:" + done.Provider}); err != nil {
					return err
				}
			}
			actionType = "car.signed"
		}
		_, err = s.Chain.Append(ctx, q, audit.Event{
			TenantID: s.Tenant, ProjectID: done.ProjectID, ActorUserID: "sigi:" + done.Provider, ActionType: actionType, SubjectType: done.SubjectType, SubjectID: done.SubjectID,
			After: map[string]any{"envelope_id": done.EnvelopeID, "provider_ref": done.ProviderRef, "artifact_ref": key, "artifact_sha256": sha, "artifact_bytes": len(data), "signers": signers, "completed_at": completedAt},
		})
		if err == nil {
			s.Log.Info("signed artifact captured", "envelope", done.EnvelopeID, "subject", done.SubjectType+":"+done.SubjectID, "artifact", key, "sha256", sha)
		}
		return err
	})
}

// Webhook verifies a provider payload through the module and reconciles the envelope it names.
func (s *Service) Webhook(ctx context.Context, provider string, body []byte, headers map[string]string) (*connectorv1.OnWebhookResponse, error) {
	if s.Sigi == nil {
		return nil, sigi.ErrUnavailable
	}
	resp, err := s.Sigi.Webhook(ctx, provider, body, headers)
	if err != nil {
		return nil, err
	}
	if resp.GetAccepted() && resp.GetEnvelopeId() != "" {
		if err := s.ReconcileRef(ctx, resp.GetEnvelopeId()); err != nil {
			s.Log.Warn("webhook reconcile", "provider_ref", resp.GetEnvelopeId(), "err", err)
		}
	}
	return resp, nil
}

// Latest returns the newest envelope for a subject, or nil.
func (s *Service) Latest(ctx context.Context, subjectType, subjectID string) (*db.SignatureEnvelope, error) {
	row, err := s.Store.Queries().LatestEnvelopeForSubject(ctx, db.LatestEnvelopeForSubjectParams{TenantID: s.Tenant, SubjectType: subjectType, SubjectID: subjectID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// Artifact reads a captured artifact.
func (s *Service) Artifact(ctx context.Context, row db.SignatureEnvelope) ([]byte, string, error) {
	if s.Blobs == nil || row.ArtifactRef == nil {
		return nil, "", blobs.ErrNotFound
	}
	return s.Blobs.Get(ctx, *row.ArtifactRef)
}

func statusName(st connectorv1.EnvelopeStatus) string {
	switch st {
	case connectorv1.EnvelopeStatus_ENVELOPE_STATUS_CREATED:
		return "created"
	case connectorv1.EnvelopeStatus_ENVELOPE_STATUS_SENT:
		return "sent"
	case connectorv1.EnvelopeStatus_ENVELOPE_STATUS_COMPLETED:
		return "completed"
	case connectorv1.EnvelopeStatus_ENVELOPE_STATUS_DECLINED:
		return "declined"
	case connectorv1.EnvelopeStatus_ENVELOPE_STATUS_VOIDED:
		return "voided"
	case connectorv1.EnvelopeStatus_ENVELOPE_STATUS_EXPIRED:
		return "expired"
	}
	return "created"
}

func ext(mediaType string) string {
	if strings.HasPrefix(mediaType, "application/pdf") {
		return ".pdf"
	}
	return ".bin"
}

func ptr[T any](v T) *T { return &v }
