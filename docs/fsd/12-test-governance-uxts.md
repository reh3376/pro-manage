# 12 — Test Governance (UxTS Adoption)

## Decisions on record

- **2026-10-13** — Adopt UxTS as test governance for pro_manage: **UATS + UDTS + UNTS from MVP**, with broader adoption through Phase 2 and beyond.
- **2026-10-13** — The UxTS Developer Guide and the shared Python runner primitives (`uxts_runner_core.py`, `uxts_report.py`) are **vendor-copied** into pro_manage (per the Portable Agent Spec). We do not take a runtime dependency on MDEMG for test governance.
- **2026-10-13** — Every connector type's gRPC contract has a **UDTS spec**. Every externally-facing REST endpoint has a **UATS spec**. All spec and fixture files are **hashed in a UNTS registry** committed to the repo.

## What UxTS is (recap)

UxTS = Universal-x Test Specification: a methodology for declarative test governance. Each framework has four layers (schema, specs, runner, CI gate) and prevents three failure modes (silent drift, 0-of-0 false pass, reinvention tax). UxTS is not a time-series layer and is not integrated at runtime — it is a build-time + CI-time concern.

## Year-one adoption

### UATS — Universal API Test Specification

**What it covers:** every externally-facing REST endpoint on pm-core (`/api/v1/*`), plus any REST surface a connector exposes (webhooks mostly).

**Why MVP:**
- The user-facing REST surface is a stability contract to the frontend and to third-party readers of exported audit records. Drift there is a correctness problem.
- Approval + metric + spreadsheet workflows depend on predictable response shapes.

**Spec shape (example):** `docs/api-spec/uats/specs/approvals_create_workflow.uats.json`
```json
{
  "name": "approvals_create_workflow",
  "endpoint": "POST /api/v1/projects/{project_id}/approvals",
  "auth": "bearer",
  "cases": [
    {
      "name": "car_under_threshold_201",
      "request": { "kind": "car", "subject_ref": "...", "amount": 50000 },
      "assertions": [
        { "type": "status_code", "equals": 201 },
        { "type": "jsonpath", "path": "$.workflow_id", "exists": true },
        { "type": "jsonpath", "path": "$.current_stage.role_required", "equals": "project_owner" }
      ]
    },
    {
      "name": "car_tier_a_denied_when_classify_blocks",
      "setup": { "mdemg_classify_fixture": "deny:car-over-5m-needs-vp-finance" },
      "request": { "kind": "car", "subject_ref": "...", "amount": 8400000 },
      "assertions": [
        { "type": "status_code", "equals": 409 },
        { "type": "jsonpath", "path": "$.violated_codes[0]", "equals": "car-over-5m-needs-vp-finance" }
      ]
    }
  ]
}
```

**Runner:** `docs/tests/uats_runner.py` — vendored from the UxTS reference, uses the shared `uxts_runner_core` and `uxts_report`. CI target `make test-uats`.

### UDTS — Universal (gRPC) Data/Contract Test Specification

**What it covers:** every internal gRPC service. In pro_manage:
- `pm.core.v1.*` (service-to-service inside pm-core cluster)
- `pm.sidecar.v1.*`
- `pm.connector.v1.*` (one per connector type)

**Why MVP:** connectors are the modularity story. Different ERPi backends implementing the same proto must behave identically — the UDTS spec is the contract.

**Spec shape:** per-RPC specs with fixture pairs (input proto → expected response proto). Fixture binaries are hashed via UNTS so a drift in a fixture binary is caught in CI.

### UNTS — Universal Hash Test Specification

**What it covers:** every UxTS spec file + every fixture file. Prevents unreviewed modifications of test contracts.

**Why MVP:** without UNTS, a bad merge could silently loosen a UATS or UDTS spec. UNTS makes those changes visible as hash mismatches in CI.

