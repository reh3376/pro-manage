//go:build integration

// Attribution re-key (B-11) against Postgres:
//
//	TEST_DATABASE_URL=... go test -tags integration -run Identity ./internal/domain/
//
// A user signs in, is invited to a project and granted a section, and lands in
// a pool; the IdP renames the login handle; the next login moves every row to
// the new handle by the stable id, the ACL sees the same access under the new
// name, the audit log records the rename with the actor's id, and the chain
// (now version 2 rows beside any version 1 rows) still verifies.
package domain_test

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/reh3376/pro-manage/services/pm-core/internal/audit"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
	"github.com/reh3376/pro-manage/services/pm-core/internal/ids"
	"github.com/reh3376/pro-manage/services/pm-core/internal/store"
)

// identityService needs Postgres only (no MDEMG, no Sigi).
func identityService(t *testing.T) *domain.Service {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
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
	return &domain.Service{Store: st, Chain: chain, Tenant: tenant, Log: log}
}

func TestIdentityRekeyFollowsRename(t *testing.T) {
	svc := identityService(t)
	ctx := context.Background()
	q := svc.Store.Queries()
	sfx := strings.ToLower(ids.New().String()[24:])
	subject, oldName, newName := "sub-"+sfx, "pat.old-"+sfx, "pat.new-"+sfx
	owner := domain.Actor{UserID: "it-owner-" + sfx, SourceIP: "127.0.0.1", RequestID: "req-id"}

	// First login creates the identity; the owner then invites, grants, and pools the handle.
	user, err := svc.RecordLogin(ctx, domain.LoginInput{Subject: subject, Username: oldName, Email: oldName + "@example.test", DisplayName: "Pat", Pools: []string{"finance_approver_eligible"}})
	if err != nil {
		t.Fatal(err)
	}
	site, err := svc.CreateSite(ctx, owner, domain.CreateSiteInput{Code: "ID-" + strings.ToUpper(sfx), Name: "identity site", Country: "US", TimeZone: "UTC", Currency: "USD"})
	if err != nil {
		t.Fatal(err)
	}
	project, err := svc.CreateProject(ctx, owner, domain.CreateProjectInput{Code: "P-ID-" + strings.ToUpper(sfx), Name: "identity project", SiteID: site.SiteID.String(), ProjectType: "reliability", FelStage: "fel1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddMember(ctx, owner, project.ProjectID, domain.MemberInput{UserID: oldName, BaseRole: "viewer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddGrant(ctx, owner, project.ProjectID, domain.GrantInput{UserID: oldName, ScopeKind: "section", ScopeTarget: "section:budget", Permission: "read", Reason: "it"}); err != nil {
		t.Fatal(err)
	}
	m, err := q.GetProjectMember(ctx, db.GetProjectMemberParams{ProjectID: project.ProjectID, UserID: oldName})
	if err != nil || m.UserRef == nil || *m.UserRef != user.UserID {
		t.Fatalf("membership not linked to the identity: %+v %v", m, err)
	}
	before, err := svc.Access(ctx, domain.Actor{UserID: oldName}, project.ProjectID)
	if err != nil || before.BaseRole != "viewer" || len(before.Grants) != 1 {
		t.Fatalf("access before rename: %+v %v", before, err)
	}

	// The IdP renames the handle; the next login carries the same subject with the new name.
	renamed, err := svc.RecordLogin(ctx, domain.LoginInput{Subject: subject, Username: newName, Email: newName + "@example.test", DisplayName: "Pat", Pools: []string{"finance_approver_eligible"}})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.UserID != user.UserID || renamed.Username != newName {
		t.Fatalf("rename did not keep the identity: %+v", renamed)
	}
	after, err := svc.Access(ctx, domain.Actor{UserID: newName}, project.ProjectID)
	if err != nil || after.BaseRole != "viewer" || len(after.Grants) != 1 {
		t.Fatalf("access after rename: %+v %v", after, err)
	}
	gone, err := svc.Access(ctx, domain.Actor{UserID: oldName}, project.ProjectID)
	if err != nil || gone.BaseRole != "" || len(gone.Grants) != 0 {
		t.Fatalf("old handle still has access: %+v %v", gone, err)
	}
	pools, err := q.PoolMembers(ctx, db.PoolMembersParams{Pool: "finance_approver_eligible", SiteID: nil})
	if err != nil {
		t.Fatal(err)
	}
	seenNew, seenOld := false, false
	for _, p := range pools {
		seenNew = seenNew || p == newName
		seenOld = seenOld || p == oldName
	}
	if !seenNew || seenOld {
		t.Fatalf("pool rows after rename: new=%v old=%v", seenNew, seenOld)
	}

	// The rename is audited with the actor's stable id, and the chain verifies with version 2 rows.
	all, err := q.ListAuditEventsInOrder(ctx, db.ListAuditEventsInOrderParams{TenantID: svc.Tenant, Seq: 0, Limit: 100000})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range all {
		if r.ActionType == "user.renamed" && r.SubjectID == user.UserID.String() {
			found = true
			if r.ActorID == nil || *r.ActorID != user.UserID || r.ChainVersion != 2 {
				t.Fatalf("rename audit row lacks the actor id or chain version: %+v", r)
			}
		}
	}
	if !found {
		t.Fatal("user.renamed not audited")
	}
	res, err := svc.Chain.Verify(ctx, q, svc.Tenant)
	if err != nil || !res.Valid {
		t.Fatalf("chain verify after re-key: %+v %v", res, err)
	}
}
