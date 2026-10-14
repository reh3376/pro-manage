# 14 — The Project as Equipment (Project TPM)

**Status: core concept, under active development.** This section states the idea, how pro_manage implements its first version, and the open questions to work through next. Other sections implement pieces of it; this is where the idea itself lives.

## Decisions on record

- **2026-10-14** — The owner's thesis is adopted as a core design concept of pro_manage: **treat the capital project itself as a piece of equipment, and manage it with TPM.** Its "losses" are delays, rework, interface clashes, and waiting on information and approvals, and they are measured, priced, and eliminated the way a plant eliminates equipment losses.
- **2026-10-14** — The concept is developed in this section and referenced from `§01`, `§03`, `§07`, `§08`, and `§09`. Novelty is not claimed publicly until a prior-art search is complete (see below).

## The thesis

A TPM plant does not accept "the line was slow this week" as an explanation. It knows what share of planned time the line ran, how fast it ran against design, and how much of what it made was right the first time. It names every loss, prices it, ranks it, and assigns a team to the biggest one. Losses shrink because they are visible.

Capital projects almost never get this treatment. A project's health is usually reported as two variances — cost and schedule — that say *how far off* the project is but not *where the time and money went*. The days spent waiting on an RFI answer, the week a crew stood by because a work package started without its materials, the rework after a failed hold point, the month a change order sat in an approval queue: these are real, recurring, measurable losses, and they disappear into "schedule slip."

**The thesis: the project is the equipment.** Its work fronts are the machines, its work packages are the production runs, and its losses can be categorized, measured, priced, and attacked with the same discipline — and the same eight pillars — that TPM applies on the plant floor.

That gives pro_manage **two views of TPM on one system**:

| | TPM *for the asset* | TPM *of the project* |
|---|---|---|
| The "equipment" | The plant asset being built or modified | The project delivering it |
| Losses | Six Big Losses on the running asset (breakdowns, changeovers, minor stops, speed, rejects) | Waiting, rework, interface clashes, unready packages, safety stops, skill gaps |
| When it matters | Design (MP information) → Early Management → Benefits Realization | FEL → Execute → Closeout |
| Measured from | Plant systems via HSEi / Metrici / CMMSi | pro_manage's own records (RFIs, submittals, packages, ITPs, deviations) |
| Pillar most in play | Early Equipment Management | All eight, adapted to project delivery |
| Currency | Priced loss ($) | Priced loss ($) — the same currency |

The shared currency matters. A dollar of project loss (a month of delay on a $40M expansion) and a dollar of asset loss (the downtime the expansion was meant to recover) can sit on the same Pareto, so a portfolio can see both "what our projects are losing" and "what our plants are losing" in one unit.

## The analogy, made concrete

### OEE → a project-effectiveness view

OEE multiplies three ratios. Each has a natural project counterpart, measured per work front or work package:

| OEE component | Plant meaning | Project analogue | Measured from |
|---|---|---|---|
| **Availability** | Run time ÷ planned production time | **Work-front availability**: time a released package was actually workable ÷ planned working time (lost to waiting on information, approvals, materials, permits, or safety stops) | Package status history, RFI/submittal aging, deviations, HSE stops |
| **Performance** | Actual rate ÷ design rate | **Productivity**: earned quantities or hours ÷ planned for the time worked (lost to interface clashes, congestion, skill gaps) | Package progress vs plan, installed quantities |
| **Quality** | Good-first-time ÷ total | **First-time-right**: work accepted without rework ÷ work completed (hold points passed first time, submittals approved first time, no NCR) | ITP hold points, submittal dispositions, NCRs |

Whether these three should be multiplied into a single "project OEE" is an open question (below). The components are useful on their own and are what pro_manage measures first.

### Six Big Losses → six big project losses (proposed mapping)

| Plant loss | OEE component | Project loss | Typical source |
|---|---|---|---|
| Breakdowns | Availability | **Safety stops and major blockers** — work halted by an incident, stand-down, or regulatory stop | HSE records |
| Setup / changeover | Availability | **Unready packages** — work released or started without its prerequisites, then stalled or remobilized | Package readiness overrides, plan-compliance misses |
| Minor stops | Performance | **Waiting on information** — short, frequent interruptions for RFIs, missing drawings, clarifications | RFI aging, deviations |
| Reduced speed | Performance | **Interface clashes and skill gaps** — work proceeds, slower than planned | Productivity vs plan, deviation tags, competency gaps |
| Startup rejects | Quality | **Commissioning rework** — punchlist and failed CQV at the end | CQV records, punchlists |
| Production rejects | Quality | **Construction rework** — NCRs, failed hold points, revise-and-resubmit cycles | NCRs, hold points, submittals |

