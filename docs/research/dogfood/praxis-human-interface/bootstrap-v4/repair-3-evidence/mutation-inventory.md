# Consolidated guard / mutation inventory (repair 3)

Generated from `mutation-inventory.json` (harness: `mutation/harness.py`, catalogue: `mutation/catalogue.py`). Every mutation is applied to a SCRATCH COPY of the repository (never the real tree); each file is restored and re-hashed (`RESTORE-CHECK OK` in the run logs). A guard is KILLED only if at least one relevant regression fails; kills attributable only to load-sensitive unrelated tests do not count.

## Universe and scope of the claim

The universe is the set of safety guards in the PRE-V4 kernel non-test source of `pkg/contracts`, `internal/bootstrapv4`, `internal/goalstore`, `internal/goaldrive` and the lifecycle/decide/settlement/inspect surfaces of `cmd/praxis`, **as enumerated in `mutation/catalogue.py`**. It folds together: the implementer's repair-1/2 mutation families (B5 completed-flag, B7 Save guard, B8 ceremony resolution and legacy acceptance, B9 controller and gate authority, B10 validator-missing / binding / pre-publication conformance, B11 image checks (N3.01-N3.08), B13 trailing content (N4), B14 dispatch fence; **B12 (stale package cache) has no mutation because no cache remains: the property is exercised by `TestKernelRepair2B12...`, not by removing code**), Astra Review #2's probes (replayed separately), Astra Review #3's M1a..M13 list re-derived against the current source, the five surviving post-worker mutations and pre-publication revalidation (I10.*, M12.*), N1-N7 guards, and the I9-I11 shared boundaries. It is **not** a claim of universal mutation completeness.

Excluded / unmodeled categories: raw OS signals; direct event-store or SQLite writes; forging by a holder of the installation storage key (accepted bootstrap trust root); deletion of authenticated rows by a raw writer of the database file; the non-Unix validation fallback; Windows and Linux paths (only darwin/arm64 was run); guards that fire only on states this platform will not produce on demand (`N3.07`); pure error-wrapping; advisory (non-safety) validation paths; the plaintext ledger's own decoders (the seal is the control).

## Counts

| Measure | Count |
|---|---|
| mutations applied (total) | 155 |
| single-guard mutations (= enumerated guards) | 147 |
| joint mutations (redundant layers removed together) | 8 |
| killed by a relevant regression | 134 |
| survived: redundant layer (another layer enforces it; a joint mutation is killed) | 11 |
| survived: redundant by construction (guarded state unreachable once an earlier boundary holds, or the OS refuses) | 7 |
| survived: equivalent mutant (no observable change) | 2 |
| survived: unmodeled on this platform | 1 |
| survived with no explanation | 0 |
| **surviving total (all classified)** | 21 |

Result sources (later overrides earlier): results-final2-main.json, results-rerun.json. The main pass (`final-main`) ran the 153 mutations defined at the time; after it, tests were strengthened and 11 mutations were re-run (`results-rerun.json`): `B10.01`, `B10.02`, `N5.24` moved from surviving to killed, joints `N5.10`, `N5.13`, `N5.18` were killed, and two mutations (`N5.14x`, `N5.14j`) were added. Earlier exploratory passes are preserved but not counted.

## Survivors (all)

