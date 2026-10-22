// Package policy resolves the effective value of a policy variable for a
// project (FSD §08 "Policy variables"): most specific scope wins,
// project → site → portfolio → project type → global default.
package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
)

// Scope identifies the project a value is resolved for. Empty fields are skipped.
type Scope struct {
	ProjectID   string
	SiteID      string
	PortfolioID string
	ProjectType string
}

// Resolved is the effective value and where it came from.
type Resolved struct {
	Key           string          `json:"key"`
	Type          string          `json:"type"`
	Unit          *string         `json:"unit,omitempty"`
	Value         json.RawMessage `json:"value"`
	ScopeKind     string          `json:"scope_kind"` // project | site | portfolio | project_type | global | default
	ScopeID       string          `json:"scope_id,omitempty"`
	SetBy         string          `json:"set_by,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	EffectiveFrom *time.Time      `json:"effective_from,omitempty"`
}

// order is the resolution precedence, most specific first.
var order = []string{"project", "site", "portfolio", "project_type", "global"}

// ErrUnknownVariable is returned for a key that is not defined.
var ErrUnknownVariable = errors.New("policy: unknown variable")

// Resolve returns the effective value of key for scope at time now.
func Resolve(ctx context.Context, q *db.Queries, key string, scope Scope, now time.Time) (Resolved, error) {
	v, err := q.GetPolicyVariable(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Resolved{}, fmt.Errorf("%w: %s", ErrUnknownVariable, key)
	}
	if err != nil {
		return Resolved{}, fmt.Errorf("policy: get variable: %w", err)
	}
	values, err := q.ListEffectivePolicyValues(ctx, db.ListEffectivePolicyValuesParams{VariableKey: key, EffectiveFrom: now})
	if err != nil {
		return Resolved{}, fmt.Errorf("policy: list values: %w", err)
	}
	return Pick(v, values, scope), nil
}

// Pick is the pure resolution step, separated for tests: values are the rows in
// force (newest first); the first match in precedence order wins; the
// variable's global_default is the fallback.
func Pick(v db.PolicyVariable, values []db.PolicyValue, scope Scope) Resolved {
	ids := map[string]string{
		"project": scope.ProjectID, "site": scope.SiteID, "portfolio": scope.PortfolioID,
		"project_type": scope.ProjectType, "global": "",
	}
	for _, kind := range order {
		id, ok := ids[kind]
		if kind != "global" && id == "" {
			continue
		}
		if !ok {
			continue
		}
		for _, pv := range values { // newest first, so the first hit is the current value for that scope
			if pv.ScopeKind == kind && pv.ScopeID == id {
				ef := pv.EffectiveFrom
				return Resolved{Key: v.Key, Type: v.Type, Unit: v.Unit, Value: pv.Value, ScopeKind: kind, ScopeID: id,
					SetBy: pv.SetBy, Reason: pv.Reason, EffectiveFrom: &ef}
			}
		}
	}
	return Resolved{Key: v.Key, Type: v.Type, Unit: v.Unit, Value: v.GlobalDefault, ScopeKind: "default"}
}
