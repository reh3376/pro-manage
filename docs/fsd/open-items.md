# Owner Decision Checklist (consolidated open items)

Status: **Living list** · Last updated: 2026-10-16

Every FSD section ends with an "Open items" block. This page collects the ones that need the owner, the company, or the MDEMG maintainer to answer, grouped by **what each answer unblocks**, so they can be worked in one sitting. The section blocks remain the detailed record; when an item is answered here, the answer is folded into the section and the row is marked done.

How to use: fill the **Answer** column (or reply in conversation with the item id). Items marked *default* have a proposed answer already in the FSD; silence at FSD ratification accepts the default.

## A. Gates Phase 0 (starting the spine)

| Id | Question | Options / default | Unblocks | Ref | Answer |
|---|---|---|---|---|---|
| OI-01 | Which delivery scenario: **A** owner plus AI collaborator (stages, no dates) or **B** small team (52-week phases)? | A or B; for B, start date and team shape | The clock on everything | roadmap | **A** (2026-10-20); re-assess at end of spine |
| OI-02 | Which host shape will IT operate for `pm-mdemg-01`: an Apple Silicon host as MDEMG packages it, or a Linux VM with GPU pass-through? Can the server network hold it? | Apple Silicon (least divergence from MDEMG's tested setup) or GPU VM (what IT usually prefers; needs a Phase 0 load test) | Phase 0 exit (measured classify p50/p95); the `§11` classify SLO rows | §06, §11 | |
| OI-03 | MDEMG version pin and model pin for MVP (`mdemg-llm-v1` Q5_K_M and its manifest SHA, or newer)? | *default:* latest stable MDEMG at ratification; `mdemg-llm-v1` Q5_K_M | Phase 0 dev profile; adapter UATS fixtures; ULTS hash record | §06, roadmap | **Dev:** `v0.11.0-beta.3` (commit `c1f14f74`), dedicated Compose stack, `apikey` auth (2026-10-20). Model pin and manifest SHA still to record |
| OI-04 | Vault technology for secrets? | HashiCorp Vault, cloud KMS, or the company's on-prem equivalent | Phase 0 (`§05`, `§11` secrets) | roadmap | **Dev:** `.env` files behind a provider interface (2026-10-20). Production tech still open |
| OI-05 | (MDEMG maintainer) Does `/v1/jiminy/classify` have a non-LLM fast path for the "no applicable constraint" case? Is there a server-side cap on classify time below 10 s? | Answer from the MDEMG code | Classify budget and pre-classification design | §06 | |
| OI-06 | (MDEMG maintainer) Does `annotations.roles` round-trip reliably through MDEMG retrieval? | Yes, or build the pm-core side-table fallback from day one | Role-filtered guidance | §06 | |
| OI-07 | Token budget for the `context` and `agent_output` summaries pm-core sends to MDEMG? | *default:* 2 000 tokens | Adapter payload shaping | §06 | |
| OI-08 | Company's existing log aggregator and metric platform (reuse vs ship Loki/Prometheus)? | Loki, Splunk, Elastic; Prometheus, Datadog | Phase 0 observability stack | §11 | |
| OI-09 | UxTS: vendored runner pins and how the CI stack seeds MDEMG fixtures (UATS `setup` is unimplemented upstream)? | *default:* runners at MDEMG's tag at ratification; a fixture-load step before `validate-all` | Phase 0 UxTS scaffold | §12 | **Vendored at `c1f14f74`; stub MDEMG in PR CI, live UATS locally; fixture-load step** (2026-10-20) |

## B. Gates the first real CAR (end of the spine, Phase 1)

| Id | Question | Options / default | Unblocks | Ref | Answer |
|---|---|---|---|---|---|
| OI-10 | IFS Cloud: which modules are licensed at the first deployment (Projects, EAM, HCM, shop floor / MES)? | List | `ERPi_IFS` scope; CMMSi_IFS / HSEi_IFS candidacy | §10 | |
| OI-11 | IFS Cloud: IAM client, service account, and permission set provisioned for `ERPi_IFS`? | Done / owner | `ERPi_IFS` build start | §10 | |
| OI-12 | IFS Cloud: Projects-module projection names captured from the API Explorer (with their `$openapi` descriptions)? | List | UDTS specs for `ERPi_IFS` | §10 | |
| OI-13 | Will IT approve an inbound path for IFS event push (and DocuSign webhooks): DMZ reverse proxy, VPN, or private connection? Or polling only? | Path, or polling | Connector design; `§11` network rules | §10, §11 | |
| OI-14 | **The DoA / approval-limits matrix and capital policy**: limits by role and amount; whether they differ by site, BU, or project type; board and capital-committee thresholds; which approvals need DocuSign; required CAR content and attachments; delegation rules when an approver is out. | Documents from Finance | Real values for the `car.*`, `sla.*`, and `capital_plan.approval.*` policy variables; the CAR chain | §08 | |
| OI-15 | Is FEL funding authorized separately from the full CAR today (with what cap), or does the company sanction once at FEL3? | Separate (keep `fel_funding`, set `car.fel_funding.cap_pct`) or once (disable by policy) | Staged CAR configuration | §03, §08 | |
| OI-16 | Threshold basis (TIC, TIC plus contingency, total project cost), reporting currency for thresholds, and who maintains the `fx.rate.*` table? | *default:* TIC; company reporting currency; Finance controls, monthly | `car.threshold.basis`, `car.threshold.currency`, roll-ups | §08, §11 | |
| OI-17 | Which workflow kinds require DocuSign (mandatory Sigi) vs internal attestation? | *default:* CAR at or above `car.esign.required_above`, change requests touching contractual terms, closeout | Sigi coupling | §08 | |
| OI-18 | The company's identity provider (Entra ID, Okta, ADFS, other)? | Vendor; OIDC or SAML | Which IdPi module is "day one" | §01, §04 | |
| OI-19 | MFA policy: hardware token for local break-glass admins beyond TOTP? MFA enforced at the IdP for SSO users? | *default:* TOTP for break-glass; MFA assumed at IdP | Break-glass and session design | §04 | |
| OI-20 | User provisioning: JIT on first login only, or SCIM / periodic sync from the IdP? | *default:* JIT, with deprovision on IdP signal | IdPi scope | §04 | |
| OI-21 | The pilot team and the three pilot projects (real, in flight)? | Names | Phase 1 scope; wave 0 | roadmap, §16 | |
| OI-22 | Named Governance Author and Policy Owner from the company? | Names, time commitment | Constraint corpus; policy values before the first governed CAR | roadmap, §08 | |
| OI-23 | Who is the change lead (internal or contracted), half-time from Phase 1? | Name | Adoption plan ownership | §16 | |
| OI-24 | Schema contract for actuals from the ERP: common chart of accounts, WBS mapping table, or both? | Depends on OI-10 to OI-12 | `ERPi_IFS` mapping YAML; `BudgetActual` | §03, §07 | |
| OI-25 | May external parties (for example a JV partner) be approvers on specific workflow kinds? | Yes (needs federation path and pool model) or no | Eligibility pools; IdPi federation | §08 | |

## C. Before onboarding wave 1 and the Phase 2 layers

| Id | Question | Options / default | Unblocks | Ref | Answer |
|---|---|---|---|---|---|
| OI-26 | Inventory of in-flight projects: count, amount band, site, where their records live today? | *default:* registration workbook circulated to owners in Phase 1 | Wave 1 sizing | §16 | |
| OI-27 | The CAR-only date the sponsor will set (and the health-review and status-deck sunset dates)? | *default:* start of Phase 3, announced at end of Phase 1 | Mandatory-use policy | §16, §08 | |
| OI-28 | Vendors in play so the right modules are built first: CMMS; MES / WMS / QMS and OEE or loss platform; schedule tools; notification channel (Teams or Slack); document store (SharePoint or Drive). | Vendor list | Phase 2 connector priorities | §01, §10, roadmap | |
| OI-29 | Does the EPCM run an EDMS (Aconex, ProjectWise, other) that should be mirrored through Doci, or do contributors raise transmittals in pro_manage? | Mirror or native | Document-control build; `Doci_ACX`-class module | §03, §10 | |
| OI-30 | The company's document numbering scheme and revision-code convention? | Letters before IFC, numbers after, or other | Register validation | §03 | |
| OI-31 | Probability bands behind the 1 to 5 likelihood scale; contingency as expected value or P-value? | *default:* expected value (P50/P80 needs a simulation, out of scope) | Risk quantification, contingency coverage | §03 | |
| OI-32 | Is a Program layer (parent of related projects, for example the same scope at two sites) needed in year one? | *default:* no; Portfolio suffices | Domain model | §03 | |
| OI-33 | External stakeholders: mainly federated SSO or tokenized links? May tokenized viewers be anonymous, or must they self-attest? | *default:* both, tokenized first; self-attest | `DashboardShare`, `pm-frontend-public` | §02, §04, §09 | |
| OI-34 | Is a Portfolio Manager role (allocates, does not approve) needed separately from Portfolio Executive? | *default:* fold into Portfolio Executive plus PMO Lead | Roles | §02 | |
| OI-35 | Do read-only auditors need a time-boxed session type? | *default:* a session attribute, not a role | Access model | §02, §04 | |
| OI-36 | Lessons-learned harvest into the governance space: Governance Author alone, or workshops with site maintenance leads per asset class? | *default:* workshops | Governance-space seeding before wave 1 | §16 | |
| OI-37 | Chart types executives and Finance want beyond the grammar; charting library pair (frontend and pm-sidecar); PDF engine? | *default:* grammar types first; one shared grammar; Chromium headless | Chart builder, exports | §09 | |
| OI-38 | Which Phase 2 connector backends are highest value? | Ordered list | Effort allocation | roadmap, §10 | |
| OI-39 | The two or three completed projects for the `§14` project-loss retrospective, and the remaining prior-art checks? | Project names; who can pull RFI logs, NCRs, submittal registers | Validating the loss model before the EPCM objects are built | §14, roadmap | |

## D. Later (does not gate the spine or wave 1)

| Id | Question | Options / default | Unblocks | Ref | Answer |
|---|---|---|---|---|---|
| OI-40 | Which ISO standards bind (9001, 55000, 27001, industry-specific)? | List | Retention bounds, access-review cadence, change-control detail | §01, §07, §11 | |
| OI-41 | Legal: does hosting a foreign site's project and contributor data on the home-country deployment need a data-transfer agreement or local notice (PIPEDA first case)? Which privacy regime values to seed per site? | Legal opinion | That site going live | §11 | |
| OI-42 | Fiscal year, capital plan cycle calendar (submission, review, approval dates), hurdle rates (IRR, payback)? | From Finance | `capital_plan.*` policy variables; Phase 3 plan cycle | §03, §08 | |
| OI-43 | On-call shape (business hours vs 24/7); cold-storage tier for backups; Kubernetes timeline and whether a parallel cluster is stood up in Phase 4? | *defaults:* business hours with ticketing after; TBD; defer K8s decision to Phase 4 | Ops planning | §11, §05, roadmap | |
| OI-44 | May third parties build connector modules; if so, what signing and review process? | *default:* not in year one | SDK governance | §10, §15 | |
| OI-45 | UxTS later items: will MDEMG's UAMS Go runner exist before Phase 2; is the UVTS guidance-relevance corpus worth building in Phase 3 and who grades it; who owns UPMTS; are UATS specs published externally? | *defaults:* spec-only until runner; yes, graded by Maintenance and Operations leads; metrics team with Finance; internal only | Phase 2 and 3 test governance | §12 | |
| OI-46 | Retention: GuidanceOutcome snapshots not tied to a governed action; generated export artifacts? | *defaults:* 180 days; 90 days ad hoc | Storage sizing | §06, §09, §11 | |
| OI-47 | OCR-index imported CAR scans for search? | *default:* yes, metadata only, nothing to MDEMG | Onboarding tool | §16 | |

## E. Technical defaults proposed (accept by silence at ratification)

These are engineering choices the FSD has already defaulted. Listed so they are visible, not because they block anything.

| Id | Choice | Default in the FSD | Ref |
|---|---|---|---|
| OI-48 | Outbox transport | Postgres outbox plus worker; revisit above ~1M events/day | §05 |
| OI-49 | Python-dependent connector backends | Separate per-backend containers, not inside pm-sidecar | §05 |
| OI-50 | Derived-metric formulas | Sandboxed Python in pm-sidecar with an allowlist (CEL/Starlark as alternative) | §05, §07 |
| OI-51 | PDF/PPTX template authoring and storage | Git-tracked in the repo, hashed and cached in Postgres | §05, §09 |
| OI-52 | `BudgetActual` vs `MetricReading` storage | Separate (financial-period semantics vs continuous) | §03 |
| OI-53 | HSE metrics in pm-TSDB | Duplicate daily for dashboards; live-fetch for drill-down | §07 |
| OI-54 | Earned value | Optional per project; derived metrics skip when inputs absent | §07 |
| OI-55 | `MetricDefinition.category` granularity | Defined with the metrics team in Phase 2 | §03 |
| OI-56 | Change request as a workflow kind | Standalone, referencing Budget and Scope | §08 |
| OI-57 | Re-submit after reject | New workflow id with explicit linkage | §08 |
| OI-58 | Approving while out of office | Allowed, with a logged confirm-OOO override | §08 |
| OI-59 | Session resumption / checkpointing with MDEMG | Phase 2 | §06 |
| OI-60 | Alert policy on Tier A classify fail-closed events | P1 above a per-minute threshold; defined with on-call shape (OI-43) | §06, §11 |
| OI-61 | UDTS fixtures | Structural assertions for most RPCs; golden bytes for the SDK conformance suite | §12 |
| OI-62 | Connector containers placement | Dedicated connector VM(s), especially for backends needing VPN or elevated access | §10 |
| OI-63 | Field-level grant syntax | Simple globs in year one | §04 |
| OI-64 | SDK Python runtime | Deferred until a Python-only vendor appears | §15 |
| OI-65 | Recorded connector fixtures | Scrubbed by the module author, reviewed in PR, stored under the module's `testdata/` | §15 |
| OI-66 | Aligning SDK scaffold / validate commands with MDEMG plugin tooling | Yes, where the shape matches | §15 |

## Answered and folded back

| Id | Answer | Folded into | Date |
|---|---|---|---|
| OI-01 | Scenario A | roadmap (Delivery scenarios, open items) | 2026-10-20 |
| OI-03 (dev part) | MDEMG `v0.11.0-beta.3` at `c1f14f74`, dedicated stack, `apikey` | §06 decisions, §05 local dev | 2026-10-20 |
| OI-04 (dev part) | `.env` behind a provider interface | §05 decisions | 2026-10-20 |
| OI-09 | Vendored at `c1f14f74`; stub MDEMG in CI; live UATS locally | §12 decisions | 2026-10-20 |
| Spine start set (not on the list) | Monorepo here, one PR per slice; this Mac with Docker Desktop; Go 1.26, Python 3.14, Next.js 16, goose, pgx + sqlc; Keycloak dev IdP; infrastructure-first order; Northfield Spirits Co. seed company; `IdPi_OIDC` first | §05 decisions, §06, roadmap | 2026-10-20 |
