"""Project-to-global promotion proposal bridge into praxis_eval.

`propose_promotion` is the fail-closed gate between a project-scoped
`HeuristicCandidate` and `praxis_eval`'s candidate/evaluation/promotion
machinery: it refuses (via `LearningPromotionError`) any heuristic that isn't
`scope == "project"`, `status == "candidate"`, backed by at least
`MIN_EVIDENCE_COUNT` evidence ids, and at or above `MIN_CONFIDENCE`
confidence -- all checked before anything else happens, so a hand-set
`confidence=1.0` never compensates for too little evidence. It then still
runs the heuristic's `proposed_configuration` and the fixed learned-heuristic
target through `praxis_learning.guardrails`, which raises `GuardrailViolation`
(propagated, not wrapped) independently of those four checks, and never calls
`praxis_eval.promotion.promote` itself -- that is `accept_promotion`'s job,
kept as a separate, explicit step.

`build_promotion_policy` always attaches an `authority_requirement` demanding
`guardrails._REQUIRED_PROMOTION_AUTHORITY_SCOPE`, and self-checks that via
`guardrails.require_authority_review` before returning -- fail-closed against
a future edit accidentally weakening it. Since no `BUILTIN_PROFILE`
auto-approves that scope, `propose_promotion` can only ever produce a
`HUMAN_REQUIRED` or `REJECTED` decision, never `ACCEPTED`.
"""

from __future__ import annotations

from typing import TYPE_CHECKING

from praxis_eval import candidates, promotion
from praxis_eval.types import MetricThreshold, PromotionPolicy
from praxis_learning import guardrails

if TYPE_CHECKING:
    import praxis_eval.candidates
    import praxis_eval.ledger
    import praxis_eval.promotion
    import praxis_eval.types
    import praxis_policy.profiles
    from praxis_learning.types import HeuristicCandidate

_SPEC_VERSION = "1.0.0"

_LEARNED_HEURISTIC_TARGET = "learned-heuristic"
MIN_EVIDENCE_COUNT = 3
MIN_CONFIDENCE = 0.75


class LearningPromotionError(Exception):
    """Raised when a heuristic candidate fails the learning-promotion gating checks."""


def build_promotion_policy(
    *, extra_thresholds: tuple["praxis_eval.types.MetricThreshold", ...] = ()
) -> "praxis_eval.types.PromotionPolicy":
    thresholds = extra_thresholds
    if not thresholds:
        # promotion-policy.schema.json requires thresholds: minItems 1. A
        # learned heuristic's own minimal bar is "does not make task success
        # worse", so that is the sensible built-in default when the caller
        # supplies no project-specific health metrics of its own.
        thresholds = (
            MetricThreshold(
                metric="task_success_rate",
                constraint="required",
                direction="higher_is_better",
            ),
        )

    policy = PromotionPolicy(
        spec_version=_SPEC_VERSION,
        thresholds=thresholds,
        name="learned-heuristic-promotion",
        authority_requirement={
            "spec_version": _SPEC_VERSION,
            "scopes": [
                {
                    "scope": guardrails._REQUIRED_PROMOTION_AUTHORITY_SCOPE,
                    "constraint": "required",
                }
            ],
        },
    )
    guardrails.require_authority_review(policy)
    return policy


def propose_promotion(
    heuristic: "HeuristicCandidate",
    *,
    registry: "praxis_eval.candidates.CandidateRegistry",
    evaluation: "praxis_eval.types.EvaluationRecord",
    baseline_evaluation: "praxis_eval.types.EvaluationRecord | None",
    profile: "praxis_policy.profiles.PolicyProfile",
    granted_scopes: frozenset[str] = frozenset(),
) -> tuple["praxis_eval.types.CandidateConfig", "praxis_eval.promotion.PromotionDecision"]:
    if heuristic.scope != "project":
        raise LearningPromotionError(
            f"heuristic {heuristic.heuristic_id!r} has scope {heuristic.scope!r}, "
            "expected 'project'"
        )
    if heuristic.status != "candidate":
        raise LearningPromotionError(
            f"heuristic {heuristic.heuristic_id!r} has status {heuristic.status!r}, "
            "expected 'candidate'"
        )
    if len(heuristic.evidence_ids) < MIN_EVIDENCE_COUNT:
        raise LearningPromotionError(
            f"heuristic {heuristic.heuristic_id!r} has {len(heuristic.evidence_ids)} "
            f"evidence id(s), requires at least {MIN_EVIDENCE_COUNT}"
        )
    if heuristic.confidence < MIN_CONFIDENCE:
        raise LearningPromotionError(
            f"heuristic {heuristic.heuristic_id!r} has confidence "
            f"{heuristic.confidence!r}, requires at least {MIN_CONFIDENCE}"
        )

    guardrails.check_configuration(heuristic.proposed_configuration)
    guardrails.check_target(_LEARNED_HEURISTIC_TARGET)

    candidate = candidates.build_candidate_config(
        heuristic.proposed_configuration,
        target=_LEARNED_HEURISTIC_TARGET,
        description=heuristic.description,
    )
    registry.register(candidate)

    policy = build_promotion_policy()
    decision = promotion.evaluate_candidate(
        candidate_id=candidate.candidate_id,
        candidate_measurements=evaluation.measurements,
        baseline_measurements=(
            baseline_evaluation.measurements if baseline_evaluation else None
        ),
        policy=policy,
        profile=profile,
        granted_scopes=granted_scopes,
    )
    return candidate, decision


def accept_promotion(
    ledger: "praxis_eval.ledger.PromotionLedger",
    registry: "praxis_eval.candidates.CandidateRegistry",
    decision: "praxis_eval.promotion.PromotionDecision",
    *,
    evaluation_ids: list[str],
) -> "praxis_eval.types.PromotionRecord":
    return promotion.promote(ledger, registry, decision, evaluation_ids=evaluation_ids)
