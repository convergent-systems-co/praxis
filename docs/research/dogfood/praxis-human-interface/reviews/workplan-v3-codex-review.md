# REVISION_REQUIRED

## Review identity and method

- Goal: `praxis-human-interface/1`
- Repository evidence commit and review start `HEAD`: `6f310a9e40a1ed774de187f721ce5cb4d38436d9`
- Review start state: clean (`git status --short --branch` showed only the branch header)
- Frozen planning evidence: `docs/research/dogfood/praxis-human-interface/plans/workplan-v3-claude.md`
- Independently recomputed planning-evidence SHA-256: `sha256:966d193d57d41402e414435999aaac138c4af00162101f472ec475470ae963f5`
- Tracked proposal: `docs/research/dogfood/praxis-human-interface/bootstrap/praxis-human-interface-proposal-v3.json`
- Independently recomputed proposal-file SHA-256: `sha256:c482c45068c97dcd0d298be0555891765cfebe46cf19236556a83bcf79928f7f`
- Independently recomputed canonical `WorkPlanProposal` digest: `sha256:d18bcd5215c24ba2799fce0e49595a5cb17f07aadd64aa5acbb087e8dc37c4e2`
- Reviewer: Codex, non-Anthropic lineage, independent session. No planner context or prior conversation summary was used. Findings were derived from committed bytes, current source, installed CLI help, and one bounded non-mutating provider probe.
- No Praxis mutation, `propose`, review persistence, authority request/decision, acceptance, attachment, unit execution, commit, or push was performed.

## 1. Executive summary

Proposal v3 is much stronger than v2. Its tracked bootstrap is reproducible without `/tmp`; its 23 candidates, 71 relationships, 152 requirement references, and 31-element coverage are internally correct; its canonical proposal digest is `sha256:d18bcd52…c4e2`; and its final dogfood unit has every success criterion and constraint in both its direct bindings and its hard-dependency closure.

It nevertheless is not defensible for authority unchanged. Three defects are blocking:

1. **The proposed authority gates are not activated before they become runnable.** The current controller treats every selected candidate identically and always resolves a worker (`internal/goaldrive/controller.go:147-165`), then invokes it (`controller.go:88-112`). Proposal v3 puts the source-changing mechanism at sequence 2 and the three `gate:` candidates at sequences 3-5 (`workplan-v3-claude.md:192-196`), but governed installation/activation is postponed to unit 22 (`workplan-v3-claude.md:260`). Completing a source unit does not replace the running/installed controller. Therefore the current controller will launch a provider for Gate A rather than create an owner-only request. A worker can then make a progressing commit with the gate completion trailer; because the repository has no declared validator, current completion accepts `repository:no-declared-validation` (`internal/goaldrive/repository.go:447-467,514-570`). This directly contradicts the plan's claim that no provider is launched for gates (`workplan-v3-claude.md:226`).
2. **The validation contract is outside the WorkPlan and outside the authority-bound proposal.** R-1 requires an operator to add an executable, tracked `.praxis/validate` before execution and admits that no unit qualification is enforced without it (`workplan-v3-claude.md:452-454`). That is implementation work, not an environmental fact. It is neither a candidate nor a digest-bound precondition of acceptance. Current code explicitly treats no validator as sufficient for unit completion. The proposal therefore cannot preserve the asserted distinction between a unit being completed and its qualification passing.
3. **Gate C partly decides the policy it says belongs to the owner.** The plan describes live-turn disposition as a Gate C question, yet mandates that the live turn finishes untouched (`workplan-v3-claude.md:254`) and says this default is not gated (`workplan-v3-claude.md:462`). Constraint C6 forbids silent rewrite/destruction; it does not choose “finish” over an authenticated pause, cancellation with preserved consequence, or quarantine. This is a material execution/governance decision embedded in implementation.

Two further issues must be resolved in the same revision: the live non-interactive decision surface remains usable for v3's own `workplan.accept`, and accepted candidate specifications are still conveyed by a path plus an unchecked digest. The plan acknowledges both underlying facts but does not close them before they matter.

This is `REVISION_REQUIRED`, not `AUTHORITY_CONFLICT`: the Goal does not prohibit the intended design. It is not `INSUFFICIENT_EVIDENCE`: the tracked evidence is sufficient to establish both the valid structural properties and the blocking execution defects.

