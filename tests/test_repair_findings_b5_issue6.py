"""Regression tests for repair-findings.md (bundle b5-issue6).

Each test reproduces one finding before its fix and must pass after it:

1. `TransitionEngine._check_evidence` returned immediately when a node had no
   own `evidence_requirement`, before ever checking whether the node is
   reached via one or more `join`-kind incoming edges. A join/fan-in node
   with no evidence_requirement of its own therefore never aggregated its
   upstream branches' gate results, so an unsatisfied upstream gate could
   never block the join transition.
2. `evaluate_gate`'s returned `GateResult.node_id` was inferred from the
   first surviving parsed record (`""` when zero records survive -- all
   stale/malformed, or none submitted), corrupting
   `aggregate_gate_results`'s `"<source_node_id>: <reason>"` traceability
   prefix for a join's upstream sources.
3. `praxis_evidence.types.gate_result_to_document` was exported but had no
   caller anywhere in `src/` or `tests/`.
4. `evaluate_gate`'s module docstring claimed the `min_confidence` check
   only applies "when confidence is present and below it", but the
   implementation also fails closed when the authoritative grade reports no
   confidence at all.
"""

from __future__ import annotations

from pathlib import Path

import pytest

import praxis_evidence.types as types_module
from praxis_evidence import gates as gates_module
from praxis_evidence.gates import evaluate_gate
from praxis_evidence.graders import GraderRegistry
from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import GradeResult, ProofRecord, proof_record_to_document
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Edge, Graph, Node
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

_GRAPH_VERSION = "1.0.0"


class _PassthroughGrader:
    """Mirrors the record's own submitted status/confidence."""

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=record.status,
            confidence=record.confidence,
            grader_kind="deterministic",
            advisory=False,
        )


class _FixedGrader:
    """Always returns the same verdict, ignoring the record's submitted status."""

    def __init__(self, status: str) -> None:
        self._status = status

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=self._status,
            confidence=record.confidence,
            grader_kind="deterministic",
            advisory=False,
        )


def _proof_record(
    proof_type: str, status: str, *, node_id: str, confidence: float | None = None
) -> dict:
    record = build_proof_record(
        run_id="run-1",
        graph_version=_GRAPH_VERSION,
        node_id=node_id,
        proof_type=proof_type,
        executor_id="executor-1",
        grader_kind="deterministic",
        status=status,
        confidence=confidence,
    )
    return proof_record_to_document(record)


def _fan_out_join_graph_upstream_gate_no_join_target_requirement() -> Graph:
    return Graph(
        spec_version="1.0.0",
        nodes={
            "start": Node(id="start", kind="task"),
            "a": Node(
                id="a",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "spec_version": "1.0.0",
                        "evidence": [
                            {"proof_type": "signoff", "constraint": "required"},
                        ],
                    }
                },
            ),
            "b": Node(id="b", kind="task"),
            "end": Node(id="end", kind="task"),  # no evidence_requirement of its own
        },
        edges=[
            Edge(source="start", target="a", kind="fan-out"),
            Edge(source="start", target="b", kind="fan-out"),
            Edge(source="a", target="end", kind="join"),
            Edge(source="b", target="end", kind="join"),
        ],
        entry_node="start",
        terminal_nodes={"end"},
    )


def test_join_with_no_own_requirement_still_aggregates_upstream_gate(tmp_path: Path):
    graph = _fan_out_join_graph_upstream_gate_no_join_target_requirement()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())
    engine = TransitionEngine(graph, store, log, grader_registry=registry)

    engine.apply("start", "start")
    engine.apply("start", "complete")

    engine.apply("a", "start")
    engine.apply("a", "complete", evidence=[_proof_record("signoff", "pass", node_id="a")])

    engine.apply("b", "start")
    engine.apply("b", "complete")

    # "a" legitimately satisfied its own gate when it completed. The
    # registered grader now fails every "signoff" proof -- "end" has no
    # evidence_requirement of its own, but it must still aggregate "a"'s
    # gate result (re-derived fresh from stored evidence) before letting the
    # join advance, exactly as it would if "end" declared its own
    # requirement.
    registry.register("signoff", "deterministic", _FixedGrader("fail"))

    engine.apply("end", "start")

    with pytest.raises(TransitionError):
        engine.apply("end", "complete")

    state = store.load()
    assert state.cursors["end"].status == NodeStatus.RUNNING.value


def test_evaluate_gate_result_node_id_is_explicit_not_inferred_from_records():
    registry = GraderRegistry()
    requirement = {
        "spec_version": _GRAPH_VERSION,
        "evidence": [{"proof_type": "unregistered-check", "constraint": "preferred"}],
    }

    # Zero records submitted -- previously GateResult.node_id was derived
    # from the first surviving parsed record and silently became "" here,
    # corrupting aggregate_gate_results' "<source_node_id>: <reason>"
    # traceability prefix whenever a join source has no surviving evidence.
    result = evaluate_gate(
        requirement, [], node_id="node-x", graph_version=_GRAPH_VERSION, registry=registry
    )

    assert result.node_id == "node-x"


def test_types_module_does_not_export_unused_gate_result_to_document():
    assert not hasattr(types_module, "gate_result_to_document"), (
        "gate_result_to_document had no caller anywhere in src/ or tests/ -- "
        "dead code should be removed rather than left as an unused export"
    )


def test_evaluate_gate_docstring_states_fail_closed_none_confidence_behavior():
    doc = gates_module.evaluate_gate.__doc__
    assert doc, "evaluate_gate must have a docstring"

    lowered = doc.lower()
    assert "is present and below it" not in lowered, (
        "the docstring must not claim the min_confidence check only applies "
        "when confidence is present -- the implementation also fails closed "
        "when the authoritative grade reports no confidence at all"
    )
    assert "absent" in lowered or "none" in lowered, (
        "the docstring must explicitly document the fail-closed behavior "
        "when the authoritative grade's confidence is absent"
    )


def test_missing_confidence_fails_closed_against_min_confidence():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    requirement = {
        "spec_version": _GRAPH_VERSION,
        "evidence": [
            {"proof_type": "test-pass", "constraint": "required", "min_confidence": 0.5}
        ],
    }
    record = _proof_record("test-pass", "pass", node_id="n1")  # no confidence submitted

    result = evaluate_gate(
        requirement, [record], node_id="n1", graph_version=_GRAPH_VERSION, registry=registry
    )

    assert result.satisfied is False
    assert any(reason.startswith("below min_confidence:") for reason in result.reasons)
