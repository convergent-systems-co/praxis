"""Tests for the project-to-global promotion proposal bridge into praxis_eval.

`propose_promotion` is the fail-closed gate between a project-scoped
`HeuristicCandidate` and `praxis_eval`'s candidate/evaluation/promotion
machinery: it refuses (via `LearningPromotionError`) any heuristic that isn't
`scope == "project"`, `status == "candidate"`, backed by at least
`MIN_EVIDENCE_COUNT` evidence ids, and at or above `MIN_CONFIDENCE`
confidence -- all checked before anything else happens, so a hand-set
`confidence=1.0` never compensates for too little evidence. It then still
runs the heuristic's `proposed_configuration` and the fixed learned-heuristic
target through `praxis_learning.guardrails`, which raises `GuardrailViolation`
(propagated, not wrapped) independently of those four checks.

`build_promotion_policy` always attaches an `authority_requirement` demanding
`guardrails._REQUIRED_PROMOTION_AUTHORITY_SCOPE`, and self-checks that via
`guardrails.require_authority_review` before returning -- since no
`BUILTIN_PROFILE` auto-approves that scope, `propose_promotion` can only ever
produce `HUMAN_REQUIRED` or `REJECTED`, never `ACCEPTED`, which is asserted
explicitly here as the "explicit reviewed promotion" guarantee. The one test
that needs an `ACCEPTED` decision constructs it by hand, since
`evaluate_candidate` itself cannot produce one without a granted scope, and
round-trips it through `accept_promotion` -- a thin, non-bypassing wrapper
around `praxis_eval.promotion.promote` -- into a real ledger record.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_eval.candidates import CandidateRegistry, build_candidate_config
from praxis_eval.ledger import PromotionLedger
from praxis_eval.promotion import PromotionDecision, PromotionOutcome
from praxis_eval.types import (
    EvaluationRecord,
    Measurement,
    PromotionGateResult,
)
from praxis_learning import guardrails
from praxis_learning.guardrails import GuardrailViolation
from praxis_learning.heuristics import HeuristicRegistry
from praxis_learning.promotion_bridge import (
    MIN_CONFIDENCE,
    MIN_EVIDENCE_COUNT,
    LearningPromotionError,
    accept_promotion,
    build_promotion_policy,
    propose_promotion,
)
from praxis_learning.types import HeuristicCandidate
from praxis_policy.profiles import BUILTIN_PROFILES

_SPEC_VERSION = "1.0.0"
_TIMESTAMP = "2026-09-06T00:00:00Z"

_CLEAN_CONFIGURATION = {
    "pattern": "retry-on-timeout",
    "steps": [{"name": "retry", "max_attempts": 3}],
}


def _heuristic(**overrides) -> HeuristicCandidate:
    fields = {
        "spec_version": _SPEC_VERSION,
        "heuristic_id": "heuristic-1",
        "project_id": "project-1",
        "scope": "project",
        "pattern": "recurrent-failure",
        "trigger": {"error_kind": "timeout"},
        "expected_outcome": "retry succeeds",
        "proposed_configuration": dict(_CLEAN_CONFIGURATION),
        "status": "candidate",
        "confidence": 0.9,
        "evidence_ids": ("obs-1", "obs-2", "obs-3"),
        "created_at": _TIMESTAMP,
        "updated_at": _TIMESTAMP,
    }
    fields.update(overrides)
    return HeuristicCandidate(**fields)


def _evaluation_record(*, candidate_id: str, measurements: tuple[Measurement, ...]) -> EvaluationRecord:
    return EvaluationRecord(
        spec_version=_SPEC_VERSION,
        evaluation_id="eval-1",
        candidate_id=candidate_id,
        workload_id="workload-1",
        measurements=measurements,
        produced_at=_TIMESTAMP,
    )


def _matching_measurements(policy) -> tuple[Measurement, ...]:
    # Equal candidate/baseline values satisfy any `required` threshold's
    # tolerance check regardless of direction or max_regression_pct (both
    # directions' `>=`/`<=` comparisons hold at equality), so this stays
    # agnostic to whatever default metric build_promotion_policy chooses.
    return tuple(Measurement(metric=t.metric, value=1.0) for t in policy.thresholds)


def test_min_evidence_count_and_min_confidence_constants():
    assert MIN_EVIDENCE_COUNT == 3
    assert MIN_CONFIDENCE == 0.75


def test_build_promotion_policy_requires_promotion_authority_scope():
    policy = build_promotion_policy()

    assert policy.thresholds
    guardrails.require_authority_review(policy)  # must not raise


def test_build_promotion_policy_invokes_its_own_authority_self_check(monkeypatch):
    original = guardrails.require_authority_review
    calls: list = []

    def spy(policy):
        calls.append(policy)
        return original(policy)

    monkeypatch.setattr(guardrails, "require_authority_review", spy)

    policy = build_promotion_policy()

    # Proves build_promotion_policy's own internal self-check fired (not just
    # that the policy happens to satisfy require_authority_review if called
    # externally, as in test_build_promotion_policy_requires_promotion_authority_scope).
    assert calls == [policy]


def test_build_promotion_policy_merges_extra_thresholds():
    from praxis_eval.types import MetricThreshold

    extra = MetricThreshold(
        metric="custom_health_metric",
        constraint="preferred",
        direction="higher_is_better",
    )

    policy = build_promotion_policy(extra_thresholds=(extra,))

    assert extra in policy.thresholds
    guardrails.require_authority_review(policy)  # must not raise


@pytest.mark.parametrize("evidence_count", [1, 2])
def test_propose_promotion_refuses_insufficient_evidence_even_at_full_confidence(
    tmp_path: Path, evidence_count: int
):
    evidence_ids = tuple(f"obs-{i}" for i in range(evidence_count))
    heuristic = _heuristic(evidence_ids=evidence_ids, confidence=1.0)
    registry = CandidateRegistry(tmp_path / "registry")
    evaluation = _evaluation_record(candidate_id="unused", measurements=(Measurement(metric="m", value=1.0),))
    profile = BUILTIN_PROFILES["standard"]

    with pytest.raises(LearningPromotionError):
        propose_promotion(
            heuristic,
            registry=registry,
            evaluation=evaluation,
            baseline_evaluation=None,
            profile=profile,
        )


def test_propose_promotion_refuses_insufficient_confidence_with_sufficient_evidence(
    tmp_path: Path,
):
    # evidence_ids defaults to 3 (== MIN_EVIDENCE_COUNT), so this isolates the
    # confidence check: it must independently gate on confidence even when
    # the evidence-count check would pass.
    heuristic = _heuristic(confidence=0.5)
    registry = CandidateRegistry(tmp_path / "registry")
    evaluation = _evaluation_record(candidate_id="unused", measurements=(Measurement(metric="m", value=1.0),))
    profile = BUILTIN_PROFILES["standard"]

    with pytest.raises(LearningPromotionError):
        propose_promotion(
            heuristic,
            registry=registry,
            evaluation=evaluation,
            baseline_evaluation=None,
            profile=profile,
        )


@pytest.mark.parametrize(
    "status", ["contradicted", "decayed", "proposed", "promoted", "rejected"]
)
def test_propose_promotion_refuses_non_candidate_status(tmp_path: Path, status: str):
    heuristic = _heuristic(status=status)
    registry = CandidateRegistry(tmp_path / "registry")
    evaluation = _evaluation_record(candidate_id="unused", measurements=(Measurement(metric="m", value=1.0),))
    profile = BUILTIN_PROFILES["standard"]

    with pytest.raises(LearningPromotionError):
        propose_promotion(
            heuristic,
            registry=registry,
            evaluation=evaluation,
            baseline_evaluation=None,
            profile=profile,
        )


def test_propose_promotion_refuses_non_project_scope(tmp_path: Path):
    heuristic = _heuristic(scope="global")
    registry = CandidateRegistry(tmp_path / "registry")
    evaluation = _evaluation_record(candidate_id="unused", measurements=(Measurement(metric="m", value=1.0),))
    profile = BUILTIN_PROFILES["standard"]

    with pytest.raises(LearningPromotionError):
        propose_promotion(
            heuristic,
            registry=registry,
            evaluation=evaluation,
            baseline_evaluation=None,
            profile=profile,
        )


def test_propose_promotion_forbidden_configuration_key_raises_guardrail_violation_even_when_gating_checks_pass(
    tmp_path: Path,
):
    heuristic = _heuristic(
        proposed_configuration={
            "pattern": "retry-on-timeout",
            "authority_requirement": {"scopes": []},
        }
    )
    registry = CandidateRegistry(tmp_path / "registry")
    evaluation = _evaluation_record(candidate_id="unused", measurements=(Measurement(metric="m", value=1.0),))
    profile = BUILTIN_PROFILES["standard"]

    with pytest.raises(GuardrailViolation):
        propose_promotion(
            heuristic,
            registry=registry,
            evaluation=evaluation,
            baseline_evaluation=None,
            profile=profile,
        )


def test_propose_promotion_forbidden_target_embedded_in_configuration_raises_guardrail_violation(
    tmp_path: Path,
):
    # `_LEARNED_HEURISTIC_TARGET` is a fixed, trusted module constant that can
    # never collide with `_FORBIDDEN_TARGETS`, so checking only that constant
    # never exercises the guard against untrusted input. A learned heuristic's
    # own (untrusted) `proposed_configuration` can still smuggle a `target`
    # key naming a forbidden subsystem; that must be caught too.
    heuristic = _heuristic(
        proposed_configuration={
            "pattern": "retry-on-timeout",
            "target": "authority",
        }
    )
    registry = CandidateRegistry(tmp_path / "registry")
    evaluation = _evaluation_record(candidate_id="unused", measurements=(Measurement(metric="m", value=1.0),))
    profile = BUILTIN_PROFILES["standard"]

    with pytest.raises(GuardrailViolation):
        propose_promotion(
            heuristic,
            registry=registry,
            evaluation=evaluation,
            baseline_evaluation=None,
            profile=profile,
        )


@pytest.mark.parametrize("profile_name", sorted(BUILTIN_PROFILES))
def test_propose_promotion_well_formed_heuristic_is_human_required_under_every_builtin_profile(
    tmp_path: Path, profile_name: str
):
    policy = build_promotion_policy()
    measurements = _matching_measurements(policy)
    heuristic = _heuristic()
    registry = CandidateRegistry(tmp_path / "registry")
    evaluation = _evaluation_record(candidate_id="unused", measurements=measurements)
    baseline_evaluation = _evaluation_record(candidate_id="unused-baseline", measurements=measurements)
    profile = BUILTIN_PROFILES[profile_name]

    candidate, decision = propose_promotion(
        heuristic,
        registry=registry,
        evaluation=evaluation,
        baseline_evaluation=baseline_evaluation,
        profile=profile,
    )

    assert decision.outcome is PromotionOutcome.HUMAN_REQUIRED
    assert decision.outcome is not PromotionOutcome.ACCEPTED
    assert registry.get(candidate.candidate_id) is not None

    # These pin the decision to praxis_eval.promotion.evaluate_candidate's
    # real measurement-comparison output (candidate == baseline satisfies
    # every threshold, so the gate itself passes) and to guardrails'
    # required-authority-scope wording, rather than merely a top-level
    # outcome that a hardcoded PromotionDecision stub could also produce.
    assert decision.gate_result.satisfied is True
    assert decision.gate_result.candidate_id == candidate.candidate_id
    assert decision.gate_result.evaluated == tuple(t.metric for t in policy.thresholds)
    assert decision.authority_outcome == "human_required"
    assert decision.reasons == (
        f"authority scope(s) require human approval: "
        f"{guardrails._REQUIRED_PROMOTION_AUTHORITY_SCOPE}",
    )


def test_propose_promotion_marks_heuristic_proposed_in_registry_when_given(
    tmp_path: Path,
):
    # Without this, confidence.apply_observation's settled-status guard
    # (status in {"proposed", "promoted", "rejected"}) is unreachable via any
    # real pipeline flow, because nothing ever writes the status transition
    # back to the HeuristicRegistry.
    policy = build_promotion_policy()
    measurements = _matching_measurements(policy)
    heuristic = _heuristic()
    heuristic_registry = HeuristicRegistry(tmp_path / "heuristics")
    heuristic_registry.save(heuristic)
    registry = CandidateRegistry(tmp_path / "registry")
    evaluation = _evaluation_record(candidate_id="unused", measurements=measurements)
    baseline_evaluation = _evaluation_record(candidate_id="unused-baseline", measurements=measurements)
    profile = BUILTIN_PROFILES["standard"]

    propose_promotion(
        heuristic,
        registry=registry,
        evaluation=evaluation,
        baseline_evaluation=baseline_evaluation,
        profile=profile,
        heuristic_registry=heuristic_registry,
    )

    stored = heuristic_registry.get(heuristic.heuristic_id)
    assert stored is not None
    assert stored.status == "proposed"


def test_propose_promotion_does_not_touch_heuristic_registry_when_not_given(
    tmp_path: Path,
):
    # heuristic_registry is optional -- omitting it must not raise, and must
    # leave no trace (backward-compatible with every existing caller).
    policy = build_promotion_policy()
    measurements = _matching_measurements(policy)
    heuristic = _heuristic()
    registry = CandidateRegistry(tmp_path / "registry")
    evaluation = _evaluation_record(candidate_id="unused", measurements=measurements)
    baseline_evaluation = _evaluation_record(candidate_id="unused-baseline", measurements=measurements)
    profile = BUILTIN_PROFILES["standard"]

    candidate, decision = propose_promotion(
        heuristic,
        registry=registry,
        evaluation=evaluation,
        baseline_evaluation=baseline_evaluation,
        profile=profile,
    )

    assert decision.outcome is PromotionOutcome.HUMAN_REQUIRED


def test_accept_promotion_marks_heuristic_promoted_in_registry_when_given(
    tmp_path: Path,
):
    registry = CandidateRegistry(tmp_path / "registry")
    candidate = build_candidate_config(dict(_CLEAN_CONFIGURATION), target="learned-heuristic")
    registry.register(candidate)
    decision = PromotionDecision(
        outcome=PromotionOutcome.ACCEPTED,
        candidate_id=candidate.candidate_id,
        gate_result=PromotionGateResult(
            candidate_id=candidate.candidate_id,
            satisfied=True,
            reasons=(),
            evaluated=(),
        ),
        authority_outcome="human_required",
        reasons=(),
    )
    ledger = PromotionLedger(tmp_path / "ledger")
    heuristic_registry = HeuristicRegistry(tmp_path / "heuristics")
    proposed_heuristic = _heuristic(status="proposed")
    heuristic_registry.save(proposed_heuristic)

    accept_promotion(
        ledger,
        registry,
        decision,
        evaluation_ids=["eval-1"],
        heuristic=proposed_heuristic,
        heuristic_registry=heuristic_registry,
    )

    stored = heuristic_registry.get(proposed_heuristic.heuristic_id)
    assert stored is not None
    assert stored.status == "promoted"


def test_accept_promotion_round_trips_a_forced_accepted_decision_into_the_ledger(
    tmp_path: Path,
):
    registry = CandidateRegistry(tmp_path / "registry")
    candidate = build_candidate_config(dict(_CLEAN_CONFIGURATION), target="learned-heuristic")
    registry.register(candidate)
    decision = PromotionDecision(
        outcome=PromotionOutcome.ACCEPTED,
        candidate_id=candidate.candidate_id,
        gate_result=PromotionGateResult(
            candidate_id=candidate.candidate_id,
            satisfied=True,
            reasons=(),
            evaluated=(),
        ),
        authority_outcome="human_required",
        reasons=(),
    )
    ledger = PromotionLedger(tmp_path / "ledger")

    record = accept_promotion(
        ledger, registry, decision, evaluation_ids=["eval-1"]
    )

    assert record.candidate_id == candidate.candidate_id
    assert record.decision == "accepted"
    assert record.evaluation_ids == ("eval-1",)
    assert ledger.active_candidate_id() == candidate.candidate_id
