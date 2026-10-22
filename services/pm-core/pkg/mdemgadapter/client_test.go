package mdemgadapter

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fake mirrors the stub's shapes closely enough for the adapter's contract.
func fake(t *testing.T, key string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if key != "" && r.Header.Get("X-API-Key") != key {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error":"invalid or missing api key"}`))
			return false
		}
		return true
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	mux.HandleFunc("GET /v1/jiminy/healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","enabled":true}`))
	})
	mux.HandleFunc("POST /v1/jiminy/guide", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !auth(w, r) {
			return
		}
		var req GuideRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.SpaceID == "" {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"space_id is required"}`))
			return
		}
		items := `[]`
		if strings.HasSuffix(req.SpaceID, ".governance") {
			items = `[{"type":"constraint","priority":"high","content":"gov rule","confidence":0.9,"constraint_code":"gov-1","tier":1},{"type":"note","priority":"low","content":"shared","confidence":0.5,"constraint_code":"shared-1"}]`
		} else if strings.HasPrefix(req.SpaceID, "proj_") {
			items = `[{"type":"note","priority":"medium","content":"proj note","confidence":0.6,"constraint_code":"proj-1"},{"type":"note","priority":"low","content":"shared dup","confidence":0.5,"constraint_code":"shared-1"}]`
		}
		w.Header().Set("X-Request-ID", r.Header.Get("X-Request-ID"))
		_, _ = w.Write([]byte(`{"data":{"guidance_id":"gd-` + req.SpaceID + `","guidance":` + items + `,"prompt_augmentation":"","confidence":0.7,"rationale":"fake","source_counts":{"constraints":1,"corrections":0,"patterns":0,"conflicts":0,"frontiers":0}}}`))
	})
	mux.HandleFunc("POST /v1/jiminy/classify", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		var req ClassifyRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if strings.HasSuffix(req.SpaceID, ".governance") && strings.Contains(req.AgentOutput, "deny-gov") {
			_, _ = w.Write([]byte(`{"data":{"verdict":"deny","denial_reason":"gov","violated_codes":["gov-deny"],"confidence":0.95}}`))
			return
		}
		if strings.HasPrefix(req.SpaceID, "proj_") && strings.Contains(req.AgentOutput, "deny-proj") {
			_, _ = w.Write([]byte(`{"data":{"verdict":"deny","denial_reason":"proj","violated_codes":["proj-deny"],"confidence":0.9}}`))
			return
		}
		if strings.Contains(req.AgentOutput, "slow") {
			time.Sleep(300 * time.Millisecond)
		}
		_, _ = w.Write([]byte(`{"data":{"verdict":"pass","confidence":0.93}}`))
	})
	mux.HandleFunc("POST /v1/jiminy/feedback", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"guidance_id":"gd-1","results":[],"applied":true}`))
	})
	mux.HandleFunc("POST /v1/conversation/observe", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"obs_id":"obs-1","node_id":"n-1","surprise_score":0.4,"surprise_factors":{},"summary":"s"}`))
	})
	mux.HandleFunc("POST /v1/jiminy/warm", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"status":"warming","age_ms":0,"debounce_ms":2000}`))
	})
	mux.HandleFunc("GET /v1/jiminy/latest", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"warm":true,"status":"ok","data":{"guidance_id":"gd-l","guidance":[],"prompt_augmentation":"","confidence":0,"rationale":"","source_counts":{}},"age_ms":1,"compute_ms":1,"stale":false}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestNewRejectsBadURL(t *testing.T) {
	if _, err := New(Options{BaseURL: "not a url"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestGuideBothMergesAndDedups(t *testing.T) {
	srv, calls := fake(t, "k")
	c, _ := New(Options{BaseURL: srv.URL, APIKey: "k"})
	ctx := WithRequestID(context.Background(), "req-42")
	m, err := c.GuideBoth(ctx, Spaces{Project: "proj_abc", Governance: "org.nf.governance"}, GuideRequest{Context: "test", SessionID: "u:p"})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected 2 guide calls, got %d", calls.Load())
	}
	if m.GovernanceGuidanceID != "gd-org.nf.governance" || m.ProjectGuidanceID != "gd-proj_abc" {
		t.Fatalf("guidance ids: %+v", m)
	}
	codes := []string{}
	for _, it := range m.Items {
		codes = append(codes, it.ConstraintCode)
	}
	if strings.Join(codes, ",") != "gov-1,proj-1,shared-1" {
		t.Fatalf("merged order/dedup wrong: %v", codes)
	}
}

func TestGuideBothSurvivesOneFailure(t *testing.T) {
	srv, _ := fake(t, "k")
	c, _ := New(Options{BaseURL: srv.URL, APIKey: "k"})
	m, err := c.GuideBoth(context.Background(), Spaces{Project: "", Governance: "org.nf.governance"}, GuideRequest{Context: "x"})
	if err == nil || !errors.Is(err, ErrBadRequest) {
		t.Fatalf("expected bad request from the empty project space, got %v", err)
	}
	if len(m.Items) != 2 || m.GovernanceGuidanceID == "" {
		t.Fatalf("governance items should still be returned: %+v", m)
	}
}

func TestClassifyBothAnyDenyWins(t *testing.T) {
	srv, _ := fake(t, "k")
	c, _ := New(Options{BaseURL: srv.URL, APIKey: "k"})
	sp := Spaces{Project: "proj_abc", Governance: "org.nf.governance"}
	d, err := c.ClassifyBoth(context.Background(), sp, ClassifyRequest{SessionID: "s", AgentOutput: "ok", ToolName: "approval.car"})
	if err != nil || d.Verdict != VerdictPass || d.DeniedBy != "" {
		t.Fatalf("pass expected: %+v %v", d, err)
	}
	d, _ = c.ClassifyBoth(context.Background(), sp, ClassifyRequest{SessionID: "s", AgentOutput: "deny-gov", ToolName: "approval.car"})
	if d.Verdict != VerdictDeny || d.DeniedBy != "governance" || d.ViolatedCodes[0] != "gov-deny" {
		t.Fatalf("governance deny expected: %+v", d)
	}
	d, _ = c.ClassifyBoth(context.Background(), sp, ClassifyRequest{SessionID: "s", AgentOutput: "deny-proj", ToolName: "approval.car"})
	if d.Verdict != VerdictDeny || d.DeniedBy != "project" {
		t.Fatalf("project deny expected: %+v", d)
	}
}

func TestClassifyBothRunsInParallelAndTimesOut(t *testing.T) {
	srv, _ := fake(t, "k")
	c, _ := New(Options{BaseURL: srv.URL, APIKey: "k", ClassifyTimeout: 2 * time.Second})
	sp := Spaces{Project: "proj_abc", Governance: "org.nf.governance"}
	start := time.Now()
	if _, err := c.ClassifyBoth(context.Background(), sp, ClassifyRequest{AgentOutput: "slow"}); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 550*time.Millisecond {
		t.Fatalf("calls should overlap (took %v)", el)
	}
	c2, _ := New(Options{BaseURL: srv.URL, APIKey: "k", ClassifyTimeout: 100 * time.Millisecond})
	_, err := c2.ClassifyBoth(context.Background(), sp, ClassifyRequest{AgentOutput: "slow"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("timeout must surface as unavailable, got %v", err)
	}
}

func TestUnauthorizedAndUnreachable(t *testing.T) {
	srv, _ := fake(t, "k")
	c, _ := New(Options{BaseURL: srv.URL, APIKey: "wrong"})
	_, err := c.Guide(context.Background(), GuideRequest{SpaceID: "proj_x", Context: "c"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	var e *Error
	if !errors.As(err, &e) || e.Status != 401 {
		t.Fatalf("expected typed error with 401, got %v", err)
	}
	down, _ := New(Options{BaseURL: "http://127.0.0.1:1", HealthTimeout: 200 * time.Millisecond})
	h := down.Health(context.Background())
	if h.Reachable || h.Error == "" {
		t.Fatalf("expected unreachable health, got %+v", h)
	}
	up := c.Health(context.Background()) // health endpoints are unauthenticated on the stub and MDEMG
	if !up.Reachable || !up.JiminyEnabled {
		t.Fatalf("expected reachable health, got %+v", up)
	}
}

func TestWritesAndWarmLatest(t *testing.T) {
	srv, _ := fake(t, "")
	c, _ := New(Options{BaseURL: srv.URL})
	ctx := context.Background()
	if fb, err := c.Feedback(ctx, FeedbackRequest{GuidanceID: "gd-1", ActionSummary: "x", SpaceID: "proj_a", Outcome: OutcomeFollowed}); err != nil || !fb.Applied {
		t.Fatalf("feedback: %v %+v", err, fb)
	}
	if ob, err := c.Observe(ctx, ObserveRequest{SpaceID: "proj_a", Content: "c", ObsType: "decision"}); err != nil || ob.ObsID == "" {
		t.Fatalf("observe: %v %+v", err, ob)
	}
	if w, err := c.Warm(ctx, WarmRequest{SpaceID: "proj_a"}); err != nil || w.Status != "warming" {
		t.Fatalf("warm: %v %+v", err, w)
	}
	if l, err := c.Latest(ctx, "proj_a", ""); err != nil || !l.Warm || l.Data == nil {
		t.Fatalf("latest: %v %+v", err, l)
	}
}

func TestSpaceNaming(t *testing.T) {
	if GovernanceSpace("northfield") != "org.northfield.governance" {
		t.Fatal(GovernanceSpace("northfield"))
	}
	if ProjectSpace("01900000-0000-7000-8000-00000000b001") != "proj_0190000000007000800000000000b001" {
		t.Fatal(ProjectSpace("01900000-0000-7000-8000-00000000b001"))
	}
}
