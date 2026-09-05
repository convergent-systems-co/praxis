"""GraderRegistry behavior.

Registers a deterministic grader for a made-up proof_type entirely from this
test file, proving overlay extension needs no core-file edit: nothing in
src/praxis_evidence/graders.py knows about "overlay-custom-check".
"""

from __future__ import annotations

import pytest

from praxis_evidence.graders import GraderRegistry, default_registry
from praxis_evidence.types import GradeResult, ProofRecord

_SPEC_VERSION = "1.0.0"


def _proof_record(proof_type: str = "overlay-custom-check") -> ProofRecord:
    return ProofRecord(
        spec_version=_SPEC_VERSION,
        proof_id="proof-1",
        run_id="run-1",
        graph_version=_SPEC_VERSION,
        node_id="n1",
        proof_type=proof_type,
        executor_id="executor-1",
        grader_kind="deterministic",
        status="pass",
    )


class _OverlayCustomCheckGrader:
    """A deterministic grader for an overlay-only proof_type, defined only in this test."""

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=record.status,
            confidence=None,
            grader_kind="deterministic",
            advisory=False,
        )


class _OverlayCustomCheckModelGrader:
    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=record.status,
            confidence=0.9,
            grader_kind="model",
            advisory=True,
        )


def test_default_registry_returns_fresh_empty_registry():
    registry = default_registry()

    assert isinstance(registry, GraderRegistry)
    assert registry.kinds_for("overlay-custom-check") == set()


def test_default_registry_returns_a_new_instance_each_call():
    first = default_registry()
    second = default_registry()

    first.register("overlay-custom-check", "deterministic", _OverlayCustomCheckGrader())

    assert second.kinds_for("overlay-custom-check") == set()


def test_register_and_get_roundtrip_for_overlay_proof_type():
    registry = default_registry()
    grader = _OverlayCustomCheckGrader()

    registry.register("overlay-custom-check", "deterministic", grader)

    assert registry.get("overlay-custom-check", "deterministic") is grader


def test_kinds_for_reflects_multiple_registrations_for_same_proof_type():
    registry = default_registry()

    registry.register("overlay-custom-check", "deterministic", _OverlayCustomCheckGrader())
    registry.register("overlay-custom-check", "model", _OverlayCustomCheckModelGrader())

    assert registry.kinds_for("overlay-custom-check") == {"deterministic", "model"}


def test_register_with_invalid_grader_kind_raises_value_error():
    registry = default_registry()

    with pytest.raises(ValueError):
        registry.register("overlay-custom-check", "bogus-kind", _OverlayCustomCheckGrader())


def test_get_for_unregistered_proof_type_returns_none():
    registry = default_registry()

    assert registry.get("nonexistent-proof-type", "deterministic") is None


def test_registered_deterministic_grader_grades_a_proof_record():
    registry = default_registry()
    registry.register("overlay-custom-check", "deterministic", _OverlayCustomCheckGrader())
    grader = registry.get("overlay-custom-check", "deterministic")

    result = grader.grade(_proof_record())

    assert result == GradeResult(
        proof_type="overlay-custom-check",
        status="pass",
        confidence=None,
        grader_kind="deterministic",
        advisory=False,
    )