## 2. V2 finding-resolution matrix

The two committed v2 reviews were read as historical evidence, not trusted as conclusions. “Evidence” below cites current repository facts or exact v3 provisions.

| Review / finding | Classification | Independent v3 assessment and evidence |
|---|---|---|
| Codex V1-1 — requirement identity only partially resolved; frozen source/manifest absent and semantics unenforced | RESOLVED | B-1/B-2/B-3 and the tracked proposal now exist and reproduce exactly. Unit 6 explicitly adds baseline resolution and a baseline-derived denominator (`workplan-v3-claude.md:228`). Current enforcement is still absent, but that absence is correctly work to be implemented rather than a false current claim. |
| Codex V1-2 — reviewer principal and caller-asserted `ReviewDigest` | PARTIALLY_RESOLVED | Gate B, content-addressed review bytes, controller-computed digest, and resolved provenance are specified. Bootstrap still relies on convention, and current `WorkPlanProposalReview.Validate` accepts any non-empty digest and string-level independence (`pkg/contracts/work_plan.go:199-247`). Gate activation is also broken. |
| Codex V1-3 — ontology | RESOLVED | The material establishment/ontology decision is separated into Gate A; ONT is limited to recording the ratified result (`workplan-v3-claude.md:244`). |
| Codex V1-4 — staging/intake/transport isolation | PARTIALLY_RESOLVED | The T1 profile now names exact flags, scratch cwd, bounded output/exit, process-group kill, and fail-closed probes (`workplan-v3-claude.md:270-301`). The exact flags compose on installed Claude 2.1.278, but the required ambient canary suite and external-write observation are not yet evidence. |
| Codex V1-5 — operational identity | RESOLVED | Single-use attempt-scoped allocation remains explicit and qualified (`workplan-v3-claude.md:230`). |
| Codex V1-6 — successor/supersession policy embedded in units | UNRESOLVED | Gate C is explicit, but its enforcement cannot activate before the gate runs, and “live turn finishes” is predetermined outside the gate (`workplan-v3-claude.md:254,462`). |
| Codex V1-7 — flow/status distinction | RESOLVED | Status remains evidence-only and settlement-only for Goal completion (`workplan-v3-claude.md:442-444`). Current inspection also labels the completion candidate non-authoritative (`cmd/praxis/goals_lifecycle.go:459-491`). |
| Codex V1-8 — installed surface skew | RESOLVED | Units 10 and 22 explicitly distinguish source, embedded binary, and installed package generation and require real activation-path qualification (`workplan-v3-claude.md:236,260`). |
| Codex V1-9 — final qualification omitted routing | RESOLVED | `DOG → RTE` is hard edge 31, and RTE is in DOG's hard closure. |
| Codex N1 — provider transport not least-privilege isolated | PARTIALLY_RESOLVED | Exact isolation and bounded-launch design is materially improved. Installed help corroborates the flags. A bounded probe returned valid structured output with zero web requests, zero subagents, and no scratch files, but did not prove suppression of all ambient managed configuration or writes outside the scratch tree. |
| Codex N2 — bootstrap artifacts absent; review digest conventional | PARTIALLY_RESOLVED | Artifact absence is resolved. The review-digest limitation remains current and is truthfully deferred to unit 13. Required B-6 v3 provenance and B-8 persisted-state evidence are not present in the bootstrap commit. |
| Codex N3 — governance decisions hidden in executable units | UNRESOLVED | The questions are named as gates, but the gates are executable by the old controller before the new mechanism is activated; Gate C also preselects a live-turn default. |
| Codex N4 — operational total order | SUPERSEDED_BY_EVIDENCE | V3 accurately describes current single-track execution and treats unique sequence as deterministic tie-breaking, not parallel architecture (`workplan-v3-claude.md:178-186`). Independent graph checks found no sequence/edge contradiction. |
| Codex dependency — downgrade INTERP→EST | RESOLVED | It is now `consumer`; both independently share Gate A, and integration is deferred to FLOW/DOG. |
| Codex dependency — add DOG→RTE hard | RESOLVED | Present as hard edge 31 and required by D7. |
| Claude matrix 1 — identity evidence only | RESOLVED | Durable B-1/B-2/B-3 plus unit 6's enforcement scope address the finding without claiming current enforcement. |
| Claude matrix 2 / N3 — reviewer principal and no model-independent path | PARTIALLY_RESOLVED | The bootstrap cross-lineage rule is clear and this review satisfies its lineage requirement. Permanent enforcement remains downstream of a gate that cannot currently enforce itself. |
| Claude matrix 3 / N2 — ontology and material governance decisions lack gates | UNRESOLVED | Gate decomposition is conceptually correct but not executable safely because activation is missing; Gate C is also partly predetermined. |
| Claude matrix 4 / N1 — T1 unsafe | PARTIALLY_RESOLVED | Exact profile and qualification plan exist; full canary/containment evidence does not. |
| Claude matrices 5-7 and 9 — identity, successor mechanics, flow/status, final binding | PARTIALLY_RESOLVED | Identity and final hard closure are correct. Successor mechanics remain blocked by Gate C's policy/activation defect. Flow/status itself is resolved. |
| Claude matrix 8 / N7 — installed skew stronger than stated | RESOLVED | Unit 10 now tests installed package, embedded package, and source separately; unit 22 owns governed installation. |
| Claude N4 — relationship misclassifications | RESOLVED | TRN→INT and EVO→EST are hard; RPP→INT is consumer; establishment authority was split into Gate A rather than forcing EST→ONT. Independent review found these classifications defensible. |
| Claude N5 — bootstrap not frozen; proposal ignored; `/tmp` source; worker source digest unchecked | PARTIALLY_RESOLVED | Bootstrap and proposal tracking are resolved and no verifier reads `/tmp`. Runtime candidate-specification resolution remains unchecked: `WorkPlanProposal.Validate` requires only non-empty candidate source fields (`pkg/contracts/work_plan.go:121-143`), and unit 6 scopes its resolver to `RequirementRef`, not candidate plan fragments (`workplan-v3-claude.md:228`). |
| Claude N6 — unit 12 lacked C2/SC2; C4/SC4 owner unclear | RESOLVED | INTERP now binds SC2/C2/C4/SC4; EST/FLOW own deterministic rendering and material questioning. |
| Claude N8 — dogfood needs human input; command idempotency | RESOLVED | D2/D14 explicitly require a real human-authored requirement, and FLOW requires restart without duplicate mutation. |
| Claude edge TRN→INT hard | RESOLVED | Present and justified by the result-source and failure contracts. |
| Claude edge EVO→EST hard | RESOLVED | Present and justified by the source-binding primitive. |
| Claude edge EST→ONT hard or split | RESOLVED | The material decision is split to Gate A; EST and ONT are each hard on GA. |
| Claude edge RPP→INT consumer | RESOLVED | Present as consumer. |

