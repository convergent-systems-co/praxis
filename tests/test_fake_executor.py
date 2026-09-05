"""RED-phase tests for the deterministic fake-executor test harness (T7).

FakeExecutor drives a run purely through TransitionEngine's public
`legal_next`/`apply` surface -- never touching RunStateStore/EventLog
directly -- so it cannot bypass transition legality itself. The `script`
gives one fully predetermined outcome event per node (e.g. "complete" or
"fail", plus any required evidence); the mechanical PENDING -> RUNNING
"start" step is not scripted since it is the only legal transition from
PENDING and requires no decision. Graphs are built inline as dataclasses,
the same convention test_transitions.py uses.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_runtime.events import EventLog
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError


def _linear_three_node_graph() -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={
            "n1": Node(id="n1", kind="task"),
            "n2": Node(id="n2", kind="task"),
            "n3": Node(id="n3", kind="task"),
        },
        edges=[
            Edge(source="n1", target="n2", kind="sequential"),
            Edge(source="n2", target="n3", kind="sequential"),
        ],
        entry_node="n1",
        terminal_nodes={"n3"},
    )


def _single_node_graph() -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={"n1": Node(id="n1", kind="task")},
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )


def test_run_to_completion_drives_linear_three_node_graph_to_terminal_success(
    tmp_path: Path,
):
    graph = _linear_three_node_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    script = {
        "n1": {"event_type": "complete", "evidence": None},
        "n2": {"event_type": "complete", "evidence": None},
        "n3": {"event_type": "complete", "evidence": None},
    }
    executor = FakeExecutor(engine, script)

    final_state = executor.run_to_completion()

    assert set(final_state.cursors) == {"n1", "n2", "n3"}
    for node_id in ("n1", "n2", "n3"):
        assert (
            final_state.cursors[node_id].status == NodeStatus.TERMINAL_SUCCESS.value
        )


def test_run_to_completion_surfaces_transition_error_for_illegal_scripted_event(
    tmp_path: Path,
):
    graph = _single_node_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    script = {
        # "teleport" is not a legal event from either PENDING or RUNNING for
        # this node, so the harness must let TransitionError propagate
        # rather than swallowing it.
        "n1": {"event_type": "teleport", "evidence": None},
    }
    executor = FakeExecutor(engine, script)

    with pytest.raises(TransitionError):
        executor.run_to_completion()
