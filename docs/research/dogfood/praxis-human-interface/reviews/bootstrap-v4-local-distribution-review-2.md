# Review 2 — LocalFirstParty distribution containment repair

**REVISION_REQUIRED**

## Exact reviewed identity

The repaired implementation commit is `9ad57319f50fc19854b6671da9e8be09d8902ecc`; repository HEAD during review is `48d254becd1b7e5e1319527b08ec2543ea17b6ac`. The one later commit is documentation/evidence only. The bounded implementation delta from Review 1 (`53aa311`) to repair changes exactly `internal/distribution/local_first_party.go`, its tests, the local package-reference CLI parser, and its tests.

## 1. Original Review-1 traversal counterexample

**ESTABLISHED CLOSED.** `versionDir` now computes absolute root/candidate paths and rejects a `filepath.Rel` result that is `..` or begins `../`. The regression reproduces Review 1's self-consistent fixture outside Root with traversal-shaped `PackageID` and confirms refusal from `Resolve`, `Info`, and `ResolveLocked`. I independently ran it with the focused distribution/CLI suite; it passed.

## 2–4. Containment and new finding L2

**ESTABLISHED — lexical containment only.** Every normal acquisition entry point reaches `load`, then `versionDir`: `Resolve`, `Info`, `FetchArtifact` (by reloading the release), and `ResolveLocked`. Empty Root/repository/version are refused. `../` and nested traversal candidates are lexically refused. The local adapter itself, not only the CLI, supplies this guard.

**FINDING L2 — filesystem containment remains bypassable by directory symlink.** `filepath.Abs` plus `filepath.Rel` is lexical; neither Root nor candidate is passed through `filepath.EvalSymlinks`, and files are then opened through the unchecked `dir`. My scratch-only probe created `Root/alias` as a symlink to `<outside>/alias`, wrote a self-consistent `alias/pkg@1` fixture only outside Root, and supplied `Repo="alias/pkg"`. All of `Resolve`, `Info`, and `ResolveLocked` succeeded. The lexical candidate is beneath Root while the OS read traverses outside it.

This is the same smallest defect class as Review 1: **LocalFirstParty does not enforce filesystem containment beneath configured Root for attacker-influenced package-tree topology.** It needs no `..`, malformed package ID, CLI path, or historical Review-1 artifact. The repair therefore does not completely close the requested counterexample class.

**Trust classification.** The external fixture was only self-consistent, not trusted: downstream `VerifyPackage` would reject it unsigned, and focused unsigned/wrong-key verification tests passed. Thus L2 is not a demonstrated signature-verification bypass. It is nevertheless a true violation of the adapter's stated canonical-Root boundary; validly signed material elsewhere could be surfaced through the local transport as catalogued local material.

Root symlinks are likewise not resolved before the lexical comparison, so the configured path can name a symlinked external tree. That may be an intentional administrator configuration choice, but it is not a physical-containment guarantee. Candidate-file symlinks also follow the OS filesystem; mismatched external file bytes are digest-refused, but a correctly pinned external file is acquired. Darwin path semantics were tested; Windows separator/case/volume semantics are **UNKNOWN**, not claimed.

## 5. CLI defense in depth

**ESTABLISHED, limited.** `parsePackageDeployRef` independently rejects a `..` slash-delimited segment in either local package ID or version, while passing `local:shared/graph@2`. It is not the only guard: direct adapter traversal is refused lexically. It does not mitigate L2 because `alias/pkg` contains no traversal segment.

## 6. Trust-boundary, TOCTOU, and GitHub regressions

**ESTABLISHED in focused scope.** The targeted tests passed for unsigned-local refusal, invalid/wrong-key signature refusal, transport-neutral `VerifyPackage` outcome, immutable dependency lock resolution, and unchanged GitHub package-reference routing. Source inspection confirms the repair does not modify `packagecatalog.VerifyPackage`, `GitHubReleases`, or resolver verification/digest re-binding paths.

**INFERRED from unchanged code plus focused regressions.** The prior Resolve→Fetch TOCTOU property remains digest re-binding at downstream verification, not atomic adapter I/O: `FetchArtifact` reloads material, while `VerifyPackage` cross-binds actual artifact bytes to the prior manifest digest. This repair did not change either function. No new TOCTOU bypass was reproduced.

## 7–8. Build identity

**ESTABLISHED.** Two fresh `git clone --no-local` checkouts detached at `9ad5731`, at distinct absolute paths, ran the versioned candidate build script with distinct fresh `GOCACHE` directories and outputs outside their worktrees. The core SHA-256 is independently `5b2d3cbfd1d6f0e1b7c57c0b3b8f326b1303470efd2d70140d13917aec4bdbac`; `go version -m` records `vcs.revision=9ad5731…`, `vcs.modified=false`. The core has no source-checkout path string. The paired build evidence is recorded separately.

## Disposition

**REVISION_REQUIRED.** The `..` fix works, the CLI layer is useful defense in depth, and the previous trust properties/reproducible build remain intact in the tested scope. L2 is a reproducible equivalent containment counterexample and blocks acceptance of this exact repaired candidate. It is **not qualified for separately authorized atomic replacement**.

No candidate implementation, test, evidence, artifact, active installation, package signature, deployment, authority ceremony, RecoveryIntent, Japetella, Git remote, or lifecycle state was modified. Only this new review artifact and its evidence were created.
