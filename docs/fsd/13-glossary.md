# 13 — Glossary

Terms, acronyms, and project-specific names.

## pro_manage terms

| Term | Meaning |
|---|---|
| **pro_manage** | This application. Capital-project management + data curation + metric tracking for a single company. |
| **pm-core** | The Go backend monolith. Serves REST + gRPC + SSE. |
| **pm-sidecar** | The Python sidecar for render/xlsx/derived-metric work. |
| **pm-workers** | Go background jobs (scheduler, outbox drain, nudges, SLA tick). |
| **pm-frontend** | The Next.js SSR+CSR frontend for authenticated users. |
| **pm-frontend-public** | The hardened Next.js renderer for tokenized dashboard shares. |
| **pm-TSDB** | pro_manage's own TimescaleDB instance for metric readings. Distinct from MDEMG's own TSDB. |
| **Project** | The central aggregate. One Owner. Full ACL for team members. Lifecycle: Initiate → Plan → Execute → Monitor → Close → Archived. |
| **Portfolio** | Group of projects visible as a unit to Portfolio Executives. |
| **ACLGrant** | Fine-grained grant on top of a ProjectMember's base role. `scope_kind ∈ {section, document, field}`. |
| **Section** | A top-level division of a project page (budget, schedule, scope, risk, docs, metrics, approvals, audit, governance, settings, team). Each has a permission bucket. |
| **MetricDefinition / MetricReading** | A metric's definition (what it means) vs its measured values over time. |
| **ApprovalWorkflow / ApprovalStage** | A multi-stage approval instance and its individual gates. |
| **AuditEvent** | Append-only, chained-hash row recording a state-changing action. |
| **GuidanceOutcome** | pro_manage-side record of MDEMG guidance presented + user response. Written alongside AuditEvent. |

## Connector modules

| Term | Meaning |
|---|---|
| **ERPi** | ERP integration connector. Supports SAP, Oracle, NetSuite, Dynamics, Odoo, etc. |
| **IdPi** | Identity provider (SSO) connector. OIDC + SAML abstractions. |
| **Sigi** | E-signature connector. DocuSign at launch; Adobe Sign, OneSpan, etc. in reach. |
| **Schedulei** | Schedule-tool connector. MS Project, Primavera P6, Smartsheet. |
| **Doci** | Document-store connector with permission mirroring. SharePoint, Drive, Confluence. |
| **Commsi** | Notification / communication connector. SMTP, Teams, Slack. |
| **Metrici** | Metric harvest connector. Relational, time-series, historian, OPC UA. |
| **HSEi** | MES/WMS/QMS / HSE system connector for safety, quality, compliance signals. |

## MDEMG terms

| Term | Meaning |
|---|---|
| **MDEMG** | Multi-Dimensional Emergent Memory Graph. The memory and advisory substrate pro_manage integrates with. |
| **J17** | AI2AI protocol for carrying trust-aware, tier-encoded guidance between an actor and MDEMG. In pro_manage, pm-core is the actor. |
| **Jiminy** | MDEMG's guidance engine. Produces `/guide` responses, enforces `/classify` verdicts, tracks escalation. |
| **Jiminy constraint** | A rule in the MDEMG graph, authored by Governance Authors. Has type (`must | must_not | should | note`), scope, severity, roles. |
| **Space** | MDEMG's tenant / partition tag. In pro_manage, `space=project_id` for day-to-day; `space=org.<id>.governance` for policy library. |
| **Session** | MDEMG session identity. In pro_manage, `session_id = <user_id>:<project_id>`. |
| **Trust (EMA)** | Jiminy's per-(session, constraint) trust score. We use EMA mode (default). |
| **Escalation ladder** | Jiminy's per-(session, constraint) state machine: `surfaced → warned → escalated → blocked`. For humans in pro_manage: `warn=1, escalate=2, block=3`. |
| **Fail-closed / fail-open tier** | pro_manage's edge policy for how to behave when MDEMG classify errors. Tier A = refuse; Tier B = proceed with warning; Tier C = bypass. |
| **CMS** | Conversation Memory Store inside MDEMG. The write path at `/v1/conversation/observe`. |
| **obs_type** | The closed enum of observation kinds pro_manage writes via CMS (`decision, correction, learning, preference, error, task, technical_note, insight, context, progress, blocker, context_signal, note, constraint, self_improvement`). |

## UxTS terms

