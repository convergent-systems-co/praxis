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

import praxis_runtime.transitions as transitions
from praxis_dashboard import sources
from praxis_dashboard.sources import DashboardSource, DashboardSourceError
from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.registry import ExecutorRegistry
from praxis_runtime.events import EventLog
from praxis_runtime.graph import GraphValidationError, load_graph
from praxis_runtime.resources import leases
from praxis_runtime.resources.leases import LeaseStore
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"
_SPEC_VERSION = "1.0.0"

_RESOURCE_CLAIM_GRAPH_DOCUMENT = {
    "spec_version": _SPEC_VERSION,
    "nodes": [
        {
            "id": "n1",
            "kind": "task",
            "metadata": {
                "resource_claims": {
                    "spec_version": _SPEC_VERSION,
                    "claims": [
                        {
                            "resource_type": "filesystem",
                            "quantity": 1,
                            "identifier": "/workspace/output.txt",
                            "access_mode": "write",
                        }
                    ],
                }
            },
        }
    ],
    "edges": [],
    "entry_node": "n1",
    "terminal_nodes": ["n1"],
}


def _capability_advertising_executor() -> FakeCapabilityExecutor:
    return FakeCapabilityExecutor(
        "executor-1",
        [
            {
                "spec_version": _SPEC_VERSION,
                "id": "cap-primary",
                "satisfies": [{"kind": "text-generation", "parameters": {"cost": 0.5}}],
            }
        ],
        script={},
    )


def test_entry_pending_state_spec_version_tracks_transitions_module(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
):
    # The dashboard's fallback RunState (built when no checkpoint exists yet)
    # must mirror TransitionEngine.current_state()'s own unchecked fallback,
    # spec_version included: a future bump to the runtime's own
    # praxis_runtime.transitions._SPEC_VERSION constant must never silently
    # desync from this module's fallback, so this reads it by reference
    # rather than duplicating the literal.
    monkeypatch.setattr(transitions, "_SPEC_VERSION", "9.9.9")
    graph = load_graph(SAMPLE_GRAPH_PATH)

    state = sources._entry_pending_state(graph)

    assert state.spec_version == "9.9.9"


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


def test_poll_live_with_populated_lease_directory_and_executor_registry_surfaces_real_data(
    tmp_path: Path,
):
    # Unlike the defaults test above, this exercises the non-None
    # lease_directory/executor_registry path end-to-end through poll_live:
    # a real acquired lease under a resource_type the graph actually
    # declares, and a real registered executor's advertisement, must both
    # flow all the way through to the returned snapshot's resources/
    # capabilities -- not just through build_resource_views/
    # build_capability_views in isolation (see test_dashboard_resource_view.py
    # and test_dashboard_executor_view.py for those unit-level proofs).
    graph_path = tmp_path / "graph.json"
    graph_path.write_text(json.dumps(_RESOURCE_CLAIM_GRAPH_DOCUMENT))
    run_directory = tmp_path / "run"
    lease_directory = tmp_path / "leases"

    lease = leases.acquire(
        LeaseStore(lease_directory),
        "filesystem",
        "/workspace/output.txt",
        "owner-a",
        now=0.0,
        ttl=10.0,
    )

    registry = ExecutorRegistry()
    registry.register("executor-1", _capability_advertising_executor())

    source = DashboardSource(
        graph_path,
        run_directory,
        lease_directory=lease_directory,
        executor_registry=registry,
    )

    snapshot = source.poll_live()

    assert len(snapshot.resources) == 1
    resource_view = snapshot.resources[0]
    assert resource_view.resource_type == "filesystem"
    assert resource_view.identifier == "/workspace/output.txt"
    assert resource_view.owner == "owner-a"
    assert resource_view.access_mode == "write"
    assert resource_view.epoch == lease.epoch

    assert len(snapshot.capabilities) == 1
    capability_view = snapshot.capabilities[0]
    assert capability_view.executor_id == "executor-1"
    assert capability_view.satisfied_kinds == ("text-generation",)
    assert capability_view.cost_hint == 0.5


def test_replay_snapshot_with_populated_lease_directory_and_executor_registry_surfaces_real_data(
    tmp_path: Path,
):
    graph_path = tmp_path / "graph.json"
    graph_path.write_text(json.dumps(_RESOURCE_CLAIM_GRAPH_DOCUMENT))
    run_directory = tmp_path / "run"
    lease_directory = tmp_path / "leases"

    graph = load_graph(graph_path)
    store = RunStateStore(run_directory / "run-state.json")
    log = EventLog(run_directory / "events")
    engine = TransitionEngine(graph, store, log)
    engine.apply("n1", "start")
    engine.apply("n1", "complete")

    leases.acquire(
        LeaseStore(lease_directory),
        "filesystem",
        "/workspace/output.txt",
        "owner-a",
        now=0.0,
        ttl=10.0,
    )

    registry = ExecutorRegistry()
    registry.register("executor-1", _capability_advertising_executor())

    source = DashboardSource(
        graph_path,
        run_directory,
        lease_directory=lease_directory,
        executor_registry=registry,
    )

    snapshot = source.replay_snapshot()

    assert snapshot.mode == "replay"
    assert len(snapshot.resources) == 1
    assert snapshot.resources[0].owner == "owner-a"
    assert len(snapshot.capabilities) == 1
    assert snapshot.capabilities[0].executor_id == "executor-1"
