package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/reh3376/pro-manage/services/pm-core/internal/db"
	"github.com/reh3376/pro-manage/services/pm-core/internal/governance"
	"github.com/reh3376/pro-manage/services/pm-core/internal/policy"
)

// CAR policy variable keys (FSD §08 "Starter variables"; seeded in db/seeds/northfield.sql).
const (
	keyThrFinance    = "car.threshold.finance_approver"
	keyThrVPFinance  = "car.threshold.vp_finance"
	keyThrCFO        = "car.threshold.cfo"
	keyThrCEO        = "car.threshold.ceo"
	keyThrBasis      = "car.threshold.basis"
	keyThrCurrency   = "car.threshold.currency"
	keyFELClass      = "car.fel_funding.required_aace_class"
	keyFELCapPct     = "car.fel_funding.cap_pct"
	keyFullClass     = "car.full.required_aace_class"
	keySLAOwner      = "sla.car.project_owner"
	keySLAFinance    = "sla.car.finance_approver"
	keySLAVPFinance  = "sla.car.vp_finance"
	keySLACFO        = "sla.car.cfo"
	keySLACEO        = "sla.car.ceo"
	keyTierCAR       = "mdemg.tier.approval.car"
	keyTierCARMinor  = "mdemg.tier.approval.car.minor"
	keyClassifyTOms  = "mdemg.classify.timeout_ms"
	keyESignAbove    = "car.esign.required_above"
	carPolicyVersion = 2
)

// carPolicyKeys is everything a CAR workflow snapshots at submission (§08 "Snapshot at start").
var carPolicyKeys = []string{
	keyThrFinance, keyThrVPFinance, keyThrCFO, keyThrCEO, keyThrBasis, keyThrCurrency,
	keyFELClass, keyFELCapPct, keyFullClass,
	keySLAOwner, keySLAFinance, keySLAVPFinance, keySLACFO, keySLACEO,
	keyTierCAR, keyTierCARMinor, keyClassifyTOms, keyESignAbove,
}

// CARPolicy is the parsed snapshot the chain is built from.
type CARPolicy struct {
	Version         int                        `json:"version"`
	ThrFinance      *big.Rat                   `json:"-"`
	ThrVPFinance    *big.Rat                   `json:"-"`
	ThrCFO          *big.Rat                   `json:"-"`
	ThrCEO          *big.Rat                   `json:"-"`
	Basis           string                     `json:"basis"`
	Currency        string                     `json:"currency"`
	FELClass        int                        `json:"fel_funding_required_aace_class"`
	FELCapPct       *big.Rat                   `json:"-"`
	ESignAbove      *big.Rat                   `json:"-"` // car.esign.required_above: DocuSign at or above, internal attestation below (§08)
	FullClass       int                        `json:"full_required_aace_class"`
	SLAHours        map[string]int             `json:"sla_hours"` // by role
	TierMajor       governance.Tier            `json:"tier_major"`
	TierMinor       governance.Tier            `json:"tier_minor"`
	ClassifyTimeout time.Duration              `json:"-"`
	Resolved        map[string]policy.Resolved `json:"resolved"` // key -> effective value and provenance
}

// MarshalJSON renders the snapshot with the money values as decimal strings.
func (p CARPolicy) MarshalJSON() ([]byte, error) {
	type alias CARPolicy
	return json.Marshal(struct {
		alias
		ThrFinance        string `json:"threshold_finance_approver"`
		ThrVPFinance      string `json:"threshold_vp_finance"`
		ThrCFO            string `json:"threshold_cfo"`
		ThrCEO            string `json:"threshold_ceo"`
		FELCapPct         string `json:"fel_funding_cap_pct"`
		ESignAbove        string `json:"threshold_esign"`
		ClassifyTimeoutMs int64  `json:"classify_timeout_ms"`
	}{alias(p), money(p.ThrFinance), money(p.ThrVPFinance), money(p.ThrCFO), money(p.ThrCEO), p.FELCapPct.FloatString(2), money(p.ESignAbove), p.ClassifyTimeout.Milliseconds()})
}

