# Plan: Bundle b4-issue32 — on-failure edge semantics in TransitionEngine

Scope: enhanced spec's Task 1 only (`docs/develop/specs/b4-issue32.md`). Task 2 (overlay
retrofit of `src/overlays/development/graph.py`) is explicitly deferred pending PR #34 and is
**not** represented in this task graph.

## Design recap (from the enhanced spec — not re-derived here)

- New edge kind: `"on-failure"` (hyphenated; matches `schemas/v1/graph.schema.json`'s
  `^[a-z0-9]+(-[a-z0-9]+)*$` pattern — no schema change needed).
- `_apply_locked` (`src/praxis_runtime/transitions.py:216-217`) must attempt successor
  advancement on `TERMINAL_FAILED` as well as `TERMINAL_SUCCESS`, keyed off the resulting
  `new_status` only (not the prior status, not the event type).
- Eligibility differs by which terminal status was reached:
  - `TERMINAL_SUCCESS`: existing behavior, unchanged (`"join"` gated via `_join_ready`,
    everything else — including a new `"on-failure"` edge — does **not** fire).
  - `TERMINAL_FAILED`: only `"on-failure"`-kind outgoing edges fire, each creating a `PENDING`
    cursor unconditionally (fan-out-style — no join-on-failure gating exists or is needed).
- No reason-filtering, no schema/dataclass changes, no `praxis_policy`/budget changes, no
  `evidence_view.py` changes — all explicitly out of scope per the enhanced spec.
- Both `src/praxis_runtime/transitions.py`'s module docstring and `docs/runtime.md`'s
  `## praxis_runtime.transitions` section must describe the new edge kind (existing
  docstring/doc-sync convention in this repo).

## Tasks

### T1 — Core engine: on-failure edge routing

**Files:** `src/praxis_runtime/transitions.py`

**Interfaces:**
- `TransitionEngine._advance_successors(self, node_id: str, cursors: dict[str, Cursor], status: NodeStatus) -> None`
  (adds a required `status` parameter — the terminal `NodeStatus` just reached — so eligibility
  can differ between the success and failure paths; both call sites below pass it explicitly).
- No other public signature changes.

**Depends on:** (none)

**Steps:**
- [ ] Read `_apply_locked`, `_advance_successors`, and `_join_ready` in full
      (`src/praxis_runtime/transitions.py`) before editing, per the spec's precedent-reading
      instruction.
- [ ] In `_apply_locked`, replace the `if new_status == NodeStatus.TERMINAL_SUCCESS:
      self._advance_successors(node_id, new_cursors)` block with a call that also fires on
      `NodeStatus.TERMINAL_FAILED`, passing `new_status` through:
      `if new_status in _TERMINAL_STATUSES: self._advance_successors(node_id, new_cursors, new_status)`.
- [ ] Update `_advance_successors` to accept `status: NodeStatus` and branch eligibility on it:
      - When `status is NodeStatus.TERMINAL_SUCCESS`: keep the current loop and `"join"`
        handling exactly as today, and additionally skip any edge with `kind == "on-failure"`
        (it must not fire on success).
      - When `status is NodeStatus.TERMINAL_FAILED`: only consider edges with
        `kind == "on-failure"`; for each whose target has no cursor yet, create a `PENDING`
        cursor unconditionally (no `_join_ready` call — there is no failure-side join).
      - Any other `status` value should not reach this method (guard is implicit via the
        `_TERMINAL_STATUSES` check at the call site; do not add a redundant runtime check).
- [ ] Update the module docstring (lines 1-38) to add a sentence describing the new
      `"on-failure"` edge kind immediately after the existing fan-out/join sentence: it fires
      only when its source reaches `TERMINAL_FAILED`, creates each target's cursor
      unconditionally (fan-out-style, no join-on-failure equivalent), and never fires on
      `TERMINAL_SUCCESS`.
- [ ] Confirm by reading (no edit needed) that `schemas/v1/graph.schema.json`'s `edges[].kind`
      pattern already accepts `"on-failure"`.
- [ ] Run `.venv/bin/python -m pytest tests/test_transitions.py tests/test_overlay_development.py tests/test_parity_fixtures.py tests/test_crash_restart.py tests/test_dashboard_evidence_view.py tests/test_repair_findings_b5_issue6.py tests/test_fake_executor.py tests/test_dashboard_resource_view.py`
      to confirm this change is a no-op against every existing graph fixture (none declares an
      `"on-failure"` edge today).

