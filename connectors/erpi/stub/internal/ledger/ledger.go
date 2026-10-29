// Package ledger is the stub's in-memory ERP: projects with a node tree, budget
// and forecast versions, an EAC, posted actuals, commitments, and the budget
// releases pm-core asked for. Everything an ERP's projects module would answer
// for the spine, without an ERP. Seeded with the Northfield fixture; the dev
// endpoints add projects and post actuals for tests.
package ledger

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
)

// Errors.
var (
	ErrNotFound = errors.New("project not found")
	ErrExists   = errors.New("project exists")
	ErrInvalid  = errors.New("invalid")
)

// Node is one node of the project tree.
type Node struct {
	Ref, ParentRef, Kind, Code, Name, Status string
	PlannedStart, PlannedFinish              time.Time
	Attributes                               map[string]string
}

// Line is one WBS or account line.
type Line struct {
	WbsPath, AccountCode, Amount string
}

// Version is a budget or forecast version.
type Version struct {
	Ref, Name, Status, Total, Currency string
	AsOf, ApprovedAt                   time.Time
	Lines                              []Line
	Attributes                         map[string]string
}

// Eac is the current estimate at completion.
type Eac struct {
	AsOf             time.Time
	Amount, Currency string
	Method           string
	Lines            []Line
}

// Actual is one posted cost line.
type Actual struct {
	Ref, WbsPath, AccountCode, Period, Amount, Currency, SourceDocRef string
	PostedAt                                                          time.Time
}

// Commitment is one PO or contract.
type Commitment struct {
	Ref, WbsPath, AccountCode, VendorRef, PORef, Amount, InvoicedToDate, Currency, Status string
	CreatedAt                                                                             time.Time
}

// Release is one budget release pm-core made.
type Release struct {
	BudgetRef, CARID, ProjectRef, Amount, Currency, ApprovalRef, PlanLineRef, Description, ReleasedBy string
	ReleasedAt                                                                                        time.Time
}

// Project is one ERP project.
type Project struct {
	Ref, Code, Name, Currency, Status string
	Nodes                             []Node
	Baselines, Forecasts              []Version
	Eac                               *Eac
	Actuals                           []Actual
	Commitments                       []Commitment
}

// Ledger is the store.
type Ledger struct {
	mu       sync.Mutex
	projects map[string]*Project
	releases map[string]Release // by car_id
	seq      int
}

// New returns an empty ledger.
func New() *Ledger {
	return &Ledger{projects: map[string]*Project{}, releases: map[string]Release{}}
}

// Add registers a project; the ref must be new.
func (l *Ledger) Add(p Project) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if p.Ref == "" {
		return fmt.Errorf("%w: project ref required", ErrInvalid)
	}
	if _, ok := l.projects[p.Ref]; ok {
		return ErrExists
	}
	cp := p
	l.projects[p.Ref] = &cp
	return nil
}

// Get returns a copy of a project.
func (l *Ledger) Get(ref string) (Project, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	p, ok := l.projects[ref]
	if !ok {
		return Project{}, ErrNotFound
	}
	return *p, nil
}

