"""Tests for `record_policy_decision`, which turns a `PolicyDecision` into an
auditable, append-only event on the run's `EventLog`.

This is purely additive: the appended `"policy-*"` event never participates
in `TransitionEngine`'s `_TRANSITIONS` legality table and never mutates
`RunState` (it doesn't go through `TransitionEngine.apply` at all) -- it is
audit-only, existing alongside whatever real transition event the caller
separately applies for the decision's own `event_type`. Uses a real
`EventLog` over `tmp_path`, same fixture style as `tests/test_event_log.py`.
"""

from __future__ import annotations

from pathlib import Path

from praxis_policy.gate import PolicyDecision, PolicyOutcome
from praxis_policy.receipts import record_policy_decision
from praxis_runtime.events import EventLog


def test_record_policy_decision_appends_event_with_reason_and_detail(tmp_path: Path) -> None:
    log = EventLog(tmp_path)
    decision = PolicyDecision(
        outcome=PolicyOutcome.HUMAN_REQUIRED,
        event_type="handoff",
        reason="retry budget exhausted",
        detail={"retries_used": 3, "max_retries": 3},
    )

    record_policy_decision(log, run_id="run-1", node_id="node-a", decision=decision)

    [event] = log.read_all()
    assert event.event_type == "policy-human-required"
    assert event.payload["reason"] == "retry budget exhausted"
    assert event.payload["retries_used"] == 3
    assert event.payload["max_retries"] == 3


def test_record_policy_decision_returns_event_with_log_assigned_seq(tmp_path: Path) -> None:
    log = EventLog(tmp_path)
    decision = PolicyDecision(
        outcome=PolicyOutcome.RETRY_ALTERNATE_EXECUTOR,
        event_type="block",
        reason="transient failure retried against an alternate executor",
        excluded_executor_ids=frozenset({"exec-1", "exec-2"}),
    )

    returned = record_policy_decision(log, run_id="run-1", node_id="node-a", decision=decision)

    assert returned.seq == 0
    assert returned.event_type == "policy-retry-alternate-executor"
    assert returned.payload["excluded_executor_ids"] == ["exec-1", "exec-2"]


def test_record_policy_decision_appended_event_is_visible_via_read_all(tmp_path: Path) -> None:
    log = EventLog(tmp_path)
    decision = PolicyDecision(
        outcome=PolicyOutcome.DENIED,
        event_type="fail",
        reason="prohibited authority scope was granted",
        detail={"denied_scopes": ["deploy:prod"]},
    )

    record_policy_decision(log, run_id="run-1", node_id="node-a", decision=decision)

    events = log.read_all()
    assert len(events) == 1
    assert events[0].node_id == "node-a"
    assert events[0].run_id == "run-1"


def test_two_decisions_for_different_nodes_get_distinct_ids_and_increasing_seq(
    tmp_path: Path,
) -> None:
    log = EventLog(tmp_path)
    first_decision = PolicyDecision(
        outcome=PolicyOutcome.HUMAN_REQUIRED,
        event_type="handoff",
        reason="required authority scope is unresolved",
        detail={"unresolved_scopes": ["deploy:prod"]},
    )
    second_decision = PolicyDecision(
        outcome=PolicyOutcome.RETRY_SAME_EXECUTOR,
        event_type="block",
        reason="transient failure retried against the same executor",
    )

    first = record_policy_decision(log, run_id="run-1", node_id="node-a", decision=first_decision)
    second = record_policy_decision(log, run_id="run-1", node_id="node-b", decision=second_decision)

    assert first.event_id != second.event_id
    assert [first.seq, second.seq] == [0, 1]

    all_events = log.read_all()
    assert [event.node_id for event in all_events] == ["node-a", "node-b"]
