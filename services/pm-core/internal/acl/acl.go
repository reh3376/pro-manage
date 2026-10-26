// Package acl is the grant evaluation of FSD §04, pure and table-driven: a
// caller's project context (base role, explicit grants, platform roles, open
// stage assignment) plus a section and a permission in, a decision out. The
// API layer loads the context once per request and asks; nothing here touches
// the database, so the rules are unit-tested exhaustively.
package acl

import (
	"sort"
	"strings"
)

// Permissions.
const (
	Read     = "read"
	Write    = "write"
	ReadFull = "read_full" // audit: full before/after payloads instead of the summary
	Deny     = "deny"
)

// Sections (§04 "What is a section").
const (
	Overview       = "overview"
	Team           = "team"
	Budget         = "budget"
	Schedule       = "schedule"
	Scope          = "scope"
	Risk           = "risk"
	Issues         = "issues"
	ChangeRequests = "change_requests"
	Documents      = "documents"
	Metrics        = "metrics"
	Approvals      = "approvals"
	Audit          = "audit"
	Governance     = "governance"
	Settings       = "settings"
)

// Sections in display order.
var Sections = []string{Overview, Team, Budget, Schedule, Scope, Risk, Issues, ChangeRequests, Documents, Metrics, Approvals, Audit, Governance, Settings}

// Base roles.
const (
	Owner       = "owner"
	Editor      = "editor"
	Viewer      = "viewer"
	Contributor = "contributor"
)

// Platform roles this package knows (§04 "Platform roles").
const (
	PlatformAdmin      = "platform_admin"
	PolicyOwner        = "policy_owner"
	GovernanceAuthor   = "governance_author"
	ComplianceOfficer  = "compliance_officer"
	PortfolioExecutive = "portfolio_executive"
)

// Grant is one explicit ACLGrant row, already filtered to the caller and unexpired.
type Grant struct {
	ScopeKind   string // section | document | field
	ScopeTarget string // section:budget | document:<id> | field:<path>
	Permission  string // read | write | read_full | deny
}

// Caller is everything the evaluator needs about the request.
type Caller struct {
	UserID        string
	PlatformRoles []string
	BaseRole      string  // "" when not a member
	Grants        []Grant // the caller's grants on this project
	StageAssignee bool    // sits on an open stage of an in-flight workflow on this project
}

// Decision is the answer and why, for the denied-access log (§04 "Audit of access decisions").
type Decision struct {
	Allowed bool
	Reason  string // owner | grant:<target>:<permission> | role:<base_role> | platform:<role> | stage_assignee | default_deny | not_a_member
}

// baseDefaults is the section table for editor and viewer. Missing means deny.
// Values are the strongest default permission.
var baseDefaults = map[string]map[string]string{
	Editor: {Overview: Read, Team: Read, Schedule: Read, Scope: Read, Risk: Read, Issues: Read, ChangeRequests: Read, Documents: Read, Metrics: Read, Approvals: Read, Audit: Read, Governance: Read, Settings: Read},
	Viewer: {Overview: Read, Team: Read, Schedule: Read, Scope: Read, Risk: Read, Issues: Read, ChangeRequests: Read, Documents: Read, Metrics: Read, Approvals: Read, Audit: Read, Governance: Read, Settings: Read},
	// Contributors are scoped by contract (§04); until contracts exist they read the overview and schedule only.
	Contributor: {Overview: Read, Schedule: Read, Scope: Read, Documents: Read, Settings: Read},
}

// stageSections is what an open-stage assignee may read without membership: the
// project header, the money being approved, and the approvals themselves.
var stageSections = map[string]bool{Overview: true, Budget: true, Approvals: true, Settings: true}

// Decide applies §04 "Grant evaluation": owner override first, then explicit
// grants from most specific to least (field > document > section; deny wins
// within a level), then the base-role default, then platform-role and
// workflow-role reads. write implies read; read_full implies read.
func Decide(c Caller, section, perm string) Decision {
	if c.BaseRole == Owner {
		return Decision{true, "owner"}
	}
	target := "section:" + section
	// Explicit grants on this section, most specific first. Document and field
	// grants are not consulted for a section-level question (they refine it).
	var matching []Grant
	for _, g := range c.Grants {
		if g.ScopeKind == "section" && g.ScopeTarget == target {
			matching = append(matching, g)
		}
	}
	sort.SliceStable(matching, func(i, j int) bool { return rank(matching[i].Permission) < rank(matching[j].Permission) })
	for _, g := range matching {
		if g.Permission == Deny {
			return Decision{false, "grant:" + target + ":deny"}
		}
	}
	for _, g := range matching {
		if implies(g.Permission, perm) {
			return Decision{true, "grant:" + target + ":" + g.Permission}
		}
	}
	if c.BaseRole != "" {
		if def, ok := baseDefaults[c.BaseRole][section]; ok && implies(def, perm) {
			return Decision{true, "role:" + c.BaseRole}
		}
	}
	for _, r := range c.PlatformRoles {
		switch {
		case r == PortfolioExecutive && perm == Read && section != Audit && section != Settings && section != Team:
			return Decision{true, "platform:" + r} // read across projects (portfolio scoping arrives with portfolios)
		case r == ComplianceOfficer && perm == Read && (section == Risk || section == Issues || section == Governance || section == Audit):
			return Decision{true, "platform:" + r}
		case r == ComplianceOfficer && perm == ReadFull && section == Audit:
			return Decision{true, "platform:" + r}
		case r == GovernanceAuthor && section == Governance:
			return Decision{true, "platform:" + r}
		case r == PolicyOwner && perm == Read && section == Settings:
			return Decision{true, "platform:" + r}
		}
	}
	if c.StageAssignee && perm == Read && stageSections[section] {
		return Decision{true, "stage_assignee"}
	}
	if c.BaseRole == "" && !c.StageAssignee {
		return Decision{false, "not_a_member"}
	}
	return Decision{false, "default_deny"}
}

// Effective is the per-section view for the UI and /access: the strongest permission the caller holds.
func Effective(c Caller) map[string]string {
	out := map[string]string{}
	for _, s := range Sections {
		switch {
		case Decide(c, s, Write).Allowed:
			out[s] = Write
		case Decide(c, s, Read).Allowed:
			out[s] = Read
		default:
			out[s] = "none"
		}
		if s == Audit && Decide(c, s, ReadFull).Allowed {
			out[s] = ReadFull
		}
	}
	return out
}

// HasPlatformRole is a small helper for platform-level routes.
func HasPlatformRole(roles []string, want string) bool {
	for _, r := range roles {
		if strings.EqualFold(r, want) {
			return true
		}
	}
	return false
}

func implies(have, want string) bool {
	switch want {
	case Read:
		return have == Read || have == Write || have == ReadFull
	case Write:
		return have == Write
	case ReadFull:
		return have == ReadFull
	}
	return false
}

func rank(p string) int {
	switch p {
	case Deny:
		return 0
	case Write:
		return 1
	case ReadFull:
		return 2
	default:
		return 3
	}
}
