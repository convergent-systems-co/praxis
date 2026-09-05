"""Single-node gate evaluation engine (praxis_evidence.gates.evaluate_gate).

Covers the fail-closed grading algorithm: missing/malformed/stale/
contradictory evidence all block; grading is authoritative over whatever
status a submitted record claims (so a submitter can't just assert
status="pass"); a registered deterministic grader always wins over an
advisory model grader; a human-only gate never default-approves; and the
required/preferred/prohibited constraint semantics.
"""

from __future__ import annotations

from praxis_evidence.gates import evaluate_gate
from praxis_evidence.graders import GraderRegistry
from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import GradeResult, ProofRecord, proof_record_to_document

_SPEC_VERSION = "1.0.0"
_GRAPH_VERSION = "1.0.0"


def _requirement(*items: dict) -> dict:
    return {"spec_version": _SPEC_VERSION, "evidence": list(items)}


def _item(proof_type: str, constraint: str, min_confidence: float | None = None) -> dict:
    item = {"proof_type": proof_type, "constraint": constraint}
    if min_confidence is not None:
        item["min_confidence"] = min_confidence
    return item


def _record(
    proof_type: str,
    status: str,
    *,
    grader_kind: str = "deterministic",
    graph_version: str = _GRAPH_VERSION,
    confidence: float | None = None,
) -> dict:
    record = build_proof_record(
        run_id="run-1",
        graph_version=graph_version,
        node_id="n1",
        proof_type=proof_type,
        executor_id="executor-1",
        grader_kind=grader_kind,
        status=status,
        confidence=confidence,
    )
    return proof_record_to_document(record)


class _PassthroughGrader:
    """Mirrors the record's own submitted status/confidence -- used where the
    test wants the grader's verdict to track whatever each record claims."""

    def __init__(self, grader_kind: str = "deterministic", advisory: bool = False) -> None:
        self._grader_kind = grader_kind
        self._advisory = advisory

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=record.status,
            confidence=record.confidence,
            grader_kind=self._grader_kind,
            advisory=self._advisory,
        )


class _FixedGrader:
    """Always returns the same verdict, ignoring the record's submitted
    status -- used to prove grading is authoritative over a submitter's
    claim."""

    def __init__(
        self,
        status: str,
        *,
        grader_kind: str = "deterministic",
        confidence: float | None = None,
        advisory: bool = False,
    ) -> None:
        self._status = status
        self._grader_kind = grader_kind
        self._confidence = confidence
        self._advisory = advisory

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=self._status,
            confidence=self._confidence,
            grader_kind=self._grader_kind,
            advisory=self._advisory,
        )


