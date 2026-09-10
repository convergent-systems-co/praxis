"""Registry timing and non-persistent telemetry integration tests."""

from __future__ import annotations

from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import ExecutionRequest, ExecutionResult, ExecutorStatus
from praxis_executors.registry import ExecutorRegistry


_REQUIREMENT = {
    "spec_version": "1.0.0",
    "requirements": [
        {
            "promise": {"spec_version": "1.0.0", "kind": "document-review"},
            "constraint": "required",
        }
    ],
}
_REQUEST = ExecutionRequest(
    promise={"spec_version": "1.0.0", "kind": "document-review"}
)


def _executor(result: ExecutionResult, *, model: str | None = None):
    entry = {"kind": "document-review"}
    if model is not None:
        entry["parameters"] = {"model": model}
    return FakeCapabilityExecutor(
        executor_id="reviewer",
        capabilities=[
            {
                "spec_version": "1.0.0",
                "satisfies": [entry],
                "auth_transport": "local",
            }
        ],
        script={"document-review": result},
    )


def test_execute_with_telemetry_returns_complete_records_and_model_config():
    registry = ExecutorRegistry()
    registry.register(
        "reviewer",
        _executor(ExecutionResult(status=ExecutorStatus.SUCCEEDED), model="review-v2"),
    )

    outcome = registry.execute_with_telemetry(
        _REQUIREMENT,
        _REQUEST,
        run_id="run-1",
        graph_version="1.0.0",
        node_id="review-1",
        node_kind="review",
        seq=3,
    )

    assert outcome.event_document["event_type"] == "complete"
    assert outcome.telemetry.wall_seconds >= 0
    assert outcome.evaluation_record.measurements[1].value == 1.0
    assert outcome.evaluation_record.candidate_id
    assert outcome.telemetry.model == "review-v2"
    assert outcome.proof_records == []


def test_execute_with_telemetry_failed_run_carries_failure_class_without_cost():
    registry = ExecutorRegistry()
    registry.register(
        "reviewer",
        _executor(ExecutionResult(status=ExecutorStatus.FAILED), model=None),
    )

    outcome = registry.execute_with_telemetry(
        _REQUIREMENT,
        _REQUEST,
        run_id="run-1",
        graph_version="1.0.0",
        node_id="review-1",
        node_kind="review",
        seq=4,
        failure_class="transient",
    )

    assert outcome.event_document["event_type"] == "fail"
    assert outcome.event_document["payload"]["failure_class"] == "transient"
    assert outcome.telemetry.model is None
    assert {measurement.metric for measurement in outcome.evaluation_record.measurements} == {
        "wall_seconds",
        "success",
    }
