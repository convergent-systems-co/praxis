# Bundle b2-issue28 — Enhanced spec

## Original content

> # Bundle b2-issue28
>
> ## Issues
> - #28 — Extend the development overlay: bundle lane
> - #29 — Extend the development overlay: recovery lane + human-interrupt
> - #30 — Refresh parity evidence after overlay lane extensions
>
> These three are bundled together because they are sequenced sub-issues of one
> feature (widen the development overlay, then refresh its own parity
> evidence to match) and share the same footprint under
> `src/overlays/development/` and `docs/parity/`. Do #28 and #29 first (either
> order, or interleaved — they touch the same files), then #30 last, since #30
> depends on #28/#29's widened coverage existing.
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b2-issue28
> branch: develop/b2-issue28, base: origin/main
>
> ## Background
>
> `src/overlays/development/graph.py` currently expresses only
> `/develop` v4's 4-node task lane (`write_tdd -> implement -> verify ->
> commit_task`). `~/ai/skills/develop/GRAPH.yaml` (read-only reference, a
> separate repository — do not modify anything under `~/ai`) defines a much
> larger graph; this bundle widens the Praxis-side overlay to also express the
> bundle lane and the recovery/human-interrupt lane, additively.
>
> ## Task 1 (issue #28): bundle lane
>
> Add to `src/overlays/development/graph.py`, following the existing
> task-lane pattern exactly:
>
> - `plan_bundle -> task_scheduler -> bundle_verify -> final_review ->
>   documentation_review -> create_pr`, with `repair_bundle` as the retry
>   branch off a failed `bundle_verify`/`final_review`.
> - Matching proof types in `manifest.py`'s `declares.proof_types` (namespaced
>   `development.*`, mirroring `development.test-pass`/
>   `development.review-approved`).
> - Extend `compat.py`'s `_EVENT_PROOF_TYPE_MAP` with the corresponding legacy
>   events (`PLAN_DONE`, `BUNDLE_VERIFY_PASSED`/`FAILED`, `REVIEW_FINDINGS`,
>   `DOC_REVIEW_DONE`, `PR_CREATED`/`BRANCH_READY`, etc).
> - Pass-through deterministic graders in `graders.py` for the new proof
>   types, matching the existing graders' shape.
>
> This is additive to the existing 4-node task-lane chain — do not restructure
> or rename anything already there.
>
> ## Task 2 (issue #29): recovery lane + human-interrupt
>
> - Add `context_recovery`, `blocker_recovery`, `awaiting_human` nodes to
>   `src/overlays/development/graph.py`, wired as the retry/escalation branch
>   off the relevant task/bundle-lane failure points.
> - Add a node that actually reaches `BLOCKED`/`HANDOFF`, making
>   `compat.legacy_status_to_node_status`'s existing mapping reachable from a
>   real graph traversal, not just a unit test of the pure function.
> - Extend `compat.py`'s event mapping with `CONCERN_TRIAGED`,
>   `TASK_REPAIR_DONE`, `NEEDS_CONTEXT` as appropriate (most of `develop`'s
>   recovery events are bookkeeping, not proof-bearing — use a parallel
>   mapping if they don't fit `_EVENT_PROOF_TYPE_MAP`'s shape).
> - A test drives the graph through a failure into
>   `context_recovery`/`blocker_recovery` and confirms `NodeStatus.BLOCKED`/
>   `HANDOFF` is reached, not just mappable in isolation.
> - Update `docs/overlays/development.md` to describe the widened graph shape.
>
> ## Task 3 (issue #30): refresh parity evidence
>
> - Update `docs/parity/state-event-migration.md`'s per-fixture tables so
>   every node/event now expressible via the widened overlay is marked
>   `expressible_in_overlay: true`, re-verifying the two fixtures currently
>   noted as reaching `terminal_success` only as a structural artifact
>   (`07-ambiguous-recovery`, `08-repair-heavy`) against the new
>   recovery-lane nodes.
> - Re-run `tests/test_parity_fixtures.py` against all 8 fixtures; every
>   fixture must still satisfy its own internal honesty invariant (no fixture
>   claims `expressible_in_overlay: true` unless it resolves to a real graph
>   node/proof type/event mapping).
> - Append a short addendum section to `docs/parity/decision.md` (do not
>   rewrite the existing T3/T4/T6/T7 sections) recording what gap categories
>   are now closed vs. still open. Performance parity stays explicitly open
>   regardless of this work.
>
> ## Acceptance
>
> - Full test suite (`pytest`) passes, including all 8 parity fixtures with
>   materially widened `expressible_in_overlay` coverage.
> - `docs/parity/decision.md` addendum is honest about what remains open.
>
> ## Resolution of prior escalation (read this before starting)
>
> A previous attempt on this bundle correctly escalated a real blocker:
> `TransitionEngine._advance_successors` only fires on `TERMINAL_SUCCESS` — it
> has no failure-triggered/conditional edge semantics at all, so
> "repair/recovery nodes wired as a retry branch off a failure" cannot be a
> real, exercised transition today. That's a core-runtime gap, not something
> this bundle can or should fix. It's now filed separately as **#32** (added
> to epic #26) — do not attempt to implement conditional edge semantics in
> `TransitionEngine` as part of this bundle.
>
> **Decided approach for this bundle: topology-only recovery edges.**
>
> - Add `context_recovery`, `blocker_recovery`, `awaiting_human`,
>   `repair_bundle` as real nodes in `Graph.nodes`, connected by real edges in
>   `Graph.edges` for bookkeeping/`expressible_in_overlay` purposes — but do
>   not claim or test that a real `TERMINAL_FAILED` transition actually
>   auto-advances into them, because it structurally cannot yet (see #32).
>   This mirrors the existing declarative-only precedent already in this
>   codebase: `docs/overlays/development.md`'s documented `conflict_fn` wiring
>   gap (a field is declared, core doesn't enforce it, and the docs say so
>   plainly) — do the same thing here, explicitly, in
>   `docs/overlays/development.md`'s updated description of the widened
>   graph.
> - The node that "reaches `BLOCKED`/`HANDOFF`" (making
>   `compat.legacy_status_to_node_status`'s existing mapping reachable) should
>   be reachable via an ordinary non-`join` **success** edge from some
>   upstream node (e.g. a node that legitimately completes into
>   `awaiting_human` as its normal next step), not via a failure edge — that
>   part does not depend on #32 at all.
> - Task 2's "a test drives the graph through a failure into
>   `context_recovery`/`blocker_recovery`" becomes: a test that manually
>   constructs a run state with a cursor already placed on
>   `context_recovery`/`blocker_recovery` (e.g. via a scripted non-`join`
>   success edge, or by directly asserting on the graph topology plus a
>   cursor placed there) and confirms `NodeStatus.BLOCKED`/`HANDOFF` from that
>   point — not a `FakeExecutor` replay that expects a real `TERMINAL_FAILED`
>   auto-advance to land there, since that mechanism does not exist yet.
> - Two existing tests assert every declared graph node reaches the same
>   terminal status for the current 4-node linear graph
>   (`tests/test_overlay_development.py` and
>   `tests/test_parity_fixtures.py`'s fixture loop). Adding nodes a happy-path
>   script never visits will break that specific assertion form. Update those
>   assertions to check the cursors a given fixture's script actually
>   produces, not blanket-iterate `graph.nodes` — do not delete or weaken the
>   underlying honesty check itself (whether a claimed node/event is real),
>   only the "every node" iteration shape.
> - Task 3's fixture-table refresh and `docs/parity/decision.md` addendum
>   should say plainly that the recovery lane's nodes/edges are now
>   structurally present (`expressible_in_overlay: true` for the node/event
>   *names*) but not yet functionally reachable via a real failure transition
>   — cross-reference #32 as the reason, the same disclosure register the
>   rest of that document already uses for the performance-parity gap. Do not
>   claim more than this.
>
> This keeps Task 1/2/3 additive to the existing 4-node chain (per Task 1's
> original instruction not to restructure it) and honest about what is and
> isn't really wired, without blocking on #32's own separate, larger design
> work.
>
> ## Delivery
>
> Open a PR against `main` referencing #28, #29, #30 with closing keywords
> (`Closes #28`, `Closes #29`, `Closes #30`). Do not merge — merge policy is
> `never` for this repository.

