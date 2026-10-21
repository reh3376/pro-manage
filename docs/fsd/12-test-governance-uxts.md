# 12: Test Governance (UxTS Adoption)

## Decisions on record

- **2026-10-13:** Adopt UxTS as test governance for pro_manage: **UATS + UDTS + UNTS from MVP**, with broader adoption through Phase 2 and beyond.
- **2026-10-13:** The shared Python runner primitives (`docs/tests/uxts_runner_core.py`, `docs/tests/uxts_report.py` in the MDEMG repo) and the reference runners are **vendor-copied** into pro_manage. We do not take a runtime dependency on MDEMG for test governance. **2026-10-20:** vendored from MDEMG commit `c1f14f74` (`v0.11.0-beta.3`), recorded in `docs/tests/VENDORED.md` with the MIT notice; re-vendored deliberately on MDEMG tags.
- **2026-10-20:** PR CI runs the UxTS suites against a **stub MDEMG** (same field shapes, canned verdicts); the live UATS specs against the pinned MDEMG run on the developer's machine via `make test-uats-live` and are a pre-merge step, since CI cannot run the local model (`§06`).
- **2026-10-13:** Every connector type's gRPC contract has a **UDTS spec**. Every externally-facing REST endpoint has a **UATS spec**. All spec and fixture files are **hashed in a UNTS registry** committed to the repo.
- **2026-10-14:** **This section is re-based on MDEMG's framework matrix** (`docs/development/UXTS_FRAMEWORK_MATRIX.md`, the authoritative inventory; `docs/features/uxts-frameworks.md`, the narrative). The earlier draft of this section expanded several framework letters wrongly (UVTS as visual tests, UITS as interaction tests, UOTS as Kubernetes manifests, UAMS as alert monitoring) and used a spec shape that does not match the shipped runners. Those errors are corrected below.
- **2026-10-14:** **UxTS is a design principle of pro_manage, not a testing detail.** It is the forcing function against silent contract drift. Every contract the FSD names (REST shapes, connector gRPC contracts, the MDEMG field shapes pro_manage depends on, the policy-variable set, derived-metric formulas, dashboards and alerts, auth boundaries, performance budgets) gets a row in a pro_manage framework matrix with a CI disposition, from Phase 0. Two policies are inherited from MDEMG verbatim: **no `continue-on-error` on a test step**, and **schema-runner parity is mandatory** (every schema field is either enforced by the runner or fails hard as unimplemented; never silently ignored).
- **2026-10-14:** **MVP set widened.** UATS, UDTS, and UNTS remain the Phase 0 core. **UOTS** (dashboards, alert rules, metric registry) and **USTS** (auth and authorization boundaries, data exposure, injection) join before the pilot, because the artifacts they pin exist by then and the failure they catch is the one a pilot user would hit first. UBTS, UOBS, UAMS follow in Phase 2. UPMTS (pro_manage-specific) in Phase 3.
- **2026-10-14:** **Companion adjudication registries** in MDEMG's DORMANT-CENSUS pattern are adopted for declaration surfaces that UxTS does not cover: policy variables, connector capabilities, metric definitions, REST routes, and loss categories. Every declaration must have an adjudicated consumer or CI fails.

## What UxTS is

UxTS (Universal-x Test Specification) is a family of versioned JSON spec frameworks. Each framework encodes machine-readable contracts for one correctness surface, and every framework shares one shape:

```
schema/<family>.schema.json   what a spec MUST look like
specs/*.<family>.json         one spec per contract
runners/<family>_runner.*     executes specs against the live system (or the filesystem)
ci.yml step                   merge-blocking or soft-fail
FRAMEWORK_MATRIX              inventory + CI disposition entry
```

The `x` is a letter naming the surface. MDEMG ships sixteen; the letters pro_manage cares about:

| Framework | Name | Surface it pins |
|---|---|---|
| **UATS** | Universal API Test Specification | HTTP endpoint acceptance: request shape, status, body assertions, variants, timeout |
| **UDTS** | Universal DevSpace Test Specification | gRPC contracts: service, method, request, expected response or stream behaviour, optional `proto_sha256` |
| **UNTS** | Universal Hash Test Specification | Hash integrity registry across every framework's specs and fixtures; verify and revert workflow |
| **UOTS** | Universal Observability Test Specification | Artifact-level observability: Grafana dashboard JSON, alert rule YAML, metric registry definitions, backup manifests. Static files; no server needed |
| **UOBS** | Universal Observability Specification | Live runtime observability behaviour: health probes, dependency checks, tracing headers, metric presence over the wire |
| **USTS** | Universal Security Test Specification | Security behaviour and hardening: rate limiting, data exposure, injection; auth boundary cases |
| **UBTS** | Universal Benchmark Test Specification | Throughput, latency, and load thresholds with fixed `p99_ms` baselines |
| **UAMS** | Universal Auth Method Specification | Auth method contracts (credential extraction, validation rules, principal construction, error responses). Spec-only upstream today |
| **UVTS** | Universal Validation Test Specification | Semantic retrieval quality against a graded question corpus, with an A/B regression gate. Live-gated by design |
| **ULTS** | Universal LLM Task Specification | LLM task contracts: `system_prompt_hash`, output schema, quality metrics. The prompt-drift tripwire |

