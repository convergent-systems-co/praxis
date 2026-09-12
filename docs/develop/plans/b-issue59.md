# Plan: Bundle b-issue59 — align `_join_ready` docstring/docs with the narrowed join-edge filter

Source of truth: `docs/develop/specs/b-issue59.md` (enhanced spec). This plan implements **fix
option 2** as decided there: no runtime behavior changes, only docstring, docs, plan-record and
test changes.

## Design recap (from the enhanced spec — not re-derived here)

- `_join_ready` (`src/praxis_runtime/transitions.py:254-264`) filters incoming edges to
  `kind == "join"` and stays exactly as shipped. No executable statement in `src/` changes.
- `_advance_successors` (`src/praxis_runtime/transitions.py:232-252`) applies the join gate per
  *traversed* edge: an `"on-failure"` outgoing edge is skipped on success, a target that already
  has a cursor is skipped, a `"join"`-kind edge is gated by `_join_ready`, and any other kind
  creates the target's cursor immediately.
- Consequence to document and pin: for a target with mixed incoming-edge kinds, whichever
  eligible edge fires first creates the cursor, and the `edge.target in cursors` guard means it
  is never created twice or reset.

### Canonical wording (use this in T1, T2 and T4 — do not re-invent per task)

Both the module docstring and `docs/runtime.md` must say, in substance:

> Fan-out edges each create an independent successor cursor as soon as their source completes; a
> join edge creates its shared successor cursor only once every `"join"`-kind incoming edge of
> that target has reported `TERMINAL_SUCCESS`. Incoming edges of other kinds do not gate that
> join — each of them still creates the target's cursor on its own source's success,
> fan-out-style — so a target with mixed incoming-edge kinds is created by whichever eligible
> edge fires first, and is never created twice.

Constraint shared by T1, T2 and T4: the resulting prose must not contain the unqualified phrase
`every incoming edge` (case-insensitive). The kind qualifier must sit inside the phrase, as
above. Prose style follows each file's existing conventions: plain identifiers in the docstring,
backticked identifiers in `docs/runtime.md`.

## Tasks

### T1 — Module docstring states the narrow join rule (AC1, AC2)

**Files:** `src/praxis_runtime/transitions.py`

**Interfaces:** none. No signature, statement or behavior change — docstring text only, plus an
optional explanatory comment inside `_join_ready`.

**Depends on:** —

