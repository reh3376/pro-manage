// Package governance is the in-process side of FSD §06: the guide-on-open read,
// the classify-on-submit gate with its fail-closed tiers, and the
// pre-classification cache. It owns no business rules; the domain decides what
// is governed and with which tier, and this package turns MDEMG's answer (or
// silence) into a Resolution the domain can act on.
package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

// Tier is an action kind's fail-closed tier (§06 CLASSIFY).
type Tier string

// Tiers.
const (
	TierA Tier = "A" // fail-closed on error
	TierB Tier = "B" // fail-open with a surfaced warning
	TierC Tier = "C" // bypass: never classified
)

// ParseTier accepts A, B, or C (any case); anything else is an error.
func ParseTier(s string) (Tier, error) {
	switch s {
	case "A", "a":
		return TierA, nil
	case "B", "b":
		return TierB, nil
	case "C", "c":
		return TierC, nil
	}
	return "", errors.New("governance: tier must be A, B, or C")
}

// User responses recorded in GuidanceOutcome (§06 audit coupling).
const (
	ResponseFollowed          = "followed"
	ResponseIgnoredWithReason = "ignored_with_reason"
	ResponseDismissed         = "dismissed"
	ResponseBlocked           = "blocked"
	ResponseOverriddenByAdmin = "overridden_by_admin"
	ResponseMDEMGUnavailable  = "mdemg_unavailable"
)

// Gate talks to MDEMG for governed actions. A nil Client means MDEMG is not
// configured, which every tier treats as unreachable.
type Gate struct {
	Client *mdemgadapter.Client
	OrgID  string // governance space is org.<OrgID>.governance
	Log    *slog.Logger
	Cache  *PreclassCache
}

// Spaces for a project.
func (g *Gate) Spaces(projectID string) mdemgadapter.Spaces {
	return mdemgadapter.Spaces{Project: mdemgadapter.ProjectSpace(projectID), Governance: mdemgadapter.GovernanceSpace(g.OrgID)}
}

// Guidance is what a page renders on open.
type Guidance struct {
	Available            bool                        `json:"available"`    // MDEMG answered
	Warm                 bool                        `json:"warm"`         // both spaces had a warmed result
	Items                []mdemgadapter.GuidanceItem `json:"items"`        // merged, governance first, high priority first
	GuidanceIDs          []string                    `json:"guidance_ids"` // ids to carry into submit and feedback
	GovernanceGuidanceID string                      `json:"governance_guidance_id,omitempty"`
	ProjectGuidanceID    string                      `json:"project_guidance_id,omitempty"`
	Error                string                      `json:"error,omitempty"`
}

// GuideOnOpen implements §06 GUIDE: warm both spaces (fire-and-forget), then read
// /latest for both and merge. It never blocks the page: an unreachable MDEMG
// returns Available=false and no items.
func (g *Gate) GuideOnOpen(ctx context.Context, spaces mdemgadapter.Spaces, sessionID, contextHint string) Guidance {
	out := Guidance{Items: []mdemgadapter.GuidanceItem{}, GuidanceIDs: []string{}}
	if g.Client == nil {
		out.Error = "mdemg not configured"
		return out
	}
	for _, sp := range []string{spaces.Governance, spaces.Project} {
		if _, err := g.Client.Warm(ctx, mdemgadapter.WarmRequest{SpaceID: sp, ContextHint: contextHint, SessionID: sessionID}); err != nil {
			g.Log.Warn("mdemg warm failed", "space", sp, "err", err)
		}
	}
	gov, govErr := g.Client.Latest(ctx, spaces.Governance, sessionID)
	proj, projErr := g.Client.Latest(ctx, spaces.Project, sessionID)
	if govErr != nil && projErr != nil {
		out.Error = errors.Join(govErr, projErr).Error()
		return out
	}
	out.Available = true
	out.Warm = gov.Warm && proj.Warm
	seen := map[string]bool{}
	for _, l := range []mdemgadapter.LatestResponse{gov, proj} {
		if l.Data == nil {
			continue
		}
		if l.Data.GuidanceID != "" {
			out.GuidanceIDs = append(out.GuidanceIDs, l.Data.GuidanceID)
		}
		for _, it := range l.Data.Guidance {
			key := it.ConstraintCode
			if key == "" {
				key = "content:" + it.Content
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out.Items = append(out.Items, it)
		}
	}
	if gov.Data != nil {
		out.GovernanceGuidanceID = gov.Data.GuidanceID
	}
	if proj.Data != nil {
		out.ProjectGuidanceID = proj.Data.GuidanceID
	}
	if err := errors.Join(govErr, projErr); err != nil {
		out.Error = err.Error()
	}
	return out
}

// ClassifyInput is one governed submission.
type ClassifyInput struct {
	ActionKind     string        // e.g. approval.car (MDEMG tool_name)
	UserID         string        // cache scope
	ProjectID      string        // cache scope
	SessionID      string        // user:project
	Payload        string        // the bounded summary MDEMG classifies (never a full form body)
	PolicySnapshot []byte        // effective policy values; part of the cache key (§06 pre-classification)
	Timeout        time.Duration // mdemg.classify.timeout_ms; zero keeps the adapter default
}

// Verdict is the gate's answer before the tier is applied.
type Verdict struct {
	Decision    mdemgadapter.Decision
	Cached      bool
	Unavailable bool   // error or timeout on either space
	Error       string // the adapter error, for the audit record
	CacheKey    string
}

// CacheKey is sha256(action_kind || payload || policy_snapshot), the §06 key.
func CacheKey(actionKind, payload string, snapshot []byte) string {
	h := sha256.New()
	h.Write([]byte(actionKind))
	h.Write([]byte{0})
	h.Write([]byte(payload))
	h.Write([]byte{0})
	h.Write(snapshot)
	return hex.EncodeToString(h.Sum(nil))
}

// Classify returns the cached verdict for an identical payload and snapshot or
// runs ClassifyBoth inline. Tier C callers must not call this.
func (g *Gate) Classify(ctx context.Context, spaces mdemgadapter.Spaces, in ClassifyInput) Verdict {
	key := CacheKey(in.ActionKind, in.Payload, in.PolicySnapshot)
	v := Verdict{CacheKey: key}
	if g.Cache != nil {
		if d, ok := g.Cache.Get(in.UserID, in.ProjectID, key); ok {
			v.Decision, v.Cached = d, true
			return v
		}
	}
	if g.Client == nil {
		v.Unavailable, v.Error = true, "mdemg not configured"
		return v
	}
	if in.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, in.Timeout)
		defer cancel()
	}
	d, err := g.Client.ClassifyBoth(ctx, spaces, mdemgadapter.ClassifyRequest{SessionID: in.SessionID, AgentOutput: in.Payload, ToolName: in.ActionKind})
	v.Decision = d
	if err != nil {
		v.Unavailable, v.Error = true, err.Error()
		return v
	}
	if g.Cache != nil {
		g.Cache.Put(in.UserID, in.ProjectID, key, d)
	}
	return v
}