The remaining six (UPTS parser conformance, UETS emergence naming, UITS T1 encoding comprehension, UTDS and UAITS training-data governance, UBENCH LLM eval) govern surfaces pro_manage does not have.

UxTS is not a time-series layer and is not integrated at runtime; it is a build-time and CI-time concern.

## Why it exists

UxTS is a forcing function. The failure class it exists to kill: a contract lives in someone's intent, drifts silently from the implementation, and surfaces months later as production damage. It prevents three compounding failures:

- **Silent drift.** A hand-written test and the code it tests can diverge without anyone noticing. A declarative spec is a single artifact that says "this endpoint MUST return this shape", diffable, reviewable, machine-validated.
- **False confidence (0 of 0 passes).** A suite that reports green while half its assertions are never executed is worse than no suite. Schema-runner parity exists to stop this.
- **Reinvention tax.** Every new concern (security, benchmarks, dashboards) otherwise invents its own format, runner, and CI wiring.

MDEMG's own incident log is the evidence: when previously-skipped CI gates were un-zombied in one pass (UXTS-CI-001, 2026-06-11), the first clean run surfaced a stale Grafana spec, four prompt drifts, two silently-failing tests, and a matrix 51 days stale. The hidden-rot rate was not zero. The policy pin that came out of it, "do not add `continue-on-error` to test steps", is adopted here as written.

**What this means for pro_manage specifically.** The FSD is full of contracts that will otherwise rot:

| Contract named in the FSD | Who breaks silently if it drifts | Framework |
|---|---|---|
| `/api/v1/*` response shapes (`§05`) | The frontend; third-party readers of exported audit records | UATS |
| Connector-type gRPC contracts (`§05`, `§10`, `§15`) | Every module of that type; the swap-a-vendor promise | UDTS (packaged as the SDK conformance suite) |
| MDEMG JSON field shapes pro_manage pins against (`§06`) | Guidance cards, classify verdicts, fail-closed tiers | UATS against the pinned MDEMG, plus recorded ULTS prompt hashes |
| Mapping YAML and workbook templates (`§07`, `§10`, `§15`) | Metric harvests and imports that keep running on wrong data | UNTS hashes; UATS on the import endpoint with hashed fixture workbooks |
| Policy-variable keys (`§08`) | A workflow stage that no longer fires; a tier that silently drops to C | Policy-variable registry (DORMANT-CENSUS pattern) |
| Derived-metric formulas (`§07`) | Dashboards and Benefits Realization verdicts | UPMTS |
| Dashboards, alert rules, metric registry (`§11`) | On-call, who finds the missing panel during the incident | UOTS (artifacts), UOBS (live) |
| Auth and authorization boundaries (`§04`) | Contributor scoping; public share tokens | USTS; UAMS for IdPi and connector auth methods |
| SLO budgets (`§11`) | Everyone, slowly | UBTS |

## Anatomy of a spec

A UATS spec, trimmed from MDEMG's `node_get.uats.json` and re-targeted at pro_manage. This is the real shape the vendored runner executes, not a sketch:

```json
{
  "uats_version": "1.0.0",
  "api": {
    "name": "Approvals Create Workflow",
    "base_url": "${PM_BASE_URL}",
    "service": "pm-core",
    "tags": ["approvals", "car", "policy"]
  },
  "metadata": {
    "author": "reh3376",
    "created": "2026-10-14",
    "description": "POST /api/v1/projects/{project_id}/approvals creates a CAR workflow and routes stage 1 to the project owner.",
    "test_type": "contract",
    "priority": "high"
  },
  "config": {
    "timeout_ms": 10000,
    "sha256": "<computed by add-hashes>"
  },
  "variables": { "project": "proj_fixture_01" },
  "request": {
    "method": "POST",
    "path": "/api/v1/projects/{{project}}/approvals",
    "headers": { "Authorization": "Bearer ${PM_TEST_TOKEN_OWNER}" },
    "body": { "kind": "car", "subject_ref": "car_fixture_01", "amount": 50000 }
  },
  "expected": {
    "status": 201,
    "body_assertions": [
      { "path": "$.workflow_id", "op": "exists" },
      { "path": "$.current_stage.role_required", "op": "equals", "expected": "project_owner" },
      { "path": "$.policy_snapshot.car.threshold.vp_finance", "op": "exists" }
    ]
  },
  "variants": [
    {
      "name": "missing_amount_400",
      "request": { "body": { "amount": "" } },
      "expected": { "status": 400 }
    },
    {
      "name": "viewer_cannot_create_403",
      "request": { "headers": { "Authorization": "Bearer ${PM_TEST_TOKEN_VIEWER}" } },
      "expected": { "status": 403 }
    },
    {
      "name": "tier_a_denied_when_classify_blocks_409",
      "request": { "body": { "amount": 8400000, "subject_ref": "car_fixture_seeded_deny" } },
      "expected": {
        "status": 409,
        "body_assertions": [
          { "path": "$.violated_codes", "op": "exists" },
          { "path": "$.fail_tier", "op": "equals", "expected": "A" }
        ]
      }
    }
  ]
}
```

Four load-bearing invariants, all inherited from MDEMG:

1. **`<framework>_version`**: the spec-schema version. The runner refuses to execute a mismatched spec.
2. **`config.sha256`**: SHA-256 of the spec content with the `sha256` field itself removed, so the hash is a function of what the spec asserts, not what it claims. Added by `add-hashes`, verified on every run.
3. **`variants`** deep-merge over the base request. **Required-field guards use empty-string overrides, never key omission** (MDEMG pin UATS-GAP-001): an omitted key inherits the base value and the variant silently asserts nothing. The `missing_amount_400` variant above sets `"amount": ""` for that reason.
4. **`metadata.author` and `created`**: provenance for drift triage.

Note on fixtures: MDEMG's UATS runner does not implement `setup` or `teardown` (it fails hard on them, per parity). The seeded Tier A deny in the third variant therefore comes from the CI stack's fixture load (a governance constraint seeded into the pinned MDEMG before the suite runs), referenced by `subject_ref`, not from a `setup` block in the spec.

## How a runner executes, and the hash contract

```
load spec → verify sha256 → substitute {{variables}} and ${ENV_VARS}
  → compose base + each variant
    → send request
      → compare status
        → walk body_assertions (JSONPath against the response)
          → CheckResult per assertion → aggregate pass/fail/error/skip → write report, exit non-zero on fail
```

Every runner has at least four subcommands: `validate` (one spec, live), `validate-all` (whole directory), `add-hashes` (compute and persist `config.sha256`), `verify-hashes` (the CI gate; refuses merge if any spec was edited without re-hashing).

The merge gate works like this: a developer edits a spec; CI runs `verify-hashes`; the recomputed hash mismatches the stored one; CI fails; the developer runs `add-hashes` locally and commits the regenerated hash **in the same PR**. "I changed the contract" becomes an explicit, reviewable commit rather than an invisible edit. Same shape as a signed-off-by line.

**UNTS** sits above the per-spec hashes: a registry (`docs/specs/unts-registry.json`) that scans every framework's specs and every fixture file (workbook templates, mapping YAML, UDTS proto hashes, golden responses), keeps the last three hashes per artifact with a source reference, and drives the revert workflow. UNTS is invoked by the other verifiers rather than being a gate of its own.

## pro_manage framework matrix (planned)

The matrix lives at `docs/tests/UXTS_FRAMEWORK_MATRIX.md` once code exists; this table is its seed. "Merge-blocking" means the PR cannot merge; "soft-fail" means the step reports but does not block; "live-gated" means the check runs against a real stack on demand and gates a specific change class rather than every PR.

