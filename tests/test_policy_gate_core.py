"""Tests for `PolicyGate`, which ties profiles, authority, budgets, and
failure classification together into `PolicyDecision`s.

`authorize_start` scenarios reuse T2's authority scenarios (AUTO_APPROVED /
HUMAN_REQUIRED / DENIED), now routed through `PolicyGate` instead of calling
`authority.evaluate_authority` directly. `decide_on_failure` scenarios prove
the priority order from the plan: substantive failure always escalates
regardless of budget; retry-budget exhaustion escalates deterministically;
transient failures with budget remaining retry same-executor by default, or
alternate-executor when the profile allows it, previously-tried executors
were passed in, and repair budget remains; repair exhaustion falls back to
same-executor retry rather than escalating, as long as retry budget remains.
"""

from __future__ import annotations

from dataclasses import replace

from praxis_policy.budgets import BudgetLedger
from praxis_policy.gate import PolicyGate, PolicyOutcome, human_denial_event_sequence
from praxis_policy.profiles import BUILTIN_PROFILES

_TRANSIENT_PAYLOAD = {"failure_class": "transient"}
_SUBSTANTIVE_PAYLOAD = {"failure_class": "substantive"}


def _profile_allowing_alternate_executor_retry(*, max_retries: int = 3, max_repairs: int = 1):
    return replace(
        BUILTIN_PROFILES["standard"],
        allow_alternate_executor_retry=True,
        default_retry_budget=max_retries,
        default_repair_budget=max_repairs,
    )


def _profile_disallowing_alternate_executor_retry(*, max_retries: int = 3):
    return replace(
        BUILTIN_PROFILES["standard"],
        allow_alternate_executor_retry=False,
        default_retry_budget=max_retries,
    )


# ---------------------------------------------------------------------------
# authorize_start
# ---------------------------------------------------------------------------


def test_authorize_start_with_no_authority_requirement_is_authorized():
    gate = PolicyGate(BUILTIN_PROFILES["standard"], BudgetLedger())

    decision = gate.authorize_start({})

    assert decision.outcome is PolicyOutcome.AUTHORIZED
    assert decision.event_type is None


def test_authorize_start_with_scope_granted_is_authorized():
    profile = BUILTIN_PROFILES["standard"]
    gate = PolicyGate(profile, BudgetLedger(), granted_authority_scopes=frozenset({"deploy:prod"}))
    node_metadata = {
        "authority_requirement": {
            "scopes": [{"scope": "deploy:prod", "constraint": "required"}]
        }
    }

    decision = gate.authorize_start(node_metadata)

    assert decision.outcome is PolicyOutcome.AUTHORIZED
    assert decision.event_type is None


def test_authorize_start_with_unresolved_required_scope_is_human_required():
    profile = BUILTIN_PROFILES["standard"]
    gate = PolicyGate(profile, BudgetLedger())
    node_metadata = {
        "authority_requirement": {
            "scopes": [{"scope": "deploy:prod", "constraint": "required"}]
        }
    }

    decision = gate.authorize_start(node_metadata)

    assert decision.outcome is PolicyOutcome.HUMAN_REQUIRED
    assert decision.event_type == "handoff"
    assert decision.detail["unresolved_scopes"] == ["deploy:prod"]


def test_authorize_start_with_prohibited_scope_granted_is_denied():
    profile = BUILTIN_PROFILES["standard"]
    gate = PolicyGate(profile, BudgetLedger(), granted_authority_scopes=frozenset({"delete:prod"}))
    node_metadata = {
        "authority_requirement": {
            "scopes": [{"scope": "delete:prod", "constraint": "prohibited"}]
        }
    }

    decision = gate.authorize_start(node_metadata)

    assert decision.outcome is PolicyOutcome.DENIED
    assert decision.event_type == "fail"
    assert decision.detail["denied_scopes"] == ["delete:prod"]


# ---------------------------------------------------------------------------
# decide_on_failure: substantive failure always escalates
# ---------------------------------------------------------------------------


def test_decide_on_failure_substantive_is_human_required_regardless_of_budget():
    profile = _profile_allowing_alternate_executor_retry(max_retries=5)
    gate = PolicyGate(profile, BudgetLedger())

    decision = gate.decide_on_failure("node-1", {}, _SUBSTANTIVE_PAYLOAD)

    assert decision.outcome is PolicyOutcome.HUMAN_REQUIRED
    assert decision.event_type == "handoff"


