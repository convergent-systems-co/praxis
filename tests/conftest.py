"""Shared test fixtures for the praxis_runtime test suite.

`_linear_graph()` is the minimal two-node graph used by test_transitions.py,
test_fail_closed_cases.py, test_checkpoint_resume.py, and
test_repair_findings_b3_issue4.py -- kept here once so those suites import
the same helper instead of each defining their own copy.

`_PassthroughGrader` is the deterministic (or, via its constructor args,
model/human) stand-in grader used by test_evidence_gates.py,
test_transitions.py, test_checkpoint_resume.py, and
test_repair_findings_b3_issue4.py wherever a test wants the grader's verdict
to track whatever each record itself claims, rather than exercising the
grading algorithm -- kept here once for the same reason as `_linear_graph`.
"""

from __future__ import annotations

from praxis_evidence.types import GradeResult, ProofRecord
from praxis_runtime.graph import Edge, Graph, Node

_SPEC_VERSION = "1.0.0"


def _linear_graph() -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={
            "n1": Node(id="n1", kind="task"),
            "n2": Node(id="n2", kind="task"),
        },
        edges=[Edge(source="n1", target="n2", kind="sequential")],
        entry_node="n1",
        terminal_nodes={"n2"},
    )


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