## 3. V3-new-finding verification matrix

| Planner-reported v3 finding | Verification | Treatment sufficiency |
|---|---|---|
| Non-interactive `goals-lifecycle --operation=decide` bypass | VERIFIED | INSUFFICIENT. The path directly calls `SaveAuthorityDecision` without terminal or OS-owner checks (`cmd/praxis/goals_lifecycle.go:211-222`), whereas `authority decide` requires both (`cmd/praxis/authority_decide.go:75-152`). Unit 2 closes only the future gate kind and explicitly leaves v3's own acceptance exposed. Before activation, it also exposes the gates themselves. |
| Unit completion lacks an adequate validation contract | VERIFIED | INSUFFICIENT. `.praxis` is ignored (`.gitignore:25-27`); no tracked validator exists; absence is accepted as completion evidence (`internal/goaldrive/repository.go:447-467,514-570`). R-1 is ungoverned work outside the proposal. |
| Weaker-kind prerequisites are described as “done” | VERIFIED | PARTIALLY SUFFICIENT. `BuildWorkerContext` places every relationship kind in `Prerequisites` (`internal/goaldrive/execution_contract.go:255-265`), and the prompt calls them “already accepted as done or not your concern” (`provider_worker.go:410-415`). Unit 6 proposes the right repair, but no activation boundary ensures later workers receive the repaired context. |
| Hard-dependency cycles silently block | VERIFIED | SUFFICIENT AS A CONTRACT CHANGE, subject to activation. `AssessWorkCandidates` reports blocked when no candidate is ready and has no cycle check (`pkg/contracts/work_selection.go:112-142`). Rejecting cycles at propose and accept is the right treatment. The actual v3 hard graph is acyclic. |
| Digest canonicalization is unstable for duplicate endpoint pairs | PARTIALLY_VERIFIED | SUFFICIENT AS A CONTRACT CHANGE. `Digest` sorts relationships only by dependent/prerequisite, not kind (`pkg/contracts/work_plan.go:250-269`); semantically equivalent duplicate-pair inputs in different orders can hash differently. No intrinsic unordered-map iteration in `Digest` was found, so that precise causal wording was not reproduced. Rejecting duplicate endpoint pairs removes the ambiguity. Actual v3 has none. |
| Lost-turn reconciliation is per generation | VERIFIED | SUFFICIENTLY SPECIFIED, subject to Gate C correction/activation. `Admit` and `LostTurns` load admissions by exact goal/version (`internal/goaldrive/admission.go:303-331,362-383`), while the checkout lease scope crosses generations. Unit 19 correctly requires predecessor lost admissions to be reconciled before governing handoff. |
| Supervision stop authority is unauthenticated | VERIFIED | PARTIALLY SUFFICIENT. `--actor-id/--actor-kind` are caller fields (`cmd/praxis/supervision.go:58-90`); `appendHumanIntervention` validates only their shape and writes `TrustUserConfirmed` (`supervision.go:274-299`). Gate C names interruption authority, but its mandatory “finish” default and activation defect prevent safe enforcement. |

