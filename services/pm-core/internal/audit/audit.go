// Package audit appends to and verifies the AuditEvent chain (FSD §08 "Audit log").
//
// Every state-changing transaction appends one or more events. Each event's
// signed_hash is HMAC-SHA256(prev_hash || canonical(event), chain key), so a
// tampered or missing row breaks every hash after it. Appends for one tenant are
// serialized with a transaction-scoped advisory lock so seq order and the chain
// agree. The chain key is a secret (AUDIT_CHAIN_KEY) that never leaves the vault.
package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
)

// GenesisHash is the prev_hash of a tenant's first event.
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// Chain signs and verifies events with one key.
type Chain struct {
	key []byte
}

// NewChain requires a key of at least 32 bytes.
func NewChain(key []byte) (*Chain, error) {
	if len(key) < 32 {
		return nil, errors.New("audit: chain key must be at least 32 bytes")
	}
	return &Chain{key: key}, nil
}

// Event is what a domain service records.
type Event struct {
	TenantID              string
	ProjectID             *uuid.UUID
	ActorUserID           string     // the login handle (display attribution)
	ActorID               *uuid.UUID // the user's stable id (B-11); nil for system actors and dev-header callers without a users row
	ActionType            string     // e.g. "site.create"
	SubjectType           string     // e.g. "site"
	SubjectID             string
	Before                any // nil for creates
	After                 any // nil for deletes
	SourceIP              string
	RequestID             string
	GuidanceID            string
	ClassificationVerdict string
	At                    time.Time // zero means now
}

// ChainVersion is what new rows sign under. Version 1 signed the handle alone;
// version 2 (B-11) adds the actor's stable id. Verify reads each row's own
// version, so a chain may hold both.
const ChainVersion int16 = 2

// canonical is the exact byte string that is signed. Field order is fixed and
// JSON is rendered with sorted keys, so the hash is reproducible from the row.
type canonical struct {
	AuditID               string          `json:"audit_id"`
	TenantID              string          `json:"tenant_id"`
	ProjectID             string          `json:"project_id"`
	ActorUserID           string          `json:"actor_user_id"`
	ActorID               *string         `json:"actor_id,omitempty"` // version 2 only; omitted under version 1 so old rows re-sign byte for byte
	ActionType            string          `json:"action_type"`
	SubjectType           string          `json:"subject_type"`
	SubjectID             string          `json:"subject_id"`
	Before                json.RawMessage `json:"before"`
	After                 json.RawMessage `json:"after"`
	At                    string          `json:"at"`
	SourceIP              string          `json:"source_ip"`
	RequestID             string          `json:"request_id"`
	GuidanceID            string          `json:"guidance_id"`
	ClassificationVerdict string          `json:"classification_verdict"`
}

func (c *Chain) sign(prev string, payload []byte) string {
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(prev))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// canonicalBytes renders the signed payload for a stored row.
func canonicalBytes(row db.AuditEvent) ([]byte, error) {
	c := canonical{
		AuditID: row.AuditID.String(), TenantID: row.TenantID, ProjectID: uuidStr(row.ProjectID),
		ActorUserID: row.ActorUserID, ActionType: row.ActionType, SubjectType: row.SubjectType, SubjectID: row.SubjectID,
		Before: normalizeJSON(row.BeforeState), After: normalizeJSON(row.AfterState),
		At: row.At.UTC().Format(time.RFC3339Nano), SourceIP: strOr(row.SourceIp), RequestID: strOr(row.RequestID),
		GuidanceID: strOr(row.GuidanceID), ClassificationVerdict: strOr(row.ClassificationVerdict),
	}
	if row.ChainVersion >= 2 {
		id := uuidStr(row.ActorID)
		c.ActorID = &id
	}
	return json.Marshal(c)
}

