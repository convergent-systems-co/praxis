"""Ingestion pipeline: extraction -> observation log -> heuristic clustering/update.

Covers the pipeline-level guarantees layered on top of the already-tested
extraction/heuristics/confidence/observations modules: observations that
share (pattern, trigger) cluster onto one heuristic across ingest_telemetry
calls, a contradicting observation folds into contradiction_ids and lowers
confidence, an observation touching an already-settled ("promoted")
heuristic is skipped without raising (the pipeline-level enforcement of
confidence.apply_observation's fail-closed rule), and every observation is
durably appended to the observation log regardless of whether its heuristic
update succeeded or was skipped.
"""

from __future__ import annotations

from pathlib import Path

from praxis_learning.heuristics import HeuristicRegistry, compute_heuristic_id
from praxis_learning.observations import ObservationLog
from praxis_learning.pipeline import ingest_telemetry
from praxis_learning.types import HeuristicCandidate

_PROJECT_ID = "proj-1"


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


def _correction_heuristic_id(*, node_id: str = "n1", previous_outcome: str = "fail") -> str:
    return compute_heuristic_id(
        _PROJECT_ID, "correction", {"node_id": node_id, "previous_outcome": previous_outcome}
    )


def test_ingest_telemetry_clusters_same_pattern_trigger_into_one_heuristic_with_two_evidence_ids(
    tmp_path: Path,
) -> None:
    log = ObservationLog(tmp_path / "observations")
    registry = HeuristicRegistry(tmp_path / "heuristics")
    records = [
        _correction_record(event_id="e1", corrected_outcome="recover"),
        _correction_record(event_id="e2", corrected_outcome="recover"),
    ]

    touched = ingest_telemetry(
        records, project_id=_PROJECT_ID, observation_log=log, heuristic_registry=registry
    )

    heuristic_id = _correction_heuristic_id()
    assert [candidate.heuristic_id for candidate in touched] == [heuristic_id, heuristic_id]
    assert len(touched[0].evidence_ids) == 1
    assert len(touched[1].evidence_ids) == 2

    stored = registry.get(heuristic_id)
    assert stored is not None
    assert len(stored.evidence_ids) == 2


def test_ingest_telemetry_contradicting_observation_folds_into_contradiction_ids_and_lowers_confidence(
    tmp_path: Path,
) -> None:
    log = ObservationLog(tmp_path / "observations")
    registry = HeuristicRegistry(tmp_path / "heuristics")

    first_touched = ingest_telemetry(
        [_correction_record(event_id="e1", corrected_outcome="recover")],
        project_id=_PROJECT_ID,
        observation_log=log,
        heuristic_registry=registry,
    )
    initial_confidence = first_touched[0].confidence

    second_touched = ingest_telemetry(
        [_correction_record(event_id="e2", corrected_outcome="give-up")],
        project_id=_PROJECT_ID,
        observation_log=log,
        heuristic_registry=registry,
    )

    heuristic_id = _correction_heuristic_id()
    stored = registry.get(heuristic_id)
    assert stored is not None
    assert len(stored.contradiction_ids) == 1
    assert stored.confidence < initial_confidence
    assert second_touched == [stored]


def test_ingest_telemetry_skips_update_for_already_promoted_heuristic_without_raising(
    tmp_path: Path,
) -> None:
    log = ObservationLog(tmp_path / "observations")
    registry = HeuristicRegistry(tmp_path / "heuristics")

    heuristic_id = _correction_heuristic_id()
    trigger = {"node_id": "n1", "previous_outcome": "fail"}
    promoted = HeuristicCandidate(
        spec_version="1.0.0",
        heuristic_id=heuristic_id,
        project_id=_PROJECT_ID,
        scope="project",
        pattern="correction",
        trigger=trigger,
        expected_outcome="recover",
        proposed_configuration={},
        status="promoted",
        confidence=0.9,
        evidence_ids=("seed-observation",),
        contradiction_ids=(),
        created_at="2026-08-01T00:00:00+00:00",
        updated_at="2026-08-01T00:00:00+00:00",
    )
    registry.save(promoted)

    touched = ingest_telemetry(
        [_correction_record(event_id="e1", corrected_outcome="recover")],
        project_id=_PROJECT_ID,
        observation_log=log,
        heuristic_registry=registry,
    )

    assert touched == []
    stored = registry.get(heuristic_id)
    assert stored == promoted


def test_ingest_telemetry_logs_every_observation_regardless_of_heuristic_outcome(
    tmp_path: Path,
) -> None:
    log = ObservationLog(tmp_path / "observations")
    registry = HeuristicRegistry(tmp_path / "heuristics")

    promoted_heuristic_id = _correction_heuristic_id(node_id="n1")
    promoted_trigger = {"node_id": "n1", "previous_outcome": "fail"}
    registry.save(
        HeuristicCandidate(
            spec_version="1.0.0",
            heuristic_id=promoted_heuristic_id,
            project_id=_PROJECT_ID,
            scope="project",
            pattern="correction",
            trigger=promoted_trigger,
            expected_outcome="recover",
            proposed_configuration={},
            status="promoted",
            confidence=0.9,
            evidence_ids=("seed-observation",),
            contradiction_ids=(),
            created_at="2026-08-01T00:00:00+00:00",
            updated_at="2026-08-01T00:00:00+00:00",
        )
    )

    records = [
        _correction_record(event_id="e1", node_id="n1", corrected_outcome="recover"),
        _correction_record(event_id="e2", node_id="n2", corrected_outcome="recover"),
    ]

    touched = ingest_telemetry(
        records, project_id=_PROJECT_ID, observation_log=log, heuristic_registry=registry
    )

    # Only the second observation (node_id n2, a brand-new heuristic) results
    # in a touched heuristic; the first was skipped as already-promoted.
    assert len(touched) == 1
    assert touched[0].heuristic_id == _correction_heuristic_id(node_id="n2")

    logged = log.read_all()
    assert len(logged) == 2
    assert {tuple(observation.source_event_ids) for observation in logged} == {("e1",), ("e2",)}
