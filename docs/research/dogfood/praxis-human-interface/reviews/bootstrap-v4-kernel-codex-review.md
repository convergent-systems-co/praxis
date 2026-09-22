REVISION_REQUIRED

# Independent pre-v4 safety-kernel implementation review

Review date: 2026-09-20. Target: `praxis-human-interface/1`.
Reviewed worktree HEAD: `ff146000aadae0ef60981d445056089f52815869`, with the supplied uncommitted candidate changes.

## 1. Executive summary

The candidate is not defensible to commit as the approved source for the clean installable bootstrap kernel. Two blocking implementation findings were identified: the supported human decision command cannot decide the requests generated for authority gates, and lifecycle continuation bypasses activation verification on acceptance/attachment paths.

The supplied evidence file hashes match. All 35 source-manifest entries and all 121 specification-record file hashes passed verification. The canonical bundle verifier reported the expected digest, cardinalities, relationship-kind distribution, and 31/31 coverage. Focused Go tests, current Go tests, affected-package race tests and vet, and the Python suite passed.

This is a stopped review, not an assertion that every requested review objective was completed. The instruction “If a defect is found, document it and stop” controlled once the blocking defect was confirmed. Already-started qualification jobs were collected; subsequent inspection was limited to substantiating the observed findings and preparing this artifact. Remaining areas are expressly unqualified below. No source or test was repaired.

This review neither authorizes Proposal v4 nor answers Gate A/B/C. The `modified:true` binaries are review candidates and must not be installed directly.

## 2. Evidence-integrity assessment

Independent `shasum -a 256` calculations matched every supplied file identity. Paths in this table are relative to `docs/research/dogfood/praxis-human-interface/`.

| Artifact | Recomputed SHA-256 |
|---|---|
| `plans/workplan-v4-codex.md` | `2fd97a1001182d30e592d6650c52aae2704fa2f871bed532aca73bdc9a10ee9a` |
| `bootstrap-v4/implementation-and-qualification-report.md` | `0f99baf458211fb4cc6aee8fd014fd1fdbb8b13b9445f565b37847dde71e1f07` |
| `bootstrap-v4/specification-bundle/manifest.json` | `5a33e46f15f59978ea72972a91879d3feff39355581fe48cb451d30c03a96215` |
| `bootstrap-v4/source-manifest.json` | `91a65408c4a7872497e468ef54ca17540b0156b680c1f43085762205b1957ddd` |
| `bootstrap-v4/qualification-results.json` | `bd043c9d7949888c087b23449e5f629a7404228f8a524859f8c1984509b0aa3e` |
| `bootstrap-v4/candidate-activation-requirements.json` | `f7fedee85c8948f8982e95baee321d6629660c701c1bf73e299c87d24c87a7e8` |
| `bootstrap-v4/preactivation-verification.json` | `1ac1ed28cc41c5b61fdb032e3a61bb7d24fdc4fac3121248a4780d446d216e98` |

The pre-activation verifier was inspected before execution. Its normal entry point writes `preactivation-verification.json`. To comply with the no-modification boundary, I parsed the script in memory, removed its single `write_text` expression, executed its remaining checks, and compared the computed result object with the existing evidence JSON. All checks and object equality passed. This was an execution adaptation for the review; the repository script was not edited.

That run recomputed the 35 source-file hashes, candidate binary hash, package manifest/archive hashes, invocation-contract digest, validator and descriptive-profile hashes, qualification digest, specification-manifest hash, and each specification-record hash. An additional archive inspection read members without extraction and independently hashed the plugin executable bytes, rather than merely trusting its manifest field.

