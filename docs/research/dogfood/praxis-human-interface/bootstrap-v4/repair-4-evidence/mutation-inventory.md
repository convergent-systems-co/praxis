# Consolidated guard / mutation inventory (repair 4)

Generated from `mutation-inventory.json` (harness: `mutation/harness.py`; `harness_b.py` is a copy that differs only in scratch-copy directory names so two passes could run at once; catalogue: `mutation/catalogue.py`). Every mutation is applied to a SCRATCH COPY of the repository (never the real tree); each file is restored and re-hashed (`RESTORE-CHECK OK` in every run log). A guard is KILLED only if at least one relevant regression fails; kills attributable only to load-sensitive unrelated tests do not count.

## Universe and scope of the claim

The universe is the set of safety guards in the PRE-V4 kernel non-test source of `pkg/contracts`, `internal/bootstrapv4`, `internal/state` (secure-blob boundary), `internal/goalstore`, `internal/goaldrive` and the lifecycle/decide/settlement/inspect surfaces of `cmd/praxis`, **as enumerated in `mutation/catalogue.py`**. Repair 3's 155 guards are carried forward with unchanged definitions and re-run against the Repair 4 source; Repair 4 adds 37 guards for I12 (deletion-monotonicity): the in-transaction revoked-or-not-live predicate, the liveness retire/seal/atomic-write APIs, decision and generation liveness at every consumer (load, replay, revocation, invalidation, lineage walk, current root, re-request, publication fence, recovery admission, owner-decision authority), root succession's atomic retire/admit, and the classification-derivation layers (fallback, generations, proposals, attribution, conflict, fail-closed reads).

**Change of universe.** Review #4 correctly noted that the Repair 3 universe *excluded* "deletion of authenticated rows by a raw writer", which is exactly the attack behind N8/N9, so its 134 kills could not bear on them. That exclusion is removed: keyless row deletion (adversary A1 in `design/i12-deletion-monotonicity.md`) is now inside the universe, and the Repair 4 guards are mutated against regressions that perform keyless SQL deletions. Still excluded: raw OS signals; forging by a holder of the installation storage key (accepted bootstrap trust root); **rollback by replaying previously copied sealed rows and total erasure of a Goal's history (residuals A2/A3, reproduced by `probes/residual_rollback_replay_test.go.txt`, not defended)**; the non-Unix validation fallback; Windows and Linux paths (only darwin/arm64 was run); states this platform will not produce on demand.

This inventory is not a claim of universal completeness. It is complete over the enumerated guard universe only.

## Counts

| Measure | Count |
|---|---|
| mutations applied (total) | 192 |
| single-guard mutations (= enumerated guards) | 182 |
| joint mutations (redundant layers removed together) | 10 |
| killed by a relevant regression | 169 |
| survived: redundant layer (another layer enforces it; a joint mutation is killed) | 13 |
| survived: redundant by construction | 7 |
| survived: equivalent mutant | 2 |
| survived: unmodeled on this platform | 1 |
| survived with no explanation | 0 |
| **surviving total (all classified)** | 23 |

Counted results: `mutation/results-final4.json`, one fresh pass of the whole catalogue against the frozen Repair 4 source (log `mutation/run-final4.log`). Earlier passes are preserved and NOT counted: the first Repair 4 pass (`results-r4-full.json`, `results-r4-only.json`; four of its mutation definitions failed to build or targeted the wrong occurrence) and the targeted re-runs (`results-r4-final.json`, `results-r4-rerun*.json`), which were run before a later source change (root liveness is now read in the same snapshot as the generations) and before the last tests were added.

Repair 4 mutations that first survived led to new regressions rather than to reclassification: R4.16 (lineage walk), R4.18-R4.21 (publication fence, recovery admission), R4.02/R4.05/R4.05b/R4.06 (in-transaction predicate and liveness APIs), R4.29/R4.31 (derivation from generations); and a test that had exercised the list-validation layer instead of the intended decrypt layer was corrected (R4.28j). R4.23's single-guard kill was attributable only to a load-sensitive test and is classified redundant-layer; joint R4.23j is killed by the generation-liveness regressions.

