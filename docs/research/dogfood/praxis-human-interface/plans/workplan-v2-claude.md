# Advisory WorkPlan proposal v2: `praxis-human-interface/1`

Advisory model output only. Every candidate and relationship below is destined for `"provenance": "model_proposal"`.
This plan grants no authority, does not review itself, and modified nothing in the repository. Praxis was not invoked.
`planner-proposal-v2.json` is **not** written by this plan.

- Goal `praxis-human-interface/1`, baseline digest `sha256:afda0866398d14a223f3a090d554c820e9e945922a936e6f1edb9c06d9177530`
- Superseded evidence: proposal v1 `sha256:c3903f9f…9030`, review `sha256:b131b45b…efcb` (`REVISION_REQUIRED`)
- Repo HEAD `00a3c36`. `STORY.md` file sha256 begins `5c62ba3c` (unchanged since v1; full digest is computed at manifest time, B0).

---

## 1. What v2 is, in one paragraph

18 candidates (v1 had 13), 61 relationships (v1 had 34), of which 18 are hard (v1 had 25). The set is a linear extension of every
relationship, so it is acyclic and deterministically schedulable. Five v1 units were split, one unit was added, none removed.
The proposal is made defensible **before** authority without any code fix: requirements are addressed by content-derived element
identity taken from an independently recomputed element manifest, and the review evidence is pinned into the authority request
through existing fields. The identity and reviewer-provenance fixes are then ordinary units in the plan, not preconditions of it.

---

## 2. What was verified against the repository (not inherited from v1)

I re-derived each fact below. Items marked **NEW** were not in v1 or the review.

### Requirement identity
- `RequirementRef.Validate` checks three non-empty strings only (`pkg/contracts/work_selection.go:17-22`). `BuildWorkPlanProposal`'s comment says it
  "verifies requirement references"; it does not (`packages/goals/workplan_proposal.go:13-21`).
- The digest sorts the five string lists (`packages/goals/canonical.go:57-60`) but `Repository.Save` stores `json.Marshal(baseline)` in **source order**
  (`internal/goalstore/repository.go:80`) and `Load` only re-checks the order-blind digest (`:123-127`).
  Completion reads **stored** order: `AssessGoalCompletion` indexes `baseline.SuccessCriteria` and `successCriterionIndex` parses `#success_criteria/<n>`
  (`internal/goaldrive/completion.go:250-253,278-291`). Coverage ignores any out-of-range or non-matching ref silently.
- **NEW.** For this Goal, source order differs from canonical order in all four lists. Success criteria source-to-canonical positions are
  `[3,8,12,6,7,11,9,2,1,5,4,10]`. v1's `success_criteria/5` therefore names one element in stored order and a different element in canonical order.
  v1's refs were probably correct in stored order, but nothing could prove it, and the digest does not pin that order.
- **NEW.** `ImportSourceDigest` (sha256 of the raw establish file) is the only pin of stored order, and it is **outside** the canonical digest
  (`canonical.go:28-46`). Source provenance is therefore not integrity-bound to the generation digest.
- **NEW.** Review coverage is keyed by `RequirementRef.ID`, a free string (`pkg/contracts/work_plan.go:218-244`). The selector review path derives
  "covered" from the proposal's own IDs (`cmd/praxis/goals_lifecycle.go:733-749`), so it is tautological. Its `ReviewDigest` is
  `sha256(reviewRef + proposalDigest + status)` (`:745`), which excludes findings, coverage and reviewer provenance.
- **NEW.** Nothing at propose, review, request, accept, attach or continue loads baseline requirements
  (`SaveAcceptedWorkPlanFromAuthorityDecision`, `repository.go:412-497`, never loads the baseline).
- **NEW, independent verification I performed.** I recomputed the canonical digest of the establish document
  (`/tmp/praxis-human-interface-establish.json`) by reproducing `CanonicalBytes` (sorted lists, Go HTML-escaping, field order). The result is
  exactly `afda0866398d14a223f3a090d554c820e9e945922a936e6f1edb9c06d9177530`. All 31 elements are distinct strings. The file's sha256 is
  `1881e31e9185b3c517c8a8b7eecf1aa965edcd9c8a7388883e4016136b6b8f0e` (this is what `ImportSourceDigest` should equal; see B2 below).

### Reviewer principal
- `WorkPlanProposalReview.Validate` requires only non-empty `PrincipalRef`, `ReviewedBy.ID != ProposedBy.ID`, `ReviewerGeneration != ProposerGeneration`
  (string inequality) (`work_plan.go:207-212`). `ReviewerProvider` exists and is read nowhere. `AcceptWorkPlan` never compares reviewer with accepter.
- **NEW.** When `--reviewer-id`/`--reviewer-generation` are omitted, the review defaults to the **installation owner** at the root generation
  (`goals_lifecycle.go:121-134`, ADR-097). The owner is also the only principal who can `decide`, so the default "independent" reviewer can be the decider.
- Reusable, already-authenticated machinery: `AuthorityGeneration` + `ValidateAuthorityGeneration` (`repository.go:1220-1236`), the OS-user +
  typed-confirmation review of root succession (`internal/goalstore/root_authority_succession.go:210-214`), `TurnRecord.{AgentID,ExecutorID,InvocationID}`
  and eventstore `Actor`/`Trust`. `ReviewerGeneration` already has the shape `ref/version@digest` of an authority-generation reference, but nothing parses it.

### Ontology
- SPEC-014 invariant 2 (line **23**, not 20 as the review cites): "A Goal Baseline is derived knowledge, never execution authority."
  The code labels establishment output `"status": "authoritative"` (`goals_lifecycle.go:85,95`). Reconcilable only if "authoritative" means
  *source of truth for the outcome contract*, which no document says.
- Execution authority sits in exactly two owner acts: the `workplan.accept` decision (bound to goal, version, baseline digest, proposal, review) and settlement
  (`complete`/`succeed`, ADR-099). `AttachAcceptedWorkPlan` re-checks the decision at attach.
- **NEW.** Goal **establishment** carries no owner authority: `establish` needs only `PRAXIS_DB` and `PRAXIS_BOOTSTRAP_RECORD` possession
  (`goals_lifecycle.go:76-85`, `:320-358`). Import is the same.
- **NEW.** "Immutable Intent re-request" is `AuthorityReRequestLineage` over an `ActionIntent`, not Goal intent, and `SaveAuthorityReRequest` has no production caller.
  It is not a path for changed requirements.

### Providers
- Codex today: `exec --ephemeral --sandbox workspace-write --skip-git-repo-check -C <dir> [--model M] -`. Claude today: `--print --output-format text
  --no-session-persistence --permission-mode acceptEdits --permission-prompts none --add-dir <dir> --allowedTools <write-capable list>` (`internal/goaldrive/provider_worker.go:518,534-538`).
  Only the Claude flags are frozen by a test; **NEW:** no test covers the Codex flags.
- `ProviderCLIWorker` returns only `provider-process:completed`. Stdout is a redacted line transcript (16 KiB/line) in the activity log, and a 1 MiB failure-diagnostic buffer.
  The sole structured stdout channel is `CommandWorker` (one `WorkerResult` JSON, repository-turn shaped, `command_worker.go:113-126`).
- Installed CLIs (help output read by me): `codex-cli 0.155.1` has `--sandbox read-only`, `--output-schema`, `-o/--output-last-message`, `--json`, `--add-dir` (**writable** only).
  Claude Code `2.1.278` has `-p`, `--output-format json`, `--json-schema`, `--tools ""|list`, `--disallowedTools`, `--permission-prompts none`, `--add-dir`, `--restricted`,
  `--no-session-persistence`. `--bare` requires `ANTHROPIC_API_KEY` and is unusable under the subscription-only rule.
