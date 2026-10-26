// Package sla is the SLA tick of FSD §08 "SLA, nudges, auto-escalation": a
// pure evaluation of one open stage against the clock (75% nudge, 100%
// overdue, grace then escalate once, then expire) and the worker loop that
// applies it: marks the stage, notifies through Commsi, and for a CAR whose
// chain ran out of time expires the workflow and the CAR with an audit row.
package sla

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/notify"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
)

// Action is what the tick should do to a stage now.
type Action int

// Actions, in escalating order; at most one per tick per stage.
const (
	None Action = iota
	Nudge
	Overdue
	Escalate
	Expire
)

func (a Action) String() string {
	return [...]string{"none", "nudge", "overdue", "escalate", "expire"}[a]
}

// State is the per-stage input to Evaluate.
type State struct {
	EnteredAt   time.Time
	DueAt       time.Time
	NudgedAt    *time.Time
	OverdueAt   *time.Time
	EscalatedAt *time.Time
	Grace       time.Duration // overdue grace before escalation (and again before expiry)
	Expires     bool          // the workflow kind expires (CAR: yes; closeout: no)
}

// DefaultGrace applies when a workflow's policy snapshot predates the variable.
const DefaultGrace = 24 * time.Hour

// Evaluate is the pure rule. The due date moves forward by the grace when a
// stage is escalated, so "past due again" after escalation means expiry.
func Evaluate(s State, now time.Time) Action {
	if s.EnteredAt.IsZero() || s.DueAt.IsZero() || !s.DueAt.After(s.EnteredAt) {
		return None
	}
	switch {
	case s.EscalatedAt != nil:
		if s.Expires && !now.Before(s.DueAt) {
			return Expire
		}
		return None
	case s.OverdueAt != nil:
		if !now.Before(s.DueAt.Add(s.Grace)) {
			return Escalate
		}
		return None
	case !now.Before(s.DueAt):
		return Overdue
	case s.NudgedAt == nil && !now.Before(s.EnteredAt.Add(time.Duration(float64(s.DueAt.Sub(s.EnteredAt))*0.75))):
		return Nudge
	}
	return None
}

// Ticker runs the loop.
type Ticker struct {
	Store    *store.Store
	Chain    *audit.Chain
	Notifier *notify.Notifier
	Tenant   string
	Log      *slog.Logger
	Now      func() time.Time // tests inject the clock
}

// Counters is what one tick did.
type Counters struct {
	Scanned, Nudged, Overdue, Escalated, Expired int
}

// Tick evaluates every open stage once.
func (t *Ticker) Tick(ctx context.Context) (Counters, error) {
	now := time.Now().UTC()
	if t.Now != nil {
		now = t.Now()
	}
	rows, err := t.Store.Queries().ListOpenStagesForTick(ctx, db.ListOpenStagesForTickParams{TenantID: t.Tenant, Limit: 500})
	if err != nil {
		return Counters{}, fmt.Errorf("sla: list: %w", err)
	}
	var c Counters
	c.Scanned = len(rows)
	for _, row := range rows {
		if row.EnteredAt == nil || row.StageDueAt == nil {
			continue
		}
		grace := graceFrom(row.PolicySnapshot)
		action := Evaluate(State{EnteredAt: *row.EnteredAt, DueAt: *row.StageDueAt, NudgedAt: row.NudgedAt, OverdueAt: row.OverdueAt, EscalatedAt: row.EscalatedAt, Grace: grace, Expires: row.WorkflowKind == "car"}, now)
		if action == None {
			continue
		}
		if err := t.apply(ctx, row, action, grace, now); err != nil {
			t.Log.Error("sla tick", "stage", row.StageID, "action", action.String(), "err", err)
			continue
		}
		switch action {
		case Nudge:
			c.Nudged++
		case Overdue:
			c.Overdue++
		case Escalate:
			c.Escalated++
		case Expire:
			c.Expired++
		}
	}
	return c, nil
}

