# Repair 6 guard/mutation inventory

Generated from the final Repair 6 source by `build_inventory.py` over `results-r6-rest.json`, `results-r6-kc-final.json`, `results-r6-n603-run1.json`, `results-r6-n603-run2.json`, `results-r6-n603-run3.json`. Scratch mutation copies live outside the repository; every mutated file was restored (RESTORE-CHECK OK in each run log).

| Measure | Count |
|---|---|
| mutations | 304 |
| single-guard | 290 |
| joint | 14 |
| killed by a relevant regression | 274 |
| redundant layer (joint proof killed) | 16 |
| redundant by construction | 9 |
| equivalent | 3 |
| unmodeled | 2 |
| suspect kills (killed only by unrelated timing tests) | 0 |
| **survived, unexplained** | **0** |

## Re-run and test-strengthening notes

- N6.03 (overflow cancels the process group): SURVIVED once in the three-worker phase-2 run (results-r6-rest.json) and was KILLED in 3 of 3 isolated reruns (results-r6-n603-run1..3.json) by TestValidationOutputBoundFailsClosedThroughTheRealCollector, the same test that killed it in Repair 5. Load-sensitive; the internal/goaldrive validation code it mutates is not changed by Repair 6. The inventory counts it as killed and records the first-pass survival here.
- R6.18, R6.19 and R6.21 SURVIVED the first Keychain-family pass and exposed three real test gaps (a write as the first operation after an interrupted re-key; a refused password state must leave both items untouched and be exactly corrupt; a governed reset must discard a pending item). The tests were strengthened and the whole Keychain family (40 mutations) was re-run fresh on the final tests (results-r6-kc-final.json): all three are killed.
- Keychain-family mutations ran with one worker, PRAXIS_REQUIRE_KEYCHAIN=1, user interaction disabled and unique service identities: no test was skipped in any of them.
- R5.73, R5.82, R5.84 are carried classifications from Repair 5 (their anchors were re-pointed because Repair 6 moved the code). R6.02 (EQUIVALENT) and R6.16 (REDUNDANT-LAYER, joint R6.16j killed) are new.

## Every mutation that was not killed, with its classification

