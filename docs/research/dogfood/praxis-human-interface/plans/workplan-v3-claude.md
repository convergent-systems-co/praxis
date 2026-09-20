# Advisory WorkPlan proposal v3: `praxis-human-interface/1`

Advisory model output only. Every candidate and relationship below is destined for `"provenance": "model_proposal"`.
This plan grants no authority, does not review itself, decides no human-governed question, and modified nothing in the repository.
Praxis was not invoked. No provider session was run. `proposal-v3.json` is **not** written by this plan.

- Goal `praxis-human-interface/1`, digest `sha256:afda0866398d14a223f3a090d554c820e9e945922a936e6f1edb9c06d9177530` (unchanged; no new human requirements)
- Superseded: v1 `sha256:c3903f9f…9030` (review `b131b45b…efcb`, REVISION_REQUIRED); v2 `sha256:0a451d8b…a198` (planning evidence `6070c772…922f`), two v2 reviews, both REVISION_REQUIRED
- Review evidence used: the Codex v2 review (text supplied by the user; no file exists in the repo yet) and the Claude v2 review (this session's earlier output; no file exists yet). **Both must be committed under `reviews/` before v3 authority** (B-6).
- Repo HEAD at planning time `8f0b996`, working tree clean.

---

## 0. Executive summary

v3 is a surgical revision of v2: **23 candidates** (v2 had 18; +5), **71 relationships** (31 hard, 24 consumer, 13 interaction, 3 advisory).
The 18-unit architecture survived and is preserved. What changed:

1. The first supported Claude transport gets an exact isolation profile (`--restricted --safe-mode --strict-mcp-config --tools "" …`) with per-version probe, process-group kill and fail-closed rules. Codex stays unsupported.
2. Three **explicit human-authority gates** are added, each preceded by a shared evidence/recommendation dossier and backed by one small enabling mechanism, so an executing agent cannot self-clear a governance decision.
3. Bootstrap evidence is specified as **tracked, digest-pinned files** (nothing in `/tmp` or `.praxis/`), and "independently verified" is separated from "enforced by Praxis".
4. A bootstrap reviewer rule (cross-lineage reviewer, no shared context, independent recomputation) covers the period before `hi-review-principal-provenance` exists.
5. Hard edges are reconciled from primary evidence: one added (dogfood → routing), one downgraded (interpretation → establishment), several added for real qualification boundaries, two removed as transitively implied. Priority/sequence is re-defined honestly as scheduling preference, not architecture.

---

## 1. Verified facts this revision relies on

Traced this session from code, installed CLIs and read-only queries (v2 facts not repeated unless changed).

**Selector and plan consumption** (`pkg/contracts/work_selection.go`, `work_relationship.go`, `internal/goaldrive/*`)
- Only `hard_dependency` gates readiness (`work_relationship.go:81`). Consumer, interaction and advisory edges only pass `Validate()`; a malformed edge of any kind fails the whole selection.
- Ready set is sorted by `(Priority, Sequence, ID)`; equal top `(priority, sequence)` fails closed with `ErrAmbiguousWorkChoice`. Nothing enforces global uniqueness of either. Priority is a hard ordering **among ready units**, never a gate.
- Goal-drive executes one candidate at a time (`runtime.go`), under an exclusive scope lease `goal-drive-scope:<checkout>|<branch>`. Nothing marks a unit in-progress, so parallel runners in separate worktrees would pick the same unit. Concurrency is not modelled anywhere.
- Hard cycles silently block their members (no cycle detection at propose/accept). Duplicate relationship pairs are accepted; two kinds on one pair makes `Digest()` ordering non-deterministic (non-stable sort on `(Dependent, Prerequisite)`).
- `BuildWorkerContext` lists **all** relationship kinds as "Prerequisite units (already accepted as done or not your concern)" (`execution_contract.go:256-265`, `provider_worker.go:414`), which is false for consumer/interaction/advisory prerequisites.
- Workers receive no unit description: only candidate ID, requirement lines (`ID: SourceRef`, no text), prerequisites/dependents and the full Goal text. A unit's specification reaches the worker only via its `source_ref` into the plan document, and nothing checks the file against `source_digest`.
- Unit completion is mechanical (ADR-099): trailer, validated progress, published checkpoint, declared validation passed **or none declared**. There is **no** `.praxis/validate` in the repo, and `.praxis/` is git-ignored (`.gitignore:26`, 0 tracked files, validation path constant `.praxis/validate`, `git_repository.go:114`). Today every unit completion would record `repository:no-declared-validation` and succeed. Per-unit "qualification" is therefore aspirational unless a tracked validation contract exists (Section 11, R-1).

**Authority machinery**
- Human-stop-and-decide exists for exactly one flow: `workplan.accept` (plus settlement/succession ledger events and owner-only repair kinds). `WorkCandidate` has no kind or human flag; completion is worker-attested; `AuthorityRequest`s bind a whole Goal generation and surface (`authority.required`) only when **no** runnable work remains (SPEC-027: a pending request never blocks a runnable sibling). `AffectedWork`/`TransitivelyBlocked` fields exist but nothing populates them.
- `authority decide` needs interactive TTY + OS user = root owner + typed `DECIDE-<OUTCOME> <digest>`; decision has **no reason field** (the `--reason` is printed, not persisted); the request `Reason` **is** persisted and digest-covered.
- **New finding:** `goals-lifecycle --operation=decide --input=<hand-authored decision>` is still live (`goals_lifecycle.go:211`), has no interactive gate or OS-user check, and can author an "owner" decision from non-secret durable state. ADR-097's "no non-interactive path" holds only for `authority decide`. The same path could satisfy `workplan.accept`, i.e. v3's own authority transition (Section 12).
- `supervise cancel|suspend` stop the provider process for anyone with DB write access; the actor is caller-asserted; no owner check.
- Establishment/import need only `PRAXIS_DB` + bootstrap-record possession: no confirmation, no authority request (SPEC-023 calls it "the supported non-interactive admission boundary"). Code and ADR-068 say "authoritative"; SPEC-014 invariant 2 (line 23) says a Baseline is "derived knowledge, never execution authority". No document reconciles this.
- ADR-099: successor after any settlement, same contract, empty ledger, predecessor completions are "evidence for N+1, never proof of it"; "cancellation, abandonment, and supersession are further dispositions the status enum leaves room for" (undefined). Settlement statuses today: `complete`, `incomplete` only, and a candidate exists only when **every** unit is complete. `governingSuperseded` needs a recorded succession, so an unsettled generation is never blocked by a newer one and nothing re-checks authority at drive time. Live turns finish on their own generation.
- Review: coverage denominator is the proposal's own requirement IDs; selector-path `ReviewDigest = sha256(reviewRef+proposalDigest+status)`; full-document path accepts any non-empty `ReviewDigest`; `ReviewerProvider` unread; reviewer ≠ proposer is string inequality; `AcceptWorkPlan` never compares reviewer with decider. A durable review record protects its own serialized bytes (`LoadWorkPlanReview` re-hashes) but does not bind any external document.
- Duplicate element text within a list is silently accepted by `GoalBaseline.Validate` and `CanonicalBytes` (sorted, not deduped). Nothing today resolves `RequirementRef` against a baseline; completion coverage parses only `#success_criteria/<n>` against **stored** order, ignoring anything else.
- Routing: `--provider` is mandatory; SPEC-051 routing is not imported by `goaldrive`/`cmd/praxis`. Unit 9 is outside the transitive **hard** closure of v2's unit 18; it reached 18 only because priority 3 sorts before 7.

**Provider surface (installed Claude Code 2.1.278, codex-cli 0.155.1; quotes from `--help`)**
- `--restricted`: removes command/code-running tools and WebFetch unless `--tools` names them; **ignores user, project and local settings files** (managed settings and `--settings` still apply; "add `--strict-mcp-config` to skip MCP servers too"); confines file tools to working directories. Says nothing about CLAUDE.md, auto-memory, skills, plugins, hooks, claude.ai connectors.
- `--safe-mode`: "all customizations (CLAUDE.md, skills, plugins, hooks, MCP servers, custom commands and agents, output styles, workflows, …) disabled … Admin-managed (policy) settings still apply. Auth, model selection, built-in tools, and permissions work normally." Sets `CLAUDE_CODE_SAFE_MODE=1`. Auto-memory only covered by "and more".
- `--strict-mcp-config`: "Only use MCP servers from `--mcp-config`, ignoring all other MCP configurations". Whether claude.ai connectors and plugin MCP servers are covered is **not stated**.
- `--tools ""` disables built-in tools only. `--disable-slash-commands` disables skills. `--json-schema`, `--output-format json`, `--no-session-persistence`, `--permission-prompts none`, `--max-budget-usd` (print-only) exist. `--bare` needs an API key (unusable under the subscription-only rule). There is **no** timeout flag and no `--max-turns` in help.
- Praxis's current adapter passes none of the isolation flags, runs with the repo as cwd, uses `acceptEdits` and a 41-pattern `--allowedTools` list, kills only the direct child on timeout (no process group), and never parses stdout (`ProviderCLIWorker` returns `provider-process:completed`). Child env is allowlisted (drops `CLAUDE_CODE_*`, `ANTHROPIC_*`) but keeps `HOME`, `CLAUDE_CONFIG_DIR`, `CODEX_HOME`, `SSH_AUTH_SOCK`.
- Codex: `--sandbox read-only` restricts writes only; `--add-dir` is writable-only; no documented read confinement; `--ignore-user-config`, `--ignore-rules`, `--ephemeral`, `--output-schema`, `-o`, `--disable <feature>` exist; whether MCP/hooks defined in config are suppressed is not stated. No test freezes Codex flags.
- Codex reviewer's live probe of the T1 shape returned valid structured output (`{"ok":true}`) in ~99 s. Reported by that reviewer; not a Praxis-recorded probe.

**Installed surface (read-only)**: `praxis version` = commit `f539b74` (embedded goals package 0.1.4); **installed active package `praxis.package.goals` = 0.1.3** (from `installed_packages`); installed `--operation` help omits `establish`, `continue`, `evaluate`, `complete`, `succeed`, `decide`; `praxis goals` / `praxis goal` are unknown; the `goals`/`design` invocation exists in source but is not in the manifest or aliases; static `cli_help.go` has no goals entries (help is generated from the installed contract). Codex independently confirmed 0.1.3 vs 0.1.4.

**Bootstrap state (from the Codex read-only `inspect`, corroborated by code)**: stored order of all 31 elements equals establish-file order; `ImportSourceDigest` = sha256 of the establish file = `1881e31e…`; v2 proposal is persisted in authoritative state; no v2 review or authority request exists.

---

## 2. Mapping v2 → v3 (Requirement 2)

| v2 candidate | v3 disposition | v3 id | Why |
|---|---|---|---|
| 1 requirement-identity-and-binding | **rescoped** | `unit:hi-requirement-identity-and-binding` | + duplicate-text rule, baseline-derived review denominator (as a reusable function), proposal structural integrity (cycles/duplicate pairs), worker-context relation-kind fidelity |
| 2 ontology-and-surface-contract | **rescoped** (decision half split out) | `unit:hi-ontology-and-surface-contract` | Now *records* ratified decisions and the surface contract after Gate A; no longer decides authority semantics |
| 3 operational-identity-allocation | **preserved** | same | Settled; ADR-100 single-use held |
| 4 advisory-evidence-staging | **preserved** (+ staging dir outside any git work tree) | same | Settled |
| 5 advisory-result-intake-and-recovery | **preserved** (+ content-addressed result bytes, provenance record used by reviewer provenance) | same | Settled |
| 6 review-principal-provenance | **rescoped** | `unit:hi-review-principal-provenance` | Permanent reviewer/review-evidence integrity; enforcement policy after Gate B |
| 7 surface-skew-conformance | **preserved** (+ installed-vs-embedded distinction) | same | Settled split |
| 8 provider-advisory-transport | **rescoped** | `unit:hi-provider-advisory-transport` | Exact isolation profile, own launcher, probe, kill semantics |
| 9 provider-routing-policy | **preserved**; now hard for dogfood | same | Closure fix |
| 10 source-bound-goal-establishment | **preserved**; gated by Gate A | same | Establishment ceremony undecided |
| 11 governed-planning-orchestration | **preserved** | same | Settled |
| 12 model-assisted-interpretation | **preserved**; gated by Gate A; hard→consumer on 10 | same | Confirmation ceremony is Gate A |
| 13 requirement-evolution… | **rescoped** | same | Contract-changing successor authority via Gate A; hard on 10 |
| 14 evolution-execution-boundary | **rescoped** | same | Gate C; default = live turn finishes, never rewritten |
| 15 human-lifecycle-flow | **preserved** | same | Split from status survived |
| 16 status-and-control-projection | **preserved** (pause acts at admission boundary only) | same | Interrupting a live turn is Gate C |
| 17 installed-human-surface-packaging | **preserved** | same | Split survived |
| 18 dogfood-acceptance | **preserved**; + hard→routing | same | D7 closure |
| — | **new** | `unit:hi-governance-decision-dossier` | Evidence + recommendations for all three gates |
| — | **new** | `unit:hi-authority-gate-mechanism` | Makes a human stop un-self-clearable |
| — | **new** | `gate:hi-authority-ceremony` (Gate A) | Human authority boundary |
| — | **new** | `gate:hi-review-independence` (Gate B) | Human authority boundary |
| — | **new** | `gate:hi-supersession-and-active-turn` (Gate C) | Human authority boundary |

Merged: none. Removed: none.

---

## 3. Human-authority gates (Requirement 10)

### 3.1 What is delegated vs what is owner authority

| Kind of question | Who decides | Examples in this plan |
|---|---|---|
| Architectural/implementation choice with no new authority or governance semantics | Execution (accepted units) | Data layout of manifests, launcher structure, allocation counter format, projection shapes, default deadlines, fixture design |
| Semantics already fixed by an accepted ADR/SPEC or by a Goal constraint | Nobody: implement it | Predecessor generation immutable; successor ledger empty; no silent completion reattribution (ADR-099, SPEC-027, C5); a live turn is not silently rewritten/destroyed (C6); single-use invocation ids (ADR-100); model output is never human authority (C3) |
| Change to who may authorize what, what a settlement means, or what independence requires | **Owner, via a durable decision** | Gates A, B, C |

### 3.2 Boundary shape

```
dossier unit (agent): evidence + options + consequences + recommendation, committed, digest D
        ↓
gate candidate (controller-owned, never launches a provider)
        creates one idempotent owner-only AuthorityRequest bound to (goal, generation, baseline digest, gate id, D)
        ↓  HUMAN_AUTHORITY_REQUIRED
owner runs interactive `authority decide` (OS-user + typed confirmation)
        ↓
durable decision (approve | reject); approve → controller records gate completion citing the decision digest
        ↓
hard-dependent units become ready; controller injects the approved decision digest + dossier path into their worker context
```
A `reject` leaves the gate incomplete and dependents blocked; it does not silently select an alternative. Revision is a new dossier version via the normal lifecycle, not an agent decision. The plan **does not pre-decide** any gate; the recommendations below are the dossier's *starting hypotheses*, stated so the owner can judge the plan, and are non-binding.

### 3.3 Gate A: `gate:hi-authority-ceremony` — questions it governs

- **A1 Establishment.** Does establishing/importing a Goal generation need owner confirmation, or stay a non-interactive evidence-admission boundary (SPEC-023)? What does "authoritative" mean given SPEC-014 inv. 2? Where is "authority is established" for SC3 ("inspect … before or as authority is established")?
  *Starting recommendation:* keep establishment non-interactive; define "authoritative" as "source of truth for the outcome contract, conferring no right to act"; execution authority stays exactly two owner acts (plan acceptance, settlement); require the canonical interpretation to be shown and its digest bound in the first `workplan.accept` request. No new authority kind.
- **A2 Typed confirmation for ordinary users.** May the ceremony change form (plain-language phrase, no hand-copied digest) while keeping interactive + OS-user + digest binding? *Recommendation:* the deliberate-typed, OS-user, interactive properties stay; only what the human must copy changes.
- **A3 Evolution confirmation matrix.** Which of {entailed, non-material clarification, material change, inconsistent, invalidating, needs-successor} proceed on the human's supplied text alone vs require explicit owner confirmation of Praxis's impact analysis. *Recommendation:* first two proceed with recorded provenance; the other four require owner confirmation.
- **A4 Inferred-material-element confirmation** (interpretation): confirmed through the same ceremony as A2.
- **A5 Direct changed-contract successor operation** vs only settle→`succeed` (which today copies the contract). SC6 needs evolution while executing.

Gates: hard prerequisites of `ontology`, `source-bound-establishment`, `model-assisted-interpretation`, `requirement-evolution` (and therefore `human-lifecycle-flow`).

### 3.4 Gate B: `gate:hi-review-independence`

- **B1** May the owner be both reviewer and decider for a model-authored plan? Does `review_all` require a non-owner reviewer? (Owner-default reviewer exists today; nothing forbids reviewer = decider.)
- **B2** Minimum independence for a *model* reviewer: distinct provider/model lineage from the proposer, or is a distinct session/generation of the same lineage enough? What happens when only one advisory provider is supported (today: Claude only)?
- **B3** What provenance a reviewer must present (human: resolvable `AuthorityGeneration`; model: advisory-invocation provenance record).
- **B4** May an owner acceptance override a `REVISION_REQUIRED`/`INSUFFICIENT_EVIDENCE` review?
  *Starting recommendation:* reviewer ≠ decider unless an explicit human review is recorded as such; model reviewer must differ in provider/model lineage from the proposer, or be human; if none is available, report an environment/capability condition (never an authority request); no override.

Gate B is a hard prerequisite of `review-principal-provenance` (enforcement policy), hence of `planning-orchestration`.

### 3.5 Gate C: `gate:hi-supersession-and-active-turn`

- **C1 Unsettled-generation disposition.** Settlement requires every unit complete, so a partially executed generation can neither settle nor `succeed`. What is the disposition when a materially changed successor is admitted: a new ADR-099 status (e.g. supersession preserving predecessor completions as evidence), abandonment, or refusal?
- **C2 Active-turn policy on materially invalidating change.** *Starting recommendation (already implied by C6):* default is that the live turn finishes and lands in its own generation's ledger; the successor becomes governing only after quiescence (no live admission, no unreleased lost turn on the predecessor). Whether a change may *authorize interruption* of a live turn, by whom, and how (the existing `supervise cancel|suspend` has no owner check).
- **C3** What happens to in-flight work an invalidating change makes inapplicable: preserved, quarantined, or marked applicability-invalid (never destroyed).

Gate C is a hard prerequisite of `evolution-execution-boundary`.

### 3.6 Questions deliberately **not** gated (and why)

- **Successor completion basis.** ADR-099 + SPEC-027 already fix it: successor units complete only by their own completion act; predecessor completion is applicability/reuse evidence. v3 does **not** amend ADR-099 to admit a "re-verified" basis; applicability evidence shortens planning only. Any future amendment is out of scope and would need its own gate. (v2's uncertainty U-5 is resolved by not adopting the amendment.)
- **What counts as a material inferred element.** Derivable from C3/C4: any element Praxis adds that creates a requirement, constraint, success criterion, non-goal or risk acceptance is material; wording normalization is not.
- **Provider routing.** Ordered preference over probed-supported providers with an explicit-flag override; a human decision only when a configured attribute of the candidate providers differs materially. No routing policy is invented beyond the Story.

### 3.7 `unit:hi-authority-gate-mechanism` — bounded scope

Reuses `AuthorityRequest`/`AuthorityDecision`, `authority pending|decide` (ADR-097), `Ledger.RecordCompletion`, the `authority.required` surface and the existing owner-only non-delegable kind precedent (`installation.repair.*`, `SaveAuthorityDecision`). No new store, no new principal type, no delegable kind (ADR-074's closed delegable set is untouched).
- Recognize gate candidates deterministically (id namespace `gate:` + gate-definition record digest carried in `source_ref`/`source_digest`, resolving to the dossier digest).
- Never launch a provider for a gate candidate; create/lookup the request idempotently; exclude a pending gate from selection while other work is ready (SPEC-027 sibling rule); emit `authority.required` only when nothing else is runnable.
- New owner-only, non-delegable kind (e.g. `goal.gate.decide`) bound to goal/generation/baseline digest/gate id/dossier digest; surfaced by `authority pending` with the question in plain language.
- Refuse worker trailers for gate ids; complete a gate only from an approved decision bound to the exact dossier digest.
- **Close the non-interactive path for this kind**: `goals-lifecycle --operation=decide` must reject `goal.gate.decide` decisions (and record the finding that it also accepts `workplan.accept`).
- Inject the approved decision digest and dossier path into `WorkerUnitContext` of hard-dependent units (workers otherwise receive no unit text).
- **Qualification (negative-first):** a worker/planner/reviewer cannot decide; a hand-authored decision document is refused; a wrong-digest or stale-dossier decision is refused; a rejected gate keeps dependents blocked; a pending gate never blocks a ready sibling; restart/replay creates exactly one request; historical `workplan.accept` behaviour is byte-identical.
- *Considered and rejected:* staging the plan into two Goal generations via settle→`succeed`→re-plan. It would need every unit complete to settle, fragments SC coverage across generations and doubles the human ceremony. It remains the fallback if the owner declines the mechanism (Uncertainty U-1).

---

## 4. Candidate set (23) and scheduling semantics (Requirement 4)

### 4.1 Semantics, stated honestly

| Concept | Meaning in v3 (matches the code) |
|---|---|
| **Hard dependency** | The only gate. Dependent is blocked until the prerequisite is durably complete. Used only where the prerequisite must be complete for the dependent to be truthfully implemented **or qualified**. |
| **Readiness** | Incomplete ∧ all hard prerequisites complete. |
| **Priority** | *Scheduling preference among ready units only.* v3 tiers: 1 = get human decisions requested early (a pending gate never blocks siblings, so human latency overlaps with independent work); 2 = units with no gate dependency (fill that latency); 3 = gated foundations; 4–7 = composition, surface, packaging, final qualification. **No monotonicity constraint along edges**; a dependent may carry a lower number than its prerequisite and simply stays blocked. |
| **Sequence** | Deterministic tie-breaking only. Kept globally unique so `ErrAmbiguousWorkChoice` cannot occur (the code does not require it). It is also a linear extension of all 71 edges, which minimizes the number of consumer/interaction neighbours a worker sees as "already done or not your concern" while incomplete. It is **not** an architectural claim. |
| **Consumer / interaction / advisory** | Non-blocking by code. They record contracts and sharing for reviewers and humans; they cannot force order. |
| **Concurrency** | Not modelled. Goal-drive is single-track under an exclusive scope lease and nothing marks a unit in progress; v3 makes no parallelism claim and creates no ties to gain any. The graph's truthful ordering requirement is its **31 hard edges (longest hard chain: 7 nodes)**; nine units are hard-ready at the start (`dossier, mechanism, identity, op-identity, staging, intake, skew-conformance, routing, status`). |

Mechanical checks performed on the edge lists below: acyclic; no duplicate pair (so `Digest()` is deterministic); no order violation between sequence and any of the 71 edges; every unit is inside unit 23's transitive hard closure; two hard edges found transitively redundant and omitted (`evolution→GateA` via `evolution→establishment`; `planning→identity` via `planning→review-principal`). The plan document must be re-verified by the B-4 scripts.

### 4.2 Table

| pri | seq | id | short | bound elements |
|---|---|---|---|---|
| 1 | 1 | `unit:hi-governance-decision-dossier` | DOS | SC3 SC4 SC6 C1 C3 C5 C6 N1 N3 |
| 1 | 2 | `unit:hi-authority-gate-mechanism` | MECH | SC4 SC5 C1 C7 N1 N4 |
| 1 | 3 | `gate:hi-authority-ceremony` | GA | SC3 SC4 SC6 C1 C3 |
| 1 | 4 | `gate:hi-review-independence` | GB | SC5 C1 C3 N1 |
| 1 | 5 | `gate:hi-supersession-and-active-turn` | GC | SC6 SC7 C1 C5 C6 |
| 2 | 6 | `unit:hi-requirement-identity-and-binding` | ID | SC5 SC7 SC8 C1 C2 C3 C7 |
| 2 | 7 | `unit:hi-operational-identity-allocation` | OPI | SC5 C1 C7 C8 C10 |
| 2 | 8 | `unit:hi-advisory-evidence-staging` | STG | SC5 C1 C2 C7 |
| 2 | 9 | `unit:hi-advisory-result-intake-and-recovery` | INT | SC5 SC8 C1 C2 C3 C7 C10 |
| 2 | 10 | `unit:hi-surface-skew-conformance` | SKW | SC10 SC11 C8 C9 N2 |
| 2 | 11 | `unit:hi-provider-advisory-transport` | TRN | SC5 C1 C3 C7 C8 N1 |
| 2 | 12 | `unit:hi-provider-routing-policy` | RTE | SC4 SC5 C7 C8 A4 |
| 3 | 13 | `unit:hi-review-principal-provenance` | RPP | SC5 C1 C2 C3 N1 |
| 3 | 14 | `unit:hi-ontology-and-surface-contract` | ONT | N3 N4 N5 A1 A2 A3 C1 C7 C8 |
| 3 | 15 | `unit:hi-source-bound-goal-establishment` | EST | SC1 SC2 SC3 SC5 C1 C2 C7 A2 |
| 4 | 16 | `unit:hi-governed-planning-orchestration` | PLN | SC5 SC8 SC10 C1 C3 C7 C10 N4 |
| 4 | 17 | `unit:hi-model-assisted-interpretation-and-ambiguity` | INTERP | SC1 SC2 SC3 SC4 C2 C3 C4 |
| 5 | 18 | `unit:hi-requirement-evolution-classification-and-successor` | EVO | SC6 SC7 C1 C2 C5 C7 |
| 5 | 19 | `unit:hi-evolution-execution-boundary` | BND | SC6 SC7 C1 C5 C6 |
| 5 | 20 | `unit:hi-human-lifecycle-flow` | FLOW | SC4 SC5 SC8 C10 N4 |
| 5 | 21 | `unit:hi-status-and-control-projection` | STAT | SC9 SC10 C8 N2 |
| 6 | 22 | `unit:hi-installed-human-surface-packaging` | PKG | SC11 C8 C9 N2 N5 |
| 7 | 23 | `unit:hi-dogfood-acceptance` | DOG | SC1–SC12, C1–C10 |

Labels are the stored-order labels of v2 (`SC<n>` = `success_criteria/<n>` …). Union of bindings still covers all 31 elements (`A1 A2 A3` in ONT/EST; `A4` in RTE; `N1` in MECH/GB/RPP/TRN; `N2` in SKW/STAT/PKG; `N3` in DOS/ONT; `N4` in MECH/ONT/PLN/FLOW; `N5` in ONT/PKG). All bindings must be materialized **by script from the manifest** (B-4), never typed.

### 4.3 Candidate responsibilities, reuse, qualification

Every unit is advisory-only until accepted. "Reuse" names existing machinery; no parallel governance is created.

**1 DOS `governance-decision-dossier`.** *Responsibility:* for Gates A/B/C author three committed dossiers (primary evidence with file:line/ADR quotes, options, consequences per option, starting recommendation, explicit "not decided" marker), plus the SPEC-014 inv. 2 reconciliation options. Decides nothing. *Reuse:* SPEC-006/014/023/027/031/054, ADR-060/064/068/069/096/097/098/099/100. *Qualification:* each dossier cites primary evidence for every claim; every gate question in 3.3–3.5 appears; dossier digests recorded; a doc check that no dossier contains a decision marker.

**2 MECH `authority-gate-mechanism`.** Section 3.7.

**3–5 Gates A/B/C.** Controller-owned; completion only from an approved owner decision bound to the dossier digest (3.2). No provider is ever launched for them.

**6 ID `requirement-identity-and-binding`.** *Responsibility:* preserve v2 identity (`<kind>:sha256:<hash of exact element text>` / `goal:<id>/<ver>#<kind>/<stored-index>` / `sha256:<text hash>`); resolver mapping a `RequirementRef` to one exact element of the exact baseline and rejecting unresolved, mismatched (`id`/`source_ref`/`source_digest` disagree) and ambiguous refs; proposal-time verification in `BuildWorkPlanProposal`; `BaselineRequirementSet(baseline)` as the **only** source of a review denominator; completion coverage by element identity with legacy positional refs still accepted; structural integrity at propose/accept (reject hard-edge cycles and duplicate relationship pairs); `WorkerUnitContext` lists only hard prerequisites as prerequisites and labels other kinds. Duplicate-text rule in Section 9. No change to any existing baseline digest. *Reuse:* `canonical.go`, `baseline.go`, `work_selection.go`, `work_plan.go`, `completion.go`, `goalstore.Repository.Load`. *Qualification:* table tests; reordered-list fixture keeps ids stable; historical baselines replay with unchanged digests; invented, mismatched, unresolved refs fail closed; proposal omitting a real element is flagged against the baseline denominator; a cycle and a two-kind duplicate pair are rejected; **replay of the accepted v3 refs** (B-7).

**7 OPI `operational-identity-allocation`** (unchanged from v2). Praxis-minted single-use id inside the `Admit` CAS as `(goal, generation, attempt counter)`; `invocation.allocated` before launch; each retry mints a new attempt; goal+generation-scoped identity and activity-stream keys; explicit `--invocation-id`/`--provider` unchanged. *Qualification:* cross-process race never mints the same id; restart in each non-terminal state yields the specified disposition and never reuses an id; identical human inputs in two attempts yield distinct ids; cross-generation collision impossible.

**8 STG `advisory-evidence-staging`.** Controller-staged read-only evidence bundle **outside the checkout and outside any git work tree** (Praxis-owned 0700 directory), sha256 manifest of every item, `ResolveAuthorizedPath` confinement, cleanup; truthful predicates from v2 Q4 (HEAD equal, `git status --porcelain --untracked-files=all` empty, `ConsequenceFingerprint` equal, refs excluding `refs/remotes/*` and `FETCH_HEAD`, `--no-optional-locks`, never `GitRepository.Snapshot`). Consumes an explicit invocation id; no dependency on automatic allocation.

**9 INT `advisory-result-intake-and-recovery`.** Advisory turn admitted through `Admit` with its own scope key and no `TurnRecord`; result-source interface (bytes + exit metadata); dedicated capture buffer (not the redacted transcript), size-limited, truncation-rejecting, exactly-one-JSON-object (reuse `CommandWorker` discipline); schema validation; **result bytes stored content-addressed by a controller-computed digest**; durable provenance record binding invocation, turn, provider, CLI version, planner generation, evidence-manifest digest, result digest; typed environment failure classes that cannot become an `AuthorityRequest`; lost-lease recovery via ADR-100. `TrustUntrusted` (ADR-040).

**10 SKW `surface-skew-conformance`.** Harness + existing-operation fixes: help/manifest for `establish`, `continue`, `evaluate`, `complete`, `succeed`, `decide`; correct option scoping; installed-vs-source detection **that compares the installed package generation (`installed_packages`, currently 0.1.3) against the binary-embedded package (0.1.4) and against source**, reporting them as three separate facts; the `goals`/`design` invocation registered or removed truthfully; static help entries; stale-doc sweep. *Qualification:* through the real activation path; a stale installed manifest against a newer binary is reported; every dispatched operation is documented and every advertised command runs; explicit-flag behaviour unchanged.

**11 TRN `provider-advisory-transport`.** Section 5. *Reuse:* `execution_contract.go` (`WorkerCapability`, launch-flag test pattern, `CapabilityError` pattern), `command_worker.go` (env allowlist, `limitedBuffer`), `providers.go` catalog. *Qualification:* Section 5.6.

**12 RTE `provider-routing-policy`.** Deterministic ordered-preference policy over catalog availability and per-CLI-version probe support; chosen provider and policy recorded; explicit `--provider` always wins; a human question only on a configured material difference; a provider without a passing probe is never selectable for advisory work. Same catalog → same choice.

**13 RPP `review-principal-provenance`** (permanent mechanism; Section 8). Hard after Gate B and identity.

**14 ONT `ontology-and-surface-contract`.** After Gate A: record the ratified Q3 table (Goal, generation, requirement, source document, WorkPlan, work unit; Story/Feature/Task as UI words only), the SPEC-014 wording as decided, the three-surface map (human/operator/forensic) with an identifier-visibility matrix, and the human vocabulary, as ADR/SPEC edits. **Creates no authority kind or store and decides nothing not in the approved Gate A record.** *Qualification:* doc consistency against SPEC-014/023/054, ADR-060/064/099; a conformance test that human-surface renderings contain no digest/generation/id unless asked.

**15 EST `source-bound-goal-establishment`** (Gate A applied). Model-free deterministic path from a human Markdown file (documented profile) to an established generation: exact source bytes persisted (new secure-blob namespace); source digest bound into the **canonical digest of new generations** through an `ArtifactRef` role (historical digests unchanged); derived generation number; inspectable "what Praxis understood" with per-element provenance (supplied / normalized); duplicate merge as `normalized` (Section 9); replay-safe; differing content under the same identity routes to evolution, and a semantically equal reformatting is detected as equal. Implements exactly the establishment ceremony Gate A approved.

**16 PLN `governed-planning-orchestration`.** At `planning_required`: staged evidence → admission → transport → intake → identity-verified proposal via the existing `propose` path → an independent reviewer via unit 13's controller-computed review → existing `continue`/`request`/`decide`/`accept`/`attach`. Invalid/uncovered output → `planning_revision_required`. The planner never supplies proposal id, generation or provenance. Fixture-driven end to end with no hand-authored JSON; self-, same-generation- and forged-provenance reviews fail closed; a planner claim of authority is inert.

**17 INTERP `model-assisted-interpretation-and-ambiguity`** (Gate A applied). Model proposes candidate interpretation through the advisory transport; stored non-authoritative; every element tagged supplied/normalized/inferred; inferred material elements need the Gate-A confirmation ceremony; ambiguity asked as a consequence-stated question; source treated as data. Interpretation provenance binds to the source digest (C2, SC2). Optional and transport-agnostic; a model outage leaves the deterministic path intact. *Qualification:* adversarial corpus cannot create a requirement, constraint, risk acceptance or authority.

**18 EVO `requirement-evolution-classification-and-successor`** (Gate A applied). Deterministic, model-free classification by element identity into {entailed, non-material clarification, material change, inconsistent, invalidating, needs-successor}; impact analysis wiring `invalidation.go`/`applicability.go`; a changed-contract successor transition with **atomic** blob+succession; applicability/reuse evidence keyed by element identity citing predecessor completion digests and consequence fingerprints, **never** completion transfer; successor source binding through unit 15's primitive. Which classes need owner confirmation is exactly the Gate A matrix. *Qualification:* one fixture per class; predecessor bytes/digests unchanged; successor ledger starts empty; reordered-but-equal list → entailed; crash between blob save and succession record repairs deterministically; works with no model; a class requiring confirmation cannot proceed without the owner decision.

**19 BND `evolution-execution-boundary`** (Gate C applied). Outgoing-generation admission fence; live turn finishes untouched (result lands in its own generation's ledger); lost turns reconcile via ADR-100; successor governing only after quiescence, quiescence including **no unreleased lost admission on the predecessor** (the per-generation admission view does not see it and the scope lease expires); drive-time authority re-check; supersession disposition and interruption authority exactly as Gate C approved. *Qualification:* in-flight turn neither rewritten nor destroyed; crash on either side of the boundary replays deterministically; older drivable generation refused after supersession; scope lease still serialises the checkout; no interruption path exists that Gate C did not authorize.

**20 FLOW `human-lifecycle-flow`.** One human flow composing establish, plan, review, decide, attach, drive, evolve with generated identities; decisions surface only where the Gate A/ontology record says human authority is required, in plain language, through the interactive `authority decide` (OS-user and typed confirmation preserved) with the exact command shown; no digest carried by hand. *Qualification:* transcript from Markdown to `drivable` contains no digest, generation number, invocation id or proposal id; each prompt links to operator evidence; restart mid-flow resumes without duplicating a mutation.

**21 STAT `status-and-control-projection`.** Evidence-only status (Section 10). Goal-level pause/resume record visible across invocations that takes effect **at the next admission boundary only**; interrupting a live turn stays the existing operator `supervise` surface until Gate C says otherwise.

**22 PKG `installed-human-surface-packaging`.** Register the new human entry points in a signed package successor (names illustrative, N5), declare every option, update help, plan version cadence (one successor per contract change, planned once), install, and prove installed == source through unit 10's harness. Forbids any unit from using an option not declared in the installed manifest.

**23 DOG `dogfood-acceptance`.** Section 13.

---

## 5. Provider isolation profile (Requirement 11)

**Scope.** Claude Code is the **first and only** supported advisory transport. Codex is **not** claimed.

### 5.1 Exact Claude profile ("T1-isolated")

Working directory: a controller-created, empty, 0700 directory under a Praxis-owned location, **outside the authoritative checkout and outside any git work tree**, deleted after the turn. Prompt (evidence bundle + instructions) on stdin. One JSON object on stdout.

```
claude --print --output-format json --json-schema <inline schema>
       --no-session-persistence --permission-prompts none
       --restricted --safe-mode --strict-mcp-config
       --tools "" --disable-slash-commands
       --max-budget-usd <policy bound> [--model <policy-selected>]
```
- **Never** passed: `--permission-mode acceptEdits`, `--allowedTools`, `--add-dir`, `--bare`, `--dangerously-skip-permissions`/`bypassPermissions`, `--mcp-config`, `--plugin-dir`/`--plugin-url`, `--agents`, `--settings`.
- Child environment: Praxis's existing allowlist (drops `CLAUDE_CODE_*`, `ANTHROPIC_*`). `HOME`, `CLAUDE_CONFIG_DIR` remain visible (needed for subscription auth), so **isolation rests on the flags plus a probe, not on the environment**.
- Why each flag: `--restricted` (ignores user/project/local settings ⇒ their hooks, removes code-running tools/WebFetch); `--safe-mode` (CLAUDE.md, skills, plugins, hooks, MCP servers, agents, commands disabled); `--strict-mcp-config` with no `--mcp-config` (zero MCP); `--tools ""` (no built-in tools); `--disable-slash-commands` (skills); `--permission-prompts none` (nothing can ask); `--no-session-persistence` (nothing saved). Redundancy is deliberate.
- **Documented gaps the probe must close** (help does not state them): auto-memory; claude.ai connectors and plugin-provided MCP servers under `--strict-mcp-config`; managed/policy settings and `--settings`, which still apply; whether `--restricted` and `--safe-mode` are mutually compatible; whether `--tools ""` composes with `--json-schema`; where the structured result appears in `--output-format json`. These are **probe questions, not claims**.

### 5.2 Verification, not assumption
1. **Per (CLI version, flag-set digest) gating probe**, recorded as durable evidence: non-mutating sessions against a scratch fixture with **canaries** in each ambient source (project `.claude/settings.json` hook, `CLAUDE.md` instruction, `.mcp.json` server, project skill, memory file; and user-level equivalents via a copied config dir where feasible). Pass = no canary fired, valid structured output, exit within deadline, no file created outside the scratch cwd, checkout predicates unchanged.
2. **Runtime self-check** where the probe shows the CLI exposes it: if a streaming/init event lists tools, MCP servers, plugins, skills, hooks or agents, launch verifies the list is empty and kills the process otherwise. If the probe shows no such surface, the runtime check is dropped and support rests on the probe alone, and that limitation is recorded.
3. **Fail closed** (typed environment/capability evidence modelled on `CapabilityError`/`capability.unsatisfiable`; never an `AuthorityRequest`; never a human question): `binary_missing`, `flag_unsupported`, `no_passing_probe` (including CLI version drift), `isolation_unsatisfied`, `cwd_inside_worktree`, `timeout`, `output_oversize`, `result_invalid`, `nonzero_exit`.

### 5.3 Bounded exit
No timeout flag exists, so bounding is process-level in a **new advisory launcher** (not `ProviderCLIWorker`): hard wall-clock deadline from the probe's observed distribution with margin (the Codex reviewer's trivial-output probe took ~99 s, so the default must not be seconds), `Setpgid` + process-group kill on deadline/cancel (the current adapter kills only the direct child), `WaitDelay`, `--max-budget-usd`, stdout cap with truncation rejected, exactly-one-JSON-object, nonzero exit → typed failure.

### 5.4 Codex classification
`unsupported for advisory transport`. Reasons from evidence: `--sandbox read-only` limits writes only, `--add-dir` is writable-only (the v1 two-path design stays refuted), no documented read confinement (the whole readable filesystem including credentials is reachable), and whether config-defined MCP/hooks are suppressed is undocumented. It becomes eligible only after (a) a recorded passing probe for that exact CLI version and flag set (`--ignore-user-config --ignore-rules --ephemeral --sandbox read-only --output-schema/-o`, `--disable` of `hooks`/`plugins`/`apps` as the probe shows effective) **and** (b) an owner-approved weaker-read-confinement policy (a future Gate D, out of scope). Until then Codex serves as the live fail-closed demonstration (D6). No Codex support is asserted anywhere in v3.

### 5.5 Reuse
`execution_contract.go` capability pattern, `command_worker.go` env allowlist and `limitedBuffer`, `providers.go` catalog, `redactProcessOutput`, `ActivityLog`, `SecureBlob`.

### 5.6 Qualification (unit 11)
Freeze-flag tests for **both** providers (Codex currently has none); helper-process fake CLIs assert exact argv, env and cwd, and that no write/bypass/`acceptEdits`/`--add-dir` flag is passed; canary probe on the real CLI recorded; unsupported/stale-probe/isolation-failed cases each yield a typed environment failure with no authority request; deadline and process-group kill tested with a fake that forks a grandchild; result-oversize/trailing-JSON/truncation rejected; a real T1 run recorded (it reappears in the dogfood).

---

## 6. Relationships (71) and rationale for every hard dependency (Requirements 6 and 7)

Notation `dependent → prerequisite`, using the short names of 4.2. Definition used for every edge: a **hard** edge exists only if the prerequisite must be complete before the dependent can truthfully be implemented **or qualified**; anything that can be built and qualified against fixtures or a shared contract is not hard.

### 6.1 Hard (31)

| # | edge | why it must be complete first |
|---|---|---|
| 1–6 | GA, GB, GC → DOS; GA, GB, GC → MECH | A gate cannot ask a truthful question without its dossier, and must never be selected before the mechanism exists: without it the controller would launch a provider and a worker could self-attest the gate |
| 7 | RPP → GB | Independence policy is owner authority; unit 13 enforces it |
| 8 | RPP → ID | Review coverage denominator comes from `BaselineRequirementSet`; "coverage cannot be tautological" is unqualifiable without it |
| 9 | ONT → GA | ONT records the ratified decisions; recording an undecided one is exactly the failure this revision exists to prevent |
| 10 | EST → GA | The establishment ceremony (and whether a pending state exists) changes EST's contract |
| 11 | INTERP → GA | Confirming inferred material elements is the Gate A ceremony |
| 12 | BND → GC | Supersession disposition and interruption authority are Gate C |
| 13 | TRN → INT | TRN implements INT's result-source interface and typed failure classes, and its real-provider qualification records through INT's admission/provenance (upgraded from v2 consumer; Claude review) |
| 14 | PLN → RPP | An automated review is meaningless unless its principal and independence are durably bound; also implies PLN → ID (v2 edge omitted as redundant) |
| 15 | PLN → INT | No durable, typed path for a planner result without it |
| 16 | PLN → STG | The planner must receive exact evidence with no human transport (Story finding) |
| 17 | EVO → ID | Semantic diff and applicability need stable element identity |
| 18 | EVO → EST | Successor provenance (C2, SC2) uses the source-bound digest primitive EST owns; EVO cannot truthfully bind provenance without it (upgraded from v2 consumer; Claude review). Implies EVO → GA (v2-style edge omitted as redundant) |
| 19 | BND → EVO | The fence has no successor to hand over to without the successor transition |
| 20–23 | FLOW → PLN, EST, OPI, ONT | Flow's `planning_required` stage, entry step, generated identities (else SC5/C10 violated) and identifier-visibility/ceremony record |
| 24–26 | PKG → FLOW, STAT, SKW | Cannot advertise a command that does not exist; packaging is qualified with the SKW harness |
| 27 | DOG → PKG | The run is on the installed surface |
| 28 | DOG → BND | Requirement evolution during execution must be demonstrated |
| 29 | DOG → INTERP | SC4 and "model cannot manufacture authority" need an interpreter |
| 30 | DOG → TRN | A real supported advisory transport must be demonstrated |
| 31 | DOG → RTE | **Added.** D7 (v2 §9) requires "policy-chosen provider". Verified: RTE was the only unit outside DOG's hard closure and reached DOG only through the consumer edge plus priority order, which is a scheduling preference. The final qualification cannot be truthfully performed without a policy-chosen provider, so it is hard on its own merits |

Transitive closure (all units are inside DOG's closure): via PKG→FLOW→{PLN→{RPP→{GB,ID},INT,STG},EST→GA,OPI,ONT}, PKG→STAT, PKG→SKW, BND→{EVO→{ID,EST},GC}, INTERP→GA, TRN→INT, RTE, plus each gate's DOS/MECH.

### 6.2 Edges I changed relative to v2 (and reviewer recommendations not applied mechanically)

| edge | v2 | v3 | Trace |
|---|---|---|---|
| INTERP → EST | hard | **consumer** | Interpretation output (candidate structure + per-element tags) can be built and qualified without establishment; the confirmation ceremony is Gate A, not EST. Integration is exercised in FLOW and DOG. **Codex applied, on independent trace.** |
| DOG → RTE | consumer | **hard** | D7 (above). **Codex applied, on independent trace.** |
| TRN → INT | consumer | **hard** | Interface and failure classes are INT's product; **Claude applied, on independent trace.** |
| EVO → EST | consumer | **hard** | Source-binding primitive. **Claude applied.** |
| EST → ONT | interaction | **interaction, plus EST → GA hard** | **Claude's "hard, or split the decision" resolved by splitting:** the establishment-authority decision is no longer inside ONT but in Gate A; gating EST on the *decision* (not on the vocabulary document) is the truthful edge. |
| RPP → INT | interaction | **consumer** | RPP consumes INT's provenance record type but can be qualified with a fixture record; both are hard-before PLN. **Claude applied.** |
| RPP → ID, INTERP → GA, EST → GA, ONT → GA, EVO → EST, BND → GC, RPP → GB | — | **hard, new** | Denominator, ceremony and gate consequences above |
| ONT → SKW | v2 had SKW → ONT advisory | **advisory, reversed** | With ONT now after Gate A, SKW (priority 2) cannot follow it; ONT's conformance test can use SKW's harness instead |
| DOG → RPP | interaction | **removed** | Implied by the hard closure |
| EVO → GA, PLN → ID | — | **omitted as transitively implied** | Keeps the hard set minimal |

No hard edge was rejected as unjustified: the remaining v2 hard edges (PLN→RPP/INT/STG, EVO→ID, BND→EVO, FLOW→PLN/EST/OPI/ONT, PKG→FLOW/STAT/SKW, DOG→PKG/BND/INTERP/TRN) were re-derived above and stand (Codex found 17 of 18 substantively justified; the one it disputed is INTERP→EST, applied).

### 6.3 Consumer (24)
`EST→ID`; `TRN→STG`; `INTERP→STG`; `INTERP→INT`; `INTERP→EST`; `RPP→INT`; `PLN→TRN`; `EVO→INTERP`; `EVO→ONT`; `BND→PLN`; `FLOW→TRN`; `FLOW→INTERP`; `FLOW→EVO`; `FLOW→BND`; `STAT→PLN`; `STAT→BND`; `STAT→OPI`; `STAT→FLOW`; `STAT→ONT`; `PKG→EST`; `DOG→PLN`; `DOG→EVO`; `DOG→FLOW`; `DOG→STAT`.

### 6.4 Interaction (13)
`EST→ONT`; `STG→OPI`; `INT→OPI`; `INTERP→TRN`; `INTERP→RTE`; `INTERP→OPI`; `PLN→OPI`; `PLN→RTE`; `EVO→PLN`; `EVO→OPI`; `BND→OPI`; `FLOW→RTE`; `RTE→TRN`.

### 6.5 Advisory (3)
`ONT→SKW`; `PKG→RTE`; `PKG→EVO`.

**Counts: hard 31, consumer 24, interaction 13, advisory 3 = 71.** Relationship `source_ref` = the committed v3 plan path; `source_digest` = its sha256; candidates' `source_ref` = `…/workplan-v3-claude.md#candidate/<id>` with the same digest. `proposer_generation` = `claude-sonnet-5/advisory-plan-v3@sha256:<digest of the committed v3 plan>`.

---

## 7. Bootstrap-evidence strategy (Requirement 12)

### 7.1 Enforced vs independently verified (must not be conflated)

| Property | Status today |
|---|---|
| Goal digest = `afda0866…` | **Independently verified** (recomputed from the establish file by two reviewers) |
| Establish file sha256 = recorded `ImportSourceDigest` `1881e31e…` | **Verified**; enforced only as an idempotency/conflict check on re-establishment |
| Stored element order = establish-file order | **Verified** by read-only `inspect` (Codex); no Praxis invariant pins it (order is outside the canonical digest) |
| 31 element ids/text digests/indices in the proposal | **Verified** (31/31; 112 refs by Codex) |
| Proposal digest | **Verified** by recomputation with the Praxis contract code |
| `RequirementRef` resolves to a real element; review denominator from the baseline | **Not enforced** (`Validate` = three non-empty strings). Becomes enforced only when unit 6 lands |
| `ReviewDigest` corresponds to an external review document | **Not enforced**; caller-supplied on the full-document path; selector digest ignores findings/coverage/provenance |
| Reviewer independence | **Not authenticated**: string inequality |
| Interactivity of the accept decision | Enforced for `authority decide`; **bypassable** via non-interactive `goals-lifecycle --operation=decide` |

### 7.2 Required durable artifacts **before authority** (all tracked in git; none in `/tmp` or `.praxis/`)
Directory: `docs/research/dogfood/praxis-human-interface/bootstrap/` (v2's promised files did not exist; that was the finding).

| id | artifact | requirement |
|---|---|---|
| B-1 | `goal-establish-source.json` | byte-exact copy of the establish document; sha256 must equal `1881e31e…8b6b8f0e` |
| B-2 | `goal-baseline-elements.json` | 31 rows: kind, stored index, canonical index, verbatim text, text sha256, derived id; recomputed Goal digest, `ImportSourceDigest`, counts, duplicate-text check (zero duplicates), **and its own sha256** |
| B-3 | `goal-stored-order-attestation.txt` | exact bytes of read-only `goals-lifecycle --operation=inspect` output for `praxis-human-interface/1`, with capture metadata (command, binary commit, UTC time) and sha256; a comparison showing stored lists = B-2 stored order and `ImportSourceDigest` = B-1 sha |
| B-4 | `verify/` | committed scripts: Goal digest recomputation (no Praxis code), manifest emitter, **proposal materializer from B-2 + the candidate/edge tables** (a model never types an id, digest or index), proposal-digest verifier; the reviewer runs a **separately written** re-implementation and both outputs are stored |
| B-5 | `proposals/proposal-v3.json` (tracked copy) + `plans/workplan-v3-claude.md` | the materialized proposal and the planning evidence, each with sha256; the proposal's digest recomputed; STORY.md full sha256 frozen (v2 left it "computed at B0") |
| B-6 | `reviews/workplan-v2-codex-review.md`, `reviews/workplan-v2-claude-review.md`, then `reviews/workplan-v3-<lineage>-review.md` + `…-provenance.json` | exact review bytes and sha256; provenance = provider, CLI version, model, session identity, start/end UTC, prompt sha256, input artifact digests, transcript sha256 taken **after** session end |
| B-7 | replay check | after unit 6 exists: replay the resolver over the accepted v3 refs; a mismatch is a finding against v3, and v3's acceptance never depended on it |
| B-8 | state-check record | read-only inspect showing v3 persisted and no conflicting review/request (as Codex did for v2) |

### 7.3 What goes into the authority request
The persisted, digest-covered `Reason` of the `workplan.accept` request must quote: B-2 digest, B-3 digest, proposal-v3 digest, planning-evidence digest, review-bytes sha256 (computed by a script run by neither the proposer nor the reviewer), reviewer provenance digest, the bootstrap-rule attestation (Section 8), and the statement "31/31 baseline elements covered; 71 relationships; enforcement of these properties is not yet in Praxis". (`AuthorityDecision` has no reason field, so the request is the only durable place.)

### 7.4 Review submission rule
Use the **full-document** review path only (the selector path's coverage is tautological and its digest ignores findings). The caller-supplied `ReviewDigest` is set by a **third party** to sha256 of the exact review bytes in B-6 and is labelled convention-verified, not enforced. Nothing in v3 asserts Praxis validates it.

### 7.5 The decision itself
v3's own authority transition must be recorded through the **interactive** `praxis authority decide` (typed confirmation), and the operator records that the non-interactive `decide` document path was not used. Hardening that path is unit 2's job (3.7); it is not a precondition of v3 but is named so the owner knows the residual gap.

---

## 8. Reviewer independence (Requirements 13 and 14)

### 8.1 Bootstrap rule (until unit 13 exists)
An authority-bound WorkPlan review is admissible **only if** all hold, and the owner ratifies the rule by accepting v3:
1. **Separately evidenced generation:** a distinct session with recorded identity and transcript digest (B-6).
2. **No shared planner context:** non-overlapping session identity/time, and a prompt sha256 that contains no planner reasoning.
3. **Different provider/model lineage from the proposer.** v3's proposer is Claude Sonnet 5, so a qualifying review must come from a **non-Anthropic lineage** (e.g. Codex, as for v1 and v2). A same-lineage review (like the Claude v2 review) is **corroborating evidence only** and cannot satisfy the rule alone.
4. **Independent recomputation:** the reviewer recomputes the Goal digest, manifest, proposal digest and requirement bindings itself (B-4 second implementation), not from the planner's report.
5. **Third-party digesting** of the review bytes (7.4); reviewer ≠ decider except where the owner explicitly records a human review as such.
6. **Blocking rule:** any recorded review on the same proposal digest with `REVISION_REQUIRED`, `INSUFFICIENT_EVIDENCE` or `AUTHORITY_CONFLICT` and an unresolved finding blocks the request until a revision or an explicit owner-authored disposition exists.

This is a **bootstrap constraint**, not the product architecture: it exists so the missing enforcement cannot authorize itself.

### 8.2 Permanent mechanism (unit 13, after Gate B) — Requirement 6
- **Content-addressed review evidence:** review bytes stored in a `SecureBlob` namespace keyed by a **controller-computed** sha256.
- **Controller-computed `ReviewDigest`** (versioned, additive; historical digests keep verifying) covering findings, coverage, reviewer provenance and status. A caller-supplied external digest is at most an advisory `claimed_external_digest` recorded next to the controller's bytes and hash, never proof.
- **Provenance chain:** provider invocation → exact result bytes (unit 9) → durable review record. Non-model reviewers resolve through `AuthorityGeneration` (`ValidateAuthorityGeneration`/lineage).
- **Controller-derived covered requirements:** the denominator is `BaselineRequirementSet(baseline)` (unit 6); the reviewer supplies judgments per element; `Covered`/`Missing`/`Invented` are set operations computed by the controller. The proposal can never define its own denominator.
- **Independence over resolved provenance** (distinct principal, distinct generation digest, distinct invocation, plus the Gate B policy for lineage/owner-as-decider); `ReviewerProvider` populated and read; reviewer-versus-accepter comparison enforced per Gate B.
- The full-document review path is retained for legacy records but marked `unverified_provenance` and is insufficient for new authority requests once this lands.
- *Qualification:* self-review, same-generation, forged generation string, unresolvable generation, reviewer = proposer via a second provenance path, owner-as-both (per Gate B), caller-asserted digest, and a review whose bytes do not hash to its record each fail closed; historical review digests verify unchanged; v1/v2 reviews replay as fixtures.

---

## 9. Requirement identity and duplicate text (Requirement 5)

- Identity preserved unchanged: `id = <kind>:sha256:<hash of exact element text>`; `source_ref = goal:<goal>/<generation>#<kind>/<stored-index>`; `source_digest = sha256:<hash of exact element text>`.
- **Enforcement (unit 6):** all three dimensions must resolve to **one** exact element of the exact generation, and the stored-index element's text hash must equal the id hash and the `source_digest`. Mismatch, unresolved, or ambiguous ⇒ fail closed at propose, review, request and completion.
- **Denominator:** `BaselineRequirementSet(baseline)` = the distinct `(kind, text-hash)` set of the loaded, digest-verified generation. Reviews and completion coverage derive from it; nothing derives it from proposal claims.
- **Duplicates in future generations:** two elements of one kind with identical text collide on the id. Rule: (a) a **new** generation built by establishment/interpretation merges duplicates as a recorded `normalized` step (non-material, no ceremony, visible in the "what Praxis understood" rendering); (b) a canonically imported generation with duplicates is **rejected at the resolver** as ambiguous; (c) a **historical** generation with duplicates keeps loading and verifying with unchanged digests, but any reference to a duplicated text fails closed as `ambiguous_duplicate` rather than being counted twice, and coverage reports it explicitly. No occurrence-ordinal id is introduced (stored order is not digest-pinned, so ordinals would re-create the v1 defect).
- This Goal: 31 elements, all distinct.

---

## 10. Status and control (Requirement 8)

The v2 split flow ↔ status/control stands (no evidence disproves it; both reviews call it resolved). Status derives only from durable records: governing state, admissions and leases, turn records, pending authority (including gate requests), completion assessment, evaluation chain and settlement decisions, plus the Goal-level control record. Unknown stays representable (`unknown` where evidence is absent). "Complete" only from a settlement decision. **No** percentage, progress, frontier, Goal completion or success unless durable state establishes it; "known frontier" is only plan work-set state (`blocked_by`, next unit, incomplete units, pending gates). `InvocationSummary`/`SummarizeTurn` (dead code today) is wired in; every item links to exact operator evidence. Pause acts at the next admission boundary; live-turn interruption remains operator-only until Gate C. Golden projections must never show a value the records do not contain; a pause set by one invocation is honoured by the next.

---

## 11. Installed/source/help coherence and readiness preconditions (Requirement 9)

Split preserved: unit 10 (harness + existing-operation fixes; reports installed **package generation** 0.1.3, binary-embedded 0.1.4 and source separately) early and independent; unit 22 (register/declare/install/prove) late. v3 forbids any unit from using an option not declared in the installed manifest, and plans the package-version cadence once (each contract change forces a signed successor).

**Execution-readiness preconditions (operator/human actions outside the WorkPlan; needed before goal-drive executes v3, not before authority):**
- **R-1 Tracked validation contract.** Add a tracked executable `.praxis/validate` (via a `.gitignore` negation, since `.praxis` is ignored) so completion records `repository:declared-validation-passed` instead of `no-declared-validation`; it must accept the bound-contract-element/`integrated` arguments. Without it, no unit's qualification is enforced at completion. If the owner prefers a different path, that is a code change and not part of this plan.
- **R-2** Commit v2 review files (B-6) so historical evidence is not chat-only.

---

## 12. Successor and supersession semantics (Requirement 16)

Preserved from v2: predecessor generations immutable; predecessor completion is historical/applicability evidence; the successor ledger starts empty; applicability/reuse evidence is keyed by element identity and cites predecessor completion digests and consequence fingerprints; completion is never silently reattributed; a changed-contract successor is created atomically (blob + succession).

Where human authority is required (3.3–3.5): (i) which change classes need owner confirmation (A3); (ii) the direct changed-contract transition (A5); (iii) disposition of an unsettled predecessor (C1); (iv) whether/by whom a live turn may be interrupted (C2); (v) treatment of inapplicable in-flight work (C3). Not gated: successor completion basis (fixed by ADR-099/SPEC-027, not amended), and the default that a live turn finishes and lands in its own generation (C6). ADR-100 lease/recovery guarantees are preserved: no fence code may bypass `Admit`, lease heartbeat, or `ReconcileLostTurn`; a lost turn on the predecessor blocks the successor becoming governing.

---

## 13. Final dogfood acceptance path (Requirement 20)

Preconditions: units 1–22 complete and installed through the normal governed installation; the evaluator is independent of the planner and implementers; R-1 satisfied; **this plan does not choose the next requirement**.

| Step | What is done | Demonstrates |
|---|---|---|
| D1 | Verify installed package, binary and help against source with unit 10's harness (three separate facts) | SC11, C9 |
| D2 | A human writes the next real requirement as plain Markdown | SC1, SC12 |
| D3 | Run only the human interface: exact source stored, canonical interpretation shown with per-element provenance, only material questions asked | SC2, SC3, SC4, C2, C4 |
| D4 | Adversarial source containing instructions: no requirement, constraint or authority is manufactured | C3 |
| D5 | Goal established; `planning_required`; planner runs through **T1-isolated Claude**; independent review by a distinct provenance-bound principal per the Gate B policy; stops only at owner authority, in plain language | SC5, SC8, C1 |
| D6 | Codex (or any provider without a passing probe) requested for planning: typed environment/capability failure, **no** authority request, no human question | fail-closed transport |
| D7 | Execution begins with Praxis-minted identities and the **policy-chosen provider** (unit 12), with an explicit-flag override shown to still win | SC5, C10 |
| D8 | Mid-execution "this is now a requirement": classification, owner confirmation where Gate A requires it, successor, applicability evidence, deterministic boundary; the in-flight provider turn is not rewritten | SC6, SC7, C5, C6 |
| D9 | Predecessor generation and completions byte-identical afterwards; successor ledger starts empty; reuse appears only as applicability evidence and the successor's own completion acts | immutable history, no reattribution |
| D10 | After reconciliation, work is derived and scheduled with no human-authored decomposition | SC8 |
| D11 | Status at each phase derives from durable state, distinguishes complete from invocation-terminated, pause survives a new invocation, `unknown` where evidence is absent | SC9 |
| D12 | Operator retrieves every exact record (digests, lineage, provenance, recoveries, gate decisions) and explicit-flag commands still work | SC10, C8 |
| D13 | Human never authored GoalBaseline JSON, digests, generations, invocation ids, WorkPlans, proposal/review/request digests, acceptance refs, and never scheduled work | SC5, SC12 |
| D14 | **Use the resulting human interface for the NEXT real human-authored requirement** (not chosen here) | SC12 |

No mocks for persistence, cryptography or containment (PRAXIS2 rule). Failure to complete D2–D14 without the bootstrap ceremony means the Story is not complete.

---

## 14. Resolution of every Codex v2 finding (Requirement 17)

| Codex finding | Verdict | Where |
|---|---|---|
| V1-1 identity: partially resolved (manifest and frozen source absent; not enforced) | **Established.** Files absent (verified); contracts unenforced (verified) | 7.1–7.2, unit 6, 9 |
| V1-2 reviewer principal: partially resolved; B5 permits a caller-asserted `ReviewDigest` | **Established** (full-document path accepts any non-empty digest; verified) | 7.4, 8 |
| V1-3 ontology: resolved | Agreed; further hardened by moving the *decision* into Gate A | 3.3, unit 14 |
| V1-4 staging/intake/transport: partially resolved; T1 launch not isolated | **Established.** Adapter passes none of the isolation flags (verified) | 5 |
| V1-5 identity: resolved | Agreed, unchanged | unit 7 |
| V1-6 successor: partially resolved; supersession/cancellation policy undecided inside units | **Established** (ADR-099 leaves supersession undefined; nothing decides live-turn policy) | 3.5, 12 |
| V1-7 flow/status: resolved | Agreed; preserved | 10 |
| V1-8 installed surface: resolved; source 0.1.4 vs installed 0.1.3; `praxis goals` unavailable | **Verified independently** (`installed_packages`, help, manifest) | unit 10, 11 |
| V1-9 final qualification: routing missing from hard closure | **Established** by trace of D7 and the closure | 6.1 #31 |
| N1 transport not least-privilege isolated (`--restricted`, `--safe-mode`, `--strict-mcp-config`; bounded exit, ~99 s probe) | **Established.** Exact semantics read from installed help; gaps that help does not state become probe questions, not claims | 5 |
| N2 asserted bootstrap artifacts absent; B5 digest conventional | **Established** | 7 |
| N3 material governance decisions hidden in units 2, 13, 14 | **Established**, with two refinements: the successor-completion basis is *not* open (ADR-099/SPEC-027 already decide it), and the live-turn *default* is fixed by C6; the genuinely open parts are gated | 3 |
| N4 total ordering by unique priority/sequence | **Partly established.** True as behaviour (the selector orders ready units by `(priority, sequence)`, so serial execution is unavoidable and single-track). But the reduced hard count never implied parallelism, nothing in the code executes in parallel, and creating ties would make the selector fail closed. v3 does not manufacture ambiguity; it redefines priority/sequence honestly and states that concurrency is not modelled | 4.1 |
| Disputed edge INTERP→EST hard → consumer | **Accepted** on independent trace | 6.2 |
| Missing edge DOG→RTE | **Accepted** on independent trace (D7, closure) | 6.1 #31 |
| Bootstrap-integrity: bindings semantically correct despite unenforced contract; chain not yet durable | **Agreed** | 7.1 |
| Reviewer recommendation not accepted | None rejected outright. Codex's "units 2, 13, 14 must stop at a separately identifiable human decision" is implemented with a mechanism rather than an unsupported convention, because the code has no way to express it | 3.7 |

---

## 15. Resolution of every Claude v2 finding (Requirement 18)

| Claude finding | Verdict | Where |
|---|---|---|
| Matrix 1 (identity evidence-only) | Agreed; Section 7 separates verified from enforced | 7.1 |
| Matrix 2 (reviewer principal; bootstrap = convention) | Agreed; bootstrap rule made explicit | 8.1 |
| Matrix 3 (ontology; open decision without a gate) | **Established**; Gate A | 3.3 |
| Matrix 4 (T1 profile incomplete) | **Established** | 5 |
| Matrix 5–7, 9 (identity, successor, flow/status, final binding) | Agreed | 4, 6, 10, 13 |
| Matrix 8 (installed skew) | Agreed and extended | unit 10 |
| **N1** T1 not confined (ambient hooks/MCP/config) | **Established** | 5 |
| **N2** governance decisions embedded in units; evolution lacks authority binding | **Established** | 3 |
| **N3** no independence policy when only Claude is supported; same-model reviewer passes string checks | **Established**; Gate B plus the bootstrap cross-lineage rule (this same finding applied to the Claude v2 review itself) | 3.4, 8.1 |
| **N4** edge misclassifications | Applied where the trace confirms it (TRN→INT, EVO→EST, RPP→INT); the 10→2 fix is resolved by **splitting the decision out** rather than making ONT a hard prerequisite | 6.2 |
| **N5** bootstrap evidence not frozen; proposal JSON git-ignored; establish source in `/tmp`; B3 by model `Write`; specs conveyed by `source_ref` with no digest check | **Established** (each verified this session) | 7.2, 4.1 (spec-conveyance note) |
| **N6** binding gaps (12 lacks SC2/C2; C4/SC4 no deterministic owner) | Unit 17 now binds C2 and SC2. The deterministic-path owner for C4/SC4 is unit 15's rendering plus the rule that Praxis adds no element silently; material-only questioning remains unit 17/FLOW | 4.2 |
| **N7** installed is 0.1.3; `continue` not in installed manifest | **Verified** and folded into unit 10/22 | 11 |
| **N8** unit 18 needs human input; command-level flow idempotency | Unit 23 declared human-gated (D2/D14); FLOW qualification already requires restart without duplicated mutation | 13, unit 20 |
| Edge: 8→5 upgrade | **Accepted** | 6.1 #13 |
| Edge: 13→10 upgrade | **Accepted** | 6.1 #18 |
| Edge: 10→2 hard or split | **Split** (Gate A), rather than making unit 2 a hard prerequisite of 10 | 6.2 |
| Edge: 6→5 to consumer | **Accepted** | 6.2 |
| Edge: 15→2 keep with gate | Accepted (FLOW→ONT hard; ONT→GA hard) | 6.1 |
| Claim: "2 of 18 hard edges could be transitively redundant" | Two found by mechanical closure and omitted | 4.1 |

---

## 16. Additional findings from this revision's own tracing (not in either review)

1. **Non-interactive `decide` bypass** (`goals-lifecycle --operation=decide`): affects `workplan.accept` today; scoped closure for the new gate kind in unit 2; residual gap disclosed for v3's own acceptance (7.5).
2. **No declared validation and `.praxis/` git-ignored:** unit completion does not enforce qualification (R-1).
3. **Worker context mislabels weaker-kind prerequisites** as done: fixed in unit 6.
4. **Cycles are silent; duplicate relationship pairs make `Digest()` non-deterministic:** rejected at propose/accept in unit 6; v3's edges verified free of both.
5. **`AuthorityDecision` has no reason field:** the request `Reason` is the only durable place for evidence digests (7.3).
6. **Lost-turn visibility is per generation:** the successor could acquire an expired scope lease over an unreconciled predecessor consequence; unit 19 quiescence includes it.
7. **`supervise cancel|suspend` is unauthenticated:** governed by Gate C; no unit widens it.

---

## 17. Existing machinery reused (no parallel governance)

Goal establishment and `GoalBaseline` canonicalization; `goalstore.Repository` (Save/Load, proposal/review/accept/attach, `SaveReplanningSuccessor`); `WorkPlanProposal`/`Review`/`AcceptWorkPlan`/`MaterializeAcceptedPlanCandidate`; authority requests/decisions and `authority pending|decide` (ADR-097) and the owner-only-kind precedent; `goals-lifecycle continue`; `Admit` leases and `ReconcileLostTurn` (ADR-100), recovery (ADR-098), completion/evaluation/settlement (ADR-099); supervision `ActivityLog`; provider workers, catalog, capability pattern and env allowlist; `plugins/workspace` path guard; `SecureBlob`; package registration/installed help. Added state is limited to: gate-decision records (an owner-only authority kind), evidence-manifest records, advisory result/provenance records and content-addressed review bytes, an invocation-allocation event, applicability evidence, and a Goal-level control record. No new principal type, acceptance store, or status daemon.

---

## 18. Remaining uncertainties

- **U-1 Gate mechanism vs staged generations.** The gate mechanism extends the authority model (owner-only kind + a completion predicate). It is bounded (3.7) but is itself a governance-shaped change the owner accepts by accepting v3. If declined, the fallback is staging via settle→`succeed`→re-plan (costlier; 3.7).
- **U-2 T1 flag composition is unprobed.** Compatibility of `--restricted` with `--safe-mode`, `--tools ""` with `--json-schema`, coverage of claude.ai connectors/plugin MCP under `--strict-mcp-config`, auto-memory, and the location of the structured result are unverified; no provider was run here.
- **U-3 Runtime self-check availability** depends on whether the CLI exposes a tool/MCP inventory in its output.
- **U-4 Same-lineage-only environments.** If no non-Anthropic reviewer is available, the bootstrap rule cannot be met and the plan waits (or the owner rules on it); Gate B decides the permanent policy.
- **U-5 Gate materiality boundaries** (which A3 classes, which C2 outcomes) are recommendations only.
- **U-6 Validation contract path.** `.praxis/validate` inside a git-ignored directory is awkward; changing the path is a code change outside this plan (R-1).
- **U-7 Package churn.** Each contract change needs a signed goals-package successor; unit 22 owns the cadence.
- **U-8 Two hard edges omitted as implied** rely on the closure staying intact; the B-4 verifier must check closure, not only edges.
- **U-9 Gate D (weaker Codex read confinement)** is not planned; Codex support is deferred.

---

## 19. Completion report

- **Plan artifact path:** `/Users/polliard/.claude/plans/pasted-content-id-bcb7-you-are-fancy-parrot.md` (to be copied to `docs/research/dogfood/praxis-human-interface/plans/workplan-v3-claude.md` and digested at B-5)
- **Candidates:** 23 (18 preserved/rescoped + 5 new)
- **Relationships:** hard 31, consumer 24, interaction 13, advisory 3 (total 71)
- **Human-authority gates:** Gate A `authority-ceremony`, Gate B `review-independence`, Gate C `supersession-and-active-turn` (plus dossier and gate mechanism)
- **Isolated Claude profile:** Section 5.1
- **Bootstrap artifacts required before authority:** B-1…B-8 (Section 7.2)
- **Bootstrap reviewer rule:** Section 8.1
- **Codex / Claude findings:** Sections 14 and 15
- **Hard dependencies:** added `DOG→RTE`, `RPP→GB`, `RPP→ID`, `ONT→GA`, `EST→GA`, `INTERP→GA`, `BND→GC`, gate→dossier/mechanism ×6; upgraded `TRN→INT`, `EVO→EST`; downgraded `INTERP→EST`; omitted as implied `EVO→GA`, `PLN→ID`; removed none as unjustified
- **Priority/sequence:** now scheduling preference and deterministic tie-breaking only; correctness rests on the 31 hard edges (Section 4.1)
- **Remaining uncertainties:** Section 18