### T2 — Failure-path test coverage

**Files:** `tests/test_transitions.py`

**Interfaces:** none new (uses existing `Edge`, `Node`, `Graph`, `TransitionEngine`,
`NodeStatus`, `EventLog`, `RunStateStore` from `praxis_runtime`).

**Depends on:** (none — the target behavior is fully pinned by the design recap above, so this
can be written independently of T1's edit landing; it exercises `TransitionEngine.apply`'s
existing public surface)

**Steps:**
- [ ] Add a small inline graph fixture (following `_fan_out_join_graph`'s dataclass-literal
      convention in this file) with at least: node `a` (`kind="task"`) with one outgoing edge
      `Edge(source="a", target="b", kind="on-failure")` to node `b` (`kind="task"`), plus a
      second outgoing edge of kind `"sequential"` from `a` to a third node `c`, so both
      boundary behaviors below can be checked against one fixture.
- [ ] Add a test that drives `a` to `TERMINAL_FAILED` via `TransitionEngine.apply(..., "fail")`
      from `RUNNING` (using the same `EventLog`/`RunStateStore`/`TransitionEngine` construction
      pattern as the existing tests in this file) and asserts the returned `RunState.cursors`
      contains `b` with status `NodeStatus.PENDING.value` — not merely that the edge is present
      in `Graph.edges`.
- [ ] In the same test (or a second one against the same fixture), assert `c` (reached only via
      the `"sequential"` edge) has **no** cursor after `a` reaches `TERMINAL_FAILED` — proving
      only `"on-failure"` edges fire on the failure path.
- [ ] Add a test proving the converse boundary: an `"on-failure"` edge does **not** fire when
      its source reaches `TERMINAL_SUCCESS` (drive `a` through `"complete"` instead of `"fail"`
      on a fixture with only the `on-failure` edge, and assert `b` has no cursor).
- [ ] Add a test driving `TERMINAL_FAILED` from `RECOVERING` (via `"interrupt"` then `"fail"`)
      confirming the same `on-failure` cursor creation, covering clarified criterion 4 (fires on
      reaching `TERMINAL_FAILED` regardless of prior status).

### T3 — Documentation: `docs/runtime.md`

**Files:** `docs/runtime.md`

**Interfaces:** none (prose only).

**Depends on:** (none — content is fully pinned by the design recap above)

**Steps:**
- [ ] In the `## praxis_runtime.transitions` section's "Fail-closed guarantee" bullet, add a
      sentence next to the existing fan-out/join sentence describing `"on-failure"` edges:
      they fire only when their source reaches `TERMINAL_FAILED`, create each target's cursor
      unconditionally (fan-out-style, no join-on-failure equivalent), and never fire on
      `TERMINAL_SUCCESS`.
- [ ] If `src/praxis_runtime/transitions.py`'s updated module docstring (T1) has already
      landed when this task runs, cross-check wording for factual consistency (not verbatim
      match) with what was just written there; if T1 hasn't landed yet, write directly from
      the design recap above — both derive from the same already-decided design, so no
      blocking wait is needed either way.

## Delivery note (not a task — for the PR-writing step)

The PR description for #32 must explicitly state that Task 2 (overlay retrofit of
`src/overlays/development/graph.py`) is deferred pending PR #34's merge, and must **not**
include a closing keyword for #29.

## Footprint / concurrency summary

All three tasks touch disjoint files (`src/praxis_runtime/transitions.py`,
`tests/test_transitions.py`, `docs/runtime.md`) and have no real dependency on each other's
completion, since the target behavior is fully pinned by the enhanced spec's clarified
acceptance criteria rather than needing to be discovered from another task's output. All run
concurrently.

- Critical path: any single task (T1, T2, or T3) — length 1 of ceiling 3.
- Serialized pairs: none (no footprint overlap).

## Verification note

`python3 <skill-dir>/runtime/schedule.py check|critical-path|conflicts` could not be run from
this worktree: the skill directory (`/Users/polliard/ai/skills/develop/...`) is outside this
session's sandboxed working directories (worktree root and run directory only), so the tool
calls were blocked. The DAG was verified manually instead: 3 tasks, each with a single-file,
non-overlapping footprint, zero `depends_on` edges among them — no cycle, no undeclared
footprint touch, no accidental serialization possible.
