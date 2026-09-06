"""Candidate registry: content-addressed identity and durable storage.

compute_candidate_id canonical-encodes `configuration` via
json.dumps(configuration, sort_keys=True, separators=(",", ":")), prefixes it
with f"{parent_candidate_id or ''}\\n", and hashes the UTF-8 bytes with
SHA-256 -- so identity is a pure function of content and lineage, never of
dict key insertion order. CandidateRegistry stores one JSON document per
candidate_id under a directory, written atomically (mirroring
praxis_runtime.state.RunStateStore.save's .tmp + os.replace pattern).
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path

import pytest

from praxis_eval.candidates import (
    CandidateRegistry,
    CandidateRegistryError,
    build_candidate_config,
    compute_candidate_id,
)
from praxis_eval.types import CandidateConfig


def _expected_id(configuration: dict, parent_candidate_id: str | None = None) -> str:
    encoded = json.dumps(configuration, sort_keys=True, separators=(",", ":"))
    payload = f"{parent_candidate_id or ''}\n{encoded}"
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()


def test_compute_candidate_id_ignores_key_insertion_order():
    config_a = {"alpha": 1, "beta": 2}
    config_b = {"beta": 2, "alpha": 1}

    assert compute_candidate_id(config_a) == compute_candidate_id(config_b)
    assert compute_candidate_id(config_a) == _expected_id(config_a)


def test_compute_candidate_id_differs_for_different_configuration_content():
    assert compute_candidate_id({"alpha": 1}) != compute_candidate_id({"alpha": 2})


def test_compute_candidate_id_differs_for_different_parent_with_same_configuration():
    configuration = {"alpha": 1}

    assert compute_candidate_id(
        configuration, parent_candidate_id="parent-1"
    ) != compute_candidate_id(configuration, parent_candidate_id="parent-2")
    assert compute_candidate_id(configuration) != compute_candidate_id(
        configuration, parent_candidate_id="parent-1"
    )


def test_build_candidate_config_round_trips_through_register_and_get(tmp_path: Path):
    registry = CandidateRegistry(tmp_path)
    config = build_candidate_config(
        {"alpha": 1}, target="routing", description="a test candidate"
    )

    registered = registry.register(config)
    fetched = registry.get(config.candidate_id)

    assert config.spec_version == "1.0.0"
    assert config.candidate_id == compute_candidate_id({"alpha": 1})
    assert registered == config
    assert fetched == config


def test_reregistering_identical_candidate_config_is_noop(tmp_path: Path):
    registry = CandidateRegistry(tmp_path)
    config = build_candidate_config({"alpha": 1})
    first = registry.register(config)

    second = registry.register(config)

    assert second == first == config


def test_reregistering_same_configuration_with_different_metadata_returns_original(
    tmp_path: Path,
):
    registry = CandidateRegistry(tmp_path)
    original = build_candidate_config(
        {"alpha": 1}, created_at="2020-01-01T00:00:00Z"
    )
    registry.register(original)

    resubmission = CandidateConfig(
        spec_version="1.0.0",
        candidate_id=original.candidate_id,
        configuration={"alpha": 1},
        created_at="2021-06-15T00:00:00Z",
    )

    result = registry.register(resubmission)

    assert result == original
    assert registry.get(original.candidate_id) == original


def test_registering_mismatched_configuration_for_existing_id_raises(tmp_path: Path):
    registry = CandidateRegistry(tmp_path)
    real_candidate = build_candidate_config({"alpha": 1})
    registry.register(real_candidate)

    forged = CandidateConfig(
        spec_version="1.0.0",
        candidate_id=real_candidate.candidate_id,
        configuration={"alpha": 999},
        created_at="2021-06-15T00:00:00Z",
    )

    with pytest.raises(CandidateRegistryError):
        registry.register(forged)


def test_get_unknown_candidate_returns_none(tmp_path: Path):
    registry = CandidateRegistry(tmp_path)

    assert registry.get("does-not-exist") is None
