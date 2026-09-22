#!/usr/bin/env python3
"""Re-hash the exact bootstrap implementation source inventory.

The inventory is every changed or untracked NON-EVIDENCE file of the working
tree relative to the base commit (`git status --porcelain -uall`), so it cannot
silently omit a new source or test file. Evidence directories (bootstrap-v4/
and reviews/) and build artifacts are excluded by construction.
"""

import hashlib
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[6]
TARGET = ROOT / "docs/research/dogfood/praxis-human-interface/bootstrap-v4/source-manifest.json"
EXCLUDED_PREFIXES = (
    "docs/research/dogfood/praxis-human-interface/bootstrap-v4/",
    "docs/research/dogfood/praxis-human-interface/reviews/",
)

manifest = json.loads(TARGET.read_text())
status = subprocess.check_output(["git", "status", "--porcelain", "-uall"], cwd=ROOT, text=True).splitlines()
paths = set()
for line in status:
    path = line[3:].strip()
    if " -> " in path:
        path = path.split(" -> ", 1)[1]
    if path.startswith(EXCLUDED_PREFIXES) or not (ROOT / path).is_file():
        continue
    if path == "praxis" or path.endswith(".test"):
        raise SystemExit("unexpected build artifact in the working tree: " + path)
    paths.add(path)
manifest["files"] = [
    {"path": path, "sha256": hashlib.sha256((ROOT / path).read_bytes()).hexdigest()}
    for path in sorted(paths)
]
TARGET.write_text(json.dumps(manifest, indent=2) + "\n")
print(len(paths), "files;", hashlib.sha256(TARGET.read_bytes()).hexdigest())
