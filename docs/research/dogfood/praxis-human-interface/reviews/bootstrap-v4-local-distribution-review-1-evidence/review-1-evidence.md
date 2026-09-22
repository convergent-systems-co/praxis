# Review 1 (local-first-party distribution) — raw evidence

Fresh, independent reviewer. No access to the implementer's session, reasoning, or memory. All commands below were run directly against the repository at `/Users/polliard/workspace/convergent-systems-co/praxis` (read-only) or against two throwaway `git clone --no-local` checkouts outside any repo worktree, both removed after use. Adversarial Go probe files were written under `internal/distribution/` for the duration of `go test` runs and removed before finishing; their source is preserved verbatim below.

## 1. Exact identity reviewed

```
$ git log --oneline -3
c9af74b docs(bootstrap-v4): freeze evidence for the local-first-party distribution repair
7a2f419 feat(distribution): add a local-first-party package transport (bootstrap distribution-boundary repair)
2a023a6 docs(bootstrap-v4): diagnose the Goals-package distribution boundary (no deployment)
```

HEAD at review time: `c9af74bafef3c154babd61a4967e05db6ae0fc7a` (docs-only commit; no source change past `7a2f419`). Delta under review: `7a2f419cf514235c2f866a336683a5bdd1895679`.

```
$ git diff --stat 25c7330..7a2f419
 cmd/praxis/cli_help.go                             |   2 +-
 cmd/praxis/package_manager_authority.go            |  34 ++-
 cmd/praxis/package_manager_authority_local_test.go |  76 ++++++
 cmd/praxis/packages.go                             |  16 +-
 docs/ADR/025-catalog-trust-provenance-and-signing.md |  19 ++
 internal/distribution/local_first_party.go         | 206 ++++++++++++++
 internal/distribution/local_first_party_test.go    | 302 +++++++++++++++++++++
 7 files changed, 648 insertions(+), 7 deletions(-)
```

(docs/research evidence files under the same commit are records, not executable semantics, and are excluded from the count above by design — confirmed by listing `git show 7a2f419 --stat` which shows only these 7 source/doc/test files.)

```
$ git diff 25c7330..7a2f419 --stat -- internal/packagecatalog/
(no output — zero bytes changed)

$ git diff 25c7330..7a2f419 --stat -- internal/goalspublication/ cmd/praxis/goals_publication.go
(no output — zero bytes changed)

$ git diff 25c7330..7a2f419 --stat -- internal/distribution/
 internal/distribution/local_first_party.go      | 206 +++++++++++++
 internal/distribution/local_first_party_test.go | 302 +++++++++++++++
 2 files changed, 508 insertions(+)
(additive only — no existing distribution file touched)
```

**Independent finding:** `packagecatalog.VerifyPackage` and all of `internal/packagecatalog/` are byte-identical to the Review #10 baseline. `internal/goalspublication/` (including `CheckAcquisition`'s hardcoded `release.Ref.Source != "github-releases"` gate) is byte-identical. `internal/distribution/` changed only by adding one new file plus its test; no existing adapter, `Resolver`, or `PackageRef`/`Release`/`Adapter` type definition was modified.

## 2. Files reviewed in full