**Steps:**
- [ ] Replace the sentence at `src/praxis_runtime/transitions.py:10-13` ("Fan-out edges … every
      incoming edge's source has reported TERMINAL_SUCCESS.") with the canonical wording above,
      wrapped to the file's existing line width and using unbackticked identifiers to match the
      surrounding docstring style.
- [ ] Leave the `"on-failure"` sentence at lines 14-16 and the evidence/join-aggregation sentence
      at lines 20-26 exactly as they are.
- [ ] Optionally add a one-line comment above the `incoming = [...]` comprehension in
      `_join_ready` noting that the `kind == "join"` filter is deliberate (issue #59) and mirrors
      `_check_evidence`'s `join_sources`. No other edit inside the function.
- [ ] Verify no executable statement changed: `git diff -- src/praxis_runtime/transitions.py`
      shows only docstring/comment lines.
- [ ] Verify the phrase check: `grep -rni "every incoming edge" src/` returns nothing.
- [ ] Run `python -m pytest tests/test_transitions.py` from the worktree root; it must be green
      with no existing test modified.

### T2 — `docs/runtime.md` fail-closed paragraph says the same thing (AC3)

**Files:** `docs/runtime.md`

**Interfaces:** none (documentation).

**Depends on:** — (uses the canonical wording from this plan, so it does not wait on T1)

**Steps:**
- [ ] In the **Fail-closed guarantee** paragraph, replace the clause at `docs/runtime.md:107-109`
      ("Fan-out edges … every incoming edge's source has reported `TERMINAL_SUCCESS`.") with the
      canonical wording above, backticking `"join"`, `TERMINAL_SUCCESS` and other identifiers as
      the surrounding paragraph already does.
- [ ] Keep the `"on-failure"` sentence at `docs/runtime.md:110-112` and the `current_state()`
      checkpoint-validation sentence at `113-116` intact.
- [ ] Do not touch `docs/dashboard.md`, `docs/evidence.md` or any other doc — they are already
      kind-qualified and are out of scope (AC9).
- [ ] Verify `grep -rni "every incoming edge" docs/runtime.md` returns nothing.

### T3 — Regression tests pinning mixed-incoming-edge-kind join behavior (AC4, AC5, AC6)

**Files:** `tests/test_transitions.py`

**Interfaces:** new module-level fixture helper
`_mixed_incoming_kind_join_graph() -> Graph` and two new test functions (suggested names
`test_mixed_kind_join_target_created_when_join_predecessor_completes_first` and
`test_mixed_kind_join_target_not_recreated_when_non_join_predecessor_completes_first`).

**Depends on:** —

**Steps:**
- [ ] Add `_mixed_incoming_kind_join_graph()` next to `_fan_out_join_graph`
      (`tests/test_transitions.py:71-88`), following its exact construction style: nodes
      `start`, `a`, `b`, `t`; edges `start→a` (`fan-out`), `start→b` (`fan-out`), `a→t`
      (`join`), `b→t` (`sequential`); `entry_node="start"`, `terminal_nodes={"t"}`.
- [ ] AC4 test: start and complete `start`, then start and complete `a` (the `"join"`-kind
      predecessor) while `b` is still `PENDING`; assert `"t" in state.cursors` and
      `state.cursors["t"].status == NodeStatus.PENDING.value`, and assert `b`'s cursor is still
      `PENDING` (it did not gate the join).
- [ ] AC5 test: same graph, drive `b` (the `"sequential"` predecessor) to `TERMINAL_SUCCESS`
      first; assert `t`'s cursor exists at `PENDING`. Then drive `a` to `TERMINAL_SUCCESS` and
      assert `t`'s cursor is unchanged — still `PENDING`, still the same single cursor object's
      status (no reset, no duplicate) — pinning the `edge.target in cursors` guard at
      `src/praxis_runtime/transitions.py:248-249`.
- [ ] Give each test a comment stating this is deliberately pinned semantics for issue #59
      (fix option 2), not incidental behavior, and naming which of the two completion orders it
      covers.
- [ ] Place both tests immediately after
      `test_join_ignores_unrelated_on_failure_incoming_edge` (`tests/test_transitions.py:441-456`)
      so the join cluster stays contiguous.
- [ ] Do not modify `test_join_advances_only_after_every_incoming_cursor_completes`
      (`:422`) or `test_join_ignores_unrelated_on_failure_incoming_edge` (`:441`) — AC6 requires
      both to pass unmodified.
- [ ] Run `python -m pytest tests/test_transitions.py` from the worktree root; all green.

### T4 — Docstring guard test against wording regression (AC7)

**Files:** `tests/test_transitions.py`

**Interfaces:** new test function, suggested name
`test_module_docstring_join_rule_is_kind_qualified`.

**Depends on:** T1 (the guard asserts on the post-T1 docstring text; running it before T1 lands
would fail)

**Steps:**
- [ ] Add the test in the negative-assertion style already used at
      `tests/test_transitions.py:568-600`: import `praxis_runtime.transitions`, assert the module
      has a docstring, normalize with `" ".join(doc.split())` and lowercase it.
- [ ] Assert the normalized docstring does **not** contain `"every incoming edge's source"` (the
      unqualified claim), with a failure message explaining that `_join_ready`
      (`src/praxis_runtime/transitions.py:254-264`) gates only on `"join"`-kind incoming edges,
      so an unqualified claim overstates what the engine waits for (issue #59).
- [ ] Add a positive assertion that the docstring mentions the `join`-kind qualifier (e.g. that
      the normalized lowercase text contains `"join"-kind incoming edge`, matching the wording
      T1 actually wrote) so the sentence cannot be deleted outright to satisfy the negative
      assertion.
- [ ] Place it directly after
      `test_module_docstring_edge_consultation_sentence_covers_both_terminal_statuses`
      (`tests/test_transitions.py:585-600`).
- [ ] Run `python -m pytest tests/test_transitions.py` from the worktree root; all green.

### T5 — Correction note on the stale `b4-issue32` plan claim (AC8)

**Files:** `docs/develop/plans/b4-issue32.md`

**Interfaces:** none (documentation).

**Depends on:** —

**Steps:**
- [ ] Append a short, clearly-marked section at the end of
      `docs/develop/plans/b4-issue32.md` (e.g. `## Correction (issue #59)`), after the existing
      "Verification note" section.
- [ ] State that the line "existing behavior, unchanged" (`docs/develop/plans/b4-issue32.md:15-16`)
      is not accurate: `_join_ready`'s incoming-edge filter was narrowed to `"join"`-kind edges in
      that bundle, which changes when a mixed-incoming-edge-kind target is created. Reference
      issue #59 and `docs/develop/specs/b-issue59.md`, and note the narrow semantics are now the
      documented, tested behavior (fix option 2).
- [ ] Do **not** rewrite lines 15-16 or any other original content — the document is a historical
      record (spec assumption A5).

## Cross-cutting checks (every task)

- **AC9 — no other edits.** The bundle diff against `origin/main` must touch only
  `src/praxis_runtime/transitions.py`, `docs/runtime.md`, `docs/develop/plans/b4-issue32.md`,
  `tests/test_transitions.py`, and this bundle's spec/plan/tasks files. Notably
  `src/praxis_dashboard/evidence_view.py`, `docs/dashboard.md`, `docs/evidence.md` and
  `examples/sample-graph.json` must not change.
- **AC10 — suite green.** Each task finishes with `python -m pytest` from the worktree root
  (`pyproject.toml:24-26` sets `testpaths` and `pythonpath = ["src"]`, so no editable install is
  needed; pytest availability is the tech lead's environment concern per spec assumption A6).

## Footprint / concurrency summary

- T1 (`src/praxis_runtime/transitions.py`), T2 (`docs/runtime.md`), T3/T4
  (`tests/test_transitions.py`) and T5 (`docs/develop/plans/b4-issue32.md`) touch four distinct
  files.
- T3 and T4 share `tests/test_transitions.py` and therefore serialize. That is correct, not a
  defect: both add tests to the file where every existing join test lives (spec assumption A3).
  They are kept as separate tasks because T3 has no dependency and can start immediately, while
  T4 must wait for T1's docstring wording.
- No bootstrap task exists: nothing in this bundle needs scaffolding before another task can
  start.
- Critical path: T1 → T4 — length 2 of ceiling 3.
- Serialized pairs: T3 / T4 (shared `tests/test_transitions.py`).

## Verification

`python3 <skill-dir>/runtime/schedule.py` against
`docs/develop/plans/b-issue59.tasks.json`:

- `check` → `OK: tasks.json is valid`
- `critical-path` → `["T1", "T4"]`, length 2, ceiling 3, within ceiling, 5 tasks
- `conflicts` → one pair, T3 / T4 on `tests/test_transitions.py`