// resolveCARPolicy resolves every key for the project at now. A key that is not
// defined is a precondition failure, not a silent default: the FSD forbids
// hard-coded numbers, so a missing variable means the policy load is incomplete.
func resolveCARPolicy(ctx context.Context, q *db.Queries, project db.Project, now time.Time) (CARPolicy, error) {
	scope := policy.Scope{ProjectID: project.ProjectID.String(), SiteID: project.SiteID.String(), ProjectType: project.ProjectType}
	if project.PortfolioID != nil {
		scope.PortfolioID = project.PortfolioID.String()
	}
	p := CARPolicy{Version: carPolicyVersion, Resolved: map[string]policy.Resolved{}, SLAHours: map[string]int{}}
	for _, k := range carPolicyKeys {
		r, err := policy.Resolve(ctx, q, k, scope, now)
		if errors.Is(err, policy.ErrUnknownVariable) {
			return p, fmt.Errorf("%w: policy variable %s is not defined (load the DoA policy set)", ErrPrecondition, k)
		}
		if err != nil {
			return p, err
		}
		p.Resolved[k] = r
	}
	var err error
	if p.ThrFinance, err = ratOf(p.Resolved[keyThrFinance]); err != nil {
		return p, err
	}
	if p.ThrVPFinance, err = ratOf(p.Resolved[keyThrVPFinance]); err != nil {
		return p, err
	}
	if p.ThrCFO, err = ratOf(p.Resolved[keyThrCFO]); err != nil {
		return p, err
	}
	if p.ThrCEO, err = ratOf(p.Resolved[keyThrCEO]); err != nil {
		return p, err
	}
	if p.FELCapPct, err = ratOf(p.Resolved[keyFELCapPct]); err != nil {
		return p, err
	}
	if p.ESignAbove, err = ratOf(p.Resolved[keyESignAbove]); err != nil {
		return p, err
	}
	if p.Basis, err = strOf(p.Resolved[keyThrBasis]); err != nil {
		return p, err
	}
	if p.Currency, err = strOf(p.Resolved[keyThrCurrency]); err != nil {
		return p, err
	}
	if p.FELClass, err = intOf(p.Resolved[keyFELClass]); err != nil {
		return p, err
	}
	if p.FullClass, err = intOf(p.Resolved[keyFullClass]); err != nil {
		return p, err
	}
	for role, k := range map[string]string{"project_owner": keySLAOwner, "finance_approver": keySLAFinance, "vp_finance": keySLAVPFinance, "cfo": keySLACFO, "ceo": keySLACEO} {
		if p.SLAHours[role], err = intOf(p.Resolved[k]); err != nil {
			return p, err
		}
	}
	tier, err := strOf(p.Resolved[keyTierCAR])
	if err != nil {
		return p, err
	}
	if p.TierMajor, err = governance.ParseTier(tier); err != nil {
		return p, fmt.Errorf("%w: %s: %v", ErrPrecondition, keyTierCAR, err)
	}
	if tier, err = strOf(p.Resolved[keyTierCARMinor]); err != nil {
		return p, err
	}
	if p.TierMinor, err = governance.ParseTier(tier); err != nil {
		return p, fmt.Errorf("%w: %s: %v", ErrPrecondition, keyTierCARMinor, err)
	}
	ms, err := intOf(p.Resolved[keyClassifyTOms])
	if err != nil {
		return p, err
	}
	p.ClassifyTimeout = time.Duration(ms) * time.Millisecond
	return p, nil
}

// StageDef is one stage of the CAR chain before it is persisted (FSD §08 "Capital authorization").
type StageDef struct {
	Order    int32  `json:"order"`
	Role     string `json:"role_required"`
	Pool     string `json:"pool"`
	SLAHours int    `json:"sla_hours"`
}

// BuildCARChain is the deterministic routing rule: owner and finance approver
// always; VP Finance, CFO, CEO when the routing amount reaches their threshold.
// It is pure so the rule can be unit tested and previewed on the form.
func BuildCARChain(routing *big.Rat, p CARPolicy) []StageDef {
	chain := []StageDef{
		{Order: 1, Role: "project_owner", Pool: "project_owner", SLAHours: p.SLAHours["project_owner"]},
		{Order: 2, Role: "finance_approver", Pool: "finance_approver_eligible", SLAHours: p.SLAHours["finance_approver"]},
	}
	for _, s := range []struct {
		role, pool string
		thr        *big.Rat
	}{{"vp_finance", "vp_finance", p.ThrVPFinance}, {"cfo", "cfo", p.ThrCFO}, {"ceo", "ceo", p.ThrCEO}} {
		if routing.Cmp(s.thr) >= 0 {
			chain = append(chain, StageDef{Order: int32(len(chain) + 1), Role: s.role, Pool: s.pool, SLAHours: p.SLAHours[s.role]})
		}
	}
	return chain
}

// CARTier applies §06's tier table: full and supplemental CARs whose routing
// amount reaches the VP Finance threshold are the major tier (A by default);
// everything else, and every FEL-funding CAR, is the minor tier (B).
func CARTier(kind string, routing *big.Rat, p CARPolicy) governance.Tier {
	if kind != "fel_funding" && routing.Cmp(p.ThrVPFinance) >= 0 {
		return p.TierMajor
	}
	return p.TierMinor
}

// ---------------------------------------------------------------- value helpers

func ratOf(r policy.Resolved) (*big.Rat, error) {
	var v any
	if err := json.Unmarshal(r.Value, &v); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrPrecondition, r.Key, err)
	}
	var s string
	switch t := v.(type) {
	case float64:
		s = big.NewRat(1, 1).SetFloat64(t).FloatString(4)
	case string:
		s = t
	default:
		return nil, fmt.Errorf("%w: %s: expected a number", ErrPrecondition, r.Key)
	}
	x, ok := new(big.Rat).SetString(strings.TrimSpace(s))
	if !ok {
		return nil, fmt.Errorf("%w: %s: not a decimal", ErrPrecondition, r.Key)
	}
	return x, nil
}

func strOf(r policy.Resolved) (string, error) {
	var s string
	if err := json.Unmarshal(r.Value, &s); err != nil {
		return "", fmt.Errorf("%w: %s: expected a string", ErrPrecondition, r.Key)
	}
	return s, nil
}

func intOf(r policy.Resolved) (int, error) {
	x, err := ratOf(r)
	if err != nil {
		return 0, err
	}
	if !x.IsInt() {
		return 0, fmt.Errorf("%w: %s: expected an integer", ErrPrecondition, r.Key)
	}
	return int(x.Num().Int64()), nil
}

// parseMoney accepts a decimal string with up to two places.
func parseMoney(s, field string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if !moneyRe.MatchString(s) {
		return nil, validationError(field, "decimal string with up to 2 places, e.g. 1500000.00")
	}
	x, _ := new(big.Rat).SetString(s)
	return x, nil
}

func money(x *big.Rat) string {
	if x == nil {
		return "0.00"
	}
	return x.FloatString(2)
}
