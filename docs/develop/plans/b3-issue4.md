# Plan: b3-issue4 — Generic graph, run-state, event, checkpoint, and transition engine

Spec: [`docs/develop/specs/b3-issue4.md`](../specs/b3-issue4.md)
Issue: #4 (depends on #2, merged)

## Architecture decisions (binding on all tasks)

- **New package `src/praxis_runtime/`**, a sibling of the existing `src/praxis_contracts/`
  package. `pyproject.toml`'s `[tool.setuptools.packages.find] where = ["src"]` has no
  `include`/`exclude` filter, so the new package is auto-discovered — **no task may edit
  `pyproject.toml`**.
- **Reuse `praxis_contracts.validator.validate_document`** (and its
  `ContractValidationError`) for every new document type this bundle introduces (graph
  definitions, events, run-state/checkpoints). Do not fork or reimplement the
  spec-version-then-schema validation pattern; import it from `praxis_contracts.validator`.
  This is what "consistent with #2's contracts" means concretely.
- **New JSON Schemas live under `schemas/v1/`**, next to the Promise/Capability ontology
  schemas, using the same `spec_version` convention (`^1\.\d+\.\d+$`) documented in
  `docs/ontology.md`. Do not edit the existing ontology schema files.
- **Stay domain-neutral.** No module, schema field, docstring, or test fixture may reference
  software-development concepts (PRs, TDD, GitHub issues, branches, code review) or a model/
  vendor name. Node/event "kind" vocabularies are open, free-form strings, mirroring
  `Promise.kind`.
- **Atomicity for run-state:** write to a temp file in the same directory and `os.replace()`
  it into place (POSIX atomic rename) so a crash mid-write never leaves a torn state file.
- **Append-only for events:** each event is one JSON line appended to an events file opened
  in append mode, flushed and `fsync`ed before the call returns; each event carries a
  monotonically increasing sequence number assigned by the event log itself (never by the
  caller) so duplicate-submission and ordering are both checkable from the log alone.
- **The transition engine is the single authority on legality.** No other module (fake
  executor, replay, migrations) may mutate run state or append events directly — they all go
  through `praxis_runtime.transitions`. This is what makes "domain overlays cannot bypass core
  transition legality" true by construction.

## Public interface surface (stabilize for issues #5, #6, #7)

Per the tech lead's note, sibling bundles will import from this module soon. Keep these names
stable once introduced:
- `praxis_runtime.graph`: `Graph`, `Node`, `Edge`, `load_graph(path) -> Graph`,
  `GraphValidationError`.
- `praxis_runtime.events`: `Event`, `EventLog`, `EventLogError`.
- `praxis_runtime.state`: `RunState`, `RunStateStore`, `Cursor`, `RunStateError`.
- `praxis_runtime.transitions`: `TransitionEngine`, `TransitionError`, `NodeStatus` (enum
  including terminal/blocked/handoff/recovery states).
- `praxis_runtime.replay`: `replay(event_log, graph) -> RunState`.
- `praxis_runtime.migrations`: `migrate_document(doc, kind) -> dict`.
- `praxis_runtime.testing.fake_executor`: `FakeExecutor`.

## Task graph

```
T0 (bootstrap)
 ├─> T1 (graph)     ─┐
 ├─> T2 (events)     ├─> T4 (transitions) ─┬─> T5 (replay) ─┬─> T9 (crash/restart tests)
 └─> T3 (state)     ─┘        │            └─> T7 (fake executor) ─┴─> T8 (e2e sample-graph test)
                               │                                   └─> T9
       T2,T3 ────────────────>T6 (migrations)
       T1,T2,T3,T4 ─────────────────────────────────────────────────> T10 (fail-closed suite)
       T1..T7 ──────────────────────────────────────────────────────> T11 (docs)
```

### T0 — Bootstrap: create the `praxis_runtime` package

**Files:** `src/praxis_runtime/__init__.py`

**Interfaces:** `__version__ = "0.1.0"` (module-level constant only; no other exports — later
tasks import from submodules directly, e.g. `from praxis_runtime.graph import Graph`, so this
file never needs editing again).