| Term | Meaning |
|---|---|
| **UxTS** | Universal-x Test Specification. Methodology for declarative test governance (schema + specs + runner + CI gate). |
| **UATS** | Universal API Test Specification. Governs REST endpoints. MVP. |
| **UDTS** | Universal (gRPC Data) Test Specification. Governs gRPC contracts. MVP. |
| **UNTS** | Universal Hash Test Specification. Hash integrity on spec + fixture files. MVP. |
| **UBTS** | Universal Benchmark Test Specification. Phase 2. |
| **USTS** | Universal Security Test Specification. Phase 2. |
| **UOTS** | Universal Operations Test Specification (manifests, charts). Phase 2+. |
| **UPMTS** | Universal Project-Metric Test Specification. pro_manage custom framework for derived-metric contracts. Phase 2. |

## Capital project terms

| Term | Meaning |
|---|---|
| **CAR** | Capital Authorization Request. The formal document that authorizes capital spend; routes through pro_manage's multi-stage approval workflow. |
| **CapEx** | Capital expenditure. Generic framing; CAR is the specific document kind in a Heaven Hill-style shop. |
| **FEL** | Front-End Loading. Pre-authorization planning. **FEL1** = conceptual (business case + rough scope). **FEL2** = scoping/development (basis of design + preliminary estimate). **FEL3** = execution-ready (defined scope + Class 3 estimate + ready for CAR). |
| **AACE Estimate Class** | AACE International's cost-estimate maturity scale. **Class 5** = concept, −50/+100%. **Class 4** = study, −30/+50%. **Class 3** = budget authorization, −20/+30%. **Class 2** = control, −15/+20%. **Class 1** = definitive/bid-ready, −10/+15%. |
| **TIC** | Total Installed Cost. All capital cost to install the asset (equipment + engineering + construction + commissioning). |
| **LCC** | Lifecycle Cost. Total cost of ownership over the asset's expected life (TIC + operation + maintenance + end-of-life). |
| **ROI (per project)** | Return on Investment. Calculated three times per project: in the business case (forecast ROI that gets the CAR approved), at closeout (actuals vs forecast cost), and at benefits realization (actuals vs forecast benefit). |
| **NPV / IRR** | Net Present Value / Internal Rate of Return. Discounted cash-flow measures of project value. |
| **Payback Period** | Time for cumulative savings or cash flow to repay the investment. |
| **CPI / SPI** | Cost / Schedule Performance Index. Earned-value metrics. CPI = BCWP / ACWP; SPI = BCWP / BCWS. Below 1.0 is bad. |
| **BCWS / BCWP / ACWP** | Budgeted Cost of Work Scheduled / Performed; Actual Cost of Work Performed. Earned-value inputs. |
| **EAC** | Estimate at Completion. Forecast of total project cost. |
| **EVM** | Earned Value Management. Framework tying scope, cost, and schedule through BCWP. |
| **Contingency drawdown** | Rate at which contingency is being used relative to percent-complete. Burning faster than progress is an early warning. |
| **Benefits Realization** | Confirming — typically 6–12 months post-startup — that the savings or capacity promised in the business case actually materialized. The third ROI calculation. |
| **EPC / EPCM** | Engineer-Procure-Construct (lump-sum delivery by a single contractor) / Engineering, Procurement, Construction Management (owner retains PM; contractor manages execution). Contracting models. |
| **WBS** | Work Breakdown Structure. Hierarchical decomposition of project scope. |
| **Change request (CR)** | A request to change scope, with declared budget and schedule impact; routes through an approval workflow. |
| **Closeout** | Final project lifecycle gate. Confirms deliverables accepted, retention armed, Benefits Realization window begins. |
| **PSC** | Project Success Criteria. Declared targets a project must meet to count as successful. |
| **SLA** | Service-Level Agreement on an approval stage's response time. |
| **OOO** | Out-of-Office. Approvers can mark themselves OOO; routing prefers non-OOO assignees. |

## TPM / EEM terms

