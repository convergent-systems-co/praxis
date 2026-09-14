#!/usr/bin/env bash
set -euo pipefail

version="${1:-v2.0.0}"
version="${version#v}"
out_dir="${2:-dist/${version}}"
source_sha="$(git rev-parse HEAD)"
qualified_source_sha="c5c5e7937b5d1c7562a72d90d761cd630baf7369"
build_dir="$(mktemp -d "${TMPDIR:-/tmp}/praxis-release-build.XXXXXX")"
trap 'rm -rf "$build_dir"' EXIT

if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
  echo "usage: $0 [version] [output-directory]" >&2
  exit 2
fi

if ! git merge-base --is-ancestor "$qualified_source_sha" HEAD; then
  echo "qualified source $qualified_source_sha is not an ancestor of HEAD" >&2
  exit 1
fi

rm -rf "$out_dir"
mkdir -p "$out_dir"

# Keep the qualified checkout's source bytes unchanged. The release version is
# a packaging concern, so the builder overlays only the main-package version
# declaration in a temporary copy for the binary build.
sed 's/const praxisVersion = "2.0.0-dev"/var praxisVersion = "'"$version"'"/' \
  cmd/praxis/core.go > "$build_dir/core.go"
printf '{"Replace":{"%s":"%s"}}\n' \
  "$(pwd)/cmd/praxis/core.go" "$build_dir/core.go" > "$build_dir/overlay.json"

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  IFS=/ read -r goos goarch <<< "$target"
  suffix=""
  [[ "$goos" == windows ]] && suffix=".exe"
  name="praxis-${version}-${goos}-${goarch}"
  work_dir="$out_dir/$name"
  mkdir -p "$work_dir"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    go build -overlay "$build_dir/overlay.json" -trimpath -ldflags "-s -w" \
    -o "$work_dir/praxis${suffix}" ./cmd/praxis
  cp LICENSE NOTICE README.md RELEASE_NOTES.md "$work_dir/"
  printf 'qualified_source_sha=%s\nrelease_version=%s\nplatform=%s/%s\n' \
    "$qualified_source_sha" "$version" "$goos" "$goarch" > "$work_dir/BUILD-METADATA"
  tar -C "$out_dir" -czf "$out_dir/$name.tar.gz" "$name"
  rm -rf "$work_dir"
done

(cd "$out_dir" && shasum -a 256 praxis-*.tar.gz > SHA256SUMS)
printf 'release artifacts written to %s\nqualified source: %s\npreparation source: %s\n' "$out_dir" "$qualified_source_sha" "$source_sha"
