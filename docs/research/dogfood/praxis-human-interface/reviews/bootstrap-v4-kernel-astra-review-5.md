REVISION_REQUIRED

# Praxis PRE-V4 Safety Kernel — independent Astra Review #5

Review date: 2026-09-21. Target: `praxis-human-interface/1`. Reviewer: this fresh Astra session. Implementer provenance: Claude Repair #4. This review did not modify implementation source, existing tests, or prior evidence.

## 1. Disposition

The frozen Repair #4 candidate is unsafe as the source of a clean build. Two independently executed production-path counterexamples establish that database-local authenticated state is not temporally authoritative:

- **N14:** replaying a copied, valid decision-liveness row after revocation restores executable authority across restart and causes the controller to mint a new sealed completion; all units then count complete.
- **N15:** a delete-only writer can erase the finite set of rows used to derive Goal classification, after which the supported import accepts stripped work and the controller invokes a worker with activation absent.

The smallest general defect class is **governance continuity without rollback- or erasure-resistant state**. Authentic bytes prove origin and integrity; they do not prove that the fact is still current, nor that an absent subject was never governed. I12 covers removal from a fixed row set, but it does not establish temporal freshness and the implementation does not satisfy I12 for combined deletion of all classification sources.

These are violations of already-authorized PRE-V4 properties, especially I3, I9, I10, I11 and I12. The review can determine that the candidate is unsafe without choosing the repair architecture, so the disposition is `REVISION_REQUIRED`, not `AUTHORITY_CONFLICT`.

## 2. Exact candidate and evidence identity

Base commit: `ff146000aadae0ef60981d445056089f52815869`. Candidate: `modified:true`, inactive and uninstalled.

| Item | Independently recomputed SHA-256 |
|---|---|
| Repair #4 implementation report | `b36079f346caf10ccce65095da9500fc88372fc0ed12d47cd9f5e891ffb1cfc6` |
| Unchanged Review #4 | `71ed5bc796749fe48f71f78f4cff637cc9cdd5293bc05d974ff589d97657c8a6` |
| Source manifest | `80812fc66afca4dcad00b7f84f7242e1947c08fa2b0558836ef22aa4b5db3e11` |
| Qualification results | `efa40e90b72b7e81872951d69d320aa1fb96d4340ebc71359e5f50d0d0a82e93` |
| Activation requirements | `5d25493151ce72e38488ab21998e98defdf8ae5a7de61131322a2d5c9059faea` |
| Pre-activation verification | `c6d4a7aa405cca5cc4170b9836e34e8b3bb529f33178adc0c3b490eaa643c7b1` |
| Candidate core | `b270805ecf6c59c9c4d6e0e8a72cbf9d3c49b0ce13b0372b26a042e17e45d972` |
| Goals package archive | `cc6d98db19f62687c4105984e1227093100c4f4f6eedf883ef2679035b72922d` |
| Package manifest | `e4bc2024e9817d11318d76d8d839fe25a2e1a4d2e77edbc73dca0e92bfb9c8b6` |
| Goals plugin | `5ec013468a9630aaa9d66d27cd9b70f3ae8e55be4a01cabc0be592d25c1118fa` |
| Active core, read-only observation | `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7` |

All 83 source-manifest entries matched their declared digests. The candidate activation record agrees with these identities and states `build_modified:true`. Active and candidate core identities remain distinct.

## 3. N14 — authentic stale liveness becomes current authority

Repair #4's A2 probe stops after `LoadAuthorityDecision` accepts a replayed liveness row. Review #5 extended the same attack through restart and the production consequence boundary.

The fixture used an authentic owner ceremony and decision, a real SQLite event ledger, the production GoalStore and gate controller, and an authenticated producer completion. While the decision was current, the adversary copied its sealed `authority_decision_live` row. The owner then revoked the decision. Controls proved that a fresh drive refused and the durable ledger still contained only the producer completion.

