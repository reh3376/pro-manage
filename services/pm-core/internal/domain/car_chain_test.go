package domain

import (
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/governance"
	"github.com/reh3376/pro-manage/services/pm-core/internal/policy"
)

func rat(s string) *big.Rat { x, _ := new(big.Rat).SetString(s); return x }

func seedPolicy() CARPolicy {
	return CARPolicy{
		ThrFinance: rat("0"), ThrVPFinance: rat("5000000"), ThrCFO: rat("25000000"), ThrCEO: rat("100000000"),
		Basis: "tic", Currency: "USD", FELClass: 5, FELCapPct: rat("5"), ESignAbove: rat("1000000"), FullClass: 3, OverdueGrace: 24,
		SLAHours:  map[string]int{"project_owner": 24, "finance_approver": 48, "vp_finance": 72, "cfo": 72, "ceo": 168},
		TierMajor: governance.TierA, TierMinor: governance.TierB, ClassifyTimeout: 10 * time.Second,
	}
}

func roles(chain []StageDef) []string {
	out := make([]string, len(chain))
	for i, s := range chain {
		out[i] = s.Role
	}
	return out
}

func TestBuildCARChain(t *testing.T) {
	p := seedPolicy()
	cases := map[string][]string{
		"640000.00":    {"project_owner", "finance_approver"},
		"4999999.99":   {"project_owner", "finance_approver"},
		"5000000.00":   {"project_owner", "finance_approver", "vp_finance"},
		"42000000.00":  {"project_owner", "finance_approver", "vp_finance", "cfo"},
		"100000000.00": {"project_owner", "finance_approver", "vp_finance", "cfo", "ceo"},
	}
	for amt, want := range cases {
		got := roles(BuildCARChain(rat(amt), p))
		if len(got) != len(want) {
			t.Fatalf("%s: got %v want %v", amt, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: got %v want %v", amt, got, want)
			}
		}
	}
	chain := BuildCARChain(rat("42000000"), p)
	if chain[3].Order != 4 || chain[3].SLAHours != 72 || chain[1].Pool != "finance_approver_eligible" {
		t.Fatalf("stage detail: %+v", chain)
	}
}

func TestCARTier(t *testing.T) {
	p := seedPolicy()
	if CARTier("full", rat("5000000"), p) != governance.TierA {
		t.Fatal("full at threshold is Tier A")
	}
	if CARTier("full", rat("4999999.99"), p) != governance.TierB {
		t.Fatal("full below threshold is Tier B")
	}
	if CARTier("supplemental", rat("6000000"), p) != governance.TierA {
		t.Fatal("supplemental cumulative over threshold is Tier A")
	}
	if CARTier("fel_funding", rat("9000000"), p) != governance.TierB {
		t.Fatal("fel_funding is always the minor tier")
	}
}

func TestPolicyValueParsing(t *testing.T) {
	if x, err := ratOf(policy.Resolved{Key: "k", Value: json.RawMessage(`5000000`)}); err != nil || x.Cmp(rat("5000000")) != 0 {
		t.Fatalf("number: %v %v", x, err)
	}
	if x, err := ratOf(policy.Resolved{Key: "k", Value: json.RawMessage(`"6750000.50"`)}); err != nil || x.Cmp(rat("6750000.50")) != 0 {
		t.Fatalf("string: %v %v", x, err)
	}
	if _, err := ratOf(policy.Resolved{Key: "k", Value: json.RawMessage(`true`)}); err == nil {
		t.Fatal("bool must fail")
	}
	if n, err := intOf(policy.Resolved{Key: "k", Value: json.RawMessage(`72`)}); err != nil || n != 72 {
		t.Fatalf("int: %v %v", n, err)
	}
	if _, err := intOf(policy.Resolved{Key: "k", Value: json.RawMessage(`7.5`)}); err == nil {
		t.Fatal("fraction must fail as integer")
	}
	b, err := json.Marshal(seedPolicy())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["threshold_vp_finance"] != "5000000.00" || m["classify_timeout_ms"] != float64(10000) || m["tier_major"] != "A" {
		t.Fatalf("snapshot json: %s", b)
	}
}

func TestParseMoney(t *testing.T) {
	if _, err := parseMoney("1,000", "amount"); err == nil {
		t.Fatal("comma must fail")
	}
	if x, err := parseMoney(" 1500000.5 ", "amount"); err != nil || money(x) != "1500000.50" {
		t.Fatalf("%v %v", x, err)
	}
}
