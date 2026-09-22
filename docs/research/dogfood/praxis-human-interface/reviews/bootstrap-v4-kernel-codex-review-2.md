REVISION_REQUIRED

# Complete second independent PRE-V4 safety-kernel review

Review date: 2026-09-20. Target: `praxis-human-interface/1`. Reviewer: this independent Codex session; no implementer sub-agent or delegated reviewer was used. Source: HEAD `ff146000aadae0ef60981d445056089f52815869` plus the supplied uncommitted candidate, identified by source-manifest SHA-256 `401a60e40169470050e590f2089652f571c7f6583b343669bcdd71a0bf8eba89`.

The first review was verified as SHA-256 `4857c10783f6692e4c76959f6ca54b2abdf8a35c34c9b8074a60421c807a6a4a` and preserved unchanged. Its unfinished areas were reviewed afresh. This review continued after finding defects and covers the full bounded PRE-V4 implementation scope. It does not authorize Proposal v4, answer Gates A/B/C, or approve installation.

## 1. Disposition

Ten findings require revision, including bypasses of gate completion, ceremony, activation, and current authority. Passing mechanism suites and coherent artifact hashes do not establish the claimed safety properties. The source is not approved as the clean-install rebuild source.

The B1 scope correction works in the configured lifecycle fixture, but native Goal-drive constructs a different, incomplete gate repository. The specific B2 continuation omissions are repaired, but import remains an activation bypass. B3's provenance comparison admits the intended transitions without loosening specification-byte equality; a separate accepted-record field, `Completed`, remains substitutable. B4 correctly handles sequential reuse and rejects a changed request under the same identity.

Independent counterexamples and positive controls are preserved in [review evidence](bootstrap-v4-kernel-codex-review-2-evidence/results.json), with [probe output](bootstrap-v4-kernel-codex-review-2-evidence/probes-final.log). Probe `PASS` means the named counterexample/control was reproduced; it is not a passing safety qualification.

## 2. Findings

### B5 — P1: acceptance can inject completed gates after owner approval

Locations: `pkg/contracts/work_plan.go:407`, `pkg/contracts/work_plan.go:415`, `internal/goaldrive/completion.go:232`.

`AcceptWorkPlan` checks specification bytes, source identity, requirements, and safety bindings but does not reject or compare `WorkCandidate.Completed`. Specification validation also omits that field. `ApplyCompletions` copies it from the accepted plan and only adds ledger completions; it does not clear unsupported completion flags.

`TestReview2CompletedFlagSubstitution` obtains a genuine fixture owner approval for a proposal whose gate has `Completed:false`, changes only the accepted gate to `Completed:true`, and successfully persists the plan through `SaveAcceptedWorkPlanFromAuthorityDecision`. Applying an empty completion ledger preserves `true`. Consequently selection can skip that gate and treat its hard dependency as satisfied without a dossier decision. This is a substitution after approval, not merely a dubious proposal a human knowingly approved.

Require uncompleted safety-plan candidates at proposal/acceptance/attachment and derive execution completion exclusively from validated generation-specific ledger evidence. Keep the B3 provenance exception separate from mutable execution state.

### B6 — P1: native Goal-drive cannot create its gate requests

Locations: `cmd/praxis/goaldrive.go:159`, `cmd/praxis/goaldrive.go:223`, `internal/goalstore/authority_gate.go`.

`buildGoalDriveRuntime` constructs the repository without `InstallationDigest` or `SafetyActivation`, then supplies that value as `AuthorityGates`. The controller's separate activation verifier does not populate either repository field. Gate reconciliation fails at `InstallationGovernanceScope("")`; supplying only the digest exposes a second failure at the persistence activation check.

`TestReview2ProductionGateStoreConfiguration` reproduces both errors using the production field configuration. The implementer's composed test instead calls `openGovernedRepository`, which supplies both fields; its comment that this is exactly how Goal-drive is wired is incorrect. Configure the native repository from the verified bootstrap identity and current activation provider, and qualify through that constructor.

