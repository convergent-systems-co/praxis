"""Fail-closed edge-case suite (T10).

Exercises graph/events/state/transitions failure paths that the per-module
suites (T1-T4) don't already cover end-to-end: malformed graph documents,
a checkpoint whose last_applied_seq claims to be ahead of the event log,
a duplicate event_id reaching TransitionEngine.apply (not just EventLog
directly), illegal transitions, and an interrupted checkpoint write observed
through a full engine.apply() call rather than the store in isolation. Every
scenario here must fail closed: raise, and never leave a partial mutation
(no event appended, no checkpoint advanced) behind.
"""

from __future__ import annotations

import copy
import json
import uuid
from pathlib import Path

import pytest

from praxis_runtime import transitions as transitions_module
from praxis_runtime.events import EventLog, EventLogError
from praxis_runtime.graph import Edge, Graph, GraphValidationError, Node, load_graph
from praxis_runtime.state import Cursor, RunState, RunStateError, RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

VALID_GRAPH = {
    "spec_version": "1.0.0",
    "nodes": [
        {"id": "start", "kind": "start"},
        {"id": "middle", "kind": "task"},
        {"id": "end", "kind": "end"},
    ],
    "edges": [
        {"source": "start", "target": "middle", "kind": "sequential"},
        {"source": "middle", "target": "end", "kind": "sequential"},
    ],
    "entry_node": "start",
    "terminal_nodes": ["end"],
}


def _write_graph(tmp_path: Path, instance: dict) -> Path:
    path = tmp_path / "graph.json"
    path.write_text(json.dumps(instance))
    return path


def _linear_graph() -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={
            "n1": Node(id="n1", kind="task"),
            "n2": Node(id="n2", kind="task"),
        },
        edges=[Edge(source="n1", target="n2", kind="sequential")],
        entry_node="n1",
        terminal_nodes={"n2"},
    )


# -- Malformed graph: fail closed, no partial Graph object returned --------


def test_missing_entry_node_key_fails_closed(tmp_path: Path):
    instance = copy.deepcopy(VALID_GRAPH)
    del instance["entry_node"]
    path = _write_graph(tmp_path, instance)

    with pytest.raises(GraphValidationError):
        load_graph(path)


def test_node_kind_violating_pattern_fails_closed(tmp_path: Path):
    instance = copy.deepcopy(VALID_GRAPH)
    instance["nodes"].append({"id": "bad", "kind": "Not_Valid!"})
    instance["edges"].append({"source": "end", "target": "bad", "kind": "sequential"})
    path = _write_graph(tmp_path, instance)

    with pytest.raises(GraphValidationError):
        load_graph(path)


# -- Stale state: checkpoint claims to be ahead of the event log -----------


def test_checkpoint_ahead_of_event_log_raises(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    engine.apply("n1", "start")  # event log now holds exactly one event: seq 0

    on_disk_run_id = store.load().run_id
    stale_state = RunState(
        spec_version="1.0.0",
        run_id=on_disk_run_id,
        cursors={"n1": Cursor(node_id="n1", status=NodeStatus.RUNNING.value)},
        last_applied_seq=7,  # far beyond the single event actually on disk
    )
    store.save(stale_state)

    with pytest.raises((TransitionError, RunStateError)):
        engine.apply("n1", "complete")


def test_checkpoint_ahead_of_empty_event_log_raises(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    # No events have ever been appended, but the checkpoint claims one has
    # already been applied -- the empty-log edge case of the same "ahead of
    # the log" guard the previous test exercises with a non-empty log.
    stale_state = RunState(
        spec_version="1.0.0",
        run_id=uuid.uuid4().hex,
        cursors={"n1": Cursor(node_id="n1", status=NodeStatus.RUNNING.value)},
        last_applied_seq=0,
    )
    store.save(stale_state)

    with pytest.raises((TransitionError, RunStateError)):
        engine.apply("n1", "complete")


# -- Duplicate events: a caller retry must not double-apply through the engine --


def test_duplicate_event_id_through_apply_does_not_double_apply(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    fixed_id = uuid.UUID(int=0)
    monkeypatch.setattr(transitions_module.uuid, "uuid4", lambda: fixed_id)

    engine.apply("n1", "start")  # seq 0, event_id = fixed_id.hex

    with pytest.raises(EventLogError):
        # A legal follow-on transition, but the mocked uuid4 reuses the same
        # event_id -- standing in for a caller retry that resends the same
        # idempotency key after a crash.
        engine.apply("n1", "complete")

    events = log.read_all()
    assert len(events) == 1
    assert events[0].event_type == "start"

    state = store.load()
    assert state.cursors["n1"].status == NodeStatus.RUNNING.value  # "complete" never applied


# -- Illegal transitions -----------------------------------------------------


def test_transition_not_in_graph_edges_from_current_status_raises(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    with pytest.raises(TransitionError):
        engine.apply("n1", "teleport")  # not a legal event_type from PENDING

    assert store.load() is None
    assert log.read_all() == []


def test_transition_on_already_terminal_node_raises(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("n1", "start")
    engine.apply("n1", "complete")

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete")  # n1 is already TERMINAL_SUCCESS

    events = log.read_all()
    assert len(events) == 2  # the rejected retry appended nothing


# -- Interrupted writes, driven through a full TransitionEngine.apply() ----


def test_interrupted_checkpoint_write_recovered_through_next_apply(tmp_path: Path):
    graph = _linear_graph()
    run_state_path = tmp_path / "run-state.json"
    store = RunStateStore(run_state_path)
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    state_after_start = engine.apply("n1", "start")

    # Simulate a crash mid-write on some other save attempt: garbage left in
    # the temp file, the real checkpoint (reflecting "start") untouched.
    (tmp_path / "run-state.json.tmp").write_text("{not valid json")

    state_after_complete = engine.apply("n1", "complete")

    assert state_after_start.cursors["n1"].status == NodeStatus.RUNNING.value
    assert state_after_complete.cursors["n1"].status == NodeStatus.TERMINAL_SUCCESS.value