- `ProviderWorkspaceManager.Create` has **no production caller**; it needs an accepted WorkPlan ref, digest and child objective
  (`pkg/contracts/provider_workspace.go:56-58`), does `git worktree add --detach` (`provider_workspace.go:64`), and changes `.git/worktrees/*` and the object store.
- **NEW.** `GitRepository.Snapshot` runs `git fetch` (`git_repository.go:258`), which updates remote-tracking refs and `FETCH_HEAD`. It cannot be used as a "nothing changed" probe.
- **NEW.** `authority.required` is emitted only when `SelectRunnableWork` returns `ErrNoRunnableWork` with a real `AuthorityRequest`, so a transport failure cannot become an authority
  request today (`controller.go:147-160`). Failure classes are otherwise untyped strings except `CapabilityError`.
- **NEW.** SPEC-051 routing (`internal/inference`, `internal/routingauthority`) is not imported by `internal/goaldrive` or `cmd/praxis`. Provider choice is an explicit catalog pick.

### Operational identity
- `--provider`, `--invocation-id`, exact goal id and version are mandatory (`internal/goaldrive/invocation.go:38-52`, `cmd/praxis/goaldrive.go:57-65`). Turn ids are already minted in `Admit`.
- Uniqueness is enforced per `(goal, version)` aggregate only (`admission.go:302-321`).
- **NEW.** `Admit` runs before `prepare`, so a pre-flight refusal (settled goal, no runnable work, authority required) consumes the invocation id durably and a retry returns
  `ErrInvocationReused` (`runtime.go:156,198-215`). This is a large part of why humans invent ids.
- **NEW.** Activity streams are keyed `(invocation, turn)` without the goal (`supervision.go:105`): a reused string across generations could collide.

### Successor and boundary
- `SaveReplanningSuccessor` clones the contract, sets `PredecessorDigest`, clears `WorkPlan`, appends evidence strings; it accepts no contract change (`repository.go:1413-1454`).
  `succeed` requires a settled generation (`goal_complete.go:280-281`).
- `packages/goals/invalidation.go` and `applicability.go` are used only in tests.
- **NEW.** Succession is non-atomic: the successor blob is saved and the succession event is recorded afterwards (`goal_complete.go:310,314`); a crash between them makes a retry fail
  with "already exists".
- **NEW.** `governingSuperseded` needs `Succession != nil`, so an unsettled older generation is never blocked by a newer one; nothing fences a live turn at a generation transition.
  There is no drive-time authority re-check.
- ADR-099 confirms the successor ledger starts empty and completions are read per generation (`controller.go:112-119`).

### Status and surface
- **NEW.** `InvocationSummary` (`internal/goaldrive/summary.go`) has no production caller; `goal-drive` prints the raw `TurnRecord` (`cmd/praxis/goaldrive.go:108`).
- **NEW.** Suspend/cancel are activity events on one `(invocation, turn)` stream. A new invocation never sees a prior invocation's suspension, so Goal-level pause is not derivable.
  Goal listing and latest-generation resolution do not exist.
- Package `praxis.package.goals@0.1.4`. Manifest invocations are only lifecycle and `goal-drive` (`packages/goals/package.go:42`). The `operation` help omits `evaluate`, `complete`,
  `succeed`, `decide`, and option descriptions are mis-scoped (`--status`, `--reason`, `--goal-id`). **NEW:** the static help catalog has no `goals-lifecycle`/`goal-drive` entry; the `goals`/`design`
  invocation is defined but unregistered; there is no `praxis goal`; docs still say `praxis goal import` (SPEC-031:5, ADR-068:8, PLAN-003:222).
  Installed `~/bin/praxis` is build `f539b74` (`modified=false`, 2026-09-19) which already has 0.1.4 but lacks the #182 fix in HEAD. The installed **package generation** in `~/.praxis/praxis.db`
  was not read, so package/help skew there is unverified.

---

## 3. Mandatory questions, answered

### Q1. Requirement identity and bootstrap integrity
Design goal: proposal v2 is defensible with **existing** fields and **independently recomputable** evidence, and the plan does not depend on the fix landing.

**Semantic identity.** An element is addressed by `(kind, sha256(verbatim UTF-8 text))`. That is order-independent, digest-stable, survives reordering, and is reusable across generations
(the property evolution needs). Duplicate texts within a list are ambiguous and rejected by the resolver.

**Requirement reference format (no schema change)**

| field | value in v2 | why it validates today |
|---|---|---|
| `id` | `<kind>:sha256:<64-hex of element text>` | any non-empty string; **it is the sole key of review coverage**, so coverage becomes reorder-safe |
| `source_ref` | `goal:praxis-human-interface/1#<kind>/<stored-index>` | keeps the v1 convention that `completion.go` parses; index is **stored/source order** |
| `source_digest` | `sha256:<64-hex of element text>` | any non-empty string; no consumer reads it |

`sameRequirements` compares whole structs, so acceptance preserves these exactly. Baseline binding stays at proposal level (`BaselineDigest`).

**Independently verifiable evidence (the element manifest).** A committed evidence document (`docs/research/dogfood/praxis-human-interface/goal-baseline-elements.json`, produced by
the bootstrap steps, not by this plan) lists for each of the 31 elements: kind, stored index, canonical index, verbatim text, text sha256. It also carries the recomputed canonical baseline digest,
the establish-file sha256, and the count. Every value is recomputable from the establish document with no Praxis code (Section 4).

### Q2. Reviewer principal
Caller-supplied ID and generation strings are **not** sufficient (Section 2). The smallest coherent boundary, reusing existing records and creating no registry:

1. `ReviewerGeneration` becomes a **resolvable** reference. Human reviewers resolve through the existing `AuthorityGeneration` (`ValidateAuthorityGeneration`, principal equality).
   Non-human reviewers resolve to a durable advisory-invocation provenance record (provider, CLI version, invocation/turn ids, envelope digest, evidence-manifest digest, result digest)
   written by the advisory intake unit. Independence is judged over the **resolved** provenance: distinct principal, distinct generation digest, distinct invocation.
2. `ReviewDigest` is extended (additive, versioned so historical digests stay valid) to cover findings, coverage and reviewer provenance.
3. Coverage is computed against the **baseline's** requirement set, not the proposal's own claim (delivered by the identity unit).
4. The owner-default reviewer stays available as an explicit human review, recorded as such. Whether it satisfies `review_all` for a model-authored plan is left to the existing recommendation-mode semantics
   (see uncertainty U-3).

For proposal v2 itself, until this lands: reviewer provenance evidence is attested in the review evidence document and pinned by digest (Section 4, B4-B6).
That is convention, stated honestly, and the plan does not claim enforcement.

### Q3. Human ontology (no new first-class ontology)
Not asserting GoalBaseline is the sole authority-bearing artifact. Reconciled model, to be ratified or corrected by the ontology unit:

| Human word | What it is in Praxis | Authority? |
|---|---|---|
| Goal | logical id + immutable generations | none by itself |
| Goal generation (`GoalBaseline`) | digest-addressed **authoritative state**: the outcome contract. SPEC-014 inv. 2: never execution authority | records what is authoritative; grants no right to act |
| Requirement | an element of a generation (criterion, constraint, non-goal, assumption); addressed by content identity | none; traceability only (`work_selection.go:7-9`) |
| Story / Intent (human sense) | the human source document, preserved and digest-bound as **evidence** | none |
| WorkPlan | accepted decomposition attached to a generation | the acceptance decision is an authority act |
| Work unit | `WorkCandidate` in an accepted plan; completion is a per-generation ADR-099 record | derived |
| Feature / Task | UI words only; no record type | none |

Two authority events surface to a human: **plan acceptance** and **settlement**. Two open questions the ontology unit must decide rather than assume:
(a) whether **Goal establishment** needs an owner confirmation (today it is key possession only), which matters because SC3 says the user inspects what Praxis understood "before or as authority is established";
(b) the wording fix ("authoritative" in output/docs versus SPEC-014 invariant 2).
No new authority kind is introduced by this plan.

