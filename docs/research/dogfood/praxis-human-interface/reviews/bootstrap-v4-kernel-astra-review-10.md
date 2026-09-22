# Astra Review #10 — N18 location-independent clean PRE-ACTIVATION candidate

**ACCEPTED**

## Exact reviewed identity

This review applies to HEAD `9edca1e5735f77ab8a4b76431749c542aa338dc7` on `feature/intent-evolution`, with an initially clean worktree (the branch was fourteen commits ahead of its configured origin). The candidate's build-source commit is `25c73305fbca532e6b402b9d4aa1ed3412305169`.

**ESTABLISHED.** `git diff --name-only 25c7330..HEAD` names only bootstrap-v4 records/report/generator paths and the N18 build evidence. Independently comparing that range against every path in the 125-entry source manifest returned zero paths. The new, versioned build recipe was introduced before the source-identity commit (`0a24b31`); it is unchanged at `25c7330` and later. No unreviewed implementation-source semantic change is present in the clean transition.

## N18 reproduction

**ESTABLISHED.** I made two new clean clones, detached each at `25c7330`, at different absolute paths and with independent fresh `GOCACHE` directories. I invoked the committed script from each checkout and wrote outputs to distinct directories *outside* both worktrees. All four `cmp` comparisons and SHA-256 values were identical, and equal the candidate values:

| artifact | SHA-256 |
|---|---|
| Core | `42cb404ff30b77b375505a2593f5541b9be63587b39e87c1792d9231851de821` |
| Goals plugin | `08514f2078c0e4a7a9093e41b4a78ec72069f7053a6e5bf670ff426ec1888829` |
| Package archive | `06e0ebaea3c0ad549d7efdf37f9894283360b36b9dfa4ccf8fa9e0a45d96cc62` |
| Package manifest | `e8894b79d8e7589cff37e98f93e31e22adb7fba885a02c19ac46156e534111d2` |

`go version -m` on the independently built core records the expected source revision, `vcs.modified=false`, and `CGO_ENABLED=1`. The script actually invokes `go build -trimpath` for both core and plugin; it has no wrapper or extra location-remapping flags. Direct `strings` searches found zero occurrences of either checkout root or any temporary/workspace absolute source-root prefix in either binary. `otool -L` shows the expected system Security/CoreFoundation and system-library load paths, not source checkout paths. The location-dependent N18 counterexample no longer reproduces in this measured Darwin/Go-toolchain environment.

The script should be invoked with output outside the worktree or in the candidate's ignored `artifacts/` path. In a scratch-only exploratory invocation with output in an unignored worktree subdirectory, the core was built before the tree became dirty but the later plugin saw `vcs.modified=true`. This does not affect the qualified candidate or the external-output reproduction above, but it bounds the recipe's documented `<output-dir>` argument; no candidate source or artifact was changed during that probe.

## Identity, evidence, and qualification

**ESTABLISHED.** The stored artifacts, source manifest, qualification result, activation record, preactivation record, and specification manifest recompute to the identities recorded in the accompanying evidence. The 125 manifest files recompute exactly. The qualification generator's predicates were inspected against their actual input logs: focused and race logs each show twelve relevant Go packages with no `FAIL`; Python records `1683 passed, 1 skipped` and `EXIT=0`; vet and diff checks report exit zero; and probe summaries contain all eight matching outcome sets plus the fourteen-scenario image replay. The N18-specific log has the correct `25c7330` source identity, four `BYTE-IDENTICAL` results, and zero root-string matches. No copy-forward inconsistency was found.

The generator is a record generator, not an independent rebuild oracle: it reads its evidence files and hashes on-disk artifacts. This review independently supplied the missing fresh-clone rebuild rather than treating its PASS as sufficient.

## Safety continuity and residuals

**ESTABLISHED, measured scope.** The source-level currentness target tests for N17 (publish signing, package-manager equivalent, immutable-consumer registry, retired root owner authentication) pass at this HEAD. Required real-Keychain N16/re-key tests pass with `PRAXIS_REQUIRE_KEYCHAIN=1` on this Darwin environment. The build-only change did not alter the 125 source files and these direct checks found no reopened path.

The six retained boundaries remain exactly boundaries, not cleared claims: package approvals remain bearer approvals through expiry; `CheckAuthorityInForceInTx` returns nil without an enabled FAA; mutable `state.Store.PublisherGeneration.State` remains outside the immutable-generation audit; Goals-publication exact-artifact recovery integration still skips without `PRAXIS_GOALS_QUALIFICATION_ASSETS`; portability beyond measured Darwin remains unestablished; and installation remains a separately authorized action. R-K2 also remains outside this review's closure.

No Proposal-v4 JSON exists in the human-interface tree. Candidate records remain `COMPLETE_PRE_ACTIVATION_CANDIDATE` and list core installation/replacement, package deployment, activation-manifest persistence, Proposal-v4 actions, and Gates A/B/C as not performed. The active `/Users/polliard/bin/praxis` hash remains `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7`.

## Disposition and non-actions

**ACCEPTED** for this exact `9edca1e` PRE-ACTIVATION candidate and its `25c7330` source/build identity. It establishes location-independent reproduction on the measured environment; it does not authorize installation, deployment, activation, Proposal v4 materialization, a push, or any Gate A/B/C decision, and it does not clear untested platforms or the retained residuals.

No candidate implementation, test, evidence, artifact, active installation, Git commit, remote, deployment, activation, Proposal v4, or Gate was changed by this review. Only this new independent review artifact and its evidence were created.