| Framework | pro_manage scope | Specs live at | Runner | CI disposition | Phase |
|---|---|---|---|---|---|
| **UATS** | Every `/api/v1/*` endpoint; connector webhooks; the import and export endpoints with hashed fixture workbooks; **the MDEMG adapter surface** (specs that pin the `guidance[]`, classify, feedback, and observe field shapes `§06` depends on, run against the pinned MDEMG) | `docs/api-spec/uats/specs/` | vendored `uats_runner.py` | merge-blocking (`validate-all` against a compose-local stack) | 0 |
| **UDTS** | `pm.core.v1.*`, `pm.sidecar.v1.*`, `pm.connector.v1.*` (one spec per RPC, `proto_sha256` pinned); each connector type's specs are packaged as the SDK conformance suite (`§15`) | `docs/api-spec/udts/specs/` | Go per-RPC tests (`tests/udts/`), as upstream | merge-blocking; nightly job against each registered sandbox connector instance | 0 |
| **UNTS** | Registry over all frameworks' specs and fixtures, mapping YAML, workbook templates, proto hashes | `docs/specs/unts-registry.json` | `make unts-update`, `make test-unts` | invoked by the other verifiers; clean state required on every PR | 0 |
| **UOTS** | Grafana dashboards, Alertmanager rule files, the Prometheus metric registry, backup manifests (`§11`) | `docs/api-spec/uots/specs/` | vendored `uots_runner.py` | merge-blocking | 1 |
| **USTS** | Every protected endpoint rejects unauthenticated and wrong-role callers; contributor scoping (`§04`); public share-token exposure (`§09`); injection; rate limiting | `docs/tests/usts/specs/` | vendored `usts_runner.py` | merge-blocking before pilot | 1 |
| **UOBS** | `/healthz` and dependency checks on every service; trace-header propagation across pm-core, connectors, sidecar; metric presence over the wire | `docs/tests/uobs/specs/` | vendored `uobs_runner.py` | soft-fail | 2 |
| **UBTS** | The `§11` SLO table as fixed `p99_ms` baselines: dashboard tiles, Tier A and B submit, classify, harvest batch, import validation | `docs/tests/ubts/specs/` + profiles | vendored `ubts_runner.py` | soft-fail smoke on PR; full profile nightly | 2 |
| **UAMS** | IdPi auth methods (OIDC, SAML), connector auth helpers (OAuth2 client credentials, API key, signed webhooks), break-glass local admin | `docs/tests/uams/specs/` | spec-only until the upstream Go runner ships; USTS consumes the specs meanwhile | none until runner exists | 2 |
| **UPMTS** | pro_manage-specific: derived-metric formula contracts (see below) | `docs/tests/upmts/specs/` | pro_manage-authored runner | merge-blocking | 3 |
| **UVTS** (candidate) | Guidance relevance for the flagship flow: a graded corpus of design-screen queries with the MP information each should surface, A/B gated on governance-space changes | `docs/tests/uvts/specs/` | vendored `uvts_runner.py`, re-targeted | live-gated | 3 |
| **ULTS** | Not authored here (pro_manage owns no prompts). pro_manage **records** the `system_prompt_hash` of the MDEMG tasks it consumes (`jiminy.classify_outcome`, `jiminy.synthesize`, `jiminy.evaluate_llm`) in its MDEMG version pin; a hash change forces a re-baseline of the adapter UATS fixtures | in the version pin | n/a | drift check on pin update | 0 |
| UPTS, UETS, UITS, UTDS, UAITS, UBENCH | Not applicable: no parsers, no emergence naming, no T1 encoding, no training data, no LLM eval in pro_manage | | | | |

### UPMTS: Universal Project-Metric Test Specification (pro_manage-specific)

MDEMG's rule is that a new framework is a last resort, minted only when all three hold: the contract shape is generic (at least three instances within a quarter), it has structural invariants that benefit from a schema (not just a hash), and it has a runner that executes against the real system. Derived metrics pass all three: dozens of formulas (`budget.variance_pct`, CPI, SPI, EAC, OEE, priced loss, project-loss rates) from Phase 3; each has inputs, a formula, units, a tolerance, and a scope; and the runner evaluates the formula through pm-sidecar against fixture inputs and compares to the expected value within tolerance.

A UPMTS spec states: "metric `budget.variance_pct` at project scope is computed from `budget.actual.last`, `budget.committed.last`, and the current `BudgetBaseline.baseline_total` via the stated formula, within tolerance ε." The runner reads metric definitions, pushes the fixture inputs, and verifies the compute path. It catches silent drift in a formula when inputs or math change.