### Q4. Advisory workspace vs artifact intake
The split is **correct, with two adjustments** from the code:
- With a controller-consumed stdout transport there is **no writable output directory** and **no worktree**, so the workspace half shrinks to a controller-staged read-only **evidence bundle**, and `ProviderWorkspaceRecord`
  (which requires an accepted WorkPlan, `provider_workspace.go:56-58`) is deliberately **not** reused.
- The advisory **admission** (a turn that is not a `TurnRecord`) belongs with intake, because it decides how a result becomes durable evidence. It is the piece the reviewer did not name.

Truthful predicates (replacing "byte-identical repository"):

| Predicate | Definition | Notes |
|---|---|---|
| Authoritative checkout untouched | `HEAD` equal; `git status --porcelain --untracked-files=all` empty; `ConsequenceFingerprint` (`execution_contract.go:294-335`) equal before and after | run with `--no-optional-locks`; never call `GitRepository.Snapshot` (it fetches) |
| Refs stable | `for-each-ref` digest **excluding** `refs/remotes/*` and `FETCH_HEAD` equal | remote-tracking refs may move for unrelated reasons |
| Advisory isolation | staged tree lives outside the authoritative checkout; only controller-chosen files, each in a durable manifest with sha256; modes 0400/0500; paths confined by `plugins/workspace` `ResolveAuthorizedPath` | |
| If a worktree is ever used | allowed metadata delta is exactly the `.git/worktrees/<id>` entry and new objects; removal must restore the pre-state set | not needed for this design |

### Q5. Provider advisory transport
Two transports, one proven implementable now:

- **T1 (required, universally implementable): inline evidence, no tools, controller-consumed stdout.** The controller renders the evidence bundle into the prompt on stdin and consumes one result on stdout.
  Claude: `--print --output-format json --json-schema <inline schema> --no-session-persistence --permission-prompts none --tools ""` (no `acceptEdits`, no `--allowedTools`, no `--add-dir`).
  Every flag exists in the installed `2.1.278` help and all but `--tools` and `--json-schema` are already in Praxis's vocabulary. The controller validates the result against the schema itself, so the
  provider's structured-output feature is a convenience: if it misbehaves, the single JSON `result` string is parsed and validated identically. No filesystem grant of any kind exists to fail.
- **T2 (optional, gated): staged read-only tree.** Same launch with `--tools "Read,Glob,Grep"`, `--add-dir <stage>`, `cmd.Dir=<stage>`, optionally `--restricted`. Enabled only after a recorded live probe passes for
  that exact CLI version.
- **Codex** can be launched `exec --ephemeral --sandbox read-only --skip-git-repo-check -` with the final message consumed from stdout (or `-o` to a controller-owned path). It gives write denial but **no read
  confinement**, and a two-path design (read-only tree + writable dir) is refuted (`--add-dir` is writable-only). Codex is therefore classified **unsupported for advisory transport** until a probe
  passes and a declared weaker-confinement policy is accepted, and serves as a live demonstration of fail-closed behaviour.
- **Fail closed as environment/capability, never authority.** A new advisory capability and evidence class modelled on `CapabilityError`/`capability.unsatisfiable`: reasons `binary_missing`,
  `flag_unsupported`, `no_passing_probe`, `result_invalid`, `timeout`. These become blocked/`environment` dispositions and can never create an `AuthorityRequest`.
- **Gating probe (first task of the transport unit, recorded as durable evidence):** one non-mutating session per provider against a scratch fixture; support is a property of a passing probe for
  *this* CLI version and flag set. No probe was run by this plan; nothing here claims live behaviour.

### Q6. Operational identity
- **Single-use is preserved and strengthened.** The ADR-100 invariant "a durable invocation identity is single-use" stands. Praxis mints the identity **inside the existing admission compare-and-set**
  as `(goal, generation, attempt counter)` and records an `invocation.allocated` event before any launch. The id is never derived from human inputs and is never reusable.
- **Replay/recovery for one attempted invocation.** An allocation is terminal or non-terminal. On restart with a non-terminal allocation: live lease means refuse (concurrent); expired/lost lease means the existing
  `ReconcileLostTurn` path; allocated-but-never-admitted means deterministically abandon. Every retry mints a **new** attempt. Because humans never see or type ids, "burning" one on a pre-flight refusal is harmless.
- **Fixes** the cross-generation uniqueness gap (goal+version in the id and in the activity-stream key).
- **Operators unchanged:** explicit `--invocation-id`/`--provider` still work and stay single-use.
- **Provider routing is a separate unit.** Deterministic ordered-preference policy over the existing catalog and probe evidence; recorded decision; human asked only on a material cost/capability/privacy/policy/authority
  consequence; operators keep explicit control. SPEC-051/#102 routing is **not** pulled in (it is not wired to goal-drive); using it is a later, separate step.

### Q7. Requirement evolution and successor semantics
- Predecessor generations stay immutable. Successors are new generations created by a new **changed-contract** successor transition (extends `SaveReplanningSuccessor`, adds atomic succession).
- Nothing is copied into the successor's completion ledger (ADR-099). Predecessor consequences produce **applicability/reuse evidence records** keyed by element identity, citing the predecessor completion digest and the
  consequence fingerprint. The successor truthfully establishes its own executable state: its plan is proposed from the successor contract; a candidate may cite applicability evidence; the successor's own completion
  requires its own ADR-099 act (see uncertainty U-5 on whether a "re-verified against successor head" completion basis needs an ADR-099 amendment).
- Provider-owned work at the boundary: a **generation-transition fence** on the outgoing generation refuses new admissions, lets a live turn run to its own disposition (never rewritten), reconciles lost turns through the
  ADR-100 path, and only then does the successor become governing. The cross-generation scope lease already serialises the checkout.

### Q8. Human flow vs status/control
Split, and status is **not** made a consumer-blocker of flow. Status derives only from durable records: governing state, admissions and leases, turn records, pending authority, completion assessment,
evaluation chain and settlement decisions, plus a new Goal-scoped control record for pause/resume. Where evidence is absent it says `unknown`. No percentage, no invented frontier, no Goal completion
except from a settlement decision. "Known frontier" is only plan work-set state (`blocked_by`, next unit, incomplete units).

### Q9. Installed surface coherence
Preserved as substantive. Split into an early, independent **conformance harness plus existing-operation fixes** and a late **packaging/installation of the new human surface**. Package is currently `0.1.4`;
the plan requires a version cadence (one signed successor per contract change, planned once) and forbids any unit from using an option not declared in the installed manifest.

### Q10. Final dogfood
Section 9.

---

## 4. Proposal-v2 bootstrap-integrity strategy

**Principle:** v2 must be trustworthy under today's code. Nothing below waits for any unit in the plan. All steps are performed by the human/controller side, not by this planner.

