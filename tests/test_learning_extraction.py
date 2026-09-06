"""Observation/event extraction from telemetry records.

Covers the four illustrative classifiers (recurrent-failure, successful-
recovery, correction, workflow-efficiency), the fail-soft skip of malformed
records, and that every emitted Observation validates against
OBSERVATION_SCHEMA_PATH.
"""

from __future__ import annotations

from praxis_contracts.validator import validate_document
from praxis_learning.extraction import extract_observations
from praxis_learning.types import Observation, OBSERVATION_SCHEMA_PATH, observation_to_document

_PROJECT_ID = "proj-1"


def _validate_all(observations):
    for observation in observations:
        validate_document(observation_to_document(observation), OBSERVATION_SCHEMA_PATH)


def test_extract_observations_recurrent_failure():
    records = [
        {
            "event_id": "e1",
            "node_id": "n1",
            "event_type": "fail",
            "payload": {"failure_class": "timeout"},
            "seq": 0,
        },
        {
            "event_id": "e2",
            "node_id": "n1",
            "event_type": "fail",
            "payload": {"failure_class": "timeout"},
            "seq": 1,
        },
    ]

    observations = extract_observations(records, project_id=_PROJECT_ID)

    recurrent = [o for o in observations if o.pattern == "recurrent-failure"]
    assert len(recurrent) == 1
    observation = recurrent[0]
    assert isinstance(observation, Observation)
    assert observation.project_id == _PROJECT_ID
    assert observation.trigger == {"node_id": "n1", "failure_class": "timeout"}
    assert observation.observed_outcome == "fail"
    assert observation.source_event_ids == ("e1", "e2")
    assert observation.observation_id
    assert observation.observed_at
    _validate_all(observations)


def test_extract_observations_successful_recovery():
    records = [
        {
            "event_id": "e1",
            "node_id": "n1",
            "event_type": "fail",
            "payload": {"failure_class": "timeout"},
            "seq": 0,
        },
        {
            "event_id": "e2",
            "node_id": "n1",
            "event_type": "complete",
            "payload": {},
            "seq": 1,
        },
    ]

    observations = extract_observations(records, project_id=_PROJECT_ID)

    recovery = [o for o in observations if o.pattern == "successful-recovery"]
    assert len(recovery) == 1
    observation = recovery[0]
    assert observation.trigger == {"node_id": "n1", "failure_class": "timeout"}
    assert observation.observed_outcome == "recover"
    assert observation.source_event_ids == ("e1", "e2")
    _validate_all(observations)


def test_extract_observations_correction():
    records = [
        {
            "event_id": "e1",
            "node_id": "n1",
            "event_type": "correction",
            "payload": {"previous_outcome": "fail", "corrected_outcome": "recover"},
            "seq": 0,
        }
    ]

    observations = extract_observations(records, project_id=_PROJECT_ID)

    assert len(observations) == 1
    observation = observations[0]
    assert observation.pattern == "correction"
    assert observation.trigger == {"node_id": "n1", "previous_outcome": "fail"}
    assert observation.observed_outcome == "recover"
    assert observation.source_event_ids == ("e1",)
    _validate_all(observations)


def test_extract_observations_correction_missing_payload_key_is_skipped():
    records = [
        {
            "event_id": "e1",
            "node_id": "n1",
            "event_type": "correction",
            "payload": {"previous_outcome": "fail"},
            "seq": 0,
        }
    ]

    observations = extract_observations(records, project_id=_PROJECT_ID)

    assert observations == []


def test_extract_observations_workflow_efficiency():
    records = [
        {
            "event_id": "e1",
            "node_id": "n1",
            "event_type": "measurement",
            "payload": {"metric": "wall_seconds", "improvement_pct": 12.5},
            "seq": 0,
        }
    ]

    observations = extract_observations(records, project_id=_PROJECT_ID)

    assert len(observations) == 1
    observation = observations[0]
    assert observation.pattern == "workflow-efficiency"
    assert observation.trigger == {"node_id": "n1", "metric": "wall_seconds"}
    assert observation.observed_outcome == "improved"
    assert observation.source_event_ids == ("e1",)
    _validate_all(observations)


def test_extract_observations_workflow_efficiency_non_positive_improvement_is_skipped():
    records = [
        {
            "event_id": "e1",
            "node_id": "n1",
            "event_type": "measurement",
            "payload": {"metric": "wall_seconds", "improvement_pct": 0},
            "seq": 0,
        }
    ]

    observations = extract_observations(records, project_id=_PROJECT_ID)

    assert observations == []


def test_extract_observations_recurrent_failure_missing_seq_is_skipped():
    records = [
        {
            "event_id": "e1",
            "node_id": "n1",
            "event_type": "fail",
            "payload": {"failure_class": "timeout"},
            # missing "seq" -- must not raise KeyError when sorting the group.
        },
        {
            "event_id": "e2",
            "node_id": "n1",
            "event_type": "fail",
            "payload": {"failure_class": "timeout"},
            "seq": 1,
        },
    ]

    # Should not raise despite the first record missing "seq".
    observations = extract_observations(records, project_id=_PROJECT_ID)

    assert observations == []


def test_extract_observations_successful_recovery_missing_seq_is_skipped():
    records = [
        {
            "event_id": "e1",
            "node_id": "n1",
            "event_type": "fail",
            "payload": {"failure_class": "timeout"},
            # missing "seq" -- must not raise KeyError when sorting by node.
        },
        {
            "event_id": "e2",
            "node_id": "n1",
            "event_type": "complete",
            "payload": {},
            "seq": 1,
        },
    ]

    # Should not raise despite the first record missing "seq".
    observations = extract_observations(records, project_id=_PROJECT_ID)

    assert observations == []


def test_extract_observations_skips_malformed_record_missing_node_id():
    records = [
        {
            "event_id": "e1",
            "event_type": "fail",
            "payload": {"failure_class": "timeout"},
            "seq": 0,
        },
        {
            "event_id": "e2",
            "node_id": "n1",
            "event_type": "measurement",
            "payload": {"metric": "wall_seconds", "improvement_pct": 5},
            "seq": 1,
        },
    ]

    # Should not raise despite the first record missing node_id.
    observations = extract_observations(records, project_id=_PROJECT_ID)

    assert len(observations) == 1
    assert observations[0].pattern == "workflow-efficiency"
