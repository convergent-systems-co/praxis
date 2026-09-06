"""Tests for promotion orchestration: gate + authority + append-only decision.

`evaluate_candidate` composes T5's `compare_measurements`, T7's
`evaluate_promotion_gate`, and `praxis_policy.authority.evaluate_authority`
into a single `PromotionDecision`. The health/regression gate is
authoritative over whether authority is even consulted: an unsatisfied gate
short-circuits straight to `REJECTED` without ever constructing an authority
decision (no reason to ask a human to approve a candidate that already
failed its objective checks) -- proven here by a policy whose
`authority_requirement` is malformed in a way that would raise if
`evaluate_authority` actually attempted to walk it.

`promote()` is the structural enforcement of "a candidate cannot become
active without recorded evaluation evidence": it raises `PromotionError`
fail-closed for anything other than an `ACCEPTED` decision, for an
`ACCEPTED` decision missing `evaluation_ids`, and for a `candidate_id` never
registered in the `CandidateRegistry`.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import pytest

from praxis_eval.candidates import CandidateRegistry, build_candidate_config
from praxis_eval.ledger import PromotionLedger
from praxis_eval.promotion import (
    PromotionDecision,
    PromotionError,
    PromotionOutcome,
    evaluate_candidate,
    promote,
)
from praxis_eval.types import Measurement, MetricThreshold, PromotionGateResult, PromotionPolicy

_SPEC_VERSION = "1.0.0"
_CANDIDATE_ID = "candidate-1"


@dataclass(frozen=True)
class _FakeProfile:
    auto_approved_authority_scopes: frozenset[str]


def _policy(
    *thresholds: MetricThreshold, authority_requirement: dict | None = None
) -> PromotionPolicy:
    return PromotionPolicy(
        spec_version=_SPEC_VERSION,
        thresholds=tuple(thresholds),
        authority_requirement=authority_requirement,
    )


def _passing_threshold() -> MetricThreshold:
    return MetricThreshold(
        metric="latency_ms",
        constraint="required",
        direction="lower_is_better",
        max_regression_pct=5,
    )


def _failing_threshold() -> MetricThreshold:
    return MetricThreshold(
        metric="latency_ms",
        constraint="required",
        direction="lower_is_better",
        max_regression_pct=5,
    )


_PASSING_CANDIDATE = (Measurement(metric="latency_ms", value=100.0),)
_PASSING_BASELINE = (Measurement(metric="latency_ms", value=100.0),)
_REGRESSED_CANDIDATE = (Measurement(metric="latency_ms", value=200.0),)


def test_satisfied_gate_no_authority_requirement_is_accepted():
    policy = _policy(_passing_threshold())
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())

    decision = evaluate_candidate(
        candidate_id=_CANDIDATE_ID,
        candidate_measurements=_PASSING_CANDIDATE,
        baseline_measurements=_PASSING_BASELINE,
        policy=policy,
        profile=profile,
    )

    assert decision.outcome is PromotionOutcome.ACCEPTED
    assert decision.candidate_id == _CANDIDATE_ID
    assert decision.gate_result.satisfied is True
    assert decision.authority_outcome == "auto_approved"


def test_unsatisfied_gate_is_rejected_without_ever_evaluating_authority():
    # A malformed authority_requirement (entries missing "scope"/"constraint")
    # would raise a KeyError if praxis_policy.authority.evaluate_authority ever
    # walked it -- proving the gate short-circuits before authority is touched.
    malformed_authority_requirement = {
        "spec_version": _SPEC_VERSION,
        "scopes": [{"nonsense": "field"}],
    }
    policy = _policy(
        _failing_threshold(), authority_requirement=malformed_authority_requirement
    )
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())

    decision = evaluate_candidate(
        candidate_id=_CANDIDATE_ID,
        candidate_measurements=_REGRESSED_CANDIDATE,
        baseline_measurements=_PASSING_BASELINE,
        policy=policy,
        profile=profile,
    )

    assert decision.outcome is PromotionOutcome.REJECTED
    assert decision.gate_result.satisfied is False
    assert decision.reasons == decision.gate_result.reasons
    assert decision.reasons != ()
    assert decision.authority_outcome is None


def test_satisfied_gate_with_required_scope_not_auto_approved_is_human_required():
    policy = _policy(
        _passing_threshold(),
        authority_requirement={
            "spec_version": _SPEC_VERSION,
            "scopes": [{"scope": "production-deploy", "constraint": "required"}],
        },
    )
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())

    decision = evaluate_candidate(
        candidate_id=_CANDIDATE_ID,
        candidate_measurements=_PASSING_CANDIDATE,
        baseline_measurements=_PASSING_BASELINE,
        policy=policy,
        profile=profile,
    )

    assert decision.outcome is PromotionOutcome.HUMAN_REQUIRED
    assert decision.authority_outcome == "human_required"
    assert len(decision.reasons) == len(decision.gate_result.reasons) + 1
    assert any("production-deploy" in reason for reason in decision.reasons)


def test_satisfied_gate_with_prohibited_scope_granted_is_rejected_via_denied():
    policy = _policy(
        _passing_threshold(),
        authority_requirement={
            "spec_version": _SPEC_VERSION,
            "scopes": [{"scope": "destructive", "constraint": "prohibited"}],
        },
    )
    profile = _FakeProfile(auto_approved_authority_scopes=frozenset())

    decision = evaluate_candidate(
        candidate_id=_CANDIDATE_ID,
        candidate_measurements=_PASSING_CANDIDATE,
        baseline_measurements=_PASSING_BASELINE,
        policy=policy,
        profile=profile,
        granted_scopes=frozenset({"destructive"}),
    )

    assert decision.outcome is PromotionOutcome.REJECTED
    assert decision.authority_outcome == "denied"
    assert len(decision.reasons) == len(decision.gate_result.reasons) + 1
    assert any("destructive" in reason for reason in decision.reasons)


def _accepted_decision(candidate_id: str) -> PromotionDecision:
    return PromotionDecision(
        outcome=PromotionOutcome.ACCEPTED,
        candidate_id=candidate_id,
        gate_result=PromotionGateResult(
            candidate_id=candidate_id, satisfied=True, reasons=(), evaluated=("latency_ms",)
        ),
        authority_outcome="auto_approved",
        reasons=(),
    )


def _human_required_decision(candidate_id: str) -> PromotionDecision:
    return PromotionDecision(
        outcome=PromotionOutcome.HUMAN_REQUIRED,
        candidate_id=candidate_id,
        gate_result=PromotionGateResult(
            candidate_id=candidate_id, satisfied=True, reasons=(), evaluated=("latency_ms",)
        ),
        authority_outcome="human_required",
        reasons=("production-deploy required",),
    )


def _rejected_decision(candidate_id: str) -> PromotionDecision:
    return PromotionDecision(
        outcome=PromotionOutcome.REJECTED,
        candidate_id=candidate_id,
        gate_result=PromotionGateResult(
            candidate_id=candidate_id,
            satisfied=False,
            reasons=("latency_ms: regressed",),
            evaluated=("latency_ms",),
        ),
        authority_outcome=None,
        reasons=("latency_ms: regressed",),
    )


def test_promote_with_accepted_decision_appends_record_and_updates_active_candidate(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")
    candidate = build_candidate_config({"alpha": 1})
    registry.register(candidate)
    decision = _accepted_decision(candidate.candidate_id)

    record = promote(ledger, registry, decision, evaluation_ids=["eval-1"])

    assert record.action == "promote"
    assert record.candidate_id == candidate.candidate_id
    assert record.decision == "accepted"
    assert record.evaluation_ids == ("eval-1",)
    assert record.authority_outcome == "auto_approved"
    assert ledger.active_candidate_id() == candidate.candidate_id


def test_promote_with_human_required_decision_raises_and_leaves_active_candidate_unchanged(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")
    candidate = build_candidate_config({"alpha": 1})
    registry.register(candidate)
    decision = _human_required_decision(candidate.candidate_id)

    with pytest.raises(PromotionError):
        promote(ledger, registry, decision, evaluation_ids=["eval-1"])

    assert ledger.active_candidate_id() is None


def test_promote_with_rejected_decision_raises_and_leaves_active_candidate_unchanged(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")
    candidate = build_candidate_config({"alpha": 1})
    registry.register(candidate)
    decision = _rejected_decision(candidate.candidate_id)

    with pytest.raises(PromotionError):
        promote(ledger, registry, decision, evaluation_ids=["eval-1"])

    assert ledger.active_candidate_id() is None


def test_promote_with_empty_evaluation_ids_raises_even_for_accepted_decision(
    tmp_path: Path,
) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")
    candidate = build_candidate_config({"alpha": 1})
    registry.register(candidate)
    decision = _accepted_decision(candidate.candidate_id)

    with pytest.raises(PromotionError):
        promote(ledger, registry, decision, evaluation_ids=[])

    assert ledger.active_candidate_id() is None


def test_promote_for_unregistered_candidate_raises(tmp_path: Path) -> None:
    ledger = PromotionLedger(tmp_path / "ledger")
    registry = CandidateRegistry(tmp_path / "registry")
    decision = _accepted_decision("never-registered")

    with pytest.raises(PromotionError):
        promote(ledger, registry, decision, evaluation_ids=["eval-1"])

    assert ledger.active_candidate_id() is None
