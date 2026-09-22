REVISION_REQUIRED

# PRE-V4 Safety Kernel — independent Astra Review #4

Review date: 2026-09-21. Target: `praxis-human-interface/1`. This is the continuation of Astra Review #4 after Astra reached a model-capability boundary. Astra produced the identity audit and both isolated row-deletion reproductions. Daybreak continued the defensive security analysis from those exact preserved bytes. Reviewer provenance is distinct from Claude Repair #3; neither reviewer implemented or repaired candidate source.

Daybreak independently replayed both preserved overlay probes unchanged and reproduced both counterexamples. Its continuation note is preserved beside the Astra evidence as `bootstrap-v4-kernel-astra-review-4-evidence/daybreak-continuation.md`.

## 1. Disposition

The candidate is unsafe as the source for a clean `modified:false` build. Two keyless SQLite row deletions independently restore authority or remove safety classification and permit new governed consequences. The stop condition is met. Review objectives not needed to establish this disposition are explicitly incomplete below.

## 2. Exact candidate reviewed and verified hashes

Base commit: `ff146000aadae0ef60981d445056089f52815869`. Candidate: the uncommitted `modified:true` tree identified by 72-entry source manifest SHA-256 `fe8708766da7a35933ba3368ff0f68e730681b7db953204a600f50490495dbe1`; every entry independently matched.

| Evidence/artifact | SHA-256 |
|---|---|
| Frozen PRE-V4 plan | `2fd97a1001182d30e592d6650c52aae2704fa2f871bed532aca73bdc9a10ee9a` |
| Review #2 | `c2704e9d12ac6fd1efd515755a9afde2716fa3c19e1823e73b2095c171d2eb30` |
| Review #3 | `0ce799f512a1fac53499aa1c1f36bc4bd7349ab20ec3a1ba40fcc6114915fd92` (expected value matched) |
| Repair #3 report, actual filename `implementation-and-qualification-report.md` | `b44eb80ac7c20e44961ab917b6bdb5a931a86747132b3d86ab8266c7ff02463c` |
| Qualification results | `8f0b46e497ab8e923350263ef149f78e03ba9358ea6b3e477b0502aaf3e6a0c2` |
| Activation requirements | `ee4f982b626dd82d624309dbd9d37b48532a53eba6c094abe81d04098a437913` |
| Pre-activation verification | `3fdbd498ab3434a0f9166a62c222eb2440ace1c767c9fddfa98df0967dba6eb9` |
| Validation profile | `12d3f0a1e97a33591d1a320213ba55e9db008bc9d234297c3057b45bc2d1a55a` |
| Specification manifest | `5a33e46f15f59978ea72972a91879d3feff39355581fe48cb451d30c03a96215` |
| Canonical specification bundle (121 records independently checked) | `17e36c3351d9e510e710c74d3cf4170441f08c150a780e030dbed0896a3cf40a` |
| Candidate core | `663cf610f647aa2f00d071a75c2d8e53c9db9895f9563e5dda418e7ecd08aec1` |
| Goals package archive/content | `cc6d98db19f62687c4105984e1227093100c4f4f6eedf883ef2679035b72922d` |
| Package manifest | `e4bc2024e9817d11318d76d8d839fe25a2e1a4d2e77edbc73dca0e92bfb9c8b6` |
| Invocation-contract set, independently recomputed | `5acebd861e0d87f9ea230201a3136bbe2798424a4fc30ffff7c49a1fd4e2f043` |

The named `third-qualification-report.md` does not exist. File identity and contents establish `implementation-and-qualification-report.md` as the current third-repair report. Complete hashes of existing review/evidence files at review time are preserved in `bootstrap-v4-kernel-astra-review-4-evidence/existing-evidence-hashes.json`.

## 3. I1–I11 assessment

I3, I9, I10, and I11 fail at the durable-state integrity boundary. N8 turns historically revoked authority back into current authority and creates a new authenticated completion. N9 removes the authoritative classification on which downgrade and activation enforcement depend, then admits and executes stripped work. I1, I2, and I4–I8 were inspected through the frozen plan, prior reviews, Repair #3 report, and relevant source, but complete independent closure was stopped after the blockers. They are not cleared by this review.

## 4. N1–N7 closure assessment

The repaired mechanisms close the preserved N1–N7 counterexamples when their authenticated rows remain present. That closure is not durable: N9 reopens N1 by deleting the classification row, and N8 defeats the current-authority/completion semantics repaired for N2/N7 by deleting revocation. N3–N7 were not completely requalified independently after the blocking findings; no positive disposition is made for them.

## 5. N8 — deletion of revocation restores authority and creates a new completion

Severity: P1. Invariant violations: I3, I10, I11.

