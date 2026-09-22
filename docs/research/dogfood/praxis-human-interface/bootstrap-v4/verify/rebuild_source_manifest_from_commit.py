#!/usr/bin/env python3
"""Re-hash the exact bootstrap implementation source inventory, for a candidate
that has been committed (working tree clean).

`rebuild_source_manifest.py` computes the inventory from `git status
--porcelain -uall`: every changed or untracked non-evidence file of the WORKING
TREE relative to whatever commit is checked out. That is exactly right for an
uncommitted review candidate, and wrong once the candidate is committed: the
working tree is then clean by construction and the status-based method would
(correctly, but uselessly) report zero files.

This script computes the same inventory -- same exclusions, same schema -- from
git history instead: every non-evidence file that differs between the pristine
pre-PRE-V4 base commit and HEAD (`git diff --name-only <base> HEAD`). The
content measured is unchanged; only where the diff is read from changes,
because there is no longer an uncommitted diff to read from status.

  python3 rebuild_source_manifest_from_commit.py <base-commit>
"""

import hashlib
import json
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[6]
TARGET = ROOT / "docs/research/dogfood/praxis-human-interface/bootstrap-v4/source-manifest.json"
EXCLUDED_PREFIXES = (
    "docs/research/dogfood/praxis-human-interface/bootstrap-v4/",
    "docs/research/dogfood/praxis-human-interface/reviews/",
)

if len(sys.argv) != 2:
    raise SystemExit("usage: rebuild_source_manifest_from_commit.py <base-commit>")
base = sys.argv[1]

status = subprocess.check_output(["git", "status", "--porcelain", "-uall"], cwd=ROOT, text=True)
if status.strip():
    raise SystemExit("working tree is not clean; use rebuild_source_manifest.py for an uncommitted candidate:\n" + status)

manifest = json.loads(TARGET.read_text())
diff = subprocess.check_output(["git", "diff", "--name-only", base, "HEAD"], cwd=ROOT, text=True).splitlines()
paths = set()
for path in diff:
    path = path.strip()
    if not path or path.startswith(EXCLUDED_PREFIXES) or not (ROOT / path).is_file():
        continue
    if path == "praxis" or path.endswith(".test"):
        raise SystemExit("unexpected build artifact in the diff: " + path)
    paths.add(path)
manifest["files"] = [
    {"path": path, "sha256": hashlib.sha256((ROOT / path).read_bytes()).hexdigest()}
    for path in sorted(paths)
]
TARGET.write_text(json.dumps(manifest, indent=2) + "\n")
print(len(paths), "files;", hashlib.sha256(TARGET.read_bytes()).hexdigest())
