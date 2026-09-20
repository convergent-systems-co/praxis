# Advisory WorkPlan Proposal v4 plan: `praxis-human-interface/1`

Advisory planning evidence only. This document is not a `WorkPlanProposal`, grants no authority, makes none of the Gate A/B/C decisions, and does not authorize bootstrap implementation or installation. Proposal v4 JSON must not be materialized until every pre-authority condition in this plan is independently verified.

Authoritative inputs:

- Goal: `praxis-human-interface/1`, canonical digest `sha256:afda0866398d14a223f3a090d554c820e9e945922a936e6f1edb9c06d9177530`.
- Frozen v3 planning evidence: `docs/research/dogfood/praxis-human-interface/plans/workplan-v3-claude.md`, SHA-256 `966d193d57d41402e414435999aaac138c4af00162101f472ec475470ae963f5`.
- Adverse independent v3 review: `docs/research/dogfood/praxis-human-interface/reviews/workplan-v3-codex-review.md`, SHA-256 `999a5937e06e9a78fe90287a8a9308e468b7adc3f5777c30c03854c510cdc2fd`, disposition `REVISION_REQUIRED`.
- Reviewed v3 proposal digest: `sha256:d18bcd5215c24ba2799fce0e49595a5cb17f07aadd64aa5acbb087e8dc37c4e2`.
- Existing source-bound establishment evidence B-1/B-2/B-3 remains authoritative. Its 31 elements are 12 success criteria, 10 constraints, 5 non-goals, and 4 assumptions.

## 1. Outcome and v3 to v4 change summary

V4 keeps the qualified v3 product decomposition and removes the circular claim that a WorkPlan can install the controller needed to execute itself safely.

The minimum revision is:

1. Remove `unit:hi-authority-gate-mechanism` from the proposal. Its safety-critical responsibilities become a separately implemented, independently qualified, installed, and activated **Bootstrap Safety Kernel** that is a precondition to materializing v4, not v4 work.
2. Reduce the proposal from 23 to 22 candidates and from 71 to 68 relationships. Only the three hard edges from Gate A/B/C to the removed mechanism disappear. The remaining 28 hard, 24 consumer, 13 interaction, and 3 advisory relationships are preserved.
3. Introduce an explicit candidate kind and fail-closed provenance transition for authority gates. A pre-kernel proposal validator rejects gate proposal provenance; a pre-kernel accepted-plan validator rejects activated gate provenance. Thus an old controller cannot persist or execute a v4 gate as ordinary work.
4. Bind v4 proposal acceptance and every drive to one exact activation manifest: core binary bytes/build commit, goals package generation/content/executable/contract digests, safety-kernel contract version, validator profile digest, candidate-spec bundle digest, and qualification-evidence digest.
5. Replace v3's out-of-plan `.praxis/validate` instruction with a frozen, content-addressed validation profile installed and qualified before v4 authority. V4 drive has no `no validator` success path.
6. Remove Gate C's predetermined “live turn always finishes” rule. Gate C presents owner-selectable alternatives; downstream code implements only the exact approved decision.
7. Remove mutation from `goals-lifecycle --operation=decide`. It may only report an exact already-persisted decision for compatibility. New v4 acceptance and gate decisions require the interactive, OS-owner-bound ceremony profile.
8. Extend source integrity from `RequirementRef` to candidates and relationships. Exact candidate and relationship specification bytes are resolved and preserved before acceptance, included in the accepted artifact digest, and injected from the accepted artifact rather than reread from a mutable plan path.
9. Move cycle rejection, duplicate-endpoint rejection, deterministic relationship ordering, weaker-kind context repair, and conservative pre-Gate-C interruption fencing into the bootstrap kernel because they must hold for the first v4 unit.
10. Preserve the v3 direction for content-derived requirement identity, 31/31 coverage, reviewer provenance, provider isolation, operational identity, flow/status separation, predecessor evidence, package/source coherence, deterministic single-track scheduling, routing, and final dogfood qualification.

No v3 claim is defended merely because it appeared in v3. The retained structure is retained because the independent review found it substantively sound.

## 2. The bootstrap architecture, stated without circularity

### 2.1 What current Praxis can and cannot govern

Current Praxis has an exact, installation-bound package deployment path. An exact package intent binds the verified closure and evidence; `authority package-deploy-approve` requires an interactive terminal and the installation owner; activation is atomic (`cmd/praxis/package_manager_authority.go`, `internal/goalstore/package_deployment_approval.go`, `internal/state/package_registry.go`).

The Goal-drive controller is nevertheless compiled into the core binary and dispatched natively (`cmd/praxis/main.go`). The goals package intentionally carries no executable binding for `goal-drive` (`packages/goals/package_test.go`). The accepted installation-lifecycle contract models a binary component, but the current implementation supplies production drivers only for storage-schema and runtime-state recovery, not general core-binary replacement (`pkg/contracts/lifecycle.go`, `internal/lifecycle`). Therefore current Praxis cannot fully govern installation of the first safety-kernel binary.