## 4. Bootstrap-integrity assessment

### Independently recomputed values

| Property | Result |
|---|---|
| B-1 tracked source SHA-256 | `sha256:1881e31e9185b3c517c8a8b7eecf1aa965edcd9c8a7388883e4016136b6b8f0e` |
| Goal identity/version | `praxis-human-interface/1` |
| Canonical Goal digest | `sha256:afda0866398d14a223f3a090d554c820e9e945922a936e6f1edb9c06d9177530` |
| Recorded `ImportSourceDigest` | `sha256:1881e31e9185b3c517c8a8b7eecf1aa965edcd9c8a7388883e4016136b6b8f0e` |
| B-2 file SHA-256 | `sha256:c0144606e29d0dde95d672e23163521677cfb18d3db0da0634bc8210e68be95a` |
| B-2 element-list digest | `sha256:8579dae2b561fb7b8717abd065333c030c0df5588323a50c64eeb98d7b8d8bfb` |
| B-3 file SHA-256 | `sha256:4f3faa2cb479d54e262020169d4e88fee53d638b0f7db591742132449effed92` |
| Inspect capture SHA-256 | `sha256:4bd7b2dd85cef92f558960116cd956bd239c39c5e72f6a3eb00ba2aa793a609d` |
| Inspect metadata SHA-256 | `sha256:8272dfcf393ad0fb56f4e694f9a8c8f7a63d65d0a014ad67bf440d9a3b8d6ab9` |
| Planning evidence SHA-256 | `sha256:966d193d57d41402e414435999aaac138c4af00162101f472ec475470ae963f5` |
| Proposal file SHA-256 | `sha256:c482c45068c97dcd0d298be0555891765cfebe46cf19236556a83bcf79928f7f` |

B-2 is a deterministic 31-row manifest derived from B-1: 12 success criteria, 10 constraints, 5 non-goals, and 4 assumptions. For every row I independently recomputed exact UTF-8 text SHA-256, content-derived ID `<kind>:sha256:<text-hash>`, stored index, and proposal `source_ref`. There are no duplicate `(kind,text-hash)` elements.

The tracked inspect capture contains the exact same list text and order as B-1/B-2 and reports the exact Goal and import digests. The capture metadata binds those bytes to an inspect command run by binary commit `f539b74f…`; that commit exists and its `inspectGoalsLifecycle` implementation performs loads/listing only (`cmd/praxis/goals_lifecycle.go` at that commit, lines 360-522). This is materially stronger than trusting a `read_only: true` assertion. The metadata is still an operator attestation, not cryptographic proof that the captured stdout came from that database; no stronger provenance mechanism exists in the bootstrap.

Both committed verification programs operate only on tracked paths. `materialize_proposal_v3.py --check` regenerated the proposal byte-for-byte, and `bootstrap_evidence.py verify` succeeded. I separately derived all element identities and bindings from B-1 rather than accepting the proposal's claimed set. Therefore Proposal v3 can be reproduced and content-verified entirely from tracked repository evidence without reading the original `/tmp` source. Matching `ImportSourceDigest` makes B-1 byte-equivalent to the imported establishment document under the normal SHA-256 integrity assumption; the obsolete recorded path is not needed.

