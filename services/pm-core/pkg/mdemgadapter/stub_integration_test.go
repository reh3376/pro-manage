//go:build integration

// Runs the adapter against a real MDEMG-shaped endpoint: the stub in the
// compose stack (CI), or the dedicated MDEMG stack on the workstation.
//
//	MDEMG_TEST_URL=http://127.0.0.1:9997 MDEMG_TEST_KEY=ci-stub-key go test -tags integration ./pkg/mdemgadapter/
//	MDEMG_TEST_URL=http://127.0.0.1:9998 MDEMG_TEST_KEY=<from .env> MDEMG_TEST_LIVE=1 go test -tags integration ./pkg/mdemgadapter/
package mdemgadapter

import (
	"context"
	"os"
	"testing"
	"time"
)

func live(t *testing.T) *Client {
	t.Helper()
	u := os.Getenv("MDEMG_TEST_URL")
	if u == "" {
		t.Skip("MDEMG_TEST_URL not set")
	}
	c, err := New(Options{BaseURL: u, APIKey: os.Getenv("MDEMG_TEST_KEY"), ClassifyTimeout: 90 * time.Second, GuideTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAgainstMDEMGShapedEndpoint(t *testing.T) {
	c := live(t)
	ctx := context.Background()
	h := c.Health(ctx)
	if !h.Reachable {
		t.Fatalf("not reachable: %+v", h)
	}
	spaces := Spaces{Project: ProjectSpace("pm-adapter-it"), Governance: "pm-adapter-it.governance"}
	if os.Getenv("MDEMG_TEST_LIVE") == "" {
		spaces.Governance = "org.pm-adapter-it.governance" // the stub returns its canned card for *.governance
	}
	m, err := c.GuideBoth(ctx, spaces, GuideRequest{Context: "adapter integration probe: user reviewing a CAR", SessionID: "it:pm-adapter"})
	if err != nil {
		t.Fatalf("guide: %v", err)
	}
	if m.GovernanceGuidanceID == "" || m.ProjectGuidanceID == "" {
		t.Fatalf("guidance ids missing: %+v", m)
	}
	d, err := c.ClassifyBoth(ctx, spaces, ClassifyRequest{SessionID: "it:pm-adapter", ToolName: "approval.car", AgentOutput: "adapter integration probe: approve a small CAR"})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if d.Verdict != VerdictPass && d.Verdict != VerdictDeny {
		t.Fatalf("verdict shape: %+v", d)
	}
	if os.Getenv("MDEMG_TEST_LIVE") == "" {
		d, err = c.ClassifyBoth(ctx, spaces, ClassifyRequest{SessionID: "it:pm-adapter", ToolName: "approval.car", AgentOutput: "stub:deny"})
		if err != nil || d.Verdict != VerdictDeny || len(d.ViolatedCodes) == 0 {
			t.Fatalf("stub canned deny: %v %+v", err, d)
		}
	}
	ob, err := c.Observe(ctx, ObserveRequest{SpaceID: spaces.Project, SessionID: "it:pm-adapter", Content: "adapter integration probe observation", ObsType: "decision", Tags: []string{"it"}})
	if err != nil || ob.ObsID == "" {
		t.Fatalf("observe: %v %+v", err, ob)
	}
	if _, err := c.Feedback(ctx, FeedbackRequest{GuidanceID: "", ActionSummary: "x", SpaceID: spaces.Project}); err == nil {
		t.Fatal("feedback without guidance_id must be rejected (400)")
	}
	if w, err := c.Warm(ctx, WarmRequest{SpaceID: spaces.Governance, ContextHint: "probe", SessionID: "it:pm-adapter"}); err != nil || w.Status == "" {
		t.Fatalf("warm: %v %+v", err, w)
	}
}
