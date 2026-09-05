"""Regression tests for repair-findings.md (bundle b3-issue4).

Each test reproduces one finding before its fix and must pass after it:

1. `replay._fold_events` opens a scratch `EventLog` but never closes it (no
   `close()` call, no context-manager use), leaking the underlying file
   handle every time `replay()`/`resume()` folds pending events. Only masked
   today by CPython's refcounting GC closing the handle as an implementation
   detail.
2. `examples/sample-graph.json`'s "decision" node has two outgoing edges
   both labeled `kind: "sequential"`. `TransitionEngine._advance_successors`
   treats every non-join outgoing edge identically (all fire unconditionally
   once the source completes), so two "sequential" edges from one source
   behave exactly like fan-out, not like an exclusive branch -- the example
   doesn't actually model a decision despite the node's kind/id.
3. `_linear_graph()` was duplicated nearly identically across
   `tests/test_transitions.py`, `tests/test_fail_closed_cases.py`,
   `tests/test_checkpoint_resume.py`, and this file instead of living once in
   `tests/conftest.py` and being imported everywhere it's needed.
4. `src/praxis_runtime/state.py` computed its schema path as a module-private
   `_SCHEMA_PATH`, inconsistent with `graph.py`/`events.py`'s public
   `SCHEMA_PATH` for the identical purpose.
5. `Cursor`/`RunState` in `src/praxis_runtime/state.py` were plain (mutable)
   dataclasses while `Node`/`Edge`/`Graph`/`Event` are frozen, despite no
   in-place mutation of either anywhere in the codebase.
6. `TransitionEngine.apply`'s evidence-requirement check runs on transitions
   to `TERMINAL_FAILED` as well as `TERMINAL_SUCCESS`, but no test exercised
   the `TERMINAL_FAILED` path.
7. Five test-file module docstrings opened with the phrase 'RED-phase tests
   for ...' / 'RED-phase test for ...', which is TDD terminology and
   violates the Epic's domain-neutrality constraint (applies to
   docstrings/comments in the deliverable too, not just runtime schema
   vocabulary).
8. `examples/sample-graph.json`'s "decision" node fans out concurrently to
   both "revise" and "approve", but `revise -> archive` and
   `approve -> archive` are labeled `kind: "sequential"` rather than
   `"join"`. `TransitionEngine._advance_successors` creates a non-join
   target cursor as soon as any single incoming edge's source completes, so
   "archive" advances on whichever of "revise"/"approve" finishes first
   instead of waiting for both -- inconsistent with the process the example
   claims to model.
9. `src/praxis_runtime/events.py`'s `EventLog` assigns `seq` from an
   in-memory counter seeded once at construction, with no lock across
   instances -- two `EventLog` instances opened concurrently on the same
   directory can race and assign colliding `seq` numbers or lose track of
   an append made by the other instance.
10. `TransitionEngine.apply` in `src/praxis_runtime/transitions.py` checks
    `evidence` but never persists it onto the committed `Event`, leaving no
    durable audit trail of what evidence satisfied a gate.
11. `docs/runtime.md` is stale relative to three repair commits on
    `events.py`/`state.py`/`transitions.py`: it omits `EventLog.close()`/the
    context-manager protocol, `append()`'s `flock`-based concurrency
    serialization, `migrate_document`-on-read for both `EventLog` and
    `RunStateStore`, `TransitionEngine`'s checkpoint-ahead-of-log fail-closed
    check, and that `TransitionEngine.apply` now persists evidence onto the
    committed `Event` for a durable audit trail.
12. `EventLog.read_all()` returns a cache refreshed only by `append()` or
    `__init__`, so a long-lived instance that never appends does not observe
    events appended concurrently by another instance/process on the same
    directory -- inconsistent with the concurrency hardening just added to
    `append()`.
"""

from __future__ import annotations

import ast
import dataclasses
import json
from collections import defaultdict
from pathlib import Path

import pytest

from conftest import _linear_graph
from praxis_runtime import state as state_module
from praxis_runtime.events import Event, EventLog
from praxis_runtime.graph import Graph, Node
from praxis_runtime.replay import replay
from praxis_runtime.state import Cursor, RunState, RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

REPO_ROOT = Path(__file__).resolve().parent.parent
SAMPLE_GRAPH_PATH = REPO_ROOT / "examples" / "sample-graph.json"
RUNTIME_DOC_PATH = REPO_ROOT / "docs" / "runtime.md"


