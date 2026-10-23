// Package mapping resolves IdP claims into pro_manage platform roles, workflow
// pools, and attributes from a declarative rule file (FSD §04 "Claims-to-role
// mapping"). It is pure: no network, no state, so it is unit-tested exhaustively
// and will be lifted into the Connector SDK unchanged.
package mapping

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Rule is one mapping entry.
type Rule struct {
	Claim      string            `yaml:"claim"`
	Contains   string            `yaml:"contains,omitempty"`
	Equals     string            `yaml:"equals,omitempty"`
	Matches    string            `yaml:"matches,omitempty"`
	Assigns    []string          `yaml:"assigns,omitempty"`
	Pools      []string          `yaml:"pools,omitempty"`
	Attributes map[string]string `yaml:"attributes,omitempty"`

	re *regexp.Regexp
}

// File is the YAML document.
type File struct {
	Mappings []Rule `yaml:"mappings"`
}

// Mapping is a compiled rule set.
type Mapping struct {
	rules []Rule
}

// Claims is the input (mirrors pm.connector.v1.Claims without the proto dependency).
type Claims struct {
	Subject           string
	Email             string
	DisplayName       string
	PreferredUsername string
	Groups            []string
	Roles             []string
	Attributes        map[string]string
}

// Result is the output.
type Result struct {
	PlatformRoles []string
	Pools         []string
	Attributes    map[string]string
}

// Load reads and compiles a mapping file.
func Load(path string) (*Mapping, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("mapping: read %s: %w", path, err)
	}
	return Parse(b)
}

// Parse compiles a YAML document.
func Parse(b []byte) (*Mapping, error) {
	var f File
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("mapping: parse: %w", err)
	}
	m := &Mapping{}
	for i := range f.Mappings {
		r := f.Mappings[i]
		if r.Claim == "" {
			return nil, fmt.Errorf("mapping: rule %d: claim is required", i)
		}
		n := 0
		for _, op := range []string{r.Contains, r.Equals, r.Matches} {
			if op != "" {
				n++
			}
		}
		if n != 1 {
			return nil, fmt.Errorf("mapping: rule %d (%s): exactly one of contains, equals, matches", i, r.Claim)
		}
		if r.Matches != "" {
			re, err := regexp.Compile(r.Matches)
			if err != nil {
				return nil, fmt.Errorf("mapping: rule %d: matches: %w", i, err)
			}
			r.re = re
		}
		if len(r.Assigns) == 0 && len(r.Pools) == 0 && len(r.Attributes) == 0 {
			return nil, fmt.Errorf("mapping: rule %d (%s): assigns nothing", i, r.Claim)
		}
		m.rules = append(m.rules, r)
	}
	if len(m.rules) == 0 {
		return nil, errors.New("mapping: no rules")
	}
	return m, nil
}

// Len is the number of rules.
func (m *Mapping) Len() int { return len(m.rules) }

// Resolve applies every rule; outputs are deduplicated and sorted so the result
// is stable for audit and for the session cache.
func (m *Mapping) Resolve(c Claims) Result {
	roles, pools := map[string]bool{}, map[string]bool{}
	attrs := map[string]string{}
	for _, r := range m.rules {
		for _, groups := range r.hits(c) {
			for _, a := range r.Assigns {
				roles[a] = true
			}
			for _, p := range r.Pools {
				pools[p] = true
			}
			for k, v := range r.Attributes {
				attrs[k] = expand(v, groups)
			}
		}
	}
	return Result{PlatformRoles: sorted(roles), Pools: sorted(pools), Attributes: attrs}
}

// hits returns one capture-group slice per matching claim value (a list claim
// may match several times, e.g. two site groups).
func (r Rule) hits(c Claims) [][]string {
	var out [][]string
	for _, v := range values(r.Claim, c) {
		switch {
		case r.Contains != "":
			if trimSlash(v) == trimSlash(r.Contains) || (!isList(r.Claim) && strings.Contains(v, r.Contains)) {
				out = append(out, []string{v})
			}
		case r.Equals != "":
			if v == r.Equals || trimSlash(v) == trimSlash(r.Equals) {
				out = append(out, []string{v})
			}
		case r.re != nil:
			if sm := r.re.FindStringSubmatch(v); sm != nil {
				out = append(out, sm)
			}
		}
	}
	return out
}

func isList(claim string) bool { return claim == "groups" || claim == "roles" }

func values(claim string, c Claims) []string {
	switch claim {
	case "groups":
		return c.Groups
	case "roles":
		return c.Roles
	case "email":
		return one(c.Email)
	case "preferred_username":
		return one(c.PreferredUsername)
	case "display_name":
		return one(c.DisplayName)
	case "subject":
		return one(c.Subject)
	}
	if name, ok := strings.CutPrefix(claim, "attributes."); ok {
		return one(c.Attributes[name])
	}
	return nil
}

func one(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func trimSlash(s string) string { return strings.TrimPrefix(s, "/") }

// expand substitutes $1..$9 with regex capture groups.
func expand(tpl string, groups []string) string {
	if !strings.Contains(tpl, "$") {
		return tpl
	}
	out := tpl
	for i := len(groups) - 1; i >= 1; i-- {
		out = strings.ReplaceAll(out, fmt.Sprintf("$%d", i), groups[i])
	}
	return out
}

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