**Depends on:** (none)

**Steps:**
- [ ] Create `src/praxis_runtime/__init__.py` containing only `__version__ = "0.1.0"`,
      matching the style of `src/praxis_contracts/__init__.py`.
- [ ] Run `pip install -e ".[dev]"` from the repo root to confirm the package is picked up
      by `setuptools.packages.find` (import `praxis_runtime` in a throwaway
      `python3 -c "import praxis_runtime"` — do not commit a throwaway file).

---

### T1 — Graph loader and validator

**Files:** `schemas/v1/graph.schema.json`, `src/praxis_runtime/graph.py`,
`tests/test_graph_loader.py`

**Interfaces:**
- `class Node`: `id: str`, `kind: str` (open vocabulary, same pattern as `Promise.kind`),
  `metadata: dict`.
- `class Edge`: `source: str`, `target: str`, `kind: str` (e.g. `"sequential"`, `"fan-out"`,
  `"join"` — open vocabulary, not an enum, so overlays can add edge kinds).
- `class Graph`: `spec_version: str`, `nodes: dict[str, Node]`, `edges: list[Edge]`,
  `entry_node: str`, `terminal_nodes: set[str]`.
- `def load_graph(path: Path) -> Graph`: reads JSON, validates via
  `praxis_contracts.validator.validate_document` against `graph.schema.json`, then checks
  graph-level invariants beyond JSON Schema's reach (edges reference existing node ids,
  `entry_node` exists, every node reachable from `entry_node`, no dangling terminal
  references) — raise `GraphValidationError` (fail closed) on any violation, wrapping
  `ContractValidationError` where applicable.
- `class GraphValidationError(Exception)`.

**Depends on:** T0

**Steps:**
- [ ] Write `schemas/v1/graph.schema.json` (draft 2020-12, `$id` under
      `https://schemas.praxis.dev/v1/graph.schema.json` matching sibling schemas' `$id`
      style): top-level `spec_version`, `nodes` (array of `{id, kind, metadata?}`, `kind`
      matching `^[a-z0-9]+(-[a-z0-9]+)*$`), `edges` (array of `{source, target, kind}`),
      `entry_node`, `terminal_nodes` (array). `additionalProperties: false` throughout,
      mirroring `schemas/v1/promise.schema.json`'s strictness.
- [ ] Implement `src/praxis_runtime/graph.py` per the interfaces above. Call
      `praxis_contracts.validator.validate_document(instance, schema_path)` for shape/version
      validation before doing graph-level structural checks.
- [ ] Write `tests/test_graph_loader.py`: a well-formed graph loads; a graph with a dangling
      edge reference fails closed with `GraphValidationError`; a graph with an unreachable
      node fails closed; version-mismatch `spec_version` surfaces the distinct "version
      mismatch" message from `validate_document` (mirror the assertion style in
      `tests/test_version_mismatch.py`).

---

### T2 — Append-only event log

**Files:** `schemas/v1/event.schema.json`, `src/praxis_runtime/events.py`,
`tests/test_event_log.py`

**Interfaces:**
- `class Event`: `spec_version: str`, `seq: int`, `run_id: str`, `node_id: str`,
  `event_type: str` (open vocabulary — e.g. `"transition-attempted"`,
  `"transition-committed"`, `"transition-rejected"`; versioned via `spec_version`, not the
  string itself), `payload: dict`, `event_id: str` (caller-supplied idempotency key).
- `class EventLog`: constructed with a directory path.
  - `def append(self, event: Event) -> Event`: assigns `seq` itself (ignores/overwrites any
    caller-supplied `seq`), rejects (raises `EventLogError`, fail closed) an `event_id` that
    already exists in the log — this is the duplicate-event guard — writes one JSON line,
    flush + `os.fsync`.
  - `def read_all(self) -> list[Event]`: replays the file in order.
- `class EventLogError(Exception)`.

**Depends on:** T0

