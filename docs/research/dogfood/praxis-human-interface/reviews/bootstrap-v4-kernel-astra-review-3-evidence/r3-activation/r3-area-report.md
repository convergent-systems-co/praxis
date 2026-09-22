# AREA 3 review — activation / runtime identity / package identity / exact JSON / spec-evidence integrity

Reviewer: forked adversarial reviewer (area 3). No repo file modified; all probes built from scratch dirs with `go -overlay`.
Scratch artifacts (all under scratchpad/r3/): image-probe-drive.py + probe/ (A/B binaries, sleeper overlay), image-probe-r3.log,
json/main.go + json-probe-r3.log, gd/r3_test.go + publish-window-probe-r3.log, bundle_indep.py + bundle-indep.log,
pre.py/preactivation-recomputed.json, specbundle-go.log, plugin-rebuild, pkg/ (extracted archive), focused-b6-b12.log.

## Summary of findings
| ID | Sev | Blocking? | Title |
|----|-----|-----------|-------|
| R3-A1 | P2 | Yes (unless the claim is narrowed and a human accepts it) | B11 fix does NOT detect in-place rewrite of the running executable on darwin; doc comment claim ("refused by the OS or caught because descriptor is re-read") is false there |
| R3-A2 | P2 | Recommend fix before commit; not a bypass by itself | B13 duplicate-key check is case-sensitive; Go decoding is case-insensitive -> `{"Status":"undecided","status":"approved"}` accepted, last wins |
| R3-A3 | P3 | No (record; recommend re-verify before publish) | No activation / governing-authority re-check between worker return and `PublishCheckpoint`; drift during a turn still pushes the checkpoint; on a claim turn the push happens, then settleCompletion refuses and returns an empty record (pushed, unrecorded) |
| R3-A4 | P3 | No | Exec->init window is real and demonstrated (documented residual) |
| R3-A5 | P3 | No | Activation manifest struct cannot carry the plan-step-9 fields (installation root, activation time, receipt digests, compat fence, predecessor recovery id) because unknown fields are refused; final manifest author must match struct exactly |
| R3-A6 | P3 | No | Activation predicate does not require build_modified=false, and source_tree/qualification digests are format-checked only (unchanged from review 2 observation; deployment-policy) |
| R3-A7 | P3 | No | Test fixtures hand-build the Repository ("mirrors the production opener") instead of calling newGovernedRepository — same class of test/production divergence as B6; production constructor `openGovernedRepositoryFromBootstrap` itself untested (needs keychain). Fixture omits `AuthorityGeneration` |

## R3-A1 — B11: in-place overwrite passes verification (darwin/arm64, Go 1.27.1)
Probe: two builds A, B (same VCS identity, same size 4214626 bytes, different bytes, `-X main.marker`), real `bootstrapv4.VerifyProcessImage`. Log: image-probe-r3.log.
| Scenario | Result |
|---|---|
| control (manifest binds A) | nil (correct) |
| atomic rename B over path; manifest binds B | refused: digest mismatch (correct) |
| atomic rename B; manifest binds A | refused: no longer the file at path (correct) |
| unlink + recreate B; manifest B / manifest A | refused / refused (correct) |
| symlink retargeted to B; manifest B | refused (correct) |
| identical bytes, new inode (cp+rename) | refused ("replaced while running") — availability false-positive only, acceptable ("replace, then restart") |
| hardlink launch, manifest names original path | nil (same inode, same bytes: correct) |
| symlink launch, manifest path = symlink or real | nil (correct) |
| **in-place truncate+write B into running exe; manifest binds B** | **nil — process is still executing A (marker prints A) yet verification passes for B** |
| **in-place same-size overwrite with B; manifest binds B** | **nil — same false accept** |
| in-place same-size overwrite; manifest binds A | refused |
| **exec->init window: path replaced by B after exec, before bootstrapv4 init; manifest binds B** | **nil — process running A verifies as B** |
Root cause: same inode after an in-place write => `os.SameFile` true, descriptor re-read hashes the NEW bytes, size check only catches size change. macOS does not return ETXTBSY for writing a running executable (the mutation succeeded). The comment at activation.go:~127 ("an in-place rewrite of a running executable is refused by the OS or is caught because the descriptor is re-read") is true on Linux (ETXTBSY) and false on darwin, the qualified/dev platform. The regression test and mutation check ("B11 same-file check KILLED") cover only rename replacement.
Provable claim actually delivered: "the bytes currently readable through the inode that the executable path resolved to at bootstrapv4 package-init hash to D, and the manifest path currently names that inode." It is NOT "the bytes of the code being executed hash to D" on darwin (in-place writes; exec->init race; no /proc/self/exe). The report's B11 section states the exec->init limit honestly but not the in-place hole. The review-2 instruction was to not accept a weaker claim labeled exact; this is such a case for in-place rewrite (e.g. an operator `cp`-installing over the live binary while a drive process lives).
Cheap closure available (not applied — review only): hash the descriptor once at capture and refuse on any later digest change; docs must then state exec->init as the sole residual. Note the darwin code-signing kernel behaviour was not observed to kill the process; not relied on.