| Id | Guard | Class | Why | Joint proof |
|---|---|---|---|---|
| N1.09 | Attach classifies its Goal (mark at attach) | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) attach re-marks the Goal that the safety proposal already classified; joint mutation N1.09j (proposal+attach) is killed by TestKernelRepair3N1AProposalClassifiesItsGoalBeforeAnythingIsAccepted / TestKernelRepair3N1Stripping... | |
| N2.10 | Gate decision owner/root authority current | REDUNDANT-BY-CONSTRUCTION | the gate-completion currency re-check of owner/root authority is masked by the plan-level governing-authority check that runs first in the same call (prepareAuthorityGate, B9.12/B9.09 killed) | |
| M12.01 | claim == selected unit before publication | REDUNDANT-LAYER | the pre-publication claim==selected check cannot be reached with a mismatching claim because the checkpoint inspection (M12.03, killed by its own message assertion) and settlement (M12.02, killed by a direct settleCompletion test) both refuse first; joint M12.04 is killed | |
| B9.11 | decision by installation owner | REDUNDANT-BY-CONSTRUCTION | the owner principal and the governance scope both derive from the same installation digest; scope is checked first (B9.10 killed), so a scope-correct decision cannot be by another owner short of a forged record | |
| B9.13 | gate decision issued by current root | REDUNDANT-BY-CONSTRUCTION | gate decisions must be issued by the CURRENT root; a decision by a superseded or revoked root already fails ValidateAuthorityGeneration (B9.12 killed) and plan-level authority (B9.09/B9.12). Reaching the current-root comparison alone needs a root succession fixture that keeps the old generation valid | |
| N4.04 | trailing content refused (walk) | REDUNDANT-LAYER | trailing-content refusal exists in the type-directed walk and again in the decode; each alone is masked by the other; joint N4.06 is killed | |
| N4.05 | trailing content refused (decode) | REDUNDANT-LAYER | see N4.04 (joint N4.06 killed) | |
| N5.02b | export verified right after checkout (creation time) | REDUNDANT-LAYER | creation-time export verification only fails fast; the same predicate set is re-proved after the run (N5.02 killed) and the content is re-hashed there | |
| N5.05 | export tree equals qualified tree | REDUNDANT-BY-CONSTRUCTION | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) an export's HEAD^{tree} is a function of its HEAD commit, which N5.04 compares; the tree comparison cannot differ while the commit is equal | |
| N5.07 | export status clean | REDUNDANT-LAYER | export status check is covered by the content re-hash and the hint scan; joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 | |
| N5.08 | export gained no untracked/excluded files | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) untracked/excluded-file scan is covered by the content re-hash (which adds all files); joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 | |
| N5.11 | committed validator digest at checkpoint (bind) | REDUNDANT-LAYER | the committed-validator digest at the checkpoint is re-checked on the exported blob (N5.12) and the whole export tree is re-hashed; the two digest layers removed together are killed (N5.13) | |
| N5.12 | exported validator digest | REDUNDANT-LAYER | the exported validator digest is re-checked on the committed blob (N5.11 killed) and the whole export tree is re-hashed after the run; joint N5.13 is killed | |
| N5.14 | validator executable mode at checkpoint | REDUNDANT-BY-CONSTRUCTION | a validator that is not an executable regular file cannot be executed: the OS refuses the exec and the run fails closed. The explicit mode checks (bind N5.14, export N5.14x) only give an earlier, clearer message; even their joint mutation N5.14j survives because exec permission still refuses | |
| N5.14x | exported validator executable mode | REDUNDANT-BY-CONSTRUCTION | see N5.14 | |
| N5.14j | joint: validator executable mode (bind + export) | REDUNDANT-BY-CONSTRUCTION | see N5.14 | |
| N5.15 | bind: HEAD is the checkpoint | REDUNDANT-LAYER | HEAD==checkpoint is asserted by four layers (bind, bind branch ref, ReadCheckpointArtifact, PushAndVerify); joint N5.18 of the bind/branch/push layers is killed | |
| N5.16 | bind: local branch points at checkpoint | REDUNDANT-LAYER | see N5.15 (joint N5.18 killed) | |
| N5.21 | publication pushes the exact qualified object id | EQUIVALENT | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) PushAndVerify has already proved HEAD==head, so pushing HEAD or the object id publishes the same commit | |
| N6.05 | collector keeps draining after overflow (no deadlock) | EQUIVALENT | after overflow the buffer-limit branch sets the same flags and cancels again and still returns len(p): the early branch only saves work | |
| N3.07 | kernel reports the signature valid | UNMODELED | the kernel never reported CS_VALID cleared in any scenario constructible here (flags 0x22020201 throughout); the check fails closed on a state this platform will not produce on demand | |