**Steps:**
- [ ] Write `schemas/v1/event.schema.json`: `spec_version`, `seq` (integer, >=0), `run_id`,
      `node_id`, `event_type` (`^[a-z0-9]+(-[a-z0-9]+)*$`), `event_id`, `payload` (open
      object). `additionalProperties: false`.
- [ ] Implement `src/praxis_runtime/events.py`. Validate each event against
      `event.schema.json` via `validate_document` before writing. Use append-mode file
      handle, one JSON object per line (JSONL), flush + `os.fsync` per write so a crash
      immediately after `append()` returns is guaranteed durable.
- [ ] Write `tests/test_event_log.py`: sequential appends get increasing `seq`; appending a
      duplicate `event_id` raises `EventLogError` and does not write a second line; `read_all`
      after re-opening a fresh `EventLog` on the same directory reconstructs the same ordered
      list (this is the "restarted process resumes from persisted events" property, tested at
      the event-log layer).

---

### T3 — Run-state store and checkpoint model

**Files:** `schemas/v1/run-state.schema.json`, `src/praxis_runtime/state.py`,
`tests/test_state_store.py`

**Interfaces:**
- `class Cursor`: `node_id: str`, `status: str` (mirrors `NodeStatus` values introduced in
  T4 — define as a plain string here since T3 has no dependency on T4; T4 imports and
  constrains this field, it does not redefine it).
- `class RunState`: `spec_version: str`, `run_id: str`, `cursors: dict[str, Cursor]`,
  `last_applied_seq: int` (the event-log `seq` this state reflects — the join point with T2's
  log, consumed by replay in T5).
- `class RunStateStore`: constructed with a file path.
  - `def load(self) -> RunState | None`: returns `None` if no checkpoint exists yet.
  - `def save(self, state: RunState) -> None`: validates against `run-state.schema.json` via
    `validate_document`, writes to `<path>.tmp`, `os.replace()`s over the target — this is
    the atomic-write guarantee.
- `class RunStateError(Exception)`.

**Depends on:** T0

**Steps:**
- [ ] Write `schemas/v1/run-state.schema.json`: `spec_version`, `run_id`, `cursors` (object
      keyed by node id, values `{node_id, status}`), `last_applied_seq` (integer, >=0).
      `additionalProperties: false`.
- [ ] Implement `src/praxis_runtime/state.py` per the interfaces above, including the
      temp-file + `os.replace()` atomic-write sequence.
- [ ] Write `tests/test_state_store.py`: `load()` on a missing file returns `None`; `save()`
      then `load()` round-trips; a `RunStateStore` pointed at a path with a stale/truncated
      temp file left over from a simulated interrupted write (write garbage to `<path>.tmp`,
      not `<path>`) still `load()`s the last good `<path>` uncorrupted — this proves atomicity
      of the interrupted-write acceptance criterion at the store layer.

---

### T4 — Deterministic transition engine

**Files:** `src/praxis_runtime/transitions.py`, `tests/test_transitions.py`

