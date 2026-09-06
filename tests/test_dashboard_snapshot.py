"""Snapshot assembly for the dashboard.

Exercises build_snapshot/snapshot_to_document via the same real
Graph/RunState/TransitionEngine/Event fixture pattern as
tests/test_end_to_end_fake_executor.py and the sibling test_dashboard_*.py
suites (examples/sample-graph.json driven by
praxis_runtime.testing.fake_executor.FakeExecutor), plus a small
single-node Graph/LeaseStore fixture for the stale-evidence/stale-lease
warning-aggregation case, where a controlled graph_version mismatch and a
short-ttl lease are needed to force both underlying views' stale_warning
fields non-None.
"""

from __future__ import annotations

import json
from pathlib import Path

from conftest import _PassthroughGrader
from praxis_dashboard.snapshot import build_snapshot, snapshot_to_document
from praxis_evidence.graders import GraderRegistry
from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import proof_record_to_document
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Graph, Node, load_graph
from praxis_runtime.resources import leases
from praxis_runtime.resources.leases import LeaseStore
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

SAMPLE_GRAPH_PATH = Path(__file__).resolve().parent.parent / "examples" / "sample-graph.json"
_SPEC_VERSION = "1.0.0"


def _make_engine(tmp_path: Path):
    graph = load_graph(SAMPLE_GRAPH_PATH)
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    return graph, log, engine


def test_mid_run_snapshot_reflects_node_and_run_summary_state(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    engine.apply("intake", "start")
    engine.apply("intake", "complete")
    run_state = engine.current_state()
    events = log.read_all()

    snapshot = build_snapshot(graph, run_state, events, engine)

    intake_view = next(v for v in snapshot.nodes if v.node_id == "intake")
    assert intake_view.status == NodeStatus.TERMINAL_SUCCESS.value
    assert snapshot.run_summary.total_nodes == len(run_state.cursors)
    assert snapshot.run_summary.run_id == run_state.run_id


def test_mode_replay_is_carried_through_verbatim(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    run_state = engine.current_state()
    events = log.read_all()

    snapshot = build_snapshot(graph, run_state, events, engine, mode="replay")

    assert snapshot.mode == "replay"


def test_no_lease_store_yields_empty_resources(tmp_path: Path):
    # Uses the resource-claim-bearing graph (not sample-graph.json, which
    # declares no resource_claims) so collect_resource_types(graph) is
    # non-empty and the `lease_store is not None` guard in build_snapshot is
    # actually exercised: without the guard, build_resource_views would be
    # called with lease_store=None and a non-empty resource_types, raising
    # AttributeError when it dereferences lease_store.
    graph = _single_node_evidence_graph()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log)
    run_state = engine.current_state()
    events = log.read_all()

    snapshot = build_snapshot(graph, run_state, events, engine, lease_store=None)

    assert snapshot.resources == ()


def test_no_advertisements_yields_empty_capabilities(tmp_path: Path):
    graph, log, engine = _make_engine(tmp_path)
    run_state = engine.current_state()
    events = log.read_all()

    snapshot = build_snapshot(graph, run_state, events, engine, advertisements=None)

    assert snapshot.capabilities == ()


def _single_node_evidence_graph(spec_version: str = _SPEC_VERSION) -> Graph:
    return Graph(
        spec_version=spec_version,
        nodes={
            "n1": Node(
                id="n1",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "spec_version": _SPEC_VERSION,
                        "evidence": [{"proof_type": "test-pass", "constraint": "required"}],
                    },
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
                    },
                },
            ),
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )


def test_snapshot_to_document_round_trips_and_collects_dedup_warnings(tmp_path: Path):
    graph = _single_node_evidence_graph()
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())

    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=registry)
    engine.apply("n1", "start")

    record = build_proof_record(
        run_id="run-1",
        graph_version=_SPEC_VERSION,
        node_id="n1",
        proof_type="test-pass",
        executor_id="executor-1",
        grader_kind="deterministic",
        status="pass",
    )
    engine.apply("n1", "complete", evidence=[proof_record_to_document(record)])
    run_state = engine.current_state()
    events = log.read_all()

    lease_store = LeaseStore(tmp_path / "leases")
    leases.acquire(
        lease_store, "filesystem", "/workspace/output.txt", "owner-a", now=0.0, ttl=0.1
    )

    # The graph passed to build_snapshot represents the *current* graph, which
    # may differ from the one live at apply-time (e.g. a dashboard attaching
    # later after the spec was reloaded). Using a different spec_version here
    # -- rather than at apply-time, which the engine's own evidence gate would
    # reject as stale and refuse to transition -- is what forces the
    # dashboard's own stale-evidence detection.
    current_graph = _single_node_evidence_graph(spec_version="0.9.0")

    snapshot = build_snapshot(
        current_graph,
        run_state,
        events,
        engine,
        lease_store=lease_store,
        grader_registry=registry,
    )

    evidence_warning = next(e.stale_warning for e in snapshot.evidence if e.stale_warning)
    lease_warning = next(r.stale_warning for r in snapshot.resources if r.stale_warning)
    assert evidence_warning in snapshot.warnings
    assert lease_warning in snapshot.warnings
    assert len(snapshot.warnings) == len(set(snapshot.warnings))

    document = snapshot_to_document(snapshot)
    round_tripped = json.loads(json.dumps(document))

    assert round_tripped == document
