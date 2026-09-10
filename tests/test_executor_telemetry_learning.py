"""Telemetry documents feed the existing learning extractor."""

from praxis_executors.interface import ExecutorStatus
from praxis_executors.telemetry import ExecutionTelemetry, build_execution_event_document
from praxis_learning.extraction import extract_observations


def _event(status: str, seq: int, failure_class: str | None = None) -> dict:
    return build_execution_event_document(
        ExecutionTelemetry(
            executor_id="reviewer",
            node_id="review-1",
            node_kind="review",
            status=status,
            wall_seconds=0.5,
            repairs=1,
            verification_passed=True,
            failure_class=failure_class,
        ),
        run_id="run-1",
        seq=seq,
    )


def test_repeated_failures_and_recovery_are_classified():
    recurrent = extract_observations(
        [_event(ExecutorStatus.FAILED.value, 0, "transient"), _event(ExecutorStatus.FAILED.value, 1, "transient")],
        project_id="project-1",
    )
    assert [observation.pattern for observation in recurrent] == ["recurrent-failure"]

    recovered = extract_observations(
        [_event(ExecutorStatus.FAILED.value, 0, "transient"), _event(ExecutorStatus.SUCCEEDED.value, 1)],
        project_id="project-1",
    )
    assert [observation.pattern for observation in recovered] == ["successful-recovery"]
