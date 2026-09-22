REVISION_REQUIRED

# Third complete independent PRE-V4 safety-kernel review

Review date: 2026-09-20. Target: `praxis-human-interface/1`. Reviewer: this Astra/Claude session, with three read-only forked reviewers covering (1) authority/ceremony/persistence, (2) goal-drive execution/validation/provider isolation, (3) activation/runtime identity/exact-JSON/specification integrity. Every blocking finding below was **re-run by the lead reviewer** from the preserved probe sources, not accepted from a fork's summary. Source under review: HEAD `ff146000aadae0ef60981d445056089f52815869` plus the uncommitted candidate identified by source-manifest SHA-256 `d130352ba8cf8244988e067c5d4d7bc13bafe79b76139cf772d67d3ffa403620` (49 files).

No repository source, test, or existing evidence file was modified. All probes ran through `go test -overlay` or scratch copies. Independent evidence is under `bootstrap-v4-kernel-astra-review-3-evidence/`.

**Input discrepancy.** The task names `bootstrap-v4/second-qualification-report.md`. That file does not exist. The authoritative current report is `bootstrap-v4/implementation-and-qualification-report.md`, which describes the second repair and was used in its place. No other input was missing.

## 1. Executive summary

The second repair is real and substantial. The eight ceremony-forgery, revocation, gate-dispatch, package-cache, malformed-JSON, and completed-flag defects of review 2 are closed at the specific counterexamples, and the gate-to-provider fence (I7) produced **no counterexample** under adversarial variation. All current qualification is green and the artifacts are byte-reproducible.

The candidate is nevertheless **not safe to commit as the rebuild source.** The prior reviews' pattern held again: every defect found this time sits where a repaired local mechanism meets a neighbouring boundary that keys on something the attacker can also change.

Six blocking findings, each with a preserved reproduction:

| # | Sev | Finding |
|---|---|---|
| N1 | P1 | Every safety fence keys on `WorkPlan.Safety != nil`. That pointer lives inside the plan bytes that `Save`/`Import` accept. A v4 plan with `Safety` stripped persists through the shared `Save` guard and the supported CLI `import`, with no activation manifest and fabricated authority lineage. B7's "one guard closes them all" is false. |
| N2 | P1 | A completed authority gate is never re-verified against its own decision. `VerifyPlanCompletions` checks flags and a public spec digest only. The completion ledger is an unauthenticated plaintext event store. A forged completion, or a revoked gate decision, still counts as a completed gate. This is the unrepaired half of B9 ("applicable gate authority") and contradicts frozen plan §3. |
| N3 | P2 | B11 does not detect an in-place overwrite of the running executable on darwin. The regression covers only rename replacement, and a code comment asserting the OS/descriptor prevents in-place rewrite is false on the qualified platform. |
| N4 | P2 | B13's duplicate-key check is case-sensitive; Go's decoder is not. `{"Status":"undecided","status":"approved"}` is accepted and decodes as `approved`. |
| N5 | P2 | Validation binding is defeated by `git update-index --assume-unchanged`/`--skip-worktree`: a checkpoint whose committed content fails the validator qualifies and is published. |
| N6 | P2 | The one-MiB validation-output bound is not enforced (promoted `bytes.Buffer.ReadFrom` bypasses the guarded `Write`); 2,000,000 bytes with exit 0 is treated as PASS. The test that "kills" this mutation calls `Write` directly, never through a process. |

Both open questions resolve favourably to the implementer's stated positions with corrections: the ceremony trust limit is **A (ACCEPTABLE_BOOTSTRAP_TRUST_ASSUMPTION)**, though the report understates it (see N2), and Path A composition is **B (REQUIRES_SINGLE_END_TO_END_TEST)** as a regression to add, not because a defect survives in it today.

## 2. Independent evidence-integrity results

Recomputed with `shasum -a 256` on the current tree; all match the implementation report.

