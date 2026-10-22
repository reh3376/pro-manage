package governance

import (
	"testing"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/pkg/mdemgadapter"
)

func TestResolveTiers(t *testing.T) {
	deny := Verdict{Decision: mdemgadapter.Decision{Verdict: mdemgadapter.VerdictDeny, ViolatedCodes: []string{"x"}}}
	pass := Verdict{Decision: mdemgadapter.Decision{Verdict: mdemgadapter.VerdictPass}}
	down := Verdict{Unavailable: true, Error: "dial"}

	cases := []struct {
		name string
		tier Tier
		v    Verdict
		resp string
		want Resolution
	}{
		{"A pass follows", TierA, pass, "", Resolution{Proceed: true, UserResponse: ResponseFollowed, Verdict: "pass"}},
		{"A pass dismissed with reason", TierA, pass, ResponseIgnoredWithReason, Resolution{Proceed: true, UserResponse: ResponseIgnoredWithReason, Verdict: "pass"}},
		{"A deny blocks", TierA, deny, "", Resolution{Denied: true, UserResponse: ResponseBlocked, Verdict: "deny"}},
		{"B deny blocks too", TierB, deny, "", Resolution{Denied: true, UserResponse: ResponseBlocked, Verdict: "deny"}},
		{"A unavailable refuses", TierA, down, "", Resolution{Refused: true, UserResponse: ResponseMDEMGUnavailable}},
		{"B unavailable proceeds with warning", TierB, down, "", Resolution{Proceed: true, UserResponse: ResponseMDEMGUnavailable}},
		{"C never classified", TierC, down, "", Resolution{Proceed: true}},
	}
	for _, c := range cases {
		if got := Resolve(c.tier, c.v, c.resp); got != c.want {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}

func TestParseTier(t *testing.T) {
	for in, want := range map[string]Tier{"A": TierA, "b": TierB, "C": TierC} {
		if got, err := ParseTier(in); err != nil || got != want {
			t.Fatalf("%q: %v %v", in, got, err)
		}
	}
	if _, err := ParseTier("D"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCacheKeyAndCache(t *testing.T) {
	k1 := CacheKey("approval.car", "payload", []byte(`{"a":1}`))
	k2 := CacheKey("approval.car", "payload", []byte(`{"a":2}`))
	if k1 == k2 || len(k1) != 64 {
		t.Fatal("snapshot must be part of the key")
	}
	c := NewPreclassCache(50 * time.Millisecond)
	c.Put("alice", "p1", k1, mdemgadapter.Decision{Verdict: "pass"})
	if d, ok := c.Get("alice", "p1", k1); !ok || d.Verdict != "pass" {
		t.Fatal("expected hit")
	}
	if _, ok := c.Get("bob", "p1", k1); ok {
		t.Fatal("cache must be scoped to the user")
	}
	time.Sleep(60 * time.Millisecond)
	if _, ok := c.Get("alice", "p1", k1); ok {
		t.Fatal("expected expiry")
	}
	if c.Len() != 0 {
		t.Fatalf("expired entry should be dropped on read, len=%d", c.Len())
	}
}

func TestClassifyWithoutClient(t *testing.T) {
	g := &Gate{OrgID: "pm_dev", Cache: NewPreclassCache(0)}
	sp := g.Spaces("01900000-0000-7000-8000-00000000b001")
	if sp.Governance != "org.pm_dev.governance" || sp.Project != "proj_0190000000007000800000000000b001" {
		t.Fatalf("spaces: %+v", sp)
	}
	v := g.Classify(t.Context(), sp, ClassifyInput{ActionKind: "approval.car", Payload: "x"})
	if !v.Unavailable || v.Cached {
		t.Fatalf("nil client must be unavailable: %+v", v)
	}
	// A cached verdict is honoured even with no client (MDEMG became unavailable after settle, §06).
	g.Cache.Put("", "", v.CacheKey, mdemgadapter.Decision{Verdict: "pass"})
	if v2 := g.Classify(t.Context(), sp, ClassifyInput{ActionKind: "approval.car", Payload: "x"}); !v2.Cached || v2.Unavailable {
		t.Fatalf("expected cache hit: %+v", v2)
	}
}
