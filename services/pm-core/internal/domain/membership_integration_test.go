//go:build integration

package domain_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/reh3376/pro-manage/services/pm-core/internal/acl"
	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/domain"
)

// TestMembershipGrantsAndAccess walks FSD §04 through the domain: the owner
// member created with the project, invitations, grants (allow and deny),
// the last-owner rule, visibility, and stage-assignee reads after a submit.
func TestMembershipGrantsAndAccess(t *testing.T) {
	f := setup(t, stubClient(t))
	ctx := context.Background()
	pid := f.project.ProjectID

	// The owner is a member from creation and sees everything.
	c, err := f.svc.Access(ctx, f.owner, pid)
	if err != nil || c.BaseRole != acl.Owner || !acl.Decide(c, acl.Budget, acl.Write).Allowed {
		t.Fatalf("owner access: %+v %v", c, err)
	}
	// A stranger sees nothing and is not listed the project.
	stranger := domain.Actor{UserID: "it-stranger"}
	if c, _ := f.svc.Access(ctx, stranger, pid); c.BaseRole != "" || acl.Decide(c, acl.Overview, acl.Read).Allowed {
		t.Fatalf("stranger must have no access: %+v", c)
	}
	if list, _ := f.svc.ListVisibleProjects(ctx, stranger, nil); len(list) != 0 {
		t.Fatalf("stranger sees %d projects", len(list))
	}

	// Invite a viewer: overview yes, budget no; a grant opens the budget; a deny masks documents.
	if _, err := f.svc.AddMember(ctx, f.owner, pid, domain.MemberInput{UserID: "it-viewer", BaseRole: "viewer"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddMember(ctx, f.owner, pid, domain.MemberInput{UserID: "x", BaseRole: "god"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("bad role: %v", err)
	}
	viewer := domain.Actor{UserID: "it-viewer"}
	c, _ = f.svc.Access(ctx, viewer, pid)
	if !acl.Decide(c, acl.Overview, acl.Read).Allowed || acl.Decide(c, acl.Budget, acl.Read).Allowed || acl.Decide(c, acl.Audit, acl.ReadFull).Allowed {
		t.Fatalf("viewer defaults: %+v", acl.Effective(c))
	}
	if _, err := f.svc.AddGrant(ctx, f.owner, pid, domain.GrantInput{UserID: "it-viewer", ScopeTarget: "section:budget", Permission: "read", Reason: "reviews the CAR"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddGrant(ctx, f.owner, pid, domain.GrantInput{UserID: "it-viewer", ScopeTarget: "section:documents", Permission: "deny"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddGrant(ctx, f.owner, pid, domain.GrantInput{UserID: "it-viewer", ScopeTarget: "section:payroll", Permission: "read"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("unknown section: %v", err)
	}
	if _, err := f.svc.AddGrant(ctx, f.owner, pid, domain.GrantInput{UserID: "it-viewer", ScopeTarget: "section:budget", Permission: "read_full"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("read_full outside audit: %v", err)
	}
	c, _ = f.svc.Access(ctx, viewer, pid)
	eff := acl.Effective(c)
	if eff[acl.Budget] != acl.Read || eff[acl.Documents] != "none" || eff[acl.Overview] != acl.Read || acl.Decide(c, acl.Budget, acl.Write).Allowed {
		t.Fatalf("viewer with grants: %+v", eff)
	}
	// On a long-lived dev database earlier runs also invited it-viewer, so check membership, not the count.
	if list, _ := f.svc.ListVisibleProjects(ctx, viewer, nil); !hasProject(list, pid) {
		t.Fatalf("viewer visibility: %d projects, none is %s", len(list), pid)
	}
	grants, _ := f.svc.ListGrants(ctx, pid)
	if len(grants) != 2 {
		t.Fatalf("grants: %d", len(grants))
	}
	if err := f.svc.RemoveGrant(ctx, f.owner, pid, grants[0].GrantID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.RemoveGrant(ctx, f.owner, pid, grants[0].GrantID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("second removal: %v", err)
	}

	// The last owner cannot be demoted or removed; a second owner can be.
	if _, err := f.svc.AddMember(ctx, f.owner, pid, domain.MemberInput{UserID: f.owner.UserID, BaseRole: "editor"}); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("demote last owner: %v", err)
	}
	if err := f.svc.RemoveMember(ctx, f.owner, pid, f.owner.UserID); !errors.Is(err, domain.ErrPrecondition) {
		t.Fatalf("remove last owner: %v", err)
	}
	if _, err := f.svc.AddMember(ctx, f.owner, pid, domain.MemberInput{UserID: "it-owner-2", BaseRole: "owner"}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.RemoveMember(ctx, f.owner, pid, "it-owner-2"); err != nil {
		t.Fatal(err)
	}
	members, _ := f.svc.ListMembers(ctx, pid)
	if len(members) != 2 {
		t.Fatalf("members: %d", len(members))
	}

	// A stage assignee reads the budget and approvals of a project they are not a member of.
	f.commitPromise(t)
	car := f.fullCAR(t, "640000.00", "Access test CAR.")
	if _, err := f.svc.SubmitCAR(ctx, f.owner, car.CAR.CarID, domain.SubmitCARInput{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.DecideCAR(ctx, f.owner, car.CAR.CarID, domain.DecideInput{Decision: "approve"}); err != nil {
		t.Fatal(err)
	}
	finance := domain.Actor{UserID: "it-finance"} // stage 2 assignee, not a member
	c, _ = f.svc.Access(ctx, finance, pid)
	if !c.StageAssignee || !acl.Decide(c, acl.Budget, acl.Read).Allowed || acl.Decide(c, acl.Budget, acl.Write).Allowed || acl.Decide(c, acl.Documents, acl.Read).Allowed {
		t.Fatalf("stage assignee: %+v", acl.Effective(c))
	}
	if list, _ := f.svc.ListVisibleProjects(ctx, finance, nil); len(list) == 0 {
		t.Fatal("stage assignee should see the project")
	}
	// Platform roles: a portfolio executive reads across projects; platform_admin has no blanket read.
	if list, _ := f.svc.ListVisibleProjects(ctx, domain.Actor{UserID: "it-exec", PlatformRoles: []string{acl.PortfolioExecutive}}, nil); len(list) == 0 {
		t.Fatal("portfolio executive should see projects")
	}
	if list, _ := f.svc.ListVisibleProjects(ctx, domain.Actor{UserID: "it-admin", PlatformRoles: []string{acl.PlatformAdmin}}, nil); len(list) != 0 {
		t.Fatal("platform admin must not see project content by role")
	}
	// Everything above left audit rows and the chain still verifies.
	rows, _ := f.svc.ProjectAudit(ctx, pid, 100)
	var kinds []string
	for _, r := range rows {
		kinds = append(kinds, r.ActionType)
	}
	joined := strings.Join(kinds, ",")
	for _, want := range []string{"member.upsert", "member.remove", "grant.add", "grant.remove"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("audit lacks %s: %s", want, joined)
		}
	}
	if r, err := f.svc.Chain.Verify(ctx, f.svc.Store.Queries(), tenant); err != nil || !r.Valid {
		t.Fatalf("chain: %+v %v", r, err)
	}
}

func hasProject(list []db.Project, id uuid.UUID) bool {
	for _, p := range list {
		if p.ProjectID == id {
			return true
		}
	}
	return false
}
