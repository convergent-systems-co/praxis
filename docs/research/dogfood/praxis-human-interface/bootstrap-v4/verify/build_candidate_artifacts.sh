#!/bin/bash
# Builds the three PRE-V4 candidate artifacts (core, goals plugin, goals
# package archive+manifest) with a build-location-independent recipe.
#
# Review #9 (N18): builds of this candidate without -trimpath embed the
# absolute checkout path in compiled bytes (measured true even with cgo
# enabled, i.e. -trimpath alone is sufficient here; no CGO_CFLAGS path-remap
# was needed). A build in one checkout directory therefore did not reproduce
# byte-identically in a fresh clone at a different path, even though the
# source and GOCACHE were otherwise identical. This script is the single,
# versioned build recipe so "rebuild the candidate" means exactly one thing,
# checkable at any commit, from any checkout path.
#
# Usage: verify/build_candidate_artifacts.sh <output-dir>
#   <output-dir>/praxis-candidate
#   <output-dir>/praxis-goals-plugin-candidate
#   <output-dir>/package/praxis-package.json
#   <output-dir>/package/praxis-package.tar.gz
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <output-dir>" >&2
  exit 2
fi
OUT="$1"
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/../../../../../.." && pwd)"

mkdir -p "$OUT/package"
cd "$ROOT"
go build -trimpath -o "$OUT/praxis-candidate" ./cmd/praxis
go build -trimpath -o "$OUT/praxis-goals-plugin-candidate" ./packages/goals/plugin
go run "$HERE/build_goals_package.go" "$OUT/praxis-goals-plugin-candidate" "$OUT/package"

echo "core:            $(shasum -a 256 "$OUT/praxis-candidate" | cut -d' ' -f1)"
echo "plugin:          $(shasum -a 256 "$OUT/praxis-goals-plugin-candidate" | cut -d' ' -f1)"
echo "package archive: $(shasum -a 256 "$OUT/package/praxis-package.tar.gz" | cut -d' ' -f1)"
echo "package manifest:$(shasum -a 256 "$OUT/package/praxis-package.json" | cut -d' ' -f1)"