| Candidate/profile identity | Recomputed digest, with `sha256:` prefix omitted |
|---|---|
| Core binary | `fbb5d7d8fdcb8074f6b8ba069c6fb288fa6333f2d2085f849eb3b4f186c5b5c3` |
| Goals package archive/content | `3a9fd7b5f334efa8044d0f819776e6ecad78d588847d08381f49efad1504ef17` |
| Goals package manifest | `23ec0c044ec657f5edb3b57d313f1d8bf9d7aa4a618392340a84f5b96fba27eb` |
| Archived plugin executable | `a93d159e839ef5300c82b2ee17da930325559e038ec3638bbc4b6fd3a1211835` |
| Invocation contracts | `5acebd861e0d87f9ea230201a3136bbe2798424a4fc30ffff7c49a1fd4e2f043` |
| `.praxis/validate` | `5c7f1924c19784d966f94d386ddcbd67ace8adcdf5f4f94f9b0020a33bea8f0c` |
| Validation-profile description | `12d3f0a1e97a33591d1a320213ba55e9db008bc9d234297c3057b45bc2d1a55a` |

`go version -m` inspected the candidate without executing it and confirmed VCS revision `ff146000aadae0ef60981d445056089f52815869`, `vcs.modified=true`, Go 1.27.1, and darwin/arm64. A reproducible rebuild tying these bytes to the complete source tree was not performed.

The canonical specification verifier returned `sha256:17e36c3351d9e510e710c74d3cf4170441f08c150a780e030dbed0896a3cf40a` and successful permutation determinism. It executes the candidate contracts' canonicalization implementation; a second independent canonicalization implementation was not completed before the stop. Separate Python counting confirmed 28 hard, 24 consumer, 13 interaction, and 3 advisory relationships, and 31 distinct covered requirement IDs. The verifier also confirmed 22 candidates, 68 relationships, and 31 requirement records.

These are byte-integrity and internal-consistency results. They do not establish that all claims in those bytes are true. In particular, the implementation report's claim of activation checks on every lifecycle mutation is contradicted by finding B2.

## 3. Implementation review

The review inspected the complete new `pkg/contracts/gate_evidence.go`, `internal/goalstore/authority_gate.go`, and `internal/bootstrapv4/activation.go`; the gate request/decision contracts and changed CLI decision path; relevant controller gate-selection/completion code; and continuation and GoalStore persistence paths. The authority-gate test was inspected to understand its coverage limit. Other changed implementation diffs were consulted, but not every changed source file received a complete review before the stop.

The existing graphify graph was used only for navigation. It predates this candidate and was not treated as evidence that the new paths are correct. No graph was rebuilt or saved.

The decisive defect appears when composing otherwise plausible contracts: the generated gate request has a scope that the only supported interactive decision command refuses. The gate unit test directly constructs a human decision with the request's scope, so its success does not demonstrate that the installed-owner CLI can produce that decision.

## 4. DOS-before-gate assessment

The inspected design separates proposal-time contracts from runtime dossiers. `ParseAuthorityGateContract` rejects the explicit concrete dossier fields it recognizes. `ResolveGateDossier` requires one matching producer/role/class/schema artifact, verifies its qualification flag and byte digest, and derives alternatives from its decoded dossier. `ReconcileAuthorityGate` builds and persists a digest-bound request before loading a decision. The controller routes a selected gate to that coordinator before worker resolution.

The required temporal chain is nevertheless broken at human decision: B1 prevents the enrolled owner from approving or rejecting the generated request. Consequently the ordinary supported chain cannot reach durable approved gate completion, even assuming DOS executed and qualified correctly.

The exact checkpoint-capture path, all malformed-dossier cases, mutable working-tree resistance, and all substitution/replay paths were not fully audited before the stop. No blanket absence-of-bypass claim is made.

## 5. Authority/bypass assessment

B1 is a mismatch between gate-request scope and installation-owner decision scope. B2 is a separate reachable CLI bypass of the newly added activation predicates.

The normal `goals-lifecycle decide` mutation was changed to recorded-decision replay, and `authority decide` requires interactive input and checks the enrolled OS user. These observations do not establish that every authority-decision persistence entry point is protected. The requested exhaustive CLI/API/internal mutation inventory was not completed.

No authority record was persisted, no human identity was impersonated, and no actual gate decision was attempted during this review. Both findings are supported by source call-path analysis, not an executed live lifecycle transition.

## 6. Activation/fail-closed assessment

