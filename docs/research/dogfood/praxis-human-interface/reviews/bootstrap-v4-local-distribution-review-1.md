# Review 1 — local-first-party package distribution transport

**REVISION_REQUIRED**

Fresh, independent reviewer. No access to the implementer's conversation, reasoning, generated evidence, or reported hashes as trusted input — all claims below were independently recomputed, rebuilt, or probed against the repository directly. Full raw command transcripts, probe source, and outputs are in the accompanying evidence file, `bootstrap-v4-local-distribution-review-1-evidence/review-1-evidence.md`.

## Exact reviewed identity

Delta under review: `7a2f419cf514235c2f866a336683a5bdd1895679` ("feat(distribution): add a local-first-party package transport (bootstrap distribution-boundary repair)") on `feature/intent-evolution`. Repository HEAD at review time: `c9af74bafef3c154babd61a4967e05db6ae0fc7a`, one commit past `7a2f419` — that final commit (`docs(bootstrap-v4): freeze evidence for the local-first-party distribution repair`) touches only `docs/research/...` evidence files, not any of the seven source/test/ADR files this review covers. Reviewed baseline: Astra Review #10's accepted `25c73305fbca532e6b402b9d4aa1ed3412305169`.

`git diff 25c7330..7a2f419 --stat` touches exactly seven files: `cmd/praxis/{cli_help.go,package_manager_authority.go,package_manager_authority_local_test.go,packages.go}`, `docs/ADR/025-catalog-trust-provenance-and-signing.md`, `internal/distribution/{local_first_party.go,local_first_party_test.go}`. **Independently confirmed zero-byte diff** on `internal/packagecatalog/` (all files, including `verification.go`) and on `internal/goalspublication/` (including `acquisition.go`'s `CheckAcquisition`, which hardcodes `release.Ref.Source != "github-releases"`). `internal/distribution/` changed only by two wholly new files (adapter + its tests) — no existing adapter, the `Resolver`, or the `PackageRef`/`Release`/`Adapter` type definitions were modified.

## Semantic delta

1. New `distribution.LocalFirstParty` adapter (`Adapter` + `LockedAdapter`, i.e. `RootAdapter`) that acquires a package from `Root/<package-id>/<version>/{manifest.json,artifact.tar.gz,identity.json,signature.json?}`, cross-checking on-disk bytes against a pinned `identity.json` on every read (`Resolve`, `Info`, `FetchArtifact`, `ResolveLocked` each independently re-read and re-verify).
2. New `distribution.SourceLocalFirstParty` / `SourceGitHubReleases` constants and `distribution.RootAdapter` interface (a naming/typing convenience; `GitHubReleases` already satisfied it).
3. `cmd/praxis/package_manager_authority.go`: new `parsePackageDeployRef` routes `local:<package-id>@<version>` (requiring `PRAXIS_LOCAL_PACKAGES_DIR`, no default) to the new adapter; anything else still goes through the unchanged `parseGitHubPackageRef`/`GitHubReleases` path. `runPackageDeploymentIntentPreview` now calls this router instead of hardcoding GitHub.
4. `cmd/praxis/packages.go`: `resolveReleasePackages`'s adapter parameter widened from concrete `GitHubReleases` to the `RootAdapter` interface; `Resolver.Sources` keyed by `release.Ref.Source` instead of the literal `"github-releases"` string (a generalization that is behaviorally identical for GitHub, since `release.Ref.Source` was always exactly that literal for a GitHub release).
5. `docs/ADR/025-catalog-trust-provenance-and-signing.md`: a narrowly-scoped Accepted addendum ratifying "distribution transport != package trust" and documenting the falsified initial hypothesis.
6. `cmd/praxis/cli_help.go`: usage-string text only.

No change to `packagecatalog.VerifyPackage`, `VerifySignature`, `Resolver.Resolve`, `authoritativeReleaseManifest`, or `goalspublication.CheckAcquisition`/`RequiresLocalLineage`.

## Trust-boundary disposition: HOLDS

Independently re-read `internal/packagecatalog/verification.go` in full (byte-identical to baseline). `VerifyPackage` unconditionally: (a) requires non-empty manifest/artifact bytes and a signature envelope whose `ManifestDigest`/`ArtifactDigest` match freshly recomputed SHA-256 digests of the actual bytes and match `manifest.ContentDigest`; (b) calls `VerifySignature` against the caller-supplied trusted-key set regardless of `SourceKind`/`SourceRef`. This is unreachable-around by the new adapter: `LocalFirstParty` never touches signature bytes except to pass through whatever `signature.json` (optionally) contains, and an absent file is passed through as a zero-value signature, which `VerifyPackage`/`VerifySignature` refuse. Independently reproduced: `TestLocalUnsignedPackageStillFailsVerification` and `TestLocalInvalidSignatureStillFailsVerification` both re-run and pass; `TestLocalTransportDoesNotChangeVerificationOutcome` independently reproduced (github-sourced and local-sourced verification of byte-identical, identically-signed material produce identical `Manifest()`/`Evidence()` content, differing only in recorded `SourceKind`). `goalspublication.CheckAcquisition`'s GitHub-specific provenance gate is unmodified and unconditionally rejects any release whose `Ref.Source != "github-releases"`, so local material structurally cannot satisfy the Goals-publication-specific check — confirmed both by reading the unchanged code and by the implementer's own `TestLocalFirstPartyCannotClaimGitHubProvenance`, independently re-run.

Publisher/currentness checks (`packagecatalog.ReviewUpdate`, dependency-lock digest binding in `Resolver.Resolve`, `authoritativeReleaseManifest`'s manifest-bytes-vs-declared-manifest cross-check) are all unchanged code paths exercised identically regardless of which `RootAdapter` supplied the bytes — confirmed by reading `resolver.go` (unmodified) and by the passing `TestResolverVerifiesImmutableTransitiveLocksAcrossCatalogTransports`/`TestResolverRejectsLockMismatchCycleAndUnavailableTransport`.

## Filesystem / path-traversal disposition: DEFECT FOUND (see Residual findings)

`LocalFirstParty.versionDir` builds `filepath.Join(a.Root, repo, version)` with **no containment check** — no rejection of `..`, path separators, or absolute segments in `repo`/`version`, and no verification that the resolved path is a descendant of `Root`. Independently, most traversal shapes I attempted were incidentally blocked by the identity-binding check (`manifest.PackageID != ref.Repo` / `manifest.Version != version`), since a traversal string passed as `repo` normally would not equal the real manifest's declared `PackageID`. But `packagecatalog.Manifest.Validate()` places no restriction on the characters allowed in `PackageID`, so a manifest can declare a `PackageID` that is itself the traversal string. I built and ran a full end-to-end counterexample (`TestReviewerProbe_FullTraversalWithMatchingManifestID`, full source in the evidence file) that placed a self-consistent, correctly-identity-pinned fixture entirely outside a `t.TempDir()`-configured `Root` and successfully had `Resolve()` read and return it, using `repo = "../002/victim"` with a matching `manifest.PackageID`. This is a genuine violation of the adapter's own doc-comment invariant ("Root is the canonical local package directory") and directly answers review requirement 5 in the affirmative: package IDs can escape the configured local package root.

This is **not** a trust-boundary bypass in the narrow sense the Critical History warns about: the traversed-to material in my reproduction carried no signature and would be refused downstream by the unconditional, unchanged `VerifyPackage`. It is a filesystem-containment/scope defect in the transport layer, independent of the packagecatalog trust pipeline. It still meets this review's REVISION_REQUIRED bar under the stated disposition rule ("any reproducible... filesystem... defect").

## TOCTOU disposition: SAFE (by digest re-binding, not atomicity)

`Resolve()`/`Info()` and `FetchArtifact()` each independently call `load()`, which independently re-reads `identity.json`, `manifest.json`, and `artifact.tar.gz` from disk and re-verifies all three digest relationships every single call — there is no window where a stale in-memory value is trusted without a fresh disk read backing it. I constructed and ran `TestReviewerProbe_TOCTOUBetweenResolveAndFetch`: swap `manifest.json` + `identity.json` + `artifact.tar.gz` to a fully self-consistent "v2" set between the `Resolve()` call and the `FetchArtifact()` call on the same `Release`. `FetchArtifact()` (correctly) returned the v2 artifact bytes — but `release.Manifest.ContentDigest`, fixed at v1 from the earlier `Resolve()` call, does **not** match the v2 bytes' digest. The downstream consumer (`resolveReleasePackages` → `packagecatalog.VerifyPackage`) combines `release.ManifestBytes` (v1, from `Resolve()`) with the artifact bytes returned by `FetchArtifact()` (v2 in the attack) and requires `bytesDigest(ArtifactBytes) == manifest.ContentDigest` — which fails closed on a v1/v2 mismatch. This is confirmed by direct code reading of `VerifyPackage`'s digest-binding block, not merely inference from the probe. TOCTOU is closed by cryptographic digest cross-binding at consumption time, not by any lock or atomic read in the adapter itself — which is sufficient, but worth noting is incidental to the adapter's design rather than a deliberate atomicity guarantee.

## Signature / currentness disposition: HOLDS

Confirmed unconditional and unchanged (see Trust-boundary disposition above). `Ed25519Verifier`/`VerifySignature`/PQ-fallback logic untouched.

## GitHub regression disposition: NONE

`parseGitHubPackageRef` is called unchanged by the non-`local:`-prefixed branch of `parsePackageDeployRef`; `cmd/praxis/packages.go`'s pre-existing `install`/`update`/`rollback`/`disable`/`uninstall` command paths still construct `distribution.GitHubReleases{...}` directly and never route through the new adapter. The `Resolver.Sources` key generalization (`release.Ref.Source` in place of the literal `"github-releases"`) is behaviorally identical for GitHub releases because `GitHubReleases.Resolve` always set `Ref.Source = SourceGitHubReleases = "github-releases"` — confirmed by reading `internal/distribution/github_releases.go` (unmodified) and re-running the pre-existing GitHub adapter/resolver/packages tests, all of which passed unmodified. `TestParsePackageDeployRefGitHubBehaviorUnchanged` independently re-run and passed.

## Adversarial probes performed

Ten adversarial angles were attempted directly against the code (not merely read from the implementer's tests), listed with outcome (full source and full transcripts in the evidence file):
- modified archive — refused (`TestLocalFirstPartyRefusesTamperedArtifact`, re-run)
- modified manifest — refused (`TestLocalFirstPartyRefusesTamperedManifest`, re-run)
- wrong package ID (fixture at mismatched requested identity) — refused (`TestLocalFirstPartyRefusesWrongRequestedIdentity`, re-run)
- wrong version (same mechanism) — refused
- identity.json copied from another package — refused (my own `TestReviewerProbe_IdentityCopiedFromAnotherPackage`, independently written and run)
- unknown/wrong `Source` — refused (`TestLocalFirstPartyRefusesUnknownSource`, re-run)
- local material claiming `github-releases` provenance — structurally refused (`TestLocalFirstPartyCannotClaimGitHubProvenance` + unchanged `goalspublication` gate, both independently re-checked)
- symlink substitution (artifact.tar.gz → external file with different bytes) — refused, digest mismatch caught it (my own `TestReviewerProbe_SymlinkArtifact`)
- dependency-lock mismatch — refused (`TestLocalFirstPartyResolveLockedRefusesWrongDependencyDigest`, `TestLocalFirstPartyResolveLockedRefusesWrongSourceKind`, re-run)
- malformed local reference (`local:`, `local:no-at-sign`, `local:@1.0.0`, `local:package-id@`) — refused (`TestParsePackageDeployRefRejectsMalformedLocalReference`, re-run)
- path traversal via package-id/version — **partially defeats containment** (see Filesystem disposition; DEFECT)
- TOCTOU between Resolve and FetchArtifact — **safe** (digest re-binding closes it; see above)
- unsigned local material — refused (re-run)
- wrong-key-signed local material — refused (re-run)

## ADR-025 addendum disposition: ACCEPTABLE

Read in full. Explicitly scoped: "Status of this addendum: Accepted. It ratifies one narrow, already-implied point of this Draft ADR; it does not accept the rest of this document's proposals as ratified architecture, and their status remains Draft." It preserves the falsified initial hypothesis and its falsifying counterexample verbatim, and enumerates what it does and does not authorize (no publisher generation, no `package.publish` grant, no signing ceremony). Independently confirmed the ratified point matches the actual implementation and no broader Draft ADR-025 proposal is smuggled in as accepted.

## Independent rebuild

Two genuine `git clone --no-local` checkouts at different absolute paths (`/private/tmp/reviewer-clone1`, `/private/tmp/reviewer-clone2`, both outside any repo worktree, both removed after use), each detached at `7a2f419cf514235c2f866a336683a5bdd1895679`, each built with the canonical `verify/build_candidate_artifacts.sh` recipe to output directories also outside both worktrees. Results:

| artifact | independently recomputed SHA-256 | matches implementer's reported hash |
|---|---|---|
| core | `835152faea3288a14ea8b033f4ea8938b1075c5e4e2dd6faf17ae300716ed260` | yes |
| goals plugin | `0c143ede59026b275e85016c3939fa48e117f559c19e400f59741b1aa79c01ef` | yes |
| package archive | `48e7a3e406f67526b225268c63e75b1a0c6e664b9052b5684410f07b40408aa5` | yes |
| package manifest | `f316889bf420d3b2f7c4045fcd047d591099406791532b0ec38a3574738ed5e5` | yes |

`cmp` on the two independently built core binaries: byte-for-byte identical. `strings` search for either checkout's absolute path or the reviewing user's name in the built core binary: zero matches. `go version -m`: `vcs.revision=7a2f419cf514235c2f866a336683a5bdd1895679`, `vcs.modified=false`. Reproducibility across independent, differently-pathed clean clones is independently established; the implementer's reported hashes are corroborated, not merely trusted.

## Residual findings

1. **[REVISION_REQUIRED] Filesystem containment.** `LocalFirstParty.versionDir` performs no path-containment check on `Root`-joined `repo`/`version` segments. A crafted `manifest.PackageID` equal to a traversal string, paired with a caller-supplied `repo` equal to the same string, lets `Resolve`/`Info`/`FetchArtifact` read a self-consistent package fixture from outside the configured `Root`. Reproduced independently (§ Filesystem disposition above; full probe in evidence file). This does not currently compound into a trust bypass, because `packagecatalog.VerifyPackage`'s unconditional signature check still gates deployment regardless of where the bytes were read from — but it violates the adapter's own stated invariant and is exactly the class of "local filesystem transport introduces an attack surface GitHub transport does not have" this review was asked to establish. Recommended fix shape (not performed by this review, per scope control): resolve `filepath.Join(a.Root, repo, version)` to an absolute path via `filepath.Abs`/`filepath.EvalSymlinks` as appropriate and require it remain prefixed by a similarly-resolved `a.Root`, failing closed otherwise; additionally consider restricting `repo`/`version` to a safe character set at the parse boundary (`parsePackageDeployRef`) as defense in depth.
2. **[Note, not blocking] Symlink handling is currently digest-gated, not identity-gated.** A symlinked `artifact.tar.gz` pointing outside `Root` is safe today only because its target bytes are re-hashed and compared to the pinned digest on every read — there is no distinct rejection of symlinks themselves. This is adequate given the current digest-binding design but is worth noting as coupled to finding 1: if containment is added, symlink traversal should be considered together with path traversal.
3. No other residual findings. All other adversarial angles required by the review scope were attempted and held.

## Disposition

**REVISION_REQUIRED.** The core architectural claim under review — "distribution transport != package trust," local acquisition cannot manufacture trust, cannot claim GitHub provenance, and all locally acquired material still crosses the unmodified, unconditional `packagecatalog.VerifyPackage` pipeline — **holds** under independent reconstruction, independent test execution, and my own adversarial probes for identity binding, TOCTOU, unsigned material, and wrong-key-signed material. The delta is reproducible byte-for-byte from clean source at a location-independent build. However, a reproducible filesystem-containment defect (finding 1) means the local transport does not yet enforce the "canonical local directory" boundary it claims for itself, which is squarely in this review's required scope (item 5) and meets the stated bar for REVISION_REQUIRED ("any reproducible... filesystem... defect"). This delta is therefore **not** yet qualified for a separately authorized atomic replacement; it requires the containment defect (and, at the implementer's discretion, the symlink-handling note) addressed and a follow-up review before that status changes.

## Non-actions

No candidate implementation, test, evidence, artifact, active installation (`/Users/polliard/bin/praxis` untouched), database (`~/.praxis/praxis.db` untouched), Git commit, remote, deployment, activation, signing, publisher ceremony, or Gate was changed by this review. Only this review artifact and its evidence file were created; the two throwaway probe test files and the two throwaway clone/build directories used to produce this review were all removed after use and are not present in the repository.
