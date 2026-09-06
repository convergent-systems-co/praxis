"""Shared data shapes for the bounded-learning subsystem.

Observation mirrors observation.schema.json, HeuristicCandidate mirrors
heuristic-candidate.schema.json. The *_to_document/*_from_document functions
convert between each dataclass and the plain-dict document shape that
praxis_contracts.validator.validate_document validates, following the
convention in praxis_eval/types.py.
"""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

SCHEMA_DIR = Path(__file__).resolve().parent.parent.parent / "schemas" / "v1"
OBSERVATION_SCHEMA_PATH = SCHEMA_DIR / "observation.schema.json"
HEURISTIC_CANDIDATE_SCHEMA_PATH = SCHEMA_DIR / "heuristic-candidate.schema.json"


@dataclass(frozen=True)
class Observation:
    spec_version: str
    observation_id: str
    project_id: str
    pattern: str
    trigger: dict
    observed_outcome: str
    source_event_ids: tuple[str, ...]
    observed_at: str
    confidence: float | None = None


@dataclass(frozen=True)
class HeuristicCandidate:
    spec_version: str
    heuristic_id: str
    project_id: str
    scope: str
    pattern: str
    trigger: dict
    expected_outcome: str
    proposed_configuration: dict
    status: str
    confidence: float
    evidence_ids: tuple[str, ...]
    created_at: str
    updated_at: str
    contradiction_ids: tuple[str, ...] = ()
    parent_heuristic_id: str | None = None
    description: str | None = None


def observation_to_document(observation: Observation) -> dict:
    document: dict = {
        "spec_version": observation.spec_version,
        "observation_id": observation.observation_id,
        "project_id": observation.project_id,
        "pattern": observation.pattern,
        "trigger": dict(observation.trigger),
        "observed_outcome": observation.observed_outcome,
        "source_event_ids": list(observation.source_event_ids),
        "observed_at": observation.observed_at,
    }
    if observation.confidence is not None:
        document["confidence"] = observation.confidence
    return document


def observation_from_document(doc: dict) -> Observation:
    return Observation(
        spec_version=doc["spec_version"],
        observation_id=doc["observation_id"],
        project_id=doc["project_id"],
        pattern=doc["pattern"],
        trigger=dict(doc["trigger"]),
        observed_outcome=doc["observed_outcome"],
        source_event_ids=tuple(doc["source_event_ids"]),
        observed_at=doc["observed_at"],
        confidence=doc.get("confidence"),
    )


def heuristic_candidate_to_document(candidate: HeuristicCandidate) -> dict:
    document: dict = {
        "spec_version": candidate.spec_version,
        "heuristic_id": candidate.heuristic_id,
        "project_id": candidate.project_id,
        "scope": candidate.scope,
        "pattern": candidate.pattern,
        "trigger": dict(candidate.trigger),
        "expected_outcome": candidate.expected_outcome,
        "proposed_configuration": dict(candidate.proposed_configuration),
        "status": candidate.status,
        "confidence": candidate.confidence,
        "evidence_ids": list(candidate.evidence_ids),
        "created_at": candidate.created_at,
        "updated_at": candidate.updated_at,
    }
    if candidate.contradiction_ids:
        document["contradiction_ids"] = list(candidate.contradiction_ids)
    if candidate.parent_heuristic_id is not None:
        document["parent_heuristic_id"] = candidate.parent_heuristic_id
    if candidate.description is not None:
        document["description"] = candidate.description
    return document


def heuristic_candidate_from_document(doc: dict) -> HeuristicCandidate:
    return HeuristicCandidate(
        spec_version=doc["spec_version"],
        heuristic_id=doc["heuristic_id"],
        project_id=doc["project_id"],
        scope=doc["scope"],
        pattern=doc["pattern"],
        trigger=dict(doc["trigger"]),
        expected_outcome=doc["expected_outcome"],
        proposed_configuration=dict(doc["proposed_configuration"]),
        status=doc["status"],
        confidence=doc["confidence"],
        evidence_ids=tuple(doc["evidence_ids"]),
        created_at=doc["created_at"],
        updated_at=doc["updated_at"],
        contradiction_ids=tuple(doc.get("contradiction_ids", ())),
        parent_heuristic_id=doc.get("parent_heuristic_id"),
        description=doc.get("description"),
    )
