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

import time
from pathlib import Path

import pytest

from conftest import _linear_graph
from praxis_runtime import transitions
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Graph, Node
from praxis_runtime.resources import leases
from praxis_runtime.resources.leases import LeaseStore
from praxis_runtime.resources.policy import ResourceAccessPolicy
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

RESOURCE_TYPE = "filesystem"
IDENTIFIER = "/workspace/output.txt"
UNDECLARED_IDENTIFIER = "/workspace/undeclared.txt"

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

OBSERVED_UNDECLARED_WRITE = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": RESOURCE_TYPE,
            "quantity": 1,
            "identifier": UNDECLARED_IDENTIFIER,
            "access_mode": "write",
        }
    ],
}

FILESYSTEM_READ_CLAIM = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": RESOURCE_TYPE,
            "quantity": 1,
            "identifier": IDENTIFIER,
            "access_mode": "read",
        }
    ],
}

FOOTPRINT_GLOB_WRITE_CLAIM = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": RESOURCE_TYPE,
            "quantity": 1,
            "identifier": "src/a/**",
            "access_mode": "write",
        }
    ],
}

OVERLAPPING_FILE_WRITE_CLAIM = {
    "spec_version": "1.0.0",
    "claims": [
        {
            "resource_type": RESOURCE_TYPE,
            "quantity": 1,
            "identifier": "src/a/file.py",
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


def test_evidence_rejection_does_not_release_lease_before_raising(tmp_path: Path):
    graph = Graph(
        spec_version="1.0.0",
        nodes={
            "n1": Node(
                id="n1",
                kind="task",
                metadata={
                    "resource_claims": FILESYSTEM_WRITE_CLAIM,
                    "evidence_requirement": {
                        "spec_version": "1.0.0",
                        "evidence": [
                            {"proof_type": "signoff", "constraint": "required"},
                        ],
                    },
                },
            )
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )
    lease_store = LeaseStore(tmp_path / "leases")
    engine = TransitionEngine(
        graph,
        RunStateStore(tmp_path / "run-state.json"),
        EventLog(tmp_path / "events"),
        resource_lease_store=lease_store,
    )

    engine.apply("n1", "start")

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete")  # no evidence supplied -> evidence gate rejects

    lease = lease_store.load(RESOURCE_TYPE, IDENTIFIER)
    assert lease is not None
    assert lease.status == "active"
    assert lease.owner == "n1"


def test_dynamic_grant_of_observed_resource_is_released_after_node_terminates(
    tmp_path: Path,
):
    lease_dir = tmp_path / "leases"

    graph_one = _single_node_graph("n1", None)
    graph_one.nodes["n1"].metadata["observed_resources"] = OBSERVED_UNDECLARED_WRITE
    engine_one = TransitionEngine(
        graph_one,
        RunStateStore(tmp_path / "run-one.json"),
        EventLog(tmp_path / "events-one"),
        resource_lease_store=LeaseStore(lease_dir),
        resource_policy=ResourceAccessPolicy.DYNAMIC,
    )
    engine_one.apply("n1", "start")
    engine_one.apply("n1", "complete")

    # n1's dynamic grant must be revalidated and released as part of the
    # same terminal transition that recorded it -- otherwise it leaks a hold
    # on the resource for up to resource_ttl seconds after n1 has already
    # terminated.
    settled = LeaseStore(lease_dir).load(RESOURCE_TYPE, UNDECLARED_IDENTIFIER)
    assert settled is not None
    assert settled.status == "released"

    # A second, independent scheduler observing the same undeclared resource
    # after n1 has fully released it must be able to dynamically acquire it
    # too, since the hold has not leaked past n1's own completion.
    graph_two = _single_node_graph("n2", None)
    graph_two.nodes["n2"].metadata["observed_resources"] = OBSERVED_UNDECLARED_WRITE
    engine_two = TransitionEngine(
        graph_two,
        RunStateStore(tmp_path / "run-two.json"),
        EventLog(tmp_path / "events-two"),
        resource_lease_store=LeaseStore(lease_dir),
        resource_policy=ResourceAccessPolicy.DYNAMIC,
    )
    engine_two.apply("n2", "start")
    engine_two.apply("n2", "complete")

    reacquired = LeaseStore(lease_dir).load(RESOURCE_TYPE, UNDECLARED_IDENTIFIER)
    assert reacquired.owner == "n2"
    assert reacquired.status == "released"


def test_block_transition_does_not_parse_resource_claims_document(
    tmp_path: Path, monkeypatch
):
    graph = _single_node_graph("n1", FILESYSTEM_WRITE_CLAIM)
    engine = TransitionEngine(
        graph,
        RunStateStore(tmp_path / "run-state.json"),
        EventLog(tmp_path / "events"),
        resource_lease_store=LeaseStore(tmp_path / "leases"),
    )
    engine.apply("n1", "start")

    original_parse_claims = transitions.claims.parse_claims
    calls = []

    def spy(document):
        calls.append(document)
        return original_parse_claims(document)

    monkeypatch.setattr(transitions.claims, "parse_claims", spy)

    engine.apply("n1", "block")

    assert calls == []


def test_settle_detects_lease_moved_to_new_generation_since_acquire(tmp_path: Path):
    lease_dir = tmp_path / "leases"
    graph = _single_node_graph("n1", FILESYSTEM_WRITE_CLAIM)
    engine = TransitionEngine(
        graph,
        RunStateStore(tmp_path / "run-state.json"),
        EventLog(tmp_path / "events"),
        resource_lease_store=LeaseStore(lease_dir),
    )

    engine.apply("n1", "start")  # acquires epoch 0

    # Simulate the lease moving to a new generation between acquire and
    # settle -- e.g. it expired and was silently re-acquired by the same
    # owner id (a retried/restarted worker) -- bumping epoch to 1 out from
    # under the engine that is still holding onto epoch 0. The replacement
    # lease is deliberately kept unexpired (far-future deadline) relative to
    # real wall-clock time, so a failure here can only come from the epoch
    # fence, not from an incidental expiry.
    now = time.time()
    store = LeaseStore(lease_dir)
    store.save(
        leases.Lease(
            resource_type=RESOURCE_TYPE,
            identifier=IDENTIFIER,
            owner="n1",
            epoch=0,
            heartbeat_deadline=now - 1.0,
            status="active",
        )
    )
    leases.acquire(store, RESOURCE_TYPE, IDENTIFIER, owner="n1", now=now, ttl=3600.0)

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete")


def test_acquire_rollback_failure_is_wrapped_in_transition_error(
    tmp_path: Path, monkeypatch
):
    # Two declared claims: the first acquires fine, the second is already
    # held by another owner so its acquire fails and the engine must roll
    # back (release) the first. If that rollback release() itself raises
    # LeaseError, callers must still see a single TransitionError -- not a
    # bare LeaseError leaking out of the fail-closed acquire path.
    second_identifier = "/workspace/second.txt"
    two_claim_doc = {
        "spec_version": "1.0.0",
        "claims": [
            {
                "resource_type": RESOURCE_TYPE,
                "quantity": 1,
                "identifier": IDENTIFIER,
                "access_mode": "write",
            },
            {
                "resource_type": RESOURCE_TYPE,
                "quantity": 1,
                "identifier": second_identifier,
                "access_mode": "write",
            },
        ],
    }
    lease_dir = tmp_path / "leases"
    # The engine acquires against real wall-clock time internally, so this
    # pre-held lease must be unexpired relative to time.time(), not 0.0.
    leases.acquire(
        LeaseStore(lease_dir),
        RESOURCE_TYPE,
        second_identifier,
        "other-owner",
        now=time.time(),
        ttl=3600.0,
    )

    graph = _single_node_graph("n1", two_claim_doc)
    engine = TransitionEngine(
        graph,
        RunStateStore(tmp_path / "run-state.json"),
        EventLog(tmp_path / "events"),
        resource_lease_store=LeaseStore(lease_dir),
    )

    def failing_release(*args, **kwargs):
        raise leases.LeaseError("rollback failure")

    monkeypatch.setattr(transitions.leases, "release", failing_release)

    with pytest.raises(TransitionError):
        engine.apply("n1", "start")


def test_two_compatible_read_claims_can_start_concurrently(tmp_path: Path):
    # Repair finding (Critical): leases.acquire is access_mode-blind, so two
    # nodes each declaring a compatible READ claim on the same identifier
    # could not both start concurrently through TransitionEngine, even
    # though claims_conflict says READ-READ never conflicts.
    lease_dir = tmp_path / "leases"

    graph_one = _single_node_graph("n1", FILESYSTEM_READ_CLAIM)
    engine_one = TransitionEngine(
        graph_one,
        RunStateStore(tmp_path / "run-one.json"),
        EventLog(tmp_path / "events-one"),
        resource_lease_store=LeaseStore(lease_dir),
    )
    engine_one.apply("n1", "start")

    graph_two = _single_node_graph("n2", FILESYSTEM_READ_CLAIM)
    engine_two = TransitionEngine(
        graph_two,
        RunStateStore(tmp_path / "run-two.json"),
        EventLog(tmp_path / "events-two"),
        resource_lease_store=LeaseStore(lease_dir),
    )

    # Must not raise: both READ claims are compatible and must be granted.
    engine_two.apply("n2", "start")


def test_overlapping_filesystem_globs_conflict_through_transition_engine(tmp_path: Path):
    # Repair finding (Critical): lease matching is exact-identifier-only, so
    # overlapping-but-non-identical filesystem footprint claims (e.g.
    # "src/a/**" and "src/a/file.py") were not detected as conflicting by
    # the engine's lease-based gating, even though the filesystem adapter's
    # own paths_overlap/footprint_conflict correctly identifies them as
    # overlapping.
    lease_dir = tmp_path / "leases"

    graph_one = _single_node_graph("n1", FOOTPRINT_GLOB_WRITE_CLAIM)
    engine_one = TransitionEngine(
        graph_one,
        RunStateStore(tmp_path / "run-one.json"),
        EventLog(tmp_path / "events-one"),
        resource_lease_store=LeaseStore(lease_dir),
    )
    engine_one.apply("n1", "start")

    graph_two = _single_node_graph("n2", OVERLAPPING_FILE_WRITE_CLAIM)
    engine_two = TransitionEngine(
        graph_two,
        RunStateStore(tmp_path / "run-two.json"),
        EventLog(tmp_path / "events-two"),
        resource_lease_store=LeaseStore(lease_dir),
    )

    with pytest.raises(TransitionError):
        engine_two.apply("n2", "start")


def test_failed_declared_claim_settlement_leaves_no_phantom_observed_resource_lease(
    tmp_path: Path,
):
    # Repair finding (Minor): _authorize_observed_resources performed the
    # dynamic observed-resource lease acquire+revalidate+release before
    # _settle_resource_claims validated the node's declared claims in the
    # same terminal-transition check. If settlement subsequently raised
    # TransitionError (e.g. a stale epoch), the dynamic lease acquire/release
    # had already persisted with no corresponding committed event -- a
    # phantom write for a transition that never officially happened.
    lease_dir = tmp_path / "leases"
    graph = _single_node_graph("n1", FILESYSTEM_WRITE_CLAIM)
    graph.nodes["n1"].metadata["observed_resources"] = OBSERVED_UNDECLARED_WRITE
    engine = TransitionEngine(
        graph,
        RunStateStore(tmp_path / "run-state.json"),
        EventLog(tmp_path / "events"),
        resource_lease_store=LeaseStore(lease_dir),
        resource_policy=ResourceAccessPolicy.DYNAMIC,
    )

    engine.apply("n1", "start")  # acquires epoch 0 for the declared claim

    # Simulate the declared claim's lease moving to a new generation between
    # acquire and settle, so settlement's revalidate will fail with a
    # stale-epoch LeaseError when "complete" is applied.
    now = time.time()
    store = LeaseStore(lease_dir)
    store.save(
        leases.Lease(
            resource_type=RESOURCE_TYPE,
            identifier=IDENTIFIER,
            owner="n1",
            epoch=0,
            heartbeat_deadline=now - 1.0,
            status="active",
        )
    )
    leases.acquire(store, RESOURCE_TYPE, IDENTIFIER, owner="n1", now=now, ttl=3600.0)

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete")

    # The dynamically observed (undeclared) resource must never have been
    # touched -- the declared claim's settlement failure must be detected
    # before any dynamic-grant lease acquire/release side effect runs.
    assert LeaseStore(lease_dir).load(RESOURCE_TYPE, UNDECLARED_IDENTIFIER) is None


def test_completing_with_undeclared_observed_resource_raises_under_strict_policy(
    tmp_path: Path,
):
    graph = Graph(
        spec_version="1.0.0",
        nodes={
            "n1": Node(
                id="n1",
                kind="task",
                metadata={
                    "resource_claims": FILESYSTEM_WRITE_CLAIM,
                    "observed_resources": OBSERVED_UNDECLARED_WRITE,
                },
            )
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )
    engine = TransitionEngine(
        graph,
        RunStateStore(tmp_path / "run-state.json"),
        EventLog(tmp_path / "events"),
        resource_lease_store=LeaseStore(tmp_path / "leases"),
    )

    engine.apply("n1", "start")

    with pytest.raises(TransitionError):
        engine.apply("n1", "complete")
