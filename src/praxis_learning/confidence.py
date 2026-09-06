"""Confidence/evidence model and contradiction/decay handling.

Confidence/decay formula: base = 1 - 1/(1 + evidence_count); decay = 0.5 **
(age_days / half_life_days); penalty = 0.25 * contradiction_count; the result
is base * decay - penalty, clamped to [0.0, 1.0].
"""

from __future__ import annotations

import dataclasses
from datetime import datetime, timezone

from praxis_learning.types import HeuristicCandidate, Observation

_DECAY_STATUS_THRESHOLD = 0.15
_CONTRADICTED_STATUS_THRESHOLD = 0.3

_SETTLED_STATUSES = {"proposed", "promoted", "rejected"}


class ConfidenceError(Exception):
    """Raised when an observation would mutate a settled heuristic."""


def compute_confidence(
    evidence_count: int,
    contradiction_count: int,
    *,
    age_days: float,
    half_life_days: float = 30.0,
) -> float:
    base = 1 - 1 / (1 + evidence_count)
    decay = 0.5 ** (age_days / half_life_days)
    penalty = 0.25 * contradiction_count
    confidence = base * decay - penalty
    return max(0.0, min(1.0, confidence))


def detect_contradiction(heuristic: HeuristicCandidate, observation: Observation) -> bool:
    return (
        observation.pattern == heuristic.pattern
        and observation.trigger == heuristic.trigger
        and observation.observed_outcome != heuristic.expected_outcome
    )


def apply_observation(
    heuristic: HeuristicCandidate,
    observation: Observation,
    *,
    now: str | None = None,
) -> HeuristicCandidate:
    if heuristic.status in _SETTLED_STATUSES:
        raise ConfidenceError(
            f"cannot apply observation to heuristic with status {heuristic.status!r}"
        )

    resolved_now = now if now is not None else datetime.now(timezone.utc).isoformat()

    is_contradiction = detect_contradiction(heuristic, observation)

    evidence_ids = heuristic.evidence_ids
    contradiction_ids = heuristic.contradiction_ids
    if is_contradiction:
        if observation.observation_id not in contradiction_ids:
            contradiction_ids = contradiction_ids + (observation.observation_id,)
    else:
        if observation.observation_id not in evidence_ids:
            evidence_ids = evidence_ids + (observation.observation_id,)

    created_at = datetime.fromisoformat(heuristic.created_at)
    now_dt = datetime.fromisoformat(resolved_now)
    age_days = (now_dt - created_at).total_seconds() / 86400.0

    confidence = compute_confidence(
        len(evidence_ids),
        len(contradiction_ids),
        age_days=age_days,
    )

    if contradiction_ids and confidence < _CONTRADICTED_STATUS_THRESHOLD:
        status = "contradicted"
    elif confidence < _DECAY_STATUS_THRESHOLD:
        status = "decayed"
    else:
        status = "candidate"

    return dataclasses.replace(
        heuristic,
        evidence_ids=evidence_ids,
        contradiction_ids=contradiction_ids,
        confidence=confidence,
        status=status,
        updated_at=resolved_now,
    )
