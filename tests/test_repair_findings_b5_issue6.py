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
5. `praxis_runtime.replay.resume()` constructed `TransitionEngine(graph,
   state_store, event_log)` without forwarding a caller-supplied
   `grader_registry`, so a `TransitionEngine` returned after crash/restart
   silently reverted to an empty registry and lost domain-overlay graders.
6. `docs/evidence.md` documented `evaluate_gate(requirement, records, *,
   graph_version, registry)`, omitting the required keyword-only `node_id`
   parameter the actual function takes.
7. `docs/evidence.md` claimed `gate_result_to_document` converts a
   `GateResult` to its document shape, but that function was already removed
   from `praxis_evidence.types` in a prior repair pass -- the doc referenced
   dead/nonexistent API.
8. `praxis_evidence.types.GATE_RESULT_SCHEMA_PATH` was exported but had no
   caller anywhere in `src/` or `tests/`.
9. `evaluate_gate`'s `"missing: <proof_type>"` reason was appended for a
   `"prohibited"` requirement item with zero submitted records, even though
   absence is that item's desired, passing state, not something to flag.
10. `tests/test_end_to_end_fake_executor.py`'s `_CountingEngine.apply` kept
    a stale `evidence: dict | None` annotation after
    `TransitionEngine.apply`'s `evidence` parameter changed to
    `list[dict] | None`.
11. `evaluate_gate` indexed `requirement["evidence"]` and
    `item["proof_type"]`/`item["constraint"]` directly with no schema
    validation or `.get()` fallback. `graph.schema.json`'s node metadata is
    `additionalProperties: true` (unvalidated), so a malformed
    `evidence_requirement` raised an uncaught `KeyError` out of
    `TransitionEngine.apply()` instead of a fail-closed `TransitionError`.
12. `schemas/v1/gate-result.schema.json` had no validator or consumer
    anywhere in `src/` or `tests/` -- `GateResult` was never constructed from
    or validated against a document, so the dataclass and schema could
    silently drift with no caller ever noticing.
"""

from __future__ import annotations

import typing
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
from praxis_runtime.replay import resume
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine, TransitionError

_DOCS_ROOT = Path(__file__).resolve().parent.parent / "docs"

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


def _evidence_gated_graph_for_resume() -> Graph:
    return Graph(
        spec_version=_GRAPH_VERSION,
        nodes={
            "n1": Node(
                id="n1",
                kind="task",
                metadata={
                    "evidence_requirement": {
                        "spec_version": _GRAPH_VERSION,
                        "evidence": [
                            {"proof_type": "signoff", "constraint": "required"},
                        ],
                    }
                },
            ),
        },
        edges=[],
        entry_node="n1",
        terminal_nodes={"n1"},
    )


def test_resume_forwards_grader_registry_to_returned_engine(tmp_path: Path):
    graph = _evidence_gated_graph_for_resume()
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    registry = GraderRegistry()
    registry.register("signoff", "deterministic", _PassthroughGrader())

    # No prior checkpoint/events -- resume() takes its "no pending events"
    # early-return path and hands back a bare TransitionEngine. Even on that
    # path it must still be constructed with the caller's grader_registry,
    # not default_registry()'s empty one.
    engine = resume(graph, store, log, grader_registry=registry)

    engine.apply("n1", "start")
    evidence = [_proof_record("signoff", "pass", node_id="n1")]

    # If resume() had silently dropped grader_registry, the returned engine
    # would fall back to an empty registry and this would raise
    # TransitionError("no grader registered: signoff") instead of
    # succeeding.
    final_state = engine.apply("n1", "complete", evidence=evidence)

    assert final_state.cursors["n1"].status == NodeStatus.TERMINAL_SUCCESS.value


def _read_evidence_doc() -> str:
    return (_DOCS_ROOT / "evidence.md").read_text()


def test_evidence_doc_evaluate_gate_signature_includes_node_id():
    doc = _read_evidence_doc()
    assert "evaluate_gate(requirement, records, *, node_id, graph_version, registry)" in doc, (
        "docs/evidence.md must document evaluate_gate's actual signature, including "
        "the required keyword-only node_id parameter -- following the old, "
        "node_id-less signature raises TypeError"
    )
    assert "evaluate_gate(requirement, records, *, graph_version, registry)" not in doc


def test_evidence_doc_does_not_reference_deleted_gate_result_to_document():
    doc = _read_evidence_doc()
    assert "gate_result_to_document" not in doc, (
        "gate_result_to_document was removed from praxis_evidence.types in a prior "
        "repair pass -- docs/evidence.md must not reference this dead API"
    )


def test_types_module_does_not_export_unused_gate_result_schema_path():
    assert not hasattr(types_module, "GATE_RESULT_SCHEMA_PATH"), (
        "GATE_RESULT_SCHEMA_PATH had no caller anywhere in src/ or tests/ -- "
        "dead code should be removed rather than left as an unused export"
    )


def test_prohibited_item_with_zero_records_has_no_missing_reason():
    registry = GraderRegistry()
    requirement = {
        "spec_version": _GRAPH_VERSION,
        "evidence": [
            {"proof_type": "banned-check", "constraint": "prohibited"},
        ],
    }

    # No records submitted at all for "banned-check" -- absence is exactly
    # what a "prohibited" constraint wants, so this must satisfy cleanly
    # with no "missing: banned-check" reason muddying the audit trail.
    result = evaluate_gate(
        requirement, [], node_id="n1", graph_version=_GRAPH_VERSION, registry=registry
    )

    assert result.satisfied is True
    assert not any(reason.startswith("missing:") for reason in result.reasons)


def test_counting_engine_apply_annotation_matches_transition_engine_apply():
    import test_end_to_end_fake_executor as e2e_module

    hints = typing.get_type_hints(e2e_module._CountingEngine.apply)
    assert hints["evidence"] == (list[dict] | None), (
        "_CountingEngine.apply's evidence annotation is stale relative to "
        "TransitionEngine.apply's evidence: list[dict] | None signature"
    )


def test_gate_result_schema_file_removed_as_unused():
    schema_path = Path(__file__).resolve().parent.parent / "schemas" / "v1" / "gate-result.schema.json"
    assert not schema_path.exists(), (
        "gate-result.schema.json had no validator or consumer anywhere in src/ "
        "or tests/ -- GateResult is never constructed from or validated against "
        "a document, so the unused schema should be removed rather than left to "
        "silently drift from the dataclass it was meant to describe"
    )
