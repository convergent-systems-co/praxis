"""Tests for the public surface of the `praxis_orchestration` package.

This file covers only what T1 owns: that the package exists, that it
re-exports the five names the plan names, that the tier table matches the
transports `praxis_executors.policy` recognizes, that `tier_transport_policy`
narrows to its tier and never admits a paid transport, that the outcome
records are frozen with the named fields, and that the ladder builder rejects
malformed attempt-id sequences. The ladder's full topology is proved in
`test_escalation_ladder_topology.py` and the runner's decision table in
`test_escalation_ladder_runner.py`; neither is re-proved here.
"""

from __future__ import annotations

import dataclasses

import pytest

import praxis_orchestration
from praxis_executors.policy import AuthTransportPolicy, _RECOGNIZED_AUTH_TRANSPORTS
from praxis_orchestration import (
    ATTEMPT_TIERS,
    AttemptOutcome,
    EscalationResult,
    build_escalation_ladder,
    run_escalation_ladder,
)
from praxis_orchestration.escalation import tier_transport_policy


def _advertisement(executor_id, auth_transport):
    """An advertisement shaped the way `AuthTransportPolicy.is_eligible` reads it."""
    return {
        "spec_version": "1.0.0",
        "executor_id": executor_id,
        "capabilities": [
            {
                "spec_version": "1.0.0",
                "id": f"{executor_id}-cap-0",
                "satisfies": [{"kind": "text-generation"}],
                "auth_transport": auth_transport,
            }
        ],
    }


def test_package_reexports_the_names_the_seam_promises():
    for name in (
        "ATTEMPT_TIERS",
        "build_escalation_ladder",
        "AttemptOutcome",
        "EscalationResult",
        "run_escalation_ladder",
    ):
        assert hasattr(praxis_orchestration, name), name


def test_tier_table_is_local_then_subscription_then_default():
    assert ATTEMPT_TIERS == (
        frozenset({"local"}),
        frozenset({"subscription_cli"}),
        None,
    )


def test_every_named_tier_member_is_a_recognized_transport():
    for tier in ATTEMPT_TIERS:
        if tier is None:
            continue
        assert tier <= _RECOGNIZED_AUTH_TRANSPORTS


def test_tier_transport_policy_restricts_to_its_tier():
    policy = tier_transport_policy(0)

    assert isinstance(policy, AuthTransportPolicy)
    assert policy.allowed_auth_transports == frozenset({"local"})
    assert policy.is_eligible("executor-a", _advertisement("executor-a", "local")) is True
    assert (
        policy.is_eligible("executor-b", _advertisement("executor-b", "subscription_cli"))
        is False
    )


def test_second_tier_restricts_to_the_subscription_transport():
    policy = tier_transport_policy(1)

    assert policy.allowed_auth_transports == frozenset({"subscription_cli"})
    assert (
        policy.is_eligible("executor-b", _advertisement("executor-b", "subscription_cli"))
        is True
    )
    assert policy.is_eligible("executor-a", _advertisement("executor-a", "local")) is False


def test_final_tier_is_the_default_constructed_policy():
    policy = tier_transport_policy(len(ATTEMPT_TIERS) - 1)

    assert policy == AuthTransportPolicy()
    assert policy.is_eligible("executor-a", _advertisement("executor-a", "local")) is True
    assert (
        policy.is_eligible("executor-b", _advertisement("executor-b", "subscription_cli"))
        is True
    )


def test_no_tier_admits_a_paid_transport():
    """The ladder never opts in to `metered_api`/`api_key`; that stays with the caller."""
    for tier_index in range(len(ATTEMPT_TIERS)):
        policy = tier_transport_policy(tier_index)
        for auth_transport in ("metered_api", "api_key"):
            advertisement = _advertisement("executor-paid", auth_transport)

            assert policy.is_eligible("executor-paid", advertisement) is False


def test_outcome_records_are_frozen_with_the_named_fields():
    attempt_fields = {field.name for field in dataclasses.fields(AttemptOutcome)}
    assert attempt_fields == {
        "node_id",
        "attempt_index",
        "executor_id",
        "status",
        "reason",
    }

    result_fields = {field.name for field in dataclasses.fields(EscalationResult)}
    assert result_fields == {
        "outcome",
        "terminal_node_id",
        "attempts",
        "tried_executor_ids",
    }

    outcome = AttemptOutcome(
        node_id="attempt-1",
        attempt_index=1,
        executor_id="executor-a",
        status="succeeded",
        reason="",
    )
    with pytest.raises(dataclasses.FrozenInstanceError):
        outcome.status = "failed"


def test_attempt_nodes_carry_no_evidence_requirement():
    """An unsatisfied gate would raise on `apply(node, "fail")` and deadlock the ladder."""
    nodes, _edges = build_escalation_ladder(
        attempt_node_ids=["attempt-1", "attempt-2"],
        human_node_id="human",
    )

    assert nodes
    for node in nodes:
        assert node.metadata == {}


def test_builder_rejects_an_empty_attempt_sequence():
    with pytest.raises(ValueError):
        build_escalation_ladder(attempt_node_ids=[], human_node_id="human")


def test_builder_rejects_duplicate_attempt_ids():
    with pytest.raises(ValueError):
        build_escalation_ladder(
            attempt_node_ids=["attempt-1", "attempt-1"],
            human_node_id="human",
        )


def test_builder_rejects_a_human_id_that_collides_with_an_attempt():
    with pytest.raises(ValueError):
        build_escalation_ladder(
            attempt_node_ids=["attempt-1", "human"],
            human_node_id="human",
        )


def test_runner_is_declared_but_not_yet_filled_in():
    """T2 replaces the body; T1 only promises the name and the signature."""
    assert callable(run_escalation_ladder)
