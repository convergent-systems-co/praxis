# Plan — b-issue60: remaining doc-consistency drift

Bundle: `b-issue60` · Issue: #60 · Branch: `develop/b-issue60` · Base: `origin/main` (`9366f5b`)
Spec: `docs/develop/specs/b-issue60.md` (enhanced) · Tasks: `docs/develop/plans/b-issue60.tasks.json`

## Shape of the work

Five acceptance criteria across four files. AC1 and AC3 both edit `docs/overlays/development.md`,
so they are one task; AC2 splits cleanly across two files that no other criterion touches; AC4 is a
single string in one test file. AC5 is the whole-suite gate and is the only task with a real
dependency edge.

Footprints are naturally disjoint — four editing tasks (T1–T4) run concurrently with no shared
file, no hub file, and no bootstrap step. T5 depends on all four and re-declares their paths
because it is allowed to repair what it finds; that serialization is the intended dependency, not
an accidental one.

Per spec **O2**, no behavior changes: `build_development_graph()`'s nodes, edges, `entry_node`,
`terminal_nodes`, and `metadata` stay byte-identical. Per **O3**, no new tests. Per **O1**/**O5**,
`benchmark/fixtures/*`, `docs/parity/decision.md`, and `benchmark/fixtures/README.md` are untouched.

---

## T1 — Retire the "wiring-gap" cross-reference and the "terminal `create_pr`" claim

Covers **AC1** and **AC3**.

**Files**
- `docs/overlays/development.md`

**Interfaces**
None. Prose only.

**Depends on**
None.

**Steps**
- [ ] In the "Recovery lane is topology-only" paragraph (around line 58), change
      `the `conflict_fn` wiring-gap paragraph under "## Resource provider"` so it no longer calls
      the target a gap. Minimal edit: `wiring-gap paragraph` -> `wiring paragraph`.
- [ ] Do **not** introduce the literal string ``**`conflict_fn` wiring`` anywhere above the existing
      occurrence near line 109. `tests/test_repair_findings_b1_issue27.py`'s `_wiring_paragraph()`
      selects on the *first* match of that string, so a new earlier match silently reroutes those
      tests to the wrong region (spec regression constraint 1).
- [ ] In the "Bundle lane" paragraph (around line 47), reword ``The terminal `create_pr` node's
      `metadata["evidence_requirement"]` ...`` to describe `create_pr` as the bundle lane's final
      (or last) node rather than "the terminal node".
- [ ] In the same paragraph, add a short clause stating that `Graph.terminal_nodes` remains
      `{"commit_task"}`, so a reader does not re-derive the same wrong conclusion. Verify the
      current value against `src/overlays/development/graph.py` (search for `terminal_nodes=`) and
      quote it exactly as the source has it.
- [ ] Leave ``the terminal `commit_task` node`` (around line 35) unchanged — it is accurate.
- [ ] Confirm every literal `tests/test_development_overlay_doc.py` pins is still present in the
      file: the six `## ` headings, the five bundle-lane edge strings, `topology-only`,
      `` `repair_bundle` -> `awaiting_human` ``, `_advance_successors`, `on-failure`,
      `TERMINAL_FAILED`, `#32` (spec regression constraint 4).
- [ ] `grep -n "wiring-gap\|wiring gap" docs/overlays/development.md` returns nothing.
- [ ] `grep -n "terminal .create_pr" docs/overlays/development.md` returns nothing.
- [ ] `python -m pytest -q tests/test_development_overlay_doc.py tests/test_repair_findings_b1_issue27.py`
      passes.

---

## T2 — Stop the graph module docstring claiming all three recovery nodes are unwired

Covers the `src/overlays/development/graph.py` half of **AC2**.

**Files**
- `src/overlays/development/graph.py`

**Interfaces**
None. Module docstring only; no signature, node, edge, or metadata changes.

**Depends on**
None.

**Steps**
- [ ] In the module docstring's recovery-lane paragraph (around lines 30–37), the sentence
      ``None of the three are dispatched work and none are wired into the task or bundle lanes
      yet`` is the overclaim. `awaiting_human` *is* wired to the bundle lane by the
      `repair_bundle -> awaiting_human` edge.
- [ ] Reword so the "not wired" claim covers only `context_recovery` and `blocker_recovery`. The
      "none of the three are dispatched work" claim is true and may stay.
- [ ] Keep the preceding sentence (``context_recovery`` and ``blocker_recovery`` have no edges;
      ``awaiting_human`` has one incoming ``on-failure`` edge from ``repair_bundle``) — it is
      already correct.
- [ ] Keep the `-- see #32 and docs/overlays/development.md for the scoping rationale` trailer; the
      #32 retry-count/budget/exhaustion gap is still open (spec **O4**).
- [ ] Verify the edge is real before writing the claim: confirm
      `Edge(source="repair_bundle", target="awaiting_human", kind="on-failure")` exists in this file
      and cite nothing beyond what the code shows.
- [ ] Leave the four substrings `tests/test_overlay_development.py:56-66` pins intact: `requirement`,
      `declarative metadata only`, `no core module`, `currently reads or enforces it`
      (spec regression constraint 2).
- [ ] Confirm nothing outside the docstring changed: `git diff src/overlays/development/graph.py`
      shows only lines inside the triple-quoted module docstring.
- [ ] `python -m pytest -q tests/test_overlay_development.py` passes.

---

## T3 — Remove the four surviving "topology-only" groupings of `awaiting_human`

Covers the `docs/parity/state-event-migration.md` half of **AC2**.

**Files**
- `docs/parity/state-event-migration.md`

**Interfaces**
None. Prose and one table cell.

**Depends on**
None.

**Steps**
- [ ] Line ~17: `a topology-only recovery/human-interrupt lane` — reword so the lane is not called
      topology-only wholesale.
- [ ] Lines ~27–28: ``the topology-only **recovery lane** (`context_recovery`, `blocker_recovery`,
      `awaiting_human`)`` — same; either drop the qualifier or scope it to the two nodes it is true
      of.
- [ ] Line ~47: the `` `awaiting_human` `` table row's annotation
      `(topology-only placeholder, see #32)` — replace with an accurate annotation noting its
      inbound `on-failure` edge from `repair_bundle`. Keep the row's Markdown table column
      alignment consistent with its neighbours.
- [ ] Line ~400: ``the topology-only recovery lane (`context_recovery`, `blocker_recovery`,
      `awaiting_human`)`` — same treatment.
- [ ] Leave the `context_recovery` and `blocker_recovery` table rows (~45–46) unchanged; their
      `topology-only placeholder` annotation is accurate.
- [ ] Leave the already-corrected passages at ~104–107 and ~414–421 unchanged.
- [ ] Do **not** edit the blockquotes at ~343 and ~392. They are fixture `notes` strings that
      `tests/test_parity_fixtures.py::test_state_event_migration_doc_reflects_fixture_notes`
      requires to match `benchmark/fixtures/*.json` verbatim, and the fixtures are out of scope
      (spec **O1**, regression constraint 3).
- [ ] Do not declare the #32 gap closed anywhere; retry counts, budgets, and exhaustion are still
      unmodelled (spec **O4**).
- [ ] `python -m pytest -q tests/test_parity_fixtures.py` passes.

---

## T4 — Correct the stale assertion message in the overlay-doc test

Covers **AC4**.

**Files**
- `tests/test_development_overlay_doc.py`

**Interfaces**
None. One message string.

**Depends on**
None.

**Steps**
- [ ] In `test_graph_section_describes_recovery_retry_edges_as_on_failure`, the `assert "#32" in
      section` failure message (around lines 133–136) reads
      `"## Graph section must cross-reference #32, matching the conflict_fn "` /
      `"wiring-gap paragraph's disclosure register"`. Change `wiring-gap` to `wiring` (or equivalent
      wording with no "gap").
- [ ] Change the message string only. Do not change the assertion, any other assertion, any helper,
      or any test name (spec **O3**).
- [ ] Leave line ~82's message (`T10 must not rewrite the existing conflict_fn wiring section`)
      alone — it already has no "gap".
- [ ] `grep -rn "wiring-gap\|wiring gap" tests/` returns only the two matches in
      `tests/test_repair_findings_b1_issue27.py`, which intentionally regression-test the *pre-#27*
      wording and must not change.
- [ ] `python -m pytest -q tests/test_development_overlay_doc.py` passes.

---

## T5 — Whole-suite gate and cross-file consistency check

Covers **AC5**, and is the single place the four edits are checked against each other.

**Files**
- `docs/overlays/development.md`
- `src/overlays/development/graph.py`
- `docs/parity/state-event-migration.md`
- `tests/test_development_overlay_doc.py`

Declared wide on purpose: this task may need to repair a literal that T1–T4 broke in each other's
assertions. It depends on all four, so the overlap is the intended ordering, not accidental
serialization.

**Interfaces**
None.

**Depends on**
T1, T2, T3, T4

**Steps**
- [ ] Run `python -m pytest -q` from the worktree root. The spec asserts no baseline result
      (assumption A6) — if a failure appears, first check whether it is pre-existing on
      `origin/main` before attributing it to this bundle.
- [ ] `grep -rn "wiring-gap\|wiring gap" docs/ src/ tests/` returns only the two intentional
      matches in `tests/test_repair_findings_b1_issue27.py`.
- [ ] `grep -rn "topology-only" docs/parity/state-event-migration.md` shows no remaining sentence or
      row that groups `awaiting_human` with `context_recovery` and `blocker_recovery`.
- [ ] Confirm `git diff origin/main -- src/overlays/development/graph.py` contains docstring lines
      only — no change to nodes, edges, `entry_node`, `terminal_nodes`, or `metadata` (spec **O2**).
- [ ] Confirm `git diff --stat origin/main` lists exactly the four files above and nothing under
      `benchmark/` (spec **O1**, **O5**).
- [ ] Confirm no new test file, test function, or assertion was added (spec **O3**).
- [ ] Repair anything the above surfaces, then re-run `python -m pytest -q`.

---

## Dependency DAG

```
T1 ─┐
T2 ─┤
T3 ─┼─> T5
T4 ─┘
```

Critical path: two tasks (any of T1–T4, then T5). T1–T4 have fully disjoint footprints and start
together with no bootstrap.

## Coverage map

| Criterion | Task |
| --- | --- |
| AC1 — no "wiring-gap" cross-reference | T1 |
| AC2 — `graph.py` docstring overclaim | T2 |
| AC2 — parity doc's four "topology-only" spots | T3 |
| AC3 — `create_pr` not "the terminal node" | T1 |
| AC4 — stale assertion message | T4 |
| AC5 — existing suite still passes | T5 |
