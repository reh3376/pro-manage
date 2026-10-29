//go:build integration

// The ERP mirror and the budget release against Postgres and the ERPi_STUB module:
//
//	TEST_DATABASE_URL=... ERPI_TEST_ADDR=127.0.0.1:7110 ERPI_TEST_HTTP=http://127.0.0.1:7111 \
//	go test -tags integration ./internal/budget/
//
// A project is registered in the stub's ledger, synced (versions, actuals,
// commitments land as mirror rows and re-sync is idempotent), an actual posted
// at the ERP shows up on the next pull, a release is requested in a
// transaction and performed by the outbox handler (budget_ref on the row, the
// CAR, and the audit log; a repeat is a no-op), and a release against an
// unknown ERP project fails permanently and is audited as such.
package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/connectors/erpi"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
)

const tenant = "it-budget"

func setup(t *testing.T) (*Service, string) {
	t.Helper()
	dsn, addr := os.Getenv("TEST_DATABASE_URL"), os.Getenv("ERPI_TEST_ADDR")
	if dsn == "" || addr == "" {
		t.Skip("TEST_DATABASE_URL and ERPI_TEST_ADDR required")
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
	ec, err := erpi.Dial(addr, "stub")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ec.Close() })
	httpBase := os.Getenv("ERPI_TEST_HTTP")
	if httpBase == "" {
		httpBase = "http://127.0.0.1:7111"
	}
	return &Service{Store: st, Chain: chain, Erpi: ec, Tenant: tenant, Log: log}, httpBase
}

func devPost(t *testing.T, url string, body any) {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		t.Fatalf("POST %s: %d", url, resp.StatusCode)
	}
}

