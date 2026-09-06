"""Confidence/evidence model and contradiction/decay handling.

Covers the documented confidence formula at several evidence counts, decay
and contradiction pushing status to "decayed"/"contradicted", the fail-closed
ConfidenceError for heuristics already in or past promotion, and idempotent
observation application (repeated observation_id does not double-count).
"""

from __future__ import annotations

import pytest

from praxis_learning.confidence import (
    ConfidenceError,
    apply_observation,
    compute_confidence,
    detect_contradiction,
)
from praxis_learning.types import HeuristicCandidate, Observation

_PATTERN = "retry-on-timeout"
_TRIGGER = {"event": "timeout"}


def _heuristic(**overrides) -> HeuristicCandidate:
    fields = dict(
        spec_version="1.0.0",
        heuristic_id="h1",
        project_id="p1",
        scope="project",
        pattern=_PATTERN,
        trigger=dict(_TRIGGER),
        expected_outcome="retry",
        proposed_configuration={},
        status="candidate",
        confidence=0.5,
        evidence_ids=(),
        created_at="2026-08-01T00:00:00+00:00",
        updated_at="2026-08-01T00:00:00+00:00",
    )
    fields.update(overrides)
    return HeuristicCandidate(**fields)


def _observation(**overrides) -> Observation:
    fields = dict(
        spec_version="1.0.0",
        observation_id="o1",
        project_id="p1",
        pattern=_PATTERN,
        trigger=dict(_TRIGGER),
        observed_outcome="retry",
        source_event_ids=("e1",),
        observed_at="2026-09-01T00:00:00+00:00",
    )
    fields.update(overrides)
    return Observation(**fields)


@pytest.mark.parametrize(
    "evidence_count,expected",
    [
        (0, 0.0),
        (1, 0.5),
        (2, 0.667),
        (3, 0.75),
        (4, 0.8),
    ],
)
def test_compute_confidence_base_values_at_zero_age_no_contradictions(evidence_count, expected):
    confidence = compute_confidence(evidence_count, 0, age_days=0.0)

    assert confidence == pytest.approx(expected, abs=1e-3)


def test_compute_confidence_decays_by_half_at_one_half_life():
    fresh = compute_confidence(4, 0, age_days=0.0)
    decayed = compute_confidence(4, 0, age_days=30.0, half_life_days=30.0)

    assert decayed == pytest.approx(fresh * 0.5, abs=1e-6)


def test_compute_confidence_applies_contradiction_penalty_and_clamps():
    # base=1-1/(1+4)=0.8, decay=1.0, penalty=0.25*3=0.75 -> 0.05
    confidence = compute_confidence(4, 3, age_days=0.0)
    assert confidence == pytest.approx(0.05, abs=1e-6)

    # Enough contradictions/decay to clamp at the floor.
    floored = compute_confidence(0, 10, age_days=0.0)
    assert floored == 0.0


def test_detect_contradiction_true_when_same_pattern_trigger_different_outcome():
    heuristic = _heuristic(expected_outcome="retry")
    observation = _observation(observed_outcome="give-up")

    assert detect_contradiction(heuristic, observation) is True


def test_detect_contradiction_false_when_outcome_matches():
    heuristic = _heuristic(expected_outcome="retry")
    observation = _observation(observed_outcome="retry")

    assert detect_contradiction(heuristic, observation) is False


@pytest.mark.parametrize("field,value", [("pattern", "other-pattern"), ("trigger", {"event": "other"})])
def test_detect_contradiction_false_when_pattern_or_trigger_differs(field, value):
    heuristic = _heuristic(expected_outcome="retry")
    observation = _observation(observed_outcome="give-up", **{field: value})

    assert detect_contradiction(heuristic, observation) is False


def test_apply_observation_corroborating_evidence_updates_confidence_and_evidence_ids():
    heuristic = _heuristic(
        status="candidate",
        confidence=0.0,
        evidence_ids=(),
        created_at="2026-09-01T00:00:00+00:00",
        updated_at="2026-09-01T00:00:00+00:00",
    )
    observation = _observation(observation_id="o1", observed_outcome="retry")

    updated = apply_observation(heuristic, observation, now="2026-09-01T00:00:00+00:00")

    assert updated is not heuristic
    assert updated.evidence_ids == ("o1",)
    assert updated.contradiction_ids == ()
    assert updated.confidence == pytest.approx(0.5, abs=1e-3)
    assert updated.status == "candidate"
    assert updated.updated_at == "2026-09-01T00:00:00+00:00"
    # Original is untouched (frozen-dataclass, dataclasses.replace convention).
    assert heuristic.evidence_ids == ()
    assert heuristic.confidence == 0.0


