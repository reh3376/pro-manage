package mdemgadapter

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

// Spaces names the two spaces every governed call consults (FSD §06 "Call-time routing").
type Spaces struct {
	Project    string // "proj_<uuid>"
	Governance string // "org.<id>.governance"
}

// GovernanceSpace is the conventional governance space id for an organization.
func GovernanceSpace(orgID string) string { return "org." + orgID + ".governance" }

// ProjectSpace is the conventional project space id.
func ProjectSpace(projectID string) string { return "proj_" + strings.ReplaceAll(projectID, "-", "") }

// GuideBoth calls Guide against the project and governance spaces, merges the
// items (deduplicated by constraint_code, governance first), and returns the
// merged list with both guidance ids. A failure of either call is returned
// along with whatever the other produced; callers treat guide failures as
// non-blocking (§06 GUIDE).
func (c *Client) GuideBoth(ctx context.Context, spaces Spaces, req GuideRequest) (Merged, error) {
	type res struct {
		resp GuideResponse
		err  error
	}
	var wg sync.WaitGroup
	results := make([]res, 2)
	for i, space := range []string{spaces.Governance, spaces.Project} {
		wg.Add(1)
		go func(i int, space string) {
			defer wg.Done()
			r := req
			r.SpaceID = space
			resp, err := c.Guide(ctx, r)
			results[i] = res{resp, err}
		}(i, space)
	}
	wg.Wait()

	m := Merged{GovernanceGuidanceID: results[0].resp.GuidanceID, ProjectGuidanceID: results[1].resp.GuidanceID}
	seen := map[string]bool{}
	for _, r := range results {
		for _, it := range r.resp.Guidance {
			key := it.ConstraintCode
			if key == "" {
				key = "content:" + it.Content
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			m.Items = append(m.Items, it)
		}
	}
	if m.Items == nil {
		m.Items = []GuidanceItem{}
	}
	sort.SliceStable(m.Items, func(i, j int) bool { return priorityRank(m.Items[i].Priority) < priorityRank(m.Items[j].Priority) })
	return m, errors.Join(results[0].err, results[1].err)
}

// Merged is the result of GuideBoth.
type Merged struct {
	GovernanceGuidanceID string         `json:"governance_guidance_id"`
	ProjectGuidanceID    string         `json:"project_guidance_id"`
	Items                []GuidanceItem `json:"items"`
}

func priorityRank(p string) int {
	switch strings.ToLower(p) {
	case "high":
		return 0
	case "medium":
		return 1
	default:
		return 2
	}
}

// Decision is the outcome of ClassifyBoth.
type Decision struct {
	Verdict       string           `json:"verdict"`             // pass | deny
	DeniedBy      string           `json:"denied_by,omitempty"` // governance | project
	Governance    ClassifyResponse `json:"governance"`
	Project       ClassifyResponse `json:"project"`
	ViolatedCodes []string         `json:"violated_codes"`
}

// ClassifyBoth runs classify against both spaces in parallel (§06: each call
// is a local LLM call measured in seconds). Any deny wins. If either call
// fails, the error is returned and the caller applies the action kind's tier:
// A refuses, B proceeds with a warning, C never called this.
func (c *Client) ClassifyBoth(ctx context.Context, spaces Spaces, req ClassifyRequest) (Decision, error) {
	var wg sync.WaitGroup
	var gov, proj ClassifyResponse
	var govErr, projErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		r := req
		r.SpaceID = spaces.Governance
		gov, govErr = c.Classify(ctx, r)
	}()
	go func() {
		defer wg.Done()
		r := req
		r.SpaceID = spaces.Project
		proj, projErr = c.Classify(ctx, r)
	}()
	wg.Wait()
	d := Decision{Verdict: VerdictPass, Governance: gov, Project: proj, ViolatedCodes: []string{}}
	if err := errors.Join(govErr, projErr); err != nil {
		return d, err
	}
	if gov.Verdict == VerdictDeny {
		d.Verdict, d.DeniedBy = VerdictDeny, "governance"
		d.ViolatedCodes = append(d.ViolatedCodes, gov.ViolatedCodes...)
	}
	if proj.Verdict == VerdictDeny {
		if d.Verdict != VerdictDeny {
			d.DeniedBy = "project"
		}
		d.Verdict = VerdictDeny
		d.ViolatedCodes = append(d.ViolatedCodes, proj.ViolatedCodes...)
	}
	return d, nil
}