| Item | SHA-256 | Match |
|---|---|---|
| Frozen v4 plan `plans/workplan-v4-codex.md` | `2fd97a1001182d30e592d6650c52aae2704fa2f871bed532aca73bdc9a10ee9a` | yes (same as review 2) |
| Source manifest | `d130352ba8cf8244988e067c5d4d7bc13bafe79b76139cf772d67d3ffa403620` | yes |
| Qualification results | `c5217905ea6313293dbfd742284182141cb5d3677d83848102eb12de0d1dc6b3` | yes |
| Candidate activation requirements | `3244a6abe435ba2b61e9f6d5a70e9853f2d4786d453724c1afcc1f6df8576982` | yes |
| Pre-activation verification | `8e7f718bff1d1c98ed655b3165d5508ea34138d6dc5244db3c47afcb50ef63e9` | yes |
| Specification manifest file | `5a33e46f15f59978ea72972a91879d3feff39355581fe48cb451d30c03a96215` | yes |
| Validation profile description | `12d3f0a1e97a33591d1a320213ba55e9db008bc9d234297c3057b45bc2d1a55a` | yes |
| `.praxis/validate` | `5c7f1924c19784d966f94d386ddcbd67ace8adcdf5f4f94f9b0020a33bea8f0c` | yes |
| Core candidate `artifacts/praxis-candidate` | `5e617aab7343e015f4f0ea43cf6ed107e46187cf0ea94e3c8e550886d3525129` | yes |
| **Independent `go build ./cmd/praxis`** | `5e617aab7343e015f4f0ea43cf6ed107e46187cf0ea94e3c8e550886d3525129` | **byte-identical** (`go1.27.1`, darwin/arm64, rev `ff146000…`, `vcs.modified=true`) |
| Plugin executable | `1def4aac85b508502489077957a4811e2e6eecd863f51db61f39612279763c50` | yes; fork 3 also rebuilt it independently and extracted it from the archive |
| Package archive / manifest | `72f09a85…28d63` / `8b71a815…0e6` | yes |
| Review 1 / review 2 | `4857c107…6a4a` / `c2704e9d…0b30` | yes, unchanged |
| Installed core `~/bin/praxis` (read-only) | `807359e48b2626abb1bcfca3ec302da743f1a0238644d6b0c64a0f888ec48fe7` | unchanged; nothing installed |

- **Source manifest completeness.** All 49 manifest entries hash correctly, and every changed or untracked non-evidence file in `git status -uall` is in the manifest (none missing). The `.gitignore` change un-ignores `.praxis/validate`; `.praxis/bootstrap/` stays ignored and no code, test, or Make target references it, so a clean checkout is sufficient.
- **Specification bundle.** Go verifier and an independent Python recomputation (fork 3) both give canonical digest `17e36c3351d9e510e710c74d3cf4170441f08c150a780e030dbed0896a3cf40a`: 22 candidates, 31 requirements, 68 relationships, unchanged.
- **Pre-activation verifier** re-run from a scratch copy produced a byte-identical result to the preserved JSON. Its claims (identities coherent) are true; note it certifies coherence of digests, not the safety properties.
- **Qualification-results ↔ logs.** The claims correspond to the raw logs in `repair-2-evidence/`; the 13 `KILLED` lines match the 13 mutations claimed (but see §15 for what independent mutation replay showed).
- **Report accuracy.** Two statements are inaccurate (see §16, §20): the historical-conformance explanation, and "one guard closes them all"/"every protected boundary" wording.

## 3. Assessment of the eight invariants

All eight are **faithful to the frozen v4 semantics** and none broadens product authority: I3 narrows executable authority to *current* authority, I7/I8 restate plan §3/§9, and supervision refusal for safety turns is the plan's C6. The implementation, however, does not enforce them at boundaries that are actually shared.

| Inv. | Faithful to frozen v4? | Shared enforcement? | Verdict |
|---|---|---|---|
| I1 execution-derived state ≠ intent | yes | Accepted-plan flag: closed at contracts boundary. Ledger completions: **not authenticated at consumption** | **Not enforced for gates (N2)**; accepted-plan route holds |
| I2 equivalent ingress, equivalent enforcement | yes | `Save` guard is keyed on an attacker-editable pointer | **Fails (N1)** |
| I3 historical ≠ current authority | yes | Plan-level authority re-verified at prepare/settle/gate-reconcile. Gate *decisions* and mid-turn publication are not | **Partial (N2, publication window)** |
| I4 missing evidence ≠ success | yes | Missing/non-exec/changed validator fail closed at all stages | Holds for the stated cases; bound is defeated by N5/N6 |
| I5 exact, unambiguous, bound to executed | yes | Case-fold duplicates (N4); worktree divergence hidden (N5); output bound unenforced (N6); integrated-run output digest not stored | **Partial** |
| I6 verify the acting runtime | yes | No caches; package re-resolved at every persistence guard and drive turn. Image: in-place overwrite undetected on darwin (N3); exec→init window; publication not re-verified | **Partial (N3)** |
| I7 gates never reach a provider | yes | `Controller.worker` is the single provider-resolution point and carries the fence; `prepare` classifies first | **Holds — no counterexample** (safety and non-safety plans, 8+ objective spellings, disguised candidates: provider calls always 0) |
| I8 authentic ceremony lineage | yes, matches plan §9 and its stated non-goal | Record/decision/request/root/OS-user matrix refused field by field; delegation refuses protected requests | **Holds under the accepted trust model** |

