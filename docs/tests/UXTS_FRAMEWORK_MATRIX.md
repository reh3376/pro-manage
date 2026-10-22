# UxTS Framework Matrix (pro_manage)

Purpose: the canonical map of each UxTS framework pro_manage runs to its schema, specs, runner, CI disposition, and status (FSD §12). If this file and a section of the FSD disagree, this file wins for the operational state and the FSD is corrected.

Last verified: 2026-10-22 (slice 0b). Runners vendored from MDEMG `c1f14f74` (`v0.11.0-beta.3`), see `VENDORED.md`. `scripts/check_matrix_currency.py` fails CI when this date is more than 30 days older than the newest spec.

## 1) Framework inventory

| Framework | Name | Status | Specs | CI disposition |
|---|---|---|---|---|
| UATS | Universal API Test Specification | **active** | 6 service specs (pm-core 3, pm-workers 1, pm-sidecar 1, pm-frontend 1), 5 stub-mdemg specs, 5 live mdemg specs | **merge-blocking**: `verify-hashes` on every PR; `validate-all` for the service and stub-mdemg sets inside the compose smoke job. Live mdemg set is **live-gated**: `make test-uats-live` on the workstation before merging anything that touches `pkg/mdemgadapter` |
| UDTS | Universal DevSpace Test Specification | planned (slice 1) | 0 | merge-blocking once `pm.core.v1` has a served RPC |
| UNTS | Universal Hash Test Specification | **active** | registry (`docs/specs/unts-registry.json`) | **merge-blocking**: `scripts/unts.py verify` on every PR |
| UOTS | Universal Observability Test Specification | vendored, no specs | 0 | merge-blocking from Phase 1 (dashboards, alert rules, metric registry) |
| USTS | Universal Security Test Specification | vendored, no specs | 0 | merge-blocking before the pilot (auth boundaries, contributor scoping, share tokens) |
| UOBS | Universal Observability Specification | not vendored | 0 | soft-fail, Phase 2 |
| UBTS | Universal Benchmark Test Specification | not vendored | 0 | soft-fail, Phase 2 |
| UAMS | Universal Auth Method Specification | not vendored | 0 | spec-only until an upstream runner exists |
| UPMTS | Universal Project-Metric Test Specification (pro_manage-minted) | not started | 0 | merge-blocking, Phase 3 |
| UVTS | Universal Validation Test Specification | not vendored | 0 | live-gated candidate, Phase 3 |

## 2) Source of truth by framework

| Framework | Schema | Specs | Runner | Invocation |
|---|---|---|---|---|
| UATS | `docs/tests/uats/schema/uats.schema.json` | `docs/tests/uats/specs/<target>/*.uats.json` | `docs/tests/uats/runners/uats_runner.py` v1.2.0 | `make test-uats` (stack), `make test-uats-live` (MDEMG), `make test-uxts-hashes` |
| UNTS | n/a (registry) | `docs/specs/unts-registry.json` | `scripts/unts.py` | `make test-unts`, `make unts-update` |
| UOTS | `docs/tests/uots/schema/uots.schema.json` | `docs/tests/uots/specs/` | `docs/tests/uots/runners/uots_runner.py` | `make test-uots` (once specs exist) |
| USTS | `docs/tests/usts/schema/usts.schema.json` | `docs/tests/usts/specs/` | `docs/tests/usts/runners/usts_runner.py` | `make test-usts` (once specs exist) |

## 3) Schema-runner parity

Inherited from MDEMG for the vendored runners (UATS hard-fails on `setup`, `teardown`, `chain`, `body_file`, `body_schema`, `oauth2`; UOTS on `log_format`, `trace_propagation`; USTS on `authentication`, `test_cases` format). pro_manage specs do not use any unimplemented field; CI's `verify-hashes` plus `validate-all` is the parity gate until a pro_manage-authored runner exists (UPMTS).

## 4) Companion adjudication registries (DORMANT-CENSUS pattern)

| Registry | Inventory | Scanner | CI |
|---|---|---|---|
| routes | `docs/inventories/routes_inventory.json` | Go `HandleFunc("METHOD /path")`, Next.js `app/**/route.ts`; Python routes adjudicated manually | **merge-blocking** (`scripts/verify_inventories.py`) and pre-commit |
| policy_variables | `docs/inventories/policy_variables_inventory.json` | `policyvar.Define("key")`, seed SQL | same (no declarations yet) |
| connector_capabilities | `docs/inventories/connector_capabilities_inventory.json` | `connectors/*/connector.yaml` | same (no declarations yet) |
| metric_definitions | `docs/inventories/metric_definitions_inventory.json` | `internal/metrics/definitions/*.yaml` | same (no declarations yet) |
| loss_categories | `docs/inventories/loss_categories_inventory.json` | `db/seeds/*.sql` | same (no declarations yet) |

## 5) Gaps and next steps

1. UDTS: no served RPC yet; `pm.core.v1.VersionService` is declared in proto only. First UDTS spec lands with slice 1.
2. UOTS and USTS: runners vendored, spec directories empty; gated from Phase 1 per the FSD.
3. The live MDEMG UATS set has not been run in CI by design (no model there). It must pass on the workstation before a PR that changes the adapter is merged; the PR description states the run date.
4. The `requests` and `jsonpath-ng` runtime dependencies are supplied ad hoc by `uv run --with`; if a fourth framework arrives, give `docs/tests` its own `pyproject.toml`.

## 6) References

- FSD §12 Test governance
- `docs/tests/VENDORED.md`
- MDEMG `docs/development/UXTS_FRAMEWORK_MATRIX.md` (upstream, at `c1f14f74`)
