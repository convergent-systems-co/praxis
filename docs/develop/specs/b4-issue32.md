# Bundle b4-issue32 (enhanced)

## Original content

> # Bundle b4-issue32
>
> ## Issues
> - #32 — Add failure-triggered (conditional) edge semantics to TransitionEngine
>
> ## Repository
> convergent-systems-co/praxis (github delivery)
>
> ## Worktree
> /Users/polliard/.ai/worktrees/convergent-systems-co/praxis/develop-b4-issue32
> branch: develop/b4-issue32, base: origin/main
>
> ## Environment
>
> A `.venv` already exists in this worktree with the package installed in
> editable mode (`.venv/bin/python -m pip install -e .[dev]`). Use
> `.venv/bin/python -m pytest` for tests and
> `.venv/bin/python -m pip install -e .[dev]` for the build step — do not use
> a bare `pytest`/`pip` command or a global `--break-system-packages` install;
> this machine's Python is externally-managed (Homebrew/PEP 668) and a global
> install will corrupt other concurrent worktrees' test runs (confirmed
> earlier this run).
>
> ## Scope note: #29 is not yet merged into this branch
>
> This branch is cut from `origin/main`, which does **not** yet include #28/#29's
> topology-only recovery-lane nodes (PR #34, still open — merge policy is
> `never` for this repo, so only a human merges it). **Scope this bundle to
> Task 1 only** (below) — the core `TransitionEngine` change, which is
> independent of #28/#29's overlay code. **Do not attempt Task 2** (retrofitting
> `src/overlays/development/graph.py`'s topology-only recovery nodes to use
> the new conditional edges) — that code doesn't exist on this branch yet.
> Task 2 is explicitly deferred to a follow-up once PR #34 merges; say so
> plainly in the PR description for #32, and leave #29 unresolved (don't add
> a closing keyword for it).
>
> ## Background
>
> Discovered while building #28/#29: `TransitionEngine._advance_successors`
> (`src/praxis_runtime/transitions.py`) only creates successor cursors when a
> node reaches `TERMINAL_SUCCESS`. There is no "on failure, go here" edge
> semantic anywhere in the engine — reaching `TERMINAL_FAILED` never advances
> to any node, scripted or otherwise. `edge.kind == "join"` is the only edge
> kind treated specially today; any other edge kind behaves identically to a
> plain sequential edge and only ever fires on success.
>
> This is a core-runtime gap: no overlay can express real recovery/retry/
> escalation routing until `TransitionEngine` itself can conditionally route
> on failure.
>
> ## Task 1: conditional/failure-triggered edge semantics in TransitionEngine
>
> Design and implement:
>
> - A graph edge attribute/kind that fires when its source node reaches
>   `TERMINAL_FAILED` (or a specific failure reason), not just
>   `TERMINAL_SUCCESS`. Read `src/praxis_runtime/transitions.py`'s
>   `_advance_successors` and `edge.kind` handling in full before designing —
>   follow the existing `"join"` special-case as the precedent for how a new
>   edge kind gets threaded through `_apply_locked`/`_advance_successors`.
> - Decide how this interacts with the existing `edge.kind` vocabulary (e.g. a
>   new kind such as `"on_failure"`) and with `praxis_policy`'s existing
>   bounded retry/repair budget machinery (`docs/policy.md`) — a failure edge
>   that fires unconditionally on every failure could bypass budget
>   enforcement if not integrated carefully. Read `docs/policy.md` in full
>   before deciding; if budget integration is a real, non-trivial design
>   question, it is fine (expected, even) to escalate that specific sub-point
>   via the enhance_spec/planning step rather than guessing — but do not
>   escalate the top-level "should this exist" question again; that has
>   already been decided (yes, build it).
> - Update `docs/runtime.md` to document the new edge semantics.
> - A test drives a small graph through a real `TERMINAL_FAILED` transition
>   and confirms the conditional edge's target node gets a cursor — not just
>   that the edge exists in `Graph.edges`.
> - Existing tests (`tests/test_overlay_development.py`,
>   `tests/test_parity_fixtures.py`, and any other `TransitionEngine` test)
>   must be unaffected, or updated deliberately with a clear reason, not
>   incidentally broken.
>
> ## Acceptance
>
> - Full test suite (`.venv/bin/python -m pytest`) passes.
> - `docs/runtime.md` documents the new edge kind/semantics.
> - PR description explicitly states Task 2 (overlay retrofit) is deferred
>   pending #34's merge, and does not close #29.
>
> ## Delivery
>
> Open a PR against `main` referencing #32 with a closing keyword (`Closes
> #32`). Do not merge — merge policy is `never` for this repository.