**The set is not complete.** Three properties the findings expose are absent from the eight:

- **I9 — Downgrade resistance.** Whether a plan is safety-bearing must not be a field an unauthenticated writer controls. A generation that was created as safety-bearing must not be replaceable by one that is not. (N1)
- **I10 — Outward effects are guarded by the same predicates as recorded effects.** A checkpoint push is an outward consequence; activation and authority are currently re-verified when the completion is *recorded*, not before the push. (Fork findings R3-A3/R1-4, non-blocking, plus N5.)
- **I11 — Completion evidence is authenticated at consumption.** Derived-from-ledger is only as strong as the ledger. (N2)

## 4. B1–B15 assessment

| Finding | Independent assessment |
|---|---|
| B1 scope | Holds. Confirmed by the composed lifecycle→ceremony→gate tests and probes. |
| B2 continuation activation | Holds for the enumerated lifecycle routes; broader "all mutations" claim is where N1 lives. |
| B3 provenance | Holds; no additional laundering pairing found. |
| B4 idempotent gate request | Holds serially; concurrent initial creation still unestablished (report discloses this). |
| B5 completed-flag substitution | **Accepted-plan route repaired**: `TestReview2CompletedFlagSubstitution` now fails at validation, and mutations of the check are killed (M1a–c). The same effect through the completion ledger remains (N2). |
| B6 native gate wiring | **Holds.** `buildGoalDriveRuntime` now uses `openGovernedRepository`/`newGoalDriveController`; every other non-test `Repository`/`Controller` constructor carries no verifier and fails closed. Test fixtures still hand-build the repository (non-blocking, §20). |
| B7 import/Save bypass | **Not repaired at the shared boundary (N1).** Refused when `Safety != nil`; bypassed by clearing it. |
| B8 ceremony bypass | **Holds against API forgery.** Copied identifiers, arbitrary/all-`f` digest, other request/outcome/alternative, wrong root version, foreign OS user, foreign owner, legacy acceptance, and delegation of a protected request are all refused. Trust boundary addressed in §5. |
| B9 stale governing authority | **Plan-level repaired**: root invalidation and decision revocation refuse drive; history stays readable. **Gate-decision level not repaired (N2)**: a revoked gate decision still counts as a completed gate (`AllUnitsComplete=true`, no authority error). Review 2 explicitly asked for "applicable gate authority". |
| B10 validation | Deleted/non-executable/changed validator, timeout, non-zero exit, symlinked entrypoint, moved HEAD, plainly dirty tree: fail closed. **Bound defeated by N5**; output bound unenforced (N6); surviving descendants after a successful exit are not reaped (`sleep 40` survived) and the frozen-plan §4.3 "timeout with surviving child" negative test does not exist. |
| B11 running image | **Weaker than claimed on darwin (N3).** |
| B12 package identity | **Holds.** No cache, `sync.Once`, or memo; every guard re-reads manifest, package state, and image. The one gap is publication (non-blocking). |
| B13 exact JSON | **Partial (N4).** Trailing value/garbage, exact and `\u`-escaped duplicates (also nested), unknown fields (where requested), BOM, empty, >1 MiB, `1e999`, `1.5→int` all refused. Case-folded duplicates accepted; invalid UTF-8 silently replaced (digest still binds bytes). |
| B14 gate objective to worker | **Holds. No counterexample.** |
| B15 gate replay without activation | From review-2 history: `ReconcileAuthorityGate` replayed an existing request with no activation check. The preserved probe `TestReview2GateReplayWithoutActivation` now fails closed inside `authority_gate.go` (the guard at line ~22 of the shared boundary), and the regression `TestKernelRepair2B15…` exists. Verified at the shared boundary. |

