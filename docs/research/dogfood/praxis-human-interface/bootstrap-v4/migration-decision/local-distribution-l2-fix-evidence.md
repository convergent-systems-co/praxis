# L2 (symlink containment) fix — post-Review-2 evidence

Date: 2026-09-22. Source commit `23766e1a95b6e5b99287a53cfda832bdf222a158`, on top of the L1 (path-traversal) fix (`9ad5731`) and its independent Review 2 (`REVISION_REQUIRED`, finding L2, `docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-local-distribution-review-2.md`).

## The finding (L2)

Review 2 confirmed the L1 lexical `..` traversal fix closes the original counterexample and all prior trust/TOCTOU/GitHub-regression properties held. It found one new defect: `filepath.Abs`/`filepath.Rel` containment is lexical only. A directory symlink placed lexically beneath `Root` (its reproduction: `Root/alias -> outside/alias`, a self-consistent fixture only under `outside/alias/pkg/1`, `Repo="alias/pkg"`) resolved outside `Root` at the filesystem level; `Resolve`, `Info`, and `ResolveLocked` all followed it. Not a signature-verification bypass — `VerifyPackage` still refuses unsigned material — but a genuine violation of the adapter's own stated canonical-`Root` containment claim.

## The fix

- `internal/distribution/local_first_party.go`: explicit, documented symlink policy. **Root itself may be a symlink** (operator-supplied configuration, not attacker-influenced input) and is canonicalized once via `filepath.EvalSymlinks`. **Every path component strictly below the resolved Root**, for every file the adapter reads (package directory, version directory, `identity.json`, `manifest.json`, `artifact.tar.gz`, `signature.json`), **must not itself be a symlink** — enforced by `noSymlinksBelow`, which walks each path component with `os.Lstat` (which does not follow symlinks) and refuses outright rather than resolving and re-validating. Combined with the existing lexical containment check, no-symlinks-below-Root makes lexical containment equal filesystem containment: without a symlink anywhere in the chain, the lexical path and the real filesystem path are identical.
- Every file read now goes through a new `readContainedFile(root, dir, name)` helper instead of bare `os.ReadFile`, so the policy applies uniformly — including to `signature.json`, where a symlinked signature file is now a hard refusal rather than silently treated as "absent, so unsigned."

## TOCTOU reasoning (reviewed, reaffirmed, not modified)

`FetchArtifact` independently re-reads and re-verifies from disk on every call (unchanged from the L1 repair); `packagecatalog.VerifyPackage`'s digest cross-binding (`bytesDigest(ArtifactBytes) == manifest.ContentDigest`) closes any window between the `Resolve`/`Info` call and the `FetchArtifact` call at the consumption boundary. This repair adds a containment gate *before* any file is read; it does not change, and does not need to change, that reasoning, and it does not claim atomic filesystem I/O — a mutable filesystem between two separate adapter calls is still caught by digest re-binding, exactly as before.

## Regression tests reproducing L2 at every point in the chain

New tests in `internal/distribution/local_first_party_test.go`, each independently confirmed refused via `Resolve`, `Info`, **and** `ResolveLocked` (`assertLocalAcquisitionRefused`):

- `TestLocalFirstPartyRefusesPackageDirectorySymlinkEscapingRoot` — Review 2's exact shape (package-directory symlink).
- `TestLocalFirstPartyRefusesVersionDirectorySymlinkEscapingRoot` — version-directory symlink.
- `TestLocalFirstPartyRefusesIntermediateSymlinkComponent` — a namespace segment (`ns/pkg`, `ns` symlinked) escaping via an intermediate component.
- `TestLocalFirstPartyRefusesIdentityFileSymlinkEscapingRoot` / `...RefusesManifestSymlinkEscapingRoot` / `...RefusesArchiveSymlinkEscapingRoot` — each of the three consumed files individually symlinked to outside content.

A companion positive test, `TestLocalFirstPartyAllowsRootItselfToBeASymlink`, establishes the explicit Root-may-be-a-symlink policy: a legitimate package placed entirely beneath the real directory a symlinked `Root` points at still acquires successfully, proving the fix is a containment gate, not a blanket symlink prohibition that would break legitimate operator configuration.

All prior tests (L1 traversal, tamper, identity mismatch, unsigned/wrong-key signature refusal, transport-equivalence, CLI routing, GitHub regression) re-run unmodified and pass — 27 tests total in `internal/distribution`.

## Qualification

- `go build ./...`, `go vet ./...`: clean.
- `gofmt -l`: clean.
- `go test $(go list ./... | grep -v /internal/conformance)`: all packages pass.
- `git diff --check`: clean.
- `go test -race -count=1 ./internal/distribution/...`: clean, all 27 tests including the six new symlink tests and the Root-as-symlink positive test.

## Reproducible build identity (fixed delta)

Two genuine `git clone --no-local` checkouts at distinct absolute paths, independent `GOCACHE`s, built with the unmodified canonical `verify/build_candidate_artifacts.sh`:

| Item | SHA-256 |
|---|---|
| core (`vcs.revision=23766e1…`, `vcs.modified=false`, `-trimpath=true`) | `772f573ed350f74679064db181d65a63f4f40e7fd72382230a6d18c2a3adb367` |
| Goals plugin | `f3fbc99100f4ec4b1cd1f7c3dc5f27fe9a4018efb8047354793399f622cc33b9` |
| package archive | `47457bbdec316c468b5cd20042c88eb11ac7626e7c45d722eb07b9d54f408e75` |
| package manifest | `e45a8cc0296340146e80609af196314e2bf89782bf7e40936a4c830e7212d53d` |

`cmp` confirmed byte-for-byte identity across both clones for all four artifacts. `strings` search for either clone's absolute checkout path in the built core: zero matches in either. Both temporary clones, build outputs, and caches were removed after use; none are present in the repository.

## Residual, explicitly out of L2's scope

Windows path/case/volume symlink semantics are unmeasured. Only the currently supported/measured Darwin environment is qualified — no portability claim is made or implied.

## Status

This is `L2_REPAIR_QUALIFIED` per the implementer's own qualification. **It is not independently accepted.** Consistent with this program's practice, and as explicitly required by the authorizing instruction for this repair, a fresh Review #3 must independently attempt to falsify this exact identity (`23766e1`, hashes above) before it is treated as qualified for any separately authorized atomic replacement.

No installation, deployment, signing, publisher ceremony, or activation was performed. The currently installed core remains the Review-10 identity `42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821`, unchanged. Working tree clean.