// Append writes one event inside the caller's transaction and returns the stored row.
func (c *Chain) Append(ctx context.Context, q *db.Queries, ev Event) (db.AuditEvent, error) {
	if ev.TenantID == "" || ev.ActorUserID == "" || ev.ActionType == "" || ev.SubjectType == "" || ev.SubjectID == "" {
		return db.AuditEvent{}, errors.New("audit: tenant, actor, action_type, subject_type, subject_id are required")
	}
	if err := q.LockAuditChain(ctx, ev.TenantID); err != nil {
		return db.AuditEvent{}, fmt.Errorf("audit: lock: %w", err)
	}
	prev, err := q.LastAuditHash(ctx, ev.TenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		prev = GenesisHash
	} else if err != nil {
		return db.AuditEvent{}, fmt.Errorf("audit: last hash: %w", err)
	}

	before, err := marshalState(ev.Before)
	if err != nil {
		return db.AuditEvent{}, err
	}
	after, err := marshalState(ev.After)
	if err != nil {
		return db.AuditEvent{}, err
	}
	at := ev.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	at = at.UTC().Truncate(time.Microsecond) // Postgres timestamptz precision; the stored and signed values must agree

	row := db.AuditEvent{
		AuditID: ids.New(), TenantID: ev.TenantID, ProjectID: ev.ProjectID, ActorUserID: ev.ActorUserID, ActorID: ev.ActorID,
		ActionType: ev.ActionType, SubjectType: ev.SubjectType, SubjectID: ev.SubjectID,
		BeforeState: before, AfterState: after, At: at,
		SourceIp: nilIfEmpty(ev.SourceIP), RequestID: nilIfEmpty(ev.RequestID),
		GuidanceID: nilIfEmpty(ev.GuidanceID), ClassificationVerdict: nilIfEmpty(ev.ClassificationVerdict),
		PrevHash: prev, ChainVersion: ChainVersion,
	}
	payload, err := canonicalBytes(row)
	if err != nil {
		return db.AuditEvent{}, fmt.Errorf("audit: canonical: %w", err)
	}
	row.SignedHash = c.sign(prev, payload)

	return q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		AuditID: row.AuditID, TenantID: row.TenantID, ProjectID: row.ProjectID, ActorUserID: row.ActorUserID,
		ActionType: row.ActionType, SubjectType: row.SubjectType, SubjectID: row.SubjectID,
		BeforeState: row.BeforeState, AfterState: row.AfterState, At: row.At,
		SourceIp: row.SourceIp, RequestID: row.RequestID, GuidanceID: row.GuidanceID,
		ClassificationVerdict: row.ClassificationVerdict, PrevHash: row.PrevHash, SignedHash: row.SignedHash,
		ActorID: row.ActorID, ChainVersion: row.ChainVersion,
	})
}

// VerifyResult reports a chain walk.
type VerifyResult struct {
	TenantID   string `json:"tenant_id"`
	Checked    int64  `json:"checked"`
	Valid      bool   `json:"valid"`
	FirstBreak *int64 `json:"first_break_seq,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// Verify walks every event of a tenant in seq order and recomputes the chain.
func (c *Chain) Verify(ctx context.Context, q *db.Queries, tenantID string) (VerifyResult, error) {
	res := VerifyResult{TenantID: tenantID, Valid: true}
	prev := GenesisHash
	var after int64
	const page = 500
	for {
		rows, err := q.ListAuditEventsInOrder(ctx, db.ListAuditEventsInOrderParams{TenantID: tenantID, Seq: after, Limit: page})
		if err != nil {
			return res, fmt.Errorf("audit: verify list: %w", err)
		}
		for _, row := range rows {
			res.Checked++
			if row.PrevHash != prev {
				return broken(res, row.Seq, "prev_hash does not match the previous row"), nil
			}
			payload, err := canonicalBytes(row)
			if err != nil {
				return res, err
			}
			if !hmac.Equal([]byte(c.sign(prev, payload)), []byte(row.SignedHash)) {
				return broken(res, row.Seq, "signed_hash does not match the row content"), nil
			}
			prev = row.SignedHash
			after = row.Seq
		}
		if len(rows) < page {
			return res, nil
		}
	}
}

func broken(res VerifyResult, seq int64, reason string) VerifyResult {
	res.Valid = false
	res.FirstBreak = &seq
	res.Reason = reason
	return res
}

func marshalState(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("audit: state: %w", err)
	}
	return normalizeJSON(b), nil
}

// normalizeJSON re-encodes with sorted keys and no whitespace so the stored
// jsonb (which Postgres may reorder) and the signed form agree.
func normalizeJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	b, err := json.Marshal(v) // encoding/json sorts map keys
	if err != nil {
		return raw
	}
	return b
}

func uuidStr(u *uuid.UUID) string {
	if u == nil {
		return ""
	}
	return u.String()
}

func strOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func nilIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}
