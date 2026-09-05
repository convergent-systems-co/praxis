"""RED-phase tests for the deterministic transition engine (T4).

TransitionEngine.apply is the single mutation entrypoint: every state change
and event-log write goes through it, so callers (fake executors, replay,
dashboards) can never bypass transition legality. Graphs are built directly
as dataclasses here (not via load_graph), keeping node/edge/status wiring in
Python -- the same inline-graph convention T9's crash/restart suite uses.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_runtime.events import EventLog
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError


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


def _fan_out_join_graph() -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={
            "start": Node(id="start", kind="task"),
            "a": Node(id="a", kind="task"),
            "b": Node(id="b", kind="task"),
            "end": Node(id="end", kind="task"),
        },
        edges=[
            Edge(source="start", target="a", kind="fan-out"),
            Edge(source="start", target="b", kind="fan-out"),
            Edge(source="a", target="end", kind="join"),
            Edge(source="b", target="end", kind="join"),
        ],
        entry_node="start",
        terminal_nodes={"end"},
    )


def _gated_graph() -> Graph:
    return Graph(
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


def test_current_state_initializes_entry_node_pending(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    state = engine.current_state()

    assert set(state.cursors) == {"n1"}
    assert state.cursors["n1"].status == NodeStatus.PENDING.value


def test_legal_transition_applies_and_persists(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    state = engine.apply("n1", "start")

    assert state.cursors["n1"].status == NodeStatus.RUNNING.value
    assert store.load() == state
    events = log.read_all()
    assert len(events) == 1
    assert events[0].node_id == "n1"
    assert events[0].event_type == "start"


def test_illegal_transition_wrong_status_raises_and_leaves_state_unchanged(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete")  # n1 is still PENDING; "start" was never applied

    assert store.load() is None
    assert log.read_all() == []


def test_illegal_transition_on_unreached_node_raises(tmp_path: Path):
    graph = _fan_out_join_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    with pytest.raises(TransitionError):
        # "a" has no cursor yet: the fan-out edge from "start" hasn't fired.
        engine.apply("a", "start")

    assert store.load() is None
    assert log.read_all() == []


def test_evidence_required_missing_or_wrong_key_raises(tmp_path: Path):
    graph = _gated_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("n1", "start")

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete", evidence=None)

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete", evidence={})

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete", evidence={"unrelated-key": True})

    state = store.load()
    assert state.cursors["n1"].status == NodeStatus.RUNNING.value
    assert len(log.read_all()) == 1  # only the earlier "start" event persisted


def test_evidence_required_present_allows_transition(tmp_path: Path):
    graph = _gated_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("n1", "start")

    state = engine.apply("n1", "complete", evidence={"signoff": {"approved": True}})

    assert state.cursors["n1"].status == NodeStatus.TERMINAL_SUCCESS.value


def test_fan_out_creates_a_cursor_for_every_target(tmp_path: Path):
    graph = _fan_out_join_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("start", "start")

    state = engine.apply("start", "complete")

    assert state.cursors["start"].status == NodeStatus.TERMINAL_SUCCESS.value
    assert state.cursors["a"].status == NodeStatus.PENDING.value
    assert state.cursors["b"].status == NodeStatus.PENDING.value
    assert "end" not in state.cursors  # join target not created until both arms finish


def test_join_advances_only_after_every_incoming_cursor_completes(tmp_path: Path):
    graph = _fan_out_join_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("start", "start")
    engine.apply("start", "complete")
    engine.apply("a", "start")

    state = engine.apply("a", "complete")
    assert "end" not in state.cursors  # b hasn't completed yet: join must not advance

    engine.apply("b", "start")
    state = engine.apply("b", "complete")

    assert "end" in state.cursors
    assert state.cursors["end"].status == NodeStatus.PENDING.value


def test_legal_next_reflects_current_status_without_mutating_state(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    before = engine.legal_next("n1")
    assert "start" in before
    assert store.load() is None  # a pure query must not create a checkpoint

    engine.apply("n1", "start")

    after = engine.legal_next("n1")
    assert "start" not in after
