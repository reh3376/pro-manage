//go:build integration

// Integration tests against a real Postgres. Run with:
//
//	TEST_DATABASE_URL=postgres://pm:...@127.0.0.1:5440/pm_core?sslmode=disable go test -tags integration ./internal/store/
//
// They apply the embedded migrations, exercise the audit chain, the
// append-only triggers, Site/Project creation with audit-in-transaction, and
// policy resolution against seeded rows. Codes are unique per run so the tests
// are repeatable on a long-lived dev database.
package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/policy"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
)

func setup(t *testing.T) (*store.Store, *domain.Service) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := store.Migrate(ctx, log, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	chain, err := audit.NewChain([]byte("integration-test-chain-key-0123456789"))
	if err != nil {
		t.Fatal(err)
	}
	return st, &domain.Service{Store: st, Chain: chain, Tenant: testTenant}
}

// testTenant keeps the integration chain apart from the running stack's "default" tenant,
// whose rows are signed with that stack's own AUDIT_CHAIN_KEY.
const testTenant = "it-test"

func suffix() string { return strings.ToUpper(ids.New().String()[24:]) }

func TestSiteProjectAuditRoundTrip(t *testing.T) {
	st, svc := setup(t)
	ctx := context.Background()
	actor := domain.Actor{UserID: "it-test", SourceIP: "127.0.0.1", RequestID: "req-1"}
	sfx := suffix()

	site, err := svc.CreateSite(ctx, actor, domain.CreateSiteInput{Code: "IT-" + sfx, Name: "Integration site", Country: "us", TimeZone: "America/Chicago", Currency: "usd"})
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if site.Code != "IT-"+sfx || site.Country != "US" || site.Currency != "USD" || site.UnitSystem != "imperial" {
		t.Fatalf("normalization failed: %+v", site)
	}
	if _, err := svc.CreateSite(ctx, actor, domain.CreateSiteInput{Code: "IT-" + sfx, Name: "dup", Country: "US", TimeZone: "UTC", Currency: "USD"}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := svc.CreateSite(ctx, actor, domain.CreateSiteInput{Code: "x", Name: "bad", Country: "US", TimeZone: "UTC", Currency: "USD"}); err == nil {
		t.Fatal("expected validation error for bad code")
	}

	budget := "1500000.00"
	start := "2026-11-01"
	p, err := svc.CreateProject(ctx, actor, domain.CreateProjectInput{Code: "P-IT-" + sfx, Name: "Integration project", SiteID: site.SiteID.String(), ProjectType: "reliability", BaselineBudgetTotal: &budget, StartDate: &start})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if p.Currency != "USD" || p.OwnerUserID != "it-test" || p.LifecycleState != "initiate" || p.FelStage != "fel1" || p.BaselineBudgetTotal == nil || *p.BaselineBudgetTotal != "1500000.00" {
		t.Fatalf("defaults failed: %+v", p)
	}
	if _, err := svc.CreateProject(ctx, actor, domain.CreateProjectInput{Code: "P-IT-X" + sfx, Name: "bad site", SiteID: ids.New().String(), ProjectType: "reliability"}); err == nil {
		t.Fatal("expected validation error for unknown site")
	}

	events, err := svc.ProjectAudit(ctx, p.ProjectID, 10)
	if err != nil || len(events) != 1 || events[0].ActionType != "project.create" || events[0].ActorUserID != "it-test" {
		t.Fatalf("project audit: %v %+v", err, events)
	}
	var after map[string]any
	if err := json.Unmarshal(events[0].AfterState, &after); err != nil || after["code"] != "P-IT-"+sfx {
		t.Fatalf("after_state should hold the project: %v %v", err, after)
	}

	res, err := svc.Chain.Verify(ctx, st.Queries(), testTenant)
	if err != nil || !res.Valid || res.Checked < 2 {
		t.Fatalf("chain should verify: %v %+v", err, res)
	}
}

func TestAuditIsAppendOnly(t *testing.T) {
	st, svc := setup(t)
	ctx := context.Background()
	q := st.Queries()
	var seq int64
	err := st.Tx(ctx, func(q *db.Queries) error {
		ev, err := svc.Chain.Append(ctx, q, audit.Event{TenantID: testTenant, ActorUserID: "it-test", ActionType: "test.append", SubjectType: "test", SubjectID: "t-" + suffix(), After: map[string]any{"k": 1}})
		seq = ev.Seq
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pool.Exec(ctx, "UPDATE audit_events SET actor_user_id = 'mallory' WHERE seq = $1", seq); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("update should be rejected by trigger, got %v", err)
	}
	if _, err := st.Pool.Exec(ctx, "DELETE FROM audit_events WHERE seq = $1", seq); err == nil || !strings.Contains(err.Error(), "append-only") {
		t.Fatalf("delete should be rejected by trigger, got %v", err)
	}
	res, err := svc.Chain.Verify(ctx, q, testTenant)
	if err != nil || !res.Valid {
		t.Fatalf("chain should still verify: %v %+v", err, res)
	}
}

func TestPolicyResolutionRoundTrip(t *testing.T) {
	st, svc := setup(t)
	ctx := context.Background()
	q := st.Queries()
	key := "it.test.threshold." + strings.ToLower(suffix())
	if _, err := q.UpsertPolicyVariable(ctx, db.UpsertPolicyVariableParams{Key: key, Type: "money", Unit: ptr("USD"), Description: "integration", GlobalDefault: json.RawMessage(`100`), OverridableAt: []string{"site", "project"}, OwnerRole: "policy_owner", CreatedBy: "it-test"}); err != nil {
		t.Fatal(err)
	}
	site, err := svc.CreateSite(ctx, domain.Actor{UserID: "it-test"}, domain.CreateSiteInput{Code: "PV-" + suffix(), Name: "policy site", Country: "CA", TimeZone: "America/Toronto", Currency: "CAD"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := svc.CreateProject(ctx, domain.Actor{UserID: "it-test"}, domain.CreateProjectInput{Code: "P-PV-" + suffix(), Name: "policy project", SiteID: site.SiteID.String(), ProjectType: "compliance"})
	if err != nil {
		t.Fatal(err)
	}
	scope := policy.Scope{ProjectID: p.ProjectID.String(), SiteID: site.SiteID.String(), ProjectType: "compliance"}
	got, err := policy.Resolve(ctx, q, key, scope, time.Now())
	if err != nil || got.ScopeKind != "default" || string(got.Value) != "100" {
		t.Fatalf("default expected: %v %+v", err, got)
	}
	if _, err := q.InsertPolicyValue(ctx, db.InsertPolicyValueParams{ValueID: ids.New(), VariableKey: key, ScopeKind: "site", ScopeID: site.SiteID.String(), Value: json.RawMessage(`250`), EffectiveFrom: time.Now().Add(-time.Minute), SetBy: "it-test", Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	got, err = policy.Resolve(ctx, q, key, scope, time.Now())
	if err != nil || got.ScopeKind != "site" || string(got.Value) != "250" {
		t.Fatalf("site override expected: %v %+v", err, got)
	}
	future := time.Now().Add(time.Hour)
	if _, err := q.InsertPolicyValue(ctx, db.InsertPolicyValueParams{ValueID: ids.New(), VariableKey: key, ScopeKind: "project", ScopeID: p.ProjectID.String(), Value: json.RawMessage(`999`), EffectiveFrom: future, SetBy: "it-test", Reason: "future"}); err != nil {
		t.Fatal(err)
	}
	got, _ = policy.Resolve(ctx, q, key, scope, time.Now())
	if got.ScopeKind != "site" {
		t.Fatalf("a future project value must not apply yet: %+v", got)
	}
	got, _ = policy.Resolve(ctx, q, key, scope, future.Add(time.Minute))
	if got.ScopeKind != "project" || string(got.Value) != "999" {
		t.Fatalf("project value should apply once effective: %+v", got)
	}
	if _, err := policy.Resolve(ctx, q, "no.such.key", scope, time.Now()); err == nil {
		t.Fatal("unknown key should error")
	}
	_ = fmt.Sprintf
}

func ptr(s string) *string { return &s }
