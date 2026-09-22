# Astra Review #7 evidence

Review date: 2026-09-21.  Candidate base HEAD: `ff14600`.

## Identity and provenance (ESTABLISHED)

Independent SHA-256 recomputation:

| Item | SHA-256 |
|---|---|
| candidate core | `a57fbce914bf3cbd82f283f5aee2c4867c022f4385bda6bfcb2316a8a1c2fdd2` |
| candidate Goals plugin | `6e10e616c53842f9e9841737fd8cf01a96b4443661eb1132fe0fda77db142fb4` |
| package archive | `ed9f6e230dfdd5d4776755c7ffb05cee3d0af384115135b2fe6c71b10dd58cab` |
| package manifest | `7529e58f5e13a3744594398f212df246e11e0005ee23726c26463da000c47bbf` |
| source manifest | `ffc63fbb33f96896f864def29686b7face5835c268bba6f52e23cfc822589e20` |
| qualification results | `428cc54c11af0b2df5839d3127344ae163e9922598db5189bf379d996a6b981b` |
| activation requirements | `42ba9698401857ea1d7304daf18ef56b4210eafba038f4311bba79e319b68cb0` |
| pre-activation verification | `87d07e0ee7d23ca7c9c800610f09dc8800585505045795c31ebd6d0f152c2b99` |
| active `/Users/polliard/bin/praxis` | `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7` |

The source manifest lists 113 files and every listed source file recomputed to its recorded digest.  The activation record says `COMPLETE_PRE_ACTIVATION_CANDIDATE`; no Proposal v4 materialization was found.  HEAD remains `ff14600`; candidate changes are uncommitted.  This establishes artifact distinction and candidate provenance, but cannot independently prove an historical non-action such as a prior push that left no local trace.

The supplied verifier writes its result into candidate evidence, so it was intentionally not run in place.  Its predicates were independently checked without writes: candidate/package/source identities, all 113 source digests, status, and absence of Proposal v4.  Therefore its PASS is verified as a result of those predicates, not treated as authority.

## N16 and re-key state machine

Source inspection establishes the intended protocol: pending password before re-key, file re-key, current-item promotion, pending removal (`internal/crypto/faa_keychain_darwin.go:575-615`); recovery first tries current then promotes pending only if it unlocks the file (`:495-552`); `Set` read-backs its anchor write and rotates every non-genesis advance (`:727-771`); `Revert` rotates as well (`:773-790`).  An opaque pre-advance file presented with only the post-advance current password reaches the corrupt-unlock path, rather than reconstituting its historical state.  This is an **INFERRED** source-level closure of N16.

An attempted independent real-Keychain execution with `PRAXIS_REQUIRE_KEYCHAIN=1` did not reach any transition: the initial temporary login-Keychain password-item write failed with status `100001`.  The harness correctly reported this as a failure, not a skip or mutant kill.  Hence real macOS N16 replay, each crash boundary, pending/current combinations, undo, and failed re-key are **UNKNOWN in this environment**.  No Keychain process/runtime state was deliberately retained.

## N15/selective erasure (ESTABLISHED where executed)

`go test ./internal/goalstore -count=1 -run 'Test(FAAWholeDatabaseRollbackFailsClosed|FAAReplayOfACopiedLivenessRowDoesNotRestoreARevokedDecision|FAAClassificationSurvivesErasureOfEveryEvidenceRow|GoalEvidenceRegistryCoversEveryGovernanceNamespace)$'` passed.  Source inspection confirms anchored classification is consulted before the sealed row and derivation (`internal/goalstore/safety_classification.go:52-84`) and that a missing protected proposal returns an error rather than a legacy classification (`internal/goalstore/faa_test.go:928-940`).  The full Repair-5 powerset result remains qualification evidence, not an independently completed run here.

## Qualification audit (ESTABLISHED scope; insufficient overall)

The final inventory has 304 mutants, 274 killed, 30 classified, zero stated unexplained survivors.  The documented R6.18/R6.19/R6.21 first-pass gaps were rerun in the Keychain family and recorded killed; R6.16 is a redundant layer with killed joint R6.16j; R6.02 is plausibly equivalent because re-keying a newly created file is fail-safe; R5.82 and R5.84 are plausibly redundant; R5.73 remains explicitly unmodeled by the real backend and is only covered by a test double.  These classifications are evidence claims, not independently re-executed here because the mandatory real-Keychain probe is unavailable.

Most importantly, the inventory has **zero mutations of `internal/goalstore/publisher_authority.go`**.  Thus its zero-unexplained-survivor claim says nothing about the counterexample below.

## N17: retired delegated publisher authority remains executable (ESTABLISHED source counterexample)

Smallest defect class: **an authority consumer treats an immutable generation's `State == active` as current and bypasses both generation liveness and FAA retirement.**

Reproduction trace on a current, unrolled-back store:

1. Create a package.publish delegated generation and its approving parent decision.
2. Retire that delegated generation through `SaveAuthorityGenerationInvalidation`.  This anchors a `generation_retired` fact, removes its liveness record, and persists the invalidation (`internal/goalstore/repository.go:1464-1482`).
3. Invoke `ResolvePackagePublishAuthority` with that generation's subject digest and an in-scope package.  It enumerates immutable generation records and accepts the retired child based on `State == active`; it loads only the parent decision with `LoadAuthorityDecision` and returns the child (`internal/goalstore/publisher_authority.go:24-60`).  It never calls `ValidateAuthorityGeneration`, `requireLive`, `governanceSnapshot`, or checks the child invalidation.
4. `publisher.SignWithPreview` invokes precisely this resolver immediately before the protected signing call (`internal/publisher/sign.go:281` via `ValidatePackagePublishAuthority`; the latter delegates to the same resolver).  A signature is therefore executable under the retired generation.

The contrast is explicit: `ValidateAuthorityGeneration` rejects an invalidation and calls `requireLive` (`internal/goalstore/repository.go:1485-1501`).  The package-publish resolver omits both checks.  This counterexample neither restores the database nor file nor any password item.  It is wholly inside D1, does not depend on R-K2, and remains available after Repair #6's N16 re-key succeeds.

## Boundary and non-actions

R-K2 remains the excluded coordinated rollback of database, dedicated Keychain file, and historical login-Keychain password state; it is not the finding.  N17 requires none of those.  No production file was repaired; no commit, push, installation, deployment, activation, Proposal v4 materialization, or Gate A/B/C decision was performed.