| id | guard | class | reason |
|---|---|---|---|
| N1.09 | Attach classifies its Goal (mark at attach) | REDUNDANT-LAYER | attach re-marks the Goal that the safety proposal already classified; joint mutation N1.09j (proposal+attach) is killed by TestKernelRepair3N1AProposalClassifiesItsGoalBeforeAnythingIsAccepted / TestKernelRepair3N1Stripping... |
| N2.10 | Gate decision owner/root authority current | REDUNDANT-BY-CONSTRUCTION | the gate-completion currency re-check of owner/root authority is masked by the plan-level governing-authority check that runs first in the same call (prepareAuthorityGate, B9.12/B9.09 killed) |
| M12.01 | claim == selected unit before publication | REDUNDANT-LAYER | the pre-publication claim==selected check cannot be reached with a mismatching claim because the checkpoint inspection (M12.03, killed by its own message assertion) and settlement (M12.02, killed by a direct settleCompletion test) both refuse first; joint M12.04 is killed |
| B9.11 | decision by installation owner | REDUNDANT-BY-CONSTRUCTION | the owner principal and the governance scope both derive from the same installation digest; scope is checked first (B9.10 killed), so a scope-correct decision cannot be by another owner short of a forged record |
| B9.13 | gate decision issued by current root | REDUNDANT-BY-CONSTRUCTION | gate decisions must be issued by the CURRENT root; a decision by a superseded or revoked root already fails ValidateAuthorityGeneration (B9.12 killed) and plan-level authority (B9.09/B9.12). Reaching the current-root comparison alone needs a root succession fixture that keeps the old generation valid |
| N4.04 | trailing content refused (walk) | REDUNDANT-LAYER | trailing-content refusal exists in the type-directed walk and again in the decode; each alone is masked by the other; joint N4.06 is killed |
| N4.05 | trailing content refused (decode) | REDUNDANT-LAYER | see N4.04 (joint N4.06 killed) |
| N5.02b | export verified right after checkout (creation time) | REDUNDANT-LAYER | creation-time export verification only fails fast; the same predicate set is re-proved after the run (N5.02 killed) and the content is re-hashed there |
| N5.05 | export tree equals qualified tree | REDUNDANT-BY-CONSTRUCTION | an export's HEAD^{tree} is a function of its HEAD commit, which N5.04 compares; the tree comparison cannot differ while the commit is equal |
| N5.07 | export status clean | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) export status check is covered by the content re-hash and the hint scan; joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 |
| N5.08 | export gained no untracked/excluded files | REDUNDANT-LAYER | untracked/excluded-file scan is covered by the content re-hash (which adds all files); joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 |
| N5.11 | committed validator digest at checkpoint (bind) | REDUNDANT-LAYER | the committed-validator digest at the checkpoint is re-checked on the exported blob (N5.12) and the whole export tree is re-hashed; the two digest layers removed together are killed (N5.13) |
| N5.12 | exported validator digest | REDUNDANT-LAYER | the exported validator digest is re-checked on the committed blob (N5.11 killed) and the whole export tree is re-hashed after the run; joint N5.13 is killed |
| N5.14 | validator executable mode at checkpoint | REDUNDANT-BY-CONSTRUCTION | a validator that is not an executable regular file cannot be executed: the OS refuses the exec and the run fails closed. The explicit mode checks (bind N5.14, export N5.14x) only give an earlier, clearer message; even their joint mutation N5.14j survives because exec permission still refuses |
| N5.14x | exported validator executable mode | REDUNDANT-BY-CONSTRUCTION | see N5.14 |
| N5.14j | joint: validator executable mode (bind + export) | REDUNDANT-BY-CONSTRUCTION | see N5.14 |
| N5.15 | bind: HEAD is the checkpoint | REDUNDANT-LAYER | HEAD==checkpoint is asserted by four layers (bind, bind branch ref, ReadCheckpointArtifact, PushAndVerify); joint N5.18 of the bind/branch/push layers is killed |
| N5.16 | bind: local branch points at checkpoint | REDUNDANT-LAYER | see N5.15 (joint N5.18 killed) |
| N5.21 | publication pushes the exact qualified object id | EQUIVALENT | PushAndVerify has already proved HEAD==head, so pushing HEAD or the object id publishes the same commit |
| N6.05 | collector keeps draining after overflow (no deadlock) | EQUIVALENT | after overflow the buffer-limit branch sets the same flags and cancels again and still returns len(p): the early branch only saves work |
| N3.07 | kernel reports the signature valid | UNMODELED | the kernel never reported CS_VALID cleared in any scenario constructible here (flags 0x22020201 throughout); the check fails closed on a state this platform will not produce on demand |
| R4.23 | owner-decision authority requires issuing generation liveness | REDUNDANT-LAYER | owner-decision issuing-generation liveness is checked in ValidateAuthorityGeneration (killed as R4.15) and again in the plan-authority path; the plan-authority check alone is masked; joint R4.23j is killed by the generation invalidation/liveness regressions |
| R4.28 | derivation fails closed on an unreadable candidate | REDUNDANT-LAYER | an undecryptable proposal row is refused at the load AND again when its (empty) payload fails to decode; each alone is masked by the other; joint R4.28j is killed by TestClassificationIsDerivedFromSurvivingSafetyBearingProposals |
| R4.29 | derivation fails closed on an unreadable generation | REDUNDANT-LAYER | an undecryptable generation row is refused at the generic load AND again when the generation is loaded for extraction; each alone is masked by the other; joint R4.29j is killed |
| R5.64 | the fact append takes the writer lock | REDUNDANT-BY-CONSTRUCTION | the store opens every transaction with _txlock=immediate (internal/state/sqlite.go), so the writer lock is already held when the builder runs; the explicit lock statement is defence in depth. Serialisation of appenders on separate connections is asserted by TestAppendGovernanceFactSerialisesAppendersOnSeparateConnections, and a second appender is also refused by the anchor's compare-and-set (R5.21) and the sequence-keyed primary key |
| R5.73 | keychain: read-back verified | UNMODELED | the real Keychain never reports a write it did not make, so the read-back guard cannot be exercised against it; the same guard on the test double (a backend that reports success without persisting) is killed by TestFAAAnchorWriteThatDoesNotStickIsRefused through the repository, and the Keychain compare-and-set guards around it are killed (R5.70-R5.72, R5.74) |
| R5.82 | keychain: a read never creates the keychain | REDUNDANT-LAYER | a read of a missing keychain is refused before anything is created; if that early return is removed the create path builds a keychain, finds no item, and the cleanup of an unpopulated new keychain removes it again, so the observable result is the same; joint R5.82j is killed by TestKeychainAnchorRemovedFileReadsAsMissing and TestKeychainAnchorCreatedButUnpopulatedKeychainIsRemoved |
| R5.84 | keychain: a missing keychain file is a missing anchor | REDUNDANT-BY-CONSTRUCTION | a stat error other than not-exist (permission, I/O) is refused explicitly, but the advisory lock file in the same directory is opened first and fails with the same condition, so the state cannot be reached without the earlier refusal |
| R6.02 | a freshly created file is not re-keyed (it has a fresh password and no earlier copy) | EQUIVALENT | re-keying a freshly created file as well is the fail-safe direction: a new file has a fresh random password and no earlier copy exists, so the extra re-key changes no security property and no test can require its absence without pinning an implementation detail (the created-file password is still unique, R6.01/R6.06/R6.08 are killed) |
| R6.16 | neither password item present is a missing-password (corrupt) anchor | REDUNDANT-LAYER | with neither password item present the code falls through to the pending branch with an empty password, the file cannot be unlocked with it, and that failure is classified corrupt as well; each layer alone is masked by the other; joint R6.16j (missing-password check + unlock-failure classification) is killed |

