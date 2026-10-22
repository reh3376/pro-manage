// stub-mdemg is a stand-in for MDEMG in PR CI (FSD §06, §12).
//
// CI cannot run MDEMG's local language model, so this binary answers the
// endpoints pm-core calls with the field shapes MDEMG v0.11.0-beta.3 returns
// (taken from internal/jiminy/types.go and MDEMG's own UATS specs) and canned
// verdicts:
//
//	GET  /healthz                  -> 200 {"status":"ok", ...}
//	GET  /v1/jiminy/healthz        -> 200 {"status":"ok","enabled":true}
//	POST /v1/jiminy/warm           -> 202 {"status":"warming","age_ms":0,"debounce_ms":...}
//	GET  /v1/jiminy/latest         -> 200 {"warm":true,"status":...,"data":{<guide payload>},"age_ms","compute_ms","stale"}
//	POST /v1/jiminy/guide          -> 200 {"data":{"guidance_id","guidance":[...],"prompt_augmentation","confidence","rationale","source_counts"}}
//	POST /v1/jiminy/classify       -> 200 {"data":{"verdict":"pass"|"deny","denial_reason","violated_codes","confidence","escalation_level"}}
//	POST /v1/jiminy/feedback       -> 200 {"guidance_id","results":[],"applied":true}; 400 {"error"} without guidance_id
//	POST /v1/conversation/observe  -> 200 {"obs_id","node_id","surprise_score","surprise_factors","summary"}
//
// Validation mirrors MDEMG: a missing space_id (or context on guide, content on
// observe) is 400 {"error": ...}; a wrong method is 405. A classify request
// whose body contains the token "stub:deny" is denied with constraint code
// "stub-deny"; everything else passes. A guide request against a space ending
// in ".governance" returns one high-priority card so the guidance rail has
// something to render; other spaces return an empty guidance array.
//
// When STUB_API_KEY is set, every request except /healthz must carry it as
// X-API-Key or a Bearer token (MDEMG's apikey auth), so pm-core's auth path is
// exercised in CI.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/config"
	"github.com/reh3376/pro-manage/services/pm-core/internal/httpx"
	"github.com/reh3376/pro-manage/services/pm-core/internal/secrets"
)