Planning-section path names drifted: section 7 asks for B-3 `.txt` and a proposal under `proposals/`, whereas the committed artifacts are JSON and under `bootstrap/`. This does not break the executable verifier, whose paths are explicit, but the plan should be corrected so the authority packet names the artifacts that actually exist. Required B-6 provenance for this v3 review and B-8 persisted v3 state are not present at the reviewed commit; they remain pre-authority evidence, not facts established by this review.

## 5. Proposal-contract and requirement-coverage assessment

### Current contract integrity

The tracked outer document names baseline `praxis-human-interface/1` and proposal store version `3`. The actual proposal ID is `praxis-human-interface-workplan-proposal-3`; proposer is `model:claude-sonnet-5:advisory-plan-materializer` of kind `model`; generation is `claude-sonnet-5/advisory-plan-v3@sha256:966d…63f5`.

Independent checks found:

- 23 unique candidate IDs;
- sequences exactly 1 through 23, all unique;
- priority histogram `{1:5, 2:7, 3:3, 4:2, 5:4, 6:1, 7:1}`;
- every candidate and relationship has `model_proposal` provenance;
- 71 relationships: 31 hard, 24 consumer, 13 interaction, 3 advisory;
- no self-edge, duplicate endpoint pair, or invalid endpoint;
- hard graph acyclic with all 23 nodes in a topological ordering;
- every relationship points from a higher sequence to a lower sequence;
- no transitively redundant hard edge;
- all 22 non-DOG candidates are in DOG's transitive hard closure.

Current `WorkPlanProposal.Validate` checks non-empty proposal/baseline identity, proposer, unique candidate IDs, non-empty candidate provenance fields, non-empty requirement refs, known relationship kinds, endpoint existence, and no self-edge (`pkg/contracts/work_plan.go:111-170`). It does **not** validate requirement resolution, priority/sequence, duplicate relationship pairs, or cycles. `BuildWorkPlanProposal` verifies the baseline digest and then calls that validation (`packages/goals/workplan_proposal.go:13-21`); the lifecycle `propose` path reconstructs Goal identity/digest from the loaded baseline and persists the result (`cmd/praxis/goals_lifecycle.go:96-120`). The tracked JSON passes exactly that current path when its outer baseline fields are used.

I recomputed canonical bytes independently by constructing the Go struct field order, injecting the loaded Goal identity/version/digest exactly as the propose path does, sorting candidates by ID and relationships by `(dependent, prerequisite)`, and SHA-256 hashing the compact JSON. Result: `sha256:d18bcd5215c24ba2799fce0e49595a5cb17f07aadd64aa5acbb087e8dc37c4e2`. A separate execution of the current Go `Validate`/`Digest` code produced the same result.

### Authoritative coverage

Coverage was derived from B-1/B-2, not from proposal claims:

| Kind | Authoritative | Covered by proposal | Result |
|---|---:|---:|---|
| Success criteria | 12 | 12 | complete |
| Constraints | 10 | 10 | complete |
| Non-goals | 5 | 5 | complete |
| Assumptions | 4 | 4 | complete |
| Total distinct elements | 31 | 31 | complete |

All 152 occurrences resolve to the exact stored-index element and have the correct content ID and digest. No candidate repeats a requirement ID.

Final qualification is directly bound to SC1-SC12 and C1-C10. Its hard closure contains every other candidate, including routing, transport, interpretation, evolution boundary, packaging, status, and all gates. D2-D14 exercise a real next requirement; D14 is an explicit second statement of SC12. Thus the coverage and final-qualification *decomposition* are complete. The proof is not executable safely until the activation and validation defects are repaired.

## 6. Human-authority-gate assessment

### Gate A — authority ceremony and establishment

The dossier questions are appropriately human: what establishment authoritativeness means, whether establishment requires confirmation, how typed confirmation may change, which evolution classes require confirmation, inferred material elements, and direct changed-contract succession. ONT/EST/INTERP/EVO are correctly downstream of Gate A.

The gate is not enforceable in the proposed order. Source implementation by MECH cannot affect the installed/current controller that selects GA immediately afterward. The plan's only explicit install step is PKG at sequence 22. No dependency or precondition binds an activated controller digest before GA/GB/GC. This allows worker execution to self-complete every gate.