def test_missing_evidence_for_required_proof_type_blocks():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    requirement = _requirement(_item("test-pass", "required"))

    result = evaluate_gate(requirement, [], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False
    assert "missing: test-pass" in result.reasons
    assert "test-pass" in result.evaluated


def test_malformed_evidence_blocks_and_does_not_count():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    requirement = _requirement(_item("test-pass", "required"))
    malformed = {
        "spec_version": _SPEC_VERSION,
        "proof_id": "proof-1",
        "run_id": "run-1",
        "graph_version": _GRAPH_VERSION,
        "node_id": "n1",
        "proof_type": "test-pass",
        "executor_id": "executor-1",
        "grader_kind": "deterministic",
        # "status" deliberately missing -> fails proof-record schema validation
    }

    result = evaluate_gate(
        requirement, [malformed], graph_version=_GRAPH_VERSION, registry=registry
    )

    assert result.satisfied is False
    assert any(reason.startswith("malformed:") for reason in result.reasons)


def test_stale_evidence_graph_version_mismatch_blocks():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    requirement = _requirement(_item("test-pass", "required"))
    stale = _record("test-pass", "pass", graph_version="0.9.0")

    result = evaluate_gate(requirement, [stale], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False
    assert any(reason.startswith("stale:") for reason in result.reasons)


def test_contradictory_evidence_blocks():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    requirement = _requirement(_item("test-pass", "required"))
    records = [
        _record("test-pass", "pass"),
        _record("test-pass", "fail"),
    ]

    result = evaluate_gate(requirement, records, graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False
    assert any(reason.startswith("contradictory:") for reason in result.reasons)


def test_false_success_deterministic_grading_overrides_submitted_status():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _FixedGrader("fail"))
    requirement = _requirement(_item("test-pass", "required"))
    record = _record("test-pass", "pass")

    result = evaluate_gate(requirement, [record], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False


def test_deterministic_preferred_over_model_advisory():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _FixedGrader("fail"))
    registry.register(
        "test-pass",
        "model",
        _FixedGrader("pass", grader_kind="model", confidence=0.95, advisory=True),
    )
    requirement = _requirement(_item("test-pass", "required"))
    record = _record("test-pass", "pass", grader_kind="model")

    result = evaluate_gate(requirement, [record], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False
    assert any("advisory" in reason.lower() for reason in result.reasons)


def test_human_review_gate_blocks_without_human_record():
    registry = GraderRegistry()
    registry.register("human-review", "human", _PassthroughGrader(grader_kind="human"))
    requirement = _requirement(_item("human-review", "required"))

    result = evaluate_gate(requirement, [], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False


def test_human_review_gate_satisfied_with_passing_human_record():
    registry = GraderRegistry()
    registry.register("human-review", "human", _PassthroughGrader(grader_kind="human"))
    requirement = _requirement(_item("human-review", "required"))
    record = _record("human-review", "pass", grader_kind="human")

    result = evaluate_gate(requirement, [record], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is True


def test_human_review_gate_blocks_when_only_non_human_record_present():
    registry = GraderRegistry()
    registry.register("human-review", "human", _PassthroughGrader(grader_kind="human"))
    requirement = _requirement(_item("human-review", "required"))
    # A non-human record (e.g. submitted by a deterministic/model executor)
    # claiming "pass" must never satisfy a human-only gate -- absence of an
    # actual human record must never be default-approved.
    record = _record("human-review", "pass", grader_kind="deterministic")

    result = evaluate_gate(requirement, [record], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False
    assert "missing human review: human-review" in result.reasons


def test_prohibited_constraint_blocks_when_matching_pass_exists():
    registry = GraderRegistry()
    registry.register("banned-check", "deterministic", _PassthroughGrader())
    requirement = _requirement(_item("banned-check", "prohibited"))
    record = _record("banned-check", "pass")

    result = evaluate_gate(requirement, [record], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False


def test_preferred_constraint_never_blocks_even_when_ungraded():
    registry = GraderRegistry()
    requirement = _requirement(_item("nice-to-have", "preferred"))

    result = evaluate_gate(requirement, [], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is True
    assert "nice-to-have" in result.evaluated


def test_no_grader_registered_blocks_with_reason():
    registry = GraderRegistry()
    requirement = _requirement(_item("unregistered-check", "required"))
    record = _record("unregistered-check", "pass")

    result = evaluate_gate(requirement, [record], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False
    assert "no grader registered: unregistered-check" in result.reasons


def test_below_min_confidence_blocks():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    requirement = _requirement(_item("test-pass", "required", min_confidence=0.9))
    record = _record("test-pass", "pass", confidence=0.5)

    result = evaluate_gate(requirement, [record], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is False
    assert any(reason.startswith("below min_confidence:") for reason in result.reasons)


def test_required_proof_type_satisfied_by_deterministic_pass():
    registry = GraderRegistry()
    registry.register("test-pass", "deterministic", _PassthroughGrader())
    requirement = _requirement(_item("test-pass", "required"))
    record = _record("test-pass", "pass")

    result = evaluate_gate(requirement, [record], graph_version=_GRAPH_VERSION, registry=registry)

    assert result.satisfied is True
    assert tuple(result.evaluated) == ("test-pass",)
