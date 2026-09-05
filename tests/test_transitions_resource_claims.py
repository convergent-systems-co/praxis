"""Resource-claim/lease gating wired into TransitionEngine.

TransitionEngine accepts an optional resource_lease_store: when a node
declares a "resource_claims" document in its metadata and a lease store is
configured, starting the node (PENDING -> RUNNING) acquires a lease per
declared claim, and completing or failing the node (-> a terminal status)
revalidates and releases each lease. Two TransitionEngine instances pointed
at the same LeaseStore path simulate two independent schedulers contending
for the same resource identifier: a conflicting claim raises TransitionError
fail-closed. A node with no "resource_claims" metadata and no
resource_lease_store configured is untouched by any of this -- the existing
tests/test_transitions.py suite continues to pass unmodified.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from conftest import _linear_graph
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Graph, Node
from praxis_runtime.resources.leases import LeaseStore
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

RESOURCE_TYPE = "filesystem"
IDENTIFIER = "/workspace/output.txt"

FILESYSTEM_WRITE_CLAIM = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": RESOURCE_TYPE,
            "quantity": 1,
            "identifier": IDENTIFIER,
            "access_mode": "write",
        }
    ],
}


def _single_node_graph(node_id: str, resource_claims: dict | None) -> Graph:
    metadata = {"resource_claims": resource_claims} if resource_claims is not None else {}
    return Graph(
        spec_version="1.0.0",
        nodes={node_id: Node(id=node_id, kind="task", metadata=metadata)},
        edges=[],
        entry_node=node_id,
        terminal_nodes={node_id},
    )


def test_start_with_declared_claim_acquires_lease(tmp_path: Path):
    graph = _single_node_graph("n1", FILESYSTEM_WRITE_CLAIM)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    lease_store = LeaseStore(tmp_path / "leases")
    engine = TransitionEngine(graph, store, log, resource_lease_store=lease_store)

    engine.apply("n1", "start")

    lease = lease_store.load(RESOURCE_TYPE, IDENTIFIER)
    assert lease is not None
    assert lease.owner == "n1"
    assert lease.status == "active"


def test_start_with_conflicting_claim_from_second_scheduler_raises(tmp_path: Path):
    lease_dir = tmp_path / "leases"

    graph_one = _single_node_graph("n1", FILESYSTEM_WRITE_CLAIM)
    engine_one = TransitionEngine(
        graph_one,
        RunStateStore(tmp_path / "run-one.json"),
        EventLog(tmp_path / "events-one"),
        resource_lease_store=LeaseStore(lease_dir),
    )
    engine_one.apply("n1", "start")

    # A second scheduler (its own TransitionEngine/state/log) pointed at the
    # same LeaseStore path contends for the same declared identifier.
    graph_two = _single_node_graph("n2", FILESYSTEM_WRITE_CLAIM)
    engine_two = TransitionEngine(
        graph_two,
        RunStateStore(tmp_path / "run-two.json"),
        EventLog(tmp_path / "events-two"),
        resource_lease_store=LeaseStore(lease_dir),
    )

    with pytest.raises(TransitionError):
        engine_two.apply("n2", "start")


def test_complete_revalidates_and_releases_lease_allowing_reacquire(tmp_path: Path):
    lease_dir = tmp_path / "leases"

    graph_one = _single_node_graph("n1", FILESYSTEM_WRITE_CLAIM)
    engine_one = TransitionEngine(
        graph_one,
        RunStateStore(tmp_path / "run-one.json"),
        EventLog(tmp_path / "events-one"),
        resource_lease_store=LeaseStore(lease_dir),
    )
    engine_one.apply("n1", "start")

    engine_one.apply("n1", "complete")

    released = LeaseStore(lease_dir).load(RESOURCE_TYPE, IDENTIFIER)
    assert released.status == "released"

    graph_two = _single_node_graph("n2", FILESYSTEM_WRITE_CLAIM)
    engine_two = TransitionEngine(
        graph_two,
        RunStateStore(tmp_path / "run-two.json"),
        EventLog(tmp_path / "events-two"),
        resource_lease_store=LeaseStore(lease_dir),
    )

    engine_two.apply("n2", "start")

    reacquired = LeaseStore(lease_dir).load(RESOURCE_TYPE, IDENTIFIER)
    assert reacquired.owner == "n2"
    assert reacquired.status == "active"


def test_node_without_resource_claims_and_no_lease_store_is_unaffected(tmp_path: Path):
    graph = _linear_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)

    state = engine.apply("n1", "start")

    assert state.cursors["n1"].status == NodeStatus.RUNNING.value