V4 does not hide this gap. There is one irreducible bootstrap transition:

- the core binary must be built from an independently reviewed clean commit and atomically installed by the existing human-controlled release/OS installation channel;
- after that binary is active, the signed goals-package successor is deployed through the existing exact `package.deploy` ceremony;
- no v4 proposal, review, request, acceptance, attachment, or drive may exist before both identities cohere and the activation manifest is persisted.

If independent reviewers do not accept the external binary-install evidence, Proposal v4 must not be materialized. Building a general binary lifecycle driver is not silently added to this Goal.

### 2.2 Bootstrap Safety Kernel: bounded contents

The kernel contains only prerequisites that must already be enforced before the first v4 candidate can run:

- explicit `work` versus `authority_gate` candidate typing;
- proposal provenance `model_gate_proposal` and accepted provenance `authority_gate`, both unknown to and rejected by the old contract;
- controller-owned gate request/decision/completion handling; no provider resolution for a gate;
- v4 activation-manifest verification at propose, request, accept, attach, and every drive;
- complete removal of non-interactive decision mutation for protected and new requests;
- exact candidate/relationship/requirement specification resolution and accepted-byte preservation;
- mandatory validation-profile resolution and fail-closed completion semantics;
- hard-cycle, self-edge, duplicate-endpoint, unknown-kind, priority, sequence, and deterministic-digest validation;
- truthful worker context: only hard prerequisites are described as completed readiness prerequisites; consumer, interaction, and advisory neighbors are labelled non-blocking context;
- a conservative pre-Gate-C fence: unauthenticated `supervise cancel|suspend` cannot affect a v4 turn, and a successor cannot govern while any predecessor admission/lost-turn consequence is unresolved. This chooses no disposition; it only refuses unsafe mutation until Gate C authorizes one.

The kernel does not implement the human interface, choose ontology, set reviewer-independence policy, choose active-turn disposition, add provider routing, or claim product success.

### 2.3 Explicit activation sequence

The following sequence occurs before v4 materialization:

1. **Freeze a bootstrap change specification.** Track exact requirements, threat model, compatibility behavior, validation profile, test plan, source commit, and expected binary/package identities. Hash every artifact.
2. **Implement and independently review the kernel.** This is separate bootstrap work, not a v4 candidate. Review must include negative tests for old-controller behavior, decision bypass, missing validator, spec drift, gate worker attempts, cycles, duplicate endpoints, weaker-kind wording, supervision mutation, and predecessor lost turns.
3. **Qualify the clean source commit.** Record exact commands, results, output digests, test-binary identities, source commit, and `git status`. A passing mechanism test is evidence, not product proof.
4. **Build a clean core binary and goals package from the same source identity.** Record core binary SHA-256, `praxis version` build commit with `modified:false`, embedded goals-package identity, signed package manifest/content/executable/contract digests, publisher lineage, and build-reproducibility evidence.
5. **Quiesce mutation.** No Goal-drive or goals-lifecycle mutation runs during the transition. Preserve any live consequences; do not cancel or rewrite them.
6. **Atomically install the core binary through the existing human-controlled release/OS channel.** This is the explicit non-Praxis bootstrap step. Preserve the predecessor binary for recovery but remove it from the active command path. Verify the active executable by resolved path, bytes digest, build provenance, and process image.
7. **Fail closed during package skew.** The new binary refuses `goal-drive` and mutating goals-lifecycle operations while installed goals-package identity differs from the kernel's required package identity. Static package governance commands remain available.
8. **Deploy the signed goals-package successor.** Use the current exact intent/evidence/request flow and the specialized interactive `authority package-deploy-approve` ceremony, then atomically activate it. Do not use `goals-lifecycle decide`.
9. **Persist a Bootstrap Activation Manifest.** It binds the installation root; active binary path/digest/build commit; embedded and installed package versions, content, executable, contract and activation-receipt digests; kernel contract version; validation-profile digest; compatibility-fence version; qualification-evidence digest; activation time; and predecessor recovery identity.
10. **Run read-only activation probes.** Prove new and restarted processes report the same identities; package/source/embedded coherence holds; old binary rejects the v4 gate proposal provenance and the accepted gate provenance before worker selection; skew fails closed; non-interactive decision input cannot mutate; missing/drifted specs and validators fail before admission; gate selection cannot resolve a worker.
11. **Freeze the v4 planning/specification bundle.** Only now may the advisory plan be transformed into exact candidate and relationship records. Independent verification produces their digests and the 31-element coverage report.
12. **Only after steps 1-11 pass may Proposal v4 be materialized, independently reviewed, persisted, and presented for authority.** V4 acceptance itself must use the activated interactive ceremony.