The keyless SQLite writer then deleted the revocation row and inserted the byte-exact earlier liveness row. It did not invoke the storage key, `Crypto`, or `KeyRef`. After database close/reopen, a fresh repository accepted the old decision and a fresh controller created a **new sealed gate completion**. Durable completions changed from one to two; both were effective; `AssessGoalCompletion` returned `AllUnitsComplete=true`.

This violates I3 directly: historical authority became current executable authority. It also violates I10 and I11 because an outward durable consequence and future eligibility were admitted by authentic but temporally stale lineage. A live provider is unnecessary; the new authenticated completion is itself the prohibited governed consequence.

## 4. N15 — combined deletion converts governed identity into legacy execution

Repair #4 claims A1 delete-only safety for combinations of deletions, but its sweeps cover single rows and selected pairs. `GoalSafetyKernel` derives classification only from three positive sources: the classification row, safety-bearing Goal baselines, and safety-bearing proposals.

Review #5 created the stripped import document before mutation, removed activation, and proved the import was refused while safety evidence survived. A keyless delete-only writer then removed:

1. the Goal's `goal_safety_classification` row;
2. the fixture's `work_plan_proposal` row;
3. the attached safety-bearing `goal_baseline` generation.

The plan-less predecessor and other authenticated governance records, including the accepted-plan record, remained. After close/reopen, the supported CLI accepted a stripped successor based on that predecessor. `GoalSafetyClassified` returned false. The production controller skipped the safety activation/current-authority path and invoked the worker once. Its deliberately empty response later caused `unsupported Goal turn outcome`; that post-dispatch error cannot retract provider or external effects.

This is achievable with the same delete-only database capability used by Review #4, without raw insertion, copied ciphertext, or the storage key. It violates I9, I10 and I12 and contradicts the implementation report's A1 claim. It also shows A3 is not limited to total database destruction: selective removal of the classification sources is enough while substantial authenticated state survives.

## 5. Independent disposition of A2 and A3

1. **Accuracy.** A2 is accurately described as replay of a copied valid sealed row, but its consequence is understated: it reaches a new authenticated completion and eligibility. A3 is directionally accurate but too narrow; selective erasure of the classification-source set is sufficient.
2. **Existing invariants.** A2 violates I3, I10 and I11. A3/N15 violates I9, I10 and I12. Neither requires inventing a new product requirement.
3. **Missing general invariant.** I12 is set-monotonic and does not cover time. The missing property is anti-rollback temporal authority: once authority or classification is retired, no earlier authentic state may become executable again. Subject governance also needs continuity: erasure of the current evidence set cannot make a previously governed subject indistinguishable from a never-governed subject.
4. **Adversary capability.** A2 uses ordinary read/write access to the same SQLite file already placed in scope; a raw SQLite writer capable of Review #4's deletes can copy and reinsert rows. N15 needs only delete access plus a supported import, so it is achievable without A2's replay capability.
5. **Proceeding with residuals.** The candidate cannot legitimately proceed. Both residuals permit new consequences that surviving current governance does not authorize.
6. **Architecture claim.** Robust resistance to arbitrary rollback or complete erasure of the entire database does require trusted freshness or subject-continuity state outside that rollback domain, or an equivalent external witness. That fact does not make the residual acceptable. N15's selective gap can also be narrowed inside the current store by consuming all surviving authenticated safety lineage, but no database-local scheme can distinguish a fully rolled-back/erased database from the historical state it exactly reproduces. Choosing the repair mechanism may require separate authority; accepting this candidate does not follow from that constraint.

## 6. Source and transaction-boundary assessment

The new liveness writes and retirements are transactionally paired at their intended API boundaries, and the root reader now lists generations and liveness records in one snapshot. That closes the reported split-read regression for the tested transition. It does not establish temporal currentness against replay because `requireLive` verifies only namespace, identity, version and digest. A replayed envelope satisfies all of those checks.