Before UPMTS exists (Phases 0 to 2), the few derived metrics that ship are pinned with Go pin tests (MDEMG's cheaper option 2), which is the right size for a handful of single-file contracts.

## Companion adjudication registries (DORMANT-CENSUS pattern)

UxTS asks "does the implementation match a frozen contract?" MDEMG's sibling registries ask a different question: "does every shipped declaration have at least one consumer?" Each is a JSON inventory plus a grep-based verifier plus a merge-blocking CI step; adding a declaration without adjudicating it (writers, readers, disposition, notes) fails CI. pro_manage adopts the pattern for five declaration surfaces where silent orphaning is the realistic failure:

| Registry | Declaration surface | Why it matters here |
|---|---|---|
| `policy_variables_inventory.json` | Every `PolicyVariable` key (`§08`) | A key with no consumer is a knob that does nothing; a consumer with no key is a hard-coded threshold creeping back in |
| `connector_capabilities_inventory.json` | Every capability a connector manifest can declare (`§15`) | A capability pm-core never routes on is a false promise in the admin UI |
| `metric_definitions_inventory.json` | Every metric id (`§07`) | A metric no dashboard, workflow, or promise reads is harvest cost with no reader |
| `routes_inventory.json` | Every REST route | As upstream: dead routes are attack surface |
| `loss_categories_inventory.json` | Every loss and project-loss category (`§03`, `§14`) | A category nothing can price or report is noise on the Pareto |

These are not UxTS specs and are deliberately not shoehorned into one; the shape is per-family aggregate, not per-contract.

## When to add a framework

Inherited from MDEMG, in order:

1. **Add a spec to an existing framework.** Another endpoint is a UATS spec, not a new framework.
2. **Add a Go pin test** (`internal/<name>pin/*_test.go`) for a single-file content pin: static config, embedded JSON, a compose manifest. Zero new surface.
3. **Add a DORMANT-CENSUS-style registry** for a new "every declaration needs a consumer" invariant.

Mint a new UxTS framework only when the contract is generic (three or more instances within a quarter), has structural invariants worth a schema, and has a runner that executes against the real system. UPMTS is the only pro_manage-minted framework planned. Compose and Kubernetes manifests (Phase 4) are pinned via option 2, not a new framework; MDEMG itself lists K8s/Helm as an open UOTS gap rather than a solved one.

## CI integration

- `make test-uats`, `make test-udts`, `make test-uots`, `make test-usts`: merge-blocking on every PR against a compose-local stack with the pinned MDEMG and seeded fixtures.
- `make test-unts`: clean hash state required on every PR.
- `make test-ubts-smoke`, `make test-uobs`: soft-fail on PR; full runs nightly against staging.
- `make test-upmts`: merge-blocking from Phase 3.
- `make test-all`: the above plus Go, Python, and TypeScript unit tests and the Next.js build.
- `make verify-inventories`: the adjudication registries, merge-blocking.
- No test step carries `continue-on-error`. A step that cannot run in CI (UVTS live-gated) is absent from CI and documented as such in the matrix, never present and ignored.
- The framework matrix carries a "last verified" date; a doc-currency check fails CI when the matrix is more than 30 days older than the newest spec.

## Developer experience

- A new API endpoint ships with its UATS spec in the same PR. A new gRPC method ships with its UDTS spec and `proto_sha256`.
- A new policy variable, connector capability, metric id, route, or loss category ships with its inventory entry in the same PR, or the pre-commit hook blocks the commit (MDEMG's inventory gate, ported).
- Changing a spec requires `add-hashes` and the UNTS re-hash in the same PR; the reviewer sees the hash change explicitly.
- Running the merge-blocking set locally takes under 2 minutes for the MVP footprint; UATS parallelizes in CI.

## Why not framework X instead

- **vs. Postman collections:** Postman is interactive and GUI-first; UxTS specs are code-reviewable, hashed, and CI-first.
- **vs. OpenAPI contract tests alone:** OpenAPI validates shape; UATS validates behaviour (status codes, side effects, state transitions) against fixtures, and the hash makes a contract change an explicit act.
- **vs. standard pytest and table tests:** those remain for unit tests. UxTS is for cross-cutting contracts governed by schema, runner parity, and hash integrity.

## Open items for Section 12

- Confirm the vendored runner version pins (`uats_runner.py` v1.2.0, `ubts_runner.py` v1.1.0, and the others at MDEMG's tag at FSD ratification) with a periodic re-vendor as upstream matures.
- Decide whether UDTS fixtures include golden response bytes or structural assertions only (first instinct: structural for most RPCs, golden for the SDK conformance suite so vendor modules are compared byte-for-byte on reference fixtures).
- Decide how the CI stack seeds MDEMG fixtures (governance constraints, project spaces) given that UATS `setup` is unimplemented upstream: a fixture-load step before `validate-all` is the first instinct.
- Confirm whether MDEMG's UAMS Go runner lands before pro_manage Phase 2; if not, decide whether pro_manage writes it (and contributes it upstream) or stays spec-only.
- Confirm whether the UVTS candidate (guidance relevance for the MP information flow) is worth a graded corpus in Phase 3, and who grades it (first instinct: the Maintenance and Operations leads who author the MP information).
- Confirm whether UATS specs are published externally (helpful if third-party consumers of pro_manage's REST ever emerge). First instinct: internal only in year one.
- Decide the owner group for UPMTS (first instinct: the metrics team with Finance for CPI, SPI, and EAC definitions).
