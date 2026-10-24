package acl

import "testing"

func TestDecideTable(t *testing.T) {
	cases := []struct {
		name    string
		c       Caller
		section string
		perm    string
		allowed bool
		reason  string
	}{
		{"owner everything", Caller{BaseRole: Owner}, Budget, Write, true, "owner"},
		{"owner audit full", Caller{BaseRole: Owner}, Audit, ReadFull, true, "owner"},
		{"editor reads overview", Caller{BaseRole: Editor}, Overview, Read, true, "role:editor"},
		{"editor cannot write without grant", Caller{BaseRole: Editor}, Scope, Write, false, "default_deny"},
		{"editor denied budget by default", Caller{BaseRole: Editor}, Budget, Read, false, "default_deny"},
		{"viewer denied settings write", Caller{BaseRole: Viewer}, Settings, Write, false, "default_deny"},
		{"viewer reads audit summary only", Caller{BaseRole: Viewer}, Audit, ReadFull, false, "default_deny"},
		{"grant read budget", Caller{BaseRole: Viewer, Grants: []Grant{{"section", "section:budget", Read}}}, Budget, Read, true, "grant:section:budget:read"},
		{"grant write implies read", Caller{BaseRole: Viewer, Grants: []Grant{{"section", "section:schedule", Write}}}, Schedule, Read, true, "grant:section:schedule:write"},
		{"explicit deny masks the role default", Caller{BaseRole: Editor, Grants: []Grant{{"section", "section:documents", Deny}}}, Documents, Read, false, "grant:section:documents:deny"},
		{"deny wins over allow at the same level", Caller{BaseRole: Editor, Grants: []Grant{{"section", "section:budget", Read}, {"section", "section:budget", Deny}}}, Budget, Read, false, "grant:section:budget:deny"},
		{"grant on another section does not leak", Caller{BaseRole: Viewer, Grants: []Grant{{"section", "section:budget", Write}}}, Settings, Write, false, "default_deny"},
		{"field grant does not answer a section question", Caller{BaseRole: Viewer, Grants: []Grant{{"field", "field:budget.salary", Read}}}, Budget, Read, false, "default_deny"},
		{"audit read_full grant", Caller{BaseRole: Editor, Grants: []Grant{{"section", "section:audit", ReadFull}}}, Audit, ReadFull, true, "grant:section:audit:read_full"},
		{"non-member", Caller{}, Overview, Read, false, "not_a_member"},
		{"platform admin has no blanket read", Caller{PlatformRoles: []string{PlatformAdmin}}, Overview, Read, false, "not_a_member"},
		{"portfolio executive reads", Caller{PlatformRoles: []string{PortfolioExecutive}}, Budget, Read, true, "platform:portfolio_executive"},
		{"portfolio executive does not write", Caller{PlatformRoles: []string{PortfolioExecutive}}, Budget, Write, false, "not_a_member"},
		{"compliance officer reads risk and full audit", Caller{PlatformRoles: []string{ComplianceOfficer}}, Audit, ReadFull, true, "platform:compliance_officer"},
		{"compliance officer no budget", Caller{PlatformRoles: []string{ComplianceOfficer}}, Budget, Read, false, "not_a_member"},
		{"stage assignee reads budget", Caller{StageAssignee: true}, Budget, Read, true, "stage_assignee"},
		{"stage assignee cannot write budget", Caller{StageAssignee: true}, Budget, Write, false, "default_deny"},
		{"stage assignee no documents", Caller{StageAssignee: true}, Documents, Read, false, "default_deny"},
		{"contributor reads schedule only", Caller{BaseRole: Contributor}, Schedule, Read, true, "role:contributor"},
		{"contributor no budget", Caller{BaseRole: Contributor}, Budget, Read, false, "default_deny"},
		{"contributor no audit", Caller{BaseRole: Contributor}, Audit, Read, false, "default_deny"},
	}
	for _, c := range cases {
		d := Decide(c.c, c.section, c.perm)
		if d.Allowed != c.allowed || d.Reason != c.reason {
			t.Errorf("%s: got %+v want allowed=%v reason=%s", c.name, d, c.allowed, c.reason)
		}
	}
}

func TestEffective(t *testing.T) {
	e := Effective(Caller{BaseRole: Editor, Grants: []Grant{{"section", "section:budget", Read}, {"section", "section:schedule", Write}, {"section", "section:audit", ReadFull}}})
	if e[Budget] != Read || e[Schedule] != Write || e[Audit] != ReadFull || e[Settings] != Read || e[Overview] != Read {
		t.Fatalf("%+v", e)
	}
	if e := Effective(Caller{}); e[Overview] != "none" || len(e) != len(Sections) {
		t.Fatalf("non-member: %+v", e)
	}
	if !HasPlatformRole([]string{"Platform_Admin"}, PlatformAdmin) || HasPlatformRole(nil, PlatformAdmin) {
		t.Fatal("HasPlatformRole")
	}
}