// guidanceItem mirrors MDEMG's jiminy.GuidanceItem JSON (tier is an int there).
type guidanceItem struct {
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

type sourceCounts struct {
	Constraints int `json:"constraints"`
	Corrections int `json:"corrections"`
	Patterns    int `json:"patterns"`
	Conflicts   int `json:"conflicts"`
	Frontiers   int `json:"frontiers"`
}

// guideResponse mirrors MDEMG's jiminy.GuideResponse (served under {"data": ...}).
type guideResponse struct {
	GuidanceID         string         `json:"guidance_id"`
	Guidance           []guidanceItem `json:"guidance"`
	PromptAugmentation string         `json:"prompt_augmentation"`
	Confidence         float64        `json:"confidence"`
	Rationale          string         `json:"rationale"`
	SourceCounts       sourceCounts   `json:"source_counts"`
}

// classifyResponse mirrors MDEMG's jiminy.ClassifyResponse (served under {"data": ...}).
type classifyResponse struct {
	Verdict         string   `json:"verdict"`
	DenialReason    string   `json:"denial_reason,omitempty"`
	ViolatedCodes   []string `json:"violated_codes,omitempty"`
	Confidence      float64  `json:"confidence"`
	EscalationLevel string   `json:"escalation_level,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "stub-mdemg:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	sp, err := secrets.FromEnvironment()
	if err != nil {
		return err
	}
	cfg, err := config.Load(ctx, "stub-mdemg", sp)
	if err != nil {
		return err
	}
	log := cfg.Logger()
	apiKey := os.Getenv("STUB_API_KEY")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", httpx.Health("stub-mdemg", config.Version))
	mux.HandleFunc("GET /v1/jiminy/healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "enabled": true, "stub": true})
	})

	mux.HandleFunc("POST /v1/jiminy/warm", func(w http.ResponseWriter, r *http.Request) {
		req := readJSON(r)
		if str(req["space_id"]) == "" {
			badRequest(w, "space_id is required")
			return
		}
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "warming", "age_ms": 0, "debounce_ms": 2000})
	})
	mux.HandleFunc("GET /v1/jiminy/latest", func(w http.ResponseWriter, r *http.Request) {
		space := r.URL.Query().Get("space_id")
		if space == "" {
			badRequest(w, "space_id is required")
			return
		}
		g := guideFor(space)
		status := "ok"
		if len(g.Guidance) == 0 {
			status = "no_guidance_available"
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"warm": true, "status": status, "data": g, "age_ms": 12, "compute_ms": 3, "stale": false,
		})
	})
	mux.HandleFunc("POST /v1/jiminy/guide", func(w http.ResponseWriter, r *http.Request) {
		req := readJSON(r)
		if str(req["space_id"]) == "" {
			badRequest(w, "space_id is required")
			return
		}
		if str(req["context"]) == "" {
			badRequest(w, "context is required")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": guideFor(str(req["space_id"]))})
	})
	mux.HandleFunc("POST /v1/jiminy/classify", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(raw, &req)
		if str(req["space_id"]) == "" {
			badRequest(w, "space_id is required")
			return
		}
		resp := classifyResponse{Verdict: "pass", ViolatedCodes: []string{}, Confidence: 0.93}
		if strings.Contains(string(raw), "stub:deny") {
			resp = classifyResponse{
				Verdict: "deny", DenialReason: "stub: payload requested a denial",
				ViolatedCodes: []string{"stub-deny"}, Confidence: 0.97, EscalationLevel: "escalated",
			}
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": resp})
	})
	mux.HandleFunc("POST /v1/jiminy/feedback", func(w http.ResponseWriter, r *http.Request) {
		req := readJSON(r)
		if str(req["guidance_id"]) == "" {
			badRequest(w, "guidance_id is required")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"guidance_id": req["guidance_id"], "results": []any{}, "applied": true})
	})
	mux.HandleFunc("POST /v1/conversation/observe", func(w http.ResponseWriter, r *http.Request) {
		req := readJSON(r)
		if str(req["space_id"]) == "" || str(req["content"]) == "" {
			badRequest(w, "space_id and content are required")
			return
		}
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"obs_id": newID("obs"), "node_id": newID("node"), "surprise_score": 0.42,
			"surprise_factors": map[string]any{"novelty": 0.5, "contradiction": 0.0}, "summary": truncate(str(req["content"]), 120),
		})
	})

	var handler http.Handler = mux
	if apiKey != "" {
		handler = requireKey(apiKey, mux)
		log.Info("api key required")
	}
	return httpx.Run(ctx, log, cfg.ListenAddr, withRequestID(handler))
}

func guideFor(space string) guideResponse {
	g := guideResponse{
		GuidanceID: newID("gd"), Guidance: []guidanceItem{}, PromptAugmentation: "",
		Confidence: 0.0, Rationale: "stub: no constraints in this space",
	}
	if strings.HasSuffix(space, ".governance") {
		g.Guidance = []guidanceItem{{
			Type: "constraint", Priority: "high",
			Content:    "A CAR at or above the VP Finance threshold requires VP Finance co-sign (stub).",
			Confidence: 0.88, SourceNodes: []string{"stub-node-1"}, EscalationLevel: "surfaced",
			ConstraintCode: "car-over-threshold-needs-vp-finance", Tier: 1,
			Annotations: map[string]string{"roles": "finance_approver,owner", "scope": "capex"},
		}}
		g.PromptAugmentation = "[T1] car-over-threshold-needs-vp-finance"
		g.Confidence = 0.88
		g.Rationale = "stub: one governance constraint surfaced"
		g.SourceCounts.Constraints = 1
	}
	return g
}

func badRequest(w http.ResponseWriter, msg string) {
	httpx.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
}

func requireKey(key string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get("X-API-Key")
		if got == "" {
			got = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if got != key {
			httpx.WriteJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or missing api key"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newID("req")
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

func readJSON(r *http.Request) map[string]any {
	var m map[string]any
	_ = json.NewDecoder(r.Body).Decode(&m)
	if m == nil {
		m = map[string]any{}
	}
	return m
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func newID(prefix string) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return fmt.Sprintf("stub-%s-%d-%s", prefix, time.Now().UnixMilli(), hex.EncodeToString(b))
}