// Resolution is what the domain does with a verdict under a tier.
type Resolution struct {
	Proceed      bool   // write the action
	UserResponse string // GuidanceOutcome.user_response
	Verdict      string // pass | deny | "" (unavailable)
	Refused      bool   // Tier A with MDEMG unavailable
	Denied       bool   // MDEMG said deny
}

// Resolve applies the tier rules of §06: deny always blocks; unavailable refuses
// under Tier A and proceeds with a recorded warning under Tier B; a pass
// proceeds with the user's response to the guidance they saw (followed,
// ignored_with_reason, dismissed).
func Resolve(tier Tier, v Verdict, userResponse string) Resolution {
	if tier == TierC {
		return Resolution{Proceed: true, UserResponse: userResponse}
	}
	if v.Unavailable {
		if tier == TierA {
			return Resolution{Refused: true, UserResponse: ResponseMDEMGUnavailable}
		}
		return Resolution{Proceed: true, UserResponse: ResponseMDEMGUnavailable}
	}
	if v.Decision.Verdict == mdemgadapter.VerdictDeny {
		return Resolution{Denied: true, UserResponse: ResponseBlocked, Verdict: mdemgadapter.VerdictDeny}
	}
	if userResponse == "" {
		userResponse = ResponseFollowed
	}
	return Resolution{Proceed: true, UserResponse: userResponse, Verdict: mdemgadapter.VerdictPass}
}

// ---------------------------------------------------------------- pre-classification cache

// PreclassCache holds verdicts for settled forms (§06: scoped to user and
// project, 10-minute TTL). In-process for the spine; Redis when pm-core runs
// more than one replica (backlog).
type PreclassCache struct {
	TTL time.Duration
	mu  sync.Mutex
	m   map[string]entry
}

type entry struct {
	d   mdemgadapter.Decision
	exp time.Time
}

// NewPreclassCache with the §06 default TTL when ttl is zero.
func NewPreclassCache(ttl time.Duration) *PreclassCache {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &PreclassCache{TTL: ttl, m: map[string]entry{}}
}

func scoped(user, project, key string) string { return user + "|" + project + "|" + key }

// Get returns a live verdict.
func (c *PreclassCache) Get(user, project, key string) (mdemgadapter.Decision, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[scoped(user, project, key)]
	if !ok || time.Now().After(e.exp) {
		delete(c.m, scoped(user, project, key))
		return mdemgadapter.Decision{}, false
	}
	return e.d, true
}

// Put stores a verdict and prunes expired entries opportunistically.
func (c *PreclassCache) Put(user, project, key string, d mdemgadapter.Decision) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, e := range c.m {
		if now.After(e.exp) {
			delete(c.m, k)
		}
	}
	c.m[scoped(user, project, key)] = entry{d: d, exp: now.Add(c.TTL)}
}

// Len is for tests and /stats.
func (c *PreclassCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}