An archived old executable cannot be made metaphysically incapable of running by new code. The enforceable guarantees are that (a) v4 does not exist before activation, (b) the old contract rejects the new gate provenance if presented with v4 bytes, (c) the active path is digest-bound, and (d) every v4 mutation rechecks that active identity. Any review requiring revocation of arbitrary copied historic binaries must classify that as an unsolved installation-security problem, not accept an attestation.

## 3. Authority boundary before and after activation

| State | Permitted behavior | Required failure behavior |
|---|---|---|
| Before kernel binary installation | Read/review existing evidence; implement and test the separate bootstrap change under its own authority/process | No v4 proposal persistence, request, acceptance, attachment, or drive |
| Kernel binary active, package not coherent | Static version/doctor/package-governance and read-only inspection | Goal-drive and mutating goals-lifecycle fail with exact expected/observed identity evidence |
| Binary/package coherent, activation manifest absent or invalid | Read-only inspection and bootstrap qualification | V4 propose/request/accept/attach/drive fail closed |
| Activation manifest valid, v4 not accepted | V4 materialization, independent review, request creation | No selector input; no gate or work execution |
| V4 accepted through required ceremony | Normal work candidates may be selected subject to validation and hard dependencies | Gate candidates never enter provider resolution; missing/drifted spec, validator, activation, or authority evidence fails before admission |
| Gate pending | Other ready ordinary siblings may run; if none are ready, surface `HUMAN_AUTHORITY_REQUIRED` | Worker trailers, model output, non-interactive decisions, or mismatched decision digests cannot complete the gate |
| Gate approved | Controller records gate completion citing request, decision, dossier and activation digests; exact dependents may proceed | No policy beyond the approved record is inferred |
| Gate rejected | Gate remains unsatisfied and dependent work remains blocked; revision requires a successor dossier/proposal path | Rejection never means an alternative was implicitly selected |

The old controller's ordinary worker path fails on v4 in two independent ways: gate proposal provenance is invalid before persistence, and accepted gate provenance is invalid during plan validation. The activated controller is the only controller that knows those values, and it routes them before worker lookup.

## 4. Validation and qualification bootstrap

### 4.1 Frozen validation profile

Before v4 authority, track and hash a `v4-validation-profile` that contains:

- profile ID/version/digest and compatible kernel identity;
- a closed set of validation operation IDs and argv templates, never worker-supplied shell text;
- repository preconditions, allowed files, timeout/output/process-tree bounds, and expected evidence schema;
- per-candidate required qualification predicates drawn from the frozen candidate specification;
- rules for mechanism tests, candidate conformance, final dogfood evidence, and independent evaluation;
- the exact rule that absence, unreadability, unknown operation, drift, timeout, truncation, non-zero exit, or malformed evidence is failure.

The profile may invoke tests added by governed units through fixed commands, but a worker cannot edit the profile, change its digest, or remove a predicate. A profile change requires a new activation manifest and fresh authority; it is never accepted as part of a unit's own completion.

### 4.2 Durable proof vocabulary

V4 keeps these states distinct:

1. **Attempted:** a turn was admitted and provider execution began.
2. **Checkpoint completed:** provider-owned consequences were preserved in a clean, published, exact commit.
3. **Mechanism/unit tests passed:** the frozen validator ran its named operations against that exact commit and stored result/output digests.
4. **Candidate conformance qualified:** all predicates in the accepted candidate specification were satisfied by controller-verified evidence.
5. **Product outcome supported by evidence:** DOG and an independent evaluation support an owner settlement decision.
6. **Goal settled complete/incomplete:** only the existing owner settlement transition is authoritative.

For v4, a `UnitCompletion` may be recorded only after states 2-4 exist for the same candidate/spec/profile/commit digests. Selector readiness consumes only that qualified completion. UI and inspection must never render states 1-3 as “qualified,” or states 1-4 as “proven.” There is no `repository:no-declared-validation` alternative for a v4 unit.

### 4.3 Qualification of the qualification mechanism

Pre-authority negative tests must demonstrate: no profile; wrong profile digest; empty predicate set; unknown operation; validator modified by the worker; successful command with malformed evidence; stale commit; output truncation; timeout with surviving child; and test pass without conformance predicates. All fail without recording completion. Restart/replay must reproduce one result and never rerun an ambiguous mutation.

## 5. Candidate set: 22 candidates

Priorities remain scheduling preferences among ready candidates. Sequences are unique deterministic tie-breakers and form a linear extension of all relationships. Only hard dependencies affect readiness.