The real production repository, production gate coordinator, real SQLite event ledger, authentic owner decision and authentic completion seals were used. Before deletion, `LoadAuthorityDecision` returned `ErrAuthorityDecisionRevoked`; a fresh controller drive returned the same refusal; and the ledger held only the producer completion. The probe then executed exactly:

```sql
DELETE FROM secure_blobs
WHERE namespace='authority_revocation'
  AND object_id=? AND object_version=?;
```

It closed and reopened the database/repository, eliminating process-cache explanations. The original decision became current again. A fresh controller minted a **new sealed gate completion**. Durable completions changed from one to two, both resolved as authenticated/effective, and `AssessGoalCompletion` returned `AllUnitsComplete=true`.

The adversary never possessed or invoked the storage key. This is not fail-closed availability damage and is not merely the continued visibility of historical truth. Absence of the revocation is interpreted as affirmative current authority, allowing a new governed durable consequence.

Source cause: `LoadAuthorityDecision` treats `authority_revocation` not-found as unrevoked. Completion consumption does re-resolve current authority, but it revalidates against the deletion-altered absence fact.

## 6. N9 — deletion of classification removes the downgrade fence and reaches a worker

Severity: P1. Invariant violations: I2, I6, I9, I10.

With classification present and activation deliberately absent, the supported `goals-lifecycle --operation=import` path refused the stripped generation. The probe then executed exactly:

```sql
DELETE FROM secure_blobs
WHERE namespace='goal_safety_classification' AND object_id=?;
```

After close/reopen, the identical CLI import succeeded as authoritative. The persisted generation had `Safety=nil`, blank candidate kinds, no specification bindings and weakened provenance. `GoalSafetyClassified` reported false. A reloaded production controller dispatched the selected candidate to the worker with activation absent (`worker.calls=1`). The worker's deliberately empty result later produced `unsupported Goal turn outcome`; that later error does not undo provider/worker dispatch, which is the consequential boundary under test.

The source comment says classification is monotone and that safety semantics never depend solely on the admitted plan. SQLite deletion makes the authenticated fact absent, and every consumer treats absence as genuine legacy state. This directly reopens the N1 downgrade through the supported import path.

Source cause: `GoalSafetyKernel` maps a missing classification row to unclassified, after which `requireSafetyConsistent` permits a nil binding and the controller skips safety activation and governing-authority checks.

## 7. Downgrade/classification attacks

Astra traced removed, zeroed, partial, malformed and alternate safety metadata through the durable classification design and then attacked the authoritative record itself. N9 is sufficient: after deletion, a stripped plan passed direct supported CLI import and reached a worker after restart. Kernel-shaped content guards did not preserve classification because the reproduced stripped representation removes their mutable tells. Other classifications were searched in the source and evidence, but the stop condition prevented a complete inventory; that objective remains incomplete.

## 8. Completion and revocation attacks

The repaired seal checks refuse forged plaintext and modified completion bytes while governing records remain intact. N8 demonstrates the stronger consumption failure: authentic old decision plus deleted revocation becomes current, and the controller itself creates exact authenticated completion evidence after restart. Historical completion/current eligibility separation therefore depends on deletion-resistant negative state that the store does not provide.

## 9. Outward-effect attacks

N8 reaches gate-completion persistence and Goal eligibility. N9 reaches worker dispatch/provider consequence with activation absent. These are actual consequential boundaries, not helper-only results. Full checkpoint/package/publication inventory and state-change timing matrix were stopped after these P1 findings and remain incomplete.

## 10. Darwin process-image identity

Primary source and Repair #3 evidence were inspected. Independent Review #4 reproduction of the full A–E Darwin matrix was not completed after the stop condition. No Linux or Windows claim is made.

## 11. Strict JSON

Production wiring and the Repair #3 exact parser were inspected. A complete independent decoder-ingress attack matrix was not completed after the stop condition.

## 12. Git/checkpoint identity

Repair #3 source/evidence for checkpoint export and exact-object publication was inspected. Independent N5/TOCTOU reproduction was not completed after the stop condition.

## 13. Validator-output bound

Repair #3 collector source, evidence, and claimed process tests were inspected. Independent at-limit/overflow/process-tree matrix was not completed after the stop condition.

## 14. Path A composition

The permanent test crosses the described production lifecycle, repository, SQLite ledger, deterministic command worker, Git publication, completion authentication, restart and ceremony boundaries. N8/N9 expose a production persistence boundary omitted by Path A: deletion of absence-based authenticated facts. Therefore Path A's pass cannot establish durable I3/I9 safety.

## 15. Mutation inventory and B10.01/B10.02