**Interfaces:**
- `class NodeStatus(enum.Enum)`: `PENDING`, `RUNNING`, `BLOCKED`, `HANDOFF`, `RECOVERING`,
  `TERMINAL_SUCCESS`, `TERMINAL_FAILED` (satisfies "terminal, blocked, handoff, and recovery
  states").
- `class TransitionError(Exception)` — raised fail-closed for: illegal transition per the
  graph's edges, missing required evidence, applying a transition to a node not in a state
  that permits it.
- `class TransitionEngine`:
  - `__init__(self, graph: Graph, state_store: RunStateStore, event_log: EventLog)`.
  - `def current_state(self) -> RunState`: loads-or-initializes from `state_store`.
  - `def apply(self, node_id: str, event_type: str, *, evidence: dict | None = None) -> RunState`:
    the single mutation entrypoint. Sequence: (1) load current `RunState`; (2) check the
    requested transition is legal given the graph's edges and the node's current
    `NodeStatus` — raise `TransitionError` if not; (3) if the target node kind declares
    required evidence (a node's `metadata` may carry an evidence-requirement-shaped block —
    reuse the `required`/`preferred`/`prohibited` vocabulary from
    `schemas/v1/evidence-requirement.schema.json` conceptually, but do not import an
    evidence-grading dependency — issue #6 owns grading; this engine only checks that
    required evidence *keys* are present in `evidence`) and it's missing, raise
    `TransitionError`; (4) append an `Event` via `event_log.append`; (5) compute the new
    `RunState` (update the node's cursor, and for fan-out edges create cursors for every
    fanned-out target, for join edges only advance once every incoming cursor reports
    terminal-success) and persist via `state_store.save`; (6) return the new `RunState`.
  - `def legal_next(self, node_id: str) -> set[str]`: pure query, no mutation — the graph
    edges reachable from `node_id`'s current status, for callers (fake executor, dashboards)
    to introspect without risking a bypass.

**Depends on:** T1, T2, T3

**Steps:**
- [ ] Implement `NodeStatus`, `TransitionError`, `TransitionEngine` per the interfaces above
      in `src/praxis_runtime/transitions.py`, importing `Graph`/`Node`/`Edge` from
      `praxis_runtime.graph`, `Event`/`EventLog` from `praxis_runtime.events`,
      `RunState`/`Cursor`/`RunStateStore` from `praxis_runtime.state`.
- [ ] Implement fan-out (one node's completion creates multiple concurrent cursors) and join
      (a node with multiple incoming edges advances only once all incoming cursors are
      `TERMINAL_SUCCESS`) as the two edge-kind behaviors from T1's `Edge.kind`.
- [ ] Write `tests/test_transitions.py` covering: a legal transition applies and persists; an
      illegal transition (edge not present in the graph, or node not in a status that
      permits the requested transition) raises `TransitionError` and leaves state/event log
      unchanged (fail-closed, no partial write); a transition requiring evidence with
      `evidence=None` or a missing key raises `TransitionError`; a fan-out node produces the
      expected set of new cursors; a join node only advances after its last incoming cursor
      completes, not before.

---

### T5 — Resume/replay support

**Files:** `src/praxis_runtime/replay.py`, `tests/test_checkpoint_resume.py`

**Interfaces:**
- `def replay(event_log: EventLog, graph: Graph) -> RunState`: reconstructs a `RunState`
  purely from `event_log.read_all()` and the graph's transition rules, independent of any
  checkpoint file — this is the "event replay reconstructs the same observable run state"
  acceptance criterion, proven by comparing its output against the checkpointed `RunState` in
  tests.
- `def resume(graph: Graph, state_store: RunStateStore, event_log: EventLog) -> TransitionEngine`:
  the process-restart entrypoint — loads the last checkpoint if present, replays any events
  with `seq > last_applied_seq` on top of it (covers a crash between event-append and
  state-save), and returns a ready `TransitionEngine`.

**Depends on:** T4

**Steps:**
- [ ] Implement `replay()` by folding `TransitionEngine`'s pure transition-computation logic
      over the ordered event list from `seq=0`, without touching the real `state_store` (use
      an in-memory `RunState` accumulator) — refactor the state-computation step out of
      `TransitionEngine.apply` into a private pure function if needed so both `apply` and
      `replay` share it (do not duplicate the fan-out/join logic).
- [ ] Implement `resume()` using `replay()` restricted to events after `last_applied_seq`.
- [ ] Write `tests/test_checkpoint_resume.py`: apply a sequence of transitions through a
      `TransitionEngine`, then call `replay()` on the same event log from scratch and assert
      the resulting `RunState` equals the engine's final state; simulate a crash between event
      append and checkpoint save (append an event directly via `EventLog`, do not update the
      checkpoint file) and assert `resume()` produces the state that reflects that event.

---

### T6 — Schema-version migration strategy

**Files:** `src/praxis_runtime/migrations.py`, `tests/test_migrations.py`

**Interfaces:**
- `MIGRATIONS: dict[str, dict[tuple[int, int], Callable[[dict], dict]]]` — registry keyed by
  document kind (`"event"`, `"run-state"`) then by `(from_minor, to_minor)` within the same
  major version (major-version jumps are out of scope — a new `schemas/v2/` directory per
  `docs/ontology.md` is a separate concern from this per-instance migration path).
- `def migrate_document(doc: dict, kind: str) -> dict`: parses `doc["spec_version"]`, applies
  every registered migration in order up to the current minor version for `kind`, returns the
  migrated dict unchanged if already current. Raises `praxis_contracts.validator.ContractValidationError`
  (fail closed, reusing the existing error type rather than inventing a parallel one) if
  `spec_version`'s major version doesn't match what this runtime supports.

**Depends on:** T2, T3

**Steps:**
- [ ] Implement `src/praxis_runtime/migrations.py` with the registry and `migrate_document`
      as above. Seed it with a real (not hypothetical) migration: none is needed yet since
      `event.schema.json`/`run-state.schema.json` start at `1.0.0`, so register the identity
      case and document, in a module docstring, exactly how a future contributor adds a
      `(1, 0) -> (1, 1)` entry when a field is added.
- [ ] Write `tests/test_migrations.py`: a `1.0.x` event/run-state document round-trips through
      `migrate_document` unchanged; a document with a major-version mismatch (`2.0.0`) raises
      `ContractValidationError` with the same "version mismatch"-shaped reason used elsewhere
      in the codebase, not a new error type.

---

### T7 — Deterministic fake-executor test harness

**Files:** `src/praxis_runtime/testing/__init__.py`,
`src/praxis_runtime/testing/fake_executor.py`

**Interfaces:**
- `class FakeExecutor`:
  - `__init__(self, engine: TransitionEngine, script: dict[str, dict])`: `script` maps
    `node_id -> {"event_type": str, "evidence": dict | None}`, a fully predetermined,
    deterministic outcome per node (no randomness, no wall-clock, no model call — this is
    what makes it suitable as the "deterministic fake-executor test harness" deliverable).
  - `def run_to_completion(self, *, max_steps: int = 1000) -> RunState`: repeatedly calls
    `engine.legal_next(...)` for every non-terminal cursor, applies the scripted transition
    for each via `engine.apply(...)`, until every cursor is terminal or `max_steps` is
    exhausted (raise `TransitionError` on exhaustion — fail closed rather than looping
    forever on a malformed script/graph pairing).

**Depends on:** T4

**Steps:**
- [ ] Create `src/praxis_runtime/testing/__init__.py` (empty, just makes the subpackage
      importable).
- [ ] Implement `FakeExecutor` in `src/praxis_runtime/testing/fake_executor.py` per the
      interface above, driving only through `TransitionEngine`'s public `legal_next`/`apply`
      — never touching `RunStateStore`/`EventLog` directly, so it cannot bypass transition
      legality (the "domain overlays cannot bypass core transition legality" property,
      exercised at the harness level).
- [ ] Add a minimal inline unit test in a new `tests/test_fake_executor.py` covering a
      3-node linear graph (built inline in the test, not from a fixture file) driven to
      completion, and one where the script requests an illegal transition and the harness
      surfaces `TransitionError` rather than swallowing it.

---

### T8 — End-to-end sample-graph acceptance test

**Files:** `examples/sample-graph.json`, `tests/test_end_to_end_fake_executor.py`

**Interfaces:** none new — consumes `load_graph`, `TransitionEngine`, `FakeExecutor`.

**Depends on:** T1, T7

**Steps:**
- [ ] Write `examples/sample-graph.json`: a non-development sample graph valid against
      `schemas/v1/graph.schema.json` — use a generic domain (e.g. a document-review pipeline:
      `intake -> review -> (revise | approve) -> archive`) with at least one fan-out and one
      join, to exercise both edge kinds end to end. No software-development vocabulary
      (issues/PRs/branches/TDD).
- [ ] Write `tests/test_end_to_end_fake_executor.py`: `load_graph()` the sample, build a
      `TransitionEngine` against a temp dir's `RunStateStore`/`EventLog`, script a
      `FakeExecutor` to drive it through every node to a terminal state, assert the final
      `RunState` has every cursor terminal and the event log's `read_all()` count matches the
      number of transitions applied — this is the "non-development sample graph can run end
      to end with deterministic fake executors" acceptance criterion.

---

### T9 — Crash/restart resume at every transition boundary

**Files:** `tests/test_crash_restart.py`

**Interfaces:** none new — consumes `TransitionEngine`, `resume`, `FakeExecutor`.

**Depends on:** T5, T7

**Steps:**
- [ ] Build a small linear-plus-fan-out-plus-join graph inline in the test (a `Graph`
      constructed directly in Python, not loaded from a file, to keep this task's footprint
      independent of T8's example file).
- [ ] For every transition boundary in a full run (i.e., after each `engine.apply()` call),
      simulate a crash by discarding the in-memory `TransitionEngine`/`FakeExecutor` and
      constructing fresh ones via `resume(graph, state_store, event_log)` against the same
      on-disk state/event directories, then continue the script to completion. Assert the
      final `RunState` is identical to a control run of the same script with no simulated
      crashes — this is the "crash/restart at every transition boundary resumes correctly"
      acceptance criterion.

---

### T10 — Fail-closed edge-case suite

**Files:** `tests/test_fail_closed_cases.py`

**Interfaces:** none new — exercises `graph`, `events`, `state`, `transitions` failure paths.

**Depends on:** T1, T2, T3, T4

**Steps:**
- [ ] Malformed graph: a graph document missing `entry_node`, and one with a `nodes` entry
      whose `kind` violates the `^[a-z0-9]+(-[a-z0-9]+)*$` pattern, both raise
      `GraphValidationError`/`ContractValidationError` from `load_graph` (fail closed, no
      partial `Graph` object returned).
- [ ] Stale state: construct a `RunState` with `last_applied_seq` referencing a `seq` beyond
      what the event log actually contains, and assert the engine/replay path raises rather
      than silently truncating or wrapping.
- [ ] Duplicate events: two `EventLog.append()` calls with the same `event_id` — assert the
      second raises `EventLogError` and the log's `read_all()` length is unaffected (already
      covered narrowly in T2's own test; here it's exercised through `TransitionEngine.apply`
      to prove the engine itself doesn't double-apply on a caller retry).
- [ ] Illegal transitions: attempt a transition not present in the graph's edges from the
      current node status; attempt a transition on an already-terminal node; both raise
      `TransitionError`.
- [ ] Interrupted writes: reuse the `RunStateStore` interrupted-write scenario from T3's test
      (garbage left in `<path>.tmp`) but drive it through a full `TransitionEngine.apply()`
      call this time, asserting the engine's next `apply()` still reads the last good
      checkpoint rather than the torn temp file.

---

### T11 — Public interface documentation

**Files:** `docs/runtime.md`

**Interfaces:** none — documentation only.

**Depends on:** T1, T2, T3, T4, T5, T6, T7

**Steps:**
- [ ] Write `docs/runtime.md` documenting, for each module in `src/praxis_runtime/`
      (`graph`, `events`, `state`, `transitions`, `replay`, `migrations`, `testing`): its
      purpose, its public classes/functions (names, signatures, one line each — copy from
      the actual committed source, not from this plan, since implementers may have adjusted
      details during T1–T7), the atomicity/append-only guarantees from the architecture
      section above, and a short "how issues #5/#6/#7 are expected to depend on this" note
      (matching relationship, evidence grading, and resource scheduling respectively build on
      top of `TransitionEngine` and the `Graph`/`RunState`/`Event` shapes — they extend node
      `metadata`/`kind` vocabulary, they do not need new core interfaces).
- [ ] Cross-link `docs/runtime.md` from `docs/ontology.md`'s existing "Schema files" table
      area is out of scope for this task (that file is #2's, not #4's) — instead add a single
      "See also" line at the top of `docs/runtime.md` linking back to `docs/ontology.md` for
      the Promise/Capability vocabulary this runtime consumes.