Plus one category with no plant equivalent but large project impact: **waiting on approval** (submittals, change orders, and the owner's own internal approvals). It is kept separate because the owner controls it directly — it is the project's Office-TPM loss.

The FSD's working category set (`§03 ProjectLossCategory`) uses these; the mapping above is a proposal to be tested against real project data.

### The eight pillars, applied to the project

| TPM pillar | Applied to the project | Where pro_manage implements it |
|---|---|---|
| Autonomous Maintenance | People closest to the work own early detection: walkdowns, deviation tags, hold-point checks | `ProjectDeviation`, field flows (`§03`, `§09`) |
| Planned Maintenance | Work packaged and released only when ready; plan compliance tracked with reasons | `WorkPackage` readiness, plan compliance (`§03`, `§08`) |
| Focused Improvement | Pick the dominant project loss from data; root-cause it; standardize the fix | Health review's dominant-loss selection (`§08`) |
| Quality Maintenance | Conditions that prevent defects: ITPs, hold points, NCR causes fed back into specs | `InspectionTestPlan`, `HoldPoint`, quality-pattern lessons (`§03`, `§06`) |
| Early Equipment Management | Constructability, commissionability, operability, and safety designed in | Design-review lenses (`§03`, `§08`) |
| Training & Education | Competency of owner, EPCM, and contractor staff matched to package needs | `TeamCompetency` (Phase 3) |
| Safety, Health & Environment | HSE prerequisites as package-readiness constraints; leading indicators per package | Readiness constraints, `hse.leading_indicators` (`§07`) |
| Office TPM | Cycle times for RFIs, submittals, change orders, and internal approvals | `admin.*` metrics, `waiting_approval` loss (`§07`) |

## How pro_manage implements version 1

1. **Capture losses as a by-product of normal work.** No one fills in a loss log. RFI aging, submittal cycles, package readiness overrides, plan-compliance misses with their reason codes, NCRs, failed hold points, deviations, and HSE stops each generate `ProjectLossEvent`s automatically (`§03`).
2. **Price them where possible.** Rework has a cost; delay is priced at the project's cost-of-delay rate where Finance has set one (`§07`).
3. **Make them visible.** The **Project reliability** dashboard is the project's OEE board: loss Pareto, package readiness, plan compliance, administrative aging (`§09`).
4. **Act on the biggest one.** Each project health review selects one dominant loss from the Pareto, assigns an owner and a countermeasure, and checks next cycle whether it shrank (`§08`).
5. **Carry the lesson forward.** Recurring project-loss patterns at closeout become governance-space learnings in MDEMG, scoped by asset class, package kind, and contracting model, so the next project's design review and FEL see them (`§06`). This is MP information for projects.

## Open questions to develop

1. **A single "project OEE" number?** Multiplying availability × productivity × first-time-right gives one comparable figure, which executives will want. It is also easy to game (redefine "planned time", reclassify rework). Options: publish components only; publish a composite only at portfolio level; or publish it with its definitions frozen per project at CAR time.
2. **Unit of measurement.** Per work package, per work front, per contractor, per discipline? Probably packages roll up to contracts and projects, but this needs testing on real data.
3. **Pricing delay.** Cost of delay is easy for capacity projects (lost margin per day) and hard for compliance or reliability projects. A default method per project type is needed.
4. **Benchmarks across projects.** With the same categories everywhere, MDEMG and the portfolio data could show "projects of this type and contracting model typically lose X% to waiting on approvals." How many closed projects before that is meaningful?
5. **Leading vs lagging.** Losses are lagging. Readiness, open-RFI age, and constraint counts are leading. Which leading indicators actually predict next month's losses?
6. **Contractor incentives.** If losses are attributed per contract, does that improve behavior or produce argument over attribution? EPCM contracts may need to reference the categories.
7. **Validation.** Before claiming results, run the loss accounting retrospectively on two or three completed projects (from their RFI logs, NCRs, and schedules) and check whether the categories capture where the time actually went.

## Prior art to check before claiming novelty

These overlap parts of the idea. A proper search should confirm what is genuinely new in the combination.

- **Lean Construction / Last Planner System** (Ballard, Lean Construction Institute): percent plan complete, variance reasons, and make-ready constraint analysis — close to package readiness and plan compliance.
- **Koskela's transformation-flow-value view of production in construction** (1992 onward): waiting and rework as flow losses.
- **Construction Industry Institute — Advanced Work Packaging** (RT-272, RT-319): readiness-driven package release.
- **OEE applied to construction equipment or to knowledge work** (assorted academic papers): may apply OEE math to machines on site or to engineering teams, rather than to the project as a whole.
- **Earned Value Management and Earned Schedule**: measure *how much* is off, not loss categories — the gap this concept fills.

What appears distinctive, pending the search: (a) treating the *whole project* as the equipment within the TPM pillar structure, (b) an OEE-style loss taxonomy for project delivery, (c) pricing project losses and asset losses in **one currency** in one system, and (d) feeding project-loss lessons forward the way EEM feeds MP information forward.

## Related sections

- `§01` — decision on the two views of TPM
- `§03` — `WorkPackage`, `RFI`, `Submittal`, `ChangeOrder`, `InspectionTestPlan`, `ProjectDeviation`, `ProjectLossEvent`
- `§06` — project-loss learnings in the governance space
- `§07` — project-loss metric family
- `§08` — EPCM workflows and dominant-loss selection in health reviews
- `§09` — Project reliability dashboard