## Survivors (all)

| Id | Guard | Class | Why |
|---|---|---|---|
| N1.09 | Attach classifies its Goal (mark at attach) | REDUNDANT-LAYER | attach re-marks the Goal that the safety proposal already classified; joint mutation N1.09j (proposal+attach) is killed by TestKernelRepair3N1AProposalClassifiesItsGoalBeforeAnythingIsAccepted / TestKernelRepair3N1Stripping... |
| N2.10 | Gate decision owner/root authority current | REDUNDANT-BY-CONSTRUCTION | the gate-completion currency re-check of owner/root authority is masked by the plan-level governing-authority check that runs first in the same call (prepareAuthorityGate, B9.12/B9.09 killed) |
| M12.01 | claim == selected unit before publication | REDUNDANT-LAYER | the pre-publication claim==selected check cannot be reached with a mismatching claim because the checkpoint inspection (M12.03, killed by its own message assertion) and settlement (M12.02, killed by a direct settleCompletion test) both refuse first; joint M12.04 is killed |
| B9.11 | decision by installation owner | REDUNDANT-BY-CONSTRUCTION | the owner principal and the governance scope both derive from the same installation digest; scope is checked first (B9.10 killed), so a scope-correct decision cannot be by another owner short of a forged record |
| B9.13 | gate decision issued by current root | REDUNDANT-BY-CONSTRUCTION | gate decisions must be issued by the CURRENT root; a decision by a superseded or revoked root already fails ValidateAuthorityGeneration (B9.12 killed) and plan-level authority (B9.09/B9.12). Reaching the current-root comparison alone needs a root succession fixture that keeps the old generation valid |
| N4.04 | trailing content refused (walk) | REDUNDANT-LAYER | trailing-content refusal exists in the type-directed walk and again in the decode; each alone is masked by the other; joint N4.06 is killed |
| N4.05 | trailing content refused (decode) | REDUNDANT-LAYER | see N4.04 (joint N4.06 killed) |
| N5.02b | export verified right after checkout (creation time) | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) creation-time export verification only fails fast; the same predicate set is re-proved after the run (N5.02 killed) and the content is re-hashed there |
| N5.05 | export tree equals qualified tree | REDUNDANT-BY-CONSTRUCTION | an export's HEAD^{tree} is a function of its HEAD commit, which N5.04 compares; the tree comparison cannot differ while the commit is equal |
| N5.07 | export status clean | REDUNDANT-LAYER | export status check is covered by the content re-hash and the hint scan; joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 |
| N5.08 | export gained no untracked/excluded files | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) untracked/excluded-file scan is covered by the content re-hash (which adds all files); joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 |
| N5.11 | committed validator digest at checkpoint (bind) | REDUNDANT-LAYER | the committed-validator digest at the checkpoint is re-checked on the exported blob (N5.12) and the whole export tree is re-hashed; the two digest layers removed together are killed (N5.13) |
| N5.12 | exported validator digest | REDUNDANT-LAYER | the exported validator digest is re-checked on the committed blob (N5.11 killed) and the whole export tree is re-hashed after the run; joint N5.13 is killed |
| N5.14 | validator executable mode at checkpoint | REDUNDANT-BY-CONSTRUCTION | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) a validator that is not an executable regular file cannot be executed: the OS refuses the exec and the run fails closed. The explicit mode checks (bind N5.14, export N5.14x) only give an earlier, clearer message; even their joint mutation N5.14j survives because exec permission still refuses |
| N5.14x | exported validator executable mode | REDUNDANT-BY-CONSTRUCTION | see N5.14 |
| N5.14j | joint: validator executable mode (bind + export) | REDUNDANT-BY-CONSTRUCTION | see N5.14 |
| N5.15 | bind: HEAD is the checkpoint | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) HEAD==checkpoint is asserted by four layers (bind, bind branch ref, ReadCheckpointArtifact, PushAndVerify); joint N5.18 of the bind/branch/push layers is killed |
| N5.16 | bind: local branch points at checkpoint | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) see N5.15 (joint N5.18 killed) |
| N5.21 | publication pushes the exact qualified object id | EQUIVALENT | PushAndVerify has already proved HEAD==head, so pushing HEAD or the object id publishes the same commit |
| N6.05 | collector keeps draining after overflow (no deadlock) | EQUIVALENT | after overflow the buffer-limit branch sets the same flags and cancels again and still returns len(p): the early branch only saves work |
| N3.07 | kernel reports the signature valid | UNMODELED | the kernel never reported CS_VALID cleared in any scenario constructible here (flags 0x22020201 throughout); the check fails closed on a state this platform will not produce on demand |
| R4.23 | owner-decision authority requires issuing generation liveness | REDUNDANT-LAYER | owner-decision issuing-generation liveness is checked in ValidateAuthorityGeneration (killed as R4.15) and again in the plan-authority path; the plan-authority check alone is masked; joint R4.23j is killed by the generation invalidation/liveness regressions |
| R4.28 | derivation fails closed on an unreadable proposal | REDUNDANT-LAYER | an undecryptable proposal row is refused at the load AND again when its (empty) payload fails to decode; each alone is masked by the other; joint R4.28j is killed by TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |

