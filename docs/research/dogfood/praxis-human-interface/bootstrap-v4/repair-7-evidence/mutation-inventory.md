# Repair 7 guard/mutation inventory

Generated from the final Repair 7 source (N17 fix + G1-G7 equivalent paths) by `build_inventory.py` over `results-r7-rest-final.json`, `results-r7-kc-final.json`. Scratch mutation copies live outside the repository; every mutated file was restored (RESTORE-CHECK OK in each run log).

| Measure | Count |
|---|---|
| mutations | 364 |
| single-guard | 341 |
| joint | 23 |
| killed by a relevant regression | 315 |
| redundant layer (joint proof killed) | 27 |
| redundant by construction | 17 |
| equivalent | 3 |
| unmodeled | 2 |
| suspect kills (killed only by unrelated timing tests) | 0 |
| **survived, unexplained** | **0** |

## Re-run and test-strengthening notes

- First-pass diagnostic runs (results-r7-new-pass1.json, results-r7-new-pass2.json, not part of the final counts) surfaced three real test gaps in the new resolver-selection code, closed before this final run: R7.09j (a generation whose Scope is the bare package identity, no 'package-namespace:' prefix, could resolve -- TestResolverRefusesAGenerationWhoseScopeIsNotThePackageNamespaceForm), R7.50 (BuildSigningPreview's own publisher-namespace guard was masked by an unrelated manifest-invocation mismatch in the first attempt at a test, not by a real gap; TestBuildSigningPreviewEnforcesThePublisherGenerationsOwnNamespace now isolates it with an internally consistent foreign-namespace manifest), and the expiry boundary now covered by TestSignWithPreviewRefusesAtTheExactExpiryBoundary / TestSignWithPreviewRefusesExpiredAuthorityBeforeSigning.
- R7.58 (SignWithPreview's own ExpiresAt check) is a genuine redundant layer, not a gap: an isolated fake-authorizer unit test was attempted to prove it in isolation from the goalstore resolver and abandoned as excessive (the AuthorityRequest/Decision validators it would need to satisfy are deep plumbing unrelated to the property under test); the masking chain is instead documented with the exact refusal text measured from the mutant run.
- Keychain-family mutations (40, R5.* and R6.*) ran with one worker, PRAXIS_REQUIRE_KEYCHAIN=1, user interaction disabled and unique service identities: no test was skipped. All 5 survivors are Repair 6 carried classifications; Repair 7 touches no Keychain code.
- Every REDUNDANT-LAYER classification among the new R7.* guards is backed by a killed joint mutation (R7.04j, R7.09j, R7.10j, R7.13j, R7.25j, R7.26j, R7.27j, R7.34j) or by a direct regression proving the underlying property (TestRevokedParentDecisionDoesNotResolvePackagePublishAuthority for R7.15; TestRetiredPackageManagerGenerationCannotBeBoundByADeploymentDecision for R7.35; TestSignWithPreviewRefusesAtTheExactExpiryBoundary for R7.58).

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
| N5.07 | export status clean | REDUNDANT-LAYER | export status check is covered by the content re-hash and the hint scan; joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 |
| N5.08 | export gained no untracked/excluded files | REDUNDANT-LAYER | untracked/excluded-file scan is covered by the content re-hash (which adds all files); joint N5.10 (status+hint+untracked+content) is killed and the content layer alone is killed by N5.09 |
| N5.11 | committed validator digest at checkpoint (bind) | REDUNDANT-LAYER | the committed-validator digest at the checkpoint is re-checked on the exported blob (N5.12) and the whole export tree is re-hashed; the two digest layers removed together are killed (N5.13) |
| N5.12 | exported validator digest | REDUNDANT-LAYER | the exported validator digest is re-checked on the committed blob (N5.11 killed) and the whole export tree is re-hashed after the run; joint N5.13 is killed |
| N5.14 | validator executable mode at checkpoint | REDUNDANT-BY-CONSTRUCTION | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) a validator that is not an executable regular file cannot be executed: the OS refuses the exec and the run fails closed. The explicit mode checks (bind N5.14, export N5.14x) only give an earlier, clearer message; even their joint mutation N5.14j survives because exec permission still refuses |
| N5.14x | exported validator executable mode | REDUNDANT-BY-CONSTRUCTION | see N5.14 |
| N5.14j | joint: validator executable mode (bind + export) | REDUNDANT-BY-CONSTRUCTION | see N5.14 |
| N5.15 | bind: HEAD is the checkpoint | REDUNDANT-LAYER | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) HEAD==checkpoint is asserted by four layers (bind, bind branch ref, ReadCheckpointArtifact, PushAndVerify); joint N5.18 of the bind/branch/push layers is killed |
| N5.16 | bind: local branch points at checkpoint | REDUNDANT-LAYER | see N5.15 (joint N5.18 killed) |
| N5.21 | publication pushes the exact qualified object id | EQUIVALENT | (its only kills were load-sensitive tests unrelated to the guard, so it is counted as a survivor) PushAndVerify has already proved HEAD==head, so pushing HEAD or the object id publishes the same commit |
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
| R7.03 | resolver: generation State is active | REDUNDANT-BY-CONSTRUCTION | a generation whose State is not "active" cannot carry a valid digest under Validate/VerifyDigest, so it can never be stored and reach the resolver at all (TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm/generation_state logs the unreachable digest error) |
| R7.04 | resolver: authority model version is the successor | REDUNDANT-LAYER | masked by the paired authority-model digest term (both must hold for a coherent, storable authority-model pair); joint R7.04j (both terms removed) is killed |
| R7.05 | resolver: authority model digest is the successor | REDUNDANT-LAYER | see R7.04 (joint R7.04j killed) |
| R7.09 | resolver: scope is a package-namespace scope | REDUNDANT-LAYER | masked by the canonical-scope-equality and namespace-membership terms for any well-formed scope; joint R7.09j (prefix + equality) is killed by the bare-package-identity-scope regression |
| R7.10 | resolver: the namespace scope is canonical | REDUNDANT-BY-CONSTRUCTION | when the trimmed namespace is malformed, PackagePublishScope returns an error, so scopeErr!=nil already refuses regardless of the forced equality; joint R7.10j (scopeErr + equality + membership) is killed |
| R7.11 | resolver: scope equals the canonical namespace scope | REDUNDANT-BY-CONSTRUCTION | for a well-formed scope, generation.Scope==want is tautological (want is computed FROM generation.Scope's own trimmed namespace); a malformed or wrong-namespace scope is caught by scopeErr or membership; joint R7.10j is killed |
| R7.13 | resolver: delegation reference is well formed | REDUNDANT-LAYER | masked by the request-load, decision-load and delegation-digest layers for any candidate the store will hold; joint R7.13j (all four) is killed |
| R7.14 | resolver: the delegation request loads | REDUNDANT-LAYER | see R7.13 (joint R7.13j killed) |
| R7.15 | resolver: the parent decision loads and is current | REDUNDANT-LAYER | see R7.13 (joint R7.13j killed); the property itself -- revoking the delegating decision refuses resolution -- is proven directly by TestRevokedParentDecisionDoesNotResolvePackagePublishAuthority |
| R7.16 | resolver: the request digest matches the decision | REDUNDANT-BY-CONSTRUCTION | the stored request cannot diverge from the decision's bound RequestDigest without the authenticated store itself being forged; see R7.13 |
| R7.17 | resolver: the generation names its delegation digest | REDUNDANT-BY-CONSTRUCTION | an empty DelegationDigest cannot occur on a generation that passed ComputeDigest/VerifyDigest, so it can never be stored (see the delegation-reference variants) |
| R7.18 | resolver: publisher and package identities are required | REDUNDANT-BY-CONSTRUCTION | an empty publisher-generation digest or package identity matches no stored generation's SubjectDigest/namespace regardless of this early guard (TestResolverDoesNotResolveForADifferentPublisherGeneration) |
| R7.21 | requireCurrentLineage refuses a cycle | REDUNDANT-BY-CONSTRUCTION | a cycle in the parent chain would require two generations whose digests each equal a function of the other's digest, which ComputeDigest's hash binding makes infeasible to construct |
| R7.25 | current listing drops invalidated generations | REDUNDANT-LAYER | the current-generation listing's invalidation filter is masked, for a resolved child, by requireCurrentLineage applied to that same child; joint R7.25j (listing filter + lineage) is killed |
| R7.26 | current listing requires the liveness record naming the digest | REDUNDANT-LAYER | see R7.25 (joint R7.26j killed) |
| R7.27 | current listing applies anchored retirement and re-anchor void | REDUNDANT-LAYER | see R7.25 (joint R7.27j killed) |
| R7.35 | package-deploy approval derivation requires a CURRENT operational generation | REDUNDANT-LAYER | DerivePackageDeploymentApproval calls validatePackageDeploymentDecision first (via SaveAuthorityDecision, already exercised), which applies requireCurrentLineage to the same operational generation; joint R7.34j (both call sites) is killed, and TestRetiredPackageManagerGenerationCannotBeBoundByADeploymentDecision kills the decision-time layer alone |
| R7.40 | owner OS-user authentication requires a root-shaped generation | REDUNDANT-BY-CONSTRUCTION | a delegated generation can never name the parent's own principal (delegation containment forbids self-delegation), so no child generation can pass the ParentRef=="" && Principal==owner test; TestOwnerOSUserAuthenticationUsesOnlyTheCurrentRoot records this directly (principal id and kind are required once a parent is set incorrectly, and containment refuses the well-formed attempt) |
| R7.58 | SignWithPreview refuses expired authority before signing | REDUNDANT-LAYER | masked by the goalstore resolver's own expiry enforcement: the delegating request/decision are stored with a matching secure-blob expiresAt, so ResolvePackagePublishAuthority already refuses ("no current bounded package.publish authority") once the delegation's ExpiresAt has passed, before SignWithPreview's own check is reached; TestSignWithPreviewRefusesAtTheExactExpiryBoundary and TestSignWithPreviewRefusesExpiredAuthorityBeforeSigning exercise the boundary end to end |

## N17 / equivalent-path (Repair 7) mutations (all 60)

| id | guard | verdict | killed by |
|---|---|---|---|
| R7.01 | package.publish resolver selects only CURRENT generations (N17 core) | KILLED | TestEveryImmutableGenerationConsumerIsClassified |
| R7.02 | package.publish resolver requires a current lineage (root superseded / ancestor retired) | KILLED | TestChildOfASupersededRootDoesNotResolvePackagePublishAuthority,TestResolverRejectsEveryCandidateThatViolatesOneSelectio |
| R7.03 | resolver: generation State is active | REDUNDANT-BY-CONSTRUCTION |  |
| R7.04 | resolver: authority model version is the successor | REDUNDANT-LAYER |  |
| R7.05 | resolver: authority model digest is the successor | REDUNDANT-LAYER |  |
| R7.06 | resolver: delegation profile is package.publish | KILLED | TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm |
| R7.07 | resolver: subject digest is the exact publisher generation | KILLED | TestResolverDoesNotResolveForADifferentPublisherGeneration,TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm |
| R7.08 | resolver: generation carries governed package.publish | KILLED | TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm |
| R7.09 | resolver: scope is a package-namespace scope | REDUNDANT-LAYER |  |
| R7.10 | resolver: the namespace scope is canonical | REDUNDANT-BY-CONSTRUCTION |  |
| R7.11 | resolver: scope equals the canonical namespace scope | REDUNDANT-BY-CONSTRUCTION |  |
| R7.12 | resolver: package identity is inside the namespace | KILLED | TestResolverDoesNotResolveForADifferentPublisherGeneration,TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm |
| R7.13 | resolver: delegation reference is well formed | REDUNDANT-LAYER |  |
| R7.14 | resolver: the delegation request loads | REDUNDANT-LAYER |  |
| R7.15 | resolver: the parent decision loads and is current | REDUNDANT-LAYER |  |
| R7.16 | resolver: the request digest matches the decision | REDUNDANT-BY-CONSTRUCTION |  |
| R7.17 | resolver: the generation names its delegation digest | REDUNDANT-BY-CONSTRUCTION |  |
| R7.18 | resolver: publisher and package identities are required | REDUNDANT-BY-CONSTRUCTION |  |
| R7.19 | requireCurrentGeneration refuses an invalidated generation | KILLED | TestChildOfASupersededRootDoesNotResolvePackagePublishAuthority,TestFAAMixedSnapshotAdversaryNeverRegainsRetiredAuthorit |
| R7.20 | requireCurrentGeneration requires the authenticated liveness record | KILLED | TestFAAEveryConsumerHonoursFactsAndFreshness,TestFAAGovernedReanchorAfterRestoreDoesNotResurrectRetiredAuthority,TestFAA |
| R7.21 | requireCurrentLineage refuses a cycle | REDUNDANT-BY-CONSTRUCTION |  |
| R7.22 | requireCurrentLineage applies currentness to every generation on the chain | KILLED | TestChildOfASupersededRootDoesNotResolvePackagePublishAuthority,TestPackageManagerGenerationOfARetiredRootDoesNotResolve |
| R7.23 | requireCurrentLineage binds the parent digest | KILLED | TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm |
| R7.24 | LoadCurrentAuthorityGeneration applies currentness | KILLED | TestLoadCurrentAuthorityGenerationRefusesARetiredGeneration |
| R7.25 | current listing drops invalidated generations | REDUNDANT-LAYER |  |
| R7.26 | current listing requires the liveness record naming the digest | REDUNDANT-LAYER |  |
| R7.27 | current listing applies anchored retirement and re-anchor void | REDUNDANT-LAYER |  |
| R7.28 | current listing refuses a store not current against the anchor | KILLED | TestCurrentGenerationListingRefusesAStoreBehindTheAnchor |
| R7.29 | ValidateAuthorityGeneration applies the shared currentness predicate | KILLED | TestEveryImmutableGenerationConsumerIsClassified,TestFAAEveryConsumerHonoursFactsAndFreshness,TestFAAGovernedReanchorAft |
| R7.30 | package-manager resolver selects only CURRENT generations | KILLED | TestEveryImmutableGenerationConsumerIsClassified |
| R7.31 | package-manager resolver requires a current lineage | KILLED | TestPackageManagerGenerationOfARetiredRootDoesNotResolveDeploymentAuthority |
| R7.32 | delegation from a parent (decision+generation entry point) requires a current parent | KILLED | TestEveryImmutableGenerationConsumerIsClassified,TestRetiredRootCannotMintADelegatedPublisherGeneration |
| R7.33 | delegation from a parent (generation entry point) requires a current parent | KILLED | TestEveryImmutableGenerationConsumerIsClassified |
| R7.34 | package-deploy decision binds a CURRENT operational generation | KILLED | TestEveryImmutableGenerationConsumerIsClassified,TestRetiredPackageManagerGenerationCannotBeBoundByADeploymentDecision |
| R7.35 | package-deploy approval derivation requires a CURRENT operational generation | REDUNDANT-LAYER |  |
| R7.36 | installation owner check requires the CURRENT installation root | KILLED | TestCurrentInstallationOwnerCheckRequiresTheCurrentRootOwnerAndOSUser,TestRetiredInstallationRootCannotApproveAPublisher |
| R7.37 | enrollment authenticates the enrolled OS user | KILLED | TestCurrentInstallationOwnerCheckRequiresTheCurrentRootOwnerAndOSUser |
| R7.38 | installation owner check requires the owner principal | KILLED | TestCurrentInstallationOwnerCheckRequiresTheCurrentRootOwnerAndOSUser |
| R7.39 | owner OS-user authentication (both abandonment paths) uses only CURRENT generations | KILLED | TestOwnerOSUserAuthenticationUsesOnlyTheCurrentRoot,TestRetiredRootShapedGenerationCannotAuthenticateTheAbandoningOwner |
| R7.40 | owner OS-user authentication requires a root-shaped generation | REDUNDANT-BY-CONSTRUCTION |  |
| R7.41 | re-anchor readmission refuses a root carrying an invalidation record | KILLED | TestReanchorReadmissionRefusesARootCarryingAnInvalidationRecord |
| R7.42 | the CLI delegation trusts only a CURRENT parent | KILLED | TestEveryImmutableGenerationConsumerIsClassified |
| R7.13j | joint: resolver refuses a candidate with no resolvable delegation (reference form + request + decision + digest layers) | KILLED | TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm,TestRevokedParentDecisionDoesNotResolvePackagePublishAutho |
| R7.01j | joint (N17 core): the resolver neither filters to current generations nor checks the lineage | KILLED | TestChildOfASupersededRootDoesNotResolvePackagePublishAuthority,TestDeletingTheInvalidationRowDoesNotReauthorizeSignWith |
| R7.25j | joint: the current listing drops invalidated generations AND the lineage applies currentness | KILLED | TestChildOfASupersededRootDoesNotResolvePackagePublishAuthority,TestPackageManagerGenerationOfARetiredRootDoesNotResolve |
| R7.26j | joint: the current listing requires the liveness record AND the lineage applies currentness | KILLED | TestChildOfASupersededRootDoesNotResolvePackagePublishAuthority,TestDeletingTheInvalidationRowDoesNotReauthorizeSignWith |
| R7.27j | joint: the current listing applies anchored retirement AND the lineage applies currentness | KILLED | TestChildOfASupersededRootDoesNotResolvePackagePublishAuthority,TestPackageManagerGenerationOfARetiredRootDoesNotResolve |
| R7.34j | joint: neither the deployment decision nor the approval derivation requires a CURRENT operational generation | KILLED | TestEveryImmutableGenerationConsumerIsClassified,TestRetiredPackageManagerGenerationCannotBeBoundByADeploymentDecision,T |
| R7.04j | joint: the resolver accepts a candidate under another authority model (version + digest terms) | KILLED | TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm |
| R7.09j | joint: the resolver accepts a scope that is not the canonical package-namespace scope (prefix check + canonical equality) | KILLED | TestResolverRefusesAGenerationWhoseScopeIsNotThePackageNamespaceForm |
| R7.10j | joint: the resolver accepts a scope whose namespace is not the requested package's (scopeErr + canonical equality + namespace membership) | KILLED | TestResolverDoesNotResolveForADifferentPublisherGeneration,TestResolverRejectsEveryCandidateThatViolatesOneSelectionTerm |
| R7.50 | BuildSigningPreview requires an active publisher generation for the package | KILLED | TestBuildSigningPreviewEnforcesThePublisherGenerationsOwnNamespace |
| R7.51 | BuildSigningPreview binds the authority key to the publisher key | KILLED | TestBuildSigningPreviewRefusesForeignNamespaceAndMismatchedAuthorityKey |
| R7.52 | SignWithPreview verifies the preview digest | KILLED | TestSignWithPreviewRefusesEachStaleBindingBeforeTheProtectedSigner |
| R7.53 | SignWithPreview refuses a preview stale for the package bytes | KILLED | TestSignWithPreviewRefusesEachStaleBindingBeforeTheProtectedSigner |
| R7.54 | SignWithPreview refuses a stale publisher generation | KILLED | TestSignWithPreviewRefusesEachStaleBindingBeforeTheProtectedSigner |
| R7.55 | SignWithPreview refuses a stale package.publish authority | KILLED | TestSignWithPreviewRefusesEachStaleBindingBeforeTheProtectedSigner |
| R7.56 | SignWithPreview refuses stale request/decision provenance | KILLED | TestSignWithPreviewRefusesEachStaleBindingBeforeTheProtectedSigner |
| R7.57 | SignWithPreview requires the protected signer to match the preview | KILLED | TestSignWithPreviewRefusesEachStaleBindingBeforeTheProtectedSigner |
| R7.58 | SignWithPreview refuses expired authority before signing | REDUNDANT-LAYER |  |
