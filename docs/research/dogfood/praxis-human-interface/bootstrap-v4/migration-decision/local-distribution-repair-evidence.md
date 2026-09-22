# Local-first-party distribution transport — repair evidence (post-Review-#10 delta)

Date: 2026-09-22. Source commit `7a2f419cf514235c2f866a336683a5bdd1895679`, on top of the Review-10-accepted baseline (`25c73305fbca532e6b402b9d4aa1ed3412305169`). This repair modifies Praxis source; the Review #10 baseline is preserved unchanged as historical accepted evidence and is **not** what this identity claims to be.

## What changed (source)

- `internal/distribution/local_first_party.go` (new): `LocalFirstParty`, a `distribution.Adapter` + `LockedAdapter` reading a package from `Root/<package-id>/<version>/{manifest.json,artifact.tar.gz,identity.json,signature.json?}`, refusing (fails closed) on any mismatch between the files on disk and their pinned `identity.json` digests, the requested version, or a locked dependency's digest. Adds `distribution.SourceLocalFirstParty`/`SourceGitHubReleases` constants and `distribution.RootAdapter` (`Adapter` + `LockedAdapter`).
- `cmd/praxis/package_manager_authority.go`: new `parsePackageDeployRef` routes `local:<package-id>@<version>` (requires `PRAXIS_LOCAL_PACKAGES_DIR`, no default) to the new adapter; anything else routes through the **unchanged** `parseGitHubPackageRef`/`GitHubReleases` path. `runPackageDeploymentIntentPreview` calls this router instead of hardcoding `GitHubReleases`.
- `cmd/praxis/packages.go`: `resolveReleasePackages`'s adapter parameter widened from the concrete `GitHubReleases` type to `distribution.RootAdapter` (an existing-behavior-preserving widening — `GitHubReleases` already satisfies it). Its `Resolver.Sources` map key changed from the literal `"github-releases"` to `release.Ref.Source` (identical value for a GitHub release; generalizes correctly for a local one).
- `docs/ADR/025-catalog-trust-provenance-and-signing.md`: adds an **Accepted** addendum ratifying "distribution transport != package trust" and preserving the falsified initial hypothesis and its correcting counterexample. The rest of Draft ADR-025 remains Draft.
- `goalspublication.CheckAcquisition` and `verifyReleasePackage` (dead code, no callers found) were inspected and **deliberately left unmodified** — the former's GitHub-specific check is narrowly scoped to the exact pinned `praxis.package.goals@0.1.0` release and correctly does not conflate transport with general trust.

`packagecatalog.VerifyPackage` itself is **unmodified**. No publisher key was created, no `package.publish` authority was established, no package was signed, no package was deployed, nothing was installed.

## Qualification

- `go build ./...`, `go vet ./...`: clean.
- `gofmt -l` on every changed file: clean. `git diff --check`: clean.
- `go test $(go list ./... | grep -v /internal/conformance)`: **all packages pass**, including the full pre-existing suite (nothing outside the touched files was modified).
- `go test -race` on `internal/distribution`, `internal/packagecatalog`, `cmd/praxis` (filtered to the new/related tests plus the pre-existing GitHub/resolver tests): clean.
- `internal/conformance` (historical attestation suite): fails, **identically to the unmodified baseline** — confirmed by `git stash`/`git stash pop` comparison against the pre-repair tree, same pre-existing, unrelated (`internal/agent`/`internal/state` staleness) failure class documented throughout this whole PRE-V4 effort. Not a new regression.

New/adversarial tests added (`internal/distribution/local_first_party_test.go`, `cmd/praxis/package_manager_authority_local_test.go`):

- exact local artifact acquisition succeeds, bytes unaltered (`TestLocalFirstPartyAcquiresExactQualifiedArtifact`);
- tampered artifact/manifest fail closed (`TestLocalFirstPartyRefusesTamperedArtifact`, `...RefusesTamperedManifest`);
- missing package fails closed (`TestLocalFirstPartyRefusesMissingPackage`);
- wrong requested/claimed identity fails closed (`TestLocalFirstPartyRefusesWrongRequestedIdentity`);
- unknown/wrong source fails closed, at both the adapter (`TestLocalFirstPartyRefusesUnknownSource`) and dependency-lock (`TestLocalFirstPartyResolveLockedRefusesWrongSourceKind`) levels;
- a local release can never carry GitHub provenance (`TestLocalFirstPartyCannotClaimGitHubProvenance`);
- a locked dependency's digest mismatch fails closed (`TestLocalFirstPartyResolveLockedRefusesWrongDependencyDigest`);
- **transport choice does not change the verification outcome or identity** for equivalent, identically-signed material (`TestLocalTransportDoesNotChangeVerificationOutcome`);
- **an unsigned local package still fails `VerifyPackage`**, exactly like any other transport (`TestLocalUnsignedPackageStillFailsVerification`);
- **an invalidly-signed local package still fails `VerifyPackage`** (`TestLocalInvalidSignatureStillFailsVerification`);
- CLI routing: `local:` prefix selects the new adapter and requires `PRAXIS_LOCAL_PACKAGES_DIR`; malformed local refs and unrecognized ref forms fail closed; the GitHub `owner/repo[@tag]` path and its adapter/token wiring are **unchanged** (`package_manager_authority_local_test.go`).

