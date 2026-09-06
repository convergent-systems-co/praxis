"""Live-attach and replay sources for the dashboard.

Exercises DashboardSource.poll_live/replay_snapshot against the same
on-disk run_directory convention (run-state.json + an events/ subdirectory)
that tests/test_end_to_end_fake_executor.py and
TransitionEngine/RunStateStore/EventLog use directly, driving
examples/sample-graph.json (a fan-out/join graph with no resource_claims,
so the lease_directory=None/executor_registry=None default path is exactly
what's exercised) through TransitionEngine + FakeExecutor.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from praxis_dashboard.sources import DashboardSource, DashboardSourceError
from praxis_runtime.events import EventLog
from praxis_runtime.graph import GraphValidationError, load_graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"


def test_poll_live_on_fresh_run_directory_shows_only_entry_node(tmp_path: Path):
    # No run-state.json and no events/ have been written yet -- poll_live
    # must fall back to the same PENDING-at-entry_node state
    # TransitionEngine.current_state() uses when unchecked.
    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path)

    snapshot = source.poll_live()

    assert snapshot.mode == "live"
    assert snapshot.run_summary.total_nodes == 1


def test_poll_live_reflects_transitions_driven_externally(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("intake", "start")
    engine.apply("intake", "complete")

    # A fresh DashboardSource over the same run_directory, constructed after
    # the transitions above were applied by an unrelated TransitionEngine
    # instance, must still see them: poll_live re-reads run-state and the
    # event log fresh on every call.
    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path)
    snapshot = source.poll_live()

    intake_view = next(v for v in snapshot.nodes if v.node_id == "intake")
    assert intake_view.status == NodeStatus.TERMINAL_SUCCESS.value


def test_replay_snapshot_after_full_run_shows_every_node_terminal_success(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    script = {node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes}
    FakeExecutor(engine, script).run_to_completion()

    # replay_snapshot must reconstruct this purely from the event log, with
    # no checkpoint file consulted -- ignoring run-state.json entirely is
    # exactly what makes it work after the owning process has exited.
    (tmp_path / "run-state.json").unlink()

    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path)
    snapshot = source.replay_snapshot()

    assert snapshot.mode == "replay"
    assert all(v.status == NodeStatus.TERMINAL_SUCCESS.value for v in snapshot.nodes)


def test_malformed_graph_path_raises_graph_validation_error(tmp_path: Path):
    # Schema-valid (one node, satisfies "nodes" minItems) but structurally
    # invalid: entry_node references a node id that doesn't exist, which
    # load_graph's own post-schema invariant check rejects.
    bad_graph_path = tmp_path / "graph.json"
    bad_graph_path.write_text(
        json.dumps(
            {
                "spec_version": "1.0.0",
                "nodes": [{"id": "n1", "kind": "task"}],
                "edges": [],
                "entry_node": "missing",
                "terminal_nodes": ["n1"],
            }
        )
    )

    with pytest.raises(GraphValidationError):
        DashboardSource(bad_graph_path, tmp_path)


def test_lease_directory_and_executor_registry_defaults_yield_empty_resources_and_capabilities(
    tmp_path: Path,
):
    source = DashboardSource(SAMPLE_GRAPH_PATH, tmp_path)

    snapshot = source.poll_live()

    assert snapshot.resources == ()
    assert snapshot.capabilities == ()
