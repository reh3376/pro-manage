#!/usr/bin/env python3
"""UNTS: hash integrity registry for UxTS specs, schemas, fixtures, and runners (FSD §12).

Usage:
  scripts/unts.py update   # recompute hashes, keep the last 3 per artifact, write the registry
  scripts/unts.py verify   # exit 1 if any tracked artifact changed, appeared, or disappeared

The registry is docs/specs/unts-registry.json, committed to the repo. Changing a
governed artifact without running `update` fails CI; the diff of the registry in
the PR is the reviewer's signal that a contract changed.
"""

from __future__ import annotations

import hashlib
import json
import sys
from datetime import UTC, datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
REGISTRY = ROOT / "docs" / "specs" / "unts-registry.json"

# Everything UNTS governs. Globs are relative to the repo root.
TRACKED_GLOBS = [
    "docs/tests/uxts_runner_core.py",
    "docs/tests/uxts_report.py",
    "docs/tests/*/schema/*.json",
    "docs/tests/*/runners/*.py",
    "docs/tests/*/specs/**/*.json",
    "docs/tests/*/fixtures/**/*",
    "docs/inventories/*.json",
    "proto/**/*.proto",
]
HISTORY = 3


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def tracked_files() -> list[Path]:
    seen: set[Path] = set()
    for g in TRACKED_GLOBS:
        for p in ROOT.glob(g):
            if p.is_file() and "__pycache__" not in p.parts:
                seen.add(p)
    return sorted(seen)


def load_registry() -> dict:
    if REGISTRY.exists():
        return json.loads(REGISTRY.read_text())
    return {"unts_version": "1.0.0", "artifacts": {}}


def update() -> int:
    reg = load_registry()
    artifacts: dict = reg.get("artifacts", {})
    now = datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
    current = {str(p.relative_to(ROOT)): sha256_file(p) for p in tracked_files()}
    changed = 0
    for rel, digest in current.items():
        entry = artifacts.get(rel, {"sha256": None, "history": []})
        if entry["sha256"] != digest:
            if entry["sha256"]:
                entry["history"] = ([{"sha256": entry["sha256"], "recorded_at": entry.get("recorded_at")}] + entry["history"])[:HISTORY]
            entry["sha256"] = digest
            entry["recorded_at"] = now
            changed += 1
        artifacts[rel] = entry
    removed = [rel for rel in artifacts if rel not in current]
    for rel in removed:
        del artifacts[rel]
    reg["artifacts"] = dict(sorted(artifacts.items()))
    reg["updated_at"] = now
    reg["count"] = len(artifacts)
    REGISTRY.parent.mkdir(parents=True, exist_ok=True)
    REGISTRY.write_text(json.dumps(reg, indent=2) + "\n")
    print(f"unts: {len(artifacts)} artifacts tracked, {changed} updated, {len(removed)} removed -> {REGISTRY.relative_to(ROOT)}")
    return 0


def verify() -> int:
    reg = load_registry()
    artifacts: dict = reg.get("artifacts", {})
    current = {str(p.relative_to(ROOT)): sha256_file(p) for p in tracked_files()}
    problems: list[str] = []
    for rel, digest in current.items():
        if rel not in artifacts:
            problems.append(f"untracked: {rel} (run scripts/unts.py update)")
        elif artifacts[rel]["sha256"] != digest:
            problems.append(f"changed:   {rel}")
    for rel in artifacts:
        if rel not in current:
            problems.append(f"missing:   {rel} (tracked but not on disk)")
    if problems:
        print("unts: FAILED")
        for p in problems:
            print("  " + p)
        print("  -> review the change, then run `scripts/unts.py update` and commit the registry in the same PR")
        return 1
    print(f"unts: ok ({len(current)} artifacts match the registry)")
    return 0


def main(argv: list[str]) -> int:
    if len(argv) != 2 or argv[1] not in {"update", "verify"}:
        print(__doc__)
        return 2
    return update() if argv[1] == "update" else verify()


if __name__ == "__main__":
    sys.exit(main(sys.argv))
