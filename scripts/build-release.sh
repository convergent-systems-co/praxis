#!/usr/bin/env bash
set -euo pipefail

version="${1:-v2.0.0}"
version="${version#v}"
out_dir="${2:-dist/${version}}"
source_sha="$(git rev-parse HEAD)"
qualified_source_sha="c5c5e7937b5d1c7562a72d90d761cd630baf7369"
go_toolchain="$(go version)"
builder_platform="$(go env GOOS)/$(go env GOARCH)"
default_targets="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"
targets="${PRAXIS_RELEASE_TARGETS:-$default_targets}"
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

target_cgo() {
  case "$1" in
    darwin/amd64|darwin/arm64) printf '1\n' ;;
    linux/amd64|linux/arm64|windows/amd64|windows/arm64) printf '0\n' ;;
    *) echo "unsupported release target: $1" >&2; return 1 ;;
  esac
}

target_bootstrap() {
  case "$1" in
    darwin/amd64|darwin/arm64) printf 'macos-keychain\n' ;;
    linux/amd64|linux/arm64|windows/amd64|windows/arm64)
      echo "release target $1 has no audited production bootstrap backend" >&2
      return 1
      ;;
    *) echo "unsupported release target: $1" >&2; return 1 ;;
  esac
}

# Validate the entire requested set before replacing any output. This prevents
# an unsupported target or missing native backend from leaving a partial release.
for target in $targets; do
  target_cgo "$target" >/dev/null
  target_bootstrap "$target" >/dev/null
done

if [[ -z "$targets" ]]; then
  echo "at least one release target is required" >&2
  exit 2
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

for target in $targets; do
  IFS=/ read -r goos goarch <<< "$target"
  suffix=""
  [[ "$goos" == windows ]] && suffix=".exe"
  name="praxis-${version}-${goos}-${goarch}"
  work_dir="$out_dir/$name"
  mkdir -p "$work_dir"
  cgo_enabled="$(target_cgo "$target")"
  bootstrap_capability="$(target_bootstrap "$target")"
  probe="$work_dir/.bootstrap-capability-probe"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED="$cgo_enabled" \
    go build -overlay "$build_dir/overlay.json" -trimpath \
    -o "$probe" ./cmd/praxis
  symbols="$work_dir/.bootstrap-capability-symbols"
  go tool nm "$probe" > "$symbols"
  if ! grep -q 'MacOSKeychainBackend' "$symbols"; then
    echo "artifact $target does not contain required bootstrap capability $bootstrap_capability" >&2
    exit 1
  fi
  rm -f "$probe" "$symbols"
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED="$cgo_enabled" \
    go build -overlay "$build_dir/overlay.json" -trimpath -ldflags "-s -w" \
    -o "$work_dir/praxis${suffix}" ./cmd/praxis
  cp LICENSE NOTICE README.md RELEASE_NOTES.md "$work_dir/"
  printf 'qualified_source_sha=%s\npreparation_source_sha=%s\nrelease_version=%s\nplatform=%s/%s\nbuilder_platform=%s\ncgo_enabled=%s\nbootstrap_capability=%s\ngo_toolchain=%s\n' \
    "$qualified_source_sha" "$source_sha" "$version" "$goos" "$goarch" \
    "$builder_platform" "$cgo_enabled" "$bootstrap_capability" "$go_toolchain" > "$work_dir/BUILD-METADATA"
  tar -C "$out_dir" -czf "$out_dir/$name.tar.gz" "$name"
  rm -rf "$work_dir"
done

(cd "$out_dir" && shasum -a 256 praxis-*.tar.gz > SHA256SUMS)
printf 'release artifacts written to %s\nqualified source: %s\npreparation source: %s\n' "$out_dir" "$qualified_source_sha" "$source_sha"
