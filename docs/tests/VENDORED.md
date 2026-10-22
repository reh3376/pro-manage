# Vendored UxTS runners

The UxTS runner primitives and reference runners in this directory are copied from the MDEMG repository (MIT License, Copyright (c) 2026 Roger Henley), per FSD §12: pro_manage takes no runtime dependency on MDEMG for test governance.

| File here | Source in MDEMG | Local change |
|---|---|---|
| `uxts_runner_core.py` | `docs/tests/uxts_runner_core.py` | none |
| `uxts_report.py` | `docs/tests/uxts_report.py` | none |
| `uats/runners/uats_runner.py` | `docs/api/api-spec/uats/runners/uats_runner.py` (v1.2.0) | `sys.path` insert points at `../..` (this repo keeps the core next to the framework directories) |
| `uats/schema/uats.schema.json` | `docs/api/api-spec/uats/schema/uats.schema.json` | none |
| `uots/runners/uots_runner.py` | `docs/api/api-spec/uots/runners/uots_runner.py` | `sys.path` insert points at `../..`; `ROOT` is `parents[3]` (repo root from `docs/tests/uots/runners`) |
| `uots/schema/uots.schema.json` | `docs/api/api-spec/uots/schema/uots.schema.json` | none |
| `usts/runners/usts_runner.py` | `docs/tests/usts/runners/usts_runner.py` | none |
| `usts/schema/usts.schema.json` | `docs/tests/usts/schema/usts.schema.json` | none |

**Source pin:** MDEMG commit `c1f14f74` (tag `v0.11.0-beta.3`, 2026-10-15), the same pin as the dedicated MDEMG dev stack.

**Runtime dependencies:** `requests` and `jsonpath-ng` (the runners are invoked through `uv run --no-project --with requests --with jsonpath-ng`, see the Makefile), nothing else.

## Re-vendoring

Deliberate, on MDEMG tags, never silently:

```bash
C=<new mdemg commit>
for f in docs/tests/uxts_runner_core.py docs/tests/uxts_report.py; do git -C ~/mdemg show $C:$f > docs/tests/$(basename $f); done
git -C ~/mdemg show $C:docs/api/api-spec/uats/runners/uats_runner.py > docs/tests/uats/runners/uats_runner.py
# ... remaining files per the table, then re-apply the local changes above, update this file's pin,
# run `make test-uxts-hashes` and `make unts-update`, and note the re-vendor in the PR.
```

The framework matrix (`UXTS_FRAMEWORK_MATRIX.md`) records the pin too; the two must agree.
