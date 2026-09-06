"""Heuristic registry: content-addressed identity + clustering/deduplication.

compute_heuristic_id derives a heuristic_id purely from `project_id`,
`pattern`, and `trigger` (deliberately excluding evidence/confidence/status),
so two observations that share those three fields but differ in
observation_id always cluster onto the same heuristic_id.
HeuristicRegistry.save overwrites in place by heuristic_id (unlike
CandidateRegistry.register's reject-on-mismatch), because a HeuristicCandidate
is expected to mutate its evidence/confidence/status fields under the same id
as new observations arrive.
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

import pytest

from praxis_learning.heuristics import (
    HeuristicRegistry,
    HeuristicRegistryError,
    build_heuristic_candidate_from_observation,
    cluster_key,
    compute_heuristic_id,
)
from praxis_learning.types import HEURISTIC_CANDIDATE_SCHEMA_PATH, Observation


def _expected_id(project_id: str, pattern: str, trigger: dict) -> str:
    encoded = json.dumps(
        {"project_id": project_id, "pattern": pattern, "trigger": trigger},
        sort_keys=True,
        separators=(",", ":"),
    )
    return hashlib.sha256(encoded.encode("utf-8")).hexdigest()


def _observation(observation_id: str, **overrides) -> Observation:
    fields = {
        "spec_version": "1.0.0",
        "observation_id": observation_id,
        "project_id": "proj-1",
        "pattern": "recurrent-failure",
        "trigger": {"tool": "bash", "error": "timeout"},
        "observed_outcome": "task failed",
        "source_event_ids": ("event-1",),
        "observed_at": "2026-01-01T00:00:00Z",
    }
    fields.update(overrides)
    return Observation(**fields)


def test_compute_heuristic_id_matches_canonical_encoding():
    assert compute_heuristic_id(
        "proj-1", "recurrent-failure", {"tool": "bash", "error": "timeout"}
    ) == _expected_id("proj-1", "recurrent-failure", {"tool": "bash", "error": "timeout"})


def test_compute_heuristic_id_ignores_trigger_key_insertion_order():
    trigger_a = {"tool": "bash", "error": "timeout"}
    trigger_b = {"error": "timeout", "tool": "bash"}

    assert compute_heuristic_id("proj-1", "recurrent-failure", trigger_a) == (
        compute_heuristic_id("proj-1", "recurrent-failure", trigger_b)
    )


def test_compute_heuristic_id_differs_for_different_pattern_or_trigger():
    base = compute_heuristic_id("proj-1", "recurrent-failure", {"tool": "bash"})

    assert base != compute_heuristic_id("proj-1", "successful-recovery", {"tool": "bash"})
    assert base != compute_heuristic_id("proj-1", "recurrent-failure", {"tool": "git"})
    assert base != compute_heuristic_id("proj-2", "recurrent-failure", {"tool": "bash"})


def test_build_heuristic_candidate_from_observation_sets_expected_defaults():
    observation = _observation("obs-1")

    candidate = build_heuristic_candidate_from_observation(
        observation,
        proposed_configuration={"retry_limit": 3},
        expected_outcome="task failed",
        created_at="2026-01-01T00:00:00Z",
    )

    assert candidate.heuristic_id == compute_heuristic_id(
        observation.project_id, observation.pattern, observation.trigger
    )
    assert candidate.scope == "project"
    assert candidate.status == "candidate"
    assert candidate.confidence == 0.5
    assert candidate.evidence_ids == ("obs-1",)
    assert candidate.contradiction_ids == ()
    assert candidate.created_at == "2026-01-01T00:00:00Z"
    assert candidate.updated_at == "2026-01-01T00:00:00Z"
    assert candidate.project_id == observation.project_id
    assert candidate.pattern == observation.pattern
    assert candidate.trigger == observation.trigger
    assert candidate.proposed_configuration == {"retry_limit": 3}
    assert candidate.expected_outcome == "task failed"


def test_two_observations_with_same_identity_fields_cluster_to_same_heuristic_id():
    observation_a = _observation("obs-a")
    observation_b = _observation("obs-b")

    candidate_a = build_heuristic_candidate_from_observation(
        observation_a,
        proposed_configuration={"retry_limit": 3},
        expected_outcome="task failed",
    )
    candidate_b = build_heuristic_candidate_from_observation(
        observation_b,
        proposed_configuration={"retry_limit": 3},
        expected_outcome="task failed",
    )

    assert candidate_a.heuristic_id == candidate_b.heuristic_id
    assert candidate_a.evidence_ids != candidate_b.evidence_ids


def test_cluster_key_matches_compute_heuristic_id_pattern_trigger_component():
    trigger = {"tool": "bash", "error": "timeout"}

    assert cluster_key("recurrent-failure", trigger) == cluster_key(
        "recurrent-failure", dict(reversed(list(trigger.items())))
    )
    assert cluster_key("recurrent-failure", trigger) != cluster_key(
        "successful-recovery", trigger
    )


def test_save_registers_and_get_round_trips(tmp_path: Path):
    registry = HeuristicRegistry(tmp_path)
    observation = _observation("obs-1")
    candidate = build_heuristic_candidate_from_observation(
        observation,
        proposed_configuration={"retry_limit": 3},
        expected_outcome="task failed",
    )

    saved = registry.save(candidate)
    fetched = registry.get(candidate.heuristic_id)

    assert saved == candidate
    assert fetched == candidate


def test_save_overwrites_in_place_rather_than_rejecting(tmp_path: Path):
    registry = HeuristicRegistry(tmp_path)
    observation = _observation("obs-1")
    candidate = build_heuristic_candidate_from_observation(
        observation,
        proposed_configuration={"retry_limit": 3},
        expected_outcome="task failed",
        created_at="2026-01-01T00:00:00Z",
    )
    registry.save(candidate)

    updated = build_heuristic_candidate_from_observation(
        _observation("obs-2"),
        proposed_configuration={"retry_limit": 5},
        expected_outcome="task failed",
        created_at="2026-01-02T00:00:00Z",
    )
    assert updated.heuristic_id == candidate.heuristic_id

    result = registry.save(updated)
    fetched = registry.get(candidate.heuristic_id)

    assert result == updated
    assert fetched == updated
    assert fetched.proposed_configuration == {"retry_limit": 5}
    assert fetched.evidence_ids == ("obs-2",)


def test_get_unknown_heuristic_returns_none(tmp_path: Path):
    registry = HeuristicRegistry(tmp_path)

    assert registry.get("does-not-exist") is None


def test_get_raises_heuristic_registry_error_on_schema_invalid_stored_document(tmp_path: Path):
    # A corrupted/hand-edited document on disk must fail closed with a
    # domain-specific error, not an unrelated KeyError/TypeError from
    # heuristic_candidate_from_document, and not silently pass through.
    registry = HeuristicRegistry(tmp_path)
    heuristic_id = "corrupt-heuristic"
    file_path = tmp_path / f"{heuristic_id}.json"
    file_path.write_text(json.dumps({"spec_version": "1.0.0", "heuristic_id": heuristic_id}))

    with pytest.raises(HeuristicRegistryError):
        registry.get(heuristic_id)


def test_list_for_project_raises_heuristic_registry_error_on_schema_invalid_stored_document(
    tmp_path: Path,
):
    registry = HeuristicRegistry(tmp_path)
    heuristic_id = "corrupt-heuristic"
    file_path = tmp_path / f"{heuristic_id}.json"
    file_path.write_text(
        json.dumps({"spec_version": "1.0.0", "heuristic_id": heuristic_id, "project_id": "proj-1"})
    )

    with pytest.raises(HeuristicRegistryError):
        registry.list_for_project("proj-1")


def test_list_for_project_returns_only_matching_project(tmp_path: Path):
    registry = HeuristicRegistry(tmp_path)
    candidate_proj1 = build_heuristic_candidate_from_observation(
        _observation("obs-1", project_id="proj-1"),
        proposed_configuration={"retry_limit": 3},
        expected_outcome="task failed",
    )
    candidate_proj2 = build_heuristic_candidate_from_observation(
        _observation("obs-2", project_id="proj-2"),
        proposed_configuration={"retry_limit": 3},
        expected_outcome="task failed",
    )
    registry.save(candidate_proj1)
    registry.save(candidate_proj2)

    result = registry.list_for_project("proj-1")

    assert result == [candidate_proj1]


def test_build_heuristic_candidate_validates_against_schema():
    observation = _observation("obs-1")

    candidate = build_heuristic_candidate_from_observation(
        observation,
        proposed_configuration={"retry_limit": 3},
        expected_outcome="task failed",
    )

    from praxis_contracts.validator import validate_document
    from praxis_learning.types import heuristic_candidate_to_document

    validate_document(
        heuristic_candidate_to_document(candidate), HEURISTIC_CANDIDATE_SCHEMA_PATH
    )
