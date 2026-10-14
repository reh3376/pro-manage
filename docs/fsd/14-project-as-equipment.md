# 14: The Project as Equipment (Project TPM)

**Status: core concept, under active development.** This section states the idea, how pro_manage implements its first version, and the open questions to work through next. Other sections implement pieces of it; this is where the idea itself lives.

## Decisions on record

- **2026-10-14:** The owner's thesis is adopted as a core design concept of pro_manage: **treat the capital project itself as a piece of equipment, and manage it with TPM.** Its "losses" are delays, rework, interface clashes, and waiting on information and approvals, and they are measured, priced, and eliminated the way a plant eliminates equipment losses.
- **2026-10-14:** The concept is developed in this section and referenced from `§01`, `§03`, `§07`, `§08`, and `§09`. Novelty is not claimed publicly until a prior-art search is complete (see below).
- **2026-10-14:** A first prior-art search is complete (Related work, below). Finding: every element has prior art on its own, but no source found combines them. The public position is "builds on" named earlier work and claims only the combination, worded as "no prior source found", never "first" or "novel". The two strongest critiques (Kalsaas on throughput, Bølviken et al. on monetizing waste) are adopted as open design questions.

## The thesis

A TPM plant does not accept "the line was slow this week" as an explanation. It knows what share of planned time the line ran, how fast it ran against design, and how much of what it made was right the first time. It names every loss, prices it, ranks it, and assigns a team to the biggest one. Losses shrink because they are visible.

Capital projects almost never get this treatment. A project's health is usually reported as two variances (cost and schedule) that say *how far off* the project is but not *where the time and money went*. The days spent waiting on an RFI answer, the week a crew stood by because a work package started without its materials, the rework after a failed hold point, the month a change order sat in an approval queue: these are real, recurring, measurable losses, and they disappear into "schedule slip."

**The thesis: the project is the equipment.** Its work fronts are the machines, its work packages are the production runs, and its losses can be categorized, measured, priced, and attacked with the same discipline, and the same eight pillars, that TPM applies on the plant floor.

That gives pro_manage **two views of TPM on one system**:

| | TPM *for the asset* | TPM *of the project* |
|---|---|---|
| The "equipment" | The plant asset being built or modified | The project delivering it |
| Losses | Six Big Losses on the running asset (breakdowns, changeovers, minor stops, speed, rejects) | Waiting, rework, interface clashes, unready packages, safety stops, skill gaps |
| When it matters | Design (MP information) → Early Management → Benefits Realization | FEL → Execute → Closeout |
| Measured from | Plant systems via HSEi / Metrici / CMMSi | pro_manage's own records (RFIs, submittals, packages, ITPs, deviations) |
| Pillar most in play | Early Equipment Management | All eight, adapted to project delivery |
| Currency | Priced loss ($) | Priced loss ($), the same currency |

The shared currency matters. A dollar of project loss (a month of delay on a $40M expansion) and a dollar of asset loss (the downtime the expansion was meant to recover) can sit on the same Pareto, so a portfolio can see both "what our projects are losing" and "what our plants are losing" in one unit.

## The analogy, made concrete

### OEE → a project-effectiveness view

OEE multiplies three ratios. Each has a natural project counterpart, measured per work front or work package:

| OEE component | Plant meaning | Project analogue | Measured from |
|---|---|---|---|
| **Availability** | Run time ÷ planned production time | **Work-front availability**: time a released package was actually workable ÷ planned working time (lost to waiting on information, approvals, materials, permits, or safety stops) | Package status history, RFI/submittal aging, document register (planned vs actual IFC), procurement delivery float, deviations, HSE stops |
| **Performance** | Actual rate ÷ design rate | **Productivity**: earned quantities or hours ÷ planned for the time worked (lost to interface clashes, congestion, skill gaps) | Package progress vs plan, installed quantities |
| **Quality** | Good-first-time ÷ total | **First-time-right**: work accepted without rework ÷ work completed (hold points passed first time, submittals approved first time, no NCR) | ITP hold points, submittal dispositions, NCRs |

Whether these three should be multiplied into a single "project OEE" is an open question (below). The components are useful on their own and are what pro_manage measures first.

### Six Big Losses → six big project losses (proposed mapping)

| Plant loss | OEE component | Project loss | Typical source |
|---|---|---|---|
| Breakdowns | Availability | **Safety stops and major blockers**: work halted by an incident, stand-down, or regulatory stop | HSE records |
| Setup / changeover | Availability | **Unready packages**: work released or started without its prerequisites, then stalled or remobilized | Package readiness overrides, plan-compliance misses |
| Minor stops | Performance | **Waiting on information**: short, frequent interruptions for RFIs, missing drawings, clarifications | RFI aging, deviations |
| Reduced speed | Performance | **Interface clashes and skill gaps**: work proceeds, slower than planned | Productivity vs plan, deviation tags, competency gaps |
| Startup rejects | Quality | **Commissioning rework**: punchlist and failed CQV at the end | CQV records, punchlists |
| Production rejects | Quality | **Construction rework**: NCRs, failed hold points, revise-and-resubmit cycles | NCRs, hold points, submittals |

