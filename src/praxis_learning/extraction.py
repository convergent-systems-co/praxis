"""Observation/event extraction from telemetry records.

Classifies raw telemetry (plain dicts shaped like praxis_runtime.events.Event
documents) into Observations under four illustrative, non-exhaustive
patterns. "fail"/"complete" are real, stable praxis_runtime.transitions
event_type values (see _TRANSITIONS in src/praxis_runtime/transitions.py);
"correction"/"measurement" are illustrative extension points this bundle
defines, not existing runtime event types. Extraction is best-effort over
telemetry it does not control the shape of: a malformed record is skipped,
never raised.
"""

from __future__ import annotations

import uuid
from datetime import datetime, timezone

from praxis_contracts.validator import validate_document
from praxis_learning.types import Observation, OBSERVATION_SCHEMA_PATH, observation_to_document

_SPEC_VERSION = "1.0.0"


def _well_formed(record: dict) -> bool:
    if not isinstance(record, dict):
        return False
    if not isinstance(record.get("payload"), dict):
        return False
    for key in ("node_id", "event_type", "event_id"):
        if not record.get(key):
            return False
    return True


def _make_observation(*, pattern: str, project_id: str, trigger: dict, observed_outcome: str, source_event_ids: tuple[str, ...]) -> Observation:
    return Observation(
        spec_version=_SPEC_VERSION,
        observation_id=uuid.uuid4().hex,
        project_id=project_id,
        pattern=pattern,
        trigger=trigger,
        observed_outcome=observed_outcome,
        source_event_ids=source_event_ids,
        observed_at=datetime.now(timezone.utc).isoformat(),
    )


def _extract_recurrent_failures(records: list[dict], *, project_id: str) -> list[Observation]:
    groups: dict[tuple[str, object], list[dict]] = {}
    for record in records:
        if record["event_type"] != "fail":
            continue
        key = (record["node_id"], record["payload"].get("failure_class"))
        groups.setdefault(key, []).append(record)

    observations = []
    for (node_id, failure_class), group in groups.items():
        if len(group) < 2:
            continue
        ordered = sorted(group, key=lambda r: r["seq"])
        observations.append(
            _make_observation(
                pattern="recurrent-failure",
                project_id=project_id,
                trigger={"node_id": node_id, "failure_class": failure_class},
                observed_outcome="fail",
                source_event_ids=tuple(r["event_id"] for r in ordered),
            )
        )
    return observations


def _extract_successful_recoveries(records: list[dict], *, project_id: str) -> list[Observation]:
    by_node: dict[str, list[dict]] = {}
    for record in records:
        by_node.setdefault(record["node_id"], []).append(record)

    observations = []
    for node_id, node_records in by_node.items():
        ordered = sorted(node_records, key=lambda r: r["seq"])
        pending_fail = None
        for record in ordered:
            event_type = record["event_type"]
            if event_type == "fail":
                pending_fail = record
            elif event_type == "complete":
                if pending_fail is not None:
                    observations.append(
                        _make_observation(
                            pattern="successful-recovery",
                            project_id=project_id,
                            trigger={
                                "node_id": node_id,
                                "failure_class": pending_fail["payload"].get("failure_class"),
                            },
                            observed_outcome="recover",
                            source_event_ids=(pending_fail["event_id"], record["event_id"]),
                        )
                    )
                pending_fail = None
    return observations


def _extract_corrections(records: list[dict], *, project_id: str) -> list[Observation]:
    observations = []
    for record in records:
        if record["event_type"] != "correction":
            continue
        payload = record["payload"]
        if "previous_outcome" not in payload or "corrected_outcome" not in payload:
            continue
        observations.append(
            _make_observation(
                pattern="correction",
                project_id=project_id,
                trigger={"node_id": record["node_id"], "previous_outcome": payload["previous_outcome"]},
                observed_outcome=payload["corrected_outcome"],
                source_event_ids=(record["event_id"],),
            )
        )
    return observations


def _extract_workflow_efficiency(records: list[dict], *, project_id: str) -> list[Observation]:
    observations = []
    for record in records:
        if record["event_type"] != "measurement":
            continue
        payload = record["payload"]
        if "metric" not in payload:
            continue
        improvement_pct = payload.get("improvement_pct")
        if not isinstance(improvement_pct, (int, float)) or isinstance(improvement_pct, bool):
            continue
        if improvement_pct <= 0:
            continue
        observations.append(
            _make_observation(
                pattern="workflow-efficiency",
                project_id=project_id,
                trigger={"node_id": record["node_id"], "metric": payload["metric"]},
                observed_outcome="improved",
                source_event_ids=(record["event_id"],),
            )
        )
    return observations


def extract_observations(telemetry_records: list[dict], *, project_id: str) -> list[Observation]:
    well_formed = [record for record in telemetry_records if _well_formed(record)]

    observations = [
        *_extract_recurrent_failures(well_formed, project_id=project_id),
        *_extract_successful_recoveries(well_formed, project_id=project_id),
        *_extract_corrections(well_formed, project_id=project_id),
        *_extract_workflow_efficiency(well_formed, project_id=project_id),
    ]

    for observation in observations:
        validate_document(observation_to_document(observation), OBSERVATION_SCHEMA_PATH)

    return observations