## All mutations

| Id | Guard | Invariants | Verdict | Killing tests (first 2) |
|---|---|---|---|---|
| N1.01 | Save refuses safety-bearing plan (B7) | I2,I9 | KILLED | TestKernelRepair2B7BaselinePersistenceSurfacesRefuseSafetyBearingPlans |
| N1.02 | Save refuses any plan for a classified Goal | I9 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestKernelRepair3N1StrippingTheSafetyBindingCannotDowngradeAClassifiedGoal |
| N1.03 | Proposal refuses legacy proposal for classified Goal | I9 | KILLED | TestKernelRepair3N1StrippingTheSafetyBindingCannotDowngradeAClassifiedGoal, TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.04 | Review refuses legacy review for classified Goal | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.05 | Legacy acceptance refuses classified Goal | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.06 | Authority-backed acceptance bridge refuses classified Goal | I9 | KILLED | TestSafetyClassificationRefusesTheAuthorityBackedAcceptanceBridgeOnItsOwn |
| N1.07 | Attach refuses legacy plan for classified Goal | I9 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.08 | Safety proposal classifies its Goal (mark at proposal) | I9 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestKernelRepair3N1AProposalClassifiesItsGoalBeforeAnythingIsAccepted |
| N1.09 | Attach classifies its Goal (mark at attach) | I9 | REDUNDANT-LAYER |  |
| N1.10 | Classification kernel-version mismatch refused (consistency) | I9 | KILLED | TestSafetyConsistencyRefusesAnotherKernelVersion |
| N1.11 | Classification kernel-version mismatch refused (mark) | I9 | KILLED | TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
| N1.12 | Missing binding on classified Goal refused | I9 | KILLED | TestKernelRepair3N1StrippingTheSafetyBindingCannotDowngradeAClassifiedGoal, TestSafetyClassificationRefusesEveryLegacyAdmissionLayerOnItsOwn |
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
| N1.09j | joint: both classification writers (proposal + attach) | I9 | KILLED (joint) | TestKernelRepair3N1AProposalClassifiesItsGoalBeforeAnythingIsAccepted, TestKernelRepair3N1StrippingTheSafetyBindingCannotDowngradeAClassifiedGoal |
| N2.01 | SealCompletion only for safety-bearing generation | I11 | KILLED | TestCompletionSealsAuthenticateExactBytesOnly |
| N2.02 | SealCompletion requires verified activation | I11,I6 | KILLED | TestCompletionSealsAuthenticateExactBytesOnly |
| N2.03 | SealCompletion refuses a conflicting existing seal | I11 | KILLED | TestCompletionSealsAuthenticateExactBytesOnly |
| N2.04 | LoadSealedCompletion checks digest equality | I11 | KILLED | TestSealedCompletionWhoseBytesDoNotMatchItsKeyIsRefused |
| N2.05 | Gate completion cites the request its dossier implies | I11 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState |
| N2.06 | Gate request must be durably recorded (digest equality) | I11 | KILLED | TestKernelRepair3N2GateCompletionRefusesARecordedRequestThatDiffersFromTheImpliedOne |
| N2.07 | Gate decision citation equality | I11 | KILLED | TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState |
| N2.08 | Gate completion ceremony resolves | I8,I11 | KILLED | TestKernelRepair3N2GateCompletionLineageIsReResolvedFromAuthenticatedState |
| N2.10 | Gate decision owner/root authority current | I3,I8,I11 | REDUNDANT-BY-CONSTRUCTION |  |
| N2.11 | Gate decision outcome must be approve | I11 | KILLED | TestKernelRepair3N2CompletionCitingARejectedGateDecisionIsNotEffective |
| N2.12 | Revoked decision is not effective (LoadAuthorityDecision revocation) | I3,I11 | KILLED | TestKernelRepair3N2RevokedGateDecisionKeepsHistoryButStopsAuthorizingWork |
| N2.13 | Consumption: completion belongs to this Goal generation | I11 | KILLED | TestKernelRepair3ConsumptionChecksGenerationAndBytesIndependentlyOfTheStore |
| N2.14 | Consumption: sealed bytes equal ledger row | I11 | KILLED | TestKernelRepair3ConsumptionChecksGenerationAndBytesIndependentlyOfTheStore |
| N2.15 | Consumption: seal must exist | I11 | KILLED (joint) | TestKernelRepair3CompletionSealedForAnotherGenerationIsRefused, TestKernelRepair3ConsumptionChecksGenerationAndBytesIndependentlyOfTheStore |
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
| N7.06 | Inspect consumes authenticated completions | I11 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestKernelRepair3InspectDoesNotDisplayAForgedCompletionAndIsNotDrivable |
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
| M12.04 | joint: all three claim==selected layers | I1 | KILLED (joint) | TestKernelRepair3ClaimOfANonSelectedUnitPublishesNothing, TestKernelRepair3SettlementRefusesANonSelectedClaimByItself |
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
| B9.10 | decision owner scope | I8 | KILLED | TestConcurrentInstallationFanoutAcrossProcesses, TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.11 | decision by installation owner | I8 | REDUNDANT-BY-CONSTRUCTION |  |
| B9.12 | decision authority generation valid (root revocation) | I3 | KILLED | TestKernelRepair3PlanAuthorityLineageIsRefusedFieldByField |
| B9.13 | gate decision issued by current root | I3,I8 | REDUNDANT-BY-CONSTRUCTION |  |
| B8.01 | protected decision requires resolvable ceremony record | I8 | KILLED | TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.02 | ceremony record must match decision | I8 | KILLED | TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState, TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.03 | ceremony record: owner/root lineage | I8 | KILLED | TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.04 | ceremony record: enrolled OS user | I8 | KILLED | TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState, TestKernelRepair3CeremonyMismatchMatrixIsRefusedForAWorkPlanAcceptance |
| B8.05 | legacy acceptance refuses safety-bearing proposal | I2,I8 | KILLED | TestKernelRepair2B8PublicPersistenceCannotManufactureCeremonyBackedState |
| B14.01 | worker fence refuses gate objective (M4a) | I7 | KILLED | TestWorkerResolutionRefusesAuthorityGateObjective |
| B14.02 | explicit gate objective routed to coordination (M4b) | I7 | KILLED | TestGateObjectiveFromEveryRouteReachesCoordinationAndNeverAProvider, TestGitBoundValidationCleansUpAfterTimeout |
| B14.03 | safety plan refuses objective outside plan (M4c) | I7,I2 | KILLED | TestKernelRepair3ExplicitObjectiveMustBeAnOpenCandidateOfTheAcceptedPlan |
| B14.04 | completed objective refused | I1 | KILLED | TestKernelRepair3ExplicitObjectiveMustBeAnOpenCandidateOfTheAcceptedPlan |
| B14.05 | selected gate goes to coordination | I7 | KILLED | TestGateObjectiveFromEveryRouteReachesCoordinationAndNeverAProvider, TestGoalGateCompletesThroughTheSupportedOwnerCeremony |
| B10.01 | post-worker validator missing blocks (M5) | I4 | KILLED | TestMissingValidatorAfterWorkerBlocksSafetyCheckpoint |
| B10.02 | bound validator required for safety checkpoint | I4 | KILLED | TestMissingValidatorAfterWorkerBlocksSafetyCheckpoint |
| B10.03 | no-declared-validation never counts for safety (M9) | I4 | KILLED | TestGitBoundValidationCleansUpAfterTimeout, TestNoDeclaredValidationCannotSettleASafetyCompletion |
| B10.04 | preflight profile-digest mismatch (M11) | I5 | KILLED | TestKernelRepair3ProfileMismatchIsRefusedBeforeTheWorkerRuns, TestValidationCancellationTerminatesDescendants |
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
| N4.06 | trailing content refused (both layers) | I5 | KILLED (joint) | TestDossierResolutionRefusesMalformedOrAmbiguousBytes, TestUnmarshalExactJSONRefusesAmbiguousSafetyEvidence |
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
| N5.06 | export index hint flags refused | I5 | KILLED | TestGitBoundValidationRefusesAnIndexHintEvenWithoutAContentChange |
| N5.07 | export status clean | I5 | REDUNDANT-LAYER |  |
| N5.08 | export gained no untracked/excluded files | I5 | REDUNDANT-LAYER |  |
| N5.09 | export content re-hashed into a fresh index (stat/index-blind layer) | I5 | KILLED | TestGitBoundValidationCleansUpAfterTimeout, TestGitBoundValidationContentRehashCatchesAStatBlindSameSizeRewrite |
| N5.10 | joint: every export post-run layer (status+hint+untracked+content) | I5 | KILLED (joint) | TestGitBoundValidationContentRehashCatchesAStatBlindSameSizeRewrite, TestGitBoundValidationDetectsExportTamperingAndAlwaysCleansUp |
| N5.11 | committed validator digest at checkpoint (bind) | I5 | REDUNDANT-LAYER |  |
| N5.12 | exported validator digest | I5 | REDUNDANT-LAYER |  |
| N5.13 | joint: committed and exported validator digest | I5 | KILLED (joint) | TestGitBoundValidationRefusesValidatorThatDiffersFromTheProfile, TestValidationDriftAfterAdmissionFailsClosedBeforePublication |
| N5.14 | validator executable mode at checkpoint | I5 | REDUNDANT-BY-CONSTRUCTION |  |
| N5.14x | exported validator executable mode | I5 | REDUNDANT-BY-CONSTRUCTION |  |
| N5.14j | joint: validator executable mode (bind + export) | I5 | REDUNDANT-BY-CONSTRUCTION (joint) |  |
| N5.15 | bind: HEAD is the checkpoint | I5,I10 | REDUNDANT-LAYER |  |
| N5.16 | bind: local branch points at checkpoint | I5,I10 | REDUNDANT-LAYER |  |
| N5.17 | publish: HEAD is the checkpoint | I10 | KILLED | TestGitBoundValidationCleansUpAfterTimeout, TestGitBoundValidationRefusesCheckpointAndTreeSubstitution |
| N5.18 | joint: every HEAD-equality layer | I5,I10 | KILLED (joint) | TestGitBoundValidationCleansUpAfterTimeout, TestGitBoundValidationRefusesCheckpointAndTreeSubstitution |
| N5.19 | qualified tree recorded at run time is re-compared | I5,I10 | KILLED | TestGitBoundValidationRefusesCheckpointAndTreeSubstitution |
| N5.20 | checkpoint must be a full object id | I5 | KILLED | TestGitBoundValidationRequiresAFullObjectId |
| N5.21 | publication pushes the exact qualified object id | I10 | EQUIVALENT |  |
| N5.22 | publication verifies the remote ref is the checkpoint | I10 | KILLED | TestGitBoundValidationPublicationVerifiesTheRemoteRef |
| N5.23 | controller git runs without worker fsmonitor/hooks | I5 | KILLED | TestGitBoundValidationNeverExecutesWorkerCheckoutHooksOrFsmonitor |
| N5.24 | controller git ignores replace objects | I5 | KILLED | TestGitBoundValidationIgnoresReplaceRefsInTheWorkerCheckout |
| N6.01 | output bound enforced in the production collector | I4,I5 | KILLED | TestBoundValidationOutputOverflowFailsClosed, TestValidationOutputBoundFailsClosedThroughTheRealCollector |
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
