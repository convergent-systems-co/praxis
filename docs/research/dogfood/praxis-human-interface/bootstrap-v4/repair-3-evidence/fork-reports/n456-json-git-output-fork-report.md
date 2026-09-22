# N4 / N5 / N6 implementer report (fork n456)

Scope: N4 exact JSON, N5 content-bound git validation, N6 output bound. Only owned files edited. Nothing committed.

## 1. Files changed / tests added
Changed:
- pkg/contracts/exact_json.go (rewritten: type-directed strict parse)
- internal/goaldrive/git_repository.go (bindValidationExecution/limitedValidationOutput removed; new collector, bindCheckpoint, export, PublishGuard, PushAndVerify pushes exact SHA, hardened controller git)
- internal/goaldrive/validation_process_unix.go (+terminateValidationProcessTree), validation_process_other.go (no-op, documented)
New tests:
- pkg/contracts/exact_json_strict_test.go: TestUnmarshalExactJSONRefusesSemanticDuplicateKeys, ...AcceptsExactlyOneCompleteUnambiguousValue, ...RefusesMalformedAndUnboundedInput, ...DifferentialAgainstEncodingJSON (400 random key-case mutations)
- internal/goaldrive/git_validation_bound_test.go: TestGitBoundValidationRunsAnExactExportNotTheWorkerCheckout, ...BindsToCommitContentNotWorkingTreeState (assume-unchanged, skip-worktree, unstaged, staged, index/worktree disagree, .git/info/exclude; committed-bad and committed-ok), ...NeverExecutesWorkerCheckoutHooksOrFsmonitor, ...RefusesCheckpointAndTreeSubstitution, ...RefusesValidatorThatDiffersFromTheProfile, ...DetectsExportTamperingAndAlwaysCleansUp (incl. stat-cache spoof, excluded file, unwritable dir), ...CleansUpAfterTimeout, ...RefusesSubstitutionOnADetachedHead
- internal/goaldrive/git_validation_output_test.go: TestValidationOutputBoundFailsClosedThroughTheRealCollector (exact bound on stdout/stderr/split passes; +1 byte, overflow with exit 0 / nonzero, interleaved, unbounded stream fail), TestBoundValidationOutputOverflowFailsClosed, TestValidationTerminatesTheWholeProcessTree, TestValidationCancellationTerminatesDescendants
Existing tests: `TestValidationOutputIsBounded` in bootstrap_v4_test.go:220 constructs the removed `limitedValidationOutput` and no longer compiles. I did not own that file. It must be deleted or replaced (the new real-process tests supersede it). I ran my suites with an overlay that drops it.

Last verified state: `go test ./pkg/contracts` ok; `go vet ./pkg/contracts` clean; goaldrive git/validation tests passed twice in a row through the overlay (~30s) before the final small edits (ls-files --others check, force-add rehash, detached-HEAD test) — those were run once green as part of the same suite. I did NOT run a final full-package goaldrive run, and did not run the replay of Astra's two preserved probes (see 4).

