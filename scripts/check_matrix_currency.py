#!/usr/bin/env python3
"""Fail when the UxTS framework matrix is stale (FSD §12 doc-currency check).

The matrix's "Last verified: YYYY-MM-DD" line must be no more than MAX_AGE_DAYS
older than the newest spec, schema, or inventory file (by git commit date, or
mtime when not in git). MDEMG learned this the hard way: a 51-day-stale matrix.
"""

from __future__ import annotations

import re
import subprocess
import sys
from datetime import date, datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
MATRIX = ROOT / "docs" / "tests" / "UXTS_FRAMEWORK_MATRIX.md"
MAX_AGE_DAYS = 30
WATCH = ["docs/tests/*/specs/**/*.json", "docs/tests/*/schema/*.json", "docs/inventories/*.json"]


def newest_change() -> date:
    newest = date(1970, 1, 1)
    for g in WATCH:
        for p in ROOT.glob(g):
            if not p.is_file():
                continue
            try:
                out = subprocess.run(["git", "log", "-1", "--format=%cs", "--", str(p)], cwd=ROOT, capture_output=True, text=True, check=True).stdout.strip()
                d = date.fromisoformat(out) if out else datetime.fromtimestamp(p.stat().st_mtime).date()
            except Exception:
                d = datetime.fromtimestamp(p.stat().st_mtime).date()
            newest = max(newest, d)
    return newest


def main() -> int:
    text = MATRIX.read_text()
    m = re.search(r"Last verified:\s*(\d{4}-\d{2}-\d{2})", text)
    if not m:
        print("matrix: no 'Last verified: YYYY-MM-DD' line")
        return 1
    verified = date.fromisoformat(m.group(1))
    newest = newest_change()
    age = (newest - verified).days
    if age > MAX_AGE_DAYS:
        print(f"matrix: STALE. Last verified {verified}, newest governed change {newest} ({age} days later, limit {MAX_AGE_DAYS}). Review the matrix and bump the date.")
        return 1
    print(f"matrix: ok (last verified {verified}, newest governed change {newest})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