**Registry location:** `docs/specs/unts-registry.json`, committed to repo. Updated by developers via `make unts-update` after reviewing changes. CI verifies clean state on every PR.

### CI integration

- `make test-uats` — runs all UATS specs against a compose-local pm-core.
- `make test-udts` — runs all UDTS specs; connector tests mocked by default, with a nightly job that targets each registered sandbox connector instance.
- `make test-unts` — runs UNTS hash verification against the registry.
- `make test-all` — runs the above plus Go/Python/TS unit tests.
- GitHub Actions workflow (or equivalent) enforces green on every PR; failures block merge.

## Phase 2 and beyond

### UBTS — Universal Benchmark Test Specification

**When:** after MVP ships and performance SLOs are documented in `§11`.

**What it covers:** critical-path performance budgets — dashboard tile queries, approval submit latency, metric harvest batch latency, Jiminy call latency. Each benchmark declares acceptable p50/p95/p99 and the CI job alerts on regression > N%.

### USTS — Universal Security Test Specification

**When:** before year one GA, for the user-facing surface.

**What it covers:** authentication requirements (every protected endpoint rejects unauthenticated), authorization requirements (every endpoint rejects wrong-role / wrong-grant callers), content-sanitization expectations, header hardening (CSP, HSTS, X-Content-Type-Options).

### UOTS — Universal Operations Test Specification

**When:** when Kubernetes migration starts (year two target).

**What it covers:** K8s manifests, Helm charts, Terraform modules. Ensures resource limits, health probes, non-root users, secret-not-in-env patterns are consistent.

### UPMTS — Universal Project-Metric Test Specification (custom)

**When:** Phase 2.

**What it covers:** pro_manage-specific metric contracts. "Metric `budget.variance_pct` at project level is computed from `budget.actual.last`, `budget.committed.last`, and the current `BudgetBaseline.baseline_total` via the stated formula, within tolerance ε." The runner reads metric definitions and verifies the compute path. Catches silent drift in derived-metric formulas when inputs or math change.

### UAMS — Universal Alert / Monitoring Test Specification

**When:** after a few months of operational data informs what alerts matter.

**What it covers:** alerting rules — every P1/P2 alert in `§11` has a spec that validates "when condition X holds, the alert fires within Y seconds; when condition clears, the alert resolves within Z seconds."

### Not planned

- UVTS (visual tests) — out of scope; we use standard snapshot testing in the frontend.
- UITS (interaction tests) — out of scope; we use Playwright for E2E.
- UETS, ULTS, UTDS, UAITS — not expected to apply.

## Developer experience

- A new API endpoint ships with its UATS spec in the same PR.
- A new gRPC method ships with its UDTS spec + fixture.
- Changing a spec requires a UNTS re-hash in the same PR; reviewer sees the hash change explicitly.
- Running the full suite locally takes under 2 minutes for the MVP footprint (UATS alone can be faster via parallelization in CI).

## Why not framework X instead

- **vs. Postman collections** — Postman is interactive and GUI-first; our specs are code-reviewable and CI-first.
- **vs. OpenAPI contract tests alone** — OpenAPI validates shape; UATS validates behavior (status codes, side-effects, state transitions) tied to fixtures.
- **vs. standard pytest/table tests** — those remain for unit tests. UxTS is for cross-cutting contract tests governed by schema + hash integrity.

## Open items for Section 12

- Confirm the vendored UxTS runner version pin. Baseline is "current MDEMG release at FSD ratification" with periodic re-vendor as upstream matures.
- Decide whether UDTS fixtures include "golden" response bytes or structural assertions only (first instinct: structural for most RPCs, golden for critical ones).
- Confirm whether we publish the UATS specs externally (helpful if any third-party consumers of pro_manage's REST ever emerge). First instinct: internal only in year one.
- Decide the owner-group for UPMTS (which team authors the derived-metric contracts). First instinct: the metrics team in collaboration with Finance for CPI/SPI/EAC definitions.