| Step | Action | Independent of |
|---|---|---|
| B0 | Freeze evidence in the repo: this plan (digest), `STORY.md` (5c62ba3c…), v1 proposal, v1 review, establish document copy | the planner |
| B1 | Deterministic script, no Praxis code: parse the establish document, recompute the canonical digest (sorted lists + Go escaping) and require `sha256:afda0866…7530`; emit the element manifest (kind, stored idx, canonical idx, text, text sha256); assert 31 elements, all distinct | Praxis |
| B2 | Stored-order attestation using **read-only** `goals-lifecycle --operation=inspect` for `praxis-human-interface/1`: stored lists equal manifest stored order and `ImportSourceDigest == sha256:1881e31e…` (the establish file). If not equal, regenerate the manifest from stored order. This is what makes `#<kind>/<n>` true | the planner |
| B3 | Materialize proposal v2 **from the manifest by script**: every `RequirementRef` is emitted from a manifest row keyed by candidate binding labels; the planner never types an id, digest or index. Script asserts every one of the 31 element ids appears in at least one candidate and no id outside the manifest appears | the planner |
| B4 | Independent verifier (different principal **and** generation from the proposer; not the planner) re-runs B1-B3 with separately written code, checks each candidate's bindings against the plan's per-unit table, and writes the review evidence document (verdict, coverage 31/31, invented scope none, reviewer provenance: provider, CLI version, session identity, timestamp) | planner |
| B5 | Submit the review through the **full-document** `review` path (`--input` with a `review` key): `ReviewDigest = sha256(review evidence document)`, `Findings` carry the verdict summary, `CoveredRequirements` equal the proposal's ids. The selector path is **not** used: its coverage is tautological and its digest ignores findings | selector defect |
| B6 | Create the authority request with `--reason` quoting: manifest digest, review-evidence digest, "31/31 baseline elements covered", and the reviewer provenance line. The `Reason` is digest-covered and printed to the human at `authority decide` | |
| B7 | After the identity unit lands: replay the resolver over the accepted v2 plan. A mismatch is a finding against v2, but v2's acceptance never depended on this | |

**Why v2 does not rely on the defect:** every requirement binding is verified by recomputation against evidence the reviewer regenerates independently, and the acceptance decision sees the evidence digests
in the request it signs. **What remains convention, not enforcement:** the reviewer's independence (string ids + attested provenance) and the stored-order attestation. Both are named in the remaining-uncertainty list.

### Element manifest (31 elements, recomputed by me)
Labels below are stored (source) order, used as `SC<n>/C<n>/N<n>/A<n>` throughout this plan. Full text digests are in the manifest; the first 16 hex are shown.

| label | source_ref path | canonical idx | text sha256 (16) | text (start) |
|---|---|---|---|---|
| SC1 | success_criteria/1 | 3 | `871db6ddfeb32bef` | An ordinary user can provide a human-readable description of a desired |
| SC2 | success_criteria/2 | 8 | `575aa0617d91a02d` | Praxis preserves the exact human source artifact and cryptographically |
| SC3 | success_criteria/3 | 12 | `6c9c0e4e70a6f981` | The user can inspect what Praxis understood before or as authority is |
| SC4 | success_criteria/4 | 6 | `134f9b4a3d05862e` | Praxis asks the human only for material ambiguities or decisions that |
| SC5 | success_criteria/5 | 7 | `01af2520261be7c0` | Praxis can establish governed execution from the human-facing interact |
| SC6 | success_criteria/6 | 11 | `37c099abb67bb49b` | The human-facing interface supports authoritative requirement evolutio |
| SC7 | success_criteria/7 | 9 | `fda39ebd8cb1b90c` | Requirement evolution performs deterministic impact analysis and prese |
| SC8 | success_criteria/8 | 2 | `917090a0601b8a56` | After governed reconciliation of changed human requirements, Praxis ca |
| SC9 | success_criteria/9 | 1 | `52e160cb9d9a017d` | A normal user can determine whether Praxis is working, what outcome is |
| SC10 | success_criteria/10 | 5 | `5a76cfb5525bed88` | Operator and forensic interfaces retain access to exact Goals, generat |
| SC11 | success_criteria/11 | 4 | `c06d727f9262fbf1` | Capabilities present in canonical Praxis source are truthfully exposed |
| SC12 | success_criteria/12 | 10 | `19655326d87dde34` | The delivered interface is dogfooded by using it for the next real hum |
| C1 | constraints/1 | 6 | `d285832cbf58c2f2` | Preserve deterministic authority boundaries and immutable historical e |
| C2 | constraints/2 | 7 | `a58bb350f837d13d` | Preserve exact provenance between human-readable source input and Prax |
| C3 | constraints/3 | 4 | `71fd5d58200329b6` | Model interpretation must not manufacture human authority, requirement |
| C4 | constraints/4 | 3 | `f6e4a731a1c0598a` | Material ambiguity must be surfaced to the human as a focused conseque |
| C5 | constraints/5 | 1 | `de0abab884b3bd63` | Existing qualified consequences must remain valid historical product r |
| C6 | constraints/6 | 8 | `b541c61566e5f20c` | Provider-owned work must not be silently rewritten or destroyed when h |
| C7 | constraints/7 | 9 | `66ddabc3dfce4926` | Reuse existing Praxis establishment, GoalBaseline, GoalStore, WorkPlan |
| C8 | constraints/8 | 10 | `8d9ab62714dadafc` | Simplifying the ordinary human interface must not remove operator or g |
| C9 | constraints/9 | 2 | `4fbd589979cecb33` | Installed package manifests, help, dynamic entry points, implementatio |
| C10 | constraints/10 | 5 | `37d37301b658dabc` | Operational identifiers that do not represent meaningful human choices |
| N1 | non_goals/1 | 4 | `5e7e3cd6a9d694d2` | Remove or weaken Praxis deterministic governance, authority, provenanc |
| N2 | non_goals/2 | 2 | `01eab588ea27b86c` | Hide low-level governance and forensic interfaces from operators or au |
| N3 | non_goals/3 | 3 | `9f2ae1d208e3fb2d` | Predetermine whether Story, Intent, Requirement, Goal, Feature, or Tas |
| N4 | non_goals/4 | 1 | `02aba9007f124bd3` | Create a parallel governance system solely to simplify the CLI. |
| N5 | non_goals/5 | 5 | `eb3b51a880e2ee07` | Require the human-facing interface to use any particular command names |
| A1 | assumptions/1 | 4 | `edde7ee1ae6a2d34` | The existing deterministic Praxis lifecycle is substantially correct a |
| A2 | assumptions/2 | 1 | `c57ca96b9d55cf0b` | Human-readable Markdown is an appropriate initial human input format b |
| A3 | assumptions/3 | 3 | `3a5a75b05551868a` | The correct relationship among Story, Intent, Requirement, and Goal re |
| A4 | assumptions/4 | 2 | `d5fc1b867511b588` | Provider routing may remain explicitly controllable by operators even  |

Candidate `source_ref`/`source_digest`: the manifest path and its sha256 (proposal-time evidence; Praxis rewrites these to the authority request on acceptance).
Relationship `source_ref`/`source_digest`: this plan's committed copy and its sha256.
`proposer_generation`: `claude-sonnet-5/advisory-plan@sha256:<digest of the committed v2 plan>`.

---

## 5. Candidate set (18)

Selection rule reminder: lowest `(priority, sequence)` among candidates whose **hard** prerequisites are complete; ties fail closed, so every sequence is unique.
All candidates are `completed:false`, `provenance: model_proposal`. Every candidate lists its bound elements as manifest-derived `RequirementRef`s (Section 3 Q1).

| p / seq | Candidate id | v1 lineage |
|---|---|---|
| 1 / 1 | `unit:hi-requirement-identity-and-binding` | v1 U02, rescoped |
| 1 / 2 | `unit:hi-ontology-and-surface-contract` | v1 U01, rescoped to a decision |
| 1 / 3 | `unit:hi-operational-identity-allocation` | v1 U03 split (half) |
| 2 / 4 | `unit:hi-advisory-evidence-staging` | v1 U04 split (half) |
| 2 / 5 | `unit:hi-advisory-result-intake-and-recovery` | v1 U04 split (half) + advisory admission |
| 2 / 6 | `unit:hi-review-principal-provenance` | **new** (review finding 2) |
| 2 / 7 | `unit:hi-surface-skew-conformance` | v1 U12 split (half) |
| 3 / 8 | `unit:hi-provider-advisory-transport` | v1 U05, rescoped |
| 3 / 9 | `unit:hi-provider-routing-policy` | v1 U03 split (half) |
| 3 / 10 | `unit:hi-source-bound-goal-establishment` | v1 U07 |
| 4 / 11 | `unit:hi-governed-planning-orchestration` | v1 U06 |
| 4 / 12 | `unit:hi-model-assisted-interpretation-and-ambiguity` | v1 U08 |
| 5 / 13 | `unit:hi-requirement-evolution-classification-and-successor` | v1 U09 |
| 5 / 14 | `unit:hi-evolution-execution-boundary` | v1 U10 |
| 5 / 15 | `unit:hi-human-lifecycle-flow` | v1 U11 split (half) |
| 5 / 16 | `unit:hi-status-and-control-projection` | v1 U11 split (half) |
| 6 / 17 | `unit:hi-installed-human-surface-packaging` | v1 U12 split (half) |
| 7 / 18 | `unit:hi-dogfood-acceptance` | v1 U13 |

