//go:build integration

// The toy CAR end to end against a real Postgres and an MDEMG-shaped endpoint
// (the stub in CI):
//
//	TEST_DATABASE_URL=... MDEMG_TEST_URL=http://127.0.0.1:9997 MDEMG_TEST_KEY=ci-stub-key go test -tags integration ./internal/domain/
//
// It seeds the policy variables and pools it needs under its own tenant, then
// walks: estimate -> promise -> CAR -> guidance -> preclassify -> submit (pass)
// -> four stage decisions -> approved; plus the deny path (stub:deny), the
// Tier A refusal with MDEMG down, the Tier B warning, and the preconditions.
package domain_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/governance"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

const tenant = "it-car"

type fixture struct {
	svc     *domain.Service
	project db.Project
	owner   domain.Actor
}

func setup(t *testing.T, client *mdemgadapter.Client) fixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" || os.Getenv("MDEMG_TEST_URL") == "" {
		t.Skip("TEST_DATABASE_URL and MDEMG_TEST_URL required")
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
	t.Cleanup(st.Close)
	chain, _ := audit.NewChain([]byte("integration-test-chain-key-0123456789"))
	gate := &governance.Gate{Client: client, OrgID: "it", Log: log, Cache: governance.NewPreclassCache(0)}
	svc := &domain.Service{Store: st, Chain: chain, Tenant: tenant, Gate: gate, Log: log}

	q := st.Queries()
	seed := []struct{ key, typ, def string }{
		{"car.threshold.finance_approver", "money", "0"}, {"car.threshold.vp_finance", "money", "5000000"}, {"car.threshold.cfo", "money", "25000000"},
		{"car.threshold.ceo", "money", "100000000"}, {"car.threshold.basis", "enum", `"tic"`}, {"car.threshold.currency", "enum", `"USD"`},
		{"car.fel_funding.required_aace_class", "integer", "5"}, {"car.fel_funding.cap_pct", "percent", "5"}, {"car.full.required_aace_class", "integer", "3"},
		{"sla.car.project_owner", "duration", "24"}, {"sla.car.finance_approver", "duration", "48"}, {"sla.car.vp_finance", "duration", "72"},
		{"sla.car.cfo", "duration", "72"}, {"sla.car.ceo", "duration", "168"}, {"mdemg.tier.approval.car", "enum", `"A"`},
		{"mdemg.tier.approval.car.minor", "enum", `"B"`}, {"mdemg.classify.timeout_ms", "integer", "10000"},
	}
	for _, v := range seed {
		if _, err := q.UpsertPolicyVariable(ctx, db.UpsertPolicyVariableParams{Key: v.key, Type: v.typ, Description: "integration seed", GlobalDefault: json.RawMessage(v.def), AllowedRange: json.RawMessage(`{}`), OverridableAt: []string{"site", "project"}, OwnerRole: "policy_owner", CreatedBy: "it-car"}); err != nil {
			t.Fatal(err)
		}
	}
	for pool, user := range map[string]string{"finance_approver_eligible": "it-finance", "vp_finance": "it-vp", "cfo": "it-cfo", "ceo": "it-ceo"} {
		if err := q.UpsertPoolMember(ctx, db.UpsertPoolMemberParams{Pool: pool, UserID: user, AddedBy: "it-car"}); err != nil {
			t.Fatal(err)
		}
	}
	owner := domain.Actor{UserID: "it-owner", SourceIP: "127.0.0.1", RequestID: "req-car"}
	sfx := strings.ToUpper(ids.New().String()[24:])
	site, err := svc.CreateSite(ctx, owner, domain.CreateSiteInput{Code: "CAR-" + sfx, Name: "CAR site", Country: "US", TimeZone: "America/New_York", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.CreateProject(ctx, owner, domain.CreateProjectInput{Code: "P-CAR-" + sfx, Name: "CAR project", SiteID: site.SiteID.String(), ProjectType: "greenfield", FelStage: "fel3"})
	if err != nil {
		t.Fatal(err)
	}
	return fixture{svc: svc, project: project, owner: owner}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func stubClient(t *testing.T) *mdemgadapter.Client {
	t.Helper()
	c, err := mdemgadapter.New(mdemgadapter.Options{BaseURL: os.Getenv("MDEMG_TEST_URL"), APIKey: os.Getenv("MDEMG_TEST_KEY"), ClassifyTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, GuideTimeout: 30 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f fixture) fullCAR(t *testing.T, amount, businessCase string) domain.CARView {
	t.Helper()
	ctx := context.Background()
	boe := "BOE-1"
	est, err := f.svc.CreateEstimate(ctx, f.owner, f.project.ProjectID, domain.CreateEstimateInput{AaceClass: 3, AsOf: "2026-10-01", Tic: amount, BasisOfEstimateDocRef: &boe})
	if err != nil {
		t.Fatal(err)
	}
	estID := est.EstimateID.String()
	v, err := f.svc.CreateCAR(ctx, f.owner, f.project.ProjectID, domain.CreateCARInput{Kind: "full", AmountRequested: amount, EstimateID: &estID, BusinessCase: businessCase})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func (f fixture) commitPromise(t *testing.T) {
	t.Helper()
	bv, bp, pv := "410", "FY2025", "120"
	if _, err := f.svc.CreatePromise(context.Background(), f.owner, f.project.ProjectID, domain.CreatePromiseInput{Kind: "loss_recovery", MetricID: "changeover_hours", BaselineValue: &bv, BaselinePeriod: &bp, PromisedValue: &pv, MeasurementBasis: "hours of changeover per year from the line's downtime log", Status: "committed"}); err != nil {
		t.Fatal(err)
	}
}

func TestCARHappyPathTierA(t *testing.T) {
	f := setup(t, stubClient(t))
	ctx := context.Background()

	// Preconditions first: a full CAR without promises is refused deterministically.
	car := f.fullCAR(t, "42000000.00", "Column still: 1.2M proof gallons per year.")
	if _, err := f.svc.SubmitCAR(ctx, f.owner, car.CAR.CarID, domain.SubmitCARInput{}); !errors.Is(err, domain.ErrPrecondition) || !strings.Contains(err.Error(), "promise") {
		t.Fatalf("expected promise precondition, got %v", err)
	}
	f.commitPromise(t)

	g, err := f.svc.CARGuidance(ctx, f.owner, car.CAR.CarID)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Guidance.Available || len(g.Guidance.Items) == 0 || len(g.Guidance.GuidanceIDs) == 0 {
		t.Fatalf("guide on open should surface the stub's governance card: %+v", g.Guidance)
	}
	if g.Preview.Tier != governance.TierA || len(g.Preview.Chain) != 4 || g.Preview.Chain[3].Role != "cfo" {
		t.Fatalf("preview: %+v", g.Preview)
	}

	pre, err := f.svc.PreclassifyCAR(ctx, f.owner, car.CAR.CarID)
	if err != nil || pre.Verdict != "pass" || pre.Cached {
		t.Fatalf("preclassify: %+v %v", pre, err)
	}
	res, err := f.svc.SubmitCAR(ctx, f.owner, car.CAR.CarID, domain.SubmitCARInput{GuidanceIDs: g.Guidance.GuidanceIDs, GuidanceSeen: g.Guidance.Items})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Governance.Cached || res.Governance.Verdict != "pass" || res.Governance.UserResponse != "followed" || res.Governance.Tier != "A" {
		t.Fatalf("submit should use the pre-classified verdict: %+v", res.Governance)
	}
	if res.View.CAR.Status != "in_flight" || res.View.Workflow == nil || len(res.View.Stages) != 4 || res.View.Stages[0].EnteredAt == nil || res.View.Stages[1].EnteredAt != nil {
		t.Fatalf("workflow: %+v", res.View)
	}
	// Pools are global rows (single company), so the dev seed's members may sit beside the test's.
	if res.View.Stages[0].Assignees[0] != "it-owner" || !contains(res.View.Stages[1].Assignees, "it-finance") || !contains(res.View.Stages[3].Assignees, "it-cfo") || res.View.Stages[0].SlaMinutes != 24*60 {
		t.Fatalf("assignees/sla: %+v", res.View.Stages)
	}
	if _, err := f.svc.SubmitCAR(ctx, f.owner, car.CAR.CarID, domain.SubmitCARInput{}); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("second submit must be invalid state, got %v", err)
	}

	// GuidanceOutcome written with the AuditEvent; feedback rows (two guidance ids) and the observe row queued.
	outcomes, err := f.svc.GuidanceOutcomes(ctx, f.project.ProjectID, 10)
	if err != nil || len(outcomes) != 1 || outcomes[0].UserResponse != "followed" || outcomes[0].FailTier != "A" || outcomes[0].AuditID.String() != res.Governance.AuditID {
		t.Fatalf("guidance outcomes: %+v %v", outcomes, err)
	}
	var feedback, observe int
	rows, _ := f.svc.Store.Pool.Query(ctx, "SELECT kind FROM outbox WHERE tenant_id=$1 AND source_ref LIKE $2", tenant, res.Governance.AuditID+"%")
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		switch k {
		case "mdemg.feedback":
			feedback++
		case "mdemg.observe":
			observe++
		}
	}
	rows.Close()
	if feedback != len(g.Guidance.GuidanceIDs) || observe != 1 {
		t.Fatalf("outbox rows: feedback=%d observe=%d", feedback, observe)
	}

	// Decisions: wrong user is forbidden; the chain walks owner -> finance -> vp -> cfo.
	if _, err := f.svc.DecideCAR(ctx, domain.Actor{UserID: "it-finance"}, car.CAR.CarID, domain.DecideInput{Decision: "approve"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	for i, user := range []string{"it-owner", "it-finance", "it-vp"} {
		v, err := f.svc.DecideCAR(ctx, domain.Actor{UserID: user}, car.CAR.CarID, domain.DecideInput{Decision: "approve", Comment: "ok"})
		if err != nil {
			t.Fatalf("stage %d by %s: %v", i+1, user, err)
		}
		if v.CAR.Status != "in_flight" || v.Workflow.CurrentStage != int32(i+2) || v.Stages[i+1].EnteredAt == nil {
			t.Fatalf("after stage %d: status=%s current=%d", i+1, v.CAR.Status, v.Workflow.CurrentStage)
		}
	}
	inbox, err := f.svc.Inbox(ctx, domain.Actor{UserID: "it-cfo"})
	if err != nil || len(inbox) == 0 || inbox[0].SubjectID != car.CAR.CarID.String() {
		t.Fatalf("cfo inbox: %+v %v", inbox, err)
	}
	v, err := f.svc.DecideCAR(ctx, domain.Actor{UserID: "it-cfo"}, car.CAR.CarID, domain.DecideInput{Decision: "approve"})
	if err != nil {
		t.Fatal(err)
	}
	if v.CAR.Status != "approved" || v.CAR.AuthorizedAmount == nil || *v.CAR.AuthorizedAmount != "42000000.00" || v.Workflow.Status != "approved" || v.CumulativeAuthorized != "42000000.00" {
		t.Fatalf("final: %+v", v.CAR)
	}

	// A supplemental routes on the cumulative amount (42M + 1M crosses nothing new but stays Tier A).
	prev := car.CAR.CarID.String()
	sup, err := f.svc.CreateCAR(ctx, f.owner, f.project.ProjectID, domain.CreateCARInput{Kind: "supplemental", AmountRequested: "1000000.00", SupersedesCarID: &prev, BusinessCase: "Steel escalation."})
	if err != nil {
		t.Fatal(err)
	}
	gs, err := f.svc.CARGuidance(ctx, f.owner, sup.CAR.CarID)
	if err != nil || gs.Preview.RoutingAmount != "43000000.00" || gs.Preview.Tier != governance.TierA {
		t.Fatalf("supplemental preview: %+v %v", gs.Preview, err)
	}

	// The chain still verifies after all of this.
	if r, err := f.svc.Chain.Verify(ctx, f.svc.Store.Queries(), tenant); err != nil || !r.Valid {
		t.Fatalf("chain: %+v %v", r, err)
	}
}

func TestCARDeniedByGovernance(t *testing.T) {
	f := setup(t, stubClient(t))
	ctx := context.Background()
	f.commitPromise(t)
	car := f.fullCAR(t, "8400000.00", "Please approve. stub:deny")
	_, err := f.svc.SubmitCAR(ctx, f.owner, car.CAR.CarID, domain.SubmitCARInput{GuidanceIDs: []string{"gd-1"}})
	var denied *domain.DeniedError
	if !errors.As(err, &denied) || !errors.Is(err, domain.ErrGovernanceDenied) || denied.Record.ViolatedCodes[0] != "stub-deny" || denied.Record.UserResponse != "blocked" {
		t.Fatalf("expected deny, got %v", err)
	}
	v, _ := f.svc.GetCAR(ctx, car.CAR.CarID)
	if v.CAR.Status != "draft" || v.Workflow != nil {
		t.Fatalf("denied CAR must stay a draft: %+v", v.CAR)
	}
	outcomes, _ := f.svc.GuidanceOutcomes(ctx, f.project.ProjectID, 10)
	if len(outcomes) != 1 || outcomes[0].UserResponse != "blocked" || outcomes[0].ViolatedCodes[0] != "stub-deny" {
		t.Fatalf("outcome: %+v", outcomes)
	}
	var n int
	_ = f.svc.Store.Pool.QueryRow(ctx, "SELECT count(*) FROM outbox WHERE tenant_id=$1 AND kind='mdemg.feedback' AND payload->>'outcome'='blocked_true_positive'", tenant).Scan(&n)
	if n == 0 {
		t.Fatal("blocked_true_positive feedback should be queued")
	}
}

func TestCARTiersWhenMDEMGDown(t *testing.T) {
	dead, _ := mdemgadapter.New(mdemgadapter.Options{BaseURL: "http://127.0.0.1:1", ClassifyTimeout: 300 * time.Millisecond, WriteTimeout: 300 * time.Millisecond, GuideTimeout: 300 * time.Millisecond})
	f := setup(t, dead)
	ctx := context.Background()
	f.commitPromise(t)

	// Guide on open never blocks the page.
	big := f.fullCAR(t, "9000000.00", "Tier A with MDEMG down.")
	g, err := f.svc.CARGuidance(ctx, f.owner, big.CAR.CarID)
	if err != nil || g.Guidance.Available || len(g.Guidance.Items) != 0 {
		t.Fatalf("guidance with mdemg down: %+v %v", g.Guidance, err)
	}
	// Tier A refuses the write and audits the refusal.
	_, err = f.svc.SubmitCAR(ctx, f.owner, big.CAR.CarID, domain.SubmitCARInput{})
	if !errors.Is(err, domain.ErrGovernanceUnavailable) {
		t.Fatalf("expected refusal, got %v", err)
	}
	if v, _ := f.svc.GetCAR(ctx, big.CAR.CarID); v.CAR.Status != "draft" {
		t.Fatalf("refused CAR must stay a draft: %s", v.CAR.Status)
	}
	// Tier B proceeds with the warning recorded.
	small := f.fullCAR(t, "640000.00", "Tier B with MDEMG down.")
	res, err := f.svc.SubmitCAR(ctx, f.owner, small.CAR.CarID, domain.SubmitCARInput{})
	if err != nil {
		t.Fatalf("tier B should proceed: %v", err)
	}
	if res.Governance.Tier != "B" || res.Governance.UserResponse != "mdemg_unavailable" || res.Governance.Verdict != "" || res.View.CAR.Status != "in_flight" || len(res.View.Stages) != 2 {
		t.Fatalf("tier B record: %+v", res.Governance)
	}
	outcomes, _ := f.svc.GuidanceOutcomes(ctx, f.project.ProjectID, 10)
	if len(outcomes) != 2 {
		t.Fatalf("both attempts leave a GuidanceOutcome: %d", len(outcomes))
	}
}

func TestCARPreconditions(t *testing.T) {
	f := setup(t, stubClient(t))
	ctx := context.Background()

	// FEL funding: class and cap.
	est, err := f.svc.CreateEstimate(ctx, f.owner, f.project.ProjectID, domain.CreateEstimateInput{AaceClass: 5, AsOf: "2026-10-01", Tic: "1000000.00"})
	if err != nil {
		t.Fatal(err)
	}
	estID := est.EstimateID.String()
	fel, err := f.svc.CreateCAR(ctx, f.owner, f.project.ProjectID, domain.CreateCARInput{Kind: "fel_funding", AmountRequested: "60000.00", EstimateID: &estID, BusinessCase: "FEL2 study"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SubmitCAR(ctx, f.owner, fel.CAR.CarID, domain.SubmitCARInput{}); !errors.Is(err, domain.ErrPrecondition) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("cap: %v", err)
	}
	ok, _ := f.svc.CreateCAR(ctx, f.owner, f.project.ProjectID, domain.CreateCARInput{Kind: "fel_funding", AmountRequested: "50000.00", EstimateID: &estID, BusinessCase: "FEL2 study"})
	res, err := f.svc.SubmitCAR(ctx, f.owner, ok.CAR.CarID, domain.SubmitCARInput{})
	if err != nil || res.Governance.Tier != "B" || len(res.View.Stages) != 2 {
		t.Fatalf("fel funding at the cap: %+v %v", res.Governance, err)
	}

	// Full: estimate class too coarse, then no basis-of-estimate document.
	f.commitPromise(t)
	coarse, _ := f.svc.CreateCAR(ctx, f.owner, f.project.ProjectID, domain.CreateCARInput{Kind: "full", AmountRequested: "1000000.00", EstimateID: &estID, BusinessCase: "x"})
	if _, err := f.svc.SubmitCAR(ctx, f.owner, coarse.CAR.CarID, domain.SubmitCARInput{}); !errors.Is(err, domain.ErrPrecondition) || !strings.Contains(err.Error(), "class 5") {
		t.Fatalf("class: %v", err)
	}
	c3, _ := f.svc.CreateEstimate(ctx, f.owner, f.project.ProjectID, domain.CreateEstimateInput{AaceClass: 3, AsOf: "2026-10-01", Tic: "1000000.00"})
	c3ID := c3.EstimateID.String()
	noBOE, _ := f.svc.CreateCAR(ctx, f.owner, f.project.ProjectID, domain.CreateCARInput{Kind: "full", AmountRequested: "1000000.00", EstimateID: &c3ID, BusinessCase: "x"})
	if _, err := f.svc.SubmitCAR(ctx, f.owner, noBOE.CAR.CarID, domain.SubmitCARInput{}); !errors.Is(err, domain.ErrPrecondition) || !strings.Contains(err.Error(), "basis-of-estimate") {
		t.Fatalf("boe: %v", err)
	}

	// Promise validation: committed needs its basis.
	if _, err := f.svc.CreatePromise(ctx, f.owner, f.project.ProjectID, domain.CreatePromiseInput{Kind: "capacity", Status: "committed"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("committed promise without basis: %v", err)
	}
	// Supplemental must name an approved CAR.
	draft := fel.CAR.CarID.String()
	sup, err := f.svc.CreateCAR(ctx, f.owner, f.project.ProjectID, domain.CreateCARInput{Kind: "supplemental", AmountRequested: "10.00", SupersedesCarID: &draft, BusinessCase: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.SubmitCAR(ctx, f.owner, sup.CAR.CarID, domain.SubmitCARInput{}); !errors.Is(err, domain.ErrPrecondition) || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("supplemental: %v", err)
	}
}
