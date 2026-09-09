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

import json
from pathlib import Path

import pytest

import overlays.development.graph as development_graph_module
from overlays.development import compat
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


def test_graph_module_docstring_does_not_overstate_requirement_enforcement():
    # Pins the code-review finding from bundle b10-issue12: node.metadata["requirement"]
    # is never read by any core module (TransitionEngine/matching/policy only consume
    # evidence_requirement/resource_claims/authority_requirement/budget_requirement/
    # policy_requirement), so the module docstring must not imply it is enforced the
    # same way those fields are.
    doc = " ".join((development_graph_module.__doc__ or "").split())
    assert "requirement" in doc
    assert "declarative metadata only" in doc
    assert "no core module" in doc
    assert "currently reads or enforces it" in doc


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

    for node_id in final_state.cursors:
        assert final_state.cursors[node_id].status == NodeStatus.TERMINAL_SUCCESS.value

    # This run only ever reaches nodes downstream of the task lane's entry
    # node (write_tdd); the bundle lane's bundle_verify/final_review are
    # never scheduled here, so an assertion that repair_bundle/awaiting_human
    # are absent would pass vacuously without exercising the on-failure edge
    # at all. See test_bundle_verify_success_edge_does_not_create_repair_bundle_cursor
    # below for the direct-engine test that actually proves that edge.


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


def _seed_run_state(tmp_path: Path, graph, node_id: str) -> RunStateStore:
    # Seed the checkpoint file RunStateStore.load() will read, bypassing
    # store.save()'s schema validation: a zero-event checkpoint must record
    # last_applied_seq=-1 (current_state()'s own sentinel for "nothing
    # applied yet") to satisfy TransitionEngine._validate_against_log's
    # "not ahead of the log" guard, but run-state.schema.json's
    # last_applied_seq has `"minimum": 0` -- any schema-valid value here
    # would trip that guard on the first apply(), exactly as
    # test_fail_closed_cases.py::test_checkpoint_ahead_of_empty_event_log_raises
    # pins for last_applied_seq=0 against an empty log.
    run_state_path = tmp_path / "run-state.json"
    run_state_path.write_text(
        json.dumps(
            {
                "spec_version": graph.spec_version,
                "run_id": "test-run",
                "cursors": {
                    node_id: {
                        "node_id": node_id,
                        "status": NodeStatus.PENDING.value,
                    }
                },
                "last_applied_seq": -1,
            }
        )
    )
    return RunStateStore(run_state_path)


def test_repair_bundle_failure_edge_reaches_awaiting_human_blocked_status(tmp_path: Path):
    # FakeExecutor cannot be used here: FakeExecutor._TERMINAL_VALUES excludes
    # both BLOCKED and HANDOFF, so it can never terminate once a cursor parks
    # at one of those statuses -- this drives the engine directly instead.
    registry = OverlayRegistry()
    activated = register_development_overlay(registry)
    graph = build_development_graph()

    store = _seed_run_state(tmp_path, graph, "repair_bundle")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=activated.grader_registry)

    engine.apply("repair_bundle", "start")
    # repair_bundle -> awaiting_human is an `on-failure` edge (graph.py): it
    # fires only when repair_bundle itself reaches TERMINAL_FAILED, never on
    # TERMINAL_SUCCESS -- drive it to "fail" here, not "complete".
    state = engine.apply("repair_bundle", "fail")

    assert state.cursors["repair_bundle"].status == NodeStatus.TERMINAL_FAILED.value
    assert "awaiting_human" in state.cursors
    assert state.cursors["awaiting_human"].status == NodeStatus.PENDING.value

    engine.apply("awaiting_human", "start")
    state = engine.apply("awaiting_human", "block")

    assert state.cursors["awaiting_human"].status == NodeStatus.BLOCKED.value
    assert compat.legacy_status_to_node_status("waiting_human") == NodeStatus.BLOCKED


def test_bundle_verify_failure_edge_reaches_repair_bundle_pending_status(tmp_path: Path):
    # Mirrors the repair_bundle failure-edge test above, but one hop
    # upstream: bundle_verify -> repair_bundle is also an `on-failure` edge
    # (graph.py), so a bundle_verify run to TERMINAL_FAILED must create a
    # repair_bundle cursor, PENDING, and a TERMINAL_SUCCESS run must not.
    registry = OverlayRegistry()
    activated = register_development_overlay(registry)
    graph = build_development_graph()

    store = _seed_run_state(tmp_path, graph, "bundle_verify")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=activated.grader_registry)

    engine.apply("bundle_verify", "start")
    state = engine.apply("bundle_verify", "fail")

    assert state.cursors["bundle_verify"].status == NodeStatus.TERMINAL_FAILED.value
    assert "repair_bundle" in state.cursors
    assert state.cursors["repair_bundle"].status == NodeStatus.PENDING.value


def test_bundle_verify_success_edge_does_not_create_repair_bundle_cursor(tmp_path: Path):
    # Mirrors the failure-edge test above, but drives bundle_verify to
    # TERMINAL_SUCCESS instead: the on-failure edge to repair_bundle (graph.py)
    # must fire only on TERMINAL_FAILED, never on TERMINAL_SUCCESS. Unlike
    # test_development_graph_reaches_terminal_success_with_passing_evidence's
    # FakeExecutor run (which never schedules bundle_verify at all, since it
    # only reaches nodes downstream of the task lane's write_tdd entry node),
    # this seeds bundle_verify directly so the edge is genuinely exercised.
    registry = OverlayRegistry()
    activated = register_development_overlay(registry)
    graph = build_development_graph()

    store = _seed_run_state(tmp_path, graph, "bundle_verify")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(graph, store, log, grader_registry=activated.grader_registry)

    engine.apply("bundle_verify", "start")
    state = engine.apply("bundle_verify", "complete")

    assert state.cursors["bundle_verify"].status == NodeStatus.TERMINAL_SUCCESS.value
    assert "final_review" in state.cursors
    assert "repair_bundle" not in state.cursors
