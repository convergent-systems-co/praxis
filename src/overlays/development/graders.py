"""Development overlay graders: deterministic graders for
"development.test-pass" and "development.review-approved" built via
`praxis_overlay.evidence.build_namespaced_grader_registry`.

Each grader implements the single-method `Grader` protocol
(docs/evidence.md: `grade(self, record: ProofRecord) -> GradeResult`) and
reads `ProofRecord.status` directly as the authoritative verdict --
deterministic, no inference beyond what the record itself states.
"""

from __future__ import annotations

from praxis_evidence.graders import GraderRegistry
from praxis_evidence.types import GradeResult, ProofRecord
from praxis_overlay.evidence import build_namespaced_grader_registry

from overlays.development.manifest import DEVELOPMENT_MANIFEST

_TEST_PASS = "development.test-pass"
_REVIEW_APPROVED = "development.review-approved"


class _StatusPassthroughGrader:
    """Deterministic grader: grades `record.status` as-is (e.g. "pass" -> "pass")."""

    def grade(self, record: ProofRecord) -> GradeResult:
        return GradeResult(
            proof_type=record.proof_type,
            status=record.status,
            confidence=None,
            grader_kind="deterministic",
            advisory=False,
        )


def build_development_grader_registry() -> GraderRegistry:
    return build_namespaced_grader_registry(
        DEVELOPMENT_MANIFEST,
        {
            (_TEST_PASS, "deterministic"): _StatusPassthroughGrader(),
            (_REVIEW_APPROVED, "deterministic"): _StatusPassthroughGrader(),
        },
    )
