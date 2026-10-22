// Package mdemgadapter is the only place pm-core talks to MDEMG (FSD §06).
//
// It pins against MDEMG v0.11.0-beta.3's JSON field shapes (the same shapes
// the stub mirrors and the UATS specs in docs/tests/uats/specs/{stub-mdemg,
// mdemg} assert), never against constraint codes. Field renames in a future
// MDEMG land here and nowhere else.
package mdemgadapter

import "encoding/json"

// GuideRequest is POST /v1/jiminy/guide.
type GuideRequest struct {
	SpaceID     string `json:"space_id"`
	Context     string `json:"context"`
	FilePath    string `json:"file_path,omitempty"`
	AgentOutput string `json:"agent_output,omitempty"`
	Query       string `json:"query,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	MaxItems    int    `json:"max_items,omitempty"`
}

// GuidanceItem mirrors jiminy.GuidanceItem.
type GuidanceItem struct {
	Type            string            `json:"type"`
	Priority        string            `json:"priority"`
	Content         string            `json:"content"`
	Confidence      float64           `json:"confidence"`
	SourceNodes     []string          `json:"source_nodes,omitempty"`
	EscalationLevel string            `json:"escalation_level,omitempty"`
	ConstraintCode  string            `json:"constraint_code,omitempty"`
	Tier            int               `json:"tier,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
}

// SourceCounts mirrors jiminy.SourceCounts.
type SourceCounts struct {
	Constraints int `json:"constraints"`
	Corrections int `json:"corrections"`
	Patterns    int `json:"patterns"`
	Conflicts   int `json:"conflicts"`
	Frontiers   int `json:"frontiers"`
	Retrievals  int `json:"retrievals,omitempty"`
}

// GuideResponse mirrors jiminy.GuideResponse (the payload under {"data": ...}).
type GuideResponse struct {
	GuidanceID           string          `json:"guidance_id"`
	Guidance             []GuidanceItem  `json:"guidance"`
	PromptAugmentation   string          `json:"prompt_augmentation"`
	SynthesizedNarrative string          `json:"synthesized_narrative,omitempty"`
	Confidence           float64         `json:"confidence"`
	Rationale            string          `json:"rationale"`
	Warnings             []string        `json:"warnings,omitempty"`
	SourceCounts         SourceCounts    `json:"source_counts"`
	SessionEscalation    json.RawMessage `json:"session_escalation,omitempty"`
	GuidanceSeq          int64           `json:"guidance_seq,omitempty"`
}

// ClassifyRequest is POST /v1/jiminy/classify.
type ClassifyRequest struct {
	SpaceID     string `json:"space_id"`
	SessionID   string `json:"session_id"`
	AgentOutput string `json:"agent_output"`
	ToolName    string `json:"tool_name"`
	FilePath    string `json:"file_path,omitempty"`
}

// Verdict values MDEMG returns.
const (
	VerdictPass = "pass"
	VerdictDeny = "deny"
)

// ClassifyResponse mirrors jiminy.ClassifyResponse (under {"data": ...}).
type ClassifyResponse struct {
	Verdict         string   `json:"verdict"`
	DenialReason    string   `json:"denial_reason,omitempty"`
	ViolatedCodes   []string `json:"violated_codes,omitempty"`
	Confidence      float64  `json:"confidence"`
	EscalationLevel string   `json:"escalation_level,omitempty"`
}

// Outcome values for feedback (FSD §06 FEEDBACK).
const (
	OutcomeFollowed             = "followed"
	OutcomeIgnoredWithReason    = "ignored_with_reason"
	OutcomeBlockedTruePositive  = "blocked_true_positive"
	OutcomeBlockedFalsePositive = "blocked_false_positive"
	OutcomeOverriddenByAdmin    = "overridden_by_admin"
)

// FeedbackRequest is POST /v1/jiminy/feedback.
type FeedbackRequest struct {
	GuidanceID    string `json:"guidance_id"`
	ActionSummary string `json:"action_summary"`
	SpaceID       string `json:"space_id"`
	SessionID     string `json:"session_id,omitempty"`
	Outcome       string `json:"outcome,omitempty"`
}

// FeedbackResponse mirrors jiminy.FeedbackResponse.
type FeedbackResponse struct {
	GuidanceID string          `json:"guidance_id"`
	Results    json.RawMessage `json:"results"`
	Applied    bool            `json:"applied"`
}

// ObserveRequest is POST /v1/conversation/observe.
type ObserveRequest struct {
	SpaceID        string         `json:"space_id"`
	SessionID      string         `json:"session_id,omitempty"`
	Content        string         `json:"content"`
	ObsType        string         `json:"obs_type"` // decision | blocker | progress | insight | error | learning
	Tags           []string       `json:"tags,omitempty"`
	StructuredData map[string]any `json:"structured_data,omitempty"`
	SourceRef      string         `json:"source_ref,omitempty"` // audit_id, for idempotent delivery (§06 OBSERVE)
}

// ObserveResponse mirrors the observe handler's reply.
type ObserveResponse struct {
	ObsID           string         `json:"obs_id"`
	NodeID          string         `json:"node_id"`
	SurpriseScore   float64        `json:"surprise_score"`
	SurpriseFactors map[string]any `json:"surprise_factors"`
	Summary         string         `json:"summary"`
}

// WarmRequest is POST /v1/jiminy/warm.
type WarmRequest struct {
	SpaceID     string `json:"space_id"`
	ContextHint string `json:"context_hint,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
}

// WarmResponse mirrors the warm handler's reply (202).
type WarmResponse struct {
	Status     string  `json:"status"` // warming | debounced
	AgeMs      float64 `json:"age_ms"`
	DebounceMs float64 `json:"debounce_ms"`
}

// LatestResponse mirrors GET /v1/jiminy/latest.
type LatestResponse struct {
	Warm      bool           `json:"warm"`
	Status    string         `json:"status"`
	Data      *GuideResponse `json:"data,omitempty"`
	AgeMs     float64        `json:"age_ms"`
	ComputeMs float64        `json:"compute_ms"`
	Stale     bool           `json:"stale"`
}

// Health is what pm-core reports about its MDEMG dependency.
type Health struct {
	Reachable     bool   `json:"reachable"`
	JiminyEnabled bool   `json:"jiminy_enabled"`
	Status        string `json:"status,omitempty"`
	Error         string `json:"error,omitempty"`
	LatencyMs     int64  `json:"latency_ms"`
}
