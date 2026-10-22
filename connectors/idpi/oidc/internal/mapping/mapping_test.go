package mapping

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func load(t *testing.T) *Mapping {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "mapping.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNorthfieldMapping(t *testing.T) {
	m := load(t)
	cases := []struct {
		name  string
		in    Claims
		roles []string
		pools []string
		attrs map[string]string
	}{
		{"owner is an employee only", Claims{Groups: []string{"/pro_manage.employees"}}, []string{}, []string{}, map[string]string{"employee": "true"}},
		{"finance approver", Claims{Groups: []string{"/pro_manage.employees", "/pro_manage.finance_approvers"}}, []string{"finance_approver_eligible"}, []string{"finance_approver_eligible"}, map[string]string{"employee": "true"}},
		{"admin by group without slash", Claims{Groups: []string{"pro_manage.platform_admins"}}, []string{"platform_admin"}, []string{}, map[string]string{}},
		{"policy owner and governance author", Claims{Groups: []string{"/pro_manage.policy_owners", "/pro_manage.governance_authors"}}, []string{"governance_author", "policy_owner"}, []string{}, map[string]string{}},
		{"site ops lead by group with site capture", Claims{Groups: []string{"/pro_manage.employees", "/site.NF-KY.operations_leads"}, Roles: []string{"site_operations_lead"}}, []string{}, []string{"site_operations_lead"}, map[string]string{"employee": "true", "site": "NF-KY"}},
		{"realm role only", Claims{Roles: []string{"compliance_officer", "default-roles-northfield"}}, []string{"compliance_officer"}, []string{}, map[string]string{}},
		{"contributor is external", Claims{Groups: []string{"/pro_manage.contributors"}}, []string{}, []string{}, map[string]string{"external": "true"}},
		{"nothing", Claims{Email: "x@y"}, []string{}, []string{}, map[string]string{}},
	}
	for _, c := range cases {
		got := m.Resolve(c.in)
		if !reflect.DeepEqual(got.PlatformRoles, c.roles) || !reflect.DeepEqual(got.Pools, c.pools) || !reflect.DeepEqual(got.Attributes, c.attrs) {
			t.Errorf("%s: got %+v", c.name, got)
		}
	}
}

func TestParseErrors(t *testing.T) {
	bad := []string{
		"mappings: []",
		"mappings:\n  - contains: x\n    assigns: [a]",
		"mappings:\n  - claim: groups\n    contains: x\n    equals: y\n    assigns: [a]",
		"mappings:\n  - claim: groups\n    contains: x",
		"mappings:\n  - claim: groups\n    matches: '('\n    assigns: [a]",
	}
	for _, b := range bad {
		if _, err := Parse([]byte(b)); err == nil {
			t.Errorf("expected error for %q", b)
		}
	}
}

func TestScalarClaimsAndAttributes(t *testing.T) {
	m, err := Parse([]byte(`
mappings:
  - claim: email
    matches: '@finance\.example$'
    assigns: [finance_approver_eligible]
  - claim: attributes.department
    equals: Finance
    pools: [finance_approver_eligible]
  - claim: preferred_username
    contains: admin
    assigns: [platform_admin]
`))
	if err != nil {
		t.Fatal(err)
	}
	got := m.Resolve(Claims{Email: "bob@finance.example", PreferredUsername: "sysadmin", Attributes: map[string]string{"department": "Finance"}})
	if !reflect.DeepEqual(got.PlatformRoles, []string{"finance_approver_eligible", "platform_admin"}) || !reflect.DeepEqual(got.Pools, []string{"finance_approver_eligible"}) {
		t.Fatalf("got %+v", got)
	}
}