### Gate B — reviewer independence

The policy questions are genuine and should remain undecided: owner-as-reviewer, lineage separation, provenance requirements, and whether adverse reviews can be overridden. The bootstrap rule appropriately requires a non-Anthropic reviewer for this Claude-authored proposal. Current contracts only compare strings and ignore `ReviewerProvider`, so a permanent mechanism is necessary.

Gate B has the same activation flaw. In addition, section 8's “blocking rule” for adverse reviews is a bootstrap convention not enforced by current request creation. The authority packet must prove the rule was followed; it cannot claim Praxis enforced it.

### Gate C — supersession and active turns

Unsettled-generation disposition, interruption authority, and treatment of inapplicable work are correctly identified as human decisions. Per-generation lost-turn visibility and unauthenticated `supervise cancel|suspend` are real current defects.

However, the plan removes the most important default from the owner's choice by mandating that a live turn finishes. C6 only protects provider-owned work from silent rewrite/destruction. A paused or cancelled turn with durable consequence preservation can also satisfy C6. Gate C must present finish, authenticated interrupt-and-preserve, and other defensible choices without hard-coding one into BND before the decision.

### Route-around analysis

Current workers are told that modifying Praxis durable state is forbidden, but governance cannot rely on prompt obedience. Two concrete route-arounds exist:

1. Before the new mechanism is activated, current Goal-drive launches a provider for a `gate:` candidate; ordinary completion mechanics can complete it.
2. `goals-lifecycle --operation=decide` remains non-interactive and accepts a hand-authored decision document. The proposed rejection is scoped only to `goal.gate.decide`; it does not protect v3's own `workplan.accept`, and it is ineffective for gates until activated.

A revision must place a governed activation boundary before any gate candidate is selectable, bind the exact activated controller/package digest, and close or cryptographically constrain every decision-writing surface for the new gate kind. V3 acceptance itself should not depend solely on an operator attestation that the bypass was not used.

## 7. Provider-isolation assessment

Installed capabilities were checked directly:

- `claude` is version `2.1.278`; its help defines `--restricted`, `--safe-mode`, `--strict-mcp-config`, `--tools ""`, `--disable-slash-commands`, `--permission-prompts none`, `--no-session-persistence`, structured output, and budget limits with the meanings quoted by the plan.
- `codex` is `0.155.1`; its help confirms `--sandbox read-only` but does not document read confinement comparable to Claude restricted mode. Deferring Codex advisory transport is correct.
- Current Praxis Claude execution is not the proposed advisory launcher: it uses repository cwd, `acceptEdits`, `--add-dir`, and a large allowed-tool list (`internal/goaldrive/provider_worker.go:526-539`). The plan correctly calls for a separate launcher.

A bounded probe ran the exact proposed Claude isolation flags from an empty 0700-style scratch directory outside any worktree, with no repository path in the prompt and a $0.10 budget. It exited successfully in 5.5 seconds and returned schema-valid `{"ok":true}`. Reported telemetry showed zero web requests, zero subagents, no permission denials, and the scratch directory remained empty. The wrapper result used structured output internally (`stop_reason: tool_use`), but exposed no external tool use.

This proves flag compatibility, structured-output location, and bounded success for the installed version. It does **not** prove all ambient hooks, user/project settings, managed policy settings, claude.ai connectors, plugin MCP, memory, or writes elsewhere are absent. The help states that safe mode disables CLAUDE.md, skills, plugins, hooks, MCP, commands, agents, and other customizations; restricted mode ignores user/project/local settings and confines file tools; strict MCP with no config excludes configured MCP; and `--tools ""` removes built-ins. Those are good layered controls. `HOME` and `CLAUDE_CONFIG_DIR` remain visible, admin policy still applies, and there is no OS sandbox around the CLI process.

Conclusion: the proposed T1 profile is a defensible candidate for a first advisory transport **only if** unit 11's exact-version canary probe, process-group deadline/kill, output bounds, zero-tool/runtime inventory where available, checkout invariants, and fail-closed version drift are implemented and recorded. “No file created outside scratch cwd” needs an explicit observation mechanism or OS containment; a scratch-directory listing cannot prove it. The proposal must not claim genuine isolation before that qualification exists.

## 8. Dependency findings

### Correct findings

