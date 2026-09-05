"""RED-phase tests for resume/replay support (T5).

replay() must reconstruct a RunState purely from EventLog.read_all() and the
graph's transition rules -- proving that the event log alone is sufficient
to recover a run, independent of the checkpoint file. resume() is the
process-restart entrypoint: it loads the last checkpoint (if any) and
replays only the events appended after it, so a crash between an event
append and its checkpoint save is never lost. Graphs are built directly as
dataclasses, matching T4's test_transitions.py convention.
"""

from __future__ import annotations

import uuid
from pathlib import Path

from conftest import _linear_graph
from praxis_runtime.events import Event, EventLog
from praxis_runtime.graph import Graph, Node
from praxis_runtime.replay import replay, resume
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

_SPEC_VERSION = "1.0.0"


def _evidence_gated_graph() -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={
            "n1": Node(
                id="n1",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "evidence": [
                            {"proof_type": "signoff", "constraint": "required"},
                        ],
                    },
                },
            ),
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )


def test_replay_reconstructs_same_state_as_engine(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    engine.apply("n1", "start")
    engine.apply("n1", "complete")
    final_state = engine.apply("n2", "start")

    replayed_state = replay(log, graph)

    assert replayed_state == final_state


def test_resume_replays_event_appended_after_last_checkpoint(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    engine.apply("n1", "start")
    checkpoint = engine.apply("n1", "complete")
    assert checkpoint.last_applied_seq == 1  # n2 is PENDING; nothing has run on it yet

    # Simulate a crash between event-append and checkpoint-save: append the
    # next event directly via the event log, without updating the checkpoint
    # file. The on-disk checkpoint still reflects last_applied_seq == 1.
    crash_event = Event(
        spec_version=_SPEC_VERSION,
        seq=0,
        run_id=checkpoint.run_id,
        node_id="n2",
        event_type="start",
        payload={},
        event_id=uuid.uuid4().hex,
    )
    log.append(crash_event)
    assert store.load().last_applied_seq == 1  # checkpoint file is unchanged by the append

    resumed_engine = resume(graph, store, log)
    resumed_state = resumed_engine.current_state()

    assert resumed_state.cursors["n2"].status == NodeStatus.RUNNING.value
    assert resumed_state.last_applied_seq == 2


def test_replay_reconstructs_state_for_node_with_evidence_requirement(tmp_path: Path):
    graph = _evidence_gated_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    engine.apply("n1", "start")
    final_state = engine.apply("n1", "complete", evidence={"signoff": {"approved": True}})

    replayed_state = replay(log, graph)

    assert replayed_state == final_state
    assert replayed_state.cursors["n1"].status == NodeStatus.TERMINAL_SUCCESS.value
