package sla

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvaluate(t *testing.T) {
	entered := time.Date(2026, 10, 26, 8, 0, 0, 0, time.UTC)
	due := entered.Add(24 * time.Hour)
	grace := 6 * time.Hour
	at := func(h float64) time.Time { return entered.Add(time.Duration(h * float64(time.Hour))) }
	ptr := func(t time.Time) *time.Time { return &t }
	base := State{EnteredAt: entered, DueAt: due, Grace: grace, Expires: true}

	cases := []struct {
		name string
		s    State
		now  time.Time
		want Action
	}{
		{"fresh stage", base, at(1), None},
		{"just under 75%", base, at(17.9), None},
		{"75% nudges", base, at(18), Nudge},
		{"already nudged", withNudged(base, ptr(at(18))), at(20), None},
		{"100% overdue", base, at(24), Overdue},
		{"overdue even if never nudged", base, at(30), Overdue},
		{"overdue inside grace waits", withOverdue(base, ptr(at(24))), at(29), None},
		{"grace passed escalates", withOverdue(base, ptr(at(24))), at(30), Escalate},
		{"escalated, new due not reached", withEscalated(base, ptr(at(30)), at(36)), at(35), None},
		{"escalated, new due passed expires", withEscalated(base, ptr(at(30)), at(36)), at(36), Expire},
		{"non-expiring kind stays overdue", withEscalated(State{EnteredAt: entered, DueAt: due, Grace: grace, Expires: false}, ptr(at(30)), at(36)), at(99), None},
		{"degenerate stage", State{EnteredAt: entered, DueAt: entered}, at(5), None},
	}
	for _, c := range cases {
		if got := Evaluate(c.s, c.now); got != c.want {
			t.Errorf("%s: got %s want %s", c.name, got, c.want)
		}
	}
}

func withNudged(s State, t *time.Time) State  { s.NudgedAt = t; return s }
func withOverdue(s State, t *time.Time) State { s.OverdueAt, s.NudgedAt = t, t; return s }
func withEscalated(s State, t *time.Time, due time.Time) State {
	s.EscalatedAt, s.OverdueAt, s.NudgedAt, s.DueAt = t, t, t, due
	return s
}

func TestGraceFromSnapshot(t *testing.T) {
	if graceFrom([]byte(`{}`)) != DefaultGrace || graceFrom(nil) != DefaultGrace {
		t.Fatal("default")
	}
	b, _ := json.Marshal(map[string]any{"sla_overdue_grace_hours": 12})
	if graceFrom(b) != 12*time.Hour {
		t.Fatal("snapshot grace")
	}
	if Nudge.String() != "nudge" || Expire.String() != "expire" {
		t.Fatal("names")
	}
}
