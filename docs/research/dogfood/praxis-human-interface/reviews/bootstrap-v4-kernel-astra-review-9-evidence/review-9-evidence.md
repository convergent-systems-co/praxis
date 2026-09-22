# Astra Review #9 evidence

## Exact observed state

- Branch/HEAD: `feature/intent-evolution`, `a9da0c3fcc8901ce34724013afa5b3c702f39a7e`.
- Build-source commit recorded by candidate: `40ba4141fcfe57a6f5cfb823ead2b7ffbf6661f9`.
- Entry worktree: clean; branch was ten commits ahead of configured `origin/feature/intent-evolution`.
- `40ba414..HEAD` names only bootstrap-v4 evidence/report/generator paths. Comparing all 125 source-manifest paths at `d931809` and HEAD found no changed byte and no manifest mismatch.
- No Proposal v4 JSON path matched `docs/research/dogfood/praxis-human-interface/**/praxis-human-interface-proposal-v4.json`.

## Independently recomputed current file hashes

```
source-manifest.json                 65b3dd2ba52008c0ac48e185b7397a0e9d5dcb21211d033d188d745b653c1fd1
qualification-results.json           39a0ce0026592d698ab4a9cd10868f807c31155b08053ab5492113c71633bdb5
candidate-activation-requirements    fa5fe5176032e4a3e2ebcc0c325d53ec4c892018d848c2daf5e9cf8975ae68aa
preactivation-verification.json      2ad16e7f598eab6387ee00abc7254731f4905f947d1954f644031207e7a0fbd9
specification-bundle/manifest.json   5a33e46f15f59978ea72972a91879d3feff39355581fe48cb451d30c03a96215
artifacts/praxis-candidate           0aa27c1da3c26177f57ba30dfea69a4fe9582057c57bb1cacf7ba136e1c76856
artifacts/praxis-goals-plugin-candidate
                                      a4c87cd2dbbb0e948fa1ddd5cc26661f0e01669685832bb9c83982f6b04b3c4c
artifacts/package/praxis-package.tar.gz
                                      96bcd466ca8eeb43da301ff7152256c82359d81b59de8d02cebc164ddf1cb85a
artifacts/package/praxis-package.json
                                      e69beead490802288a4c478eb0b62c3c961a9b1b9b53df63399d39b664a99cd3
```

## Fresh reconstruction counterexample (N18)

An isolated `git clone --no-local` was checked out detached at `40ba414...`; it contained no ignored candidate artifacts. A fresh temporary `GOCACHE` was used for:

```
go build -o review-praxis ./cmd/praxis
go build -o review-goals-plugin ./packages/goals/plugin
go run ./docs/research/dogfood/praxis-human-interface/bootstrap-v4/verify/build_goals_package.go review-goals-plugin review-package
```

Output hashes: core `48630eaa5b7dba545aae659b17110f14f7bf3abdd30586c3cb4fd065e9fcb9a5`; plugin `d79ea8466f818acb134122f4ca2a2f95bd0c7f70c9d28cac236b124a2cc52224`; archive `6dc254b6ae04e23e54bfa8fc0414e9762ef3d46f590520e04c6e2703d60d007c`; manifest `a4a35cfe9ea1b956f5a0ab238809dc6bbd8adcc9c91ef5faad2569d8e4cc6c3f`.

`go version -m review-praxis` nevertheless reported `vcs.revision=40ba4141fcfe57a6f5cfb823ead2b7ffbf6661f9` and `vcs.modified=false`. `strings` locates `/private/tmp/praxis-review9.m4HUSy/...` in the fresh binary and `/Users/polliard/workspace/convergent-systems-co/praxis/...` in the recorded candidate. The documented builds use no `-trimpath` or other location-normalization contract.

## Guard and Keychain probes

- Candidate: `go test -count=1 ./internal/goalstore ./internal/goalspublication -run 'Test(RetiredDelegatedPublisherGenerationDoesNotResolvePackagePublishAuthority|InvalidatedDelegatedPublisherGenerationPreventsBuildSigningPreview|RetiredPackageManagerGenerationDoesNotResolveDeploymentAuthority|EveryImmutableGenerationConsumerIsClassified|RetiredRootShapedGenerationCannotAuthenticateTheAbandoningOwner|LoadCurrentAuthorityGenerationRefusesARetiredGeneration)$'` → PASS.
- Scratch only: replacing `currentAuthorityGenerations` with `ListAuthorityGenerations` in the publish resolver and removing `requireCurrentLineage` made `TestRetiredDelegatedPublisherGenerationDoesNotResolvePackagePublishAuthority` fail: `a retired delegated package.publish generation still resolves and would authorize signing`.
- Real-Keychain command (outside sandbox after sandbox Keychain status `100001`): `PRAXIS_REQUIRE_KEYCHAIN=1 go test -count=1 ./internal/goalstore ./internal/crypto -run 'Test(Repair6OpaqueKeychainFileReplayIsRefused|KeychainAnchorOpaqueFileReplayIsRefusedOnceTheAnchorHasAdvanced|Anchor.*(Rekey|Interrupted|Undo|Pending))'` → PASS for both packages.

The candidate `preactivation_evidence.py` was executed once; it completed PASS and left no Git diff. Its implementation writes `preactivation-verification.json`, so it is a deterministic self-consistency check, not an independent non-mutating rebuild verifier.
