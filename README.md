# pro_manage

**An owner-side capital-project management, data-curation, and metrics system for a TPM-driven process industry.**

Status: **FSD and roadmap in [`docs/fsd/`](./docs/fsd/README.md); spine development started 2026-10-20** under the owner-plus-AI scenario (see the roadmap's "Delivery scenarios"). Code arrives one slice per PR, infrastructure first.

---

## What this is

pro_manage is a custom application being designed to carry a capital project from Front-End Loading through Vertical Startup to Benefits Realization, and to make the loop that TPM is actually about (**MP information from running assets → design that incorporates it → vertical startup → benefits realization**) legible, auditable, and inheritable from one project to the next.

> **Core idea: the project as equipment.** pro_manage treats the capital project itself as a piece of equipment and manages it with TPM. A plant names, prices, and eliminates the losses on its machines; pro_manage does the same for the project's own losses (**delays, rework, interface clashes, and waiting on information and approvals**), which today disappear into "schedule slip." The eight TPM pillars are applied to project delivery, losses are captured as a by-product of normal work (RFIs, submittals, work packages, hold points), and project losses and plant losses are priced in the same dollars on the same Pareto (the ranked bar chart of losses by dollar value that a plant uses to pick what to fix first). It builds on Lean Construction (Last Planner), Advanced Work Packaging, and earlier work adapting OEE to construction workflow; a first prior-art search found no source that combines these elements. See [`docs/fsd/14-project-as-equipment.md`](./docs/fsd/14-project-as-equipment.md).

It is for a single company, hosted on the company's intranet in containers, tuned to the discipline of **CAR → FEL → EEM → Vertical Startup → Benefits Realization** at a scale of 100–500 users across 200+ concurrent capital projects ranging from $25,000 refurbishments to $250,000,000 greenfields.

**Target context:** distilling, bottling, food & beverage manufacturing, chemicals, and similar process industries where TPM is the operating language.

**Where it sits in TPM:** pro_manage is the system for the EEM pillar and capital-scale Focused Improvement. It integrates with the plant's operations TPM tools (CMMS, connected-worker, OEE / loss platforms) rather than rebuilding them: it reads their losses, abnormality tags, and failure history in, and hands assets and turnover packages back out. Business cases promise recoverable losses priced in dollars, and Benefits Realization verifies them. Alongside that, it applies TPM to the project itself (the core idea above), with EPCM and contractor staff working in the system on their own packages.

**Works with the systems the company already runs.** pro_manage does not replace the ERP, the maintenance system, or the plant's data systems; it connects to them through pluggable connector modules named `<Type>i_<CODE>` (for example `ERPi_IFS` for IFS Cloud, `ERPi_NS` for NetSuite, `CMMSi_MX` for MaintainX), so changing vendors is a configuration change, not a rewrite. Nine connector types cover ERP, single sign-on, e-signature, scheduling tools, document stores, notifications, plant metrics (historians, OPC UA, databases), plant operations systems (MES, QMS, OEE and loss platforms), and the CMMS. When the ERP has a capital-projects module (the first target, IFS Cloud, does), the ERP stays the system of record for budgets, forecasts, earned value, subcontracts, and change orders; pro_manage mirrors them and adds the TPM and governance layer. With a financials-only ERP, pro_manage owns those records instead. Writes back to the ERP are limited to operational records plus one controlled budget release per approved CAR. Details: [integrations catalog (§10)](./docs/fsd/10-integrations.md), [system of record by deployment profile (§03)](./docs/fsd/03-domain-model.md#system-of-record-by-deployment-profile), and [connector framework (§05)](./docs/fsd/05-architecture.md#modular-connector-framework).

**The organizing question**, phrased the way a plant manager would: *"Did the asset start up clean, and did Operations and Maintenance inherit something they could run?"* Every capability, workflow, and metric in pro_manage ties back to that.

---

## Where to find things

| Looking for | Open |
|---|---|
| **The hero document (read this first)** | [`docs/fsd/README.md`](./docs/fsd/README.md) |
| Vision, scope, success criteria | [`docs/fsd/01-vision-and-scope.md`](./docs/fsd/01-vision-and-scope.md) |
| Personas (incl. site operations & maintenance reps, construction manager, EPCM / contractor contributors), use cases, journeys | [`docs/fsd/02-personas-and-use-cases.md`](./docs/fsd/02-personas-and-use-cases.md) |
| Domain model (EEM-stage lifecycle, Asset, Loss, BusinessCasePromise, DesignReview, MPInformation, TurnoverPackage, BenefitsRealization, Contract, WorkPackage, RFI / Submittal / ChangeOrder, ProjectLoss) | [`docs/fsd/03-domain-model.md`](./docs/fsd/03-domain-model.md) |
| Access control + identity (platform / project / workflow roles, contract-scoped contributors, full ACL per section/doc/field) | [`docs/fsd/04-access-control-and-identity.md`](./docs/fsd/04-access-control-and-identity.md) |
| Architecture (Go core + Python sidecar + Next.js + modular connectors) | [`docs/fsd/05-architecture.md`](./docs/fsd/05-architecture.md) |
| MDEMG integration (incl. flagship MP-information flow) | [`docs/fsd/06-mdemg-integration.md`](./docs/fsd/06-mdemg-integration.md) |
| Metrics (incl. TPM / Plant Ops, priced losses with verified recovery, and project losses) | [`docs/fsd/07-metrics-and-data-curation.md`](./docs/fsd/07-metrics-and-data-curation.md) |
| Approvals (configurable per-project thresholds, CAR, baseline change, design review, CQV, shutdown & tie-in, Early Mgmt exit, closeout, Benefits Realization, health review, RFI / submittal / change order, package release, hold points) | [`docs/fsd/08-approvals-and-workflows.md`](./docs/fsd/08-approvals-and-workflows.md) |
| Presentations (live dashboards incl. Project reliability, user-built charts and graphs with image and data export, scheduled exports, on-demand decks, mobile field flows) | [`docs/fsd/09-presentations-and-reporting.md`](./docs/fsd/09-presentations-and-reporting.md) |
| Integrations catalog (ERPi, IdPi, Sigi, Schedulei, Doci, Commsi, Metrici, HSEi, CMMSi) | [`docs/fsd/10-integrations.md`](./docs/fsd/10-integrations.md) |
| Connector SDK (build new `<Type>i_<CODE>` modules) | [`docs/fsd/15-connector-sdk.md`](./docs/fsd/15-connector-sdk.md) |
| Onboarding, data migration, and adoption (levels, bulk import, waves, change management, scorecard) | [`docs/fsd/16-onboarding-migration-and-adoption.md`](./docs/fsd/16-onboarding-migration-and-adoption.md) |
| Operations & non-functional requirements | [`docs/fsd/11-ops-and-nfr.md`](./docs/fsd/11-ops-and-nfr.md) |
| Test governance (UxTS: hashed contract specs per surface, framework matrix, adjudication registries) | [`docs/fsd/12-test-governance-uxts.md`](./docs/fsd/12-test-governance-uxts.md) |
| **The project as equipment: Project TPM (core concept)** | [`docs/fsd/14-project-as-equipment.md`](./docs/fsd/14-project-as-equipment.md) |
| Glossary (TPM, EEM, CAR, AACE, LCC/TIC, OEE/MTBF/…) | [`docs/fsd/13-glossary.md`](./docs/fsd/13-glossary.md) |
| Phased roadmap (thin spine, delivery scenarios, layer catalogue), milestones, risk register | [`docs/fsd/roadmap.md`](./docs/fsd/roadmap.md) |
| Owner decision checklist (every open item, grouped by what it unblocks) | [`docs/fsd/open-items.md`](./docs/fsd/open-items.md) |
| Engineering backlog (repo hardening, tooling, hygiene items) | [`docs/backlog.md`](./docs/backlog.md) |

---

## Technology at a glance

- **Backend:** Go monolith (`pm-core`) with a Python sidecar (`pm-sidecar`) for render / spreadsheets / derived-metric formulas, and a Go workers tier (`pm-workers`) for scheduled jobs and outbox dispatch.
- **Frontend:** Next.js (TypeScript / React), SSR + CSR; a hardened `pm-frontend-public` variant for tokenized dashboard shares.
- **APIs:** gRPC between internal services; REST + SSE / WebSockets to the frontend.
- **Data stores:** Postgres (OLTP source of truth), TimescaleDB (metric readings and loss events), Redis (sessions / cache), an S3-compatible object store (RustFS in dev). All containerized.
- **Governance substrate:** [MDEMG](https://github.com/reh3376/mdemg) running adjacent on an isolated internal network, used for advisory guidance and institutional memory (MP information flow). Its language model runs locally on the MDEMG host, so no project data is sent to a third-party AI service. pro_manage owns identity, authorization, workflows, and the authoritative audit log.
- **Connectors:** nine pluggable modules (**ERPi, IdPi, Sigi, Schedulei, Doci, Commsi, Metrici, HSEi, CMMSi**), each a container with a stable gRPC contract. Admins swap backends without a core release. New modules are built with the Connector SDK (shared auth, retry, polling, and event plumbing plus a conformance test kit).
- **Deployment:** on-prem VMs with containers via docker-compose, designed so a later move to Kubernetes is a lift-and-shift rather than a redesign.
- **Test governance:** UxTS, MDEMG's family of hashed, schema-governed contract specs, adopted as a design principle rather than a test detail: every contract the FSD names (REST shapes, connector gRPC contracts, MDEMG field shapes, policy variables, metric formulas, dashboards and alerts, auth boundaries) gets a spec, a hash, and a CI disposition. UATS, UDTS, and UNTS from Phase 0; UOTS and USTS before the pilot; UBTS, UOBS, UAMS, and a pro_manage-minted UPMTS later. No test step is allowed to soft-fail by accident.

---

## Getting started (development)

Prerequisites on the workstation: Docker Desktop, Go 1.26, `uv` (Python 3.14), Node 22 with `pnpm` 12, `buf`. For the real MDEMG: `llama-server` on `:8102` and Ollama on `:11434` with `nomic-embed-text` pulled (see `deploy/compose/.env.example`).

```bash
make env          # create deploy/compose/.env with generated dev keys
make mdemg-up     # pro_manage's dedicated MDEMG stack (pinned v0.11.0-beta.3)
make dev          # Postgres, TimescaleDB, Redis, RustFS, Keycloak, Mailpit, pm-core, pm-workers, pm-sidecar, pm-frontend
make dev-stub     # same stack but against the stub MDEMG, as PR CI runs it
make test         # Go tests, sidecar tests, frontend type check
make lint         # go vet, ruff, mypy, buf lint
make uxts         # UxTS gates: spec hashes, UNTS registry, adjudication inventories, matrix currency
make test-uats    # UATS contract specs against the running dev stack (and test-uats-stub, test-uats-live)
make test-udts    # UDTS gRPC contract specs against pm-core and the IdPi_OIDC module
make check-outbox # prove the outbox path: pm-core enqueue -> pm-workers drain -> MDEMG
make check-car    # the Phase 0 exit test: a toy CAR end to end (guide card, classify, audit + GuidanceOutcome, four-stage chain, e-signature, feedback)
make check-login  # SSO end to end: pm-core -> IdPi_OIDC -> Keycloak login form -> session cookie -> /api/v1 -> logout
make hooks        # install the pre-commit gate (same checks as CI, before every commit)
make help         # everything else
```

Then `make db-seed` loads the Northfield Spirits Co. data (two sites, five projects, the starter policy variables, the approver pools for the dev users, and one approved CAR of history). The API is at `http://localhost:8080/api/v1`. Sign in through the frontend's link or `http://localhost:8080/auth/login` (the dev Keycloak users, password `northfield-dev`); the `pm_session` cookie then authenticates every call. In development and CI the `X-Dev-Actor` header still names an acting user directly and `X-Dev-Roles` lists platform roles (`PM_DEV_ACTOR_HEADER`), which is what the examples below use. Access follows FSD §04: the project's owner sees everything, members see what their base role and grants allow, and `GET /api/v1/projects/{id}/access` shows the caller's effective permission per section:

```bash
curl -s -H 'X-Dev-Actor: alice.owner' localhost:8080/api/v1/projects | jq '.items[].code'
curl -s -H 'X-Dev-Actor: alice.owner' localhost:8080/api/v1/projects/01900000-0000-7000-8000-00000000b004/policy/car.threshold.vp_finance
curl -s -H 'X-Dev-Actor: alice.owner' localhost:8080/api/v1/audit/verify
curl -s -H 'X-Dev-Actor: alice.owner' localhost:8080/api/v1/cars/01900000-0000-7000-8000-00000000e002/guidance | jq '.guidance.items, .preview.chain'
curl -s -H 'X-Dev-Actor: bob.finance' localhost:8080/api/v1/inbox
curl -s -H 'X-Dev-Actor: fay.ops' localhost:8080/api/v1/projects/01900000-0000-7000-8000-00000000b001/access | jq .sections
```

The governed action today is the CAR (`POST /api/v1/projects/{id}/cars`, then `/cars/{id}/guidance`, `/preclassify`, `/submit`, `/decide`): guide on open from both MDEMG spaces, classify on submit under the kind's fail-closed tier (`mdemg.tier.approval.car`), the chain of §08 built from the policy snapshot, every decision audited, and the `GuidanceOutcome` written with the `AuditEvent`. `scripts/car_check.sh` walks it end to end and is what CI runs.

Services on the host: frontend `:3100`, pm-core `:8080` (REST) and `:8090` (gRPC, reflection on in dev), pm-workers `:8081`, sidecar `:7070` (HTTP) and `:7071` (gRPC), IdPi_OIDC `:7080` (gRPC) and `:7081` (HTTP), Sigi_STUB `:7090` (gRPC) and `:7091` (HTTP, dev signing), Keycloak `:8180` (realm `northfield`, users such as `alice.owner` / `northfield-dev`), Mailpit `:8026`, RustFS console `:9101`, dedicated MDEMG `:9998`. The seed company is the fictional Northfield Spirits Co.

Layout: `services/pm-core` (Go: `cmd/pm-core`, `cmd/pm-workers`, `cmd/stub-mdemg`), `services/pm-sidecar` (Python), `apps/pm-frontend` (Next.js), `connectors/<type>/<code>/` (connector modules, each its own Go module; `idpi/oidc` first), `proto/`, `deploy/compose/`, `docs/`.

## Development workflow

- Default development branch: `pro-dev01`. Changes flow via pull requests into `main` for owner review and approval.
- PRs are opened automatically by `.github/workflows/auto-pr.yml` (as `github-actions[bot]`) when `pro-dev01` is pushed, so the owner can formally approve each one. PRs are never opened by hand.
- Direct commits to `main` are not permitted; `main` is the reviewed-and-merged state only.
- Code lands one slice per PR (a scaffold, a set of migrations, an adapter), small enough to review in one sitting and leaving `main` runnable after each merge.
- Commits are attributed to the actual author (Roger Henley for human-authored work; `Claude <noreply@anthropic.com>` for AI-assisted drafting).

---

## Status

The FSD and roadmap are ratified and the **thin spine** is being built under Scenario A (owner plus AI collaborator), one reviewed PR per slice on `pro-dev01`. Landed so far: the monorepo scaffold and compose stack (0a), the UxTS gates (0b), pm-core with Sites, Projects, policy variables and the hash-chained audit log (1a), the gRPC surface with UDTS (1b), `pkg/mdemgadapter` (2a), the Postgres outbox and pm-workers drain (2b), the toy CAR end to end with the governance gate (3), the connector contract with the `IdPi_OIDC` module against the dev Keycloak (4a), SSO sign-in with just-in-time users, Redis sessions, and approver pools from IdP claims (4b), project membership with the section-level ACL of §04 on every project route (5), and the e-signature envelope on CAR approval with the signed PDF captured to the object store (6, against the `Sigi_STUB` module). The roadmap's Phase 0 exit test (a CAR approval that renders a guidance card, classifies on submit, writes the AuditEvent with its GuidanceOutcome, and dispatches feedback) runs in CI against the stub MDEMG. Next on the spine: `ERPi_IFS` (budget mirror and ReleaseBudget), Commsi nudges with the SLA tick, the DocuSign backend, and the persona screens.

The design keeps iterating under direct dialogue between the owner and an AI collaborator (Claude Code), with decisions captured in each FSD section's **Decisions on record** block and open items under **Open items for Section N**. See [`docs/fsd/roadmap.md`](./docs/fsd/roadmap.md) and [`docs/backlog.md`](./docs/backlog.md).

---

## Author / Owner

**Roger Edward Henley II:** thirty years on the owner's side of industrial capital work; eleven years inside Kentucky distilleries; author and architect of pro_manage.

**AI collaborator:** Claude (Anthropic) via Claude Code, under the owner's direction, 2026-10-13 onward.

**Related project:** [MDEMG](https://github.com/reh3376/mdemg): Multi-Dimensional Emergent Memory Graph, the governance substrate pro_manage integrates with.

## License

[MIT](./LICENSE), the same license as MDEMG.
