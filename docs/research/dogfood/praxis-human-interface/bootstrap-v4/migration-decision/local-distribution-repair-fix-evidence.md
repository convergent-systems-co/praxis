# Local-distribution path-containment fix — post-Review-1 evidence

Date: 2026-09-22. Source commit `9ad57319f50fc19854b6671da9e8be09d8902ecc`, on top of the local-distribution repair (`7a2f419`) and its independent Review 1 (`REVISION_REQUIRED`, `docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-local-distribution-review-1.md`).

## The finding

Review 1's one blocking finding: `LocalFirstParty.versionDir` built `filepath.Join(a.Root, repo, version)` with no containment check. Because `packagecatalog.Manifest.Validate` places no character restriction on `PackageID`, a manifest could declare a `PackageID` equal to a traversal-shaped `repo` argument, satisfying the adapter's own identity-binding check while resolving outside the configured `Root`. Review 1 reproduced this independently with its own probe and full geometry check before reporting it.

## The fix

- `internal/distribution/local_first_party.go`: `versionDir` now resolves `Root` and the joined candidate directory to absolute paths and requires a `filepath.Rel`-based containment relationship, refusing whenever the resolved directory is not `Root` itself or a descendant of it. This is the single choke point `Resolve`/`Info`/`FetchArtifact`/`ResolveLocked` all route through via `load()`, so the fix applies uniformly.
- `cmd/praxis/package_manager_authority.go`: defense in depth — `parsePackageDeployRef` now rejects a `..` path segment in the package id or version at the CLI parse boundary, before a `PackageRef` is even constructed (`containsPathTraversalSegment`). This does not reject `/` generally, since this codebase's own package ids use it as a namespace separator (e.g. `shared/graph` in `internal/distribution/resolver_test.go`) — only the traversal shape.

## Regression test reproducing the exact finding

`TestLocalFirstPartyRefusesPathTraversalEvenWithMatchingManifestID` (`internal/distribution/local_first_party_test.go`) reproduces Review 1's counterexample geometry precisely: a fixture placed entirely outside a `t.TempDir()`-configured root, with `manifest.PackageID` literally equal to the traversal-shaped `repo` argument (`"../outside-root/victim"`), and an explicit assertion that the test's own `filepath.Join` arithmetic actually reaches the planted fixture before asserting anything about the adapter (so a future refactor of the test can't silently stop testing what it claims to). Run against `Resolve`, `Info`, and `ResolveLocked`. `TestParsePackageDeployRefRejectsPathTraversal` (`cmd/praxis/package_manager_authority_local_test.go`) covers the CLI-layer check, including a regression guard that a legitimate slash-namespaced package id (`shared/graph`) is still accepted.

## Qualification

- `go build ./...`, `go vet ./...`: clean.
- `gofmt -l` on every changed file: clean.
- `go test $(go list ./... | grep -v /internal/conformance)`: all packages pass.
- `git diff --check`: clean.
- `go test -race` on `internal/distribution` and `cmd/praxis` (filtered to the new/related tests plus the full pre-existing GitHub/resolver suite): clean, including the new regression tests.

## Reproducible build identity (fixed delta)

Two genuine `git clone --no-local` checkouts at distinct absolute paths, independent `GOCACHE`s, built with the unmodified canonical `verify/build_candidate_artifacts.sh`:

| Item | SHA-256 |
|---|---|
| core (`vcs.revision=9ad5731…`, `vcs.modified=false`, `-trimpath=true`) | `5b2d3cbfd1d6f0e1b7c57c0b3b8f326b1303470efd2d70140d13917aec4bdbac` |
| Goals plugin | `5829df4fbb3812c66e4440d946a033ecd7d3648a30301167469357c669d767a4` |
| package archive | `b59415c360a426b50e8331e9bf3e1968840c40abf61991d5bd317fd46700a255` |
| package manifest | `7dd37cef062fe46f2b4859cfc5e12fd286dbde26f3c69dbf74cc7e474c36732f` |

`cmp` confirmed byte-for-byte identity across both clones for all four artifacts. `strings` search for either clone's absolute checkout path in the built core: zero matches in either. Both temporary clones, their build outputs, and their caches were removed after use; none are present in the repository.

## Status

This fixes the one REVISION_REQUIRED finding from Review 1. **It has not itself been independently reviewed.** Consistent with this whole program's practice, a fresh review of this exact identity (`9ad5731`, hashes above) is required before it is treated as qualified for anything beyond this record — not requested or performed automatically by this fix.

No installation, deployment, signing, publisher ceremony, or activation was performed. The currently installed core is the Review-10 identity `42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821` (installed earlier in this session under separate, explicit authorization); it remains unchanged and untouched by this fix. Working tree clean.
