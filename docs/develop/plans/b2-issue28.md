# Bundle b2-issue28 — Implementation plan

Source: `docs/develop/specs/b2-issue28.md` (spec-enhancer output; plan against its
"Clarified acceptance criteria" / "Assumptions made" sections, not the raw issue text).

## Design summary carried into every task below

- **Node sets are closed lists**, per spec clarifications 1–2:
  - Bundle lane (issue #28): `plan_bundle`, `task_scheduler`, `bundle_verify`,
    `final_review`, `documentation_review`, `create_pr`, `repair_bundle`.
  - Recovery lane (issue #29): `context_recovery`, `blocker_recovery`, `awaiting_human`.
  - `enhance_spec`, `bundle_scheduler`, `human_required`, `repair_task` are **not** added by
    this bundle (explicitly out of scope).
- **Recovery-lane nodes are topology-only placeholders** (per spec's "Decided approach" and
  the filed-separately #32 core-runtime gap): `context_recovery` and `blocker_recovery` get
  **no edges at all** — inventing a "verify/implement → context_recovery" edge would fire
  unconditionally on the *upstream node's success* (per
  `TransitionEngine._advance_successors`, `src/praxis_runtime/transitions.py:216-235`), which
  would misrepresent a failure-branch as a success-branch. Only one edge is added anywhere in
  the recovery lane: `repair_bundle -> awaiting_human` (T3), because `repair_bundle`'s own
  `GRAPH.yaml` semantics (`exhausted -> awaiting_human`) are the one case the spec's
  resolve-or-name test (clarification 6) actually resolves.
- **Capability-kind reuse:** new bundle-lane task nodes reuse the two existing
  `development.*` capability kinds (`development.code-generation`, `development.code-review`)
  for their declarative `requirement` metadata rather than minting new ones — this field is
  inert (no core module enforces it, per `graph.py`'s own module docstring), so minting new
  capability kinds is unnecessary scope. Recovery-lane nodes get no `requirement` metadata at
  all (they are not dispatched work).
- **New proof types** (clarification 3, exact names given — implementer may rename if a
  strictly better fit turns up, per the spec's own resolve-or-name framing, but keep the same
  kebab-case dot-namespaced convention):
  `development.plan-done`, `development.bundle-verify-pass`, `development.doc-review-done`,
  `development.pr-created`. `BUNDLE_VERIFY_FAILED`/`REVIEW_FINDINGS` map to `None` (rejection
  signals, not passing evidence). `REVIEW_APPROVED` is reused as-is for `final_review`'s gate —
  no new proof type.
- **`evidence_requirement` placement** (clarification 4): only `create_pr` (bundle lane's
  terminal node) gets one, requiring the four new proof types above, mirroring how only
  `commit_task` carries one in the existing task lane. All other new nodes get `requirement`
  (bundle lane) or nothing (recovery lane).
- **New event→node mapping** (clarification 5): a second dict in `compat.py`,
  `_EVENT_NODE_MAP`, distinct in shape from `_EVENT_PROOF_TYPE_MAP` because these events are
  bookkeeping, not evidence: `CONCERN_TRIAGED -> repair_bundle`, `TASK_REPAIR_DONE -> verify`,
  `NEEDS_CONTEXT -> context_recovery`, with an accessor `legacy_event_to_recovery_node`.
- **The BLOCKED/HANDOFF test** (clarification 6) cannot use `FakeExecutor` (its
  `_TERMINAL_VALUES` excludes `BLOCKED`/`HANDOFF`, so `run_to_completion()` never converges once
  a cursor parks there). It must drive `TransitionEngine.apply()` directly. See T8's Steps for
  the exact call sequence.
- **`load_graph()`'s reachability check does not apply** (clarification 7):
  `build_development_graph()` hand-constructs `Graph(...)` and never calls `load_graph()`, so
  the new nodes being unreachable from `entry_node="write_tdd"` is legal today. Worth one line
  in `docs/overlays/development.md` (T10) so a future reader isn't surprised.
- **Fixture "expressible" flips are membership-only** (clarification 8):
  `test_fixture_reaches_expected_terminal_status_with_honest_expressibility_claims` checks
  node-id membership in `graph.nodes` / proof-type membership / event-mapping resolution — not
  reachability. `human_required` and `repair_task` stay `false` in every fixture; everything
  else the widened overlay names becomes real and should flip `true`.
- **Test iteration-shape fix** (clarification 9): both
  `tests/test_overlay_development.py` (`assert set(final_state.cursors) == set(graph.nodes)`
  plus its following loop) and `tests/test_parity_fixtures.py` (`for node_id in graph.nodes:
  assert final_state.cursors[node_id]...`) currently blanket-iterate `graph.nodes`. Once
  `graph.nodes` includes nodes no script ever creates a cursor for, this raises `KeyError`/fails
  equality for **all 8 fixtures**, not just new ones. Fix: iterate `final_state.cursors` (the
  cursors a script actually produced), not `graph.nodes`. This changes iteration shape only —
  the honesty check itself (T9) stays exactly as strict.
- **`docs/parity/decision.md` addendum** (clarification 10) is a new appended section, not a
  rewrite of `## T3`/`## T4`/`## T6`/`## T7`.

## Tasks

### T1 — Bundle-lane nodes/edges in `graph.py` (issue #28)

**Files:** `src/overlays/development/graph.py`

**Interfaces:** Extends `build_development_graph()`'s `nodes` dict and `edges` list in place
(no signature change). Adds a module-level `_BUNDLE_EVIDENCE_REQUIREMENT` dict constant
mirroring the existing `_EVIDENCE_REQUIREMENT` shape.

**Depends on:** none

**Steps:**
- [ ] Add seven nodes to the `nodes` dict in `build_development_graph()`: `plan_bundle`,
      `task_scheduler`, `bundle_verify`, `final_review`, `documentation_review`, `create_pr`,
      `repair_bundle`. Give each a `kind` string following the existing kebab-case convention
      (e.g. `"plan-bundle"`, `"task-scheduler"`, ...).
- [ ] Give `plan_bundle`, `task_scheduler`, `bundle_verify`, `final_review`,
      `documentation_review` a `metadata={"requirement": _requirement(...)}` using one of the
      two existing capability kinds (`development.code-generation` or
      `development.code-review`) — pick whichever reads closer to the node's real role, no new
      capability kind needed (this field is declarative-only, per the module docstring).
- [ ] Give `create_pr` `metadata={"requirement": _requirement(...), "evidence_requirement":
      _BUNDLE_EVIDENCE_REQUIREMENT}`. Define `_BUNDLE_EVIDENCE_REQUIREMENT` as a module-level
      constant (same shape as `_EVIDENCE_REQUIREMENT`) requiring all four new proof types as
      `"required"`: `development.plan-done`, `development.bundle-verify-pass`,
      `development.doc-review-done`, `development.pr-created`. (These proof types are declared
      in `manifest.py` by T4, independently — this task only needs the literal strings, no
      import from `manifest.py`.)
- [ ] Give `repair_bundle` `metadata={"requirement": _requirement(...)}` (same pattern, no
      `evidence_requirement` — it is not the bundle lane's terminal node).
- [ ] Add edges (non-`"join"` kind, matching the existing `"sequential"` convention) to the
      `edges` list: `plan_bundle -> task_scheduler`, `task_scheduler -> bundle_verify`,
      `bundle_verify -> final_review`, `final_review -> documentation_review`,
      `documentation_review -> create_pr`. Add `bundle_verify -> repair_bundle` and
      `final_review -> repair_bundle` (the "retry branch off a failed bundle_verify/
      final_review" the spec names) — note in a code comment, mirroring this plan's summary
      above, that these fire unconditionally on the source's `TERMINAL_SUCCESS`
      (`TransitionEngine._advance_successors`), not conditionally on a failure outcome; this is
      the same acknowledged gap `docs/parity/decision.md`/`docs/overlays/development.md` already
      disclose elsewhere (filed as #32), not something this task can fix.
- [ ] Do **not** modify `entry_node` or `terminal_nodes`, and do not touch the existing
      `write_tdd`/`implement`/`verify`/`commit_task` nodes or edges.
- [ ] Update the module docstring's opening description to mention the bundle lane now exists
      alongside the task lane (one or two sentences; T2/T10 will layer on top of this for the
      recovery lane and the full doc page respectively — don't try to write the final wording
      here, just don't leave the docstring claiming only a 4-node graph).
- [ ] Run `pytest tests/test_overlay_development.py tests/test_parity_fixtures.py` — expect the
      existing `assert set(final_state.cursors) == set(graph.nodes)`-shaped assertions to now
      fail (that's expected and is T8/T9's job to fix, not this task's).

---

### T2 — Recovery-lane nodes in `graph.py` (issue #29)

**Files:** `src/overlays/development/graph.py`

**Interfaces:** Extends `build_development_graph()`'s `nodes` dict in place.

**Depends on:** none (touches the same file as T1/T3 — will serialize with them by footprint,
correctly; no ordering is required between T1 and T2 themselves)

**Steps:**
- [ ] Add three nodes to the `nodes` dict: `context_recovery`, `blocker_recovery`,
      `awaiting_human`, each with `metadata={}` (no `requirement`, no `evidence_requirement` —
      per this plan's design summary, these are topology-only placeholders, not dispatched
      work).
- [ ] Add **no edges** for `context_recovery` or `blocker_recovery` in this task — see the
      design summary above for why (an edge from an upstream node would fire on that node's
      *success*, misrepresenting a failure branch). T3 adds the one recovery-lane edge that is
      legitimate.
- [ ] Add a one-paragraph module docstring note (or extend T1's addition if it lands first)
      naming the recovery lane and citing that its nodes are topology-only per #32 — again,
      T10 owns the full doc-page rewrite; keep this to what's needed so the module docstring
      isn't misleading on its own.
- [ ] Do not touch `entry_node`, `terminal_nodes`, or the existing task-lane nodes/edges.

---

### T3 — Wire `repair_bundle -> awaiting_human` edge

**Files:** `src/overlays/development/graph.py`

**Interfaces:** Adds one `Edge` to `build_development_graph()`'s `edges` list.

**Depends on:** T1 (needs `repair_bundle` to exist), T2 (needs `awaiting_human` to exist)

**Steps:**
- [ ] Add `Edge(source="repair_bundle", target="awaiting_human", kind="sequential")` (or
      whichever non-`"join"` kind string T1 used elsewhere) to `edges`.
- [ ] Add a code comment at the edge (or extend the one T1 left on `repair_bundle`'s other
      outgoing edges) stating plainly: this edge fires unconditionally on `repair_bundle`
      reaching `TERMINAL_SUCCESS`, standing in for `GRAPH.yaml`'s `repair_bundle` `exhausted`
      route — there is no way to express "only on the exhausted outcome" without the
      conditional-edge semantics filed separately as #32.
- [ ] Confirm (read, don't re-derive) `src/praxis_runtime/transitions.py:216-235`
      (`_advance_successors`) to double check the edge `kind` you chose is not `"join"` — a
      `"join"` edge here would additionally require `_join_ready`, which is not the intent.
- [ ] Run `pytest tests/test_overlay_development.py -k terminal_success` — should still pass
      for the existing 4-node scripted test paths (this edge is not reachable from
      `write_tdd`, so it doesn't affect them); the two `set(...)==set(graph.nodes)`-shaped
      assertions remain failing until T8 lands, which is expected.

---

### T4 — New bundle-lane proof types in `manifest.py`

**Files:** `src/overlays/development/manifest.py`

**Interfaces:** Extends `_DOCUMENT["declares"]["proof_types"]` (no function signature change).

**Depends on:** none

**Steps:**
- [ ] Add four strings to `_DOCUMENT["declares"]["proof_types"]`: `"development.plan-done"`,
      `"development.bundle-verify-pass"`, `"development.doc-review-done"`,
      `"development.pr-created"`.
- [ ] Verify each matches `schemas/v1/overlay-manifest.schema.json`'s `namespacedString`
      pattern (`^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+(-[a-z0-9]+)*)+$`) before committing — cite
      the check in a comment only if you deviate from the names given here.
- [ ] Do not touch `declares.capability_kinds`, `resource_types`, or
      `requested_capability_kinds` — this bundle reuses existing capability kinds (see design
      summary).
- [ ] Run `pytest tests/test_overlay_development.py -k manifest` to confirm
      `load_manifest`/schema validation still passes with the widened list.

---

### T5 — Pass-through graders for the new proof types

**Files:** `src/overlays/development/graders.py`

**Interfaces:** Extends `build_development_grader_registry()`'s registry dict (no signature
change); reuses the existing `_StatusPassthroughGrader` class.

**Depends on:** T4 (`build_namespaced_grader_registry` raises `ValueError` if a grader's proof
type isn't in `DEVELOPMENT_MANIFEST.declares.proof_types` — `src/praxis_overlay/evidence.py:21-25`
— so this task cannot land correctly before T4's manifest entries exist)

**Steps:**
- [ ] Add four entries to the dict passed into `build_namespaced_grader_registry`, one per new
      proof type from T4, each keyed `(proof_type, "deterministic")` and valued
      `_StatusPassthroughGrader()` — identical shape to the two existing entries.
- [ ] Run `pytest tests/test_overlay_development.py -k register_development_overlay` to confirm
      `register_development_overlay` still activates cleanly (this call path constructs the
      grader registry and would raise if T4 hadn't landed).

---

### T6 — `compat.py`: extend `_EVENT_PROOF_TYPE_MAP` for bundle-lane events

**Files:** `src/overlays/development/compat.py`

**Interfaces:** Extends the module-level `_EVENT_PROOF_TYPE_MAP` dict (no function signature
change to `legacy_event_to_proof_type`).

**Depends on:** none (uses literal proof-type strings; does not import `manifest.py`)

**Steps:**
- [ ] Add to `_EVENT_PROOF_TYPE_MAP`: `"PLAN_DONE": "development.plan-done"`,
      `"BUNDLE_VERIFY_PASSED": "development.bundle-verify-pass"`,
      `"DOC_REVIEW_DONE": "development.doc-review-done"`,
      `"PR_CREATED": "development.pr-created"`, `"BRANCH_READY": "development.pr-created"` (both
      legacy events map to the same proof type — they represent the same delivery outcome under
      `delivery.github`/`delivery.local` respectively), `"BUNDLE_VERIFY_FAILED": None`,
      `"REVIEW_FINDINGS": None`.
- [ ] Leave the existing `"REVIEW_APPROVED": "development.review-approved"` entry unchanged —
      it is reused as-is for `final_review`'s gate too (no new proof type, no code change here).
- [ ] Extend the module docstring's description of what `legacy_event_to_proof_type` covers to
      mention the bundle-lane events now included, keeping the existing "representative slice,
      not full transliteration" framing.
- [ ] Run `pytest tests/test_overlay_development.py` (full file) to confirm nothing regresses.

---

### T7 — `compat.py`: add `_EVENT_NODE_MAP` / `legacy_event_to_recovery_node`

**Files:** `src/overlays/development/compat.py`

**Interfaces:** New module-level dict `_EVENT_NODE_MAP: dict[str, str | None]` and new function
`legacy_event_to_recovery_node(legacy_event: str) -> str | None`.

**Depends on:** none (touches the same file as T6 — will serialize with it by footprint; no
code-level dependency between the two)

**Steps:**
- [ ] Add `_EVENT_NODE_MAP = {"CONCERN_TRIAGED": "repair_bundle", "TASK_REPAIR_DONE": "verify",
      "NEEDS_CONTEXT": "context_recovery"}`.
- [ ] Add `legacy_event_to_recovery_node(legacy_event: str) -> str | None:` returning
      `_EVENT_NODE_MAP.get(legacy_event)`, mirroring `legacy_event_to_proof_type`'s shape
      exactly.
- [ ] Add a docstring paragraph (module-level or on the function) explaining why this is a
      second, parallel mapping rather than folded into `_EVENT_PROOF_TYPE_MAP`: these three
      events are bookkeeping/routing signals, not pass/fail evidence, so they don't fit that
      dict's `str | None`-as-proof-type shape.
- [ ] Note in a comment that `repair_task` is deliberately not one of the target node ids here
      (it is not added by this bundle — `CONCERN_TRIAGED` maps to `repair_bundle` instead, the
      "adjacent" node that does exist).
- [ ] Run `pytest tests/test_overlay_development.py` (full file) to confirm nothing regresses.

---

### T8 — `test_overlay_development.py`: fix iteration shape, add BLOCKED/HANDOFF traversal test

**Files:** `tests/test_overlay_development.py`

**Interfaces:** None new (test-only file); adds new test function(s), e.g.
`test_repair_bundle_success_edge_reaches_awaiting_human_blocked_status`.

**Depends on:** T3 (needs `repair_bundle`, `awaiting_human`, and the edge between them to exist)

**Steps:**
- [ ] In `test_development_graph_reaches_terminal_success_with_passing_evidence`, change
      `assert set(final_state.cursors) == set(graph.nodes)` and the following
      `for node_id in graph.nodes:` loop to iterate `final_state.cursors` instead (e.g.
      `for node_id in final_state.cursors: assert final_state.cursors[node_id].status ==
      NodeStatus.TERMINAL_SUCCESS.value`), dropping the now-invalid full-equality assertion
      against `graph.nodes` (`graph.nodes` now includes bundle/recovery-lane nodes this script
      never reaches). Leave the `script = {node_id: ... for node_id in graph.nodes}` line as-is
      — it's harmless (`FakeExecutor` only consults script entries for cursors that actually
      exist).
- [ ] Add a new test that proves `NodeStatus.BLOCKED`/`HANDOFF` is reached via a real graph
      traversal (not `FakeExecutor` — it cannot terminate once a cursor parks at `BLOCKED`/
      `HANDOFF` since `FakeExecutor._TERMINAL_VALUES` excludes both). Concretely:
      1. Build `graph = build_development_graph()`, a fresh `TransitionEngine` over a temp
         `RunStateStore`/`EventLog` (reuse `_build_engine`), but seed the initial `RunState`
         directly via the store with a single cursor already at `repair_bundle`,
         `NodeStatus.PENDING.value` (bypassing `engine.current_state()`'s default of seeding
         `entry_node`, since `repair_bundle` isn't reachable from `write_tdd`).
      2. `engine.apply("repair_bundle", "start")` → `RUNNING`.
      3. `engine.apply("repair_bundle", "complete")` → `TERMINAL_SUCCESS`; this triggers
         `_advance_successors`, which should create a `PENDING` cursor at `awaiting_human` via
         the new edge — assert it exists in the returned state.
      4. `engine.apply("awaiting_human", "start")` → `RUNNING`.
      5. `engine.apply("awaiting_human", "block")` (or `"handoff"`) → assert the resulting
         cursor status equals `NodeStatus.BLOCKED.value` (or `NodeStatus.HANDOFF.value`).
      6. Optionally also assert `compat.legacy_status_to_node_status("waiting_human") ==
         NodeStatus.BLOCKED` (or `"handoff"` → `NodeStatus.HANDOFF`) alongside, to explicitly
         tie this real-traversal proof back to the existing pure-function mapping the spec asks
         to make "reachable, not just mappable in isolation."
- [ ] Run `pytest tests/test_overlay_development.py -v` — all tests, old and new, must pass.

---

### T9 — `test_parity_fixtures.py`: fix iteration shape, widen honesty check

**Files:** `tests/test_parity_fixtures.py`

**Interfaces:** Adds an import of `legacy_event_to_recovery_node` from
`overlays.development.compat`; no function signature changes.

**Depends on:** T3 (final graph topology), T4 (new proof types declared), T6 (bundle-lane event
map), T7 (`legacy_event_to_recovery_node` must exist to import)

**Steps:**
- [ ] In `test_fixture_reaches_expected_terminal_status_with_honest_expressibility_claims`,
      change `for node_id in graph.nodes: assert final_state.cursors[node_id].status ==
      fixture["expected_terminal_status"]` to iterate `final_state.cursors` instead (same shape
      fix as T8, applied here because `graph.nodes` now includes nodes no fixture's script
      reaches — this currently affects **all 8 fixtures**, not just new ones).
- [ ] Import `legacy_event_to_recovery_node` alongside the existing
      `legacy_event_to_proof_type` import.
      Extend the honesty-invariant assertion's condition to also accept
      `legacy_event_to_recovery_node(node_or_event) in node_ids` (in addition to the existing
      `node_or_event in node_ids`, `node_or_event in proof_types`, and
      `legacy_event_to_proof_type(node_or_event) in proof_types` checks) — this is what lets
      `NEEDS_CONTEXT`/`CONCERN_TRIAGED`/`TASK_REPAIR_DONE` legitimately flip to
      `expressible_in_overlay: true` in fixtures 07/08 (T17/T18) without weakening the check
      for anything else. Update the assertion's failure message to mention this third avenue.
- [ ] Update the module docstring's description of the honesty invariant (currently says
      "resolves to one of those proof types") to also mention the parallel node-mapping avenue.
      Do **not** introduce any of the forbidden phrases
      `test_module_docstring_documents_invariants_not_pipeline_process` checks for
      (`repair-findings.md`, `repair-cycle`, `repair cycle`, `b12-issue13`).
- [ ] Run `pytest tests/test_parity_fixtures.py -v` — expect fixture-content-dependent
      assertions (the honesty check per fixture, and `test_state_event_migration_doc_lists_
      every_legacy_expected_entry`) to still fail until T11–T19 land; that's expected. The
      iteration-shape fix itself and the two schema/docstring tests should pass immediately.

---

### T10 — `docs/overlays/development.md`: describe the widened graph shape

**Files:** `docs/overlays/development.md`

**Depends on:** T3, T4, T6, T7 (needs the final, settled topology and mapping names to describe
accurately)

**Steps:**
- [ ] Under "## Graph", add a subsection (or extend the existing one) describing the bundle
      lane (`plan_bundle -> task_scheduler -> bundle_verify -> final_review ->
      documentation_review -> create_pr`, with `repair_bundle` as the retry branch off
      `bundle_verify`/`final_review`) and the recovery lane (`context_recovery`,
      `blocker_recovery`, `awaiting_human` — topology-only, no edges except
      `repair_bundle -> awaiting_human`).
- [ ] State plainly, in the same disclosure register as the existing `conflict_fn` wiring-gap
      paragraph (`docs/overlays/development.md:62-70`), that the recovery/retry edges are
      topology-only: `TransitionEngine._advance_successors` only fires on `TERMINAL_SUCCESS`
      with no conditional/failure-triggered semantics (filed separately as #32), so these edges
      do not yet express "retry on failure" — only that the node/edge *names* are expressible.
- [ ] Add a short note (per clarification 7) that `build_development_graph()` bypasses
      `load_graph()`'s reachability check entirely (hand-constructs `Graph(...)`), so the new
      nodes being unreachable from `entry_node="write_tdd"` is legal today but would fail
      closed if this function were ever refactored to load from a JSON document instead — out
      of scope for this bundle, just a heads-up for a future reader.
- [ ] Update "## Manifest" to list the four new proof types from T4.
- [ ] Update "## Graders" to list the four new graders from T5.
- [ ] Do not rewrite the "core-boundary rule" preamble or the "conflict_fn wiring gap" section
      — extend around them.

---

### T11 — Refresh fixture `01-simple-bug-fix.json` expressibility

**Files:** `benchmark/fixtures/01-simple-bug-fix.json`

**Depends on:** T3, T4, T6, T7

**Steps:**
- [ ] Flip `expressible_in_overlay` to `true` for every `legacy_expected` entry whose
      `node_or_event` is one of: `plan_bundle`, `task_scheduler`, `bundle_verify`,
      `final_review`, `documentation_review`, `create_pr`, `PLAN_DONE`, `BUNDLE_VERIFY_PASSED`,
      `DOC_REVIEW_DONE`, `PR_CREATED`, `BRANCH_READY` (this fixture's actual entry list — check
      against the file itself, not this plan's summary, since not every fixture lists every
      name).
- [ ] Leave `false` for anything not covered above (e.g. `bundle_scheduler`, `repair_task`,
      `human_required`, `TASK_STARTED`, `TDD_DONE`, `IMPLEMENT_DONE`, `TASK_COMMITTED`,
      `BUNDLE_TASKS_COMPLETE`) if present in this fixture.
- [ ] Do not add or remove `legacy_expected` entries, and do not touch `praxis_script` or
      `expected_terminal_status` — only `expressible_in_overlay` values change.
- [ ] Validate with `python3 -c "import json; json.load(open('benchmark/fixtures/01-simple-bug-fix.json'))"`.

---

### T12 — Refresh fixture `02-feature-implementation.json` expressibility

**Files:** `benchmark/fixtures/02-feature-implementation.json`

**Depends on:** T3, T4, T6, T7

**Steps:** Same recipe as T11, applied to this fixture's own `legacy_expected` entries (per the
file's actual list: it includes `plan_bundle`, `task_scheduler`, `bundle_verify`,
`final_review`, `documentation_review`, `create_pr`, `bundle_scheduler`, `PLAN_DONE`,
`TDD_DONE`, `IMPLEMENT_DONE`, `TASK_COMMITTED`, `BUNDLE_TASKS_COMPLETE`,
`BUNDLE_VERIFY_PASSED`, `DOC_REVIEW_DONE`, `PR_CREATED`, `BRANCH_READY` — flip only the ones
this plan's design summary marks expressible; `bundle_scheduler`/`TDD_DONE`/`IMPLEMENT_DONE`/
`TASK_COMMITTED`/`BUNDLE_TASKS_COMPLETE` stay `false`).

---

### T13 — Refresh fixture `03-multi-file-change.json` expressibility

**Files:** `benchmark/fixtures/03-multi-file-change.json`

**Depends on:** T3, T4, T6, T7

**Steps:** Same recipe as T11. This fixture's list includes `blocker_recovery`,
`context_recovery`, `awaiting_human`, `human_required`, `create_pr`, `PLAN_DONE`, `TDD_DONE`,
`IMPLEMENT_DONE`, `VERIFY_DONE` (already `true`), `TASK_COMMITTED`, `FOOTPRINT_VIOLATION`,
`PR_CREATED`, `BRANCH_READY` — flip `blocker_recovery`, `context_recovery`, `awaiting_human`,
`create_pr`, `PLAN_DONE`, `PR_CREATED`, `BRANCH_READY` to `true`; `human_required`, `TDD_DONE`,
`IMPLEMENT_DONE`, `TASK_COMMITTED`, `FOOTPRINT_VIOLATION` stay `false`.

---

### T14 — Refresh fixture `04-security-remediation.json` expressibility

**Files:** `benchmark/fixtures/04-security-remediation.json`

**Depends on:** T3, T4, T6, T7

**Steps:** Same recipe as T11. Includes `repair_task`, `repair_bundle`, `context_recovery`,
`awaiting_human`, `human_required`, `BUNDLE_VERIFY_FAILED`, `REVIEW_FINDINGS` alongside the
usual bundle-lane set — flip `repair_bundle`, `context_recovery`, `awaiting_human`, plus the
usual bundle-lane nodes/events (`plan_bundle`, `task_scheduler`, `bundle_verify`,
`final_review`, `documentation_review`, `create_pr`, `PLAN_DONE`, `BUNDLE_VERIFY_PASSED`,
`DOC_REVIEW_DONE`, `PR_CREATED`, `BRANCH_READY`) to `true`; `repair_task`, `human_required`,
`TASK_STARTED`, `TDD_DONE`, `IMPLEMENT_DONE`, `TASK_COMMITTED`, `BUNDLE_TASKS_COMPLETE`,
`BUNDLE_VERIFY_FAILED`, `REVIEW_FINDINGS` stay `false`. This is also the fixture
`test_evidence_gate_fails_closed_on_fixtures_failing_test_pass_proof` re-uses verbatim (via
`copy.deepcopy`) — do not change `praxis_script`.

---

### T15 — Refresh fixture `05-iac-change.json` expressibility

**Files:** `benchmark/fixtures/05-iac-change.json`

**Depends on:** T3, T4, T6, T7

**Steps:** Same recipe as T11. Includes `repair_task` (stays `false`) alongside the usual
bundle-lane set (flip `plan_bundle`, `task_scheduler`, `bundle_verify`, `final_review`,
`documentation_review`, `create_pr`, `PLAN_DONE`, `BUNDLE_VERIFY_PASSED`, `DOC_REVIEW_DONE`,
`PR_CREATED`, `BRANCH_READY` to `true`).

---

### T16 — Refresh fixture `06-dependency-upgrade.json` expressibility

**Files:** `benchmark/fixtures/06-dependency-upgrade.json`

**Depends on:** T3, T4, T6, T7

**Steps:** Same recipe as T11. Includes `bundle_scheduler` and `FOOTPRINT_VIOLATION` (both stay
`false`) alongside the usual bundle-lane set (flip the same seven node ids / five events as
T11/T15 to `true`).

---

### T17 — Refresh fixture `07-ambiguous-recovery.json` expressibility + notes

**Files:** `benchmark/fixtures/07-ambiguous-recovery.json`

**Depends on:** T3, T4, T6, T7

**Steps:**
- [ ] Flip `context_recovery` and `awaiting_human` to `true` (both are now real
      `build_development_graph()` node ids). Flip `NEEDS_CONTEXT` to `true` (now resolves via
      `legacy_event_to_recovery_node` to `context_recovery`, per T7/T9).
- [ ] Leave `plan_bundle`, `task_scheduler`, `human_required` as `false` (bundle-lane nodes not
      in this fixture's happy-path script area / `human_required` explicitly excluded).
- [ ] **Revise the top-level `notes` entry.** Its current text claims "this scenario's defining
      recovery path has no representation in the overlay at any point past `implement`, because
      the overlay has neither a recovery lane nor a human-interrupt node" — this is no longer
      true (the recovery-lane nodes now exist). Rewrite to state the more precise, still-honest
      claim: `context_recovery`/`awaiting_human` are now structurally present
      (`expressible_in_overlay: true` for the node/event *names*), but the `verify`/`commit_task`
      terminal-success path this fixture's `praxis_script` actually drives is still a structural
      artifact of `FakeExecutor` requiring every scripted node to reach a terminal status — the
      recovery path itself is not functionally reachable via a real failure-triggered
      transition (cite #32), because no edge from `implement`'s failure into `context_recovery`
      exists or could exist without that core-runtime gap.
- [ ] This fixture's note text is checked verbatim by
      `test_state_event_migration_doc_reflects_fixture_notes` — coordinate the exact wording
      with T19 (same note must be quoted verbatim in `docs/parity/state-event-migration.md`'s
      `07-ambiguous-recovery` section) or finalize this file first and have T19 quote it exactly.

---

### T18 — Refresh fixture `08-repair-heavy.json` expressibility + notes

**Files:** `benchmark/fixtures/08-repair-heavy.json`

**Depends on:** T3, T4, T6, T7

**Steps:**
- [ ] Flip to `true`: `plan_bundle`, `task_scheduler`, `bundle_verify`, `final_review`,
      `documentation_review`, `create_pr`, `repair_bundle`, `awaiting_human`, `PLAN_DONE`,
      `BUNDLE_VERIFY_PASSED`, `DOC_REVIEW_DONE`, `PR_CREATED`, `BRANCH_READY`, `CONCERN_TRIAGED`
      (resolves via `legacy_event_to_recovery_node` to `repair_bundle`), `TASK_REPAIR_DONE`
      (resolves to `verify`).
- [ ] Leave `false`: `repair_task`, `human_required`, `bundle_scheduler`, `TASK_STARTED`,
      `TDD_DONE`, `IMPLEMENT_DONE`, `TASK_COMMITTED`, `BUNDLE_TASKS_COMPLETE`,
      `BUNDLE_VERIFY_FAILED`, `REVIEW_FINDINGS`.
- [ ] **Revise the top-level `notes` entry** the same way as T17: the current claim ("the
      overlay has no repair lane -- there is no node to script a fail-then-repair_task-then-
      retry cycle against") needs the same "structurally present, not yet functionally
      reachable, see #32" nuance — `repair_bundle` now exists as a node, but there is still no
      real failure-triggered edge into it from `verify`/`bundle_verify`, and `repair_task`
      itself was never added. Coordinate exact wording with T19 as in T17.

---

### T19 — Rewrite `docs/parity/state-event-migration.md` for the widened overlay

**Files:** `docs/parity/state-event-migration.md`

**Depends on:** T3, T4, T6, T7

**Steps:**
- [ ] Update the "## What is expressible" table to add rows for every newly-expressible node id
      (`plan_bundle`, `task_scheduler`, `bundle_verify`, `final_review`, `documentation_review`,
      `create_pr`, `repair_bundle`, `context_recovery`, `blocker_recovery`, `awaiting_human`)
      and event (`PLAN_DONE`, `BUNDLE_VERIFY_PASSED`, `DOC_REVIEW_DONE`, `PR_CREATED`/
      `BRANCH_READY`, `NEEDS_CONTEXT`, `CONCERN_TRIAGED`, `TASK_REPAIR_DONE`), citing whether
      each resolves via node-id membership, `legacy_event_to_proof_type`, or the new
      `legacy_event_to_recovery_node`.
- [ ] Rewrite "## Why the rest isn't expressible": Category B shrinks to just `bundle_scheduler`
      (orchestrator lane, still excluded). Category R's node list shrinks to just `repair_task`
      (still excluded) — but add a new paragraph explaining that `repair_bundle`,
      `context_recovery`, `blocker_recovery`, `awaiting_human` have moved from "no counterpart"
      to "structurally present (`expressible_in_overlay: true`) but not yet functionally
      reachable via a real failure-triggered transition" — cite #32 explicitly, same disclosure
      register as the existing `conflict_fn`/performance-parity caveats elsewhere in this
      document set. Category H (`human_required`/`NEEDS_CONTEXT`'s *node* absence) — note that
      `NEEDS_CONTEXT` itself is now expressible (via `context_recovery`), but `human_required`
      the node is still not.
- [ ] For every one of the 8 per-scenario tables (`### 01-simple-bug-fix` through
      `### 08-repair-heavy`), flip the "Expressible"/"Category" columns to match each fixture's
      final `legacy_expected` array from T11–T18 exactly — this document's own claim ("each
      table lists every `legacy_expected` entry") is re-checked live by
      `test_state_event_migration_doc_lists_every_legacy_expected_entry`, so entries must match
      1:1, in the fixture's own order, with the real JSON (do not hand-derive these; read each
      finished fixture file).
- [ ] Update the `07-ambiguous-recovery` and `08-repair-heavy` "Fixture notes" blockquotes to
      quote T17/T18's revised `notes` text verbatim — `test_state_event_migration_doc_reflects_
      fixture_notes` checks this exactly.
- [ ] Update "## This is an accepted contract migration, not silent normalization" to reflect
      the new, smaller residual gap (only `bundle_scheduler`, `repair_task`, `human_required`,
      and the bookkeeping-only events without a proof/node mapping remain unexpressed) and the
      "structurally present, not functionally reachable, see #32" framing for the recovery
      lane.
- [ ] This task necessarily runs after T11–T18 conceptually (it must transcribe their final
      JSON) even though it has no file-footprint overlap with them — read the finished fixture
      files rather than re-deriving values from this plan's per-fixture summaries above, in
      case of any discrepancy the fixture JSON is authoritative.

---

### T20 — Append gap-closure addendum to `docs/parity/decision.md`

**Files:** `docs/parity/decision.md`

**Depends on:** T3, T4, T6, T7

**Steps:**
- [ ] Append a new, separately-headed section after the existing content (do not edit
      `## T3`/`## T4`/`## T6`/`## T7`): `## Addendum (issue #30): widened-overlay gap closure`.
- [ ] State which of categories B/R/H (per `docs/parity/state-event-migration.md`'s taxonomy,
      as rewritten by T19) are now closed vs. still open: category B is closed except
      `bundle_scheduler`; category R's node/event *names* are now structurally present but the
      spec's own "Decided approach" means they are **not** closed in the functional sense —
      state plainly "structurally present, not yet functionally reachable via a real failure
      transition, see #32" for `repair_bundle`/`context_recovery`/`blocker_recovery`/
      `awaiting_human`/`CONCERN_TRIAGED`/`TASK_REPAIR_DONE`; category H's `awaiting_human` slice
      is covered by the same caveat, while `human_required`/the node itself remains fully open.
- [ ] Explicitly restate that performance parity (`## T6`/`## T7` above) remains open regardless
      of this work — do not let the addendum imply otherwise.
- [ ] Cross-reference `docs/parity/state-event-migration.md` (T19) and
      `docs/overlays/development.md` (T10) rather than duplicating their tables.

## Critical path

`T1 -> T3 -> T8` (length 3 of ceiling 10; `T1 -> T3 -> T9`, and the `T4 -> T5` chain, are the
same length or shorter). Planning this bundle stayed on the single serial path; all 20
implementation tasks below it can run with real, disjoint-footprint concurrency once their
`depends_on` are satisfied.