### B7 — P1: supported baseline import bypasses activation and attachment

Locations: `cmd/praxis/goals_lifecycle.go:89`, `cmd/praxis/goalimport.go:26`, `internal/goalstore/import.go:67`, `internal/goalstore/repository.go:61`.

The import path accepts a baseline containing a structurally valid safety-bearing WorkPlan. Neither import nor generic `Repository.Save` invokes the activation predicate or the accepted-plan attachment boundary. The comment that import never admits a WorkPlan is not enforced.

`TestReview2CLIImportWithoutActivation` removes the activation manifest, imports a new generation carrying the safety-bearing plan through the real `goals-lifecycle --operation=import` surface, then reloads the persisted generation successfully. `TestReview2SaveBaselineWithoutActivation` independently confirms the generic persistence gap with a nil verifier. Importing a plan also bypasses the normal durable acceptance/decision lookup; canonical JSON and a matching digest are not authorization.

Reject embedded plans at the baseline-only import surface, or route them through the same authenticated attachment transaction. Enforce activation for safety-bearing baseline writes at the shared persistence boundary.

### B8 — P1: public persistence APIs still bypass the owner ceremony

Locations: `internal/goalstore/repository.go:387`, `internal/goalstore/repository.go:750`, `pkg/contracts/authority_request.go:712`, `cmd/praxis/authority_decide.go:189`.

There are two independently reproduced paths:

- `SaveAcceptedWorkPlan` accepts a caller-supplied legacy `WorkPlanAcceptance` with fabricated authority strings and no durable AuthorityDecision. With an acceptable persisted review and valid activation, it persists a safety plan; `AttachAcceptedWorkPlan` skips decision validation when its `AuthorityRequestID` is empty. `TestReview2LegacyAcceptanceWithoutCeremony` persists and attaches that plan.
- `SaveAuthorityDecision` treats a syntactically valid SHA-256 string as ceremony evidence. No independently recorded ceremony is resolved or authenticated. `TestReview2ArbitraryCeremonyDigest` copies the legitimate owner/root identifiers, supplies an arbitrary all-`f` ceremony digest, persists a gate approval, and consumes it as approved without calling the interactive decision ceremony.

These are in-process repository API counterexamples, not a claim that the read-only lifecycle `decide` command was reopened or that encrypted storage was broken. They contradict the frozen requirement that protected persistence reject copied public fields without actual ceremony evidence.

Disallow legacy acceptance for safety plans and require exact authority-backed lineage. Make protected decisions depend on evidence established by the trusted ceremony boundary rather than a caller-chosen digest. If only CLI authentication is intended, that narrower trust model must be explicitly revised and reviewed; it is not the current claimed persistence guarantee.

### B9 — P1: drive does not revalidate an attached plan's governing authority

Locations: `internal/goaldrive/runtime.go:79`, `internal/goaldrive/controller.go:131`, `internal/goalstore/repository.go:112`.

Drive reloads a digest-valid baseline and verifies activation, but does not reload and validate its acceptance decision/root lineage before selection. Baseline loading checks content integrity, not current authority. Gate reconciliation validates a gate's own decision, not the WorkPlan acceptance that permits reaching the gate.

`TestReview2RevokedWorkPlanStillGovernsGate` attaches an owner-approved plan, durably revokes its acceptance decision, verifies that `LoadAuthorityDecision` refuses it, reloads the baseline through a fresh read, and still obtains a new gate request from the controller. Ordinary selection uses the same missing check. Completed gates are also overlaid without a fresh decision-authority check.

Revalidate the governing acceptance and applicable gate authority at the execution boundary, including restart/recovery and subsequent continuous turns. Immutable historic evidence must remain inspectable without remaining executable authority.

### B10 — P1: validator disappearance can qualify a checkpoint, and completion can invent mechanism success

