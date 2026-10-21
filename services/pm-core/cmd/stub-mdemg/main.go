// stub-mdemg is a stand-in for MDEMG in PR CI (FSD §06, §12).
//
// CI cannot run MDEMG's local language model, so this binary answers the
// endpoints pm-core calls with the same field shapes and canned verdicts:
//
//	GET  /healthz
//	POST /v1/jiminy/warm           -> 202 {"status":"warming"}
//	GET  /v1/jiminy/latest         -> guidance[] for the session
//	POST /v1/jiminy/guide          -> guidance[]
//	POST /v1/jiminy/classify       -> {verdict, denial_reason, violated_codes, confidence, escalation_level}
//	POST /v1/jiminy/feedback       -> 202 {"status":"recorded"}
//	POST /v1/conversation/observe  -> 202 {"status":"observed","observation_id":...}
//
// Canned behaviour: a classify request whose body contains the token
// "stub:deny" is denied with constraint code "stub-deny"; everything else is
// allowed. A guide request against a space ending in ".governance" returns one
// high-priority card so the guidance rail has something to render.
//
// When STUB_API_KEY is set, every request must carry it as X-API-Key or a
// Bearer token, mirroring MDEMG's apikey auth so pm-core's auth path is tested.
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

type guidanceItem struct {
	Type            string         `json:"type"`
	Priority        string         `json:"priority"`
	Content         string         `json:"content"`
	Confidence      float64        `json:"confidence"`
	EscalationLevel string         `json:"escalation_level"`
	ConstraintCode  string         `json:"constraint_code"`
	Tier            string         `json:"tier"`
	Annotations     map[string]any `json:"annotations"`
	SourceNodes     []string       `json:"source_nodes"`
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

	mux.HandleFunc("POST /v1/jiminy/warm", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "warming", "request_id": newID()})
	})
	mux.HandleFunc("GET /v1/jiminy/latest", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"guidance_id": newID(), "session_id": r.URL.Query().Get("session_id"),
			"guidance": guidanceFor(r.URL.Query().Get("space_id")), "warmed": true,
		})
	})
	mux.HandleFunc("POST /v1/jiminy/guide", func(w http.ResponseWriter, r *http.Request) {
		req := readJSON(r)
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"guidance_id": newID(), "session_id": req["session_id"],
			"guidance": guidanceFor(str(req["space_id"])),
		})
	})
	mux.HandleFunc("POST /v1/jiminy/classify", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		deny := strings.Contains(string(raw), "stub:deny")
		resp := map[string]any{
			"verdict": "allow", "denial_reason": "", "violated_codes": []string{},
			"confidence": 0.93, "escalation_level": "none", "request_id": newID(),
		}
		if deny {
			resp["verdict"] = "deny"
			resp["denial_reason"] = "stub: payload requested a denial"
			resp["violated_codes"] = []string{"stub-deny"}
			resp["escalation_level"] = "escalated"
		}
		httpx.WriteJSON(w, http.StatusOK, resp)
	})
	mux.HandleFunc("POST /v1/jiminy/feedback", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "recorded", "request_id": newID()})
	})
	mux.HandleFunc("POST /v1/conversation/observe", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"status": "observed", "observation_id": newID(), "request_id": newID()})
	})

	var handler http.Handler = mux
	if apiKey != "" {
		handler = requireKey(apiKey, mux)
		log.Info("api key required")
	}
	return httpx.Run(ctx, log, cfg.ListenAddr, withRequestID(handler))
}

func guidanceFor(space string) []guidanceItem {
	if !strings.HasSuffix(space, ".governance") {
		return []guidanceItem{}
	}
	return []guidanceItem{{
		Type: "constraint", Priority: "high",
		Content:    "A CAR at or above the VP Finance threshold requires VP Finance co-sign (stub).",
		Confidence: 0.88, EscalationLevel: "surfaced",
		ConstraintCode: "car-over-threshold-needs-vp-finance", Tier: "A",
		Annotations: map[string]any{"roles": []string{"finance_approver", "owner"}, "scope": "capex"},
		SourceNodes: []string{"stub-node-1"},
	}}
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
			id = newID()
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

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("stub-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b))
}
