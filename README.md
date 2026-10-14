# pro_manage

**An owner-side capital-project management, data-curation, and metrics system for a TPM-driven process industry.**

Status: **Working draft — FSD and roadmap in [`docs/fsd/`](./docs/fsd/README.md).** No application code committed yet; design and specification are the current artifact.

---

## What this is

pro_manage is a custom application being designed to carry a capital project from Front-End Loading through Vertical Startup to Benefits Realization — and to make the loop that TPM is actually about (**MP information from running assets → design that incorporates it → vertical startup → benefits realization**) legible, auditable, and inheritable from one project to the next.

It is for a single company, hosted on the company's intranet in containers, tuned to the discipline of **CAR → FEL → EEM → Vertical Startup → Benefits Realization** at a scale of 100–500 users across 200+ concurrent capital projects ranging from $25,000 refurbishments to $250,000,000 greenfields.

**Target context:** distilling, bottling, food & beverage manufacturing, chemicals, and similar process industries where TPM is the operating language.

**The organizing question**, phrased the way a plant manager would: *"Did the asset start up clean, and did Operations and Maintenance inherit something they could run?"* Every capability, workflow, and metric in pro_manage ties back to that.

---

## Where to find things

| Looking for | Open |
|---|---|
| **The hero document — read this first** | [`docs/fsd/README.md`](./docs/fsd/README.md) |
| Vision, scope, success criteria | [`docs/fsd/01-vision-and-scope.md`](./docs/fsd/01-vision-and-scope.md) |
| Personas, use cases, journeys | [`docs/fsd/02-personas-and-use-cases.md`](./docs/fsd/02-personas-and-use-cases.md) |
| Domain model (EEM-stage lifecycle, DesignReview, MPInformation, TurnoverPackage, BenefitsRealization) | [`docs/fsd/03-domain-model.md`](./docs/fsd/03-domain-model.md) |
| Access control + identity (full ACL per section/doc/field) | [`docs/fsd/04-access-control-and-identity.md`](./docs/fsd/04-access-control-and-identity.md) |
| Architecture (Go core + Python sidecar + Next.js + modular connectors) | [`docs/fsd/05-architecture.md`](./docs/fsd/05-architecture.md) |
| MDEMG integration (incl. flagship MP-information flow) | [`docs/fsd/06-mdemg-integration.md`](./docs/fsd/06-mdemg-integration.md) |
| Metrics (incl. TPM / Plant Ops family: OEE, MTBF, Startup Curve, …) | [`docs/fsd/07-metrics-and-data-curation.md`](./docs/fsd/07-metrics-and-data-curation.md) |
| Approvals (CAR, baseline change, design review, CQV, Early Mgmt exit, closeout) | [`docs/fsd/08-approvals-and-workflows.md`](./docs/fsd/08-approvals-and-workflows.md) |
| Presentations (live dashboards, scheduled exports, on-demand decks) | [`docs/fsd/09-presentations-and-reporting.md`](./docs/fsd/09-presentations-and-reporting.md) |
| Integrations catalog (ERPi, IdPi, Sigi, Schedulei, Doci, Commsi, Metrici, HSEi, CMMSi) | [`docs/fsd/10-integrations.md`](./docs/fsd/10-integrations.md) |
| Operations & non-functional requirements | [`docs/fsd/11-ops-and-nfr.md`](./docs/fsd/11-ops-and-nfr.md) |
| Test governance (UxTS: UATS, UDTS, UNTS, …) | [`docs/fsd/12-test-governance-uxts.md`](./docs/fsd/12-test-governance-uxts.md) |
| Glossary (TPM, EEM, CAR, AACE, LCC/TIC, OEE/MTBF/…) | [`docs/fsd/13-glossary.md`](./docs/fsd/13-glossary.md) |
| Phased roadmap, milestones, risk register | [`docs/fsd/roadmap.md`](./docs/fsd/roadmap.md) |

---

## Technology at a glance

- **Backend:** Go monolith (`pm-core`) with a Python sidecar (`pm-sidecar`) for render / spreadsheets / derived-metric formulas, and a Go workers tier (`pm-workers`) for scheduled jobs and outbox dispatch.
- **Frontend:** Next.js (TypeScript / React) — SSR + CSR; a hardened `pm-frontend-public` variant for tokenized dashboard shares.
- **APIs:** gRPC between internal services; REST + SSE / WebSockets to the frontend.
- **Data stores:** Postgres (OLTP source of truth), TimescaleDB (metric readings), Redis (sessions / cache), MinIO (S3-compatible object storage). All containerized.
- **Governance substrate:** [MDEMG](https://github.com/reh3376/mdemg) running adjacent on an isolated internal network, used for advisory guidance and institutional memory (MP information flow). pro_manage owns identity, authorization, workflows, and the authoritative audit log.
- **Connectors:** nine pluggable modules — **ERPi, IdPi, Sigi, Schedulei, Doci, Commsi, Metrici, HSEi, CMMSi** — each a container with a stable gRPC contract. Admins swap backends without a core release.
- **Deployment:** on-prem VMs with containers via docker-compose, designed so a later move to Kubernetes is a lift-and-shift rather than a redesign.
- **Test governance:** UxTS from MVP — UATS (API contracts), UDTS (gRPC contracts), UNTS (hash integrity on specs + fixtures) — in CI from day one.

---

## Development workflow

- Default development branch: `pro-dev01`. Changes flow via pull requests into `main` for owner review.
- Direct commits to `main` are not permitted; `main` is the reviewed-and-merged state only.
- Commits are attributed to the actual author (Roger Henley for human-authored work; `Claude Opus 4.7 <noreply@anthropic.com>` for AI-assisted drafting).

---

## Status

This repository is a **living FSD and roadmap**, not yet an implementation. The design is being iterated under direct dialogue between the owner and an AI collaborator (Claude Code), with decisions captured in each FSD section's **Decisions on record** block and open items under **Open items for Section N**.

Implementation begins once the FSD is ratified and the Phase 0 team shape is set — see [`docs/fsd/roadmap.md`](./docs/fsd/roadmap.md).

---

## Author / Owner

**Roger Edward Henley II** — thirty years on the owner's side of industrial capital work; eleven years inside Kentucky distilleries; author and architect of pro_manage.

**AI collaborator:** Claude Opus 4.7 via Claude Code, under the owner's direction, 2026-10-13 onward.

**Related project:** [MDEMG](https://github.com/reh3376/mdemg) — Multi-Dimensional Emergent Memory Graph, the governance substrate pro_manage integrates with.