// Refs lists project refs, sorted.
func (l *Ledger) Refs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.projects))
	for r := range l.projects {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// PostActual appends an actual (dev endpoint). An empty ref gets a generated one.
func (l *Ledger) PostActual(ref string, a Actual) (Actual, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	p, ok := l.projects[ref]
	if !ok {
		return Actual{}, ErrNotFound
	}
	if _, ok := new(big.Rat).SetString(a.Amount); !ok {
		return Actual{}, fmt.Errorf("%w: amount %q", ErrInvalid, a.Amount)
	}
	l.seq++
	if a.Ref == "" {
		a.Ref = fmt.Sprintf("ACT-%s-%06d", ref, l.seq)
	}
	if a.Currency == "" {
		a.Currency = p.Currency
	}
	if a.PostedAt.IsZero() {
		a.PostedAt = time.Now().UTC()
	}
	if a.Period == "" {
		a.Period = a.PostedAt.Format("2006-01")
	}
	p.Actuals = append(p.Actuals, a)
	return a, nil
}

// Release records a budget release, idempotent on the CAR id: a repeat returns
// the earlier release and reports it as such. The released amount becomes an
// approved budget version on the project, the way an ERP's budget module would
// show it.
func (l *Ledger) Release(r Release) (Release, bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if r.CARID == "" {
		return Release{}, false, fmt.Errorf("%w: car_id required", ErrInvalid)
	}
	if prev, ok := l.releases[r.CARID]; ok {
		return prev, true, nil
	}
	p, ok := l.projects[r.ProjectRef]
	if !ok {
		return Release{}, false, ErrNotFound
	}
	amt, ok := new(big.Rat).SetString(r.Amount)
	if !ok || amt.Sign() <= 0 {
		return Release{}, false, fmt.Errorf("%w: amount %q must be a positive decimal", ErrInvalid, r.Amount)
	}
	if r.Currency == "" {
		r.Currency = p.Currency
	}
	if !strings.EqualFold(r.Currency, p.Currency) {
		return Release{}, false, fmt.Errorf("%w: currency %s does not match the project's %s", ErrInvalid, r.Currency, p.Currency)
	}
	l.seq++
	r.BudgetRef = fmt.Sprintf("BUD-%s-%04d", p.Ref, l.seq)
	r.ReleasedAt = time.Now().UTC()
	r.Amount = amt.FloatString(2)
	l.releases[r.CARID] = r
	name := "Release " + r.Description
	if r.Description == "" {
		name = "Release CAR " + r.CARID
	}
	p.Baselines = append(p.Baselines, Version{Ref: r.BudgetRef, Name: name, Status: "approved", Total: r.Amount, Currency: r.Currency, AsOf: r.ReleasedAt, ApprovedAt: r.ReleasedAt,
		Lines: []Line{{WbsPath: p.Code, AccountCode: "CAPEX", Amount: r.Amount}}, Attributes: map[string]string{"car_id": r.CARID, "approval_ref": r.ApprovalRef, "released_by": r.ReleasedBy}})
	return r, false, nil
}

// Releases lists every release, newest first.
func (l *Ledger) Releases() []Release {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Release, 0, len(l.releases))
	for _, r := range l.releases {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ReleasedAt.After(out[j].ReleasedAt) })
	return out
}

// Len is the project count.
func (l *Ledger) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.projects)
}