| pri | seq | candidate | short | authoritative bindings | disposition from v3 |
|---:|---:|---|---|---|---|
| 1 | 1 | `unit:hi-governance-decision-dossier` | DOS | SC3 SC4 SC6 C1 C3 C5 C6 N1 N3 | changed |
| 1 | 2 | `gate:hi-authority-ceremony` | GA | SC3 SC4 SC6 C1 C3 | changed contract, same questions |
| 1 | 3 | `gate:hi-review-independence` | GB | SC5 C1 C3 N1 | changed contract, same questions |
| 1 | 4 | `gate:hi-supersession-and-active-turn` | GC | SC6 SC7 C1 C5 C6 | changed alternatives |
| 2 | 5 | `unit:hi-requirement-identity-and-binding` | ID | SC5 SC7 SC8 C1 C2 C3 C7 | rescoped |
| 2 | 6 | `unit:hi-operational-identity-allocation` | OPI | SC5 C1 C7 C8 C10 | preserved |
| 2 | 7 | `unit:hi-advisory-evidence-staging` | STG | SC5 C1 C2 C7 | preserved |
| 2 | 8 | `unit:hi-advisory-result-intake-and-recovery` | INT | SC5 SC8 C1 C2 C3 C7 C10 | preserved |
| 2 | 9 | `unit:hi-surface-skew-conformance` | SKW | SC10 SC11 C8 C9 N2 | preserved |
| 2 | 10 | `unit:hi-provider-advisory-transport` | TRN | SC5 C1 C3 C7 C8 N1 | strengthened qualification |
| 2 | 11 | `unit:hi-provider-routing-policy` | RTE | SC4 SC5 C7 C8 A4 | preserved |
| 3 | 12 | `unit:hi-review-principal-provenance` | RPP | SC5 C1 C2 C3 N1 | preserved, explicitly downstream of Gate B |
| 3 | 13 | `unit:hi-ontology-and-surface-contract` | ONT | N3 N4 N5 A1 A2 A3 C1 C7 C8 | preserved |
| 3 | 14 | `unit:hi-source-bound-goal-establishment` | EST | SC1 SC2 SC3 SC5 C1 C2 C7 A2 | preserved |
| 4 | 15 | `unit:hi-governed-planning-orchestration` | PLN | SC5 SC8 SC10 C1 C3 C7 C10 N4 | ceremony binding strengthened |
| 4 | 16 | `unit:hi-model-assisted-interpretation-and-ambiguity` | INTERP | SC1 SC2 SC3 SC4 C2 C3 C4 | preserved |
| 5 | 17 | `unit:hi-requirement-evolution-classification-and-successor` | EVO | SC6 SC7 C1 C2 C5 C7 | preserved |
| 5 | 18 | `unit:hi-evolution-execution-boundary` | BND | SC6 SC7 C1 C5 C6 | changed to implement only Gate C decision |
| 5 | 19 | `unit:hi-human-lifecycle-flow` | FLOW | SC4 SC5 SC8 C10 N4 | decision route strengthened |
| 5 | 20 | `unit:hi-status-and-control-projection` | STAT | SC9 SC10 C8 N2 | proof labels and supervision state strengthened |
| 6 | 21 | `unit:hi-installed-human-surface-packaging` | PKG | SC11 C8 C9 N2 N5 | activation coherence strengthened |
| 7 | 22 | `unit:hi-dogfood-acceptance` | DOG | SC1-SC12 C1-C10 | bootstrap and proof assertions strengthened |

`unit:hi-authority-gate-mechanism` is removed, not postponed. Its justified functionality is in the pre-v4 kernel. Removing its bindings does not reduce authoritative coverage because every element it referenced is also bound by retained candidates. The v4 materializer must independently rederive and report 31/31 coverage from B-1/B-2.

## 6. Bounded responsibilities of changed candidates

Unlisted candidates keep their v3 responsibilities and qualification, subject to the global accepted-spec and validation rules above.

### DOS — governance decision dossier

Produce three separate content-addressed dossiers. Each contains primary evidence, all supported alternatives and consequences, a non-binding recommendation if useful, and an explicit `undecided` marker. Gate C must include at least: finish and reconcile; authenticated interrupt with durable consequence preservation; and any pause/quarantine option the implemented architecture can actually support. The validator rejects missing alternatives or language that presents one as already chosen.

### GA / GB / GC — authority gates

These are typed `authority_gate` candidates, not work. Their accepted specification names the question schema and dossier role. The controller creates one owner-only, non-delegable, digest-bound request and never resolves a provider. Approval records only the owner's exact answer; rejection blocks dependents. Gate completion cannot be proposed by a commit trailer.

GA retains v3's establishment/authority, typed-confirmation, evolution-confirmation, inferred-material-element, and changed-contract-successor questions. GB retains reviewer/decider separation, lineage independence, provenance, and adverse-review override questions. GC uses the semantics in Section 8.