## Clarified acceptance criteria

1. **Task 1's node/edge set is exhaustive and closed.** Exactly the seven
   named nodes: `plan_bundle`, `task_scheduler`, `bundle_verify`,
   `final_review`, `documentation_review`, `create_pr`, `repair_bundle`. Per
   `~/ai/skills/develop/GRAPH.yaml`'s `lanes.bundle`, the bundle lane also
   technically owns `enhance_spec` — it is deliberately **not** in Task 1's
   list and must stay out of this bundle's graph; do not add it by inference
   from `GRAPH.yaml`. `bundle_scheduler` (orchestrator lane in `GRAPH.yaml`)
   is likewise excluded.
2. **Task 2's node set is exhaustive and closed.** Exactly `context_recovery`,
   `blocker_recovery`, `awaiting_human`, per `GRAPH.yaml`'s `lanes.shared`.
   `human_required` (orchestrator lane in `GRAPH.yaml`, grouped with the
   shared-lane recovery nodes only under "category H" in
   `docs/parity/state-event-migration.md`'s taxonomy, not `GRAPH.yaml`'s own
   lane structure) is **not** added as a graph node by this bundle — Task 2's
   "reaches `BLOCKED`/`HANDOFF`" requirement is satisfied by `awaiting_human`
   (clarification 8 below), which is already in Task 2's explicit list.