def test_decide_on_failure_missing_payload_classifies_substantive_and_is_human_required():
    profile = _profile_allowing_alternate_executor_retry(max_retries=5)
    gate = PolicyGate(profile, BudgetLedger())

    decision = gate.decide_on_failure("node-1", {}, None)

    assert decision.outcome is PolicyOutcome.HUMAN_REQUIRED
    assert decision.event_type == "handoff"


# ---------------------------------------------------------------------------
# decide_on_failure: retry budget exhaustion escalates deterministically
# ---------------------------------------------------------------------------


def test_decide_on_failure_retry_budget_exhaustion_is_human_required_with_matching_detail():
    profile = _profile_disallowing_alternate_executor_retry(max_retries=2)
    gate = PolicyGate(profile, BudgetLedger())

    first = gate.decide_on_failure("node-1", {}, _TRANSIENT_PAYLOAD)
    second = gate.decide_on_failure("node-1", {}, _TRANSIENT_PAYLOAD)
    third = gate.decide_on_failure("node-1", {}, _TRANSIENT_PAYLOAD)

    assert first.outcome is PolicyOutcome.RETRY_SAME_EXECUTOR
    assert second.outcome is PolicyOutcome.RETRY_SAME_EXECUTOR
    assert third.outcome is PolicyOutcome.HUMAN_REQUIRED
    assert third.event_type == "handoff"
    assert third.detail["retries_used"] == third.detail["max_retries"] == 2


# ---------------------------------------------------------------------------
# decide_on_failure: transient with budget remaining
# ---------------------------------------------------------------------------


def test_decide_on_failure_transient_with_no_previously_tried_executors_is_retry_same_executor():
    profile = _profile_allowing_alternate_executor_retry(max_retries=3, max_repairs=1)
    gate = PolicyGate(profile, BudgetLedger())

    decision = gate.decide_on_failure("node-1", {}, _TRANSIENT_PAYLOAD)

    assert decision.outcome is PolicyOutcome.RETRY_SAME_EXECUTOR
    assert decision.event_type == "block"


def test_decide_on_failure_transient_with_alternate_retry_allowed_and_repair_budget_is_retry_alternate_executor():
    profile = _profile_allowing_alternate_executor_retry(max_retries=3, max_repairs=1)
    gate = PolicyGate(profile, BudgetLedger())
    previously_tried = frozenset({"executor-a"})

    decision = gate.decide_on_failure(
        "node-1", {}, _TRANSIENT_PAYLOAD, previously_tried_executor_ids=previously_tried
    )

    assert decision.outcome is PolicyOutcome.RETRY_ALTERNATE_EXECUTOR
    assert decision.event_type == "block"
    assert decision.excluded_executor_ids == previously_tried


def test_decide_on_failure_transient_with_alternate_retry_disallowed_is_retry_same_executor():
    profile = _profile_disallowing_alternate_executor_retry(max_retries=3)
    gate = PolicyGate(profile, BudgetLedger())
    previously_tried = frozenset({"executor-a"})

    decision = gate.decide_on_failure(
        "node-1", {}, _TRANSIENT_PAYLOAD, previously_tried_executor_ids=previously_tried
    )

    assert decision.outcome is PolicyOutcome.RETRY_SAME_EXECUTOR
    assert decision.event_type == "block"


def test_decide_on_failure_transient_with_repair_budget_exhausted_is_retry_same_executor():
    profile = _profile_allowing_alternate_executor_retry(max_retries=3, max_repairs=1)
    ledger = BudgetLedger()
    gate = PolicyGate(profile, ledger)
    previously_tried = frozenset({"executor-a"})

    # Exhaust the single repair budget unit on a first transient failure.
    first = gate.decide_on_failure(
        "node-1", {}, _TRANSIENT_PAYLOAD, previously_tried_executor_ids=previously_tried
    )
    assert first.outcome is PolicyOutcome.RETRY_ALTERNATE_EXECUTOR

    second = gate.decide_on_failure(
        "node-1", {}, _TRANSIENT_PAYLOAD, previously_tried_executor_ids=previously_tried
    )

    assert second.outcome is PolicyOutcome.RETRY_SAME_EXECUTOR
    assert second.event_type == "block"


# ---------------------------------------------------------------------------
# human_denial_event_sequence
# ---------------------------------------------------------------------------


def test_human_denial_event_sequence_is_accept_then_fail():
    assert human_denial_event_sequence() == ["accept", "fail"]