Classification fallback is fail-closed for unreadable surviving rows, but absence of the entire enumerated source set is interpreted as `classified=false`. The authenticated acceptance that survives N15 is not consulted. Thus two locally safe rules compose unsafely: “missing classification → derive” plus “no derivation source → legacy.”

## 7. Mutation inventory audit

The reported counts are internally consistent: 192 total, 182 single-guard, 10 joint, 169 killed and 23 classified survivors. The catalogue expressly excludes A2 rollback and A3 total erasure, so those counts cannot support accepting either residual. The survivor table includes 13 redundant layers, seven redundant-by-construction, two equivalent and one unmodeled Darwin guard; Review #5 found no need to overturn those classifications because N14/N15 are outside the enumerated universe and already determine disposition.

The claim that A1 combinations are covered is stronger than the mutation evidence. The preserved tests and mutation catalogue do not provide an exhaustive powerset deletion proof; N15 supplies a three-row counterexample beyond the single/pair sweeps.

## 8. Qualification audit

Primary evidence supports that the reported ordinary qualification ran: focused Go tests, `make test-current` (44 packages), race tests, `go vet`, Python (1,683 passed, one skipped), specification verification, prior-probe replays, rebuilds and `git diff --check` are recorded as passing. Review #5 independently replayed the preserved A2 probe and independently ran N14/N15. Green suites do not constrain the excluded residual class.

The base/candidate stale-attestation files are byte-identical, each 1,237 lines representing the same reported 1,236 stale entries, so historical conformance remains pre-existing red. Flaky measurement recorded one failure in 120 base runs and two in 120 candidate runs, using the known lease/succession-contender failure modes. The report also admits that an earlier Repair #4 measurement exposed an introduced split-snapshot liveness failure, but its intermediate log was overwritten. The final source contains the same-snapshot reader; the missing intermediate log weakens audit provenance but is not needed for this disposition.

## 9. N8–N13 and prior invariants

The preserved Review #4 attacks no longer reproduce by deletion of one row, and source inspection shows positive liveness checks at decision/generation consumers, publication and recovery. Those local fixes do not close the generalized boundary. N14 reopens the N8 consequence using authentic stale state; N15 reopens the N9 downgrade using combined deletion. Therefore Repair #4 cannot claim I3/I9/I10/I11/I12 as composed invariants even if its individual N8–N13 regression tests pass.

The complete requested search outside this blocker—additional publication races, every cache disagreement, all survivor joint mutations, and full requalification of older N1–N7 platform-specific paths—was stopped under the established blocking-defect discipline. No positive conclusion is implied for unreviewed areas.

## 10. Preserved Review #5 evidence

- `bootstrap-v4-kernel-astra-review-5-evidence/n14_rollback_consequence_test.go.txt` — SHA-256 `1f1f080789e48a9f31d9d8e2a67cf6007e168d59d7a3c3d4ce8f03c60d34b600`
- `bootstrap-v4-kernel-astra-review-5-evidence/overlay.json` — SHA-256 `9f7829a2c3da8f7ce3eab46e5abcdb25f03a4cfbffc05cf3a2615a31d210c217`
- `bootstrap-v4-kernel-astra-review-5-evidence/n14-rollback-consequence.log` — SHA-256 `16097829d04506e8c0933fb88d76063609bf0a78c1fb7810964f588654af6988`

The probe overlay adds only a scratch test path. No candidate implementation file or prior evidence file was edited.

## 11. Non-actions and transition status

Nothing was committed, pushed, installed, deployed or activated. Proposal v4 was not materialized or submitted. Gates A, B and C remain undecided. The candidate remains `modified:true`; the active core remains distinct. The clean-build and separately authorized activation sequence must not begin from this candidate.

The required change is to make governance continuity resistant to stale authenticated replay and to prevent erasure of the classification-source set from producing legacy permission. This report does not choose or implement that repair.