## R3-A2 — B13 case-insensitive duplicate keys (json-probe-r3.log)
UnmarshalExactJSON: trailing value/garbage refused; exact and \u-escaped duplicate keys refused (nested too); unknown fields refused when requested; BOM refused; empty/>1MiB refused; 1e999, 1.5->int refused. But:
- `{"Status":"undecided","status":"approved"}` -> err=nil, status="approved" (Go folds case; dup check compares exact strings). Same nested (`{"A":1,"a":2}` -> 2).
- invalid UTF-8 inside strings accepted and silently replaced with U+FFFD (digest still binds bytes, so no substitution; parser-disagreement class only).
- `null` root accepted (zero value; downstream required-field checks apply).
- specification parsers (candidate, relationship, gate contract, governed outputs) pass disallowUnknown=false; only dossier, ceremony record and activation manifest refuse unknown fields (matches report's stated scope).
Impact: parser disagreement between Go and any case-sensitive reader (Python bundle verifier etc.) on dossier/manifest/spec bytes; no direct authority bypass found because dossier status is compared to `undecided` after Go's last-wins decoding and bytes are digest-pinned. Still, the B13 claim "no duplicate object keys at any depth" is not fully met.
Plain json.Unmarshal remains on store-internal payloads (goalstore/*, goaldrive ledger/completion event payloads). Goalstore payloads are authenticated ciphertext (acceptable under the stated trust model). Ledger/event-store payloads (completion.go:184, ledger.go:196, admission.go) are not; consistent with the report's acknowledged "direct event-store writes" limit. command_worker.go:117 (worker stdout) enforces single-object; worker output is proposal-only.

## R3-A3 — activation/authority not re-resolved before publication (publish-window-probe-r3.log)
Overlay test in internal/goaldrive with a verifier valid for one call then failing:
- progress-only turn (commit, no completion claim): 1 verify call total (admission only); checkpoint published, remote HEAD moved, no error.
- claim turn: 2 calls; `PublishCheckpoint` runs first (remote HEAD moved), then settleCompletion refuses; error path returns `TurnRecord{}` without recordTurn -> pushed checkpoint with no durable turn record.
The same structure applies to VerifyGoverningAuthority (revocation mid-turn). Report claim "resolved at every protected boundary" / B9 "future execution refuses" holds for future turns; the in-flight publication is the gap. Outward consequence is a git push of a checkpoint from a turn admitted under valid activation, so non-blocking, but should be recorded or closed by verifying before PublishCheckpoint (repository.go:344).

## R3-A4 — exec->init
Demonstrated with an earlier-initialized package (`internal/aaasleep`) sleeping in init: replace path after exec, process running A verifies against manifest for B. Window = runtime + all packages initialised before bootstrapv4 (not measured for the real binary; probably ms). Documented in report; assess as acceptable-with-honest-claim only if R3-A1 is fixed and the human accepts residual; Windows unqualified.

## B12 / I6 — package identity re-resolution: VERIFIED with one gap
- `lifecycleSafetyActivation.Verify` -> `verifyLifecycleSafety` re-reads env manifest path, opens read-only DB, `SelectedPackage` + state=="active", recomputes contract digest, re-verifies image; no cache/sync.Once/memo anywhere (grep clean). getenv nil fails closed.
- Consumers: GoalStore persistence guard at SaveWorkPlanProposal (repository.go:296), Review (358), AcceptFromAuthorityDecision (471), AuthorityRequest (649), AuthorityDecision (790), Attach (170), ReconcileAuthorityGate (authority_gate.go:22); Controller.prepare (controller.go:193), settleCompletion (repository.go:725); lifecycle CLI continue path (goals_lifecycle.go:~400) additionally verifies explicitly. Save/Import/Finalize refuse safety plans outright (ErrSafetyPlanRequiresAttachment).
- Test `TestKernelRepair2B12...`/`PathD` pass here (focused-b6-b12.log). They flip installed_packages state, remove manifest, skew image (substituted), wrong profile.
- Gap: publication (R3-A3). Residual acknowledged: registry manifest fields compared, installed plugin executable bytes not re-hashed by this verifier (I did not verify boundexec re-hashes at launch; boundexec compares registry digests, executor.go:133).
- Perf note (non-safety): full-binary SHA-256 (29.9 MB) per verification call.

## B6 — VERIFIED at construction, test-fixture caveat
buildGoalDriveRuntime now calls openGovernedRepository (-> newGovernedRepository with InstallationDigest, SafetyActivation, AuthorityGeneration) and newGoalDriveController (AuthorityRequests, AuthorityGates, GoverningAuthority, SafetyActivation). Test calls real buildGoalDriveRuntime and reconciles a gate through the returned store; passes. Other non-test constructors of goalstore.Repository (core.go:189, authoritymodel.go:60 read-only opener, authoritybootstrap.go:432) and goaldrive.Controller (supervision.go:231, 407) carry no SafetyActivation: requireSafetyActivation and prepare/settle refuse when nil (fail CLOSED); supervision controllers only reconcile/cancel, they do not execute turns. No fail-open constructor found. Caveat R3-A7: test fixture repositories are hand-built; only inspection proves newGovernedRepository equals what fixtures mirror.

## R3-A5 / A6 — activation-manifest semantics vs frozen plan (workplan-v4-codex.md steps 9, 74)
Manifest fields: version, kernel_version, source_commit, source_tree_digest, build_modified, active_binary_path/digest, package id/version/content/exec/contract digests, validation_profile_digest, specification_bundle_digest, qualification_evidence_digest. Plan step 9 additionally lists installation root, activation-receipt digests, compatibility-fence version, activation time, predecessor recovery identity. With DisallowUnknownFields those cannot be present in the final manifest; needs reconciling before the final manifest is written (either extend struct or the plan/report must say they are out of the kernel manifest). VerifyManifestFile compares only profile+spec digests to the plan binding and package identity; source_tree_digest and qualification_evidence_digest are validated as SHA-256 format only; build_modified:true is accepted if the binary agrees. Clean-build is enforced only procedurally (transition step). Not a regression from review 2.

## Specification / evidence integrity — independently recomputed
- Source manifest: 49 files, 0 hash mismatches, no changed/untracked non-evidence file missing (git status vs manifest).
- Identities recomputed and match report/requirements: core 5e617aab…5129; plugin exe 1def4aac…c50 (archive extraction and independent `go build ./packages/goals/plugin`); archive 72f09a85…28d63; package manifest 8b71a815…0e6; source manifest d130352b…3620; qualification results c5217905…1dc6b3; requirements 3244a6ab…6982; preactivation 8e7f718b…63e9; validator 5c7f1924…8f0b; profile description 12d3f0a1…a55a; bundle manifest file 5a33e46f…6215; v4 plan 2fd97a10…ee9a; review1 4857c107…; review2 c2704e9d…. Candidate rebuilt from source: byte-identical. Candidate `go version -m`: rev ff146000…, vcs.modified=true.
- preactivation_evidence.py run from a scratch copy (ROOT pointed at repo, output redirected): output BYTE-IDENTICAL to the preserved preactivation-verification.json.
- Go specification verifier: PASS, digest 17e36c33…0a. Independent Python re-implementation of ComputeSpecificationBundleDigest from the raw record files (base64, sorted kind/key, compact JSON): identical 17e36c33…0a; counts 22/31/68.
- Package archive: contains exactly plugins/praxis-goals-plugin (sha 1def4aac…) and plugins/praxis-goals.json (82fcd756… = manifest plugin digest).
- qualification-results.json ↔ repair-2-evidence logs: focused/race logs all-ok; historical.log shows internal/learning stale attestation (the report's "varies by run" is plausible, I observed only what the parent's background run will show); mutation log has 13 KILLED lines matching the 13 claimed. "B11 same-file check KILLED" is consistent with A1 (test only covers rename).
- Not verified by me: python.log/test-current.log/vet logs re-run (parent's background run owns those); Windows; boundexec launch-time hashing.

## Verdict for area 3
B6 verified; B12 verified (gap A3, non-blocking); B13 substantially fixed but not fully (A2, P2); B11 fix is real for pathname replacement (atomic rename/unlink/symlink) and I could not falsify those, but the claim is over-broad on darwin for in-place rewrite (A1, P2) and exec->init remains (documented). Recommend REVISION_REQUIRED for area 3 on A1 (and A2), everything else non-blocking.
