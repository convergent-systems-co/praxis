"""Resource claim/lease projection for the dashboard.

Exercises collect_resource_types (graph metadata -> resource_type set,
sourced from both a node's declared "resource_claims" and its observed
"observed_resources" documents -- both share resource-claim.schema.json's
shape, see src/praxis_runtime/resources/observed.py) and
build_resource_views (LeaseStore -> read-only LeaseView snapshot).

Stale-lease detection: a lease whose heartbeat_deadline has passed but whose
status is still "active" (not yet reaped by a new acquire/renew, per
src/praxis_runtime/resources/leases.py's own expiry model) must be surfaced
with expired=True and a non-None stale_warning -- unlike a released lease,
which active_writer_leases/active_reader_leases never return at all (their
own status == "active" filter), so no LeaseView is fabricated for it.
"""

from __future__ import annotations

from pathlib import Path

from praxis_dashboard.resource_view import (
    LeaseView,
    build_resource_views,
    collect_resource_types,
)
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.resources import leases
from praxis_runtime.resources.leases import LeaseStore

_SPEC_VERSION = "1.0.0"

FILESYSTEM_WRITE_CLAIM = {
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

COMPUTE_SLOT_OBSERVED = {
    "spec_version": _SPEC_VERSION,
    "claims": [
        {
            "resource_type": "compute-slot",
            "quantity": 1,
            "identifier": "slot-1",
            "access_mode": "read",
        }
    ],
}


def _single_node_graph(node_id: str, metadata: dict) -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={node_id: Node(id=node_id, kind="task", metadata=metadata)},
        edges=[],
        entry_node=node_id,
        terminal_nodes={node_id},
    )


def _two_node_graph() -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={
            "n1": Node(id="n1", kind="task", metadata={"resource_claims": FILESYSTEM_WRITE_CLAIM}),
            "n2": Node(id="n2", kind="task", metadata={"observed_resources": COMPUTE_SLOT_OBSERVED}),
        },
        edges=[Edge(source="n1", target="n2", kind="sequential")],
        entry_node="n1",
        terminal_nodes={"n2"},
    )


def test_collect_resource_types_reads_declared_resource_claims():
    graph = _single_node_graph("n1", {"resource_claims": FILESYSTEM_WRITE_CLAIM})

    assert collect_resource_types(graph) == frozenset({"filesystem"})


def test_collect_resource_types_reads_observed_resources():
    graph = _single_node_graph("n1", {"observed_resources": COMPUTE_SLOT_OBSERVED})

    assert collect_resource_types(graph) == frozenset({"compute-slot"})


def test_collect_resource_types_unions_across_nodes():
    graph = _two_node_graph()

    assert collect_resource_types(graph) == frozenset({"filesystem", "compute-slot"})


def test_active_unexpired_writer_lease_produces_write_view(tmp_path: Path):
    store = LeaseStore(tmp_path)
    lease = leases.acquire(
        store, "filesystem", "/workspace/output.txt", "owner-a", now=0.0, ttl=10.0
    )

    views = build_resource_views(store, frozenset({"filesystem"}), now=1.0)

    assert views == (
        LeaseView(
            resource_type="filesystem",
            identifier="/workspace/output.txt",
            owner="owner-a",
            access_mode="write",
            epoch=lease.epoch,
            expired=False,
            stale_warning=None,
        ),
    )


def test_active_unexpired_reader_lease_produces_read_view(tmp_path: Path):
    store = LeaseStore(tmp_path)
    lease = leases.acquire(
        store, "compute-slot", "slot-1", "owner-b", now=0.0, ttl=10.0, access_mode="read"
    )

    views = build_resource_views(store, frozenset({"compute-slot"}), now=1.0)

    assert views == (
        LeaseView(
            resource_type="compute-slot",
            identifier="slot-1",
            owner="owner-b",
            access_mode="read",
            epoch=lease.epoch,
            expired=False,
            stale_warning=None,
        ),
    )


def test_expired_but_still_active_writer_lease_produces_stale_warning(tmp_path: Path):
    store = LeaseStore(tmp_path)
    leases.acquire(store, "filesystem", "/workspace/output.txt", "owner-a", now=0.0, ttl=0.1)

    views = build_resource_views(store, frozenset({"filesystem"}), now=5.0)

    assert len(views) == 1
    view = views[0]
    assert view.access_mode == "write"
    assert view.expired is True
    assert view.stale_warning is not None
    assert "/workspace/output.txt" in view.stale_warning


def test_expired_but_still_active_reader_lease_produces_stale_warning(tmp_path: Path):
    store = LeaseStore(tmp_path)
    leases.acquire(
        store, "compute-slot", "slot-1", "owner-b", now=0.0, ttl=0.1, access_mode="read"
    )

    views = build_resource_views(store, frozenset({"compute-slot"}), now=5.0)

    assert len(views) == 1
    view = views[0]
    assert view.access_mode == "read"
    assert view.expired is True
    assert view.stale_warning is not None
    assert "slot-1" in view.stale_warning


def test_released_writer_lease_produces_no_view(tmp_path: Path):
    store = LeaseStore(tmp_path)
    leases.acquire(store, "filesystem", "/workspace/output.txt", "owner-a", now=0.0, ttl=10.0)
    leases.release(store, "filesystem", "/workspace/output.txt", "owner-a", 0)

    views = build_resource_views(store, frozenset({"filesystem"}), now=1.0)

    assert views == ()


def test_released_reader_lease_produces_no_view(tmp_path: Path):
    store = LeaseStore(tmp_path)
    leases.acquire(
        store, "compute-slot", "slot-1", "owner-b", now=0.0, ttl=10.0, access_mode="read"
    )
    leases.release(store, "compute-slot", "slot-1", "owner-b", 0, access_mode="read")

    views = build_resource_views(store, frozenset({"compute-slot"}), now=1.0)

    assert views == ()