- `DOG → RTE` is properly hard and fixes v2's final-qualification gap.
- `INTERP → EST` is correctly downgraded to consumer; Gate A, FLOW, and DOG provide the necessary integration points.
- `TRN → INT`, `EVO → EST`, `RPP → ID`, and the downstream gate dependencies are substantively justified.
- No direct hard edge is transitively implied by another hard path; no hard cycle exists.
- Non-hard edges are not readiness gates under current code. Their sequence-forward layout reduces misleading worker context but does not create architectural authority.

### Missing hard/activation dependencies

1. **Each gate needs an activated mechanism, not merely completed source.** Add a governed activation/package unit (or a staged-generation boundary using already-active machinery) and make GA/GB/GC hard-dependent on its durable activated digest. `GA/GB/GC → MECH` is insufficient.
2. **Execution needs a digest-bound validator before the first unit.** R-1 must be committed bootstrap evidence and an acceptance/drive precondition, or become governed work executed under an already adequate validation mechanism. It cannot remain an operator-authored side task.
3. **Candidate specification resolution needs enforcement.** An early contract unit or acceptance precondition must verify each candidate/relationship `source_ref` against `source_digest`, preserve the exact accepted specification bytes, and inject the resolved text into worker context. Requirement-ref enforcement alone does not close this provenance gap.

### Relationship-kind observations

No existing hard edge should be downgraded merely for concurrency: current Goal-drive is single-track, and the hard edges express truth conditions rather than throughput. The 24 consumer, 13 interaction, and 3 advisory edges are reasonable documentation relationships. The missing relationships above concern activation and proof prerequisites that are not represented by any present candidate.

## 9. Proof and qualification assessment

Current Praxis correctly distinguishes several layers in its data model:

1. A provider process ending is only `provider-process:completed`.
2. A progressing, clean, published checkpoint can support a durable unit completion.
3. Structural completion/coverage is explicitly “never criterion satisfaction” (`internal/goaldrive/completion.go:218-230`).
4. Goal completion requires evaluation and a settlement decision; inspection labels a candidate non-authoritative and “never proof” (`cmd/praxis/goals_lifecycle.go:459-491`).

The break is between layers 2 and 3. `unitCompletionPredicates` accepts either `declared-validation-passed` **or** `no-declared-validation` (`internal/goaldrive/repository.go:514-531`). Thus a durable “unit complete” record currently proves a published progressing commit and a trailer, not mechanism tests, unit conformance, or the candidate's stated qualification. V3 knows this but places the remedy outside its authority-bound work.

The final DOG unit's D1-D14 path is a meaningful conformance/product qualification design, and settlement remains the only source of Goal “complete.” A revision must ensure that:

- `completed` never renders as `qualified` unless a digest-bound validation contract ran;
- unit tests/mechanism tests remain distinct from conformance qualification;
- DOG's evidence is stored and bound to an independent evaluation;
- “product outcome proven” is not rendered from structural completion, provider statements, or test success alone;
- a human settlement can say complete/satisfied based on evidence without rewriting historical facts into mathematical “proof.”

## 10. Scope and architecture assessment

Most units stay within the Goal and compose existing GoalStore, WorkPlan, authority, supervision, recovery, settlement, successor, SecureBlob, and installed-package mechanisms. The proposal does not settle the Story/Intent/Requirement/Goal ontology in advance, and provider routing remains operator-overridable.

The gate mechanism is not automatically a parallel governance system: an owner-only `AuthorityRequest` plus a controller completion predicate can be a bounded extension of existing machinery. But encoding a new control-node type solely through the `gate:` ID namespace, then relying on source changes that are not active, is not a valid architecture. The revision should use an explicit contract/type and an activated controller boundary, or use staged Goal generations with currently enforced semantics.

R-1 is unnecessary out-of-band machinery as written. Validation infrastructure is part of governed implementation/proof and must be inside the proposal or frozen as an authority-bound bootstrap precondition. Likewise, a plan file path is not a durable specification store merely because its SHA is copied into `source_digest`; resolution must be enforced.

Gate C's mandated finish behavior is the only premature material product/governance decision found. The plan's other recommendations are clearly labelled non-binding and leave owner choices open.

## 11. New blocking findings

### B1 — Gate mechanism has no activation boundary