## Clarified acceptance criteria

1. **New edge kind name: `"on-failure"` (hyphen, not underscore).** The spec's example
   name, `"on_failure"`, would fail schema validation: `schemas/v1/graph.schema.json`'s
   `edges[].kind` pattern is `^[a-z0-9]+(-[a-z0-9]+)*$` (lowercase alphanumerics separated
   only by hyphens — underscore is not in the character class). Every existing edge kind in
   the repo already follows the hyphenated form (`"sequential"`, `"fan-out"`, `"join"` —
   see `tests/conftest.py::_linear_graph`, `tests/test_transitions.py::_fan_out_join_graph`,
   `src/overlays/development/graph.py`, `src/overlays/trivial/overlay.py`). Use
   `"on-failure"`.

2. **`_apply_locked` must route on `TERMINAL_FAILED`, not just `TERMINAL_SUCCESS`.**
   Today (`src/praxis_runtime/transitions.py:216-217`) successor-advancement is gated
   entirely on success:
   ```python
   if new_status == NodeStatus.TERMINAL_SUCCESS:
       self._advance_successors(node_id, new_cursors)
   ```
   This call must also run when `new_status == NodeStatus.TERMINAL_FAILED`, with the
   per-edge-kind eligibility rules differing by which terminal status was reached (see next
   point). This is the actual gap: reaching `TERMINAL_FAILED` today never even attempts
   successor advancement, regardless of `edge.kind`.

3. **On-failure edges use fan-out semantics, not join semantics, on the failure path.**
   The spec directs following `"join"` as "the precedent for how a new edge kind gets
   threaded through," which is precedent for the *mechanism* (an `edge.kind` check inside
   `_advance_successors`/`_apply_locked`), not for the *readiness rule*. Concretely: when a
   node reaches `TERMINAL_FAILED`, every outgoing edge of kind `"on-failure"` whose target
   has no cursor yet gets a `PENDING` cursor created immediately (same unconditional rule
   `_advance_successors` already applies to `"fan-out"`/`"sequential"` edges on success) —
   there is no failure-side equivalent of `_join_ready` gating multiple incoming
   `"on-failure"` edges. The spec's Task 1 only asks for a single-edge failure test and does
   not describe multi-source failure convergence, and nothing in the existing codebase
   (`docs/policy.md`'s four-state mapping, `src/overlays/development/graph.py`'s
   topology-only recovery nodes) implies one is needed for this bundle.

4. **On-failure edges fire on reaching `TERMINAL_FAILED` from any prior state, not on a
   specific event or prior status.** `TERMINAL_FAILED` is reachable via the `"fail"` event
   from both `RUNNING` and `RECOVERING` (`_TRANSITIONS`). Successor-advancement should key
   off the resulting `new_status`, exactly as the existing success path does — it does not
   need to special-case which prior status the node came from.

5. **Existing tests are unaffected by construction, not just by diligence.** No graph in
   the current test suite (`tests/test_transitions.py`, `tests/test_overlay_development.py`,
   `tests/test_parity_fixtures.py`, `tests/test_crash_restart.py`,
   `tests/test_dashboard_evidence_view.py`, `src/overlays/development/graph.py`,
   `src/overlays/trivial/overlay.py`) declares an edge of kind `"on-failure"` — confirmed by
   grepping every `Edge(...)`/`kind="..."` construction in `src/` and `tests/`. Since the new
   TERMINAL_FAILED routing only creates cursors for `"on-failure"`-kind edges, it is a
   strict no-op against every graph that exists today; the "must be unaffected" acceptance
   criterion should hold without needing to touch any of those files. Confirm this with a
   full test run rather than assuming it, since this reasoning could be wrong if a graph
   fixture is added by a concurrent bundle before this one lands.

