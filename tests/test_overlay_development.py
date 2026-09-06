"""Development overlay: ports the current `develop` skill's graph/policy
semantics onto Praxis through the overlay contract (T1-T4).

This is the concrete proof for acceptance criterion "`develop` can express
its existing graph and policies through the overlay contract": the overlay
registers into a fresh `OverlayRegistry` with no namespace collision, its
`build_development_graph()` linear chain runs to `TERMINAL_SUCCESS` end to
end through the real `TransitionEngine`/`FakeExecutor` surface (mirroring
`test_end_to_end_fake_executor.py`'s convention), and the terminal node's
evidence gate is proven to be genuinely wired in -- not bypassed -- by
showing a failing `development.test-pass` proof record blocks the run with
`TransitionError` via the overlay's own `grader_registry`.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from overlays.development.manifest import DEVELOPMENT_MANIFEST
from overlays.development.graph import build_development_graph
from overlays.development.overlay import register_development_overlay
from praxis_overlay.registry import ActivatedOverlay, OverlayRegistry
from praxis_runtime.events import EventLog
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

_TEST_PASS = "development.test-pass"
_REVIEW_APPROVED = "development.review-approved"


def _proof_record(*, node_id: str, proof_type: str, status: str, graph_version: str) -> dict:
    return {
        "spec_version": "1.0.0",
        "proof_id": f"{node_id}-{proof_type}-{status}",
        "run_id": "test-run",
        "graph_version": graph_version,
        "node_id": node_id,
        "proof_type": proof_type,
        "executor_id": "test-harness",
        "grader_kind": "deterministic",
        "status": status,
    }


def _build_engine(tmp_path: Path, graph, grader_registry) -> TransitionEngine:
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    return TransitionEngine(graph, store, log, grader_registry=grader_registry)


def test_development_manifest_declares_required_vocabulary():
    assert DEVELOPMENT_MANIFEST.namespace == "development"
    assert _TEST_PASS in DEVELOPMENT_MANIFEST.declares.proof_types
    assert _REVIEW_APPROVED in DEVELOPMENT_MANIFEST.declares.proof_types
    assert "development.filesystem" in DEVELOPMENT_MANIFEST.declares.resource_types
    assert "development.code-generation" in DEVELOPMENT_MANIFEST.declares.capability_kinds
    assert "development.code-review" in DEVELOPMENT_MANIFEST.declares.capability_kinds


def test_register_development_overlay_activates_into_fresh_registry():
    registry = OverlayRegistry()

    activated = register_development_overlay(registry)

    assert isinstance(activated, ActivatedOverlay)
    assert activated.manifest.namespace == "development"
    assert registry.namespaces() == frozenset({"development"})
    assert registry.get(activated.manifest.overlay_id) is activated
    assert activated.resource_provider is not None
    assert activated.resource_provider.resource_types() == frozenset({"development.filesystem"})


def test_development_graph_reaches_terminal_success_with_passing_evidence(tmp_path: Path):
    registry = OverlayRegistry()
    activated = register_development_overlay(registry)
    graph = build_development_graph()
    terminal_node_id = next(iter(graph.terminal_nodes))

    engine = _build_engine(tmp_path, graph, activated.grader_registry)
    script = {node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes}
    script[terminal_node_id] = {
        "event_type": "complete",
        "evidence": [
            _proof_record(
                node_id=terminal_node_id,
                proof_type=_TEST_PASS,
                status="pass",
                graph_version=graph.spec_version,
            ),
            _proof_record(
                node_id=terminal_node_id,
                proof_type=_REVIEW_APPROVED,
                status="pass",
                graph_version=graph.spec_version,
            ),
        ],
    }

    final_state = FakeExecutor(engine, script).run_to_completion()

    assert set(final_state.cursors) == set(graph.nodes)
    for node_id in graph.nodes:
        assert final_state.cursors[node_id].status == NodeStatus.TERMINAL_SUCCESS.value


def test_development_graph_evidence_gate_rejects_failing_test_pass_proof(tmp_path: Path):
    registry = OverlayRegistry()
    activated = register_development_overlay(registry)
    graph = build_development_graph()
    terminal_node_id = next(iter(graph.terminal_nodes))

    engine = _build_engine(tmp_path, graph, activated.grader_registry)
    script = {node_id: {"event_type": "complete", "evidence": None} for node_id in graph.nodes}
    script[terminal_node_id] = {
        "event_type": "complete",
        "evidence": [
            _proof_record(
                node_id=terminal_node_id,
                proof_type=_TEST_PASS,
                status="fail",
                graph_version=graph.spec_version,
            ),
            _proof_record(
                node_id=terminal_node_id,
                proof_type=_REVIEW_APPROVED,
                status="pass",
                graph_version=graph.spec_version,
            ),
        ],
    }

    with pytest.raises(TransitionError):
        FakeExecutor(engine, script).run_to_completion()
