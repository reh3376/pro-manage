package policy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
)

func val(kind, id, v string, at time.Time) db.PolicyValue {
	return db.PolicyValue{VariableKey: "car.threshold.vp_finance", ScopeKind: kind, ScopeID: id, Value: json.RawMessage(v), EffectiveFrom: at, SetBy: "dana.policy"}
}

func TestPickPrecedence(t *testing.T) {
	variable := db.PolicyVariable{Key: "car.threshold.vp_finance", Type: "money", GlobalDefault: json.RawMessage(`5000000`)}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	values := []db.PolicyValue{
		val("project", "p1", `7500000`, t0.Add(48*time.Hour)),
		val("site", "nf-on", `6000000`, t0.Add(24*time.Hour)),
		val("portfolio", "spirits", `5500000`, t0),
		val("global", "", `5000000`, t0),
	}
	scope := Scope{ProjectID: "p1", SiteID: "nf-on", PortfolioID: "spirits", ProjectType: "greenfield"}

	got := Pick(variable, values, scope)
	if got.ScopeKind != "project" || string(got.Value) != `7500000` {
		t.Fatalf("project override should win: %+v", got)
	}
	got = Pick(variable, values, Scope{SiteID: "nf-on", PortfolioID: "spirits"})
	if got.ScopeKind != "site" || string(got.Value) != `6000000` {
		t.Fatalf("site should win without a project value: %+v", got)
	}
	got = Pick(variable, values, Scope{ProjectID: "other", SiteID: "nf-ky", PortfolioID: "spirits"})
	if got.ScopeKind != "portfolio" || string(got.Value) != `5500000` {
		t.Fatalf("portfolio should win: %+v", got)
	}
	got = Pick(variable, values, Scope{ProjectID: "other", SiteID: "nf-ky", PortfolioID: "other"})
	if got.ScopeKind != "global" || string(got.Value) != `5000000` {
		t.Fatalf("global value should win over the default: %+v", got)
	}
	got = Pick(variable, nil, scope)
	if got.ScopeKind != "default" || string(got.Value) != `5000000` {
		t.Fatalf("default should apply with no values: %+v", got)
	}
}

func TestPickNewestWithinScope(t *testing.T) {
	variable := db.PolicyVariable{Key: "k", Type: "money", GlobalDefault: json.RawMessage(`1`)}
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	values := []db.PolicyValue{ // newest first, as the query orders them
		val("project", "p1", `3`, t0.Add(time.Hour)),
		val("project", "p1", `2`, t0),
	}
	if got := Pick(variable, values, Scope{ProjectID: "p1"}); string(got.Value) != `3` {
		t.Fatalf("newest value for the scope should win: %+v", got)
	}
}