Locations: `internal/goaldrive/repository.go:479`, `internal/goaldrive/repository.go:562`, `internal/goaldrive/repository.go:638`.

The preflight requires a validator, but the post-worker digest check is nested inside `if declared`. If the worker removes the validator or its executable bit, the code records `repository:no-declared-validation` and sets validated progress instead of failing. Publication follows before candidate-conformance validation. Thus an unvalidated checkpoint can be published even when subsequent conformance execution fails.

`unitCompletionPredicates` accepts the no-validator marker for safety plans. After predicate acknowledgements, `settleCompletion` unconditionally sets `MechanismTestsPassed=true`; it does not require the integrated-pass evidence. Independent probes reproduce both post-worker checkpoint qualification without a validator and a durable mechanism/conformance-qualified completion from a no-validator record. The completion probe isolates the boundary with an acknowledging adapter; it does not claim a permanently deleted Git validator can execute.

Additionally, each candidate predicate executes a mutable checkout path without rechecking its bound digest, and no complete post-validation clean-tree/HEAD check binds all tests to the recorded checkpoint. A source hash checked earlier is not an immutable execution handle.

Fail closed on disappearance at every stage, require explicit integrated-pass evidence for safety completion, and bind validation execution and results to the same immutable profile/checkpoint before publication and completion.

### B11 — P1: process-image verification hashes the current pathname, not the executing image

Location: `internal/bootstrapv4/activation.go:109` (file read at line 122).

`verifyProcessImage` resolves `os.Executable()` and reads the file currently at that path. An atomic replacement can leave the old image running while the pathname contains different bytes. VCS revision and modified state do not uniquely distinguish two builds.

An independent executable probe used the actual `bootstrapv4.VerifyFile`, without substituting its process checks. Two binaries A and B shared VCS revision/modified identity but differed in bytes and a marker. A was started, its pathname atomically replaced with B, then A verified a manifest binding B. Output: `RUNNING A` followed by `VERIFY running=A result=<nil>`. Only temporary probe executables were replaced. See [output](bootstrap-v4-kernel-codex-review-2-evidence/image-probe.log).

Bind verification to the loaded executable object/image using a platform-supported mechanism, or enforce and qualify a process-lifetime installation protocol that makes replacement undetectability impossible. The current claim of exact running-image verification is false.

### B12 — P1: continuous drive uses a stale active-package snapshot

Locations: `cmd/praxis/goaldrive.go:218`, `cmd/praxis/goaldrive.go:233`.

The native runtime captures `ActivePackageIdentity` once. Later `fileSafetyActivation.Verify` calls reuse that struct and do not query installed state. Disabling or replacing the selected package while the process survives is invisible on later turns. Lifecycle verification correctly reloads package state, so the two enforcement paths differ.

`TestReview2CachedPackageIdentity` constructs the same drive verifier, disables the fixture package in its temporary database, confirms that a fresh package lookup and lifecycle check fail, and observes drive verification succeed. The fixture substitutes executable/VCS checks only; package and manifest verification are real.

Resolve the selected package at every protected execution boundary and coordinate that check with package changes. Registry manifest fields alone also do not rehash installed executable bytes; artifact verification elsewhere must be explicitly tied to the admitted runtime identity.

### B13 — P2: malformed dossier suffixes are accepted

Location: `pkg/contracts/gate_evidence.go:185`.

The decoder reads one JSON value and never requires EOF. A valid dossier followed by non-JSON garbage, or by a second object claiming `status:approved`, is accepted when its full byte digest matches. Both cases pass `TestReview2MalformedDossierAccepted`. `DisallowUnknownFields` does not fix this. Duplicate JSON keys likewise retain Go's last-value semantics; no uniqueness/canonical-record parser is applied.

