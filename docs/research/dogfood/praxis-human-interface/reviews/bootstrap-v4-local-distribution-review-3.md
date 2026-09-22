# Review 3 — LocalFirstParty L2 symlink-containment repair

**REVISION_REQUIRED**

## Exact reviewed identity and delta

Repair source identity: `23766e1a95b6e5b99287a53cfda832bdf222a158`. Review HEAD: `94e17831e6e063a39536c9f2af9e77d04f130f12`. The later commit is documentation evidence only. The only implementation/test files changed from `9ad5731` to the repair are `internal/distribution/local_first_party.go` and `internal/distribution/local_first_party_test.go`; `packagecatalog.VerifyPackage`, `internal/goalspublication/acquisition.go`, and `internal/distribution/github_releases.go` are byte-identical across that range.

## L2 static-symlink repair

**ESTABLISHED, but insufficient.** `versionDir` resolves operator-configured Root with `EvalSymlinks`, applies the prior lexical `Rel` check, and calls `noSymlinksBelow`. Every ordinary adapter read flows through `load` and `readContainedFile`, so stable package/version/intermediate-directory and individual identity/manifest/archive/signature-file symlinks are rejected by `Lstat`. Root itself may legitimately be a symlink. The prior `..` traversal remains lexically refused.

## Finding L3 — check-to-open symlink-swap containment escape

**ESTABLISHED.** `readContainedFile` performs `noSymlinksBelow(root, path)` (a pathname `Lstat` walk) and subsequently `os.ReadFile(path)` (a new pathname lookup). It is not an atomic containment operation. A caller able to mutate the package tree can rename a checked regular parent directory and replace it with a symlink to an external directory between those calls.

My reviewer-only deterministic interleaving probe successfully checked `root/pivot/pkg/1/artifact.tar.gz`, replaced `root/pivot` with an external symlink, then executed the helper's next pathname read. The read returned `OUTSIDE` bytes. This is a permitted execution of the exact two operations, not an assertion about comment intent. An unsynchronized high-iteration race did not happen to land in the narrow scheduler window on this host; that does not make the distinct system calls atomic.

L3 applies to every adapter file read and to path components. Later digest rebinding can reject a cross-call manifest/artifact mismatch, but it does not make the adapter's claimed filesystem-containment property true or prevent acquisition of external self-consistent bytes. This is not a demonstrated signature-verification bypass, but it is an in-scope filesystem-containment defect.

## Trust, traversal, and regressions

**ESTABLISHED by source delta inspection.** This repair does not alter packagecatalog signature verification, GitHub acquisition, Goals GitHub provenance, publisher/currentness, or lexical traversal code. No evidence shows unsigned/wrong-key refusal, local-vs-GitHub provenance, or GitHub routing weakened. Those properties do not cure L3.

**INFERRED.** The prior digest-rebinding TOCTOU argument remains code-identical but is not an atomic filesystem-containment guard. Windows case/volume/normalization behavior is unmeasured and UNKNOWN.

## Disposition

**REVISION_REQUIRED.** Commit `23766e1` rejects stable symlinks but does not completely close L2's filesystem-containment class. It is not qualified for separately authorized atomic replacement. No installation is authorized.

No candidate source, evidence, artifact, installed binary, package, signature, deployment, ceremony, RecoveryIntent, Japetella, database, Git remote, or lifecycle state was modified by this review. Reviewer-only scratch material was removed after recording this evidence.
