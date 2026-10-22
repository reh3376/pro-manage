#!/usr/bin/env python3
"""Adjudication registries in MDEMG's DORMANT-CENSUS pattern (FSD §12).

Every shipped declaration on five surfaces must have an adjudicated inventory
entry (writers, readers, disposition, notes), and every scanned inventory entry
must still exist in the source tree. Merge-blocking in CI; also run by the
pre-commit hook.

  scripts/verify_inventories.py            # verify (exit 1 on any gap)
  scripts/verify_inventories.py --scan     # print what the scanners find (to seed new entries)

Surfaces and their declaration scanners:
  routes                HTTP routes: Go `mux.HandleFunc("METHOD /path"...)` and Next.js `app/**/route.ts`
  policy_variables      PolicyVariable keys declared in Go (`policyvar.Define("key"`) or migration seeds
  connector_capabilities Capability names declared in connector manifests (`connectors/*/connector.yaml`)
  metric_definitions    Metric ids declared in `services/pm-core/internal/metrics/definitions/*.yaml`
  loss_categories       Loss and project-loss category ids in `services/pm-core/db/seeds/*.sql`

Scanners for surfaces that have no declarations yet return an empty set; the
verifier then only checks that the inventory has no scanned entries pointing at
nothing. Entries with "source": "manual" are not required to be found by a
scanner (used for routes in languages the scanner does not read, e.g. the
Python sidecar).
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
INV = ROOT / "docs" / "inventories"
REQUIRED_FIELDS = {"writers", "readers", "disposition", "notes"}
DISPOSITIONS = {"active", "planned", "deprecated", "internal"}


def _read(path: Path) -> str:
    try:
        return path.read_text(encoding="utf-8")
    except UnicodeDecodeError:
        return ""


def scan_routes() -> dict[str, str]:
    found: dict[str, str] = {}
    # Go 1.22+ method-qualified patterns on mux.Handle(...) and mux.HandleFunc(...).
    go_re = re.compile(r'\.Handle(?:Func)?\(\s*"([A-Z]+)\s+(/[^"\s]*)"')
    for p in (ROOT / "services").rglob("*.go"):
        if p.name.endswith("_test.go"):
            continue
        service = p.relative_to(ROOT).parts[2] if len(p.relative_to(ROOT).parts) > 3 and p.relative_to(ROOT).parts[2] == "cmd" else None
        parts = p.relative_to(ROOT).parts
        binary = parts[3] if len(parts) > 3 and parts[2] == "cmd" else parts[1]
        for m in go_re.finditer(_read(p)):
            found[f"{binary} {m.group(1)} {m.group(2)}"] = str(p.relative_to(ROOT))
    app = ROOT / "apps" / "pm-frontend" / "app"
    for p in app.rglob("route.ts"):
        rel = p.relative_to(app).parent
        route = "/" + "/".join(seg for seg in rel.parts if not seg.startswith("("))
        text = _read(p)
        for method in re.findall(r"export\s+(?:async\s+)?function\s+(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\b", text):
            found[f"pm-frontend {method} {route}"] = str(p.relative_to(ROOT))
    return found


def scan_policy_variables() -> dict[str, str]:
    found: dict[str, str] = {}
    pat = re.compile(r'policyvar\.Define\(\s*"([a-z0-9_.<>]+)"')
    for p in (ROOT / "services" / "pm-core").rglob("*.go"):
        for m in pat.finditer(_read(p)):
            found[m.group(1)] = str(p.relative_to(ROOT))
    # Seed SQL: every tuple of an INSERT INTO policy_variables statement (multi-row inserts included).
    # Statement runs to ON CONFLICT or the next INSERT (descriptions may contain semicolons).
    stmt_pat = re.compile(r"INSERT INTO policy_variables\b.*?(?=ON CONFLICT|INSERT INTO|\Z)", re.I | re.S)
    tuple_pat = re.compile(r"\(\s*'([a-z0-9_.<>]+)'\s*,")
    for p in (ROOT / "services" / "pm-core").rglob("*.sql"):
        for stmt in stmt_pat.finditer(_read(p)):
            for m in tuple_pat.finditer(stmt.group(0)):
                found[m.group(1)] = str(p.relative_to(ROOT))
    return found


def scan_connector_capabilities() -> dict[str, str]:
    found: dict[str, str] = {}
    for p in (ROOT / "connectors").glob("*/connector.yaml"):
        text = _read(p)
        block = re.search(r"^capabilities:\s*\n((?:\s+-\s+.*\n?)+)", text, re.M)
        if block:
            for cap in re.findall(r"-\s+([a-z0-9_]+)", block.group(1)):
                found[f"{p.parent.name}:{cap}"] = str(p.relative_to(ROOT))
    return found


def scan_metric_definitions() -> dict[str, str]:
    found: dict[str, str] = {}
    for p in (ROOT / "services" / "pm-core" / "internal" / "metrics" / "definitions").glob("*.yaml"):
        for m in re.finditer(r"^\s*-\s*id:\s*([a-z0-9_.]+)", _read(p), re.M):
            found[m.group(1)] = str(p.relative_to(ROOT))
    return found


def scan_loss_categories() -> dict[str, str]:
    found: dict[str, str] = {}
    pat = re.compile(r"INSERT INTO (?:project_)?loss_categories[^;]*?VALUES\s*\(\s*'([a-z0-9_]+)'", re.I | re.S)
    for p in (ROOT / "services" / "pm-core" / "db" / "seeds").glob("*.sql"):
        for m in pat.finditer(_read(p)):
            found[m.group(1)] = str(p.relative_to(ROOT))
    return found


SURFACES = {
    "routes": scan_routes,
    "policy_variables": scan_policy_variables,
    "connector_capabilities": scan_connector_capabilities,
    "metric_definitions": scan_metric_definitions,
    "loss_categories": scan_loss_categories,
}


def load_inventory(name: str) -> dict:
    path = INV / f"{name}_inventory.json"
    if not path.exists():
        return {"surface": name, "entries": {}}
    return json.loads(path.read_text())


def verify() -> int:
    problems: list[str] = []
    for name, scanner in SURFACES.items():
        inv = load_inventory(name)
        entries: dict = inv.get("entries", {})
        found = scanner()
        for key, where in sorted(found.items()):
            if key not in entries:
                problems.append(f"[{name}] declared but not adjudicated: {key}  ({where})")
        for key, entry in sorted(entries.items()):
            missing = REQUIRED_FIELDS - set(entry)
            if missing:
                problems.append(f"[{name}] entry {key} lacks fields: {sorted(missing)}")
            if entry.get("disposition") not in DISPOSITIONS:
                problems.append(f"[{name}] entry {key} has invalid disposition {entry.get('disposition')!r}")
            if entry.get("source", "scanned") == "scanned" and key not in found:
                problems.append(f"[{name}] adjudicated but no longer declared: {key}")
            if not entry.get("readers") and entry.get("disposition") == "active":
                problems.append(f"[{name}] active entry {key} has no readers (dormant?)")
        print(f"{name}: {len(found)} declared, {len(entries)} adjudicated")
    if problems:
        print("inventories: FAILED")
        for p in problems:
            print("  " + p)
        return 1
    print("inventories: ok")
    return 0


def scan() -> int:
    for name, scanner in SURFACES.items():
        print(f"== {name}")
        for key, where in sorted(scanner().items()):
            print(f"  {key}    <- {where}")
    return 0


if __name__ == "__main__":
    sys.exit(scan() if "--scan" in sys.argv[1:] else verify())