3. **New proof types (Task 1, bullet 2) — proposed concrete names**, following
   the existing `development.test-pass` / `development.review-approved`
   kebab-case, dot-namespaced convention
   (`schemas/v1/overlay-manifest.schema.json`'s `namespacedString` pattern,
   `^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+(-[a-z0-9]+)*)+$`) and the existing
   split in `compat.py`'s own module docstring (only *evidence-bearing
   (passing)* events get a proof type, everything else maps to `None`):
   - `PLAN_DONE` → `development.plan-done`
   - `BUNDLE_VERIFY_PASSED` → `development.bundle-verify-pass`
   - `DOC_REVIEW_DONE` → `development.doc-review-done`
   - `PR_CREATED` / `BRANCH_READY` → `development.pr-created` (both legacy
     events represent the same delivery outcome under
     `delivery.github`/`delivery.local` respectively per `GRAPH.yaml`'s
     `create_pr` node comment — map both legacy event names to the one proof
     type)
   - `BUNDLE_VERIFY_FAILED`, `REVIEW_FINDINGS` → map to `None` in
     `_EVENT_PROOF_TYPE_MAP`, same as `PERSONA_DISPATCHED` today: these are
     rejection/routing signals, not evidence that a gate passed.
   - `REVIEW_APPROVED` — already mapped to `development.review-approved`;
     leave that mapping as-is and reuse it for `final_review`'s outcome
     rather than minting a second proof type, since `GRAPH.yaml` itself uses
     the single event name `REVIEW_APPROVED` for both the task lane's
     `verify`/`commit_task` gate and the bundle lane's `final_review` gate,
     and `legacy_event_to_proof_type` is a pure per-event-name function with
     no node context to disambiguate by.
   This naming is a fill under the resolve-or-name test (existing convention
   cited, reversible, no security/deployment impact), not a hard requirement
   — the implementer may pick different literal strings as long as they
   follow the same convention.
