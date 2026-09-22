# Astra Review #9 — clean/reproducible PRE-ACTIVATION candidate

**REVISION_REQUIRED**

## Scope and exact identity

This is an independent adversarial review of the repository state at `a9da0c3fcc8901ce34724013afa5b3c702f39a7e` on `feature/intent-evolution`.  At entry the worktree was clean (`git status --short --branch` reported only `## feature/intent-evolution...origin/feature/intent-evolution [ahead 10]`).  The claimed build-source commit is `40ba4141fcfe57a6f5cfb823ead2b7ffbf6661f9`, not HEAD.  `40ba414..HEAD` changes only bootstrap-v4 reports, records, rebuild log, and evidence-generation script; all 125 files in the source manifest have bytes identical at `d931809` and HEAD.

Review #8 accepted the uncommitted Repair #7 source, not this later clean artifact identity.  The repository record describes a limited clean-commit → rebuild → requalification → independent-verification transition.  Nothing in the inspected evidence authorizes installation, deployment, activation, Proposal v4 materialization, Gates A/B/C, or a push.  This review does not authorize any of those actions.

## Finding N18 — clean reconstruction does not reproduce the asserted artifacts

**ESTABLISHED.**  A fresh local clone detached at the exact claimed source commit `40ba414...`, with a newly created `GOCACHE`, produced the following artifacts using the documented commands (`go build ./cmd/praxis`, `go build ./packages/goals/plugin`, then `go run .../build_goals_package.go`):

| artifact | recorded/current SHA-256 | fresh clean-clone SHA-256 |
|---|---|---|
| Core | `0aa27c1da3c26177f57ba30dfea69a4fe9582057c57bb1cacf7ba136e1c76856` | `48630eaa5b7dba545aae659b17110f14f7bf3abdd30586c3cb4fd065e9fcb9a5` |
| Goals plugin | `a4c87cd2dbbb0e948fa1ddd5cc26661f0e01669685832bb9c83982f6b04b3c4c` | `d79ea8466f818acb134122f4ca2a2f95bd0c7f70c9d28cac236b124a2cc52224` |
| Package archive | `96bcd466ca8eeb43da301ff7152256c82359d81b59de8d02cebc164ddf1cb85a` | `6dc254b6ae04e23e54bfa8fc0414e9762ef3d46f590520e04c6e2703d60d007c` |
| Package manifest | `e69beead490802288a4c478eb0b62c3c961a9b1b9b53df63399d39b664a99cd3` | `a4a35cfe9ea1b956f5a0ab238809dc6bbd8adcc9c91ef5faad2569d8e4cc6c3f` |

The fresh core still embeds `vcs.revision=40ba414...` and `vcs.modified=false`.  The fresh and recorded binaries contain their respective absolute checkout paths (`/private/tmp/praxis-review9.m4HUSy/...` and `/Users/polliard/workspace/convergent-systems-co/praxis/...`) in source-file strings.  Therefore the mismatch is explained by an unpinned build-location input, not a stale Git revision or modified tree.  The downstream package identities necessarily differ because the plugin bytes differ.

The candidate's own `preactivation_evidence.py` only hashes the already-present ignored artifacts and linked records; it does not rebuild them or enforce a location-independent build.  Its PASS is internally consistent but is not independent clean-build verification.  Since artifacts are intentionally absent from Git, this path dependence prevents a future clean checkout from reconstructing the committed artifact identities.  This violates the claimed *reproducible*, byte-identical PRE-ACTIVATION state.

The smallest general defect class is **uncontrolled build-environment provenance**: the claimed artifact identity is a function of absolute checkout location, which is neither recorded nor constrained.  This is review-significant even though the source set is unchanged.

## Identity and provenance

**ESTABLISHED.**  Current stored-artifact hashes equal the values printed in the implementation report: core `0aa27c1…6856`, plugin `a4c87cd2…b4c4c`, archive `96bcd466…b85a`, manifest `e69beead…9cd3`.  The source-manifest SHA-256 is `65b3dd2ba52008c0ac48e185b7397a0e9d5dcb21211d033d188d745b653c1fd1`, records 125 files, and every current file matches its listed hash.  Other current evidence hashes are in the evidence record.

**ESTABLISHED.**  `candidate-activation-requirements.json` has status `COMPLETE_PRE_ACTIVATION_CANDIDATE`, records `vcs_modified:false`, and its `not_performed` list retains installation, deployment, activation-manifest persistence, Proposal v4 operations, and Gate A/B/C decisions. No `praxis-human-interface-proposal-v4.json` exists below the human-interface tree.

**INFERRED.** Local Git evidence supports that post-`40ba414` commits are evidence/tooling only and that the inspected tree is unpushed relative to its configured origin.  It cannot prove a historical non-action outside available local state.

## Security reassessment

**ESTABLISHED, limited to executed/audited scope.** The current source has N17's intended resolver structure: `ResolvePackagePublishAuthority` enumerates `currentAuthorityGenerations` and requires current ancestry before return; both signing entry points resolve it. `TestRetiredDelegatedPublisherGenerationDoesNotResolvePackagePublishAuthority`, the package-manager equivalent, the immutable-consumer registry, and the retired-root owner-authentication test passed on this candidate. In an isolated scratch clone, replacing the resolver's current enumeration with `ListAuthorityGenerations` and removing the lineage guard caused that retired-publisher test to fail by resolving the retired authority.  This establishes the tested guard is load-bearing; it does not expand Review #8's audit to mutable `PublisherGeneration` state.

**ESTABLISHED, measured Darwin scope.** With `PRAXIS_REQUIRE_KEYCHAIN=1` and real login-Keychain access, targeted opaque-file replay and re-key/interruption/undo/pending-state tests in `internal/goalstore` and `internal/crypto` passed. The first sandboxed run was unable to write a Keychain password item (`status 100001`); the same isolated tests passed when run with the platform Keychain facility. This neither establishes portability beyond the measured Darwin environment nor resolves R-K2.

## Qualification and residuals

**ESTABLISHED.** The source did not change after the accepted Repair #7 source: independently comparing each manifest file at `d931809` and HEAD found zero changes. Thus historic source-level N17/mutation/probe evidence is relevant to source semantics, but it cannot establish the new artifact reproducibility claim. I reran the qualifications invalidated by the clean transition: clean reconstruction, source-manifest comparison, currentness guard tests, and real-Keychain target tests. I did not rerun the full expensive historical suite because its source premises did not change and this review already has a decisive clean-build counterexample.

Known residuals remain unchanged: bearer approvals survive retirement until expiry; `CheckAuthorityInForceInTx` succeeds without an FAA; `state.Store.PublisherGeneration` is mutable and outside the immutable-generation audit; goals-publication recovery integration needs `PRAXIS_GOALS_QUALIFICATION_ASSETS`; Keychain portability is unestablished off measured Darwin; and R-K2 remains the coordinated database/dedicated-Keychain/historical-login-password rollback residual.

## Disposition and non-actions

**REVISION_REQUIRED.** The claimed clean, reproducible PRE-ACTIVATION artifact identities are not independently reproducible from the exact committed source in a fresh checkout. The candidate therefore does **not** warrant the claimed reproducible PRE-ACTIVATION state. This disposition does not reject the source-level N17 or measured N16 guards, and it does not decide or authorize any later lifecycle transition.

No implementation, existing tests, candidate evidence, ignored candidate artifacts, active installation, commit, push, install, deploy, activation, Proposal v4 materialization, or Gate A/B/C decision was changed by this review. Scratch-only mutations and builds are documented in the evidence record.