### ID — requirement identity and binding

Retain content-derived requirement identity, exact three-way resolution, baseline-derived review denominator, historical duplicate behavior, and coverage-by-identity. Remove claims that this unit first makes the proposal graph or worker context safe; those are already kernel preconditions. ID generalizes and productizes the kernel's resolver for all Goal lifecycle surfaces and qualifies historical compatibility.

### TRN — provider advisory transport

Keep Claude as the first possible advisory transport and Codex unsupported. A flag's existence is not qualification. Support is enabled only for an exact `(provider binary digest, version, flag-set digest, OS-containment profile digest)` with a passing durable probe. Qualification must demonstrate no ambient hooks, CLAUDE.md, skills, agents, plugins, commands, memory, configured/plugin/connector MCP, unintended built-in tools, repository access/mutation, writes outside scratch, or surviving child process. If the platform cannot positively enforce or observe external writes, the transport remains `unsupported`; a clean scratch directory alone is insufficient.

### PLN — governed planning orchestration

Use only the hardened interactive decision path for WorkPlan acceptance. It may consume bootstrap review evidence until RPP exists, then must use RPP's Gate-B-derived policy. It cannot downgrade an adverse review or manufacture an acceptance ceremony.

### BND — evolution execution boundary

Implement exactly the approved Gate C record. Shared invariants are fixed regardless of the choice: provider-owned consequence is never silently rewritten or destroyed; all predecessor admissions and lost-turn consequences are globally reconciled; historical ledgers stay immutable; successor work receives no predecessor completion as proof; and drive rechecks governing authority. Finish, interrupt, pause, or quarantine behavior exists only if and as authorized.

### FLOW — human lifecycle flow

Expose authority decisions only through the interactive owner ceremony. Remove mutating `goals-lifecycle decide` from examples, help, and dispatch. Compatibility may display an already-recorded legacy decision but cannot persist one. Every accepted decision links to ceremony evidence and the exact request digest.

### STAT — status and control projection

Render attempted, checkpoint-completed, tests-passed, conformance-qualified, outcome-evidence, and settlement separately. Show pending Gate C restrictions and authenticated supervision authority. Never infer `proven` from a unit completion, test pass, or provider statement.

### PKG — installed human surface packaging

Package only after source, embedded binary, goals plugin executable, installed package generation, invocation contract, validation profile, and activation manifest identities cohere. Do not imply that package activation installs the native Goal-drive controller.

### DOG — final dogfood acceptance

Retain direct SC1-SC12 and C1-C10 bindings and the full hard closure. Add negative demonstrations for: old-controller rejection; activation drift; missing validator; candidate/relationship spec drift; non-interactive decision mutation; gate worker routing; unsupported provider isolation; unauthenticated interruption; and predecessor lost-turn takeover. DOG plus independent evaluation supports a settlement decision; it does not label the product mathematically proven.

## 7. Complete relationship set and changes

Notation is `dependent -> prerequisite`. Every relationship receives its own exact specification bytes and digest in the v4 specification bundle.

### 7.1 Hard dependencies (28)

1. `GA -> DOS`
2. `GB -> DOS`
3. `GC -> DOS`
4. `RPP -> GB`
5. `RPP -> ID`
6. `ONT -> GA`
7. `EST -> GA`
8. `INTERP -> GA`
9. `BND -> GC`
10. `TRN -> INT`
11. `PLN -> RPP`
12. `PLN -> INT`
13. `PLN -> STG`
14. `EVO -> ID`
15. `EVO -> EST`
16. `BND -> EVO`
17. `FLOW -> PLN`
18. `FLOW -> EST`
19. `FLOW -> OPI`
20. `FLOW -> ONT`
21. `PKG -> FLOW`
22. `PKG -> STAT`
23. `PKG -> SKW`
24. `DOG -> PKG`
25. `DOG -> BND`
26. `DOG -> INTERP`
27. `DOG -> TRN`
28. `DOG -> RTE`

Relative to v3, remove only `GA -> MECH`, `GB -> MECH`, and `GC -> MECH`, because MECH is no longer a candidate. Activation and validation are stronger plan-level admission preconditions, not fake WorkPlan edges to already-installed infrastructure.

### 7.2 Consumer relationships (24, unchanged)

`EST->ID`; `TRN->STG`; `INTERP->STG`; `INTERP->INT`; `INTERP->EST`; `RPP->INT`; `PLN->TRN`; `EVO->INTERP`; `EVO->ONT`; `BND->PLN`; `FLOW->TRN`; `FLOW->INTERP`; `FLOW->EVO`; `FLOW->BND`; `STAT->PLN`; `STAT->BND`; `STAT->OPI`; `STAT->FLOW`; `STAT->ONT`; `PKG->EST`; `DOG->PLN`; `DOG->EVO`; `DOG->FLOW`; `DOG->STAT`.

