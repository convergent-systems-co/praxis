"""Escalation integration tests: exhausted retry budgets, authority denial,
and end-to-end policy escalation, all driven through a real
`TransitionEngine` over `tmp_path` rather than asserting on `PolicyDecision`
values alone (that is `test_policy_gate_core.py`'s job).

Graphs/nodes are built directly as dataclasses, following
`tests/test_transitions.py`'s inline-graph convention (not
`conftest._linear_graph`, since these fixtures need custom `metadata`).
Uses a generic field-operation domain (no software-development vocabulary,
per the epic's constraint), following the same non-development-domain
convention as `tests/test_policy_gate_alternate_executor.py`.
"""

from __future__ import annotations

from dataclasses import replace
from pathlib import Path

import pytest

from praxis_policy.budgets import BudgetLedger
from praxis_policy.gate import PolicyGate, PolicyOutcome, human_denial_event_sequence
from praxis_policy.profiles import BUILTIN_PROFILES
from praxis_policy.receipts import record_policy_decision
from praxis_runtime.events import EventLog
from praxis_runtime.graph import Graph, Node
from praxis_runtime.state import RunStateStore
from praxis_runtime.transitions import NodeStatus, TransitionEngine

_SPEC_VERSION = "1.0.0"
_TRANSIENT_PAYLOAD = {"failure_class": "transient"}


def _single_node_graph(node: Node) -> Graph:
    return Graph(
        spec_version=_SPEC_VERSION,
        nodes={node.id: node},
        edges=[],
        entry_node=node.id,
        terminal_nodes={node.id},
    )


def _make_engine(tmp_path: Path, node: Node) -> TransitionEngine:
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    return TransitionEngine(_single_node_graph(node), store, log)


def test_exhausted_retry_budget_escalates_to_handoff_and_records_a_receipt(tmp_path: Path):
    node = Node(
        id="survey-1",
        kind="task",
        metadata={
            "budget_requirement": {"spec_version": _SPEC_VERSION, "max_retries": 2},
        },
    )
    store = RunStateStore(tmp_path / "run-state.json")
    log = EventLog(tmp_path / "events")
    engine = TransitionEngine(_single_node_graph(node), store, log)
    engine.apply(node.id, "start")

    profile = replace(
        BUILTIN_PROFILES["standard"],
        allow_alternate_executor_retry=False,
        default_retry_budget=3,
    )
    gate = PolicyGate(profile, BudgetLedger())

    decision = None
    for _ in range(10):
        decision = gate.decide_on_failure(node.id, node.metadata, _TRANSIENT_PAYLOAD)
        if decision.outcome is PolicyOutcome.HUMAN_REQUIRED:
            break
        assert decision.outcome is PolicyOutcome.RETRY_SAME_EXECUTOR
        assert decision.event_type == "block"
        engine.apply(node.id, "block")
        engine.apply(node.id, "resume")
    else:
        pytest.fail("retry budget was never exhausted")

    assert decision.outcome is PolicyOutcome.HUMAN_REQUIRED
    assert decision.event_type == "handoff"
    assert decision.detail["retries_used"] == decision.detail["max_retries"] == 2

    state = engine.apply(node.id, decision.event_type)
    assert state.cursors[node.id].status == NodeStatus.HANDOFF.value

    receipt = record_policy_decision(
        log, run_id=state.run_id, node_id=node.id, decision=decision
    )

    recorded = log.read_all()
    assert receipt.event_id in {event.event_id for event in recorded}
    assert receipt.event_type == "policy-human-required"
    assert receipt.payload["reason"] == "retry budget exhausted"


def test_authority_denial_drives_running_node_straight_to_terminal_failed(tmp_path: Path):
    node = Node(
        id="survey-2",
        kind="task",
        metadata={
            "authority_requirement": {
                "spec_version": _SPEC_VERSION,
                "scopes": [{"scope": "destructive", "constraint": "prohibited"}],
            },
        },
    )
    engine = _make_engine(tmp_path, node)
    engine.apply(node.id, "start")

    gate = PolicyGate(
        BUILTIN_PROFILES["standard"],
        BudgetLedger(),
        granted_authority_scopes=frozenset({"destructive"}),
    )

    decision = gate.authorize_start(node.metadata)

    assert decision.outcome is PolicyOutcome.DENIED
    assert decision.event_type == "fail"

    state = engine.apply(node.id, decision.event_type)
    assert state.cursors[node.id].status == NodeStatus.TERMINAL_FAILED.value


def test_policy_escalation_end_to_end_reaches_terminal_failed_via_human_denial(
    tmp_path: Path,
):
    node = Node(
        id="survey-3",
        kind="task",
        metadata={
            "authority_requirement": {
                "spec_version": _SPEC_VERSION,
                "scopes": [{"scope": "field-directive", "constraint": "required"}],
            },
        },
    )
    engine = _make_engine(tmp_path, node)
    engine.apply(node.id, "start")

    gate = PolicyGate(BUILTIN_PROFILES["standard"], BudgetLedger())

    decision = gate.authorize_start(node.metadata)
    assert decision.outcome is PolicyOutcome.HUMAN_REQUIRED
    assert decision.event_type == "handoff"

    state = engine.apply(node.id, decision.event_type)
    assert state.cursors[node.id].status == NodeStatus.HANDOFF.value

    for event_type in human_denial_event_sequence():
        state = engine.apply(node.id, event_type)

    assert state.cursors[node.id].status == NodeStatus.TERMINAL_FAILED.value