Split/merge/remove summary: **split** U03, U04, U11, U12; **rescoped** U01, U02, U05; **added** `hi-review-principal-provenance`; **merged** none; **removed** none.

Each block: responsibility · bound elements · existing machinery reused · qualification. "Advisory-only" is implicit everywhere.

### 1. `hi-requirement-identity-and-binding`  (SC5 SC7 SC8 C1 C2 C3)
- **Responsibility.** Derive stable element identity `(kind, text digest)` from a generation; a resolver that maps any `RequirementRef` to a real element of the exact baseline and rejects unresolved, mismatched
  (`id`/`source_ref`/`source_digest` disagree) or ambiguous (duplicate text) refs; proposal-time verification in `BuildWorkPlanProposal`; review coverage computed against the **baseline** element set;
  completion coverage by element identity with the legacy positional `#success_criteria/<n>` accepted for old plans; no change to any existing baseline digest.
- **Reuses.** `packages/goals/canonical.go`, `baseline.go`, `pkg/contracts/work_selection.go`, `work_plan.go`, `internal/goaldrive/completion.go`, `goalstore.Repository.Load`.
- **Qualification.** Table tests: resolve/fail-closed. Reordered-list fixture keeps ids stable. Historical baselines replay with unchanged digests. Invented requirement rejected. Proposal omitting a real element flagged. Review coverage
  cannot be tautological. **Replays proposal v2's own accepted refs** (B7). Legacy positional plans still complete.

### 2. `hi-ontology-and-surface-contract`  (N3 N4 N5 A1 A2 A3 C1 C7 C8)
- **Responsibility.** ADR+SPEC decision: the Q3 table ratified or corrected; the "authoritative state vs execution authority" wording reconciled with SPEC-014 inv. 2; the decision on Goal establishment
  (owner confirmation or explicitly non-authoritative provenance); the three-surface map (human/operator/forensic) with an identifier-visibility matrix; the human vocabulary.
  Creates **no** new authority kind or store. Corrects the stale `praxis goal import` references' meaning.
- **Reuses.** SPEC-006/014/023/054, ADR-002/060/064/096/097/099, `packages/goals/baseline.go`.
- **Qualification.** Doc consistency check against SPEC-014, 023, 054, ADR-060, 064, 099; a conformance test that human-surface renderings contain no digest or generation number unless asked.

### 3. `hi-operational-identity-allocation`  (SC5 C1 C7 C8 C10)
- **Responsibility.** Praxis-minted, single-use, durably allocated invocation identity (Q6); pre-flight refusals no longer strand the operator; goal+generation-scoped identity and activity streams; default turn limits;
  explicit operator flags unchanged.
- **Reuses.** `internal/goaldrive/admission.go` (`Admit`, event-store CAS), `ReconcileLostTurn`, `--recover-turn` (ADR-098), `runtime.go`, supervision `ActivityLog`.
- **Qualification.** Cross-process race (`admission_red_test.go` style): two allocators never mint the same id. Restart with each non-terminal allocation state yields the specified deterministic disposition and **never reuses an id**.
  Identical human inputs in two attempts yield two distinct ids. Cross-generation string collision impossible. Explicit `--invocation-id` behaviour unchanged.

### 4. `hi-advisory-evidence-staging`  (SC5 C1 C2 C7)
- **Responsibility.** Controller-staged, read-only evidence bundle **outside** the authoritative checkout: exact Goal generation and complete baseline digest, element manifest, human source artifact, repository evidence at the exact HEAD,
  each item with sha256 in a durable manifest; path confinement; cleanup; the Q4 cleanliness predicates. Consumes an existing explicit invocation identity (no dependency on automatic allocation).
- **Reuses.** `plugins/workspace` `ResolveAuthorizedPath`, `EvidenceRef`, `BuildContextPack`; `SecureBlob` namespace in goalstore (new namespace, same pattern as `putWorkPlanBlob`); `ConsequenceFingerprint`.
- **Qualification.** Fixture tree; symlink/traversal/oversize items rejected; manifest digests tamper-evident; authoritative-checkout predicates hold before and after; no `git fetch`; cleanup never removes non-clean state.

### 5. `hi-advisory-result-intake-and-recovery`  (SC5 SC8 C1 C2 C3 C7 C10)
- **Responsibility.** Advisory turn admitted through `Admit` with a distinct scope key and **no** `TurnRecord`; a narrow result-source interface (stdout bytes plus exit metadata) implemented by fixtures here and by providers in the transport unit;
  dedicated capture buffer (not the redacted transcript), size-limited, truncation-rejecting, trailing-JSON-rejecting; schema validation; a durable provenance record binding invocation, turn, provider, CLI version, planner generation,
  evidence-manifest digest and result digest; typed environment failure classes that cannot become an `AuthorityRequest`; crash/lost-lease recovery through ADR-100 reconciliation.
- **Reuses.** `Admit`, `CommandWorker`'s one-object stdout discipline (`command_worker.go:113-126`), `CapabilityError` pattern, `ActivityLog`, `SecureBlob`, `ReconcileLostTurn`.
- **Qualification.** Fixture result source: valid, truncated, trailing garbage, oversize, wrong schema, hostile content. Each failure class is an `environment` disposition and creates no authority request. Restart mid-turn reconciles. Result stored as
  `TrustUntrusted` (ADR-040). Provenance links all four digests.

### 6. `hi-review-principal-provenance`  (SC5 C1 C3 N1)
- **Responsibility.** Q2: resolvable reviewer/proposer provenance; independence over resolved provenance; `ReviewDigest` v2 covering findings, coverage, provenance (versioned, historical digests intact); explicit disposition when the
  reviewer is the installation owner; `ReviewerProvider` finally populated; reviewer-versus-accepter comparison.
- **Reuses.** `AuthorityGeneration`, `ValidateAuthorityGeneration`, `LoadAuthorityGeneration`, `root_authority_succession.go` review pattern, `TurnRecord`/eventstore actor, `WorkPlanProposalReview.Validate`.
- **Qualification.** Self-review, same-generation review, forged generation string, unresolvable generation, reviewer equal to proposer via a second provenance path, and owner-as-both each fail closed with the specified reason. Historical review digests verify unchanged. Replays
  the v1 review evidence and proposal v2's review as fixtures.

### 7. `hi-surface-skew-conformance`  (SC10 SC11 C8 C9 N2)
- **Responsibility.** Conformance harness and existing-operation fixes: help and manifest for `evaluate`, `complete`, `succeed`, `decide`; correct option scoping (`--status`, `--reason`, `--goal-id`); static help entries for `goals-lifecycle` and
  `goal-drive`; installed-versus-source skew detection (installed package generation and binary revision versus source); stale-doc sweep (`praxis goal import`); a test that every dispatched operation is documented and every advertised command runs.
- **Reuses.** `packages/goals/invocation.go`, `package.go`, `cmd/praxis/cli_help.go`, `Registry.Resolve`, `emitted_command_contract_test.go`, `cli_help_installed_test.go`, `goals_package_surface_test.go`.
- **Qualification.** Through the real activation path. A stale installed manifest against a newer binary is detected and reported truthfully. Explicit-flag behaviour of `goals-lifecycle`, `inspect`, `supervise`, `goal-drive` is unchanged.