### 7.3 Interaction relationships (13, unchanged)

`EST->ONT`; `STG->OPI`; `INT->OPI`; `INTERP->TRN`; `INTERP->RTE`; `INTERP->OPI`; `PLN->OPI`; `PLN->RTE`; `EVO->PLN`; `EVO->OPI`; `BND->OPI`; `FLOW->RTE`; `RTE->TRN`.

### 7.4 Advisory relationships (3, unchanged)

`ONT->SKW`; `PKG->RTE`; `PKG->EVO`.

Independent verification must confirm: 22 unique candidates; sequences 1-22; valid priorities; 68 unique endpoint pairs; no self-edge; known kinds; hard acyclicity; no transitively redundant hard edge; every non-DOG candidate in DOG's hard closure; and deterministic canonical sorting by dependent, prerequisite, kind, source digest, and source ref. Duplicate endpoints are rejected even when kinds differ.

## 8. Gate semantics without making the decisions

### Gate A — authority ceremony and establishment

Questions remain those from v3. The dossier must reconcile “authoritative Goal state” with “derived knowledge, never execution authority”; ask whether establishment needs confirmation; define allowed typed-ceremony changes; present the evolution confirmation matrix; cover inferred material elements; and ask about direct changed-contract succession. No starting recommendation becomes executable policy without the owner decision.

### Gate B — reviewer independence

Questions remain those from v3: owner-as-reviewer/decider, minimum independent model lineage, resolvable human/model provenance, and adverse-review override. Until RPP is implemented, v4 uses a bootstrap evidence rule: review is from a human or a genuinely different model/provider lineage, in a fresh context, with independent recomputation, and its complete bytes/digest are tracked. This is procedural evidence, not a claim of current contract enforcement.

### Gate C — supersession and active turns

Gate C must separately ask:

- disposition of a partially executed unsettled predecessor;
- whether a materially invalidating change permits a live turn to finish and how its consequences are reconciled;
- whether authenticated interruption is permitted, by whom, for which classes, and how every consequence is durably preserved;
- whether pause or quarantine is available, what it blocks, and how it is resumed/reconciled;
- treatment of work made inapplicable: preserve, quarantine, mark applicability-invalid, or another evidenced option;
- how unreleased predecessor lost turns affect successor governance.

C6 fixes only the invariant that provider-owned work is not silently rewritten or destroyed. It selects none of these dispositions. Before Gate C approval, the kernel permits preservation and observation but refuses unauthenticated interruption and successor takeover.

## 9. Decision-path hardening

The bootstrap kernel must make these rules active before a v4 request exists:

1. `goals-lifecycle --operation=decide` cannot persist any `AuthorityDecision`. For compatibility it may load and display an exact existing decision, with `replay:true`, but input cannot create or change state.
2. V4 `workplan.accept` and `goal.gate.decide` requests carry a required ceremony profile, exact activation-manifest digest, and exact request digest.
3. Only `authority decide` may resolve these requests. It re-derives the installation owner and active root from durable state, checks interactive TTY and authenticated OS user, displays the exact request/questions, requires typed digest-bound confirmation, and stores a ceremony-evidence digest.
4. `SaveAuthorityDecision` rejects a protected request lacking the required ceremony evidence/profile, even if all public decision fields were copied correctly.
5. Acceptance revalidates decision, ceremony, activation, proposal, review, baseline, candidate-spec bundle, and validator-profile digests in the commit transaction.
6. Package deployment continues through its specialized exact interactive approval. Legacy decisions remain verifiable historical evidence; compatibility never makes them sufficient for new v4 acceptance or gates.
7. Help, installed invocation contracts, source dispatch, and negative tests all agree that non-interactive decision mutation is unavailable.

The current OS-user ceremony is the existing authentication root. V4 does not claim stronger cryptographic human authentication than current Praxis provides.

## 10. Candidate and relationship specification integrity

Before Proposal v4 materialization, transform this frozen plan into a tracked canonical specification bundle. Do not use Markdown anchors plus a whole-file digest as the executable source.

For every candidate, canonical bytes must include: ID; explicit kind; bounded responsibility; exclusions; requirement refs; priority/sequence; accepted qualification predicates; gate question schema if applicable; and provenance. For every relationship, canonical bytes must include: endpoints; kind; rationale; and provenance. Each record gets its own SHA-256 and content-addressed source reference.

The resolver must:

