"""End-to-end bounded-learning lifecycle test.

Exercises the public interfaces of T1-T8 together against real
temp-directory-backed `ObservationLog`/`HeuristicRegistry`/
`praxis_eval.candidates.CandidateRegistry` instances -- no mocking of this
package's own modules (mirrors `tests/test_promotion_end_to_end.py`'s own
stated discipline). `pipeline.ingest_telemetry` feeds a sequence of telemetry
records that first accumulates too little evidence to promote, then enough
evidence and confidence to reach the promotion bridge, then a contradiction
that knocks it back below the promotion bar -- proving "one observation
cannot become an active global rule" end to end. A second test proves the
"cannot modify authority/policy/security/graph-legality without explicit
reviewed promotion" acceptance criterion at the full pipeline-to-promotion-
bridge seam, not just `guardrails.py` in isolation.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from praxis_eval.candidates import CandidateRegistry
from praxis_eval.promotion import PromotionOutcome
from praxis_eval.types import EvaluationRecord, Measurement
from praxis_learning.guardrails import GuardrailViolation
from praxis_learning.heuristics import HeuristicRegistry, compute_heuristic_id
from praxis_learning.observations import ObservationLog
from praxis_learning.pipeline import ingest_telemetry
from praxis_learning.promotion_bridge import (
    LearningPromotionError,
    build_promotion_policy,
    propose_promotion,
)
from praxis_policy.profiles import BUILTIN_PROFILES

_PROJECT_ID = "proj-e2e"
_SPEC_VERSION = "1.0.0"
_TIMESTAMP = "2026-09-06T00:00:00Z"


def _correction_record(
    *,
    event_id: str,
    node_id: str = "n1",
    previous_outcome: str = "fail",
    corrected_outcome: str = "recover",
    seq: int = 0,
) -> dict:
    return {
        "event_id": event_id,
        "node_id": node_id,
        "event_type": "correction",
        "payload": {"previous_outcome": previous_outcome, "corrected_outcome": corrected_outcome},
        "seq": seq,
    }


def _heuristic_id(*, node_id: str = "n1", previous_outcome: str = "fail") -> str:
    return compute_heuristic_id(
        _PROJECT_ID, "correction", {"node_id": node_id, "previous_outcome": previous_outcome}
    )


def _matching_evaluations(policy) -> tuple[EvaluationRecord, EvaluationRecord]:
    # Equal candidate/baseline values satisfy any `required` threshold's
    # tolerance check regardless of direction or max_regression_pct (both
    # directions' `>=`/`<=` comparisons hold at equality), so this stays
    # agnostic to whatever default metric build_promotion_policy chooses.
    measurements = tuple(Measurement(metric=t.metric, value=1.0) for t in policy.thresholds)
    evaluation = EvaluationRecord(
        spec_version=_SPEC_VERSION,
        evaluation_id="eval-candidate",
        candidate_id="unused",
        workload_id="workload-e2e",
        measurements=measurements,
        produced_at=_TIMESTAMP,
    )
    baseline_evaluation = EvaluationRecord(
        spec_version=_SPEC_VERSION,
        evaluation_id="eval-baseline",
        candidate_id="unused-baseline",
        workload_id="workload-e2e",
        measurements=measurements,
        produced_at=_TIMESTAMP,
    )
    return evaluation, baseline_evaluation


def test_lifecycle_thin_evidence_then_sufficient_then_contradicted(tmp_path: Path) -> None:
    log = ObservationLog(tmp_path / "observations")
    heuristic_registry = HeuristicRegistry(tmp_path / "heuristics")
    candidate_registry = CandidateRegistry(tmp_path / "candidates")
    policy = build_promotion_policy()
    evaluation, baseline_evaluation = _matching_evaluations(policy)
    profile = BUILTIN_PROFILES["standard"]
    heuristic_id = _heuristic_id()

    # (a) One corroborating observation is too thin to promote: evidence
    # count is below MIN_EVIDENCE_COUNT regardless of confidence.
    ingest_telemetry(
        [_correction_record(event_id="e1")],
        project_id=_PROJECT_ID,
        observation_log=log,
        heuristic_registry=heuristic_registry,
    )
    thin = heuristic_registry.get(heuristic_id)
    assert thin is not None
    assert len(thin.evidence_ids) == 1

    with pytest.raises(LearningPromotionError):
        propose_promotion(
            thin,
            registry=candidate_registry,
            evaluation=evaluation,
            baseline_evaluation=baseline_evaluation,
            profile=profile,
        )

    # Still too thin at two corroborating observations.
    ingest_telemetry(
        [_correction_record(event_id="e2")],
        project_id=_PROJECT_ID,
        observation_log=log,
        heuristic_registry=heuristic_registry,
    )
    still_thin = heuristic_registry.get(heuristic_id)
    assert still_thin is not None
    assert len(still_thin.evidence_ids) == 2

    with pytest.raises(LearningPromotionError):
        propose_promotion(
            still_thin,
            registry=candidate_registry,
            evaluation=evaluation,
            baseline_evaluation=baseline_evaluation,
            profile=profile,
        )

    # (b) Accumulate to four corroborating observations -- past
    # MIN_EVIDENCE_COUNT (3) with enough margin above MIN_CONFIDENCE (0.75)
    # that the formula's tiny real-clock time-decay term can never tip it
    # back under the threshold. propose_promotion now succeeds, but the
    # decision is HUMAN_REQUIRED under a BUILTIN_PROFILE, never ACCEPTED --
    # "no explicit reviewed promotion, no activation."
    ingest_telemetry(
        [_correction_record(event_id="e3")],
        project_id=_PROJECT_ID,
        observation_log=log,
        heuristic_registry=heuristic_registry,
    )
    ingest_telemetry(
        [_correction_record(event_id="e4")],
        project_id=_PROJECT_ID,
        observation_log=log,
        heuristic_registry=heuristic_registry,
    )
    sufficient = heuristic_registry.get(heuristic_id)
    assert sufficient is not None
    assert len(sufficient.evidence_ids) == 4
    assert sufficient.confidence >= 0.75

    candidate, decision = propose_promotion(
        sufficient,
        registry=candidate_registry,
        evaluation=evaluation,
        baseline_evaluation=baseline_evaluation,
        profile=profile,
    )
    assert decision.outcome is PromotionOutcome.HUMAN_REQUIRED
    assert decision.outcome is not PromotionOutcome.ACCEPTED
    assert candidate_registry.get(candidate.candidate_id) is not None

    # (c) A contradicting observation against the already-sufficiently-
    # evidenced heuristic lowers its confidence back under MIN_CONFIDENCE, so
    # propose_promotion now raises for that heuristic again.
    ingest_telemetry(
        [_correction_record(event_id="e5", corrected_outcome="give-up")],
        project_id=_PROJECT_ID,
        observation_log=log,
        heuristic_registry=heuristic_registry,
    )
    contradicted = heuristic_registry.get(heuristic_id)
    assert contradicted is not None
    assert len(contradicted.contradiction_ids) == 1
    assert contradicted.confidence < sufficient.confidence

    with pytest.raises(LearningPromotionError):
        propose_promotion(
            contradicted,
            registry=candidate_registry,
            evaluation=evaluation,
            baseline_evaluation=baseline_evaluation,
            profile=profile,
        )


def test_forbidden_configuration_key_rejected_at_pipeline_to_promotion_bridge_seam(
    tmp_path: Path,
) -> None:
    log = ObservationLog(tmp_path / "observations")
    heuristic_registry = HeuristicRegistry(tmp_path / "heuristics")
    candidate_registry = CandidateRegistry(tmp_path / "candidates")
    policy = build_promotion_policy()
    evaluation, baseline_evaluation = _matching_evaluations(policy)
    profile = BUILTIN_PROFILES["standard"]
    heuristic_id = _heuristic_id()

    forbidden_configuration = {
        "pattern": "retry-on-timeout",
        "authority_requirement": {"scopes": []},
    }

    # Accumulate ample evidence/confidence (mirrors (b) above) so the
    # gating checks in propose_promotion would pass on their own -- the
    # guardrail rejection below must come from the configuration content,
    # not from a starved candidate.
    for i in range(4):
        ingest_telemetry(
            [_correction_record(event_id=f"forbidden-{i}")],
            project_id=_PROJECT_ID,
            observation_log=log,
            heuristic_registry=heuristic_registry,
            default_proposed_configuration=forbidden_configuration,
        )

    heuristic = heuristic_registry.get(heuristic_id)
    assert heuristic is not None
    assert len(heuristic.evidence_ids) == 4
    assert heuristic.confidence >= 0.75
    assert heuristic.proposed_configuration == forbidden_configuration

    with pytest.raises(GuardrailViolation):
        propose_promotion(
            heuristic,
            registry=candidate_registry,
            evaluation=evaluation,
            baseline_evaluation=baseline_evaluation,
            profile=profile,
        )
