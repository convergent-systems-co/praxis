"""Tests for the append-only, project-scoped observation log.

ObservationLog persists Observations as JSONL, rejecting a duplicate
`observation_id` outright, so a caller retry after a crash can never
double-append an observation. Unlike praxis_eval.ledger.PromotionLedger,
there is no `seq` to reassign -- Observation has no `seq` field, so dedupe
is purely on `observation_id`. `read_for_project` is the read-side
enforcement that a project's observations are queried scoped, by default.
"""

from __future__ import annotations

from dataclasses import replace
from pathlib import Path

import pytest

from praxis_learning.observations import ObservationLog, ObservationLogError
from praxis_learning.types import Observation


def _make_observation(
    *,
    observation_id: str,
    project_id: str = "proj-a",
    pattern: str = "recurrent-failure",
    observed_outcome: str = "retry succeeded",
    source_event_ids: tuple[str, ...] = ("evt-1",),
    trigger: dict | None = None,
) -> Observation:
    return Observation(
        spec_version="1.0.0",
        observation_id=observation_id,
        project_id=project_id,
        pattern=pattern,
        trigger=dict(trigger) if trigger is not None else {"node_id": "n1"},
        observed_outcome=observed_outcome,
        source_event_ids=source_event_ids,
        observed_at="2026-09-06T00:00:00Z",
    )


def test_append_then_read_all_round_trips(tmp_path: Path) -> None:
    log = ObservationLog(tmp_path)

    first = log.append(_make_observation(observation_id="obs-1"))
    second = log.append(_make_observation(observation_id="obs-2"))

    records = log.read_all()

    assert [record.observation_id for record in records] == ["obs-1", "obs-2"]
    assert records[0] == first
    assert records[1] == second


def test_reopening_reconstructs_all_previously_appended_observations(tmp_path: Path) -> None:
    log = ObservationLog(tmp_path)
    log.append(_make_observation(observation_id="obs-1"))
    log.append(_make_observation(observation_id="obs-2"))

    reopened = ObservationLog(tmp_path)
    records = reopened.read_all()

    assert [record.observation_id for record in records] == ["obs-1", "obs-2"]


def test_duplicate_observation_id_raises_and_does_not_corrupt_file(tmp_path: Path) -> None:
    log = ObservationLog(tmp_path)
    log.append(_make_observation(observation_id="obs-1", project_id="proj-a"))

    with pytest.raises(ObservationLogError):
        log.append(_make_observation(observation_id="obs-1", project_id="proj-b"))

    records = log.read_all()
    assert [record.observation_id for record in records] == ["obs-1"]
    assert records[0].project_id == "proj-a"

    reopened = ObservationLog(tmp_path)
    reopened_records = reopened.read_all()
    assert [record.observation_id for record in reopened_records] == ["obs-1"]
    assert reopened_records[0].project_id == "proj-a"


def test_append_raises_on_schema_violation_and_does_not_write_it(tmp_path: Path) -> None:
    log = ObservationLog(tmp_path)
    invalid = replace(_make_observation(observation_id="obs-invalid"), confidence=1.5)

    with pytest.raises(ObservationLogError):
        log.append(invalid)

    assert log.read_all() == []

    reopened = ObservationLog(tmp_path)
    assert reopened.read_all() == []


def test_read_for_project_filters_to_matching_project_id(tmp_path: Path) -> None:
    log = ObservationLog(tmp_path)
    log.append(_make_observation(observation_id="obs-1", project_id="proj-a"))
    log.append(_make_observation(observation_id="obs-2", project_id="proj-b"))
    log.append(_make_observation(observation_id="obs-3", project_id="proj-a"))

    scoped = log.read_for_project("proj-a")

    assert [record.observation_id for record in scoped] == ["obs-1", "obs-3"]


def test_two_instances_on_same_directory_serialize_appends_without_loss(tmp_path: Path) -> None:
    first_log = ObservationLog(tmp_path)
    second_log = ObservationLog(tmp_path)

    first_log.append(_make_observation(observation_id="obs-1"))
    second_log.append(_make_observation(observation_id="obs-2"))
    first_log.append(_make_observation(observation_id="obs-3"))

    records = first_log.read_all()
    assert [record.observation_id for record in records] == ["obs-1", "obs-2", "obs-3"]

    reread_from_second = second_log.read_all()
    assert [record.observation_id for record in reread_from_second] == [
        "obs-1",
        "obs-2",
        "obs-3",
    ]


def test_second_instance_rejects_observation_id_appended_by_first_instance(
    tmp_path: Path,
) -> None:
    first_log = ObservationLog(tmp_path)
    second_log = ObservationLog(tmp_path)

    first_log.append(_make_observation(observation_id="dup-1", project_id="proj-a"))

    with pytest.raises(ObservationLogError):
        second_log.append(_make_observation(observation_id="dup-1", project_id="proj-b"))

    records = first_log.read_all()
    assert [record.observation_id for record in records] == ["dup-1"]
    assert records[0].project_id == "proj-a"