The 155-entry catalogue and survivor classification were read. It explicitly excludes “deletion of authenticated rows by a raw writer,” the exact attack producing N8/N9. Thus the 134 killed mutations do not bear on these blockers. The evidence says B10.01/B10.02 were rerun with causal assertions after timing-sensitive false kills; Review #4 did not independently mutate them again after the stop condition and makes no new causal-kill certification.

## 16. Flaky test

The preserved measurement is 1/60 failures on untouched `ff14600` and 1/60 on the candidate for `TestConcurrentInstallationFanoutAcrossProcesses`, with the same failure text. This supports pre-existing nondeterminism and is unrelated to N8/N9. It was not used as kill evidence here.

## 17. Historical conformance

The preserved scans report identical sets: 1,236 stale immutable entries across 278 attestations at untouched `ff14600` and candidate. No historical attestation was changed. Review #4 found no evidence that N8/N9 alter that pre-existing-red classification.

## 18. Storage-key trust and SQLite deletion

The prior `ACCEPTABLE_BOOTSTRAP_TRUST_ASSUMPTION` for a storage-key holder remains unchanged. N8/N9 need no storage key. Treating arbitrary database-file write access as implicitly equal to the storage-key/ceremony trust root would nullify the stated purpose of authenticated records. More decisively, the candidate report itself asks Review #4 to determine whether deletion causes authority resurrection or safety downgrade; the real-path probes prove both. This residual is a consequential integrity failure, not an accepted cryptographic limitation.

## 19. Validation isolation

The disclosed non-hermetic validator/origin boundary was read. Its acceptability under frozen semantics was not finally adjudicated after the P1 stop condition.

## 20. Ceremony boundary

No weaker bypass around the accepted OS-user/install-root ceremony was needed. N8 resurrects a ceremony-authentic decision after its authentic revocation is deleted. The ceremony assumption remains as previously classified; durable revocation semantics do not.

## 21. Equivalent-path search

The search covered classification, supported import, controller reload, authority revocation, gate completion creation/consumption, restart, activation absence, and worker dispatch. It found N8 and N9. Other requested surfaces remain incomplete under the stop rule.

## 22. Qualification results

Independent Review #4 ran the two real-path overlay probes in `./cmd/praxis`; both passed as counterexample reproductions in 0.656 s. “PASS” means the asserted unsafe behavior occurred. Identity/source/specification verification passed as described in §2. Expensive broad qualification was not rerun because it cannot change the blocking disposition.

## 23. Active-versus-candidate state

Read-only SHA-256 of active `/Users/polliard/bin/praxis` is `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7`, distinct from candidate core `663cf610…08aec1`. Candidate reports `vcs.modified:true`. Nothing was installed, deployed, activated, committed, pushed, materialized or decided by this review.

## 24. Preserved reproduction evidence

- `bootstrap-v4-kernel-astra-review-4-evidence/row_deletion_test.go.txt` — SHA-256 `13ac96026628aedde7d5d822b4adcac236da58c5c8067bd83b9e9ca3e8b47f35`
- `bootstrap-v4-kernel-astra-review-4-evidence/overlay.json` — SHA-256 `2349751310f833289d440314b2ff5ae93dbfef536635b3eb35f48dba52820c68`
- `bootstrap-v4-kernel-astra-review-4-evidence/row-deletion.log` — SHA-256 `c5acbd3c599491bf42cd5b230e9115b3645e49f50c91a0a906ad42cc417409db`
- `bootstrap-v4-kernel-astra-review-4-evidence/identity-verification.json` — SHA-256 `20949c2b2976dba3211494ac1507cb63c437ee57298a38be796c3b0fb3fd0175`
- `bootstrap-v4-kernel-astra-review-4-evidence/initial-git-status.txt` — SHA-256 `e8c8a487e839fe9d258e481ff225d9e2dffe3598838b6ef3269e47cbb8445d69`
- `bootstrap-v4-kernel-astra-review-4-evidence/daybreak-continuation.md` — SHA-256 `e7619c7bfeaa6ac1c870ed4208abf345db67bb68c4e346fffdf6748ac8c90b98`

The overlay adds only a scratch test path; candidate implementation and existing evidence bytes were not edited.

## 25. Exact rationale and remaining transition

`REVISION_REQUIRED`. N8 restores revoked authority and causes a new authenticated completion/eligibility transition. N9 removes the authenticated classification fence, admits a stripped plan through the supported CLI, and reaches worker execution with activation absent. Both survive database close/reopen. Green qualification under intact rows does not constrain either attack, and the mutation inventory explicitly excluded them.

The sequence “commit reviewed source → clean rebuild → requalification → separately authorized activation” must not begin from this candidate. No transition or repair was performed.