def test_replay_closes_its_scratch_event_log(monkeypatch, tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    engine.apply("n1", "start")
    engine.apply("n1", "complete")

    closed_handles = []
    original_close = EventLog.close

    def tracking_close(self):
        closed_handles.append(self)
        original_close(self)

    monkeypatch.setattr(EventLog, "close", tracking_close)

    replay(log, graph)

    assert closed_handles, (
        "replay()'s scratch EventLog (opened inside _fold_events) was never "
        "closed -- this leaks a file descriptor per replay()/resume() call "
        "with pending events"
    )


def test_sample_graph_does_not_mislabel_fan_out_as_sequential():
    document = json.loads(SAMPLE_GRAPH_PATH.read_text())

    outgoing_by_source = defaultdict(list)
    for edge in document["edges"]:
        outgoing_by_source[edge["source"]].append(edge)

    for source, edges in outgoing_by_source.items():
        if len(edges) <= 1:
            continue
        sequential_targets = [edge["target"] for edge in edges if edge["kind"] == "sequential"]
        assert not sequential_targets, (
            f"node {source!r} has {len(edges)} outgoing edges but labels "
            f"{sequential_targets} as 'sequential' -- TransitionEngine fires "
            "every non-join outgoing edge unconditionally once the source "
            "completes, so this behaves exactly like fan-out, not an "
            "exclusive branch, despite the label implying otherwise"
        )


def test_linear_graph_helper_is_shared_from_conftest():
    import conftest
    import test_checkpoint_resume
    import test_fail_closed_cases
    import test_transitions

    assert test_transitions._linear_graph is conftest._linear_graph
    assert test_fail_closed_cases._linear_graph is conftest._linear_graph
    assert test_checkpoint_resume._linear_graph is conftest._linear_graph


def test_state_module_exposes_public_schema_path():
    assert state_module.SCHEMA_PATH == (
        REPO_ROOT / "schemas" / "v1" / "run-state.schema.json"
    )


def test_cursor_and_run_state_are_frozen():
    cursor = Cursor(node_id="n1", status="pending")
    with pytest.raises(dataclasses.FrozenInstanceError):
        cursor.status = "running"

    state = RunState(spec_version="1.0.0", run_id="run-1", cursors={}, last_applied_seq=-1)
    with pytest.raises(dataclasses.FrozenInstanceError):
        state.last_applied_seq = 0


def test_evidence_required_enforced_on_transition_to_terminal_failed(tmp_path: Path):
    graph = Graph(
        spec_version="1.0.0",
        nodes={
            "n1": Node(
                id="n1",
                kind="gate",
                metadata={
                    "evidence_requirement": {
                        "spec_version": "1.0.0",
                        "evidence": [
                            {"proof_type": "incident-report", "constraint": "required"},
                        ],
                    }
                },
            ),
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("n1", "start")

    with pytest.raises(TransitionError):
        engine.apply("n1", "fail", evidence=None)

    state = engine.apply("n1", "fail", evidence={"incident-report": {"ref": "r1"}})
    assert state.cursors["n1"].status == NodeStatus.TERMINAL_FAILED.value


_BANNED_TDD_TERMS = ("red-phase", "red phase")

_DOCSTRING_TEST_FILES = (
    "test_transitions.py",
    "test_event_log.py",
    "test_checkpoint_resume.py",
    "test_crash_restart.py",
    "test_fake_executor.py",
)


@pytest.mark.parametrize("test_file", _DOCSTRING_TEST_FILES)
def test_test_module_docstrings_are_domain_neutral(test_file):
    source = (REPO_ROOT / "tests" / test_file).read_text()
    module_doc = ast.get_docstring(ast.parse(source))
    assert module_doc, f"{test_file} has no module docstring"

    lowered = module_doc.lower()
    for term in _BANNED_TDD_TERMS:
        assert term not in lowered, (
            f"{test_file}'s module docstring contains banned TDD term "
            f"{term!r} -- the Epic's domain-neutrality constraint applies "
            "to docstrings/comments in the deliverable, not just runtime "
            "schema vocabulary"
        )


def test_sample_graph_archive_requires_all_fanned_out_branches_to_join():
    document = json.loads(SAMPLE_GRAPH_PATH.read_text())

    incoming_by_target = defaultdict(list)
    for edge in document["edges"]:
        incoming_by_target[edge["target"]].append(edge)

    for target, incoming in incoming_by_target.items():
        if len(incoming) <= 1:
            continue
        non_join = [edge["source"] for edge in incoming if edge["kind"] != "join"]
        assert not non_join, (
            f"node {target!r} has {len(incoming)} incoming edges but "
            f"{non_join} are not 'join' -- TransitionEngine creates the "
            "target cursor as soon as any single non-join incoming edge's "
            "source completes, so the target advances on the first "
            "finished branch rather than waiting for all of them"
        )


def test_concurrent_event_log_instances_do_not_collide_on_seq(tmp_path: Path):
    log_a = EventLog(tmp_path / "events")
    log_b = EventLog(tmp_path / "events")

    stored_a = log_a.append(
        Event(
            spec_version="1.0.0",
            seq=0,
            run_id="run-1",
            node_id="node-a",
            event_type="transition-attempted",
            payload={},
            event_id="evt-a",
        )
    )
    stored_b = log_b.append(
        Event(
            spec_version="1.0.0",
            seq=0,
            run_id="run-1",
            node_id="node-b",
            event_type="transition-attempted",
            payload={},
            event_id="evt-b",
        )
    )

    assert stored_a.seq != stored_b.seq, (
        "two EventLog instances opened concurrently on the same directory "
        "assigned the same seq -- EventLog.append must serialize across "
        "instances/processes and recompute the authoritative seq from disk "
        "rather than trusting an in-memory counter cached at construction "
        "time"
    )

    log_a.close()
    log_b.close()

    reread = EventLog(tmp_path / "events")
    events = reread.read_all()
    reread.close()

    assert len(events) == 2
    assert [event.seq for event in events] == [0, 1]


def test_evidence_supplied_to_apply_is_persisted_on_the_event(tmp_path: Path):
    graph = Graph(
        spec_version="1.0.0",
        nodes={
            "n1": Node(
                id="n1",
                kind="gate",
                metadata={
                    "evidence_requirement": {
                        "spec_version": "1.0.0",
                        "evidence": [
                            {"proof_type": "signoff", "constraint": "required"},
                        ],
                    }
                },
            ),
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("n1", "start")

    engine.apply("n1", "complete", evidence={"signoff": {"approved": True}})

    events = log.read_all()
    assert events[-1].payload.get("evidence") == {"signoff": {"approved": True}}, (
        "evidence supplied to apply() must be persisted onto the committed "
        "Event so there is a durable audit trail of what evidence satisfied "
        "a gate"
    )


def test_runtime_doc_documents_recent_repair_additions():
    text = RUNTIME_DOC_PATH.read_text()
    lowered = text.lower()

    assert "close()" in text or "context manager" in lowered or "context-manager" in lowered, (
        "docs/runtime.md must document EventLog.close()/the context-manager "
        "protocol for releasing the underlying file handle"
    )
    assert "flock" in lowered, (
        "docs/runtime.md must document append()'s flock-based concurrency "
        "serialization across EventLog instances/processes"
    )
    assert lowered.count("migrate_document") >= 2, (
        "docs/runtime.md must document migrate_document-on-read for both "
        "EventLog and RunStateStore, not just the migrations module itself"
    )
    assert "last_applied_seq" in text and "ahead" in lowered, (
        "docs/runtime.md must document TransitionEngine's checkpoint-ahead-"
        "of-log fail-closed check"
    )
    assert "audit trail" in lowered, (
        "docs/runtime.md must document that TransitionEngine.apply persists "
        "evidence onto the committed Event for a durable audit trail"
    )


def test_read_all_observes_concurrent_appends_from_another_instance(tmp_path: Path):
    log_a = EventLog(tmp_path / "events")
    log_b = EventLog(tmp_path / "events")

    log_a.append(
        Event(
            spec_version="1.0.0",
            seq=0,
            run_id="run-1",
            node_id="node-a",
            event_type="transition-attempted",
            payload={},
            event_id="evt-a",
        )
    )

    assert len(log_b.read_all()) == 1, (
        "EventLog.read_all() must observe events appended by another "
        "instance/process on the same directory, not only refresh its cache "
        "on append()/__init__ -- inconsistent with the concurrency hardening "
        "already added to append()"
    )

    log_a.close()
    log_b.close()