Require exactly one complete, unambiguous schema value and reject duplicate keys. Apply consistent parsing to activation/specification documents too. Hashing exact bytes detects alteration but does not make malformed or ambiguous bytes valid evidence. This finding does not claim that trailing text independently grants approval; it violates the mandatory malformed-evidence refusal and creates parser disagreement.

### B14 — P1: an explicit gate objective can reach a worker

Locations: `internal/goaldrive/controller.go:142`, `internal/goaldrive/controller.go:210`, `internal/goaldrive/controller.go:237`.

When `ChildObjective` is supplied and `WorkCandidates` is empty, preparation skips initial materialization/selection. Its gate check iterates the empty candidate slice. Only after worker resolution does it materialize candidates for context, with no repeated gate-kind check.

`TestReview2ExplicitGateObjectiveReachesWorker` supplies a valid attached safety plan and explicit gate objective, leaves candidates to normal context materialization, and observes one worker call without gate coordination. The worker is an in-memory probe; no external provider ran. This demonstrates the controller API shape also used for pinned recovery objectives, not that ordinary automatic selection chooses gates as work. A legitimate predecessor gate-worker turn was not manufactured.

Materialize and validate the selected objective before any worker resolution, independent of whether selection or recovery supplied its ID. Refuse authority-gate objectives on every provider dispatch path.

## 3. B1–B4 repair assessment

| Repair | Assessment |
|---|---|
| B1, authority versus subject scope | Contract correction is sound: installation scope, exact Goal subject, owner, root generation, request digest, and offered alternative are checked. The CLI ceremony test passes. Native integration is still broken by B6; persistence ceremony authenticity is B8. |
| B2, continuation activation | Direct selector, JSON, continuation acceptance/attachment, request, and review paths now check activation. Review verification precedes persistence. The persistence verifier defaults closed on the covered methods. The broader all-mutations claim fails at import/Save (B7), and native gate configuration is missing (B6). |
| B3, accepted provenance | Proposal matching remains exact; accepted matching permits equality plus `model_proposal → plan` and `model_gate_proposal → authority_gate`. Candidate kind/provenance validation and relationship provenance validation reject cross-kind/unsupported pairings. Acceptance preserves exact proposal specification bytes/refs/digests and bundle binding. No extra provenance-laundering pairing was found. B5 is a separate executable-field substitution. |
| B4, deterministic requests | Sequential identical requests are reused; fresh repository handles in the supplied integration test exercise reload. An independent same-ID/different-checkpoint probe refuses the conflict and verifies the original record remains unchanged. Request digest covers lineage beyond the ID seed. The implementation is read-then-insert, not an atomic concurrent upsert; overlapping initial creators may still see an immutable-write conflict. This review establishes serial turn/restart idempotence, not concurrent linearizability. |

## 4. Full-path and boundary assessment