`bootstrapv4.VerifyFile` checks the activation file digest against the safety binding; validates manifest fields; compares plan validator/specification identities and the supplied active-package identity; checks executable path and file bytes; and compares VCS revision and modified state.

That function does not protect callers that never invoke it. B2 demonstrates such a path for continuation acceptance/attachment, including after an activation manifest becomes unavailable or invalid. The underlying GoalStore methods do not supply the missing runtime activation check.

Additional limits remain unqualified: correspondence between `SourceTreeDigest`/`QualificationEvidenceDigest` fields and actual runtime evidence, process-image versus executable-path replacement, live package skew after runtime construction, predecessor behavior, and all identity-skew cases. Their requested adversarial review was not completed. Existing passing activation tests do not substitute for that review.

## 7. Specification-integrity assessment

The supplied specification bytes and declared counts are coherent. The bundle verifier passed contract validation, expected relationship-kind counts, coverage, deterministic permutation, and DOG hard-closure checks.

The proposal-source resolver was inspected: it rejects absolute/traversing references, resolves symlinks, checks confinement, reads and hashes source bytes, and compares them with preserved specification bytes; requirement text is compared to the exact baseline element. The review did not complete adversarial filesystem tests or establish race resistance between path validation and opening. Canonicalization ambiguity, duplicate JSON keys, trailing data, and unbounded specification sizes remain unaudited here.

## 8. Validation/proof assessment

The digest-bound validator script was read. Its candidate operations require a named Go test to appear as passed before emitting a structured acknowledgement. Unknown operations fail. Its integrated operation invokes `make test-current`.

The independently run suites below establish mechanism-test results for this review tree. They establish neither candidate product conformance nor product evidence nor settlement. The entire execution-to-checkpoint-to-validation-to-conformance-to-completion call path was not completely audited before stopping. No assertion that missing/failed validation can never become success is made.

## 9. Restart/recovery assessment

The inspected gate request ID incorporates baseline ID/version, candidate ID/specification digest, and dossier digest. The gate request also carries checkpoint lineage and alternatives. The inspected gate test demonstrates changed dossier bytes changing request identity and invalidating a decision for the original request.

This is limited evidence. Durable store reload, request replay, completion replay, lost turns, predecessor generations, and post-restart dossier selection were not exhaustively checked. Restart does not fix B1: rebuilding the same gate request preserves its incompatible scope. B2 specifically allows a continuation after prior acceptance to attach a plan without rechecking present activation.

## 10. Security findings

Two blocking findings are substantiated below. B1 is a governance-path availability/conformance defect. B2 violates an explicit activation enforcement boundary for durable mutations; it does not establish that providers can execute without the separate drive check.

No complete security clearance is given for path traversal, symlink races, TOCTOU, digest/canonicalization confusion, process-tree escape, command injection, evidence bounds, supervision authentication, principal spoofing, or mutable evidence after qualification. These objectives remain open because this review stopped at confirmed defects.

## 11. Qualification results

Let `A` denote `./pkg/contracts ./internal/bootstrapv4 ./internal/goaldrive ./internal/goalstore ./packages/goals ./cmd/praxis`; the race/vet invocations used those same six packages, with the final two in reversed order.

| Check executed independently | Result |
|---|---|
| `go test A` | PASS, exit 0 |
| `make test-current` | PASS, exit 0 |
| `go test -race A` | PASS, exit 0 |
| `go vet A` | PASS, exit 0 |
| `PYTHONDONTWRITEBYTECODE=1 python3 -m pytest -p no:cacheprovider` | PASS, 1,683 passed, 1 skipped |
| `go run ./docs/research/dogfood/praxis-human-interface/bootstrap-v4/verify/specification_bundle.go` | PASS, expected canonical digest and counts |
| Pre-activation verifier checks with only its output write suppressed in memory | PASS; generated result equals preserved evidence |
| `git diff --check` | PASS |

Some Go package results were served from the Go test cache, as shown by the command output. The current-test invocation emitted a sandbox-denied module-stat-cache write warning, but package enumeration and tests completed with exit 0. That warning is not represented as a test failure or silently retried as a transient error.