Preserved review-2 probe replay (mine, using the implementer's as-replayed copies; diff to the originals reviewed): **13 adverse probes no longer reproduce; positive control `TestReview2GateRequestConflictPreservesOriginal` passes.** Caveat: four of the thirteen (`ProductionGateStoreConfiguration`, `GateReplayWithoutActivation`, `CompletionWithoutMechanismPass`, `ExplicitGateObjectiveReachesWorker`) now fail earlier, for "no verifier configured" reasons, and so do not by themselves demonstrate closure of the underlying defect. The closure evidence for those is the newer regressions and the independent probes below, and they were adequate for B14 and B15. The as-replayed adaptations changed only the verifier construction and one argument.

## 5. Ceremony trust boundary — disposition A (ACCEPTABLE_BOOTSTRAP_TRUST_ASSUMPTION)

**What possession of the installation storage key (or a `Repository` handle carrying it) permits**, demonstrated by probe R1-F: a caller supplies only public fields, builds a syntactically matching `OwnerCeremonyEvidence` for the enrolled OS user, calls the exported `SaveOwnerCeremony`, then `SaveAuthorityDecision`, and a gate is approved with no TTY and no typed confirmation. The same holder can bypass both methods with `Store.PutSecureBlob` and `Crypto.Seal` into any namespace, and can therefore forge roots, requests, decisions, and ceremony records equally. The ceremony record adds protection only against code paths that reach public persistence APIs *without* the key.

**Why A.** The frozen plan claims exactly what the mechanism delivers and expressly disclaims more:

- §9(4): "`SaveAuthorityDecision` rejects a protected request lacking the required ceremony evidence/profile, even if all public decision fields were copied correctly." The implementation meets this: copied fields and a fabricated digest are refused.
- §9: "The current OS-user ceremony is the existing authentication root. V4 does not claim stronger cryptographic human authentication than current Praxis provides."
- Open issue 4: the ceremony evidence "must bind TTY/OS-owner checks without pretending those checks are cryptographic personhood."

**Assumption accepted under A:** protected authority is as trustworthy as (a) a same-user process's access to the keychain-held installation key and (b) the TTY plus authenticated-OS-user check (which accepts any character device, so a pty satisfies it; this is the pre-existing authentication root, not a regression). An out-of-process attestation key or hardware confirmation would be new authority semantics; it is not required by the frozen claims and I do not invent it.

**Two corrections to the report's statement of this limit, neither changing A:**

1. The report says the record is "only as trustworthy as the installation's storage key". That is accurate for ceremony *records* but **understates the effective gate boundary**: gate completion is trusted from a plaintext ledger event that needs no key (N2). Once N2 is repaired, the storage key is the boundary the report describes.
2. Ceremony evidence does not bind `DecisionRef`, `ExpiresAt`, `IssuedAt`, or `AuthorityDigest`, and nothing consumes `ConfirmationDigest`/`ConfirmedAt`; the typed confirmation is derivable from public data. Non-blocking under A, worth recording.

A is therefore conditional on N2 being repaired, or on a human decision (C) that the completion ledger is same-user-trusted. The frozen plan (§3, "mismatched decision digests cannot complete the gate") does not support that reading.

## 6. Path A composition — disposition B (REQUIRES_SINGLE_END_TO_END_TEST)

The shipped evidence is two halves joined at a hand-built attached baseline. `cmd/praxis` never runs a worker turn; the `goaldrive` half uses stubbed activation and authority verifiers and a fixture baseline. So the production authority verifier never runs after a worker turn, and `Runtime.Execute` over a real SQLite ledger with a safety baseline is not exercised end to end. That is an untested seam where the earlier defect classes (verifier wiring differing between lifecycle and drive; fixture baseline differing from attach-produced baseline) could survive.

I closed the seam with one deterministic run using no live provider, `TestR2EndToEndPathAThenGate` (preserved): real lifecycle → attach → `buildGoalDriveRuntime` → real SQLite ledger, GoalStore, and activation verifiers → real bare git remote → scripted `CommandWorker` → bound integrated and conformance validation → publish → qualified completion with dossier capture → fresh runtime → pending gate → real owner ceremony → gate completion. It passed first time; **no composition defect exists today.** The disposition is B because that test should be added to the repository as a regression, not because a live provider is needed. Note it passes only with a modified copy of the repair harness helper (also preserved), which the maintainers should fold in properly. Its passing does not bear on N1–N6, which are all outside its path.

## 7. Authority and bypass assessment

Holds: legacy acceptance of safety plans, delegated protected decisions, forged or mismatched ceremony records, and revoked plan-level authority. Fails: N1 (downgrade via `Safety=nil`), N2 (gate completion authority). Non-blocking: an accepted plan for goal A can be attached to a different Goal ID with identical content, because the baseline digest omits the Goal ID and attach does not compare `request.BaselineID`/`proposal.GoalID` to the target (probe R1-G, P2, non-blocking because the same owner-approved plan is attached).

**Nuance on N1, stated so its severity is not overread.** The stripped plan has blank candidate kinds and no `Safety`, so it is semantically a *legacy* plan and its "gate" is an ordinary candidate; the probe's worker call is therefore ordinary work on an ordinary plan. The defect is not that a gate reached a provider in the kernel's sense; it is that a v4-shaped, owner-approved plan can be **replaced, without activation, authority lineage, or ceremony, by a downgrade** that opts out of every kernel fence, via the shared `Save` guard and the supported `goals-lifecycle --operation=import`. The precondition is the same as review 2's B7 (Repository handle or CLI), which was treated as blocking. It is blocking here on the same basis, and the frozen plan's requirement that a plan enter only via proposal→…→attach is not enforced for `Safety == nil`.

## 8. Activation and runtime-identity assessment

- Manifest bytes, path, package identity, and plan bindings are verified per boundary with no caching (B12 verified).
- **N3 (P2):** two builds A and B with identical VCS identity and equal size, using the real `VerifyProcessImage`:
  - atomic rename, unlink+recreate, and symlink retarget: refused;
  - identical bytes at a new inode: refused (availability false positive; acceptable under "replace, then restart");
  - hardlink and symlink launches: accepted correctly;
  - **in-place truncate+write and in-place same-size overwrite of the running executable with B's bytes, manifest binding B: verification returns `nil` while the process is still executing A (its marker prints A).** macOS does not return ETXTBSY; the inode is unchanged so `os.SameFile` is true and the descriptor re-read hashes the new bytes. The source comment claiming the OS refuses this or the descriptor re-read catches it is true on Linux and false on darwin.
  - exec→init window: demonstrated (process running A verifies as B). The report discloses this honestly.
- **The claim actually delivered** is: "the bytes currently readable through the inode that the executable path resolved to at `bootstrapv4` package initialization hash to D, and the manifest path currently names that inode." It is **not** "the executing code hashes to D" on darwin. Review 2 required that a weaker claim not be labelled exact running-image verification; the report's B11 section states the exec→init limit but not the in-place hole, and the code comment contradicts reality. An operator `cp`-installing over a live binary triggers it without any attacker.
- The regression and its mutation ("same-file check KILLED") cover rename replacement only.
- Residual, unchanged and disclosed: manifest `build_modified:true` is accepted if the binary agrees, `source_tree_digest` and `qualification_evidence_digest` are format-checked only, and installed executable bytes are not re-hashed by the activation verifier. Clean-build is procedural (see §18).

## 9. Specification and evidence-integrity assessment

Bundle, manifest, requirement records, and artifact identities all recompute (§2). Parsing: see N4. Specification parsers (candidate, relationship, gate contract, governed outputs) still allow unknown fields; dossier, ceremony record, and activation manifest refuse them, matching the report's stated scope. The activation-manifest struct also cannot carry frozen-plan step-9 fields (installation root, activation time, receipt digests, compatibility fence, predecessor recovery identity) because unknown fields are refused; the final manifest author must match the struct exactly (non-blocking).

## 10. Validation and proof assessment

Fails closed: deleted, non-executable, or changed validator; timeout (killed at 1.0 s); descendant holding stdout (WaitDelay expiry at 5 s); non-zero exit; symlinked entrypoint; moved HEAD; ordinary dirty tree.

- **N5.** `bindValidationExecution` trusts `git status`. A worker commits `flag=bad`, writes `flag=ok` in the working tree, and marks it `--assume-unchanged` (or `--skip-worktree`); the checkpoint qualifies and is published although `HEAD:flag="bad"` fails the validator (real bare remote, real controller; remote HEAD moved). A `.git/info/exclude`d untracked file is likewise invisible. `core.fsmonitor` and gitignored-file variants were not probed but are the same class. The checkout is worker-writable, so this is inside the threat model.
- **N6.** `limitedValidationOutput` embeds `bytes.Buffer`; `os/exec`'s `io.Copy` uses the promoted `ReadFrom`, bypassing the size-checking `Write`. Probe: 2,000,000 bytes and exit 0 returned `err=nil, outlen=2000000`. Frozen plan §4.1/§4.3 make truncation a failure. `TestValidationOutputIsBounded` calls `Write` directly, so the mutation is "killed" only by a test that cannot see the defect. A descendant that redirects its file descriptors also survives a successful return.
- Non-blocking: the integrated-run output digest is not stored (only predicate outputs are); the `.praxis/validate` digest freezes the entrypoint only, and `make test-current`, the Makefile, and the tests remain worker-editable (also noted in review 2 and now omitted from the report's "not closed" list); `PushAndVerify` pushes `HEAD` with no re-bind between qualification and push.

## 11. Restart and recovery assessment

Restart, recovery, and pinned/replayed objectives all pass through `prepare` and re-verify activation and plan-level governing authority; the ledger is the only completion source. Cross-generation completion replay is rejected. Worker-supplied `CheckpointEvidence` is not exploitable because a safety turn reaches `Progress` only after the controller's own bound validation. Gaps: N2 (gate completion survives restart without decision re-resolution), and an in-flight turn's publication is not re-verified (a checkpoint pushed after revocation or activation drift during a long turn; on a claim turn the push happens and then `settleCompletion` refuses and returns an empty record, leaving a pushed checkpoint with no durable turn record). Also non-blocking: an explicit/recovery objective bypasses hard-dependency readiness; a pending gate halts the drive even when independent ready siblings exist (fail-closed, and plan §3 says siblings "may" run).

## 12. Provider and gate isolation assessment

**Holds.** `Controller.worker` (`controller.go`) is the only place a provider is resolved and calls `refuseGateDispatch` first; its callers are `invoke`, `invokeRepositoryTurn`, and `prepare`; `Runtime` seeds `ChildObjective` only from the ledger-recorded recovery objective. Probes covered safety and non-safety plans, objective spellings `g`, ` g`, `g `, `G`, `g\n`, `\tg`, `g\x00`, `goal/1/g`, and caller-supplied candidate slices disguising the gate as ordinary; provider calls were always zero. A gate candidate in a non-safety plan goes to gate coordination (which then refuses) rather than to a provider.

Two caveats that do not break the I7 claim: N1 lets a plan be downgraded to one without gate candidates at all; N2 lets the gate be marked done without a decision, after which its dependents proceed to a provider (`worker.calls=1`, `gate-coordinator.calls=0` in the unit probe).

## 13. Independent equivalent-path inventory

| Surface | Writers / paths found | Reaches shared enforcement? |
|---|---|---|
| Accepted WorkPlan persistence | legacy `SaveAcceptedWorkPlan`; authority-backed `…FromAuthorityDecision`; `AttachAcceptedWorkPlan` | Yes; legacy refused for safety plans |
| WorkPlan attachment | `AttachAcceptedWorkPlan` only | Yes; but does not bind Goal ID (non-blocking) |
| Baseline import/save/finalize | `Save` (used by `Import`, `Finalize`, `goal_establish.go`), attach, `SaveReplanningSuccessor` | **Only when `Safety != nil` (N1)** |
| AuthorityDecision persistence | `SaveAuthorityDecision`; delegation variants | Yes |
| Ceremony persistence | `SaveOwnerCeremony` (sole production caller `authority_decide.go`) | Yes; key/handle holder can call it (accepted, A) |
| Completion creation/overlay | `settleCompletion`, `coordinateGate`, `MaterializeTurnCompletion`, `RecordCompletion` (raw ledger), `ApplyCompletions`, work-set view, assessment | Readers pass `VerifyPlanCompletions`/`ApplyCompletions`; **ledger events are unauthenticated (N2)** |
| Provider dispatch | `worker` ← `invoke`, `invokeRepositoryTurn`, `prepare` | Yes, single fence |
| Explicit/recovery/replay objectives | `prepare` classification | Yes (readiness bypass noted) |
| Activation construction | `newGovernedRepository`, `newGoalDriveController`; other constructors carry no verifier | Yes; fail closed |
| Process-image verification | `bootstrapv4.verifyProcessImage` over init-time descriptor | Partial (N3, exec→init) |
| Package identity resolution | `lifecycleSafetyActivation` per call | Yes; no caches |
| Validator/profile resolution | `RunBoundValidation`, `deriveRepositoryOutcome`, `qualifySafetyUnit`, `VerifyValidationBinding` | Partial (N5, N6) |
| Authority revocation | `LoadAuthorityDecision`, `VerifyGoverningAuthority` | Plan-level yes; **gate-level no (N2)** |
| Restart/recovery | all via `prepare` | Yes, subject to N2 |
| Safety-bearing JSON decoding | `UnmarshalExactJSON` for dossier/gate/output/candidate/relationship/manifest/ceremony | Partial (N4); ledger/event payloads plain `json.Unmarshal`, unauthenticated |
| Publication (outward effect) | `PublishCheckpoint`/`PushAndVerify` | **No re-verification before push (I10)** |

## 14. Newly discovered findings

N1–N6 above are new relative to the implementer's inventory and to B1–B15. Non-blocking new findings: cross-Goal attach (R1-G); publication not re-verified (I10); pending gate blocks independent siblings; explicit objective skips hard-dependency readiness; ceremony record leaves four decision fields unbound; integrated-run output digest not stored; test fixtures hand-build the `Repository` instead of using `newGovernedRepository` (and omit `AuthorityGeneration`), the same class of test/production drift as B6, with the keychain-backed production constructor untested.

## 15. Qualification results

All run by me on the frozen source; nothing modified.

| Check | Result |
|---|---|
| `make test-current` | PASS, 44 packages `ok`, exit 0 |
| `go test -race -count=1` on contracts, bootstrapv4, goaldrive, goalstore, goals package, cmd/praxis | PASS, all six |
| `go vet ./...` | PASS, exit 0 |
| Python suite (bytecode/cache disabled) | 1,683 passed, 1 skipped |
| Specification verifier (Go) and independent Python recomputation | PASS, digest `17e36c33…`, 22/31/68 |
| Pre-activation verifier (scratch copy) | PASS, byte-identical to preserved JSON |
| Independent core rebuild | byte-identical |
| Preserved review-2 probe replay | 13 no longer reproduce; positive control passes (§4) |
| Actual A/B process-image probes | rename/unlink/symlink refused; **in-place overwrite accepted (N3)** |
| Independent adverse probes (R1/R2/R3 sets) | N1, N2, N3, N4, N5, N6 reproduced; B8/B9-plan/B14 refusals confirmed |
| Mutation replay (independent, scratch copy, 21 mutations) | Killed: B5 ×3, B9 controller, B9 settle-adjacent (M3), B14 ×2, B10 ×4. **Survived with no test:** settle-time authority re-verify, pre-gate-completion re-verify, settle-time activation re-verify, settle-time binding re-check, and a guard against claiming a non-selected unit. Survived because a second layer still refuses: four. So the after-worker re-check calls (TOCTOU half of I3/I6) have no regression coverage, and the implementer's "13 of 13" describes their selected set, not the guard inventory. |
| `git diff --check` | PASS |
| Historical `internal/conformance` | FAIL, classified in §16 |
| Final repo status | unchanged tracked/untracked source set; only this review and its evidence directory added |

## 16. Historical-conformance classification

Verified rather than assumed: I enumerated every `ExecutionAttestation.SourceDigests` entry against the recomputed `SourceSetDigest` for **an untouched extract of HEAD `ff14600` and for the candidate working tree** (scratch copies, scanner preserved). Both trees show the **identical** stale set: 1,236 stale source entries across 278 attestations, in particular including `cmd/praxis`, `internal/goaldrive`, `internal/goalstore`, `pkg/contracts`, and `packages/goals`. The suite stops at whichever stale attestation is reached first, and that varies per run (the base failed on `internal/state`, the candidate run on `internal/agent`).

Classification: **PRE_EXISTING_RED, not introduced by this candidate.** The report's explanation is inaccurate in one respect: it says three failing packages predate the work and `internal/goalstore` is "legitimately changed"; in fact the goalstore, goaldrive, contracts, cmd/praxis, and goals-package attestations were already stale at base. The conclusion (not a regression, nothing rewritten, no whole-system conformance claim) stands. No attestation was edited.

## 17. Scope assessment

The candidate remains a bounded PRE-V4 safety kernel. Changes are confined to contracts, activation, selection, authority/ceremony, proof/validation, gate coordination, supervision refusal (plan C6), a package version bump, the validator entrypoint, and evidence. No ordinary Proposal-v4 product unit, human interface, reviewer-principal policy, provider routing, or Gate A/B/C answer was introduced. The first bootstrap-successor package bump `0.1.4→0.1.5` and the `.gitignore` un-ignore of `.praxis/validate` are proportionate. The repair added a durable ceremony record type, a governing-authority verifier, and a bound-validation runner; each maps to a frozen-plan requirement (§9, §4), so I classify them as required kernel work rather than control-plane expansion. The eight invariants do not broaden product authority.

## 18. Clean-commit and rebuild transition assessment

The candidate is `modified:true` and must not be installed; nothing was installed. Because the disposition is REVISION_REQUIRED, the transition is **blocked at its first step**. The required sequence remains: repair N1–N6 → refresh evidence → another independent review of the repair → freeze review → commit exactly the reviewed source/evidence → clean tree → rebuild and requalify → obtain `modified:false` identities → independently verify exact identities/evidence → separate human authorization for exact core replacement → governed package deploy → final activation manifest.

Notes for that sequence: `.praxis/validate` becomes tracked only through the `.gitignore` change and must be in the commit; the `modified:false` build will change core, plugin, package, source-manifest, qualification, and pre-activation identities, so all dependent digests must be regenerated; core replacement follows "replace, then restart" (N3 makes that protocol, not detection, the real control on darwin); clean-build enforcement is procedural because the activation predicate accepts `build_modified:true` when the binary agrees.

## 19. Blocking findings

N1 through N6 (§1), with severities P1, P1, P2, P2, P2, P2. Preserved reproductions are in the adjacent evidence directory (`r1-authority/`, `r2-execution/`, `r3-activation/`). Fix directions, not applied: N1 refuse any `WorkPlan != nil` in `Save` and let attach persist independently, or derive safety status from durable authority state (I9); N2 resolve the gate request and decision by digest through the GoalStore at consumption and require them current, owner-bound, and ceremony-backed; N3 hash the descriptor once at capture and refuse any later change, and correct the comment and the claim; N4 canonicalize keys case-insensitively in the duplicate check; N5 use `git ls-files -v`/`--others --ignored` or verify `HEAD` tree content via a clean detached export for the validation run; N6 wrap the buffer so only `Write` is exposed and add a real-process test.

## 20. Non-blocking findings

Cross-Goal attach (R1-G); publication not re-verified before push (I10); in-flight claim-turn leaves pushed checkpoint without a durable record; ceremony record leaves `DecisionRef`/`ExpiresAt`/`IssuedAt`/`AuthorityDigest` unbound and consumes neither `ConfirmationDigest` nor `ConfirmedAt`; `isInteractiveTerminal` accepts any character device (pre-existing root); pending gate blocks independent ready siblings; explicit objective skips hard-dependency readiness; mutation-coverage gaps for post-worker re-checks; integrated-run output digest not stored; `.praxis/validate` freezes the entrypoint only and this is missing from the report's "not closed" list; activation manifest cannot carry plan step-9 fields; `build_modified:true` and format-only digest checks in the activation predicate; test fixtures hand-build the `Repository`; four replayed review-2 probes now fail for a weaker reason than the defect; report wording ("one guard closes them all", "every protected boundary") overstates; the fabricated-lineage plan accepted by plain `Save` (R1-C) is predecessor behaviour and is subsumed by N1's fix; specification parsers still permit unknown fields; ledger and event payloads use plain decoding (a disclosed limit); B4 concurrency unestablished; exec→init window and Windows/Linux unqualified (disclosed).

## 21. Exact rationale for the final disposition

**REVISION_REQUIRED.** Two P1s and four P2s are reproduced, and each contradicts either a claim in the implementation report or a requirement in the frozen v4 plan:

- N1 falsifies the report's central B7 claim and the plan's rule that a plan enters only by the authority-backed path, using the same precondition that made B7 blocking in review 2.
- N2 is the unrepaired half of B9 that review 2 named explicitly, and contradicts plan §3.
- N3, N4, N6 are each a case of a delivered mechanism that is weaker than the property claimed, in the manner review 2 forbade ("do not accept a weaker claim labeled as exact").
- N5 lets a qualifying validation attest content that is not what is published.

The disposition is not AUTHORITY_CONFLICT (no frozen-plan requirement is contradicted by another; the ceremony question resolves under the plan's own text) and not INSUFFICIENT_EVIDENCE (each blocker has a runnable reproduction). It is not a condemnation of the repair: I7, I8, B6, B8, B12, B14, B15 and the plan-level half of B9 are sound, the qualification is green, and every blocker has a narrow fix. It is also a repeat of the observed pattern: three independent reviews have each found defects only after crossing component boundaries, so I recommend the next repair be driven by I9, I10, and I11 above rather than by the six counterexamples alone, and that the next reviewer again run its own inventory.

Not verified here: raw SQLite insertion of a forged completion against a real database file (inferred from schema and the unit-level probe); keychain ACL behaviour for a different same-user binary; the git-remote publication race; concurrent-mutator validation races; Linux and Windows behaviour; and whether the package runtime hashes the plugin executable at launch (registry digests are compared).

Nothing was committed, pushed, installed, deployed, or materialized. Proposal v4 lifecycle transitions were not invoked and Gates A, B, and C remain unanswered. No activation manifest was created.