| Area reviewed | Result and limits |
|---|---|
| DOS before gate | Selected gates resolve one qualified producer/role/class/schema artifact, validate its byte digest and dossier identity, then derive alternatives. Ordinary path avoids worker selection. B5/B6/B8/B13/B14 prevent the blanket guarantee. |
| Checkpoint capture | Git `show <checkpoint>:<path>` preserves exact committed bytes; mutable checkout dossier edits do not alter already captured evidence. Completion validates producer, specification/profile digests and checkpoint, and rejects duplicate role/class. The Git reader checks its size only after buffering the full blob; it is not a bounded streaming read. |
| Authority mutation inventory | Reviewed lifecycle propose/review/request/decide/accept/bind/attach/continue/import, interactive authority decide, legacy and authority-backed acceptance, generic baseline Save/Import/Finalize, authority request/decision persistence, gate reconciliation, completion persistence, historical materialization, and supervision controls. Findings distinguish supported CLI routes from in-process APIs. |
| Ceremony | TTY and authenticated OS-user checks, canonical root scope, typed request/outcome/alternative confirmation, and exact decision replay are present in CLI. Noninteractive lifecycle decide only reads. The stored digest is an assertion, not independently verified ceremony evidence (B8). |
| Activation | Missing manifest and ordinary hash/path/package/binding skew fail in covered paths. Source-tree and qualification digests are format-checked manifest assertions, not independently resolved runtime evidence. Process replacement and long-lived package drift fail the intended guarantee (B11/B12). `BuildModified` is compared to the manifest, not required false; the clean-build restriction remains a deployment-policy prerequisite. |
| Specification integrity | Independently verified bytes, canonical bundle digest, exact source requirements, identities, graph constraints and DOG closure. Embedded bytes survive acceptance and reload; only sanctioned provenance rewrites are allowed. CLI resolution confines static paths and symlinks and checks bytes against preserved evidence. It uses separate symlink resolution and opening, so confinement is not race-free; exact digest/byte matching prevents arbitrary substituted content from passing. No bounded source/specification read or duplicate-key canonicality rule exists. |
| Validation/proof | Frozen profile rejects unknown operations and requires a named passing Go test before acknowledgement. Kernel qualification is not product proof. Missing post-worker validator and unsupported mechanism-pass promotion are B10. Profile invokes mutable repository tests/Makefile; hashing its entrypoint alone does not freeze all executed semantics. |
| Restart/recovery | Durable requests and completion artifacts retain exact bytes and generation identities. Historical safety completion reconstruction is explicitly refused. B4 serial replay works; B9 and B14 expose missing authority/dispatch checks. Gate completions lack worker checkpoints by design and stop the turn. |
| Predecessor behavior | Reviewed predecessor provenance switches reject the new proposal/accepted gate values. This establishes source-level fail-closed behavior for intact gate-bearing plans, not revocation of arbitrary copied binaries or safety-stripped plans. No active predecessor installation was mutated or used for a live lifecycle probe. |
| Admissions and supervision | Unreleased other-generation admissions fence a safety successor; the affected test passes. The check runs once per Runtime.Execute, outside admission's atomic transaction, so it is not a general proof against concurrent other-scope admission. Safety allocation markers make CLI cancel/suspend refuse unauthenticated requests. Raw OS signals and direct event-store writes are outside that CLI guard. Gate C remains undecided; no answer was supplied. |
| Security/process bounds | Argument-vector execution avoids shell interpolation of predicate and artifact arguments. Validator execution has timeout, output limit, wait delay, and Unix process-group cancellation. These are not an OS sandbox: descendants can escape a process group, and non-Unix fallback has weaker cleanup. Provider isolation remains downstream unqualified product work. |
| Minimum scope | Changes are concentrated on contracts, activation, selection, authority, proof, safety tests/profile, package version and evidence. No ordinary v4 unit implementation was run. The unconditional package lookup also changes legacy drive setup behavior; keep any compatibility requirement explicit when repairing B12. |
| Clean-commit transition | Byte-reproducible dirty candidates are review evidence only. Repair and refresh evidence, independently review, then commit/rebuild/requalify clean source and verify final identities. Core replacement, governed package deployment, and final activation remain separate future actions. |

The table records both supported properties and observed implementation limits. It is not a formal proof of absence of every possible defect. No area was omitted because an earlier finding triggered a stop.

## 5. Evidence integrity and independent qualification

All 39 source-manifest entries and 121 specification records match their declared hashes. The preactivation script was inspected and executed with only its output write removed in memory; its computed result equals the preserved JSON. The archived plugin executable was independently hashed from archive bytes, without extraction.

