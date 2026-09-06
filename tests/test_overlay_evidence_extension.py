"""Evidence/grader extension point.

build_namespaced_grader_registry builds a fresh praxis_evidence.graders.
GraderRegistry (via default_registry(), which returns a new, empty registry
on every call -- no shared singleton) and registers each supplied grader
under its (proof_type, grader_kind) key, failing closed with
EvidenceExtensionError before any registration happens if a proof_type key
is not present in manifest.declares.proof_types.
"""

from __future__ import annotations

import copy

import pytest

from praxis_evidence.graders import GraderRegistry
from praxis_evidence.types import GradeResult, ProofRecord
from praxis_overlay.evidence import EvidenceExtensionError, build_namespaced_grader_registry
from praxis_overlay.manifest import load_manifest

VALID_MANIFEST_DOCUMENT = {
    "spec_version": "1.0.0",
    "overlay_id": "development-overlay",
    "namespace": "development",
    "version": "0.1.0",
    "description": "Ports the current develop skill's graph/policy semantics onto Praxis.",
    "declares": {
        "capability_kinds": ["development.code-generation", "development.code-review"],
        "proof_types": ["development.test-pass", "development.review-approved"],
        "resource_types": ["development.filesystem"],
        "authority_scopes": ["development.merge-authority"],
    },
    "requested_capability_kinds": [
        "development.code-generation",
        "development.code-review",
    ],
}


class _FakeGrader:
    def __init__(self, status: str = "pass") -> None:
        self._status = status

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=self._status,
            confidence=None,
            grader_kind="deterministic",
            advisory=False,
        )


def _manifest():
    return load_manifest(copy.deepcopy(VALID_MANIFEST_DOCUMENT))


def test_declared_proof_types_build_registry_returning_supplied_graders():
    manifest = _manifest()
    grader = _FakeGrader()

    registry = build_namespaced_grader_registry(
        manifest, {("development.test-pass", "deterministic"): grader}
    )

    assert isinstance(registry, GraderRegistry)
    assert registry.get("development.test-pass", "deterministic") is grader


def test_undeclared_proof_type_raises_evidence_extension_error_before_any_registration():
    manifest = _manifest()
    grader = _FakeGrader()

    with pytest.raises(EvidenceExtensionError) as excinfo:
        build_namespaced_grader_registry(
            manifest,
            {
                ("development.test-pass", "deterministic"): grader,
                ("other.undeclared-proof-type", "deterministic"): grader,
            },
        )

    assert "other.undeclared-proof-type" in str(excinfo.value)

    # No partial registration leaked into a shared default: a second, fully
    # valid call still starts from a clean registry with only its own entry.
    registry = build_namespaced_grader_registry(
        manifest, {("development.review-approved", "deterministic"): grader}
    )
    assert registry.get("development.review-approved", "deterministic") is grader
    assert registry.get("development.test-pass", "deterministic") is None
