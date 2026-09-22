# Consolidated guard / mutation inventory (repair 5)

Generated from `mutation-inventory.json` (harness: `mutation/harness.py`; catalogue: `mutation/catalogue.py`). Every mutation is applied to a SCRATCH COPY of the repository (never the real tree); each file is restored and re-hashed (`RESTORE-CHECK OK` in every run log). A guard is KILLED only if at least one relevant regression fails; kills attributable only to load-sensitive unrelated tests do not count.

## Universe and scope of the claim

The universe is the set of safety guards in the PRE-V4 kernel non-test source of `pkg/contracts`, `internal/bootstrapv4`, `internal/state`, `internal/faa`, `internal/crypto` (the Keychain anchor), `internal/goalstore`, `internal/goaldrive`, `internal/lifecycle` and the lifecycle/decide/settlement/inspect/re-anchor surfaces of `cmd/praxis`, **as enumerated in `mutation/catalogue.py`**: Repair 3's 155 guards and Repair 4's guards carried forward and re-run against the Repair 5 source (four Repair 4 guards were re-expressed because Repair 5 rewrote the code they mutate), plus **76 Repair 5 guards** for the forward authority anchor: chain verification (installation, predecessor, content head, gaps, genesis, re-anchor bridge, completeness), store-versus-anchor comparison, the snapshot and its retry rule, the admission stamp, the write-ahead append (idempotence, anchor advance, undo on commit failure, writer lock, only-facts), anchoring of revocation, invalidation, succession and classification, freshness at every consumer (decision load, liveness, root resolution, recovery admission, the in-transaction guard, the governed repair guard), the registry evidence kinds, the missing-proposal rule, the governed re-anchor (plan binding, sequence, root re-admission and its completion, orphan preservation, root selection, interactive/OS-user/confirmation checks), the state primitives, the composition root and the Keychain backend.

**What mutation does not model.** A syntactic mutation cannot represent an adversary who restores earlier state. Rollback and freshness are covered by explicit state-transition and enumeration tests (whole-database rollback, replay, truncation, gap, corruption, every anchor state, every crash and failure interleaving, concurrency, the real Keychain end to end, the 256-subset evidence powerset, the 4096-store mixed-snapshot enumeration); the mutations in this inventory prove that each guard those tests rely on is load-bearing. No claim of exhaustive rollback resistance is made from mutation counts.

Still excluded: raw OS signals; forging by a holder of the installation storage key or an approved Keychain accessor (accepted bootstrap trust root); anchor destruction combined with a whole-installation wipe and a machine-level restore of Keychain and database together (residuals, `design/forward-authority-anchor.md` section 7); the non-Unix validation fallback; Windows and Linux (only darwin/arm64 was run); states this platform will not produce on demand.

## Counts

| Measure | Count |
|---|---|
| mutations applied (total) | 269 |
| single-guard mutations (= enumerated guards) | 258 |
| joint mutations (redundant layers removed together) | 11 |
| killed by a relevant regression | 244 |
| survived: redundant layer (another layer enforces it; a joint mutation is killed) | 14 |
| survived: redundant by construction | 7 |
| survived: equivalent mutant | 2 |
| survived: unmodeled on this platform | 2 |
| survived with no explanation | 0 |
| **surviving total (all classified)** | 25 |
| Repair 5 guards killed / total | 74 / 76 |

Counted results (union, later files override earlier ones; the exact list is `result_sources` in `mutation-inventory.json`): the Repair 3-derived and first Repair 5 mutations come from `results-r5-full.json` (one fresh pass of the whole catalogue against the Repair 5 source before the final reader-ordering change; log `run-r5-full.log`) with the identifiers re-run afterwards (`results-r5-rerun.json` to `results-r5-rerun4.json`). **Every Repair 4 and Repair 5 mutation (114 identifiers, `R4.*` and `R5.*`) was then re-run against the FROZEN final source** (`results-final5b.json`, log `run-final5b.log`) and three were re-run after the last test strengthening (`results-final5c.json`: R5.18, R5.75, R5.76). Later files override earlier ones. Mutations of Repair 3 origin (`B*`, `N*`, `M*`) ran on the snapshot that differs from the final source only in `internal/goalstore/governance.go` (reader ordering and the writer-lock barrier) and `internal/state/secure_blob.go` (`Store.WithWriteLock`); none of those guards touches that code. The first pass ran Repair 5 mutations with `-short` (the two exhaustive enumerations skipped) and re-ran any survivor in full before calling it one. Every mutation that first survived, failed to build or had a stale definition was handled as follows, and nothing was reclassified to make a number:

* **Test gaps closed by new regressions** (each then killed): R5.01, R5.02, R5.04, R5.05 (each chain rule proven on its own with validly sealed facts), R5.15 (an anchor naming another installation), R5.18 (an invalid chain is not retried), R5.23 and R5.24 (initialisation refusals), R5.34 (succession anchored, rollback by replay refused), R5.36 (a root retired by fact), R5.37 (a root admitted before the re-anchor), R5.40 and R5.41 (classification anchored when marked and when only derived), R5.51 and R5.52 (the in-transaction guard needs the decision and the generation each on its own), R5.53 (the repair guard hook; its first test was vacuous because the fixture lacked a decision liveness record, found by adding a positive control), R5.57 (an already-complete re-admission is not rewritten), R5.59 and R5.60 (root selection), R5.62 and R5.63 (the fact append's undo and namespace guard), R5.66 and R5.67 (the ceremony's interactive and OS-user checks were first tested with a wrong confirmation, which refused for a different reason; each is now offered the correct digest-bound confirmation), R5.74 (a Keychain item holding another installation's state), R4.25 (the surviving-generation derivation, once every other evidence kind is also removed), R5.75 and R5.76 (the reader barrier: a real disagreement behind the writer lock is refused at once and a lag that resolves behind the lock is accepted without backing off; both tests now give the reader a two- or three-second backoff so that a reader that skips the lock path is caught by elapsed time; found as survivors on the final-source re-run and killed after the test was tightened).
* **Build failures corrected, not counted:** R5.19, R5.32, R5.56, R5.58, R4.25 (unused variable after the mutation). Stale R4.* definitions (the code they mutated was rewritten) were re-expressed against the new code; the first-pass ANCHOR-ERROR results are preserved and not counted.
* **Classified survivors** are listed below with reasons and, where they are redundant layers, the joint mutation that is killed.

## Survivors (all)

| Id | Guard | Class | Why |
|---|---|---|---|
| N1.09 | Attach classifies its Goal (mark at attach) | REDUNDANT-LAYER | attach re-marks the Goal that the safety proposal already classified; joint mutation N1.09j (proposal+attach) is killed by TestKernelRepair3N1AProposalClassifiesItsGoalBeforeAnythingIsAccepted / TestKernelRepair3N1Stripping... |
| N2.10 | Gate decision owner/root authority current | REDUNDANT-BY-CONSTRUCTION | the gate-completion currency re-check of owner/root authority is masked by the plan-level governing-authority check that runs first in the same call (prepareAuthorityGate, B9.12/B9.09 killed) |
| M12.01 | claim == selected unit before publication | REDUNDANT-LAYER | the pre-publication claim==selected check cannot be reached with a mismatching claim because the checkpoint inspection (M12.03, killed by its own message assertion) and settlement (M12.02, killed by a direct settleCompletion test) both refuse first; joint M12.04 is killed |
| B9.11 | decision by installation owner | REDUNDANT-BY-CONSTRUCTION | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) the owner principal and the governance scope both derive from the same installation digest; scope is checked first (B9.10 killed), so a scope-correct decision cannot be by another owner short of a forged record |
| N4.04 | trailing content refused (walk) | REDUNDANT-LAYER | trailing-content refusal exists in the type-directed walk and again in the decode; each alone is masked by the other; joint N4.06 is killed |
| N4.05 | trailing content refused (decode) | REDUNDANT-LAYER | see N4.04 (joint N4.06 killed) |
| N5.02b | export verified right after checkout (creation time) | REDUNDANT-LAYER | creation-time export verification only fails fast; the same predicate set is re-proved after the run (N5.02 killed) and the content is re-hashed there |
| N5.05 | export tree equals qualified tree | REDUNDANT-BY-CONSTRUCTION | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) an export's HEAD^{tree} is a function of its HEAD commit, which N5.04 compares; the tree comparison cannot differ while the commit is equal |
| N5.07 | export status clean | REDUNDANT-LAYER | export status check is covered by the content re-hash and the hint scan; joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 |
| N5.08 | export gained no untracked/excluded files | REDUNDANT-LAYER | untracked/excluded-file scan is covered by the content re-hash (which adds all files); joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 |
| N5.11 | committed validator digest at checkpoint (bind) | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) the committed-validator digest at the checkpoint is re-checked on the exported blob (N5.12) and the whole export tree is re-hashed; the two digest layers removed together are killed (N5.13) |
| N5.12 | exported validator digest | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) the exported validator digest is re-checked on the committed blob (N5.11 killed) and the whole export tree is re-hashed after the run; joint N5.13 is killed |
| N5.14 | validator executable mode at checkpoint | REDUNDANT-BY-CONSTRUCTION | a validator that is not an executable regular file cannot be executed: the OS refuses the exec and the run fails closed. The explicit mode checks (bind N5.14, export N5.14x) only give an earlier, clearer message; even their joint mutation N5.14j survives because exec permission still refuses |
| N5.14x | exported validator executable mode | REDUNDANT-BY-CONSTRUCTION | see N5.14 |
| N5.14j | joint: validator executable mode (bind + export) | REDUNDANT-BY-CONSTRUCTION | see N5.14 |
| N5.15 | bind: HEAD is the checkpoint | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) HEAD==checkpoint is asserted by four layers (bind, bind branch ref, ReadCheckpointArtifact, PushAndVerify); joint N5.18 of the bind/branch/push layers is killed |
| N5.16 | bind: local branch points at checkpoint | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) see N5.15 (joint N5.18 killed) |
| N5.21 | publication pushes the exact qualified object id | EQUIVALENT | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) PushAndVerify has already proved HEAD==head, so pushing HEAD or the object id publishes the same commit |
| N6.05 | collector keeps draining after overflow (no deadlock) | EQUIVALENT | after overflow the buffer-limit branch sets the same flags and cancels again and still returns len(p): the early branch only saves work |
| N3.07 | kernel reports the signature valid | UNMODELED | the kernel never reported CS_VALID cleared in any scenario constructible here (flags 0x22020201 throughout); the check fails closed on a state this platform will not produce on demand |
| R4.23 | owner-decision authority requires issuing generation liveness | REDUNDANT-LAYER | owner-decision issuing-generation liveness is checked in ValidateAuthorityGeneration (killed as R4.15) and again in the plan-authority path; the plan-authority check alone is masked; joint R4.23j is killed by the generation invalidation/liveness regressions |
| R4.28 | derivation fails closed on an unreadable candidate | REDUNDANT-LAYER | an undecryptable proposal row is refused at the load AND again when its (empty) payload fails to decode; each alone is masked by the other; joint R4.28j is killed by TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.29 | derivation fails closed on an unreadable generation | REDUNDANT-LAYER | an undecryptable generation row is refused at the generic load AND again when the generation is loaded for extraction; each alone is masked by the other; joint R4.29j is killed |
| R5.64 | the fact append takes the writer lock | REDUNDANT-BY-CONSTRUCTION | the store opens every transaction with _txlock=immediate (internal/state/sqlite.go), so the writer lock is already held when the builder runs; the explicit lock statement is defence in depth. Serialisation of appenders on separate connections is asserted by TestAppendGovernanceFactSerialisesAppendersOnSeparateConnections, and a second appender is also refused by the anchor's compare-and-set (R5.21) and the sequence-keyed primary key |
| R5.73 | keychain: read-back verified | UNMODELED | the real Keychain never reports a write it did not make, so the read-back guard cannot be exercised against it; the same guard on the test double (a backend that reports success without persisting) is killed by TestFAAAnchorWriteThatDoesNotStickIsRefused through the repository, and the Keychain compare-and-set guards around it are killed (R5.70-R5.72, R5.74) |

## All mutations