6. **Both documentation sources need the update, not just `docs/runtime.md`.**
   `src/praxis_runtime/transitions.py`'s module docstring (lines 1–38) and
   `docs/runtime.md`'s `## praxis_runtime.transitions` section currently carry near-duplicate
   prose describing the same success/join/fan-out behavior (compare the docstring's "Fan-out
   edges each create an independent successor cursor..." sentence to the identical sentence
   under `docs/runtime.md`'s "Fail-closed guarantee" bullet) — this is the repo's existing
   convention for keeping the two in sync. Update both to describe the new `"on-failure"`
   edge kind and when it fires, not only `docs/runtime.md`.

## Explicitly out of scope

- **Task 2 (overlay retrofit of `src/overlays/development/graph.py`)** — unchanged from the
  original spec: deferred until PR #34 merges. #29 stays unresolved; no closing keyword for
  it in this bundle's PR.
- **Filtering an on-failure edge by a specific failure reason.** The spec names this as an
  optional elaboration ("or a specific failure reason"), not a required one. There is no
  existing convention to build it against: `Edge` has exactly three fields
  (`source`/`target`/`kind`) and `schemas/v1/graph.schema.json`'s edge object schema is
  `additionalProperties: false`, so a reason-matching field would be a new schema surface;
  `TransitionEngine.apply()`'s signature (`node_id`, `event_type`, `evidence`) has no `reason`
  parameter either, and the one place a failure gets classified today
  (`praxis_policy.failure_classification.classify_failure`, reading
  `payload["failure_class"]`) lives in `praxis_policy`, a layer above `TransitionEngine` that
  by design (`docs/policy.md`) never calls `TransitionEngine.apply` itself and that
  `TransitionEngine` has no dependency on. Building reason-specific routing would mean
  designing a new schema field and engine parameter from scratch — a real interface-precision
  decision beyond what Task 1 scopes. Implement the unconditional "fires on any
  `TERMINAL_FAILED`" version; reason-specific routing is a candidate for a future issue if a
  concrete overlay need for it shows up (e.g. during the Task 2 retrofit).
- **Any change to `src/praxis_dashboard/evidence_view.py`.** It only special-cases
  `edge.kind == "join"` (mirroring `_check_evidence`'s join-source lookup, for evidence
  gating) and is unaffected by a new edge kind that isn't `"join"`.
- **Any change to `docs/policy.md`.** Its four-state mapping table describes `NodeStatus`
  transitions the policy layer drives via `event_type`, not graph-edge routing; adding an
  edge kind at the `TransitionEngine` layer doesn't change anything that table documents (see
  the budget-integration assumption below for why).

## Assumptions made

- **On-failure edges don't bypass `praxis_policy`'s retry/repair budget enforcement, because
  that enforcement already completes entirely before a node ever reaches `TERMINAL_FAILED`.**
  Per `docs/policy.md`'s "four-state mapping" and `PolicyGate.decide_on_failure`: a transient
  failure with retry budget remaining routes to `BLOCKED`/`RECOVERING` (`"block"`/`"resume"`
  events), consuming budget on each loop; only once budget is exhausted, the failure is
  classified `SUBSTANTIVE`, or a human explicitly denies via `HANDOFF -> accept -> RUNNING ->
  fail`, does a node ever actually transition to `TERMINAL_FAILED`. `praxis_policy` also
  never calls `TransitionEngine.apply` itself and never imports `praxis_runtime.transitions`
  — by the existing "decide here, wire there" design, `TransitionEngine` has no visibility
  into budget state at all, on the success path or the failure path. On-failure edges are
  therefore routing that happens *after* whatever policy decision already produced a real
  `TERMINAL_FAILED`, the same relationship success-path edges already have to any policy
  decision that produced a real `TERMINAL_SUCCESS` — this is not new coupling, and there is
  nothing for it to bypass. Evidence: `docs/policy.md`'s "The four-state mapping" table and
  "`praxis_policy` never imports `praxis_runtime.transitions`..." paragraph. This resolves the
  spec's explicitly-permitted escalation point (Task 1's second bullet) rather than leaving it
  as an open question, since the repository already answers it.
- **Edge kind name is `"on-failure"`**, per Clarified acceptance criterion 1 (schema pattern
  + existing naming convention).
- **On-failure edges use fan-out (unconditional, per-edge), not join, readiness semantics**,
  per Clarified acceptance criterion 3 (nothing in the spec or existing code calls for a
  failure-side join, and the schema/dataclass have no field to express one).
- **Failure-reason filtering is out of scope for this bundle**, per the "Explicitly out of
  scope" section above (no existing schema/interface surface to build it against without a
  new design of its own).

## Open questions

None. Every gap the original spec flagged as possibly needing escalation (the budget
interaction) resolves cleanly from `docs/policy.md`'s own documented design; the remaining
gaps (edge-kind naming, readiness semantics, doc-sync scope) all resolve from concrete,
on-record evidence in the repository per the resolve-or-name test.