def test_apply_observation_contradiction_can_flip_status_to_contradicted():
    heuristic = _heuristic(
        status="candidate",
        confidence=0.05,
        evidence_ids=(),
        contradiction_ids=(),
        created_at="2026-09-01T00:00:00+00:00",
    )
    observation = _observation(observation_id="o-bad", observed_outcome="give-up")

    updated = apply_observation(heuristic, observation, now="2026-09-01T00:00:00+00:00")

    assert updated.contradiction_ids == ("o-bad",)
    assert updated.evidence_ids == ()
    assert updated.confidence < 0.3
    assert updated.status == "contradicted"


def test_apply_observation_decay_alone_can_flip_status_to_decayed():
    heuristic = _heuristic(
        status="candidate",
        confidence=0.8,
        evidence_ids=("o0",),
        created_at="2026-01-01T00:00:00+00:00",
    )
    # 1 evidence, no new observation content changes evidence, but confidence
    # is recomputed from age alone: base=0.5 at evidence_count=1, heavy decay
    # over many half-lives pushes confidence below the decay threshold.
    observation = _observation(observation_id="o0", observed_outcome="retry")

    updated = apply_observation(heuristic, observation, now="2027-01-01T00:00:00+00:00")

    assert updated.confidence < 0.15
    assert updated.status == "decayed"


@pytest.mark.parametrize("status", ["proposed", "promoted", "rejected"])
def test_apply_observation_raises_confidence_error_for_settled_statuses(status):
    heuristic = _heuristic(status=status)
    observation = _observation()

    with pytest.raises(ConfidenceError):
        apply_observation(heuristic, observation, now="2026-09-01T00:00:00+00:00")


def test_apply_observation_repeated_observation_id_is_idempotent_for_evidence():
    heuristic = _heuristic(
        status="candidate",
        confidence=0.0,
        evidence_ids=(),
        created_at="2026-09-01T00:00:00+00:00",
        updated_at="2026-09-01T00:00:00+00:00",
    )
    observation = _observation(observation_id="o1", observed_outcome="retry")

    once = apply_observation(heuristic, observation, now="2026-09-01T00:00:00+00:00")
    twice = apply_observation(once, observation, now="2026-09-01T00:00:00+00:00")

    assert twice.evidence_ids == ("o1",)
    assert twice.confidence == once.confidence


def test_apply_observation_decays_from_last_reinforcement_not_creation():
    # Reinforced 9 times since creation, most recently 1 day before `now`;
    # created 10 days before `now`. docs/learning.md documents decay as
    # happening "since the heuristic's last reinforcement" (updated_at), so
    # only the 1-day gap since the last reinforcement should count -- not the
    # full 10-day gap since creation. At evidence_count=10, base=1-1/11=0.9091;
    # decay over 1 day (half_life=30) is ~0.977, giving confidence ~0.888,
    # comfortably above MIN_CONFIDENCE=0.75. Anchoring decay to created_at
    # instead would use age_days=10, giving confidence ~0.7215 -- below
    # MIN_CONFIDENCE -- which is exactly the bug this test pins.
    heuristic = _heuristic(
        status="candidate",
        confidence=0.8,
        evidence_ids=tuple(f"o{i}" for i in range(9)),
        created_at="2026-08-22T00:00:00+00:00",
        updated_at="2026-08-31T00:00:00+00:00",
    )
    observation = _observation(observation_id="o9", observed_outcome="retry")

    updated = apply_observation(heuristic, observation, now="2026-09-01T00:00:00+00:00")

    assert updated.evidence_ids == tuple(f"o{i}" for i in range(9)) + ("o9",)
    assert updated.confidence > 0.75
    assert updated.confidence == pytest.approx(0.8884, abs=1e-3)


def test_apply_observation_repeated_observation_id_is_idempotent_for_contradiction():
    heuristic = _heuristic(
        status="candidate",
        confidence=0.5,
        contradiction_ids=(),
        created_at="2026-09-01T00:00:00+00:00",
    )
    observation = _observation(observation_id="o-bad", observed_outcome="give-up")

    once = apply_observation(heuristic, observation, now="2026-09-01T00:00:00+00:00")
    twice = apply_observation(once, observation, now="2026-09-01T00:00:00+00:00")

    assert twice.contradiction_ids == ("o-bad",)
    assert twice.confidence == once.confidence