## All mutations

| Id | Guard | Invariants | Verdict | Killing tests (first 2) |
|---|---|---|---|---|
| N1.01 | Save refuses safety-bearing plan (B7) | I2,I9 | KILLED | TestKernelRepair2B7BaselinePersistenceSurfacesRefuseSafetyBearingPlans |
| N1.02 | Save refuses any plan for a classified Goal | I9 | KILLED | TestKernelRepair3N1StrippingTheSafetyBindingCannotDowngradeAClassifiedGoal, TestRepair4N9ClassificationRowDeletionStillRefusesStrippedImport |
| N1.03 | Proposal refuses legacy proposal for classified Goal | I9 | KILLED | TestKernelRepair3N1StrippingTheSafetyBindingCannotDowngradeAClassifiedGoal, TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.04 | Review refuses legacy review for classified Goal | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.05 | Legacy acceptance refuses classified Goal | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.06 | Authority-backed acceptance bridge refuses classified Goal | I9 | KILLED | TestSafetyClassificationRefusesTheAuthorityBackedAcceptanceBridgeOnItsOwn |
| N1.07 | Attach refuses legacy plan for classified Goal | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.08 | Safety proposal classifies its Goal (mark at proposal) | I9 | KILLED | TestRepair4N9ClassificationRowDeletionStillRefusesStrippedImport |
| N1.09 | Attach classifies its Goal (mark at attach) | I9 | REDUNDANT-LAYER |  |
| N1.10 | Classification kernel-version mismatch refused (consistency) | I9 | KILLED | TestSafetyConsistencyRefusesAnotherKernelVersion |
| N1.11 | Classification kernel-version mismatch refused (mark) | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.12 | Missing binding on classified Goal refused | I9 | KILLED | TestEmittedNextActionsAreExecutableProductContracts, TestRevocationRowDeletionDoesNotRestoreAUnprotectedDecision |
| N1.13 | Attach binds accepted plan to its Goal (R1-G) | I9,I2 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestKernelRepair3AcceptedPlanCannotAttachToADifferentGoal |
| N1.14 | Controller refuses classified-but-unbound generation | I9,I7 | KILLED | TestKernelRepair3ClassifiedGoalWithoutSafetyBindingIsRefusedBeforeAnyWorker, TestKernelRepair3MaterializationRefusesAClassifiedGoalWithoutTheBinding |
| N1.15 | Kernel-shaped content refused (WorkPlan.Validate) | I9 | KILLED | TestKernelShapedContentWithoutSafetyBindingIsRefused |
| N1.16 | Kernel-shaped content refused (proposal Validate) | I9 | KILLED | TestKernelShapedContentWithoutSafetyBindingIsRefused |
| N1.17 | Kernel-shaped: explicit kind | I9 | KILLED | TestRejectKernelShapedWithoutSafetyEachTellRefusesIndependently |
| N1.18 | Kernel-shaped: preserved specification bytes | I9 | KILLED | TestRejectKernelShapedWithoutSafetyEachTellRefusesIndependently |
| N1.19 | Kernel-shaped: gate provenance | I9 | KILLED | TestRejectKernelShapedWithoutSafetyEachTellRefusesIndependently |
| N1.20 | Kernel-shaped: relationship specification | I9 | KILLED | TestKernelShapedContentWithoutSafetyBindingIsRefused |
| N1.21 | Runtime admission fence uses classification | I9,I2 | KILLED | TestKernelRepair3RuntimeRefusesADowngradedGenerationBeforeAdmission |
| N1.22 | Materialization refuses safety-bearing turns via classification | I9,I1 | KILLED | TestKernelRepair3MaterializationRefusesAClassifiedGoalWithoutTheBinding |
| N1.09j | joint: both classification writers (proposal + attach) | I9 | KILLED | TestRepair4N9ClassificationRowDeletionStillRefusesStrippedImport |
| N2.01 | SealCompletion only for safety-bearing generation | I11 | KILLED | TestCompletionSealsAuthenticateExactBytesOnly |
| N2.02 | SealCompletion requires verified activation | I11,I6 | KILLED | TestCompletionSealsAuthenticateExactBytesOnly |
| N2.03 | SealCompletion refuses a conflicting existing seal | I11 | KILLED | TestCompletionSealsAuthenticateExactBytesOnly |
| N2.04 | LoadSealedCompletion checks digest equality | I11 | KILLED | TestSealedCompletionWhoseBytesDoNotMatchItsKeyIsRefused |
| N2.05 | Gate completion cites the request its dossier implies | I11 | KILLED | TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState |
| N2.06 | Gate request must be durably recorded (digest equality) | I11 | KILLED | TestKernelRepair3N2GateCompletionRefusesARecordedRequestThatDiffersFromTheImpliedOne |
| N2.07 | Gate decision citation equality | I11 | KILLED | TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState |
| N2.08 | Gate completion ceremony resolves | I8,I11 | KILLED | TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState |
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
| B5.02 | ApplyCompletions clears supplied flag (M1b) | I1 | KILLED | TestCompletionIsDerivedOnlyFromQualifiedLedgerEvidence, TestGitBoundValidationCleansUpAfterTimeout |
| B5.03 | VerifyPlanCompletions structural check (M1c) | I1,I4 | KILLED | TestCompletionIsDerivedOnlyFromQualifiedLedgerEvidence |
| B9.01 | controller verifies governing authority (M2a) | I3 | KILLED | TestKernelRepair3EveryOutwardEffectIsAuthorizedBeforeItHappens, TestKernelRepair3GateCompletionIsAuthorizedAtTheMomentItIsRecorded |
| B9.02 | prepare verifies governing authority | I3 | KILLED | TestKernelRepair3RevokedAuthorityIsRefusedBeforeTheWorkerRuns |
| B9.03 | gate reconcile: governing authority (M3) | I3 | KILLED | TestKernelRepair2B9RevokedGoverningAuthorityStopsFutureExecution |
| B9.04 | gate reconcile: activation (B15) | I2,I6 | KILLED | TestKernelRepair2B15GateReplayRequiresActivationAtTheSharedBoundary |
| B9.05 | plan authority lineage present | I3 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.06 | plan authority request is the acceptance request | I3 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.07 | plan authority request carries ceremony+activation binding | I3,I8 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.08 | plan authority decision matches plan lineage | I3 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.09 | baseline is the persisted generation | I3 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.10 | decision owner scope | I8 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.11 | decision by installation owner | I8 | REDUNDANT-BY-CONSTRUCTION |  |
| B9.12 | decision authority generation valid (root revocation) | I3 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.13 | gate decision issued by current root | I3,I8 | REDUNDANT-BY-CONSTRUCTION |  |
| B8.01 | protected decision requires resolvable ceremony record | I8 | KILLED | TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.02 | ceremony record must match decision | I8 | KILLED | TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState, TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.03 | ceremony record: owner/root lineage | I8 | KILLED | TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.04 | ceremony record: enrolled OS user | I8 | KILLED | TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState, TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.05 | legacy acceptance refuses safety-bearing proposal | I2,I8 | KILLED | TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState |
| B14.01 | worker fence refuses gate objective (M4a) | I7 | KILLED | TestWorkerResolutionRefusesAuthorityGateObjective |
| B14.02 | explicit gate objective routed to coordination (M4b) | I7 | KILLED | TestGateObjectiveFromEveryRouteReachesCoordinationAndNeverAProvider |
| B14.03 | safety plan refuses objective outside plan (M4c) | I7,I2 | KILLED | TestKernelRepair3ExplicitObjectiveMustBeAnOpenCandidateOfTheAcceptedPlan, TestValidationCancellationTerminatesDescendants |
| B14.04 | completed objective refused | I1 | KILLED | TestGitBoundValidationCleansUpAfterTimeout, TestKernelRepair3ExplicitObjectiveMustBeAnOpenCandidateOfTheAcceptedPlan |
| B14.05 | selected gate goes to coordination | I7 | KILLED | TestGateObjectiveFromEveryRouteReachesCoordinationAndNeverAProvider, TestGoalGateCompletesThroughTheSupportedOwnerCeremony |
| B10.01 | post-worker validator missing blocks (M5) | I4 | KILLED | TestMissingValidatorAfterWorkerBlocksSafetyCheckpoint |
| B10.02 | bound validator required for safety checkpoint | I4 | KILLED | TestMissingValidatorAfterWorkerBlocksSafetyCheckpoint |
| B10.03 | no-declared-validation never counts for safety (M9) | I4 | KILLED | TestNoDeclaredValidationCannotSettleASafetyCompletion |
| B10.04 | preflight profile-digest mismatch (M11) | I5 | KILLED | TestKernelRepair3ProfileMismatchIsRefusedBeforeTheWorkerRuns |
| B10.05 | candidate conformance decided before publication (M7b) | I4,I5 | KILLED | TestFailedCandidateConformanceIsDecidedBeforePublication |
| B10.06 | conformance acknowledgement required (M7a) | I4,I5 | KILLED | TestSafetyCompletionRejectsMalformedConformanceEvidence |
| B10.07 | integrated-pass evidence required to qualify | I4 | KILLED | TestKernelRepair3QualificationRequiresIntegratedPassEvidence, TestValidationCancellationTerminatesDescendants |
| B10.08 | settlement re-qualifies when not qualified before | I4 | KILLED | TestSafetyCompletionRejectsMalformedConformanceEvidence |
| B10.09 | governed output size bound | I5 | KILLED | TestKernelRepair3GovernedOutputMustBePresentAndBounded, TestValidationCancellationTerminatesDescendants |
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
| N5.02b | export verified right after checkout (creation time) | I5 | REDUNDANT-LAYER | TestValidationCancellationTerminatesDescendants |
| N5.03 | checkpoint re-bound after the run | I5,I10 | KILLED | TestGitBoundValidationRefusesAValidatorThatMovesTheWorkerCheckoutDuringTheRun, TestValidationCancellationTerminatesDescendants |
| N5.04 | export HEAD equals checkpoint | I5 | KILLED | TestGitBoundValidationRefusesAnExportWhoseHeadMoved |
| N5.05 | export tree equals qualified tree | I5 | REDUNDANT-BY-CONSTRUCTION |  |
| N5.06 | export index hint flags refused | I5 | KILLED | TestGitBoundValidationRefusesAnIndexHintEvenWithoutAContentChange |
| N5.07 | export status clean | I5 | REDUNDANT-LAYER |  |
| N5.08 | export gained no untracked/excluded files | I5 | REDUNDANT-LAYER | TestGitBoundValidationCleansUpAfterTimeout |
| N5.09 | export content re-hashed into a fresh index (stat/index-blind layer) | I5 | KILLED | TestGitBoundValidationContentRehashCatchesAStatBlindSameSizeRewrite, TestValidationCancellationTerminatesDescendants |
| N5.10 | joint: every export post-run layer (status+hint+untracked+content) | I5 | KILLED | TestGitBoundValidationContentRehashCatchesAStatBlindSameSizeRewrite, TestGitBoundValidationDetectsExportTamperingAndAlwaysCleansUp |
| N5.11 | committed validator digest at checkpoint (bind) | I5 | REDUNDANT-LAYER |  |
| N5.12 | exported validator digest | I5 | REDUNDANT-LAYER |  |
| N5.13 | joint: committed and exported validator digest | I5 | KILLED | TestGitBoundValidationRefusesValidatorThatDiffersFromTheProfile, TestValidationDriftAfterAdmissionFailsClosedBeforePublication |
| N5.14 | validator executable mode at checkpoint | I5 | REDUNDANT-BY-CONSTRUCTION | TestGitBoundValidationCleansUpAfterTimeout |
| N5.14x | exported validator executable mode | I5 | REDUNDANT-BY-CONSTRUCTION |  |
| N5.14j | joint: validator executable mode (bind + export) | I5 | REDUNDANT-BY-CONSTRUCTION |  |
| N5.15 | bind: HEAD is the checkpoint | I5,I10 | REDUNDANT-LAYER | TestValidationCancellationTerminatesDescendants |
| N5.16 | bind: local branch points at checkpoint | I5,I10 | REDUNDANT-LAYER | TestValidationCancellationTerminatesDescendants |
| N5.17 | publish: HEAD is the checkpoint | I10 | KILLED | TestGitBoundValidationRefusesCheckpointAndTreeSubstitution, TestGitBoundValidationRefusesSubstitutionOnADetachedHead |
| N5.18 | joint: every HEAD-equality layer | I5,I10 | KILLED | TestGitBoundValidationRefusesCheckpointAndTreeSubstitution, TestGitBoundValidationRefusesSubstitutionOnADetachedHead |
| N5.19 | qualified tree recorded at run time is re-compared | I5,I10 | KILLED | TestGitBoundValidationRefusesCheckpointAndTreeSubstitution |
| N5.20 | checkpoint must be a full object id | I5 | KILLED | TestGitBoundValidationRequiresAFullObjectId |
| N5.21 | publication pushes the exact qualified object id | I10 | EQUIVALENT |  |
| N5.22 | publication verifies the remote ref is the checkpoint | I10 | KILLED | TestGitBoundValidationPublicationVerifiesTheRemoteRef, TestValidationCancellationTerminatesDescendants |
| N5.23 | controller git runs without worker fsmonitor/hooks | I5 | KILLED | TestGitBoundValidationNeverExecutesWorkerCheckoutHooksOrFsmonitor |
| N5.24 | controller git ignores replace objects | I5 | KILLED | TestGitBoundValidationCleansUpAfterTimeout, TestGitBoundValidationIgnoresReplaceRefsInTheWorkerCheckout |
| N6.01 | output bound enforced in the production collector | I4,I5 | KILLED | TestBoundValidationOutputOverflowFailsClosed, TestValidationCancellationTerminatesDescendants |
| N6.02 | overflow fails closed even with exit 0 | I4,I5 | KILLED | TestBoundValidationOutputOverflowFailsClosed, TestValidationOutputBoundFailsClosedThroughTheRealCollector |
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
| R4.11 | LoadAuthorityDecision requires decision liveness | I3,I12 | KILLED | TestDecisionLivenessRowDeletionFailsClosed, TestRevocationRowDeletionDoesNotRestoreAUnprotectedDecision |
| R4.12 | identical decision replay cannot resurrect a retired decision | I12 | KILLED | TestRevocationRowDeletionDoesNotRestoreAUnprotectedDecision |
| R4.13 | revocation retires the decision liveness record | I3,I12 | KILLED | TestRevocationRowDeletionDoesNotRestoreAUnprotectedDecision |
| R4.14 | generation invalidation retires the generation liveness record | I3,I12 | KILLED | TestGenerationInvalidationRowDeletionDoesNotRestoreTheGeneration |
| R4.15 | ValidateAuthorityGeneration requires generation liveness | I3,I12 | KILLED | TestGenerationInvalidationRowDeletionDoesNotRestoreTheGeneration, TestGenerationLivenessRowDeletionFailsClosed |
| R4.16 | lineage walk requires liveness of every generation | I3,I12 | KILLED | TestLineageWalkRefusesAChildWithoutItsLivenessRecord |
| R4.17 | current installation root must be positively live (same snapshot as the generations) | I3,I12 | KILLED | TestRootLivenessRowDeletionLeavesNoCurrentRoot, TestRootSuccessionRollbackByRowDeletionDoesNotReviveThePredecessor |
| R4.18 | publication consumer: generation liveness | I10,I12 | KILLED | TestPublicationFencesRequireLivenessRecords |
| R4.19 | publication consumer: decision liveness | I10,I12 | KILLED | TestPublicationFencesRequireLivenessRecords |
| R4.20 | recovery admission: decision liveness | I10,I12 | KILLED | TestRecoveryAdmissionRecheckRequiresLivenessRecords |
| R4.21 | recovery admission: generation liveness | I10,I12 | KILLED | TestRecoveryAdmissionRecheckRequiresLivenessRecords |
| R4.22 | ordinary re-request requires the predecessor decision liveness | I3,I12 | KILLED | TestSaveAuthorityReRequestUsesImmutableHistoricalEvidenceAndConverges |
| R4.23 | owner-decision authority requires issuing generation liveness | I3,I8,I12 | REDUNDANT-LAYER |  |
| R4.23j | joint: owner-decision generation liveness (plan authority + ValidateAuthorityGeneration) | I3,I8,I12 | KILLED | TestGenerationInvalidationRowDeletionDoesNotRestoreTheGeneration, TestGenerationLivenessRowDeletionFailsClosed |
| R4.24 | classification is derived when the classification row is missing | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.24e | classification derivation end-to-end (N9 counterexample) | I9,I12 | KILLED | TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed, TestRepair4N9ClassificationRowDeletionStillRefusesStrippedImport |
| R4.25 | derivation reads surviving generations of the Goal | I9,I12 | KILLED | TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed, TestRepair4SweepClassificationRowPlusAnyOtherRowStillClassified |
| R4.26 | derivation reads surviving proposals of the Goal | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals, TestRepair4SweepClassificationRowPlusAnyOtherRowStillClassified |
| R4.27 | derivation refuses conflicting kernel evidence | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.28 | derivation fails closed on an unreadable proposal | I9,I12 | REDUNDANT-LAYER |  |
| R4.28j | joint: proposal decrypt failure + decode failure both swallowed | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.29 | derivation fails closed on an unreadable generation | I9,I12 | KILLED | TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed |
| R4.30 | derivation attributes proposals by Goal identity | I9,I12 | KILLED | TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.31 | derivation attributes generations by Goal identity | I9,I12 | KILLED | TestRepair4ClassificationDerivationFromGenerationsIsAttributedAndFailsClosed |
| R4.32 | decision liveness written atomically for delegated decision+generation | I12 | KILLED | TestAbandonmentConfirmationAppendsExactFrozenPayload, TestAbandonmentConfirmationRejectsChangedStateAndSubstitutedBytes |
| R4.33 | protected decision written with its liveness in one transaction | I12 | KILLED | TestActivationRefusalSurvivesRestartAndRecoversOnlyWithRestoredActivation, TestContinuationAcceptsAndAttachesUnderValidActivation |