The historical `internal/conformance` suite was not independently run or investigated before the stop. Its reported stale-attestation classification is therefore **not independently confirmed by this review**. The report's red classification is preserved, not promoted to green. No attestation was changed.

## 12. Scope assessment

The inspected additions address safety-kernel concerns: evidence binding, gate routing, validation, activation, and authority boundaries. No ordinary Proposal-v4 product work was executed in this review. A full minimum-scope assessment of every changed file was not completed. No separate unnecessary-scope finding is established.

## 13. Clean-commit transition assessment

The intended sequence—independent acceptance of reviewed source, commit, clean rebuild/requalification, new `modified:false` identities, independent identity/evidence verification, and separate exact human authorization of core replacement—is appropriate in principle. This candidate has not satisfied its first condition.

Committing and rebuilding identical defective source will preserve B1 and B2. A clean VCS identity does not repair incompatible authority scope or add omitted activation checks. Revised source, refreshed dependent evidence, and another independent review are required before treating a commit as the approved rebuild source. No direct installation of the present candidate is recommended.

## 14. Blocking findings

### B1 — P1: generated authority-gate requests cannot be decided through the supported owner ceremony

Locations, relative to repository root:

- `internal/goalstore/authority_gate.go:66`: `BuildAuthorityGateRequest` sets `RequestedScope` to `"goal:" + baseline.ID + "/" + baseline.Version`.
- `pkg/contracts/authority_request.go:135`: the installation governance root scope is derived as `installation-governance:<bootstrap digest>`.
- `cmd/praxis/authoritybootstrap.go:376`: enrollment derives that canonical scope and rejects a different supplied scope.
- `cmd/praxis/authority_decide.go:148`: the decision command resolves the current installation root.
- `cmd/praxis/authority_decide.go:156`: it requires `request.RequestedScope == root.Scope` before presenting the confirmation ceremony.

Call path: controller selection (`internal/goaldrive/controller.go:183`) → `ReconcileAuthorityGate` → `BuildAuthorityGateRequest` → durable pending request → `authority decide` → unconditional scope mismatch rejection. Approval with a valid offered alternative and rejection both hit the same check. Noninteractive lifecycle decide cannot supply the missing decision because it is now read-only.

For the target, the request scope has the form `goal:praxis-human-interface/<attached generation>`; a canonical root scope begins `installation-governance:`. They cannot be equal. The error is deterministic and precedes the interactive prompt. Even correctly qualified DOS evidence cannot make the chain proceed to human decision and durable approved gate completion.

The inspected gate test (`internal/goalstore/authority_gate_test.go:44`) directly constructs a decision using the gate request's scope. It does not exercise root enrollment and the actual CLI ceremony, explaining why its passing result does not cover this mismatch.

Required resolution for a future review: reconcile request and owner-authority scope through the intended governed decision path, preserving exact request/dossier/alternative binding, and qualify that complete path against a canonical enrolled root. Do not weaken or bypass authority checks as an operational workaround. No repair was made here.

### B2 — P1: lifecycle continuation bypasses runtime activation verification

Locations, relative to repository root:

- `cmd/praxis/goals_lifecycle.go`: dispatch returns directly to `continueGoalsLifecycle` for operation `continue`, before the individually guarded lifecycle cases.
- `cmd/praxis/goals_continuation.go:112`: an approved request is promoted using `SaveAcceptedWorkPlanFromAuthorityDecision` without `verifyLifecycleSafety`.
- `cmd/praxis/goals_continuation.go:242`: an accepted plan is attached using `AttachAcceptedWorkPlan` without `verifyLifecycleSafety`.
- `internal/goalstore/repository.go:441`: acceptance checks stored ceremony/activation binding fields, but does not verify current runtime activation.
- `internal/goalstore/repository.go:137`: attachment checks source, accepted plan, and decision lineage, but contains no current activation verifier before persisting the successor.

Concrete trigger: an exact safety-bearing plan was accepted while activation was valid, but has not yet been attached. The activation manifest is subsequently removed or becomes invalid. `goals-lifecycle --operation=continue` can select the already accepted plan and persist its successor baseline through `attachContinuedPlan`. The guarded direct `attach` command would instead invoke runtime verification and refuse. With a still-valid approved, properly bound request, the continuation acceptance path likewise lacks that runtime predicate.

