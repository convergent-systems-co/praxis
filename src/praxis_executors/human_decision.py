"""Capture an explicit human answer at the policy/evidence boundary.

ADR 0002 keeps a human outside the registered executor set. This module
therefore records only an answer that was actually supplied: it creates a
human-graded proof record and uses the same telemetry shape as machine
executions. It never imports or dispatches into `praxis_runtime`.
"""

from __future__ import annotations

from dataclasses import dataclass

from praxis_evidence.proof import build_proof_record
from praxis_evidence.types import proof_record_to_document

from .interface import ExecutorStatus
from .telemetry import ExecutionTelemetry


@dataclass(frozen=True)
class HumanDecision:
    node_id: str
    verdict: str
    decided_by_executor_id: str
    escalations: int = 1


_VERDICTS = frozenset({"pass", "fail", "inconclusive"})


def build_human_proof_record(
    decision: HumanDecision,
    *,
    run_id: str,
    graph_version: str,
    proof_type: str,
) -> dict:
    """Build one schema-valid human proof record from an explicit verdict."""
    if decision.verdict not in _VERDICTS:
        raise ValueError(
            f"invalid human verdict {decision.verdict!r}; expected one of "
            f"{sorted(_VERDICTS)}"
        )
    return proof_record_to_document(
        build_proof_record(
            run_id=run_id,
            graph_version=graph_version,
            node_id=decision.node_id,
            proof_type=proof_type,
            executor_id=decision.decided_by_executor_id,
            grader_kind="human",
            status=decision.verdict,
        )
    )


def human_decision_telemetry(
    decision: HumanDecision, *, node_kind: str, wall_seconds: float
) -> ExecutionTelemetry:
    """Represent the explicit human answer on the common telemetry path."""
    status = (
        ExecutorStatus.SUCCEEDED.value
        if decision.verdict == "pass"
        else ExecutorStatus.FAILED.value
    )
    return ExecutionTelemetry(
        executor_id=decision.decided_by_executor_id,
        node_id=decision.node_id,
        node_kind=node_kind,
        status=status,
        wall_seconds=wall_seconds,
        human_interrupts=decision.escalations,
    )


def human_denial_events() -> list[str]:
    """Return the existing two-hop transition sequence for a human denial."""
    from praxis_policy.gate import human_denial_event_sequence

    return human_denial_event_sequence()