## 2. Decoder audit
Fixed for every caller of UnmarshalExactJSON (type-directed exact-name keys, folded-key rejection incl. Kelvin sign/long-s, dup keys, invalid UTF-8, unpaired surrogate escapes, depth cap 64, 1 MiB, trailing values, BOM):
| decoder | file:line | source | exact |
|---|---|---|---|
| candidate specification | pkg/contracts/work_selection.go:148 | external bytes, digest-pinned | YES (now strict) |
| relationship specification | pkg/contracts/work_relationship.go:68 | same | YES |
| gate contract / governed outputs | pkg/contracts/gate_evidence.go:117,145 | same | YES |
| dossier | pkg/contracts/gate_evidence.go:186 | checkpoint bytes | YES |
| ceremony record | internal/goalstore/ceremony.go:74 | store | YES |
| safety classification record | internal/goalstore/safety_classification.go:49 | store (parent's new file) | YES |
| activation manifest | internal/bootstrapv4/activation.go:90 | external file | YES |
NOT exact (plain json.Unmarshal / Decoder) — YOU must wire, files not owned:
| decoder | file:line | source | exact? |
|---|---|---|---|
| baseline import document | cmd/praxis/goalimport.go:29 | EXTERNAL file | no |
| lifecycle request bodies (propose/review/request/decide/accept/attach; carry WorkPlanProposal/WorkPlan/decision) | cmd/praxis/goals_lifecycle.go:104,186,251,278,328,393 | EXTERNAL --input | no |
| lifecycle probe/selector | cmd/praxis/goals_lifecycle.go:1017,1026 | external | no |
| goal establishment document | cmd/praxis/goal_establish.go:43 | external | partial: unknown fields + single value refused; case-fold not |
| evaluation document | cmd/praxis/goal_complete.go:57 | external | no |
| worker result | internal/goaldrive/command_worker.go:117 | worker stdout | partial: single object; case-fold not |
| goal-drive argv env | cmd/praxis/goaldrive.go:280 | env | no (array of strings; low risk) |
| baseline load | internal/goalstore/repository.go:144 | store-authenticated | no — recommend UnmarshalExactJSON: it decodes the plan incl. Safety |
| proposal / accepted plan / requests / decisions | internal/goalstore/repository.go:352,419,595,651,720,746,776,929 | store-authenticated | no (acceptable under storage-key trust; cheap to wire) |
| other goalstore payloads (historical_authority, goals_publication*, publisher_governance, pending_authority, root_authority_succession, authority_rerequest) | many | store-authenticated | no — not safety-classification bearing; not audited line by line |
| completion / turn / admission / goal-completion ledger events | internal/goaldrive/completion.go:184, ledger.go:196, supervision.go:220, goal_completion.go:202-220, admission.go:175,181 | UNAUTHENTICATED plaintext event store | no — your N2 sealing is the control; strict parsing is defense in depth |
| authority request wire | pkg/contracts/authority_request.go:105 | store | no |
Recommendation: wire UnmarshalExactJSON (disallowUnknown=true where the type is closed) at goalimport.go:29, all six goals_lifecycle.go request decodes, goal_complete.go:57, repository.go:144/352. Caveat: types with custom UnmarshalJSON are checked structurally only (duplicates), not by field.

## 3. Mutation results
Killed by a relevant regression: N4 case-fold guard (2 tests); N6 embedded-buffer collector; N6 overflow-ignored; N6 no tree kill; N6 no cancel on overflow (killed only by TEST TIMEOUT/hang — no named failing test; I later added a 20s goroutine bound so it now fails by assertion, NOT re-run); N5 validator-runs-in-worker-checkout; N5 post-run export verify removed; N5 publish guard neutralised; N5 push HEAD instead of exact id; N5 controller git hardening removed; N5 recorded-tree comparison removed.
SURVIVED (all four re-run after adding targeted tests; still SURVIVED):
- fresh-index content rehash removed: expected to be killed by the new "stat-cache spoofed in-place rewrite" case; it survived. I have NOT established why (candidates: that case is caught by `git status` in the export despite trustctime/checkStat config, i.e. redundant layer; or the spoof does not work as intended). Unresolved. Treat the rehash as an unproven layer.
- hint-flag scan + status removed (rehash kept): redundant with the rehash, by construction (status/hint are the cheap layer, rehash the content layer). Individually unkillable unless the other layer is blind; the two layers can only be jointly mutated. Joint mutation NOT run.
- HEAD-equality removed from bindCheckpoint: expected to be killed by the new detached-HEAD test; survived. NOT explained. PushAndVerify has its own HeadIs and the branch-ref comparison covers the non-detached case, but the detached-HEAD test asserts PublishGuard itself fails, so the survival suggests another check in bindCheckpoint (or the test) refuses first. Unresolved; I could not finish the diagnosis.
- committed-validator digest check removed (first run BUILD-FAILED: unused variable; fixed the mutant, re-run SURVIVED): redundant — exportCheckpoint re-checks the exported validator digest, and the test "committed validator differs from the profile" is refused there. Missing test I could add: bindCheckpoint alone (VerifyValidationBinding) with a swapped committed validator.
NOT RUN: joint mutation of status+hint+rehash; mutations of the ls-files --others check and the force-add/excludesFile rehash change; re-run of "N6 no cancel" after the assertion change.

## 4. Residual limits / not done
- Astra's preserved probes (TestR2WorktreeDivergence..., TestR2BindingPredicate..., TestR2ValidatorTimeout...) were NOT replayed after the final edits. The affirmative behaviour they test (committed-bad checkpoint with assume-unchanged/skip-worktree is refused; 2,000,000-byte exit 0 is refused) is covered by my own equivalent tests, but that is not the required replay.
- Caller wiring needed in repository.go (not owned): (a) call `validator.PublishGuard(ctx, record.EndHead, profileDigest)` on the CheckpointValidator immediately before PublishCheckpoint for a safety-bearing turn (add it to the CheckpointValidator interface), together with your activation/authority re-verification; (b) settleCompletion's `VerifyValidationBinding` is now content-based (no worktree/status reliance); (c) deriveRepositoryOutcome still requires `snapshot.Clean` (status-based) — that is an ordinary cleanliness policy, no longer used as byte-identity evidence.
- RunBoundValidation now works on an export clone under os.TempDir; the export contains the checkpoint tree only. A validator that needs untracked/ignored files from the worker checkout (build caches) will no longer see them — behaviour change to note.
- `qualifiedTrees` is a process-global sync.Map keyed by dir+checkpoint (never evicted; bounded by checkpoint count per process); a restart loses the recorded tree (then only the recompute-and-compare inside a run applies).
- Validation is still not an OS sandbox: a descendant that changes process group (setsid) escapes the group kill; non-unix has no tree kill.
- The content rehash trusts the local git binary and object store; objects are read from the worker checkout's object database (clone copies them), so a worker that corrupts objects is detected only by git's own hashing.
- Advisory paths (RunDeclaredValidationWith for non-safety plans / Goal evaluation) still execute the worker checkout directly; only the output bound and tree-kill were fixed there.
- Linux/macOS git semantics assumed; Windows unqualified. Unpaired-surrogate rejection is stricter than encoding/json (intentional).
