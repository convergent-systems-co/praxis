"""Regression tests for repair-findings.md (bundle b6-issue7).

Each test reproduces one finding before its fix and must pass after it:

1. (Critical) `leases.acquire`'s default `conflict_fn` was plain identifier
   equality; combined with `transitions.py`'s `_lease_conflict_fn` only
   special-casing `"filesystem"` (whose own `paths_overlap` already treats
   `"*"` as overlapping everything), a workspace-wide fallback claim
   (`identifier="*"`) did not block a conflicting narrower claim of the same
   `resource_type` through the real `TransitionEngine`/`LeaseStore`
   enforcement path for any non-filesystem resource type.
2. (Important) `docs/resources.md`'s documented signatures for
   `leases.acquire`/`renew`/`release`/`revalidate` omitted the actual
   `access_mode`/`conflict_fn` parameters, and its `LeaseStore` section
   omitted `load_reader`/`active_writer_leases`/`active_reader_leases`/
   `save(reader=)`/`lock()`.
3. (Important) `footprint_conflict`/`plan_footprint_claims`/
   `new_footprint_scheduler` in
   `src/praxis_runtime/resources/adapters/filesystem.py` were undocumented
   in `docs/resources.md`'s filesystem-adapter section.
4. (Minor) `TransitionEngine._acquired_epoch` did a full linear scan of the
   event log per declared claim on every terminal transition, instead of
   scanning the node's own "start" event once per transition.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_runtime.events import Event, EventLog
from praxis_runtime.graph import Graph, Node
from praxis_runtime.resources.leases import LeaseError, LeaseStore, acquire
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import TransitionEngine, TransitionError

REPO_ROOT = Path(__file__).resolve().parent.parent
RESOURCES_DOC_PATH = REPO_ROOT / "docs" / "resources.md"


def _single_node_graph(node_id: str, resource_claims: dict) -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={node_id: Node(id=node_id, kind="task", metadata={"resource_claims": resource_claims})},
        edges=[],
        entry_node=node_id,
        terminal_nodes={node_id},
    )


def test_wildcard_identifier_blocks_narrower_claim_via_default_conflict_fn(tmp_path: Path):
    store = LeaseStore(tmp_path)

    acquire(store, "compute-slot", "*", "owner-a", now=0.0, ttl=10.0)

    with pytest.raises(LeaseError):
        acquire(store, "compute-slot", "gpu-1", "owner-b", now=0.0, ttl=10.0)


def test_wildcard_fallback_claim_blocks_conflicting_claim_for_non_filesystem_type_via_transition_engine(
    tmp_path: Path,
):
    lease_dir = tmp_path / "leases"
    wildcard_claim = {
        "spec_version": "1.0.0",
        "claims": [
            {
                "resource_type": "compute-slot",
                "quantity": 1,
                "identifier": "*",
                "access_mode": "write",
            }
        ],
    }
    narrower_claim = {
        "spec_version": "1.0.0",
        "claims": [
            {
                "resource_type": "compute-slot",
                "quantity": 1,
                "identifier": "gpu-1",
                "access_mode": "write",
            }
        ],
    }

    graph_one = _single_node_graph("n1", wildcard_claim)
    engine_one = TransitionEngine(
        graph_one,
        RunStateStore(tmp_path / "run-one.json"),
        EventLog(tmp_path / "events-one"),
        resource_lease_store=LeaseStore(lease_dir),
    )
    engine_one.apply("n1", "start")

    graph_two = _single_node_graph("n2", narrower_claim)
    engine_two = TransitionEngine(
        graph_two,
        RunStateStore(tmp_path / "run-two.json"),
        EventLog(tmp_path / "events-two"),
        resource_lease_store=LeaseStore(lease_dir),
    )

    with pytest.raises(TransitionError):
        engine_two.apply("n2", "start")


def test_resources_doc_documents_lease_access_mode_and_conflict_fn_parameters():
    text = RESOURCES_DOC_PATH.read_text()

    assert "access_mode" in text and "conflict_fn" in text, (
        "docs/resources.md must document acquire/renew/release/revalidate's "
        "actual access_mode and conflict_fn parameters"
    )
    for member in ("load_reader", "active_writer_leases", "active_reader_leases", "lock("):
        assert member in text, (
            f"docs/resources.md's LeaseStore section must document {member!r}, "
            "which exists in the landed code"
        )
    assert "reader=" in text or "reader: bool" in text, (
        "docs/resources.md must document LeaseStore.save's reader= parameter"
    )


def test_resources_doc_documents_footprint_conflict_helpers():
    text = RESOURCES_DOC_PATH.read_text()

    for name in ("footprint_conflict", "plan_footprint_claims", "new_footprint_scheduler"):
        assert name in text, (
            f"docs/resources.md's filesystem-adapter section must document "
            f"{name!r}, which is exported from "
            "src/praxis_runtime/resources/adapters/filesystem.py"
        )


def test_terminal_settlement_reads_event_log_once_regardless_of_declared_claim_count(
    tmp_path: Path, monkeypatch
):
    three_claim_doc = {
        "spec_version": "1.0.0",
        "claims": [
            {
                "resource_type": "filesystem",
                "quantity": 1,
                "identifier": f"/workspace/f{i}.txt",
                "access_mode": "write",
            }
            for i in range(3)
        ],
    }
    graph = _single_node_graph("n1", three_claim_doc)
    engine = TransitionEngine(
        graph,
        RunStateStore(tmp_path / "run-state.json"),
        EventLog(tmp_path / "events"),
        resource_lease_store=LeaseStore(tmp_path / "leases"),
    )
    engine.apply("n1", "start")

    call_count = 0
    original_read_all = EventLog.read_all

    def counting_read_all(self):
        nonlocal call_count
        call_count += 1
        return original_read_all(self)

    monkeypatch.setattr(EventLog, "read_all", counting_read_all)

    engine.apply("n1", "complete")

    assert call_count <= 2, (
        f"settling a terminal transition with 3 declared claims called "
        f"EventLog.read_all() {call_count} times -- _acquired_epoch must scan "
        "the event log once per terminal transition (to find the node's own "
        "'start' event), not once per declared claim per revalidate/release "
        "call"
    )
