"""Telemetry records do not invent resource or cost observations."""

from praxis_executors.interface import ExecutionRequest, ExecutionResult, ExecutorStatus
from praxis_executors.registry import ExecutorRegistry
from praxis_executors.adapters.fake import FakeCapabilityExecutor


def test_fake_execution_has_only_observed_wall_and_success_metrics():
    registry = ExecutorRegistry()
    registry.register(
        "reviewer",
        FakeCapabilityExecutor(
            "reviewer",
            [{"spec_version": "1.0.0", "satisfies": [{"kind": "review"}], "auth_transport": "local"}],
            {"review": ExecutionResult(status=ExecutorStatus.SUCCEEDED)},
        ),
    )
    outcome = registry.execute_with_telemetry(
        {"spec_version": "1.0.0", "requirements": [{"promise": {"spec_version": "1.0.0", "kind": "review"}, "constraint": "required"}]},
        ExecutionRequest(promise={"spec_version": "1.0.0", "kind": "review"}),
        run_id="run-1", graph_version="1.0.0", node_id="review-1", node_kind="review", seq=0,
    )
    assert {m.metric for m in outcome.evaluation_record.measurements} == {"wall_seconds", "success"}
    assert all(m.unit != "unknown" for m in outcome.evaluation_record.measurements)