| Id | Guard | Invariants | Verdict | Killing tests (first 2) |
|---|---|---|---|---|
| N1.01 | Save refuses safety-bearing plan (B7) | I2,I9 | KILLED | TestKernelRepair2B7BaselinePersistenceSurfacesRefuseSafetyBearingPlans |
| N1.02 | Save refuses any plan for a classified Goal | I9 | KILLED | TestKernelRepair3N1StrippingTheSafetyBindingCannotDowngradeAClassifiedGoal, TestRepair4N9ClassificationRowDeletionStillRefusesStrippedImport |
| N1.03 | Proposal refuses legacy proposal for classified Goal | I9 | KILLED | TestKernelRepair3N1StrippingTheSafetyBindingCannotDowngradeAClassifiedGoal, TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.04 | Review refuses legacy review for classified Goal | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.05 | Legacy acceptance refuses classified Goal | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.06 | Authority-backed acceptance bridge refuses classified Goal | I9 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestSafetyClassificationRefusesTheAuthorityBackedAcceptanceBridgeOnItsOwn |
| N1.07 | Attach refuses legacy plan for classified Goal | I9 | KILLED | TestFAAConcurrentAppendersAndReadersKeepAValidChain, TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.08 | Safety proposal classifies its Goal (mark at proposal) | I9 | KILLED | TestRepair4N9ClassificationRowDeletionStillRefusesStrippedImport, TestRepair5EvidencePowersetClassificationQualification |
| N1.09 | Attach classifies its Goal (mark at attach) | I9 | REDUNDANT-LAYER |  |
| N1.10 | Classification kernel-version mismatch refused (consistency) | I9 | KILLED | TestSafetyConsistencyRefusesAnotherKernelVersion |
| N1.11 | Classification kernel-version mismatch refused (mark) | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.12 | Missing binding on classified Goal refused | I9 | KILLED | TestEmittedNextActionsAreExecutableProductContracts, TestFAAKeychainBackendDetectsWholeDatabaseRollbackEndToEnd |
| N1.13 | Attach binds accepted plan to its Goal (R1-G) | I9,I2 | KILLED | TestKernelRepair3AcceptedPlanCannotAttachToADifferentGoal |
| N1.14 | Controller refuses classified-but-unbound generation | I9,I7 | KILLED | TestKernelRepair3ClassifiedGoalWithoutSafetyBindingIsRefusedBeforeAnyWorker, TestKernelRepair3MaterializationRefusesAClassifiedGoalWithoutTheBinding |
| N1.15 | Kernel-shaped content refused (WorkPlan.Validate) | I9 | KILLED | TestKernelShapedContentWithoutSafetyBindingIsRefused |
| N1.16 | Kernel-shaped content refused (proposal Validate) | I9 | KILLED | TestKernelShapedContentWithoutSafetyBindingIsRefused |
| N1.17 | Kernel-shaped: explicit kind | I9 | KILLED | TestRejectKernelShapedWithoutSafetyEachTellRefusesIndependently |
| N1.18 | Kernel-shaped: preserved specification bytes | I9 | KILLED | TestRejectKernelShapedWithoutSafetyEachTellRefusesIndependently |
| N1.19 | Kernel-shaped: gate provenance | I9 | KILLED | TestRejectKernelShapedWithoutSafetyEachTellRefusesIndependently |
| N1.20 | Kernel-shaped: relationship specification | I9 | KILLED | TestKernelShapedContentWithoutSafetyBindingIsRefused |
| N1.21 | Runtime admission fence uses classification | I9,I2 | KILLED | TestKernelRepair3RuntimeRefusesADowngradedGenerationBeforeAdmission |
| N1.22 | Materialization refuses safety-bearing turns via classification | I9,I1 | KILLED | TestKernelRepair3MaterializationRefusesAClassifiedGoalWithoutTheBinding |
| N1.09j | joint: both classification writers (proposal + attach) | I9 | KILLED | TestRepair4N9ClassificationRowDeletionStillRefusesStrippedImport, TestRepair5EvidencePowersetClassificationQualification |
| N2.01 | SealCompletion only for safety-bearing generation | I11 | KILLED | TestCompletionSealsAuthenticateExactBytesOnly |
| N2.02 | SealCompletion requires verified activation | I11,I6 | KILLED | TestCompletionSealsAuthenticateExactBytesOnly |
| N2.03 | SealCompletion refuses a conflicting existing seal | I11 | KILLED | TestCompletionSealsAuthenticateExactBytesOnly |
| N2.04 | LoadSealedCompletion checks digest equality | I11 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestSealedCompletionWhoseBytesDoNotMatchItsKeyIsRefused |
| N2.05 | Gate completion cites the request its dossier implies | I11 | KILLED | TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState |
| N2.06 | Gate request must be durably recorded (digest equality) | I11 | KILLED | TestKernelRepair3N2GateCompletionRefusesARecordedRequestThatDiffersFromTheImpliedOne |
| N2.07 | Gate decision citation equality | I11 | KILLED | TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState |
| N2.08 | Gate completion ceremony resolves | I8,I11 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState |
| N2.10 | Gate decision owner/root authority current | I3,I8,I11 | REDUNDANT-BY-CONSTRUCTION |  |
| N2.11 | Gate decision outcome must be approve | I11 | KILLED | TestKernelRepair3N2CompletionCitingARejectedGateDecisionIsNotEffective |
| N2.12 | Revoked decision is not effective (LoadAuthorityDecision revocation) | I3,I11 | KILLED | TestKernelRepair3N2RevokedGateDecisionKeepsHistoryButStopsAuthorizingWork |
| N2.13 | Consumption: completion belongs to this Goal generation | I11 | KILLED | TestKernelRepair3ConsumptionChecksGenerationAndBytesIndependentlyOfTheStore |
| N2.14 | Consumption: sealed bytes equal ledger row | I11 | KILLED | TestKernelRepair3ConsumptionChecksGenerationAndBytesIndependentlyOfTheStore |
| N2.15 | Consumption: seal must exist | I11 | KILLED | TestKernelRepair3CompletionSealedForAnotherGenerationIsRefused, TestKernelRepair3ConsumptionChecksGenerationAndBytesIndependentlyOfTheStore |
| N2.16 | Consumption: authentication skipped entirely | I11 | KILLED | TestKernelRepair3CompletionSealedForAnotherGenerationIsRefused, TestKernelRepair3ConsumptionChecksGenerationAndBytesIndependentlyOfTheStore |
| N2.17 | Consumption: gate citations required | I11 | KILLED | TestKernelRepair3GateCompletionWithoutItsCitationsIsRefused |
| N2.18 | Consumption: gate lineage re-verified | I11 | KILLED | TestKernelRepair3DemotionDoesNotFollowNonBlockingRelationships, TestKernelRepair3DemotionPropagatesToUnitsThatCompletedUnderTheGate |
| N2.19 | Consumption: not-effective gate demoted to history | I3,I11 | KILLED | TestKernelRepair3DemotionDoesNotFollowNonBlockingRelationships, TestKernelRepair3DemotionPropagatesToUnitsThatCompletedUnderTheGate |
| N2.20 | Consumption: demotion propagates over hard dependencies | I3,I11 | KILLED | TestKernelRepair3DemotionDoesNotFollowNonBlockingRelationships |
| N2.21 | Consumption: effective/historical split | I3,I11 | KILLED | TestKernelRepair3DemotionDoesNotFollowNonBlockingRelationships, TestKernelRepair3DemotionPropagatesToUnitsThatCompletedUnderTheGate |
| N2.22 | Recording seals before append | I11 | KILLED | TestGoalGateCompletesThroughTheSupportedOwnerCeremony, TestGoalGateOwnerCeremonyAdversarialRejections |
| N2.23 | Controller prepare consumes authenticated completions (select) | I11 | KILLED | TestKernelRepair3ForgedLedgerGateCompletionIsRefusedNotTrusted, TestKernelRepair3ForgedLedgerUnitCompletionIsRefused |
| N2.24 | Controller consumes authenticated completions (explicit objective) | I11 | KILLED | TestKernelRepair3ExplicitObjectiveMustBeCurrentlyRunnable |
| N2.25 | Goal candidate derived from authenticated completions | I11 | KILLED | TestKernelRepair3GoalCandidateIsDerivedOnlyFromEffectiveCompletions |
| N2.26 | Explicit objective must be currently runnable | I3,I11 | KILLED | TestKernelRepair3ExplicitObjectiveMustBeCurrentlyRunnable |
| N7.01 | Candidate completions: historical refuses settlement | I3,I11 | KILLED | TestKernelRepair3GoalCompletionCandidateMustEqualTheAuthenticatedCompletions |
| N7.02 | Candidate completions: count equality | I11 | KILLED | TestKernelRepair3GoalCompletionCandidateMustEqualTheAuthenticatedCompletions |
| N7.03 | Candidate completions: per-unit equality | I11 | KILLED | TestKernelRepair3GoalCompletionCandidateMustEqualTheAuthenticatedCompletions |
| N7.04 | Settlement operation verifies candidate | I11 | KILLED | TestKernelRepair3N7SettlementSurfacesRefuseAForgedCompletionCandidate |
| N7.05 | Evaluation operation verifies candidate | I11 | KILLED | TestKernelRepair3N7SettlementSurfacesRefuseAForgedCompletionCandidate |
| N7.06 | Inspect consumes authenticated completions | I11 | KILLED | TestKernelRepair3InspectDoesNotDisplayAForgedCompletionAndIsNotDrivable |
| N7.07 | Inspect: authentication error makes generation not drivable | I11 | KILLED | TestKernelRepair3InspectDoesNotDisplayAForgedCompletionAndIsNotDrivable |
| I10.01 | authorizeEffect: activation predicate | I6,I10 | KILLED | TestKernelRepair3EveryOutwardEffectIsAuthorizedBeforeItHappens |
| I10.02 | authorizeEffect: verifier configured | I6,I10 | KILLED | TestKernelRepair3EffectBoundaryFailsClosedWithoutAnActivationVerifier |
| I10.03 | authorizeEffect: governing authority | I3,I10 | KILLED | TestKernelRepair3EveryOutwardEffectIsAuthorizedBeforeItHappens, TestKernelRepair3GateCompletionIsAuthorizedAtTheMomentItIsRecorded |
| I10.04 | authorizeEffect: lease held | I10 | KILLED | TestKernelRepair3LostLeaseRefusesTheEffect |
| I10.05 | authorizeEffect: content binding | I5,I10 | KILLED | TestKernelRepair3EveryOutwardEffectIsAuthorizedBeforeItHappens |
| I10.06 | call site: before checkpoint publication | I10 | KILLED | TestKernelRepair3EveryOutwardEffectIsAuthorizedBeforeItHappens, TestKernelRepair3LostLeaseRefusesTheEffect |
| I10.07 | call site: before completion record (M2b/M2d/M6b) | I3,I6,I5,I10 | KILLED | TestKernelRepair3EveryOutwardEffectIsAuthorizedBeforeItHappens, TestKernelRepair3LostLeaseRefusesTheEffect |
| I10.08 | call site: before gate completion (M2c) | I3,I10 | KILLED | TestKernelRepair3GateCompletionIsAuthorizedAtTheMomentItIsRecorded |
| I10.09 | post-publication settlement failure recorded (published-but-ungoverned) | I10 | KILLED | TestKernelRepair3EveryOutwardEffectIsAuthorizedBeforeItHappens |
| I10.10 | lost lease records nothing after publication | I10 | KILLED | TestKernelRepair3LostLeaseRefusesTheEffect |
| M12.01 | claim == selected unit before publication | I1 | REDUNDANT-LAYER |  |
| M12.02 | claim == selected unit at settlement | I1 | KILLED | TestKernelRepair3SettlementRefusesANonSelectedClaimByItself |
| M12.03 | claim == selected unit at checkpoint inspection | I1 | KILLED | TestKernelRepair3ClaimOfANonSelectedUnitPublishesNothing |
| M12.04 | joint: all three claim==selected layers | I1 | KILLED | TestKernelRepair3ClaimOfANonSelectedUnitPublishesNothing, TestKernelRepair3SettlementRefusesANonSelectedClaimByItself |
| B5.01 | accepted plan refuses supplied Completed flag (M1a) | I1 | KILLED | TestKernelRepair2B5AcceptanceCannotInjectCompletedGate, TestSafetyPlanRejectsSuppliedCompletion |
| B5.02 | ApplyCompletions clears supplied flag (M1b) | I1 | KILLED | TestCompletionIsDerivedOnlyFromQualifiedLedgerEvidence |
| B5.03 | VerifyPlanCompletions structural check (M1c) | I1,I4 | KILLED | TestCompletionIsDerivedOnlyFromQualifiedLedgerEvidence |
| B9.01 | controller verifies governing authority (M2a) | I3 | KILLED | TestKernelRepair3EveryOutwardEffectIsAuthorizedBeforeItHappens, TestKernelRepair3GateCompletionIsAuthorizedAtTheMomentItIsRecorded |
| B9.02 | prepare verifies governing authority | I3 | KILLED | TestKernelRepair3RevokedAuthorityIsRefusedBeforeTheWorkerRuns, TestValidationCancellationTerminatesDescendants |
| B9.03 | gate reconcile: governing authority (M3) | I3 | KILLED | TestKernelRepair2B9RevokedGoverningAuthorityStopsFutureExecution |
| B9.04 | gate reconcile: activation (B15) | I2,I6 | KILLED | TestFAAConcurrentAppendersAndReadersKeepAValidChain, TestKernelRepair2B15GateReplayRequiresActivationAtTheSharedBoundary |
| B9.05 | plan authority lineage present | I3 | KILLED | TestFAAConcurrentAppendersAndReadersKeepAValidChain, TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.06 | plan authority request is the acceptance request | I3 | KILLED | TestFAAConcurrentAppendersAndReadersKeepAValidChain, TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.07 | plan authority request carries ceremony+activation binding | I3,I8 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.08 | plan authority decision matches plan lineage | I3 | KILLED | TestFAAConcurrentAppendersAndReadersKeepAValidChain, TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.09 | baseline is the persisted generation | I3 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.10 | decision owner scope | I8 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.11 | decision by installation owner | I8 | REDUNDANT-BY-CONSTRUCTION |  |
| B9.12 | decision authority generation valid (root revocation) | I3 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.13 | gate decision issued by current root | I3,I8 | KILLED | TestFAAKeychainBackendDetectsWholeDatabaseRollbackEndToEnd |
| B8.01 | protected decision requires resolvable ceremony record | I8 | KILLED | TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.02 | ceremony record must match decision | I8 | KILLED | TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState, TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.03 | ceremony record: owner/root lineage | I8 | KILLED | TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.04 | ceremony record: enrolled OS user | I8 | KILLED | TestFAAConcurrentAppendersAndReadersKeepAValidChain, TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState |
| B8.05 | legacy acceptance refuses safety-bearing proposal | I2,I8 | KILLED | TestFAAConcurrentAppendersAndReadersKeepAValidChain, TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState |
| B14.01 | worker fence refuses gate objective (M4a) | I7 | KILLED | TestWorkerResolutionRefusesAuthorityGateObjective |
| B14.02 | explicit gate objective routed to coordination (M4b) | I7 | KILLED | TestGateObjectiveFromEveryRouteReachesCoordinationAndNeverAProvider, TestGitBoundValidationCleansUpAfterTimeout |
| B14.03 | safety plan refuses objective outside plan (M4c) | I7,I2 | KILLED | TestKernelRepair3ExplicitObjectiveMustBeAnOpenCandidateOfTheAcceptedPlan |
| B14.04 | completed objective refused | I1 | KILLED | TestKernelRepair3ExplicitObjectiveMustBeAnOpenCandidateOfTheAcceptedPlan |
| B14.05 | selected gate goes to coordination | I7 | KILLED | TestGateObjectiveFromEveryRouteReachesCoordinationAndNeverAProvider, TestGoalGateCompletesThroughTheSupportedOwnerCeremony |
| B10.01 | post-worker validator missing blocks (M5) | I4 | KILLED | TestMissingValidatorAfterWorkerBlocksSafetyCheckpoint |
| B10.02 | bound validator required for safety checkpoint | I4 | KILLED | TestMissingValidatorAfterWorkerBlocksSafetyCheckpoint |
| B10.03 | no-declared-validation never counts for safety (M9) | I4 | KILLED | TestNoDeclaredValidationCannotSettleASafetyCompletion |
| B10.04 | preflight profile-digest mismatch (M11) | I5 | KILLED | TestKernelRepair3ProfileMismatchIsRefusedBeforeTheWorkerRuns |
| B10.05 | candidate conformance decided before publication (M7b) | I4,I5 | KILLED | TestFailedCandidateConformanceIsDecidedBeforePublication |
| B10.06 | conformance acknowledgement required (M7a) | I4,I5 | KILLED | TestSafetyCompletionRejectsMalformedConformanceEvidence |
| B10.07 | integrated-pass evidence required to qualify | I4 | KILLED | TestKernelRepair3QualificationRequiresIntegratedPassEvidence |
| B10.08 | settlement re-qualifies when not qualified before | I4 | KILLED | TestSafetyCompletionRejectsMalformedConformanceEvidence |
| B10.09 | governed output size bound | I5 | KILLED | TestKernelRepair3GovernedOutputMustBePresentAndBounded |
| N4.01 | input empty/oversize refused | I5 | KILLED | TestUnmarshalExactJSONRefusesAmbiguousSafetyEvidence, TestUnmarshalExactJSONRefusesMalformedAndUnboundedInput |
| N4.02 | invalid UTF-8 refused | I5 | KILLED | TestUnmarshalExactJSONRefusesMalformedAndUnboundedInput |
| N4.03 | unpaired surrogate escape refused | I5 | KILLED | TestUnmarshalExactJSONRefusesMalformedAndUnboundedInput |
| N4.04 | trailing content refused (walk) | I5 | REDUNDANT-LAYER |  |
| N4.05 | trailing content refused (decode) | I5 | REDUNDANT-LAYER |  |
| N4.06 | trailing content refused (both layers) | I5 | KILLED | TestDossierResolutionRefusesMalformedOrAmbiguousBytes, TestUnmarshalExactJSONRefusesAmbiguousSafetyEvidence |
| N4.07 | nesting depth bound | I5 | KILLED | TestUnmarshalExactJSONNestingBoundRefusesOtherwiseValidDocuments |
| N4.08 | textual duplicate key refused | I5 | KILLED | TestUnmarshalExactJSONRefusesAmbiguousSafetyEvidence, TestUnmarshalExactJSONRefusesSemanticDuplicateKeys |
| N4.10 | case-folded key refused (N4) | I5 | KILLED | TestUnmarshalExactJSONDifferentialAgainstEncodingJSON, TestUnmarshalExactJSONRefusesSemanticDuplicateKeys |
| N4.11 | unknown fields refused when requested | I5 | KILLED | TestUnmarshalExactJSONRefusesAmbiguousSafetyEvidence, TestUnmarshalExactJSONRefusesMalformedAndUnboundedInput |
| N4.12 | external import decoder is exact | I5 | KILLED | TestKernelRepair3N4ExternalImportRefusesSemanticDuplicateKeys |
| N5.01 | validator runs in the private export, not the worker checkout | I5 | KILLED | TestGitBoundValidationBindsToCommitContentNotWorkingTreeState, TestGitBoundValidationContentRehashCatchesAStatBlindSameSizeRewrite |
| N5.02 | export re-verified after the run | I5 | KILLED | TestGitBoundValidationContentRehashCatchesAStatBlindSameSizeRewrite, TestGitBoundValidationDetectsExportTamperingAndAlwaysCleansUp |
| N5.02b | export verified right after checkout (creation time) | I5 | REDUNDANT-LAYER |  |
| N5.03 | checkpoint re-bound after the run | I5,I10 | KILLED | TestGitBoundValidationRefusesAValidatorThatMovesTheWorkerCheckoutDuringTheRun |
| N5.04 | export HEAD equals checkpoint | I5 | KILLED | TestGitBoundValidationRefusesAnExportWhoseHeadMoved, TestValidationCancellationTerminatesDescendants |
| N5.05 | export tree equals qualified tree | I5 | REDUNDANT-BY-CONSTRUCTION |  |
| N5.06 | export index hint flags refused | I5 | KILLED | TestGitBoundValidationRefusesAnIndexHintEvenWithoutAContentChange, TestValidationCancellationTerminatesDescendants |
| N5.07 | export status clean | I5 | REDUNDANT-LAYER |  |
| N5.08 | export gained no untracked/excluded files | I5 | REDUNDANT-LAYER |  |
| N5.09 | export content re-hashed into a fresh index (stat/index-blind layer) | I5 | KILLED | TestGitBoundValidationContentRehashCatchesAStatBlindSameSizeRewrite |
| N5.10 | joint: every export post-run layer (status+hint+untracked+content) | I5 | KILLED | TestGitBoundValidationContentRehashCatchesAStatBlindSameSizeRewrite, TestGitBoundValidationDetectsExportTamperingAndAlwaysCleansUp |
| N5.11 | committed validator digest at checkpoint (bind) | I5 | REDUNDANT-LAYER |  |
| N5.12 | exported validator digest | I5 | REDUNDANT-LAYER |  |
| N5.13 | joint: committed and exported validator digest | I5 | KILLED | TestGitBoundValidationRefusesValidatorThatDiffersFromTheProfile, TestValidationDriftAfterAdmissionFailsClosedBeforePublication |
| N5.14 | validator executable mode at checkpoint | I5 | REDUNDANT-BY-CONSTRUCTION |  |
| N5.14x | exported validator executable mode | I5 | REDUNDANT-BY-CONSTRUCTION |  |
| N5.14j | joint: validator executable mode (bind + export) | I5 | REDUNDANT-BY-CONSTRUCTION |  |
| N5.15 | bind: HEAD is the checkpoint | I5,I10 | REDUNDANT-LAYER |  |
| N5.16 | bind: local branch points at checkpoint | I5,I10 | REDUNDANT-LAYER |  |
| N5.17 | publish: HEAD is the checkpoint | I10 | KILLED | TestGitBoundValidationRefusesCheckpointAndTreeSubstitution, TestGitBoundValidationRefusesSubstitutionOnADetachedHead |
| N5.18 | joint: every HEAD-equality layer | I5,I10 | KILLED | TestGitBoundValidationRefusesCheckpointAndTreeSubstitution, TestGitBoundValidationRefusesSubstitutionOnADetachedHead |
| N5.19 | qualified tree recorded at run time is re-compared | I5,I10 | KILLED | TestGitBoundValidationRefusesCheckpointAndTreeSubstitution, TestValidationCancellationTerminatesDescendants |
| N5.20 | checkpoint must be a full object id | I5 | KILLED | TestGitBoundValidationRequiresAFullObjectId, TestValidationCancellationTerminatesDescendants |
| N5.21 | publication pushes the exact qualified object id | I10 | EQUIVALENT |  |
| N5.22 | publication verifies the remote ref is the checkpoint | I10 | KILLED | TestGitBoundValidationPublicationVerifiesTheRemoteRef |
| N5.23 | controller git runs without worker fsmonitor/hooks | I5 | KILLED | TestGitBoundValidationNeverExecutesWorkerCheckoutHooksOrFsmonitor |
| N5.24 | controller git ignores replace objects | I5 | KILLED | TestGitBoundValidationCleansUpAfterTimeout, TestGitBoundValidationIgnoresReplaceRefsInTheWorkerCheckout |
| N6.01 | output bound enforced in the production collector | I4,I5 | KILLED | TestBoundValidationOutputOverflowFailsClosed, TestValidationCancellationTerminatesDescendants |
| N6.02 | overflow fails closed even with exit 0 | I4,I5 | KILLED | TestBoundValidationOutputOverflowFailsClosed, TestValidationCancellationTerminatesDescendants |
| N6.03 | overflow cancels (kills) the process group | I4 | KILLED | TestValidationOutputBoundFailsClosedThroughTheRealCollector |
| N6.04 | process tree terminated after validator returns | I4 | KILLED | TestValidationTerminatesTheWholeProcessTree |
| N6.05 | collector keeps draining after overflow (no deadlock) | I4 | EQUIVALENT |  |
| N3.01 | image digest equals manifest digest | I6 | KILLED | TestRunningImageManifestDigestMustBeTheImageDigest |
| N3.02 | loaded-code binding (kernel cdhash vs file) | I6 | KILLED | TestRunningImageExecToInitWindow, TestRunningImageInPlaceOverwriteIsRefused |
| N3.03 | manifest path still names the executing file | I6 | KILLED | TestRunningImagePathnameMutationMatrix, TestRunningImageVerificationSurvivesPathnameReplacement |
| N3.04 | kernel cdhash must match a CodeDirectory of the file | I6 | KILLED | TestDarwinKernelIdentityMatchesTheRunningFile, TestRunningImageExecToInitWindow |
| N3.05 | page hashes recomputed from file bytes | I6 | KILLED | TestVerifyExecutedImageFailsClosedOnUnsupportedOrMalformedImages, TestVerifyExecutedImageRefusesEveryCodeChange |
| N3.06 | code limit ends at the embedded signature | I6 | KILLED | TestVerifyExecutedImageRefusesUnhashedBytesBeforeTheSignature |
| N3.07 | kernel reports the signature valid | I6 | UNMODELED |  |
| N3.08 | image size change refused | I6 | KILLED | TestRunningImageLengthChangeIsRefused |
| R4.01 | in-tx revoked-or-not-live: negative record refuses | I3,I12 | KILLED | TestIsSecureBlobRevokedInTx |
| R4.02 | in-tx revoked-or-not-live: absent liveness refuses | I12 | KILLED | TestUnlessRevokedWriteRequiresGenerationLiveness |
| R4.03 | in-tx liveness exemption only for facts created by this transaction | I12 | KILLED | TestDecisionLivenessRowDeletionFailsClosed, TestDeleteLivenessRecordRefusesEveryOtherNamespace |
| R4.04 | retire API refuses every non-liveness namespace | I12 | KILLED | TestDeleteLivenessRecordRefusesEveryOtherNamespace |
| R4.05 | liveness sealing refuses every non-liveness namespace | I12 | KILLED | TestLivenessSealingAndProbeRefuseOtherNamespaces |
| R4.05b | in-tx liveness probe refuses non-liveness namespaces | I12 | KILLED | TestLivenessSealingAndProbeRefuseOtherNamespaces |
| R4.06 | atomic multi-write accepts liveness records only | I12 | KILLED | TestPutSecureBlobsAtomicallyAcceptsOnlyLivenessCompanions |
| R4.07 | succession retires the predecessor liveness in its own transaction | I12 | KILLED | TestRootSuccessionRollbackByRowDeletionDoesNotReviveThePredecessor |
| R4.08 | succession makes the successor live in its own transaction | I12 | KILLED | TestHistoricalRootModernizationEstablishesSingleCurrentRoot, TestInstallationRepairRequestsAreIndependentDurableDecisions |
| R4.09 | generation liveness commits with the generation | I12 | KILLED | TestAbandonmentConfirmationAppendsExactFrozenPayload, TestAbandonmentConfirmationRejectsChangedStateAndSubstitutedBytes |
| R4.10 | decision liveness commits with the decision | I12 | KILLED | TestDecisionLivenessRowDeletionFailsClosed, TestDerivedPackageApprovalIsConsumableByDeployment |
| R4.11 | LoadAuthorityDecision requires decision liveness | I3,I12 | KILLED | TestDecisionLivenessRowDeletionFailsClosed, TestFAACrashBetweenFactAndEffectsLeavesTheDecisionRetired |
| R4.12 | identical decision replay cannot resurrect a retired decision | I12 | KILLED | TestRevocationRowDeletionDoesNotRestoreAUnprotectedDecision |
| R4.13 | revocation retires the decision liveness record | I3,I12 | KILLED | TestFAAReplayOfACopiedLivenessRowDoesNotRestoreARevokedDecision, TestRevocationRowDeletionDoesNotRestoreAUnprotectedDecision |
| R4.14 | generation invalidation retires the generation liveness record | I3,I12 | KILLED | TestGenerationInvalidationRowDeletionDoesNotRestoreTheGeneration |
| R4.15 | ValidateAuthorityGeneration requires generation liveness | I3,I12 | KILLED | TestFAAEveryConsumerHonoursFactsAndFreshness, TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority |
| R4.16 | lineage walk requires liveness of every generation | I3,I12 | KILLED | TestLineageWalkRefusesAChildWithoutItsLivenessRecord |
| R4.17 | current installation root must be positively live (same snapshot as the generations) | I3,I12 | KILLED | TestRootLivenessRowDeletionLeavesNoCurrentRoot, TestRootSuccessionRollbackByRowDeletionDoesNotReviveThePredecessor |
| R4.18 | publication consumer: generation liveness | I10,I12 | KILLED | TestPublicationFencesRequireLivenessRecords |
| R4.19 | publication consumer: decision liveness | I10,I12 | KILLED | TestPublicationFencesRequireLivenessRecords |
| R4.20 | recovery admission: decision liveness | I10,I12 | KILLED | TestRecoveryAdmissionRecheckRequiresLivenessRecords |
| R4.21 | recovery admission: generation liveness | I10,I12 | KILLED | TestRecoveryAdmissionRecheckRequiresLivenessRecords |
| R4.22 | ordinary re-request requires the predecessor decision liveness | I3,I12 | KILLED | TestSaveAuthorityReRequestUsesImmutableHistoricalEvidenceAndConverges |
| R4.23 | owner-decision authority requires issuing generation liveness | I3,I8,I12 | REDUNDANT-LAYER |  |
| R4.23j | joint: owner-decision generation liveness (plan authority + ValidateAuthorityGeneration) | I3,I8,I12 | KILLED | TestFAAEveryConsumerHonoursFactsAndFreshness, TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority |
| R4.24 | classification is derived when the classification row is missing | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.24e | classification derivation end-to-end (N9 counterexample) | I9,I12 | KILLED | TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed |
| R4.25 | derivation reads surviving generations of the Goal | I9,I12 | KILLED | TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed |
| R4.26 | derivation reads the safety binding of proposals | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals, TestConcurrentInstallationFanoutAcrossProcesses |
| R4.27 | derivation refuses conflicting kernel evidence | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.28 | derivation fails closed on an unreadable candidate | I9,I12 | REDUNDANT-LAYER |  |
| R4.28j | joint: candidate decrypt failure + extraction failure both swallowed | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.29 | derivation fails closed on an unreadable generation | I9,I12 | REDUNDANT-LAYER |  |
| R4.29j | joint: candidate decrypt failure + generation load failure both swallowed | I9,I12 | KILLED | TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed |
| R4.30 | derivation attributes records by Goal identity | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.31 | derivation attributes generations by Goal identity | I9,I12 | KILLED | TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed |
| R4.32 | decision liveness written atomically for delegated decision+generation | I12 | KILLED | TestAbandonmentConfirmationAppendsExactFrozenPayload, TestAbandonmentConfirmationRejectsChangedStateAndSubstitutedBytes |
| R4.33 | protected decision written with its liveness in one transaction | I12 | KILLED | TestActivationRefusalSurvivesRestartAndRecoversOnlyWithRestoredActivation, TestContinuationAcceptsAndAttachesUnderValidActivation |
| R5.01 | chain: fact belongs to this installation | I13 | KILLED | TestVerifyChainEachRuleRefusesOnItsOwn |
| R5.02 | chain: fact chains to its predecessor | I13 | KILLED | TestVerifyChainEachRuleRefusesOnItsOwn |
| R5.03 | chain: fact head matches its content | I13 | KILLED | TestVerifyChainRefusesEveryStructuralFailure |
| R5.04 | chain: gap refused outside a re-anchor | I13,I14 | KILLED | TestVerifyChainEachRuleRefusesOnItsOwn |
| R5.05 | chain: must begin with a genesis or a re-anchor | I13 | KILLED | TestVerifyChainEachRuleRefusesOnItsOwn |
| R5.06 | chain: genesis only first, at 0, with a nonce | I13 | KILLED | TestVerifyChainRefusesEveryStructuralFailure |
| R5.07 | chain: re-anchor bridges exactly from the verified chain | I13 | KILLED | TestVerifyChainRefusesEveryStructuralFailure |
| R5.08 | chain: decision fact complete | I13 | KILLED | TestVerifyChainRefusesEveryStructuralFailure |
| R5.09 | chain: generation fact complete | I13 | KILLED | TestVerifyChainRefusesEveryStructuralFailure |
| R5.10 | chain: classification fact complete | I14 | KILLED | TestVerifyChainRefusesEveryStructuralFailure |
| R5.11 | compare: store behind the anchor | I13,I14 | KILLED | TestCompareClassifiesTheStoreAgainstTheAnchor, TestFAAClassificationSurvivesErasureOfEveryEvidenceRow |
| R5.12 | compare: store ahead of the anchor | I13 | KILLED | TestCompareClassifiesTheStoreAgainstTheAnchor, TestFAAAnchorStatesFailClosed |
| R5.13 | compare: same length, different head | I13 | KILLED | TestCompareClassifiesTheStoreAgainstTheAnchor, TestFAAAnchorStatesFailClosed |
| R5.14 | genesis requires a nonce and installation | I13 | KILLED | TestGenesisIsBoundToItsNonceAndInstallation |
| R5.15 | snapshot: anchor names this installation | I13 | KILLED | TestFAARepositoryRefusesAnAnchorThatNamesAnotherInstallation |
| R5.16 | snapshot: store related to anchor (observation) | I13,I14 | KILLED | TestFAAAnchorStatesFailClosed, TestFAAClassificationSurvivesErasureOfEveryEvidenceRow |
| R5.17 | append: store related to anchor before extending | I13 | KILLED | TestFAACrashAfterAnchorAdvanceBeforeCommitStrandsFailClosed |
| R5.18 | lag is re-observed behind the writer lock only for a plain lag | I13 | KILLED | TestFAAReaderBarrierWaitsOutAnInFlightWriteAndRefusesRealDisagreement, TestFAARepositoryRefusesAnAnchorThatNamesAnotherInstallation |
| R5.75 | a real disagreement behind the lock is refused at once | I13 | KILLED | TestFAAReaderBarrierWaitsOutAnInFlightWriteAndRefusesRealDisagreement |
| R5.76 | a lag that resolves behind the writer lock is accepted | I13 | KILLED | TestFAAReaderWaitsForAWriterBetweenAnchorAdvanceAndCommit |
| R5.19 | admission is stamped with the anchor sequence | I13 | KILLED | TestFAAAuthorityAdmittedAfterAReanchorIsCurrent |
| R5.20 | append is idempotent for a recorded transition | I13 | KILLED | TestFAACrashBetweenFactAndEffectsLeavesTheDecisionRetired |
| R5.21 | append advances the anchor | I13 | KILLED | TestFAAAnchorStatesFailClosed, TestFAAAnchorWriteThatDoesNotStickIsRefused |
| R5.22 | append reverts the anchor when the commit fails | I13 | KILLED | TestFAACommitFailureRevertsTheAnchor |
| R5.23 | an anchor is not created next to existing state | I13,I14 | KILLED | TestFAAInitialisationRefusesExistingStateAndNonZeroAnchors |
| R5.24 | a crashed initialisation is replaced only at sequence 0 | I13 | KILLED | TestFAAInitialisationRefusesExistingStateAndNonZeroAnchors |
| R5.25 | re-anchor voids earlier admissions | I13 | KILLED | TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority, TestFAARootAdmittedBeforeTheReanchorIsNotCurrent |
| R5.26 | the in-transaction snapshot relates store to anchor | I13 | KILLED | TestFAAAnchorStatesFailClosed, TestFAAClassificationSurvivesErasureOfEveryEvidenceRow |
| R5.27 | decision load consults freshness first | I3,I13 | KILLED | TestFAAAnchorStatesFailClosed, TestFAAFactChainTruncationErasureAndDamageFailClosed |
| R5.28 | liveness: retired by an anchored fact | I3,I13 | KILLED | TestFAACrashBetweenFactAndEffectsLeavesTheDecisionRetired, TestFAAEveryConsumerHonoursFactsAndFreshness |
| R5.29 | liveness: admitted before the latest re-anchor is void | I13 | KILLED | TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority, TestRepair5WholeDatabaseRollbackIsRefusedAndRecoveredOnlyByTheGovernedCeremony |
| R5.30 | liveness: freshness consulted (requireLive) | I13 | KILLED | TestFAAEveryConsumerHonoursFactsAndFreshness, TestFAAWholeDatabaseRollbackFailsClosed |
| R5.31 | liveness by identity: freshness consulted | I13 | KILLED | TestFAAEveryConsumerHonoursFactsAndFreshness |
| R5.32 | revocation is anchored before its effects | I3,I13 | KILLED | TestFAAAnchorStatesFailClosed, TestFAAFactChainTruncationErasureAndDamageFailClosed |
| R5.33 | invalidation is anchored before its effects | I3,I13 | KILLED | TestFAAMixedSnapshotAdversaryNeverRegainsRetiredAuthority |
| R5.34 | succession anchors the supersession | I13 | KILLED | TestFAARootSuccessionIsAnchoredAndItsRollbackByReplayIsRefused |
| R5.35 | root resolution: snapshot refusal propagates | I13 | KILLED | TestFAAEveryConsumerHonoursFactsAndFreshness, TestFAAReanchorCoversMissingUnreadableAndAheadAnchors |
| R5.36 | root resolution: retired root excluded | I13 | KILLED | TestFAARootRetiredByFactAloneIsNotCurrent, TestFAARootSuccessionIsAnchoredAndItsRollbackByReplayIsRefused |
| R5.37 | root resolution: pre-recovery root excluded | I13 | KILLED | TestFAARootAdmittedBeforeTheReanchorIsNotCurrent |
| R5.38 | classification: anchored fact is authoritative | I9,I14 | KILLED | TestFAAClassificationSurvivesErasureOfEveryEvidenceRow, TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority |
| R5.39 | classification: freshness refusal propagates | I9,I13,I14 | KILLED | TestFAAClassificationSurvivesErasureOfEveryEvidenceRow, TestFAAFactChainTruncationErasureAndDamageFailClosed |
| R5.40 | classification is anchored when first marked | I14 | KILLED | TestFAAClassificationIsAnchoredWhenMarkedAndWhenOnlyDerived |
| R5.41 | classification is anchored when derived-classified | I14 | KILLED | TestFAAClassificationIsAnchoredWhenMarkedAndWhenOnlyDerived |
| R5.42 | evidence registry: proposal evidences governance | I9,I14 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals, TestGoalEvidenceRegistryCoversEveryGovernanceNamespace |
| R5.43 | evidence registry: review evidences governance | I9,I14 | KILLED | TestGoalEvidenceRegistryCoversEveryGovernanceNamespace |
| R5.44 | evidence registry: acceptance evidences governance | I9,I14 | KILLED | TestGoalEvidenceRegistryCoversEveryGovernanceNamespace |
| R5.45 | evidence registry: request evidences governance | I9,I14 | KILLED | TestGoalEvidenceRegistryCoversEveryGovernanceNamespace |
| R5.46 | evidence registry: decision evidences governance | I9,I14 | KILLED | TestGoalEvidenceRegistryCoversEveryGovernanceNamespace |
| R5.47 | evidence registry: baseline generation evidences governance | I9,I14 | KILLED | TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed |
| R5.48 | evidence registry: completion seal evidences governance | I9,I14 | KILLED | TestRepair5EvidencePowersetClassificationQualification |
| R5.49 | protected request without its proposal fails closed | I14 | KILLED | TestProtectedRequestWithoutItsProposalFailsClosed |
| R5.50 | recovery admission consults the anchored chain | I10,I13 | KILLED | TestFAARecoveryAdmissionRecheckConsultsTheAnchoredChain |
| R5.51 | in-tx guard checks the decision | I13 | KILLED | TestFAAInTransactionGuardNeedsTheDecisionAndTheGenerationEachOnItsOwn |
| R5.52 | in-tx guard checks the generation | I13 | KILLED | TestFAAInTransactionGuardNeedsTheDecisionAndTheGenerationEachOnItsOwn |
| R5.53 | repair guard consults the anchor | I13 | KILLED | TestRuntimeStateTransactionalGuardConsultsTheForwardAuthorityAnchor |
| R5.54 | re-anchor is bound to the confirmed plan | I13 | KILLED | TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority |
| R5.55 | re-anchor sequence exceeds the anchor's | I13 | KILLED | TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority, TestFAAReanchorCoversMissingUnreadableAndAheadAnchors |
| R5.56 | re-anchor re-admits the attested root | I13 | KILLED | TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority, TestFAAReanchorCoversMissingUnreadableAndAheadAnchors |
| R5.57 | re-anchor completes an interrupted re-admission | I13 | KILLED | TestFAAReanchorRootSelectionAndIdempotence |
| R5.58 | re-anchor preserves orphaned facts | I14 | KILLED | TestFAAReanchorPreservesOrphanedFactsAndBridgesFromTheLastValidLink |
| R5.59 | re-anchor needs exactly one un-retired root | I13 | KILLED | TestFAAReanchorRootSelectionAndIdempotence |
| R5.60 | re-anchor excludes a root retired by fact | I13 | KILLED | TestFAAReanchorRootSelectionAndIdempotence |
| R5.61 | re-anchor only completes; a consistent store writes nothing | I13 | KILLED | TestFAAReanchorInterruptedBeforeRootReadmissionIsCompletedByTheSameCeremony, TestFAAReanchorRootSelectionAndIdempotence |
| R5.62 | fact append reverts the anchor on commit failure | I13 | KILLED | TestAppendGovernanceFactUndoesOnCommitFailureAndAcceptsOnlyFacts |
| R5.63 | only facts may be appended through the chain | I13 | KILLED | TestAppendGovernanceFactUndoesOnCommitFailureAndAcceptsOnlyFacts |
| R5.64 | the fact append takes the writer lock | I13 | REDUNDANT-BY-CONSTRUCTION |  |
| R5.65 | production constructor attaches the anchor | I13 | KILLED | TestRepair5MissingAnchorFailsClosedAndTheCeremonyRecoversIt, TestRepair5N14ReplayedLivenessDoesNotMintACompletionAfterRestart |
| R5.66 | re-anchor requires an interactive terminal | I8,I13 | KILLED | TestRepair5WholeDatabaseRollbackIsRefusedAndRecoveredOnlyByTheGovernedCeremony |
| R5.67 | re-anchor requires the enrolling OS user | I8,I13 | KILLED | TestRepair5MissingAnchorFailsClosedAndTheCeremonyRecoversIt, TestRepair5WholeDatabaseRollbackIsRefusedAndRecoveredOnlyByTheGovernedCeremony |
| R5.68 | re-anchor requires the digest-bound confirmation | I8,I13 | KILLED | TestRepair5WholeDatabaseRollbackIsRefusedAndRecoveredOnlyByTheGovernedCeremony |
| R5.69 | status reports a store that is not consumable | I13 | KILLED | TestRepair5WholeDatabaseRollbackIsRefusedAndRecoveredOnlyByTheGovernedCeremony |
| R5.70 | keychain: not strictly forward | I13 | KILLED | TestKeychainAnchorIsForwardOnlyCompareAndSetAndFailsClosed |
| R5.71 | keychain: expectation compared | I13 | KILLED | TestKeychainAnchorIsForwardOnlyCompareAndSetAndFailsClosed |
| R5.72 | keychain: create refuses an existing item | I13 | KILLED | TestKeychainAnchorIsForwardOnlyCompareAndSetAndFailsClosed |
| R5.73 | keychain: read-back verified | I13 | UNMODELED |  |
| R5.74 | keychain: another installation's state never returned | I13 | KILLED | TestKeychainAnchorNeverReturnsAnotherInstallationsState |