## Reproducible build identity

Built with the existing, unmodified `verify/build_candidate_artifacts.sh` (`-trimpath`, `go build`). Independently reconfirmed byte-identical across three separate builds: the real checkout (clean tree) and two genuine fresh `git clone --no-local` checkouts at distinct absolute paths with independent `GOCACHE`s.

| Item | SHA-256 |
|---|---|
| core (`vcs.revision=7a2f419…`, `vcs.modified=false`, `-trimpath=true`) | `835152faea3288a14ea8b033f4ea8938b1075c5e4e2dd6faf17ae300716ed260` |
| Goals plugin | `0c143ede59026b275e85016c3939fa48e117f559c19e400f59741b1aa79c01ef` |
| package archive | `48e7a3e406f67526b225268c63e75b1a0c6e664b9052b5684410f07b40408aa5` |
| package manifest | `f316889bf420d3b2f7c4045fcd047d591099406791532b0ec38a3574738ed5e5` |

`strings`-searched for both fresh-clone checkout roots in the resulting core binary: zero matches in either. `cmp` confirmed byte-for-byte identity across all three builds for all four artifacts.

**One operator-error note, corrected before this table was finalized (not a reproducibility defect):** an intermediate real-checkout build was accidentally invoked with a *relative* output path while the working directory was the repository root, landing an untracked `artifacts/` directory outside the gitignored `bootstrap-v4/**/artifacts/` path. This flipped `vcs.modified` to `true` for that one build and produced different bytes — exactly the already-documented, non-blocking observation from Astra Review #10 (`vcs.modified` mid-build-sequence sensitivity to in-worktree, non-ignored output). The stray directory was removed, the tree reconfirmed clean, and the rebuild above reproduced byte-identically across all three independent builds. This is a usage-discipline note, not a defect in `build_candidate_artifacts.sh` or in build determinism.

## Relationship to the Review #10 baseline

Review #10 accepted `9edca1e5735f77ab8a4b76431749c542aa338dc7` / source `25c73305fbca532e6b402b9d4aa1ed3412305169`. This repair is a new, later commit (`7a2f419`) on top of that accepted baseline — it is a **distinct, unreviewed identity**. The Review #10 evidence, its acceptance, and its artifact identities are unmodified and remain the historical accepted record for that exact prior identity; nothing here rewrites or supersedes them silently.

## Promotion/installation work required before this repaired core could drive package deployment

1. A fresh independent review of this exact delta (source `7a2f419`, identities above) — not performed or requested by this repair phase.
2. Separate, explicit human authorization for atomic core-binary replacement of the *currently installed* candidate (`42cb404f…`, the Review-10 identity) with this repaired one — not authorized here.
3. The full first-party signing ceremony diagnosed earlier this session (`publisher key-create` → `enroll-preview/-approve/enroll` → `publisher authority-preview/-proposal/-review/-request` → `publisher sign-preview/sign`) — none of it executed here.
4. A `PRAXIS_LOCAL_PACKAGES_DIR` populated with the exact qualified Goals package in the adapter's expected layout (`manifest.json`, `artifact.tar.gz`, `identity.json` pinning the exact digests above, plus `signature.json` once signed) — not created here.

## Can the qualified Goals package then traverse local acquisition → ordinary verification → deployment without GitHub, once signed?

Yes, architecturally, once (3) and (4) above are done: `package-deploy-intent-preview local:praxis.package.goals@0.1.5` would route through `LocalFirstParty`, and the **same, unmodified** `packagecatalog.VerifyPackage` pipeline that has always governed GitHub-sourced packages. No GitHub publication is required by the code as it now stands. This was verified structurally (routing, adapter interface satisfaction, unchanged verification call) and by the adversarial test suite above, not by an end-to-end run against the real Goals package — that end-to-end run itself requires the still-unperformed signing ceremony and is out of this phase's authorization.