4. **`evidence_requirement` placement (Task 1, bullets 2 + 4).** Following the
   task lane's own pattern exactly (only the lane's terminal node,
   `commit_task`, carries `evidence_requirement`; the other task-lane nodes
   carry only the declarative `requirement` metadata) — only `create_pr` (the
   bundle lane's terminal node) gets an `evidence_requirement`, requiring the
   bundle-lane proof types clarification 3 defines as passing evidence.
   Intermediate bundle-lane nodes (`plan_bundle`, `task_scheduler`,
   `bundle_verify`, `final_review`, `documentation_review`) get the same
   `requirement`-only metadata shape `write_tdd`/`implement`/`verify`
   already use.
5. **Task 2's `CONCERN_TRIAGED` / `TASK_REPAIR_DONE` / `NEEDS_CONTEXT`
   mapping.** These do not fit `_EVENT_PROOF_TYPE_MAP`'s `str | None` shape as
   pass/fail evidence (per Task 2's own text: "most of `develop`'s recovery
   events are bookkeeping, not proof-bearing"). Add a second, parallel dict in
   `compat.py` (e.g. `_EVENT_NODE_MAP: dict[str, str | None]`, mirroring
   `_EVENT_PROOF_TYPE_MAP`'s shape) mapping each of these three event names to
   the graph node id it corresponds to (`CONCERN_TRIAGED` → a triage step
   adjacent to `repair_task`/`repair_bundle`, `TASK_REPAIR_DONE` → return to
   `verify`, `NEEDS_CONTEXT` → `context_recovery`), with a matching
   `legacy_event_to_recovery_node` (or similarly-named) accessor function,
   analogous to `legacy_event_to_proof_type`. Exact function/dict naming is
   left to the implementer; the shape (a second explicit mapping, not a
   shoehorn into `_EVENT_PROOF_TYPE_MAP`) is the clarified part. Note that
   `repair_task` itself is **not** one of the nodes this bundle adds
   (clarifications 1–2's node lists are closed and neither lists it) — the
   `CONCERN_TRIAGED`/`TASK_REPAIR_DONE` mapping targets are graph nodes that
   already exist or are added by this bundle, not a new `repair_task` node.
6. **Which upstream node's success edge reaches `awaiting_human`, and how the
   test proves `NodeStatus.BLOCKED`/`HANDOFF` is reached (Task 2's "not just
   mappable in isolation" requirement).** Resolving the spec's own "Decided
   approach" ("a node that legitimately completes into `awaiting_human` as
   its normal next step") against `GRAPH.yaml`'s actual routes: `repair_bundle`
   is the only node this bundle adds whose real `GRAPH.yaml` semantics
   (`routes: exhausted: awaiting_human`) legitimately lead to `awaiting_human`.
   Concretely:
   - Add an edge `repair_bundle -> awaiting_human` (`kind` other than
     `"join"`) to `Graph.edges`. This is the edge that stands in for
     `GRAPH.yaml`'s `repair_bundle` `exhausted` route, expressed the only way
     `TransitionEngine` can express a `repair_bundle` outgoing edge today:
     unconditionally, on `repair_bundle` reaching `NodeStatus.TERMINAL_SUCCESS`
     (`TransitionEngine._advance_successors` fires for every non-`join`
     outgoing edge once the source reaches `TERMINAL_SUCCESS` —
     `src/praxis_runtime/transitions.py:216-235` — there is no way to express
     "only on the exhausted outcome" short of #32). If `repair_bundle` also
     carries a `done`-branch outgoing edge back into the bundle lane (not
     required by Task 1's or Task 2's text, and not needed to satisfy either
     task — see "explicitly out of scope"), document plainly wherever the
     edges are described that both fire together today, not conditionally.
   - Confirmed from `praxis_runtime/transitions.py`'s own `_TRANSITIONS`
     table: `NodeStatus.BLOCKED`/`HANDOFF` are reached only by applying a
     `"block"`/`"handoff"` event to a node cursor that is already `RUNNING`
     (`_TRANSITIONS[NodeStatus.RUNNING]`, lines 81-87) — never automatically
     via `_advance_successors`. So the test needs: (a) drive some prior node
     to `TERMINAL_SUCCESS` so `_advance_successors` creates a `PENDING`
     cursor at `awaiting_human` via the new edge, (b) apply `"start"` to reach
     `RUNNING`, (c) apply `"block"` or `"handoff"` to reach
     `NodeStatus.BLOCKED`/`HANDOFF`. Steps (a)-(c) can be done either by
     calling `TransitionEngine.apply()` directly node-by-node, or by
     constructing a `RunState`/cursor positioned at `repair_bundle` already
     `TERMINAL_SUCCESS` and applying from there — either satisfies "a real
     graph traversal, not just a unit test of the pure function."
   - **This cannot be a `FakeExecutor.run_to_completion()`-driven fixture
     script.** `FakeExecutor._TERMINAL_VALUES` is only
     `{TERMINAL_SUCCESS, TERMINAL_FAILED}`
     (`src/praxis_runtime/testing/fake_executor.py:18`); `BLOCKED`/`HANDOFF`
     are not terminal, so a cursor parked there is never dropped from
     `run_to_completion()`'s `non_terminal` list, no further `"start"`
     transition is legal from `BLOCKED`/`HANDOFF`
     (`_TRANSITIONS[NodeStatus.BLOCKED] == {"resume": RUNNING}`, no `"start"`
     key), and the script has only one scripted outcome per node id — the
     loop cannot converge and `run_to_completion()` never returns for such a
     script. This confirms the spec's own instruction: the Task 2 test must
     drive `TransitionEngine.apply()` directly, not go through
     `FakeExecutor`/a `benchmark/fixtures/*.json` entry.
7. **`load_graph`'s reachability check does not apply here — worth naming
   explicitly.** `src/praxis_runtime/graph.py`'s `load_graph()` (which reads a
   graph from a JSON document) enforces that every node is reachable from
   `entry_node`, failing closed with `GraphValidationError` otherwise
   (`graph.py:87-92`). `build_development_graph()` constructs the `Graph`
   dataclass directly, bypassing `load_graph()` and this check entirely — so
   the new bundle-lane and recovery-lane nodes (which have no edge from
   `write_tdd`'s existing chain, per Task 1's "do not restructure... anything
   already there") being unreachable from `entry_node="write_tdd"` is
   currently legal. If `build_development_graph()` is ever refactored to load
   from a JSON document via `load_graph()` instead of hand-constructing
   `Graph(...)`, this unreachability would then fail closed — out of scope
   for this bundle, but worth a one-line note wherever the widened graph's
   shape is documented (`docs/overlays/development.md`) so a future reader
   isn't surprised.
8. **Every fixture's per-scenario table update (Task 3) is bounded by what
   Tasks 1/2 actually make expressible**, not by "everything in
   `GRAPH.yaml`." A `legacy_expected` entry only flips to
   `expressible_in_overlay: true` if it resolves to a real graph node id, a
   real `DEVELOPMENT_MANIFEST.declares.proof_types` entry, or a legacy event
   `legacy_event_to_proof_type`/the new parallel mapping resolves to one of
   those — exactly the existing honesty-invariant check in
   `tests/test_parity_fixtures.py::test_fixture_reaches_expected_terminal_status_with_honest_expressibility_claims`,
   which checks **node-id membership in `graph.nodes`**, not reachability
   from `entry_node` (confirms clarification 7's distinction matters:
   `expressible_in_overlay: true` for `plan_bundle` etc. only requires the
   node id to exist in `graph.nodes`, not that a fixture's script actually
   reaches it). `human_required` (excluded per clarification 2) stays
   `expressible_in_overlay: false` in every fixture table; do not flip it.
   `repair_task` (not added by this bundle — clarification 5) also stays
   `expressible_in_overlay: false`.
9. **`test_overlay_development.py` and `test_parity_fixtures.py`'s "every
   graph node reaches terminal status" assertions must change shape, not
   just gain more nodes.** `test_overlay_development.py:118`
   (`assert set(final_state.cursors) == set(graph.nodes)`) and
   `test_parity_fixtures.py:109-110`
   (`for node_id in graph.nodes: assert final_state.cursors[node_id].status
   == fixture["expected_terminal_status"]`) both currently iterate
   `graph.nodes` directly. Once `graph.nodes` includes nodes no
   fixture's/existing test's script ever creates a cursor for (every
   bundle-lane and recovery-lane node — none are reachable from
   `write_tdd` per clarification 7), `final_state.cursors[node_id]` for those
   node ids raises `KeyError` (`test_parity_fixtures.py`) or the `set`
   equality fails (`test_overlay_development.py`), for **all 8 fixtures**,
   not just new ones. Both assertions need to iterate only the cursors a
   given script actually produces (e.g. `for node_id in final_state.cursors:`
   or `for node_id in script:`), not blanket-iterate `graph.nodes` — per the
   spec's own instruction, this changes the iteration shape only, not the
   underlying honesty check (whether a claimed `expressible_in_overlay: true`
   entry is real), which stays exactly as strict.
10. **`docs/parity/decision.md` addendum** is a new, separately-headed section
    (e.g. "## Addendum (issue #30): widened-overlay gap closure") appended
    after the existing content, not interleaved with the existing
    `## T3` / `## T4` / `## T6` / `## T7` sections (`docs/parity/decision.md:12,26,38,77`).
    It restates, in the same disclosure register as the rest of that
    document, which of categories B/R/H
    (`docs/parity/state-event-migration.md`'s taxonomy) are now closed vs.
    still open — per the "Decided approach," category R (repair/recovery) and
    the `awaiting_human` slice of category H become "structurally present,
    not yet functionally reachable via a real failure transition, see #32"
    rather than fully closed — and explicitly repeats that performance parity
    (`docs/parity/decision.md`'s `## T6`/`## T7` sections) remains open
    regardless.

## Explicitly out of scope

- `enhance_spec`, `bundle_scheduler` (orchestrator lane), `repair_task`
  (task lane), `human_required` (orchestrator lane), and the entire
  orchestrator lane (`scan`, `bundle`, `handoff`, `monitor_prs`,
  `cleanup_merged`, `audit_merged`, `audit_triage`, `complete`) and clean lane
  (`clean_discover` etc.) from `GRAPH.yaml` — none of these are named in Task
  1 or Task 2's explicit node lists, and none are added by this bundle.
- Implementing conditional/failure-triggered edge semantics in
  `TransitionEngine` — filed separately as #32; this bundle's recovery/retry
  edges are topology-only per the spec's own "Decided approach."
- A `repair_bundle` "done" edge back into the bundle lane (e.g.
  `repair_bundle -> bundle_verify`) — not required by Task 1's or Task 2's
  text to satisfy either task's acceptance; the implementer may add it for
  topological completeness, but if so must document that
  `TransitionEngine._advance_successors`' unconditional fan-out means both
  outgoing edges fire together on any `repair_bundle` `TERMINAL_SUCCESS`
  today, not selectively.
- `GRAPH.yaml`'s `legacy_nodes` remapping table (pre-v4 node name aliases) —
  not referenced anywhere in this bundle's tasks; no fixture currently
  exercises a legacy node name.
- Modifying anything under `~/.ai/skills/develop` or `~/ai/skills/develop` —
  explicitly forbidden by the Background section; those paths are read-only
  reference outside this repository.
- Reproducing `develop`'s actual routing/dispatch semantics (persona
  dispatch, capacity tiers, concurrency ceilings, headless tech-lead
  execution) — this bundle, like #12/#13 before it, only widens which
  node/event *names* the overlay's graph can express, not a functional
  re-implementation of the `develop` skill's runtime.
- Performance parity — `docs/parity/decision.md`'s existing T6/T7 sections
  already hold it open pending a second baseline sample or a genuinely
  comparable live Praxis run; this bundle does not attempt to close it, per
  the bundle's own Task 3 instruction.
- Wiring the development overlay into the legacy `develop` skill's actual
  dispatch path — out of scope per
  `docs/overlays/development-compat.md`'s existing framing, unaffected by
  this bundle.
- Refactoring `build_development_graph()` to load from a JSON document via
  `load_graph()` — clarification 7 only notes the reachability-check
  interaction as a documentation caveat, not a change to make.

## Assumptions made

- **Proof type names, `_EVENT_PROOF_TYPE_MAP`/`None` split for pass vs. fail
  events, `evidence_requirement` placement, and the parallel event→node
  mapping's shape** (clarifications 3, 4, 5). Evidence:
  `src/overlays/development/graph.py`'s existing task-lane pattern (only
  `commit_task` carries `evidence_requirement`), `compat.py`'s own module
  docstring (evidence-bearing vs. bookkeeping split), and
  `schemas/v1/overlay-manifest.schema.json`'s `namespacedString` pattern. All
  five resolve-or-name conditions hold: inside Task 1/2's stated scope, a
  sibling pattern already on record, doesn't change any existing test's
  meaning (these events currently map to nothing), no security/deployment
  impact, trivially correctable by the implementer if the naming is wrong.
- **`enhance_spec`/`bundle_scheduler`/`human_required`/`repair_task` stay out
  of the graph** (clarifications 1, 2, 5, 8). Evidence: Task 1 and Task 2's
  node lists are given as explicit, closed lists in the spec text itself, not
  "port the bundle lane" or "port the shared lane" in general; taking the
  literal list at face value rather than expanding it from `GRAPH.yaml`
  matches `docs/overlays/development.md`'s own established principle ("This
  is deliberately not a full port ... only needs to demonstrate the existing
  graph *can be expressed*, not reproduce every node").
- **`repair_bundle -> awaiting_human` as the concrete success edge Task 2's
  "reaches `BLOCKED`/`HANDOFF`" requirement hangs off** (clarification 6).
  Evidence: `GRAPH.yaml`'s own `repair_bundle` node definition
  (`routes: exhausted: awaiting_human`) is the only added node whose real
  semantics legitimately lead to `awaiting_human`; the spec's own "Decided
  approach" text names exactly this pattern ("a node that legitimately
  completes into `awaiting_human` as its normal next step") without naming
  the specific node, and `repair_bundle` is the only candidate available
  among the nodes this bundle actually adds. All five resolve-or-name
  conditions hold: inside Task 2's stated scope (it names the pattern, not
  the node), a sibling convention on record (`GRAPH.yaml` itself, plus the
  `conflict_fn` declarative-only precedent for how to express a structurally-
  present-but-not-fully-enforced edge), doesn't change what "reaches
  `BLOCKED`/`HANDOFF`" means, no security/deployment impact, and trivially
  correctable (a different upstream node could be substituted later without
  touching anything else).
- **The Task 2 test drives `TransitionEngine.apply()` directly rather than
  `FakeExecutor`** (clarification 6). Evidence: `FakeExecutor._TERMINAL_VALUES`
  excludes `BLOCKED`/`HANDOFF`
  (`src/praxis_runtime/testing/fake_executor.py:18`), so
  `run_to_completion()` cannot terminate once a cursor reaches either status
  — this is a mechanical fact about the existing test harness, not a design
  choice, so it isn't marked as an open question.
- **`test_overlay_development.py`'s and `test_parity_fixtures.py`'s
  "every graph node" assertions must change to "every cursor the script
  produced"** (clarification 9). Evidence: direct trace through
  `_advance_successors`' reachability and the two tests' current assertion
  code (line numbers cited in clarification 9) — this is the spec's own
  explicit instruction, restated here with the exact code locations so the
  planner doesn't have to re-derive which lines change.
- **`docs/parity/decision.md` addendum placement** as a new appended section.
  Evidence: Task 3's own instruction ("do not rewrite the existing
  T3/T4/T6/T7 sections") directly implies additive placement; matches how
  `docs/parity/state-event-migration.md`'s "This is an accepted contract
  migration" section already reads.

## Open questions

None. The prior attempt's escalation (whether recovery/retry edges could be
real, exercised `TERMINAL_FAILED`-triggered transitions) is resolved by the
spec's own "Decided approach" section: they are topology-only, `TERMINAL_FAILED`-conditional
routing is explicitly out of scope (filed as #32), and the one genuinely
open sub-question the decided approach left unstated — which specific
upstream node's success edge should reach `awaiting_human`, and how the
Task 2 test can concretely prove `NodeStatus.BLOCKED`/`HANDOFF` is reached
given `FakeExecutor`'s inability to terminate on those statuses — resolves
under the resolve-or-name test (clarification 6 and its evidence above) using
`GRAPH.yaml`'s own `repair_bundle` route and `transitions.py`'s documented
transition table, without inventing new design beyond what the spec and the
repository already establish.
