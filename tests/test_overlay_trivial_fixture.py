"""Trivial non-development overlay fixture.

`overlays.trivial` is a deliberately non-software-development-shaped overlay
(a two-step draft-then-publish content pipeline) whose only purpose is to
prove the overlay contract (`praxis_overlay`) is generic rather than
development-shaped by accident. This suite pins two things: (1) a fresh
`OverlayRegistry` accepts this overlay and the `development` overlay side by
side with no namespace collision -- the concrete proof for the "genuinely
generic" acceptance criterion -- and (2) `build_trivial_graph()` actually
runs to `TERMINAL_SUCCESS` through the same public
`TransitionEngine`/`FakeExecutor` surface every other overlay uses.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import proof_record_to_document
from praxis_overlay.manifest import OverlayManifest
from praxis_overlay.registry import OverlayRegistry
from praxis_runtime.events import EventLog
from praxis_runtime.state import RunStateStore
from praxis_runtime.testing.fake_executor import FakeExecutor
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

from overlays.trivial.overlay import (
    TRIVIAL_MANIFEST,
    build_trivial_grader_registry,
    build_trivial_graph,
    register_trivial_overlay,
)


def test_trivial_manifest_declares_expected_namespace_and_vocabulary():
    assert isinstance(TRIVIAL_MANIFEST, OverlayManifest)
    assert TRIVIAL_MANIFEST.namespace == "trivial"
    assert TRIVIAL_MANIFEST.declares.proof_types == ["trivial.quality-check"]
    assert TRIVIAL_MANIFEST.declares.resource_types == ["trivial.dataset"]
    assert TRIVIAL_MANIFEST.declares.capability_kinds == ["trivial.content-generation"]
    assert TRIVIAL_MANIFEST.requested_capability_kinds == ["trivial.content-generation"]


def test_trivial_graph_is_a_two_node_linear_pipeline_with_one_terminal_node():
    graph = build_trivial_graph()

    assert len(graph.nodes) == 2
    assert len(graph.terminal_nodes) == 1


def test_trivial_and_development_overlays_coexist_in_the_same_registry():
    # Imported lazily (not at module scope) so this test's outcome depends
    # only on the `development` overlay's own readiness, never blocking
    # collection of this file's other, self-contained trivial-only tests.
    from overlays.development.overlay import register_development_overlay

    registry = OverlayRegistry()

    development_activated = register_development_overlay(registry)
    trivial_activated = register_trivial_overlay(registry)

    assert development_activated.manifest.namespace == "development"
    assert trivial_activated.manifest.namespace == "trivial"
    assert registry.namespaces() == frozenset({"development", "trivial"})


def test_trivial_graph_reaches_terminal_success_via_fake_executor(tmp_path: Path):
    graph = build_trivial_graph()
    grader_registry = build_trivial_grader_registry()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=grader_registry)

    (terminal_node_id,) = graph.terminal_nodes
    passing_proof = proof_record_to_document(
        build_proof_record(
            run_id="trivial-fixture-run",
            graph_version=graph.spec_version,
            node_id=terminal_node_id,
            proof_type="trivial.quality-check",
            executor_id="trivial-fixture-executor",
            grader_kind="deterministic",
            status="pass",
        )
    )
    script = {
        node_id: {
            "event_type": "complete",
            "evidence": [passing_proof] if node_id == terminal_node_id else None,
        }
        for node_id in graph.nodes
    }

    final_state = FakeExecutor(engine, script).run_to_completion()

    for node_id in graph.nodes:
        assert final_state.cursors[node_id].status == NodeStatus.TERMINAL_SUCCESS.value


def test_trivial_graph_evidence_gate_rejects_failing_quality_check_proof(tmp_path: Path):
    graph = build_trivial_graph()
    grader_registry = build_trivial_grader_registry()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=grader_registry)

    (terminal_node_id,) = graph.terminal_nodes
    failing_proof = proof_record_to_document(
        build_proof_record(
            run_id="trivial-fixture-run",
            graph_version=graph.spec_version,
            node_id=terminal_node_id,
            proof_type="trivial.quality-check",
            executor_id="trivial-fixture-executor",
            grader_kind="deterministic",
            status="fail",
        )
    )
    script = {
        node_id: {
            "event_type": "complete",
            "evidence": [failing_proof] if node_id == terminal_node_id else None,
        }
        for node_id in graph.nodes
    }

    with pytest.raises(TransitionError):
        FakeExecutor(engine, script).run_to_completion()