### 8. `hi-provider-advisory-transport`  (SC5 C1 C3 C7 C8 N1)
- **Responsibility.** New advisory capability class; **T1** no-tools inline-evidence stdout transport for Claude as the first supported provider; optional **T2**; Codex read-only profile classified per Q5; recorded per-CLI-version probe evidence; typed fail-closed
  when no passing probe; the advisory launch profile recorded as `execution.envelope` before start; never passes `acceptEdits`, write tools, `--bare`, or any bypass flag.
- **Reuses.** `internal/goaldrive/provider_worker.go`, `execution_contract.go` (`WorkerCapability`, launch-flag test pattern), `cmd/praxis/providers.go` catalog, `redactProcessOutput`.
- **Qualification.** Launch-flag tests for **both** providers (Codex currently has none). Helper-process fake CLIs assert exact flags and env, and that no bypass or write flag is passed. Gating probe result recorded. Unsupported provider yields a typed environment failure.
  Real-provider T1 run is part of unit qualification and reappears in the final dogfood.

### 9. `hi-provider-routing-policy`  (SC4 SC5 C7 C8 A4)
- **Responsibility.** Deterministic ordered-preference policy over catalog availability and transport-probe support; the chosen provider and the policy that chose it recorded; the human is asked only when the choice has material
  cost/capability/privacy/policy/authority consequence, expressed as a typed decision; operators keep explicit provider control.
- **Reuses.** `goalDriveProviderCatalog` (`cmd/praxis/providers.go`), `ExecutionTarget`/`ExecutorSurface` as a vocabulary only (not wired), decision-request pattern of ADR-097.
- **Qualification.** Same catalog yields the same choice; explicit `--provider` always wins; material-choice fixture reaches the human, non-material never does; a provider without a passing probe is not selectable for advisory work.

### 10. `hi-source-bound-goal-establishment`  (SC1 SC2 SC3 SC5 C2 C7 A2)
- **Responsibility.** The model-free deterministic path from a human Markdown file (a documented deterministic profile) to an established generation: exact source bytes persisted (new secure-blob namespace), source digest bound into the **canonical**
  digest of new generations (as an `ArtifactRef` role, so it is inside the digest), derived generation number, inspectable "what Praxis understood" rendering with per-element provenance (supplied / normalized), restart and replay safe, replay conflicts
  routed to evolution rather than failing on byte-format differences.
- **Reuses.** `establishGoalBaseline`, `GoalBaseline.Artifacts`, `ImportSource*`, `goalstore.Repository`, `SecureBlob`.
- **Qualification.** Source bytes recoverable and digest-verified; exact replay idempotent; reformatted-but-equal source detected as equal semantics; different content under the same identity routes to evolution; model-free run passes; output never contains a
  generation, digest or id the user supplied. Existing historical baseline digests unchanged.

### 11. `hi-governed-planning-orchestration`  (SC5 SC8 SC10 C1 C3 C7 C10 N4)
- **Responsibility.** At `planning_required`, Praxis runs a planner through staged evidence, admission, transport and intake; validates output with the identity unit's verifier; persists the proposal via the existing `propose` path; runs the independent reviewer as a
  distinct, provenance-bound principal; hands the result to the existing `continue`/`request`/`decide`/`accept`/`attach` chain; invalid or uncovered output goes to `planning_revision_required`. The planner never supplies proposal id, generation or provenance.
  The evidence manifest defines an optional evidence-class slot for later applicability evidence.
- **Reuses.** `goals-lifecycle continue` (SPEC-054), `BuildWorkPlanProposal`, `MaterializeAcceptedPlanCandidate`, authority requests/decisions (ADR-097), goalstore, and units 1, 4, 5, 6, 8.
- **Qualification.** Fixture-driven end to end from baseline to `drivable` with **no hand-authored JSON**. Invalid, uncovered, self-reviewed, same-generation-reviewed and forged-provenance outputs each fail closed. A planner claim of authority is inert. Restart/replay reproduces the same
  request and successor without duplicating a mutation. Real-provider run reappears in the final dogfood.

### 12. `hi-model-assisted-interpretation-and-ambiguity`  (SC1 SC3 SC4 C3 C4)
- **Responsibility.** A model proposes a candidate interpretation of expressive human input through the advisory transport; stored as non-authoritative evidence; every element tagged supplied / normalized / inferred; inferred **material** elements need typed human
  confirmation (the ADR-097 pattern); ambiguity asked as a consequence-stated question; source text treated as data; interpretation is optional and provider-transport agnostic.
- **Reuses.** Establishment (unit 10), the advisory stack (units 4, 5, 8), `Decisions` with `HumanRequired`/`AutoAccepted` validation (`baseline.go:111-115`), ADR-040.
- **Qualification.** Adversarial corpus (source text containing instructions) cannot create a requirement, constraint, risk acceptance or authority; every material inferred element requires confirmation; non-material normalization asks nothing; a model outage leaves the deterministic path intact.

### 13. `hi-requirement-evolution-classification-and-successor`  (SC6 SC7 C1 C2 C5 C7)
- **Responsibility.** "This is now a requirement": a deterministic (model-free) classification against the current generation by element identity into {entailed, non-material clarification, material change, inconsistent, invalidating, needs-successor}; impact analysis
  (wires `invalidation.go` and `applicability.go`); a changed-contract successor transition with atomic succession; applicability/reuse evidence records (never reattributed completion, Q7); an ADR-099 amendment stating the successor-completion basis.
- **Reuses.** `SaveReplanningSuccessor`, `succeed`, `packages/goals/invalidation.go`, `applicability.go`, ADR-060/064/099, unit 1 identity, unit 10 source binding, unit 12 as an optional input.
- **Qualification.** A fixture per taxonomy class. Predecessor generation bytes and digests unchanged. Successor completion ledger starts empty; applicability evidence cites predecessor completion digests; a reordered but semantically unchanged list yields "entailed". Crash between blob save and
  succession record repairs deterministically. Works with no model.

### 14. `hi-evolution-execution-boundary`  (SC6 C1 C5 C6)
- **Responsibility.** The Q7 fence: outgoing-generation admission fence; live turns finish untouched; lost turns reconcile; supersession of an unsettled generation by a change (the disposition kind is decided in the ADR amendment); drive-time authority re-check; successor becomes governing only after
  quiescence.
- **Reuses.** ADR-100 admission and lease machinery (`admission.go`, `ReconcileLostTurn`), `governing_state.go`, supervision, unit 13.
- **Qualification.** A provider-owned turn in flight is neither rewritten nor destroyed and its result lands in its own generation's ledger. A crash on either side of the boundary replays deterministically. The scope lease still serialises the checkout. Older drivable generation is refused after supersession.

### 15. `hi-human-lifecycle-flow`  (SC4 SC5 SC8 C10 N4)
- **Responsibility.** One human flow composing establish, plan, review, decide, attach, drive, and evolve using generated identities. Decisions surface only where the ontology says a human authority is required, in plain language, routed to the existing interactive
  `authority decide` (typed confirmation and OS-user check preserved), with the exact command shown so no digest is carried by hand.
- **Reuses.** Units 2, 3, 10, 11; `goals-lifecycle continue`; `authority pending|decide` (ADR-097); goal-drive.
- **Qualification.** A human transcript from Markdown to `drivable` contains no digest, generation number, invocation id or proposal id; each prompt links to exact operator evidence; only material items reach the human; restart mid-flow resumes without duplicating a mutation.

### 16. `hi-status-and-control-projection`  (SC9 SC10 C8 N2)
- **Responsibility.** Q8: a status projection over durable records; wires `SummarizeTurn`/`InvocationSummary`; Goal-level pause/resume control record visible across invocations; latest-generation resolution; explicit `unknown`; "complete" only from a settlement decision;
  invocation termination distinguished from completion. Every item links to exact operator evidence.