// Northfield seeds the dev ledger with the seed company's projects (keyed on
// pro_manage's project code, which is the stub's project-code mapping).
func Northfield() []Project {
	d := func(s string) time.Time { t, _ := time.Parse("2006-01-02", s); return t }
	return []Project{
		{
			Ref: "P-NF-KY-7", Code: "P-NF-KY-7", Name: "Still House 2 column still", Currency: "USD", Status: "planning",
			Nodes: []Node{
				{Ref: "P-NF-KY-7", Kind: "project", Code: "P-NF-KY-7", Name: "Still House 2 column still", Status: "planning", PlannedStart: d("2026-09-01"), PlannedFinish: d("2028-03-31")},
				{Ref: "P-NF-KY-7.10", ParentRef: "P-NF-KY-7", Kind: "sub_project", Code: "10", Name: "Process equipment", Status: "planning"},
				{Ref: "P-NF-KY-7.20", ParentRef: "P-NF-KY-7", Kind: "sub_project", Code: "20", Name: "Civil and structural", Status: "planning"},
				{Ref: "P-NF-KY-7.M1", ParentRef: "P-NF-KY-7", Kind: "milestone", Code: "M1", Name: "Full CAR approved", Status: "open", PlannedFinish: d("2026-11-30")},
			},
			Baselines: []Version{{Ref: "BL-1", Name: "FEL2 budget placeholder", Status: "superseded", Total: "3500000.00", Currency: "USD", AsOf: d("2026-03-01"), ApprovedAt: d("2026-03-05"),
				Lines: []Line{{WbsPath: "P-NF-KY-7.10", AccountCode: "CAPEX", Amount: "2000000.00"}, {WbsPath: "P-NF-KY-7.20", AccountCode: "CAPEX", Amount: "1500000.00"}}}},
			Forecasts: []Version{{Ref: "FC-2026-Q3", Name: "Forecast 2026-Q3", Status: "approved", Total: "42600000.00", Currency: "USD", AsOf: d("2026-09-30"),
				Lines: []Line{{WbsPath: "P-NF-KY-7.10", AccountCode: "CAPEX", Amount: "27100000.00"}, {WbsPath: "P-NF-KY-7.20", AccountCode: "CAPEX", Amount: "15500000.00"}}}},
			Eac: &Eac{AsOf: d("2026-10-15"), Amount: "42600000.00", Currency: "USD", Method: "manual"},
			Actuals: []Actual{
				{Ref: "ACT-7-0001", WbsPath: "P-NF-KY-7.10", AccountCode: "ENG", Period: "2026-08", Amount: "412500.00", Currency: "USD", SourceDocRef: "INV-EPCM-2026-0812", PostedAt: d("2026-08-28")},
				{Ref: "ACT-7-0002", WbsPath: "P-NF-KY-7.10", AccountCode: "ENG", Period: "2026-09", Amount: "488000.00", Currency: "USD", SourceDocRef: "INV-EPCM-2026-0919", PostedAt: d("2026-09-29")},
				{Ref: "ACT-7-0003", WbsPath: "P-NF-KY-7.20", AccountCode: "SURVEY", Period: "2026-09", Amount: "36200.00", Currency: "USD", SourceDocRef: "INV-GEO-2026-0902", PostedAt: d("2026-09-30")},
			},
			Commitments: []Commitment{
				{Ref: "PO-7-00412", WbsPath: "P-NF-KY-7.10", AccountCode: "ENG", VendorRef: "V-EPCM-01", PORef: "PO-00412", Amount: "2400000.00", InvoicedToDate: "900500.00", Currency: "USD", Status: "open", CreatedAt: d("2026-07-15")},
				{Ref: "PO-7-00418", WbsPath: "P-NF-KY-7.20", AccountCode: "SURVEY", VendorRef: "V-GEO-04", PORef: "PO-00418", Amount: "36200.00", InvoicedToDate: "36200.00", Currency: "USD", Status: "closed", CreatedAt: d("2026-08-20")},
			},
		},
		{
			Ref: "P-NF-KY-12", Code: "P-NF-KY-12", Name: "Bottling Line 3 changeover reduction", Currency: "USD", Status: "execution",
			Nodes: []Node{{Ref: "P-NF-KY-12", Kind: "project", Code: "P-NF-KY-12", Name: "Bottling Line 3 changeover reduction", Status: "execution", PlannedStart: d("2026-04-15"), PlannedFinish: d("2026-12-20")}},
			Baselines: []Version{{Ref: "BL-1", Name: "Baseline 1 (CR-2401)", Status: "approved", Total: "1850000.00", Currency: "USD", AsOf: d("2026-04-02"), ApprovedAt: d("2026-04-02"),
				Lines: []Line{{WbsPath: "P-NF-KY-12", AccountCode: "CAPEX", Amount: "1850000.00"}}, Attributes: map[string]string{"car": "CR-2401"}}},
			Forecasts: []Version{{Ref: "FC-2026-Q3", Name: "Forecast 2026-Q3", Status: "approved", Total: "1790000.00", Currency: "USD", AsOf: d("2026-09-30")}},
			Eac:       &Eac{AsOf: d("2026-10-15"), Amount: "1790000.00", Currency: "USD", Method: "ratio"},
			Actuals: []Actual{
				{Ref: "ACT-12-0001", WbsPath: "P-NF-KY-12", AccountCode: "EQUIP", Period: "2026-06", Amount: "640000.00", Currency: "USD", SourceDocRef: "INV-TOOL-2026-0611", PostedAt: d("2026-06-18")},
				{Ref: "ACT-12-0002", WbsPath: "P-NF-KY-12", AccountCode: "INSTALL", Period: "2026-08", Amount: "385000.00", Currency: "USD", SourceDocRef: "INV-MECH-2026-0803", PostedAt: d("2026-08-21")},
				{Ref: "ACT-12-0003", WbsPath: "P-NF-KY-12", AccountCode: "AUTOMATION", Period: "2026-09", Amount: "212400.00", Currency: "USD", SourceDocRef: "INV-SI-2026-0917", PostedAt: d("2026-09-24")},
			},
			Commitments: []Commitment{
				{Ref: "PO-12-00390", WbsPath: "P-NF-KY-12", AccountCode: "EQUIP", VendorRef: "V-TOOL-02", PORef: "PO-00390", Amount: "640000.00", InvoicedToDate: "640000.00", Currency: "USD", Status: "closed", CreatedAt: d("2026-05-02")},
				{Ref: "PO-12-00401", WbsPath: "P-NF-KY-12", AccountCode: "AUTOMATION", VendorRef: "V-SI-07", PORef: "PO-00401", Amount: "480000.00", InvoicedToDate: "212400.00", Currency: "USD", Status: "open", CreatedAt: d("2026-06-30")},
			},
		},
		{
			Ref: "P-NF-KY-15", Code: "P-NF-KY-15", Name: "Boiler 2 burner replacement", Currency: "USD", Status: "initiation",
			Nodes: []Node{{Ref: "P-NF-KY-15", Kind: "project", Code: "P-NF-KY-15", Name: "Boiler 2 burner replacement", Status: "initiation"}},
		},
		{
			Ref: "P-NF-ON-3", Code: "P-NF-ON-3", Name: "Dust collection upgrade, dry goods", Currency: "CAD", Status: "planning",
			Nodes:     []Node{{Ref: "P-NF-ON-3", Kind: "project", Code: "P-NF-ON-3", Name: "Dust collection upgrade, dry goods", Status: "planning", PlannedStart: d("2026-10-01"), PlannedFinish: d("2027-05-30")}},
			Baselines: []Version{{Ref: "BL-1", Name: "FEL2 placeholder", Status: "approved", Total: "980000.00", Currency: "CAD", AsOf: d("2026-09-15"), ApprovedAt: d("2026-09-20")}},
			Actuals:   []Actual{{Ref: "ACT-3-0001", WbsPath: "P-NF-ON-3", AccountCode: "ENG", Period: "2026-10", Amount: "41800.00", Currency: "CAD", SourceDocRef: "INV-ENG-2026-1003", PostedAt: d("2026-10-12")}},
		},
		{
			Ref: "P-NF-ON-5", Code: "P-NF-ON-5", Name: "Case packer refurbishment", Currency: "CAD", Status: "execution",
			Nodes:       []Node{{Ref: "P-NF-ON-5", Kind: "project", Code: "P-NF-ON-5", Name: "Case packer refurbishment", Status: "execution", PlannedStart: d("2026-08-01"), PlannedFinish: d("2026-11-15")}},
			Baselines:   []Version{{Ref: "BL-1", Name: "Baseline 1", Status: "approved", Total: "60000.00", Currency: "CAD", AsOf: d("2026-07-20"), ApprovedAt: d("2026-07-22")}},
			Actuals:     []Actual{{Ref: "ACT-5-0001", WbsPath: "P-NF-ON-5", AccountCode: "PARTS", Period: "2026-09", Amount: "22150.00", Currency: "CAD", SourceDocRef: "INV-OEM-2026-0909", PostedAt: d("2026-09-16")}},
			Commitments: []Commitment{{Ref: "PO-5-00440", WbsPath: "P-NF-ON-5", AccountCode: "PARTS", VendorRef: "V-OEM-03", PORef: "PO-00440", Amount: "38000.00", InvoicedToDate: "22150.00", Currency: "CAD", Status: "open", CreatedAt: d("2026-08-05")}},
		},
	}
}