| Identity | Recomputed SHA-256, prefix omitted |
|---|---|
| Frozen v4 plan | `2fd97a1001182d30e592d6650c52aae2704fa2f871bed532aca73bdc9a10ee9a` |
| Implementation report | `92c139b1ef0b014eedeb468b0f497206575c8181a685d411844927b2a1d82857` |
| Source manifest | `401a60e40169470050e590f2089652f571c7f6583b343669bcdd71a0bf8eba89` |
| Qualification results | `e41897d79da42af9c2cad16251aad4ee2e4738900a86b27e647341b765e58736` |
| Candidate activation requirements | `418d3516880144f4a3efdf11812db8818fee7225b640e68f103d963914994cd9` |
| Preactivation verification | `de27cdae31ad3345036ece52a85c899fdeaa4a452c21e89e780abb14dcbcf1a0` |
| Core candidate and independent rebuild | `45b9a89c1a318c7db4dc899ddf944f657b392dd03e74e9abbdef74d763a96a57` |
| Package archive | `b5538c78a7aadac6aae4cd5a6140b62ebccfa4a1c57da5fe3fdbfc764b1ee840` |
| Package manifest | `fec3f4267178eb96e63e13fa14f6280112d12efb6e5b5512c7e2edd3b41111a9` |
| Archived plugin executable | `3733ecb30ed31d02706648d14bbbcf8b1d85dadea6330a1619e501aef7eff765` |
| Specification manifest file | `5a33e46f15f59978ea72972a91879d3feff39355581fe48cb451d30c03a96215` |
| Canonical specification bundle | `17e36c3351d9e510e710c74d3cf4170441f08c150a780e030dbed0896a3cf40a` |

The Go bundle verifier and a separate Python implementation of canonical bundle hashing agree. Independent Python checks rederived all 31 requirement identities/texts/source refs from `bootstrap/goal-establish-source.json`: 12 success criteria, 10 constraints, 5 non-goals, 4 assumptions. Checks also confirmed 22 candidates, sequences 1–22, 68 unique relationships, kinds 28/24/13/3, hard acyclicity, no redundant hard edge, and DOG's complete 21-candidate hard closure. These are specification checks, not implemented product conformance.

The independent `go build ./cmd/praxis` output exactly matches the candidate core. `go version -m` reports Go 1.27.1, darwin/arm64, the reviewed HEAD, and `vcs.modified=true`. Plugin bytes were verified against the archive; a separate plugin rebuild was not claimed.

| Independently executed check | Result |
|---|---|
| Focused six Go packages: contracts, bootstrapv4, goaldrive, goalstore, goals package, CLI | PASS; some results cached |
| `make test-current` | PASS with temporary writable GOCACHE; initial run failed package enumeration on sandbox-denied cache access |
| `go test -race` on the same six packages | PASS |
| `go vet` on the same six packages | PASS |
| Python, bytecode/cache writes disabled | 1,683 passed, 1 skipped |
| Go specification verifier and independent Python recomputation | PASS |
| Source/artifact/preactivation integrity | PASS |
| Independent core rebuild | Byte-identical |
| Independent overlay probes | Counterexamples and controls reproduced; see logs |
| Actual process-image replacement probe | Incorrect acceptance reproduced |
| `go test ./internal/conformance` | FAIL: immutable internal/state attestation is stale |
| `git diff --check` | PASS |

The historical failure was independently observed, not merely repeated from the implementation report. That run stops at the stale `internal/state` attestation; it does not independently enumerate every other stale attestation claimed by the implementer. No attestation was edited.

## 6. Review artifacts and non-actions

The existing graphify graph was queried for navigation and recognized as predating the candidate; it was not rebuilt or treated as safety evidence. Independent probes use Go overlays and temporary encrypted fixture databases, never the active installation. Their sources, replay instructions, logs and machine-readable result are in the adjacent evidence directory.

No implementation or existing test file was repaired. All 39 reviewed source hashes were rechecked unchanged. The initial tree was dirty with the supplied candidate; the review adds only this report and its evidence. The previous review and implementer qualification files remain unchanged.

No production authority record, Goal generation, Proposal v4, gate decision, provider turn, package activation, binary installation, commit or push was made. Fixture decisions and temporary executable replacement were test operations only. Gates A/B/C remain undecided. The next required step is a repaired candidate with refreshed evidence and another independent review, not installation of these bytes.