- **Reuses.** `governing_state.go`, `inspect`, `summary.go`, supervision `ActivityLog`, ADR-099 evaluation chain and settlement, pending-authority projection.
- **Qualification.** Golden projections: active, allowed-to-continue, paused, blocked, recovering, authority-required, invocation-terminated versus complete. No fixture may show a percentage, frontier or Goal completion that the durable records do not contain.
  Pause set by one invocation is honoured by the next.

### 17. `hi-installed-human-surface-packaging`  (SC11 C8 C9 N2 N5)
- **Responsibility.** Register the new human entry points in a package successor (names are illustrative, N5), declare every option, update help, plan the version cadence, run installation, and prove installed == source through unit 7's harness.
- **Reuses.** Unit 7 harness, `packages/goals/package.go`, package publication/installation lifecycle (ADR-096), `Registry`.
- **Qualification.** Real activation path; every advertised command executes; installed manifest, help, dynamic entry points and source agree; explicit operator flags unchanged.

### 18. `hi-dogfood-acceptance`  (SC1-SC12 C3 C5 C6 C8 C9)
- **Responsibility.** Qualification pack plus the dogfood run in Section 9. No mocks for persistence, cryptography or containment (PRAXIS2 rule).

---

## 6. Relationships (61)

Notation `dependent → prerequisite`. Numbers are sequence numbers. Kinds: **hard** = must be complete before the dependent can be truthfully implemented or qualified; **consumer** = dependent uses the prerequisite's output but can be built and
qualified against fixtures; **interaction** = shared contract to settle jointly, no completion order required; **advisory** = informative only.

### Hard (18) with rationale
| # | dependent → prerequisite | why it must be complete first |
|---|---|---|
| 1 | 11 planning → 1 identity | orchestrated proposals cannot be validated or reviewed truthfully without baseline-resolved refs and baseline-derived coverage |
| 2 | 11 planning → 6 review-principal | an automated review is meaningless unless its principal and independence are durably bound |
| 3 | 11 planning → 5 result-intake | there is no durable, typed path for a planner result without it |
| 4 | 11 planning → 4 evidence-staging | the planner must receive exact evidence with no human transport (Story finding) |
| 5 | 12 interpretation → 10 establishment | the confirmed interpretation has nowhere to land except source-bound establishment |
| 6 | 13 evolution → 1 identity | semantic diff and applicability need stable element identity |
| 7 | 14 boundary → 13 evolution | the fence has no successor to hand over to without the successor transition |
| 8 | 15 flow → 11 planning | the flow's `planning_required` stage does not exist without it |
| 9 | 15 flow → 10 establishment | the flow's entry step |
| 10 | 15 flow → 3 op-identity | otherwise the human must still supply ids (violates SC5, C10) |
| 11 | 15 flow → 2 ontology | which authority events surface to a human, and the establishment-authority decision, are ontology decisions |
| 12 | 17 packaging → 15 flow | cannot advertise a command that does not exist |
| 13 | 17 packaging → 16 status | same |
| 14 | 17 packaging → 7 skew-conformance | packaging is qualified with that harness |
| 15 | 18 dogfood → 17 packaging | the run is on the installed surface |
| 16 | 18 dogfood → 14 boundary | requirement evolution during execution must be demonstrated |
| 17 | 18 dogfood → 12 interpretation | SC4 and "model cannot manufacture authority" are demonstrated only with an interpreter |
| 18 | 18 dogfood → 8 transport | a real supported advisory transport must be demonstrated |

### Consumer (25)
`establishment 10 → identity 1`; `transport 8 → intake 5`; `transport 8 → staging 4`; `interpretation 12 → staging 4`; `interpretation 12 → intake 5`; `planning 11 → transport 8`; `evolution 13 → establishment 10`;
`evolution 13 → interpretation 12`; `evolution 13 → ontology 2`; `boundary 14 → planning 11`; `flow 15 → transport 8`; `flow 15 → interpretation 12`; `flow 15 → evolution 13`; `flow 15 → boundary 14`;
`status 16 → planning 11`; `status 16 → boundary 14`; `status 16 → op-identity 3`; `status 16 → flow 15`; `status 16 → ontology 2`; `packaging 17 → establishment 10`;
`dogfood 18 → routing 9`; `dogfood 18 → planning 11`; `dogfood 18 → evolution 13`; `dogfood 18 → flow 15`; `dogfood 18 → status 16`.

### Interaction (15)
`establishment 10 → ontology 2`; `staging 4 → op-identity 3`; `intake 5 → op-identity 3`; `review-principal 6 → intake 5`; `interpretation 12 → transport 8`; `interpretation 12 → routing 9`; `interpretation 12 → op-identity 3`;
`planning 11 → op-identity 3`; `planning 11 → routing 9`; `evolution 13 → planning 11`; `evolution 13 → op-identity 3`; `boundary 14 → op-identity 3`; `flow 15 → routing 9`; `routing 9 → transport 8`; `dogfood 18 → review-principal 6`.

### Advisory (3)
`skew-conformance 7 → ontology 2`; `packaging 17 → routing 9`; `packaging 17 → evolution 13`.

**Counts: hard 18, consumer 25, interaction 15, advisory 3 = 61.**

### Acyclicity and determinism
Sequence 1..18 is a linear extension of all 61 edges (every dependent has a larger sequence than every prerequisite; checked mechanically over the list above, zero violations). Priorities are non-decreasing along every edge. Sequences are unique, so
`SelectRunnableWork` never sees a `(priority, sequence)` tie. Longest hard chain has 5 nodes (`4/5/6/1 → 11 → 15 → 17 → 18`), versus a v1 chain that serialised the ontology unit in front of nearly everything.

### Disposition of the seven v1 disputed hard edges
| v1 dependent → prerequisite | v2 |
|---|---|
| requirement-identity → ontology | **no edge** (the review suggested *interaction*; I go one step further because element identity is content-derived and has no vocabulary-dependent content, so even an interaction edge would only force ontology to sequence 1). Vocabulary is consumed by evolution 13 and status 16 instead |
| workspace/intake → ontology | **removed** |
| workspace/intake → operational-identity | **interaction** (staging 4, intake 5 → op-identity 3) |
| provider-envelope → workspace | **consumer** (transport 8 → staging 4, intake 5) |
| interpretation → workspace | **consumer** (12 → 4, 5) |
| interpretation → envelope | **interaction** (12 → 8) |
| evolution → interpretation | **consumer** (13 → 12); deterministic evolution works without a model |

---

## 7. Existing machinery reused (no parallel governance)
Goal establishment and `GoalBaseline` canonicalization; `goalstore.Repository` (`Save`, `Load`, `SaveWorkPlanProposal`, `SaveWorkPlanReview`, `AttachAcceptedWorkPlan`, `SaveReplanningSuccessor`); `WorkPlanProposal`/`Review`/`AcceptWorkPlan`/`MaterializeAcceptedPlanCandidate`;
authority requests and `authority pending|decide` (ADR-097); `goals-lifecycle continue` (SPEC-054); goal-drive runtime; `Admit` leases and `ReconcileLostTurn` (ADR-100); recovery (ADR-098); completion/evaluation/settlement (ADR-099); supervision `ActivityLog`;
provider workers and the provider catalog; `plugins/workspace` path and evidence helpers; `SecureBlob`; package registration and installed help. Added state is limited to: evidence-manifest records, advisory result/provenance records, an invocation-allocation event,
applicability evidence, and a Goal-level control record. No new authority type, no new acceptance store, no status daemon.

---

## 8. Resolution of the nine v1 review findings

