"""Failure classification, exhaustion, and audit-surface tests."""

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
            {"promise": {"spec_version": _SPEC_VERSION, "kind": _KIND}, "constraint": "required"}
        ],
    }


def _executor(executor_id: str, transport: str, result: ExecutionResult):
    return FakeCapabilityExecutor(
        executor_id=executor_id,
        capabilities=[
            {
                "spec_version": _SPEC_VERSION,
                "satisfies": [{"kind": _KIND}],
                "auth_transport": transport,
            }
        ],
        script={_KIND: result},
    )


def _run(tmp_path: Path, registry: ExecutorRegistry, *, profile="standard", ledger=None):
    attempt_ids = ["review-first", "review-second", "review-third"]
    human_id = "review-human"
    nodes, edges = build_escalation_ladder(
        attempt_node_ids=attempt_ids, human_node_id=human_id
    )
    event_log = EventLog(tmp_path / "events")
    engine = TransitionEngine(
        Graph(
            spec_version=_SPEC_VERSION,
            nodes={node.id: node for node in nodes},
            edges=edges,
            entry_node=attempt_ids[0],
            terminal_nodes={human_id},
        ),
        RunStateStore(tmp_path / "run-state.json"),
        event_log,
    )
    ledger = ledger or BudgetLedger()
    result = run_escalation_ladder(
        engine,
        registry,
        PolicyGate(BUILTIN_PROFILES[profile], ledger),
        event_log,
        run_id="review-run",
        graph_version=_SPEC_VERSION,
        requirement=_requirement(),
        request=ExecutionRequest(
            promise={"spec_version": _SPEC_VERSION, "kind": _KIND}
        ),
        attempt_node_ids=attempt_ids,
        human_node_id=human_id,
    )
    return result, engine, event_log, ledger, attempt_ids, human_id


def test_unreported_or_unknown_failure_is_handed_off_on_first_rung(tmp_path: Path):
    for payload in ({}, {"failure_class": "mysterious"}):
        registry = ExecutorRegistry()
        registry.register(
            "review-local",
            _executor(
                "review-local",
                "local",
                ExecutionResult(status=ExecutorStatus.FAILED, payload=payload),
            ),
        )
        result, engine, _log, _ledger, attempt_ids, human_id = _run(
            tmp_path / str(len(payload)), registry
        )

        assert result.outcome == "human_required"
        assert result.terminal_node_id == attempt_ids[0]
        state = engine.current_state()
        assert state.cursors[attempt_ids[0]].status == NodeStatus.HANDOFF.value
        assert attempt_ids[1] not in state.cursors
        assert attempt_ids[2] not in state.cursors
        assert human_id not in state.cursors


def test_transient_exhaustion_on_last_rung_hands_off_the_human_cursor(tmp_path: Path):
    registry = ExecutorRegistry()
    transient = ExecutionResult(
        status=ExecutorStatus.FAILED, payload={"failure_class": "transient"}
    )
    registry.register("review-local", _executor("review-local", "local", transient))
    registry.register(
        "review-subscription", _executor("review-subscription", "subscription_cli", transient)
    )
    registry.register("review-oauth", _executor("review-oauth", "oauth_cli", transient))

    result, engine, event_log, _ledger, attempt_ids, human_id = _run(
        tmp_path, registry, profile="fast"
    )
    state = engine.current_state()
    assert result.outcome == "human_required"
    assert result.terminal_node_id == human_id
    assert state.cursors[attempt_ids[-1]].status == NodeStatus.TERMINAL_FAILED.value
    assert state.cursors[human_id].status == NodeStatus.HANDOFF.value
    assert any(event.event_type == "policy-retry-same-executor" for event in event_log.read_all())


def test_empty_first_tier_skips_without_spending_budget(tmp_path: Path):
    registry = ExecutorRegistry()
    ledger = BudgetLedger()
    registry.register(
        "review-subscription",
        _executor(
            "review-subscription",
            "subscription_cli",
            ExecutionResult(status=ExecutorStatus.SUCCEEDED),
        ),
    )
    result, engine, _log, ledger, attempt_ids, human_id = _run(
        tmp_path, registry, ledger=ledger
    )

    assert result.outcome == "succeeded"
    assert result.terminal_node_id == attempt_ids[1]
    assert result.attempts[0].status == "no-candidate"
    assert result.attempts[0].executor_id is None
    assert ledger.retries_used(attempt_ids[0]) == 0
    assert ledger.repairs_used(attempt_ids[0]) == 0
    assert human_id not in engine.current_state().cursors


def test_policy_receipts_and_tried_executor_ids_are_auditable(tmp_path: Path):
    registry = ExecutorRegistry()
    transient = ExecutionResult(
        status=ExecutorStatus.FAILED, payload={"failure_class": "transient"}
    )
    registry.register("review-local", _executor("review-local", "local", transient))
    registry.register(
        "review-subscription",
        _executor(
            "review-subscription",
            "subscription_cli",
            ExecutionResult(status=ExecutorStatus.SUCCEEDED, evidence={"peer-attestation": True}),
        ),
    )
    result, _engine, event_log, _ledger, _attempt_ids, _human_id = _run(
        tmp_path, registry
    )

    assert result.tried_executor_ids == frozenset({"review-local", "review-subscription"})
    receipts = [event for event in event_log.read_all() if event.event_type.startswith("policy-")]
    assert receipts
    assert receipts[0].event_type == "policy-retry-alternate-executor"
    assert receipts[0].payload["reason"]
    proof_events = [event for event in event_log.read_all() if event.event_type == "complete"]
    assert proof_events[0].payload["evidence"][0]["executor_id"] == "review-subscription"