| Term | Meaning |
|---|---|
| **TPM** | Total Productive Maintenance. Discipline for maximizing equipment effectiveness through operator engagement, autonomous maintenance, planned maintenance, focused improvement, early equipment management, and five other pillars. |
| **TPM pillars** | The eight canonical pillars: Autonomous Maintenance, Planned Maintenance, Focused Improvement, Early Equipment Management (EEM), Quality Maintenance, Training & Education, Office TPM, Safety/Health/Environment. |
| **EEM** | Early Equipment Management. The TPM pillar for new equipment and new lines. Goal: **vertical startup** — reaching target performance immediately at startup rather than after months of debugging. |
| **EEM stages** | Six stages, each with a gate and exit criteria: 1) Concept/Planning, 2) Design (basic → detailed), 3) Fabrication/Build (incl. FAT), 4) Installation (incl. SAT), 5) Commissioning/Trial Operation, 6) Early Management Period (initial flow control until exit criteria met). |
| **MP information** | Maintenance Prevention information. Failures, losses, difficulties, and operability issues harvested from running equipment that get fed into the design of the next asset. The core input to EEM. |
| **"Easy-to" criteria** | The seven design-review lenses of EEM: easy to operate, maintain, clean, change over, inspect, keep safe, and make quality product. |
| **Vertical startup** | New equipment hitting target performance (rate, quality, OEE) immediately at startup. Measured by the startup curve. |
| **Startup curve** | Daily production rate (and quality) as a percentage of nameplate, from Day 1 of production forward. The direct scorecard for EEM. |
| **Time to Nameplate** | Days or weeks from first production to sustained target rate. |
| **Early Management Period** | Formal post-startup phase with extra monitoring. Project owns the asset until declared exit criteria (sustained OEE, quality holds) are met, then formal handover to normal operations. |
| **Turnover Package** | The structured handover artifact at Early Management exit: equipment master data, spares, PM tasks, procedures, cleaning procedures, calibration certs, standard work. Must be writable to the plant CMMS. |
| **CQV** | Commissioning, Qualification, Validation. The pre-startup discipline for proving the asset works as designed. More formal in regulated industries (FDA/pharma); still practiced as FAT/SAT + commissioning records in distilling and food & beverage. |
| **FAT / SAT** | Factory Acceptance Test (at the vendor's shop before shipping) / Site Acceptance Test (in the plant after installation). |
| **Autonomous Maintenance (AM)** | TPM pillar: operators perform routine maintenance (cleaning, inspection, lubrication) on their own equipment. |
| **Planned Maintenance (PM)** | TPM pillar: scheduled time-based or condition-based maintenance tasks executed by the maintenance team. Not to be confused with Project Manager. |
| **Focused Improvement (FI)** | TPM pillar: targeted problem-solving on specific losses using tools like loss trees, 5-Whys, PDCA. |
| **Loss tree** | Taxonomy of production losses (planned downtime, breakdowns, setups, minor stops, speed losses, defects, startup losses) used to prioritize improvement activity. |
| **Standard work** | Documented, agreed-on way to perform a task repeatably. |
| **5S** | Workplace organization method: Sort, Set-in-order, Shine, Standardize, Sustain. |
| **SMED** | Single-Minute Exchange of Die. Reducing changeover time via distinction between internal and external setup elements. |

## Plant Operations KPIs

| Term | Meaning |
|---|---|
| **OEE** | Overall Equipment Effectiveness. **Availability × Performance × Quality.** World-class is often quoted around 85%. |
| **Availability** | Run time ÷ planned production time. The OEE component most affected by breakdowns and long changeovers. |
| **Performance (rate)** | Speed versus design speed while running: (ideal cycle time × total count) ÷ run time. |
| **Quality (OEE component)** | Good units ÷ total units produced during the period. |
| **TEEP** | Total Effective Equipment Performance. OEE × Loading (scheduled time ÷ total calendar time). Shows hidden capacity. |
| **FPY / RFT** | First Pass Yield / Right First Time. Share of product that is good the first time, no rework. |
| **MTBF** | Mean Time Between Failures. Total operating time ÷ number of failures. Reliability measure for repairable assets. |
| **MTTR** | Mean Time To Repair. Total repair downtime ÷ number of repairs. Maintainability measure. |
| **MTTF** | Mean Time To Failure. Average life of a non-repairable component. Sets spare-parts strategy. |
| **Capacity Utilization** | Actual output ÷ maximum possible output. Key justification for new-capacity capital. |
| **Schedule Adherence / Production Attainment** | Whether the plant made what it planned, when it planned. |
| **Cycle Time** | Actual time to complete one unit or batch, versus engineering standard. |
| **Takt Time** | Pace needed to meet customer demand: available production time ÷ demand. New equipment sizing: cycle time must beat takt time with margin. |
| **CPU** | Cost Per Unit. Total production cost ÷ units produced. |
| **COPQ** | Cost of Poor Quality. Scrap + rework + reblend + returns + inspection + downgrades. |
| **EPU** | Energy per Unit. Energy used ÷ output (e.g., therms of natural gas per proof gallon). |
| **OLE** | Overall Labor Effectiveness. Labor availability × labor performance × quality. Labor counterpart to OEE. |
| **Changeover Time** | Time from last good unit of one product to first good unit of the next. SMED projects attack this. |
| **Planned Maintenance Percentage** | Planned maintenance hours ÷ total maintenance hours. Healthy plant: ~80%+. |
| **Startup Curve** | Daily rate as a % of nameplate from Day 1 forward. Scorecard for EEM success. |

## Plant systems and automation

| Term | Meaning |
|---|---|
| **CMMS** | Computerized Maintenance Management System. The system of record for maintenance work orders, PMs, spares, and asset history. pro_manage integrates via CMMSi for MP-information read and turnover-package write. |
| **MES** | Manufacturing Execution System. Operational-layer software managing production execution, often sitting between the plant floor and ERP. pro_manage reads signals via HSEi. |
| **SCADA** | Supervisory Control and Data Acquisition. Operator-facing supervisory layer over PLCs. |
| **PLC** | Programmable Logic Controller. Real-time control of equipment. |
| **HMI** | Human-Machine Interface. The operator screen; a surface of SCADA. |
| **Historian** | Time-series database for process signals (OSIsoft PI, Honeywell PHD, Aveva Wonderware, etc.). pro_manage pulls via Metrici. |
| **OPC UA** | Open protocol for industrial automation data exchange. |
| **P&ID** | Piping and Instrumentation Diagram. Design document for a process plant. |
| **MCC** | Motor Control Center. Electrical equipment cabinet housing starters for multiple motors. |
| **LOTO** | Lockout/Tagout. Safety procedure for isolating energy sources during maintenance. |
| **Clean-in-Place (CIP)** | Cleaning method for process equipment without disassembly. |

## Codes and regulatory

| Term | Meaning |
|---|---|
| **NEC** | National Electrical Code. |
| **NFPA 70E** | National Fire Protection Association standard for electrical safety in the workplace (arc flash). |
| **NFPA 30** | NFPA standard for flammable and combustible liquids (relevant to distilling). |
| **NFPA 652 / 61** | NFPA combustible-dust standards (relevant to grain handling). |
| **UL 508A** | Underwriters Laboratories standard for industrial control panels. |
| **Hazardous area classification** | Classifying areas by presence of flammable gases/vapors (Class I Div 1/2) or combustible dusts (Class II). Shapes equipment selection and electrical design. |
| **TTB** | Alcohol and Tobacco Tax and Trade Bureau. US regulator of distilled spirits. |
| **FDA / EPA / OSHA / MSHA** | Food and Drug Administration / Environmental Protection Agency / Occupational Safety and Health Administration / Mine Safety and Health Administration. |
| **MSHA** | Mine Safety and Health Administration (relevant to mining-industry work in author's background). |

## Infrastructure + architecture terms

| Term | Meaning |
|---|---|
| **SSO** | Single Sign-On. |
| **OIDC** | OpenID Connect, the standard pm-core prefers for interactive auth. |
| **SAML** | Security Assertion Markup Language, the alternate IdP protocol. |
| **MFA** | Multi-Factor Authentication. |
| **Vault** | The deployment's secret store (HashiCorp Vault, cloud KMS, on-prem equivalent). |
| **Outbox pattern** | Durable at-least-once dispatch of side-effects (MDEMG writes, notifications) via a Postgres queue drained by pm-workers. |
| **SSE** | Server-Sent Events. Used for live dashboard updates. |
| **RED metrics** | Rate, Errors, Duration — the standard service-health trio. |
| **RTO / RPO** | Recovery Time Objective / Recovery Point Objective. |
| **ISO 9001 / 55000 / 27001** | Quality / asset management / information security standards. |
| **SOX** | Sarbanes-Oxley. Audit discipline baseline, even without an SOX obligation. |

## Compliance + audit terms

| Term | Meaning |
|---|---|
| **Chain of custody** | For pro_manage: the audit chain from action → AuditEvent → GuidanceOutcome → MDEMG-side correlation. |
| **Legal hold** | Per-project retention freeze. No deletion or downsampling until released. |
| **Attestation** | A user acknowledging terms before viewing a tokenized dashboard. |
