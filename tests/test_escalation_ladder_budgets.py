"""Profile behavior and whole-ladder budget-keying tests."""

from __future__ import annotations

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
from praxis_runtime.transitions import TransitionEngine
from praxis_orchestration import build_escalation_ladder, run_escalation_ladder


_SPEC_VERSION = "1.0.0"
_KIND = "document-review"


def _executor(executor_id: str, transport: str):
    return FakeCapabilityExecutor(
        executor_id=executor_id,
        capabilities=[
            {
                "spec_version": _SPEC_VERSION,
                "satisfies": [{"kind": _KIND}],
                "auth_transport": transport,
            }
        ],
        script={
            _KIND: ExecutionResult(
                status=ExecutorStatus.FAILED,
                payload={"failure_class": "transient"},
            )
        },
    )


def _run(tmp_path: Path, profile_name: str, executor_specs):
    attempts = ["review-first", "review-second", "review-third"]
    human = "review-human"
    nodes, edges = build_escalation_ladder(attempt_node_ids=attempts, human_node_id=human)
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(
        Graph(
            spec_version=_SPEC_VERSION,
            nodes={node.id: node for node in nodes},
            edges=edges,
            entry_node=attempts[0],
            terminal_nodes={human},
        ),
        RunStateStore(tmp_path / "run-state.json"),
        log,
    )
    registry = ExecutorRegistry()
    for executor_id, transport in executor_specs:
        registry.register(executor_id, _executor(executor_id, transport))
    ledger = BudgetLedger()
    result = run_escalation_ladder(
        engine,
        registry,
        PolicyGate(BUILTIN_PROFILES[profile_name], ledger),
        log,
        run_id="review-run",
        graph_version=_SPEC_VERSION,
        requirement={
            "spec_version": _SPEC_VERSION,
            "requirements": [
                {
                    "promise": {"spec_version": _SPEC_VERSION, "kind": _KIND},
                    "constraint": "required",
                }
            ],
        },
        request=ExecutionRequest(
            promise={"spec_version": _SPEC_VERSION, "kind": _KIND}
        ),
        attempt_node_ids=attempts,
        human_node_id=human,
    )
    return result, engine, log, ledger, attempts, human


def test_standard_advances_once_then_retries_in_place(tmp_path: Path):
    result, _engine, log, ledger, attempts, _human = _run(
        tmp_path,
        "standard",
        [("review-local", "local"), ("review-subscription", "subscription_cli")],
    )

    assert result.outcome == "human_required"
    assert [outcome.attempt_index for outcome in result.attempts] == [1, 2]
    assert ledger.retries_used(attempts[0]) == 3
    assert ledger.repairs_used(attempts[0]) == 1
    assert ledger.retries_used(attempts[1]) == 0
    assert [event.event_type for event in log.read_all()].count("policy-retry-same-executor") == 2


def test_fast_uses_all_three_tiers_before_in_place_retry(tmp_path: Path):
    result, _engine, _log, ledger, attempts, _human = _run(
        tmp_path,
        "fast",
        [
            ("review-local", "local"),
            ("review-subscription", "subscription_cli"),
            ("review-oauth", "oauth_cli"),
        ],
    )

    assert result.outcome == "human_required"
    assert [outcome.attempt_index for outcome in result.attempts] == [1, 2, 3]
    assert result.tried_executor_ids == frozenset(
        {"review-local", "review-subscription", "review-oauth"}
    )
    assert ledger.repairs_used(attempts[0]) == 2
    assert ledger.retries_used(attempts[0]) == 5
    assert all(ledger.retries_used(node_id) == 0 for node_id in attempts[1:])


def test_regulated_stops_at_the_first_transient_failure(tmp_path: Path):
    result, _engine, _log, ledger, attempts, _human = _run(
        tmp_path, "regulated", [("review-local", "local")]
    )

    assert result.outcome == "human_required"
    assert result.terminal_node_id == attempts[0]
    assert ledger.retries_used(attempts[0]) == 0


def test_strict_retries_same_executor_through_block_and_resume(tmp_path: Path):
    result, _engine, log, ledger, attempts, _human = _run(
        tmp_path, "strict", [("review-local", "local")]
    )

    assert result.outcome == "human_required"
    assert result.terminal_node_id == attempts[0]
    event_types = [event.event_type for event in log.read_all()]
    assert event_types.count("block") == 1
    assert event_types.count("resume") == 1
    assert ledger.retries_used(attempts[0]) == 1
    assert all(ledger.retries_used(node_id) == 0 for node_id in attempts[1:])