| # | Finding | Verdict after independent check | Where resolved |
|---|---|---|---|
| 1 | U02 real integrity defect; v1 used unsafe positional refs | **Supported, verified, and extended** (tautological selector coverage; review digest ignores findings; `ImportSourceDigest` outside the digest; duplicate texts unrejected; nothing after `propose` loads the baseline). v1's refs were likely correct in stored order but unprovable | Unit 1; Section 4 (v2 is content-anchored and evidence-backed without the fix) |
| 2 | Reviewer independence is string comparison | **Supported and extended** (owner-default reviewer can equal the decider; `ReviewerProvider` unread; no reviewer-vs-accepter check) | Unit 6; B4-B6 for v2 |
| 3 | Ontology prematurely decided | **Supported**; corrected line ref (SPEC-014 inv. 2 is line 23); added that establishment carries no owner authority | Unit 2; Q3 |
| 4 | Split workspace vs intake; wrong "byte-identical" predicate; provider two-path infeasible | **Supported with adjustment**: split confirmed; no writable dir and no worktree at all under T1; advisory admission is placed with intake; `Snapshot` fetches so it is unusable as a probe | Units 4, 5, 8; Q4, Q5 |
| 5 | Identity determinism conflicts with ADR-100 single-use | **Supported and extended** (pre-flight refusals burn ids; identity unique only per generation; activity stream key lacks goal) | Unit 3; Q6 |
| 6 | Successor must preserve consequences as evidence, not completion | **Supported and verified**; extended (non-atomic succession; supersession only after settlement; no fence; no drive-time authority re-check) | Units 13, 14; Q7 |
| 7 | U11 unbounded; split flow vs status | **Supported**; extended (`InvocationSummary` dead code; no Goal-level pause visible across invocations) | Units 15, 16; Q8 |
| 8 | U12 necessary; concrete skew | **Supported, corrected and extended**: installed binary is `f539b74` with 0.1.4 already; installed package generation unverified; static help lacks `goals-lifecycle`/`goal-drive`; `goals` invocation unregistered; stale docs | Units 7, 17; Q9 |
| 9 | U13 bindings incomplete | **Supported**; v2 binds SC1-SC12 explicitly and adds transport and fail-closed proof | Unit 18; Section 9 |

**Not adopted verbatim:** the review's recommendation to "extend U02/U06" for reviewer binding: I made it its own unit because it touches the review record, the authority records and advisory provenance, and can be qualified independently.

---

## 9. Final dogfood acceptance path (unit 18)

Preconditions: units 1-17 complete; installed via the normal governed installation; evaluator independent of planner and implementer. This plan does **not** choose the next requirement.

| Step | What is done | Demonstrates |
|---|---|---|
| D1 | Verify installed package, binary and help against source with unit 7's harness | SC11, C9 |
| D2 | A human writes the next real requirement as plain Markdown | SC1, SC12 |
| D3 | Run only the human interface. Praxis stores the exact source, shows what it understood with per-element provenance, and asks only a material question, if any | SC2, SC3, SC4, C4 |
| D4 | Adversarial source containing instructions: no requirement, constraint or authority is manufactured | C3 |
| D5 | Goal established; `planning_required` reached; planner runs through **the supported real provider transport (Claude T1)**; independent review by a distinct provenance-bound principal; stops only at owner authority, answered as a plain-language decision | SC5, SC8 |
| D6 | Codex (or a provider without a passing probe) is requested for planning: Praxis reports a typed environment/capability failure, creates no authority request, and asks the human nothing | truthful fail-closed |
| D7 | Execution begins with Praxis-minted identities and policy-chosen provider | SC5, C10 |
| D8 | Mid-execution: "this is now a requirement". Classification, successor, applicability evidence, deterministic boundary; the in-flight provider turn is not rewritten | SC6, SC7, C5, C6 |
| D9 | Predecessor generation and completions are byte-identical afterwards; successor ledger starts empty; reuse appears only as applicability evidence and successor's own completion acts | immutable history, no reattribution |
| D10 | After reconciliation, work is derived and scheduled with no human-authored decomposition | SC8 |
| D11 | Status at each phase reads from durable state, distinguishes complete from invocation-terminated, pause survives a new invocation, shows `unknown` where evidence is absent | SC9 |
| D12 | Operator retrieves every exact record (digests, lineage, provenance, recoveries) and explicit-flag commands still work | SC10, C8 |
| D13 | Human never authored GoalBaseline JSON, digests, generations, invocation ids, WorkPlans, proposal/review/request digests or acceptance refs, and never scheduled work | SC5, SC12 |

Failure to complete D2-D13 without the bootstrap ceremony means the Story is not complete.

---

## 10. Remaining architectural uncertainties

- **U-1 Stored-order attestation (B2).** The plan assumes read-only `inspect` shows stored order and `ImportSourceDigest`. If it does not, the manifest's `#<kind>/<n>` cannot be independently confirmed and must be regenerated from another read-only export.
- **U-2 Transport behaviour is unverified live.** Flags exist in the installed help and binary strings, but no provider was started. Whether `--tools ""` coexists with `--json-schema`, and Codex's stdout under `--output-schema` + `read-only`, are probe questions. T1 does not depend on the structured-output feature because the controller validates.
- **U-3 Owner-as-reviewer.** Whether an owner-authored review satisfies `review_all` for a model-authored plan, and whether reviewer must differ from decider, is a governance decision the plan does not make.
- **U-4 Establishment authority.** Whether Goal establishment needs an owner confirmation (and how, without a new authority kind) is decided by unit 2; unit 10 depends on the answer only through an interaction edge.
- **U-5 Successor completion basis.** Whether "re-verified against the successor's head, citing predecessor evidence" is admissible as a successor completion under ADR-099 needs an amendment; if rejected, applicability evidence shortens planning but every successor unit is completed normally.
- **U-6 Supersession disposition.** A change-driven supersession of an unsettled generation may need a settlement-adjacent disposition distinct from INCOMPLETE.
- **U-7 Typed confirmation for ordinary users.** `authority decide` requires a typed `DECIDE-<OUTCOME> <digest>` confirmation. The flow shows the exact command, but whether that ceremony is acceptable for an ordinary user is unresolved.
- **U-8 Package churn.** Each contract change forces a signed goals-package successor (0.1.5+). Unit 17 owns the cadence; unit 7 must not declare options no unit implements.
- **U-9 Installed package generation.** `~/.praxis/praxis.db` was not read, so installed-package skew is unverified.
- **U-10 Element-identity migration.** Baselines with duplicate element text (none in this Goal; all 31 are distinct) need an explicit rule.

---

## 11. Completion report (as requested)

- **Plan artifact path:** `/Users/polliard/.claude/plans/synthetic-tickling-pnueli.md`
- **Candidate count:** 18
- **Relationships by kind:** hard_dependency 18, consumer 25, interaction 15, advisory 3 (total 61)
- **v1 units split / rescoped / added / removed:** split U03 (→ operational-identity-allocation, provider-routing-policy), U04 (→ advisory-evidence-staging, advisory-result-intake-and-recovery), U11 (→ human-lifecycle-flow, status-and-control-projection), U12 (→ surface-skew-conformance, installed-human-surface-packaging); rescoped U01 (a decision), U02 (semantic element identity), U05 (transport, not a two-path envelope); added hi-review-principal-provenance; merged none; removed none. U06-U10 and U13 retained with revised dependencies and bindings.
- **Disposition of the nine findings:** all nine supported after independent verification, five extended with new evidence, one recommendation restructured (Section 8).
- **How v2 avoids relying on the requirement-binding defect:** content-derived requirement ids and text digests come from an independently recomputed element manifest (Goal digest reproduced as `sha256:afda0866…7530`), refs are materialized by script from that manifest, an independent principal re-verifies coverage 31/31, and the evidence digests are pinned into the review record and the authority request the owner signs, all through fields that exist today (Section 4).
- **Remaining uncertainties:** Section 10.

Not done in this session: writing `planner-proposal-v2.json`, invoking Praxis, running any provider, or modifying any repository file.
