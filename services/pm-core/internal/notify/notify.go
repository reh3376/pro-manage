// Package notify is how pm-core tells people things (FSD §08 workflow events,
// §08 "SLA, nudges, auto-escalation", §10 Commsi): a notification is written
// to the outbox inside the transaction that caused it and delivered by
// pm-workers through the Commsi module, so a request is never held hostage
// by a mail relay and nothing is announced that did not commit.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	connectorv1 "github.com/reh3376/pro-manage/services/pm-core/gen/pm/connector/v1"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/outbox"
)

// Kinds the spine sends (the Commsi contract's comment lists them).
const (
	KindApprovalRequested = "approval.requested"
	KindApprovalNudge     = "approval.nudge"
	KindApprovalOverdue   = "approval.overdue"
	KindApprovalEscalated = "approval.escalated"
	KindApprovalExpired   = "approval.expired"
	KindApprovalDecided   = "approval.decided"
	KindSignatureRequest  = "signature.requested"
	KindAlertOperational  = "alert.operational"
)

// Notification is what a domain event asks to send.
type Notification struct {
	Kind        string
	SubjectType string
	SubjectID   string
	Recipients  []string // login handles; emails resolved from users, else <handle>@FallbackDomain
	Subject     string
	Body        string
	DeepLink    string // relative to the frontend (/inbox, /cars/<id>)
	DedupeKey   string // defaults to kind:subject:recipient-set
	Metadata    map[string]string
	DueAt       *time.Time
}

// Notifier enqueues notifications.
type Notifier struct {
	Tenant         string
	FallbackDomain string // dev: northfield.example; empty means users without an email are skipped
}

// Enqueue resolves the recipients and writes the outbox row inside q's transaction.
// Recipients without a resolvable email are dropped (and reported); a
// notification with no recipients left is not enqueued.
func (n *Notifier) Enqueue(ctx context.Context, q *db.Queries, msg Notification) (sent int, skipped []string, err error) {
	if msg.Kind == "" || msg.Subject == "" {
		return 0, nil, errors.New("notify: kind and subject are required")
	}
	req := &connectorv1.SendNotificationRequest{Kind: msg.Kind, SubjectType: msg.SubjectType, SubjectId: msg.SubjectID, Subject: msg.Subject, BodyText: msg.Body, DeepLink: msg.DeepLink, Metadata: msg.Metadata}
	if msg.DueAt != nil {
		req.DueAt = timestamppb.New(*msg.DueAt)
	}
	seen := map[string]bool{}
	for _, handle := range msg.Recipients {
		handle = strings.TrimSpace(handle)
		if handle == "" || seen[handle] {
			continue
		}
		seen[handle] = true
		r := &connectorv1.Recipient{UserId: handle, Name: handle}
		u, err := q.GetUserByUsername(ctx, db.GetUserByUsernameParams{TenantID: n.Tenant, Username: handle})
		switch {
		case err == nil && u.Email != "":
			r.Email, r.Name = u.Email, u.DisplayName
			if r.Name == "" {
				r.Name = handle
			}
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return 0, nil, err
		case n.FallbackDomain != "":
			r.Email = handle + "@" + n.FallbackDomain
		default:
			skipped = append(skipped, handle)
			continue
		}
		req.Recipients = append(req.Recipients, r)
	}
	if len(req.Recipients) == 0 {
		return 0, skipped, nil
	}
	if msg.DedupeKey == "" {
		handles := make([]string, 0, len(req.Recipients))
		for _, r := range req.Recipients {
			handles = append(handles, r.UserId)
		}
		msg.DedupeKey = msg.Kind + ":" + msg.SubjectType + ":" + msg.SubjectID + ":" + strings.Join(handles, ",")
	}
	req.DedupeKey = msg.DedupeKey
	raw, err := protojson.Marshal(req)
	if err != nil {
		return 0, skipped, err
	}
	// The source_ref makes a retried transaction idempotent: same event, same recipients, one row.
	ref := "notify:" + msg.DedupeKey + ":" + time.Now().UTC().Format("20060102T150405")
	if err := outbox.Enqueue(ctx, q, n.Tenant, outbox.KindCommsiSend, ref, json.RawMessage(raw)); err != nil {
		return 0, skipped, err
	}
	return len(req.Recipients), skipped, nil
}

// Decode turns an outbox payload back into the request (the drainer's side).
func Decode(raw json.RawMessage) (*connectorv1.SendNotificationRequest, error) {
	var req connectorv1.SendNotificationRequest
	if err := protojson.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("notify: payload: %w", err)
	}
	return &req, nil
}