Plus one category with no plant equivalent but large project impact: **waiting on approval** (submittals, change orders, and the owner's own internal approvals). It is kept separate because the owner controls it directly; it is the project's Office-TPM loss. **Waiting on material** (a released package held by a late procurement item) is recorded as its own category in `§03` so that expediting failures are visible separately from information and approval delays; in this mapping it sits with unready packages, since it is a precondition miss.

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

1. **A single "project OEE" number?** Multiplying availability × productivity × first-time-right gives one comparable figure, which executives will want. It is also easy to game (redefine "planned time", reclassify rework). Options: publish components only; publish a composite only at portfolio level; or publish it with its definitions frozen per project at CAR time. Whatever is chosen, the three factors and the loss Pareto are always shown next to any composite, and the composite is never used to rank contractors.
2. **Unit of measurement.** Per work package, per work front, per contractor, per discipline? Probably packages roll up to contracts and projects, but this needs testing on real data.
3. **Pricing delay.** Cost of delay is easy for capacity projects (lost margin per day) and hard for compliance or reliability projects. A default method per project type is needed. Working rule: for an owner, delay is priced as the deferred contribution margin of the asset under construction (Reinertsen's cost of delay), and only for losses that actually reached the critical path; a local delay absorbed by float or a feeding buffer is recorded in hours but priced near zero.
4. **Benchmarks across projects.** With the same categories everywhere, MDEMG and the portfolio data could show "projects of this type and contracting model typically lose X% to waiting on approvals." How many closed projects before that is meaningful?
5. **Leading vs lagging.** Losses are lagging. Readiness, open-RFI age, and constraint counts are leading. Which leading indicators actually predict next month's losses?
6. **Contractor incentives.** If losses are attributed per contract, does that improve behavior or produce argument over attribution? EPCM contracts may need to reference the categories.
7. **Validation.** Before claiming results, run the loss accounting retrospectively on two or three completed projects (from their RFI logs, NCRs, and schedules) and check whether the categories capture where the time actually went. A worked example from one real project is also the strongest answer to the critiques in items 8 and 9.
8. **Defining the performance term (Kalsaas's critique).** The researcher who came closest to a project OEE concluded that construction workflow should be measured by work intensity, not throughput, because construction output has no natural ideal cycle time. The productivity factor therefore needs an explicit definition, for example earned hours against available hours on ready work fronts, and an argument for why it avoids that problem. The engineering phase of EPCM, where no OEE precedent exists at all, is where the index is most likely to fail and most worth piloting carefully.
9. **Dollars as a ranking layer, not the record (Bølviken, Rooke and Koskela's critique).** Their waste taxonomy deliberately avoids money because monetizing hides production detail. Design answer: the primary record is physical (hours, days, events, by category and package); dollars are a prioritization layer computed from it with an explicit, versioned pricing rule (item 3). Dashboards always allow drilling from a dollar figure back to the events behind it.
10. **Commercial and legal exposure.** Construction's richest loss-factor lists (MCAA factors, measured-mile methods) were built to price contractor damages in disputes. An owner that prices losses caused by its own approval delays is, in effect, documenting a contractor's delay claim. Before go-live, agree with legal and commercial on record retention, who sees owner-caused loss figures, and how they relate to contract notice and claims clauses.

## Related work and prior art

Project TPM builds directly on existing work: Nakajima's TPM and the Six Big Losses; JIPM's Office TPM and Early Equipment Management; Koskela's flow view of construction and his "making-do" waste; Ballard's Last Planner System; the transformation-flow-value waste taxonomy (Bølviken, Rooke and Koskela); CII Advanced Work Packaging; and Kalsaas's adaptation of OEE loss groups to construction workflow. What pro_manage proposes is the **combination**. A first, limited literature, product, and patent search (October 2026) found no prior source that presents it; the search was not exhaustive, and references to earlier work are welcome.

### Element-by-element comparison

Closeness: **High** = an existing source does substantially the same thing; **Moderate** = the same idea at a different level or without a key feature; **Low** = only adjacent ideas; **None found** = nothing comparable surfaced.

| Element | Closest prior art | How close | What differs |
|---|---|---|---|
| OEE-style project index (work-front availability × productivity × first-time-right) | Kalsaas, IGLC 2012 and 2013 (OEE downtime, speed, and defect loss groups applied to site workflow); AWP constraint-free work packages; Overall Labor Effectiveness | Moderate | Kalsaas worked at crew level from worker self-reports and never multiplied the factors into one index. AWP gates availability but computes no ratio. OLE is a manufacturing workforce metric. No precedent found for OEE in engineering or knowledge work. |
| "Six big project losses" plus waiting on approval | Last Planner reasons for non-completion (Ballard 2000); Koskela's making-do; TFV waste taxonomy; Morgan and Liker's product-development wastes; JIPM "management loss" | High on content, low on form | The categories are well documented, and Last Planner already puts them on a Pareto, counted in occurrences rather than dollars. None of these taxonomies descends from the Six Big Losses, and none treats waiting on approval as a top-level project loss. |
| Eight TPM pillars applied to EPCM delivery | Scattered lean analogues (root-cause loops, make-ready, coaching); the EEM pillar itself | None found | "TPM in construction" literature is about maintaining construction machines, not the project. |
| Project and plant losses in one dollar currency on one Pareto | IPA's front-end loading and early-operability studies; CII PDRI correlations; rework cost studies (CII Field Rework Index, Hwang et al.); Reinertsen's cost of delay | Low | IPA and CII link project practice to operating results through portfolio statistics, reported as separate indices. Rework is priced on its own. No owner system found ranks individual project and plant loss events in one ledger. |
| Project-loss lessons fed forward like MP information | EEM / MP information (plant losses into the next design); CII constructability programs and lessons-learned research (RT-230) | Moderate | EEM carries *plant* losses into projects; Project TPM carries the project's *own* delivery losses forward. Existing lessons-learned programs are mostly free text at close-out, not quantified, priced loss categories. |
| Automatic capture from normal project records | Procore RFI analytics; Autodesk Construction IQ; Touchplan / Outbuild variance and constraint logs | High for capture, low for loss accounting | The data is already captured routinely; converting it into lost hours and dollars by category is what is missing. |
| One dominant loss attacked per health review | TPM focused improvement; Last Planner Pareto with 5 Whys; Critical Chain buffer-cause ranking | Moderate | Last Planner works weekly at site level on counted reasons; Critical Chain ranks causes in days within one project. Neither selects a dollar-dominant loss at an owner-level review. |

The pattern: the closer an element sits to day-to-day site production control, the more prior art it has; the closer it sits to owner-level economics and TPM's management system, the less. Project TPM is therefore best positioned as an **owner-level economic layer over Last Planner and AWP**, consuming their records rather than replacing them.

### Confidence

- **High** that the loss categories, automatic capture of RFI and constraint data, and lessons-learned feed-forward are not new in themselves.
- **Moderate** that no published English-language source presents a multiplied project OEE, a "six big project losses" grouping, or an eight-pillar project framework.
- **Low to moderate** that no one has combined project and plant losses in one dollar ledger, because benchmarking databases (IPA) and owner-internal systems are not public.
- **Low** for JIPM's own texts, Japanese-language TPM literature, non-English patents, and paywalled journals, which the search did not reach.

Patentability is a separate question from originality: applying a public equipment-loss structure to a project could be argued as obvious, so the concept is positioned as a management contribution, not an invention.

### Remaining checks

1. Google Scholar and Scopus queries combining "overall equipment effectiveness", "total productive maintenance", and "six big losses" with "construction project", "Last Planner", "capital project", "EPC", and "EPCM".
2. Forward citations of Kalsaas 2012 and 2013 (anyone who built a project OEE most likely cited them).
3. Two unread papers that could already price or index project losses: a 2021 *Production Planning & Control* paper on waste in engineer-to-order projects, and Lagos et al. (IGLC 2020) on the impact of Last Planner reasons for non-compliance.
4. JIPM primary texts and a Japanese-language search.
5. A patent search by classification (CPC G06Q 10/06, G06Q 50/08), including non-English patents.

### Key sources

- Ballard, G. (2000). *The Last Planner System of Production Control.* PhD thesis, University of Birmingham.
- Ballard, G. and Tommelein, I. (2016). *Current Process Benchmark for the Last Planner System.* P2SL, UC Berkeley.
- Bølviken, T., Rooke, J., and Koskela, L. (2014). "The wastes of production in construction: a TFV based taxonomy." IGLC-22.
- Construction Industry Institute. Advanced Work Packaging (RT-272, RT-319); *Lessons Learned About Lessons Learned* (RT-230).
- Kalsaas, B. T. (2012, 2013). Papers on measuring workflow in construction using OEE loss groups. IGLC-20 and IGLC-21 (iglc.net).
- Koskela, L. (1992). *Application of the New Production Philosophy to Construction.* CIFE Technical Report 72, Stanford.
- Koskela, L. (2004). "Making-do: the eighth category of waste." IGLC-12.
- Nakajima, S. (1988). *Introduction to TPM.* Productivity Press.
- Reinertsen, D. (2009). *The Principles of Product Development Flow.* Celeritas.

## Related sections

- `§01`: decision on the two views of TPM
- `§03`: `WorkPackage`, `RFI`, `Submittal`, `ChangeOrder`, `InspectionTestPlan`, `ProjectDeviation`, `ProjectLossEvent`
- `§06`: project-loss learnings in the governance space
- `§07`: project-loss metric family
- `§08`: EPCM workflows and dominant-loss selection in health reviews
- `§09`: Project reliability dashboard
