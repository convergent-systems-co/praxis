"""Test for crash/restart resume at every transition boundary (T9).

Drives a fan-out-plus-join graph (the same shape T4's test_transitions.py
uses) to completion twice with the same deterministic script: once
uninterrupted via FakeExecutor as the control run, and once where every
single `engine.apply()` call is immediately followed by discarding the
in-memory TransitionEngine and rebuilding one via `resume()` against the
same on-disk state/event directories -- simulating a process crash at every
transition boundary. The two runs must reach the same cursor states, proving
resume() never loses or double-applies a transition regardless of when the
crash happens. `run_id` is excluded from the comparison since it is
independently randomly generated per run and carries no information about
correctness.
"""

from __future__ import annotations

import dataclasses
import uuid
from pathlib import Path

from praxis_runtime.events import Event, EventLog
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.replay import resume
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

_TERMINAL_VALUES = {NodeStatus.TERMINAL_SUCCESS.value, NodeStatus.TERMINAL_FAILED.value}

_SCRIPT = {
    "start": {"event_type": "complete", "evidence": None},
    "a": {"event_type": "complete", "evidence": None},
    "b": {"event_type": "complete", "evidence": None},
    "end": {"event_type": "complete", "evidence": None},
}


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


def _apply_one_transition(engine: TransitionEngine, script: dict[str, dict]) -> bool:
    """Apply exactly one legal transition; report whether work remains.

    Mirrors FakeExecutor's per-node decision (mechanical PENDING->RUNNING
    "start", else the scripted terminal event) but stops after a single
    engine.apply() call, so the caller can simulate a crash at every
    individual transition boundary rather than only between FakeExecutor's
    grouped rounds.
    """
    state = engine.current_state()
    for node_id, cursor in state.cursors.items():
        if cursor.status in _TERMINAL_VALUES:
            continue
        legal = engine.legal_next(node_id)
        if "start" in legal:
            engine.apply(node_id, "start")
        else:
            scripted = script[node_id]
            engine.apply(node_id, scripted["event_type"], evidence=scripted["evidence"])
        break

    state = engine.current_state()
    return any(cursor.status not in _TERMINAL_VALUES for cursor in state.cursors.values())


def test_crash_and_resume_after_every_transition_matches_uninterrupted_control_run(
    tmp_path: Path,
):
    control_store = RunStateStore(tmp_path / "control" / "run-state.json")
    control_log = EventLog(tmp_path / "control" / "events")
    control_engine = TransitionEngine(_fan_out_join_graph(), control_store, control_log)
    control_executor = FakeExecutor(control_engine, _SCRIPT)

    control_final_state = control_executor.run_to_completion()

    assert all(
        cursor.status == NodeStatus.TERMINAL_SUCCESS.value
        for cursor in control_final_state.cursors.values()
    )

    graph = _fan_out_join_graph()
    crash_state_path = tmp_path / "crash" / "run-state.json"
    crash_events_dir = tmp_path / "crash" / "events"
    crash_store = RunStateStore(crash_state_path)
    crash_log = EventLog(crash_events_dir)

    engine = resume(graph, crash_store, crash_log)
    while True:
        work_remains = _apply_one_transition(engine, _SCRIPT)

        # Simulate a crash: discard the in-memory TransitionEngine (and, in
        # a real process, the FakeExecutor driving it) and rebuild purely
        # from the same on-disk state/event directories via resume().
        crash_store = RunStateStore(crash_state_path)
        crash_log = EventLog(crash_events_dir)
        engine = resume(graph, crash_store, crash_log)

        if not work_remains:
            break

    crash_final_state = engine.current_state()

    assert dataclasses.replace(crash_final_state, run_id="") == dataclasses.replace(
        control_final_state, run_id=""
    )


def test_resume_reconciles_checkpoint_that_lags_the_event_log(tmp_path: Path):
    """Exercise resume()'s reconciliation directly, not just via full re-runs.

    TransitionEngine.apply() appends the event and saves the checkpoint in
    one synchronous call, so a crash simulated only *between* apply() calls
    (as in the test above) never actually lands in the event-log/checkpoint
    divergence window that resume()'s reconciliation (replay.py lines
    108-118) exists to repair. Reproduce that window directly: append an
    event to the on-disk log without saving a matching checkpoint (mirroring
    a crash after EventLog.append() durably persists but before
    state_store.save() runs inside apply()), then assert resume() folds the
    dangling event into the returned engine's state and persists the
    reconciled checkpoint. If reconciliation were removed, resume() would
    hand back the stale checkpoint as-is, and the assertions below would see
    "start" still "running" with no "a"/"b" successor cursors.
    """
    graph = _fan_out_join_graph()
    state_path = tmp_path / "lag" / "run-state.json"
    events_dir = tmp_path / "lag" / "events"
    store = RunStateStore(state_path)
    log = EventLog(events_dir)

    engine = TransitionEngine(graph, store, log)
    state = engine.apply("start", "start")
    assert store.load().last_applied_seq == 0

    # Simulate a crash after the event is durably appended but before the
    # matching checkpoint save: write straight to the event log, bypassing
    # engine.apply() and its synchronous state_store.save().
    log.append(
        Event(
            spec_version="1.0.0",
            seq=0,
            run_id=state.run_id,
            node_id="start",
            event_type="complete",
            payload={},
            event_id=uuid.uuid4().hex,
        )
    )
    assert store.load().last_applied_seq == 0  # checkpoint still lags the log

    resumed_engine = resume(graph, store, log)
    reconciled = resumed_engine.current_state()

    assert reconciled.last_applied_seq == 1
    assert reconciled.cursors["start"].status == NodeStatus.TERMINAL_SUCCESS.value
    assert reconciled.cursors["a"].status == NodeStatus.PENDING.value
    assert reconciled.cursors["b"].status == NodeStatus.PENDING.value

    # resume() must persist the reconciliation, not just hand back an
    # in-memory patch -- a subsequent load from disk has to see it too.
    persisted = store.load()
    assert persisted.last_applied_seq == 1
    assert persisted.cursors["start"].status == NodeStatus.TERMINAL_SUCCESS.value
