//go:build integration

// The SLA tick against Postgres: a CAR workflow whose stage clock is wound
// back walks nudge -> overdue -> escalate -> expire across four ticks of an
// injected clock, each step once, with the notifications queued and the CAR
// expired with its audit row.
//
//	TEST_DATABASE_URL=... go test -tags integration ./internal/sla/
package sla

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/notify"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
)

const tenant = "it-sla"

func TestTickWalksTheStageToExpiry(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := store.Migrate(ctx, log, dsn); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	chain, _ := audit.NewChain([]byte("integration-test-chain-key-0123456789"))

	// Minimal rows: site, project, CAR, in-flight workflow with a 24 h stage entered 1 h ago, grace 6 h in the snapshot.
	site, project, car, wf, stage := ids.New(), ids.New(), ids.New(), ids.New(), ids.New()
	sfx := ids.New().String()[24:]
	entered := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	due := entered.Add(24 * time.Hour)
	snapshot, _ := json.Marshal(map[string]any{"sla_overdue_grace_hours": 6})
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO sites (site_id, tenant_id, code, name, country, time_zone, currency, created_by, updated_by) VALUES ($1,$2,$3,'SLA site','US','UTC','USD','it','it')`, []any{site, tenant, "SLA-" + sfx}},
		{`INSERT INTO projects (project_id, tenant_id, code, name, site_id, currency, owner_user_id, lifecycle_state, fel_stage, project_type, onboarding_level, created_by, updated_by) VALUES ($1,$2,$3,'SLA project',$4,'USD','it-owner','plan','fel3','greenfield','full','it','it')`, []any{project, tenant, "P-SLA-" + sfx, site}},
		{`INSERT INTO capital_authorizations (car_id, tenant_id, project_id, code, kind, amount_requested, currency, business_case, status, workflow_id, created_by, updated_by) VALUES ($1,$2,$3,$4,'full',640000,'USD','sla','in_flight',$5,'it','it')`, []any{car, tenant, project, "CR-SLA-" + sfx, wf}},
		{`INSERT INTO approval_workflows (workflow_id, tenant_id, project_id, kind, subject_type, subject_id, initiated_by, current_stage, status, policy_snapshot) VALUES ($1,$2,$3,'car','capital_authorization',$4,'it-owner',1,'in_flight',$5)`, []any{wf, tenant, project, car.String(), snapshot}},
		{`INSERT INTO approval_stages (stage_id, workflow_id, stage_order, role_required, assignees, sla_minutes, entered_at, stage_due_at) VALUES ($1,$2,1,'finance_approver','{it-finance}',1440,$3,$4)`, []any{stage, wf, entered, due}},
	} {
		if _, err := st.Pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	clock := entered.Add(time.Hour)
	tk := &Ticker{Store: st, Chain: chain, Notifier: &notify.Notifier{Tenant: tenant, FallbackDomain: "it.example"}, Tenant: tenant, Log: log, Now: func() time.Time { return clock }}
	kinds := func() []string {
		rows, err := st.Pool.Query(ctx, `SELECT payload->>'kind' FROM outbox WHERE tenant_id=$1 AND kind='commsi.send' AND payload->>'subjectId'=$2 ORDER BY created_at`, tenant, car.String())
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var k string
			_ = rows.Scan(&k)
			out = append(out, k)
		}
		return out
	}
	stageCols := func() (nudged, overdue, escalated *time.Time, dueAt time.Time) {
		if err := st.Pool.QueryRow(ctx, `SELECT nudged_at, overdue_at, escalated_at, stage_due_at FROM approval_stages WHERE stage_id=$1`, stage).Scan(&nudged, &overdue, &escalated, &dueAt); err != nil {
			t.Fatal(err)
		}
		return
	}

	// 1 h in: nothing.
	if c, err := tk.Tick(ctx); err != nil || c.Nudged+c.Overdue+c.Escalated+c.Expired != 0 {
		t.Fatalf("fresh: %+v %v", c, err)
	}
	// 18 h: nudge once.
	clock = entered.Add(18 * time.Hour)
	if c, _ := tk.Tick(ctx); c.Nudged != 1 {
		t.Fatalf("nudge: %+v", c)
	}
	if c, _ := tk.Tick(ctx); c.Nudged != 0 {
		t.Fatalf("nudge twice: %+v", c)
	}
	if n, _, _, _ := stageCols(); n == nil {
		t.Fatal("nudged_at")
	}
	// 24 h: overdue once (assignees and owner).
	clock = entered.Add(24 * time.Hour)
	if c, _ := tk.Tick(ctx); c.Overdue != 1 {
		t.Fatalf("overdue: %+v", c)
	}
	// 29 h: inside the 6 h grace, nothing; 30 h: escalate, due moves to 36 h.
	clock = entered.Add(29 * time.Hour)
	if c, _ := tk.Tick(ctx); c.Escalated != 0 {
		t.Fatalf("grace: %+v", c)
	}
	clock = entered.Add(30 * time.Hour)
	if c, _ := tk.Tick(ctx); c.Escalated != 1 {
		t.Fatalf("escalate: %+v", c)
	}
	if _, _, e, d := stageCols(); e == nil || !d.Equal(entered.Add(36*time.Hour)) {
		t.Fatalf("escalated: %v due %v", e, d)
	}
	// 35 h: nothing; 36 h: expire the workflow and the CAR.
	clock = entered.Add(35 * time.Hour)
	if c, _ := tk.Tick(ctx); c.Expired != 0 {
		t.Fatalf("before expiry: %+v", c)
	}
	clock = entered.Add(36 * time.Hour)
	if c, _ := tk.Tick(ctx); c.Expired != 1 {
		t.Fatalf("expire: %+v", c)
	}
	var carStatus, wfStatus string
	_ = st.Pool.QueryRow(ctx, `SELECT status FROM capital_authorizations WHERE car_id=$1`, car).Scan(&carStatus)
	_ = st.Pool.QueryRow(ctx, `SELECT status FROM approval_workflows WHERE workflow_id=$1`, wf).Scan(&wfStatus)
	if carStatus != "expired" || wfStatus != "expired" {
		t.Fatalf("statuses: car=%s workflow=%s", carStatus, wfStatus)
	}
	// Nothing left to scan; the notifications went out in order; the audit chain holds the escalation and expiry.
	if c, _ := tk.Tick(ctx); c.Scanned != 0 {
		t.Fatalf("after expiry still scanning %d", c.Scanned)
	}
	got := kinds()
	want := []string{notify.KindApprovalNudge, notify.KindApprovalOverdue, notify.KindApprovalEscalated, notify.KindApprovalExpired}
	if len(got) != len(want) {
		t.Fatalf("notifications: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("notifications: %v", got)
		}
	}
	var audited int
	_ = st.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE tenant_id=$1 AND subject_id=$2 AND action_type IN ('workflow.escalated','car.expired')`, tenant, car.String()).Scan(&audited)
	if audited != 2 {
		t.Fatalf("audit rows: %d", audited)
	}
	if r, err := chain.Verify(ctx, st.Queries(), tenant); err != nil || !r.Valid {
		t.Fatalf("chain: %+v %v", r, err)
	}
	_ = uuid.Nil
}