**Severity: critical.** MECH completion changes repository source, not the controller selecting the next unit. PKG installs only at unit 22. Current code has no `gate:` special case and launches a worker for the selected candidate. With no validator, a worker can durably complete GA/GB/GC. Required revision: activate and bind the mechanism before gates become selectable, and prove the old controller cannot execute them as ordinary work.

### B2 — Required validation is ungoverned and unbound

**Severity: high.** R-1 asks a human/operator to add executable repository code outside the WorkPlan. Acceptance does not bind its bytes, and drive does not require it. Required revision: commit and digest-bind the validator before authority or add a governed bootstrap/activation design that cannot complete units without it.

### B3 — Gate C predetermines live-turn disposition

**Severity: high.** “Live turn finishes untouched” is not entailed by C6 and is simultaneously presented as implementation law and a gate question. Required revision: make all material live-turn alternatives owner-selectable; downstream BND implements only the exact approved record.

### B4 — Non-interactive decision bypass remains relevant before and during v3

**Severity: high.** A procedural promise to use interactive `authority decide` does not preserve a deterministic authority boundary. Required revision: close or strictly constrain the non-interactive decision mutation before it can satisfy v3 acceptance or any gate, with compatibility handling explicitly reviewed.

### B5 — Accepted unit specifications are not resolved against their digests

**Severity: medium-high.** Workers receive an ID, requirement IDs/refs, and a plan path; the controller neither resolves the candidate fragment nor compares the file to `source_digest`. Unit 6 does not explicitly fix candidate/relationship specification resolution. Required revision: content-address and inject exact accepted unit text, or validate the referenced artifact at selection and fail closed on drift.

### B6 — Authority packet is not yet complete

**Severity: procedural blocker, not a design defect.** At commit `6f310a9e…`, v3 was not persisted and therefore B-8 does not exist; this v3 review/provenance and its third-party digest also did not yet exist. The plan itself requires these before authority. They must be produced after revision without weakening the independence rule.

## 12. Exact rationale for disposition

`REVISION_REQUIRED` is the only defensible disposition.

The proposal's bytes, canonical digest, requirement bindings, coverage, graph structure, dogfood closure, and most v2 corrections are verifiable and sound. Evidence is not insufficient. There is also no irreconcilable conflict with the authoritative Goal.

Authority should nevertheless not be asked to accept this exact proposal because its central protection—three non-self-clearable human gates—does not exist in the controller that would encounter those candidates. The proposal confuses source completion with runtime activation, and current completion semantics make the resulting bypass concrete. It also requires ungoverned validator implementation outside the WorkPlan and embeds a Gate C choice before the owner decides it. These are authority/proof defects, not editorial nits.

A defensible v4 should, at minimum:

1. add a governed, digest-bound activation boundary before any gate candidate;
2. make the controller fail closed on `gate:` candidates until that exact mechanism is active;
3. bind a tracked validator as a pre-authority/pre-drive artifact or govern its creation without permitting unvalidated completions;
4. leave live-turn disposition fully to Gate C;
5. close the non-interactive decision bypass for v3 acceptance and gate decisions;
6. resolve candidate/relationship specification bytes against their recorded digests;
7. regenerate the proposal/digest/coverage/graph evidence and then produce B-6/B-8 for the revised proposal.

## Verification commands and results

- `python3 .../verify/bootstrap_evidence.py verify` — PASS.
- `python3 .../verify/materialize_proposal_v3.py --check` — PASS; 23 candidates, 71 relationships, 152 refs.
- Current Go `WorkPlanProposal.Validate/Digest` — PASS; digest `sha256:d18bcd5215c24ba2799fce0e49595a5cb17f07aadd64aa5acbb087e8dc37c4e2`.
- Independent Python reconstruction of Go canonical bytes — 70,208 bytes; same digest.
- Independent graph/binding check — PASS; 31/31 elements, no bad refs, duplicate pairs, self-edges, cycles, sequence inversions, or redundant hard edges.
- `go test ./pkg/contracts ./packages/goals ./internal/goaldrive ./cmd/praxis` with `GOCACHE=/private/tmp/praxis-v3-go-cache` — PASS. The first attempt failed only because the sandbox denied the default user Go cache; redirecting the cache resolved it.
- Installed CLI check — Claude Code `2.1.278`, Codex CLI `0.155.1`.
- Bounded Claude T1 flag-composition probe — PASS for structured output and exit; not a complete ambient-isolation qualification.

