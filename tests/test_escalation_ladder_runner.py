"""Integration tests for the executor-failure escalation runner."""

from __future__ import annotations

from dataclasses import replace
from pathlib import Path

from praxis_executors.adapters.fake import FakeCapabilityExecutor
from praxis_executors.interface import ExecutionRequest, ExecutionResult, ExecutorStatus
from praxis_executors.registry import ExecutorRegistry
from praxis_policy.budgets import BudgetLedger
from praxis_policy.gate import PolicyGate
from praxis_policy.profiles import BUILTIN_PROFILES
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Graph
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine
from praxis_orchestration import build_escalation_ladder, run_escalation_ladder


_SPEC_VERSION = "1.0.0"
_KIND = "document-review"


def _requirement() -> dict:
    return {
        "spec_version": _SPEC_VERSION,
        "requirements": [
            {
                "promise": {"spec_version": _SPEC_VERSION, "kind": _KIND},
                "constraint": "required",
            }
        ],
    }


def _request() -> ExecutionRequest:
    return ExecutionRequest(
        promise={"spec_version": _SPEC_VERSION, "kind": _KIND}
    )


def _reviewer(
    executor_id: str, auth_transport: str, result: ExecutionResult
) -> FakeCapabilityExecutor:
    return FakeCapabilityExecutor(
        executor_id=executor_id,
        capabilities=[
            {
                "spec_version": _SPEC_VERSION,
                "satisfies": [{"kind": _KIND}],
                "auth_transport": auth_transport,
            }
        ],
        script={_KIND: result},
    )


def _harness(tmp_path: Path):
    attempt_ids = ["review-first", "review-second", "review-third"]
    human_id = "review-human"
    nodes, edges = build_escalation_ladder(
        attempt_node_ids=attempt_ids, human_node_id=human_id
    )
    graph = Graph(
        spec_version=_SPEC_VERSION,
        nodes={node.id: node for node in nodes},
        edges=edges,
        entry_node=attempt_ids[0],
        terminal_nodes={human_id},
    )
    event_log = EventLog(tmp_path / "events")
    engine = TransitionEngine(
        graph, RunStateStore(tmp_path / "run-state.json"), event_log
    )
    return attempt_ids, human_id, event_log, engine


def test_success_on_first_rung_records_proof_and_does_not_start_successors(
    tmp_path: Path,
):
    attempt_ids, human_id, event_log, engine = _harness(tmp_path)
    registry = ExecutorRegistry()
    registry.register(
        "review-local",
        _reviewer(
            "review-local",
            "local",
            ExecutionResult(
                status=ExecutorStatus.SUCCEEDED,
                evidence={"peer-attestation": True},
            ),
        ),
    )

    result = run_escalation_ladder(
        engine,
        registry,
        PolicyGate(BUILTIN_PROFILES["standard"], BudgetLedger()),
        event_log,
        run_id="review-run",
        graph_version=_SPEC_VERSION,
        requirement=_requirement(),
        request=_request(),
        attempt_node_ids=attempt_ids,
        human_node_id=human_id,
    )

    assert result.outcome == "succeeded"
    assert result.terminal_node_id == attempt_ids[0]
    assert result.tried_executor_ids == frozenset({"review-local"})
    state = engine.current_state()
    assert state.cursors[attempt_ids[0]].status == NodeStatus.TERMINAL_SUCCESS.value
    assert attempt_ids[1] not in state.cursors
    assert attempt_ids[2] not in state.cursors
    assert human_id not in state.cursors

    applied = [event for event in event_log.read_all() if event.node_id == attempt_ids[0]]
    complete = next(event for event in applied if event.event_type == "complete")
    assert complete.payload["evidence"][0]["executor_id"] == "review-local"


def test_transient_failures_advance_local_subscription_and_default_tiers(
    tmp_path: Path,
):
    attempt_ids, _human_id, event_log, engine = _harness(tmp_path)
    registry = ExecutorRegistry()
    transient = ExecutionResult(
        status=ExecutorStatus.FAILED, payload={"failure_class": "transient"}
    )
    registry.register("review-local", _reviewer("review-local", "local", transient))
    registry.register(
        "review-subscription", _reviewer("review-subscription", "subscription_cli", transient)
    )
    registry.register(
        "review-oauth",
        _reviewer(
            "review-oauth",
            "oauth_cli",
            ExecutionResult(status=ExecutorStatus.SUCCEEDED),
        ),
    )

    profile = replace(
        BUILTIN_PROFILES["fast"],
        default_retry_budget=5,
        default_repair_budget=2,
    )
    result = run_escalation_ladder(
        engine,
        registry,
        PolicyGate(profile, BudgetLedger()),
        event_log,
        run_id="review-run",
        graph_version=_SPEC_VERSION,
        requirement=_requirement(),
        request=_request(),
        attempt_node_ids=attempt_ids,
        human_node_id="review-human",
    )

    assert result.outcome == "succeeded"
    assert result.terminal_node_id == attempt_ids[2]
    assert result.tried_executor_ids == frozenset(
        {"review-local", "review-subscription", "review-oauth"}
    )
    assert [outcome.attempt_index for outcome in result.attempts] == [1, 2, 3]
    assert [outcome.executor_id for outcome in result.attempts] == [
        "review-local",
        "review-subscription",
        "review-oauth",
    ]
    policy_events = [
        event for event in event_log.read_all() if event.event_type.startswith("policy-")
    ]
    assert [event.event_type for event in policy_events] == [
        "policy-retry-alternate-executor",
        "policy-retry-alternate-executor",
    ]
    assert all("reason" in event.payload for event in policy_events)