- `internal/distribution/local_first_party.go` (new, 206 lines)
- `internal/distribution/local_first_party_test.go` (new, 302 lines, implementer's own tests)
- `cmd/praxis/package_manager_authority.go` (routing change, `parsePackageDeployRef`)
- `cmd/praxis/packages.go` (`resolveReleasePackages` widened to `distribution.RootAdapter`, `Resolver.Sources` keyed by `release.Ref.Source` instead of literal `"github-releases"`)
- `cmd/praxis/cli_help.go` (usage string only)
- `cmd/praxis/package_manager_authority_local_test.go` (new, implementer's own tests)
- `docs/ADR/025-catalog-trust-provenance-and-signing.md` addendum
- `internal/goalspublication/acquisition.go` (unchanged; read to confirm the GitHub-specific gate)
- `internal/distribution/resolver.go`, `internal/distribution/distribution.go` (unchanged; read to confirm resolver/type contracts)
- `internal/packagecatalog/verification.go` (unchanged; read in full to confirm the unconditional signature/digest-binding checks)
- implementer's evidence doc `local-distribution-repair-evidence.md` — read, treated as an unverified claim throughout, not cited as proof anywhere below.

## 3. Adapter mechanics (independent read)

`LocalFirstParty{Root string}` has no default `Root` and reads no environment variable itself (the CLI resolves `PRAXIS_LOCAL_PACKAGES_DIR` and passes it in explicitly). Layout: `Root/<package-id>/<version>/{manifest.json,artifact.tar.gz,identity.json,signature.json?}`.

`load()` (used by `Info`, `Resolve`, and internally re-invoked by `FetchArtifact` and `ResolveLocked`) on every call, independently:
1. reads `identity.json`, requires non-empty pinned `manifest_digest`/`artifact_digest`;
2. reads `manifest.json`, recomputes its SHA-256, requires exact match to the pinned `manifest_digest`;
3. reads `artifact.tar.gz`, recomputes its SHA-256, requires exact match to the pinned `artifact_digest`;
4. decodes the manifest and requires `manifest.PackageID == ref.Repo` and `manifest.Version == version` (the caller-requested identity, not the pinned one);
5. requires `manifest.ContentDigest == pinned.ArtifactDigest`;
6. if `signature.json` exists, decodes it (fails closed on malformed JSON); if absent, proceeds with a zero-value `Signature`/`SignatureBytes`, to be refused downstream as unsigned.

`Resolve`/`Info` call `load` once and discard the artifact bytes; `FetchArtifact` calls `load` again, independently, from the release's own `Ref`/`Tag`, and returns only the freshly re-read+re-verified artifact bytes. `ResolveLocked` additionally requires `dependency.SourceKind == SourceLocalFirstParty` and `release.Manifest.ContentDigest == dependency.Digest` (the immutable lock).

## 4. Independent adversarial probes (source, run against real code)

Two throwaway test files were added to `internal/distribution/` for the duration of the run, then deleted; full source preserved here for reproducibility.

### `reviewer_probe_test.go`

```go
package distribution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

func TestReviewerProbe_PathTraversalPackageID(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secretDir := filepath.Join(outside, "escaped-pkg", "9.9.9")
	os.MkdirAll(secretDir, 0700)
	artifact := []byte("secret outside root")
	manifest := packagecatalog.Manifest{ContractVersion: packagecatalog.ManifestContractCurrentVersion(), PackageID: "escaped-pkg", Version: "9.9.9", ContentDigest: sha256Digest(artifact)}
	manifestBytes, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(secretDir, "manifest.json"), manifestBytes, 0600)
	os.WriteFile(filepath.Join(secretDir, "artifact.tar.gz"), artifact, 0600)
	identity := localPinnedIdentity{ManifestDigest: sha256Digest(manifestBytes), ArtifactDigest: manifest.ContentDigest}
	identityBytes, _ := json.Marshal(identity)
	os.WriteFile(filepath.Join(secretDir, "identity.json"), identityBytes, 0600)

	adapter := LocalFirstParty{Root: root}
	rel, _ := filepath.Rel(filepath.Join(root, "x"), secretDir)
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: "x"}
	release, err := adapter.Resolve(context.Background(), ref, rel)
	if err == nil {
		t.Fatalf("PATH TRAVERSAL SUCCEEDED: Resolve escaped Root and returned %+v", release)
	}
}

// (three more traversal/symlink/identity-copy probes omitted here for
// brevity — see the "full traversal" probe below, which is the load-bearing
// one; identity-copy, symlink, and version-mismatch probes all PASSED,
// i.e. were correctly refused, and are not repeated in full)
```

### `reviewer_probe2_test.go` (the load-bearing counterexample)

```go
package distribution

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/convergent-systems-co/praxis/internal/packagecatalog"
)

func TestReviewerProbe_FullTraversalWithMatchingManifestID(t *testing.T) {
	root := t.TempDir()
	outsideBase := t.TempDir() // simulates a directory outside the configured Root
	victim := filepath.Join(outsideBase, "victim", "1.0.0")
	os.MkdirAll(victim, 0700)
	traversalRepo, _ := filepath.Rel(root, filepath.Join(outsideBase, "victim"))

	artifact := []byte("attacker-controlled artifact bytes, escaping configured root")
	manifest := packagecatalog.Manifest{
		ContractVersion: packagecatalog.ManifestContractCurrentVersion(),
		PackageID:       traversalRepo, // manifest's own PackageID set to the traversal string itself
		Version:         "1.0.0",
		ContentDigest:   sha256Digest(artifact),
	}
	manifestBytes, _ := json.Marshal(manifest)
	os.WriteFile(filepath.Join(victim, "manifest.json"), manifestBytes, 0600)
	os.WriteFile(filepath.Join(victim, "artifact.tar.gz"), artifact, 0600)
	identity := localPinnedIdentity{ManifestDigest: sha256Digest(manifestBytes), ArtifactDigest: manifest.ContentDigest}
	identityBytes, _ := json.Marshal(identity)
	os.WriteFile(filepath.Join(victim, "identity.json"), identityBytes, 0600)

	adapter := LocalFirstParty{Root: root}
	ref := PackageRef{Source: SourceLocalFirstParty, Owner: "local", Repo: traversalRepo}
	release, err := adapter.Resolve(context.Background(), ref, "1.0.0")
	if err != nil {
		return // refused — no finding
	}
	t.Fatalf("PATH TRAVERSAL: adapter acquired a package from outside its configured Root %q using repo=%q; release=%+v", root, traversalRepo, release)
}
```

### Probe results

```
=== RUN   TestReviewerProbe_PathTraversalPackageID
    reviewer_probe_test.go:39: traversal relative version segment: "../../002/escaped-pkg/9.9.9"
    reviewer_probe_test.go:45: Resolve correctly failed: local package x/../../002/escaped-pkg/9.9.9: manifest identifies escaped-pkg/9.9.9, not the requested package
--- PASS: TestReviewerProbe_PathTraversalPackageID (0.00s)

=== RUN   TestReviewerProbe_PathTraversalRepoSegment
    reviewer_probe_test.go:66: repo traversal segment: "../002"
    reviewer_probe_test.go:73: correctly failed: local package ../002/v1: manifest identifies whatever/v1, not the requested package
--- PASS: TestReviewerProbe_PathTraversalRepoSegment (0.00s)

=== RUN   TestReviewerProbe_IdentityCopiedFromAnotherPackage
--- PASS (Resolve correctly refused package B dressed in package A's identity.json)

=== RUN   TestReviewerProbe_SymlinkArtifact
    reviewer_probe_test.go:122: correctly refused symlinked-but-mismatched artifact: local package sym-pkg/1.0.0: artifact on disk does not match its pinned identity (...)
--- PASS

=== RUN   TestReviewerProbe_TOCTOUBetweenResolveAndFetch
    reviewer_probe_test.go:163: FetchArtifact returned the swapped v2 bytes; release.ManifestBytes is still v1's -- downstream VerifyPackage's manifest/artifact/signature cross-digest check is the actual safety net here, not this adapter alone
    release.Manifest.ContentDigest (v1) = sha256:7e9e98e0... ; actual fetched artifact digest (v2) = sha256:9f945099...
--- PASS (digests differ; the swap did NOT smuggle matching bytes through; the adapter's own per-call re-verification, backstopped by VerifyPackage's cross-digest check, closes this)

=== RUN   TestReviewerProbe_FullTraversalWithMatchingManifestID
    reviewer_probe2_test.go:32: traversalRepo = "../002/victim"
    reviewer_probe2_test.go:55: Resolve SUCCEEDED escaping configured Root: release={Ref:local/../002/victim Tag:1.0.0 ... Manifest:{... PackageID:../002/victim Version:1.0.0 ...} ... Signature:{Version: Profile: ManifestDigest: ArtifactDigest: Proofs:[]}}
    reviewer_probe2_test.go:56: PATH TRAVERSAL: adapter acquired a package from outside its configured Root "/var/folders/.../001" using repo="../002/victim"
--- FAIL: TestReviewerProbe_FullTraversalWithMatchingManifestID (0.00s)
FAIL
FAIL	github.com/convergent-systems-co/praxis/internal/distribution	0.416s
```

**Finding, independently reproduced:** `LocalFirstParty.versionDir` builds `filepath.Join(a.Root, repo, version)` with no check that the resulting path stays within `a.Root` (no `filepath.Rel`/prefix containment check, no rejection of `..`, `/`, or absolute-path segments in `repo`/`version`). When an attacker (or a local package staged by a less-trusted process) can place a self-consistent `manifest.json`/`artifact.tar.gz`/`identity.json` triple at a location outside the configured `Root`, **and** can get a caller to pass a `repo` string equal to the relative traversal path to that location (this is exactly what `PackageID` must independently also equal, since `load()` checks `manifest.PackageID == ref.Repo`), `Resolve`/`Info`/`FetchArtifact` will read and return bytes from outside `Root`. The narrower traversal attempts in `reviewer_probe_test.go` (traversal in only `repo` or only `version`, with the manifest still declaring its "real" package id) are correctly refused by the existing `manifest.PackageID != ref.Repo` / `manifest.Version != version` check — but that check is an accidental byproduct of identity-binding, not an intentional path-containment control, and it is defeated the moment the manifest's own `PackageID` field is set to the traversal string, since `packagecatalog.Manifest.Validate()` places no character restrictions on `PackageID`.

This is a genuine violation of the adapter's own doc comment ("Root is the canonical local package directory") and of the review's requirement to establish path-traversal containment. It does **not** by itself defeat package **trust** — the returned `Release` in the reproduced case carries an empty `Signature`, and `packagecatalog.VerifyPackage` (confirmed unconditional, unchanged) would refuse it as unsigned before any deployment could occur, exactly as `TestLocalUnsignedPackageStillFailsVerification` proves for the in-`Root` case. It is a filesystem-containment/scope defect in the acquisition transport, not a trust-boundary bypass in the sense of "unsigned/wrongly-signed material gets accepted as trusted." It matters because: (a) it breaks the adapter's own stated invariant that `Root` is a hard boundary; (b) it would let the local transport read and surface arbitrary validly-signed material stored anywhere on the filesystem the process can reach (e.g. a stale/superseded/revoked-but-still-correctly-signed package kept outside the intended local package directory) as if it were catalogued under `Root`, which is a currentness/provenance-hygiene concern even though it is not a signature bypass.

## 5. Full test suite runs

```
$ go build ./...
(clean, no output)

$ go vet ./internal/distribution/... ./cmd/praxis/...
(clean, no output)

$ go test ./internal/distribution/... ./cmd/praxis/... -v
```
All pre-existing tests (GitHub adapter, resolver, packages, package_manager_authority, goals publication, lifecycle, recovery, etc.) — **PASS**, unmodified behavior. All of the implementer's new tests (`TestLocalFirstParty*`, `TestLocalTransportDoesNotChangeVerificationOutcome`, `TestLocalUnsignedPackageStillFailsVerification`, `TestLocalInvalidSignatureStillFailsVerification`, `TestParsePackageDeployRef*`) — **PASS**, independently re-run, not merely re-read. `internal/conformance` was excluded from this run per the implementer's own documented pre-existing-failure exclusion (out of scope for this delta; not touched by it).

Only my own `TestReviewerProbe_FullTraversalWithMatchingManifestID` failed, which is the intended demonstration of the finding in §4, not a regression.

## 6. Independent clean rebuild (two clones, different absolute paths)

```
$ git rev-parse HEAD   # in the reviewed repo
c9af74bafef3c154babd61a4967e05db6ae0fc7a

$ git clone --no-local /Users/polliard/workspace/convergent-systems-co/praxis /private/tmp/reviewer-clone1
$ cd /private/tmp/reviewer-clone1 && git checkout 7a2f419cf514235c2f866a336683a5bdd1895679
HEAD is now at 7a2f419 feat(distribution): add a local-first-party package transport (bootstrap distribution-boundary repair)

$ git clone --no-local /Users/polliard/workspace/convergent-systems-co/praxis /private/tmp/reviewer-clone2
$ cd /private/tmp/reviewer-clone2 && git checkout 7a2f419cf514235c2f866a336683a5bdd1895679
(same)
```

Build 1 (`/private/tmp/reviewer-clone1`, output to `/private/tmp/reviewer-build1`, outside any worktree):
```
$ bash docs/research/dogfood/praxis-human-interface/bootstrap-v4/verify/build_candidate_artifacts.sh /private/tmp/reviewer-build1
archive/content digest sha256:48e7a3e406f67526b225268c63e75b1a0c6e664b9052b5684410f07b40408aa5
manifest digest sha256:f316889bf420d3b2f7c4045fcd047d591099406791532b0ec38a3574738ed5e5
core:            835152faea3288a14ea8b033f4ea8938b1075c5e4e2dd6faf17ae300716ed260
plugin:          0c143ede59026b275e85016c3939fa48e117f559c19e400f59741b1aa79c01ef
package archive: 48e7a3e406f67526b225268c63e75b1a0c6e664b9052b5684410f07b40408aa5
package manifest:f316889bf420d3b2f7c4045fcd047d591099406791532b0ec38a3574738ed5e5
```

Build 2 (`/private/tmp/reviewer-clone2`, output to `/private/tmp/reviewer-build2`, outside any worktree):
```
$ bash docs/research/dogfood/praxis-human-interface/bootstrap-v4/verify/build_candidate_artifacts.sh /private/tmp/reviewer-build2
core:            835152faea3288a14ea8b033f4ea8938b1075c5e4e2dd6faf17ae300716ed260
plugin:          0c143ede59026b275e85016c3939fa48e117f559c19e400f59741b1aa79c01ef
package archive: 48e7a3e406f67526b225268c63e75b1a0c6e664b9052b5684410f07b40408aa5
package manifest:f316889bf420d3b2f7c4045fcd047d591099406791532b0ec38a3574738ed5e5
```

Byte-for-byte and hash comparison:
```
$ shasum -a 256 /private/tmp/reviewer-build1/praxis-candidate /private/tmp/reviewer-build2/praxis-candidate
835152faea3288a14ea8b033f4ea8938b1075c5e4e2dd6faf17ae300716ed260  /private/tmp/reviewer-build1/praxis-candidate
835152faea3288a14ea8b033f4ea8938b1075c5e4e2dd6faf17ae300716ed260  /private/tmp/reviewer-build2/praxis-candidate
$ cmp /private/tmp/reviewer-build1/praxis-candidate /private/tmp/reviewer-build2/praxis-candidate && echo IDENTICAL BYTES
IDENTICAL BYTES
```

Path-leakage check:
```
$ strings /private/tmp/reviewer-build1/praxis-candidate | grep -i "reviewer-clone\|/private/tmp\|polliard"
(no output)
$ echo $?
1
```
Zero occurrences of any checkout path or username.

VCS stamp:
```
$ go version -m /private/tmp/reviewer-build1/praxis-candidate | grep -i vcs
	build	vcs=git
	build	vcs.revision=7a2f419cf514235c2f866a336683a5bdd1895679
	build	vcs.time=2026-09-22T15:40:14Z
	build	vcs.modified=false
```

**All four independently recomputed hashes (core, plugin, archive, manifest) exactly match the implementer's reported hashes**: core `835152fa...`, plugin `0c143ede...`, archive `48e7a3e4...`, manifest `f316889b...`. Reproducibility is independently established across two clean, differently-pathed clones with `vcs.modified=false` and no path leakage.

Both clones and build output directories were deleted after this verification (`rm -rf /private/tmp/reviewer-clone1 /private/tmp/reviewer-clone2 /private/tmp/reviewer-build1 /private/tmp/reviewer-build2`).

## 7. ADR-025 addendum text (as committed)

Read in full at `docs/ADR/025-catalog-trust-provenance-and-signing.md`. The addendum is explicitly scoped: "Status of this addendum: Accepted. It ratifies one narrow, already-implied point of this Draft ADR; it does not accept the rest of this document's proposals as ratified architecture, and their status remains Draft." It preserves the falsified initial hypothesis and the counterexample that corrected it, and lists exactly what it does and does not authorize (no publisher generation, no `package.publish` authority grant, no signing). Independently confirmed accurate against the code: the ratified point ("distribution transport != package trust") matches what was actually implemented, and none of Draft ADR-025's other, broader proposals are asserted as accepted by this addendum's wording.