## Repair 6 re-key lifecycle mutations (all 25)

| id | guard | verdict | killed by |
|---|---|---|---|
| R6.01 | Set re-keys the file after every successful advance (N16 replay is refused) | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword,TestKeychainAnchorEveryAdv |
| R6.29 | N16 end to end: Review 6's replay of an opaque earlier keychain file plus its database is refused (no re-key after Set) | KILLED | TestRepair6OpaqueKeychainFileReplayIsRefused |
| R6.02 | a freshly created file is not re-keyed (it has a fresh password and no earlier copy) | EQUIVALENT |  |
| R6.03 | a failed re-key takes the advance back (the refused Set leaves the anchor where it was) | KILLED | TestKeychainAnchorFailedRekeyNeverAdvancesOrStrandsTheAnchor,TestKeychainAnchorMissingRekeyEntryPointRefusesEveryAdvance |
| R6.04 | a failed re-key is unavailable, never success or corruption (Set) | KILLED | TestKeychainAnchorFailedRekeyNeverAdvancesOrStrandsTheAnchor,TestKeychainAnchorMissingRekeyEntryPointRefusesEveryAdvance |
| R6.05 | an advance that could not be taken back after a failed re-key is reported, not hidden | KILLED | TestKeychainAnchorFailedRekeyThatCannotBeUndoneIsTheStrandedAheadState |
| R6.06 | Revert (undo) re-keys the file too | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword,TestKeychainAnchorEveryAdv |
| R6.07 | a failed re-key during an undo is reported | KILLED | TestKeychainAnchorInterruptedUndoRecoversAtEveryStep,TestKeychainAnchorMissingRekeyEntryPointRefusesEveryAdvanceAndStill |
| R6.08 | the new password is fresh, never the old one (no reuse) | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword,TestKeychainAnchorEveryAdv |
| R6.09 | the new password is recorded as pending BEFORE the file changes | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword,TestKeychainAnchorFailedRe |
| R6.10 | a refused re-key discards its pending password | KILLED | TestKeychainAnchorMissingRekeyEntryPointRefusesEveryAdvanceAndStillReads,TestKeychainAnchorRekeyDependsOnAnUndocumentedE |
| R6.11 | the current password item is updated after the file is re-keyed | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword,TestKeychainAnchorCurrentA |
| R6.12 | the pending password is removed once the re-key is complete | KILLED | TestKeychainAnchorEveryAdvanceAndUndoRotatesThePassword |
| R6.13 | an interrupted re-key is finished from the pending password | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword,TestKeychainAnchorCurrentA |
| R6.14 | a stale or junk pending password is discarded once the current one opens the file | KILLED | TestKeychainAnchorCurrentAndPendingPasswordStateIsFailClosedForEveryCombination,TestKeychainAnchorFailedRekeyNeverAdvanc |
| R6.15 | a malformed pending item is junk, discarded and never a password | KILLED | TestKeychainAnchorCurrentAndPendingPasswordStateIsFailClosedForEveryCombination |
| R6.16 | neither password item present is a missing-password (corrupt) anchor | REDUNDANT-LAYER |  |
| R6.17 | a wrong current password with no usable pending one is a corrupt anchor, not unavailable | KILLED | TestKeychainAnchorCurrentAndPendingPasswordStateIsFailClosedForEveryCombination,TestKeychainAnchorOnlyTheGovernedResetRe |
| R6.18 | a promoted pending password is the password the next re-key starts from | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword |
| R6.19 | the pending password is checked against the file before it is promoted | KILLED | TestKeychainAnchorCurrentAndPendingPasswordStateIsFailClosedForEveryCombination |
| R6.20 | a missing re-key entry point refuses the re-key (no success without freshness) | KILLED | TestKeychainAnchorMissingRekeyEntryPointRefusesEveryAdvanceAndStillReads |
| R6.21 | a governed reset discards a pending password with the pair it belongs to | KILLED | TestKeychainAnchorOnlyTheGovernedResetRecoversAReplayedFileAndItManufacturesNothing |
| R6.22 | the re-key compares the earlier password it was given (a wrong old password is refused) | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword,TestKeychainAnchorCurrentA |
| R6.23 | an undo's re-key failure does not report success (Revert returns the state write result only) | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword,TestKeychainAnchorEveryAdv |
| R6.16j | joint: neither password item present is a corrupt anchor (missing-password check + unlock-failure classification) | KILLED | TestKeychainAnchorAWriteRightAfterAnInterruptedRekeyPromotesThenRekeysFromThePromotedPassword,TestKeychainAnchorCurrentA |