// newProject makes a tenant-scoped project in pm-core and the same project in the stub ledger.
func newProject(t *testing.T, s *Service, httpBase, currency, baseline string) (db.Project, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	q := s.Store.Queries()
	code := fmt.Sprintf("IT-BUD-%d", time.Now().UnixNano()%1_000_000_000)
	site, err := q.CreateSite(ctx, db.CreateSiteParams{SiteID: ids.New(), TenantID: tenant, Code: code + "-S", Name: "it site", Country: "US", TimeZone: "UTC", Currency: currency, UnitSystem: "imperial",
		RegulatoryRegime: []string{}, PrivacyRegime: []string{}, Address: json.RawMessage(`{}`), CreatedBy: "it"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := q.CreateProject(ctx, db.CreateProjectParams{ProjectID: ids.New(), TenantID: tenant, Code: code, Name: "it project", SiteID: site.SiteID, Currency: currency, OwnerUserID: "it.owner",
		LifecycleState: "plan", FelStage: "fel3", ProjectType: "efficiency", HazardousAreaClassification: "none", OnboardingLevel: "full", TpmPillarTags: []string{}, RegulatoryStamps: []string{}, Tags: []string{}, CreatedBy: "it"})
	if err != nil {
		t.Fatal(err)
	}
	devPost(t, httpBase+"/dev/projects", map[string]any{"ref": code, "code": code, "name": "it project", "currency": currency, "status": "execution", "baseline": baseline})
	if err := q.SetProjectERPRef(ctx, db.SetProjectERPRefParams{TenantID: tenant, ProjectID: p.ProjectID, ErpProjectRef: &code, UpdatedBy: "it"}); err != nil {
		t.Fatal(err)
	}
	p, _ = q.GetProject(ctx, db.GetProjectParams{TenantID: tenant, ProjectID: p.ProjectID})
	return p, site.SiteID
}

func newCAR(t *testing.T, s *Service, p db.Project, amount string) db.CapitalAuthorization {
	t.Helper()
	ctx := context.Background()
	car, err := s.Store.Queries().CreateCAR(ctx, db.CreateCARParams{CarID: ids.New(), TenantID: tenant, ProjectID: p.ProjectID, Code: "CR-IT-" + ids.New().String()[24:], Kind: "full",
		AmountRequested: amount, Currency: p.Currency, Basis: "tic", BusinessCase: "it", FelStageAtSubmission: &p.FelStage, CreatedBy: "it.owner"})
	if err != nil {
		t.Fatal(err)
	}
	return car
}

func TestMirrorSyncAndRelease(t *testing.T) {
	s, httpBase := setup(t)
	ctx := context.Background()
	q := s.Store.Queries()
	p, _ := newProject(t, s, httpBase, "USD", "1000000.00")

	// First pull: the baseline arrives; actuals and commitments are empty but the feeds answered.
	res, err := s.SyncProject(ctx, p)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Baselines != 1 || len(res.Skipped) != 0 && res.Skipped[0] != "eac" {
		t.Fatalf("first sync: %+v", res)
	}
	sum, err := s.ProjectSummary(ctx, mustProject(t, q, p.ProjectID))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Baseline == nil || sum.Baseline.Total != "1000000.00" || sum.SyncedAt == nil {
		t.Fatalf("summary after first sync: %+v", sum)
	}

	// An actual posted at the ERP shows up on the next pull; the pull is idempotent on the ERP refs.
	devPost(t, httpBase+"/dev/projects/"+p.Code+"/actuals", map[string]any{"ref": "ACT-IT-1", "wbs_path": p.Code, "account_code": "ENG", "amount": "12345.67"})
	for i := 0; i < 2; i++ {
		if _, err := s.SyncProject(ctx, p); err != nil {
			t.Fatalf("sync %d: %v", i, err)
		}
	}
	act, _ := q.SumBudgetActuals(ctx, db.SumBudgetActualsParams{TenantID: tenant, ProjectID: p.ProjectID})
	if act.Rows != 1 || act.Total != "12345.67" {
		t.Fatalf("actuals after re-sync: rows=%d total=%s", act.Rows, act.Total)
	}
	sum, _ = s.ProjectSummary(ctx, mustProject(t, q, p.ProjectID))
	if sum.RemainingToSpend != "987654.33" {
		t.Fatalf("remaining to spend: %s", sum.RemainingToSpend)
	}

	// Release: requested in a transaction (the approval's), performed by the outbox handler.
	car := newCAR(t, s, p, "250000.00")
	var releaseID uuid.UUID
	var payload []byte
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		ev, err := s.Chain.Append(ctx, q, audit.Event{TenantID: tenant, ProjectID: &p.ProjectID, ActorUserID: "it.owner", ActionType: "car.approve", SubjectType: "capital_authorization", SubjectID: car.CarID.String()})
		if err != nil {
			return err
		}
		releaseID, err = s.RequestRelease(ctx, q, ReleaseRequest{ProjectID: p.ProjectID, ProjectRef: p.Code, CARID: car.CarID, CARCode: car.Code, Amount: "250000.00", Currency: "USD", Description: "CAR " + car.Code, RequestedBy: "it.owner", AuditID: ev.AuditID},
			func(kind, sourceRef string, pl any) error {
				if kind != OutboxKind {
					t.Fatalf("kind %s", kind)
				}
				payload, _ = json.Marshal(pl)
				return nil
			})
		return err
	})
	if err != nil {
		t.Fatalf("request release: %v", err)
	}
	// A second request for the same CAR returns the same release and enqueues nothing.
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		again, err := s.RequestRelease(ctx, q, ReleaseRequest{ProjectID: p.ProjectID, ProjectRef: p.Code, CARID: car.CarID, Amount: "250000.00", Currency: "USD", RequestedBy: "it.owner", AuditID: uuid.Nil},
			func(string, string, any) error { t.Fatal("second request must not enqueue"); return nil })
		if err != nil {
			return err
		}
		if again != releaseID {
			t.Fatalf("second request returned %s, want %s", again, releaseID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReleaseAtERP(ctx, payload); err != nil {
		t.Fatalf("release at ERP: %v", err)
	}
	if err := s.ReleaseAtERP(ctx, payload); err != nil { // a retried outbox row is a no-op
		t.Fatalf("second release at ERP: %v", err)
	}
	rel, err := s.ReleaseForCAR(ctx, car.CarID)
	if err != nil || rel == nil {
		t.Fatalf("release row: %v %v", rel, err)
	}
	if rel.Status != "released" || rel.ErpBudgetRef == nil || *rel.ErpBudgetRef == "" || rel.ReleasedAt == nil {
		t.Fatalf("release row after release: %+v", rel)
	}
	got, _ := q.GetCAR(ctx, db.GetCARParams{TenantID: tenant, CarID: car.CarID})
	if got.ErpBudgetRef == nil || *got.ErpBudgetRef != *rel.ErpBudgetRef {
		t.Fatalf("CAR erp_budget_ref: %v, want %s", got.ErpBudgetRef, *rel.ErpBudgetRef)
	}
	if !hasAudit(t, q, p.ProjectID, car.CarID, "car.budget_released") {
		t.Fatal("car.budget_released not audited")
	}
	// The release shows in the mirror as an approved budget version on the next pull, and in the summary.
	if _, err := s.SyncProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	sum, _ = s.ProjectSummary(ctx, mustProject(t, q, p.ProjectID))
	if sum.ReleasedTotal != "250000.00" || len(sum.Releases) != 1 {
		t.Fatalf("summary after release: released=%s releases=%d", sum.ReleasedTotal, len(sum.Releases))
	}
	found := false
	for _, v := range sum.Versions {
		if v.VersionRef == *rel.ErpBudgetRef {
			found = true
		}
	}
	if !found {
		t.Fatalf("mirror lacks the released budget version %s", *rel.ErpBudgetRef)
	}
}

func TestReleaseAgainstUnknownERPProjectFailsPermanently(t *testing.T) {
	s, httpBase := setup(t)
	ctx := context.Background()
	q := s.Store.Queries()
	p, _ := newProject(t, s, httpBase, "USD", "")
	car := newCAR(t, s, p, "10.00")
	var payload []byte
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		ev, err := s.Chain.Append(ctx, q, audit.Event{TenantID: tenant, ProjectID: &p.ProjectID, ActorUserID: "it.owner", ActionType: "car.approve", SubjectType: "capital_authorization", SubjectID: car.CarID.String()})
		if err != nil {
			return err
		}
		_, err = s.RequestRelease(ctx, q, ReleaseRequest{ProjectID: p.ProjectID, ProjectRef: "P-NOT-IN-ERP", CARID: car.CarID, Amount: "10.00", Currency: "USD", RequestedBy: "it.owner", AuditID: ev.AuditID},
			func(_, _ string, pl any) error { payload, _ = json.Marshal(pl); return nil })
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.ReleaseAtERP(ctx, payload)
	if err == nil || !erpiPermanent(err) {
		t.Fatalf("want a permanent error, got %v", err)
	}
	rel, _ := s.ReleaseForCAR(ctx, car.CarID)
	if rel == nil || rel.Status != "failed" || rel.LastError == nil {
		t.Fatalf("release row: %+v", rel)
	}
	if !hasAudit(t, q, p.ProjectID, car.CarID, "car.budget_release_failed") {
		t.Fatal("car.budget_release_failed not audited")
	}
}

func TestSyncWithoutERPRefIsNotFound(t *testing.T) {
	s, _ := setup(t)
	p := db.Project{ProjectID: ids.New(), Code: "no-ref", Currency: "USD"}
	if _, err := s.SyncProject(context.Background(), p); err == nil || !errors.Is(err, erpi.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func erpiPermanent(err error) bool {
	return errors.Is(err, erpi.ErrRejected) || errors.Is(err, erpi.ErrNotSupported) || errors.Is(err, erpi.ErrNotFound)
}

func mustProject(t *testing.T, q *db.Queries, id uuid.UUID) db.Project {
	t.Helper()
	p, err := q.GetProject(context.Background(), db.GetProjectParams{TenantID: tenant, ProjectID: id})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func hasAudit(t *testing.T, q *db.Queries, projectID, carID uuid.UUID, action string) bool {
	t.Helper()
	rows, err := q.ListAuditEventsForProject(context.Background(), db.ListAuditEventsForProjectParams{TenantID: tenant, ProjectID: &projectID, Limit: 200})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.ActionType == action && r.SubjectID == carID.String() {
			return true
		}
	}
	return false
}