The same continuation code also constructs new WorkPlan requests without the added safety ceremony/activation fields (`cmd/praxis/goals_continuation.go:190`). Later acceptance has a check that can reject these; this does not repair the missing activation predicate on continuation mutations.

This finding concerns current activation enforcement, not fabrication of acceptance authority. It does not claim that a later drive passes its separate safety check. The requirement explicitly forbids acceptance and attachment mutations without exact activation, so refusing only at drive is insufficient.

Required resolution for a future review: enforce equivalent current activation predicates across continuation and direct lifecycle paths, and test absent/skewed activation using valid persisted safety-bearing lineage. No lifecycle transition was executed and no repair was made here.

## 15. Non-blocking findings and review limitations

- The normal pre-activation verification script writes its output evidence. A future read-only verification mode would simplify independent review. This review suppressed only that write in memory and retained all checks.
- Byte hashes and passing mechanism tests were reproducible, but they do not support the implementation report's universal claims about mutation coverage.
- Historical-attestation classification, exhaustive changed-source review, predecessor probes, malformed evidence/filesystem cases, and restart/security coverage remain open. These are review limitations, not claims that those areas passed or independently discovered defects in each area.
- The dirty candidate identity is expected at this stage and is not itself a defect. It remains ineligible for direct installation.

## 16. Exact rationale for disposition and final repository status

`REVISION_REQUIRED` follows from concrete implementation defects, not merely missing final activation or the expected dirty review tree. B1 prevents the required governed human decision path from functioning. B2 permits safety-bearing lifecycle mutations through a path that omits the required runtime activation predicate. Either blocks treating this source as the approved basis for rebuilding the installable safety kernel.

The remaining review cannot be treated as complete or approved because the user required stopping on discovery of a defect. It must resume against revised evidence in a subsequent authorized review.

No source, test, bootstrap evidence, attestation, installed core, package deployment, activation state, or Goal lifecycle record was modified. No commit or push was made. The sole intended repository change made by this review is this artifact.

Final `git status --short` (the candidate entries were present at review start; the review artifact is the sole added entry):

```text
 M .gitignore
 M cmd/praxis/authority_decide.go
 M cmd/praxis/cli_help.go
 M cmd/praxis/goaldrive.go
 M cmd/praxis/goals_import_surface_test.go
 M cmd/praxis/goals_lifecycle.go
 M cmd/praxis/supervision.go
 M internal/goaldrive/admission.go
 M internal/goaldrive/completion.go
 M internal/goaldrive/controller.go
 M internal/goaldrive/execution_contract.go
 M internal/goaldrive/git_repository.go
 M internal/goaldrive/materialization.go
 M internal/goaldrive/repository.go
 M internal/goaldrive/runtime.go
 M internal/goalstore/repository.go
 M packages/goals/package.go
 M packages/goals/package_test.go
 M pkg/contracts/authority_request.go
 M pkg/contracts/work_plan.go
 M pkg/contracts/work_relationship.go
 M pkg/contracts/work_selection.go
?? .praxis/
?? cmd/praxis/bootstrap_v4_test.go
?? docs/research/dogfood/praxis-human-interface/bootstrap-v4/
?? docs/research/dogfood/praxis-human-interface/reviews/bootstrap-v4-kernel-codex-review.md
?? internal/bootstrapv4/
?? internal/goaldrive/bootstrap_v4_test.go
?? internal/goaldrive/validation_process_other.go
?? internal/goaldrive/validation_process_unix.go
?? internal/goalstore/authority_gate.go
?? internal/goalstore/authority_gate_test.go
?? pkg/contracts/gate_evidence.go
?? pkg/contracts/gate_evidence_test.go
?? pkg/contracts/work_plan_safety.go
?? pkg/contracts/work_plan_safety_test.go
```

The exact SHA-256 of this review is reported separately after writing it, to avoid a self-referential hash.