- load the exact tracked bundle and validate its manifest digest;
- resolve each candidate, relationship, and requirement ref to exactly one record;
- compare record bytes to `source_digest` and all identity fields;
- reject missing, duplicate, ambiguous, invented, or drifted records;
- preserve the exact resolved bytes in a durable accepted-spec blob set;
- include the blob-set digest and activation/validation profile digests in proposal, review, request, acceptance, and WorkPlan canonical digests;
- inject candidate text, qualification predicates, exact requirement text, hard prerequisites, and separately labelled non-hard context from the accepted blob set;
- never reread the live plan file for worker context;
- fail before admission if any accepted blob or activation binding is unavailable or inconsistent.

Materialization must not overwrite candidate/relationship provenance with only an authority-request anchor, as current `MaterializeAcceptedPlanCandidate` does. Authority provenance and specification provenance are separate bindings and both survive acceptance.

## 11. Reconciliation of the Codex v3 review

| V3 finding | V4 disposition | Required evidence before authority |
|---|---|---|
| B1 gate mechanism has no activation boundary | Resolved in plan by removing MECH and requiring pre-v4 kernel activation; old/new provenance values make old contracts reject v4 | activation manifest; old-binary negative probes; no v4 state before activation; gate-no-worker test |
| B2 validation ungoverned/unbound | Resolved in plan by frozen validator profile required at acceptance and drive; no missing-validator completion | profile bytes/digest; negative suite; qualification records tied to exact commits |
| B3 Gate C predetermines finish | Resolved in plan; all material alternatives remain owner choices | Gate C dossier schema test and independent review |
| B4 non-interactive decide bypass | Resolved only by bootstrap kernel, before v4 acceptance | mutation-negative tests through source and installed surfaces; ceremony-bound acceptance test |
| B5 candidate specs unresolved | Resolved by canonical candidate/relationship bundle and durable accepted bytes | bundle verifier, drift/missing/ambiguity tests, worker-context capture |
| B6 authority packet incomplete | Procedural; must be produced for v4 | tracked v4 plan/proposal/review provenance and persisted-state capture |
| Provider isolation only directionally supported | TRN remains unqualified until exact runtime/OS containment canary passes | version-bound real-provider probe and external-write enforcement/observation |
| Reviewer-principal enforcement downstream | Deliberately remains after Gate B; bootstrap cross-lineage rule covers v4 only | independent v4 review plus later RPP qualification |
| Weaker prerequisites described as done | Moved into kernel because first worker context must be truthful | context golden tests for all four relationship kinds |
| Hard cycles silently block | Moved into kernel; reject at propose, review, request, accept, attach, and drive | cycle fixtures at every boundary |
| Duplicate endpoint/digest ambiguity | Moved into kernel; reject duplicate endpoint pair and fully order digest keys | permutation/property tests and duplicate-kind fixture |
| Lost-turn reconciliation is per generation | Conservative cross-generation fence in kernel; policy completed by BND after Gate C | predecessor-lost-turn takeover negatives and BND qualification |
| Supervision stop authority unauthenticated | Kernel refuses unauthenticated mutation for v4; Gate C/BND define allowed authenticated behavior | installed `supervise` negative tests and exact authorized-action tests after Gate C |
| Bootstrap path-name drift | Corrected: authority packet uses actual tracked paths and generated manifest paths, never stale prose names | path-existence/digest verifier |
| B-6/B-8 outstanding | Remain mandatory pre-authority artifacts, not assumed facts | tracked independent review/provenance and read-only persisted v4 capture |

## 12. Bootstrap and pre-authority evidence set

Reuse B-1/B-2/B-3 without rewriting them. Create new versioned files only after their stated events occur:

- B4-v4: independent verifier source plus output covering 31-element identity/coverage, candidates, relationships, hard closure, cycles, duplicates, sequences, deterministic digest, and specification resolution.
- B5-v4: frozen v4 plan bytes and SHA-256.
- B6-v4: complete independent review bytes, reviewer provenance, third-party-computed digest, and disposition.
- B7-v4: proposal materializer/verifier and byte-for-byte reproduction output.
- B8-v4: read-only persisted proposal/review/request state captures as applicable, each with capture metadata and command/binary identity.
- B9-v4: Bootstrap Safety Kernel change specification, source commit, independent code reviews, test evidence, and exact core/package build identities.
- B10-v4: Bootstrap Activation Manifest and read-only activation/skew/old-controller probes.
- B11-v4: frozen validation profile and its negative qualification evidence.
- B12-v4: canonical candidate/relationship specification bundle and per-record manifest.

B8 must not claim an acceptance or decision that has not occurred. Before authority it should prove proposal/review persistence and absence/presence of the exact pending request truthfully. None of these files may rely on `/tmp`, ignored `.praxis` content, conversational history, or mutable external URLs.

## 13. Requirement coverage and final qualification

The v4 materializer must derive the authoritative denominator independently from B-1/B-2, not from proposal claims. Required result: 12/12 success criteria, 10/10 constraints, 5/5 non-goals, 4/4 assumptions, 31/31 total. Content-derived identity remains `<kind>:sha256:<SHA-256 exact UTF-8 text>` with exact stored-index source refs and exact text digests.