func (t *Ticker) apply(ctx context.Context, row db.ListOpenStagesForTickRow, action Action, grace time.Duration, now time.Time) error {
	return t.Store.Tx(ctx, func(q *db.Queries) error {
		subject, link, code := t.describe(ctx, q, row)
		owner := row.InitiatedBy
		base := notify.Notification{SubjectType: row.SubjectType, SubjectID: row.SubjectID, DeepLink: link, Metadata: map[string]string{"stage": fmt.Sprint(row.StageOrder), "role": row.RoleRequired, "code": code}, DueAt: row.StageDueAt}
		switch action {
		case Nudge:
			n, err := q.MarkStageNudged(ctx, db.MarkStageNudgedParams{StageID: row.StageID, NudgedAt: &now})
			if err != nil || n == 0 {
				return err
			}
			base.Kind, base.Recipients = notify.KindApprovalNudge, row.Assignees
			base.Subject = fmt.Sprintf("Reminder: %s awaits your decision (stage %d, %s)", subject, row.StageOrder, row.RoleRequired)
			base.Body = fmt.Sprintf("Three quarters of the SLA for stage %d (%s) of %s has elapsed. The stage is due %s.", row.StageOrder, row.RoleRequired, subject, row.StageDueAt.UTC().Format(time.RFC1123))
		case Overdue:
			n, err := q.MarkStageOverdue(ctx, db.MarkStageOverdueParams{StageID: row.StageID, OverdueAt: &now})
			if err != nil || n == 0 {
				return err
			}
			base.Kind, base.Recipients = notify.KindApprovalOverdue, append(append([]string{}, row.Assignees...), owner)
			base.Subject = fmt.Sprintf("Overdue: %s (stage %d, %s)", subject, row.StageOrder, row.RoleRequired)
			base.Body = fmt.Sprintf("Stage %d (%s) of %s passed its SLA at %s. It escalates after a grace of %s if still undecided.", row.StageOrder, row.RoleRequired, subject, row.StageDueAt.UTC().Format(time.RFC1123), grace)
		case Escalate:
			due := now.Add(grace)
			n, err := q.MarkStageEscalated(ctx, db.MarkStageEscalatedParams{StageID: row.StageID, EscalatedAt: &now, StageDueAt: &due})
			if err != nil || n == 0 {
				return err
			}
			base.Kind, base.Recipients, base.DueAt = notify.KindApprovalEscalated, append(append([]string{}, row.Assignees...), owner), &due
			base.Subject = fmt.Sprintf("Escalated: %s (stage %d, %s)", subject, row.StageOrder, row.RoleRequired)
			base.Body = fmt.Sprintf("Stage %d (%s) of %s is escalated: the grace period passed without a decision. One final window of %s runs until %s, after which the request expires and the owner must resubmit.", row.StageOrder, row.RoleRequired, subject, grace, due.UTC().Format(time.RFC1123))
			if _, err := t.Chain.Append(ctx, q, audit.Event{TenantID: t.Tenant, ProjectID: row.ProjectID, ActorUserID: "pm-workers:sla", ActionType: "workflow.escalated", SubjectType: row.SubjectType, SubjectID: row.SubjectID,
				After: map[string]any{"workflow_id": row.WorkflowID, "stage": row.StageOrder, "role": row.RoleRequired, "assignees": row.Assignees, "new_due_at": due}}); err != nil {
				return err
			}
		case Expire:
			n, err := q.ExpireWorkflow(ctx, db.ExpireWorkflowParams{TenantID: t.Tenant, WorkflowID: row.WorkflowID})
			if err != nil || n == 0 {
				return err
			}
			if row.SubjectType == "capital_authorization" {
				if carID, err := ids.Parse(row.SubjectID); err == nil {
					if _, err := q.SetCARExpired(ctx, db.SetCARExpiredParams{TenantID: t.Tenant, CarID: carID, UpdatedBy: "pm-workers:sla"}); err != nil {
						return err
					}
				}
			}
			base.Kind, base.Recipients = notify.KindApprovalExpired, append(append([]string{}, row.Assignees...), owner)
			base.Subject = fmt.Sprintf("Expired: %s was not decided in time", subject)
			base.Body = fmt.Sprintf("%s expired at stage %d (%s) after escalation. The owner may revise and resubmit, which starts a new approval chain.", subject, row.StageOrder, row.RoleRequired)
			if _, err := t.Chain.Append(ctx, q, audit.Event{TenantID: t.Tenant, ProjectID: row.ProjectID, ActorUserID: "pm-workers:sla", ActionType: expiredAction(row.SubjectType), SubjectType: row.SubjectType, SubjectID: row.SubjectID,
				After: map[string]any{"workflow_id": row.WorkflowID, "stage": row.StageOrder, "role": row.RoleRequired, "assignees": row.Assignees}}); err != nil {
				return err
			}
		}
		if t.Notifier == nil {
			return nil
		}
		_, skipped, err := t.Notifier.Enqueue(ctx, q, base)
		if len(skipped) > 0 {
			t.Log.Warn("sla notification: recipients without email skipped", "kind", base.Kind, "skipped", skipped)
		}
		return err
	})
}

// describe names the subject for a human and its deep link.
func (t *Ticker) describe(ctx context.Context, q *db.Queries, row db.ListOpenStagesForTickRow) (subject, link, code string) {
	if row.SubjectType == "capital_authorization" {
		if carID, err := ids.Parse(row.SubjectID); err == nil {
			if car, err := q.GetCAR(ctx, db.GetCARParams{TenantID: t.Tenant, CarID: carID}); err == nil {
				return fmt.Sprintf("CAR %s (%s %s)", car.Code, car.AmountRequested, car.Currency), "/cars/" + car.CarID.String(), car.Code
			}
		}
	}
	return row.SubjectType + " " + row.SubjectID, "/inbox", row.SubjectID
}

func expiredAction(subjectType string) string {
	if subjectType == "capital_authorization" {
		return "car.expired"
	}
	return strings.ReplaceAll(subjectType, " ", "_") + ".expired"
}

// graceFrom reads sla_overdue_grace_hours from the workflow's policy snapshot.
func graceFrom(snapshot []byte) time.Duration {
	var m struct {
		Hours float64 `json:"sla_overdue_grace_hours"`
	}
	if err := json.Unmarshal(snapshot, &m); err != nil || m.Hours <= 0 {
		return DefaultGrace
	}
	return time.Duration(m.Hours * float64(time.Hour))
}