DOG remains directly bound to SC1-SC12 and C1-C10. Every other candidate must remain in DOG's transitive hard closure. The real dogfood run retains v3 D1-D14, with these corrections:

- use the exact activated controller/package/validator/spec identities;
- provider transport is used only after runtime isolation qualification;
- the mid-execution change follows the owner's actual Gate C decision, not a baked-in finish rule;
- predecessor lost turns and supervision authorization are exercised adversarially;
- inspection distinguishes attempt, checkpoint, test, qualification, outcome evidence, and settlement;
- final independent evaluation cites durable evidence; only owner settlement marks the Goal complete.

## 14. Remaining uncertainties

1. **Core-binary installation trust.** Current Praxis lacks an implemented general binary replacement driver. V4 depends on a bounded external human-controlled bootstrap install plus exact post-install evidence. If that trust anchor is unacceptable, the prerequisite is a separately governed installation-lifecycle project, not another v4 unit.
2. **Historic binary reachability.** New provenance values make old contracts fail closed on v4, and the active path is digest-bound, but arbitrary copied binaries cannot be revoked by repository state. The deployment threat model must say whether active-path control is sufficient.
3. **Canonical specification schema.** Exact field names and canonical encoding must be frozen and independently reviewed before materialization. The semantics above are mandatory; this plan does not choose an unreviewed wire format.
4. **Ceremony evidence format.** It must bind TTY/OS-owner checks without pretending those checks are cryptographic personhood. Compatibility for legacy decisions needs adversarial review.
5. **Validation operation set.** It must be expressive enough for all 22 candidates without allowing workers to rewrite their own qualification. Unknown or insufficient predicates require proposal/profile revision.
6. **Gate A/B/C answers.** All remain undecided. Their downstream units may need a proposal successor if an answer invalidates a candidate specification.
7. **Provider OS containment.** Installed Claude flags are promising but do not alone prove external-write exclusion or managed-policy isolation. No provider is supported if the runtime qualification cannot close that gap.
8. **Bootstrap reviewer availability.** If no appropriately independent reviewer is available, v4 waits. Gate B cannot retroactively validate its own bootstrap review.

## 15. Independent verification required before v4 authority

An independent reviewer must, from committed bytes and active read-only state:

1. recompute the v4 plan digest and every bootstrap artifact digest;
2. rederive Goal identity/digest and all 31 authoritative elements from B-1/B-2/B-3;
3. reproduce Proposal v4 bytes and canonical digest using the then-current contract;
4. verify all 22 candidates, 68 relationships, exact kinds, sequences, priorities, refs, digests, provenance, no duplicates/self-edges/cycles, and DOG hard closure;
5. resolve every requirement, candidate, and relationship record from the canonical bundle and compare the accepted stored bytes;
6. inspect current source for actual enforcement at propose, review, request, accept, attach, selection, worker-context construction, completion, supervision, and restart;
7. prove the active binary/package/validator/spec identities match the activation manifest and that skew fails closed;
8. run the old-controller and gate-no-worker negative probes without repository mutation;
9. prove non-interactive decision input cannot persist v4 acceptance or any gate decision;
10. prove missing/drifted validator or candidate spec cannot admit a turn or complete a unit;
11. verify Gate C's dossier is neutral and complete and that no downstream code predetermines a disposition;
12. verify bootstrap reviewer independence/provenance and permanent RPP's downstream status are described truthfully;
13. assess the exact installed provider and OS-containment probe without treating help text or flag existence as isolation proof;
14. verify predecessor lost-turn and supervision fences before Gate C and exact authorized behavior after it;
15. verify inspection language never upgrades attempted/completed/tested/qualified into product proof;
16. confirm B6/B8 and all other authority-packet evidence exist at the reviewed commit and use the actual tracked paths;
17. confirm the working tree was clean at review start and report it at review end.

Any failure in items 1-16 is a pre-authority blocker. The remedy is revision or additional evidence, never an operator promise.

## 16. Planning result

- Advisory v4 candidates: 22.
- Relationships: 28 hard, 24 consumer, 13 interaction, 3 advisory; 68 total.
- Removed candidate: `unit:hi-authority-gate-mechanism`.
- New v4 candidates: none.
- Changed candidates: DOS, GA, GB, GC, ID, TRN, PLN, BND, FLOW, STAT, PKG, DOG.
- Gate questions preserved: A and B unchanged in substance; C expanded and de-predetermined.
- Bootstrap transition: explicit, pre-v4, exact-identity-bound, and partly dependent on an acknowledged external core-binary installation trust anchor.
- Proposal v4 JSON: intentionally not materialized.
- Authority: not requested or decided.
